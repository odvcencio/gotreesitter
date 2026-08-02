# cgo_harness

This module contains CGo-only parity and baseline benchmark harnesses used to compare `gotreesitter` against native C tree-sitter parsers.

## Unified Harness Gate

Do not start local OOM diagnosis with the unified gate runner. It aggregates
broad correctness, parity, and perf work and makes it harder to identify which
language is responsible for a memory spike.

For local work, start with one language per container:

```sh
bash cgo_harness/docker/run_single_grammar_parity.sh typescript
bash cgo_harness/docker/run_grammargen_focus_targets.sh --mode real-corpus --langs typescript
bash cgo_harness/docker/run_grammargen_focus_targets.sh --mode cgo --langs typescript
```

Use the unified gate runner only in CI or deliberate lab-style sweeps:

```sh
go run ./cmd/harnessgate -mode all
```

This executes:

- root correctness (`go test ./... -count=1`)
- curated cgo parity suites
- stable perf trio (optionally benchgate-compared to a baseline)

Artifacts are written under `harness_out/`.

Optional weighted confidence scoring can be enabled from `harnessgate` using
either a built-in profile (`top50`, `core90`) or a custom manifest JSON:

```sh
go run ./cmd/harnessgate -mode correctness \
  -real-corpus-dir cgo_harness/corpus_real \
  -real-corpus-langs top10 \
  -confidence-profile core90 \
  -confidence-min 0.90
```

Framework details (oracles, corpus tiers, gate policy):

- `cgo_harness/HARNESS_FRAMEWORK.md`

## Run Parity Tests

Default parity runs use `smoke` mode: a small representative subset that is
fast enough for CI. For local OOM diagnosis, prefer the single-language Docker
commands above instead of broad host-side sweeps. The direct `go test` examples
below are best treated as CI/lab references, not the default local workflow.

```sh
go test . -tags treesitter_c_parity \
  -run '^TestParityFreshParse$|^TestParityIncrementalParse$|^TestParityHasNoErrors$|^TestParityIssue3Repros$|^TestParityGLRCanaryGo$|^TestParityGLRCanarySet$|^TestParityGLRCapPressureTopLanguages$|^TestParityGateCoverageRatchet$|^TestParityHighlight$' \
  -count=1 -v
```

Set `GTS_PARITY_MODE=top50` for the top-50 correctness lock set, or
`GTS_PARITY_MODE=exhaustive` for the full curated sweep and the larger
diagnostic suites:

```sh
GTS_PARITY_MODE=top50 \
go test . -tags treesitter_c_parity \
  -run '^TestParityFreshParse$|^TestParityIncrementalParse$|^TestParityHasNoErrors$|^TestParityTop50ParseSmoke$|^TestParityTop50ParseMaterializationTrends$' \
  -count=1 -v

GTS_PARITY_MODE=exhaustive \
go test . -tags treesitter_c_parity \
  -run '^TestParityFreshParse$|^TestParityIncrementalParse$|^TestParityHasNoErrors$|^TestParityIssue3Repros$|^TestParityGLRCanaryGo$|^TestParityGLRCanarySet$|^TestParityGLRCapPressureTopLanguages$|^TestParityGateCoverageRatchet$|^TestParityHighlight$|^TestParityHighlightAllGrammars$' \
  -count=1 -v

GTS_PARITY_MODE=exhaustive \
go test . -tags treesitter_c_parity \
  -run '^TestParityCorpusFreshParse$' \
  -count=1 -v

GTS_PARITY_MODE=exhaustive \
go test . -tags treesitter_c_parity \
  -run '^TestParityYAMLCorpus$|^TestParityYAMLCorpusStructural$|^TestParityYAMLCorpusSummary$' \
  -count=1 -v
```

## Run Top-50 Parity Benchmarks

`BenchmarkParityTop50ParseFull` prechecks gotreesitter-vs-C structural parity
for each selected language, then benchmarks both parsers side by side. Keep
local diagnosis narrow with `GTS_PARITY_BENCH_LANGS`; omit it only for CI/lab
top-50 sweeps.

