import json,subprocess,statistics,random,pathlib,re,os
root=pathlib.Path(os.environ['Q2_RECEIPT_DIR'])
oracle=os.environ['C_ORACLE_ARTIFACT'];source=os.environ['GO_CLIFF_INPUT']
rows=[]
for seed in range(1,21):
    policies=[False,True]
    random.Random(seed).shuffle(policies)
    for enabled in policies:
        windows=[]
        axes=['go','c','c','go'] if seed%2 else ['c','go','go','c']
        for i,axis in enumerate(axes):
            stem=root/f'ratio-{seed}-{int(enabled)}-{i}-{axis}'
            cmd=[os.environ['GO_RETRY_PROBE' if enabled else 'GO_RETRY_BASELINE_PROBE'],'-source',source,'-budget='+str(enabled).lower(),'-reps','3'] if axis=='go' else [oracle,'measure','full',source,'1','3','10000000']
            output=subprocess.check_output(['/usr/bin/time','-v','-o',str(stem)+'.time']+cmd,text=True)
            pathlib.Path(str(stem)+'.txt').write_text(output)
            times=pathlib.Path(str(stem)+'.time').read_text()
            rss=int(re.search(r'Maximum resident set size \(kbytes\): (\d+)',times)[1])
            if axis=='go':
                value=json.loads(output)
                samples=value['times_ns']
            else:
                assert 'status=ok' in output,output
                samples=[int(n) for n in re.findall(r'sample_ns=(\d+)',output)]
                assert len(samples)==3,output
                value={}
            windows.append(dict(axis=axis,samples_ns=samples,max_rss_kib=rss,details=value))
        go=[w for w in windows if w['axis']=='go'];c=[w for w in windows if w['axis']=='c']
        gons=statistics.median([n for w in go for n in w['samples_ns']]);cns=statistics.median([n for w in c for n in w['samples_ns']])
        row=dict(seed=seed,budget=enabled,go_ns=gons,c_ns=cns,time_ratio=gons/cns,go_max_rss_kib=max(w['max_rss_kib'] for w in go),c_max_rss_kib=max(w['max_rss_kib'] for w in c),windows=windows)
        rows.append(row)
        (root/'ratios.json').write_text(json.dumps(rows,indent=2))
        print(seed,enabled,round(gons),round(cns),round(row['time_ratio'],2),flush=True)
for enabled in [False,True]:
    r=[r for r in rows if r['budget']==enabled]
    print('summary',enabled,json.dumps({k:statistics.median(v[k] for v in r) for k in ['go_ns','c_ns','time_ratio','go_max_rss_kib','c_max_rss_kib']}),flush=True)
