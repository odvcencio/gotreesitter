#!/usr/bin/env bash
# Scanner differential test for ONE grammar, in one container: the compiled C
# external scanner (with the C runtime's own lexer) against the Go port, call by
# call. See scandiff_test.go for the switches.
#
#   bash cgo_harness/docker/run_parity_in_docker.sh --label scandiff-<language> \
#     -- "bash /workspace/cgo_harness/docker/scandiff.sh <language> [calls] [seed]"
#
#   bash cgo_harness/docker/scandiff.sh --self-test
# checks the output check on fake outputs and proves that each bad case fails.
#
# Environment: SCANDIFF_COVER=1 builds the C scanner with --coverage and reports
# gcov line and branch counts; SCANDIFF_REPORT=1 never fails on differences (a
# run that compares nothing still fails); SCANDIFF_OUT=<folder> receives the
# reproducers. Other SCANDIFF_* switches pass through to the test.
#
# A scanner can hang or crash on a damaged saved state. The test exits with 87 on a
# hang and names the call; this script then restarts the run and skips that call.
#
# A run counts only when its output has a "--- PASS: TestScannerDiff" line, no
# skip, a SCANDIFF summary line for the asked grammar, and a positive
# ordinary_calls count. A missing summary, a skipped test, a zero count or a
# missing grammar therefore fails the script.
set -uo pipefail

# check_output <language> <file>: succeeds when the output of one run is acceptable.
check_output() {
  local lang=$1 file=$2
  grep -q -- '^--- PASS: TestScannerDiff ' "$file" || { echo "no '--- PASS: TestScannerDiff' line" >&2; return 1; }
  if grep -q -- '^--- SKIP' "$file"; then echo "the test was skipped" >&2; return 1; fi
  local line; line=$(grep -m1 "^SCANDIFF lang=${lang} " "$file") || { echo "no SCANDIFF summary line for ${lang}" >&2; return 1; }
  local n; n=$(printf '%s\n' "$line" | sed -n 's/.* ordinary_calls=\([0-9][0-9]*\) .*/\1/p')
  [ -n "$n" ] && [ "$n" -gt 0 ] || { echo "ordinary_calls is not above 0 (${n:-missing})" >&2; return 1; }
  local rm; rm=$(printf '%s\n' "$line" | sed -n 's/.* report_mode=\([a-z]*\) .*/\1/p')
  [ -n "$rm" ] || { echo "no report_mode field in the summary" >&2; return 1; }
  local cc; cc=$(printf '%s\n' "$line" | sed -n 's/.* c_crashes=\([0-9][0-9]*\) .*/\1/p')
  [ -n "$cc" ] || { echo "no c_crashes field in the summary" >&2; return 1; }
  if [ "$cc" -gt 0 ] && [ "$rm" != true ]; then echo "C scanner crashed $cc times in a gate run" >&2; return 1; fi
  local sk; sk=$(printf '%s\n' "$line" | sed -n 's/.* skipped=\([0-9][0-9]*\) .*/\1/p')
  [ -n "$sk" ] && [ "$sk" -eq 0 ] || { echo "skipped calls: ${sk:-missing}" >&2; return 1; }
}

# refuse_mutant: the mutant switches exist only in self-test builds (tag scandiff_mutants); this script never
# builds with that tag, so a set SCANDIFF_MUTANT is a mistake or an attempt to weaken a run.
refuse_mutant() {
  if [ -n "${SCANDIFF_MUTANT:-}" ]; then echo "SCANDIFF_MUTANT is set: mutant switches exist only in self-test builds" >&2; return 1; fi
  return 0
}

