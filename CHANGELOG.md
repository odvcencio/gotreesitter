# Changelog

All notable changes to this project are documented in this file.

This project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
for tags and release notes while still in `0.x`.

## [Unreleased]

## [0.22.0] - 2026-07-08

Roadmap checkpoint release after the v0.21 engine cut. This release lands the
first four campaign buckets after v0.21.0: the runtime/recovery foundation,
the external lex-state election ledger, broad precise ExternalLexStates
coverage, and the Cobol large-table/recovery cleanup. It does not claim that
all grammar tiers are parity-clean yet; the remaining tier-IV rows stay visible
and classified for the next campaign waves. It also does not claim universal
near-C parser performance across the registry yet; v0.22.0 publishes the
perf-scan scaffold and targeted wins while the broader wave-3 ratchet remains
tracked for the next release line.

### Added

- `cgo_harness/perf_scan`: a nightly Go-vs-C scoring harness and CI proposal
  for tracking full-parse and incremental parser performance without mixing
  correctness and performance gates.
- External lex-state election ledger for all 206 tracked grammars
  (`cgo_harness/tier_scan/external_lex_elections.{md,json}`), including the
  default-elected, staged, missing-ELS, and no-scanner categories used by the
  C-recovery rollout.
- Precise ExternalLexStates tables for a broad set of external-scanner
  grammars, with JavaScript and Cobol kept staged behind their explicit
  opt-in policies until their defaults are separately certified.
- Release-tier scan documentation and generation support. Tier publication is
  staged in-tree, but the zero-IV release block is intentionally not enabled
  for this checkpoint release.

### Changed

- Runtime conflict handling now centralizes C's repetition-skip dispatch rule
  behind the shared conflict-policy path, leaving only languages with
  measured recovery-shape hazards on explicit opt-outs or scoped handlers.
- External scanner symbol binding now uses positional table metadata instead
  of brittle name-only assumptions, reducing stale-table failure modes across
  generated blobs.
- C# namespace recovery and forest/retry paths avoid redundant large-span
  work, keeping the boundedness tests focused on parser behavior instead of
  retry overhead.
- Arena overflow slab growth is capped so pathological large parses fail or
  recover under an explicit memory budget rather than growing unbounded slabs.

### Fixed

- Generated parse tables no longer silently truncate action or GOTO indexes
  that exceed `uint16`. The runtime now supports large state targets, and the
  generator reports actionable boundary errors for unsupported table shapes.
- Cobol fixed-format compatibility now preserves program headers and recovered
  paragraph structure across large-table parses. Focused Docker gates measured
  Cobol at 25/25 real-corpus parity and 20/20 direct C-oracle parity for this
  release line.
- Cobol recovered paragraph-header normalization now preserves unrelated parent
  `HasError` state, so a clean recovered header no longer masks a separate
  retained error in the same `procedure_division` subtree.
- C recovery table validation now checks both action and GOTO bounds before
  accepting the recovery path, preventing large-table grammars from taking a
  silently invalid C-recovery route.
- Recovery cycle diagnostics now include non-materializing acyclicity checks
  around transient recovery children, guarding against parent-link cycles in
  debug sweeps.

### Performance

- GLR merge-equivalence and recovery-path cache work removes the largest
  repetition-boundary cliffs found after v0.21.0 while keeping correctness
  gates separate from benchmark gates.
- Bash retry overhead, C# namespace recovery retries, and TypeScript
  fourslash arena pressure are reduced by targeted fast-path and cap fixes.
- The perf harness is present for nightly scoring and follow-up CI wiring, but
  this release does not enable a universal near-C performance gate.

## [0.21.0] - 2026-07-06

The engine release. The generalized GLR parser core — a C-faithful error
recovery engine plus a GSS-forest fast path — replaces the v0.20.x
per-language recovery approximations as the default runtime. Error recovery
for 123 elected grammars now reproduces C tree-sitter's decisions
(strategy-1 election order, per-stack-version error-mode lexing identity,
error-cost model, and condense scheduling were each verified
decision-for-decision against an instrumented tree-sitter v0.25.0 oracle).
The campaign branch's full correctness backlog — 42 known failing tests at
its peak — is zero in this release, and the root test suite dropped from
~340s to ~62s along the way.

### Changed

- **Error-tree shapes for elected languages now match C tree-sitter** in many
  previously divergent constructs: PHP static named functions, the authzed
  recovery family, Angular non-null assertions, FIDL versioned layout
  modifiers, Julia trailing-comma assignment tuples, doxygen comment blocks,
  and Go range-with-function-literal bodies, among others. Downstreams that
  pinned the old (non-C) error shapes for these languages will see diffs on
  upgrade — the new shapes are the C-oracle-verified ones.
- **Go grammar: real automatic-semicolon insertion** via an external scanner
  (`_automatic_semicolon`), replacing the grammar-level approximation. Fixes
  the byte-strict ASI parity gap; spurious-`HasError` files on a large real
  repo walk dropped 436 → 17.
- The strategy-1 recovery election was rewritten to C's order and cost basis
  (merged-version m0 basis, finished-tree-first cost checks, missing-token
  versions created before the recover pass, shiftable originals preserved as
  their own paths, record-time dedup), and ordinary reduces no longer
  dissolve extra ERROR carriers (C keeps every popped subtree).
- Recovery-time lookahead identity: the engine now lexes error-mode
  lookaheads per stack version like C (`LexModes[0]` at ERROR_STATE),
  including an engine-side error-mode relex for custom token sources, with
  the capability forwarded through included-range wrappers.

### Fixed

- Conflict-policy metadata inference no longer vetoes hand-written
  repeat-boundary conflict resolvers for embedded languages. The veto —
  intended only for grammargen-generated grammars — silently disabled the
  C#, Java, C, Rust, TypeScript, PHP, Python (and more) resolvers; C#
  designer-style files forked ~32 GLR stacks per statement and exhausted the
  arena (2064 stacks); they now parse with 1 stack well under the 500ms
  test budget.
- Forest engine: hidden-symbol dedup starvation and a cap-eviction tie
  livelock no longer force dead-end declines on valid input (bash/CMake/JSON
  repeat-heavy shapes; python `module_repeat1` worklist blowup); the
  EOF-recovery competition probe no longer declines ordinary clean input;
  and `Parser.SetTimeoutMicros` is now enforced inside the forest path
  (previously unenforceable for forest-dispatched languages).
- Forest cap-eviction comparisons no longer walk full subtrees per
  comparison (O(bytes²) on repetitive inputs): raw-shape content
  fingerprints, dirty-keyed resident caches, and exact per-node error-rank
  memoization take C# designer-style n=300 from ~1.9s to ~282ms in the
  forest path.
- Root spans no longer shrink when a hidden childless leaf vanishes during
  invisible-root-child flattening (doxygen whole-block comment shapes).
- The authzed hand-written lexer emits `_whitespace` extra tokens like C's
  generated lexer instead of silently skipping horizontal whitespace, fixing
  a 1-byte recovery-anchor divergence.
- The doxygen whole-block-comment ERROR normalizer no longer collapses
  recovered structure (highlight queries produce results again); a
  recovered-structure guard scopes the collapse to genuinely empty ERROR
  trees.
- Trailing-trivia trimming is gated on symbol visibility again (a
  generalization had dropped the guard, breaking field-mapping
  preservation).
- Skipped-real-gap recovery: a stray token dropped by the lexer
  mid-production (Julia `f(a,b) = c[d]` tuple assignments) no longer
  corrupts the enclosing production or kills the stack; C's nested shape is
  restored.
- Seven grammars that regressed from parity-clean during campaign
  development (cmake, git_config, git_rebase, regex, ruby, tsx, twig) were
  re-measured: six healed by the engine fixes and are locked with
  oracle-verified regression-pin tests; regex's remaining hidden-child
  divergence is pinned with a self-healing skip and tracked.
- CI: the full `-race` suite now actually runs for non-draft PRs without
  panicking Go's default per-package timeout (`-timeout 35m -p 1`, 60m job
  budget); wall-clock boundedness contracts skip under `-race`
  (instrumentation-slowdown measurement, not parser boundedness);
  `./grammargen` runs as a non-blocking visibility step until its
  enumerated pre-existing backlog (stale markdown blob, two Dart parity
  gaps) is burned down.

### Added

- `docs/authoring-languages.md` — adding a language without forking:
  grammar.json → grammargen → blob → `Register`/`RegisterExtension`/taproot,
  the `wantsForest` opt-in, generator budgets, and blob provenance
  discipline.
- `docs/external-scanners.md` — when a grammar needs an external scanner and
  the Go porting contract (emit extras, C-EOF behavior, error-mode lexing,
  token-source responsibilities), with Pawn's five externals as the worked
  case study.
- Oracle-verified clean-regression pin tests for seven grammars
  (`grammars/clean_regression_pins_test.go`).

### Performance

