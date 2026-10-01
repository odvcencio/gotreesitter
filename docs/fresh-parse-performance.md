# Fresh legacy parse profiles, 2026-10-01

The largest verified fresh timing gain is at Go 1 MiB: 988.226 -> 905.886 ms/op (-8.33%). Arena retention cuts allocation bytes by 88.14%. The Go 137 KiB target remains unmet. Profiles across eight languages at both sizes rank node construction first, followed by lexing; JavaScript also spends heavily on forest maps and candidate vectors.

The baseline engine is `f9828512cc58f1bc77b856c2b3451e85266c38b1`. Benchmark definitions are at `52aee909bf0d267b2f18d4c6120c4e4a766ab7ae`. All eight languages use the existing deterministic `benchfixtures.GeneratedSource` shapes, at minimum sizes of 137 KiB and 1 MiB. Actual sources can extend beyond the requested boundary to finish a declaration. These shapes are an assumption for this campaign: they are reproducible, but they do not identify the owner’s earlier quiet-host fixture.

The measured operation calls `Parser.Parse`, validates acceptance and full root coverage, and calls `Tree.Release`. Source generation, grammar loading, parser construction, and one warm parse precede timing. Admission is explicitly legacy; existing forest defaults remain active. No incremental tree is supplied. Publication C timing uses the authenticated static native driver with locked C runtime `f5afe475deb7c0bae6407fb776c76824f717bb61`, locked grammar sources, and `-O2 -DNDEBUG`. Its `full_operation` axis includes `ts_parser_parse_string`, root coverage validation, and `ts_tree_delete`. Process launch, source loading, grammar loading, and parser construction remain outside the native clock. The existing `full` axis retains its original timing region. The callback-based C benchmark remains diagnostic only; its reader callbacks and different assertion flags make it unsuitable for native ratios. C benchmark B/op and allocs/op describe the Go launcher, not C allocation. Every process authenticates each fixture once by matching its static deep digest to the locked parity transport before timing.

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

## Unrestricted raw-shape deferral: rejected

The first hash candidate kept its lossless sidecar and the same bounded hash reservation, then computed fingerprints when a comparator requested them. The timing table below records that experiment. It was superseded after the deep-chain probe showed higher stack memory; these numbers do not describe the final implementation.

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

The full R4 invariant sweep is not green at the baseline: 195 languages pass, four have tree mismatches, and seven exceed the two-minute per-language diagnostic timeout. After the hash change, 196 pass, the same four mismatches remain, and six time out. F# completes after timing out before. The unchanged mismatches are AWK, JavaScript, Meson, and Twig; the remaining timeouts are Elsa, Godot Resource, Haskell, Kotlin, Nickel, and PowerShell. No expectation or gate threshold changed. This does not certify the timed-out languages.

METRIC: fresh_java_137_ns | 88316297 -> 82014391.5 | raw-shape hash candidate, source SHA in receipt | GeneratedSource(java, 137 KiB)
METRIC: fresh_cpp_1024_ns | 780705128 -> 715604161.5 | raw-shape hash candidate, source SHA in receipt | GeneratedSource(cpp, 1 MiB)
METRIC: ledger_rows | 412 -> 412 | raw-shape hash candidate | both routes, 206 languages
METRIC: go_137_work_tokens_nodes_stacks | 58089/135534/2 -> 58089/135534/2 | raw-shape hash candidate | GeneratedSource(go, 137 KiB)

The [hash receipt](receipts/fresh-lazy-hash-2026-10-01.json) records medians, work tuples, source identity, and invariant outcomes. These results do not establish the owner’s 2.5x-C target. Paired locked-C ratios follow in the final campaign receipt.

## Retain the transient-parent node arena

Full parses using the existing transient-parent mode now bound the primary node reservation by the existing retention limit. Ordinary overflow growth provides the remaining nodes. This lets an arena that already fits the total pool limit survive release instead of dropping a primary slab that exceeds its own limit. No pool limit, parse budget, node limit, or route changes. Fixed API/environment budgets and internally capped budgets keep the old reservation and charging behavior.

