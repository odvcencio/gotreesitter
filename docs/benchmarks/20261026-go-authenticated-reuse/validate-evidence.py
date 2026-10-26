#!/usr/bin/env python3
"""Validate archived qualification records without executing archived code."""
from pathlib import Path, PurePosixPath
import argparse
import gzip
import hashlib
import json
import re
import statistics
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


def work_comparison(text):
    arms = {}
    expected = {(a, c, k, p) for a in (False, True) for c in (False, True)
                for k in ('insert', 'replace', 'delete') for p in ('fresh', 'edit')}
    for arm in ('before', 'after'):
        rows = {}
        for assignment in (0, 1):
            records = [json.loads(s[5:]) for s in text('v1', f'{arm}-work-{assignment}.log').splitlines() if s.startswith('WORK ')]
            assert len(records) == 12
            for row in records:
                key = tuple(row[k] for k in ('assignment', 'candidate', 'kind', 'phase'))
                assert key not in rows
                work = row['work']
                assert work['contract'] == 'gts-work-count/v2' and work['overflow'] is False
                names, attempts = set(work['outside_attempt']), work['attempts'] or []
                for name in names:
                    assert work[name] == work['outside_attempt'][name]+sum(a['counters'][name] for a in attempts)
                for attempt in attempts:
                    for name in names:
                        assert attempt['counters'][name] == sum(attempt[p][name] for p in ('entry_to_caps', 'caps_to_finalize', 'finalize'))
                rows[key] = row
        assert rows.keys() == expected
        arms[arm] = rows
    result = []
    for key, before in arms['before'].items():
        after = arms['after'][key]
        for name in ('assignment', 'candidate', 'kind', 'phase', 'has_error', 'end_byte'):
            assert before[name] == after[name]
        changes = {name: {'before': before['work'][name], 'after': after['work'][name]}
                   for name in before['work']['outside_attempt'] if before['work'][name] != after['work'][name]}
        assert all(row['after'] <= row['before'] for row in changes.values())
        selected = {name: {'before': value, 'after': after['selected'][name]}
                    for name, value in (before['selected'] or {}).items() if value != after['selected'][name]}
        assert set(selected) <= {'ArenaBaselineBytes', 'ArenaBytesAllocated'}
        result.append({**dict(zip(('assignment', 'candidate', 'kind', 'phase'), key)),
                       'work_equal': before['work'] == after['work'],
                       'before_attempts': len(before['work']['attempts'] or []),
                       'after_attempts': len(after['work']['attempts'] or []),
                       'total_changes': changes, 'selected_arena_changes': selected})
    assert sum(row['work_equal'] for row in result) == 20
    return result


def session_rows(raw):
    rows = [json.loads(line.split('TRUSTED_GO_SESSION_STEP ', 1)[1])
            for line in raw.splitlines() if 'TRUSTED_GO_SESSION_STEP ' in line]
    assert len(rows) == 144
    for api in ('plain', 'profiled'):
        selected = [row for row in rows if row['API'] == api]
        assert [row['Step'] for row in selected] == list(range(72))
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
            assert row['ProducerKey'] == 8195 and row['VerifierEvents'] == 0
            assert row['BypassEvents'] == row['WouldCertify'] == 1 and row['NotWouldCertify'] == 0
            assert not row['FreshVerified'] and row['BypassCertified'] and row['NoEditAllocs'] == 0
            assert row['SharedTopLevel'] > 0
            if api == 'profiled':
                assert row['ReusedSubtrees'] > 0 and row['ReusedBytes'] > 0
        assert current == original
    return len(rows)


def benchmarks(raw):
    samples, order, seed = {}, {}, None
    for line in raw.splitlines():
        match = re.match(r'# seed: (\d+); position: (\d+)', line)
        if match:
            seed = int(match[1])
            assert seed not in order
            order[seed] = {'position': int(match[2]), 'names': []}
        if line.startswith('BenchmarkGTSAuthenticatedReuse/'):
            columns = line.split()
            name = re.sub(r'-\d+$', '', columns[0]).split('/', 1)[1]
            assert seed is not None and int(columns[1]) > 0
            assert name not in order[seed]['names']
            order[seed]['names'].append(name)
            metrics = {columns[i+1]: float(columns[i]) for i in range(2, len(columns), 2)}
            assert {'ns/op', 'B/op', 'allocs/op'} <= metrics.keys()
            samples.setdefault(name, {})[seed] = metrics
    assert set(order) == set(range(1, 21)) and len(samples) == 6
    assert all(set(rows) == set(order) for rows in samples.values())
    assert raw.count('\nPASS\n') == 20
    return samples, order


