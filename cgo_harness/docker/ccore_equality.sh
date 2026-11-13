#!/usr/bin/env bash
# Equality of internal/ccore with pinned C for ONE grammar, in one container.
#
#   bash cgo_harness/docker/run_parity_in_docker.sh --label ci-ccore-<language> ... \
#     -- "bash /workspace/cgo_harness/docker/ccore_equality.sh <language>"
#
# The checks are the tests in cgo_harness/ccore_*_test.go: parse log (trace),
# visible-tree digest, subtree records, edit sessions, lookup tables and the
# committed goldens; for json also lexprobe_test.go (the exact-lexer record
# against the compiled C lexer). Every input is a file under a testdata folder (named in
# the tests) or a literal or generated input in the tests; no private input is
# read. A difference fails the run. Nothing here parses through internal/ccore
# outside tests.
#
# A test counts only when its output has a "--- PASS: <test>" line AND a
# "CCCOUNT test=<test> set=<set> checked=<n>" line with n > 0 for the input
# set that was asked for. A wrong -test.run pattern, a skipped test or a test
# that compared nothing therefore fails the script.
#
#   bash cgo_harness/docker/ccore_equality.sh --self-test
# runs check_output on fake outputs and proves that each of those cases fails.
set -euo pipefail

# The campaign switches must not be set in a required check.
unset CCORE_REPORT_ONLY CCORE_UPDATE_GOLDEN

# check_output <test> <set> <output file>: succeeds when the output holds the
# PASS line and a count line for the set with a positive count. It prints the
# count on success and the reason on failure.
check_output() {
  local test=$1 set=$2 file=$3
  if ! grep -q -- "^--- PASS: ${test} " "$file"; then
    echo "no '--- PASS: ${test}' line" >&2; return 1
  fi
  local n
  n=$(sed -n "s/^CCCOUNT test=${test} set=${set} checked=\([0-9][0-9]*\)\$/\1/p" "$file" | awk '{s += $1} END {print s + 0}')
  if [ "$n" -le 0 ]; then
    echo "no positive 'CCCOUNT test=${test} set=${set}' line (checked ${n})" >&2; return 1
  fi
  # The exact-lexer probe must have compared the committed record: a grammar
  # without a registered record prints "committed_record=none" and still passes
  # the test itself, which would let the gate lose that comparison silently.
  if [ "$test" = TestLexProbe ] && ! grep -q '^CCLEXPROBE .* committed_record=equal ok=true' "$file"; then
    echo "no 'CCLEXPROBE ... committed_record=equal ok=true' line (is the record registered?)" >&2; return 1
  fi
  echo "$n"
}

