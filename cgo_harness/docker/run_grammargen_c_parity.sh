#!/usr/bin/env bash
#
# Run grammargen-vs-C parity tests inside a Docker container with bounded
# memory and CPU to prevent OOM blowups from large grammar generation or GLR
# stack explosions.
#
# Usage:
#   cgo_harness/docker/run_grammargen_c_parity.sh [OPTIONS]
#
# Options:
#   --memory MEM       Container memory limit (default: 8g)
#   --cpus N           CPU limit (default: 4)
#   --max-cases N      Max corpus samples per grammar (default: 20)
#   --max-bytes N      Max sample size in bytes (default: 262144)
#   --langs LANGS      Comma-separated language filter (default: all)
#   --ratchet-update   Write ratchet floor file after run
#   --label LABEL      Label for output directory
#   --timeout MINS     Test timeout in minutes (default: 45)
#   --src-dir DIR      Source directory (default: repo root)
#   --seed-dir DIR     Optional host seed dir copied into /grammar_parity
#   --offline          Do not clone missing grammar repos inside the container
#   --no-build         Skip docker image build
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Defaults.
MEMORY="8g"
CPUS="4"
MAX_CASES="20"
MAX_BYTES="262144"
LANGS=""
RATCHET_UPDATE=""
LABEL=""
TIMEOUT_MINS="45"
SRC_DIR="$REPO_ROOT"
SEED_DIR=""
OFFLINE=0
BUILD_IMAGE=1

while [[ $# -gt 0 ]]; do
    case "$1" in
        --memory)      MEMORY="$2"; shift 2 ;;
        --cpus)        CPUS="$2"; shift 2 ;;
        --max-cases)   MAX_CASES="$2"; shift 2 ;;
        --max-bytes)   MAX_BYTES="$2"; shift 2 ;;
        --langs)       LANGS="$2"; shift 2 ;;
        --ratchet-update) RATCHET_UPDATE="1"; shift ;;
        --label)       LABEL="$2"; shift 2 ;;
        --timeout)     TIMEOUT_MINS="$2"; shift 2 ;;
        --src-dir)     SRC_DIR="$2"; shift 2 ;;
        --seed-dir)    SEED_DIR="$2"; shift 2 ;;
        --offline)     OFFLINE=1; shift ;;
        --no-build)    BUILD_IMAGE=0; shift ;;
        *) echo "Unknown option: $1" >&2; exit 1 ;;
    esac
done

IMAGE_TAG="gotreesitter-grammargen-cparity:latest"
TIMESTAMP="$(date +%Y%m%d_%H%M%S)"
OUT_DIR="$REPO_ROOT/harness_out/grammargen_cparity/${TIMESTAMP}"
if [[ -n "$LABEL" ]]; then
    OUT_DIR="${OUT_DIR}-${LABEL}"
fi
mkdir -p "$OUT_DIR"

echo "=== grammargen C parity test ==="
echo "  memory:     $MEMORY"
echo "  cpus:       $CPUS"
echo "  max_cases:  $MAX_CASES"
echo "  max_bytes:  $MAX_BYTES"
echo "  langs:      ${LANGS:-all}"
echo "  ratchet:    ${RATCHET_UPDATE:-no}"
echo "  timeout:    ${TIMEOUT_MINS}m"
echo "  offline:    $OFFLINE"
echo "  seed dir:   ${SEED_DIR:-none}"
echo "  output:     $OUT_DIR"
echo ""

# Build docker image.
if [[ "$BUILD_IMAGE" == "1" ]]; then
    echo "--- Building Docker image ---"
    docker build -t "$IMAGE_TAG" -f "$SCRIPT_DIR/Dockerfile" "$SCRIPT_DIR" 2>&1 | tail -5
fi

SRC_DIR="${SRC_DIR/#\~/$HOME}"
SRC_DIR="$(cd "$SRC_DIR" && pwd)"

SEED_MOUNT_ARGS=()
if [[ -n "$SEED_DIR" ]]; then
    SEED_DIR="${SEED_DIR/#\~/$HOME}"
    if [[ ! -d "$SEED_DIR" ]]; then
        echo "seed dir does not exist: $SEED_DIR" >&2
        exit 1
    fi
    SEED_DIR="$(cd "$SEED_DIR" && pwd)"
    SEED_MOUNT_ARGS=(-v "$SEED_DIR:/seed:ro")
fi

