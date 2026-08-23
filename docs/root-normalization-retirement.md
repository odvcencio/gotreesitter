# Root and result-normalization retirement plan

The repository root is intentionally the public `gotreesitter` Go package.
Its width is nevertheless a maintenance liability: at the v0.47.0 boundary it
contains 214 production Go files and 312 root-package test files. The
`parser_result*.go` compatibility tier accounts for 114 production files and
79 test files. This plan reduces that surface without inventing cosmetic
package boundaries or weakening C-tree parity.

The mechanically checked compatibility inventory remains
[`testdata/result_compat_ownership_v1.json`](../testdata/result_compat_ownership_v1.json).
This document defines the retirement program around that inventory.

## End state

The root is clean when all of the following are true:

1. Public API files and parser-engine subsystems have clear ownership in
   [the repository map](repository-map.md).
2. No generic returned-tree repair or repeated post-finalization fixpoint
   remains. Materialization owns generic node, field, alias, trivia, and span
   invariants before publication.
3. Every remaining language compatibility entry is either being retired by a
   named upstream mechanism or is deliberately retained with a documented
   public/C-oracle boundary reason.
4. New parser behavior is implemented in scheduler, recovery, derivation
   election, scanner checkpoints, incremental reuse, or materialization—not
   as a new language-name patch.
5. Independent data or policy is extracted from the root only when it can use
   a narrow internal API without exporting node-arena, stack, or pending-parent
   internals. File movement alone is not progress.
6. Generated binaries, corpora, profiles, and run receipts follow the artifact
   policy in the repository map and do not accumulate as unexplained root
   clutter.

The primary ratchets are live compatibility entries, live generic/fixpoint
arms, and production `parser_result*.go` lines. Raw root file count is reported
but is not a target by itself; splitting one package into arbitrary files or
directories does not improve ownership.

## Rules for every retirement PR

Each PR retires one ownership mechanism or one proven-inert family:

1. Identify the authoritative upstream owner and the exact registered entries.
2. Add or strengthen a witness that observes the invariant before the
   normalizer runs. For suspected dead code, instrument and census first.
3. Implement the capability or invariant once in the owning subsystem. Do not
   add a replacement language allowlist, source-text heuristic, or per-language
   parser patch.
4. Use a language-neutral producer invariant by default. Do not replace a
   retired patch with another language-specific returned-tree condition.
5. Prove production, compact, forest, and incremental routes. Run C-oracle
   parity one grammar at a time for every affected language.
6. Delete the normalizer and its exclusively owned helpers and tests. Preserve
   useful acceptance assertions at the new owner.
7. Update the registry, this plan when its schedule changes, the compatibility
   guide, and the changelog. Retired registry entries remain as historical
   receipts.
8. Run Canopy impact and quality checks, review the diff directly, and record
   the ratchet in Hyphae.

Correctness is the merge gate. Performance measurements may select the next
candidate, but a performance result cannot substitute for route parity.

The explicit forest route now shares the automatic route's `ERROR`-root
decline. It records `error_root` and returns no tree. Ledger date-suffix and
year-directive recovery use the production fallback after that decline. The
Ledger receipt records both forest routes and locked-C parity. Reopen Ledger
retirement if a future witness differs on any covered route.

## 2026-08-22 normalization checkpoint

Status: NO-GO. Do not retire an entry from this checkpoint.

Base commit: `d530d969429550555a384525352120138ef6f05d`.
The current registry denominator is 32 dispatcher arms, 34 dispatcher
languages, one predicate, zero generic passes, zero post-finalization arms,
zero post-finalization languages, 33 live entries, 55 retired entries, and 36
live language labels.

The focused evidence keeps the DTD entry live.

| Candidate | Exact passing subset | Current divergence | Evidence |
| --- | --- | --- | --- |
| DTD | `historical-large-dbits` raw and production digest `a1655ece34d0000ed54e2954faaf93c315fc86e9cea65430022fda9b33677e1d`; `historical-large-docbook` raw and production digest `5f61a372e602e6e0f772ad1a053e5cc42492add24bb045eef10370096d8cd04f` | `parser-produced-pe-reference-trigger`: `/extSubset/elementdecl[0]/ERROR[4]/Name[0]`, Go `error=true`, C `error=false`; `historical-medium-calstblx`: `/extSubset/AttlistDecl[31]/AttDef[3]/ERROR[3]/)[0]`, Go `error=true`, C `error=false` | `TestDTDDispatchRouteReceipts`; `TestDTDDispatchRetirementLockedCParity`; `harness_out/docker/20260822T164115Z`; `harness_out/docker/20260822T164122Z` |

The DTD route receipt covers raw, production, compact, forest or forest-fallback,
and incremental parses for all four sources. It requires equal production digests
and zero dispatcher rewrites on every route.
The known blocker tree digests are:

- `parser-produced-pe-reference-trigger`: Go `3e32d101e13010d7e964bcd68524291d3439309022f5aeff218d1e1c20478f0c`; C `5c2393834cf7a941dfc5e0c86dacb344cb122822631b379e21f9bf607544c860`.
- `historical-medium-calstblx`: Go `6aafeee4581dbcbea8dc807d04a56339d500c18fd7a9f034f885439fadaf2311`; C `6316281505e3891906174c07c691814c0b187d3619aa455fc01174efd2736a3e`.

The route digests are:

- `parser-produced-pe-reference-trigger`: `3e32d101e13010d7e964bcd68524291d3439309022f5aeff218d1e1c20478f0c`.
- `historical-medium-calstblx`: `6aafeee4581dbcbea8dc807d04a56339d500c18fd7a9f034f885439fadaf2311`.
- `historical-large-dbits`: `a1655ece34d0000ed54e2954faaf93c315fc86e9cea65430022fda9b33677e1d`.
- `historical-large-docbook`: `5f61a372e602e6e0f772ad1a053e5cc42492add24bb045eef10370096d8cd04f`.

Reopen DTD only after both known divergences close. Then retain exact raw,
production, compact, forest, incremental, and locked-C receipts for all four
sources.

Do not change the registry until the matching candidate condition is complete.

## 2026-08-22 JSDoc producer-fix checkpoint

Status: producer fix accepted by the focused gates at this checkpoint. The
separate retirement slice below removes the now-inert JSDoc compatibility arm.

Base commit: `a01f8037319c9f8f0ea12ae3c96523112656ce22`.

The shared lexer now records the start of a skipped prefix on the emitted real
token. The parser accepts that prefix as padding only when it begins at the
stack byte offset. Root finalization preserves the span for an authenticated
leading skip. It does not preserve an unproven non-trivia gap. This change
keeps the JSDoc normalizer and registry entry unchanged.

The focused Docker receipts pass for both parser-produced sources:

| Witness | Source SHA-256 | Raw and production digest | Dispatch rewrites |
| --- | --- | --- | ---: |
| multi-tag trigger | `8a1683a43035994f3abf03f2f9556b96514a745018c5373ff77d3127fb27d201` | `63238fbed1257ab8d7b198d02beed85a8f4915b5a48149a85bf7b7a993e9388b` | 0 |
| single-tag control | `0f4dbe6ca5d62b8c033c09ac26689c787a66298540c46b3af7a9760a7240b5ce` | `b8aec819c51b10e62d76937186fc52a8cdf91b66f4d198baa53f4a5860b3b232` | 0 |

The route receipt records these exact routes:

- Trigger: compact fallback for `accepted-leaf-tiling-gap`, forest fallback
  at `51:22:dead_end`, and incremental fallback for
  `external_scanner_unsupported`.
- Control: compact direct, forest fallback at
  `31:0:nolook_relex_empty`, and incremental fallback for
  `external_scanner_unsupported`.

At this checkpoint, the direct-route rule was: the control compact direct
route bypassed normalization and required no `dispatch.jsdoc` record. Its
forest production fallback required zero JSDoc rewrites and could record
`dispatch.jsdoc`. Other fallback routes required their own record with zero
rewrites. The retirement receipt below removes that record.

The focused Docker gates pass for the JavaScript unicode regression, the
included-range regression, and the existing non-trivia gap rejection tests.
The locked-C receipt compares symbols, fields, spans, points, extras, missing
and error flags, and the deep digest for both witnesses.

The Bash witness `e \\ cho hi` remains clean and matches locked C. The Erlang
witnesses `\x010` and `\x100` remain clean with the exact root span `1..2`.

The exact Docker artifacts are:

- `harness_out/docker/20260822T181926Z-jsdoc-v20-layout-provenance-20260822`
- `harness_out/docker/20260822T181942Z-jsdoc-v21-included-range-20260822`
- `harness_out/docker/20260822T181951Z-jsdoc-v22-root-leading-gap-20260822`
- `harness_out/docker/20260822T182000Z-jsdoc-v23-bash-gap-20260822`
- `harness_out/docker/20260822T182017Z-jsdoc-v25-a0-manifest-20260822`
- `harness_out/docker/20260822T182106Z-jsdoc-v26-erlang-locked-c-final-20260822`
- `harness_out/docker/20260822T182215Z-jsdoc-v27-bash-locked-c-final-20260822`
- `harness_out/docker/20260822T182230Z-jsdoc-v28-jsdoc-locked-c-final-20260822`
- `harness_out/docker/20260822T182245Z-jsdoc-v29-js-unicode-final-20260822`
- `harness_out/docker/20260822T182252Z-jsdoc-v30-gap-boundaries-final-20260822`

Retire the JSDoc compatibility arm only after raw and production trees match
locked C, both receipts report zero rewrites, and every listed route passes.
Reopen the candidate if any node, flag, digest, or route result diverges.

## 2026-08-22 JSDoc dispatcher retirement

Status: GO in the isolated retirement worktree. The exact base is
`f5adfc7091bad6f8adb5088a32f3b2912561fc72`.

Native JSDoc reduction now emits both registered producer witnesses without
result rewrites. The retirement removes `dispatch.jsdoc` from the live switch,
deletes `parser_result_jsdoc.go`, and keeps lexer skip provenance unchanged.