if [ "${1:-}" = "--self-test" ]; then
  d=$(mktemp -d); trap 'rm -rf "$d"' EXIT
  fails=0
  expect() { # description, expected exit (0 or 1), test, set, file
    local desc=$1 want=$2; shift 2
    if check_output "$@" >/dev/null 2>&1; then got=0; else got=1; fi
    if [ "$got" = "$want" ]; then echo "ok   self-test: ${desc}"; else echo "FAIL self-test: ${desc} (exit ${got}, want ${want})"; fails=1; fi
  }
  printf '%s\n' '=== RUN   TestX' 'CCCOUNT test=TestX set=json checked=5' '--- PASS: TestX (0.01s)' 'PASS' >"$d/good"
  printf '%s\n' '=== RUN   TestX' 'CCCOUNT test=TestX set=json checked=5' 'PASS' >"$d/nopass"
  printf '%s\n' '=== RUN   TestX' 'CCCOUNT test=TestX set=json checked=0' '--- PASS: TestX (0.01s)' 'PASS' >"$d/zero"
  printf '%s\n' '=== RUN   TestX' '--- PASS: TestX (0.00s)' 'PASS' >"$d/nocount"
  printf '%s\n' '=== RUN   TestX' '--- SKIP: TestX (0.00s)' 'PASS' >"$d/skip"
  printf '%s\n' 'testing: warning: no tests to run' 'PASS' >"$d/norun"
  printf '%s\n' '=== RUN   TestX' 'CCCOUNT test=TestX set=other checked=9' '--- PASS: TestX (0.01s)' 'PASS' >"$d/otherset"
  printf '%s\n' '=== RUN   TestLexProbe' 'CCLEXPROBE language=json main_states=44 record_bytes=208 committed_record=equal ok=true' 'CCCOUNT test=TestLexProbe set=json checked=88' '--- PASS: TestLexProbe (1.00s)' 'PASS' >"$d/probe-good"
  printf '%s\n' '=== RUN   TestLexProbe' 'CCLEXPROBE language=json main_states=44 record_bytes=208 committed_record=equal ok=false' 'CCCOUNT test=TestLexProbe set=json checked=88' '--- FAIL: TestLexProbe (1.00s)' 'FAIL' >"$d/probe-differs"
  printf '%s\n' '=== RUN   TestLexProbe' '--- SKIP: TestLexProbe (0.00s)' 'PASS' >"$d/probe-unset"
  printf '%s\n' '=== RUN   TestLexProbe' 'CCLEXPROBE language=json main_states=44 record_bytes=208 committed_record=none ok=true' 'CCCOUNT test=TestLexProbe set=json checked=88' '--- PASS: TestLexProbe (1.00s)' 'PASS' >"$d/probe-norecord"
  expect "good output is accepted" 0 TestX json "$d/good"
  expect "lex probe output that passed is accepted" 0 TestLexProbe json "$d/probe-good"
  expect "lex probe that found a difference fails" 1 TestLexProbe json "$d/probe-differs"
  expect "lex probe that passed without a registered record (committed_record=none) fails" 1 TestLexProbe json "$d/probe-norecord"
  expect "lex probe without LEXPROBE_LANG (skipped) fails" 1 TestLexProbe json "$d/probe-unset"
  expect "missing PASS line fails" 1 TestX json "$d/nopass"
  expect "zero count fails" 1 TestX json "$d/zero"
  expect "no count line fails" 1 TestX json "$d/nocount"
  expect "skipped test fails" 1 TestX json "$d/skip"
  expect "pattern that matches no test fails" 1 TestX json "$d/norun"
  expect "count of another set fails" 1 TestX json "$d/otherset"
  expect "misspelled test name fails" 1 TestXx json "$d/good"
  printf '%s\n' '=== RUN   TestCcoreKeyword' 'CCKWINC place=1 k=0' 'CCCOUNT test=TestCcoreKeyword set=typescript checked=241' '--- PASS: TestCcoreKeyword (4.80s)' 'PASS' >"$d/kw"
  printf '%s\n' '=== RUN   TestCcoreKeyword' '--- SKIP: TestCcoreKeyword (0.00s)' 'PASS' >"$d/kwskip"
  printf '%s\n' '=== RUN   TestCcoreStoreChain' 'CCCOUNT test=TestCcoreStoreChain set=python checked=0' '--- PASS: TestCcoreStoreChain (9.00s)' 'PASS' >"$d/chain0"
  expect "keyword set output is accepted" 0 TestCcoreKeyword typescript "$d/kw"
  expect "keyword set skipped (CCORE_KW unset) fails" 1 TestCcoreKeyword typescript "$d/kwskip"
  printf '%s\n' '=== RUN   TestCcoreNodes' 'CCCOUNT test=TestCcoreNodes set=json checked=9' 'CCCOUNT test=TestCcoreNodes set=json-masks checked=4' '--- PASS: TestCcoreNodes (1.00s)' 'PASS' >"$d/nodes"
  printf '%s\n' '=== RUN   TestCcoreNodes' 'CCCOUNT test=TestCcoreNodes set=json checked=9' 'CCCOUNT test=TestCcoreNodes set=json-masks checked=0' '--- PASS: TestCcoreNodes (1.00s)' 'PASS' >"$d/nodes0"
  expect "node readers with masks are accepted" 0 TestCcoreNodes json-masks "$d/nodes"
  expect "node readers with zero masks fail" 1 TestCcoreNodes json-masks "$d/nodes0"
  expect "store chain that sampled nothing fails" 1 TestCcoreStoreChain python "$d/chain0"
  exit "$fails"
fi

lang=${1:?usage: ccore_equality.sh <language> | --self-test}
cd "$(dirname "${BASH_SOURCE[0]}")/.."   # cgo_harness

# Input sets per language (CCORE_LANG values), and the edit-session set.
case "$lang" in
  json)       sets="json json-eof json-rec-d"; inc="json-session"; golden=1; chain=""; kw="" ;;
  python)     sets="python python-eof python-session"; inc="python-session"; golden=0; chain=python; kw="" ;;
  c)          sets="c c-eof c-session"; inc=""; golden=0; chain=""; kw="" ;;
  typescript) sets="typescript typescript-session"; inc="typescript-session"; golden=0; chain=""; kw=typescript ;;
  *) echo "ccore_equality.sh: no input sets for language '$lang'" >&2; exit 2 ;;
esac

export GOWORK=off GOMAXPROCS=1
bin="${TMPDIR:-/tmp}/ccore-equality.$$.test"
trap 'rm -f "$bin"' EXIT
go test -c -tags 'cgo treesitter_c_parity' -o "$bin" .

