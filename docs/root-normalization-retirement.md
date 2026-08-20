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
The assignment, generated-command, and `if`-field subpasses remain live.
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
| FIDL versioned layout modifiers | retirement change | 1 dispatcher arm | 0 | compatibility-free producer, production, compact fallback, forest-fail-closed, incremental reuse, and isolated C-oracle parity |
| HLSL subscript-assignment declarator | retirement change | 1 HLSL member | 0 | negative dynamic precedence election, compatibility-free producer, production, compact fallback, forest, incremental reuse, and isolated C-oracle parity |
| Swift ternary source reparse | retirement change | 1 Swift subpass | 0 | exact 16-case manifest, native producer, production, compact fallback, forest fail-closed behavior, incremental fresh fallback, and isolated C-oracle parity |

Mark a row merged only after CI and merge evidence exist. Detailed per-entry
receipts stay in the JSON registry and durable run findings stay in Hyphae.
