# Roadmap

The current release is **v0.55.0**.

The production parser remains the default. An explicit process setting now
outranks the language admission allowlist. A parser override retains first priority.
This behavior change requires a minor release.

This release includes these fixes:

- Correct Python escape spans and `list_splat` binding. Django C tree differences
  fell from 101 to 6 across 2,932 files.
- Reject impossible anchored query runs, preserve highlight capture order,
  and correct `IsPatternRooted` for quantified roots.
  The 8 KB Nushell query fell from 16,577.358 to 0.360 ms on production.
- Remove quadratic C# election work. The reporter's 32 KB fixture fell from
  12,662 to 900 ms on production.
- Bound Make rescue and Dart reuse with low yield. Production measurements fell
  from 27,377.173 to 13.015 ms for Make and 258.646 to 119.063 ms for Dart.
- Build certified HTTP comment sections in linear time. Production parsing at
  32 KB fell from 2,160.112 to 1.657 ms and now returns the complete C tree.

The linked [release notes](../CHANGELOG.md#0550---2026-09-24) define the fixtures,
measurement limits, and separate correctness evidence. Measurements use Linux amd64.
Make, HTTP, and Dart fixtures reconstruct the reported shapes.
These results do not establish Windows performance or complete compact parser graduation.

Issue [#454](https://github.com/odvcencio/gotreesitter/issues/454) remains open:

- PR [#1280](https://github.com/odvcencio/gotreesitter/pull/1280) remains excluded.
  Transient-error incremental trees and diff and LESS edit mismatches remain unresolved.
- C# takes about three seconds at 137 KB. Continue profiling graph stack merges
  toward the sub-second target.
- Scala's grammar regression has a Linux bisect result. Its Windows edit slowdown
  remains unconfirmed on Linux, and the shared parser cost remains under investigation.
- Blank HTTP `# ` comments need a locked C regression and a section selection fix.
- Obtain the TOML editing-session script and remaining fixture generators.
  Request a Windows arm64 and amd64 rerun.

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

Run `GTS_ADMISSION_CANDIDATE=0 go run ./cmd/issue454bench scala_report 137 full`.
Select `replace`, `insert`, or `delete` instead of `full` to measure an edit.
The `scala_report` fixture repeats independent objects and methods after
`package demo`. The older `scala` fixture remains available for comparison.

A bisect from v0.53.0 to v0.54.0 identifies commit `74159ec1b`, which
updated the Scala grammar and scanner. On the 137 KB fixture, the old grammar
uses one parser stack. The new grammar forks 3,183 times, reaches three live
stacks, and allocates 14.83 MB of graph-structured stack scratch. Both versions
accept the whole source and return 57,281 nodes. The runtime and scanner cost
remains under investigation. Keep the new grammar's syntax coverage and the
locked C parity while reducing that cost.

### C++ delete shortcut from issue #454

The reporter did not share the C++ generator or edit position. The `cpp`
fixture in `cmd/issue454bench` is a deterministic reconstruction.

Run `GTS_ADMISSION_CANDIDATE=0 go run ./cmd/issue454bench cpp 137 delete`.
Both versions reject subtree reuse with `external_scanner_unsupported`.
On the reconstruction, v0.53.0 consumed 23 tokens and stopped at
`iteration_limit`. Both old trees ended at byte 57 and returned 41 nodes.
v0.54.0 consumed 53,991 tokens, reached the end, and returned 79,708 nodes.
Both new trees ended at byte 140,294 with `accepted` and matched each other.

The fast old result came from an incomplete parse, not a valid reuse
shortcut. Do not restore it. A matching node count alone cannot certify
the tree. Compare the reporter's original fixture with the locked C oracle
if the reporter supplies it.
