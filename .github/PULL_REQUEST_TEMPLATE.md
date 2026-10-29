## What does this PR do?

<!-- One or two sentences. State the outcome, not a commit diary. -->

## Why this approach?

<!-- Brief rationale. Mention the main tradeoff or rejected alternative only if it matters. -->

## Correctness

- [ ] Focused package or unit tests ran for the touched behavior
- [ ] Affected parity validation ran in Docker, one language or grammar at a time
- [ ] If grammargen or parity logic changed, ran the smallest relevant focus target in Docker
- [ ] Documented anything intentionally left unvalidated in this PR
- [ ] Engine PR: before-and-after deterministic counters are in the PR body (work counters did not rise, reuse counters did not fall)
- [ ] Invariant gate ran for each language the PR can affect (incremental equals fresh, ERROR root implies HasError, root covers input, no-edit reparse allocates nothing)
- [ ] No language-name comparison in engine files (D6); one v1 lane only

<!-- Keep correctness gating separate from performance gating. -->
<!-- Do not use host-side repo-wide sweeps. -->
<!-- Examples:
- bash cgo_harness/docker/run_parity_in_docker.sh -- "cd /workspace && go test ./grammargen -run '^TestName$' -count=1"
- bash cgo_harness/docker/run_single_grammar_parity.sh <grammar>
- bash cgo_harness/docker/run_grammargen_focus_targets.sh --mode real-corpus --langs <language>
- bash cgo_harness/docker/run_grammargen_focus_targets.sh --mode cgo --langs <language>
-->

## Performance

- [ ] If perf-relevant, compared before and after with `scripts/run_randomized_benchmarks.sh` (`GOMAXPROCS=1`, `-count=1` per process, 20 shuffle seeds, `-benchtime=750ms`, `-benchmem`); a fixed-order run is not evidence
- [ ] Included before and after numbers, or explicitly said perf is not the goal of this PR
- [ ] If large-grammar behavior changed, checked bounded RSS, timeout, or OOM behavior under Docker

<!-- Benchmark after correctness passes. Prefer the primary bench trio in AGENTS.md. -->

## Maintainability

- [ ] No unnecessary abstractions or speculative cleanup outside the PR's scope
- [ ] Generated files or harness changes are only here because they are required by the change
- [ ] No new dependencies without justification

## Self-review

Review your own diff before requesting review. Walk through it as if you're seeing it for the first time, and keep any self-review comments concise and focused on the non-obvious parts.

- [ ] Reviewed my own diff end to end
- [ ] Left comments on notable sections or the crux of the solution
- [ ] Called out anything I'm unsure about or want a second opinion on

## Due diligence

- [ ] Read the code you're changing before changing it
- [ ] Checked that similar patterns exist elsewhere in the codebase and stayed consistent
- [ ] If touching the parser or lexer: validated against diverse affected grammars
- [ ] If adding a grammar or scanner: verified against upstream C output