The registry census changes as follows:

| Measure | Before | After |
| --- | ---: | ---: |
| Live dispatcher arms | 32 | 31 |
| Live dispatcher languages | 34 | 33 |
| Live registry entries | 33 | 32 |
| Retired registry entries | 55 | 56 |
| A0 manifest languages and receipts | 15 | 14 |

The removed JSDoc A0 receipt covered three files, 169 visited nodes, and zero
rewrites. The focused route receipt still covers both producer sources:

- multi-tag trigger: source SHA-256
  `8a1683a43035994f3abf03f2f9556b96514a745018c5373ff77d3127fb27d201`, deep
  digest `63238fbed1257ab8d7b198d02beed85a8f4915b5a48149a85bf7b7a993e9388b`;
- single-tag control: source SHA-256
  `0f4dbe6ca5d62b8c033c09ac26689c787a66298540c46b3af7a9760a7240b5ce`, deep
  digest `b8aec819c51b10e62d76937186fc52a8cdf91b66f4d198baa53f4a5860b3b232`.

Both raw and production routes report zero rewrites. Compact, forest, and
incremental routes retain their documented direct or fallback behavior.

The locked-C receipt compares symbols, fields, spans, points, extras, missing
and error flags, and deep digests for both sources. Raw and production match C
exactly.

The focused Docker artifacts are:

- `harness_out/docker/20260822T193405Z-jsdoc-retirement-routes-main`;
- `harness_out/docker/20260822T193425Z-jsdoc-retirement-locked-c-main`;
- `harness_out/docker/20260822T193636Z-jsdoc-retirement-registry-provenance-main`;
- `harness_out/docker/20260822T193645Z-jsdoc-retirement-census-provenance-main`;
- `harness_out/docker/20260822T193504Z-jsdoc-retirement-javascript-family-main`;
- `harness_out/docker/20260822T193619Z-jsdoc-retirement-provenance-main`.

The focused gates pass. The provenance gate accepts retirement commit
`e074c5b22fafc12d587612404bd0b626a2ec628c` and records deterministic
`dispatch.jsdoc` deletion evidence. Reopen this retirement if a future JSDoc
witness rewrites a node or diverges from locked C on any covered route.

## 2026-08-22 Doxygen blocker receipt

Status: `NO-GO`. `KEEP LIVE`: `dispatch.doxygen`.

Base commit: `97a7bde26bac9b1a110bbf9216cc681ca59cc5aa`.
The accepted receipt was first recorded on `ed0568d9a822b1c83e7bb7e69b0e0f4b8ad529cf`.
The initial probe used `0c34a681db29a3e8d27e488c5f26d6d7a5f02592`.

The current registry denominator is 31 dispatcher arms and 33 dispatcher
languages. It has one predicate, zero generic passes, zero post-finalization
arms, zero post-finalization languages, 32 live entries, and 56 retired
entries.
The registry has 35 live language labels, or 34 after case folding.

The real-corpus census covered 20 languages. It found 5 inert arms, 15 active
arms, 14 uncovered registered arms, and 31 languages without a dispatcher arm.
Doxygen remains uncovered because the mounted corpus has no Doxygen directory.
The three dedicated A0 fixtures provide its current census evidence.

The A0 manifest remains authenticated with 14 languages and 14 receipts. Its
Doxygen receipt records 3 files, 2 checks, 2 runs, 4 visited nodes, zero
rewrites, 3 error roots, and zero parse errors.

The focused route receipt covers raw, production, compact, forest, and
incremental routes for all three A0 files. Raw and production report zero
rewrites for every file. The CMakeLists and example files record
`dispatch.doxygen` with zero rewrites. The metrics file has no Doxygen pass
record because its root is a whole-input `ERROR` node. Compact parsing falls
back at the parser-core end-of-file check. Forest parsing falls back at
`dead_end`. Incremental parsing falls back for the external scanner.

The same receipt covers the historical childless and recovered witnesses. The
named Doxygen pass rewrites 3 nodes for the childless witness and 14 nodes for
the recovered witness. Each count remains exact on production, compact,
forest-fallback, and incremental-fallback routes.

The locked-C receipt keeps three A0 divergence classes exact:

- `medium__CMakeLists.txt`: `/document`, category `type`, Go `document`, C
  `ERROR`.
- `medium__metrics.py`: `/ERROR`, category `shape`, Go `children=0`, C
  `children=279`.
- `small__example.cfg`: `/document`, category `type`, Go `document`, C
  `ERROR`.

The registered Doxygen smoke witness matches locked C exactly. Its raw and
production digest is
`1ae089a98760be594f06d0820951e01714097e99621cc2cd4428ce09ba867083`.

Do not retire `dispatch.doxygen` until each known divergence closes, the
historical rewrite counts reach zero, and every route matches locked C.

The focused Docker artifacts are:

- `harness_out/docker/20260822T202538Z-doxygen-blocker-routes-final`;
- `harness_out/docker/20260822T202308Z-doxygen-blocker-locked-c`;
- `harness_out/docker/20260822T202324Z-doxygen-blocker-registry-a0`;
- `harness_out/docker/20260822T202335Z-doxygen-blocker-census`.

## 2026-08-22 Apex blocker receipt

Status: `NO-GO`. `KEEP LIVE`: `dispatch.apex`.

Base commit: `f42b88ac9014537d20d3edd76e2c9caa4330a579`.

Select Apex next because its registry entry has four witnesses and the
strongest remaining focused route evidence. The existing classic-route tests
and the A3 sweep report no clean-route divergence.

The registry denominator has 31 dispatcher arms, 33 dispatcher languages, and
one predicate. It has zero generic passes, zero post-finalization arms, zero
post-finalization languages, 32 live entries, and 56 retired entries.

The A0 manifest has 14 languages and 14 receipts. It includes Apex with three
files, three checked, three run, and zero rewrites. Only the seven-fixture
tracked census excludes Apex. The full real-corpus census is unavailable
because `cgo_harness/corpus_real` is absent.

The Apex A3 sweep covers 25 constructed sources. It accepts 21 compact routes
and declines four routes. It reports zero unadjudicated divergences. The real
source count is zero because Apex has no `corpus_real` directory.

The registry names these four Apex witness families:

- `cgo_harness/parity_cgo_test.go`;
- `cgo_harness/apex_generic_local_parity_test.go`;
- `cgo_harness/apex_a3_certification_sweep_test.go`;
- `grammars/apex_class_literal_election_native_regression_test.go`.

The isolated route receipt adds five clean witnesses and three malformed
witnesses. Every clean raw, production, compact, and incremental tree matches
locked C. The raw, production, compact, and incremental routes report zero
`dispatch.apex` rewrites for every clean witness.

| Witness | Bytes | Source SHA-256 | Clean Go and locked C deep digest |
| --- | ---: | --- | --- |
| `registered-class-literal-alias` | 69 | `67d9865508abbd1a9640cbc87d8cbddf72a4b183b745682fe34f503153abad00` | `35a8cb0bdcf84a752c313b3c9cf296d4bf2acaac240646e0b32578c858832096` |
| `registered-qualified-class-literal-alias` | 70 | `6e38260d6936f22392a03eb1a8c15ac18e7d2998cc7b9ea6364093f146077601` | `74c5a760a71ec70ba65163ce52911f205bb92225f64c6948b34f997a78626069` |
| `registered-nested-generic-local` | 107 | `9de68692750cafd3be200abcb067c465922abf43a11b440c6718252ebc4f3bd3` | `7d39cbbd2e1ae5eb23bfd005f4a893688c380f3366cc0c2894d66d752e93f0a3` |
| `registered-right-shift` | 75 | `f4c26cdc96c489b569b5ed006a98f67e80218924e7d420a6417f19e44df30259` | `968e0352600624b2111c45a75dfc8d280da6b81d56d7c7bf3baf40ec91cf7dde` |
| `positive-plain-field-access` | 73 | `1e1e8663abedb7b72258789988e431d5fe4381022fb8898694f5ea83c53e77eb` | `571dfece7252cfa94c445516451636440e1a85e7638c9974dee7d278b8c66ac8` |

The compact route accepts the generic, shift, and plain-field witnesses. It
declines both class-literal witnesses for material-acceptance-election.

The forest route exposes two blockers:

- `registered-class-literal-alias`: `dispatch.apex` rewrites 3 nodes. The Go
  digest is `691872382855aafce9519a115ca30ac191c4a118d4ab7170e88e0193fd2f5bb6`.
  Locked C is `35a8cb0bdcf84a752c313b3c9cf296d4bf2acaac240646e0b32578c858832096`.
  The first difference is the empty Go field versus C field `object` at
  `/parser_output/class_declaration[0]/class_body[3]/method_declaration[1]/block[3]/local_variable_declaration[1]/variable_declarator[1]/field_access[2]/identifier[0]`.
- `registered-qualified-class-literal-alias`: the forest route rewrites 0
  nodes. The Go digest is `d07fbabd2be95f7f44171295d492ca520128437dcc5dcd503ef4fa007f69edd3`.
  Locked C is `74c5a760a71ec70ba65163ce52911f205bb92225f64c6948b34f997a78626069`.
  The first difference is `class_literal` versus `field_access` at
  `/parser_output/class_declaration[0]/class_body[3]/method_declaration[1]/block[3]/local_variable_declaration[1]/variable_declarator[1]/class_literal[2]`.

The three malformed witnesses produce error roots. Their raw, production,
compact, and incremental trees share one Go digest, but differ from C:

- `malformed-missing-class-body`: source SHA-256
  `a3bd159e3c54195698c93db0910da07c022a13b6f398abb5845bb88d6f5c7f86`;
  Go `471d9b5ccdc6ec2d3edb1db54108f4c4c29bacbadabf1cb453b990b403c3dea9`;
  C `5b51972dce75e001300bfe94099d660c769c2a81106f9b2334f09ba467b2efe0`;
  first difference `/parser_output/ERROR[0]/void_type[4]`, empty Go field versus
  C field `type`.
