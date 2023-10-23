package xm

type streamChannel struct {
	sampleOffset   float64
	note           *patternNote
	volume         float64
	computedVolume [2]float64

	// Ping-pong loop state.
	reverse bool
}
