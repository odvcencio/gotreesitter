# Compact scheduler overhead qualification — 2026-10-17

**Qualified combined result:** two compact Go timings use **0.46% / 0.80% less
time**, and all three YAML paths use **0.59–1.22% less time**, in twenty shuffled
pairs. Allocation counts remain exact. Ordinary YAML still declines to legacy;
no language graduates in this bucket.

Runtime commit: `027a10d5a75eb59b45a7a0c3477e9254a7c8553e`. The effective baseline
is `87997c2fa33d0789e057827f031eabcdaec88ca5`, which already includes PR #1487's
lineage and canonical-head-mask changes. These are current-runtime comparisons,
not compact/legacy or Go/C graduation ratios. Independent bucket percentages
are not added.

## Change

Scanner binding resolves both core-local checkpoint IDs and binds the exact
pair in one internal operation. It avoids two discarded digest-receipt copies
and duplicate identity lookups. Empty checkpoints, invalid-ID error priority,
active-transaction rejection, overflow, and boundary invalidation keep their
existing behavior. Rejected bindings leave both scheduler and phase state
unchanged.

General reduction-boundary aggregation moves into an internal helper. The
existing sole fresh-pop return remains before that workspace. Predecessor and
historical provenance reads, certificate authentication, action reads after
callbacks, source-owner restoration, output order, and counters retain their
original behavior. No proof state or ownership check is removed. The public
election record stays unchanged.

## Direct combined measurement

The repository randomized runner uses Go **1.25.14**, `GOMAXPROCS=1`, CPU 6,
6 GiB Docker, `GOMEMLIMIT=5GiB`, `-count=1`, `750ms`, and `-benchmem`.
Baseline/head order alternates; each seed uses its own process and both
benchmark locks remain held. YAML seeds are **28901–28920**; Go seeds are
**29001–29020**. All 440 timed rows are retained, with exact work/route columns.

| Path | Before → after | Time direction | p |
| --- | ---: | ---: | ---: |
| Go rewrite | 1.705299 → 1.698429 ms | −0.40% | .063 |
| Go query | 8.111461 → 8.092896 ms | −0.23% | .242 |
| Go language | 8.433762 → 8.395248 ms | **−0.46%** | **.011** |
| Go large | 86.093295 → 85.735123 ms | −0.42% | .512 |
| Go R4 compact | 788.314 → 781.979 µs | **−0.80%** | **<.001** |
| Owned YAML R4 | 340.996 → 337.138 µs | **−1.13%** | **.002** |
| Owned YAML 8 KiB | 1.331759 → 1.315473 ms | **−1.22%** | **<.001** |
| Ordinary YAML attempt plus fallback | 187.598 → 186.484 µs | **−0.59%** | **.026** |

Only the bold reductions are supported. Go allocation counts remain
**21 / 120 / 239 / 1,705 / 22**; YAML counts remain **109 / 336 / 37**.
Stream byte medians round **44,312.5 → 44,313 B/op** and Go language rounds
**150,419 → 150,418.5 B/op**; neither supplies a memory-saving claim.

| Default Go control | Before → after | Time direction | p |
| --- | ---: | ---: | ---: |
| Full | 3.900654 → 3.863360 ms | −0.96% | <.001 |
| First edit | 99.618 → 98.867 µs | −0.75% | <.001 |
| No edit | 3.114 → 3.133 ns | +0.61% | .004 |

These controls are retained without attributing their movements to compact
scheduler work. No-edit stays **0 B/op / zero allocations**. All directions
and metrics are in the [measurement data](benchmarks/20261017-scheduler-overhead/measurement.json).

## Components and rejected source

Binding alone passes focused correctness and all 414 counter rows. Its first
twenty YAML pairs measure owned-R4 **−0.99%, p=.020**; fresh confirmation gives
**−0.56%, p=.033**. All forty give **−0.76%, p=.002**. Ordinary and stream timing
differences remain inconclusive. All five compact Go results are inconclusive.

Reduction aggregation alone also preserves all 414 rows. Ordinary YAML improves
**1.13%, p<.001** initially and **1.24%, p=.001** in confirmation. The pooled
forty-pair result is **−1.15%, p<.001**. Compact Go and owned YAML remain
inconclusive. Default-control movements are recorded without compact-work
attribution. The [binding](benchmarks/20261017-scheduler-overhead/binding/measurement.json)
and [reduction](benchmarks/20261017-scheduler-overhead/reduction/measurement.json)
screens retain every initial/confirmation sample and their source patches.
Published patches use zero-context hunks (`git apply --unidiff-zero`); applying
each to its frozen baseline reproduces every measured candidate file exactly.
Standalone sources receive focused qualification; the full fleet applies to
the directly measured combined source.

