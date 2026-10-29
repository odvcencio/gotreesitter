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

# Inspect the index so this check also rejects forced additions.
if [[ -n "$(git -C "$repo_root" ls-files -- docs/benchmarks/ docs/perf/ docs/performance/ cgo_harness/perf_scan/evidence/ cgo_harness/perf_scan/out/ harness_out/)" ]]; then
	fail 'performance evidence archives must remain outside the repository'
fi
pass 'the index excludes performance evidence archives'

bash "$script_dir/check_evidence_paths.sh" >/dev/null || fail 'the tracked tree holds evidence paths'
pass 'the tracked tree holds no evidence paths'

snapshot_paths=(
	docs/perf-attribution-receipt.json
	cgo_harness/perf_scan/wave3_sweep_status.json
	cgo_harness/perf_scan/wave3_sweep_status.md
	cgo_harness/testdata/canonical_incremental_before_state_receipt_v1.json
	cgo_harness/perf_scan/out/canonical_incremental_before_state_receipt_v1.json
	cgo_harness/perf_scan/out/wave3_sweep_status.json
	cgo_harness/perf_scan/out/wave3_sweep_status.md
	cgo_harness/perf_scan/out/authoritative_YYYYMMDDTHHMMSSZ/scoreboard.json
)
if [[ -n "$(git -C "$repo_root" ls-files -- "${snapshot_paths[@]}")" ]]; then
	fail 'generated performance snapshots must remain outside the repository'
fi
for snapshot in "${snapshot_paths[@]}"; do
	git -C "$repo_root" check-ignore --no-index --quiet -- "$snapshot" || fail "generated snapshot is not ignored: $snapshot"
done
for input in \
	cgo_harness/perf_scan/perf_ratio_budgets.json \
	cgo_harness/perf_scan/corpus_sources.lock.sha256 \
	cgo_harness/testdata/canonical_go_incremental_edits.json \
	cgo_harness/work_count/paired_semantic_trace_receipt_v1.json \
	cgo_harness/tier_scan/external_lex_elections.json; do
	if git -C "$repo_root" check-ignore --no-index --quiet -- "$input"; then
		fail "required input is ignored: $input"
	fi
done
pass 'generated snapshots stay ignored while budget and test inputs remain available'

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
cp -- "$repo_root/.canopyignore" "$fixture/.canopyignore"
cp -- "$repo_root/.gitignore" "$fixture/.gitignore"
cp -- "$repo_root/scripts/canopy_query.sh" "$fixture/scripts/canopy_query.sh"
git -C "$fixture" init -q
git -C "$fixture" config user.email test@example.invalid
git -C "$fixture" config user.name 'Hygiene Test'
git -C "$fixture" add .canopyignore .gitignore scripts/canopy_query.sh
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
	docs/performance
	docs/perf
	cgo_harness/perf_scan/evidence
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

# Evidence guard: build a small repository and check both modes.
guard_repo="$test_root/guard-fixture"
mkdir -p -- "$guard_repo/scripts" "$guard_repo/docs/changelog"
cp -- "$script_dir/check_evidence_paths.sh" "$guard_repo/scripts/check_evidence_paths.sh"
printf '# Guide\n' >"$guard_repo/docs/guide.md"
printf '# Maintenance\n' >"$guard_repo/docs/compact-parser-maintenance.md"
git -C "$guard_repo" init -q
git -C "$guard_repo" config user.email test@example.invalid
git -C "$guard_repo" config user.name 'Hygiene Test'
git -C "$guard_repo" add -A
git -C "$guard_repo" commit -qm base
guard_base=$(git -C "$guard_repo" rev-parse HEAD)
guard="$guard_repo/scripts/check_evidence_paths.sh"

bash "$guard" >/dev/null || fail 'the evidence guard rejected a clean tree'
bash "$guard" "$guard_base" >/dev/null || fail 'the evidence guard rejected a clean diff'
pass 'the evidence guard accepts a tree with user-facing docs'

