#!/usr/bin/env bash

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
repo_root=$(cd -- "$script_dir/.." && pwd -P)
runner="$script_dir/run_randomized_benchmarks.sh"
test_root=$(mktemp -d "${TMPDIR:-/tmp}/gotreesitter-bench-lock-test.XXXXXX")
mock_bin="$test_root/bin"
mkdir -p -- "$mock_bin"

first_pid=""
dash_pid=""
signal_pid=""
independent_a_pid=""
independent_b_pid=""
paired_pid=""
paired_release=""

cleanup() {
	local pid
	[[ -z "$paired_release" ]] || : >"$paired_release"
	for pid in "$first_pid" "$dash_pid" "$signal_pid" "$independent_a_pid" "$independent_b_pid" "$paired_pid"; do
		if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
			kill -TERM "$pid" 2>/dev/null || true
		fi
	done
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

assert_status() {
	local status=$1 file=$2
	[[ "$(tail -n 1 "$file")" == "# status: $status" ]] || fail "final status is not $status in $file"
}

line_count() {
	if [[ ! -f "$1" ]]; then
		printf '0\n'
		return
	fi
	wc -l <"$1" | tr -d ' '
}

wait_for_file() {
	local file=$1
	local attempt
	for ((attempt = 0; attempt < 500; attempt++)); do
		[[ -e "$file" ]] && return 0
		sleep 0.01
	done
	fail "timed out waiting for $file"
}

wait_for_status() {
	local pid=$1
	local status=0
	wait "$pid" || status=$?
	return "$status"
}

cat >"$mock_bin/go" <<'EOF'
#!/usr/bin/env bash

set -euo pipefail

: "${MOCK_GO_LOG:?MOCK_GO_LOG is required}"
printf 'pid=%s args=%s\n' "$$" "$*" >>"$MOCK_GO_LOG"
if [[ -n "${MOCK_GO_SEQUENCE_LOG:-}" ]]; then
	printf '%s\t%s\t%s\n' "$PWD" "${GOMAXPROCS:-}" "$*" >>"$MOCK_GO_SEQUENCE_LOG"
fi
if [[ -n "${MOCK_GO_FAIL_CALL:-}" ]] && [[ "$(wc -l <"$MOCK_GO_LOG")" -eq "$MOCK_GO_FAIL_CALL" ]]; then
	printf '%s\n' 'mock benchmark failure' >&2
	exit 23
fi
if [[ -n "${MOCK_GO_READY:-}" ]]; then
	: >"$MOCK_GO_READY"
fi
if [[ -n "${MOCK_GO_WAIT_FILE:-}" ]]; then
	while [[ ! -e "$MOCK_GO_WAIT_FILE" ]]; do
		sleep 0.01
	done
fi
case "${MOCK_GO_OUTPUT_KIND:-valid}" in
	none) printf '%s\n' 'PASS' ;;
	malformed) printf '%s\n' 'BenchmarkWitness-1 1 invalid ns/op 16 B/op 1 allocs/op' ;;
	numeric-subbenchmark) printf '%s\n' 'BenchmarkFixture/size-64 5 100 ns/op 16 B/op 1 allocs/op' ;;
	trio|partial-trio|duplicate-trio|invalid-trio)
		printf '%s\n' 'BenchmarkGoParseFullDFA-12 5 100 ns/op 16 B/op 1 allocs/op'
		printf '%s\n' 'BenchmarkGoParseIncrementalSingleByteEditDFA-12 5 20 ns/op 0 B/op 0 allocs/op'
		case "$MOCK_GO_OUTPUT_KIND" in
			partial-trio)
				if [[ "$(wc -l <"$MOCK_GO_LOG")" -eq 3 ]]; then
					exit 0
				fi
				;;
			duplicate-trio)
				printf '%s\n' 'BenchmarkGoParseFullDFA-12 5 100 ns/op 16 B/op 1 allocs/op'
				;;
			invalid-trio)
				printf '%s\n' 'BenchmarkGoParseIncrementalNoEditDFA-12 5 1 ns/op 0 allocs/op'
				exit 0
				;;
		esac
		printf '%s\n' 'BenchmarkGoParseIncrementalNoEditDFA-12 5 1 ns/op 0 B/op 0 allocs/op'
		;;
	*) printf '%s\n' 'BenchmarkWitness-1 1 100 ns/op 16 B/op 1 allocs/op' ;;
esac
EOF
chmod +x "$mock_bin/go"