An earlier [election-state split](benchmarks/20261017-scheduler-overhead/rejected-election-state/screen.md)
shrinks retained election storage **264 → 200 bytes**, but establishes no
compact Go throughput gain and slows the owned stream **0.56%, p<.001**.
It is rejected. Its complete 440-row comparison, layout, tests and patch remain
separate. The runtime carries none of that representation change. The first
reduction-extraction build's unused import was fixed before successful checks
and timing; its failed build evidence remains local.

## Correctness and deterministic counters

All **412 ordinary plus two owned-YAML rows** match before/after and the prior
published dataset exactly. The [counter reference](benchmarks/20261017-scheduler-overhead/counter-reference.json)
records hashes and links the complete identical values. No ledger reset or
exception is used. Go candidate first edit and owned YAML first edit retain
their existing legacy fallback; these are not compact incremental wins.

Four full canonical Core/scheduler vectors and pinned deep-tree digests remain
exact. Full core normal, phase-zero-A, race, 386 and selected-store one-pass
checks pass. All **206 × 72-step edit sessions**, the owned-YAML session,
Go/YAML/owned-YAML races, and allocation-free no-edit gates pass. All **618
locked-C fixture checks** pass, one grammar per process.

The strict census preserves all **204 rows**: **120 compact C-exact**, **39
passing legacy routes**, and **45 existing legacy mismatches**. The legacy
routes include four successes and one mismatch before a compact attempt;
35 successes and 44 mismatches record a fallback. Known legacy mismatches are
not reported as passing parity.

One-MiB Go and ordinary/owned YAML match locked C, cover the full source, and
accept without errors. Memory-budget and retention contracts pass. Actual
checkout guards confirm **756 root Go files / 12 API tag sets**, unchanged
**454 language-name uses / 101 environment reads**, and **65 selectors / seven
workflows / 7,732 discovered functions**, with no stale references.

## RSS failure retained and investigated

The initial unchanged-Go baseline measures **401.11 RSS bytes/source byte**,
above the unchanged **400** ceiling. That probe remains a failure in the
evidence. The initial candidate measures **366.41**. The first full wrapper
stops at the baseline assertion; no source, ceiling or parser contract changes.

Twenty fresh matched Go pairs with explicit `GOMAXPROCS=1` repeat the same
binaries and one-MiB witness. Every before/after repeat remains below 400:

| Matched standalone Go RSS | Baseline | Candidate |
| --- | ---: | ---: |
| Median bytes/source byte | 344.63 | 352.48 |
| Maximum bytes/source byte | 370.27 | 376.06 |
| Ceiling failures in 20 processes | 0 | 0 |

The candidate median moves **+2.28%**, within the unchanged **10% ratchet**.
No RSS improvement is claimed. Initial ordinary/owned YAML candidate probes
measure **68.94 / 121.14**; their baseline probes measure **68.77 / 117.10**.
All initial candidate probes satisfy 400. An earlier twenty-pair diagnostic
omitted explicit `GOMAXPROCS`; it is retained separately and excluded from the
matched ratchet. All original and repeat records remain in
[RSS safety data](benchmarks/20261017-scheduler-overhead/rss-safety.json).

After verifying those unchanged binaries and source pins, the suite resumes
at the remaining invariant/C/strict/race gates. The resumed wrapper and actual
source guards exit zero without OOM or timeout. The qualification status
explicitly retains the initial baseline RSS failure. The separate C+Go oracle
process also holds a C tree and compatibility views for every node; its RSS
scope is not the standalone parser measurement.

## Reproduction and scope

Raw paired samples, full frozen source pins, final source hashes, component
patches, probes and qualification are under
[benchmarks/20261017-scheduler-overhead](benchmarks/20261017-scheduler-overhead/source-pins.json).
For timing reproduction, mount clean checkouts at `/baseline` (the baseline
above) and `/workspace` (the runtime commit above), plus a writable `/evidence`.
Run the published `reproduction/prepare-benchmark-overlay.py.txt` with the
artifact directory and `/evidence`, then `check.sh` and `perf.sh` inside the
same pinned Docker configuration, under both benchmark locks. It verifies the
source hashes and prepares only the diagnostic benchmark/ledger overlays;
it never creates a passing marker. Run `benchstat` on the host after samples
complete. Full local commands, frozen inputs, failed/resumed logs and pins
remain in `gotreesitter-evidence/20261017-scheduler-binding-aggregation`.

Ordinary YAML ownership, the remaining Go gap, frozen-M1 E-A comparisons and
graduation remain open. Routing, allowlists and the global default stay
unchanged. This advances E-A7/E-A4 work without marking either issue complete.
