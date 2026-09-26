#!/usr/bin/env bash

set -euo pipefail

if (($# != 1)); then
	printf 'Usage: bash scripts/bench_baseline.sh OUTPUT\n' >&2
	exit 2
fi

export GOWORK=off

exec bash scripts/run_randomized_benchmarks.sh \
	--output "$1" \
	--bench-regex '^(BenchmarkGoParseFullDFA|BenchmarkGoParseIncrementalSingleByteEditDFA|BenchmarkGoParseIncrementalNoEditDFA)$' \
	--require-benchmarks 'BenchmarkGoParseFullDFA,BenchmarkGoParseIncrementalSingleByteEditDFA,BenchmarkGoParseIncrementalNoEditDFA'
