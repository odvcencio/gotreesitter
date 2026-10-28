#!/usr/bin/env bash
# Reject benchmark evidence in the repository.
#
#   check_evidence_paths.sh            check every tracked path (the index)
#   check_evidence_paths.sh BASE_SHA   check only paths added since BASE_SHA
#
# Evidence belongs in the PR body, the GitHub release named "evidence", or the
# Hyphae space for this repository (see AGENTS.md, "Evidence stays out of the
# repository"). The paths named in scripts/evidence_paths_allowlist.txt are
# existing files that wait for retirement. The list may only shrink.
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
repo_root=$(cd -- "$script_dir/.." && pwd -P)
allowlist_rel=scripts/evidence_paths_allowlist.txt
allowlist="$repo_root/$allowlist_rel"
max_doc_bytes=200000

base=${1:-}
if [[ $# -gt 1 ]]; then
	printf 'usage: %s [BASE_SHA]\n' "$0" >&2
	exit 2
fi

git_() { git -C "$repo_root" "$@"; }

# Patterns for paths that hold evidence, checked against each docs/ path.
name_patterns=(
	'^docs/benchmarks/'
	'^docs/.*-(receipts?|screens?|census|certificate|certification|proof|readiness)(-v[0-9]+)?(-[0-9]{8})?\.(md|json)$'
	'^docs/.*-[0-9]{8}\.(md|json)$'
	'^docs/.*-[0-9]{4}-[0-9]{2}-[0-9]{2}\.(md|json)$'
	'^docs/compact-'
	'^docs/.*\.(tar|gz|tgz|zst|zip|log|rss|pprof|csv|jsonl|patch|txt)$'
)
# The one user-facing document that the compact- prefix rule would catch.
name_exempt='^docs/compact-parser-maintenance\.md$'

# Allowlisted paths. A line holds a path, then " # reason".
allowed=()
if [[ -f "$allowlist" ]]; then
	while IFS= read -r line; do
		line=${line%%#*}
		line=${line%"${line##*[![:space:]]}"}
		[[ -n "$line" ]] && allowed+=("$line")
	done <"$allowlist"
fi

is_allowed() {
	local entry
	for entry in ${allowed[@]+"${allowed[@]}"}; do
		[[ "$entry" == "$1" ]] && return 0
	done
	return 1
}

fail_count=0
reject() {
	printf 'evidence guard: %s: %s\n' "$1" "$2" >&2
	fail_count=$((fail_count + 1))
}

if [[ -n "$base" ]]; then
	git_ rev-parse --verify --quiet "$base^{commit}" >/dev/null ||
		{ printf 'evidence guard: base commit not found: %s\n' "$base" >&2; exit 2; }
	size_ref=HEAD:
	list_cmd=(git_ diff --name-only --no-renames --diff-filter=A "$base" HEAD --)
else
	size_ref=:
	list_cmd=(git_ ls-files --)
fi

while IFS= read -r path; do
	[[ "$path" == docs/* ]] || continue
	is_allowed "$path" && continue
	for pattern in "${name_patterns[@]}"; do
		if [[ "$path" =~ $pattern ]] && ! [[ "$path" =~ $name_exempt ]]; then
			reject "$path" "evidence-like path (matches $pattern)"
			continue 2
		fi
	done
	if [[ "$path" != docs/changelog/* ]]; then
		bytes=$(git_ cat-file -s "${size_ref}${path}" 2>/dev/null || echo 0)
		if ((bytes > max_doc_bytes)); then
			reject "$path" "$bytes bytes exceeds the $max_doc_bytes-byte limit for docs files"
		fi
	fi
done < <("${list_cmd[@]}")

# Every allowlist entry must still name a tracked file, so the list shrinks as
# files retire.
for entry in ${allowed[@]+"${allowed[@]}"}; do
	if [[ -z "$(git_ ls-files -- "$entry")" ]]; then
		reject "$entry" "allowlist entry names no tracked file; remove it from $allowlist_rel"
	fi
done

# With a base, the allowlist may not gain entries.
if [[ -n "$base" ]] && git_ cat-file -e "$base:$allowlist_rel" 2>/dev/null; then
	base_entries=$(git_ show "$base:$allowlist_rel" | sed 's/[[:space:]]*#.*$//')
	for entry in ${allowed[@]+"${allowed[@]}"}; do
		if ! grep -qxF -- "$entry" <<<"$base_entries"; then
			reject "$entry" "new allowlist entry; the list may only shrink"
		fi
	done
fi

if ((fail_count > 0)); then
	cat >&2 <<'EOF'

Evidence stays out of the repository. Put numbers and verdicts in the PR body,
raw files in the "evidence" GitHub release (gh release upload evidence <file>
--repo odvcencio/gotreesitter), and written receipts in the Hyphae space
hypha://m31labs/gotreesitter under reports/receipts/.
EOF
	exit 1
fi
if [[ -n "$base" ]]; then
	printf 'ok - no new evidence paths since %s\n' "$base"
else
	printf 'ok - no evidence paths in the tracked tree\n'
fi
