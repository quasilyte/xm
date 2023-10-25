package xm

import (
	"math"
)

type numeric interface {
	uint8 | int | float64
}

func clampMin[T numeric](v, min T) T {
	if v < min {
		return min
	}
	return v
}

func clampMax[T numeric](v, max T) T {
	if v > max {
		return max
	}
	return v
}

func clamp[T numeric](v, min, max T) T {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func calcSamplesPerTick(sampleRate, bpm float64) (samplesPerTick float64, bytesPerTick int) {
	samplesPerTick = math.Round(sampleRate / (bpm * 0.4))
	const (
		channels       = 2
		bytesPerSample = 2
	)
	bytesPerTick = int(samplesPerTick) * channels * bytesPerSample
	return samplesPerTick, bytesPerTick
}

func calcRealNote(note uint8, inst *instrument) float64 {
	fnote := float64(note)
	var frelativeNote float64
	var ffinetune float64
	if inst != nil {
		frelativeNote = float64(inst.relativeNote)
		ffinetune = float64(inst.finetune)
	}
	return (fnote + frelativeNote + ffinetune/128) - 1
}

func linearPeriod(note float64) float64 {
	return 7680.0 - note*64.0
}

func linearFrequency(period float64) float64 {
	return 8363.0 * math.Pow(2, (4608-period)/768)
}

func envelopeLerp(a, b envelopePoint, frame int) float64 {
	if frame <= a.frame {
		return a.value
	}
	if frame >= b.frame {
		return b.value
	}
	p := float64(frame-a.frame) / float64(b.frame-a.frame)
	return a.value*(1-p) + b.value*p
}