def performance(text):
    before, aorder = benchmarks(text('perf', 'before.txt'))
    after, border = benchmarks(text('perf', 'after.txt'))
    assert before.keys() == after.keys()
    for seed in range(1, 21):
        assert aorder[seed]['names'] == border[seed]['names']
        assert aorder[seed]['position']+border[seed]['position'] == 3
        assert aorder[seed]['position'] == (1 if seed % 2 else 2)
    result = {}
    for name in before:
        metrics = {}
        for metric in before[name][1]:
            a = [before[name][seed][metric] for seed in range(1, 21)]
            b = [after[name][seed][metric] for seed in range(1, 21)]
            am, bm = statistics.median(a), statistics.median(b)
            metrics[metric] = {'before_median': am, 'after_median': bm,
                               'median_change_percent': (bm/am-1)*100 if am else None,
                               'paired_ratio_median': statistics.median([y/x for x, y in zip(a, b)]) if all(a) else None}
        result[name] = metrics
    rss = {}
    for size in (140288, 1048576):
        arms = {}
        for arm in ('baseline', 'candidate'):
            values = []
            for seed in range(1, 21):
                raw = text('perf', f'rss-{arm}-{size}-{seed}.time')
                assert 'Exit status: 0' in raw
                values.append(int(re.search(r'Maximum resident set size \(kbytes\): (\d+)', raw)[1]))
                assert text('perf', f'rss-{arm}-{size}-{seed}.stderr') == ''
            arms[arm] = {'samples_kib': values, 'median_kib': statistics.median(values), 'maximum_kib': max(values)}
        arms['median_change_percent'] = (arms['candidate']['median_kib']/arms['baseline']['median_kib']-1)*100
        rss[str(size)] = arms
    retention = {}
    for arm in ('baseline', 'candidate'):
        rows = list(map(json.loads, text('perf', f'chain-{arm}.jsonl').splitlines()))
        by_step = {row['step']: row for row in rows}
        assert set(by_step) == {0, 10, 100, 1000, -1}
        assert by_step[1000]['heap_alloc'] <= by_step[100]['heap_alloc']*1.10+1048576
        assert 'Exit status: 0' in text('perf', f'chain-{arm}.time')
        assert text('perf', f'chain-{arm}.stderr') == ''
        retention[arm] = rows
    return {'paired_seeds': 20, 'benchmarks': result, 'rss': rss, 'retention': retention}


