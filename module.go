package xm

import (
	"github.com/quasilyte/xm/internal/xmdb"
	"github.com/quasilyte/xm/xmfile"
)

type module struct {
	instruments []instrument

	patterns     []pattern
	patternOrder []*pattern

	effectTab []noteEffect
	noteTab   []patternNote

	sampleRate  float64
	bpm         float64
	ticksPerRow int

	// These values store the defaults for the stream.
	samplesPerTick float64
	bytesPerTick   int
	secondsPerRow  float64
}

type moduleConfig struct {
	sampleRate uint
	bpm        uint
	tempo      uint
	subSamples bool
}

type pattern struct {
	numChannels int
	numRows     int
	notes       []uint16
}

type patternNote struct {
	inst   *instrument
	period float64
	raw    float64
	flags  patternNoteFlags

	effect effectKey // Can be empty, see effectKey.IsEmpty()
}

func (n *patternNote) Kind() patternNoteKind {
	return patternNoteKind(n.flags >> (64 - 2))
}

type patternNoteKind int

const (
	noteEmpty patternNoteKind = iota
	noteGhostInstrument
	noteGhost
	noteNormal
)

type patternNoteFlags uint64

const (
	noteHasNotePortamento = 1 << iota
	noteHasArpeggio
	noteHasVibrato
	noteValid
	noteBadInstrument
	noteInitialized
)

func (f patternNoteFlags) Contains(v patternNoteFlags) bool {
	return f&v != 0
}

type noteEffect struct {
	op         xmdb.EffectOp
	rawValue   uint8
	arp        [3]uint8
	floatValue float64
}

type instrument struct {
	samples      []int16
	finetune     int8
	relativeNote int8

	volume  float64
	panning float64

	sampleStepMultiplier float64

	volumeEnvelope  envelope
	panningEnvelope envelope

	volumeFadeoutStep float64

	loopType   xmfile.SampleLoopType
	loopLength float64
	loopStart  float64
	loopEnd    float64

	numSubSamples int
	id            int

	sample16bit bool
}

type envelope struct {
	flags          xmfile.EnvelopeFlags
	sustainPoint   uint8
	loopEndPoint   uint8
	loopStartPoint uint8

	sustainFrame int
	loopEndFrame int
	loopLength   int

	points []envelopePoint
}

type envelopePoint struct {
	frame int
	value float64
}

type effectKey uint16

func makeEffectKey(index, length uint) effectKey {
	return effectKey((uint16(index) << 2) | (uint16(length) & 0b11))
}

func (k effectKey) IsEmpty() bool { return k == 0 }

func (k effectKey) Len() uint { return uint(k & 0b11) }

func (k effectKey) Index() uint { return uint(k >> 2) }
