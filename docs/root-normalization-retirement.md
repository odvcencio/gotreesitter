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

## 2026-08-24 C and C++ dispatcher blocker receipt

Status: `KEEP LIVE / NO-GO`. Keep `dispatch.c_cpp` live.

Base commit: `ab2010d74da5330d64dbddb0d9c58969da766d6d`.
This receipt changes no parser, registry, or test behavior.

The registry arm is `dispatch.c_cpp`. It covers `c` and `cpp`.
It calls `normalizeCCompatibilityWithParser` in `parser_result_c.go`.
Its authoritative owner is `derivation_election_selection`.
The C++ subpass calls `normalizeCppMalformedClassFunctionDefinition`.
Canopy traces the dispatcher to all seven C and C++ compatibility walks.
Canopy also traces the C++ malformed-class rewrite and its field reconstruction.

Retirement requires native derivation selection for every registered witness.
Require exact production, compact, forest, incremental, and C-oracle outputs.

### Grammar and oracle identity

The grammar lock has SHA-256
`9ddb6324afd014f6ecdd1cae3dd1ba238f1e62ce03d126e6d8b267ce34d72ecb`.
It pins C to commit `ae19b676b13bdcc13b7665397e6d9b14975473dd`.
It pins C++ to commit `8b5b49eb196bec7040441bee33b2c9a4838d6967`.

The embedded Go C grammar blob has SHA-256
`9aee42825fd1446ce5b754951db26edadcdba5d2f26b61578a30e87ed2dbbd3c`.
The embedded Go C++ grammar blob has SHA-256
`d351f902c8f2ca85257a9296d3c9991862d57701ac6e9006e386ae173fd35178`.

The locked C oracle uses contract `tree-sitter-c-v1`.
It uses binding `github.com/tree-sitter/go-tree-sitter` version `v0.25.0`.
Its binding commit is `adc13ffd8b2c0b01b878fda9f7c422ce0df5fad3`.
Its runtime is version `0.25.1` at commit
`f5afe475deb7c0bae6407fb776c76824f717bb61`.
The compiler is `/usr/bin/cc`, Debian 12.2.0.

The C oracle grammar artifact has SHA-256
`adbc130b95c3a8bacda2cace3b1073f0262c35863c64cbf13aba671eaf04f20d`.
The C++ oracle grammar artifact has SHA-256
`72917cdbce9526d245ee631f06c7830b564b72dc8ab592b19ca13815f15a7f32`.

### Census and corpus coverage

The initial dispatcher census (A0) has 14 languages and 42 files.
Its committed source is `testdata/dispatcher_census_a0_manifest_v1.json`.
It excludes both C and C++.
Its parser revision is `3c55dca287c9dd6ed987c764b9aafd90b22281a2`.
The tracked census has seven fixtures and no C or C++ fixture.
Its committed source is `testdata/dispatcher_census_tracked_v1.json`.

The A0 manifest declares `corpus_sources.lock`.
That file is absent from this worktree.
The authenticated corpus lock therefore has no committed C or C++ receipt.
The grammar lock alone does not prove corpus coverage.

### Route evidence

The clean C++ witness is 61 bytes.
Its source SHA-256 is
`6bc15c37731ac9d1f14aed5157c4f22233ee04ec327b554643c48e428c04d437`.
Its locked-C digest is
`88f181c147ad7d3931da91e713bd2321a07bf0022a0091767ce994a6f94025b4`.
Raw, production, token-source production, compact, and forest match C.
Normal incremental parsing matches C but reports unsupported scanner reuse.
Token-source incremental parsing reuses five subtrees and 31 bytes.
It still diverges with digest
`95107e18b0867af36eb3db0f8ca1da82545d2f57a964bce1b1065f31ebee99d2`.

The C++ recovery witness is 347 bytes.
Its source SHA-256 is
`66a2bc245f487266b2281ae0407d2045adb5f834cc2ea236b545cafd3e94379f`.
Its locked-C digest is
`09ff7ac3e0e1aa3a9ee610b741c9926f428a1455563d11f0e1bec996ad99794e`.
Raw and production use digest
`9fa1894c3d15e4de5e58c9513be13b816a26ea467e890b29817312b45673b360`.
The first difference is `/translation_unit`, where C has an error and Go does not.
Compact declines at the scheduler frontier and falls back to production.
Forest declines. Normal incremental parsing reports unsupported scanner reuse.
Token-source production changes the digest to
`b32c5a96585e23270ea003b6a335b645adffeccdd70aa6c55b2771faa24ed8c8`.
Its first difference is
`/translation_unit/function_definition[1]/class_specifier[0]`.
The difference is field metadata: Go has no field and C has `type`.
Token-source incremental parsing reports `incremental_parse_full_retry`.
It reuses six subtrees and 11 bytes, but it keeps the field divergence.

The clean C witness is 29 bytes.
Its source SHA-256 is
`2ad75d95660563887d8d3f1d0ae1dcf18c2379cbd83a5c72f5ab276351ee6949`.
Every tested route matches C at digest
`b35547117f044e74311e70eeb45bd3967598fdca38a963937e2eeaad29bed7b7`.
Incremental parsing reuses four subtrees and 14 bytes.

The C recovery witness is 1,023 bytes.
Its source SHA-256 is
`5d50ac3fdf9303cccf76fd9c4c0be0c9c4c48839b8c2ba72939fd96494f164ba`.
Its locked-C digest is
`35c4b6da0092ff35252de94385dbe959f282c317b6904a9c1e2a380d5481fbf9`.
Go uses digest
`026f2be4af2d3105a2d0930abd4374970b48eb0deafc9a398a3472d9ddcb4c16`.
The first difference is
`/translation_unit/function_definition[0]/compound_statement[2]/ERROR[2]/number_literal[0]`.
Go marks the node as an error. C does not.
Compact declines during recovery. Forest declines.
Incremental parsing reuses six subtrees and 20 bytes, but the digest remains different.
It allocates 9,208 new nodes after the edit.

### Focused C++ field proof

`TestCppTemplateTypeParameterParity` passes.
`TestCppCollapsedKeywordCompatibilityParity` passes.
`TestCppMalformedClassFunctionDefinitionRecoveryParity` fails by design.
It reports nine missing field names on the 347-byte recovery witness:

- `type`, `name`, and `body` under the class carrier;
- `declarator` at two levels;
- `scope` and `name` under the declarator;
- `parameters` and `body` on the function definition.

The C++ field-preservation repair is language-specific.
It changes field metadata on one C++ recovery shape.
It does not resolve the C recovery divergence.
It does not resolve the token-source incremental retry.
Do not replace this arm with a C++ branch or a source-specific rule.

### Resource bounds and artifacts

Docker used image `gotreesitter/cgo-harness:go1.25-local`.
Each run used 4 GiB memory, one central processing unit, and 512 process IDs.
Each run used `GOMAXPROCS=1`, `GOFLAGS=-p=1`, and `GOMEMLIMIT=3GiB`.
The test timeout was 20 minutes.
Successful runs exited zero without timeout or out-of-memory failure.
The focused parity failure exited one because its nine-field assertion failed.

The successful route artifacts are:

- `/tmp/gts-n31e-artifacts/20260823T064647Z-n31e-c`;
- `/tmp/gts-n31e-artifacts/20260823T064713Z-n31e-cpp`;
- `/tmp/gts-n31e-artifacts/20260823T065124Z-n31e-c-routes`;
- `/tmp/gts-n31e-artifacts/20260823T065140Z-n31e-cpp-routes`.

The focused negative artifact is:

- `/tmp/gts-n31e-artifacts/20260823T065234Z-n31e-cpp-focused`.

The token-source attempt is excluded because its parity assertion failed:

- `/tmp/gts-n31e-artifacts/20260823T065042Z-n31e-cpp-token-source`.

The first C setup and package-path attempts are excluded:

- `/tmp/gts-n31e-artifacts/20260823T064616Z-n31e-c`;
- `/tmp/gts-n31e-artifacts/20260823T064622Z-n31e-c`.

The C issue #454 ratchet artifacts are excluded from this C/C++ receipt:

- `/tmp/gts-n31e-artifacts/20260823T065017Z-n31e-c-issue454`;
- `/tmp/gts-n31e-artifacts/20260823T065223Z-n31e-c-ratchet`.

### Decision and reopening conditions

Keep `dispatch.c_cpp` live. No safe generic correction is proven.
The C++ field repair is language-specific and incomplete.
The C recovery gap and token-source gap remain separate blockers.
Ship no parser, registry, or test change.

Reopen retirement only after all conditions pass:

1. Add C and C++ to the A0 manifest and tracked census denominator.
2. Restore an authenticated `corpus_sources.lock` with C and C++ entries.
3. Run every registered witness at the locked grammar revisions.
4. Prove exact raw, production, compact, forest, incremental, and C-oracle trees.
5. Preserve incremental reuse or retry receipts for both C and C++.
6. Replace the C++ field repair with a generic producer invariant.
7. Close the C recovery and token-source gaps without language-name rules.

Keep the registry arm unchanged until every condition passes.

## 2026-08-24 AWK dispatcher blocker receipt

Status: `KEEP LIVE / NO-GO`. Keep `dispatch.awk` live.

Base commit: `5648911ecf509df8ec870a1214917d9e95cf54f1`.
This receipt adds one focused test. It changes no parser or registry behavior.

The registry entry is `dispatch.awk`. It calls
`normalizeAwkCompatibility` in `parser_result_awk.go`. Its authoritative owner
is `scheduler_action_semantics`. Canopy confirms the dispatcher call at
`parser_result_compat.go:125`. Retirement requires exact production, compact,
forest, incremental, and locked-C output for every registered witness.

The grammar lock pins AWK to Beaglefoot commit
`34bbdc7cce8e803096f47b625979e34c1be38127`. The generated AWK blob SHA-256 is
`925312ca0bc6e279602402c64700b8198c55ed949ac967ce92bae40f7f21cedf`.
The A0 (initial dispatcher census) manifest has 14 languages and 42 files.
It excludes AWK. Its parser revision is
`3c55dca287c9dd6ed987c764b9aafd90b22281a2`. Its grammar-lock digest is
`9ddb6324afd014f6ecdd1cae3dd1ba238f1e62ce03d126e6d8b267ce34d72ecb`.
The manifest names `corpus_sources.lock`, but that file is absent from this
worktree. The sidecar records the expected SHA-256
`41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`.

The tracked census has seven fixtures. It has no AWK fixture. The outline
baseline has 206 languages. AWK is T4, with no tags query, smoke capture, or
recorded real corpus. The external AWK checkout has 454 files, including 316
files under `testdir`. It is pinned by the authenticated lock at
`5739fd79bcfc75ba7526773d0cf634521f8aca3c`.

The tracked fixture is
`testdata/awk_recovered_rule_split/T.gawk.b64`. Its decoded source is 7,392
bytes with SHA-256
`f3dd8c811b2ad06c865fb1ad59ac0098fef57bdbb89377ac96ddb4e845f6bfba`.
The fixture comment and the top-50 manifest cite provenance commit
`61a7c75e225e3035390be32d635545e40d8c5faf`. That commit is distinct from the
authenticated corpus-lock revision. The current external `testdir/T.gawk`
has the same bytes and SHA-256.

The focused test is
`cgo_harness/awk_dispatch_blocker_receipt_test.go`. Both witnesses cover raw,
production, compact, forest, and locked-C routes.
Only the clean witness has an incremental receipt. Recovery incremental telemetry remains absent. The clean source is 18 bytes with SHA-256
`99d1043aabedfc2a53a4d50d35fd0e5f257beb49612617c0855c37ab4baa6ec1`.
Every clean route has digest
`6cd4e8645947bff0604ea5131f9b2188322a021b84db5f3f7c729a76b330d5d2`.
The clean incremental route reuses one subtree and 18 bytes with zero
reported reparse time. The AWK external scanner is stateless, preserves state
on scan failure, and supports incremental reuse.

