# Compact route real-corpus matrix

Current evidence date: 2026-08-22.
Current base commit: `f298328a` from `main`.
Current candidate base commit: `f298328a`.

## C26a Swift issue #576 token-production blocker

Receipt base: `18d63b6f7802b28a0ddb889327fcd4ebebb99426`.

Status: **NO-GO / KEEP LIVE**. This receipt does not change compact route
admission, D6 status, or the bounded matrix counts. Keep issue #576 open.

The smallest existing witness is the 20-byte source `let x = unsafe bar()`.
Its source SHA-256 digest is
`b511d81ace2a89b05e8e5e0ca6730c10f2ac9295111dae013097c7c6be8861fe`.
The Go deep-tree digest is
`860b79483c37e217690deae43036bada15b259bed77713606124fa851702e62f`.
The locked C deep-tree digest is
`c64b894edc4a20e15f2b4127bad4223f698c8996dba091c06c34aa89386d3c68`.

The first divergence is
`/source_file/property_declaration[0]/call_expression[3]/ERROR[1]`, bytes
15..18. Go emits a childless `ERROR` node. Locked C emits an `ERROR` node with
the `bar` child. The two existing Swift corpus witnesses share this unsafe
expression-prefix cause.

The locked C reference parser uses grammar version `0.7.2`, grammar commit
`41d6e5fe811ec94229ee71771174a8cce558dfee`, runtime `0.25.1`, and runtime
commit `f5afe475deb7c0bae6407fb776c76824f717bb61`. Its lexer skips `bar`,
emits an error token of size four, detects the error, and resumes through the
previous recovery point. Go emits identifier symbol `160`, reaches state 47
without a parser action, and enters generic recovery. Go therefore does not
receive the C error token before it materializes the childless `ERROR` node.

The exact minimal-witness C logger trace is at
`/tmp/gts-c26a-correction-artifacts/20260823T024242Z-swift576-c-trace-exact`.
It records the skipped characters, the four-byte `ERROR` token, `detect_error`,
`resume`, and `recover_to_previous state:2543, depth:2`. The trace also records
the 20-byte C and Go trees. The Go state trace is at
`/tmp/gts-c26a-artifacts/20260823T022437Z-swift576-predicate-trace-min`.

Canopy attributes the generic C error-mode acquisition to
`cRecoverAcquireToken` in `parser_recover_c.go`. The
`cRecoverResumeLookahead` path serves custom source fallback. The
`pushLexErrorRunLeaf` path owns the C-shaped error wrapper. Those paths cannot
restore a token that the deterministic finite automaton (DFA) token source did
not produce. The divergence is therefore in token production and grammar-table
behavior, before generic error materialization. The direct structural queries
were:

- `scripts/canopy_query.sh search symbols parser_recover_c.go --limit 120 --no-cache`
- `canopy graph calls 'pushLexErrorRunLeaf' parser_reduce.go --no-cache --depth 3`
- `canopy graph calls 'cRecoverAcquireToken' parser_recover_c.go --no-cache --depth 3`
- `canopy graph calls --reverse 'cRecoverResumeLookahead' parser_recover_c.go --no-cache --depth 2`

The isolated experiment tried a grammar-agnostic predicate. When normal
lookahead had no parser action, it relexed from broad error mode before
recovery. The prototype touched only `parser.go` and
`parser_dfa_token_source.go`. The focused Swift run kept both deep-tree
digests and the first divergence unchanged. The prototype was removed. No
production or test change survives.

The focused Docker artifacts are:

- `/tmp/gts-c26a-artifacts/20260823T021728Z-swift576-minimal`
- `/tmp/gts-c26a-artifacts/20260823T021757Z-swift576-corpus`
- `/tmp/gts-c26a-artifacts/20260823T022322Z-swift576-predicate2`
- `/tmp/gts-c26a-correction-artifacts/20260823T024140Z-swift576-minimal-current`
- `/tmp/gts-c26a-correction-artifacts/20260823T024242Z-swift576-c-trace-exact`

An upstream regeneration also left the witness unchanged. It produced blob
digest `be5cd0bf8df7077804fe4b54ee47d76005c9a85c7c33b857ef6d2aff34461286`.
The shipped Go Swift blob is
`be4575bc0acc3c60324aab635d067f940ac5f0557b80a8e3565d1e7d02d53582`.
The upstream probe used revision
`172ada1cc4117d0260d9340680b4134adba2bc2c`, package version `0.7.3`, and
these artifacts:

- `/tmp/gts-swift-upstream-probe-artifacts/20260823T000638Z-swift-upstream-issue576-range`
- `/tmp/gts-swift-upstream-probe-artifacts/20260823T000433Z-swift-upstream-issue576-parity`

Reopen implementation work after a grammar or lexer revision changes token
production for the `unsafe` expression prefix. Reopen it after a generic
runtime change produces the locked C error token without a Swift-specific
rule. Then run the 20-byte witness and one corpus witness in separate Docker
parity runs. Keep issue #576 open until both witnesses pass.

## C26b Swift issue #576 conformance-list recovery blocker

Receipt base: `838aba943038248529429a572c4d6d98359bd87e`.

Status: **NO-GO / KEEP LIVE**. No production or test change survives. Keep
issue #576 open.

The 66-byte witness is:

```swift
protocol P {
  associatedtype Stride: SignedNumeric, Comparable
}
```

The source SHA-256 digest is
`9cbe9baf046f1ec48eef0c3fb6e4ceadf2e38fa24e6fc23a3c9bf77d2b7b03a9`.
The Go deep-tree digest is
`8146289d41f1937be85d27f1c0b03ce84eb2d5e7f27b0ba7300bba11c19f274e`.
The locked C deep-tree digest is
`596a0910e6111ff5db146630797aea26722d830aa6ea06f4933d0b7062dd4bed`.

Both roots span `0..66` and report an error. The first divergent node is
`/source_file/protocol_declaration[0]/protocol_body[2]/ERROR[2]`.
Go emits `ERROR[51:64]` with one child, the comma at `51..52`.
Locked C emits `ERROR[51:63]` with two children:

- The comma at `51..52`.
- A nested `ERROR` node named `Comparable` at `53..63`.

The Go recovery trace identifies the first event. At byte `51`, the comma has
no parser action in state `2811`. Three recovery stacks pause. Recovery then
resumes at state `1584`. Condense chooses the ordinary lineage over the error
group at byte `63`.

At byte `64`, Go receives a zero-width `_implicit_semi` token.
`cRecoverDispatchInError` passes this token to `cAbsorbTokenIntoError`.
This extends the open Go error to byte `64`. Locked C ends the error at byte
`63` and keeps `Comparable` as a nested error.

