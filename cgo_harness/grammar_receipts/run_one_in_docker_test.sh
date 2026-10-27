#!/usr/bin/env bash
set -euo pipefail

# Checks the Docker steps of run_one_in_docker.sh with a stub runner and a stub
# docker: which containers it starts, in which order, and what it does when the
# extraction of an injection grammar's samples fails.

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
script="$script_dir/run_one_in_docker.sh"
test_root=$(mktemp -d "${TMPDIR:-/tmp}/gotreesitter-receipt-docker-test.XXXXXX")

cleanup() {
	rm -rf -- "$test_root"
}
trap cleanup EXIT

fail() {
	printf 'not ok - %s\n' "$1" >&2
	exit 1
}

pass() {
	printf 'ok - %s\n' "$1"
}

assert_contains() {
	local needle=$1
	local file=$2
	grep -F -- "$needle" "$file" >/dev/null || fail "missing '$needle' in $file"
}

assert_not_contains() {
	local needle=$1
	local file=$2
	if grep -F -- "$needle" "$file" >/dev/null; then
		fail "unexpected '$needle' in $file"
	fi
}

if [[ ! -x /usr/bin/time ]]; then
	printf '%s\n' 'skip - /usr/bin/time is missing'
	exit 0
fi

# The script finds its runner next to the repository root, so run a copy of it
# in a small repository that holds a stub runner.
repo="$test_root/repo"
mkdir -p -- "$repo/cgo_harness/grammar_receipts" "$repo/cgo_harness/docker" "$test_root/bin"
cp -- "$script" "$repo/cgo_harness/grammar_receipts/run_one_in_docker.sh"

# The stub runner logs one line per container: "<kind> corpus-tree=<yes|no>".
# An extraction writes a partial tree, then exits with STUB_EXTRACT_STATUS. The
# receipt run reports whether its read-only corpus holds that tree and exits
# with STUB_RECEIPT_STATUS.
cat >"$repo/cgo_harness/docker/run_parity_in_docker.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
label=""
mounts=()
while [[ $# -gt 0 ]]; do
	case "$1" in
		--label) label=$2; shift 2 ;;
		--mount) mounts+=("$2"); shift 2 ;;
		--out-root | --cpus | --memory | --wall-timeout) shift 2 ;;
		--no-build) shift ;;
		--) break ;;
		*) echo "stub runner: unknown option $1" >&2; exit 2 ;;
	esac
done
grammar=${label#grammar-receipt-}
if [[ "$grammar" == *-extract ]]; then
	grammar=${grammar%-extract}
	tree="${mounts[0]%%:*}/.gts-extracted/$grammar"
	mkdir -p -- "$tree"
	printf 'sample\n' >"$tree/00001.sample"
	echo "extract corpus-tree=yes" >>"$STUB_LOG"
	exit "${STUB_EXTRACT_STATUS:-0}"
fi
tree=no
for mount in "${mounts[@]}"; do
	if [[ "$mount" == *:/corpus_sources:ro && -d "${mount%:/corpus_sources:ro}/$grammar/.gts-extracted/$grammar" ]]; then
		tree=yes
	fi
done
echo "receipt corpus-tree=$tree" >>"$STUB_LOG"
exit "${STUB_RECEIPT_STATUS:-0}"
EOF
chmod +x "$repo/cgo_harness/docker/run_parity_in_docker.sh"

# The image exists and no parity container runs.
cat >"$test_root/bin/docker" <<'EOF'
#!/usr/bin/env bash
case "$1" in
	image | ps) exit 0 ;;
esac
echo "unexpected docker call: $*" >&2
exit 1
EOF
chmod +x "$test_root/bin/docker"

git -C "$repo" init -q
git -C "$repo" add -A
git -C "$repo" -c user.name=test -c user.email=test@example.invalid commit -q -m fixture

# run_case <grammar> <lock-subdir> [KEY=value ...] runs the script on a fresh
# corpus root and leaves its status, output and runner log in $test_root.
run_case() {
	local grammar=$1 subdir=$2
	shift 2
	rm -rf -- "$test_root/corpus" "$test_root/out" "$test_root/calls.log"
	mkdir -p -- "$test_root/corpus/$grammar"
	printf '%s https://example.invalid/%s %s %s .sample\n' "$grammar" "$grammar" "$(printf '%040d' 1)" "$subdir" >"$test_root/lock"
	: >"$test_root/calls.log"
	case_status=0
	env PATH="$test_root/bin:$PATH" STUB_LOG="$test_root/calls.log" "$@" \
		bash "$repo/cgo_harness/grammar_receipts/run_one_in_docker.sh" \
		"$grammar" "$test_root/corpus" "$test_root/lock" "$(printf '%064d' 1)" "$test_root/out" \
		>"$test_root/case.stdout" 2>"$test_root/case.stderr" || case_status=$?
}

# A grammar that reads its checkout directly gets one container, the receipt run.
run_case go . STUB_EXTRACT_STATUS=1
[[ "$case_status" -eq 0 ]] || fail "plain grammar returned $case_status, want 0"
[[ "$(cat "$test_root/calls.log")" == "receipt corpus-tree=no" ]] || fail "plain grammar ran: $(cat "$test_root/calls.log")"
pass 'a plain grammar gets only the receipt container'

# An injection grammar extracts first; the receipt run sees the tree.
run_case comment .gts-extracted/comment
[[ "$case_status" -eq 0 ]] || fail "injection grammar returned $case_status, want 0"
[[ "$(paste -sd, "$test_root/calls.log")" == "extract corpus-tree=yes,receipt corpus-tree=yes" ]] || fail "injection grammar ran: $(paste -sd, "$test_root/calls.log")"
assert_not_contains '::warning' "$test_root/case.stdout"
pass 'an injection grammar extracts, then runs the receipt on the tree'

# A failed extraction warns, removes its partial tree and still runs the receipt.
run_case gitcommit .gts-extracted/gitcommit STUB_EXTRACT_STATUS=1
[[ "$case_status" -eq 0 ]] || fail "failed extraction returned $case_status, want 0"
[[ "$(paste -sd, "$test_root/calls.log")" == "extract corpus-tree=yes,receipt corpus-tree=no" ]] || fail "failed extraction ran: $(paste -sd, "$test_root/calls.log")"
assert_contains '::warning title=Corpus extraction failed::gitcommit: the extraction run failed (status 1)' "$test_root/case.stdout"
[[ ! -e "$test_root/corpus/gitcommit/.gts-extracted/gitcommit" ]] || fail 'the partial tree is still there'
pass 'a failed extraction warns, removes the partial tree and runs the receipt'

# A timeout of the extraction container (status 124) is handled the same way.
run_case jsdoc .gts-extracted/jsdoc STUB_EXTRACT_STATUS=124
[[ "$case_status" -eq 0 ]] || fail "timed-out extraction returned $case_status, want 0"
assert_contains 'the extraction run failed (status 124)' "$test_root/case.stdout"
[[ "$(paste -sd, "$test_root/calls.log")" == "extract corpus-tree=yes,receipt corpus-tree=no" ]] || fail "timed-out extraction ran: $(paste -sd, "$test_root/calls.log")"
pass 'a timed-out extraction is handled like a failed one'

# The script still fails when the receipt run fails, so no error is lost.
run_case doxygen .gts-extracted/doxygen STUB_EXTRACT_STATUS=1 STUB_RECEIPT_STATUS=3
[[ "$case_status" -eq 3 ]] || fail "failed receipt run returned $case_status, want 3"
pass 'a failed receipt run still fails the script'

printf '%s\n' 'all run_one_in_docker.sh tests passed'