- `malformed-class-literal-dot`: source SHA-256
  `d2683c1e724221f18ea27687120a863e8aa80ce0af745a61f03934e2f4a405f6`;
  Go `ed9ef8a595f589c87334ea8abfc5e41aba6664656ae1e58114fc16a6fed39bbd`;
  C `a6bcbd2878b4529fc71604aa42cd1c21e07aa24e50cc57dc049b80dbd4b177d4`;
  first difference at `/parser_output/class_declaration[0]/class_body[3]/method_declaration[1]/block[3]/local_variable_declaration[1]`, with 3 Go children and 4 C children.
- `malformed-class-literal-close`: source SHA-256
  `d876a4c358f1841cbe7e29994c96da39de55ecb79ff6f5c5cd5ecdc3e0fd3ad5`;
  Go `27f56a9b801f65fade3be3ce9732f04ede66962a49a9bce3ec22395b0cc5cdfb`;
  C `a64f5e5f4d06be1d29fc1805ca14ad59247904b1b43b45769dabe69c9f2f27db`;
  first difference at `/parser_output/ERROR[0]`, with 10 Go children and 12 C children.

The malformed compact route declines during recovery. The malformed forest
route declines. Every incremental run uses old-tree reuse without unsupported
fallback.

The live positive controls are:

- `TestApexClassLiteralElectionNeedsNoResultCompatibility`;
- `TestApexClassLiteralElectionRoutesMatch`;
- `TestApexClassLiteralForestStillNeedsResultCompatibility`;
- `TestApexPlainFieldAccessUnaffected`.

Do not retire `dispatch.apex`. The forest route still rewrites the unqualified
class-literal witness and fails exact locked-C parity on both class-literal
witnesses. The malformed recovery witnesses also fail exact parity.

The focused Docker artifacts are:

- `harness_out/docker/20260822T212753Z-apex-native`;
- `harness_out/docker/20260822T212848Z-apex-a3`;
- `harness_out/docker/20260822T213413Z-apex-registry-census`;
- `harness_out/docker/20260822T213506Z-apex-blocker-routes-edited`;
- `harness_out/docker/20260822T220318Z-apex-blocker-final`;
- `harness_out/docker/20260822T220051Z-apex-census-corrected`.

## 2026-08-22 document type definition (DTD) blocker receipt

Status: `NO-GO`. `KEEP LIVE`: `dispatch.dtd`.

A document type definition (DTD) is the grammar declaration tested here.

Base commit: `30f470f5c2bf18540f7a18b2b22a7e33b88d4e10`.
Apex is in this base commit. This isolated draft changes no production or
registry files.

The registry has 88 entries. It has 78 dispatcher arms, 3 dispatcher
subpasses, 1 dispatcher predicate, 3 generic passes, and 3 fixpoints.
The live registry has 31 dispatcher arms, 33 dispatcher languages, 1
predicate, 32 live entries, and 56 retired entries. It has 35 live language
labels, or 34 after case folding.

Canopy confirms `normalizeDTDCompatibility` at
`parser_result_dtd.go:3`. It confirms the dispatcher call at
`parser_result_compat.go:143`. It also confirms the focused route and locked-C
guard functions.

The A0 (initial dispatcher census) manifest has 14 languages, 42 files, and
14 receipts. The DTD receipt has 3 files, 3 checks, and 3 runs. It visits
105771 nodes. It records zero rewrites, two error roots, and zero parse
errors.

The tracked receipt has 7 fixtures. It has no DTD fixture. Its limitation
states that the full authenticated corpus remains an external release gate.
The full real-corpus census is unavailable because `cgo_harness/corpus_real`
is absent. `TestDispatcherArmCensusOverRealCorpus` skips for this reason.

The DTD probe covers four authenticated witnesses:

- `parser-produced-pe-reference-trigger`, source SHA-256
  `f6903445e1a330ae0fd42b19c43538ea30b0da6261c1fe5ae452fc713597f0c7`,
  30 bytes;
- `historical-medium-calstblx`, source SHA-256
  `54c96c2aa55e2a95b4d0f9ac30df90cfdd717fa1c52f6d3547f1cbd3c8ad4b85`,
  9174 bytes;
- `historical-large-dbits`, source SHA-256
  `923e8f6ea911bd940ea95d957028fa3155a11b54a756bbe291d3710e110172d9`,
  285292 bytes;
- `historical-large-docbook`, source SHA-256
  `4f54c108abea1e4ae8e13e98d79bc0534d442012ed7ab40fcb4052dc843f65dd`,
  198540 bytes.

The combined receipt covers raw, production, compact, forest, and incremental
routes. The raw route matches the production route by deep digest for all four
witnesses. Production, compact, forest, and incremental routes each record
zero `dispatch.dtd` rewrites.

The route results are:

- The PE-reference witness uses compact fallback because
  `parser-core fresh-full runner did not accept EOF`. The forest fallback uses
  offset 23, symbol 1, with reason `dead_end`. Each recorded dispatch route
  visits 13 nodes. Incremental parsing reuses 6 subtrees and 20 bytes.
- The Calstblx witness uses compact fallback because
  `parser-core fresh-full runner did not accept EOF`. The forest fallback uses
  offset 3926, symbol 15, with reason `dead_end`. Each recorded dispatch route
  visits 890 nodes. Incremental parsing reuses 451 subtrees and 5306 bytes.
- The Dbits witness uses compact fallback because
  `parser-core fresh-full runner did not accept EOF`. The forest fallback uses
  offset 281605, symbol 18, with reason `dead_end`. Each recorded dispatch
  route visits 62538 nodes. Incremental parsing reuses 40706 subtrees and
  220587 bytes.
- The DocBook witness uses compact fallback and direct forest parsing.
  Each recorded dispatch route visits 42343 nodes. Incremental parsing reuses 1
  subtree and 198540 bytes.

The locked-C receipt compares symbols, fields, spans, points, extras, missing
flags, error flags, and deep digests on raw and production trees.
The two recovery witnesses still diverge:

- `parser-produced-pe-reference-trigger`: path
  `/extSubset/elementdecl[0]/ERROR[4]/Name[0]`; Go `error=true`; C
  `error=false`. Go digest `3e32d101e13010d7e964bcd68524291d3439309022f5aeff218d1e1c20478f0c`;
  C digest `5c2393834cf7a941dfc5e0c86dacb344cb122822631b379e21f9bf607544c860`;
- `historical-medium-calstblx`: path
  `/extSubset/AttlistDecl[31]/AttDef[3]/ERROR[3]/)[0]`; Go `error=true`; C
  `error=false`. Go digest `6aafeee4581dbcbea8dc807d04a56339d500c18fd7a9f034f885439fadaf2311`;
  C digest `6316281505e3891906174c07c691814c0b187d3619aa455fc01174efd2736a3e`.

The Dbits and DocBook witnesses match locked C exactly on raw and production
routes. Their digests are `a1655ece34d0000ed54e2954faaf93c315fc86e9cea65430022fda9b33677e1d`
and `5f61a372e602e6e0f772ad1a053e5cc42492add24bb045eef10370096d8cd04f`.
The recovery unit guard also passes.

The rebased route, locked-C, registry, census, and unit-recovery receipts
passed. The real-corpus census reported a controlled skip because the corpus
is absent.

Keep `dispatch.dtd` live until both C mismatches close and the authenticated
corpus becomes available. Do not change the registry or delete the arm.

The focused Docker artifacts are:

- `/tmp/dtd-post-apex-artifacts/20260822T230021Z-dtd-route-receipt`;
- `/tmp/dtd-post-apex-artifacts/20260822T230030Z-dtd-locked-c`;
- `/tmp/dtd-post-apex-artifacts/20260822T230204Z-dtd-census`;
- `/tmp/dtd-post-apex-artifacts/20260822T230220Z-dtd-registry`;
- `/tmp/dtd-post-apex-artifacts/20260822T230229Z-dtd-real-corpus-census`;
- `/tmp/dtd-post-apex-artifacts/20260822T230234Z-dtd-unit-recovery`.

## 2026-08-22 Ada blocker receipt

Status: `NO-GO`. `KEEP LIVE`: `dispatch.ada`.

Base commit: `30f470f5c2bf18540f7a18b2b22a7e33b88d4e10`.

Select Ada after the Apex, Rust, Doxygen, and DTD investigations. Ada has the
strongest remaining focused evidence among the zero-rewrite A0 arms. Existing
Ada tests cover raw parity, derivation elections, and the A3 compact
certification sweep.

The remaining zero-rewrite A0 arms are Ada, Corn, HLSL, and Wolfram. Corn and
Wolfram lack an equivalent focused locked-C route receipt. HLSL has two known
live members. Ada has the broadest focused evidence among these candidates.

The ownership registry records 78 dispatcher arms. It records 31 live arms,
47 retired arms, one live predicate, 32 live entries, and 56 retired entries.
The live dispatcher arms cover 33 language labels.

The registry records `dispatch.ada` with these subpasses:

- `dispatch.ada.constraint-kind-election`;
- `dispatch.ada.aggregate-kind-election`.

Both subpasses call `normalizeAdaCompatibilityWithCensus` in
`parser_result_ada.go`. The registry assigns derivation election selection as
the authoritative owner.

The A0 (initial dispatcher census) manifest has 14 languages and 14 receipts.
Ada reports three files, three checked, three runs, 17861 nodes visited, zero
rewritten nodes, zero error roots, and zero parse errors.

The tracked census has seven fixtures across six languages. It excludes Ada.
The full real-corpus census is unavailable because
`cgo_harness/corpus_real` is absent. The focused census test skips this lane.

