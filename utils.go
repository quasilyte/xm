package xm

import (
	"math"
)

func clampMin(v, min float64) float64 {
	if v < min {
		return min
	}
	return v
}

func clampMax(v, max float64) float64 {
	if v > max {
		return max
	}
	return v
}

func clamp(v, min, max float64) float64 {
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
