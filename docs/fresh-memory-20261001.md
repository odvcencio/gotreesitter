# Fresh parse memory, 2026-10-01

Bounded scanner checkpoint chunks remove large backing-array copies. Compact field headers reduce each field metadata record from 48 to 32 bytes on amd64. The public Node remains 104 bytes. The engine changes are `60ef79513` and `69af4dfd7`; measurement support is `c75c30706`, with wide-capacity coverage in `fc102fe4a`.

Baseline: `9148a96db`. Locked C runtime: `f5afe475deb7c0bae6407fb776c76824f717bb61`, with the grammar commits and executable hashes in the [JSON receipt](measurements/fresh-memory-20261001/comparison.json). Both revisions match C exactly on all ten generated workloads, on cold and warm parses. Fresh token, node, and stack counts are unchanged. The corpus lock digest was verified externally; the lock is not included.

Docker used Go 1.25.14, GOWORK=off, GOMAXPROCS=1, an 8 GiB container limit, and GOMEMLIMIT=6GiB. Timing uses scripts/run_randomized_benchmarks.sh, 750ms, one process per explicit seed, alternating baseline/candidate order, and Go-C-C-Go legs. The final timing CPU is pinned to CPU 3. Medians and min/max ranges describe this busy VM; small deltas and incomplete screens do not establish a CPU improvement.

Each operation includes a fresh parse, coverage/error validation, and tree release/delete. Source generation, grammar loading, parser construction, warm-up, subprocess launching, and digest validation are outside reported operation time. Go B/op and allocs/op are Go benchmark heap metrics. Native C calls count successful malloc/calloc/realloc callbacks and bytes are requested sizes, excluding allocator headers and rounding. Direct allocations outside tree-sitter callbacks are excluded. C benchmark B/op and allocs/op measure subprocess transport and are not native memory measurements.

137 KiB sources contain complete declarations, so actual byte lengths differ slightly. Small-input Go heap metrics depend on benchmark iteration counts and arena reuse. Their allocation distributions are included; the footprint change is also visible in retained arena bytes. One MiB allocation bytes are much steadier.

| Language | Target KiB | Input bytes | Paired seeds | Go B/op before → after | Go allocs/op before → after | C requested B/op | C allocator calls/op |
|---|---:|---:|---:|---:|---:|---:|---:|
| go | 137 | 140,368 | 20/20 | 176,648 → 148,952 | 27 → 24 | 8,351,040 | 89,777 |
| go | 1024 | 1,048,591 | 20/20 | 180,107,856 → 177,551,952 | 97 → 97 | 61,203,944 | 657,968 |
| java | 137 | 140,327 | 20/20 | 48,472 → 48,472 | 14 → 14 | 7,667,824 | 82,336 |
| java | 1024 | 1,048,602 | 20/20 | 185,145,424 → 180,009,040 | 122 → 122 | 56,601,760 | 607,783 |
| typescript | 137 | 140,304 | 5/20 | 33,888 → 33,888 | 29 → 29 | 7,160,000 | 77,046 |
| typescript | 1024 | 1,048,617 | 5/20 | 163,112,600 → 160,556,696 | 113 → 113 | 52,788,664 | 568,027 |
| python | 137 | 140,348 | 20/20 | 16,795,856 → 16,795,856 | 18 → 18 | 10,892,064 | 118,479 |
| python | 1024 | 1,048,617 | 20/20 | 861,207,048 → 629,910,392 | 1,620 → 985 | 79,565,328 | 865,478 |
| c_sharp | 137 | 140,356 | 5/20 | 2,036,248 → 2,036,248 | 23,911 → 23,911 | 8,592,712 | 93,883 |
| c_sharp | 1024 | 1,048,645 | 5/20 | 398,944,440 → 393,808,056 | 172,806 → 172,806 | 62,133,712 | 678,868 |

RSS is the whole process maximum resident set from /usr/bin/time -v, divided by actual input bytes. Each value below is the median of three independent processes; brackets give min/max. Go used BenchmarkFreshMemory with 3x measured operations and its normal calibration/warm-up. C used the uninstrumented native driver with one warm-up and three measured operations. These include grammar/runtime/input overhead and GC effects; they are not isolated tree size.