Go 1 MiB moves from 981.686 to 903.763 ms/op (-7.94%, p<0.001, 20 paired seeds). Bytes fall from 159,129,104 to 18,875,608 per operation; allocations fall from 81 to 14. Tokens/nodes/maximum stacks remain 425742/993391/2. Go 137 KiB and Java have no established timing change. Java 1 MiB keeps 107 allocations per operation.

Five unprofiled Go 1 MiB processes give median peak RSS 413,452 -> 261,832 KiB, and maximum 414,376 -> 265,972 KiB. The hash-only median is 414,152 KiB. Fixed-operation RSS runs do not supply timing evidence.

METRIC: fresh_go_1024_ns | 981685622.5 -> 903762804.0 | transient arena candidate, source SHAs in receipt | GeneratedSource(go, 1 MiB)
METRIC: fresh_go_1024_bytes | 159129104 -> 18875608 | transient arena candidate | complete fresh parse and release
METRIC: fresh_go_1024_allocs | 81 -> 14 | transient arena candidate | complete fresh parse and release
METRIC: go_1024_work_tokens_nodes_stacks | 425742/993391/2 -> 425742/993391/2 | transient arena candidate | GeneratedSource(go, 1 MiB)

The [arena receipt](receipts/fresh-transient-arena-2026-10-01.json) records every median and RSS sample.

## Pool forest candidate vectors

Forest alternatives now store temporary candidate vectors in disjoint slices from bounded chunks. The existing retention limit bounds cached pointers; allocations above it still use ordinary backing arrays. Release clears retained references. Candidate order, deduplication, and selection are unchanged. The generic storage starts in `internal/slicearena`.

JavaScript 137 KiB: 2,026,328 -> 76,896 B/op; 103,297.0 -> 30.0 allocations/op. Median time changes +1.50% with no established difference. This is an allocation improvement, not a timing win.

JavaScript 1024 KiB: 268,173,692 -> 263,037,548 B/op; 752,449.5 -> 361,201.0 allocations/op. Median time changes +4.02% with no established difference. This is an allocation improvement, not a timing win.

The 1 MiB maximum RSS probe moves 880788 -> 873808 KiB. It remains above the existing RSS floor, as it was before. No threshold or exemption changes. The pinned ledger and generated-work value sets remain identical. Focused tests check vector isolation, unchanged candidate order, deduplicated updates, bounded retention, zero-allocation warm storage reuse, and cleared references after release.

METRIC: fresh_javascript_137_allocs | 103297.0 -> 30.0 | bounded forest-vector candidate, source SHAs in receipt | GeneratedSource(javascript, 137 KiB)
METRIC: fresh_javascript_1024_allocs | 752449.5 -> 361201.0 | bounded forest-vector candidate, source SHAs in receipt | GeneratedSource(javascript, 1024 KiB)

The [forest-vector receipt](receipts/fresh-forest-vectors-2026-10-01.json) records the comparison and its limits.

The combined final engine passes all 16 cold/two-warm fresh-C cases, the focused Docker tests and race checks, and both large Go edit/no-edit invariant cases. Its R4 sweep has 197 passing languages, the same four failing tree digests, and five diagnostic timeouts (Elsa, Haskell, Kotlin, Nickel, PowerShell). F# and Godot Resource complete after timing out in the baseline. These outcomes preserve the baseline failures; they do not certify the timed-out grammars.

## Native complete-operation comparison

Use `BenchmarkFreshGeneratedStaticC` with `treesitter_c_parity treesitter_c_perfscan` to reproduce native C timings. The generated size order follows the same explicit shuffle seed as Go. `GTS_FRESH_STATIC_C_IDENTITY_DIR` records authenticated runtime/compiler/grammar/artifact identities and fixture/deep hashes. The new native axis and the original full axis pass the focused protocol test. Static linkage rejection, cache tampering, build-input mutation, compiler flags, timeout classification, and failure-vocabulary tests also pass in Docker.

## Bound deferred hash depth

The final hash candidate records pending depth in the alignment padding that the original cache entry already had. At depth 64 it computes a fingerprint checkpoint. Missing metadata takes the original eager reconstruction path. On architectures without padding, capture retains eager hashing. Cache-entry sizes, hash width, cache capacity, budget charging, and exact comparison remain unchanged.

