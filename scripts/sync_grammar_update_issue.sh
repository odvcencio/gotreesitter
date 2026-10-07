#!/usr/bin/env bash
# Reconcile the single open grammar update issue after a successful report.
set -euo pipefail

PLAN=${1:?expected update plan}
BODY=${2:?expected rendered issue body}
: "${GH_REPO:?expected GitHub repository}"
TITLE="Grammar updates available"
LABEL="grammar-updates"

PENDING=$(jq -er '
  if (.error_count // 0) != 0 or .write_applied == true then
    error("expected a successful read-only update plan")
  else [.results[] | select(.status == "available")] | length end
' "$PLAN")

# The title and label identify reports; other grammar issues stay untouched.
# Workflow concurrency serializes scheduled and manual runs.
ISSUES=$(gh issue list --repo "$GH_REPO" --state open --label "$LABEL" \
  --limit 1000 --json number,title)
MATCHES=$(jq -c --arg title "$TITLE" \
  '[.[] | select(.title == $title)] | sort_by(.number) | map(.number)' <<< "$ISSUES")
mapfile -t NUMBERS < <(jq -r '.[]' <<< "$MATCHES")

if [ "$PENDING" -eq 0 ]; then
  for number in "${NUMBERS[@]}"; do
    gh issue close "$number" --repo "$GH_REPO" --reason completed
  done
elif [ "${#NUMBERS[@]}" -eq 0 ]; then
  gh label create "$LABEL" --repo "$GH_REPO" --color "0E8A16" \
    --description "Available upstream grammar updates" --force
  gh issue create --repo "$GH_REPO" --title "$TITLE" --label "$LABEL" --body-file "$BODY"
else
  gh issue edit "${NUMBERS[0]}" --repo "$GH_REPO" --title "$TITLE" --body-file "$BODY"
  for number in "${NUMBERS[@]:1}"; do
    gh issue close "$number" --repo "$GH_REPO" --reason "not planned"
  done
fi
