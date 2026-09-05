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

## Reduced work witness

A separate size-series diagnostic preserves complete functions and stops after the first memory-budget fallback.
Every incremental digest matches fresh Go parsing.

| Functions | Source bytes | Reported incremental new nodes | Fresh result allocated nodes | Fallback |
| --- | ---: | ---: | ---: | --- |
| 1 | 40 | 130 | 66 | none |
| 4 | 160 | 826 | 288 | none |
| 16 | 664 | 5,394 | 1,176 | none |
| 64 | 2,776 | 35,250 | 4,728 | none |
| 256 | 11,848 | 338,994 | 18,936 | none |
| 1,024 | 48,808 | 2,561,571 | 75,768 | full retry |
| 2,048 | 102,056 | 3,213,594 | 151,544 | memory-budget full retry |

These counters retain the scope limitations described above.
The 664-byte source provides a small regression for excessive work without reaching a memory limit.
It is not the smallest possible witness. The 102,056-byte source retains the memory-budget fallback.

Read [the reduced source](c-fallback-attribution-2026-09-05/c-delete-16-functions.c) and [size-series receipts](c-fallback-attribution-2026-09-05/reduction-summary.json).
Delete the first character of `x0` to reproduce the edit.
Apply `reduction-test.patch` after `diagnostic.patch` and run `TestCDeleteFallbackReductionDiagnostic` with the same Docker limits.
The run completed without memory failure or timeout. Locked-C verification remains separate.

## Locked-C check for the reduced witness

A separate C container checked the 664-byte witness against locked C.
It preserves the known first divergence from the 1 KiB issue #454 ratchet:

- Path: `/translation_unit/function_definition[0]/compound_statement[2]/ERROR[2]/number_literal[0]`.
- Category: `error`.
- Go value: `true`.
- C value: `false`.

The test passed by preserving that known difference. It does not establish parity.
Keep this difference explicit when validating the next optimization.
The reduced source is a work regression, not a completed correctness certificate.

Apply `c664-oracle.patch` to the recorded baseline in an isolated checkout.
Run `TestIssue454C664ByteLockedCDiagnostic` under `cgo_harness` with the `treesitter_c_parity` tag in Docker.
The container completed without memory failure or timeout.
Read the adjacent `c664-oracle.txt` receipt.
