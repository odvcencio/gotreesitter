# Incremental edit-session recovery receipt

Measured on 2026-10-01 against baseline `9148a96db0dcf95594d72671f48862d72bfee343`.
The fixes are `9b070d785` (token-source verification) and `ddbf24387`
(dependency traversal), followed by `7ff54092e` (streams without rebuilders)
and `150f1454f` (settled fresh results). The final two fixes use
`0920ba6eb133b4465595c9e1a232da4bf0e4f7cf` as their baseline.
No pins, allowlists, corpus locks, or thresholds changed.

Token-source entries deferred recovery verification to an accepted-error merge
retry that only the DFA entries run. Inserting `x` before the following C input
incorrectly marked the unchanged enum as erroneous:

```c
/**/#e
enum{L,/**/C,/**/T,E};r e[]{}
```

The fixed result matches fresh Go and locked C, including fields, spans, points,
flags, and deep digest. The regression covers four public token-source entries
on both routes. Verification adds work to this previously incorrect result:
tokens 24 → 48 and new nodes 165 → 274; reuse stays at zero.

The separate dependency fix removes a second walk of error-bearing children.
Each materialized recovery level previously doubled the work. The 64-level
regression times out after five seconds with the unchanged helper and completes
immediately with the fix. Missing-leaf padding/lookahead and lazy pending-parent
checks remain covered by their existing tests.

METRIC: C edit steps equal fresh Go | 71/72 -> 72/72 | 9b070d785 | Git ctype.c, 1016 bytes, 72 edits
METRIC: C edit steps equal fresh C | 5/72 -> 6/72 | 9b070d785 | same locked file and edits
METRIC: R4 edit sessions completing | 201/206 -> 206/206 | ddbf24387 | 72 edits per grammar, same 90s process bound
METRIC: depth-16 dependency check | 1828456.5 ns -> 151.8 ns | ddbf24387 | 20 paired seeds, 750ms, GOMAXPROCS=1
METRIC: depth-8 dependency check | 7023.50 ns -> 72.36 ns | ddbf24387 | same settings

