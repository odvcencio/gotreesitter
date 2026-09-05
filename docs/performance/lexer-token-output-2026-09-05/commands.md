# Recorded commands

Create two worktrees at measured baseline `da1150c6d6a2d581ce31f44ba4c5b8241ec431ae`.
Apply the adjacent `candidate.patch` to the candidate worktree with `git apply`.
Keep the baseline worktree unchanged.
The patch reconstructs the measured candidate without fetching local commit `236ca848`.
The integration branch contains byte-identical versions of all three changed Go files.
Copy these scripts into a fresh evidence directory.
Update the absolute worktree and evidence paths before execution.
Run one Docker stage at a time.
Keep the shared benchmark lock.
The image contains Go 1.25.14 for linux/amd64.
The scripts record all measured settings.

The preflight source and full disassembly remain in the durable archive.
Read ARCHIVE.md for their locations.

## run-stage.sh

```bash
#!/usr/bin/env bash
set -euo pipefail
stage=$1
cd /home/draco/work/gts-lexer-token-output-20260905
bash cgo_harness/docker/run_parity_in_docker.sh --no-build \
 --label "lexer-token-output-$stage" \
 --out-root /tmp/gts-lexer-token-output-20260905 \
 --mount /tmp/gts-lexer-token-output-20260905:/evidence \
 --mount /home/draco/work/gts-token-invariant-perf-candidate-20260905:/baseline:ro \
 --mount /home/draco/work/gotreesitter/.git:/home/draco/work/gotreesitter/.git:ro \
 --mount /home/draco/work/gts-lexer-token-output-20260905:/home/draco/work/gts-lexer-token-output-20260905:ro \
 --mount /home/draco/work/gts-token-invariant-perf-candidate-20260905:/home/draco/work/gts-token-invariant-perf-candidate-20260905:ro \
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
  go test -c -tags gts_parsercorephase0 -o "/evidence/$role.test" .
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
exec 9>>/bench.lock
flock -n 9
for pair in 1 2 3; do
  if ((pair % 2 == 1)); then roles=(baseline candidate); else roles=(candidate baseline); fi
  for role in "${roles[@]}"; do
    if [[ "$role" == baseline ]]; then cd /baseline; else cd /workspace; fi
    /usr/bin/time -v -o "/evidence/$role-large-rss-$pair.txt" \
      "/evidence/$role.test" -test.run '^$' \
      -test.bench '^BenchmarkGoParseWarmRealDFA$/^grammargen_lr$' \
      -test.benchtime=1x -test.benchmem -test.count=1 -test.shuffle="$pair" \
      -test.parallel=1 -test.timeout=3m > "/evidence/$role-large-rss-$pair.log"
    printf 'completed %s RSS probe %s\n' "$role" "$pair"
  done
done
```
