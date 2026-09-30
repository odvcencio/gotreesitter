# PR 1396 verification

The fix preserves zero-width external tokens and lets an unchanged scanner
marker select the accepting GLR branch. It drops an incompatible sibling only
when another sibling accepts, no whitespace was consumed, and the nonempty
serialized scanner state is unchanged. State-changing layout transitions keep
their existing per-version handling. The PureScript grammar certification and
layout boundaries from the submitted PR remain intact.

The comparison baseline is the submitted PR, `2380ce9b6`. The engine correction
is `d267d2d1b`; `3cea6fa59` refreshes two expectations separately. The dynamic-scope C regression is `f8e7a9d79`; `be4aead48` refreshes its
measured census pin. `1ee4453cf` isolates the CI commit lists. No thresholds,
allowlists, grammar locks, or deterministic counter ledger pins changed.

## Correctness evidence

- The four PureScript layout regressions match the locked C tree on default and
  compact routes: symbols, fields, byte and point spans, error/missing flags,
  and deep digest.
- Perl's short grep, parenthesis-recovery, and dynamic-scope witnesses match the complete
  locked C tree on both routes. The corrected call in the full map-grep fixture
  also matches C. A pre-existing list-expression mismatch later in that full
  fixture remains; this is not a claim of complete map-grep C parity.
- The 333-test `phase0_tagged_suite` selection passes. The seeded map-grep gate
  passes without skipping, and the K–P grammar/runtime race shard passes.
- The complete deterministic ledger passes: 412 rows, 206 languages, unchanged
  2% threshold. The six common PureScript, Perl, and Haskell sample rows are
  byte-for-byte identical before and after, including decline reasons.

| Sample / route | Full tokens before → after | New nodes before → after | Peak stacks before → after | Edit nodes before → after |
| --- | ---: | ---: | ---: | ---: |
| PureScript default | 171 → 171 | 414 → 414 | 1 → 1 | 450 → 450 |
| PureScript compact | 171 → 171 | 449 → 449 | 1 → 1 | 450 → 450 |
| Perl default | 91 → 91 | 258 → 258 | 1 → 1 | 266 → 266 |
| Perl compact | 91 → 91 | 266 → 266 | 1 → 1 | 266 → 266 |
| Haskell default | 476 → 476 | 1282 → 1282 | 4 → 4 | 1342 → 1342 |

Reuse bytes and block splices stay at zero in these edit samples. Haskell's
compact sample keeps the same existing live-link-cap decline (`9 > 8`).

The fresh-parity comparison visits all 206 registered grammars, one language
per process, selecting the four largest locked corpus files at or below 1 KiB.
C and corpus inputs are available for 186 grammars, covering 703 files on both
routes. Default C matches increase from 574/703 to 575/703; compact matches
remain 576/703. No previously passing C match, parse success, or root coverage
is lost. Perl's `ext/Opcode/ops.pm` gains the default C match. The other 20
registrations have the same unavailable-input result before and after.

The corpus lock was fetched outside the repository and verified as SHA-256
`41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`.
The oracle uses tree-sitter runtime 0.25.1 at
`f5afe475deb7c0bae6407fb776c76824f717bb61`, the repository's locked C grammar
revisions, and go-tree-sitter v0.25.0. The pinned Perl checkout lacks
`parser.c`; the existing loader generates it with tree-sitter 0.24.7 in the
writable temporary checkout. CI now permits that generation.

## Expectations requiring owner approval

The two `update(pins)` commits are separate from the engine fix:

- Map-grep declaration digest: `633141732a3b` → `20159a4cbccd`. The default
  tree now selects C's `function_call_expression` at bytes `[229,235)` rather
  than `ambiguous_function_call_expression`; both routes agree.
- Recovery native-accept/fallback deltas with rescue disabled: `0/1` → `1/0`.
  Both routes match C's complete recovery tree. Clean-tree assertions and the
  rescue-enabled twin remain.

`be4aead48` refreshes Perl's no-packed-head refusal census from 46 to 43.
The sole changed witness is `local_dynamic_scope`: the default tree gains
complete locked C parity, and its three incompatible siblings no longer reach
merge refusals. Default C matches on the 17 Perl census sources increase
11/17 → 12/17; compact remains 12/17. The other 16 tree comparisons stay
identical. C merges remain 57, Go merges remain 0, and over-merge sources
remain 0. All other pins and aggregate merge totals stay fixed. The complete
merge census and Foo[int](a) witness gate pass with the refresh.

## Rejected candidates

| Candidate | Evidence for rejection |
| --- | --- |
| Preserve only state-changing zero-width tokens | Perl locked-corpus fresh C matches fall from 2/4 to 1/4. |
| Drop incompatible siblings for every zero-width external token | Less peak live versions rise from 6 to 7, failing the existing ledger. |
| Require no skipped whitespace but ignore scanner-state changes | One previously passing Djot default C match is lost. |

The final scanner-state condition avoids all three regressions.

## Reproduction

Run each correctness command with `GOWORK=off` inside
`cgo_harness/docker/run_parity_in_docker.sh`. Seed locked Perl and PureScript
repositories with `cgo_harness/seed_parity_repos.sh` first. Run each language's
C and edit-session checks in its own process.

```sh
GOWORK=off go test ./grammars ./grammars/runtime -race -run '^Test[K-P].*$' -count=1
GOWORK=off go test . -tags gts_parsercorephase0 -run '^TestAdmissionCandidatePerlMapGrepRoutesCompactByDefault$' -count=1
GOWORK=off go test ./grammars -run '^TestPurescriptDeriveInstanceWhereLayout$' -count=1
# From cgo_harness, one language per command:
GOWORK=off go test . -tags 'treesitter_c_parity gts_parsercorephase0' -run '^TestPerlZeroWidthMarkersSelectLockedCBranches$' -count=1
GOWORK=off go test . -tags 'treesitter_c_parity gts_parsercorephase0' -run '^TestPurescriptLayoutTransitionsLockedC$' -count=1
```

