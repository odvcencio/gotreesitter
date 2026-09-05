from pathlib import Path
import hashlib, json, re, subprocess
root=Path(__file__).resolve().parent
stages=[]
for path in sorted(root.glob('20*-lexer-token-output-*/metadata.txt')):
    data=dict(line.split('=',1) for line in path.read_text().splitlines() if '=' in line)
    assert data['exit_code']=='0' and data['oom_killed']=='false' and data['wall_timed_out']=='false', path
    stages.append({'directory':str(path.parent.relative_to(root)), 'exit_code':0,'oom_killed':False,'wall_timed_out':False,'start_utc':data['run_start_utc'],'end_utc':data['run_end_utc']})
assert len(stages)==8, len(stages)
(root/'stage-summary.json').write_text(json.dumps(stages,indent=2)+'\n')
rss={'fixture':'grammargen_lr','bytes':235626,'route':'forced legacy','scope':'Full parse process peak RSS includes admission, validation, and warm-up. It measures one legacy full parse and exercises the shared lexer. It does not isolate temporary proof stack space. It does not establish retention or compact memory behavior.','observations_kib':{}}
for role in ('baseline','candidate'):
    values=[]
    for pair in (1,2,3):
        raw=(root/f'{role}-large-rss-{pair}.txt').read_text()
        assert 'Exit status: 0' in raw
        value=re.search(r'Maximum resident set size \(kbytes\): (\d+)',raw)
        assert value
        values.append(int(value[1]))
    rss['observations_kib'][role]=values
    controls=(root/f'{role}-correctness.txt').read_text()
    assert controls.rstrip().endswith('PASS')
    assert len(re.findall(r'^--- PASS:',controls,re.M)) == (57 if role == 'baseline' else 60)
(root/'rss-summary.json').write_text(json.dumps(rss,indent=2)+'\n')
binaries={str(path.relative_to(root)):hashlib.sha256(path.read_bytes()).hexdigest() for path in root.rglob('*.test') if 'c-reference-cache' not in path.parts}
(root/'binary-hashes.json').write_text(json.dumps(binaries,indent=2)+'\n')
preflight=json.loads((root/'probe/candidate-preflight.json').read_text())
assert preflight['complete'] and preflight['initial_routes']==1 and preflight['initial_fallbacks']==0
assert len(preflight['edits'])==5
for edit in preflight['edits']:
    assert edit['complete'] and edit['same_root'] and edit['dependency_checks']==1 and edit['reparse_nanos']==0 and edit['new_nodes_allocated']==0
    assert edit['deep_tree_sha256']==edit['fresh_deep_tree_sha256']
print(json.dumps({'stages':stages,'rss':rss,'binary_count':len(binaries)},indent=2))