Direct no-cache Canopy queries trace token acquisition through
`cRecoverAcquireToken`. They trace recovery through `cRecoverResumeLookahead`,
`cRecoverToState`, and `cCondenseAndResume`. They trace materialization through
`cAppendVisibleSplice` and `cSetNodeSpan`. They trace zero-width absorption
through `cRecoverDispatchInError` and `cAbsorbTokenIntoError`.
No Swift-specific parser-result helper participates in this path.

The corpus witness is `grammars/testdata/swift_corpus/stdlib_Stride.swift`.
It contains 26,410 bytes. Its source SHA-256 digest is
`4890e76decb629479ada497343e93f6c979d8f438c72b9ef0769b7b61e5e6fe1`.
The Go deep-tree digest is
`42d6eb25ba0da35d3e45e4b41cbb5d1f86f06beee67ba1f0f5c12f477c510073`.
The locked C deep-tree digest is
`992c548f12780cae0ca9eb7f456a32dd27b9f625e4748a898e22cf2cc3075a7b`.
Go emits one whole-file `ERROR` node at `0..26410`. Locked C emits the local
target error at `3991..4003` with two children. The first full-tree difference
is the existing root shape divergence. The known-failing corpus ratchet passes.

Existing generic recovery tests require zero-width skipped tokens to advance
and to update error spans. A generic change that drops this behavior could
change other grammars. A condense-ranking change also lacks cross-language
proof. Do not add a Swift rule, source-hash exception, blob exception, or
witness repair. The audit found no safe grammar-agnostic correction.

The focused Docker artifacts are:

- Minimal probe: `/tmp/gts-c26b-artifacts/20260823T031009Z-swift576-associatedtype-probe-base`.
- Corpus probe: `/tmp/gts-c26b-artifacts/20260823T031024Z-swift576-associatedtype-probe-corpus-base`.
- Corpus ratchet: `/tmp/gts-c26b-artifacts/20260823T031211Z-swift576-associatedtype-corpus-ratchet-base`.
- Go recovery trace: `/tmp/gts-c26b-artifacts/20260823T031320Z-swift576-associatedtype-go-trace-base`.
- Field projection guard: `/tmp/gts-c26b-artifacts/20260823T030838Z-swift576-associatedtype-minimal-base`.

Each run used one Swift workload, one CPU, and `GOMAXPROCS=1`. Every run
passed without an out-of-memory failure or timeout.

Reopen implementation work after an upstream grammar or table change, or
after a generic recovery change gains cross-language proof. Require the
minimal witness, the corpus ratchet, locked-C parity, field projection, and
generic zero-width tests to pass. Keep issue #576 open until both witnesses
match locked C.

## C26c Swift issue #576 masking-shift recovery blocker

Receipt base: `77ec4288a115e4ddb1969d2945e8507dad92f1af`.

This masking-shift item originated as closed issue #574.
Issue #576 is the open umbrella for this Swift recovery family.

Status: **NO-GO / KEEP LIVE**. No production or test change survives. Keep
issue #576 open.

The canonical witness includes its trailing newline:

```swift
let x = 1&<<7
```

It has 14 bytes. Its secure hash algorithm 256 (SHA-256) source digest is
`25d8869d28d13d391a46d2afcab12662c4a9c62b3c9555be966767caf27bc720`.
The Go tree digest is
`0a080f094102d27305084a234d22396a1c4b64cad5be9ab55a9969249f2a67aa`.
The locked C tree digest is
`14b99aace77ea88a972e0d1bbefcdef9f226bb764aeba12bb41c2cf1509610e9`.

Both roots span `0:14` and report an error. Go has one root child. Its
`property_declaration` spans `0:13`, and its `infix_expression` spans `8:13`.
That expression contains `integer_literal[8:9]`, `_custom_operator[9:11]`,
`ERROR[11:12]` with `<`, and `integer_literal[12:13]`.

Locked C has two root children. Its `property_declaration` spans `0:12`, and
its `infix_expression` spans `8:12`. The second root child is
`ERROR[12:13]`, with `integer_literal[12:13]` as its child. The first
divergence is `/source_file`, where Go has `children=1` and C has
`children=2`.

The 13-byte control has no trailing newline. Its source digest is
`82b80d62359b54747565d50be90f55f6177299dde3c43237b1efddc417339809`.
Its Go tree digest is
`1b4e24b4b42e2f38df92cf0a7d9ffd2345373dad5b3766f120a61b3419284740`.
Its locked C tree digest is
`b226b4ce49ab883efb4614f0cb05ad92a34ba39262beb80f8bd502c38130ffff`.
Both roots span `0:13` and have two children. Go reports no error. Locked C
reports an error. This control is not the tracked root-child absorption case.

The Go and C traces share these symbols through the final integer:

- `let[0:3]`
- `identifier x[4:5]`
- `=[6:7]`
- `integer_literal[8:9]`
- `_custom_operator[9:11]`
- `<[11:12]`
- `integer_literal[12:13]`

The implicit-semicolon spans differ:

- Go logs `_implicit_semi[14:14]`.
- C logs a one-byte `_implicit_semi` at byte `13`.

Both traces reach end of input at byte `14`. Go labels this token `end`.
C also labels this token `end`.

The first Go recovery event occurs at byte `12`. The integer literal has no
parser action in state `130`. Go calls `cRecoverToState` at state `291` with
depth `2`. Recovery creates ten stacks. Condense rejects the missing-group
lineage because the ordinary lineage has lower cost. The final Go tree keeps
`7` inside the infix expression.

The first locked-C recovery event occurs in state `1459` at byte `12`. C
lexes an integer literal of size one, detects an error, and resumes version
zero. It recovers with missing `custom_operator_token1` in state `432`, then
recovers to previous state `623` at depth `2`. It skips the integer literal.
The final locked-C tree keeps `7` in a sibling `ERROR` node.

Canopy assigns the ownership path to generic recovery and materialization:

- `parser.go:4526` `parseInternal` reaches `parser_recover_c.go:776`
  `cRecoverAcquireToken`.
- `parser_recover_c.go:4110` `cRecoverToState` uses
  `cAppendVisibleSplice:4064` and `cSetNodeSpan:4099`.
- `parser_recover_c.go:4462` `cCondenseAndResume` uses the recovery version
  comparison and lookahead paths.
- `parser.go:3876` `materializeTransientChildrenForReturnedTree` finalizes
  transient nodes.

