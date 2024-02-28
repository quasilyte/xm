package xm

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/quasilyte/xm/internal/xmdb"
	"github.com/quasilyte/xm/xmfile"
)

type moduleCompiler struct {
	result module

	effectSet map[uint64]effectKey

	effectBuf []xmdb.Effect

	samplePool []int16

	subSamples bool
}

func compileModule(m *xmfile.Module, config moduleConfig) (module, error) {
	c := &moduleCompiler{
		effectSet:  make(map[uint64]effectKey, 24),
		effectBuf:  make([]xmdb.Effect, 0, 4),
		subSamples: config.subSamples,
	}
	c.result = module{
		sampleRate:  float64(config.sampleRate),
		bpm:         float64(config.bpm),
		ticksPerRow: int(config.tempo),
		effectTab:   make([]noteEffect, 0, 24),
		noteTab:     make([]patternNote, len(m.Notes)),
	}
	err := c.compile(m)
	return c.result, err
}

func (c *moduleCompiler) compile(m *xmfile.Module) error {
	if (m.Flags & (0b1)) != 1 {
		return errors.New("the Amiga frequency table is not supported yet")
	}

	c.result.samplesPerTick, c.result.bytesPerTick = calcSamplesPerTick(c.result.sampleRate, c.result.bpm)
	c.result.secondsPerRow = calcSecondsPerRow(c.result.ticksPerRow, c.result.bpm)

	if err := c.compileInstruments(m); err != nil {
		return err
	}

	if err := c.compilePatterns(m); err != nil {
		return err
	}

	// Not assign pattern note flags.
	for i := range c.result.patterns {
		p := &c.result.patterns[i]
		for j := range p.notes {
			n := &c.result.noteTab[p.notes[j]]
			// Some flags could be already set, therefore we need to use |= assignment.
			n.flags |= c.generateNoteFlags(n)
		}
	}

	if len(c.samplePool) != 0 {
		panic("miscalculated sample pool size?")
	}

	return nil
}

func (c *moduleCompiler) makeSampleBuf(l int) []int16 {
	if len(c.samplePool) < l {
		// Should never happen.
		panic("failed to preallocate just enough memory?")
	}
	buf := c.samplePool[:l]
	c.samplePool = c.samplePool[l:]
	return buf
}

func (c *moduleCompiler) compileInstruments(m *xmfile.Module) error {
	c.result.instruments = make([]instrument, m.NumInstruments)
	for i, rawInst := range m.Instruments {
		if len(rawInst.Samples) == 0 {
			continue
		}
		dstInst, err := c.compileInstrument(rawInst)
		if err != nil {
			return fmt.Errorf("instrument[%d (%02X)]: %w", i+1, i+1, err)
		}
		dstInst.id = i
		c.result.instruments[i] = dstInst
	}

	combinedSampleSize := 0
	for i := range m.Instruments {
		rawInst := m.Instruments[i]
		if len(rawInst.Samples) == 0 {
			continue
		}
		dstInst := &c.result.instruments[i]
		combinedSampleSize += c.calculateTotalSampleSize(dstInst, &rawInst.Samples[0])
	}
	// This 1 allocation should be enough for all samples.
	c.samplePool = make([]int16, combinedSampleSize)

	// Now we have the memory to allocate and load the samples.
	for i := range m.Instruments {
		rawInst := m.Instruments[i]
		if len(rawInst.Samples) == 0 {
			continue
		}
		dstInst := &c.result.instruments[i]
		c.loadInstrumentSample(dstInst, &rawInst.Samples[0])
	}

	return nil
}

