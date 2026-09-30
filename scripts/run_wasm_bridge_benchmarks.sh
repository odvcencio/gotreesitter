#!/usr/bin/env bash
set -euo pipefail
# Immutable binaries let the standard shuffled runner compare the actual JS
# bridge without making a Go test package part of either parser revision.
if [[ $# != 6 ]]; then
  echo 'Usage: run_wasm_bridge_benchmarks.sh before.wasm after.wasm wasm_exec.js go.bin before.txt after.txt' >&2
  exit 2
fi
script_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
before_artifact=$(realpath "$1")
after_artifact=$(realpath "$2")
export GTS_WASM_BENCH_BOOTSTRAP=$(realpath "$3")
export GTS_WASM_BENCH_BLOB=$(realpath "$4")
before_output=$(realpath -m "$5")
after_output=$(realpath -m "$6")
export GOWORK=off GOMAXPROCS=1
probe_root=$(mktemp -d)
trap 'rm -r -- "$probe_root"' EXIT
for phase in before after; do
  mkdir "$probe_root/$phase"
  printf 'module wasmprobe\n\ngo 1.25.0\n' > "$probe_root/$phase/go.mod"
  cp "$script_root/wasm_bridge_benchmark_test.go" "$probe_root/$phase/bridge_test.go"
done
cp "$script_root/measure_wasm_runtime.cjs" "$probe_root/probe.cjs"
export GTS_WASM_BENCH_SCRIPT="$probe_root/probe.cjs"
ln -s "$before_artifact" "$probe_root/before/runtime.wasm"
ln -s "$after_artifact" "$probe_root/after/runtime.wasm"
cd "$probe_root/after"
bash "$script_root/run_randomized_benchmarks.sh" \
  --baseline-root "$probe_root/before" --baseline-output "$before_output" \
  --output "$after_output" --bench-regex '^BenchmarkWASMBridge64KiB$'
