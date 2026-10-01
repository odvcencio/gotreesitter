# Compact incremental edit receipt

Compact serves 69 of the 72 generated language/size/edit-mode cells (138 of 144 edit directions), up from 18 cells (36 directions). Python at 1 MiB remains outside compact’s existing node budget. The served clean cells match compact fresh parsing and locked-C fresh parsing, including fields, flags, byte ranges and points. The Go parser-cliff clean edit sessions now use compact and match C; the baseline fell back to legacy and returned a different tree.

No grammar is proposed for graduation. All eight languages exceed the 400-byte-per-source-byte RSS ceiling, syntax-breaking fresh-C recovery differences remain, and some languages lose to legacy on fresh parsing or one-byte edits. Admission, defaults, node budgets, cliff limits and gate thresholds are unchanged. The companion [machine-readable receipt](compact-incremental-edit-receipt.json) contains source and grammar identities, audit reasons, deterministic counters, resource observations and every timing sample.

The baseline is `6d8692210` on `incr/c-reuse-model`, because [#1402](https://github.com/odvcencio/gotreesitter/pull/1402) was still open at receipt time. Most timing and counter measurements authenticate the engine files in `de23264ae`. The recovered dependency walk fix is `a82f6fe84`; it has separate paired timing evidence. C++’s final scanner policy is `8f46d4313`. The last allocation correction is `51d8281fc`; the timing tables precede that correction and do not establish the final timing/RSS ratchets. No pull request was opened; the integration lead receives this branch.

## Implementation and proof

The compact adapter records lexer read dependencies, including failed probes and rollback, and seals them onto native subtree summaries. Reuse rejects changed bytes, unknown read history, fragile or recovered edges, missing-node dependencies, incompatible lexer modes and a failed fresh first-leaf lexical check. Complete scanner boundary states are copied from the core reduction history, including hidden zero-width scanner changes, rather than inferred from projected public children. The scanner state before lookahead must match; borrowing restores the exact end state. A trusted empty checkpoint is distinct from an absent checkpoint.

Python, Rust and C++ use explicit checkpoint proofs. TypeScript’s scanner has a stateless certificate. Scanners that inspect bytes before their scan start conservatively invalidate the unknown prefix history. Historical GLR forks can reuse only after the live survivor has a complete, clean, equivalent predecessor path. Borrowed nodes keep their arena ownership and bounds. Unknown history and recovery retain a fresh proof.

Read storage is bounded by the existing parser budget and retained-memory limits. The recovered missing-node dependency walk now visits error descendants once: a 48-level recovered chain previously expanded recursively twice at each level. C++ keeps its legacy scanner reuse opt-out; compact’s complete checkpoint proof is independent of that opt-out. Empty raw-string state uses the scanner’s original serialization.

## Admission and correctness

Each row covers 32 KiB, 137 KiB and 1 MiB; one-byte replacement, 100-byte comment insertion/removal and a 4 KiB comment splice/removal. Each served cell runs four alternating edits and three allocation checks for a reparse without edits. The admission audit records both edit directions. Generated sources are pinned deterministic reconstructions, rather than unpublished owner fixtures.

| Language | Edit directions served before → after | Clean served cells | Remaining decline |
| --- | ---: | ---: | --- |
| Go | 0/18 → 18/18 | 9/9 | None on these clean fixtures |
| JavaScript | 18/18 → 18/18 | 9/9 | None on these clean fixtures |
| TypeScript | 0/18 → 18/18 | 9/9 | None on these clean fixtures |
| Python | 0/18 → 12/18 | 6/9 | 1 MiB node budget, three edit modes |
| Rust | 0/18 → 18/18 | 9/9 | None on these clean fixtures |
| Java | 18/18 → 18/18 | 9/9 | None on these clean fixtures |
| C | 0/18 → 18/18 | 9/9 | None on these clean fixtures |
| C++ | 0/18 → 18/18 | 9/9 | None on these clean fixtures |

The served clean matrix passes 276 alternating edit steps against both fresh implementations. Stateful scanner witnesses add 36 strict steps across Python triple-quoted strings and indentation, Rust documentation comments and raw delimiters, and C++ raw delimiters. Python’s former prefix-abstention witness adds eight strict steps at 20 KiB and 137 KiB. The Go parser-cliff fixture adds 12 strict clean edit steps. ERROR-root flags, coverage or an explanatory stop, and zero-allocation no-edit reparses are checked.

The Python 1 MiB positive gate fails without changing its limit: at 262,144 core nodes and byte 224,764 of 1,048,617, projected memory is 1,223,011 units, above the existing 1,048,576 limit. The same clean source declines before this work. C accepts it; this is an outstanding compact capacity problem.

The fixed 72-step syntax-breaking sessions complete for all eight languages. Every step equals compact fresh parsing (576 steps). They fail the separate fresh-C gate: Go 72, JavaScript 72, TypeScript 69, Python 72, Rust 72, Java 69, C 69 and C++ 1 fresh-C discrepancies. The first discrepancy for each language was reproduced on the baseline. Later steps were checked for D8 after each C discrepancy; the test still reports failure. The Go parser-cliff 72-step recovery session also completes with D8 equality and retains fresh-C recovery differences. These are not passing C-parity receipts.

The 206-grammar, 48-step invariant sweep passes for all legacy grammars and 204 compact grammars. Compact TOML and Doxygen fail; both failures reproduce on the baseline. Every grammar runs in its own process. Focused root, internal core and read-history unit gates pass. Focused container race gates pass, including the recovered dependency fix.

C# was checked as an additional unresolved case. Its 32 KiB clean fixture still declines fresh compact parsing because a converged reduction no-action drop lacks proof that one non-blended survivor covers the alternatives. The fallback matches C. This change does not establish a C# edit route.

## Deterministic work before timing

These are complete requested-route table lookups for the 137 KiB one-byte edit, including declined compact work, fallback and verification. They are not just selected-pass counts. Compact’s direct aggregate shift/reduction and lexer-front-door hooks are incomplete; their zero values mean unavailable, not zero work. Full-parse lookup counts stay unchanged for each language.

| Language | Whole lookups before → after | Reused subtrees before → after | Reused bytes before → after |
| --- | ---: | ---: | ---: |
| Go | 359,605 → 23,018 | 1,770 → 1,765 | 136,788 → 136,811 |
| JavaScript | 29,751 → 29,593 | 1,638 → 1,642 | 137,016 → 137,059 |
| TypeScript | 261,143 → 23,213 | 1,303 → 1,288 | 137,694 → 137,700 |
| Python | 859,252 → 16,290 | 0 → 2,010 | 0 → 134,282 |
| Rust | 538,800 → 27,404 | 21,238 → 1,519 | 39,849 → 137,301 |
| Java | 38,220 → 38,114 | 2,110 → 2,114 | 138,153 → 138,185 |
| C | 282,580 → 48,656 | 1,527 → 3,035 | 137,219 → 129,625 |
| C++ | 264,812 → 20,650 | 0 → 2,573 | 0 → 137,704 |

C reuses 137,219 → 129,625 bytes (-5.53%), beyond the 2% ledger tolerance, while whole lookups fall 82.8%. Its selected rebuild grows from 1,585 → 9,131 nodes and 43 → 4,566 tokens. This remains a ledger blocker; no threshold or ledger row was reset. Rust borrows fewer, larger subtrees: 21,238 → 1,519 (-92.85%), while reused bytes rise from 39,849 → 137,301 (+244.55%). That granularity change is also a guarded reuse-count decrease and needs owner review before treating the ledger as green. Go subtree reuse falls 0.28%, TypeScript 1.15%; bytes reused rise in both cases. All edit modes and both directions are retained in the JSON receipt.

An intermediate candidate repeated first-leaf action lookup work: JavaScript whole lookups rose 29,751 → 31,235 and Java 38,220 → 40,228. Passing the scheduler’s already-resolved row to candidate checks produces 29,593 and 38,114. The counter increase candidate was rejected.

## Timings and graduation matrix

137 KiB rows use 20 randomized seeds for seven languages and 20 completed seeds for the final C++ policy, 750 ms per benchmark, one process per seed, `GOMAXPROCS=1`, `-count=1` and `-benchmem`. Go timings include `Tree.Edit`, incremental parsing, fallback where needed and release of the preceding tree. C uses native batches of edit, parse and release with pre-created input buffers, avoiding Go wrappers and per-read source callbacks. C allocation metrics count native request bytes and reallocations. This protocol differs from #1402’s C-wrapper timings; its absolute Go/C ratios cannot be compared directly.

C and legacy timing controls drift substantially across campaign windows on the shared VM. For example, unchanged Go C one-byte timing moves from 0.814 → 1.932 ms. Absolute before/after timing ratchets are therefore unproven. Same-campaign compact/legacy and paired per-seed Go/C ratios show the relative ranking. Baseline and after samples, bytes/op and allocations/op are all published, including unfavorable changes.

| Language | Operation | Compact ms | Legacy ms | C ms | Compact/C | Legacy/C |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| Go | fresh | 464.61 | 348.58 | 103.16 | 5.53x | 3.45x |
| Go | byte | 64.71 | 36.91 | 1.93 | 32.79x | 18.93x |
| Go | edit100 | 84.58 | 384.67 | 2.00 | 36.14x | 191.87x |
| Go | splice | 70.77 | 387.93 | 2.08 | 33.01x | 176.08x |
| JavaScript | fresh | 289.61 | 299.59 | 61.35 | 4.67x | 5.12x |
| JavaScript | byte | 48.22 | 307.68 | 1.99 | 23.65x | 159.02x |
| JavaScript | edit100 | 52.55 | 317.73 | 1.92 | 26.11x | 167.19x |
| JavaScript | splice | 50.79 | 337.37 | 2.01 | 24.33x | 155.04x |
| TypeScript | fresh | 306.32 | 184.46 | 60.92 | 5.07x | 3.03x |
| TypeScript | byte | 90.50 | 26.88 | 1.20 | 77.42x | 21.81x |
| TypeScript | edit100 | 100.53 | 236.38 | 1.19 | 80.84x | 191.75x |
| TypeScript | splice | 95.32 | 225.80 | 1.21 | 76.73x | 183.42x |
| Python | fresh | 591.96 | 724.57 | 103.19 | 5.84x | 7.22x |
| Python | byte | 55.43 | 732.50 | 97.15 | 0.56x | 7.12x |
| Python | edit100 | 59.95 | 744.54 | 79.65 | 0.77x | 8.93x |
| Python | splice | 55.77 | 780.66 | 82.86 | 0.72x | 9.37x |
| Rust | fresh | 381.48 | 188.50 | 58.84 | 6.16x | 3.03x |
| Rust | byte | 50.63 | 467.38 | 1.98 | 25.07x | 231.37x |
| Rust | edit100 | 51.73 | 445.67 | 2.01 | 26.10x | 223.55x |
| Rust | splice | 52.25 | 450.68 | 2.03 | 25.47x | 220.39x |
| Java | fresh | 357.99 | 219.14 | 72.95 | 4.80x | 2.92x |
| Java | byte | 70.78 | 31.22 | 2.11 | 37.18x | 15.44x |
| Java | edit100 | 74.26 | 258.77 | 2.05 | 36.47x | 128.63x |
| Java | splice | 75.72 | 250.92 | 2.13 | 36.47x | 115.12x |
| C | fresh | 318.81 | 254.79 | 71.56 | 4.60x | 3.42x |
| C | byte | 99.64 | 29.42 | 1.36 | 71.71x | 20.99x |
| C | edit100 | 98.77 | 265.08 | 1.38 | 73.09x | 183.85x |
| C | splice | 96.34 | 268.19 | 1.39 | 71.05x | 188.87x |
| C++ | fresh | 793.39 | 379.99 | 140.54 | 5.56x | 2.65x |
| C++ | byte | 112.89 | 411.19 | 4.20 | 27.56x | 91.06x |
| C++ | edit100 | 129.92 | 387.85 | 4.49 | 28.21x | 87.46x |
| C++ | splice | 127.12 | 419.44 | 4.29 | 28.85x | 91.61x |

| Language | Beats legacy on all edits? | Beats legacy fresh? | Pathological evidence | Graduation |
| --- | --- | --- | --- | --- |
| Go | No at 137 KiB | No at 137 KiB | Clean cliff matches C; legacy returns an error tree | Blocked: RSS and remaining correctness/gate work |
| JavaScript | Yes at 137 KiB; side sizes preliminary | Yes at 137 KiB; side sizes preliminary | Recovery D8 passes; fresh-C differences remain | Blocked: RSS and remaining correctness/gate work |
| TypeScript | No at 137 KiB | No at 137 KiB | Recovery D8 passes; fresh-C differences remain | Blocked: RSS and remaining correctness/gate work |
| Python | Yes at 137 KiB; side sizes preliminary | Yes at 137 KiB; side sizes preliminary | Recovery D8 passes; fresh-C differences remain | Blocked: RSS and remaining correctness/gate work |
| Rust | Yes at 137 KiB; side sizes preliminary | No at 137 KiB | Recovery D8 passes; fresh-C differences remain | Blocked: RSS and remaining correctness/gate work |
| Java | No at 137 KiB | No at 137 KiB | Recovery D8 passes; fresh-C differences remain | Blocked: RSS and remaining correctness/gate work |
| C | No at 137 KiB | No at 137 KiB | Recovery D8 passes; fresh-C differences remain | Blocked: RSS and remaining correctness/gate work |
| C++ | Yes at 137 KiB; side sizes preliminary | No at 137 KiB | Recovery D8 passes; fresh-C differences remain | Blocked: RSS and remaining correctness/gate work |

The published [benchstat output](compact-incremental-edit-benchstat.txt) compares the completed primary timing campaigns. The final allocation correction has separate warmed observations and no completed final randomized timing comparison.

The full D13/E-A gate is not complete: three median real files and the largest locked real file, 16 fixed deletion sites, latency percentiles, paired Go-C-C-Go ordering, startup/load measurements and retained-memory ratchets remain. Generated ranking alone does not justify graduation. Go has a correctness reason to prefer compact on its pinned clean cliff, but still fails fresh and one-byte legacy comparisons and RSS.

METRIC: go 137 KiB one-byte whole edit compact/C | 180.67x → 32.79x | legacy/C after 18.93x | served directions 0/18 → 18/18 | graduation blocked

METRIC: javascript 137 KiB one-byte whole edit compact/C | 23.73x → 23.65x | legacy/C after 159.02x | served directions 18/18 → 18/18 | graduation blocked

METRIC: typescript 137 KiB one-byte whole edit compact/C | 234.07x → 77.42x | legacy/C after 21.81x | served directions 0/18 → 18/18 | graduation blocked

METRIC: python 137 KiB one-byte whole edit compact/C | 7.12x → 0.56x | legacy/C after 7.12x | served directions 0/18 → 12/18 | graduation blocked

METRIC: rust 137 KiB one-byte whole edit compact/C | 186.01x → 25.07x | legacy/C after 231.37x | served directions 0/18 → 18/18 | graduation blocked

METRIC: java 137 KiB one-byte whole edit compact/C | 33.18x → 37.18x | legacy/C after 15.44x | served directions 18/18 → 18/18 | graduation blocked

METRIC: c 137 KiB one-byte whole edit compact/C | 172.72x → 71.71x | legacy/C after 20.99x | served directions 0/18 → 18/18 | graduation blocked

METRIC: cpp 137 KiB one-byte whole edit compact/C | 91.63x → 27.56x | legacy/C after 91.06x | served directions 0/18 → 18/18 | graduation blocked

## Peak RSS

Each observation runs one language, one route and one operation in a separate process under `/usr/bin/time -v`, using the default GC policy, 8 GiB container memory and a 6 GiB Go memory limit. The process parses a 1 MiB source and completes four operations; RSS includes grammar loading. All 192 before/after C, legacy and compact observations completed without an OOM. The hard 400-byte assertion remains active and fails for all eight compact languages. These are individual peak observations, not 20-run RSS medians.

| Language | Compact bytes/source byte before, four-mode range | After, four-mode range | Hard ceiling |
| --- | ---: | ---: | --- |
| Go | 545.4–859.7 | 507.8–567.2 | FAIL |
| JavaScript | 434.9–463.2 | 446.5–478.7 | FAIL |
| TypeScript | 1062.5–1217.4 | 1006.9–1305.4 | FAIL |
| Python | 1354.0–2068.6 | 1303.6–2240.5 | FAIL |
| Rust | 579.5–1289.7 | 539.4–603.4 | FAIL |
| Java | 515.0–610.1 | 554.4–687.0 | FAIL |
| C | 459.4–868.3 | 462.1–483.5 | FAIL |
| C++ | 657.4–923.8 | 646.7–868.8 | FAIL |

Java compact fresh RSS rises from 610.1 → 687.0 bytes/source byte (+12.6%) in the initial individual observations. Three paired repeats give medians 667.3 → 687.0 (+2.95%), with after values 596.9–693.0; the initial 10% breach does not reproduce in that median. This variability and the limited repeat count leave the RSS ratchet unproven. Directional memory regressions are not waived. The first C++ checkpoint candidate raised legacy fresh RSS from 423.3 → 517.9 (+22.4%) and compact fresh from 657.4 → 835.0 (+27.0%). Keeping only legacy’s reuse opt-out did not fix the memory cost (548.8 legacy / 809.7 compact in that trial). The final empty-serialization candidate fixes the legacy regression to 448.7 bytes/source byte (+6.0% versus baseline). Compact fresh still rises to 868.8 (+32.2%), while edits are about 646.9. That compact fresh regression remains unresolved and blocks the RSS ratchet. The final observations supersede the rejected C++ rows.

## Control benchmarks and attribution

The standard Go trio has 20 paired randomized seeds. Full parsing is 20.027 → 20.387 ms (benchstat p=0.989), one-byte control edits 395.333 → 395.610 µs (p=0.398), and no-edit reparses 19.12 → 17.96 ns (p=0.380). Full parsing stays at eight allocations, control edits at five, and no-edit reparses at zero bytes and zero allocations. Control edit bytes/op rise 392 → 400 (+2.04%); this directional increase is recorded and does not exceed the 10% allocation ratchet.

The recovered-dependency microbenchmark has 20 paired seeds. Depth 10 falls 79,415 → 469.20 ns; depth 16 falls 4,743,059.5 → 791.45 ns. Both allocate zero bytes and objects before and after. The 72-step JavaScript recovery session completes in 16.82 s after the fix; the prior duplicate walk was still inside `Tree.Edit` after 21 minutes. The strict session’s remaining C differences still fail its gate.

A 30-second Go compact 137 KiB one-byte CPU profile serves all 576 edits. Of samples within the complete edit operation, `Tree.Edit` takes 0.08%, reuse selection and certification 34.78%, and reparse/rebuild 65.14%. Materialization accounts for 46.87% of all sampled CPU and dependency byte comparisons for about 11.6%. Complete sampled stacks classify these groups and exclude grammar loading, initial parsing, release and background runtime frames from the operation denominator. Profiling timings are not comparison evidence.

## Reproduction and review

Run the focused root and internal tests through `cgo_harness/docker/run_parity_in_docker.sh`. In the cgo harness, set `GTS_CEILING_LANGUAGE` to one grammar and run `TestCompactEditsReuse`, `TestCompactEditsScannerStates`, `TestCompactEditsSession`, `TestCompactEditsFrontier`, and `TestCompactEditsRSS` individually. Set `GTS_CEILING_SIZES=32k,137k,1m`. The Python 1 MiB, recovery C comparisons, known fleet failures and RSS assertions are expected to report their unresolved gate failures; do not skip or weaken them.

For timing, run the randomized benchmark script inside Docker from `cgo_harness` with `--package . --tags treesitter_c_parity,gts_engine_ceiling --bench-regex "^BenchmarkEngineCeiling$"`, `GTS_CEILING_LANGUAGE=<grammar>` and `GTS_CEILING_SIZES=137k`. Set `GTS_CEILING_INCLUDE_DECLINES=1` to measure the complete requested route, including fallback. Benchmark receipts explicitly count served/declined operations; this switch does not change the positive correctness admission gate. Use `TestCompactEditsWork` with the additional `gts_workcount` tag for complete counter observations, and never use that tag for timing.

The external corpus lock was fetched from the repository’s configured variable because the VM environment lacked `GTS_CORPUS_LOCK_URL`; its SHA-256 was verified as `41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`. The lock and its URL were not copied into the repository. The reference is the lane’s locked C runtime 0.25.1 (`f5afe475deb7c0bae6407fb776c76824f717bb61`) and Go binding v0.25.0. Per-grammar artifact identities are in the JSON.

NEEDS-APPROVAL: `b326a5d40`, the separate Python expectation refresh. The 20 KiB prefix witness changes tokens 7,259 → 398, nodes 14,516 → 408, reuse 0 → 383 subtrees / 19,715 bytes. At 137 KiB, tokens change 48,850 → 2,587, nodes 97,698 → 2,597, reuse 0 → 2,572 / 135,114 bytes. Eight strict compact-fresh/C-fresh steps justify the positive expectation. The old negative test remains with an explicitly opted-out scanner. No ledger thresholds changed. C reused bytes and Rust subtree granularity still need ledger review; no exception or pin reset is included.

Other rejected work included a 16K read-storage retention candidate that added about 2.1 MiB of fresh Go allocations per parse, a relaxed historical-fork check that failed its rollback witness, and scanner states inferred from public child projections that failed Python’s triple-string witness. Bounded retention, clean live-path proofs and native reduction boundary states replaced them.

Remaining work is the Python 1 MiB compact capacity limit; C# frontier proof; fresh-C recovery differences; C and Rust reuse ledger decreases; RSS and memory regressions; complete D13/E-A corpus, percentile and pathological gates; and the remaining resource/performance ratchets. The branch rebased onto main `9148a96db` after #1402 merged; final measurements follow below. No graduation, global default change, legacy deletion or gate exception is part of this branch.

## Preliminary side-size observations

32 KiB and 1 MiB measurements use three paired seeds for the earlier implementation. The final C++ policy side-size campaign completed all three paired seeds at both sizes. The earlier C++ side-size timings enabled legacy reuse and are retained as rejected intermediate evidence, rather than final comparisons. Python 1 MiB uses one paired seed and explicitly measures compact decline plus legacy fallback. These samples are insufficient for graduation. Go, JavaScript and TypeScript were measured before the resolved-action optimization; other side sizes use that optimization. Full samples and memory/allocation metrics are in the JSON.

| Language | Size | Operation | Compact ms | Legacy ms | C ms | Seeds |
| --- | --- | --- | ---: | ---: | ---: | ---: |
| C | 1m | byte | 718.95 | 328.98 | 10.51 | 3 |
| C | 1m | edit100 | 878.50 | 2,588.73 | 13.61 | 3 |
| C | 1m | fresh | 2,346.44 | 1,838.97 | 492.74 | 3 |
| C | 1m | splice | 803.24 | 2,554.63 | 13.12 | 3 |
| C | 32k | byte | 17.29 | 6.11 | 0.39 | 3 |
| C | 32k | edit100 | 17.11 | 69.30 | 0.30 | 3 |
| C | 32k | fresh | 68.75 | 47.53 | 15.21 | 3 |
| C | 32k | splice | 17.04 | 53.62 | 0.33 | 3 |
| C++ | 1m | byte | 1,488.82 | 1,754.30 | 36.53 | 3 |
| C++ | 1m | edit100 | 849.06 | 1,923.49 | 42.46 | 3 |
| C++ | 1m | fresh | 3,437.69 | 1,750.72 | 751.86 | 3 |
| C++ | 1m | splice | 1,168.38 | 2,512.86 | 31.44 | 3 |
| C++ | 32k | byte | 8.08 | 60.84 | 0.73 | 3 |
| C++ | 32k | edit100 | 11.16 | 71.90 | 0.56 | 3 |
| C++ | 32k | fresh | 128.44 | 56.23 | 16.17 | 3 |
| C++ | 32k | splice | 15.36 | 69.94 | 0.69 | 3 |
| Go | 1m | byte | 619.15 | 161.37 | 12.62 | 3 |
| Go | 1m | edit100 | 648.08 | 2,065.38 | 12.41 | 3 |
| Go | 1m | fresh | 2,760.17 | 1,771.62 | 491.82 | 3 |
| Go | 1m | splice | 642.80 | 2,049.27 | 13.33 | 3 |
| Go | 32k | byte | 7.72 | 4.40 | 0.33 | 3 |
| Go | 32k | edit100 | 8.57 | 54.53 | 0.33 | 3 |
| Go | 32k | fresh | 80.15 | 45.38 | 13.63 | 3 |
| Go | 32k | splice | 8.47 | 51.97 | 0.35 | 3 |
| Java | 1m | byte | 960.95 | 279.21 | 21.53 | 3 |
| Java | 1m | edit100 | 1,164.69 | 2,742.35 | 25.80 | 3 |
| Java | 1m | fresh | 2,880.18 | 1,977.98 | 702.43 | 3 |
| Java | 1m | splice | 1,107.48 | 2,475.88 | 25.98 | 3 |
| Java | 32k | byte | 13.00 | 5.63 | 0.39 | 3 |
| Java | 32k | edit100 | 14.93 | 61.73 | 0.60 | 3 |
| Java | 32k | fresh | 91.04 | 56.65 | 18.45 | 3 |
| Java | 32k | splice | 16.09 | 61.21 | 0.61 | 3 |
| JavaScript | 1m | byte | 1,207.59 | 223.15 | 36.48 | 3 |
| JavaScript | 1m | edit100 | 766.80 | 4,601.98 | 34.06 | 3 |
| JavaScript | 1m | fresh | 2,367.16 | 5,851.30 | 480.25 | 3 |
| JavaScript | 1m | splice | 1,092.23 | 4,141.52 | 48.03 | 3 |
| JavaScript | 32k | byte | 9.19 | 83.39 | 0.49 | 3 |
| JavaScript | 32k | edit100 | 11.29 | 94.60 | 0.46 | 3 |
| JavaScript | 32k | fresh | 87.83 | 51.01 | 14.54 | 3 |
| JavaScript | 32k | splice | 9.76 | 79.51 | 0.51 | 3 |
| Python | 1m | byte | 6,623.78 | 5,436.11 | 679.31 | 1 |
| Python | 1m | edit100 | 6,426.34 | 5,589.92 | 498.99 | 1 |
| Python | 1m | fresh | 7,177.33 | 6,343.54 | 684.68 | 1 |
| Python | 1m | splice | 6,844.64 | 5,787.39 | 499.56 | 1 |
| Python | 32k | byte | 9.97 | 205.36 | 29.27 | 3 |
| Python | 32k | edit100 | 12.31 | 196.91 | 22.13 | 3 |
| Python | 32k | fresh | 154.64 | 168.23 | 22.32 | 3 |
| Python | 32k | splice | 11.84 | 208.55 | 21.98 | 3 |
| Rust | 1m | byte | 617.07 | 3,263.33 | 28.89 | 3 |
| Rust | 1m | edit100 | 660.26 | 3,369.06 | 27.93 | 3 |
| Rust | 1m | fresh | 2,798.46 | 1,585.19 | 453.61 | 3 |
| Rust | 1m | splice | 634.80 | 3,039.63 | 28.30 | 3 |
| Rust | 32k | byte | 9.36 | 99.87 | 0.44 | 3 |
| Rust | 32k | edit100 | 9.44 | 100.99 | 0.49 | 3 |
| Rust | 32k | fresh | 85.01 | 43.66 | 12.80 | 3 |
| Rust | 32k | splice | 9.68 | 100.03 | 0.52 | 3 |
| TypeScript | 1m | byte | 879.11 | 289.76 | 18.48 | 3 |
| TypeScript | 1m | edit100 | 964.43 | 2,152.64 | 17.56 | 3 |
| TypeScript | 1m | fresh | 2,616.13 | 1,800.69 | 680.07 | 3 |
| TypeScript | 1m | splice | 900.91 | 2,362.67 | 19.86 | 3 |
| TypeScript | 32k | byte | 21.45 | 6.40 | 0.33 | 3 |
| TypeScript | 32k | edit100 | 24.37 | 57.33 | 0.32 | 3 |
| TypeScript | 32k | fresh | 81.61 | 56.40 | 16.73 | 3 |
| TypeScript | 32k | splice | 25.92 | 54.86 | 0.32 | 3 |

## Final allocation correction

`51d8281fc` rejects the allocating checkpoint-copy implementation. Every retained arena snapshot is authenticated against the core’s complete serialized bytes before use; a changed boundary still copies and retains both exact states. The read-history allocation guard is retained for the same scheduler and rebound when a history changes owners. The focused compact/memory/checkpoint/forest gate passes, strict Python/Rust/C++ scanner and clean edit gates pass, and all 32 warmed whole-operation allocation cells pass. The ownership regression proves a transferred history obeys its new owner’s one-byte budget.

These paired fresh 137 KiB probes warm both directions, disable GC only for two counted operations, and restore GC afterwards. They count complete requested operations; they are allocation evidence, not timing or RSS evidence.

| Language | Baseline allocations/op → final | Baseline bytes/op → final |
| --- | ---: | ---: |
| go | 10 → 10 | 4776 → 4776 |
| javascript | 9.5 → 9 | 832 → 808 |
| typescript | 10 → 10 | 888 → 888 |
| python | 114473 → 14074 | 885560 → 242984 |
| rust | 100132 → 11 | 101000 → 880 |
| java | 7 → 7 | 392 → 392 |
| c | 1535 → 1535 | 102976 → 102976 |
| cpp | 11 → 11 | 22464 → 22464 |

The rejected intermediate fresh probes used 267,082 allocations / 2,010,032 bytes for Python and 251,833 / 252,712 for Rust. The final probes restore the baseline counts for Go, TypeScript, Java, C and C++, reduce JavaScript slightly, and reduce Python and Rust substantially. All edit-mode allocation observations are in the JSON. Final randomized timing and RSS measurements after this correction remain outstanding.

## Go parser-cliff timing

The pinned 75,688-byte fixture has 20 paired seeds on `de23264ae`. Compact matches locked-C fresh parsing; legacy’s clean fixture produces an error tree. The latency comparison therefore includes a correctness difference. The final recovered-dependency fix and allocation correction are measured separately.

| Operation | Compact ms | Legacy ms | C ms | Compact/C, median paired ratio |
| --- | ---: | ---: | ---: | ---: |
| fresh | 166.81 | 2570.50 | 27.574 | 5.98x |
| byte | 11.73 | 5615.62 | 0.137 | 81.91x |
| edit100 | 12.19 | 5741.07 | 0.136 | 87.32x |
| splice | 11.68 | 5835.37 | 0.170 | 68.05x |

The final allocation/checkpoint/guard focused race gate passes (1.268 s).

## Post-merge measurements

PR [#1403](https://github.com/odvcencio/gotreesitter/pull/1403) rebased onto main `9148a96db` after #1402 merged. The final engine revision is `f61c8afe2`; `ec670383a` fixes the checkpoint witness’s default-tag build. The preceding tables preserve the original campaign rather than relabeling those measurements as final. The [continuation JSON](compact-incremental-edit-continuation.json) identifies the final source bytes, work rows, resource observations and paired timing samples.

The final storage correction retains authenticated empty scanner boundary pairs in a core metadata bit and a public-arena bitmap. It keeps the 44-byte core record and distinguishes empty state from absent proof, including after replacement, rollback and reset. All 128 measured work/profile/route rows are identical before and after the storage correction.

The rejected core-only candidate has 20 paired C++ fresh RSS medians 663.94 → 748.17 bytes/source byte (+12.69%), breaching the 10% relative ratchet. The complete bitmap candidate has 20 paired medians 666.02 → 672.70 (+1.00%). Its baseline range is 644.71–724.96; final range 645.64–736.32. The final delta is below 5%. The relative ratchet passes this comparison; the absolute 400-byte ceiling still fails.

A one-process Java fresh RSS observation rose 669.46 → 933.60. The follow-up has 20/20 paired baseline/final observations: medians 689.79 → 696.63 (+0.99%), baseline range 571.59–775.04, final 571.22–782.38. The paired comparison, not the single observation, supplies the relative ratchet evidence. Absolute RSS remains failed.

The final fleet repeats all 48 edit steps and zero-allocation no-edit checks, one grammar per process: legacy 206/206; compact 204/206. Only TOML and Doxygen fail, matching the reproduced earlier failures. Focused root, memory-budget, checkpoint, forest and full parser-core suites pass, including focused race checks (root 1.713 s; core 1.556 s). Final Python/Rust/C++ scanner and 137 KiB clean-edit witnesses pass. Remaining fresh-C recovery and Python 1 MiB failures are not waived.

| Final clean-edit grammar | Recorded clean/scanner/prefix steps | Completed | Failed cells |
| --- | ---: | --- | --- |
| go | 36 | yes | none recorded |
| javascript | 36 | yes | none recorded |
| typescript | 36 | yes | none recorded |
| python | 44 | yes | TestCompactEditsReuse/1m/byte, TestCompactEditsReuse/1m/edit100, TestCompactEditsReuse/1m/splice |
| rust | 48 | yes | none recorded |
| java | 36 | yes | none recorded |
| c | 36 | yes | none recorded |
| cpp | 48 | yes | none recorded |

The final pinned Go parser-cliff witness records 12 clean edit steps; status PASS. Every passing step checks compact incremental = compact fresh = locked-C fresh, full input coverage and error status.

The final timing campaign uses 20 paired alternating seeds, 750 ms, GOMAXPROCS=1, one process per grammar/seed, standard allocation metrics and native C controls. All compact declines include fallback. The shared host has sustained CPU pressure; the first Python attempt on an over-contended core was interrupted and retained separately. It is excluded from final comparison evidence, and the complete campaign restarted on another core. These generated workloads do not complete the D13/E-A corpus gate.

| Language | Complete paired seeds | Byte compact ms | Byte legacy ms | Byte C ms | Byte compact/C | Fresh compact/C | Fresh compact/legacy |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| go | 20 | 166.00 | 88.42 | 5.01 | 33.15 | 5.58 | 1.61 |
| javascript | 20 | 127.87 | 898.00 | 6.53 | 19.44 | 4.79 | 0.91 |
| typescript | 20 | 176.49 | 60.93 | 2.37 | 75.79 | 5.49 | 1.61 |
| python | 20 | 176.84 | 1876.90 | 266.57 | 0.60 | 5.43 | 0.80 |
| rust | 20 | 118.37 | 1077.37 | 5.04 | 23.43 | 6.08 | 1.96 |
| java | 20 | 211.23 | 93.20 | 6.27 | 33.99 | 4.51 | 1.63 |
| c | 20 | 361.28 | 150.94 | 4.87 | 74.78 | 5.00 | 1.44 |
| cpp | 20 | 57.82 | 321.60 | 2.26 | 26.43 | 6.14 | 1.70 |

Rows with fewer than 20 completed paired seeds remain preliminary. Final operation-level distributions and all three benchmark metrics are in the JSON. No grammar graduates: hard RSS/C-relative/correctness failures and incomplete pathological/corpus gates remain. Go retains its clean parser-cliff correctness reason.

The C-relative ratios below are medians of compact/C ratios within the same seed. This separates relative performance from shared-host wall-time drift. The absolute times and benchstat output remain available; normalization does not waive any gate.

| Language | Operation | Paired seeds | Compact/C before → after | Relative change | Final compact/legacy | Allocation count before → after |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| go | fresh | 20 | 4.82 → 5.58 | +15.77% | 1.61 | 10 → 10 |
| go | byte | 20 | 186.77 → 33.15 | -82.25% | 1.92 | 349.5 → 137.5 |
| go | edit100 | 20 | 172.61 → 33.82 | -80.40% | 0.20 | 356 → 139.5 |
| go | splice | 20 | 192.30 → 33.84 | -82.40% | 0.20 | 357 → 141 |
| javascript | fresh | 20 | 4.17 → 4.79 | +14.87% | 0.91 | 9 → 9 |
| javascript | byte | 20 | 18.36 → 19.44 | +5.90% | 0.15 | 121 → 121.5 |
| javascript | edit100 | 20 | 20.49 → 22.90 | +11.74% | 0.16 | 125 → 125 |
| javascript | splice | 20 | 19.60 → 21.35 | +8.92% | 0.16 | 126 → 125.5 |
| typescript | fresh | 20 | 4.22 → 5.49 | +30.07% | 1.61 | 10 → 10 |
| typescript | byte | 20 | 227.96 → 75.79 | -66.75% | 3.02 | 46399 → 41512 |
| typescript | edit100 | 20 | 237.25 → 78.88 | -66.75% | 0.39 | 46403 → 40618 |
| typescript | splice | 20 | 234.69 → 72.55 | -69.09% | 0.36 | 46403 → 40619 |
| python | fresh | 20 | 5.02 → 5.43 | +8.15% | 0.80 | 114473 → 14074 |
| python | byte | 20 | 7.07 → 0.60 | -91.55% | 0.09 | 71 → 2178.5 |
| python | edit100 | 20 | 8.42 → 0.82 | -90.24% | 0.10 | 53 → 2170 |
| python | splice | 20 | 9.23 → 0.74 | -92.02% | 0.09 | 71 → 2170 |
| rust | fresh | 20 | 5.01 → 6.08 | +21.31% | 1.96 | 100132 → 11 |
| rust | byte | 20 | 157.41 → 23.43 | -85.11% | 0.10 | 546.5 → 1640 |
| rust | edit100 | 20 | 154.24 → 20.87 | -86.47% | 0.11 | 549 → 1643 |
| rust | splice | 20 | 188.36 → 15.80 | -91.61% | 0.11 | 550 → 1643 |
| java | fresh | 20 | 4.35 → 4.51 | +3.74% | 1.63 | 7 → 7 |
| java | byte | 20 | 34.25 → 33.99 | -0.76% | 2.19 | 150 → 152 |
| java | edit100 | 20 | 35.19 → 36.70 | +4.31% | 0.32 | 164 → 165.5 |
| java | splice | 20 | 35.22 → 36.80 | +4.48% | 0.30 | 164 → 167 |
| c | fresh | 20 | 4.31 → 5.00 | +16.10% | 1.44 | 1535 → 1535 |
| c | byte | 20 | 214.86 → 74.78 | -65.20% | 2.33 | 3201 → 1655 |
| c | edit100 | 20 | 208.86 → 75.68 | -63.76% | 0.37 | 3205 → 1667 |
| c | splice | 20 | 200.22 → 76.19 | -61.95% | 0.37 | 3207 → 1659 |
| cpp | fresh | 20 | 4.00 → 6.14 | +53.64% | 1.70 | 11 → 11 |
| cpp | byte | 20 | 81.01 → 26.43 | -67.37% | 0.27 | 21 → 152 |
| cpp | edit100 | 20 | 89.17 → 26.06 | -70.77% | 0.25 | 32 → 161.5 |
| cpp | splice | 20 | 79.96 → 26.01 | -67.47% | 0.30 | 38 → 163 |

| Language | 1 MiB fresh RSS before → after | Byte | 100-byte | Splice |
| --- | ---: | ---: | ---: | ---: |
| go | 543.83 → 550.24 | 620.24 → 497.78 | 879.72 → 533.58 | 874.21 → 510.21 |
| javascript | 490.52 → 477.44 | 462.92 → 446.06 | 469.88 → 456.70 | 438.42 → 471.89 |
| typescript | 994.50 → 1041.08 | 1215.53 → 1113.64 | 1202.48 → 1134.19 | 1187.86 → 1215.69 |
| python | 1285.31 → 1229.56 | 1995.80 → 2019.08 | 1915.96 → 2111.99 | 2157.18 → 2233.73 |
| rust | 568.56 → 604.47 | 1222.30 → 594.89 | 1247.21 → 600.55 | 1245.15 → 567.74 |
| java | 669.46 → 933.60 | 582.72 → 543.30 | 576.33 → 573.76 | 571.59 → 545.85 |
| c | 437.73 → 462.77 | 428.91 → 450.77 | 752.63 → 449.49 | 796.31 → 447.63 |
| cpp | 655.81 → 673.58 | 1023.96 → 583.03 | 916.26 → 634.33 | 1047.33 → 635.49 |

RSS values are bytes per source byte. This table has one process per revision/operation and is descriptive; the C++ fresh comparison above has the required 20 paired observations. Every recorded absolute observation exceeds 400. No memory threshold was changed.

| Language | Beats legacy on all three edits | Beats legacy on fresh | Pathological/fresh-C gate | Graduation |
| --- | --- | --- | --- | --- |
| go | no | no | remaining failures; clean cliff passes | blocked |
| javascript | yes | yes | remaining failures; not fully passed | blocked |
| typescript | no | no | remaining failures; not fully passed | blocked |
| python | yes | yes | remaining failures; not fully passed | blocked |
| rust | yes | no | remaining failures; not fully passed | blocked |
| java | no | no | remaining failures; not fully passed | blocked |
| c | no | no | remaining failures; not fully passed | blocked |
| cpp | yes | no | remaining failures; not fully passed | blocked |

The full E-A corpus, real-file D13 bands, 16-site edits, p50/p99 latency, startup/retained memory and complete pathological C parity remain unverified. The earlier 32 KiB/1 MiB timing campaign is preliminary and remains tied to its recorded source revision. Admission and defaults are unchanged.

NEEDS-APPROVAL: rebased pin commit `6974fd13a` (formerly `b326a5d40`) refreshes only the proven Python reuse expectation. C byte reuse decreases 137,219 → 129,625 bytes (−5.53%); Rust byte subtree reuse decreases 21,238 → 1,519 while reused bytes increase 39,849 → 137,301. These ledger rows still require owner review; no exception, reset or threshold change is applied.

CI follow-up `4810b8b90` moves the complete retention test into the existing scanner-dependency test file with the same build tag and assertions: root files 757 → 756, unchanged budget 756. The root-file snapshot stays unchanged. All 12 API tag sets and the R6 guard pass after the separate receipt refresh.

NEEDS-APPROVAL: `2fa436185` refreshes the default API snapshot by one interface (`CompactCheckpointedExternalScanner`), corrects the C++ certification wording to its explicit legacy opt-out, and preserves the Python/Starlark fail-closed test with opted-out scanners. Ordinary Python now has positive 63/65/137 KiB coverage. Sixteen strict insertion/deletion steps at 8/63/65/137 KiB match compact fresh and locked-C fresh with zero-allocation no-edit reparses. At 8 KiB the former zero-reuse expectation becomes 184 subtrees / 7,806 bytes, 195 tokens and 204 new nodes. Focused negative, positive, documentation and retained-capacity tests pass. Production engine bytes remain `f61c8afe2`.

The complete [counter-ledger audit](compact-incremental-edit-ledger-audit.json) observes 412 rows / 206 grammars: 63 changed rows and 37 counter violations. Eight default full-parse rows also drift. CI first rejects Apex’s candidate edit route changing from legacy fallback to compact reuse; the audit records every row rather than stopping there. Some compact block-splice observations are zero; no cross-engine semantic mapping is assumed. These broader changes remain unapproved and not proved eligible for a ledger refresh. The checked-in ledger and its 2% tolerance are unchanged.

Buckley’s commit-generation backend refused requests because its credits were exhausted. Subsequent commits use Buckley’s local strict message/diff check and an explicit Git commit, followed by push. This is the only commit-flow fallback; no safety or performance gate was disabled.

The bounded forward-walk lookup prototype preserves all 128 work/profile/route rows and passes 96 strict 137 KiB clean edit steps plus focused units. Its 20 paired fresh Go/C ratios are 5.39 → 5.61 (+4.14%, below 5%; no demonstrated Go improvement). C is 4.95 → 4.54 (−8.31%), with raw compact time 618.0 → 569.6 ms (−7.83%, p=0.017). Go raw time 772.2 → 738.7 ms has p=0.620. This candidate is rejected because it does not establish the needed Go improvement.

The second, binary-search-hint prototype has 20 paired go seeds: fresh Go/C 5.52 → 5.46 (-1.07%). Complete timing comparison. It is not shipped in the measured engine revision.

The second, binary-search-hint prototype has 20 paired c seeds: fresh Go/C 4.93 → 4.80 (-2.59%). Complete timing comparison. It is not shipped in the measured engine revision.

The second prototype’s raw fresh times are Go 790.1 → 764.4 ms (−3.25%, p=0.004) and C 583.6 → 560.9 ms (−3.89%, p=0.018). Focused root/internal units and 24 Go/C clean-C edit steps pass. It is rejected as insufficient: both raw and C-relative gains are below 5%, the main-relative fresh ratchets remain unresolved, and the full fleet/large-size campaign was not repeated for this prototype.

[CI compile, parity-cgo and perf-regression pass on the scanner/API revision](https://github.com/odvcencio/gotreesitter/actions/runs/36849412542). Broad draft correctness still fails the SQL malformed memory ceiling: 70,550,536 bytes against 67,764,224. SQL-only reruns on both main and final pass with matching memory/work observations (about 15.3–15.6 MB). Retained scratch from prior parses remains unresolved; this is not claimed as pre-existing. The ceiling and test stay unchanged. The counter-ledger check also remains failed.