func (c *moduleCompiler) loadInstrumentSample(inst *instrument, sample *xmfile.InstrumentSample) {
	// dstSamples is large enough to store the extended loop as well as sub-samples.
	// We'll ignore sub-samples during the processing and then add them in a separate step.
	// This makes the code a little bit easier to understand and less prone to nasty bugs.
	dstSamples := c.makeSampleBuf(c.calculateTotalSampleSize(inst, sample))
	numSamples := c.numSamples(sample)
	sampleSize := c.calculateSampleSize(inst, sample)

	if sample.Is16bits() {
		v := int16(0)
		k := 0
		for i := 0; i < len(sample.Data); i += 2 {
			u := binary.LittleEndian.Uint16(sample.Data[i:])
			v += int16(u)
			dstSamples[k] = v
			k++
		}
	} else {
		// Convert 8-bit samples into signed 16-bit samples.
		// Also note that sample.Data stores deltas while
		// dstInst will store the absolute values.
		v := int8(0)
		for i, delta := range sample.Data {
			v += int8(delta)
			dstSamples[i] = int16((int(v) << 8))
		}
	}

	switch sample.LoopType() {
	case xmfile.SampleLoopNone:
		// Make it work by making loopEnd unreachable.
		inst.loopEnd = math.MaxInt
	case xmfile.SampleLoopForward:
		// Do nothing.
	case xmfile.SampleLoopPingPong:
		// Turn ping-pong loop into a forward loop.
		// [1 2 3 4 5] => [1 2 3 4 5 | 4 3 2]
		// [1 2 3 4]   => [1 2 3 4 | 3 2]
		loopLength := int(inst.loopLength)
		numExtraSamples := loopLength - 2
		inst.loopLength += float64(numExtraSamples)
		inst.loopEnd += float64(numExtraSamples)
		for i := 0; i < numExtraSamples; i++ {
			dstIndex := numSamples + i
			srcIndex := numSamples - 2 - i
			dstSamples[dstIndex] = dstSamples[srcIndex]
		}
	}

	inst.samples = dstSamples
	inst.sampleStepMultiplier = 1.0
	if c.subSamples {
		c.insertSubSamples(inst, sample, sampleSize)
	}
}

func (c *moduleCompiler) insertSubSamples(inst *instrument, sample *xmfile.InstrumentSample, sampleSize int) {
	// Sub samples make the compiler harder, but they do make the playback faster

	numSub := c.numSubSamples(sample)
	if numSub == 0 {
		return
	}
	samplesToProcess := sampleSize - 1
	subSamplesToProcess := (sampleSize - 1) * numSub

	// To avoid writing over the data we want to read during the next iteration,
	// we iterate from the end of the array.
	// That trailing part of the array is zero-filled and we can write over it easily.
	// By the time we'll start overwriting data, it will become unrelated.

	tStep := 0.0
	switch numSub {
	case 1:
		tStep = 0.5
	case 3:
		tStep = 0.25
	case 4:
		tStep = 0.2
	case 7:
		tStep = 0.125
	}

	dstSamples := inst.samples

	k := len(inst.samples) - 1
	kStep := numSub + 1
	for i := samplesToProcess; i > 0; i-- {
		t := tStep
		u := dstSamples[i-1]
		v := dstSamples[i]
		uf := float64(u)
		vf := float64(v)
		dstSamples[k-kStep] = u
		dstSamples[k] = v
		for j := 0; j < numSub; j++ {
			index := k - j - 1
			dstSamples[index] = int16(lerp(vf, uf, t))
			t += tStep
			subSamplesToProcess--
		}
		k -= kStep
	}

	inst.sampleStepMultiplier = float64(sampleSize+((sampleSize-1)*numSub)) / float64(sampleSize)

	if sample.LoopType() != xmfile.SampleLoopNone {
		if numSub != 0 {
			inst.numSubSamples = numSub
			inst.loopEnd = float64(int(inst.loopEnd)*(numSub+1) - numSub)
			inst.loopStart = float64(int(inst.loopStart) * (numSub + 1))
			inst.loopLength = float64(int(inst.loopLength)*(numSub+1) - numSub)
		}
	}

	// Some sanity checks.
	if subSamplesToProcess != 0 {
		panic("processed sub-sample count mismatched")
	}
}

