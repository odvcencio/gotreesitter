# Roadmap

The current release is **v0.54.0**.

The production route (the mature GLR engine) is the default for every
eligible fresh parse; the compact ("candidate") route is now opt-in through
`GTS_ADMISSION_CANDIDATE=1`, inverted from the prior release. This release
answers downstream issue #454's remaining field reports:

- Default the compact admission route to off; buildbox measured it 1.1x to
  2.2x slower than production on typical files across several languages.
- Throttle the compact scheduler's memory-footprint poll and make
  `ExternalLexer`'s read-frontier tracking lazy, benefiting every route that
  uses an external scanner.
- Fix C and Java `TokenSource` scanning so incremental reuse cannot resume
  mid-literal, arm the reuse-budget stop on plain `ParseIncremental`, exempt
  extra leaves from the compact reuse proof, fall back to a fresh parse on a
  missing `Tree.Edit` with a length change, and route Groovy incremental
  parses through a fresh full parse.
- Invalidate the GLR shape-prefix cache only on a link-0 rewrite, fixing a
  superlinear parse past nesting depth 1600.
- Preserve explicit end-token acceptance in the C lexer DFA, keep named
  `token.immediate()` terminals at their authored precedence, and fix a YAML
  bare-`[`/`{` error shape that diverged from the C reference.

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

### Admission route precedence

A parser override takes priority over all process settings. An explicit process
setting takes priority over the language allowlist. This rule applies to both
`SetAdmissionCandidateRouteDefault(false)` and `GTS_ADMISSION_CANDIDATE=0`.
The allowlist widens only the implicit production default. An unrecognized
environment value leaves the default implicit.

Use `WithHighlighterAdmissionCandidateRoute(false)` to pin the document parser
and injected-language parsers to production. Set the option to `true` to request
the compact route. Eligibility checks can still decline a compact parse.
Without this option, injected-language parsers remain on production.

### Scala fixture from issue #454

Run `GTS_ADMISSION_CANDIDATE=0 go run ./cmd/issue454bench scala-report 137 full`.
Select `replace`, `insert`, or `delete` instead of `full` to measure an edit.
The `scala-report` fixture repeats independent objects and methods after
`package demo`. The older `scala` fixture remains available for comparison.

### C++ delete shortcut from issue #454

Do not restore the old 36-token shortcut based on node counts alone. Equal
node counts do not prove equal node types, ranges, fields, or parse states.
The report does not include the C++ source generator or the deleted byte.
Obtain that fixture and compare the old incremental tree with a fresh tree
and the locked C oracle before changing the reuse gate.
