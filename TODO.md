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

Corner-cases:
- play an empty pattern if pattern order entry index is invalid
- jump to an overflowing pattern

Correctness:
- tests (check the generated PCM bytes)
