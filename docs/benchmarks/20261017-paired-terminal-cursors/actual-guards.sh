#!/usr/bin/env bash
set -euo pipefail
test -f /evidence/fleet/complete.txt
test -f /evidence/rss-matched/complete.txt
export GOWORK=off GOMAXPROCS=1 GOFLAGS='-p=1 -buildvcs=false' GIT_OPTIONAL_LOCKS=0
cd /workspace
git config --global --add safe.directory /workspace
python3 - <<'PINS'
from pathlib import Path
import hashlib,json
for n,h in json.loads(Path('/evidence/actual-source-pins.json').read_text()).items():assert hashlib.sha256((Path('/workspace')/n).read_bytes()).hexdigest()==h,n
PINS
go test ./internal/parsercorephase0 -count=1 -timeout=8m > /evidence/actual-core.log 2>&1
go test . -tags gts_parsercorephase0 -run '^Test(DiagnosticParserCoreCanonical|ParserCoreFreshFullRunnerRepeatedCanonicalLifecycle)$' -count=1 -v -timeout=8m > /evidence/actual-canonical.log 2>&1
go run ./cmd/v1layout -base a03d0842994a85aa26cf494c8c67404e1e64e999 > /evidence/actual-layout.log 2>&1
go run ./cmd/v1guard -root /workspace -base a03d0842994a85aa26cf494c8c67404e1e64e999 > /evidence/actual-v1guard.log 2>&1
go run ./cmd/citestplan -stale-run-names -repo-root /workspace > /evidence/actual-selectors.log 2>&1
go run -tags gts_parsercorephase0 ./cmd/perfcounterledger --write --output /evidence/actual-ledger.json > /evidence/actual-ledger.log 2>&1
GTS_LAYOUT_OWNED_PROBE=1 go run -tags gts_parsercorephase0 -overlay /evidence/actual-owned-overlay.json ./cmd/perfcounterledger --langs yaml > /evidence/actual-owned-ledger.json 2> /evidence/actual-owned-ledger.log
python3 - <<'CHECK'
from pathlib import Path
import hashlib,json
p=Path('/evidence')
for kind,n in [('ledger',412),('owned-ledger',2)]:
 a=json.loads((p/f'actual-{kind}.json').read_text());b=json.loads((p/f'head-{kind}.json').read_text());assert a==b and len(a['rows'])==n,kind
 print('actual source',kind,n,'rows exact',flush=True)
for n,h in json.loads((p/'actual-source-pins.json').read_text()).items():assert hashlib.sha256((Path('/workspace')/n).read_bytes()).hexdigest()==h,n
CHECK
printf 'complete\n' > /evidence/actual-guards-complete.txt
echo 'actual paired-cursor source, layout, API, R6, CI selectors and counters passed'
