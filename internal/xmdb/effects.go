package xmdb

import (
	"github.com/quasilyte/xm/xmfile"
)

type Effect struct {
	Op  EffectOp
	Arg uint8
}

type EffectOp int

const (
	EffectNone EffectOp = iota

	// Encoding: effect=0x00
	// Arg: semitone offsets
	EffectArpeggio

	// Encoding: effect=0x0C [or] volume byte
	// Arg: volume level
	EffectSetVolume

	// Encoding: part of the volume byte
	EffectVolumeSlideDown
	EffectVolumeSlideUp

	// Encoding: effect=0x0A
	// Arg: slide up/down speed
	EffectVolumeSlide

	// Encoding: effect=0x0D
	// Arg: target row number (on the next pattern)
	EffectPatternBreak

	// Encoding: effect=0x14 [or] key-off note
	// Arg: tick number (always a first tick for key-off note)
	EffectKeyOff
)

func ConvertEffect(n xmfile.PatternNote) Effect {
	e := Effect{Arg: n.EffectParameter}

	switch n.EffectType {
	case 0x00:
		if n.EffectParameter != 0 {
			e.Op = EffectArpeggio
		}

	case 0x0A:
		e.Op = EffectVolumeSlide

	case 0x0D:
		e.Op = EffectPatternBreak

	case 0x14:
		e.Op = EffectKeyOff
	}

	return e
}

func EffectFromVolumeByte(v uint8) Effect {
	var e Effect

	switch {
	case v <= 0x0F:
		// Do nothing.

	case v >= 0x10 && v <= 0x50:
		e.Op = EffectSetVolume
		e.Arg = v - 0x10

	case v >= 0x60 && v <= 0x6f:
		e.Op = EffectVolumeSlideDown
		e.Arg = v & 0x0F
	case v >= 0x70 && v <= 0x7f:
		e.Op = EffectVolumeSlideUp
		e.Arg = v & 0x0F
	}

	return e
}

func (e Effect) AsUint16() uint16 {
	return (uint16(e.Op) << 8) | uint16(e.Arg)
}
