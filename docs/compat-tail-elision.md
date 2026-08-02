# Compact fresh-path compat-tail elision

Campaign v7 tranche A5 (compatibility retirement) and tranche C1 (perf,
cross-listed). This document defines the elision, the eligibility mechanism,
and the equivalence proof for each elided step. Tranche C0's
[perf-attribution board](perf-attribution.md) names the `compat-tail`
component this tranche narrows.

## What this changes

`admission_switch_candidate.go`'s `tryCompactFullParseRoute` is the compact
fresh-path route. After the compact scheduler accepts and materializes a
tree, it used to run three fixed steps on every parse, regardless of
language:

1. `normalizeReturnedTreeForParse` — the result-compatibility dispatch switch
   (`runLanguageResultCompatibility`, `parser_result_compat.go`) plus a
   full-tree error-summary walk (`summarizeResultErrorsWithStop`).
2. `resolveCRecoverySwallowedError` — the C-recovery-swallow safety net.
3. `maybeCompactReturnedFullTree` — the final-tree arena compaction pass.

A language can lack all three: no live result-compatibility dispatcher arm,
no live dispatcher predicate, and no live generic pass. For that language,
all three steps are provably no-ops on this route. See "Equivalence proof"
below. `tryCompactFullParseRoute` now calls
`finalizeCompactReturnedTreeForParse` instead, for such a language. Step 1
has two pieces that are NOT no-ops: a terminal stop-reason check and
root-span finalization. Those two still run. The rest is skipped.

## Eligibility mechanism

`result_compat_elision.go`'s `resultCompatibilityElisionEligible(lang)` is a
computed set, not a maintained list. It embeds
[`testdata/result_compat_ownership_v1.json`](../testdata/result_compat_ownership_v1.json)
(`//go:embed`) and marks a language ineligible when the registry records a
`"live"` `dispatcher_arm`, `dispatcher_predicate`, or `generic_pass` entry
naming it (a live `generic_pass` naming `"*"` would mark every language
ineligible; none exists today).

This registry is not a second source of truth that could drift from the
switch: `TestResultCompatibilityOwnershipRegistry`
(`compat_ownership_test.go`) parses `runLanguageResultCompatibility`'s switch
and `isCobolLanguage`'s predicate with `go/ast` and fails unless every switch
case has a matching `"live"` registry entry with the identical function set,
and every `"live"` entry names an existing switch case. A future dispatcher
arm cannot ship without updating the registry, and updating the registry is
what keeps `resultCompatibilityElisionEligible` correct — no second edit
required at the elision call site.

## Eligible languages (2026-08-02, this registry)

163 of the 206 registered languages are eligible: every language without a
`"live"` entry in `testdata/result_compat_ownership_v1.json`. They take
`finalizeCompactReturnedTreeForParse` whenever they take the compact route.
The 43 **not** eligible languages are every language with a live dispatcher
arm or the live cobol predicate: `ada`, `apex`, `authzed`, `awk`, `bash`,
`bitbake`, `c`, `c_sharp`, `cobol`, `cooklang`, `corn`, `cpp`, `dart`,
`doxygen`, `dtd`, `elixir`, `enforce`, `fidl`, `go`, `hlsl`, `hyprlang`,
`javascript`, `jsdoc`, `julia`, `kotlin`, `ledger`, `ninja`, `perl`, `php`,
`powershell`, `python`, `ql`, `rust`, `scala`, `solidity`, `sql`, `swift`,
`templ`, `tsx`, `typescript`, `wgsl`, `wolfram`, `yaml`. `go` is in this
list: `dispatch.go` is `"live"` (`normalizeGoReturnedTreeCompatibility`
still runs), so `grammargen_lr` and the other three canonical Go fixtures do
not take the reduced tail. Eligible languages include every retired-arm
language (for example `html`, `linkerscript`, `erlang`, `ruby`, `ocaml`, `d`,
`ebnf`, `zig`) and every language that never had an arm at all (for example
`java`, `css`, `json`, `toml`). `TestResultCompatibilityElisionSetMatchesRegistry`
(`result_compat_elision_test.go`) enumerates the exact set against the
registry on every test run.

## Equivalence proof

Each step's removal is proven for the compact fresh path specifically, not
asserted as a general property of `parser_result_compat.go`.

**Step 1a — the dispatch switch.** For an eligible language,
`runLanguageResultCompatibility`'s switch has no matching case (by
construction — see "Eligibility mechanism"), so it falls through to
`return resultCompatibilityResult{stopReason: ctx.stopReason()}`: a pure
`ctx.stopReason()` (`p.activeParseStopReason()`) read, no tree mutation.

**Step 1b — the error-summary walk.** `applyResultCompatibility` (called with
`summarizeErrors=true` from this path) always runs
`summarizeResultErrorsWithStop`, an O(nodes) walk over a clean tree (a fast
path short-circuits it when `root.hasError()` is already true). Its result —
`(stopReason, errorSummary)` — is entirely discarded by this path's caller:
`(*Parser).normalizeReturnedTree` calls `normalizeResultCompatibility` without
capturing its return value and returns `p.parseStopReasonNow()` — the same
`p.activeParseStopReason()` call `ctx.stopReason()` uses — computed fresh
afterward, independent of anything the walk found. `errorSummary` is never
stored on the compact-route tree either: `tree.resultErrorSummary` keeps its
zero value (`resultErrorSummaryUnknown`) all the way through
`tryCompactFullParseRoute`, exactly as it does with the elision active, since
this path never reads or writes that field. The walk therefore only affects
wall-clock time, never any observable field; `finalizeCompactReturnedTreeForParse`
reproduces the one real effect (a terminal-stop-reason check) directly.

