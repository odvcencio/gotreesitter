# Complete parse-operation accounting

A public parse call now reports every engine attempt in `ParseRuntime.OperationWork`, including attempts whose result was discarded. `Total` equals the sum of `Initial`, `Compact`, `Retry`, `Fallback`, `Verification`, `Recovery`, and `Forest`. Each phase records attempts, token reads, allocated nodes, dispatch, proof, or comparison iterations, and tracked storage growth. Emergency heap roots and result materialization count as node work. The other runtime fields still describe the selected attempt. Incremental profiles report total operation tokens and nodes.

Children borrow the same deadline, cancellation state, explicit node and iteration limits, memory limit, and process-heap baseline. Active parent work consumes the child's remaining work allowance. Verification comparisons check the same deadline and remaining work while walking nodes and wide child lists, including lazy-view growth. Cancellation and timeout remain sticky until the outer call ends. Independent calls reset this state; reparses without edits retain their zero-allocation path.

Tracked bytes count positive arena, scratch, and compact scheduler growth beyond entry capacity, plus emergency heap roots. This counter retains work after storage is released. The memory limit bounds live growth, including an active parent's storage, so released work does not exhaust later memory allowance. Process-heap guards retain one baseline across attempts, including tiny verification of a large input. These counters are deterministic work receipts; benchmark `B/op` and RSS measure complete allocation and process costs separately.

## Accounting receipts

Baseline engine revision: `f9828512c`. First accounting increment: `4e92c0327`. Final engine increment: `2c0b4494e`. Fixtures and editing sessions come from the committed R4 manifest and its fixed first edit.

METRIC: parse attempt accounting | 2/8 (25%) -> 8/8 (100%) | f9828512c | Go 49-byte recovery edit, package p -> package pp
METRIC: verifier attempt accounting | 0/6 (0%) -> 6/6 (100%) | f9828512c | Go 49-byte recovery edit, package p -> package pp
METRIC: expired-deadline verifier token reads | 6 -> 0 | f9828512c | arithmetic 1+2+3, parent deadline already expired
METRIC: candidate-decline ledger coverage | 0/92 (0%) -> 92/92 (100%) | f9828512c | 206 R4 fixtures, default and candidate routes, full parse and fixed first edit

The Go recovery edit previously reported 20 tokens and 56 nodes. It now reports 146 tokens and 320 nodes: one initial attempt, one retry, and six verification attempts. An independent retry trace confirms eight attempts. The resulting tree equals fresh Go parsing. The expired-deadline baseline verifier accepted after six tokens and seven iterations; the shared deadline stops it before any token read.

The ledger previously omitted full and edit work for 92 candidate declines. It now retains these rows and records the fallback work. Availability rises from 320/412 to 412/412 rows for both full and edited parses. Attempts and iterations use the existing 2% counter ratchet. Thresholds and allowlists are unchanged.

## Validation and reproduction

Run focused tests inside the Docker harness, with `GOWORK=off` and `GOMAXPROCS=1`:

```sh
go test -tags gts_parsercorephase0,gts_workcount . \
  -run 'TestParseOperation.*|TestIncrementalFreshVerifierShares.*|TestIncrementalVerificationCountsDiscardedAttempts|TestOperationAttemptCountMatchesIndependentRetryTrace' \
  -count=1
go test ./internal/sched ./cmd/perfcounterledger -count=1
```

The invariant gate runs one grammar per process using `GOTREESITTER_V1_INVARIANT_LANGUAGE` and `TestV1InvariantGateR4EditSession`. It checks every edit-session step, root coverage and errors, and zero allocations for an unchanged reparse. Receipt digests compare the baseline and current complete trees for both routes, full parsing, and the fixed first edit.

The randomized wrapper uses 20 seeds, one process per seed, alternating baseline/current order, `750ms`, and allocation reporting. The primary trio covers full parsing, a single-byte edit, and unchanged reparsing. `BenchmarkAccountingCompleteGoCCGo` runs Go-C-C-Go subbenchmarks on the canonical Go `same_line_length_change` fixture. Each timed operation includes `Tree.Edit`, incremental parsing, result checks, and releasing the old tree. Admission checks fresh/incremental deep digests in both edit directions before timing.

