# Paired reduction-terminal cursors — 2026-10-17

The owned YAML 8 KiB document stream takes **1.20% less time, p=.007**, in
twenty shuffled pairs. Compact Go differences remain inconclusive. Allocation
counts and all 414 work/reuse ledger rows remain exact. Ordinary YAML still
declines to legacy; this change does not graduate a language.

Runtime: `5e52cf2a2644f3d38785513eebc39f8cf8a506b2`. Baseline:
`a03d0842994a85aa26cf494c8c67404e1e64e999`, including the earlier scheduler
binding and reduction-aggregation bucket. Percentages from separate buckets
are not added. The [source reference](benchmarks/20261017-paired-terminal-cursors/source-reference.json)
and [patch](benchmarks/20261017-paired-terminal-cursors/runtime.patch) pin the
three changed files and the shared 3,506-file baseline manifest. Apply the
zero-context patch with `git apply --unidiff-zero`.

## Change and correctness boundary

Reduction-history comparison fully validates the left terminal before visiting
the right side. When the right cursor reaches the same live terminal ID, that
immediate proof also covers the right occurrence. A shared leaf already at
the right stack top consumes one logical visit and pops directly. Parent
walks, child order, ordinary/missing/error/opaque checks, distinct-terminal
scanner and prefix comparisons, and the arena-derived traversal budget retain
their existing behavior. The 14-visits/13-budget regression still rejects.

Scanner-pair and checkpoint reads are pure within this interval. No callback
or Core mutation intervenes, and no proof survives the current pair or crosses
a Core boundary. The left checkpoint error or decline still precedes malformed
right metadata. No retained cache, record-layout change, ownership relaxation,
or new route is introduced. The [proof audit](benchmarks/20261017-paired-terminal-cursors/paired-proof-audit.json)
records the source hashes and focused cases.

An earlier untimed control passed focused gates before the direct leaf pop was
added. Its first test expected no error from an invalid reused-opaque terminal;
the unchanged baseline correctly returned a stale-provenance error. The test
expectation was corrected for both roles. The original attempt remains in the
local evidence; no timing result is claimed for that control.

## Measurement

Go 1.25.14, CPU 6, `GOMAXPROCS=1`, 6 GiB Docker, `GOMEMLIMIT=5GiB`,
`-count=1`, `750ms`, and `-benchmem`; both benchmark locks serialize the run.
The repository randomized runner uses one process per seed and alternates
baseline/candidate order. Go seeds are **30601–30620**, YAML **30701–30720**.
All **440 timed rows** remain in the evidence.

| Path | Before → after | Time direction | p |
| --- | ---: | ---: | ---: |
| Go R4 compact | 781.576 → 781.767 µs | +0.02% | .547 |
| Go rewrite | 1.700024 → 1.699298 ms | −0.04% | .989 |
| Go query | 8.079288 → 8.104303 ms | +0.31% | .547 |
| Go language | 8.417133 → 8.426794 ms | +0.11% | .883 |
| Go large | 85.975755 → 86.217267 ms | +0.28% | .862 |
| Owned YAML 8 KiB | 1.317898 → 1.302036 ms | **−1.20%** | **.007** |
| Owned YAML R4 | 338.701 → 336.220 µs | −0.73% | .211 |
| Ordinary YAML attempt plus fallback | 188.048 → 190.076 µs | +1.08% | .072 |

Only the bold change is statistically supported. Allocation counts stay
**22 / 21 / 120 / 239 / 1,705** for Go and **336 / 109 / 37** for YAML in
the table's order. Go language bytes round **150,418 → 150,419 B/op**;
this is not a memory-saving claim.

| Default Go control | Before → after | Time direction | p |
| --- | ---: | ---: | ---: |
| Full | 3.867578 → 3.907140 ms | **+1.02%** | **<.001** |
| First edit | 98.793 → 98.807 µs | +0.01% | .888 |
| No edit | 3.127 → 3.130 ns | +0.10% | .292 |

The supported default-full slowdown is retained without attribution to compact
work. No-edit remains **0 B/op / zero allocations**. Raw samples, every metric,
and benchstat output are in the [measurement bundle](benchmarks/20261017-paired-terminal-cursors/measurement.json).

## Qualification

All **412 ordinary plus two owned-YAML ledger rows** equal baseline and the
earlier published values. The [counter reference](benchmarks/20261017-paired-terminal-cursors/counter-reference.json)
links complete values and identical before/after/actual hashes. Four complete
canonical Core/scheduler vectors and pinned deep-tree digests also stay exact.
No ledger exception or threshold change is used.

Full core normal, phase-zero-A, race, 386 and selected-store one-pass checks
pass. All **206 × 72-step edit sessions**, the owned-YAML session,
Go/YAML/owned-YAML races, memory/retention contracts and allocation-free no-edit
gates pass. **618 locked-C fixture checks** pass one grammar per process.
The strict census preserves all **204 rows: 120 compact C-exact, 39 passing
legacy routes, and 45 existing legacy mismatches**. Passing legacy routes
include 35 recorded fallbacks and four routes before an attempt; the existing
mismatches include 44 fallbacks and one route before an attempt. Existing
legacy mismatches are not reported as passing parity.

One-MiB Go and ordinary/owned YAML accept cleanly, cover the input and match
locked C in both roles. The [qualification report](benchmarks/20261017-paired-terminal-cursors/qualification.json)
and per-language records preserve the gate counts and scope.

## RSS safety and actual-source guards

All six initial standalone RSS probes pass the unchanged **400 B/source B**
ceiling. The initial Go increase **341.85 → 395.31** remains in the evidence.
Twenty additional matched pairs per case, seeds **31001–31020**, retain all
120 processes with explicit `GOMAXPROCS=1`, alternating role order and shuffled
case order. All baseline and candidate peaks pass 400; all three median ratios
pass the unchanged 10% ratchet.

| One-MiB case | Median before → after, RSS B/source B | Median ratio |
| --- | ---: | ---: |
| Go | 352.225 → 343.304 | .97467 |
| Ordinary YAML | 68.817 → 68.974 | 1.00227 |
| Owned YAML | 116.101 → 113.946 | .98145 |

The highest candidate Go peak across initial and matched probes is **395.31**.
These safety probes support no statistical RSS-saving claim. All **126 raw
records** are retained in the [RSS report](benchmarks/20261017-paired-terminal-cursors/rss-safety.json)
and [logs](benchmarks/20261017-paired-terminal-cursors/rss-raw.txt).

Exact-source checks verify all **3,508 tracked Go files**, **756 root Go files /
12 API tag sets**, **454 language-name uses / 101 environment reads**, and
**65 selectors / seven workflows / 7,736 discovered functions**. Actual ledger
values equal measured values. The initial filesystem guard also scanned an
ignored `.worktrees/post738` checkout and failed. Its output is retained. The
same unchanged guard passes against every tracked source file, with no
allowlist edits; source hashes match the measured candidate throughout.

Ordinary YAML ownership, the remaining Go throughput gap, frozen-M1 E-A
comparisons and graduation remain open. Allowlists and the global default stay
unchanged. This bucket claims no independent review or main merge.
