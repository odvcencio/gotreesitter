#!/usr/bin/env python3
"""Rebuild the harness-local C oracle from the commits in its contract."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
DEST = ROOT / 'internal/coracle'
PATCH = ROOT / 'oracle/binding_compat.patch'


def sha(data):
    return hashlib.sha256(data).hexdigest()


def git(repo, *args):
    return subprocess.check_output(['git', '-C', str(repo), *args])


def pins():
    source = (ROOT / 'parity_c_loader_cgo.go').read_text()
    return {key: re.search(r'\bCOracle' + key + r'\s*=\s*"([^"]+)"', source).group(1)
            for key in ('BindingCommit', 'BindingVersion', 'RuntimeCommit', 'RuntimeVersion')}


def files(root):
    return {p.relative_to(root).as_posix(): sha(p.read_bytes())
            for p in sorted(root.rglob('*')) if p.is_file() and p.name != 'upstream.json'}


def verify():
    manifest = json.loads((DEST / 'upstream.json').read_text())
    if manifest['pins'] != pins():
        raise SystemExit('vendored oracle provenance differs from the contract pins')
    if manifest['binding_patch_sha256'] != sha(PATCH.read_bytes()):
        raise SystemExit('binding compatibility patch differs from the recorded patch')
    if manifest['files'] != files(DEST):
        raise SystemExit('vendored oracle bytes differ from the recorded sources')
    print('C oracle source provenance verified')


def refresh():
    contract = pins()
    with tempfile.TemporaryDirectory(prefix='gts-oracle-sources-') as tmp:
        tmp = Path(tmp)
        repos = {}
        for kind, url, commit in (
            ('binding', 'https://github.com/tree-sitter/go-tree-sitter.git', contract['BindingCommit']),
            ('runtime', 'https://github.com/tree-sitter/tree-sitter.git', contract['RuntimeCommit']),
        ):
            repo = tmp / kind
            repo.mkdir()
            subprocess.run(['git', 'init', '-q', str(repo)], check=True)
            subprocess.run(['git', '-C', str(repo), 'fetch', '--quiet', '--depth=1', url, commit], check=True)
            subprocess.run(['git', '-C', str(repo), 'checkout', '--quiet', '--detach', 'FETCH_HEAD'], check=True)
            if git(repo, 'rev-parse', 'HEAD').decode().strip() != commit:
                raise SystemExit(kind + ' commit mismatch')
            repos[kind] = repo
        stage = tmp / 'oracle'
        stage.mkdir()
        for source in repos['binding'].iterdir():
            if (source.suffix == '.go' and not source.name.endswith('_test.go')) or source.name in ('allocator.c', 'allocator.h', 'LICENSE'):
                shutil.copyfile(source, stage / source.name)
        for part in ('src', 'include'):
            shutil.copytree(repos['runtime'] / 'lib' / part, stage / part)
        shutil.copyfile(ROOT / 'oracle/provenance.go.txt', stage / 'provenance.go')
        # The module supplies production bindings only. Grammar dependencies
        # in the upstream test module cannot change our locked grammar inputs.
        (stage / 'go.mod').write_text('module github.com/tree-sitter/go-tree-sitter\n\ngo 1.23\n\nrequire github.com/mattn/go-pointer v0.0.1\n')
        subprocess.run(['git', 'apply', '--unsafe-paths', '--directory=' + str(stage), str(PATCH)], check=True, cwd=tmp)
        manifest = {
            'schema': 'gts-c-oracle-sources/v1', 'pins': contract,
            'binding_repository': 'https://github.com/tree-sitter/go-tree-sitter.git',
            'runtime_repository': 'https://github.com/tree-sitter/tree-sitter.git',
            'runtime_src_tree_oid': git(repos['runtime'], 'rev-parse', 'HEAD:lib/src').decode().strip(),
            'runtime_include_tree_oid': git(repos['runtime'], 'rev-parse', 'HEAD:lib/include').decode().strip(),
            'runtime_license_sha256': sha((repos['runtime'] / 'LICENSE').read_bytes()),
            'binding_patch_sha256': sha(PATCH.read_bytes()), 'files': files(stage),
        }
        (stage / 'upstream.json').write_text(json.dumps(manifest, indent=2, sort_keys=True) + '\n')
        if DEST.exists():
            shutil.rmtree(DEST)
        shutil.copytree(stage, DEST)
    verify()


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--verify', action='store_true', help='check local bytes without network access')
    args = parser.parse_args()
    verify() if args.verify else refresh()
