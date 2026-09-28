#!/usr/bin/env bash

set -euo pipefail

if (($# < 1)); then
	printf 'Usage: bash scripts/bench_baseline.sh OUTPUT [paired benchmark options]\n' >&2
	exit 2
fi

output_path=$1
shift
export GOWORK=off

exec bash scripts/run_randomized_benchmarks.sh \
	--output "$output_path" \
	--bench-regex '^(BenchmarkGoParseFullDFA|BenchmarkGoParseIncrementalSingleByteEditDFA|BenchmarkGoParseIncrementalNoEditDFA)$' \
	--require-benchmarks 'BenchmarkGoParseFullDFA,BenchmarkGoParseIncrementalSingleByteEditDFA,BenchmarkGoParseIncrementalNoEditDFA' \
	"$@"
