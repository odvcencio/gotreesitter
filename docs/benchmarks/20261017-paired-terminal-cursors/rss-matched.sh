#!/usr/bin/env bash
set -euo pipefail
export GOMAXPROCS=1
python3 /evidence/repeat_rss.py
