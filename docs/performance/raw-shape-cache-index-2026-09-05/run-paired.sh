#!/usr/bin/env bash
set -euo pipefail
export GOMAXPROCS=1 GOT_BENCH_FUNC_COUNT=500
export GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=safe.directory GIT_CONFIG_VALUE_0='*'
unset GOT_STATS GTS_ADMISSION_CANDIDATE GOT_GLR_FOREST GOT_GLR_MAX_STACKS GOT_PARSE_NODE_LIMIT_SCALE GOT_GLR_MAX_MERGE_PER_KEY GOT_GLR_FORCE_CONFLICT_WIDTH GOT_C_RECOVERY GOT_FAITHFUL_CONDENSE GOT_PARSE_ACTION_TIMING GOT_PARSE_REDUCE_TIMING GTS_C_OLD_LEGACY
cd /workspace
bash scripts/run_randomized_benchmarks.sh --output /evidence/candidate-bench.txt --baseline-root /baseline --baseline-output /evidence/baseline-bench.txt --lock-path /bench.lock --runs 20 --seed-start 1 --benchtime 750ms --tags gts_parsercorephase0 --bench-regex '^(BenchmarkGoParse(FullDFA|IncrementalSingleByteEditDFA|IncrementalNoEditDFA)|BenchmarkCIncrementalRecoveryGrowth)$' --require-benchmarks BenchmarkGoParseFullDFA,BenchmarkGoParseIncrementalSingleByteEditDFA,BenchmarkGoParseIncrementalNoEditDFA,BenchmarkCIncrementalRecoveryGrowth/functions16,BenchmarkCIncrementalRecoveryGrowth/functions256
