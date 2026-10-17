# Canonical frontier head-mask qualification — 2026-10-17

**Qualified Go win:** five compact benchmarks use **0.89–1.74% less parse
time** in twenty shuffled pairs. Heap-allocation counts stay exact. All forty
YAML pairs are retained; their pooled timing differences are inconclusive.
Ordinary YAML still declines to legacy. No language graduates in this bucket.

Runtime commit: `75507f35ec6caece2125157b192e7c54060573e3`. The effective baseline is
`aa49318d79ead29a15dc94eab9b5be413d395790`, including the earlier lineage storage
change in PR #1487. Each bucket has its own pinned measurement; their timing
percentages are not added. The baseline integration checkout receives an
immutable full-source overlay, so its raw Git header differs from its effective
source. All 3,501 baseline Go files and candidate inputs are hashed.

## Change and correctness argument

The existing normalization scan still resolves every boundary, performs
canonical remapping, and compares version-owned lexer state. During that same
scan, an eight-byte local mask records each physical head's low six ID bits.
Distinct bits imply distinct head IDs, which imply distinct complete canonical
keys. Equal IDs always collide. Any collision, zero ID, or frontier wider than
eight retains the exact existing grouping path.

A frontier with distinct keys has only singleton groups. The shortcut resets
reduction freshness and publishes the existing normalized buffer; every other
header field remains unchanged. The internal helper adds no retained parser
state. Tests cover duplicate IDs, distinct IDs sharing a bit, maximum IDs,
unchanged rejected masks, all 64 buckets, input/output ownership, and nonzero
provenance/recovery/version metadata. No authentication, rollback or ownership
check is removed.

The preceding diagnostic census finds all keys distinct in **93–94%** of small
Go canonicalization calls. That measures full-key distinctness, rather than
mask coverage: mask collisions can conservatively retain exact grouping.

## Paired measurements

Runs use the repository randomized runner, Go **1.25.14**, `GOMAXPROCS=1`, CPU 6,
6 GiB Docker, `GOMEMLIMIT=5GiB`, `-count=1`, `750ms`, and `-benchmem`. Each seed
uses a separate process; baseline/head order alternates. Both benchmark locks
are held throughout. Go seeds are **27501–27520**. YAML uses **27601–27620** and
confirmation seeds **27701–27720**. All work/route columns remain exact.

| Path | Before → after | Time direction | p |
| --- | ---: | ---: | ---: |
| Go rewrite | 1.735350 ms → 1.714800 ms | -1.18% | <.001 |
| Go query | 8.294895 ms → 8.150674 ms | -1.74% | 0.001 |
| Go language | 8.581243 ms → 8.481118 ms | -1.17% | 0.015 |
| Go large | 87.824337 ms → 86.583416 ms | -1.41% | 0.004 |
| Go R4 compact | 797.606 µs → 790.499 µs | -0.89% | 0.004 |

All five Go reductions are supported. Allocation counts stay **21 / 120 / 239 /
1,705 / 22**, respectively, and their byte medians stay exact. These are
comparisons against the current compact source, not legacy/C ratios or the
frozen M1 graduation baseline.

| Path | Before → after | Time direction | p |
| --- | ---: | ---: | ---: |
| Ordinary YAML attempt + legacy | 189.513 µs → 190.178 µs | +0.35% | 0.276 |
| Owned YAML R4 | 348.522 µs → 345.448 µs | -0.88% | 0.058 |
| Owned YAML 8 KiB stream | 1.357522 ms → 1.347250 ms | -0.76% | 0.057 |

All three pooled YAML differences are inconclusive. The first twenty stream
pairs gave **−1.48%, p=.049**, with high baseline variance. The confirmation
run gave an inconclusive **p=.588**; the pooled forty-pair result is **p=.057**.
The initial marginal result is not retained as a YAML throughput claim.
Allocation counts remain **37 / 109 / 336**. Stream byte medians move
**44,312.5 → 44,313 B/op**; this rounding-sized change is not a memory win.

