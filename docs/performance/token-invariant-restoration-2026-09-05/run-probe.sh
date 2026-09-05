#!/usr/bin/env bash
source /evidence/environment.sh
for role in release candidate; do
  if [[ "$role" == release ]]; then cd /baseline; expected=0; else cd /workspace; expected=1; fi
  go test -c -overlay /evidence/probe/overlay.docker.json -tags gts_parsercorephase0 -o "/evidence/probe/$role-probe.test" .
  GTS_EXPECT_TOKEN_INVARIANT_REUSE="$expected" \
  GTS_TOKEN_INVARIANT_PREFLIGHT_OUTPUT="/evidence/probe/$role-preflight.json" \
    "/evidence/probe/$role-probe.test" -test.run '^TestTokenInvariantExactBenchmarkPreflight$' \
      -test.v -test.count=1 -test.parallel=1 -test.timeout=3m > "/evidence/probe/$role-preflight.log"
  printf 'completed %s exact workload preflight\n' "$role"
done