C receipts use the locked 0.27 runtime `6070dbfefd326bd735e5683eb128cc1b57dad0c0`, supplied by oracle commit `3a0e7df5`. The runtime module is mounted as a dependency override for both baseline and current harnesses; engine code is unchanged in the baseline. Verify the oracle manifest before running. Keep the external corpus lock outside the repository and verify its digest.

The first increment used the loop node counter, which omitted 16 allocations in result construction on the recovery-edit witness. The final operation counter uses arena allocations and records emergency heap roots. A verifier with no remaining iterations still constructs one error root; its operation receipt now records that node and its bytes even though loop nodes and token reads remain zero. This node-pin correction is separate from the code change.

Rejected candidate: charging cumulative tracked bytes as a memory allowance stopped verification after 64 MiB of released work, even on a one-byte arithmetic input. The live-growth policy accepts that input, reads two tokens, and retains the earlier 64 MiB in the work receipt.


## Correctness at the original engine revision

Locked C 0.27 fresh and incremental smoke parity each passed 206/206 grammars, one grammar per process. Go compact recovery and Stage 6 regressions passed. All 824 baseline/current complete-tree digests match. The strict refreshed counter ledger passed all 412 language/route rows; its 2% boundary tests passed. Focused shared-budget tests passed with compact enabled and disabled. Go compact execution, profile schema, and Python prefix-frontier regressions passed. L0 checked 756 root Go files and 12 API tag sets.

The invariant sweep passed 198/206. Four languages retain the same baseline edit-session digest mismatch: awk, JavaScript, Meson, and Twig. Elsa, Haskell, Kotlin, and PowerShell retain the baseline five-minute timeout. Baseline Nickel also timed out; the candidate finished in 279.97 seconds, so this run does not claim a Nickel correctness improvement. The sweep used the final allocator and live-budget implementation before comparison traversal checks were added; the final traversal checks passed focused tests, all 824 tree-digest comparisons, and the C fleet.

Python's leading-comment first-child edit also exposes one result parent: the loop reports 37 nodes, the complete operation reports 38, and the profile reports 38. Incremental Go, fresh Go, and locked C share digest `bc302c1d538df0da042ebb9e07828378d630fe3111802802dc8cbe837faa241e`. The expectation now checks complete operation nodes and retains the loop count as a floor.

[The pin receipt](receipts/operation-accounting-pin-refresh.json) lists every changed row, before/after values, stale pin fields against the actual baseline, and all tree digests. Full receipts expand from 320 to 412 rows, tokens from 119,083 to 217,330, and nodes from 352,871 to 1,045,126. Edited receipts expand from 320 to 412 rows, tokens from 84,910 to 269,304, and nodes from 307,996 to 1,537,226. Full and edit totals expose 643 and 898 attempts and 312,749 and 487,267 iterations. The earlier ledger omitted attempt and iteration fields; their absence did not mean zero work.

## Large-file memory receipt

The fixed Go fixture has 1,048,580 bytes and SHA-256 `fb311bdf8ed3c1d206b55e941308719baf0faf2f5469df98669a3c7fe8a616da`. Full parsing and the middle numeric edit finish with accepted, complete, error-free trees. Baseline Go, current Go, and C share result digest `74d7d8d918baaafa8da354f6b4c852f4b2d1240e645f212736f14d1fe49244f5`.

Process peak RSS was 582,568 KiB on the baseline, 547,216 KiB on the current engine, and 86,636 KiB for C. Both Go measurements exceed the existing 400-bytes-per-source-byte release limit. This is a process measurement, including fixture setup, full parsing, `Tree.Edit`, incremental parsing, deep validation, and releasing handles; it is not a randomized timing receipt.

Reproduce each backend separately inside the Docker harness with the locked C dependency:

```sh
GTS_ACCOUNTING_RSS_BACKEND=Go /usr/bin/time -v go test . \
  -tags treesitter_c_parity,gts_parsercorephase0 \
  -run '^TestAccountingLargeCompleteOperationRSS$' -count=1 -v
GTS_ACCOUNTING_RSS_BACKEND=C /usr/bin/time -v go test . \
  -tags treesitter_c_parity,gts_parsercorephase0 \
  -run '^TestAccountingLargeCompleteOperationRSS$' -count=1 -v
```

