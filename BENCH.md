# BENCH — canonical performance claims and how to reproduce them

This is the single authoritative page for gotreesitter performance claims.
Every number here is either pinned to a release receipt in
[CHANGELOG.md](CHANGELOG.md) or derived from the ratcheted fleet ledger in
[`cgo_harness/perf_scan`](cgo_harness/perf_scan/). Anything not on this page
is not a claim.

## The one-paragraph story

gotreesitter trades some raw full-parse speed for portability. It is pure
Go, has no cgo, cross-compiles anywhere Go does (including `wasip1`), and
stays fully visible to `go test -race`. Editor-style incremental workloads
are where it is fast outright — a no-edit reparse takes nanoseconds and a
one-byte edit runs at microsecond scale on the historical control, both
zero-allocation. Full parses are ratcheted against the C runtime
language-by-language, with explicit caveats instead of averaged marketing
numbers.

## Canonical benchmark status

The generated 500-function Go source is a historical straight-LR control. It
contains no imports, selectors, methods, types, comments, strings, or
control flow, and it never forks under the current parser. It remains
useful for tracking the incremental fast paths and single-stack
regressions, but it is not a representative full-parse headline.

The project has withdrawn the former **1.895x C** full-parse headline and
its **29% materialization** decomposition in favor of the locked real-code
replacement below. The old comparison also used different Go grammar
artifacts: gotreesitter used the project-locked 1,425-state/214-symbol
grammar, while the C benchmark used a 1,404-state/212-symbol grammar
bundled by the old smacker binding.

Historical control results, retained as workload-specific receipts:

| Lane | Benchmark | Historical result |
|---|---|---|
| Full parse (materialized, straight LR) | `BenchmarkGoParseFullDFA` | 10.907 ms on the pinned quiet host |
| One-byte incremental edit | `BenchmarkGoParseIncrementalSingleByteEditDFA` | 649 ns/op, 0 allocs |
| No-edit reparse | `BenchmarkGoParseIncrementalNoEditDFA` | 2.43 ns/op, 0 allocs |

Reproduce the historical control:

```sh
GOMAXPROCS=1 go test . -run '^$' \
  -bench 'BenchmarkGoParseFullDFA|BenchmarkGoParseIncrementalSingleByteEditDFA|BenchmarkGoParseIncrementalNoEditDFA' \
  -benchmem -count=10 -benchtime=750ms
```

`BenchmarkGoParseCoreDFA` is a parser-loop diagnostic (no tree
materialization). The project never quotes its numbers as full-parse
numbers. See the benchmark-integrity note below.

### Historical quiet-host receipt

The v0.24.1 audit withdrew the pre-correction full-parse headlines pending a
quiet-host rerun of the corrected public benchmark. First such receipt,
2026-07-12, main @ 04f75d15, Intel Xeon D-2141I @ 2.20 GHz (idle host,
`taskset -c 14`, `GOMAXPROCS=1`, `-count=10 -benchtime=750ms`), medians:

| Lane | ns/op | B/op | allocs/op |
|---|---|---|---|
| `BenchmarkGoParseFullDFA` | 12,245,000 | 1,527 | **9** |
| `BenchmarkGoParseIncrementalSingleByteEditDFA` | 1,976 | 0 | **0** |
| `BenchmarkGoParseIncrementalNoEditDFA` | 9.85 | 0 | **0** |
| `BenchmarkGoParseCoreDFA` (diagnostic) | 8,737,000 | 996 | 6 |

Wall-clock numbers are host-specific — this is a low-clock server part, so
do not compare it against dev-box history. The allocation counts remain
valid for this fixture. The full-minus-core decomposition does not
generalize beyond this straight-LR control.

### Editor-latency (O(edit)) status

The campaign O(edit) workstream targets deterministic, per-keystroke
reparse cost. Its continuous instrument is workstream W5, a continuous
integration (CI) gate. W5 has not yet published a sealed receipt for the
numbers below. Cite them as v0.44.0/v0.44.1 CHANGELOG measurements, not as
sealed claims.

