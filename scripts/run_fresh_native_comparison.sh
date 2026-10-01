#!/usr/bin/env bash
# Run complete operations through the standard randomized protocol, one grammar
# per process. Use inside the parity Docker image with an authenticated C cache.
set -euo pipefail
if (($# < 3 || $# > 4)); then
  echo 'Usage: run_fresh_native_comparison.sh BASELINE_ROOT OUTPUT_DIR LANGUAGE [CYCLES]' >&2
  exit 2
fi
baseline_root=$(realpath "$1")
output_dir=$(realpath -m "$2")
language=$3
cycles=${4:-10}
if [[ ! "$language" =~ ^[a-z0-9_]+$ || ! "$cycles" =~ ^[1-9][0-9]*$ ]]; then
  echo 'Invalid language or cycle count' >&2
  exit 2
fi
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
repo_root=$(dirname "$script_dir")
if [[ ! -d "$baseline_root" || ! -d "$repo_root/cgo_harness" ]]; then
  echo 'Both benchmark roots must exist' >&2
  exit 2
fi
mkdir -p "$output_dir"
export GOWORK=off GOMAXPROCS=1
cd "$repo_root/cgo_harness"
regex="^(BenchmarkFreshGenerated|BenchmarkFreshGeneratedStaticC)$/^${language}$/^(32|137|1024)KiB$"
for ((cycle=1; cycle<=cycles; cycle++)); do
  seed=$((cycle*2-1))
  roles=(before after)
  if ((cycle%2==0)); then roles=(after before); fi
  for role in "${roles[@]}"; do
    go_root=$repo_root
    if [[ "$role" == before ]]; then go_root=$baseline_root; fi
    # Each two-seed comparison is Go-C-C-Go. Role order alternates across
    # cycles so baseline and candidate experience the same shared-host load.
    while true; do
      status=0
      bash "$repo_root/scripts/run_randomized_benchmarks.sh" \
        --runs 2 --seed-start "$seed" \
        --output "$output_dir/c-${role}-${language}-${cycle}.txt" \
        --baseline-root "$go_root" \
        --baseline-output "$output_dir/go-${role}-${language}-${cycle}.txt" \
        --tags 'treesitter_c_parity treesitter_c_perfscan' --bench-regex "$regex" \
        > "$output_dir/${role}-${language}-${cycle}.log" 2>&1 || status=$?
      if ((status == 0)); then break; fi
      if ((status != 75)); then exit "$status"; fi
      # A busy benchmark lock creates no samples. Wait for the other campaign
      # before retrying this whole Go-C-C-Go cycle.
      sleep 5
    done
    for axis in go c; do
      benchmark=BenchmarkFreshGenerated
      if [[ "$axis" == c ]]; then benchmark=BenchmarkFreshGeneratedStaticC; fi
      for size in 32 137 1024; do
        if ! awk -v name="$benchmark/$language/${size}KiB" \
          '$1 == name { count++ } END { exit count != 2 }' \
          "$output_dir/${axis}-${role}-${language}-${cycle}.txt"; then
          echo "Missing or duplicate samples: $axis $role $language $size cycle $cycle" >&2
          exit 1
        fi
      done
    done
  done
  printf 'native %s cycle %d/%d complete\n' "$language" "$cycle" "$cycles"
done
for role in before after; do
  for axis in go c; do
    combined="$output_dir/${axis}-${role}-${language}.txt"
    if [[ -e "$combined" ]]; then
      echo "Combined output already exists: $combined" >&2
      exit 2
    fi
    for ((cycle=1; cycle<=cycles; cycle++)); do
      cat "$output_dir/${axis}-${role}-${language}-${cycle}.txt" >> "$combined"
    done
  done
done
printf 'complete\n' > "$output_dir/${language}.done"
