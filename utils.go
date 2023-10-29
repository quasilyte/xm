package xm

import (
	"math"
)

type numeric interface {
	uint8 | int | float64
}

func slideTowards[T numeric](v, goal, delta T) T {
	if v > goal {
		return clampMin(v-delta, goal)
	}
	if v < goal {
		return clampMax(v+delta, goal)
	}
	return v
}

func lerp(u, v, t float64) float64 {
	return u + t*(v-u)
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

func waveform(step uint8) float64 {
	return -math.Sin(2 * 3.141592 * float64(step) / 0x40)
}

func calcRealNote(fnote float64, inst *instrument) float64 {
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

func putPCM(buf []byte, left, right uint16) {
	_ = buf[3] // Early bound check
	buf[0] = byte(left)
	buf[1] = byte(left >> 8)
	buf[2] = byte(right)
	buf[3] = byte(right >> 8)
}
