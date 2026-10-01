# Fresh legacy parse profiles, 2026-10-01

Complete fresh parsing spends most of its CPU time constructing reduction nodes and the final tree. The first optimization candidate defers raw-shape fingerprints until a comparator requests them. The next candidates address arena retention and forest allocations.

The baseline engine is `f9828512cc58f1bc77b856c2b3451e85266c38b1`. Benchmark definitions are at `52aee909bf0d267b2f18d4c6120c4e4a766ab7ae`. All eight languages use the existing deterministic `benchfixtures.GeneratedSource` shapes, at minimum sizes of 137 KiB and 1 MiB. Actual sources can extend beyond the requested boundary to finish a declaration. These shapes are an assumption for this campaign: they are reproducible, but they do not identify the owner’s earlier quiet-host fixture.

The measured operation calls `Parser.Parse`, validates acceptance and full root coverage, and calls `Tree.Release`. Source generation, grammar loading, parser construction, and one warm parse precede timing. Admission is explicitly legacy; existing forest defaults remain active. No incremental tree is supplied. C timing uses `Parse(source, nil)` and `Tree.Close` through the locked C runtime `f5afe475deb7c0bae6407fb776c76824f717bb61` and locked grammar artifacts.

Profiles use Go 1.26.8, `GOWORK=off`, `GOMAXPROCS=1`, one process per language and size, CPU affinity, `-benchtime=8s`, `-benchmem`, CPU and allocation profiles, and `/usr/bin/time -v`. The allocation report filters to parse/release call stacks. The shared VM also runs other lanes. Profile timings are diagnostic; only the randomized comparison script supplies before/after timing evidence.

The table groups exclusive flat CPU samples by function name. This approximates attribution; it does not measure separate parser phases. Process profiles also include benchmark calibration and grammar initialization. Percentages describe these generated ASCII shapes. UTF handling below 1% here does not establish its cost for Unicode input.

| Language | KiB | Lexing % | Tables/maps % | Stack/forest % | Nodes/reductions % | Allocation/GC % | UTF % | Driver/other % | Max RSS KiB |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| go | 137 | 15.9 | 3.7 | 12.6 | 37.4 | 5.1 | 0.1 | 24.9 | 58400 |
| go | 1024 | 17.7 | 4.9 | 9.5 | 36.3 | 8.6 | 0.0 | 22.7 | 491400 |
| javascript | 137 | 11.5 | 24.4 | 22.5 | 25.0 | 11.4 | 0.0 | 5.2 | 109076 |
| javascript | 1024 | 16.2 | 15.7 | 12.1 | 21.8 | 21.0 | 0.2 | 12.8 | 906220 |
| typescript | 137 | 22.1 | 2.7 | 6.4 | 36.7 | 4.6 | 0.8 | 26.2 | 54076 |
| typescript | 1024 | 22.0 | 3.3 | 6.9 | 35.8 | 7.3 | 0.2 | 24.7 | 405868 |
| python | 137 | 16.3 | 3.5 | 16.6 | 35.7 | 7.5 | 0.1 | 20.3 | 211408 |
| python | 1024 | 13.6 | 2.6 | 18.6 | 27.0 | 16.4 | 0.0 | 21.8 | 1624540 |
| rust | 137 | 22.9 | 3.7 | 6.9 | 35.5 | 3.9 | 0.3 | 26.3 | 59184 |
| rust | 1024 | 24.9 | 3.1 | 5.6 | 33.6 | 10.0 | 0.0 | 22.7 | 511480 |
| java | 137 | 13.5 | 4.4 | 8.6 | 44.8 | 3.1 | 0.1 | 25.2 | 55860 |
| java | 1024 | 13.2 | 3.6 | 8.3 | 40.5 | 7.5 | 0.0 | 27.0 | 374872 |
| c | 137 | 12.7 | 3.4 | 12.5 | 42.3 | 4.2 | 0.0 | 24.5 | 67860 |
| c | 1024 | 10.5 | 3.8 | 15.3 | 42.6 | 5.8 | 0.0 | 21.9 | 392588 |
| cpp | 137 | 13.8 | 3.6 | 6.2 | 44.8 | 5.9 | 0.0 | 25.5 | 84408 |
| cpp | 1024 | 12.9 | 3.6 | 6.4 | 43.9 | 7.6 | 0.0 | 25.6 | 445644 |

Node/reduction work ranks first in seven languages. Java and C++ devote about 40–45% of flat samples to it. Go’s transient GSS reduction call stack accounts for about 20% of sampled CPU, final construction for about 12%, and raw-shape capture for about 5%. Java/C++ raw-shape capture accounts for roughly 7–8%. Lexing ranks next in most languages. JavaScript differs: map control-group matching alone accounts for about 15% of 137 KiB flat samples, and forest candidate-vector growth accounts for about 54% of sampled parse allocation.

At Go 1 MiB, primary node reservation accounts for about 66% of sampled parse allocation. The baseline reserves 1,048,591 primary nodes and exceeds the existing arena-pool retention limit. Complete operations consequently allocate 159,129,104 bytes and 81 objects in the paired benchmark. A bounded primary reservation can retain the arena, but applying it to every parse increased Java allocations; that candidate was rejected.

