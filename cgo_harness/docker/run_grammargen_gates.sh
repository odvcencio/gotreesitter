#!/usr/bin/env bash
#
# Grammargen lane gate runner.
#
# Three gate levels enforce the parser/grammargen contract:
#
#   Gate 0 — Compiler Health (grammargen-only)
#     Import, generate, emit stable artifacts. No panics, no OOM.
#     "The compiler still works."
#
#   Gate 1 — Parser Correctness Protection (hard stop)
#     Unit tests, top50 smoke, CGo parity canaries.
#     Ratcheted: no new fails, no pass→drift, no timeout/killed.
#     "The parser is still correct."
#
#   Gate 2 — Promotion Board (only after Gate 1)
#     Reduced frontier (7-language breaker set), full parity, perf trio.
#     "Grammargen earns promotion into the parser-locked lane."
#
# The key rule: a grammargen change must never "fix" parser correctness by
# also changing the parser at the same time.
#
# Parser lock point: ff0bf8d (fix/top50-parity-burndown)
#
# Usage:
#   run_grammargen_gates.sh --gate 0         # compiler health only
#   run_grammargen_gates.sh --gate 1         # parser correctness
#   run_grammargen_gates.sh --gate 2         # promotion board
#   run_grammargen_gates.sh --gate all       # run 0 → 1 → 2 sequentially
#   run_grammargen_gates.sh --gate 0,1       # run 0 then 1
#
# Options:
#   --gate <level>       Gate level(s): 0, 1, 2, all, or comma-separated
#   --repo-root <path>   Repository/worktree root (default: auto-detect)
#   --memory <limit>     Container memory limit (default: 8g, gate2 uses 12g)
#   --cpus <count>       CPU limit (default: 4)
#   --label <name>       Run label suffix
#   --no-build           Skip docker image build
#   --frontier-langs <l> Override reduced frontier languages (comma-separated)
#   -h, --help           Show this help
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

GATE_LEVELS=""
MEMORY_LIMIT="8g"
CPUS_LIMIT="4"
PIDS_LIMIT="4096"
LABEL=""
BUILD_IMAGE=1
FRONTIER_LANGS="bash,c,c_sharp,cpp,dart,python,scala"
IMAGE_TAG="gotreesitter/cgo-harness:go1.24-local"

PARSER_LOCK_COMMIT="ff0bf8d"

usage() {
  cat <<'EOF'
Usage: run_grammargen_gates.sh --gate <level> [options]

Gate levels:
  0       Compiler health (grammargen-only tests)
  1       Parser correctness protection (unit + CGo canaries)
  2       Promotion board (frontier + full parity + perf)
  all     Run 0 → 1 → 2 sequentially (stop on first failure)
  0,1     Comma-separated subset

Options:
  --gate <level>          Required. Gate level(s) to run.
  --repo-root <path>      Repository/worktree root
  --memory <limit>        Container memory limit (default: 8g)
  --cpus <count>          CPU limit (default: 4)
  --label <name>          Run label suffix
  --no-build              Skip docker image build
  --frontier-langs <list> Override reduced frontier (default: bash,c,c_sharp,cpp,dart,python,scala)
  -h, --help              Show this help

Policy:
  grammargen-next may merge forward if Gate 0 passes and reduced frontier
  does not regress. It may merge into the parser-locked lane only if Gate 1
  is non-regressing and Gate 2 is neutral-or-better.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --gate)         GATE_LEVELS="$2"; shift 2 ;;
    --repo-root)    REPO_ROOT="$2"; shift 2 ;;
    --memory)       MEMORY_LIMIT="$2"; shift 2 ;;
    --cpus)         CPUS_LIMIT="$2"; shift 2 ;;
    --label)        LABEL="$2"; shift 2 ;;
    --no-build)     BUILD_IMAGE=0; shift ;;
    --frontier-langs) FRONTIER_LANGS="$2"; shift 2 ;;
    -h|--help)      usage; exit 0 ;;
    *)              echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if [[ -z "$GATE_LEVELS" ]]; then
  echo "error: --gate is required" >&2
  usage >&2
  exit 2
