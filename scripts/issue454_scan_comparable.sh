#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
bench_bin="$(mktemp "${TMPDIR:-/tmp}/issue454bench.XXXXXX")"
trap 'rm -f "$bench_bin"' EXIT

cd "$repo_root"
GOWORK=off go build -o "$bench_bin" ./cmd/issue454bench

# C# is the reported outlier. Scala uses the exact published generator.
# The other 47 comparable shapes are deterministic reconstructions.
languages=(
  rust scala_report cpp less diff python tsx elixir sql go ruby perl ini sh ps1
  kotlin xml typescript vue java graphql make r php zig svelte scss javascript
  objc hcl json css toml nix yaml swift rst haskell lua proto cmake dockerfile
  md groovy templ csv html c
)

for language in "${languages[@]}"; do
  for size_kb in 32 64; do
    GOMAXPROCS=1 GOWORK=off GOGC=off "$bench_bin" "$language" "$size_kb" full 1
  done
done