func (c *moduleCompiler) compileInstrument(inst xmfile.Instrument) (instrument, error) {
	if len(inst.Samples) != 1 {
		return instrument{}, fmt.Errorf("multi-sample instruments are not supported yet (found %d)", len(inst.Samples))
	}

	sample := &inst.Samples[0]

	loopEnd := sample.LoopStart + sample.LoopLength
	loopStart := sample.LoopStart
	loopLength := sample.LoopLength
	if sample.LoopStart > sample.Length {
		loopStart = loopLength
	}
	if loopEnd > sample.Length {
		loopEnd = sample.Length
	}
	loopLength = loopEnd - loopStart
	if sample.Is16bits() {
		loopEnd /= 2
		loopStart /= 2
		loopLength /= 2
	}
	switch sample.LoopType() {
	case xmfile.SampleLoopNone:
		// OK.
	case xmfile.SampleLoopForward:
		if loopStart > loopEnd {
			return instrument{}, errors.New("sample loopStart > loopEnd")
		}
	case xmfile.SampleLoopPingPong:
		if len(sample.Data) < 2 || loopLength < 2 {
			return instrument{}, errors.New("a ping-pong sample loop can't be shorter than 2")
		}
	default:
		return instrument{}, errors.New("unsupported loop type (one shot?)")
	}

	volumeEnvelope := c.compileEnvelope(inst.EnvelopeVolume, inst.VolumeFlags,
		inst.VolumeSustainPoint, inst.VolumeLoopStartPoint, inst.VolumeLoopEndPoint)
	panningEnvelope := c.compileEnvelope(inst.EnvelopePanning, inst.PanningFlags,
		inst.PanningSustainPoint, inst.PanningLoopStartPoint, inst.PanningLoopEndPoint)

	dstInst := instrument{
		finetune:     int8(sample.Finetune),
		relativeNote: int8(sample.RelativeNote),

		volume:  float64(sample.Volume) / 64,
		panning: float64(sample.Panning) / 256,

		volumeEnvelope:  volumeEnvelope,
		panningEnvelope: panningEnvelope,

		volumeFadeoutStep: float64(inst.VolumeFadeout) / 32768,

		loopType:   sample.LoopType(),
		loopLength: float64(loopLength),
		loopStart:  float64(loopStart),
		loopEnd:    float64(loopEnd),

		sample16bit: sample.Is16bits(),
	}

	switch dstInst.loopType {
	case xmfile.SampleLoopForward, xmfile.SampleLoopNone, xmfile.SampleLoopPingPong:
		// OK
	default:
		return dstInst, errors.New("unknown sample loop type")
	}

	return dstInst, nil
}

func (c *moduleCompiler) compileEnvelope(points []xmfile.EnvelopePoint, flags xmfile.EnvelopeFlags, sustain, start, end uint8) envelope {
	e := envelope{
		flags:          flags,
		sustainPoint:   sustain,
		loopEndPoint:   end,
		loopStartPoint: start,
	}

	if len(points) > 0 {
		e.loopStartPoint = clampMax(e.loopStartPoint, uint8(len(points))-1)
		e.loopEndPoint = clampMax(e.loopEndPoint, uint8(len(points))-1)
		e.points = make([]envelopePoint, len(points))
		for i, p := range points {
			e.points[i] = envelopePoint{
				frame: int(p.X),
				value: float64(p.Y),
			}
		}

		e.sustainFrame = int(points[e.sustainPoint].X)
		e.loopEndFrame = int(points[e.loopEndPoint].X)
		loopStartFrame := int(points[e.loopStartPoint].X)
		e.loopLength = e.loopEndFrame - loopStartFrame
	}

	return e
}

