#!/usr/bin/env bash
set -euo pipefail
export GOWORK=off GOMAXPROCS=1
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
exec python3 "$root/scripts/editor_latency_gate.py" run "$@"