- Full-parse memory (Go grammar contract benchmark): **−91% B/op, −80%
  allocs/op** vs the pre-campaign baseline.
- Incremental single-byte edit: **0 B/op, 0 allocs/op** (baseline was 176 B /
  3 allocs) at CPU parity (~1.4μs on CI hardware; ~70× faster than native C
  on the same workload). The external-scanner leaf-fastpath bailout
  introduced by ASI was replaced with a pooled verification source, and
  per-parse lexer/closure allocations were pooled away.
- Full-parse CPU is microarchitecture-dependent vs the pre-campaign
  baseline: −20% on modern desktop cores, +17% on 2-core CI-runner hardware
  (the engine parses Go via the production path instead of the retired
  forest dispatch, plus real ASI lexing at ~7-9%). The CI perf contract was
  rebased to the v0.21.0 engine; the runner-side delta is accepted and
  tracked for reclamation.
- No-edit reparse: ~7.5ns, 0 allocs (within the CI gate threshold vs
  baseline).
- Root test suite wall-clock: ~340s → ~62s (the two C# boundedness tests no
  longer burn 100-200s each before failing).

## [0.20.9] - 2026-07-02

Patch release recovering C# large-namespace method/type declarations and the
Swift ternary/conditional operator, both via post-parse source recovery
passes, plus a CI stability fix for the new C# recovery test under `-race`.

### Fixed

