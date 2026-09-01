#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RUNNER="$SCRIPT_DIR/run_parity_in_docker.sh"
WORKTREE_PATH_GUARD="$SCRIPT_DIR/../../scripts/validate_worktree_path.sh"

IMAGE_TAG="gotreesitter/cgo-harness:go1.25-local"
MEMORY_LIMIT="8g"
CPUS_LIMIT="4"
PIDS_LIMIT="4096"
MAX_PARALLEL="2"
BUILD_IMAGE=1
STRICT_SCALA=0
RUN_REGEX=""
OUT_ROOT=""
ALLOW_HOST_OVERSUBSCRIBE=0
declare -a EXPERIMENTS=()
declare -a CUSTOM_CMD=()

usage() {
  cat <<'EOF'
Usage: run_parity_experiments.sh [options]
       run_parity_experiments.sh [options] -- <custom command>

Run multiple parity experiments against different worktrees/repo roots with
bounded parallelism. Each experiment is defined as:

  --experiment <label>=<repo_root>

Examples:
  run_parity_experiments.sh \
    --experiment main=/home/me/work/gotreesitter \
    --experiment glr=/home/me/work/gts-glr \
    --max-parallel 2 --memory 6g

  run_parity_experiments.sh \
    --experiment scala=/home/me/work/gts-scala \
    --strict-scala \
    -- "cd /workspace/cgo_harness && go test . -tags treesitter_c_parity -run '^TestParityScalaRealWorldCorpus$' -count=1 -v"

Options:
  --experiment <label>=<repo_root>  Add an experiment (repeatable)
  --max-parallel <n>                Max concurrent experiments (default: 2)
  --image <tag>                     Docker image tag
  --memory <limit>                  Per-container memory limit (default: 8g)
  --cpus <count>                    Per-container CPU limit (default: 4)
  --pids <count>                    Per-container PID limit (default: 4096)
  --run <regex>                     go test -run regex (default command only)
  --strict-scala                    Include strict scala probe (default command only)
  --out-root <path>                 Shared artifact root for experiment runs
  --allow-host-oversubscribe        Allow max-parallel * memory to exceed the
                                    host memory guard. Intended only for
                                    dedicated CI hosts.
  --no-build                        Skip docker build step
  -h, --help                        Show help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --experiment)
      EXPERIMENTS+=("$2")
      shift 2
      ;;
    --max-parallel)
      MAX_PARALLEL="$2"
      shift 2
      ;;
    --image)
      IMAGE_TAG="$2"
      shift 2
      ;;
    --memory)
      MEMORY_LIMIT="$2"
      shift 2
      ;;
    --cpus)
      CPUS_LIMIT="$2"
      shift 2
      ;;
    --pids)
      PIDS_LIMIT="$2"
      shift 2
      ;;
    --run)
      RUN_REGEX="$2"
      shift 2
      ;;
    --strict-scala)
      STRICT_SCALA=1
      shift
      ;;
    --out-root)
      OUT_ROOT="$2"
      shift 2
      ;;
    --allow-host-oversubscribe)
      ALLOW_HOST_OVERSUBSCRIBE=1
      shift
      ;;
    --no-build)
      BUILD_IMAGE=0
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      CUSTOM_CMD=("$@")
      break
      ;;
    *)
      echo "unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ "${#EXPERIMENTS[@]}" -eq 0 ]]; then
  echo "at least one --experiment is required" >&2
  usage >&2
  exit 2
fi

if ! [[ "$MAX_PARALLEL" =~ ^[1-9][0-9]*$ ]]; then
  echo "invalid --max-parallel: $MAX_PARALLEL" >&2
  exit 2
fi

if [[ ! -x "$WORKTREE_PATH_GUARD" ]]; then
  echo "worktree path guard is not executable: $WORKTREE_PATH_GUARD" >&2
  exit 2
fi
for exp in "${EXPERIMENTS[@]}"; do
  label="${exp%%=*}"
  repo="${exp#*=}"
  if [[ -z "$label" || -z "$repo" || "$label" == "$exp" ]]; then
    echo "invalid --experiment format: $exp (want <label>=<repo_root>)" >&2
    exit 2
  fi
  repo="${repo/#\~/$HOME}"
  "$WORKTREE_PATH_GUARD" "$repo" >/dev/null
done

docker_memory_limit_to_bytes() {
  local value="$1"
  local number unit
  value="${value//[[:space:]]/}"
  if [[ "$value" =~ ^([0-9]+)([bBkKmMgG]?)$ ]]; then
    number="${BASH_REMATCH[1]}"
    unit="${BASH_REMATCH[2],,}"
  else
    return 1
  fi
  case "$unit" in
    ""|b) printf '%s\n' "$number" ;;
    k) printf '%s\n' "$((number * 1024))" ;;
    m) printf '%s\n' "$((number * 1024 * 1024))" ;;
    g) printf '%s\n' "$((number * 1024 * 1024 * 1024))" ;;
    *) return 1 ;;
  esac
}

