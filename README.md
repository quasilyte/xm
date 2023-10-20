# xm

This package is intended to be used in game development in Go with [Ebitengine](https://github.com/hajimehoshi/ebiten/).

If you just need to parse an XM file, you can use the `xm/xmfile` package without importing the `xm` package itself.

The `xm` package provides an XM music stream that produces 16-bit signed PCM LE data. This process can be describes as:

1. Read and decode the XM file (`xmfile` package)
2. Convert XM file data into something optimized for playing
3. Create a player object that can go through this data and produce PCM chunks

This package implements some of the common XM effects. Feel free to submit a PR to fill the feature gap.

Why would you even need an XM player in your game? The answer is simple: size. This is very important in web exports of your game. An average OGG file can have a size of 6-8mb while the same song in XM can fit in ~300kb or even less.
