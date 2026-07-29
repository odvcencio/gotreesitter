# Changelog

All notable changes to this project are documented in this file.

This project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
for tags and release notes while still in `0.x`.

## [Unreleased]

### Performance

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

### Removed

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

## [0.24.0] - 2026-07-11

JavaScript large-file parity and parser memory-economy release. With an explicit
2 GiB parser budget, the 3,447,275-byte Poppler witness now reaches exact EOF
with no error and exact structural parity. Its allocation and arena-capacity
reductions reproduced in a same-day base/head audit, and a separate single-pass
runtime gate completes inside a hard 2 GiB container. JavaScript's broader
focused gate is 25/25 no-error, S-expression, and deep parity. The shipped
512 MiB Poppler budget gap remains open and tracked in the performance ledger.

### Added

- Version-aware zero-width external-token relex probes for stateless scanners,
  including transactional token-source rollback when a speculative relex is
  rejected.
- Parse-runtime memory attribution for arena, scratch, GSS, runtime heap, and
  runtime sys budget stops, plus detailed arena/raw-shape/transient counters in
  parse-gap reports.
- An opt-in `GOT_TRANSIENT_REDUCE_CHECKPOINT_MB` path that materializes the
  live linear stack and reuses transient slabs once a configured threshold is
  crossed.

### Changed

- JavaScript's precise external lex-state table is now enabled for ordinary
  scanner arbitration. Faithful C-recovery competition remains explicitly
  default-opted-out because it still regresses clean large-file throughput.
- Resolved single-path GSS stacks demote back to contiguous entries, and GSS,
  transient-child, and transient-parent overflow slabs use bounded growth.
- `Node`, `rawShape`, and `rawShapeChild` layouts remove alignment waste and
  pack raw-shape edge metadata, shrinking the records from 152 to 144 bytes,
  32 to 24 bytes, and 24 to 16 bytes respectively.

### Fixed

- JavaScript automatic-semicolon arbitration now probes same-line comments in
  the pinned C-scanner order, so the zero-width ASI precedes a trailing comment
  extra and the comment remains owned by the surrounding statement list.
- The JavaScript block-comment probe uses a labeled loop break; adjacent block
  comments can no longer consume the following token during speculative ASI
  scanning.
- Transient checkpoint recycling now follows raw-shape-only sidecar edges and
  retargets them to arena clones before slab addresses are reused, preventing
  later ambiguity decisions from observing overwritten reductions.
- Transient checkpoints defer when pending-parent compaction is active; pending
  payloads can retain nodes outside the semantic/raw-shape graph and must not
  observe recycled transient slabs.

### Performance

- A same-day #221/#222 audit reproduced the Poppler memory gains: 4.150 GB/op
  fell to 3.329 GB/op (-19.8%) and arena capacity fell from 1.422 GB to
  1.282 GB (-9.8%) while exact deep parity stayed green. Go wall time moved
  -2.1% in that sequential sample. An earlier sample recorded 11.754 s and a
  1.63x Go/C ratio, but later loaded-host samples ranged from 2.92x to 3.46x;
  v0.24.0 therefore does not ratchet Poppler timing until a pinned quiet-host
  sweep replaces the variable measurements.
- A prebuilt single-pass Poppler runtime probe accepts the exact 3,447,275-byte
  witness with the explicit 2 GiB parser budget under a hard 2 GiB cgroup at
  1,729,836 KiB maximum RSS. The separate Go/C deep-parity oracle remains in
  its 8 GiB envelope because it retains both giant trees for comparison.
- Memory-budget diagnostics are embedded in `Parser` so attribution adds no
  steady-state parser-core allocation. The v0.24.0 `978 B`/`5 allocs` sample
  used the then-mislabeled no-tree benchmark and is not a full-parse allocation
  claim; both incremental lanes remained at zero allocation.

## [0.23.1] - 2026-07-10

Generator throughput and certification follow-up. This cut banks the shared,
deterministic blob encoder and the bounded recursive-extra construction that
landed immediately after the exhaustive-parity release. It also removes the
C# generation cliff caused by repeatedly rebuilding the same skipped-extra
lex-preemption analysis. The release does not claim that Crystal's broader
real-corpus parity tail is closed; the newly visible floors remain explicit.

### Added

- Crystal-specific regression coverage proving that the bounded LALR item-set
  path preserves heredoc interpolation and completes the locked generation
  pipeline at 14,471 states with 1/1 exact parity.
