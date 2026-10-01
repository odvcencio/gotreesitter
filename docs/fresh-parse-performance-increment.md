# Fresh arena retention after the merged reuse model

The Go 1 MiB complete fresh operation improves 7.66% against merged #1402: 1.878 -> 1.735 s/op on the shared VM (20 paired alternating seeds, p<0.001). Allocation bytes fall 180,107,856 -> 46,137,560 (-74.38%), and allocations fall 97 -> 32 (-67.01%). The Go 137 KiB result is unchanged (228.1 -> 230.4 ms, p=0.640). The 2.5x-C target remains unmet.

The baseline is `9148a96db0dcf95594d72671f48862d72bfee343`; the measured engine is `2c9e2ea57`. Runs use `scripts/run_randomized_benchmarks.sh`, Go 1.26.8, `GOWORK=off`, `GOMAXPROCS=1`, CPU2, one process per explicit shuffle seed, count1, 750ms and benchmem. The same generated source sizes are explicitly shuffled. These are generated shapes, not the owner's unidentified quiet-host fixture. Other VM lanes saturate the CPUs, so close timing deltas remain provisional. Native measurements use the locked, authenticated C driver. The current outer before/after loop interleaves baseline cycles, so those ratios remain diagnostic; contiguous Go-C-C-Go cycles for both roles remain required.

Full transient-parent parses bound the primary node reservation by the existing retention limit. Ordinary overflow provides the remaining nodes. Fixed API, environment and internally capped budgets retain their old reservation and charging behavior. Merged #1402 adds 12,179,644 bytes of reuse metadata to the 1 MiB fixture; with the first candidate, unused raw-shape scratch then pushes the arena above the existing 128 MiB pool ceiling. After clearing every payload's raw reference, the final candidate releases raw-only overflow when necessary and keeps its first warm slabs. Useful node storage and certificates can then survive tree release under the same ceiling. Oversized node storage still fails the ceiling. No parse budget, retention ceiling, threshold or exemption changes.

Forest alternative candidate vectors use disjoint slices from bounded chunks, with ordinary allocation beyond the existing bound. Release clears references. Candidate order, deduplication and selection remain the same. The candidate policies live in `internal/slicearena` and `internal/slabretention`.

All deferred-hash candidates were rejected. The bounded candidate did not improve Go 137 KiB and added 6.54% primary edit time. Restricting deferral to full arenas still added 5.83% edit time and 12.11% no-edit time. The unrestricted version increased a one-million-shape synthetic chain's RSS from 148532 to 566972 KiB; its checkpoint replacement preserved the hash at148480KiB but did not justify its timing tradeoff. Eager raw-shape hashing now matches main.

After rebase and the final retention policy, focused Docker budget/raw-shape/arena tests, large Go repeated-fresh and three edit-equals-fresh steps, zero-allocation no-edit checks, all eight target R4 invariant checks and the 412-row counter ledger pass. New regressions verify warm prefix reuse, cleared discarded headers and continued rejection of oversized node storage. All 16 generated cold/two-warm C comparisons pass. All 206 curated fresh-C checks pass. The full R4 audit and race checks continue before readiness. Baseline Elsa exceeds15minutes in the existing dependency walk; the audit records the failure and continues to the other languages without waiving that gate.

CI's L0 failure is fixed by consolidating the new benchmark and invariant functions into existing files. Their canonical Go AST bodies are unchanged. The existing guard passes 756/756 root files and all 12 API tag sets; R6 retains 454 language-name uses and 101 environment reads. No test or census pin changes. CI also requires the new `internal/slicearena` tests in its support execution lane; adding that package preserves coverage. Its package-plan test and execution census pass in Docker.

METRIC: fresh_go_1024_ns | 1878364075.5 -> 1734527778 | 2c9e2ea57 vs9148a96db | GeneratedSource(go,1 MiB),20 paired alternating seeds
METRIC: fresh_go_1024_bytes | 180107856 -> 46137560 | 2c9e2ea57 vs9148a96db | complete fresh parse and tree release
METRIC: fresh_go_1024_allocs | 97 -> 32 | 2c9e2ea57 vs9148a96db | complete fresh parse and tree release
METRIC: go_137_work_tokens_nodes_stacks | 58089/135534/2 -> 58089/135534/2 | 2c9e2ea57 vs9148a96db | GeneratedSource(go,137 KiB)
METRIC: go_1024_work_tokens_nodes_stacks | 425742/993391/2 -> 425742/993391/2 | 2c9e2ea57 vs9148a96db | GeneratedSource(go,1 MiB)
METRIC: counter_ledger_rows | 412 -> 412 | 2c9e2ea57 vs9148a96db | 206 grammars,both routes

The earlier [profile campaign](fresh-parse-performance.md) covers all eight requested languages at both sizes and records rejected candidates. Its original-engine timings do not describe the merged reuse model. Final native ratios, RSS, all-language counter value sets and audit outcomes follow in the resumed receipt.

The resumed [receipt](receipts/fresh-retention-2026-10-01.json) authenticates the measured sources and records each completed seed, counter value sets and audit outcomes. The first primary-trio sweep reports edits 328.0 -> 326.3 microseconds (inconclusive), full parsing 15.99 -> 15.93 milliseconds (inconclusive), and no-edit 15.77 -> 17.00 nanoseconds (+7.77%). Baseline and candidate benchmark registration order differed after the L0 consolidation, so a second sweep with identical benchmark source files is in progress. Both no-edit runs allocate zero bytes and zero objects.

An identical four-operation RSS probe reports Go 1 MiB 363296 -> 310604 KiB (-14.51%), Java 335292 -> 335524 KiB, JavaScript 966348 -> 927488 KiB, and TypeScript 411076 -> 607840 KiB (+47.87%). These are single-process observations. The TypeScript increase blocks readiness if confirmed; 20 alternating process pairs are being collected. Existing RSS floor failures remain recorded, without exemptions or threshold changes.

Completed 20-seed host comparisons against merged main:

| Language | KiB | Before ms | After ms | Change | Before/after bytes | Before/after allocs |
|---|---:|---:|---:|---:|---:|---:|
| go | 137 | 228.052 | 230.363 | +1.01% | 1246 / 1240 | 8 / 8 |
| go | 1024 | 1878.364 | 1734.528 | -7.66% | 1.80108e+08 / 4.61376e+07 | 97 / 32 |
| java | 137 | 166.540 | 172.780 | +3.75% | 48472 / 48472 | 14 / 14 |
| java | 1024 | 1381.947 | 1357.669 | -1.76% | 1.85145e+08 / 1.85145e+08 | 122 / 122 |
| javascript | 137 | 299.965 | 295.315 | -1.55% | 2.02633e+06 / 76896 | 103297 / 30 |
| javascript | 1024 | 4055.112 | 4131.503 | +1.88% | 2.74414e+08 / 2.69278e+08 | 752475 / 361220 |
| typescript | 137 | 169.817 | 169.253 | -0.33% | 33888 / 33888 | 29 / 29 |
| typescript | 1024 | 1381.321 | 1490.732 | +7.92% | 1.63113e+08 / 1.42405e+08 | 113 / 56 |