The A3 (compact certification sweep) has 23 constructed sources and no real
corpus sources. It reports 20 accepted routes, three declined routes, and zero
unadjudicated divergences. The decline classes are two
`material-acceptance-election` cases and one `no-eof-accept` case.

The isolated receipt covers nine clean witnesses and two malformed witnesses.
It runs raw, production, compact, forest, and incremental routes. Every
incremental run reuses the old tree and reports `reuse_unsupported=false`.

| Witness | Source SHA-256 | Locked C digest | Route result |
| --- | --- | --- | --- |
| `positional-object-decl-access` | `438601cf3ab4e8530a28ef369308d32f33c9ba93cb2af7f1ddba990be33f4710` | `4802317080861161572ad64b8186f0cc207df05eb4c348c2ac00ba95f937f02a` | Raw differs. Production rewrites 21 nodes and matches. Compact falls back and matches. Forest and incremental match. |
| `positional-subtype-decl-access` | `75297a2141a0752b6f85b0e6603dd1629ed8f8d742c1494d453b3ca1e6d8be5d` | `6364a3353a36a4fe9d23fab0a1479425b4dc35e9ae89c5194ebb5f44496762be` | Raw differs. Production rewrites 21 nodes and matches. Compact falls back and matches. Forest and incremental match. |
| `positional-record-component-access` | `115014deda4ea4f6ea5c955d123f39e879c82be371446cd0a34b7d81a7f35bea` | `725d1f35f72151989492731f321816097e74ec2303811998987008ac115084d0` | Raw differs. Production rewrites 24 nodes and matches. Compact falls back and matches. Forest and incremental match. |
| `positional-nested-selector-access` | `26f9e5730eff55859815ac7d7f7d1eb2d9703fd756acfd730ae4e7f891456be8` | `a98a7102a5379b460c251254e52d86cd51f41accedefdbdefde3bfd137a25902` | Raw differs. Production rewrites 24 nodes and matches. Compact falls back and matches. Forest and incremental match. |
| `positional-allocator-access` | `0e692276407f81c0335054ddb2ba0f87823aabeb6fcc0d272ea485c6cc60244c` | `39c57a7494b06c554088cfbf481565214f82957011f2f116d00bcd19fe470100` | Raw differs. Production rewrites 16 nodes and still differs. Compact, forest, and incremental retain the same divergence. |
| `positional-object-decl-size` | `b18f3836c72f58b167394a08dc857059f2c38b5f4af7e1f89f7edfcc7ca3be07` | `736482700417ccad93eb25b2baff8cadfab5502608899c1a2dc270276c47a473` | Raw differs. Production rewrites 18 nodes and still differs. Compact, forest, and incremental retain the same divergence. |
| `positional-array-aggregate` | `f507e195d424e0deea4651429140847a5ec7aef3cbddcbb3d9f55290b22aaba6` | `d58ee0a74c0cda930f06996759bf776efd2aaa263e43906347c38f993cf7d6a6` | Raw differs. Production and forest rewrite 18 nodes and match. Compact accepts directly and matches. Incremental matches. |
| `named-association-control` | `4e53062ce52bee02944da551d4202771624fec474001c7b588934d946eecd0b5` | `1ff63805d27116ae37f295a738ea5edbdf8e67d4b35a6ce12252221b5450216a` | All five routes match. All `dispatch.ada` pass counts are zero. |
| `array-others-control` | `234c680409f4a7c7719cdf3e30db7ac46b15ba6470b072dcdcba6dcf93b0cff1` | `792acfe40259059b0cc90820e27afdea91d5a057d8aeb895d2d4c85ac81a5c6e` | All five routes match. The `dispatch.ada` pass count is zero. |
| `malformed-truncated-association` | `1045848382c3e5ffd917d614196ba9fde076a561529b21b9e26c6c966f62a044` | `4f4c3a61c7bb09d5aabf794d4b1c7af48b53c1d7238925a9663aafb73fa8ff26` | Raw, production, compact, and incremental differ under an `ERROR` root. Forest declines. No rewrite occurs. |
| `malformed-truncated-array-aggregate` | `deac6f71adfa4f875839eb6e2fd0f7cb7b4793123a8d9e486ef2fbba47028dab` | `a5fe2f991a85e4f5009a4b5f3831dcbc9d85658f04dac5db49868465cc071298` | Raw, production, compact, and incremental differ under an `ERROR` root. Forest declines. No rewrite occurs. |

The compact route falls back for every positional attribute witness. It accepts
the array aggregate and both controls directly. Malformed compact parses fall
back during recovery.

The forest route accepts all nine clean witnesses. It declines both malformed
witnesses. The incremental route reuses old trees for all eleven witnesses.

The raw route differs from locked C at these clean witnesses:

- `positional-object-decl-access`: `discriminant_constraint` versus
  `index_constraint` at
  `/compilation/compilation_unit[0]/subprogram_body[0]/non_empty_declarative_part[2]/object_declaration[0]/discriminant_constraint[3]`;
- `positional-subtype-decl-access`: `discriminant_constraint` versus
  `index_constraint` at
  `/compilation/compilation_unit[0]/subprogram_body[0]/non_empty_declarative_part[2]/subtype_declaration[0]/discriminant_constraint[4]`;
- `positional-record-component-access`: `discriminant_constraint` versus
  `index_constraint` at
  `/compilation/compilation_unit[0]/subprogram_body[0]/non_empty_declarative_part[2]/full_type_declaration[0]/record_type_definition[3]/record_definition[0]/component_list[1]/component_declaration[0]/component_definition[2]/discriminant_constraint[1]`;
- `positional-nested-selector-access`: `discriminant_constraint` versus
  `index_constraint` at
  `/compilation/compilation_unit[0]/subprogram_body[0]/non_empty_declarative_part[2]/object_declaration[0]/discriminant_constraint[3]`;
- `positional-allocator-access`: `discriminant_association` versus
  `expression` at
  `/compilation/compilation_unit[0]/subprogram_body[0]/handled_sequence_of_statements[3]/assignment_statement[0]/expression[2]/term[0]/allocator[0]/discriminant_constraint[2]/discriminant_association[1]`;
- `positional-object-decl-size`: `discriminant_constraint` versus
  `index_constraint` at
  `/compilation/compilation_unit[0]/subprogram_body[0]/non_empty_declarative_part[2]/object_declaration[0]/discriminant_constraint[3]`;
- `positional-array-aggregate`: `record_aggregate` versus
  `positional_array_aggregate` at
  `/compilation/compilation_unit[0]/package_declaration[0]/object_declaration[4]/expression[5]/term[0]/record_aggregate[0]`.

The production route still differs after the native dispatch pass for these
witnesses:

- `positional-allocator-access`: `index_constraint` versus
  `discriminant_constraint` at
  `/compilation/compilation_unit[0]/subprogram_body[0]/handled_sequence_of_statements[3]/assignment_statement[0]/expression[2]/term[0]/allocator[0]/index_constraint[2]`;
- `positional-object-decl-size`: `access` versus `identifier` in the
  attribute designator at
  `/compilation/compilation_unit[0]/subprogram_body[0]/non_empty_declarative_part[2]/object_declaration[0]/index_constraint[3]/attribute_designator[3]/access[0]`.

The malformed association witness differs at `/compilation/ERROR[0]`:
Go emits `ERROR`, while locked C emits `compilation_unit`.

The malformed aggregate witness differs at
`/compilation/ERROR[0]/identifier[7]`: Go has an empty field, while locked C
has the `subtype_mark` field.

The live controls are `named-association-control` and
`array-others-control`. Existing controls also pass in
`TestAdaConstraintKindElectionCParity`.

Keep `dispatch.ada` live. The raw route still elects the wrong derivation for
seven clean witnesses. The production pass still rewrites seven witnesses and
fails exact parity on two of them. Two malformed witnesses fail exact parity.

Do not change registry or production state.
Reopen retirement only after the derivation election owner emits exact trees.
Require zero `dispatch.ada` rewrites on all five routes.
Require both malformed witnesses to match locked C.

The focused Docker artifacts are:

- `/tmp/ada-next-live-rebased-artifacts/20260822T232057Z-ada-route-probe`;
- `/tmp/ada-next-live-rebased-artifacts/20260822T232109Z-ada-existing-guards`;
- `/tmp/ada-next-live-rebased-artifacts/20260822T232118Z-ada-census-registry`;
- `/tmp/ada-next-live-rebased-artifacts/20260822T232128Z-ada-real-census`.

## Ordered program

### R0 — inventory and containment

Status: complete.

- The repository map names the root subsystems and local-artifact policy.
- The ownership registry rejects unregistered dispatcher, predicate, generic,
  and post-finalization behavior.
- Collapsed named-leaf reconstruction and previously identified dead
  compatibility helpers have retirement receipts.
- The incremental campaign is capability-based; scanner checkpoint admission
  and GSS ownership no longer depend on language exceptions.

### R1 — eliminate shared tail and fixpoint scaffolding

Status: complete.

1. Land the clean-root trailing-trivia owner and delete the generic
   trailing-extra compatibility pass.
2. Remove JavaScript from the returned-tree second pass after proving its
   canonical compatibility pipeline already owns the final root span.
3. Retire the remaining generic terminal-leaf pass by moving the invariant
   into construction/materialization. This must cover lazy compact child
   references without forcing them.
4. Retire HTML and Scala second-pass arms independently. Use exact range
   receipts to retire the HTML arm. This change removes Scala span
   repairs.
   Scala recovery, field, and annotation repairs now run once.
   This change deletes the shared fixpoint.
   Delete the shared fixpoint only after the Scala arm retires.

Exit: zero generic compatibility passes and zero post-finalization fixpoint
arms.

### R2 — census and remove inert language passes

Status: the first dispatcher retirement merged in PR #463.
PR #470 retired the Rust dot-range repair.
Other Rust recovery behavior remains live.

#### `dispatch.rust` blocker receipt — 2026-08-22

Status: NO-GO. Keep `dispatch.rust` live.

