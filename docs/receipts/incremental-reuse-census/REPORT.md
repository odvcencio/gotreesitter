Real-code reuse fails for different reasons. Go and Java lose almost all physical node reuse while multiple GLR stacks bar the reuse dispatcher. C# never enters reuse because its scanner does not opt in. Languages that retain most old nodes still pay for fresh verification or discarded retries. PowerShell additionally exposes an exponential dependency walk already fixed on another branch.

The instrument is enabled only by `gts_incr_census`. With it disabled, production binaries contain no census hook symbols, and checked parse digests, work counters and retained identities match the original engine. No engine policy, existing pin, ledger limit or graduation list changed.

| Language | Nodes retained | Ranked edit costs: time % / lost-node % | Observer time |
|---|---:|---|---:|
| Go | 2.1% | multiple GLR stacks **67.6/97.4**; fresh recovery verification **15.3/0.0**; reparse and rebuild **4.0/0.0** | 10.3% |
| JavaScript | 82.6% | reparse and rebuild **16.9/0.0**; reuse selection **14.3/0.0**; fresh recovery verification **11.2/0.0** | 40.9% |
| TypeScript | 96.4% | reparse and rebuild **17.1/0.0**; reuse selection **12.0/0.0**; fresh recovery verification **11.3/0.0** | 37.7% |
| Python | 73.7% | fresh recovery verification **46.8/53.7**; reparse and rebuild **13.1/0.0**; reuse selection **5.6/0.0** | 21.3% |
| Rust | 57.9% | full-parse retry **26.6/0.0**; reparse and rebuild **16.8/0.0**; reuse selection **7.5/0.0** | 31.2% |
| Java | 4.1% | full-parse retry **77.6/0.0**; multiple GLR stacks **19.1/97.9**; reparse and rebuild **1.2/0.0** | 1.7% |
| C# | 0.0% | unsupported scanner → fresh parse **100.0/100.0**; Tree.Edit **0.0/0.0**; missing-node dependency proof **0.0/0.0** | 0.0% |
| PowerShell* | 66.8% | fresh recovery verification **69.2/0.0**; full-parse retry **16.6/0.0**; multiple GLR stacks **2.6/15.1** | 5.9% |

The table covers the **144 requested edits**, with time-weighted exclusive wall time and lost-node-weighted physical identity shares across both sizes and all three sites. A cell is `time % / share of old nodes not retained %`. Time includes work performed while a dispatcher barrier is active, not just evaluating its Boolean. Selection/rebuild rows often own time and no lost nodes; their guards have separate loss rows below. Observer self-time is shown separately; snapshot/final walks are excluded. This is not CPU sampling or a prediction of savings from removing a guard.

