#!/usr/bin/env bash
set -euo pipefail

# Multiple runner services may share a VM, HOME, and /tmp. setup-go restores
# archives directly into Go's cache paths, which can overwrite mapped imports
# in another job. The runner clears RUNNER_TEMP between jobs.
: "${RUNNER_TEMP:?RUNNER_TEMP is required}"
: "${GITHUB_ENV:?GITHUB_ENV is required}"
mkdir -p "$RUNNER_TEMP/go-build" "$RUNNER_TEMP/go"
{
  printf 'TMPDIR=%s\n' "$RUNNER_TEMP"
  printf 'GOTMPDIR=%s\n' "$RUNNER_TEMP"
  printf 'GOCACHE=%s/go-build\n' "$RUNNER_TEMP"
  printf 'GOPATH=%s/go\n' "$RUNNER_TEMP"
} >> "$GITHUB_ENV"
