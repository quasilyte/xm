package xm

import (
	"github.com/quasilyte/xm/xmfile"
)

// Synthesizer can be used to play individual XM notes.
//
// It is more efficient and convenient to use for this
// use case than a stream with a constant module re-loading.
//
// Experimental: synthesizer API may change in the near future.
type Synthesizer struct {
	stream       *Stream
	noteCompiler *moduleCompiler
}

type SynthesizerConfig struct {
	NumChannels int
}

func NewSynthesizer(config SynthesizerConfig) *Synthesizer {
	stream := NewStream()
	stream.channels = make([]streamChannel, config.NumChannels)
	stream.activeChannels = make([]*streamChannel, config.NumChannels)
	return &Synthesizer{
		stream:       stream,
		noteCompiler: newSynthCompiler(),
	}
}

// SetVolume adjusts the global volume scaling for the underlying stream.
func (s *Synthesizer) SetVolume(v float64) {
	s.stream.SetVolume(v)
}

// LoadInstruments prepares the instruments from the module
// for further use.
//
// Loading instruments involves module compilation,
// so it should not be called on a hot path repeatedly.
// If you need multiple modules at once, consider
// using several synthesizers, one per module.
//
// The patterns don't really matter as this method
// is only interested in instruments (and samples).
func (s *Synthesizer) LoadInstruments(m *xmfile.Module, config LoadModuleConfig) error {
	s.stream.applyConfigDefaults(m, &config)

	s.stream.activeChannels = s.stream.activeChannels[:0]

	instOnly := xmfile.Module{
		Name:           m.Name,
		Version:        m.Version,
		NumInstruments: m.NumInstruments,
		Flags:          m.Flags,
		Instruments:    m.Instruments,
	}

	compiled, err := compileModule(&instOnly, moduleConfig{
		sampleRate: config.SampleRate,
		bpm:        config.BPM,
		tempo:      config.Tempo,
		subSamples: config.LinearInterpolation,
	})
	if err != nil {
		return err
	}

	s.stream.assignCompiledModule(compiled)

	s.stream.module.patterns = []pattern{
		{
			numChannels: len(s.stream.channels),
			numRows:     1,
			notes:       make([]uint16, len(s.stream.channels)),
		},
	}
	s.stream.module.patternOrder = []*pattern{
		&s.stream.module.patterns[0],
	}

	pat := &s.stream.module.patterns[0]
	for i := range pat.notes {
		pat.notes[i] = uint16(i)
	}

	return nil
}

func (s *Synthesizer) prepareToPlay(duration float64) {
	s.noteCompiler.reset(&s.stream.module)

	s.stream.module.effectTab = s.stream.module.effectTab[:0]
	s.stream.module.noteTab = s.stream.module.noteTab[:0]

	if duration == 0 {
		s.stream.setTempo(240)
	} else {
		s.stream.setTempo(1 + int(ticksPerSecond(s.stream.bpm)*duration))
	}
}

// PlayNote plays one or more notes up to the specified duration.
// Using 0 for the duration will play it for several seconds.
func (s *Synthesizer) PlayNote(duration float64, notes ...xmfile.PatternNote) error {
	s.prepareToPlay(duration)

	chansUsed := 0
	for _, note := range notes {
		n, _, err := s.noteCompiler.compileNote(note)
		if err != nil {
			return err
		}
		s.stream.module.noteTab = append(s.stream.module.noteTab, n)
		chansUsed++
		if chansUsed >= len(s.stream.channels) {
			break
		}
	}

	for chansUsed < len(s.stream.channels) {
		s.stream.module.noteTab = append(s.stream.module.noteTab, patternNote{})
		chansUsed++
	}

	return nil
}

func (s *Synthesizer) Read(b []byte) (int, error) {
	return s.stream.Read(b)
}

func (s *Synthesizer) Rewind() {
	s.stream.Rewind()
}

func (s *Synthesizer) Seek(offset int64, whence int) (int64, error) {
	return s.stream.Seek(offset, whence)
}