```sh
GOMAXPROCS=1 GTS_PARITY_MODE=top50 GTS_PARITY_BENCH_LANGS=java,python,rust \
go test . -tags treesitter_c_parity -run '^$' \
  -bench '^BenchmarkParityTop50ParseFull/' \
  -benchmem -count=10 -benchtime=750ms
```

## Run Locked Canonical Incremental Benchmarks

`BenchmarkParityGoCanonicalIncremental` uses a versioned, hash-locked matrix
that distinguishes unchanged-snapshot identity, same-length leaf validation,
real-code incremental GLR work, recovery, and stateful external-scanner edits.
Every changed-source lane runs in both directions and requires fresh Go = C and
C incremental = fresh C, including exact deep digests, full spans, and accepted
stop state. Timing-eligible lanes also require Go incremental = fresh Go.
Runtime/profile assertions prove the named mechanism actually ran and reject
any unplanned full-parse fallback. The manifest never searches dynamically for
an easier edit.

The locked `language.go` length-change row proves the bounded accepted-error
retry path in both directions. The retry retains the edited old-tree route,
uses the ordinary fresh-parse merge policy, and is admitted only when the
second result is strictly better. The receipt records the attempt, adoption,
merge cap, typed cause, and route alongside exact four-way parity evidence.

The unchanged-snapshot identity lane, small Go leaf fixture, and Python scanner
fixture are semantic controls. They remain timing eligible but are reported
separately and must not be included in representative real-code aggregates or
performance headlines. The representative aggregate contains only the four
genuine real-code edit rows.

```sh
bash cgo_harness/docker/run_parity_in_docker.sh --cpus 1 -- \
  "cd /workspace/cgo_harness && GOMAXPROCS=1 go test . \
    -tags treesitter_c_parity -run '^$' \
    -bench '^BenchmarkParityGoCanonicalIncremental/' \
    -benchmem -count=10 -benchtime=750ms"
```

Run the same admission without timing and atomically publish its machine-readable
before-state receipt under `cgo_harness` with:

```sh
bash cgo_harness/docker/run_parity_in_docker.sh --cpus 1 -- \
  "cd /workspace/cgo_harness && GOMAXPROCS=1 \
    GTS_CANONICAL_GO_INCREMENTAL=1 \
    GTS_CANONICAL_INCREMENTAL_RECEIPT_OUT=testdata/canonical_incremental_before_state_receipt_v1.json \
    go test . \
    -tags treesitter_c_parity \
    -run '^TestCanonicalGoIncrementalParity$' -count=1 -v"
```

The receipt destination is removed before admission and replaced through a
same-directory atomic rename only after every lane matches its locked expected
outcome. This prevents a failed run from leaving a stale artifact while still
publishing only a fully admitted `status: passed` closure receipt. Paths outside
`cgo_harness` are rejected.

## Run Dynamic Real-Corpus Parser Benchmarks

`BenchmarkParityRealCorpusParse*` uses `cgo_harness/corpus_real/<language>`
fixtures and compares gotreesitter against the C tree-sitter runtime for full
parse, single-byte incremental edit, and no-edit incremental parse. Strict
structural parity is the default precheck.

```sh
GOMAXPROCS=1 GTS_REAL_CORPUS_BENCH_LANGS=go \
go test . -tags treesitter_c_parity -run '^$' \
  -bench '^BenchmarkParityRealCorpusParse(Full|IncrementalSingleByteEdit|IncrementalNoEdit)/go/' \
  -benchmem -count=10 -benchtime=750ms
```

Useful narrow-run knobs:

- `GTS_REAL_CORPUS_BENCH_LANGS=c,cpp,c_sharp`
- `GTS_REAL_CORPUS_BENCH_ORDER=path|largest|smallest`
- `GTS_REAL_CORPUS_BENCH_MAX_FILES=1`
- `GTS_REAL_CORPUS_BENCH_MAX_FILE_BYTES=20000`
- `GTS_REAL_CORPUS_BENCH_SKIP_MISMATCH=1` to benchmark only parity-clean files.
- `GTS_REAL_CORPUS_BENCH_ALLOW_MISMATCH=1` for timing-only diagnosis when the
  selected corpus exposes a known structural mismatch.
