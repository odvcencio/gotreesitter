#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
DOCKER_RUNNER="$REPO_ROOT/cgo_harness/docker/run_parity_in_docker.sh"
IMAGE_TAG="gotreesitter/cgo-harness:go1.25-local"

usage() {
  cat <<'EOF'
Usage: run_one_in_docker.sh <grammar> <corpus-root> <corpus-lock> <expected-lock-sha256> <output-dir> [lean-corpus]

Runs one grammar receipt generator in the locked cgo harness Docker image.
For Lean, pass the extra final argument and use the Lean source lock.
EOF
}

if [[ $# -lt 5 || $# -gt 6 ]]; then
  usage >&2
  exit 2
fi

GRAMMAR="$1"
CORPUS_ROOT="$(cd "$2" && pwd -P)"
CORPUS_LOCK="$(cd "$(dirname "$3")" && pwd -P)/$(basename "$3")"
EXPECTED_LOCK_SHA="$4"
OUTPUT_DIR="$5"
LEAN_CORPUS="${6:-0}"
WALL_TIMEOUT="${GTS_GRAMMAR_RECEIPT_WALL_TIMEOUT:-90m}"
GOTREESITTER_COMMIT="$(git -C "$REPO_ROOT" rev-parse HEAD)"
if [[ -n "$(git -C "$REPO_ROOT" status --porcelain)" ]]; then
  GIT_TREE_DIRTY=true
else
  GIT_TREE_DIRTY=false
fi

if ! [[ "$GRAMMAR" =~ ^[a-z0-9_]+$ ]]; then
  echo "invalid grammar name: $GRAMMAR" >&2
  exit 2
fi
if [[ ! -f "$CORPUS_LOCK" ]]; then
  echo "corpus lock does not exist: $CORPUS_LOCK" >&2
  exit 2
fi
mkdir -p "$OUTPUT_DIR"
OUTPUT_DIR="$(cd "$OUTPUT_DIR" && pwd -P)"

EXTRA_MOUNTS=(
  --mount "$CORPUS_ROOT:/corpus_sources:ro"
  --mount "$CORPUS_LOCK:/input/corpus_sources.lock:ro"
  --mount "$OUTPUT_DIR:/receipts"
)
INNER_ARGS=(
  -grammar "$GRAMMAR"
  -repo-root /workspace
  -gotreesitter-commit "$GOTREESITTER_COMMIT"
  "-git-tree-dirty=$GIT_TREE_DIRTY"
  -corpus-root /corpus_sources
  -corpus-lock /input/corpus_sources.lock
  -corpus-lock-sha256 "$EXPECTED_LOCK_SHA"
  -out "/receipts/$GRAMMAR.json"
)
if [[ "$LEAN_CORPUS" == "1" ]]; then
  INNER_ARGS+=(
    -lean-c-oracle-lock /workspace/cgo_harness/grammar_receipts/lean_c_oracle.lock
  )
fi

TIME_PATH="/receipts/.$GRAMMAR.time"
INNER_CMD="cd /workspace/cgo_harness && /usr/bin/time -f 'receipt_user_seconds=%U\\nreceipt_system_seconds=%S\\nreceipt_elapsed_seconds=%e' -o '$TIME_PATH' env GOWORK=off go run -tags treesitter_c_parity ./cmd/gts_grammar_receipt"
for arg in "${INNER_ARGS[@]}"; do
  INNER_CMD+=" '$arg'"
done

BUILD_ARGS=()
IMAGE_CREATED=0
cleanup() {
  if [[ "$IMAGE_CREATED" == "1" ]]; then
    docker image rm "$IMAGE_TAG" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT
if docker image inspect "$IMAGE_TAG" >/dev/null 2>&1; then
  BUILD_ARGS+=(--no-build)
elif [[ "${GTS_RECEIPT_NO_BUILD:-0}" == "1" ]]; then
  echo "required Docker image is missing: $IMAGE_TAG" >&2
  exit 1
else
  IMAGE_CREATED=1
fi

if [[ "${GTS_GRAMMAR_RECEIPT_ALLOW_CONCURRENT:-0}" != "1" ]] && docker ps --format '{{.Names}}' | grep -Eq '^gts-parity-'; then
  echo "another gts parity container is running; refusing grammar receipt run" >&2
  exit 1
fi

RUN_ARGS=(
  "${BUILD_ARGS[@]}"
  --out-root "$OUTPUT_DIR/.docker"
  --label "grammar-receipt-$GRAMMAR"
  --wall-timeout "$WALL_TIMEOUT"
  "${EXTRA_MOUNTS[@]}"
  -- "$INNER_CMD"
)
set +e
/usr/bin/time -f 'runner_elapsed_seconds=%e' -o "$OUTPUT_DIR/.$GRAMMAR.runner-time" \
  "$DOCKER_RUNNER" "${RUN_ARGS[@]}"
RUN_STATUS=$?
set -e
if [[ "$RUN_STATUS" == "124" ]]; then
  TIMEOUT_CMD="cd /workspace/cgo_harness && env GOWORK=off go run -tags treesitter_c_parity ./cmd/gts_grammar_receipt"
  TIMEOUT_ARGS=(
    -grammar "$GRAMMAR"
    -repo-root /workspace
    -gotreesitter-commit "$GOTREESITTER_COMMIT"
    "-git-tree-dirty=$GIT_TREE_DIRTY"
    -corpus-root /corpus_sources
    -corpus-lock /input/corpus_sources.lock
    -corpus-lock-sha256 "$EXPECTED_LOCK_SHA"
    -out "/receipts/$GRAMMAR.json"
    -record-timeout
    -time-limit "$WALL_TIMEOUT"
  )
  if [[ "$LEAN_CORPUS" == "1" ]]; then
    TIMEOUT_ARGS+=(-lean-c-oracle-lock /workspace/cgo_harness/grammar_receipts/lean_c_oracle.lock)
  fi
  for arg in "${TIMEOUT_ARGS[@]}"; do
    TIMEOUT_CMD+=" '$arg'"
  done
  if [[ "${GTS_GRAMMAR_RECEIPT_ALLOW_CONCURRENT:-0}" != "1" ]] && docker ps --format '{{.Names}}' | grep -Eq '^gts-parity-'; then
    echo "another gts parity container is running; refusing timeout receipt run" >&2
    exit 1
  fi
  "$DOCKER_RUNNER" --no-build \
    --out-root "$OUTPUT_DIR/.docker" \
    --label "grammar-receipt-$GRAMMAR-timeout-record" \
    --wall-timeout 10m \
    "${EXTRA_MOUNTS[@]}" \
    -- "$TIMEOUT_CMD"
  exit 0
fi
exit "$RUN_STATUS"
