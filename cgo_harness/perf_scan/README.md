# perf_scan — per-language Go-vs-C real-corpus timing scoreboard

Measures gotreesitter (pure Go) against the C tree-sitter reference, per
language, on real corpus files, and emits a scoreboard (JSON + markdown).
This is the measurement half of the fresh full-parse performance bar:
universal C-similar performance on materialized trees from real source files.

The tool lives in `cgo_harness/zz_perf_scan_test.go` behind the build tags
`treesitter_c_parity treesitter_c_perfscan` — it never compiles into normal
builds or the parity suites. Outputs land under `cgo_harness/perf_scan/out/`
(git-ignored).

## What is measured (per language, per file)

| axis | Go side | C side | authoritative |
|---|---|---|---|
| `full` | `Parser.Parse` / `ParseWithTokenSource` (fresh) | locked upstream `ts_parser_parse_string(NULL, src)` in a fully static sidecar | yes |

Protocol per file: `warmup` untimed attempts, then `reps` timed attempts on
each implementation; the reported number is the median, with min/max recorded.
The C timing process loads the immutable source snapshot and constructs its
parser before sampling. Its timer covers only `ts_parser_parse_string`; process
startup, source loading, parser setup, and tree deletion are excluded. Go
sampling retains the existing parser and parse-call timing path. To avoid a
systematic implementation-order bias while keeping C process setup outside the
timer, each selected file contains one whole Go measurement block and one whole
static-C measurement block. Their order alternates across selected-file
ordinals; there is no within-file AB/BA interleaving or per-repetition
alternation. The language-name SHA-256 low bit selects the first file's block
order, so one-file rows start on different sides across the fleet. The common
protocol and each file's actual order are serialized and reducer-checked.
Per-file ratio = Go median / C median. Language aggregate = ratio-by-total
(sum of Go medians / sum of C medians) plus median-of-file-ratios.

Before accepting any C sample, the harness parses that exact source through
both the in-process parity binding and the fully static sidecar and requires an
identical `gts-deep-tree-v1` digest. Each shard serializes the locked binding,
runtime, grammar, parser/scanner source hashes, exact C/C++ compile and static
link flags, compiler/linker paths, versions, executable hashes, artifact hash,
linkage proof, and every per-file admission digest. The reducer rejects missing
or mixed identity,
dynamic linkage, wrong commits or flags, and absent deep admission. The
content-keyed artifact cache installs the executable and a build-key/artifact-
hash manifest together by atomic directory rename. After linking and before a
manifest or shared cache path becomes visible, the harness rechecks the
already-resolved compiler/linker versions and hashes, pinned runtime/grammar
worktrees and tree identities, and every runtime/parser/scanner/driver hash
used by that build. Cache hits repeat the same input check before use. Drift
retries once from a fresh identity capture and generated-source workspace;
persistent instability fails closed. Reuse also rechecks the manifest,
executable hash, required symbols, and static linkage; an absent or mismatched
manifest quarantines the entry and forces a verified rebuild.
After validation, execution uses a private copy whose hash and linkage are
reverified, so replacing the shared cache path cannot change an in-flight
shard's executable. This closes realistic same-user concurrent source/tool/cache
replacement; it does not claim adversarial protection from a privileged actor
that can alter a running compiler or process memory. Publication schema is
fresh-full-only. Legacy `noedit` and `edit` diagnostics
can still be requested locally, but any such shard is reducer-inadmissible.

Deep admission uses iterative tree cursors in both the static sidecar and the
in-process cgo binding, so deep or wide witnesses do not consume one call-stack
frame per syntax node. The dump parse and cgo digest each have an explicit
wall bound independent of the parser timeout. Admission, parser, transport,
digest, measurement, and protocol failures use a closed `c_oracle_*`
vocabulary with distinct parser/transport/digest timeout statuses; generic
`c_error` and `c_oracle_error` evidence is inadmissible. Any C timeout,
incomplete tree, transport error, or digest mismatch fails the hard gate, but
it does not suppress the bounded Go full parse: Go completion/stop
classification and timing evidence are still emitted, with no fabricated
ratio. The reducer accepts such a row as authenticated failure evidence only
when its source hash, typed C status/detail, measurement order, static identity,
and bounded Go evidence are complete; missing or contradictory provenance
remains inadmissible.

