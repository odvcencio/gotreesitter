# Lineage proof sharing: rejected throughput screen

Evidence date: 2026-10-15 UTC. Baseline: `c498e889ec2d86382a3a0baad58020d7df7145b3`.
The prototype remains outside the checkout in local
`gotreesitter-evidence/20261015-lineage-proof-sharing`; no runtime is promoted.
All merges to main remain held.

The graph census suggested sharing repeated owner-neutral metadata. The prototype
replaces each dense 32-byte record with an 8-byte owner/proof-reference slot and
an exact interner of immutable 28-byte proofs. Each writer copies and republishes
all metadata, and rollback restores previous slot identities and truncates the
proof arena. Both memory gauges count retained proof/index capacity, and EOF
recovery clones both arrays. Zero is implicit; hash collisions compare the full
value. No proof, ownership check, field, action, or route is removed.

This reduces retained lineage storage, but **fails the performance gate**:
large Go time rises **10.60%**. Query Go rises **9.12%**, Go R4 **9.06%**, owned
YAML R4 **6.31%**, and owned YAML stream **5.65%**; all five are significant
(p<.001). No allocation-count median improves. Do not trade this result into
production or infer that a smaller graph is faster.

## Randomized comparison

Twenty alternating shuffled pairs use the repository runner, one process per
seed, `GOMAXPROCS=1`, `-count=1`, 750 ms, and `-benchmem`, under both repository
locks in serialized 6 GiB Docker on CPU 6. Both roles use the same public benchmark
helper and frozen source overlays. Every seed and source hash passes the final
audit, and timed route/node metrics are exact.

| Benchmark | Median ns/op before → after | Time direction | B/op before → after | Allocations/op before → after |
| --- | ---: | ---: | ---: | ---: |
| `BenchmarkReductionPublicGo/query_compile` | 9.68788e+06 → 1.05715e+07 | +9.12% | 138739 → 138739 | 130 → 130 |
| `BenchmarkGoParseIncrementalNoEditDFA` | 3.1645 → 3.0835 | -2.56% | 0 → 0 | 0 → 0 |
| `BenchmarkReductionPublicGo/grammargen_lr` | 1.02812e+08 → 1.13713e+08 | +10.60% | 3.25173e+06 → 3.25173e+06 | 1717 → 1717 |
| `BenchmarkAdmissionR4Compact` | 825770 → 900620 | +9.06% | 1600 → 1600 | 22 → 22 |
| `BenchmarkGoParseFullDFA` | 4.03067e+06 → 4.01183e+06 | -0.47% | 1159.5 → 1159 | 6 → 6 |
| `BenchmarkGoParseIncrementalSingleByteEditDFA` | 99389.5 → 100126 | +0.74% | 394 → 394 | 5 → 5 |
| `BenchmarkAdmissionR4OwnedExternalLexer` | 352644 → 374896 | +6.31% | 12872 → 12872 | 109 → 109 |
| `BenchmarkAdmissionYAMLDocumentStreamOwned` | 1.38374e+06 → 1.46198e+06 | +5.65% | 44313 → 44313 | 336 → 336 |

The default full-parse direction is inconclusive (p=.081). The edit and no-edit
controls move +0.74% / −2.56% (both p<.001), bypass this compact storage path, and
are not attributed to it. No-edit stays at zero bytes and allocations.

## Storage census and correctness scope

A separate completed construction census materializes all four canonical Go
fixtures and verifies their pinned deep digests and unchanged scheduler/Core work.
These capacities include dense slots, the proof arena, and the lookup index:

| Fixture | Retained lineage bytes before → after | Direction | Nonzero interned proofs |
| --- | ---: | ---: | ---: |
| `rewrite` | 122,784 → 51,172 | -58.32% | 449 |
| `query_compile` | 484,032 → 194,736 | -59.77% | 1,796 |
| `language` | 993,312 → 362,996 | -63.46% | 2,430 |
| `grammargen_lr` | 5,655,040 → 2,364,032 | -58.20% | 21,764 |

All **414 deterministic rows** remain exactly equal: 412 ordinary plus two
diagnostic-owned YAML rows. Full core and phase-zero-A tests pass, as do default
and opt-in C4 checks, all 206 static table shapes, focused Go/YAML root tests,
Go/YAML/CMake/HTML/Python edit invariants, the owned-YAML invariant, and focused
locked-C checks for those languages. No-edit invariant observations remain zero.
The two initial local build/test attempts were repaired before this frozen screen.

Buckley's completed source-only review establishes no defect in the supplied
changes. Its inline-width caveat is resolved by the pinned
`alternativeSetInlineCapacity = 1`; exact proof equality remains decisive. The
first larger review packet timed out without a report and receives no approval
claim. No source-only review substitutes for tests or merge approval.

**No full 206-session invariant fleet, 618-fixture C fleet, strict 204-row census,
race, memory-budget, or large-file RSS qualification is transferred to this
rejected runtime.** The prior qualified core is unchanged. The independent
[dense flag-packing follow-up](compact-layout-dispatch-screens-20261015.md)
also fails to establish a compact throughput gain and remains unpromoted.

## YAML retry-cache attribution

The separate local `20261015-yaml-shift-snapshot-fields` probe uses current
`c498e889e` source with an observation hook only. Both the 1,998-byte R4 and
four-document witness preserve route, public S-expression/span/error, token/node
counts, Core/scheduler work, and restored scanner/parser/GLR state. Frozen-source
hashes pass. This is a diagnostic, not a runtime change or a ledger comparison.

At the preceding state-176, zero-width token-111 shift, independent scanning
returns the same token but differs in exactly **`extZeroState`**, the active
zero-width retry-cache key. At the subsequent actual state-819/token-93 shift,
both independent replay and replay chained from the divergent prior snapshot
converge to the complete shared after snapshot. The existing cache equality
rules remain intact. These observations provide **no ownership certificate**:
a future proof must bind the actual shift history, retry-cache state, all
participants, alternatives and Core lifetime before ordinary YAML admission.
