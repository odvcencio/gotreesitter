# Recorded commands

Use baseline `236ca848239679221715b386115fc2910f6cf4e1` in two separate worktrees.
Apply the adjacent candidate.patch to one worktree with git apply.
The baseline contains the earlier shared lexer improvement.
Reconstruct it from `da1150c6` with the patch in the lexer token output report when the local commit is unavailable.
Copy these scripts into a fresh evidence directory and update all absolute paths.
Run each Docker stage once, in the recorded order.
Use Go 1.25.14 and the recorded container limits.

The original failed baseline receipt and full profiles remain in the durable archive:
`/home/draco/work/gts-authenticated-edit-optimization-evidence-20260905/full-parse-arena-reuse`.
The archive excludes compiled test binaries and the C build cache.
Read correctness-summary.md before reproducing the corrected ownership gate.
Create merged-incremental_test.go.txt with `git show d9916dbaa1f3469397bd85ce27286ee98e16074a:incremental_test.go`.
The same overlay maps both worktree files to that exact merged test.

## Correctness selection

```text
^Test(FullParseNodeCapacity.*|GoFullParseRepeatedParserPreservesLiveTree|Ensure.*NodeCapacity.*|AllocNodeUsesOverflowSlabsWhenPrimaryExhausted|Arena.*|NodeRetentionCapRespectsByteLimit|ChildSlabStalePointersAfterReset|ShouldNotRetryMemoryBudgetParse|EvictionGuardPreventsOversizedArenaReuse|OversizedFullArenaReleaseClearsAllDuplicateCheckoutSlots|ParseFullArena.*|NodeRetentionCapRespectsByteLimit|ChildSlabStalePointersAfterReset|ShouldNotRetryMemoryBudgetParse|ParserScratch.*Budget.*|GoFullParseBenchmarkFixturesParseClean|GoFullParseBenchmarkSourceStaysWithinDefaultNodeBudget|ParseIncrementalReleaseKeepsBorrowedNodesAlive|ParseReuseStateRetainsTransitiveBorrowedArenaLifecycle|NodeFieldMetadataArena.*)$
```

## Correctness overlay

```json
{
  "Replace": {
    "/baseline/incremental_test.go": "/evidence/merged-incremental_test.go.txt",
    "/workspace/incremental_test.go": "/evidence/merged-incremental_test.go.txt"
  }
}
```

## run-stage.sh

```bash
#!/usr/bin/env bash
set -euo pipefail
stage=$1
cd /home/draco/work/gts-full-parse-arena-reuse-20260905
bash cgo_harness/docker/run_parity_in_docker.sh --no-build \
 --label "full-parse-arena-reuse-$stage" \
 --out-root /tmp/gts-general-full-parse-20260905 \
 --mount /tmp/gts-general-full-parse-20260905:/evidence \
 --mount /home/draco/work/gts-full-parse-general-baseline-20260905:/baseline:ro \
 --mount /home/draco/work/gotreesitter/.git:/home/draco/work/gotreesitter/.git:ro \
 --mount /home/draco/work/gts-full-parse-arena-reuse-20260905:/home/draco/work/gts-full-parse-arena-reuse-20260905:ro \
 --mount /home/draco/work/gts-full-parse-general-baseline-20260905:/home/draco/work/gts-full-parse-general-baseline-20260905:ro \
 --mount /tmp/gotreesitter-run-randomized-benchmarks.lock:/bench.lock \
 --memory 4g --gomemlimit 3GiB --cpus 1 --cpuset-cpus 2 --pids 512 \
 --wall-timeout 15m -- "bash /evidence/run-$stage.sh"
```

## run-correctness.sh

```bash
#!/usr/bin/env bash
set -euo pipefail
export GOMAXPROCS=1
unset GOT_STATS GTS_ADMISSION_CANDIDATE GOT_GLR_FOREST GOT_GLR_MAX_STACKS GOT_PARSE_NODE_LIMIT_SCALE GOT_GLR_MAX_MERGE_PER_KEY GOT_GLR_FORCE_CONFLICT_WIDTH GOT_C_RECOVERY GOT_FAITHFUL_CONDENSE
for role in baseline candidate; do
  if [[ "$role" == baseline ]]; then cd /baseline; else cd /workspace; fi
  go test -c -overlay /evidence/correctness-overlay.json -tags gts_parsercorephase0 -o "/evidence/$role.test" .
  "/evidence/$role.test" -test.run "$(cat /evidence/correctness-regex.txt)" \
    -test.v -test.count=1 -test.parallel=1 -test.timeout=3m > "/evidence/$role-correctness.txt"
  printf 'completed %s correctness controls\n' "$role"
done
```

## run-go-oracle.sh

```bash
#!/usr/bin/env bash
set -euo pipefail
export GOMAXPROCS=1 GTS_PARITY_C_REF_BUILD_CACHE=/evidence/c-reference-cache
unset GOT_STATS GTS_ADMISSION_CANDIDATE GOT_GLR_FOREST GOT_GLR_MAX_STACKS GOT_PARSE_NODE_LIMIT_SCALE GOT_GLR_MAX_MERGE_PER_KEY GOT_GLR_FORCE_CONFLICT_WIDTH GOT_C_RECOVERY GOT_FAITHFUL_CONDENSE
cd /workspace/cgo_harness
go test . -tags treesitter_c_parity,gts_parsercorephase0 -run '^TestGoTokenInvariantLookaheadNumeric(Repair|Reuse)LockedC$' -v -count=1 -parallel=1 -timeout=3m
```

## run-json-oracle.sh

