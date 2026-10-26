#!/usr/bin/env python3
"""Validate recorded qualification evidence without executing archived code."""
from pathlib import Path, PurePosixPath
import argparse
import gzip
import hashlib
import json
import re
import tarfile


def sha(data):
    return hashlib.sha256(data).hexdigest()


def archive(path):
    files = {}
    with tarfile.open(path) as tar:
        for member in tar:
            name = PurePosixPath(member.name)
            assert member.isfile() and not name.is_absolute() and '..' not in name.parts
            assert member.name not in files
            files[member.name] = tar.extractfile(member).read()
    return files


def differences(before, after, path=''):
    assert type(before) is type(after), path
    if isinstance(before, dict):
        assert before.keys() == after.keys(), path
        return [d for key in before for d in differences(before[key], after[key], path+'/'+key)]
    if isinstance(before, list):
        assert len(before) == len(after), path
        return [d for i, (a, b) in enumerate(zip(before, after)) for d in differences(a, b, path+'/'+str(i))]
    if before == after:
        return []
    return [{'path': path, 'before': before, 'after': after}]


def work_comparison(text, stage):
    arms = {}
    expected = {(assignment, candidate, kind, phase)
                for assignment in (False, True) for candidate in (False, True)
                for kind in ('insert', 'replace', 'delete') for phase in ('fresh', 'edit')}
    for arm in ('before', 'after'):
        rows = {}
        for assignment in (0, 1):
            raw = text(stage, f'{arm}-work-{assignment}.log')
            records = [json.loads(line[5:]) for line in raw.splitlines() if line.startswith('WORK ')]
            assert len(records) == 12
            for row in records:
                key = tuple(row[name] for name in ('assignment', 'candidate', 'kind', 'phase'))
                assert key not in rows and row['assignment'] == bool(assignment)
                work = row['work']
                assert work['contract'] == 'gts-work-count/v2' and work['overflow'] is False
                names = set(work['outside_attempt'])
                attempts = work['attempts'] or []
                for name in names:
                    assert work[name] == work['outside_attempt'][name]+sum(a['counters'][name] for a in attempts)
                for attempt in attempts:
                    for phase in ('entry_to_caps', 'caps_to_finalize', 'finalize', 'counters'):
                        assert set(attempt[phase]) == names
                    for name in names:
                        assert attempt['counters'][name] == sum(attempt[phase][name] for phase in ('entry_to_caps', 'caps_to_finalize', 'finalize'))
                assert row['selected'] is None or not any(name.endswith('Nanos') for name in row['selected'])
                rows[key] = row
        assert rows.keys() == expected
        arms[arm] = rows
    comparison = []
    for key, before in arms['before'].items():
        after = arms['after'][key]
        for name in ('assignment', 'candidate', 'kind', 'phase', 'has_error', 'end_byte'):
            assert before[name] == after[name]
        comparison.append({**dict(zip(('assignment', 'candidate', 'kind', 'phase'), key)),
                           'work_equal': before['work'] == after['work'],
                           'record_equal': before == after,
                           'differences': differences(before, after)})
    return comparison