The C file is [Git's ctype.c](https://github.com/git/git/blob/9ac3f193c05c2237e2b14ebaa1149e9fc8a1abe0/ctype.c),
SHA-256 `ef2be0e95b607f509f1fc1cc6ed458bba0a2ae3f9cb94fcf799c3cd54b10a4d8`.
The remaining 66 differences in that session also occur in fresh Go parsing.

A stream that opts into reuse but cannot rebuild itself has the same missing
comma defect. Stable token boundaries do not certify recovery choices. The
parser now takes the fresh path before consuming such a stream for general
reuse. Three additional public entries on both routes match locked C exactly;
all 14 C witness cases pass. Tokens stay at 24, new nodes fall from 165 to 112,
and reuse stays at zero. No-edit allocations remain zero.

METRIC: C entries without rebuilders equal locked C | 0/6 -> 6/6 | 7ff54092e | 36-byte witness, three entries on two routes
METRIC: C witness new nodes without a rebuilder | 165 -> 112 | 7ff54092e | profiled prefix insertion, tokens 24 and reused bytes 0 unchanged

An independent replacement audit passes 12,148 Go cases, then finds a Python
failure in a 1021-byte fixture. Replacing `g` with `(` in this 20-byte reduction
reproduces it:

```python
t
from s import(eg,)
```

Fresh verification had already selected an accepted error tree. The incremental
API retried that fresh result at wider limits and selected a clean module,
erasing the syntax error. Accepted results from the fresh route now keep their
ordinary retry decision. The reduced and original inputs match fresh Go on
both routes and both ordinary/profiled entries. The reduced regression also
checks root coverage, error reporting, and zero no-edit allocations.

METRIC: Python witness equals fresh Go | 0/4 -> 4/4 | 150f1454f | 20-byte replacement, two entries on two routes
METRIC: Python witness tokens | 33 -> 21 | 150f1454f | profiled replacement, reused bytes 0 unchanged
METRIC: Python witness new nodes | 92 -> 61 | 150f1454f | same replacement

Fresh Go still differs from locked C in the Python witness's recovery shape.
The fix restores D8 and the syntax-error report without changing fresh parsing.
The published reuse-lane revision `f554905ce` also reproduces the witness, with
different reuse attribution. Replay this regression when integrating that lane.

The R4 sessions previously exceeding 90 seconds now complete as follows:

| Grammar | After, seconds |
| --- | ---: |
| Elsa | 0.72 |
| Haskell | 0.21 |
| Kotlin | 1.28 |
| Nickel | 0.14 |
| PowerShell | 2.95 |

All 206 R4 sessions and 206 smoke sessions pass fresh-Go equality, root coverage
or explained stops, ERROR-root reporting, and zero no-edit allocations. This is
24,720 edit steps. The 412-row deterministic ledger passes at its existing 2%
threshold, including a complete rerun after the final two guards. For the
initial two fixes, six scoped before/after ledgers are byte-identical. Representative
R4 edit counters are unchanged:

| Default DFA route | Tokens | New nodes | Reused bytes | Block splices |
| --- | ---: | ---: | ---: | ---: |
| Go, before and after | 572 | 3802 | 1166 | 296 |
| C, before and after | 255 | 2903 | 1093 | 32 |

Timing for the initial two fixes uses `scripts/run_randomized_benchmarks.sh`,
20 paired seeds, alternating
baseline/current order, one process per seed, `GOMAXPROCS=1`, `-count=1`,
`-benchtime=750ms`, and `-benchmem`. Docker uses Go 1.25 on Linux/amd64 with a
4 GiB memory limit and a 3 GiB Go memory limit. Host CPU: Intel Core Ultra 9 285.

| Workload | Before | After | Allocation result |
| --- | ---: | ---: | --- |
| Go full DFA | 4.046 ms | 4.007 ms | 8 allocs/op unchanged |
| Go single-byte edit DFA | 135.3 µs | 134.4 µs | 5 allocs/op unchanged |
| Go no-edit DFA | 3.429 ns | 3.446 ns | 0 bytes and allocations |
| C four-file full parse | 3.927 ms | 3.880 ms | 3.168 MiB and 3397 allocations, unchanged |
| C four-file edit | 22.65 ms | 22.43 ms | 27.21 MiB unchanged; 244 → 243 allocations, not significant |
| C four-file no-edit | 3.710 µs | 3.701 µs | 0 bytes and allocations |
| Dependency chain, depth 8 | 7023.50 ns | 72.36 ns | 0 bytes and allocations |
| Dependency chain, depth 16 | 1828456.5 ns | 151.8 ns | 0 bytes and allocations |

`benchstat` finds no significant Go or C API time change. Both dependency
benchmarks improve significantly. The C workload includes token-source
construction and four files per operation.

A separate 1 MiB dense-C diagnostic retains the old, incremental, and fresh
trees. Both versions accept and cover all input, return equal incremental/fresh
trees, and take the same memory-budget full retry. Both report tokens 414848,
new nodes 4787527, zero reuse, and peak RSS 1298284 KiB. Its high RSS is already
present in the baseline; these changes do not reduce it.

The authenticated corpus audit covers 186 grammars, including Lean's separately
locked corpus. Each runs 72 cumulative edits. All 186 invariant gates pass in
the baseline and initial-two-fix versions, and every Go incremental/fresh digest is identical between the
two versions. The largest-file selector is limited to 1 KiB by the current
receipt tool. This differs from older, larger-file census selections.

Full fresh-C session passes stay at 38 for the repeatable grammars. CMake adds
an unstable possible 39th pass: identical binaries and input produce different
C trees. Four baseline replays match fresh C at 15, 15, 72, and 15 of 72 steps;
four fixed replays match at 72/72, with two C incremental/fresh differences in
the first replay. Go digests stay identical. The locked [CMake scanner](https://github.com/uyha/tree-sitter-cmake/blob/58993af75218bc99a1f5a04c832a5937e7c422cb/src/scanner.c)
allocates uninitialized state and resets the bracket level without resetting
the token field. No oracle configuration or gate was changed to hide this.

The failing priority-language sessions also diverge in fresh Go parsing. Their
fresh-parser fixes remain with the separate lane. These two changes do not
raise the repeatable full fresh-C grammar count.

The final two guards pass the 206-language correctness and counter gates.
Their paired timing run remains pending: another process holds the shared
heavy-work lock during an extended pause. The 186-grammar fresh-C census has
not been repeated after these guards. The larger 2253-byte PowerShell R4 replay
now finishes within the same 90-second bound and equals fresh Go at 72/72
steps, but equals fresh C at 25/72 steps. It adds no full fresh-C pass.

METRIC: repeatable full fresh-C sessions | 38/186 -> 38/186 | ddbf24387 | authenticated 1 KiB corpus selection, 72 edits

The canonical priority sessions have the following locked-C matches. Each
retains 72/72 incremental/fresh-Go matches before and after. The fresh-Go/C
match count equals the incremental-Go/C count in every row. The C regression
above uses another selected file, so its improvement does not change this
canonical C session.

| Grammar | Before, steps | After, steps | First failing step |
| --- | ---: | ---: | ---: |
| Go | 8/72 | 8/72 | 9 |
| Python | 0/72 | 0/72 | 1 |
| JavaScript | 0/72 | 0/72 | 1 |
| TypeScript | 4/72 | 4/72 | 5 |
| Java | 72/72 | 72/72 | None |
| Rust | 0/72 | 0/72 | 1 |
| C | 5/72 | 5/72 | 6 |
| C++ | 0/72 | 0/72 | 1 |
| C# | 6/72 | 6/72 | 7 |
| Ruby | 8/72 | 8/72 | 8 |
| PHP | 9/72 | 9/72 | 10 |
| Bash | 0/72 | 0/72 | 1 |

To reproduce the focused regressions, run these commands inside the bounded
Docker parity runner, serialized by the workstation's heavy-work lock:

```sh
(cd cgo_harness && GOWORK=off GOMAXPROCS=1 go test . -tags treesitter_c_parity \
  -run '^TestTokenSourceRecoveryFrontierMatchesFreshC$' -count=1)
GOWORK=off GOMAXPROCS=1 go test . \
  -run '^TestTreeEditSkipsDeepMissingDependencyBeforeReplacement$' -count=1
GOWORK=off GOMAXPROCS=1 go test . \
  -run '^TestIncrementalFreshVerificationDoesNotRetryAgain$' -count=1
GOWORK=off GOMAXPROCS=1 go run ./cmd/perfcounterledger
```

For each registry language, run `TestV1InvariantGateR4EditSession` with
`GOTREESITTER_V1_INVARIANT_LANGUAGE` set to that language, and
`TestIncrementalCReuseFleetInvariant/<language>` in a separate process.

For paired timing, mount the baseline checkout and use:

```sh
bash scripts/run_randomized_benchmarks.sh \
  --baseline-root "$BASELINE_ROOT" --baseline-output "$BEFORE_OUTPUT" \
  --output "$AFTER_OUTPUT" \
  --bench-regex '^(BenchmarkGoParseFullDFA|BenchmarkGoParseIncrementalSingleByteEditDFA|BenchmarkGoParseIncrementalNoEditDFA)$'
```

For the dependency benchmark, copy only its new test/benchmark file into the
baseline checkout, retain the baseline production helper, and select
`BenchmarkMissingNodeDependencyErrorChain`. For C, run the same script from
`cgo_harness`, use `treesitter_c_parity`, and select
`BenchmarkParityRealCorpusParse(Full|IncrementalSingleByteEdit|IncrementalNoEdit)/c/(gotreesitter|tree-sitter-c)`.
Set `GTS_REAL_CORPUS_BENCH_LANGS=c`, the authenticated corpus root and lock,
lock filtering, largest-file ordering, four files, and a 1024-byte file limit.
