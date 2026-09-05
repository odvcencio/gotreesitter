#!/usr/bin/env bash
set -euo pipefail
export GOMAXPROCS=1 GOT_GLR_MAX_MERGE_PER_KEY=1
cd /workspace
/usr/bin/time -v -o /evidence/cap1-rss.txt go test -overlay /evidence/reduction-overlay.json ./grammars -run '^TestCDeleteFallbackReductionDiagnostic$' -v -count=1 -parallel=1 -timeout=3m > /evidence/cap1.txt 2>&1
