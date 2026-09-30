#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

readonly ORACLE_BINDING_MODULE="github.com/tree-sitter/go-tree-sitter"
readonly ORACLE_BINDING_VERSION="v0.25.0"
readonly ORACLE_BINDING_COMMIT="adc13ffd8b2c0b01b878fda9f7c422ce0df5fad3"
readonly ORACLE_RUNTIME_REPO="https://github.com/tree-sitter/tree-sitter.git"
readonly ORACLE_RUNTIME_COMMIT="6070dbfefd326bd735e5683eb128cc1b57dad0c0"
readonly ORACLE_GRAMMAR_REPO="https://github.com/tree-sitter/tree-sitter-go.git"
readonly ORACLE_GRAMMAR_COMMIT="2346a3ab1bb3857b48b29d779a1ef9799a248cd7"
readonly ORACLE_CFLAGS="-O2 -DNDEBUG -std=c11 -D_POSIX_C_SOURCE=200112L -D_DEFAULT_SOURCE"

FUNC_COUNT="${1:-500}"
FULL_ITERS="${2:-2000}"
INC_ITERS="${3:-20000}"
SOURCE_PATH="${4:-}"

CC_BIN="${CC:-cc}"
AR_BIN="${AR:-ar}"
CACHE_ROOT="${GTS_C_ORACLE_CACHE:-$REPO_ROOT/harness_out/c_oracle}"
SOURCE_ROOT="$CACHE_ROOT/sources"
ARTIFACT_ROOT="$CACHE_ROOT/artifacts"
RUNTIME_DIR="$SOURCE_ROOT/tree-sitter-$ORACLE_RUNTIME_COMMIT"
GRAMMAR_DIR="$SOURCE_ROOT/tree-sitter-go-$ORACLE_GRAMMAR_COMMIT"
RUN_TMP_DIR=""
SOURCE_SNAPSHOT=""

cleanup() {
  if [[ -n "$RUN_TMP_DIR" ]]; then
    rm -rf -- "$RUN_TMP_DIR"
  fi
}
trap cleanup EXIT

if [[ -n "$SOURCE_PATH" ]]; then
  if [[ ! -f "$SOURCE_PATH" ]]; then
    echo "source path is not a regular file: $SOURCE_PATH" >&2
    exit 2
  fi
  RUN_TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/gts-c-oracle-run.XXXXXX")"
  SOURCE_SNAPSHOT="$RUN_TMP_DIR/source.snapshot"
  cp -- "$SOURCE_PATH" "$SOURCE_SNAPSHOT"
  chmod 0444 "$SOURCE_SNAPSHOT"
fi

require_clean_checkout() {
  local label="$1"
  local dir="$2"
  local status ignored
  status="$(git -C "$dir" status --porcelain=v1 --untracked-files=all)"
  ignored="$(git -C "$dir" ls-files --others --ignored --exclude-standard)"
  if [[ -n "$status" || -n "$ignored" ]]; then
    echo "$label checkout is dirty or contains untracked/ignored files: $dir" >&2
    if [[ -n "$status" ]]; then
      printf '%s\n' "$status" >&2
    fi
    if [[ -n "$ignored" ]]; then
      printf '!! %s\n' "$ignored" >&2
    fi
    exit 2
  fi
}

ensure_checkout() {
  local label="$1"
  local repo_url="$2"
  local commit="$3"
  local dest="$4"
  local current=""
  if [[ -d "$dest/.git" ]]; then
    require_clean_checkout "$label" "$dest"
    current="$(git -C "$dest" rev-parse HEAD 2>/dev/null || true)"
  fi
  if [[ "$current" == "$commit" ]]; then
    return
  fi

  mkdir -p "$(dirname "$dest")"
  local tmp="${dest}.tmp.$$"
  rm -rf "$tmp"
  git clone --filter=blob:none --no-checkout "$repo_url" "$tmp"
  git -C "$tmp" checkout --detach "$commit"
  rm -rf "$dest"
  mv "$tmp" "$dest"
}

ensure_checkout "runtime" "$ORACLE_RUNTIME_REPO" "$ORACLE_RUNTIME_COMMIT" "$RUNTIME_DIR"
ensure_checkout "grammar" "$ORACLE_GRAMMAR_REPO" "$ORACLE_GRAMMAR_COMMIT" "$GRAMMAR_DIR"

