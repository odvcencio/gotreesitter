#!/usr/bin/env python3
"""Validate completed paired runs and summarize every edit workload."""
import argparse
import collections
import hashlib
import json
import pathlib
import re
import statistics


LANGUAGES = ('go', 'javascript', 'typescript', 'python', 'rust', 'java',
             'c_sharp', 'powershell')
METRICS = ('ns/op', 'B/op', 'allocs/op', 'go-ns/edit', 'c-ns/edit', 'go/c',
           'go-tokens/edit', 'go-nodes/edit')
WORKLOADS = {(size, kind) for size in (32768, 140288)
             for kind in ('one_byte', '100_byte', 'splice')}


def read(path):
    raw = path.read_bytes()
    text = raw.decode()
    if text.splitlines()[-1] != '# status: complete':
        raise ValueError(f'{path.name}: incomplete campaign')
    completed = re.findall(r'^# completed seed: (\d+);', text, re.M)
    if sorted(map(int, completed)) != list(range(1, 21)):
        raise ValueError(f'{path.name}: expected each of seeds 1..20 once')
    for header in ('# benchtime: 750ms', '# GOMAXPROCS: 1',
                   '# count per process: 1', '# protocol: paired-alternating-seeds'):
        if header not in text.splitlines():
            raise ValueError(f'{path.name}: missing {header}')
    samples = collections.defaultdict(lambda: collections.defaultdict(list))
    seen = set()
    seed = None
    role = re.search(r'^# role: (head|baseline)$', text, re.M)
    if role is None:
        raise ValueError(f'{path.name}: missing paired role')
    for line in text.splitlines():
        marker = re.fullmatch(r'# seed: (\d+); position: ([12])', line)
        if marker:
            seed, position = map(int, marker.groups())
            expected = 1 if (seed % 2 == 0) == (role[1] == 'head') else 2
            if position != expected:
                raise ValueError(f'{path.name}: wrong paired position for seed {seed}')
        if not line.startswith('BenchmarkIncrementalReuseCensus/'):
            continue
        fields = line.split()
        name = re.fullmatch(r'BenchmarkIncrementalReuseCensus/(\d+)/(\w+)(?:-\d+)?', fields[0])
        if name is None:
            raise ValueError(f'{path.name}: unexpected benchmark name')
        key = (int(name[1]), name[2])
        if seed not in range(1, 21) or (seed, key) in seen:
            raise ValueError(f'{path.name}: missing seed or duplicate sample {seed}/{key}')
        seen.add((seed, key))
        values = {unit: float(value) for value, unit in zip(fields[2::2], fields[3::2])}
        for metric in METRICS:
            samples[key][metric].append(values[metric])
    if samples.keys() != WORKLOADS:
        raise ValueError(f'{path.name}: incomplete edit matrix')
    if seen != {(seed, key) for seed in range(1, 21) for key in WORKLOADS}:
        raise ValueError(f'{path.name}: incomplete per-seed edit matrix')
    for key, metrics in samples.items():
        for metric, values in metrics.items():
            if len(values) != 20:
                raise ValueError(f'{path.name}: {key}/{metric} needs 20 samples')
    return samples, hashlib.sha256(raw).hexdigest()


