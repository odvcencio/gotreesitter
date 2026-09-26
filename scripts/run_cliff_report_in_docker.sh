#!/usr/bin/env bash
set -uo pipefail

# Run inside the locked C-oracle Docker image. Each grammar gets one process.
cd /workspace || exit 1
mkdir -p harness_out/cliffs
failures=0
read -r -a languages <<< "${GTS_CLIFF_LANGUAGES:-c_sharp php go python elixir markdown html}"
for language in "${languages[@]}"; do
  if ! (
    cd cgo_harness || exit 1
    GOWORK=off GOMAXPROCS=1 GTS_ADMISSION_CANDIDATE=1 \
      GTS_CLIFF_LANGUAGE="$language" \
      GTS_CLIFF_REPORT_PATH="/workspace/harness_out/cliffs/$language.json" \
      go test -tags treesitter_c_parity -run '^TestCliffReport$' -count=1 -timeout 30m -v
  ); then
    failures=1
  fi
  if [[ ! -s "harness_out/cliffs/$language.json" ]]; then
    echo "missing cliff report: $language" >&2
    failures=1
  fi
done
exit "$failures"
