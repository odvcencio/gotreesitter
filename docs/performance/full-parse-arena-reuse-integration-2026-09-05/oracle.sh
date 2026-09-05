#!/usr/bin/env bash
set -euo pipefail
export GOMAXPROCS=1 GTS_PARITY_C_REF_BUILD_CACHE=/evidence/c-reference-cache
unset GOT_STATS GTS_ADMISSION_CANDIDATE GOT_GLR_FOREST GOT_GLR_MAX_STACKS GOT_PARSE_NODE_LIMIT_SCALE GOT_GLR_MAX_MERGE_PER_KEY GOT_GLR_FORCE_CONFLICT_WIDTH GOT_C_RECOVERY GOT_FAITHFUL_CONDENSE
cd /workspace/cgo_harness
case "$1" in
 go) selection='^TestGoTokenInvariantLookaheadNumeric(Repair|Reuse)LockedC$';;
 json) selection='^TestParity(FreshParse|IncrementalParse)$/^json$';;
 javascript) selection='^TestParity(FreshParse|IncrementalParse)$/^javascript$';;
esac
go test . -tags treesitter_c_parity,gts_parsercorephase0 -run "$selection" -v -count=1 -parallel=1 -timeout=3m > "/evidence/$1-oracle.txt" 2>&1
