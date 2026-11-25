#!/usr/bin/env bash
# Graduation gate for ONE grammar, in the parity container.
#
#   bash scripts/graduation_gate.sh [options] <language>
#   bash scripts/graduation_gate.sh --self-test
#
# Options
#   --report          report mode: run every requested item, print one line per
#                     item, exit 0. Without it the script is the
#                     gate: it exits 1 at the first item that does not pass and
#                     skips the later items.
#   --items 1,2,3     run only these items (default: 1 to 10)
#   --negative <n>    self-check: corrupt one expectation of item n; the item
#                     must then fail (sets GRAD_NEGATIVE=<n>)
#   --no-race         skip the -race build of item 8
#   --candidate <p>   with --report --census only: item 0 treats the grammar as certified
#                     with a record that lists the scan providers <p> (comma-separated,
#                     for example table-v1; "none" lists no provider). A grammar whose
#                     registry entry has a token-source factory must then be served on
#                     every route that uses it; with "none" item 0 fails and names the route.
#   --census          with --report only: run the items for a grammar that has
#                     no certificate. Item 0 is still printed as it is, and
#                     the other items run under the test certification. The
#                     gate itself never runs without a certificate.
#
# Output: one line per item
#   GRAD <language> item=<n> <pass|fail|n/a> <numbers> [first failing input]
# A check that did not run prints n/a with the reason; n/a does not pass in
# gate mode.
# --report --census may set CCORE_VIEWS=0|1.
# All other runs reject CCORE_VIEWS and use the build default.
# GTS_ADMISSION_CANDIDATE is unset so it cannot change the default route.
#
# Items (tests in cgo_harness and admission_switch_test.go)
#   1 fresh parse equals pinned C on every input of the grammar's corpus
#   2 invariants: the 72-step session and 16 sites per edit class; incremental
#     equals fresh, ERROR root implies HasError, the root covers the input, a
#     reparse without edits allocates nothing
#   3 incremental parse equals C's on the session, and the core's records
#     (token lookahead bytes included) equal C's along C's own chain of reparses
#   4 declines during items 1 to 3: none, or each has an entry in
#     cgo_harness/testdata/graduation_deny.txt with a reopening condition
#   5 reuse equals C's subtree events, or a same-length views shortcut reuses all bytes and equals C's tree.
#   6 on the same history as pinned C: the core lexes C's tokens, its processed version
#     width equals C's largest version_count at a "process version" event, and its new heap
#     records are at most 1.10 x C's subtree creations + 16 (C's count comes from the oracle
#     built with the tag gts_subtree_counters: heap leaves, new nodes and clones during the
#     parse, without the comparison trees of select_children). A same-length shortcut that
#     returns the old tree keeps the rule: tokens between 1 and C's, tree equal to C's. The
#     default engine's tokens, new nodes and maximum live versions are printed, not gated.
#   7 highlight, tags, MISSING, ERROR and supertype query results (the set of
#     captures: name and byte range)
#   8 safety: stop reasons, a failing core and scanner, release orders, readers
#     (also under -race), store bound: over 300 edits the live record slots (handed out
#     and not free) settle by edit 100 (later edits add at most 5 percent) and stay at or
#     below 2.5 x the first tree's; page bytes are reported, not gated
#   9 peak resident set size of a 1 MiB fresh parse and of 1,000 edits, at or below the default
#     engine's; each run is a child process that resets the peak after its setup and
#     keeps its heap within 32 MiB of what it holds (a soft memory limit)
#   10 time, 20 paired rounds at 32 KiB, 137 KiB and 1 MiB, median at or below the default engine's
#
# Environment
#   GRAD_CORPUS_DIR   folder with <language>/<file> parity-corpus inputs (required by item 1)
#   GRAD_OUT          folder for the logs (default: a new temporary folder)
#   GRAD_RSS_RUNS     runs per arm for item 9 (default 3)
#   GRAD_RSS_TIMEOUT  seconds a run of item 9 may take before it is killed (default 480);
#                     a killed run reports the last progress line as a lower bound.
#                     The 1,000 edits are prepared once before the timed runs (not timed).
#
# The default engine in items 6, 9 and 10 is the present engine of the build under
# test with the route off, not a pinned release of it.
#
# Needs: go with cgo, the pinned C oracle (cgo_harness/docker/run_parity_in_docker.sh sets it up).
set -uo pipefail
unset CCORE_REPORT_ONLY CCORE_UPDATE_GOLDEN GRAD_NEGATIVE GRAD_CENSUS GRAD_CANDIDATE GRAD_CANDIDATE_PROVIDERS GTS_ADMISSION_CANDIDATE

# result_line <log> <item>: the last GRADRESULT line of an item in a log.
result_line() { grep -a "^GRADRESULT item=$2 " "$1" 2>/dev/null | tail -n 1; }

