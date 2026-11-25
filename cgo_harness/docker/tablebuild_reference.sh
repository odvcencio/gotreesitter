#!/usr/bin/env bash
set -euo pipefail

# Build the table-build reference set of one grammar, in the folder layout that
# internal/lexbuild reads through LEXBUILD_REF and that
# cgo_harness/tablebuild_ref_c_test.go reads through TABLEBUILD_REF:
#
#   <out>/<grammar>/src/grammar.json      pinned
#   <out>/<grammar>/src/node-types.json   pinned
#   <out>/<grammar>/parser-pinned.c       pinned src/parser.c
#   <out>/<grammar>/parser.c              tree-sitter generate, CLI v0.27.1
#   <out>/<grammar>/parser-noopt.c        the same with --disable-optimizations
#   <out>/<grammar>/tree_sitter/parser.h  written by the same run
#   <out>/<grammar>/parser-<v>.c          one per --also-cli <v>
#   <out>/<grammar>/MANIFEST              commit, ABI, CLI versions, sha256
#
# Every generator run reads the pinned src/grammar.json, never grammar.js, and
# asks for the ABI of the pinned parser.c. The CLIs are the unmodified npm
# builds that cgo_harness/docker/Dockerfile installs under
# /opt/tree-sitter-cli/<version>. Run it inside the parity container:
#
#   bash cgo_harness/docker/run_parity_in_docker.sh -- \
#     "cd /workspace && bash cgo_harness/docker/tablebuild_reference.sh go --also-cli 0.25.8"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
LOCK_FILE="$REPO_ROOT/grammars/languages.lock"
OUT_ROOT="$REPO_ROOT/harness_out/tablebuild_ref"
CLI_ROOT="${TABLEBUILD_CLI_ROOT:-/opt/tree-sitter-cli}"
MAIN_CLI="0.27.1"
ALSO_CLI=()
NAME=""

usage() {
  cat <<'EOF'
Usage: tablebuild_reference.sh <grammar> [--out DIR] [--also-cli VERSION]...

  <grammar>           a name in grammars/languages.lock
  --out DIR           reference root (default: <repo>/harness_out/tablebuild_ref)
  --also-cli VERSION  also write parser-VERSION.c with the CLI of that version
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --out) OUT_ROOT="$2"; shift 2 ;;
    --also-cli) ALSO_CLI+=("$2"); shift 2 ;;
    -h|--help) usage; exit 0 ;;
    -*) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
    *)
      if [[ -n "$NAME" ]]; then
        echo "one grammar per run" >&2
        exit 2
      fi
      NAME="$1"; shift ;;
  esac
done
if [[ -z "$NAME" ]]; then
  usage >&2
  exit 2
fi

cli() {
  local bin="$CLI_ROOT/$1/bin/tree-sitter"
  if [[ ! -x "$bin" ]]; then
    echo "tree-sitter CLI $1 is not installed at $bin (rebuild the parity image)" >&2
    exit 1
  fi
  echo "$bin"
}

read -r URL COMMIT SUBDIR < <(awk -v n="$NAME" '$1 == n { print $2, $3, ($4 == "" ? "src" : $4); exit }' "$LOCK_FILE")
if [[ -z "${URL:-}" || -z "${COMMIT:-}" ]]; then
  echo "no pinned entry for $NAME in $LOCK_FILE" >&2
  exit 1
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
REPO="$WORK/repo"
git init -q "$REPO"
git -C "$REPO" remote add origin "$URL"
fetched=0
for attempt in 1 2 3 4; do
  if git -C "$REPO" fetch -q --depth=1 origin "$COMMIT"; then
    fetched=1
    break
  fi
  sleep $((attempt * 5))
done
if [[ "$fetched" != 1 ]]; then
  echo "fetch of $URL $COMMIT failed" >&2
  exit 1
fi
git -C "$REPO" checkout -q --detach FETCH_HEAD
HEAD="$(git -C "$REPO" rev-parse HEAD)"
if [[ "$HEAD" != "$COMMIT" ]]; then
  echo "checked out $HEAD, pinned $COMMIT" >&2
  exit 1
fi

SRC="$REPO/$SUBDIR"
for f in grammar.json node-types.json parser.c; do
  if [[ ! -f "$SRC/$f" ]]; then
    echo "$NAME@$COMMIT has no $SUBDIR/$f: no pinned reference" >&2
    exit 1
  fi
done
# The generator reads tree-sitter.json (the grammar version that ABI 15 writes
# into the language metadata) from the folder above src.
GRAMMAR_ROOT="$(dirname "$SRC")"
ABI="$(sed -n 's/^#define LANGUAGE_VERSION \([0-9][0-9]*\)$/\1/p' "$SRC/parser.c")"
if [[ -z "$ABI" ]]; then
  echo "pinned parser.c has no LANGUAGE_VERSION" >&2
  exit 1
fi

DEST="$OUT_ROOT/$NAME"
rm -rf "$DEST"
mkdir -p "$DEST/src" "$DEST/tree_sitter"
cp "$SRC/grammar.json" "$SRC/node-types.json" "$DEST/src/"
cp "$SRC/parser.c" "$DEST/parser-pinned.c"
# The licence of the grammar, for a fixture that copies its grammar.json.
for f in LICENSE LICENSE.md LICENSE.txt; do
  if [[ -f "$REPO/$f" ]]; then
    cp "$REPO/$f" "$DEST/LICENSE"
    break
  fi
done

# generate <version> <output file> [flags...]: one unmodified generator run.
generate() {
  local version="$1" outfile="$2"
  shift 2
  local bin out
  bin="$(cli "$version")"
  out="$WORK/gen-$version-$(basename "$outfile" .c)"
  if ! (cd "$GRAMMAR_ROOT" && "$bin" generate --abi "$ABI" "$@" -o "$out" "$SRC/grammar.json") >"$WORK/gen.log" 2>&1; then
    echo "GENERATE-FAILED $NAME $version $* : $(tr '\n' ' ' <"$WORK/gen.log" | cut -c1-400)"
    return 1
  fi
  cp "$out/parser.c" "$DEST/$outfile"
  if [[ "$outfile" == parser.c ]]; then
    cp "$out/tree_sitter/parser.h" "$DEST/tree_sitter/parser.h"
  fi
}

generate "$MAIN_CLI" parser.c
generate "$MAIN_CLI" parser-noopt.c --disable-optimizations || true
for v in "${ALSO_CLI[@]}"; do
  generate "$v" "parser-$v.c" || true
done

{
  echo "grammar $NAME"
  echo "repo $URL"
  echo "commit $COMMIT"
  echo "abi $ABI"
  for v in "$MAIN_CLI" "${ALSO_CLI[@]}"; do
    echo "cli $v $("$(cli "$v")" --version)"
  done
  (cd "$DEST" && find . -type f ! -name MANIFEST | LC_ALL=C sort | xargs sha256sum)
} >"$DEST/MANIFEST"

# Quick verdict; cgo_harness/tablebuild_ref_c_test.go names the first table
# that differs. The first line is the version comment when there is one.
strip_version() {
  sed '1{/^\/\* Automatically @\{0,1\}generated by tree-sitter/d;}' "$1"
}
for f in "$DEST"/parser.c "$DEST"/parser-[0-9]*.c; do
  [[ -f "$f" ]] || continue
  if cmp -s <(strip_version "$DEST/parser-pinned.c") <(strip_version "$f"); then
    echo "ELIGIBILITY $NAME $(basename "$f") equal to pinned apart from the version comment"
  else
    echo "ELIGIBILITY $NAME $(basename "$f") differs from pinned"
  fi
done
echo "reference set: $DEST"