- `GOT_PARSE_PHASE_TIMING=1` to enable parser-loop, token, action, GLR stack,
  and result-selection/tree-build/finalization phase timing for full parses
  beyond the default large-Python diagnostic lane.

The gotreesitter incremental lanes also report parser attribution counters:
`edit_ns/op`, `parse_wall_ns/op`, `reuse_ns/op`, `reparse_ns/op`,
`unattributed_ns/op`, parser buckets such as `parser_loop_ns/op`,
`token_next_ns/op`, `action_dispatch_ns/op`, `action_lookup_ns/op`,
`action_apply_ns/op`, `glr_merge_ns/op`, and `glr_cull_ns/op`, reused
subtree/byte counts, reuse rejection counts, GLR stack iteration counts,
recovery counts, survivor-node counts, and result phase buckets such as
`result_tree_build_ns/op`, `result_compatibility_ns/op`,
`result_parent_link_ns/op`, and `normalization_ns/op`. The no-edit lane
reports zero parser work when the unchanged-tree fast path returns the previous
tree.

For cross-language optimization sweeps, use the Docker matrix runner. It runs
one language per container, enables phase timing by default, keeps the raw logs,
and writes a ranked JSON/Markdown report with Go/C ratios and top attribution
buckets:

```sh
bash cgo_harness/docker/run_real_corpus_bench_matrix.sh \
  --langs go,python,rust,java,javascript,typescript,c \
  --count 5 \
  --benchtime 750ms
```

Useful matrix diagnosis presets:

```sh
# Time only parity-clean files when a language has known corpus mismatches.
bash cgo_harness/docker/run_real_corpus_bench_matrix.sh \
  --langs rust,javascript \
  --skip-mismatch

# Timing-only probe for a bounded C-family lane.
bash cgo_harness/docker/run_real_corpus_bench_matrix.sh \
  --langs c \
  --allow-mismatch \
  --max-file-bytes 20000
```

To rebuild a report from existing logs:

```sh
cd cgo_harness
go run ./cmd/real_corpus_bench_report \
  -input ../harness_out/real_corpus_bench_matrix/<run>/docker \
  -out-md ../harness_out/real_corpus_bench_matrix/<run>/REAL_CORPUS_BENCH_REPORT.md
```

### Certify external-scanner retry suppression

Before adding an exact-blob runtime profile that suppresses the duplicated
external-scanner retry ladder, run the opt-in corpus certifier. It parses every
selected file twice: once with the repeat enabled and once with the candidate
profile, while preserving the complete accepted-error widening and final-merge
ladder in both paths. Stop reason, full span, error state, and the deep tree
digest must match file by file, while the candidate must eliminate retry
attempts and improve aggregate wall time by at least 2%. Every file is also
parsed by the locked C oracle; the baseline and candidate must preserve the
same oracle relation, including named pre-existing mismatch, crash, and timeout
rows. The JSON receipt is written on success or on the first counterexample.

```sh
cd cgo_harness
GOWORK=off GOMAXPROCS=1 \
GTS_PARITY_ALLOW_HOST=1 \
GTS_RETRY_PROFILE_CERT=1 \
GTS_RETRY_PROFILE_CERT_LANG=crystal \
GTS_RETRY_PROFILE_CERT_CANDIDATE_REVISION="$(git rev-parse HEAD)" \
GTS_RETRY_PROFILE_CERT_OUT=../harness_out/retry-profile-crystal.json \
GTS_REAL_CORPUS_BENCH_ROOT=/path/to/corpus_sources \
GTS_REAL_CORPUS_BENCH_LOCK=/path/to/corpus_sources.lock \
go test -tags 'treesitter_c_parity treesitter_c_perfscan gts_workcount' . \
  -run '^TestRetryProfileCorpusCertification$' -v -count=1 -timeout 2h
```

