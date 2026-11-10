#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
scope="$script_dir/editor_latency_scope.sh"
test_root=$(mktemp -d "${TMPDIR:-/tmp}/editor-latency-scope-test.XXXXXX")
trap 'rm -rf -- "$test_root"' EXIT

cases=0
fail() {
	printf 'not ok - %s\n' "$1" >&2
	exit 1
}

# check <name> <expected> <paths (newline separated)> [marker: yes|no]
check() {
	local name=$1 want=$2 paths=$3 marker=${4-yes}
	local dir="$test_root/case$cases"
	mkdir -p "$dir/root/.github"
	if [[ $marker == yes ]]; then
		: >"$dir/root/.github/ccore-unrouted"
	fi
	if [[ -n $paths ]]; then printf '%s\n' "$paths" >"$dir/paths"; else : >"$dir/paths"; fi
	local got
	got=$(bash "$scope" "$dir/paths" "$dir/root" 2>"$dir/reason") || fail "$name: script exited non-zero"
	[[ -s $dir/reason ]] || fail "$name: no reason on standard error"
	[[ $got == "$want" ]] || fail "$name: got $got, want $want ($(cat "$dir/reason"))"
	cases=$((cases + 1))
	printf 'ok - %s\n' "$name"
}

ccore=$'internal/ccore/parser.go\ninternal/ccore/testdata/a.txt'

check 'only internal/ccore files skip' false "$ccore"
check 'ccore plus a root Go file runs' true "$ccore"$'\nccore_bind.go'
check 'ci.yml changed, any content: runs' true '.github/workflows/ci.yml'
check 'ci.yml with ccore paths runs (timeout change has no watched word)' true "$ccore"$'\n.github/workflows/ci.yml'
check 'missing marker runs' true "$ccore" no
check 'marker deleted in the pull request runs' true "$ccore"$'\n.github/ccore-unrouted'
check 'the scope script changing runs' true "$ccore"$'\n.github/scripts/editor_latency_scope.sh'
check 'empty path list runs' true ''
check 'cgo_harness ccore test, corecords file and ccore_equality.sh skip' false $'cgo_harness/ccore_equality_test.go\ncgo_harness/internal/corecords/a.go\ncgo_harness/docker/ccore_equality.sh'
check 'cgo_harness file that is not ccore_ runs' true $'internal/ccore/a.go\ncgo_harness/parity_cgo_test.go'
check 'other cgo_harness docker script runs' true 'cgo_harness/docker/run_parity_in_docker.sh'
check 'cgo_harness ccore_ file that is not a test runs' true 'cgo_harness/ccore_runtime.go'
check 'oracle source named ccore_ runs' true 'cgo_harness/internal/coracle/ccore_allocator.go'
check 'oracle C source runs' true 'cgo_harness/internal/coracle/src/parser.c'
check 'nested ccore test under a subfolder runs' true 'cgo_harness/sub/ccore_x_test.go'
check 'nested file under corecords runs' true 'cgo_harness/internal/corecords/sub/a.go'
check 'ccore test with an empty name runs' true 'cgo_harness/ccore__test.go'
check 'lookalike internal/ccorex runs' true 'internal/ccorex/a.go'
check 'internal/ccore as a file name runs' true 'internal/ccore'
check 'internal/ccore/ with nothing after it runs' true 'internal/ccore/'
check 'ccore_equality.sh in another folder runs' true 'cgo_harness/ccore_equality.sh'
check 'grammars/ runs' true $'internal/ccore/a.go\ngrammars/go.go'
check 'cmd/ runs' true $'internal/ccore/a.go\ncmd/v1layout/main.go'
check 'go.mod runs' true $'internal/ccore/a.go\ngo.mod'
check 'documentation only skips' false $'docs/guide.md\nREADME.md\nLICENSE'
check 'nested markdown outside docs runs' true 'internal/other/README.md'
check 'path with parent segment runs' true 'internal/ccore/../../parser.go'
check 'blank line in the list runs' true $'internal/ccore/a.go\n\ninternal/ccore/b.go'

# Errors fail towards running the job.
[[ $(bash "$scope" /nonexistent /nonexistent /nonexistent 2>/dev/null) == true ]] || fail 'unreadable input must print true'
[[ $(bash "$scope" 2>/dev/null) == true ]] || fail 'missing arguments must print true'
cases=$((cases + 2))
printf 'ok - errors print true\n'
printf '%d cases passed\n' "$cases"
