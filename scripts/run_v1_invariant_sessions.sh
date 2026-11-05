#!/usr/bin/env bash
# Run from a Docker container with the repository mounted read-only. Each
# language/route gets its own process; keep partial results on the first failure.
set -euo pipefail
if [[ ! -f /.dockerenv ]]; then
  echo 'run_v1_invariant_sessions.sh requires Docker isolation' >&2
  exit 2
fi
if [[ $# != 1 ]]; then
  echo 'usage: run_v1_invariant_sessions.sh <empty-output-directory>' >&2
  exit 2
fi
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mkdir -p "$1"
evidence="$(cd "$1" && pwd)"
case "$evidence/" in "$repo_root/"*) echo 'output must be outside the read-only source tree' >&2; exit 2 ;; esac
python3 - "$evidence" <<'PYCODE'
from pathlib import Path
import sys
if any(Path(sys.argv[1]).iterdir()):
    raise SystemExit('output directory is not empty; preserve the prior run and choose a new directory')
PYCODE
export PATH=/usr/local/go/bin:$PATH GOMAXPROCS=1 GOWORK=off
export GOFLAGS='-p=1 -mod=readonly -buildvcs=false'
cd "$repo_root"
go version > "$evidence/go-version.txt"
go env -json GOOS GOARCH CGO_ENABLED GOVERSION > "$evidence/go-env.json"
python3 - "$evidence/source-before.json" <<'PYCODE'
from pathlib import Path
import hashlib, json, os, sys
files = {str(p): hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(Path('.').rglob('*.go')) if p.is_file()}
Path(sys.argv[1]).write_text(json.dumps({'source_sha': os.environ.get('GTS_V1_SOURCE_SHA'), 'go_files': files}, sort_keys=True) + '\n')
PYCODE
go test -c -tags gts_parsercorephase0 -o "$evidence/root.test" . > "$evidence/root-build.log" 2>&1
go build -tags gts_parsercorephase0 -o "$evidence/ledger" ./cmd/perfcounterledger > "$evidence/ledger-build.log" 2>&1
"$evidence/ledger" --write --output "$evidence/counters.json" > "$evidence/counter-build.log" 2>&1
python3 - "$evidence" <<'PYCODE'
from pathlib import Path
import json, re, sys
out = Path(sys.argv[1])
rows = json.loads((out / 'counters.json').read_text())['rows']
languages = sorted({row['language'] for row in rows})
reference = json.loads(Path('internal/benchfixtures/counter_ledger.json').read_text())['rows']
expected_languages = {row['language'] for row in reference}
assert set(languages) == expected_languages, 'generated ledger must cover every reviewed language'
assert languages and len(rows) == 2 * len(languages)
for language in languages:
    assert re.fullmatch(r'[a-zA-Z0-9_-]+', language), language
    assert sorted(row['route'] for row in rows if row['language'] == language) == ['candidate', 'default'], language
(out / 'languages.txt').write_text('\n'.join(languages) + '\n')
PYCODE
failed=0
for route in 0 1; do
  while IFS= read -r language; do
    log="invariant-${route}-${language}.log"
    if GTS_ADMISSION_CANDIDATE="$route" GOTREESITTER_V1_INVARIANT_LANGUAGE="$language" \
      "$evidence/root.test" -test.run '^TestV1InvariantGateR4EditSession$' \
      -test.count=1 -test.v -test.timeout=5m > "$evidence/$log" 2>&1; then
      status=0
    else
      status=$?
    fi
    printf '%s\t%s\t%s\n' "$route" "$language" "$status" >> "$evidence/status.tsv"
    if [[ "$status" != 0 ]]; then
      failed="$status"
      break 2
    fi
  done < "$evidence/languages.txt"
done
python3 - "$evidence" <<'PYCODE'
from pathlib import Path
import hashlib, json, re, sys
out = Path(sys.argv[1])
before = json.loads((out / 'source-before.json').read_text())
after = {str(p): hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(Path('.').rglob('*.go')) if p.is_file()}
languages = (out / 'languages.txt').read_text().splitlines()
expected = {(str(route), language) for route in [0, 1] for language in languages}
seen, passed, rejected = set(), [], []
for row in (out / 'status.tsv').read_text().splitlines():
    route, language, code = row.split('\t')
    key = (route, language)
    assert key in expected and key not in seen, key
    seen.add(key)
    text = (out / ('invariant-%s-%s.log' % key)).read_text()
    receipt = re.search(r'language=' + re.escape(language) + r' fixture=\S+ sample_bytes=\d+ session_steps=(\d+) no_edit_allocs=(\S+)', text)
    valid = code == '0' and re.search(r'^PASS$', text, re.M) and '--- FAIL:' not in text and '--- SKIP:' not in text and receipt is not None and receipt[1] == '72' and receipt[2] == '0'
    (passed if valid else rejected).append({'route': int(route), 'language': language, 'exit_code': int(code)})
complete = seen == expected and not rejected and before['go_files'] == after
summary = {'passed': complete, 'source_sha': before['source_sha'], 'languages': len(languages), 'required_sessions': len(expected), 'passed_sessions': len(passed), 'completed_edit_steps': 72 * len(passed), 'no_edit_allocations_in_passed_sessions': 0, 'source_unchanged': before['go_files'] == after, 'rejected': rejected, 'missing': [list(x) for x in sorted(expected - seen)], 'binary_sha256': hashlib.sha256((out / 'root.test').read_bytes()).hexdigest(), 'counter_sha256': hashlib.sha256((out / 'counters.json').read_bytes()).hexdigest(), 'timing_claim': False}
(out / 'summary.json').write_text(json.dumps(summary, indent=2) + '\n')
print(json.dumps({k: v for k, v in summary.items() if k not in ['rejected', 'missing']}, sort_keys=True))
if not complete:
    raise SystemExit('v1 edit-session gate incomplete or failed; inspect summary.json and per-language logs')
PYCODE
exit "$failed"