Use `GTS_REAL_CORPUS_BENCH_ORDER`, `GTS_REAL_CORPUS_BENCH_MAX_FILES`, and
`GTS_REAL_CORPUS_BENCH_MAX_FILE_BYTES` for bounded pilots. A passing pilot is
not a certification; the profile should be banked only after the locked full
corpus passes. Each completed file is also appended to
`$GTS_RETRY_PROFILE_CERT_OUT.files.jsonl`. Set
`GTS_RETRY_PROFILE_CERT_RESUME=1` to reuse only schema-v2 rows whose exact
candidate revision, clean Git worktree, blob, corpus selection and lock,
parser-affecting environment, and complete static-C oracle identity still
match. The certifier revalidates each resumed path, source hash, parse result,
class, deep digest, and oracle relation before accumulation, rejects rows no
longer selected by the corpus, and never resumes a counterexample. The
explicit candidate revision is required when Go build metadata does not expose
one and must resolve to the clean checkout's exact `HEAD`. C-oracle crashes and
watchdog stops remain explicit row statuses.

## Run Parity Tests In Docker Sandbox

This keeps heavy parity runs isolated from your host/WSL memory space and
captures container failure metadata (`OOMKilled`, exit code, state error).

```sh
chmod +x cgo_harness/docker/run_parity_in_docker.sh
cgo_harness/docker/run_parity_in_docker.sh \
  --memory 8g \
  --cpus 4

# Optional: exclude one or more languages from parity loops in this run.
GTS_PARITY_SKIP_LANGS=scala \
  cgo_harness/docker/run_parity_in_docker.sh \
  --memory 8g \
  --cpus 4

# Optional: force the full exhaustive sweep inside the container.
GTS_PARITY_MODE=exhaustive \
  cgo_harness/docker/run_parity_in_docker.sh \
  --memory 8g \
  --cpus 4
```

Run strict Scala real-world parity in the same sandbox:

```sh
cgo_harness/docker/run_parity_in_docker.sh \
  --memory 8g \
  --cpus 4 \
  --strict-scala
```

Run against a specific worktree/repo root:

```sh
cgo_harness/docker/run_parity_in_docker.sh \
  --repo-root /path/to/worktree \
  --label glr-exp-a \
  --memory 8g \
  --cpus 4
```

Artifacts are written to `<out-root>/<timestamp>[-label]/` (default out-root is
`<repo-root>/harness_out/docker`):

- `container.log`
- `inspect.json`
- `metadata.txt`

## Run Multiple Worktree Experiments In Parallel

Use the experiment runner to fan out 2-3 bounded containers across different
worktrees while preserving per-experiment artifacts/metadata.

```sh
chmod +x cgo_harness/docker/run_parity_experiments.sh
cgo_harness/docker/run_parity_experiments.sh \
  --experiment main=/home/me/work/gotreesitter \
  --experiment glr-a=/home/me/work/gts-glr-a \
  --experiment glr-b=/home/me/work/gts-glr-b \
  --max-parallel 2 \
  --memory 6g \
  --cpus 2
```

You can also provide a custom command (applied to each experiment):

```sh
cgo_harness/docker/run_parity_experiments.sh \
  --experiment scala=/home/me/work/gts-scala \
  --max-parallel 1 \
  -- "cd /workspace/cgo_harness && GTS_PARITY_SCALA_REALWORLD_STRICT=1 go test . -tags treesitter_c_parity -run '^TestParityScalaRealWorldCorpus$' -count=1 -v"
```

Optional Scala real-world structural parity probe:

```sh
go test . -tags treesitter_c_parity \
  -run '^TestParityScalaRealWorldCorpus$' \
  -count=1 -v
```

Scala real-world probe modes:

- default: regression ratchet against pinned divergence baselines with stable budgets (`GOT_PARSE_NODE_LIMIT_SCALE=3`, `GOT_GLR_MAX_STACKS=8` unless already set)
- strict: exact parity required (zero divergences + no Go error nodes)

Strict mode command:

```sh
GTS_PARITY_SCALA_REALWORLD_STRICT=1 \
  go test . -tags treesitter_c_parity \
  -run '^TestParityScalaRealWorldCorpus$' \
  -count=1 -v
```

## Run Parity Breaker Sweeps (Opt-In)

