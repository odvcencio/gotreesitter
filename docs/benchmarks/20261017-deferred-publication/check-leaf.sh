#!/usr/bin/env bash
set -euo pipefail
export GOWORK=off GOMAXPROCS=1 GOFLAGS='-p=1 -buildvcs=false -overlay=/evidence/overlay.json'
cd /workspace
go test . -tags gts_parsercorephase0 -run '^Test(CompactDeferredPublication|CompactLegacy|CompactReuseDependency|CompactAcceptanceComparisonWorkspace|DiagnosticParserCoreGeneric|ParserCoreFreshFullRunnerRepeatedCanonicalLifecycle|CompactFullLeafSizeBudget|NoTreeNodeSizeBudget)' -count=1 -v -timeout=5m > /evidence/leaf-root-integration.log 2>&1
for lang in go yaml; do
 GTS_ADMISSION_CANDIDATE=1 GOTREESITTER_V1_INVARIANT_LANGUAGE=$lang go test . -tags gts_parsercorephase0 -run '^TestV1InvariantGateR4EditSession$' -count=1 -v -timeout=5m > /evidence/leaf-$lang-invariant.log 2>&1
 cd /workspace/cgo_harness
 GTS_ADMISSION_CANDIDATE=1 GTS_PARITY_MODE=exhaustive go test . -tags gts_parsercorephase0,treesitter_c_parity -run "^(TestParityFreshParse|TestParityIncrementalParse|TestParityHasNoErrors)$/^${lang}$" -count=1 -v -timeout=5m > /evidence/leaf-$lang-c.log 2>&1
 cd /workspace
 echo "$lang invariant and locked C passed"
done
go run -tags gts_parsercorephase0 ./cmd/perfcounterledger --write --output /evidence/leaf-ledger.json > /evidence/leaf-ledger.log 2>&1
GTS_LAYOUT_OWNED_PROBE=1 go run -tags gts_parsercorephase0 ./cmd/perfcounterledger --langs yaml > /evidence/leaf-owned-ledger.json 2> /evidence/leaf-owned-ledger.log
printf 'complete\n' > /evidence/leaf-check-complete.txt
