#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
RUNNER="$SCRIPT_DIR/run_parity_in_docker.sh"

IMAGE_TAG="gotreesitter/cgo-harness:go1.25-local"
MEMORY_LIMIT="4g"
CPUS_LIMIT="1"
PIDS_LIMIT="4096"
GOMAXPROCS_VALUE="1"
GOMEMLIMIT_VALUE="3GiB"
LANGS_CSV="go,python,rust,java,c"
MODES_CSV="cgo_full,go_full,go_no_tree,go_parse_query,go_cursor_walk,go_edit,go_noop_incremental"
CORPUS_PATH="cgo_harness/corpus_manifest.json"
QUERIES_PATH="cgo_harness/query_manifest.json"
EDITS_PATH="cgo_harness/edit_fixtures.json"
COUNT="10"
LABEL="parse-gap"
OUT_ROOT="$REPO_ROOT/harness_out/parse_gap"
ALLOW_PARITY_FAIL=0
TIME_PARITY_FAILURES=0
REQUIRE_PARITY_LANGS=""
GATE_ONLY=0
BUILD_IMAGE=1
ARENA_BREAKDOWN=0
PHASE_TIMING=0
HOT_SHAPES=0
EQUIV_COUNTERS=0
RUNTIME_AUDIT=0
REDUCE_TIMING=0
ACTION_TIMING=0

usage() {
  cat <<'EOF'
Usage: run_parse_gap_report.sh [options]

Run the parse-gap report harness in the cgo parity Docker image.

Options:
  --image <tag>             Docker image tag (default: gotreesitter/cgo-harness:go1.25-local)
  --repo-root <path>        Repository/worktree root mounted at /workspace
  --out-root <path>         Output root (default: harness_out/parse_gap)
  --label <label>           Output run label (default: parse-gap)
  --langs <list>            Comma-separated languages (default: go,python,rust,java,c)
  --modes <list>            Comma-separated modes
  --corpus <path>           Corpus manifest path (default: cgo_harness/corpus_manifest.json)
  --queries <path>          Query manifest path (default: cgo_harness/query_manifest.json)
  --edits <path>            Edit fixtures path (default: cgo_harness/edit_fixtures.json)
  --count <n>               Iterations per sample/mode (default: 10)
  --memory <limit>          Docker memory limit (default: 4g)
  --cpus <count>            Docker CPU limit (default: 1)
  --pids <count>            Docker PID limit (default: 4096)
  --gomaxprocs <n>          GOMAXPROCS inside container (default: 1)
  --gomemlimit <value>      GOMEMLIMIT inside container (default: 3GiB)
  --allow-parity-fail       Write rows for parity-blocked samples and exit zero unless modes fail
  --require-parity-langs <list>
                            Comma-separated languages that must pass parity even with --allow-parity-fail
  --time-parity-failures    Also run timing modes for parity-blocked samples
  --gate-only               Run parse/highlight/query correctness gates only
  --arena-breakdown         Enable detailed gotreesitter arena breakdown in report rows
  --phase-timing            Enable parser phase/subphase timing in report rows
  --hot-shapes <n>          Include top-N GLR fork/reduce/merge hot-shape rows in runtime JSON
  --equiv-counters          Enable lightweight GLR equivalence attribution counters
  --runtime-audit           Enable heavyweight survivor/runtime audit counters
  --reduce-timing           Enable reduce subphase timing in report rows
  --action-timing           Enable action-dispatch subphase timing in report rows
  --no-build                Skip Docker image build in underlying runner
  -h, --help                Show this help

Artifacts:
  <out-root>/<timestamp>-<label>/results.jsonl
  <out-root>/<timestamp>-<label>/metadata.json
  <out-root>/<timestamp>-<label>/summary.md
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --image) IMAGE_TAG="$2"; shift 2 ;;
    --repo-root) REPO_ROOT="$2"; shift 2 ;;
    --out-root) OUT_ROOT="$2"; shift 2 ;;
    --label) LABEL="$2"; shift 2 ;;
    --langs) LANGS_CSV="$2"; shift 2 ;;
    --modes) MODES_CSV="$2"; shift 2 ;;
    --corpus) CORPUS_PATH="$2"; shift 2 ;;
    --queries) QUERIES_PATH="$2"; shift 2 ;;
    --edits) EDITS_PATH="$2"; shift 2 ;;
    --count) COUNT="$2"; shift 2 ;;
    --memory) MEMORY_LIMIT="$2"; shift 2 ;;
    --cpus) CPUS_LIMIT="$2"; shift 2 ;;
    --pids) PIDS_LIMIT="$2"; shift 2 ;;
    --gomaxprocs) GOMAXPROCS_VALUE="$2"; shift 2 ;;
    --gomemlimit) GOMEMLIMIT_VALUE="$2"; shift 2 ;;
    --allow-parity-fail) ALLOW_PARITY_FAIL=1; shift ;;
    --require-parity-langs) REQUIRE_PARITY_LANGS="$2"; shift 2 ;;
    --time-parity-failures) TIME_PARITY_FAILURES=1; shift ;;
    --gate-only) GATE_ONLY=1; shift ;;
    --arena-breakdown) ARENA_BREAKDOWN=1; shift ;;
    --phase-timing) PHASE_TIMING=1; shift ;;
    --hot-shapes) HOT_SHAPES="$2"; shift 2 ;;
    --equiv-counters) EQUIV_COUNTERS=1; shift ;;
    --runtime-audit) RUNTIME_AUDIT=1; shift ;;
    --reduce-timing) REDUCE_TIMING=1; PHASE_TIMING=1; shift ;;
    --action-timing) ACTION_TIMING=1; PHASE_TIMING=1; shift ;;
    --no-build) BUILD_IMAGE=0; shift ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"