def compare(directory, fixtures, languages):
    rows = []
    hashes = {}
    for language in languages:
        before, before_hash = read(directory / f'{language}-before.txt')
        after, after_hash = read(directory / f'{language}-after.txt')
        stat_path = directory / f'{language}-benchstat.txt'
        if not stat_path.is_file():
            raise ValueError(f'{language}: missing benchstat comparison')
        hashes[language] = dict(before=before_hash, after=after_hash,
                                benchstat=hashlib.sha256(stat_path.read_bytes()).hexdigest())
        for size, kind in sorted(WORKLOADS):
            metrics = {}
            for metric in METRICS:
                a, b = before[size, kind][metric], after[size, kind][metric]
                x, y = statistics.median(a), statistics.median(b)
                metrics[metric] = dict(before=x, after=y,
                    change_pct=100 * (y / x - 1) if x else (0 if y == 0 else None),
                    before_range=[min(a), max(a)], after_range=[min(b), max(b)])
                if metric in ('go-tokens/edit', 'go-nodes/edit') and (set(a) != set(b) or len(set(a)) != 1):
                    raise ValueError(f'{language}/{size}/{kind}: changed deterministic {metric}')
            fixture = next(f for f in fixtures if f['language'] == language and f['target_bytes'] == size)
            rows.append(dict(language=language, target_bytes=size, bytes=fixture['bytes'],
                             edit=kind, samples=20, metrics=metrics,
                             dependency_commit='4afda6f2d5dccb0c4742b30aac6af56f02be4a3d'
                                 if language == 'powershell' else None))
    return dict(schema='gts-incremental-reuse-benchmark-summary/v1',
                method='Median of 20 paired alternating shuffle seeds, GOMAXPROCS=1, count=1, 750ms, benchmem, CPU affinity 5. ns/op, B/op and allocs/op describe a four-edit cycle (two Go, two C); custom metrics describe one edit.',
                raw_sha256=hashes, workloads=rows)


def render(summary, revision):
    lines = ['Each row reports medians from 20 paired randomized processes. Timing excludes fresh-tree setup and includes Tree.Edit. Standard B/op and allocs/op cover a four-edit cycle; Go and C columns cover one edit. PowerShell* includes the existing dependency fix in both measurement checkouts.', '',
             '| Language | Target / actual bytes | Edit | Go ms before → after | C ms before → after | Go/C before → after | Go time change | Cycle B/op before → after | Cycle allocs/op before → after |',
             '|---|---|---|---:|---:|---:|---:|---:|---:|']
    receipts = []
    for row in summary['workloads']:
        m = row['metrics']
        language = row['language'] + ('*' if row['dependency_commit'] else '')
        pair = lambda unit, scale, fmt: f"{format(m[unit]['before']/scale, fmt)} → {format(m[unit]['after']/scale, fmt)}"
        lines.append(f"| {language} | {row['target_bytes']} / {row['bytes']} | {row['edit']} | {pair('go-ns/edit', 1e6, '.3f')} | {pair('c-ns/edit', 1e6, '.3f')} | {pair('go/c', 1, '.2f')} | {m['go-ns/edit']['change_pct']:+.2f}% | {pair('B/op', 1, ',.0f')} | {pair('allocs/op', 1, ',.1f')} |")
        ratio = m['go/c']
        dependency = ' + matched dependency 4afda6f2d' if row['dependency_commit'] else ''
        receipts.append(f"METRIC: {row['language']}/{row['target_bytes']}/{row['edit']} edit Go/C | {ratio['before']:.3f} -> {ratio['after']:.3f} | {revision}{dependency} | {row['bytes']} bytes, middle-site {row['edit']}, 20 seeds")
    lines += ['', 'Directional timing changes are reported for every workload, including nonsignificant increases. See the per-language benchstat files for confidence intervals and p-values; observer-enabled census timings are separate. Go/C ratios on unequal parse results are diagnostic costs.', '', '```text', *receipts, '```', '']
    return '\n'.join(lines)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('directory', type=pathlib.Path)
    parser.add_argument('--languages', nargs='+', choices=LANGUAGES, default=LANGUAGES)
    parser.add_argument('--fixtures', type=pathlib.Path,
                        default=pathlib.Path('docs/receipts/incremental-reuse-census/fixtures.json'))
    parser.add_argument('--revision', required=True)
    parser.add_argument('--output', type=pathlib.Path, required=True)
    parser.add_argument('--markdown', type=pathlib.Path, required=True)
    args = parser.parse_args()
    fixtures = json.loads(args.fixtures.read_text())['files']
    summary = compare(args.directory, fixtures, args.languages)
    summary['instrument_revision'] = args.revision
    summary['baseline_revision'] = 'f9828512cc58f1bc77b856c2b3451e85266c38b1'
    args.output.write_text(json.dumps(summary, indent=2) + '\n')
    args.markdown.write_text(render(summary, args.revision))


if __name__ == '__main__':
    main()