if [ "${1:-}" = "--self-test" ]; then
  d=$(mktemp -d); trap 'rm -rf "$d"' EXIT
  fails=0
  expect() { # description, expected exit, language, file
    local desc=$1 want=$2; shift 2
    if check_output "$@" >/dev/null 2>&1; then got=0; else got=1; fi
    if [ "$got" = "$want" ]; then echo "ok   self-test: ${desc}"; else echo "FAIL self-test: ${desc} (exit ${got}, want ${want})"; fails=1; fi
  }
  sum='SCANDIFF lang=css report_mode=false mode= seed=1 calls=100 ordinary_calls=100 skipped=0 c_crashes=0 diff_calls=0'
  printf '%s\n' "$sum" '--- PASS: TestScannerDiff (0.10s)' 'PASS' >"$d/good"
  printf '%s\n' "$sum" 'PASS' >"$d/nopass"
  printf '%s\n' '--- SKIP: TestScannerDiff (0.00s)' 'PASS' >"$d/skip"
  printf '%s\n' '--- PASS: TestScannerDiff (0.10s)' 'PASS' >"$d/nosummary"
  printf '%s\n' "${sum/ordinary_calls=100/ordinary_calls=0}" '--- PASS: TestScannerDiff (0.10s)' >"$d/zero"
  printf '%s\n' "${sum/lang=css/lang=scss}" '--- PASS: TestScannerDiff (0.10s)' >"$d/otherlang"
  printf '%s\n' "${sum/skipped=0/skipped=2}" '--- PASS: TestScannerDiff (0.10s)' >"$d/skipped"
  printf '%s\n' "${sum/c_crashes=0/c_crashes=3}" '--- PASS: TestScannerDiff (0.10s)' >"$d/crash"
  printf '%s\n' "${sum/c_crashes=0/c_crashes=3}" '--- PASS: TestScannerDiff (0.10s)' | sed 's/report_mode=false/report_mode=true/' >"$d/crashreport"
  printf '%s\n' "${sum/report_mode=false /}" '--- PASS: TestScannerDiff (0.10s)' >"$d/noreport"
  if (SCANDIFF_MUTANT=len; refuse_mutant) 2>/dev/null; then echo "FAIL self-test: a mutant variable must be refused"; fails=1; else echo "ok   self-test: a mutant variable is refused"; fi
  expect "good output is accepted" 0 css "$d/good"
  expect "missing PASS line fails" 1 css "$d/nopass"
  expect "skipped test fails" 1 css "$d/skip"
  expect "missing summary fails" 1 css "$d/nosummary"
  expect "zero ordinary calls fails" 1 css "$d/zero"
  expect "summary of another grammar fails" 1 css "$d/otherlang"
  expect "skipped calls fail" 1 css "$d/skipped"
  expect "C crashes in a gate run fail" 1 css "$d/crash"
  expect "C crashes in report mode are reported, not failed here" 0 css "$d/crashreport"
  expect "a summary without report_mode fails" 1 css "$d/noreport"
  exit "$fails"
fi

refuse_mutant || exit 2
lang=${1:?usage: scandiff.sh <language> [calls] [seed] | --self-test}
calls=${2:-200000}
seed=${3:-1}
case "$calls" in ''|*[!0-9]*) echo "calls must be a number above 0" >&2; exit 2;; esac
[ "$calls" -gt 0 ] || { echo "calls must be above 0" >&2; exit 2; }
case "$seed" in ''|*[!0-9]*) echo "seed must be a number" >&2; exit 2;; esac
cd "$(dirname "${BASH_SOURCE[0]}")/.."   # cgo_harness
export GOWORK=off GOMAXPROCS=1 SCANDIFF_REQUIRE=1
bin="${TMPDIR:-/tmp}/scandiff.$$.test"
trap 'rm -f "$bin"' EXIT
go test -c -vet=off -tags 'cgo treesitter_c_parity' -o "$bin" . || exit 2
skip=""
for try in 1 2 3 4 5 6; do
  log=$(mktemp)
  env SCANDIFF_LANG="$lang" SCANDIFF_CALLS="$calls" SCANDIFF_SEED="$seed" SCANDIFF_SKIP="$skip" \
    "$bin" -test.run '^TestScannerDiff$' -test.count=1 -test.v -test.timeout=40m >"$log" 2>&1
  rc=$?
  if [ "$rc" -eq 87 ]; then
    n=$(sed -n 's/^SCANDIFF_HANG .* call=\([0-9]*\)$/\1/p' "$log" | tail -1)
    skip="$skip,$n"; echo "hang at call $n; restarting without it"; rm -f "$log"; continue
  fi
  grep -E '^(SCANDIFF|--- |FAIL|PASS|ok )' "$log"
  if [ "$rc" -eq 0 ] && ! check_output "$lang" "$log"; then rc=4; fi
  # a run that needed a hang to be skipped never passes
  if [ "$rc" -eq 0 ] && [ -n "$skip" ]; then echo "hang skipped: $skip" >&2; rc=5; fi
  rm -f "$log"
  exit "$rc"
done
echo "too many hangs" >&2
exit 3
