# Parity + Perf Harness Framework

This document defines the unified harness model for correctness and performance.
It is intended to keep regressions obvious, reproducible, and attributable.

## 1) Oracles and Invariants

Use differential testing against the C runtime as the primary oracle.

- Structural oracle:
  - Node-by-node comparison (`type`, `start/end byte`, `named`, `missing`, child count)
  - Parse runtime must report complete parse (`stopReason=accepted`, `truncated=false`)
- Highlight oracle:
  - Capture-level diff by `(capture_name, start_byte, end_byte)`
  - Curated gate is strict "no new regressions"

## 2) Tiered Corpus Model

Maintain four corpus tiers per language:

- `smoke`: tiny valid samples for fast CI and parser sanity.
- `real_small/real_medium/real_large`: pinned OSS files from manifest-driven repo snapshots.
- `edge_cases`: minimized repro fixtures for grammar traps (conflicts, extras, recovery, injections).
- `invalid_cases`: malformed inputs that validate error recovery stability.

Real corpus sourcing should remain lock-backed and reproducible. Use:

- `cgo_harness/cmd/build_real_corpus`
- `cgo_harness/testdata/top50_manifest.json`

Build command:

```sh
go run ./cgo_harness/cmd/build_real_corpus \
  -profile cgo_harness/testdata/top50_manifest.json \
  -out cgo_harness/corpus_real
```

Real corpus quality bar:

- Target `small`/`medium`/`large` bucket coverage per language.
- Files come from lock-pinned upstream commits.
- Manifest records source path + commit + SHA256 for reproducibility.
- Enforced minimum: at least 2 files per language and total bytes above the
  medium threshold.

Optional quality check:

```sh
cd cgo_harness
GTS_REAL_CORPUS_MANIFEST=corpus_real/manifest.json \
  go test . -run TestRealCorpusManifestQuality -count=1
```

## 3) Gate Separation

Correctness and perf are intentionally separate gates.

- Correctness gate:
  - `go test ./... -count=1`
  - cgo parity suites:
    - `TestParityFreshParse`
    - `TestParityIncrementalParse`
    - `TestParityHasNoErrors`
    - `TestParityIssue3Repros`
    - `TestParityGLRCanaryGo`
    - `TestParityGLRCanarySet`
    - `TestParityGLRCapPressureTopLanguages`
    - `TestParityHighlight`
- Optional deep probe:
  - `TestParityScalaRealWorldCorpus`
- Perf gate:
  - Stable bench trio only:
    - `BenchmarkGoParseFullDFA`
    - `BenchmarkGoParseIncrementalSingleByteEditDFA`
    - `BenchmarkGoParseIncrementalNoEditDFA`
  - Stable settings:
    - `GOMAXPROCS=1`
    - `-count=10`
    - `-benchtime=750ms`
    - `-benchmem`
  - Compare with `cmd/benchgate` thresholds.

Do not infer correctness from perf numbers.

## 4) Unified Runner

Use `cmd/harnessgate` to run correctness and perf in one reproducible command.

Examples:

```sh
# Full local harness (root tests + cgo parity + perf trio)
go run ./cmd/harnessgate -mode all

# Correctness only
go run ./cmd/harnessgate -mode correctness

# Perf only, compared against an existing baseline output
go run ./cmd/harnessgate -mode perf -bench-base harness_out/base.txt

# Add real-corpus parity pass
go run ./cmd/harnessgate -mode correctness \
  -real-corpus-dir cgo_harness/corpus_real \
  -real-corpus-langs top10

# Add weighted confidence gate (built-in profile)
go run ./cmd/harnessgate -mode correctness \
  -real-corpus-dir cgo_harness/corpus_real \
  -real-corpus-langs top10 \
  -confidence-profile core90 \
  -confidence-min 0.90
```

Artifacts are written to `harness_out/` (logs + summary markdown).

## 5) Failure-to-Fixture Loop

When parity fails:

1. Reduce to smallest reproducer.
2. Add it to `edge_cases` for that language.
3. Keep it in CI permanently.

This is required for long-term stability on GLR and highlight edge behavior.

## 6) Coverage Strategy

Track confidence using:

- Structural coverage:
  - language pass/fail
  - divergence count trend
- Highlight coverage:
  - capture parity trend
  - known-degraded thresholds (must shrink or stay flat)
- Incremental coverage:
  - fresh vs incremental equivalence across deterministic edit sequences.

Target policy:

- Curated set: merge-blocking strictness.
- All languages: no panics, no truncation, no new regressions.
