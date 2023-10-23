package xm

type streamChannel struct {
	note *patternNote

	sampleOffset   float64
	volume         float64
	computedVolume [2]float64

	// Ping-pong loop state.
	reverse bool
}
