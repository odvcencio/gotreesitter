#!/usr/bin/env bash
set -euo pipefail
export GOMAXPROCS=1
unset GOT_STATS GTS_ADMISSION_CANDIDATE GOT_GLR_FOREST GOT_GLR_MAX_STACKS GOT_PARSE_NODE_LIMIT_SCALE GOT_GLR_MAX_MERGE_PER_KEY GOT_GLR_FORCE_CONFLICT_WIDTH GOT_C_RECOVERY GOT_FAITHFUL_CONDENSE
cd /workspace
/usr/bin/time -v -o /evidence/max-rss.txt go test -overlay /evidence/overlay.json ./grammars -run '^TestIssue454CIncrementalDeleteMatchesFresh$' -v -count=1 -parallel=1 -timeout=4m > /evidence/fixture.txt 2>&1