func (c *moduleCompiler) compilePatterns(m *xmfile.Module) error {
	c.result.patterns = make([]pattern, m.NumPatterns)
	c.result.patternOrder = make([]*pattern, len(m.PatternOrder))

	// Bind pattern order to the actual patterns.
	for i, patternIndex := range m.PatternOrder {
		c.result.patternOrder[i] = &c.result.patterns[patternIndex]
	}

	numNotes := 0
	for i := range m.Patterns {
		numNotes += len(m.Patterns[i].Rows) * m.NumChannels
	}

	noteSlicePool := make([]uint16, numNotes)
	noteSliceOffset := 0

	for i := range m.Patterns {
		rawPat := &m.Patterns[i]
		pat := &c.result.patterns[i]
		pat.numChannels = m.NumChannels
		pat.numRows = len(rawPat.Rows)

		numNotes := len(rawPat.Rows) * m.NumChannels
		pat.notes = noteSlicePool[noteSliceOffset : noteSliceOffset+numNotes]
		noteSliceOffset += numNotes

		noteIndex := 0
		for _, row := range rawPat.Rows {
			for _, noteID := range row.Notes {
				rawNote := m.Notes[noteID]
				var n patternNote
				var inst *instrument
				badInstrument := false
				if rawNote.Instrument != 0 {
					i := int(rawNote.Instrument) - 1
					if i < len(c.result.instruments) {
						inst = &c.result.instruments[i]
					} else {
						badInstrument = true
					}
				}

				fnote := float64(rawNote.Note)
				period := 0.0
				isValid := rawNote.Note > 0 && rawNote.Note < 97
				if isValid && rawNote.Instrument > 0 {
					period = linearPeriod(calcRealNote(fnote, inst))
				}

				e1 := xmdb.Effect{}
				if rawNote.Note == 97 {
					e1.Op = xmdb.EffectEarlyKeyOff
				}
				e2 := xmdb.EffectFromVolumeByte(rawNote.Volume)
				e3 := xmdb.ConvertEffect(rawNote)
				ek, err := c.compileEffect(e1, e2, e3)
				if err != nil {
					return err
				}

				n = patternNote{
					raw:    fnote,
					period: period,
					inst:   inst,
					effect: ek,
				}
				if isValid {
					n.flags |= noteValid
				}
				if badInstrument {
					n.flags |= noteBadInstrument
				}

				var kind patternNoteKind
				switch {
				case rawNote.Note == 0 && rawNote.Instrument == 0:
					kind = noteEmpty
				case rawNote.Note == 0 && rawNote.Instrument > 0:
					kind = noteGhostInstrument
				case n.flags.Contains(noteValid) && rawNote.Instrument == 0:
					kind = noteGhost
				case n.flags.Contains(noteValid) && rawNote.Instrument > 0:
					kind = noteNormal
				default:
					// Probably a special note like "key off".
					kind = noteEmpty
				}
				n.flags |= patternNoteFlags(kind) << (64 - 2)

				pat.notes[noteIndex] = noteID
				if !c.result.noteTab[noteID].flags.Contains(noteInitialized) {
					n.flags |= noteInitialized
					c.result.noteTab[noteID] = n
				}
				noteIndex++
			}
		}
	}

	return nil
}

func (c *moduleCompiler) generateNoteFlags(n *patternNote) patternNoteFlags {
	var flags patternNoteFlags

	numEffects := n.effect.Len()
	offset := n.effect.Index()
	for _, e := range c.result.effectTab[offset : offset+numEffects] {
		switch e.op {
		case xmdb.EffectNotePortamento:
			flags |= noteHasNotePortamento
		case xmdb.EffectArpeggio:
			flags |= noteHasArpeggio
		case xmdb.EffectVibrato, xmdb.EffectVibratoWithVolumeSlide:
			flags |= noteHasVibrato
		}
	}

	return flags
}