The full axis also records an untimed corpus-policy classification for each
file. `clean` requires a successful Go parse whose root spans `[0,len(source))`,
did not stop early, and contains no `ERROR` nodes. Every other result is
non-clean: ordinary full-span error trees are `error`, while timeout, memory,
node/iteration/stack, and other early-stop results are `stopped` with their
parser reason retained. The first Go warmup supplies the classification when
warmups are enabled and remains authoritative even if a later timed attempt
fails. With `GTS_PERF_SCAN_WARMUP=0`, a distinct classification parse always
runs after all timed full-parse samples, including after a timed failure, so it
cannot warm the measurement it describes. A probe that returns an ordinary
parse error or structured parser stop does not modify timing samples, axis
status, structured timing stops, verdicts, or the hard gate. Root error
presence is a precomputed flag; the scan does not add a tree walk to the timed
path. Both this tool and `parse_gap_report` use the shared
`IsAcceptedFullSpanCleanGoTree` predicate for the tree policy.

Per-language `full_parse_split` totals report clean and non-clean/error files
separately. Stopped files are a named subset of the error side, matching the
`Clean=false` corpus policy used by `parse_gap_report` and
`parse_gap_correlate`; because stopped files have no exact ratio, only
status=`ok` rows contribute to class timing totals and ratios. `error_share`
is non-clean files divided by all classified files. These fields are
diagnostic only: they do not alter file selection, coverage, verdict buckets,
or the existing hard-gate thresholds.

Set `GTS_PERF_SCAN_RUNTIME_EVIDENCE=1` to attach opt-in runtime evidence to
each retained Go full-parse classification. The record includes recovery
entries, recovery cost walks, retry attempts, the selected retry rung, parser
allocation counters, phase timing, materialization timing, and arena capacity
facts. The scan captures these facts outside timed samples. With this knob, the
scan pins its parser to the production route. It suppresses compact admission,
automatic forest dispatch, and forest recovery replacement. It does not change
the shipped default, parser limits, or hard-gate decisions. Treat the output as
cohort evidence, not performance credit.

Verdict buckets: `<=0.10x`, `<=1.2x`, `<=2x`, `>2x`, `cliff>10x`. The first
bucket is reported separately as a 10x-or-better win. The hard gate evaluates
the exact per-file full-parse ratio: `<=10.0x` passes and `>10.0x` fails. A
healthy aggregate can never hide a single cliff.

Notes on interpretation:
- The scan is a timing/resource gate, not a structural-correctness gate.
  Structural and error-tree parity remain owned by the parity suites and
  tier_scan. A missing or non-OK full-parse measurement still fails this gate
  closed because no exact Go/C ratio was produced; that is coverage, not a
  claim that this harness proved structural parity.
- Incremental and no-edit performance use separate benchmark lanes. They are
  deliberately outside this publication scoreboard.

## Cliff containment (why one 17s file cannot hang the sweep)

Three layers record structured stops in `scoreboard.json`:

1. Per-attempt budget (`GTS_PERF_SCAN_FILE_BUDGET_MS`, default 5000): Go via
   `Parser.SetTimeoutMicros` (partial tree + `ParseStoppedEarly`), C via
   `ts_parser_set_timeout_micros` (nil tree + parser reset). A timed-out Go
   file is recorded as `go_timeout`, `go_budget_stop`, or `go_stopped`, with
   the parser stop reason preserved. Lower-bound ratios remain telemetry, but
   every Go parser timeout or budget stop fails the hard gate.
2. Per-file cgo admission subprocess with an RSS limit
   (`GTS_PERF_SCAN_CGO_ADMISSION_RSS_LIMIT_MB`, default 4096): the scan stops
   pathological C trees. It records `c_oracle_resource_limit` and continues
   with the Go measurement and later files. The admission record retains its
   C stop when the full axis must retain a later Go stop.
