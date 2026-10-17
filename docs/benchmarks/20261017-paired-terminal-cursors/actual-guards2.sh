#!/usr/bin/env bash
set -euo pipefail
test -f /evidence/fleet/complete.txt
test -f /evidence/rss-matched/complete.txt
export GOWORK=off GOMAXPROCS=1 GOFLAGS='-p=1 -buildvcs=false' GIT_OPTIONAL_LOCKS=0
cd /workspace
git config --global --add safe.directory /workspace
git config --global --add safe.directory /evidence/tracked-source
python3 - <<'BEFORE'
from pathlib import Path
import hashlib,json
p=Path('/evidence');pins=json.loads((p/'actual-source-pins.json').read_text())
for n,h in pins.items():
 for root in ['/workspace','/evidence/tracked-source']:assert hashlib.sha256((Path(root)/n).read_bytes()).hexdigest()==h,n
assert '\nok  ' in (p/'actual-core.log').read_text() or (p/'actual-core.log').read_text().startswith('ok  ')
assert '\nPASS\n' in (p/'actual-canonical.log').read_text()
assert '756/756 root Go files, 12 API tag sets' in (p/'actual-layout.log').read_text()
BEFORE
GIT_DIR=/workspace/.git GIT_WORK_TREE=/evidence/tracked-source go run ./cmd/v1guard -root /evidence/tracked-source -base a03d0842994a85aa26cf494c8c67404e1e64e999 > /evidence/actual-v1guard-tracked.log 2>&1
go run ./cmd/citestplan -stale-run-names -repo-root /evidence/tracked-source > /evidence/actual-selectors.log 2>&1
go run -tags gts_parsercorephase0 ./cmd/perfcounterledger --write --output /evidence/actual-ledger.json > /evidence/actual-ledger.log 2>&1
GTS_LAYOUT_OWNED_PROBE=1 go run -tags gts_parsercorephase0 -overlay /evidence/actual-owned-overlay.json ./cmd/perfcounterledger --langs yaml > /evidence/actual-owned-ledger.json 2> /evidence/actual-owned-ledger.log
python3 - <<'AFTER'
from pathlib import Path
import hashlib,json
p=Path('/evidence')
for kind,n in [('ledger',412),('owned-ledger',2)]:
 a=json.loads((p/f'actual-{kind}.json').read_text());b=json.loads((p/f'head-{kind}.json').read_text());assert a==b and len(a['rows'])==n,kind
 print('actual source',kind,n,'rows exact',flush=True)
for n,h in json.loads((p/'actual-source-pins.json').read_text()).items():assert hashlib.sha256((Path('/workspace')/n).read_bytes()).hexdigest()==h,n
AFTER
printf 'complete\n' > /evidence/actual-guards-complete.txt
echo 'actual paired-cursor tracked source, layout, API, R6, CI selectors and counters passed'
