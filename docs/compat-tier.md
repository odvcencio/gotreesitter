# The C-faithful result-compatibility tier

The root package contains a visible `parser_result_<language>*.go` tier.
It is post-parse compatibility scaffolding: where the parser does not yet
reproduce a C tree-selection or materialization behavior through a general
engine mechanism, a bounded pass reconciles the returned tree with the C
oracle.

This page is a readable guide. The mechanically checked source of truth is
[`testdata/result_compat_ownership_v1.json`](../testdata/result_compat_ownership_v1.json).
Do not update dispatcher coverage, ownership, or retirement state here
without updating that registry.

## Current denominator

The v1 registry freezes the current source and registry:

- 35 explicit `runLanguageResultCompatibility` switch arms;
- one predicate-dispatched COBOL entry matching exactly `cobol` or `COBOL`;
- zero generic passes after language dispatch;
- zero post-finalization second-pass fixpoint arms.

The current registry contains 36 live entries and 49 retired entries. It
names 39 language labels and 38 case-folded language mappings because C/C++,
TypeScript/TSX, and the COBOL case variants share entries. The live entries
contain nine named subpasses. These counts include all accepted retirements.
The registry covers
only this documented internal result-compatibility tier. Scheduler experiments
and other engine research belong in their owning subsystem's durable traces.

Native parsing already recovers a FIDL versioned-layout-modifier declaration
into the C-equivalent error shape. This change retires the FIDL dispatcher
arm.

R2 of `docs/root-normalization-retirement.md` retired three dispatcher arms:
OCaml's collapsed named-leaf restoration, Ruby's top-level module bound
shrink, and half of HTML's arm (the ERROR-root nested-custom-tag
reconstruction). At that checkpoint, HTML's range function stayed live.
R1 later retired that separate second-pass function. The registry keeps both
retired mechanisms as distinct historical receipts.

R3 also retires Arduino's primitive-type arm.
DFA keyword promotion now emits those types before compatibility.
Native Objective-C projection also owns protocol type identifiers.
Generic result selection owns the remaining ambiguous `sizeof` operand.
No Objective-C result-compatibility repair remains live.

The HLSL dispatcher arm retires its subscript-assignment declarator member.
`structured_binding_declarator` carries a negative dynamic precedence in the
grammar, so a clean election already prefers the subscript-assignment
expression natively. The negative-number cast and unorm-buffer members
remain live.

The parser covers every byte in each recovered EBNF source.
This change removes the EBNF compatibility arm.

The exact collapsed-child policy retains a bare Rust `..` token.
The merged-left-side conflict rule selects chained dot-range shifts.
The dot-range compatibility repair is retired.

Each registry entry has a stable ID, functions and files, languages, purpose,
authoritative owner, witnesses, a retirement condition, coverage fields for
production/compact/forest/incremental/C-oracle routes, status, and optional
receipt references. A retired entry remains in the registry with its commit
and receipt references; deleting the historical record is not retirement.

## A0 R1 denominator receipt

Revision R1 of the campaign requires a current firing witness or a filed
defect for every live arm. The evidence-only A0 refresh used commit
`146f334c9ea82e4a29bfe89bccad86f49fb8377b`.

The current smoke probe passed **36 of 36** live entries. It checked 38
case-folded language mappings. Every entry returned `checked >= 1`, `run >= 1`,
and `nodes_visited > 0` through the production parse path with
`GTS_DISPATCHER_CENSUS=1`. The exact log is:

`harness_out/docker/20260813T141019Z-a0-current-arm-probe-v3/container.log`

The current production tier contains 74 `parser_result*.go` files and 50,619
production lines. It contains 45 matching test files and 12,508 test lines.
The root package contains 192 production Go files and 329 test Go files.

The current real-source census observed 21 arm identifiers: six inert arms
and 15 active arms. Its exact log is:

`harness_out/docker/20260813T141146Z-a0-current-real-corpus-census/container.log`

The corpus census did not cover these 17 live arms:

`dispatch.ada`, `dispatch.apex`, `dispatch.authzed`, `dispatch.bitbake`,
`predicate.cobol-exact`, `dispatch.cooklang`, `dispatch.corn`,
`dispatch.doxygen`, `dispatch.dtd`, `dispatch.hlsl`, `dispatch.jsdoc`,
`dispatch.ledger`, `dispatch.ninja`, `dispatch.solidity`, `dispatch.templ`,
`dispatch.wgsl`, and `dispatch.wolfram`.

This remains defect `A0-CORPUS-001`, an evidence gap. Add a small real-source
corpus for each arm before using rewrite frequency for retirement or
attribution. No live arm lacks a firing witness.

