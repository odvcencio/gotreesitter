#!/usr/bin/env bash
source /evidence/environment.sh
unset GOT_BENCH_FUNC_COUNT
exec 9>>/bench.lock
flock -n 9
for pair in 1 2 3; do
  if ((pair % 2 == 1)); then roles=(release candidate); else roles=(candidate release); fi
  for role in "${roles[@]}"; do
    if [[ "$role" == release ]]; then cd /baseline; else cd /workspace; fi
    /usr/bin/time -v -o "/evidence/$role-large-rss-$pair.txt" \
      "/evidence/$role.test" -test.run '^$' \
      -test.bench '^BenchmarkGoParseWarmRealDFA$/^grammargen_lr$' \
      -test.benchtime=1x -test.benchmem -test.count=1 -test.shuffle="$pair" \
      -test.parallel=1 -test.timeout=3m > "/evidence/$role-large-rss-$pair.log"
    printf 'completed %s RSS probe %s\n' "$role" "$pair"
  done
done