# Build env vars for the container.
# When ratchet update is requested, redirect floors to /out so it persists
# (the source tree is mounted read-only).
FLOORS_PATH="/src/cgo_harness/testdata/grammargen_cgo_parity_floors.json"
if [[ -n "$RATCHET_UPDATE" ]]; then
    FLOORS_PATH="/out/grammargen_cgo_parity_floors.json"
    # Seed from existing floors if available.
    if [[ -f "$SRC_DIR/cgo_harness/testdata/grammargen_cgo_parity_floors.json" ]]; then
        cp "$SRC_DIR/cgo_harness/testdata/grammargen_cgo_parity_floors.json" "$OUT_DIR/grammargen_cgo_parity_floors.json"
    fi
fi

ENV_ARGS=(
    -e "GTS_GRAMMARGEN_CGO_ENABLE=1"
    -e "GTS_GRAMMARGEN_CGO_ROOT=/tmp/grammar_parity"
    -e "GTS_PARITY_REPO_ROOT=/tmp/grammar_parity"
    -e "GTS_GRAMMARGEN_CGO_MAX_CASES=$MAX_CASES"
    -e "GTS_GRAMMARGEN_CGO_MAX_BYTES=$MAX_BYTES"
    -e "GTS_GRAMMARGEN_CGO_FLOORS_PATH=$FLOORS_PATH"
)
if [[ -n "$LANGS" ]]; then
    ENV_ARGS+=(-e "GTS_GRAMMARGEN_CGO_LANGS=$LANGS")
fi
if [[ -n "$RATCHET_UPDATE" ]]; then
    ENV_ARGS+=(-e "GTS_GRAMMARGEN_CGO_RATCHET_UPDATE=1")
fi

read -r -d '' CONTAINER_CMD <<EOF || true
set -euo pipefail
export PATH=/usr/local/go/bin:\$PATH

LOCK_FILE="/src/grammars/languages.lock"
GRAMMAR_ROOT="/tmp/grammar_parity"
SEED_DIR_IN_CONTAINER="/seed"
OFFLINE_MODE="$OFFLINE"
LANG_FILTER="${LANGS}"

mkdir -p "\$GRAMMAR_ROOT"

