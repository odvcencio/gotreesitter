# BENCH — canonical performance claims and how to reproduce them

This is the single authoritative page for gotreesitter performance claims.
Every number here is pinned to a release receipt in
[CHANGELOG.md](CHANGELOG.md) or derived from the ratcheted fleet ledger in
[`cgo_harness/perf_scan`](cgo_harness/perf_scan/), or sealed in the
editor-latency CI artifacts. Anything not
on this page is not a claim.

## The one-paragraph story

gotreesitter trades some raw full-parse speed for portability. It is pure
Go, has no cgo, cross-compiles anywhere Go does (including `wasip1`), and
stays fully visible to `go test -race`. Editor-style incremental workloads
are where it is fast outright on the historical control. A no-edit reparse
takes nanoseconds with zero allocations. A one-byte edit takes hundreds of
microseconds and allocates five objects per operation on the current baseline.
Full parses are
ratcheted against the C runtime language-by-language, with explicit caveats
instead of averaged marketing numbers.

## Canonical benchmark status

The generated 500-function Go source is a historical straight-LR control. It
contains no imports, selectors, methods, types, comments, strings, or
control flow, and it never forks under the current parser. It remains
useful for tracking the incremental fast paths and single-stack
regressions, but it is not a representative full-parse headline.

The full-parse headline is the locked real-code result below. Do not quote
the earlier **1.895x C** ratio or its **29% materialization** split: they
compared different Go grammar artifacts (1,425 states and 214 symbols in the
project-locked grammar, 1,404 states and 212 symbols in the grammar bundled
by the old smacker binding).

`BenchmarkGoParseCoreDFA` is a parser-loop diagnostic (no tree
materialization). The project never quotes its numbers as full-parse
numbers. See the benchmark-integrity note below.

### Primary trio baseline

Source: `origin/main` at `5cbd4fb2508cd694fe62d811982f4b018e65132f`.
Measured with Go 1.26.4 on a dedicated benchmark host
(Xeon Platinum 8481C, four cores with SMT off, 31 GB RAM) running Ubuntu 24.04.
Host: pinned to CPU 2; load1 ranged from 0.00 to 0.94 and steal time was zero.

```sh
GOMAXPROCS=1 GOWORK=off taskset -c 2 bash scripts/bench_baseline.sh /tmp/gts-v1-quiet-main-trio.txt
```

The runner uses 20 shuffled seeds and one process per seed. Each process uses
`GOMAXPROCS=1`, `GOWORK=off`, `-count=1`, `-benchtime=750ms`, and `-benchmem`.
These values are medians of 20 runs:

| Benchmark | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| `BenchmarkGoParseFullDFA` | 8,572,502 | 1,275 | 8 |
| `BenchmarkGoParseIncrementalSingleByteEditDFA` | 186,357 | 388 | 5 |
| `BenchmarkGoParseIncrementalNoEditDFA` | 8.30 | 0 | 0 |

A repeated 20-seed control on the same source differed by -0.11% for full
parse, -0.08% for edit, and +0.21% for no-edit. To isolate the host effect
from source changes, the published source revision (`92db945f28de67be51de8235c9cd4e25a900f648`)
was also run twice on this host. Its quiet medians were 8,686,195 ns, 177,281 ns,
and 8.318 ns. Against the previously published values from a loaded host, those exact-source
results are lower by 40.5%, 35.1%, and 45.0%, respectively.

The edit benchmark allocates one scratch buffer after it starts its timer.
That setup adds about 6 B/op at this duration.

### Editor-latency (O(edit)) status

The editor-latency gate runs on code PRs that change engine source, in the required CI `build`
aggregate, as one `editor_latency` job per language (the scope rule is in
`cgo_harness/editor_latency/README.md`). Each job compares the exact PR base and head on one pinned CPU. Any edit cell whose
median paired Go latency rises by more than **5%** fails the job; C slowdown
cannot excuse a Go regression. Cancellation, incomplete evidence, oracle
drift, a correctness failure, and a counter-gate failure also block the job.
The job emits `METRIC:` lines, a GitHub summary, and retained evidence.

