#!/usr/bin/env bash
set -euo pipefail
test -f /evidence/perf-complete.txt
export GOWORK=off GOMAXPROCS=1 GOFLAGS='-p=1 -buildvcs=false -overlay=/evidence/overlay.json'
cd /baseline
export GTS_ADMISSION_REAL_CORPUS_LANGS=go
go test . -tags gts_parsercorephase0 -run '^$' -bench '^BenchmarkParserCoreFreshFullCanonical/language$' -benchtime=10s -count=1 -benchmem -o /evidence/profile-go.test -cpuprofile=/evidence/profile-go.cpu > /evidence/profile-go.log 2>&1
go tool pprof -top -nodecount=45 -cum /evidence/profile-go.test /evidence/profile-go.cpu > /evidence/profile-go-cumulative.txt
go tool pprof -top -nodecount=45 /evidence/profile-go.test /evidence/profile-go.cpu > /evidence/profile-go-flat.txt
export GTS_ADMISSION_REAL_CORPUS_LANGS=yaml
go test . -tags gts_parsercorephase0 -run '^$' -bench '^BenchmarkAdmissionYAMLDocumentStreamOwned$' -benchtime=10s -count=1 -benchmem -o /evidence/profile-yaml.test -cpuprofile=/evidence/profile-yaml.cpu > /evidence/profile-yaml.log 2>&1
go tool pprof -top -nodecount=45 -cum /evidence/profile-yaml.test /evidence/profile-yaml.cpu > /evidence/profile-yaml-cumulative.txt
go tool pprof -top -nodecount=45 /evidence/profile-yaml.test /evidence/profile-yaml.cpu > /evidence/profile-yaml-flat.txt
printf 'complete\n' > /evidence/profile-complete.txt
