#!/usr/bin/env bash
set -euo pipefail

# Per-grammar Docker runner for grammargen real corpus parity.
# Runs each grammar in its own container with strict memory limits.
# If one grammar OOMs, only its container dies — WSL stays alive.
#
# Usage:
#   ./run_single_grammar_parity.sh python          # test one grammar
#   ./run_single_grammar_parity.sh --langs css,c_lang --jobs 2
#   ./run_single_grammar_parity.sh --all            # test all grammars sequentially
#   ./run_single_grammar_parity.sh --list           # list available grammars
#   ./run_single_grammar_parity.sh --failing        # test only grammars with gaps

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RUNNER="$SCRIPT_DIR/run_parity_in_docker.sh"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

DEFAULT_MEMORY_LIMIT="8g"
DEFAULT_CPUS_LIMIT="4"
DEFAULT_PIDS_LIMIT="4096"
DEFAULT_TIMEOUT_PER_GRAMMAR="15m"
DEFAULT_MAX_CASES="25"
DEFAULT_PROFILE="aggressive"
DEFAULT_GOMAXPROCS_VALUE=""
DEFAULT_GOFLAGS_VALUE=""
DEFAULT_JOBS="1"

FORTRAN_SAFE_MEMORY_LIMIT="3g"
FORTRAN_SAFE_CPUS_LIMIT="1"
FORTRAN_SAFE_PIDS_LIMIT="512"
FORTRAN_SAFE_GOMAXPROCS_VALUE="1"
FORTRAN_SAFE_GOFLAGS_VALUE="-p=1"
FORTRAN_SAFE_LR0_CORE_BUDGET="160000000"
FORTRAN_SAFE_GENERATE_TIMEOUT="15m"

IMAGE_TAG="gotreesitter/cgo-harness:go1.25-local"
MEMORY_LIMIT="$DEFAULT_MEMORY_LIMIT"
CPUS_LIMIT="$DEFAULT_CPUS_LIMIT"
PIDS_LIMIT="$DEFAULT_PIDS_LIMIT"
TIMEOUT_PER_GRAMMAR="$DEFAULT_TIMEOUT_PER_GRAMMAR"
MAX_CASES="$DEFAULT_MAX_CASES"
PROFILE="$DEFAULT_PROFILE"
REPORT_DIR="$REPO_ROOT/cgo_harness/reports"
BUILD_IMAGE=1
SEED_DIR=""
OFFLINE=0
LR_SPLIT=0
GOMAXPROCS_VALUE="$DEFAULT_GOMAXPROCS_VALUE"
GOFLAGS_VALUE="$DEFAULT_GOFLAGS_VALUE"
LR0_CORE_BUDGET=""
GENERATE_TIMEOUT=""
FORTRAN_SAFE_DEFAULTS=1
REQUIRE_PARITY=0
JOBS="$DEFAULT_JOBS"
ALLOW_HOST_OVERSUBSCRIBE=0

MEMORY_SET=0
CPUS_SET=0
PIDS_SET=0
GOMAXPROCS_SET=0
GOFLAGS_SET=0
LR0_CORE_BUDGET_SET=0
GENERATE_TIMEOUT_SET=0

# All grammars in the test set (alphabetical order matching importParityGrammars).
ALL_GRAMMARS=(
  bash c_lang comment cpon css csv dart diff dockerfile dot eds eex elixir forth
  git_config git_rebase gitattributes gitcommit go_lang gomod graphql haskell
  hcl html ini javascript jsdoc json json5 lua make nix ocaml pem php promql
  properties proto python regex requirements ron scala scheme sql ssh_config
  swift todotxt toml yaml
  # Large grammars (previously skipped):
  rust c_sharp java ruby crystal cpp kotlin cuda
  # TypeScript family:
  typescript tsx
  # Legacy/enterprise languages:
  cobol fortran perl erlang d
)

# Grammars with known parity gaps (from floor file v14).
FAILING_GRAMMARS=(
  bash c_lang comment cpon diff dockerfile dot eex elixir git_config
  gitattributes gitcommit go_lang gomod haskell hcl html ini javascript
  jsdoc lua make nix ocaml php promql python regex requirements scala
  sql swift yaml
  # Large grammars (no baseline yet):
  rust c_sharp java ruby crystal cpp kotlin
)

