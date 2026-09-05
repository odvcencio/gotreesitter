#!/usr/bin/env bash
set -euo pipefail
cd /workspace
bash /paired-driver.sh \
  --baseline-root /baseline \
  --baseline-output /evidence/owner-trio-base.txt \
  --output /evidence/owner-trio-head.txt \
  --lock-path /bench.lock --runs 20 --seed-start 1 --benchtime 750ms \
  --tags gts_parsercorephase0 --package . \
  --bench-regex '^(BenchmarkGoParseFullDFA|BenchmarkGoParseIncrementalSingleByteEditDFA|BenchmarkGoParseIncrementalNoEditDFA|BenchmarkAdmissionCandidateGoQueryCompileWarmRoute)$' \
  --require-benchmarks 'BenchmarkGoParseFullDFA,BenchmarkGoParseIncrementalSingleByteEditDFA,BenchmarkGoParseIncrementalNoEditDFA,BenchmarkAdmissionCandidateGoQueryCompileWarmRoute'