The registry still gives every live entry the shared witness path
`cgo_harness/parity_cgo_test.go` and leaves `receipt_refs` empty. This remains
defect `A0-REGISTRY-002`, a traceability gap. Keep it separate from firing
coverage. The durable receipts are
`hypha-receipt:2026-08-11:a0-r1-denominator-refresh` and the current
`hypha-receipt:2026-08-13:a0-current-denominator-refresh-v1`.

### A0 current-head recheck

The current-head Docker recheck used gotreesitter commit
`83ee98eaf85a6bac6b898aa7686ced09389886a5` and completed without an
out-of-memory stop or wall timeout. It reproduced the same evidence:

- 36 of 36 live entries fired in the smoke census.
- The real-source census observed 21 arm identifiers: 6 inert and 15 active.
- The real-source corpus left 17 registered arms uncovered.
- The probe covered 30 languages without a registered dispatcher arm.

The run artifact is
`harness_out/docker/20260813T182414Z-a0-current-arm-census-v1/`.
The container log SHA-256 is
`084390350f5189ef4b5991dd77f6e95f62654d431f665fc6f19433c311735e39`.
The metadata SHA-256 is
`a4a2f981182081c5bdc352182ffe03b32e5755ceaaaa03c8181900c6dcacbcbb`.
The inspection SHA-256 is
`6a5c120caaf2a5fce4a22f56dbc3b93e14ad46e19185619dea054963be8e32fc`.

This recheck confirms A0-CORPUS-001 and A0-REGISTRY-002. It does not close
A0 and changes no parser routing or compatibility arm.

### A0 current source-lock inventory

The external source inventory ran at the current campaign head with corpus
lock SHA-256
`41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`.
It found all 206 locked language roots checked out at their expected commits.
All 206 roots had matching files, with zero missing source roots or files.

The inventory report SHA-256 is
`631fb5831708a2083252869586f5d936ef5282be20ff179c8dec5415c506e39a`.
The selected-language list SHA-256 is
`e81cc7845a3d61210a8f8a450a65f800a53857ac5bdd816e5cdeab482b344cf6`.

The report supplies source candidates for all 17 uncovered live arms. The
repository corpus manifest still contains only 50 languages, so 156 languages
remain without checked-in corpus entries. External source availability is
provenance evidence, not a firing witness. Keep A0-CORPUS-001 open until the
selected files enter the reviewed corpus and the arm witnesses are recorded.

The inventory is evidence-only. It changes no parser route, compatibility arm,
grammar identity, or performance board.

### A0 current-head traceability census

The current-head Docker census used commit
`3a0060ea412bd7c05b5dd534615d483e80ba49c1`. The smoke probe passed 36 of 36
live registry entries. The real-source probe observed 21 arm identifiers: six
inert and 15 active. Seventeen registered arms remained uncovered because
their selected `corpus_real` directories do not exist. Thirty languages were
not applicable because they have no registered arm.

The run passed without an out-of-memory stop or wall timeout. It took 77.825
seconds inside the Docker test.

Run artifact:
`harness_out/docker/20260813T184742Z-a0-current-traceability-v1/`.

- Container log SHA-256: `d8729e426ae08f84d0297fc27248c39c00cd22c8803a70cadb28081dcc1bb2ec`.
- Metadata SHA-256: `8da613ce0283786c50f33579eb9261fda4beb0bfa652a73209376219804232c3`.
- Inspection SHA-256: `97fc54540cbe0fb76466aac7ba1c5b2209f51d80379ed658743301ddd79abea1`.

This receipt proves the current smoke witness and the current corpus defects.
It does not populate `receipt_refs` or close A0-CORPUS-001 and A0-REGISTRY-002.

## Ownership

Compatibility functions describe symptoms. Their `authoritative_owner`
records the earliest subsystem that must eventually produce the C-faithful
result:

| Owner | Responsibility |
|---|---|
| `scheduler_action_semantics` | action ordering, recovery, and work execution |
| `derivation_election_selection` | ambiguity and winning-derivation choice |
| `materialization` | node, field, alias, trivia, and span construction |
| `scanner_checkpoint_state` | external-scanner state and restoration |
| `incremental_edit_reuse` | edit invalidation and subtree reuse |
| `public_compatibility` | compatibility intentionally retained at an exported API boundary |

The route fields use a closed vocabulary enforced by the registry test:

- live dispatcher, predicate, and generic entries use
  `shared_result_compatibility_tail` for production/compact/forest/incremental
  and `curated_single_grammar_parity` for the C oracle;
