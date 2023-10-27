package xmdb

import (
	"fmt"

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

	// Encoding: effect=0x01
	// Arg: portamento speed
	EffectPortamentoUp

	// Encoding: effect=0x02
	// Arg: portamento speed
	EffectPortamentoDown

	// Encoding: effect=0x03
	// Arg: portamento speed
	// Note: also known as portamento-to-none and tone portamento
	EffectNotePortamento

	// Encoding: effect=0x04
	// Arg: speed & depth
	EffectVibrato

	// Encoding: effect=0x06
	// Arg: same as in EffectVolumeSlide
	EffectVibratoWithVolumeSlide

	// Encoding: effect=0x0C [or] volume byte
	// Arg: volume level
	EffectSetVolume

	// Encoding: part of the volume byte
	EffectVolumeSlideDown
	EffectVolumeSlideUp

	// Encoding: part of the volume byte
	EffectFineVolumeSlideDown
	EffectFineVolumeSlideUp

	// Encoding: part of the volume byte
	EffectPanningSlideLeft
	EffectPanningSlideRight

	// Encoding: effect=0x0F with arg>0x1F
	// Arg: new BPM value
	EffectSetBPM

	// Encoding: effect=0x0F with arg<=0x1F
	// Arg: new tempo (spd) value
	EffectSetTempo

	// Encoding: effect=0x0A
	// Arg: slide up/down speed
	EffectVolumeSlide

	// Encoding: effect=0x0D
	// Arg: target row number (on the next pattern)
	EffectPatternBreak

	// Encoding: key-off note (97)
	// Note: it's always the first effect in the list
	EffectEarlyKeyOff

	// Encoding: effect=0x14
	// Arg: tick number
	EffectKeyOff

	// Encoding: effect=0x0E and x=C
	// Arg: tick number
	EffectNoteCut

	// Encoding: effect=0x19
	// Arg: slide left/right speed
	EffectPanningSlide

	// Encoding: effect=0x08 [or] volume byte
	// Arg: panning position
	EffectSetPanning

	// Encoding effect=0x09
	// Arg: offset
	EffectSampleOffset
)

func ConvertEffect(n xmfile.PatternNote) Effect {
	e := Effect{Arg: n.EffectParameter}

	switch n.EffectType {
	case 0x00:
		if n.EffectParameter != 0 {
			e.Op = EffectArpeggio
		}

	case 0x01:
		e.Op = EffectPortamentoUp

	case 0x02:
		e.Op = EffectPortamentoDown

	case 0x03:
		e.Op = EffectNotePortamento

	case 0x04:
		e.Op = EffectVibrato

	case 0x06:
		e.Op = EffectVibratoWithVolumeSlide

	case 0x08:
		e.Op = EffectSetPanning

	case 0x09:
		e.Op = EffectSampleOffset

	case 0x0A:
		e.Op = EffectVolumeSlide

	case 0x0C:
		e.Op = EffectSetVolume

	case 0x0D:
		e.Op = EffectPatternBreak

	case 0x0E:
		switch e.Arg >> 4 {
		case 0x0C:
			e.Op = EffectNoteCut
		}

	case 0x0F:
		if n.EffectParameter == 0 {
			break
		}
		if n.EffectParameter > 0x1F {
			e.Op = EffectSetBPM
		} else {
			e.Op = EffectSetTempo
		}

	case 0x19:
		e.Op = EffectPanningSlide

	case 0x14:
		e.Op = EffectKeyOff

	default:
		fmt.Printf("unsupported effect: %02X\n", n.EffectType)
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

	case v >= 0x60 && v <= 0x6F:
		e.Op = EffectVolumeSlideDown
		e.Arg = v & 0x0F

	case v >= 0x70 && v <= 0x7F:
		e.Op = EffectVolumeSlideUp
		e.Arg = v & 0x0F

	case v >= 0x80 && v <= 0x8F:
		e.Op = EffectFineVolumeSlideDown
		e.Arg = v & 0x0F

	case v >= 0x90 && v <= 0x9F:
		e.Op = EffectFineVolumeSlideUp
		e.Arg = v & 0x0F

	case v >= 0xC0 && v <= 0xCF:
		argBits := v & 0x0F
		e.Op = EffectSetPanning
		e.Arg = (argBits << 4) | argBits

	case v >= 0xD0 && v <= 0xDF:
		e.Op = EffectPanningSlideLeft
		e.Arg = v & 0x0F

	case v >= 0xE0 && v <= 0xEF:
		e.Op = EffectPanningSlideRight
		e.Arg = v & 0x0F

	default:
		fmt.Printf("unhandled volume column: %02X\n", v)
	}

	return e
}

func (e Effect) AsUint16() uint16 {
	return (uint16(e.Op) << 8) | uint16(e.Arg)
}