# field <line> <name>: the value of name=... (a quoted value keeps its spaces).
field() {
  local line=$1 name=$2
  local re
  re="(^|[[:space:]])${name}=\"(([^\"\\\\]|\\\\.)*)\""
  if [[ "$line" =~ $re ]]; then local v=${BASH_REMATCH[2]}; v=${v//\\\"/\"}; printf '%s' "${v//\\\\/\\}"; return; fi
  if [[ "$line" =~ (^|[[:space:]])${name}=([^[:space:]]*) ]]; then printf '%s' "${BASH_REMATCH[2]}"; fi
}

# numbers_of <line>: everything between status=... and first="...".
numbers_of() {
  local line=$1
  line=${line#*status=* }
  line=${line%% first=*}
  printf '%s' "$line"
}

# emit <language> <item> <status> <numbers> <first>: prints the report line
# and remembers the status. status is pass, fail or na.
LAST_STATUS=na
views=unknown
emit() {
  local lang=$1 item=$2 status=$3 numbers=$4 first=${5:-}
  LAST_STATUS=$status
  [ "$status" = na ] && status="n/a"
  case "$item" in
    8|9|10) [[ " $numbers " == *" source="* ]] || numbers="$numbers source=unavailable bytes=0" ;;
  esac
  [[ " $numbers " == *" views="* ]] || numbers="views=$views $numbers"
  if [ -n "$first" ]; then
    printf 'GRAD %s item=%s %s %s [%s]\n' "$lang" "$item" "$status" "$numbers" "$first"
  else
    printf 'GRAD %s item=%s %s %s\n' "$lang" "$item" "$status" "$numbers"
  fi
}

# emit_from_log <language> <item> <log> <exit code of the test>
# A test that exited non-zero before it printed its result line is a failure;
# a log without a result line and with a skip is n/a.
emit_from_log() {
  local lang=$1 item=$2 log=$3 rc=$4 line
  line=$(result_line "$log" "$item")
  if [ -z "$line" ]; then
    if grep -aq -- '--- SKIP' "$log"; then
      emit "$lang" "$item" na "no result" "the test was skipped"
    elif [ "$rc" -ne 0 ]; then
      emit "$lang" "$item" fail "no result" "the test did not finish (exit $rc): $(grep -a -m1 -E 'FAIL|panic|Fatal|fatal' "$log" | cut -c1-200)"
    else
      emit "$lang" "$item" na "no result" "the test printed no result line"
    fi
    return
  fi
  local status; status=$(field "$line" status)
  if [ "$item" = 1 ] && [ "${report:-0}" = 0 ] && [ "$status" = pass ] && [ "$(field "$line" corpus)" = none ]; then
    emit "$lang" "$item" na "$(numbers_of "$line")" "parity corpus is missing or empty"
    return
  fi
  case "$status" in pass|fail|na|skip|pending) ;; *) status=fail ;; esac
  local first; first=$(field "$line" first)
  if [ "$rc" -ne 0 ] && [ "$status" = pass ]; then
    status=fail
    first="test exited $rc: $(grep -a -m1 -E 'Error:|Error Trace:|FAIL|panic|fatal|Errorf' "$log" | cut -c1-200)"
  fi
  emit "$lang" "$item" "$status" "$(numbers_of "$line")" "$first"
}

# decline_check <language> <deny file> <log>...: item 4 on the GRADDECL lines of the logs.
# It prints "status<TAB>numbers<TAB>first".
decline_check() {
  local lang=$1 deny=$2; shift 2
  local seen=0 total=0 bad="" summary="" reason n entry missing=""
  local reasons=" "
  local log line list
  for log in "$@"; do
    [ -f "$log" ] || continue
    while IFS= read -r line; do
      entry=$(field "$line" item)
      case "$entry" in 1|2|3) ;; *) continue ;; esac
      seen=$((seen + 1)); kv_set have "$entry" 1
      list=$(field "$line" declines)
      [ -z "$list" ] && continue
      IFS=',' read -ra parts <<<"$list"
      for p in "${parts[@]}"; do
        reason=${p%%=*}; n=${p#*=}
        [[ "$reasons" == *" $reason "* ]] || reasons+="$reason "
        kv_set sum "$reason" $(( $(kv_get sum "$reason") + n ))
      done
    done < <(grep -a '^GRADDECL ' "$log")
  done
  if [ "$seen" -eq 0 ]; then printf 'na\tdeclines=unknown\tno decline counters (items 1 to 3 did not run)\n'; return; fi
  if [ "${GRAD_NEGATIVE:-}" = 4 ]; then [[ "$reasons" == *" negative_check "* ]] || reasons+="negative_check "; kv_set sum negative_check 1; fi
  for reason in $reasons; do
    total=$((total + $(kv_get sum "$reason")))
    summary+="${reason}=$(kv_get sum "$reason"),"
    entry=$(awk -F'\t' -v l="$lang" -v r="$reason" '$0 !~ /^#/ && $1 == l && $2 == r && $3 != "" {print $3; exit}' "$deny" 2>/dev/null)
    if [ -z "$entry" ] && [ -z "$bad" ]; then bad="$reason ($(kv_get sum "$reason") declines, no deny entry with a reopening condition)"; fi
  done
  summary=${summary%,}
  if [ -n "$bad" ]; then printf 'fail\tdeclines=%s\t%s\n' "$summary" "$bad"; return; fi
  # Item 4 speaks for items 1 to 3. A run that left one of them out cannot pass it.
  for entry in 1 2 3; do [ -n "$(kv_get have "$entry")" ] || missing+="$entry "; done
  if [ -n "$missing" ]; then printf 'na\tdeclines=%s counters_read=%s\tno decline counters of item(s) %s\n' "${summary:-0}" "$seen" "${missing% }"; return; fi
  if [ "$total" -eq 0 ]; then printf 'pass\tdeclines=0 counters_read=%s\t\n' "$seen"; return; fi
  printf 'pass\tdeclines=%s all_denied=true\t\n' "$summary"
}

