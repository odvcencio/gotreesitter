# perf_scan — per-language Go-vs-C real-corpus timing scoreboard

Measures gotreesitter (pure Go) against the C tree-sitter reference, per
language, on real corpus files, and emits a scoreboard (JSON + markdown).
This is the measurement half of the "drop-in tree-sitter replacement" bar:
universal C-similar performance on full parse / no-edit reparse /
incremental edit.

The tool lives in `cgo_harness/zz_perf_scan_test.go` behind the build tags
`treesitter_c_parity treesitter_c_perfscan` — it never compiles into normal
builds or the parity suites. Outputs land under `cgo_harness/perf_scan/out/`
(git-ignored).

## What is measured (per language, per file)

| axis | Go side | C side | default |
|---|---|---|---|
| `full` | `Parser.Parse` / `ParseWithTokenSource` (fresh) | `ts_parser_parse(NULL, src)` | on |
| `noedit` | `ParseIncremental(src, oldTree)`, unchanged source | `ts_parser_parse(oldTree, src)` | on |
| `edit` | single-byte replace + `Tree.Edit` + incremental reparse | same via C `Tree.Edit` | opt-in (`GTS_PERF_SCAN_AXES=full,noedit,edit`) |

Protocol per file/axis: `warmup` untimed attempts, then `reps` timed attempts
alternating Go/C (drift-resistant on shared boxes); the reported number is the
median, with min/max recorded. Per-file ratio = Go median / C median. Language
aggregate = ratio-by-total (sum of Go medians / sum of C medians) plus
median-of-file-ratios.

Verdict buckets: `<=1.2x`, `<=2x`, `>2x`, `cliff>10x`. Any single file over
10x — or a file that hits the parse budget — escalates the language to
`cliff>10x` so cliffs cannot hide behind healthy averages.

Notes on interpretation:
- The scan is timing-grade, not correctness-grade: structural parity is owned
  by the parity suites / tier_scan. Truncated or errored parses are excluded
  from medians and surfaced as per-file statuses.
- The Go no-edit path may legitimately short-circuit (near-zero ns) when the
  engine returns the old tree for an unchanged reparse; the C side always
  pays its reuse walk. The scoreboard reports honest wall time of the API call.
- The `edit` axis verifies only that both incremental reparses complete
  (completeness, not structural parity) before timing — see Phase 2.

## Cliff containment (why one 17s file cannot hang the sweep)

Two layers:
1. Per-attempt budget (`GTS_PERF_SCAN_FILE_BUDGET_MS`, default 5000): Go via
   `Parser.SetTimeoutMicros` (partial tree + `ParseStoppedEarly`), C via
   `ts_parser_set_timeout_micros` (nil tree + parser reset). A timed-out Go
   file is recorded as `go_timeout` with a **lower-bound ratio**
   (`budget / C median`, `ratio_is_lower_bound=true`) — the cliff is surfaced,
   not hung on.
2. Per-language subprocess with a hard wall-clock kill
   (`GTS_PERF_SCAN_LANG_TIMEOUT_MS`, default 10 min): the sweep re-execs the
   test binary per language, so hard hangs, native crashes in a C grammar, or
   OOMs cost one language row (`lang_timeout` / `error` with log tail), never
   the sweep. Partial per-file results survive because the child rewrites its
   fragment after every file.

## Running

Requires: cgo + C toolchain, the parity container OR `GTS_PARITY_ALLOW_HOST=1`,
and a corpus. C reference grammars are built/loaded by the existing parity
machinery (`ParityCLanguage`, `grammars/languages.lock`, cached under
`harness_out/parity_c_ref_cache/`).

Smoke (explicit languages, default corpus `corpus_real/`):

```sh
cd cgo_harness
GOWORK=off GTS_PARITY_ALLOW_HOST=1 GTS_PERF_SCAN=1 \
  GTS_PERF_SCAN_LANGS=go,python,bash,json,c_sharp \
  go test -tags "treesitter_c_parity treesitter_c_perfscan" \
  -run '^TestPerfScanSweep$' -v -count=1 -timeout 0 .
```