| Language | Target KiB | Go RSS/input before | Go RSS/input after | C RSS/input |
|---|---:|---:|---:|---:|
| go | 137 | 388.6 [388.6, 391.5] | 388.4 [387.0, 404.4] | 67.3 [67.2, 67.3] |
| go | 1024 | 420.0 [356.3, 420.4] | 351.9 [349.9, 419.2] | 60.8 [60.8, 60.8] |
| java | 137 | 366.4 [365.5, 367.3] | 363.1 [362.4, 403.3] | 70.1 [69.8, 70.1] |
| java | 1024 | 426.7 [426.7, 426.7] | 421.9 [319.2, 422.3] | 61.9 [61.9, 61.9] |
| typescript | 137 | 344.9 [344.8, 345.9] | 341.0 [338.5, 341.2] | 70.5 [70.5, 71.3] |
| typescript | 1024 | 363.9 [302.9, 364.0] | 361.8 [326.9, 362.0] | 58.8 [58.8, 58.8] |
| python | 137 | 1178.9 [1132.5, 1190.1] | 1084.1 [1072.7, 1088.3] | 77.3 [77.3, 77.6] |
| python | 1024 | 1535.1 [1283.3, 1553.6] | 1260.1 [1224.9, 1318.4] | 68.6 [68.6, 68.6] |
| c_sharp | 137 | 708.3 [706.7, 708.7] | 706.4 [705.1, 706.4] | 84.3 [84.3, 84.3] |
| c_sharp | 1024 | 799.1 [798.6, 800.6] | 783.8 [606.4, 789.4] | 64.0 [64.0, 64.0] |

CPU medians combine the GoA/GoB operation samples. Go/C is the median of each paired cycle’s (GoA + GoB)/(CA + CB). Only seeds whose complete processes passed on both revisions enter the comparison. Brackets give min/max operation seconds, not confidence intervals.

| Language | Target KiB | Paired seeds | Go seconds before | Go seconds after | Median change | Go/C before → after |
|---|---:|---:|---:|---:|---:|---:|
| go | 137 | 20/20 | 0.777 [0.466, 1.034] | 0.784 [0.536, 0.965] | +0.94%; under 5% | 3.94 → 4.11 |
| go | 1024 | 20/20 | 6.307 [3.964, 9.419] | 5.898 [4.171, 8.379] | -6.48% | 4.58 → 4.29 |
| java | 137 | 20/20 | 0.624 [0.488, 0.784] | 0.607 [0.485, 0.832] | -2.67%; under 5% | 3.02 → 3.07 |
| java | 1024 | 20/20 | 4.581 [3.882, 5.618] | 4.361 [3.862, 5.601] | -4.81%; under 5% | 3.24 → 3.20 |
| typescript | 137 | 5/20 | 0.575 [0.528, 0.629] | 0.546 [0.391, 0.572] | -4.98%; under 5% | 3.57 → 3.35 |
| typescript | 1024 | 5/20 | 4.355 [3.810, 5.614] | 4.159 [3.025, 4.472] | -4.50%; under 5% | 3.69 → 3.56 |
| python | 137 | 20/20 | 1.247 [0.579, 2.057] | 1.312 [0.673, 2.184] | +5.27% | 7.79 → 7.90 |
| python | 1024 | 20/20 | 12.708 [5.879, 20.576] | 12.423 [6.139, 18.796] | -2.24%; under 5% | 10.60 → 10.19 |
| c_sharp | 137 | 5/20 | 1.387 [0.815, 1.759] | 1.237 [0.885, 1.785] | -10.80% | 8.78 → 7.97 |
| c_sharp | 1024 | 5/20 | 10.660 [8.479, 11.076] | 10.077 [7.699, 11.129] | -5.47% | 10.64 → 9.62 |

The complete Go and Java timing comparisons show no statistically clear CPU improvement in either Go leg. Directional regressions are retained above. Go 137 KiB operation time rises by less than 5%; its paired Go/C ratio rises by 4.22%. The Python 137 KiB operation median also rises; compare its paired C ratio and the wide sample range before attributing that movement to the code.

Validation (focused output is in measurements/fresh-memory-20261001/focused-test-results.txt):

