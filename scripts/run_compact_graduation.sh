#!/usr/bin/env bash
set -euo pipefail

# Invoke inside cgo_harness/docker/run_parity_in_docker.sh with an output
# directory mounted outside the repository. Every test process has one grammar.
# Failed gates remain failed in their .exit files; measurement continues so
# the matrix can explain all blockers rather than hiding cells after a failure.
if [[ "${GTS_PARITY_IN_DOCKER:-}" != 1 ]]; then
  echo "compact graduation measurements require Docker isolation" >&2
  exit 2
fi
if (($# < 1)); then
  echo "usage: scripts/run_compact_graduation.sh OUTPUT_DIR [language ...]" >&2
  exit 2
fi
output=$1
shift
languages=("$@")
if ((${#languages[@]} == 0)); then
  languages=(go java javascript typescript python rust c cpp c_sharp ruby php bash)
fi
repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
mkdir -p "$output"
output=$(cd "$output" && pwd)
export GOWORK=off GOMAXPROCS=1
cd "$repo/cgo_harness"
tags=treesitter_c_parity,gts_engine_ceiling
go test -c -tags "$tags" -o "$output/graduation.test" .
go test -c -tags "$tags,gts_workcount" -o "$output/work.test" .

record_step() {
  local label=$1 status
  shift
  if "$@" >"$output/$label.log" 2>&1; then
    status=0
  else
    status=$?
  fi
  echo "$status" >"$output/$label.exit"
  echo "$label: exit $status"
}

# Correctness and counters precede every timed comparison.
for language in "${languages[@]}"; do
  export GTS_CEILING_LANGUAGE=$language
  for size in 32k 137k 1m; do
    export GTS_CEILING_SIZES=$size
    record_step "correctness-$language-$size" /usr/bin/time -v \
      "$output/graduation.test" -test.run '^TestCompactGraduationCorrectness$' \
      -test.v -test.count=1 -test.timeout=10m
  done
  unset GTS_CEILING_SIZES
  record_step "work-$language" "$output/work.test" \
    -test.run '^TestCompactEditsWork$/(32k|137k|1m)/(fresh|byte)/(legacy|compact)$' \
    -test.v -test.count=1 -test.timeout=10m
done

for language in "${languages[@]}"; do
  export GTS_CEILING_LANGUAGE=$language
  unset GTS_CEILING_SIZES
  record_step "timing-$language" bash "$repo/scripts/run_randomized_benchmarks.sh" \
    --output "$output/timing-$language.txt" --package . --tags "$tags" \
    --bench-regex '^BenchmarkCompactGraduation$' --runs 20 \
    --seed-start 1 --benchtime 750ms --lock-path "$output/randomized.lock"
  export GTS_CEILING_SIZES=1m
  for operation in fresh byte; do
    export GTS_CEILING_RSS_MODE=$operation
    for repeat in 1 2 3; do
      for engine in legacy compact C; do
        export GTS_CEILING_RSS_ENGINE=$engine
        record_step "rss-$language-$operation-$engine-$repeat" /usr/bin/time -v \
          "$output/graduation.test" -test.run '^TestCompactEditsRSS$' \
          -test.v -test.count=1 -test.timeout=10m
      done
    done
  done
done

failed=0
for status in "$output"/*.exit; do
  if [[ "$(cat "$status")" != 0 ]]; then
    failed=1
  fi
done
exit "$failed"
