#!/usr/bin/env bash
# Pinned A/B bench harness for grammargen/parser-core changes.
#
# Reduces system noise to detect <5% improvements:
#   - taskset pins to a single dedicated CPU (default: 18, near the end so
#     the OS scheduler is less likely to want it)
#   - alternates A and B runs in rounds to cancel out drift / thermals
#   - aggregates with benchstat for proper statistical comparison
#
# Usage:
#   ./ab_pinned.sh <baseline-ref> [<treatment-ref>] [-- <extra go-test args>]
#
# Examples:
#   ./ab_pinned.sh HEAD                       # baseline only, save numbers
#   ./ab_pinned.sh HEAD~1 HEAD                # compare HEAD vs HEAD~1
#   ./ab_pinned.sh main glr-post83-parser-core
#
# Env knobs:
#   AB_LANG       language to bench (default: javascript)
#   AB_BENCH      bench filter (default: BenchmarkParityRealCorpusParseFull/javascript/gotreesitter)
#   AB_BENCHTIME  -benchtime value (default: 10x)
#   AB_ROUNDS     number of A/B rounds (default: 10 → 10 baseline + 10 treatment)
#   AB_CPU        CPU index to pin (default: 18)
#   AB_OUT        output dir (default: cgo_harness/bench/runs/<timestamp>)
#   AB_PHASE_TIMING set "1" to enable GOT_PARSE_PHASE_TIMING (default: 1)
#
# This script:
#   1. snapshots the working tree (must be clean)
#   2. checks out baseline, runs N rounds
#   3. checks out treatment, runs N rounds (interleaved if AB_INTERLEAVE=1)
#   4. restores working tree
#   5. benchstat the two output files

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

LANG_NAME="${AB_LANG:-javascript}"
BENCH="${AB_BENCH:-BenchmarkParityRealCorpusParseFull/${LANG_NAME}/gotreesitter}"
BENCHTIME="${AB_BENCHTIME:-10x}"
ROUNDS="${AB_ROUNDS:-10}"
CPU_PIN="${AB_CPU:-18}"
PHASE_TIMING="${AB_PHASE_TIMING:-1}"
TS="$(date +%Y%m%dT%H%M%SZ)"
OUT_DIR="${AB_OUT:-${REPO_ROOT}/cgo_harness/bench/runs/${TS}}"
INTERLEAVE="${AB_INTERLEAVE:-1}"

BASELINE_REF="${1:-}"
TREATMENT_REF="${2:-}"
if [[ -z "$BASELINE_REF" ]]; then
  echo "usage: $0 <baseline-ref> [<treatment-ref>]" >&2
  exit 2
fi

mkdir -p "$OUT_DIR"
BASELINE_OUT="$OUT_DIR/baseline.bench"
TREATMENT_OUT="$OUT_DIR/treatment.bench"
: >"$BASELINE_OUT"
[[ -n "$TREATMENT_REF" ]] && : >"$TREATMENT_OUT"

# Snapshot to restore later (the user might have local commits we don't want to
# disturb). We record HEAD and re-check it out at the end.
ORIG_HEAD="$(git -C "$REPO_ROOT" rev-parse HEAD)"
if ! git -C "$REPO_ROOT" diff-index --quiet HEAD --; then
  echo "ERROR: working tree dirty. Commit or stash before running A/B." >&2
  exit 3
fi

cleanup() {
  git -C "$REPO_ROOT" checkout --quiet "$ORIG_HEAD" || true
}
trap cleanup EXIT

# Resolve refs to commits so detached-HEAD checkouts succeed.
BASELINE_SHA="$(git -C "$REPO_ROOT" rev-parse "$BASELINE_REF")"
TREATMENT_SHA=""
[[ -n "$TREATMENT_REF" ]] && TREATMENT_SHA="$(git -C "$REPO_ROOT" rev-parse "$TREATMENT_REF")"

run_one() {
  local label="$1" sha="$2" outfile="$3" round_id="$4"
  git -C "$REPO_ROOT" checkout --quiet "$sha"
  local timing_env=""
  [[ "$PHASE_TIMING" == "1" ]] && timing_env="GOT_PARSE_PHASE_TIMING=1"
  local raw
  raw=$(cd "$REPO_ROOT/cgo_harness" && \
    taskset -c "$CPU_PIN" env GTS_REAL_CORPUS_BENCH_LANGS="$LANG_NAME" \
      GTS_REAL_CORPUS_BENCH_SKIP_MISMATCH=1 $timing_env \
      go test . -tags treesitter_c_parity -run='^$' \
      -bench="^${BENCH}\$" -benchtime="$BENCHTIME" -count=1 2>&1 | \
    grep "^Benchmark")
  if [[ -n "$raw" ]]; then
    # Strip phase-timing decorations so benchstat sees a clean ns/op line.
    # benchstat expects "BenchmarkName-N <iters> <ns/op> ns/op [optional more metrics]"
    # The bench output already starts with that, but has long per-counter trailing
    # text. Keep just the first 4 tokens + " ns/op" + ALL remaining counters that
    # have ns/op suffix so benchstat can compare phase timings too.
    echo "# round=$round_id sha=$sha label=$label cpu=$CPU_PIN" >>"$outfile"
    echo "$raw" >>"$outfile"
  else
    echo "# round=$round_id sha=$sha label=$label cpu=$CPU_PIN FAILED-NO-OUTPUT" >>"$outfile"
  fi
}

echo "A/B harness: baseline=$BASELINE_SHA treatment=${TREATMENT_SHA:-<none>}"
echo "  rounds=$ROUNDS bench=$BENCH cpu=$CPU_PIN benchtime=$BENCHTIME phase=$PHASE_TIMING"
echo "  out=$OUT_DIR"
echo ""

if [[ -n "$TREATMENT_SHA" && "$INTERLEAVE" == "1" ]]; then
  for r in $(seq 1 "$ROUNDS"); do
    printf "round %2d/%2d B... " "$r" "$ROUNDS"
    run_one baseline  "$BASELINE_SHA"  "$BASELINE_OUT"  "$r"
    printf "T... "
    run_one treatment "$TREATMENT_SHA" "$TREATMENT_OUT" "$r"
    printf "ok\n"
  done
else
  for r in $(seq 1 "$ROUNDS"); do
    printf "baseline round %2d/%2d... " "$r" "$ROUNDS"
    run_one baseline "$BASELINE_SHA" "$BASELINE_OUT" "$r"
    printf "ok\n"
  done
  if [[ -n "$TREATMENT_SHA" ]]; then
    for r in $(seq 1 "$ROUNDS"); do
      printf "treatment round %2d/%2d... " "$r" "$ROUNDS"
      run_one treatment "$TREATMENT_SHA" "$TREATMENT_OUT" "$r"
      printf "ok\n"
    done
  fi
fi

echo ""
if [[ -n "$TREATMENT_SHA" ]]; then
  echo "=== benchstat ==="
  benchstat -col '/sha' "$BASELINE_OUT" "$TREATMENT_OUT"
else
  echo "=== benchstat (baseline only) ==="
  benchstat "$BASELINE_OUT"
fi
echo ""
echo "raw output: $OUT_DIR"
