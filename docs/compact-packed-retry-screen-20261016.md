# Packed lexer retry flags: reject general runtime activation

Evidence date: 2026-10-16 UTC. Baseline: `86879374e37ef0028443121d1bf12214377653a6`.

The private prototype removes **17.90–17.91% of owned YAML allocated bytes**
and **16.51–18.45% of allocation objects**, but ordinary YAML takes **1.33% more
time**. It is rejected as a general runtime change. No runtime source, route,
allowlist, default or counter exception is promoted. Main merges remain held
until the owner lifts the 0.56.0 hold.

## Representation and correctness boundary

The prototype replaces external-token retry `[]bool` fields in the live DFA
source and snapshots with an internal packed value. At most 128 flags occupy
two uint64 words, with shared immutable length metadata. Larger masks use
private backing, a native-int logical length stored by value and deep ownership
through `Clone` / `CloneInto`. Typed backing includes its descriptor and full
capacity in footprint charges. Shared snapshot/scratch backing is charged once.
The amd64 field stays 24 bytes; the 386 field grows 12→20 bytes.

This preserves flag indices above 65,535, active/inactive guard equality,
scanner/checkpoint rules, reset, restore and independent snapshot mutations.
The prototype adds no language-name dispatch. Tests adapt the existing slice
capacity assertions into byte-budget, private-storage identity, small-value
independence and full retained-backing clearing assertions. No root Go file is
added. Source remains outside the checkout.

The exact initial timed source passes **176 existing focused parser tests**,
leaf/root races, leaf and library 386 checks, focused YAML locked-C cases,
Go/YAML 48-step incremental sessions with zero-allocation no-edit reparses,
core tests, S5/scanner ownership, lifecycle and memory-budget checks.
All **414 ledger rows** match baseline exactly; all four canonical Go deep
digests and `Core.Work()` vectors match, and raw counters do not increase.
These are scoped checks, not a full-fleet qualification.

## Completed twenty-pair YAML screen

| Case | Median time before → after | Time direction | p | B/op before → after | Objects before → after |
| --- | ---: | ---: | ---: | ---: | ---: |
| Ordinary attempt/fallback | 193.6395 → 196.215 µs | +1.33% | .001 | 16,760 → 16,632 | 37 → 36 |
| Owned R4 | 347.634 → 346.659 µs | -0.28% | .091 | 12,872 → 10,568 | 109 → 91 |
| Owned 8 KiB stream | 1.363362 → 1.353957 ms | -0.69% | .033 | 44,312 → 36,376 | 336 → 274 |

Owned R4 timing is inconclusive. The stream improves slightly, while the ordinary
path significantly regresses. The owned allocation benefit does not qualify
ordinary YAML throughput or remove its graduation blocker. Ordinary allocation
bytes fall 0.76% and objects 2.70%; those savings do not erase its time cost.
All timed route/completion columns remain exact.

The repository randomized runner uses **20 alternating shuffled pairs**, seeds
**21901–21920**, one process per seed, `GOMAXPROCS=1`, `-count=1`, 750 ms and
`-benchmem`, CPU 6, 6 GiB Docker and both locks. Both roles produce all 60 samples
and complete without OOM or timeout. Source/native hashes, seed order, samples
and deterministic columns pass the audit. No Go timing, full 206-session fleet,
strict 204-file census or 1 MiB RSS qualification is claimed for this experiment.

## Review finding and private repair

Two larger Buckley source-review attempts finish execution without a completed
report: one reaches the 16-read limit and one reports `invalid_completion`.
Neither counts as approval. A smaller review of the complete leaf package and
its tests returns a concrete defect: `ResetRetained` drops oversized backing
before clearing it, contrary to that helper's stated clearing contract. The
returned value is empty, but a prior raw value alias can still see the flags.
This is a defect in the private prototype, not a demonstrated defect in the
committed runtime.

A new regression first fails on the exact old helper with
`discard retained retry bits in prior backing`. The private repair clears all
backing words before checking the retention limit; the regression and complete
leaf normal/race/386 suites pass. The review covers leaf source and tests only;
it does not certify parser integration or independent execution. Ignored
workspace review packets are removed.

The repair changes the private source after the completed screen. Its leaf
checks do not transfer parser or timing certification: the table above belongs
only to the archived pre-repair source. No runtime source is promoted after
either version. Future work must qualify any revised boundary between live
lexer flags and owned snapshots on its own exact source.

Early build failures for a stale import, mechanical test imports and an
overlay-only vet working directory are archived separately. The directory
failure is resolved with a container-only bind, leaving default vet enabled.
None supplies a parser failure or a performance sample.

Evidence: `gotreesitter-evidence/20261016-packed-retry-mask`, especially
`before-clear-rejected/{parser-pins.json,perf-audit.json,precheck/audit.json}`,
`decision.json`, `review-leaf-audit.json`, `discard-regression-before.log`,
`discard-regression-source-before` and `leaf-fixed-audit.json`. The qualified
[sixteen-slot Go improvement](compact-relex-capacity-receipt-20261016.md) remains
separate, with its own exact source and full qualification.
