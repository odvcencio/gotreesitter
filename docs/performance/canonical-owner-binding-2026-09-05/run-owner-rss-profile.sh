#!/usr/bin/env bash
set -euo pipefail
export GOMAXPROCS=1
export GTS_PARITY_C_REF_BUILD_CACHE=/evidence/canonical-c-cache
cd /baseline/cgo_harness
go test . -tags 'treesitter_c_parity,gts_parsercorephase0' -c -o /evidence/owner-canonical-base.test
cd /workspace/cgo_harness
go test . -tags 'treesitter_c_parity,gts_parsercorephase0' -c -o /evidence/owner-canonical-head.test
for role in baseline candidate; do
  if [[ "$role" == baseline ]]; then
    cd /baseline/cgo_harness
    binary=/evidence/owner-canonical-base.test
  else
    cd /workspace/cgo_harness
    binary=/evidence/owner-canonical-head.test
  fi
  /usr/bin/time -v -o "/evidence/owner-${role}-large-rss.txt" \
    "$binary" -test.run '^$' \
    -test.bench '^BenchmarkParityGoCanonicalFull$/^grammargen_lr$/^gotreesitter$' \
    -test.benchtime=1x -test.benchmem -test.count=1 \
    > "/evidence/owner-${role}-large-probe.txt"
done
cd /workspace
go test . -tags gts_parsercorephase0 -run '^$' \
  -bench '^BenchmarkAdmissionCandidateGoQueryCompileWarmRoute$' \
  -benchtime=10s -count=1 -benchmem \
  -cpuprofile=/evidence/owner-residual-query.cpu \
  -memprofile=/evidence/owner-residual-query.mem \
  -o /evidence/owner-residual-query.test
go tool pprof -top -nodecount=40 /evidence/owner-residual-query.test /evidence/owner-residual-query.cpu
go tool pprof -top -alloc_space -nodecount=25 /evidence/owner-residual-query.test /evidence/owner-residual-query.mem
