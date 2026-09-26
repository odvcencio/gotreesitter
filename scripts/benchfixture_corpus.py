#!/usr/bin/env python3
"""Pin a small source sample and four size-selected files for each grammar."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[1]
OUTPUT = ROOT / 'internal/benchfixtures'
MANIFEST = OUTPUT / 'real_corpus.json'
SMALL = OUTPUT / 'testdata/real'
SKIP = {'.git', 'node_modules', 'vendor', 'build', 'dist', 'target', 'generated'}
LICENSE_NAMES = ('LICENSE', 'LICENCE', 'COPYING', 'NOTICE')
MANIFEST_ONLY = {'csv', 'enforce'}


def digest(data):
    return hashlib.sha256(data).hexdigest()


def lock_entries(path):
    for line in path.read_text().splitlines():
        words = line.split()
        if len(words) >= 3 and not words[0].startswith('#'):
            yield words[0], words[1], words[2], words[3] if len(words) > 3 else '.', tuple(words[4].split(',')) if len(words) > 4 else ()


def repo_files(repo, extensions, relaxed=False):
    raw = subprocess.check_output(['git', '-C', str(repo), 'ls-files', '-z'])
    suffixes = tuple((Path(ext).suffix if '/' in ext else ext).lower() for ext in extensions)
    for name in raw.decode('utf-8', 'surrogateescape').split('\0'):
        if not name:
            continue
        path = Path(name)
        if any(part in SKIP for part in path.parts):
            continue
        if not relaxed and any(part in ('test', 'tests', 'fixtures', 'corpus') for part in path.parts):
            continue
        if extensions and not name.lower().endswith(suffixes):
            continue
        if not extensions and path.suffix.lower() in ('.md', '.txt', '.png', '.jpg', '.svg', '.lock'):
            continue
        if path.name.upper().startswith(LICENSE_NAMES):
            continue
        source = repo / path
        if not source.is_file():
            continue
        size = source.stat().st_size
        if 32 <= size <= 512 * 1024:
            yield size, name


def license_for(repo):
    matches = sorted(p for p in repo.iterdir() if p.is_file() and (
        any(word in p.name.upper() for word in LICENSE_NAMES) or
        p.name.upper() in ('COPYRIGHT', 'EPL-V10.HTML')))
    if not matches:
        package = repo / 'package.json'
        if package.is_file():
            metadata = json.loads(package.read_text())
            if isinstance(metadata.get('license'), str):
                return {'path': 'package.json', 'sha256': digest(package.read_bytes()),
                        'name': metadata['license']}
        readme = repo / 'README.md'
        if readme.is_file() and 'DayZ Public License (DPL)' in readme.read_text(errors='replace'):
            return {'path': 'README.md', 'sha256': digest(readme.read_bytes()),
                    'name': 'DayZ Public License'}
        return {'path': '', 'sha256': '', 'name': 'unidentified'}
    p = matches[0]
    data = p.read_bytes()
    lead = data[:1024].decode('utf-8', 'replace').lower()
    names = [('Apache-2.0', 'apache license'), ('MIT', 'mit license'), ('BSD', 'bsd license'),
             ('GPL', 'gnu general public license'), ('MPL-2.0', 'mozilla public license'),
             ('ISC', 'isc license'), ('Unlicense', 'unlicense')]
    license_name = next((name for name, needle in names if needle in lead), 'see license file')
    return {'path': p.name, 'sha256': digest(data), 'name': license_name}


def select(files):
    files.sort(key=lambda item: (item[0], item[1]))
    if not files:
        return []
    largest = files[-1]
    middle = (len(files) - 1) // 2
    positions = sorted(range(len(files) - 1), key=lambda i: (abs(i - middle), i))[:3]
    return [files[i] for i in sorted(positions)] + [largest]


def generate(args):
    rows = []
    missing = []
    total = 0
    SMALL.mkdir(parents=True, exist_ok=True)
    entries = list(lock_entries(args.lock))
    for index, (language, url, commit, subdir, exts) in enumerate(entries, 1):
        repo = args.source_root / language
        if not repo.is_dir():
            missing.append(f'{language}: no checkout')
            continue
        head = subprocess.check_output(['git', '-C', str(repo), 'rev-parse', 'HEAD'], text=True).strip()
        if head != commit:
            missing.append(f'{language}: checkout {head} differs from {commit}')
            continue
        files = list(repo_files(repo, exts))
        if not files:
            files = list(repo_files(repo, exts, relaxed=True))
        if not files:
            files = list(repo_files(repo, (), relaxed=True))
        if not files:
            missing.append(f'{language}: no source files')
            continue
        while files:
            choices = select(files)
            sample = next((item for item in files if item[0] <= 16 * 1024), min(files))
            bad = {name for _, name in choices + [sample] if b'\0' in (repo / name).read_bytes()}
            if not bad:
                break
            files = [item for item in files if item[1] not in bad]
        if not files:
            missing.append(f'{language}: no text source files')
            continue
        license_info = license_for(repo)
        for role, (size, name) in [('sample', sample), *[(f'median_{i}', f) for i, f in enumerate(choices[:-1], 1)], ('largest', max(files))]:
            data = (repo / name).read_bytes()
            if b'\0' in data or digest(data) == digest(b''):
                continue
            committed = role == 'sample' and language not in MANIFEST_ONLY
            if committed:
                (SMALL / language).write_bytes(data)
                total += len(data)
            elif role == 'sample':
                (SMALL / language).unlink(missing_ok=True)
            rows.append({'language': language, 'role': role, 'bytes': len(data), 'sha256': digest(data),
                         'repo': url, 'commit': commit, 'path': name,
                         'license': license_info, 'committed_path': f'testdata/real/{language}' if committed else ''})
        if index % 25 == 0:
            print(f'{index}/{len(entries)} languages', file=sys.stderr)
    if missing:
        print('\n'.join(missing), file=sys.stderr)
        raise SystemExit(f'{len(missing)} languages have no pinned corpus')
    MANIFEST.write_text(json.dumps({'schema': 'benchfixture-real-corpus-v1', 'languages': len(entries),
                                    'selection': 'three median files and the largest within 512 KiB; one small committed sample',
                                    'entries': rows}, indent=2) + '\n')
    print(f'{len(entries)} languages, {len(rows)} entries, {total} committed bytes')


def verify(args):
    manifest = json.loads(MANIFEST.read_text())
    errors = []
    checked = 0
    for row in manifest['entries']:
        if args.role != 'all' and row['role'] != args.role:
            continue
        if args.language and row['language'] != args.language:
            continue
        if row['committed_path']:
            path = OUTPUT / row['committed_path']
        elif args.source_root:
            path = args.source_root / row['language'] / row['path']
        else:
            continue
        checked += 1
        if not path.is_file() or digest(path.read_bytes()) != row['sha256']:
            errors.append(str(path))
    if errors:
        raise SystemExit('fixture mismatch:\n' + '\n'.join(errors[:30]))
    print(f'verified {checked} files')


def fetch(args):
    manifest = json.loads(MANIFEST.read_text())
    selected = [row for row in manifest['entries'] if not args.language or row['language'] == args.language]
    if not selected:
        raise SystemExit('no entries for the selected language')
    if not args.output:
        raise SystemExit('fetch requires --output')
    args.output.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='benchfixture-corpus-') as temp:
        for language in sorted({row['language'] for row in selected}):
            rows = [row for row in selected if row['language'] == language]
            repo = args.source_root / language if args.source_root else Path(temp) / language
            if not args.source_root:
                subprocess.run(['git', 'clone', '--filter=blob:none', '--no-checkout', rows[0]['repo'], str(repo)], check=True)
            head = subprocess.check_output(['git', '-C', str(repo), 'rev-parse', 'HEAD'], text=True).strip()
            if args.source_root and head != rows[0]['commit']:
                raise SystemExit(f'{language}: checkout {head} differs from {rows[0]["commit"]}')
            license_info = rows[0]['license']
            if license_info['path']:
                license_data = subprocess.check_output(['git', '-C', str(repo), 'show',
                    f'{rows[0]["commit"]}:{license_info["path"]}'])
                if digest(license_data) != license_info['sha256']:
                    raise SystemExit(f'{language}: license digest mismatch')
            for row in rows:
                source_path = Path(row['path'])
                if source_path.is_absolute() or '..' in source_path.parts:
                    raise SystemExit(f'{language}: unsafe source path')
                data = subprocess.check_output(['git', '-C', str(repo), 'show',
                    f'{row["commit"]}:{row["path"]}'])
                if len(data) != row['bytes'] or digest(data) != row['sha256']:
                    raise SystemExit(f'{language}/{row["role"]}: source digest mismatch')
                target = args.output / language / row['role']
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_bytes(data)
    print(f'fetched and verified {len(selected)} files')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=['generate', 'verify', 'fetch'])
    parser.add_argument('--source-root', type=Path)
    parser.add_argument('--lock', type=Path)
    parser.add_argument('--language')
    parser.add_argument('--output', type=Path)
    parser.add_argument('--role', choices=['sample', 'all'], default='all')
    args = parser.parse_args()
    if args.command == 'generate':
        if not args.source_root or not args.lock:
            parser.error('generate requires --source-root and --lock')
        generate(args)
    elif args.command == 'verify':
        verify(args)
    else:
        fetch(args)


if __name__ == '__main__':
    main()