real_date=$(command -v date)
cat >"$mock_bin/date" <<'EOF'
#!/usr/bin/env bash

set -euo pipefail

if [[ -n "${MOCK_DATE_AFTER_CALLS:-}" ]] && [[ -f "$MOCK_GO_LOG" ]] && \
	[[ "$(wc -l <"$MOCK_GO_LOG")" -eq "$MOCK_DATE_AFTER_CALLS" ]] && [[ ! -e "$MOCK_DATE_READY" ]]; then
	: >"$MOCK_DATE_READY"
	while [[ ! -e "$MOCK_DATE_RELEASE" ]]; do
		sleep 0.01
	done
fi
exec "$MOCK_REAL_DATE" "$@"
EOF
chmod +x "$mock_bin/date"
export MOCK_REAL_DATE="$real_date"

cd -- "$repo_root"

first_lock="$test_root/first.lock"
first_log="$test_root/first-go.log"
first_ready="$test_root/first-ready"
first_release="$test_root/first-release"
first_output="$test_root/first.txt"
first_stderr="$test_root/first.stderr"

PATH="$mock_bin:$PATH" \
MOCK_GO_LOG="$first_log" \
MOCK_GO_READY="$first_ready" \
MOCK_GO_WAIT_FILE="$first_release" \
bash "$runner" --output "$first_output" --runs 1 --seed-start 7 \
	--benchtime 1ns --lock-path "$first_lock" >"$test_root/first.stdout" 2>"$first_stderr" &
first_pid=$!
wait_for_file "$first_ready"
assert_contains "pid=$first_pid" "$first_lock"
assert_contains "started=" "$first_lock"
assert_contains "cwd=$repo_root" "$first_lock"
assert_contains "output=$first_output" "$first_lock"
if flock -n "$first_lock" -c ':'; then
	fail 'the first runner did not hold its lock'
fi

second_output="$test_root/second.txt"
second_stderr="$test_root/second.stderr"
second_status=0
PATH="$mock_bin:$PATH" \
MOCK_GO_LOG="$first_log" \
bash "$runner" --output "$second_output" --runs 1 --seed-start 8 \
	--benchtime 1ns --lock-path "$first_lock" >"$test_root/second.stdout" 2>"$second_stderr" || second_status=$?
[[ "$second_status" -eq 75 ]] || fail "second runner returned $second_status, want 75"
assert_contains 'benchmark lock is busy' "$second_stderr"
assert_contains "pid=$first_pid" "$second_stderr"
assert_contains "output=$first_output" "$second_stderr"
[[ "$(line_count "$first_log")" -eq 1 ]] || fail 'second runner invoked go'
pass 'the first runner owns the lock and the second runner fails before go'

: >"$first_release"
first_status=0
wait "$first_pid" || first_status=$?
[[ "$first_status" -eq 0 ]] || fail "first runner returned $first_status"
assert_status complete "$first_output"
assert_contains '# protocol: single-seed-processes' "$first_output"
assert_contains '# completed runs: 1' "$first_output"

printf '%s\n' \
	'pid=stale' \
	'started=1970-01-01T00:00:00Z' \
	'cwd=/stale/worktree' \
	'output=/stale/output' >"$first_lock"
PATH="$mock_bin:$PATH" \
MOCK_GO_LOG="$first_log" \
bash "$runner" --output "$test_root/stale-replacement.txt" --runs 1 --seed-start 9 \
	--benchtime 1ns --lock-path "$first_lock" >"$test_root/stale.stdout" 2>"$test_root/stale.stderr"
[[ "$(line_count "$first_log")" -eq 2 ]] || fail 'stale metadata blocked a released lock'
pass 'stale metadata does not block after lock release'

dash_lock='-benchmark.lock'
dash_lock_file="$test_root/$dash_lock"
dash_log="$test_root/dash-go.log"
dash_ready="$test_root/dash-ready"
dash_release="$test_root/dash-release"
dash_output="$test_root/dash-first.txt"
dash_second_stderr="$test_root/dash-second.stderr"
(
	cd -- "$test_root"
	PATH="$mock_bin:$PATH" \
	MOCK_GO_LOG="$dash_log" \
	MOCK_GO_READY="$dash_ready" \
	MOCK_GO_WAIT_FILE="$dash_release" \
	bash "$runner" --output "$dash_output" --runs 1 --seed-start 10 \
		--benchtime 1ns --lock-path "$dash_lock" >"$test_root/dash-first.stdout" \
		2>"$test_root/dash-first.stderr"
) &
dash_pid=$!
wait_for_file "$dash_ready"
if flock -n "$dash_lock_file" -c ':'; then
	fail 'the dash-prefixed lock was not held'
