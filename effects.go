package xm

type effectOp uint8

const (
	effectNone effectOp = iota

	// Encoding: effect=0x0C or volume byte
	// Arg: volume level
	effectSetVolume
)