The recovery raw Go digest is
`6d53efe8af8b1e47aaf1defa8d2a727a6bcd43c7a9fc37516c6bb2b45ad0db56`.
Production and compact digest to
`cead9d68f270583fa37ed19b470ca4482ce315b41a30528b7432e95a07fefee8`.
Locked C digests to
`bb33c51db03cf6f16c5b206ce6d47d8369e4904f86466fecd9440311e5995925`.
The first divergence is at `/program`, where raw Go has 454 children and
locked C has 338. Production and compact have 408 children. The first deep
span differs at `rule[0:11]` in Go and `rule[0:47]` in C. Compact falls back
to production because the fresh full runner does not accept end of file.
The forest route declines. The raw-to-production digest change records live
compatibility work; route telemetry `rewrites=0` does not mean zero
normalizer rewrites.

Docker used image `gotreesitter/cgo-harness:go1.25-local`, 4 GiB memory, one
CPU, 4,096 PIDs, `GOMAXPROCS=1`, `GOFLAGS=-p=1`, and `GOMEMLIMIT=3GiB`.
The successful artifacts are:

- `/tmp/gts-n31d-awk-receipt-draft/harness_out/docker/20260823T061200Z`
- `/tmp/gts-n31d-awk-receipt-draft/harness_out/docker/20260823T061046Z`
- `/tmp/gts-n31d-awk-receipt-draft/harness_out/docker/20260823T061107Z`

These artifacts completed without timeout or out-of-memory failure. The original
generated probe sources are absent. The following failed attempts remain
excluded from the successful receipt:

- `/tmp/gts-n31d-artifacts/20260823T054932Z-awk-c-routes` failed at probe compilation.
- `/tmp/gts-n31d-artifacts/20260823T055314Z-awk-normalizer-order` failed at test setup.

No safe grammar-agnostic correction follows from one recovery mismatch. Keep
the arm live until the authenticated AWK corpus enters the A0 and tracked
coverage, recovery incremental telemetry is recorded, and every registered
witness matches locked C on all required routes. Then trace the correction to
scheduler action semantics. Ship no parser or registry change.

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

## 2026-08-24 C# dispatcher blocker receipt

Status: `NO-GO`. Keep `dispatch.c_sharp` live.

Base commit: `ef57c9d1b73bac046ef40f2a111bb76db643ebfd`.
This receipt adds one focused probe test. It changes no parser or registry
behavior.

The registry contains 88 entries. It has 31 live dispatcher arms, one live
dispatcher predicate, and 35 live dispatcher labels. The C# entry is
`dispatch.c_sharp`. Its function is `normalizeCSharpCompatibility` in
`parser_result_csharp.go`. Its authoritative owner is
`scheduler_action_semantics`. Retirement requires exact production, compact,
forest, incremental, and locked-C output for every registered witness.

The grammar lock pins C# to commit
`88366631d598ce6595ec655ce1591b315cffb14c` in
`grammars/languages.lock`. The A0 (initial dispatcher census) manifest has 14
languages, 42 files, and 14 receipts. It excludes C#. The seven-fixture
tracked census includes one C# fixture:
`testdata/parser_result/csharp/jsontextreader_excerpt.cs`. It records one
check, one run, 2,093 visited nodes, and 2,085 rewritten nodes.

The full authenticated corpus is unavailable to the committed census.
`cgo_harness/corpus_real` does not exist, so
`TestDispatcherArmCensusOverRealCorpus` skips. An external C# source checkout
exists, but it is not the committed corpus gate.

The focused probe is
`cgo_harness/csharp_dispatch_blocker_receipt_test.go`. It runs one C# grammar
workload and records raw, production, compact, forest, incremental, and
locked-C routes. The fresh route receipt is
`/tmp/gts-n31a-csharp-audit/harness_out/docker/20260823T030038Z-csharp-audit-route-final-v2`.

The A0 witness is `jsontextreader_excerpt.cs`. Its source SHA-256 is
`d76fd62cfc90076c11d86cb7d7a0058df181231aa3b34f30e549f650b5294d4a`.
The Go deep digest is
`6e5eb91f5577569ca2adebf26056095af492a5921ed09c72b05cba045dca57dc`.
The locked-C deep digest is
`17a882ecc47150a396236512827eb2dd077ff2d65d9923d79d4ba98cb0b66abf`.
The raw first divergence is at `/compilation_unit`, with five Go children and
six C children. Production, compact, and incremental report a root error flag
of `false` in Go and `true` in C. Production, compact, and incremental record
2,085 dispatcher rewrites. Forest declines.

The positive producer control is `class C { public int M() { return 1; } }`.
Its source SHA-256 is
`a6946abc5b7086a1ba0d6cd585882b3b8a20d96b6e578351cf07ecf993964362`.
Every route matches locked C at digest
`58cdc6772314e0e82478a1d5811db6c8c09d939b5ac690ced9fb9695e0946ae7`.
The production, forest, and incremental dispatch passes visit 22 nodes and
rewrite zero nodes. Compact records no compatibility pass because it accepts
the native tree. Compact and forest accept the control.

The historical issue #454 witness is 140,289 bytes. Its source SHA-256 is
`a0de6cfb0e98995f41f1bac3931a4d0300ab8d34f68dd30843afecd9ee984711`.
The Go digest is
`4e6e7e9f33ca204763aff7a4d3e8ab4aee089ad057a9515cbc37a7c9a35f49aa`.
The locked-C digest is
`d9ca44d4b6d5d7d555e5066a2c45fa329afb0fa237791746abe855fd31494ae4`.
The first divergence is
`/compilation_unit/namespace_declaration[0]/declaration_list[2]/class_declaration[1]/declaration_list[4]/method_declaration[1]/block[5]/expression_statement[1]/assignment_expression[0]/ERROR[1]/integer_literal[0]`.
Go marks the node as an error. C does not. Production marks the native
recovered structure authoritative, but the digest still differs. The
production, compact, and incremental dispatch passes visit 57,067 nodes
and rewrite zero nodes. Compact and forest decline during recovery.

The malformed witness is `class C { void M() { int x = ;`. Its source
SHA-256 is
`86a8c9f0a2ea38797add255cbbffcbe748af3c6f465d3db989b9dffd182d4ce8`.
The Go digest is
`5140aac5a98ce1a8fa774400df978fa57b87ffcbe1d66fb159a82fe0de6553e2`.
The locked-C digest is
`b252b21dc16f944cda8457956f65879a0222792efd936673448083a3b678aabc`.
The first divergence is `/compilation_unit/ERROR[0]`; C has an extra child.
The production, compact, and incremental dispatch passes visit 19 nodes and
rewrite zero nodes. Compact and forest decline during recovery.

Every incremental receipt reports `external_scanner_unsupported`. Each run
uses a fresh parse, with zero reused subtrees and zero reused bytes. The C#
external-lex-state regression passes. The `DeclaredTypeManager.cs` first-pass
guard also passes when the external source checkout is mounted. The fresh
isolated audit artifacts are:

- `/tmp/gts-n31a-csharp-audit/harness_out/docker/20260823T030038Z-csharp-audit-route-final-v2` — route receipt;
- `/tmp/gts-n31a-csharp-audit/harness_out/docker/20260823T030139Z-csharp-audit-document-guard` — document guard;
- `/tmp/gts-n31a-csharp-audit/harness_out/docker/20260823T025339Z-csharp-audit-scanner` — scanner regression;
- `/tmp/gts-n31a-csharp-audit/harness_out/docker/20260823T025346Z-csharp-audit-scanner-corpus` — mounted scanner corpus;
- `/tmp/gts-n31a-csharp-audit/harness_out/docker/20260823T025357Z-csharp-audit-registry` — registry receipt;
- `/tmp/gts-n31a-csharp-audit/harness_out/docker/20260823T025404Z-csharp-audit-a0` — A0 receipt;
- `/tmp/gts-n31a-csharp-audit/harness_out/docker/20260823T025410Z-csharp-audit-tracked` — tracked receipt;
- `/tmp/gts-n31a-csharp-audit/harness_out/docker/20260823T025418Z-csharp-audit-census` — real-corpus skip receipt.

Canopy confirms the dispatcher call from
`runLanguageResultCompatibility` to `dispatcherArmCensus`, and the C# call to
`normalizeCSharpCompatibility`. Its recovery helpers own top-level chunks,
namespaces, and type declarations. The probe does not prove that the generic
scheduler emits the C tree. It therefore does not support retirement.

Reopen C# retirement only after the authenticated corpus becomes available
and every registered recovery witness matches locked C on raw, production,
compact, forest, incremental, and C-oracle routes. Require zero dispatcher
rewrites on the matching witnesses. Keep the registry unchanged.

## 2026-08-24 TypeScript dispatcher blocker receipt

Status: `KEEP LIVE / NO-GO`. Keep `dispatch.typescript` live.

Base commit: `731f8a9d9440a006b2cc6b56ef5b31c0ff3b5ce7`.
This receipt adds one focused route test. It changes no parser or registry
behavior.

The registry entry is `dispatch.typescript`. It calls
`normalizeTypeScriptTreeCompatibilityWithParser` in
`parser_result_javascript_typescript.go`. Its authoritative owner is
`derivation_election_selection`. Retirement requires exact production,
compact, forest, incremental, and locked-C output for every registered
witness.

The grammar lock pins TypeScript and TypeScript JSX (TSX) to commit
`75b3874edb2dc714fb1fd77a32013d0f8699989f` in
`grammars/languages.lock`. The A0 (initial dispatcher census) manifest has 14
languages and 42 files. It excludes TypeScript and TSX. Its parser revision is
`3c55dca287c9dd6ed987c764b9aafd90b22281a2`. Its grammar lock digest is
`9ddb6324afd014f6ecdd1cae3dd1ba238f1e62ce03d126e6d8b267ce34d72ecb`.
The manifest names `corpus_sources.lock`, but that file is absent.

The tracked census has seven fixtures. Its TypeScript receipt is
`cgo_harness/corpus_structural/typescript_sample.ts`. The source SHA-256 is
`40b4a7a06fde353d8c2b726acb16f59aab44d49d1b6257c37345c2a1f56b9fb7`.
The receipt records one check, one run, 1,462 visited nodes, 15 rewritten
nodes, and no error root. The 15 rewrites remain active evidence. They do not
support retirement.

The full authenticated TypeScript and TSX corpus is unavailable. Neither
`cgo_harness/corpus_real/typescript` nor `cgo_harness/corpus_real/tsx` exists.
The checked-in `grammars/testdata/typescript_issue_544.ts` file is a structural
control, not an authenticated corpus receipt. The corpus manifests reference
TypeScript paths that are not present in this worktree.

The focused probe is
`cgo_harness/typescript_dispatch_blocker_receipt_test.go`. It records raw,
production, compact, forest, incremental, and locked-C routes. It also checks
the TypeScript external scanner. The scanner reports
`supports_incremental_reuse=true`.

The tracked witness matches locked C on every available route. Its deep digest
is `0c29d566e57e5bdee435a7c8f17578bc2b0e5ff53c8dfea720655fec2b9f7f39`.
Production records `dispatch.typescript` as 1,462 visited and 15 rewritten.
Compact declines with this fail-closed reason:
`converged-path reduction split no-action drop lacks alternative-set coverage
by one non-blended survivor`. Forest accepts. Incremental reuse records 406
subtrees and 2,289 bytes. Its pass records 1,462 visited and 10 rewritten.

The positive control `const value: number = 1;` matches locked C at digest
`1e38064181b465fdf83382149c49a085ccad8cc2a7fcefad67b187c6d87ee619`.
Production records 12 visited and zero rewritten. Compact accepts, forest
accepts, and incremental reuse records 5 subtrees and 18 bytes.

