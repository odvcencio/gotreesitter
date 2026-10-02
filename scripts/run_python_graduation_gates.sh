#!/usr/bin/env bash
set -euo pipefail

# Keep the corpus external and authenticate its checkout before invoking this
# script through the Docker harness. Preserve every failed gate observation.
if [[ "${GTS_PARITY_IN_DOCKER:-}" != 1 || $# != 1 ]]; then
  echo "usage (inside Docker): scripts/run_python_graduation_gates.sh EXTERNAL_OUTPUT_DIR" >&2
  exit 2
fi
repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
mkdir -p "$1"
output=$(cd "$1" && pwd)
export GOWORK=off GOMAXPROCS=1 GTS_CEILING_LANGUAGE=python
cd "$repo/cgo_harness"
go test -c -tags treesitter_c_parity,gts_engine_ceiling -o "$output/python-gates.test" .
failed=0
for suite in TestPythonGraduationCorpusOutput TestPythonGraduationEditSession \
  TestPythonGraduationTupleLists TestCompactEditsIssue728PythonScanner; do
  status=0
  "$output/python-gates.test" -test.run "^$suite$" -test.v -test.count=1 \
    -test.timeout=20m > "$output/$suite.log" 2>&1 || status=$?
  printf '%s\n' "$status" > "$output/$suite.exit"
  printf '%s: exit %s\n' "$suite" "$status"
  if ((status != 0)); then failed=1; fi
done
read -r corpus_status < "$output/TestPythonGraduationCorpusOutput.exit"
if ((corpus_status != 0)) && [[ -f corpus_real/python/enum.py ]]; then
  # Keep a compact-only corpus mismatch small enough to diagnose. These are
  # observations; the failed corpus gate above remains a failure.
  go build -buildvcs=false -tags treesitter_c_parity -o "$output/gts_mismatch" ./cmd/gts_mismatch
  "$output/gts_mismatch" -mode check -grammar python -in corpus_real/python/enum.py \
    > "$output/enum-check.json"
  "$output/gts_mismatch" -mode min -grammar python -route compact \
    -in corpus_real/python/enum.py -out "$output/enum-min.py" \
    -timeout 3m -max-tests 3000 > "$output/enum-min.json"
  "$output/gts_mismatch" -mode show -grammar python -route compact \
    -in "$output/enum-min.py" > "$output/enum-trees.txt"
fi
exit "$failed"
