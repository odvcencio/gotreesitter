# Changelog archive: v0.24.1 – v0.44.0

Older entries moved out of the root [CHANGELOG.md](../../CHANGELOG.md) to
keep it focused on current releases. See [archive-1.md](archive-1.md) for
more recent history and [archive-3.md](archive-3.md) for earlier history.

## [0.44.0] - 2026-07-20

### Fixed

- **Swift's `Language()` call no longer exceeds the 256 MB CI memory
  ceiling.** This closes the known issue noted in v0.43.1. grammargen's
  `buildLexDFA` rebuilt an independent lexer DFA per lex mode, with no
  sharing across modes. Swift's about 331 lex modes each rebuilt the same
  about 190-state identifier, operator, and comment automaton from scratch.
  A new post-construction minimization pass, `grammargen/dfa_minimize.go`,
  merges observationally-equivalent DFA states across lex-mode boundaries
  (PR #396). Swift's LexStates table drops from 63,150 to 2,067 entries.
  Its `Language()` call now retains about 25 MB, down from about 490 MB.
  `swift.bin` is regenerated and recertified against the Swift regression
  suite and corpus, shrinking from 7,474,360 to 373,401 bytes. Byte-identical
  lexing is proven across a 10-grammar parity corpus. The blob format is
  unchanged. The known-exception entry for Swift is removed from the CI
  memory-ceiling gate.
- **grammargen's C code emitter** had several defects. This release fixes:
  - duplicate C identifiers for anonymous tokens, a compile error;
  - infinite lexer loops at EOF on negated character classes;
  - wrong re-lex targets for multi-character skips (CRLF, backslash-newline);
  - an alias-stride bound sized from the wrong table, causing out-of-bounds
    reads.

  A new C-runtime parity harness compiles the emitted `parser.c` against the
  tree-sitter v0.25.0 runtime (PR #391). It byte-compares the resulting AST
  against the pure-Go oracle.
- **Incremental memo-cache growth is now input-deterministic.** Growth used
  to depend on prior parser history, not only the current parse. Identical
  (source, edit) pairs could take different growth paths depending on
  earlier use. A new adaptive trigger, driven by the `cNodeMemoThrash`
  collision count of the current parse alone, now controls growth (PR #392,
  issue #380 follow-up). Clean, non-pathological parses still stay at the
  small 128-entry default.
- **Incremental reuse is now barred for trees built by the phase-zero
  compact parser.** This closes reuse holes on three entry points:
  - the DFA entry point;
  - the custom-token-source entry point;
  - the token-invariant-leaf-fastpath entry point.

  `ParseIncremental` now forces a full fresh parse when the old tree is
  compact-materialized (PR #393). A new top-down ParseState table-replay
  mechanism reconstructs parser states over the full compact derivation. It
  runs before hidden-node elision and grammar aliasing. It falls back to a
  sentinel state when a state cannot be reliably reconstructed, for example
  for extra or comment leaves.

### Improved

- **CSS editor-style incremental edits now reuse far more of the unchanged
  tree.** A fragility-gated top-level sibling block-splice replaces the old
  cmake/css name allowlist (PR #395, campaign O(edit) workstream W1).
  Admission is now per node: a fragility bit plus byte equality, not a
  language name. On a CSS editor-edit measurement, `rootNonLeafChanged`
  rejections near the top of the tree drop from 1,379 to 14. Node reuse
  rises from 63.8% to 99.5%. Go incremental node reuse does not improve in
  this release. A deeper, pre-existing engine gap blocks it: the reuse check
  runs before the eager reduce chain settles state. Work on that gap
  continues.

### Added

- A rollback safety valve: the `GTS_GRAMMARGEN_DISABLE_LEX_MINIMIZE`
  environment flag falls back to the raw, per-mode lexer tables. Use it only
  if a correctness question comes up (PR #396).

### Docs

- Refresh documentation prose to the ASD-STE100 style guide across
  README.md, BENCH.md, AGENTS.md, and the authoring-languages and
  external-scanners guides (PR #385).
- Clarify the GLR stack cap default in AGENTS.md (PR #394). This release
  also corrects the stale release-status paragraph in README.md.

### Known Issues

- Go incremental node reuse is unchanged by the W1 fragility-gated splice
  (PR #395). The reuse check runs before the eager reduce chain settles
  state, blocking admission. A fix is tracked.

## [0.43.1] - 2026-07-20

### Fixed

- **TypeScript/TSX bare default parameters** (`function f(a = 1) {}`) no
  longer collapse to `ERROR`. The fix widens the GLR merge budget when a
  default-parameter shape is present. It covers plain, array-element
  (`[a = 1]`), renamed-property (`{a: b = 1}`), unicode-identifier, and
  comment-adjacent forms, in both TypeScript and TSX (PR #389). The root
  cause: the cap-one merge budget discarded the correct derivation by score
  before any structural comparison.
- Destructuring declarations with defaults (`const [a = 1] = arr`) now parse
  correctly. This fix is incidental coverage from the same change.

### Improved

- Grammar blob decoding pre-sizes its buffer from the gzip size hint. This
  change cuts total allocation churn about 32 percent and peak memory about
  11 percent across all 206 grammars.

### Added

- A CI memory-ceiling test sweeps all 206 grammars. It fails any
  `Language()` load above 256 MB. Swift is a documented exception.
- Environment-gated diagnostic tests characterize the incremental
  insert-retry timing nondeterminism from issue #380 (PR #388).

### Known Issues

- Swift's `Language()` call still retains about 491 MB. The root cause is
  upstream in grammargen: the lexer DFA rebuilds once per lex mode. A full
  fix is tracked.

## [0.43.0] - 2026-07-20

### Fixed

- **Incremental length-changing edits now reuse the unchanged suffix** instead
  of re-lexing the whole tail to EOF (issue #380, step 1). The reuse byte-guard
  previously sliced the pre-edit source at post-edit (shifted) coordinates, so
  any insert/delete rejected every suffix subtree as ancestor-dirty and
  reparsed the entire suffix; it now reverse-maps coordinates through the
  recorded edits. A follow-up guard prevents a clamped, edit-overlapping node
  from being reused when its text actually changed (previously a
  silent-corruption path on length-changing replace/insert edits).
  Interior/non-leaf subtree reuse across such edits remains limited — a tracked
  follow-up.
- Restore F# external-scanner checkpoints to the locked grammar's byte layout,
  keeping incremental scanner state compatible with the reference runtime.
- Apply the upstream-C repetition-skip conflict fold while dispatching around
  incrementally reusable syntax. Python deletion edits that previously returned
  a complete but structurally divergent tree now match fresh parsing and the
  locked C oracle across a systematic minimal-witness sweep.
- Fail closed to a fresh parse for Python-derived external scanners (Python,
  Mojo, and Starlark) when edits require checkpoint reuse, until each
  indentation-state restoration path is certified exact. Same-length
  token-invariant leaf validation still reuses the complete old tree without
  reparsing. Included-range token-source wrappers preserve the underlying
  scanner's fallback reason in incremental profiles.
- **Go `new(T)`/`make(T)`** now retags a sole bare-identifier argument to
  `type_identifier`, matching the locked C oracle for that shape. This is a
  targeted **partial** fix: qualified (`new(pkg.Type)`), pointer (`new(*T)`),
  and parenthesized type arguments still differ from C and require a
  parser-table-level fix (tracked).

### Added

- **Incremental-invariant correctness gate** (default-on). It sweeps a curated
  real-world corpus applying single-byte delete/insert/replace edits and
  asserts `ParseIncremental` is structurally identical to a fresh `Parse`
  wherever the fresh parse is genuinely clean — turning previously-silent
  incremental corruption into explicit CI failures. Cleanliness is checked by a
  recursive `IsError()`/`IsMissing()` walk (not `HasError()`), governed by a
  tracked allowlist with a staleness ratchet.
- **Gated one-pass selected-store builder** (build-tagged, off by default) — a
  diagnostic parser-core materialization candidate that does not touch the
  production parse path and is admitted only when it produces byte-identical
  deep trees to the staged builder across the canonical fixtures.

### Tooling

- Extend the authenticated work-count board with direct main-DFA callable-entry
  and resolved action-cell diagnostics for Go and locked static C, while keeping
  Go union-frontier elections and C per-version lex requests explicitly
  unavailable for cross-engine comparison.

## [0.42.0] - 2026-07-18

### Performance

- Refresh the opt-in Go build-time PGO artifact with a hash-verified composite
  of the established production profile and authenticated selected, clean, and
  accepted-error parsing profiles. On the pinned quiet host, the composite is
  4.01% faster than the previous profile by equal-fixture geomean across the
  four selected-store fixtures, with every fixture faster, while preserving
  the existing five-grammar production workload (statistically
  indistinguishable, with a -0.14% median point estimate) and improving it by
  5.37% versus PGO-off. Production allocation medians move by +0.43% B/op and
  +0.07% allocs/op. Exact selected-store admission and the production corpus
  digest remain unchanged; checked-in inputs and a deterministic composition
  script make the artifact reproducible.
- Keep compact-scheduler dispatch and reduction scratch cleanup panic-safe in
  small wrappers so their large action bodies no longer register three runtime
  defers per reduction-bearing pass. On the pinned quiet host, the authenticated
  four-fixture `BenchmarkDiagnosticParserCoreCanonicalTotal` lane—compact
  canonical parsing plus public-node materialization—improves by 4.19% by
  equal-fixture geomean, with every fixture 3.73-5.28% faster, unchanged
  allocations, exact work and deep-tree digests, and zero fallback. The route
  remains build-tagged and diagnostic-only.
- Return BibTeX, CSS, Yuck, Bash, SCSS, C#, Agda, Ledger, Authzed, Make, and
  TLA+ automatic forest routing to explicit-only experiments after an
  authenticated full-manifest audit. The exact BibTeX, CSS, SCSS, and Yuck
  routes were 32.7%, 32.3%, 4.8%, and 46.2% slower than production. Ledger and
  Make dispatched 0/1 and 0/19 files. Bash routed 1.3% slower and differed from
  direct C on 61/1,263 dispatches; C# routed 9.3% faster but differed on
  212/1,427; Agda routed 7.5% slower and differed on 1,444/2,070; Authzed routed
  3.9x slower and differed on 27/35; TLA+ routed 3.2x slower and differed on
  105/267. Explicit `Language.WantsForest`, recovery, incremental, and direct
  forest experiments remain available.
- Avoid repeating the complete external-scanner full-parse retry ladder for the
  exact built-in Crystal and Matlab grammar artifacts, while retaining the
  entire accepted-error widening and final-merge ladder once. In the exact-head
  certification at `c6de0991`, the locked 2,380-file Crystal corpus preserves
  every deep tree and C admission relation, reduces attempts from 13,445 to
  8,120, lowers aggregate parse wall time by 24.62%, and lowers allocated bytes
  by 10.19%. On the locked 1,434-file Matlab corpus, attempts fall from 7,866 to
  4,650, wall time by 40.99%, and allocated bytes by 32.67%, again with exact
  output and oracle relation preservation on every file.
- Execute Go highlight queries directly over the build-tagged compact selected
  store through value node handles, without constructing public-node proxies.
  All four locked real-Go fixtures preserve exact ordered query captures,
  directives, and final highlight ranges; unsupported missing-node queries
  decline explicitly. On the exact rebased revision, selected-store query time
  improves by 8.51% and B/op by 14.18% against the public-tree control by
  equal-fixture geomean. End-to-end selected parse plus highlight improves by
  12.90% with 26.38% fewer B/op, and every fixture median is faster. A balanced
  two-order public `Query.Execute` control preserves wall time while reducing
  B/op and allocations by 14.50%; direct streaming-cursor allocations remain
  unchanged. The selected route remains diagnostic-only.
- Move Faust, CMake, and Erlang automatic forest routing back to explicit-only
  experiments after full locked-corpus recertification superseded their earlier
  small-corpus receipts. Faust remained exact across 706 files but routed in
  9.94 seconds versus 6.52 seconds for production; CMake remained exact across
  11,506 files but routed in 35.03 seconds versus 23.36 seconds. Erlang's route
  improved 4,114 files from 213.14 to 183.94 seconds, but 175 routed trees
  differed from production and 125 forest trees differed from the direct C
  oracle. Explicit `Language.WantsForest` experiments remain available for all
  three while route overhead and Erlang result selection are improved.
- Keep Common Lisp on the production parser unless callers explicitly request
  the forest path. On the locked 1,357-file corpus, the routed parser took
  173.6 seconds versus 46.0 seconds for production, dispatched one file, fell
  back on 1,356, and diverged on the sole dispatched result. A separate direct
  C-oracle audit rejected that result as well. On the authenticated
  largest-eight static-C board, disabling automatic routing completed all eight
  files instead of timing out on two, cut the six matched files by 58.3%,
  reduced isolated sweep wall time by 19.9%, and lowered max RSS from 2.14 GB
  to 746 MB. Explicit forest and recovery experiments remain available.
- Add an opt-in, build-tagged selected-tree backing store at the compact
  parser/consumer boundary. Accepted payloads are sealed only for the direct
  consumer; the public-node control remains store-free. The store preserves
  occurrence identity and authenticated visible metadata across compact-core
  resets, polls cancellation and resource limits, enforces occurrence and
  retained-byte caps before growth, builds its quadratic unary policy only on
  demand, and returns each atomic record/child backing pair through an explicit
  synchronized release lifecycle. On the exact reviewed revision, the direct
  consumer improves the same-revision public-node boundary by 13.27% across
  the four locked fixtures, with every fixture 11.18-15.58% faster, B/op down
  56.42%, lower RSS, exact work, and zero fallback. Its strict locked-static-C
  publication measures 2.685181x C by equal-fixture geomean, 2.676794x by
  fixed-suite sum, and 2.791974x on the worst fixture. The route remains
  diagnostic-only and intentionally omits parser-state metadata.

### Tooling

- Extend the locked static-C publication driver with an authenticated
  selected-store backend and retain selected-store bytes alongside total
  allocation, work, fallback, and RSS metrics.
- Add an opt-in retry-profile corpus certifier that compares the duplicated
  external-scanner retry ladder with an exact-blob candidate file by file while
  preserving accepted-error widening and recording the locked C-oracle
  admission relation without treating pre-existing corpus gaps as candidate
  regressions. Its success and counterexample receipts include deep-tree
  digests, stop/full-span state, attempt rungs, allocation totals, clean/error
  splits, and locked-corpus source identities. Schema-v2 journals fail closed
  on unknown or dirty candidate revisions, mixed schemas, oracle or parser
  configuration drift, duplicate or unselected paths, and any resumed row that
  no longer revalidates exactly; fresh rows are validated before publication.

### Fixed

- Keep compact-parser arena and selected-root cap arithmetic portable on
  32-bit targets by widening lengths before uint32-bound checks and additions.

## [0.41.0] - 2026-07-18

### Performance

- Run the authenticated fresh compact scheduler as one fail-closed session,
  resetting the entire compact core after any error or panic instead of taking
  a rollback checkpoint for every successful operation. Its clean pinned-host
  publication measures 3.118130x static C by equal-fixture geomean, 3.169740x
  by fixed-suite sum, and 3.185522x on the worst fixture, with exact static-C
  admission and zero fallback. The compact route remains build-tagged and
  diagnostic-only.
- Bypass general graph enumeration when a compact-parser reduction follows a
  single-link stack path, while retaining the existing enumerator for branched
  paths and preserving its resource-limit checks. On the pinned quiet host,
  the authenticated `query_compile` candidate improves total time by 2.51%
  with unchanged parser work, bytes, and allocations. The compact route
  remains build-tagged and diagnostic-only.
- Skip redundant production-metadata remapping while materializing a compact
  tree whose terminals and reductions were already authenticated at
  construction. Generic diagnostic publication retains the full validation
  path. A balanced two-order quiet-host board improves the authenticated
  four-fixture fresh-full geomean by 3.44%, with every fixture improving by
  2.36-4.50%, materialization improving by 13.20%, unchanged compact work, and
  zero fallback. The compact route remains build-tagged and diagnostic-only.
- Preclassify immutable compact-parser action rows and route singleton shift,
  reduce, and extra actions without repeatedly interpreting or copying the row.
  Two reverse-order quiet-host runs improve the authenticated four-fixture
  candidate Total geomean by 4.13-4.26%, with every fixture improving by
  3.42-5.00%, unchanged parser work and fallback counts, and a worst
  candidate/static-C ratio below 3.90x. The candidate remains build-tagged and
  diagnostic-only.
- Cache exact source points in a bounded, allocation-free materialization-local
  table for the build-tagged compact parser. The four authenticated fixtures
  reuse 59.02-62.17% of point lookups; two reverse-order quiet-host boards
  improve equal-fixture candidate time by 2.14-2.35%, with every fixture
  faster, unchanged parser work, and unchanged fallback counts. This route
  remains diagnostic-only.

### Documentation

- Publish the authenticated v0.40.0 fresh, materialized real-Go receipt:
  4.851050x static C by equal-fixture geomean, 5.472406x by fixed-suite sum,
  and 5.608320x on the worst fixture. The 0.716% geomean improvement over
  v0.39.0 is below the reproducible 2% win threshold, so this is a baseline
  refresh rather than a banked performance win.

### Fixed

- Scheduler transaction token misuse on a different diagnostic core now
  poisons and rolls back only the called core, without mutating the token owner.
- Deferred result-compatibility finalization stays lazy while trees are owned by
  parser retry/selection code, then synchronizes every public read that can
  observe normalized nodes or diagnostics, including pooled tree values.
- Query byte and point ranges now match the locked C runtime for half-open
  boundaries, zero-width nodes at the range start, reversed range updates, and
  zero-valued unbounded-end sentinels.
- DFA token-source seeks clamp past-EOF offsets before integer narrowing and
  preserve exact EOF coordinates across both skip APIs, including 32-bit builds.
- Query string literals now decode control, quote, and backslash escapes through
  execution and reject unescaped newlines like the locked C query parser.
- Grammar imports now decode C string and Unicode escapes without losing the
  reversible question-mark spelling shared with grammargen; refreshed Agda and
  Dhall blobs expose their Unicode symbols correctly. Generated C now uses the
  ABI-appropriate lexer-mode layout, emits flattened parse-action offsets, and
  validates complete ABI-15 supertype metadata before emission. Lowercase
  keyword leaves are classified from parser-reachable ownership like
  tree-sitter.
- Query `MISSING` patterns now test missing nodes, and inert `#is?`/`#is-not?`
  properties are available through public metadata accessors. Descendant range
  walks now match upstream behavior for reversed ranges and zero-width missing
  children.
- Highlight queries now resolve supported built-in inheritance chains across
  registration order and same-name replacements without duplicating cyclic
  queries. Incompatible locked grammar/query pairs remain fail-closed.
- Incremental parses that accept a full-span ERROR tree under a wider merge
  policy may retry once with the corresponding fresh-parse policy and adopt
  only a strictly better result. Runtime and profile diagnostics report the
  retry attempt, selection, cap, cause, and whether old-tree reuse was active.
- Token-invariant single-leaf edits stay outside accepted-error retry routing,
  avoiding a whole-tree error scan on the one-token validation path.

### Tooling

- Add a bounded, build-tagged parser trace that separates lookup cells from
  execution-time cell reconstruction and retains whole-parse aggregates after
  its chronological event prefix fills. Scanner checkpoints bind their cached
  state to the current event token span and remain distinct from unavailable
  state after relexing. Collision keys have explicit memory caps; reaching a
  cap exposes unaudited counts, marks the audit incomplete, and blocks claims
  that require complete collision evidence. A base-pinned content manifest and
  fail-closed paired receipt identify which production, compact, and locked-C
  observations can actually be compared. Observer equality and untagged
  assembly tests keep the trace diagnostic-only.
- Add the four-fixture authenticated Go/static-C work-count board with direct
  counters at their exact hook boundaries, Go-only representation rows marked
  incomparable, and missing mandatory instrumentation reported separately from
  out-of-band work-ratio audit findings.
- Add the build-tagged compact parser-core candidate and a work-board backend
  that authenticates its exact EOF acceptance, selected tree digest, ranges,
  fields, selected-node census, and repeat-identical work counts on four locked
  real-Go fixtures. The candidate remains diagnostic-only and fail-closed: its
  materializer does not preserve `ParseState` or `PreGotoState`, so this
  admission is not a production-routing, incremental, recovery, or exact public
  node-API compatibility claim.
- Add a bounded, build-tagged selected-occurrence capability for the compact
  parser candidate. It preserves repeated physical occurrences, construction
  states, and checked subtree spans without copying the observer proof; its
  borrowed immutable windows allow read-only re-entry and block lifecycle
  mutation until released. Exact admissions and isolated race coverage remain
  green, with no measured performance-regression claim.
- Bank a paired quiet-host receipt against the locked static `-O2` C oracle.
  At the exact post-fusion revision, public `Parser.Parse` measures 4.813350x C
  by equal-fixture geomean and 5.419730x C by fixed-suite sum of medians. The
  build-tagged compact candidate measures 3.847233x C and 3.988613x C,
  respectively, with a 4.018193x worst fixture and zero fallback in every timed
  sample. These branch-only candidate numbers apply only to its authenticated
  clean fresh-full surface; they do not replace the public parser claim.
- Fuse nested transaction checkpoints across the build-tagged compact
  scheduler while preserving standalone rollback and capability semantics.
  The authenticated four-fixture Total geomean improves by 8.25%, every
  fixture improves by 7.21-8.96%, and allocation counts remain unchanged.
- Add a versioned, locked incremental admission matrix that separates identity,
  leaf-validation, real-code GLR, recovery, and stateful-scanner behavior using
  runtime evidence. It rejects full-parse fallback, authenticates both edit
  directions against fresh Go and C trees, and atomically publishes a
  machine-readable closure receipt only after every row passes.

- Real-corpus grammar parity can use a durable configurable corpus root and
  split-grammar corpus layouts without silently losing colliding basenames.
  Eligible-sample caps now apply on every generated-result path, committed
  floors reject over-cap rows, and the aggressive runner and floor share the
  same 30-sample limit.

## [0.40.0] - 2026-07-17

### Performance

- Build-time PGO. Ships a default profile (`pgo/default.pgo`) and a repdriver
  tool; the `parity_report` build compiles with it. About 7% wall-clock
  reduction, byte-identical across all 206 grammars.
- Forest-index allocation overhaul. The forest alternative index is now pooled
  across parses and the per-compare throwaway comparison slices are eliminated.
  On the forest-path grammars (C#, Bash, CMake) allocation bytes drop 86–96% and
  GC-cycle CPU 48–90%, with byte-identical trees.
- GLR result-comparator copy elimination. The forest disambiguation comparator
  chain now takes stack pointers instead of copying a 104-byte stack value per
  call, removing about twelve `runtime.duffcopy` calls per compare. About 14%
  wall-clock reduction on the forest-path grammars, byte-identical.
- Forest reducer pooling. The per-parse forest reducer is now pooled, cutting C#
  parse allocation a further ~51% by bytes, byte-identical.

### Security

- Query matcher work budget. The `-All` quantifier matchers now charge a
  per-execution work budget, bounding worst-case combinatorial blow-up on
  adversarial query/source pairs. Exposed via `Cursor.DidExceedMatchLimit` and
  configurable with `SetMatchWorkBudget` (default 1,000,000).

### Fixed

- Incremental parsing no longer reuses a stale subtree when an edit shifts a
  token boundary that abuts a reused node's right edge. Both the leaf and the
  non-leaf (wrapped-token) reuse paths now reject reuse when the freshly lexed
  token's end byte disagrees with the stored boundary, preventing spurious
  `ERROR` nodes on common edits such as deleting the whitespace between two
  identifiers (e.g. Clojure `(a b)` → `(ab)`). Verified byte-identical to a
  fresh parse across the C-oracle incremental parity harness.

### Documentation

- Label the authenticated `2c702656` parser receipt as the v0.39.0
  production-code baseline rather than implying that its revision is current
  main after the documentation-only release commits.

## [0.39.0] - 2026-07-17

Correctness-and-evidence release. Query ranges, literals, missing-node patterns,
property metadata, highlight inheritance, lazy tree finalization, DFA EOF
seeking, grammar imports, and generated C metadata now match their locked
contracts more closely. Locked incremental and work-count receipts authenticate
the exercised behavior, while durable corpus roots, split-grammar layouts, and
bounded floors make real-corpus checks reproducible. The authenticated
production receipt at `2c702656` measures public `Parser.Parse` at 4.886056x C
by equal-fixture geomean, 5.517602x C by fixed-suite sum, and 5.648204x C on the
worst fixture against the locked static `-O2` C oracle.

### Fixed

- Deferred result-compatibility finalization stays lazy while trees are owned by
  parser retry/selection code, then synchronizes every public read that can
  observe normalized nodes or diagnostics, including pooled tree values.
- Query byte and point ranges now match the locked C runtime for half-open
  boundaries, zero-width nodes at the range start, reversed range updates, and
  zero-valued unbounded-end sentinels.
- DFA token-source seeks clamp past-EOF offsets before integer narrowing and
  preserve exact EOF coordinates across both skip APIs, including 32-bit builds.
- Query string literals now decode control, quote, and backslash escapes through
  execution and reject unescaped newlines like the locked C query parser.
- Grammar imports now decode C string and Unicode escapes without losing the
  reversible question-mark spelling shared with grammargen; refreshed Agda and
  Dhall blobs expose their Unicode symbols correctly. Generated C now uses the
  ABI-appropriate lexer-mode layout, emits flattened parse-action offsets, and
  validates complete ABI-15 supertype metadata before emission. Lowercase
  keyword leaves are classified from parser-reachable ownership like
  tree-sitter.
- Query `MISSING` patterns now test missing nodes, and inert `#is?`/`#is-not?`
  properties are available through public metadata accessors. Descendant range
  walks now match upstream behavior for reversed ranges and zero-width missing
  children.
- Highlight queries now resolve supported built-in inheritance chains across
  registration order and same-name replacements without duplicating cyclic
  queries. Incompatible locked grammar/query pairs remain fail-closed.
- Incremental parses that accept a full-span ERROR tree under a wider merge
  policy may retry once with the corresponding fresh-parse policy and adopt
  only a strictly better result. Runtime and profile diagnostics report the
  retry attempt, selection, cap, cause, and whether old-tree reuse was active.
- Token-invariant single-leaf edits stay outside accepted-error retry routing,
  avoiding a whole-tree error scan on the one-token validation path.

### Tooling

- Bank an authenticated quiet-host production receipt against the locked
  static `-O2` C oracle. At the v0.39.0 production-code baseline `2c702656`,
  public `Parser.Parse` measures 4.886056x C by equal-fixture geomean and
  5.517602x C by fixed-suite sum of medians, with a 5.648204x worst fixture.
- Add a bounded, build-tagged parser trace that separates lookup cells from
  execution-time cell reconstruction and retains whole-parse aggregates after
  its chronological event prefix fills. Scanner checkpoints bind their cached
  state to the current event token span and remain distinct from unavailable
  state after relexing. Collision keys have explicit memory caps; reaching a
  cap exposes unaudited counts, marks the audit incomplete, and blocks claims
  that require complete collision evidence. A base-pinned content manifest and
  fail-closed paired receipt identify which production, compact, and locked-C
  observations can actually be compared. Observer equality and untagged
  assembly tests keep the trace diagnostic-only.
- Add the four-fixture authenticated Go/static-C work-count board with direct
  counters at their exact hook boundaries, Go-only representation rows marked
  incomparable, and missing mandatory instrumentation reported separately from
  out-of-band work-ratio audit findings.
- Add a versioned, locked incremental admission matrix that separates identity,
  leaf-validation, real-code GLR, recovery, and stateful-scanner behavior using
  runtime evidence. It rejects full-parse fallback, authenticates both edit
  directions against fresh Go and C trees, and atomically publishes a
  machine-readable closure receipt only after every row passes.
- Real-corpus grammar parity can use a durable configurable corpus root and
  split-grammar corpus layouts without silently losing colliding basenames.
  Eligible-sample caps now apply on every generated-result path, committed
  floors reject over-cap rows, and the aggressive runner and floor share the
  same 30-sample limit.

## [0.38.0] - 2026-07-16

Incremental-correctness, full-parse-efficiency, and benchmark-hardening release.
Incremental parsing now preserves fresh-parse selection across GLR reuse, score,
cull, and retry edges; terminal materialization stops and multiline edits report
accurately; evidence-gated arena and merge policies reduce full-parse cost; and
authenticated static-C, fleet, and forest measurements are stricter.

### Performance

- The exact locked Odin grammar now caps first-pass arena preallocation for
  large ASCII token-sparse sources using a complete structural-density scan.
  This cuts arena allocation by 72% and full-parse time by 6% on the locked
  6.2 MB Odin test-vector witness with a byte-identical tree. Non-ASCII input
  fails open, while custom, same-name, stale-blob, and other fleet grammars
  retain the baseline policy.
- GLR boundary merging now rejects candidates with unequal cumulative scores
  before recovery-cost and graph-equivalence work. The order-balanced canonical
  real-Go benchmark improved by 4.3% geomean with unchanged parser work, tree
  identity, arena use, and stack maxima.

### Fixed

- Incremental parsing now preserves the configured bounded GLR width and
  rejects reused leaves whose stored parser state conflicts with the current
  shift. This prevents stale leaf context and over-aggressive two-stack
  pruning from changing selected trees on token-class and recovery edits.
- An incremental parse whose full-parse retry produces no strictly better
  tree now keeps its first-pass result instead of replacing it with a
  quality-tied fresh tree. This stops spurious `incremental_parse_full_retry`
  reporting on grammars whose intended trees contain ERROR productions and
  legitimately use the full GLR width.
- Reused subtrees now credit their cumulative dynamic precedence to the GLR
  stack score, so score-sensitive merge, cull, and result-selection decisions
  in an incremental parse match a fresh parse of the same structure.
- The GLR stack-cull trigger no longer depends on the arena class: incremental
  parses keep the same cull slack window as fresh parses, which previously
  pruned disambiguating forks early and changed selected trees on the
  early-newline canonical witness.
- A parse whose result materialization stops on a terminal condition (for
  example the memory budget tripping while the tree is being built) after the
  parser loop already accepted now reports that condition through
  `Tree.ParseStopReason` instead of `accepted`. Previously such a parse could
  return a sentinel full-span ERROR root labeled as a successful parse.
- Multiline tree edits now keep node byte and point ranges aligned with the C
  runtime across insertions, deletions, and replacements.
- Rewriter edits now reject reversed and out-of-source byte ranges instead of
  panicking while applying them.

### Tooling

- Report-mode fleet reduction now preserves closed-vocabulary
  `no_static_c_oracle`, `no_corpus`, and `no_corpus_files` shards as fatal
  closure findings in the combined artifact. Certification remains fail-closed,
  and report mode still rejects untyped, contradictory, or mixed oracle
  evidence.
- Add a diagnostic-only, authenticated Go/static-C GLR work-count contract for
  the locked real-Go `query_compile` fixture. A separate ordinary untagged Go
  child performs admission before tagged Go and fully static C diagnostic
  children report saturating direct action/pop/selected-tree counters and
  explicitly labeled representation proxies. Go counters attribute each
  `parseInternal` attempt to a logical retry rung, resolved cap mode, parser
  loop, and finalization; `accept_actions` is explicitly an action count, and
  aggregate counters must equal attempts plus the outside-attempt residual.
  Frozen retry-active and straight-LR witnesses pin the attribution semantics;
  a Go-only v3 supplement now records a bounded, attempt-local convergence
  frontier across reduction selection, post-reduce packing, boundary merge and
  cull, pending work, terminal acceptance, packed-root expansion, and final
  selection. It retains the first 256 events plus first rejection evidence,
  uses attempt-local decision IDs for target/candidate pairs, records scanner
  checkpoint identity at the current token election, detects partial merge
  mutations from exact semantic GSS writes, separates saturation from
  truncation, serializes no pointer identities, and
  leaves the shared v2 Go/static-C counter semantics unchanged. Authenticated
  v4 receipts bind both manifests and fail closed on malformed convergence
  payloads.
  Authoritative receipts require a clean Git source identity; compile from
  sealed private Go and C input snapshots; bind sanitized build/runtime
  environments; independently verify fixture, grammar, GLR-regime, span, and
  deep-tree identities; contain the complete cold static-C admission plus all
  repository, compiler, linker, identity, and linkage-verifier descendants in
  wall-bounded process groups; and publish atomically only after all rechecks
  pass.
- The static C oracle now recognizes locked grammar entry points declared with
  either C's empty `()` or `(void)` parameter spelling, restoring artifact
  construction for SCSS while rejecting genuinely parameterized near misses.
- The authenticated fleet scoreboard now times fresh full parses against a
  per-language, fully static executable built from the locked upstream runtime
  and grammar sources. Every selected file requires matching static/cgo deep
  tree digests, and reduction fails closed on missing or mixed oracle identity,
  dynamic linkage, source/flag drift, or legacy incremental axes. Deep dumps
  and cgo admissions use iterative cursors with independent wall bounds; C
  failures retain bounded Go evidence without fabricating ratios. Each file
  measures one whole Go block and one whole static-C block, alternating their
  order across file ordinals; compiler/linker absolute paths and executable
  hashes are part of the serialized protocol. The reducer preserves complete,
  typed C-oracle failures as authenticated closure
  failures while rejecting untyped, generic, or incomplete evidence. Admission,
  parser, transport, digest, measurement, and protocol failures now have a
  closed serialized status vocabulary. Content-keyed
  static executables are atomically installed with build-key/artifact-hash
  manifests only after a post-link recheck of the captured compiler, linker,
  pinned source trees, and every compiled source hash; cache hits repeat that
  check before use, and unstable inputs are recaptured once before failing.
  Shards execute a reverified private artifact snapshot, and per-language
  wall/RSS stops kill the entire child process group. The budget and status
  tools read both schema generations while keeping v1
  historical ratchets separate from v2 full-only hard-gate verdicts.
- Forest-routing performance screens now require fresh order-balanced
  confirmation before promotion. Immutable content-addressed trial, run-config,
  cohort, and index receipts bind the selected head to the recorded host
  fingerprint, image, and single-CPU resource configuration. Failed or
  drifting attempts remain unpublished: the runner executes the recorded image
  digest, verifies the created container identity, and reauthenticates every
  corpus read by manifest size and SHA-256 before timing. The reducer requires
  locked-C coverage for every routed path, emits explicit A+B or A-B-B-A
  plans, pools reverse-order evidence, and keeps possible C-oracle corrections
  review-required. Repeated trials release every tree on success and negative
  paths, complete the full corpus before the next sweep, and remain isolated
  to one language per container.

## [0.37.0] - 2026-07-14

Full-parse benchmark-integrity, forest-certification, and GLR-performance
release. Publication now uses one locked static C oracle and authenticated,
forking real-Go fixtures; authenticated C-first fleet evidence gates automatic
forest routes; general multi-stack work is reduced; and high-level highlight
and tag parsing can be bounded. The 206-grammar curated structural-parity
milestone remains banked.

### Added

- Highlighter and tagger construction now accept parser timeout options, and
  their byte-oriented incremental APIs have strict variants that return the
  partial tree with `ErrParseStoppedEarly` while skipping query execution.

### Changed

- `ParseForestExperimental` now reports only a tree produced by the
  experimental forest parser. A forest decline returns `nil, false` with
  `ForestDeclineInfo` diagnostics instead of silently substituting a
  production-parser result, so callers can measure and certify forest routing
  without mistaking fallback work for a forest success.

### Tooling

- Canonical real-Go benchmark admission now ratchets each fixture's multi-stack
  runtime regime and required syntax coverage across Go, cgo, and static-C
  preflights. Publication samples fail closed if a fixture drifts back toward a
  straight-LR control workload.
- The authenticated real-corpus source for Git rebase fixtures now uses the
  grammar repository's committed highlight corpus, so performance and forest
  audit manifests cover all 206 languages.
- One-language forest audit shards now revalidate only their selected corpus
  checkout and files while retaining the complete manifest identity, avoiding
  a repeated fleet-wide authentication pass for every isolated container.
- Forest eligibility sweeps now share an authenticated, revision-pinned corpus
  manifest between production and C-oracle lanes. Per-language Docker runs
  verify source checkout identity and exact file hashes, compare complete trees
  including anonymous children, points, flags, and fields, and emit strict
  resumable result shards for deterministic fleet reduction. Generated corpus
  files may be untracked only beneath the lock-declared
  `.gts-extracted/<language>` directory; tracked changes and untracked files
  elsewhere still fail authentication. The production lane separately times
  and verifies the actual forest-enabled automatic route on every file, so
  promotion requires exact routed parity and a net wall-time improvement after
  production fallbacks, not merely a fast forest attempt.
- C-first forest screening now terminally records `no_forest_coverage` when a
  complete authenticated C-oracle shard declines every file without timing
  out. The reducer skips the potentially expensive production lane for that
  non-promotable class while leaving missing and timeout-ambiguous evidence
  incomplete.
- Forest manifests, real-corpus benchmarks, and corpus inventory now share one
  file-selection policy for lock matchers, registry extensions, and canonical
  extensionless filenames such as `go.mod`. This lets authenticated manifests
  cover every lock entry that has an eligible source while keeping explicit
  lock matchers authoritative.

### Performance

- Automatic forest routing now covers the exact checked-in AWK, KDL, and
  Uxntal grammar artifacts after authenticated corpus gates found zero
  forest/C-oracle divergence on accepted forest files, zero
  routed/production divergence on every file, and an aggregate route wall-time
  win. The opt-in is attached through blob-identity runtime profiles, so
  same-name custom and adapted grammars remain on the conservative production
  path.

- Multi-stack DFA token elections now scan each unique active parser state once
  and reuse that result while scoring candidates, instead of rescanning the
  state for every candidate. The authenticated real-Go matrix avoided
  80-84% of those repeated scans and improved full parse by 3.5-9.8% across
  four fixtures, with unchanged parser shape, arena bytes, allocations, and
  exact 25/25 strict Go parity.
- GLR merge, hashing, shape, equivalence, and recovery-trace helpers now pass
  stack descriptors by pointer instead of repeatedly copying the 104-byte
  values. The authenticated real-Go matrix improved by 3.54% geomean, with
  every fixture improved or statistically unchanged and identical arena,
  token, stack, iteration, node, depth, and normalization counters.
- Fresh full parses now share the parser's existing no-error-payload proof with
  GSS merge and C-recovery cost selection, avoiding recursive graph and subtree
  walks until an `ERROR`, `MISSING`, or inherited error is actually
  constructed. Paired runs of a 148 KiB clean Java witness improved by 43-47%
  with unchanged full-span acceptance; isolated Go, Python, and Swift corpus
  parity remained exact, and incremental/reuse parses retain the conservative
  path.
- Conflict-reduction frontiers now reuse one fixed-table lookup when reading
  and updating the `forked` and `seen` flags for a reduction key. Paired runs
  of a 707 KiB clean Dart witness improved by a further 6-13%, with fresh and
  incremental C parity unchanged.
- Automatic forest dispatch for the exact built-in JavaScript grammar now
  limits the speculative forest phase to 128 MiB while preserving the caller's
  full budget for the production fallback and for explicit forest parsing. A
  20,784-file locked-corpus comparison preserved every tree, byte range, span,
  error, and stop outcome while reducing aggregate parse time by 3.09%,
  allocated bytes by 2.96%, and peak RSS by 19.02%. Caller-provided, modified,
  and same-name grammars retain the existing full-budget behavior.

### Fixed

- Accept pinned C-oracle checkouts whose raw tracked bytes match the locked
  commit even when an upstream `.gitattributes` rule makes Git report a fresh
  clone as modified, while continuing to reject real byte, mode, and untracked
  changes.
- Real-Go benchmark fixture admission now drains its arena-pool state after
  validation, and the arena GC-retention regression establishes its own clean
  pool boundary instead of measuring memory retained by earlier tests.
- C-oracle forest audits now compare wide syntax nodes with a linear tree
  cursor, avoiding quadratic indexed-child walks while retaining exact fields,
  spans, flags, anonymous children, and child order.
- Forest audit timeouts now stop the parser synchronously instead of leaving
  abandoned parse goroutines to contend with later files in the same shard.
- Go/C benchmark admission now uses the same locked upstream runtime and Go
  grammar as structural parity, fingerprints the `-O2` C artifact, and times
  immutable, clean, forking real-Go fixtures with symmetric tree lifecycles.
  The generated 500-function source remains a straight-LR regression control;
  its former 1.895x headline and 29% materialization decomposition are
  withdrawn because the C lane used a different grammar and the source never
  exercised the multi-stack path. A checked-in strict receipt driver now
  reproduces the pinned-core Go-C-C-Go schedule; both C transports reject dirty
  pinned-source caches, and the static lane snapshots each input once before
  identity, parity, and timing checks. The first complete publication receipt
  establishes the corrected full-parse baseline at 5.481673x C by equal-fixture
  geomean and 6.313799x C for the fixed-suite sum of medians, with per-fixture
  ratios from 4.639849x to 6.513909x.

## [0.36.0] - 2026-07-13

Parser recovery, recurring-work, grammar-contract, and browser-runtime release.
C-recovery elections and retained memo invalidation reduce fixed overhead;
retry selection and generated-language provenance are stricter; and the browser
runtime gains persistent incremental documents plus reproducible selected-
language bundles for Go and TinyGo.

This release supersedes v0.35.0. That tag was published from incomplete
ancestry; its browser-runtime changes have been reconciled here with every
change on the current main line. The v0.35.0 tag remains immutable so existing
Go module downloads continue to identify one source revision.

### Added

- The browser runtime now supports persistent UTF-16 documents through
  `open`, `update`, `close`, and `queryDocument`. Updates compute a
  surrogate-safe minimal edit, reuse the prior parse tree, and run highlights,
  tags, and bounded queries over the same retained tree while leaving the
  existing stateless parse, query, and highlight APIs intact.
- `cmd/wasmassets` now emits reproducible single-language browser bundles for
  either the Go or TinyGo WebAssembly compiler. Bundles contain the external
  grammar blob, highlight and optional tags queries, the matching compiler
  bootstrap, and a manifest with compiler identity and SHA-256 digests; the
  runtime build is restricted to the selected grammar's tables, registry, and
  scanner support instead of embedding the full grammar fleet.

### Performance

- Linearized C-recovery strategy-1 elections with reusable cursor and dedupe
  scratch. Against exact current main on the pinned quiet core, KDL recovery
  improved 19.26% with 29.39% fewer bytes and 30.83% fewer allocations, while
  the tiny-clean control remained statistically unchanged.
- Reused C-recovery node memos now use generation invalidation instead of
  clearing a retained 16K-entry cache on every parse. On the pinned quiet
  host, recovery-primed KDL tiny-clean parses fell from 31.98 microseconds to
  13.46 microseconds (57.93%), while alternating error/clean parses improved
  by 3.06%, with unchanged bytes and allocations per operation.
- The exact built-in Meson grammar now skips the redundant accepted-error
  retry ladder only for sources of at least 2 KiB. A locked 1,549-file corpus
  certification preserved complete and structural trees across all 28
  eligible error-bearing files and cut their aggregate one-pass parse time by
  78.04% (1.410s to 0.310s). Smaller inputs keep the generic retry ladder,
  including seven witnesses where retrying changes the selected tree.
- The exact built-in Enforce grammar now reuses a certified complete
  accepted-error widened result instead of repeating it with recovery enabled
  on sources of at least 128 KiB. Locked full-tree, structural, and semantic
  runtime checks remained exact; count-10 runs cut playerbase time by 28.01%
  and itembase time by 7.31%, with corresponding allocation reductions and an
  unchanged clean control.

### Fixed

- Browser runtime results are now assembled as explicit JavaScript objects and
  arrays, avoiding TinyGo's primitive-only `syscall/js.ValueOf` path while
  preserving the richer structured-tree and bounded-query wire formats.
- The real-corpus Docker runner now forwards `REAL_CORPUS_ONLY`, allowing a
  reproducible single-language run without switching to a different wrapper.
- HTML range normalization no longer extends already-closed child elements
  across trailing trivia to an enclosing end tag; genuinely unclosed recovered
  element chains retain their C-compatible range extension.
- Full-parse retry selection now preserves an accepted error tree when a later
  retry stops early, instead of replacing it with a farther provisional tree.
- Grammargen-owned Go, Regex, and Swift blobs now share one registry
  provenance contract, and ts2go's Go regeneration hint uses the safe
  `grammargen emit go` command without LR splitting.
- Fleet scoreboard reduction now canonicalizes hard-gate finding order and
  records clean reducer provenance separately from immutable measurement
  provenance, allowing later reducer fixes to authenticate historical shards
  without weakening tamper checks.

## [0.34.0] - 2026-07-13

Forest-routing performance and compatibility-hygiene release. Automatic
dispatch now avoids five language paths that consistently discarded their
forest result, while confirmed-dead C, C++, and Rust compatibility walks are
removed after full-corpus verification.

### Changed

- Automatic forest dispatch no longer speculates through Beancount by default.
  The exact four-file clean corpus produced no forest return, so every parse
  paid for a discarded forest before production. In matched automatic-parser
  scans, removing that retry cut the 347 KiB witness from 30.255x to 2.130x C
  and the corpus aggregate from 27.621x to 2.088x, eliminating the only ratio
  above 10x while returning the same accepted, full-span, error-free trees.
  Explicit forest experiments and Beancount's certified recovery policy remain
  available.
- Automatic forest dispatch no longer speculates through Org or Vimdoc by
  default. Representative clean witnesses produced no forest return and
  returned the exact production tree after declining at EOF. Production-only
  routing cut fresh parses by about 97%; on reused parsers it also removed the
  repeated 97% Vimdoc penalty, while Org's bounded decline memo had already
  made warm parses production-like. Explicit forest experiments and both
  certified recovery policies remain available.
- Automatic forest dispatch no longer speculates through Fish or Racket by
  default. Two locked clean witnesses per language produced no forest return
  and returned the exact production tree after declining at EOF. Removing the
  discarded attempt cut fresh Fish parses by 90-95% and fresh Racket parses by
  94-95%; reused 234-248 KiB witnesses improved by 94-96%, while smaller warm
  parses were already protected by the bounded decline memo. Explicit forest
  experiments and both certified recovery policies remain available.

### Removed

- Five confirmed-dead C/C++ post-parse compatibility passes, found by a
  full-corpus census (c: ~974 files from git/git; cpp: ~71 files from fmt;
  each parsed both clean and truncated to the first 55% of every second file,
  through the production C token-source backend, not the generic DFA lexer):
  pointer-assignment precedence rewriting (a full-tree postorder walk on
  every c/cpp parse); collapsed-keyword-children restoration (a second
  full-tree walk on every c/cpp parse, covering the null/type-qualifier/
  storage-class/noexcept/lambda-default-capture rule families); both
  preprocessor-directive-shape sub-rewrites (whitespace-separated
  function-macro reshape and directive-range extension); the
  declaration-bounds and variadic-ellipsis handlers from the fused
  declaration/variadic walk, plus their now-exclusive comment-scan helper
  cluster; and the top-level-item-wrapper collapse (also checked for any
  error/recovery path that could construct a visible `_top_level_item` node
  before cutting; none found, including on the truncated/error-inducing
  corpus phase). Zero rewrites were observed across every corpus file in both
  phases for all five passes. The fused walk itself remains — its builtin
  primitive-type-identifier promotion and preprocessor newline-span extension
  handlers are confirmed live and unaffected. `normalizeCTranslationUnitRoot`
  and the typedef-struct error-recovery branch remain untouched pending their
  own repros. Net ~575 lines removed from `parser_result_c.go` (~944
  including pruned dead-pass-only tests); verified byte-identical
  (S-expression and span hashes) across the entire c and cpp corpora (1,045
  files), and both the root package's production-backed
  `BenchmarkCPPConditionClauseAmbiguityDFA` and the grammars package's
  `BenchmarkParse_C` show a statistically significant ~13-27% drop in parse
  time and 26-50% drop in allocations per parse from dropping the two
  full-tree walks.
- Rust's collapsed named-leaf-children compat pass
  (`parser_result_rust_recovery.go`), a full-tree walk gated on the source
  containing `true`, `false`, `..`, or `;` — a gate that opens on essentially
  every real Rust file. A full-corpus re-verification (all 37,127 `.rs` files
  under the rust corpus, parsed both clean and truncated to 55% on every
  second file — 55,691 parses total) recorded 130,018 gate fires and zero
  rewrites. The companion candidate, Rust's dot-range-expressions walk, was
  re-verified the same way and found live (3,030 real rewrites on the same
  corpus, the simplest case being a bare `..` full-range slice index such as
  `s[..]`) and is therefore kept untouched. Net 228 lines removed; verified
  byte-identical (S-expression and full node-span dumps) on 30 real Rust
  files spanning size and content (macro-heavy, `..`-heavy), with the Rust
  parity suite, including the dot-range-motivated weird-expressions fixture,
  unaffected.

## [0.33.0] - 2026-07-13

Recurring parser and forest performance, recovery allocation, compatibility,
and lifecycle-hygiene release. Warm parsers avoid repeated stable forest
declines and oversized runtime-record copies, missing-shift recovery reuses
parser-state chains, exact bundled C# blobs skip redundant post-parse work,
arena and browser-WASM lifetimes are tightened, and confirmed-dead
compatibility code is removed.

### Performance

- Missing-token recovery now materializes each parser-state chain once per
  attempt and reuses pointer-free state buffers across candidate simulations,
  instead of rebuilding and allocating the same deep chain for each fallback.
  On the pinned 163 KB C++ recovery witness this reduced full-parse time by
  28.4%, allocated bytes by 79.9%, and allocation count by 41.1%, while
  preserving the accepted full-span error tree and parser-runtime counters.

### Changed

- Automatic forest dispatch now remembers stable semantic declines for a small,
  bounded set of unchanged sources and routes warm recurring full parses
  directly to the production parser after exact source verification. Explicit
  forest experiments remain uncached; resource-, timeout-, cancellation-, and
  work-cap-driven declines are never remembered. On the pinned Make witnesses
  this removed repeated discarded forest construction, cutting both a clean
  18 KiB parse and the 129 KiB
  error-bearing parse by about 90% while preserving the returned production
  trees and runtime status.
- Automatic forest dispatch no longer speculates through CSV by default. An
  all-23-file corpus census produced no forest fast-path returns: the two
  largest files exhausted the forest budget and the other 21 conservatively
  declined at EOF before repeating the parse in production. Explicit forest
  experiments and CSV's certified recovery policy remain available.
- Internal retry and tree-selection decisions now inspect each tree's stored
  parse-runtime record in place instead of repeatedly copying the 2,928-byte
  public snapshot. `Tree.ParseRuntime()` remains a value API with its live
  final-child-counter overlay unchanged. In pinned recurring benchmarks this
  reduced the five-byte KDL floor by 11.2% and Java's registered token-source
  path by 14.9%, with unchanged bytes and allocations per operation; the
  standard full/incremental benchmark trio remained neutral.
- Tiny fresh full parses now reserve a source-scaled logical range from the
  existing physical entry-scratch slab, avoiding repeated clearing of unused
  stack entries while preserving incremental and large-source reservations.
- The exact bundled C# grammar now advertises native result compatibility for
  `notnull` constraints, Unicode identifier spans, scoped-lambda statements and
  blocks, and LINQ query expressions, allowing the runtime to skip the five
  corresponding post-parse passes for that certified blob. The implementations
  remain quarantined conservative fallbacks for legacy blobs, grammargen output,
  caller-built languages, and overrides unless those artifacts explicitly carry
  the relevant append-only capability bits. Runtime-profile attachment remains
  pinned to the exact blob SHA; attaching scanner support by name alone does not
  certify native result shapes. The skip is backed by the 1,700-file C# corpus
  sweep, the original motivating fixtures, an 84-program LINQ battery, explicit
  capability round-trip and identity gates, and direct embedded-grammar Unicode,
  scoped-lambda, and LINQ regressions.

### Fixed

- Oversized full arenas rejected by `Release` now clear every matching stale
  checkout reference from the pool's unused backing slots instead of remaining
  reachable after rejection. Ordinary checkout and successful repooling remain
  unchanged.
- The browser runtime now releases every parse tree returned by both the
  runtime and grammargen WASM bridges. Runtime queries stream through a
  500-match cursor limit instead of materializing an unbounded result before
  slicing it, and structured trees report `truncated` only when the 20,000-node
  payload limit actually omits a node rather than when a tree exactly fills it.
  Empty sources now return stable empty results from both bridges instead of
  dereferencing a nil root; unexpected nil parse results and language handles
  now fail safely at the browser boundary.
- Blob-loaded browser languages now retain any registered token-source factory
  for parsing, queries, and highlighting, matching the certified registry path
  used outside WASM. Grammar-subset builds now attach those factories regardless
  of file-init order and no longer revive Go's intentionally disabled scanner
  path. Reloads publish a language and highlighter together, clear stale
  highlighters when no query is supplied, and leave the prior pair intact when a
  replacement query is invalid.

### Removed

- Four confirmed-dead post-parse compatibility passes, found by a full-corpus
  census (every source file under go ~11.4k, java ~9.2k, ruby ~3.5k, and
  haskell ~2.2k real-world corpora, each parsed both clean and truncated to
  the first 55% of every second file): Go's dot-leaf walk, a second full-tree
  DFS on the canonical parse lane gated only on the source containing `.`,
  whose sole job was synthesizing the anonymous `.` child under an
  already-childless `dot` import-alias node — the DFA emits that child
  directly on every real parse, so the walk visited every node in the tree
  without ever performing its one rewrite; Java's entire compat block
  (primitive-type token collapse, dotted-assignment-declaration reshape, and
  recovered-program-root retagging — `parser_result_java.go` in full); Ruby's
  `then`-span start walk; and three of Haskell's seven compat passes
  (collapsed named-leaf children, `let`-bound local-binds start, and
  quasiquote start). Zero rewrites were observed across every corpus file in
  both phases despite substantial visit counts (Java's primitive-type check
  alone matched 82,504 times). Haskell's remaining four passes — including
  `normalizeHaskellRootImportField`, which rewrites on nearly every parse —
  and Ruby's top-level module-bounds fixup, which fires on truncated input,
  are unaffected and untouched. Net ~886 lines removed; verified
  byte-identical (S-expression and span hashes) on real Go, Java, Ruby, and
  Haskell samples, and a benchmark variant of the canonical Go parse
  benchmark with a single added period (the synthetic canonical benchmark
  source contains no `.` bytes and so never exercised the removed walk's
  gate either way) shows a statistically significant ~37% drop in
  allocations per parse from dropping the walk.

## [0.32.0] - 2026-07-13

Browser query/structured-tree, compatibility cleanup, clean-parse recovery
performance, and Bash parity-coverage release. The runtime WASM target now
exposes structured parsing and queries with both UTF-8 and UTF-16 spans. Six
dead JavaScript/TypeScript rewrites are gone, clean parses avoid recursively
re-summing C-recovery subtrees, and Bash's committed real-corpus floor is backed
by an executable witness.

### Added

- The browser-focused WASM runtime now parses JavaScript strings through the
  UTF-16 parser entry point and exposes structured JSON trees and bounded query
  results for languages loaded through `loadBlob`. Tree nodes and query
  captures carry both canonical UTF-8 byte offsets and JavaScript UTF-16
  code-unit offsets; node- and match-count limits report when results were
  truncated.
- A dedicated bash real-corpus parity witness (`grammargen/bash_parity_test.go`),
  mirroring the existing Python witness. Previously the bash entry in the
  `real_corpus_parity_floors.json` v3 floor file (25 eligible / 9 no-error / 6
  S-expression / 6 deep) was phantom: the generic real-corpus loop skips any
  grammar without a `jsonPath`/`path`, and bash had neither and no dedicated
  test, so nothing exercised that floor. The new witness grammargen-compiles
  the locked tree-sitter-bash grammar and reproduces the floor exactly,
  skipping (not failing) when the corpus is not seeded locally. A companion
  reducer test pins `echo ${x}` and `echo ${#x}` as working controls and
  `echo ${x:-y}` as a self-healing known-defect witness for the underlying
  grammargen expansion-suffix table defect.

### Removed

- Six confirmed-dead post-parse rewrite passes from the fused JavaScript/
  TypeScript/TSX compat walk: statement-keyword (`if`/`while`) leaf retype,
  `empty_statement` semicolon retype, `existential_type` collapse, call-
  precedence reshape, and unary- and binary-precedence rotation, plus their
  exclusively-owned helpers and an already-unreachable standalone fallback
  path from an earlier compat-tier sunset. A census over roughly 23 MB of
  real JavaScript/TypeScript/TSX (including undici.js and TypeScript's own
  checker.ts, parser.ts, and utilities.ts), the original regression corpus
  that added these passes, and independent adversarial precedence chains
  found zero rewrites from any of the six; later grammargen table fixes
  already produce the correct tree shape directly, so the passes had become
  dead weight on every JS/TS/TSX parse. The compat pipeline's two remaining
  live fixups (top-level object-literal reinterpretation and trailing-
  continue-comment reattachment) and the memory-budget stop-polling on the
  surviving walk are unaffected. Net ~1,100 lines removed; verified byte-
  identical (S-expression and spans) on real JS, TS, and TSX samples.

### Performance

- Clean full parses now make C-recovery condense summaries demand-driven.
  Before any error-bearing payload exists, condense charges only the exact
  open-recovery costs instead of recursively re-summing clean subtrees; visible
  node counts are evaluated only by the unequal-cost comparison branch that
  consumes them. On a 4,096-entry generated Go composite witness this reduces
  a full accepted parse from 18.72 s to 93 ms and one-shot maximum RSS from
  1,388,988 KiB to 498,240 KiB, with the same full, error-free S-expression.
  The exact 726,532-byte Go manifest witness now completes in 0.39 s with full
  span and no error. The standard full/incremental/no-edit benchmark trio and
  KDL recovery benchmark retain unchanged allocations and show no candidate
  regression.

### Fixed

- Missing extra shifts, including the C-family zero-width missing-token case,
  and alias-prefixed recovered-suffix resyncs now mark error-bearing content
  before the next condense pass. This keeps the clean-subtree proof exact
  without adding node metadata, parser caches, or language-specific fast paths.

## [0.31.0] - 2026-07-13

Memory containment, Python parity, and authenticated fleet-reporting release.
Failed forest attempts now apply the parser's runtime heap and system memory
guard, and discarded forest GSS slab batches no longer remain live behind the
retention cap. Python real-corpus S-expression and deep parity return to 25/25
after removal of a misfiring compatibility fold. Fleet reducers can publish
valid failing scoreboards while certification remains blocking.

### Changed

- The authenticated performance-shard reducer now distinguishes reporting from
  certification. `report` mode publishes a recomputed PASS or FAIL fleet board
  without turning valid failure evidence into a reducer error; the default
  `certify` mode publishes the same artifact before blocking on a combined
  FAIL. Exact stored shard gates may be PASS or FAIL, while missing, stale, or
  malformed evidence still fails closed.
- The Docker parity wrapper accepts a fixed `--hostname` and records it in run
  metadata so one-language containers on the same physical benchmark host can
  produce a consistent authenticated host identity.
- The tier-scan guide now describes full-corpus tier publication as a staged
  release gate instead of claiming the 33 GB scan runs for every release. The
  committed tier board remains explicitly unreleased until a fresh full scan
  is intentionally published.

### Performance

- Forest parsing now applies the parser's existing runtime heap and system
  memory guard in addition to its node-arena budget, covering GSS slabs and
  alternative indexes before a failed forest attempt falls back to production
  parsing. On the default-budget JavaScript Poppler witness, this moved the
  forest decline from byte 1,147,865 to roughly byte 360,000, cut combined
  elapsed time from 5.313 s to 2.610 s, total allocation from 2.450 GB to
  1.289 GB, and maximum RSS from 1,961,948 KiB to 1,012,296 KiB while
  preserving exact stopped-tree hashes. Final-diff successful-forest B-C-C-B
  timing remained neutral (+1.4%, p=0.142), with a small measured allocation
  cost (+0.28% B/op and +0.01% allocs/op). This bounds a failed attempt;
  Poppler still reports the ordinary 512 MiB production fallback stop and is
  not claimed to complete within that policy.

### Fixed

- Removed `foldPythonTrailingSelfCallIntoNestedFunction`, a Python
  compat-normalization heuristic that spuriously folded a same-named trailing
  call into a preceding nested function's block when that function's body
  ended in a dangling `;` before a dedent. Raw parser results already matched
  the reference; only the post-parse fold diverged. Python real-corpus
  S-expression and deep parity both improve from 20/25 to 25/25.
- The pooled forest GSS slab now clears outer batch references discarded by
  its 32 MiB retention cap. On the 3,447,275-byte JavaScript Poppler witness
  under the default 512 MiB parser budget, this reduced one-GC live heap from
  1,475,142,360 to 608,167,688 bytes and eliminated all 866,975,744 bytes of
  hidden tail references while preserving the 33,488,896-byte warm prefix and
  identical parse output. Peak RSS remained effectively unchanged because the
  batches are still allocated before release; a recurring successful-forest
  benchmark was neutral (2.627 ms to 2.636 ms, p=0.947, n=20) with unchanged
  bytes and allocations.

## [0.30.0] - 2026-07-12

Recurring-parser performance and fleet-measurement integrity release. Reused
parsers now invalidate the 16,384-entry clean-zero front cache by epoch,
reducing recurring one-byte KDL and JSON wall time by 33.10% and 35.67% with
unchanged allocation counts while the primary benchmark trio remains neutral.
Certified runtime profiles retain the required D, Groovy, and C# retry
policies. The real-corpus tooling now distinguishes clean, error-bearing, and
stopped parses and can reduce revision-pinned one-language checkpoints into a
single authenticated fleet report without rerunning parsers.

### Changed

- The real-corpus performance scan now supports resumable one-language shard
  campaigns with a blocking merge-only reducer. New scoreboards record their
  repository revision and clean-source state; reduction requires exactly one
  quiet, unexcluded, hard-gate-clean shard per authenticated lock language at
  one revision, host/runtime identity, and measurement configuration, then
  recomputes the fleet aggregates,
  clean/error split, coverage, and hard gate before emitting authoritative
  JSON and Markdown.
- The real-corpus Go/C performance scoreboard now classifies each full parse
  as clean, error-bearing, or stopped outside the timed path, and reports
  per-language clean/error counts, timing totals, ratios, stopped subsets, and
  error share. Existing coverage and zero-cliff gates remain unchanged.
- Large D and Groovy accepted-error parses now retain their certified initial
  stack ceilings through exact-blob runtime profiles, and C# skips its
  redundant first same-stack merge retry through the same fail-closed profile
  mechanism. Caller-adapted grammars, incremental fallbacks, and explicit
  diagnostic overrides retain the conservative retry ladder.

### Performance

- Reused parsers now invalidate the pointer-free clean-zero front cache by
  advancing its epoch instead of clearing all 16,384 entries between parses.
  On recurring one-byte KDL and JSON witnesses this reduces wall time by
  33.10% and 35.67%, respectively, with unchanged allocation counts; the
  materialized full-parse, single-byte incremental, and no-edit benchmark trio
  remains neutral.

### Fixed

- `real_corpus_inventory --require-corpus-sources` now rejects pinned corpus
  checkouts that contain no benchmark-eligible regular files matching the
  language's source policy. Inventory and benchmarks share traversal and
  subdirectory validation, so invalid paths and scan failures are reported
  instead of allowing an empty language sweep to appear complete.

## [0.29.0] - 2026-07-12

Recurring-parser performance and compatibility-cleanup release. Repeated small
parses now pay for the current input rather than stale pooled capacity, forest
parsing returns its token-source resources promptly, and the common small
forest indexes stay inline. On the recurring C# and CSS witnesses, wall time
falls 34–35% and allocated bytes fall 81–82%; across a selected six-language
family, geomean wall time falls 3.18% and bytes fall 6.81%. The primary Go
benchmark trio remains neutral while full-parse bytes fall 18.68%.

### Performance

- Forest parsing now acquires the parser's reusable DFA token source and closes
  it at the parse boundary. This removes the recurring scanner buffer and
  source/lexer allocations while preserving external-scanner checkpoints in
  the result arena before the source is returned.
- `gssForestIndex` and `forestAlternativeIndex` keep their common small sets in
  inline storage and allocate spill space only when needed. Insertion order,
  lookup identity, and cache reset semantics are unchanged.
- Full parses size their initial GLR entry reservation from the current source
  length. A large prior parse can no longer make every later tiny parse clear a
  retained 65,536-entry slab; incremental reuse keeps its established capacity.
- Visible alias targets are precomputed once per parser, removing repeated
  language-table scans during result normalization without treating hidden
  aliases as visible terminal leaves.

### Added

- Recurring tiny-input benchmarks for JSON, C#, CSS, Java, and the DFA parser
  expose warm-pool lifecycle and fixed-overhead regressions directly.

### Changed

- Trailing-span compat shims for Caddy, Comment, Fortran, Nim, Pug, and RST
  moved onto a single data-driven `trailingSpanRules` table
  (`normalizeResultTrailingSpanCompatibility`) instead of six separate
  `runLanguageResultCompatibility` switch arms and four hand-written wrapper
  functions (`normalizeNimTopLevelCallEnd`, `normalizeCommentTrailingExtraTrivia`,
  `normalizeRSTTopLevelSectionEnd`, `normalizeFortranStatementLineBreaks`).
  Each row names the language, the shared primitive it drives (extend the
  sole top-level child across a trailing line break, trim a trailing
  invisible extra-trivia child at the root, shrink a top-level child's end
  off trailing whitespace, or extend a statement across the line break
  before its next sibling), and that primitive's node-kind parameters, so
  adding another language to any of these four span shapes is a table row,
  not a new function. Fortran's statement-vs-sibling pass is generalized
  from a hardcoded `program`/`program_statement` walk into
  `extendChildLineBreakBeforeNextSibling`, parameterized the same way.
  The four wrapper functions being retired were already thin call-throughs
  into shared primitives from an earlier consolidation, so this pass nets a
  modest line increase (the switch/wrapper boilerplate shrinks, but the new
  table and its per-row rationale comments are larger than the code they
  replace) in exchange for one auditable, greppable rule set instead of six
  scattered dispatch sites.
- The go.mod, Dart, and C repetition-conflict compat-tier helpers
  (`gomodRepetitionShiftConflictChoice`, `dartRepetitionShiftConflictChoice`,
  `cRepetitionShiftConflictChoice`) are retired in favor of certified,
  blob-SHA-pinned `ConflictPolicies` rows in `grammars/runtime_profiles.go`.
  C's rule recurs at thousands of table rows (reduce-symbol identity alone,
  not table position), so it is the first profile to use two new sentinel
  values, `ConflictPolicyAnyState`/`ConflictPolicyAnyLookahead`, matching
  every state/lookahead instead of one exact row. Dot's equivalent helper is
  retired outright rather than migrated: it was already dead code in the
  shipped dispatch path (dot never opted out of the engine-wide C
  repetition-skip fold, which already folds it with a flat parse stack), and
  reviving it as a live policy grew the LR parse-stack depth O(n) with
  statement count for no fork-count benefit. C#'s helper is left in place: it
  depends on the literal source text of a contextual keyword (`scoped`),
  which `ConflictPolicies`' state/lookahead-symbol matching cannot express.
## [0.28.0] - 2026-07-12

Containment closure and measurement-honesty release. The runtime memory
budget is now path-uniform across every public parse construction: the
parse loop, the Go compat walk, and the JS/TS fused compat walk all poll
the same budget and surface the same stop reason. On the quiet host, a
bare `Parse` of the Poppler witness (3.4 MB ambiguous JavaScript) stops
bounded at ~1.78 GiB peak RSS under the default 512 MiB budget — a path
that previously escaped accounting entirely — and completes clean under a
2 GiB budget. Error-recovery throughput improves ~18% on recovery-heavy
workloads, and the perf ledger tooling learns to separate clean-parse
from error-recovery throughput so tail-language ratchet rows stop
conflating the two.

### Performance

- The C-recovery per-subtree error-cost/visible-count memo
  (`cNodeErrorCost`/`cNodeVisibleSubtreeCount`) is now a fixed-capacity,
  pointer-keyed 2-way set-associative cache instead of a
  `map[*Node]cNodeMemoEntry`: warm CPU profiles of error-bearing parses on
  fleet-tail languages (kdl, uxntal) showed `runtime.mapaccess2_fast64` as
  the single hottest leaf, driven almost entirely by these two lookups.
  Every decision made by the recovery cost-competition machinery
  (`cRecoverStrategy1Election`, `cHandleError`, `cCondenseAndResume`, etc.)
  is unchanged — a cache miss simply falls back to the same full recompute
  as before. The cache starts small (matching the old map's practical
  per-parse footprint) and grows to its full working-set size only the
  first time a parse actually enters C error handling, so clean parses of
  recovery-capable grammars are unaffected (measured neutral-to-positive on
  the canonical Go workload). A synthetic KDL truncated/garbage-suffix
  recovery benchmark (`BenchmarkKDLRecoveryGarbageSuffix`) improves ~18%
  (83.2ms to 68.2ms median, p=0.002, n=6).

### Added

- A pointer-light tree measurement rig: benchmarks and a
  structure-of-arrays prototype (`pointer_light_measurement_test.go`,
  `pointer_light_soa_test.go`) that measure bytes-per-node, GC scan
  cost, and walk throughput for the current pointer-rich node layout
  against a contiguous index-based layout, plus a
  constructed-versus-final node census. These are the standing gate
  instruments for the frozen-tree store investigation.
- `parse_gap_report` and `parse_gap_correlate` now split each language's
  Go/C ratio by corpus-file policy: every sample is classified `clean`
  (Go tree has no ERROR nodes and did not stop early) or error-bearing, and
  the per-language ledger reports `clean_ratio`/`error_ratio` plus
  `clean_file_count`/`error_file_count`/`error_file_share` alongside the
  existing combined ratio. This keeps an error-dense tail-language corpus
  from making clean-parse throughput look artificially slow in ratchet
  decisions.

### Fixed

- The Go compat-normalization walk now honors the parse memory budget: it
  is skipped when the parse already carries a budget stop, polls the
  runtime budget at its existing walk stride, and surfaces the stop reason
  on the final tree via a sticky trip flag. Previously the walk ran
  budget-blind at result finalization and could balloon on recovered
  trees after the parse loop had stopped cleanly. Clean-parse trees are
  byte-identical. The JS/TS fused walk has the same blind spot (no stop
  polling at all) and is tracked separately.
- The JS/TS fused compat walk (and its unary/binary candidate-index
  rebuild) now carries the same containment as the Go compat walk above:
  it is skipped when the parse already carries a budget stop, polls
  timeout/cancellation/memory-budget at the same coarse, ~1024-node
  stride, and surfaces the stop reason via the same sticky trip flag.
  Previously this walk polled nothing at all — no timeout, cancellation,
  or memory-budget check of any kind — and could run to completion
  budget-blind regardless of tree size. Clean-parse JS/TS trees are
  byte-identical; no measurable regression on the canonical Go benchmark.

## [0.27.0] - 2026-07-12

Containment and canonical-parse-lever release. Memory-budget enforcement is
now layered (volume-triggered polling, in-merge checks, and an absolute hard
ceiling) so runaway parses stop instead of ballooning, while certified
bounded-overshoot witnesses still complete. Two independent hot-path levers
land together: single-stack raw-shape elision and supertype hidden-choice
collapse. Combined same-host receipt on the canonical Go workload: full
parse 12.25 ms to 10.91 ms — 2.14x to **1.89x** the C runtime measured in
the same session — with allocations unchanged (9 per full parse, zero on
both incremental lanes). The compat tier continues shrinking, the field-map
generation ceiling is lifted (Bash and Dart now carry real-corpus floor
rows), and parity floors are reproducible against lock-pinned corpora.

### Added

- Memory-budget containment is now layered: volume-triggered polling
  forces a real budget check whenever tracked arena growth exceeds 64 MiB
  since the last check (bypassing the iteration-count poll mask), the GLR
  stack-merge survivor loop polls the budget mid-grind, and a decoupled
  absolute hard ceiling (`GOT_PARSE_MEMORY_HARD_CEILING_MB`, default
  2048, 0 = off) stops runaway growth regardless of soft-budget
  overshoot tolerance. A bare-`Parse` giant-table witness now stops with
  `ParseStopMemoryBudget` at 2.6-4x budget instead of ballooning; the
  Poppler witness still completes full-span under its certified 2 GiB
  budget.

- A hard zero-cliff gate for nightly fleet perf sweeps, with a
  hard-gate-only mode on the perf-scan budget checker; the scheduled
  perf-scan gate is disabled in favor of the nightly hard gate.
- Runtime profiles for ASM (bounded stack retries), Haxe, Odin, and SCSS.
- Dedicated non-terminal alias-map parity coverage: derivation gates for
  Go, Swift, and Caddy mirroring the Lua gate, plus live-parse regression
  tests for each language's alias behaviors.
- `BENCH.md`: the canonical performance-claims page, including the first
  pinned quiet-host receipt for the corrected full-parse benchmark and a
  same-host C-baseline calibration (full parse 2.14x C on the canonical
  workload; incremental lanes orders of magnitude faster than the cgo
  binding path).
- `docs/compat-tier.md` documenting the C-faithful result-normalization
  tier and its retirement policy.
- A reproducible real-corpus floors workflow: corpora seeded at
  `grammars/languages.lock` SHAs (`scripts/seed_real_corpus_from_lock.sh`
  plus a committed seed manifest), opt-in ratchet regeneration, and floor
  artifacts captured in the mounted workspace.

### Changed

- Collapsed-named-leaf compat adapters for Kotlin, Hack, Dart, and Elixir
  moved onto the data-driven `resultCollapsedNamedLeafRules` table
  (previously data-driven for Ruby and Apex only): Kotlin's
  `identifier -> simple_identifier`, Hack's `true`/`false`/`null` literal
  wrappers, Dart's `super`/`this`, and Elixir's `nil` are now table rows
  instead of hand-written adapter functions. The table gained a `bySource`
  column so a row can pick the source-text-verified matcher
  (`normalizeCollapsedNamedLeafChildrenBySource`, needed when the
  collapsed span must be confirmed before a child is attached) instead of
  the plain structural one. Hack's dedicated compat file and switch arm
  are retired entirely (all three of its rules were table-eligible); Dart
  and Elixir keep their compat functions for unrelated rewrites but lose
  the adapter that only fired these rules. Net ~37 lines of per-language
  adapter code retired in favor of ~7 declarative table rows. Haskell's
  `wildcard -> "_"` was evaluated for the same migration but left
  in place: the anonymous token name `"_"` collides with the special
  query-wildcard sentinel in `Language.symbolByNameAndNamed`/`SymbolByName`
  (both short-circuit to `(0, true)` for `name == "_"`), so migrating it
  through the shared table resolves the child to `Symbol(0)` (EOF) instead
  of the real anonymous `_` token; OCaml, HCL, and Rust were left
  unmigrated too (OCaml's and most of HCL's rules need multi-candidate
  source disambiguation the one-parent/one-child schema doesn't represent,
  and HCL/Rust also fold their collapse checks into a single perf-tuned
  tree walk that per-rule table entries would fragment).
- Real-corpus parity floors regenerated against lock-pinned corpora:
  55 grammars, 851/1026 deep parity, including first-ever bash and dart
  rows; the docker wrapper's default skip list shrinks to OCaml only.
- GLR replay stacks use interned structural nodes, and GSS prefix
  aggregate caching and scratch retention are tightened.
- Certified full-parse retry passes are bounded, and redundant certified
  retries are skipped.

### Fixed

- grammargen field maps no longer emit one entry run per production:
  entries deduplicate by ProductionID (compaction fingerprints include the
  field set, so shared IDs always carry identical fields). This removes
  60-87% orphaned entries from the shipped grammargen blobs
  (go.bin 673 to 267 entries, swift.bin 2904 to 384) and lifts the uint16
  field-map ceiling that made Bash (65,536) and Dart (65,538) generation-
  fatal; both now generate and carry real-corpus floor rows. A regression
  test pins one-reachable-run-per-ID through the real compaction path.
- The Swift certified retry profile is re-pinned to the regenerated blob
  SHA (fail-closed certification behaved as designed).

### Removed

- Eight retired Python compat-normalization helpers and their orphaned
  tests (test-only since the combined single-pass source-flags path), and
  six test-only compat wrappers plus one dead Go range normalizer, with
  all remaining test coverage redirected to the live variants.

### Performance

- grammargen's hidden-choice passthrough table no longer excludes supertype
  symbols whose alternatives are all neutral-unary; Go's `_statement` and
  `_simple_statement` wrappers now collapse in the zero-allocation unary
  reduce path (1,500 wrapper nodes eliminated per canonical-workload parse).
  Only two bits change in the regenerated go.bin — parse tables, field
  maps, alias and supertype query tables are byte-identical, and supertype
  query predicates match by concrete descendant so observable trees and
  captures are unchanged. Canonical quiet-host full parse improves ~3.4%.
- Raw-shape capture and content hashing are elided while a parse has only
  ever had a single GLR stack and has not entered error recovery; capture
  resumes permanently at the first fork or recovery event. Shape-dependent
  tie-breaks are unaffected: elided prefix nodes are only ever compared to
  themselves (structural pointer-sharing), evidenced by forced-descent and
  recovery differential tests. Canonical quiet-host full parse improves
  ~4.8% with allocations unchanged (9/0/0 preserved).

- gssNode layout compacted to a 64-byte budget on 64-bit targets
  (pointer-backed extra links with uint8 count/cap, uint32 depth,
  aggVisValid bool), enforced by a size-budget test and a compile-time
  uint8 guard; transient GSS slabs are recycled after linear demotion with
  address-keyed caches invalidated and fingerprinted spine memoization
  preserved. Canonical quiet-host lanes are timing-neutral with
  allocations unchanged (9/0/0).
- Contiguous recovery cost calculation and recovery stack allocation are
  optimized; tree error state is cached and compat walk frames reused.

## [0.26.1] - 2026-07-11

Large-tree memory follow-up to v0.26.0. Exceptionally large completed full
parses can now release arena storage retained by discarded GLR alternatives
before returning to the caller. This patch does not change the public API.

### Changed

- Accepted fresh UTF-8 DFA full parses with unique arena ownership are copied
  into a right-sized arena when the retained arena is at least 512 MiB, the
  projected reclaim is at least 256 MiB and 30%, and the parser memory budget
  leaves enough headroom for both arenas during the copy.
- Compaction runs after retry selection, result normalization, and recovery
  resolution. Forest, incremental, included-range, borrowed-arena, deferred
  compatibility/checkpoint, and lazy final-child results remain unchanged.
- Final-tree cloning now preserves arena-backed field metadata and avoids
  empty external-scanner checkpoint lookups.

### Performance

- On the exact 3,447,275-byte JavaScript Poppler witness under a hard 2 GiB
  container, retained heap after GC fell from 862,803,056 to 409,862,040 bytes
  (-431.96 MiB, -52.50%) while preserving accepted error-free EOF output and
  exact Go/C S-expression and deep parity.
- The controlled full-parse, one-byte incremental, and no-edit incremental
  benchmark trio was statistically unchanged. The Poppler macro probe's
  elapsed time increased 7.98% and peak RSS increased 2.30%, so this release
  makes no full-parse latency or peak-RSS improvement claim.

## [0.26.0] - 2026-07-11

Parser-memory, registry-lifecycle, and build-hygiene release following v0.25.0.
It shrinks common node state, removes a per-parse ranking memo, and stops
returned trees from retaining parser-only shape overflow. This minor release
adds an exported diagnostic field; callers using positional `ArenaBreakdown`
literals must update them.

### Added

- `ArenaBreakdown.NodeFieldMetadataBytesAllocated` reports storage used by
  arena-backed node field metadata.

### Changed

- Node field IDs and field-source slice headers now live in bounded arena
  sidecars. Accessors preserve the previous shallow-copy and shared-backing
  semantics across parsing, normalization, cloning, and tree mutation.
- Documentation-only pull requests use an explicit CI scope gate so required
  checks resolve without running compile, race, parity, or performance suites.
- Language-authoring documentation now reflects forest fallback/recovery and
  the hard parse-action group overflow check.

### Fixed

- Extension grammar generation is now synchronized and memoized, including
  failures, so concurrent first access cannot race or regenerate repeatedly.
- `ParseFilePooled` replaces a cached parser pool when a same-name registry
  update supplies a different language instance.
- Parser-only raw-shape references and excess slab storage are reclaimed after
  final tree materialization, including both forest result paths. Parse-time
  arena accounting remains intact, and a bounded warm prefix is retained for
  reuse.

### Removed

- Removed unused internal forwarding helpers from GLR stack-entry comparison
  and performance-scan summarization.

### Performance

- Arena-backed field metadata shrinks `Node` from 144 to 104 bytes and removes
  245,966,616 bytes from the exact Poppler arena allocation while preserving
  exact structural parity.
- Current-arena node error ranks are cached inline instead of in an arena-wide
  map. The pinned full-parse benchmark improved from 7.813 ms to 6.750 ms and
  from 100 to 30 allocations per operation; incremental and query baselines
  were unchanged.
- Bounded raw-shape reclamation reduces the hard-2-GiB Poppler probe's retained
  post-GC heap by exactly 192 MiB with exact deep C parity. The controlled
  primary benchmark was statistically unchanged; peak RSS is not claimed as
  improved.

## [0.25.0] - 2026-07-11

Performance, memory, and runtime-hygiene release following v0.24.1. It makes
pending-parent field metadata compact and exact, removes retired zero-only
telemetry, and narrows redundant Java retry passes behind an exact-blob
profile. This minor release intentionally includes the exported diagnostic
telemetry removals listed below. It also re-certifies the exact Poppler witness
inside a hard 2 GiB envelope without claiming that JavaScript's throughput tail
is closed.

### Changed

- Pending-parent child entries now pack their full 16-bit field ID and field
  source beside the payload kind. Fielded parents use one 16-byte entry per
  child instead of a second sidecar entry, and materialization no longer
  reconstructs direct fields from grammar tables.

### Fixed

- Stack dedupe and GSS link merging now treat pending-parent hashes as coarse
  prefilters and recursively verify packed fields, field sources, and nested
  pending descendants. Missing arena context and excessive depth fail closed
  instead of allowing a hash collision to collapse distinct alternatives.

### Removed

- Removed the retired direct `no_alias` reduction-attribution lane from
  `ParseRuntime`, `ArenaBreakdown`, `PerfCounters`, and the Java/Python and
  parse-gap reports. The path has had no production producer since reductions
  moved to `all_visible` or `scratch_no_alias`; every exposed value was
  permanently zero.
- Removed two unexported transient-materialization wrappers used only by tests;
  tests now call the stop-aware implementations directly.

### Performance

- Java's exact built-in grammar profile keeps the initial 14-stack ceiling on
  large fresh parses whose first result accepts at EOF with an error. The
  cap-16 same-stack merge retry remains intact; only two proven-redundant
  cap-64 passes are suppressed, while overrides and incremental paths retain
  the conservative generic ladder.
- The exact 3,447,275-byte JavaScript Poppler witness now has a current-main
  receipt for no-error, S-expression, and deep C parity plus a 1,708,712 KiB
  hard-RSS run. Its full parse remains 3.50x C, so JavaScript stays pending on
  throughput and retained-node work.

## [0.24.1] - 2026-07-11

Performance-contract and repository-hygiene follow-up to v0.24.0. This patch
corrects the canonical full-parse benchmark before the long-tail optimization
campaign continues, banks focused Caddy and Kotlin wins with fail-closed
certification, and deletes superseded conflict and profiling machinery. It does
not change the v0.24.0 Poppler memory claim or declare the remaining fleet
performance tail closed.

### Fixed

- Caddy's SHA-pinned recovered string-literal repetition row now follows C's
  deterministic reduce after active recovery ends, preventing a quadratic GLR
  fork/refold cliff while preserving exact C tree parity on the witness.
- `cmd/ts2go` now accepts non-terminal aliases from the grammar's alias-symbol
  range instead of incorrectly rejecting every alias ID above `SymbolCount`.
- `BenchmarkGoParseFullDFA` now exercises the public `Parser.Parse` path and a
  fully materialized tree. The former implementation silently enabled the
  no-tree diagnostic and was mislabeled as a full parse.
- `ParseNoResultCompatibilityBenchmarkOnly` no longer implicitly enables the
  no-tree path. Its result is materialized, so `parse_gap_report` can separate
  no-tree parser-core cost from the broader no-compat diagnostic. Some
  large-input diagnostic materialization strategies still key off this mode,
  so it is not yet a pure compatibility-only A/B.

### Changed

- Added the explicitly diagnostic `BenchmarkGoParseCoreDFA` lane and withdrew
  the older generated-Go full-parse headline pending a pinned quiet-host rerun
  of the corrected public benchmark.
- External-scanner full-parse retry suppression now uses explicit certified
  language-profile metadata instead of parser-core language-name checks.
  Python and Dart retain their existing behavior, and Kotlin now treats the
  first retry ladder's selected tree as authoritative. Built-in policies are
  pinned to the exact checked-in blob SHA-256; caller-constructed, adapted, and
  override languages retain the conservative generic retry path.

### Removed

- Fourteen retired language-specific repetition/conflict dispatch helpers and
  their dead Java, JavaScript, and TypeScript implementation closure. The
  production C-faithful global repetition fold remains the sole active path.
- The superseded Python compatibility profiler, temporary C# wave-2 profiler,
  and an unreferenced perf-recording helper, removing 137 lines of obsolete
  diagnostic surface in favor of `parse_gap_report`, the retained shape
  harness, and standard Go profiles.

### Performance

- The 687-byte Caddy security-header witness now allocates about 5.4 MB/op and
  completes in milliseconds in the loaded-host smoke probe, down from roughly
  2.07 GB/op and seconds before the certified row policy. The exact CGo deep
  parity gate and bounded runtime regression both pass; timing is not ratcheted
  until a quiet-host sample is available.
- Kotlin's pinned eight-file performance set drops from 1.423 seconds to 790
  milliseconds in a one-CPU Docker A/B after removing the redundant second
  external-scanner retry ladder. All eight selected trees retain identical
  S-expression hashes, stop reasons, EOF spans, and error states.

