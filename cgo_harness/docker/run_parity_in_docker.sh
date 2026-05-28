#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
OUT_ROOT="$REPO_ROOT/harness_out/docker"
LABEL=""

IMAGE_TAG="gotreesitter/cgo-harness:go1.25-local"
MEMORY_LIMIT="8g"
CPUS_LIMIT="4"
# CPUSET_CPUS pins the container to a specific set of physical CPUs via
# docker's --cpuset-cpus. Pinning matters more than CFS quota for benchmark
# stability — without it, the kernel scheduler can move the container
# between cores, blowing cache state and producing 20-30% wall-time
# variance between otherwise identical runs. Empty = no pinning (legacy
# CFS-quota-only behavior).
CPUSET_CPUS=""
PIDS_LIMIT="4096"
PARITY_RUN='^TestParityFreshParse$|^TestParityIncrementalParse$|^TestParityHasNoErrors$|^TestParityIssue3Repros$|^TestParityGLRCanaryGo$|^TestParityGLRCanarySet$|^TestParityGLRCapPressureTopLanguages$|^TestParityHighlight$'
STRICT_SCALA=0
BUILD_IMAGE=1

usage() {
  cat <<'EOF'
Usage: run_parity_in_docker.sh [options] [-- <custom command>]

Options:
  --image <tag>          Docker image tag (default: gotreesitter/cgo-harness:go1.25-local)
  --repo-root <path>     Repository/worktree root mounted at /workspace
  --out-root <path>      Artifact output root (default: <repo-root>/harness_out/docker)
  --label <name>         Optional run label (used in container/artifact naming)
  --memory <limit>       Container memory limit (default: 8g)
  --cpus <count>         CPU limit passed to Docker (default: 4)
  --cpuset-cpus <list>   Pin container to specific CPUs via --cpuset-cpus
                         (e.g. "18" or "16-19"). Empty = no pinning, but
                         benchmark stability suffers — use this for any
                         perf-comparison run.
  --pids <count>         PID limit passed to Docker (default: 4096)
  --run <regex>          go test -run regex for default parity command
  --strict-scala         Also run strict Scala real-world parity probe
  --no-build             Skip docker build step
  -h, --help             Show this help

Environment passthrough (if set):
  GOTOOLCHAIN
  GOMAXPROCS
  GOT_GLR_MAX_STACKS
  GOT_GLR_V2_COMPACT_FULL_LEAVES
  GOT_GLR_V2_PENDING_PARENTS
  GOT_PARSE_NODE_LIMIT_SCALE
  GOT_GLR_FORCE_CONFLICT_WIDTH
  GTS_PARITY_SKIP_LANGS

Artifacts are written to <out-root>/<timestamp>[-<label>]/:
  - container.log
  - inspect.json
  - metadata.txt
EOF
}

CUSTOM_CMD=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --image)
      IMAGE_TAG="$2"
      shift 2
      ;;
    --repo-root)
      REPO_ROOT="$2"
      shift 2
      ;;
    --out-root)
      OUT_ROOT="$2"
      shift 2
      ;;
    --label)
      LABEL="$2"
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
    --cpuset-cpus)
      CPUSET_CPUS="$2"
      shift 2
      ;;
    --pids)
      PIDS_LIMIT="$2"
      shift 2
      ;;
    --run)
      PARITY_RUN="$2"
      shift 2
      ;;
    --strict-scala)
      STRICT_SCALA=1
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

REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"
OUT_ROOT="${OUT_ROOT/#\~/$HOME}"
if [[ ! -d "$REPO_ROOT" ]]; then
  echo "repo root does not exist: $REPO_ROOT" >&2
  exit 2
fi
mkdir -p "$OUT_ROOT"

sanitize_label() {
  local in="$1"
  in="${in,,}"
  in="$(echo "$in" | sed -E 's/[^a-z0-9_.-]+/-/g; s/^-+//; s/-+$//; s/-+/-/g')"
  if [[ -z "$in" ]]; then
    in="run"
  fi
  echo "$in"
}

LABEL_SLUG=""
if [[ -n "$LABEL" ]]; then
  LABEL_SLUG="$(sanitize_label "$LABEL")"
fi

if [[ "$BUILD_IMAGE" == "1" ]]; then
  docker build -t "$IMAGE_TAG" "$SCRIPT_DIR"
fi

STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT_DIR="$OUT_ROOT/$STAMP"
if [[ -n "$LABEL_SLUG" ]]; then
  OUT_DIR="${OUT_DIR}-${LABEL_SLUG}"
