#!/usr/bin/env bash
set -euo pipefail
cd /workspace
unset GTS_ADMISSION_CANDIDATE
bash /paired-driver.sh \
  --baseline-root /baseline \
  --baseline-output /evidence/release-base.txt \
  --output /evidence/release-head.txt --lock-path /bench.lock \
  --runs 20 --seed-start 1 --benchtime 750ms \
  --tags gts_parsercorephase0 --package . \
  --bench-regex '^(BenchmarkGoParseFullDFA|BenchmarkGoParseIncrementalSingleByteEditDFA|BenchmarkGoParseIncrementalNoEditDFA)$' \
  --require-benchmarks 'BenchmarkGoParseFullDFA,BenchmarkGoParseIncrementalSingleByteEditDFA,BenchmarkGoParseIncrementalNoEditDFA'