3. Per-language subprocess with a hard wall-clock kill
   (`GTS_PERF_SCAN_LANG_TIMEOUT_MS`, default 10 min): the sweep re-execs the
   test binary per language, so hard hangs, native crashes in a C grammar, or
   OOMs cost one language row, never the sweep. Wall timeout, RSS watchdog,
   SIGKILL/OOM, and other process signals are classified separately and retain
   the active file/axis/implementation when a fragment exists. A Go wall/RSS/
   OOM stop fails the hard gate; incomplete coverage also fails closed.

## Running

Requires: cgo, a C/C++ toolchain with static runtime libraries, `nm`, `readelf`,
the parity container OR `GTS_PARITY_ALLOW_HOST=1`, and a corpus. The admission
binding uses the existing parity machinery (`ParityCLanguage`). The timing
sidecar independently builds the same locked runtime and grammar from
`grammars/languages.lock`, cached under `harness_out/c_oracle/fleet_static/` by
default.

Exploratory smoke (explicit languages, default corpus `corpus_real/`). This
opts out of the hard gate because it does not use the authenticated fleet lock:

```sh
cd cgo_harness
GOWORK=off GTS_PARITY_ALLOW_HOST=1 GTS_PERF_SCAN=1 GTS_PERF_SCAN_HARD_GATE=0 \
  GTS_PERF_SCAN_LANGS=go,python,bash,json,c_sharp \
  go test -tags "treesitter_c_parity treesitter_c_perfscan" \
  -run '^TestPerfScanSweep$' -v -count=1 -timeout 0 .
```

Authoritative full sweep on a quiet box. With no `GTS_PERF_SCAN_LANGS`, the
hard gate selects every language in the authenticated corpus lock and requires
complete fleet coverage:

```sh
cd cgo_harness
GOWORK=off GTS_PARITY_ALLOW_HOST=1 GTS_PERF_SCAN=1 \
  GTS_PERF_SCAN_CORPUS_ROOT=/home/draco/work/gotreesitter-corpora/corpus_sources \
  GTS_REAL_CORPUS_BENCH_LOCK=/home/draco/work/gotreesitter-corpora/corpus_sources.lock \
  GTS_PERF_SCAN_HARD_GATE=1 GTS_PERF_SCAN_REQUIRE_FLEET=1 \
  GTS_PERF_SCAN_MAX_FILES=8 GTS_PERF_SCAN_ORDER=largest \
  GTS_PERF_SCAN_REPS=5 GTS_PERF_SCAN_FILE_BUDGET_MS=10000 \
  GTS_PERF_SCAN_CHILD_RSS_LIMIT_MB=6144 \
  GTS_PERF_SCAN_CGO_ADMISSION_RSS_LIMIT_MB=4096 \
  GTS_PERF_SCAN_OUT=perf_scan/out/authoritative_$(date -u +%Y%m%dT%H%M%SZ) \
  go test -tags "treesitter_c_parity treesitter_c_perfscan" \
      -run '^TestPerfScanSweep$' -v -count=1 -timeout 0 .
```

### Resumable one-language shards and authenticated reduction

Long fleet refreshes can run as independent one-language scoreboards. Keep the
measurement knobs identical, set `GTS_PERF_SCAN_REQUIRE_FLEET=0`, and give each
language its own output directory:

```sh
cd cgo_harness
GOWORK=off GTS_PARITY_ALLOW_HOST=1 GTS_PERF_SCAN=1 \
  GTS_PERF_SCAN_CORPUS_ROOT=/home/draco/work/gotreesitter-corpora/corpus_sources \
  GTS_REAL_CORPUS_BENCH_LOCK=/home/draco/work/gotreesitter-corpora/corpus_sources.lock \
  GTS_PERF_SCAN_HARD_GATE=1 GTS_PERF_SCAN_REQUIRE_FLEET=0 \
  GTS_PERF_SCAN_LANGS=go GTS_PERF_SCAN_MAX_FILES=8 \
  GTS_PERF_SCAN_ORDER=largest GTS_PERF_SCAN_REPS=5 \
  GTS_PERF_SCAN_FILE_BUDGET_MS=10000 \
  GTS_PERF_SCAN_OUT=perf_scan/out/shards/go \
  go test -tags "treesitter_c_parity treesitter_c_perfscan" \
  -run '^TestPerfScanSweep$' -v -count=1 -timeout 0 .
```

