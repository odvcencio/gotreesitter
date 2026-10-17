# Terminal-selection screens — 2026-10-17

**Both private sources are rejected.** Selecting discarded hidden terminals
reduces construction work, but neither source establishes a compact Go
throughput gain. No runtime, test, route, allowlist or ledger baseline changes.

The baseline is qualified runtime `5e52cf2a2644f3d38785513eebc39f8cf8a506b2`.
These screens follow the [deferred-publication rejection](compact-deferred-publication-screen-20261017.md).
They construct neither a compact leaf nor a public leaf for an elided terminal;
full consumption adds no replacement construction.

## Two protocols

The first source carries parent-use information in each materialization view.
The second restores the existing view layout and invokes a selector only for
terminal occurrences at the end of a non-unary raw reduction. Both require a
hidden non-extra, non-missing, non-fragile terminal with no skipped-prefix proof,
no reused payload, no parent aliases or fields, and fresh publication without
scanner checkpoints or recovery. Root and unary terminals retain construction.
Core still validates every record, replay transition and exclusive ownership.

Only a final raw child is eligible: it cannot widen a following sibling through
the existing hidden-padding rule. The parent keeps the original compact span.
A reusable scalar entry supplies the omitted terminal's exact raw extent to the
coverage check. No shared marker or scalar entry enters the public tree.

## Throughput

Each cell shows time direction and p. Bold differences are supported; unbolded
directions are inconclusive. Independent comparisons are not pooled or added.

| Path | Parent metadata in view | Narrow terminal selector |
| --- | ---: | ---: |
| Go R4 | **+4.77% / .001** | +1.15% / .134 |
| Go rewrite | **+1.52% / <.001** | +0.36% / .086 |
| Go query | **+1.22% / <.001** | +0.38% / .211 |
| Go language | **+1.27% / <.001** | +0.31% / .201 |
| Go large | **+2.15% / <.001** | +0.57% / .102 |
| Ordinary YAML attempt + fallback | +0.16% / .341 | **-0.70% / .026** |
| Owned YAML R4 | **+1.85% / <.001** | +0.36% / .134 |
| Owned YAML 8 KiB | **+1.49% / .023** | **+0.75% / .013** |

The view protocol slows every compact Go workload and both owned YAML
workloads. The narrower protocol removes the supported Go regressions, but all
five compact Go directions remain inconclusive. Its ordinary-YAML improvement
comes with a supported owned-stream regression. Neither source is promoted.

## Full tree consumption

An additional public candidate benchmark parses each fixture, traverses every
child, reads fields, checks parent links and verifies a stable checksum. Its
preflight digest and checksum match between roles. Every timed iteration must
stay on compact, cover the input and preserve the checksum.

| Go fixture | View protocol direction / p | Selector direction / p |
| --- | ---: | ---: |
| real_sample | **+1.20% / <.001** | +0.12% / .565 |
| rewrite | **+1.01% / <.001** | +0.40% / .174 |
| query_compile | **+1.04% / <.001** | +0.19% / .820 |
| language | **+0.27% / .043** | -0.13% / .659 |
| grammargen_lr | **+1.25% / .005** | -0.59% / .174 |

The first protocol slows all five consumption workloads; the second establishes
no supported consumption improvement. All allocation counts stay exact.
Fractional or rounding-sized allocated-byte movements supply no memory claim.

Default Go controls remain separate. View-protocol full/edit/no-edit directions
are **+0.78% / -0.22% / -0.59%** (full and no-edit supported). Selector controls
are **-0.24% / +1.13% / +0.27%** (only edit supported). These shifts remain
unattributed. No-edit stays **0 B/op / zero allocations**.

## Construction and correctness

Both sources remove **14 / 35 / 220 / 234 / 2,201** leaf constructions from Go
real-sample/rewrite/query/language/large parses. Public arena counts become
**1,237 / 2,212 / 11,106 / 10,683 / 105,763**, and stay at those values after
full consumption. Every deep digest equals the baseline; other native work
counters remain exact. These count reductions do not establish throughput or
RSS savings.

Across **414 ledger rows**, the only differences are **69 lower node counters
in 51 candidate rows**. Routes, tokens, peak versions, reuse and other fields
remain exact; both owned-YAML rows stay exact. No counter exception is needed
or applied.

Both sources pass full core normal/phase-zero-A tests, focused core race/386,
focused publication/lifecycle/memory and root race checks, Go/YAML/owned-YAML
72-step edit invariants, and Go/YAML locked-C fresh/incremental/error suites.
The new tests preserve walker ownership/replay rules, reject unsupported
elision contexts, clear scratch references and retain raw-byte coverage checks.
Neither source receives all-language invariants, full C-fleet, strict-census or
RSS qualification after the failed timing screen.

## Evidence and reproduction

The [result](benchmarks/20261017-terminal-selection/result.json) retains the
protocol and limits. Both comparisons use twenty alternating shuffled pairs,
one process per seed, Go 1.25.14, CPU 6, `GOMAXPROCS=1`, `-count=1`, `750ms`,
`-benchmem`, serialized 6 GiB Docker and both benchmark locks. View Go/YAML
seeds are **31901-31920 / 32001-32020**; selector seeds are
**32101-32120 / 32201-32220**. All **1,280 timed rows** are retained.

[View source, construction vectors, metrics and raw pairs](benchmarks/20261017-terminal-selection/view/measurement.json)
and [selector source, vectors, metrics and raw pairs](benchmarks/20261017-terminal-selection/selector/measurement.json)
keep each comparison separate. Each directory includes its zero-context runtime
patch, source hashes, diagnostic benchmark source, scripts and validation logs.
For an isolated baseline checkout, apply that patch with `git apply --unidiff-zero`
and install the two diagnostic `.go.txt` benchmark/census sources under their
scripted test filenames. The archived scripts document the frozen-overlay lab;
use the repository randomized runner and the same explicit seed ranges in
Docker. Single-iteration consumption preflights validate setup only and supply
no comparison evidence.

The next larger target is graph-link storage, informed by the separate
[packed-link codec proof](compact-packed-link-codec-proof-20261017.md). That
component proof is not a parser speed or memory result.
