# xm

This package is intended to be used in game development in Go with [Ebitengine](https://github.com/hajimehoshi/ebiten/).

If you just need to parse an XM file, you can use the `xm/xmfile` package without importing the `xm` package itself.

The `xm` package provides an XM music stream that produces 16-bit signed PCM LE data. This process can be describes as:

1. Read and decode the XM file (`xmfile` package)
2. Convert XM file data into something optimized for playing
3. Create a player object that can go through this data and produce PCM chunks

This package implements some of the common XM effects. Feel free to submit a PR to fill the feature gap.

Why would you even need an XM player in your game? The answer is simple: size. This is very important in web exports of your game. An average OGG file can have a size of 6-8mb while the same song in XM can fit in ~300kb or even less.

## Installation

```bash
go get github.com/quasilyte/xm
```

## Quick Start

1. Create a parser to decode XM files into Go objects.

```go
// import "github.com/quasilyte/xm/xmfile"
// See ParserConfig docs to learn the options available.
xmParser := xmfile.NewParser(xmfile.ParserConfig{})
```

2. Decode the XM files that you want to work with.

```go
// xmModule can be manipulated as needed, it's just a data, after all.
// You can add some effects to the module, or mute some instruments, etc.
xmModule, err := xmParser.ParseFromBytes(data)
```

3. Compile an XM module into a playable stream.

```go
// import "github.com/quasilyte/xm"
// You can re-load a module into a stream by using LoadModule again.
// See LoadModuleConfig docs to learn the options available.
xmStream := xm.NewStream()
err := xmStream.LoadModule(xmModule, xm.LoadModuleConfig{})
```

4. Use some audio driver to play the PCM data.

```go
// This example uses Ebitengine audio.
// This library produces 16-bit signed PCM LE data.
sampleRate := 44100
audioContext := audio.NewContext(sampleRate)
player, err := audioContext.NewPlayer(xmStream)

// Now player object can be used to play the XM track.
```

There is an XM event listener API available too.

You don't have to use Ebitengine, but this library was created with Ebitengine in mind.

See [cmd/ebitengine-example](cmd/ebitengine-example/main.go) for a full example.
