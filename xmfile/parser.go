package xmfile

import (
	"encoding/binary"
	"fmt"
	"strings"
)

type parser struct {
	// Data holds the XM file input data bytes.
	data []byte

	// Offset is our current position inside the data.
	offset int

	// Module holds the results of XM parsing.
	module Module

	uint16pool     objectPool[uint16]
	patternRowPool objectPool[PatternRow]

	noteSet map[uint64]uint16

	scratchEnvelopePoints [24]EnvelopePoint

	config ParserConfig

	needsReset bool

	// These fields below are needed for better error reporting.
	stage         string
	stageIndex    int
	subStage      string
	subStageIndex int
}

func newParser(config ParserConfig) *parser {
	p := &parser{
		noteSet: make(map[uint64]uint16, 512),
		config:  config,
	}
	p.module.Notes = make([]PatternNote, 0, 512)
	initObjectPool(&p.uint16pool, 2048*8)
	initObjectPool(&p.patternRowPool, 64*20)
	return p
}

func (p *parser) Parse(data []byte) error {
	p.data = data
	p.reset()
	p.needsReset = true
	return p.parse()
}

func (p *parser) reset() {
	if !p.needsReset {
		// This will only happen during the first run of the parser.
		return
	}

	p.offset = 0
	for k := range p.noteSet {
		delete(p.noteSet, k)
	}
	p.uint16pool.Reset()
	p.patternRowPool.Reset()

	// Now reset the module.
	{
		notes := p.module.Notes[:0]
		patterns := p.module.Patterns[:0]
		instruments := p.module.Instruments[:0]
		patternOrder := p.module.PatternOrder[:0]
		p.module = Module{
			Notes:        notes,
			Patterns:     patterns,
			Instruments:  instruments,
			PatternOrder: patternOrder,
		}
	}
}

func (p *parser) startStage(name string) {
	p.stage = name
	p.stageIndex = -1
	p.subStage = ""
	p.subStageIndex = -1
}

func (p *parser) startSubStage(name string) {
	p.subStage = name
	p.subStageIndex = -1
}

func (p *parser) formatStage() string {
	var b strings.Builder
	b.Grow(len(p.stage) + len(p.subStage) + 16)
	b.WriteString(p.stage)
	if p.stageIndex >= 0 {
		fmt.Fprintf(&b, "[%d]", p.stageIndex)
	}
	if p.subStage != "" {
		b.WriteByte('.')
		b.WriteString(p.subStage)
		if p.subStageIndex >= 0 {
			fmt.Fprintf(&b, "[%d]", p.subStageIndex)
		}
	}
	return b.String()
}

func (p *parser) errorf(format string, args ...any) *ParseError {
	text := fmt.Sprintf(format, args...)
	tag := p.formatStage()
	if tag != "" {
		text = tag + ": " + text
	}
	e := &ParseError{
		Message: text,
		Offset:  p.offset,
	}
	return e
}

func (p *parser) dataBytesRemaining() int {
	return len(p.data) - p.offset
}

func (p *parser) sliceData(l int) []byte {
	return p.data[p.offset : p.offset+l]
}

func (p *parser) skip(l int, what string) {
	if p.dataBytesRemaining() < l {
		panic(p.errorf("unexpected EOF while reading %s", what))
	}
	p.offset += l
}

func (p *parser) read(l int, what string) []byte {
	if p.dataBytesRemaining() < l {
		panic(p.errorf("unexpected EOF while reading %s", what))
	}
	b := p.sliceData(l)
	p.offset += l
	return b
}

func (p *parser) readOptionalString(l int, what string) string {
	if !p.config.NeedStrings {
		p.skip(l, what)
		return ""
	}
	return p.readString(l, what)
}

func (p *parser) readString(l int, what string) string {
	if p.dataBytesRemaining() < l {
		panic(p.errorf("unexpected EOF while reading %s", what))
	}
	stringBytes := p.sliceData(l)
	p.offset += l
	return convertCstring(stringBytes)
}

func (p *parser) readDword(what string) int32 {
	if p.dataBytesRemaining() < 4 {
		panic(p.errorf("unexpected EOF while reading %s", what))
	}
	v := binary.LittleEndian.Uint32(p.sliceData(4))
	p.offset += 4
	return int32(v)
}