The token sequence does not differ first. A Swift-specific lexer rule that
joins `&<<` is rejected. A generic condense change that prefers an error group
is rejected because existing tests require the lower-cost clean stack. A
generic operator or materialization heuristic lacks cross-language proof. A
source-hash exception, blob exception, or witness repair is also rejected.

The corpus witness is `grammars/testdata/swift_corpus/stdlib_ASCII.swift`.
It has 3,115 bytes. Its source digest is
`0db11184c2f5e94ae43dd5349cc37ae55fc9acca4dd2eb82b0311daefca72540`.
Its Go tree digest is
`e4cc31fc7b2c14ad9c466cabc4614cb002ac3fad8cb671eaba22a4d1661b1572`.
Its locked C tree digest is
`4721985dfa027f12728eb183afd782e77bb5a44fff22a0e4aa002e2495f648f3`.
Both roots span `0:3115`, have 14 children, and report an error. The target
`1&<<7` occurrence starts at byte `1513`. The first full-corpus difference is
the existing `user_type` versus `simple_identifier` difference at
`/source_file/class_declaration[12]/class_body[4]/function_declaration[5]/function_body[8]/statements[1]/control_transfer_statement[0]/call_expression[1]/navigation_expression[0]/user_type[0]`.
The known-failing `stdlib_ASCII.swift` corpus ratchet passes.

The focused Docker artifacts are:

- `/tmp/gts-c26c-artifacts/20260823T033300Z-swift576-masking-shift-probe-minimal-canonical`
- `/tmp/gts-c26c-artifacts/20260823T033224Z-swift576-masking-shift-probe-minimal-exact`
- `/tmp/gts-c26c-artifacts/20260823T033433Z-swift576-masking-shift-tree-dump`
- `/tmp/gts-c26c-artifacts/20260823T033312Z-swift576-masking-shift-probe-corpus`
- `/tmp/gts-c26c-artifacts/20260823T033324Z-swift576-masking-shift-corpus-ratchet`
- `/tmp/gts-c26c-artifacts/20260823T033347Z-swift576-masking-shift-go-trace-exact`
- `/tmp/gts-c26c-artifacts/20260823T033508Z-swift576-masking-shift-c-logger`

The missing-fixture trace failed and is excluded:

- `/tmp/gts-c26c-artifacts/20260823T033336Z-swift576-masking-shift-go-trace`

The Canopy audit artifacts are:

- `/tmp/c26c-canopy-corrected-parse.json`
- `/tmp/c26c-canopy-corrected-acquire.json`
- `/tmp/c26c-canopy-corrected-recover.json`
- `/tmp/c26c-canopy-corrected-condense.json`
- `/tmp/c26c-canopy-corrected-materialize.json`

Reopen implementation work only after a generic recovery or materialization
change produces the locked-C sibling `ERROR` without a Swift-specific rule.
Require the 14-byte witness, the 13-byte control, the corpus ratchet, focused
generic recovery tests, and locked-C parity to pass. Keep issue #576 open.

## C26d Swift issue #576 optional-binding recovery blocker

Receipt base: `11d9aec70eaef0c0d65c3cd14b8f594d64869c7b`.

Evidence was collected on `731f8a9d9440a006b2cc6b56ef5b31c0ff3b5ce7`.
The probe artifacts use `issue-590` labels. Issue #576 remains the open
umbrella for this Swift recovery family.

Status: **NO-GO / KEEP ISSUE #576 OPEN**. No production or test change
survives.

The focused probe uses two witnesses. The first witness has the label
`issue-590-chunked-minimal`. It has 118 bytes. Its source SHA-256 digest is
`66a0ffea75dcba15e0fe65f8c1e14221421a888e3f80b3809ba195ce88a5c0ef`.
Locked C reports a clean `source_file` root over `0..118`. Its tree digest is
`0cdb8862939d6d21cbb9b6792939381144e980ee5e45ee9dcab97a3e0930e9e7`.

The Go raw, production, compact, incremental-production, and
incremental-compact routes all report `has_error=true`. Each route reports a
`source_file` root over `0..118` with digest
`3a90ba023387b89a692aa8e9c755ee9d54008437486b26e90a77552ffec03c3b`.
The first error is `/source_file/ERROR[0]` over `0..118`. The nested error is
`/source_file/ERROR[0]/statements[8]/if_statement[0]/ERROR[5]` over `98..114`.

The Go forest route reports a clean root over `0..118` with digest
`e44be93f73b5bf5b669aba5dbc9d65e7ac065798e4a33471a70ac91df16c286e`.
It still differs from locked C at
`/source_file/class_declaration[0]/class_body[2]/function_declaration[1]/function_body[4]/statements[1]/if_statement[0]/statements[6]/if_statement[0]/call_expression[1]/call_suffix[1]/value_arguments[0]/value_argument[3]/navigation_expression[0]/user_type[0]`.
Go emits `user_type`. Locked C emits `simple_identifier`.

The second witness has the label `issue-590-chunked-corpus`. It is
`grammars/testdata/swift_corpus/swift-algorithms_Chunked.swift`. It has 27,814
bytes. Its source SHA-256 digest is
`78edea4a5ca6c8bf26b1cdf9c4f40b3f38cca0782cd45ec5ccddd14c02f7d38c`.
Locked C reports a clean `source_file` root over `0..27814`. Its tree digest
is `48b7dc83a6cfb41c5a9a631abf91b71b24e5295fb4dc35c9052ede79d984bbf8`.

The Go raw, production, compact, incremental-production, and
incremental-compact routes all report `has_error=true`. Each route reports a
`source_file` root over `0..27814` with digest
`27f732bd09e3676f54b637f1dfd2fbb87762f2b447e3a87d352e513df47c9bfc`.
The corpus forest route declines with `forest route declined`. It produces no
forest digest. The Go corpus result therefore does not establish C parity.

The runtime records no concrete recovery action for either witness. It
reports `action=false` with zero state, byte, token, type, and result fields.
It also reports `normalize=0/0`, `rewritten=0`, and `retry=0`.

Fresh production and compact routes report one `swift_legacy` entry for each
witness. Raw routes report zero. The minimal forest route reports zero. The
incremental logs omit this field. Do not infer a complete legacy-pass total.

Both incremental-production and incremental-compact routes report
`external_scanner_unsupported`. They report zero reused subtrees, zero reused
bytes, and `OldTreeReuseRoute=false`. They do not establish incremental reuse.

The focused condition-family controls passed fresh production, incremental
production, fresh compact, and incremental compact routes. The controls are
`issue-590-condition-list` with digest
`f9b26a3b9b7a881e53dbe70a512733c98b6c77f86ab7550a04924b370035df63` and
`issue-591-else-if-binding` with digest
`018cecb4cb76ff3c7bbecb40546c1832fdfcaa644debd1c489c634a762b91c84`.
These controls do not establish parity for the optional-binding witness.

