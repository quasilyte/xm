package xm

import (
	"github.com/quasilyte/xm/xmfile"
)

type module struct {
	instruments []instrument

	patterns     []pattern
	patternOrder []*pattern

	sampleRate     float64
	bpm            float64
	ticksPerRow    float64
	samplesPerTick float64
	bytesPerTick   int
}

type moduleConfig struct {
	sampleRate uint
	bpm        uint
	tempo      uint
}

type pattern struct {
	numChannels int
	numRows     int
	notes       []patternNote
}

type patternNote struct {
	inst       *instrument
	freq       float64
	sampleStep float64

	effect1 noteEffect // Volume byte converted to the standard effect
	effect2 noteEffect
}

type noteEffect struct {
	op  effectOp
	arg uint8
}

type instrument struct {
	samples      []int16
	finetune     int8
	relativeNote int8

	volume float64

	loopType   xmfile.SampleLoopType
	loopLength float64
	loopStart  float64
	loopEnd    float64
}
