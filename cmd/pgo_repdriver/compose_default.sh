#!/usr/bin/env sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
production="$script_dir/inputs/production-v1.pgo"
selected="$script_dir/inputs/selected-clean-error-v1.pgo"
output=${1:-"$script_dir/../parity_report/default.pgo"}
go_cmd=${GO:-go}

production_sha=b55eb63bf103b1c53f7ff7defa3a7d77f40408924a641a5045a622e3b3dafcb3
selected_sha=0d1faade90a2fd4b307886c68f0cec98bce3ff859a28371d9c80ece123d02a9a
output_sha=1e5e9aea594f4fcbc3c0fb5d4064d2fb856b13b2faf0994f747520615eaa2ae2

hash_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		echo "compose_default: need sha256sum or shasum" >&2
		exit 1
	fi
}

check_hash() {
	path=$1
	want=$2
	got=$(hash_file "$path")
	if [ "$got" != "$want" ]; then
		echo "compose_default: $path: got SHA-256 $got, want $want" >&2
		exit 1
	fi
}

check_hash "$production" "$production_sha"
check_hash "$selected" "$selected_sha"

mkdir -p "$(dirname -- "$output")"
output_dir=$(CDPATH= cd -- "$(dirname -- "$output")" && pwd)
output_abs="$output_dir/$(basename -- "$output")"
case "$output_abs" in
	"$production"|"$selected")
		echo "compose_default: refusing to overwrite immutable input $output_abs" >&2
		exit 1
		;;
esac
tmp=$(mktemp "${output}.tmp.XXXXXX")
trap 'rm -f "$tmp"' EXIT HUP INT TERM

# Multiplicity is deliberate: one production profile plus two copies of the
# broader selected/clean/error profile was the Pareto winner on the locked
# selected-store and production-corpus gates.
"$go_cmd" tool pprof -proto -output="$tmp" \
	"$production" "$selected" "$selected" >/dev/null
check_hash "$tmp" "$output_sha"
chmod 0644 "$tmp"
mv "$tmp" "$output"
trap - EXIT HUP INT TERM

echo "$output_sha  $output"