Use breaker sweeps to aggressively search for structural/highlight parity
regressions via deterministic source mutations and optional real-corpus runs.
These tests are disabled by default.
They are discovery-oriented and may intentionally fail until divergences are
burned down.

```sh
cd cgo_harness
GTS_PARITY_BREAKER=1 \
GTS_PARITY_BREAKER_MAX_LANGS=50 \
GTS_PARITY_BREAKER_MAX_MUTATIONS=12 \
go test . -tags treesitter_c_parity \
  -run '^TestParityMutationSweepStructural$|^TestParityMutationSweepHighlight$' \
  -count=1 -v
```

Common controls:

- `GTS_PARITY_BREAKER_LANGS=go,scala,...` explicit language allow-list.
- `GTS_PARITY_BREAKER_PIN_LANGS=scala,...` force-priority languages into capped runs.
- `GTS_PARITY_BREAKER_MAX_LANGS=<n>` cap selected language count after filters.
- `GTS_PARITY_BREAKER_SHARDS=<n>` and `GTS_PARITY_BREAKER_SHARD_INDEX=<0..n-1>` deterministic sharding for parallel matrix runs.
- `GTS_PARITY_BREAKER_INCLUDE_DEGRADED=1` include known degraded languages in sweep selection.
- `GTS_PARITY_SKIP_LANGS=scala,...` exclude languages from parity loops (fresh/incremental/highlight/breaker).

Example: 3-way sharded matrix with Scala pinned across shards:

```sh
cd cgo_harness
for i in 0 1 2; do
  GTS_PARITY_BREAKER=1 \
  GTS_PARITY_BREAKER_SHARDS=3 \
  GTS_PARITY_BREAKER_SHARD_INDEX="$i" \
  GTS_PARITY_BREAKER_PIN_LANGS=scala \
  GTS_PARITY_BREAKER_MAX_LANGS=40 \
  GTS_PARITY_BREAKER_MAX_MUTATIONS=12 \
  go test . -tags treesitter_c_parity \
    -run '^TestParityMutationSweepStructural$' \
    -count=1 -v &
done
wait
```

Optional real-corpus structural sweep from a generated manifest:

```sh
cd cgo_harness
GTS_PARITY_BREAKER=1 \
GTS_PARITY_BREAKER_CORPUS_MANIFEST=../harness_out/corpus_degraded7/manifest.json \
GTS_PARITY_BREAKER_CORPUS_MAX_FILES=2 \
GTS_PARITY_BREAKER_CORPUS_MAX_BYTES=65536 \
go test . -tags treesitter_c_parity \
  -run '^TestParityBreakerRealCorpusStructural$' \
  -count=1 -v
```

## Run Corpus Parity (`dump.v1`)

This command compares `gotreesitter` vs the native C oracle, emits `dump.v1`
artifacts for both runtimes, writes JSONL results, and updates `PARITY.md`.

```sh
go run -tags treesitter_c_parity ./cmd/corpus_parity \
  --lang top10 \
  --corpus ./corpus \
  --out ./parity_out/results.jsonl \
  --artifact-dir ./parity_out/dump_v1 \
  --artifact-mode failures \
  --scoreboard ./PARITY.md
```

Notes:

- `--lang` accepts `top10` (default), a single language (`go`), or a comma-separated list.
- For multiple languages, corpus layout is `--corpus/<language>/**`.
- For a single language (`--lang go`), `--corpus` can point directly at that language directory.
- `--artifact-mode failures` is recommended for large real-corpus sweeps; it keeps dump artifacts only for failing files.
- `--fail-on-mismatch` is recommended for gate runs; it still writes JSONL and artifacts, then exits non-zero if any row has `pass=false`.
- `--workers N` parallelizes files within each language with one Go parser and one C parser per worker. The default is `1`; use higher values only inside a memory-bounded container and pair them with matching CPU/GOMAXPROCS limits. JSONL output remains sorted by input file order.

## Build Real Corpus (Lock-Pinned)

Use the corpus builder to materialize production-grade real corpus fixtures from
`grammars/languages.lock` pinned upstream commits:

```sh
go run ./cgo_harness/cmd/build_real_corpus \
  -profile cgo_harness/testdata/top50_manifest.json \
  -out cgo_harness/corpus_real
```

Notes:

- Selection is deterministic and bucketed (`small`, `medium`, `large`) per language.
- Selection targets `small`/`medium`/`large` buckets per language, with deterministic fallback when one bucket has no candidates.
- Combine `-profile` with `-langs` to rebuild a validated profile subset.
- The builder clears each selected language directory before it writes files.
- Use `-merge-existing` to retain unselected manifest entries during a subset refresh.
- Source files are pulled from pinned upstream commits and recorded in
  `cgo_harness/corpus_real/manifest.json` with SHA256 + source path metadata.
- Validate corpus quality bar:

```sh
cd cgo_harness
GTS_REAL_CORPUS_MANIFEST=corpus_real/manifest.json \
  go test . -run TestRealCorpusManifestQuality -count=1
```

- Use this corpus with the parity runner:

```sh
go run ./cmd/harnessgate -mode correctness \
  -real-corpus-dir cgo_harness/corpus_real \
  -real-corpus-langs top50
```

- Produce an explicit L3/L4 board from the manifest + parity results:

```sh
go run ./cmd/real_corpus_board \
  --manifest cgo_harness/corpus_real/manifest.json \
  --results harness_out/03_real_corpus_results.jsonl \
  --out-json harness_out/03_real_corpus_board.json \
  --out-md harness_out/03_real_corpus_board.md \
  --l4-limit 20
```

Notes:

- `L3` is all `medium` entries present in the built manifest.
- `L4` is all `large` entries by default, or the top `N` heavy-duty languages by
  max large-file bytes when `--l4-limit` is set.
- `cmd/harnessgate` can generate the same board directly when passed
  `-real-corpus-manifest` and optional `-real-corpus-l4-limit`.

## Focused Grammargen Targets

For the current high-value grammargen lane, use the focused Docker runner:

```sh
bash cgo_harness/docker/run_grammargen_focus_targets.sh --mode real-corpus --langs typescript
bash cgo_harness/docker/run_grammargen_focus_targets.sh --mode cgo --langs typescript
```

It limits work to `javascript`, `typescript`, `tsx`, `c`, `cpp`, `c_sharp`,
`cobol`, and `fortran`; keep local diagnosis to one `--langs` value at a time.
Each real-corpus grammar runs in its own container, and the direct
grammargen-vs-C oracle also runs one language at a time. That keeps OOMs
contained to a single target instead of killing the host. `fortran` is
currently real-corpus-only because the direct C-oracle harness does not expose
it yet.

For Fortran real-corpus work, the single-grammar runner and focused target lane
now default to a tighter bounded preset unless you explicitly override it or
pass `--unsafe-fortran-defaults`: `--memory 3g`, `--cpus 1`, `--pids 512`,
`GOMAXPROCS=1`, `GOFLAGS=-p=1`, `GOT_LALR_LR0_CORE_BUDGET=160000000`, and
`GTS_GRAMMARGEN_REAL_CORPUS_GENERATE_TIMEOUT=15m`.

The underlying real-corpus test accepts a durable checkout root through
`GTS_GRAMMARGEN_REAL_CORPUS_ROOT`; use
`GTS_GRAMMARGEN_REAL_CORPUS_ONLY=<grammar>` to keep a direct run scoped to one
grammar:

```sh
GTS_GRAMMARGEN_REAL_CORPUS_ENABLE=1 \
GTS_GRAMMARGEN_REAL_CORPUS_ROOT=/path/to/grammar-checkouts \
GTS_GRAMMARGEN_REAL_CORPUS_ONLY=markdown_inline \
go test ./grammargen \
  -run '^TestMultiGrammarImportRealCorpusParity$' -count=1 -v
```

For the Docker runners, pass `--seed-dir <path>` to either
`run_grammargen_real_corpus_in_docker.sh` or
`run_grammargen_focus_targets.sh`; the seed directory must be under the
repository root and is copied into the container's configured corpus root.
Split-grammar repositories such as Markdown and XML need no additional flag:
the test probes the grammar package root above `src/grammar.json` for its corpus
before falling back to the repository root.