| Path | Before → after | Time direction | p |
| --- | ---: | ---: | ---: |
| Default Go full | 3.915954 ms → 3.918274 ms | +0.06% | 0.989 |
| Default Go first edit | 99.518 µs → 99.872 µs | +0.36% | 0.055 |
| Default Go no edit | 3.165 ns → 3.119 ns | -1.45% | <.001 |

The no-edit control bypasses this operation, so its supported time movement is
recorded without attributing it to canonicalization. It stays **0 B/op / zero
allocations**. The full/edit directions are inconclusive. Every direction is
preserved in the [measurement data](benchmarks/20261017-canonical-head-mask/measurement.json).

## Qualification

All **412 ordinary plus two owned-YAML work/reuse rows** are identical before
and after, including actual-checkout exports. No counter exception is used.
The four canonical Go fixtures retain every core/scheduler work field and
pinned deep-tree digest. Full core tests pass normally, with phase-zero-A,
races and 386, plus the selected-store one-pass build.

All **206 edit sessions × 72 steps** pass with allocation-free no-edit reparses.
All **618 C fixture checks** pass, one grammar per process. The strict real-file
census keeps all **204 rows** exactly: **120 compact C-exact acceptances, 39
passing legacy routes, and 45 existing legacy mismatches**. Of the legacy
routes, 35 successes/44 mismatches record a fallback; four successes/one
mismatch use legacy before a compact attempt. The broader legacy debt is not
reported as passing parity.

Go/YAML/owned-YAML invariant races and memory-budget/retention gates pass.
One-MiB Go and ordinary/owned YAML cover the full input, accept without errors,
and match locked C. Standalone parser safety probes remain under the unchanged
**400 RSS bytes per source byte** ceiling:

| Standalone parser | Before RSS/source byte | After RSS/source byte |
| --- | ---: | ---: |
| go | 328.98 | 339.23 |
| yaml ordinary | 68.76 | 66.74 |
| yaml owned | 114.59 | 117.97 |

These are single safety probes, not statistical RSS improvements. The separate
C+Go oracle process also retains a C tree and creates compatibility views for
every node; its different memory scope is retained in local raw logs and is
not used as standalone parser RSS.

Actual-source guards pass: **756 root Go files / 12 API tag sets**, unchanged
**454 language-name uses / 101 environment reads**, and **65 CI selectors / 7
workflows / 7,728 discovered test functions**, with no stale selector. The first
isolated source-guard snapshot lacked its two registry files. They were copied
unchanged and the check resumed; no registry, policy, or parser constraint was
relaxed. Final fleet, timing and resumed guard wrappers exit zero without OOM
or timeout.

## Screens retained without promotion

A generic full-key distinctness check passes focused correctness and all 414
rows, then measures five Go gains of **1.01–1.63%** but slows ordinary YAML
**0.95%, p=.018**. It stays outside the runtime. An array-based head proof also
passes focused gates and counters; it is replaced by the scalar mask before
timing and carries no performance claim. Neither precursor receives this
candidate's full-fleet or RSS qualification.

The ownership census finds no deep-equal snapshots hidden behind distinct
pointers. At equal cursors, **127 R4 / 396 stream comparisons** also differ in
serialized scanner payload. These are comparison counts, not unique version
counts. Their instrumentation timing/allocation numbers are not performance
evidence; those histories remain separate.

Raw samples, full before/after counters, complete canonical work vectors,
qualification and screen notes are under
[benchmarks/20261017-canonical-head-mask](benchmarks/20261017-canonical-head-mask/measurement.json).
Local frozen inputs, commands, logs and source pins remain in
`gotreesitter-evidence/20261017-canonical-head-mask`. Ordinary YAML ownership,
the remaining Go gap, E-A exit and graduation remain open. A follow-up election
copy investigation must retain the state vector consumed by EOF scanner
quiescence as well as checkpoint identities.
