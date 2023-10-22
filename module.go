package xm

import (
	"errors"
	"math"

	"github.com/quasilyte/xm/xmfile"
)

type module struct {
	instruments []instrument

	patterns     []pattern
	patternOrder []*pattern

	sampleRate     float64
	bpm            float64
	ticksPerRow    float64
	samplesPerTick float64
	bytesPerTick   int
}

type moduleConfig struct {
	sampleRate uint
	bpm        uint
	tempo      uint
}

type pattern struct {
	numChannels int
	numRows     int
	notes       []patternNote
}

type patternNote struct {
	inst       *instrument
	freq       float64
	sampleStep float64
}

type instrument struct {
	samples  []int16
	finetune int8
}

func (m *module) Assign(raw *xmfile.Module, config moduleConfig) error {
	*m = module{
		sampleRate:  float64(config.sampleRate),
		bpm:         float64(config.bpm),
		ticksPerRow: float64(config.tempo),
	}

	m.samplesPerTick = math.Round(m.sampleRate / (m.bpm * 0.4))
	{
		const (
			channels       = 2
			bytesPerSample = 2
		)
		m.bytesPerTick = int(m.samplesPerTick) * channels * bytesPerSample
	}

	if err := m.assignInstruments(raw); err != nil {
		return err
	}

	m.assignPatterns(raw)

	return nil
}

func (m *module) assignInstruments(raw *xmfile.Module) error {
	m.instruments = make([]instrument, raw.NumInstruments)
	for i, inst := range raw.Instruments {
		if len(inst.Samples) == 0 {
			continue
		}
		if len(inst.Samples) != 1 {
			return errors.New("multi-sample instruments are not supported yet")
		}

		// Convert 8-bit samples into signed 16-bit samples.
		// Also note that sample.Data stores deltas while
		// dstInst will store the absolute values.
		sample := inst.Samples[0]
		dstSamples := make([]int16, len(sample.Data))
		v := uint8(0)
		for i, delta := range sample.Data {
			v = uint8(int(v) + int(int8(delta)))
			dstSamples[i] = int16((int(v) << 8))
		}

		dstInst := instrument{
			samples:  dstSamples,
			finetune: int8(sample.Finetune),
		}
		m.instruments[i] = dstInst
	}

	return nil
}

func (m *module) assignPatterns(raw *xmfile.Module) {
	m.patterns = make([]pattern, raw.NumPatterns)
	m.patternOrder = make([]*pattern, len(raw.PatternOrder))

	// Bind pattern order to the actual patterns.
	for i, patternIndex := range raw.PatternOrder {
		m.patternOrder[i] = &m.patterns[patternIndex]
	}

	for i := range raw.Patterns {
		rawPat := &raw.Patterns[i]
		pat := &m.patterns[i]
		pat.numChannels = raw.NumChannels
		pat.numRows = len(rawPat.Rows)
		pat.notes = make([]patternNote, 0, len(rawPat.Rows)*raw.NumChannels)
		for _, row := range rawPat.Rows {
			for _, rawNote := range row.Notes {
				var n patternNote
				if rawNote.Instrument != 0 {
					inst := &m.instruments[rawNote.Instrument-1]
					period := linearPeriod(float64(rawNote.Note - 1))
					freq := linearFrequency(period)
					n = patternNote{
						freq:       freq,
						inst:       inst,
						sampleStep: freq / m.sampleRate,
					}
				}
				pat.notes = append(pat.notes, n)
			}
		}
	}
}
