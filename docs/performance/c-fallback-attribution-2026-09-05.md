# C incremental fallback attribution

Baseline: `b3a5f9d4129cdb255e01053e6ebf5a7d188fe4b5`. Date: September 5, 2026.

The 137 KiB C fixture still performs an expensive incremental attempt before its memory-budget fallback.
All three incremental results match fresh Go trees by deep digest.
This run does not establish locked-C parity or a performance comparison.

## Observed work

| Edit | Reused subtrees | Reused bytes | Incremental tokens | Fresh tokens |
| --- | ---: | ---: | ---: | ---: |
| Replace | 2 | 47 | 41,729 | 41,746 |
| Insert | 2 | 47 | 41,729 | 41,746 |
| Delete | 4 | 18 | 41,751 reported | 41,746 |

Replace and insert rebuild almost the whole file despite reporting reuse.
The delete profile combines counters from different attempts.

The abandoned delete attempt records:

- Stop reason: `memory_budget`.
- Allocated nodes: 3,165,354.
- Consumed tokens: 15,583.
- Arena storage: 539,573,680 bytes.
- Scratch storage: 215,488,076 bytes.
- Parser loop: 16.30 seconds in this diagnostic run.

The final fallback allocates 205,934 nodes and consumes 41,746 tokens.
The complete incremental call takes 16.71 seconds. A separate fresh call takes 343.8 milliseconds.
The explicit fresh compact attempt declines after approximately 0.73 milliseconds.
The main observed cost therefore precedes the legacy memory-budget fallback.

These timings include diagnostic instrumentation and one fixed-order sample.
Do not use them as benchmark evidence or as a speedup claim.
Maximum resident set size is 1,062,712 KiB for the test command, including compilation and all edits.
Docker completed without an out-of-memory event or timeout.

## Attribution defect

The retry calls `copyParseRuntimeToTiming` with the replacement tree.
This overwrites token, storage, and phase counters from the abandoned attempt.
The public profile reports 39,558,952 arena bytes, 22,812,236 scratch bytes, and zero parser-loop time.
Its new-node counter retains the abandoned attempt instead of including the final fallback.
Elapsed reparse time includes both attempts.
These mixed meanings prevent reliable work attribution from the public profile alone.

## Next implementation

Reduce the abandoned incremental work after the arena change completes review.
First preserve the smallest source that triggers the excessive work.
Then test a generic early decline or invalid-reuse correction against the current fallback result.
Preserve the memory-budget contract and successful replace/insert reuse.
Keep this change separate from compact recovery selection.

Require exact fresh/incremental results and locked-C comparisons for the reduced witness.
Measure complete-operation work before running randomized timing comparisons.
Track telemetry corrections under issue #1052 and the editor cost under issue #454.

## Reproduction

Read the [summary](c-fallback-attribution-2026-09-05/summary.json) and adjacent logs.
Check `SHA256SUMS` before reproducing the diagnostic.

Create an isolated checkout at the recorded baseline.
Apply `diagnostic.patch` there. It adds logging only.
Run `TestIssue454CIncrementalDeleteMatchesFresh` in Docker with Go 1.25.14 and `GOMAXPROCS=1`.
Use one CPU, a 4 GiB container limit, `GOMEMLIMIT=3GiB`, and a four-minute test timeout.
The original run used equivalent file overlays. The adjacent scripts preserve those paths.
The full local archive is `/home/draco/work/gts-authenticated-edit-optimization-evidence-20260905/c-fallback-attribution`.