```bash
#!/usr/bin/env bash
set -euo pipefail
export GOMAXPROCS=1 GTS_PARITY_C_REF_BUILD_CACHE=/evidence/c-reference-cache
unset GOT_STATS GTS_ADMISSION_CANDIDATE GOT_GLR_FOREST GOT_GLR_MAX_STACKS GOT_PARSE_NODE_LIMIT_SCALE GOT_GLR_MAX_MERGE_PER_KEY GOT_GLR_FORCE_CONFLICT_WIDTH GOT_C_RECOVERY GOT_FAITHFUL_CONDENSE
cd /workspace/cgo_harness
go test . -tags treesitter_c_parity,gts_parsercorephase0 -run '^TestParity(FreshParse|IncrementalParse)$/^json$' -v -count=1 -parallel=1 -timeout=3m
```

## run-javascript-oracle.sh

```bash
#!/usr/bin/env bash
set -euo pipefail
export GOMAXPROCS=1 GTS_PARITY_C_REF_BUILD_CACHE=/evidence/c-reference-cache
unset GOT_STATS GTS_ADMISSION_CANDIDATE GOT_GLR_FOREST GOT_GLR_MAX_STACKS GOT_PARSE_NODE_LIMIT_SCALE GOT_GLR_MAX_MERGE_PER_KEY GOT_GLR_FORCE_CONFLICT_WIDTH GOT_C_RECOVERY GOT_FAITHFUL_CONDENSE
cd /workspace/cgo_harness
go test . -tags treesitter_c_parity,gts_parsercorephase0 -run '^TestParity(FreshParse|IncrementalParse)$/^javascript$' -v -count=1 -parallel=1 -timeout=3m
```

## run-paired.sh

```bash
#!/usr/bin/env bash
set -euo pipefail
export GOMAXPROCS=1 GOT_BENCH_FUNC_COUNT=500
export GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=safe.directory GIT_CONFIG_VALUE_0='*'
unset GOT_STATS GTS_ADMISSION_CANDIDATE GOT_GLR_FOREST GOT_GLR_MAX_STACKS GOT_PARSE_NODE_LIMIT_SCALE GOT_GLR_MAX_MERGE_PER_KEY GOT_GLR_FORCE_CONFLICT_WIDTH GOT_C_RECOVERY GOT_FAITHFUL_CONDENSE
cd /workspace
bash scripts/run_randomized_benchmarks.sh --output /evidence/candidate-trio.txt \
 --baseline-root /baseline --baseline-output /evidence/baseline-trio.txt \
 --lock-path /bench.lock --runs 20 --seed-start 1 --benchtime 750ms --tags gts_parsercorephase0 \
 --bench-regex '^BenchmarkGoParse(FullDFA|IncrementalSingleByteEditDFA|IncrementalNoEditDFA)$' \
 --require-benchmarks BenchmarkGoParseFullDFA,BenchmarkGoParseIncrementalSingleByteEditDFA,BenchmarkGoParseIncrementalNoEditDFA
```

## run-macro-paired.sh

```bash
#!/usr/bin/env bash
set -euo pipefail
export GOMAXPROCS=1
unset GOT_BENCH_FUNC_COUNT
export GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=safe.directory GIT_CONFIG_VALUE_0='*'
unset GOT_STATS GTS_ADMISSION_CANDIDATE GOT_GLR_FOREST GOT_GLR_MAX_STACKS GOT_PARSE_NODE_LIMIT_SCALE GOT_GLR_MAX_MERGE_PER_KEY GOT_GLR_FORCE_CONFLICT_WIDTH GOT_C_RECOVERY GOT_FAITHFUL_CONDENSE
cd /workspace
bash scripts/run_randomized_benchmarks.sh --output /evidence/candidate-macro.txt \
 --baseline-root /baseline --baseline-output /evidence/baseline-macro.txt \
 --lock-path /bench.lock --runs 20 --seed-start 1 --benchtime 750ms --tags gts_parsercorephase0 \
 --bench-regex '^BenchmarkGoParseWarmRealDFA$/^grammargen_lr$' \
 --require-benchmarks BenchmarkGoParseWarmRealDFA/grammargen_lr
```

## run-rss.sh

```bash
#!/usr/bin/env bash
set -euo pipefail
export GOMAXPROCS=1
unset GOT_STATS GOT_BENCH_FUNC_COUNT GTS_ADMISSION_CANDIDATE GOT_GLR_FOREST GOT_GLR_MAX_STACKS GOT_PARSE_NODE_LIMIT_SCALE GOT_GLR_MAX_MERGE_PER_KEY GOT_GLR_FORCE_CONFLICT_WIDTH GOT_C_RECOVERY GOT_FAITHFUL_CONDENSE
for role in baseline candidate; do
  if [[ "$role" == baseline ]]; then cd /baseline; else cd /workspace; fi
  go test -c -tags gts_parsercorephase0 -o "/evidence/$role-rss.test" .
done
exec 9>>/bench.lock
flock -n 9
for pair in 1 2 3; do
  if ((pair % 2 == 1)); then roles=(baseline candidate); else roles=(candidate baseline); fi
  for role in "${roles[@]}"; do
    if [[ "$role" == baseline ]]; then cd /baseline; else cd /workspace; fi
    /usr/bin/time -v -o "/evidence/$role-large-rss-$pair.txt" \
      "/evidence/$role-rss.test" -test.run '^$' \
      -test.bench '^BenchmarkGoParseWarmRealDFA$/^grammargen_lr$' \
      -test.benchtime=1x -test.benchmem -test.count=1 -test.shuffle="$pair" \
      -test.parallel=1 -test.timeout=3m > "/evidence/$role-large-rss-$pair.log"
    printf 'completed %s RSS probe %s\n' "$role" "$pair"
  done
done
```