fi

REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"
OUT_ROOT="$REPO_ROOT/harness_out/gates"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
LABEL_SLUG=""
if [[ -n "$LABEL" ]]; then
  LABEL_SLUG="$(echo "$LABEL" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9_.-]+/-/g; s/^-+//; s/-+$//; s/-+/-/g')"
fi

# Parse gate levels into array.
declare -a GATES=()
if [[ "$GATE_LEVELS" == "all" ]]; then
  GATES=(0 1 2)
else
  IFS=',' read -r -a GATES <<< "$GATE_LEVELS"
fi

# Build image once if needed.
if [[ "$BUILD_IMAGE" == "1" ]]; then
  echo "=== Building Docker image ==="
  docker build -t "$IMAGE_TAG" "$SCRIPT_DIR" 2>&1 | tail -3
  echo ""
fi

# Helper: run a command inside Docker, capture output, return exit code.
run_in_docker() {
  local label="$1"
  local mem="$2"
  local inner_cmd="$3"

  local out_dir="$OUT_ROOT/$STAMP"
  if [[ -n "$LABEL_SLUG" ]]; then
    out_dir="${out_dir}-${LABEL_SLUG}"
  fi
  out_dir="${out_dir}/${label}"
  mkdir -p "$out_dir"

  local container_name="gts-gate-${label}-${STAMP,,}"
  local cid=""
  local cleanup_fn="docker rm -f \"\$cid\" >/dev/null 2>&1 || true"

  local full_cmd="export PATH=/usr/local/go/bin:\$PATH; $inner_cmd"

  cid="$(docker create \
    --name "$container_name" \
    --init \
    --dns 10.255.255.254 --dns 8.8.8.8 \
    --memory "$mem" \
    --memory-swap "$mem" \
    --cpus "$CPUS_LIMIT" \
    --pids-limit "$PIDS_LIMIT" \
    --mount "type=bind,src=$REPO_ROOT,dst=/workspace" \
    --mount "type=volume,src=gotreesitter-go-mod-cache,dst=/go/pkg/mod" \
    --mount "type=volume,src=gotreesitter-go-build-cache,dst=/root/.cache/go-build" \
    "$IMAGE_TAG" \
    bash -c "$full_cmd")"

  docker start "$cid" >/dev/null
  docker logs -f "$cid" 2>&1 | tee "$out_dir/container.log"
  local exit_code
  exit_code="$(docker wait "$cid")"

  local oom_killed
  oom_killed="$(docker inspect -f '{{.State.OOMKilled}}' "$cid" 2>/dev/null || echo "unknown")"

  {
    echo "gate=$label"
    echo "exit_code=$exit_code"
    echo "oom_killed=$oom_killed"
    echo "memory=$mem"
    echo "cpus=$CPUS_LIMIT"
    echo "parser_lock=$PARSER_LOCK_COMMIT"
    echo "timestamp=$STAMP"
    echo "repo_root=$REPO_ROOT"
  } > "$out_dir/metadata.txt"

  docker rm -f "$cid" >/dev/null 2>&1 || true

  if [[ "$oom_killed" == "true" ]]; then
    echo "*** GATE $label: OOM KILLED ***"
    echo "oom" > "$out_dir/verdict.txt"
    return 137
  fi

  if [[ "$exit_code" == "0" ]]; then
    echo "pass" > "$out_dir/verdict.txt"
  else
    echo "fail" > "$out_dir/verdict.txt"
  fi

  return "$exit_code"
}