*PowerShell uses the existing dependency fix [`4afda6f2d`](https://github.com/odvcencio/gotreesitter/commit/4afda6f2d5dccb0c4742b30aac6af56f02be4a3d) in both isolated measurement checkouts. Original before/after invariant sessions timed out after 20 minutes in the recursive dependency walk. With the fix, all 72 session steps pass (4.45 s before, 5.48 s after), including zero no-edit allocations. The original per-edit cost share is undefined for unfinished edits. These control numbers do not claim original PowerShell throughput. Its nearest locked large source is 104,415 bytes, rather than exactly 137 KiB.

The five largest guards/fallbacks follow. The stalled original dependency walk ranks first by its observed lower bound. The remaining four are ranked by aggregate exclusive time in the completed requested matrix; numeric controls, observer overhead and general parsing/selection work are excluded.

| Cost rank | Guard or fallback and origin | Correctness protection | Locked C structural rule |
|---|---|---|---|
| 1, original stalled workload | Missing-node dependency recursion; [`e91b944fc`](https://github.com/odvcencio/gotreesitter/commit/e91b944fc6f11b1bcc71a814970cd383f56b0128) | Do not skip a zero-width recovery insertion whose padding/lookahead extends past its visible span. Both helpers walked ordinary error descendants, doubling visits at each nesting level. Existing [`4afda6f2d`](https://github.com/odvcencio/gotreesitter/commit/4afda6f2d5dccb0c4742b30aac6af56f02be4a3d) removes the duplicate traversal; the instrument branch does not include it. | `ts_subtree_summarize_children` stores the aggregate lookahead bound; `ts_subtree_edit` prunes using that bound and column dependence. No repeated whole error-subtree proof. |
| 2 | Unsupported external scanner → fresh parse; [`b32f549fc`](https://github.com/odvcencio/gotreesitter/commit/b32f549fc77f8a534ec70addc8816ecd5f37c058) | Skipping tokens without restoring serialized mutable scanner state can change the next token (the original policy explicitly cites indentation state). The origin provides safety-policy tests, rather than a named C# bug. C# declines every tested edited parse. | `ts_parser__reuse_node` checks serialized scanner-state equality at a boundary. The Go opt-in interface adds a broader admission barrier. |
| 3 | Fresh recovery verification; [`8e4b65238`](https://github.com/odvcencio/gotreesitter/commit/8e4b6523836e965f533167196c2e3936775a3aba) (#1280, issue #454) | Changed recovery choices, mismatched ownership frontiers and transient errors can make incremental output differ from fresh Go. Publish only a verified result or a fresh replacement. | Changed/error/missing/fragile subtrees and incompatible first-leaf modes are refused or broken down before reuse. There is no whole fresh parse for post-hoc equality. |
| 4 | Full-parse retry after accepted errors; [`46b551bba`](https://github.com/odvcencio/gotreesitter/commit/46b551bba9093e16902cb1fc739b02e432f07d1f); DFA extension [`bf7ab0567`](https://github.com/odvcencio/gotreesitter/commit/bf7ab0567122e863ef831e8aa56dbee93127ad93) | Prefer a clean/complete result after exhausted stacks, node budgets or a large accepted error. The later DFA safety net protects issue #454's budget-aborted, truncated error result. Java and Rust spend substantial time on retries that do not win. | Version-local recovery/error-cost selection is structural; the C implementation has no matching Go whole-file retry ladder. |
| 5 | Global single-live-stack reuse admission; [`1b6805552`](https://github.com/odvcencio/gotreesitter/commit/1b68055524f24c78e874feba2e88ec21a1102758) | The original implementation inserts reuse into one destination stack. The guard avoids applying that splice across a branching frontier without an ownership proof. No named regression attached to the origin was found; this is a conservative original limitation. | `ts_parser__reuse_node` takes a stack version and checks that version's state; there is no blanket global `len(stacks)==1` condition. |


C comparisons were checked against the pinned [parser](https://github.com/tree-sitter/tree-sitter/blob/f5afe475deb7c0bae6407fb776c76824f717bb61/lib/src/parser.c) and [subtree implementation](https://github.com/tree-sitter/tree-sitter/blob/f5afe475deb7c0bae6407fb776c76824f717bb61/lib/src/subtree.c), runtime 0.25.1 (`f5afe475`), binding 0.25.0. The corresponding rules preserve safety; bypassing a Go guard without the missing structural proof would be unsafe.

For the historical same-width path, [`93a8724dc`](https://github.com/odvcencio/gotreesitter/commit/93a8724dc916b3848cacde2d7395ac366d06d0f6) disabled unsafe reuse and [`d9916dbaa`](https://github.com/odvcencio/gotreesitter/commit/d9916dbaa1f3469397bd85ce27286ee98e16074a) restored it with bounded lexical proof. The longest-match witness `(abbbd;` → `(abbbc;` shows why an edit after a token can change that earlier token. The C aggregate lookahead rule marks the dependency structurally. The 48 numeric controls are separate in `summary.json`; this proof is not the leading cost in the requested insertion/splice matrix. Additional origins: state/shift compatibility [`1bffa9ad2`](https://github.com/odvcencio/gotreesitter/commit/1bffa9ad265bf32b6ab5b03d2e9f1b261a253239), stale leaf boundaries [`1dfc6cde2`](https://github.com/odvcencio/gotreesitter/commit/1dfc6cde2bc491fe191cd88cc19343b4f37be980), ownership divergence [`95a1d1315`](https://github.com/odvcencio/gotreesitter/commit/95a1d13156b82e2221415342c9ac8048f2caf644), and fragility [`d88027b5f`](https://github.com/odvcencio/gotreesitter/commit/d88027b5f34504344fa0ffdd069349b563a8445a).

| Language | Actual fixture bytes (32 / 137 KiB target) | Diagnostic peak RSS MiB | Incremental/fresh Go failures | Fresh Go/C failures | C incremental/fresh failures |
|---|---|---:|---:|---:|---:|
| Go | 32686 / 140026 | 269.9 | 0/24 | 8/24 | 0/24 |
| JavaScript | 32689 / 141043 | 149.9 | 5/24 | 5/24 | 0/24 |
| TypeScript | 32652 / 140212 | 872.6 | 0/24 | 9/24 | 1/24 |
| Python | 32768 / 139106 | 273.5 | 0/24 | 10/24 | 0/24 |
| Rust | 32765 / 140019 | 455.2 | 0/24 | 16/24 | 0/24 |
| Java | 32728 / 144870 | 639.6 | 4/24 | 8/24 | 0/24 |
| C# | 32846 / 134787 | 531.6 | 0/24 | 24/24 | 0/24 |
| PowerShell* | 32666 / 104415 | 563.6 | 0/24 | 24/24 | 0/24 |

Nine incremental/fresh Go failures and all checked original fresh-Go/C failures are unchanged from the baseline. The independent R4 invariant gate passes Go, TypeScript, Python, Rust, Java and C# before/after; JavaScript fails at the same step-2 digest, and original PowerShell times out. Existing probe/session ledgers and core no-edit allocation tests pass where those suites provide a language fixture. Smoke fresh/incremental/no-error C parity passes each of the eight languages in a separate process. The new inventory preserves failures instead of treating it as graduation.

Two completed original PowerShell cells match the fixed controls in digest, tokens and allocated nodes. The middle 32 KiB one-byte cell took a diagnostic median **73,275.889 ms** in the original engine versus **57.040 ms** with the existing fix; the quarter-site cell was 34.410 versus 61.211 ms. These six-sample diagnostic timings identify the stall and local variation; they are not a randomized before/after optimization receipt. The instrument branch does not ship this fix.

The locked C runtime also disagrees with its own fresh parse in one unchanged baseline/current cell: TypeScript 32 KiB splice at the middle site (offset 16,326; edited SHA-256 `e59c04ee1b4cb45bdc20d0b1f8e5d9ef27a77f1142f472c7fcee972aa40375d5`). Fresh C remains the correctness oracle. Go/C timing ratios on unequal results are diagnostic execution costs, not matched-parity performance targets.

Java core no-edit allocates zero. Its cached token-source factory allocates **1 → 1**, and the registry-style wrapper allocates **2 → 2** on the smoke fixture. That pre-existing API path still violates the broader no-edit allocation goal.

All 192 instrumented callbacks match their unobserved counterpart. Work/identity comparisons cover 168 original-engine cells plus 24 matched PowerShell dependency controls. Timing rows plus observer time equal each edit wall time exactly; node loss rows sum to the physical old nodes absent from the selected tree. `final_fallback` means the last event across attempts: PowerShell has nine requested cells with fallback events but no selected zero-reuse result, so the cost is paid even though the final tree still reuses nodes. Python has nine requested selected zero-reuse results.

| Language | Returned-runtime tokens before → after | Returned-runtime nodes allocated before → after | Retained identities before → after |
|---|---:|---:|---:|
| Go | 327,088 → 327,088 | 1,820,563 → 1,820,563 | 8,989 → 8,989 |
| JavaScript | 15,609 → 15,609 | 59,351 → 59,351 | 159,532 → 159,532 |
| TypeScript | 23,115 → 23,115 | 286,129 → 286,129 | 1,240,309 → 1,240,309 |
| Python | 115,374 → 115,374 | 322,939 → 322,939 | 448,363 → 448,363 |
| Rust | 105,811 → 105,811 | 381,529 → 381,529 | 396,701 → 396,701 |
| Java | 292,300 → 292,300 | 3,136,790 → 3,136,790 | 17,645 → 17,645 |
| C# | 223,364 → 223,364 | 3,387,195 → 3,387,195 | 0 → 0 |
| PowerShell* | 95,529 → 95,529 | 858,448 → 858,448 | 356,225 → 356,225 |

These are returned-tree runtime counters, not totals for discarded verification/retry attempts. All 14 checked work/reuse fields and three identity counts match exactly, a stricter comparison than the design’s 2% ledger tolerance. Existing pins and thresholds were not refreshed.


Go: ranked requested-edit reasons

| Rank | Reason | Edit time % | Lost-node share % | Decisions |
|---:|---|---:|---:|---:|
| 1 | multiple live stacks: reuse not offered | 67.623 | 97.350 | 376,425 |
| 2 | fresh recovery verification | 15.316 | 0.000 | 16 |
| 3 | reparse / rebuild / dispatch | 3.960 | 0.000 | 0 |
| 4 | reuse selection / cursor overhead | 0.909 | 0.000 | 0 |
| 5 | accepted-error retry | 0.660 | 0.000 | 0 |
| 6 | other subtree eligibility / resume guard | 0.306 | 0.000 | 59,414 |
| 7 | leaf lexical / shift / goto compatibility guard | 0.213 | 1.235 | 50,597 |
| 8 | Tree.Edit | 0.188 | 0.000 | 0 |
| 9 | cursor: already passed | 0.116 | 0.000 | 21,759 |
| 10 | source-byte / ancestor edit guard | 0.094 | 0.042 | 186 |
| 11 | other cursor filter / eligibility guard | 0.075 | 0.018 | 19,752 |
| 12 | top-level scope guard | 0.073 | 1.212 | 10,267 |
| 13 | scanner checkpoint / leaf-only guard | 0.072 | 0.000 | 19,143 |
| 14 | missing-node dependency proof | 0.043 | 0.000 | 6,710 |
| 15 | fragile-subtree guard | 0.005 | 0.107 | 1,945 |
| 16 | parser ownership / pre-goto frontier guard | 0.002 | 0.001 | 318 |
| 17 | reuseNonLeafTargetStateOnStack/n.ChildCount() == 0 | 0.001 | 0.000 | 396 |
| 18 | reuseNonLeafTargetStateOnStack/n == nil | 0.001 | 0.000 | 396 |
| 19 | reuseNonLeafTargetStateOnStack/end_of_candidates | 0.001 | 0.000 | 217 |
| 20 | lexical dependency / edited-leaf proof | 0.001 | 0.000 | 140 |
| 21 | reuseNonLeafTargetStateOnStack/s == nil | 0.001 | 0.000 | 396 |
| 22 | tree rebuilding / normalization | 0.001 | 0.000 | 0 |
| 23 | attemptCompactIncrementalParse/reuseModes&sched.OldTreeReuse != 0 || p.recoveryInitialOnly || !p.admissionCandidateFullParseEligible(nil, true) | 0.001 | 0.000 | 18 |
| 24 | dirty-node guard | 0.001 | 0.034 | 57 |
| 25 | compact attempt / admission | 0.000 | 0.000 | 0 |
| 26 | reuseNonLeafTargetStateOnStack/goto_state | 0.000 | 0.000 | 40 |

JavaScript: ranked requested-edit reasons

| Rank | Reason | Edit time % | Lost-node share % | Decisions |
|---:|---|---:|---:|---:|
| 1 | reparse / rebuild / dispatch | 16.898 | 0.000 | 0 |
| 2 | reuse selection / cursor overhead | 14.256 | 0.000 | 0 |
| 3 | fresh recovery verification | 11.234 | 0.000 | 8 |
| 4 | dispatch/eof_lookahead: reuse not offered | 4.017 | 0.000 | 0 |
| 5 | leaf lexical / shift / goto compatibility guard | 3.386 | 47.857 | 229,657 |
| 6 | other subtree eligibility / resume guard | 3.255 | 0.012 | 266,853 |
| 7 | scanner checkpoint / leaf-only guard | 1.137 | 0.000 | 81,218 |
| 8 | source-byte / ancestor edit guard | 1.004 | 0.750 | 210 |
| 9 | multiple live stacks: reuse not offered | 0.816 | 4.102 | 1,629 |
| 10 | top-level scope guard | 0.793 | 40.710 | 34,387 |
| 11 | other cursor filter / eligibility guard | 0.727 | 0.595 | 61,320 |
| 12 | parser ownership / pre-goto frontier guard | 0.619 | 5.851 | 23,898 |
| 13 | Tree.Edit | 0.291 | 0.000 | 0 |
| 14 | reuseNonLeafTargetStateOnStack/n.ChildCount() == 0 | 0.124 | 0.000 | 12,608 |
| 15 | reuseNonLeafTargetStateOnStack/n == nil | 0.123 | 0.000 | 12,608 |
| 16 | reuseNonLeafTargetStateOnStack/s == nil | 0.122 | 0.000 | 12,608 |
| 17 | accepted-error retry | 0.107 | 0.000 | 0 |
| 18 | missing-node dependency proof | 0.094 | 0.000 | 4,653 |
| 19 | cursor: already passed | 0.074 | 0.000 | 6,092 |
| 20 | reuseNonLeafTargetStateOnStack/end_of_candidates | 0.010 | 0.000 | 659 |
| 21 | tree rebuilding / normalization | 0.003 | 0.000 | 0 |
| 22 | lexical dependency / edited-leaf proof | 0.003 | 0.000 | 120 |
| 23 | attemptCompactIncrementalParse/reuseModes&sched.OldTreeReuse != 0 || p.recoveryInitialOnly || !p.admissionCandidateFullParseEligible(nil, true) | 0.002 | 0.000 | 18 |
| 24 | dirty-node guard | 0.002 | 0.122 | 48 |
| 25 | compact attempt / admission | 0.001 | 0.000 | 0 |
| 26 | dispatch/eof_lookahead | 0.000 | 0.000 | 48 |

TypeScript: ranked requested-edit reasons

| Rank | Reason | Edit time % | Lost-node share % | Decisions |
|---:|---|---:|---:|---:|
| 1 | reparse / rebuild / dispatch | 17.123 | 0.000 | 0 |
| 2 | reuse selection / cursor overhead | 12.007 | 0.000 | 0 |
| 3 | fresh recovery verification | 11.301 | 0.000 | 12 |
| 4 | dispatch/eof_lookahead: reuse not offered | 4.720 | 0.000 | 0 |
| 5 | multiple live stacks: reuse not offered | 3.891 | 73.477 | 42,220 |
| 6 | other subtree eligibility / resume guard | 2.910 | 0.000 | 1,853,941 |
| 7 | leaf lexical / shift / goto compatibility guard | 2.423 | 4.073 | 1,242,116 |
| 8 | full / memory-budget retry | 1.722 | 18.196 | 1 |
| 9 | other cursor filter / eligibility guard | 1.310 | 0.665 | 585,796 |
| 10 | scanner checkpoint / leaf-only guard | 0.889 | 0.000 | 508,479 |
| 11 | accepted-error retry | 0.861 | 0.000 | 0 |
| 12 | source-byte / ancestor edit guard | 0.745 | 0.370 | 257 |
| 13 | cursor: already passed | 0.619 | 0.000 | 239,848 |
| 14 | Tree.Edit | 0.476 | 0.000 | 0 |
| 15 | parser ownership / pre-goto frontier guard | 0.399 | 0.731 | 205,316 |
| 16 | missing-node dependency proof | 0.274 | 0.000 | 91,601 |
| 17 | top-level scope guard | 0.270 | 1.859 | 112,321 |
| 18 | reuseNonLeafTargetStateOnStack/n == nil | 0.125 | 0.000 | 103,007 |
| 19 | reuseNonLeafTargetStateOnStack/n.ChildCount() == 0 | 0.124 | 0.000 | 103,007 |
| 20 | reuseNonLeafTargetStateOnStack/s == nil | 0.115 | 0.000 | 103,007 |
| 21 | reuseNonLeafTargetStateOnStack/end_of_candidates | 0.001 | 0.000 | 365 |
| 22 | lexical dependency / edited-leaf proof | 0.001 | 0.000 | 150 |
| 23 | tree rebuilding / normalization | 0.000 | 0.000 | 0 |
| 24 | dirty-node guard | 0.000 | 0.587 | 61 |
| 25 | attemptCompactIncrementalParse/reuseModes&sched.OldTreeReuse != 0 || p.recoveryInitialOnly || !p.admissionCandidateFullParseEligible(nil, true) | 0.000 | 0.000 | 18 |
| 26 | compact attempt / admission | 0.000 | 0.000 | 0 |
| 27 | fragile-subtree guard | 0.000 | 0.009 | 56 |
| 28 | fresh fallback: full / memory-budget retry | 0.000 | 0.000 | 0 |
| 29 | not_offered_or_selected | 0.000 | 0.035 | 0 |
| 30 | dispatch/eof_lookahead | 0.000 | 0.000 | 56 |

Python: ranked requested-edit reasons

| Rank | Reason | Edit time % | Lost-node share % | Decisions |
|---:|---|---:|---:|---:|
| 1 | fresh recovery verification | 46.792 | 53.715 | 45 |
| 2 | reparse / rebuild / dispatch | 13.093 | 0.000 | 0 |
| 3 | reuse selection / cursor overhead | 5.644 | 0.000 | 0 |
| 4 | dispatch/eof_lookahead: reuse not offered | 3.274 | 0.000 | 0 |
| 5 | other subtree eligibility / resume guard | 2.821 | 0.000 | 461,924 |
| 6 | multiple live stacks: reuse not offered | 2.309 | 0.704 | 3,611 |
| 7 | leaf lexical / shift / goto compatibility guard | 1.682 | 20.102 | 334,604 |
| 8 | top-level scope guard | 0.553 | 11.034 | 53,934 |
| 9 | scanner checkpoint / leaf-only guard | 0.472 | 0.000 | 108,304 |
| 10 | source-byte / ancestor edit guard | 0.434 | 0.061 | 142 |
| 11 | other cursor filter / eligibility guard | 0.361 | 0.066 | 89,894 |
| 12 | parser ownership / pre-goto frontier guard | 0.288 | 7.122 | 63,773 |
| 13 | canReuseNodeWithExternalScannerCheckpointAtLookahead/end_of_candidates | 0.286 | 0.000 | 32,989 |
| 14 | Tree.Edit | 0.241 | 0.000 | 0 |
| 15 | reuseNonLeafTargetStateOnStack/n.ChildCount() == 0 | 0.111 | 0.000 | 34,236 |
| 16 | reuseNonLeafTargetStateOnStack/n == nil | 0.111 | 0.000 | 34,236 |
| 17 | reuseNonLeafTargetStateOnStack/s == nil | 0.106 | 0.000 | 34,236 |
| 18 | canReuseNodeWithExternalScannerCheckpointAtLookahead/!dts.externalScannerStateAtLookaheadStartMatches(want, lookaheadStart) | 0.048 | 4.657 | 5,725 |
| 19 | cursor: already passed | 0.035 | 0.000 | 7,439 |
| 20 | missing-node dependency proof | 0.024 | 0.000 | 3,308 |
| 21 | canReuseNodeWithExternalScannerCheckpointAtLookahead/node == nil || startState != node.PreGotoState() | 0.019 | 2.412 | 4,892 |
| 22 | reuseNonLeafTargetStateOnStack/end_of_candidates | 0.014 | 0.000 | 2,349 |
| 23 | fragile-subtree guard | 0.002 | 0.100 | 533 |
| 24 | tree rebuilding / normalization | 0.001 | 0.000 | 0 |
| 25 | attemptCompactIncrementalParse/reuseModes&sched.OldTreeReuse != 0 || p.recoveryInitialOnly || !p.admissionCandidateFullParseEligible(nil, true) | 0.001 | 0.000 | 18 |
| 26 | lexical dependency / edited-leaf proof | 0.001 | 0.000 | 90 |
| 27 | accepted-error retry | 0.001 | 0.000 | 0 |
| 28 | fresh fallback: fresh recovery verification | 0.000 | 0.000 | 0 |
| 29 | dirty-node guard | 0.000 | 0.016 | 38 |
| 30 | compact attempt / admission | 0.000 | 0.000 | 0 |
| 31 | reuseNonLeafTargetStateOnStack/goto_state | 0.000 | 0.011 | 13 |
| 32 | dispatch/eof_lookahead | 0.000 | 0.000 | 34 |

Rust: ranked requested-edit reasons

| Rank | Reason | Edit time % | Lost-node share % | Decisions |
|---:|---|---:|---:|---:|
| 1 | full / memory-budget retry | 26.613 | 0.000 | 0 |
| 2 | reparse / rebuild / dispatch | 16.848 | 0.000 | 0 |
| 3 | reuse selection / cursor overhead | 7.547 | 0.000 | 0 |
| 4 | other subtree eligibility / resume guard | 3.722 | 0.000 | 1,663,175 |
| 5 | multiple live stacks: reuse not offered | 3.502 | 12.815 | 33,274 |
| 6 | leaf lexical / shift / goto compatibility guard | 2.676 | 45.197 | 1,361,708 |
| 7 | fresh recovery verification | 1.951 | 0.000 | 4 |
| 8 | dispatch/eof_lookahead: reuse not offered | 1.517 | 0.000 | 0 |
| 9 | top-level scope guard | 0.843 | 23.461 | 205,479 |
| 10 | source-byte / ancestor edit guard | 0.789 | 0.122 | 358 |
| 11 | scanner checkpoint / leaf-only guard | 0.625 | 0.000 | 374,200 |
| 12 | other cursor filter / eligibility guard | 0.520 | 0.075 | 367,068 |
| 13 | canReuseNodeWithExternalScannerCheckpointAtLookahead/end_of_candidates | 0.466 | 0.000 | 149,848 |
| 14 | parser ownership / pre-goto frontier guard | 0.345 | 15.169 | 203,600 |
| 15 | accepted-error retry | 0.276 | 0.000 | 0 |
| 16 | reuseNonLeafTargetStateOnStack/n.ChildCount() == 0 | 0.128 | 0.000 | 106,000 |
| 17 | reuseNonLeafTargetStateOnStack/n == nil | 0.128 | 0.000 | 106,000 |
| 18 | reuseNonLeafTargetStateOnStack/s == nil | 0.119 | 0.000 | 106,000 |
| 19 | Tree.Edit | 0.075 | 0.000 | 0 |
| 20 | cursor: already passed | 0.051 | 0.000 | 35,556 |
| 21 | canReuseNodeWithExternalScannerCheckpointAtLookahead/node == nil || startState != node.PreGotoState() | 0.027 | 3.140 | 19,587 |
| 22 | reuseNonLeafTargetStateOnStack/end_of_candidates | 0.012 | 0.000 | 4,227 |
| 23 | missing-node dependency proof | 0.009 | 0.000 | 3,636 |
| 24 | lexical dependency / edited-leaf proof | 0.001 | 0.000 | 170 |
| 25 | tree rebuilding / normalization | 0.000 | 0.000 | 0 |
| 26 | dirty-node guard | 0.000 | 0.022 | 74 |
| 27 | attemptCompactIncrementalParse/reuseModes&sched.OldTreeReuse != 0 || p.recoveryInitialOnly || !p.admissionCandidateFullParseEligible(nil, true) | 0.000 | 0.000 | 18 |
| 28 | compact attempt / admission | 0.000 | 0.000 | 0 |
| 29 | dispatch/eof_lookahead | 0.000 | 0.000 | 1,552 |

Java: ranked requested-edit reasons

| Rank | Reason | Edit time % | Lost-node share % | Decisions |
|---:|---|---:|---:|---:|
| 1 | full / memory-budget retry | 77.649 | 0.000 | 0 |
| 2 | multiple live stacks: reuse not offered | 19.130 | 97.857 | 240,304 |
| 3 | reparse / rebuild / dispatch | 1.164 | 0.000 | 0 |
| 4 | reuse selection / cursor overhead | 0.123 | 0.000 | 0 |
| 5 | Tree.Edit | 0.103 | 0.000 | 0 |
| 6 | other subtree eligibility / resume guard | 0.043 | 0.000 | 34,977 |
| 7 | leaf lexical / shift / goto compatibility guard | 0.030 | 1.025 | 26,971 |
| 8 | cursor: already passed | 0.014 | 0.000 | 9,933 |
| 9 | source-byte / ancestor edit guard | 0.013 | 0.045 | 71 |
| 10 | other cursor filter / eligibility guard | 0.012 | 0.008 | 8,380 |
| 11 | missing-node dependency proof | 0.011 | 0.000 | 6,142 |
| 12 | top-level scope guard | 0.008 | 0.828 | 4,814 |
| 13 | scanner checkpoint / leaf-only guard | 0.007 | 0.000 | 6,516 |
| 14 | canReuseNodeWithExternalScannerCheckpointAtLookahead/dts == nil | 0.003 | 0.000 | 4,286 |
| 15 | parser ownership / pre-goto frontier guard | 0.002 | 0.070 | 2,202 |
| 16 | dispatch/eof_lookahead: reuse not offered | 0.001 | 0.000 | 0 |
| 17 | reuseNonLeafTargetStateOnStack/n == nil | 0.001 | 0.000 | 1,191 |
| 18 | reuseNonLeafTargetStateOnStack/s == nil | 0.001 | 0.000 | 1,191 |
| 19 | reuseNonLeafTargetStateOnStack/n.ChildCount() == 0 | 0.001 | 0.000 | 1,191 |
| 20 | fragile-subtree guard | 0.001 | 0.149 | 921 |
| 21 | tree rebuilding / normalization | 0.000 | 0.000 | 0 |
| 22 | lexical dependency / edited-leaf proof | 0.000 | 0.000 | 90 |
| 23 | reuseNonLeafTargetStateOnStack/end_of_candidates | 0.000 | 0.000 | 90 |
| 24 | dirty-node guard | 0.000 | 0.018 | 31 |
| 25 | dispatch/eof_lookahead | 0.000 | 0.000 | 2 |

C#: ranked requested-edit reasons

| Rank | Reason | Edit time % | Lost-node share % | Decisions |
|---:|---|---:|---:|---:|
| 1 | unsupported external scanner: fresh fallback | 99.982 | 100.000 | 18 |
| 2 | Tree.Edit | 0.012 | 0.000 | 0 |
| 3 | missing-node dependency proof | 0.002 | 0.000 | 3,737 |
| 4 | reparse / rebuild / dispatch | 0.001 | 0.000 | 0 |
| 5 | tree rebuilding / normalization | 0.000 | 0.000 | 0 |
| 6 | attemptCompactIncrementalParse/reuseModes&sched.OldTreeReuse != 0 || p.recoveryInitialOnly || !p.admissionCandidateFullParseEligible(nil, true) | 0.000 | 0.000 | 18 |
| 7 | lexical dependency / edited-leaf proof | 0.000 | 0.000 | 90 |
| 8 | accepted-error retry | 0.000 | 0.000 | 0 |
| 9 | compact attempt / admission | 0.000 | 0.000 | 0 |

PowerShell*: ranked requested-edit reasons

| Rank | Reason | Edit time % | Lost-node share % | Decisions |
|---:|---|---:|---:|---:|
| 1 | fresh recovery verification | 69.176 | 0.000 | 81 |
| 2 | full / memory-budget retry | 16.593 | 0.000 | 0 |
| 3 | multiple live stacks: reuse not offered | 2.596 | 15.067 | 36,243 |
| 4 | reparse / rebuild / dispatch | 1.811 | 0.000 | 0 |
| 5 | reuse selection / cursor overhead | 1.065 | 0.000 | 0 |
| 6 | other subtree eligibility / resume guard | 0.788 | 13.455 | 1,877,143 |
| 7 | dispatch/eof_lookahead: reuse not offered | 0.692 | 0.000 | 0 |
| 8 | top-level scope guard | 0.321 | 27.306 | 370,638 |
| 9 | leaf lexical / shift / goto compatibility guard | 0.238 | 16.185 | 359,512 |
| 10 | accepted-error retry | 0.214 | 0.000 | 0 |
| 11 | scanner checkpoint / leaf-only guard | 0.142 | 0.000 | 345,919 |
| 12 | source-byte / ancestor edit guard | 0.141 | 0.266 | 558 |
| 13 | parser ownership / pre-goto frontier guard | 0.074 | 7.732 | 145,604 |
| 14 | other cursor filter / eligibility guard | 0.041 | 0.131 | 114,816 |
| 15 | Tree.Edit | 0.032 | 0.000 | 0 |
| 16 | reuseNonLeafTargetStateOnStack/n.ChildCount() == 0 | 0.028 | 0.000 | 80,107 |
| 17 | reuseNonLeafTargetStateOnStack/n == nil | 0.028 | 0.000 | 80,107 |
| 18 | reuseNonLeafTargetStateOnStack/s == nil | 0.028 | 0.000 | 80,107 |
| 19 | missing-node dependency proof | 0.024 | 0.000 | 15,326 |
| 20 | fragile-subtree guard | 0.022 | 18.898 | 62,997 |
| 21 | cursor: already passed | 0.019 | 0.000 | 40,349 |
| 22 | reuseNonLeafTargetStateOnStack/end_of_candidates | 0.005 | 0.000 | 7,305 |
| 23 | error-bearing node guard | 0.003 | 0.926 | 3,009 |
| 24 | lexical dependency / edited-leaf proof | 0.000 | 0.000 | 180 |
| 25 | tree rebuilding / normalization | 0.000 | 0.000 | 0 |
| 26 | attemptCompactIncrementalParse/reuseModes&sched.OldTreeReuse != 0 || p.recoveryInitialOnly || !p.admissionCandidateFullParseEligible(nil, true) | 0.000 | 0.000 | 18 |
| 27 | fresh fallback: fresh recovery verification | 0.000 | 0.000 | 0 |
| 28 | dirty-node guard | 0.000 | 0.033 | 86 |
| 29 | compact attempt / admission | 0.000 | 0.000 | 0 |
| 30 | dispatch/eof_lookahead | 0.000 | 0.000 | 70 |

`summary.json` also contains exact reason labels, every edit’s runtime/digests/verdicts, 192 complete cell summaries, per-class/per-size ranks, event counts, C artifact identities, and SHA-256 for each unabridged raw JSONL. Raw event arrays remain in local campaign artifacts; they are not embedded in the checked-in summary. The external corpus lock remains outside the repository.

Measurements ran locally because access to the requested benchmark VM failed for missing cloud authentication scopes. Other jobs shared the VM; CPU affinity separated the accepted timing run. An initial Go run that shared CPU 7 (~50% per process) was rejected and all 20 seeds rerun on CPU 5. No new optimization speedup is claimed. The requested 1 MiB memory contract and the owner’s original fixture timings were not remeasured here.

Reproduction uses `scripts/run_incremental_reuse_census.sh` and the comparison/reducer scripts described in [the method](../../incremental-reuse-census.md). Baseline `f9828512` is augmented only with the census test/benchmark harness, the unobserved identity adapter, the fixture manifest and the internal recorder package; production baseline engine files remain original. PowerShell additionally applies the exact existing dependency patch, SHA-256 recorded in `summary.json`. All performance comparisons use `scripts/run_randomized_benchmarks.sh`, GOMAXPROCS=1, 20 explicit seeds, one seed per process, count=1, 750 ms and benchmem.
