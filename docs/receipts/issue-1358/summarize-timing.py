import re, json, statistics, pathlib, sys
root=pathlib.Path(sys.argv[1])
rows={}
for role in ['base','head']:
    seed=None; data={}
    raw = (root/f'awk-{role}.txt').read_text()
    assert raw.rstrip().endswith('# status: complete')
    for line in raw.splitlines():
        match=re.match(r'# seed: (\d+);',line)
        if match:seed=int(match[1])
        fields=line.split()
        if not fields or not fields[0].startswith('BenchmarkAWKIssue1358/'):continue
        _,mode,engine=fields[0].split('/')
        metrics={fields[i+1]:float(fields[i]) for i in range(2,len(fields)-1,2)}
        data.setdefault(seed,{}).setdefault(mode,{})[engine]=metrics
    result={}
    for mode in ['Full','Insert','NoEdit']:
        samples=[]
        for seed,cases in sorted(data.items()):
            engines=cases[mode]
            go={metric:statistics.mean(engines[e][metric] for e in ['GoFirst','GoSecond']) for metric in ['ns/op','B/op','allocs/op','errors/op']}
            c=statistics.mean(engines[e]['ns/op'] for e in ['CFirst','CSecond'])
            samples.append({'seed':seed,**go,'C ns/op':c,'Go/C':go['ns/op']/c})
        assert len(samples) == 20 and {s['seed'] for s in samples} == set(range(1, 21))
        result[mode]={metric:statistics.median(s[metric] for s in samples) for metric in samples[0] if metric!='seed'}
        result[mode]['seeds']=len(samples)
        result[mode]['samples']=samples
    rows[role]=result
for mode in ['Full','Insert','NoEdit']:
    before=rows['base'][mode];after=rows['head'][mode]
    print(mode)
    for metric in ['ns/op','C ns/op','Go/C','B/op','allocs/op','errors/op']:
        change=(after[metric]/before[metric]-1)*100 if before[metric] else None
        print(f'  {metric}: {before[metric]:.6g} -> {after[metric]:.6g}; change={change}')
(root/'performance-summary.json').write_text(json.dumps(rows,indent=2)+'\n')