The typed-arrow control `const f = (a: A): B => a;` matches locked C at digest
`6c5d7858e8ca512ff1f3082e2f4be701ce95c4741ea01ddb41e1f2d681e83d00` under the
default cap. Production records 21 visited and zero rewritten. Compact falls
back, forest accepts, and incremental reuse records 6 subtrees and 10 bytes.
The generic-arrow comma control also matches locked C. Its production pass is
27 visited and zero rewritten. Compact falls back and incremental reuse records
4 subtrees and 8 bytes.

The simple generic-call control matches locked C at digest
`5fd25b615488dd97468bc371bfb05af01b4fa13d17559c52c35246d27174739b`.
Production records 14 visited and zero rewritten. Compact falls back, forest
accepts, and incremental reuse records 2 subtrees and 9 bytes.

The existing ternary generic-call selection test remains skipped. Its direct
route witness currently matches locked C at digest
`d0ba1c98d9058b3aff2795d1cf5b019a57306a6b247724f92dc961a62b802f02`.
The production pass records 51 visited and zero rewritten. Compact accepts and
incremental reuse records 9 subtrees and 52 bytes. The skipped test still
marks the unresolved PrecDynamic tie-break boundary.

The checked-in issue-544 structural control matches locked C at digest
`6049f72952e720eb432bad16a60ca80f541f16785f17d2ea143b5e4ac3422103`.
Production records 832 visited and two rewritten. Compact falls back, forest
accepts, and incremental reuse records 11 subtrees and 64 bytes.

The controlled diagnostic forces `GOT_GLR_MAX_MERGE_PER_KEY=1`. It is not the
shipping profile. The typed-arrow raw Go digest becomes
`43ea0e22e93ca342e3180c8675e86c043674bec8d056d775cffeb30f2e017a42`.
Locked C remains
`6c5d7858e8ca512ff1f3082e2f4be701ce95c4741ea01ddb41e1f2d681e83d00`.
The first divergence is `/program`, where Go has an error and C does not.
Production records 20 visited and zero rewritten at cap one. Forest still
returns the exact C tree. The generic-arrow comma form shows the same
cap-one selection family. This diagnostic identifies a parser-core boundary.
It does not prove a safe grammar-agnostic correction.

The held-out `typescript/src/lib/webworker.generated.d.ts` receipt remains
open. The 786,262-byte source fails the default 512 MiB budget with Go root
`ERROR`, C root `program`, and child counts 330 versus 1,484. The source is
absent from this worktree. This receipt was not rerun locally. The
recorded threshold remains above 576 MiB and at or below 640 MiB.

Canopy traces the first producer path from
`runLanguageResultCompatibility` to `dispatcherArmCensus`, then to
`normalizeTypeScriptTreeCompatibilityWithParser`, the fused JavaScript and
TypeScript walk, and the TypeScript candidate collector. The route evidence
does not show that the producer emits the C tree for the authenticated corpus.
No safe producer or parser-core fix is proven.

The focused artifacts are:

- `/tmp/gts-n31b-artifacts/20260823T035236Z-n31b-registry-tracked`;
- `/tmp/gts-n31b-artifacts/20260823T035715Z-n31b-typescript-core`;
- `/tmp/gts-n31b-artifacts/20260823T035737Z-n31b-typescript-parser-result`;
- `/tmp/gts-n31b-artifacts/20260823T040002Z-n31b-typescript-routes-v4`;
- `/tmp/gts-n31b-artifacts/20260823T040319Z-n31b-typescript-cap1-gap`;
- `/tmp/gts-n31b-artifacts/20260823T040340Z-n31b-typescript-lineage-parity`;
- `/tmp/gts-n31b-artifacts/20260823T040347Z-n31b-typescript-grammar-gaps`;
- `/tmp/gts-n31b-artifacts/20260823T040450Z-n31b-typescript-import-types`;
- `/tmp/gts-n31b-artifacts/20260823T040459Z-n31b-typescript-grammar-regressions`.
- `/tmp/gts-n31b-artifacts/20260823T041728Z-n31b-typescript-blocker-receipt-routes-v2`;
- `/tmp/gts-n31b-artifacts/20260823T041826Z-n31b-typescript-blocker-receipt-cap1`;
- `/tmp/gts-n31b-artifacts/20260823T041925Z-n31b-typescript-blocker-receipt-docs-v3`.

Retire `dispatch.typescript` only after all of these conditions hold:

1. Add `corpus_sources.lock` and authenticated TypeScript and TSX A0 receipts.
2. Rerun every registered witness at the locked grammar revision.
3. Prove exact raw, production, compact, forest, incremental, and locked-C
   output for every witness and control.
4. Preserve exact scanner reuse receipts on all incremental routes.
5. Resolve and unskip the generic-call selection test with a
   grammar-agnostic PrecDynamic tie-break proof.
6. Prove typed-arrow selection without a source-specific exception.
7. Close the default-budget webworker divergence with a safe condense design.

Keep `dispatch.typescript` live until each condition passes.
Keep the registry entry unchanged until each condition passes.

## 2026-08-24 Python dispatcher blocker receipt

Status: `NO-GO`. Keep `dispatch.python` live.

Base commit: `14f6692fac65eab817f65af8cc6072e423ca6563`.
This receipt adds one focused route and document guard test. It changes no
parser or registry behavior.

The registry arm is `dispatch.python`. Its function is
`normalizePythonCompatibilityWithParser` in `parser_result_python.go`. Its
authoritative owner is `scheduler_action_semantics`. Its route coverage is
production, compact, forest, incremental, and locked-C. The registry status
is `live`. Retirement requires exact native output for every registered
witness on every covered route.

The A0 (initial dispatcher census) manifest has 14 languages, 42 files, and
14 receipts. It excludes Python. Its parser revision is
`3c55dca287c9dd6ed987c764b9aafd90b22281a2`. Its grammar lock SHA-256 is
`9ddb6324afd014f6ecdd1cae3dd1ba238f1e62ce03d126e6d8b267ce34d72ecb`.
Its totals are `checked=44`, `run=44`, `nodes_visited=313572`,
`nodes_rewritten=3267`, `error_roots=20`, and `parse_errors=0`.

The tracked census has seven fixtures. Its Python fixture is
`cgo_harness/corpus_structural/python_sample.py`. Its source SHA-256 is
`3de858b73ba43ad3c2d43b9cfc08426f62dd4ae7bcf1358e3989ed311b435157`.
It records one check, one run, 1,547 visited nodes, zero rewritten nodes, and
no error root. The tracked totals are `checked=9`, `run=9`,
`nodes_visited=26022`, and `nodes_rewritten=2107`.

The authenticated Python corpus and corpus lock are unavailable. The
`cgo_harness/corpus_real` directory does not exist. The manifests reference
that directory, but they cannot provide its files. The sidecar
`cgo_harness/perf_scan/corpus_sources.lock.sha256` records
`41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`.
The sidecar records the expected corpus lock SHA, but it does not provide the
corpus. The full Python A3 corpus sweep therefore skips.

The focused test is
`cgo_harness/python_dispatch_blocker_receipt_test.go`. It pins three
witnesses, their source bytes, source SHA-256 values, locked-C digests, Go
digests, first divergences, route results, and dispatcher pass counts.
Pass counts use the format `checked/run/visited/rewritten`.

The positive witness is 32 bytes. Its source SHA-256 is
`6a1661337725eea3d5f3e26c38c3c3536f2c9fbfb66e04ae73f2dcc1a1afdd03`.
Its raw Go digest is
`1ee859d4c1d2489f24dd57e0671a1832480b1c43afb960c10f798ce9f71f9759`.
Locked C and normalized Go use digest
`577a8b7b9281fa12c48dfa239a977c82f3a94e3d248253663c4a6fafc9121622`.
The raw first divergence is
`/module/assignment[1]/pattern_list[2]`, where Go uses `pattern_list` and C
uses `expression_list`.

The raw positive witness differs from locked C, but production, compact,
forest, and incremental routes match locked C. Production records
`1/1/24/1`. Compact accepts the candidate route and records no dispatcher
pass, with counters `0/0->1/0`. Forest records `1/1/24/0`. Incremental
records `1/1/24/1`.
Raw records no dispatcher pass.

The bare-tuple f-string witness is 26 bytes. Its source SHA-256 is
`7d0029944fcffb700144302da9b1b80b03da8f89d716772b3a207dca9ba543a7`.
Its Go digest is
`89ca835ae4fb5cf19d38e40b6ae4f09c99c66987ca77d8b4921aa9f593aaa641`.
Locked C uses digest
`84c987ddc73cc06bcc63e0cc860ecaa58560a46db882c775815ecb8867f95c07`.
The first divergence is
`/module/assignment[2]/string[2]/interpolation[1]/pattern_list[1]`, where Go
uses `pattern_list` and C uses `expression_list`.

The splat f-string witness is 26 bytes. Its source SHA-256 is
`660a9ed55b63e6b98cfc70db1776895ec9046a16c906c33b3d273bee496a121d`.
Its Go digest is
`102ebedd10a3864a2640cb293f541e42f63b4f1ce3d60c9f219d7088b4f484c6`.
Locked C uses digest
`e646688923780dab15e472c1754d89e87ebfdb669fafeda109d4a2d630b4a4c9`.
The first divergence is
`/module/assignment[1]/string[2]/interpolation[1]/pattern_list[1]`, where Go
uses `pattern_list` and C uses `expression_list`.

The two f-string witnesses differ from locked C on every route. Production,
compact, forest, and incremental retain the Go digest. Their dispatcher
counts are listed below. Raw records no dispatcher pass for both witnesses.

- Bare tuple: production `1/1/22/0`, compact accepted with counters
  `1/0->2/0`, forest `1/1/22/1`, and incremental `1/1/22/0`.
- Splat: production `1/1/24/0`, compact accepted with counters `2/0->3/0`,
  forest `1/1/24/0`, and incremental `1/1/24/0`.

All route receipts set `native_authoritative=false`. Every incremental route
reports `external_scanner_unsupported`, with zero reused subtrees and zero
reused bytes. Incremental equality is therefore a fresh-parse result, not a
reuse proof.

The existing load-bearing C-oracle test passes its three assignment witnesses.
The root-error and byte-drop tests pass. The existing known-gap test skips both
f-string witnesses. The skipped cases remain explicit blockers.

Canopy traces `runLanguageResultCompatibility` to `dispatcherArmCensus` and
then to `normalizePythonCompatibilityWithParser`. It also traces the generic
selection paths `reduceForkWindowPreference` and
`stackCompareForResultSelectionWithRawShape`. The f-string tie affects
`for`, `except`, `with`, `del`, and `match` target shapes. A generic scheduler
change could affect every grammar. No safe generic correction is proven.

The Docker controls use one Python workload, one CPU, 4 GiB memory,
`GOMAXPROCS=1`, `-p=1`, and test parallelism one. The successful final
artifacts are:

- `/tmp/gts-n31c-python-blocker-20260824/harness_out/docker/20260823T052051Z-n31c-python-blocker-final5` — route and document guard;
- `/tmp/gts-n31c-python-blocker-20260824/harness_out/docker/20260823T052108Z-n31c-python-document-final6` — final document guard;
- `/tmp/gts-n31c-python-blocker-20260824/harness_out/docker/20260823T051420Z-n31c-python-receipts-final` — canonical Python receipts;
- `/tmp/gts-n31c-python-blocker-20260824/harness_out/docker/20260823T051431Z-n31c-python-census-final` — tracked census;
- `/tmp/gts-n31c-python-blocker-20260824/harness_out/docker/20260823T051442Z-n31c-python-a3-corpus-absence` — authenticated corpus skip.

Each run completed without timeout or out-of-memory failure. Maximum resident
set size was 303,840 KB for the route guard, 277,480 KB for the final document
guard, 231,200 KB for the canonical receipts, 609,280 KB for the tracked
census, and 230,560 KB for the corpus skip.

