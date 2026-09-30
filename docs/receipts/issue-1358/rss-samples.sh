#!/usr/bin/env bash
set -euo pipefail
if [[ $# != 6 ]]; then
  printf 'usage: %s fixture baseline-go fixed-go native-c c-grammar output-directory\n' "$0" >&2
  exit 2
fi
fixture=$1
baseline_go=$2
fixed_go=$3
native_c=$4
c_grammar=$5
output=$6
mkdir -p "$output"
export GOWORK=off GOMAXPROCS=1 GOMEMLIMIT=6GiB
for seed in $(seq 1 20); do
  for mode in full insert noedit; do
    if (( seed % 2 == 1 )); then roles='base c head'; else roles='head c base'; fi
    for role in $roles; do
      prefix="$output/$mode-$role-$seed"
      case "$role" in
        c) /usr/bin/time -v "$native_c" "$fixture" "$mode" 10 "$c_grammar" > "$prefix.out" 2> "$prefix.time" ;;
        base) /usr/bin/time -v env ALLOW_EXPECTED_ERROR=1 "$baseline_go" "$fixture" "$mode" 10 > "$prefix.out" 2> "$prefix.time" ;;
        head) /usr/bin/time -v "$fixed_go" "$fixture" "$mode" 10 > "$prefix.out" 2> "$prefix.time" ;;
      esac
    done
  done
done
