#!/usr/bin/env bash
# Check that every editor-latency language job of this run attempt succeeded.
#
# Usage: editor_latency_legs.sh <jobs-json> <fixtures-json>
#
# <jobs-json> is the output of
#   gh api --paginate repos/<repo>/actions/runs/<run id>/attempts/<attempt>/jobs
# (one JSON object per page, all pages). <fixtures-json> is
# cgo_harness/editor_latency/fixtures.json; the expected languages are its
# "languages" followed by "supplemental_languages".
#
# Exits 0 only if, for every expected language, exactly one job named
# "editor_latency (<language>)" exists, is completed, and has conclusion
# "success". It also fails when a page's total_count differs from the jobs read,
# when an input is unreadable, or on any other error. The matrix aggregate
# (needs.editor_latency.result) is not enough on its own: a language job that
# is cancelled before it gets a runner can leave the aggregate at success.
#
# Two entries with one name fail. After "re-run failed jobs" the attempt's list
# holds every job exactly once, the jobs that were not re-run with their earlier
# conclusion (measured on run 42687308051, attempts 1 and 2, 10 November 2026),
# so a repeated name is never expected.

set -euo pipefail

fail() {
	echo "editor_latency legs: $1" >&2
	exit 1
}

jobs_file=${1-}
fixtures_file=${2-}
[[ -n $jobs_file && -n $fixtures_file ]] || fail 'usage: editor_latency_legs.sh <jobs-json> <fixtures-json>'
[[ -r $jobs_file && -r $fixtures_file ]] || fail 'an input is unreadable'
command -v jq > /dev/null || fail 'jq is required'

expected=$(jq -er '(.languages + .supplemental_languages) | if length > 0 and all(type == "string") then .[] else error("no languages") end' "$fixtures_file") ||
	fail 'cannot read the expected languages'

# Accept one object per page (gh --paginate) or an array of pages (--slurp).
all=$(jq -s 'map(if type == "array" then .[] else . end)' "$jobs_file") || fail 'jobs file is not valid JSON'
pages=$(jq 'length' <<<"$all")
((pages > 0)) || fail 'jobs file is empty'
jq -e 'all(type == "object" and (.jobs | type == "array"))' <<<"$all" > /dev/null || fail 'a page has no jobs array'
total=$(jq -r '[.[] | .total_count] | if all(type == "number") then (.[0] // 0) else -1 end' <<<"$all")
read_count=$(jq '[.[] | .jobs | length] | add' <<<"$all")
if ((total < 0)) || ((total != read_count)); then
	fail "read $read_count jobs but total_count is $total (missing page?)"
fi

bad=0
while IFS= read -r language; do
	name="editor_latency ($language)"
	n=$(jq --arg n "$name" '[.[] | .jobs[] | select(.name == $n)] | length' <<<"$all")
	if ((n != 1)); then
		echo "editor_latency legs: $n jobs named '$name', want exactly 1" >&2
		bad=1
		continue
	fi
	state=$(jq -r --arg n "$name" '[.[] | .jobs[] | select(.name == $n)][0] | "\(.status)/\(.conclusion)"' <<<"$all")
	if [[ $state != completed/success ]]; then
		echo "editor_latency legs: '$name' is $state, want completed/success" >&2
		bad=1
	fi
done <<<"$expected"
((bad == 0)) || exit 1
echo "editor_latency legs: all $(wc -l <<<"$expected") language jobs succeeded"
