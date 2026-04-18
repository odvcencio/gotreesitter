#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

MODDIR="$(cd "$ROOT_DIR" && go list -m -f '{{.Dir}}' github.com/smacker/go-tree-sitter)"
BUILD_DIR="$(mktemp -d)"
trap 'rm -rf "$BUILD_DIR"' EXIT
CFLAGS=(-O3 -DNDEBUG -I"$MODDIR" -w)
CXXFLAGS=(-O3 -DNDEBUG -I"$MODDIR" -w)
if [[ -n "${CFLAGS_EXTRA:-}" ]]; then
  # shellcheck disable=SC2206
  extra=(${CFLAGS_EXTRA})
  CFLAGS+=("${extra[@]}")
  CXXFLAGS+=("${extra[@]}")
fi

CORE_SRCS=(
  "$MODDIR/alloc.c"
  "$MODDIR/get_changed_ranges.c"
  "$MODDIR/language.c"
  "$MODDIR/lexer.c"
  "$MODDIR/node.c"
  "$MODDIR/parser.c"
  "$MODDIR/stack.c"
  "$MODDIR/subtree.c"
  "$MODDIR/tree.c"
  "$MODDIR/tree_cursor.c"
  "$MODDIR/query.c"
  "$MODDIR/wasm_store.c"
)

for src in "${CORE_SRCS[@]}"; do
  base="$(basename "$src" .c)"
  gcc "${CFLAGS[@]}" -c "$src" -o "$BUILD_DIR/core_${base}.o"
done

run_one() {
  local label="$1"
  local lang_dir="$2"
  local lang_fn="$3"
  local sample="$4"
  local iters="$5"

  local parser_src="$MODDIR/$lang_dir/parser.c"
  local scanner_c="$MODDIR/$lang_dir/scanner.c"
  local scanner_cc="$MODDIR/$lang_dir/scanner.cc"

  if [[ ! -f "$parser_src" ]]; then
    echo "lang=$label skip=missing_parser"
    return
  fi

  local lang_build="$BUILD_DIR/$label"
  mkdir -p "$lang_build"

  gcc "${CFLAGS[@]}" -DTS_LANG_FN=$lang_fn \
    -c "$SCRIPT_DIR/parse_bench.c" -o "$lang_build/bench.o"

  gcc "${CFLAGS[@]}" -c "$parser_src" -o "$lang_build/parser.o"

  local linker="gcc"
  local extra_objects=()

  if [[ -f "$scanner_c" ]]; then
    gcc "${CFLAGS[@]}" -c "$scanner_c" -o "$lang_build/scanner.o"
    extra_objects+=("$lang_build/scanner.o")
  elif [[ -f "$scanner_cc" ]]; then
    g++ "${CXXFLAGS[@]}" -c "$scanner_cc" -o "$lang_build/scanner.o"
    extra_objects+=("$lang_build/scanner.o")
    linker="g++"
  fi

  "$linker" -O3 \
    "$lang_build/bench.o" \
    "$lang_build/parser.o" \
    "${extra_objects[@]}" \
    "$BUILD_DIR"/core_*.o \
    -o "$lang_build/bench"

  "$lang_build/bench" "$label" "$SCRIPT_DIR/testdata/samples/$sample" "$iters"
}

echo "pure-c full-parse matrix (tree-sitter C runtime, no cgo):"
run_one c c tree_sitter_c c.txt 200000
run_one go golang tree_sitter_go go.txt 120000
run_one java java tree_sitter_java java.txt 120000
run_one html html tree_sitter_html html.txt 150000
run_one lua lua tree_sitter_lua lua.txt 150000
run_one toml toml tree_sitter_toml toml.txt 150000
run_one yaml yaml tree_sitter_yaml yaml.txt 120000