# rss_child_na reads an unavailable workload without mistaking it for a crash.
rss_child_na() {
  local line=$1
  [ "$(field "$line" status)" = na ] || return 1
  printf '%s' "$(field "$line" reason)"
}

# source_child_status accepts only the statuses emitted by source preparation.
source_child_status() {
  local status
  status=$(field "$1" status)
  case "$status" in pass|na) printf '%s' "$status" ;; *) printf '%s' invalid ;; esac
}

# median <numbers...>
median() { printf '%s\n' "$@" | sort -n | awk '{a[NR]=$1} END {if (NR) print a[int((NR+1)/2)]}'; }

# kv_set <table> <key> <value> and kv_get <table> <key>: a small map kept in
# variables named kv_<table>_<key>. bash 3.2 (macOS) has no associative arrays.
# A key keeps its letters, digits and underscores; any other character becomes _.
kv_name() { printf 'kv_%s_%s' "$1" "${2//[^A-Za-z0-9_]/_}"; }
kv_set() { local n; n=$(kv_name "$1" "$2"); printf -v "$n" '%s' "$3"; }
kv_get() { local n; n=$(kv_name "$1" "$2"); printf '%s' "${!n:-}"; }

# run_killable <seconds> <command...>: runs the command and kills it (SIGKILL)
# after the time limit; the exit code is 137 for a killed command, as with
# "timeout -s KILL". macOS has no timeout(1), so a watcher process does the job.
run_killable() {
  local secs=$1; shift
  if command -v timeout >/dev/null 2>&1; then timeout -s KILL "$secs" "$@"; return; fi
  local pid wpid rc
  "$@" &
  pid=$!
  ( sleep "$secs"; kill -KILL "$pid" 2>/dev/null ) &
  wpid=$!
  wait "$pid"; rc=$?
  pkill -P "$wpid" 2>/dev/null
  kill "$wpid" 2>/dev/null
  wait "$wpid" 2>/dev/null
  return "$rc"
}