usage() {
  cat <<'USAGE'
Usage: run_single_grammar_parity.sh [options] <grammar|--langs <list>|--all|--failing|--list>

Run grammargen real corpus parity for individual grammars in isolated Docker
containers. Each grammar gets its own container with strict memory limits.

Arguments:
  <grammar>        Test a single grammar by name (e.g. python, bash, scala)
  --langs <list>   Test a comma-separated grammar list (e.g. css,c_lang)
  --all            Test all grammars sequentially
  --failing        Test only grammars with known parity gaps
  --list           List all available grammar names

Options:
  --memory <limit>     Container memory limit (default: 8g)
  --cpus <count>       Container CPU limit (default: 4)
  --pids <count>       Container PID limit (default: 4096)
  --timeout <duration> Go test timeout per grammar (default: 15m)
  --max-cases <n>      Max samples per grammar (default: 25)
  --profile <name>     smoke|balanced|aggressive (default: aggressive)
  --report-dir <path>  Directory for diagnostic logs (default: cgo_harness/reports)
  --seed-dir <path>    Host grammar repos directory (under repo root)
  --offline            Skip network cloning, require --seed-dir
  --lr-split           Enable LR(1) splitting (GTS_GRAMMARGEN_LR_SPLIT=1)
  --gomaxprocs <n>     Export GOMAXPROCS inside the container
  --goflags <value>    Export GOFLAGS inside the container (for example: -p=1)
  --lr0-core-budget <n>
                       Export GOT_LALR_LR0_CORE_BUDGET inside the container.
                       If unset, fortran defaults to 160000000 so the
                       compact LALR path can finish while still bounding
                       runaway LR(0) growth.
  --generate-timeout <dur>
                       Export GTS_GRAMMARGEN_REAL_CORPUS_GENERATE_TIMEOUT.
                       If unset, fortran defaults to 15m so the memory-safe
                       LALR path can finish in Docker.
  --require-parity     Require every eligible sample to have no generated
                       errors and exact S-expression/deep parity. TypeScript
                       enables this automatically.
  --unsafe-fortran-defaults
                       Disable the default bounded Fortran preset. By default,
                       Fortran runs that do not set explicit resource controls
                       use memory=3g, cpus=1, pids=512, GOMAXPROCS=1,
                       GOFLAGS=-p=1, lr0_core_budget=160000000, and
                       generate_timeout=15m.
  --jobs <n>           Concurrent per-grammar containers (default: 1).
                       Each grammar still runs in its own memory/pid-limited
                       container. Aggregate container memory is guarded
                       against host MemAvailable by default.
  --allow-host-oversubscribe
                       Allow --jobs * --memory to exceed the host memory
                       guard. Intended only for dedicated CI hosts.
  --no-build           Skip Docker image build
  -h, --help           Show this help

Output:
  Per-grammar logs saved to <report-dir>/diag_<grammar>.log
  Summary line printed to stdout for each grammar.
USAGE
}

MODE=""
TARGET_GRAMMAR=""
LANGS_CSV=""

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
    --langs)
      if [[ -n "$MODE" ]]; then
        echo "cannot combine --langs with grammar name/--all/--failing/--list" >&2
        exit 2
      fi
      MODE="langs"
      LANGS_CSV="$2"
      shift 2
      ;;
    --memory)
      MEMORY_LIMIT="$2"
      MEMORY_SET=1
      shift 2
      ;;
    --cpus)
      CPUS_LIMIT="$2"
      CPUS_SET=1
      shift 2
      ;;
    --pids)
      PIDS_LIMIT="$2"
      PIDS_SET=1
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
    --gomaxprocs)
      GOMAXPROCS_VALUE="$2"
      GOMAXPROCS_SET=1
      shift 2
      ;;
    --goflags)
      GOFLAGS_VALUE="$2"
      GOFLAGS_SET=1
      shift 2
      ;;
    --lr0-core-budget)
      LR0_CORE_BUDGET="$2"
      LR0_CORE_BUDGET_SET=1
      shift 2
      ;;
    --generate-timeout)
      GENERATE_TIMEOUT="$2"
      GENERATE_TIMEOUT_SET=1
      shift 2
      ;;
    --require-parity)
      REQUIRE_PARITY=1
      shift
      ;;
    --unsafe-fortran-defaults)
      FORTRAN_SAFE_DEFAULTS=0
      shift
      ;;
    --jobs)
      JOBS="$2"
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
    -*)
      echo "unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -n "$MODE" ]]; then
        echo "cannot combine grammar name with --langs/--all/--failing/--list" >&2
        exit 2
      fi
      MODE="single"
      TARGET_GRAMMAR="$1"
      shift
      ;;
  esac