# ─── Gate 0: Compiler Health ───────────────────────────────────────────────────
gate0() {
  echo "╔══════════════════════════════════════════════════════════════╗"
  echo "║  GATE 0: Compiler Health                                    ║"
  echo "║  Import · Generate · Emit · No panics · No OOM             ║"
  echo "╚══════════════════════════════════════════════════════════════╝"
  echo ""

  local cmd
  read -r -d '' cmd <<'GATE0_CMD' || true
set -eo pipefail
cd /workspace

echo "--- Gate 0a: grammargen unit tests ---"
go test ./grammargen -run '^Test(JSON|Calc|GLR|Keyword|Ext|AliasSuper|Parity|MultiGrammarImportPipeline)' \
  -count=1 -v -timeout 10m 2>&1

echo ""
echo "--- Gate 0b: grammargen generation determinism ---"
go test ./grammargen -run '^TestParityGenerationDeterministic$' \
  -count=1 -v -timeout 5m 2>&1

echo ""
echo "--- Gate 0c: grammargen JSON golden gate ---"
go test ./grammargen -run '^TestParityJSONCorrectnessGolden$' \
  -count=1 -v -timeout 2m 2>&1

echo ""
echo "=== GATE 0 PASSED ==="
GATE0_CMD

  run_in_docker "gate0" "$MEMORY_LIMIT" "$cmd"
}

# ─── Gate 1: Parser Correctness Protection ─────────────────────────────────────
gate1() {
  echo "╔══════════════════════════════════════════════════════════════╗"
  echo "║  GATE 1: Parser Correctness Protection                      ║"
  echo "║  Unit tests · Top50 · CGo canaries · HARD STOP              ║"
  echo "║  Lock point: $PARSER_LOCK_COMMIT                                      ║"
  echo "╚══════════════════════════════════════════════════════════════╝"
  echo ""

  local cmd
  read -r -d '' cmd <<'GATE1_CMD' || true
set -eo pipefail
cd /workspace

echo "--- Gate 1a: full unit test suite ---"
go test ./... -count=1 -timeout 15m 2>&1

echo ""
echo "--- Gate 1b: top50 smoke (no errors) ---"
go test ./grammars -run '^TestTop50ParseSmokeNoErrors$' \
  -count=1 -v -timeout 5m 2>&1

echo ""
echo "--- Gate 1c: CGo parity canaries ---"
cd /workspace/cgo_harness
go test . -tags treesitter_c_parity \
  -run '^TestParityFreshParse$|^TestParityHasNoErrors$|^TestParityIssue3Repros$|^TestParityGLRCanaryGo$' \
  -count=1 -v -timeout 20m 2>&1

echo ""
echo "=== GATE 1 PASSED ==="
GATE1_CMD

  run_in_docker "gate1" "$MEMORY_LIMIT" "$cmd"
}

