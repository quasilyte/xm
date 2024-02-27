package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/quasilyte/xm"
	"github.com/quasilyte/xm/xmfile"
)

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
	}

	if err := ebiten.RunGame(g); err != nil {
		panic(err)
	}
}

type game struct {
	player   *audio.Player
	filename string
}

func (g *game) Update() error {
	if !g.player.IsPlaying() {
		g.player.Play()
	}

	return nil
}

func (g *game) Draw(screen *ebiten.Image) {
	ebitenutil.DebugPrint(screen, fmt.Sprintf("Playing %s...", g.filename))
}

func (g *game) Layout(_, _ int) (int, int) {
	return 640, 480
}
