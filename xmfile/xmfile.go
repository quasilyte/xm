package xmfile

import (
	"fmt"
	"io"
)

// ParserConfig customizes parser behavior.
type ParserConfig struct {
	// NeedStrings tells whether this parser needs to load optional strings
	// like instrument names. String loading usually means more allocations.
	NeedStrings bool
}

// Parser implements XM file decoding.
//
// It's optimized for multi-use: if you need to load more than one XM module,
// use the same parser to do so.
// See Parse method comments to learn more details.
type Parser struct {
	impl *parser
}

// NewParser creates a ready-to-use XM parser.
// The specified config will be used for all Parse calls.
func NewParser(config ParserConfig) *Parser {
	return &Parser{
		impl: newParser(config),
	}
}

// ParseFromBytes is like Parse, but it uses the byte slide directly.
func (p *Parser) ParseFromBytes(data []byte) (*Module, error) {
	err := p.impl.Parse(data)
	return &p.impl.module, err
}

// Parse decodes the XM module file.
//
// Note that calling Parse again invalidates previously returned module.
// This allows a better memory-reuse inside the parser for multi-use cases.
// If you want to keep more than one module object at time, perform deep cloning.
func (p *Parser) Parse(r io.Reader) (*Module, error) {
	// TODO: may want to re-use data buffer between the Parse calls.
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read data: %w", err)
	}
	return p.ParseFromBytes(data)
}

// Module is a parsed XM file contents.
// This is a raw module format that is not optimized for playback.
type Module struct {
	Name string

	TrackerName string

	// Major and minor version numbers.
	// Version[0] is a major version.
	// Version[1] is a minor version.
	Version [2]byte

	SongLength      int
	RestartPosition int

	NumChannels    int
	NumPatterns    int
	NumInstruments int

	// 0 - Amiga
	// 1 - Linear
	Flags uint16

	DefaultTempo int
	DefaultBPM   int

	PatternOrder []uint8

	Patterns []Pattern

	Notes []PatternNote

	// This pattern is generated only once and then used for every empty pattern in Patterns.
	EmptyPattern Pattern

	Instruments []Instrument
}

type Pattern struct {
	// Whether this pattern is an auto-generated empty pattern.
	IsEmpty bool

	Rows []PatternRow
}

type PatternRow struct {
	Notes []uint16
}

type PatternNote struct {
	ID uint16

	Note            uint8
	Instrument      uint8
	Volume          uint8
	EffectType      uint8
	EffectParameter uint8
}

type Instrument struct {
	Name string

	KeymapAssignments []byte
	EnvelopeVolume    []EnvelopePoint
	EnvelopePanning   []EnvelopePoint

	VolumeSustainPoint    uint8
	VolumeLoopStartPoint  uint8
	VolumeLoopEndPoint    uint8
	PanningSustainPoint   uint8
	PanningLoopStartPoint uint8
	PanningLoopEndPoint   uint8

	VolumeFlags  EnvelopeFlags
	PanningFlags EnvelopeFlags

	VibratoType  uint8
	VibratoSweep uint8
	VibratoDepth uint8
	VibratoRate  uint8

	VolumeFadeout int

	Samples []InstrumentSample
}

type EnvelopePoint struct {
	X uint16
	Y uint16
}

type InstrumentSample struct {
	Name         string
	Length       int
	LoopStart    int
	LoopLength   int
	Volume       int
	Finetune     int
	TypeFlags    uint8
	Panning      uint8
	RelativeNote int
	Format       SampleFormat
	Data         []uint8
}

type SampleLoopType int

const (
	SampleLoopNone SampleLoopType = iota
	SampleLoopForward
	SampleLoopPingPong
	SampleLoopUnknown
)

func (s *InstrumentSample) LoopType() SampleLoopType {
	bits := s.TypeFlags & 0b11
	return SampleLoopType(bits)
}

func (s *InstrumentSample) Is16bits() bool {
	return (s.TypeFlags & (1 << 4)) != 0
}

type EnvelopeFlags uint8

func (f EnvelopeFlags) IsOn() bool {
	return f&(1<<0) != 0
}

func (f EnvelopeFlags) SustainEnabled() bool {
	return f&(1<<1) != 0
}

func (f EnvelopeFlags) LoopEnabled() bool {
	return f&(1<<2) != 0
}

type SampleFormat int

const (
	SampleFormatDeltaPacked SampleFormat = iota
	SampleFormatADPCM
)