Together, steps 1a and 1b mean `normalizeReturnedTreeForParse`'s branch for
an eligible, not-yet-normalized compact tree (`tree.hasDeferredResultCompatibility()`
is always false here — only production's `resultRootBuild.finishTree` ever
calls `deferResultCompatibility`, which the compact route never runs; and
`tree.resultCompatibilityApplied` is always false on a freshly
compact-materialized tree) reduces to one `p.activeParseStopReason()` read,
one `tree.resultCompatibilityApplied = true` assignment, and
`finalizeReturnedTreeRootSpan` — exactly what `finalizeCompactReturnedTreeForParse`
does.

**Step 2 — the C-recovery-swallow resolver.** `resolveCRecoverySwallowedError`
returns its input tree unchanged unless
`rt.CRecoveryEnteredErrorState && rt.CRecoveryDroppedErrorForClean` (both
read from the tree's own captured `ParseRuntime`, by design — see the
function's doc comment). The only assignment site for either field
(`parser.go`, inside the classic GLR engine's own result-building code) never
runs on the compact route: `materializeDiagnosticParserCoreAcceptedSelection`
(`parsercore_phase0_driver.go`) constructs the returned tree's `ParseRuntime`
from a fresh struct literal that never touches either field, so both stay
`false` for every compact-route tree, every language. This proof does not
depend on eligibility at all; the elision is still gated on it for a single,
auditable call site rather than two.

**Step 3 — final-tree compaction.** `maybeCompactReturnedFullTree`'s
eligibility gate (`eligibleForFinalTreeCompaction`) requires the tree's
retained arena to be at least 512 MiB before it does any O(nodes) work; below
that it is a fixed, small number of field comparisons regardless of
elision. A compact-route tree is a single materialized derivation (the
scheduler's acceptance gate requires exactly one accepted head), not
production's multi-alternative GLR arena, so it does not carry the abandoned-
fork bloat this pass exists to reclaim. No canonical or real-corpus fixture
this campaign measures comes close to the 512 MiB floor. Skipping the call
never changes tree content (compaction only re-homes nodes into a smaller
arena; `docs/compat-tail-elision.md`'s own gate — a full deep-tree digest
comparison — would catch any content change either way); the only
theoretical cost is retaining more memory than necessary on a hypothetical
sub-512-MiB-margin gigantic eligible-language parse. No fixture in this
repository's corpus is close to that scale.

## Gates

- `TestResultCompatibilityElisionSetMatchesRegistry` and
  `TestResultCompatibilityElisionEligibleSpotChecks`
  (`result_compat_elision_test.go`): the computed eligibility set matches an
  independent re-read of the registry, plus named spot checks.
- `TestCompatTailElisionEquivalenceSmoke`
  (`admission_compat_tail_elision_test.go`, tag `gts_parsercorephase0`):
  parses every language's committed smoke sample twice — once through
  `finalizeCompactReturnedTreeForParse` (registry default), once with the
  test-only `SetResultCompatibilityElisionForceDisabledForTest` kill switch
  forcing the full, unelided tail — for every language whose registry
  eligibility and compact-route acceptance both hold, and requires an
  identical `gts-deep-tree-v1` digest. Unconditional; part of the ordinary
  test run.
- `TestCompatTailElisionEquivalenceRealCorpus` (same file): the same
  comparison over `cgo_harness/corpus_real/<lang>` files when present
  (git-ignored, opt-in via `GTS_ADMISSION_REAL_CORPUS=1`, mirroring
  `TestAdmissionCandidateRealCorpusMatrix`'s existing convention).
- `TestAdmissionCandidateScorecard206` with `GTS_ADMISSION_SCORECARD_RATCHET=1`:
  unaffected — the compact-vs-production scorecard, which this change does
  not touch structurally, must still read exactly PASS=198 DIVERGE=0
  FALLBACK=3 SKIP=5.
- Existing per-language `cgo_harness` C-oracle parity tests for eligible
  languages (for example crystal, EBNF, caddy) are unaffected: this change
  never runs on the production route those tests exercise directly, and the
  compact route they exercise indirectly (through `Parser.Parse`) is
  covered by the equivalence gates above.

## Measured recovery

See the PR description for the measured before/after on
`BenchmarkAdmissionCandidateGoQueryCompileWarmRoute` and the
`cgo_harness/attribution` shipped-route reading. Both exercise Go, which is
**not** elision-eligible (`dispatch.go` is live), so neither benchmark moves
from this change; the PR records that finding rather than a synthetic
Go-only "recovery" number. See the PR body for the eligible-language timing
this tranche could actually attribute the recovery to, and its own noise-floor
caveats.
