#!/usr/bin/env bash
set -euo pipefail

# Per-grammar Docker runner for grammargen real corpus parity.
# Runs ONE grammar at a time in its own container with strict memory limits.
# If one grammar OOMs, only its container dies — WSL stays alive.
#
# Usage:
#   ./run_single_grammar_parity.sh python          # test one grammar
#   ./run_single_grammar_parity.sh --all            # test all grammars sequentially
#   ./run_single_grammar_parity.sh --list           # list available grammars
#   ./run_single_grammar_parity.sh --failing        # test only grammars with gaps

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RUNNER="$SCRIPT_DIR/run_parity_in_docker.sh"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

IMAGE_TAG="gotreesitter/cgo-harness:go1.24-local"
MEMORY_LIMIT="8g"
TIMEOUT_PER_GRAMMAR="15m"
MAX_CASES="25"
PROFILE="aggressive"
REPORT_DIR="$REPO_ROOT/cgo_harness/reports"
BUILD_IMAGE=1
SEED_DIR=""
OFFLINE=0
LR_SPLIT=0

# All grammars in the test set (alphabetical order matching importParityGrammars).
ALL_GRAMMARS=(
  bash c_lang comment cpon css csv diff dockerfile dot eds eex elixir forth
  git_config git_rebase gitattributes gitcommit go_lang gomod graphql haskell
  hcl html ini javascript jsdoc json json5 lua make nix ocaml pem php promql
  properties proto python regex requirements ron scala scheme sql ssh_config
  swift todotxt toml yaml
  # Large grammars (previously skipped):
  rust c_sharp java ruby cpp kotlin
)

# Grammars with known parity gaps (from floor file v14).
FAILING_GRAMMARS=(
  bash c_lang comment cpon diff dockerfile dot eex elixir git_config
  gitattributes gitcommit go_lang gomod haskell hcl html ini javascript
  jsdoc lua make nix ocaml php promql python regex requirements scala
  sql swift yaml
  # Large grammars (no baseline yet):
  rust c_sharp java ruby cpp kotlin
)

usage() {
  cat <<'USAGE'
Usage: run_single_grammar_parity.sh [options] <grammar|--all|--failing|--list>

Run grammargen real corpus parity for individual grammars in isolated Docker
containers. Each grammar gets its own container with strict memory limits.

Arguments:
  <grammar>        Test a single grammar by name (e.g. python, bash, scala)
  --all            Test all grammars sequentially
  --failing        Test only grammars with known parity gaps
  --list           List all available grammar names

Options:
  --memory <limit>     Container memory limit (default: 8g)
  --timeout <duration> Go test timeout per grammar (default: 15m)
  --max-cases <n>      Max samples per grammar (default: 25)
  --profile <name>     smoke|balanced|aggressive (default: aggressive)
  --report-dir <path>  Directory for diagnostic logs (default: cgo_harness/reports)
  --seed-dir <path>    Host grammar repos directory (under repo root)
  --offline            Skip network cloning, require --seed-dir
  --lr-split           Enable LR(1) splitting (GTS_GRAMMARGEN_LR_SPLIT=1)
  --no-build           Skip Docker image build
  -h, --help           Show this help

Output:
  Per-grammar logs saved to <report-dir>/diag_<grammar>.log
  Summary line printed to stdout for each grammar.
USAGE
}

MODE=""
TARGET_GRAMMAR=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --all)
      MODE="all"
      shift
      ;;
    --failing)
      MODE="failing"
      shift
      ;;
    --list)
      MODE="list"
      shift
      ;;
    --memory)
      MEMORY_LIMIT="$2"
      shift 2
      ;;
    --timeout)
      TIMEOUT_PER_GRAMMAR="$2"
      shift 2
      ;;
    --max-cases)
      MAX_CASES="$2"
      shift 2
      ;;
    --profile)
      PROFILE="$2"
      shift 2
      ;;
    --report-dir)
      REPORT_DIR="$2"
      shift 2
      ;;
    --seed-dir)
      SEED_DIR="$2"
      shift 2
      ;;
    --offline)
      OFFLINE=1
      shift
      ;;
    --lr-split)
      LR_SPLIT=1
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
    -*)
      echo "unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -n "$MODE" ]]; then
        echo "cannot combine grammar name with --all/--failing/--list" >&2
        exit 2
      fi
      MODE="single"
      TARGET_GRAMMAR="$1"
      shift
      ;;
  esac
