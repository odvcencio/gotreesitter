# Compact parser reuse work

The graduation assessment remains HOLD.
Two compiled-dispatch experiments passed focused correctness checks but showed no reliable performance improvement.
Both runtime changes were removed.

## Measured comparison

The baseline is revision 7845de4d, the candidate preserved after the previous milestone.
Each comparison uses 20 paired seeds, fresh processes, one CPU, and CPU affinity 18.
Each operation includes initial compact parsing, four alternating edits, and all tree releases.
Parser construction remains outside timing.
The benchmark uses the profiled incremental API.

CI means confidence interval.
Ratios use geometric means of paired candidate/baseline ratios.
The 95% intervals use 50,000 bootstrap resamples of the 20 pairs.
Intervals describe this host and protocol, not uncertainty across machines.

| History | Looping dispatch ratio [95% CI] | Single-operation dispatch ratio [95% CI] |
| --- | ---: | ---: |
| token_class_change | 1.002 [0.963, 1.044] | 1.033 [0.991, 1.083] |
| early_newline | 1.002 [0.961, 1.043] | 0.997 [0.952, 1.036] |
| newline_prefix_call_conversion | 1.004 [0.963, 1.048] | 0.995 [0.963, 1.026] |
| same_line_length_change | 1.009 [0.973, 1.051] | 1.006 [0.981, 1.039] |
| recovery_deletion | 0.993 [0.952, 1.034] | 0.989 [0.960, 1.018] |

All intervals include 1.0. Benchstat found no timing improvement for either experiment.
Allocation counts did not change.
Allocated-byte ratios varied slightly with arena allocation and benchmark calibration.
The evidence does not justify retaining either runtime change.

Both experiments preserved the exact token, node, subtree-reuse, and byte-reuse counts.
Each experiment passed the native histories with compiled dispatch disabled and enabled.
The test required actual compiled passes when enabled.
The same test failed on the starting revision because incremental parsing disabled that route.

The normal and fused-instruction variants passed the fresh-C and parent-navigation checks.
Focused cancellation, memory-budget, nested-reuse, and scheduler tests passed.
The full core suite passed 793 tests and subtests during the first experiment.
The maintained runtime is restored to 7845de4d, so this pass makes no new heap-retention claim.

## Repeated work is the larger target

The preserved attribution receipts separate each edit.
These counts come from the previous milestone; the new experiments reproduced the compact totals.

| History | Legacy tokens by edit | Compact tokens by edit | Legacy new nodes by edit | Compact new nodes by edit |
| --- | --- | --- | --- | --- |
| Token class | 437, 95, 90, 90 | 630, 630, 630, 630 | 1740, 314, 307, 309 | 1475, 1475, 1475, 1475 |
| Early newline | 3187, 502, 502, 502 | 3485, 3485, 3485, 3485 | 9798, 1162, 1162, 1162 | 7494, 7494, 7494, 7494 |
| Prefix call/conversion | 1324, 223, 223, 223 | 1423, 1423, 1423, 1423 | 4070, 446, 446, 446 | 3054, 3054, 3054, 3054 |

Legacy rebuilds more nodes on its first newline edit, then reuses more on later edits.
Compact repeats the same work across all four edits.
The first-edit cost and the later-edit cost require separate analysis.

Diagnostic traces found 233 rejected nonterminal candidates per token-class edit.
Of these, 231 were below the permitted nesting level.
All 233 failed the recorded dependency check.
These reason counts overlap; they must not be added together.

The newline histories had only 13 rejected nonterminal candidates at that same check.
The early-newline trace also found 26 unchanged top-level functions larger than 100 bytes with fragility flags.
Fragility blocks these candidates before the state check.
The 26-function count excludes smaller nodes.

The source confirms two limits:
- Dependency publication only considers direct children of top-level items.
- Compact materialization preserves a conservative ambiguity flag on public nodes.

These findings identify missing reuse opportunities.
They do not prove that every rejected candidate is safe to borrow.

## Next implementation target

Compare compact and repaired legacy after the first edit.
Identify which rebuilt nodes gain reuse eligibility in legacy.
Record their states, ambiguity decisions, boundaries, and dependency extents.

Separate fragility caused by the node's own reduction from fragility inherited through projection.
Keep recovery, source identity, scanner state, and selected-derivation checks intact.
Add a counterexample test before widening any reuse rule.
Then repeat native parity, parent navigation, and the paired lifecycle comparison.

A successful change must reduce repeated parsing or node construction.
A dispatch-only change has not demonstrated enough value in these two experiments.

## Maintained benchmark

BenchmarkGoCompactFourEditLifecycle covers the three required histories.
Each history has explicit compact and legacy lanes.
The compact lane requires actual native reuse and zero legacy entries.
The legacy lane requires exactly one legacy entry per edit.
Both lanes begin each operation with compact initial parsing.

The benchmark reports:
- Wall-clock time, allocated bytes, and allocation count.
- Initial token and node counts.
- Edited token and node counts.
- Reused bytes across the four edits.

Use scripts/run_randomized_benchmarks.sh for performance comparisons.
Use 20 seeds, 750 ms per benchmark, GOMAXPROCS=1, and pinned Docker execution.
The one-iteration verification receipt only validates execution and counters.
It is not comparative performance evidence.

## Evidence and disposition

harness_out/reuse-work-20260909 contains both comparison rounds.
Each round includes raw samples, Benchstat output, source hashes, patches, and correctness receipts.
The source hashes matched before and after each measurement.

The prior milestone archive remains unchanged.
The decision report now records HOLD and withdraws the unsupported redesign inference.
No runtime optimization from this pass remains enabled.
No legacy entry point was removed.
