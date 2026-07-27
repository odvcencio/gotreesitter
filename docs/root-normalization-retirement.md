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

Status: the first retirement PR is prepared; eight more dispatcher arms
censused zero and stayed live pending a wider corpus or a narrower sub-pass
split.

The mandatory shape is census before migration. Historical audits already
found that table or engine fixes can leave old normalizers behind.

1. Re-run the Rust dot-range census over the full authenticated Rust corpus.
   If rewrites remain zero and synthetic controls fire, remove the pass and
   its exclusive traversal in one PR.
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
- Swift: ternary-expression recovery.
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

Status: queued.

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
| Shared returned-tree fixpoint | deleted on branch `codex/generalize-scala-produced-spans-20260726` | 1 inert arm | 0 | ownership denominator, focused route tests, and exact Scala C-oracle receipt |

Mark a row merged only after CI and merge evidence exist. Detailed per-entry
receipts stay in the JSON registry and durable run findings stay in Hyphae.