Keep dispatch.python live until scheduler_action_semantics emits the locked-C
tree for every witness and route. Reopen retirement only after the
authenticated corpus and lock become available, the A0 denominator includes
Python, both f-string gaps close, the skipped tests run, and all route digests
match. Preserve the scanner limitation as a separate incremental receipt.

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

## 2026-08-23 Corn blocker receipt

Status: NO-GO. KEEP LIVE: `dispatch.corn`. The Corn arm remains live.

Base commit: `613a0775a329998a0af0c6f24251c8008c6783aa`.

Select Corn after the Apex, Rust, Doxygen, DTD, Ada, and HLSL investigations.
Corn has the strongest remaining clean zero-rewrite A0 evidence. BitBake has
40,358 visited nodes but two error roots. Wolfram has 77 visited nodes and
three error roots. Corn has 792 visited nodes and zero error roots.

The ownership registry contains 88 entries. It contains 78 dispatcher arms,
three dispatcher subpasses, one dispatcher predicate, three generic passes,
and three fixpoint passes. It contains 31 live dispatcher arms, 33
dispatcher-arm language labels, one live predicate, 32 live entries, and 56
retired entries. It contains zero live generic passes and zero live
post-finalization arms.

The registry keeps `dispatch.corn` live. It names
`normalizeCornCompatibility` in `parser_result_corn.go`. The authoritative
owner is `scheduler_action_semantics`. The registered witness is
`cgo_harness/parity_cgo_test.go`. The registry covers production, compact,
forest, incremental, and locked-C routes.

The A0 (authenticated dispatcher census) manifest has 14 languages, 42 files,
and 14 receipts. A0 has three Corn files, three checked, three run, and zero
rewrites. It records 792 visited nodes, zero error roots, and zero parse
errors. The files are:

- `small__compact.corn`;
- `small__complex.corn`;
- `small__readme_example.corn`.

The tracked census has seven fixtures in six languages. It excludes Corn. The
authenticated real-corpus census is unavailable because
`cgo_harness/corpus_real` is not mounted. No Corn A3 receipt is registered.

The focused receipt covers six witnesses. It runs raw, production, compact,
forest, incremental, and locked-C routes. It includes three A0 files, one
quoted-path trigger, one plain-path control, and one malformed witness. Corn
has no external scanner. Every incremental route reuses the old tree and
reports `reuse_unsupported=false`.

The three A0 witnesses match locked C on every accepted route. They report
zero `dispatch.corn` rewrites:

| Witness | Source SHA-256 | Locked C and Go digest | Production pass |
| --- | --- | --- | --- |
| `a0-compact` | `e9793277f21b19593024cf7f670934333deefb54eae122caaa1f66cf41c7606a` | `6925a38d869d585460c85ab53d4d3486e675f6e6ed8e97f92e6c5702bcf951ad` | `1/1/246/0` |
| `a0-complex` | `98aaba0d478418a7855fa538cb9cd52ab81b2d2e581402d212bc3951a6a5db02` | `2e25bed381c2d79a7ba0bdd909dfbd30e738b3c8be29a7fc7352ea1fd975969a` | `1/1/287/0` |
| `a0-readme-example` | `7d412d6e3c5e396818885601df14ad092b8cbe4aac690a45d7a4c29e0410da94` | `8d705f274c5421a5a60b9ef31013e8768f0c12dbd737bedca57ce3730a644a50` | `1/1/259/0` |

The four values are checked, run, visited, and rewritten. Compact accepts all
three files. Forest accepts all three files. Incremental reuse reports 57,
97, and 138 reused subtrees for the three files.

The quoted-path trigger uses source SHA-256
`7bcdf348ffbe92ef87dcb79b45ebae007811fe7af717c2d2cdea09f8c61872c8`.
Raw Go digest is
`2e3f95ebc022876fe74c65948a03d6725855723e24592f6a56851b474b175ff8`.
Locked C digest is
`92b1b5257e650d85af9692640cf30b287453eb7ec15af25f1c1560899d3b7615`.
Raw Go differs at `/source_file/object[0]`: Go has seven children and C has
eight. The raw tree has an error root. Production, compact, and incremental
routes produce digest
`714e0b57f2920716ba5ac832d2284ac17db8189e7e12c21f4f6978e3d3936e2e`.
They still differ from locked C at that path. Production and compact routes
rewrite four nodes. Forest declines. Incremental reuse reports three reused
subtrees. The production pass reports `1/1/71/4`.
The quoted-path trigger rewrites four nodes and still differs from locked C.

The plain-path control uses source SHA-256
`40dc0c83705af7393578499a43eab93269e4b91b99f3c1ec901dad1fd76f4bd0`.
All routes match locked C with digest
`1c9dd43135dfa64e87c1af706f3fa9d66c420ec4003d54dd0e37e5d2f9896a08`.
All routes report zero rewrites. Compact accepts the control. Forest accepts
the control. Incremental reuse reports two reused subtrees.

The malformed quoted-path witness uses source SHA-256
`b86b200a916bc043d21c2b9b45f35968db1b88733c789984ae070cde7d56d324`.
Raw Go digest is
`39af32691e83a255b944db485ae4b05a164ab696da70860215805d75c7b7a73c`.
Locked C, production, compact, and incremental digest is
`e55d7e35692aa8895adedf98d61b37f5b4d6fbdbc78f5a4106ae2cac94badbdf`.
The malformed quoted-path witness differs from locked C at the error flag.
Raw differs at
`/source_file/object[0]/pair[1]/path[0]/path_seg[0]/ERROR[1]/.[0]`.
Go marks the dot error. C does not. Production performs one rewrite and
matches C. Compact falls back because the scheduler has no table action for
the elected token. Forest declines. Incremental reuse reports two reused
subtrees.

The receipt remains NO-GO. Keep `dispatch.corn` live. The quoted-path trigger
still differs from locked C after four rewrites. The malformed witness still
requires one recovery rewrite.

Keep dispatch.corn live until scheduler_action_semantics emits the locked-C
quoted-path tree. Require malformed recovery to match locked C. Require exact
production, compact, forest, incremental, and locked-C receipts. Require the
authenticated Corn corpus before retirement. Do not change the registry or
production code.

The successful focused Docker artifacts are:

- `/tmp/corn-next-artifacts/20260823T002827Z-corn-next-final` — route and document guard;
- `/tmp/corn-next-artifacts/20260823T002844Z-corn-next-document-guard-final` — final document guard;
- `/tmp/corn-next-artifacts/20260823T002433Z-corn-next-registry` — registry receipt;
- `/tmp/corn-next-artifacts/20260823T002440Z-corn-next-a0` — A0 receipt;
- `/tmp/corn-next-artifacts/20260823T002449Z-corn-next-tracked` — tracked census receipt;
- `/tmp/corn-next-artifacts/20260823T002457Z-corn-next-real-corpus` — controlled corpus skip.
## 2026-08-24 BitBake blocker receipt

Status: `NO-GO`. `KEEP LIVE`: `dispatch.bitbake`.
The BitBake arm remains live.

Base commit: `9f6380a8a0795b631a0b5e3a573253977f673bf4`.

Select BitBake after excluding the Apex, Rust, Doxygen, DTD, Ada, HLSL, and
Corn arms. BitBake has 40358 A0 visited nodes and zero rewrites. Wolfram has
77 A0 visited nodes, three error roots, and zero rewrites. BitBake has the
stronger remaining evidence.

The ownership registry contains 88 entries. It contains 78 dispatcher arms,
one dispatcher predicate, three dispatcher subpasses, three generic passes,
and three second-pass fixpoints. It contains 31 live dispatcher arms, one
live predicate, 32 live entries, and 56 retired entries. The live dispatcher
arms cover 33 language labels.

The registry records `dispatch.bitbake` as a `dispatcher_arm`. Its function is
`normalizeBitbakeCompatibility` in `parser_result_bitbake.go`. Its
authoritative owner is `scheduler_action_semantics`. Its registered witness is
`cgo_harness/parity_cgo_test.go`.

The production, compact, forest, and incremental routes use the shared
compatibility tail. The C oracle route uses the curated single-grammar parity
path.

The A0 (initial dispatcher census) manifest contains 14 languages and 42
files. A0 has three BitBake files, three checked, three run, and zero rewrites.
It records 40358 visited nodes, two error roots, and zero parse errors.

The tracked census contains seven fixtures across six languages. It excludes
BitBake and Wolfram. The authenticated real-corpus census is unavailable
because `cgo_harness/corpus_real` is absent. The focused census test skips
this lane.

The focused receipt covers eight witnesses:

| Witness | Bytes | Source SHA-256 | Go digest | Locked-C digest | Result |
| --- | ---: | --- | --- | --- | --- |
| `a0-small-error` | 259 | `fbdb85e443edd378e944e5a1416c0c4a1e485f0cd38f70a5ac75748073a15d12` | `6154c15dcc0a1f365b7cfb0bf0cc120f3e365b55a491ca793a0cf170bc5cfe92` | `6154c15dcc0a1f365b7cfb0bf0cc120f3e365b55a491ca793a0cf170bc5cfe92` | Exact. |
| `a0-medium-clang-git` | 9229 | `7deb41efd839d8b5b8b2c98589614377d12fd81fa6033824330084e07c5eaf9f` | `f9d3bffb1d39a952d36d56b4ac15e2fe69e020d2cebabc359c8e897ab4ba2c2c` | `7eb2e581801fd0dbf1ed79a9eaf2ad357062725d6b26a238a168b3e14473d064` | Diverges at `/recipe`: shape, children 72 versus 71. |
| `a0-large-linux-firmware` | 168224 | `eaa9e3f2354345d558717c4791a67144d8b27767674bf5468e157a0e0a332ff6` | `d3c65cb639ef93ddbc96a03b39abbd184ac28ee8261cd894aab3bc6cc63e048d` | `9627acb8fdaf0b4cb9adaf5c23dc0229ea5cfcdeca63e8b8856645654ff33734` | Diverges at `/recipe`: shape, children 2056 versus 2054. |
| `addtask-error-wrapper` | 964 | `35ccf9d007ef76548258088b18adb39d7c0509452b4fb62bd7dfdc94cdcdf780` | `54ff030bff3669038144d08a72aa778c6e7d5f44de6682ce0fbc78c31fc9620c` | `464f3db4c1ad8be4dabc16c9631524f4cee1d99c9c1c6f4abfad6d6bfe8cabed` | Diverges at `/recipe`: Go error false, C error true. |
| `function-flag-assignment` | 148 | `8655832f5acd3b4ada197881f41b0110657572aa0c791ecf4524fbc10603a1eb` | `6fda1b4df6c27597c4b6218dd30b0b6c55a51f4d7e7adfff8756fe05f5adef7e` | `dd7147f575544eadae2a0c5aa1ecd571495d66f40da096cba1fd9b309635bd61` | Diverges at `/recipe/function_definition[0]/ERROR[4]`: shape, children 10 versus 9. |
| `adjacent-override-functions` | 230 | `068037e13b101cf01d0a36da586f798972fa39b99ba0c2a86dc17996bc34d185` | `e939ebdd239811c6154def7a5e4c3faa1a4024f8540ece27d4e3d78bb92c7eed` | `f972c3f88404f41643400bfa2c200c72f2ddbcb93295a258c8b1f404e2cb7d9b` | Diverges at `/recipe`: shape, children 2 versus 3. |
| `plain-assignment-control` | 18 | `b6a30f2e69cfe6ad0a3ca514e855b86b3c7e9403c5106d7d2239c3f42abea292` | `99187413f4fea3d1d6a10fb61768341c4188db1bd8582cdb041837d8ff49306b` | `99187413f4fea3d1d6a10fb61768341c4188db1bd8582cdb041837d8ff49306b` | Exact no-op control. |
| `malformed-shell-function` | 40 | `248dddc07429da83d3f05a14da1137270b7715dca43aa41abf58a51109b14477` | `f3ad263fcb44ab306ad7b1cc6a510840ce86c2683c7c94df7333fdec57edeae0` | `f3ad263fcb44ab306ad7b1cc6a510840ce86c2683c7c94df7333fdec57edeae0` | Exact error tree. |