func (p *parser) readWord(what string) int16 {
	if p.dataBytesRemaining() < 2 {
		panic(p.errorf("unexpected EOF while reading %s", what))
	}
	v := binary.LittleEndian.Uint16(p.sliceData(2))
	p.offset += 2
	return int16(v)
}

func (p *parser) readByte(what string) uint8 {
	if p.dataBytesRemaining() < 1 {
		panic(p.errorf("unexpected EOF while reading %s", what))
	}
	b := p.data[p.offset]
	p.offset++
	return b
}

func (p *parser) parse() (err error) {
	defer func() {
		rv := recover()
		if rv != nil {
			if panicErr, ok := rv.(*ParseError); ok {
				err = panicErr
			} else {
				panic(rv)
			}
		}
	}()

	p.parseModule()

	return err // See the deferred call aboves
}

func (p *parser) parseModule() {
	// Add an empty note (ID=0).
	p.module.Notes = append(p.module.Notes, PatternNote{})

	p.startStage("header")
	p.parseHeader()

	p.startStage("pattern")
	for i := 0; i < p.module.NumPatterns; i++ {
		p.stageIndex = i
		pat := p.parsePattern()
		p.module.Patterns = append(p.module.Patterns, pat)
	}

	p.startStage("instrument")
	for i := 0; i < p.module.NumInstruments; i++ {
		p.stageIndex = i
		inst := p.parseInstrument()
		p.module.Instruments = append(p.module.Instruments, inst)
	}
}

func (p *parser) parseHeader() {
	idText := p.readString(17, "id text")
	if !strings.EqualFold(idText, "extended module: ") {
		panic(p.errorf("unexpected ID text: %q", idText))
	}

	p.module.Name = strings.TrimSpace(p.readString(20, "module name"))

	if b := p.readByte("magic byte"); b != 0x1a {
		panic(p.errorf("expected 0x1a, found 0x%0x", b))
	}

	p.module.TrackerName = strings.TrimSpace(p.readString(20, "tracker name"))

	version := p.readWord("version")
	p.module.Version[0] = uint8(version >> 8)
	p.module.Version[1] = uint8(version & 0xff)

	headerSize := p.readDword("header size") - 4
	if p.dataBytesRemaining() < int(headerSize) {
		panic(p.errorf("invalid header size: %d", headerSize))
	}
	offset := p.offset + int(headerSize)

	p.module.SongLength = int(p.readWord("song length"))
	if p.module.SongLength <= 0 || p.module.SongLength > 256 {
		panic(p.errorf("invalid song length value: %d", p.module.SongLength))
	}

	p.module.RestartPosition = int(p.readWord("restart position"))
	if p.module.RestartPosition > p.module.SongLength {
		p.module.RestartPosition = 0
	}

	p.module.NumChannels = int(p.readWord("number of channels"))
	p.module.NumPatterns = int(p.readWord("number of patterns"))
	p.module.NumInstruments = int(p.readWord("number of instruments"))

	p.module.Flags = uint16(p.readWord("flags"))
	p.module.DefaultTempo = int(p.readWord("default tempo"))
	p.module.DefaultBPM = int(p.readWord("default bpm"))

	p.module.PatternOrder = p.read(p.module.SongLength, "pattern order table")

	p.offset = offset
}

