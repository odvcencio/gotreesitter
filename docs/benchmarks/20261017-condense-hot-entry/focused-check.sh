#!/usr/bin/env bash
set -euo pipefail
export GOWORK=off GOMAXPROCS=1 GOFLAGS='-p=1 -buildvcs=false -overlay=/evidence/overlay.json'
cd /baseline
go test ./internal/parsercorephase0 -run '^Test.*(Condense|Historical|Fold|Shallow|Precedence|Owner|Rollback)' -count=1 -v -timeout=3m > /evidence/baseline-core-fold-tests.log 2>&1
cd /workspace
go test ./internal/parsercorephase0 -count=1 -timeout=8m > /evidence/head-core.log 2>&1
go test ./internal/parsercorephase0 -tags gts_parsercorephase0,gts_parsercorephase0a -count=1 -timeout=8m > /evidence/head-core-phase0a.log 2>&1
go test ./internal/parsercorephase0 -run '^Test(CanonicalHeadMask|LineageRecoveryCostReadAfterPlanCallback)$' -count=1 -v -timeout=3m > /evidence/head-callback-fixed.log 2>&1
printf 'complete\n' > /evidence/unit-complete.txt
go test . -tags gts_parsercorephase0 -run '^Test(DiagnosticParserCoreCanonical|DiagnosticParserCoreGenericScheduler|DiagnosticParserCorePerVersionLexer|DiagnosticParserCoreOwnedLexer|DiagnosticParserCoreSeed|DiagnosticParserCoreGeneric|ParserCoreLanguageTables|ParserCoreFreshFullRunnerRepeatedCanonicalLifecycle|CompactYAML|CompactReuseDependency|CompactLegacyRead)' -count=1 -v -timeout=8m > /evidence/head-root-focus.log 2>&1
for lang in go yaml; do
 GTS_ADMISSION_CANDIDATE=1 GOTREESITTER_V1_INVARIANT_LANGUAGE=$lang go test . -tags gts_parsercorephase0 -run '^TestV1InvariantGateR4EditSession$' -count=1 -v -timeout=5m > /evidence/head-$lang-invariant.log 2>&1
 cd /workspace/cgo_harness
 GTS_ADMISSION_CANDIDATE=1 GTS_PARITY_MODE=exhaustive go test . -tags gts_parsercorephase0,treesitter_c_parity -run "^(TestParityFreshParse|TestParityIncrementalParse|TestParityHasNoErrors)$/^${lang}$" -count=1 -v -timeout=5m > /evidence/head-$lang-c.log 2>&1
 cd /workspace
 echo "$lang invariant and locked C passed"
done
for role in base head; do
 if [[ $role == base ]]; then cd /baseline; else cd /workspace; fi
 go run -tags gts_parsercorephase0 ./cmd/perfcounterledger --write --output /evidence/$role-ledger.json > /evidence/$role-ledger.log 2>&1
 GTS_LAYOUT_OWNED_PROBE=1 go run -tags gts_parsercorephase0 ./cmd/perfcounterledger --langs yaml > /evidence/$role-owned-ledger.json 2> /evidence/$role-owned-ledger.log
done
python3 - <<'CHECK'
from pathlib import Path
import json
p=Path('/evidence')
for kind,n in [('ledger',412),('owned-ledger',2)]:
 a=json.loads((p/f'base-{kind}.json').read_text());h=json.loads((p/f'head-{kind}.json').read_text())
 assert a==h and len(h['rows'])==n,(kind,'counter difference')
 print(kind,n,'rows exact',flush=True)
CHECK
cd /workspace
GTS_LAYOUT_OWNED_PROBE=1 GTS_ADMISSION_CANDIDATE=1 GOTREESITTER_V1_INVARIANT_LANGUAGE=yaml go test . -tags gts_parsercorephase0 -run '^TestV1InvariantGateR4EditSession$' -count=1 -v -timeout=5m > /evidence/head-owned-yaml-invariant.log 2>&1
cd /workspace/cgo_harness
go test . -tags gts_parsercorephase0,treesitter_c_parity -run '^(TestCompactYAMLDocumentStreamLockedC|TestCompactYAMLVersionOwnershipCOracle|TestCompactRecoveryYAMLRecoverEOFIncrementalCOracle|TestNegativeRelexCanonicalGoLockedC)$' -count=1 -v -timeout=8m > /evidence/head-extra-c.log 2>&1
cd /workspace
go test ./internal/parsercorephase0 -race -count=1 -timeout=8m > /evidence/head-core-race.log 2>&1
GOARCH=386 CGO_ENABLED=0 go test ./internal/parsercorephase0 -count=1 -timeout=8m > /evidence/head-core-386.log 2>&1
for role in base head; do
 if [[ $role == base ]]; then cd /baseline; else cd /workspace; fi
 go test -c ./internal/parsercorephase0 -tags gts_parsercorephase0 -o /evidence/frame-$role.test
 go tool objdump -s 'condenseWithOutcomeAtomic$' /evidence/frame-$role.test > /evidence/frame-$role-condense.asm
done
printf 'complete\n' > /evidence/check-complete.txt
echo 'condense hot-entry correctness and counters passed'
