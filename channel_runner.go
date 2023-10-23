package xm

type streamChannel struct {
	// Note-related data.
	inst       *instrument
	period     float64
	sampleStep float64
	effect     effectKey
	sustain    bool

	sampleOffset   float64
	volume         float64
	fadeoutVolume  float64
	computedVolume [2]float64

	// Arpeggio effect state.
	arpeggioTicked     bool
	arpeggioRunning    bool
	arpeggioNoteOffset float64

	// Ping-pong loop state.
	reverse bool
}
