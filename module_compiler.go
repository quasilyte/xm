package xm

import (
	"encoding/binary"
	"errors"
	"math"

	"github.com/quasilyte/xm/xmfile"
)

type moduleCompiler struct {
	result module
}

func compileModule(m *xmfile.Module, config moduleConfig) (module, error) {
	c := &moduleCompiler{}
	c.result = module{
		sampleRate:  float64(config.sampleRate),
		bpm:         float64(config.bpm),
		ticksPerRow: float64(config.tempo),
	}
	err := c.compile(m)
	return c.result, err
}

func (c *moduleCompiler) compile(m *xmfile.Module) error {
	if (m.Flags & (0b1)) != 1 {
		return errors.New("the Amiga frequency table is not supported yet")
	}

	c.result.samplesPerTick = math.Round(c.result.sampleRate / (c.result.bpm * 0.4))
	{
		const (
			channels       = 2
			bytesPerSample = 2
		)
		c.result.bytesPerTick = int(c.result.samplesPerTick) * channels * bytesPerSample
	}

	if err := c.compileInstruments(m); err != nil {
		return err
	}

	if err := c.compilePatterns(m); err != nil {
		return err
	}

	return nil
}

func (c *moduleCompiler) compileInstruments(m *xmfile.Module) error {
	c.result.instruments = make([]instrument, m.NumInstruments)
	for i, inst := range m.Instruments {
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

		c.result.instruments[i] = dstInst
	}

	return nil
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
		for _, row := range rawPat.Rows {
			for _, rawNote := range row.Notes {
				var n patternNote
				if rawNote.Instrument != 0 {
					inst := &c.result.instruments[rawNote.Instrument-1]
					realNote := calcRealNote(rawNote.Note, inst)
					period := linearPeriod(realNote)
					freq := linearFrequency(period)
					// var effect0 noteEffect
					// if rawNote.Note == 97 {
					// 	effect0.op = effectKeyOff
					// }
					n = patternNote{
						freq:       freq,
						inst:       inst,
						sampleStep: freq / c.result.sampleRate,
						effect1:    volumeByteToEffect(rawNote.Volume),
					}
				}
				pat.notes = append(pat.notes, n)
			}
		}
	}

	return nil
}