- Direct-C Crystal visibility in the grammargen harness. The current measured
  floor is 16/20 no-error and 11/20 tree parity, while the aggressive corpus
  remains 10/26 exact and tracked for follow-up.

### Changed

- Runtime, grammargen, and `cmd/ts2go` blob production now share one
  deterministic encoder, including stable ordering for map-bearing trailer
  data and preservation of large-state GOTO metadata.
- Recursive heredoc extras are constructed with bounded on-the-fly LALR
  item-set core merging instead of merge-history expansion. Ruby's locked
  pipeline completes at 11,915 states with exact parity, and unrelated
  grammars stay on the legacy construction path.

### Fixed

- Skip-extra lex-preemption results are memoized by lookahead during lex-mode
  construction. On the pinned C# witness, generation plus the real-corpus
  test now completes in 39.38 seconds where the exact base still times out
  after five minutes; CGo parity remains 20/20 with zero divergences and the
  existing 24/25 no-error, 20/25 deep-parity corpus floor is preserved.

## [0.23.0] - 2026-07-10

Exhaustive parity closure release. The curated structural matrix is now
206/206 pass with no known-degraded skips: the stale-skip ratchet landed first,
then the final Norg alias-target divergence was fixed and its exemption removed.
This cut also banks the parser, recovery, scanner, and Wave 3 measurement work
that landed after v0.22.5. It does not claim that every measured grammar is
near-C on performance; the remaining JavaScript, Scala, and other cliffs stay
explicit in the perf ledger rather than being hidden by the release milestone.

### Added

- An exhaustive-parity assertion that reparses every remaining
  known-degraded language and fails CI when an exemption has gone stale.
- CI wiring and ratchets for a zero-entry known-degraded structural list, so a
  future exemption requires an explicit, reviewable policy change.

### Changed

- Wave 3 perf metadata and budgets were refreshed for CMake, Java, Kotlin,
  Lua, C#, Rust, Swift, Crystal, Scala, TypeScript, and JavaScript.
- JavaScript's Poppler memory cliff and Scala's incomplete largest-corpus run
  are recorded as attribution evidence instead of being presented as ordinary
  green measurements.

### Fixed

- COBOL `EXEC CICS` error normalization now trims recovered
  `procedure_division`/`program_definition` spans to the last material child
  when C stops before trailing trivia, while preserving C's zero-width EOF
  recovery shape. A cgo-backed adversarial fixture now guards the recovered
  error signal and byte spans against the C oracle.
- Nested terminal aliases now collapse through metadata-mediated chains, and
  punctuation-prefixed literals in word-declaring grammars retain the same
  leaf structure as tree-sitter C.
- Visible alias targets preserve distinct-named children, closing Norg's final
  four structural divergences and removing the last exhaustive-parity skip.
- External-scanner tokens now retain whether they were shifted exclusively as
  extras, preventing literal trailing whitespace inside Python string content
  from selecting an after-whitespace lex mode and suppressing an immediate
  escape token.
- Doxygen and JSDoc recovered trees now match the C oracle through scoped
  result normalizers.
- HCL external scanner symbols are bound positionally, preserving scanner ABI
  behavior when generated symbol names differ from the shipped language.

## [0.22.5] - 2026-07-09

Scoped held-out perf-ratchet release. This release follows the fleet coverage
cut with the exclusion machinery and ledger language needed to keep Wave 3
honest: Groovy is now budgeted under a named scoped basis, while D and F#
remain explicit held-outs because their current large-file witnesses fail in
the C-reference side or hit the harness RSS watchdog before a stable Go-vs-C
ratio can be ratcheted.

### Added

- perf-scan file exclusion support for reproducible scoped reruns, including
  `GTS_PERF_SCAN_EXCLUDE_PATHS`, persisted scoreboard config, and status
  reporting for measurement-basis exclusions.
- `perf_scan_status` coverage accounting for scoped held-out budget rows, so
  budgeted-with-caveat languages are visible separately from ordinary green
  rows and hard held-outs.
- Groovy Wave-3 scoped held-out budget row excluding
  `groovy/subprojects/performance/src/files/pleac11_15.groovy`, with observed
  full-parse and no-edit ratios recorded from the Docker perf sweep.

### Changed

- The perf-ratchet policy now treats expansion of
  `measurement_basis.exclude_paths` as budget loosening that requires the same
  RCA evidence as loosening a ratio threshold.
