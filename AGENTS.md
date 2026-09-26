## Agent Workflow: GLR Parity + Performance

This file defines how agents should work in this repo.

### v1 design

The owner's [v1 design](docs/v1-design.md) governs engine work and supersedes conflicting campaign guidance.
The full design is `hypha://m31labs/gotreesitter/specs/spec.gotreesitter-v1-design`.
Keep one active pull request (PR) per lane:

- Core: `sched`, `gss`, and `recover`.
- Incremental: `incr`.
- Output: `tree` and `compat`.
- Legacy fixes: Q items.

PRs that change only graduation allowlists and receipts do not occupy the core lane.
Add no language-name comparisons in engine files (D6). Start new engine code in `internal/` (D9).
Require incremental parsing to equal fresh Go parsing (D8). Use a fresh parse wherever this invariant remains unproven.
Check deterministic counters before randomized timing (D10). Work counters must not rise; reuse counters must not fall.
Put before-and-after counters in every engine PR body. Apply the design's 2% ledger threshold.

Run the invariant gate for every language that an engine PR can affect:

- Incremental parsing equals fresh parsing at every edit-session step.
- An `ERROR` root implies `HasError()`.
- The root covers the input, or the stop reason explains the gap.
- A reparse without edits allocates nothing.

Graduate each language through the allowlist after the E-A exit gate.
Flip the global default only after all top-50 languages graduate, and only in a release candidate (D7).
Keep O-Q1 through O-Q7 open. Q0 requires O-Q4. Section 8 implements O11 under organization decision 0012.

### 1) Non-negotiables
- Use `scripts/canopy_query.sh` for cached structural searches and analyses.
- Use direct `canopy ... --no-cache` commands for narrow searches of changed files.
- Use `rg` for literal text. Plain `canopy search grep` patterns start structural parsing.
- Scope structural Canopy searches to the smallest useful path.
- Apply `--limit` to structural searches.
- Do not run concurrent repository-wide Canopy searches.
- The query wrapper detects changed, missing, and new source files.
- The query wrapper uses a fresh scoped query when the cache is stale.
- Run `scripts/refresh_canopy_index.sh` before a broad search when the source set changed.
- The refresh script promotes its temporary cache only after a successful build and validation.
- Do not run repo-wide `go test ./...` or `go test ./... -race` on the host. Broad host sweeps can OOM developer machines and make attribution harder.
- For heavy correctness, parity, or race coverage, use Docker isolation only and keep runs to one language/grammar at a time.
- Keep correctness gating separate from performance gating.
- Prefer reproducible runs over ad-hoc spot checks.
- When chasing an OOM, keep narrowing the workload until one language and one suite remain.

### 2) Correctness Gate (must stay green)
Run before and after performance changes:
- Focused package/unit tests inside Docker, scoped with `-run` whenever possible.
- Parity-focused Docker suites under `cgo_harness`, one language at a time.

When you change GLR/incremental logic, validate parity first, then validate performance.

### 3) Standard Perf Loop
Use this loop for optimization work:
1. Baseline with stable settings.
2. Make one focused change.
3. Pass correctness and invariant gates, then compare deterministic counters.
4. Re-run the same randomized benchmarks and compare results with `benchstat`.
5. Apply the [v1 hard gates and ratchets](docs/v1-design.md#performance-gate-one-language). Record directional regressions and their tradeoffs.

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

Use the [v1 performance gate](docs/v1-design.md#performance-gate-one-language)
for hard failures and per-language ratchets.
Track work counters and reuse counters before timing.
D13 defines three target layers:

- An engine floor for all 206 grammars.
- Tuned targets for the top 50.
- Stretch targets for the top 20.

The `2x` full-parse goal and edits at or below C apply to the top-20 stretch layer.
O-Q3 remains open; do not make the proposed v1.0 speed targets release-blocking without the owner's decision.
Hard failures and ratchets remain mandatory. Record every directional regression and explain its tradeoff.

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
- `bash scripts/run_randomized_benchmarks.sh --output /tmp/gotreesitter-bench.txt`

Full-parse non-truncation probe:
- `GOT_PARSE_NODE_LIMIT_SCALE=3` may be used for diagnostic full-parse runs when default node budget truncates benchmark/parity cases.
- `GOT_GLR_MAX_STACKS=...` overrides the default GLR stack cap of 8.

### 8) Prose Standard: Plain language

Write plainly: lead with the point, use common words and the active voice, keep each term consistent, back claims with evidence (numbers, links, test output), and say what you did not verify. M31 agents: see decision 0012 and the `writing-plainly` skill in hypha://m31labs/hyphae.

Use numbers from pinned benchmarks or CI runs as receipts.