The raw and production routes emit the same digest for every witness. The
production pass checks, runs, visits, and rewrites these counts:

| Witness | `dispatch.bitbake` checked/run/visited/rewritten |
| --- | ---: |
| `a0-small-error` | `1/1/48/0` |
| `a0-medium-clang-git` | `1/1/1486/0` |
| `a0-large-linux-firmware` | `1/1/38824/0` |
| `addtask-error-wrapper` | `1/1/144/0` |
| `function-flag-assignment` | `1/1/35/0` |
| `adjacent-override-functions` | `1/1/37/0` |
| `plain-assignment-control` | `1/1/9/0` |
| `malformed-shell-function` | `1/1/7/0` |

Every raw, production, compact, and incremental tree reports zero normalization
nodes rewritten. Each accepted forest tree also reports zero rewrites.

The medium A0 witness differs from locked C at the recipe shape. The large
A0 witness has the same divergence category. The three constructed producer
witnesses also differ from locked C. The malformed shell-function witness
remains a recovery blocker.

The compact route accepts `a0-small-error` and
`plain-assignment-control`. It falls back for the other six witnesses with
reason `compact route declined at recovery [mechanism=recovery-entered]: did
not accept EOF: generic scheduler has no table action for the elected token`.

The forest route accepts `a0-small-error` and
`plain-assignment-control`. It declines the other six witnesses.

BitBake has a registered external scanner. It does not implement incremental
reuse support. Every incremental route therefore reports `reuse=false`,
`unsupported=true`, reason `external_scanner_unsupported`, zero reused
subtrees, and zero reused bytes.
The incremental digest equals the raw digest for every witness. It therefore
matches C for three witnesses and preserves the five locked-C divergences.

The raw, production, compact, forest, incremental, and C-oracle receipts do
not prove native ownership. Keep dispatch.bitbake live until scheduler_action_semantics emits the locked-C shell-function tree.
Require `scheduler_action_semantics` to emit that tree for every
registered witness. Require exact route parity and an authenticated corpus
before retirement. Do not change the registry or production state.

The current-main Docker artifacts are:

- `/tmp/gts-bitbake-audit-20260824/harness_out/docker/20260823T024229Z-bitbake-current-route-final` — route probe;
- `/tmp/gts-bitbake-audit-20260824/harness_out/docker/20260823T024239Z-bitbake-current-document-final` — document guard;
- `/tmp/gts-bitbake-audit-20260824/harness_out/docker/20260823T023912Z-bitbake-current-registry` — registry receipt;
- `/tmp/gts-bitbake-audit-20260824/harness_out/docker/20260823T023919Z-bitbake-current-a0` — A0 receipt;
- `/tmp/gts-bitbake-audit-20260824/harness_out/docker/20260823T023927Z-bitbake-current-tracked` — tracked census receipt;
- `/tmp/gts-bitbake-audit-20260824/harness_out/docker/20260823T023935Z-bitbake-current-real-corpus` — real-corpus availability receipt.

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

## 2026-08-24 Authzed dispatcher blocker receipt

Status: NO-GO. Keep `dispatch.authzed` live.

A0 means the initial dispatcher census. This receipt uses evidence base
`ab2010d74da5330d64dbddb0d9c58969da766d6d`; its publication base is
`5d39d9658f5071c5c0f476eaadc6ae067e6c77e1`.

The registry has 88 entries, 32 live entries, and 56 retired entries. The
live denominator has 31 dispatcher arms. The Authzed arm calls
`normalizeAuthzedCompatibility` in `parser_result_authzed.go`.

The registry assigns this arm to `scheduler_action_semantics`. It lists
`cgo_harness/parity_cgo_test.go` as its witness. It requires exact native
output for every registered witness and every production, compact, forest,
incremental, and C-oracle route.

### Ownership and producer reachability

Canopy traced this call chain:

```text
Parse or parseWithTokenSource
  -> normalizeReturnedTreeForParse
  -> normalizeReturnedTree
  -> normalizeResultCompatibility
  -> applyResultCompatibility
  -> runLanguageResultCompatibility
  -> normalizeAuthzedCompatibility
```

The incremental chain calls the same compatibility tail after changed-tree
parsing. Compact and forest routes enter through result finalization before
the same compatibility tail. Canopy found no shared producer invariant that
explains the Authzed mismatches below.

The dispatcher invokes these ten direct passes:

```text
normalizeAuthzedCleanRootShape
normalizeAuthzedObjectCaveatRecovery
normalizeAuthzedUnclosedCaveatRecovery
normalizeAuthzedStrayCaveatTailRecovery
normalizeAuthzedSingleQuotedCaveatRecovery
normalizeAuthzedSingleQuotedCaveatBlockRecovery
normalizeAuthzedUnsupportedUseDirective
normalizeAuthzedMalformedDefinitionRoot
normalizeAuthzedMissingPermissionExpression
normalizeAuthzedWholeRootErrorTrivia
```

Canopy found these helper members in the same file:

```text
authzedDefinitionFromFlatRootChildren
authzedRebuildSingleQuotedCaveatBlock
authzedNormalizeDefinitionInvalidRelation
authzedPartialRelationBeforeError
authzedRelationFieldIDs
authzedNormalizeMalformedDefinitionErrorChild
authzedNormalizeMalformedDefinitionErrorChildChildren
authzedCollapseToSourceFileError
authzedCollapseToSourceFileWithLeadingError
authzedUnsupportedUseDirectiveStart
authzedLeadingCommentChildren
authzedLeadingDefinitionsBeforeUse
authzedMalformedDefinitionErrorChildren
authzedCoalescePartialPermissionBeforeLineError
authzedPermissionFieldIDs
authzedFindDirectChild
authzedRecoveryChildren
authzedFindDirectChildText
authzedNextByte
authzedLineEnd
authzedNextNonHorizontalSpace
authzedExtraError
authzedCaveatExpressionStatement
authzedInterpretedStringExpressionStatement
authzedIsBinaryOperator
authzedBinaryExpressionFieldIDs
authzedCaveatFieldIDs
authzedDefinitionFieldIDs
authzedAppendRootTailFromSource
authzedSimpleDefinitionFromSource
authzedSimpleBlockFromSource
authzedLeafByName
authzedEOFLeaf
authzedLeaf
authzedSetNodeRange
authzedHasNewlineBeforeNextRetainedChild
authzedRetainedErrorTriviaChildren
```

### Registry, census, and locks

The Authzed arm has three A0 files, three checked files, and three run files.
The A0 receipt records 8,326 visited nodes, 18 rewrites, one error root, and
zero parse errors. Its parser revision is
`3c55dca287c9dd6ed987c764b9aafd90b22281a2`, older than this receipt base.

The tracked census has seven fixtures, six languages, nine checked files,
nine run files, 26,022 visited nodes, 2,107 rewrites, and zero error roots.
It has no Authzed fixture. It is a small continuous-integration ratchet and
does not replace the authenticated corpus.

The A0 Authzed files are:

| Witness | Bytes | Source SHA-256 | Authenticated source |
| --- | ---: | --- | --- |
| `large__superlarge.zed` | 106126 | `86921255ed996dbf67a519adf1e33d5353aa873f025f77e2150ded76ac81197c` | `authzed/spicedb` commit `024601e66c148a398be733f06f444e14c57ccd25`, `pkg/schemadsl/parser/tests/superlarge.zed` |
| `small__doccomments.zed` | 692 | `fac84d359ebf4b628e54f567b30657792550d0c58441c42fb10a2a044b379574` | same source commit, `pkg/schemadsl/parser/tests/doccomments.zed` |
| `small__localimport_with_quotes_in_quotes.zed` | 280 | `a78131bee5849e2ce1b002605896a090a0003e052574e76ea469c3b895c534e0` | same source commit, `pkg/schemadsl/parser/tests/localimport_with_quotes_in_quotes.zed` |

The grammar lock pins `authzed` to
`https://github.com/mleonidas/tree-sitter-authzed` commit
`83e5c26a8687eb4688fe91d690c735cc3d21ad81`. The lock file SHA-256 is
`9ddb6324afd014f6ecdd1cae3dd1ba238f1e62ce03d126e6d8b267ce34d72ecb`.

Authzed has no external scanner. The Go token source supports incremental
reuse. The C oracle is tree-sitter `0.25.1` at commit
`f5afe475deb7c0bae6407fb776c76824f717bb61`. Its grammar uses the same locked
commit. The loaded oracle artifact SHA-256 is
`a4751c61607fc68c14adfe4b438390c7a5555bb2f58f690002be67889e228159`.

The authenticated corpus lock is unavailable. The repository has no
`corpus_sources.lock` or `cgo_harness/corpus_real` directory. The available
sidecar records only the expected lock SHA-256
`41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`.

### Route receipt

The focused probe used 11 witnesses: three A0 files, two clean positive
controls, and six malformed recovery controls. Every Go route compared its
tree with the locked C oracle. `=` means exact deep digest equality. `!=`
means that the listed first divergence remains.

The focused test asserts every source and locked-C digest. It asserts route
mode, divergence path and category, rewrite counts, and incremental reuse state.
It does not pin reuse subtree counts or reused bytes. Those counts do not define
the blocker. It does not pin visited counts because the A0 denominator drifts.
It does not pin whitespace-only divergence values.

The clean controls are `positive-clean-schema`, 388 bytes with source SHA-256
`32ac093bcf4c41596f2d6ace463df54bdd2d8ad3880ad0241c0993612a56b502`, and
`positive-empty-definition`, 19 bytes with source SHA-256
`5b622d08904a68f8dc95905b1807d0792434f203aa3962084aa8c7ff60606e71`.

| Witness | Go digest | C digest | Raw | Production | Compact / forest | Incremental |
| --- | --- | --- | --- | --- | --- | --- |
| `a0-large-superlarge` | `d9c96edd4ddc6b69484fb1fc71b1f5da15386509b91d3b7a797ef1d849e08345` | same | `=` | `=`, 0 rewrites | accepted `=` / declined | `=`, reuse 5182 subtrees and 99087 bytes |
| `a0-small-doccomments` | `d40e29166c95e851e9c4e924cb588bde163b118c2f336df7cd631e09638551ce` | `3106320fc4b5649e232fc96f368a1bb8d25d3eae3baa3b30e4042692176981fd` | `!=` at `/source_file`, children 16 versus 6 | `!=`, 0 rewrites | fallback / accepted `!=` | `!=`, no reuse; `forest_recovery_fallback` |
| `a0-small-localimport` | `febf866c4e81009c7dc7be7abca2716bb7dba6a59696c8bae222a13f65c8c069` | `442465537f4e5ee65298701ef8832b6c326a48c6523c40abeb25eaa105601d25` | `!=` at `/source_file/ERROR[2]/\\n[0]` | `=`, 17 rewrites | fallback / declined | `=`, reuse 2 subtrees and 118 bytes |
| `positive-clean-schema` | `e5a70c33ef46081de56bd282ed8b314a17577fed33fe18390afcd2626da3a15c` | same | `=` | `=`, 0 rewrites | accepted `=` / declined | `=`, reuse 52 subtrees and 301 bytes |
| `positive-empty-definition` | `9314a3abd9b53ae58dd026fc30d8929a07c4fdbb72857a63d1a650b15844bb6c` | same | `=` | `=`, 0 rewrites | accepted `=` / accepted `=` | `=`, reuse 4 subtrees and 16 bytes |

The refreshed A0 production routes visited 8,322 nodes and rewrote 17 nodes.
The static receipt reports 8,326 nodes and 18 rewrites. This four-node and
one-rewrite drift blocks a denominator update until the A0 run uses this
parser base.