def session_rows(raw):
    rows = [json.loads(line.split('TRUSTED_GO_SESSION_STEP ', 1)[1])
            for line in raw.splitlines() if 'TRUSTED_GO_SESSION_STEP ' in line]
    for api in ('plain', 'profiled'):
        selected = [row for row in rows if row['API'] == api]
        if not selected:
            continue
        assert len(selected) == 72 and [row['Step'] for row in selected] == list(range(72))
        current = b'package main\n\nimport "fmt"\n\n'
        function = 0
        while len(current) < 137*1024:
            current += f'func f{function}(a int, b int) int {{\n\tx := a + b\n\tfmt.Println("f{function}", x)\n\treturn x\n}}\n\n'.encode()
            function += 1
        original = current
        assert len(current) == 140368 and sha(current) == '5434575bb249338937331c3756e10dcec34237d107f523ea315996fb6de0ef8f'
        for row in selected:
            prefix = f'func f{row["Function"]}(a int, b int) int {{\n\t'.encode()
            at = current.index(prefix)+len(prefix)
            assert row['EditByte'] == at and row['BeforeBytes'] == len(current) and row['BeforeSourceSHA'] == sha(current)
            if row['Kind'] == 'grow':
                current = current[:at]+b'x'+current[at:]
            else:
                assert row['Kind'] == 'shrink' and current[at:at+2] == b'xx'
                current = current[:at]+current[at+1:]
            assert row['AfterBytes'] == len(current) and row['AfterSourceSHA'] == sha(current)
            assert row['ProducerKey'] == 8195
            assert row['VerifierEvents'] == row['WouldCertify'] == 1 and row['NotWouldCertify'] == 0
            assert row['FreshVerified'] and not row['BypassCertified'] and row['NoEditAllocs'] == 0
            assert row['SharedTopLevel'] > 0
            if api == 'profiled':
                assert row['ReusedSubtrees'] > 0 and row['ReusedBytes'] > 0
        assert current == original
    return len(rows)