done

if [[ -z "$MODE" ]]; then
  echo "error: specify a grammar name, --langs, --all, --failing, or --list" >&2
  usage >&2
  exit 2
fi

if [[ "$MODE" == "list" ]]; then
  printf '%s\n' "${ALL_GRAMMARS[@]}"
  exit 0
fi

canonical_grammar() {
  local grammar
  grammar="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')"
  grammar="${grammar//[[:space:]]/}"
  case "$grammar" in
    c) echo "c_lang" ;;
    go) echo "go_lang" ;;
    c++|cplusplus) echo "cpp" ;;
    c#|csharp) echo "c_sharp" ;;
    js) echo "javascript" ;;
    ts) echo "typescript" ;;
    *) echo "$grammar" ;;
  esac
}

is_known_grammar() {
  local grammar="$1"
  local known
  for known in "${ALL_GRAMMARS[@]}"; do
    if [[ "$known" == "$grammar" ]]; then
      return 0
    fi
  done
  return 1
}

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

require_positive_int() {
  local name="$1"
  local value="$2"
  if ! [[ "$value" =~ ^[1-9][0-9]*$ ]]; then
    echo "$name must be a positive integer, got: $value" >&2
    exit 2
  fi
}

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

resolve_fortran_setting() {
  local grammar="$1"
  local current="$2"
  local was_set="$3"
  local safe_default="$4"

  if [[ "$grammar" == "fortran" && "$FORTRAN_SAFE_DEFAULTS" == "1" && "$was_set" == "0" ]]; then
    echo "$safe_default"
    return
  fi

  echo "$current"
}

effective_grammar_memory_limit() {
  local grammar="$1"
  resolve_fortran_setting "$grammar" "$MEMORY_LIMIT" "$MEMORY_SET" "$FORTRAN_SAFE_MEMORY_LIMIT"
}

guard_parallel_memory_budget() {
  local -a targets=("$@")
  local target_count="${#targets[@]}"
  local effective_jobs="$JOBS"
  if [[ "$target_count" =~ ^[1-9][0-9]*$ && "$effective_jobs" -gt "$target_count" ]]; then
    effective_jobs="$target_count"
  fi
  if [[ "$effective_jobs" -le 1 || "$ALLOW_HOST_OVERSUBSCRIBE" == "1" ]]; then
    return 0
  fi
  local available_bytes aggregate_bytes guard_bytes
  available_bytes="$(host_mem_available_bytes || true)"
  if [[ -z "$available_bytes" ]]; then
    echo "warning: could not read host MemAvailable; proceeding with --jobs=$JOBS memory=$MEMORY_LIMIT" >&2
    return 0
  fi

  local grammar limit_bytes limit
  local -a ranked_limits=()
  for grammar in "${targets[@]}"; do
    limit="$(effective_grammar_memory_limit "$grammar")"
    limit_bytes="$(docker_memory_limit_to_bytes "$limit" || true)"
    if [[ -z "$limit_bytes" ]]; then
      echo "warning: could not parse memory limit for $grammar: $limit; proceeding with --jobs=$JOBS" >&2
      return 0
    fi
    ranked_limits+=("$limit_bytes|$grammar|$limit")
  done

  local count=0 entry detail grammar_name limit_text
  local -a selected_limits=()
  aggregate_bytes=0
  while IFS= read -r entry; do
    [[ -n "$entry" ]] || continue
    limit_bytes="${entry%%|*}"
    detail="${entry#*|}"
    grammar_name="${detail%%|*}"
    limit_text="${detail#*|}"
    aggregate_bytes="$((aggregate_bytes + limit_bytes))"
    selected_limits+=("$grammar_name=$limit_text")
    ((count+=1))
    if [[ "$count" -ge "$effective_jobs" ]]; then
      break
    fi
  done < <(printf '%s\n' "${ranked_limits[@]}" | sort -t '|' -k1,1nr)

  guard_bytes="$((available_bytes * 80 / 100))"
  if [[ "$aggregate_bytes" -gt "$guard_bytes" ]]; then
    {
      echo "refusing --jobs=$JOBS: aggregate effective container memory exceeds 80% of host MemAvailable"
      echo "effective_jobs=$effective_jobs"
      echo "aggregate_bytes=$aggregate_bytes memavailable_bytes=$available_bytes guard_bytes=$guard_bytes"
      echo "selected_limits=${selected_limits[*]}"
      echo "lower --jobs/--memory or pass --allow-host-oversubscribe on a dedicated host"
    } >&2
    exit 2
  fi
}

