Optimization:
- add benchmarks
- don't initialize runners for empty channels
- cut comment-only samples

Features:
+ forward looping
+ ping pong looping
+ 16-bit samples
- amiga frequency
- panning
- panning envelopes
- volume envelopes

Effects:
- 0x0D pattern break
- 3xx portamento to notes

Bugs:
- vr_befo.xm results in an error (consumed 9271 extra bytes of the pattern data)
- bronzed_girl.xm results in an error (unknown sample encoding scheme)
- hardcore.xm results in an error (unknown sample encoding scheme)

Corner-cases:
- play an empty pattern if pattern order entry index is invalid

Correctness:
- tests (check the generated PCM bytes)
- make rewind/reset/seek work properly
- test streaming API (io.Reader) with Ebitengine audio players
