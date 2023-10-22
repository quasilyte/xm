Optimization:
- add benchmarks
- don't initialize runners for empty channels

Features:
+ forward looping
- ping pong looping
+ 16-bit samples
- amiga frequency
- panning
- panning envelopes
- volume envelopes
- effects

Corner-cases:
- play an empty pattern if pattern order entry index is invalid

Correctness:
- tests (check the generated PCM bytes)
- make rewind/reset/seek work properly
- test streaming API (io.Reader) with Ebitengine audio players