- CSS editor-style incremental edits: node reuse rises from 63.8% to 99.5%
  (PR #395, workstream W1).
- Go clean-file incremental edits: `ReuseRejectRootNonLeafChanged` holds at
  a constant 9, regardless of file size (PR #398, workstream W1b).
- A 137KB near-top insert: about 22ms, down from about 47ms on the same
  fixture (PR #398, workstream W1b).

See [CHANGELOG.md](CHANGELOG.md) v0.44.0 and v0.44.1 for full context,
including known issues.

## The one C oracle

Every new "versus C" claim names and fingerprints the same oracle inputs:

- upstream tree-sitter runtime v0.25.1, commit
  `f5afe475deb7c0bae6407fb776c76824f717bb61`;
- `github.com/tree-sitter/go-tree-sitter v0.25.0` wrapper commit
  `adc13ffd8b2c0b01b878fda9f7c422ce0df5fad3` for in-process parity;
- tree-sitter-go commit
  `2346a3ab1bb3857b48b29d779a1ef9799a248cd7`, from
  `grammars/languages.lock`;
- C runtime and grammar compiled with `-O2` into the static publication
  artifact, with compiler identity, flags, and artifact SHA-256 in the receipt.

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
blends it into the human-code headline. A pinned external-project fixture
will be added to check repository self-reference.

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

### Sealed epoch — v9 (hardware-attested, authoritative)

This epoch supersedes v8 below. It is the project's current Go-vs-C
authority. The project sealed it inside a hardened confidential-computing
enclave, then independently verified every cryptographic layer. The enclave
ran on AMD Secure Encrypted Virtualization (SEV) Confidential Space, with
debug mode disabled since boot. An independent audit verified the
attestation token's RS256 signature (RSA with SHA-256) against Google's
live JSON Web Key Set (JWKS). The audit also verified the Ed25519 receipt
signature and confirmed that the payload nonce binds to the signed receipt
bytes. Every check in the independent verification tool reported `OK`;
none reported `FAIL`.

The sealed epoch times one full, non-incremental parse per fixture, using
the same method as v8, v0.45.0, and run6. Each Go run calls the public
`Parser.Parse` API, then checks root completeness: byte 0 to end of
source, with no parse error. The C oracle runs the matching
`public_validated` lane: parse, root completeness, and a check for
`ts_node_has_error`. Neither side walks the full tree. Each fixture-backend
pair runs with `GOMAXPROCS=1`. Each pair runs for at least 10 seconds of
elapsed time. Go binaries are built with `GOAMD64=v3` (the Go compiler's
x86-64 microarchitecture-level flag) and profile-guided optimization
(PGO), pinned to profile SHA-256
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

Per-fixture change against the v8 epoch (production 4.575x, compact
3.681x geomean):

| Fixture | Production delta | Compact delta |
|---|---:|---:|
| `rewrite.go` | +20.45% | +6.28% |
| `query_compile.go` | +7.79% | +17.51% |
| `language.go` | +0.96% | +9.06% |
| `grammargen/lr.go` | -6.43% | +0.99% |
| **Geomean** | **+5.24%** | **+8.30%** |

Per-fixture change against the v0.45.0 epoch (production 5.526x, compact
2.9975x geomean):

| Fixture | Production delta | Compact delta |
|---|---:|---:|
| `rewrite.go` | -7.71% | +41.31% |
| `query_compile.go` | -28.24% | +27.48% |
| `language.go` | -21.13% | +33.95% |
| `grammargen/lr.go` | +10.30% | +29.62% |
| **Geomean** | **-12.88%** | **+32.99%** |

Production ratios rose on three fixtures and fell on one
(`grammargen/lr.go`) against v8; the production geomean rose 5.24%.
Compact ratios rose on all four fixtures against v8; the compact geomean
rose 8.30%. This is the second straight compact regression: v8 regressed
compact against v0.45.0, and this epoch regresses compact again, against
v8. All four compact ratios stay above 3.0x C. No model bound a target
ratio for this seal. Report the measurement as measured, not adjusted to
fit an expectation.

Mandatory caveats:

- This epoch ran only at the 10-second benchtime floor; it did not resample
  at 750ms or 5s.
- The C oracle binary is byte-identical to v8's, run6's, and v0.45.0's: all
  four cite SHA-256
  `a2aaf98ec7b869d5e1a311fe209fe2bdc31335a60d2840a1ffc3d72877cd3274`. The C
  side did not change between epochs; only the Go binaries and the sealed
  commit changed.
- 46 commits landed on gotreesitter `main` between this epoch's build
  commit (`492cd600`) and the v8 epoch's build commit (`3325e0b1`). Three
  named changes landed in that range: the provenance-predicate flip with
  rank-walk retirement, the single-header condense/elect fast-path
  successors to PR #580, and the condense direct-append plus dispatch
  cell-validation hoist (PR #622). Each merged on a correctness argument,
  with timing explicitly deferred to this seal. This receipt seals the
  current tip; it does not isolate any one change. Read the deltas above
  as the combined effect of every merge in that range, plus ordinary
  Confidential Space VM-to-VM hardware variance across separate launches.
  This receipt cannot separate those causes.
- The A/A null figures are reported values, not sealed pass/fail gates. The
  receipt embeds no numeric A/A threshold. Both this epoch's A/A geomeans
  sit close to 1.0 (production 0.9989, C-oracle 0.9985), so measurement
  noise alone does not explain the size of the deltas above.

Citation:

- Run ID: `strictboundary-20260802T062212Z-v9`.
- Image: `strict-boundary:v9`, digest
  `sha256:80c809f0335e1422e6f073b1f776cfa09c3f4f5d1c2f8abe8fef686b08d5970b`
  (10s benchtime, hardened, authoritative), reference
  `us-central1-docker.pkg.dev/bookt-cc/gts-cs-bench/strict-boundary:v9`.
- SHA-256 of the C oracle binary:
  `a2aaf98ec7b869d5e1a311fe209fe2bdc31335a60d2840a1ffc3d72877cd3274`
  (unchanged from v8, run6, and v0.45.0).
- SHA-256 of the 10-second driver source
  (`cgo_harness/pure_c/go_timing_oracle_10s.c`):
  `4512710e99d36d161145113ce980d740bf1668470219140be0c15df5ce9ce58c`.
- SHA-256 of the PGO profile (`pgo/default.pgo`):
  `1e5e9aea594f4fcbc3c0fb5d4064d2fb856b13b2faf0994f747520615eaa2ae2`.
- gotreesitter git HEAD: `492cd600cfd2bfd0bd3e8b3233fc2f477ff0e887` (short
  form `492cd600`). This was the `main` tip at seal time: the merge commit
  for PR #622 (`cedar/l1-followon-fastpaths`).
- Verification: the sealed run metadata records `attestation_status: ok`.
  An independent re-check confirmed the RS256 signature against Google's
  live JWKS at fetch time. The payload nonce
  (`e20102e23972b033af5e63c01fc1e12177edef4f03a0ee1b99a96064c69c192f`)
  matches the attestation token's `eat_nonce` claim, binding the token to
  this exact receipt payload. The independent verification tool
  (`gts-cs-bench-harness/verify/main.go`) printed every check as `OK`,
  zero `NOTE`, zero `FAIL`: full PASS verdict.

This epoch's ratios are not comparable to the historical bare-metal
receipts further below. The enclave method (single-run `ns/op` at
`GOMAXPROCS=1` with 10s benchtime) and the bare-metal method (median of ten
process-isolated samples) measure differently. A higher enclave ratio does
not indicate a Go regression against those receipts.

The enclave image itself is not reproducible outside Confidential Space.
Treat its output as an attested measurement, not a locally rerunnable
script. Reproduce the driver and oracle sources from git `492cd600` on
`main`.

### Superseded epochs and receipts (index)

Each superseded receipt below stays a valid historical claim. Its full
section lives in this file's Git history and in CHANGELOG.md at the named
version. This index keeps the claim; it does not repeat the method.

| Receipt | Geomean or headline | Where |
|---|---|---|
| Sealed epoch v8 | production 4.575x C, compact 3.681x C | commit `3325e0b1` |
| Sealed epoch v0.45.0 | production 5.526x C, compact 2.9975x C | v0.45.0 |
| Sealed epoch run6 | first 10-second-floor enclave run | v0.45.0 series |
| v0.40.0 production baseline | 4.851050x C geomean | v0.40.0 |
| v0.39.0 production baseline | historical straight-LR control | v0.39.0 |
| v0.27.0 combined receipt | withdrawn 1.895x C ratio | v0.27.0 |
| Same-host C calibration | withdrawn, oracle mismatch | v0.24.1 audit |
| Compact candidate, selected-store, work-count receipts | diagnostic lanes, never public claims | CHANGELOG v0.41.0 to v0.48.0 |

## Go-vs-C fleet scoreboard (full parse, real corpora)

Source of truth: [`cgo_harness/perf_scan/perf_ratio_budgets.json`](cgo_harness/perf_scan/perf_ratio_budgets.json)
and the wave-3 ledger. 204 of 206 languages carry ratcheted budgets
(D and F# are held out pending memory RCA; every exclusion is named in the
[known-gap ledger](cgo_harness/perf_scan/wave3_sweep_status.md)).

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

## Memory receipts (the v0.24 → v0.26.1 campaign)

On the exact Poppler witness under a hard 2 GiB container:

- Retained post-GC heap: 862,803,056 → **409,862,040 bytes (−52.5%)**
  after final-tree arena compaction (v0.26.1).
- Node header: **144 → 104 bytes** via arena-backed field-metadata
  sidecars (v0.26.0).
- Bounded raw-shape reclamation: **−192 MiB** retained (v0.26.0).

Each claim shipped with exact Go/C S-expression and deep parity on the same
witness; memory wins that break the selected tree do not merge.

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
5. **Benchmark integrity is audited.** A 2026-07-11 audit found that the
   then-canonical full-parse benchmark silently ran a no-tree diagnostic
   path; the project withdrew the affected headline numbers (1.54 ms /
   7 allocs and successors). A 2026-07-14 audit then found that the
   replacement workload never forked and that its C lane used a different
   grammar. The project withdrew those claims too, rather than patch around
   them.
6. **Quiet-host discipline.** Publication runs use `GOMAXPROCS=1`, a pinned
   core, ten process-isolated samples per backend and fixture in five
   paired Go-C-C-Go cycles, at least 750 ms of internal timing, and Go
   `benchmem`. Contended-box measurements are smoke evidence only.

## Multi-workload tracking

```sh
go run ./cmd/benchmatrix --count 10   # bench_out/matrix.{json,md} + raw logs
```

The default matrix includes a bounded, warmed language-family full-parse
group reported in MB/s, plus the Go/editor lanes above.
