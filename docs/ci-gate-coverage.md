# CI gate coverage map

## Grammar-lock stamp receipts, September 20, 2026

Ten `cgo_harness` receipt tests pinned a `*GrammarLockSHA256` constant.
The constant held the SHA-256 of `grammars/languages.lock` from before
the Kotlin grammar bump. The lock changed four more times since: Kotlin,
Swift, Scala, and a six-grammar batch (C, C++, Elixir, JSON, Nix, and
PHP). The pinned constant no longer matched the file after each change.
No CI lane ran these receipts, so the drift went unnoticed.

### Classification

Each receipt's own digests key to one grammar entry inside the lock
file. For example, the Julia receipt's evidence depends only on the
`julia` line. None of the four lock changes since the pin touched any of
the ten receipts' own lines: Julia, WGSL, Templ, Wolfram, Solidity,
Perl, Go, Doxygen, Cooklang, or SQL. The whole-file hash comparison was
therefore a provenance stamp, not a precondition. It existed to make
lock drift fail loudly. It did not exist because the receipt's own
evidence depends on every other language's pinned commit. All ten
receipts are shape (a) in this sense. None are shape (b): a receipt
whose own digests tie to the exact lock file, byte for byte.

### Fix

`cgo_harness/grammar_lock_stamp_helpers_test.go` adds
`currentGrammarLockSHA256(t)`. It hashes the live
`grammars/languages.lock` file. Each of the ten receipts now derives its
lock-hash stamp from this helper instead of a frozen literal. A future
lock bump that does not touch the receipt's own grammar line no longer
fails the receipt. The affected files are:

- `julia_dispatch_blocker_receipt_test.go`
- `wgsl_n31r_dispatcher_blocker_receipt_test.go`
- `cooklang_next_live_probe_test.go`
- `templ_n31q_dispatcher_blocker_receipt_test.go`
- `wolfram_next_live_probe_test.go`
- `solidity_next_live_probe_test.go`
- `perl_n31s_dispatcher_blocker_receipt_test.go`
- `go_next_live_probe_test.go`
- `doxygen_next_live_probe_test.go`
- `sql_n31v_dispatcher_blocker_receipt_test.go` (this one only logged the
  stale hash; it had no comparison to fail)

`docs/root-normalization-retirement.md` keeps its historical
`9ddb6324af...` markers unchanged. That document is a dated log of past
receipt states. Two of its guard tests
(`TestDartNextLiveArmReceiptDocument`, `TestCooklangNextLiveArmReceiptDocument`)
check that the log text still contains those historical entries. Neither
guard test hashes a live file. Lock drift never affected either one.

### Docker results and why four receipts stay out of the gate

A Docker run confirmed the fix on all ten receipts against the current
lock and blobs. Every receipt's evidence-identity check passed.
Cooklang, Wolfram, Solidity, Doxygen, SQL, and Perl passed in full.

Julia, WGSL, Templ, and Go passed their grammar-lock stamp check. Each
one then failed on an unrelated, pre-existing digest or route mismatch.
The stale lock-hash check had masked these failures before, since the
receipt failed earlier, at the hash comparison:

- Julia: `TestJuliaDispatchBlockerReceiptRoutes/real-julia-utils` reports
  an incremental-reuse count of `0/0` where the witness expects `3/18`.
- WGSL: `TestWGSLN31rDispatcherBlockerReceipt/a0-normalMap` and
  `.../a0-radiosity` report raw-route digests that do not match their
  witness table.
- Templ: `TestTemplN31qLiveArmLockedCRoutes/a0-medium-main`,
  `.../clean-component-import`, and `.../malformed-dangling-quote`
  report raw-route digests that do not match their witness table.
- Go: `TestGoNextLiveArmProbe/new-make-types` reports an accepted compact
  route where the witness expects a fallback, and the included-ranges
  case reports a digest that does not match its pin.

These four regressions are pre-existing parser-behavior drift. The
grammar lock is not their cause. Fixing them needs separate,
per-language correctness investigation. `grammar-lock-stamp-receipts-cgo`
in `ci.yml` gates only the six receipts that pass in full. Adding the
other four now would turn `build` red for reasons this change did not
cause and cannot fix.

Once a fix lands for one of the four digests above, re-run its receipt
locally in Docker, for example:

```sh
bash cgo_harness/docker/run_parity_in_docker.sh --no-build \
  --run '^TestJuliaDispatchBlockerReceiptRoutes$'
```

Then add its test name to the `grammar-lock-stamp-receipts-cgo` job.

### Adjudication of the four masked receipts, September 20, 2026

A follow-up adjudication ran each of the four receipts above in Docker
against the C oracle and bisected each digest move to its causing
commit. All four causes are deliberate, already-merged parser
improvements, not new regressions:

