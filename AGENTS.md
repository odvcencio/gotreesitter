## Agent Workflow: GLR Parity + Performance

This file defines how agents should work in this repo.

### 1) Non-negotiables
- `canopy` (successor to `gts-suite`) is the structural-analysis tool of choice for inspection, review, dead-code, impact, and hotspot runs. The earlier memory-pressure caveats no longer apply — run it freely on host.
- Do not run repo-wide `go test ./...` or `go test ./... -race` on the host. Broad host sweeps can OOM developer machines and make attribution harder.
- For heavy correctness, parity, or race coverage, use Docker isolation only and keep runs to one language/grammar at a time.
- Keep correctness gating separate from performance gating.
- Prefer reproducible runs over ad-hoc spot checks.
- When chasing an OOM, keep narrowing the workload until one language and one suite remain.

### 2) Correctness Gate (must stay green)
Run before and after performance changes:
- Focused package/unit tests inside Docker, scoped with `-run` whenever possible.
- Parity-focused Docker suites under `cgo_harness`, one language at a time.

When changing GLR/incremental logic, require parity validation first, then perf validation.

### 3) Standard Perf Loop
Use this loop for optimization work:
1. Baseline with stable settings.
2. Make one focused change.
3. Re-run the same benchmarks.
4. Keep changes only if `benchstat` improves target metrics without correctness regressions.

Stable settings:
- `GOMAXPROCS=1`
- `-count=10`
- `-benchtime=750ms`
- `-benchmem`

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

Current readiness targets for PR merge:
- Full parse: within `2x` of C/cgo baseline on agreed macro workload.
- Incremental single-byte edits: at or better than C/cgo baseline.
- No memory strangulation or crash behavior under large corpus runs.

### 5) Attribution for Incremental Hot Path
When profiling incremental edits, split attribution into:
- `Tree.Edit(edit)`
- reuse-cursor/reuse-selection work
- reparse/rebuild work

Use profiled runs to decide whether the next win comes from:
- reuse/invalidation scope,
- GLR/recovery/materialization cost,
- or allocator sizing/retention.

### 6) Commit Discipline
- Keep commits scoped and bisectable.
- Drop scratch/debug artifacts before commit.
- Use project commit flow:
  - `git add ...`
  - `buckley commit --yes --minimal-output`

### 7) Gate Presets
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
- `GOMAXPROCS=1 go test . -run '^$' -bench 'BenchmarkGoParseFullDFA|BenchmarkGoParseIncrementalSingleByteEditDFA|BenchmarkGoParseIncrementalNoEditDFA' -benchmem -count=10 -benchtime=750ms`

Full-parse non-truncation probe:
- `GOT_PARSE_NODE_LIMIT_SCALE=3` may be used for diagnostic full-parse runs when default node budget truncates benchmark/parity cases.
- `GOT_GLR_MAX_STACKS=...` overrides the default GLR stack cap (default 6).
