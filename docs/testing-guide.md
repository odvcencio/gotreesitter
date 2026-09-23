# Testing

```sh
bash cgo_harness/docker/run_single_grammar_parity.sh typescript
```

For local correctness/parity work, prefer isolated one-language Docker runs:

```sh
# Real-corpus parity for one grammar
bash cgo_harness/docker/run_single_grammar_parity.sh typescript

# Focused grammargen real-corpus lane for one language
bash cgo_harness/docker/run_grammargen_focus_targets.sh --mode real-corpus --langs typescript

# Focused grammargen-vs-C lane for one language
bash cgo_harness/docker/run_grammargen_focus_targets.sh --mode cgo --langs typescript
```

`run_grammargen_focus_targets.sh` is the safest local lane for high-value
grammars: it runs one grammar per container and defaults to a
single-worker profile (`--cpus 1`, `--pids 512`, `GOMAXPROCS=1`,
`GOFLAGS=-p=1`).

For Fortran, both real-corpus runners also default to a tighter bounded
local preset unless you explicitly override it or pass
`--unsafe-fortran-defaults`: `--memory 3g`, `--cpus 1`, `--pids 512`,
`GOMAXPROCS=1`, `GOFLAGS=-p=1`, `GOT_LALR_LR0_CORE_BUDGET=160000000`, and
`GTS_GRAMMARGEN_REAL_CORPUS_GENERATE_TIMEOUT=15m`.

If you only need a fast package-local regression check, keep it in Docker
and narrow the `-run` regex:

```sh
bash cgo_harness/docker/run_parity_in_docker.sh \
  -- "cd /workspace && go test ./grammargen -run '^TestTypeScriptConditionalTypeParity$' -count=1"
```

Avoid `go test ./...` and host-side multi-language or race sweeps on
developer machines while chasing OOMs. Use CI or a dedicated container when
you need broader race coverage.

Other focused correctness/parity commands:

```sh
# Top-50 smoke correctness for the grammars package only
bash cgo_harness/docker/run_parity_in_docker.sh \
  -- "cd /workspace && go test ./grammars -run '^TestTop50(ParseSmokeNoErrors|CorrectnessListMatchesLockFile)$' -count=1 -v"

# Top-50 grammargen import/parity registry coverage
bash cgo_harness/docker/run_parity_in_docker.sh \
  -- "cd /workspace && go test ./grammargen -run '^TestTop50GrammarImportParityCoverage$' -count=1 -v"

# C-oracle parity suites inside the cgo harness
bash cgo_harness/docker/run_parity_in_docker.sh \
  --run '^TestParityFreshParse$|^TestParityHasNoErrors$|^TestParityIssue3Repros$|^TestParityGLRCanaryGo$'
bash cgo_harness/docker/run_parity_in_docker.sh \
  --run '^TestParityCorpusFreshParse$'
```

CI may still run broader race coverage on hosted runners. Do not copy those
commands onto a developer host during OOM diagnosis.

Test suite covers: smoke tests (206 grammars), golden S-expression snapshots, highlight query validation, query pattern matching, incremental reparse correctness, error recovery, GLR fork/merge, injection parsing, source rewriting, and fuzz targets.

See [AGENTS.md](../AGENTS.md) for the full agent workflow (correctness gates, perf loop, and gate presets) that governs how changes are validated before merge.
