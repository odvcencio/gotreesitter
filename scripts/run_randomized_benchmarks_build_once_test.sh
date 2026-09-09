#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
runner="$script_dir/run_randomized_benchmarks.sh"
test_root=$(mktemp -d "${TMPDIR:-/tmp}/gotreesitter-build-once-test.XXXXXX")
signal_pid=""
cleanup() {
 if [[ -n "$signal_pid" ]] && kill -0 "$signal_pid" 2>/dev/null; then
  kill -TERM -- "-$signal_pid" 2>/dev/null || true
  wait "$signal_pid" 2>/dev/null || true
 fi
 rm -rf -- "$test_root"
}
trap cleanup EXIT
fail() { printf 'not ok - %s\n' "$1" >&2; exit 1; }
pass() { printf 'ok - %s\n' "$1"; }
assert_status() {
 [[ "$(tail -1 "$1")" == "# status: $2" ]] || fail "wrong status in $1"
}
assert_clean() {
 if compgen -G "$test_root/tmp/gotreesitter-benchmark-build.*" >/dev/null; then
  fail 'temporary test binaries remain'
 fi
 flock -n "$test_root/run.lock" -c ':' || fail 'the campaign retained its lock'
}
mkdir -p "$test_root/tmp" "$test_root/bin"
export TMPDIR="$test_root/tmp"
export REAL_GO
REAL_GO=$(command -v go)
export BUILD_COMMAND_LOG="$test_root/go.log"
cat > "$test_root/bin/go" <<'GO_WRAPPER'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\t%s\n' "$PWD" "$*" >> "$BUILD_COMMAND_LOG"
exec "$REAL_GO" "$@"
GO_WRAPPER
chmod +x "$test_root/bin/go"
export PATH="$test_root/bin:$PATH"
export BENCH_PROCESS_LOG="$test_root/processes.log"

for lane in baseline head; do
 root="$test_root/$lane checkout"
 mkdir -p "$root/cmd/fixture"
 printf 'module example.org/benchmarkfixture\n\ngo 1.25\n' > "$root/go.mod"
 printf '%s' "$lane" > "$root/cmd/fixture/fixture.txt"
 cat > "$root/cmd/fixture/fixture_test.go" <<GO_FIXTURE
package fixture

import (
 "fmt"
 "os"
 "runtime"
 "testing"
 "time"
)

const lane = "$lane"
var result int

