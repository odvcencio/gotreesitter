#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RUNNER="$SCRIPT_DIR/run_parity_in_docker.sh"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

IMAGE_TAG="gotreesitter/cgo-harness:go1.24-local"
MEMORY_LIMIT="12g"
CPUS_LIMIT="4"
PIDS_LIMIT="4096"
OUT_ROOT=""
LABEL="grammargen-real-corpus"
PROFILE="aggressive"
MAX_CASES="25"
MAX_GRAMMARS="0"
SEED_DIR=""
CONTAINER_SEED_DIR=""
OFFLINE=0
BUILD_IMAGE=1

usage() {
  cat <<'USAGE'
Usage: run_grammargen_real_corpus_in_docker.sh [options]

Run grammargen real-corpus parity in an isolated Docker container using
cgo_harness/docker/run_parity_in_docker.sh as the execution harness.

Options:
  --repo-root <path>      Repository/worktree root mounted at /workspace
  --image <tag>           Docker image tag (default: gotreesitter/cgo-harness:go1.24-local)
  --memory <limit>        Container memory limit (default: 8g)
  --cpus <count>          CPU limit (default: 4)
  --pids <count>          PID limit (default: 4096)
  --out-root <path>       Artifact output root (optional)
  --label <name>          Run label suffix (default: grammargen-real-corpus)
  --profile <name>        Real-corpus profile: smoke|balanced|aggressive (default: aggressive)
  --max-cases <n>         Max eligible samples per grammar (default: 25)
  --max-grammars <n>      Max grammars to exercise (0 = unlimited, default: 12)
  --seed-dir <path>       Host seed directory under repo root with grammar repos to copy into /tmp/grammar_parity
  --offline               Do not attempt network cloning; require --seed-dir
  --no-build              Skip docker image build
  -h, --help              Show this help

Notes:
  - The container seeds `/tmp/grammar_parity` from `--seed-dir` when provided.
  - Unless `--offline` is set, it also bootstraps a deterministic subset of
    grammar repos from upstream Git remotes.
  - It then runs:
      go test ./grammargen -run '^TestMultiGrammarImportRealCorpusParity$' -count=1 -v
  - This wrapper sets:
      GTS_GRAMMARGEN_REAL_CORPUS_ENABLE=1
      GTS_GRAMMARGEN_REAL_CORPUS_ALLOW_PARTIAL=1
      GTS_GRAMMARGEN_REAL_CORPUS_FLOORS_PATH=/tmp/real_corpus_parity_floors.json
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      REPO_ROOT="$2"
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
    --out-root)
      OUT_ROOT="$2"
      shift 2
      ;;
    --label)
      LABEL="$2"
      shift 2
      ;;
    --profile)
      PROFILE="$2"
      shift 2
      ;;
    --max-cases)
      MAX_CASES="$2"
      shift 2
      ;;
    --max-grammars)
      MAX_GRAMMARS="$2"
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
    --no-build)
      BUILD_IMAGE=0
      shift
      ;;
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

if [[ ! -x "$RUNNER" ]]; then
  echo "runner script is missing or not executable: $RUNNER" >&2
  exit 2
fi

REPO_ROOT="${REPO_ROOT/#\~/$HOME}"
if [[ ! -d "$REPO_ROOT" ]]; then
  echo "repo root does not exist: $REPO_ROOT" >&2
  exit 2
fi
REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"

case "$PROFILE" in
  smoke|balanced|aggressive)
    ;;
  *)
    echo "invalid --profile: $PROFILE (expected smoke|balanced|aggressive)" >&2
    exit 2
    ;;
esac

if ! [[ "$MAX_CASES" =~ ^[1-9][0-9]*$ ]]; then
  echo "invalid --max-cases: $MAX_CASES" >&2
  exit 2
fi

if [[ "$MAX_GRAMMARS" != "0" ]] && ! [[ "$MAX_GRAMMARS" =~ ^[1-9][0-9]*$ ]]; then
  echo "invalid --max-grammars: $MAX_GRAMMARS" >&2
  exit 2
fi

