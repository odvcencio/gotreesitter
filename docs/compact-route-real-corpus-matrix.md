# Compact route real-corpus matrix

Current evidence date: 2026-08-25.
Current base commit: `e4557bac36ec1794922a1ecce9ca5772d31f1a31` from `main`.
Current candidate base commit: `e4557bac36ec1794922a1ecce9ca5772d31f1a31`.

## Latest Swift issue #576 parser recovery candidate

Receipt base: `da6f71471aaaa835503accaa1bc2083ced90b4e6`.

Status: **GO FOR REVIEW / KEEP ISSUE #576 OPEN**. This candidate changes the
generic C-style recovery path. It does not change compact-route admission.

During recovery, the active DFA source replays from the exact skipped-prefix
offset. It verifies the stack offset, token span, points, token identity, and
scanner state. It resynchronizes the source before it emits `errorSymbol`.
Recovery replay requires a non-empty scanner checkpoint and live state. A
checkpointless scanner rejects recovery replay without changing the scanner.
Generic relex still accepts stateless scanners with empty serialization. The
Swift scanner now serializes its complete state. The parser records error-mode
lexing only after the DFA produces the token. The recovery path excludes tokens
from an external scanner. Each outer parse operation resets the recovery memo
size. Nested retries keep the larger memo. Temporary growth retains an existing
16,384-entry standard slab. Cleanup restores that slab without an allocation.
A shorter retained slab remains short. The parser returns a rejected recovery
probe before the legacy retry. Merge scratch drops its preflight state at a pool
boundary.

Entry scratch also enforces the source-sized reservation after pool reuse. It
moves an adequate retained slab to the front when possible. Otherwise, it adds
the required slab and keeps smaller slabs for later growth. This policy stops a
small control parse from changing the next large parse.

Swift no longer claims that failed scans preserve scanner state. The token
source restores failed scans by default. Swift explicitly retains its required
failed-scan state change. Retention takes precedence over failure preservation.
The source records actual start and end checkpoints for an internal token.
Each alternate retry starts from the original snapshot. After terminal
failure, only the last failed attempt remains live. Recovery replay still
requires equal checkpoints.
Incremental fast-forward restores the recorded end checkpoint. Generic relex
rejects a synthetic end-of-file lookahead that would replace a real token.
Swift defers `>?` to the DFA only when an unmatched `<` exists.

The 20-byte witness now matches locked C. The two corpus witnesses still
differ:

| Witness | Go deep SHA-256 | Locked C deep SHA-256 | Result |
|---|---|---|---|
| `let x = unsafe bar()` | `c64b894edc4a20e15f2b4127bad4223f698c8996dba091c06c34aa89386d3c68` | `c64b894edc4a20e15f2b4127bad4223f698c8996dba091c06c34aa89386d3c68` | exact |
| `stdlib_FloatingPointToString.swift` | `7cb588c1f7b44cf490d8fcddd11adb0cc56238e891156687c26660568a7f7447` | `ab96dddf088487acc700d72af9342c338901504dcf1d32b9644e9f6f6638190d` | mismatch |
| `stdlib_CollectionAlgorithms.swift` | `a3e737087be92518dbe1f8481a2b5169529b4f557b7f00033ab9da21d7aa32c9` | `132d332f511f12735d80e846f52ec1fddf5f3d0dcd7a097779640a7710497487` | mismatch |

Focused Docker gates passed with one Swift grammar, one CPU, and one test
worker. The artifacts are:

- Failed-scan mutation: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T104928Z-swift576-second-review-failed-scan`
- Incremental fast-forward: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T105001Z-swift576-second-review-fast-forward`
- Swift checkpoint grammar tests: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T105029Z-swift576-second-review-swift-checkpoints-v2`
- Generic relex contract: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T105035Z-swift576-second-review-generic-relex`
- Parser and scanner tests: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T105047Z-swift576-second-review-focused`
- Repeated clean-to-large sequence: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T105132Z-swift576-second-review-clean-large`
- Memory contract: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T105237Z-swift576-second-review-memory-contract`
- AWK recovery control: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T105100Z-swift576-second-review-awk`
- Both Swift corpus witnesses: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T105216Z-swift576-second-review-large-telemetry`

The TypeScript receipt changed only documentation and a focused test. The
Swift production inputs stayed unchanged during the rebase. These identity
gates passed on `da6f71471aaaa835503accaa1bc2083ced90b4e6`:

- Transition and generic relex tests: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T110309Z-swift576-da6-identity-root`
- Swift checkpoint grammar tests: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T110323Z-swift576-da6-swift-checkpoints`
- AWK recovery control: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T110332Z-swift576-da6-awk`
- Pooled minimal Swift parity: `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T110349Z-swift576-da6-pooled-minimal`

The scanner repair benchmark used 20 seeds and a 750 millisecond duration.
No timing or allocation result regressed. The geometric mean time decreased
3.57 percent. Bytes per operation stayed unchanged. The warmed large-witness
maximum resident set size had one 594240 KiB before sample and one 597160 KiB
after sample. The observed increase is 0.491384 percent, or about 0.49 percent.
The raw benchmark files are
`/tmp/swift576-scanner-checkpoint-before.txt` and
`/tmp/swift576-scanner-checkpoint-after.txt`.

PR #967 exposed Templ, Swift operator, recovery memo, and Markdown reuse
regressions. The repair restores default failed-scan rollback. Swift keeps an
explicit failure-retention capability. The final scanner, Swift, Templ, and
Markdown artifacts are:

- `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T121348Z-swift967-repair-final-scanner`
- `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T121422Z-swift967-repair-final-swift`
- `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T121436Z-swift967-repair-final-templ`
- `/tmp/gotreesitter-swift576-push-20260824/harness_out/docker/20260824T121446Z-swift967-repair-final-markdown-race`

The 20-seed primary trio changed by 1.242879 percent in the median geomean.
No primary timing lane changed significantly. Bytes and allocations per
operation stayed unchanged. One warmed large-witness sample changed from
565920 KiB to 565280 KiB. The observed decrease is 0.113090 percent.

The large witnesses still fail correctness. Keep issue #576 open until both
corpus witnesses match locked C.

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

Receipt base: `f0904533b6398775d5df5e01bc34d32feee34900`.

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

## C26f Swift issue #576 FloatingPointToString recovery blocker

Receipt base: `1c30650814ec6e65cbf31184301bf4776f3e5f41`.

Evidence base: `5648911ecf509df8ec870a1214917d9e95cf54f1`.

Status: **NO-GO / KEEP ISSUE #576 OPEN**. No production or test change
survives.

This receipt covers one tracked Swift source and one faithful prefix.
It does not claim an authoritative first-token trace.

### Grammar identities

The source is
`grammars/testdata/swift_corpus/stdlib_FloatingPointToString.swift`.
The manifest names `swiftlang/swift` at `swift-6.3-RELEASE`.
The manifest path is `stdlib/public/core/FloatingPointToString.swift`.

The Go Swift identity is:

| Field | Value |
|---|---|
| `go_swift_blob_sha256` | `be4575bc0acc3c60324aab635d067f940ac5f0557b80a8e3565d1e7d02d53582` |
| `swift_grammar_commit` | `41d6e5fe811ec94229ee71771174a8cce558dfee` |

The locked C identity is:

| Field | Value |
|---|---|
| `c_contract` | `tree-sitter-c-v1` |
| `c_transport` | `cgo_parity_binding` |
| `c_binding` | `v0.25.0@adc13ffd8b2c0b01b878fda9f7c422ce0df5fad3` |
| `c_runtime` | `0.25.1@f5afe475deb7c0bae6407fb776c76824f717bb61` |
| `c_grammar_repo` | [tree-sitter-swift](https://github.com/alex-pinkus/tree-sitter-swift) |
| `c_grammar_commit` | `41d6e5fe811ec94229ee71771174a8cce558dfee` |
| `c_grammar_artifact` | `/workspace/harness_out/parity_c_ref_cache/linux_amd64/swift-8d91e4446b76c6a7.so` |
| `c_grammar_artifact_sha256` | `2a9f14046d4ca88b6db1316ee5f48b876aea1700e3c09811b3c87257fe827c5c` |
| `compiler` | `/usr/bin/cc`, Debian 12.2.0-14+deb12u1 |

Two C26f route logs contain an identity defect:

| Artifact | Incorrect field values |
|---|---|
| `/tmp/gts-c26f-artifacts/20260823T061320Z-c26f-routes/container.log:2` | `go_grammar_commit=2346a3ab1bb3857b48b29d779a1ef9799a248cd7`; `go_grammar_blob_sha256=9cf914d26d962d1a62e7954f8b20b302337a44cb7d4a07218eec482c45a57a08` |
| `/tmp/gts-c26f-artifacts/20260823T061947Z-c26f-routes-gomax1/container.log:2` | `go:2346a3ab1bb3857b48b29d779a1ef9799a248cd7/9cf914d26d962d1a62e7954f8b20b302337a44cb7d4a07218eec482c45a57a08` |

These fields identify tree-sitter-Go. They do not identify Swift.
Treat them as artifact defects. Use the corrected Swift identity above.

### Witness bytes and tree digests

The full source has 104,681 bytes.
Its SHA-256 digest is
`ec96801e5237dff8da773f617a8a2f36e95b6a0a7c94b581855a451cd6507fdc`.
The faithful prefix has 7,316 bytes.
Its SHA-256 digest is
`6ebf3b26112a3df3611eafe82cd16ffa3f639b7b0f57608d9fdc422ccde78e72`.
The prefix ends after the complete `_float16ToStringImpl` function.

| Witness | Go deep SHA-256 | C deep SHA-256 | Go root | C root | First difference |
|---|---|---|---|---|---|
| Full, 104,681 bytes | `ec51c633a3f99515cc0cd1c0cff435a44ddc7db8e83705977d28f78bdfb0fc0e` | `ab96dddf088487acc700d72af9342c338901504dcf1d32b9644e9f6f6638190d` | `0..104681`, 1 child | `0..104681`, 353 children | `/source_file`, shape, Go children 1; C children 353 |
| Prefix, 7,316 bytes | `1460dfcc13dee3135ca5f8368cb58bdd374c203a06c45b19a23e56cca179cf36` | `2b72cea53a148ffc499ce3f8a817cfb8a27cf2a08efc158c2e011150426704d8` | `0..7316`, 130 children | `0..7316`, 130 children | `/source_file/function_declaration[129]/function_body[14]`, shape, Go children 5; C children 4 |

The full first errors are:

| Tree | First error |
|---|---|
| Go | `ERROR[0..104680] children=719` |
| Locked C | `ERROR[6828..6839] children=3` |

The prefix first errors are:

| Tree | First error |
|---|---|
| Go | `ERROR[6827..7314] children=6` |
| Locked C | `ERROR[6828..6839] children=3` |

The first full-source difference is a root shape difference.
The first prefix difference is a function-body child count.
The artifacts do not prove a first-token cause.

### Route results

Raw, production, compact, and incremental Go routes retain the Go digest.
Each route differs from locked C for both witnesses.

| Route | Full result | Prefix result |
|---|---|---|
| Raw | `exact=false`; Go deep digest above; stop accepted | `exact=false`; Go deep digest above; stop accepted |
| Production | `exact=false`; Go deep digest above; stop accepted | `exact=false`; Go deep digest above; stop accepted |
| Compact | `0/0 -> 0/1`; fallback at recovery | `0/1 -> 0/2`; fallback at recovery |
| Forest | `accepted=false`; offset 6254; symbol 175; `dead_end`; states `[10 2814]` | Same decline and state set |
| Incremental | `exact=false`; zero reuse; 124,563 new nodes | `exact=false`; zero reuse; 1,877 new nodes |

The compact decline is:

```text
compact route declined at recovery [mechanism=recovery-entered]: did not accept EOF: generic scheduler has no table action for the elected token
```

The forest route declines at offset 6254 with symbol 175.
It reports `dead_end` and states `[10 2814]` for both witnesses.

Incremental reuse is unsupported because of
`external_scanner_unsupported`.
It reports zero reused subtrees and zero reused bytes.
The full incremental profile reports `new_nodes=124563`.
The prefix profile reports `new_nodes=1877`.

### Recovery and pass telemetry

The full route reports these recovery values:

| Entries | Strategy 1 | Competitions | Walks | Error nodes | Error span | Retry passes | Retry attempts | Selected |
|---:|---:|---:|---:|---:|---:|---:|---:|---|
| 390 | 1,621 | 193,358 | 386,716 | 372 | 104,680 | 4 | 4 | `initial_merge` |

The full retry reason is `initial_result_requires_merge_width`.
The incremental route reports eight retry passes in its final fallback ladder.
The prefix route reports 13 entries, 36 strategy-one actions, 917
competitions, 1,834 walks, 6 error nodes, and error span 487.
The prefix route reports zero retry passes and zero retry attempts.

The full route reports `max_stacks=28` and `peak_depth=637`.
The prefix route reports `max_stacks=14` and `peak_depth=153`.

Normalization performs no rewrites.
Full pass counters are `1/1/19290/0` for each Swift pass.
Prefix pass counters are `1/1/447/0` for each Swift pass.
The pass visits do not establish parity.

### Memory and run controls

Resident set size (RSS) values are workload observations.
They are not a performance comparison.

| Artifact | Workload | RSS | Controls |
|---|---|---:|---|
| `/tmp/gts-c26f-artifacts/20260823T060621Z-c26f-prefix-raw` | Raw probe | 445,268 KiB | One CPU; the log does not state `GOMAXPROCS=1` |
| `/tmp/gts-c26f-artifacts/20260823T061320Z-c26f-routes` | Full and prefix routes | 1,210,240 KiB | One CPU; the log does not state `GOMAXPROCS=1` |
| `/tmp/gts-c26f-artifacts/20260823T061947Z-c26f-routes-gomax1` | Full and prefix routes | 988,640 KiB | One CPU; `GOMAXPROCS=1` |
| `/tmp/gts-c26f-artifacts/20260823T062159Z-c26f-reduction-min` | Incomplete reductions | 297,728 KiB | One CPU; `GOMAXPROCS=1` |

The Docker runs used 4 GiB of memory, a 3 GiB Go memory limit, `-p=1`,
test parallelism one, and a 20-minute timeout.
The retained logs report exit code zero.
No run timed out or hit an out-of-memory failure.
Do not compare these RSS values as a stable performance result.

### Reduction scope and exclusions

The raw prefix artifact includes two excluded, different-family slices:

| Bytes | Source SHA-256 | Exclusion |
|---:|---|---|
| 781 | `24bd84e42e555749833bc8652e3ca6893297593ca532af6faeef689db3b21008` | `/source_file/comparison_expression[3]`; Go has `comparison_expression`, C has `constructor_expression` |
| 711 | `9dae29100a640d762fa8eca8cec13a336d46778e4c0126e1f9d5c7dbe6a34eac` | `/source_file/comparison_expression[3]`; Go has `comparison_expression`, C has `constructor_expression` |

These slices are not C26f evidence.
They use a different divergence family.

The minimum reduction artifact is
`/tmp/gts-c26f-artifacts/20260823T062159Z-c26f-reduction-min`.
It contains incomplete prefixes at 6805, 6855, 6883, 6912, and 6913 bytes.
It does not contain the canonical 7,316-byte prefix.

The full reduction sweep is
`/tmp/gts-c26f-artifacts/20260823T062139Z-c26f-reduction-sweep`.
It contains endpoints 6913, 6978, 7042, 7088, 7121, 7266, and 7316.
Only endpoint 7316 is the complete canonical prefix.
Do not use the other endpoints as complete witnesses.

### Canopy reachability

Canopy version: `v0.18.0-19-g01a5f95-dirty`.

Scoped no-cache queries confirmed these reachable generic paths:

- `cHandleError`: `parser_recover_c.go:3290-3566`; call at line 4670.
- `cCondenseAndResume`: `parser.go:6713` and `parser.go:6879`.
- `parseForRecoveryWithMode`: `parser_api.go:568`; wrappers at lines 540 and 546.
- `nextGLRUnionDFAToken`: `parser_dfa_token_source.go:1113-1279`; calls at lines 539 and 746.
- `dispatchPass`: `parsercore_phase0_driver.go:5521`; called by `run` at line 5503.
- `dropGenericNoActionHeads`: `parsercore_phase0_driver.go:7203-7304`; calls at lines 5856 and 5976.

These paths show reachability only.
The retained artifacts contain tree shapes and counters.
They do not contain an authoritative first-token event trace.
Therefore, this receipt does not assign the first divergence to one token.

### Decision and reopening condition

Keep issue #576 open.
Do not ship parser, grammar, or test changes.
Do not add a Swift rule, source hash exception, or blob exception.

Reopen implementation work only after a generic grammar, lexer, or parser
producer change supplies an authoritative trace and a safe proof.
Require the corrected Swift identities and an authenticated source lock.
Require Go and locked C equality on the full and 7,316-byte witnesses.
Run raw, production, compact, forest, and incremental routes.
Require compact and forest routes to produce certified comparisons.
Require the same proof on the authenticated corpus.

## C26h Swift issue #576 MutableSpan producer-boundary blocker

Receipt base: `41d0b9de133de777aeba9c1dca091903da052a7f`.

Evidence base: `54c7f521505e23b7a32c84c2a14d3bd3175c09dd`.

Status: **NO-GO / KEEP ISSUE #576 OPEN**. No production or test change
survives.

This receipt extends C26f with a durable producer trace.
It does not alter compact admission or D6 status.

### Identities and prefixes

The source is
`grammars/testdata/swift_corpus/stdlib_FloatingPointToString.swift`.
It has 104,681 bytes.
Its source SHA-256 digest is
`ec96801e5237dff8da773f617a8a2f36e95b6a0a7c94b581855a451cd6507fdc`.

The Go Swift blob is
`be4575bc0acc3c60324aab635d067f940ac5f0557b80a8e3565d1e7d02d53582`.
The Swift grammar commit is
`41d6e5fe811ec94229ee71771174a8cce558dfee`.

The locked C grammar artifact is
`2a9f14046d4ca88b6db1316ee5f48b876aea1700e3c09811b3c87257fe827c5c`.
The locked C runtime is `0.25.1` at commit
`f5afe475deb7c0bae6407fb776c76824f717bb61`.

The full-source Go deep digest is
`ec51c633a3f99515cc0cd1c0cff435a44ddc7db8e83705977d28f78bdfb0fc0e`.
The full-source locked-C deep digest is
`ab96dddf088487acc700d72af9342c338901504dcf1d32b9644e9f6f6638190d`.

The six focused prefixes have these identities and digests:

| Bytes | Source SHA-256 | Go deep SHA-256 | Locked-C deep SHA-256 | First difference |
|---:|---|---|---|---|
| 6,805 | `525674aacceaeddb55fddef838cbb6b167db636dc0a171df3a8351ca15248c30` | `205b67cae26d6d42733c0bc1223e27406d2a435c25a162ed994a6571303ef688` | `47d1f1c78f541925f9deebf02d38bb78a68860057905e251aad4c99890fe3c99` | `/source_file`, type, Go `source_file`, C `ERROR` |
| 6,855 | `d3e340b8ef6c87769933cbb000557eeb87eff04ae8fbc27568ccefeecb0f44f7` | `1b8299a2c6ce8c6f7aafed2468fb426b191941e5966e0fa84bf162739c807498` | `3f3ea8f5fcdfba0ae9ac2e8d1b8672f2e1dfcd34c3d6625781bb0c54724ab8c4` | `function_declaration[129]/function_body[14]`, shape, 5 versus 4 children |
| 6,883 | `1d067da11cccfb64db6cb1025748994cb53cfdfb00a761b06e8b31b93af411ae` | `ac5aa2b8ff258c40c1b9405189fcea61d7d93b609fa018bb65ff40840eb37834` | `5e1bb7b780e5dedb4df2017895e203da0fb4d4e00c0eac6769788c430593494c` | `function_declaration[129]/function_body[14]`, shape, 5 versus 4 children |
| 6,912 | `c65a11b82e34178bb7189140f85e81b3c632ee4ed292b34b6257f294f25de690` | `73496d47059acfd101e1e3ee752cd8709e11432fbe0656510e8016f7fcb8ad07` | `90c282b8896a39a531e789084e80383fc48f2bffb23bb1fc7a39dce51d2950b9` | `/source_file`, type, Go `source_file`, C `ERROR` |
| 6,913 | `7f7870469f8b9e345b4295aa98d067235b0fee476617ee1f9379d54d96d7053a` | `06868a0ba3857e49cf34aba018f31e2ee8b1339153913503c4fc8468ebb2905b` | `9f43c73bc5858107e510b2455060e3f315ee17a6526bc4001fe106dce454b09a` | `function_declaration[129]/function_body[14]`, shape, 5 versus 4 children |
| 7,316 | `6ebf3b26112a3df3611eafe82cd16ffa3f639b7b0f57608d9fdc422ccde78e72` | `1460dfcc13dee3135ca5f8368cb58bdd374c203a06c45b19a23e56cca179cf36` | `2b72cea53a148ffc499ce3f8a817cfb8a27cf2a08efc158c2e011150426704d8` | `function_declaration[129]/function_body[14]`, shape, 5 versus 4 children |

Raw, production, compact, and incremental Go routes retain each Go digest.
Each route differs from locked C for every prefix.

### Durable producer trace

The event-origin artifacts are:

- `/tmp/gts-c26h-swift-20260824/harness_out/docker/20260823T074540Z-c26h-swift-event-origin/container.log`
- `/tmp/gts-c26h-swift-20260824/harness_out/docker/20260823T074540Z-c26h-swift-event-origin/metadata.txt`
- `/tmp/gts-c26h-swift-20260824/harness_out/docker/20260823T074540Z-c26h-swift-event-origin/inspect.json`

The trace records this Go event:

```text
DFA tok 160 ... 6828 6839 MutableSpan state=10
```

Go then emits token 35 from byte 6839.

Locked C enters external state 1 and internal state 0 at row 156, column 23.
It emits `ERROR` with size 2.
It calls `detect_error`, resumes version 0, and skips the `ERROR` token.

The C trace later skips unrecognized characters before later recovery events.
The first mismatch is therefore producer-visible.
It occurs before recovery election, condense, or tree materialization.

### Route and resource results

The route artifacts are:

- `/tmp/gts-c26h-swift-20260824/harness_out/docker/20260823T074338Z-c26h-swift-probe-cgo/container.log`
- `/tmp/gts-c26h-swift-20260824/harness_out/docker/20260823T074338Z-c26h-swift-probe-cgo/metadata.txt`
- `/tmp/gts-c26h-swift-20260824/harness_out/docker/20260823T074338Z-c26h-swift-probe-cgo/inspect.json`

The timed route artifacts are:

- `/tmp/gts-c26h-swift-20260824/harness_out/docker/20260823T074507Z-c26h-swift-routes-time/container.log`
- `/tmp/gts-c26h-swift-20260824/harness_out/docker/20260823T074507Z-c26h-swift-routes-time/metadata.txt`
- `/tmp/gts-c26h-swift-20260824/harness_out/docker/20260823T074507Z-c26h-swift-routes-time/inspect.json`

The compact route declines every prefix.
Each focused counter delta records zero admissions and one fallback.

The forest route declines every prefix.

The incremental route reports `external_scanner_unsupported`.
It reuses zero subtrees and zero bytes.
At 7,316 bytes, it allocates 1,877 new nodes and sees 14 stacks.

The timed run observes 232,640 KiB maximum resident set size (RSS).
This value is a workload observation, not a performance comparison.

The explicit controls are one Swift grammar workload, one CPU, 4 GiB,
`-parallel=1`, `GOFLAGS=-p=1`, and `GOMEMLIMIT=3GiB`.
The wrapper metadata does not record `GOMAXPROCS=1`.
This receipt makes no claim about that setting.

The route and event runs report exit code zero.
They report no out-of-memory failure and no timeout.

### Canopy ownership and candidate boundary

Canopy version: `v0.18.0-19-g01a5f95-dirty`.

Scoped no-cache queries trace these generic paths:

- `dfaTokenSource.Next` at `parser_dfa_token_source.go:466`.
- `nextExternalToken` at `parser_dfa_token_source.go:2967`.
- `nextTokenForLexState` at `parser_dfa_token_source.go:945`.
- `cRecoverAcquireToken` at `parser_recover_c.go:776`.
- `cRecoverInternalErrorModeToken` at `parser_recover_c.go:926`.
- `cHandleError` at `parser_recover_c.go:3290`.
- `cRecover` at `parser_recover_c.go:3579`.
- `cCondenseAndResume` at `parser_recover_c.go:4462`.
- `updateParserStateTokenSource` at `parser.go:7823`.

The internal error-mode helper declines languages with an external scanner.
The Swift scanner therefore keeps its own error-mode semantics.

Do not convert identifier token 160 to `ERROR`.
That change could reject valid identifiers in other states and grammars.

Do not force Go to skip the identifier.
That change could discard a valid producer token.

Do not use a Swift rule, source hash, grammar blob, or artifact policy.
No grammar-agnostic correction is proven.

### Artifacts and reopening condition

The local C26h report is
`/tmp/gts-c26h-artifacts/20260823T0745Z-c26h-swift-boundary/report.md`.
Its SHA-256 digest is
`86f7d0aac31462ad8bce98d56b360eeb324a7137b910b6603541c997479b2a6e`.

The local report records the corrected controls and all six route digests.
The prior C26g route and event logs remain retained:

- `/tmp/gts-c26g-swift-20260824/harness_out/docker/20260823T071059Z-c26g-swift-prefix-routes/container.log`
- `/tmp/gts-c26g-swift-20260824/harness_out/docker/20260823T071125Z-c26g-swift-event-trace/container.log`

Reopen implementation work only after a scanner-aware, grammar-agnostic
error-mode token-frontier contract gains proof.
Require matching token identity, span, and skip behavior on this boundary.
Require clean controls and locked-C parity on raw, production, compact,
forest, and incremental routes.
Require the authenticated corpus and its source lock before retirement.
Keep issue #576 open until these conditions pass.

## C26i Swift issue #576 scanner-aware token-frontier blocker

Evidence base: `137860ebd80921094e5a8069007d49188dcb5e50`.
Publication base: `7b6f40fe089283674f5d0d19408d2380f77caf68`.

Status: **NO-GO / KEEP ISSUE #576 OPEN**. No production or test change
survives.

This receipt audits the C26h producer boundary from the evidence base.
The evidence base changes no parser, lexer, scanner, or grammar code.

### Witness identities and results

The source is
`grammars/testdata/swift_corpus/stdlib_FloatingPointToString.swift`.
It has 104,681 bytes.
Its source SHA-256 digest is
`ec96801e5237dff8da773f617a8a2f36e95b6a0a7c94b581855a451cd6507fdc`.

The Go Swift blob is
`be4575bc0acc3c60324aab635d067f940ac5f0557b80a8e3565d1e7d02d53582`.
The Swift grammar commit is
`41d6e5fe811ec94229ee71771174a8cce558dfee`.
The locked C grammar artifact SHA-256 digest is
`2a9f14046d4ca88b6db1316ee5f48b876aea1700e3c09811b3c87257fe827c5c`.
The locked C runtime is `0.25.1` at commit
`f5afe475deb7c0bae6407fb776c76824f717bb61`.

The full-source Go deep digest is
`ec51c633a3f99515cc0cd1c0cff435a44ddc7db8e83705977d28f78bdfb0fc0e`.
The full-source locked-C deep digest is
`ab96dddf088487acc700d72af9342c338901504dcf1d32b9644e9f6f6638190d`.
Go has one root child. Locked C has 353 root children.
Go's first error spans `0..104680`. C's first error spans `6828..6839`.

The six prefix witnesses retain the C26h identities and results.

| Bytes | Source SHA-256 | Go deep SHA-256 | Locked-C deep SHA-256 |
|---:|---|---|---|
| 6,805 | `525674aacceaeddb55fddef838cbb6b167db636dc0a171df3a8351ca15248c30` | `205b67cae26d6d42733c0bc1223e27406d2a435c25a162ed994a6571303ef688` | `47d1f1c78f541925f9deebf02d38bb78a68860057905e251aad4c99890fe3c99` |
| 6,855 | `d3e340b8ef6c87769933cbb000557eeb87eff04ae8fbc27568ccefeecb0f44f7` | `1b8299a2c6ce8c6f7aafed2468fb426b191941e5966e0fa84bf162739c807498` | `3f3ea8f5fcdfba0ae9ac2e8d1b8672f2e1dfcd34c3d6625781bb0c54724ab8c4` |
| 6,883 | `1d067da11cccfb64db6cb1025748994cb53cfdfb00a761b06e8b31b93af411ae` | `ac5aa2b8ff258c40c1b9405189fcea61d7d93b609fa018bb65ff40840eb37834` | `5e1bb7b780e5dedb4df2017895e203da0fb4d4e00c0eac6769788c430593494c` |
| 6,912 | `c65a11b82e34178bb7189140f85e81b3c632ee4ed292b34b6257f294f25de690` | `73496d47059acfd101e1e3ee752cd8709e11432fbe0656510e8016f7fcb8ad07` | `90c282b8896a39a531e789084e80383fc48f2bffb23bb1fc7a39dce51d2950b9` |
| 6,913 | `7f7870469f8b9e345b4295aa98d067235b0fee476617ee1f9379d54d96d7053a` | `06868a0ba3857e49cf34aba018f31e2ee8b1339153913503c4fc8468ebb2905b` | `9f43c73bc5858107e510b2455060e3f315ee17a6526bc4001fe106dce454b09a` |
| 7,316 | `6ebf3b26112a3df3611eafe82cd16ffa3f639b7b0f57608d9fdc422ccde78e72` | `1460dfcc13dee3135ca5f8368cb58bdd374c203a06c45b19a23e56cca179cf36` | `2b72cea53a148ffc499ce3f8a817cfb8a27cf2a08efc158c2e011150426704d8` |

Raw, production, compact, and incremental routes retain the Go digest.
Each route differs from locked C on every prefix.
Compact declines every prefix with one fallback and zero admissions.
Forest declines every prefix.
Incremental reports `external_scanner_unsupported` and zero reuse.
The 7,316-byte run allocates 1,877 new nodes and sees 14 stacks.

### Focused validation

The full-witness Docker artifact is
`/tmp/gts-c26i-swift-20260824/harness_out/docker/20260823T081122Z-c26i-swift-full-base`.
Its command runs the full and minimal Swift mismatch guards.
The run passed with exit code zero.
It used one Swift grammar, one CPU, 4 GiB, `-parallel=1`, `GOFLAGS=-p=1`,
and `GOMEMLIMIT=3GiB`.
The command did not set `GOMAXPROCS`.
The run had no out-of-memory kill and no wall timeout.

The exact six-prefix route artifacts remain at:

- `/tmp/gts-c26h-swift-20260824/harness_out/docker/20260823T074338Z-c26h-swift-probe-cgo`
- `/tmp/gts-c26h-swift-20260824/harness_out/docker/20260823T074507Z-c26h-swift-routes-time`

The timed C26h route observed 232,640 KiB maximum resident set size (RSS).
This value is a workload observation, not a performance comparison.

### Producer trace and Canopy ownership

The durable event trace is
`/tmp/gts-c26h-swift-20260824/harness_out/docker/20260823T074540Z-c26h-swift-event-origin/container.log`.
It records this Go event:

```text
DFA tok 160 ... 6828 6839 MutableSpan state=10
```

Go then emits token 35 from byte 6839.
Locked C reaches row 156, column 21, with parser state 2408.
It tries external state 25 and internal state 716.
Both normal attempts fail.
It retries external state 1 and internal state 0.
It skips one unrecognized character and emits `ERROR` with size 2 at column 23.
It calls `detect_error`, resumes version 0, and skips the `ERROR` token.

The C and Go traces use different parser-state number spaces.
Do not equate C state 2408 with Go state 198 or state 10.
The first difference occurs before recovery election, condense, or materialization.

Canopy version `v0.18.0-19-g01a5f95-dirty` traced these generic paths:

- `dfaTokenSource.Next` to `nextExternalToken` and `nextDFAToken`.
- `nextDFAToken` to `scanPreferredTokenForState` and `nextTokenForLexState`.
- `cRecoverAcquireToken` to `cRecoverInternalErrorModeToken`.
- `updateParserStateTokenSource` to the shared parser-state frontier.
- `cHandleError`, `cRecover`, and `cCondenseAndResume` after token production.

The scoped commands were:

```text
scripts/canopy_query.sh search symbols parser_dfa_token_source.go --limit 120
scripts/canopy_query.sh search symbols parser_recover_c.go --limit 120
canopy graph calls 'nextExternalToken' parser_dfa_token_source.go --no-cache --reverse --depth 3
canopy graph calls 'scanPreferredTokenForState' parser_dfa_token_source.go --no-cache --reverse --depth 3
canopy graph calls 'cRecoverAcquireToken' parser_recover_c.go --no-cache --depth 3
```

### Generic contract boundary

The reusable invariant is this:

> A parser version in recovery error mode must retry its scanner-aware error
> frontier before it accepts a normal token. Apply this retry only after its
> normal external and internal lex attempts fail.

The contract must bind the parser version, scanner state, external lex state,
internal lex state, token span, and skip result.
The current token source shares one scanner payload across live versions.
The internal error-mode helper rejects languages with an external scanner.
Swift serializes its raw-string hash only.
Its carried previous-rune state is not serialized.
No generic scanner-state transfer proof exists.

Changing identifier symbol 160 to `ERROR` can reject valid identifiers.
Forcing Go to skip the identifier can discard valid input.
No grammar-agnostic candidate diff exists.
The temporary inspection test was removed.

D6a and D6b authenticate compact drop frontiers after token production.
They cannot create C's missing token or alter scanner state.
D6 frontier work does not change this blocker.

### C26j per-version scanner ownership follow-up

Evidence base: `137860ebd80921094e5a8069007d49188dcb5e50`.

The C26j audit found one mutable `externalPayload` in each
`dfaTokenSource`. A GLR stack clone copies parser and recovery data, but it
does not copy an external scanner payload or checkpoint.

`updateParserStateTokenSource` selects one primary parser state. It sends the
union of active GLR states to the shared token source. Retry snapshots save and
restore that same shared payload.

`relexTokenForStackLexState` uses the internal DFA for a parser version. It
does not re-enter the external scanner. `cCondenseAndResume` selects and
resumes a version, but it does not transfer scanner state to that version.

Swift `Serialize` writes its raw-string hash count. `Deserialize` clears
`carriedPreviousRune` and `carriedPreviousValid`. Serialized bytes alone
cannot restore every value that can affect the next scanner call.

The default-off, opt-in complete checkpoint capability must meet seven requirements:

1. Clone complete scanner state for every parser version.
2. Bind the checkpoint to scanner identity, grammar blob, parser-version
   lineage, source byte and point, external lex state, and serialized state.
3. Record state before and after successful and failed scans.
4. Copy the checkpoint when GLR creates a parser version.
5. Transfer the selected checkpoint during recovery relex, condense, and
   resume, and discard checkpoints for dead versions.
6. Require exact checkpoint identity before versions merge or share a token.
7. Revalidate the checkpoint before edit reuse, then fail closed when the
   scanner does not support the contract.

The root controls passed
`TestNextExternalTokenRetriesScannerAfterFailedPreferredCandidate`,
`TestCRecoveryGateValidatesParseTableActionAndGotoBounds`, and
`TestParserExternalScannerToken`. The scanner-free control
`TestParserIncrementalArithmeticEditMatchesFreshParse` also passed.
The Swift controls passed
`TestParitySwiftCleanRecoveryProbeControls` and both known-mismatch guards.

The root-control artifact is
`/tmp/gts-c26j-scanner-contract-20260824/harness_out/docker/20260823T082443Z-c26j-controls`.
The Swift-control artifact is
`/tmp/gts-c26j-scanner-contract-20260824/harness_out/docker/20260823T082454Z-c26j-swift`.
The Swift run used one Swift grammar. Both runs used one CPU, 4 GiB,
`-parallel=1`, `GOFLAGS=-p=1`, and `GOMEMLIMIT=3GiB`.
Neither metadata record sets `GOMAXPROCS`.
Both runs passed without an out-of-memory kill or wall timeout.

The next proof must exercise two parser versions through a failed scan, GLR
fork, C recovery retry, condense, resume, merge, and edit reuse. Run one
external-scanner control and one scanner-free control. Require locked-C parity
before testing compact admission.

### C26k conclusion

Evidence base: `5d39d9658f5071c5c0f476eaadc6ae067e6c77e1`.

The existing `CheckpointedExternalScanner` contract proves incremental
checkpoint boundaries only. It does not assign scanner state to a parser
version. `Language` has no generic grammar-blob identity. A record-only slice
cannot prove ownership across GLR forks, merges, or recovery resumes.

The integration blast radius includes the token source, GLR stack and
scheduler, merge paths, C recovery, and incremental reuse. Focused controls
passed. The exact artifacts are:

- `/tmp/gts-c26k-checkpoint-contract-20260824/harness_out/docker/20260823T084110Z-c26k-root-controls`
- `/tmp/gts-c26k-checkpoint-contract-20260824/harness_out/docker/20260823T084134Z-c26k-external-control`
- `/tmp/gts-c26k-checkpoint-contract-20260824/harness_out/docker/20260823T084144Z-c26k-scanner-free-control`
- `/tmp/gts-c26k-checkpoint-contract-20260824/harness_out/docker/20260823T084153Z-c26k-offmode-bench`
- `/tmp/gts-c26k-checkpoint-contract-20260824/harness_out/docker/20260823T084251Z-c26k-checkpoint-boundary-controls`

The runs used one CPU, 4 GiB, `GOMEMLIMIT=3GiB`, `GOFLAGS=-p=1`, and
`-parallel=1`. The metadata does not set `GOMAXPROCS`. No candidate exists, so
the off-mode benchmark is observational only. It recorded 13.69 ms/op,
14.03 MB/op, and 3,742 allocations per operation.

The narrower prerequisite is a complete checkpoint API with identity. Add
synthetic tests for independent fork copies, exact merge identity, elected
state transfer during recovery, failed-scan restoration, an external-scanner
control, and a scanner-free control. Keep the capability default off and fail
closed when identity or completeness is unavailable. No code or test change
was made.

## C26l Swift issue #576 checkpoint foundation

Evidence and publication base:
`929609ccde78b0c9f4e57cf2225e0ae1204149cb`.

Status: **ACCEPTED DEFAULT-OFF FOUNDATION**. Keep issue #576 open.

C26l adds an opt-in identity-bearing checkpoint capability. It does not add a
parser production call site. It does not change Swift, grammar, generalized
LR (GLR), recovery, merge, or incremental paths. It does not change behavior
when the capability is absent.

### API and safety boundary

`ExternalScannerCheckpointIdentityProvider` embeds the existing
`CheckpointedExternalScanner` contract. It adds `CheckpointIdentity`, which
must return stable, non-empty scanner and grammar identifiers. Each identifier
must be at most 256 bytes. The existing contract requires complete
serialization. C26l also requires non-empty serialized bytes from `Serialize`.

The internal owned record stores:

- scanner and grammar identifiers;
- source byte and source point;
- external lexer state;
- token start and end bytes;
- serialized scanner bytes.

Capture fails closed when the capability is absent, disabled, or incomplete.
It also fails closed when identity is missing or too large, serialization is
empty or too large, or the token span is inverted. Capture deep-copies identity
and serialized bytes. Fork cloning makes independent copies. Share and merge
require exact identity, source position, lexer state, token span, and
serialized bytes. Restore checks exact identity before it calls `Deserialize`.
It serializes the restored payload into the bounded buffer and requires exact
length and byte equality. It returns false on any mismatch. Callers must
discard a payload after failed verification. Restore passes a copied byte
slice and rejects an inverted token span.

The synthetic lifecycle tests cover:

- absent capability, incomplete identity, oversized identity, or invalid
  serialization;
- owned identity and serialized state bytes;
- independent fork copies;
- exact merge equality and mismatch;
- elected recovery-state transfer;
- restore after a failed scan;
- an internally overlong record and restore verification mismatch;
- identity-mismatch restore failure;
- an external-scanner control;
- scanner-free off mode.

### Structural proof and focused validation

The new files are
`external_scanner_checkpoint_capability.go` and
`external_scanner_checkpoint_capability_test.go`. Scoped Canopy searches found
the new definitions and test references. Direct no-cache Canopy impact for
`captureExternalScannerCheckpointRecord` returned only 14 affected test
functions in `external_scanner_checkpoint_capability_test.go`. It returned no
affected production file. A literal search found no production caller outside
the new API file. This proves zero production call sites. Therefore the
absent-capability path has no parser runtime or allocation change. This is a
structural proof, not a performance benchmark.

The focused Docker artifact is
`/tmp/gts-c26l-checkpoint-foundation-rebase/harness_out/docker/20260823T102036Z-c26l-checkpoint-foundation-rebase`.
The run executed the C26l tests in the root package. It used one central
processing unit (CPU), 4 GiB, `GOMEMLIMIT=3GiB`, `GOFLAGS=-p=1`, and
`-parallel=1`. The metadata does not set `GOMAXPROCS`. The run passed with
exit code zero. It had no out-of-memory kill and no wall timeout. This focused
validation did not collect maximum resident set size (RSS).

This foundation does not prove scanner ownership across real parser versions.
It does not prove GLR scheduling, recovery, merge, or edit-reuse parity. The
next proof must connect the capability to those paths without enabling it for
Swift or any grammar. Keep issue #576 open until that proof passes against
locked C.

### C26i decision and reopening condition

Keep issue #576 open.
Ship no parser, grammar, or test change.

Reopen implementation work only after a generic scanner-aware frontier
capability proves all of the following:

- Preserve per-version scanner state or an authenticated scanner checkpoint.
- Match normal and error token identity, span, and skip behavior.
- Pass clean controls and all six prefixes in locked-C parity.
- Pass the full witness on raw, production, compact, forest, and incremental routes.
- Use an authenticated source lock and corpus evidence.


## C26q SQL scanner identity gate

Publication base: `a62b9db306bcb983852cbf0043852546e864e856`.

Status: **GO FOR THE BOUNDED IDENTITY GATE.**

C26q binds an external scanner checkpoint to its scanner and grammar identity.
The gate runs at incremental checkpoint reuse. It does not change GLR
scheduling, recovery election, condense, merge, or forest ownership.

### Identity inputs

The native SQL scanner uses these authenticated inputs:

- Upstream repository: [tree-sitter-sql](https://github.com/m-novikov/tree-sitter-sql).
- Upstream commit: `587f30d184b058450be2a2330878210c5f33b3f9`.
- `src/grammar.json`: 319,142 bytes,
  SHA-256 `42f011860137175a5a0cb820d1a694e5ccca1d17f226729ff6a4e886910cde1c`.
- `src/scanner.cc`: 4,333 bytes,
  SHA-256 `d437ad9f517d7a1f4248ccd05abe58370b5040c0037c877dab1f0aefeaa04af6`.
- External names: `_dollar_quoted_string_tag`,
  `_dollar_quoted_string_content`, and `_dollar_quoted_string_end_tag`.
- Scanner semantics: tagged state, zero-length empty state, bounded tags,
  content-end scanning, state preservation on failure, and a 4,095-byte tag
  limit.

The scanner identity is the raw SHA-256 value
`7e493677411a501e6d8592c6b9cc158e21a1bfed44c72ca914e2d81e4e34861d`.
It includes the local Go port hash
`588328cd27eea49e88b704b9bd8e46958046564187a5db1a70f6622308a7fff8`.
The focused test hashes the marked local source region. A semantics change
fails the test until the scanner identity is updated with review.

The checked-in SQL blob is `grammars/grammar_blobs/sql.bin`. It has 581,443
bytes and SHA-256
`e21421cbab52b54cf5ba15c8f78a2bb4729bf4e8c0da14368069e897de451268`.
The trusted exact-byte loader records this hash before it adapts the SQL
scanner. The public `Language` API has no grammar identity setter. Callers
cannot relabel a loaded language after adaptation.

`gotreesitter.LoadLanguage` is the trusted construction path for this identity.
It hashes the exact input bytes after envelope validation. A manually created
`Language` has no trusted grammar identity and fails closed at reuse. The
focused tests reject scanner and grammar drift when raw checkpoint bytes remain
equal. The SQL source guard uses test-only file reads. Production code does not
read its source file. Identity set and inherit paths charge only the exact
state delta. Conflict state removes the charge, and reset clears it.

The override loader reads the exact override bytes and hashes them before
scanner adaptation. It never copies the checked-in grammar identity.
Each SQL identity contains two raw 32-byte SHA-256 values. The arena accounts
for 64 identity bytes in the parser memory budget. Arena reset clears them.
Tree copies and checkpoint copies preserve them. Legacy scanners without the
capability keep their previous reuse behavior.

### Native SQL certification

The native SQL language loaded the checked-in blob identity above. The focused
tests passed for identity stability, local-port source binding, checkpoint
round trips, scanner failure restore, native blob identity, cross-scanner
rejection, exact arena accounting, and SQL incremental certification.

The route probe reported this digest for every accepted route:
`2c78089192b45b7aa6f2870fa8d2e492f00ce403c6b4b26fd59d6dfa28572982`.
Production, compact, included-ranges, and incremental routes matched.
Forest admission was true. The route artifact used one CPU, 4 GiB,
`GOMEMLIMIT=3GiB`, and `GOFLAGS=-p=1`.

`/tmp/gts-c26q-hardening-artifacts/20260823T141907Z-c26q-hardening-sql-route`

Its `container.log` SHA-256 is
`269d63d598e8ffbcf9590412af73e2f18469846dc9108e29d11e8eb15b662ce4`.
Its `metadata.txt` SHA-256 is
`d40840ee4aa12992707fd8344e73ab2b540e6abb10e17b21fcf2beb6293d5cb4`.

The SQL incremental certification artifact is:

`/tmp/gts-c26q-hardening-artifacts/20260823T141430Z-c26q-hardening-sql-incremental`

Its `container.log` SHA-256 is
`a9c95e8b30bea5437c8f0e39116a08b70565b5ccada0280ec97073546261156f`.
Its `metadata.txt` SHA-256 is
`a607c51e6cceef40f86a9a8ecb5ea6666b504c00b64d518840b05b8675d68c79`.

The post-hardening raw and range gate covered SQL scanner recovery, the
no-tree retry, SQL incremental certification, and the included-range parser
probe. Its artifact is:

`/tmp/gts-c26q-hardening-artifacts/20260823T143124Z-c26q-hardening-sql-raw-ranges`

Its `container.log` SHA-256 is
`8d475defa727ff4e5c07647a2aadf68520c9e4892e8ab6efd024b76ced7042f2`.
Its `metadata.txt` SHA-256 is
`2b4fadadb9726a4d00d6cb32039daaebff9fd322e9151045c41fb5226a2677ac`.

The SQL real-corpus probe passed 25 of 25 no-error cases, 25 of 25
S-expression comparisons, and 25 of 25 deep comparisons. It saw 28 of 186
available samples. Maximum resident set size was 1,087,976 KiB. The run used
one CPU, 4 GiB, `GOMAXPROCS=1`, and `GOFLAGS=-p=1`.
The Docker artifact is:

`/tmp/gts-c26q-production-proof-20260823/harness_out/docker/20260823T142119Z-diag-sql`

Its `container.log` SHA-256 is
`c397c45908dfbe46bc8375d9f1e33fe44056d7b7ee9da70b78f1eb008e15acff`.
Its `metadata.txt` SHA-256 is
`995ff318234b4e2325904a24339b677ef9825787f8fd6eacacf5268654afc10a`.

### Generated SQL identity gate

The direct grammargen route now generates and reloads its exact blob through
`LoadLanguage`. SQL scanner adaptation then receives the generated blob hash.
The focused regression checks scanner and language identities for equality.
Missing or changed identities fail closed before checkpoint reuse.

This proof uses publication base
`d54147516440a91b8eda6983251c7cd6c4be2707` and the authenticated SQL source
under `/tmp/grammar_parity/sql`. The final focused Docker artifact is:

`/tmp/gts-c26r-artifacts/20260823T155352Z-c26r-sql-rebase-d5414751`

Its `container.log` SHA-256 is
`a70b0473a0a59f3cfcec52dfef99c9dc88e58a2c6fc8be952c717f7dc597fe7b`.
Its `metadata.txt` SHA-256 is
`1d86b47e74bd620424e46407ede358a9eef30d89afb27f1b5198e8e2f19b894e`.
The test logged scanner identity
`7e493677411a501e6d8592c6b9cc158e21a1bfed44c72ca914e2d81e4e34861d` and
generated blob identity
`4ffb2a6d09e2000126f10101db9028d28e0752ac3e4f83e401f045c3b028ca7c`.
The old parse recorded 13 checkpoint records, 4 checkpoint leaves, and 4
snapshots. The edited parse
reused 1 subtree and 16 bytes. Fresh and incremental trees matched.
The reference SQL scanner supplied stale grammar identity
`e21421cbab52b54cf5ba15c8f78a2bb4729bf4e8c0da14368069e897de451268`.
The stale checkpoint reused zero subtrees and zero bytes.

The full regression artifact recorded the existing dollar-quote divergence:

`/tmp/gts-c26r-artifacts/20260823T154425Z-c26r-sql-full-final`

Its `container.log` SHA-256 is
`7b77af8d87280b5e1de43b1234504ab7ec110f68385530d223a3562e41dbecd2`.
Its `metadata.txt` SHA-256 is
`ccede21b768050fcd6b08cfe8b4e735c3e584da63a85e7ff0bc1d818f773507e`.

The route remains blocked by four generated SQL locked-C divergences. Keep
generated SQL compact parity gated until those trees match node for node.

### Generated SQL override proof

The generated override tests use the checked-in SQL blob as input. They
change only the language name before gzip and gob encoding. The positive
override is 582,159 bytes with SHA-256
`2e484c002ffc7773fe02944901c5b2e87194932ca32025c0e3d9dac597540149`.
Its scanner receives that override hash, not the checked-in hash. Equal
override identities preserve incremental reuse and fresh-tree equality.

The drift test uses these two generated blobs:

- First: 582,165 bytes,
  SHA-256 `ec701f2ea2286db40bb0b6fa09957aab6f901962bfa5f7c103db6a0459ae2fb6`.
- Second: 582,165 bytes,
  SHA-256 `493e63f1a5645be6c6948244abb0be5654b624e98f9aed97fcff848e0fa4fe3f`.

The new language rejects the old tree after this drift. It reuses zero
subtrees and zero bytes. The fresh and incremental trees remain equal.
Adaptation without a target grammar identity keeps the scanner usable but
returns no checkpoint identity. The identity gate then fails closed.

The post-hardening SQL and override gate artifact is:

`/tmp/gts-c26q-hardening-artifacts/20260823T141319Z-c26q-hardening-sql`

Its `container.log` SHA-256 is
`2e125602ce5fb7ff33d8b9464b8edd37784ab46459cdebcd55485b72ec182cd5`.
Its `metadata.txt` SHA-256 is
`1d461d8740c49d8d18a79b5ef9ff5e922ca6891d1b7b9530ba56be652c29d241`.

The post-hardening race artifact covers the native scanner and generated
override tests. It used one CPU, 4 GiB, `GOMAXPROCS=1`, and `GOFLAGS=-p=1`.
Its artifact is:

`/tmp/gts-c26q-hardening-artifacts/20260823T141349Z-c26q-hardening-sql-race`

Its `container.log` SHA-256 is
`bd6d48591e98924cd49ba82725b5b864a98076d9553b69f2b808646f49adbed3`.
Its `metadata.txt` SHA-256 is
`915f4559d2a01d13a6fd99bd99187a7a764ff062281d0aee340ba31a5e35e847`.

### Generated SQL locked-C boundary

The generated SQL route is not the checked-in native SQL blob route. The
locked-C probe used 25 eligible samples. It reported 23 of 25 no-error
results, 21 of 25 tree matches, and four divergences.

The four known divergences are:

- `SELECT $$hey$$;`: generated SQL adds an `ERROR` at bytes 12 to 14.
- `SELECT $$(a + b)$$;`: generated SQL adds an `ERROR` at bytes 16 to 18.
- `CREATE DOMAIN test;`: generated `CREATE_DOMAIN` has one child; C has zero.
- `CREATE DOMAIN test AS text;`: generated `CREATE_DOMAIN` has one child; C has zero.

The checked-in SQL blob matches C for these witnesses. These four generated
grammar divergences remain outside the bounded native identity gate.

The post-hardening primary locked-C artifact is:

`/tmp/gts-c26q-production-proof-20260823/harness_out/grammargen_cparity/20260823_071725-c26q-hardening-sql-locked-c`

Its `container.log` SHA-256 is
`3da06f65285aa598d7b2f931a1b7e519314dcf7be9ac833e5b2977ae6d21e2c8`.
Its `metadata.txt` SHA-256 is
`7e28e8013623b9f3664621b1d34739f27393f14615af1eaf50db18118c947bad`.

### Post-hardening validation

The final hardening reran the root, SQL, race, incremental, route,
real-corpus, and locked-C gates. Each run used one container and one CPU.
No run reported an out-of-memory kill or a wall timeout.

The final root gate artifact is:

`/tmp/gts-c26q-hardening-artifacts/20260823T143340Z-c26q-hardening-root-final`

Its `container.log` SHA-256 is
`9cf08c045266c069239c06b10a6e134fe7e6eb7c797d9804aa35ec5bd5aa77f9`.
Its `metadata.txt` SHA-256 is
`1ce71c4845672333cf1fdcf2b2291d3ed0e4a8c703ac27a227ecf395d6dd9ddf`.

The root race artifact is:

`/tmp/gts-c26q-hardening-artifacts/20260823T141242Z-c26q-hardening-root-race`

Its `container.log` SHA-256 is
`cb69e6ec92349fa703a93e0fa45dccdfe087b7b838412cc23eed17b8f3daca6a`.
Its `metadata.txt` SHA-256 is
`8fcf380350708abaf4337d3d5b9e952da3128c67a807351e88dd9d4bb5e86d39`.

The SQL race, incremental, route, and diagnostic screen artifacts are:

- SQL race: `/tmp/gts-c26q-hardening-artifacts/20260823T141349Z-c26q-hardening-sql-race`.
- Incremental: `/tmp/gts-c26q-hardening-artifacts/20260823T141430Z-c26q-hardening-sql-incremental`.
- Routes: `/tmp/gts-c26q-hardening-artifacts/20260823T141907Z-c26q-hardening-sql-route`.
- Diagnostic screen, candidate: `/tmp/gts-c26q-hardening-artifacts/20260823T142008Z-c26q-hardening-sql-perf-candidate`.
- Diagnostic screen, base: `/tmp/gts-c26q-hardening-artifacts/20260823T142021Z-c26q-hardening-sql-perf-base`.

The diagnostic screen is not performance evidence. It used a temporary
benchmark and one sample. Treat the accounting change as correctness-only.

The real-corpus artifact is:

`/tmp/gts-c26q-production-proof-20260823/harness_out/docker/20260823T142119Z-diag-sql`

The locked-C artifact is:

`/tmp/gts-c26q-production-proof-20260823/harness_out/grammargen_cparity/20260823_071725-c26q-hardening-sql-locked-c`

The SQL race metadata records `GOMAXPROCS=1` and `GOFLAGS=-p=1`.
The route metadata records `GOMEMLIMIT=3GiB` and `GOFLAGS=-p=1`.
The real-corpus metadata records `GOMAXPROCS=1` and `GOFLAGS=-p=1`.

### Focused Docker gates and scope

The root identity tests passed. The root race tests passed. SQL scanner,
override, incremental, raw, included-ranges, real-corpus, and locked-C gates
also passed their process exit checks. No run reported an out-of-memory kill
or wall timeout. The focused runs used one CPU and 4 GiB. SQL race and
locked-C runs set `GOMAXPROCS=1` and `GOFLAGS=-p=1`. The real-corpus metadata
records `GOMAXPROCS=1` and `GOFLAGS=-p=1`.

The proof excludes Swift, other grammar changes, GLR scheduler changes,
recovery election changes, condense or merge changes, forest ownership
changes, and performance claims. It does not close the generated SQL parity
gap.

The code and test patch SHA-256 against the publication base is
`cdc8d0d83204a7ea613442cafa461d961f27b81d3ad7fd3e3e85aebe3906ee48`.
The patch contains these files:

- `arena.go`
- `external_scanner_checkpoint_capability.go`
- `external_scanner_checkpoint_identity_test.go`
- `external_scanner_checkpoints.go`
- `grammars/embedded_loader.go`
- `grammars/grammargen_blob_override.go`
- `grammars/grammargen_blob_override_test.go`
- `grammars/sql_scanner.go`
- `grammars/sql_scanner_test.go`
- `language.go`
- `load_language.go`
- `parser.go`
- `tree.go`

The receipt adds only `CHANGELOG.md` and this document. No C26q document
guard test exists. Therefore, no two-stage document-guard chain is needed.
The diagnostic C parity log change was restored before this receipt.

Several commands were quarantined. The root command `go test ./cgo_harness ...`
was invalid because `cgo_harness` is a separate module. A later command used
`cd /workspace/cgo_harness && go test . ...`. A trial passed the unsupported
`--gomaxprocs` option to `run_parity_in_docker.sh`; the wrapper printed usage.
Later probes passed unsupported `--out-root` and `--src-dir` options, and one
sent SQL to a wrapper that excludes SQL. Those probes produced no test
evidence. The final direct runner commands above are the evidence.

### C26q decision and reopening condition

GO applies only to the bounded native SQL identity gate and generated override
fail-closed behavior. This gate does not prove generated SQL parity with C.
Keep generated SQL compact parity gated.

Reopen the generated SQL path only after all of the following conditions hold:

- Explain and fix the four generated SQL locked-C divergences.
- Prove generated source and grammar identities from an authenticated lock.
- Pass equal-identity reuse and changed-identity rejection on generated SQL.
- Pass raw, production, compact, forest, included-ranges, and incremental
  parity for native and generated SQL.
- Avoid source-hash, blob-exception, witness-repair, and language-specific
  policy.

Keep the generated SQL route gated until it passes the full locked-C proof.

## Current bounded result

The bounded matrix completed with no silent divergence.

| Status | Files |
|---|---:|
| PASS | 63 |
| FALLBACK | 26 |
| SKIP | 8 |
| DIVERGE | 0 |
| ERROR | 0 |
| Total | 97 |

The compact route now admits no-primary multi-derivation end-of-file (EOF)
frontiers through a bounded materiality comparison. It selects a deterministic
provisional candidate only for that comparison. It requires exact public-tree
equality for every live candidate and caps the comparison at eight candidates.
The route fails closed when the cap, context, materialization, or equality
proof fails.

The C# 642-byte `variableDeclarations.cs` row now routes. Production, compact,
and locked-C report the exact deep digest
`005b39bd9a68ff9775129d3fb793b9d7a58b9f56812bb9ca9bd0eb753465dd86`.
The C# collapsed-token regression fixture and three Dart constructor probes
(class, private, and enum) route without a dispatch pass and with exact
locked-C trees. These focused probes are outside the matrix and do not change
its counts.
The change grants no language profile or digest grant.

The 97-row matrix moves from 62 PASS, 27 FALLBACK, and 8 SKIP to 63 PASS,
26 FALLBACK, and 8 SKIP. It has zero DIVERGE and ERROR rows.

The corpus manifest contains 149 verified files across 50 languages.
Its SHA-256 digest is
`14e811c4c278570e795a4a79f387dd15c61ff20718e3430f2091fe386e35c92b`.
This run selected files at or below 16,383 bytes and excluded AWK.
The AWK medium file needs a separate slow-path budget.

The selected HTML rows are:

- `small__index.html`, 258 bytes, source SHA-256
  `d6255f90847aca0286078542cd8ab6a9da0687069222bb868ab0315b05396a86`.
- `medium__a00355.html`, 9,218 bytes, source SHA-256
  `c166899fb1aeceb0a3967731a6a37cad628133f82fb17bb3c282b4afe7bf85bd`.

The compact route now collects raw terminal spans and compact subtree
identities during authenticated materialization. It preserves hidden terminal
bytes and exact public `ERROR` provenance. Caller-owned scratch bounds retained
storage. Bounded polling preserves cancellation response. The change uses no
language-specific rule.

The direct route served 64 percent of the selected files.
Production served every fallback and every ineligible file.

The C# `small__variableDeclarations.cs` row changed from fallback to direct
admission. It has 642 bytes and the exact production, compact, and locked-C
deep digest recorded above.

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
GTS_ADMISSION_REAL_CORPUS_EXCLUDE_LANGS=awk \
GTS_ADMISSION_REAL_CORPUS_MAX_BYTES=16383 \
GOMAXPROCS=1 go test . \
  -tags gts_parsercorephase0 \
  -run '^TestAdmissionCandidateRealCorpusMatrix$' \
  -count=1 \
  -v
```

The test reads `cgo_harness/corpus_real/manifest.json` by default.
Use `GTS_ADMISSION_REAL_CORPUS_MANIFEST` to select another manifest.
This command reproduces the current 97-row bounded result.
Ratchet mode applies only to the historical 110-row receipt.

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

## C26y SQL compact checkpoint trace

C26y used base
`e24ccf5a87bbd7febc21f67f014c2d5301d229d0`.

Status: NO-GO.

Restore the pinned source with:

```sh
bash scripts/seed_real_corpus_from_lock.sh /tmp/gts-c26y-pinned-grammar-20260824 sql
```

The source used commit
`587f30d184b058450be2a2330878210c5f33b3f9`.

- `src/grammar.json`: `42f011860137175a5a0cb820d1a694e5ccca1d17f226729ff6a4e886910cde1c`
- `src/scanner.cc`: `d437ad9f517d7a1f4248ccd05abe58370b5040c0037c877dab1f0aefeaa04af6`

The direct production and locked-C parity artifact is
`/tmp/gts-c26y-artifacts-prod2/20260823T174027Z-c26y-production-direct-trace`.
Its log SHA-256 is
`9db58d347835f6c2fede671ebce47c89c840f8651bed93fb109c6be24ea3cbc5`.
Its metadata SHA-256 is
`17b8ca563d6e41000490371aff422500fea4857d9551e9bdffc458e0b9e2b012`.

The direct generated route recorded 13 records, 4 leaves, and 4 snapshots,
but it returned an error tree. The locked blob returned the correct tree and
recorded 14 records, 5 leaves, and 5 snapshots. The locked C comparison in
the same parity suite matched the locked blob tree. The C binding does not
expose scanner serialization bytes.

The target-symbol wrapper production artifact is
`/tmp/gts-c26y-artifacts-wrapperprod2/20260823T174239Z-c26y-wrapper-production-trace2`.
Its log SHA-256 is
`87658133020bc5232ce5e2b67b12fb61b6d253a39f3af56137894ef937b575a0`.
Its metadata SHA-256 is
`5fe266fdab999e4c2b825ff6ed81e54c4503c487a4779595cc567f95b36e0207`.
The wrapper returned the correct tree. It recorded 14 records, 5 leaves, and
5 snapshots. Its target symbols differed, but its scanner bytes matched the
locked blob at every checkpoint boundary.

The successful compact-admission artifact is
`/tmp/gts-c26y-artifacts-compact2/20260823T174539Z-c26y-compact-wrapper-trace2`.
Its log SHA-256 is
`81ff5dee3f401dd15e11b0bc3a6bee5f15e86aed18c8e177c015af5228a7ba09`.
Its metadata SHA-256 is
`4a6329873e021ad0db5a4d28c968971e629669a58fee81baccf8d5e0e1314563`.
The compact scheduler captured the same four current checkpoint pairs:

- `7-9`: `00` to `242400`
- `9-12`: `242400` to `242400`
- `12-14`: `242400` to `00`
- `14-15`: `00` to `00`

The compact tree returned zero records, zero leaves, and zero snapshots. Its
incremental probe reused zero subtrees and zero bytes. The route therefore
has a semantic scanner trace but no tree-owned sidecar proof. Do not enable
checkpointed compact reuse from this trace alone.

The 13/4/4 values are path-specific. They come from the direct generated
route's failed external-token mapping and error recovery. The invariant for a
correct SQL route is the four byte-span and serialized-state pairs above,
plus a complete tree with no error. A future candidate must materialize these
sidecars, preserve scanner and grammar identity, and prove nonzero reuse.
Keep SQL compact admission gated. Do not require the path-specific 13/4/4
receipt for a semantically correct route.

## C26z SQL compact checkpoint sidecars

C26z used base
`f8b9d718ee19f65598e274035f5481a899ab2b72`.

Status: NO-GO.

The diagnostic candidate attached the scheduler's authenticated checkpoint
pairs to compact terminal nodes before parent construction. It also bound
adapted scanners to the target grammar blob identity. Duplicate spans with
different states failed closed. The candidate was reverted after the C26aa
screen because compact replay did not prove production-equivalent reuse.

The pinned SQL source remains commit
`587f30d184b058450be2a2330878210c5f33b3f9` with grammar hash
`42f011860137175a5a0cb820d1a694e5ccca1d17f226729ff6a4e886910cde1c` and
scanner hash
`d437ad9f517d7a1f4248ccd05abe58370b5040c0037c877dab1f0aefeaa04af6`.

The generated SQL witness records 14 checkpoint records, 5 leaf nodes, and
5 snapshot payloads. It uses these scanner transitions:

- `7-9`: `00` to `242400`
- `9-12`: `242400` to `242400`
- `12-14`: `242400` to `00`
- `14-15`: `00` to `00`

The generated tree matches the locked blob and locked C trees for all three
focused inputs. Incremental parsing reuses 1 subtree and 6 bytes. A stale
grammar identity reuses 0 subtrees and 0 bytes. The focused Docker run passed:

```text
/tmp/gts-c26z-artifacts/20260823T181038Z-c26z-final-gate
```

The container log SHA-256 is
`1bb984c67b55adaaf6ded853f31473c0eeb38bc844c9c046af6ba2b9b2c99c4c`.
The metadata SHA-256 is
`d866055e83db3d9e17b45c5007b6acc2d46cca28bed28363d7951ce18a159e8d`.

The unit gate passed in
`/tmp/gts-c26z-artifacts/20260823T181230Z-c26z-unit-final`, and the grammar
binding gate passed in
`/tmp/gts-c26z-artifacts/20260823T181309Z-c26z-grammars-final`.
Their container log and metadata hashes are
`2bea71d9e86e304da3da8c7e4b04c16549c0d867408423bd79ca4f0e75d8fda6` and
`a89df6d2adb725d9cfefb55b04dc6d8b6f338155c142fa8d7d51f8bf05829215`, and
`e24f45dcc56d9960e364a9288f64731806b5a9156da9ad6196c30fe0f6589424` and
`8acd3e1d407a44fea29389ba25097d2a14191edd5fe494dfc482e93e549884ae`.

The diagnostic compact route had tree-owned sidecars and safe nonzero reuse.
The observed 6-byte reuse was lower than the earlier 16-byte production
receipt. C26aa traced the difference to parse-state replay, not span
selection. The generated compact content leaf covered bytes `9-12` with
replay state `180`; checkpoint scanning then returned an error span `9-14`.
The locked production leaf used state `16613` and returned the expected
`9-12` span. No generic state remap passed the equality and stale-identity
guards. The production and test changes were reverted. Keep SQL compact
admission gated until a generic replay contract proves production-equivalent
reuse.

## C26ad SQL compact replay state provenance

C26ad used evidence base
`cf58fba517ed4fa6a8f5d1328ac2f850d48a8c75`.
This receipt is rebased onto publication base
`515df769b9b4e2f8e3ea715e78b75a44faa3b6d6` after PR #868 merged.

Status: NO-GO. Keep SQL compact replay gated.

The pinned SQL source remains commit
`587f30d184b058450be2a2330878210c5f33b3f9`.

- `src/grammar.json`: `42f011860137175a5a0cb820d1a694e5ccca1d17f226729ff6a4e886910cde1c`
- `src/scanner.cc`: `d437ad9f517d7a1f4248ccd05abe58370b5040c0037c877dab1f0aefeaa04af6`

The source was mounted read-only from
`/tmp/gts-c26ad-grammar_parity`.

### Clean baseline

Run the focused generated SQL parity test with this command:

```sh
bash cgo_harness/docker/run_parity_in_docker.sh \
  --repo-root /tmp/gts-c26ad-sql-state-provenance-20260824 \
  --out-root /tmp/gts-c26ad-artifacts \
  --label c26ad-clean-baseline --no-build --memory 4g --cpus 1 \
  --goflags -p=1 --test-parallel 1 --timeout 10m \
  --mount /tmp/gts-c26ad-grammar_parity:/tmp/grammar_parity:ro -- \
  'export PATH=/usr/local/go/bin:$PATH; cd /workspace/cgo_harness && \
   go test -tags "cgo treesitter_c_parity" . \
   -run "^TestSQLGrammargenCGORegressionCases$" -count=1 -v'
```

The artifact is
`/tmp/gts-c26ad-artifacts/20260823T201241Z-c26ad-clean-baseline`.

- `container.log`: `d00a60bdabbcb837f69972c60020cd0fa77816ec1737957941d0b30cc79e8441`
- `metadata.txt`: `54dddcb902722f22553947592c221c0fb9a1c5fae9e3b7d0f0c5f3b135c4bc42`
- `inspect.json`: `98deafbaf286b9cb219d17517cde873c5e18913b3f6829376edfd68a303bb90e`

The run used one central processing unit, 4 GiB, `GOMEMLIMIT=6GiB`,
`GOFLAGS=-p=1`, and test parallelism one. It failed only the generated
dollar-quoted witness. The run passed the identifier and parenthesized
witnesses. The generated tree has an `ERROR` root child, while the locked blob
and locked C trees match.

The generated route recorded 13 old checkpoints, 4 old leaves, 4 old snapshots,
1 reused subtree, and 16 reused bytes. The stale grammar route recorded zero
reused subtrees and zero bytes. These counters do not certify the failed
dollar-quoted route.

### State provenance

Generation copies normalized external symbol IDs into `Language.ExternalSymbols`
(`grammargen/assemble.go:179-188`). The blob encoder serializes that language
(`grammargen/encode.go:102-106`). `LoadLanguage` restores the language tables
and records the exact blob hash (`load_language.go:34-59`).

The embedded loader binds a registered scanner to the target language before
it falls back to external-order adaptation (`grammars/embedded_loader.go:437-499`).
The SQL scanner currently binds grammar identity only. Its `Scan` method reads
symbols from the reference `SqlLanguage()` (`grammars/sql_scanner.go:74-86`,
`grammars/sql_scanner.go:155-160`).

The narrow SQL source repair is to bind the target language's external symbols
inside `ExternalScannerForLanguage` and use them in `Scan`. The temporary probe
used that repair and passed the generated production and compact witnesses.
This repair is SQL-specific. It does not preserve parser-state identity across
different language tables, so C26ad reverted it and did not ship it.

The generated and locked identities are:

| Route | Grammar or blob identity | External symbols | Content state | Scanner span |
| --- | --- | --- | --- | --- |
| Generated production | `4ffb2a6d09e2000126f10101db9028d28e0752ac3e4f83e401f045c3b028ca7c` | `[286,287,288]` | `pre=180`, `parse=449` | `[9,12)` |
| Generated compact, target symbols | `4ffb2a6d09e2000126f10101db9028d28e0752ac3e4f83e401f045c3b028ca7c` | `[286,287,288]` | `pre=180`, `parse=449` | `[9,12)` |
| Locked production | `e21421cbab52b54cf5ba15c8f78a2bb4729bf4e8c0da14368069e897de451268` | `[285,286,287]` | `pre=16613`, `parse=16652` | `[9,12)` |

The generated scanner identity is
`7e493677411a501e6d8592c6b9cc158e21a1bfed44c72ca914e2d81e4e34861d`.
The locked identity uses the same scanner implementation and a different
grammar blob identity.

Temporary target-symbol binding produced the generated rows above. Its
diagnostic artifact is
`/tmp/gts-c26ad-artifacts/20260823T200735Z-temp-state-three-languages`.
It passed its temporary probe. Its container log, metadata, and inspection
hashes are:

- `container.log`: `ed3141558aaf211d66608ed3ad08122177c51277ad5ccfb9b8ce5f6400e3f8a6`
- `metadata.txt`: `cd6cecaa55441901200004709e2918cbc2a9d6b46615d8a786045979ef7ed91c`
- `inspect.json`: `20fe89ad5fc3f5a82802b5983e23e699150dfebf0565083eddab234058c3204c`

The target-symbol scan at state `180` used generated content symbol `287` and
returned action `309` with span `[9,12)`. The compact route reconstructed the
same generated states and returned the correct tree. The temporary files and
test changes were removed after this probe.

The earlier C26aa mixed-table witness remains at
`/tmp/gts-c26z-artifacts/20260823T182021Z-c26aa-state-debug`.
Its container log hash is
`ac3f991ed44055fd93a1fb3a689adcd21a40eabd0937546fa6e1981001974737`.
It shows generated state `180` scanning `[9,14)` and locked state `16613`
scanning `[9,12)`. Its generated probe also reports `want_sym=277`, which is
not the generated symbol table in the target-symbol probe. This confirms that
the C26aa comparison crossed scanner or language-table identity.

### Replay boundary

Compact records retain symbol, production, span, child, alias, and external
flags. They retain sparse scanner checkpoint references for authenticated
external terminals (`internal/parsercorephase0/core.go:691-731`,
`internal/parsercorephase0/core.go:5115-5133`).

`MaterializationSubtreeView` does not expose parser states or checkpoint
references (`internal/parsercorephase0/core.go:822-841`). The materializer
replays each compact derivation against the parser's current language table and
stamps the resulting states (`parsercore_phase0_driver.go:4786-4828`).
`replayTransition` selects that current table's shift or goto action
(`parsestate_replay.go:109-126`). Production leaves receive their states from
the current shift action (`parser_reduce.go:2542-2582`).

The incremental leaf path restores `preGotoState`, reauthenticates the scanner,
and requires an exact token symbol and span (`incremental_leaf_fastpath.go:60-86`,
`incremental_leaf_fastpath.go:848-860`).

The reuse cursor walks old nodes in source order. `tryReuseSubtree` checks the
current parser state, the stored node state, and the scanner checkpoint gate
before it reuses a node (`incremental.go:10-85`, `incremental.go:609-657`).
`reuseTargetState` looks up the current language table and rejects a leaf when
its stored parse state does not match a current shift (`incremental.go:1028-1065`).

State `180` and state `16613` therefore have local meaning. State `180` proves
the generated table transition. State `16613` proves the locked table
transition. A numeric remap cannot prove equal shift, goto, reduction, hidden
derivation, or scanner-validity behavior. Span equality and scanner identity do
not add that proof. The compact record has no generic cross-table state map.

The current origin route remains fail-closed when compact scanner sidecars are
absent. The temporary generated compact route had no runtime parse work and
reconstructed the generated states only after target-symbol binding. No generic
state remap passed the equality and stale-identity guards. The C26z candidate
was reverted. No code or test change survives C26ad.

The focused scanner unit gate passed:

```sh
go test ./grammars -run '^TestSQLScanner' -count=1
```

### C26ad decision and reopening condition

Reject the generic state-remap fix. Keep compact replay disabled for this
scanner route.

Reopen only after a replay contract binds all of these values to one language
identity:

- parser action and goto tables;
- external symbol order and scanner identity;
- scanner checkpoint bytes and source spans;
- compact derivation identity, including hidden symbols and aliases; and
- a parity test that proves fresh, compact, incremental, and locked-C trees.

The contract must reject a missing or changed identity before it attempts
scanner replay. A numeric state translation alone is not sufficient.

## C26ag generic compact table identity guard

C26ag used publication base
`ae6e49448be249dd52dca5a95ba187fdd3000fe6`.
The isolated worktree was `/tmp/gts-c26ag-table-identity-20260824`.

Status: ACCEPTED guard. Keep the SQL compact route gated for the remaining
parity gap.

The guard adds a dependency-neutral `TableIdentityProvider` contract at
`internal/parsercorephase0/core.go:277-282`.
`Core` stores the producer identity at construction and compares it before
parser-state replay (`internal/parsercorephase0/core.go:898-900,1690-1708`).
The root adapter supplies the identity from its current `Language`
(`parsercore_phase0_driver.go:552-562`).
Replay returns an identity decline before it allocates or walks replay states
when the identity differs (`parsestate_replay_compact.go:145-155`).

Loaded languages use their exact compressed grammar blob SHA-256.
In-memory generated languages use one process-local producer token.
The contract does not translate numeric parser states.

The existing reuse cursor requires the same `*Language` pointer
(`parser.go:3450-3452`). Scanner checkpoint reuse requires scanner and grammar
identity (`external_scanner_checkpoint_capability.go:153-159`). These gates
remain unchanged. The new core guard covers the earlier replay boundary.

### Identity evidence

The focused tests cover matching identity, producer drift, and missing identity.
The blob test proves that two loaded copies use the same exact blob SHA-256.
The language-swap test proves that replay returns
`DiagnosticParserCoreIdentity` before state reconstruction.

The test files are:

- `internal/parsercorephase0/core_test.go`
- `parsercore_phase0_action_test.go`
- `parsercore_phase0_language_tables_internal_test.go`

### Docker validation

The SQL scanner unit gate ran in the combined focused command below.

Run the generated SQL, locked-C, fresh-tree, incremental-reuse, and
stale-identity gate:

```sh
bash cgo_harness/docker/run_parity_in_docker.sh \
  --repo-root /tmp/gts-c26ag-table-identity-20260824 \
  --out-root /tmp/gts-c26ag-ae6e-artifacts \
  --label c26ag-sql-generated-locked-c --no-build --memory 4g --cpus 1 \
  --goflags -p=1 --test-parallel 1 --timeout 10m \
  --mount /tmp/gts-c26ad-grammar_parity:/tmp/grammar_parity:ro -- \
  'export PATH=/usr/local/go/bin:$PATH; cd /workspace/cgo_harness && \
   go test -tags "cgo treesitter_c_parity" . \
   -run "^TestSQLGrammargenCGORegressionCases$" -count=1 -v'
```

Artifact:
`/tmp/gts-c26ag-ae6e-artifacts/20260824T010956Z-c26ag-ae6e-sql-generated-locked-c`.

- `container.log`: `34f6ef038f16fa22dfb0ac9edb7633e889b76a231c317f5788a4c7f7a3d03ed9`
- `metadata.txt`: `05132d52f87ef1fcf24d9759d2a97653764faa334db10350162df9524c907b01`
- `inspect.json`: `c61c48886aa2f6ea56af080fc5b742d464689b623dae4497fe946a2c14afc05e`

The generated identity matched the generated blob:
`4ffb2a6d09e2000126f10101db9028d28e0752ac3e4f83e401f045c3b028ca7c`.
The route reused one subtree and 16 bytes.
The stale identity route reused zero subtrees and zero bytes.
The identifier and parenthesized Boolean cases passed.
The dollar-quoted case failed with the known generated-vs-locked-C error
tree difference. The guard did not change that result.

Run the focused identity tests:

```sh
bash cgo_harness/docker/run_parity_in_docker.sh \
  --repo-root /tmp/gts-c26ag-table-identity-20260824 \
  --out-root /tmp/gts-c26ag-ae6e-artifacts \
  --label c26ag-ae6e-focused --no-build --memory 4g --cpus 1 \
  --goflags -p=1 --test-parallel 1 --timeout 10m -- \
  'export PATH=/usr/local/go/bin:$PATH; cd /workspace && \
   go test ./internal/parsercorephase0 \
   -run "^TestCoreTableIdentityCapturesAndRejectsProducerDrift$" -count=1 && \
   go test -tags gts_parsercorephase0 . \
   -run "^TestParserCore(ReplayRejectsLanguageTableSwap|RootTablesIdentityUsesLanguageBlobHash)$" -count=1 && \
   go test -race -tags gts_parsercorephase0 . \
   -run "^TestParserCoreReplayRejectsLanguageTableSwap$" -count=1 && \
   go test -tags gts_no_parsercorephase0 . -run "^$" -count=1'
```

Artifact:
`/tmp/gts-c26ag-ae6e-artifacts/20260824T010926Z-c26ag-ae6e-focused`.

- `container.log`: `2c5455352c780dfd865d2c38c112a7fd44c11912094ffe7abcd909a291121002`
- `metadata.txt`: `decaddbe79940108b3f7d14d2e31f942a364a78f7376fbb936482bf586719a39`
- `inspect.json`: `4832627711918dda32dd69b40c38bcf8f34b3a09a3571624c0c2084ac1fbeaf9`

The focused core and root tests passed.

The loaded-blob identity, race, and compile-out tests ran in the combined
focused command above. They passed in this artifact:
`/tmp/gts-c26ag-ae6e-artifacts/20260824T010926Z-c26ag-ae6e-focused`.

### C26ag decision and reopening condition

Accept the generic table-identity guard. Do not add SQL-specific symbol maps.
Keep the SQL compact route gated until generated SQL matches locked C on the
dollar-quoted witness and the replay contract covers all derivation metadata.

Reopen the SQL route only after a Docker gate proves all of these conditions:

- equal table identity permits replay;
- changed or missing identity declines before replay;
- scanner identity and checkpoint bytes remain exact;
- fresh, compact, incremental, and locked-C trees match; and
- the generated SQL dollar-quoted witness passes without a state remap.

## C26ah generated SQL dollar-quote producer blocker

C26ah used publication base
`d72987b44b76cf39aa4ad0f5fff03860eed7cd0d`.
The isolated worktree was `/tmp/gotreesitter-compact-next.oFLjU5`.

Status: **NO-GO / KEEP LIVE**. Keep SQL compact admission gated.
No parser, scanner, grammar, or compact admission change ships.
The receipt adds one focused probe, one changelog entry, and this section.

### Producer and witness

The pinned SQL producer is `m-novikov/tree-sitter-sql` at commit
`587f30d184b058450be2a2330878210c5f33b3f9`.
The source hashes are:

- `src/grammar.json`: `42f011860137175a5a0cb820d1a694e5ccca1d17f226729ff6a4e886910cde1c`.
- `src/scanner.cc`: `d437ad9f517d7a1f4248ccd05abe58370b5040c0037c877dab1f0aefeaa04af6`.

The generated language uses blob identity
`4ffb2a6d09e2000126f10101db9028d28e0752ac3e4f83e401f045c3b028ca7c`.
Its scanner identity is
`7e493677411a501e6d8592c6b9cc158e21a1bfed44c72ca914e2d81e4e34861d`.
The checked-in SQL blob identity is
`e21421cbab52b54cf5ba15c8f78a2bb4729bf4e8c0da14368069e897de451268`.

The witness is the 16-byte source `SELECT $$hey$$;\n`.
Its source SHA-256 is
`c65f30545110c37897fb7fe364af31ff572b35796963ec8fc59b37b76d319912`.
The generated tree digest is
`7ae91c65aebee3a10eeef68804af8b74bbf27620b200dfe842916898fc90dd46`.
The checked-in tree digest is
`f093882f4f27897036dd245c3e17f1dad2d7cd72e470e8594b5492150a2c451e`.
The locked-C tree digest is the same checked-in digest.

The first difference is `/source_file` child count.
The generated tree has three children. The checked-in and locked-C trees have
two children. The generated tree has an error. The other two trees are clean.
This difference starts in the generated producer output, before compact replay.

### Compact route evidence

The focused probe checks the generated identity before it parses the witness.
It checks the checked-in tree against the locked-C tree.
It also checks the generated tree against both references.
The probe is `cgo_harness/sql_c26ah_generated_dollar_quote_blocker_test.go`.

The compact candidate recorded `routed=0` and `fallback=1`.
Its exact fallback reason was:

```text
compact route declined at recovery [mechanism=recovery-entered]: did not accept EOF: generic scheduler has no table action for the elected token
```

The compact fallback tree equals the generated production tree.
It does not claim compact parity. It preserves the current fail-closed route.

### Focused Docker gate

Run one SQL grammar with one CPU and one Go test worker:

```sh
GOMAXPROCS=1 bash cgo_harness/docker/run_parity_in_docker.sh \
  --repo-root /tmp/gotreesitter-compact-next.oFLjU5 \
  --out-root /tmp/gotreesitter-c26ah-artifacts \
  --label c26ah-sql-blocker-gomax1 --no-build --memory 4g --cpus 1 \
  --gomemlimit 3GiB --goflags -p=1 --test-parallel 1 --timeout 10m \
  --mount /tmp/gotreesitter-sql-seed.ada3CT:/tmp/grammar_parity:ro -- \
  'cd /workspace/cgo_harness && \
   go test -tags "cgo treesitter_c_parity" . \
   -run "^TestSQLC26ahGeneratedDollarQuoteBlocker$" -count=1 \
   -parallel=1 -timeout=10m -v'
```

The run passed with exit code zero.
It used one SQL grammar, one CPU, `GOMAXPROCS=1`, and `GOFLAGS=-p=1`.
It reported no out-of-memory kill and no wall timeout.

The artifact is
`/tmp/gotreesitter-c26ah-artifacts/20260824T015950Z-c26ah-sql-blocker-final2`.

- `container.log`: `40e12556fdbf0c806e1d54b71e59a0566f8e6eab09f62d9ceeb37742f7ce4d88`.
- `metadata.txt`: `cd0c53681a3ae278e5ad6d2b26609952a868efa0550715803c9ed5dfe49eb01c`.
- `inspect.json`: `f61f7fe393588d334610829acee2d3cfa34d892f7ed098c0be2f02c96e4f908b`.

The existing strict grammargen regression still fails on this witness.
The failure is the expected generated-versus-locked-C producer difference.
The new receipt makes that blocker and the compact fallback explicit.

### Rebase applicability

Remote main added the Perl dispatcher receipt and related documentation after
the original C26ah base.
It did not change the SQL grammar lock, SQL blob, SQL scanner, grammargen
inputs, parser core, or SQL parity test.
The current hashes match the `6eed698a13e7371fa978adb893e8b89ad1cd81ba`
hashes for every SQL executable input.
The original focused artifact therefore remains applicable after this rebase.
No Docker rerun was required.

### C26ah decision and reopening condition

Keep the SQL route live and gated. Do not add a parser-state remap.
Do not add a SQL-specific symbol map.

Reopen only after a producer or grammar revision makes the generated tree
match the checked-in and locked-C trees for this witness.
Then rerun the strict grammargen parity test and the compact route gate.
Require equal table identity, scanner identity, fresh-tree equality, compact
equality, incremental equality, and locked-C equality.

No performance gate ran for C26ah. This receipt makes no performance claim.

## C26ai generated SQL tagged dollar-quote producer witness

C26ai used publication base
`7a14a701eb2a5d623ce128e792ee67820a734c8b`.
It follows the C26ah untagged dollar-quote receipt.

Status: **NO-GO / KEEP LIVE**. Keep SQL compact admission gated.
This slice adds one tagged producer witness, one focused Docker probe, one
changelog entry, and this section. It adds no parser or route change.

### Producer and witness

The pinned SQL producer is `m-novikov/tree-sitter-sql` at commit
`587f30d184b058450be2a2330878210c5f33b3f9`.
The grammar source SHA-256 is
`42f011860137175a5a0cb820d1a694e5ccca1d17f226729ff6a4e886910cde1c`.
The scanner source SHA-256 is
`d437ad9f517d7a1f4248ccd05abe58370b5040c0037c877dab1f0aefeaa04af6`.

The generated blob identity is
`4ffb2a6d09e2000126f10101db9028d28e0752ac3e4f83e401f045c3b028ca7c`.
The scanner identity is
`7e493677411a501e6d8592c6b9cc158e21a1bfed44c72ca914e2d81e4e34861d`.
The checked-in SQL blob identity is
`e21421cbab52b54cf5ba15c8f78a2bb4729bf4e8c0da14368069e897de451268`.

The witness is the 22-byte source `SELECT $tag$hey$tag$;\n`.
Its source SHA-256 is
`1279d93f715690fee6c8af53fa774d0108c19846d9418d86d53edec0d743bc88`.
The generated tree digest is
`ba05964c2f2c62e56a3ee9470c76dc55098f7c2ea4f656607da30c5f8af212d4`.
The checked-in and locked-C tree digest is
`824c8bdf7107be3632bd0a43d89e0324ebc0802dcd8c4dca014130224f930ef6`.

The generated tree has three root children and an error.
The checked-in and locked-C trees have two root children and no error.
The first divergence is:

```text
root: ChildCount go=3 c=2 (goType="source_file" cType="source_file" goBytes=[0-22] cBytes=[0-22])
```

The difference starts in generated producer output, before compact replay.
The tagged witness extends the same producer boundary shown by C26ah.

### Focused Docker gate

The probe is
`cgo_harness/sql_c26ai_generated_tagged_dollar_quote_test.go`.
It checks generated identity, checked-in parity, and locked-C parity.
Run one SQL grammar with one CPU and one Go test worker:

```sh
GOMAXPROCS=1 bash cgo_harness/docker/run_parity_in_docker.sh \
  --repo-root /tmp/gotreesitter-graduation-next.VP39zH \
  --out-root /tmp/gotreesitter-c26ai-artifacts \
  --label c26ai-sql-tagged-dollar-quote-final --no-build --memory 4g --cpus 1 \
  --gomemlimit 3GiB --goflags -p=1 --test-parallel 1 --timeout 10m \
  --mount /tmp/gotreesitter-sql-seed.ada3CT:/tmp/grammar_parity:ro -- \
  'cd /workspace/cgo_harness && \
   go test -tags "cgo treesitter_c_parity" . \
   -run "^TestSQLC26aiGeneratedTaggedDollarQuote$" -count=1 \
   -parallel=1 -timeout=10m -v'
```

The run passed with exit code zero.
It used one SQL grammar, one CPU, `GOMAXPROCS=1`, and `GOFLAGS=-p=1`.
It reported no out-of-memory kill and no wall timeout.

The artifact is
`/tmp/gotreesitter-c26ai-artifacts/20260824T024111Z-c26ai-sql-tagged-dollar-quote-final`.

- `container.log`: `94ebf803c12c2381c67467de3d4041c61b2513e420335983a22bdab74ebd5fce`.
- `metadata.txt`: `496d6ff1881e87f6c09f571f9995cdf6aee25d9260c7581fae1c1087dece92d3`.
- `inspect.json`: `69210f60086dd2c2510c374d6bc928b7ce90206d523f0ce248895ba806583978`.

The generated tree remains different from locked C.
The probe records a producer blocker, not a compact parity claim.

### Rebase applicability

The C26ah receipt and the intervening Julia, highlighting, and recovery
receipts are present on this base.
The SQL grammar lock, SQL blob, SQL scanner, parser, and grammargen inputs
did not change between `d72987b44b76cf39aa4ad0f5fff03860eed7cd0d` and this base.
The C26ai artifact therefore remains applicable after this rebase.
No Docker rerun was required.

### C26ai decision and reopening condition

Keep SQL compact admission gated. Do not add a parser-state remap.
Do not add a SQL-specific symbol map.

Reopen only after a producer or grammar revision makes both dollar-quote
witnesses match the checked-in and locked-C trees.
Then rerun the C26ah and C26ai Docker probes and the strict grammargen gate.
Require equal table identity, scanner identity, fresh-tree equality, compact
equality, incremental equality, and locked-C equality.

No performance gate ran for C26ai. This receipt makes no performance claim.

## C26aj generated SQL `CREATE DOMAIN` producer witness

C26aj used publication base
`83e0cfbc30ad82e2f327d58e35eea9f438a0ffda`.
It follows the C26ai tagged dollar-quote receipt.

Status: **NO-GO / KEEP LIVE**. Keep SQL compact admission gated.
This slice adds one producer witness, one focused Docker probe, one changelog
entry, and this section. It adds no parser or route change.

### Producer and witness

The pinned SQL producer is `m-novikov/tree-sitter-sql` at commit
`587f30d184b058450be2a2330878210c5f33b3f9`.
The grammar source SHA-256 is
`42f011860137175a5a0cb820d1a694e5ccca1d17f226729ff6a4e886910cde1c`.
The scanner source SHA-256 is
`d437ad9f517d7a1f4248ccd05abe58370b5040c0037c877dab1f0aefeaa04af6`.

The generated blob identity is
`4ffb2a6d09e2000126f10101db9028d28e0752ac3e4f83e401f045c3b028ca7c`.
The scanner identity is
`7e493677411a501e6d8592c6b9cc158e21a1bfed44c72ca914e2d81e4e34861d`.
The checked-in SQL blob identity is
`e21421cbab52b54cf5ba15c8f78a2bb4729bf4e8c0da14368069e897de451268`.

The witness is the 19-byte source `CREATE DOMAIN test;`.
Its source SHA-256 is
`94c5d360b7205e2bd6e84fa28efc2cb3ee2cbc89aa6b759dd3349e578dd133c8`.
The generated tree digest is
`f08d628fa30d83cf92352dbfcab4885b7422ac0fadde9c756e747e6c116dc044`.
The checked-in and locked-C tree digest is
`e72fb9fb57180d5db5be9b649969c7e722d20e1ea4040075260258ee293ce630`.

Both generated and checked-in trees are clean with two root children.
The generated `CREATE_DOMAIN` node has one child. Locked C has no child.
The first divergence is:

```text
root[0][1]: ChildCount go=1 c=0 (goType="CREATE_DOMAIN" cType="CREATE_DOMAIN" goBytes=[7-13] cBytes=[7-13])
```

The difference starts in generated producer output, before compact replay.
The witness is the next smallest remaining SQL producer gap after C26ai.

### Focused Docker gate

The probe is
`cgo_harness/sql_c26aj_generated_create_domain_test.go`.
It checks generated identity, checked-in parity, and locked-C parity.
Run one SQL grammar with one CPU and one Go test worker:

```sh
GOMAXPROCS=1 bash cgo_harness/docker/run_parity_in_docker.sh \
  --repo-root /tmp/gotreesitter-graduation-next.EaYNNn \
  --out-root /tmp/gotreesitter-c26aj-artifacts \
  --label c26aj-sql-create-domain-final --no-build --memory 4g --cpus 1 \
  --gomemlimit 3GiB --goflags -p=1 --test-parallel 1 --timeout 10m \
  --mount /tmp/gotreesitter-sql-seed.ada3CT:/tmp/grammar_parity:ro -- \
  'cd /workspace/cgo_harness && \
   go test -tags "cgo treesitter_c_parity" . \
   -run "^TestSQLC26ajGeneratedCreateDomain$" -count=1 \
   -parallel=1 -timeout=10m -v'
```

The run passed with exit code zero.
It used one SQL grammar, one CPU, `GOMAXPROCS=1`, and `GOFLAGS=-p=1`.
It reported no out-of-memory kill and no wall timeout.

The artifact is
`/tmp/gotreesitter-c26aj-artifacts/20260824T053231Z-c26aj-sql-create-domain-final`.

- `container.log`: `d2aa7a24577775a055d838fd619f17fd7f559395ad76f89b078bd6dcd94b62cf`.
- `metadata.txt`: `7b1bdbc8fc7d1ac21e1692419a4eda2b066ec1003895b70f04e54824d8197ab0`.
- `inspect.json`: `3db73a6ff767692081b99beed02ccd9d93b37a78427c8f001a7ebbaa28a743a0`.

The generated tree remains different from locked C.
The probe records a producer blocker, not a compact parity claim.

### C26aj decision and reopening condition

Keep SQL compact admission gated. Do not add a parser-state remap.
Do not add a SQL-specific symbol map.

Reopen only after the SQL producer or grammar revision makes both
`CREATE DOMAIN` witnesses match the checked-in and locked-C trees.
Then rerun the C26aj gate with C26ah and C26ai coverage.
Require equal table identity, scanner identity, fresh-tree equality, compact
equality, incremental equality, and locked-C equality.

No performance gate ran for C26aj. This receipt makes no performance claim.

## C26ak generated SQL `CREATE DOMAIN AS` producer witness

C26ak used publication base
`83e0cfbc30ad82e2f327d58e35eea9f438a0ffda`.
It follows the queued C26aj `CREATE DOMAIN` receipt.

Status: **NO-GO / KEEP LIVE**. Keep SQL compact admission gated.
This slice adds one producer witness, one focused Docker probe, one changelog
entry, and this section. It adds no parser or route change.

### Producer and witness

The pinned SQL producer is `m-novikov/tree-sitter-sql` at commit
`587f30d184b058450be2a2330878210c5f33b3f9`.
The grammar source SHA-256 is
`42f011860137175a5a0cb820d1a694e5ccca1d17f226729ff6a4e886910cde1c`.
The scanner source SHA-256 is
`d437ad9f517d7a1f4248ccd05abe58370b5040c0037c877dab1f0aefeaa04af6`.

The generated blob identity is
`4ffb2a6d09e2000126f10101db9028d28e0752ac3e4f83e401f045c3b028ca7c`.
The scanner identity is
`7e493677411a501e6d8592c6b9cc158e21a1bfed44c72ca914e2d81e4e34861d`.
The checked-in SQL blob identity is
`e21421cbab52b54cf5ba15c8f78a2bb4729bf4e8c0da14368069e897de451268`.

The witness is the 27-byte source `CREATE DOMAIN test AS text;`.
Its source SHA-256 is
`0cad93f1b70ffa192008df866587c05b206ee404cedd5a0025f542c39c6a504b`.
The generated tree digest is
`67366d68529459613040eb60c5eef47b371fd3091dbe3283c82d7bcff287ea9c`.
The checked-in and locked-C tree digest is
`d0daa4279f00eb8c7278992cff6a870c98bf3deee2292b28f997adbe2f434916`.

Both generated and checked-in trees are clean with two root children.
The generated `CREATE_DOMAIN` node has one child. Locked C has no child.
The first divergence is:

```text
root[0][1]: ChildCount go=1 c=0 (goType="CREATE_DOMAIN" cType="CREATE_DOMAIN" goBytes=[7-13] cBytes=[7-13])
```

The difference starts in generated producer output, before compact replay.
This witness is distinct from C26ah, C26ai, and the queued C26aj witness.

### Focused Docker gate

The probe is
`cgo_harness/sql_c26ak_generated_create_domain_as_test.go`.
It checks generated identity, checked-in parity, and locked-C parity.
Run one SQL grammar with one CPU and one Go test worker:

```sh
GOMAXPROCS=1 bash cgo_harness/docker/run_parity_in_docker.sh \
  --repo-root /tmp/gotreesitter-graduation-next.e5Kf8Z \
  --out-root /tmp/gotreesitter-c26ak-artifacts \
  --label c26ak-sql-create-domain-as-final --no-build --memory 4g --cpus 1 \
  --gomemlimit 3GiB --goflags -p=1 --test-parallel 1 --timeout 10m \
  --mount /tmp/gotreesitter-sql-seed.ada3CT:/tmp/grammar_parity:ro -- \
  'cd /workspace/cgo_harness && \
   go test -tags "cgo treesitter_c_parity" . \
   -run "^TestSQLC26akGeneratedCreateDomainAs$" -count=1 \
   -parallel=1 -timeout=10m -v'
```

The run passed with exit code zero.
It used one SQL grammar, one CPU, `GOMAXPROCS=1`, and `GOFLAGS=-p=1`.
It reported no out-of-memory kill and no wall timeout.

The artifact is
`/tmp/gotreesitter-c26ak-artifacts/20260824T054920Z-c26ak-sql-create-domain-as-final`.

- `container.log`: `50452ea4d162b689cf5fcf59233f055b6715f8acdc168ef28cd993aae694ef08`.
- `metadata.txt`: `44da1ae400911935701d1507342454873b6ecad7f1a9dd0b8d3c695f8e159f00`.
- `inspect.json`: `9faae0b5768c81811cbc843bcc02c1d3b01f664a4a86bd8446cd664557e31b05`.

The generated tree remains different from locked C.
The probe records a producer blocker, not a compact parity claim.

### C26ak decision and reopening condition

Keep SQL compact admission gated. Do not add a parser-state remap.
Do not add a SQL-specific symbol map.

Reopen only after the SQL producer or grammar revision makes both
`CREATE DOMAIN` witnesses match the checked-in and locked-C trees.
Then rerun the C26ak gate with C26ah, C26ai, and the queued C26aj coverage.
Require equal table identity, scanner identity, fresh-tree equality, compact
equality, incremental equality, and locked-C equality.

No performance gate ran for C26ak. This receipt makes no performance claim.

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