- Large C# files whose class body is shredded by a cumulative GLR failure (e.g.
  Newtonsoft.Json's `JsonTextReader.cs` / `JsonReader.cs`) now recover their
  `method_declaration` nodes instead of yielding only a comment-filled namespace
  shell. Follow-up to #115/#116: the source-based type/method reconstruction was
  gated off above 4096 bytes, so nothing rebuilt the members of a large collapsed
  class. Namespace recovery now falls back, when the child-based pass surfaces no
  method, to a **per-member bounded** source reconstruction — the type shell's
  header is reparsed for its modifiers/name/base list, and each member is
  recovered on its own (a method via signature-shell + lenient block, other
  members by a single small wrapped reparse), skipping any that still won't parse.
  Each reparse is a single small snippet capped by size and count and honors the
  parser timeout, so the anti-OOM guarantees from #64/#98/#106 are preserved and
  the whole-file 4096-byte gate is unchanged. `JsonTextReader.cs` now recovers 68
  methods (was 0) and `JsonReader.cs` 41 (was 0). Thanks @richardwooding (#136, #138).
- Swift ternary/conditional operator (`cond ? a : b`) now recovers instead of
  dropping `? a : b` into an `ERROR` node in every position. The runtime Swift
  blob never fired the `ternary_expression` reduction, so any function containing
  a ternary lost its whole parse (collapsing to
  `_modifierless_function_declaration_no_body`). A post-parse recovery pass
  reconstructs the `ternary_expression` — reparsing the source with each
  `? if_true : if_false` tail blanked so the condition parses in place, then
  splicing a synthesised node with the upstream `condition`/`if_true`/`if_false`
  layout. The rewrite is accepted only when the result is error-free and
  byte-faithful, so non-ternary code is never affected. Thanks @richardwooding
  (#135, #137).

### CI

- `TestCSharpLargeShreddedNamespaceRecoversMethods` now skips under `go test
  -race`: the per-member bounded recovery reparses each class member as its
  own small GLR parse, which normally finishes well inside the parser's
  timeout budget, but race-detector instrumentation slows the same work enough
  to trip the parser's internal wall-clock timeout. Non-race coverage keeps
  the full recovery assertions; mirrors the existing Scala realworld-recovery
  `-race` skip.

## [0.20.8] - 2026-07-01

Adds consumer-controllable forest parsing.

### Added

- Downstream consumers that generate a parser table with grammargen can opt
  their own grammar into the GSS-forest GLR fast path without forking, via three
  surfaces: the `Language.WantsForest` field (gob-serialized into blobs), the
  `grammargen.Grammar.WantsForest` flag, and a declarative
  `"gotreesitter": { "wantsForest": true }` object in `grammar.json` (read by
  `ImportGrammarJSON`, mirrored back by `ExportGrammarJSON` only when set, so
  standard grammars' output is unchanged). Built-in languages keep their curated,
  byte-range parity-certified forest defaults; consumer opt-in is at the
  consumer's responsibility, with the forest's decline→production fallback still
  preventing hard failures on declined inputs. `ExtendGrammar` inherits the flag
  from its base grammar (#134).

## [0.20.7] - 2026-06-29

Patch release for parser timeout propagation and targeted language recovery
scanner fixes merged after v0.20.6.

### Fixed

- Parser timeout and cancellation budgets now flow through the parser loop,
  recovery reparses, result compatibility/finalization, and Go normalization,
  so strict parses stop consistently instead of continuing unbounded work after
  the primary parse (#114, #128).
- F# external-scanner keyword dedent fallback now guards empty indentation
  stacks for `then`, `and`, `with`, `else`, `elif`, and `end`, preventing
  scanner panics on malformed or edge-case indentation (#129, #130).
- Swift `if … else if …` chains no longer collapse the enclosing function to
  `_modifierless_function_declaration_no_body` (#131). The trailing-closure
  ambiguity recovery now follows the whole if/else-if chain — the chained `if`
  keyword is swallowed into an ERROR node, so it is discovered by scanning from
  the body's matching close brace — and requires a byte-faithful reparse so a
  partially-bracketed chain (which silently truncates without an ERROR node) is
  rejected rather than accepted (#132).

## [0.20.6] - 2026-06-28

Patch release for parser recovery correctness, grammargen parity, and the
forest/performance workstream merged after v0.20.5.

### Added

- Strict parse variants return `ErrParseStoppedEarly` for timeout,
  cancellation, token-source EOF, and parser safety-limit partial trees while
  preserving the returned tree for diagnostics.
- `NodeAtByte` and `NamedNodeAtByte` helpers on `Tree` and `Node` for editor
  offset lookup without hand-written tree walks.
- One-pass code-understanding helpers for common definition spans, call
  references, heritage edges, and enclosing-definition lookup.
- Benchmarks comparing the one-pass code-understanding helpers against the
  tags-query path for both parse-plus-inspect and already-parsed trees.
- `grammars.LoadLanguage(name, blob)` attaches registered external scanners and
  external lex-state tables when loading raw grammar blobs.
- `Language.Size()` reports approximate decoded table and lookup-cache bytes for
  diagnostics and cache policy decisions.

### Fixed

- JavaScript, TypeScript, and TSX automatic-semicolon scanning now preserves
  standalone block statements before simple assignments such as `{a}b=c`,
  matching the C parser on minified bundle shapes (#111).
- Large Go files with wide table-driven literals now have an opt-in Cobra
  regression gate so release validation catches parser stack overflows like the
  `ParserPool.Parse` crash reported against `command_test.go` and
  `completions_test.go` (#110).
- Recovered result trees now strip self-references and ancestor back-edges
  before parent-link wiring, while keeping `children`, `fieldIDs`, and
  `fieldSources` aligned when a cyclic edge is removed (#121).
- Go and Go module recovered parses keep the grammar `source_file` root when
  child nodes contain parse errors, matching the root-shape behavior already
  used for SQL and Swift (#112).
- `grammargen` now treats explicit precedence wrappers around finite string
  choices, such as `prec.right(choice("=", "+=", "-="))`, as reducible
  nonterminals instead of overlapping named lexer tokens. This restores wrapper
  nodes and lets LR precedence resolve the intended conflict (#122).
- Swift functions that iterate a `for…in` loop over a range (`0..<n`, `0...n`)
  or a call expression (`stride(from:to:by:)`) no longer silently collapse to
  `_modifierless_function_declaration_no_body` with the loop body spilled out as
  file-level siblings. As with the `if`/`while` case, the loop body brace was
  being consumed as a trailing closure of the iterable; recovery now re-parses
  the affected `for…in` headers with synthetic parentheses around the iterable
  and maps the result back to byte-faithful original coordinates. Because this
  misparse produced no `ERROR` node, the recovery pass now runs whenever the
  detection walk finds a collapsed header rather than only on errored trees
  (#123).

### Testing

- Added `TestGoCobraLargeFileParseRegression`, gated by
  `GTS_COBRA_REGRESSION_ROOT`, for exact large-file release validation without
  making normal test runs network- or corpus-dependent.

## [0.20.5] - 2026-06-24

### Changed

- `grammargen` no longer imports the `grammars` registry. The `grammar.js`
  importer (`ImportGrammarJS`) previously pulled in `grammars` for the embedded
  JavaScript language, which transitively bundled all ~200 grammar blobs
  (~22MB) into *every* consumer that merely defined a grammar via the DSL —
  including `taproot` and all downstream DSLs. The JS language is now injected
  via `SetJSGrammarProvider`; blank-import `grammargen/grammarjs` (or `cmd`s
  that need `-js`) to register it. Net effect: `grammargen`, `taproot`, and
  anything that only defines/loads a grammar are now grammar-registry-free.

## [0.20.4] - 2026-06-24

### Added

- `taproot/walk`: a grammar-free core of the `taproot` harness. It loads a
  tree-sitter `Language` from a pre-generated blob (`LanguageFromBlob`) and
  navigates the CST (`Walker`, `ParseFromBlob`, `ParseWithLanguage`) depending
  only on the gotreesitter runtime — not `grammargen` or the `grammars`
  registry. DSLs that embed a generated grammar blob can now parse/highlight
  without linking the ~200-grammar registry (~22 MB). The grammargen-backed
  build-from-DSL fallbacks remain in `taproot`, which re-exports `walk.Walker`
  so existing `taproot.Walker`/`Parse` callers are unaffected.

## [0.20.3] - 2026-06-23

### Fixed

- C# files whose namespace body does not parse cleanly no longer collapse into
  a single top-level `ERROR` node with zero recoverable declarations. Namespace
  recovery now falls back to a best-effort `namespace_declaration` built from
  the existing sub-parse, surfacing the type declarations (and the members that
  parsed) instead of discarding the whole file. C# brace matching used during
  recovery is now trivia-aware, so braces inside char literals, strings and
  comments no longer truncate a recovered declaration's span (#115).
- Swift functions whose `if`/`while` condition contains a comparison operator
  (`<` / `>` / `==`, etc.) no longer collapse into an `ERROR` tree with no
  recoverable `function_declaration`. The body brace was being consumed as a
  trailing closure of the condition's last operand; recovery now re-parses the
  affected conditions with synthetic parentheses to remove the ambiguity and
  maps the result back to byte-faithful original coordinates (#118).
- Go: `normalizeGoDotLeafChildren` now walks dotted-selector chains with an
  iterative DFS instead of recursion, removing a stack-depth risk on very long
  selector chains.

### Changed

- Removed dead unexported code (#117).

### Testing

- Banked a (skipped) regression guard,
  `TestJavaScriptBlockThenAssignmentParsesClean`, for the JavaScript
  block-then-simple-assignment GLR collapse (`{a}b=c`, #111). The root cause is
  the JSX-attribute-continuation ASI heuristic in the JS scanner; the fix is
  still pending (targeted for the C-oracle-verified parity line). Remove the
  `t.Skip` to validate once fixed.

## [0.20.2] - 2026-06-06

Patch release for the post-0.20 parser reliability and code-understanding
surface fixes. This tagged release references the issue fixes merged after
v0.20.1.

### Fixed

- C# namespace recovery now routes recursive recovery snippets through the
  guarded recovery parser path, preventing the v0.20.0-rc3 namespace OOM case
  and propagating timeout/cancellation guardrails into recovery parses
  (#98, #106).
- Swift license-header and top-level declaration recovery now preserves
  `import Foundation` followed by declarations, covering the real-world Swift
  misparse report while avoiding recursive recovery (#99, #107).
- Inferred Go tags no longer capture return type identifiers such as `int` or
  `error` as `definition.function` tags (#100, #109).
- JavaScript/TypeScript optional-chain, TypeScript dynamic-import, and Python
  `case _:`/block-start normalization now match the C tree-sitter shapes used
  by the parity harness (#101, #102, #103, #108).

### Testing

- Added a small multi-language structural corpus parity gate for Go, Java,
  JavaScript, Python, and TypeScript (#104).
- Validated the patch train with focused Docker parity/unit gates plus green
  CI build, freshness, cgo parity smoke, and perf-regression checks on the
  merged fix PRs.

## [0.20.1] - 2026-06-06

Taproot stable release.

### Added

- Taproot blob-loading helpers and stable parser harness coverage for the
  extracted Taproot DSL surface.

## [0.20.0] - 2026-06-03

GLR parser-core release after the 0.20 release-candidate line.

### Fixed

- Fixed an infinite spin on repeated zero-width external tokens in
  `markdown_inline`.

## [0.20.0-rc4] - 2026-06-03

Fourth 0.20 release candidate.

### Added

- Extracted Taproot as a reusable DSL parsing harness with diagnostics.

## [0.20.0-rc3] - 2026-05-30

Third release candidate on the 0.20 line. Parser-core GLR performance wins —
C now parses at/below parity with tree-sitter C on real corpus — plus parser
correctness fixes and markdown grammargen advances. CI green (build,
parity-cgo, perf-regression).

### Performance

- **GLR fork reduction across the ring matrix** (#96). Extended the
  `RepetitionShiftConflictChoice` resolver to collapse spurious reduce/shift
  forks at boundaries where tree-sitter C resolves deterministically (verified
  per state against C's `parser.c`; the reduce is a zero-progress dead-end with
  no `conflicts:` entanglement). Every change is byte-for-byte C-parity-verified
  against libtree-sitter.
  - C: `translation_unit_repeat1` (top-level item list) + `preproc_if_repeat1`
    (preprocessor body) collapse — `large__cluster.c` drops from 20,866 to
    1,099 GLR forks (−95%) and ~−30% parse wall, bringing C to/below parity.
  - Rust: macro token-tree (`delim_token_tree_repeat1`) continuation-token
    fork reduction.
  - Token source: O(1) valid-external-symbol fast path mirroring C's
    `external_lex_state` indexing (single active state references the
    precomputed row instead of rebuilding it per token).
  - Consolidated ring A/B (real corpus): geomean −6.62%; C −30%, java −11%,
    go −10%; no language regressed.

### Fixed

- Race in deferred parent-link wiring (#95).
- Kotlin object declaration misparse (#94).
- grammargen: terminate `html_block` type 6/7 at a blank line.
- grammargen: de-merge `link_reference_definition` soft-break terminator.

### Added

- grammargen: self-contained CommonMark §3–§6 markdown parity corpus.

## [0.19.1] - 2026-05-23

Kotlin source-file query compatibility patch.

### Fixed
- Kotlin recovered parse roots that contain top-level package/import/class
  nodes are now returned as `source_file` roots instead of `ERROR` roots, while
  preserving child error state for diagnostics.
- Fragmented top-level Kotlin `fun` declarations recovered under an error root
  now expose `function_declaration` shape with the `fun` keyword, name, and
  parameter list, while retaining error state. This preserves downstream
  Aspect/Gazelle Orion queries without AXL rewrites on files with recoverable
  syntax errors.

## [0.19.0] - 2026-05-23

GLR materialization, query parity, and parser hot-path release.

### Added
- Reduce-chain hint metadata is now attached to embedded grammars and used by
  the parser to classify hot reduction chains without re-deriving them at run
  time. Rust carries a targeted reduce-chain hint in this release.
- Parser and harness attribution now expose reduce-chain timing, action
  dispatch/apply/lookup timing, GLR merge and cull timing, result-tree
  materialization timing, lazy-child materialization cost, and GLR equivalence
  hot-spot counters.
- Real-corpus benchmark and parse-gap reporting tools now surface top parse
  gaps, parser phase timing, result attribution, symbol names, active
  reduce-chain hints, and compiled-test-binary RSS lanes for CI.

### Changed
- Final tree construction now keeps compact/lazy final child references deeper
  into parser result assembly, tree traversal, cursor movement, query matching,
  descendant lookup, sibling scans, and edit paths. Public nodes are
  materialized on demand instead of eagerly for every reduction result.
- Parser hot paths cache action classes, lex-mode rows, visible symbol lookups,
  JS/TS normalization traits, reduce-chain signatures, and language metadata
  needed for full parses.
- Full-parse scratch allocation is capped and tuned for medium sources so large
  files do not preallocate excessive entry storage.
- JavaScript, TypeScript, TSX, Python, Rust, Go, C, and Java compatibility
  normalization now routes more work through dense/lazy child accessors and
  source-gated fast paths.
- The default reduce-chain hint path is enabled while the explicit reduce-chain
  experiment knob was removed.

### Fixed
- Query matching now preserves tree-sitter-compatible behavior for nested
  repeated children, namedness-sensitive candidates, field matching through
  parent links, and lazy final child refs. This fixes downstream public queries
  such as nested Kotlin `source_file -> import_list -> import_header` patterns
  without requiring query rewrites.
- `#lua-match?` predicate parity and query predicate stack storage now match
  expected tree-sitter semantics more closely.
- GLR materialization preserves pending direct fields, hidden-child field
  metadata, compact leaf parents, lazy final child refs through edits, and
  sidecar child counts across tree operations.
- Incremental reuse handles lazy child refs, no-op edits, top-level reuse, and
  external-scanner checkpoint rebuilds without forcing broad materialization.
- Parser compatibility repairs restore or preserve tree shapes for Go,
  JavaScript/TypeScript optional chains, Python collapsed keyword leaves, Rust
  recovery/doc comments/repetition conflicts, C parity recovery, Java unary
  wrappers and annotation parses, and TypeScript repetition conflicts.
- Large GLR merge caps, terminal-node stack equivalence, zero-width sidecar
  traversal, and C merge survivor caps now fail boundedly instead of corrupting
  branch selection or retaining excessive alternatives.

### Removed
- Removed legacy generated grammar register stubs that were hidden behind the
  obsolete `legacy_generated_register_stubs` build tag; the generated registry
  is now the single checked-in grammar registration surface.
- Folded the one-off Go blob regeneration command into `cmd/grammargen` via
  `-lr-split`, reducing the public command set while keeping the regeneration
  path available.
- Removed the legacy host-side race wrapper in favor of CI or Docker-scoped
  race validation.
- Removed the obsolete scoped Canopy Docker runner; Canopy now runs directly on
  host for structural analysis.
- Removed the undocumented `grammarlsp` side package and its LSP/SegmentIO
  dependencies from the root module.
- Collapsed stale internal aliases/helpers around token-source reparsing,
  snippet parsing, COBOL dispatch, and perf counter structs.

### Performance
- Standard Go/editor benchmark median on the release cut:
  full DFA parse `~1.54 ms`, incremental single-byte edit `~649 ns`, no-edit
  incremental reparse `~2.43 ns`. Full parse now reports `728 B/op` and
  `7 allocs/op` on this workload.
- Lazy final-child refs and deferred parent-link materialization reduce full
  parse result-tree construction work while preserving query, cursor, edit, and
  traversal behavior.
- GLR stack equivalence checks short-circuit earlier, cache more relevant
  frontier state, and expose true-share metrics for remaining ambiguity hot
  spots.

### Testing
- Focused release benchmark command:
  `GOMAXPROCS=1 go test . -run '^$' -bench 'BenchmarkGoParseFullDFA|BenchmarkGoParseIncrementalSingleByteEditDFA|BenchmarkGoParseIncrementalNoEditDFA' -benchmem -count=10 -benchtime=750ms`.
- Query parity was checked against the original nested Kotlin queries used by
  downstream Aspect/Gazelle Orion plugins.
- CI and harness work now prefer compiled test binaries for RSS benchmarks and
  keep heavy parity/perf runs language-scoped.

## [0.18.0] - 2026-05-19

Cold dependency extraction and parser materialization diagnostics release.

### Added
- Language-neutral import extraction APIs:
  `ExtractImports`, `ExtractImportsFromSource`, and
  `ExtractImportsFromSourceWithReport`. The source extractor reports status,
  reason, and fallback recommendation so callers can use a fast source scan and
  fall back to a full tree parse only when needed.
- Source-vs-tree import parity fixtures and Docker corpus gates for Go, Java,
  Python, and optional Starlark corpora.
- `cgo_harness/cmd/import_replay`, a Bazel/Gazelle-shaped replay command that
  scans repositories and compares cgo tree extraction, Go tree extraction, and
  hybrid source extraction with normalized dependency-output diffs and timing
  JSON.
- Python corpus parsing and materialization benchmarks, including no-tree,
  no-tree-plus-checkpoints, full no-compat, full compat, arena, checkpoint,
  GSS, transient reduction, final materialization, and normalization counters.
- Parser runtime attribution for constructed/final node volume, arena usage,
  checkpoint storage, reduction/transient storage, final tree materialization,
  normalization timing, and GLR collapse behavior.
- Experimental GLR v2 scaffolding for compact full leaves and pending parents,
  kept diagnostic/controlled while materialization coverage is broadened.

### Changed
- Python full parses now use sparse external scanner checkpoint storage,
  scanner snapshot reuse, capped large-file arena headroom, transient
  reduction-child storage, deferred parent links, and source-gated
  compatibility normalization.
- No-tree benchmark paths carry compact no-tree payloads and skip public tree
  construction/checkpoint work when the benchmark mode does not need it.
- Python compatibility repair now skips clean subtrees and avoids running
  f-string, keyword, and punctuation normalization passes when source flags
  prove they cannot apply.
- Import extraction for Python handles preamble comments, `__future__` imports,
  multiline imports, relative imports, and triple-quoted strings consistently
  across source and tree extractors.
- Diagnostic tests and Canopy scratch output are kept out of normal repository
  noise; `.canopy/` is ignored and stale tracked scratch programs were removed.

### Fixed
- Python token precedence now preserves longer prefix literals such as `**`
  instead of letting shared shorter literals split them during generated-parser
  lexing.
- Python external scanner lex-state registration is restored for generated
  grammar parity and corpus parsing.
- Java annotation declarations and zero-version GLR cache invalidation no
  longer corrupt branch selection.
- Transient parent reuse in result assembly now preserves child ownership and
  avoids unsafe reuse across incompatible reduction paths.
- Go source dependency replay now exposes full-tree completeness failures that
  would otherwise hide imports in cold dependency scans.

### Performance
- On a `rules_python` replay with 564 Python files and 949 imports, hybrid
  source extraction matched cgo tree extraction and Go tree extraction exactly:
  cgo parse+extract was about `158 ms`, Go tree extraction about `1.17 s`, and
  source extraction about `3.9 ms`.
- On a `rules_jvm` replay with 231 Java files and 1028 imports, hybrid source
  extraction matched cgo and Go tree extraction exactly: cgo parse+extract was
  about `70 ms`, Go tree extraction about `276 ms`, and source extraction about
  `1 ms`.
- On an `aspect-gazelle` Go replay with 148 files and 916 refs, hybrid source
  extraction matched cgo tree extraction exactly in about `0.7 ms`; Go full-tree
  extraction missed 8 refs because three full-tree parses were incomplete.
- Sparse checkpoint storage and snapshot reuse substantially reduce Python
  checkpoint bytes/token on stress and PyMuPDF lanes while preserving corpus and
  grammargen-vs-cgo parity.

### Testing
- Focused import extraction tests cover Go, Java, Python, Starlark, source
  fallback reporting, Python preamble comments, docstrings, and future imports.
- Docker import replay smoke passed under `4 GB` memory and `1 CPU` with
  `oom_killed=false`.
- Python real corpus and grammargen-vs-cgo parity remain the gate for parser
  correctness work; heavy parity and performance lanes stay Docker-scoped and
  language-scoped.

## [0.17.4] - 2026-05-17

Python parser compatibility patch for downstream Gazelle consumers.

### Fixed
- Python `_` identifiers no longer get retokenized as EOF during contextual
  literal repair. This prevents clean prefix parses from silently truncating
  modules before later assignments or nested imports.

## [0.17.3] - 2026-05-17

Module compatibility patch for downstream Gazelle consumers.

### Fixed
- Lower the module `go` directive from `1.25.0` to `1.22.0` and pin
  `golang.org/x/sync` to `v0.11.0`, avoiding accidental Go 1.25 upgrades for
  downstream consumers that import gotreesitter only as a parser runtime.

## [0.17.2] - 2026-05-17

Query predicate compatibility patch for downstream Gazelle consumers.

### Fixed
- Query compilation and evaluation now support `#has-parent?` with immediate
  parent semantics, matching the existing `#not-has-parent?` support.

## [0.17.1] - 2026-05-17

Kotlin parser compatibility patch for downstream Gazelle consumers.

### Fixed
- Kotlin dotted package and import headers parse without errors after the
  grammar refresh, including wildcard imports and files that combine package,
  import, and `fun interface` declarations.
- Kotlin import-only external tokens can relex through shared package/import
  parser states when the winning branch needs ordinary DFA tokens such as `.`
  or `import`.

## [0.17.0] - 2026-05-17

Java corpus parity and parser-performance release.

### Added
- Java corpus Docker harnesses for seeded Apache Lucene stress testing,
  including largest/random corpus selection, timeout sweeps, cgo comparison
  benchmarks, UAX generated-file stress runs, materialization profiles, runtime
  diagnostics, and ambiguity profiling.
- `Parser.ParseNoTreeBenchmarkOnly` for diagnostic parser-loop benchmarks that
  suppress full public tree materialization while keeping lexing and parse
  actions active.
- Language-family full-parse benchmark matrix controls, warm parser reuse
  benchmarks, and parser scratch/reset regression coverage.
- Top-50 `grammargen` parity coverage checks and focused fixtures for Java,
  Bash, Python, Swift, comment, CPON, git config, gomod, ini, and related
  imported-grammar edge cases.

### Changed
- Java parsing now handles contextual keyword/token selection, compact generic
  close-angle splitting, switch rule labels versus lambdas, shift expressions
  before calls, array initializer commas, repetition shifts, and downstream
  recovery cases much closer to the C runtime.
- Parser hot paths cache language traits on DFA token sources, preserve scratch
  buffers across pooled token-source resets, clear GLR/GSS scratch by written
  range and epoch, and reduce parser clearing/lookup overhead.
- Initial Java full parses defer parent-link wiring until the tree API needs it,
  avoiding public tree bookkeeping during the parse-time materialization hot
  path.
- Edited trees now reuse the old primary arena directly where possible, and
  borrowed arenas are deduplicated to reduce incremental parse retention churn.
- HTML-family scanner deserialization reuses tag snapshots and shared ASCII
  lookup construction while preserving first-match behavior.

### Fixed
- Bash generated-parser parity issues around command names, statement
  boundaries, broad DFA relexing, and arithmetic expansion token normalization.
- Comment tag parsing, parser compatibility normalization, parser-valid
  zero-width token preference, broad relex candidate matching, and string
  whitespace recovery behavior.
- `grammargen` normalization and conflict-resolution gaps for lexical choices,
  aliased inline precedence, long Unicode escapes, augmented start symbols,
  terminal collisions, Python/Swift parity regressions, Julia assignment
  conflicts, D binary repeat, PowerShell binary repeat, and gomod grouped
  retract intervals.
- Parser reset paths now avoid stale node-equivalence and GSS cache hits after
  reuse.

### Performance
- Main-branch Go/editor benchmark median on the standard generated Go workload:
  full DFA parse `~1.98 ms`, incremental single-byte edit `~666 ns`, no-edit
  incremental reparse `~2.84 ns`, with full parse at `5 allocs/op`.
- Java Lucene largest top-10 Docker benchmark: Go full DFA `~537 ms`, Go
  no-tree diagnostic `~402 ms`, cgo full `~394 ms`; full/cgo is about `1.36x`.
- Java generated UAX file Docker benchmark: Go full DFA `~306 ms`, Go no-tree
  diagnostic `~235 ms`, cgo full `~213 ms`; full/cgo is about `1.44x`.

### Testing
- CI for the release commit includes green build, freshness, cgo parity smoke,
  and perf-regression gates on PR #80.
- Java real-corpus parity and large-file timeout diagnostics are now
  reproducible through bounded Docker lanes rather than ad-hoc local runs.

## [0.16.0] - 2026-05-06

Grammar extensibility, UTF-16 input, and parser-resilience release.

### Added
- Native UTF-16 parser/editor APIs, including UTF-16 byte parsing, token source
  factories, incremental parser variants, edit mapping, injection reuse, and
  descendant range lookup helpers for editor integrations.
- `grammargen` DSL sources and extension smoke coverage for Kotlin and Swift.
- `grammargen` constructors for JavaScript, TypeScript, TSX, and Fortran, plus
  imported grammar coverage and TypeScript inline-rule filtering documentation.
- Grammar update guard tooling for scanner-facing grammar refreshes so
  automation can distinguish safe lock updates from changes that require
  scanner-port work and focused parity validation.

### Changed
- Parser-result compatibility shims now route through an explicit strut
  registry, with language-owned helper files and shared normalization helpers
  split out of mixed parser-result modules.
- The cgo harness now runs on Go 1.25.

### Fixed
- External scanner fallback binding now assigns unmatched tokens to the next
  available external symbol instead of relying on positional token indexes when
  name-based binding partially succeeds.
- Python f-string scanner checkpoints now recompute interpolated-string state
  from the delimiter stack after deserialize, preserving `DEDENT` behavior for
  issue #53. Includes Fraser Isbester's fork commit and a follow-up regression
  hardening pass.
- C# pathological recovery is bounded, full-parse retries stop after timeout,
  parser arena budgets are repaired, and C# repetition shift conflicts are
  handled without unbounded GLR growth.
- TypeScript parity gaps and GLR merge scratch handling were corrected.
- Fortran `grammargen` parity gaps were closed.

### Testing
- Added focused Swift and Kotlin `ExtendGrammar` smoke tests.
- Added and hardened imported grammar coverage for the new JavaScript,
  TypeScript, TSX, and Fortran `grammargen` constructors.
- Kept C#, Fortran, TypeScript, Swift, and Kotlin parity work scoped through
  Docker grammar-focused lanes and scanner update gating.

## [0.15.3] - 2026-04-26

Parser stability and harness release.

### Added
- C/C++ lexer bridge now accepts `#embed` directive lines and
  `__has_embed(...)` conditional feature-test forms (including parameter
  variants) without parse errors.
- Scoped Canopy harness runner under `cgo_harness/docker/`. The wrapper mounts
  the host Canopy binary into the Docker harness, applies memory/CPU/PID caps,
  uses a host-side timeout watchdog, and scopes analysis to one package with
  generated blobs/worktrees excluded by default.

### Changed
- `ts2go` batch execution is parallelized, reducing generated-grammar
  conversion wall time on multi-core machines.
- External scanner adaptation now tolerates source/target external-symbol count
  mismatches. `AdaptExternalScannerByExternalOrder` can match shared symbols by
  name, leave unpaired target symbols disabled, and size the source-valid bitmap
  to the source scanner rather than assuming equal external lists.
- Moved cgo harness sample/profile fixtures under `testdata` directories and
  updated the harness docs and scripts to use the new paths.
- GLR stack culling now shares the keyed retention path across full and
  incremental parses while preserving the previous incremental tie-breaks.
- Parser-result compatibility dispatch is now separated from core tree
  assembly, with mixed compatibility shims split into language-owned files and
  shared node helpers moved out of language-specific modules.
- Parser tests are split by responsibility, public parser-result regression
  tests live under `parser_result_test`, and larger parser-result Python source
  fixtures now live under `testdata/parser_result`.

### Removed
- Dropped unused query matcher rollback compatibility wrappers now that
  predicate-aware matching is the only call path.
- Removed unused internal parser, reduce, incremental, and parser-result helpers
  left behind by recent recovery and normalization rewrites.
- Removed stale internal planning/spec docs from the OSS tree.
- Removed unused private grammar and grammargen helper code found by the
  maintenance sweep.
- Moved ad-hoc grammargen diagnostic tests behind an explicit build tag and
  removed the print-only disassembly lexer probe from the normal test suite.
- Removed the duplicate legacy GLR stack-retention selector from parser
  internals.

### Fixed
- Re-landed the arena-retention and repo-cleanup fixes from the recovered
  main-line commits after the accidental reset.

### Performance
- JavaScript and TypeScript full parses cap merge survivors per key at 4. Large
  JS bundles can otherwise keep too many near-equivalent GLR branches alive and
  spend most parse time in merge-equivalence checks. Incremental parsing and TSX
  keep their existing budgets.
- Markdown and markdown_inline full parses use tighter initial GLR stacks and a
  higher markdown-specific node budget. Dense inline-heavy markdown now prunes
  early without forcing repeated node-limit retries on normal documents.

## [0.15.2] - 2026-04-21

Reconciliation release. The `release/v0.15.x` line and `main` had drifted apart; v0.15.2 unifies them so subsequent work has a single forward branch to build on.

### Added
- **Swift ABI mangling grammar** (`grammargen/swift_abi_grammar.go`, `SwiftABIManglingGrammar()`). Parses the `$s` / `$S` / `$e` / `_T0` Swift symbol-mangling prefixes. Intended for tooling that needs to walk demangled Swift symbols without invoking the Swift toolchain.
- **`cmd/grammar_updater -verify-pins`** flag. Validates that every locked commit in `grammars/languages.manifest` is still fetchable from its declared remote before any sync runs. `verifyRemotePins` / `verifyRemoteCommit` deduplicate by `repo+commit` to keep the check cheap on large manifests.
- **`cmd/grammar_updater -sync-manifest-only`** flag. Limits a sync pass to manifest entries that are new since the last run. `syncMissingEntriesFromManifest` now returns a map so callers can apply an allow-list filter.

### Changed
- **Plan-doc directories are now gitignored** (`.claude/`, `docs/blog-outlines/`, `docs/plans/`, `docs/superpowers/`) along with the `benchgate` binary. Plan docs are working references and should not ship with the repo.

### Removed
- Four stray plan-doc files that had been committed under `docs/plans/` and `docs/superpowers/` prior to the gitignore update.

## [0.15.1] - 2026-04-18

### Fixed
- Query matching now backtracks when structurally valid child candidates fail predicates, fixing Starlark nested-dictionary predicate cases.
- Full arena reset now clears full node backing arrays so stale node pointers cannot keep released tree memory live after GC.
- Retry parsing now releases the original tree when a retry result wins, returning the losing arena promptly instead of waiting for GC/finalization.

### Performance
- The GLR node-equivalence cache hardening is now on the main release line, including the smaller L2-friendly cache and depth-key guard.

## [0.15.0] - 2026-04-17

### Added
- `ParsePolicy.ShouldSkipDir` lets gateway consumers prune a directory before descending into it. This is intended for large generated/vendor trees where even file discovery and language detection can create avoidable memory pressure.

### Changed
- Parser-result compatibility normalization now keeps language-specific dispatch sequences in the `parser_result_*.go` files instead of centralizing every per-language call chain in `parser_result.go`.
- Tier-1 grammar pins and blobs refreshed after the v0.14.0 release line, including Kotlin, Rust, Dart, Elixir, Erlang, OCaml, PHP, Ruby, and Swift follow-ups while keeping the Scala lock pin on the known-good ref.
- Grammargen real-corpus parity floor data now includes four additional grammars from the current focus board.

### Fixed
- `ImportGrammarJSON` now drops reserved-word sets when the imported grammar does not expose a `RESERVED` wrapper, avoiding stale reserved metadata on grammars that should not carry it.
- Rust scanner support now ports `string_close` external-token handling for the refreshed lock pin.
- Scala LexModes fixtures now compare tail-relative layout after reverting the problematic lock pin.

### Performance
- GLR node-equivalence cache now fits more comfortably in L2 by reducing the cache size and checking the epoch before touching the rest of a cache slot.
- `Tree.Edit` stops scanning already-sorted right-side siblings when an edit has no tail shift to apply.

## [0.14.0] - 2026-04-17

### Changed
- **Go grammar now ships as a grammargen-compiled blob** (PR #35). Our pure-Go LALR(1) + LR(1) state-splitting compiler produces a different state layout that sidesteps a dead-end in tree-sitter-go's C tables where `}` had no action after certain nested switch/case/if patterns. gotreesitter's own `parser_reduce.go`, `parser.go`, and `parser_test.go` now parse cleanly (`HasError=false`); the old blob wrapped them in ERROR. Adds `cmd/emit_grammargen_go_blob` for one-shot regeneration as grammargen evolves.
- **Go initial GLR stack cap raised from 2 to 32** (PR #36). The previous cap=2 default was introduced for the ts2go Go blob to avoid exponential blowup on large files, relying on the retry-with-widening cycle for edge cases. grammargen's Go blob has a different conflict profile where the blowup no longer applies, but cap=2 was triggering a guaranteed two-retry cycle on every non-trivial Go file. Retry invocations across the self-parse benchmark: 8 → 0.
- **Custom `GoTokenSource` no longer registered by default** (PR #35). The grammargen blob ships DFA tables that parse Go on their own; `GoTokenSource` remains available via the public API for callers carrying their own ts2go Go blob.
- **Zig grammar migrated** from `maxxnino/tree-sitter-zig` (inactive since 2024-10) to `tree-sitter-grammars/tree-sitter-zig` (active upstream, PR #32, addresses #31). Wholesale PascalCase → snake_case node-name rename; 28 % smaller blob (62 948 → 45 316 bytes). Three upstream `#lua-match?` highlight predicates rewritten as `#match?` for portability. Review-follow-up commit addresses four gemini-flagged issues: anchored type regex, `...` moved to `@operator`, broken `.` anchors on `field_expression` patterns removed, duplicated `&`/`-%` operators deduped.
- **Arena initial-sizing heuristic** `sourceLen × 4` → `sourceLen / 4` (PR #33). The old formula over-allocated 10-16× for Go (~1 node per 5-10 input bytes); the adaptive hint handles subsequent parses.
- **Arena retention ceiling preserved across resets** instead of trimmed back to the default slab size (PR #33). Warm-reuse workloads keep adaptive capacity across parses and stop re-reallocating the primary slab.
- **Retry path releases losing candidate-tree arenas eagerly** (PR #34). Previously arenas only returned to the pool at GC-finalize time, starving subsequent retries in the same warm loop of reusable capacity.
- **Tier-1 grammar lock SHAs refreshed** (PR #26). 10 tier-1 grammars bumped to current upstream tips: dart, elixir, erlang, kotlin, ocaml, php, ruby, rust, scala, swift. Lock-only change; blob regeneration is a separate workflow.

### Fixed
- **Parser pool aliasing on recovery token sources** (PR #30 by @rasmus-theca). Recovery reparsing was acquiring a pooled `dfaTokenSource` while the outer parse still held one, causing a use-after-return when the outer parse finished first. Adds `newDFATokenSourceDirect` with `noPool: true` so recovery nests safely inside an active parse, and extracts an `initDFATokenSource` helper.

### Added
- `DrainArenaPools()` + `releaseNodeRefs` on `reuseCursor`/`reuseScratch` (PR #25 by @vdergachev). Arenas held in the pool are strong Go references and are not collected by the GC until explicitly drained; call after a large batch scan to allow reclamation.
- `BenchmarkSelfParse` and `BenchmarkSelfParseWarmReuse` — regression-guard benchmarks that parse gotreesitter's own pathological root files. Intended to catch memory-footprint regressions on dense real-world Go source.

### Removed
- Dead GLR helper functions (PR #29 by @Lars-L): `recomputeByteOffset`, `stackEntriesEqual*`, `gssStackEntriesEqual*`, `stackEntryNodesEquivalent*Frontier`.

### Performance
Stacked effect across PR #25 + #33 + #34 + #35 + #36 on `BenchmarkSelfParseWarmReuse` (six gotreesitter root files, 5-iter warm bench, Docker 4 g / 4 cpus):

| mode | pre-0.14.0 | 0.14.0 | delta |
|---|---:|---:|---:|
| cold (fresh Parser per iter) | 574 MB/op | 225 MB/op | **-60.8 %** |
| warm (one Parser reused) | 498 MB/op | 229 MB/op | **-54.0 %** |
| warm + GC drain between rounds | 522 MB/op | 252 MB/op | **-51.7 %** |

Warm-reuse throughput ~10 % higher. 206-grammar parity green under `GTS_PARITY_MODE=exhaustive`.

## [0.13.4] - 2026-04-05

### Fixed
- **Injection parser arena leak** (PR #24 by @vdergachev): `InjectionParser.Parse` and `ParseIncremental` never released previous parse trees, causing the arena pool to allocate new arenas instead of reusing freed ones (~3 MB per parse of a 180-byte HTML+JS document). Fixed by tracking the previous result and releasing it before the next parse. Also fixes a use-after-free in `ParseIncremental` when the caller passes back the previous `Parse` result as `oldResult`.

### Added
- Injection parser benchmarks: `BenchmarkInjectionParser_Parse`, `BenchmarkInjectionParser_ParseIncremental`, `BenchmarkInjectionParser_ParseReuse`, and arena-reuse regression tests.

## [0.13.3] - 2026-04-04

### Added
- `BlobByName` API for serving grammar blobs over HTTP.
- Fortran-style word rules for keyword capture in grammargen.
- New benchmarks: `BenchmarkParserPoolSerial`, `BenchmarkParserPoolConcurrentThroughput`, `BenchmarkDetectLanguage`, `BenchmarkLoadLanguage`, and more.

### Changed
- **GLR large-file performance**: parsing a 147KB protobuf-generated Go file drops from 4+ minutes to ~420ms (PR #22 by @vdergachev). Removes redundant node zeroing in the arena allocator, optimizes the GLR equivalence cache (4x larger, improved hash distribution, cheap field checks before cache lookup), splits GSS node allocation into a hot-path/slow-path pair, and sets `maxGLRStacks=2` for Go to prevent exponential stack blowup.
- **Allocation elimination across query, walk, detection, and lexer** (PR #21 by @rsnodgrass): O(1) extension index with `sync.RWMutex` for thread-safe `DetectLanguage`, `sync.Pool`-backed `Walk` stack, highlight buffer reuse, gzip ISIZE pre-sizing for `LoadLanguage`, and TypeScript scanner scratch reuse.

### Fixed
- **Incremental parsing after deletions** (issue #23): `HighlightIncremental` returned fewer ranges than `Highlight` after sequential single-character deletions. The incremental reuse cursor offered leaf nodes from under dirty ancestors with stale parser-state metadata (byte positions were shifted by the edit but parser states were not updated). Fixed by requiring byte-content equality between old and new source for all candidate nodes under dirty ancestors.
- Benchgate now applies a minimum absolute ns floor to prevent CI noise false positives on sub-nanosecond benchmarks.

## [0.13.0] - 2026-03-31

### Added
- `SkipTreeParse` hook on `ParsePolicy` — allows consumers to read file source bytes without paying for a full tree-sitter AST parse. When the hook returns true, the gateway populates `Source` but leaves `Tree` nil. Enables fast regex-based symbol extraction for large generated files (protobuf stubs, codegen output) that would otherwise stall the parser for minutes.

### Changed
- LR0/LALR construction uses packed 4-byte core entries, bucketed kernel maps, and inlined context-tag computation to reduce GC pressure and allocations during grammar generation.
- Performance pass: reduced allocations across injection arenas, query execution, tagger, and sexp serialization.

### Fixed
- Injection fast-path now uses document-relative coordinates instead of node-relative.

## [0.12.2] - 2026-03-30

### Added
- Bounded Docker presets for Fortran real-corpus grammargen runs, plus focused SQL imported-parity and direct-C regression coverage.
- Additional C#, YAML, Rust, and SQL parity tests and parser result helpers carried in from the `yaml-parity-drive` integration branch.

### Changed
- Large-grammar grammargen generation now uses lower-memory LR0/LALR data structures, tighter scratch reuse, and configurable generation budgets/timeouts to keep Fortran investigation lanes bounded.
- Parser-result normalization is split across smaller language-focused files to make recovery logic easier to maintain and extend.

### Fixed
- Imported SQL `grammar.json` round-trips no longer conflate anonymous string literals with inline regex terminals that share the same display text, restoring the affected `SELECT`/`INSERT` parity cases.
- LALR lookahead bitset initialization is now lazy-safe for tests that construct `lrContext` directly.
- `Node.Text()` edge cases, scanner adaptation, and several C#/YAML/Rust recovery and parity regressions were corrected on the merged branch.

## [0.12.1] - 2026-03-28

### Changed
- Refreshed the README roadmap/version snapshot so it reflects the shipped `grammargen` release line and the current parser/performance priorities.

### Fixed
- `grammars/scanner_lookup_test.go` no longer copies a full `Language` value when checking scanner adaptation, avoiding the `go vet` lock-copy failure caused by embedded `sync.Once` fields.

## [0.12.0] - 2026-03-28

### Added
- `grammargen` now imports and emits tree-sitter ABI 15 reserved-word sets, preserving reserved-word metadata through grammar extension and normalization.
- Added Python pattern-matching and f-string parity coverage, plus comprehensive YAML and C# parity and regression suites including a Docker-isolated C# CGO regression lane.
- Added parser recovery and normalization coverage for Rust dot ranges, Rust token trees and struct expressions, YAML recovered roots, and C# namespaces, query expressions, type declarations, Unicode identifiers, and implicit `var` restoration.

### Changed
- GLR stack equivalence checks now skip recursive frontier descent where possible and cache frontier equivalence per parse to reduce duplicate merge work on ambiguous parses.

### Fixed
- Restored Python real-corpus parity with keyword-leaf repair, print and interpolation normalization, and trailing self-call recovery in repaired blocks.
- Tightened Rust parity for macro token bindings, token trees, pattern statements, recovered function items, and struct-expression spans.
- Imported-language scanner adaptation now preserves existing `ExternalLexStates` instead of overwriting them during scanner wiring.

## [0.11.2] - 2026-03-26

### Added
- Focused TypeScript and TSX snippet parity cases for const type parameters, template literal types, enums, and class method bodies drawn from corpus-style inputs.
- COBOL snippet parity coverage for close/open statements, PIC forms, and `perform ... varying` cases that previously escaped smaller parity checks.
- CSS to the curated `cgo_harness` focus-target board so it runs through the same isolated real-corpus and cgo parity entrypoints as the other tracked grammars.

### Fixed
- DFA token selection now evaluates base and after-whitespace lex modes from one shared path, restoring CSS function-value parity and JavaScript template-string corpus parity without skipping valid immediate tokens.
- Imported-language parity adapts external scanners more defensively, including lowercase grammar-name lookup, so generated COBOL scanner wiring stays aligned with embedded references.
- Hidden passthrough flattening preserves transitive alternatives without recursing indefinitely, keeping COBOL normalization parity-safe on imported grammars.
- The COBOL real-corpus lane no longer forces the choice-lifting threshold that was driving deep-parity regressions.

## [0.11.1] - 2026-03-25

### Changed
- `grammargen` skips conflict diagnostics and provenance on the plain `GenerateLanguage` fast path unless a report or LR splitting actually needs them.

### Fixed
- Restored CSS real-corpus parity to 25/25 on no-error, sexpr parity, and deep parity.
- Tightened parser and `grammargen` parity across C/C++, JavaScript/TypeScript/TSX, COBOL, and C# normalization paths.
- Fixed after-whitespace lex modes, unary reduction collapse, and Python pass-statement normalization regressions called out in the `v0.11.1` release.

## [0.11.0] - 2026-03-24

### Added
- Grammar subset support with build tags and blob overrides for smaller focused builds.
- Race-test guards for heavyweight suites so correctness coverage can stay enabled without host OOM pressure.

### Changed
- Broad-lex fallback in `grammargen` became environment-controlled instead of always-on.
- Grammar parity coverage expanded again, including explicit-precedence handling in imported grammars.

### Fixed
- COBOL division and `perform` span normalization.
- Scala compilation-unit reconstruction and Go trivia-boundary handling in the runtime parser.

## [0.10.1] - 2026-03-19

### Fixed
- Re-registering a grammar now replaces the existing entry instead of appending a duplicate registration.

## [0.10.0] - 2026-03-18

### Added
- `grammargen.GenerateLanguageAndBlob` and `GenerateLanguageAndBlobWithContext` for one-pass compiled language plus blob output.
- Smoke and exhaustive parity modes in `cgo_harness` so required CI stays fast while deeper validation remains available.
- Pattern-based keyword detection, `ChoiceLiftThreshold`, and broader large-grammar controls in `grammargen`.

### Changed
- Large-grammar generation now uses wider `StateID` values and additional LALR/LR performance work to stay tractable on bigger grammars.

### Fixed
- Parity and normalization regressions across CSS, JavaScript/TypeScript/TSX, Python, Haskell, C/C++, Scala, and external-token handling.
- Immediate-token, after-whitespace lex-mode, and hidden external-token behavior in `grammargen` and the runtime parser.

## [0.9.2] - 2026-03-17

### Added
- `ExtensionEntry.InheritHighlights` for dynamic grammar highlight inheritance.

## [0.9.1] - 2026-03-17

### Added
- `grammars.LoadLanguageFromBlob` for loading compiled language blobs directly at runtime.

## [0.9.0] - 2026-03-17

### Added
- Initial `grammargen` release with grammar composition support and runtime integration work.
- Split WASM builds for the runtime and `grammargen`, plus browser-side runtime support for client-side highlighting.
- `RegisterExtension`-era dynamic grammar work, including the LSP proxy and related runtime improvements.

## [0.8.1] - 2026-03-16

### Added
- Highlight-query inheritance for TypeScript and TSX, fixing the major capture drop in those bundled highlight queries.

## [0.8.0] - 2026-03-16

### Added
- Structural `grep` engine with metavariables, `where`/`replace` blocks, rewrite support, and integration coverage.
- Concurrent grammar gateway for walking and parsing files, plus binary-file detection, cancellation guards, and progress reporting.
- Walk-and-parse integration tests, docs, and metadata-only `AllLanguages` enumeration.

## [0.7.4] - 2026-03-16

### Fixed
- Reordered the JSON highlight query so object keys win the intended highlight priority.

## [0.7.3] - 2026-03-16

### Added
- Swift external scanner with full lexical support: all 33 external tokens, operator disambiguation, raw strings with interpolation, block comments, semicolon insertion, and compiler directives.
- File extension registration for 48 languages.
- Pooled file parsing to reduce parser allocations.
- Token source state snapshot/restore for incremental leaf fast path.

### Changed
- Swift grammar source switched from abandoned `tree-sitter/tree-sitter-swift` to actively maintained `alex-pinkus/tree-sitter-swift`.
- External scanner count increased from 112 to 116.
- All 206 grammars now produce error-free parse trees (previously 3 degraded).

### Fixed
- Swift C parity: lock file updated to match the grammar used for blob generation.

## [0.7.0] - 2026-03-15

### Added
- Incremental parsing engine: fast path for token-invariant leaf edits, top-level node reuse after edits, dirty-flag clearing along modified path only, and external scanner checkpoints for incremental reuse.
- Adaptive arena sizing and GSS capacity hinting for incremental and full parses.
- Parser timeout and cancellation support (`WithTimeout`, `WithCancellation`).
- Parser pool for concurrent parse workloads.
- Arena memory budget to prevent OOM crashes.
- Linguist-style language detection: filename, extension, and interpreter/shebang-based detection with display names (`cmd/gen_linguist`, `grammars/linguist_*.go`).
- Syntax highlighting queries for 40+ additional languages including top-50 grammars, norg, promql, and tmux.
- Native TOML lexer with date/time parsing.
- GLR-aware C preprocessor lexer with function-like macros, signed literals, and synthetic endif.
- Query metadata accessors for captures, strings, and pattern ranges.
- Query match limits, depth bounds, and symbol alias support.
- `Tree.Copy`, `Parser.Language`, `Node.Edit`, and `RootNodeWithOffset` API additions.
- Parser logging and tree DOT visualization for debugging.
- Multi-strategy full parse retry with bounded escalation.
- Dense token lookup for small parser states.
- Real-world corpus parity board and reporter (`cgo_harness`).
- GLR canary set and cap-pressure tests for parity regression detection.
- CI grammar freshness validation, tiered benchmark baselines, and coverage ratchet.

### Changed
- Structural language parity coverage expanded from 54 to 100 curated languages.
- Parser reduce hot path optimized: scratch buffers, pre-computed alias sequences, fast visible reduce path, deferred hidden node flattening to visible parent boundary.
- GLR engine tuned: lazy GSS node hashing in single-stack mode, key-based stack culling, small-path merge optimization, temporary stack oversubscription before culling.
- Query engine optimized: dense array for root pattern lookup, compile-time alternation matching index, avoid heap allocation for candidate indices.
- Go and TypeScript normalization refactored to symbol-based context; span attribution switched on language.

### Fixed
- Top-50 parity burndown: broad fixes across lexers, normalization, scanners, and GLR paths reducing degraded grammars to 0.
- GLR robustness: deterministic stack culling, correct tie-breaking for duplicate stacks, all-dead stack recovery, preferred visible tokens in union DFA on exact ties, higher action specificity on same lexeme.
- External scanner fixes: correct MarkEnd ordering, retry with state validation table, deterministic external-scanner mode for parity.
- Field attribution: prevent inherited field misassignment across GLR branches, correct field assignment for C# join clauses, skip inherited field projection when target span has direct fields.
- Span calculation: correct span for invisible nodes in GLR reduce, chain hidden spans via backward scan, extend parent span to window with predecessor boundary clamping.
- Query fixes: handle repeated field names with sibling capture accumulation, multi-sibling grouping patterns with wildcard root.
- Zero-width token handling to match C tree-sitter semantics.
- Byte offset-based UTF-8 column tracking in lexer.
- Infinite missing-token recovery cycles prevented.
- Conflicting inherited field IDs in `buildFieldIDs` resolved.

## [0.6.0] - 2026-03-01

### Added
- `ParseWith` functional options API (`WithOldTree`, `WithTokenSource`, `WithProfiling`) and `ParseResult`.
- Parser runtime diagnostics surfaced on `Tree` (`ParseRuntime`, stop-reason/truncation metadata).
- Top-50 grammar smoke correctness gate and expanded cgo parity suites (fresh parse, no-error corpus checks, issue repros, GLR canary).
- Grammar lock update automation (`cmd/grammar_updater` + CI workflow integration).
- Configurable injection parser nesting depth.

### Changed
- Full-parse GLR behavior tuned for correctness-first performance:
  - lower default global GLR stack cap with better top-K retention behavior,
  - improved merge/pruning hot paths and profiling counters,
  - benchmark harness tightened to avoid truncated-parse results.
- Significant parser/query maintainability refactors:
  - parser/query monoliths split into focused files (`parser_*`, `query_compile_*`).
- README benchmark and gate documentation refreshed to match current numbers and commands.

### Fixed
- Multiple parity/correctness regressions in HTML/YAML/disassembly paths and grammar support wiring.
- Query predicate parsing and generated query edge cases.
- Rewriter multi-edit coordinate handling and parser profile availability signaling.

## [0.5.2] - 2026-02-24

### Fixed
- Simplified asm register-label query pattern fix in bundled grammar queries.

## [0.5.1] - 2026-02-24

### Fixed
- Corrected tree-sitter query node types in bundled grammar queries.

## [0.4.0] - 2026-02-24

### Fixed
- Parser span-calculation correctness fixes.
- `ts2go` GOTO/action detection fixes.

## [0.3.0] - 2026-02-23

### Added
- Benchmark suite for parser/query/highlighter/tagger paths.
- Fuzzing targets and stress-test coverage.

## [0.2.0] - 2026-02-23

### Added
- Broad grammar expansion with external-scanner support across 80+ grammars.

## [0.1.0] - 2026-02-19

### Added
- Initial standalone pure-Go runtime module.
- External scanner VM foundation and base parser/lexer/tree infrastructure.

[Unreleased]: https://github.com/odvcencio/gotreesitter/compare/v0.22.0...HEAD
[0.22.0]: https://github.com/odvcencio/gotreesitter/compare/v0.21.0...v0.22.0
[0.21.0]: https://github.com/odvcencio/gotreesitter/compare/v0.20.9...v0.21.0
[0.20.9]: https://github.com/odvcencio/gotreesitter/compare/v0.20.8...v0.20.9
[0.20.8]: https://github.com/odvcencio/gotreesitter/compare/v0.20.7...v0.20.8
[0.20.7]: https://github.com/odvcencio/gotreesitter/compare/v0.20.6...v0.20.7
[0.20.6]: https://github.com/odvcencio/gotreesitter/compare/v0.20.5...v0.20.6
[0.20.5]: https://github.com/odvcencio/gotreesitter/compare/v0.20.4...v0.20.5
[0.20.4]: https://github.com/odvcencio/gotreesitter/compare/v0.20.3...v0.20.4
[0.20.3]: https://github.com/odvcencio/gotreesitter/compare/v0.20.2...v0.20.3
[0.20.2]: https://github.com/odvcencio/gotreesitter/compare/v0.20.1...v0.20.2
[0.20.1]: https://github.com/odvcencio/gotreesitter/compare/v0.20.0...v0.20.1
[0.20.0]: https://github.com/odvcencio/gotreesitter/compare/v0.20.0-rc4...v0.20.0
[0.20.0-rc4]: https://github.com/odvcencio/gotreesitter/compare/v0.20.0-rc3...v0.20.0-rc4
[0.20.0-rc3]: https://github.com/odvcencio/gotreesitter/compare/v0.19.1...v0.20.0-rc3
[0.19.1]: https://github.com/odvcencio/gotreesitter/compare/v0.19.0...v0.19.1
[0.19.0]: https://github.com/odvcencio/gotreesitter/compare/v0.18.0...v0.19.0
[0.18.0]: https://github.com/odvcencio/gotreesitter/compare/v0.17.4...v0.18.0
[0.17.4]: https://github.com/odvcencio/gotreesitter/compare/v0.17.3...v0.17.4
[0.17.3]: https://github.com/odvcencio/gotreesitter/compare/v0.17.2...v0.17.3
[0.17.2]: https://github.com/odvcencio/gotreesitter/compare/v0.17.1...v0.17.2
[0.17.1]: https://github.com/odvcencio/gotreesitter/compare/v0.17.0...v0.17.1
[0.17.0]: https://github.com/odvcencio/gotreesitter/compare/v0.16.0...v0.17.0
[0.16.0]: https://github.com/odvcencio/gotreesitter/compare/v0.15.3...v0.16.0
[0.15.3]: https://github.com/odvcencio/gotreesitter/compare/v0.15.2...v0.15.3
[0.15.2]: https://github.com/odvcencio/gotreesitter/compare/v0.15.1...v0.15.2
[0.15.1]: https://github.com/odvcencio/gotreesitter/compare/v0.15.0...v0.15.1
[0.15.0]: https://github.com/odvcencio/gotreesitter/compare/v0.14.0...v0.15.0
[0.14.0]: https://github.com/odvcencio/gotreesitter/compare/v0.13.4...v0.14.0
[0.13.4]: https://github.com/odvcencio/gotreesitter/compare/v0.13.3...v0.13.4
[0.13.3]: https://github.com/odvcencio/gotreesitter/compare/v0.13.0...v0.13.3
[0.13.0]: https://github.com/odvcencio/gotreesitter/compare/v0.12.2...v0.13.0
[0.12.2]: https://github.com/odvcencio/gotreesitter/compare/v0.12.1...v0.12.2
[0.12.1]: https://github.com/odvcencio/gotreesitter/compare/v0.12.0...v0.12.1
[0.12.0]: https://github.com/odvcencio/gotreesitter/compare/v0.11.2...v0.12.0
[0.11.2]: https://github.com/odvcencio/gotreesitter/compare/v0.11.1...v0.11.2
[0.11.1]: https://github.com/odvcencio/gotreesitter/compare/v0.11.0...v0.11.1
[0.11.0]: https://github.com/odvcencio/gotreesitter/compare/v0.10.1...v0.11.0
[0.10.1]: https://github.com/odvcencio/gotreesitter/compare/v0.10.0...v0.10.1
[0.10.0]: https://github.com/odvcencio/gotreesitter/compare/v0.9.2...v0.10.0
[0.9.2]: https://github.com/odvcencio/gotreesitter/compare/v0.9.1...v0.9.2
[0.9.1]: https://github.com/odvcencio/gotreesitter/compare/v0.9.0...v0.9.1
[0.9.0]: https://github.com/odvcencio/gotreesitter/compare/v0.8.1...v0.9.0
[0.8.1]: https://github.com/odvcencio/gotreesitter/compare/v0.8.0...v0.8.1
[0.8.0]: https://github.com/odvcencio/gotreesitter/compare/v0.7.4...v0.8.0
[0.7.4]: https://github.com/odvcencio/gotreesitter/compare/v0.7.3...v0.7.4
[0.7.3]: https://github.com/odvcencio/gotreesitter/compare/v0.7.0...v0.7.3
[0.7.0]: https://github.com/odvcencio/gotreesitter/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/odvcencio/gotreesitter/compare/v0.5.2...v0.6.0
[0.5.2]: https://github.com/odvcencio/gotreesitter/compare/v0.5.1...v0.5.2
[0.5.1]: https://github.com/odvcencio/gotreesitter/compare/v0.4.0...v0.5.1
[0.4.0]: https://github.com/odvcencio/gotreesitter/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/odvcencio/gotreesitter/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/odvcencio/gotreesitter/compare/v0.1.0...v0.2.0