if [ "${1:-}" = "--self-test" ]; then
  d=$(mktemp -d); trap 'rm -rf "$d"' EXIT
  fails=0
  ok() { if [ "$2" = "$3" ]; then echo "ok   self-test: $1"; else echo "FAIL self-test: $1 (got '$2', want '$3')"; fails=1; fi; }
  printf '%s\n' 'GRADRESULT item=1 status=pass inputs=3 served=3 first=""' >"$d/pass.log"
  printf '%s\n' 'GRADRESULT item=1 status=fail inputs=3 served=3 first="a.json: 2 differences"' >"$d/fail.log"
  printf '%s\n' 'GRADRESULT item=1 status=na inputs=3 served=0 first="the route declined every input"' >"$d/na.log"
  printf '%s\n' '--- SKIP: TestGraduationFresh (0.00s)' >"$d/skip.log"
  printf '%s\n' 'panic: boom' >"$d/crash.log"
  printf '%s\n' 'GRADRESULT item=1 status=pass inputs=3 first=""' >"$d/passrc.log"
  ok "RSS child unavailable reason" "$(rss_child_na 'GRADRSS status=na reason="only 3 clean edits"')" "only 3 clean edits"
  ok "RSS child success is not unavailable" "$(rss_child_na 'GRADRSS status=pass' || printf not-na)" "not-na"
  ok "source preparation pass" "$(source_child_status 'GRADSOURCE status=pass')" "pass"
  ok "source preparation unavailable" "$(source_child_status 'GRADSOURCE status=na')" "na"
  ok "source preparation invalid" "$(source_child_status 'GRADSOURCE status=unknown')" "invalid"
  ok "pass line" "$(emit_from_log x 1 "$d/pass.log" 0 | cut -d' ' -f4)" "pass"
  ok "fail line keeps the first failing input" "$(emit_from_log x 1 "$d/fail.log" 0 | grep -c 'a.json: 2 differences')" "1"
  ok "fail line status" "$(emit_from_log x 1 "$d/fail.log" 0 | cut -d' ' -f4)" "fail"
  ok "n/a line prints n/a" "$(emit_from_log x 1 "$d/na.log" 0 | cut -d' ' -f4)" "n/a"
  ok "skipped test is n/a" "$(emit_from_log x 1 "$d/skip.log" 0 | cut -d' ' -f4)" "n/a"
  ok "crash is a failure" "$(emit_from_log x 1 "$d/crash.log" 2 | cut -d' ' -f4)" "fail"
  ok "pass line with a non-zero exit is a failure" "$(emit_from_log x 1 "$d/passrc.log" 1 | cut -d' ' -f4)" "fail"
  ok "non-zero exit explains the failure" "$(emit_from_log x 1 "$d/passrc.log" 1 | grep -c 'test exited 1')" "1"
  printf '%s\n' 'GRADRESULT item=5 status=pass views=true steps=1 first=""' >"$d/views.log"
  ok "result keeps its views arm" "$(emit_from_log x 5 "$d/views.log" 0 | grep -c 'views=true')" "1"
  printf '%s\n' 'GRADRESULT item=5 status=skip steps=72 first="fresh-only mode"' >"$d/skip5.log"
  ok "skip line prints skip" "$(emit_from_log x 5 "$d/skip5.log" 0 | cut -d' ' -f4)" "skip"
  ok "wrong item number finds no result" "$(emit_from_log x 2 "$d/pass.log" 0 | cut -d' ' -f4)" "n/a"
  printf '%s\n' 'GRADDECL item=1 served_fresh=3 served_incr=0 declines=' 'GRADDECL item=2 served_fresh=3 served_incr=5 declines=' 'GRADDECL item=3 served_fresh=1 served_incr=5 declines=' 'GRADRESULT item=2 status=pass reparses_served=5 first=""' >"$d/d0.log"
  printf '%s\n' 'GRADDECL item=1 served_fresh=3 served_incr=0 declines=' 'GRADDECL item=2 served_fresh=3 served_incr=0 declines=' 'GRADDECL item=3 served_fresh=1 served_incr=0 declines=' 'GRADRESULT item=2 status=pass reparses_served=0 first=""' >"$d/dz.log"
  printf '%s\n' 'GRADDECL item=1 served_fresh=2 served_incr=0 declines=old_tree_edit_mismatch=2' 'GRADDECL item=2 served_fresh=1 served_incr=5 declines=' 'GRADDECL item=3 served_fresh=1 served_incr=5 declines=' 'GRADRESULT item=2 status=pass reparses_served=5 first=""' >"$d/d1.log"
  printf '%s\n' 'GRADDECL item=2 served_fresh=1 served_incr=5 declines=' 'GRADRESULT item=2 status=pass reparses_served=5 first=""' >"$d/dmissing.log"
  printf '# header\nx\told_tree_edit_mismatch\tafter the edit check is exact\n' >"$d/deny"
  printf '# header\nx\told_tree_edit_mismatch\t\n' >"$d/deny_noreopen"
  ok "no declines pass" "$(decline_check x "$d/deny" "$d/d0.log" | cut -f1)" "pass"
  ok "no declines with fresh only" "$(decline_check x "$d/deny" "$d/dz.log" | cut -f1)" "pass"
  ok "decline without a deny entry fails" "$(decline_check x /dev/null "$d/d1.log" | cut -f1)" "fail"
  ok "decline with a deny entry passes" "$(decline_check x "$d/deny" "$d/d1.log" | cut -f1)" "pass"
  ok "deny entry without a reopening condition fails" "$(decline_check x "$d/deny_noreopen" "$d/d1.log" | cut -f1)" "fail"
  ok "deny entry of another language fails" "$(decline_check y "$d/deny" "$d/d1.log" | cut -f1)" "fail"
  printf '%s\n' 'GRADDECL item=1 served_fresh=2 declines=a=2,b=1' 'GRADDECL item=2 served_fresh=1 declines=a=3' 'GRADDECL item=3 served_fresh=1 declines=' >"$d/dmulti.log"
  ok "declines of several reasons add up" "$(decline_check x /dev/null "$d/dmulti.log" | cut -f2)" "declines=a=5,b=1"
  ok "no counters is n/a" "$(decline_check x "$d/deny" "$d/pass.log" | cut -f1)" "na"
  ok "counters of one item only is n/a" "$(decline_check x "$d/deny" "$d/dmissing.log" | cut -f1)" "na"
  ok "the n/a names the missing items" "$(decline_check x "$d/deny" "$d/dmissing.log" | cut -f3)" "no decline counters of item(s) 1 3"
  printf '%s\n' 'GRADRESULT item=2 status=pass steps=2 served_incremental=1 fresh_reasons=old_has_error:1 first=""' >"$d/fallback.log"
  ok "fallback counts kept" "$(emit_from_log x 2 "$d/fallback.log" 0 | grep -c 'fresh_reasons=old_has_error:1')" "1"
  printf '%s\n' 'GRADRESULT item=7 status=pass omitted=tags comparisons=2 first=""' >"$d/query.log"
  ok "omitted query kept" "$(emit_from_log x 7 "$d/query.log" 0 | grep -c 'omitted=tags')" "1"
  printf '%s\n' 'GRADRESULT item=1 status=pass corpus=none first=""' >"$d/corpus.log"
  ok "missing corpus in gate" "$(report=0; emit_from_log x 1 "$d/corpus.log" 0 | cut -d' ' -f4)" "n/a"
  ok "missing corpus in report" "$(report=1; emit_from_log x 1 "$d/corpus.log" 0 | cut -d' ' -f4)" "pass"
  ok "median" "$(median 5 1 3)" "3"
  printf '%s\n' 'GRADRESULT item=0 status=fail blob=ab modes=0 need=3 first="not served"' >"$d/served.log"
  printf '%s\n' 'GRADRESULT item=10 status=pending rounds=20 first="same-length cell pending"' >"$d/pend.log"
  ok "a pending cell prints pending" "$(emit_from_log x 10 "$d/pend.log" 0 | cut -d' ' -f4)" "pending"
  out=$(env -u CCORE_VIEWS -u GRAD_CENSUS -u GRAD_NEGATIVE GRAD_SELF_TEST_STATUS=pending bash "$0" --self-test-gate 2>&1); rc=$?
  ok "gate mode does not exit 0 while a cell is pending" "$rc" "1"
  ok "an uncertified grammar fails item 0" "$(emit_from_log x 0 "$d/served.log" 0 | cut -d' ' -f4)" "fail"
  printf '%s\n' 'GRADRESULT item=1 status=fail n=1 first="has_error go=\"x\" c=1"' >"$d/quote.log"
  ok "a quoted value keeps its inner quotes" "$(emit_from_log x 1 "$d/quote.log" 0 | grep -c 'has_error go="x" c=1')" "1"
  # gate mode: a failing item stops the run
  out=$(env -u CCORE_VIEWS -u GRAD_CENSUS -u GRAD_NEGATIVE GRAD_SELF_TEST_STATUS=fail bash "$0" --self-test-gate 2>&1); rc=$?
  ok "gate mode exits non-zero at the first failure" "$rc" "1"
  ok "gate mode skips the later items" "$(grep -c 'item=2' <<<"$out")" "0"
  out=$(env -u CCORE_VIEWS -u GRAD_CENSUS -u GRAD_NEGATIVE GRAD_SELF_TEST_STATUS=na bash "$0" --self-test-gate 2>&1); rc=$?
  ok "gate mode does not pass n/a" "$rc" "1"
  out=$(env -u CCORE_VIEWS -u GRAD_CENSUS -u GRAD_NEGATIVE GRAD_SELF_TEST_STATUS=fail bash "$0" --report --self-test-gate 2>&1); rc=$?
  ok "report mode exits 0" "$rc" "0"
  ok "report mode runs the later items" "$(grep -c 'item=2' <<<"$out")" "1"
  out=$(env -u CCORE_VIEWS -u GRAD_CENSUS -u GRAD_NEGATIVE bash "$0" --census json 2>&1); rc=$?
  ok "--census without --report is refused" "$rc" "2"
  out=$(env -u CCORE_VIEWS -u GRAD_CENSUS -u GRAD_NEGATIVE bash "$0" --report --candidate table-v1 json 2>&1); rc=$?
  ok "--candidate without --census is refused" "$rc" "2"
  out=$(env -u CCORE_VIEWS -u GRAD_CENSUS -u GRAD_NEGATIVE bash "$0" --items 0 json 2>&1); rc=$?
  ok "--items without --report is refused" "$rc" "2"
  ok "--items refusal message" "$out" "graduation_gate.sh: --items needs --report (a gate run runs every item)"
  out=$(env -u CCORE_VIEWS -u GRAD_CENSUS -u GRAD_NEGATIVE CCORE_VIEWS=0 bash "$0" --report json 2>&1); rc=$?
  ok "CCORE_VIEWS without --census is refused" "$rc" "2"
  ok "CCORE_VIEWS refusal message" "$out" "graduation_gate.sh: CCORE_VIEWS is for --report --census; the gate measures the default views switch"
  out=$(env -u CCORE_VIEWS -u GRAD_CENSUS -u GRAD_NEGATIVE bash "$0" --report --items 0 --self-test-gate 2>&1); rc=$?
  ok "--report --items 0 is accepted" "$rc" "0"
  ok "--report --items 0 reports item 0" "$(grep -c 'item=0' <<<"$out")" "1"
  if [ "${GRAD_SELF_TEST_NESTED:-0}" != 1 ]; then
    out=$(env -u GRAD_CENSUS -u GRAD_NEGATIVE CCORE_VIEWS=1 GRAD_SELF_TEST_NESTED=1 bash "$0" --self-test 2>&1); rc=$?
    ok "self-test with CCORE_VIEWS exported" "$rc" "0"
  fi
  exit "$fails"
