package xm

type streamChannel struct {
	// Note-related data.
	inst       *instrument
	note       *patternNote
	period     float64
	sampleStep float64
	effect     effectKey
	keyOn      bool

	panning float64

	sampleOffset   float64
	volume         float64
	fadeoutVolume  float64
	computedVolume [2]float64

	// Arpeggio effect state.
	arpeggioRunning    bool
	arpeggioNoteOffset float64

	volumeSlideValue    float64
	portamentoUpValue   float64
	portamentoDownValue float64

	notePortamentoTargetPeriod float64
	notePortamentoValue        float64

	// Vibrato effect state.
	vibratoRunning      bool
	vibratoPeriodOffset float64
	vibratoDepth        float64
	vibratoStep         uint8
	vibratoSpeed        uint8

	// Ping-pong loop state.
	reverse bool

	volumeEnvelope  envelopeRunner
	panningEnvelope envelopeRunner
}

type envelopeRunner struct {
	envelope

	value float64
	frame int
}
