package xm

import (
	"encoding/binary"
	"errors"
	"math"

	"github.com/quasilyte/xm/internal/xmdb"
	"github.com/quasilyte/xm/xmfile"
)

type Stream struct {
	module module

	pattern           *pattern
	patternIndex      int
	patternRowsRemain int
	patternRowIndex   int
	rowTicksRemain    int
	tickIndex         int

	// Pattern break state.
	jumpKind    jumpKind
	jumpPattern int
	jumpRow     int

	volumeScaling float64

	// These values can change during the playback.
	bpm            float64
	samplesPerTick float64
	ticksPerRow    int // Also known as "tempo" and "spd"
	bytesPerTick   int

	channels []streamChannel
}

type jumpKind uint8

const (
	jumpNone jumpKind = iota
	jumpPatternBreak
)

type StreamInfo struct {
	BytesPerTick uint
}

// LoadModuleConfig configures the XM module loading.
//
// These settings can't be changed after a module is loaded.
//
// Some extra configurations are available via Stream methods:
//   - Stream.SetVolume()
//
// These extra configuration methods can be used even after a module is loaded.
type LoadModuleConfig struct {
	// BPM sets the playback speed.
	// Higher BPM will make the music play faster.
	//
	// A zero value will use the XM module default BPM value.
	// If that value is zero as well, a value of 120 will be used.
	BPM uint

	// Tempo (called "Spd" in MilkyTracker) specifies the number of ticks per pattern row.
	// Perhaps a bit counter-intuitively, higher values make
	// the song play slower as there are more resolution steps inside a
	// single pattern row.
	//
	// A zero value will use the XM module default Tempo value.
	// If that value is zero as well, a value of 6 will be used.
	// (6 is a default value in MilkyTracker.)
	Tempo uint

	// The sound device sample rate.
	// If you're using Ebitengine, it's the same value that
	// was used to create an audio context.
	// The most common value is 44100.
	//
	// A zero value will assume a sample rate of 44100.
	//
	// Note: only two values are supported right now, 44100 and 0.
	// Therefore, you can only play XM tracks at sample rate of 44100.
	// This limitation can go away later.
	SampleRate uint
}

// NewPlayer allocates a player that can load and play XM tracks.
// Use LoadModule method to finish player initialization.
func NewStream() *Stream {
	return &Stream{
		volumeScaling: 0.8,
	}
}

// SetVolume adjusts the global volume scaling for the stream.
// The default value is 0.8; a value of 0 disables the sound.
// The value is clamped in [0, 1].
func (s *Stream) SetVolume(v float64) {
	s.volumeScaling = clamp(v, 0, 1)
}

func (s *Stream) LoadModule(m *xmfile.Module, config LoadModuleConfig) error {
	if config.SampleRate == 0 {
		config.SampleRate = 44100
	}
	if config.BPM == 0 {
		config.BPM = uint(m.DefaultBPM)
		if config.BPM == 0 {
			config.BPM = 120
		}
	}
	if config.Tempo == 0 {
		config.Tempo = uint(m.DefaultTempo)
		if config.Tempo == 0 {
			config.Tempo = 6
		}
	}

	if config.SampleRate != 44100 {
		return errors.New("unsupported sample rate (only 44100 is supported)")
	}

	if cap(s.channels) < m.NumChannels {
		s.channels = make([]streamChannel, m.NumChannels)
	}
	s.channels = s.channels[:m.NumChannels]

	compiled, err := compileModule(m, moduleConfig{
		sampleRate: config.SampleRate,
		bpm:        config.BPM,
		tempo:      config.Tempo,
	})
	if err != nil {
		return err
	}
	s.module = compiled

	s.Rewind()

	return nil
}

// Read puts next PCM bytes into provided slice.
//
// The slice is expected to fit at least a single tick.
// With BPM=120, Tempo=10 and SampleRate=44100 a single tick
// would require 882*bytesPerSample*numChannels = 2208 bytes.
// Note that this library only supports stereo output (numChannels=2)
// and it produces 16-bit (2 bytes per sample) LE PCM data.
// If you need to have precise info, use Stream.GetInfo() method.
//
// If there is a tail in b that was not written to due to the lack
// of space for a whole tick, n<len(b) will be returned.
// It doesn't make send to pass a slice that is smaller than a single
// tick chunk (2k+ bytes), but it makes sense to pass a bigger slice
// as this method will try to fit as many ticks as possible.
//
// When stream has no bytes to produce, io.EOF error is returned.
func (s *Stream) Read(b []byte) (int, error) {
	written := 0

	bytesPerTick := s.module.bytesPerTick
	for len(b) > bytesPerTick {
		s.nextTick()
		s.readTick(b[:bytesPerTick])

		written += bytesPerTick
		b = b[bytesPerTick:]
	}

	return written, nil
}