Authoritative full sweep on a QUIET box (all languages that have both a corpus
dir and a registry entry are auto-discovered from the corpus root):

```sh
cd cgo_harness
GOWORK=off GTS_PARITY_ALLOW_HOST=1 GTS_PERF_SCAN=1 \
  GTS_PERF_SCAN_CORPUS_ROOT=/home/draco/work/gotreesitter-corpora/corpus_sources \
  GTS_REAL_CORPUS_BENCH_LOCK=/home/draco/work/gotreesitter-corpora/corpus_sources.lock \
  GTS_PERF_SCAN_MAX_FILES=16 GTS_PERF_SCAN_ORDER=largest \
  GTS_PERF_SCAN_REPS=7 GTS_PERF_SCAN_FILE_BUDGET_MS=10000 \
  GTS_PERF_SCAN_OUT=perf_scan/out/authoritative_$(date -u +%Y%m%dT%H%M%SZ) \
  go test -tags "treesitter_c_parity treesitter_c_perfscan" \
  -run '^TestPerfScanSweep$' -v -count=1 -timeout 0 .
```

When the corpus root ends in `corpus_sources`/`corpus-sources` the existing
lock-filter machinery restricts file selection to each language's subdir and
extensions from the corpus lock (same rules as the real-corpus parity
benchmarks). Point `GTS_REAL_CORPUS_BENCH_LOCK` at the corpus lock
(`corpus_sources.lock` next to the corpus checkouts); without it the filter
falls back to `grammars/languages.lock`, whose `subdir` column describes
grammar repos, not corpus repos.

## Knobs (all env)

| env | default | meaning |
|---|---|---|
| `GTS_PERF_SCAN` | — | master gate; `1` to run |
| `GTS_PERF_SCAN_LANGS` | auto-discover | comma list for the sweep |
| `GTS_PERF_SCAN_LANG` | — | single language (child mode; set by the sweep) |
| `GTS_PERF_SCAN_OUT` | `perf_scan/out/scan_<UTC>` | output dir |
| `GTS_PERF_SCAN_CORPUS_ROOT` | `corpus_real` | corpus root (per-language subdirs) |
| `GTS_PERF_SCAN_REPS` | 5 | timed reps per file/axis/impl |
| `GTS_PERF_SCAN_WARMUP` | 1 | untimed warmup attempts |
| `GTS_PERF_SCAN_FILE_BUDGET_MS` | 5000 | per parse-attempt budget |
| `GTS_PERF_SCAN_LANG_TIMEOUT_MS` | 600000 | hard subprocess kill per language |
| `GTS_PERF_SCAN_MAX_FILES` | 16 | files per language (after ordering) |
| `GTS_PERF_SCAN_MIN_FILE_BYTES` / `_MAX_FILE_BYTES` | 0 / 4MiB | size filters |
| `GTS_PERF_SCAN_ORDER` | `largest` | `largest` / `smallest` / `path` selection order |
| `GTS_PERF_SCAN_AXES` | `full,noedit` | add `edit` for the incremental-edit axis |
| `GTS_PERF_SCAN_CONTENDED` | auto (loadavg) | mark run as contended (smoke-only numbers) |
| `GTS_PERF_SCAN_INPROCESS` | 0 | debug: run languages in-process (no crash isolation) |
| `GTS_PERF_SCAN_EDIT_CANDIDATES` | 16 | edit-site candidates tried per file |

Also honored: `GTS_PARITY_ALLOW_HOST`, `GTS_PARITY_SKIP_LANGS`,
`GTS_PARITY_REPO_ROOT`, `GTS_PARITY_REPO_CACHE`,
`GTS_PARITY_C_REF_BUILD_CACHE` (C reference build machinery).

## Outputs

```
<out>/scoreboard.json   machine-readable (schema gts-perf-scan/v1)
<out>/scoreboard.md     human scoreboard + cliff appendix
<out>/langs/<lang>.json per-language fragments (partial results survive kills)
<out>/logs/<lang>.log   child stdout/stderr per language
```