Base commit: `97a7bde26bac9b1a110bbf9216cc681ca59cc5aa`.

The registry contains 88 entries: 78 dispatcher arms, three dispatcher
subpasses, one dispatcher predicate, three generic passes, and three fixpoint
passes. Its denominator contains 31 dispatcher arms, 33 dispatcher languages,
one predicate, zero generic passes, zero post-finalization arms, 32 live
entries, and 56 retired entries.

The receipt covers 23 Rust witnesses. Every witness reports zero production
rewrites. The raw and production deep digests match for every witness.

The witness groups are:

- Registered smoke and tracked route witnesses: `ownership-registry-smoke`
  (25 bytes), `tracked-census-rust_ast.rs` (66,281 bytes).
- Clean recovery controls: `admission-depth` (76), `token-tree` (334),
  `recovered-impl-item` (493), `doc-comment` (39), `token-binding` (35),
  `pattern-statement` (52), and `struct-expression` (60).
- Expanded clean controls: `admission-direct-external-payload` (6,436),
  `outline-rust-lib` (382), `parity-lifetime-and-abstract-types` (159),
  `parity-pattern-statements` (175), `parity-macro-invocations` (132),
  `parity-weird-expressions` (745), and `parity-weird-top-level` (780).
- Malformed recovery controls: `malformed-function-impl-type` (42),
  `malformed-top-level-impl` (54), `malformed-let-closure` (31),
  `malformed-token-tree` (31), `malformed-pattern-statement` (28),
  `malformed-struct-expression` (48), and `malformed-doc-comment` (15).

The nine route witnesses report `dispatch.rust` checked 1, run 1, and
rewritten 0. Their visited-node totals are 16, 17,506, 42, 164, 160, 20,
19, 31, and 22, in the order listed above.

The locked-C deep receipt covers the registered smoke and tracked witnesses.
Both raw, production, compact, forest, and incremental trees match locked C.

| Witness | Source SHA-256 | Raw, production, and C digest | Appended route digest |
| --- | --- | --- | --- |
| `ownership-registry-smoke` | `5ea70a939eaf182a1d392818bc49b8f25a74334f1002a409865050305a1abf5e` | `f7120d52d2861af7fe4dc6b6d7f0073404236fe8cc6f812f9e40a16253521c63` | `7db86c0ca563a97cef50489560a0819768cc58ad0fce9dd567a12fcd82dc6c07` |
| `tracked-census-rust_ast.rs` | `43fc2344174da29bb3c032b260a009828e4636965c1ab8cfff62b651caf91b92` | `be989d589814c4ca7e4b62203ad79e817a1c8c8903182f3d74b4a11421415faa` | `59aa777b44241ea9755a8065fd92efe57ef15e2b9965362d1bc06dd58516cd61` |

The compact route admitted both witnesses directly. It recorded two candidate
admissions and zero fallbacks. The forest route accepted both witnesses with
`ForestFastPath=true`. The incremental route matched the target digest, but
it used the documented `external_scanner_unsupported` fallback. It reused
zero subtrees and zero bytes.

The expanded and malformed traces emitted no dispatcher transcript records.
They therefore found no actual Rust rewrite in the 23-source probe. The live
positive controls still exercise direct recovery mutations:

- `TestNormalizeRustTokenBindingPatterns`;
- `TestNormalizeRustRecoveredFunctionItems`;
- `TestNormalizeRustRecoveredPatternStatementsRetagsCleanTopLevelRoot`.

The full authenticated Rust corpus census remains unavailable. The A0 manifest
contains 14 languages, 42 fixtures, and 14 receipts. It contains no Rust
entry. The tracked census contains seven fixtures and one Rust entry. The
real-corpus census skips because `cgo_harness/corpus_real` is absent.

The Rust real-corpus parity run reports two structural differences in one
`weird-exprs.rs` sample. It parses 25 of 25 samples without errors, but only
24 of 25 samples match the reference tree.

- `root/function_item[12]/block/let_declaration/unary_expression/parenthesized_expression/binary_expression/call_expression/parenthesized_expression/closure_expression/closure_parameters/tuple_pattern/or_pattern`: generated `or_pattern`, reference `closure_expression`, source `|__@_|__`;
- `root/function_item[21]/block/let_declaration[1]/tuple_pattern/or_pattern`: generated `or_pattern`, reference `closure_expression`, source `|x| x`.

These mismatches block retirement. The registry condition also requires exact
native output for every registered witness and every listed route. The current
receipt lacks the full authenticated Rust corpus and does not remove the
positive recovery controls. Do not change the registry or delete the arm.

Focused Docker artifacts:

- `/tmp/gts-dispatch-rust-probe.4eC4qF/harness_out/docker/20260822T205644Z` — locked-C deep routes;
- `/tmp/gts-dispatch-rust-probe.4eC4qF/harness_out/docker/20260822T205936Z` — expanded clean trace;
- `/tmp/gts-dispatch-rust-probe.4eC4qF/harness_out/docker/20260822T210303Z` — malformed recovery trace;
- `/tmp/gts-dispatch-rust-probe.4eC4qF/harness_out/docker/20260822T210319Z` — positive controls;
- `/tmp/gts-dispatch-rust-probe.4eC4qF/harness_out/docker/20260822T205946Z-diag-rust` — Rust real-corpus mismatch;
- `/tmp/gts-dispatch-rust-probe.4eC4qF/harness_out/docker/20260822T205656Z` — registry receipt;
- `/tmp/gts-dispatch-rust-probe.4eC4qF/harness_out/docker/20260822T205722Z` — A0 manifest receipt;
- `/tmp/gts-dispatch-rust-probe.4eC4qF/harness_out/docker/20260822T205754Z` — unavailable full-corpus receipt.
- `/tmp/gts-dispatch-rust-blocker.nhpp7W/harness_out/docker/20260822T210941Z` — registry validation;
- `/tmp/gts-dispatch-rust-blocker.nhpp7W/harness_out/docker/20260822T211014Z` — A0 and tracked census validation;
- `/tmp/gts-dispatch-rust-blocker.nhpp7W/harness_out/docker/20260822T214433Z` — focused blocker guard.

Keep this entry live until a producer fix closes both structural mismatches.
Require the authenticated Rust corpus.
Move the recovery controls to their owning subsystem.

## 2026-08-22 HLSL blocker receipt

Status: NO-GO. KEEP LIVE: `dispatch.hlsl`. The HLSL arm remains live.

Base commit: `0b4470b0156afb1e3492f7f5f6a618c9e50f7c33`.

The registry contains 88 entries. It contains 78 dispatcher arms, three
dispatcher subpasses, one dispatcher predicate, three generic passes, and
three fixpoint passes. The live denominator contains 31 dispatcher arms, 33
dispatcher-arm language labels, one predicate, 32 live entries, and 56 retired
entries. It contains zero generic passes and zero post-finalization arms.
It contains zero post-finalization language labels.

The registry keeps `dispatch.hlsl` live. It names
`normalizeHLSLCompatibility` in `parser_result_hlsl.go`. Its remaining
members repair negative-number casts and unorm buffer template arguments.
Its three registered witnesses are:

- `cgo_harness/parity_cgo_test.go`;
- `grammars/hlsl_subscript_assignment_native_regression_test.go`;
- `cgo_harness/hlsl_subscript_assignment_parity_cgo_test.go`.

The A0 (authenticated dispatcher census) manifest has 14 languages, 42 files,
and 14 receipts. A0 has three HLSL files, three checked, three run, and zero
rewrites.
It records 100,424 visited nodes, one error root, and zero parse errors.
The HLSL files are:

- `large__scalar-operators-assign-exact-precision.hlsl`;
- `medium__SubD11_SmoothPS.hlsl`;
- `small__atomic_cast1.hlsl`.

The tracked census has seven fixtures in six languages. It excludes HLSL.
The authenticated real-corpus census is unavailable because
`cgo_harness/corpus_real` is not mounted. No HLSL A3 receipt is registered.

The focused route receipt uses six witnesses. It covers raw, production,
compact, forest, incremental, and locked-C routes. It also includes two
malformed recovery witnesses and two native subscript controls.

The clean cast witness uses source SHA-256
`495d7be10c3780d26df6dc12d4190d6621a5fc51543a041374e65166ca572132`.
Its raw Go digest is
`e5d64d87ea862f2e28d3f53dd6bf53dd21936ee79496c9137d2fa944171eb1ca`.
Locked C digest is
`87800a73e5ce82b935120f2a14ae20ea6cc61023a631333cc7986a52f78a6ead`.
Raw and C differ at
`/translation_unit/function_definition[0]/compound_statement[2]/return_statement[1]/cast_expression[1]`.
Go has `cast_expression`. C has `binary_expression`.

Production, compact, forest, and incremental routes produce digest
`c6895007d3b8523d87aa77eadf1123f1e96c60def279c9ce77f506609f74334f`.
The clean cast witness rewrites one cast_expression node on production,
compact, forest, and incremental routes. The normalized routes diverge from
locked C at the missing left field.
The first normalized difference is
`/translation_unit/function_definition[0]/compound_statement[2]/return_statement[1]/binary_expression[1]/parenthesized_expression[0]`, with Go field empty and C field `left`.
Forest and incremental receipts report
`dispatch.hlsl` as `1/1/23/8`. Incremental reuse is unsupported because the
HLSL external scanner requires a fresh fallback.
The four values are checked, run, visited, and rewritten. The final `8` is the
dispatcher node-rewrite count. The source-level rewrite is one
cast_expression-to-binary replacement.

The malformed cast witness uses source SHA-256
`5626b370caee23a1da24c0c428ef6acece8539f59c9af40de0e1a62e8c4703d8`.
It keeps Go digest
`1313f71496a9b8c1f981085f87c1b1bc3eb484815c640554e63697d301414f02` on raw,
production, compact, and incremental routes. Locked C digest is
`ff8d5517a3727d8cc08631b3e71fc4efb99818cf800fb2db0b96206dee969b6b`.
The first difference remains the cast-versus-binary node type at
`/translation_unit/function_definition[0]/compound_statement[2]/return_statement[1]/cast_expression[1]`.
The forest route declines. Compact recovery falls back because the scheduler
has no table action for the elected token. Incremental reuse is unsupported.

