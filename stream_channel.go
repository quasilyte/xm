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

func (ch *streamChannel) assignNote(n *patternNote) {
	// Some sensible row note states:
	//
	//	[note] [instrument]
	//	no     no           keep playing the current note (if any)
	//	no     yes          "ghost instrument" (keeps the sample offset)
	//	yes    no           "ghost note" (keeps the volume)
	//	yes    yes          normal note play
	//
	// In practice, it's more complicated due to various effects
	// that may affect the logical consistency.

	ch.note = n
	ch.effect = n.effect
	ch.vibratoPeriodOffset = 0

	if n.inst == nil && !n.flags.Contains(noteValid) {
		// An empty note: do nothing.
		return
	}

	inst := n.inst

	hasNotePortamento := n.flags.Contains(noteHasNotePortamento)
	if hasNotePortamento && inst == nil {
		inst = ch.inst
	}

	if n.period != 0 {
		ch.keyOn = true
		ch.resetEnvelopes()
	}

	if inst == nil {
		if n.period != 0 {
			ch.period = n.period
		}
	} else {
		// Start playing next note.
		if n.period != 0 && !hasNotePortamento {
			ch.sampleOffset = 0
			ch.reverse = false
			ch.period = n.period
		}
		ch.volumeEnvelope.envelope = inst.volumeEnvelope
		ch.panningEnvelope.envelope = inst.panningEnvelope
		ch.volume = inst.volume
		ch.inst = inst
		ch.panning = inst.panning
	}
}

type envelopeRunner struct {
	envelope

	value float64
	frame int
}
