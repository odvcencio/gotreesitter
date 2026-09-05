#!/usr/bin/env bash
set -euo pipefail
cd /home/draco/work/gts-arena-reuse-integration-20260905
bash cgo_harness/docker/run_parity_in_docker.sh --no-build --label "arena-integration-$1" --out-root /tmp/gts-arena-reuse-integration-20260905 --mount /tmp/gts-arena-reuse-integration-20260905:/evidence --memory 4g --gomemlimit 3GiB --cpus 1 --cpuset-cpus 2 --pids 512 --wall-timeout 10m -- "bash /evidence/oracle.sh $1"
