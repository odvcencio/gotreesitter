# Constructed cold receipts — rejected

**HOLD: V5 regresses all three full-parse workloads.** It constructs reuse lookahead/dependency facts during parse actions and replaces the final cold-receipt capture pass on supported syntax. A cached atomic capacity census replaces the recycle-pool lock on budget reads. No runtime is promoted, no language graduates and all main merges remain held.

The actual 19,294-byte primary500 Go source activates construction-time output and receipt planes: 15,505 raw records, 11,004 public occurrences, no SelectedStore and **no original cold-capture call**. Release and workcount builds prove activation before inspection. Tree shape and every cold receipt match the old publisher. Core and board work are unchanged; only `table_lookups_proxy` decreases 41,515→26,010. Ordinary Go R4 still declines at its first comment and retains the old publisher; YAML keeps the old publisher too. Registration or successful routing alone is not treated as activation.

The completed standard runner comparison contains **60 matched pairs / 120 processes**: 20 explicit seeds per series, alternating process order, count 1, 750 ms, GOMAXPROCS=1 and benchmem. Network-disabled Docker used one CPU and 4 GiB on Linux amd64, Intel Core Ultra 9 285, Go 1.26.6. All source/control guards pass; every R4 compact sample reports one route and zero fallback.

| Workload | Production → V5 | Change | p |
|---|---:|---:|---:|
| Generated Go full | 7.0989775 → 7.815501 ms | +10.09% | <.001 |
| Go edit | 67.523 → 64.683 µs | −4.21% | <.001 |
| Go no-edit | 3.1025 → 2.963 ns | −4.50% | <.001 |
| Real Go R4 compact | 763.6545 → 796.423 µs | +4.29% | <.001 |
| YAML R4 compact | 193.7385 → 199.8135 µs | +3.14% | .002 |
| Go R4 legacy control | 637.4695 → 634.9815 µs | −0.39% | .080 |
| YAML R4 legacy control | 136.5365 → 133.341 µs | −2.34% | <.001 |

Full allocations decrease 22→21 while bytes increase 261,543→261,743. Edit remains five allocations, with 407→406 B; no-edit remains zero bytes/allocations. Go/YAML R4 compact remain 1,112/5,456 B and 14/39 allocations. The edit gains do not offset the three full-parse regressions. No parse-plus-query/traversal timing or large-file qualification is claimed after this rejection.

## Correctness and memory scope

The qualified version is frozen V5 plus the **read-binding-v2** repair. Core code is unchanged by the binding repairs. Sixteen producer/pool/receipt families pass in release, workcount and race builds; two shadow clone families pass. The read-history generation variant passes42 `internal/incr` families and nine closure/generation families under race checking. Fifty-five existing Core families, 36 focused API/memory families, three Go public-API families and eight new API/budget families pass. Nine public race families cover constructed receipts, authority/source mutations, copy/concurrent read/reset and capacity.

Three strict constructed-receipt C families cover the actual primary500 source, three coordinate cases and three independent edits starting from a real constructed lease. They compare against the pinned C 0.25.1 oracle across symbols, fields, spans, points, flags and digest. Recovery/fresh verification remains intact. Ten existing Go and eight YAML C families also pass. Four R4 sessions pass 288 steps with no-edit allocations zero; all four actual default/candidate Go/YAML ledger rows equal corrected production exactly. Corpus sessions exercise the original publisher and supply fallback safety, not constructed-receipt coverage.

The independent public-capacity test manually enumerates backing capacities and verifies exact soft/hard cutoffs, unused cache capacity, stable surviving leases, reservation-time unrelated release, reset ownership conservation, retirement and late-release refusal. Both parent and V5 variants pass. Completed lease planes own their allocations; views borrow them, exclude duplicate capacity charges and neither clear nor recycle the original planes during copied-view materialization.

The first tracking primitive used existing header padding. Review then found that Reset(same source length), re-enable and re-record could invalidate earlier provisional cells without changing their pointer. The retained epoch variant charges a 72-byte amd64 header (eight additional bytes), increments generation on every Reset, and fails closed on overflow. Header-math test variants use the actual header API; all payload, capacity and failure assertions remain. Unknown/backward histories, history reset/rearm and source-pointer swaps retain original capture. No new reuse certificate is inferred from a read frontier.

## Retained failures and source handoff

- The first integrated build failed because a test helper redeclared `e`; a separate one-line test-only repair removes that declaration. Runtime and assertions are unchanged.
- Core tests passed but every real 1/17/500-function positive rejected final read authority. The driver restores its prior lexer reader before publication. The first handoff repair also compared against a temporary materialization-source field that the runner deliberately clears.
- A diagnostic records genuine handoff prerequisites: successful accepted run, one accepted payload, matching live reader, valid tracking, exact EOF and source identity. The corrected version binds the actual publication source once and checks its identity together with the saved lexer, source, cursor, reset epoch, history pointer and generation. New-source-owner, nil/short/capacity/repeated binding and later mutation controls remain strict. All earlier red sources/output remain archived.

CPU profiles are attribution only. In the exact V5 profile, syntax completion consumes 10.53% cumulative CPU. Receipt synchronization consumes 8.75%, including 8.46% in owned fact updates and 5.64% in cell closure; syntax construction consumes 6.23% nested in completion. The old capture pass is absent. New synchronization/closure work replaced much of its cost. Nested percentages must not be added. The next experiment must fuse raw, syntax and receipt writes inside an authenticated callback-free commit, while preserving genuine table/GOTO, growth, Gap and cancellation boundaries. No speedup is inferred from this profile.

[summary.json](summary.json) preserves paired medians and rejection; [verification.json](verification.json) records focused scope. [source-identities.json](source-identities.json), [evidence.tar.gz](evidence.tar.gz) and its [member index](contents.json.gz) retain frozen versions, failed outputs, corrected controls, source/authentication guards, benchmark reports and profiles. No native executables, caches, credentials, private CSV or unrelated sealed-reducer runtime are included. Replay uses recorded bounded Docker commands against pinned sources/dependencies. Full 206-grammar, E-A, large-file RSS, Go 1.22/1.23, Windows, Mac and ARM64 qualification remain open. Run `sha256sum -c SHA256SUMS` and `python3 validate-evidence.py` for read-only validation.