runtime_head="$(git -C "$RUNTIME_DIR" rev-parse HEAD)"
grammar_head="$(git -C "$GRAMMAR_DIR" rev-parse HEAD)"
if [[ "$runtime_head" != "$ORACLE_RUNTIME_COMMIT" ]]; then
  echo "runtime checkout mismatch: got $runtime_head want $ORACLE_RUNTIME_COMMIT" >&2
  exit 2
fi
if [[ "$grammar_head" != "$ORACLE_GRAMMAR_COMMIT" ]]; then
  echo "grammar checkout mismatch: got $grammar_head want $ORACLE_GRAMMAR_COMMIT" >&2
  exit 2
fi
require_clean_checkout "runtime" "$RUNTIME_DIR"
require_clean_checkout "grammar" "$GRAMMAR_DIR"

compiler_path="$(command -v "$CC_BIN")"
compiler_version="$($CC_BIN --version | head -n 1)"
driver_sha="$(sha256sum "$SCRIPT_DIR/go_benchmark.c" | awk '{print $1}')"
runtime_lib_sha="$(sha256sum "$RUNTIME_DIR/lib/src/lib.c" | awk '{print $1}')"
grammar_parser_sha="$(sha256sum "$GRAMMAR_DIR/src/parser.c" | awk '{print $1}')"
runtime_lib_src_tree_oid="$(git -C "$RUNTIME_DIR" rev-parse "$runtime_head:lib/src")"
grammar_src_tree_oid="$(git -C "$GRAMMAR_DIR" rev-parse "$grammar_head:src")"
build_key="$({
  printf '%s\n' "$ORACLE_BINDING_COMMIT"
  printf '%s\n' "$ORACLE_RUNTIME_COMMIT"
  printf '%s\n' "$ORACLE_GRAMMAR_COMMIT"
  printf '%s\n' "$ORACLE_CFLAGS"
  printf '%s\n' "$compiler_path"
  printf '%s\n' "$compiler_version"
  printf '%s\n' "$driver_sha"
  printf '%s\n' "$runtime_lib_sha"
  printf '%s\n' "$grammar_parser_sha"
  printf '%s\n' "$runtime_lib_src_tree_oid"
  printf '%s\n' "$grammar_src_tree_oid"
} | sha256sum | awk '{print $1}')"

build_dir="$ARTIFACT_ROOT/${build_key:0:20}"
artifact="$build_dir/go_benchmark"
if [[ ! -x "$artifact" ]]; then
  tmp_build="${build_dir}.tmp.$$"
  rm -rf "$tmp_build"
  mkdir -p "$tmp_build"

  # Compile both the upstream runtime and the exact locked Go grammar at -O2,
  # archive them, then link those archives into one publication artifact.
  # This is static with respect to tree-sitter and the grammar; the platform C
  # library remains whatever the host toolchain normally links.
  # shellcheck disable=SC2206
  cflags=($ORACLE_CFLAGS)
  "$CC_BIN" "${cflags[@]}" \
    -I"$RUNTIME_DIR/lib/include" -I"$RUNTIME_DIR/lib/src" \
    -c "$RUNTIME_DIR/lib/src/lib.c" -o "$tmp_build/tree_sitter_runtime.o"
  "$AR_BIN" rcs "$tmp_build/libtree-sitter-oracle.a" "$tmp_build/tree_sitter_runtime.o"

  "$CC_BIN" "${cflags[@]}" -I"$GRAMMAR_DIR/src" \
    -c "$GRAMMAR_DIR/src/parser.c" -o "$tmp_build/tree_sitter_go.o"
  "$AR_BIN" rcs "$tmp_build/libtree-sitter-go-oracle.a" "$tmp_build/tree_sitter_go.o"

  "$CC_BIN" "${cflags[@]}" -I"$RUNTIME_DIR/lib/include/tree_sitter" \
    -c "$SCRIPT_DIR/go_benchmark.c" -o "$tmp_build/go_benchmark.o"
  "$CC_BIN" -O2 "$tmp_build/go_benchmark.o" \
    "$tmp_build/libtree-sitter-go-oracle.a" \
    "$tmp_build/libtree-sitter-oracle.a" \
    -o "$tmp_build/go_benchmark"

  mkdir -p "$ARTIFACT_ROOT"
  rm -rf "$build_dir"
  mv "$tmp_build" "$build_dir"
