#!/usr/bin/env bash
# Provision the pinned Markdown corpus before an explicit import parity run.
# Unit tests do not download this corpus.

set -euo pipefail

PINNED_SHA="c3570720f7f7bbad22fe96603f106276618e0cf5"
UPSTREAM_URL="https://github.com/tree-sitter-grammars/tree-sitter-markdown"
TARGET_DIR="/tmp/grammar_parity/markdown"

verify_corpus() {
  local grammar
  for grammar in tree-sitter-markdown tree-sitter-markdown-inline; do
    if [[ ! -s "$TARGET_DIR/$grammar/src/grammar.json" ]]; then
      echo "missing pinned corpus: $TARGET_DIR/$grammar/src/grammar.json" >&2
      return 1
    fi
  done
}

if [ -d "$TARGET_DIR/.git" ]; then
  current_sha=$(git -C "$TARGET_DIR" rev-parse HEAD)
  if [ "$current_sha" = "$PINNED_SHA" ]; then
    verify_corpus
    exit 0
  fi
  echo "tree-sitter-markdown at $TARGET_DIR is at $current_sha, expected $PINNED_SHA; resyncing..."
  # Fetch the pinned SHA explicitly so this works even when the existing
  # clone is shallow (e.g. set up by seed_parity_repos.sh with --depth=1).
  # GitHub allows fetching by SHA via uploadpack.allowReachableSHA1InWant.
  git -C "$TARGET_DIR" fetch origin "$PINNED_SHA"
  # Use reset --hard rather than checkout so a dirty working tree (which is
  # disposable scratch under /tmp/grammar_parity) doesn't make resync fail.
  git -C "$TARGET_DIR" reset --hard "$PINNED_SHA"
  verify_corpus
  exit 0
fi

# If TARGET_DIR exists but isn't a git checkout (e.g. seed_parity_repos.sh
# copied a plain tree in), git clone would fail with "destination path
# already exists and is not an empty directory". Wipe it first, but only
# when it's safely under /tmp/grammar_parity/ — refuse otherwise so a typo
# in TARGET_DIR can't rm -rf something unrelated.
if [ -e "$TARGET_DIR" ] && [ ! -d "$TARGET_DIR/.git" ]; then
  case "$TARGET_DIR" in
    /tmp/grammar_parity/*) rm -rf "$TARGET_DIR" ;;
    *) echo "refusing to rm -rf TARGET_DIR=$TARGET_DIR (not under /tmp/grammar_parity)"; exit 2 ;;
  esac
fi

mkdir -p "$(dirname "$TARGET_DIR")"
git clone "$UPSTREAM_URL" "$TARGET_DIR"
git -C "$TARGET_DIR" checkout "$PINNED_SHA"
verify_corpus