The Swift detector boundaries are narrow:

- `swiftIfStatementSwallowedThenBlockKeywordEnd` requires a first child of
  type `if`. It accepts only an `ERROR` child whose first child is `else`.
- `swiftFindElseIfKeywordEnd` skips trivia. It accepts only word-boundaried
  `else` followed by `if`.
- `swiftFindOptionalBindingRHSEnd` stops at the first depth-zero comma. It
  ignores commas inside parentheses or brackets, comments, and strings.
- `swiftFindConditionBodyBrace` finds the first depth-zero `{`. It skips
  comments and strings and rejects a depth-zero `}` or `;`.

Direct no-cache Canopy queries traced this ownership path:

- `normalizeResultCompatibility` in `parser_result_compat.go:28` calls
  `applyResultCompatibility` at line 41.
- `applyResultCompatibility` calls `runLanguageResultCompatibility` at line
  103.
- `runLanguageResultCompatibility` calls `dispatcherArmSubpassCensus` at line
  408 for `dispatch.swift`.
- The Swift arm calls `normalizeSwiftCompatibilityWithCensus` at
  `parser_result_swift.go:22`.
- That path calls `normalizeSwiftRecoveredTrailingClosureConditions` at
  `parser_result_swift_conditions.go:40`.
- The condition helper calls `swiftCollectConditionParenInserts` at line 114,
  then `parseSwiftCleanFullSourceRecovery` at `parser_result_swift.go:62`.
- Recovery calls `parseForRecoveryWithMode` at `parser_api.go:568`, then
  `Parse` at line 1064.

No safe generic parser-core correction remains. The generic swallowed-error
hook returns unchanged when the returned root has an error. The compact error
region hook fails closed for deeper resume, missing insertion, or end of file.
Neither hook can synthesize the missing Swift structure.

The successful Docker runs used one Swift workload at a time, one CPU,
`GOMAXPROCS=1`, `-p=1`, and test parallelism one. They completed without an
out-of-memory failure or timeout:

- Probe: `/tmp/gts-c26d-artifacts/20260823T043130Z-swift-590-probe-v2`.
- Corpus ratchet: `/tmp/gts-c26d-artifacts/20260823T043210Z-swift-chunked-ratchet`.
- Condition controls: `/tmp/gts-c26d-artifacts/20260823T042421Z-swift-condition-family`.

The corpus manifest test and `TestSwiftCorpusProbeMatchesLegacy` passed. The
legacy comparison proves Go probe and legacy equality only. It does not prove
Go and locked-C parity.

Reopen implementation work after a grammar, table, or generic parser change.
Require Go and locked C equality on all six minimal routes. Run raw,
production, compact, forest, incremental-production, and incremental-compact
routes. Then run the chunked corpus controls on every available route. Require
the corpus forest route to produce a certified comparison instead of
declining. Require the condition-family controls to pass. Keep issue #576 open
until these controls pass.

## C26e Swift issue #576 CollectionAlgorithms recovery blocker

Receipt base: `2b755f744aef8dd253a4415ca4a5816fa85b0dbb`.

Evidence base: `14f6692fac65eab817f65af8cc6072e423ca6563`.

Status: **NO-GO / KEEP ISSUE #576 OPEN**. No production or test change
survives.

The locked C identity is:

- Grammar commit `41d6e5fe811ec94229ee71771174a8cce558dfee`.
- Runtime `0.25.1`, commit `f5afe475deb7c0bae6407fb776c76824f717bb61`.
- Grammar artifact SHA-256
  `2a9f14046d4ca88b6db1316ee5f48b876aea1700e3c09811b3c87257fe827c5c`.

The 16,871-byte witness is the exact prefix of
`grammars/testdata/swift_corpus/stdlib_CollectionAlgorithms.swift`.
Its source SHA-256 digest is
`39591b90ee9164a0bc594f2206945946ea27c0ee24149a23ed9755f9466c703d`.
The tracked source has 24,056 bytes. Its source SHA-256 digest is
`1aae0051b0bfb50e17c7ac94961ee7cab7332367dcc16e827d2482be7a2dc5a`.
The prefix is therefore a byte-exact part of the tracked full source.

The prefix locked-C digest is
`030878796edce87bdae03b7fb51be6e92c52e81482a9a94edc822248c3aad9d1`.
The Go digest on raw, production, compact, and incremental routes is
`cd95cd60eb4cac2ad3d1dd7652eec9b5c188c98338e9d4fe22c9223dacedba03`.
Both roots report an error over `0..16871`. Locked C has 37 root children.
Go has 39 root children.

The prefix first-error records are:

- Go: `/source_file/class_declaration[35]/class_body[2]/function_declaration[48]/function_body[9]/ERROR[2]@13078..13223/children=2`.
- Locked C: `/source_file/class_declaration[36]/class_body[3]/function_declaration[43]/function_body[9]/statements[1]/property_declaration[0]/try_expression[3]/call_expression[1]/ERROR[1]@15495..15534/children=5`.

Go reports eight errors. Locked C reports seven errors. The first root
difference is the child count. Do not apply these prefix error paths to the
full corpus.

The full-corpus locked-C digest is
`132d332f511f12735d80e846f52ec1fddf5f3d0dcd7a097779640a7710497487`.
The Go digest on raw, production, compact, and incremental routes is
`23035510c7f709e0cf029509c1d54aef62fefe27535e12e10a7bd874c0479fe2`.
Both roots report an error over `0..24056`. Locked C has 50 root children.
Go has 39 root children. The full-corpus record reports only this root shape
difference. It does not claim a full-corpus first-error path.

All four Go routes retain their Go digest for both witnesses. Each route
reports `stop=accepted`, `c_recovery=true`, `dropped=false`, `retries=0`,
`retry_reason='\x00'`, `normalization=0/0`, `rewritten=0`, and
`authority=false`. The prefix profile reports `stacks=15`, `depth=120`,
`nodes=10511`, and `error_ceiling=0/0`, `error_peak=9/11`. The full-corpus
profile reports `stacks=16`, `depth=144`, `nodes=16483`, and the same error
ceiling and peak.

The compact route falls back for both witnesses. Its counters move from
`before=0/0` to `after=0/1`. The reason is:

```text
compact route declined at recovery [mechanism=recovery-entered]: did not accept EOF: generic scheduler has no table action for the elected token
```

The forest route declines for both witnesses:
`accepted=false reason="dead_end" states=[10 524 46 646]`.