def validate(directory, source_root=None):
    summary = json.loads((directory/'summary.json').read_text())
    files = archive(directory/'evidence.tar.gz')
    index = json.loads(gzip.decompress((directory/'contents.json.gz').read_bytes()))
    assert files.keys() == index.keys()
    for name, data in files.items():
        assert index[name] == {'bytes': len(data), 'sha256': sha(data)}
    choices = {key: [value] for key, value in files.items()}
    for relative, expected in summary['inherited_archives'].items():
        path = directory/relative
        assert sha(path.read_bytes()) == expected
        for key, value in archive(path).items():
            choices.setdefault(key, []).append(value)

    def bound(virtual, digest=None):
        key = virtual.lstrip('/')
        prefix = 'harness_out/docker/go-authenticated-reuse-20261026/'
        if key.startswith(prefix):
            key = 'controls/' + key[len(prefix):]
        candidates = [files[key]] if digest is None and key in files else choices.get(key, [])
        if virtual.startswith(('/candidate/', '/baseline/')):
            relative = virtual.split('/', 2)[2]
            candidates = choices.get('candidate-inputs/'+relative, [])
            if source_root is not None:
                candidates += [(source_root/relative).read_bytes()]
        if digest is not None:
            candidates = [data for data in candidates if sha(data) == digest]
        assert candidates and all(data == candidates[0] for data in candidates), (virtual, digest)
        return candidates[0]

    text = lambda stage, name: files[f'outputs/{stage}/{name}'].decode()
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
    passed = {}
    for stage in ('v1', 'extended', 'perf'):
        assert json.loads(text(stage, 'finished.json' if stage == 'v1' else 'exit.json'))['exit_code'] == 0
        state = json.loads(text(stage, 'container-state.json'))
        assert state['state']['ExitCode'] == 0 and not state['state']['OOMKilled']
        assert state['memory'] == state['memory_swap'] == 4*1024**3 and state['nano_cpus'] == 10**9 and state['network'] == 'none'
        for when in ('before', 'after'):
            for stem in ('source', 'controls'):
                assert text(stage, f'{stem}-{when}.log') == ''
        for name in summary['passing_logs'][stage]:
            raw = text(stage, name)
            assert not re.search(r'--- (FAIL|SKIP):|^FAIL\b|panic:|WARNING: DATA RACE', raw, re.M)
            assert re.search(r'^PASS$', raw, re.M)
            passed[stage+'/'+name] = len(re.findall(r'^\s*--- PASS:', raw, re.M))
    for language in ('go', 'yaml'):
        for route in ('default', 'candidate'):
            assert text('v1', f'baseline-{language}-{route}.json') == text('v1', f'candidate-{language}-{route}.json')
    assert work_comparison(text) == summary['complete_operation_comparisons']
    assert session_rows(text('v1', 'clean-plain.log')+text('v1', 'clean-profiled.log')) == 144
    assert session_rows(text('extended', 'race-clean-plain.log')+text('extended', 'race-clean-profiled.log')) == 144
    for route in (0, 1):
        assert 'session_steps=72 no_edit_allocs=0' in text('v1', f'invariant-{route}.log')
    assert passed['v1/locked-c.log'] == passed['extended/locked-c027.log'] == 127
    assert '10eb5ac119eec9cb4267a8e035066473983f5f3a' in text('extended', 'runtime-probe.log')
    digest = lambda raw: re.findall(r'deep digest ([0-9a-f]{64})', raw)
    assert digest(text('v1', 'locked-c.log')) == digest(text('extended', 'locked-c027.log'))
    for when in ('before', 'after'):
        for stem in ('oracle', 'binding', 'extended'):
            assert text('extended', f'{stem}-{when}.log') == ''
        assert text('perf', f'perf-{when}.log') == ''
        staged = json.loads(text('extended', f'staged-{when}.json'))
        assert staged['cgo_files'] == 14 and staged['bodies_byte_identical_to_frozen_adaptation']
        assert not staged['module_cache_changed'] and not staged['project_dependencies_changed']
    runtime = json.loads(files['controls/overlay.runtime.json'])['Replace']
    oracle = json.loads(files['controls/extended/overlay.c027.json'])['Replace']
    assert len(runtime) == 14 and len(oracle) == 30
    assert all(oracle.get(target) == backing for target, backing in runtime.items())
    for mapping in ('overlay.runtime.json', 'overlay.tests.json', 'extended/overlay.tests.json', 'extended/overlay.c027.json', 'perf/overlay.json'):
        for backing in json.loads(files['controls/'+mapping])['Replace'].values():
            bound(backing)
    for stage, log in (('v1', 'real-go.log'), ('extended', 'race-go.log')):
        records = re.findall(r'REAL_ADMISSION_SUMMARY source=(\S+) bytes=(\d+) cases=(\d+) would_certify=(\d+)', text(stage, log))
        assert len(records) == 4 and sum(int(row[2]) for row in records) == 108
        assert sum(int(row[3]) for row in records) == 62
    lifecycle = re.findall(r'lifecycle=(\S+) verifier_events=(\d+) bypass_events=(\d+) certified=(true|false) fresh_equal=true', text('extended', 'lifecycle.log'))
    assert len(lifecycle) == 13 and sum(int(row[2]) for row in lifecycle) == 2
    assert performance(text) == summary['performance']
    v11 = files['controls/v11/inputs.sha256']
    for line in v11.decode().splitlines():
        digest, virtual = line.split('  ', 1)
        bound(virtual, digest)
    completed = json.loads(files['controls/final-correctness/out/completed.json'])
    assert completed['exit_code'] == 0 and completed['v11_inputs_sha256'] == sha(v11)
    assert completed['fleet_languages'] == 206 and completed['fleet_route_runs'] == 412 and completed['oracle_matching_cases'] == 127
    for name, digest in completed['files_sha256'].items():
        assert sha(files['controls/final-correctness/out/'+name]) == digest
    fleet = json.loads(files['controls/out-v11-fleet-counters/summary.json'])
    assert fleet['languages'] == 206 and fleet['routes_per_language'] == 2 and fleet['baseline_candidate_ledgers_byte_identical']
    for stage in ('out-v11-counters', 'out-v11-fleet-counters', 'out-v11-perf', 'out-v11-alloc-profile'):
        state = json.loads(files['controls/'+stage+'/container-state.json'])
        assert state['state']['ExitCode'] == 0 and not state['state']['OOMKilled']
    assert performance(lambda stage, name: files['controls/out-v11-perf/'+name].decode()) == summary['v11']['performance']
    assert '+253.38%' in files['controls/out-v11-perf/benchstat.txt'].decode()
    assert '+54.54%' in files['controls/out-v11-perf/benchstat.txt'].decode()
    assert summary['v11']['status'] == 'scoped native-Go reuse and primary trio checks passed; broader gates unverified' and summary['v11']['main_merges_held'] and not summary['v11']['runtime_promoted']
    authority = files['controls/review/result.log'].decode()
    for case in ('copy', 'modified-symbol-table', 'forged-receipt'):
        assert f'untrusted {case} acquired producer key 8195' in authority
    assert '--- FAIL: TestLanguageCopyDoesNotAuthenticate' in authority
    assert summary['v1_authority_review']['status'] == 'failed'
    for stage in ('profile', 'v2-pilot', 'v3-pilot', 'v3-proof'):
        state = json.loads(text(stage, 'container-state.json'))
        assert state['state']['ExitCode'] == 0 and not state['state']['OOMKilled']
        assert state['memory'] == state['memory_swap'] == 4*1024**3 and state['nano_cpus'] == 10**9 and state['network'] == 'none'
    for line in text('v2-pilot', 'v2.sha256').splitlines():
        digest, virtual = line.split('  ', 1)
        bound(virtual, digest)
    for arm in ('before', 'after'):
        raw = text('v2-pilot', arm+'.txt')
        assert set(map(int, re.findall(r'# seed: (\d+); position:', raw))) == set(range(21, 25))
        assert raw.count('\nPASS\n') == 4
    for stage in ('v3-proof', 'v3-pilot'):
        for line in text(stage, 'v3.sha256').splitlines():
            digest, virtual = line.split('  ', 1)
            bound(virtual, digest)
    assert re.search(r'^PASS$', text('v3-proof', 'proof-tests.log'), re.M)
    for arm in ('before', 'after'):
        raw = text('v3-pilot', arm+'.txt')
        assert set(map(int, re.findall(r'# seed: (\d+); position:', raw))) == set(range(25, 29))
        assert raw.count('\nPASS\n') == 4
    assert summary['v3_pilot']['status'] == 'rejected' and not summary['v3_pilot']['qualification']
    assert summary['v2_pilot']['status'] == 'rejected' and not summary['v2_pilot']['qualification']
    assert summary['qualification']['status'] == 'rejected'
    assert '+21.77%' in text('perf', 'benchstat.txt')
    assert summary['main_merges_held'] and not summary['graduated'] and not summary['owner_exception_used']
    return {'validated': True, 'members': len(files), 'control_checks': checks, 'passing_records': passed,
            'release_bypass_steps': 144, 'race_bypass_steps': 144, 'complete_work_equal': 20,
            'complete_work_improved': 4, 'paired_seeds': 20, 'main_merges_held': True}


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--source-root', type=Path)
    args = parser.parse_args()
    print(json.dumps(validate(Path(__file__).resolve().parent, args.source_root), indent=2))