# ─── Gate 2: Promotion Board ──────────────────────────────────────────────────
gate2() {
  local frontier_display="${FRONTIER_LANGS//,/, }"
  echo "╔══════════════════════════════════════════════════════════════╗"
  echo "║  GATE 2: Promotion Board                                    ║"
  echo "║  Reduced frontier · Full parity · Perf trio                 ║"
  echo "║  Frontier: $frontier_display"
  echo "╚══════════════════════════════════════════════════════════════╝"
  echo ""

  # Gate 2 uses more memory for real-corpus work.
  local g2_memory="12g"

  local cmd
  cmd="$(cat <<GATE2_CMD
set -eo pipefail
cd /workspace

echo "--- Gate 2a: reduced frontier (grammargen real-corpus) ---"
echo "    Languages: $FRONTIER_LANGS"

# Clone the frontier repos.
mkdir -p /tmp/grammar_parity
LOCK_FILE="/workspace/grammars/languages.lock"

clone_repo() {
  local name="\$1"
  local url="\$2"
  local dest="/tmp/grammar_parity/\$name"
  for attempt in 1 2 3; do
    if [[ -d "\$dest/.git" ]]; then
      return 0
    fi
    rm -rf "\$dest"
    git clone --depth=1 "\$url" "\$dest" && return 0
    sleep 2
  done
  echo "WARN: failed to clone \$name" >&2
  return 1
}

# Clone reduced frontier repos.
clone_repo bash https://github.com/tree-sitter/tree-sitter-bash.git || true
clone_repo c https://github.com/tree-sitter/tree-sitter-c.git || true
clone_repo c_sharp https://github.com/tree-sitter/tree-sitter-c-sharp.git || true
clone_repo cpp https://github.com/tree-sitter-grammars/tree-sitter-cpp.git || true
clone_repo dart https://github.com/UserNobworthy/tree-sitter-dart.git || true
clone_repo python https://github.com/tree-sitter/tree-sitter-python.git || true
clone_repo scala https://github.com/tree-sitter/tree-sitter-scala.git || true

# Also clone easy canaries for baseline sanity.
clone_repo json https://github.com/tree-sitter/tree-sitter-json.git || true
clone_repo css https://github.com/tree-sitter/tree-sitter-css.git || true
clone_repo html https://github.com/tree-sitter/tree-sitter-html.git || true

echo '{}' > /tmp/real_corpus_parity_floors.json

/usr/bin/time -v env \\
  GTS_GRAMMARGEN_REAL_CORPUS_ENABLE=1 \\
  GTS_GRAMMARGEN_REAL_CORPUS_ROOT=/tmp/grammar_parity \\
  GTS_GRAMMARGEN_REAL_CORPUS_PROFILE=aggressive \\
  GTS_GRAMMARGEN_REAL_CORPUS_MAX_CASES=25 \\
  GTS_GRAMMARGEN_REAL_CORPUS_MAX_GRAMMARS=0 \\
  GTS_GRAMMARGEN_REAL_CORPUS_ALLOW_PARTIAL=1 \\
  GTS_GRAMMARGEN_REAL_CORPUS_FLOORS_PATH=/tmp/real_corpus_parity_floors.json \\
  go test ./grammargen -run '^TestMultiGrammarImportRealCorpusParity\$' \\
  -count=1 -v -timeout 30m 2>&1

echo ""
echo "--- Gate 2b: full CGo parity board ---"
cd /workspace/cgo_harness
go test . -tags treesitter_c_parity \\
  -run '^TestParityFreshParse\$|^TestParityHasNoErrors\$|^TestParityIssue3Repros\$|^TestParityGLRCanaryGo\$|^TestParityGLRCanarySet\$|^TestParityGLRCapPressureTopLanguages\$|^TestParityHighlight\$' \\
  -count=1 -v -timeout 30m 2>&1

echo ""
echo "--- Gate 2c: perf trio ---"
cd /workspace
GOMAXPROCS=1 go test -bench '^BenchmarkParse' -benchtime=750ms -count=5 -timeout 10m 2>&1

echo ""
echo "=== GATE 2 PASSED ==="
GATE2_CMD
)"

  run_in_docker "gate2" "$g2_memory" "$cmd"
}

# ─── Main: run requested gates sequentially, stop on first failure ─────────────
OVERALL_RESULT=0
GATE_RESULTS=()

for g in "${GATES[@]}"; do
  g="$(echo "$g" | tr -d '[:space:]')"
  case "$g" in
    0)
      if gate0; then
        GATE_RESULTS+=("Gate 0: PASS")
      else
        GATE_RESULTS+=("Gate 0: FAIL")
        OVERALL_RESULT=1
        echo ""
        echo "*** Gate 0 failed. Stopping. ***"
        break
      fi
      ;;
    1)
      if gate1; then
        GATE_RESULTS+=("Gate 1: PASS")
      else
        GATE_RESULTS+=("Gate 1: FAIL")
        OVERALL_RESULT=1
        echo ""
        echo "*** Gate 1 failed. Stopping. grammargen change must not regress parser. ***"
        break
      fi
      ;;
    2)
      if gate2; then
        GATE_RESULTS+=("Gate 2: PASS")
      else
        GATE_RESULTS+=("Gate 2: FAIL (review promotion board)")
        OVERALL_RESULT=1
      fi
      ;;
    *)
      echo "unknown gate level: $g" >&2
      exit 2
      ;;
  esac
  echo ""
done

echo ""
echo "╔══════════════════════════════════════════════════════════════╗"
echo "║  Gate Summary                                                ║"
echo "╠══════════════════════════════════════════════════════════════╣"
for r in "${GATE_RESULTS[@]}"; do
  printf "║  %-58s ║\n" "$r"
done
echo "╠══════════════════════════════════════════════════════════════╣"
printf "║  %-58s ║\n" "Artifacts: $OUT_ROOT/$STAMP"
echo "╚══════════════════════════════════════════════════════════════╝"

exit "$OVERALL_RESULT"