The valid unorm buffer control uses source SHA-256
`23d5e4a473c6518140c0f63fd9c58d91b79d792562a616e9c56a2adb28dd127c`.
It matches locked C on raw, production, compact,
and incremental routes. Its Go and C digest is
`8cbb8f76171423dfab0b254e1602c4619a85b1f0ab7184edfadbad3b886ad647`.
Production, compact, and incremental report `dispatch.hlsl` as `1/1/15/0`.
The forest route declines. The malformed unorm witness uses source SHA-256
`95326c7ce08df600cc54d8e785a6794a51099f51968e697b2e6e3903c9c62dcd`.
It keeps Go digest
`05b2bf7a42e5c0ebbcf90d1ed35fddff902e995f97ce767404941a6299d337d9`.
Locked C digest is
`d3aa05dea7693d685f06f9dd32b9f2b155c92dac3bc5715b4195a4e7857de668`.
Production, compact, and incremental report `dispatch.hlsl` as `1/1/14/0`.
The first difference is `/translation_unit/expression_statement[0]`.
Go has `expression_statement`. C has `declaration`. The malformed forest
route declines. Compact recovery falls back because the scheduler has no
table action for the elected token. Incremental reuse is unsupported.
The two unorm witnesses produce no unorm rewrite. The authenticated corpus is
required before classifying that member as unreachable.

The valid unorm compact route also falls back. Its scheduler frontier lacks
alternative-set coverage by one non-blended survivor. The function subscript
compact route falls back for the same scheduler-frontier reason. The top-level
subscript compact route falls back because it lacks a sole homogeneous accept
frontier. Forest accepts both subscript controls.

The function subscript control uses source SHA-256
`d288bf6a5b940b282cb1285d60209e4690589374d2ed76e214fc572d993e42f6`.
It matches locked C on every route with digest
`688445ba93476a6948684073b044cc8cf3dd13cbf4fbe889681221e12b774737`.
The top-level control uses source SHA-256
`fb0d81ee0973f6d1611b7cccea93cfccb267087fb65002d5d6d8a1fe541f639f`.
It matches locked C on every route with digest
`55f06b3603d46533fa00b5bdb6b586df5f6999aad99829764c05140bb1335266`.
Both controls report zero rewrites. They preserve the retired subscript
member's native election. Compact may decline at a scheduler frontier.

The successful focused Docker artifacts are:

- `/tmp/hlsl-next-artifacts/20260822T235342Z-hlsl-next-blocker-final3` — route and document guard;
- `/tmp/hlsl-next-artifacts/20260822T235504Z-hlsl-next-document-guard-final2` — final document guard;
- `/tmp/hlsl-next-artifacts/20260822T234934Z-hlsl-next-registry` — registry receipt;
- `/tmp/hlsl-next-artifacts/20260822T234940Z-hlsl-next-a0` — A0 manifest receipt;
- `/tmp/hlsl-next-artifacts/20260822T234948Z-hlsl-next-tracked` — tracked census receipt;
- `/tmp/hlsl-next-artifacts/20260822T234955Z-hlsl-next-real-corpus` — unavailable corpus receipt.

The receipt remains NO-GO. Keep `dispatch.hlsl` live. Require the native
producer to emit the locked-C cast and unorm shapes before compatibility.
Require exact production, compact, forest, incremental, and locked-C parity.
Keep dispatch.hlsl live until scheduler_action_semantics emits the C field.
Require the authenticated HLSL corpus before retirement.

The mandatory shape is census before migration. Historical audits already
found that table or engine fixes can leave old normalizers behind.

1. Retire the Rust dot-range pass in two checkpoints.
   The exact collapsed-child policy now retains each bare anonymous `..` token.
   The authenticated producer census found no remaining bare-range candidate.
   It covered 37,121 nonempty, clean files and 18,506 truncated files.
   The merged-left-side conflict rule now selects chained dot-range shifts.
   PR #470 removed the remaining repair.
2. Re-census any pass whose original bug is now covered upstream or whose
   registered witness no longer reaches it. A zero rewrite count is only
   actionable when positive controls prove the probe.
3. Keep live Rust doc-comment behavior and recovery-only token-tree behavior
   out of dead-code PRs; they belong to materialization and recovered-forest
   work respectively.

The tracked CI ratchet in `testdata/dispatcher_census_tracked_v1.json` pins
source hashes, arm identities, and exact census totals for a small source set.
It does not replace the full authenticated corpus, which remains an external
release gate.

Exit: no compatibility pass is retained solely because an old fixture calls
it directly.

First retirement PR: eleven dispatcher arms censused zero rewrites over the
real corpus. The eleven were bash, elixir, html, julia, kotlin, ocaml, php,
ruby, rust, swift, and yaml. A per-language re-verification then added
native-parse regression tests. These tests check the engine's output
directly; they do not just repeat the corpus census.

The re-verification found three arms genuinely dead:

- OCaml's collapsed named-leaf restoration.
- Ruby's top-level module bound shrink.
- Half of HTML's arm: the ERROR-root nested-custom-tag reconstruction. At this
  R2 checkpoint, the separate returned-tree second pass still called the range
  fixup. R1 later retired that independent function.

This PR retires those three arms.

The re-verification kept the other eight arms live. For each of the eight, a
registered witness or a new native-parse regression test still fires on a
real construct. The thin corpus sample happened to miss that construct:

- Rust: recovered function items and token-tree recovery.
- Julia: return-range, macro-juxtaposition, and matrix-subscript repairs.
- Kotlin: the generic-call-with-trailing-lambda repair. This is a common
  Gradle DSL shape.
- PHP: list-destructuring retyping.
- Swift: ternary-expression recovery. This subpass was live at the R2
  checkpoint. The regenerated Swift grammar blob now owns that shape.
- YAML: malformed-flow-collection recovery.
- Bash: multi-assignment splitting, for example `a=1 b=2 c=3`. A first,
  single-line probe missed this; a second, adversarial probe caught it.
- Elixir: the hidden-newline-before-comment filter. A first probe reused
  source strings the normalizer was still active for, so it missed the
  construct too; a later native-parse regression test caught it.

This is the exact failure mode item 2 above warns about. A zero rewrite
count over a three-file corpus is a lead, not proof. Only a native-parse
regression test — run after removing the candidate code, not before — can
confirm dead code.

### R3 — move materialization invariants upstream

Status: in progress.

PR #471 retired the Lua, Make, and Zig field-projection arms.
PR #472 retired the trailing-span family.
The regenerated Swift grammar blob now emits native ternary expressions.
This change retires the Swift ternary source-reparse subpass.
Generic reduction now preserves the hidden named Kotlin call wrapper.
This change retires the Kotlin interpolated-call subpass.
Shared root finalization now owns the leading-trivia root family.
This change removes seven language-local repairs and retires Squirrel's arm.
Pinned alias maps now own the CUE, Git Commit, and R collapsed children.
This change retires three more dispatcher arms.
Reduction now owns one collapsed-token family across HCL, CPON, C#, and
PowerShell. This change retires CPON's dispatcher arm.
The other three arms remain live for unrelated repairs.
Reduction and root acceptance now own Haskell and Erlang root fields.
This change retires two language-local field repairs.
Native reduction and root finalization own Haskell section spans.
The remaining Haskell dispatcher arm is retired.
Shared root finalization now removes hidden whitespace extras at every root
position. The rule preserves visible extras, fields, spans, and lazy child
references. Native HCL reduction already owns each body span.
The HCL dispatcher arm is retired.
Native reduction already owns D `module_def` bounds.
The D dispatcher remains live for unrelated shape repairs.
Native derivation election chooses the correct Erlang macro replacement.
Native reduction owns Erlang top-level form spans.
The Erlang dispatcher arm is retired.
The DFA keyword path now owns Arduino primitive-type projection.
Native Objective-C materialization owns protocol type identifiers.
This change retires Arduino's arm and one Objective-C subpass.
Other Objective-C repairs remain live.
Generic result election now preserves visible named unary wrappers.
This change retires the D template-call type wrapper.
Native visible-wrapper election also preserves D storage classes.
This change retires the D storage-class wrapper.
Native reduction places D type qualifiers inside the following type.
This change retires the D variable-type qualifier repair.
Native reduction and derivation selection now own each remaining D call target.
This change retires the D dispatcher arm.
The field-aware C-oracle receipt is:
`harness_out/docker/20260728T070352Z-retire-d-storage-class-c-oracle-fields`.
The D qualifier C-oracle receipt is:
`harness_out/docker/20260728T081051Z`.
Final-line-break probes now preserve qualified, template, and simple callees.
Exact stack-node equivalence preserves deep Objective-C alternatives.
Generic alias-target selection now owns `@encode` identifiers and function
pointer expressions.
This change retires two Objective-C subpasses.
Native derivation selection also owns single and concatenated `@` strings.
This change retires a third Objective-C subpass.
Raw-shape equivalence now preserves compound struct type specifiers.
This change retires a fourth Objective-C subpass.
The parser now folds raw descendants into certified materializing-shape
hashes.
This change preserves method type identifiers before result compatibility.
It retires a fifth Objective-C subpass.
One Objective-C subpass remains live.
Generic result selection preserves the expression and type alternatives for
an Objective-C `sizeof` operand.
It selects the C-equivalent expression for an unknown type name.
This change retires the final Objective-C subpass and its dispatcher arm.
The parser covers every byte in each recovered EBNF source.
This change removes the EBNF dispatcher arm.
The native reduction path sets Dart switch-expression body fields.
It sets the target field for nested Elixir calls.
The Dart and Elixir dispatcher arms remain live for unrelated repairs.
Reduction now owns the remaining Scala and SQL field corrections.
Inherited edges fill anonymous gaps between repeated direct descendants.
They do not cross a leading separator without direct descendant evidence.
This change removes three Scala repairs and the SQL `INTO` cleanup.
The field-aware C-oracle receipt is:
`harness_out/docker/20260728T111158Z-objc-struct-sized-postfilter-final`.
The method type C-oracle receipt is:
`harness_out/docker/20260728T113024Z`.
The dispatcher census now records each remaining D and Objective-C subpass.
Native HTTP actions already emit complete document sections.
Forest selection now preserves the equivalent recorded container alternative.
This change retires the inert section-coalescing subpass and its dispatcher arm.
Native Bash reduction already emits complete command-name concatenations.
This change retires the inert command-name subpass.
Native Bash scheduling already emits the assignment action for the generated-
command witness.
This change retires the generated-command assignment subpass and its dispatcher
arm. The assignment-wrapper and `if`-field probes remain native producer
controls.
Native Ninja reduction emits both registered A0 trees without rewrites.
The exact raw, production, compact, forest, incremental, and locked C receipts
match for both witnesses. This change retires the Ninja dispatcher arm.
Reopen the entry if a future witness rewrites a node or diverges on any route.
Native Ledger reduction emits the same tree for both parser-trigger witnesses
and the registered A0 witness without rewrites.
The exact raw, production, compact, forest, incremental, and locked C receipts
match for all three witnesses. Compact fallback and `error_root` forest
fallbacks are documented for the Ledger triggers.
This change retires the Ledger dispatcher arm.
Reopen the entry if a future witness rewrites a node or diverges on any route.
Native FIDL recovery already emits the C-equivalent versioned-layout-modifier
error shape for stray modifier arguments.
This change retires the FIDL dispatcher arm.
The HLSL grammar's negative dynamic precedence on structured-binding
declarators already makes native election prefer a subscript-assignment
expression over a structured-binding declaration.
This change retires the HLSL subscript-assignment member.
The negative-number cast and unorm-buffer members remain live.