## Run C Baseline Benchmarks

```sh
GOMAXPROCS=1 go test . -run '^$' -tags treesitter_c_bench \
  -bench 'BenchmarkCTreeSitterGoParseFull|BenchmarkCTreeSitterGoParseIncrementalSingleByteEdit|BenchmarkCTreeSitterGoParseIncrementalNoEdit' \
  -benchmem -count=10 -benchtime=750ms
```

`treesitter_c_bench` and `treesitter_c_parity` use the same
`COracleLanguage` binding and the exact grammar commit from
`grammars/languages.lock`. The Go baseline therefore no longer uses the
unrelated smacker runtime or its bundled grammar. This in-process transport is
for parity and diagnostic calibration: the upstream runtime is statically
included in the Go test binary, while the locked grammar is compiled with
`-O2` and loaded as a shared object. It is not the static publication oracle.

Inspect the full runtime, grammar, compiler, linkage, artifact path and SHA-256
before measuring:

```sh
go test . -tags treesitter_c_bench \
  -run '^TestCOracleContractPreflight$' -count=1 -v
```

These harnesses are intentionally kept in a separate module so the root
`gotreesitter` module remains pure-Go in dependency metadata. Historical
smacker-backed corpus probes remain behind the explicit
`treesitter_c_bench_legacy` tag and are not admissible for C/Go ratio claims.

## Run Pure-C Runtime Matrix (No CGo)

This compares against the tree-sitter C runtime compiled directly with `gcc`/`g++` and does not execute through Go cgo bindings.

```sh
./pure_c/run_matrix.sh
```

The matrix currently runs full-parse benchmarks for:

- `c`
- `go`
- `java`
- `html`
- `lua`
- `toml`
- `yaml`

## Run Pure-C Go Incremental Benchmark (No CGo)

This is the publication C oracle. It checks out and records these exact inputs:

- `github.com/tree-sitter/go-tree-sitter` `v0.25.0`, binding commit
  `adc13ffd8b2c0b01b878fda9f7c422ce0df5fad3`;
- upstream tree-sitter runtime commit
  `f5afe475deb7c0bae6407fb776c76824f717bb61` (`0.25.1`);
- tree-sitter-go commit from `grammars/languages.lock`, currently
  `2346a3ab1bb3857b48b29d779a1ef9799a248cd7`.

The runtime and grammar are compiled with fixed `-O2` flags, archived, and
statically linked into one standalone benchmark executable. The platform C
library still follows the host toolchain. The runner prints compiler identity,
source hashes, artifact path/SHA-256, and a `gts-deep-tree-v1` structural digest
before timing. Set `GTS_C_ORACLE_EXPECTED_DEEP_SHA256` to require a digest
already admitted by gotreesitter and the cgo parity transport.

The synthetic LR control reproduces full parse, incremental single-byte edit,
random-edit, and no-edit numbers:

```sh
./pure_c/run_go_benchmark.sh
```

Optional arguments:

```sh
./pure_c/run_go_benchmark.sh <func_count> <full_iters> <inc_iters> [source.go]
```

Example:

```sh
./pure_c/run_go_benchmark.sh 500 2000 20000
```

Passing a real Go source file runs the full-parse lane only. The artifact does
one untimed warm parse and deep validation before its internal timing loop:

```sh
./pure_c/run_go_benchmark.sh 500 2000 20000 ../query_compile.go
```

Validate that the static and cgo transports produce the frozen deep digest on
all four canonical fixtures:

```sh
go test . -tags treesitter_c_bench \
  -run '^TestCOracleStaticDeepParity$' -count=1 -v
```

From the repository root, the strict publication driver performs that
admission, materializes the authenticated real-Go fixture matrix, calibrates
the static C loop, and collects ten pinned-core Go-C-C-Go samples per backend
and fixture:

```sh
bash cgo_harness/pure_c/run_canonical_go_full_parse.sh --core <idle-cpu>
```