fi
mkdir -p "$OUT_DIR"

DEFAULT_CMD="cd /workspace/cgo_harness && /usr/bin/time -v go test . -tags treesitter_c_parity -run '$PARITY_RUN' -count=1 -v"
if [[ "$STRICT_SCALA" == "1" ]]; then
  DEFAULT_CMD="$DEFAULT_CMD && /usr/bin/time -v env GTS_PARITY_SCALA_REALWORLD_STRICT=1 go test . -tags treesitter_c_parity -run '^TestParityScalaRealWorldCorpus$' -count=1 -v"
fi

if [[ ${#CUSTOM_CMD[@]} -gt 0 ]]; then
  INNER_CMD="${CUSTOM_CMD[*]}"
else
  INNER_CMD="$DEFAULT_CMD"
fi
INNER_CMD="export PATH=/usr/local/go/bin:\$PATH; $INNER_CMD"

ENV_ARGS=()
for var in GOTOOLCHAIN GOMAXPROCS GOT_GLR_MAX_STACKS GOT_GLR_V2_COMPACT_FULL_LEAVES GOT_GLR_V2_PENDING_PARENTS GOT_PARSE_NODE_LIMIT_SCALE GOT_GLR_FORCE_CONFLICT_WIDTH GTS_PARITY_MODE GTS_PARITY_SKIP_LANGS; do
  if [[ -n "${!var:-}" ]]; then
    ENV_ARGS+=("-e" "$var=${!var}")
  fi
done

CONTAINER_NAME="gts-parity-${STAMP,,}"
if [[ -n "$LABEL_SLUG" ]]; then
  CONTAINER_NAME="${CONTAINER_NAME}-${LABEL_SLUG}"
fi
CID=""
cleanup() {
  if [[ -n "$CID" ]]; then
    docker rm -f "$CID" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

CPUSET_ARGS=()
if [[ -n "$CPUSET_CPUS" ]]; then
  CPUSET_ARGS+=(--cpuset-cpus "$CPUSET_CPUS")
fi

CID="$(docker create \
  --name "$CONTAINER_NAME" \
  --init \
  --memory "$MEMORY_LIMIT" \
  --memory-swap "$MEMORY_LIMIT" \
  --cpus "$CPUS_LIMIT" \
  "${CPUSET_ARGS[@]}" \
  --pids-limit "$PIDS_LIMIT" \
  --mount "type=bind,src=$REPO_ROOT,dst=/workspace" \
  --mount "type=volume,src=gotreesitter-go-mod-cache,dst=/go/pkg/mod" \
  --mount "type=volume,src=gotreesitter-go-build-cache,dst=/root/.cache/go-build" \
  "${ENV_ARGS[@]}" \
  "$IMAGE_TAG" \
  bash -c "$INNER_CMD")"

docker start "$CID" >/dev/null
docker logs -f "$CID" 2>&1 | tee "$OUT_DIR/container.log"
EXIT_CODE="$(docker wait "$CID")"
docker inspect "$CID" >"$OUT_DIR/inspect.json"

OOM_KILLED="$(docker inspect -f '{{.State.OOMKilled}}' "$CID")"
STATE_ERROR="$(docker inspect -f '{{.State.Error}}' "$CID")"

{
  echo "container_name=$CONTAINER_NAME"
  echo "container_id=$CID"
  echo "image=$IMAGE_TAG"
  echo "memory=$MEMORY_LIMIT"
  echo "cpus=$CPUS_LIMIT"
  echo "cpuset_cpus=$CPUSET_CPUS"
  echo "pids=$PIDS_LIMIT"
  echo "strict_scala=$STRICT_SCALA"
  echo "exit_code=$EXIT_CODE"
  echo "oom_killed=$OOM_KILLED"
  echo "state_error=$STATE_ERROR"
  echo "repo_root=$REPO_ROOT"
  echo "out_root=$OUT_ROOT"
  echo "label=$LABEL_SLUG"
  echo "command=$INNER_CMD"
} >"$OUT_DIR/metadata.txt"

echo "docker parity run complete"
echo "artifacts: $OUT_DIR"
echo "exit_code: $EXIT_CODE"
echo "oom_killed: $OOM_KILLED"
if [[ -n "$STATE_ERROR" ]]; then
  echo "docker_state_error: $STATE_ERROR"
fi

if [[ "$EXIT_CODE" != "0" ]]; then
  exit "$EXIT_CODE"
fi