Group by invariant, not language:

1. terminal leaves and hidden trivia;
2. root and child span ownership;
3. field and alias projection;
4. recovered-node construction and parent/child attachment.

A mechanism PR may delete several dispatcher entries. A one-language
regression is acceptable as a witness, but the implementation must be
capability- or metadata-driven and apply to every matching grammar.

Exit: materialization-owned compatibility entries are either retired or
explicitly classified as retained format-boundary behavior.

### R4 — derivation election and scheduler/recovery

Status: mechanism work required.

The compact scheduler now owns bounded no-lookahead reductions.
This removes three smoke fallbacks without a language-specific runtime rule.
Byte-boundary progress now admits COBOL zero-width extras.
This removes one more smoke fallback.
The Cooklang smoke fixture no longer requires production recovery.
This removes one fixture-induced fallback without a runtime change.
No result normalizer retired in this step.

1. Express ambiguity and dynamic-precedence decisions in certified conflict
   or derivation policy when the parser actually observes competing actions.
2. Do not force deterministic post-parse rewrites into conflict policy; the
   JS/TS census proved that these are different classes.
3. Build recovered-forest ownership in the parser core before deleting the
   recovery-heavy families. Validate in increasing risk order:
   AWK/C/Kotlin/Rust, then COBOL/Scala/Authzed/PowerShell and other
   oracle-heavy families.
4. Keep scanner state and incremental invalidation fixes in their own owners;
   neither is a reason to add result compatibility.

Exit: scheduler/action and derivation/election entries shrink through shared
engine behavior, with no per-language parser exceptions.

### R5 — extract only stable internal boundaries

Status: last.

After normalization ownership has moved upstream, reconsider package
extraction for pure policy, immutable generated metadata, or harness support.
Do not create an `internal/resultcompat` package: it would merely export or
cycle through `Node`, arenas, stack entries, and pending parents while
preserving the liability.

Exit: any remaining broad root subsystem is broad because it shares runtime
ownership, not because unrelated utilities or generated artifacts were left
there.

## 2026-08-23 Solidity blocker receipt

Status: NO-GO. KEEP LIVE: `dispatch.solidity`.

Base commit: `7498a678c52029a82f312e9637ecb66b15defa0b`.

Select Solidity after WGSL. Its next ranked evidence has the highest cited
rewrite count among the remaining live arms.

The registry has 88 entries. It has 78 dispatcher arms, three dispatcher
subpasses, one dispatcher predicate, three generic passes, and three fixpoint
passes. The live denominator has 31 dispatcher arms, 33 dispatcher-arm
language labels, one predicate, 32 live entries, and 56 retired entries. It
has zero live generic or fixpoint passes.

The registry keeps `dispatch.solidity` live. It names these functions in
`parser_result_solidity.go`:

- `normalizeSolidityMemberObjectWrappers`;
- `normalizeSolidityCallExpressionAliases`.

The authoritative owner is `derivation_election_selection`. The registry
requires exact production, compact, forest, incremental, and locked-C route
receipts for every registered witness before retirement.

The A0 (initial dispatcher census) manifest has 14 languages, 14 receipts,
and 42 files. It records 44 checks, 44 runs, 313572 visited nodes, 3267
rewrites, 20 error roots, and zero parse errors. Solidity records three files,
three checks, three runs, 26897 visited nodes, 666 rewrites, zero error roots,
and zero parse errors.

The Solidity A0 files are:

| File | Bytes | Source SHA-256 |
| --- | ---: | --- |
| `small__IERC3156.sol` | 263 | `9fbd10c6970c328f348c9a86604bdad336743caeda2547f94b6a86d8a906c961` |
| `medium__Initializable.sol` | 9279 | `f527a063813c2bf60c153fb08e38539578935402894fcc36fac42324ca325d3b` |
| `large__Packing.sol` | 64872 | `766829f6d9758a1318dd009143912d7aa6bbafa4f4b2a137c94d7f81a73b38ac` |

The tracked census has seven fixtures across six languages. It excludes
Solidity. The authenticated real-corpus census is unavailable. The full
authenticated corpus is unavailable because `cgo_harness/corpus_real` is
absent.

The locked C oracle uses grammar commit `048fe686cb1fde267243739b8bdbec8fc3a55272`.
The focused receipt covers eight witnesses on raw, production, compact,
forest, incremental, and locked-C routes.

The registered A0 and positive witnesses report these exact results:

- `a0-small-IERC3156` matches locked C on every route. Forest and incremental
  report `1/1/31/0`. Incremental reuse is `15/179`.
- `a0-medium-Initializable` has raw digest
  `b38a5f0babca0fec5a4b6c6fad6169ad0f201e0606f8400553ca2034e731c8dd` and
  locked-C digest `9c73deee203b676abf35a10a7dfa02c6ed90ee21209f9745bcb0256fd935526f`.
  The first raw and normalized difference is the `member_expression` versus
  `unary_expression` at
  `/source_file/contract_declaration[4]/contract_body[3]/modifier_definition[12]/function_body[4]/statement[4]/variable_declaration_statement[0]/expression[2]/member_expression[0]`.
  Production, compact, and incremental report `1/1/798/666`. Forest reports
  `1/1/817/604` and a second `_primary_expression` versus `identifier`
  difference. Compact falls back at the scheduler frontier. Incremental reuse
  is `46/840`.
- `a0-large-Packing` matches locked C on raw, production, compact, and
  incremental routes. Those routes report `1/1/26068/0`. Forest reports
  `1/1/26458/0` and diverges at
  `/source_file/library_declaration[6]/contract_body[2]/function_definition[58]/function_body[10]/statement[1]/if_statement[0]/expression[2]/binary_expression[0]/expression[0]/_primary_expression[0]`.
  Compact falls back because the live-link cap reaches `9 > 8`. Incremental
  reuse is `6898/26115`.
- `positive-control` matches locked C on raw, production, compact, and
  incremental routes. Forest reports a `_primary_expression` versus
  `identifier` difference. Incremental reuse is `14/53`.

The focused member and call witnesses expose both live Solidity functions:

- `clean-member` matches locked C after production, compact, and incremental
  routes rewrite the unary member wrapper. Each of those routes reports
  `1/1/42/7`. Forest matches without a rewrite and reports `1/1/41/0`. Raw differs at
  `/source_file/contract_declaration[0]/contract_body[2]/function_definition[1]/function_body[8]/statement[1]/return_statement[0]/expression[1]/member_expression[0]/expression[0]`.
  Incremental reuse is `14/53`.
- `clean-call-alias` matches locked C on raw. Production, compact, and
  incremental routes report `1/1/45/12` but keep `call_expression` where C
  has `type_cast_expression`. Forest reports `1/1/46/13` and the same type
  difference. Incremental reuse is `16/61`.

The malformed controls remain live evidence:

- `malformed-member` has an `ERROR` root. Raw reports digest
  `64d54df15129a3845acd6eda9e9470f40dc50f40108b7646c72c956516072d69`.
  Production, compact, and incremental report digest
  `4d876136804ad8a663cdd5fce91f04cdfab2f3bd215c7ea499b92d72dd577690` and
  `1/1/42/7`. Every returned route differs from locked C by `children=3`
  versus `children=4` at the return statement. Forest declines. Incremental
  reuse is `13/52`.
- `malformed-call` has an `ERROR` root. Raw, production, and compact report
  digest `9272deb09841144e85fd5ed32a3a6bc6b6f1d39b2221e0692db918c7f3c33d2d`.
  Incremental reports digest
  `afb72aead66613b4cb37a32bdf9aacd8a945963e1af1b6c47f0f2999359e071d`.
  Production and compact report `1/1/43/0`. Incremental reports `1/1/42/0`.
  Every returned route differs from locked C by `children=3` versus `children=4`
  at the return statement. Forest declines. Compact recovery falls back at the
  elected token. Incremental reuse is `14/59`.

