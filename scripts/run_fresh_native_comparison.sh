#!/usr/bin/env bash
# Compare both Go revisions against the same adjacent native C controls.
set -euo pipefail
if (($# < 3 || $# > 6)); then
  echo 'Usage: run_fresh_native_comparison.sh BASELINE_ROOT OUTPUT_DIR LANGUAGE [CYCLES] [SIZES_KIB] [legacy|default]' >&2
  exit 2
fi
baseline_root=$(realpath "$1")
output_dir=$(realpath -m "$2")
language=$3
cycles=10
sizes=32,137,1024
route=legacy
if (($# >= 4)); then cycles=$4; fi
if (($# >= 5)); then sizes=$5; fi
if (($# >= 6)); then route=$6; fi
if [[ ! "$language" =~ ^[a-z0-9_]+$ || ! "$cycles" =~ ^[1-9][0-9]*$ ||
      ! "$sizes" =~ ^(32|137|1024)(,(32|137|1024))*$ ||
      ! "$route" =~ ^(legacy|default)$ ]]; then
  echo 'Invalid language, cycle count, sizes or route' >&2
  exit 2
fi
script_dir=$(cd -- "$(dirname -- "$0")" && pwd -P)
repo_root=$(dirname "$script_dir")
if [[ ! -d "$baseline_root" || ! -d "$repo_root/cgo_harness" ]]; then
  echo 'Both benchmark roots must exist' >&2
  exit 2
fi
go_benchmark=BenchmarkFreshGenerated
if [[ "$route" == default ]]; then go_benchmark=BenchmarkFreshGeneratedDefault; fi
size_words=$(tr ',' ' ' <<< "$sizes")
size_regex=$(tr ',' '|' <<< "$sizes")
required_go=
required_c=
seen_sizes=' '
for size in $size_words; do
  if [[ "$seen_sizes" == *" $size "* ]]; then
    echo 'Duplicate size' >&2
    exit 2
  fi
  seen_sizes="$seen_sizes$size "
  if [[ -n "$required_go" ]]; then
    required_go="$required_go,"
    required_c="$required_c,"
  fi
  required_go="$required_go$go_benchmark/$language/$size""KiB"
  required_c="$required_c""BenchmarkFreshGeneratedStaticC/$language/$size""KiB"
done
mkdir -p "$output_dir"
export GOWORK=off GOMAXPROCS=1

run_sample() {
  local root=$1 axis=$2 role=$3 seed=$4 benchmark required status path
  benchmark=$go_benchmark
  required=$required_go
  if [[ "$axis" == c ]]; then
    benchmark=BenchmarkFreshGeneratedStaticC
    required=$required_c
  fi
  path="$output_dir/$axis-$role-$language-$seed.txt"
  while true; do
    status=0
    (
      cd "$root"
      bash "$repo_root/scripts/run_randomized_benchmarks.sh" \
        --runs 1 --seed-start "$seed" --output "$path" \
        --tags 'treesitter_c_parity treesitter_c_perfscan' \
        --bench-regex "^$benchmark$/^$language$/^($size_regex)KiB$" \
        --require-benchmarks "$required"
    ) > "$output_dir/$axis-$role-$language-$seed.log" 2>&1 || status=$?
    if ((status == 0)); then break; fi
    if ((status != 75)); then return "$status"; fi
    # Lock contention writes no samples; retry this unchanged measurement.
    sleep 5
  done
  for size in $size_words; do
    if ! awk -v name="$benchmark/$language/$size""KiB" \
      '$1 == name { count++ } END { exit count != 1 }' "$path"; then
      echo "Missing or duplicate sample: $axis $role $language $size seed $seed" >&2
      return 1
    fi
  done
}

for ((seed=1; seed<=cycles*2; seed++)); do
  first=before
  last=after
  first_root=$baseline_root
  last_root=$repo_root
  if ((seed%2==0)); then
    first=after
    last=before
    first_root=$repo_root
    last_root=$baseline_root
  fi
  # One block is baseline-Go, C, C, candidate-Go, reversed on even seeds.
  # Both Go revisions use this block's identical C denominator and seed.
  run_sample "$first_root" go "$first" "$seed"
  run_sample "$repo_root/cgo_harness" c control1 "$seed"
  run_sample "$repo_root/cgo_harness" c control2 "$seed"
  run_sample "$last_root" go "$last" "$seed"
  printf 'native %s %s seed %d/%d complete\n' "$language" "$route" "$seed" "$((cycles*2))"
done
for specification in 'go before' 'go after' 'c control1' 'c control2'; do
  read -r axis role <<< "$specification"
  combined="$output_dir/$axis-$role-$language.txt"
  if [[ -e "$combined" ]]; then
    echo "Combined output already exists: $combined" >&2
    exit 2
  fi
  for ((seed=1; seed<=cycles*2; seed++)); do
    cat "$output_dir/$axis-$role-$language-$seed.txt" >> "$combined"
  done
done
printf 'complete route=%s seeds=%d sizes=%s\n' "$route" "$((cycles*2))" "$sizes" > "$output_dir/$language.done"
