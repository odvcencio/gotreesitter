#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
repo_root=$(cd -- "$script_dir/.." && pwd -P)
path_guard="$script_dir/validate_worktree_path.sh"
test_root=$(mktemp -d "${TMPDIR:-/tmp}/gotreesitter-hygiene-test.XXXXXX")

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

[[ -x "$path_guard" ]] || fail "path guard is not executable: $path_guard"
resolved="$($path_guard "$repo_root")"
[[ "$resolved" == "$repo_root" ]] || fail "path guard changed the repository root"
pass 'the repository root passes the worktree path guard'

mkdir -p -- "$test_root/not-a-repo"
if "$path_guard" "$test_root/not-a-repo" >"$test_root/not-a-repo.out" 2>"$test_root/not-a-repo.err"; then
	fail 'a non-repository directory passed the worktree path guard'
fi
assert_contains 'not a Git worktree' "$test_root/not-a-repo.err"
pass 'the path guard rejects a non-repository directory'

status_path="$test_root/Preparing worktree (detached HEAD 3c55dca2)"
mkdir -p -- "$status_path"
if "$path_guard" "$status_path" >"$test_root/status.out" 2>"$test_root/status.err"; then
	fail 'worktree status text passed the path guard'
fi
assert_contains 'worktree status text' "$test_root/status.err"
pass 'the path guard rejects pasted worktree status text'

if bash "$repo_root/cgo_harness/docker/run_parity_in_docker.sh" \
	--repo-root "$status_path" --no-build >"$test_root/runner.out" 2>"$test_root/runner.err"; then
	fail 'the Docker parity runner accepted pasted worktree status text'
fi
assert_contains 'worktree status text' "$test_root/runner.err"
pass 'the Docker parity runner rejects pasted worktree status text'

if bash "$repo_root/cgo_harness/docker/run_parity_experiments.sh" \
	--experiment "status=$status_path" --max-parallel 1 --memory 1g --no-build \
	>"$test_root/experiments.out" 2>"$test_root/experiments.err"; then
	fail 'the parity experiment runner accepted pasted worktree status text'
fi
assert_contains 'worktree status text' "$test_root/experiments.err"
pass 'the parity experiment runner rejects pasted worktree status text'

named_worktree="$test_root/Preparing worktree (detached HEAD valid)"
git init -q "$named_worktree"
resolved="$($path_guard "$named_worktree")"
[[ "$resolved" == "$named_worktree" ]] || fail 'a valid status-like worktree name was rejected'
pass 'the path guard accepts a valid worktree with a status-like name'

nested_worktree="$named_worktree/nested"
mkdir -p -- "$nested_worktree"
if "$path_guard" "$nested_worktree" >"$test_root/nested.out" 2>"$test_root/nested.err"; then
	fail 'a nested directory passed as a worktree root'
fi
assert_contains 'not the worktree root' "$test_root/nested.err"
pass 'the path guard rejects a nested directory'

linebreak_path="$test_root/Preparing worktree (detached HEAD 3c55dca2)"
linebreak_path+=$'\n'
if "$path_guard" "$linebreak_path" >"$test_root/linebreak.out" 2>"$test_root/linebreak.err"; then
	fail 'a path with a line break passed the path guard'
fi
assert_contains 'line break' "$test_root/linebreak.err"
pass 'the path guard rejects a path with a line break'

newline_target="$test_root/symlink-target-newline"
newline_target+=$'\n'
mkdir -p -- "$newline_target"
ln -s -- "$newline_target" "$test_root/newline-link"
if "$path_guard" "$test_root/newline-link" >"$test_root/newline-link.out" 2>"$test_root/newline-link.err"; then
	fail 'a symlink to a newline path passed the path guard'
fi
assert_contains 'canonical path contains a line break' "$test_root/newline-link.err"
pass 'the path guard rejects a symlink target with a line break'