def validate(directory, source_root=None):
    summary = json.loads((directory/'summary.json').read_text())
    files = archive(directory/'evidence.tar.gz')
    index = json.loads(gzip.decompress((directory/'contents.json.gz').read_bytes()))
    assert files.keys() == index.keys()
    for name, data in files.items():
        assert index[name] == {'bytes': len(data), 'sha256': sha(data)}, name
    choices = {key: [value] for key, value in files.items()}
    for relative, expected in summary['inherited_archives'].items():
        path = directory/relative
        assert sha(path.read_bytes()) == expected
        for key, value in archive(path).items():
            choices.setdefault(key, []).append(value)

    def bound(virtual, digest=None):
        candidates = choices.get(virtual.lstrip('/'), [])
        if virtual.startswith('/candidate/'):
            candidates = choices.get('candidate-inputs/'+virtual.removeprefix('/candidate/'), [])
            if source_root is not None:
                candidates += [(source_root/virtual.removeprefix('/candidate/')).read_bytes()]
        if digest is not None:
            candidates = [data for data in candidates if sha(data) == digest]
        assert candidates, (virtual, digest)
        assert all(data == candidates[0] for data in candidates), virtual
        return candidates[0]

    def text(stage, name):
        return files[f'outputs/{stage}/{name}'].decode()

    checks = 0
    for name, data in files.items():
        if name.startswith('controls/') and name.endswith('/controls.sha256'):
            for line in data.decode().splitlines():
                digest, virtual = line.split('  ', 1)
                bound(virtual, digest)
                checks += 1
    if source_root is not None:
        for line in files['controls/source.sha256'].decode().splitlines():
            digest, virtual = line.split('  ', 1)
            bound(virtual, digest)
    for stage, code in summary['terminal_codes'].items():
        assert json.loads(text(stage, 'finished.json'))['exit_code'] == code
        container = json.loads(text(stage, 'container-state.json'))
        assert container['state']['ExitCode'] == code and not container['state']['OOMKilled']
        assert container['memory'] == container['memory_swap'] == 4*1024**3
        assert container['nano_cpus'] == 10**9 and container['network'] == 'none'
        for name in ('source-before.log', 'source-after.log', 'controls-before.log', 'controls-after.log'):
            assert text(stage, name) == '', (stage, name)
    assert summary['terminal_codes']['v2'] == 1
    assert 'small=0 would=0 want small=true' in text('v2', 'admission-boundaries.log')

    passed = {}
    for stage, logs in summary['passing_logs'].items():
        for name in logs:
            raw = text(stage, name)
            assert not re.search(r'--- (FAIL|SKIP):|^FAIL\b|panic:|WARNING: DATA RACE', raw, re.M), (stage, name)
            assert re.search(r'^PASS$', raw, re.M), (stage, name)
            passed[stage+'/'+name] = len(re.findall(r'^\s*--- PASS:', raw, re.M))
    for stage in ('v4', 'v5', 'v6'):
        for language in ('go', 'yaml'):
            for route in ('default', 'candidate'):
                assert text(stage, f'baseline-{language}-{route}.json') == text(stage, f'candidate-{language}-{route}.json')
        assert session_rows(text(stage, 'clean-plain.log')+text(stage, 'clean-profiled.log')) == 144
        for route in (0, 1):
            assert 'session_steps=72 no_edit_allocs=0' in text(stage, f'invariant-{route}.log')
        assert passed[stage+'/locked-c.log'] == 127
    comparisons = {stage: work_comparison(text, stage) for stage in ('v4-extended', 'v5', 'v6')}
    assert comparisons == summary['complete_operation_comparisons']
    for stage in summary['oracle_stages']:
        for stem in ('oracle', 'binding', 'extended'):
            for when in ('before', 'after'):
                assert text(stage, f'{stem}-{when}.log') == ''
        assert '10eb5ac119eec9cb4267a8e035066473983f5f3a' in text(stage, 'runtime-probe.log')
        assert passed[stage+'/locked-c027.log'] == 127
        digests = lambda raw: re.findall(r'deep digest ([0-9a-f]{64})', raw)
        source_stage = summary['oracle_stages'][stage]
        runtime = json.loads(files[f'controls/{source_stage}/overlay.runtime.json'])['Replace']
        composed = json.loads(files[f'controls/{stage}/overlay.c027.json'])['Replace']
        assert len(composed) == len(runtime)+16
        assert all(composed.get(target) == backing for target, backing in runtime.items())
        for backing in composed.values():
            bound(backing)
        assert digests(text(stage, 'locked-c027.log')) == digests(text(source_stage, 'locked-c.log'))
        for when in ('before', 'after'):
            staged = json.loads(text(stage, f'staged-{when}.json'))
            assert staged['cgo_files'] == 14 and staged['bodies_byte_identical_to_frozen_adaptation']
            assert not staged['module_cache_changed'] and not staged['project_dependencies_changed']
    for stage in summary['race_stages']:
        assert session_rows(text(stage, 'race-go.log')) == 144
    for stage, log in summary['real_source_logs'].items():
        raw = text(stage, log)
        records = re.findall(r'REAL_ADMISSION_SUMMARY source=(\S+) bytes=(\d+) cases=(\d+) would_certify=(\d+)', raw)
        assert len(records) == 4 and sum(int(row[2]) for row in records) == 108
        assert sum(int(row[3]) for row in records) == 62
    for stage in ('v4', 'v5', 'v6'):
        mapping = json.loads(files[f'controls/{stage}/overlay.runtime.json'])['Replace']
        for backing in mapping.values():
            bound(backing)
    assert summary['qualified_shadow_stage'] == 'v5'
    adverse = [change for row in comparisons['v5'] for change in row['differences']
               if change['path'].startswith('/work/') and change['path'].count('/') == 2
               and type(change['before']) is int and change['after'] > change['before']]
    assert adverse == summary['complete_work_adverse_directions']
    assert len(adverse) == 4 and all(change == {'path': '/work/table_lookups_proxy', 'before': 345393, 'after': 345408} for change in adverse)
    assert not summary['counter_direction_passed'] and not summary['owner_exception_used']
    assert summary['fresh_verifier_active'] and summary['main_merges_held']
    assert not summary['timing_ran'] and not summary['runtime_promoted'] and not summary['graduated']
    return {'validated': True, 'members': len(files), 'control_checks': checks,
            'passing_records': passed,
            'complete_work_identical': {stage: sum(row['work_equal'] for row in rows) for stage, rows in comparisons.items()},
            'fresh_verifier_active': True, 'main_merges_held': True}


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--source-root', type=Path)
    args = parser.parse_args()
    print(json.dumps(validate(Path(__file__).resolve().parent, args.source_root), indent=2))