require_positive_int "--jobs" "$JOBS"

# Determine grammar list.
declare -a GRAMMARS
case "$MODE" in
  single)
    GRAMMARS=("$(canonical_grammar "$TARGET_GRAMMAR")")
    if ! is_known_grammar "${GRAMMARS[0]}"; then
      echo "unknown grammar: $TARGET_GRAMMAR" >&2
      exit 2
    fi
    ;;
  langs)
    declare -A seen_grammars=()
    IFS=',' read -r -a raw_grammars <<< "$LANGS_CSV"
    for raw_grammar in "${raw_grammars[@]}"; do
      grammar="$(canonical_grammar "$raw_grammar")"
      if [[ -z "$grammar" ]]; then
        continue
      fi
      if ! is_known_grammar "$grammar"; then
        echo "unknown grammar in --langs: $raw_grammar" >&2
        exit 2
      fi
      if [[ -n "${seen_grammars[$grammar]:-}" ]]; then
        continue
      fi
      seen_grammars[$grammar]=1
      GRAMMARS+=("$grammar")
    done
    if [[ "${#GRAMMARS[@]}" -eq 0 ]]; then
      echo "--langs selected no grammars" >&2
      exit 2
    fi
    ;;
  all)
    GRAMMARS=("${ALL_GRAMMARS[@]}")
    ;;
  failing)
    GRAMMARS=("${FAILING_GRAMMARS[@]}")
    ;;
esac

guard_parallel_memory_budget "${GRAMMARS[@]}"

# Build image once.
if [[ "$BUILD_IMAGE" == "1" ]]; then
  echo "Building Docker image..."
  docker build -t "$IMAGE_TAG" "$SCRIPT_DIR"
  echo ""
fi

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
    [cuda]="https://github.com/theHamsta/tree-sitter-cuda.git"
    [typescript]="https://github.com/tree-sitter/tree-sitter-typescript.git"
    [tsx]="https://github.com/tree-sitter/tree-sitter-typescript.git"
    [cobol]="https://github.com/yutaro-sakamoto/tree-sitter-cobol.git"
    [fortran]="https://github.com/stadelmanma/tree-sitter-fortran.git"
    [perl]="https://github.com/tree-sitter-perl/tree-sitter-perl.git"
    [erlang]="https://github.com/WhatsApp/tree-sitter-erlang.git"
    [d]="https://github.com/CyberShadow/tree-sitter-d.git"
    [dart]="https://github.com/UserNobody14/tree-sitter-dart.git"
  )

  # Map grammar names to repo directory names (some differ).
  declare -A REPO_NAMES=(
    [c_lang]="c"
    [go_lang]="go"
    [gitcommit]="gitcommit_gbprod"
    [c_sharp]="c_sharp"
    [typescript]="typescript"
    [tsx]="typescript"
  )

  # Map runner names to languages.lock names.
  declare -A LOCK_NAMES=(
    [c_lang]="c"
    [go_lang]="go"
  )

  local repo_name="${REPO_NAMES[$grammar]:-$grammar}"
  local lock_name="${LOCK_NAMES[$grammar]:-$grammar}"
  local url="${REPO_URLS[$grammar]:-}"

  cat <<CLONE_EOF
lock_file="/workspace/grammars/languages.lock"
lock_url=\$(awk -v target="$lock_name" '\$1 == target && \$1 !~ /^#/ { print \$2; exit }' "\$lock_file")
lock_commit=\$(awk -v target="$lock_name" '\$1 == target && \$1 !~ /^#/ { print \$3; exit }' "\$lock_file")
if [[ -n "\$lock_url" && -n "\$lock_commit" ]]; then
  if [[ -d "/tmp/grammar_parity/$repo_name/.git" ]]; then
    git -C "/tmp/grammar_parity/$repo_name" remote set-url origin "\$lock_url" >/dev/null 2>&1 || true
  else
    rm -rf "/tmp/grammar_parity/$repo_name"
    git clone --depth=1 "\$lock_url" "/tmp/grammar_parity/$repo_name" || echo "WARN: clone failed for $grammar"
  fi
  if [[ -d "/tmp/grammar_parity/$repo_name/.git" ]]; then
    git config --global --add safe.directory "/tmp/grammar_parity/$repo_name" >/dev/null 2>&1 || true
    git -C "/tmp/grammar_parity/$repo_name" fetch --depth=1 origin "\$lock_commit" >/dev/null 2>&1 || true
    git -C "/tmp/grammar_parity/$repo_name" checkout --detach "\$lock_commit" >/dev/null 2>&1 || echo "WARN: checkout failed for $grammar @ \$lock_commit"
  fi
