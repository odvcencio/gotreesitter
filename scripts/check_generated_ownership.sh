#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

failures=()
checked=0
source_list="$(mktemp)"
trap 'rm -f "$source_list"' EXIT

git ls-files --cached --others --exclude-standard -z -- '*.go' >"$source_list"

expected_owner() {
  local source_file="$1"
  case "$source_file" in
    grammars/linguist_gen.go)
      printf '%s\n' 'cmd/gen_linguist'
      ;;
    grammars/z_subset_blob_embed_*.go)
      printf '%s\n' 'cmd/gen_subset_blob_embeds'
      ;;
    grammars/embedded_grammars_gen.go|grammars/registry_builtin_gen.go)
      printf '%s\n' 'cmd/ts2go batch'
      ;;
    grammars/*_external_lex_states_gen.go)
      printf '%s\n' 'cmd/ts2go'
      ;;
    grammars/runtime/*_reserved_words_gen.go)
      printf '%s\n' 'cmd/ts2go'
      ;;
    grammargen/fortran_grammar.go|grammargen/go_grammar.go|grammargen/javascript_grammar.go|grammargen/kotlin_grammar.go|grammargen/swift_grammar.go|grammargen/tsx_grammar.go|grammargen/typescript_grammar.go)
      printf '%s\n' 'cmd/grammargen emit'
      ;;
  esac
}

owner_exists() {
  case "$1" in
    cmd/gen_linguist)
      [[ -d cmd/gen_linguist ]]
      ;;
    cmd/gen_subset_blob_embeds|cmd/gen_grammar_packages)
      [[ -d "$1" ]]
      ;;
    'cmd/grammargen emit')
      [[ -d cmd/grammargen ]]
      ;;
    cmd/ts2go|'cmd/ts2go batch')
      [[ -d cmd/ts2go ]]
      ;;
    cmd/tsquery)
      [[ -d cmd/tsquery ]]
      ;;
    *)
      return 1
      ;;
  esac
}

while IFS= read -r -d '' source_file; do
  if [[ ! -f "$source_file" ]]; then
    continue
  fi
  marker="$(awk 'NR <= 10 && /^\/\/ Code generated/ { print; exit }' "$source_file")"
  marker_line="$(awk 'NR <= 10 && /^\/\/ Code generated/ { print NR; exit }' "$source_file")"
  package_line="$(awk '/^package / { print NR; exit }' "$source_file")"
  wanted_owner="$(expected_owner "$source_file")"
  base_name="${source_file##*/}"

  if [[ -z "$marker" ]]; then
    if [[ -n "$wanted_owner" || "$base_name" == *_gen.go || "$base_name" == *_generated.go ]]; then
      failures+=("$source_file: missing generated-code marker")
    fi
    continue
  fi

  if [[ ! "$marker" =~ ^//\ Code\ generated\ by\ (.+)\;\ DO\ NOT\ EDIT\.$ ]]; then
    failures+=("$source_file: nonstandard marker: $marker")
    continue
  fi
  if [[ -n "$package_line" && "$marker_line" -gt "$package_line" ]]; then
    failures+=("$source_file: generated-code marker follows the package clause")
  fi

  owner="${BASH_REMATCH[1]}"
  if ! owner_exists "$owner"; then
    failures+=("$source_file: unknown generator owner: $owner")
  fi
  if [[ -n "$wanted_owner" && "$owner" != "$wanted_owner" ]]; then
    failures+=("$source_file: owner is $owner; want $wanted_owner")
  fi
  checked=$((checked + 1))
done <"$source_list"

if (( ${#failures[@]} != 0 )); then
  printf '%s\n' "${failures[@]}" >&2
  exit 1
fi

printf 'checked %d generated Go files\n' "$checked"
