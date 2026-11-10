#!/usr/bin/env bash
# Decide whether the paired editor-latency job must run for a pull request.
#
# Usage: editor_latency_scope.sh <changed-paths-file> <repo-root>
#
# Prints "true" (run the job) or "false" (skip it) on standard output and one
# line of reason on standard error. It prints "false" only when every check
# below passes. Any error, unreadable input, or unknown path prints "true".
#
# Skip only if all hold:
#   1. .github/ccore-unrouted exists in the repository root.
#   2. The changed-path list is not empty and every path is in the skip set.
#      Each rule is an anchored regular expression, not a shell glob:
#        ^internal/ccore/.+$                       (the slash is required)
#        ^cgo_harness/ccore_[^/]+_test\.go$        (a test file, directly in cgo_harness/)
#        ^cgo_harness/internal/corecords/[^/]+$    (directly in that folder)
#        ^cgo_harness/docker/ccore_equality\.sh$
#        ^docs/.+$, root *.md, LICENSE
#   3. Neither the marker nor this script is among the changed paths.
# Any change to .github/workflows/ci.yml runs the job, whatever its content.

decide() {
	local paths_file=${1-} root=${2-}
	if [[ -z $paths_file || -z $root ]]; then
		echo true 'missing argument'
		return
	fi
	if [[ ! -r $paths_file || ! -d $root ]]; then
		echo true 'an input is unreadable'
		return
	fi
	if [[ ! -f $root/.github/ccore-unrouted ]]; then
		echo true 'marker .github/ccore-unrouted is absent'
		return
	fi
	local count=0 path
	local skip_rules=(
		'^internal/ccore/.+$'
		'^cgo_harness/ccore_[^/]+_test\.go$'
		'^cgo_harness/internal/corecords/[^/]+$'
		'^cgo_harness/docker/ccore_equality\.sh$'
		'^docs/.+$'
		'^LICENSE$'
		'^[^/]+\.md$'
	)
	local rule matched
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
		.github/workflows/ci.yml)
			echo true 'ci.yml changed'
			return
			;;
		esac
		matched=false
		for rule in "${skip_rules[@]}"; do
			if [[ $path =~ $rule ]]; then
				matched=true
				break
			fi
		done
		if [[ $matched != true ]]; then
			echo true "path outside the skip set: $path"
			return
		fi
	done <"$paths_file"
	if ((count == 0)); then
		echo true 'changed-path list is empty'
		return
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