failed=0
total_trace=0 total_digest=0 total_records=0 total_inc=0
last_n=0
extra_set="" # when set, run() also requires a positive count line for this second set
run() { # test-name, set name for the count line, output-line prefix to count, then NAME=value pairs
  local test=$1 set=$2 prefix=$3; shift 3
  local out; out=$(mktemp)
  last_n=0
  if env "$@" "$bin" -test.run "^${test}\$" -test.count=1 -test.v -test.timeout=40m >"$out" 2>&1; then
    local n
    if n=$(check_output "$test" "$set" "$out" 2>"$out.why"); then
      last_n=$(grep -c "^${prefix} " "$out" || true)
      if [ -n "$extra_set" ]; then
        if m=$(check_output "$test" "$extra_set" "$out" 2>"$out.why"); then
          echo "ok   ${test} $* checked=${n} ${extra_set}=${m} lines=${last_n}"
        else
          echo "FAIL ${test} $*: $(cat "$out.why")"; tail -n 20 "$out"; failed=1
        fi
      else
        echo "ok   ${test} $* checked=${n} lines=${last_n}"
      fi
    else
      echo "FAIL ${test} $*: $(cat "$out.why")"; tail -n 20 "$out"; failed=1
    fi
  else
    echo "FAIL ${test} $*"; tail -n 40 "$out"; failed=1
  fi
  rm -f "$out" "$out.why"
}

norm="CCORE_NORM_NAMES=1"; [ "$lang" = json ] && norm="CCORE_NORM_NAMES="
for set in $sets; do
  run TestCcoreTrace   "$set" CCTRACE  "$norm" CCORE_LANG=$set; total_trace=$((total_trace + last_n))
  run TestCcoreDigest  "$set" CCDIGEST "$norm" CCORE_LANG=$set; total_digest=$((total_digest + last_n))
  run TestCcoreRecords "$set" CCREC    "$norm" CCORE_LANG=$set; total_records=$((total_records + last_n))
done
if [ -n "$inc" ]; then
  run TestCcoreIncremental "$inc" CCINC "$norm" CCORE_INC_SET=$inc; total_inc=$((total_inc + last_n))
fi
run TestCcoreTables "$lang" CCTABLES CCORE_LANGS=$lang
# The exact-lexer record against the compiled C lexer: every lex state at the end
# of input and on lookahead -1, and every code point against the shipped blob.
if [ "$lang" = json ]; then run TestLexProbe json CCLEXPROBE LEXPROBE_LANG=json LEXPROBE_REQUIRE_RECORD=1 LEXPROBE_FULL=1 LEXPROBE_BLOB_STRICT=1; fi
run TestCcoreScannerIsolation "$lang" CCCOUNT "$norm" CCORE_LANG=$lang
# The value-node readers against C (every node of the language's samples: flags, positions,
# counts, children by index, field names, cursor field, supertype mask), fresh and after
# four ts_tree_edit edits. Each needs a positive node count; the fresh test also needs a
# positive count of children with a non-zero supertype mask (all four languages have supertypes).
extra_set="${lang}-masks"
run TestCcoreNodes "$lang" CCNODE "$norm" CCORE_LANG=$lang
extra_set=""
run TestCcoreNodesEdited "$lang" CCNODEEDIT "$norm" CCORE_LANG=$lang
if [ "$golden" = 1 ]; then run TestCcoreGolden json CCGOLDEN CCORE_LANG=$lang; fi
# Store tests cheap enough for CI. The timing runs, the 10,000-edit chains and the
# stopped-parse, handle and race tests are not run in CI.
# TestCcoreStoreChain: 300 edits that keep a generated Python file valid, 31 of
# them (step 1 and every 10th) compared with C (trace, digest, records, reuse, lexed and reduced counts).
if [ -n "$chain" ]; then
  run TestCcoreStoreChain "$chain" CCMEM "$norm" CCORE_CHAIN_LANG=$chain CCORE_CHAIN_STEPS=300 CCORE_CHAIN_SAMPLE=10
fi
# TestCcoreKeyword: the heap-keyword set (a generated TypeScript file whose keyword token
# is a heap leaf while several versions are alive), fresh and in 240 edits.
if [ -n "$kw" ]; then
  run TestCcoreKeyword "$kw" CCKWINC "$norm" CCORE_KW=1 CCORE_KW_LANG=$kw
fi

echo "SUMMARY language=$lang trace_lines=$total_trace digest_lines=$total_digest record_lines=$total_records incremental_lines=$total_inc failed=$failed"
# A run that skipped every input proves nothing.
if [ "$total_trace" -eq 0 ] || [ "$total_digest" -eq 0 ] || [ "$total_records" -eq 0 ]; then
  echo "no input was compared for $lang" >&2
  failed=1
fi
exit "$failed"