The focused receipt traces every observed rewrite:

- A0 `Initializable`: 666 production, compact, and incremental rewrites; 604
  forest rewrites.
- Clean member: 7 production, compact, and incremental rewrites; zero forest
  rewrites.
- Clean call alias: 12 production, compact, and incremental rewrites; 13
  forest rewrites.
- Malformed member: 7 production, compact, and incremental rewrites.
- All other covered routes report zero rewrites.

The six existing Solidity normalizer unit tests pass. The positive control
keeps native output live. No registry or production code changes are included.

The successful focused Docker artifacts are:

- `/tmp/solidity-next-artifacts/20260823T015041Z-solidity-registry` — registry receipt;
- `/tmp/solidity-next-artifacts/20260823T015045Z-solidity-a0` — A0 receipt;
- `/tmp/solidity-next-artifacts/20260823T015048Z-solidity-tracked` — tracked census receipt;
- `/tmp/solidity-next-artifacts/20260823T015049Z-solidity-real` — unavailable corpus receipt;
- `/tmp/solidity-next-artifacts/20260823T015050Z-solidity-unit` — Solidity unit receipt;
- `/tmp/solidity-next-artifacts/20260823T015212Z-solidity-blocker-final9` — final route and document guard.

Keep `dispatch.solidity` live until the producer closes every locked-C
divergence, the malformed controls match, and the authenticated corpus is
available. Then rerun all six routes before changing the registry.

## Progress ledger

| Ratchet | Status | Before | After | Evidence |
| --- | --- | ---: | ---: | --- |
| Generic trailing-extra pass | merged in PR #453 | 2 generic passes | 1 | RST and Comment production/compact/forest/incremental witnesses plus isolated C-oracle parity |
| JavaScript returned-tree arm | merged in PR #459 | 3 fixpoint arms | 2 | pre-second-pass root-span witness, JavaScript real-corpus parity, and 30/30 valid incremental/fresh edits |
| R2 dead dispatcher arms (OCaml, Ruby, half of HTML) | merged in PR #463 | 78 dispatcher arms | 75 | real-corpus census, native-parse regression tests per language, `TestResultCompatibilityOwnershipRegistry` |
| Generic terminal-leaf mutation | merged in PR #465 | 1 tree mutation | 0 | production, compact, forest, incremental, scanner-aware corpus, and Go C-oracle receipts |
| HTML returned-tree range arm | merged in PR #467 | 2 fixpoint arms | 1 | producer unit, absolute production/compact/forest/incremental ranges and points, nonzero incremental reuse, and exact C ranges and points |
| Scala returned-tree span repairs | checkpoint A commit `c334bace7da734d40e481ee236f5293b37db9a38` | 7 Scala calls | 4 duplicate calls plus one inert marker | producer controls, exact production, compact, forest, incremental, fresh, and C ranges and points |
| Scala returned-tree duplicate calls | retirement commit `d82f9c2cadb81242cb324ba751aa2805038d4b60` | 4 duplicate calls plus one inert marker | one inert marker | mandatory fixtures, authenticated corpus census, and canonical first-pass fingerprint |
| Shared returned-tree fixpoint | merged in PR #469 | 1 inert arm | 0 | ownership denominator, focused route tests, and exact Scala C-oracle receipt |
| Rust dot-range repair | merged in PR #470 | 1 materialization subfamily | 0 | collapsed-child census and merged-left-side conflict receipts |
| Lua, Make, and Zig field projection | merged in PR #471 | 3 dispatcher arms | 0 | pre-compatibility producer and production, compact, forest, incremental, and C-oracle receipts |
| Trailing root and child spans | retirement change | 4 dispatcher arms / 9 languages | 0 | native producer, production, compact, forest, incremental, reuse, and C-oracle receipts |
| Leading root trivia | retirement change | 7 local repairs / 1 dispatcher arm | 0 | native producer, production, compact, forest, incremental, reuse, and C-oracle receipts |
| Alias-preserved wrappers | retirement change | 3 dispatcher arms | 0 | pinned alias maps, native producer, production, compact, forest, incremental, reuse, and C-oracle receipts |
| Collapsed token wrappers | retirement change | 4 local repair families / 1 dispatcher arm | 0 | native producer, production, compact, forest, incremental, reuse, and four isolated C-oracle receipts |
| Haskell and Erlang root fields | retirement change | 2 local field repairs | 0 | native producer, production, incremental, reuse, and isolated C-oracle receipts |
| Zero-width artifact repairs | merged in PR #480 | 2 language walks | 0 | Haskell scanner control-token and Typst historical repetition-fold witnesses |
| Haskell section spans | retirement commit `aadc2fed64f072499f8cc9485f7cd86db2a274c3` | 1 dispatcher arm | 0 | zero-rewrite real-corpus census, native production, compact, incremental, forest-limit, and isolated C-oracle receipts |
| Hidden root trivia | retirement commit `49d776674b2f599fa162874bbf74dc119fa9e7d4` | 1 dispatcher arm | 0 | generalized root finalization, 114 native HCL body spans, four result routes, and isolated C-oracle parity |
| D module bounds | retirement change | 1 language-local span walk | 0 | compatibility-free producer, production, compact, forest, incremental reuse, and isolated C-oracle receipts |
| Erlang replacement election and form spans | retirement commit `144b30c9ee085406335f4549272e1ae843427993` | 1 dispatcher arm | 0 | zero-rewrite real-corpus census, native producer, production, compact, forest, incremental reuse, and isolated C-oracle receipts |
| D template-call type wrappers | retirement change | 1 D subpass | 0 | generalized visible named wrapper election, compatibility-free producer, production, forest, incremental, and isolated C-oracle receipts |
| Objective-C encode and function-pointer repairs | retirement change | 2 Objective-C subpasses | 0 | exact stack equivalence, generic alias selection, production, incremental, and isolated field-aware C-oracle receipts |
| Objective-C compound struct types | retirement change | 1 Objective-C subpass | 0 | raw-shape hash equivalence, compatibility-free producer, production, census, and isolated C-oracle receipt |
| D and Objective-C subpass census | retirement change | 2 aggregate arm receipts | 6 named live subpass receipts | positive controls, exact fingerprints, and absent retired labels |
| Objective-C `sizeof` operands | retirement change | 1 Objective-C subpass / 1 dispatcher arm | 0 | retained generalized alternatives, generic result selection, compatibility-free producer, census, and isolated C-oracle receipt |
| D storage-class wrappers | retirement change | 1 D subpass | 0 | visible named wrapper election, compatibility-free producer, production, compact fallback, forest, incremental reuse, and isolated C-oracle parity |
| D variable-type qualifiers | retirement change | 1 D subpass | 0 | compatibility-free producer, production, compact fallback, forest, incremental reuse, and isolated C-oracle parity |
| D call-expression targets | retirement commit `6a650454e5698d64a0148629cfa444b3dbce6877` | 2 D subpasses / 1 dispatcher arm | 0 | compatibility-free producer, production, compact fallback, forest, incremental, and three isolated C-oracle receipts |
| Certified unary named wrappers | retirement change | 1 dispatcher arm | 0 | exact-profile census, compatibility-free producer, production, compact fallback, forest fail-closed behavior, incremental, parent links, deterministic digest, and isolated C-oracle parity |
| Scala and SQL field projection | merged in PR #522 | 4 local field repairs | 0 | native reduction, production, compact fallback, forest, incremental reuse, and isolated Scala and SQL parity |
| Dart and Elixir inherited fields | retirement change | 2 language-local field repairs | 0 | compatibility-free producer, refreshed corpus, production, compact, forest, incremental, and isolated C-oracle receipts |
| HTTP document sections | retirement change | 1 subpass / 1 dispatcher arm | 0 | zero-rewrite exact and locked census, compatibility-free producer, compact fail-closed behavior, forest, incremental reuse, and isolated C-oracle receipts |
| Bash command names | retirement change | 1 Bash subpass | 0 | compatibility-free producer, production, compact fallback, forest, incremental reuse, exact 25-case baseline at `83548f55`, and isolated C-oracle parity |
| Bash generated-command assignments | retirement change | 1 Bash subpass / 1 dispatcher arm | 0 | exact raw and production witness, production, compact direct or fallback, forest, incremental fresh or reuse, and locked C parity |
| Ninja recovery and returned-tree shape | retirement change | 1 dispatcher arm | 0 | two A0 witnesses, raw and production, compact direct, forest direct, incremental reuse, and isolated locked-C parity |
| Ledger recovery and returned-tree shape | retirement change | 1 dispatcher arm | 0 | two parser-trigger witnesses plus one A0 witness, raw and production, compact fallback, `error_root` forest fallback, incremental reuse, and isolated locked-C parity |
| FIDL versioned layout modifiers | retirement change | 1 dispatcher arm | 0 | compatibility-free producer, production, compact fallback, forest-fail-closed, incremental reuse, and isolated C-oracle parity |
| HLSL subscript-assignment declarator | retirement change | 1 HLSL member | 0 | negative dynamic precedence election, compatibility-free producer, production, compact fallback, forest, incremental reuse, and isolated C-oracle parity |
| Swift ternary source reparse | retirement change | 1 Swift subpass | 0 | exact 16-case manifest, native producer, production, compact fallback, forest fail-closed behavior, incremental fresh fallback, and isolated C-oracle parity |
| JavaScript dynamic-import token child | retirement change | 1 JavaScript subpass | 0 | exact historical controls, generic collapsed-child producer, production, direct compact, strict forest, edited incremental reuse, and isolated C-oracle parity |
| JSDoc recovery and returned-tree shape | retirement change | 1 dispatcher arm | 0 | two producer witnesses, raw and production zero-rewrite receipt, compact or fallback routes, incremental fallback, and isolated locked-C parity |

Mark a row merged only after CI and merge evidence exist. Detailed per-entry
receipts stay in the JSON registry and durable run findings stay in Hyphae.
