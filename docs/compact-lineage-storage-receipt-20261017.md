# Compact lineage storage screens and callback regression guard

These independent screens start at `d7729488c72a585c4b5e1cb41023478953517874`.
The predecessor includes #1486's YAML lookahead cache in both comparison roles.
Runtime commit: `39f842c5b2196795a3894d81f6b030ed1dae7100`. No storage change alters routing, allowlists, or graduation status.

The fixed baseline already handles recovery-cost updates during reduction
planning. A new regression test preserves that behavior while storage changes:
a metadata callback writes cost 17 on a source created during scheduler
speculation, and both subsequent goto calls must observe clean-child selection
disabled. Baseline passes. The initial lazy-sidecar prototype fails twice because
it snapshots cost before the callback. Reading cost after the plan callback fixes
that prototype; the witness then passes normal, phase-zero-A, race, and 386 core
suites. This is a regression guard for optimization work, not a claim of a shipped
runtime defect.

## Shared immutable provenance: rejected

The first variant replaces each dense 28-byte record with a 16-byte mutable
owner/cost/reference slot and a shared immutable 16-byte provenance record.
A bounded 64-entry memo compares complete values on every hit. Collisions evict
lookup entries only; published references remain stable. Rollback truncates the
arena and clears the memo. Both capacity gauges include retained arena and memo
storage, and diagnostic EOF clones copy and isolate both buffers.

Retained large-Go lineage capacity falls 4,948,160 → 3,204,608 bytes (−35.24%).
This does not pay for the extra publication work:

| Case | Parse-time change | p |
| --- | ---: | ---: |
| Go rewrite | +10.07% | .002 |
| Go query compilation | +11.00% | <.001 |
| Go language fixture | +10.79% | <.001 |
| Go grammar generator | +10.95% | <.001 |
| Public Go R4 compact | +9.91% | <.001 |
| Ordinary YAML compact attempt plus fallback | +0.84% | .017 |
| Owned YAML R4 | +5.50% | <.001 |
| Owned YAML 8 KiB stream | +5.79% | <.001 |

Ordinary YAML allocations rise 37 → 43 (+16.22%), above the 10% allocation
ratchet, while bytes fall 16,760 → 16,360. Owned YAML bytes and object counts
remain exact. Compact Go object counts stay exact; tiny language/large-file byte
rounding differences are not memory wins. No runtime is promoted.

## Direct mutable provenance with lazy recovery: rejected

The second variant preserves direct mutable provenance in 20-byte records.
Only a nonzero cost or drop-reference index creates the separate recovery array.
Untouched entries read as zero; extension clears stale retained entries.
Length marks and the existing mutation journal restore nested transactions.
The first nonzero write reserves recovery capacity to match the hot arena;
all retained capacity remains in the footprint and retention gauges.

All four Go witnesses keep recovery storage absent and reduce retained lineage
capacity 28.57%. Large Go falls 4,948,160 → 3,534,400 bytes. Go rewrite/query/
language/large/R4 time changes are +0.65% / −0.45% / +0.38% / −0.85% / −0.35%,
all inconclusive (p=.149/.620/.398/.602/.968). Owned YAML R4 and stream changes
are −0.08% / −0.18%, also inconclusive (p=.698/.512).

Ordinary YAML slows 1.55% (p=.026), despite bytes falling 16,760 → 16,120;
allocations stay 37. This version is not promoted as a throughput improvement.

## Sixteen-byte mutable provenance: retained-storage win

The final independent variant also packs the two provenance
booleans into one byte. Recovery cost and reference storage stay separate and
lazy. Owner, alternative set, scalar rank, lineage, and both flags retain their
values. Existing sibling-merge rollback snapshots compare complete logical
records, including the recovery fields. This target has no proof interner. All four Go fixtures reduce retained lineage
capacity **42.86%**; large Go falls **4,948,160 → 2,827,520 bytes**.

Twenty shuffled pairs measure:

| Case | Parse-time change | p | Result |
| --- | ---: | ---: | --- |
| Go rewrite | −0.48% | .040 | Small supported gain |
| Go query compilation | +0.21% | .968 | Inconclusive |
| Go language fixture | −0.13% | .242 | Inconclusive |
| Go grammar generator | −0.76% | .174 | Inconclusive |
| Public Go R4 compact | +0.32% | .201 | Inconclusive |
| Ordinary YAML attempt plus fallback | −0.54% | .157 | Inconclusive |
| Owned YAML R4 | −0.91% | .063 | Inconclusive |
| Owned YAML 8 KiB stream | −0.51% | .003 | Small supported gain |

