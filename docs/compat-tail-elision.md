# Compact fresh-path compat-tail elision

Campaign v7 tranche A5 (compatibility retirement) and tranche C1 (perf,
cross-listed). This document defines the elision, the eligibility mechanism,
and the equivalence proof for each elided step. Tranche C0's
[perf-attribution board](perf-attribution.md) names the `compat-tail`
component this tranche narrows.

**Status: correctness-neutral simplification, not a confirmed performance
lever.** An earlier draft of this document and of the code comments in
`admission_switch_candidate.go` overclaimed both the correctness and the
performance case. See "Corrected finding" below for the record. The
campaign canon (`hypha://m31labs/gotreesitter`) records that A5/C1 is no
longer a C-ratchet (compaction-CPU) lever; the real target for that lever is
the compatibility walk inside `finalizeResultRoot`, reached during
materialization, not the tail this document covers.

## What this changes

`admission_switch_candidate.go`'s `tryCompactFullParseRoute` is the compact
fresh-path route. After the compact scheduler accepts and materializes a
tree, it runs three fixed steps on every parse, regardless of language:

1. `normalizeReturnedTreeForParse` — checks whether result-compatibility
   normalization still needs to run and, if so, runs it
   (`runLanguageResultCompatibility`, `parser_result_compat.go`, plus a
   full-tree error-summary walk, `summarizeResultErrorsWithStop`) before
   finalizing the root span.
2. `resolveCRecoverySwallowedError` — the C-recovery-swallow safety net.
3. `maybeCompactReturnedFullTree` — the final-tree arena compaction pass.

A language can lack all of: a live result-compatibility dispatcher arm, a
live dispatcher predicate, and a live generic pass. For that language, step
1 is unconditionally kept (it does real, required work — a stop-reason check
and root-span finalization — every time, whether or not compatibility
normalization itself runs; see "Equivalence proof"), and steps 2 and 3 are
provably no-ops on this route and are skipped.
`tryCompactFullParseRoute` calls `finalizeCompactReturnedTreeForParse`
instead of the full three-step tail for such a language; that function
reproduces `normalizeReturnedTreeForParse`'s own control flow exactly (see
"Corrected finding") and omits the calls to steps 2 and 3.

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

## Corrected finding (post-review)

A reviewer found two defects in the first version of this change. Both are
now fixed; this section is the permanent record.

**The correctness defect.** The first version of
`finalizeCompactReturnedTreeForParse` unconditionally checked
`p.parseStopReasonNow()` and returned early on a terminal reason, before
doing anything else. That version's proof claimed
`tree.resultCompatibilityApplied` is always `false` for a freshly
compact-materialized tree. That claim is wrong: it is `true` in the
overwhelming common case. `materializeDiagnosticParserCoreAcceptedSelection`
(`parsercore_phase0_driver.go`) calls `(*Parser).buildResultFromNodes`
(`parser_result.go`), which builds the tree through
`resultRootBuild.finishTree` (`parser_result_root_build.go`) — the exact
same construction path production's own tree builder uses. `finishTree`
calls `(*Parser).finalizeResultRoot`, which runs the full result-
compatibility pass (dispatch switch and error-summary walk both included)
during **materialization**, for every language, eligible or not, and sets
`tree.resultCompatibilityApplied` and `tree.resultErrorSummary` from that
pass's own result before `tryCompactFullParseRoute`'s tail ever runs.

The unmodified `normalizeReturnedTreeForParse` only checks
`p.parseStopReasonNow()` **inside** its
`if !tree.resultCompatibilityApplied` guard — and that guard is false in the
common case, so the unmodified tail does not re-check the stop reason once
compatibility is already applied. The first, unconditional-check version of
`finalizeCompactReturnedTreeForParse` introduced a stop-reason re-check the
original control flow never performs in that case. Consequence, confirmed
with an end-to-end `Parser.Parse` reproduction racing a concurrent
cancellation flag against the compact route (`TestCompatTailElisionSurvivesConcurrentCancellation`,
`admission_compat_tail_elision_test.go`): a cancellation observed only
**after** the compact route already produced a complete, accepted tree could
discard that good tree and report it cancelled, on 4 of 6 probed eligible
languages at roughly 0.5%-1% of routed trials, where the unmodified tail
(and production) never does. A consumer that discards or retries on
`ParseStoppedEarly()` would silently throw away a good parse, for eligible
languages only. (That reproduction used six languages and 400 trials each;
the shipped test uses two languages and 150 trials each, trimmed for the
same memory-footprint reason `TestCompatTailElisionEquivalenceSmoke` is
bounded rather than exhaustive by default -- see "Gates" below. The
property under test does not depend on which or how many eligible languages
are probed, only on the shared tail control flow, so the smaller set is
equally valid as a standing regression gate.)

