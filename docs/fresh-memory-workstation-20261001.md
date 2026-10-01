# Fresh parse memory: workstation continuation, 2026-10-01

All five requested languages now have a complete 20-seed fresh-parse comparison. Go, Java, and Python retain their authenticated VM receipts. TypeScript and C# have new independent workstation comparisons; VM and workstation timing samples are not pooled.

The engine stays at `60ef79513` plus `69af4dfd7`, compared with main at `9148a96db`. The ten declaration-complete 137 KiB and 1 MiB fixtures match the locked native C runtime `f5afe475deb7c0bae6407fb776c76824f717bb61` and its C binding on both revisions, cold and warm. The continuation reproduces every original source/tree digest and C allocator count. All five target R4 edit sessions pass; their ten default/candidate ledger rows remain identical to the committed 206-language comparison. No pins or thresholds change.

The benchmark moved to `roottest/bench` to preserve the 756/756 root census and all 12 API tag sets. CI now assigns the two new internal helper packages to the support race lane. The header-copy test reads and checks its replacement header, resolving static analysis without removing its original ownership assertion. The implementation, parser defaults, and language graduation lists remain unchanged by these continuation fixes.

Workstation settings: Intel Core Ultra 9 285, CPU 18, Go 1.25.14, GOWORK=off, GOMAXPROCS=1, 4 GiB Docker limit, GOMEMLIMIT=3GiB, 750ms, count=1, and benchmem. Every heavy host command uses the shared lock. Randomized comparisons alternate revision order per explicit shuffle seed and use complete-operation Go-C-C-Go cycles. Grammar loading, parser construction, source generation, native launch, and digest preflight are outside operation time; parse, coverage/error checks, and tree release/delete are inside.

Go heap metrics measure allocation volume; native C counts successful allocator callbacks and requested bytes, excluding headers/rounding and direct allocations outside those callbacks. Native memory instrumentation runs separately from uninstrumented CPU and RSS. Go C-leg B/op describes subprocess transport. RSS is the whole-process high-water mark from `/usr/bin/time -v`, normalized by actual source bytes, including runtime/grammar/source/GC overhead.

| Language | KiB | Input bytes | Go B/op before → after | Go allocs/op before → after | Native C B/op | Native C calls/op |
|---|---:|---:|---:|---:|---:|---:|
| typescript | 137 | 140,304 | 33,888 → 33,888 | 29 → 29 | 7,160,000 | 77,046 |
| typescript | 1024 | 1,048,617 | 163,112,600 → 160,556,696 | 113 → 113 | 52,788,664 | 568,027 |
| c_sharp | 137 | 140,356 | 2,036,248 → 2,036,248 | 23,911 → 23,911 | 8,592,712 | 93,883 |
| c_sharp | 1024 | 1,048,645 | 398,944,440 → 393,808,056 | 172,806 → 172,806 | 62,133,712 | 678,868 |

Twenty independent RSS process pairs alternate revision order for each language/size. Each Go process uses the same warm-up/calibration and 3x measured operations; native C uses one warm-up and three operations. Brackets are min/max, not confidence intervals.

| Language | KiB | Go RSS/input before | Go RSS/input after | C RSS/input |
|---|---:|---:|---:|---:|
| typescript | 137 | 303.586 [299.615, 306.155] | 304.870 [302.068, 306.272] | 69.948 [69.948, 69.948] |
| typescript | 1024 | 329.948 [294.387, 351.041] | 328.925 [292.321, 458.345] | 58.732 [58.732, 58.732] |
| c_sharp | 137 | 668.377 [665.809, 669.778] | 664.758 [663.357, 666.159] | 85.564 [85.564, 85.564] |
| c_sharp | 1024 | 665.597 [565.060, 785.128] | 705.774 [581.423, 885.535] | 64.261 [64.261, 64.261] |

CPU medians combine GoA/GoB samples; the Go/C ratio is the median per seed of `(GoA+GoB)/(CA+CB)`. Brackets give the min/max of the 40 Go operation samples. Ratios use 20 paired cycles.

| Language | KiB | Go seconds before | Go seconds after | Median change | Go/C before → after |
|---|---:|---:|---:|---:|---:|
| typescript | 137 | 0.051 [0.045, 0.073] | 0.050 [0.045, 0.084] | -1.39%; under 5% | 3.030 → 2.961 |
| typescript | 1024 | 0.436 [0.413, 0.590] | 0.442 [0.400, 0.753] | +1.53%; under 5% | 3.140 → 3.091 |
| c_sharp | 137 | 0.191 [0.139, 0.395] | 0.176 [0.147, 0.282] | -7.58%; inconclusive | 7.615 → 7.648 |
| c_sharp | 1024 | 1.738 [1.495, 2.142] | 1.728 [1.494, 2.516] | -0.60%; under 5% | 9.118 → 9.419 |

Benchstat finds no clear timing change in either Go leg for either language/size. TypeScript 1 MiB operation time directionally rises 1.53%; its paired ratio falls 1.56%. C# 1 MiB operation time falls 0.60%; its paired ratio rises 3.30%, under 5%. C# 137 KiB's 7.58% operation median decrease is inconclusive. RSS medians are mostly neutral; C# 1 MiB rises 6.04% (p=0.096), inside the unchanged 10% ratchet. Its 1.29% allocation-volume reduction does not establish an RSS improvement.

The RSS ceiling is unresolved. TypeScript 1 MiB's candidate peak reaches 458.345 bytes/input byte although its median falls slightly. A baseline diagnostic with GC tracing reaches 463.865 bytes/input byte on repetition 22; the same-protocol untraced baseline check is pending. C# remains above 400 on both revisions. The original VM receipts also retain Python's RSS ceiling failures and 1 MiB median Go/C above 10, plus identical R4 timeouts for Kotlin, Haskell, PowerShell, and Elsa. These results add no exemption and do not claim the release floor has been reached.

Validation: serialized Docker helper/metadata/checkpoint/arena/layout/memory-contract checks, exact locked-C preflights for all ten workloads on both revisions, five R4 edit sessions, ten unchanged scoped ledger rows, amd64/386 helper checks, focused race checks, and existing static-oracle tamper/compile-flag checks. L0, R6, and CI package-plan guards pass; all 32 CI checks at `103d996b0` pass. Original receipt hashes verify 31/31. [Validation output](measurements/fresh-memory-workstation-20261001/validation.txt), [JSON receipt](measurements/fresh-memory-workstation-20261001/continuation.json), [RSS statistics](measurements/fresh-memory-workstation-20261001/rss-benchstat.txt), and the [original five-language report](fresh-memory-20261001.md) retain the evidence.

Assumptions and remaining work: generated complete declarations supply the requested sizes; corpus-backed performance and Go 1.26 timings were not measured. The C callback counts have the scope described above. Node/arena retention remains with the fresh-speed lane (#1404); this PR changes checkpoint growth and field storage. No update(pins) commit is needed.

Reproduce with the original report's detached-baseline and native preflight steps. Select `--package ./roottest/bench`, require all eight Go-C-C-Go benchmark rows, run 20 seeds, and use the same pinned Docker CPU and memory settings for both revisions. RSS runs compile that package's test binary and execute `-test.run '^$' -test.bench '^BenchmarkFreshMemory$/<language>/<size>KiB$' -test.benchtime=3x -test.count=1` in 20 independent process pairs.
