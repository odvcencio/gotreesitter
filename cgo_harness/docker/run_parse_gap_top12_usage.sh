#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

LANGS="go,typescript,tsx,javascript,java,python,rust,c,cpp,c_sharp,json,css"
MODES="cgo_full,go_full,go_no_tree,go_parse_query,go_cursor_walk,go_edit,go_noop_incremental"
CORPUS="cgo_harness/corpus_manifest_top12_usage.json"
QUERIES="cgo_harness/query_manifest_top12_usage.json"
LABEL="top12-usage-ring"

"$SCRIPT_DIR/run_parse_gap_report.sh" \
  --langs "$LANGS" \
  --modes "$MODES" \
  --corpus "$CORPUS" \
  --queries "$QUERIES" \
  --require-parity-langs "typescript" \
  --label "$LABEL" \
  "$@"