The incremental route reports
`reuse=false unsupported=true reason="external_scanner_unsupported"` and
`old_reuse=false`. It reports zero reused subtrees, zero reused bytes, zero
accepted retries, no adopted retry, and zero recovery searches and hits.
The prefix reports `new_nodes=9105` and `max_stacks=15`. The full corpus
reports `new_nodes=14490` and `max_stacks=16`.

Canopy v0.18.0-19-g01a5f95 attributes the generic path to
`cHandleError` in `parser_recover_c.go` and
`nextGLRUnionDFAToken` in `parser_dfa_token_source.go`. The recovery graph is
at `/tmp/gts-c26e-correct-canopy-recovery.json`. The token graph is at
`/tmp/gts-c26e-correct-canopy-token.json`. Direct no-cache queries confirmed
the generic recovery and token-preference paths. No safe grammar-agnostic
correction is proven. The Swift normalization path did not run a repair:
`normalization=0/0`, `rewritten=0`.

The successful Docker artifacts are:

- `/tmp/gts-c26e-correct/harness_out/docker/20260823T053216Z-c26e-correct-swift576-collection-final2`
- `/tmp/gts-c26e-correct/harness_out/docker/20260823T052929Z-c26e-correct-swift576-collection-full`

Each run used one Swift workload, one CPU, 4 GiB of container memory,
`GOMAXPROCS=1`, `-p=1`, test parallelism one, and a 3 GiB Go memory limit.
Each run used a 20-minute test timeout. Both focused Docker tests passed with
exit code zero. Neither run timed out or hit an out-of-memory failure.

The authenticated corpus is unavailable. The repository has no
`cgo_harness/corpus_real` directory and no
`cgo_harness/perf_scan/corpus_sources.lock` file. The sidecar
`cgo_harness/perf_scan/corpus_sources.lock.sha256` does not supply that lock.
The tracked full source is a control, not an authenticated release corpus.

The shared 20-byte unsafe control is `let x = unsafe bar()`.
Its source SHA-256 digest is
`b511d81ace2a89b05e8e5e0ca6730c10f2ac9295111dae013097c7c6be8861fe`.
Its Go digest is
`860b79483c37e217690deae43036bada15b259bed77713606124fa851702e62f`.
Its locked-C digest is
`c64b894edc4a20e15f2b4127bad4223f698c8996dba091c06c34aa89386d3c68`.
The first divergence is
`/source_file/property_declaration[0]/call_expression[3]/ERROR[1]`, bytes
`15..18`. Go emits a childless error. Locked C includes the `bar` child.
This control shares the unsafe-expression-prefix family. It does not prove
parity for either CollectionAlgorithms witness.

The 645-byte construction at
`/tmp/gts-c26e-correct/harness_out/docker/20260823T052725Z-c26e-correct-swift576-collection`
is excluded. It used the wrong source and is not a valid C26e witness.

Reopen implementation work only after a grammar, lexer, or generic
token-recovery change supplies a safe proof. Require locked-C equality for the
20-byte control, the 16,871-byte prefix, and the 24,056-byte full source on
raw, production, compact, forest, and incremental routes. Require compact
and forest routes to produce certified comparisons. Require the authenticated
corpus and its source lock before release retirement. Keep issue #576 open
until these conditions pass.

## Current bounded result

The bounded matrix completed with no silent divergence.

| Status | Files |
|---|---:|
| PASS | 70 |
| FALLBACK | 30 |
| SKIP | 10 |
| DIVERGE | 0 |
| ERROR | 0 |
| Total | 110 |

The corpus manifest contains 147 verified files across 50 languages.
This run selected files smaller than 16,384 bytes and excluded AWK.
The AWK medium file needs a separate slow-path budget.

The direct route served 64 percent of the selected files.
Production served every fallback and every ineligible file.

## D6a producer evidence

D6a publishes an authenticated drop-cohort frontier with the feature disabled
by default. It does not enable route admission, frontier history, or frontier
verification. The bounded matrix counts above remain unchanged because D6a is
producer-only.

The isolated Docker targets accepted these published and member counts:

| Grammar | Target | Published/member | Route | Natural fallback |
|---|---|---:|---:|---|
| Go | `query_compile` | 2/6 | 0/1 | Unchanged |
| Go | `rewrite` | 2/6 | 0/1 | Unchanged |
| Go | `language` | 2/2 | 0/1 | Unchanged |
| Go | `grammargen_lr` | 2/2 | 0/1 | Unchanged |
| Erlang | `macro_function_clauses` | 8/26 | 0/1 | Unchanged |
| Erlang | `macro_expanded_top_level_function` | 3/6 | 0/1 | Unchanged |
| Haskell | `smoke` | 2/2 | 0/1 | Unchanged |
| JavaScript | `functions` | 2/8 | 0/1 | Unchanged |
| Bash | `converged_split` | 4/24 | 0/1 | Unchanged |

The gate fixed complete frontier byte accounting and stale-token transaction
poisoning.
It ran 10 internal tests. The root and no-core gates passed.
The final Docker artifacts span `20260822T065000Z` through
`20260822T065433Z`.

### Interleaved performance receipt

The accepted receipt uses 20 interleaved seeds and the primary benchmark trio.

| Benchmark | Candidate versus base | Probability |
|---|---:|---:|
| Full parse | -3.01% | `p=0.678` |
| Incremental single-byte edit | -1.09% | `p=0.758` |
| Incremental no-edit | +7.34% | `p=0.910` |
| Geomean | +0.97% | — |

Full-parse bytes per operation changed by -3.10 percent.
Allocation counts remained unchanged.
Discard the earlier sequential full-set comparison because unrelated host
tests overlapped the candidate run.

This receipt does not graduate the route and does not verify D6b.

## D6b internal-consumer evidence

D6b adds a default-off authenticated internal consumer for complete frontiers.
It does not activate the route driver, admission, or production drop wiring.

The driver verifier collects exact current heads and references. It requires
one common nonzero frontier sequence, rebuilds the frontier token, and consumes
the authenticated frontier immediately before each of the three drop sites.
Zero or mixed sequences fail closed. The verifier remains default-off, and the
route remains ungraduated.

The consumer enforces these contracts:

- Authenticate the owner, epoch, election, token, frontier seal, and ordered records.
- Reject producer-cap violations, blended references, malformed offsets, and mismatched reference sets.
- Require one survivor member to match every dropped participant within one common cohort.
- Match the exact survivor reference and compare action and full derivation identity, metadata, and bytes.
- Journal the consumed state only after proof succeeds, then restore it through checkpoint rollback.

The focused Docker gate results are:

