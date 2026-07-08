# CI proposal: nightly Go-vs-C perf scoreboard (non-blocking)

Status: PROPOSAL ONLY for the expensive nightly sweep. The fast PR lane only
validates the checked-in budget schema and the checker unit tests; it does not
run real-corpus timing. The job below is written as a comment block for the
coordinator to adopt (or adapt) once the corpus-provisioning question is
settled.

## Job shape

- Nightly schedule, `continue-on-error: true` end to end — the scoreboard is
  telemetry, not a gate. Regressions become visible in the uploaded artifact
  and (optionally) a tracking issue, never a red main.
- Single dedicated runner, no test parallelism inside the job (`-p 1`, one
  language subprocess at a time) — timing fidelity requires an uncontended
  box. The scan auto-marks `contended=true` from loadavg if that assumption
  breaks, so polluted numbers are self-labeling.
- Reuses the parity container discipline: the suite refuses to run outside a
  container unless `GTS_PARITY_ALLOW_HOST=1`.

```yaml
# --- PROPOSED .github/workflows/perf-scan-nightly.yml (do not commit from here) ---
# name: perf-scan-nightly
# on:
#   schedule:
#     - cron: "17 6 * * *"   # nightly, off-peak UTC
#   workflow_dispatch: {}     # manual re-runs for quiet-box confirmation
#
# jobs:
#   perf-scan:
#     runs-on: [self-hosted, perf]   # see "corpus provisioning" below
#     timeout-minutes: 300
#     continue-on-error: true        # non-blocking by construction
#     steps:
#       - uses: actions/checkout@v4
#
#       # Corpus provisioning — pick ONE of the options discussed below.
#       # Option A (artifact bucket):
#       # - run: aws s3 sync s3://gts-perf-corpus/corpus_sources /corpus/corpus_sources
#       # Option B (checked-in minimal subset): nothing to do, corpus ships in-repo.
#       # Option C (self-hosted runner): corpus pre-provisioned at a fixed path.
#
#       - name: Restore C reference build cache
#         uses: actions/cache@v4
#         with:
#           path: harness_out/parity_c_ref_cache
#           key: parity-c-ref-${{ hashFiles('grammars/languages.lock') }}
#
#       - name: Run perf scan sweep
#         working-directory: cgo_harness
#         env:
#           GOWORK: "off"
#           GTS_PARITY_ALLOW_HOST: "1"        # or run inside the parity container
#           GTS_PERF_SCAN: "1"
#           GTS_PERF_SCAN_CORPUS_ROOT: /corpus/corpus_sources
#           GTS_REAL_CORPUS_BENCH_LOCK: /corpus/corpus_sources.lock
#           GTS_PERF_SCAN_MAX_FILES: "8"
#           GTS_PERF_SCAN_ORDER: largest
#           GTS_PERF_SCAN_REPS: "5"
#           GTS_PERF_SCAN_FILE_BUDGET_MS: "10000"
#           GTS_PERF_SCAN_LANG_TIMEOUT_MS: "900000"
#           GTS_PERF_SCAN_OUT: perf_scan/out/nightly
#         run: |
#           go test -tags "treesitter_c_parity treesitter_c_perfscan" \
#             -run '^TestPerfScanSweep$' -v -count=1 -timeout 0 -p 1 .
#
#       - name: Check perf ratio budget
#         working-directory: cgo_harness
#         run: |
#           GOWORK=off go run ./cmd/perf_scan_budget \
#             -budget perf_scan/perf_ratio_budgets.json \
#             -scoreboard perf_scan/out/nightly/scoreboard.json \
#             -require-all-budget-langs \
#             -out-md perf_scan/out/nightly/budget.md
#
#       - name: Upload scoreboard
#         if: always()
#         uses: actions/upload-artifact@v4
#         with:
#           name: perf-scoreboard-${{ github.run_id }}
#           path: cgo_harness/perf_scan/out/nightly
#           retention-days: 90
#
#       # Optional trend step: diff scoreboard.json against the previous run's
#       # artifact and open/update a tracking issue when a language's verdict
#       # bucket regresses (<=1.2x -> <=2x etc.) or a new cliff appears.
# --- END PROPOSAL ---
```

## The corpus provisioning question (stated honestly)

The real corpus is local-only today: `cgo_harness/corpus_real/` is
git-ignored (generated onto dev boxes) and the big per-language repo
checkouts live at `/home/draco/work/gotreesitter-corpora/corpus_sources`
(~GBs, one full upstream repo per language). CI has neither. Options:

**Option A — corpus artifact bucket (recommended).** Snapshot the pinned
corpus (or a curated per-language slice of it) into an object-store bucket
(S3/GCS/Harbor OCI artifact), keyed by a corpus lock hash. The job syncs it
at start (~minutes, cacheable on a self-hosted runner). Pros: real corpus,
reproducible, no repo bloat, works on any runner with credentials. Cons:
infra + credentials to maintain; snapshot must be re-cut when the corpus
profile changes.

**Option B — checked-in minimal corpus subset.** Commit a small curated
subset (e.g. the existing small/medium/large triple per language, ~5-10 MB
total) under a new `cgo_harness/corpus_ci/`. Pros: zero infra, works on
hosted runners, deterministic. Cons: weak cliff coverage — the known cliffs
(e.g. bash on ~100 KB git test scripts) only reproduce on files larger than
what is reasonable to commit; upstream licenses must be vetted before
vendoring. Usable as a smoke tier, not as the authoritative sweep.

**Option C — self-hosted runner with pre-provisioned corpus.** Label a quiet
self-hosted box (`[self-hosted, perf]`), provision `corpus_sources` once at a
fixed path, point `GTS_PERF_SCAN_CORPUS_ROOT` at it. Pros: zero per-run
transfer, the same box every night makes ratios comparable across runs; this
matches how the numbers are produced locally today. Cons: single point of
failure, box must actually stay quiet (the `contended` auto-flag guards
this), corpus drift is manual.

Pragmatic recommendation: **C now, A later** — start with the self-hosted
quiet box (fastest path to nightly trend data, identical to today's manual
runs), and graduate to the artifact bucket when a second runner or team
consumers appear. Keep B only if a hosted-runner smoke tier is wanted on PRs.

## Non-blocking regression visibility

The implemented `cmd/perf_scan_budget` step compares each nightly
`scoreboard.json` against the checked-in ratio budget and fails the step for
new ratio, timeout, truncation, or C-reference failures. The nightly job should
remain `continue-on-error: true`, so regressions are visible in the job summary
and uploaded artifact without blocking unrelated PRs. A later trend step can
still diff consecutive artifacts to flag verdict-bucket movement or open/update
a pinned tracking issue.