The compact route accepted three witnesses. It fell back on eight witnesses.
The fallback reason was: `generic scheduler has no table action for the
elected token` while recovery did not accept end of file. The forest route
accepted two witnesses and declined nine witnesses.

### Malformed and recovery controls

| Witness | Bytes / source SHA-256 | Go digest | C digest | First divergence | Production / incremental |
| --- | ---: | --- | --- | --- | --- |
| `recovery-single-quoted-caveat` | 177 / `9bc941c8a15cdc634cc27e8b0694b1e2d7f42c76f32c3eabb06eaa4556d3077b` | `320b6bec5f40133b859fefe9ab71b6e9365d43daff175deba61f9e8ec90bbfeb` | `1e725f32692398293473557052f21ded90b7b638e8a26367b8fcbe3243978078` | `/source_file/\\n[5]`: Go empty type, C `}` | `!=`, 0 / `!=`, reuse 20 and 124 bytes |
| `recovery-stray-caveat-tail` | 84 / `a15377a7a655c1543f7146b0667f9862cae28a288d04a09902709348b0ff2149` | `40cb42f2872b8e27b26adf238c56ba1ead92e12c035d84588a47605b05df7c8c` | `50d04110529fd05ab2d699c5fea863bb9a90e6c485339bb83979f851664ad68b` | `/source_file/\\n[3]`: Go empty type, C `}` | `!=`, 0 / `!=`, reuse 10 and 58 bytes |
| `recovery-unclosed-caveat` | 114 / `e8d2fcc0194cf928e66ba9c5f9c7db189766848513fcec7683f9d4c69a6ea6ac` | `21fc05856ec3e6188a3d37e9763067750f6ca5bc3a6114560033e2897ade1ecd` | `281c359752fd28f8e51bae5329e70d9edfd78be0ba115ae022e502b38cbca93b` | `/source_file/caveat[2]/block_c[3]/ERROR[2]/{[0]`: Go error, C no error | `!=`, 0 / `!=`, reuse 15 and 82 bytes |
| `recovery-use-directive` | 110 / `54f2f51dd7c0a9d03c312e346224eb56144f84c1d8abf143af11f4578aee18f9` | `f62b682b3119d3e50336e18ec258f6ceca80a9aaac89cc264a7941a75448581e` | `e4eef8bc22e9f86a8aec754a2020a503e48ca5d70ffca10d5ce80fc358a8acdf` | `/source_file/ERROR[0]/\\n[0]`: Go error, C no error | `=`, 11 / `=`, reuse true, zero subtrees and zero bytes |
| `recovery-missing-permission-expression` | 44 / `c70345ec2be6e59fee29a9913ae4983cac0f775d4f69bad97b11c001e5c771ab` | `d92cae8ce3fe890414a86663d628ed38ad44ad9440427d0b74224bc275d3bbb7` | same | none | `=`, 0 / `=`, reuse 6 and 34 bytes |
| `recovery-malformed-relation` | 43 / `7e36b3bf0f6f2be9c578aa5393ca82f2d5e3ba9768ada6ebfa1884c09fe5e2cb` | `19e021d8e44fef573b634a370fd3bba4e6175f315e041cfcbcb802d48a8a6cb3` | `fdbf594f12c71c8924e103838bc96ce963008f897bfed7a4355c2dff7bda3cf2` | `/source_file/ERROR[0]/\\n[5]`: Go error, C no error | `!=`, 0 / `!=`, reuse 5 and 34 bytes |

All malformed compact routes fell back. All malformed forest routes declined.
The incremental route matched production for every witness. It reused the
old tree except for `a0-small-doccomments`, which reported unsupported reuse
after forest recovery fallback.

The clean controls matched the C oracle on every accepted route. They show
that the probe can observe zero rewrites and exact native output. The
`a0-small-localimport` witness needs 17 production rewrites. The
`recovery-use-directive` witness needs 11 production rewrites. These positive
rewrite controls keep the arm live.

### Authzed decision and reopening conditions

No safe shared producer invariant was identified. The root shape mismatch,
the local-import repair, and the recovery mismatches use different producer
invariants. A shared parser-core change would risk unrelated languages.

Keep `dispatch.authzed` live. Ship no production or registry change.

Keep `dispatch.authzed` live until a producer change closes every recorded divergence.
Remove the 17-node local-import repair and the 11-node use-directive repair.
Require exact deep parity on all registered witnesses and all six routes.
Require the authenticated Authzed corpus and its source lock.
Re-run A0 and the tracked census against the current parser base.
Use a generic parser-core fix only when Canopy proves a shared producer
invariant. Do not add an Authzed-specific branch.

Focused Docker artifacts:

- `/tmp/gts-n31f-authzed-artifacts/20260823T083429Z-rebased-routes/container.log`;
  SHA-256 `6264cfdea00d4b04a311c4af674cfe3b87b15efdb67f21fa8319b34ac8a76b37`.
- `/tmp/gts-n31f-authzed-artifacts/20260823T083429Z-rebased-routes/inspect.json`;
  SHA-256 `537ed43f9d373a2f7bc886413efbdbc8e42d99ba673bb103e342c121c0db1017`.
- `/tmp/gts-n31f-authzed-artifacts/20260823T083429Z-rebased-routes/metadata.txt`;
  SHA-256 `95cc9e7b3a58b58b4bb36cf340d5ad625a0cad143b90d68da1b90ca95f263964`.
- `/tmp/gts-n31f-authzed-artifacts/20260823T083450Z-rebased-document/container.log`;
  SHA-256 `a9dcdee98656bb472baa0683eb879d484762a2f029aaa6b99ecb000298ebbfc1`.
- `/tmp/gts-n31f-authzed-artifacts/20260823T083450Z-rebased-document/inspect.json`;
  SHA-256 `d1c4ee1f580266ec75914384ae4a5f9cc32d052243d2031741b184fc17002e4d`.
- `/tmp/gts-n31f-authzed-artifacts/20260823T083450Z-rebased-document/metadata.txt`;
  SHA-256 `f0c069027cbc32233da2dbb049488065a09d7a4aca57196bc88fcdfa92390d09`.
- `/tmp/gts-n31f-authzed-artifacts/transcript/authzed-transcript.jsonl`;
  SHA-256 `367eeeb4764eadc071f85ce75c094947e1074e66380729b4b1bf00320fe2f023`.

The transcript has eight `dispatch.authzed` arm records. Its effect objects
are empty, so the pass counters above provide the exact rewrite counts.

The route Docker run used one Authzed grammar, one CPU, 4 GiB memory,
512 process slots, a 3 GiB Go memory limit, `GOMAXPROCS=1`, `GOFLAGS=-p=1`,
one test worker, and a 20-minute test timeout. It passed with no out-of-memory
kill and no wall timeout. Maximum resident set size was 231200 KiB. The
document guard used the same limits and passed with no out-of-memory kill or
wall timeout. Its maximum resident set size was 231520 KiB.


## 2026-08-23 Dart dispatcher blocker receipt

Status: `KEEP LIVE / NO-GO`. Keep `dispatch.dart` live.
The Dart arm remains live.

Evidence base: `7b6f40fe089283674f5d0d19408d2380f77caf68`.
Publication base: `09cb5faa41af35a6bc84fefccbab1a17850d38cc`.
This isolated draft changes only the receipt and its focused ratchet test.

### Dart ownership and producer reachability

The registry contains 88 entries. It contains 78 dispatcher arms, one
dispatcher predicate, three dispatcher subpasses, three generic passes, and
three second-pass fixpoints. It contains 31 live dispatcher arms, 33 live
language labels, 32 live entries, and 56 retired entries.

The registry entry is `dispatch.dart`. Its function is
`normalizeDartCompatibility` in `parser_result_dart.go`. Its authoritative
owner is `derivation_election_selection`. Its witness is
`cgo_harness/parity_cgo_test.go`. Its production, compact, forest, and
incremental routes use `shared_result_compatibility_tail`. Its C-oracle route
uses `curated_single_grammar_parity`.

Canopy traces this call chain:

```text
Parse or parseWithTokenSource
  -> normalizeReturnedTreeForParse
  -> normalizeReturnedTree
  -> normalizeResultCompatibility
  -> applyResultCompatibility
  -> runLanguageResultCompatibility
  -> normalizeDartCompatibility
```

The Dart function calls helpers for free calls, function-type calls, nested
relational calls, generic return calls, and constructor signatures. These
helpers repair returned-tree choices after derivation selection. Canopy found
no shared producer invariant that closes the recorded Dart differences.

Retirement requires native derivation selection for every registered witness.
Require exact production, compact, forest, incremental, and locked-C output.

### Dart grammar, oracle, and census identity

The grammar lock pins Dart to
`https://github.com/UserNobody14/tree-sitter-dart` commit
`0fc19c3a57b1109802af41d2b8f60d8835c5da3a`.
The grammar lock SHA-256 is
`9ddb6324afd014f6ecdd1cae3dd1ba238f1e62ce03d126e6d8b267ce34d72ecb`.
The embedded Dart grammar blob SHA-256 is
`06bac15a9921a2e6af2810fb37ecb29a358b120e137345b9af5fb5f6c6632f59`.

The locked C oracle uses contract `tree-sitter-c-v1`.
It uses binding `github.com/tree-sitter/go-tree-sitter` version `v0.25.0`.
Its binding commit is `adc13ffd8b2c0b01b878fda9f7c422ce0df5fad3`.
Its runtime is version `0.25.1` at commit
`f5afe475deb7c0bae6407fb776c76824f717bb61`.
The compiler is `/usr/bin/cc`, Debian 12.2.0.

The Dart external scanner is present and supports incremental reuse.
The tier receipt records the default election and `0/40` staged-state parity.
The parser limits Dart incremental reuse to sources up to 256 KiB.

A0 means the initial dispatcher census. A0 has 14 languages and 42 files.
It has 14 receipt rows, 44 checked files, 44 run files, 3,267 rewrites, and
20 error roots. It has no Dart entry or receipt.
Its parser revision is `3c55dca287c9dd6ed987c764b9aafd90b22281a2`.

The tracked census has seven fixtures across six languages. It has nine
checked files, nine run files, 2,107 rewrites, and no error roots.
It has no Dart fixture. The authenticated corpus is unavailable because
`cgo_harness/corpus_real` and `cgo_harness/perf_scan/corpus_sources.lock` are
absent. The sidecar records only expected lock SHA-256
`41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`.

The two existing Dart selection-fidelity tests remain skipped. They cover a
single type-argument free call and a complex generic return call.

### Dart focused route receipt

The focused probe uses eight clean witnesses and one malformed recovery
witness. It asserts each source SHA-256, locked-C digest, route digest, first
divergence path and category, `dispatch.dart` pass rewrites, compact mode,
forest mode, and incremental reuse state.

The common positive controls are the class, private, and enum constructor
witnesses. Every route matches locked C for each control. The relational
control also matches locked C on every route without a rewrite.