func (p *parser) parsePattern() Pattern {
	var pat Pattern
	patternHeaderLength := p.readDword("pattern header length")
	if patternHeaderLength < 9 {
		panic(p.errorf("invalid pattern header length: %d", patternHeaderLength))
	}
	p.skip(1, "packing type")
	numRows := int(p.readWord("number of rows"))
	if numRows <= 0 || numRows > 256 {
		panic(p.errorf("invalid number of rows: %d", numRows))
	}

	packedPatternDataSize := p.readWord("packed pattern data size")
	if p.dataBytesRemaining() < int(packedPatternDataSize) {
		panic(p.errorf("incomplete packed pattern data"))
	}
	offset := p.offset + int(packedPatternDataSize)

	// Skip is usually 0, but the specs says we should respect the stated header size.
	p.skip(int(9-patternHeaderLength), "skip pattern metadata")

	if packedPatternDataSize == 0 {
		if p.module.EmptyPattern.Rows == nil {
			// Generate a Standard Empty pattern.
			numRows = 64
			pat.IsEmpty = true
			pat.Rows = p.patternRowPool.MakeSlice(numRows)
			// Every byte is expected to be 0x80 (0b1000_0000).
			// This results in MSB set, but no "read_x" bits make
			// every note be 0.
			// Therefore, we fill it with completely empty notes.
			for i := range pat.Rows {
				// Notes are zero values already, no extra loop is needed.
				pat.Rows[i].Notes = p.uint16pool.MakeSlice(p.module.NumChannels)
			}
			p.module.EmptyPattern = pat
		}
		pat = p.module.EmptyPattern
	} else {
		// TODO: read until all (number of rows)*(number of channels) are consumed?
		// The docs claim that numRows may be imprecise in some XM files.
		pat.Rows = p.patternRowPool.MakeSlice(numRows)
		for i := range pat.Rows {
			pat.Rows[i].Notes = p.uint16pool.MakeSlice(p.module.NumChannels)
			for j := 0; j < p.module.NumChannels; j++ {
				var note PatternNote
				b := p.readByte("first note byte")
				readNote := true
				readInstrument := true
				readVolume := true
				readEffectType := true
				readEffectParameter := true
				if b&0b10000000 != 0 {
					// When MSB is set, an alternative (compact) scheme is used for this note.
					// Some bytes may be missing (they default to 0).
					readNote = b&(1<<0) != 0
					readInstrument = b&(1<<1) != 0
					readVolume = b&(1<<2) != 0
					readEffectType = b&(1<<3) != 0
					readEffectParameter = b&(1<<4) != 0
				} else {
					// The first byte was a note.
					readNote = false
					note.Note = b
				}
				if readNote {
					note.Note = p.readByte("pattern note")
				}
				if readInstrument {
					note.Instrument = p.readByte("pattern instrument")
				}
				if readVolume {
					note.Volume = p.readByte("pattern volume")
				}
				if readEffectType {
					note.EffectType = p.readByte("effect type")
				}
				if readEffectParameter {
					note.EffectParameter = p.readByte("effect type parameter")
				}

				id := p.internNote(note)
				pat.Rows[i].Notes[j] = id
			}
		}
	}

	if p.offset < offset {
		panic(p.errorf("found %d redundant bytes in the pattern data", offset-p.offset))
	}
	if p.offset > offset {
		panic(p.errorf("consumed %d extra bytes of the pattern data", p.offset-offset))
	}

	return pat
}

