# Fresh parsing: reusable arenas and forest vectors

Go 1 MiB complete fresh parse/validate/release time falls 9.89%, from 1.959 to 1.765 seconds (20 paired randomized seeds, p<0.001). Allocated bytes fall 180,107,856 -> 46,137,560 (-74.38%), and allocations fall 97 -> 32 (-67.01%). Go 137 KiB remains statistically unchanged: 234.8 -> 230.2 ms, p=0.341. The 2.5x-C target remains unproven.

The baseline is merged #1402, `9148a96db0dcf95594d72671f48862d72bfee343`; the retained fresh engine is `a0b0d5ee7`. Runs use `scripts/run_randomized_benchmarks.sh`, Go 1.26.8, GOWORK=off, GOMAXPROCS=1, CPU0, count1, 750ms and benchmem. Each process uses one explicit shuffle seed; baseline/head order and generated sizes alternate. Both revisions use identical benchmark source files. Existing GeneratedSource shapes substitute for the unidentified quiet-host fixture. Other VM lanes saturate the CPUs; differences below 5% remain provisional.

Transient-parent full parses bound their primary node reservation within the existing 64 MiB retention limit; ordinary overflow supplies remaining nodes. Fixed API, environment and internally capped budgets retain their original reservation and charging behavior. After all tree payload raw references are cleared, raw-only overflow may be released when it would evict an otherwise reusable full arena. This happens only when the primary storage can survive reset. The existing 128 MiB total ceiling and eviction guard remain unchanged; oversized node storage still fails them. Bounded forest candidate vectors live in disjoint chunks, retain candidate order and selection, and clear references at release. New storage policies live in internal/slicearena and internal/slabretention.

All 16 generated cold/two-warm locked-C comparisons and all 206 curated fresh-C grammar cases pass. Docker budget/raw-shape/arena tests, large Go repeated-fresh and three valid edit-equals-fresh steps, zero-allocation no-edit checks, all eight target R4 checks, focused root/vector race tests, and the 412-row counter ledger pass. Root census and API checks remain 756/756 files and 12 tag sets; R6 remains 454 language-name uses and 101 environment reads. The vector package is registered in the existing CI support race lane. No tests, pins, budgets, thresholds, exemptions, defaults or graduation lists change.

The primary trio is neutral: full parse 17.146 -> 17.147 ms, one-byte edit 332.075 -> 329.564 microseconds (p=0.192), and no-edit 16.440 -> 16.565 ns (+0.76%, below 5%). No-edit remains zero bytes and zero allocations. Earlier no-edit sweeps measured +7.77% and +5.09%; the final matched-source sweep is retained alongside those observations rather than erasing them.

TypeScript 1 MiB is restored to baseline: 1.517 -> 1.520 s (p=0.904), with identical allocation bytes and counts. Twenty identical four-operation process pairs give median peak RSS 342812 -> 342422 KiB (-0.11%). The broad raw-scratch policy was rejected: its 20-seed TypeScript time rose 7.92%, and its 20-pair RSS median rose 350122 -> 607934 KiB (+73.64%). Its ordinary primary storage was replaced during reset while its retained sidecars kept memory live. The primary-storage qualification avoids this retention. Eager raw-shape hashes match main; the deferred-hash experiments were also rejected for memory or edit regressions.

The full R4 audit identified duplicate traversal in edit dependency proofs: normal error-node children were checked once by nodeEndsBeforeEditDependency and again by its stack-entry caller, doubling the work at each level. A separate candidate removes the second walk only for normal nodes, preserving the missing-node receipt path. A 128-level regression and focused missing-dependency tests pass. Godot Resource, Elsa, Haskell, Kotlin, Nickel and PowerShell now pass all 72 edit steps with zero no-edit allocations in 1.03, 8.04, 2.10, 13.31, 1.32 and 30.44 seconds respectively. Baseline reruns repeatedly hit 15-minute timeouts. The full 206-language audit and C edit checks for this additional fix are in progress; no gate failure is waived.

The locked real-corpus pilot checks each language's five largest files at or below 1 MiB, without mismatch bypasses. Go and Java pass before/after; JavaScript, TypeScript, Python, Rust, C and C++ retain their baseline first mismatches. This pilot is not full real-corpus certification.

Native complete-operation C measurements use the authenticated locked runtime and grammar sources, O2 DNDEBUG and fully static linkage. C's clock includes root validation and tree deletion, and excludes launch, input loading and parser setup. Contiguous Go-C-C-Go cycles run for each role; role order alternates over ten cycles, giving 20 samples. The final sweep uses Go 1.25.14 on CPU2. Earlier incomplete CPU1 cycles and the interleaved-role experiment are archived as diagnostics. Native launcher B/op and allocs/op do not measure C allocation.