elif [[ -n "$url" && ! -d "/tmp/grammar_parity/$repo_name" ]]; then
  git clone --depth=1 "$url" "/tmp/grammar_parity/$repo_name" || echo "WARN: clone failed for $grammar"
else
  echo "WARN: no clone URL for $grammar"
fi
CLONE_EOF
}

docker_memory_limit_to_gomemlimit() {
  local raw="$1"
  local lower

  raw="${raw//[[:space:]]/}"
  lower="$(printf '%s' "$raw" | tr '[:upper:]' '[:lower:]')"

  if [[ "$lower" =~ ^([0-9]+)$ ]]; then
    echo "${BASH_REMATCH[1]}B"
    return 0
  fi
  if [[ "$lower" =~ ^([0-9]+)b$ ]]; then
    echo "${BASH_REMATCH[1]}B"
    return 0
  fi
  if [[ "$lower" =~ ^([0-9]+)(k|kb|ki|kib)$ ]]; then
    echo "${BASH_REMATCH[1]}KiB"
    return 0
  fi
  if [[ "$lower" =~ ^([0-9]+)(m|mb|mi|mib)$ ]]; then
    echo "${BASH_REMATCH[1]}MiB"
    return 0
  fi
  if [[ "$lower" =~ ^([0-9]+)(g|gb|gi|gib)$ ]]; then
    echo "${BASH_REMATCH[1]}GiB"
    return 0
  fi

  return 1
}

run_grammar() {
  local grammar="$1"
  local log_file="$REPORT_DIR/diag_${grammar}.log"
  local effective_memory_limit
  local effective_cpus_limit
  local effective_pids_limit
  local effective_gomaxprocs_value
  local effective_goflags_value
  local effective_lr0_core_budget
  local effective_generate_timeout
  local effective_gomemlimit=""
  local require_parity="$REQUIRE_PARITY"

  effective_memory_limit="$(resolve_fortran_setting "$grammar" "$MEMORY_LIMIT" "$MEMORY_SET" "$FORTRAN_SAFE_MEMORY_LIMIT")"
  effective_cpus_limit="$(resolve_fortran_setting "$grammar" "$CPUS_LIMIT" "$CPUS_SET" "$FORTRAN_SAFE_CPUS_LIMIT")"
  effective_pids_limit="$(resolve_fortran_setting "$grammar" "$PIDS_LIMIT" "$PIDS_SET" "$FORTRAN_SAFE_PIDS_LIMIT")"
  effective_gomaxprocs_value="$(resolve_fortran_setting "$grammar" "$GOMAXPROCS_VALUE" "$GOMAXPROCS_SET" "$FORTRAN_SAFE_GOMAXPROCS_VALUE")"
  effective_goflags_value="$(resolve_fortran_setting "$grammar" "$GOFLAGS_VALUE" "$GOFLAGS_SET" "$FORTRAN_SAFE_GOFLAGS_VALUE")"
  effective_lr0_core_budget="$(resolve_fortran_setting "$grammar" "$LR0_CORE_BUDGET" "$LR0_CORE_BUDGET_SET" "$FORTRAN_SAFE_LR0_CORE_BUDGET")"
  effective_generate_timeout="$(resolve_fortran_setting "$grammar" "$GENERATE_TIMEOUT" "$GENERATE_TIMEOUT_SET" "$FORTRAN_SAFE_GENERATE_TIMEOUT")"
  effective_gomemlimit="$(docker_memory_limit_to_gomemlimit "$effective_memory_limit" || true)"
  case "$grammar" in
    typescript)
      require_parity=1
      ;;
  esac

  echo "=== Testing: $grammar (memory=$effective_memory_limit cpus=$effective_cpus_limit pids=$effective_pids_limit timeout=$TIMEOUT_PER_GRAMMAR generate_timeout=${effective_generate_timeout:-inherit} gomaxprocs=${effective_gomaxprocs_value:-inherit} goflags=${effective_goflags_value:-inherit} lr0_core_budget=${effective_lr0_core_budget:-inherit} gomemlimit=${effective_gomemlimit:-inherit} require_parity=$require_parity) ==="

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

  local source_overlay_block=""
  case "$grammar" in
    typescript|tsx)
      source_overlay_block="
