package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/quasilyte/xm"
	"github.com/quasilyte/xm/xmfile"
)

/*
note indexes
C  = 0
C# = 1
D  = 2
D# = 3
E  = 4
F  = 5
F# = 6
G  = 7
G# = 8
A  = 9
A# = 10
B  = 11

D#5 = 52
octave := 5-1
(octave × 12) + note_index + 1 = 52
*/

// This simple CLI tool plays the specified XM track using Ebitengine audio player.

func main() {
	flag.Parse()
	flag.Usage = func() {
		fmt.Printf("usage: go run ./cmd/ebitengine-example path/to/music.xm")
		flag.PrintDefaults()
	}
	if len(flag.Args()) < 1 {
		panic("expected at least 1 command-line argument")
	}
	filename := flag.Args()[0]

	// Create a usable XM stream.
	data, err := os.ReadFile(filename)
	if err != nil {
		panic(fmt.Errorf("read XM file: %v", err))
	}
	xmParser := xmfile.NewParser(xmfile.ParserConfig{})
	xmModule, err := xmParser.ParseFromBytes(data)
	if err != nil {
		panic(fmt.Errorf("parsing XM file: %v", err))
	}
	xmStream := xm.NewStream()
	if err := xmStream.LoadModule(xmModule, xm.LoadModuleConfig{}); err != nil {
		panic(fmt.Sprintf("compiling XM module: %v", err))
	}

	for _, n := range xmModule.Patterns[6].Rows[4].Notes {
		fmt.Println(xmModule.Notes[n])
	}

	// Create a sound player using the Ebitengine audio context.
	// You can have multiple players, but only one audio context.
	// See Ebitengine docs to learn more.
	sampleRate := 44100
	audioContext := audio.NewContext(sampleRate)
	player, err := audioContext.NewPlayer(xmStream)
	if err != nil {
		panic(err)
	}

	g := &game{
		player:   player,
		filename: filename,
		paused:   true,
	}

	g.synth = xm.NewSynthesizer(xm.SynthesizerConfig{
		NumChannels: 2,
	})
	if err := g.synth.LoadInstruments(xmModule, xm.LoadModuleConfig{}); err != nil {
		panic(err)
	}
	{
		player, err := audioContext.NewPlayer(g.synth)
		if err != nil {
			panic(err)
		}
		g.synthPlayer = player
	}

	if err := ebiten.RunGame(g); err != nil {
		panic(err)
	}
}

type game struct {
	player *audio.Player

	synth       *xm.Synthesizer
	synthPlayer *audio.Player

	filename string
	paused   bool
}

func (g *game) Update() error {
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		g.paused = !g.paused
		if g.player.IsPlaying() {
			g.player.Pause()
		} else {
			g.player.Play()
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.Key1) {
		g.synth.PlayNote(0, xmfile.PatternNote{
			Note:       52,
			Instrument: 17,
		})
		g.synthPlayer.Rewind()
		g.synthPlayer.Play()
	}
	if inpututil.IsKeyJustPressed(ebiten.Key2) {
		g.synth.PlayNote(0, xmfile.PatternNote{
			Note:       52,
			Instrument: 18,
		})
		g.synthPlayer.Rewind()
		g.synthPlayer.Play()
	}

	return nil
}

func (g *game) Draw(screen *ebiten.Image) {
	if g.paused {
		ebitenutil.DebugPrint(screen, "Paused... press SPACE")
	} else {
		ebitenutil.DebugPrint(screen, fmt.Sprintf("Playing %s...", g.filename))
	}
}

func (g *game) Layout(_, _ int) (int, int) {
	return 640, 480
}