func (s *Stream) Rewind() {
	s.patternIndex = -1
	s.patternRowsRemain = 0
	s.patternRowIndex = -1
	s.rowTicksRemain = 0
	s.tickIndex = -1

	s.ticksPerRow = s.module.ticksPerRow
	s.setBPM(s.module.bpm)
}

func (s *Stream) setBPM(bpm float64) {
	s.bpm = bpm
	s.samplesPerTick, s.bytesPerTick = calcSamplesPerTick(s.module.sampleRate, s.bpm)
}

func (s *Stream) GetInfo() StreamInfo {
	return StreamInfo{
		BytesPerTick: uint(s.module.bytesPerTick),
	}
}

func (s *Stream) nextTick() {
	if s.rowTicksRemain == 0 {
		s.nextRow()
	}

	s.rowTicksRemain--
	s.tickIndex++

	for j := range s.channels {
		ch := &s.channels[j]
		note := ch.note

		s.tickEnvelopes(ch)

		panning := ch.panning + (ch.panningEnvelope.value-0.5)*(0.5-abs(ch.panning-0.5))*2

		volume := s.volumeScaling * ch.volume * ch.fadeoutVolume * ch.volumeEnvelope.value
		ch.computedVolume[0] = volume * math.Sqrt(1.0-panning)
		ch.computedVolume[1] = volume * math.Sqrt(panning)

		if !ch.effect.IsEmpty() {
			s.applyTickEffect(ch)
		}

		if ch.arpeggioRunning && !note.flags.Contains(noteHasArpeggio) {
			ch.arpeggioRunning = false
			ch.arpeggioNoteOffset = 0
		}
		if ch.vibratoRunning && !note.flags.Contains(noteHasVibrato) {
			ch.vibratoRunning = false
			ch.vibratoPeriodOffset = 0
		}

		freq := linearFrequency(ch.period - (64 * ch.arpeggioNoteOffset) - (16 * ch.vibratoPeriodOffset))
		ch.sampleStep = freq / s.module.sampleRate
	}
}

func (s *Stream) tickEnvelopes(ch *streamChannel) {
	if ch.inst == nil {
		return
	}

	if ch.volumeEnvelope.flags.IsOn() {
		if !ch.keyOn {
			ch.fadeoutVolume = clampMin(ch.fadeoutVolume-ch.inst.volumeFadeoutStep, 0)
		}
		s.envelopeTick(ch, &ch.volumeEnvelope)
	}

	if ch.panningEnvelope.flags.IsOn() {
		s.envelopeTick(ch, &ch.panningEnvelope)
	}
}

func (s *Stream) envelopeTick(ch *streamChannel, e *envelopeRunner) {
	if len(e.points) < 2 {
		panic("unimplemented")
	}

	if e.flags.LoopEnabled() {
		if e.frame >= e.loopEndFrame {
			e.frame -= e.loopLength
		}
	}

	i := 0
	for i < len(e.points)-2 {
		if e.points[i].frame <= e.frame && e.points[i+1].frame >= e.frame {
			break
		}
		i++
	}

	e.value = envelopeLerp(e.points[i], e.points[i+1], e.frame) * (1.0 / 64.0)

	if !ch.keyOn || !e.flags.SustainEnabled() || e.frame != e.sustainFrame {
		e.frame++
	}
}