| Target | Result | Artifact |
|---|---|---|
| `TestG18D6b` | PASS, exit 0 | `harness_out/docker/20260822T101332Z-d6b-proof-tests-v4` |
| `TestG18DropCohortFrontier` plus `TestG18D6b` | PASS, exit 0 | `harness_out/docker/20260822T101347Z-d6b-frontier-regression-v4` |
| `TestG18D6bDriver` | PASS, exit 0, no out-of-memory (OOM), no timeout | `harness_out/docker/20260822T123532Z-d6b-driver-refresh-v19` |
| `TestG18D6aProducerTelemetry/go/language` | PASS, exit 0, no OOM, no timeout | `harness_out/docker/20260822T123627Z-d6a-go-language-refresh-v21` |
| `TestG18D6aProducerTelemetry/erlang/macro_expanded_top_level_function` | PASS, exit 0, no OOM, no timeout | `harness_out/docker/20260822T123634Z-d6a-erlang-refresh-v22` |
| Default no-tag compile | PASS, exit 0, no OOM, no timeout | `harness_out/docker/20260822T123642Z-d6b-no-tag-refresh-v23` |
| Combined D6a/D6b gate | FAIL, exit 1, no OOM, no timeout | `harness_out/docker/20260822T123547Z-d6b-d6a-combined-refresh-v20` |
| Combined D6a/D6b gate | FAIL, exit 1, no OOM, no timeout | `harness_out/docker/20260822T123655Z-d6b-d6a-combined-refresh-v24` |

Both combined gates failed only in the documented process-global pool telemetry comparison.
The affected subtests changed between runs. The isolated targets passed. No OOM or timeout occurred.

### Safe D6b decline

D6a authenticates each frontier member independently. D6b also requires one common action and exact derivation.

An authenticated frontier can place every dropped participant in one cohort with the survivor candidate.
If it has no common proof, the consumer returns a typed decline.
The scheduler then runs the existing alternative-set proof.

A mixed-cohort frontier is not a safe decline. The consumer returns a fatal error and poisons the transaction.

Keep every malformed, stale, foreign, resealed, blended, different-cohort, or otherwise unauthenticated frontier as a fatal error.
Do not weaken exact derivation comparison.

The `grammargen_lr` witness demonstrates this boundary:

- D6a publishes two complete frontiers.
- The target frontier has two members with equal actions and different derivation digests.
- D6b declines the frontier without changing its state or journal.
- The existing alternative-set proof declines, so production fallback serves the tree.
- Candidate and production trees have equal locked-C deep digests.

The focused Docker receipts are:

- `harness_out/docker/20260822T184936Z-d6b-core-typed-decline-refresh-review7`
- `harness_out/docker/20260822T184953Z-d6b-grammargen-lr-safe-decline-refresh-review8`
- `harness_out/docker/20260822T185008Z-d6b-driver-positive-fatal-refresh-review9`
- `harness_out/docker/20260822T185021Z-d6b-layout-budgets-refresh-review10`

These receipts do not graduate direct D6b admission.

The matrix counts and performance counts remain unchanged.
This evidence does not graduate the route or wire production drops.

### D6c blocker receipt: `grammargen_lr`

The receipt branch uses base commit `ed0568d9`. The focused Docker reproduction
ran at original probe commit `0c34a681` and records the authenticated frontier
used by the D6b decline. The frontier drops participant `1`.

| Member | Derivation digest | Length | Continuation state | Head byte |
|---|---|---:|---:|---:|
| Survivor | `9b1c3a249bec15d4b74a7462f701c491e022be80f7a51a5590f1520a76fd2c06` | 5,254 | 1,141 | 1,046 |
| Dropped | `d72a6fe90ca3aec9883bd00494eb8ca7110ede90d5f09fb5000fdc6441a79e8f` | 5,324 | 680 | 1,046 |

Both members use the same authenticated scanner checkpoint at source span
`1046..1047`. The first continuation-state difference is `1141` versus `680`.

The first public projection difference is path `[0,4]`, span `1030..1037`,
source text `prodIdx`:

- The survivor materializes terminal `identifier` symbol `86`, production `0`.
- The dropped member materializes `parameter_declaration` symbol `113`,
  production `36`, with one `_simple_type` child under field `type`.

The focused regression test records a typed `no_action` D6b decline. The
existing `grammargen_lr` fallback gate also passes. Candidate and production
trees retain locked-C deep digest
`1472cfd9a014d4034dbc1456afd12c282630ef787c3543cf0cecb73619883ad2`.

The labeled Docker artifacts are:

- `/tmp/gts-d6c-nogo-receipt-20260822/harness_out/docker/20260822T202359Z-d6c-frontier-state-public-shape`
- `/tmp/gts-d6c-nogo-receipt-20260822/harness_out/docker/20260822T202427Z-d6c-grammargen-lr-fallback`

The D6c decision is NO-GO. A drop may proceed only when the authenticated
continuation state and the canonical public projection match. Any state or
public-shape mismatch must decline and preserve the production fallback.

## Current fallback taxonomy

The 30 fallbacks divide into 15 clean production trees and 15 production error trees.

| Class | Clean | Error tree | Total | Exact trigger |
|---|---:|---:|---:|---|
| Recovery handoff | 0 | 13 | 13 | The elected token has no table action at end-of-file. |
| Selected-lineage ownership | 11 | 1 | 12 | A converged split drop lacks one selected-lineage proof. |
| Certified repetition conflicts | 3 | 0 | 3 | The generic scheduler declines a repetition shift. |
| Acceptance-frontier ownership | 1 | 1 | 2 | The end-of-file frontier has more than one active head. |
| Total | 15 | 15 | 30 | |

The recovery handoff class contains these witnesses:

- Dart
- Go module
- INI, two files
- Make
- Objective-C
- PHP, two files
- PowerShell
- SQL, two files
- Swift
- TypeScript

The selected-lineage class contains these witnesses:

- C#
- D
- Dart
- Elixir
- Julia
- Kotlin, two files
- Perl, two files
- Scala, two files
- TSX

The repetition class contains one C# file and two Haskell files.
The acceptance class contains one Bash file and one Markdown file.

The scheduler now scopes each condense action to live headers.
It no longer counts removed versions against the shared link cap.
This retires the prior Go module, OCaml, and Rust shared-cap witnesses.
The live scope retains discarded boundary history as split provenance.
Both Perl witnesses remain on the production route without an exact lineage proof.

## Class prerequisites

The recovery handoff must preserve production error-tree ownership.
Do not convert a failed compact acceptance into a clean result.

