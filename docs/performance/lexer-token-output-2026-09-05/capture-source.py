from pathlib import Path
import datetime, hashlib, json, subprocess, sys
root = Path(__file__).resolve().parent
phase = sys.argv[1]
assert phase in ('before', 'after')
def git(path, *args):
    return subprocess.check_output(['git', '-C', str(path), *args])
def sha(data):
    return hashlib.sha256(data).hexdigest()
paths = {
    'baseline': Path('/home/draco/work/gts-token-invariant-perf-candidate-20260905'),
    'candidate': Path('/home/draco/work/gts-lexer-token-output-20260905'),
}
record = {'created': datetime.datetime.now(datetime.timezone.utc).isoformat(),
          'comparison': 'da1150c6 vs the same commit plus the recorded three-file patch', 'sources': {}}
for role, path in paths.items():
    files = git(path, 'ls-files', '-z').decode().split('\0')[:-1]
    inventory = [{'path': file, 'sha256': sha((path / file).read_bytes())} for file in files]
    payload = (json.dumps(inventory, indent=2) + '\n').encode()
    manifest = root / (role + '-source-files.json')
    if phase == 'before':
        manifest.write_bytes(payload)
    else:
        assert payload == manifest.read_bytes(), role + ' source files changed'
    record['sources'][role] = {
        'path': str(path), 'head': git(path, 'rev-parse', 'HEAD').decode().strip(),
        'status': git(path, 'status', '--porcelain=v1').decode(),
        'tracked_files': len(files), 'source_manifest_sha256': sha(payload),
    }
patch = git(paths['candidate'], 'diff', '--binary')
assert patch == (root / 'candidate.patch').read_bytes()
record['patch_sha256'] = sha(patch)
record['changed_files'] = git(paths['candidate'], 'diff', '--name-only').decode().splitlines()
assert record['changed_files'] == ['lexer.go', 'lexer_token_output_test.go', 'token_invariant_lexical_proof.go']
assert record['sources']['baseline']['status'] == ''
assert all(value['head'] == 'da1150c6d6a2d581ce31f44ba4c5b8241ec431ae' for value in record['sources'].values())
if phase == 'after':
    previous = json.loads((root / 'source-before.json').read_text())
    assert record['sources'] == previous['sources']
    assert record['patch_sha256'] == previous['patch_sha256']
(root / ('source-' + phase + '.json')).write_text(json.dumps(record, indent=2) + '\n')
print(json.dumps(record, indent=2))