Completed 20-seed host measurements:

| Language | KiB | Before ms | After ms | Change | Before/after bytes | Before/after allocs |
|---|---:|---:|---:|---:|---:|---:|
| go | 137 | 234.840 | 230.171 | -1.99% | 5856 / 1240 | 8.5 / 8 |
| go | 1024 | 1959.015 | 1765.177 | -9.89% | 1.80108e+08 / 4.61376e+07 | 97 / 32 |
| java | 137 | 177.278 | 178.124 | +0.48% | 48472 / 48472 | 14 / 14 |
| java | 1024 | 1439.266 | 1426.537 | -0.88% | 1.85145e+08 / 1.85145e+08 | 122 / 122 |
| javascript | 137 | 314.565 | 308.719 | -1.86% | 2.02633e+06 / 76896 | 103297 / 30 |
| javascript | 1024 | 4296.310 | 4218.138 | -1.82% | 2.74321e+08 / 2.69222e+08 | 752471 / 361216 |
| typescript | 137 | 189.566 | 188.780 | -0.41% | 33888 / 33888 | 29 / 29 |
| typescript | 1024 | 1517.415 | 1520.161 | +0.18% | 1.63113e+08 / 1.63113e+08 | 113 / 113 |
| python | 137 | 591.095 | 586.977 | -0.70% | 1.67959e+07 / 1.67959e+07 | 18 / 18 |
| python | 1024 | 5913.021 | 6104.312 | +3.24% | 8.6121e+08 / 8.6121e+08 | 1621 / 1621 |
| rust | 137 | 167.947 | 171.706 | +2.24% | 1256 / 1256 | 9 / 9 |
| rust | 1024 | 1468.987 | 1473.940 | +0.34% | 2.23472e+08 / 2.23472e+08 | 92 / 92 |
| c | 137 | 181.834 | 183.740 | +1.05% | 132624 / 132624 | 1541 / 1541 |
| c | 1024 | 1606.887 | 1608.116 | +0.08% | 1.98565e+08 / 1.98565e+08 | 11290 / 11290 |
| cpp | 137 | 172.945 | 170.173 | -1.60% | 23024 / 23024 | 10 / 10 |
| cpp | 1024 | 1362.178 | 1393.720 | +2.32% | 1.72139e+08 / 1.72139e+08 | 95 / 95 |

Both the original and retained-engine CPU/alloc profile campaigns cover all eight requested languages and both sizes. Reduction/node work remains the largest common cost, followed by lexing and stack/forest work. Parse-table lookup is smaller. UTF costs are near zero on these ASCII fixtures; Unicode performance is not established. Allocation profiles include fixture/grammar setup outside the benchmark clock, which dominates nearly allocation-free small warm parses. Profiled fixed-order timings are not comparison evidence.

METRIC: fresh_go_1024_ns | 1959014824.5 -> 1765176567.5 | a0b0d5ee7 vs 9148a96db | complete operation,20 paired alternating seeds
METRIC: fresh_go_1024_bytes | 180107856 -> 46137560 | a0b0d5ee7 vs 9148a96db | complete operation
METRIC: fresh_go_1024_allocs | 97 -> 32 | a0b0d5ee7 vs 9148a96db | complete operation
METRIC: go_137_work_tokens_nodes_stacks | 58089/135534/2 -> 58089/135534/2 | a0b0d5ee7 vs 9148a96db | GeneratedSource(go,137 KiB)
METRIC: go_1024_work_tokens_nodes_stacks | 425742/993391/2 -> 425742/993391/2 | a0b0d5ee7 vs 9148a96db | GeneratedSource(go,1 MiB)
METRIC: counter_ledger_rows | 412 -> 412 | a0b0d5ee7 vs 9148a96db | 206 grammars,both routes

The [retained-engine receipt](receipts/fresh-primary-retention-2026-10-01.json) records every completed sample, counter value set, source hash, native identity, RSS and audit outcome. The [profile receipt](receipts/fresh-retention-profiles-2026-10-01.json) records both profile hashes and approximate exclusive flat categories for all 16 workloads. The [rejected broad-policy receipt](receipts/fresh-retention-2026-10-01.json) preserves all eight language sweeps. The [original campaign](fresh-parse-performance.md) records the earlier baseline and rejected candidates. Remaining native, RSS and invariant results will be added without filling incomplete sweeps from older candidates.
