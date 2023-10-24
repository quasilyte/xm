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

	// Encoding: effect=0x0A
	// Arg: slide up/down speed
	EffectVolumeSlide

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

	case v <= 0x50:
		// Set volume effect.
		e.Op = EffectSetVolume
		e.Arg = v - 0x10
	}

	return e
}

func (e Effect) AsUint16() uint16 {
	return (uint16(e.Op) << 8) | uint16(e.Arg)
}
