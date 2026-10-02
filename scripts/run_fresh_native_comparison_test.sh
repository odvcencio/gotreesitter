#!/usr/bin/env bash
set -euo pipefail
script_dir=$(cd -- "$(dirname -- "$0")" && pwd -P)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT
mkdir -p "$test_dir/after/scripts" "$test_dir/after/cgo_harness" "$test_dir/before" "$test_dir/bin"
cp "$script_dir/run_fresh_native_comparison.sh" "$test_dir/after/scripts/"
printf 'after\n' > "$test_dir/after/.role"
printf 'before\n' > "$test_dir/before/.role"
printf 'c\n' > "$test_dir/after/cgo_harness/.role"
export GTS_NATIVE_TEST_TRACE="$test_dir/trace"
export GTS_NATIVE_TEST_BUSY_MARKER="$test_dir/busy"
cat > "$test_dir/bin/sleep" <<'SH'
#!/usr/bin/env bash
[[ "$1" == 5 ]]
SH
chmod +x "$test_dir/bin/sleep"
export PATH="$test_dir/bin:$PATH"
cat > "$test_dir/after/scripts/run_randomized_benchmarks.sh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
output= seed= required= runs= tags=
while (($#)); do
 case "$1" in
 --output) output=$2;;
 --seed-start) seed=$2;;
 --require-benchmarks) required=$2;;
 --runs) runs=$2;;
 --tags) tags=$2;;
 --bench-regex) ;;
 *) exit 2;;
 esac
 shift 2
done
[[ "$GOWORK" == off && "$GOMAXPROCS" == 1 && "$runs" == 1 ]]
[[ "$tags" == 'treesitter_c_parity treesitter_c_perfscan' ]]
if [[ "$GTS_NATIVE_TEST_BUSY" == 1 && ! -e "$GTS_NATIVE_TEST_BUSY_MARKER" ]]; then
 touch "$GTS_NATIVE_TEST_BUSY_MARKER"
 exit 75
fi
role=$(cat .role)
case "$output" in
 */go-before-*) [[ "$role" == before ]];;
 */go-after-*) [[ "$role" == after ]];;
 */c-control1-*) [[ "$role" == c ]]; role=control1;;
 */c-control2-*) [[ "$role" == c ]]; role=control2;;
 *) exit 2;;
esac
printf '%s %s\n' "$seed" "$role" >> "$GTS_NATIVE_TEST_TRACE"
[[ ! -e "$output" ]]
name_words=$(tr ',' ' ' <<< "$required")
for name in $name_words; do
 if [[ "$GTS_NATIVE_TEST_MISSING" == 1 && "$role" == control1 ]]; then continue; fi
 printf '%s 1 100 ns/op 8 B/op 2 allocs/op\n' "$name" >> "$output"
 if [[ "$GTS_NATIVE_TEST_DUPLICATE" == 1 && "$role" == control1 ]]; then
  printf '%s 1 100 ns/op 8 B/op 2 allocs/op\n' "$name" >> "$output"
 fi
done
touch "$output"
SH
export GTS_NATIVE_TEST_BUSY=1 GTS_NATIVE_TEST_MISSING=0 GTS_NATIVE_TEST_DUPLICATE=0
driver="$test_dir/after/scripts/run_fresh_native_comparison.sh"
bash "$driver" "$test_dir/before" "$test_dir/full" java > /dev/null
awk '
 {
  seed=int((NR-1)/4)+1; position=(NR-1)%4
  first=seed%2 ? "before" : "after"; last=seed%2 ? "after" : "before"
  want=position==0 ? first : position==1 ? "control1" : position==2 ? "control2" : last
  if ($1!=seed || $2!=want) exit 1
 }
 END { if (NR!=80) exit 1 }
' "$GTS_NATIVE_TEST_TRACE"
for name in go-before go-after c-control1 c-control2; do
 [[ $(wc -l < "$test_dir/full/$name-java.txt") == 60 ]]
done
[[ $(cat "$test_dir/full/java.done") == 'complete route=legacy seeds=20 sizes=32,137,1024' ]]
export GTS_NATIVE_TEST_BUSY=0
: > "$GTS_NATIVE_TEST_TRACE"
bash "$driver" "$test_dir/before" "$test_dir/narrow" java 1 1024 default > /dev/null
[[ $(wc -l < "$test_dir/narrow/go-after-java.txt") == 2 ]]
awk '$1!="BenchmarkFreshGeneratedDefault/java/1024KiB" { exit 1 }' "$test_dir/narrow/go-after-java.txt"
for invalid in '32,32' '137,99' ''; do
 if bash "$driver" "$test_dir/before" "$test_dir/invalid" java 1 "$invalid" > /dev/null 2>&1; then
  echo 'Invalid sizes were accepted' >&2; exit 1
 fi
 [[ ! -e "$test_dir/invalid" ]]
done
export GTS_NATIVE_TEST_DUPLICATE=1
if bash "$driver" "$test_dir/before" "$test_dir/duplicate" java 1 1024 > /dev/null 2>&1; then
 echo 'Duplicate C sample was accepted' >&2; exit 1
fi
[[ ! -e "$test_dir/duplicate/java.done" ]]
export GTS_NATIVE_TEST_DUPLICATE=0 GTS_NATIVE_TEST_MISSING=1
if bash "$driver" "$test_dir/before" "$test_dir/missing" java 1 1024 > /dev/null 2>&1; then
 echo 'Missing C sample was accepted' >&2; exit 1
fi
[[ ! -e "$test_dir/missing/java.done" ]]
echo 'fresh native comparison contracts pass'