| Witness | Bytes | Source SHA-256 | Locked-C digest | Raw / production / compact / forest / incremental |
| --- | ---: | --- | --- | --- |
| `single-type-free-call` | 56 | `09d754270caf75b1ff52126fcbe2ed8cccaf34cb05fbda8ba24b042f01297e51` | `4d1969523ca9d897b2d5154740cc3b34372ee6e8b100384cbec327eef80e5f26` | `4592ebedc702c08c6694e1631a31acdb1a9aedd068110c2961194f897d28de58` / same / same / C / same |
| `complex-void-function-type-call` | 167 | `98073341a8cdc5dfb052f667072fa968141cd1538852c9143db2619991971210` | `f06b1f0088eead7eb346506de36942dfaea6d9a1fd2089d23ac683cb620eddac` | `c2865b8258531e7d5caaa5b79ea036ab1cfd64518328f627f72713b1707f41ab` / C / C / C / C |
| `nested-function-type-call` | 211 | `9fc3f24b921fa53d0e66ebe1b7ba216a7e4ac7e4b5bfb23688c3b4ca442bbb0f` | `fe747a4e459dd83d2c02d7d21d4e3d608367fd03e2609986464a1fcaf2f94760` | `adae85234671a704c4b9efd5a6c3fcfc6f445db4cdfca6f505b8a3f58357a84c` / C / C / C / C |
| `complex-generic-return-call` | 170 | `a40a05fc1cdace3bd60fa334475c12ecfe1e804c1fca55612483addf9cb3059a` | `03aac429467894dc376a66896c792e92a5236ec403cffa8517cffb8c929b1dfc` | `b424eb39c84e034d2dd3acdf3522f452e0517cccea06da36c0ba4b39dbd7e0ae` / same / same / C / same |
| `class-constructor` | 46 | `af8089b2a7696122fc2d2a8197469c9b484a0dded579f7b3bd4275d5d9b2b164` | `4f0ac97990578f9eb695a85db58d310c9674ca58c702a22937bcb224ea95f0b1` | C / C / C / C / C |
| `private-constructor` | 91 | `c9fbb9bddc9aa395934e00a3a45df2bab56b1285584876cca563e17f8e94197d` | `b5f6fcdde084e50cfc8dd4fce75514f5a632e52130e52a70c411bf42c89366d8` | C / C / C / C / C |
| `enum-constructor` | 112 | `5cf4ddaf26fd35854bcd5b41a5bbf0a6ed0bb1717cf98bed6825adda041f3f7c` | `ac3f1a92f0ec55fe7862e61bde3b92cfab1a4f393dbfab9280c10d5243902285` | C / C / C / C / C |
| `relational-control` | 31 | `54b13169d4e0b73846307abcd87184c19e4c0304e9ce077e461d7e1d9460a8a3` | `8545e51b499fd18fe3aa153fcdeef2873a2637567ad4fe6a87a9ec85adc186b0` | C / C / C / C / C |
| `library-recovery` | 9 | `09c63fb57f8540f571c1defda4fbdc59ec9ec1cdfe3c3e23a1613c083abc04e7` | `324de0a5a06943713b7a9346b4029a8d35f796a8144f2f748c78afcd7662e5f9` | C / C / C / C / C |

The single-type witness differs from locked C on raw, production, compact,
and incremental routes. Its first difference is
`/program/class_definition[0]/class_body[2]/declaration[1]/initialized_identifier_list[1]/initialized_identifier[0]/relational_expression[2]/identifier[0]`.
The category is `type`. The forest route rewrites 10 nodes and matches C.
The compact route falls back. Incremental parsing reuses five subtrees and
18 bytes.

The void and nested function-type witnesses differ from C on raw only.
Production, compact, forest, and incremental routes match C after four
`dispatch.dart` rewrites on production, compact, and incremental routes.
Their first raw difference is the same relational-expression path under
`class_body[4]`. Its category is `type`. Incremental parsing reuses five
subtrees and 26 bytes for each witness.

The generic-return witness differs from C on raw, production, compact, and
incremental routes. Its first difference is
`/program/class_definition[0]/class_body[4]/declaration[1]/initialized_identifier_list[2]/initialized_identifier[0]`.
The category is `shape`; Go has three children and C has four.
The forest route rewrites 44 nodes and matches C. Incremental parsing reuses
five subtrees and 26 bytes.

The constructor controls match C on every route without a rewrite.
Their compact routes fall back because the fresh full runner rejects EOF.
Their forest routes are accepted. The relational control compact route is
accepted without a `dispatch.dart` pass. Its incremental route reuses five
subtrees and 13 bytes.

The malformed `library;` witness matches C on raw, production, compact, and
incremental routes. Compact falls back during recovery. Forest declines.
Incremental parsing reuses two subtrees and 14 bytes.

Production pass counts are `1/1/27/0`, `1/1/62/4`, `1/1/78/4`,
`1/1/63/0`, `1/1/19/0`, `1/1/28/0`, `1/1/46/0`, `1/1/20/0`, and
`1/1/6/0` in witness order. Forest pass counts are `1/1/27/10`,
`1/1/62/0`, `1/1/78/0`, and `1/1/63/44` for the first four witnesses.
All later accepted forest controls report zero rewrites.
Visited counts are diagnostic. The focused test ratchets checked, run, and
rewritten counts.

Every returned Go route reports `NativeRecoveredStructureAuthoritative=false`.

### Dart decision, limits, and reopening conditions

The raw, production, compact, forest, incremental, and locked-C receipts do
not prove native ownership. Keep `dispatch.dart` live.
No safe shared producer invariant was identified. Ship no parser, registry,
or production change.

Reopen retirement only after all conditions pass:

1. Restore an authenticated Dart corpus and its source lock.
2. Add Dart to the A0 and tracked denominators, or add an authenticated Dart receipt.
3. Make derivation election emit the locked-C tree for every generic-call witness.
4. Prove exact production, compact, forest, incremental, and locked-C output.
5. Preserve the bounded Dart scanner-reuse receipt through 256 KiB.
6. Remove the Dart rewrite helpers only after the producer proof passes.

Do not add a Dart-specific parser branch. Do not change the registry or
production state until every condition passes.

The successful focused Docker artifacts are:

- `/tmp/gts-n31h-dart-artifacts/20260823T094153Z-n31h-dart-ratchet-fix-final` — route ratchet; `container.log` SHA-256 `d096d137b5f4b25edc4d8a1f87b2067cd26e43d18e0229ff608a0116a0a82563`.
- `/tmp/gts-n31h-dart-artifacts/20260823T094327Z-n31h-dart-document-fix-final` — document guard; `container.log` SHA-256 `1102ce9697cb851d9d8bdac0de3b1cdef9bb0b123c736e4c8db47b7648e3a8e0`.
- `/tmp/gts-n31h-dart-artifacts/20260823T091057Z-n31h-dart-registry` — registry gate; `container.log` SHA-256 `04c3d0e371a1be593aebe1f1723dc99f634c4cc3ea4dd73a1963cf00f4c16059`.
- `/tmp/gts-n31h-dart-artifacts/20260823T091105Z-n31h-dart-a0` — A0 gate; `container.log` SHA-256 `9b732373da75a783947ab8a2cebbc685763d2980800461c07ffb0754096c1346`.
- `/tmp/gts-n31h-dart-artifacts/20260823T091113Z-n31h-dart-tracked` — tracked census gate; `container.log` SHA-256 `fd098996c733a13d11f268796a17957ae9c432c87e9c9209a21a8549bce5c19d`.
- `/tmp/gts-n31h-dart-artifacts/20260823T091121Z-n31h-dart-real-corpus` — controlled unavailable-corpus result; `container.log` SHA-256 `a1a4c0590e4788b508dd36f5d6d9be33a4a0e8f3f96df45081bbb784b341384e`.

Each run used one grammar, one CPU, 4 GiB memory, 512 process IDs,
`GOMAXPROCS=1`, `GOFLAGS=-p=1`, and a 3 GiB Go memory limit.
The route run used a 20-minute timeout. Every successful run exited zero.
No run timed out or exhausted memory.

## 2026-08-24 Cooklang dispatcher blocker receipt

Status: `KEEP LIVE / NO-GO`. Keep `dispatch.cooklang` live.
The Cooklang dispatcher arm remains live.

Evidence base: `7498a678c52029a82f312e9637ecb66b15defa0b`.
Publication base: `675697a1144fad306489c5142aedaae0825545d9`.
This receipt changes only the receipt document, CHANGELOG.md, and focused test.
It changes no parser, registry, or production code.

The registry contains 88 entries. It contains 78 dispatcher arms, three
dispatcher subpasses, one dispatcher predicate, three generic passes, and
three fixpoint passes. The live denominator contains 31 dispatcher arms, 33
dispatcher-arm language labels, one predicate, 32 live entries, and 56
retired entries. It contains zero live generic or fixpoint passes.

The registry names `normalizeCooklangCompatibilityWithCensus` in
`parser_result_cooklang.go`. Its owner is `scheduler_action_semantics`.
The registered subpass is `dispatch.cooklang.recovered-recipe`. It names
`normalizeCooklangRecoveredRecipe` in the same file. Its witness is
`parser_result_test/materialization_subpass_census_test.go`. Its purpose is to
construct recovered steps, metadata, punctuation errors, and fence comments.
The arm witness is `cgo_harness/parity_cgo_test.go`.

The locked grammar is `tree-sitter-cooklang` at commit
`4ebe237c1cf64cf3826fc249e9ec0988fe07e58e`.
The grammar lock SHA-256 is
`9ddb6324afd014f6ecdd1cae3dd1ba238f1e62ce03d126e6d8b267ce34d72ecb`.
The embedded Go grammar blob SHA-256 is
`2fd57f20461bcc0830fd14420cd06ad08750dd3cbbc440670e63056d59b3692a`.
The Cooklang language has an external scanner.

The A0 (authenticated dispatcher census) manifest has 14 languages, 42 files,
and 14 receipts. Cooklang has three files, three checked, three run, 1037
nodes visited, 1021 rewrites, two error roots, and zero parse errors.
The A0 aggregate has 44 checked, 44 run, 313572 nodes visited, 3267 rewrites,
20 error roots, and zero parse errors.

The Cooklang A0 files are:

- `testdata/dispatcher_census_a0/cooklang/medium__complex_test_recipe.cook` — source SHA-256 `6120e9cafce48a745c0f5dade752499883bc2b07230cae93d6a452a788c7ba74`;
- `testdata/dispatcher_census_a0/cooklang/medium__frontmatter_test_recipe.cook` — source SHA-256 `956fcaf1c14e0e915efded324450789cb5d9a896cf60e4feafb71b918c8e621a`;
- `testdata/dispatcher_census_a0/cooklang/medium__test_recipe.cook` — source SHA-256 `1acb11626700218ebc8ff8b7d445e1a257b12af35dea7dbc0fcef0587a79468f`.

The tracked census has seven fixtures across six languages. It omits Cooklang.
Its aggregate has nine checked, nine run, 26022 nodes visited, 2107 rewrites,
and zero error roots.
The authenticated real-corpus census is unavailable. The worktree lacks
`cgo_harness/corpus_real`, `corpus_sources.lock`, and
`cgo_harness/perf_scan/corpus_sources.lock`. The A0 sidecar records only the
expected corpus lock SHA-256
`41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`.

The focused receipt covers nine witnesses. It covers raw, production, compact,
forest, incremental, and locked-C routes. The test pins each source and C
digest. It records both Cooklang pass identities on every route that runs a
pass. Every Go tree reports
`NativeRecoveredStructureAuthoritative=false`. Forest declines seven witnesses
and accepts two controls.

The three A0 witnesses diverge from locked C on raw, production, compact, and
incremental routes. Forest declines all three witnesses. The first differences
are:

- `medium__complex_test_recipe.cook`: raw first differs at `/recipe`, shape.
  Go has `children=22`. C has `children=42`. Recovery has `children=23`.
- `medium__frontmatter_test_recipe.cook`: raw first differs at `/recipe`, shape.
  Go has `children=36`. C has `children=47`. Recovery has `children=35`.
- `medium__test_recipe.cook`: raw first differs at `/recipe`, shape.
  Go has `children=34`. C has `children=43`. Recovery changes the first
  difference to error, `false` versus `true`.

The A0 rewrite receipts are `1/1/450/442`, `1/1/289/287`, and `1/1/298/292`.
The values are checked, run, visited, and rewritten. Both registered Cooklang
pass identities report each value. The focused guard ratchets checked, run,
and rewritten values. It treats visited values as diagnostic.

The three raw-digest controls from rejected pull request (PR) #793 are
retained.