func (p *parser) parseInstrument() Instrument {
	var inst Instrument
	instrumentHeaderSize := p.readDword("instrument header size") - 4
	if p.dataBytesRemaining() < int(instrumentHeaderSize) {
		panic(p.errorf("incomplete instrument header data"))
	}
	offset := p.offset + int(instrumentHeaderSize)

	inst.Name = p.readOptionalString(22, "instrument name")

	p.skip(1, "instrument type")

	numSamples := p.readWord("number of samples")
	if numSamples == 0 {
		if p.offset > offset {
			panic(p.errorf("consumed %d extra bytes", p.offset-offset))
		}
		p.offset = offset
		return inst
	}

	sampleHeaderSize := p.readDword("instrument sample header size") - 4
	if p.dataBytesRemaining() < int(sampleHeaderSize) {
		panic(p.errorf("incomplete instrument sample header data"))
	}
	inst.KeymapAssignments = p.read(96, "instrument samples keymap assignments")

	inst.EnvelopeVolume = p.scratchEnvelopePoints[:12]
	for i := range inst.EnvelopeVolume {
		x := uint16(p.readWord("envelope volume point x"))
		y := uint16(p.readWord("envelope volume point y"))
		inst.EnvelopeVolume[i] = EnvelopePoint{X: x, Y: y}
	}
	inst.EnvelopePanning = p.scratchEnvelopePoints[12:]
	for i := range inst.EnvelopePanning {
		x := uint16(p.readWord("envelope panning point x"))
		y := uint16(p.readWord("envelope panning point y"))
		inst.EnvelopePanning[i] = EnvelopePoint{X: x, Y: y}
	}

	numVolumePoints := p.readByte("number of volume points")
	if numVolumePoints > 12 {
		numVolumePoints = 12
	}
	if numVolumePoints != 0 {
		allocated := make([]EnvelopePoint, numVolumePoints)
		copy(allocated, inst.EnvelopeVolume)
		inst.EnvelopeVolume = allocated
	} else {
		inst.EnvelopeVolume = nil
	}

	numPanningPoints := p.readByte("number of panning points")
	if numPanningPoints > 12 {
		numPanningPoints = 12
	}
	if numPanningPoints != 0 {
		allocated := make([]EnvelopePoint, numPanningPoints)
		copy(allocated, inst.EnvelopePanning)
		inst.EnvelopePanning = allocated
	} else {
		inst.EnvelopePanning = nil
	}

	inst.VolumeSustainPoint = p.readByte("volume sustain point")
	inst.VolumeLoopStartPoint = p.readByte("volume loop start point")
	inst.VolumeLoopEndPoint = p.readByte("volume loop end point")
	inst.PanningSustainPoint = p.readByte("panning sustain point")
	inst.PanningLoopStartPoint = p.readByte("panning loop start point")
	inst.PanningLoopEndPoint = p.readByte("panning loop end point")

	inst.VolumeFlags = EnvelopeFlags(p.readByte("volume type"))
	inst.PanningFlags = EnvelopeFlags(p.readByte("panning type"))

	inst.VibratoType = p.readByte("vibrato type")
	inst.VibratoSweep = p.readByte("vibrato sweep")
	inst.VibratoDepth = p.readByte("vibrato depth")
	inst.VibratoRate = p.readByte("vibrato rate")

	inst.VolumeFadeout = int(p.readWord("volume fadeout"))

	if p.offset > offset {
		panic(p.errorf("consumed %d extra bytes", p.offset-offset))
	}
	p.offset = offset

	inst.Samples = make([]InstrumentSample, numSamples)
	p.startSubStage("sample")
	for i := range inst.Samples {
		p.subStageIndex = i
		sample := &inst.Samples[i]
		p.parseInstrumentSampleHeader(sample)
	}

	p.startSubStage("sampledata")
	for i := range inst.Samples {
		p.subStageIndex = i
		sample := &inst.Samples[i]
		if sample.Length == 0 {
			continue
		}
		sample.Data = p.read(sample.Length, "sample data")
	}

	return inst
}

func (p *parser) parseInstrumentSampleHeader(sample *InstrumentSample) {
	sampleLength := p.readDword("sample length")
	if p.dataBytesRemaining() < int(sampleLength) {
		panic(p.errorf("incomplete instrument sample data"))
	}

	sample.Length = int(sampleLength)
	sample.LoopStart = int(p.readDword("sample loop start"))
	sample.LoopLength = int(p.readDword("sample loop length"))
	sample.Volume = int(p.readByte("sample volume"))
	sample.Finetune = int(p.readByte("sample finetune"))
	sample.TypeFlags = p.readByte("sample type")
	sample.Panning = p.readByte("sample panning")
	sample.RelativeNote = int(p.readByte("sample relative note number"))

	format := p.readByte("sample encoding")
	switch format {
	case 0:
		sample.Format = SampleFormatDeltaPacked
	case 0xAD:
		sample.Format = SampleFormatADPCM
	default:
		panic(p.errorf("unknown sample encoding scheme (%#02x)", format))
	}

	sample.Name = p.readOptionalString(22, "sample name")
}

func (p *parser) noteHash(n PatternNote) uint64 {
	return (uint64(n.Note) << 0) |
		(uint64(n.Instrument) << 8) |
		(uint64(n.Volume) << 16) |
		(uint64(n.EffectType) << 24) |
		(uint64(n.EffectParameter) << 32)
}

func (p *parser) internNote(n PatternNote) uint16 {
	hash := p.noteHash(n)
	if id, ok := p.noteSet[hash]; ok {
		return id
	}

	id := uint16(len(p.module.Notes))
	n.ID = id
	p.module.Notes = append(p.module.Notes, n)
	p.noteSet[hash] = id

	return id
}
