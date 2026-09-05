#!/usr/bin/env bash
source /evidence/environment.sh
for role in release candidate; do
  if [[ "$role" == release ]]; then cd /baseline; else cd /workspace; fi
  go test -c -tags gts_parsercorephase0 -o "/evidence/$role.test" .
  "/evidence/$role.test" -test.run '^(TestGoFullParseBenchmarkFixturesParseClean|TestReleaseTokenInvariantFallbackLongestMatch)$' -test.v -test.count=1 -test.parallel=1 -test.timeout=3m > "/evidence/$role-controls.txt"
  if [[ "$role" == candidate ]]; then
    "/evidence/$role.test" -test.run '^TestTokenInvariantLookaheadGo(Control|FloatControl)$' -test.v -test.count=1 -test.parallel=1 -test.timeout=3m > /evidence/candidate-reuse-controls.txt
  fi
  go version > "/evidence/$role-go-version.txt"
  go version -m "/evidence/$role.test" > "/evidence/$role-binary-build.txt"
  printf 'completed %s controls\n' "$role"
done
