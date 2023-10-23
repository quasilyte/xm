package xm

import (
	"encoding/binary"
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

	effect1 noteEffect // Volume byte converted to the standard effect
	effect2 noteEffect
}

type noteEffect struct {
	op  effectOp
	arg uint8
}

type instrument struct {
	samples      []int16
	finetune     int8
	relativeNote int8

	volume float64

	loopType   xmfile.SampleLoopType
	loopLength float64
	loopStart  float64
	loopEnd    float64
}

func (m *module) Assign(raw *xmfile.Module, config moduleConfig) error {
	*m = module{
		sampleRate:  float64(config.sampleRate),
		bpm:         float64(config.bpm),
		ticksPerRow: float64(config.tempo),
	}

	if (raw.Flags & (0b1)) != 1 {
		return errors.New("the Amiga frequency table is not supported yet")
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

		numSamples := len(sample.Data)
		if sample.Is16bits() {
			numSamples /= 2
		}
		dstSamples := make([]int16, numSamples)

		loopEnd := sample.LoopStart + sample.LoopLength - 1
		loopStart := sample.LoopStart
		loopLength := sample.LoopLength
		if sample.LoopStart > sample.Length {
			loopStart = loopLength
		}
		if loopEnd > sample.Length {
			loopEnd = sample.Length - 1
		}
		loopLength = loopEnd - loopStart
		if sample.LoopType() == xmfile.SampleLoopForward {
			if loopStart > loopEnd {
				return errors.New("sample loopStart > loopEnd")
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
			v := int8(0)
			for i, delta := range sample.Data {
				v += int8(delta)
				dstSamples[i] = int16((int(v) << 8))
			}
		}

		dstInst := instrument{
			samples: dstSamples,

			finetune:     int8(sample.Finetune),
			relativeNote: int8(sample.RelativeNote),

			volume: float64(sample.Volume) / 0x40,

			loopType:   sample.LoopType(),
			loopLength: float64(loopLength),
			loopStart:  float64(loopStart),
			loopEnd:    float64(loopEnd),
		}

		switch dstInst.loopType {
		case xmfile.SampleLoopForward, xmfile.SampleLoopNone, xmfile.SampleLoopPingPong:
			// OK
		default:
			return errors.New("unknown sample loop type")
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
					realNote := calcRealNote(rawNote.Note, inst)
					period := linearPeriod(realNote)
					freq := linearFrequency(period)
					n = patternNote{
						freq:       freq,
						inst:       inst,
						sampleStep: freq / m.sampleRate,
						effect1:    volumeByteToEffect(rawNote.Volume),
					}
				}
				pat.notes = append(pat.notes, n)
			}
		}
	}
}
