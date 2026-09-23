# Roadmap

The current release is **v0.53.0**.

Eligible fresh parses use the compact parser by default, with legacy fallback
for unsupported cases. This release fixes public API contract faults and
C-parity gaps found by a repository audit:

- One timeout deadline for each parse, across the compact and production routes.
- Safe tree handles: a stale `Release` does nothing, and an unchanged
  incremental parse no longer invalidates its result when the caller releases
  the old tree.
- Incremental reuse that is correct for multi-edit sequences and changed
  included ranges.
- A default memory budget that grows with the input size.
- Reserved-word keyword promotion as in C, for six grammars.
- Query predicates that check every node of a quantified capture.

These changes do not complete compact parser graduation.

Keep benchmark results, profiles, and source snapshots outside the repository.
Publish reproducible evidence in pull requests or external artifacts.

Publication still requires the mandatory gates in
[the release checklist](releasing.md#release-checklist).
The owner approved only the dated
[v0.52.0](releasing.md#v0520-only-tag-creation-exception) and
[v0.53.0](releasing.md#v0530-only-tag-creation-exception)
tag-creation exceptions.

Detailed shipped evidence lives in [CHANGELOG.md](../CHANGELOG.md) and its
[archive](changelog/). Standard minor releases may ship on any day after the
exact commit on `main` passes the full hosted continuous integration
workflow. Require complete correctness and performance evidence; do not use
a calendar delay as a replacement for release evidence. The immutable-tag
process and urgent-patch exception are documented in
[docs/releasing.md](releasing.md).

## Now — cleanup, ownership, and explainability

- Correctness, portability, and supported parser depth are banked. Preserve
  their receipts and keep correctness gates distinct from the currently
  advisory performance gates.
- Prioritize repository maintenance: remove obsolete experiments and
  telemetry, reduce duplication, clarify subsystem ownership, improve
  documentation, and keep ownership receipts current.
- Retire result-normalization shims only after the authoritative parser,
  scanner, materializer, or incremental mechanism owns the behavior and the
  required route receipts prove the shim inert. Follow the
  [compat-tier retirement guide](compat-tier.md) for the mechanical
  retirement contract.
- Keep the authenticated public benchmarks as regression signals and preserve
  their historical claims. Parser-core, no-tree, compact-candidate, and
  synthetic lanes remain diagnostic rather than public performance claims.
- After cleanup, the next major performance milestone is public
  `Parser.Parse` at no more than **1.5x C** on the locked canonical real-code
  benchmark. It is a future target, not a current gate or achieved result.

## Measured memory boundary

- A production frozen-tree store remains closed by the current measurements:
  whole-tree conversion does not recover enough full-parse time and
  regresses incremental reuse. Do not treat pointer-light migration as an
  authorized next step without new evidence that clears those gates.
- Bounded semispace retention, earlier reclamation of unselected
  construction debris, and tighter parser-budget-to-process-RSS behavior
  remain viable experiments. Each must preserve query, cursor, and
  incremental semantics.

## Deferred — Go-native experience and broader architecture

- Context-aware `Engine`/`ParseRequest`, typed limits and stop reasons,
  immutable registries/profiles, and internal pooling/diagnostic modes.
- `Document`/`Snapshot` ownership for edits, changed ranges, cached queries,
  highlights, tags, injections, and release lifetimes.
- Changed-range analysis plans, a provenance-rich sectioned grammar bundle,
  stronger scanner conformance tooling, and measured optional AOT tiers.