fi

report=0; census=0; candidate=""; items="1,2,3,4,5,6,7,8,9,10"; negative=""; race=1; selftestgate=0; lang=""; items_set=0
while [ $# -gt 0 ]; do
  case "$1" in
    --report) report=1 ;;
    --items) items=$2; items_set=1; shift ;;
    --negative) negative=$2; shift ;;
    --no-race) race=0 ;;
    --census) census=1 ;;
    --candidate) candidate=$2; shift ;;
    --self-test-gate) selftestgate=1 ;;
    -*) echo "graduation_gate.sh: unknown option $1" >&2; exit 2 ;;
    *) lang=$1 ;;
  esac
  shift
done

# finish <exit code of this script when no item stopped the run>
# stop_if_needed: gate mode stops at the first item that does not pass.
stop_if_needed() { if [ "$report" = 0 ] && [ "$LAST_STATUS" != pass ]; then echo "graduation_gate.sh: item $1 did not pass; later items skipped" >&2; exit 1; fi; }

if [ "$census" = 1 ] && [ "$report" = 0 ]; then echo "graduation_gate.sh: --census needs --report" >&2; exit 2; fi
if [ "$items_set" = 1 ] && [ "$report" = 0 ]; then echo "graduation_gate.sh: --items needs --report (a gate run runs every item)" >&2; exit 2; fi
if [ "$census" = 0 ] && [ "${CCORE_VIEWS+x}" = x ]; then echo "graduation_gate.sh: CCORE_VIEWS is for --report --census; the gate measures the default views switch" >&2; exit 2; fi
if [ "$census" = 1 ]; then export GRAD_CENSUS=1; fi
if [ -n "$candidate" ]; then
  if [ "$census" = 0 ]; then echo "graduation_gate.sh: --candidate needs --report --census" >&2; exit 2; fi
  export GRAD_CANDIDATE=1
  if [ "$candidate" = none ]; then export GRAD_CANDIDATE_PROVIDERS=; else export GRAD_CANDIDATE_PROVIDERS=$candidate; fi
