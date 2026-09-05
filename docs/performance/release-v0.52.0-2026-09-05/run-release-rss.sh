#!/usr/bin/env bash
set -euo pipefail
export GOMAXPROCS=1
export GTS_PARITY_C_REF_BUILD_CACHE=/evidence/canonical-c-cache
cd /workspace/cgo_harness
go test . -tags 'treesitter_c_parity,gts_parsercorephase0' -c -o /evidence/release-canonical.test
/usr/bin/time -v -o /evidence/release-large-rss.txt \
  /evidence/release-canonical.test -test.run '^$' \
  -test.bench '^BenchmarkParityGoCanonicalFull$/^grammargen_lr$/^gotreesitter$' \
  -test.benchtime=1x -test.benchmem -test.count=1
