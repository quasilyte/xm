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

	// This ID is needed mostly for debugging,
	// therefore we put it to the object's tail.
	id int
}

func (ch *streamChannel) Reset() {
	*ch = streamChannel{}
}

func (ch *streamChannel) resetEnvelopes() {
	ch.fadeoutVolume = 1
	ch.volumeEnvelope.value = 1
	ch.volumeEnvelope.frame = 0
	ch.panningEnvelope.value = 0.5
	ch.panningEnvelope.frame = 0
}

type envelopeRunner struct {
	envelope

	value float64
	frame int
}
