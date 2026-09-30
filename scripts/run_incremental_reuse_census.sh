#!/usr/bin/env bash
# Run one language at a time. Keep the authenticated corpus lock outside the repo.
set -euo pipefail
export GOWORK=off
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
source_root=${1:?usage: run_incremental_reuse_census.sh CORPUS_ROOT OUTPUT_DIR [LANGUAGE ...]}
output_root=${2:?usage: run_incremental_reuse_census.sh CORPUS_ROOT OUTPUT_DIR [LANGUAGE ...]}
shift 2
source_root=$(realpath "$source_root")
mkdir -p "$output_root"
output_root=$(realpath "$output_root")
languages=("$@")
if ((${#languages[@]} == 0)); then languages=(go javascript typescript python rust java c_sharp powershell); fi
lock_file=$(mktemp /tmp/gts-incremental-census-lock.XXXXXX)
trap 'rm -f "$lock_file"' EXIT
bash "$root/scripts/fetch_grammar_receipt_lock.sh" "${GTS_CORPUS_LOCK_URL:-}" "$lock_file"
python3 - "$root/docs/receipts/incremental-reuse-census/fixtures.json" "$lock_file" "$source_root" "${languages[@]}" <<'PY'
import hashlib,json,pathlib,subprocess,sys
manifest=json.load(open(sys.argv[1]))
lock=pathlib.Path(sys.argv[2]).read_bytes()
assert hashlib.sha256(lock).hexdigest()==manifest['lock_sha256'], 'corpus lock differs'
pins={p[0]:p for l in lock.decode().splitlines() if (p:=l.split()) and not p[0].startswith('#')}
source=pathlib.Path(sys.argv[3]);selected=set(sys.argv[4:])
for fixture in manifest['files']:
    name=fixture['language']
    if name not in selected: continue
    pin=pins[name]
    assert fixture['commit']==pin[2] and fixture['repo']==pin[1], 'fixture source identity differs'
    repo=source/name
    head=subprocess.check_output(['git','-C',str(repo),'rev-parse','HEAD'],text=True).strip()
    assert head==pin[2], f'{name}: checkout differs from lock'
    data=(repo/fixture['path']).read_bytes()
    assert len(data)==fixture['bytes'] and hashlib.sha256(data).hexdigest()==fixture['sha256'], f'{name}: file differs'
PY
for language in "${languages[@]}"; do
    case "$language" in go|javascript|typescript|python|rust|java|c_sharp|powershell) ;; *) echo "unsupported census language: $language" >&2; exit 2 ;; esac
    bash "$root/cgo_harness/docker/run_parity_in_docker.sh" --no-build \
        --label "incr-census-$language" --cpus 1 --cpuset-cpus "${GTS_INCR_CENSUS_CPU:-0}" \
        --mount "$source_root:/corpus:ro" --mount "$output_root:/evidence" -- \
        "cd /workspace/cgo_harness && GOWORK=off GOMAXPROCS=1 GTS_INCR_CENSUS_LANG=$language GTS_INCR_CENSUS_ROOT=/corpus GTS_INCR_CENSUS_OUT=/evidence/$language.jsonl go test -c -tags 'treesitter_c_parity gts_incr_census' -o /evidence/census.test && GOWORK=off GOMAXPROCS=1 GTS_INCR_CENSUS_LANG=$language GTS_INCR_CENSUS_ROOT=/corpus GTS_INCR_CENSUS_OUT=/evidence/$language.jsonl /usr/bin/time -v /evidence/census.test -test.v -test.run '^TestIncrementalReuseCensus$' -test.count=1 -test.timeout=30m" \
        >"$output_root/$language.log" 2>&1
done
python3 "$root/scripts/reduce_incremental_reuse_census.py" "$output_root" --output "$output_root/summary.json"
