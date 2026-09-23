# Benchmark notes and history

Canonical, linkable performance claims live in [BENCH.md](../BENCH.md) — the
real-code full-parse matrix, historical incremental controls, the Go-vs-C
fleet scoreboard, memory receipts, and the methodology that keeps them
honest. This page holds the surrounding narrative and historical corrections
that used to live in the README.

`BenchmarkGoParseFullDFA` calls `Parser.Parse`, requires a complete root, and
releases the materialized tree. Its generated 500-function source is now a
historical straight-LR control, not the representative full-parse headline.
`BenchmarkGoParseCoreDFA` remains a parser-loop diagnostic that suppresses
ordinary tree materialization.

## Historical corrections

A 2026-07-11 audit found that older versions of `BenchmarkGoParseFullDFA`
called `ParseNoResultCompatibilityBenchmarkOnly`, which also enabled the
no-tree path. The previously published `1.54 ms`, `728 B/op`, and
`7 allocs/op` figures — and v0.24.0's later `978 B/op` and `5 allocs/op`
figures — therefore described parser-core diagnostics, not a full
materialized parse. The project withdrew those headline comparisons. The
corrected historical control later measured 10.907 ms on a pinned quiet
host. A second audit found that this source never forks and that its C
comparison used a different Go grammar from gotreesitter. The project
withdrew the former 1.895x ratio and 29% materialization decomposition
rather than promote them as representative claims.

The replacement canonical matrix freezes four clean, human-authored Go files
spanning 5-236 KiB. They reach 12-18 live stacks and constructed-to-selected
node ratios of 3.65-4.47. Admission requires exact deep parity against one
oracle: upstream tree-sitter v0.25.1 commit `f5afe475…`, tree-sitter-go
commit `2346a3ab…`, compiled with `-O2` into a fingerprinted static C
artifact. The benchmark and parity lanes share those oracle sources and
identity; see [BENCH.md](../BENCH.md) for the full contract and fixture hashes.

The first strict materialized real-Go publication receipt, shipped with
v0.37.0, measured **5.481673x C** by equal-fixture geomean and **6.313799x
C** for the fixed-suite sum of medians. Those corrected real-code results,
rather than the withdrawn 1.895x straight-LR comparison, established the
full-parse baseline; [BENCH.md](../BENCH.md) records every per-fixture median,
RSS value, and receipt hash.

The current Go-versus-C authority is the sealed v9 epoch in
[BENCH.md](../BENCH.md#sealed-epoch--v9-hardware-attested-authoritative). It
measures public `Parser.Parse` at **4.815x C** by equal-fixture geomean over
the four locked real-Go fixtures, and the compact route at **3.986x C**. The
epoch ran inside a hardware-attested enclave with one pinned CPU, and an
independent verification confirmed every cryptographic layer. Its ratios
are not comparable to the earlier bare-metal receipts, so the v0.37.0 and
v0.40.0 rows stay historical claims only. See [BENCH.md](../BENCH.md) for the
per-fixture table, the A/A null test, the exact identities, and the
support boundary.

The historical incremental measurements on the same generated 500-function
Go workload were `649 ns` for a one-byte edit and `2.43 ns` for a no-edit
reparse. They remain narrow workload-specific controls. The locked
incremental matrix validates correctness and classifies work, but it does
not establish a general comparative Go/C speed headline.

## Reproduction commands

```sh
# Authenticated real-code Go full-parse lanes:
GOMAXPROCS=1 go test . -run '^$' \
  -bench '^BenchmarkGoParseWarmRealDFA$' \
  -benchmem -count=10 -benchtime=750ms

# Complete locked static-C publication receipt (clean, quiet, Docker host):
bash cgo_harness/pure_c/run_canonical_go_full_parse.sh --core <idle-cpu>

# Build-tagged compact fresh-full candidate (diagnostic, fail-closed):
bash cgo_harness/pure_c/run_canonical_go_full_parse.sh \
  --go-backend candidate --core <idle-cpu>

# Historical straight-LR full parse plus incremental API controls:
GOMAXPROCS=1 go test . -run '^$' \
  -bench 'BenchmarkGoParseFullDFA|BenchmarkGoParseIncrementalSingleByteEditDFA|BenchmarkGoParseIncrementalNoEditDFA' \
  -benchmem -count=10 -benchtime=750ms

# Parser-core attribution only:
GOMAXPROCS=1 go test . -run '^$' \
  -bench '^BenchmarkGoParseCoreDFA$' \
  -benchmem -count=10 -benchtime=750ms
```

Correctness and performance are gated separately. For Go-vs-C real-corpus
timing, see [`cgo_harness/perf_scan`](../cgo_harness/perf_scan/README.md); its
ratchet records caveats, timeouts, and resource gaps instead of
extrapolating from this generated-Go microbenchmark.

## Benchmark matrix

For repeatable multi-workload tracking:

```sh
go run ./cmd/benchmatrix --count 10
```

This emits `bench_out/matrix.json` (machine-readable), `bench_out/matrix.md`
(summary), and raw logs under `bench_out/raw/`. The default matrix includes
a bounded, warmed language-family full-parse group, reported with `MB/s` so
you can compare parser throughput across generated source sizes. Use
`--only-family` to isolate that group, `--family-unit-count` to scale it, or
`--no-family` for the narrower Go/editor matrix.

## Known limitations

- **Full-parse throughput**: the sealed v9 epoch measures public
  `Parser.Parse` at **4.815x C** by equal-fixture geomean over the four
  locked real-Go fixtures, with `grammargen/lr.go` as the worst fixture at
  **6.065x C** (see [BENCH.md](../BENCH.md)). The compact route measures
  **3.986x C** on the same fixtures. The former ~2.1x row used a
  straight-LR synthetic and a different Go grammar, so it stays historical
  only. The
  locked incremental matrix validates correctness and classifies work,
  but general incremental Go/C performance has no current
  publication-grade headline. Full-parse throughput varies by grammar
  and corpus shape; GLR-heavy code, highly ambiguous languages, and very
  large generated files remain the main performance frontier.
- **GLR safety caps**: the parser enforces iteration, stack depth, and
  node count limits proportional to input size. These prevent
  pathological blowup on grammars with high ambiguity, but they impose
  a ceiling on the maximum input complexity that parses without error.
  The caps are tunable but not removable without risking unbounded
  resource consumption.
