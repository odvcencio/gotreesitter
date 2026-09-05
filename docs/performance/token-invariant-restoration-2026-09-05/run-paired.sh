#!/usr/bin/env bash
source /evidence/environment.sh
cd /workspace
bash scripts/run_randomized_benchmarks.sh \
 --output /evidence/candidate-trio.txt \
 --baseline-root /baseline --baseline-output /evidence/release-trio.txt \
 --lock-path /bench.lock --runs 20 --seed-start 1 --benchtime 750ms \
 --tags gts_parsercorephase0 --package . \
 --bench-regex '^BenchmarkGoParse(FullDFA|IncrementalSingleByteEditDFA|IncrementalNoEditDFA)$' \
 --require-benchmarks BenchmarkGoParseFullDFA,BenchmarkGoParseIncrementalSingleByteEditDFA,BenchmarkGoParseIncrementalNoEditDFA
