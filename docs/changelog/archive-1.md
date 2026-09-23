# Changelog archive: v0.44.1 – v0.51.0

Older entries moved out of the root [CHANGELOG.md](../../CHANGELOG.md) to
keep it focused on current releases. See [archive-2.md](archive-2.md) and
[archive-3.md](archive-3.md) for earlier history.

## [0.51.0] - 2026-08-16

### Added

- Add `grammars.ParseFilePooledStrict`. It rejects a partial tree and returns
  the parser stop error.

- Add `Tagger.TagStrict`. It returns tags only after a complete parse and
  preserves parser errors.

### Changed

- Bound pending fork-stack retention. Keep a bounded reserve during parsing,
  and drop oversized storage at parse and parser-pool boundaries.

- Separate C-runtime recovery walk, convergence, and fallback state. Reset
  these states across parse, retry, snippet, pool, and iteration boundaries.

- Provision dense reach marks when generalized LR (GLR) slabs grow. Owned nodes
  no longer use the pointer-map fallback.

### Fixed

- Clear stale C-runtime recovery state after a clean condense. A clean suffix
  no longer inherits an active recovery-cost gate.

### Tests

- Align the Application Binary Interface version 14 (ABI-14) lexer-mode probe
  with generated end-of-file behavior. The probe now checks the emitted layout
  without injecting an end-of-file state.

### Continuous integration (CI)