fi
dash_second_status=0
(
	cd -- "$test_root"
	PATH="$mock_bin:$PATH" \
	MOCK_GO_LOG="$dash_log" \
	bash "$runner" --output "$test_root/dash-second.txt" --runs 1 --seed-start 11 \
		--benchtime 1ns --lock-path "$dash_lock" >"$test_root/dash-second.stdout" \
		2>"$dash_second_stderr"
) || dash_second_status=$?
[[ "$dash_second_status" -eq 75 ]] || fail "dash-prefixed runner returned $dash_second_status, want 75"
assert_contains "benchmark lock is busy: $dash_lock" "$dash_second_stderr"
assert_contains 'owner: pid=' "$dash_second_stderr"
assert_contains 'owner: started=' "$dash_second_stderr"
assert_contains "owner: cwd=$test_root" "$dash_second_stderr"
assert_contains "owner: output=$dash_output" "$dash_second_stderr"
[[ "$(line_count "$dash_log")" -eq 1 ]] || fail 'dash-prefixed second runner invoked go'
: >"$dash_release"
dash_status=0
wait "$dash_pid" || dash_status=$?
[[ "$dash_status" -eq 0 ]] || fail "dash-prefixed first runner returned $dash_status"
pass 'dash-prefixed lock paths report owner metadata before go'

signal_lock="$test_root/signal.lock"
signal_log="$test_root/signal-go.log"
signal_ready="$test_root/signal-ready"
signal_release="$test_root/signal-release"
PATH="$mock_bin:$PATH" \
MOCK_GO_LOG="$signal_log" \
MOCK_GO_READY="$signal_ready" \
MOCK_GO_WAIT_FILE="$signal_release" \
setsid bash "$runner" --output "$test_root/signal.txt" --runs 1 --seed-start 10 \
	--benchtime 1ns --lock-path "$signal_lock" >"$test_root/signal.stdout" 2>"$test_root/signal.stderr" &
signal_pid=$!
wait_for_file "$signal_ready"
kill -TERM -- "-$signal_pid" 2>/dev/null || kill -TERM "$signal_pid"
signal_status=0
wait "$signal_pid" || signal_status=$?
[[ "$signal_status" -ne 0 ]] || fail 'signal termination returned success'
assert_status incomplete "$test_root/signal.txt"
if ! flock -n "$signal_lock" -c ':'; then
	fail 'the signal path did not release the lock'
fi
PATH="$mock_bin:$PATH" \
MOCK_GO_LOG="$signal_log" \
bash "$runner" --output "$test_root/signal-replacement.txt" --runs 1 --seed-start 11 \
	--benchtime 1ns --lock-path "$signal_lock" >"$test_root/signal-replacement.stdout" \
	2>"$test_root/signal-replacement.stderr"
[[ "$(line_count "$signal_log")" -eq 2 ]] || fail 'the released signal lock rejected a new runner'
pass 'signal cleanup releases the lock'

independent_a_lock="$test_root/independent-a.lock"
independent_b_lock="$test_root/independent-b.lock"
independent_a_log="$test_root/independent-a-go.log"
independent_b_log="$test_root/independent-b-go.log"
independent_a_ready="$test_root/independent-a-ready"
independent_b_ready="$test_root/independent-b-ready"
independent_a_release="$test_root/independent-a-release"
independent_b_release="$test_root/independent-b-release"
PATH="$mock_bin:$PATH" \
MOCK_GO_LOG="$independent_a_log" \
MOCK_GO_READY="$independent_a_ready" \
MOCK_GO_WAIT_FILE="$independent_a_release" \
bash "$runner" --output "$test_root/independent-a.txt" --runs 1 --seed-start 12 \
	--benchtime 1ns --lock-path "$independent_a_lock" >"$test_root/independent-a.stdout" \
	2>"$test_root/independent-a.stderr" &
independent_a_pid=$!
PATH="$mock_bin:$PATH" \
MOCK_GO_LOG="$independent_b_log" \
MOCK_GO_READY="$independent_b_ready" \
MOCK_GO_WAIT_FILE="$independent_b_release" \
bash "$runner" --output "$test_root/independent-b.txt" --runs 1 --seed-start 13 \
	--benchtime 1ns --lock-path "$independent_b_lock" >"$test_root/independent-b.stdout" \
	2>"$test_root/independent-b.stderr" &
