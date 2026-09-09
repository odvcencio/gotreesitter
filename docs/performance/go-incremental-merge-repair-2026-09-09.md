# Go repeated incremental merge repair

The implicit Go incremental merge cap is one. Tied cap-one alternatives
remain in the graph. Explicit wider caps and retry overrides retain their
precedence. The convergence change applies only to Go incremental parses;
fresh parses and other languages keep their existing policies.

This fixes two repeated-edit regressions: a clean call/conversion tree
mismatch in the 65,953-byte prefix, and a forest recovery fallback in the
235,626-byte early-newline fixture. The new regression test fails on the
baseline and passes with this change.

## Paired fixed-history performance

Each operation creates the initial tree, performs four alternating edits,
and releases the final tree. A parser and loaded grammar remain alive
between operations. Time and allocation include initial parsing and tree
release; edit node/token counters sum only the incremental profiles.

| History | Baseline ms | Changed ms | Time change | Baseline MiB allocated | Changed MiB allocated |
| --- | ---: | ---: | ---: | ---: | ---: |
| token_class_change | 158.83 | 22.06 | -86.11% | 35.70 | 5.62 |
| same_line_length_change | 156.41 | 27.02 | -82.73% | 43.48 | 16.77 |
| early_newline | 1528.96 | 244.92 | -83.98% | 472.46 | 80.57 |
| recovery_deletion | 35.92 | 10.58 | -70.55% | 7.68 | 2.60 |

All four time comparisons have benchstat p < 0.001, n=20 per lane.
Each workload has the same four-edit sequence in both lanes. Different
calibrated operation counts cannot change the mix of first and later edits.

Reported edit nodes fall from 185,885 to 2,670 for token changes; 175,610
to 1,940 for length changes; 1,326,636 to 13,284 for newline edits; and
40,292 to 4,919 for deletion recovery. The newline history drops from one
forest recovery fallback to zero. Other measured histories have zero on
both lanes.

These measurements use the profiled API. They do not establish complete
graph work, retained heap, peak old/new overlap, or an RSS improvement.
All four workloads run in the same per-seed process, with separate parsers.
Shared pools can affect absolute allocation versus a standalone workload.
Do not splice these rows into the prior single-workload campaign.

## Protocol and source identity

Baseline: 346117d6a5a570d4fe4aa5c95454bf7deca37c6d.
Candidate starts at 0133ac42c43e03a953cd4abefc5fc077a5ae467d plus the saved
runtime/test diff. Identical new benchmark source is present in both lanes.
Twenty seeds, 750 ms calibration, alternating checkout order, build-once
mode, and a fresh process per seed and lane produce 160 validated rows.
Docker uses Go 1.25.14, GOMAXPROCS=1, CPU 18, one CPU and an 8 GiB limit.
The campaign completes without timeout or OOM.

Before measurement, the manifest hashes 3,165 candidate files and 3,162
baseline files. Every recorded hash matches after measurement. Binary
hashes before and after execution are recorded by the runner. Later edits
only adapt internal policy tests and add this report; runtime and benchmark
source remain the measured versions.

## Correctness and limits

The permanent C-oracle regression covers four representative canonical Go
histories and the prefix witness. It performs four edits per history,
compares each returned tree with fresh C, and rejects unsupported reuse.

The Go root incremental selection passes all 37 test/subtest results,
including the invariant sweep. A malformed-input test now requires zero
merge retries while preserving deep-tree equality and old-tree ownership.
It formerly required one retry, which the narrower incremental default
makes unnecessary.

Policy tests cover Go fresh and incremental defaults, explicit one/wider
caps, positive and negative retry overrides, other languages, and fresh-only
certification. Generic retry selection and ownership tests use Dart's
narrower fresh policy, so the adoption and loser-release branches remain
exercised. The final policy selection passes.

The broad Go C harness is not green. Baseline and candidate have identical
statuses for every shared test. Failures remain in forest generic-conflict
election, new(a.b.C), next-live probe/receipt tests, and production included
range recovery. Candidate adds six passing results from the new test and
its five cases. The saved comparison records every status difference.
These existing failures remain graduation work.

Native compact reuse can still decline on its clean-version requirement.
This repair improves the legacy incremental continuation. It does not
certify native compact reuse, memory retention, full route coverage, or
legacy retirement.

## Evidence and next work

Evidence lives in harness_out/go-incremental-merge-fix: red/green tests,
baseline/candidate harness logs, status comparison, root/policy checks,
source manifests, raw timing rows, commands, process accounting and
benchstat results. The temporary baseline benchmark copy is removed after
measurement; the benchmark remains as maintained source in the candidate.

Next: native compact repeated reuse, per-edit startup attribution, retained
memory and old/new overlap, and the remaining forest/recovery failures.
Broader publication and retirement gates stay open.