typescript_repo=/tmp/grammar_parity/typescript
typescript_patch=/workspace/grammars/patches/tree-sitter-typescript-import-type.patch
if ! git -C \"\$typescript_repo\" apply --reverse --check \"\$typescript_patch\" >/dev/null 2>&1; then
  git -C \"\$typescript_repo\" apply --check \"\$typescript_patch\"
  git -C \"\$typescript_repo\" apply \"\$typescript_patch\"
fi
(
  cd \"\$typescript_repo\"
  npm ci
  cd \"$grammar\"
  ../node_modules/.bin/tree-sitter generate
)
"
      ;;
  esac

  local lr0_core_budget_env=""
  if [[ -n "$effective_lr0_core_budget" ]]; then
    lr0_core_budget_env="GOT_LALR_LR0_CORE_BUDGET=$effective_lr0_core_budget"
  fi
  local parser_stack_env=""
  case "$grammar" in
    typescript|tsx)
      parser_stack_env="GOT_GLR_MAX_STACKS=64"
      ;;
  esac
  local generate_timeout_env=""
  if [[ -n "$effective_generate_timeout" ]]; then
    generate_timeout_env="GTS_GRAMMARGEN_REAL_CORPUS_GENERATE_TIMEOUT=$effective_generate_timeout"
  fi

  local inner_cmd
  read -r -d '' inner_cmd <<INNER_EOF || true
set -eo pipefail
export PATH=/usr/local/go/bin:\$PATH
if [[ -n "$effective_gomemlimit" ]]; then
  export GOMEMLIMIT="$effective_gomemlimit"
fi
if [[ -n "$effective_gomaxprocs_value" ]]; then
  export GOMAXPROCS="$effective_gomaxprocs_value"
fi
if [[ -n "$effective_goflags_value" ]]; then
  export GOFLAGS="$effective_goflags_value"
fi
mkdir -p /tmp/grammar_parity
$seed_block
$clone_block
$source_overlay_block

echo '{}' > /tmp/real_corpus_parity_floors.json
cd /workspace
/usr/bin/time -v env \
  GTS_GRAMMARGEN_REAL_CORPUS_ENABLE=1 \
  GTS_GRAMMARGEN_REAL_CORPUS_ROOT=/tmp/grammar_parity \
  GTS_GRAMMARGEN_REAL_CORPUS_PROFILE=$PROFILE \
  GTS_GRAMMARGEN_REAL_CORPUS_MAX_CASES=$MAX_CASES \
  GTS_GRAMMARGEN_REAL_CORPUS_REQUIRE_PARITY=$require_parity \
  GTS_GRAMMARGEN_REAL_CORPUS_ALLOW_PARTIAL=1 \
  GTS_GRAMMARGEN_REAL_CORPUS_FLOORS_PATH=/tmp/real_corpus_parity_floors.json \
  GTS_GRAMMARGEN_REAL_CORPUS_ONLY=$grammar \
  $lr0_core_budget_env \
  $parser_stack_env \
  $generate_timeout_env \
  $lr_split_env \
  go test ./grammargen -run '^TestMultiGrammarImportRealCorpusParity\$' -count=1 -v -timeout $TIMEOUT_PER_GRAMMAR