- retired production and incremental routes use `retired_exact_receipt`;
- retired compact and forest routes use `retired_exact_receipt` or
  `retired_exact_or_fail_closed_receipt`;
- the C oracle uses `retired_exact_receipt` or
  `retired_known_divergence_receipt`.

Each retired entry must include a retirement commit and a receipt reference.

These are evidence labels, not claims that all five engines have independent
implementations. The shared-tail value means that route reaches the same
post-parse normalization tail.

## Materialization subpass census

One live arm, Bash, still uses `materialization` as its aggregate owner.
Ada's and Apex's entries now aggregate under `derivation_election_selection`.
Two of Ada's three subpasses pick a winning derivation from source text. They
choose between grammatically distinct productions. The entry-level owner now
matches that majority. The association-choice subpass still builds its own
wrapper. It keeps `materialization` at the subpass level. Apex's single
subpass relabels a locked C-oracle-diverging derivation election: a grammar-
declared conflict between `class_literal` and `field_access` with no
`prec.dynamic` tie-break, where both derivations reach acceptance and
gotreesitter's runtime accept-selection elects the derivation the C oracle
does not; the entry-level owner now matches. Each arm declares its distinct
subpasses in the registry.

Set `GTS_DISPATCHER_CENSUS=1` to record each subpass and its aggregate arm.
The census does not change parser output when it is disabled.

| Arm | Subpass | Upstream owner |
|---|---|---|
| Ada | constraint kind election | `derivation_election_selection` |
| Ada | aggregate kind election | `derivation_election_selection` |
| Ada | association choice construction | `materialization` |
| Apex | generic local declaration | `derivation_election_selection` |
| Apex | class literal alias | `derivation_election_selection` |
| Bash | assignment wrapper flattening | `materialization` |
| Bash | generated command assignment | `scheduler_action_semantics` |
| Bash | `if` condition field projection | `materialization` |
| Cooklang | trailing step tail | `scanner_checkpoint_state` |
| Cooklang | recovered recipe | `scheduler_action_semantics` |

Cooklang calls its trailing-tail subpass from `parser_result_misc_spans.go`.
The registry now includes that function and file.

The HTTP section-coalescing subpass retired after its exact and locked probes
reported zero rewrites. Native route tests now preserve that receipt.

The Bash command-name concatenation subpass also retired.
Native reduction already constructs the historical command name.
The exact 25-case corpus matches baseline `83548f55`.

The live probes parse each source without result compatibility.
They also compare census-enabled output with normal production output.
Each digest matches base commit
`4f077315d3aa5dcec4f5392c7eb17a1bbc27a455`.
The locked Ada probes come from tree-sitter-ada commit
`6b58259a08b1a22ba0247a7ce30be384db618da6`.

## Why it exists

Two GLR parsers can accept the same input and still return different trees
under ambiguity, error recovery, aliasing, or extra/trivia attachment.
gotreesitter's parity target is byte-exact agreement with the selected C tree,
including error shapes and recovered spans. The cgo-backed suites under
`cgo_harness/` provide that oracle.

The tier stays internal because it operates on arena and node internals before
the tree is returned. Moving it into a package with exported plumbing would
make the exported API surface worse without changing its ownership.

## Current progress: collapsed children

All 27 registered collapsed child rows for the ten affected built-in
languages now produce their child shape natively. The occurrence policy is
admitted by the exact-profile receipt and compiled from exact named parent and
raw-child metadata identities, so true adapted clones retaining both take the
same upstream construction path. A display-name or pair-level metadata match
alone does not admit a caller-built or custom artifact. Focused production,
compact, forest, and incremental witnesses prove the shape without a
compatibility traversal, and the per-pair C-oracle census is nonzero and equal.
The generic reconstruction walk is therefore retired and deleted. Once raw
child identity has been lost, a display-name-compatible caller artifact is no
longer guessed into shape; custom artifacts must carry the explicit native
capability and exact metadata receipt to opt in.

## Current progress: unary named wrappers

Reduction materialization now owns certified same-span unary wrapper chains.
The parser core uses public-parent, wrapper, leaf, and parser-state identities.
It contains no language-name condition.

The exact F# artifact certifies one declaration-name state.
Its `long_identifier` materializes as the named `identifier` leaf.
Expression identifiers and dotted identifiers retain their wrappers.
Custom, adapted, and stale artifacts retain conservative behavior.

Production and incremental routes return the exact tree without compatibility.
The compact route returns the exact tree or uses production.
The forest route fails closed for this profile.
The pinned C parser supplies the parity oracle.

## Current progress: collapsed token wrappers

Reduction now owns one four-language collapsed-token family.
It uses grammar metadata and token identity without a language list.