func (s *Stream) nextRow() {
	if s.jumpKind == jumpNone {
		// Normal execution.
		if s.patternRowsRemain == 0 {
			s.nextPattern()
		}
		s.patternRowIndex++
		s.patternRowsRemain--
	} else {
		// Execute a pattern jump.
		s.jumpKind = jumpNone
		s.selectPattern(s.jumpPattern)
		s.patternRowIndex = s.jumpRow
		s.patternRowsRemain = s.pattern.numRows - s.patternRowIndex - 1
	}

	noteOffset := s.pattern.numChannels * s.patternRowIndex
	notes := s.pattern.notes[noteOffset : noteOffset+s.pattern.numChannels]

	for i := range s.channels {
		ch := &s.channels[i]
		n := &notes[i]
		ch.note = n

		inst := n.inst

		hasNotePortamento := n.flags.Contains(noteHasNotePortamento)
		if hasNotePortamento && inst == nil {
			inst = ch.inst
		}

		if inst == nil {
			ch.effect = n.effect
			if n.period != 0 {
				ch.period = n.period
			}
			if ch.inst != nil && ch.sampleOffset >= float64(len(ch.inst.samples)) {
				ch.inst = nil
			}
		} else {
			// Start playing next note.
			if n.period != 0 && !hasNotePortamento {
				ch.sampleOffset = 0
				ch.reverse = false
				ch.period = n.period
			}
			ch.effect = n.effect
			ch.volume = inst.volume
			ch.inst = inst
			ch.panning = inst.panning
			ch.keyOn = true
			ch.fadeoutVolume = 1
			ch.volumeEnvelope.value = 1
			ch.volumeEnvelope.frame = 0
			ch.volumeEnvelope.envelope = inst.volumeEnvelope
			ch.panningEnvelope.value = 0.5
			ch.panningEnvelope.frame = 0
			ch.panningEnvelope.envelope = inst.panningEnvelope
			ch.vibratoPeriodOffset = 0
		}

		if !ch.effect.IsEmpty() {
			s.applyRowEffect(ch, n)
		}
	}

	s.rowTicksRemain = s.ticksPerRow
	s.tickIndex = -1
}

func (s *Stream) applyRowEffect(ch *streamChannel, n *patternNote) {
	numEffects := ch.effect.Len()
	offset := ch.effect.Index()
	for _, e := range s.module.effectTab[offset : offset+numEffects] {
		switch e.op {
		case xmdb.EffectSetVolume:
			ch.volume = e.floatValue

		case xmdb.EffectKeyOff:
			if e.rawValue != 0 {
				break
			}
			s.keyOff(ch)

		case xmdb.EffectVolumeSlide, xmdb.EffectVibratoWithVolumeSlide:
			if e.floatValue != 0 {
				ch.volumeSlideValue = e.floatValue
			}

		case xmdb.EffectPortamentoUp:
			if e.floatValue != 0 {
				ch.portamentoUpValue = e.floatValue
			}

		case xmdb.EffectPortamentoDown:
			if e.floatValue != 0 {
				ch.portamentoDownValue = e.floatValue
			}

		case xmdb.EffectNotePortamento:
			if n.raw == 0 {
				break
			}
			if e.floatValue != 0 {
				ch.notePortamentoValue = e.floatValue
			}
			// TODO: can we precalculate this period in the compiler, somehow?
			ch.notePortamentoTargetPeriod = linearPeriod(calcRealNote(n.raw, ch.inst))

		case xmdb.EffectVibrato:
			if e.arp[0] != 0 {
				ch.vibratoSpeed = e.arp[0]
			}
			if e.floatValue != 0 {
				ch.vibratoDepth = e.floatValue
			}

		case xmdb.EffectPatternBreak:
			s.jumpKind = jumpPatternBreak
			s.jumpPattern = s.patternIndex + 1
			s.jumpRow = int(e.arp[0])

		case xmdb.EffectSetBPM:
			s.setBPM(e.floatValue)

		case xmdb.EffectSetTempo:
			s.ticksPerRow = int(e.rawValue)
		}
	}
}

func (s *Stream) keyOff(ch *streamChannel) {
	ch.keyOn = false
	if ch.inst == nil || !ch.volumeEnvelope.flags.IsOn() {
		ch.volume = 0
	}
}

func (s *Stream) vibrato(ch *streamChannel) {
	ch.vibratoStep += ch.vibratoSpeed
	ch.vibratoPeriodOffset = -2 * waveform(ch.vibratoStep) * ch.vibratoDepth
}

