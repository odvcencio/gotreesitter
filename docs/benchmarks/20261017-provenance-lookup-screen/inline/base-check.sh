#!/usr/bin/env bash
set -euo pipefail
export GOWORK=off GOMAXPROCS=1 GOFLAGS='-p=1 -buildvcs=false -overlay=/evidence/overlay.json'
cd /baseline
go test ./internal/parsercorephase0 -count=1 -timeout=8m > /evidence/base-core.log 2>&1
go test . -tags gts_parsercorephase0 -run '^Test(DiagnosticParserCoreCanonical|DiagnosticParserCoreOwnedLexer|DiagnosticParserCoreSeed|DiagnosticParserCoreGeneric|ParserCoreLanguageTables|ParserCoreFreshFullRunnerRepeatedCanonicalLifecycle|CompactYAML)' -count=1 -v -timeout=8m > /evidence/base-root.log 2>&1
printf 'complete\n' > /evidence/base-complete.txt
bash /evidence/check.sh