- HCL wrapper nodes keep their punctuation and boolean token children.
- C# modifiers, booleans, and the `global` identifier keep their token children.
- PowerShell assignment operators keep their token children.
- CPON null nodes remain childless after the same-name token collapse.

The CPON dispatcher arm is retired.
The HCL, C#, and PowerShell arms remain live for unrelated repairs.
Compatibility-free, production, compact, forest, and incremental routes match.
Each isolated C-oracle receipt also matches.

## Current progress: alias-preserved wrappers

The pinned CUE, Git Commit, and R blobs now carry their C alias maps.
Materialization uses that metadata before it publishes a tree.

The native producer now keeps these collapsed children:

- The CUE `value` wrapper keeps its named `identifier`.
- The Git Commit `message` wrapper keeps its named `message_line`.
- The R `string_content` wrapper keeps its named `escape_sequence`.

All four native routes return the same trees without a compatibility walk.
CUE proves nonzero old-tree reuse.
Git Commit and R record their external scanner reuse limit.
The locked C parsers return the same child shapes and spans.

## Current progress: terminal leaves

The generic terminal-leaf mutation is removed. Reduction and alias
materialization now own its tree shape.

The route receipt covers production, compact, forest, and incremental parsing.
The scanner-aware corpus receipt covers 45 languages and 868,010 nodes.
It finds no retired shape and reports 161 languages as uncovered.
The focused Go tree also matches the locked C tree.

The full error-summary walk remains. It preserves exact retry selection for
under-set descendant errors and keeps the existing stop polls.

The registry records checkpoint 1 commit
`31bc9f1ed88bc930d22d0c2eaedc84195604cce1` as `retired_commit`.
PR #465 merged the retirement.

`normalizeResultCompatibility` now calls `summarizeResultErrorsWithStop`
directly.

## Current progress: trailing root trivia

Clean hidden whitespace-only root tails are now finalized as root span coverage
instead of reconstructed in the shared compatibility tail. The rule applies
before result compatibility, including compatibility-free production and
compact parses; forest and incremental routes share the same root finalizer.
Error roots retain their recovery extra, and lazy final-child references are
filtered without draining the compact range. Real RST and Comment fixtures are
exact against their C oracles. The generic trailing-extra pass is retired.

## Current progress: hidden root trivia

Shared root finalization now removes hidden, childless whitespace extras at
every root position. It preserves visible comments, fields, spans, and lazy
final child references. Error roots keep every extra as recovery evidence.

The HCL real-corpus census found no body span mismatch across 114 bodies.
Production, compact, forest, and incremental routes return the native shape.
HCL records its external scanner reuse limit. The locked C parser matches.
No HCL result-compatibility arm remains.

## Current progress: trailing root and child spans

Materialization now owns the retired trailing-span family for nine languages.
This family covered Caddy, Comment, Fortran, Just, Nginx, Nim, Pascal, Pug,
and RST.

The compact scheduler admits a zero-width extra only when an external scanner
produces it and the scanner checkpoint or parser state advances. Forest
publication omits zero-width synchronization extras as visible children.
These extras still extend the source range.

Production, compact, forest, incremental, and locked C routes return exact
spans for all nine languages. Pascal proves actual old-tree reuse. Comment
proves its expected root-only invalidation. External scanner limits prevent
reuse for the other seven languages.

## Current progress: leading root trivia

Shared root finalization now excludes unowned leading token padding.
It preserves a prefix when the first retained child owns that prefix.

The compact admission gate accepts the same first-token start.
This rule applies to all result routes without a language allowlist.

The change removes local repairs for:

- BibTeX.
- CPON.
- CUE.
- D.
- Forth.
- Kotlin.
- Squirrel.

Squirrel needed no other result repair, so its dispatcher arm is retired.

## Current progress: field projection

Reduction now owns the retired Lua, Make, and Zig field shapes.
It projects inherited fields through hidden productions.
It also projects direct fields across each flattened child span.
The Zig grammar metadata does not assign `field_constant` to initializer lists.

Reduction also owns the Haskell and Erlang root field shapes.
The field plan retains each conflicting inherited mapping.
The reduce path projects a mapping only when a named child matches its field.
Root acceptance preserves producer fields when it absorbs surrounding trivia.

The native reduction path sets Dart switch-expression body fields.
It sets the target field for nested Elixir calls.
The two language-local repairs are inert and removed.
Inherited reduction fields now preserve repeated direct descendants.
They fill anonymous separators only between those direct descendants.
An inherited field without direct evidence cannot cross a leading separator.
These rules remove three Scala field repairs and the SQL `INTO` cleanup.