fi

if [ "$selftestgate" = 1 ]; then
  if [ "$items_set" = 1 ] && [ "$items" = 0 ]; then emit selftest 0 pass "numbers" ""; exit 0; fi
  for i in 1 2; do
    emit selftest "$i" "${GRAD_SELF_TEST_STATUS:-pass}" "numbers" ""
    stop_if_needed "$i"
  done
  exit 0
fi

[ -n "$lang" ] || { echo "usage: graduation_gate.sh [--report [--census [--candidate providers]]] [--items list] [--negative n] [--no-race] <language> | --self-test" >&2; exit 2; }
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
if [ -n "${GRAD_OUT:-}" ]; then
  mkdir -p "$GRAD_OUT" || exit 2
  out=$(mktemp -d "$GRAD_OUT/gate.XXXXXXXX") || exit 2
else
  out=$(mktemp -d) || exit 2
fi
trap 'printf "graduation_gate.sh: logs=%s\n" "$out"' EXIT
if [[ ! "$items" =~ ^([0-9]|10)(,([0-9]|10))*$ ]]; then echo "graduation_gate.sh: invalid item list: $items" >&2; exit 2; fi
if [ -n "$negative" ] && [[ ! "$negative" =~ ^([0-9]|10)$ ]]; then echo "graduation_gate.sh: invalid negative item: $negative" >&2; exit 2; fi
export GOWORK=off GOMAXPROCS=${GOMAXPROCS:-1} GRAD_LANG=$lang
[ -n "$negative" ] && export GRAD_NEGATIVE=$negative
want() { [[ ",$items," == *",$1,"* ]]; }

hbin="$out/harness.test"; rbin="$out/root.test"; rrace="$out/root.race.test"
need_harness=1
if [ "$need_harness" = 1 ] && [ ! -x "$hbin" ]; then
  if ! (cd "$root/cgo_harness" && go test -c -vet=off -tags 'cgo treesitter_c_parity gts_ccore_test_certify gts_subtree_counters' -o "$hbin" .) >"$out/build-harness.log" 2>&1; then
    emit "$lang" 0 fail "build failed" "$(tail -n 3 "$out/build-harness.log" | tr '\n' ' ' | cut -c1-200)"
    if [ "$report" = 1 ]; then
      for i in $(tr ',' ' ' <<<"$items"); do [ "$i" = 0 ] || emit "$lang" "$i" na "not run" "item 0 build failed"; done
    fi
    [ "$report" = 1 ] && exit 0 || exit 1
  fi
fi
cd "$root/cgo_harness" || exit 2
export GTS_PARITY_IN_DOCKER=${GTS_PARITY_IN_DOCKER:-1}

# run_harness <item> <test>: runs a harness test; sets RC and the log path in LOG.
run_harness() {
  LOG="$out/item$1.log"
  "$hbin" -test.run "^$2\$" -test.count=1 -test.v -test.timeout=40m >"$LOG" 2>&1
  RC=$?
}

# Item 0: the core serves the grammar by default (a certificate for the blob).
# Without a certificate the other items run only when --census asks for them.
run_harness 0 TestGraduationServed
observed_views=$(field "$(result_line "$LOG" 0)" views)
case "$observed_views" in true|false) views=$observed_views ;; esac
emit_from_log "$lang" 0 "$LOG" $RC
if [ "$LAST_STATUS" != pass ] && [ "$census" = 0 ]; then
  if [ "$report" = 1 ]; then
    for i in $(tr ',' ' ' <<<"$items"); do [ "$i" = 0 ] || emit "$lang" "$i" na "not run" "item 0 did not pass: the grammar is not served by the core by default"; done
    exit 0
  fi
  echo "graduation_gate.sh: item 0 did not pass; all items skipped" >&2; exit 1
fi