copy_seed_dir() {
    if [[ ! -d "\$SEED_DIR_IN_CONTAINER" ]]; then
        return
    fi
    shopt -s nullglob
    for src in "\$SEED_DIR_IN_CONTAINER"/*; do
        if [[ ! -d "\$src" ]]; then
            continue
        fi
        local name
        name="\$(basename "\$src")"
        rm -rf "\$GRAMMAR_ROOT/\$name"
        cp -a "\$src" "\$GRAMMAR_ROOT/\$name"
        chown -R root:root "\$GRAMMAR_ROOT/\$name" 2>/dev/null || true
    done
    shopt -u nullglob
}

canonical_lock_name() {
    case "\$1" in
        gitcommit_gbprod) echo "gitcommit" ;;
        *) echo "\$1" ;;
    esac
}

root_dir_for_lang() {
    case "\$1" in
        gitcommit) echo "gitcommit_gbprod" ;;
        gitcommit_gbprod) echo "gitcommit_gbprod" ;;
        tsx|typescript) echo "typescript" ;;
        markdown|markdown_inline) echo "markdown" ;;
        xml|dtd) echo "xml" ;;
        php) echo "php" ;;
        ocaml) echo "ocaml" ;;
        csv) echo "csv" ;;
        *) echo "\$1" ;;
    esac
}

lock_repo_url() {
    local lock_name="\$1"
    awk -v target="\$lock_name" '\$1 == target && \$1 !~ /^#/ { print \$2; exit }' "\$LOCK_FILE"
}

lock_repo_commit() {
    local lock_name="\$1"
    awk -v target="\$lock_name" '\$1 == target && \$1 !~ /^#/ { print \$3; exit }' "\$LOCK_FILE"
}

clone_or_update_repo() {
    local lang_name="\$1"
    local lock_name root_dir url commit dest

    lock_name="\$(canonical_lock_name "\$lang_name")"
    root_dir="\$(root_dir_for_lang "\$lang_name")"
    url="\$(lock_repo_url "\$lock_name")"
    commit="\$(lock_repo_commit "\$lock_name")"
    dest="\$GRAMMAR_ROOT/\$root_dir"

    if [[ -z "\$url" || -z "\$commit" ]]; then
        echo "missing languages.lock entry for \$lock_name" >&2
        exit 2
    fi

    if [[ -d "\$dest/.git" ]]; then
        if [[ "\$OFFLINE_MODE" == "1" ]]; then
            return
        fi
        git config --global --add safe.directory "\$dest" >/dev/null 2>&1 || true
        if git -C "\$dest" cat-file -e "\$commit^{commit}" >/dev/null 2>&1; then
            git -C "\$dest" checkout --detach "\$commit" >/dev/null 2>&1
            return
        fi
        git -C "\$dest" fetch --depth=1 origin "\$commit" >/dev/null 2>&1
        git -C "\$dest" checkout --detach "\$commit" >/dev/null 2>&1
        return
    fi

    if [[ "\$OFFLINE_MODE" == "1" ]]; then
        echo "missing seeded repo for \$lang_name at \$dest and --offline is set" >&2
        exit 2
    fi

    rm -rf "\$dest"
    git clone --depth=1 "\$url" "\$dest" >/dev/null 2>&1
    git -C "\$dest" fetch --depth=1 origin "\$commit" >/dev/null 2>&1
    git -C "\$dest" checkout --detach "\$commit" >/dev/null 2>&1
}

default_langs=(
    json json5 css html graphql toml ini hcl nix sql make scala gomod go c
    csv diff gitcommit dot ron proto comment pem dockerfile gitattributes jq
    regex eds eex todotxt git_rebase gitignore git_config forth cpon scheme
    textproto promql jsdoc properties requirements ssh_config corn
)

declare -A seen_roots=()
required_langs=()
if [[ -n "\$LANG_FILTER" ]]; then
    IFS=',' read -r -a required_langs <<< "\$LANG_FILTER"
else
    required_langs=("\${default_langs[@]}")
fi

copy_seed_dir

for lang in "\${required_langs[@]}"; do
    lang="\${lang//[[:space:]]/}"
    if [[ -z "\$lang" ]]; then
        continue
    fi
    root_dir="\$(root_dir_for_lang "\$lang")"
    if [[ -n "\${seen_roots[\$root_dir]:-}" ]]; then
        continue
    fi
    seen_roots["\$root_dir"]=1
    clone_or_update_repo "\$lang"
done

cd /src/cgo_harness

echo "=== container start: \$(date -Iseconds) ===" | tee /out/container.log

go test . \
    -tags treesitter_c_parity \
    -run "^TestGrammargenCGOParity\$" \
    -count=1 \
    -v \
    -timeout ${TIMEOUT_MINS}m \
    2>&1 | tee -a /out/container.log

EXIT_CODE=\${PIPESTATUS[0]}

echo "" >> /out/container.log
echo "=== container end: \$(date -Iseconds) exit=\$EXIT_CODE ===" >> /out/container.log

if [[ -f /src/cgo_harness/testdata/grammargen_cgo_parity_floors.json ]]; then
    cp /src/cgo_harness/testdata/grammargen_cgo_parity_floors.json /out/floors_baseline.json 2>/dev/null || true
fi

exit \$EXIT_CODE
EOF

echo "--- Running tests in container ---"
set +e
docker run \
    --rm \
    --memory="$MEMORY" \
    --cpus="$CPUS" \
    --memory-swap="$MEMORY" \
    --oom-kill-disable=false \
    -v "$SRC_DIR:/src:ro" \
    -v "$OUT_DIR:/out" \
    --mount type=volume,src=gotreesitter-go-mod-cache,dst=/go/pkg/mod \
    --mount type=volume,src=gotreesitter-go-build-cache,dst=/root/.cache/go-build \
    "${SEED_MOUNT_ARGS[@]}" \
    "${ENV_ARGS[@]}" \
    "$IMAGE_TAG" \
    bash -c "$CONTAINER_CMD"
CONTAINER_EXIT=$?
set -e

# Save metadata.
cat > "$OUT_DIR/metadata.txt" <<EOF
timestamp: $TIMESTAMP
memory: $MEMORY
cpus: $CPUS
max_cases: $MAX_CASES
max_bytes: $MAX_BYTES
langs: ${LANGS:-all}
ratchet_update: ${RATCHET_UPDATE:-no}
timeout_mins: $TIMEOUT_MINS
offline: $OFFLINE
seed_dir: ${SEED_DIR:-none}
exit_code: $CONTAINER_EXIT
EOF

echo ""
echo "=== Done (exit=$CONTAINER_EXIT) ==="
echo "  Output: $OUT_DIR"
echo "  Log:    $OUT_DIR/container.log"

exit "$CONTAINER_EXIT"