Selected-lineage ownership must identify the exact surviving reduction path.
An artifact certificate can authorize only a pinned grammar with C-oracle evidence.

Certified repetition conflicts need one reusable conflict rule or exact artifact evidence.
Do not add a grammar-name branch to the scheduler.

Post-accept continuation must preserve an accepted result while live end-of-file reductions finish.
The Markdown continuation reaches score 160 and branch 8.
It materializes the exact production digest `a411b8648c76`.
The path still needs one selected-lineage proof for an intervening converged split drop.

The Bash frontier has one accept head and one table-dead head.
Production returns an error tree for that file.
Keep this file on the production route.

## Historical bounded result at `382080a3`

Commit `963fae08` recorded this receipt on 2026-07-29.
The run used `382080a3` as its base.
It is not the current matrix result.
It enabled ratchet mode, excluded AWK, and set the maximum file size to 16,383 bytes.

| Status | Files |
|---|---:|
| PASS | 67 |
| FALLBACK | 33 |
| SKIP | 10 |
| DIVERGE | 0 |
| ERROR | 0 |
| Total | 110 |

That corpus manifest contained 147 verified files across 50 languages.
The run selected files smaller than 16,384 bytes and excluded AWK.
Its manifest included one additional Elm highlight file.
The current manifest includes that generated file.

That receipt also recorded this 206-language smoke scorecard:

| Status | Languages |
|---|---:|
| PASS | 200 |
| FALLBACK | 1 |
| SKIP | 5 |
| DIVERGE | 0 |
| ERROR | 0 |

## Earlier campaign receipts

The following sections preserve earlier certification, fixture, corpus, rejection, and performance evidence.
They do not replace the current bounded result.

### Certified acceptance frontiers

Exact artifact profiles now enable three generic selection mechanisms.

| Language | Mechanism | Pinned C commit |
|---|---|---|
| HTTP | Accept one EOF head and drop no-action siblings | `db8b4398de90b6d0b6c780aba96aaa2cd8e9202c` |
| Robot | Accept one EOF head and drop no-action siblings | `278958ff2fc44732833f717ee864c9fe4dae6e11` |
| Meson | Select the sole primary accepted derivation | `c84f3540624b81fc44067030afce2ff78d6ede05` |
| Bash | Allow converged-path reduction split drops | `a06c2e4415e9bc0346c6b86d401879ffb44058f7` |
| Erlang | Allow converged-path reduction split drops | `1d78195c4fbb1fc027eb3e4220427f1eb8bfc89e` |
| Haskell | Allow converged-path reduction split drops | `0975ef72fc3c47b530309ca93937d7d143523628` |
| JavaScript | Allow converged-path reduction split drops | `58404d8cf191d69f2674a8fd507bd5776f46cb11` |
| Python | Allow converged-path reduction split drops | `bffb65a8cfe4e46290331dfef0dbf0ef3679de11` |

Separate Docker runs compared each selected Go tree with its pinned C parser.
All eight passed field-aware exhaustive comparison.

The evidence is in these artifact directories:

- `harness_out/docker/20260728T050445Z-compact-frontier-http-c-oracle-fields`
- `harness_out/docker/20260728T050509Z-compact-frontier-robot-c-oracle-fields`
- `harness_out/docker/20260728T050512Z-compact-frontier-meson-c-oracle-fields`
- `harness_out/docker/20260728T054141Z-compact-split-bash-c-oracle-fields`
- `harness_out/docker/20260728T054151Z-compact-split-erlang-c-oracle-fields`
- `harness_out/docker/20260728T054158Z-compact-split-haskell-c-oracle-fields`
- `harness_out/docker/20260728T054206Z-compact-split-javascript-c-oracle-fields`
- `harness_out/docker/20260728T104725Z-pr491-python-cert-final`

Custom, adapted, stale, and same-name grammars retain conservative defaults.

### Bounded no-lookahead reductions

The generic scheduler now supports one authenticated synthetic-EOF shape.
One runnable head can apply one reduction and re-elect at the same byte.
A transparent goto marks the reduced node as an extra.
A root reduction must meet authenticated EOF on the next election.
The scheduler declines wider frontiers, scanner changes, and runaway re-election.

This mechanism routes the Doxygen, JSDoc, and VHDL smoke fixtures.
Field-aware C-oracle runs passed for all three grammars:

- `harness_out/docker/20260728T061644Z-compact-nolookahead-doxygen-c-oracle-fields`
- `harness_out/docker/20260728T061726Z-compact-nolookahead-jsdoc-c-oracle-fields`
- `harness_out/docker/20260728T061735Z-compact-nolookahead-vhdl-c-oracle-fields`

### Zero-width extras with byte progress

The generic scheduler now recognizes progress from a parser boundary to the
end of a zero-width extra token.
It still declines when the byte, parser state, and scanner state do not change.

This mechanism routes the COBOL smoke fixture.
Its field-aware C-oracle run passed:

- `harness_out/docker/20260728T064111Z-compact-zero-width-cobol-c-oracle-fields`

### Cooklang smoke fixture

The prior Cooklang smoke source ended an ingredient instruction with a period.
Production recovery discarded that period.
The fixture now uses a valid ingredient instruction without the period.
The corrected source routes directly.
The old dotted source remains a required compact fallback.

The Cooklang field-aware C-oracle run passed:

- `harness_out/docker/20260728T064513Z-compact-clean-smoke-cooklang-c-oracle-fields`

### Lock-pinned corpus refresh

The previous 109-file matrix used a stale generated corpus directory.
The builder at `382080a3` selected one additional Elm highlight source.
Two independent rebuilds produced the same normalized manifest hash.
After volatile fields are removed, the hash is
`1e9998f1e4282c3c3397f518638a8779e016a0038064903f8de90f48b781661e`.

| Field | Value |
|---|---|
| Repository | `elm-tooling/tree-sitter-elm` |
| Commit | `6d9511c28181db66daee4e883f811f6251220943` |
| Source path | `test/highlight/basic.elm` |
| Bytes | 1,231 |
| SHA-256 | `8fca87bd8cc2735e83704acd8d06ffbc6cf04e386505de45596218d7fb72642c` |
| Compact result | PASS |

The tracked [Elm fixture](../testdata/admission_direct/elm_highlight_basic.elm)
protects this source when the generated corpus directory is absent.

The canonical ratchet enforces these bounds:

- At least 110 selected rows.
- At least 67 direct PASS rows.
- At most 33 FALLBACK rows.
- Exactly 10 SKIP rows.
- No DIVERGE or ERROR rows.

Ratchet mode rejects noncanonical language, bucket, AWK, and byte filters.
Manifest order does not affect the aggregate bounds.

