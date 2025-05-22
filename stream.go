package xm

import (
	"errors"
	"io"
	"math"

	"github.com/quasilyte/xm/internal/xmdb"
	"github.com/quasilyte/xm/xmfile"
)

// Stream wraps the compiled XM module, making it possible to Read() its PCM bytes.
//
// The Read() method produces 16-bit little endian PCM bytes; this is what ebiten/audio
// package extects. Use Stream as an io.Reader argument for audio.NewPlayer().
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

	settings streamSettings

	// These values can change during the playback.
	globalVolume   float64
	bpm            float64
	samplesPerTick float64
	ticksPerRow    int // Also known as "tempo" and "spd"
	bytesPerTick   int
	bytePos        int // Used to report the current pos via Seek()
	t              float64
	secondsPerRow  float64

	channels       []streamChannel
	activeChannels []*streamChannel
}

type streamSettings struct {
	volumeScaling float64
	loop          bool
	eventHandler  func(e StreamEvent)
}

type jumpKind uint8

const (
	jumpNone jumpKind = iota
	jumpPatternBreak
)

// StreamInfo contains a compiled XM module stream information like bytes per tick, etc.
type StreamInfo struct {
	// BytesPerTick tell how much bytes this stream needs to fit a single XM tick.
	// This value is important, since any slice smaller than this will give no effect
	// for Read() function. Any greater values will work OK for it.
	BytesPerTick uint

	// MemoryUsage approximates the compiled XM module size in bytes.
	// This can be important if you want to analyze linear interpolation (sub-samples)
	// effect on your modules.
	MemoryUsage uint
}

// LoadModuleConfig configures the XM module loading.
//
// These settings can't be changed after a module is loaded.
//
// Some extra configurations are available via Stream methods:
//   - Stream.SetVolume()
//   - Stream.SetLooping()
//
// These extra configuration methods can be used even after a module is loaded.
type LoadModuleConfig struct {
	// LinearInterpolation enables sub-samples that will make some music sound smoother.
	// On average, this option will make loaded track to require ~x2 memory.
	//
	// The best way to figure out whether you need it or not is to listen to the results.
	// Most XM players you can find have linear interpolation (lerp) enabled by default.
	//
	// A zero value means "no interpolation".
	//
	// This should not be confused with volume ramping.
	// The volume ramping is always enabled and can't be turned off.
	LinearInterpolation bool

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
		settings: streamSettings{
			volumeScaling: 0.8,
		},
	}
}

// SetEventHandler installs an event listener to the stream.
//
// f is called on every stream event.
//
// Events are produced when the XM track is being played.
// Therefore, calling Read() may produce multiple events.
//
// Experimental: the events handling API may change significantly in the future.
func (s *Stream) SetEventHandler(f func(e StreamEvent)) {
	s.settings.eventHandler = f
}

// SetVolume adjusts the global volume scaling for the stream.
// The default value is 0.8; a value of 0 disables the sound.
// The value is clamped in [0, 1].
func (s *Stream) SetVolume(v float64) {
	s.settings.volumeScaling = clamp(v, 0, 1)
}

// SetLooping enables a simple looping from the beginning of the stream.
// When looping is enables, Read will never return EOF.
//
// Note that some XM tracks include the trailing jump/pattern break
// effect that will make it loop in a more beautiful way.
// Use this looping flag only if XM track does not have one.
// You may need to perform some XM editing if there is a jump, but you
// still want to use this basic looping scheme.
//
// Note: prefer this option to the InfiniteLoop provided by Ebitengine audio.
// This native way of looping is ~free while InfiniteLoop has some overhead.
func (s *Stream) SetLooping(loop bool) {
	s.settings.loop = loop
}

// LoadModule assigns a new XM module to this stream.
//
// Loading a module involves its compilation which is a slow process.
// You want to load modules as rarely as possible (preferably exactly once)
// and then play them via streams without ever releasing the memory.
func (s *Stream) LoadModule(m *xmfile.Module, config LoadModuleConfig) error {
	s.applyConfigDefaults(m, &config)

	if config.SampleRate != 44100 {
		return errors.New("unsupported sample rate (only 44100 is supported)")
	}

	if cap(s.channels) < m.NumChannels {
		s.channels = make([]streamChannel, m.NumChannels)
		s.activeChannels = make([]*streamChannel, m.NumChannels)
	}
	s.channels = s.channels[:m.NumChannels]
	s.activeChannels = s.activeChannels[:0]

	compiled, err := compileModule(m, moduleConfig{
		sampleRate: config.SampleRate,
		bpm:        config.BPM,
		tempo:      config.Tempo,
		subSamples: config.LinearInterpolation,
	})
	if err != nil {
		return err
	}
	s.module = compiled

	// Call a rewind() that won't trigger a Sync event.
	s.rewind()

	return nil
}

func (s *Stream) applyConfigDefaults(m *xmfile.Module, config *LoadModuleConfig) {
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
}

