#!/usr/bin/env bash
set -euo pipefail
test -f /evidence/check-complete.txt
export GOWORK=off GOMAXPROCS=1 GOFLAGS='-p=1 -buildvcs=false -overlay=/evidence/overlay.json'
cd /workspace
go test ./internal/parsercorephase0 -run '^TestPackedGraphLinksProductionGeneratedRoundTrips$' -count=1 -v -timeout=5m > /evidence/production-property.log 2>&1
go test ./internal/parsercorephase0 -tags gts_eof_recovery_shadow -run '^TestDiagnosticEOFRecovery' -count=1 -timeout=5m > /evidence/core-shadow.log 2>&1
python3 /evidence/check-counters.py
for role in base head;do
 if [[ $role == base ]];then cd /baseline;else cd /workspace;fi
 go test . -tags gts_parsercorephase0 -run '^$' -bench '^BenchmarkCompactGoFullConsume$' -benchtime=1x -count=1 -benchmem -timeout=5m > /evidence/$role-consume-preflight.log 2>&1
 echo "$role consumption preflight passed (untimed qualification only)"
done
printf 'complete\n' > /evidence/qual-complete.txt
git config --global --add safe.directory /workspace
git config --global --add safe.directory /baseline
export GIT_OPTIONAL_LOCKS=0
cd /workspace
export GTS_ADMISSION_REAL_CORPUS_LANGS=go
bash scripts/run_randomized_benchmarks.sh --output /evidence/go-head.txt --baseline-root /baseline --baseline-output /evidence/go-base.txt --bench-regex '^Benchmark(ParserCoreFreshFullCanonical|GoParseFullDFA|GoParseIncrementalSingleByteEditDFA|GoParseIncrementalNoEditDFA|AdmissionR4Compact)$' --require-benchmarks BenchmarkParserCoreFreshFullCanonical/rewrite,BenchmarkParserCoreFreshFullCanonical/query_compile,BenchmarkParserCoreFreshFullCanonical/language,BenchmarkParserCoreFreshFullCanonical/grammargen_lr,BenchmarkGoParseFullDFA,BenchmarkGoParseIncrementalSingleByteEditDFA,BenchmarkGoParseIncrementalNoEditDFA,BenchmarkAdmissionR4Compact --runs 20 --seed-start 32601 --benchtime 750ms > /evidence/perf-go.log 2>&1
export GTS_ADMISSION_REAL_CORPUS_LANGS=yaml
bash scripts/run_randomized_benchmarks.sh --output /evidence/yaml-head.txt --baseline-root /baseline --baseline-output /evidence/yaml-base.txt --bench-regex '^Benchmark(AdmissionR4Compact|AdmissionR4OwnedExternalLexer|AdmissionYAMLDocumentStreamOwned)$' --require-benchmarks BenchmarkAdmissionR4Compact,BenchmarkAdmissionR4OwnedExternalLexer,BenchmarkAdmissionYAMLDocumentStreamOwned --runs 20 --seed-start 32701 --benchtime 750ms > /evidence/perf-yaml.log 2>&1
printf 'complete\n' > /evidence/perf-complete.txt