Repeat for every language in the authenticated lock. Completed shard
scoreboards are durable checkpoints; the reducer reads them without invoking a
parser, so an interrupted campaign resumes from the missing languages rather
than restarting the fleet. When each shard runs in its own Docker container,
pass the same `--hostname` value to `run_parity_in_docker.sh` for every
container on one physical host (for example `--hostname gts-perf-quiet-01`).
Container-generated hostnames are unique and therefore cannot authenticate as
one measurement host. Once every shard exists, run the reducer:

A shard with a measured hard-gate FAIL writes its scoreboard and then returns
nonzero. The campaign coordinator must validate and retain that scoreboard,
record the failed gate, and continue with the next language; do not let a bare
`set -e` stop the fleet at its first valid cliff. Process crashes or missing,
stale, or unauthenticated scoreboards remain retry conditions.

```sh
cd cgo_harness
GOWORK=off GTS_PARITY_ALLOW_HOST=1 GTS_PERF_SCAN=1 \
  GTS_REAL_CORPUS_BENCH_LOCK=/home/draco/work/gotreesitter-corpora/corpus_sources.lock \
  GTS_PERF_SCAN_REDUCE_INPUTS='perf_scan/out/shards/*/scoreboard.json' \
  GTS_PERF_SCAN_REDUCE_MODE=report \
  GTS_PERF_SCAN_OUT=perf_scan/out/authoritative_reduced \
  go test -tags "treesitter_c_parity treesitter_c_perfscan" \
  -run '^TestPerfScanReduce$' -v -count=1 -timeout 0 .
```

Reduction requires exactly one fresh-full `gts-perf-scan/v2` scoreboard for
every locked language. It
rejects missing or duplicate languages, schema or measurement-config drift,
corpus-lock drift, path exclusions, contended runs, mixed or absent repository
revisions, dirty source trees, mixed host/runtime identities, contradictory or
malformed file classifications, pre-existing reduction provenance, and absent
or stale shard hard gates. It also rejects absent or mixed static-C identities,
non-static linkage, compile/link flag drift, wrong runtime or grammar commits,
and any selected file without matching static/cgo deep-tree admission. Raw
one-language measurement shards must not contain
a `reduction` object. A stored shard gate may be either PASS or FAIL, but it
must exactly equal the gate recomputed from that shard's evidence after
canonical ordering. Published `failures` and `fast_full_files` are sorted by
their complete finding and stop attribution, so Go map iteration order cannot
change an authenticated gate or combined artifact. Ratio cliffs, parser stops,
missing ratios, and partial or zero file coverage are valid failure evidence
and remain visible in the combined FAIL board. A host identity includes hostname,
GOOS/GOARCH, CPU count, `GOMAXPROCS`, and Go version; both start and end load
averages must remain below the contention threshold. The
only permitted per-shard config differences are the one-element `languages`
selection and `require_fleet=false`; the combined config is rewritten to the
full lock language list with `require_fleet=true`. The reducer recomputes every
language aggregate, verdict summary, fleet clean/error split, coverage record,
and the full hard gate before writing authoritative JSON and Markdown. The
reducer execution is authenticated independently: its checkout/build must be
clean, but it may be a later revision than the immutable measurement revision
recorded by every shard. The combined board preserves the shard
`git_revision`/`git_clean` fields as measurement provenance and adds a
`reduction` object containing the reducer's `git_revision` and `git_clean`
attestation; Markdown renders both revisions explicitly.