### Rejected C# convergence candidate

The cap-one GSS convergence candidate did not preserve C# parity.
Commit `3204480b` lost field attributes and a modifier in this 65-byte witness:

```csharp
[System.Serializable]
struct S
{
    [System.Obsolete]
    int x;
}
```

Setting `GOT_GLR_MAX_MERGE_PER_KEY=16` restored the expected tree.
The 25-case Docker suite also found a net correctness and memory loss.

| Metric | `main` | Candidate |
|---|---:|---:|
| No-error outcomes | 24/25 | 24/25 |
| Deep tree matches | 20/25 | 19/25 |
| Maximum RSS | 1,382,676 KiB | 1,601,596 KiB |

The campaign rejected this candidate.

### Performance gate

The stable benchmark trio used:

- `GOMAXPROCS=1`
- `-count=10`
- `-benchtime=750ms`
- `-benchmem`

The comparison uses `f639fbaa` as the base.

| Benchmark | Time | Bytes | Allocations |
|---|---:|---:|---:|
| Full DFA parse | -8.01% | -12.06% | unchanged |
| Incremental single-byte edit | -5.71% | unchanged | unchanged |
| Incremental no-edit | +1.48% | unchanged | unchanged |

The large 5,000-function probe completed without an out-of-memory failure.
A balanced rerun measured 83,240 KiB for the base and 83,360 KiB for the head.

Compact reduction outputs now carry the multi-pop fact directly.
This avoids copying the complete compact work record twice per reduction.

The compact scheduler now stores its seed frontier in its own allocation.
The warm full-parse benchmark moves from 20,352 to 20,328 bytes per operation.
Allocation count moves from 66 to 65 per operation.
Time remains statistically unchanged (`p=0.853`).

## Reproduce the run

Run this command from the repository root:

```sh
GTS_ADMISSION_REAL_CORPUS=1 \
GTS_ADMISSION_REAL_CORPUS_RATCHET=1 \
GTS_ADMISSION_REAL_CORPUS_EXCLUDE_LANGS=awk \
GTS_ADMISSION_REAL_CORPUS_MAX_BYTES=16383 \
GTS_ADMISSION_CENSUS=1 \
GOMAXPROCS=1 go test . \
  -tags gts_parsercorephase0 \
  -run '^TestAdmissionCandidateRealCorpusMatrix$' \
  -count=1 \
  -v
```

The test reads `cgo_harness/corpus_real/manifest.json` by default.
Use `GTS_ADMISSION_REAL_CORPUS_MANIFEST` to select another manifest.

Add ratchet mode when you reproduce the current 110-row receipt.
The current result satisfies the canonical 110-row bounds.

Use these optional filters:

- `GTS_ADMISSION_REAL_CORPUS_LANGS`
- `GTS_ADMISSION_REAL_CORPUS_EXCLUDE_LANGS`
- `GTS_ADMISSION_REAL_CORPUS_BUCKETS`
- `GTS_ADMISSION_REAL_CORPUS_MAX_BYTES`

## Per-language matrix

| Language | PASS | FALLBACK | SKIP |
|---|---:|---:|---:|
| bash | 2 | 1 | 0 |
| c | 0 | 0 | 2 |
| c_sharp | 0 | 2 | 0 |
| clojure | 2 | 0 | 0 |
| cmake | 2 | 0 | 0 |
| cpp | 0 | 0 | 3 |
| css | 1 | 0 | 0 |
| d | 1 | 1 | 0 |
| dart | 0 | 2 | 0 |
| elixir | 2 | 1 | 0 |
| elm | 3 | 0 | 0 |
| erlang | 2 | 0 | 0 |
| go | 2 | 0 | 0 |
| gomod | 2 | 1 | 0 |
| graphql | 3 | 0 | 0 |
| haskell | 0 | 2 | 0 |
| hcl | 2 | 0 | 0 |
| html | 3 | 0 | 0 |
| ini | 0 | 2 | 0 |
| java | 0 | 0 | 2 |
| javascript | 1 | 0 | 0 |
| json | 0 | 0 | 3 |
| json5 | 2 | 0 | 0 |
| julia | 1 | 1 | 0 |
| kotlin | 0 | 2 | 0 |
| lua | 2 | 0 | 0 |
| make | 1 | 1 | 0 |
| markdown | 1 | 1 | 0 |
| nix | 3 | 0 | 0 |
| objc | 1 | 1 | 0 |
| ocaml | 2 | 0 | 0 |
| perl | 1 | 2 | 0 |
| php | 1 | 2 | 0 |
| powershell | 1 | 1 | 0 |
| python | 2 | 0 | 0 |
| r | 3 | 0 | 0 |
| ruby | 2 | 0 | 0 |
| rust | 1 | 0 | 0 |
| scala | 0 | 2 | 0 |
| scss | 2 | 0 | 0 |
| sql | 1 | 2 | 0 |
| svelte | 2 | 0 | 0 |
| swift | 1 | 1 | 0 |
| toml | 3 | 0 | 0 |
| tsx | 1 | 1 | 0 |
| typescript | 1 | 1 | 0 |
| xml | 2 | 0 | 0 |
| yaml | 3 | 0 | 0 |
| zig | 2 | 0 | 0 |

## Historical safety boundary receipt

The refreshed Kotlin fixture exposed one silent compact divergence.
The 38-byte reduction is:

```kotlin
internal actual fun f(): String = "x"
```

Production Go and tree-sitter C return the same error-bearing tree.
The compact parser previously returned a clean function declaration.

The compact scheduler merged two conflict paths.
A later reduction split those paths into separate heads.
The scheduler then dropped one head and accepted the other.

Admission fails closed after this unproved frontier shape.
Exact built-in profiles can certify the behavior against the C oracle.
Custom, adapted, stale, and unproved built-in grammars stay conservative.

The boundary originally moved four files to FALLBACK:

- Bash medium
- Erlang medium
- JavaScript small
- Kotlin small

Focused field-aware C-oracle receipts now certify Bash, Erlang, and JavaScript.
Those files route directly again. Kotlin remains fail-closed.

## Corpus state

The current manifest has these properties:

- 50 declared languages
- 50 languages with selected files
- 147 files
- No missing files
- No size mismatches
- No SHA-256 mismatches
- No stale source commits
- No absolute output paths

The focused quality gate passes:

```sh
cd cgo_harness
GTS_REAL_CORPUS_MANIFEST=corpus_real/manifest.json \
  go test . -run '^TestRealCorpusManifestQuality$' -count=1
```

The corpus directory is ignored by Git.
Rebuild it from the exact profile before a new campaign.