for item in 1 2 3 4 5 6 7 8 9 10; do
  want $item || continue
  case $item in
    1) run_harness 1 TestGraduationFresh; emit_from_log "$lang" 1 "$LOG" $RC ;;
    2) run_harness 2 TestGraduationInvariants; emit_from_log "$lang" 2 "$LOG" $RC ;;
    3) run_harness 3 TestGraduationIncremental; emit_from_log "$lang" 3 "$LOG" $RC ;;
    4)
      res=$(decline_check "$lang" "$root/cgo_harness/testdata/graduation_deny.txt" "$out/item1.log" "$out/item2.log" "$out/item3.log")
      IFS=$'\t' read -r st nums first <<<"$res"
      emit "$lang" 4 "$st" "$nums" "$first" ;;
    5) run_harness 5 TestGraduationReuse; emit_from_log "$lang" 5 "$LOG" $RC ;;
    6) run_harness 6 TestGraduationCounters; emit_from_log "$lang" 6 "$LOG" $RC ;;
    7) run_harness 7 TestGraduationQueries; emit_from_log "$lang" 7 "$LOG" $RC; grep -a '^GRADNOTE item=7 ' "$LOG" || : ;;
    8)
      source_file="$out/item8-source-$$"
      edits_file="$out/item8-edits-$$"
      GRAD_SOURCE_FILE="$source_file" GRAD_EDITS_FILE="$edits_file" "$hbin" -test.run '^TestGraduationStoreSource$' -test.count=1 -test.v -test.timeout=40m >"$out/item8-source.log" 2>&1
      source_rc=$?
      source_line=$(grep -a '^GRADSOURCE ' "$out/item8-source.log" | tail -n 1)
      if [ "$source_rc" -ne 0 ] || [ -z "$source_line" ]; then
        emit "$lang" 8 fail "source=unknown bytes=0" "C source preparation failed (exit $source_rc)"
        stop_if_needed 8
        continue
      fi
      source_name=$(field "$source_line" source)
      source_bytes=$(field "$source_line" bytes)
      source_status=$(source_child_status "$source_line")
      if [ "$source_status" = invalid ]; then
        emit "$lang" 8 fail "source=unknown bytes=0" "C source preparation reported an invalid status"
        stop_if_needed 8
        continue
      fi
      [ "$source_status" = pass ] || source_file=""
      export GRAD_SOURCE_REQUIRED=1 GRAD_SOURCE_FILE="$source_file" GRAD_SOURCE_NAME="$source_name" GRAD_EDITS_FILE="$edits_file"
      cd "$root" || exit 2
      go test -c -vet=off -tags gts_ccore_test_certify -o "$rbin" . >"$out/build-root.log" 2>&1 || { emit "$lang" 8 fail "source=$source_name bytes=$source_bytes build=failed" "$(tail -n 3 "$out/build-root.log" | tr '\n' ' ' | cut -c1-200)"; cd "$root/cgo_harness" || exit 2; stop_if_needed 8; continue; }
      "$rbin" -test.run '^TestGraduationSafety$' -test.count=1 -test.v -test.timeout=40m >"$out/item8.log" 2>&1; rc=$?
      safety_line=$(emit_from_log "$lang" 8 "$out/item8.log" $rc)
      safety_status=$(cut -d' ' -f4 <<<"$safety_line")
      if [ "$race" = 1 ]; then
        go test -race -c -vet=off -tags gts_ccore_test_certify -o "$rrace" . >"$out/build-race.log" 2>&1
        if [ -x "$rrace" ]; then
          "$rrace" -test.run '^TestGraduationSafety$' -test.count=1 -test.v -test.timeout=40m >"$out/item8-race.log" 2>&1; rrc=$?
          if [ "$rrc" -ne 0 ] || grep -aq 'WARNING: DATA RACE' "$out/item8-race.log"; then
            safety_line="GRAD $lang item=8 fail views=$views source=$source_name bytes=$source_bytes race_run=failed [$(grep -a -m1 -E 'DATA RACE|FAIL|GRADSAFE.*fail' "$out/item8-race.log" | cut -c1-200)]"
            safety_status=fail
          else
            race_status=$(field "$(result_line "$out/item8-race.log" 8)" status)
            if [ "$race_status" != pass ] && [ "$safety_status" != fail ]; then safety_status=na; safety_line="GRAD $lang item=8 n/a views=$views source=$source_name bytes=$source_bytes race_status=$race_status [race checks did not pass]"; fi
          fi
        else
          if [ "$safety_status" != fail ]; then
            safety_status=na
            safety_line="GRAD $lang item=8 n/a views=$views source=$source_name bytes=$source_bytes race_build=failed [$(tail -n 2 "$out/build-race.log" | tr '\n' ' ' | cut -c1-200)]"
          fi
        fi
      fi
      if [ "$race" != 1 ] && [ "$safety_status" = pass ]; then safety_status=na; safety_line="GRAD $lang item=8 n/a views=$views source=$source_name bytes=$source_bytes race=not_run [race check is required]"; fi
      printf '%s\n' "$safety_line"
      LAST_STATUS=$safety_status
      cd "$root/cgo_harness" || exit 2
      unset GRAD_SOURCE_REQUIRED GRAD_SOURCE_FILE GRAD_SOURCE_NAME GRAD_EDITS_FILE
      ;;
    9)
      runs=${GRAD_RSS_RUNS:-3}
      if [[ ! "$runs" =~ ^[1-9][0-9]*$ ]]; then emit "$lang" 9 fail "runs=$runs" "runs must be positive"; stop_if_needed 9; continue; fi
      fail9=""; na9=""; source9=""; bytes9=""
      # Prepare the 1,000 edits once, outside the timed runs: checking each edit
      # with pinned C on a 1 MiB source can take most of a run's time limit.
      edits9="$out/item9-edits.json"
      GRAD_RSS_PREPARE=1 GRAD_RSS_EDITS_FILE="$edits9" GRAD_RSS_ARM=route GRAD_RSS_WORK=edits1000 "$hbin" -test.run '^TestGraduationRSSChild$' -test.count=1 -test.v -test.timeout=40m >"$out/item9-prepare.log" 2>&1
      if grep -aq '^GRADRSSPREP status=pass' "$out/item9-prepare.log"; then export GRAD_RSS_EDITS_FILE="$edits9"; else unset GRAD_RSS_EDITS_FILE; fi
      for r in $(seq 1 "$runs"); do
        for work in fresh edits1000; do
          arms="route default"; [ $((r % 2)) = 0 ] && arms="default route"
          for arm in $arms; do
            GRAD_RSS_ARM=$arm GRAD_RSS_WORK=$work run_killable "${GRAD_RSS_TIMEOUT:-480}" "$hbin" -test.run '^TestGraduationRSSChild$' -test.count=1 -test.v -test.timeout=40m >"$out/item9-$work-$arm-$r.log" 2>&1
            child_rc=$?
            line=$(grep -a '^GRADRSS ' "$out/item9-$work-$arm-$r.log" | tail -n 1)
            kb=$(field "$line" maxrss_kb)
            child_na=$(rss_child_na "$line" || true)
            if [ -n "$line" ]; then source9=$(field "$line" source); bytes9=$(field "$line" bytes); fi
            if [ "$child_rc" -eq 0 ] && [ -n "$child_na" ]; then
              na9="$work/$arm: $child_na"
            elif [ "$child_rc" -ne 0 ] && [ "$child_rc" -ne 124 ] && [ "$child_rc" -ne 137 ]; then
              fail9="$work/$arm run=$r exit=$child_rc: $(tail -n 2 "$out/item9-$work-$arm-$r.log" | tr '\n' ' ')"
            elif [ -z "$kb" ]; then
              # A killed process can only report a lower bound.
              pl=$(grep -a '^GRADPROG ' "$out/item9-$work-$arm-$r.log" | tail -n 1)
              kb=$(field "$pl" maxrss_kb)
              if [ -z "$kb" ]; then fail9="$work/$arm run $r gave no maximum resident set size"; else kv_set rss "$work.$arm" "$(kv_get rss "$work.$arm")$kb "; kv_set killed "$work.$arm" "$(kv_get killed "$work.$arm")$r(after $(field "$pl" edits) edits) "; fi
            else kv_set rss "$work.$arm" "$(kv_get rss "$work.$arm")$kb "; fi
          done
        done
      done
      if [ -n "$fail9" ]; then emit "$lang" 9 fail "runs=$runs source=$source9 bytes=$bytes9" "$fail9"; elif [ -n "$na9" ]; then emit "$lang" 9 na "runs=$runs source=$source9 bytes=$bytes9" "$na9"; else
        status=pass; first=""; nums="runs=$runs source=$source9 bytes=$bytes9"
        for work in fresh edits1000; do
          rss_route=$(kv_get rss "$work.route"); rss_default=$(kv_get rss "$work.default")
          # The values are plain numbers: word splitting gives one argument each.
          mr=$(median $rss_route); ml=$(median $rss_default)
          [ "${GRAD_NEGATIVE:-}" = 9 ] && mr=$((ml + 1))
          if [ -n "$(kv_get killed "$work.route")" ]; then status=na; first="$work: route did not finish; its $mr KB value is a lower bound"; fi
          nums+=" ${work}_1MiB_route=${mr}KB(${rss_route% }) ${work}_1MiB_default=${ml}KB(${rss_default% })"
          for arm in route default; do
            killed_runs=$(kv_get killed "$work.$arm")
            [ -n "$killed_runs" ] && nums+=" ${work}_${arm}_killed_runs=${killed_runs% }(values are lower bounds)"
          done
          if [ "$mr" -gt "$ml" ]; then
            if [ -n "$(kv_get killed "$work.default")" ]; then [ "$status" = fail ] || status=na; [ -z "$first" ] && first="$work: default engine was killed; its lower bound $ml KB is below the route's $mr KB"
            else status=fail; [ -z "$first" ] && first="$work: route median $mr KB, default engine $ml KB"; fi
          fi
        done
        emit "$lang" 9 "$status" "$nums" "$first"
      fi
      unset GRAD_RSS_EDITS_FILE ;;
    10) run_harness 10 TestGraduationTime; emit_from_log "$lang" 10 "$LOG" $RC ;;
  esac
  stop_if_needed "$item"
done
exit 0