- The project merged [pull request (PR) #732](https://github.com/odvcencio/gotreesitter/pull/732).
  Its CI-only change separates query-fleet smoke tests from regular shards and
  is not part of this release.

### Open work

The following items remain open:

- [Issue #454](https://github.com/odvcencio/gotreesitter/issues/454) tracks
  downstream field reports.

- [Issue #576](https://github.com/odvcencio/gotreesitter/issues/576) tracks
  Swift recovery divergence.

- [Issue #586](https://github.com/odvcencio/gotreesitter/issues/586) tracks
  shared GLR error-cost bounds.

- The [Swift #586 compact-parser receipt](https://github.com/odvcencio/gotreesitter/blob/6b6d49341699df9314b77d52ea92dc950e7364e4/docs/swift-586-compact-correctness-blocker.md)
  records a NO-GO recovery-cost and token-producer blocker. Keep the issue open.

- [Issue #728](https://github.com/odvcencio/gotreesitter/issues/728) tracks
  external-scanner incremental reuse.

## [0.50.1] - 2026-08-14

### Fixed

- Restore single-language `grammar_subset` builds. Shared lexer helpers and
  Python-derived scanner state now compile with each grammar that uses them.
  Derivative-only builds do not register Python scanner metadata.

- Add a blocking subset build sweep. Continuous integration now builds all
  206 registered grammars with their individual `grammar_subset` tags.

## [0.50.0] - 2026-08-14

### Added

- `TestOutlineOracleDifferential` (`cgo_harness/outline_differential_test.go`)
  runs each language's resolved tags query through both the pure-Go query
  engine and the official C tree-sitter runtime, then diffs the two capture
  streams. It hard-asserts capture parity for the core nine outline
  languages (`go`, `python`, `javascript`, `typescript`, `tsx`, `rust`,
  `java`, `c`, `cpp`) and logs a census for every other language with a
  resolvable tags query.

- `TestOutlineCoverageWitnesses` (`grammars/outline_coverage_witness_test.go`)
  pins 30 languages against the real `Outliner` pipeline. Each case names an
  exact symbol its resolved tags query must produce, so a pattern that
  compiles but never fires now fails the test instead of hiding behind a
  non-empty query string.

- File-outline tags-query coverage rose from a 30-language floor to 84 of
  206 registered languages: 83 with a real-corpus fixture and 1 with a
  smoke-sample fixture (`grammars/testdata/outline_census/baseline.json`).
  `TestInferredTagsQueryCoverage` (`grammars/registry_test.go`) now enforces
  84 as the floor. See `docs/outline.md` for the full coverage tiers and the
  file outline API.

- `OutlineSymbol.Owner` now resolves on every `OutlineTree` call. A rule
  attached through `WithOutlineOwnerRules` matches a symbol by `NodeType`,
  reads its `OwnerField`, and descends through the rule's `Unwrap` node
  types until it reaches exactly one `NameTypes` terminal. Any other
  outcome — an absent field, or a walk that reaches zero or more than one
  terminal — leaves `Owner` empty and counts one
  `OutlineReport.OwnerRuleMisses`; a `NodeType` no attached rule names
  touches neither field.

- `grammars.OutlineOwnerRules(entry)` gates the shipped owner-rule table by
  symbol and field presence in each language's own compiled grammar, the
  same way `ResolveTagsQuery`'s inference table is gated, and composes
  directly with `gotreesitter.WithOutlineOwnerRules`. The shipped Go rule
  resolves all four receiver shapes — value, pointer, generic value, and
  generic pointer — to the receiver's base type name. See `docs/outline.md`
  for the full resolution contract and a worked example.
### Removed

- Native root-extra folding and reduction now own Elixir comment placement
  and map entry grouping. Parsing drops the hidden
  `_newline_before_comment` scanner token without a post-parse pass.
  Reduction groups map keyword pairs and wraps update and arrow entries in
  the map grammar's `binary_operator` node. This removes the Elixir
  result-compatibility dispatcher arm.
### Removed

- Native scheduling and reduction now own the Enforce `const int` formal
  parameter shape. Parsing classifies `const` as a
  `formal_parameter_modifier` and `int` as `type_int` directly. It keeps
  the parameter's own name and default value instead of losing them to a
  misread type/name pair. This removes the Enforce result-compatibility
  dispatcher arm.

### Fixed

- TypeScript and TSX no longer split a signed right-shift operator into two
  generic closers. `splitCompactCloseAngleToken` narrows a `>>` token to a
  single `>` so nested closers such as `Array<Array<string>>` parse. Java
  gated that split behind an "unclosed `<` precedes this run" check, because
  `>>` is also a shift operator there; TypeScript and TSX carried no such
  gate. Any `>>` whose next byte was one of `( ) [ ] { } , . ; : ?` was torn
  apart, so `x = a >> (b)` failed to parse while `x = a >> b` succeeded. The
  angle-depth gate now covers TypeScript and TSX, and runs only when a `>>`
  symbol is actually active in the state: with no shift alternative to
  protect, narrowing to `>` remains the only way to make progress. Found
  while migrating the GoSX browser runtime to TypeScript, on
  `indices[i] = (src[i >> 3] >> (i & 7)) & 1;`.
  `on`, `off`, `yes`, `no`) at lex time. The grammar's word token pattern can
  absorb leading whitespace before a keyword, and the keyword re-lex
  previously required its match to start at byte zero, so it missed that
  case and left the value as a generic string token. DFA keyword promotion
  now skips the leading run first, the same way tree-sitter's own generated
  keyword lexer does. This retires the Hyprlang dispatcher arm and fixes a
  related bug: a trailing space after the keyword no longer produces an
  incorrect boolean node.

- Seven of the file outline's core nine languages carried tags-query rows
  that reference a child node type the grammar never produces at that
  position: `javascript`, `typescript`, `tsx`, `java`, `c`, `cpp`, and
  `rust`. The C query compiler rejects such a row as an "Impossible
  pattern" and drops the entire multi-pattern query with it. The pure-Go
  query engine has no matching check, so it compiled the same row and
  silently matched nothing. The dead rows are now removed or corrected.
  `TestOutlineOracleDifferential`'s core-nine tier reports capture-stream
  parity for all nine languages, and the fix changed no Go outline output —
  confirmed by unchanged golden fixtures.

### Removed

- **The Bash assignment-wrapper and if-condition-field repairs.** Native
  reduction already builds the C-shaped `variable_assignments` wrapper for
  two or more consecutive assignments. It already sets the `condition`
  field on an if-statement's condition tokens too. The Bash compatibility
  pass no longer splices that wrapper's children into the enclosing node or
  rebuilds the field afterward.
  tree-sitter-bash's own corpus (`test/corpus/literals.txt`) pins the same
  wrapper for two top-level assignments.
  Production, compact, forest, and incremental routes match the raw parse
  exactly, and the isolated C-oracle comparison matches for every case.
  One unrelated Bash subpass remains live.
- The FIDL result-compatibility dispatcher arm is retired. Native recovery
  already builds the C-equivalent error shape for a versioned-layout-modifier
  declaration whose modifier keyword carries a stray `(name=value)` argument
  list. Production, compact, forest, and incremental routes produce the same
  tree, and an isolated C-oracle parity check confirms the shape.
- The HLSL subscript-assignment declarator member of the result-compatibility
  dispatcher arm is retired. `structured_binding_declarator` carries a
  negative dynamic precedence in the grammar. Native parsing already elects
  the C-equivalent subscript-assignment expression for `Name[index] = value;`
  without a post-parse pass. Production, compact, forest, and incremental
  routes stay exact. The negative-number cast and unorm-buffer members of the
  HLSL arm remain live.

## [0.49.0] - 2026-08-11

### Removed

- One member of the Go result-compatibility arm is retired: the member
  that widened the root span across a trailing end-of-file newline.
  `extendNodeToTrailingWhitespace` runs unconditionally after the
  compatibility pass and accepts a superset of the byte set the Go member
  tested, so the Go member could never reach a span the later pass did not
  already reach.
  A census recorded 35,244 gate entries and zero rewrites.
  The four pinned canonical Go deep-tree digests are byte-identical, and
  the exhaustive C-oracle fresh and incremental parity sweep stays green.

### Added

- `FactProgram` compiles selected definition, call, heritage, and import work
  into a dense 16-bit instruction stream. One program can process compatible
  trees with one traversal while the individual extraction APIs remain stable.
  The 20-seed combined benchmark reduced Go tree inspection time by 71.19%
  for definitions and calls. The all-facts path reduced time by 84.14%, bytes
  by 1.88%, and allocations by 49.41%. Parse-plus-extraction stayed unchanged.

- Opt-in Lean 4 support now provides a grammar blob, scanner, highlights,
  outline tags, and focused corpus tests in `grammars/lean`. The default
  206-language registry remains unchanged.

- The V10 fleet harness now runs bounded Google Cloud spot workers across all
  registered languages. It enforces wall, memory, disk, and automatic deletion
  limits. Accepted epoch `20260808T202958Z-v10-full-5003ffba` completed all
  1,435 measurements for 206 languages.

- `scripts/run_randomized_benchmarks.sh` now runs one process for each shuffle
  seed and randomizes benchmark order. The standard comparison uses 20 seeds,
  `GOMAXPROCS=1`, a 750 millisecond benchmark time, and memory reporting.

- Merge-event census instrumentation now records merge decisions and refusal
  gates on the production and C-oracle paths. Its first constructed receipt
  reports 15 Go merges against 191 C merges across 104 sources. It records no
  source where Go merges more often than C.

- A regression guard covers issue #660.
  It checks anonymous comma nodes in Python imports and subscripts on both
  parser routes.

- The included-ranges route now has committed test coverage.
  `Parser.SetIncludedRanges` had no test for any language, and injection
  uses that call for every injected child.
  A Markdown document with two or more Go fences reaches it in production.
  Four new gates cover the route: a root-symbol gate, a positive control
  that proves the Go arm still rewrites the tree there, a C-oracle table
  that pins the measured root of both parsers across four range
  geometries, and a deletion guard.
  The route is not at parity with C, and the new tests do not claim it is.
  The root span matches C only when the first range starts at byte 0 and
  the last ends at end of file, which is a shape an injection child never
  receives.
  One pinned geometry publishes an `ERROR` root where C publishes
  `source_file`.
  That case is an open defect in included-range clipping, recorded so the
  fix moves the pin.

- The Go result-compatibility arm's three members are now registered as
  named census subpasses, so a census receipt names the member that
  rewrote the tree instead of only the arm.
  Census receipts are recorded on the production route only; the default
  candidate route leaves `ParseRuntime().NormalizationPasses` nil.

### Fixed

- C enum lists with three or more enumerators no longer publish `ERROR` or
  `MISSING` nodes. A clean forest result can replace a recovered tree only
  after it covers the full source and contains no recovery nodes. This fixes
  [issue #667](https://github.com/odvcencio/gotreesitter/issues/667).

- `Node.HasErrorOrMissing` reports both recovery node forms. The
  `grammargen parse -strict` command now rejects either form.

- JavaScript, TypeScript, and TSX scanners now bind external results through
  each language's positional symbol table. Regenerated blobs no longer mistype
  shifted external symbols.

- The full-parse retry selector no longer releases an incumbent when a
  candidate aliases it. This restores selected roots across retry and compact
  fallback paths.

- The accepted-error retry ladder now honors explicit stack and merge caps.
  It also keeps bounded pass counts and configured wall budgets across retries.

- Kotlin published an `ERROR` root instead of `source_file` when a parse used
  `Parser.SetIncludedRanges` with more than one range and the parse entered
  recovery.
  The recovered-root normalization that owns this result was removed on
  2026-08-02 as dead code.
  The census behind that removal measured the fresh, over-64-KiB, incremental
  and pinned routes.
  It never measured the included-ranges route, and the member is live there.
  The member is restored.
  On the committed witness, `testdata/included_ranges/kotlin_work_queue_test.kt`
  with three ranges, the root returns to `source_file`, which is the kind the
  locked Kotlin C reference runtime publishes for the same input.
  Production reaches this route through injection.
  The member retags the root.
  Its downstream consequence is not always toward the reference runtime: on one
  measured file the retag lets a later stage flatten a clean
  `class_declaration` into root-level members.
  The route now has committed test coverage for Kotlin, in the root package and
  in the C-parity lane.
- The C-recovery missing-token search (`cHandleError` /
  `cDoAllPotentialReductions`, `parser_recover_c.go`) cloned the whole GSS
  stack without a work limit.
  A 4-byte erlang input and a 56-byte jsdoc input drove heap use past 2 GB
  in seconds.
  Two new loop ceilings now bound the search directly.
  `cRecoverMaxReductionCandidateAttempts` caps candidate attempts within one
  `cDoAllPotentialReductions` call.
  `cRecoverMaxMissingTokenTrials` caps total trials across one
  `cHandleError` search.
  `cRecoverMaxReductionCandidateAttempts` is the active mechanism.
  It is what stops every known witness and every corpus file measured so
  far.
  `cRecoverMaxMissingTokenTrials` has not fired once on any of them.
  It stays in place as an unexercised backstop for input this codebase has
  not sampled yet, not as a mechanism this fix currently relies on.
  A new `Parser.budgetScratch` pointer feeds GSS-scratch allocation into
  the existing 512 MB soft memory budget.
  The check already in the main parse loop now covers the C-recovery
  candidate search too.

  Measured through the shipped regression test on the `erlang_pfx_017_71b`
  witness (`testdata/recovery_memory_bound_witnesses/`): heap growth after
  this fix is 144.1 MB on the production route and 138.9 MB on the compact
  route, in well under half a second.
  Before this fix, the same input reached 695 MB and 52.0 seconds.
  Both post-fix numbers are the real, reproducible figures.
  An earlier draft of this fix reported smaller ones.

  `parser_memory_budget_runtime.go`'s `runtimeMemoryHardCeilingEnabled`
  function keeps its exact prior behavior.
  This fix adds a comment there recording why an earlier draft's
  source-length-independent hard ceiling was tried and dropped.
  It reopened issue #454's determinism symptom class on sub-64 KiB input.
  It also cost 3.6-9x more time on ordinary small parses, with no
  offsetting protection over the two loop ceilings above.

- The GSS-forest link cap could silently drop the widest hidden-symbol
  alternative when it tied a narrower one on score and error cost.
  The cap kept the earlier arrival by default in every tie.
  json5's flat-array grammar shape hits this tie constantly on ordinary
  input.
  Other forest-default languages hit it rarely or not at all on their
  current tables.
  The cap now keeps the wider alternative when two links tie on the same
  symbol and the same end byte.
  A narrower-tie or cross-symbol tie keeps the prior behavior unchanged.
  `Parser.ForestCapTieStats()` is a new method.
  It reports how often the tie fires and how often the fix changes the
  outcome.
  Set `GOT_FOREST_CAP_TIE_DUMP=1` to also record a bounded per-decision
  receipt list.

- Corrected the root-cause comment on the javascript declared-conflict
  election witness test.
  `grammargen/lr.go` already retains the `labeled_statement`/`_property_name`
  GLR fork that the real tree-sitter-javascript grammar declares.
  That fix landed 2026-03-16.
  The shipped `javascript.bin` blob predates the fix by two weeks.
  Nothing resynced the blob afterward, so the raw parse still diverges from
  the C oracle today.
  A new unit test pins the retention rule directly against the generator.
  Regressions now surface without a blob rebuild.

- The Swift optional-binding vs trailing-closure fix (#542) added a
  shift/reduce precedence branch to `grammargen/lr.go`.
  It ran before the declared-conflict retention check.
  It also matched ordinary undeclared conflicts in other grammars and
  picked the wrong side.
  JavaScript's `update_expression` vs `binary_expression` conflict is one
  example.
  The branch now runs last, after declared-conflict retention and the
  ordinary precedence ladder get a chance to resolve the conflict first.
  The Swift case from #542 still resolves correctly.
  A new test now guards javascript and typescript regeneration against the
  C reference parser on real source files.

### Changed

- Parser stop checks now skip inactive callbacks and keep the common callback
  direct. Result materialization reads the wall clock every 64 checkpoints.
  Cancellation and sticky stop checks still run at every checkpoint.

- GLR recovery now computes C-compatible error cost and visible counts in one
  tree walk. Memo indexing uses pointer-bit folds and checks the primary way
  first. Graph-structured stack (GSS) nodes store clean-zero merge results
  without a larger node layout. Extra-link mutations invalidate the result.

- C-recovery promotes an error stack to the graph-structured stack before
  reduction forks. Deep recovery branches now share their immutable prefix.
  The Swift recovery witness reduced time by 9.96%, bytes by 59.65%, and mean
  peak resident memory by 22.09%. The 20-seed combined suite reduced KDL
  recovery time by 1.20%, bytes by 13.14%, and allocations by 1.58%.
  Other parser timings stayed neutral.

- The randomized benchmark suite now accepts an exact recovery corpus file and
  language. The 20-seed comparison against the release boundary reduced the
  timing geomean by 1.77%. Elixir recovery improved by 15.21%, KDL recovery by
  9.12%, full parse by 1.16%, and incremental no-edit by 6.51%.
  `FactProgram` parse and extraction improved by 1.23%.
  The parser-core control stayed neutral. No timing, byte, or allocation metric
  had a significant regression.

- The guarded parser-core bytecode experiment now supports `REDUCE_CHAIN` and
  `REDUCE_SHIFT`. The corridor remains off by default. Each superinstruction
  also requires its own experiment gate.

- Synthetic-root replay now hashes frames and gap cursors, memoizes gap tokens
  and advance transitions, reuses scratch, and pools external token sources.
  Paged advance and close streams bound retained memo storage.
  Advance memoization cut the hard Elixir target latency by 22.62 percent.
  Close paging cut bytes per operation by 5.15 percent and allocations by
  60.48 percent. Its latency remained neutral across 20 balanced pairs. All
  16 combined-suite latency rows also remained neutral.

- Exact C and V runtime profiles now avoid certified duplicate retry work.
  Other grammars keep the conservative retry ladder.

- Performance counters now expose maximum resident memory, replay closure
  distribution, memo capacity skips, and parser stop attribution.

- The compact fresh-path route now skips two tail steps for a language with
  no live result-compatibility entry: the C-recovery-swallow resolver and
  the final-tree compaction pass.
  The eligible set is computed from
  `testdata/result_compat_ownership_v1.json`.
  It is not a maintained list.
  A future dispatcher arm cannot silently escape it.
  163 of 206 registered languages are eligible today.
  Go is not one of them.
  `dispatch.go` stays live, so `grammargen_lr` and the other three canonical
  Go fixtures still take the full tail.
  A deep-tree digest comparison (elided against unelided) is exact across
  every eligible language's smoke sample.
  It is also exact across every real-corpus file this campaign measured, up
  to 484 KB.
  This is a correctness-neutral simplification, not a measured performance
  win: the result-compatibility dispatch and its error-summary walk already
  run once, during materialization, for every language; the tail's own copy
  of that work was already unreachable in the common case before this
  change, eligible language or not.
  Two eligible-language timing probes (OCaml, Zig) and the Go warm-route
  benchmark all read within this shared host's noise floor, consistent with
  that finding.
  See the PR for the full reading and `docs/compat-tail-elision.md` for the
  corrected performance and correctness analysis.

- The condense-candidate dispatch path no longer passes a closure through
  two wrapper layers per event.
  Each shift, cohort, and reduction entry point now validates the scheduler
  owner and calls the uncheckpointed operation directly.
  Behavior is unchanged; every identity, work-count, and allocation check
  still passes.
  Local timing on a shared host showed no significant change across four
  fixtures.
  The host carried heavy background load throughout the run, so the result
  is not a sealed measurement.

- Removed the 64 KiB source-length eligibility decline from the compact
  admission switch.
  A fresh full parse of any size now attempts the compact route first.
  The scheduler's stop-control poll bounds a large or pathological input:
  it compares the compact core's own real retained-memory footprint
  against the same soft memory budget production honors, and falls back to
  production with a matching `ParseStopMemoryBudget` stop reason when the
  budget is exceeded.
  Every decline path now releases the compact core's retained capacity
  before returning, not only its logical record count, so a production
  fallback does not run alongside megabytes of memory an earlier declined
  attempt on the same parser left allocated.
  An operator watching `AdmissionCandidateCounters()` sees this directly:
  a large input that declines bumps the fallback count exactly as a small
  one always did.
  This bounds retained footprint, not the compact scheduler's own transient
  per-token allocation during a declined attempt; closing that remaining
  gap trades against how large an input the compact route can still serve,
  and is an open follow-up, not resolved by this change.
  Routing only changes; every canonical tree digest stays identical.

## [0.48.1] - 2026-08-04

### Fixed

- Anonymous separator nodes in Python import and subscript forms stay
  unfielded.
  The patch restores field-name behavior from the reference C runtime.
  This fixes [issue #660](https://github.com/odvcencio/gotreesitter/issues/660).

## [0.48.0] - 2026-08-01

### Added

- A validated Swift corpus now guards real-code parsing.
  Twelve files come from swiftlang/swift 6.3 and apple/swift-algorithms 1.2.1.
  A ratcheting expectations test fails on any regression or unrecorded fix.
  Five upstream grammar gaps are recorded in issues
  [#574](https://github.com/odvcencio/gotreesitter/issues/574) through
  [#578](https://github.com/odvcencio/gotreesitter/issues/578).

- The dispatcher census now reports distinct Ada, Apex, Bash, and Cooklang
  materialization subpasses.
  Compatibility-free probes record active and inert producer behavior.

- `grammargen -js-cli` now resolves `grammar.js` with Tree-sitter 0.26 or newer.
  It imports the temporary canonical `grammar.json` through the existing path.
  The explicit flag warns that grammar evaluation executes JavaScript.

- `grammargen -js-cli` now identifies a missing JavaScript runtime when
  Tree-sitter cannot start Node. The command help and README list both
  prerequisites.

- The canonical compact real-corpus matrix now records 70 direct routes,
  30 fallbacks, exactly 10 skips, and no divergence or error.
  The bounded current receipt covers 110 rows.

### Performance

- Live-header scoping reduces compact full-parse allocation counts by
  11.27 percent on the 235,626-byte Go fixture.
  Allocated bytes fall by 26.64 percent.
  Parse time remains statistically unchanged for that fixture.
  The rewrite fixture regresses by 19.99 percent.
  The query-compile fixture regresses by 15.77 percent.

- The parser now skips four redundant source reconstruction passes for
  certified isolated C# recovered roots.
  The receipt requires one one-byte error, no missing nodes, and matching raw
  top-level spans.
  The 137 KiB deletion witness still matches the pinned C parser.
  Median full-parse time falls from 8.24 seconds to 5.01 seconds.
  Median memory falls from 608 MB to 195 MB.
  Median allocation count falls from 286,009 to 9,663.
  `BenchmarkIssue454CSharpRecoveredFullParse` uses `GOMAXPROCS=1`,
  `-benchmem`, `-benchtime=1x`, and `-count=5`.

- The graph-structured stack shape walk now skips a duplicate cache lookup
  after a known head miss.
  The standard full-parse benchmark improves by 5.12 percent across 20 samples.
  Allocations remain at nine per parse.

- Graph-structured stack hashing now selects inline or pooled walk storage
  before it collects nodes.
  The 235,626-byte Go fixture allocates 24.84 KiB instead of 110.34 KiB.
  Allocations fall from 511 to 169 per parse.
  Parse time and maximum resident set size remain unchanged.

- Outer parser-state re-lex transactions now reuse one 4 KiB scanner-state
  buffer.
  The 235,626-byte Go fixture allocates 7.583 MiB instead of 48.073 MiB.
  Allocations fall from 10,404 to 513 per parse.
  Two stable pairs improve parse time by 9.52 to 15.46 percent.
  Maximum resident set size falls from 220,236 KiB to 185,920 KiB.

- Direct parser-state re-lex probes now use their existing outer transaction.
  This removes a redundant scanner-state snapshot.
  The 235,626-byte Go fixture allocates 48.07 MiB instead of 86.69 MiB.
  Allocations fall from 20,290 to 10,400 per parse.
  Three stable benchmark pairs improve parse time by 10.90 to 15.26 percent.

- The compact scheduler now stores its common rollback frontier inline.
  This removes one allocation from a fresh full parse.

- The fresh compact runner now reuses its scheduler storage across parses.
  Full-parse allocations fall from 15 to 14 per operation.
  Parse time and allocated bytes remain statistically unchanged.

- The parser now reuses its bound stop-check callback across compact full
  parses.
  This removes one allocation and 16 bytes per operation.
  Full-parse time and maximum resident set size remain unchanged.
  Incremental parses retain zero allocations.

- Fresh compact full parses now store the scheduler receipt inside the
  scheduler allocation.
  Full-parse allocations fall from 17 to 16 per operation.
  Parse time, allocated bytes, and maximum resident set size remain unchanged.

- The compact full-parse receipt now stores its acceptance value in the
  scheduler receipt allocation.
  Full-parse allocations fall from 18 to 17 per operation.
  Parse time and allocated bytes remain statistically unchanged.

- PR #498 moved the single-header compact dispatch cell onto the stack.
  This removes one allocation from the common full-parse path.
  The stable Go benchmark improves full-parse time by 15.57 percent.
  Allocated bytes fall by 20.25 percent.
  The allocation count falls by 7.89 percent.

- Certified graph-structured stack convergence now merges duplicate C# full-parse
  stacks while it retains their packed alternatives.
  The 137 KiB witness improves from 1.629 seconds to 98.7 milliseconds.
  Allocated bytes fall from 135.6 MB to 50.4 MB.
  The allocation count falls from 7,820 to 1,429 per parse.
  Incremental parses retain their previous merge policy.

- The compact full-parse runner now reuses buffers during canonicalization and
  tree materialization.
  Warm materialization drops from 136,584 to 272 bytes per operation.
  Its allocation count drops from 47 to 8.
  Total warm allocation drops from 20,440 to 5,208 bytes per operation.
  Total parse time remains statistically unchanged.

- The compact scheduler stores its one-element seed frontier inside the
  scheduler allocation.
  The warm full-parse benchmark drops from 20,352 to 20,328 bytes per operation.
  Allocations drop from 66 to 65 per operation.
  Parse time remains statistically unchanged.

### Fixed

- Swift now recovers an `if`/`else` whose comparison condition ends with a
  parenthesised member access in the then-branch (a call argument or a
  parenthesised negation). The then-block no longer swallows the trailing
  `else` as a call's trailing closure.
  This fixes [#560](https://github.com/odvcencio/gotreesitter/issues/560).

- A Swift optional generic type such as `Range<Int>?` now parses cleanly.
  The token source defers the closer to the DFA only when a reduce action
  closes an open `type_arguments` production.
  This fixes [#556](https://github.com/odvcencio/gotreesitter/issues/556).

- A Swift constrained extension with a multiline `where` clause now parses
  cleanly. The scanner carries the resolved previous rune across comment
  handoffs instead of re-reading a raw source byte.
  This fixes [#557](https://github.com/odvcencio/gotreesitter/issues/557).

- Swift nested `if let` chains inside methods now parse cleanly.
  The recovery pass brackets only the right-hand side of the binding.
  This fixes [#558](https://github.com/odvcencio/gotreesitter/issues/558).

- Three or more nested Swift generic type arguments such as `A<B<C<Int>>>`
  now parse cleanly. The split fires only when an unclosed `<` sits open, so
  custom operators such as `>>>` stay intact.
  This fixes [#559](https://github.com/odvcencio/gotreesitter/issues/559).

- A Swift method that contains a `for` loop over a range, followed by another
  method, now parses cleanly. The recovery pass also fires when the `for`
  statement forms with an error inside it.
  This fixes [#561](https://github.com/odvcencio/gotreesitter/issues/561).

- Raw error-cost walks now retain each captured child shape reference.
  A later mutable node update cannot create a recursive shape cycle.
  The Rust aggressive corpus completes all 25 bounded parses without a crash.

- TypeScript and TSX now parse `in`, `out`, and `in out` variance
  annotations on type parameters.
  The source overlay uses the semantics from upstream pull request 361.
  This fixes [issue #539](https://github.com/odvcencio/gotreesitter/issues/539).

- TypeScript and TSX now separate adjacent generic call signatures at a
  newline.
  The grammar uses the dedicated function-signature separator.
  Generic automatic-semicolon behavior remains unchanged.
  This fixes [issue #540](https://github.com/odvcencio/gotreesitter/issues/540).

- Compact reductions now merge only with live scheduler headers.
  Removed historical versions no longer consume the shared boundary link cap.
  This retires four real-corpus fallbacks without a tree divergence.

- Shared DFA token election now prefers one composable close angle over a
  wider close-angle token.
  Nested TypeScript union arguments now retain the generic-call lineage.
  This fixes [issue #541](https://github.com/odvcencio/gotreesitter/issues/541).
  Apex nested generic declarations now match the pinned C tree without a
  result rewrite.
  This retires the Apex generic local declaration compatibility pass.

- The full-parse retry ladder now retains a widened candidate until it reads
  the candidate's runtime receipt.
  This enables the existing combined stack-and-merge retry for the ZodUnion
  fixture.
  This fixes [issue #544](https://github.com/odvcencio/gotreesitter/issues/544).

- Swift optional bindings now keep the statement body separate from a trailing
  closure. The generator now uses exact advanced LR item precedence.
  Production, compact, and pinned C tests cover the correction.
  The regenerated Swift blob retains its exact runtime profile certification.
  This fixes [#542](https://github.com/odvcencio/gotreesitter/issues/542).

- The DFA lexer now splits adjacent Swift generic closers by parser state.
  It preserves `>>` when the active state accepts the shift operator.
  The pinned C oracle now matches without divergence.
  This fixes [#543](https://github.com/odvcencio/gotreesitter/issues/543).

- Native `grammar.js` import now ignores comments in semantic AST children.
  All 206 pinned grammars show import coverage increasing from 62 to 70.

- AWK recovery now captures the original splice parent before it constructs a
  replacement concatenation.
  This prevents self-parent links during recovered expression materialization.
  A locked 7,392-byte production fixture now verifies bounded completion and
  a stable tree digest.

- Compact recursive insertion now proves external token identity from exact
  scanner checkpoints.
  Mismatched or missing checkpoints fail closed.
  Locked Kotlin, OCaml, Perl, and Rust fixtures now reach their next parser gate.

- Forest result selection now preserves an existing same-symbol container.
  Its children must exactly match adjacent visible root containers.
  This removes the inert HTTP section-coalescing compatibility pass.

- The native reduction path now sets Dart switch-expression body fields.
  It now sets the target field for nested Elixir calls.
  This change removes two inert language-local field repairs.

- Compact graph insertion now persists exact predecessor merges across a
  bounded 16-level path.
  Non-exact nested edges and deeper paths still fail closed.
  Locked C#, Elixir, Perl, and Scala fixtures now reach their next parser gate.

- The compact admission census now separates runnable no-table-action stops
  from paused frontiers.
  The real-corpus matrix labels production error trees.
  Clean graduation coverage no longer counts recovery fixtures as parser gaps.

- Compact graph branches now re-lex one exact token span for each parser state
  when the shared symbol has no action.
  Each alternative keeps the shared byte range and scanner checkpoint.
  The medium Scala corpus now reaches compact acceptance.
  A separate proof for joined reduction paths still gates direct publication.

- Exact grammar profiles can now flatten certified same-span unary wrappers
  during reduction materialization.
  The F# profile removes its declaration-name compatibility walk.
  Expression and dotted identifiers retain their wrappers.
  Compact and forest routes retain fail-closed behavior.

- Native C-style recovery now owns Angular, BibTeX, Chatito, and Electronic Data
  Sheet materialization for every registered recovery witness.
  This removes four inert result-compatibility dispatcher arms.
  Compact and forest routes retain fail-closed behavior.

- Native parser results now retain the expected Hurl and INI root types.
  This removes both expected-root fallback compatibility arms as one class.
  Compact and forest routes retain fail-closed behavior.

- Native recovery now owns Forth and Luau recovery-action materialization.
  Forth keeps C-equivalent missing terminators and empty-definition errors.
  Luau keeps recovered `end` tokens as identifiers.
  This removes both result-compatibility dispatcher arms.

- Parser recovery now owns skipped error materialization for Robot variables
  and Scheme quote-family forms.
  This removes both compatibility dispatcher arms as one defect class.
  Compact and forest routes retain fail-closed behavior.

- C-style recovery now marks an absorbed `ERROR` token as named.
  Recovered INI trees now match the pinned C parser at this node boundary.

- The tracked dispatcher census now includes the locked JavaScript
  convergence fixture.
  The receipt exposes seven active compatibility rewrites on a direct route.

- Native Crystal scanner lookahead now skips whitespace after hash and
  named-tuple openers.
  Exact token boundaries retire the Crystal compatibility dispatcher arm.

- PRs #497 and #500 add locked GraphQL and Svelte direct-route fixtures.
  Each fixture records its source commit and SHA-256 digest.
  Dedicated C-oracle tests require exact trees and zero fallback.

- PR #499 initializes the native Typst scanner indentation stack on creation.
  Native scanner semantics remove the nested-list comma artifact.
  This retires the remaining Typst compatibility dispatcher arm.

- Accepted-error C# full parses now retry with a certified merge width after
  cap-one convergence.
  Exact grammar identity gates the policy.
  Explicit environment settings keep precedence.
  This preserves recovered declarations while clean full parses remain on the
  faster path.

- Native ReScript materialization now owns value identifier path aliases.
  This change adds two small corpus fixtures.
  It removes the ReScript compatibility arm.

- Native Linker Script recovery now owns named error nodes and root spans.
  This change adds clean and recovered corpus fixtures.
  It removes the Linker Script compatibility arm.

- The parser covers every byte in each recovered EBNF source.
  This change removes the EBNF compatibility arm.

- Native visible-wrapper election now owns D storage classes.
  This retires the matching result-normalization subpass.
  Native reduction also owns D variable-type qualifiers.
  Native call targets now match C for qualified, template, and simple callees.
  This retires the D dispatcher arm.

- The Cooklang smoke fixture now uses a valid ingredient instruction.
  The previous period required production recovery and was omitted from the
  resulting tree.
  The valid fixture routes directly while the recovered form still falls back.
  The smoke scorecard reports 200 direct routes and one fallback.

- Compact admission now treats zero-width extras as progress when their token
  end advances the parser boundary.
  COBOL fixed-format padding now routes directly without weakening the
  same-byte no-progress guard.
  The smoke scorecard reports 199 direct routes and two fallbacks.

- Compact admission now supports bounded no-lookahead reductions.
  One runnable head can reduce a synthetic EOF and re-elect at the same byte.
  Transparent gotos mark the reduced node as an extra.
  A root reduction requires authenticated EOF on the next election.
  Doxygen, JSDoc, and VHDL now route directly.
  The smoke scorecard reports 198 direct routes and three fallbacks.

- Compact admission now supports two certified acceptance-frontier shapes.
  HTTP and Robot can drop EOF siblings with no actions.
  Meson can select the sole primary accepted derivation.
  Exact blob profiles and field-aware C-oracle receipts guard these choices.
  The current smoke scorecard reports 195 direct routes and no divergence.

- Compact admission now permits certified converged-path reduction split drops
  for the exact Bash, Erlang, Haskell, and JavaScript artifacts.
  Field-aware C-oracle receipts cover each selected compact tree.
  Three real-corpus files and the Haskell smoke fixture now route directly.

- Compact reduction outputs now carry their multi-pop fact directly.
  This avoids two full work snapshots on every reduction.
  The stable full-parse control improves by 8 percent against `main`.
  Full-parse allocation falls by 12 percent with no new allocations.

- The dispatcher census now records each live D and Objective-C subpass.
  Exact fingerprints retain spans, points, fields, flags, and parser states.
  The census does not materialize compact final-child references.

- The parser now folds raw descendant content into certified
  materializing-shape hashes.
  This prevents shallow GSS merges from discarding Objective-C method types.
  The parser now owns those identifiers before result compatibility.
  This retires a fifth Objective-C normalization subpass.

- Generic result selection preserves both valid Objective-C `sizeof` branches.
  It selects the C-equivalent expression branch for an unknown type name.
  This retires the final Objective-C subpass and its dispatcher arm.

- DFA keyword promotion now owns Arduino primitive types before compatibility.
  Native materialization owns Objective-C protocol type identifiers.
  This retires Arduino's dispatcher arm and the matching Objective-C subpass.

- Erlang macro replacement election now stays in the parser.
  It distinguishes function clauses from case and receive clauses.
  Reduction already emits exact top-level form spans.
  These owners retire the Erlang result-compatibility arm.

- Compact parse-state replay now visits each derivation node once.
  Its depth-first worklist retains only the active derivation path.
  The stable full-parse benchmark improves without new allocations or
  incremental regressions.

- Clean roots now keep hidden, childless whitespace extras as span coverage.
  They do not publish those extras as children.
  Visible comments and error-root evidence remain unchanged.
  Final-child filtering now preserves fields without materializing lazy ranges.

- The shared root-extra classifier now drops zero-width scanner tokens from
  child lists. The repetition-skip fold also stops the historical Typst
  comma artifact. These producer rules retire two returned-tree walks.
  Typst keeps its dispatcher arm for other repairs.

- Root spans now exclude unowned leading token padding through one shared
  materialization rule. This removes seven language-local repairs.
  Compact admission now accepts the same first-token start as the C oracle.
  Squirrel's result-compatibility dispatcher arm is retired.

- Rust dot ranges now parse without a result repair.
  The exact collapsed-child policy retains each bare `..` token.
  The merged-left-side conflict rule selects chained dot-range shifts.

- The no-live-action re-lex no longer checks the grammar's name. This recovery
  step re-lexes the lookahead when no live stack has any parse action for it,
  and it was gated to JavaScript. The condition it guards is grammar
  independent: `noLiveStackCanAcceptLookahead` already proves that no live stack
  can consume the token, so re-lexing cannot take a token another version was
  going to use, whatever the grammar. Every grammar now gets the same recovery
  step. This retires one per-language gate from the parser core.

- Large GLR parses allocate far less. Two hot-path buffers grew without
  amortization or reuse:

  - The three merge-scratch helpers (`ensureMergeResultCap`,
    `ensureMergeSlotCap` and `ensureMergeLargeSlotCap`) allocated exactly the
    requested length. Every merge pass sizes them from the live-stack count, so
    a parse whose stack count climbs reallocated the whole buffer on each
    increment, which makes total allocation grow with the square of the peak
    stack count. One element of the large-slot buffer holds two 256-entry
    arrays, so this dominated wide parses. They now double.
  - `gssNodeCanReach` built a new fallback map each time a link graph outgrew
    its 64-entry local array. On grammars that reach that size routinely, the
    map became the largest single allocation in the parse. The fallback map is
    now pooled and reused.
  - `gssNodeHash` grew a fresh walk buffer on the heap whenever an unhashed
    chain was longer than its 32-entry inline array. Ordinary parses stop at
    the first already-hashed node, so this only bites where an error path
    rebuilds deep chains: a PHP edit that introduces a transient error spent
    124 MB there. The walk buffer is now pooled.

  Measured on a C# corpus of repeated method declarations, with the grammar
  loaded before measuring: allocation per source byte falls from 119,036 to
  17,207, total allocation for an 8.7 KB input falls from 989 MB to 143 MB, and
  the parse runs in 200 ms instead of 335 ms. On a 137 KiB input, allocation
  falls from 5462 MB to 1111 MB and the parse takes 4.0 s instead of 6.5 s.
  Java, C++, Go and JavaScript allocate exactly as before.

- A GLR stack that needs a different tokenization of the same bytes now gets
  one. tree-sitter C lexes once per parse version, so two versions in different
  states can receive different symbols for the same characters. This engine
  lexes one token for every stack, which is cheaper and correct while all live
  stacks accept that token. Where one stack's state required a different symbol,
  that stack found no parse action, paused, and the condense step dropped it
  because an unpaused rival was still alive. The rival then reached a dead end
  and the whole file became one `ERROR` node. Scala shows the failure most
  clearly, because `+`, `-`, `!` and `~` are its only prefix operators: in
  `if (a) c + 2` the correct derivation needs `+` as the generic
  `operator_identifier`, while the rival needs the dedicated `+` token that
  exists so `prefix_expression` can spell unary plus. `while (a) c + 2` failed
  the same way, and `if (a) c * 2` always worked because `*` is not a prefix
  operator. The parser now re-lexes at the stack's own byte offset with the
  stack's own lex mode before pausing it. It adopts the result only when the new
  token covers the same byte span, which keeps every version at the same offset,
  and only when the stack's state has a real action for the new symbol. The
  re-lex reads the internal lexer only, so no external scanner state changes.
  Clean parses are unaffected, because the probe runs only where a stack would
  otherwise pause. Any grammar whose characters lex differently by state gains
  the same protection.

- Parse results no longer depend on garbage-collection timing. The soft
  per-parse memory budget stopped a parse when `runtime.MemStats.HeapAlloc` or
  `runtime.MemStats.Sys` grew past the budget. Both values cover the whole
  process, and the garbage collector paces both, so the stopping point was not
  a function of the input. The same bytes returned a different tree on each
  run. Five parses of one 137 KiB C# file returned five different trees, of
  1, 11048, 19178, 22928 and 31928 nodes. Each tree reported
  `HasError() == false` over only part of the input. The budget arms at 64 KiB,
  so every language was exposed above that size. Only the absolute hard ceiling
  (`GOT_PARSE_MEMORY_HARD_CEILING_MB`) now stops a parse from a runtime memory
  reading, because that ceiling guards against running out of memory and is not
  a shaping decision. The arena budget, the scratch budget, and the node and
  stack limits continue to bound memory. Those layers measure what the parse
  itself allocated, so they stop the same input at the same place every time.
  A downstream user reported this behavior in issue #454.

### Changed

- v0.48.0 adds fields to `FullParseAcceptedErrorRetryProfile`, `ParseRuntime`,
  and `DiagnosticParserCoreGenericWork`. Change unkeyed literals to keyed
  literals before you upgrade.

### Removed

- **The Bash command-name concatenation repair.** Native reduction now
  constructs the complete command name before result compatibility.
  The historical producer, all result routes, and the isolated C oracle match.
  The 25-case Bash corpus matches baseline `83548f55` exactly.
  Three unrelated Bash subpasses remain live.

- **The D template-call type result repair.** Generic result election now
  preserves a visible named unary wrapper over its direct-child alternative.
  Production, forest, incremental, and isolated C-oracle receipts match.
  Four unrelated D subpasses remain live.

- **Two Objective-C result repairs.** Exact stack-node equivalence preserves
  deep alternatives for generic alias-target selection.
  Native selection now owns `@encode` identifiers and function-pointer
  expression shapes.
  Production, incremental, and field-aware C-oracle receipts match.
  Native selection also owns single and concatenated `@` strings.
  Raw-shape equivalence now preserves compound struct type specifiers.
  Two unrelated Objective-C subpasses remain live.

- **The D module-bound result repair.** Native reduction already excludes
  leading comments and trailing trivia from each `module_def` span.
  Production, compact, forest, incremental, and C-oracle routes match.
  Incremental parsing reuses the old tree.
  The D dispatcher remains live for unrelated shape repairs.

- **The HCL root normalization pass.** Shared root finalization now removes
  hidden whitespace extras at every root position.
  Native reduction already produces each exact HCL body span.
  Production, compact, forest, incremental, and locked C receipts match.
  The three-file census found no mismatch across 114 body nodes.
  This removes the HCL result-compatibility dispatcher arm.

- **The Haskell section-span result repair.** Native reduction and root
  finalization already produce the exact `imports` and `declarations` ranges.
  The real-corpus census found no remaining rewrite.
  Production, compact, incremental, and locked C receipts match.
  The forest route retains its existing section reduction-cap limit.
  This removes the remaining Haskell dispatcher arm.

- **The source-driven collapsed-token repair family for HCL, CPON, C#, and
  PowerShell.** Reduction now preserves each required anonymous token child.
  The same-name collapse keeps CPON null nodes childless.
  This removes the CPON dispatcher arm.
  The other three arms remain live for unrelated repairs.
  Compatibility-free, production, compact, forest, incremental, and isolated
  C-oracle receipts return the same trees.

- **The CUE, Git Commit, and R alias-map result repairs.** Their pinned blobs
  now carry the nonterminal alias metadata from each C parser table.
  Materialization keeps the required named child under each collapsed wrapper.
  Production, compact, forest, incremental, and locked C receipts match.
  CUE also proves nonzero old-tree reuse.
  Git Commit and R record their external scanner reuse limit.

- **The trailing root and child span compatibility family.** Materialization
  now owns the exact spans for Caddy, Comment, Fortran, Just, Nginx, Nim,
  Pascal, Pug, and RST. The compact scheduler admits progressing zero-width
  external extras. Forest publication omits zero-width synchronization extras
  as children while it retains their source-range ownership. Native producer,
  production, compact, forest, incremental, reuse, and isolated C-oracle
  receipts support the removal of four dispatcher arms.

- **The Lua, Make, and Zig field-projection passes.** Reduction now projects
  inherited and direct fields through hidden productions.
  The Zig grammar metadata emits initializer lists without `field_constant`.
  Compatibility-free, production, compact, forest, incremental, and locked C
  receipts return the same fields.
  Make and Zig preserve old-tree reuse.
  Lua records its external scanner reuse limitation.

- **The Haskell and Erlang root field repairs.** Reduction now retains each
  inherited field conflict and projects it by an exact named-symbol match.
  Root acceptance preserves producer field metadata when it absorbs trivia.
  Compatibility-free, production, incremental, and isolated C-oracle receipts
  preserve the expected root fields.

- **The Scala returned-tree span repair subfamily.** A language-neutral
  in-place rewrite refresh now preserves a valid producer-owned span and can
  widen it. This change deletes the Scala function-end and case-clause helpers.
  It also removes the second-pass root-end call and its duplicate case-clause
  block. Production, compact, forest, changed incremental, fresh, and
  locked C routes return the exact ranges and points. Scala incremental reuse
  remains unsupported and reports zero reuse.

- **The duplicate Scala returned-tree repair calls.** Recovery, field, and
  annotation repair now runs only in the canonical compatibility pass.
  Mandatory fixtures and the authenticated corpus report zero mutations when
  the deleted calls run again.

- **The shared returned-tree fixpoint.** The last Scala arm became inert after
  checkpoints A and B. The publication paths no longer call a repeated
  post-finalization normalizer.

- **The HTML returned-tree range fixup.** Materialization now extends recovered
  custom elements through each structural `_implicit_end_tag` child.
  Production, compact, forest, and incremental routes return the exact
  absolute ranges. The incremental route also proves nonzero old-tree reuse.
  The locked C reference parser returns the same recovered ranges and points.

- **The generic terminal-leaf tree mutation.** Reduction and alias
  materialization now own the terminal shape. Production, compact, forest,
  incremental, scanner-aware corpus, and locked Go C-oracle receipts find no
  retired shape. The exact retry error summary and stop polling remain as a
  read-only full-tree walk.

- **Three dead per-language result-normalization dispatcher arms** (R2 of
  `docs/root-normalization-retirement.md`). The three are OCaml's collapsed
  named-leaf restoration, Ruby's top-level module bound shrink, and HTML's
  ERROR-root nested-custom-tag reconstruction
  (`normalizeHTMLRecoveredNestedCustomTags`). At the R2 checkpoint, HTML's
  separate range function stayed live. The R1 item above now removes that
  function independently. A real-corpus census measured zero rewrites for all
  three dispatcher arms. Native-parse tests confirm that the reduce engine
  already produces the corrected shape without them.

  A fourth candidate, Elixir, stayed live. Its census also measured zero
  rewrites over the real corpus. A native-parse regression test found the
  cause: the corpus sample lacked the triggering construct. Two consecutive
  top-level comments — a common file-header shape — still lose their hidden
  `_newline_before_comment` sibling without the normalizer. The ownership
  registry keeps all four entries as historical receipts.

### Changed

- JavaScript program-end finalization now has one authoritative compatibility
  owner. The redundant returned-tree second pass is retired; production,
  compact final-child-ref, forest, and incremental publication continue to use
  the canonical JavaScript compatibility pipeline before the tree is exposed.

- Clean hidden whitespace-only root tails are now owned by root finalization,
  retiring a generic compatibility pass while preserving error-root recovery
  extras and lazy compact child references.

### Performance

- Same-length single-byte replacements now mark the affected path without
  recomputing unchanged spans. Other edits and compact child references keep
  the general editor. The pinned incremental benchmark improves 2.10 percent
  with zero allocations.

- Fresh parse finalization now computes the retry error summary while it wires
  parent links. This removes one complete tree traversal.
  Deferred and incremental paths retain their separate summary walk.
  Under-flagged errors and stop polling keep their existing behavior.
  The pinned Go full-parse benchmark improves 0.81 percent.

- Parser retry policy now snapshots override presence with each parsed value.
  This removes repeated environment lookups from the incremental hot path.
  The pinned edited incremental benchmark improves 1.28 percent with zero
  allocations.

## [0.47.1] - 2026-07-28

### Fixed

- Recovery reductions preserve deferred parent links during fresh parses.
  Valid Go files remain complete during final result materialization.
  The invariant guard still rejects invalid transient replacements.

## [0.47.0] - 2026-07-22

### Changed

- **Stateful GSS forest trees now enter incremental reuse through exact
  checkpoint receipts.** Admission requires the scanner's generic checkpoint
  and incremental-reuse capabilities, then authenticates non-empty start and
  end snapshots at every reachable token boundary. A missing endpoint declines
  the forest as `scanner_checkpoint_unavailable` instead of letting distinct
  unrepresentable states collide as empty snapshots. A length-changing
  stateful witness requires actual subtree reuse and deep fresh-tree equality;
  a synthetic absent-checkpoint scanner locks the fail-closed path.

- **GSS forest trees now use capability-based incremental admission.** Forest
  construction records exact pre-goto ownership for every reusable subtree,
  and the reuse cursor requires that ownership before transferring top-level
  nodes. Languages without an external scanner, plus scanners with an explicit
  stateless/failure-preserving proof, can therefore reuse forest-built trees
  without a language-name allowlist. AWK, KDL, Nix, Squirrel, and Uxntal are
  newly admitted through a shared multi-position and 137 KiB fresh-tree
  differential.

- **JavaScript, TypeScript, and TSX leading incremental reuse is admitted.**
  The generic byte-identity, fragility, and scanner gates now govern unchanged
  leading siblings without a language-name holdback. Exhaustive clean byte-edit
  sweeps compare the complete incremental tree directly with a fresh parse, and
  the 20 KiB/137 KiB latency gate plus its opt-in 1 MiB tier lock middle and
  end edits to small, size-independent work counters. Transient-error
  insert/delete/replace edits retain separate recovery and memory bounds.

- **TypeScript and TSX now parse import-type queries in generic call type
  arguments.** Forms such as `foo<typeof import("module")>()` and
  `foo<import("module").Name>()` use a pinned upstream grammar overlay that is
  applied identically during ts2go generation and C-oracle parity builds.
  Ordinary dynamic `import()` expressions remain call expressions.

- **Eight additional stateless scanners are certified for changed-edit reuse:**
  Comment, Dhall, DTD, Foam, Godot Resource, Kconfig, Odin, and RON. Each
  passes the shared multi-position 4 KiB edit matrix and a 137 KiB
  changed-length fresh-tree differential with actual subtree reuse. Kconfig's
  deliberately small 16-byte macro floor keeps its parser-level ownership
  residual visible without treating performance as a correctness gate.

- **Twelve more stateless scanners are certified for changed-edit reuse:**
  EditorConfig, Fennel, Fish, GN, Janet, Julia, Less, Liquid, Pkl, Racket,
  TableGen, and Yuck. The shared fresh-tree matrix enforces real reuse across
  edit classes and positions, with measured 137 KiB floors that preserve low
  ownership-reuse cases as visible performance residuals.

- **The stateless-scanner admission matrix now also covers Gleam, Move, Tcl,
  and WGSL.** Each scanner passes the shared 4 KiB multi-position edit matrix
  and 137 KiB fresh-tree differential with a measured reuse floor. AWK and
  Squirrel remain fail-closed because their old trees use the GSS forest fast
  path; scanner statelessness alone does not bypass that parser-level gate.

- **Stateless external-scanner reuse now covers Cue, D, Elixir, and Erlang.**
  Capability markers replace language-name admission, while a strict recorded
  pre-goto ownership check prevents stale whole-sibling transfer for the
  certified stateless class. A shared differential matrix covers
  insert/delete/replace edits across 4 KiB fixtures and a 137 KiB macro lane,
  requiring real reuse and exact fresh-tree identity. Checkpointed reuse also
  authenticates scanner state at the current lookahead's start rather than its
  post-lex live state. Stateful scanners without a complete proof remain
  fail-closed.

- **HTML external-scanner reuse is checkpoint-certified for clean old trees.**
  The complete open-tag stack now serializes exactly or returns an absent
  checkpoint; oversized depth, custom names, and buffer exhaustion can no
  longer truncate or alias state. Malformed checkpoint bytes are rejected,
  failed scans preserve state, and token relexing rejects absent start or live
  checkpoints. Changed-length edit witnesses at three positions plus a 137 KiB
  lane require real reuse and fresh-tree equality. Error-bearing old HTML trees
  remain an explicit fresh-parse fallback while recovery ownership is still
  uncertified.

- **SQL external-scanner reuse is now checkpoint-certified.** The runtime's
  checkpoint and checkpointless-reuse gates are capability based rather than
  language-name allowlists. SQL records a complete dollar-quote-tag state
  whenever it fits the checkpoint buffer (including an explicit empty-state
  checkpoint), preserves that state on failed scans, and fails closed only for
  incremental reuse when a valid tag is too large to restore exactly; full
  parsing continues to accept the tag, matching C semantics. Svelte remains
  opted out of changed-edit reuse pending certification of its scanner-wide
  raw-text and expression-block behavior.
  Clean and recovered insert/delete/replace witnesses at the start, middle,
  and end of roughly 20 KiB and 137 KiB files enforce fresh-tree equality,
  full-span coverage, deterministic work, and bounded memory; an opt-in tier
  repeats the matrix at 1 MiB. This is a correctness/admission certification,
  not an O(edit) claim: the 1 MiB lane is catastrophe-bounded and its measured
  allocation/RSS scaling remains an explicit performance residual.

- **Collapsed named-leaf ownership now covers exact adapted artifacts.**
  The 23 registered parent/raw-child pairs compile into the native reduction,
  alias, forest, and compact-materialization policy for exact built-ins as well
  as true adapted clones retaining the exact-profile receipt and exact named
  parent/raw-child metadata identities; display-name or pair-level metadata
  matches do not admit arbitrary custom grammars. Focused adapted
  incremental/fresh witnesses require exact
  deep-tree equality and zero safety-net rewrites. A quantified synthetic
  lost-identity residual remains unsupported because its live construction
  provenance is unknown. No language-specific normalizer was added.

## [0.46.0] - 2026-07-21

### Added

- **Phase-3 admission switch** (PR #417). A per-parser option, a global
  option, and the `GTS_ADMISSION_CANDIDATE` environment variable route
  eligible full parses through the compact parser core. Internal
  sub-parsers stay suppressed. A 206-language scorecard guards the
  route: 48 languages parse byte-exact, 153 fall back fail-closed, 5
  skip, 0 diverge. It initially landed off by default; the Changed entry
  below records its promotion after the admission evidence was sealed.
- **Compact-route coverage census** (PR #419).
  `docs/compact-route-coverage-census.md` classifies the 153 fallback
  languages into five scheduler-capability classes. The census found no
  multi-derivation blockers.
- **Oracle v3 parity tools** in `cgo_harness` (PR #413). The root
  library module is unchanged by that PR.
- **The W5 editor-latency matrix now covers five languages.** Go,
  JavaScript, TypeScript, Python, and CSS run insert, delete, and replace
  edits at the start, middle, and end of roughly 20 KiB and 137 KiB inputs;
  the manual full sweep adds 1 MiB. JavaScript and TypeScript also carry a
  transient-error delete lane with deterministic ceilings for parser work,
  retries, stack width, and allocator memory, while retaining fresh-parse
  structural equality as the correctness oracle.
- **External-scanner incremental-reuse contracts are now published per
  language.** The 119-language matrix distinguishes certified, bounded,
  explicit-opt-out, and uncertified scanners. SQL, HTML, and Markdown now
  document their fail-closed production full-parse fallback for changed edits
  after the narrow token-invariant leaf exception declines.

### Changed

- **Result compatibility cleanup.** A source-of-truth ownership registry,
  supporting documentation, and a CI guard now track compatibility passes and
  their retirement criteria. The dead terminal-normalization wrapper was
  removed, along with the unreferenced `walkResultTreePostorderUntil` and
  `rewriteResultTreeChildrenPostorderWithStats` traversal helpers. Recovered-tree
  cycle repair was replaced by an always-on, non-mutating validator that fails
  closed with the public `ParseStopInvariantViolation` reason. The roadmap now
  puts repository maintenance, explainability, documentation, ownership
  receipts, and upstream retirement of normalization shims before the next
  major performance milestone; performance gates remain advisory during this
  cleanup. All 23 collapsed named-leaf rows for six exact-profile built-in
  languages now materialize natively across their admitted routes. The generic
  compatibility walk and its synthetic reconstruction helpers are retired;
  exact-profile adapted artifacts retain the native route, while unregistered
  custom artifacts fail closed instead of inferring children from display names.

- **Compact admission now ratchets breadth, depth, and edit reuse.** The shared
  production clean-tail proof admits compact roots that stop immediately before
  trailing parser padding, raising the 206-language smoke scorecard from 48 to
  166 byte-exact routes (35 fail-closed fallbacks, 5 token-source skips, 0
  divergences). Representative multi-line fixtures freeze production and
  candidate digests, while CI enforces both the breadth floor and routed depth.
  Admission materialization now carries a per-tree parser-state replay proof:
  grammars with complete required states and proven scanner quiescence retain
  incremental subtree reuse; unproven/stateful scanners remain barred, apart
  from independently re-lexed token-invariant single-leaf edits. Certified or
  explicitly requested forest routes retain precedence unless the caller
  explicitly forces the compact candidate.

- **The compact parser core is now the default full-parse route for eligible
  languages** (Phase-3 admission flip). `Parser.Parse` routes a fresh, full,
  production-DFA parse of an eligible grammar through the compact
  `internal/parsercorephase0` engine, then materializes a public tree. The tree
  is byte-exact with the production engine on the routed, verified surface: the
  166 byte-exact scorecard routes and the canonical fixtures. The runner's strict
  acceptance gate fails closed to production on any input it cannot reproduce
  byte-for-byte. The compact engine promotes from the `gts_parsercorephase0`
  opt-in tag into the default build; the emergency opt-out tag
  `gts_no_parsercorephase0` compiles it back out.

  Evidence for the admission:

  - **Correctness.** 206 of 206 curated parity fixtures pass. The deep-tree
    digest is 100 percent exact on the canonical fixtures. The 206-language
    scorecard through the switch reports 166 byte-exact routes, 0 divergences,
    35 fail-closed fallbacks, and 5 token-source skips.
  - **Fail-closed generic-call conflict class.** On the ambiguous Go construct
    `Foo[int](a)`, production and the tree-sitter-go C oracle select
    `type_conversion_expression(generic_type)`. The compact scheduler cannot
    yet rank that conflict by dynamic precedence, so it declines the
    unauthorized tie fold and falls back to production. The returned tree stays
    byte-exact while `TestAdmissionCandidateGoTypeConversionFailsClosed` keeps
    the route/fallback behavior explicit.
  - **Timing.** The quiet-host publication run (lane
    strictboundary-20260720T231334Z-v6 phase3, n2d-standard-4, 5 ABBA cycles)
    measured a production-over-candidate geomean speedup of 1.8321 (gate is at
    or above 1.0204) on the warm direct-runner path
    (`BenchmarkParserCoreFreshFullCanonical`). The worst fixture ran 1.526 times
    faster. Peak resident set size fell on all four fixtures (grammargen_lr
    94 MB against 206 MB). The deep C-oracle parity preflight passed in the same
    run.
  - **Adapter Parse-path reconciliation.** The 1.8321 geomean was measured on
    the direct-runner path, not on `Parser.Parse`. The initial adapter regressed
    time and allocations, because it rebuilt the compact action tables on every
    fresh `Parser` and did not pool the materialization scratch. Two adapter
    fixes removed that tax:
    - a per-`*Language` table cache builds the converted action and reduction
      tables once per language (about 95 KiB retained) instead of once per parse;
      and
    - a per-`Parser` runner reuses the materialization scratch, the public-tree
      node buffers, and the Go-compatibility walk stack across parses.

    After the fixes, warm route-ON allocations on `BenchmarkGoParseFullDFA` fall
    from about 71 to about 24 per operation, and bytes per operation from about
    99 KiB to about 17 KiB. On the human-authored Go fixtures
    (`BenchmarkGoParseWarmRealDFA`) the routed parses now run about 1.45 to 1.66
    times faster than production through `Parser.Parse` (geomean of the routed
    fixtures about 1.57 times). The remaining gap to the 1.8321 direct-runner
    number is the production Parse tail the adapter still runs. On the synthetic,
    highly repetitive `BenchmarkGoParseFullDFA` source the routed parse stays
    slower than production, because the compact scheduler dominates that input;
    the speedup holds on the human-authored fixtures the sealed number measured.
  - **Sealed epoch (v0.45.0).** The compact route measured 2.9975 times the C
    reference against the production route at 5.526 times, hardware-attested and
    verified. Allocation levels sit at 92 to 316 allocations per operation
    against 14 to 200 for production, admission-compatible per the 2026-07-20
    owner ruling.
  - **Known gap.** The candidate retained-heap column reported NA in the phase3
    environment; resident set size is the resource evidence.

  Escape hatch: set `GTS_ADMISSION_CANDIDATE=0` (or `false`, `off`, `no`) to
  force every parse back onto the production route. Any other value, or an unset
  variable, keeps the compact route on. `(*Parser).SetAdmissionCandidateRoute`
  overrides the process default per parser.

  Dual-route statement: `ParseIncremental` and every reuse-consuming parser
  operation stay on the production engine. A compact old tree may now supply
  reusable subtrees only when its materialization attached the required replay
  states and its scanner is provably quiescent; otherwise reuse fails closed to
  a full production parse. Token-invariant single-leaf edits are separately
  re-lexed and admitted only on exact symbol/span identity.

  Memory-budget contract: the compact scheduler does not poll the automatic
  large-input memory budget. The switch declines every input at or above the
  source-length floor where the production route arms that budget (64 KiB), so
  such inputs stay on production and honor `ParseStopMemoryBudget`. Adding
  scheduler-level budget polling to the compact route is the follow-on campaign.

- **The GLR steady-state merge now compares structure before score**
  (PR #416). The TypeScript and TSX steady-state merge budget widens
  from one survivor to two. The wider budget activates the structural
  comparison at the merge site. This is the structural cure for the
  detector class behind issue #389 and issue #402.
- **Admitted clean top-level edits now reuse both leading and trailing
  sibling runs** (PR #418, PR #421, campaign O(edit)). PR #418 bounds arena
  normalization to the edited range. A 1MB clean-Go near-top keystroke drops
  from about 356ms to about 53ms. PR #421 splices the leading run of unchanged
  top-level items, the mirror of the trailing block-splice. A 1MB mid-file
  keystroke drops from about 269ms to about 67ms for Go, and from about 168ms
  to about 45ms for CSS. At 137KB, mid-file reuse rejects fall from 16,450 to
  10. Length- or point-changing `Tree.Edit` calls still maintain coordinates
  through affected trailing sibling subtrees, so this is not an absolute
  whole-call `O(edit)` claim. JavaScript, TypeScript, and TSX keep their
  previous leading-run behavior until the T2c scanner proofs land. The W5
  latency gate locks these counters per edit position.

### Removed

- **The four TypeScript merge-width source-text detectors** (PR #422).
  The structure-before-score cap-two steady state from PR #416
  subsumes all four detector shapes, so the detector functions, their
  wrapper gates, their helpers, and the test seam are deleted. A
  byte-match test proves cap-two produces trees identical to the old
  cap-six widening on the destructured shape, at 300KB scale. One
  behavioral change: an accepted-error incremental retry for the
  destructured-arrow shape now runs one base-cap retry pass. The
  strict retry-preference gate keeps the selected tree the same or
  strictly better.

## [0.45.0] - 2026-07-20

### Fixed

- **TypeScript arrow functions with a return-type annotation** no longer
  collapse to `ERROR` as a `const`/`let` initializer (issue #402, PR
  #409). Example: `const f = (a: A): B => { ... }`. The typed-arrow and
  destructured-arrow-return-type detectors added in PR #389 did not
  cover this shape. Neither required the arrow to be immediately
  preceded by `)`. A typed, non-destructured parameter list combined
  with an explicit return-type annotation fell through both. This fix
  adds a dedicated detector for that shape. It widens the merge budget
  to two survivors, matching the typed-arrow and default-parameter
  cases. TSX was unaffected; its wider JSX conflict set already kept a
  second survivor alive. The detector also covers parenthesized return
  types: `(a: A): (B) => a`, `(a: A): (string | number) => a`, and
  `(a: A): (() => B) => a`. Its backward colon scan now balances
  parentheses, so a colon nested inside the return type is not mistaken
  for the top-level boundary. This is the **third** source-heuristic
  merge-width detector guarding the same root cause as the PR #389
  default-parameter fix. That root cause: the GLR engine's steady-state
  merge budget discards a live fork by score before any structural
  comparison runs.
  The structural cure — comparing candidate forks structurally before
  falling back to score at the merge site — remains tracked, in active
  development on `codex/glr-structure-before-score`. Like its
  siblings, this detector's backward scan is bounded to a
  512/2048-byte window; a return-type expression whose own top-level
  colon sits past that window silently misses the widening.
- **Go `new(pkg.Type)`, `new(*T)`, `new(**T)`, and parenthesized type
  arguments** now byte-match the C oracle (issue #375 class, valid
  forms; PR #408). The parser previously parsed these structured type
  arguments as expressions, breaking node-for-node parity.
  `new`/`make` selector, unary, and parenthesized arguments now
  relabel to the C grammar's type shapes. Byte spans and tree
  structure are preserved. A composite-literal guard suite locks the
  boundary: this fix does not touch composite-literal type positions,
  which already matched.

### Added

- **A per-boundary scanner-quiescence classifier replaces the
  external-scanner reuse allowlist.** It proves reuse soundness for
  stateless scanners, refutes stateful opt-out scanners, and defers to
  the checkpoint match for checkpoint-based scanners (PR #407,
  campaign O(edit) workstream W4). A new exported
  `StatelessExternalScanner` interface, in `language.go`, lets a
  scanner declare itself stateless. `GoExternalScanner` now implements
  it, under five documented proof obligations that block cross-token
  state from leaking into reuse boundaries. A new
  `ReuseRejectScannerUnquiescent` counter, on `IncrementalParseProfile`,
  tracks boundaries the classifier rejects. An adversarial oracle
  sweep proves byte-identical incremental and fresh Go parses across
  newlines, raw strings, and comments; the classifier never blocks Go
  reuse on that sweep. This change is behavior-neutral groundwork: it
  does not itself change any parse output.
- **A new editor-latency CI gate enforces deterministic incremental
  counters.** It sweeps insert, delete, and replace edits at three
  file positions, across roughly 20KB, 137KB, and 1MB fixtures in four
  languages (PR #405, campaign O(edit) workstream W5). The gate checks
  counter ceilings, byte-reuse floors, and structural parity against a
  fresh parse, on every default `go test` run. A determinism check
  runs fresh parsers on identical inputs and asserts the counters
  match exactly. A manual-dispatch CI job covers the slower 137KB and
  1MB tiers.
- **A query silent-wrong witness suite locks four query tranches
  against the C oracle** (PR #410). D1 range queries, D4 `MISSING`
  alternation patterns, and D8 `#is?` predicates now match the oracle
  under committed tests. D3 supertype patterns and D5 quantified
  captures are tracked, not fixed: their tests are skip-guarded, with
  the query-engine-scope limitation documented inline.

### Improved

- **Unchanged top-level siblings now splice back as a single block**,
  inside one parse-loop iteration, instead of one sibling at a time
  (PR #411, campaign O(edit) workstream W1 block-splice composition).
  A new scanner-quiescence check lets a quiescent, non-checkpoint
  external scanner skip re-lexing an unchanged span in O(1), instead
  of token by token. A new `BlockSpliceSteps` profiling counter tracks
  block-splice activations. On a clean-Go, near-top, single-byte
  insert, a 1MB file measures about 148ms on the prior release and
  about 82-88ms on this one. CSS near-top edits re-lex only 2 tokens.
  Honest note: the campaign's 60ms target for the 1MB fixture is not
  met. The residual cost is dominated by O(nodes) result
  materialization outside the splice path — the Go-compatibility
  normalization walk, EOF result-selection, and incremental arena
  zeroing. Threading the edited range through that walk is the
  tracked next lever.
- **The diagnostic compact-route materializer allocates far less per
  operation** (PR #412). Cohort processing, frontier dropping, and
  election-state tracking now reuse scratch buffers and compact
  in-place, instead of allocating maps and slices per operation.
  Measured allocations drop from 1,931-91,341 to 92-316 allocs/op
  across the fixture set, a 98.5% geomean reduction. The work graph is
  provably unchanged. This is a diagnostic/candidate-route
  improvement, not a change to the shipping default parse path.

### Docs

- Publish the sealed run6 benchmark epoch as authoritative in
  BENCH.md, superseding the v0.40.0 baseline receipt (PR #403).
- Correct README claims about `ParseIncremental`'s reuse scope, and
  document the grammargen real-corpus parity floor (PR #404).
- Add a Phase-3 admission timing runbook, with locked fixtures, host
  selection paths, and statistical thresholds (PR #406).
- Sweep remaining documentation prose to the ASD-STE100 style guide
  (PR #414).

### Known Issues

- The 1MB near-top edit still misses the campaign's 60ms target, at
  about 82-88ms (PR #411). Threading the edited range through Go's
  compatibility-normalization walk is the tracked next lever.
- GLR-heavy files with genuine ambiguity still see little wall-time
  change from the O(edit) work, because settling and block-splice run
  on a single stack only (PR #398, PR #411).
- Query-engine tranches D3 (supertype patterns) and D5 (quantified
  captures) remain silently wrong against the C oracle. Both are
  tracked outside query-engine scope, with skip-guarded witness tests
  (PR #410).
- The structural merge-policy fix for TypeScript/TSX GLR fork discard
  is still tracked, in development on
  `codex/glr-structure-before-score`. This release's arrow-return-type
  fix (PR #409) is a third source-heuristic detector, not the
  structural cure.

## [0.44.1] - 2026-07-20

### Fixed

- **Swift's certified runtime profile now attaches again.** PR #396's
  `swift.bin` regeneration (the DFA-minimization fix, v0.44.0) did not
  update the profile's pinned blob digest in
  `grammars/runtime_profiles.go`. The stale digest silently dropped
  Swift's external-scanner skip-repeat policy and its accepted-error
  retry-skip policy (PR #400). The impact was performance-only.
  Error-bearing Swift parses ran redundant retry ladders. Clean Swift
  parses, and the v0.44.0 memory win, were unaffected. This release
  updates the pinned digest to match the regenerated blob. No other
  grammar's profile carries a stale digest.

### Improved

- **Go clean-file incremental parses now reuse the top-level suffix instead
  of reparsing it.** This closes, for clean top-level edits, the Go reuse
  gap that v0.44.0 listed as a known issue. It is not an absolute
  whole-call `O(edit)` claim: length- or point-changing `Tree.Edit` calls
  still maintain coordinates through affected trailing sibling subtrees.
  Before dispatch reaches the reuse check, the parser now applies any
  pending eager-default reduce chain to the live stack.
  This is campaign O(edit) workstream W1b (PR #398), and it closes the
  settling gap that blocked the W1 splice (PR #395) for Go.
  `ReuseRejectRootNonLeafChanged` now holds at a small constant, 9,
  regardless of file size. Node allocations drop 11 to 16 times. A 137KB
  near-top insert now takes about 22ms, down from about 47ms. A 1MB file
  takes about 162ms, down from about 356ms.
- **Incremental parses over a provably clean old tree start the C-parity
  cost-competition flag false**, matching fresh-parse behavior (PR #399,
  campaign O(edit) workstream W3). Previously every incremental parse
  started this flag conservatively true, even when the old tree carried
  no errors. Outputs are proven unchanged: a differential over 8,361
  edits is byte-identical to the prior behavior. The effect on wall time
  is small today. It grows as reuse rates rise, particularly once W1b's
  Go localization compounds with it.

### Known Issues

- The 1MB near-top edit still misses the campaign's 60ms target, at
  about 162ms (PR #398). Composing the W1b settling fix with the W1
  block-splice is the tracked follow-up.
- GLR-heavy files with genuine ambiguity see little wall-time change
  from W1b (PR #398), because settling runs on a single stack only.

