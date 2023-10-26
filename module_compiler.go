package xm

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/quasilyte/xm/internal/xmdb"
	"github.com/quasilyte/xm/xmfile"
)

type moduleCompiler struct {
	result module

	effectSet map[uint64]effectKey

	effectBuf []xmdb.Effect
}

func compileModule(m *xmfile.Module, config moduleConfig) (module, error) {
	c := &moduleCompiler{
		effectSet: make(map[uint64]effectKey, 16),
		effectBuf: make([]xmdb.Effect, 0, 4),
	}
	c.result = module{
		sampleRate:  float64(config.sampleRate),
		bpm:         float64(config.bpm),
		ticksPerRow: int(config.tempo),
	}
	err := c.compile(m)
	return c.result, err
}

func (c *moduleCompiler) compile(m *xmfile.Module) error {
	if (m.Flags & (0b1)) != 1 {
		return errors.New("the Amiga frequency table is not supported yet")
	}

	c.result.samplesPerTick, c.result.bytesPerTick = calcSamplesPerTick(c.result.sampleRate, c.result.bpm)

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
			n := &p.notes[j]
			// Some flags could be already set, therefore we need to use |= assignment.
			n.flags |= c.generateNoteFlags(n)
		}
	}

	return nil
}

func (c *moduleCompiler) compileInstruments(m *xmfile.Module) error {
	c.result.instruments = make([]instrument, m.NumInstruments)
	for i, inst := range m.Instruments {
		if len(inst.Samples) == 0 {
			continue
		}
		dstInst, err := c.compileInstrument(m, inst)
		if err != nil {
			return fmt.Errorf("instrument[%d (%02X)]: %w", i, i, err)
		}
		c.result.instruments[i] = dstInst
	}

	return nil
}

func (c *moduleCompiler) compileInstrument(m *xmfile.Module, inst xmfile.Instrument) (instrument, error) {
	if len(inst.Samples) != 1 {
		return instrument{}, fmt.Errorf("multi-sample instruments are not supported yet (found %d)", len(inst.Samples))
	}

	sample := inst.Samples[0]

	numSamples := len(sample.Data)
	if sample.Is16bits() {
		numSamples /= 2
	}
	dstSamples := make([]int16, numSamples)

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
	if sample.LoopType() == xmfile.SampleLoopForward {
		if loopStart > loopEnd {
			return instrument{}, errors.New("sample loopStart > loopEnd")
		}
	}

	if sample.Is16bits() {
		loopEnd /= 2
		loopStart /= 2
		loopLength /= 2

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

	volumeEnvelope := c.compileEnvelope(inst.EnvelopeVolume, inst.VolumeFlags,
		inst.VolumeSustainPoint, inst.VolumeLoopStartPoint, inst.VolumeLoopEndPoint)
	panningEnvelope := c.compileEnvelope(inst.EnvelopePanning, inst.PanningFlags,
		inst.PanningSustainPoint, inst.PanningLoopStartPoint, inst.PanningLoopEndPoint)

	dstInst := instrument{
		samples: dstSamples,

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

	for i := range m.Patterns {
		rawPat := &m.Patterns[i]
		pat := &c.result.patterns[i]
		pat.numChannels = m.NumChannels
		pat.numRows = len(rawPat.Rows)
		pat.notes = make([]patternNote, 0, len(rawPat.Rows)*m.NumChannels)
		var activeInst *instrument
		for _, row := range rawPat.Rows {
			for _, rawNote := range row.Notes {
				var n patternNote
				var inst *instrument
				if rawNote.Instrument != 0 {
					i := int(rawNote.Instrument) - 1
					// TODO: what to do with overflowing instrument?
					if i < len(c.result.instruments) {
						inst = &c.result.instruments[i]
					}
				}

				if inst != nil {
					activeInst = inst
				}

				fnote := float64(rawNote.Note)
				period := linearPeriod(calcRealNote(fnote, activeInst))
				if rawNote.Note == 0 || rawNote.Note == 97 {
					period = 0
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
				if rawNote.Note > 0 && rawNote.Note < 97 {
					n.flags |= noteValid
				}
				pat.notes = append(pat.notes, n)
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
		case xmdb.EffectSetVolume:
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

		case xmdb.EffectVolumeSlide, xmdb.EffectVibratoWithVolumeSlide:
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

		case xmdb.EffectSetBPM:
			compiled.floatValue = float64(e.Arg)
		}

		c.result.effectTab = append(c.result.effectTab, compiled)
		realLength++
	}

	k := makeEffectKey(uint(index), uint(realLength))
	c.effectSet[hash] = k
	return k, nil
}