// Seek partially implements io.Seeker.
//
// You can use it for two things:
//  1. (0, SeekStart) for rewind
//  2. (0, SeekCurrent) to get the byte pos inside the stream
func (s *Stream) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		if offset == 0 {
			s.Rewind()
			return 0, nil
		}

	case io.SeekCurrent:
		if offset == 0 {
			return int64(s.bytePos), nil
		}
	}

	return 0, errors.New("unsupported Seek call")
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
	eof := false

	for len(b) > s.bytesPerTick {
		if !s.nextTick() {
			eof = true
			break
		}
		s.readTick(b[:s.bytesPerTick])

		written += s.bytesPerTick
		b = b[s.bytesPerTick:]
	}

	s.bytePos += written

	if eof {
		if s.settings.loop {
			s.Rewind()
			return written, nil
		}
		return written, io.EOF
	}
	return written, nil
}

// Rewind prepares the stream to play the module right from the start.
// Doing rewind is relatively cheap.
func (s *Stream) Rewind() {
	if s.settings.eventHandler != nil {
		s.settings.eventHandler(StreamEvent{
			Kind:  EventSync,
			Time:  s.t,
			value: math.Float64bits(0),
		})
	}
	s.rewind()
}

func (s *Stream) rewind() {
	// Make all fields zero-initialized just to be safe.
	// Copying the module object is redundant, but oh well (it's a shallow copy anyway).
	*s = Stream{
		module:         s.module,
		channels:       s.channels,
		activeChannels: s.activeChannels,
		settings:       s.settings,
	}

	// Now initialize the player to the "ready to start" state.
	// This code is used as a final part of the constructor as well.

	for i := range s.channels {
		ch := &s.channels[i]
		ch.Reset()
		ch.id = i
	}

	s.globalVolume = 1.0
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
	s.secondsPerRow = calcSecondsPerRow(s.module.ticksPerRow, s.bpm)
}

// GetInfo returns stream-related info.
// See StreamInfo for more details.
func (s *Stream) GetInfo() StreamInfo {
	return StreamInfo{
		BytesPerTick: uint(s.bytesPerTick),
		MemoryUsage:  moduleSize(&s.module),
	}
}