# guard_rejects NAME PATH [BYTES]: add one tracked file, then expect both modes to fail.
guard_rejects() {
	local name=$1 path=$2 bytes=${3:-16}
	mkdir -p -- "$guard_repo/$(dirname -- "$path")"
	head -c "$bytes" /dev/zero | tr '\0' 'x' >"$guard_repo/$path"
	git -C "$guard_repo" add -f -- "$path"
	if bash "$guard" >"$test_root/guard.out" 2>&1; then
		fail "the evidence guard accepted $name in tree mode"
	fi
	assert_contains "$path" "$test_root/guard.out"
	git -C "$guard_repo" commit -qm "add $name"
	if bash "$guard" "$guard_base" >"$test_root/guard.out" 2>&1; then
		fail "the evidence guard accepted $name in base mode"
	fi
	assert_contains "$path" "$test_root/guard.out"
	git -C "$guard_repo" reset -q --hard "$guard_base"
	pass "the evidence guard rejects $name"
}
guard_rejects 'a benchmarks folder file' docs/benchmarks/zz-probe/x.txt
guard_rejects 'a receipt-named doc with a date' docs/zz-probe-receipt-20261027.md
guard_rejects 'a certificate-named doc' docs/zz-probe-certificate-v1.md
guard_rejects 'a dated doc' docs/zz-probe-20261027.md
guard_rejects 'a compact- prefixed doc' docs/compact-zz-probe.md
guard_rejects 'an archive under docs' docs/zz-probe.tar.gz
guard_rejects 'a raw log under docs' docs/zz-probe.log
guard_rejects 'a docs file over 200,000 bytes' docs/zz-big.md 200001

mkdir -p -- "$guard_repo/docs/changelog"
head -c 300000 /dev/zero | tr '\0' 'x' >"$guard_repo/docs/changelog/archive-9.md"
git -C "$guard_repo" add -f -- docs/changelog/archive-9.md
bash "$guard" >/dev/null || fail 'the evidence guard rejected a large changelog archive'
git -C "$guard_repo" reset -q --hard "$guard_base"
git -C "$guard_repo" clean -fdq
pass 'the evidence guard allows large changelog archives'

# Allowlist: it spares a named file, must name a tracked file, and may not grow.
printf 'x\n' >"$guard_repo/docs/old-receipt-20260101.md"
printf 'docs/old-receipt-20260101.md # kept until retirement\n' >"$guard_repo/scripts/evidence_paths_allowlist.txt"
git -C "$guard_repo" add -f -- docs/old-receipt-20260101.md scripts/evidence_paths_allowlist.txt
git -C "$guard_repo" commit -qm 'allowlist one file'
allow_base=$(git -C "$guard_repo" rev-parse HEAD)
bash "$guard" >/dev/null || fail 'the evidence guard rejected an allowlisted file'
bash "$guard" "$allow_base" >/dev/null || fail 'the evidence guard rejected an unchanged allowlist'
printf 'docs/old-receipt-20260101.md # kept\ndocs/guide.md # new entry\n' >"$guard_repo/scripts/evidence_paths_allowlist.txt"
git -C "$guard_repo" commit -qam 'grow allowlist'
if bash "$guard" "$allow_base" >"$test_root/guard.out" 2>&1; then
	fail 'the evidence guard accepted a new allowlist entry'
fi
assert_contains 'may only shrink' "$test_root/guard.out"
git -C "$guard_repo" reset -q --hard "$allow_base"
git -C "$guard_repo" rm -q -f -- docs/old-receipt-20260101.md
if bash "$guard" >"$test_root/guard.out" 2>&1; then
	fail 'the evidence guard accepted a stale allowlist entry'
fi
assert_contains 'names no tracked file' "$test_root/guard.out"
pass 'the evidence guard honors a shrink-only allowlist'
