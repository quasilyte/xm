package xm

type effectOp uint8

const (
	effectNone effectOp = iota

	// Encoding: effect=0x0C or volume byte
	// Arg: volume level
	effectSetVolume

	// Encoding: effect=0x14 or key-off note
	// Arg: tick number (always a first tick for key-off note)
	effectKeyOff
)
