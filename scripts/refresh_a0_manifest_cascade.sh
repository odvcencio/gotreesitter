#!/usr/bin/env bash
# Refresh the A0 manifest receipt cascade after a grammar lock change.
#
# A lock change moves three values in order:
#   1. sha256(grammars/languages.lock) becomes "grammar_lock_sha256" in
#      testdata/dispatcher_census_a0_manifest_v1.json.
#   2. sha256(that manifest) becomes the *A0ManifestSHA256 constant in six
#      cgo_harness receipt tests.
#   3. With --tracked, sha256(testdata/dispatcher_census_tracked_v1.json)
#      becomes the *TrackedManifestSHA256 constant in five of those tests.
# Every hash is the plain SHA-256 of the file bytes.
#
# The script edits only the files listed below. It never touches
# docs/root-normalization-retirement.md, which is a historical log.
#
# Usage:
#   scripts/refresh_a0_manifest_cascade.sh             rewrite stale values
#   scripts/refresh_a0_manifest_cascade.sh --check     exit 1 when stale, edit nothing
#   scripts/refresh_a0_manifest_cascade.sh --tracked   also refresh the tracked cascade
#   scripts/refresh_a0_manifest_cascade.sh --repo-root DIR
#
# Requires GNU sed and sha256sum.
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
repo_root=$(cd -- "$script_dir/.." && pwd -P)
check_only=0
tracked=0

while [[ $# -gt 0 ]]; do
	case "$1" in
	--check) check_only=1 ;;
	--tracked) tracked=1 ;;
	--repo-root)
		shift
		repo_root=$(cd -- "$1" && pwd -P)
		;;
	--repo-root=*) repo_root=$(cd -- "${1#--repo-root=}" && pwd -P) ;;
	-h | --help)
		sed -n '2,21p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*)
		echo "unknown argument: $1" >&2
		exit 2
		;;
	esac
	shift
done

lock_path="grammars/languages.lock"
a0_manifest_path="testdata/dispatcher_census_a0_manifest_v1.json"
tracked_manifest_path="testdata/dispatcher_census_tracked_v1.json"
a0_receipt_files=(
	cgo_harness/julia_dispatch_blocker_receipt_test.go
	cgo_harness/perl_n31s_dispatcher_blocker_receipt_test.go
	cgo_harness/solidity_next_live_probe_test.go
	cgo_harness/templ_n31q_dispatcher_blocker_receipt_test.go
	cgo_harness/wgsl_n31r_dispatcher_blocker_receipt_test.go
	cgo_harness/wolfram_next_live_probe_test.go
)
tracked_receipt_files=(
	cgo_harness/julia_dispatch_blocker_receipt_test.go
	cgo_harness/perl_n31s_dispatcher_blocker_receipt_test.go
	cgo_harness/solidity_next_live_probe_test.go
	cgo_harness/wgsl_n31r_dispatcher_blocker_receipt_test.go
	cgo_harness/wolfram_next_live_probe_test.go
)

lock_field_re='^([[:space:]]*"grammar_lock_sha256":[[:space:]]*")[0-9a-f]{64}(")'
stale=0

sha256_of() {
	sha256sum -- "$1" | cut -d' ' -f1
}

require_file() {
	if [[ ! -f "$repo_root/$1" ]]; then
		echo "missing file: $1" >&2
		exit 1
	fi
}

# constant_value FILE PATTERN prints the 64-hex value of the first constant
# whose name ends in PATTERN (an extended regex fragment).
constant_value() {
	local file="$1" pattern="$2"
	sed -nE "s/^[[:space:]]*[A-Za-z0-9_]*${pattern}[[:space:]]*=[[:space:]]*\"([0-9a-f]{64})\".*/\1/p" "$file" | head -n 1
}

# set_constant FILE PATTERN NEW rewrites the matching constant in place.
set_constant() {
	local file="$1" pattern="$2" new="$3"
	sed -i -E "s/^([[:space:]]*[A-Za-z0-9_]*${pattern}[[:space:]]*=[[:space:]]*\")[0-9a-f]{64}(\")/\1${new}\2/" "$file"
}

# refresh_constants LABEL PATTERN NEW FILE... reports each file and rewrites
# the stale ones unless --check is set.
refresh_constants() {
	local label="$1" pattern="$2" new="$3"
	shift 3
	local file old
	for file in "$@"; do
		require_file "$file"
		old=$(constant_value "$repo_root/$file" "$pattern")
		if [[ -z "$old" ]]; then
			echo "no ${label} constant found in $file" >&2
			exit 1
		fi
		if [[ "$old" == "$new" ]]; then
			printf '%s %s: current (%s)\n' "$label" "$file" "${new:0:12}"
			continue
		fi
		stale=1
		printf '%s %s: %s -> %s\n' "$label" "$file" "${old:0:12}" "${new:0:12}"
		if [[ $check_only -eq 0 ]]; then
			set_constant "$repo_root/$file" "$pattern" "$new"
		fi
	done
}

require_file "$lock_path"
require_file "$a0_manifest_path"

lock_sha=$(sha256_of "$repo_root/$lock_path")
manifest_lock_sha=$(sed -nE 's/^[[:space:]]*"grammar_lock_sha256":[[:space:]]*"([0-9a-f]{64})".*/\1/p' "$repo_root/$a0_manifest_path" | head -n 1)
if [[ -z "$manifest_lock_sha" ]]; then
	echo "no grammar_lock_sha256 field in $a0_manifest_path" >&2
	exit 1
fi

if [[ "$manifest_lock_sha" == "$lock_sha" ]]; then
	printf 'lock %s: current (%s)\n' "$a0_manifest_path" "${lock_sha:0:12}"
else
	stale=1
	printf 'lock %s: %s -> %s\n' "$a0_manifest_path" "${manifest_lock_sha:0:12}" "${lock_sha:0:12}"
	if [[ $check_only -eq 0 ]]; then
		sed -i -E "s/${lock_field_re}/\1${lock_sha}\2/" "$repo_root/$a0_manifest_path"
	fi
fi

# --check leaves the manifest as it is, so hash the manifest the rewrite
# would produce.
if [[ $check_only -eq 1 && "$manifest_lock_sha" != "$lock_sha" ]]; then
	a0_manifest_sha=$(sed -E "s/${lock_field_re}/\1${lock_sha}\2/" "$repo_root/$a0_manifest_path" | sha256sum | cut -d' ' -f1)
else
	a0_manifest_sha=$(sha256_of "$repo_root/$a0_manifest_path")
fi

refresh_constants "a0" 'A0ManifestSHA[0-9]*' "$a0_manifest_sha" "${a0_receipt_files[@]}"

if [[ $tracked -eq 1 ]]; then
	require_file "$tracked_manifest_path"
	tracked_manifest_sha=$(sha256_of "$repo_root/$tracked_manifest_path")
	refresh_constants "tracked" 'TrackedManifestSHA[0-9]*' "$tracked_manifest_sha" "${tracked_receipt_files[@]}"
fi

if [[ $stale -eq 0 ]]; then
	echo "cascade is current"
	exit 0
fi
if [[ $check_only -eq 1 ]]; then
	echo "cascade is stale; run scripts/refresh_a0_manifest_cascade.sh" >&2
	exit 1
fi
echo "cascade refreshed"
