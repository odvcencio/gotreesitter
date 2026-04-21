# Changelog

All notable changes to this project are documented in this file.

This project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
for tags and release notes while still in `0.x`.

## [Unreleased]

### Added
- C/C++ lexer bridge now accepts `#embed` directive lines and
  `__has_embed(...)` conditional feature-test forms (including parameter
  variants) without parse errors.

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

[Unreleased]: https://github.com/odvcencio/gotreesitter/compare/v0.15.1...HEAD
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
