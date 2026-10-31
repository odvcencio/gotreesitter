#!/usr/bin/env bash
set -uo pipefail

# Run inside the locked C-oracle Docker image. Each fixture gets one process.
#
# GTS_CLIFF_LANGUAGES names the fixtures to run, separated by spaces. It keeps
# its old name, but it holds fixture names: several fixtures can share a
# grammar. Without it, the script runs every fixture in the table in
# cgo_harness/cliff_fixture_table_test.go.
#
# GTS_CLIFF_REVISION, GTS_CLIFF_EVENT, and GTS_CLIFF_ON_DEFAULT_BRANCH, when
# set, go into every report row.
#
# Two files in harness_out/cliffs tell the workflow whether the run is whole:
#   expected.txt          the fixtures this run measures, one per line
#   fixture-table.FAILED  the output of TestCliffFixtureTable, only when it failed
cd /workspace || exit 1
mkdir -p harness_out/cliffs
# Each file describes one run, so drop the last run's copies.
rm -f harness_out/cliffs/expected.txt harness_out/cliffs/fixture-table.FAILED
failures=0
# TestCliffFixtureTable checks every fixture against its three manifests in
# about 0.02 s. A failure does not stop the measurement. It leaves a marker, and
# the workflow fails a nightly that has one.
if ! table=$(cd cgo_harness && GOWORK=off \
  go test -tags treesitter_c_parity -run '^TestCliffFixtureTable$' -count=1 -timeout 30m -v 2>&1); then
  echo "TestCliffFixtureTable failed:" >&2
  printf '%s\n' "$table" | tee harness_out/cliffs/fixture-table.FAILED >&2
  failures=1
else
  echo "TestCliffFixtureTable passed."
fi
if [[ -n "${GTS_CLIFF_LANGUAGES:-}" ]]; then
  read -r -a fixtures <<< "$GTS_CLIFF_LANGUAGES"
else
  # The test table is the only list of fixtures, so ask the test for it.
  if ! listing=$(cd cgo_harness && GOWORK=off GTS_CLIFF_LIST=1 \
    go test -tags treesitter_c_parity -run '^TestCliffFixtureNames$' -count=1 -timeout 30m -v 2>&1); then
    echo "cannot list the cliff fixtures:" >&2
    printf '%s\n' "$listing" >&2
    exit 1
  fi
  mapfile -t fixtures < <(sed -n 's/^CLIFF_FIXTURE //p' <<< "$listing")
fi
if [[ ${#fixtures[@]} -eq 0 ]]; then
  echo "no cliff fixtures to run" >&2
  exit 1
fi
# The workflow compares this list with the report files. Each fixture removes
# its old report first, so one that fails before it writes a report shows as
# missing.
printf '%s\n' "${fixtures[@]}" > harness_out/cliffs/expected.txt
for fixture in "${fixtures[@]}"; do
  rm -f "harness_out/cliffs/$fixture.json"
  if ! (
    cd cgo_harness || exit 1
    GOWORK=off GOMAXPROCS=1 GTS_ADMISSION_CANDIDATE=1 \
      GTS_CLIFF_LANGUAGE="$fixture" \
      GTS_CLIFF_REPORT_PATH="/workspace/harness_out/cliffs/$fixture.json" \
      go test -tags treesitter_c_parity -run '^TestCliffReport$' -count=1 -timeout 30m -v
  ); then
    failures=1
  fi
  if [[ ! -s "harness_out/cliffs/$fixture.json" ]]; then
    echo "missing cliff report: $fixture" >&2
    failures=1
  fi
done
exit "$failures"
