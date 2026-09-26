# Pinned parser fixtures

The generated manifest pins 59 shapes from `cmd/issue454bench` at 32 KiB, 137 KiB, and 1 MiB. Each row records the actual byte count and a SHA-256 digest. A 137 KiB row also pins its 72-step editing session. The session uses a linear congruential generator with seed 4242. It cycles through insertion, deletion, and replacement.

The real corpus manifest covers all 206 grammars and records 982 source entries. It records each upstream repository, commit, path, source digest, and available license evidence. Selection targets three median files and the largest file within 512 KiB. Five grammars use a named alternate source repository with matching files. Twenty repositories contain fewer than four eligible files. The manifest records every available selection for them.

The repository includes 204 small source samples. They total 438,282 bytes. The CSV and Enforce samples remain in the manifest only. Vega states that dataset licenses vary. Enforce uses the DayZ Public License. Review those terms before local use. All 206 samples have pinned editing sessions. The other source files stay upstream.

Run these commands to check the committed inputs:

```sh
python3 scripts/benchfixture_corpus.py verify --role sample
GOWORK=off go run ./cmd/benchfixturehash
```

Fetch and verify a larger corpus file when a measurement needs it:

```sh
python3 scripts/benchfixture_corpus.py fetch --language go --output /tmp/gts-bench-corpus
```

The fetch command verifies the source commit, license evidence digest, file length, and SHA-256 digest. Regenerate the corpus manifest from the locked local checkouts with `generate`. Then run `GOWORK=off go run ./cmd/benchfixturehash --write --external-root <checkout-root>`. Review the manifest and digest changes before committing them.