independent_b_pid=$!
wait_for_file "$independent_a_ready"
wait_for_file "$independent_b_ready"
if flock -n "$independent_a_lock" -c ':'; then
	fail 'explicit lock path A was not held'
fi
if flock -n "$independent_b_lock" -c ':'; then
	fail 'explicit lock path B was not held'
fi
: >"$independent_a_release"
: >"$independent_b_release"
independent_a_status=0
independent_b_status=0
wait "$independent_a_pid" || independent_a_status=$?
wait "$independent_b_pid" || independent_b_status=$?
[[ "$independent_a_status" -eq 0 ]] || fail "lock path A returned $independent_a_status"
[[ "$independent_b_status" -eq 0 ]] || fail "lock path B returned $independent_b_status"
[[ "$(line_count "$independent_a_log")" -eq 1 ]] || fail 'lock path A did not invoke its mock go'
[[ "$(line_count "$independent_b_log")" -eq 1 ]] || fail 'lock path B did not invoke its mock go'
pass 'distinct explicit lock paths run independently'

baseline_root="$test_root/baseline checkout/cgo_harness"
mkdir -p -- "$baseline_root"
head_root="$repo_root/cgo_harness"
paired_log="$test_root/paired-go.log"
paired_sequence="$test_root/paired-sequence.txt"
paired_head="$test_root/paired-head.txt"
paired_base="$test_root/paired-base.txt"
paired_bench='^BenchmarkWitness(A|B)$'
(
	cd -- "$head_root"
	PATH="$mock_bin:$PATH" MOCK_GO_LOG="$paired_log" MOCK_GO_SEQUENCE_LOG="$paired_sequence" \
	bash "$runner" --output "$paired_head" --baseline-root "$baseline_root" \
		--baseline-output "$paired_base" --runs 4 --seed-start 17 --benchtime 37ms \
		--tags fixture,other --package ./cmd/fixture --bench-regex "$paired_bench" \
		--lock-path "$test_root/paired.lock" >"$test_root/paired.stdout" 2>"$test_root/paired.stderr"
)
{
	for ((offset = 0; offset < 4; offset++)); do
		seed=$((17 + offset))
		if ((offset % 2 == 0)); then
			roots=("$baseline_root" "$head_root")
		else
			roots=("$head_root" "$baseline_root")
		fi
		for root in "${roots[@]}"; do
			printf '%s\t1\ttest -tags fixture,other ./cmd/fixture -run ^$ -bench %s -benchmem -count=1 -benchtime=37ms -shuffle=%d\n' \
				"$root" "$paired_bench" "$seed"
		done
	done
} >"$test_root/paired-expected.txt"
cmp "$paired_sequence" "$test_root/paired-expected.txt" || fail 'paired order, directory, environment, or arguments differ'
for file in "$paired_head" "$paired_base"; do
	assert_status complete "$file"
	assert_contains '# protocol: paired-alternating-seeds' "$file"
	assert_contains '# completed runs: 4' "$file"
	assert_contains '# completed seed: 20;' "$file"
done
assert_contains "# working directory: $head_root" "$paired_head"
assert_contains "# repository root: $repo_root" "$paired_head"
assert_contains "# revision: $(git -C "$repo_root" rev-parse HEAD)" "$paired_head"
assert_contains '# repository root: unavailable' "$paired_base"
assert_contains '# dirty: unknown' "$paired_base"
assert_contains '# source metadata: informational; no source snapshot authentication' "$paired_head"
assert_contains '# seed: 17; position: 1' "$paired_base"
assert_contains '# seed: 18; position: 1' "$paired_head"
pass 'paired seeds alternate directories with identical settings and separate source metadata'

reject_count=0
expect_rejected() {
	local status=0 head
	reject_count=$((reject_count + 1))
	head="$test_root/rejected-head-$reject_count.txt"
	PATH="$mock_bin:$PATH" MOCK_GO_LOG="$test_root/rejected-go.log" \
	bash "$runner" --output "$head" --runs 1 --lock-path "$test_root/rejected.lock" "$@" \
		>"$test_root/rejected.stdout" 2>"$test_root/rejected.stderr" || status=$?
	[[ "$status" -eq 2 ]] || fail "invalid paired arguments returned $status, want 2"
	[[ ! -e "$head" ]] || fail 'invalid arguments created an output'
	[[ ! -e "$test_root/rejected-go.log" ]] || fail 'invalid arguments invoked go'
}
expect_rejected --baseline-root "$baseline_root"
expect_rejected --baseline-output "$test_root/unused.txt"
expect_rejected --baseline-root '' --baseline-output "$test_root/unused.txt"
expect_rejected --baseline-root "$test_root/missing" --baseline-output "$test_root/unused.txt"
expect_rejected --baseline-root "$baseline_root" --baseline-output "$paired_base"
expect_rejected --baseline-root "$baseline_root" --baseline-output "$test_root/rejected-head-6.txt"
mkdir -p "$test_root/real-output"
ln -s "$test_root/real-output" "$test_root/output-alias"
expect_rejected --output "$test_root/real-output/new.txt" --baseline-root "$baseline_root" \
	--baseline-output "$test_root/output-alias/new.txt"