`GTS_PERF_SCAN_REDUCE_MODE=report` writes the recomputed PASS or FAIL board and
returns success after evidence authentication. `certify` is the default; it
writes the identical board, then fails the test when the combined hard gate is
FAIL and reports the artifact path and finding count. An unknown mode fails
before any scoreboard is published.

Only report mode can close a complete fleet cohort around a terminal
`no_static_c_oracle`, `no_corpus`, or `no_corpus_files` shard. The reducer
authenticates that closed status and requires an otherwise empty measurement
payload, preserves it as a fatal coverage finding, and omits only that row from
the combined static-oracle language manifest. Certification rejects the same
row before publication. Missing, generic, contradictory, or mixed identity
evidence remains inadmissible in both modes.

New scoreboards record the full repository revision and clean-source attestation.
Revisionless v1 JSON still decodes for existing readers, but is deliberately
ineligible for authoritative reduction. Git and Go build VCS metadata must
agree and neither may report modification. `GTS_PERF_SCAN_GIT_REVISION` cannot
override discovered metadata; it accepts a full 40- or 64-hex revision only
when neither source is available, together with explicit
`GTS_PERF_SCAN_GIT_CLEAN=1`. Exploratory `GTS_PERF_SCAN_HARD_GATE=0` runs may
record `git_clean=false` so uncommitted candidates remain benchmarkable, but
those scoreboards cannot enter authoritative reduction.

Hard-gate mode requires `GTS_REAL_CORPUS_BENCH_LOCK`. Its SHA-256 must match
`perf_scan/corpus_sources.lock.sha256` and the same digest in the budget file;
a missing, changed, empty, or registry-incomplete lock aborts before timing.
When the corpus root ends in `corpus_sources`/`corpus-sources`, the existing
lock-filter machinery restricts file selection to each language's locked
subdirectory and extensions. This prevents the unsafe fallback to
`grammars/languages.lock`, whose `subdir` column describes grammar repos, not
corpus repos.

The dedicated workflow additionally verifies all 206 corpus checkout `HEAD`s,
locked subpaths, and tracked worktree/index cleanliness before mounting the
corpus read-only. It cannot require a completely empty untracked set because
the corpus builder deliberately supplies nested dependency checkouts and
`.gts-extracted` fixture trees that some lock rows select; see
`CI_PROPOSAL.md` for that provisioning boundary.

## Knobs (all env)

