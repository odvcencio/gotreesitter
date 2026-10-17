# Cached visible-count walk screen — 2026-10-17

**Rejected:** separating the cold visible-node walk from cached child totals
produces no supported compact Go or YAML throughput improvement in twenty
shuffled pairs. The runtime remains private. No full-fleet, strict-census, RSS,
graduation or route-activation qualification is claimed for this source.

Baseline: `5e52cf2a2644f3d38785513eebc39f8cf8a506b2`, including the qualified
paired-terminal cursors. The earlier unchanged-Go profile put visible-count
work at about **3.64% cumulative CPU**. The candidate moves cache misses into
an internal helper and reads already-cached child totals directly, avoiding
the recursive occurrence-adjustment call for a cache hit. It adds no retained
storage. Root `ERROR_REPEAT`, descendant aliases, extra-node rules, malformed
metadata and predecessor checks, overflow order, policy rebinding and cache
publication after success retain their behavior. None of this establishes a
measured speedup.

## Twenty-pair result

The repository randomized runner uses Go **1.25.14**, `GOMAXPROCS=1`, CPU 6,
6 GiB Docker, `GOMEMLIMIT=5GiB`, `-count=1`, `750ms` and `-benchmem`.
Each seed runs in its own process, baseline/candidate order alternates, and
both benchmark locks stay held. Go seeds are **31101–31120** and YAML seeds
**31201–31220**. All **440 timed rows** are retained.

| Compact path | Before → after | Time direction | p |
| --- | ---: | ---: | ---: |
| Go R4 | 785.663 → 786.183 µs | +0.07% | .718 |
| Go rewrite | 1.704776 → 1.709277 ms | +0.26% | .355 |
| Go query | 8.113928 → 8.117012 ms | +0.04% | .355 |
| Go language | 8.470820 → 8.458640 ms | −0.14% | .698 |
| Go large | 86.708943 → 86.510451 ms | −0.23% | .758 |
| Ordinary YAML attempt plus fallback | 190.278 → 190.579 µs | +0.16% | .764 |
| Owned YAML R4 | 337.472 → 337.041 µs | −0.13% | .941 |
| Owned YAML 8 KiB | 1.309383 → 1.305464 ms | −0.30% | .799 |

All eight differences are inconclusive. Allocation counts remain exact: Go
**22 / 21 / 120 / 239 / 1,705**, YAML **37 / 109 / 336**, in table order.
Go language bytes round **150,418.5 → 150,418 B/op**; this supplies no memory
claim. Owned-stream bytes remain **44,312 B/op**.

| Default Go control | Before → after | Time direction | p |
| --- | ---: | ---: | ---: |
| Full | 3.933556 → 3.912915 ms | −0.52% | .020 |
| First edit | 99.250 → 98.876 µs | −0.38% | .149 |
| No edit | 3.1545 → 3.0230 ns | −4.17% | <.001 |

The supported default-full and no-edit shifts remain unattributed. They are
not evidence of a compact-runtime gain. No-edit remains **0 B/op / zero
allocations**. The [measurement](benchmarks/20261017-visible-count-cold-walk/measurement.json)
and raw benchstat files retain every metric and direction.

## Correctness and source limits

All **412 ordinary plus two owned-YAML ledger rows** are byte-identical before
and after, including the existing routes and work/reuse counters. The
[counter reference](benchmarks/20261017-visible-count-cold-walk/counter-reference.json)
links the complete published values. The candidate passes full core normal,
phase-zero-A, race and 386 tests, focused scheduler/callback tests, Go/YAML
and owned-YAML 72-step invariants, focused locked-C fresh/edit/error checks,
and the YAML EOF/version-ownership and contextual-Go C regressions.

Existing occurrence, alias/extra, hidden `ERROR_REPEAT`, shared-DAG overflow,
mutable-policy, rollback/reset, malformed-metadata and score-error-priority
tests pass against unchanged baseline and candidate. The baseline's broader
qualification belongs to the [paired-cursor receipt](compact-paired-terminal-cursors-receipt-20261017.md);
it is not transferred to this rejected candidate.

The [source reference](benchmarks/20261017-visible-count-cold-walk/source-reference.json)
pins all 3,508 baseline Go files through the preceding source reference and
both candidate files by hash. Apply its zero-context reconstruction patch
with `git apply --unidiff-zero`. The source, raw samples, runner/check scripts,
counter hashes and gate-log hashes/tails are retained. No candidate runtime
file enters the PR.
