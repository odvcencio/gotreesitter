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