func TestMain(m *testing.M) {
 cwd, err := os.Getwd()
 if err != nil { panic(err) }
 f, err := os.OpenFile(os.Getenv("BENCH_PROCESS_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
 if err != nil { panic(err) }
 fmt.Fprintf(f, "%s\t%d\t%s\n", lane, os.Getpid(), cwd)
 if err := f.Close(); err != nil { panic(err) }
 os.Exit(m.Run())
}

func BenchmarkWitness(b *testing.B) {
 data, err := os.ReadFile("fixture.txt")
 if err != nil || string(data) != lane {
  b.Fatalf("package fixture: data=%q err=%v", data, err)
 }
 if runtime.GOMAXPROCS(0) != 1 || b.N != 1 {
  b.Fatalf("process settings: GOMAXPROCS=%d N=%d", runtime.GOMAXPROCS(0), b.N)
 }
 if os.Getenv("BENCH_FAIL_LANE") == lane { b.Fatal("requested benchmark failure") }
 if ready := os.Getenv("BENCH_WAIT_READY"); ready != "" {
  if err := os.WriteFile(ready, nil, 0600); err != nil { b.Fatal(err) }
  for { time.Sleep(time.Millisecond) }
 }
 if os.Getenv("BENCH_REMOVE_BINARY") == lane {
  path, err := os.Executable()
  if err != nil { b.Fatal(err) }
  if err := os.Remove(path); err != nil { b.Fatal(err) }
 }
 b.ReportAllocs()
 b.ResetTimer()
 for range b.N { result++ }
}
GO_FIXTURE
done

head_root="$test_root/head checkout"
baseline_root="$test_root/baseline checkout"
run_pair() {
 local label=$1
 shift
 (
  cd "$head_root"
  bash "$runner" --build-once --output "$test_root/$label-head.txt" \
   --baseline-root "$baseline_root" --baseline-output "$test_root/$label-base.txt" \
   --package ./cmd/fixture --tags '' --bench-regex '^BenchmarkWitness$' \
   --require-benchmarks BenchmarkWitness --benchtime 1x --runs 4 --seed-start 17 \
   --lock-path "$test_root/run.lock" "$@"
 ) > "$test_root/$label.stdout" 2> "$test_root/$label.stderr"
}

run_pair complete
[[ "$(grep -c $'\ttest -c ' "$BUILD_COMMAND_LOG")" == 2 ]] || fail 'each checkout must compile exactly once'
[[ "$(wc -l < "$BENCH_PROCESS_LOG")" == 8 ]] || fail 'four pairs must run eight test processes'
[[ "$(cut -f2 "$BENCH_PROCESS_LOG" | sort -u | wc -l)" == 8 ]] || fail 'a seed did not use a fresh process'
printf '%s\n' baseline head head baseline baseline head head baseline > "$test_root/expected-order"
cut -f1 "$BENCH_PROCESS_LOG" > "$test_root/actual-order"
cmp "$test_root/expected-order" "$test_root/actual-order" || fail 'paired execution order changed'
while IFS=$'\t' read -r lane pid cwd; do
 [[ "$cwd" == "$test_root/$lane checkout/cmd/fixture" ]] || fail 'the binary used the checkout root instead of the package directory'
done < "$BENCH_PROCESS_LOG"
for output in "$test_root/complete-head.txt" "$test_root/complete-base.txt"; do
 assert_status "$output" complete
 grep -Eq '^# test binary sha256: [0-9a-f]{64}$' "$output" || fail 'missing binary identity'
 grep -F '# completed seed: 20;' "$output" >/dev/null || fail 'missing final seed'
done
assert_clean
pass 'one build per checkout preserves package directories, fresh processes, settings, and paired order'

: > "$BENCH_PROCESS_LOG"
status=0
BENCH_FAIL_LANE=head run_pair failed || status=$?
[[ "$status" != 0 ]] || fail 'a failed benchmark completed'
[[ "$(wc -l < "$BENCH_PROCESS_LOG")" == 2 ]] || fail 'the campaign continued after a failed benchmark'
assert_status "$test_root/failed-head.txt" incomplete
assert_status "$test_root/failed-base.txt" incomplete
assert_clean
pass 'benchmark failure preserves incomplete outputs and removes both binaries'

: > "$BENCH_PROCESS_LOG"
status=0
BENCH_REMOVE_BINARY=head run_pair missing-binary --runs 1 || status=$?
[[ "$status" != 0 ]] || fail 'a missing measured binary passed identity verification'
assert_status "$test_root/missing-binary-head.txt" incomplete
assert_status "$test_root/missing-binary-base.txt" incomplete
assert_clean
pass 'binary identity verification fails closed'

for flags in '-p=1 -timeout=0' '-p=1 -exec=ignored' '-p=1 -short'; do
 status=0
 label="flags-${flags##*-}"
 GOFLAGS="$flags" run_pair "$label" || status=$?
 [[ "$status" == 2 ]] || fail 'implicit test execution flags were silently dropped'
 assert_status "$test_root/$label-head.txt" incomplete
 assert_status "$test_root/$label-base.txt" incomplete
 assert_clean
done
pass 'test execution flags in GOFLAGS require the original Go-command mode'

printf 'invalid Go source\n' > "$baseline_root/cmd/fixture/broken.go"
: > "$BENCH_PROCESS_LOG"
status=0
run_pair compile-failed || status=$?
[[ "$status" != 0 ]] || fail 'a compile failure completed'
[[ ! -s "$BENCH_PROCESS_LOG" ]] || fail 'a benchmark ran after a compile failure'
assert_status "$test_root/compile-failed-head.txt" incomplete
assert_status "$test_root/compile-failed-base.txt" incomplete
assert_clean
rm "$baseline_root/cmd/fixture/broken.go"
pass 'compile failures release the lock and remove temporary binaries'

mkdir -p "$baseline_root/other"
printf 'package other\n' > "$baseline_root/other/other.go"
status=0
run_pair multiple --package ./... || status=$?
[[ "$status" == 2 ]] || fail 'multiple packages did not receive an explicit error'
assert_status "$test_root/multiple-head.txt" incomplete
assert_status "$test_root/multiple-base.txt" incomplete
assert_clean
pass 'build-once rejects multiple packages without selecting one silently'

ready="$test_root/signal-ready"
(
 cd "$head_root"
 export BENCH_WAIT_READY="$ready"
 exec setsid bash "$runner" --build-once --output "$test_root/signal.txt" \
  --package ./cmd/fixture --tags '' --bench-regex '^BenchmarkWitness$' \
  --require-benchmarks BenchmarkWitness --benchtime 1x --runs 2 \
  --lock-path "$test_root/run.lock"
) > "$test_root/signal.stdout" 2> "$test_root/signal.stderr" &
signal_pid=$!
for ((attempt=0; attempt<1000; attempt++)); do
 [[ -e "$ready" ]] && break
 sleep 0.01
done
[[ -e "$ready" ]] || fail 'the signal fixture did not start'
kill -TERM -- "-$signal_pid"
status=0
wait "$signal_pid" || status=$?
signal_pid=""
[[ "$status" != 0 ]] || fail 'signal termination completed'
assert_status "$test_root/signal.txt" incomplete
assert_clean
pass 'signal termination removes the binary and releases the lock'
printf '%s\n' 'all build-once benchmark runner tests passed'
