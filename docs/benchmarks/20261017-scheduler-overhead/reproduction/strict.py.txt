import collections,json,os,pathlib,re,subprocess,sys

p=pathlib.Path('/evidence/fleet')
role=sys.argv[1]
assert role in ['baseline','candidate']
rows=[r for r in json.loads(pathlib.Path('/workspace/internal/benchfixtures/real_corpus.json').read_text())['entries'] if r['role']=='sample' and r.get('committed_path')]
assert len(rows)==204
results=[]
for row in rows:
    lang=row['language']
    env=dict(os.environ,GTS_ADMISSION_REAL_CORPUS_LANGS=lang)
    run=subprocess.run([str(p/f'{role}-cgo.test'),'-test.run','^TestCompactR4StructuralLockedC$','-test.v','-test.count=1','-test.timeout=2m'],cwd='/workspace/cgo_harness',env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True,timeout=130)
    (p/f'{role}-strict-{lang}.log').write_text(run.stdout)
    assert '--- SKIP' not in run.stdout,lang
    route=re.search(r'compact=(\d+) fallback=(\d+) reason=(.*)',run.stdout)
    assert route,lang
    result=dict(language=lang,status='PASS' if run.returncode==0 and '--- PASS:' in run.stdout else 'FAIL',compact=int(route[1]),fallback=int(route[2]),reason=route[3],first_difference=next((line.strip() for line in run.stdout.splitlines() if 'go=' in line),''))
    assert run.returncode in [0,1],(lang,run.returncode)
    if result['compact']==1:assert result['status']=='PASS',(lang,result)
    results.append(result)
    print(role,lang,result['status'],result['compact'],result['fallback'],flush=True)
(p/f'{role}-strict-census.json').write_text(json.dumps(results,indent=2)+'\n')
print('TOTAL',dict(collections.Counter(r['status'] for r in results)),flush=True)
print('ACCEPTED',dict(collections.Counter(r['status'] for r in results if r['compact']==1)),flush=True)
if role=='candidate':
    baseline=json.loads((p/'baseline-strict-census.json').read_text())
    assert sorted(baseline,key=lambda r:r['language'])==sorted(results,key=lambda r:r['language']),'strict census changed; no exception'
    print('All 204 strict rows exactly match fresh native baseline',flush=True)
