# Q1 preflight budget evidence

This change bounds the parse-long cleanliness walk used by legacy merge preflight. The dense epoch tables from [PR #1360](https://github.com/odvcencio/gotreesitter/pull/1360) were already on main. This patch keeps those tables and the 65,536-unit limit, charges the shared cleanliness walk to that limit, and clears unfinished DFS marks when the walk stops. Completed cache results survive. Refs #1305.

A 16-node witness reproduces the gap on main: a four-unit budget reports one unit and never trips, on both clean and dirty chains. The patched walk stops at four units. A later retry proves the correct clean or dirty result. A second witness checks that an interrupted dirty branch cannot invalidate a completed clean sibling or leave an unfinished parent marked clean.

The initial engine baseline is `8a78a7673e502b9d3eb1ae7b7f99328b951b3976`. The branch rebases onto `c0fd2895f45366116ee366bd4844deca1305bfdc`, which adds CI runner tests without changing parser code. The engine patch applies cleanly to `sol61/g6-merge-election` at `9297823e`; the G6 election change and this budget change occupy different parts of `glr.go`.

## Deterministic counters

The 412-row, 206-language ledger gate passes. The separately captured before and after snapshots are byte-identical, SHA-256 `cc32935db65906abdae75112c6df065a7607a25a3606f21de368497ad69104ce`. Totals sum full-parse and first-edit events; live versions is the maximum. These are measured snapshots, rather than sums of the checked-in tolerance ledger.

| Route | Tokens before → after | New nodes before → after | Maximum live before → after | Reused bytes before → after | Block splices before → after |
| --- | ---: | ---: | ---: | ---: | ---: |
| Default | 135,882 → 135,882 | 499,590 → 499,590 | 54 → 54 | 147,115 → 147,115 | 10,027 → 10,027 |
| Candidate | 68,111 → 68,111 | 161,277 → 161,277 | 8 → 8 | 75,956 → 75,956 | 2,867 → 2,867 |

No counter, verdict, grammar, or invariant pin is refreshed by this lane.

## Reproduction

Run heavy checks in Docker with `GOWORK=off`. Select one language per process. The corpus fetch verifies the 982 manifest files at their source commits; the top-50 verdict set contains 247 real files and seven pinned cliff fixtures. The authenticated corpus lock stays outside the repository and matches SHA-256 `41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`.

```sh
GOWORK=off go test . -run '^(TestGSS|TestGLRMergeScratch)' -count=1
GOWORK=off GOTREESITTER_V1_INVARIANT_LANGUAGE=c_sharp go test . -run '^TestV1InvariantGateR4EditSession$' -count=1
cd cgo_harness
GOWORK=off GTS_PARITY_MODE=exhaustive go test -tags treesitter_c_parity . -run '^TestParity(FreshParse|IncrementalParse|HasNoErrors)$/^c_sharp$' -count=1
GOWORK=off go test -tags treesitter_c_parity . -run '^TestQ1NativeAllocationProbePreservesFreshTree$' -count=1
```

The benchmark validates the exact fresh C tree digest before timing. C uses the repository's locked binding transport for runtime `f5afe475deb7c0bae6407fb776c76824f717bb61` (0.25.1), through the pinned Go binding. The oracle-upgrade lane remains separate. Use the randomized wrapper for paired before-and-after runs:

```sh
cd cgo_harness
GOWORK=off GTS_Q1_BENCH_LANGUAGE=c_sharp bash ../scripts/run_randomized_benchmarks.sh \
  --output /tmp/q1-after.txt --baseline-root /baseline/cgo_harness \
  --baseline-output /tmp/q1-before.txt --package . --tags treesitter_c_parity \
  --bench-regex '^BenchmarkQ1CliffFull$' --runs 20 --benchtime 750ms
```

Each seed uses one process, `GOMAXPROCS=1`, `-count=1`, and `-benchmem`. Each revision runs Go–C–C–Go. C native allocation requests are counted in separate untimed warm parses. The counter delegates to the original allocator and restores it afterwards. Native bytes count requested allocation sizes; realloc counts as a request even when libc can resize in place. These metrics exclude scanner allocations that bypass the runtime allocator. The ordinary C `B/op` and `allocs/op` rows describe the Go binding and must not stand in for native C allocations.

RSS uses a separate process for each engine on identical generated 1 MiB C# input. The recorded peak is sampled immediately after parsing, before tree inspection. The tree and source digests must match across all rows. RSS includes process startup, grammar loading, and native allocations.

## C transport identity

The pinned Go binding is `github.com/tree-sitter/go-tree-sitter@v0.25.0`. Its bundled `parser.c` has SHA-256 `5e8ae76a9ff12d09d22b1a489124061e82c686a4721470e42f0648c02401080c`; upstream at the declared runtime commit has SHA-256 `c5dc79e198a74ae9b0e2b0a911e279fc54b2c8f99587f100e54d5862202f8010`. The only parser-source difference is the omitted `canceled_balancing = false` assignment in reset. A fresh parser initializes that field to false and a successful parse clears it before returning a tree. No canceled parse appears in these measurements. The existing oracle contract preflight passes. These receipts identify the actual binding transport; they do not claim a separately built standalone upstream-C timing baseline.

## Cold-walk performance

The selected implementation keeps the remaining budget local, publishes consumed units on return, and clears active frames in the rollback pass. Walks outside preflight retain their original unlimited behavior. Every comparison below uses 20 seeds and the randomized wrapper.

| Chain nodes | Before | After | Change | Recorded work before → after |
| ---: | ---: | ---: | ---: | ---: |
| 16 | 142.20 ns | 131.30 ns | -7.67% | 1 → 33 |
| 4,096 | 25.22 µs | 26.39 µs | +4.64% | 1 → 8,193 |
| 65,536 | 446.59 µs | 486.04 µs | +8.83% | 1 → 65,536 |
| 131,072 | 936.43 µs | 490.29 µs | -47.64% | 1 → 65,536 |

At 131,072 nodes, retained frame capacity falls from 2,359,296 to 1,196,032 bytes, a 49.31% reduction. At and above the cap, each attempted walk trips the 65,536-unit budget. The completed clean-sibling cache survives an interrupted dirty branch. The extra work reported for short graphs measures traversal that the old counter omitted; it is not an increase in parser work. All 412 parser ledger rows remain exactly equal.

The original per-step field-update implementation was rejected after a clean 20-seed repeat: 143.5 → 180.4 ns at 16 nodes (+25.71%) and 25.55 → 33.32 µs at 4,096 nodes (+30.42%). A second local-accounting variant reached 125.4 ns, 26.91 µs, 490.6 µs, and 495.0 µs at the four depths; its cap-boundary overhead was +9.77%. The selected variant reduces that overhead to +8.83%. The remaining small-graph regressions pay for the enforced limit and valid-cache rollback. No threshold changed.

## File timing and native allocation ratios

Each row combines the two Go and two C phases per seed. Go/C is the median of the 20 paired per-seed ratios. The native C allocation probe runs three untimed warm parses before the timing cycle. C binding `B/op` is retained in the raw output but is not used as native C memory.

| Fixture | Seeds before / after | Go/C before → after | Change | Go/native-C bytes after | Go/native-C allocation requests after |
| --- | ---: | ---: | ---: | ---: | ---: |
| cliff-c_sharp | 20 / 20 | 3.382 → 3.355 | -0.81% | 0.625× | 0.256× |
| cliff-elixir | 20 / 20 | 1.214 → 1.212 | -0.15% | 0.000× | 0.000× |
| cliff-markdown | 20 / 20 | 1.702 → 1.708 | +0.37% | 0.001× | 0.000× |
| cliff-python | 20 / 20 | 3.357 → 3.336 | -0.61% | 0.002× | 0.001× |
| csharp-140288 | 20 / 20 | 3.256 → 3.368 | +3.46% | 0.237× | 0.255× |
| csharp-largest | 20 / 20 | 4.435 → 4.525 | +2.03% | 9.982× | 0.980× |
| csharp-median_1 | 20 / 20 | 2.801 → 2.788 | -0.46% | 17.097× | 0.342× |
| csharp-median_2 | 20 / 20 | 18.549 → 18.552 | +0.02% | 10.561× | 1.363× |
| csharp-median_3 | 20 / 20 | 42.064 → 41.998 | -0.16% | 17.216× | 1.271× |

The locked C# median files 2 and 3 already exceed the hard 10× Go/C limit on both revisions, approximately 18.55× and 42× respectively. No exemption was added. Only completed 20-seed files supply comparison evidence. Partial files, if any, are listed as unfinished work in the final status. The primary Go trio is also recorded under `receipts/q1-preflight/`, against the same current-main engine baseline. The CPU profiles attribute preflight cost; their samples are not independent timing comparisons.

## Peak RSS

On the identical 1,048,645-byte generated C# fixture, Go peak RSS is 442,904 → 442,496 KiB and C is 103,412 → 103,472 KiB. Go/C is 4.283× → 4.276×. All four source digests and fresh-tree digests agree. The Go peak is 432.5 → 432.1 bytes per source byte; both exceed the design's 400-byte hard limit. This is an existing failure, not a new exemption.

## Validation and remaining work

- Focused GSS/merge tests, the bounded-walk regression, and focused race checks pass.
- The root package and CI-planning tests pass after supplying Git context to the container. The complete `GOWORK=off go test ./... -count=1 -parallel=1 -timeout=25m` run also exposes the existing Swift constructor expectation (306 rules versus 299) and a grammargen package timeout. Unchanged main reproduces the Swift failure; the isolated ABI14 test passes.
- Fresh, incremental, and error-status parity execute all 618 selected subtests across 206 grammars, with zero skips and zero failures, before and after.
- The 254 real/cliff fixtures preserve 326,236 accepts, 104,441 rejects, and every verdict digest, with zero work-limit trips. The original checked-in full-corpus pins already differ on main for C#, PHP, C++, Elixir, and Haskell. The direct pinned gate remains failing for those same five; no pin is refreshed.
- The report-only cliff detector retains existing failures: C# legacy multi-stack share is 99.70% versus C 0.00%; Python legacy has eight stacks versus C two and 99.87% multi-stack share versus C 17.86%. Before/after frontier counters agree. Other selected cliff results are recorded in the summary.
- The final invariant run passes 197 of 206 languages. Awk, JavaScript, Meson, and Twig retain the same incremental/fresh digest mismatches. Elsa, Haskell, Kotlin, Nickel, and PowerShell time out after two minutes in the existing missing-node dependency walk. Godot Resource passes in 114.52 seconds; baseline timed out at 120 seconds after consuming 114.23 CPU seconds. This near-boundary outcome is not evidence of a semantic fix.
- The final engine patch applies cleanly to G6 at `9297823e`. The initial budget patch also passed the focused GSS/merge suite when combined with that branch. The final generic regression and race checks pass independently.

The baseline received the same diagnostic probes and benchmarks, without changing its production parser. The source SHA in the summary ties the measured candidate to the committed parser after rebasing over CI-only main changes. Correctness, counters, and timing stay separate; only two Docker containers were launched. A separate lane lock was used because the shared Docker lock was held by an idle container. Correctness ran with a two-CPU container quota; every timing process used one CPU, affinity CPU 0, and `GOMAXPROCS=1`.

Main later advanced to `f9828512c` through incremental fixes #1390 and #1394 while the timing run was active. These receipts remain tied to the audited `c0fd2895f` base; its parser file is identical to the initial main baseline. The newer incremental fixes are not included in this branch or in the stated invariant results. Integration must rerun the invariant and ledger gates.

Refs #1305. The missing work bound is fixed, but the issue cannot be closed while its invariant, original-pin, broad-suite, and RSS gates remain failing. Corpus equivalence does not prove that declining an over-budget merge preserves canonical fresh-C trees on every possible input. The full per-language performance matrix, including 16-site edit percentiles, is not claimed here. No gate, exemption, allowlist, corpus lock, or pin changed; no approval item is introduced.

Unfinished timing work: csharp-1048576.txt (20 seeds not complete).