All allocation-count medians stay exact. Ordinary YAML bytes fall
**16,760 → 15,608** (−6.87%) with 37 allocations. Tiny language/large-Go/owned-
stream byte rounding differences are retained in the raw samples and receive
no additional memory-win claim. Default Go full/edit/no-edit controls move
−0.49% / −0.06% / +4.21% (p=.001/.683/<.001), bypass this representation, and
receive no attribution. No-edit stays at zero bytes and objects.

This is a retained-storage improvement with two small timing gains. It does
not establish a broad Go throughput improvement, complete E-A2/E-A3, remove
ordinary YAML's ownership decline, or authorize graduation.

The final source passes **206 invariant sessions**, **618 C fixture checks**,
owned YAML's invariant, core and selected-store variant suites, 386 checks,
core and Go/YAML/owned-YAML races, and memory-budget/retention checks. The
**204-file strict census stays exactly equal**: 120 compact accepts, 39 passing
fallbacks, and 45 existing fallback mismatches. No mismatch is relabeled as a
pass. All 414 deterministic ledger rows remain exact, as do the complete Core
and scheduler vectors for all four canonical Go witnesses.

Separate 1 MiB correctness probes require full accepted coverage and deep
locked-C equality. Standalone parser RSS probes use the same authenticated
source without keeping a C tree or walking every compatibility node:

| Route | Bytes of input | RSS bytes/source byte before → after |
| --- | ---: | ---: |
| Go compact | 1,048,591 | 387.96 → 355.01 |
| Ordinary YAML | 1,049,572 | 69.74 → 68.63 |
| Owned YAML | 1,049,572 | 117.44 → 116.65 |

All standalone readings stay below 400 RSS bytes/source byte. These are single
safety probes, not statistical RSS-win evidence. Initial combined C+Go deep-
walk probes are retained separately; their Go reading exceeds 400 and is not a
standalone parser measurement. Correctness and parser RSS use separate
processes; no threshold or runtime memory limit is relaxed.

Raw [Go statistics](benchmarks/20261017-lineage-hot16/benchstat-go.txt),
[YAML statistics](benchmarks/20261017-lineage-hot16/benchstat-yaml.txt),
[counters](benchmarks/20261017-lineage-hot16/counters.json), and
[measurement metadata](benchmarks/20261017-lineage-hot16/measurement.json)
record the result. Raw files begin with an incomplete header and end with
`# status: complete`; all 20 samples per benchmark are retained.

## Protocol and scope

Each completed comparison uses the repository randomized runner, 20 alternating
shuffled pairs, one process per seed, `GOMAXPROCS=1`, `-count=1`, 750 ms, and
`-benchmem`. Go and YAML run in separate processes under both shared locks,
inside 6 GiB Docker on CPU 6 with Go 1.25.14. Every sample is retained.

Before timing, 412 ordinary ledger rows plus two diagnostic-owned YAML rows
remain exact. Focused canonical/lifecycle/dependency/legacy-read tests,
Go/YAML/owned-YAML 72-step edit invariants, focused locked-C fixtures and YAML
cursor/document/ownership/recovery cases, core races and 386 tests pass.
No-edit reparses allocate zero. Canonical storage censuses verify pinned tree
digests; the sidecar variants also compare all Core and scheduler work vectors.

The default Go full/edit/no-edit controls bypass the compact representation.
Their directions are reported without attribution: sharing −0.88% / −0.45% /
+2.96% (p=.038/.165/.001); plain sidecar −0.03% / −0.82% / +6.07%
(p=.547/.414/<.001). All no-edit byte and allocation samples remain zero.

No full 206-session invariant fleet, 618-fixture C fleet, strict 204-file census,
large-file RSS, or complete memory-budget qualification is transferred to a
rejected prototype. Neither storage capacity nor a single diagnostic parse time establishes a
throughput or statistical RSS win.

Local evidence contains frozen runtime/baseline overlays, source pins, raw
samples, counter files, benchmark statistics, and archived failed build/witness
attempts under `gotreesitter-evidence/20261017-lineage-direct-memo`,
`20261017-lineage-recovery-sidecar`, and `20261017-lineage-hot16`.