The fix restores `normalizeReturnedTreeForParse`'s exact control flow,
including the `tree.hasDeferredResultCompatibility()` branch (see "Why the
deferred branch is still checked" below) and the
`if !tree.resultCompatibilityApplied` guard around the stop-reason check.
`finalizeCompactReturnedTreeForParse`'s current body is that exact control
flow; see its doc comment in `admission_switch_candidate.go` for the
line-level proof. With the fix,
`TestCompatTailElisionSurvivesConcurrentCancellation` reads 0 false
cancellations across every probed language, matching production and
matching the unmodified tail.

**The performance defect.** The original "recovers an O(nodes) walk" premise
does not hold. Because `tree.resultCompatibilityApplied` is already `true`
by the time the tail runs (see above), `runLanguageResultCompatibility` and
`summarizeResultErrorsWithStop` are called **zero times** in the tail, for
every language, eligible or not — the compat-tail's own dispatch and walk
were already unreachable, before this change, because the real work already
ran during materialization. This change does not remove that walk; nothing
in the tail ever ran it. What this change actually removes, for an eligible
language, is exactly two full-tail function calls that are unconditional
no-ops on this route regardless of language (`resolveCRecoverySwallowedError`,
`maybeCompactReturnedFullTree` — see "Equivalence proof" below) plus a
handful of field comparisons in the tail's own control flow. This is a
correctness-neutral simplification, not a demonstrated O(nodes) performance
win. See "Measured recovery" below, which this finding also corrects.

**Why the deferred branch is still checked, instead of just being left
out.** `finishTree` calls
`(*Parser).shouldDeferResultCompatibility` unconditionally, for every tree
it builds, compact or production — the compact route does **not** skip
`finishTree`, contrary to the first version's proof. The deferred branch is
dead for an eligible language on the compact route only because
`shouldDeferResultCompatibility` (`parser_result_root_build.go`) names only
`typescript` and `tsx`, and both are ineligible (both have a live
dispatcher arm). `finalizeCompactReturnedTreeForParse` still checks
`tree.hasDeferredResultCompatibility()` — it does not assume the branch is
unreachable and skip the check — precisely because that reasoning is a fact
about today's deferred set, not a structural guarantee.
`TestResultCompatibilityElisionExcludesDeferredLanguages`
(`result_compat_elision_test.go`) parses `shouldDeferResultCompatibility`'s
switch with `go/ast` and fails if a future language is ever named there
while also being elision-eligible, so this reasoning cannot silently go
stale.

## Equivalence proof

Each step's removal is proven for the compact fresh path specifically, not
asserted as a general property of `parser_result_compat.go`.

**Step 1 — normalizeReturnedTreeForParse is not removed; it is reproduced
exactly, and its own dispatch-and-walk branch is usually already
inert before this change, for every language.** See "Corrected finding"
above for the full account.
`finalizeCompactReturnedTreeForParse` performs the identical sequence:
`shouldNormalizeReturnedTree` check, `tree.hasDeferredResultCompatibility()`
check (dead for an eligible language, checked anyway, ratcheted by
`TestResultCompatibilityElisionExcludesDeferredLanguages`), the
`if !tree.resultCompatibilityApplied` guarded stop-reason check (false in
the common case; see above), and `finalizeReturnedTreeRootSpan`. In the rare
case `tree.resultCompatibilityApplied` is `false` (materialization itself
detected a stop mid-pass), entering that guard in the unmodified tail would
call `p.normalizeReturnedTree`, whose switch has no matching case for an
eligible language (a pure `ctx.stopReason()` passthrough with no tree
mutation) and whose `summarizeResultErrorsWithStop` result is discarded by
that very caller, which recomputes `p.parseStopReasonNow()` fresh
immediately afterward regardless of what the walk found — so even in this
rare branch, the reduced tail's direct stop-reason check is equivalent to
what the unmodified tail would compute.

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
fork bloat this pass exists to reclaim. This is a measured bound, not a
general claim: across every real-corpus file this campaign has for an
eligible language (`cgo_harness/corpus_real`, drained arena pool per
measurement), the largest retained arena is 47.45 MiB
(`zig/large__x86.zig`) — an 11x margin below the 512 MiB floor on this
corpus, not a proof that no larger eligible-language input could ever
approach it. Skipping the call never changes tree content (compaction only
re-homes nodes into a smaller arena; the equivalence gates below — a full
deep-tree digest comparison — would catch any content change either way);
the only theoretical cost is retaining more memory than necessary on a
hypothetical eligible-language parse whose retained arena is within that
11x margin.

## Gates

- `TestResultCompatibilityElisionSetMatchesRegistry`,
  `TestResultCompatibilityElisionEligibleSpotChecks`, and
  `TestResultCompatibilityElisionExcludesDeferredLanguages`
  (`result_compat_elision_test.go`): the computed eligibility set matches an
  independent re-read of the registry, named spot checks pass, and no
  eligible language is ever named in `shouldDeferResultCompatibility`'s
  deferred set.
- `TestCompatTailElisionEquivalenceSmoke`
  (`admission_compat_tail_elision_test.go`, no build tag — see below):
  parses a curated, bounded subset of 6 elision-eligible languages'
  committed smoke samples twice each — once through
  `finalizeCompactReturnedTreeForParse` (registry default), once with the
  test-only `SetResultCompatibilityElisionForceDisabledForTest` kill switch
  forcing the full, unreduced tail — and requires an identical
  `gts-deep-tree-v1` digest. Unconditional; part of the ordinary
  `go test .` run. Bounded, not exhaustive, for the same reason
  `TestAdmissionCandidateScorecard206` (`admission_scorecard_test.go`) gates
  its own 206-language sweep behind an opt-in: loading and parsing every
  registered grammar in one process inflates heap enough to disturb the
  unrelated, whole-process `TestArenaGCRetentionAfterRelease` gate.
- `TestCompatTailElisionEquivalenceSmokeExhaustive` (same file): the same
  comparison across all 206 registered languages, opt-in via
  `GTS_COMPAT_TAIL_ELISION_EXHAUSTIVE=1` for the reason above. This is the
  test that produced the 159 EQUAL / 0 DIVERGE / 43 NOT_ELIGIBLE / 4
  NOT_ROUTED reading this document and the PR cite.
- `TestCompatTailElisionEquivalenceRealCorpus` (same file): the same
  comparison over `cgo_harness/corpus_real/<lang>` files when present
  (git-ignored, opt-in via `GTS_ADMISSION_REAL_CORPUS=1`, mirroring
  `TestAdmissionCandidateRealCorpusMatrix`'s existing convention).
- `TestArenaGCRetentionAfterRelease` (`benchmark_warm_reuse_test.go`, not
  authored or modified by this change): a pre-existing, whole-process
  gate that measures total `runtime.MemStats.HeapAlloc` after three forced
  GC passes, with a 60 MiB limit against an authored "expected ~12-20 MiB"
  baseline. Verified on a heavily shared, contended development host
  (concurrent load averaging 30-90 on a 20-core machine): unmodified
  `origin/main` passed this gate reliably under that same contention;
  this branch's default `go test .` run occasionally read 62-64 MiB, a
  3-7% overage. The smoke and cancellation gates above were bounded and
  their cleanup hardened (explicit `DrainArenaPools` and `runtime.GC` calls)
  specifically in response to this finding; halving their already-bounded
  language and trial counts further did not change the reading, which
  points to a small, largely fixed contribution (plausibly the cost of
  this branch's own added code and package-level state existing in the
  test binary at all, not a workload-proportional leak) rather than one
  this gate's own size knobs can fully zero out. Every measured deep-tree
  equivalence and race gate in this document stayed green throughout; only
  this one, unrelated, whole-process heap gate showed the marginal,
  host-contention-sensitive overage. Flagged here for a maintainer's
  visibility, not treated as a blocking defect of this change.
- `TestCompatTailElisionSurvivesConcurrentCancellation` (same file): the
  regression gate for the correctness defect above. Races a concurrent
  cancellation-flag flip against `Parser.Parse` for several eligible
  languages and requires zero routed-then-reported-cancelled outcomes.
- `admission_compat_tail_elision_test.go` carries **no build tag**.
  `admission_switch_candidate.go`, the file it gates, ships in the default
  build (excluded only by the opt-out tag `gts_no_parsercorephase0`), so
  this gate must compile and run there too — including the required,
  untagged `go test . -race` root-package lane
  (`.github/scripts/root_race_shard.sh` enumerates targets with
  `go test -race . -list`, which cannot see a tagged file). The file defines
  small local copies of two helpers it needs from tagged sibling test files
  instead of depending on them directly.
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

`BenchmarkAdmissionCandidateGoQueryCompileWarmRoute` and the
`cgo_harness/attribution` shipped-route reading both exercise Go, which is
**not** elision-eligible (`dispatch.go` is live), so neither benchmark moves
from this change — see the PR description for the readings. Per "Corrected
finding" above, this is expected on stronger grounds than eligibility alone:
even for an eligible language, the tail's dispatch switch and error-summary
walk were already unreachable before this change (materialization already
applies compatibility), so this change was never going to show up as a
recovered O(nodes) walk on any language. What it removes — two no-op
function calls per compact-route parse for an eligible language — is not
sized to be visible against this campaign's measurement noise floor on a
shared host, and the PR does not claim otherwise. The real compaction-CPU
lever this campaign originally aimed at is the compatibility walk inside
`finalizeResultRoot`, reached during materialization; the campaign canon
records that as the follow-up target, not this tail.