- Julia: the `real-julia-utils` incremental-reuse count moved from
  `3/18` to `0/0`. Commit `75f4e7553` ("separate selected reuse from
  attempted work", issue #454) stopped a full retry from crediting the
  discarded attempt's reuse work to the selected result. `0/0` is the
  correct count for a result that reused nothing. Re-pinned.
- WGSL: the `a0-normalMap` and `a0-radiosity` raw-route digests moved.
  Commit `a122eac7d` ("align C recovery parity and expand the recovery
  board") changed which nodes carry an error bit on absorbed leaves.
  The grammar-specific compatibility patches in
  `parser_result_wgsl.go` matched the old, less-correct shape. They no
  longer fire. The per-kind node histogram distance to the C oracle
  stayed the same before and after the change. Re-pinned.
- Templ: the `a0-medium-main`, `clean-component-import`, and
  `malformed-dangling-quote` raw-route digests moved. Same cause as
  WGSL, commit `a122eac7d`. Re-pinned.
- Go: `new-make-types` now reports an accepted compact route where the
  witness expected a fallback. The included-ranges digest also moved.
  Two separate causes explain this. Commit `6dab5c951` ("Admit bounded
  materiality-equal EOF frontiers", #981) widened compact-route EOF
  acceptance. `new-make-types` now takes the fast compact path and
  reaches the same result as the C-matching production route. Commit
  `a122eac7d` changed the included-ranges parse the same way it changed
  WGSL and Templ. Re-pinned.

`grammar-lock-stamp-receipts-cgo` now gates all ten receipts.

## Package execution coverage, September 18, 2026

The non-root race matrix now includes 13 previously omitted test packages.
These packages cover:

- queries, highlighting, parsing, and benchmark support under `roottest`;
- the WebAssembly runtime bridge;
- corpus comparison and benchmark fixtures;
- grammar patches, blobs, and standalone grammar support.

Run `go run ./cmd/citestplan` to check the package inventory.
The command reads the workflow filters and the host race-build package metadata.
It rejects test packages without an execution lane.
The race jobs use the same command with `-lane` to select their packages.

The root tests, compact internal tests, and generator tests retain their dedicated jobs.
The generator job remains informational. Its presence does not establish a blocking correctness gate.

This check verifies package assignment. It does not certify individual tests or conditional assertions.
Build tags, missing fixtures, environment gates, and explicit skips still require separate checks.
The nested `cgo_harness` module and browser execution remain outside this package inventory.

## Corrections to the August audit, 2026-09-19

The sections below preserve the August audit as a historical record. Four
of its findings are now stale. Read each one together with its correction
here.

- **Race shard count.** The audit says tests land in "one of the four
  `race_root_shards`". `ci.yml`'s `race_root_shards` job now runs five
  shards (`shard: [0, 1, 2, 3, 4]`).
- **Emergency stub build.** The audit says the `gts_no_parsercorephase0`
  emergency stub build is "never built by any CI job" and that "the
  fail-closed emergency path itself is untested" (Part 2b and the
  backlog). `ci.yml`'s `compile` job now builds it
  (`go build -tags gts_no_parsercorephase0 ./...`) and runs one named
  regression test against it
  (`TestAdmissionSwitchDefaultBuildFallsBackLoudly`). The route is no
  longer completely untested, though one test does not give it full
  coverage.
- **`corpuscheck`, `wasm/runtime`, `internal/benchfixtures`, and
  `cmd/grammargen`.** Part 2d and the backlog say no CI lane reaches these
  packages, and that `cmd/grammargen` is excluded by an accident in an
  awk-based exclusion rule. `race_packages` now reaches all four:
  `corpuscheck` and `internal/benchfixtures` through the `support` lane's
  package filter, `wasm/runtime` through its own dedicated `wasm` lane,
  and `cmd/grammargen` through the `cmd` lane. `race_packages` no longer
  uses awk; `cmd/citestplan -lane <name>` assigns packages to lanes by
  regular-expression match, with no grammargen-specific exclusion. Run
  `go run ./cmd/citestplan -lane cmd` to confirm `cmd/grammargen` is in
  the list.
- **The `build` job is not a required GitHub check.** Part 3a says a
  cgo census job is listed "in the required `build` gate's `needs`" and
  that it therefore "blocks a merge when it fails". `build` does aggregate
  the other jobs' results inside `ci.yml` through `needs` and
  `require_success`, and fails when one of them fails. That aggregation
  is enforced inside the workflow file. Since 2026-09-19, branch
  protection on `main` requires the `build` check from GitHub Actions.
  Branch protection does not apply to administrators, so an administrator
  merge can still bypass a red `build`.

The following sections preserve the August audit and its historical findings.


Date: 2026-08-02. Base commit: `158d0eeb` (origin/main). Branch:
`yew/wire-dark-gates`.

## Why this exists

Four times in one day, a review found a test that exists, passes locally, and
never runs in CI: a build-tagged equivalence test invisible to the untagged
lanes; a `parser_result_test` package with no gate set running it; a
corruption case anchored to the wrong opcode; and the two env-gated
differentials this branch wires (below). A merged PR had cited two of those
differentials' results ("159 EQUAL / 0 DIVERGE") as evidence, even though
neither had ever executed in CI. This document is the coverage map so the
next worker can see the gap instead of rediscovering it, plus a systematic
sweep for the rest of the class.

## Part 1: fixed in this branch

### 1a. `TestCompatTailElisionEquivalenceSmokeExhaustive` — now wired

`GTS_COMPAT_TAIL_ELISION_EXHAUSTIVE` gated the 163(+)-language exhaustive
compat-tail elision equivalence gate
(`admission_compat_tail_elision_test.go`). It appeared nowhere under
`.github/`. Fixed: added to the existing `compact_admission_ratchet` lane
(`.github/workflows/ci.yml`), which already runs a 206-language sweep under
`-tags gts_parsercorephase0` and — checked explicitly — does not select
`TestArenaGCRetentionAfterRelease`, so the whole-process heap interaction
that forced the bounded/exhaustive split does not apply there.

- Verified locally with the lane's exact `-run` expression plus the new
  test, env vars matching `ci.yml` exactly
  (`GTS_ADMISSION_SCORECARD=1 GTS_ADMISSION_SCORECARD_RATCHET=1
  GTS_COMPAT_TAIL_ELISION_EXHAUSTIVE=1 GOMAXPROCS=1 go test -tags
  gts_parsercorephase0 . -run '...' -count=1 -timeout 10m`): all 8 selected
  tests pass, including `EQUAL=159 DIVERGE=0 NOT_ELIGIBLE=43 NOT_ROUTED=4
  ASYMMETRIC_ROUTE=0 ERROR=0 total=206` — the exact 159/0 split the merged
  PR cited.
- Timed: baseline lane (existing 7 tests) 4.22s wall; with the exhaustive
  test added, 6.85s wall. Added cost ≈2.6s, comfortably inside the
  "about 4 seconds" estimate and the stop-rule budget.
- Confirmed `TestArenaGCRetentionAfterRelease` is absent from the lane's
  `-run` selection (grep of `benchmark_warm_reuse_test.go` vs. the regex).

### 1b. `TestCompatTailElisionEquivalenceRealCorpus` — cannot run in CI; skip made loud

`GTS_ADMISSION_REAL_CORPUS` reads `cgo_harness/corpus_real/manifest.json`.
`cgo_harness/corpus_real/` is git-ignored (`.gitignore:36`) and no CI job
provisions it (no workflow or script references the path). **This gate
cannot run in CI today** — decided not to force it, per the stop rule.
Instead both of its skip paths were made loud: each now prints the reason
directly to stderr (bypassing the testing package's per-test buffering, so
it is visible in a plain compiled-binary run and is captured verbatim by
`-json`, which `race_root_shards`/`race_root_isolated` already use for every
untagged root-package test, including this one — see below), and the
messages now name the exact missing fixture path and state plainly that the
gate's results are not CI evidence.

Note: this file (`admission_compat_tail_elision_test.go`) carries no build
tag, so `TestCompatTailElisionEquivalenceRealCorpus` and
`TestCompatTailElisionEquivalenceSmokeExhaustive` were both already being
*compiled and skip-executed* inside the untagged `race_root_shards` /
`race_root_isolated` matrix (confirmed via `go test -race . -list`) — they
were reached by CI, just always skipped there. The env-gate fix above is
still required because `race_root_shards` never sets either env var, so
without the `compact_admission_ratchet` wiring the exhaustive gate still
never executed its body anywhere in CI.

### 1c. Deterministic cancellation detector — new, 20/20

`TestCompatTailElisionSurvivesConcurrentCancellation` (end-to-end, races a
goroutine's flag flip against `Parser.Parse`) catches a reintroduced
regression in `finalizeCompactReturnedTreeForParse` about 90% of the time
(reviewer's measurement: 18/20). Added, alongside it (not replacing it):
`TestFinalizeCompactReturnedTreeForParsePreservesCompletedTreeUnderPresetCancellation`
(`admission_switch_candidate_tail_cancellation_internal_test.go`, package
`gotreesitter`, `//go:build !gts_no_parsercorephase0` — the default build,
same tag as the production file it tests). It materializes a real
compact-route tree via the same `runner.parse` call
`tryCompactFullParseRoute` itself makes, stops short of that function's own
tail call, sets the cancellation flag, and only then calls the tail function
directly — no timing window. Both tail functions are exercised on their own
real tree: `finalizeCompactReturnedTreeForParse` (the function that carried
the historical bug) and `normalizeReturnedTreeForParse` (the production
oracle it must match).

**Proof (reverted the fix in a scratch copy, ran 20 times, restored):**
reverting `finalizeCompactReturnedTreeForParse` to the unconditional
stop-reason-check form made the new test fail **20 of 20** runs, always at
the same assertion, always with `ParseStopReason()="cancelled"` — the exact
regression signature. The `normalizeReturnedTreeForParse` subtest (the
never-buggy oracle) stayed green throughout, as expected. The file was then
restored to `git diff --exit-code` clean.

The new test is untagged-build-visible: `go test -race . -list` includes it,
so it lands automatically in one of the four `race_root_shards` (or
`race_root_isolated`, if later added to that manifest) without any further
CI wiring.

## Part 2: systematic audit

### 2a. `os.Getenv`/`t.Setenv` gates on a `_test.go` file, cross-referenced against `.github/`

61 distinct env-var names gate test behavior in root-module `_test.go` files
(excluding the separate `cgo_harness` module, which has its own large surface
and is called out separately below, not exhaustively). 6 are referenced under
`.github/` after this branch:

| Env var | Set in CI? | Where |
|---|---|---|
| `GOTREESITTER_GRAMMAR_BLOB_DIR` | Yes | `ci.yml` (`parity_report`) |
| `GOT_BENCH_FUNC_COUNT` | Yes | `ci.yml` (`perf-regression`) |
| `GTS_ADMISSION_SCORECARD` | Yes | `ci.yml` (`compact_admission_ratchet`) |
| `GTS_ADMISSION_SCORECARD_RATCHET` | Yes | `ci.yml` (`compact_admission_ratchet`) |
| `GTS_COMPAT_TAIL_ELISION_EXHAUSTIVE` | **Yes (this branch)** | `ci.yml` (`compact_admission_ratchet`) |
| `GTS_W5_SLOW_TIER` | Yes | `ci.yml` (`oedit_latency_gate_full`, `workflow_dispatch` only) |

The other 55 are never set anywhere under `.github/`. Most are legitimate
opt-in diagnostics (trace/debug output, report paths, config knobs) whose
absence does not skip a meaningful assertion — the base test still runs and
asserts without them (`GOT_STATS`, `GTS_ATTRIBUTION_OUT`/`_DURATION`,
`GTS_CSS_DEBUG_DFA`, `GTS_DECODE*`, `DART_H5_TRACE*`, `DIAG_TS_*`,
`GTS_C4_PASSMIX_RESULT`, `GTS_RUST_*_DIAGNOSTIC`, `GTS_ADMISSION_SCORECARD_STRICT`,
`GTS_CORE100_STRICT_SMOKE`, `GTS_SQL_SCANNER_SLOW_TIER`, `GOT_GLR_MAX_STACKS`,
`GOT_LALR_LR0_CORE_BUDGET`, `GOT_GLR_FOREST`), or are named manual
reproduction harnesses for a specific historical issue
(`GTS_REPRO_FILE`/`_INSERT`/`_DELETE`/`_FULL`/`_ORGANIC`/`_ORGANIC_DEL`/
`_WARM_PROFILE`/`_PROFILE_OUT`, all in the `issue380_*_repro_test.go` files) —
not correctness gates a PR would cite as evidence, lower priority.

The subset below **does** skip a real assertion when unset, and none of them
run in CI:

| Env var | Test(s) | Notes |
|---|---|---|
| `GTS_ADMISSION_REAL_CORPUS`(+`_MANIFEST`/`_MAX_BYTES`/`_RATCHET`) | `TestAdmissionCandidateRealCorpusMatrix` (`admission_real_corpus_matrix_test.go`) | Same fixture problem as 1b, un-loudened sibling gate — see backlog. |
| `GOT_RECOVERY_SWEEP` | `TestCRecoveryCorpusTruncationAcyclicitySweep`-class (`parser_recover_cycle_sweep_test.go`) | "slow; multi-language corpora" per its own skip message. |
| `GTS_B4B_SHADOW_CENSUS_REPORT` | Two tests across `parsercore_phase0_b4b_shadow_census_test.go` and `..._internal_test.go` | Report-generation, gated off by default. |
| `GTS_CORPORA_GRAMMAR_PARITY` | `corpuscheck/census_gts_corpora_test.go` | Compounded: the `corpuscheck` package itself is not reached by any CI lane — see 2d. |
| `GTS_FOREST_INCR` | `TestForestIncrementalCorrectness` | Reads `cgo_harness/corpus_real/...` paths — gitignored, unavailable in CI even if the var were set. |
| `GTS_GRAMMARGEN_FORTRAN_TARGETED_PARITY`, `GTS_GRAMMARGEN_SPLIT_EVAL`, `GTS_GRAMMARGEN_SPLIT_LANG_EVAL` | `grammargen/fortran_targeted_parity_test.go`, `grammargen/lr_split_real_test.go` | In `grammargen`, already documented non-blocking/advisory in `ci.yml`; lower severity by that existing design. |
| `GTS_HTTP_LOCKED_CORPUS` | `grammars/http_section_native_regression_test.go` | `grammars` package **is** run in CI; this one test inside it never asserts anything there. |
| `GTS_RUST_DOT_RANGE_CORPUS`(+`_EXPECT_FILES`) | `grammars/rust_dot_range_census_test.go` | Same shape as above: reached package, unreached test. |
| `GTS_WORK_COUNT_SOURCE`/`_RESULT`/`_FIXTURE` | `work_count_parsercore_child_internal_test.go` and siblings | Moot in practice: gated behind the `gts_workcount` build tag, which is never passed anywhere in `.github/` — see 2b. |
| `OEDIT_MEASURE` | `oedit_range_normalize_bench_test.go` | Benchmark-shaped measurement, not part of default `go test` execution regardless. |

### 2b. `//go:build` tags on test files, cross-referenced against tags CI builds with

Distinct tag expressions found (root module, counts = number of `_test.go`
files):

| Tag expression | Files | Built by CI? |
|---|---|---|
| `!gts_no_parsercorephase0` (default build) | 15 | Yes — every untagged/default lane |
| (no tag, plain default build) | (majority of the tree) | Yes |
| `!grammar_subset` / `!grammar_subset \|\| grammar_subset_X` | 35 + ~28 | Yes, as the default (`grammar_subset` is never set by CI, so `!grammar_subset` is always true) |
| `gts_parsercorephase0` (bare or combined, e.g. `&& gts_parsercorephase0a`) | 57 (root package) | Partially — see below |
| `gts_workcount` (bare or combined) | 7 + 2 combined + 2 negated | **No** — `gts_workcount` appears nowhere under `.github/` |
| `race` / `!race` | 4 + 4 | Yes, both sides (race lanes vs. non-race `compile`/`draft-correctness`) |
| `gts_no_parsercorephase0` (emergency stub build) | 1 (+1 negated combo) | **No** — never built by any CI job |
| `grammar_blobs_external`, `docker_parity`, `grammar_set_core` | 1 each | Situational — `grammar_blobs_external` is used by `parity_report`; the other two not found under `.github/` |

Headline finding: the **`gts_parsercorephase0`-tagged surface is large and
mostly unexecuted**. 57 root-package `_test.go` files carry that tag (alone
or combined), containing **277** top-level `Test`/`Fuzz`/`Benchmark`
functions. Exactly two CI jobs pass `-tags gts_parsercorephase0` to the root
package (`selected_store_admission`, `compact_admission_ratchet`), and both
use narrow, explicit `-run` allow-lists — together naming on the order of a
dozen distinct test functions. The rest of the 277 are compiled (a real
build-break there would be caught) but their bodies never execute in CI.
This is the same defect class as the two items fixed in Part 1, at a much
larger scale; it is not something this branch can respond to inside the stop
rules (would mean redesigning multiple CI lanes), so it is filed in the
backlog below rather than fixed here.

The emergency opt-out build (`-tags gts_no_parsercorephase0`, the stub route
in `admission_switch_stub.go`) is never compiled by any CI job — the
fail-closed emergency path itself is untested.

### 2c. `t.Skip`/`t.Skipf` on a missing fixture or directory: loud or silent?

~90 such skips were found (root module + `grammargen` + `grammars`,
excluding `cgo_harness`). Nearly all already carry a reason string, so by the
strict "has a logged reason" reading almost none are silent — but under
plain `go test` (no `-v`), Go's test tool suppresses **all** output
(including any reason string) from a package that ends up passing, skip
included; the message becomes visible only under `-v`, under `-json` (which
`race_root_shards`/`race_root_isolated` already use for the untagged root
package), or when the test fails. That is normal Go behavior, not a bug
introduced here, but it is exactly the mechanism that let the two Part-1
gates go unnoticed. The loudening fix in 1b (`fmt.Fprintln(os.Stderr, ...)`
before `t.Skip`) is the direct countermeasure and is only applied to that
one gate so far; the sibling gate `GTS_ADMISSION_REAL_CORPUS` in
`admission_real_corpus_matrix_test.go` has the identical shape and is not
yet loudened (backlog below).

**Separate, more serious finding: hardcoded developer-machine paths.** Five
test files reference an absolute path under `/home/draco/...` and `t.Skipf`
when it is absent:

| File | Path referenced |
|---|---|
| `parser_result_go_compat_byte_identity_test.go` | `/home/draco/work/gotreesitter-corpora/corpus_sources/{go,fidl}/.../zerrors_windows.go` |
| `parser_recover_cycle_sweep_test.go` | same (shares `zerrorsCandidatePaths`) |
| `grammars/csharp_external_lex_states_regression_test.go` | `/home/draco/work/gotreesitter-corpora/corpus_sources/c_sharp/.../DeclaredTypeManager.cs` |
| `grammargen/markdown_grammar_test.go` | `/home/draco/work/mdpp/examples/conformance` |
| `grammargen/markdown_grammar_corpus_test.go` | `/home/draco/work/mdpp/testdata/cst-snapshots` |

These are not env-gated at all — there is no variable a CI workflow could
set to enable them. They can only ever run on the machine(s) that happen to
have those exact paths, i.e. effectively never in CI and only for whichever
engineer's checkout layout happens to match. `TestGoZerrorsNormalizerByteIdentity`
and `TestCRecoveryZerrorsTruncationAcyclic` in particular are named
regression tests for a documented historical defect ("hung forever",
"O(n^2) hardening") — real regression coverage that is currently
unreachable by anyone but the original author. See backlog, high severity.

### 2d. Package directories whose tests are not reached by CI lane package selections

Cross-referencing `go list ./...` against every package-selecting `-run`/
package-filter pattern in `ci.yml` (the same awk logic `race_packages` uses,
run locally against this branch):

| Package | Test files | Reached by any CI lane? |
|---|---|---|
| `corpuscheck` | 5 (`census_test.go`, `census_gts_corpora_test.go`, `compare_test.go`, `format_test.go`, `sexpr_test.go`) | **No.** Compiled only (`go build ./...`, `go vet ./...`, `go test ./... -run '^$'` in `compile`); no lane's package filter matches `corpuscheck` (only `corpuscheck/cmd/corpuscheck` matches `/cmd/`, a different package). |
| `wasm/runtime` | 3 (`blob_test.go`, `bridge_test.go` [260 lines], `incremental_test.go`) | **No.** Same as above — compile-checked only. |
| `internal/benchfixtures` | 2 (`deep_digest_test.go`, `go_fixtures_test.go`) | **No.** Notable: this package's `InspectGoTree` digest function is what dozens of *other* tests (including the ones fixed in Part 1) trust as their oracle; its own test suite is compile-checked only. |
| `cmd/grammargen` | 2 (`commands_test.go`, `grammar_js_cli_test.go`) | **No — and this one is an accident.** `race_packages`'s `cmd` lane filter is `/cmd/`, which should match, but the same job's exclusion rule `$0 ~ /\/grammargen$/ { next }` — intended to keep the advisory top-level `grammargen` package out of the blocking suite — also matches `cmd/grammargen` on the same string suffix and drops it too. |
| `parser_result_test` | — | Already fixed (prior PR): matched by the `parser-result` lane (`/parser_result_test$`). Confirmed present and passing below. |
| `internal/parsercorephase0` | many | Reached — explicit `go test -race ./internal/parsercorephase0` in `selected_store_admission` (both tag variants). |
| `taproot`, `taproot/diag`, `taproot/walk`, `grep` | — | Reached — `support` lane (`/grep$\|/taproot($\|/)`), confirmed via the exact awk match. |

`cmd/grammargen`'s exclusion is a one-line, low-risk wiring fix (change the
exclusion rule to match the top-level `grammargen` package exactly instead of
by suffix); it is filed in the backlog rather than made in this branch to
keep this PR's CI-config surface to the two items in scope, per the stop
rule ("if wiring ... makes ANY test in it fail ... stop and report") — this
fix touches a shared `race_packages` matrix entry outside the two named
targets and deserves its own verification pass.

### `cgo_harness` (separate Go module) — not audited to the same depth

`cgo_harness/go.mod` makes it a separate module with its own, much larger
surface (94 files tagged `cgo && treesitter_c_parity`, plus several smaller
combinations). The `parity-cgo`, `parity-cgo-exhaustive`, and
`compact-t3-oracle-cgo` jobs each pass narrow, explicit `-run` allow-lists
into that module, the same shape as the `gts_parsercorephase0` finding above.
Given this branch's scope and the stop rules, this was noted but not
quantified test-by-test; a dedicated audit pass is filed in the backlog.

## Part 3: later additions

### 3a. `merge-event-census-cgo` — wired with the census it runs (2026-08-02)

Stage M0 of `spec.merge-time-election.v1` added two cgo tests,
`TestMergeEventCensusBaseline` and `TestMergeEventCensusFooIntWitness`
(`cgo_harness/merge_event_census_test.go`). Both sit behind the new
`gts_merge_census` build tag, so no existing lane could reach them: every cgo
parity lane passes a fixed `-run` allow-list AND a fixed `-tags` string, and
`run_parity_in_docker.sh` hardcodes `-tags treesitter_c_parity` in its default
command.

Fixed in the same change that added the tests: a new `merge-event-census-cgo`
job uses the docker runner's custom-command form to pass
`-tags 'cgo treesitter_c_parity gts_merge_census'`, and the job is listed in
the required `build` gate's `needs` and in its `require_success` checks. The
census therefore blocks a merge when it fails.

### 3b. The stage-D0 derivation-set differential is a DARK GATE and its pin has drifted

`TestDerivationSetDifferentialBaselineCensus` and
`TestDerivationSetDifferentialWitnessReproduction`
(`cgo_harness/derivation_set_differential_test.go`, PR #646) need
`-tags gts_derivation_set_census`. No workflow passes that tag. Searching
`.github/` for `gts_derivation_set_census` returns nothing.

The consequence is already measurable. On unmodified `main` at `56410214`,
the baseline census reports 24 set differences against a pin of 32, so the
test FAILS:

```
TOTAL SET-DIFFERENCE COUNT (constructed)=24 (all corpora)=24
D0 baseline total set-difference count is 24, pinned at 32
```

The drift is a FALL, which is the direction stages D1 and D2 want, but it
landed with no receipt because nothing ran the gate. Re-pinning belongs to the
owner of that lane with the per-witness adjudication the file's own comment
demands, so this branch reports the drift and does not re-fit the number.

### 3c. `TestOutlineOracleDifferential` joins `parity-cgo`'s fixed `--run` regex (2026-08-11)

The outline C-oracle differential (`cgo_harness/outline_differential_test.go`,
`spec.outline-api.v1` gate G-2) now appears in the `parity-cgo` job's fixed
`--run` regex (`.github/workflows/ci.yml`), next to its query/capture-parity
sibling `TestParityHighlight`. The job runs on pull requests and pushes to
`main` that the `exhaustive_parity_scope` gate selects for code CI. The
test's `CoreNine` subtest hard-gates the nine core languages. Its
`FleetCensus` subtest only logs a census row for every other language.
Naming the whole test in the regex is safe.

## Backlog (not fixed in this branch)

| Item | Severity | Why |
|---|---|---|
| `cmd/grammargen` accidentally excluded from `race_packages` by a suffix-matching exclusion rule meant for top-level `grammargen` only | Medium | One-line fix, but shares a matrix entry with other lanes; needs its own verification pass per the stop rule. |
| `corpuscheck`, `wasm/runtime`, `internal/benchfixtures` packages reached by no CI lane (compile-checked only) | Medium–High | `internal/benchfixtures` is the digest oracle many other tests trust; its own correctness is unverified by CI. |
| Hardcoded `/home/draco/...` absolute paths in 5 test files, including two named historical-regression tests (`TestGoZerrorsNormalizerByteIdentity`, `TestCRecoveryZerrorsTruncationAcyclic`) | High | Not just uncovered — unreachable by CI or any engineer without that exact local layout; no env var exists to opt in. |
| `gts_parsercorephase0`-tagged surface: 277 top-level test functions across 57 root-package files, only on the order of a dozen actually executed by CI's narrow `-run` allow-lists | High (scale) | Same defect class as the two fixed items, much larger; needs a lane redesign, not a one-line change. |
| `gts_workcount` build tag never passed by any CI job | Medium | Entire work-count differential harness (9+ files) compiled nowhere, run nowhere. |
| `gts_derivation_set_census` build tag never passed by any CI job; its pinned baseline has already drifted from 32 to 24 on `main` | High | See 3b. The gate exists, fails on `main`, and nothing reports it. |
| `gts_no_parsercorephase0` (emergency stub) build never compiled by CI | Medium | The fail-closed emergency route itself is untested; would only be discovered broken during an actual incident. |
| `GTS_ADMISSION_REAL_CORPUS` in `admission_real_corpus_matrix_test.go` has the same silent-by-default shape as the gate loudened in 1b, not yet loudened itself | Low–Medium | Same fixture problem (`cgo_harness/corpus_real`), same fix shape as 1b; not touched here to keep this branch's diff minimal. |
| `cgo_harness` module's own `gts_parsercorephase0`-style narrow-`-run`-vs-broad-tag-surface pattern (94 `treesitter_c_parity`-tagged files) | Unquantified | Flagged, not measured; separate module, separate audit warranted. |

## Unverified-claim findings

The only confirmed case of a **merged PR citing results from a gate that
never ran in CI** is the one this branch fixes (1a/1b): `159 EQUAL / 0
DIVERGE` for the exhaustive compat-tail elision gate, and the (still
CI-unreachable) real-corpus variant's results. No other specific merged-PR
citation was traced during this audit — the `gts_parsercorephase0` and
`gts_workcount` findings above are coverage gaps, not confirmed cases of a
specific cited claim, but given their scale they carry the same risk and are
flagged at High/Medium severity above rather than left implicit.

## Part 4: `phase0_tagged_suite` lane (2026-09-03, issue #1052)

A review of PR #1051 found that two of its new tests never ran in CI. The
two existing tagged lanes (`selected_store_admission`,
`compact_admission_ratchet`) each use a fixed, narrow `-run` allow-list, the
same pattern the "Headline finding" above already named at scale. This
section closes most of that gap with one new job.

Counts, root package, `-tags gts_parsercorephase0` against the default
(untagged) build:

- Tag-exclusive top-level functions (`go test -tags gts_parsercorephase0 .
  -list '.*'` minus the untagged `-list '.*'`): 361 — 349 `Test` functions,
  12 `Benchmark` functions.
- The 12 `Benchmark` functions stay out of scope: `-run` (what every tagged
  lane in this file uses) never selects a benchmark; only `-bench` executes
  one. They need their own gate design, not this lane.
- Of the 349 tag-exclusive `Test` functions, 15 already ran, matched by the
  three existing `-run` allow-lists (`selected_store_admission`,
  `compact_admission_ratchet`, and the `compact-t3-oracle-cgo` job's
  per-version-lexer-Scala Docker step). 334 did not run anywhere: set U.
- `phase0_tagged_suite` now runs 322 of the 334. Measured together, in one
  process, GOMAXPROCS=1: 86.6s wall, 1.5 GiB max RSS.
- 12 of the 334 are excluded, by name, with a one-line reason in the job's
  YAML comment:
  - 11 fail today on `main`, each with a genuine assertion mismatch (for
    example, `TestAdmissionCandidateElmHighlightDirect`: digest mismatch;
    `TestG18DropPathRejectsForeignInlineRED`: "RED: foreign inline lineage
    certified a current-arena drop"). These are pre-existing defects, not
    caused by this lane; fixing them is out of scope for this maintenance
    change.
  - 1 (`TestG18D6aProducerTelemetry`) passes alone but fails when run with
    the other 321: it compares a captured D3 runtime pool baseline against
    one carried over from earlier tests in the same process, so it is
    order-dependent on accumulated global state. Excluded rather than
    reordered, since reordering risks moving the same defect onto a
    different test.
- After this change: 337 of 349 tag-exclusive `Test` functions run in CI
  (15 previously wired plus 322 new), up from 15. 12 remain excluded for the
  reasons above.

This does not close the `cgo_harness` module's own version of the same
finding (Part 2d, "unquantified"), and it does not touch the `gts_workcount`
or `gts_derivation_set_census` tags, or the `gts_no_parsercorephase0`
emergency stub build — all still in the backlog above.

## Part 5: harness image pinning and build-once (2026-09-19)

Run 35469222709 on `main` failed in `compact-t3-oracle-cgo`. Its last
step, `docker build`, pulled `node:22-bookworm` from Docker Hub and hit an
auth-token read reset. All tests in that job had already passed. The flake
sat in the image pull, not the test suite. This section records the fix.

### Base images pinned by digest

`cgo_harness/docker/Dockerfile` now pins `node:22-bookworm` and
`golang:1.25-bookworm` by digest instead of by floating tag. A digest pin
makes the pull content-addressed, so a registry retry fetches the same
bytes instead of racing a tag that can move. Digests were resolved
2026-09-19 with `docker buildx imagetools inspect <image>:<tag>`.

Refresh procedure (one command per base image, then update the `FROM`
line and its resolution-date comment):

```sh
docker buildx imagetools inspect node:22-bookworm --format '{{json .Manifest}}' \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["digest"])'
```

`tree-sitter-cli` stays pinned at `0.24.7`. Neither `cgo_harness/README.md`
nor `BENCH.md` binds this CLI version to the 0.25 runtime oracle. Those
docs name the parity runtime only: `go-tree-sitter v0.25.0` and upstream
runtime `v0.25.1`. The ABI negotiation in `parity_c_loader_cgo.go` targets
ABI 15 first. It falls back through lower ABI values when the installed
CLI cannot emit ABI 15. `tree-sitter-cli` 0.24.7 works inside that
fallback range today. A CLI version bump needs its own parity validation
pass. That work is out of scope for this network-flake fix.

### Build once per job

Jobs that call `cgo_harness/docker/run_parity_in_docker.sh` more than once
used to rebuild the image on every call. `run_parity_in_docker.sh` now
takes a `--build-only` flag that builds the image and exits without
starting a container. Each multi-call job (`compact-t3-oracle-cgo`,
`parity-cgo`, `parity-cgo-exhaustive`) runs one "Build cgo harness image"
step with `--build-only`, then passes `--no-build` on every later call in
that job. Single-call jobs (`merge-event-census-cgo`,
`perf-scan-gate.yml`'s `fleet` job) are unchanged.

### Retry policy for transient pulls

`run_parity_in_docker.sh` wraps `docker build` in a bounded retry: three
attempts, with a 10-second wait before the second attempt and a
30-second wait before the third. It retries only on a known network-flake
failure class. That class covers `connection reset`, `TLS handshake`,
`failed to resolve source metadata`, `i/o timeout`, `503`, and `429`. Any
other failure — a bad Dockerfile line, a failed
`RUN` step, disk space — exits at once instead of retrying blindly.
`cgo_harness/docker/run_parity_in_docker_test.sh` covers the retry-and-
succeed, fail-fast, and retries-exhausted paths with a mock `docker`
binary.

### Action pins

Every third-party action referenced from `.github/workflows/*.yml` is
pinned to a commit SHA, with a `# vN` comment for the human-readable
version:

| Action | SHA | Tag |
|---|---|---|
| `actions/checkout` | `11d5960a326750d5838078e36cf38b85af677262` | v4 |
| `actions/setup-go` | `40f1582b2485089dde7abd97c1529aa768e1baff` | v5 |
| `actions/cache` | `0057852bfaa89a56745cba8c7296529d2fc39830` | v4 |
| `actions/upload-artifact` | `ea165f8d65b6e75b540449e92b4886f43607fa02` | v4 |
| `peter-evans/create-pull-request` | `c5a7806660adbe173f04e3e038b0ccdcd758773c` | v6 |

Resolve a new SHA for a tag with `gh api
repos/<owner>/<repo>/git/ref/tags/<tag>`. Each tag above already points
straight at a commit. No dereference step was needed.
