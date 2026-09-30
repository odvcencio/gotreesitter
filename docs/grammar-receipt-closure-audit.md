# O4 receipt and E-C closure audit

This audit checks issues [#1324](https://github.com/odvcencio/gotreesitter/issues/1324)
and [#1321](https://github.com/odvcencio/gotreesitter/issues/1321) against main
`f4fab3296a4c8ef322e301f433c546ab0a5bd098`. The receipt fix is commit
`bf339cf79f473d716a9b8c48410ce1c446bd1398`. It changes receipt generation,
reporting, workflow cadence, and their tests. It changes no parser code,
counter pins, or gate thresholds.

## O4 findings and fix

[PR #1361](https://github.com/odvcencio/gotreesitter/pull/1361) added the
versioned generator and local batch. [PR #1372](https://github.com/odvcencio/gotreesitter/pull/1372)
made hosted receipt jobs available. Both merged, but three gaps remained:

- The schedule was weekly (`41 9 * * 1`). It now runs every day at 09:41 UTC
  (`41 9 * * *`). Nightly and published-release plans both contain exactly
  the 206 default locked grammars plus opt-in Lean, one grammar per matrix job.
- The summary returned success with zero receipts. The workflow now passes
  its planned matrix to the summary, which requires exactly one receipt per
  selected grammar. Missing, duplicate, unexpected, malformed, or wrong-schema
  receipts cannot satisfy coverage. Failed summaries still publish their
  available evidence. Recorded parity failures, unavailable samples, and
  timeouts count as receipts, never as successful graduation evidence.
- Incremental results required Go to match C incremental as well as fresh C.
  The September 29 owner decision makes fresh locked C canonical and keeps
  D8. The verdict now requires Go incremental to equal fresh Go, match fresh
  C, and pass the invariants. C incremental digests, equality flags, and
  failures remain separate evidence in the JSON and diagnostic report.

The smallest coverage witness is an empty report directory. On audited main,
`python3 scripts/summarize_grammar_receipts.py --report-dir "$REPORT_DIR"`
returned zero and wrote `Receipts: 0 (target 207)`. With an expected matrix
containing Go, the fixed command returns nonzero and reports `missing: go`.
The regression tests also distinguish a Go tree matching only C incremental
from a tree matching canonical fresh C; only the latter can pass with D8 and
all invariants satisfied.

## Verification

Both clean revisions generated all 207 receipts: 206 default grammars and
opt-in Lean. Each process handled exactly one grammar. The selected corpus
contained 707 files and 13,464 edit-session steps.

| Result | Main | Receipt fix |
| --- | ---: | ---: |
| Receipts generated | 207 | 207 |
| Fresh parity pass / fail / unavailable | 131 / 56 / 20 | 131 / 56 / 20 |
| Incremental parity pass / fail / unavailable | 38 / 149 / 20 | 38 / 149 / 20 |
| Invariant pass / fail / unavailable | 185 / 2 / 20 | 185 / 2 / 20 |
| Matched incremental steps | 4,889 | 4,891 |
| Recorded timeouts / generator errors | 0 / 0 | 0 / 0 |

All 55,270 canonical Go, fresh-C, and source digest comparisons were
identical. All grammar identities, C identities, and corpus file identities
matched. No receipt status regressed. The two newly accepted steps are Elm
insertion step 1 (`reactor/src/NotFound.elm:site-00`) and Squirrel insertion
step 8 (`samples/class.nut:site-07`). In both, Go incremental, fresh Go, and
fresh locked C already matched. Only C incremental differed. The fix retains
those exact disagreements as separate evidence rather than rejecting the
correct canonical result.

The candidate records 146 C incremental diagnostic steps. RST had 29
C-incremental-only digest differences between runs; Go and fresh C did not
change. Three replays of each unchanged binary produced three distinct C incremental
sessions per binary, while all six canonical Go/fresh-C/source sessions were
identical. The C incremental variation therefore reproduces independently
of the receipt change and remains diagnostic evidence.

The two invariant failures were unchanged HTML and PowerShell D8 cases,
already covered by [PR #1390](https://github.com/odvcencio/gotreesitter/pull/1390)
and [PR #1394](https://github.com/odvcencio/gotreesitter/pull/1394). These
receipts record unresolved parser work rather than hiding it.

The candidate's summed generator elapsed time was 260.14 seconds; its peak
per-process generator RSS was 159,732 KiB. These bounded tooling figures
include grammar/oracle setup and are not Go/C parser benchmarks.

The authenticated corpus lock was fetched outside the repository from
`GTS_CORPUS_LOCK_URL`. Its SHA-256 is
`41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`.
All 206 corpus checkouts matched their pinned revisions. Lean used its
separate checked-in opt-in locks. The current locked C runtime is 0.25.1 at
`f5afe475deb7c0bae6407fb776c76824f717bb61`; upgrading it is separate work in
[#1325](https://github.com/odvcencio/gotreesitter/issues/1325).

- `actionlint .github/workflows/grammar-receipts.yml` passed (actionlint 1.7.7).
- `python3 scripts/grammar_receipts_test.py` passed, including nightly/release
  coverage, missing/duplicate/unexpected receipts, malformed/wrong-schema
  files, failed parity, unavailable samples, timeouts, and separate C evidence.
- Docker `GOWORK=off go test ./cmd/citestplan ./internal/grammarreceipt -count=1`
  passed.
- Docker `GOWORK=off go test ./cmd/gts_grammar_receipt -tags treesitter_c_parity -count=1`
  passed from the cgo harness module.
- Docker `GOMAXPROCS=1 GOWORK=off go run ./cmd/perfcounterledger` passed:
  `counter ledger passed: 412 rows, 206 languages`. The pinned ledger is
  unchanged, SHA-256
  `057cba93bd791d995f814e404516c1a24b232f2b8450fa3cb4546dae337c6615`.
- Docker full `GOWORK=off go test ./... -count=1 -parallel=1 -timeout=60m`:
  completed with only the four pre-existing grammargen failures below.
  The root package passed in 373.002 seconds. No container OOM or wall
  timeout occurred.
- The four grammargen failures reproduced on untouched main with
  `GOWORK=off go test ./grammargen -run '^(TestImportedKotlinSwiftGrammarConstructors|TestMarkdownGrammarCommonMarkParity|TestMarkdownFencedCodeBlockContentParity|TestShippedBlobsNonTerminalAliasMapInventory)$' -count=1`.
  Swift reports 306 rules against a 299 expectation; the two Markdown tests
  disagree with the bundled parser; alias-map inventory reports ten uncovered
  languages. No expectation was refreshed to hide these failures.

All heavy work used two Docker containers, with per-grammar receipt processes
serialized in the second container. Receipts and the authenticated corpus
lock remain outside the repository. Lean's first attempt found no `origin`
remote in the fetched checkout; setting the URL from its lock and retrying
produced its receipt.

The first broad attempt could not read the shared Git history from a Docker
worktree mount. The next exposed Git's ownership check under container root.
Mounting the common Git directory and trusting only the two mounted
repositories fixed the setup; the lineage regression passed independently.
The final full gate was repeated rather than suppressing that test.

A successful hosted smoke run already verified artifact upload:
[run 36660832857](https://github.com/odvcencio/gotreesitter/actions/runs/36660832857).
Its three downloaded receipts have the locked C commit and grammar blob
SHA-256 identities. The fixed coverage summary accepts all three and reports
fresh parity 3/3, incremental parity 2/3, and invariants 3/3. These are that
run's results, not graduation claims or current engine benchmarks.

No engine optimization was made, and no Go/C speedup is claimed. Receipt
runner timings measure evidence generation, not parser performance. The
receipt comparison checks tree/source digests rather than randomized timing.

## E-C remains open

[PR #1350](https://github.com/odvcencio/gotreesitter/pull/1350) merged capability
admission and staged reservations, but expressly left the 10% fallback target
unmet. Its historical 20-seed TypeScript median-file overheads were 39.0%,
184.9%, and 215.5%; YAML largest was 24.6%, with median-file overheads of
92.3%, 115.8%, and 102.6%. These are published PR measurements, not new timing
claims for audited main.

The current E-A sanity test still skips declined/fallback inputs and uses a
6.0 ratio ceiling. Focused Docker runs confirmed the YAML sample is skipped
with `routed=0 fallback=1` and a `no_action` decline. The TypeScript sample
was accepted and passed with candidate/legacy ratio 1.527 over the test's 25
interleaved iterations. This is diagnostic gate output, not a randomized
benchmark comparison or a Go/C measurement.
This does not establish the issue's exit requirements. The closure audit was
posted in [issue #1321](https://github.com/odvcencio/gotreesitter/issues/1321#issuecomment-5903604816).

## Integration and approval

The receipt fix must be integrated before #1324 can close. A complete
nightly/release artifact run on the integrated workflow remains to be
confirmed. This audit does not certify the failing or unavailable grammars;
cohort graduation remains separate. #1321 stays open for early-decline
performance work and declined-file E-A exit evidence.

NEEDS-APPROVAL: no pin or threshold refresh. The branch is pushed for lead
integration; no PR was opened, following the owner's autonomy rule.
