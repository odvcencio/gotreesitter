#!/usr/bin/env bash
# Fetch the authenticated corpus lock at run time and check it against the
# digest committed in cgo_harness/perf_scan/corpus_sources.lock.sha256.
#
# The lock itself is deliberately not committed: the blocker receipts under
# cgo_harness assert that the repository has no corpus_sources.lock, so a
# committed copy could stand in for authenticated evidence. The digest is the
# authentication; the URL only says where to read the bytes from.
#
# Usage: fetch_grammar_receipt_lock.sh <url> <output-path>
# Exit status: 0 fetched and verified, 3 no URL given, 1 anything else.
set -euo pipefail

url="${1:-}"
out="${2:?usage: fetch_grammar_receipt_lock.sh <url> <output-path>}"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
want="$(awk 'NF {print $1; exit}' "$root/cgo_harness/perf_scan/corpus_sources.lock.sha256")"

if [[ -z "$url" ]]; then
  echo "no corpus lock URL: set the repository variable GTS_CORPUS_LOCK_URL, or pass the corpus_lock_url input" >&2
  exit 3
fi
case "$url" in
  https://*) ;;
  *) echo "corpus lock URL must be https" >&2; exit 1 ;;
esac
mkdir -p "$(dirname "$out")"
curl -fsSL --retry 3 --max-time 60 --max-filesize 1048576 "$url" -o "$out"
got="$(sha256sum "$out" | awk '{print $1}')"
if [[ "$got" != "$want" ]]; then
  rm -f "$out"
  echo "corpus lock digest $got does not match the pinned digest $want" >&2
  exit 1
fi
echo "corpus lock verified: $want"
