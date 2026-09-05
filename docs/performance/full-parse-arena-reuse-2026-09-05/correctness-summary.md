# Correctness record

The initial baseline run failed one stale ownership assertion.
The test expected a new root after a same-width edit.
Authenticated token reuse can now retain that root.
Preserve the failed output in baseline-correctness-original-failure.txt.

Apply the exact incremental_test.go from merged commit d9916dba through an identical overlay for both sources.
That test changes edit width to exercise distinct primary and borrowed arenas.
The corrected run passed 48 baseline tests and 52 candidate tests.
The four new controls cover retained capacity, insufficient capacity, live arena rejection, and repeated parser ownership.
Existing controls cover clearing, retained capacity limits, eviction, byte accounting, budget checks, and frozen fixture digests.

Separate containers passed Go repair and reuse checks against C on both parser routes.
Separate JSON and JavaScript containers passed fresh and incremental structural parity without skips.
These cases do not establish universal language parity.

The correctness overlay changes tests only.
The randomized timing scripts do not use this overlay.
The source reviewer found no actionable defect before correctness began.
That review did not include runtime measurements.
