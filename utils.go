package xm

import (
	"math"
)

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func linearPeriod(note float64) float64 {
	return 7680.0 - note*64.0
}

func linearFrequency(period float64) float64 {
	return 8363.0 * math.Pow(2, (4608-period)/768)
}
