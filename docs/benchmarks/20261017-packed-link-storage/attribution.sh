#!/usr/bin/env bash
set -euo pipefail
test -f /evidence/perf-complete.txt
export GOWORK=off GOMAXPROCS=1 GOFLAGS='-p=1 -buildvcs=false -overlay=/evidence/overlay.json'
cd /workspace
go build -gcflags='github.com/odvcencio/gotreesitter/internal/parsercorephase0=-m=2' ./internal/parsercorephase0 > /evidence/compiler-stdout.log 2> /evidence/compiler.log
for role in base head;do
 if [[ $role == base ]];then cd /baseline;else cd /workspace;fi
 GTS_ADMISSION_REAL_CORPUS_LANGS=go go test . -tags gts_parsercorephase0 -run '^$' -bench '^BenchmarkAdmissionR4Compact$' -benchtime=3s -count=1 -cpuprofile /evidence/$role.cpu.pprof -timeout=5m > /evidence/$role-profile.log 2>&1
 go tool pprof -top -nodecount=35 /evidence/$role.cpu.pprof > /evidence/$role-pprof-top.txt 2>&1
 go tool pprof -top -cum -nodecount=35 /evidence/$role.cpu.pprof > /evidence/$role-pprof-cum.txt 2>&1
 echo "$role CPU attribution captured (not comparison timing evidence)"
done
printf 'complete\n' > /evidence/attribution-complete.txt
