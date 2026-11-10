#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
legs="$script_dir/editor_latency_legs.sh"
fixtures="$script_dir/../../cgo_harness/editor_latency/fixtures.json"
dir=$(mktemp -d "${TMPDIR:-/tmp}/editor-latency-legs-test.XXXXXX")
trap 'rm -rf -- "$dir"' EXIT
cases=0
fail() {
	printf 'not ok - %s\n' "$1" >&2
	exit 1
}

langs=$(jq -r '(.languages + .supplemental_languages)[]' "$fixtures")
# jobs_json <jq filter applied to the list of jobs>
jobs_json() {
	jq -n --arg langs "$langs" '
		[ ($langs | split("\n")[]) | {name: "editor_latency (\(.))", status: "completed", conclusion: "success"} ]
		| '"$1"'
		| {total_count: length, jobs: .}'
}
# expect <name> <0|1> <jobs file>
expect() {
	local rc=0
	bash "$legs" "$3" "$fixtures" > "$dir/out" 2>&1 || rc=$?
	[[ $rc == "$2" ]] || fail "$1: exit $rc, want $2 ($(cat "$dir/out"))"
	cases=$((cases + 1))
	printf 'ok - %s\n' "$1"
}

jobs_json '.' > "$dir/ok.json"
expect 'all ten succeed' 0 "$dir/ok.json"
[[ $(jq '.jobs | length' "$dir/ok.json") == 10 ]] || fail 'fixture must hold ten languages'

jobs_json '. + [{name: "build", status: "in_progress", conclusion: null}]' > "$dir/extra.json"
expect 'unrelated jobs are ignored' 0 "$dir/extra.json"

jobs_json 'map(select(.name != "editor_latency (go)"))' > "$dir/missing.json"
expect 'one missing' 1 "$dir/missing.json"

jobs_json 'map(if .name == "editor_latency (php)" then .conclusion = "cancelled" | .steps = [] else . end)' > "$dir/cancelled.json"
expect 'one cancelled with no steps' 1 "$dir/cancelled.json"

jobs_json 'map(if .name == "editor_latency (bash)" then .conclusion = "skipped" else . end)' > "$dir/skipped.json"
expect 'one skipped' 1 "$dir/skipped.json"

jobs_json 'map(if .name == "editor_latency (tsx)" then .conclusion = "failure" else . end)' > "$dir/failed.json"
expect 'one failed' 1 "$dir/failed.json"

jobs_json 'map(if .name == "editor_latency (java)" then .status = "in_progress" | .conclusion = null else . end)' > "$dir/running.json"
expect 'one still running' 1 "$dir/running.json"

jobs_json '. + [map(select(.name == "editor_latency (python)"))[0]]' > "$dir/dup.json"
expect 'one duplicated' 1 "$dir/dup.json"

jobs_json '. + [(map(select(.name == "editor_latency (c_sharp)"))[0] | .conclusion = "failure")]' > "$dir/rerun.json"
expect 'failed job rerun and now success: two entries fail' 1 "$dir/rerun.json"

jobs_json 'map(select(.name != "editor_latency (go)")) + [{name: "editor_latency (golang)", status: "completed", conclusion: "success"}]' > "$dir/lookalike.json"
expect 'a similar name does not count' 1 "$dir/lookalike.json"

# Pages: one object per page, as gh --paginate prints them.
jq '{total_count: .total_count, jobs: .jobs[:6]}' "$dir/ok.json" > "$dir/pages.json"
jq '{total_count: .total_count, jobs: .jobs[6:]}' "$dir/ok.json" >> "$dir/pages.json"
expect 'two pages together succeed' 0 "$dir/pages.json"
jq '{total_count: .total_count, jobs: .jobs[:6]}' "$dir/ok.json" > "$dir/page1.json"
expect 'a missing second page fails' 1 "$dir/page1.json"

echo '{"message":"Resource not accessible","status":"403"}' > "$dir/error.json"
expect 'an API error body fails' 1 "$dir/error.json"
: > "$dir/empty.json"
expect 'an empty file fails' 1 "$dir/empty.json"
echo 'not json' > "$dir/bad.json"
expect 'invalid JSON fails' 1 "$dir/bad.json"
expect 'a missing file fails' 1 "$dir/none.json"

echo '{"languages":[],"supplemental_languages":[]}' > "$dir/nolangs.json"
rc=0; bash "$legs" "$dir/ok.json" "$dir/nolangs.json" > /dev/null 2>&1 || rc=$?
[[ $rc == 1 ]] || fail 'an empty expected list must fail'
cases=$((cases + 1))
printf 'ok - an empty expected list fails\n'
rc=0; bash "$legs" > /dev/null 2>&1 || rc=$?
[[ $rc == 1 ]] || fail 'missing arguments must fail'
cases=$((cases + 1))
printf '%d cases passed\n' "$cases"
