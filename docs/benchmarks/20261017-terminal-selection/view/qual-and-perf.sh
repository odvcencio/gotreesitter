#!/usr/bin/env bash
set -euo pipefail
test -f /evidence/check-complete.txt
test -f /evidence/counter-gate.json
export GOWORK=off GOMAXPROCS=1 GOFLAGS='-p=1 -buildvcs=false -overlay=/evidence/overlay.json'
cd /workspace
go test ./internal/parsercorephase0 -count=1 -timeout=8m > /evidence/core-all.log 2>&1
go test ./internal/parsercorephase0 -tags gts_parsercorephase0,gts_parsercorephase0a -count=1 -timeout=8m > /evidence/core-phase0a.log 2>&1
go test ./internal/parsercorephase0 -race -run '^Test(Materialization|Prebuilt|HiddenTail|VisitMaterialization)' -count=1 -timeout=5m > /evidence/core-race.log 2>&1
GOARCH=386 CGO_ENABLED=0 go test ./internal/parsercorephase0 -run '^Test(Materialization|Prebuilt|HiddenTail|VisitMaterialization)' -count=1 -timeout=5m > /evidence/core-386.log 2>&1
go test . -tags gts_parsercorephase0 -race -run '^Test(CompactHiddenTail|CompactAcceptanceComparisonWorkspace|ParserCoreFreshFullRunnerRepeatedCanonicalLifecycle)' -count=1 -timeout=5m > /evidence/root-race.log 2>&1
GTS_LAYOUT_OWNED_PROBE=1 GTS_ADMISSION_CANDIDATE=1 GOTREESITTER_V1_INVARIANT_LANGUAGE=yaml go test . -tags gts_parsercorephase0 -run '^TestV1InvariantGateR4EditSession$' -count=1 -v -timeout=5m > /evidence/owned-yaml-invariant.log 2>&1
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
bash scripts/run_randomized_benchmarks.sh --output /evidence/go-head.txt --baseline-root /baseline --baseline-output /evidence/go-base.txt --bench-regex '^Benchmark(ParserCoreFreshFullCanonical|GoParseFullDFA|GoParseIncrementalSingleByteEditDFA|GoParseIncrementalNoEditDFA|AdmissionR4Compact|CompactGoFullConsume)$' --require-benchmarks BenchmarkParserCoreFreshFullCanonical/rewrite,BenchmarkParserCoreFreshFullCanonical/query_compile,BenchmarkParserCoreFreshFullCanonical/language,BenchmarkParserCoreFreshFullCanonical/grammargen_lr,BenchmarkGoParseFullDFA,BenchmarkGoParseIncrementalSingleByteEditDFA,BenchmarkGoParseIncrementalNoEditDFA,BenchmarkAdmissionR4Compact,BenchmarkCompactGoFullConsume/real_sample,BenchmarkCompactGoFullConsume/rewrite,BenchmarkCompactGoFullConsume/query_compile,BenchmarkCompactGoFullConsume/language,BenchmarkCompactGoFullConsume/grammargen_lr --runs 20 --seed-start 31901 --benchtime 750ms > /evidence/perf-go.log 2>&1
export GTS_ADMISSION_REAL_CORPUS_LANGS=yaml
bash scripts/run_randomized_benchmarks.sh --output /evidence/yaml-head.txt --baseline-root /baseline --baseline-output /evidence/yaml-base.txt --bench-regex '^Benchmark(AdmissionR4Compact|AdmissionR4OwnedExternalLexer|AdmissionYAMLDocumentStreamOwned)$' --require-benchmarks BenchmarkAdmissionR4Compact,BenchmarkAdmissionR4OwnedExternalLexer,BenchmarkAdmissionYAMLDocumentStreamOwned --runs 20 --seed-start 32001 --benchtime 750ms > /evidence/perf-yaml.log 2>&1
printf 'complete\n' > /evidence/perf-complete.txt