INNER_EOF

  local exit_code=0
  "$RUNNER" \
    --image "$IMAGE_TAG" \
    --repo-root "$REPO_ROOT" \
    --memory "$effective_memory_limit" \
    --cpus "$effective_cpus_limit" \
    --pids "$effective_pids_limit" \
    --label "diag-${grammar}" \
    --no-build \
    -- "$inner_cmd" 2>&1 | tee "$log_file" || exit_code=$?

  # Extract summary line from log.
  local summary
  summary=$(grep -E 'real-corpus\[' "$log_file" 2>/dev/null | tail -1 || echo "NO SUMMARY")
  local is_oom="false"
  local parity_status="fail"
  if grep -q '^oom_killed: true$' "$log_file" 2>/dev/null; then
    is_oom="true"
  elif [[ "$summary" =~ no-error[[:space:]]+([0-9]+)/([0-9]+),[[:space:]]+sexpr[[:space:]]+parity[[:space:]]+([0-9]+)/([0-9]+),[[:space:]]+deep[[:space:]]+parity[[:space:]]+([0-9]+)/([0-9]+) ]]; then
    if [[ "${BASH_REMATCH[1]}" == "${BASH_REMATCH[2]}" &&
          "${BASH_REMATCH[3]}" == "${BASH_REMATCH[4]}" &&
          "${BASH_REMATCH[5]}" == "${BASH_REMATCH[6]}" ]]; then
      parity_status="ok"
    fi
  fi

  if [[ "$is_oom" == "true" ]]; then
    echo "RESULT: $grammar — OOM KILLED"
  elif [[ "$exit_code" != "0" ]]; then
    echo "RESULT: $grammar — FAILED (exit=$exit_code) | $summary"
  elif [[ "$parity_status" == "ok" ]]; then
    echo "RESULT: $grammar — PARITY | $summary"
  else
    echo "RESULT: $grammar — MISMATCH | $summary"
  fi
  echo ""

  return 0  # Always continue to next grammar.
}

# Run grammars.
total=${#GRAMMARS[@]}
echo "Running $total grammar(s) with per-grammar Docker isolation"
echo "Memory: $MEMORY_LIMIT | Timeout: $TIMEOUT_PER_GRAMMAR | Profile: $PROFILE | Cases: $MAX_CASES"
echo "Require parity: $REQUIRE_PARITY (typescript is always strict)"
echo "Fortran bounded preset: $FORTRAN_SAFE_DEFAULTS"
echo "Jobs: $JOBS | Allow host oversubscribe: $ALLOW_HOST_OVERSUBSCRIBE"
echo "Reports: $REPORT_DIR"
echo ""

passed=0
failed=0
oom=0

record_grammar_result() {
  local grammar="$1"
  local log
  log="$REPORT_DIR/diag_${grammar}.log"
  if grep -q '^oom_killed: true$' "$log" 2>/dev/null; then
    ((oom++)) || true
  elif grep -q '^exit_code: 0$' "$log" 2>/dev/null &&
       grep -Eq 'real-corpus\[.*no-error[[:space:]]+([0-9]+)/\1,[[:space:]]+sexpr[[:space:]]+parity[[:space:]]+([0-9]+)/\2,[[:space:]]+deep[[:space:]]+parity[[:space:]]+([0-9]+)/\3' "$log" 2>/dev/null; then
    ((passed++)) || true
  else
    ((failed++)) || true
  fi
}

if [[ "$JOBS" -eq 1 || "$total" -eq 1 ]]; then
  for grammar in "${GRAMMARS[@]}"; do
    run_grammar "$grammar" || true
    record_grammar_result "$grammar"
  done
else
  declare -a pids=()
  declare -a pid_grammars=()

  wait_for_one() {
    local finished_pid grammar idx rc
    if [[ "${#pids[@]}" -eq 0 ]]; then
      return 0
    fi
    if wait -n -p finished_pid "${pids[@]}"; then
      rc=0
    else
      rc=$?
    fi
    grammar="$finished_pid"
    for idx in "${!pids[@]}"; do
      if [[ "${pids[$idx]}" == "$finished_pid" ]]; then
        grammar="${pid_grammars[$idx]}"
        unset 'pids[idx]'
        unset 'pid_grammars[idx]'
        pids=("${pids[@]}")
        pid_grammars=("${pid_grammars[@]}")
        break
      fi
    done
    echo "[done] grammar parity: $grammar exit=$rc"
  }

  for grammar in "${GRAMMARS[@]}"; do
    while [[ "${#pids[@]}" -ge "$JOBS" ]]; do
      wait_for_one
    done
    echo "[start] grammar parity: $grammar"
    run_grammar "$grammar" &
    pids+=("$!")
    pid_grammars+=("$grammar")
  done

  while [[ "${#pids[@]}" -gt 0 ]]; do
    wait_for_one
  done

  for grammar in "${GRAMMARS[@]}"; do
    record_grammar_result "$grammar"
  done
fi

echo "========================================="
echo "SUMMARY: $passed passed, $failed failed, $oom OOM out of $total grammars"
echo "Reports saved to: $REPORT_DIR"

if (( failed > 0 || oom > 0 )); then
  exit 1
fi
