# Changelog

All notable changes to this project are documented in this file.

This project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
for tags and release notes while still in `0.x`.

## [Unreleased]

## [0.55.1] - 2026-09-26

### Fixed

- Restore the Python initial stack cap of two when the source has no possible unpacking expression.
  Keep eight stacks for possible unpacking and preserve explicit stack overrides
  ([#1290](https://github.com/odvcencio/gotreesitter/pull/1290)).
  The report's inline generator builds 371,146 work nodes instead of 623,912.
  Production fresh parsing fell from 267.331 to 161.811 ms; the tree retains 59,105 nodes.
  The seven list-splat fixtures still match locked C. Django retains six differences across 2,932 files.
- Preserve conflict forks beyond stack depth 4,096 and reset the dispatch guard when reductions reach a new minimum depth
  ([#1291](https://github.com/odvcencio/gotreesitter/pull/1291)).
  C now returns clean, complete trees for 4,091 functions and up to 65,536 string calls in one function.
  Both lexers pass fresh, incremental, and locked C comparisons at the tested counts.
  At 8,192 calls, TokenSource parsing fell from 2,445.8 to 115.7 ms.
  The deterministic finite automaton (DFA) lexer fell from 2,425.1 to 123.7 ms.
- Remove a duplicate clone during hidden-node alias materialization
  ([#1292](https://github.com/odvcencio/gotreesitter/pull/1292)).
  Scala work nodes fell from 168,685 to 149,592 on both routes; the tree retains 57,281 nodes.
  Production fresh parsing fell from 73.565 to 70.638 ms.
  Work remains 4.5% above the old grammar's 143,197 nodes.
- Extend the poor-yield reuse guard to roots with more than four children
  ([#1292](https://github.com/odvcencio/gotreesitter/pull/1292)).
  A Dart proxy fixture now retries fresh for expensive edits and retains profitable suffix reuse.
  Production insertion fell from 240.990 to 123.209 ms; deletion fell from 718.243 to 123.759 ms.

### Documentation

- Align the roadmap, repository map, and agent workflow with the owner's v1 design
  ([#1329](https://github.com/odvcencio/gotreesitter/pull/1329)).
  This patch adds no public API or admission-default change.

### Measurement scope

The linked pull requests contain separate correctness and performance evidence.
Each completed the randomized Go benchmark trio with 20 shuffle seeds and 750 ms per case.
The runs used one process per seed and `GOMAXPROCS=1`.
No primary timing change was significant; primary allocations stayed at eight, five, and zero per operation.
These Linux measurements do not establish Windows performance.
Python uses the reporter's inline generator. Dart uses a 137 KiB proxy with 45.4% baseline reuse.
The reporter measured 41.8% Dart reuse; the exact fixture remains unverified.
Scala compact timing varied, with some medians increasing despite fewer work nodes.
The standard Scala grammar importer still rejects `_end_marker_named_tail`; focused locked C checks pass.
C corpus results remain 23/25 clean files and 20/25 deep matches.

### Publication exception

On 2026-09-26, the owner authorized the v0.55.1-only tag-creation exception.
The exception permits publication without a workflow-only tag-creation actor rule.
All other gates remain mandatory under [the release process](docs/releasing.md#v0551-only-tag-creation-exception).
This urgent patch addresses the Python regression and false C errors reported in [#454](https://github.com/odvcencio/gotreesitter/issues/454).
Waiting prolongs Python keystroke delays and false errors in valid C documents.

### Known gaps

- [#1280](https://github.com/odvcencio/gotreesitter/pull/1280) remains open and excluded.
  Transient-error incremental trees, `ERROR` roots with false `HasError()`, and diff and LESS edit mismatches remain unresolved.
- C# still misses the sub-second target at 137 KB. This patch contains no C# merge-preflight optimization.
- Scala retains extra work from the refreshed grammar and seven existing suffix gaps.
- VHDL inner `@spell` captures, blank HTTP comments, and compact sibling traversal remain unresolved.
- Six Django tree differences, two CSV comma witnesses, and the PHP compact recovery winner remain open.

## [0.55.0] - 2026-09-24

### Added

- Add `WithHighlighterAdmissionCandidateRoute` to select the route for the
  document parser and injected parsers ([#1281](https://github.com/odvcencio/gotreesitter/pull/1281)).
- Add optional compact parser telemetry for peak frontier headers and derivations
  ([#1285](https://github.com/odvcencio/gotreesitter/pull/1285)).
  Locked C checks cover three fresh JSON trees and seven fresh CSV witnesses.
  One incremental edit per language also matches with subtree reuse.

### Changed

- Give explicit process settings priority over the language admission allowlist.
  A parser override still has first priority. The allowlist now widens only
  the implicit default. This behavior change requires a minor release.
  Both `SetAdmissionCandidateRouteDefault(false)` and
  `GTS_ADMISSION_CANDIDATE=0` keep production selected
  ([#1281](https://github.com/odvcencio/gotreesitter/pull/1281)).

### Fixed

- Ignore `Accept` when the compact reduction collector checks whether a source can shift
  ([#1284](https://github.com/odvcencio/gotreesitter/pull/1284)).
  Four strict Scala witnesses match locked C without compact fallback.
- Elect accepted material paths inside the winning compact recovery group
  ([#1286](https://github.com/odvcencio/gotreesitter/pull/1286)).
  Preserve acceptance identity across grammar forks and closed error regions.
  Decline unsupported paths without publishing a guessed tree. The PHP compact request still falls back before acceptance.
- Bound end-of-file (EOF) recovery versions before allocating reduction parents
  ([#1287](https://github.com/odvcencio/gotreesitter/pull/1287)).
  Apply the native symbol-zero trial only to the certified Scala grammar blob.
  The Scala suffix suite records 50 exact matches and seven unchanged gaps across 57 cases.
  The 104,681-byte Swift witness completes without truncation or a memory stop.
- Prevent overlapping Python `escape_sequence` nodes when escaped backslashes
  precede a newline. All 18 fixtures now match the locked C tree on both routes;
  12 matched before the fix. Fixes [#1275](https://github.com/odvcencio/gotreesitter/issues/1275)
  ([#1276](https://github.com/odvcencio/gotreesitter/pull/1276)).
- Preserve Python `list_splat` binding for attribute and subscript suffixes
  during mixed flat and graph stack merges. Seven C fixtures match on both routes.
  Exact C tree differences across 2,932 Django files fell from 101 to 6.
  Fixes [#1274](https://github.com/odvcencio/gotreesitter/issues/1274)
  ([#1277](https://github.com/odvcencio/gotreesitter/pull/1277)).
- Reject query runs that cannot reach a required successor before enumerating
  capture combinations. Preserve highlight capture order for identical spans
  and prefer a highlight over `@spell`. Report quantified roots as non-rooted
  in `IsPatternRooted` ([#1278](https://github.com/odvcencio/gotreesitter/pull/1278)).
  The production query on 755 Nushell comments fell from 16,577.358 to 0.360 ms.
  Full highlighting of that 8 KB file took 20.132 ms after the change.
- Cache raw-shape error costs during generalized left-to-right (GLR) elections.
  Bound graph reachability checks and reject impossible merges earlier
  ([#1279](https://github.com/odvcencio/gotreesitter/pull/1279)).
  The reporter's C# fixture changed from quadratic growth to approximately linear growth.
  Production parsing at 32 KB fell from 12,662 to 900 ms.
  A separate 137 KB run measured a 3,008 ms median after the final merge precheck.
- Stop Make forest rescue at the first dead end when it cannot accept recovery.
  A reconstructed 3.5 KB fixture with an early error fell from 27,377.173 to
  13.015 ms on production ([#1282](https://github.com/odvcencio/gotreesitter/pull/1282)).
- Build C-certified HTTP sections directly for newline-terminated, nonblank
  `# ` comments under the certified grammar hash. Preserve deadlines and
  memory budgets; disable incremental reuse for these direct trees
  ([#1282](https://github.com/odvcencio/gotreesitter/pull/1282)).
  Production parsing at 32 KB fell from 2,160.112 to 1.657 ms.
  The result now matches the complete C tree instead of returning one error node.
  Maximum resident memory fell from 2,178,624 to 12,892 KiB.
- Stop incremental reuse with low yield after bounded work and retry a fresh parse.
  The guard requires an early edit, a live graph fork, and at most four root children.
  The guard applies across grammars ([#1282](https://github.com/odvcencio/gotreesitter/pull/1282)).
  Production insertion in the reconstructed 137 KB Dart fixture fell from
  258.646 to 119.063 ms. Maximum resident memory fell from 220,912 to 123,796 KiB.

### Measurement scope

These measurements come from the linked pull requests on Linux amd64.
They do not establish Windows performance. C# runs used the reporter's exact
fixture; Make, HTTP, and Dart used deterministic reconstructions.
Host load varied during the C# sweeps. The Nushell baseline used one repetition;
the result after the change used the median of three repetitions.

PR #1282 completed the standard Go trio with 20 shuffle seeds and 750 ms per benchmark.
It used one process per seed, `GOMAXPROCS=1`, and `GOWORK=off`.
The benchmark comparison found no significant time change:

| Benchmark | Before ns/op | After ns/op | B/op, unchanged | allocs/op, unchanged |
| --- | ---: | ---: | ---: | ---: |
| Full parse | 3,456,000 | 3,411,000 | 1.224 KiB | 8 |
| Single-byte edit | 91,310 | 90,720 | 386 | 5 |
| No edit | 3.307 | 3.294 | 0 | 0 |

The linked pull requests contain reproduction commands and separate correctness evidence.

PRs #1284–#1287 each record 20-seed randomized comparisons without significant changes in the three primary parse times.
PR #1285 shared its host with another workload; its timing evidence has that limit.
PR #1286 measured 111,092 KiB maximum resident memory for a 512 KiB Go parse.
PR #1287 measured warm Swift maximum resident memory of 257,440 KiB before and 257,332 KiB after the change.
Its primary allocation counts remained 8, 5, and 0.

### Publication exception

On 2026-09-24, the owner approved a one-time tag-creation exception for v0.55.0.
The exception permits publication without a workflow-only tag-creation actor rule.
All other gates remain mandatory under [the release process](docs/releasing.md#v0550-only-tag-creation-exception).

### Known gaps

- [#1280](https://github.com/odvcencio/gotreesitter/pull/1280) remains open and is excluded.
  Transient-error incremental trees and the diff and LESS edit mismatches remain unresolved.
  Its locked C gate still fails for malformed JavaScript, LESS, and TOML fixtures.
- C# still takes about three seconds at 137 KB on the measured host.
  Graph stack merges remain under investigation; the sub-second target remains open.
- Scala remains unresolved. A Linux bisect identifies grammar update `74159ec1b`
  as the fresh-parse regression trigger. Linux edits did not reproduce the
  Windows slowdown. Preserve the new grammar's coverage while investigating its cost.
- Blank HTTP `# ` comments retain the existing parser route and need a locked C regression.
- Six Django files retain separate C tree differences after the Python splat fix.
- CSV `,` and `a,b,` still differ from locked C on both parser routes.
- The PHP compact recovery winner remains open; the generated corpus records 6 of 25 exact deep matches.
- Seven Scala suffix gaps predate the EOF recovery bound. Other grammar blobs retain the exact EOF row.

## [0.54.0] - 2026-09-23

### Added

- Add `FactProgram.ExtractInto` to reuse caller-owned fact storage across trees.
- Add a per-language admission allowlist (empty by default) so a later change
  can graduate one language's compact route without flipping the global
  default.
- Add the `admission_route_performance_sanity` CI gate, comparing the compact
  route against the production route in-process, interleaved, on a fixed
  9-language corpus.

### Changed

- Default the compact ("candidate") admission route to off. Buildbox's
  tamarack harness measured it 1.1x to 2.2x slower than production on typical
  files across Python, Rust, Markdown, Lua, CSS, Bash, and Go, and
  TypeScript/YAML paid for both routes on every parse (compact declines, then
  production reparses). Set `GTS_ADMISSION_CANDIDATE=1` (or `true`/`on`/`yes`)
  to opt back in.
- Throttle the compact scheduler's memory-footprint poll: recompute the exact
  footprint on the first poll of a parse attempt, every 64th poll after it, or
  whenever a cheap, O(1) capacity-based growth proxy shows growth covering
  1/16th of the configured budget since the last exact check. This bounds
  worst-case overshoot by construction, not just by dispatch count. Measured
  peak-footprint overshoot: 1.25-2.16% over budget on an adversarial witness.
- Make `ExternalLexer`'s read-frontier tracking lazy: skip the frontier
  recompute once the recorded values already provably cover the current
  position. This benefits every route that uses an external scanner (YAML,
  Python, Markdown, Bash, and others), not just the compact route.

### Fixed

- Preserve explicit end-token acceptance when importing and minimizing C lexer
  states. Regenerate CSV from its locked C source. Older blobs retain their
  end-of-input fallback.
- grammargen gives a named `token.immediate()` terminal with a string body the
  same specificity bonus that an inline one gets. The lexer again accepts the
  immediate terminal instead of a plain string literal with the same text.
  Swift `var opt: Int?` parsed with an ERROR after a table rebuild, because the
  anonymous `?` won the same-span tie over `_immediate_quest`. Pattern-bodied
  named immediate terminals keep their authored precedence.
- Fix `parser_result_yaml.go` silently clearing `HasError()` for a lone
  unmatched YAML flow-collection opener (for example a bare `[` at end of
  input), which diverged from the C reference's `(ERROR ...)` shape. The
  production route now declines recovery for a bare `[`/`{` and promotes an
  all-`ERROR` reduction to the `ERROR` root itself. This bug predates this
  release; the compact-route default flip is what exposed it.
- The scanner fuzz allocation check now measures the minimum across 5 samples
  instead of requiring every re-run to exceed budget, removing a transient
  GC-timing false failure. The byte-budget assertion is skipped under the
  race detector, where instrumentation noise swung a measured TotalAlloc delta
  for the same input from about 66KB to about 29MB (more than 400x) and could
  no longer separate noise from a real finding. The elapsed-time check and the
  parser memory budget (`WithParserPoolMemoryBudgetBytes`) remain the hard
  bounds under `-race`.
- Invalidate the GLR shape-prefix cache only when a stack merge rewrites a
  link 0, not on every successful merge. A fixed nesting depth that forks and
  merges on every token used to rewalk the whole spine on the next head hash,
  which turned the parse superlinear past depth 1600 (issue #454). An
  extra-link-only merge now keeps its exact cached prefixes.
- Resume C and Java `TokenSource` scanning from the still-valid queued
  literal-tail tokens, instead of resetting and relexing, when incremental
  reuse resumes inside an already-queued string or character literal. The
  reset path used to drop the queue and misread the remaining bytes, so the
  incremental tree could differ from a fresh parse.
- Track reused bytes for plain `ParseIncremental`, not only for
  `ParseIncrementalProfiled`, so the reuse-budget stop arms on both entry
  points. `ParseIncremental` allocates no timing record, so a reuse-hostile
  edit taken through that entry point never armed the stop before this fix.
- Exempt extra (comment and whitespace) leaves from the compact incremental
  reuse proof. One unproven extra leaf used to disable incremental reuse for
  the whole compact tree; an ordinary leaf still requires a proof.
- Fall back to a fresh full parse when `ParseIncremental` receives an old
  tree with no recorded `Tree.Edit` call and a new source of a different
  length. The incremental path used to trust stale byte positions against
  the new source in that case, with no error or `ParseStoppedEarly` signal.
- Route every Groovy incremental parse through a fresh full parse. The
  Groovy grammar table derives a function-call juxtaposition only for a
  block's first statement, so spliced incremental reuse could produce a tree
  that a fresh parse of the same bytes never produces. Groovy's incremental
  performance is unchanged; parity with a fresh parse is now guaranteed
  instead of usually holding.

### Known gaps

- COBOL's column-dependency edit-invalidation over-invalidates a later-line
  token after an earlier-line, non-crossing edit. This is fail-safe (extra
  reparse work, not a wrong tree); COBOL does not support incremental reuse
  today, so it has no current observable effect. Root cause is out of scope
  for this release.

### Breaking Changes

- `GTS_ADMISSION_CANDIDATE` now defaults off; its meaning is inverted from the
  prior release. Set it to `1`, `true`, `on`, or `yes` to keep the compact
  route on.

### Performance evidence

Buildbox's tamarack harness measured the default route's in-process Go/C
ratio across three independent interleaved runs on a fixed 9-language
typical-file corpus, comparing this release's candidate code against the
prior default:

| lang | before | after | speedup |
| --- | ---: | ---: | ---: |
| go | 5.97x | 5.42x | 1.10x |
| python | 2.87x | 1.54x | 1.86x |
| typescript | 3.17x (100% fallback) | 2.31x | 1.37x |
| rust | 5.06x | 2.53x | 2.00x |
| yaml | 3.56x (100% fallback) | 2.34x | 1.52x |
| bash | 3.65x | 2.08x | 1.75x |
| markdown | 6.36x | 2.87x | 2.22x |
| lua | 4.66x | 2.33x | 2.00x |
| css | 4.60x | 2.36x | 1.95x |

The lever-2 overshoot-bound refinement (dominant-capacity growth trigger) cost
nothing measurable; the ratio table is unchanged within run-to-run noise after
it landed. Source: #1264. A downstream #454 report attributes its resolved
regressions to the production route becoming the default.

## [0.53.0] - 2026-09-19

### Release overview

- This release fixes public API contract faults and C-parity gaps that a
  repository audit found. It also includes the maintenance work merged after
  v0.52.0.
- Each parse applies its timeout once. Tree handles are safe to release in
  the C order. Incremental reuse is correct for edit sequences and changed
  included ranges.
- The default memory budget grows with the input size, so valid large inputs
  no longer stop early.
- Reserved words, query predicates, and generated grammar tables match C in
  more cases.
- Eligible fresh parses use the compact parser by default. Unsupported cases
  retain the legacy fallback. This release does not complete compact parser
  graduation.

### Performance evidence

Paired randomized benchmarks compare v0.52.0 with the v0.53.0 candidate code at
`48503fef`. The run used 20 shuffle seeds, alternating order, `-benchtime=750ms`,
`GOMAXPROCS=1`, and the `gts_parsercorephase0` tag. It ran in the harness
container with 4 GiB of memory and one pinned CPU (Intel Core Ultra 9 285).

| Benchmark | v0.52.0 | v0.53.0 | Change |
| --- | ---: | ---: | ---: |
| `BenchmarkGoParseFullDFA` | 13.099 ms, 258.6 KiB, 42 allocs | 9.637 ms, 226.5 KiB, 37 allocs | -26.43% time |
| `BenchmarkGoParseIncrementalSingleByteEditDFA` | 2,294.1 us, 184.2 KiB, 95 allocs | 127.5 us, 3.5 KiB, 5 allocs | -94.44% time |
| `BenchmarkGoParseIncrementalNoEditDFA` | 2.612 ns, 0 allocs | 4.043 ns, 0 allocs | +54.80% time |

All three time changes have p=0.000 with n=20.

- The single-byte edit gain comes from the authenticated token-invariant
  reuse that v0.52.0 disabled and that returned after it.
- The no-edit regression is 1.4 ns. The unchanged-tree fast path now adds a
  tree handle and compares included ranges. It stays in single-digit
  nanoseconds with no allocations. Recover it in a later release.
- Each returned tree now allocates one new `Tree` value, because released
  trees no longer return to a pool.

A one-shot large-file run parses the canonical `grammargen/lr.go` fixture with
`BenchmarkParityGoCanonicalFull` under `/usr/bin/time -v` in the same container.

| Measure | v0.52.0 | v0.53.0 |
| --- | ---: | ---: |
| Maximum resident set size | 151,444 KiB | 137,540 KiB |
| Bytes allocated for each parse | 4,304,096 B | 134,560 B |
| Allocations for each parse | 64,132 | 3,080 |

The run used `GOMAXPROCS=1` and `-benchtime=1x`. The raw outputs stay outside the repository.

### Parse timeout

- Apply `SetTimeoutMicros` once for each parse. The compact route and the production fallback now share one deadline.
- A parse that the compact route stopped on a timeout ran for about twice the configured timeout before this change.

### Tree handles

- Make a second `Release` on a released tree do nothing. A stale call no longer frees the tree of a later parse.
- Stop pooling `Tree` values. Each returned tree now costs one new 3,296-byte value.
- Add a handle when an unchanged incremental parse returns its old tree. Releasing the old tree no longer invalidates the result.
- Document that `ParseIncremental` updates parent links in the old tree, and that `Node.Edit` after `Tree.Edit` moves spans twice.

### Incremental edit sequences

- Clear dirty nodes on byte-identical source only when the recorded edits restore every node span. A delete-then-reinsert sequence no longer reuses a collapsed leaf.
- Parse again from the start when the parser included ranges differ from the old tree ranges.
- Compare random multi-edit sequences with fresh parses for JSON and Go.

### Memory budget

- Scale the default per-parse memory budget with the input. It is the larger of 512 MiB and 512 bytes for each input byte. Valid 7 MB JSON no longer stops with `ParseStopMemoryBudget`.
- Keep the process-heap ceiling at the larger of 2 GiB and twice the budget.
- Add `Parser.SetMemoryBudgetBytes`, `Parser.MemoryBudgetBytes`, and `WithParserPoolMemoryBudgetBytes` for a fixed budget.

### Reserved words

- Promote a reserved word to its keyword token, as C `ts_parser__lex` does. JavaScript `var if = 1;` now reports an error.
- Attach reserved-word tables for JavaScript, OCaml, PHP, Pkl, Python, and templ from generated sidecars. Blob hashes do not change.
- Add the `ts2go -reservedwords-only` sidecar mode.

### Query predicates

- Apply text predicates to every node of a quantified capture. The `any-` predicates need one matching node.
- Reject a predicate that names a capture the query has not bound, as C does.
- Keep matching unchanged after `DisableCapture`. The returned match only omits the disabled capture.

### Generated grammar tables

- Encode shift targets with 32 bits in action-group keys. Grammars with more than 65,535 states no longer merge unrelated shift actions.
- Expand case-insensitive character-class ranges without extra characters.
- Reject a production with more than 255 right-hand-side symbols.

### Continuous integration gates

- Fail continuous integration when a workflow `-run` pattern names a test that does not exist. Correct seven stale names.
- Run the exhaustive 206-language parity sweep every night.
- Require the `build` check for merges to `main`. Administrators can still bypass it.

### Stack hashing

- Pack node flags with masks and shifts without changing hash values.

### Generated CSS token precedence

- Preserve the authored precedence of named immediate tokens.
- Keep longer preferred tokens reachable after an immediate token accepts a prefix.
- Parse escaped CSS unit suffixes without introducing error nodes.

### Generated HCL splat expressions

- Preserve right associativity when a proven repeat continuation has equal precedence.
- Keep attribute and index chains inside HCL splats in nested blocks.
- Compare generated splat trees with the locked C grammar.

### Generated supertype aliases

- Use public alias symbols in generated supertype maps.
- Remove duplicate subtype entries after alias resolution.
- Test named aliases, anonymous name collisions, query captures, and the locked C map for Go's `_simple_type`.

### Test package execution coverage

- Execute 13 previously omitted test packages in continuous integration.
- Check package assignments against the workflow and use the same plan for race execution.
- Reject new test packages without an execution lane.

### Compact parser maintenance

- Move scheduler memory accounting and stop checks into a dedicated source file.
- Share symbol metadata projection between recovery and selected-store materialization.
- Include retained recovery-memo capacity in scheduler memory limits, including after reset.
- Document source ownership and the checks required for new retained state.

### Recovery symbol allocation

- Reuse one symbol table for recovery per compact parse across all language grammars.
- Count the table in the memory budget and discard it when the scheduler resets.
- Reduce allocated bytes by 81.58 percent on the real Go query compiler fixture. Parse timing shows no significant change.

### Recovery memo pressure

- Compute leaf error costs and visible counts without occupying recovery memo entries.
- Preserve error-region costs, missing-token costs, cache limits, and cleanup.

### Compact incremental allocation

- Grow borrowed incremental arenas as needed instead of reserving capacity for the complete source.
- Preserve full-parse reservation, memory limits, and cleanup after a compact decline.

### Incremental profile accounting

- Count discarded retry work without adding its reuse coverage to the selected result.
- Preserve fallback work, phase times, and maximum resource counts in incremental profiles.
- Keep reuse status and parse boundaries tied to the returned tree.

### Standalone grammar packages

- Add generated standalone packages for all 206 blob-backed grammars without build tags.
- Share scanners, decoder repairs, caches, and certified profiles through `grammars/runtime`.
- Remove the aggregate catalog dependency from the native Lean package.

### Generated source ownership

- Standardize generated Go markers and name each generator command as the
  owner. Continuous integration now rejects unknown owners, malformed markers,
  and generated file names without markers.
- Move the grammargen marker before the package clause. Go tools can now
  identify emitted grammar source as generated code.

### C parity program, round one: query semantics

- Resolve node types in query patterns the way the C query compiler does:
  only a visible or supertype named symbol is a node type. A hidden rule
  name or an anonymous token in a node pattern is now a compile error, as in
  C. The inferred tags queries use the same lookup, so a grammar whose
  `call` is a keyword no longer receives a call pattern.
- Compile a supertype node pattern such as `(expression)` into a wildcard
  step that requires the supertype among the node's hidden ancestors, and
  support the `super/sub` form with the C subtype check. Nodes record the
  hidden supertype wrappers that reduction elided in a parallel arena table
  (`Node` stays 104 bytes); the record survives final tree compaction and
  incremental clones.
- Port the C wildcard-root rule: a pattern whose root is a wildcard (a
  supertype counts) and whose first child is a concrete node type never
  tests the root. `(expression (identifier) @i)` matches every identifier
  whose parent is not an ERROR node, as it does in C.
- Wildcard steps never match ERROR nodes, and a top-level bare `_` pattern
  compiles.
- Share one Lua-pattern compiler between production queries and both C query comparisons.
  The outline comparison now evaluates `#lua-match?` predicates instead of rejecting them.
- `TestParityQuerySemantics` runs 103 query cases on both engines: 101
  agree, 2 carry a named divergence (an aliased subtype in the grammargen
  supertype map, and a hidden wrapper lost inside a compact error region).
  Highlight parity holds on 204 of 206 languages with no tolerance entry;
  hare and luau, the last two tolerated languages, now match C.
- `TestParitySupertypeMap` compares every grammar's ABI 15 supertype map
  with the C runtime: 40 languages agree, 29 diverge. The board is
  informational until the grammargen map is rebuilt.

### C parity program, round two: recovery

- Add `TestParityRecoveryBoard`: 78 malformed sources in eight languages
  parsed on the C oracle and on every Go route, compared node by node. The
  default route agrees on 36 (29 before this round); with the C recovery
  port forced on for JavaScript, 46.
- An absorbed leaf inside an ERROR region carries no error bit, as in C,
  where only a missing leaf has an error cost. The region proof that used
  to decide when a leaf could stay clean is gone.
- The A0 dispatcher census receipts for cobol and wgsl, and the cooklang
  witness digests, now pin the trees without leaf error bits. The cobol
  `MBANK30.cpy` fixture matches the C oracle exactly.
- Keyword capture follows `ts_parser__lex`: a keyword stays a keyword when
  the parse state has an action for it or reserves it; otherwise the lexer
  returns the word token. The reserved-word rule was inverted before.
- A missing leaf takes an inherited field, as a relevant child does in C.
- cpp, html, javascript, and julia stay on the legacy recovery path behind
  measured witnesses recorded in `docs/c-parity-boards.md`.
- An accepted GLR version stays out of the stack merge, as C removes it
  from the version pool, so a recovery fork created at end of input still
  competes as its own tree. An ERROR node keeps the fields a hidden child
  gave its spliced children. The recovery board moves to 36 of 78.
- The incremental invariant gate records its first two entries: python
  `setup.py` byte 1241 (delete and replace) parses without an error bit on
  both routes while C reports an ERROR, and the fresh and incremental
  parses keep a different number of GLR stacks after the site. The C
  keyword rule exposed the site; the divergence itself is older.
- Retire the Python interpolation compatibility pass after native clean-tie
  election reaches parity with the pinned C parser.
- Keep raw-shape ordering for forest-local alternatives when compact primary
  derivation selection is certified. Python f-string splats now match locked C
  on the forest route.

### Compact core cost, round two (issue #454)

- Fuse the top-down parse-state replay into the postorder materialization
  visit. The visit computes each subtree's pre-goto and parse state at push
  time with the same transition rules, so the tree needs no second
  full-derivation pass and no arena-length replay tables.
  `TestCompactFusedReplayMatchesTopDownReplay` proves the states equal the
  separate replay on every subtree.
- Remove the dead `tokenCell` election record and its five save-and-restore
  sites, read the reuse-dependency subtree count and head path count through
  narrow accessors instead of `Core.Stats`, and build the election record in
  place.
- Stop copying large records on the hot path: headers, reduction outputs,
  pop paths, boundary outputs, and canonical groups are read through
  pointers; the
  direct-append condense reads the predecessor it already resolved instead of
  validating a synthetic link and resolving it again; a zero stored cost no
  longer republishes a fresh node's lineage.
- Validate link records at node publication, including copied adjacencies.
  Single-link pop enumeration can trust immutable published records.
  The relex payload scratch no longer clears its whole buffer on
  every election, the head owner record runs without a closure per dispatch,
  and a single fresh reduction output updates its header in place.
- Earlier exploratory measurements predate the correctness review and
  benchmark lifetime fixes. They do not establish current performance gains.
  The route decision record retains them as historical measurements.
- Extract the accepted-tree visit into `compactMaterializer`, a struct the
  scheduler can drive as well as the postorder pass. The postorder pass
  now fills one scratch view in place and visits it through a pointer, and
  it can skip subtrees that already own a public node
  (`VisitMaterializationPostorderPrebuilt`). The extraction changes no
  tree and no work count.
- Add the eager materialization lane (`GTS_COMPACT_EAGER=1`). After each
  single-header shift and each in-place reduction the scheduler builds the
  new subtree's public node at once, and it builds the subtrees a
  multi-header phase left pending as soon as a single header consumes them.
  On every Go witness the lane builds the whole tree before acceptance and
  publishes the same tree, the same replay stamps, and the same work as the
  postorder pass (`TestCompactEagerMaterializationMatchesPostorder`). The
  lane stays off by default: on the Go 137 KiB witness it costs about ten
  percent more wall time, because construction interleaved with dispatch
  loses the locality of the batch pass while the compact core still writes
  every record. The lane is the construction half of the single-head kernel,
  which will stop writing compact records for subtrees that already own a
  public node.
- Skip the canonical-boundary probe when a single header holds a node the
  dispatch just published: a fresh node is the latest node of its phase
  identity, so the probe would return the head the header already holds.
  The generic shift, the in-place reduction, and the corridor direct shift
  all take the skip when the header sits outside recovery isolation with no
  pending freshness; the skip records the barrier, the header peak, and the
  verifier binding, so every work vector and receipt stays identical. Parents take their span
  from the point index only when their visible children do not tile the
  record, and a reduction sums its pop payload work once.
- Turn the C4 bytecode corridor on by default (stage 3 of
  spec.c4-bytecode-isa.v1). The 137 KiB full-parse comparison is faster on
  14 of 15 grammars. A JavaScript recovery mutation once changed the C tree
  with the lane on; the lane now stays off while a version-owned lexer
  request is live, and the evidence for the default is: the runtime
  equivalence test keeps every work count and digest equal; the exhaustive
  curated structural parity suite (fresh, incremental, no-error) passes on
  every grammar with the lane on; the pinned-oracle T3 recovery adjudication
  in the harness container matches C on every html and JavaScript witness
  with the lane on; and the JavaScript recovery mutation differentials pass
  in both modes. `GTS_C4_CORRIDOR=0` turns the lane off.
- Keep version-owned lexer requests on the generic dispatch path. The
  corridor reads a shared token and cannot publish an owned request.
- Preserve separate canonicalization output buffers for single headers.
  Reusing the input slice changed earlier snapshots and broke rollback isolation.
- Answer point lookups from the line of the previous answer or the next
  line before the hashed cache and the binary search: materialization asks
  for points in source order. Skip the scanner-provenance search for a
  terminal that cannot carry an entry, and the skipped-prefix search when
  no prefix was recorded. Together about 3 percent on the Go 137 KiB
  witness.

### Production engine fixes kept until retirement (issue #454)

The compact route stays the default fresh full-parse route. The owner's
direction is to retire the production engine once the compact core
outperforms it; until then production still serves incremental, injection,
included-range, and fallback parses, so these fixes stay.

- Isolate parser scratch lifetimes across parses. A pooled scratch kept the
  transient parent and child slabs of the largest earlier parse, up to 512K
  elements, and billed them to every later parse in the process: a 4 KiB
  parse after a 315 KiB parse reported 35 MB of inherited scratch. Each parse
  now drops inherited transient slabs above four times its own initial arena
  estimate before it starts. A new small-large-small test guards the bound
  through the new `ParseRuntime.TransientScratchBytesAllocated` counter.
- Shrink `Token` from 80 to 64 bytes. The five unexported provenance bits
  pack into one flag byte, and the stack position behind a synthetic missing
  token moves to a parser-owned anchor table that the token indexes. Tokens
  are copied by value on every election and dispatch, so the size shows up
  directly as copy cost on both routes. The public fields are unchanged.
- Bound reuse-hostile incremental parses. An old-tree reuse parse that has
  built four times the larger of the old tree's nodes and the fresh-parse
  arena estimate while reusing under one eighth of the source now stops with
  `ParseStopReuseBudget`, and the parser runs one plain full parse, the same
  fail-closed retry the memory budget uses. The issue #454 C single-byte
  delete built 3.2 million nodes before the memory budget stopped it; it now
  stops near 370 thousand and returns the fresh-parse tree. The profile names
  the retry `incremental_parse_reuse_budget_full_retry`.

### Compact route repair (issue #454)

- Repair the three regressions on the compact candidate route that issue
  [#454](https://github.com/odvcencio/gotreesitter/issues/454) measured on
  137 KiB editor fixtures.
- Compact error recovery scales linearly. The recovery cost memo grew to the
  exact size on every store and was reallocated on every call, so a fresh
  parse of a 16 KiB Go file with one syntax error took 4.9 seconds. The memo
  now grows geometrically and lives for the whole parse. The same parse takes
  49 milliseconds, and the 137 KiB single-byte delete completes in 178
  milliseconds instead of never.
- Compact-materialized old trees reuse top-level siblings under the same
  compatible-goto contract as production trees. TOML insert reuse returns
  from 62 percent to 100 percent, and TypeScript from 54 percent to 98 percent.
- A synthesized root no longer disables reuse for the whole tree. INI files
  that end in a blank line return from 0 percent to 97 percent reuse. The
  unsupported-reuse reason now names the clause that failed.
- The compact incremental attempt declines after eight unauthenticated
  in-scope candidates or 32 KiB past the edit with zero reuse, so INI and
  JSON no longer pay a discarded whole-file compact parse per keystroke.
- The compact scheduler checks its current footprint at every memory-budget
  poll. A cached small footprint did not detect subsequent storage growth.
  Regression tests cover both the memory budget and the hard ceiling.
- The compact scheduler skips avoidable per-token work: the checkpoint
  interner compares against the last interned
  record before hashing, the relex probe authenticates its payload by byte
  comparison instead of SHA-256, and the materialization walk passes records
  by pointer. Earlier performance measurements predate the review fixes.
  Run randomized comparisons before reporting gains for the corrected code.
- Halt a production GLR stack at a no-action point when a sibling stack
  accepts the lookahead, before the previous-shift recovery runs. Pull
  request [#709](https://github.com/odvcencio/gotreesitter/pull/709) added a
  per-stack re-lex that kept the constructor-specifier fork of
  `static inline void f(int *v) {}` alive, and the recovered fork won
  selection with an ERROR node. Five C++ witnesses now match the compact route.
- Add `cmd/issue454bench`, which reproduces the downstream measurements on
  synthetic fixtures with an optional CPU profile.
- Serve a mid-file transient-error keystroke on a compact old tree with
  production incremental reuse, the v0.48.1 mechanism, when the compact
  borrow attempt declines at recovery. The fresh compact recovery route never
  produced those trees; it declined after a whole-file pass. Edits within 256
  bytes of end of file keep the compact recovery route. Go single-byte deletes
  drop from 178 to 15 milliseconds at 137 KiB, and every measured tree equals
  the fresh default-route parse except two pre-existing divergences that the
  new parity gate documents.
- Skip the fail-closed whole-file reparse after an incremental parse whose
  errors sit inside top-level items covering at most a quarter of the source.
  Pull request #613's wide-stack condition fired on TypeScript's ordinary GLR
  ambiguity, so a single-byte delete at 137 KiB cost 296 milliseconds against
  78 at v0.48.1; it now costs 75. Degenerate results still retry.
- Decline an unpublishable compact recovery when its region commits instead
  of after a whole-file pass. A fresh compact parse of a 137 KiB Go file with
  a mid-file error drops from about 424 to 275 milliseconds; the production
  parse alone costs 240.
- Shrink `Token` from 88 to 80 bytes, memoize the scanner identity
  fingerprint per parse, and compute per-election checkpoint receipt digests
  only under full receipts. Scala and CMake compact full parses gain about
  another 12 percent.
- Cut the production engine's drift since v0.48.1, which both routes
  inherit. The token source passes tokens by pointer through its per-token
  helper chain instead of copying 80 bytes about ten times per token, both
  lexers decode the frontier rune only for non-ASCII bytes, the contextual
  close-angle probe checks the token bytes before symbol names, and the
  external scanner failure-mode probes are answered once per language.
  Production full parses of 137 KiB fixtures move from 1.1 to 1.4 times
  v0.48.1 to 1.06 to 1.19 times, with Rust at 1.33. The report attributes
  the remaining gap and records the compact route's graduation status.
- Remove four avoidable per-token costs from the compact scheduler: the
  cap-pressure poll reads the node count without validating the head, the
  per-state relex probe caches the scanner contract and identity and uses
  scheduler-owned snapshot scratch, the election reads the cached checkpoint
  identity instead of asking the order adapter, and the reuse-proof
  invalidation takes the lineage record by pointer. Go compact full parses
  gain 7 percent; the other grammars are within 2 percent.

### Compact parser correctness

- Authenticate terminal aliases at ordinary grammar reductions during recovery. Preserve separate rules for synthetic ERROR reductions and retain span coverage checks.
- Preserve inherited alternative history when a reduction joins an active sibling.
- Retain convergence and resurrection restrictions after adoption. Preserve existing blended-history rejection checks.
- Parse `foo<A00>(2);` through compact without fallback, with exact locked-C tree parity.
  Malformed TypeScript recovery remains unfinished.
- Preserve numeric-edit reuse for newly admitted TypeScript trees after authenticating lexer dependencies and scanner equivalence.

### Incremental correctness

- Restore bounded token-invariant reuse after authenticating earlier lexical
  dependencies and the edited token. Unknown coverage requires reparsing.
- Retain examined-byte coverage through failed scans, rollback, and accepted
  tree ownership. Include UTF-8 continuation bytes beyond token boundaries.
- Add optional scanner byte-equivalence declarations. These declarations do
  not authorize general subtree reuse or replace scanner checkpoints.
- Check repeated edits against locked C trees for Go, CSS, SCSS, TypeScript,
  and Julia. Keep malformed and unsupported cases on their existing fallback paths.

Twenty paired benchmark samples compare this change with v0.52.0.
Generated Go single-byte edits improve from 3,033.2 to 182.1 microseconds.
Allocations decrease from 95 to 3 per edit. Full parsing regresses 1.68 percent.
See [pull request #1093](https://github.com/odvcencio/gotreesitter/pull/1093) for the restoration and its validation.
These changes do not complete compact parser graduation or retire the legacy parser.

## [0.52.0] - 2026-09-05

### Release overview

- Eligible fresh parses use the compact parser by default. Unsupported cases
  retain the legacy fallback. This release does not complete compact parser
  graduation.
- Compact incremental parsing now reuses authenticated unchanged subtrees.
  The bounded route includes nested nonterminals with lexer dependency proofs.
  Edits invalidate affected proofs, including dependencies on examined suffix bytes.
- The authenticated Go profile supports bounded end-of-file recovery with
  native error-tree construction. Unsupported recovery cases retain fallback.
- The detailed entries below preserve the development history, including
  rejected experiments, superseded measurements, and unresolved parity blockers.

Issue [#1087](https://github.com/odvcencio/gotreesitter/issues/1087) remains open.
This release temporarily disables the unsafe same-width token-invariant shortcut.
Real edits reparse through the supported incremental or full-parse route.
Ordinary subtree reuse and no-edit reuse remain available.
Restore the shortcut only after complete lexical dependency proofs pass.
The full correction remains deferred; this mitigation does not close the issue.
The owner approved the temporary slowdown. The measured single-byte edit rises
from 1.706 us to 3,350.460 us, with 184.4 KiB and 95 allocations per operation.
Full-parse and no-edit timing changes are not significant. Full parsing reduces
allocated bytes by 42.90 percent and allocations by 99.72 percent.

Lexical error-leaf flags and TypeScript recovery divergences remain graduation work.
The attempted flag correction changed AWK recovery selection and remains deferred.

### Performance tooling and optimizations

- Pull request [#1089](https://github.com/odvcencio/gotreesitter/pull/1089)
  restores paired performance evidence and authenticated work counts.
  The paired driver alternates execution order and rejects incomplete evidence.
- Pull request [#1090](https://github.com/odvcencio/gotreesitter/pull/1090)
  caches recovery visible-subtree counts and removes canonicalization callback
  allocations. The cache reduces time across four frozen Go fixtures.
  The callback change reduces allocations without a significant timing change.
  These measurements precede the temporary shortcut mitigation.

The owner authorized a v0.52.0-only exception for an unsupported tag-creation
actor rule. All other publication gates remain mandatory. See
[the dated exception](docs/releasing.md#v0520-only-tag-creation-exception).

### Performance

- Bound temporary conflict-frontier growth with the maximum of the resolved
  stack-cull trigger and the four-stack full-parse overflow window. The
  reported pinned-file result was 250.8 seconds with an error root. The
  changed parser completes the pinned file in about 3 seconds with a clean
  root and no truncation. The raw non-API benchmark route retains its
  historical unbounded frontier search. Exact C# route and census checks pass,
  as does uniform initializer parity through 72 entries. The cap does not fix
  the separate 87-byte branch limitation.

- Reduced the temporary recovery memo tier from 262,144 entries to 131,072
  entries. The active table now uses 3 MiB on 64-bit systems. The retained
  standard table uses 384 KiB. Total memo storage is 3.375 MiB before slice
  overhead. The 20-seed C# issue #454 benchmark reduces bytes per operation
  by `20.79%`, with `p<0.001`. Time is `155.9` ms for base and `158.0` ms for
  the changed build, with `p=0.529`. Allocations are `998.5` and `998.0`, with
  `p=0.074`. Both results are inconclusive. Two execution orders reduce
  maximum resident set size by `11,520` KiB and `4,160` KiB. The primary trio
  is order-sensitive and inconclusive. Focused C# correctness, locked-C
  parity, and memory-budget contract tests pass.

- Recorded P25aw at merged main commit
  `af9ded2b77b7828b12b1d2da7c9fff8dd5ca053b`. Four authenticated Swift corpus
  edits used old trees and one final-newline edit. Every profile reported
  `external_scanner_unsupported`, zero reused subtrees, zero reused bytes, and
  no incremental arena route. The result is **NO-GO / NO WITNESS**. No
  six-seed, 20-seed, or maximum resident set size (RSS) run was justified.
  See `docs/perf-attribution.md` for exact commands, identities, and artifacts.

- Recorded P25at at publication base `b65a9c235915edc3198851cf07b0257e3caed6d6`.
  Measurements used `3c2a2106102769bab891047174dbcfec15045e74`; relevant
  source files are unchanged between these bases.
  `nodeArena.ensureNodeCapacity` ran twice for authenticated recovery.
  The exact-fit candidate improved recovery bytes per operation (B/op) by
  `9.39%`, but increased allocations by `4.92%`. Primary full parse time
  increased by `+7.95%`.
  Primary full parse B/op increased by `+7.88%`. Primary edit time increased
  by `+12.47%`.
  The candidate is **NO-GO / REVERTED**. No 20-seed or maximum resident set
  size (RSS) run was justified.
  See `docs/perf-attribution.md` for exact identities, commands, and artifacts.

- Recorded P25ay at base `af9ded2b77b7828b12b1d2da7c9fff8dd5ca053b`.
  The probe measured hidden-field flattening across the primary trio, recovery,
  and JavaScript control.
  Recovery made 2,207 helper calls and appended 1,213 nodes.
  JavaScript made 399 helper calls and made 25 deferred field calls.
  Primary no-edit made zero helper calls.
  P25ab showed that the helper was not the dominant allocation source.
  No grammar-agnostic candidate met the proof boundary.
  The result is **NO-GO / NO CANDIDATE**.
  No six-seed, twenty-seed, or maximum resident set size (RSS) run was justified.
  See `docs/perf-attribution.md` for exact commands, artifacts, and hashes.
- Recorded P25az at main commit
  `6eed698a13e7371fa978adb893e8b89ad1cd81ba`. The recovery-prefix merge
  contract passed before and after the test-only contract addition. The helper
  remains a coarse synthetic-profile signal. No production candidate met the
  proof boundary. The result is **NO-GO / NO CANDIDATE**. No randomized
  benchmark or maximum resident set size (RSS) run was justified. See
  `docs/perf-attribution.md` for exact commands, artifacts, and hashes.

- Recorded P25ba at main commit
  `d72987b44b76cf39aa4ad0f5fff03860eed7cd0d`. Canopy selected the
  `cNodeErrorCostLangWithScratch` recovery-cost contract after the queued P25az
  prefix aggregate arm. Focused recovery-cost and memo tests passed in Docker.
  No authenticated issue #454 witness supplied call-depth or cache-hit proof.
  The result is **NO-GO / NO CANDIDATE**. No benchmark or maximum resident set
  size (RSS) run was justified. See `docs/perf-attribution.md` for the command,
  artifact hashes, and reopening condition.

- Recorded P25bb at main commit
  `83e0cfbc30ad82e2f327d58e35eea9f438a0ffda`. Canopy selected the
  `cRecoverStrategy1Election` summary contract after P25ba. Focused summary,
  election, scratch-reuse, and allocation tests passed in Docker. No
  authenticated issue #454 witness supplied election depth or reuse proof.
  The result is **NO-GO / NO CANDIDATE**. No randomized benchmark or maximum
  resident set size (RSS) run was justified. See `docs/perf-attribution.md`
  for the command, artifact hashes, and reopening condition.

- P25bc records the main commit
  `83e0cfbc30ad82e2f327d58e35eea9f438a0ffda`. Canopy selected the
  `cDoAllPotentialReductions` reduction-search contract after the queued P25bb
  election-summary arm. Focused reduction, version-order, summary-lifetime,
  and pointer-release tests passed in Docker. No authenticated issue #454
  witness supplied reduction-attempt or allocation proof. The receipt marks
  the result **NO-GO / NO CANDIDATE**. The proof boundary did not justify a
  randomized benchmark or maximum resident set size (RSS) measurement. See
  `docs/perf-attribution.md` for the command, artifact hashes, and reopening
  condition.

### Correctness

- Bind identity-bearing external scanner checkpoints to compact per-version
  lexer snapshots. Reject incomplete identities before publication. Reject
  changed identities before restore or owned snapshot publication. Do not
  share an equivalent sidecar after identity drift. Forward identities through
  scanner order adapters. Preserve legacy scanner behavior.

- Graduated the Markdown inline smoke route with four exact, compact-only
  conflict policies bound to the built-in grammar blob. Three repetition
  reductions require at least two live compact headers. The HTML entry row
  selects its sole shift. All other repetition rows still fail closed.
  The locked upstream corpus reports 15 direct routes and 348 fallbacks across
  363 cases. Every direct tree matches C exactly. The nested-emphasis
  counterexample remains a required fallback. The admission scorecard now
  reports 201 PASS, zero DIVERGE, zero FALLBACK, five SKIP, and zero ERROR rows.

- Apply the production route's contextual close-angle deferral in the
  compact dispatch and the C4 corridor (issue #983). Add the two Swift
  witnesses to the committed route-equality seed corpus. Totals stay
  unchanged: 69/0/30/10 canonical, 88/0/46/12 full. Re-adjudicate the
  `swift_log_1` and `swift_log_2` rows in the T3 oracle witness manifest:
  the compact route now reports the error tree, so `compact_has_error`
  moves from false to true and all three runtimes agree on both rows.

- Added bounded materiality-only acceptance for no-primary multi-derivation
  end-of-file (EOF) frontiers. The route selects a deterministic provisional
  candidate only for the existing bounded public-tree comparison. It routes
  only when every live candidate materializes to the same public tree. The
  comparison caps live candidates at eight and fails closed on a cap, missing
  context, materialization failure, or tree mismatch. The C# 642-byte
  `variableDeclarations.cs` row now routes. The 97-row matrix moves from 62
  PASS, 27 FALLBACK, and 8 SKIP to 63 PASS, 26 FALLBACK, and 8 SKIP, with zero
  DIVERGE or ERROR rows. Production, compact, and locked-C report the exact
  deep digest `005b39bd9a68ff9775129d3fb793b9d7a58b9f56812bb9ca9bd0eb753465dd86`.
  The C# collapsed-token regression fixture and three Dart constructor probes
  (class, private, and enum) route without a dispatch pass and with exact
  locked-C trees.
  The change grants no language profile or digest grant.

- Added a stored cumulative maximum for dynamic precedence, which ranks
  competing parse paths. Each node keeps C's running value. Merge
  transactions seed the fold from that stored value and apply the C rule
  table from `stack_node_add_link`: a same-pair assignment overwrites the
  value and can lower it, mergeable and appended links max the incoming
  predecessor's stored value plus the payload contribution, and duplicate
  or lower same-pair links change nothing. Publication never recomputes
  the value from the final link array. Leaf payloads contribute zero. The
  rule-table tests in `internal/parsercorephase0/precedence_rule_table_test.go`
  pin one case per C branch. The canonical real-corpus ratchet now pins
  69 PASS and at most 30 FALLBACK over 109 rows. The `nodeRecord` grows
  from 24 to 32 bytes. The Haskell small
  `Main.hs` row has 260 bytes and source SHA-256
  `c60b55de99836dacb00a0f2808835895132996a95d52304cefab548b8cbdef65`.
  It routes with counters `1/0` and real recursive link-union work. Production,
  compact, and locked C report deep digest
  `6e3806c54c39af93701157f87ac8ee9ef947108d66b7d6069d6908a4d5c71e9f`.
  The matrix moves from 63 PASS, 26 FALLBACK, and 8 SKIP to 64 PASS,
  25 FALLBACK, and 8 SKIP, with zero DIVERGE or ERROR rows. The eight-link
  cap remains. No language profile or grammar digest grant was added.

- Raw accepted-leaf coverage now preserves hidden terminals and exact `ERROR`
  provenance with bounded reusable scratch and cancellation polling. The
  authenticated matrix moves from 60 PASS, 29 FALLBACK, and 8 SKIP to 62 PASS,
  27 FALLBACK, and 8 SKIP, with zero DIVERGE or ERROR rows. Both HTML rows now
  convert directly. Isolated HTML Docker parity is 25/25.

- Added a default-off Core link provenance sidecar. It stores finalized
  drop-cohort reference indexes by LinkID. Compact routing and admission stay
  unchanged.

- Added a default-off reduction output provenance handoff. It carries an
  authenticated graph-chain reference from published node metadata. The
  reference follows `next` links and does not require adjacent LinkID values.
  It performs no adjacency walk and changes no derivation format, routing,
  admission, or compact counters.

- Included-range parsing now starts byte-seek sources at the first selected
  byte. The parser seeds its initial stack there and preserves the configured
  start point for recovery gaps. A non-seek source preserves a complete
  overlapping token because it cannot reproduce a trimmed boundary token.
  For grammars without an external scanner or external symbols, the internal
  DFA now advances across selected gaps before token acceptance. It preserves
  logical text and cursor state during reset, relex, and generalized LR (GLR)
  probes. A scannerless synthetic external token now advances the source to
  its reported end before the next token. The cursor increases the AMD64
  Parser layout by 40 bytes, from 2,184 to 2,224 bytes.
  The locked Go route now matches C at root byte 26. Child counts remain 10
  for Go and seven for C. Compact and forest included-range routes remain
  uncertified. Keep `dispatch.go.source-file-root` live until exact shape and
  all required route proofs pass. See `docs/root-normalization-retirement.md`.

- Recorded a generic Swift issue #576 recovery candidate at base
  `da6f71471aaaa835503accaa1bc2083ced90b4e6`. The active deterministic finite
  automaton (DFA) source now replays recovery from the exact skipped-prefix
  offset. It requires the original span, points, token identity, and unchanged
  scanner state. It resynchronizes the source before it emits `errorSymbol`.
  Recovery replay requires a non-empty scanner checkpoint and live state. A
  checkpointless scanner rejects recovery replay without changing the scanner.
  Generic relex still accepts stateless scanners with empty serialization. The
  Swift scanner now serializes its complete state for the recovery proof. The
  scanner no longer claims that failed scans preserve this state. The token
  source restores failed scans by default. A scanner can explicitly retain a
  required failed-scan state change. Swift uses that capability and records
  distinct start and end checkpoints. Incremental fast-forward restores the
  recorded end checkpoint. Generic relex rejects a synthetic end-of-file
  lookahead that would replace a real token. Swift defers `>?` to the DFA only
  when an unmatched `<` exists in the active scope.
  The parser records error-mode lexing only after the DFA produces the token.
  An outer parse operation resets the recovery memo size. Nested retries stay
  warm. Temporary memo growth restores an existing 16,384-entry standard slab.
  A shorter retained slab remains short. Two pooled tests protect the recovery
  lifecycle. The parser returns a rejected recovery probe before its legacy retry. Entry
  scratch now restores the required large-parse reservation after a small
  pooled parse. The 20-byte witness matches the locked C tree. The
  `stdlib_FloatingPointToString.swift` and `stdlib_CollectionAlgorithms.swift`
  witnesses still differ from locked C. The 20-seed repair comparison found no
  significant primary timing change. Bytes and allocations stayed unchanged.
  One warmed large-witness sample decreased 0.113090 percent. Keep issue #576
  open. See [the Swift #576 blocker receipt](https://github.com/odvcencio/gotreesitter/blob/6b6d49341699df9314b77d52ea92dc950e7364e4/docs/swift-576-compact-correctness-blocker.md).

- Recorded the unreceipted `dispatch.julia` blocker at base
  `b35b86cb84d620305515abf970d5598c9573a48b`. The focused Docker receipt
  covers raw, production, compact, forest, incremental, and locked-C routes.
  It pins the Julia grammar, blob, manifests, C artifact, runtime, binding,
  compiler, scanner, source hashes, route digests, divergences, compact
  outcomes, dispatch counts, and incremental reuse. The recovered return-range
  witness flips the root error flag. The checked-in Julia source keeps a type
  divergence. Compact falls back on two witnesses, and the authenticated corpus
  lock is absent. Keep the arm live. No parser or registry change is included.
- Recorded the next single-grammar normalization blocker, `dispatch.kotlin`,
  at base `d72987b44b76cf39aa4ad0f5fff03860eed7cd0d`.
  The focused Docker gate used the parser-produced witness
  `tasks.named<KotlinCompile>("compile") {}`.
  The raw and production deep digests differ.
  Production records `dispatch.kotlin` as checked 1, run 1, visited 22,
  and rewritten 23. The recovered-root subpass records zero rewrites.
  Keep the parent arm live. Stop before compact, strict forest, edited
  incremental, fatal locked-C, survivor, registry, or dead-reference proofs.
  Reopen only after native Kotlin derivation emits every listed shape without
  parent rewrites or parser-state changes. No parser or registry change ships.
  See `docs/root-normalization-retirement.md`.

- Recorded a TypeScript current-main follow-up at base
  `ab84e809297e945a6debd52ec3e211956b497893`.
  The one-CPU Docker gate reran raw, production, compact, forest, incremental,
  locked-C, scanner, and cap-one typed-arrow routes.
  The tracked witness keeps 15 production rewrites, 10 incremental rewrites,
  406 reused subtrees, and 2,289 reused bytes.
  Compact keeps its scheduler-frontier fallback, and the authenticated
  TypeScript and TSX corpus lock remains absent.
  Keep the arm live. No parser or registry change is included.
  See `docs/root-normalization-retirement.md`.

- Recorded the next unreceipted `dispatch.templ` blocker at base
  `3c2a2106102769bab891047174dbcfec15045e74`. The A0 manifest records three
  Templ files, three checks, three runs, 1138 visited nodes, 76 rewrites, and
  one error root. The focused Docker receipt covers raw, production with
  admission forced off, compact, forest, incremental, and locked-C routes.
  It pins source, manifest, grammar lock and blob, C artifact, runtime,
  binding, compiler, scanner identity, route digests, divergences, and dispatch
  counts. It also pins compact counters and reasons, forest outcomes, and
  incremental reuse. Templ has no authenticated corpus lock. The medium
  witness retains a locked-C divergence. The small witness matches locked C
  only after dispatch.
  Compact falls back, and the external scanner provides no incremental reuse.
  Keep the arm live. No registry or production code changes are included. See
  `docs/root-normalization-retirement.md`.
- Recorded the unreceipted `dispatch.wgsl` blocker.
  The evidence and measurement base is
  `cf58fba517ed4fa6a8f5d1328ac2f850d48a8c75`.
  The review publication base is
  `d134ed5f963c7ed1d27fa1247aeb2a16746ab585`.
  The receipt covers three A0 WGSL witnesses and two controls.
  It covers raw, production, compact, forest, incremental, and locked-C routes.
  It pins source, grammar, blob, C artifact, runtime, binding, compiler, and scanner identities.
  It also pins route digests, divergences, dispatch counts, compact outcomes, and reuse counts.
  The one-CPU Docker test passed.
  Keep the arm live because production rewrites remain.
  Compact also falls back on recovery.
  Locked C still differs in shape, type, and error state.
  No parser or registry change is included.
  See `docs/root-normalization-retirement.md`.
- Recorded the unreceipted `dispatch.perl` blocker at base
  `6eed698a13e7371fa978adb893e8b89ad1cd81ba`.
  The focused Docker receipt covers raw, production, compact, forest,
  incremental, and locked-C routes for two source-hashed Perl witnesses.
  It pins the grammar lock, blob, scanner, C artifact, runtime, binding,
  compiler, source digests, route digests, dispatch counts, compact counters,
  forest results, and incremental reuse status.
  Raw trees diverge from locked C, while normalized routes match after dispatch.
  The Perl corpus lock is absent, and the scanner provides no incremental reuse.
  Keep the arm live. No parser or registry change is included.
  See `docs/root-normalization-retirement.md`.

- Recorded the unreceipted `dispatch.wolfram` blocker at base
  `f8b9d718ee19f65598e274035f5481a899ab2b72`. The A0 manifest records three
  Wolfram files, six checks, six runs, 77 visited nodes, zero rewrites, and
  three error roots. The receipt covers raw, production, compact, forest,
  incremental, and locked-C routes. It pins the grammar lock, embedded blob,
  C artifact, runtime, binding, compiler, grammar repository, source digests,
  route digests, divergences, dispatch counts, compact deltas, and external
  scanner identity. The incremental profile records
  `external_scanner_unsupported` with zero reuse. Other routes log scanner
  identity without inventing reuse fields. The full authenticated corpus is
  absent. Keep the arm live.
  No registry or production code changes are included. See
  `docs/root-normalization-retirement.md`.
- Recorded the C26y SQL compact checkpoint trace on base
  `e24ccf5a87bbd7febc21f67f014c2d5301d229d0`. The pinned source uses commit
  `587f30d184b058450be2a2330878210c5f33b3f9`. The grammar and scanner source
  hashes are `42f011860137175a5a0cb820d1a694e5ccca1d17f226729ff6a4e886910cde1c`
  and `d437ad9f517d7a1f4248ccd05abe58370b5040c0037c877dab1f0aefeaa04af6`.
  The semantically correct locked-blob and target-symbol routes share four
  scanner transitions. At `7-9`, the state changes from `00` to `242400`.
  At `9-12`, it remains `242400`. At `12-14`, it changes to `00`. At `14-15`,
  it remains `00`. The direct generated route emits an error tree with
  13/4/4 values. Correct production routes record 14/5/5. Compact admission
  records zero sidecars and zero incremental reuse. Reject the compact
  candidate.
  Keep SQL compact admission gated until compact sidecars and safe nonzero
  reuse have a generic proof.

- Screened generic compact-admission sidecars for checkpointed scanners in
  C26z. The diagnostic SQL route recorded 14 records, 5 leaves, and 5
  snapshots with the correct `7-9`, `9-12`, `12-14`, and `14-15` scanner
  transitions. Generated, locked-blob, and locked-C trees matched. Compact
  reuse was 1 subtree and 6 bytes. Production reuse was 1 subtree and 16
  bytes. The candidate was reverted after C26aa traced the gap to compact
  parse-state replay. Generated content replay state `180` made checkpoint
  scanning return error span `9-14`. Locked production state `16613` returned
  expected span `9-12`. A stale grammar identity reused zero subtrees and zero
  bytes. Keep SQL compact admission gated.

- Recorded the C26ad SQL compact replay state-provenance blocker at publication
  base `515df769b9b4e2f8e3ea715e78b75a44faa3b6d6`. The evidence base was
  `cf58fba517ed4fa6a8f5d1328ac2f850d48a8c75`. The pinned source uses commit
  `587f30d184b058450be2a2330878210c5f33b3f9`. Its grammar and scanner source
  hashes are `42f011860137175a5a0cb820d1a694e5ccca1d17f226729ff6a4e886910cde1c`
  and `d437ad9f517d7a1f4248ccd05abe58370b5040c0037c877dab1f0aefeaa04af6`.
  Generated and locked SQL tables assign different state and external-symbol
  identities. Generated state `180` is valid with generated symbol `287` and
  scans content `[9,12)`. Locked state `16613` is valid with locked symbol `286`
  and scans the same span. A numeric remap cannot prove equal parser
  transitions across tables. The clean Docker route still fails generated
  dollar-quoted parity with the generated scanner's reference symbol mapping;
  temporary target-symbol binding repaired that route but did not establish a
  generic replay contract. No parser, scanner, or test change ships. Keep SQL
  compact replay gated. See `docs/compact-route-real-corpus-matrix.md`.

- Added the C26ag generic table-identity guard at base
  `ae6e49448be249dd52dca5a95ba187fdd3000fe6`. The compact core captures the
  producer identity at construction. Loaded languages use the exact grammar
  blob SHA-256. In-memory languages use one process-local producer token.
  Replay now declines before state reconstruction when the producer identity
  changes or is absent. The guard preserves same-language replay and does not
  translate numeric parser states. Focused identity tests passed in Docker.
  SQL scanner unit tests passed. The generated SQL route still has the known
  dollar-quoted locked-C divergence. Keep SQL compact admission gated until
  that parity gap and the complete replay contract are resolved. See
  `docs/compact-route-real-corpus-matrix.md`.

- Recorded the C26ah generated SQL dollar-quote producer blocker at base
  `d72987b44b76cf39aa4ad0f5fff03860eed7cd0d`.
  The 16-byte witness `SELECT $$hey$$;\n` has source SHA-256
  `c65f30545110c37897fb7fe364af31ff572b35796963ec8fc59b37b76d319912`.
  The generated tree has three source-file children and an error.
  The checked-in and locked-C trees have two children and no error.
  Their deep digest is
  `f093882f4f27897036dd245c3e17f1dad2d7cd72e470e8594b5492150a2c451e`.
  The generated digest is
  `7ae91c65aebee3a10eeef68804af8b74bbf27620b200dfe842916898fc90dd46`.
  Compact admission recorded zero routes and one fallback.
  The fallback used the generic recovery no-table-action reason.
  Keep SQL compact admission gated. No parser, scanner, or grammar change
  ships. See `docs/compact-route-real-corpus-matrix.md`.

- Recorded the C26ai tagged SQL dollar-quote producer witness at base
  `7a14a701eb2a5d623ce128e792ee67820a734c8b`.
  The 22-byte witness `SELECT $tag$hey$tag$;\n` has source SHA-256
  `1279d93f715690fee6c8af53fa774d0108c19846d9418d86d53edec0d743bc88`.
  The generated tree has three source-file children and an error.
  The checked-in and locked-C trees have two children and no error.
  Their deep digest is
  `824c8bdf7107be3632bd0a43d89e0324ebc0802dcd8c4dca014130224f930ef6`.
  The generated digest is
  `ba05964c2f2c62e56a3ee9470c76dc55098f7c2ea4f656607da30c5f8af212d4`.
  The first divergence is the generated root child count.
  Keep SQL compact admission gated. No parser, scanner, or grammar change
  ships. See `docs/compact-route-real-corpus-matrix.md`.

- Recorded the C26aj generated SQL `CREATE DOMAIN` producer witness at base
  `83e0cfbc30ad82e2f327d58e35eea9f438a0ffda`.
  The 19-byte witness `CREATE DOMAIN test;` has source SHA-256
  `94c5d360b7205e2bd6e84fa28efc2cb3ee2cbc89aa6b759dd3349e578dd133c8`.
  Generated and checked-in trees are both clean with two root children.
  The generated `CREATE_DOMAIN` node has one child; locked C has none.
  The generated digest is
  `f08d628fa30d83cf92352dbfcab4885b7422ac0fadde9c756e747e6c116dc044`.
  The checked-in and locked-C digest is
  `e72fb9fb57180d5db5be9b649969c7e722d20e1ea4040075260258ee293ce630`.
  Keep SQL compact admission gated. No parser, scanner, or grammar change
  ships. See `docs/compact-route-real-corpus-matrix.md`.

- Recorded the C26ak generated SQL `CREATE DOMAIN AS` producer witness at base
  `83e0cfbc30ad82e2f327d58e35eea9f438a0ffda`.
  The 27-byte witness `CREATE DOMAIN test AS text;` has source SHA-256
  `0cad93f1b70ffa192008df866587c05b206ee404cedd5a0025f542c39c6a504b`.
  Generated and checked-in trees are both clean with two root children.
  The generated `CREATE_DOMAIN` node has one child; locked C has none.
  The generated digest is
  `67366d68529459613040eb60c5eef47b371fd3091dbe3283c82d7bcff287ea9c`.
  The checked-in and locked-C digest is
  `d0daa4279f00eb8c7278992cff6a870c98bf3deee2292b28f997adbe2f434916`.
  Keep SQL compact admission gated. No parser, scanner, or grammar change
  ships. See `docs/compact-route-real-corpus-matrix.md`.

- Added authenticated generated-SQL scanner identity to the grammargen C
  parity route. The route now reloads the generated blob through `LoadLanguage`
  before scanner adaptation. The scanner binds checkpoints to exact generated
  bytes. The focused edit reuses 1 subtree and 16 bytes after recording 13
  checkpoint records, 4 checkpoint leaves, and 4 snapshots. Fresh and
  incremental trees match. A stale
  reference grammar identity reuses zero subtrees and zero bytes. The four
  generated SQL locked-C divergences remain. Keep generated SQL compact parity
  gated until generated and locked-C trees match.

- Recorded the `dispatch.solidity` blocker at publication base
  `e24ccf5a87bbd7febc21f67f014c2d5301d229d0`. Keep the arm live. The A0
  manifest records three Solidity files, three checks, three runs, 26897
  visited nodes, and 666 rewrites. The source files use OpenZeppelin commit
  `48ab75f29abaa315fad7fa7b8338f92bb07376a7`. The grammar lock, Solidity
  blob, C grammar artifact, runtime, binding, and grammar repository identities
  are pinned. The receipt covers raw, production, compact, forest, incremental,
  and locked-C routes. Production forces admission off, and the Docker run sets
  `GTS_ADMISSION_CANDIDATE=0`. Compact records exact routed and fallback deltas
  for its accepted and fallback cases. The focused receipt reports locked-C
  divergences for Initializable, call aliases, malformed controls, and forest
  witnesses. The authenticated Solidity corpus lock is absent. No registry or
  production code changes are included. See
  `docs/root-normalization-retirement.md`.

- Added the C26q SQL scanner identity gate on publication base
  `a62b9db306bcb983852cbf0043852546e864e856`. Native SQL binds each
  checkpoint to its scanner and exact grammar blob. Generated overrides hash
  their exact bytes before scanner adaptation. Equal identities permit reuse;
  missing or changed identities fail closed. The scanner identity is
  `7e493677411a501e6d8592c6b9cc158e21a1bfed44c72ca914e2d81e4e34861d`.
  It includes local port hash
  `588328cd27eea49e88b704b9bd8e46958046564187a5db1a70f6622308a7fff8`.
  A test hashes the marked local source region. Production code does not read
  source files. The exact-byte loader owns grammar identity. The public
  language API cannot relabel a loaded language. Equal raw bytes with a
  changed scanner identity fail closed. Arena identity set and inherit paths
  use exact allocation deltas. Reset clears the identity. Native SQL passed
  the route and real-corpus gates. Generated SQL still has four locked-C
  divergences. A one-shot accounting screen is diagnostic only. It does not
  provide release performance evidence. Keep the SQL identity gate scoped.

- Recorded the C26i, C26j, and C26k Swift issue #576 scanner checkpoint
  blocker. C26i and C26j use evidence base
  `137860ebd80921094e5a8069007d49188dcb5e50`. C26k uses evidence base
  `5d39d9658f5071c5c0f476eaadc6ae067e6c77e1`. This receipt uses publication
  base `7b6f40fe089283674f5d0d19408d2380f77caf68`. Locked C retries its
  external and internal lexer in error mode, then emits `ERROR` at row 156.
  Go emits identifier symbol `160` for `MutableSpan` before recovery. No
  generic per-version scanner checkpoint contract exists. No parser, grammar,
  or test change ships. Keep issue #576 open. See
  `docs/compact-route-real-corpus-matrix.md` for identities, traces, routes,
  and reopening conditions.

- Accepted the C26l default-off external-scanner checkpoint foundation at
  evidence and publication base
  `929609ccde78b0c9f4e57cf2225e0ae1204149cb`. The opt-in API
  requires stable, non-empty scanner and grammar identities. It accepts only
  identities of at most 256 bytes and complete, non-empty serialized state.
  It owns source position, lexer state, token span, identity, and serialized
  bytes. It deep-copies state and fails closed on missing capability,
  incomplete state, inverted spans, or identity mismatch. The API has zero
  production call sites. It does not change parser, grammar,
  Swift, generalized LR (GLR), recovery, merge, or incremental behavior.
  Restore verifies the serialized payload after `Deserialize` and fails closed
  on any length or byte mismatch. Callers must discard a payload after failed
  verification.
  Synthetic lifecycle tests cover capture, fork, merge, recovery transfer,
  failed-scan restore, overlong-record rejection, restore verification
  mismatch, external-scanner use, and scanner-free off mode. The
  focused Docker run used
  one central processing unit (CPU), 4 GiB, `GOMEMLIMIT=3GiB`, `GOFLAGS=-p=1`,
  and `-parallel=1`.
  Metadata does not set `GOMAXPROCS`. The run passed without an out-of-memory
  kill or wall timeout. Its artifact is
  `/tmp/gts-c26l-checkpoint-foundation-rebase/harness_out/docker/20260823T102036Z-c26l-checkpoint-foundation-rebase`.
  Keep issue #576 open until a real scanner and parser lifecycle use this
  capability with locked-C parity.

- Recorded the N31e C and C++ dispatcher blocker at main commit
  `ab2010d74da5330d64dbddb0d9c58969da766d6d`. Keep `dispatch.c_cpp` live.
  The initial dispatcher census (A0) excludes both languages, and the
  declared corpus lock is absent. The C++ clean control matches locked C.
  The recovery witness still differs in the root error shape and field
  metadata. The focused parity test reports nine missing C++ field names.
  The C++ field-preservation repair is language-specific. It does not resolve
  the C recovery or token-source gaps. Ship no parser, registry, or test
  change. See `docs/root-normalization-retirement.md` for identities,
  artifacts, exclusions, and reopening conditions.
- Recorded the N31f Authzed dispatcher blocker at evidence base
  `ab2010d74da5330d64dbddb0d9c58969da766d6d` and publication base
  `5d39d9658f5071c5c0f476eaadc6ae067e6c77e1`. Keep `dispatch.authzed` live.
  A0 lists three Authzed files, three checked files, three run files, and
  18 recorded rewrites. The refreshed route probe reports 17 rewrites and a
  four-node receipt drift. The probe covers raw, production, compact, forest,
  incremental, and locked C routes. It includes clean controls and malformed
  recovery witnesses. The authenticated corpus and its source lock are
  unavailable. The focused guard pins each source and C digest, route mode,
  divergence path and category, rewrite count, and reuse state. No safe shared
  producer invariant was identified. Ship no parser or registry change. See
  `docs/root-normalization-retirement.md` for the route receipt, artifacts,
  and reopening condition.

- Recorded the N31h Dart dispatcher blocker at evidence base
  `7b6f40fe089283674f5d0d19408d2380f77caf68` and publication base
  `09cb5faa41af35a6bc84fefccbab1a17850d38cc`. Keep `dispatch.dart` live.
  The focused probe covers eight clean witnesses and one malformed witness
  on raw, production, compact, forest, incremental, and locked-C routes.
  Four generic-call witnesses differ from locked C on raw routes. The
  single-type and generic-return witnesses still differ on production and
  incremental routes. Forest repairs those witnesses with 10 and 44
  `dispatch.dart` rewrites. The authenticated Dart corpus is unavailable.
  The focused guard pins source and C digests, divergence paths, route mode,
  pass rewrites, and scanner reuse. No safe shared producer invariant was
  identified. Ship no parser, registry, or production change. See
  `docs/root-normalization-retirement.md` for the receipt and reopening
  condition.

- Recorded the AWK dispatcher blocker at main commit
  `5648911ecf509df8ec870a1214917d9e95cf54f1`. Keep `dispatch.awk` live.
  The focused receipt proves clean raw, production, compact, forest, and
  incremental parity. The tracked `T.gawk` fixture uses provenance commit
  `61a7c75e225e3035390be32d635545e40d8c5faf`. The authenticated corpus lock
  uses AWK revision `5739fd79bcfc75ba7526773d0cf634521f8aca3c`.
  The recovery witness still differs from locked C. Ship no parser or
  registry change. See `docs/root-normalization-retirement.md` for the
  route receipt, failed attempts, and reopening condition.

- Rechecked the AWK dispatcher blocker at current main commit
  `83e0cfbc30ad82e2f327d58e35eea9f438a0ffda`. The one-language Docker guard
  passed with one CPU. Clean raw, production, compact, forest, and
  incremental routes still match locked C. The recovery witness still differs
  in shape, and recovery incremental telemetry remains absent. Keep
  `dispatch.awk` live. Ship no parser or registry change. See
  `docs/root-normalization-retirement.md` for the artifact hashes and
  reopening conditions.

- Recorded the SQL dispatcher blocker at base
  `ac90e46ace3c4ac6fb6bbc9f0897e449c949cfad`.
  The focused Docker receipt covers raw, production, compact, forest,
  incremental, and locked-C routes for a trailing select-list recovery.
  Raw Go output differs from locked C at the source-file child count.
  Production, compact, and incremental routes match locked C after
  `dispatch.sql` runs. Compact falls back, forest declines, and incremental
  reuse records zero subtrees and zero bytes. Container inspection records
  `GOMAXPROCS=1`, one CPU, and a 4 GiB memory limit. The receipt does not cover
  the other two SQL helpers. Keep `dispatch.sql` live. No parser or registry
  change ships. See `docs/root-normalization-retirement.md`.

- Recorded the N31i Cooklang dispatcher blocker at evidence base
  `7498a678c52029a82f312e9637ecb66b15defa0b` and publication base
  `675697a1144fad306489c5142aedaae0825545d9`. Keep `dispatch.cooklang` live.
  A0 lists three Cooklang files, three checked files, three run files, and
  1,021 rewrites. The focused probe covers nine witnesses and six routes.
  Seven non-control witnesses differ from locked C on at least one route.
  Four witnesses differ from locked C on the production route.
  Compact fallback and forest decline remain active for those witnesses.
  Incremental parsing reports external-scanner fallback with zero reuse.
  The authenticated corpus and source lock are unavailable. The focused guard
  pins source, grammar, C, route, pass, forest, and counter identities. Ship no
  parser, registry, or production change. See
  `docs/root-normalization-retirement.md` for the receipt and reopening
  conditions.

- Recorded the N31j Go dispatcher blocker at evidence base
  `929609ccde78b0c9f4e57cf2225e0ae1204149cb` and publication base
  `af8a9a5bdb5bd1ac03762bc9a4f1a89f42463682`. Keep `dispatch.go` live.
  The focused receipt covers six standard witnesses and the included-ranges
  route. All standard routes match locked C except raw `new`/`make` input.
  Included ranges still differ in root range and child count. The three Go
  subpasses report exact checked, run, and rewrite counts. The authenticated
  Go corpus lock is unavailable. Ship no parser, registry, or production
  change. See `docs/root-normalization-retirement.md` for the route receipt,
  artifacts, and reopening condition.

- Recorded the N31k Doxygen dispatcher blocker at evidence and publication
  base `a62b9db306bcb983852cbf0043852546e864e856`. Keep `dispatch.doxygen`
  live. The focused probe covers six witnesses across raw, production,
  compact, forest, incremental, and locked-C routes. The A0 fixtures have
  zero rewrites. The compact counters record exact routed and fallback deltas.
  The probe uses a deterministic trailing-space deletion for every incremental
  witness. The historical childless and recovered witnesses require three and
  fourteen dispatcher rewrites, respectively. The childless and smoke C digests are pinned,
  and exact witnesses require Go/C digest equality. The A0 fixtures and the
  recovered historical witness still show locked-C divergence, and the
  authenticated Doxygen corpus is unavailable. Ship no
  parser, registry, or production change. See
  `docs/root-normalization-retirement.md` for identities, artifacts, and
  reopening conditions.

### Performance

- Rejected the P25ar lazy node-equivalence table allocation at base
  `e24ccf5a87bbd7febc21f67f014c2d5301d229d0`. The candidate kept the full
  `16384` entry table and deferred its allocation until the first store.
  Focused cache tests and authenticated recovery parity passed. Swift time
  improved `9.36%` and JavaScript time improved `5.38%`, but both lanes had
  zero bytes-per-operation change. Recovery time regressed `6.10%`, and the
  primary single-byte edit control regressed `2.29%`. Primary full-parse
  bytes per operation rose `0.55%`. The candidate failed the allocation and
  one-percent control gates. Reverted `glr.go` and `glr_test.go`. Retain the
  P25aq and P25ar evidence in `docs/perf-attribution.md`.

- Recorded the P25aq node-equivalence telemetry screen at base
  `e24ccf5a87bbd7febc21f67f014c2d5301d229d0`. Recovery deletion had 3,431
  lookups, 49 hits, and five depth-zero misses. Swift had 20,549 lookups,
  9,072 hits, and 91 depth-zero misses. The standalone P25aq telemetry log
  recorded zero JavaScript lookups and zero hits. A later focused-cache rerun
  recorded six lookups and two hits; this receipt uses that rerun value. The
  receipt does not infer a cause. The primary trio had zero probes. Depth-zero
  probes produced no hits and were not material. No bypass candidate was
  tested. The focused cache tests and authenticated recovery parity passed.
  The full canonical run stopped at the existing Python external-scanner
  fallback case. No code, test, or documentation change shipped. See
  `docs/perf-attribution.md` for artifact hashes.

- Rejected the P25an two-entry node-equivalence cache after collision
  hardening and two-language screening. The deterministic test covered full
  node keys, versions, depth, epoch, eviction, and parse-result stability.
  The Swift lane regressed time by `45.38%`, which exceeded the one-percent
  limit. The JavaScript lane improved time by `8.45%`, but it could not offset
  the Swift regression. Restored the `16384` entry cache. See
  `docs/perf-attribution.md` for artifacts and hashes.

- Recorded the P25ak-P25al node-equivalence cache screen at base
  `5eaa38e536e54530b3d795c6c0a56d927d3d0e0e`. P25ak traced the four fixed
  merge caches. The node cache used 512 KiB and had the lowest hit rate.
  The accepted candidate changed `glrNodeEquivCacheSize` from `16384` to
  `2`. Its recovery `recovery_deletion` drained screen reduced bytes per
  operation by `11.41%`. P25al passed focused Go, JavaScript, Python, and Rust
  Docker checks. The primary trio improved time by `6.64%` to `7.76%`. Only
  full parse reduced bytes per operation, by `7.56%`; other trio allocation
  metrics stayed unchanged. A known JavaScript control failed in both
  baseline controls; three focused JavaScript regressions passed in both
  trees. P25an later rejected the candidate on a Swift time regression. See
  `docs/perf-attribution.md` for artifacts, hashes, and limits.

- Recorded the P25ao adaptive node-equivalence cache screen at base
  `d54147516440a91b8eda6983251c7cd6c4be2707`. The candidate started with two
  entries and doubled after mixed misses and hit streaks. Focused collision
  and epoch tests passed, but Swift time rose `1.72%` and authenticated
  recovery deletion time rose `26.34%`. Recovery bytes rose `0.17%` and
  allocations rose `0.90%`. The candidate was reverted. Keep the fixed
  `16384` entry cache. See `docs/perf-attribution.md` for artifacts and
  hashes.

- Recorded the P25x-P25ab performance blocker at publication base
  `8c80a46e450d906fc9ce1665c189497b02483a3e`. P25x used evidence base
  `3d6cd2628f7a42c348f51dce0a0ed9b92b183c6a`. P25y and P25z used evidence
  base `675697a1144fad306489c5142aedaae0825545d9`. P25aa and P25ab used
  evidence base `2c533f5c19f5f7ab9b586cd8454f0cdc4ece014b`. P25x's
  condense guard passed focused correctness checks but lacked a proven generic
  invariant. P25y regressed the primary trio by geometric means of `+7.01%`,
  `+3.79%`, and `+2.23%`; recovery rose `+2.41%`. P25z found no safe
  reduction shortcut. P25aa's field-scan candidate rose `+1.70%` on the
  six-seed recovery screen. P25ab found material field-path execution, but no
  safe optimization. Node-arena growth used `74.98%` of allocated bytes.
  The retry bypass remains correctness-invalid. No 20-seed run was warranted.
  Earlier P25y figures `+1.40%`, `+2.95%`, `+0.44%`, and `+2.62%` are
  unsupported by this artifact set. Earlier P25aa figures `+1.00%` and
  `+0.86%` are also unsupported. The authenticated raw files supersede both
  sets of figures.
  No code ships. Keep issue #454 open. See `docs/perf-attribution.md` for
  artifacts and the reopening condition.

- Recorded the P25ac-P25ae issue #454 incremental arena retention screen at
  P25ac evidence base `8c80a46e450d906fc9ce1665c189497b02483a3e`, P25ad-P25ae
  evidence base `af8a9a5bdb5bd1ac03762bc9a4f1a89f42463682`, and publication
  base `6a22fcf82c4d84ab613c68084ad356eb52bb4eac`. P25ac used a same-length
  edit that took the token-invariant leaf fast path, so it recorded zero
  incremental arena acquisition and zero new nodes. P25ad traced that path
  and selected the authenticated Go `recovery_deletion` edit. P25ae's 2 MiB
  retention candidate improved the pooled lane, but the drained control
  improved bytes per operation by only 0.61% and increased maximum resident
  set size (RSS) by 7.39%. Keep issue #454 open. The candidate was fully
  reverted. Ship no
  production or test change. Reopen only when the drained lane meets the
  allocation target and the RSS limit. See `docs/perf-attribution.md` for
  artifacts, counters, limits, and the superseding receipt.

- Recorded the P25k-P25w performance blocker at publication base
  `3d6cd2628f7a42c348f51dce0a0ed9b92b183c6a`. P25k ranked the remaining
  source-owned full-parse costs but found no safe duplicate operation.
  P25l through P25n found no safe dispatch, token-source, lexer, or GLR
  ranking reduction. P25o could not use branch counters because Docker
  reported `No permission to enable branches event`. P25p found no new
  target. P25q separated incremental edit and no-edit costs: the edit path
  spent `88.8 ns` in `Tree.Edit`, `420.0 ns` in reuse, and zero time in
  reparse or rebuild; the no-edit path was a same-source identity check.
  All profile containers used one CPU and reported no timeout or out-of-memory
  event. P25r through P25t isolated the Go `recovery_deletion` forward path,
  but found no generic operation to remove. P25u audited that evidence and
  kept the performance lane open. P25v compared the accepted-error retry with
  a diagnostic bypass; all deep Go/C digests stayed equal, but single-run wall
  and RSS differences are diagnostic, not performance evidence. P25v's focused
  parity diagnostic passed, but no candidate passed the proof boundary. No
  production code ships, and issue #454 remains open. P25w superseded the
  P25v hypothesis: the independent exact route exposed an incremental digest
  mismatch when the retry was bypassed. No randomized performance gate ran.
  See
  `docs/perf-attribution.md` for artifacts and the
  reopening condition.

- Recorded the P25h-P25j parser-core dispatch blocker at evidence base
  `41d0b9de133de777aeba9c1dca091903da052a7f`. P25i used evidence base
  `137860ebd80921094e5a8069007d49188dcb5e50`. P25j used evidence base
  `5d39d9658f5071c5c0f476eaadc6ae067e6c77e1`. The publication base is
  `526815a853713ef8af114170f87e94eed6438e85`. A fresh quiet profile placed
  `3.31%` flat CPU time in `dispatchPassActive` and `13.25%` in
  `runtime.duffcopy`. A value projection copied `head`, `s3Region`,
  `shifted`, `accepted`, and `paused`. Its raw diff SHA-256 is
  `1a21f58236d65b6f2c91d74c6a9be322c4d0e095a929452218b60ebfcfeae776`.
  The primary trio geometric mean (geomean) improved `0.79%`, but the
  authenticated generalized LR (GLR) control regressed `11.03%`
  and bytes per operation
  regressed `16.61%`. Warm three-sample maximum resident set size (RSS)
  increased from `600000 KiB` to `610240 KiB`. Structural counters stayed
  equal. Focused tests and Go parity reproduced baseline results. No code
  ships. A P25i scalar follow-up removed the aggregate snapshot. Its six-seed
  diagnostic screen was neutral on authenticated generalized LR (GLR), and
  its warm RSS median increased `0.95%`. No P25i 20-seed publication occurred.
  Keep issue #454 open. See `docs/perf-attribution.md` for limits, artifacts,
  and the field-projection reopening condition.

  P25j screened one distinct 224-byte `linearGroup` copy outside the rejected
  dispatch header snapshots. Field-wise initialization removed that assembly
  copy while preserving the zero-value slot invariant. Its raw diff SHA-256 is
  `a055e19160f401b266c6afb43a4be8e24bd058434c05ba6b3050b3629b27404b`.
  The six-seed alternating screen was neutral. Its geomean changed by
  `-0.07%`, with no systematic
  bytes-per-operation, allocation, or parser-work change. The candidate was
  rejected before a 20-seed campaign or RSS measurement. Ship no code. See
  `docs/perf-attribution.md` for profile identities, artifacts, and reopening
  conditions.

- Recorded the P25g parser-core dispatch blocker at evidence base
  `1c30650814ec6e65cbf31184301bf4776f3e5f41` and publication base
  `54c7f521505e23b7a32c84c2a14d3bd3175c09dd`. A fresh quiet profile measured
  central processing unit (CPU) time. It placed `12.29%` flat time in
  `runtime.duffcopy` and `5.98%` in the generic dispatch pass. The rejected hypothesis replaced one 224-byte header value
  copy with indexed access. Its raw diff SHA-256 is
  `7c9b90937af20c0d5ffb5768e224fe718201a2592f2f4cf1469e50da40fbac65`.
  The candidate regressed the primary trio geomean by `+7.02%`, and the
  authenticated `grammargen_lr` control by `+54.28%`. Full-parse bytes rose
  `+9.49%`. The three-sample maximum resident set size (RSS) median rose
  `+1.92%`. No code ships.
  Keep issue #454 open. See `docs/perf-attribution.md` for the proof limits,
  artifacts, and field-level projection reopening condition.

- Removed duplicate dynamic-precedence dispatch in `gssEntryHash` while
  preserving semantics. The primary trio changed by `+0.16%` for full parsing,
  `+0.21%` for single-byte edits, and `-3.86%` for no-edit parsing. Its
  geometric mean (geomean) changed by `-1.18%`. Bytes per operation and
  allocations per operation stayed unchanged. The real `grammargen_lr` run
  improved by `-3.13%` (`p=0.004`, `n=20`). The three-sample maximum resident
  set size (RSS) medians were `615840 KiB` and `593760 KiB` (`-3.59%`). The
  candidate has two `157 allocs/op`
  outliers; other samples report `156 allocs/op`. Focused Go, locked-C, hash,
  merge, forest, raw-shape, and incremental gates passed. The P25e profile
  supplies attribution. Its SHA-256 is
  `96bead7a8c448a597e4986839b000b602a08bf72b5ffaa5685fa72dcc432715c`.
  Exclude the failed `20260823T061724Z` runtime probe. No accepted run timed
  out or exhausted memory. Keep issue #454 open. See
  `docs/perf-attribution.md` for the evidence base and validation artifacts.

- Recorded the P25e issue #454 fresh-profile investigation collected at
  evidence base `14f6692fac65eab817f65af8cc6072e423ca6563` and published
  from main commit `2b755f744aef8dd253a4415ca4a5816fa85b0dbb`. The workload
  used one Go
  grammar and four authenticated fixtures. It used 20 sequential seeds,
  `GOMAXPROCS=1`, `GOFLAGS=-p=1`, `-count=1`, `-benchtime=750ms`,
  `-benchmem`, and shuffle seeds 1 through 20. The receipt records every
  fixture identity, deep digest, metric mean, and metric range. One resident
  set size (RSS) sample measured `601600 KiB` on four Docker CPUs without
  CPU pinning. The CPU profile SHA-256 is
  `96bead7a8c448a597e4986839b000b602a08bf72b5ffaa5685fa72dcc432715c`.
  The profile includes untimed admission work. The broad frames do not
  identify a safe grammar-agnostic candidate. The evidence has no
  incremental or issue #454 recovery proof. Exclude the no-test
  `050742Z` run and the compile-failed `050817Z` run. Keep issue #454 live.
  Ship no candidate code. See `docs/perf-attribution.md` for the complete
  evidence receipt and reopening condition.

- Recorded the P25d issue #454 C# fresh-parse performance blocker at base
  commit `731f8a9d9440a006b2cc6b56ef5b31c0ff3b5ce7`. The synthetic source
  sweep used requested sizes from 2 KiB through 32 KiB. It has no direct
  locked-C parity proof and does not measure incremental parsing because the
  C# scanner does not support incremental reuse. Keep issue #454 live. Ship
  no candidate code. Three failed construction artifacts are quarantined;
  corrected replacements passed. See `docs/perf-attribution.md` for exact
  source hashes, controls, resource results, artifacts, and reopening
  conditions.

- Recorded the P25c issue #454 Objective-C transient-delete performance
  blocker at source commit `838aba943038248529429a572c4d6d98359bd87e`.
  The eight-size sweep found no discrete threshold between 512 KiB and
  1 MiB. Each delete used five attempt entries and four retry passes.
  Fresh and incremental trees matched by digest. The Objective-C stack-cap
  helper is grammar-specific. No safe grammar-agnostic candidate passed the
  proof boundary. Keep issue #454 and the performance arm live. No production
  or test change survives. See `docs/perf-attribution.md` for the receipt,
  artifacts, and reopening condition.

- Recorded the P25b issue #454 performance blocker at source commit
  `18d63b6f7802b28a0ddb889327fcd4ebebb99426`. The exact 137 KiB base source
  matched the locked C tree. The one-byte edited source did not. The Go fresh
  and incremental edited trees matched each other, but both differed from C
  at the first error node. The incremental run rejected reuse, allocated
  3,165,354 to 3,213,594 transient nodes, and replaced the discarded result
  with one default-budget full parse. The focused Docker correctness and
  locked-C ratchets passed. Keep issue #454 and the performance arm live. No
  production code changed. See `docs/perf-attribution.md` for the receipt,
  artifacts, and reopening condition.

- Recorded the P25a fresh-profile performance blocker at main commit
  `0448715e9a80305556b687b6ecaf041da42e9d9d`. The primary trio produced no
  defensible bounded candidate. The profile used one CPU, `GOMAXPROCS=1`,
  stable single-process settings. The recorded commands ran sequentially.
  Full parsing used 12,453 bytes per operation (B/op) and four allocations
  per operation (allocs/op). The incremental lanes used zero B/op and zero
  allocs/op. Maximum resident set size (RSS) ranged from 49,868 to 50,668
  KiB. No code changed. The receipt includes no Docker correctness run or
  20-seed publication. Keep the performance arm live. See
  `docs/perf-attribution.md` for the proof boundary, artifacts, and reopening
  condition.

- Recorded the P24i final performance blocker at main commit
  `603f64155651888d46937e6b5df461873283b9a1`. After P24a through P24h, no
  safe bounded candidate remained. P24i made no code change and ran no
  20-seed campaign. The focused Docker baseline passed at
  `/tmp/gotreesitter-p24i-investigation/harness_out/docker/20260822T234737Z`.
  Keep the performance arm live until a fresh quiet-host profile identifies a
  bounded candidate. See `docs/perf-attribution.md` for the proof obligations.

- Recorded the P24h transient-parent dispatch predicate rejection. The
  candidate changed only `parser_reduce.go` at base commit
  `30f470f5c2bf18540f7a18b2b22a7e33b88d4e10`. The candidate diff SHA-256 is
  `2a57aef7c0eac33b802439506ac01b92e29d3286ee5a78fc482f26d4b7630c8c`.
  Focused Docker tests and Go and C parity matched the baseline. The accepted
  20-seed, 40-process publication found no significant timing improvement.
  Full-parse bytes increased in the raw means, while allocations stayed
  neutral. The candidate code does not ship. See `docs/perf-attribution.md`
  for the full receipt and reopening condition.

- Recorded the P24g conditional recovery-scratch reset rejection. The
  candidate changed only `parser.go` at base commit
  `30f470f5c2bf18540f7a18b2b22a7e33b88d4e10`. The candidate diff SHA-256 is
  `f1bf93c698c41d06c77a123ee6ab255e7caae61f6406d7171cbeee4355bdaa01`.
  Focused Docker tests passed. Go real-corpus and Go-to-C parity matched the
  baseline known divergences. The 20-seed, 40-process publication found
  significant regressions in full parsing and no-edit incremental parsing.
  The candidate code does not ship. See `docs/perf-attribution.md` for the
  full receipt and reopening condition.

- Recorded the P24f stack-entry state extraction rejection. The candidate
  changed exactly one file, `no_tree_node.go`, from experiment base commit
  `48e844d9a73863cb92367c4db10f02bc5c09375d`. This receipt uses main commit
  `f42b88ac9014537d20d3edd76e2c9caa4330a579`. The candidate diff SHA-256 is
  `3f930b8109c3a2be5b8aabfa4161c71148aa9a15277e2407dab32976b8c8e7d1`.
  Focused Docker tests and Go and C parity passed. The 20-seed publication
  used 40 isolated processes, alternating order, `GOMAXPROCS=1`, `-count=1`,
  750 milliseconds, and `-benchmem` on the primary trio. Benchstat found a
  +0.04% geomean change with no significant lane improvement. The paired
  geomean result was −0.236%, with a 95% interval of [−0.772%, +0.300%].
  Allocations stayed neutral. An auxiliary run measured 591,840 KiB baseline
  resident set size (RSS) and 608,000 KiB candidate RSS. The candidate code
  does not ship. See
  `docs/perf-attribution.md` for the proof boundary and reopening condition.

- Recorded the P24e raw-shape hashing rejection. The candidate changed only
  `raw_shape.go` at base commit
  `098620ad5e39d7b69b258239d1059a1e33bea892`. The candidate diff SHA-256 is
  `e92471c690c7d6fed16d593044a449b0557b8b78380718173731a70a62842996`.
  This receipt is based on main commit
  `56b97e092d0fb034bddb9e65cc617ebb933cc718`.
  Focused Docker tests and Go and C parity passed. The accepted 20-seed,
  40-process publication found a +0.16% geometric mean (geomean) change, with
  no significant lane improvement. The parity receipt records two tree matches
  and three known baseline divergences; see
  `/tmp/gotreesitter-p24e-candidate/harness_out/grammargen_cparity/20260822_143121-p24e-raw-shape-go/container.log`.
  Full parsing used 54.10 KiB and four allocations per operation in both
  variants. The candidate code does not ship. See `docs/perf-attribution.md`
  for the full receipt and reopening condition.

- Recorded the P24d authenticated single-head shift rejection. The candidate
  changed exactly two files in the generic parser-core driver and its tests.
  The combined candidate diff SHA-256 is
  `5181912ec91f0bc1ecd504d06db488d3dd9145fdc5d3d5486300756661a4fc59`.
  The accepted 20-seed publication found a +0.72% geomean regression, a
  significant no-edit regression, and a significant full-parse byte increase.
  The candidate code does not ship. See docs/perf-attribution.md for the
  correctness, parity, and performance receipt.

- Recorded the P24c exact-single-pop direct-append rejection. The direct-append
  path itself already shipped in ancestor `b9106e78635244b59dc9c9b75aa4863b89c99630`.
  P24c tested only helper inlining inside `condenseWithOutcomeAtomic`. The
  candidate changed exactly these two files:
  `internal/parsercorephase0/core.go` and
  `internal/parsercorephase0/condense_direct_append_test.go`.
  The audit patch hashes are `2dbcf54b185214e609d0cade7bf53ea6408150e6b0857ac104f24c3fee759438`
  for the core patch, `418f60f9d20948d161adfe510fc068cc622ca4660f95aeef7476cde6ff05b0af`
  for the focused test patch, and
  `48a5ab605ff49373017aa5a5a31b423692619a3cecf71d9be9fd9f46a37190c3` for
  the combined two-file patch. Focused Docker correctness passed after the
  final fix. The stale pre-final package attempt is excluded. Generic driver
  failures reproduced on baseline. JSON and CSS parity each passed 5/5.
  The accepted 20-seed alternating publication found a +1.72% geomean
  regression. The candidate code does not ship. See docs/perf-attribution.md
  for the full receipt.

- Recorded the P24b owner-plus-lineage scheduler transaction as a performance
  no-go. The candidate changed three files and had diff SHA-256
  c064480db960885558cce4eff8b22679a3dffed996d908f938fa5e480f92dee.
  Focused correctness gates passed. The publication used 20 seeds, 40
  isolated processes, and 120 rows. Benchstat found no significant target
  improvement. Paired means rose in all three lanes. The candidate code does
  not ship. The corrected unit receipt is
  `/tmp/gts-p24b-owner-lineage-20260822/harness_out/docker/20260822T184141Z-p24b-owner-lineage-unit-corrected-20260822`.
  It runs all seven combined owner-lineage tests and the existing owner-only
  rollback control. The earlier `20260822T180959Z-p24b-owner-lineage-unit-final2`
  receipt is superseded and excluded: its anchored expression matched no
  combined test. See docs/perf-attribution.md for the full receipt.

### Correctness

- Record the C26h Swift issue #576 producer-boundary blocker at main commit
  `41d0b9de133de777aeba9c1dca091903da052a7f`. The six prefixes from
  `FloatingPointToString.swift` retain different Go and locked-C digests.
  Go emits deterministic finite automaton (DFA) identifier token 160 for
  `MutableSpan` at bytes `6828..6839`. Locked C emits `ERROR`, detects the
  error, resumes, and skips it. The divergence occurs before recovery
  election and tree materialization. Compact and forest routes decline.
  Incremental reuse reports `external_scanner_unsupported`. The explicit
  controls were one CPU, 4 GiB, `-parallel=1`, `GOFLAGS=-p=1`, and
  `GOMEMLIMIT=3GiB`. The wrapper does not record `GOMAXPROCS=1`. A timed
  run observed 232,640 KiB maximum resident set size (RSS). No safe
  grammar-agnostic correction is proven. Ship no parser, grammar, or test
  change. Keep issue #576 open. See
  `docs/compact-route-real-corpus-matrix.md` for digests, traces, artifacts,
  and the scanner-aware reopening contract.

- Record the C26f Swift issue #576 `FloatingPointToString.swift` recovery
  blocker at evidence base `5648911ecf509df8ec870a1214917d9e95cf54f1`.
  The 104,681-byte source and its complete 7,316-byte prefix have raw,
  production, and incremental Go trees that differ from locked C. Compact
  and forest routes decline before a certified comparison. The Go Swift blob is
  `be4575bc0acc3c60324aab635d067f940ac5f0557b80a8e3565d1e7d02d53582`.
  The Swift grammar commit is
  `41d6e5fe811ec94229ee71771174a8cce558dfee`.
  C26f artifacts contain incorrect tree-sitter-Go identity fields.
  Treat those fields as artifact defects, not evidence. No production or
  test change survives. Keep issue #576 open. See
  `docs/compact-route-real-corpus-matrix.md` for digests, route telemetry,
  artifact exclusions, Canopy reachability, and reopening conditions.

- Record the C26e Swift issue #576 CollectionAlgorithms recovery blocker at
  current main commit `f0904533b6398775d5df5e01bc34d32feee34900`. Evidence
  comes from `14f6692fac65eab817f65af8cc6072e423ca6563`. The exact 16,871-byte
  prefix and the 24,056-byte tracked source differ from locked C on the raw,
  production, compact, and incremental routes. Compact fallback declines at
  recovery. Forest declines at `dead_end`. Incremental reuse is unsupported
  because of `external_scanner_unsupported`. The authenticated corpus and
  source lock are unavailable. No production or test change survives. Keep
  issue #576 open. See `docs/compact-route-real-corpus-matrix.md` for the
  20-byte unsafe control, evidence, artifacts, and reopening conditions.

- Record the C26d Swift optional-binding recovery blocker at main commit
  `11d9aec70eaef0c0d65c3cd14b8f594d64869c7b`. The `issue-590` artifacts show
  a 118-byte witness and a 27,814-byte corpus witness that differ from locked
  C. Forest reports a `user_type` versus `simple_identifier` mismatch for the
  minimal witness, and the corpus forest route declines. Incremental routes
  report `external_scanner_unsupported` with zero reuse. No production or test
  change survives. Keep issue #576 open. See
  `docs/compact-route-real-corpus-matrix.md` for digests and controls.

- Record the C26c Swift masking-shift blocker under open issue #576.
  The main commit is `77ec4288a115e4ddb1969d2945e8507dad92f1af`.
  This item originated as closed issue #574. The canonical 14-byte
  newline witness has Go and locked-C tree digests
  `0a080f094102d27305084a234d22396a1c4b64cad5be9ab55a9969249f2a67aa` and
  `14b99aace77ea88a972e0d1bbefcdef9f226bb764aeba12bb41c2cf1509610e9`.
  Go absorbs `7` into the infix expression. Locked C emits a sibling error.
  The known-failing `stdlib_ASCII.swift` ratchet passes; it does not establish
  Go/C parity. The audit found no safe grammar-agnostic fix.
  No production or test change survives. Keep issue #576 open. See
  `docs/compact-route-real-corpus-matrix.md` for the control, traces, and
  reopening condition.

- Record the N31b TypeScript dispatcher blocker at main commit
  `731f8a9d9440a006b2cc6b56ef5b31c0ff3b5ce7`. The tracked fixture has source
  SHA-256 `40b4a7a06fde353d8c2b726acb16f59aab44d49d1b6257c37345c2a1f56b9fb7`.
  It records 1,462 visited nodes and 15 rewritten nodes. The available
  positive, typed-arrow, generic-call, and issue-544 controls match locked C
  on the checked routes. Compact fallback remains fail closed. Incremental
  scanner reuse remains enabled. The cap-one diagnostic exposes a typed-arrow
  root-error divergence. The A0 manifest and authenticated corpus do not
  include TypeScript. The generic-call selection test remains skipped. The
  held-out webworker receipt was not rerun because its source is unavailable.
  Keep `dispatch.typescript` live. No production code changed. See
  `docs/root-normalization-retirement.md` for the receipt, artifacts, and
  reopening condition.

- Record the N31c Python dispatcher blocker at main commit
  `14f6692fac65eab817f65af8cc6072e423ca6563`. The A0 manifest excludes
  Python, and the authenticated Python corpus and lock are unavailable. The
  tracked Python fixture records zero rewrites. The positive witness differs
  from locked C only before normalization; production, compact, forest, and
  incremental routes match. Two f-string witnesses differ from locked C on
  every route. Incremental parsing reports
  `external_scanner_unsupported` with zero reuse. Keep `dispatch.python` live.
  No production or registry change survives. See
  `docs/root-normalization-retirement.md` for the receipt, artifacts, and
  reopening condition.

- Record the C26b Swift issue #576 conformance-list recovery blocker at main
  commit `838aba943038248529429a572c4d6d98359bd87e`. The 66-byte
  `associatedtype` witness still differs from locked C. Go emits
  `ERROR[51:64]` with the comma only. Locked C emits `ERROR[51:63]` with a
  nested `Comparable` error. The Go trace first pauses on the comma in parser
  state `2811`. A zero-width `_implicit_semi` then extends the Go span. The
  `stdlib_Stride.swift` corpus ratchet passes, but the audit found no
  grammar-agnostic fix. No production or test change survives. Keep issue #576
  open. See
  `docs/compact-route-real-corpus-matrix.md` for the receipt.

- Record the C26a Swift issue #576 token-production blocker at main commit
  `18d63b6f7802b28a0ddb889327fcd4ebebb99426`. The 20-byte witness still
  differs from the locked C reference parser at the first error node. The
  isolated grammar-agnostic predicate did not change the output. No
  production or test change survives. Keep issue #576 open. See
  `docs/compact-route-real-corpus-matrix.md` for the digests, trace artifact,
  and reopening condition.

### Added

- Add the default-off authenticated D6a drop-cohort frontier producer. It
  binds the scheduler owner and epoch, the election token and scanner
  checkpoints, the ordered participants, the action identity, the derivation
  bytes, digest, and checkpoint, and a seal. D6a does not enable admission,
  history, or verification.

- Add default-off authenticated D6b consumption. Prove common survivor action
  and full derivation. Roll back consumed-state journals. Leave driver and
  admission paths inactive.

- Add default-off D6b driver verification before no-action drops. Keep route
  admission and production drops disabled.

- Keep D6b fail-closed for malformed frontier records. Require every dropped
  participant to share one cohort with the survivor candidate before returning
  a typed decline for a missing common action or exact derivation. Treat a
  mixed-cohort frontier as a fatal error. Continue with the existing
  alternative-set proof. The `grammargen_lr` witness uses production fallback
  because that proof also declines. Keep direct D6b admission ungraduated.

- Record the D6c `grammargen_lr` blocker on receipt branch base `ed0568d9`.
  The original probe ran at `0c34a681`. The survivor derivation digest is
  `9b1c3a249bec15d4b74a7462f701c491e022be80f7a51a5590f1520a76fd2c06`
  with continuation state `1141`. The dropped derivation digest is
  `d72a6fe90ca3aec9883bd00494eb8ca7110ede90d5f09fb5000fdc6441a79e8f` with
  continuation state `680`. At path `[0,4]`, span `1030..1037` (`prodIdx`),
  the survivor is `identifier` symbol `86`, production `0`; the dropped node
  is `parameter_declaration` symbol `113`, production `36`, with one `type`
  child. The D6c admission is a NO-GO. Any continuation-state or public-shape
  mismatch must decline and preserve the existing locked-C production fallback.
  The focused Docker receipt passes the typed D6b decline and the existing
  fallback gate; both trees retain locked-C digest
  `1472cfd9a014d4034dbc1456afd12c282630ef787c3543cf0cecb73619883ad2`.
  The labeled wrapper artifacts are
  `/tmp/gts-d6c-nogo-receipt-20260822/harness_out/docker/20260822T202359Z-d6c-frontier-state-public-shape`
  and
  `/tmp/gts-d6c-nogo-receipt-20260822/harness_out/docker/20260822T202427Z-d6c-grammargen-lr-fallback`.

- Record the Swift #576 unsafe minimal and corpus divergences as open. Keep
  the focused locked-C receipt without changing parser behavior. The locked
  grammar uses commit `41d6e5fe811ec94229ee71771174a8cce558dfee`, version
  `0.7.2`. Current upstream main uses commit
  `172ada1cc4117d0260d9340680b4134adba2bc2c`, version `0.7.3`. Regeneration
  changes the blob size and hash, but it keeps the minimal Go digest and the
  first `bar` mismatch. The corpus target region remains an `ERROR` span from
  `6828` to `6984`, with `MutableSpan` at `6828..6839`. No parser or grammar
  code ships. Keep issue #576 open. See the
  [Swift compact-parser blocker receipt](https://github.com/odvcencio/gotreesitter/blob/6b6d49341699df9314b77d52ea92dc950e7364e4/docs/swift-576-compact-correctness-blocker.md)
  for the full provenance probe and reopening condition.

### Removed

- Swift ternary expressions now come directly from the regenerated grammar
  blob. Parsing no longer runs the source-reparse compatibility pass.

- JavaScript dynamic imports now retain their keyword child during parsing.
  Parsing no longer runs the leaf-repair compatibility path.

- Kotlin interpolated calls now retain their call-expression wrapper during
  reduction. Parsing no longer runs the wrapper-repair compatibility path.

- Bash generated-command assignments now come directly from native scheduling.
  The `zipname=npm-$(node ../cli.js -v).zip` witness matches the raw,
  production, compact, forest, incremental, and locked C trees. The route
  receipt also records direct compact admission or a bounded fallback, plus
  incremental fresh or reuse behavior. Parsing no longer runs the
  generated-command assignment compatibility path.

- Ninja recovery trees now come directly from native reduction. The two A0
  (initial dispatcher census) witnesses
  match raw, production, compact, forest, incremental, and locked C
  receipts. Parsing no longer runs the Ninja compatibility path.

- Ledger recovery trees now come directly from native reduction. The two
  parser-trigger witnesses and the A0 witness match raw, production, compact,
  forest, incremental, and locked C receipts. Parsing no longer runs the
  Ledger compatibility path.

- JSDoc recovery trees now come directly from native lexer and reduction
  behavior. Both producer witnesses match raw, production, compact, forest,
  incremental, and locked C receipts with zero rewrites. Parsing no longer
  runs the JSDoc compatibility path.

### Fixed

- Record the `dispatch.c_sharp` blocker at main commit
  `ef57c9d1b73bac046ef40f2a111bb76db643ebfd`. The A0 manifest excludes C#;
  the tracked census records one C# fixture with 2,085 rewrites. The focused
  probe keeps one exact positive control, but the A0 and recovery witnesses
  still differ from locked C. Forest declines the recovery witnesses, and
  incremental reuse remains unsupported because the grammar has an external
  scanner. Keep the arm live. No parser or registry code changes ship. See
  `docs/root-normalization-retirement.md` for the digests, artifacts, and
  reopening condition.

- Rechecked the `dispatch.c_sharp` blocker at current main commit
  `83e0cfbc30ad82e2f327d58e35eea9f438a0ffda`. The one-language Docker guard
  passed with one CPU. The positive control matches locked C on every route.
  The A0, issue #454, and malformed witnesses still differ from locked C.
  Forest declines those witnesses, and incremental reuse remains unsupported.
  Keep the arm live. Ship no parser or registry code changes. See
  `docs/root-normalization-retirement.md` for current artifact hashes and
  reopening conditions.

- Corrected the PHP issue #454 recovery-leaf flags at base commit
  `55681868d3a23971d042f9f79083fd6d39c7e33b`. A recovery region now needs a
  source-bearing parsed prefix. Each cleared leaf also needs positive internal
  deterministic finite automaton (DFA) provenance.
  The prefix proof rejects pending, missing, error, dirty, and invalid payloads.
  It also rejects payloads that end after the current token starts.
  Recovery rejects end-of-input, zero-width, generated, external, missing,
  no-lookahead, and error-mode tokens. The raw, production, compact fallback,
  and locked-C `gts-deep-tree-v1` digests now equal
  `1516308c38163089778464ad171875308c559af11af7c8c03ee17ae4eacd23c6`.
  Compact still records `routed=0` and `fallback=1` on recovery. This change
  does not graduate PHP compact admission. The incremental memory budget,
  resident set size, and remaining issue #454 performance work stay open. See
  `docs/issue-454-compact-correctness-blocker.md`.

- Reject the issue #454 generic recovery candidate from PR #793. The candidate
  code and test hash is
  `71fdb2ab00f8f31e74b7e165f381c0856bd3720abdeb4d1556454d0cc75c50fa`.
  The rejected parser diff hash is
  `60a2252eca3f65cf427d709c96729eb41572f2388b1fad64c946d390c3a94db1`.
  Continuous integration (CI) run `32609724840` exposed deterministic Cobol,
  WGSL, and Cooklang changes. The field splice passes focused tests and
  preserves the audited Cobol, WGSL, and Cooklang baselines. No
  grammar-agnostic predicate supports the leaf change. Reject the C-name guard.
  No parser code ships. Keep issue #454 open. See
  [the issue #454 rejection receipt](docs/issue-454-compact-correctness-blocker.md).

- Record the issue #454 compact-parser correctness blocker. A 1 KiB fresh Go
  tree differs from locked C at
  `/translation_unit/function_definition[0]/compound_statement[2]/ERROR[2]/number_literal[0]`.
  Issue #454 reports C latency with correctness marked OK. This repository uses
  an internal deterministic C witness. Go marks the `number_literal` as an
  error, and C does not. The original source has 140,288 bytes. The edited
  source has 140,287 bytes. The same difference persists at 4, 16, 64, and
  137 KiB. Incremental Go equals fresh Go at 137 KiB, and the memory-budget fallback reports
  `incremental_parse_memory_budget_full_retry`. The fresh-parse mismatch makes
  retry selection unable to fix C parity. Status: NO-GO. Keep issue #454 open.
  See [the issue #454 compact-parser correctness blocker receipt](docs/issue-454-compact-correctness-blocker.md).

- Record the `dispatch.rust` blocker receipt at base
  `97a7bde26bac9b1a110bbf9216cc681ca59cc5aa`. Keep the arm live. The receipt
  covers 23 zero-rewrite Rust witnesses and exact locked-C production, compact,
  forest, and incremental output for the registered smoke and tracked
  witnesses. The full authenticated Rust census is unavailable. The Rust
  real-corpus run still reports two structural mismatches in `weird-exprs.rs`.
  Keep the registry unchanged. See `docs/root-normalization-retirement.md`.
- Record the `dispatch.apex` blocker receipt at base
  `f42b88ac9014537d20d3edd76e2c9caa4330a579`. Keep the arm live. The receipt
  covers four registered witness families, five clean route witnesses, and
  three malformed recovery witnesses. The forest route still diverges from
  locked C. See `docs/root-normalization-retirement.md`.

- Record the `dispatch.hlsl` blocker receipt at base
  `0b4470b0156afb1e3492f7f5f6a618c9e50f7c33`. Keep the arm live. The A0
  manifest records three HLSL files, three checks, three runs, and zero
  rewrites. The clean negative-number cast rewrites on production, compact,
  forest, and incremental routes, but normalized output misses the C `left`
  field. The malformed cast and unorm witnesses still diverge from locked C.
  The authenticated HLSL corpus is unavailable. No registry or production
  code changes are included. See `docs/root-normalization-retirement.md`.

- Reconfirm the DTD locked-C blocker at base
  `30f470f5c2bf18540f7a18b2b22a7e33b88d4e10`. Keep `dispatch.dtd` live. The
  raw route matches the production route by deep digest for all four
  witnesses. Production, compact, forest, and incremental routes record zero
  dispatcher rewrites. Two recovery witnesses still differ from locked C at
  exact error flags. The full authenticated corpus remains unavailable. See
  `docs/root-normalization-retirement.md`.

- Record the `dispatch.corn` blocker receipt at base
  `613a0775a329998a0af0c6f24251c8008c6783aa`. Keep the arm live. The A0
  manifest records three Corn files, three checks, three runs, and zero
  rewrites. The quoted-path trigger rewrites four nodes and still differs from
  locked C. The malformed quoted-path witness differs in one error flag. The
  full Corn corpus is unavailable. No registry or production code changes are
  included. See `docs/root-normalization-retirement.md`.

- Record the `dispatch.ada` blocker receipt at base
  `30f470f5c2bf18540f7a18b2b22a7e33b88d4e10`. Keep the arm live. The receipt
  covers nine clean and two malformed witnesses across raw, production,
  compact, forest, and incremental routes. The raw route elects the wrong
  derivation for seven clean witnesses. The production pass rewrites seven
  witnesses and still differs on two. Both malformed witnesses differ from
  locked C. The full real-corpus census is unavailable. See
  `docs/root-normalization-retirement.md`.

- Record the `dispatch.bitbake` blocker receipt at base
  `9f6380a8a0795b631a0b5e3a573253977f673bf4`. Keep the arm live. A0 covers
  three files, 40358 visited nodes, two error roots, and zero rewrites. The
  focused receipt covers eight witnesses across raw, production, compact,
  forest, incremental, and locked-C routes. The medium and large A0 witnesses
  differ at `/recipe`. Three constructed producer witnesses also differ. The
  full authenticated corpus is unavailable. Do not change registry or
  production state. See `docs/root-normalization-retirement.md`.

- Record a durable Doxygen locked-C blocker receipt. Keep `dispatch.doxygen`
  live. The three A0 witnesses report zero raw and production rewrites. The
  historical childless and recovered routes report 3 and 14 named rewrites
  across production, compact, forest, and incremental routes. The real corpus
  has no Doxygen directory, so its census remains uncovered. The current
  denominator is 31 dispatcher arms, 33 dispatcher languages, 32 live entries,
  and 56 retired entries. The focused artifacts are
  `harness_out/docker/20260822T202538Z-doxygen-blocker-routes-final` and
  `harness_out/docker/20260822T202308Z-doxygen-blocker-locked-c`, with the
  registry and census gates at
  `harness_out/docker/20260822T202324Z-doxygen-blocker-registry-a0` and
  `harness_out/docker/20260822T202335Z-doxygen-blocker-census`. Locked C
  differs at `/document` for CMakeLists and example, and at `/ERROR` with
  `children=0` versus `children=279` for metrics. Status: `NO-GO`;
  `KEEP LIVE`.

- Make the explicit forest route fail closed on an `ERROR` root. Reuse the
  existing forest decline decision. Keep Ledger recovery triggers on the
  production fallback until their forest trees match the locked C tree.

- Preserve lexer skip-transition provenance for JSDoc padding. The producer
  checkpoint records zero rewrites and equal raw and production digests on
  every covered route. Preserve the root span for an authenticated leading DFA
  skip. Keep unproven non-trivia gaps fail-closed.

- Align the Bash skipped-escape and Erlang leading-skip witnesses with locked
  C. Keep Bash `e \\ cho hi` clean. Keep Erlang `\x010` and `\x100` clean with
  root span `1..2`.

- Correct the Markdown scanner documentation for [issue #454](https://github.com/odvcencio/gotreesitter/issues/454).
  Mark Markdown as `certified reuse`. Changed-token and shape-changing edits
  can still use the production full-parse fallback when reuse gates reject the
  edit. Keep Markdown Inline as `fallback (uncertified)`.

- End recovery convergence after a clean condense. Pending forks no longer
  keep clean C initializer suffixes on the recovery merge path.

### Performance

- Record the P24a memo-probe benchmark as a NO-GO. The single-byte edit lane is
  0.93% slower with p<0.001. Do not ship the two-file candidate. See
  `docs/perf-attribution.md` for the receipt.

## Older releases

Entries before v0.52.0 moved to the changelog archive to keep this file
focused on current releases:

- [v0.44.1 – v0.51.0](docs/changelog/archive-1.md)
- [v0.24.1 – v0.44.0](docs/changelog/archive-2.md)
- [v0.1.0 – v0.24.0](docs/changelog/archive-3.md)

[Unreleased]: https://github.com/odvcencio/gotreesitter/compare/v0.55.1...HEAD
[0.55.1]: https://github.com/odvcencio/gotreesitter/compare/v0.55.0...v0.55.1
[0.54.0]: https://github.com/odvcencio/gotreesitter/compare/v0.53.0...v0.54.0

[0.55.0]: https://github.com/odvcencio/gotreesitter/compare/v0.54.0...v0.55.0