- `Add @salt{1%tsp}.\n` uses source SHA-256
  `8dd8b584db0c0ef919fdcc229645c2cb5d7697c7f555624b3c075e7f1a4eb53a`.
  Raw digest is
  `0e6880ec4902576c2a6de014424c3cba7eef99cdc5fd8fded8ceb6382a6df9cd`.
  Locked C and result digest are
  `c6e4535b725516550ca7a0ee4c69974799c2d2d10fed4e5f1ba6b71e43c5ba8a`.
  Production, compact, and incremental report `1/1/13/4`. Forest declines.
- The recovered recipe uses source SHA-256
  `8fefd1eb97742b1ef8349e9e51b5260c28295ed0d583d12eb5fbc04db579ce8a`.
  Raw digest is
  `896d9f79d941c3869dca7b855bae45738392d02519f0fbe3ac45cc2623fcfa2f`.
  Locked C digest is
  `3ae5ffba70cd0922976d24ed3e4d254cbb9d356639e8485b8b4b3abdc2667133`.
  Production, compact, and incremental report digest
  `814240a8aff9c3e253b37ce1ff535ff2cd96510c6afea40680a270405699967f`.
  They report `1/1/20/18`. Forest declines.
- `Add @salt{1%tsp}.` uses source SHA-256
  `6d60ac3d4e9155e84ead7b1f9e751728dcebafe03d6561d913f3f18f58a14297`.
  Raw digest is
  `f49ca1a85a0b2ee7ed7f07993d6bc8b103d66311f83d4a026e085cf2013a69ec`.
  Locked C and result digest are
  `dd3692a1a0e9145af9f2d082126a1e798d60cbe74942427746d3f0e83bd31e1c`.
  Production, compact, and incremental report `1/1/13/4`. Forest declines.

The recovered recipe still differs after 18 rewrites. The mismatch is
`/recipe`, shape, `children=5` versus `children=7`.

The malformed frontmatter witness has source SHA-256
`f2c2b1b02d9b3497d42c1cc53b94373974bd7b525af438d7dcf73391725f3615`.
Raw differs from locked C at `/recipe`, error, `true` versus `false`.
Production, compact, and incremental match locked C. They report
`1/1/13/11`. Forest declines.

The clean control and the no-op ingredient control match locked C on every
route. Forest accepts both controls. The clean control reports `1/1/11/1` on
production, forest, and incremental routes. The no-op control reports
`1/1/7/1` on those routes. Compact records no Cooklang pass for either control.

Compact falls back for seven witnesses. It accepts the clean and no-op
controls. A fallback keeps the routed counter unchanged and increments the
fallback counter by one. An accepted route increments the routed counter by one
and keeps the fallback counter unchanged. The fallback says that the generic
scheduler has no table action for the elected token. The guard checks each
transition exactly.

Incremental parsing reports `external_scanner_unsupported` for every witness.
It reuses zero subtrees and zero bytes. These scanner fallbacks are route
results, not retirement proof. Forest returns nil when it declines and returns a
non-nil tree when it accepts. The guard releases any unexpected tree before it
fails.

The locked C oracle uses runtime `0.25.1` at commit
`f5afe475deb7c0bae6407fb776c76824f717bb61`. The successful route run used C
artifact SHA-256
`009b897908abbc248d72855a7926b70715ac7955fed1f44c8fb2e8148f7ee83c`.

The successful focused Docker route receipt is under
`/tmp/gts-n31i-cooklang-rebased-artifacts/20260823T105932Z-final-rebased-routes`.
Its `container.log` SHA-256 is
`c83f451f157271c078abd286af1355a9687b37d864f39315d58e45079c9c3502`.

The successful document guard receipt is under
`/tmp/gts-n31i-cooklang-rebased-artifacts/20260823T105956Z-final-rebased-document`.
Its `container.log` SHA-256 is
`b14a8c8a5f18610c20657cff0868bd3832f888cb11c319c946c4585d5b5ae78e`.

Keep `dispatch.cooklang` live. Require a producer fix for the A0 and recovered
recipe differences. Require the authenticated Cooklang corpus and source lock.
Move scanner and recovery controls to their owning subsystems before retirement.
Keep dispatch.cooklang live until scheduler_action_semantics emits the locked-C
tree on every route. Require zero required dispatcher rewrites and exact
production, compact, forest, incremental, and locked-C parity. Do not change
the registry or production state before every condition passes.

## 2026-08-24 Go dispatcher blocker receipt

Status: `KEEP LIVE / NO-GO`. Keep `dispatch.go` live.
The evidence base is
`929609ccde78b0c9f4e57cf2225e0ae1204149cb`. The publication base is
`2c533f5c19f5f7ab9b586cd8454f0cdc4ece014b`.
This receipt changes only CHANGELOG.md, this document, and the focused test.
It changes no production or registry file.

The registry has 88 entries. It has 31 live dispatcher arms, 32 live entries,
and 56 retired entries. The Go arm calls
`normalizeGoReturnedTreeCompatibilityWithCensus` in `parser_result_go.go`.
It has three live subpasses:

- `dispatch.go.source-file-root` owns included-ranges root recovery.
- `dispatch.go.compat-walk` owns semicolon and sibling-boundary materialization.
- `dispatch.go.new-make-type` owns `new` and `make` type election.

The first subpass belongs to `scheduler_action_semantics`.
The compatibility walk belongs to `materialization`.
The type election belongs to `derivation_election_selection`.
The receipt keeps all three owners visible.

The grammar lock pins tree-sitter-go commit
`2346a3ab1bb3857b48b29d779a1ef9799a248cd7`.
The grammar lock SHA-256 is
`9ddb6324afd014f6ecdd1cae3dd1ba238f1e62ce03d126e6d8b267ce34d72ecb`.
The embedded Go grammar blob SHA-256 is
`9cf914d26d962d1a62e7954f8b20b302337a44cb7d4a07218eec482c45a57a08`.

The A0 (authenticated dispatcher census) manifest has 14 languages, 42 files,
and 14 receipts. It has no Go entry. The manifest names
`corpus_sources.lock`, but no committed lock is available for Go.
The tracked census has seven fixtures. Its Go fixture is
`cgo_harness/corpus_structural/go_sample.go`.
It has source SHA-256
`262c7b8359a364b56db8549f65f4f2e01ed43de096e607dd05f50b94bd45efb0`.
It records one checked file, one run file, 768 visited nodes, and zero
rewrites. The tracked aggregate has nine checked files, nine run files,
26,022 visited nodes, 2,107 rewrites, and zero error roots.
The mounted real corpus is ignored and is not authenticated evidence.

The C oracle uses contract `tree-sitter-c-v1`, runtime `0.25.1`, and runtime
commit `f5afe475deb7c0bae6407fb776c76824f717bb61`.
The C grammar artifact SHA-256 is
`f17d674be6e3e7a0edb521defa31d3174c877b01414fd758a4988673423573f7`.

The focused receipt covers six tracked or pinned witnesses. It covers raw,
production, compact, forest, incremental, and locked-C routes.
The included-ranges route adds the Go root-recovery witness.
The test pins every source SHA and C digest.
It pins checked, run, and rewritten counts for all three subpasses.
Visited counts remain diagnostic.

The standard route results are:

- The tracked witness has 2,194 bytes and C digest
  `833b5685c629a0224ed220aceb98679ee7a81197c7c9d0e3d0516dfed1b9cdf9`.
  Every standard route matches C. Each production pass reports `1/1/0`.
  Compact accepts. Forest returns a tree. Incremental reuse reports 25
  subtrees and 68 bytes.
- The clean semicolon and sibling witness has 110 bytes and C digest
  `e0570a37201e4cf73162dd11b4508aaa8c169706af8f5cd87462b5cd12e4207f`.
  Every standard route matches C. Each production pass reports `1/1/0`.
  Compact accepts. Forest returns a tree. Incremental reuse reports 23
  subtrees and 57 bytes.
- The malformed recovery witness has 43 bytes and C digest
  `9c04d41835d5bff3fcd1ca7db2f1279508819fcca287942813236091b0c3c088`.
  Every standard tree matches C and has an error root. Each production pass
  reports `1/1/0`. Compact falls back. Forest returns nil. Incremental reuse
  reports 14 subtrees and 42 bytes.
- The no-op control has 21 bytes and C digest
  `2b5a222aef26944bb46ea6bdc1bbc17da9af330252dc3317610965e7feb514e5`.
  Every standard route matches C. Each production pass reports `1/1/0`.
  Compact accepts. Forest returns a tree. Incremental reuse reports five
  subtrees and 13 bytes.
- The `new` and `make` witness has 79 bytes. Its raw digest is
  `322f6d6db609c49fb8b09a52d2d9e9ae0853c56d2349bc89a9af1df8c6ca4374`.
  Its production, compact, forest, incremental, and C digest is
  `329fac609c64a2e338b8e0532b63dd6a38e3f1b6da311dab68e0eb3576a664a6`.
  Raw first differs at
  `/source_file/function_declaration[1]/block[3]/statement_list[1]/assignment_statement[0]/expression_list[2]/call_expression[0]/argument_list[1]/selector_expression[1]`.
  Go has `selector_expression`. C has `qualified_type`.
  The three production passes report `1/1/0`, `1/1/0`, and `1/1/9`.
  Compact falls back. Forest returns a tree. Incremental reuse reports seven
  subtrees and 16 bytes.
- The recovery control has 66 bytes and C digest
  `3c74ff188c1085bb06dd82824347a8e4ff63760b5bfd51dff01663625f53927d`.
  Every standard route matches C. Each production pass reports `1/1/0`.
  Compact accepts. Forest returns a tree. Incremental reuse reports 16
  subtrees and 35 bytes.

The compact guard accepts only `routed+1/fallback` unchanged, or routed
unchanged and `fallback+1`. The malformed and `new`/`make` witnesses use the
fallback transition. The malformed reason is recovery entry.
The `new`/`make` reason is an uncertified scheduler frontier.

The included-ranges source has 276 bytes and SHA-256
`978465a10f7d814c2183eed2e4ceedfd1dd467efd780210ae39be8e170f3b01b`.
The C digest is
`9c8e5bb506bb345a577beb351f7b9230cca5e2e02cc4fd619e21f607657f290f`.
The Go digest is
`8231fb8782e5699708e183619b68e1d030ab09e60de99ac52e3c092ffc56d59c`.
Both roots are `source_file` with an error flag.
Go uses range `0..276` with 10 children.
C uses range `26..276` with seven children.
The first difference is the root range.
The Go arm records zero root rewrites, 25 compatibility rewrites, and zero
`new`/`make` rewrites on this route.

Every Go tree reports `NativeRecoveredStructureAuthoritative=false`.
Forest returns nil when it declines and a non-nil tree when it accepts.
The guard releases an unexpected tree before it fails.
Incremental routes require positive old-tree reuse and reject unsupported reuse.
The exact reuse values remain receipt evidence, not a brittle counter contract.

The route receipt is under
`/tmp/gts-n31j-go-rebased-artifacts/20260823T112605Z-final-rebased-ratchet`.
Its `container.log` SHA-256 is
`aeb53730c138395ef4618d7c1cfee805c720908dacc1cf8cd63248c72ae2d1d8`.
The document guard receipt is under
`/tmp/gts-n31j-go-rebased-artifacts/20260823T112634Z-final-rebased-document`.
Its `container.log` SHA-256 is
`15084a11ddaf6f0628919623a371ed1b39a3156ac817e4e7f008a2cf7ed2eaf2`.
The run used one Go grammar, one CPU, 4 GiB, 512 process IDs,
`GOMEMLIMIT=3GiB`, `GOMAXPROCS=1`, `GOFLAGS=-p=1`, `-parallel=1`, and a
20-minute timeout. It exited zero without timeout or out-of-memory failure.

No safe shared producer invariant was identified.
Keep `dispatch.go` live until a producer emits exact C output for every
authenticated witness and route. Require zero rewrites in all three subpasses.
Require an authenticated Go corpus and source lock.
Require included-ranges root, semicolon, sibling-boundary, and `new`/`make`
parity before retirement. Do not change production or registry state.

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