The surface follows the first eight languages in
[`grammars/update_tier1_top50.txt`](grammars/update_tier1_top50.txt): TypeScript,
TSX, JavaScript, Python, Java, C#, PHP, and Bash. Go and PowerShell are included
as supplemental controls for the reported regressions. The
[manifest](cgo_harness/editor_latency/fixtures.json) pins whole human-authored
files, immutable upstream commits, hashes, and every edit. Each file receives
a one-byte replacement and undo, a 100-byte comment insertion and deletion,
and a 17-keystroke EOF append. Typing starts after a complete comment prefix
or a PowerShell variable prefix; malformed-prefix recovery remains outside
this receipt.

Every step first passes fresh Go = incremental Go = fresh C = incremental C,
using deep tree digests, both profiled and public Go APIs, complete roots,
accepted parses, and unchanged immutable C snapshots. Correctness runs in
Docker one language at a time before the paired 2% counter ledger and timing.
The C oracle is the locked v0.27.1 runtime, v0.25.0 Go binding, and locked
`-O2` grammars. Both revisions use exactly the same driver and oracle.

Timing uses twenty matching shuffle seeds, alternating base/head order,
Go-C-C-Go samples, `GOMAXPROCS=1`, `GOWORK=off`, `-count=1`, `750ms`, and
`-benchmem` through `scripts/run_randomized_benchmarks.sh`. An operation is a
complete session; the receipt normalizes it per edit. It times `Tree.Edit`,
reparse, and previous-tree release. Fresh Go resets and C snapshot restoration
are excluded. C allocation numbers cover the Go binding, including the tiny
typing snapshot clone, rather than native allocation. See the
[protocol and reproduction command](cgo_harness/editor_latency/README.md).

Each run uploads its evidence as a CI artifact: timings, counters, oracle
fingerprints, environment details, METRIC lines, and a SHA-256 seal. The corpus
lock is never committed.

The fixed manifest selects whole files and valid typing seeds that pass every
step; it never selects or skips fixtures at run time.

## The one C oracle

Every new "versus C" claim names and fingerprints the same oracle inputs:

- upstream tree-sitter runtime v0.27.1, commit
  `10eb5ac119eec9cb4267a8e035066473983f5f3a`;
- `github.com/tree-sitter/go-tree-sitter v0.25.0` wrapper commit
  `adc13ffd8b2c0b01b878fda9f7c422ce0df5fad3` for in-process parity, with its
  sources vendored in `cgo_harness/internal/coracle` and the runtime sources
  pinned by `cgo_harness/internal/coracle/upstream.json`;
- tree-sitter-go commit
  `2346a3ab1bb3857b48b29d779a1ef9799a248cd7`, from
  `grammars/languages.lock`;
- C runtime and grammar compiled with `-O2` into the static publication
  artifact, with compiler identity, flags, and artifact SHA-256 in the receipt.

Receipts and ratios published before the oracle moved to v0.27.1 measured
against v0.25.1 (commit `f5afe475deb7c0bae6407fb776c76824f717bb61`) and stay as
recorded; they are not re-measured. New "versus C" claims name v0.27.1.

The in-process cgo parity transport and the static throughput artifact must
consume those same runtime and grammar sources. The former
`treesitter_c_bench`/`treesitter_c_parity` binding split is a harness defect,
not an accepted source of two oracles.

## Canonical real-Go full-parse matrix

The replacement headline uses immutable snapshots of clean, human-authored Go
that exercise genuine GLR forking:

| Fixture | Bytes | SHA-256 |
|---|---:|---|
| `rewrite.go` | 5,116 | `74c0705f8729670559492fb5460a01b2a1a2a109928e1aeb52736e485e8ff097` |
| `query_compile.go` | 20,168 | `b788ee19b0075f0b9b567a9f93ea657e715bc8a6a40a99d3ca5c761404e71894` |
| `language.go` | 41,387 | `009aa9fd5352c712f3839670c7df8a9b00ae878ee20dc88131a438b2d5edfd9a` |
| `grammargen/lr.go` | 235,626 | `a7e4a1a64b25a60aea36183b9d6d53dcd9240942cdb10e67a3cf9e6ce30f95b2` |