| env | default | meaning |
|---|---|---|
| `GTS_PERF_SCAN` | — | master gate; `1` to run |
| `GTS_PERF_SCAN_HARD_GATE` | 1 | enforce the fail-closed per-file/full-fleet gate; set `0` only for exploratory telemetry |
| `GTS_PERF_SCAN_REQUIRE_FLEET` | 1 when `GTS_PERF_SCAN_LANGS` is empty | require the selected language count to equal the authenticated lock |
| `GTS_PERF_SCAN_LANGS` | locked fleet (hard) / auto-discover (exploratory) | comma list for a targeted sweep |
| `GTS_PERF_SCAN_LANG` | — | single language (child mode; set by the sweep) |
| `GTS_PERF_SCAN_OUT` | `perf_scan/out/scan_<UTC>` | output dir |
| `GTS_PERF_SCAN_REDUCE_INPUTS` | — | reducer-only comma list of scoreboard paths/globs; requires exactly one authenticated shard per lock language |
| `GTS_PERF_SCAN_REDUCE_MODE` | `certify` | `report` always publishes and returns success for authenticated PASS/FAIL evidence; `certify` publishes the same board and then blocks on combined FAIL |
| `GTS_PERF_SCAN_GIT_REVISION` | auto-discovered | full repository object ID override for metadata-poor build environments |
| `GTS_PERF_SCAN_GIT_CLEAN` | — | explicit clean-source attestation required with a metadata-poor revision override |
| `GTS_PERF_SCAN_CORPUS_ROOT` | `corpus_real` | corpus root (per-language subdirs) |
| `GTS_PERF_SCAN_REPS` | 5 | timed reps per file/axis/impl |
| `GTS_PERF_SCAN_WARMUP` | 1 | untimed warmup attempts |
| `GTS_PERF_SCAN_FILE_BUDGET_MS` | 5000 | per parse-attempt budget |
| `GTS_PERF_SCAN_LANG_TIMEOUT_MS` | 600000 | hard subprocess kill per language |
| `GTS_PERF_SCAN_MAX_FILES` | 16 | files per language (after ordering) |
| `GTS_PERF_SCAN_MIN_FILE_BYTES` / `_MAX_FILE_BYTES` | 0 / 4MiB | size filters |
| `GTS_PERF_SCAN_EXCLUDE_PATHS` | — | comma-separated language-relative or `language/path` exact paths/globs to omit from selection; globs use Go `path.Match` semantics (`*` does not cross `/`, and `**` is not recursive), while trailing `/` means recursive directory prefix. Intended for named C-oracle cliff witnesses that remain tracked in the ledger |
| `GTS_PERF_SCAN_ORDER` | `largest` | `largest` / `smallest` / `path` selection order |
| `GTS_PERF_SCAN_AXES` | `full` | publication accepts only `full`; legacy `noedit`/`edit` requests are diagnostic and reducer-inadmissible |
| `GTS_PERF_SCAN_CONTENDED` | auto (loadavg) | mark run as contended (smoke-only numbers) |
| `GTS_PERF_SCAN_INPROCESS` | 0 | debug: run languages in-process (no crash isolation) |
| `GTS_PERF_SCAN_EDIT_CANDIDATES` | 16 | edit-site candidates tried per file |
| `GTS_PERF_SCAN_CHILD_RSS_LIMIT_MB` | 0 | optional parent-side RSS watchdog for the per-language child process group; when set, kills and reaps the child and its descendants before a container cgroup OOM can kill the sweep parent |
| `GTS_PERF_SCAN_CGO_ADMISSION_RSS_LIMIT_MB` | 4096 | RSS watchdog for each exact-source cgo admission child; hard-gate scans require a positive value |
| `GTS_PERF_SCAN_RUNTIME_EVIDENCE` | 0 | attach runtime facts to retained Go classifications and pin the scan to the production route |
| `GTS_REAL_CORPUS_BENCH_LOCK` | required by hard gate | authenticated corpus selection lock; digest is checked before any language runs |
| `GTS_C_ORACLE_CACHE` | `harness_out/c_oracle/fleet_static` | cache for pinned runtime/grammar sources and content-keyed fully static timing artifacts |

Also honored: `GTS_PARITY_ALLOW_HOST`, `GTS_PARITY_SKIP_LANGS`,
`GTS_PARITY_REPO_ROOT`, `GTS_PARITY_REPO_CACHE`,
`GTS_PARITY_C_REF_BUILD_CACHE` (C reference build machinery).

## Outputs

```
<out>/scoreboard.json   machine-readable (schema gts-perf-scan/v2)
<out>/scoreboard.md     human scoreboard + cliff appendix
<out>/langs/<lang>.json per-language fragments (partial results survive kills)
<out>/logs/<lang>.log   child stdout/stderr per language
```

`scoreboard.json` carries the repository revision, host metadata (loadavg at
start/end), the full config, authenticated corpus coverage, a `contended` flag,
static-C oracle identity and admissions, structured stop records, per-language
full-parse aggregates, per-file medians/ratios/statuses,
optional per-file full-parse classifications, per-language clean/error timing
splits, a fleet clean/error timing split, and a `hard_gate` report. The markdown
renders the same class totals and lists each non-clean file with its reason.
The report lists failures and separately lists full-parse files at `<=0.10x`.
Both files are staged and synced before publication; Markdown is published
first and `scoreboard.json` is the final commit marker. A failed JSON publish
rolls Markdown back, and temporary checkpoint files are removed on failure.

If a language child process is killed while measuring a file, the latest
partial fragment includes `active_file`, 1-based `active_file_index`, and
`active_file_bytes`. Once measurement starts, it also includes `active_axis`,
`active_impl`, `active_phase`, and 1-based `active_attempt` when an attempt
number applies. `active_file` is the canonical active-measurement signal; these
fields are omitted on completed language rows. They exist so OOM and hard-kill
rows still identify the exact file and measurement phase even when no per-file
result could be checkpointed.

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

