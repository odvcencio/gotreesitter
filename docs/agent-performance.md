# Correctness and performance workflow

Read before optimization, GLR/incremental changes, heavy correctness/parity/race work, benchmark comparisons, or releases.

### Correctness Gate (must stay green)
Run before and after performance changes:
- Focused package/unit tests inside Docker, scoped with `-run` whenever possible.
- Parity-focused Docker suites under `cgo_harness`, one language at a time.

When you change GLR/incremental logic, validate parity first, then validate performance.

### 3) Standard Perf Loop
Use this loop for optimization work:
1. Baseline with stable settings.
2. Make one focused change.
3. Re-run the same benchmarks.
4. Keep changes only if `benchstat` improves target metrics without correctness regressions.

Stable settings:
- `GOMAXPROCS=1`
- `-count=1` per process
- one process per explicit shuffle seed
- 20 shuffle seeds by default
- `-benchtime=750ms`
- `-benchmem`

Use `scripts/run_randomized_benchmarks.sh` for every before-and-after
comparison. Do not use a fixed-order benchmark run as comparison evidence.

Primary bench trio:
- `BenchmarkGoParseFullDFA`
- `BenchmarkGoParseIncrementalSingleByteEditDFA`
- `BenchmarkGoParseIncrementalNoEditDFA`

### 4) Metrics and Targets
Track at minimum:
- `ns/op`
- `B/op`
- `allocs/op`
- Max RSS on large-file runs (`/usr/bin/time -v`)

Release-blocking performance safety requirements:
- Do not permit a crash, an out-of-memory failure, or runaway memory retention.
- Preserve the parser memory-budget contracts.
- Complete the benchmark suite and publish reproducible evidence.

Directional performance goals:
- Full parse: within `2x` of C/cgo baseline on agreed macro workload.
- Incremental single-byte edits: at or better than C/cgo baseline.
- Improve `ns/op`, `B/op`, allocation count, and maximum resident set size.

The directional goals do not block a merge or a release. Record each
regression and explain its tradeoff. Continue the optimization work after the
release when correctness, portability, and depth justify the change.

### 5) Attribution for Incremental Hot Path
When profiling incremental edits, split attribution into:
- `Tree.Edit(edit)`
- reuse-cursor/reuse-selection work
- reparse/rebuild work

Use profiled runs to decide whether the next win comes from:
- reuse/invalidation scope,
- GLR/recovery/materialization cost,
- or allocator sizing/retention.

### Gate Presets
Correctness preset:
- `bash cgo_harness/docker/run_parity_in_docker.sh -- "cd /workspace && go test ./grammargen -run '^TestName$' -count=1"` for the smallest regression that covers the change
- `bash cgo_harness/docker/run_single_grammar_parity.sh <grammar>`
- `bash cgo_harness/docker/run_grammargen_focus_targets.sh --mode real-corpus --langs <language>`
- `bash cgo_harness/docker/run_grammargen_focus_targets.sh --mode cgo --langs <language>`
- Do not batch multiple languages together while diagnosing regressions or OOMs.

Race preset:
- CI or dedicated-container only.
- Do not run host-side repo-wide race sweeps while diagnosing OOMs.

Perf preset (stable settings):
- `bash scripts/run_randomized_benchmarks.sh --output /tmp/gotreesitter-bench.txt`

Full-parse non-truncation probe:
- `GOT_PARSE_NODE_LIMIT_SCALE=3` may be used for diagnostic full-parse runs when default node budget truncates benchmark/parity cases.
- `GOT_GLR_MAX_STACKS=...` overrides the default GLR stack cap of 8.