- F# and D Wave-3 ledger entries now carry the exact C-reference timeout/RSS
  witnesses found by the scoped reruns instead of presenting them as ordinary
  unmeasured gaps.

## [0.22.4] - 2026-07-09

Wave-3 perf-fleet coverage and ongoing correctness checks. This release
line extends perf-scan measurement coverage: the Go-vs-C full-parse ratio
ratchet now covers 203/206 grammars (up from the targeted subset measured at
the v0.22.0 checkpoint) via batches 1–7 plus a fleet gap-close sweep, while
keeping the three held-out grammars explicit in the ledger. It does not yet
claim universal near-C throughput: the ratchet records where every grammar
stands, including held-out rows and known cliffs, and optimizing that tail
plus a memory-blowup class in a few grammars remains tracked for the next
waves.

### Added

- Wave-3 fleet perf ratchet: Go-vs-C full-parse ratio budgets extended to
  203/206 grammars (batches 1–7 plus a fleet gap-close sweep), turning sparse
  fleet coverage into a per-language scoreboard with `perf_scan_status`
  coverage reporting and CI budget validation.
- Auto-triggered scoped CGo parity on `parser_result_*` and recovery-path PRs,
  so masking-normalization or recovery changes surface byte-exact
  verification against the tree-sitter v0.25.0 oracle instead of relying only
  on smoke coverage.
- Runtime memory budget with unified materialization stop checks, bounding
  worst-case parse memory (with a small-source fast path).
- perf-scan harness hardening: explicit C-reference failure budget, active-file
  tracking in partial fragments, and a parent-side RSS watchdog.
- Non-terminal alias maps carried in ts2go blobs as part of the parallel
  correctness work, preserving parent-context aliasing across the blob
  boundary.
- Grammargen now derives `Language.NonTerminalAliasMap`, with Lua parity,
  shipped-blob inventory coverage, and synthetic edge-case tests for
  self-recursive alias wrappers, singleton aliases, terminal aliases, and
  deterministic row ordering.

### Changed

- COBOL promoted to Tier III and marked parity-clean after the `EXEC CICS`
  parity fix below, retaining the 25/25 real-corpus and 20/20 direct C-oracle
  parity gates for this line.
- CGo parity tests now run inside Docker; grammar race lanes split by test
  range / package group for CI throughput.
- Groovy and D large-file GLR retry paths now use scoped stack ceilings to
  contain Go-side RSS cliffs; both remain explicitly tracked in the perf ledger
  until their exact Go-vs-C rows are ratchetable.

### Fixed

- COBOL if-header `EXEC CICS` error parity aligned with the C parser.

## [0.22.3] - 2026-07-08

Cobol frontier and perf-ratchet release. This release completes the focused
Wave 4 Cobol frontier work and adds the first checked-in Wave 3 perf budget
ratchet. It does not claim universal near-C performance; the ratchet and CI
validator make future claims auditable.

### Added

- `cgo_harness/cmd/perf_scan_budget` and the initial 20-language perf-ratio
  budget seed, with a fast CI validation job wired into the aggregate build.

### Changed

- Cobol precise ELS is elected by default, and recovered BBANK roots plus
  recovery-frontier shapes are normalized against the C oracle.
- Race tests and cgo harness cleanup paths are split and tag-gated so CI stays
  more responsive while preserving the correctness lanes.

### Fixed

- Cobol zero-width skipped tokens are absorbed into open `ERROR` regions so
  recovered trees keep the C-oracle error carriers instead of timing out or
  dropping them.

## [0.22.2] - 2026-07-08

C++ recovery parity fold. This release fixes the focused C++ malformed-class
recovery gap without enabling cpp C-recovery globally.

### Added

- Registry-backed `REPRO_GO_BACKEND=registry` mode for C tree-dump
  diagnostics.

### Fixed

- Malformed `class` bodies followed by `void A::b() {}` now normalize to the
  C-oracle recovered `function_definition` shape.
- Added scoped cpp result-compatibility coverage for recovered
  `class_specifier`, retagged `namespace_identifier`, and nested extra
  `ERROR(identifier)` shapes.

## [0.22.1] - 2026-07-08

Cobol oracle cleanup and CI/dead-code maintenance release.

### Changed