fi

if command -v nm >/dev/null 2>&1; then
  nm "$artifact" | grep -E '[[:space:]]T[[:space:]]ts_parser_parse$' >/dev/null || {
    echo "oracle artifact does not define ts_parser_parse" >&2
    exit 2
  }
  nm "$artifact" | grep -E '[[:space:]]T[[:space:]]tree_sitter_go$' >/dev/null || {
    echo "oracle artifact does not define tree_sitter_go" >&2
    exit 2
  }
fi
if command -v readelf >/dev/null 2>&1 && \
  readelf -d "$artifact" | grep -E 'NEEDED.*(tree-sitter|tree_sitter)' >/dev/null; then
  echo "oracle artifact unexpectedly depends on a shared tree-sitter library" >&2
  exit 2
fi

artifact_sha="$(sha256sum "$artifact" | awk '{print $1}')"
if [[ -n "$SOURCE_SNAPSHOT" ]]; then
  source_sha="$(sha256sum "$SOURCE_SNAPSHOT" | awk '{print $1}')"
  deep_sha="$("$artifact" --dump "$SOURCE_SNAPSHOT" | sha256sum | awk '{print $1}')"
else
  source_sha="synthetic_lr_control"
  deep_sha="$("$artifact" --dump-synthetic "$FUNC_COUNT" | sha256sum | awk '{print $1}')"
fi
if [[ -n "${GTS_C_ORACLE_EXPECTED_DEEP_SHA256:-}" && \
  "$deep_sha" != "$GTS_C_ORACLE_EXPECTED_DEEP_SHA256" ]]; then
  echo "oracle deep digest mismatch: got $deep_sha want $GTS_C_ORACLE_EXPECTED_DEEP_SHA256" >&2
  exit 2
fi

printf 'oracle_contract=tree-sitter-c-v1\n'
printf 'oracle_transport=static_executable\n'
printf 'oracle_linkage=runtime_and_grammar_statically_linked\n'
printf 'oracle_binding_module=%s\n' "$ORACLE_BINDING_MODULE"
printf 'oracle_binding_version=%s\n' "$ORACLE_BINDING_VERSION"
printf 'oracle_binding_commit=%s\n' "$ORACLE_BINDING_COMMIT"
printf 'oracle_runtime_commit=%s\n' "$ORACLE_RUNTIME_COMMIT"
printf 'oracle_grammar_commit=%s\n' "$ORACLE_GRAMMAR_COMMIT"
printf 'oracle_compiler=%s\n' "$compiler_path"
printf 'oracle_compiler_version=%s\n' "$compiler_version"
printf 'oracle_cflags=%s\n' "$ORACLE_CFLAGS"
printf 'oracle_build_key_sha256=%s\n' "$build_key"
printf 'oracle_runtime_checkout_clean=true\n'
printf 'oracle_runtime_lib_sha256=%s\n' "$runtime_lib_sha"
printf 'oracle_runtime_lib_src_tree_oid=%s\n' "$runtime_lib_src_tree_oid"
printf 'oracle_grammar_checkout_clean=true\n'
printf 'oracle_grammar_parser_sha256=%s\n' "$grammar_parser_sha"
printf 'oracle_grammar_src_tree_oid=%s\n' "$grammar_src_tree_oid"
printf 'oracle_driver_sha256=%s\n' "$driver_sha"
printf 'oracle_artifact_path=%s\n' "$artifact"
printf 'oracle_artifact_sha256=%s\n' "$artifact_sha"
printf 'oracle_deep_digest_format=gts-deep-tree-v1\n'
if [[ -n "$SOURCE_SNAPSHOT" ]]; then
  printf 'oracle_source_mode=immutable_snapshot\n'
else
  printf 'oracle_source_mode=synthetic_lr_control\n'
fi
printf 'oracle_source_sha256=%s\n' "$source_sha"
printf 'oracle_deep_sha256=%s\n' "$deep_sha"

args=("$FUNC_COUNT" "$FULL_ITERS" "$INC_ITERS")
if [[ -n "$SOURCE_SNAPSHOT" ]]; then
  args+=("$SOURCE_SNAPSHOT")
fi
"$artifact" "${args[@]}"