func (s *Stream) applyTickEffect(ch *streamChannel) {
	numEffects := ch.effect.Len()
	offset := ch.effect.Index()

	for _, e := range s.module.effectTab[offset : offset+numEffects] {
		switch e.op {
		case xmdb.EffectPortamentoUp:
			if s.tickIndex == 0 {
				break
			}
			// XM_MINPERIOD is defined as 50 in MilkyTracker.
			ch.period = clampMin(ch.period-ch.portamentoUpValue, 50)

		case xmdb.EffectPortamentoDown:
			if s.tickIndex == 0 {
				break
			}
			ch.period += ch.portamentoDownValue

		case xmdb.EffectNotePortamento:
			if s.tickIndex == 0 {
				break
			}
			if ch.notePortamentoTargetPeriod == 0 {
				break
			}
			if ch.period == ch.notePortamentoTargetPeriod {
				break
			}
			ch.period = slideTowards(ch.period, ch.notePortamentoTargetPeriod, ch.notePortamentoValue)

		case xmdb.EffectVibrato:
			if s.tickIndex == 0 {
				break
			}
			ch.vibratoRunning = true
			s.vibrato(ch)

		case xmdb.EffectKeyOff:
			if e.rawValue != uint8(s.tickIndex) {
				break
			}
			s.keyOff(ch)

		case xmdb.EffectArpeggio:
			i := s.tickIndex % 3
			ch.arpeggioNoteOffset = float64(e.arp[i])
			ch.arpeggioRunning = i != 0

		case xmdb.EffectVolumeSlide:
			if s.tickIndex == 0 {
				break
			}
			ch.volume = clamp(ch.volume+ch.volumeSlideValue, 0, 1)

		case xmdb.EffectVibratoWithVolumeSlide:
			if s.tickIndex == 0 {
				break
			}
			ch.vibratoRunning = true
			s.vibrato(ch)
			ch.volume = clamp(ch.volume+ch.volumeSlideValue, 0, 1)

		case xmdb.EffectVolumeSlideDown:
			ch.volume = clampMin(ch.volume-e.floatValue, 0)
		case xmdb.EffectVolumeSlideUp:
			ch.volume = clampMax(ch.volume+e.floatValue, 1)
		}
	}
}

func (s *Stream) nextPattern() {
	s.selectPattern(s.patternIndex + 1)
}

func (s *Stream) selectPattern(i int) {
	s.patternIndex = i
	s.pattern = s.module.patternOrder[s.patternIndex]

	s.patternRowIndex = -1
	s.patternRowsRemain = s.pattern.numRows
}

func (s *Stream) readTick(b []byte) {
	n := int(s.module.samplesPerTick) * 4
	for i := 0; i < n; i += 4 {
		left := int16(0) // TODO: maybe use floats here?
		right := int16(0)
		for j := range s.channels {
			ch := &s.channels[j]
			inst := ch.inst
			if inst == nil {
				continue
			}
			sampleOffset := int(ch.sampleOffset)
			if sampleOffset >= len(inst.samples) {
				continue
			}
			v := inst.samples[sampleOffset]

			// 0.25 is an amplification heuristic to avoid clipping.
			left += int16(0.25 * float64(v) * ch.computedVolume[0])
			right += int16(0.25 * float64(v) * ch.computedVolume[1])

			switch inst.loopType {
			case xmfile.SampleLoopNone:
				ch.sampleOffset += ch.sampleStep
			case xmfile.SampleLoopForward:
				ch.sampleOffset += ch.sampleStep
				if ch.sampleOffset >= inst.loopEnd {
					for ch.sampleOffset >= inst.loopEnd {
						ch.sampleOffset -= inst.loopLength
					}
				}

			case xmfile.SampleLoopPingPong:
				if ch.reverse {
					ch.sampleOffset -= ch.sampleStep
					if ch.sampleOffset <= inst.loopStart {
						ch.reverse = false
						ch.sampleOffset = abs(float64(int(inst.loopStart) + (int(ch.sampleOffset) % int(inst.loopLength))))
					}
				} else {
					ch.sampleOffset += ch.sampleStep
					if ch.sampleOffset >= inst.loopEnd {
						ch.reverse = true
						ch.sampleOffset = float64(int(inst.loopEnd) - (int(ch.sampleOffset) % int(inst.loopLength)))
					}
				}
			}
		}

		// Stereo channel 1.
		binary.LittleEndian.PutUint16(b[i:], uint16(left))
		// Stereo channel 2.
		binary.LittleEndian.PutUint16(b[i+2:], uint16(right))
	}
}
