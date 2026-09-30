#!/usr/bin/env python3
"""Reduce exhaustive per-edit decisions without inventing reparse attribution."""
import argparse
import collections
import hashlib
import json
import pathlib
import statistics


def reason_group(reason):
    if reason.startswith(('dispatch/multiple_live_stacks','reparse/dispatch/multiple_live_stacks')):
        return 'multiple live stacks: reuse not offered'
    if reason.startswith('reparse/'):
        return reason[8:] + ': reuse not offered'
    if reason.startswith('fresh/'):
        return 'fresh fallback: ' + reason_group(reason[6:])
    if reason.startswith(('tokenInvariantPrimitive', 'tokenInvariantEditDependencies', 'verification/primitive', 'verification/lexical', 'leaf_proof', 'tryTokenInvariantLeafEdit', 'scanTokenInvariantEditedLeaf', 'verification/edited_leaf')):
        return 'lexical dependency / edited-leaf proof'
    if reason.startswith(('verification/recovery_frontier', 'incrementalTreesStructurallyEqual', 'verification/tree_compare', 'recovery_frontier_trigger')) or reason == 'recovery_frontier_unproven':
        return 'fresh recovery verification'
    if reason == 'checkpointed_scanner_prefix_frontier_unproven' or 'prefix_frontier' in reason:
        return 'scanner prefix frontier fallback'
    if reason.startswith('retry/accepted_error'):
        return 'accepted-error retry'
    if reason.startswith('retry/'):
        return 'full / memory-budget retry'
    if reason.startswith('route') or reason == 'reparse_and_rebuild':
        return 'reparse / rebuild / dispatch'
    if reason.startswith(('rebuild/', 'reuse/tree_rebind')):
        return 'tree rebuilding / normalization'
    if reason == 'tree_edit':
        return 'Tree.Edit'
    if 'already_passed' in reason or reason.startswith('candidates/n.startByte'):
        return 'cursor: already passed'
    if reason.startswith('verification/source_bytes') or 'ancestor_changed_bytes' in reason or 'nodeBytesUnchanged' in reason or 'changed_bytes' in reason:
        return 'source-byte / ancestor edit guard'
    if 'has_error' in reason or 'HasError()' in reason:
        return 'error-bearing node guard'
    if 'dirty' in reason:
        return 'dirty-node guard'
    if 'fragile' in reason or 'isFragile' in reason:
        return 'fragile-subtree guard'
    if 'ownsCurrentFrontier' in reason or 'ownership_frontier' in reason or 'PreGotoState() != state' in reason or 'interior_frontier' in reason:
        return 'parser ownership / pre-goto frontier guard'
    if 'scanner' in reason or 'checkpoint' in reason:
        return 'scanner checkpoint / leaf-only guard'
    if 'topLevelSiblingBlockSpliceEligible' in reason:
        return 'top-level scope guard'
    if 'compact_state_proof' in reason or 'compactNodeStateProofAvailable' in reason or 'compactNodeMayBeReused' in reason or 'compact_frontier' in reason:
        return 'compact node / frontier proof guard'
    if 'leaf_symbol_changed' in reason or 'leaf_boundary_changed' in reason or 'leaf_shift_state' in reason or reason.startswith('reuseTargetState/'):
        return 'leaf lexical / shift / goto compatibility guard'
    if reason.startswith('verification/compact_dependency'):
        return 'compact lookahead dependency guard'
    if reason.startswith('candidateState/'):
        return 'compact candidate eligibility guard'
    if reason.startswith('selection/'):
        return 'reuse selection / cursor overhead'
    if reason.startswith('compact/') or reason.startswith('tryCompactIncrementalReuse/'):
        return 'compact attempt / admission'
    if reason.startswith(('tryReuseSubtree/', 'reuseNode/')):
        return 'other subtree eligibility / resume guard'
    if reason.startswith(('advance/', 'reusableIndexedEntry/', 'collectTopLevelCandidates/', 'reset/')):
        return 'other cursor filter / eligibility guard'
    return reason