Publication mode requires a clean worktree, a quiet Docker-capable host, and no
parser or Go runtime tuning overrides. Short smoke runs require `--diagnostic`;
their receipts are explicitly marked `NONPUBLICATION_DIAGNOSTIC`.

## Run the Authenticated Work-Count Board

The four-fixture work-count board compares diagnostic Go and locked static-C
parser events without timing either instrumented artifact. Its versioned event
contract records whether each engine's value is present or unavailable and
whether the two definitions are comparable. A present zero is distinct from a
missing hook; a Go/C ratio is emitted only for comparable, present values with
a nonzero C denominator.

```sh
cd cgo_harness
GTS_WORK_COUNT_BOARD=1 \
GTS_WORK_COUNT_BOARD_RECEIPT=/tmp/gotreesitter-work-count-board.json \
go test -tags 'treesitter_c_parity treesitter_c_perfscan' . \
  -run '^TestAuthenticatedFourFixtureWorkCountBoard$' -count=1 -v
```

Authoritative receipts require a clean source tree. The admission verifies all
four frozen source and deep-tree identities, locked grammar and runtime inputs,
static linkage, identical tagged/untagged Go scheduling, and absence of
diagnostic scaffolding in the ordinary Go build. Instrumented artifacts are
always marked timing-ineligible. `instrumentation_status` is `blocked` until
every mandatory event has a paired direct hook with one shared semantic
definition. Independently, `work_audit_status` is `findings` when a comparable,
present mandatory-event ratio falls outside the inclusive `[0.8,1.2]` band;
those findings do not change instrumentation completeness.

## Run Go Head-to-Head Comparison

This runs both:

- `gotreesitter` Go benchmarks
- pure-C runtime benchmark (no cgo)

```sh
./pure_c/run_go_head_to_head.sh
```

## Run Multi-Language Head-to-Head Matrix

This runs:

- pure-C runtime matrix (`c`, `go`, `java`, `html`, `lua`, `toml`, `yaml`)
- matching `gotreesitter` benchmarks
- a combined summary table with per-language speedup ratios

```sh
./pure_c/run_matrix_head_to_head.sh
```

## Run Full Claim Suite (3-way, multi-size, repeated)

This runs repeated benchmarks across:

- `gotreesitter` (pure Go)
- tree-sitter C runtime via cgo bindings
- tree-sitter C runtime compiled directly with GCC (no cgo)

and generates a median-based report.

```sh
./pure_c/run_claim_suite.sh
```

Tunable inputs:

```sh
RUNS=7 SIZES="500 2000 10000" CFLAGS_EXTRA="-march=native -flto" ./pure_c/run_claim_suite.sh
```

## Run the Derivation-Set Differential (Opt-In)

The derivation-set differential compares, at every accept, the compact core's
live derivation set against the reference runtime's live version set. It is
stage D0 of `spec.derivation-set-equivalence.v1`.

The compact side records the set behind the `gts_derivation_set_census` build
tag. The default build carries no census code at all, so the shipped parse
path is unchanged. The C side reconstructs the version set from
`ts_parser_set_logger` output alone; the vendored C source stays unpatched.

```sh
cd cgo_harness
GOFLAGS=-buildvcs=false \
GTS_PARITY_ALLOW_HOST=1 \
GTS_PARITY_C_REF_BUILD_CACHE="$PWD/../harness_out/parity_c_ref_cache" \
CGO_ENABLED=1 \
go test -tags "cgo treesitter_c_parity gts_derivation_set_census" \
  -run '^TestDerivationSetDifferential' -count=1 -v -timeout 40m .
```

Two receipts run:

- `TestDerivationSetDifferentialWitnessReproduction` pins the classification
  of the eight witnesses the R2 measurement falsified.
- `TestDerivationSetDifferentialBaselineCensus` publishes the per-language
  baseline: EXTRA, MISSING, DIFFERENT and ORDER counts, plus the mechanism
  attribution. The constructed-source totals are pinned and reproduce on any
  host. The real-corpus totals are reported separately, because
  `cgo_harness/corpus_real` is a generated, gitignored fixture.
