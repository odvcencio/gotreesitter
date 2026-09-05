#!/usr/bin/env bash
set -euo pipefail
cd /home/draco/work/gts-c-fallback-attribution-20260905
bash cgo_harness/docker/run_parity_in_docker.sh --no-build --label c-fallback-attribution --out-root /tmp/gts-c-fallback-attribution-20260905 --mount /tmp/gts-c-fallback-attribution-20260905:/evidence --memory 4g --gomemlimit 3GiB --cpus 1 --cpuset-cpus 2 --pids 512 --wall-timeout 8m -- 'bash /evidence/run.sh'
