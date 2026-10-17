#!/usr/bin/env bash
set -euo pipefail
export GOWORK=off GOMAXPROCS=1 GOFLAGS='-p=1 -buildvcs=false -overlay=/evidence/overlay.json'
for role in base head; do
 if [[ $role == base ]];then cd /baseline;else cd /workspace;fi
 go test . -tags gts_parsercorephase0,gts_workcount -run '^TestDeferredPublicationWorkCensus$' -count=1 -v -timeout=5m > /evidence/$role-publication-work.log 2>&1
 echo "$role parse and consume census passed"
done
printf 'complete\n' > /evidence/work-check-complete.txt
