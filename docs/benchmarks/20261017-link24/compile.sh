#!/usr/bin/env bash
set -euo pipefail
export GOWORK=off GOMAXPROCS=1 GOFLAGS='-p=1 -buildvcs=false -overlay=/evidence/overlay.json'
cd /workspace
go build -gcflags='github.com/odvcencio/gotreesitter/internal/parsercorephase0=-m=2' ./internal/parsercorephase0 > /evidence/compiler-stdout.log 2> /evidence/compiler.log
go test ./internal/parsercorephase0 -run '^Test(PackedLinkRecord|PackedGraphLinks)' -count=1 -timeout=5m > /evidence/compile.log 2>&1
