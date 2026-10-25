#!/usr/bin/env bash
set -euo pipefail

# Run after the uninstrumented gate. GOT_STATS changes the measured workload;
# these samples explain costs and must never replace the timing-gate inputs.
if (($# != 4)); then
  echo 'usage: run_go_edit_attribution.sh BASE_ROOT OUTPUT_DIR BASE_BINARY HEAD_BINARY' >&2
  exit 2
fi
baseline_root=$1
output_dir=$2
baseline_binary=$3
head_binary=$4
mkdir -p "$output_dir"
output_dir=$(cd "$output_dir" && pwd)
export GOMAXPROCS=1

{
  go version
  lscpu
} > "$output_dir/environment.txt"

# Verbose testing repeats the benchmark name after its STATS output, keeping
# each final timing row intact for the driver's completeness checks.
GOT_STATS=1 GOFLAGS="${GOFLAGS:-} -v" bash scripts/run_randomized_benchmarks.sh \
  --baseline-root "$baseline_root" \
  --baseline-output "$output_dir/base-stats.txt" \
  --output "$output_dir/head-stats.txt" \
  --runs 20 --seed-start 1 --benchtime 750ms --package . --tags '' \
  --bench-regex '^BenchmarkGoParseIncrementalSingleByteEditDFA$' \
  --require-benchmarks BenchmarkGoParseIncrementalSingleByteEditDFA

# Sampling the ordinary entry point also accounts for work outside the
# profiled parser's reuse/rebuild timers, including source copying and Release.
for role in base head; do
  binary=$head_binary
  root=$PWD
  if [[ $role == base ]]; then
    binary=$baseline_binary
    root=$baseline_root
  fi
  (
    cd "$root"
    unset GOT_STATS
    "$binary" -test.run '^$' \
      -test.bench '^BenchmarkGoParseIncrementalSingleByteEditDFA$' \
      -test.shuffle 1 -test.count 1 -test.benchtime 10s -test.benchmem \
      -test.cpuprofile "$output_dir/$role.cpu.pprof" \
      > "$output_dir/$role-profile.txt" 2>&1
  )
  go tool pprof -top -nodecount=60 "$binary" "$output_dir/$role.cpu.pprof" \
    > "$output_dir/$role-cpu.txt"
done
