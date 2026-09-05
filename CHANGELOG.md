# Changelog

All notable changes to this project are documented in this file.

This project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
for tags and release notes while still in `0.x`.

## [Unreleased]

### Compact parser correctness

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
See the [performance report](docs/performance/token-invariant-restoration-2026-09-05.md)
for raw results, workload limits, and memory observations.
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
See the [release performance report](docs/performance/release-v0.52.0-2026-09-05.md).

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
  Read the [recovery count report](docs/performance/recovery-visible-count-2026-09-05.md)
  and [canonicalization report](docs/performance/canonical-owner-binding-2026-09-05.md)
  for measured commits, paired results, and limits.
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

[Unreleased]: https://github.com/odvcencio/gotreesitter/compare/v0.52.0...HEAD
[0.52.0]: https://github.com/odvcencio/gotreesitter/compare/v0.51.0...v0.52.0
[0.51.0]: https://github.com/odvcencio/gotreesitter/compare/v0.50.1...v0.51.0
[0.50.1]: https://github.com/odvcencio/gotreesitter/compare/v0.50.0...v0.50.1
[0.50.0]: https://github.com/odvcencio/gotreesitter/compare/v0.49.0...v0.50.0
[0.49.0]: https://github.com/odvcencio/gotreesitter/compare/v0.48.1...v0.49.0
[0.48.1]: https://github.com/odvcencio/gotreesitter/compare/v0.48.0...v0.48.1
[0.48.0]: https://github.com/odvcencio/gotreesitter/compare/v0.47.1...v0.48.0
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
