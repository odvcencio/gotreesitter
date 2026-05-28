#!/usr/bin/env bash
# Sets up the out-of-tree grammar.json corpus needed by parity_test.go's
# markdown / markdown_inline entries. Idempotent — does nothing if already
# set up. Pins the upstream tree-sitter-markdown to a specific SHA so the
# parity corpus is reproducible across checkouts.

set -euo pipefail

PINNED_SHA="c3570720f7f7bbad22fe96603f106276618e0cf5"
UPSTREAM_URL="https://github.com/tree-sitter-grammars/tree-sitter-markdown"
TARGET_DIR="/tmp/grammar_parity/markdown"

if [ -d "$TARGET_DIR/.git" ]; then
  current_sha=$(git -C "$TARGET_DIR" rev-parse HEAD)
  if [ "$current_sha" = "$PINNED_SHA" ]; then
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
