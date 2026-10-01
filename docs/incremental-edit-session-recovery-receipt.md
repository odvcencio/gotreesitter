# Incremental edit-session recovery receipt

Measured on 2026-10-01 against baseline `9148a96db0dcf95594d72671f48862d72bfee343`.
The fixes are `9b070d785` (token-source verification) and `ddbf24387`
(dependency traversal). No pins, allowlists, corpus locks, or thresholds changed.

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
threshold. Six scoped before/after ledgers are byte-identical. Representative
R4 edit counters are unchanged:

| Default DFA route | Tokens | New nodes | Reused bytes | Block splices |
| --- | ---: | ---: | ---: | ---: |
| Go, before and after | 572 | 3802 | 1166 | 296 |
| C, before and after | 255 | 2903 | 1093 | 32 |

Timing uses `scripts/run_randomized_benchmarks.sh`, 20 paired seeds, alternating
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
both versions, and every Go incremental/fresh digest is identical between the
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

To reproduce the focused regressions, run these commands inside the bounded
Docker parity runner, serialized by the workstation's heavy-work lock:

```sh
(cd cgo_harness && GOWORK=off GOMAXPROCS=1 go test . -tags treesitter_c_parity \
  -run '^TestTokenSourceRecoveryFrontierMatchesFreshC$' -count=1)
GOWORK=off GOMAXPROCS=1 go test . \
  -run '^TestTreeEditSkipsDeepMissingDependencyBeforeReplacement$' -count=1
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