An isolated synthetic chain of one million captured shapes gives the same fingerprint `8277e92b948ba9c0` before, with unrestricted deferral, and with checkpoints. Peak RSS is 148,532 -> 566,972 -> 148,480 KiB. This probe measures internal hash storage and stack use, not a complete language parse or a timing comparison. The unrestricted candidate was rejected for that memory growth.

The checkpoint candidate passes the 16 generated cold/two-warm fresh-C cases, all 206 curated fresh-C cases, the selected real-file witnesses, focused parser-budget tests, and Docker race checks. Its R4 sweep has 197 passes, four identical baseline mismatch digests, and five existing diagnostic timeouts. All 412 ledger rows remain unchanged. Layout and depth-policy tests pass on amd64 and 386; the original cache-entry size is preserved on both.

METRIC: deep_hash_rss_kib | 148532 -> 148480 | checkpoint candidate, source hashes in final receipt | one million captured shapes; unrestricted rejected at566972KiB
METRIC: ledger_rows | 412 -> 412 | checkpoint candidate | both routes,206 languages

The earlier unrestricted timing table supplies no timing claim for the checkpoint implementation.

## Final checkpoint measurements

The table uses completed 20-seed paired before/after Go runs. Native C reference runs separately completed 20 seeds for all 16 fixtures. The ratios below divide those medians: they are diagnostic, because this final comparison did not complete the required paired Go-C-C-Go protocol on a quiet host. The generated fixtures differ from the owner’s unidentified quiet-host fixture. Differences below 5% are provisional; Java and TypeScript’s small directional regressions remain recorded.

| Language | KiB | Go before ms | Go after ms | Change | Native C ms | Diagnostic Go/C before → after | B/op before → after | allocs/op before → after |
| --- | ---: | ---: | ---: | ---: | ---: | --- | --- | --- |
| go | 137 | 120.034 | 119.343 | -0.58% | 30.059 | 3.993 → 3.970 | 19707 → 17066 | 10 → 9.5 |
| go | 1024 | 988.226 | 905.886 | -8.33% | 246.112 | 4.015 → 3.681 | 1.59129e+08 → 1.88756e+07 | 81 → 14 |
| java | 137 | 85.975 | 87.491 | +1.76% | 28.191 | 3.050 → 3.103 | 48472 → 48472 | 14 → 14 |
| java | 1024 | 703.970 | 714.765 | +1.53% | 238.288 | 2.954 → 3.000 | 1.72555e+08 → 1.72555e+08 | 107 → 107 |
| javascript | 137 | 168.153 | 166.051 | -1.25% | 27.482 | 6.119 → 6.042 | 2.02633e+06 → 76896 | 103297 → 30 |
| javascript | 1024 | 2351.491 | 2342.835 | -0.37% | 231.532 | 10.156 → 10.119 | 2.68137e+08 → 2.62853e+08 | 752444 → 361177 |
| typescript | 137 | 95.445 | 96.927 | +1.55% | 28.490 | 3.350 → 3.402 | 33888 → 33888 | 29 → 29 |
| typescript | 1024 | 809.670 | 821.420 | +1.45% | 240.316 | 3.369 → 3.418 | 1.63113e+08 → 1.63113e+08 | 113 → 113 |
| python | 137 | pending | pending | pending | 45.206 | pending | pending | pending |
| python | 1024 | pending | pending | pending | 353.184 | pending | pending | pending |
| rust | 137 | pending | pending | pending | 28.196 | pending | pending | pending |
| rust | 1024 | pending | pending | pending | 237.510 | pending | pending | pending |
| c | 137 | pending | pending | pending | 27.793 | pending | pending | pending |
| c | 1024 | pending | pending | pending | 230.145 | pending | pending | pending |
| cpp | 137 | pending | pending | pending | 33.019 | pending | pending | pending |
| cpp | 1024 | pending | pending | pending | 270.075 | pending | pending | pending |

The Go 1 MiB result has p<0.001. Its tokens/nodes/maximum stacks stay 425742/993391/2. Go 137 KiB has p=0.121 and changes less than 1%; it establishes no timing win. The final allocation result is 159,129,104 -> 18,875,608 B/op and 81 -> 14 allocations/op at Go 1 MiB.