def reduce_files(directory):
    languages=[]
    for path in sorted(directory.glob('*.jsonl')):
        data=[json.loads(line) for line in path.read_text().splitlines() if line.strip()]
        rows=[row for row in data if 'census' in row]
        if not rows:
            continue
        lang=rows[0]['language']
        expected={(size,kind,site) for size in (32768,140288) for kind in ('one_byte','100_byte','splice','numeric_replace') for site in range(3)}
        actual={(row['target_bytes'],row['class'],row['site']) for row in rows}
        if actual != expected or len(rows)!=len(expected):
            raise ValueError(f'{lang}: incomplete or duplicate edit matrix: {sorted(expected-actual)}')
        raw=collections.defaultdict(lambda:dict(nanos=0,lost_nodes=0,decisions=0))
        groups=collections.defaultdict(lambda:dict(nanos=0,lost_nodes=0,decisions=0))
        edit_total=observer_total=lost_total=old_total=reused_total=0
        for row in rows:
            report=row['census']
            if not report.get('schema'):
                continue
            accounted=sum(r['nanos'] for r in report['rows'])+report['observe_nanos']
            if accounted != report['edit_nanos']:
                raise ValueError(f'{lang}: overlapping or missing timing {accounted} != {report["edit_nanos"]}')
            if report['old_nodes']!=report['lost_nodes']+report['reused_nodes'] or sum(r['lost_nodes'] for r in report['rows'])!=report['lost_nodes']:
                raise ValueError(f'{lang}: incomplete old-node partition')
            edit_total+=report['edit_nanos'];observer_total+=report['observe_nanos'];lost_total+=report['lost_nodes'];old_total+=report['old_nodes'];reused_total+=report['reused_nodes']
            for reason in report['rows']:
                for key in ('nanos','lost_nodes','decisions'):
                    raw[reason['reason']][key]+=reason[key]
                    groups[reason_group(reason['reason'])][key]+=reason[key]
        def ranked(values):
            return sorted([dict(reason=reason,**value,time_share_pct=100*value['nanos']/max(1,edit_total),lost_node_share_pct=100*value['lost_nodes']/max(1,lost_total)) for reason,value in values.items()],key=lambda row:(-row['nanos'],-row['lost_nodes'],row['reason']))
        workloads=[]
        for size in (32768,140288):
            for kind in ('one_byte','100_byte','splice','numeric_replace'):
                selected=[row for row in rows if row['target_bytes']==size and row['class']==kind]
                go=[n for row in selected for n in row['go_nanos']];c=[n for row in selected for n in row['c_nanos']]
                workloads.append(dict(target_bytes=size,actual_bytes=selected[0]['bytes'],edit_class=kind,go_median_ns=statistics.median(go),go_min_ns=min(go),go_max_ns=max(go),c_median_ns=statistics.median(c),go_c_ratio=statistics.median(go)/statistics.median(c),fresh_go_median_ns=statistics.median(row['fresh_go_nanos'] for row in selected)))
        # Preserve each edit's counters, digests, and verdicts but keep full events
        # in the raw artifact. Its digest authenticates that unabridged event log.
        compact=[]
        for row in rows:
            item=dict(row);report=dict(item['census']);report['event_count']=len(report.pop('events',[]));item['census']=report;compact.append(item)
        languages.append(dict(language=lang,raw_sha256=hashlib.sha256(path.read_bytes()).hexdigest(),edits=len(rows),old_nodes=old_total,reused_nodes=reused_total,lost_nodes=lost_total,reuse_share_pct=100*reused_total/max(1,old_total),edit_nanos=edit_total,observer_nanos=observer_total,observer_share_pct=100*observer_total/max(1,edit_total),incremental_go_mismatches=sum(not r['incremental_equals_fresh_go'] for r in rows),fresh_go_c_mismatches=sum(not r['fresh_go_equals_c'] for r in rows),incremental_c_mismatches=sum(not r['incremental_c_equals_fresh_c'] for r in rows),observer_mismatches=sum(not r['observed_equals_unobserved'] for r in rows),ranked_reasons=ranked(raw),ranked_groups=ranked(groups),workloads=workloads,edits_detail=compact,oracle=data[0].get('oracle')))
    return dict(schema='gts-incremental-reuse-census-summary/v1',method='Exclusive diagnostic wall time; observer cost explicit; lost nodes partitioned by selected-result identity and nearest observed refusal. Reparse time is not assigned to a guard without evidence.',languages=languages)


def main():
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('directory',type=pathlib.Path);parser.add_argument('--output',required=True,type=pathlib.Path);args=parser.parse_args()
    result=reduce_files(args.directory);args.output.write_text(json.dumps(result,indent=2)+'\n')
    for lang in result['languages']:
        print(f"{lang['language']}: {lang['edits']} edits, {lang['reuse_share_pct']:.1f}% nodes retained, Go/fresh mismatches={lang['incremental_go_mismatches']}, fresh Go/C mismatches={lang['fresh_go_c_mismatches']}")
        for row in lang['ranked_groups'][:5]:
            print(f"  {row['reason']}: {row['time_share_pct']:.1f}% time / {row['lost_node_share_pct']:.1f}% lost nodes")

if __name__=='__main__':
    main()
