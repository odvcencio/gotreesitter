#!/usr/bin/env bash
set -euo pipefail
export GOMAXPROCS=1 GOT_BENCH_FUNC_COUNT=500
unset GOT_STATS GTS_ADMISSION_CANDIDATE GOT_GLR_FOREST GOT_GLR_MAX_STACKS GOT_PARSE_NODE_LIMIT_SCALE GOT_GLR_MAX_MERGE_PER_KEY GOT_GLR_FORCE_CONFLICT_WIDTH GOT_C_RECOVERY GOT_FAITHFUL_CONDENSE
cd /workspace
go test -c -overlay /evidence/probe/overlay.docker.json -tags gts_parsercorephase0 -o /evidence/probe/candidate-preflight.test .
GTS_EXPECT_TOKEN_INVARIANT_REUSE=1 GTS_TOKEN_INVARIANT_PREFLIGHT_OUTPUT=/evidence/probe/candidate-preflight.json \
 /evidence/probe/candidate-preflight.test -test.run '^TestTokenInvariantExactBenchmarkPreflight$' -test.v -test.count=1 -test.parallel=1 -test.timeout=3m > /evidence/probe/candidate-preflight.log