The full-module command runs in Docker with the linked worktree's common Git
metadata mounted read-only and `/workspace` configured as a safe directory.
Initial broad reruns hit disk exhaustion; a separate initial lifecycle failure
came from missing common Git metadata. Neither condition changed source or
assertions; both commands were rerun with the environment corrected.

## Edit-session and broad-test limits

The R4 gate ran for all 206 registered languages, one process per language,
using 72 edits. It passed for 196 within the initial three-minute diagnostic
process limit. Godot Resource and Nickel passed focused repeats with more
diagnostic time: 178.44 and 162.18 seconds, respectively, versus 151.12 and
152.13 seconds on the baseline. Both retain zero no-edit allocations and
incremental/fresh equality at every step. No parser budget, gate threshold,
assertion, or fixture changed for these repeats.

The result is 198 passing languages and eight failures or timeouts reproduced
on the submitted baseline:

- AWK, JavaScript, Meson, and Twig reproduce the same failing edit step and
  identical incremental/fresh digest pair before and after.
- Elsa, Haskell, Kotlin, and PowerShell time out in the same existing
  missing-node dependency traversal on both revisions.

The smaller locked-corpus receipts for PureScript, Perl, and Haskell all
pass their 72-step Go incremental/fresh invariant. Existing C incremental
mismatches in those receipts remain; Go incremental/fresh equality is not
claimed to establish C incremental parity.

The broad root-module `GOWORK=off go test ./... -count=1 -timeout=35m` run
passed the root package and every touched package. It also exposed four
existing `grammargen` failures, all reproduced by a focused baseline run:
`TestImportedKotlinSwiftGrammarConstructors` (Swift rule count 306, expected
299), `TestMarkdownGrammarCommonMarkParity`,
`TestMarkdownFencedCodeBlockContentParity`, and
`TestShippedBlobsNonTerminalAliasMapInventory` (11 uncertified languages).
The overall broad sweep hit its 45-minute container wall limit after those
failures; it is not counted as a passing full-module sweep.

A broad untagged `cgo_harness` module sweep passed its actual harness and tool
packages, then failed on `corpus_structural/go_sample.go` because that parsing
fixture has an undefined `Language`. The submitted baseline reproduces this
compile error. The new locked C tests and the complete merge-census gate
pass separately with their required tags.

## Published receipt

[Raw measurements and comparison summaries](receipts/pr1396/) include both
20-seed campaigns, their benchstat reports, common before/after counters,
fresh-parity and R4 summaries, and `/usr/bin/time -v` resource measurements.
The primary timing campaign uses clean, fixed commits throughout. The repeat
spans a test-only census pin commit; its engine files remain identical to
`d267d2d1b` throughout. The raw metadata records that difference.

The code/pin head `be4aead48` passed all GitHub checks on September 30, 2026:
71 successes and three skips from existing workflow conditions. The required
`build` aggregate passed in
[CI run 36664379639](https://github.com/odvcencio/gotreesitter/actions/runs/36664379639).
No new PR was opened and no merge was performed.

## Performance protocol

The primary comparison uses `scripts/run_randomized_benchmarks.sh` with
20 paired, alternating shuffle seeds (1–20), `GOMAXPROCS=1`, `-count=1`,
`-benchtime=750ms`, and `-benchmem`, in Docker pinned to one CPU. Both inputs
use the submitted PR's grammar artifacts. The before revision is `2380ce9b6`;
the after engine source is `d267d2d1b`.

| Benchmark | Median ns/op before → after | B/op before → after | Allocs/op before → after |
| --- | ---: | ---: | ---: |
| Full Go DFA | 17,613,309 → 16,147,747.5 | 1303.5 → 1302.5 | 8 → 8 |
| Single-byte incremental edit | 309,013 → 334,896 | 390 → 391 | 5 → 5 |
| No-edit incremental parse | 15.41 → 15.78 | 0 → 0 | 0 → 0 |

Benchstat reports no significant full/no-edit timing change. The edit change
is +8.38% (`p=0.028`), below the unchanged 10% ratchet. Its +0.26% bytes/op
change does not add allocations. The correctness tradeoff is retaining C's
branch selection rather than preserving an incorrect ambiguous-call tree.
A second independent 20-seed campaign (101–120) does not reproduce a
significant timing regression (`p=0.512` for edits). Its medians are:

| Benchmark | Median ns/op before → after | B/op before → after | Allocs/op before → after |
| --- | ---: | ---: | ---: |
| Full Go DFA | 13,909,474 → 10,840,968.5 | 1289 → 1280 | 8 → 8 |
| Single-byte incremental edit | 267,937.5 → 220,356 | 389.5 → 389 | 5 → 5 |
| No-edit incremental parse | 13.955 → 11.555 | 0 → 0 | 0 → 0 |

The broad timing intervals show variation during this campaign. Both runs
are retained; neither establishes a causal speedup. The measured directional
regression in the first run remains reported rather than discarded.

A separate resource probe uses the existing full-DFA benchmark with
`GOT_BENCH_FUNC_COUNT=24500` and `-benchtime=1x`. `/usr/bin/time -v` measures
RSS around the compiled test binary, excluding compiler memory. Both inputs
parse all 1,031,294 bytes successfully. Peak RSS is 147,796 → 147,504 KiB
(146.8 → 146.5 bytes per source byte), with 136,348,112 B/op and 613
allocs/op on both. This single-iteration probe supplies resource evidence;
it is not randomized latency comparison evidence.
