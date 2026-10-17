#!/usr/bin/env bash
set -euo pipefail
export GOWORK=off GOMAXPROCS=1 GOFLAGS='-p=1 -buildvcs=false -overlay=/evidence/unit-overlay.json'
cd /workspace
go test ./internal/parsercorephase0 -run '^TestPackedLinkRecord' -count=1 -v -timeout=5m > /evidence/codec-unit.log 2>&1
go test ./internal/parsercorephase0 -race -run '^TestPackedLinkRecord' -count=1 -v -timeout=5m > /evidence/codec-race.log 2>&1
GOARCH=386 CGO_ENABLED=0 go test ./internal/parsercorephase0 -run '^TestPackedLinkRecord' -count=1 -v -timeout=5m > /evidence/codec-386.log 2>&1
go test ./internal/parsercorephase0 -count=1 -timeout=8m > /evidence/core-all.log 2>&1
export GOFLAGS='-p=1 -buildvcs=false -overlay=/evidence/overlay.json'
bash /evidence/run.sh
printf 'complete\n' > /evidence/codec-proof-complete.txt
