# Go incremental merge candidate

Status: measured experiment. Runtime changes are saved as a patch, not adopted.
Compact parser graduation remains open.

## Result

The early-newline fixed history improved from 1.402 seconds to 220.0 ms.
Each operation creates the initial tree, performs four alternating edits,
and releases the final tree. The parser and loaded grammar remain alive
between operations. Initial parse, edits, profiled incremental parsing,
and tree release are timed. No edit is warmed away.

| Metric per fixed history | Baseline median | Candidate median | Change |
| --- | ---: | ---: | ---: |
| Wall time | 1.402 s | 220.0 ms | -84.31% |
| Go allocation | 514.28 MiB | 80.22 MiB | -84.40% |
| Allocations | 229,870 | 21,850 | -90.50% |
| Reported edit nodes | 1,326,636 | 13,284 | -99.00% |
| Reported edit tokens | 183,788 | 4,744 | -97.42% |
| Recovery fallbacks | 1 | 0 | -100% |
| Reused bytes across edits | 517,840 | 880,117 | +69.96% |

Rounded values come from benchstat. Raw rows and exact medians are retained.
Node and token metrics sum the four incremental profiles; they exclude the
initial parse. Wall time and Go allocation include it. These counters do
not establish complete graph work, retained memory, or native allocation.
No RSS or retention improvement is claimed.

## Candidate

Baseline: 346117d6a5a570d4fe4aa5c95454bf7deca37c6d.
The candidate sets the implicit Go incremental merge cap to one and enables
graph-preserving cap-one convergence. Explicit merge-width settings remain
available. The experimental patch also removes the fresh-only restriction
from existing convergence opt-ins. That broader change must be narrowed
before adoption; the measurements here cover Go only.

Patch: harness_out/incremental-merge-isolation/default-experiment.patch.
SHA-256: a20c790beff188ff641fcf7030ceecacb6630f22658439d53a007c675485fa2a.
The live runtime diff matched this hash after both benchmark campaigns.

Go does not have FullParseGSSConvergenceEnabled in the checked-in runtime
profile. The only enabled profile found is C#. Go's fresh merge policy uses
a separate optional global faithful-convergence setting. Do not infer an
incremental certification from the C# profile or from the fresh policy.

## Correctness controls

Setting cap one only after a default initial parse fixed both the full
235,626-byte newline witness and the 65,953-byte prefix shape witness.
Four representative canonical Go histories then passed four edits each
against fresh C tree digests. Graph-preserving convergence passed the same
four histories. The implicit-default candidate also passed those histories.

Additional candidate checks passed: compact growth, comment, same-width,
shrink and multiline edits; repeated same-width edits; all six canonical
Go cases; missing-brace edits from production and compact old trees against
fresh and incremental C; included ranges; and the Go parity incremental
case. No Python case was run in the Go process.

The initial native compact attempt still declines where it requires one
clean shared-lexer version. The improvement is in the legacy incremental
continuation. It does not graduate native compact reuse.

## Measurement protocol

Both campaigns use 20 seeds, 750 ms calibration, paired alternating checkout
order, build-once mode, and a fresh binary process for each seed and lane.
Docker uses Go 1.25.14, GOMAXPROCS=1, CPU 18, one CPU, and an 8 GiB limit.
All 40 rows per campaign completed without timeout or OOM.

The original calibrated single-edit benchmark improved from 531.445 ms/op
to 7.968 ms/op. However, baseline samples execute 2–3 edits and candidate
samples execute 97–111. Their startup amortization differs. Use the fixed
history result above for a comparison with equal edit sequences.

The fixed-history probe uses identical benchmark source in both checkouts.
It creates an initial tree for every operation, so each operation always
contains exactly four edits. Tree-digest validation runs in the separate
correctness controls, outside this timing benchmark.

The runner records binary hashes before and after execution. Nested-module
Git metadata is unavailable inside the container. The recorded baseline
revision and saved runtime patch identify the experiment; there was no
complete pre-run source manifest for this campaign. The prior campaign's
different source manifest must not be presented as authentication for this
one.

## Next gate

Narrow the convergence change to the intended Go policy and add policy
precedence tests. Validate the wider Go conflict and repeated-edit surface.
Compare all representative workloads, not only early_newline. Measure
first-forward, first-reverse and later-edit costs separately, including
retained memory and live old/new tree overlap. Preserve explicit overrides
and the compact fallback receipts.

## Evidence and cleanup

Remote evidence: harness_out/incremental-merge-isolation. It contains the
probe sources, exact patches, JSON test output, raw benchmark rows, runner
commands, process accounting, benchstat results and Docker receipts.
Temporary runtime edits and compiled-package probes are removed after
capture. The baseline checkout is retained for follow-up comparisons.