func (s *Stream) nextTick() bool {
	if s.rowTicksRemain == 0 {
		if !s.nextRow() {
			return false
		}
	}

	s.rowTicksRemain--
	s.tickIndex++

	s.activeChannels = s.activeChannels[:0]
	baseVolume := s.settings.volumeScaling * s.globalVolume
	for j := range s.channels {
		ch := &s.channels[j]
		note := ch.note

		s.tickEnvelopes(ch)

		panning := ch.panning + (ch.panningEnvelope.value-0.5)*(0.5-abs(ch.panning-0.5))*2

		// 0.25 is an amplification heuristic to avoid clipping.
		volume := 0.25 * baseVolume * ch.volume * ch.fadeoutVolume * ch.volumeEnvelope.value
		ch.targetVolume[0] = volume * math.Sqrt(1.0-panning)
		ch.targetVolume[1] = volume * math.Sqrt(panning)

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
		if ch.inst != nil {
			ch.sampleStep *= ch.inst.sampleStepMultiplier
		}

		if ch.IsActive() {
			s.activeChannels = append(s.activeChannels, ch)
		}
	}

	return true
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

func (s *Stream) nextRow() bool {
	if s.jumpKind == jumpNone {
		// Normal execution.
		if s.patternRowsRemain == 0 {
			if !s.nextPattern() {
				return false
			}
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
	m := &s.module

	for i := range s.channels {
		s.advanceChannelRow(&s.channels[i], &m.noteTab[notes[i]])
	}

	s.t += s.secondsPerRow
	s.rowTicksRemain = s.ticksPerRow
	s.tickIndex = -1
	return true
}

func (s *Stream) advanceChannelRow(ch *streamChannel, n *patternNote) {
	ch.assignNote(n)

	if !ch.effect.IsEmpty() {
		s.applyRowEffect(ch, n)
	}

	if s.settings.eventHandler != nil && n.raw != 0 {
		instID := 255 // It's a sentinel value that fits 8 bits
		if ch.inst != nil {
			instID = ch.inst.id
		}
		value := uint64(n.raw) | uint64(instID<<8) | (uint64(math.Float32bits(float32(ch.volume))) << 16)
		s.settings.eventHandler(StreamEvent{
			Kind:    EventNote,
			Channel: ch.id,
			Time:    s.t,
			value:   value,
		})
	}
}

func (s *Stream) applyRowEffect(ch *streamChannel, n *patternNote) {
	numEffects := ch.effect.Len()
	offset := ch.effect.Index()
	for _, e := range s.module.effectTab[offset : offset+numEffects] {
		switch e.op {
		case xmdb.EffectSetVolume:
			ch.volume = e.floatValue

		case xmdb.EffectEarlyKeyOff:
			s.keyOff(ch)

		case xmdb.EffectVolumeSlide, xmdb.EffectVibratoWithVolumeSlide:
			if e.floatValue != 0 {
				ch.volumeSlideValue = e.floatValue
			}

		case xmdb.EffectGlobalVolumeSlide:
			if e.floatValue != 0 {
				ch.globalVolumeSlideValue = e.floatValue
			}

		case xmdb.EffectPanningSlide:
			if e.floatValue != 0 {
				ch.panningSlideValue = e.floatValue
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
			// Note: notePortamentoValue was guarded by e.floatValue>0 condition
			// before, but it looks incorrect?
			ch.notePortamentoValue = e.floatValue
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

		case xmdb.EffectFineVolumeSlideDown:
			ch.volume = clampMin(ch.volume-e.floatValue, 0)
		case xmdb.EffectFineVolumeSlideUp:
			ch.volume = clampMax(ch.volume+e.floatValue, 1)

		case xmdb.EffectSetGlobalVolume:
			s.globalVolume = e.floatValue

		case xmdb.EffectSetPanning:
			ch.panning = e.floatValue

		case xmdb.EffectSampleOffset:
			if ch.inst == nil {
				break
			}
			// TODO: can we precalculate this period in the compiler, somehow?
			// I'm afraid of the current instrument dependency (which can be
			// inferred by the compiler, but it won't work in case of a
			// pattern jump, etc.)
			// Since this is not a hot path, let's compute the offset the hard way.
			offset := 0.0
			if ch.inst.sample16bit {
				offset = e.floatValue * 0.5
			} else {
				offset = e.floatValue
			}
			if ch.inst.numSubSamples != 0 {
				offset = float64(int(offset) * (ch.inst.numSubSamples + 1))
			}
			ch.sampleOffset = offset
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

		case xmdb.EffectSetEnvelopePos:
			ch.volumeEnvelope.frame = int(e.rawValue)

		case xmdb.EffectNoteCut:
			if e.arp[0] != uint8(s.tickIndex) {
				break
			}
			ch.volume = 0

		case xmdb.EffectArpeggio:
			i := s.tickIndex % 3
			ch.arpeggioNoteOffset = float64(e.arp[i])
			ch.arpeggioRunning = i != 0

		case xmdb.EffectVolumeSlide:
			if s.tickIndex == 0 {
				break
			}
			ch.volume = clamp(ch.volume+ch.volumeSlideValue, 0, 1)

		case xmdb.EffectGlobalVolumeSlide:
			if s.tickIndex == 0 {
				break
			}
			s.globalVolume = clamp(s.globalVolume+ch.globalVolumeSlideValue, 0, 1)

		case xmdb.EffectPanningSlide:
			if s.tickIndex == 0 {
				break
			}
			ch.panning = clamp(ch.panning+ch.panningSlideValue, 0, 1)

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

		case xmdb.EffectPanningSlideLeft:
			ch.panning = clampMin(ch.panning-e.floatValue, 0)
		case xmdb.EffectPanningSlideRight:
			ch.panning = clampMax(ch.panning+e.floatValue, 1)
		}
	}
}

func (s *Stream) nextPattern() bool {
	i := s.patternIndex + 1
	if i >= len(s.module.patternOrder) {
		return false
	}
	s.selectPattern(i)
	return true
}

func (s *Stream) selectPattern(i int) {
	s.patternIndex = i
	s.pattern = s.module.patternOrder[s.patternIndex]

	s.patternRowIndex = -1
	s.patternRowsRemain = s.pattern.numRows
}

func (s *Stream) readTick(b []byte) {
	// This function dominates the music rendering execution time.
	// It's important to keep it very efficient.
	// The slightest change inside this nested loop can result in ~10% playback
	// performance regression.

	n := len(b)

	const (
		rampBytes  = 2 * 2 * numRampPoints
		volumeRamp = 1.0 / 180.0
	)

	for i := 0; i < rampBytes; i += 4 {
		left := int16(0)
		right := int16(0)

		for _, ch := range s.activeChannels {
			v := float64(ch.NextSample())
			if ch.rampFrame < uint(len(ch.rampSamples)) {
				v = lerp(ch.rampSamples[ch.rampFrame], v, float64(ch.rampFrame)/float64(len(ch.rampSamples)))
			}
			left += int16(v * ch.computedVolume[0])
			right += int16(v * ch.computedVolume[1])
			ch.rampFrame++
			ch.computedVolume[0] = slideTowards(ch.computedVolume[0], ch.targetVolume[0], volumeRamp)
			ch.computedVolume[1] = slideTowards(ch.computedVolume[1], ch.targetVolume[1], volumeRamp)
		}

		putPCM(b[i:], uint16(left), uint16(right))
	}

	for i := rampBytes; i < n; i += 4 {
		left := int16(0)
		right := int16(0)

		for _, ch := range s.activeChannels {
			v := float64(ch.NextSample())
			left += int16(v * ch.computedVolume[0])
			right += int16(v * ch.computedVolume[1])
		}

		putPCM(b[i:], uint16(left), uint16(right))
	}
}
