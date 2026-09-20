#!/usr/bin/env bash
# Contract test for scripts/refresh_a0_manifest_cascade.sh.
#
# The test copies the live cascade files into a scratch tree, perturbs the
# lock and the tracked manifest, and checks that the refresher:
#   - reports the live tree as current;
#   - fails --check on a stale tree without editing anything;
#   - rewrites the manifest field and every receipt constant;
#   - is idempotent;
#   - refreshes the tracked cascade only with --tracked;
#   - never touches the historical retirement log.
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
repo_root=$(cd -- "$script_dir/.." && pwd -P)
refresher="$script_dir/refresh_a0_manifest_cascade.sh"
test_root=$(mktemp -d "${TMPDIR:-/tmp}/gotreesitter-a0-cascade-test.XXXXXX")

cleanup() {
	rm -rf -- "$test_root"
}
trap cleanup EXIT

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

sha_of() {
	sha256sum -- "$1" | cut -d' ' -f1
}

a0_constant() {
	sed -nE 's/^[[:space:]]*[A-Za-z0-9_]*A0ManifestSHA[0-9]*[[:space:]]*=[[:space:]]*"([0-9a-f]{64})".*/\1/p' "$1" | head -n 1
}

tracked_constant() {
	sed -nE 's/^[[:space:]]*[A-Za-z0-9_]*TrackedManifestSHA[0-9]*[[:space:]]*=[[:space:]]*"([0-9a-f]{64})".*/\1/p' "$1" | head -n 1
}

lock_field() {
	sed -nE 's/^[[:space:]]*"grammar_lock_sha256":[[:space:]]*"([0-9a-f]{64})".*/\1/p' "$1" | head -n 1
}

copy_fixture() {
	mkdir -p -- "$test_root/$(dirname -- "$1")"
	cp -- "$repo_root/$1" "$test_root/$1"
}

a0_files=(
	julia_dispatch_blocker_receipt_test.go
	perl_n31s_dispatcher_blocker_receipt_test.go
	solidity_next_live_probe_test.go
	templ_n31q_dispatcher_blocker_receipt_test.go
	wgsl_n31r_dispatcher_blocker_receipt_test.go
	wolfram_next_live_probe_test.go
)
tracked_files=(
	julia_dispatch_blocker_receipt_test.go
	perl_n31s_dispatcher_blocker_receipt_test.go
	solidity_next_live_probe_test.go
	wgsl_n31r_dispatcher_blocker_receipt_test.go
	wolfram_next_live_probe_test.go
)

copy_fixture grammars/languages.lock
copy_fixture testdata/dispatcher_census_a0_manifest_v1.json
copy_fixture testdata/dispatcher_census_tracked_v1.json
for f in "${a0_files[@]}"; do
	copy_fixture "cgo_harness/$f"
done

# A historical log that quotes the live hashes must never change.
mkdir -p -- "$test_root/docs"
printf 'a0 %s\ntracked %s\n' \
	"$(sha_of "$repo_root/testdata/dispatcher_census_a0_manifest_v1.json")" \
	"$(sha_of "$repo_root/testdata/dispatcher_census_tracked_v1.json")" \
	>"$test_root/docs/root-normalization-retirement.md"
log_before=$(sha_of "$test_root/docs/root-normalization-retirement.md")

# 1. The live tree is current.
bash "$refresher" --repo-root "$test_root" --check --tracked >/dev/null ||
	fail "live cascade reported stale"

# 2. A perturbed lock fails --check, and --check edits nothing.
printf '# cascade contract test perturbation\n' >>"$test_root/grammars/languages.lock"
manifest_before=$(sha_of "$test_root/testdata/dispatcher_census_a0_manifest_v1.json")
julia_before=$(sha_of "$test_root/cgo_harness/julia_dispatch_blocker_receipt_test.go")
if bash "$refresher" --repo-root "$test_root" --check >/dev/null 2>&1; then
	fail "--check passed on a stale cascade"
fi
[[ "$manifest_before" == "$(sha_of "$test_root/testdata/dispatcher_census_a0_manifest_v1.json")" ]] ||
	fail "--check edited the manifest"
[[ "$julia_before" == "$(sha_of "$test_root/cgo_harness/julia_dispatch_blocker_receipt_test.go")" ]] ||
	fail "--check edited a receipt"

# 3. The refresh rewrites the lock field and all six A0 constants.
bash "$refresher" --repo-root "$test_root" >/dev/null
want_lock=$(sha_of "$test_root/grammars/languages.lock")
[[ "$(lock_field "$test_root/testdata/dispatcher_census_a0_manifest_v1.json")" == "$want_lock" ]] ||
	fail "manifest lock field not refreshed"
want_a0=$(sha_of "$test_root/testdata/dispatcher_census_a0_manifest_v1.json")
for f in "${a0_files[@]}"; do
	[[ "$(a0_constant "$test_root/cgo_harness/$f")" == "$want_a0" ]] ||
		fail "A0 constant not refreshed in $f"
done

# 4. A second run is a no-op.
tree_before=$(cat "$test_root"/cgo_harness/*.go "$test_root"/testdata/*.json | sha256sum)
bash "$refresher" --repo-root "$test_root" --check >/dev/null ||
	fail "refreshed cascade reported stale"
bash "$refresher" --repo-root "$test_root" >/dev/null
[[ "$tree_before" == "$(cat "$test_root"/cgo_harness/*.go "$test_root"/testdata/*.json | sha256sum)" ]] ||
	fail "second run changed files"

# 5. The tracked cascade refreshes only with --tracked.
printf '\n' >>"$test_root/testdata/dispatcher_census_tracked_v1.json"
bash "$refresher" --repo-root "$test_root" --check >/dev/null ||
	fail "tracked drift must not fail the default check"
if bash "$refresher" --repo-root "$test_root" --check --tracked >/dev/null 2>&1; then
	fail "--check --tracked passed on a stale tracked cascade"
fi
bash "$refresher" --repo-root "$test_root" --tracked >/dev/null
want_tracked=$(sha_of "$test_root/testdata/dispatcher_census_tracked_v1.json")
for f in "${tracked_files[@]}"; do
	[[ "$(tracked_constant "$test_root/cgo_harness/$f")" == "$want_tracked" ]] ||
		fail "tracked constant not refreshed in $f"
done
# The templ receipt has no tracked constant and keeps its A0 value.
[[ "$(a0_constant "$test_root/cgo_harness/templ_n31q_dispatcher_blocker_receipt_test.go")" == "$want_a0" ]] ||
	fail "templ A0 constant changed during a tracked refresh"

# 6. The historical log is untouched.
[[ "$log_before" == "$(sha_of "$test_root/docs/root-normalization-retirement.md")" ]] ||
	fail "historical log changed"

echo "refresh_a0_manifest_cascade.sh: ok"
