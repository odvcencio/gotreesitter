#!/usr/bin/env bash
set -euo pipefail
stage=$1
cd /home/draco/work/gts-lexer-token-output-20260905
bash cgo_harness/docker/run_parity_in_docker.sh --no-build \
 --label "lexer-token-output-$stage" \
 --out-root /tmp/gts-lexer-token-output-20260905 \
 --mount /tmp/gts-lexer-token-output-20260905:/evidence \
 --mount /home/draco/work/gts-token-invariant-perf-candidate-20260905:/baseline:ro \
 --mount /home/draco/work/gotreesitter/.git:/home/draco/work/gotreesitter/.git:ro \
 --mount /home/draco/work/gts-lexer-token-output-20260905:/home/draco/work/gts-lexer-token-output-20260905:ro \
 --mount /home/draco/work/gts-token-invariant-perf-candidate-20260905:/home/draco/work/gts-token-invariant-perf-candidate-20260905:ro \
 --mount /tmp/gotreesitter-run-randomized-benchmarks.lock:/bench.lock \
 --memory 4g --gomemlimit 3GiB --cpus 1 --cpuset-cpus 2 --pids 512 \
 --wall-timeout 15m -- "bash /evidence/run-$stage.sh"
