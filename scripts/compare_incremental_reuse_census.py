#!/usr/bin/env python3
"""Require unchanged results, work and node identity before comparing timing."""
import argparse
import json
import pathlib


WORK = (
    'TokensConsumed', 'NodesAllocated', 'Iterations', 'MaxStacksSeen',
    'GSSNodesAllocated', 'ParentNodesAllocated', 'LeafNodesAllocated',
    'MergeStacksIn', 'MergeStacksOut', 'NormalizationNodesVisited',
    'LeafNodesConstructed', 'ParentNodesConstructed',
    'CompactIncrementalReusedSubtrees', 'CompactIncrementalReusedBytes',
)
RESULT = (
    'source_sha256', 'edited_sha256', 'go_digest', 'fresh_go_digest', 'c_digest',
    'incremental_equals_fresh_go', 'fresh_go_equals_c',
    'incremental_c_equals_fresh_c', 'observed_equals_unobserved',
)


def read(path):
    rows = {}
    with path.open() as source:
        for line in source:
            row = json.loads(line)
            if 'census' not in row:
                continue
            row['census'].pop('events', None)
            key = (row['target_bytes'], row['class'], row['site'])
            if key in rows:
                raise ValueError(f'duplicate edit: {path.name} {key}')
            rows[key] = row
    expected = {(size, kind, site) for size in (32768, 140288)
                for kind in ('one_byte', '100_byte', 'splice', 'numeric_replace')
                for site in range(3)}
    if rows.keys() != expected:
        raise ValueError(f'incomplete edit matrix: {path.name}')
    return rows


def compare(before, after, language):
    left, right = read(before / (language + '.jsonl')), read(after / (language + '.jsonl'))
    totals = {key: [0, 0] for key in WORK}
    totals.update({key: [0, 0] for key in ('old_nodes', 'reused_nodes', 'lost_nodes')})
    for edit, a in left.items():
        b = right[edit]
        for key in RESULT:
            if a[key] != b[key]:
                raise ValueError(f'{language} {edit}: changed {key}')
        if not b['observed_equals_unobserved']:
            raise ValueError(f'{language} {edit}: observer changed result')
        for key in WORK:
            x, y = a['runtime'][key], b['runtime'][key]
            if x != y:
                raise ValueError(f'{language} {edit}: {key}: {x} -> {y}')
            totals[key][0] += x
            totals[key][1] += y
        for key in ('old_nodes', 'reused_nodes', 'lost_nodes'):
            x, y = a['census'][key], b['census'][key]
            if x != y:
                raise ValueError(f'{language} {edit}: {key}: {x} -> {y}')
            totals[key][0] += x
            totals[key][1] += y
    return dict(language=language, edits=len(left), exact_results=True,
                exact_work_and_reuse=True, before_after=totals)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('before', type=pathlib.Path)
    parser.add_argument('after', type=pathlib.Path)
    parser.add_argument('languages', nargs='+')
    parser.add_argument('--output', type=pathlib.Path)
    args = parser.parse_args()
    results = [compare(args.before, args.after, lang) for lang in args.languages]
    rendered = json.dumps(dict(schema='gts-incremental-reuse-census-comparison/v1',
                               languages=results), indent=2) + '\n'
    if args.output:
        args.output.write_text(rendered)
    else:
        print(rendered, end='')


if __name__ == '__main__':
    main()