- Exact deep tree digests against locked native C and the C binding on all ten workloads, for both engine revisions and three cold/warm Go parses.
- Focused metadata, header/backing ownership, checkpoint, Python scanner-state fallback, layout, arena/pool, final compaction, and memory-budget tests in Docker.
- amd64/386 helper coverage and focused race tests. The wide-capacity constructor uses zero-sized elements, so the test needs no large allocation.
- Existing static-oracle tamper, input revalidation, scanner compile-flag, assertion-flag, and language-symbol security tests.
- One-language-at-a-time R4 edit-session gates and default/candidate work/reuse ledgers for all 206 languages. No counters, digests, census pins, expectations, or thresholds were refreshed.

Invariant outcomes: baseline {'pass': 202, 'timeout': 4}; candidate {'pass': 202, 'timeout': 4}. Identical ledger pairs: 206/206. Changed rows: [].

Baseline R4 timeouts occur in Kotlin, Haskell, PowerShell, and Elsa; the candidate reproduces them. A cancelled overlapping Nickel run was repeated separately. These baseline failures remain unresolved. Baseline Python native digest transport was killed on CPU 0; the identical executable and unchanged deadline passed on CPU 3.

The 1 MiB RSS ceiling remains exceeded by some existing workloads. Compare the full ranges, including maximum values. Python and C# remain well above 400 bytes/input byte. This receipt does not add an exemption, change a threshold, graduate a language, or claim the release floor has been reached.

Completed paired timing seeds: go 20/20, java 20/20, typescript 5/20, python 20/20, c_sharp 5/20. Any comparison below 20 seeds remains unfinished at the four-hour limit. Profiling still points to large node reservations and GSS alternate-link/fork allocations, especially the C# object count. This increment addresses checkpoint growth and field metadata storage.

Rejected checkpoint draft: geometric minimum reservation changed a sparse eight-slot reserve to 128 slots. The final implementation keeps exact sparse reservation at eight; the existing expectation passes unchanged. Python 137 KiB retained arena bytes rise by about 0.42% from chunk slack, below the design ledger threshold; B/op and all work/reuse counters remain unchanged.

Assumptions: generated declaration-complete fixtures provide the requested sizes; the supported Docker toolchain is the measurement target. Corpus-backed performance and Go 1.26 timing were not measured. Buckley’s API had no credits and its local backend rejected the configured models; scoped commits used git after the requested Buckley attempts.

The workstation continuation moved the memory harness into `roottest/bench` so the L0 root census remains 756/756. Both benchmark bodies are preserved; only the external package name changes. The relocated package builds in Docker, and all 12 API tag sets pass the layout gate. No census pin or gate threshold changes.

Reproduction uses the committed benchmark and oracle helpers. Copy `roottest/bench/benchmark_fresh_memory_test.go` into the same package in a detached baseline at 9148a96db. To rerun baseline C validation, also copy the oracle test, memory driver, and builder wrapper; do not copy engine changes. Keep the corpus lock external. Within the harness Docker container:

```bash
export GOWORK=off GOMAXPROCS=1
export GTS_C_ORACLE_CACHE=/evidence/native-cache
export GTS_FRESH_MEMORY_OUTPUT=/evidence/native
export GTS_FRESH_MEMORY_LANG=python
cd /workspace/cgo_harness
go test -tags treesitter_c_parity,treesitter_c_perfscan -run "^TestFreshMemoryLockedC$" -count=1 -v
cd /workspace
export GTS_FRESH_MEMORY_NATIVE_DIR=/evidence/native
bash scripts/run_randomized_benchmarks.sh --baseline-root /baseline \
  --baseline-output /evidence/python-before.txt --output /evidence/python-after.txt \
  --package ./roottest/bench --tags "" --bench-regex "^BenchmarkFreshMemoryGoC$" --runs 20 --benchtime 750ms
```

Repeat with go, java, typescript, and c_sharp, in separate processes. Require the eight exact benchmark rows per language as shown in the raw receipts. For RSS, compile each revision’s `./roottest/bench` test binary, then run /usr/bin/time -v with -test.run "^$", -test.bench "^BenchmarkFreshMemory$/<language>/<size>KiB$", -test.benchtime=3x, and -test.count=1. Run the native artifact separately with measure full_operation <source> 1 3 30000000. Alternate revision order over three processes and preserve their maximum RSS values.
