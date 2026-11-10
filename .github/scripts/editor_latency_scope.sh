#!/usr/bin/env bash
# Decide whether the paired editor-latency job must run for a pull request.
#
# Usage: editor_latency_scope.sh <changed-paths-file> <ci-yml-diff-file> <repo-root>
#
# Prints "true" (run the job) or "false" (skip it) on standard output and one
# line of reason on standard error. It prints "false" only when every check
# below passes. Any error, unreadable input, or unknown path prints "true".
#
# Skip only if all hold:
#   1. .github/ccore-unrouted exists in the repository root.
#   2. The changed-path list is not empty and every path is in the skip set:
#      internal/ccore/**; under cgo_harness/: ccore_* files,
#      cgo_harness/internal/corecords/**, cgo_harness/docker/ccore_equality.sh;
#      docs/**, root *.md, LICENSE; .github/workflows/ci.yml when its diff has
#      no added or removed line naming the latency job, this rule, or w5.
#   3. Neither the marker nor this script is among the changed paths.

decide() {
	local paths_file=${1-} diff_file=${2-} root=${3-}
	if [[ -z $paths_file || -z $diff_file || -z $root ]]; then
		echo true 'missing argument'
		return
	fi
	if [[ ! -r $paths_file || ! -r $diff_file || ! -d $root ]]; then
		echo true 'an input is unreadable'
		return
	fi
	if [[ ! -f $root/.github/ccore-unrouted ]]; then
		echo true 'marker .github/ccore-unrouted is absent'
		return
	fi
	local count=0 path ci_changed=false
	while IFS= read -r path || [[ -n $path ]]; do
		count=$((count + 1))
		case $path in
		'' | /* | ../* | */../* | */.. | .. | */./* | ./*)
			echo true "unusual path: $path"
			return
			;;
		.github/ccore-unrouted | .github/scripts/editor_latency_scope.sh)
			echo true "the rule itself changes: $path"
			return
			;;
		internal/ccore/*) ;;
		cgo_harness/internal/corecords/*) ;;
		cgo_harness/docker/ccore_equality.sh) ;;
		cgo_harness/ccore_* | cgo_harness/*/ccore_*) ;;
		docs/*) ;;
		LICENSE) ;;
		.github/workflows/ci.yml) ci_changed=true ;;
		*)
			if [[ $path == *.md && $path != */* ]]; then
				continue
			fi
			echo true "path outside the skip set: $path"
			return
			;;
		esac
	done <"$paths_file"
	if ((count == 0)); then
		echo true 'changed-path list is empty'
		return
	fi
	if [[ $ci_changed == true ]]; then
		local hits
		if [[ ! -s $diff_file ]]; then
			echo true 'ci.yml changed but its diff is empty'
			return
		fi
		hits=$(grep -E '^[+-]' "$diff_file" |
			grep -Ei 'editor_latency|run_editor_latency|ccore-unrouted|editor_latency_scope|w5' | head -n 1)
		if [[ -n $hits ]]; then
			echo true "ci.yml diff touches the latency rule: $hits"
			return
		fi
	fi
	echo false "only ccore, its harness, and documentation paths changed ($count paths)"
}

main() {
	local out
	if ! out=$(decide "$@" 2>&1); then
		echo true
		echo 'error while deciding; running the job' >&2
		return 0
	fi
	local value=${out%% *} reason=${out#* }
	if [[ $value != true && $value != false ]]; then
		value=true
		reason='unexpected result; running the job'
	fi
	echo "$value"
	echo "$reason" >&2
	return 0
}

main "$@" || {
	echo true
	exit 0
}