OUT_ROOT="${OUT_ROOT/#\~/$HOME}"
if [[ "$OUT_ROOT" != /* ]]; then
  OUT_ROOT="$REPO_ROOT/$OUT_ROOT"
fi
mkdir -p "$OUT_ROOT"

sanitize_label() {
  local in="$1"
  in="${in,,}"
  in="$(echo "$in" | sed -E 's/[^a-z0-9_.-]+/-/g; s/^-+//; s/-+$//; s/-+/-/g')"
  if [[ -z "$in" ]]; then
    in="parse-gap"
  fi
  echo "$in"
}

LABEL_SLUG="$(sanitize_label "$LABEL")"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT_DIR="$OUT_ROOT/$STAMP-$LABEL_SLUG"
mkdir -p "$OUT_DIR"

case "$OUT_DIR" in
  "$REPO_ROOT"/*) ;;
  *)
    echo "--out-root must resolve under the repo root so the Docker mount can write artifacts" >&2
    echo "repo root: $REPO_ROOT" >&2
    echo "out dir:   $OUT_DIR" >&2
    exit 2
    ;;
esac

OUT_REL="${OUT_DIR#"$REPO_ROOT"/}"
ALLOW_ARG=()
if [[ "$ALLOW_PARITY_FAIL" == "1" ]]; then
  ALLOW_ARG=(--allow-parity-fail)
fi
BUILD_ARG=()
if [[ "$BUILD_IMAGE" == "0" ]]; then
  BUILD_ARG=(--no-build)
fi
REDUCE_CHAIN_HINTS="${GOT_GLR_REDUCE_CHAIN_HINTS-}"
GLR_MAX_STACKS="${GOT_GLR_MAX_STACKS-}"
GLR_MAX_MERGE_PER_KEY="${GOT_GLR_MAX_MERGE_PER_KEY-}"
PARSE_NODE_LIMIT_SCALE="${GOT_PARSE_NODE_LIMIT_SCALE-}"

{
  echo "commit=$(git -C "$REPO_ROOT" rev-parse HEAD)"
  echo "branch=$(git -C "$REPO_ROOT" rev-parse --abbrev-ref HEAD)"
  echo "dirty_status_begin"
  git -C "$REPO_ROOT" status --short
  echo "dirty_status_end"
  echo "image=$IMAGE_TAG"
  echo "memory=$MEMORY_LIMIT"
  echo "cpus=$CPUS_LIMIT"
  echo "pids=$PIDS_LIMIT"
  echo "gomaxprocs=$GOMAXPROCS_VALUE"
  echo "gomemlimit=$GOMEMLIMIT_VALUE"
  echo "langs=$LANGS_CSV"
  echo "modes=$MODES_CSV"
  echo "corpus=$CORPUS_PATH"
  echo "queries=$QUERIES_PATH"
  echo "edits=$EDITS_PATH"
  echo "count=$COUNT"
  echo "allow_parity_fail=$ALLOW_PARITY_FAIL"
  echo "require_parity_langs=$REQUIRE_PARITY_LANGS"
  echo "time_parity_failures=$TIME_PARITY_FAILURES"
  echo "gate_only=$GATE_ONLY"
  echo "arena_breakdown=$ARENA_BREAKDOWN"
  echo "phase_timing=$PHASE_TIMING"
  echo "hot_shapes=$HOT_SHAPES"
  echo "equiv_counters=$EQUIV_COUNTERS"
  echo "runtime_audit=$RUNTIME_AUDIT"
  echo "reduce_timing=$REDUCE_TIMING"
  echo "action_timing=$ACTION_TIMING"
  echo "got_glr_reduce_chain_hints=$REDUCE_CHAIN_HINTS"
  echo "got_glr_max_stacks=$GLR_MAX_STACKS"
  echo "got_glr_max_merge_per_key=$GLR_MAX_MERGE_PER_KEY"
  echo "got_parse_node_limit_scale=$PARSE_NODE_LIMIT_SCALE"
} >"$OUT_DIR/wrapper-metadata.txt"

allow_arg_text=""
if [[ ${#ALLOW_ARG[@]} -gt 0 ]]; then
  allow_arg_text="--allow-parity-fail"
fi
time_parity_arg_text=""
if [[ "$TIME_PARITY_FAILURES" == "1" ]]; then
  time_parity_arg_text="--time-parity-failures"
fi
require_parity_arg_text=""
if [[ -n "$REQUIRE_PARITY_LANGS" ]]; then
  require_parity_arg_text="--require-parity-langs '$REQUIRE_PARITY_LANGS'"
fi
gate_only_arg_text=""
if [[ "$GATE_ONLY" == "1" ]]; then
  gate_only_arg_text="--gate-only"
fi
arena_breakdown_arg_text=""
if [[ "$ARENA_BREAKDOWN" == "1" ]]; then
  arena_breakdown_arg_text="--arena-breakdown"
fi
phase_timing_arg_text=""
phase_timing_env_text="GOT_PARSE_PHASE_TIMING='0'"
if [[ "$PHASE_TIMING" == "1" ]]; then
  phase_timing_arg_text="--phase-timing"
  phase_timing_env_text="GOT_PARSE_PHASE_TIMING='1'"
fi
hot_shapes_arg_text=""
if [[ "$HOT_SHAPES" != "0" ]]; then
  hot_shapes_arg_text="--hot-shapes '$HOT_SHAPES'"
fi
equiv_counters_arg_text=""
if [[ "$EQUIV_COUNTERS" == "1" ]]; then
  equiv_counters_arg_text="--equiv-counters"
fi
runtime_audit_arg_text=""
if [[ "$RUNTIME_AUDIT" == "1" ]]; then
  runtime_audit_arg_text="--runtime-audit"
fi
reduce_timing_arg_text=""
reduce_timing_env_text="GOT_PARSE_REDUCE_TIMING='0'"
if [[ "$REDUCE_TIMING" == "1" ]]; then
  reduce_timing_arg_text="--reduce-timing"
  reduce_timing_env_text="GOT_PARSE_REDUCE_TIMING='1'"
fi
action_timing_arg_text=""
action_timing_env_text="GOT_PARSE_ACTION_TIMING='0'"
if [[ "$ACTION_TIMING" == "1" ]]; then
  action_timing_arg_text="--action-timing"
  action_timing_env_text="GOT_PARSE_ACTION_TIMING='1'"
fi

inner_cmd=$(cat <<EOF
cd /workspace/cgo_harness
env \
  GOMAXPROCS='$GOMAXPROCS_VALUE' \
  GOMEMLIMIT='$GOMEMLIMIT_VALUE' \
  $phase_timing_env_text \
  $reduce_timing_env_text \
  $action_timing_env_text \
  GOT_GLR_REDUCE_CHAIN_HINTS='$REDUCE_CHAIN_HINTS' \
  GOT_GLR_MAX_STACKS='$GLR_MAX_STACKS' \
  GOT_GLR_MAX_MERGE_PER_KEY='$GLR_MAX_MERGE_PER_KEY' \
  GOT_PARSE_NODE_LIMIT_SCALE='$PARSE_NODE_LIMIT_SCALE' \
  GTS_PARSE_GAP_DOCKER_IMAGE='$IMAGE_TAG' \
  GTS_PARSE_GAP_CPUS='$CPUS_LIMIT' \
  GTS_PARSE_GAP_MEMORY='$MEMORY_LIMIT' \
  go test ./cmd/parse_gap_report -tags 'treesitter_c_parity perf' -run '^$' -count=1
env \
  GOMAXPROCS='$GOMAXPROCS_VALUE' \
  GOMEMLIMIT='$GOMEMLIMIT_VALUE' \
  $phase_timing_env_text \
  $reduce_timing_env_text \
  $action_timing_env_text \
  GOT_GLR_REDUCE_CHAIN_HINTS='$REDUCE_CHAIN_HINTS' \
  GOT_GLR_MAX_STACKS='$GLR_MAX_STACKS' \
  GOT_GLR_MAX_MERGE_PER_KEY='$GLR_MAX_MERGE_PER_KEY' \
  GOT_PARSE_NODE_LIMIT_SCALE='$PARSE_NODE_LIMIT_SCALE' \
  GTS_PARSE_GAP_DOCKER_IMAGE='$IMAGE_TAG' \
  GTS_PARSE_GAP_CPUS='$CPUS_LIMIT' \
  GTS_PARSE_GAP_MEMORY='$MEMORY_LIMIT' \
  /usr/bin/time -v go run -tags 'treesitter_c_parity perf' ./cmd/parse_gap_report \
    --repo-root /workspace \
    --langs '$LANGS_CSV' \
    --modes '$MODES_CSV' \
    --corpus '$CORPUS_PATH' \
    --queries '$QUERIES_PATH' \
    --edits '$EDITS_PATH' \
    --count '$COUNT' \
    --out '/workspace/$OUT_REL' \
    $allow_arg_text \
    $require_parity_arg_text \
    $time_parity_arg_text \
    $gate_only_arg_text \
    $arena_breakdown_arg_text \
    $phase_timing_arg_text \
    $hot_shapes_arg_text \
    $equiv_counters_arg_text \
    $runtime_audit_arg_text \
    $reduce_timing_arg_text \
    $action_timing_arg_text
EOF
)

"$RUNNER" \
  --image "$IMAGE_TAG" \
  --repo-root "$REPO_ROOT" \
  --out-root "$OUT_DIR/docker" \
  --label "$LABEL_SLUG" \
  --memory "$MEMORY_LIMIT" \
  --cpus "$CPUS_LIMIT" \
  --pids "$PIDS_LIMIT" \
  "${BUILD_ARG[@]}" \
  -- "$inner_cmd"

echo "parse gap report complete"
echo "artifacts: $OUT_DIR"