## Current progress: Erlang native ownership

Native derivation election chooses the correct Erlang macro replacement.
It separates function clauses from case and receive clauses.
Reduction also owns each top-level form span.

The authenticated Erlang corpus reports zero dispatcher rewrites.
Compatibility-free witnesses cover both macro replacements.
They also cover the top-level form spans.
Production, compact, forest, and incremental routes return exact trees.
The incremental route reuses the old tree.
The reference C parser returns the same shapes and spans.
The Erlang dispatcher arm is retired.

Compatibility-free parses prove each producer shape.
Production, compact, forest, and incremental routes return the same trees.
Make and Zig prove old-tree reuse.
Lua records its external scanner reuse limitation.
The locked C parsers return the same fields.

Erlang proves old-tree reuse and exact isolated C parity.
Haskell records its external scanner reuse limit.
Its isolated C results match the unchanged base floors.

## Current progress: zero-width artifacts

The root classifier drops visible zero-width extras from child lists.
The classifier keeps every extra when it computes the root byte range.
The C-faithful repetition-skip fold stops the historical Typst comma
artifact. These shared producer rules retire two returned-tree walks.

The native Haskell scanner proves UPDATE is a real zero-width token.
Typst uses a minimized witness from its pinned component corpus.
Production, forest, and incremental routes contain no retired artifact.
The shared compact-child receipt keeps final references lazy.
Typst keeps its dispatcher arm for other repairs.

## Current progress: Haskell section spans

Native reduction owns the Haskell `imports` and `declarations` ranges.
The real-corpus census found zero Haskell span rewrites across three files.
Production and compact routes return the exact native ranges.
The incremental route returns the same ranges after its scanner limitation.
The Haskell forest route publishes its existing module-only witness.
It declines the section witness at its existing reduction cap.
The locked C parser returns the same section shapes and ranges.
No Haskell result-compatibility arm remains.

## The retired second pass

Checkpoint A removed the span calls and their two exclusive helpers.
Checkpoint B removed the four duplicate recovery, field, and annotation calls.
The final checkpoint deletes the marker and the shared fixpoint.

Child rewrites preserve a valid producer-owned span. A rewrite can widen the
span but cannot shrink it. This language-neutral rule
replaces the Scala function, block, case-clause, and root span repairs.

The route receipt covers production, compact, forest, changed incremental,
and fresh parsing. Scala incremental reuse is unsupported because its
external scanner lacks reuse support. The receipt records zero reused
subtrees and bytes. It does not claim incremental reuse.

HTML no longer participates in this fixpoint. Materialization extends each
recovered custom element through its structural `_implicit_end_tag` child.
Production, compact, forest, and incremental routes return the same absolute
ranges and points. The incremental receipt proves nonzero old-tree reuse.
The locked C parser returns the same recovered ranges and points.

JavaScript no longer participates in this fixpoint. Its canonical
compatibility pipeline already extends `program` and recovery-root terminator
tails after every JavaScript shape and span rewrite. The only intervening work
before returned-tree publication is the read-only error-summary walk and
optional parent-link wiring. Neither can shorten or reclassify the root.
The registry retains a retired historical entry for the deleted JavaScript
arm and its production, compact-final-ref, forest, and incremental receipts.

The authenticated Scala corpus reports zero second-pass mutations.
The mandatory fixture witness also reports zero mutations.
No post-finalization fixpoint remains.

## Editing and validation

When adding, moving, or retiring compatibility code:

1. Update the JSON registry in the same change.
2. Keep dispatcher languages, called functions, ownership, witnesses, routes,
   and retirement evidence explicit.
3. For retirement, keep the entry and set `status` to `retired`, with
   `retired_commit` and at least one `receipt_refs` item.
4. Run the focused registry gate:

```sh
go test . -run '^TestResultCompatibilityOwnershipRegistry$' -count=1
```

The test parses `parser_result_compat.go`, `parser_result_helpers.go`, and
`parser_api.go` with the Go AST. It fails when a dispatcher arm or predicate
is unregistered, the exact COBOL predicate language set drifts, a generic pass
changes without a registry update, a post-finalization language arm or call
changes, or a live registered function disappears from its declared files.
The COBOL check requires the canonical nil-guard AND parenthesized two-equality
OR AST. The fixpoint check counts clause-local call occurrences and rejects
normalization in default clauses or outside registered cases. The gate also
enforces kind/status combinations, route vocabulary and semantics, required
metadata, and referenced witness paths.

This document is hand-maintained explanatory text, not generated output. The
JSON registry and the focused Go test are the regeneration/validation
contract.