These reach 12-18 live stacks, thousands to tens of thousands of multi-stack
iterations, and constructed-to-selected-node ratios of 3.65-4.47 on the
admitting revision. This page reports generated code separately and never
blends it into the human-code headline.

Produce the complete publication receipt from a clean worktree on a quiet
Docker-capable host:

```sh
bash cgo_harness/pure_c/run_canonical_go_full_parse.sh --core <idle-cpu>
```

The driver authenticates and materializes the four snapshots, admits exact
Go, cgo, and static-C trees, and builds the locked `-O2` oracle. It then
collects ten process-isolated samples per backend and fixture in five
Go-C-C-Go cycles. It fails closed on dirty source, parser or Go runtime
overrides, noisy-host admission, identity drift, and incomplete receipts.
Shortened or skipped gates require `--diagnostic` and carry the
`NONPUBLICATION_DIAGNOSTIC` label.

### Go-vs-C result on the four real-Go fixtures

This is the project's current Go-vs-C result. It times one full,
non-incremental parse per fixture. Each Go run calls the public
`Parser.Parse` API, then checks root completeness: byte 0 to end of source,
with no parse error. The C oracle runs the matching `public_validated` lane:
parse, root completeness, and a check for `ts_node_has_error`. Neither side
walks the full tree. Each fixture-backend pair runs with `GOMAXPROCS=1` for at
least 10 seconds of elapsed time. Go binaries are built with `GOAMD64=v3` (the
Go compiler's x86-64 microarchitecture-level flag) and profile-guided
optimization (PGO), pinned to profile SHA-256
`1e5e9aea594f4fcbc3c0fb5d4064d2fb856b13b2faf0994f747520615eaa2ae2`. An
anti-cheat checksum over the parsed source blocks dead-code elimination.
An A/A null test reruns the same binary against itself 3 times and reports
the median absolute delta per fixture, to bound measurement noise.

| Fixture | Production Go / C | Compact Go / C |
|---|---:|---:|
| `rewrite.go` | 4.653x | 4.348x |
| `query_compile.go` | 4.509x | 4.030x |
| `language.go` | 4.223x | 4.126x |
| `grammargen/lr.go` | 6.065x | 3.493x |
| **Geomean** | **4.815x** | **3.986x** |

The production equal-fixture geomean is **4.815x C**. The compact
equal-fixture geomean is **3.986x C**. The A/A null test reports a
production self-ratio geomean of **0.9989**, with maximum absolute delta
**1.42%**. It reports a C-oracle self-ratio geomean of **0.9985**, with
maximum absolute delta **0.69%**.

Caveats:

- The run used the 10-second benchtime floor; it did not resample at 750ms
  or 5s.
- The C oracle binary has SHA-256
  `a2aaf98ec7b869d5e1a311fe209fe2bdc31335a60d2840a1ffc3d72877cd3274`.
- The A/A null figures are reported values, not pass/fail gates. Both A/A
  geomeans sit close to 1.0, so measurement noise alone does not explain the
  size of the changes against earlier runs.
- The run was built from gotreesitter commit
  `492cd600cfd2bfd0bd3e8b3233fc2f477ff0e887`. The sealed run used a hardened
  confidential-computing environment that is not reproducible outside it, so
  treat its output as an attested measurement, not a locally rerunnable
  script. Reproduce the driver and oracle sources from that commit.
- These ratios are not comparable to the bare-metal receipts that
  `BENCH.md` carried earlier. The sealed method (single-run `ns/op` at
  `GOMAXPROCS=1` with 10s benchtime) and the bare-metal method (median of ten
  process-isolated samples) measure differently.

Earlier receipts are in [BENCH.md at `6b6d4934`](https://github.com/odvcencio/gotreesitter/blob/6b6d49341699df9314b77d52ea92dc950e7364e4/BENCH.md).

## Go-vs-C fleet scoreboard (full parse, real corpora)

Source of truth: [`cgo_harness/perf_scan/perf_ratio_budgets.json`](cgo_harness/perf_scan/perf_ratio_budgets.json).
It covers 204 of 206 languages. D and F# remain held out pending memory analysis.
The budget's `known_budget_class_gaps` field names each gap.

Distribution of observed full-parse ratios (Go time / C time, largest-file
basis, as of the 2026-07-11 ledger):

| Bucket | Languages |
|---|---|
| ≤ 1x (Go at or faster than C) | 10 |
| 1–2x | 64 |
| 2–3x | 29 |
| > 3x | 101 |

Median observed ratio: **~3x**. Small-file DSL grammars dominate the long
tail (up to ~650x on `uxntal`) — C finishes in microseconds there, so the
ratio is mostly fixed per-parse overhead, plus a small set of named
GLR-ambiguity cliffs. Ratios are budgets with caveats, not endorsements:
`green_with_caveat` rows record exactly what the project measured and what
it did not.

Named large-file witnesses (tracked, not hidden): JavaScript
`poppler.js` (3.4 MB — exact parity inside a hard 2 GiB container at
1,708,712 KiB max RSS; full parse 3.50x C), TypeScript
`webworker.generated.d.ts`, Groovy `pleac11_15.groovy`, and the
generated-table class (Go `opGen.go` / `rewriteAMD64.go` and in-repo
witnesses).

Fleet report reduction can preserve a terminal shard only when it carries
the closed status `no_static_c_oracle`, `no_corpus`, or `no_corpus_files`
and carries no measurement or oracle payload. These rows remain fatal
closure findings, and the combined oracle-language manifest omits them.
Certification still rejects them; both modes reject missing, generic,
contradictory, or mixed identity evidence.

## Methodology (why these numbers can be trusted)

1. **Correctness and performance are separate gates.** Before timing, every
   fixture must be clean and full-span. It must preserve exact symbol, byte
   and point ranges, named/extra/missing flags, field ownership, and child
   order against the locked C oracle. See `cgo_harness/`.
2. **Ratchets, not snapshots.** Perf budgets only tighten
   (`cmd/benchgate`, `perf_scan` hard zero-cliff gate); parity exemptions
   only shrink (currently zero).
3. **Benchmark identity fails closed.** The harness records fixture bytes,
   runtime commit, grammar commit, compiler, flags, and the C artifact
   hash. A mismatch aborts admission instead of silently producing another
   ratio.
4. **Lifecycle and warm state are symmetric.** Each backend receives an
   untimed warm parse; the timed lane includes parse, first root
   validation, and tree release/close. The harness measures cold
   construction/loading separately.
5. **Benchmark integrity is audited.** A full-parse benchmark must run the
   tree-building path, must use a workload that forks, and must use the same
   grammar artifact in the C lane.
   Report a measurement as measured, not adjusted to fit an expectation.
6. **Quiet-host discipline.** Publication runs use `GOMAXPROCS=1`, a pinned
   core, ten process-isolated samples per backend and fixture in five
   paired Go-C-C-Go cycles, at least 750 ms of internal timing, and Go
   `benchmem`. Contended-box measurements are smoke evidence only.
7. **Memory changes keep parity.** A memory change ships with exact Go/C S-expression and deep parity on the same witness. A memory win that breaks the selected tree does not merge.

## Multi-workload tracking

```sh
go run ./cmd/benchmatrix --count 10   # bench_out/matrix.{json,md} + raw logs
```

The default matrix includes a bounded, warmed language-family full-parse
group reported in MB/s, plus the Go/editor lanes above.

## Reproduction

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
timing, see [`cgo_harness/perf_scan`](cgo_harness/perf_scan/README.md); its
ratchet records caveats, timeouts, and resource gaps instead of
extrapolating from this generated-Go microbenchmark.

### Benchmark matrix

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

- **Full-parse throughput**: public `Parser.Parse` measures
  **4.815x C** by equal-fixture geomean over the four
  locked real-Go fixtures, with `grammargen/lr.go` as the worst fixture at
  **6.065x C** (see this file). The compact route measures
  **3.986x C** on the same fixtures. The
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