Summarize the tracked Wave 3 fleet status from the checked-in budget and the
language catalog:

```sh
cd cgo_harness
GOWORK=off go run ./cmd/perf_scan_status \
  -budget perf_scan/perf_ratio_budgets.json \
  -fleet tier_scan/exts.tsv \
  -out-json perf_scan/wave3_sweep_status.json \
  -out-md perf_scan/wave3_sweep_status.md
```

This status is ledger-grade: it reports budgeted fleet coverage, languages
held out of the ratchet, seed sources, and known budget-class gaps without
requiring git-ignored local scoreboard artifacts. To fold in local scoreboards
when they exist, add `-scoreboards 'perf_scan/out/wave3_batch*/scoreboard.json'`;
contended runs remain explicitly labeled as smoke/visibility evidence.

Compare a historical v1 scoreboard against the matching aggregate ratchet:

```sh
cd cgo_harness
GOWORK=off go run ./cmd/perf_scan_budget \
  -budget perf_scan/perf_ratio_budgets.json \
  -scoreboard perf_scan/out/authoritative_YYYYMMDDTHHMMSSZ/scoreboard.json \
  -require-all-budget-langs \
  -out-md perf_scan/out/authoritative_YYYYMMDDTHHMMSSZ/budget.md
```

The checked-in historical budget was seeded with `order=largest`,
`max_files=8`, `reps=5`, `warmup=1`, `file_budget_ms=10000`, and the former
in-process C transport. It remains historical context, not admissible v2
publication evidence. Generate the replacement static-C full-parse board with
the same corpus-selection knobs inside the parity container:

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

Evaluate the resulting v2 board with the current fail-closed contract:

```sh
cd cgo_harness
GOWORK=off go run ./cmd/perf_scan_budget \
  -budget perf_scan/perf_ratio_budgets.json \
  -scoreboard perf_scan/out/ratchet_YYYYMMDDTHHMMSSZ/scoreboard.json \
  -hard-gate-only \
  -out-md perf_scan/out/ratchet_YYYYMMDDTHHMMSSZ/hard-gate.md
```

The budget command deliberately keeps the transports separate: v1 boards may
only be compared with the historical aggregate ratchets, while v2 boards may
only be evaluated with `-hard-gate-only`. A v2 board contains exactly the
`full` axis, so applying old `noedit` allowances or ratios to it would be a
false comparison.

For a targeted v2 language refresh, scope the hard-gate evaluation:

```sh
cd cgo_harness
GOWORK=off go run ./cmd/perf_scan_budget \
  -budget perf_scan/perf_ratio_budgets.json \
  -scoreboard perf_scan/out/go_refresh/scoreboard.json \
  -hard-gate-only \
  -langs go
```

The checker first applies the independent hard rule to every file: each full
axis must be `ok`, contain positive Go/C timings, and have ratio `<=10.0x`;
every structured or legacy Go stop fails on every axis. It then applies the
historical aggregate ratchets (`ratio_by_total`, optional
`ratio_median_of_files`, timeout/error counts, and C-reference allowances).
Those historical allowances cannot waive the hard rule. Strict config also
requires `hard_gate=true` and the authenticated corpus-lock digest in addition
to the measurement knobs (`reps`, `warmup`, `file_budget_ms`, `max_files`,
`order`, exclusions, and axes).

The universal hard-gate run passes `-hard-gate-only`. That mode requires a v2,
full-only,
unexcluded scoreboard and checks authenticated fleet coverage plus the exact
per-file hard rules without applying historical aggregates whose seeded sample
basis included an exclusion. Historical ratchets remain available through the
normal comparator on scoreboards produced with their exact recorded basis.

Older `gts-perf-scan/v1` scoreboards remain historical analysis only. They do
not carry the fully static oracle identity or per-file cgo/static admission and
cannot establish a new hard-gate pass or enter the v2 shard reducer.

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
