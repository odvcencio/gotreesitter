# Compact parser parent publication follow-up — 2026-09-10

Status: the targeted publication fix is ready for review. General compact replacement remains on HOLD.

## Trigger

The canonical Go deletion history exposed a stale parent link after an incremental result was selected. A recovered integer leaf at the deletion boundary still pointed at an ERROR parent from a discarded speculative branch. The wrong link was visible before and after releasing the prior tree, so this was a result-publication defect rather than an ownership-release defect.

The same shape reproduced in the compact and legacy lanes. The compact lane made the cause easier to isolate: the selected result borrowed the leaf from an older arena, while the discarded branch had most recently written that leaf's parent field. Deferring a parent walk on the newly allocated result arena could not repair a link stored on the borrowed node's arena.

## Fix

The result-root builder now wires parent links from the selected root for incremental results as well as fresh results. The walk crosses borrowed children, so every borrowed node is rebound to the selected parent before the result is exposed. Language-specific deferred-parent modes remain governed by their existing policy.

The parser-core driver no longer immediately defers the selected incremental root after the builder has published those links. That removed a redundant second walk on the first parent or sibling access while preserving the existing cancellation fallback and arena ownership rules.

The maintained tests cover both halves of the ownership boundary:

- A root unit fixture builds selected and discarded parents that share either a fresh child or a borrowed child. The selected child must point at the selected parent.
- The borrowed-arena materialization test checks that the selected parent is published and that both arenas remain alive after the old tree is released.
- The canonical deletion history checks fresh-C parity and walks every returned parent link after releasing the previous tree.

## Verification

All runs used the repository Docker harness with one pinned CPU (CPU 18), GOMAXPROCS=1, and the Go 1.25 image.

- 'go test -tags gts_parsercorephase0 ./internal/parsercorephase0' passed.
- The focused root suite covering borrowed materialization, navigation, and selected-parent publication passed.
- The focused CGO suite passed TestGoCompactTwentyNativeEditsLockedC, TestGoDeletionHistoryNavigationLockedC, and TestGoCompactSuffixReuseUnderFragileFunctionLockedC.
- The affected comparison retained 423 baseline pass outcomes and 32 baseline failures; the candidate had 425 pass outcomes and 30 failures. The only existing outcome changes were the two deletion-navigation failures, which now pass. The new selected-parent unit test also passes on the candidate and fails on the c913c490 control.
- The broad root package still has the pre-existing scheduler, recovery, forest, language-corpus, and fixture failures recorded in the Docker receipt. No new failure was introduced by this fix.

The deletion history is now structurally sound as a returned tree, but it is still a legacy recovery execution. Its compact incremental profile reports no native edits because the current materializer deliberately declines recovery through borrowed subtrees.

## Parent-fix cost comparison

The implementation was compared with c913c490c5caf7c41138310fbfefddd7c6758694, the immediately preceding compact candidate. Both lanes used the same compact four-edit lifecycle and the same 20 paired seeds. Parser and grammar construction were outside the timer; initial parsing, four edits, all releases, and benchmark calibration were inside it.

Ratios are geometric means of paired candidate/control measurements. The 95% intervals use 50,000 paired bootstrap resamples with PRNG seed 20260909. The comparison describes this pinned host and process protocol. It is an attribution control, not a replacement gate.

| History | c913 control median ms | Parent-fix median ms | Candidate/control ratio [95% CI] | Allocated-byte ratio | Allocation-count ratio |
| --- | ---: | ---: | ---: | ---: | ---: |
| Token-class change | 21.508 | 21.714 | 1.013 [0.963, 1.070] | 1.026 [0.999, 1.056] | 1.001 |
| Length change | 25.712 | 22.615 | 0.987 [0.945, 1.034] | 1.001 [0.975, 1.026] | 1.000 |
| Early newline | 229.880 | 244.811 | 0.993 [0.940, 1.049] | 1.010 [0.957, 1.066] | 1.000 |
| Prefix call/conversion | 54.527 | 56.157 | 0.998 [0.942, 1.069] | 0.987 [0.927, 1.052] | 1.000 |
| Deletion recovery | 12.337 | 11.534 | 0.998 [0.925, 1.105] | 1.000 [0.999, 1.000] | 1.000 |

Benchstat found no significant timing or allocation-count change in any history. Work receipts remained identical: the four native histories consumed the same token, node, certificate, and reused-byte counts in both lanes; deletion remained a one-decline legacy history. The parent walk changes publication and access correctness, not scheduler work.

The required lifecycle comparison against repaired legacy remains the stronger gate. That run is recorded in compact-suffix-reuse-2026-09-09.md: all four native histories stay below the 1.10 wall-time ceiling, and the deletion fallback meets the ceiling while remaining non-native.

## Remaining graduation gate

The finite next implementation target is native deletion recovery through borrowed derivations. The scheduler must preserve selected recovery lineage, state and token proof, arena ownership, and public parent projection when a recovered branch contains borrowed subtrees. The current guard in materialization rejects this combination intentionally; removing it without a proof would reintroduce the stale-link class of bug.

Scanner checkpoint transfer, included-range recovery, broader language coverage, and the retirement authorization boundary remain open. No legacy entry point was removed.

## Evidence

The raw parent-fix comparison, root and CGO receipts, source copies of removed diagnostics, and control metadata are in:

- harness_out/deletion-recovery-20260909/
- harness_out/deletion-recovery-20260909/sources/
- harness_out/reuse-eligibility-20260909/
- harness_out/compact-suffix-reuse-20260909.zip

The removed milestone and attribution probes remain under sources/diagnostics/; they are not production test inputs.