func (c *moduleCompiler) compileEffect(e1, e2, e3 xmdb.Effect) (effectKey, error) {
	hash := (uint64(e1.AsUint16()) << (0 * 16)) | (uint64(e2.AsUint16()) << (1 * 16)) | (uint64(e3.AsUint16()) << (2 * 16))
	if hash == 0 {
		return effectKey(0), nil
	}
	if k, ok := c.effectSet[hash]; ok {
		// This effects combination is already interned.
		return k, nil
	}

	index := len(c.result.effectTab)

	buf := c.effectBuf[:0]
	if e1.Op != xmdb.EffectNone {
		buf = append(buf, e1)
	}
	if e2.Op != xmdb.EffectNone {
		buf = append(buf, e2)
	}
	if e3.Op != xmdb.EffectNone {
		buf = append(buf, e3)
	}

	realLength := 0
	for _, e := range buf {
		compiled := noteEffect{
			op:       e.Op,
			rawValue: e.Arg,
		}

		switch e.Op {
		case xmdb.EffectSetVolume, xmdb.EffectSetGlobalVolume:
			v := e.Arg
			if v > 64 {
				v = 64
			}
			compiled.floatValue = float64(v) / 64

		case xmdb.EffectKeyOff:
			if e.Arg > uint8(c.result.ticksPerRow-1) {
				// This effect will have no effect. Discard it.
				continue
			}

		case xmdb.EffectArpeggio:
			compiled.arp[0] = 0              // The original note
			compiled.arp[1] = e.Arg >> 4     // X note delta
			compiled.arp[2] = e.Arg & 0b1111 // Y note delta

			// TODO: depending on the tracker-style, use XY or YX order?
			// For now, use Fasttracker II convention with YX.
			compiled.arp[1], compiled.arp[2] = compiled.arp[2], compiled.arp[1]

		case xmdb.EffectVolumeSlideUp, xmdb.EffectVolumeSlideDown, xmdb.EffectFineVolumeSlideUp, xmdb.EffectFineVolumeSlideDown:
			compiled.floatValue = float64(e.Arg) / 64

		case xmdb.EffectPortamentoUp, xmdb.EffectPortamentoDown, xmdb.EffectNotePortamento:
			compiled.floatValue = float64(e.Arg) * 4

		case xmdb.EffectVibrato:
			compiled.arp[0] = e.Arg >> 4                       // speed
			compiled.floatValue = float64(e.Arg&0b1111) / 0x0F // depth

		case xmdb.EffectVolumeSlide, xmdb.EffectVibratoWithVolumeSlide, xmdb.EffectGlobalVolumeSlide:
			slideUp := e.Arg >> 4
			slideDown := e.Arg & 0b1111
			if slideUp > 0 && slideDown > 0 {
				return effectKey(0), errors.New("volume slide uses both up & down (XY) values")
			}
			if slideUp > 0 {
				compiled.floatValue = float64(slideUp) / 64
			} else {
				compiled.floatValue = -(float64(slideDown) / 64)
			}

		case xmdb.EffectPatternBreak:
			compiled.arp[0] = (e.Arg>>4)*10 + (e.Arg & 0b1111)

		case xmdb.EffectSetPanning:
			compiled.floatValue = float64(e.Arg) / 255

		case xmdb.EffectSetBPM:
			compiled.floatValue = float64(e.Arg)

		case xmdb.EffectNoteCut:
			compiled.arp[0] = e.Arg & 0b1111

		case xmdb.EffectPanningSlide:
			slideRight := e.Arg >> 4
			slideLeft := e.Arg & 0b1111
			if slideRight > 0 && slideLeft > 0 {
				return effectKey(0), errors.New("panning slide uses both right & left (XY) values")
			}
			if slideRight > 0 {
				compiled.floatValue = float64(slideRight) / 255
			} else {
				compiled.floatValue = -(float64(slideLeft) / 255)
			}

		case xmdb.EffectPanningSlideLeft, xmdb.EffectPanningSlideRight:
			compiled.floatValue = float64(e.Arg) / 255

		case xmdb.EffectSampleOffset:
			compiled.floatValue = float64(e.Arg) * 256
		}

		c.result.effectTab = append(c.result.effectTab, compiled)
		realLength++
	}

	k := makeEffectKey(uint(index), uint(realLength))
	c.effectSet[hash] = k
	return k, nil
}

func (c *moduleCompiler) calculateSampleSize(inst *instrument, sample *xmfile.InstrumentSample) int {
	n := c.numSamples(sample)
	if sample.LoopType() == xmfile.SampleLoopPingPong {
		n += int(inst.loopLength) - 2
	}
	return n
}

func (c *moduleCompiler) calculateTotalSampleSize(inst *instrument, sample *xmfile.InstrumentSample) int {
	n := c.calculateSampleSize(inst, sample)
	if numSub := c.numSubSamples(sample); numSub != 0 {
		n += (n - 1) * numSub
	}
	return n
}

func (c *moduleCompiler) numSamples(sample *xmfile.InstrumentSample) int {
	n := len(sample.Data)
	if sample.Is16bits() {
		n /= 2
	}
	return n
}

func (c *moduleCompiler) numSubSamples(sample *xmfile.InstrumentSample) int {
	if !c.subSamples {
		return 0
	}
	// Depending on the sample size, insert different number of sub-samples.
	numSamples := c.numSamples(sample)
	switch {
	case numSamples <= 1:
		return 0
	case numSamples <= 256:
		return 7
	case numSamples <= 1024:
		return 4
	case numSamples <= 4096:
		return 3
	default:
		return 1
	}
}