host_mem_available_bytes() {
  awk '/^MemAvailable:/ { printf "%.0f\n", $2 * 1024 }' /proc/meminfo 2>/dev/null
}

guard_parallel_memory_budget() {
  local effective_parallel="$MAX_PARALLEL"
  if [[ "$effective_parallel" -gt "${#EXPERIMENTS[@]}" ]]; then
    effective_parallel="${#EXPERIMENTS[@]}"
  fi
  if [[ "$effective_parallel" -le 1 || "$ALLOW_HOST_OVERSUBSCRIBE" == "1" ]]; then
    return 0
  fi
  local limit_bytes available_bytes aggregate_bytes guard_bytes
  limit_bytes="$(docker_memory_limit_to_bytes "$MEMORY_LIMIT" || true)"
  available_bytes="$(host_mem_available_bytes || true)"
  if [[ -z "$limit_bytes" || -z "$available_bytes" ]]; then
    echo "warning: could not parse memory guard inputs; proceeding with --max-parallel=$MAX_PARALLEL memory=$MEMORY_LIMIT" >&2
    return 0
  fi
  aggregate_bytes="$((limit_bytes * effective_parallel))"
  guard_bytes="$((available_bytes * 80 / 100))"
  if [[ "$aggregate_bytes" -gt "$guard_bytes" ]]; then
    {
      echo "refusing --max-parallel=$MAX_PARALLEL with --memory=$MEMORY_LIMIT: aggregate container memory exceeds 80% of host MemAvailable"
      echo "effective_parallel=$effective_parallel"
      echo "aggregate_bytes=$aggregate_bytes memavailable_bytes=$available_bytes guard_bytes=$guard_bytes"
      echo "lower --max-parallel/--memory or pass --allow-host-oversubscribe on a dedicated host"
    } >&2
    exit 2
  fi
}

guard_parallel_memory_budget

if [[ "$BUILD_IMAGE" == "1" ]]; then
  docker build -t "$IMAGE_TAG" "$SCRIPT_DIR"
fi

STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
if [[ -n "$OUT_ROOT" ]]; then
  OUT_ROOT="${OUT_ROOT/#\~/$HOME}"
  mkdir -p "$OUT_ROOT"
fi

declare -a pids=()
declare -a labels=()
overall_status=0

wait_for_one() {
  local i pid rc
  if [[ "${#pids[@]}" -eq 0 ]]; then
    return 0
  fi
  pid="${pids[0]}"
  if wait "$pid"; then
    rc=0
  else
    rc=$?
    overall_status=1
  fi
  echo "[done] ${labels[0]} exit=$rc"
  pids=("${pids[@]:1}")
  labels=("${labels[@]:1}")
}

for exp in "${EXPERIMENTS[@]}"; do
  label="${exp%%=*}"
  repo="${exp#*=}"
  if [[ -z "$label" || -z "$repo" || "$label" == "$exp" ]]; then
    echo "invalid --experiment format: $exp (want <label>=<repo_root>)" >&2
    exit 2
  fi
  repo="${repo/#\~/$HOME}"
  if [[ ! -d "$repo" ]]; then
    echo "experiment repo root does not exist: $repo" >&2
    exit 2
  fi

  while [[ "${#pids[@]}" -ge "$MAX_PARALLEL" ]]; do
    wait_for_one
  done

  cmd=(
    "$RUNNER"
    --no-build
    --image "$IMAGE_TAG"
    --memory "$MEMORY_LIMIT"
    --cpus "$CPUS_LIMIT"
    --pids "$PIDS_LIMIT"
    --repo-root "$repo"
    --label "$label"
  )
  if [[ -n "$OUT_ROOT" ]]; then
    cmd+=(--out-root "$OUT_ROOT")
  fi
  if [[ -n "$RUN_REGEX" ]]; then
    cmd+=(--run "$RUN_REGEX")
  fi
  if [[ "$STRICT_SCALA" == "1" ]]; then
    cmd+=(--strict-scala)
  fi
  if [[ "${#CUSTOM_CMD[@]}" -gt 0 ]]; then
    cmd+=(-- "${CUSTOM_CMD[@]}")
  fi

  echo "[start] label=$label repo=$repo"
  "${cmd[@]}" &
  pids+=("$!")
  labels+=("$label")
done

while [[ "${#pids[@]}" -gt 0 ]]; do
  wait_for_one
done

exit "$overall_status"