`scoreboard.json` carries host metadata (loadavg at start/end), the full
config, a `contended` flag, per-language per-axis aggregates, and per-file
medians/ratios/statuses.

## Ratio budget ratchet

Wave 3 seeds a checked-in ratio budget at
`cgo_harness/perf_scan/perf_ratio_budgets.json`. The file is intentionally a
ratchet, not an aspirational target list: values may be tightened after better
measurements or engine fixes, but loosening a budget needs a root-cause note in
the PR that explains why the old bound is no longer reachable.

Validate the budget file itself:

```sh
cd cgo_harness
GOWORK=off go run ./cmd/perf_scan_budget \
  -budget perf_scan/perf_ratio_budgets.json
```

Compare an authoritative scoreboard against the ratchet:

```sh
cd cgo_harness
GOWORK=off go run ./cmd/perf_scan_budget \
  -budget perf_scan/perf_ratio_budgets.json \
  -scoreboard perf_scan/out/authoritative_YYYYMMDDTHHMMSSZ/scoreboard.json \
  -require-all-budget-langs \
  -out-md perf_scan/out/authoritative_YYYYMMDDTHHMMSSZ/budget.md
```

The checked-in budget was seeded with `order=largest`, `max_files=8`,
`reps=5`, `warmup=1`, `file_budget_ms=10000`, and axes `full,noedit`. Generate
a strict ratchet scoreboard with those same knobs inside the parity container:

```sh
bash cgo_harness/docker/run_parity_in_docker.sh \
  --label perf-scan-ratchet \
  --memory 8g \
  --cpus 2 \
  --pids 4096 \
  --gomemlimit 6GiB \
  --goflags -p=1 \
  --mount /home/draco/work/gotreesitter-corpora:/corpus:ro \
  -- "cd /workspace/cgo_harness && \
      GOWORK=off GTS_PERF_SCAN=1 \
      GTS_PERF_SCAN_CORPUS_ROOT=/corpus/corpus_sources \
      GTS_REAL_CORPUS_BENCH_LOCK=/corpus/corpus_sources.lock \
      GTS_PERF_SCAN_MAX_FILES=8 GTS_PERF_SCAN_ORDER=largest \
      GTS_PERF_SCAN_REPS=5 GTS_PERF_SCAN_FILE_BUDGET_MS=10000 \
      GTS_PERF_SCAN_OUT=perf_scan/out/ratchet_\$(date -u +%Y%m%dT%H%M%SZ) \
      go test -tags 'treesitter_c_parity treesitter_c_perfscan' \
      -run '^TestPerfScanSweep$' -v -count=1 -timeout 0 ."
```

For a targeted language refresh, scope the comparison:

```sh
cd cgo_harness
GOWORK=off go run ./cmd/perf_scan_budget \
  -budget perf_scan/perf_ratio_budgets.json \
  -scoreboard perf_scan/out/go_refresh/scoreboard.json \
  -langs go
```

The checker gates `ratio_by_total`, optional `ratio_median_of_files`,
`go_timeout` counts, and Go-side error/truncation counts. It also rejects C
reference failures and, by default, requires the scoreboard's structured
measurement knobs (`reps`, `warmup`, `file_budget_ms`, `max_files`, `order`,
and axes) to match the budget metadata.

## Phase 2 (documented, not built)

- Correctness-verified `edit` axis: verify structural parity of the
  incremental result against a fresh parse before timing (the machinery exists
  in `benchmark_real_corpus_parity_test.go`; it roughly doubles cost, so it
  stays out of the default sweep).
- Multi-site edit sampling (median over K edit sites per file instead of the
  first verified site).
- Allocation / RSS axes (Go `ReportAllocs` analogue vs C arena growth).
- Trend storage across nightly artifacts and issue updates when a language
  worsens after budget comparison — see CI_PROPOSAL.md.