An identical unprofiled four-operation workload supplies final peak RSS below. These single process samples are diagnostic; Go’s arena-only five-process comparison confirms the reduction (median 413,452 -> 261,832 KiB). Java’s baseline single sample was an outlier: 20 baseline processes give median 377,588 KiB, while the final checkpoint probe is 379,228 KiB. The earlier unrestricted combined candidate’s 20-process Java median was 378,888 KiB. No memory threshold changes. JavaScript’s final single-process RSS rises 9.58%, which requires repeat measurement; the existing JavaScript, Python, Rust and C++ RSS floor failures remain.

| Language | KiB | Before peak RSS KiB | Final peak RSS KiB |
| --- | ---: | ---: | ---: |
| go | 137 | 56036 | 56176 |
| go | 1024 | 394908 | 264900 |
| java | 137 | 51584 | 52576 |
| java | 1024 | 324004 | 379228 |
| javascript | 137 | 78580 | 74548 |
| javascript | 1024 | 880788 | 965168 |
| typescript | 137 | 49292 | 48992 |
| typescript | 1024 | 381924 | 397884 |
| python | 137 | 159764 | 174376 |
| python | 1024 | 1626836 | 1600536 |
| rust | 137 | 58100 | 59012 |
| rust | 1024 | 632920 | 443196 |
| c | 137 | 62980 | 63520 |
| c | 1024 | 382396 | 331792 |
| cpp | 137 | 83660 | 83360 |
| cpp | 1024 | 436164 | 436188 |

The bounded checkpoint candidate’s primary-trio comparison increased the one-byte edit benchmark from 168.3 to 179.3 µs (+6.54%). A second candidate restored eager hashing in incremental arenas, but still increased edits by 5.83% and no-edit time by 12.11%; it was rejected and reverted. The bounded checkpoint engine remains. Its edit slowdown is a measured directional regression traded for the Go 1 MiB fresh time, allocation and RSS gains; quiet-host review remains required before integration. No exception or threshold change was made.

| Primary benchmark | Before ns/op | After ns/op | Change |
| --- | ---: | ---: | ---: |
| GoParseIncrementalSingleByteEditDFA | 168256 | 179260 | +6.54% |
| GoParseIncrementalNoEditDFA | 8.6125 | 8.612 | -0.01% |
| GoParseFullDFA | 8.72291e+06 | 8.5895e+06 | -1.53% |

The checkpoint primary run uses 20 paired alternating seeds on CPU5. No-edit reparse remains zero bytes and zero allocations. The rejected incremental-only run uses CPU7. Both used the standard script and passed their correctness and deterministic checks before timing. The final engine restores the checkpoint source used by the Go and Java measurements. JavaScript and TypeScript timing runs spanned the incremental-only trial; their full-parse policy remained the same, but compilation-layout effects remain possible, so their timing deltas are provisional.

METRIC: final_fresh_go_1024_ns | 988225968.5 -> 905886234.5 | checkpoint candidate; exact engine source hashes in receipt | GeneratedSource(go,1 MiB),20 paired seeds
METRIC: final_fresh_go_1024_bytes | 159129104 -> 18875608 | checkpoint candidate | complete fresh parse and release
METRIC: final_fresh_go_1024_allocs | 81 -> 14 | checkpoint candidate | complete fresh parse and release
METRIC: final_go_1024_work_tokens_nodes_stacks | 425742/993391/2 -> 425742/993391/2 | checkpoint candidate | GeneratedSource(go,1 MiB)

The [final receipt](receipts/fresh-final-2026-10-01.json) records all samples, authenticated native identities, source hashes, counter tuples, RSS, primary-trio results and rejected candidates. Profiles cover all requested eight languages and both sizes. Final checkpoint 20-seed Go comparisons remain pending for python, rust, c, cpp. Earlier rejected hash-candidate timings do not fill these gaps. Full fresh-C real-corpus certification, a fully green R4 gate and quiet paired native C remeasurement remain required. No pin refresh, approval item, graduation, exemption or threshold change accompanies this campaign.