if [[ -n "$SEED_DIR" ]]; then
  SEED_DIR="${SEED_DIR/#\~/$HOME}"
  if [[ ! -d "$SEED_DIR" ]]; then
    echo "seed dir does not exist: $SEED_DIR" >&2
    exit 2
  fi
  SEED_DIR="$(cd "$SEED_DIR" && pwd)"
  if [[ "$SEED_DIR" == "$REPO_ROOT" ]]; then
    echo "seed dir must be a subdirectory under repo root, not the repo root itself" >&2
    exit 2
  fi
  case "$SEED_DIR" in
    "$REPO_ROOT"/*)
      CONTAINER_SEED_DIR="/workspace/${SEED_DIR#"$REPO_ROOT"/}"
      ;;
    *)
      echo "seed dir must be under repo root so it is visible inside the container: $SEED_DIR" >&2
      exit 2
      ;;
  esac
fi

if [[ "$OFFLINE" == "1" && -z "$CONTAINER_SEED_DIR" ]]; then
  echo "--offline requires --seed-dir under repo root" >&2
  exit 2
fi

read -r -d '' CUSTOM_CMD <<EOF2 || true
set -eo pipefail
export PATH=/usr/local/go/bin:\$PATH
mkdir -p /tmp/grammar_parity
echo '{}' > /tmp/real_corpus_parity_floors.json

SEED_DIR_IN_CONTAINER="$CONTAINER_SEED_DIR"
OFFLINE_MODE="$OFFLINE"

if [[ -n "\$SEED_DIR_IN_CONTAINER" && -d "\$SEED_DIR_IN_CONTAINER" ]]; then
  for src in "\$SEED_DIR_IN_CONTAINER"/*; do
    if [[ ! -d "\$src" ]]; then
      continue
    fi
    name="\$(basename "\$src")"
    rm -rf "/tmp/grammar_parity/\$name"
    cp -a "\$src" "/tmp/grammar_parity/\$name"
  done
fi

clone_repo() {
  local name="\$1"
  local url="\$2"
  local dest="/tmp/grammar_parity/\$name"
  local attempt
  for attempt in 1 2 3; do
    if [[ -d "\$dest/.git" ]]; then
      git -C "\$dest" fetch --depth=1 origin && git -C "\$dest" reset --hard FETCH_HEAD && return 0
    else
      rm -rf "\$dest"
      git clone --depth=1 "\$url" "\$dest" && return 0
    fi
    sleep 2
  done
  return 1
}

if [[ "\$OFFLINE_MODE" != "1" ]]; then
  # Deterministic subset with mature real-world corpora used by importParityGrammars.
  # Each clone_repo call is best-effort; failure of one repo doesn't block others.
  clone_repo json https://github.com/tree-sitter/tree-sitter-json.git || true
  clone_repo css https://github.com/tree-sitter/tree-sitter-css.git || true
  clone_repo html https://github.com/tree-sitter/tree-sitter-html.git || true
  clone_repo graphql https://github.com/bkegley/tree-sitter-graphql.git || true
  clone_repo toml https://github.com/tree-sitter/tree-sitter-toml.git || true
  clone_repo dockerfile https://github.com/camdencheek/tree-sitter-dockerfile.git || true
  clone_repo ini https://github.com/justinmk/tree-sitter-ini.git || true
  clone_repo properties https://github.com/tree-sitter-grammars/tree-sitter-properties.git || true
  clone_repo jsdoc https://github.com/tree-sitter/tree-sitter-jsdoc.git || true
  clone_repo csv https://github.com/amaanq/tree-sitter-csv.git || true
  clone_repo json5 https://github.com/Joakker/tree-sitter-json5.git || true
  clone_repo diff https://github.com/the-mikedavis/tree-sitter-diff.git || true
  clone_repo dot https://github.com/rydesun/tree-sitter-dot.git || true
  clone_repo ron https://github.com/amaanq/tree-sitter-ron.git || true
  clone_repo proto https://github.com/treywood/tree-sitter-proto.git || true
  clone_repo comment https://github.com/stsewd/tree-sitter-comment.git || true
  clone_repo regex https://github.com/tree-sitter/tree-sitter-regex.git || true
  clone_repo nix https://github.com/nix-community/tree-sitter-nix.git || true
  clone_repo jq https://github.com/flurie/tree-sitter-jq.git || true
  clone_repo hcl https://github.com/tree-sitter-grammars/tree-sitter-hcl.git || true
  clone_repo scheme https://github.com/6cdh/tree-sitter-scheme.git || true
  clone_repo forth https://github.com/AlexanderBrevig/tree-sitter-forth.git || true
  clone_repo corn https://github.com/jakestanger/tree-sitter-corn.git || true
  clone_repo cpon https://github.com/psvz/tree-sitter-cpon.git || true
  clone_repo textproto https://github.com/PorterAtGoogle/tree-sitter-textproto.git || true
  clone_repo promql https://github.com/MichaHoffmann/tree-sitter-promql.git || true
  clone_repo gitignore https://github.com/shuber/tree-sitter-gitignore.git || true
  clone_repo eds https://github.com/uyha/tree-sitter-eds.git || true
  clone_repo go https://github.com/tree-sitter/tree-sitter-go.git || true
  clone_repo c https://github.com/tree-sitter/tree-sitter-c.git || true
  clone_repo sql https://github.com/m-novikov/tree-sitter-sql.git || true
  clone_repo make https://github.com/alemuller/tree-sitter-make.git || true
  clone_repo javascript https://github.com/tree-sitter/tree-sitter-javascript.git || true
  clone_repo python https://github.com/tree-sitter/tree-sitter-python.git || true
  clone_repo ruby https://github.com/tree-sitter/tree-sitter-ruby.git || true
  clone_repo rust https://github.com/tree-sitter/tree-sitter-rust.git || true
  clone_repo bash https://github.com/tree-sitter/tree-sitter-bash.git || true
  clone_repo java https://github.com/tree-sitter/tree-sitter-java.git || true
  clone_repo lua https://github.com/tree-sitter-grammars/tree-sitter-lua.git || true
  clone_repo kotlin https://github.com/fwcd/tree-sitter-kotlin.git || true
  clone_repo php https://github.com/tree-sitter/tree-sitter-php.git || true
  clone_repo elixir https://github.com/elixir-lang/tree-sitter-elixir.git || true
  clone_repo c_sharp https://github.com/tree-sitter/tree-sitter-c-sharp.git || true
  clone_repo ocaml https://github.com/tree-sitter/tree-sitter-ocaml.git || true
  clone_repo dart https://github.com/UserNobworthy/tree-sitter-dart.git || true
  clone_repo scala https://github.com/tree-sitter/tree-sitter-scala.git || true
  clone_repo swift https://github.com/tree-sitter/tree-sitter-swift.git || true
  clone_repo haskell https://github.com/tree-sitter/tree-sitter-haskell.git || true
  clone_repo yaml https://github.com/tree-sitter-grammars/tree-sitter-yaml.git || true
  clone_repo markdown https://github.com/tree-sitter-grammars/tree-sitter-markdown.git || true

  # Batch 4: no-external-scanner grammars with corpus test data
  clone_repo requirements https://github.com/tree-sitter-grammars/tree-sitter-requirements.git || true
  clone_repo gitcommit_gbprod https://github.com/gbprod/tree-sitter-gitcommit.git || true
  clone_repo git_rebase https://github.com/the-mikedavis/tree-sitter-git-rebase.git || true
  clone_repo gitattributes https://github.com/tree-sitter-grammars/tree-sitter-gitattributes.git || true
  clone_repo git_config https://github.com/the-mikedavis/tree-sitter-git-config.git || true
  clone_repo ssh_config https://github.com/tree-sitter-grammars/tree-sitter-ssh-config.git || true
  clone_repo todotxt https://github.com/arnarg/tree-sitter-todotxt.git || true
  clone_repo pem https://github.com/ObserverOfTime/tree-sitter-pem.git || true
  clone_repo gomod https://github.com/camdencheek/tree-sitter-go-mod.git || true
  clone_repo eex https://github.com/connorlay/tree-sitter-eex.git || true
  clone_repo cpon https://github.com/amaanq/tree-sitter-cpon.git || true
fi

if ! find /tmp/grammar_parity -mindepth 1 -maxdepth 1 -type d | grep -q .; then
  echo "no grammar repos available under /tmp/grammar_parity after seed/bootstrap" >&2
  exit 2
fi

cd /workspace
/usr/bin/time -v env \
  GTS_GRAMMARGEN_REAL_CORPUS_ENABLE=1 \
  GTS_GRAMMARGEN_REAL_CORPUS_ROOT=/tmp/grammar_parity \
  GTS_GRAMMARGEN_REAL_CORPUS_PROFILE=$PROFILE \
  GTS_GRAMMARGEN_REAL_CORPUS_MAX_CASES=$MAX_CASES \
  GTS_GRAMMARGEN_REAL_CORPUS_MAX_GRAMMARS=$MAX_GRAMMARS \
  GTS_GRAMMARGEN_REAL_CORPUS_ALLOW_PARTIAL=1 \
  GTS_GRAMMARGEN_REAL_CORPUS_FLOORS_PATH=/tmp/real_corpus_parity_floors.json \
  GTS_GRAMMARGEN_REAL_CORPUS_SKIP=rust,c_sharp,java,ruby,cpp,kotlin,css,scala,go_lang,c_lang,python,javascript \
  go test ./grammargen -run '^TestMultiGrammarImportRealCorpusParity$' -count=1 -v
EOF2

CMD=(
  "$RUNNER"
  --image "$IMAGE_TAG"
  --repo-root "$REPO_ROOT"
  --memory "$MEMORY_LIMIT"
  --cpus "$CPUS_LIMIT"
  --pids "$PIDS_LIMIT"
  --label "$LABEL"
)
if [[ "$BUILD_IMAGE" == "0" ]]; then
  CMD+=(--no-build)
fi
if [[ -n "$OUT_ROOT" ]]; then
  CMD+=(--out-root "$OUT_ROOT")
fi
CMD+=(-- "$CUSTOM_CMD")

"${CMD[@]}"