The published process RSS numbers run the compiled test binary under `time`, excluding compilation. Compile with `go test -c` before measuring to match them.

## Assumptions and remaining gates

The supplied audit index was absent, so this run rebuilt evidence from engine revision `f9828512c`, the committed R4 corpus manifest, and the specified oracle pin. The oracle's 102-file digest manifest was verified. The external corpus lock digest is `41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`; the lock is not committed.

The VM also ran other lanes. The primary trio was pinned to CPU 7. The first C comparison changed CPU during collection and is rejected as comparison evidence. The fixed-CPU comparison below completed all twenty seeds on CPU 5. CPU 7 was shared with another pinned workload during the primary trio. Treat timing differences as receipts with this noise limit. C benchmark `B/op` and `allocs/op` cover the Go bridge. [PR #1406](https://github.com/odvcencio/gotreesitter/pull/1406) adds a separate untimed native receipt: 180 requests and 35,248.5 requested bytes per complete edit operation, identical on baseline and current C. Native allocation requests include runtime hooks, UTF-8 CStrings, and the input-payload handle. Native C time and process RSS include native work.

At the original revisions, the accounting metric was met and eight invariant failures remained. The integrated results below replace that invariant census. Existing Go/C and RSS hard failures and the full multi-size, sixteen-site release performance matrix remain. Pin refreshes require owner review. No graduation, gate threshold, or exemption changes are included.


## Rejected first timing receipt

The first final-engine trio completed twenty seeds on the same CPU for both revisions. Full parse was 34.97 ms -> 40.51 ms; single-byte editing was 707.5 microseconds -> 798.8 microseconds (+12.90%, p=0.049); unchanged reparsing was 31.91 ns -> 38.22 ns. Allocations stayed 8/5/0, including zero bytes for unchanged reparsing. The edit median exceeds the 10% timing ratchet. These receipts are retained and are not accepted as a performance pass. CPU contention produced wide intervals; the fixed-CPU rerun below remains within the ratchet and finds no significant timing change. The first run remains rejected evidence.

The raw [baseline trio](receipts/operation-accounting-trio-before.txt), [current trio](receipts/operation-accounting-trio-after.txt), and [benchstat comparison](receipts/operation-accounting-trio-benchstat.txt) identify the exact revisions and settings. The complete-operation C comparison below uses one fixed CPU for all samples. No gate is waived.


## Fixed-CPU randomized complete-operation timing

Engine revisions are `f9828512c` before and `2c0b4494e` after. [Raw samples and settings](receipts/operation-accounting-stable-performance.json) record 20 shuffled seeds per revision. The source manifest verifies that the tested engine files match the committed source.

| Workload | Before median | After median | Change |
| --- | ---: | ---: | ---: |
| GoParseFullDFA | 36,264,029.500 ns | 35,424,542.500 ns | -2.31% |
| GoParseIncrementalSingleByteEditDFA | 733,983.000 ns | 735,939.000 ns | +0.27% |
| GoParseIncrementalNoEditDFA | 32.185 ns | 31.115 ns | -3.32% |

GoParseFullDFA: bytes/op 1386 -> 1374; allocs/op 8 -> 8. GoParseIncrementalSingleByteEditDFA: bytes/op 399.5 -> 400; allocs/op 5 -> 5. GoParseIncrementalNoEditDFA: bytes/op 0 -> 0; allocs/op 0 -> 0.

The fixed canonical Go fixture has 41,387 source bytes, 41,394 edited bytes, and source SHA-256 `009aa9fd5352c712f3839670c7df8a9b00ae878ee20dc88131a438b2d5edfd9a`. The mean of the two Go phase medians divided by the mean of the two C phase medians is 497.086x -> 528.870x (+6.39%). Both exceed the existing 10x release hard limit. No speed win is claimed. Directional changes and significance are in the [trio benchstat receipt](receipts/operation-accounting-stable-trio-benchstat.txt) and [C cycle receipt](receipts/operation-accounting-stable-c_cycles-benchstat.txt). C benchmark allocation metrics cover the Go bridge only; an untimed native allocation receipt is collected separately.

| Cycle phase | Before ns/op | After ns/op | Before/after B/op | Before/after allocs/op |
| --- | ---: | ---: | ---: | ---: |
| GoBefore | 394,743,744 | 409,456,228 | 1.78528e+07 / 1.68107e+07 | 336.5 / 315 |
| CBefore | 787,889 | 775,614 | 18704 / 18704 | 9 / 9 |
| CAfter | 777,338 | 758,151 | 18704 / 18704 | 9 / 9 |
| GoAfter | 383,308,554 | 401,706,488 | 1.58001e+07 / 1.62694e+07 | 281.5 / 295 |

## Integration with the new verifier on main

Main revision `9148a96db` moved verification into a shared helper and added token-source factory verification. Engine commit `4a4c850a9` preserves those routes and applies the same deadline, configured work limits, live memory allowance, and comparison accounting. The factory regression stops before token reads for expired deadlines and exhausted iteration allowance.

METRIC: parse attempt accounting | 2/11 (18.18%) -> 11/11 (100%) | 9148a96db | Go 49-byte recovery edit, package p -> package pp
METRIC: verifier attempt accounting | 0/9 -> 9/9 (100%) | 9148a96db | Go 49-byte recovery edit, package p -> package pp

Unmodified main's independent trace records eleven attempts while its selected profile reports 18 tokens and 54 nodes. Complete accounting records eleven attempts, nine in verification, 207 tokens and 450 allocated nodes. The result equals fresh Go; main's result digest is `b8729191ee91f629a573f7ad2cc9a14a03f33211361190bb025d8b5b24f7f868`. All 824 R4 complete-tree digests and stop/coverage/error fields match unmodified main. The original receipt above remains tied to its original revisions. The integrated branch has its own counter, correctness, RSS, and randomized timing checks.


## Integrated correctness and newly exposed ceilings

At main `9148a96db` and engine `4a4c850a9`, all 824 full/first-edit tree digests and stop, coverage, and error fields match. The [integrated correctness receipt](receipts/operation-accounting-integrated-correctness.json) records focused enabled/disabled budget tests, factory verification, compact execution, legacy reuse, profile schema, strict 412-row counter gating, Python prefix fallback, Go compact/Stage 6 parity, and Python's extra result parent. Locked C 0.27 fresh and incremental smoke parity passed 206/206 grammars, one grammar per process.

The exact integrated invariant sweep passed 200/206. Elsa, Godot Resource, Haskell, Kotlin, Nickel, and PowerShell each reached the unchanged five-minute limit on both the integrated engine and unmodified main. AWK, JavaScript, Meson, and Twig now pass on the integrated main revision. No timeout or failing gate was skipped or relaxed.

Complete profiles expose resource-ceiling failures that the old profiles hid. The [gate-work receipt](receipts/operation-accounting-exposed-gate-work.json) runs each grammar separately, records the original gate before and after, and proves identical complete-tree/fresh hashes and independent legacy attempt traces. TypeScript and SQL each already perform 29 attempts; complete accounting records all 29. JavaScript's independent trace covers its three legacy attempts; complete accounting also includes its verification forest attempt.

| Fixed start-position insertion | Legacy attempts before/after | Profile tokens before/after | Profile nodes before/after | Existing gate |
| --- | ---: | ---: | ---: | --- |
| JavaScript W5 20 KB, duplicate assignment operator | 3 / 3 | 2470 / 8643 | 10478 / 24044 | PASS -> FAIL |
| TypeScript W5 20 KB, duplicate assignment operator | 29 / 29 | 2506 / 164560 | 11766 / 410205 | PASS -> FAIL |
| TSX W5 20 KB, duplicate assignment operator | 5 / 5 | 2506 / 20512 | 11766 / 56037 | PASS -> FAIL |
| SQL 4 KiB malformed dollar quote, insert numeric byte | 29 / 29 | 1206 / 19809 | 7820 / 116087 | PASS -> FAIL |

These are newly visible costs, with unchanged trees and legacy attempt schedules. Their ceilings remain unchanged. The engine PR remains draft; reducing this existing retry/verification work is required to make those gates green under complete accounting. The gate receipt retains the exact failed ceiling messages.

The separate layout expectation refresh `bcb60af45` records Parser 2336 -> 2824 bytes and ParseRuntime 3120 -> 3440 bytes. Tree stays 208 bytes and the full-parse retry offset stays 1064. The focused layout test passes. The extra runtime storage consists of total work and seven phase records; the parser holds the shared ledger and budget state. This refresh requires owner review alongside the other pin commits.

On the fixed 1,048,580-byte Go workload, main's RSS was 561,856 KiB, accounting's 508,944 KiB, and C's 86,768 KiB. All three returned accepted, complete, error-free trees with digest `74d7d8d918baaafa8da354f6b4c852f4b2d1240e645f212736f14d1fe49244f5`. Both Go measurements still exceed the unchanged 400-bytes-per-source-byte release limit.

Configured node and iteration allowances are shared across attempts; source-derived engine safety ceilings remain in force inside each attempt. Independent calls reset the shared allowance. The native C accounting increment is commit `0a06fb1ae`, [PR #1406](https://github.com/odvcencio/gotreesitter/pull/1406). Its existing CI checks, including both WebAssembly targets, are green. The first WebAssembly collection hit the unchanged ten-minute limit; the retry passed. No gate setting changed.


## Integrated fixed-CPU randomized timing

Engine revisions are `9148a96db` before and `4a4c850a9` after. [Raw samples and settings](receipts/operation-accounting-integrated-performance.json) record 20 shuffled seeds per revision. The source manifest verifies that the tested engine files match the committed source.

| Workload | Before median | After median | Change |
| --- | ---: | ---: | ---: |
| GoParseFullDFA | 49,027,950.500 ns | 51,603,264.000 ns | +5.25% |
| GoParseIncrementalSingleByteEditDFA | 957,635.500 ns | 1,067,834.000 ns | +11.51% |
| GoParseIncrementalNoEditDFA | 43.850 ns | 45.120 ns | +2.90% |

GoParseFullDFA: bytes/op 1446.5 -> 1459; allocs/op 8 -> 8. GoParseIncrementalSingleByteEditDFA: bytes/op 407 -> 410; allocs/op 5 -> 5. GoParseIncrementalNoEditDFA: bytes/op 0 -> 0; allocs/op 0 -> 0.

The fixed canonical Go fixture has 41,387 source bytes, 41,394 edited bytes, and source SHA-256 `009aa9fd5352c712f3839670c7df8a9b00ae878ee20dc88131a438b2d5edfd9a`. The mean of the two Go phase medians divided by the mean of the two C phase medians is 645.035x -> 612.600x (-5.03%). Both exceed the existing 10x release hard limit. The edit result exceeds the unchanged 10% timing ratchet (+11.51%, p=0.004). This candidate is rejected as a performance pass. No speed win is claimed. Directional changes and significance are in the [trio benchstat receipt](receipts/operation-accounting-integrated-trio-benchstat.txt) and [C cycle receipt](receipts/operation-accounting-integrated-c_cycles-benchstat.txt). C benchmark allocation metrics cover the Go bridge only; an untimed native allocation receipt is collected separately.

| Cycle phase | Before ns/op | After ns/op | Before/after B/op | Before/after allocs/op |
| --- | ---: | ---: | ---: | ---: |
| GoBefore | 466,961,370 | 488,178,544 | 1.87466e+07 / 1.87471e+07 | 373.5 / 374 |
| CBefore | 738,070 | 782,424 | 18704 / 18704 | 9 / 9 |
| CAfter | 758,831 | 708,971 | 18704 / 18704 | 9 / 9 |
| GoAfter | 498,591,656 | 425,450,225 | 1.68515e+07 / 1.68612e+07 | 324.5 / 326 |

The complete C allocation diagnostic combines 18,704 Go bridge bytes and 35,248.5 native requested bytes: 53,952.5 bytes/op, with 9 Go allocations and 180 native requests. Go reports allocator-accounted bytes; native C reports successful requested bytes. These diagnostics preserve the existing release gates.

A follow-up candidate reuses one scratch-byte sum across the operation, scratch, and runtime memory checks. It runs in an isolated checkout and is not shipped without complete verification.