Baseline profiled 1 MiB RSS already exceeds the design floor for Go, JavaScript, Python, Rust, and C++. Profiling itself adds memory, so the final receipt must also report an identical unprofiled before/after workload. No thresholds or exemptions change.

Machine-readable profile rows are in [the profile receipt](receipts/fresh-parse-profiles-2026-10-01.json). Raw binary profiles are local campaign artifacts; commands below reproduce them.

```bash
GOWORK=off GOMAXPROCS=1 go test -c -o /tmp/gts-fresh.test .
GOWORK=off GOMAXPROCS=1 /usr/bin/time -v /tmp/gts-fresh.test \
  -test.run '^$' -test.bench '^BenchmarkFreshGenerated$/^go$/^137KiB$' \
  -test.benchtime=8s -test.benchmem \
  -test.cpuprofile=/tmp/gts-fresh.cpu -test.memprofile=/tmp/gts-fresh.mem
GOWORK=off go tool pprof -top /tmp/gts-fresh.test /tmp/gts-fresh.cpu
GOWORK=off go tool pprof -top -alloc_space -focus='Parser.*Parse|Tree.*Release' \
  /tmp/gts-fresh.test /tmp/gts-fresh.mem
```

## Defer raw-shape hashes

Raw-shape capture now keeps its lossless reduction sidecar and reserves the same bounded hash cache, then computes the fingerprint when a comparator requests it. Cache misses already use this reconstruction path. Hash width, exact comparison, cache limits, and parse-budget accounting remain unchanged.

Twenty paired shuffle seeds use the standard 750 ms duration, one process per seed, `GOMAXPROCS=1`, and the same complete operation. The shared-host results are:

| Language | KiB | Before ms/op | After ms/op | Change | Interpretation |
| --- | ---: | ---: | ---: | ---: | --- |
| go | 137 | 122.971 | 117.763 | -4.24% | provisional: below 5% |
| go | 1024 | 1006.601 | 970.974 | -3.54% | provisional: below 5% |
| javascript | 137 | 234.187 | 211.445 | -9.71% | inconclusive |
| javascript | 1024 | 2793.301 | 2777.522 | -0.56% | provisional: below 5% |
| typescript | 137 | 163.419 | 149.520 | -8.51% | inconclusive |
| typescript | 1024 | 1335.744 | 1215.245 | -9.02% | lower time |
| python | 137 | 426.408 | 369.608 | -13.32% | inconclusive |
| python | 1024 | 3974.962 | 4191.577 | +5.45% | inconclusive |
| rust | 137 | 94.230 | 90.037 | -4.45% | provisional: below 5% |
| rust | 1024 | 848.558 | 809.589 | -4.59% | provisional: below 5% |
| java | 137 | 88.316 | 82.014 | -7.14% | lower time |
| java | 1024 | 715.816 | 673.733 | -5.88% | lower time |
| c | 137 | 99.118 | 92.493 | -6.68% | lower time |
| c | 1024 | 955.304 | 871.185 | -8.81% | lower time |
| cpp | 137 | 92.340 | 87.175 | -5.59% | lower time |
| cpp | 1024 | 780.705 | 715.604 | -8.34% | lower time |

Python 1 MiB moves from 3.975 to 4.192 seconds (+5.45%, p=0.529). Its confidence intervals are wide; this is an inconclusive directional regression, not a demonstrated improvement. It requires quiet-host remeasurement. JavaScript 1 MiB can take the existing forest path or its GLR fallback; both produce the locked-C tree, and both runtime-counter tuples occur before and after. No counter-accounting change belongs to this patch.

All 412 pinned ledger rows across 206 languages remain identical. Generated tokens, nodes, and maximum stacks have identical before/after values or value sets in all 16 workloads. The locked-C test passes cold and two warm operations for every workload. Large Go tests pass repeated fresh parsing, three valid edits equal to fresh trees, root/error/stop checks, and zero-allocation no-edit reparses at both sizes. Focused raw-shape/transient tests and a Docker race check for concurrent parser-pool use pass.

The full R4 invariant sweep is not green at the baseline: 195 languages pass, four have tree mismatches, and seven exceed the two-minute per-language diagnostic timeout. After the hash change, 196 pass, the same four mismatches remain, and six time out. F# completes after timing out before. The unchanged mismatches are AWK, Elsa, JavaScript, and Twig; the remaining timeouts are Godot Resource, Haskell, Kotlin, Meson, Nickel, and PowerShell. No expectation or gate threshold changed. This does not certify the timed-out languages.

METRIC: fresh_java_137_ns | 88316297 -> 82014391.5 | raw-shape hash candidate, source SHA in receipt | GeneratedSource(java, 137 KiB)
METRIC: fresh_cpp_1024_ns | 780694612 -> 715557174.5 | raw-shape hash candidate, source SHA in receipt | GeneratedSource(cpp, 1 MiB)
METRIC: ledger_rows | 412 -> 412 | raw-shape hash candidate | both routes, 206 languages
METRIC: go_137_work_tokens_nodes_stacks | 58089/135534/2 -> 58089/135534/2 | raw-shape hash candidate | GeneratedSource(go, 137 KiB)

The [hash receipt](receipts/fresh-lazy-hash-2026-10-01.json) records medians, work tuples, source identity, and invariant outcomes. These results do not establish the owner’s 2.5x-C target. Paired locked-C ratios follow in the final campaign receipt.
