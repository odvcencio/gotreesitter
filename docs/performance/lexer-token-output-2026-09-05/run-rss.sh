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