- CI build gating is split into faster parallel jobs.
- GLR debug/stat scaffolding, scanner/staticcheck leftovers, cgo_harness dead
  helpers, and untagged C-benchmark helpers were removed or gated.

### Fixed

- Cobol fixed-format trailing trivia spans now match the C oracle while
  preserving free-format long-line content.
- Added focused Cobol cgo regression coverage and validated Cobol against the
  80-case C oracle and strict real-corpus gates.

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

[Unreleased]: https://github.com/odvcencio/gotreesitter/compare/v0.47.1...HEAD
[0.47.1]: https://github.com/odvcencio/gotreesitter/compare/v0.47.0...v0.47.1
[0.47.0]: https://github.com/odvcencio/gotreesitter/compare/v0.46.0...v0.47.0
[0.46.0]: https://github.com/odvcencio/gotreesitter/compare/v0.45.0...v0.46.0
[0.45.0]: https://github.com/odvcencio/gotreesitter/compare/v0.44.1...v0.45.0
[0.44.1]: https://github.com/odvcencio/gotreesitter/compare/v0.44.0...v0.44.1
[0.44.0]: https://github.com/odvcencio/gotreesitter/compare/v0.43.1...v0.44.0
[0.43.1]: https://github.com/odvcencio/gotreesitter/compare/v0.43.0...v0.43.1
[0.43.0]: https://github.com/odvcencio/gotreesitter/compare/v0.42.0...v0.43.0
[0.42.0]: https://github.com/odvcencio/gotreesitter/compare/v0.41.0...v0.42.0
[0.41.0]: https://github.com/odvcencio/gotreesitter/compare/v0.40.0...v0.41.0
[0.40.0]: https://github.com/odvcencio/gotreesitter/compare/v0.39.0...v0.40.0
[0.39.0]: https://github.com/odvcencio/gotreesitter/compare/v0.38.0...v0.39.0
[0.38.0]: https://github.com/odvcencio/gotreesitter/compare/v0.37.0...v0.38.0
[0.37.0]: https://github.com/odvcencio/gotreesitter/compare/v0.36.0...v0.37.0
[0.36.0]: https://github.com/odvcencio/gotreesitter/compare/v0.34.0...v0.36.0
[0.34.0]: https://github.com/odvcencio/gotreesitter/compare/v0.33.0...v0.34.0
[0.33.0]: https://github.com/odvcencio/gotreesitter/compare/v0.32.0...v0.33.0
[0.32.0]: https://github.com/odvcencio/gotreesitter/compare/v0.31.0...v0.32.0
[0.31.0]: https://github.com/odvcencio/gotreesitter/compare/v0.30.0...v0.31.0
[0.30.0]: https://github.com/odvcencio/gotreesitter/compare/v0.29.0...v0.30.0
[0.29.0]: https://github.com/odvcencio/gotreesitter/compare/v0.28.0...v0.29.0
[0.28.0]: https://github.com/odvcencio/gotreesitter/compare/v0.27.0...v0.28.0
[0.27.0]: https://github.com/odvcencio/gotreesitter/compare/v0.26.1...v0.27.0
[0.26.1]: https://github.com/odvcencio/gotreesitter/compare/v0.26.0...v0.26.1
[0.26.0]: https://github.com/odvcencio/gotreesitter/compare/v0.25.0...v0.26.0
[0.25.0]: https://github.com/odvcencio/gotreesitter/compare/v0.24.1...v0.25.0
[0.24.1]: https://github.com/odvcencio/gotreesitter/compare/v0.24.0...v0.24.1
[0.24.0]: https://github.com/odvcencio/gotreesitter/compare/v0.23.1...v0.24.0
[0.23.1]: https://github.com/odvcencio/gotreesitter/compare/v0.23.0...v0.23.1
[0.23.0]: https://github.com/odvcencio/gotreesitter/compare/v0.22.5...v0.23.0
[0.22.5]: https://github.com/odvcencio/gotreesitter/compare/v0.22.4...v0.22.5
[0.22.4]: https://github.com/odvcencio/gotreesitter/compare/v0.22.3...v0.22.4
[0.22.3]: https://github.com/odvcencio/gotreesitter/compare/v0.22.2...v0.22.3
[0.22.2]: https://github.com/odvcencio/gotreesitter/compare/v0.22.1...v0.22.2
[0.22.1]: https://github.com/odvcencio/gotreesitter/compare/v0.22.0...v0.22.1
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
