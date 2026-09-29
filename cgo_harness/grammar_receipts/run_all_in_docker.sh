#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
LOCK_PATH="${GTS_GRAMMAR_RECEIPT_DOCKER_LOCK:-/tmp/gts-docker.lock}"
REPORT_DIR="${GTS_GRAMMAR_RECEIPT_REPORT_DIR:-${1:-/tmp/grammar-receipts}}"
CORPUS_ROOT="${GTS_GRAMMAR_RECEIPT_CORPUS_ROOT:-}"
CORPUS_LOCK="${GTS_GRAMMAR_RECEIPT_CORPUS_LOCK:-}"
IMAGE_TAG="gotreesitter/cgo-harness:go1.25-local"

if [[ $# -gt 1 ]]; then
  echo "Usage: run_all_in_docker.sh [report-dir]" >&2
  exit 2
fi
if [[ ! -d "$CORPUS_ROOT" || ! -f "$CORPUS_LOCK" ]]; then
  echo "set GTS_GRAMMAR_RECEIPT_CORPUS_ROOT and GTS_GRAMMAR_RECEIPT_CORPUS_LOCK to the corpus checkout and its lock" >&2
  exit 2
fi
mkdir -p "$REPORT_DIR"
REPORT_DIR="$(cd "$REPORT_DIR" && pwd -P)"
EXPECTED_LOCK_SHA="$(awk 'NF {print $1; exit}' "$REPO_ROOT/cgo_harness/perf_scan/corpus_sources.lock.sha256")"
GOTREESITTER_COMMIT="$(git -C "$REPO_ROOT" rev-parse HEAD)"
if [[ -n "$(git -C "$REPO_ROOT" status --porcelain)" ]]; then
  echo "worktree must be clean before resuming or generating grammar receipts" >&2
  exit 2
fi
GIT_TREE_DIRTY=false
START_FILE="$REPORT_DIR/.batch-start-epoch"
if [[ ! -f "$START_FILE" ]]; then
  date +%s > "$START_FILE"
fi
: >> "$REPORT_DIR/generator-errors.txt"

(
  cd "$REPO_ROOT"
  sha256sum -c cgo_harness/grammar_receipts/lean_c_oracle.lock.sha256
  sha256sum -c cgo_harness/grammar_receipts/lean_corpus_sources.lock.sha256
)

IMAGE_CREATED=0
LEAN_ROOT=""
cleanup() {
  if [[ "$IMAGE_CREATED" == "1" ]]; then
    docker image rm "$IMAGE_TAG" >/dev/null 2>&1 || true
  fi
  if [[ -n "$LEAN_ROOT" ]]; then
    rm -rf -- "$LEAN_ROOT"
  fi
}
trap cleanup EXIT
IMAGE_STATUS="$(flock "$LOCK_PATH" bash -c '
  set -euo pipefail
  image_tag="$1"
  runner="$2"
  if docker image inspect "$image_tag" >/dev/null 2>&1; then
    printf "existing"
  else
    "$runner" --build-only >/dev/null
    printf "created"
  fi
' bash "$IMAGE_TAG" "$REPO_ROOT/cgo_harness/docker/run_parity_in_docker.sh")"
if [[ "$IMAGE_STATUS" == "created" ]]; then
  IMAGE_CREATED=1
fi
export GTS_RECEIPT_NO_BUILD=1

flock "$LOCK_PATH" bash -c '
  set -euo pipefail
  if docker ps --format "{{.Names}}" | grep -Eq "^gts-parity-"; then
    echo "another gts parity container is running; refusing schema test" >&2
    exit 1
  fi
  exec "$@"
' bash "$REPO_ROOT/cgo_harness/docker/run_parity_in_docker.sh" --no-build \
  --out-root "$REPORT_DIR/.docker" \
  --label grammar-receipt-schema \
  --wall-timeout 15m \
  -- 'cd /workspace && GOWORK=off go test ./internal/grammarreceipt'

receipt_is_current() {
  local receipt="$1" grammar="$2" expected_sha="$3"
  python3 - "$receipt" "$grammar" "$GOTREESITTER_COMMIT" "$expected_sha" <<'PY'
import json, sys
try:
    data=json.load(open(sys.argv[1]))
except (OSError, ValueError):
    raise SystemExit(1)
corpus = data.get("corpus", {})
raise SystemExit(0 if (
    data.get("schema") == "gts-grammar-receipt/v1"
    and data.get("grammar", {}).get("name") == sys.argv[2]
    and data.get("gotreesitter_commit") == sys.argv[3]
    and data.get("git_tree_dirty") is False
    and corpus.get("lock_sha256") == sys.argv[4]
) else 1)
PY
}

mapfile -t GRAMMARS < <(awk '!/^#/ && NF {print $1}' "$REPO_ROOT/grammars/languages.lock")
if [[ "${#GRAMMARS[@]}" -ne 206 ]]; then
  echo "grammars/languages.lock has ${#GRAMMARS[@]} entries, want 206" >&2
  exit 1
fi

LEAN_ROOT="$(mktemp -d /tmp/gts-v1-o4-lean-corpus.XXXXXX)"
mkdir -p "$LEAN_ROOT/lean"
git -C "$LEAN_ROOT/lean" init -q
git -C "$LEAN_ROOT/lean" remote add origin https://github.com/leanprover/lean4
git -C "$LEAN_ROOT/lean" fetch --quiet --depth 1 origin f3b06c705e6c85f5314019d5d3baab0fec5b580c
git -C "$LEAN_ROOT/lean" checkout --quiet --detach f3b06c705e6c85f5314019d5d3baab0fec5b580c

RUN_FAILURES=()
NEW_RESULTS=0
for grammar in "${GRAMMARS[@]}" lean; do
  receipt="$REPORT_DIR/$grammar.json"
  if [[ "$grammar" == "lean" ]]; then
    lock="$SCRIPT_DIR/lean_corpus_sources.lock"
    lock_sha="$(awk 'NF {print $1; exit}' "$SCRIPT_DIR/lean_corpus_sources.lock.sha256")"
  else
    lock="$CORPUS_LOCK"
    lock_sha="$EXPECTED_LOCK_SHA"
  fi
  if receipt_is_current "$receipt" "$grammar" "$lock_sha"; then
    echo "skip $grammar: receipt matches current commit and corpus lock"
    continue
  fi
  if [[ -e "$receipt" ]]; then
    mv "$receipt" "$receipt.invalid.$(date -u +%Y%m%dT%H%M%SZ)"
  fi

  if [[ "$grammar" == "lean" ]]; then
    RUN_ARGS=("$grammar" "$LEAN_ROOT" "$lock" "$lock_sha" "$REPORT_DIR" 1)
  else
    RUN_ARGS=("$grammar" "$CORPUS_ROOT" "$lock" "$lock_sha" "$REPORT_DIR")
  fi
  set +e
  flock "$LOCK_PATH" "$SCRIPT_DIR/run_one_in_docker.sh" "${RUN_ARGS[@]}"
  RUN_STATUS=$?
  set -e
  if [[ "$RUN_STATUS" != "0" ]]; then
    RUN_FAILURES+=("$grammar")
    printf '%s\terror\t%s\texit=%s\ttime_limit=%s\n' "$grammar" "$(date -u +%FT%TZ)" "$RUN_STATUS" "${GTS_GRAMMAR_RECEIPT_WALL_TIMEOUT:-90m}" >> "$REPORT_DIR/generator-errors.txt"
  fi

  if receipt_is_current "$receipt" "$grammar" "$lock_sha"; then
    ((NEW_RESULTS+=1))
    if (( NEW_RESULTS % 20 == 0 )); then
      printf '%s completed=%d grammar=%s receipts=%d\n' "$(date -u +%FT%TZ)" "$NEW_RESULTS" "$grammar" "$(find "$REPORT_DIR" -maxdepth 1 -type f -name '*.json' | wc -l)" >> "$REPORT_DIR/batch-progress.log"
      printf '%s\n' "$(( $(date +%s) - $(cat "$START_FILE") ))" > "$REPORT_DIR/.run-wall-seconds"
      python3 "$REPO_ROOT/scripts/summarize_grammar_receipts.py" --report-dir "$REPORT_DIR"
    else
      echo "completed $grammar"
    fi
  else
    RUN_FAILURES+=("$grammar")
    printf '%s\terror\t%s\tno-valid-receipt\ttime_limit=%s\n' "$grammar" "$(date -u +%FT%TZ)" "${GTS_GRAMMAR_RECEIPT_WALL_TIMEOUT:-90m}" >> "$REPORT_DIR/generator-errors.txt"
    echo "$grammar: no valid receipt emitted" >&2
  fi
done

printf '%s\n' "$(( $(date +%s) - $(cat "$START_FILE") ))" > "$REPORT_DIR/.run-wall-seconds"
python3 "$REPO_ROOT/scripts/summarize_grammar_receipts.py" --report-dir "$REPORT_DIR"
if [[ "${#RUN_FAILURES[@]}" -gt 0 ]]; then
  printf 'receipt generation errors for %d grammar(s): %s\n' "${#RUN_FAILURES[@]}" "${RUN_FAILURES[*]}" >&2
  exit 1
fi
