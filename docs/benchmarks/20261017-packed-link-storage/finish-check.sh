#!/usr/bin/env bash
set -euo pipefail
export GOWORK=off GOMAXPROCS=1 GOFLAGS='-p=1 -buildvcs=false -overlay=/evidence/overlay.json'
cd /workspace
go test ./internal/parsercorephase0 -count=1 -timeout=8m > /evidence/core-all.log 2>&1
go test ./internal/parsercorephase0 -tags gts_parsercorephase0,gts_parsercorephase0a -count=1 -timeout=8m > /evidence/core-phase0a.log 2>&1
go test ./internal/parsercorephase0 -race -run '^Test(PackedLinkRecord|PackedGraphLinks|SchedulerTransactionCommitAndRollback|SoleDerivation)' -count=1 -timeout=5m > /evidence/core-race.log 2>&1
GOARCH=386 CGO_ENABLED=0 go test ./internal/parsercorephase0 -run '^Test(PackedLinkRecord|PackedGraphLinks|SchedulerTransactionCommitAndRollback|SoleDerivation)' -count=1 -timeout=5m > /evidence/core-386.log 2>&1
go test . -tags gts_parsercorephase0 -race -run '^Test(CompactAcceptanceComparisonWorkspace|ParserCoreFreshFullRunnerRepeatedCanonicalLifecycle)' -count=1 -timeout=5m > /evidence/root-race.log 2>&1
GTS_LAYOUT_OWNED_PROBE=1 GTS_ADMISSION_CANDIDATE=1 GOTREESITTER_V1_INVARIANT_LANGUAGE=yaml go test . -tags gts_parsercorephase0 -run '^TestV1InvariantGateR4EditSession$' -count=1 -v -timeout=5m > /evidence/owned-yaml-invariant.log 2>&1
printf 'complete\n' > /evidence/check-complete.txt