carriage_target="$test_root/symlink-target-carriage"
carriage_target+=$'\r'
mkdir -p -- "$carriage_target"
ln -s -- "$carriage_target" "$test_root/carriage-link"
if "$path_guard" "$test_root/carriage-link" >"$test_root/carriage-link.out" 2>"$test_root/carriage-link.err"; then
	fail 'a symlink to a carriage-return path passed the path guard'
fi
assert_contains 'canonical path contains a line break' "$test_root/carriage-link.err"
pass 'the path guard rejects a symlink target with a carriage return'

fixture="$test_root/canopy-fixture"
mkdir -p -- "$fixture/scripts"
cp -- "$repo_root/.graftignore" "$fixture/.graftignore"
cp -- "$repo_root/.gitignore" "$fixture/.gitignore"
cp -- "$repo_root/scripts/canopy_query.sh" "$fixture/scripts/canopy_query.sh"
git -C "$fixture" init -q
git -C "$fixture" config user.email test@example.invalid
git -C "$fixture" config user.name 'Hygiene Test'
git -C "$fixture" add .graftignore .gitignore scripts/canopy_query.sh
git -C "$fixture" commit -qm 'fixture'

ignored_paths=(
	.codex
	.agents
	.gts
	.tiller
	.canopy
	.golden
	.parity_seed
	tmp
	nul
	benchgate
	docs/blog-outlines
	docs/plans
	docs/superpowers
	cgo_harness/bench/runs
	cgo_harness/grammar_seed
	cgo_harness/real_corpus_bench_report
	cgo_harness/reports
	cgo_harness/corpus_real
	harness_out
	parity_out
	.worktrees
)
for relative_path in "${ignored_paths[@]}"; do
	mkdir -p -- "$fixture/$relative_path"
	printf 'package fixture\n' >"$fixture/$relative_path/private.go"
done
malformed_dir="$fixture/Preparing worktree (detached HEAD 3c55dca2)"
malformed_dir+=$'\n'
mkdir -p -- "$malformed_dir"
printf 'package fixture\n' >"$malformed_dir/private.go"

mkdir -p -- "$fixture/.canopy" "$test_root/bin"
printf '{"files":[{"path":"tracked.go"}]}\n' >"$fixture/.canopy/index.json"
git -C "$fixture" rev-parse HEAD >"$fixture/.canopy/index.git-head"
fake_canopy="$test_root/bin/canopy"
cat >"$fake_canopy" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

: "${CANOPY_LOG:?CANOPY_LOG is required}"
printf 'argc=%s' "$#" >>"$CANOPY_LOG"
for argument in "$@"; do
	printf ' arg=%q' "$argument" >>"$CANOPY_LOG"
done
printf '\n' >>"$CANOPY_LOG"
EOF
chmod +x "$fake_canopy"

CANOPY_BIN="$fake_canopy" CANOPY_LOG="$test_root/canopy.log" \
	bash "$fixture/scripts/canopy_query.sh" search symbols "$fixture"
assert_not_contains 'arg=--no-cache' "$test_root/canopy.log"
pass 'ignored private and transient paths do not stale the Canopy cache'

printf 'package fixture\n' >"$fixture/visible.go"
: >"$test_root/canopy.log"
CANOPY_BIN="$fake_canopy" CANOPY_LOG="$test_root/canopy.log" \
	bash "$fixture/scripts/canopy_query.sh" search symbols "$fixture"
assert_contains 'arg=--no-cache' "$test_root/canopy.log"
pass 'an unignored source path still stales the Canopy cache'

bash -n "$repo_root/scripts/canopy_query.sh"
bash -n "$repo_root/scripts/refresh_canopy_index.sh"
bash -n "$repo_root/cgo_harness/docker/run_parity_in_docker.sh"
bash -n "$repo_root/cgo_harness/docker/run_parity_experiments.sh"
pass 'the changed shell entry points pass syntax checks'
