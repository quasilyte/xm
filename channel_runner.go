package xm

type streamChannel struct {
	sampleOffset   float64
	note           *patternNote
	computedVolume [2]float64
}