[[ ! -e "$test_root/real-output/new.txt" ]] || fail 'aliased outputs created a file'
ln -s "$test_root/dangling-target.txt" "$test_root/dangling-output.txt"
expect_rejected --baseline-root "$baseline_root" --baseline-output "$test_root/dangling-output.txt"
[[ ! -e "$test_root/dangling-target.txt" ]] || fail 'a dangling output symlink was followed'
expect_rejected --output "$test_root/rejected.lock"
expect_rejected --require-benchmarks ''
expect_rejected --require-benchmarks 'BenchmarkWitness,BenchmarkWitness'
expect_rejected --require-benchmarks 'BenchmarkWitness,'
expect_rejected --require-benchmarks 'BenchmarkWitness, AnotherBenchmark'
pass 'paired options and output collisions fail before file creation or go'

failed_log="$test_root/failed-go.log"
failed_head="$test_root/failed-head.txt"
failed_base="$test_root/failed-base.txt"
failed_status=0
PATH="$mock_bin:$PATH" MOCK_GO_LOG="$failed_log" MOCK_GO_FAIL_CALL=3 \
bash "$runner" --output "$failed_head" --baseline-root "$baseline_root" --baseline-output "$failed_base" \
	--runs 3 --seed-start 17 --tags '' --lock-path "$test_root/failed.lock" \
	>"$test_root/failed.stdout" 2>"$test_root/failed.stderr" || failed_status=$?
[[ "$failed_status" -eq 23 ]] || fail "partial failure returned $failed_status, want 23"
[[ "$(line_count "$failed_log")" -eq 3 ]] || fail 'the runner continued after a failed benchmark'
for file in "$failed_head" "$failed_base"; do
	assert_status incomplete "$file"
	assert_contains '# completed seed: 17;' "$file"
	assert_not_contains '# completed seed: 18;' "$file"
	assert_not_contains '# status: complete' "$file"
done
assert_contains 'args=test -tags  . -run ^$' "$failed_log"
if ! flock -n "$test_root/failed.lock" -c ':'; then
	fail 'a failed pair retained the host lock'
fi
pass 'partial paired failure preserves diagnostics and leaves both outputs incomplete'

for output_kind in none malformed; do
	empty_status=0
	PATH="$mock_bin:$PATH" MOCK_GO_LOG="$test_root/$output_kind-go.log" MOCK_GO_OUTPUT_KIND="$output_kind" \
	bash "$runner" --output "$test_root/$output_kind-head.txt" --baseline-root "$baseline_root" \
		--baseline-output "$test_root/$output_kind-base.txt" --runs 2 --lock-path "$test_root/$output_kind.lock" \
		>"$test_root/$output_kind.stdout" 2>"$test_root/$output_kind.stderr" || empty_status=$?
	[[ "$empty_status" -eq 1 ]] || fail "$output_kind benchmark output returned $empty_status, want 1"
	[[ "$(line_count "$test_root/$output_kind-go.log")" -eq 1 ]] || fail 'invalid benchmark output did not stop the campaign'
	assert_status incomplete "$test_root/$output_kind-head.txt"
	assert_status incomplete "$test_root/$output_kind-base.txt"
	assert_contains 'produced no valid benchmark timing row' "$test_root/$output_kind.stderr"
done
pass 'successful processes without valid timing rows cannot complete a campaign'

