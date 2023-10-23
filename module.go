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
	inst   *instrument
	period float64

	effect effectKey // Can be empty, see effectKey.IsEmpty()
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

	volume float64

	volumeFlags  xmfile.EnvelopeFlags
	panningFlags xmfile.EnvelopeFlags

	volumeFadeoutStep float64

	loopType   xmfile.SampleLoopType
	loopLength float64
	loopStart  float64
	loopEnd    float64
}

type effectKey uint16

func makeEffectKey(index, length uint) effectKey {
	return effectKey((uint16(index) << 2) | (uint16(length) & 0b11))
}

func (k effectKey) IsEmpty() bool { return k == 0 }

func (k effectKey) Len() uint { return uint(k & 0b11) }

func (k effectKey) Index() uint { return uint(k >> 2) }
