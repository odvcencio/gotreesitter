#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
repo_root=$(cd -- "$script_dir/../.." && pwd -P)
runner="$script_dir/run_parity_in_docker.sh"
test_root=$(mktemp -d "${TMPDIR:-/tmp}/gotreesitter-parity-docker-test.XXXXXX")
mock_bin="$test_root/bin"
mkdir -p -- "$mock_bin"

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

# fake_docker writes a mock docker binary that answers "build" subcommands
# from a scripted attempt log. Each answer is one line of the form
# "<exit_code>|<stderr text>". Lines are consumed in order, one per
# invocation; the last line repeats for any invocation past the end.
fake_docker() {
	local answers_file=$1
	cat >"$mock_bin/docker" <<EOF
#!/usr/bin/env bash
set -euo pipefail
if [[ "\$1" != "build" ]]; then
	echo "unsupported docker subcommand: \$*" >&2
	exit 1
fi
answers_file="$answers_file"
count_file="$test_root/docker-call-count"
count=0
[[ -f "\$count_file" ]] && count=\$(cat "\$count_file")
count=\$((count + 1))
echo "\$count" >"\$count_file"
total_lines=\$(wc -l <"\$answers_file")
line_no=\$count
if [[ "\$line_no" -gt "\$total_lines" ]]; then
	line_no=\$total_lines
fi
answer=\$(sed -n "\${line_no}p" "\$answers_file")
exit_code="\${answer%%|*}"
message="\${answer#*|}"
if [[ -n "\$message" ]]; then
	echo "\$message" >&2
fi
exit "\$exit_code"
EOF
	chmod +x "$mock_bin/docker"
}

call_count() {
	if [[ ! -f "$test_root/docker-call-count" ]]; then
		printf '0\n'
		return
	fi
	cat "$test_root/docker-call-count"
}

reset_call_count() {
	rm -f -- "$test_root/docker-call-count"
}

# --help still documents --build-only without invoking docker.
help_output="$test_root/help.txt"
bash "$runner" --help >"$help_output"
assert_contains '--build-only' "$help_output"
pass '--help documents --build-only'

# --build-only and --no-build are mutually exclusive.
reset_call_count
conflict_status=0
bash "$runner" --repo-root "$repo_root" --build-only --no-build \
	>"$test_root/conflict.stdout" 2>"$test_root/conflict.stderr" || conflict_status=$?
[[ "$conflict_status" -eq 2 ]] || fail "conflicting build flags returned $conflict_status, want 2"
assert_contains 'mutually exclusive' "$test_root/conflict.stderr"
pass '--build-only rejects --no-build'

# A network-class failure retries and then succeeds; the run exits 0 without
# starting a container.
reset_call_count
printf '1|Error response from daemon: Get "https://registry-1.docker.io/v2/": read: connection reset by peer\n0|fake build ok\n' \
	>"$test_root/flake-answers.txt"
fake_docker "$test_root/flake-answers.txt"
flake_status=0
PATH="$mock_bin:$PATH" bash "$runner" --repo-root "$repo_root" --build-only \
	>"$test_root/flake.stdout" 2>"$test_root/flake.stderr" || flake_status=$?
[[ "$flake_status" -eq 0 ]] || fail "retry-then-success build returned $flake_status, want 0"
[[ "$(call_count)" -eq 2 ]] || fail "expected 2 docker build attempts, got $(call_count)"
assert_contains 'retrying in 10s' "$test_root/flake.stderr"
assert_contains 'image built:' "$test_root/flake.stdout"
pass 'a transient network failure retries once and then succeeds'

# A non-network failure exits at once with no retry.
reset_call_count
printf '1|Dockerfile:5 unknown instruction FOOBAR\n' >"$test_root/hard-answers.txt"
fake_docker "$test_root/hard-answers.txt"
hard_status=0
PATH="$mock_bin:$PATH" bash "$runner" --repo-root "$repo_root" --build-only \
	>"$test_root/hard.stdout" 2>"$test_root/hard.stderr" || hard_status=$?
[[ "$hard_status" -eq 1 ]] || fail "non-network build failure returned $hard_status, want 1"
[[ "$(call_count)" -eq 1 ]] || fail "expected 1 docker build attempt, got $(call_count)"
assert_contains 'not retrying' "$test_root/hard.stderr"
pass 'a non-network build failure does not retry'

# Persistent network failures exhaust all three attempts and exit non-zero.
reset_call_count
printf '1|i/o timeout\n1|i/o timeout\n1|i/o timeout\n' >"$test_root/exhausted-answers.txt"
fake_docker "$test_root/exhausted-answers.txt"
exhausted_status=0
PATH="$mock_bin:$PATH" bash "$runner" --repo-root "$repo_root" --build-only \
	>"$test_root/exhausted.stdout" 2>"$test_root/exhausted.stderr" || exhausted_status=$?
[[ "$exhausted_status" -eq 1 ]] || fail "exhausted retries returned $exhausted_status, want 1"
[[ "$(call_count)" -eq 3 ]] || fail "expected 3 docker build attempts, got $(call_count)"
assert_contains 'retrying in 10s' "$test_root/exhausted.stderr"
assert_contains 'retrying in 30s' "$test_root/exhausted.stderr"
assert_contains 'giving up' "$test_root/exhausted.stderr"
pass 'persistent network failures exhaust three attempts and fail'

printf '%s\n' 'all run_parity_in_docker.sh tests passed'