required_trio='BenchmarkGoParseFullDFA,BenchmarkGoParseIncrementalSingleByteEditDFA,BenchmarkGoParseIncrementalNoEditDFA'
for output_kind in trio partial-trio duplicate-trio invalid-trio; do
	trio_status=0
	PATH="$mock_bin:$PATH" MOCK_GO_LOG="$test_root/$output_kind-go.log" MOCK_GO_OUTPUT_KIND="$output_kind" \
	bash "$runner" --output "$test_root/$output_kind-head.txt" --baseline-root "$baseline_root" \
		--baseline-output "$test_root/$output_kind-base.txt" --runs 2 --seed-start 17 \
		--require-benchmarks "$required_trio" --lock-path "$test_root/$output_kind.lock" \
		>"$test_root/$output_kind.stdout" 2>"$test_root/$output_kind.stderr" || trio_status=$?
	if [[ "$output_kind" == trio ]]; then
		[[ "$trio_status" -eq 0 ]] || fail "the complete benchmark set returned $trio_status"
		assert_status complete "$test_root/$output_kind-head.txt"
		assert_status complete "$test_root/$output_kind-base.txt"
		[[ "$(line_count "$test_root/$output_kind-go.log")" -eq 4 ]] || fail 'the complete set did not finish both seeds'
	else
		[[ "$trio_status" -eq 1 ]] || fail "$output_kind returned $trio_status, want 1"
		assert_status incomplete "$test_root/$output_kind-head.txt"
		assert_status incomplete "$test_root/$output_kind-base.txt"
		assert_contains 'required benchmark BenchmarkGoParse' "$test_root/$output_kind.stderr"
	fi
done
[[ "$(line_count "$test_root/partial-trio-go.log")" -eq 3 ]] || fail 'a partial later seed did not stop the campaign'
assert_contains '# completed seed: 17;' "$test_root/partial-trio-head.txt"
assert_not_contains '# completed seed: 18;' "$test_root/partial-trio-head.txt"
pass 'required names need one complete metric row in every seed, including later seeds'

PATH="$mock_bin:$PATH" MOCK_GO_LOG="$test_root/numeric-subbenchmark.log" MOCK_GO_OUTPUT_KIND=numeric-subbenchmark \
bash "$runner" --output "$test_root/numeric-subbenchmark.txt" --runs 1 \
	--require-benchmarks 'BenchmarkFixture/size-64' --lock-path "$test_root/numeric-subbenchmark.lock" \
	>"$test_root/numeric-subbenchmark.stdout" 2>"$test_root/numeric-subbenchmark.stderr"
assert_status complete "$test_root/numeric-subbenchmark.txt"
pass 'exact numeric subbenchmark names take precedence over CPU suffix normalization'

paired_lock="$test_root/paired-interval.lock"
paired_ready="$test_root/paired-interval-ready"
paired_release="$test_root/paired-interval-release"
interval_log="$test_root/paired-interval-go.log"
PATH="$mock_bin:$PATH" MOCK_GO_LOG="$interval_log" MOCK_DATE_AFTER_CALLS=2 \
MOCK_DATE_READY="$paired_ready" MOCK_DATE_RELEASE="$paired_release" \
bash "$runner" --output "$test_root/interval-head.txt" --baseline-root "$baseline_root" \
	--baseline-output "$test_root/interval-base.txt" --runs 2 --lock-path "$paired_lock" \
	>"$test_root/interval.stdout" 2>"$test_root/interval.stderr" &
paired_pid=$!
wait_for_file "$paired_ready"
[[ "$(line_count "$interval_log")" -eq 2 ]] || fail 'the pause did not occur between seed pairs'
if flock -n "$paired_lock" -c ':'; then
	fail 'the host lock was released between seed pairs'
fi
interval_status=0
PATH="$mock_bin:$PATH" MOCK_GO_LOG="$interval_log" \
bash "$runner" --output "$test_root/interval-contender.txt" --runs 1 --lock-path "$paired_lock" \
	>"$test_root/interval-contender.stdout" 2>"$test_root/interval-contender.stderr" || interval_status=$?
[[ "$interval_status" -eq 75 ]] || fail "the between-pair contender returned $interval_status, want 75"
assert_contains "baseline_root=$baseline_root" "$test_root/interval-contender.stderr"
[[ "$(line_count "$interval_log")" -eq 2 ]] || fail 'the contender invoked go between seed pairs'
: >"$paired_release"
wait "$paired_pid" || fail 'the paired campaign failed after resuming'
[[ "$(line_count "$interval_log")" -eq 4 ]] || fail 'the paired campaign did not finish both seeds'
assert_status complete "$test_root/interval-head.txt"
assert_status complete "$test_root/interval-base.txt"
pass 'one host lock covers the interval between paired seeds'

assert_not_contains 'go test' "$test_root/second.stderr"
printf '%s\n' 'all randomized benchmark runner tests passed'
