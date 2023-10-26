Optimization:
- add benchmarks
- don't initialize runners for empty channels
- cut comment-only samples

Features:
- amiga frequency
- instrument vibrato settings

Bugs:
- bronzed_girl.xm results in an error (unknown sample encoding scheme)
- hardcore.xm results in an error (unknown sample encoding scheme)
- ping-pong loops don't work correctly

Corner-cases:
- play an empty pattern if pattern order entry index is invalid

Correctness:
- tests (check the generated PCM bytes)
