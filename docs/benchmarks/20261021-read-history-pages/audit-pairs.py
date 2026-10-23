from pathlib import Path
import json, re, statistics, sys

directory = Path(sys.argv[1])
language = sys.argv[2]
start = {'go': 147001, 'yaml': 148001}[language]
expected_count = {'go': 7, 'yaml': 6}[language]
expected_seeds = list(range(start, start + 20))

def parse(path, role):
    text = path.read_text()
    for value in ['# protocol: paired-alternating-seeds', '# GOMAXPROCS: 1', '# count per process: 1',
                  '# benchtime: 750ms', '# suite seed environment: GTS_BENCHMARK_SEED', '# completed runs: 20']:
        assert value in text, (path.name, value)
    assert text.rstrip().endswith('# status: complete')
    result = {}
    current = None
    for line in text.splitlines():
        match = re.match(r'# seed: (\d+); position: (\d+)$', line)
        if match:
            seed, position = map(int, match.groups())
            assert seed not in result
            expected_position = 1 if seed % 2 else 2
            if role == 'head':
                expected_position = 3 - expected_position
            assert position == expected_position
            current = result[seed] = {'position': position, 'rows': {}, 'order': [], 'declared_order': None}
        elif line.startswith('Benchmark'):
            parts = line.split()
            name, iterations = parts[:2]
            assert current is not None and name not in current['rows'] and int(iterations) > 0
            assert (len(parts) - 2) % 2 == 0
            metrics = {parts[i + 1]: float(parts[i]) for i in range(2, len(parts), 2)}
            assert {'ns/op', 'B/op', 'allocs/op'} <= set(metrics)
            current['rows'][name] = metrics
            current['order'].append(name)
        elif line.startswith('# workload order: '):
            current['declared_order'] = line[len('# workload order: '):].split(',')
    assert list(result) == expected_seeds
    for seed, value in result.items():
        assert len(value['rows']) == expected_count
        assert value['order'] == value['declared_order']
    assert len({tuple(value['order']) for value in result.values()}) >= 4
    return result

before = parse(directory / (language + '-before.txt'), 'baseline')
after = parse(directory / (language + '-after.txt'), 'head')
names = set(before[start]['rows'])
work_metrics = {'tokens/op', 'public-nodes/op', 'compact-routes/op', 'compact-fallbacks/op'}
for seed in expected_seeds:
    assert before[seed]['order'] == after[seed]['order']
    assert set(before[seed]['rows']) == set(after[seed]['rows']) == names
    for name in names:
        b, a = before[seed]['rows'][name], after[seed]['rows'][name]
        assert set(b) == set(a)
        for unit in work_metrics & set(b):
            assert b[unit] == a[unit], (seed, name, unit, b[unit], a[unit])
        if name.endswith('/no_edit_dfa'):
            assert b['B/op'] == a['B/op'] == b['allocs/op'] == a['allocs/op'] == 0

ratchets = []
for name in sorted(names):
    entry = {'name': name, 'metrics': {}}
    for unit in ['ns/op', 'B/op', 'allocs/op']:
        b = statistics.median(before[seed]['rows'][name][unit] for seed in expected_seeds)
        a = statistics.median(after[seed]['rows'][name][unit] for seed in expected_seeds)
        ratio = a / b if b else None
        entry['metrics'][unit] = {'before_median': b, 'after_median': a, 'ratio': ratio,
                                  'inside_10pct_ratchet': a == 0 if b == 0 else ratio <= 1.10}
    ratchets.append(entry)

audit = {'pairing_passed': True, 'language': language, 'seeds': expected_seeds,
         'rows_per_variant': 20 * expected_count, 'identical_child_order_per_pair': True,
         'alternating_revision_order': True, 'randomized_across_seeds': True,
         'all_reported_work_metrics_equal': True, 'no_edit_zero_bytes_and_allocations': language == 'go',
         'ratchets': ratchets,
         'all_median_ratchets_pass': all(v['inside_10pct_ratchet'] for row in ratchets for v in row['metrics'].values()),
         'final_source_post_audit_required': True}
(directory / (language + '-pair-audit.json')).write_text(json.dumps(audit, indent=2) + '\n')
print(json.dumps({key: audit[key] for key in ['language', 'rows_per_variant', 'pairing_passed', 'all_median_ratchets_pass']}))