done

if [[ -z "$MODE" ]]; then
  echo "error: specify a grammar name, --all, --failing, or --list" >&2
  usage >&2
  exit 2
fi

if [[ "$MODE" == "list" ]]; then
  printf '%s\n' "${ALL_GRAMMARS[@]}"
  exit 0
fi

# Validate seed dir if provided.
CONTAINER_SEED_DIR=""
if [[ -n "$SEED_DIR" ]]; then
  SEED_DIR="${SEED_DIR/#\~/$HOME}"
  if [[ ! -d "$SEED_DIR" ]]; then
    echo "seed dir does not exist: $SEED_DIR" >&2
    exit 2
  fi
  SEED_DIR="$(cd "$SEED_DIR" && pwd)"
  case "$SEED_DIR" in
    "$REPO_ROOT"/*)
      CONTAINER_SEED_DIR="/workspace/${SEED_DIR#"$REPO_ROOT"/}"
      ;;
    *)
      echo "seed dir must be under repo root: $SEED_DIR" >&2
      exit 2
      ;;
  esac
fi

if [[ "$OFFLINE" == "1" && -z "$CONTAINER_SEED_DIR" ]]; then
  echo "--offline requires --seed-dir under repo root" >&2
  exit 2
fi

mkdir -p "$REPORT_DIR"

# Build image once.
if [[ "$BUILD_IMAGE" == "1" ]]; then
  echo "Building Docker image..."
  docker build -t "$IMAGE_TAG" "$SCRIPT_DIR"
  echo ""
fi

# Determine grammar list.
declare -a GRAMMARS
case "$MODE" in
  single)
    GRAMMARS=("$TARGET_GRAMMAR")
    ;;
  all)
    GRAMMARS=("${ALL_GRAMMARS[@]}")
    ;;
  failing)
    GRAMMARS=("${FAILING_GRAMMARS[@]}")
    ;;
esac

# Clone function for Docker inner command.
make_clone_block() {
  local grammar="$1"
  # Map grammar names to repo URLs.
  declare -A REPO_URLS=(
    [bash]="https://github.com/tree-sitter/tree-sitter-bash.git"
    [c_lang]="https://github.com/tree-sitter/tree-sitter-c.git"
    [comment]="https://github.com/stsewd/tree-sitter-comment.git"
    [cpon]="https://github.com/psvz/tree-sitter-cpon.git"
    [css]="https://github.com/tree-sitter/tree-sitter-css.git"
    [csv]="https://github.com/amaanq/tree-sitter-csv.git"
    [diff]="https://github.com/the-mikedavis/tree-sitter-diff.git"
    [dockerfile]="https://github.com/camdencheek/tree-sitter-dockerfile.git"
    [dot]="https://github.com/rydesun/tree-sitter-dot.git"
    [eds]="https://github.com/uyha/tree-sitter-eds.git"
    [eex]="https://github.com/connorlay/tree-sitter-eex.git"
    [elixir]="https://github.com/elixir-lang/tree-sitter-elixir.git"
    [forth]="https://github.com/AlexanderBrevig/tree-sitter-forth.git"
    [git_config]="https://github.com/the-mikedavis/tree-sitter-git-config.git"
    [git_rebase]="https://github.com/the-mikedavis/tree-sitter-git-rebase.git"
    [gitattributes]="https://github.com/tree-sitter-grammars/tree-sitter-gitattributes.git"
    [gitcommit]="https://github.com/gbprod/tree-sitter-gitcommit.git"
    [go_lang]="https://github.com/tree-sitter/tree-sitter-go.git"
    [gomod]="https://github.com/camdencheek/tree-sitter-go-mod.git"
    [graphql]="https://github.com/bkegley/tree-sitter-graphql.git"
    [haskell]="https://github.com/tree-sitter/tree-sitter-haskell.git"
    [hcl]="https://github.com/tree-sitter-grammars/tree-sitter-hcl.git"
    [html]="https://github.com/tree-sitter/tree-sitter-html.git"
    [ini]="https://github.com/justinmk/tree-sitter-ini.git"
    [javascript]="https://github.com/tree-sitter/tree-sitter-javascript.git"
    [jsdoc]="https://github.com/tree-sitter/tree-sitter-jsdoc.git"
    [json]="https://github.com/tree-sitter/tree-sitter-json.git"
    [json5]="https://github.com/Joakker/tree-sitter-json5.git"
    [lua]="https://github.com/tree-sitter-grammars/tree-sitter-lua.git"
    [make]="https://github.com/alemuller/tree-sitter-make.git"
    [nix]="https://github.com/nix-community/tree-sitter-nix.git"
    [ocaml]="https://github.com/tree-sitter/tree-sitter-ocaml.git"
    [pem]="https://github.com/ObserverOfTime/tree-sitter-pem.git"
    [php]="https://github.com/tree-sitter/tree-sitter-php.git"
    [promql]="https://github.com/MichaHoffmann/tree-sitter-promql.git"
    [properties]="https://github.com/tree-sitter-grammars/tree-sitter-properties.git"
    [proto]="https://github.com/treywood/tree-sitter-proto.git"
    [python]="https://github.com/tree-sitter/tree-sitter-python.git"
    [regex]="https://github.com/tree-sitter/tree-sitter-regex.git"
    [requirements]="https://github.com/tree-sitter-grammars/tree-sitter-requirements.git"
    [ron]="https://github.com/amaanq/tree-sitter-ron.git"
    [scala]="https://github.com/tree-sitter/tree-sitter-scala.git"
    [scheme]="https://github.com/6cdh/tree-sitter-scheme.git"
    [sql]="https://github.com/m-novikov/tree-sitter-sql.git"
    [ssh_config]="https://github.com/tree-sitter-grammars/tree-sitter-ssh-config.git"
    [swift]="https://github.com/tree-sitter/tree-sitter-swift.git"
    [todotxt]="https://github.com/arnarg/tree-sitter-todotxt.git"
    [toml]="https://github.com/tree-sitter/tree-sitter-toml.git"
    [yaml]="https://github.com/tree-sitter-grammars/tree-sitter-yaml.git"
    [rust]="https://github.com/tree-sitter/tree-sitter-rust.git"
    [c_sharp]="https://github.com/tree-sitter/tree-sitter-c-sharp.git"
    [java]="https://github.com/tree-sitter/tree-sitter-java.git"
    [ruby]="https://github.com/tree-sitter/tree-sitter-ruby.git"
    [cpp]="https://github.com/tree-sitter/tree-sitter-cpp.git"
    [kotlin]="https://github.com/fwcd/tree-sitter-kotlin.git"
  )

  # Map grammar names to repo directory names (some differ).
  declare -A REPO_NAMES=(
    [c_lang]="c"
    [go_lang]="go"
    [gitcommit]="gitcommit_gbprod"
    [c_sharp]="c_sharp"
  )

  local repo_name="${REPO_NAMES[$grammar]:-$grammar}"
  local url="${REPO_URLS[$grammar]:-}"

  if [[ -z "$url" ]]; then
    echo "# Unknown grammar: $grammar — no clone URL"
    return
  fi

  cat <<CLONE_EOF
if [[ ! -d "/tmp/grammar_parity/$repo_name" ]]; then
  git clone --depth=1 "$url" "/tmp/grammar_parity/$repo_name" || echo "WARN: clone failed for $grammar"
fi
CLONE_EOF
}

run_grammar() {
  local grammar="$1"
  local log_file="$REPORT_DIR/diag_${grammar}.log"

  echo "=== Testing: $grammar (memory=$MEMORY_LIMIT, timeout=$TIMEOUT_PER_GRAMMAR) ==="

  # Build inner command for Docker.
  local lr_split_env=""
  if [[ "$LR_SPLIT" == "1" ]]; then
    lr_split_env="GTS_GRAMMARGEN_LR_SPLIT=1"
  fi

  local seed_block=""
  if [[ -n "$CONTAINER_SEED_DIR" ]]; then
    seed_block="
if [[ -d \"$CONTAINER_SEED_DIR\" ]]; then
  for src in \"$CONTAINER_SEED_DIR\"/*; do
    [[ -d \"\$src\" ]] || continue
    name=\"\$(basename \"\$src\")\"
    rm -rf \"/tmp/grammar_parity/\$name\"
    cp -a \"\$src\" \"/tmp/grammar_parity/\$name\"
  done
fi"
  fi

  local clone_block=""
  if [[ "$OFFLINE" != "1" ]]; then
    clone_block="$(make_clone_block "$grammar")"
  fi

  local inner_cmd
  read -r -d '' inner_cmd <<INNER_EOF || true
set -eo pipefail
export PATH=/usr/local/go/bin:\$PATH
export GOMEMLIMIT=6GiB
mkdir -p /tmp/grammar_parity
$seed_block
$clone_block

echo '{}' > /tmp/real_corpus_parity_floors.json
cd /workspace
/usr/bin/time -v env \
  GTS_GRAMMARGEN_REAL_CORPUS_ENABLE=1 \
  GTS_GRAMMARGEN_REAL_CORPUS_ROOT=/tmp/grammar_parity \
  GTS_GRAMMARGEN_REAL_CORPUS_PROFILE=$PROFILE \
  GTS_GRAMMARGEN_REAL_CORPUS_MAX_CASES=$MAX_CASES \
  GTS_GRAMMARGEN_REAL_CORPUS_ALLOW_PARTIAL=1 \
  GTS_GRAMMARGEN_REAL_CORPUS_FLOORS_PATH=/tmp/real_corpus_parity_floors.json \
  GTS_GRAMMARGEN_REAL_CORPUS_ONLY=$grammar \
  $lr_split_env \
  go test ./grammargen -run '^TestMultiGrammarImportRealCorpusParity\$' -count=1 -v -timeout $TIMEOUT_PER_GRAMMAR
INNER_EOF

  local exit_code=0
  "$RUNNER" \
    --image "$IMAGE_TAG" \
    --repo-root "$REPO_ROOT" \
    --memory "$MEMORY_LIMIT" \
    --label "diag-${grammar}" \
    --no-build \
    -- "$inner_cmd" 2>&1 | tee "$log_file" || exit_code=$?

  # Extract summary line from log.
  local summary
  summary=$(grep -E 'real-corpus\[' "$log_file" 2>/dev/null | tail -1 || echo "NO SUMMARY")
  local is_oom="false"
  if grep -q '^oom_killed: true$' "$log_file" 2>/dev/null; then
    is_oom="true"
  fi

  if [[ "$is_oom" == "true" ]]; then
    echo "RESULT: $grammar — OOM KILLED"
  elif [[ "$exit_code" != "0" ]]; then
    echo "RESULT: $grammar — FAILED (exit=$exit_code) | $summary"
  else
    echo "RESULT: $grammar — OK | $summary"
  fi
  echo ""

  return 0  # Always continue to next grammar.
}

# Run grammars.
total=${#GRAMMARS[@]}
echo "Running $total grammar(s) with per-grammar Docker isolation"
echo "Memory: $MEMORY_LIMIT | Timeout: $TIMEOUT_PER_GRAMMAR | Profile: $PROFILE | Cases: $MAX_CASES"
echo "Reports: $REPORT_DIR"
echo ""

passed=0
failed=0
oom=0
for grammar in "${GRAMMARS[@]}"; do
  run_grammar "$grammar" || true
  # Check result from log.
  log="$REPORT_DIR/diag_${grammar}.log"
  if grep -q '^oom_killed: true$' "$log" 2>/dev/null; then
    ((oom++)) || true
  elif grep -q '^exit_code: 0$' "$log" 2>/dev/null; then
    ((passed++)) || true
  else
    ((failed++)) || true
  fi
done

echo "========================================="
echo "SUMMARY: $passed passed, $failed failed, $oom OOM out of $total grammars"
echo "Reports saved to: $REPORT_DIR"
