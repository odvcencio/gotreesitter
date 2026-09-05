#!/usr/bin/env bash
source /evidence/environment.sh
for role in release candidate; do
  if [[ "$role" == release ]]; then cd /baseline; else cd /workspace; fi
  GOT_STATS=1 "/evidence/$role.test" -test.run '^$' \
    -test.bench '^BenchmarkGoParseIncrementalSingleByteEditDFA$' \
    -test.benchtime=5x -test.benchmem -test.count=1 -test.shuffle=1 \
    -test.parallel=1 -test.timeout=3m > "/evidence/$role-uninspected-telemetry.txt"
  printf 'completed %s uninspected benchmark telemetry\n' "$role"
done
