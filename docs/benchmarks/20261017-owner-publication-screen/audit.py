from pathlib import Path
from statistics import median
import re,json,hashlib,subprocess
p=Path(__file__).parent
assert (p/'perf-complete.txt').read_text().strip()=='complete'
assert (p/'check-complete.txt').read_text().strip()=='complete'
inputs=json.loads((p/'input-pins.json').read_text())
assert all(hashlib.sha256((p/n).read_bytes()).hexdigest()==h for n,h in inputs.items())
frozen=p.parent/'20261017-packed-single-actions'/'baseline'
base=json.loads((p/'source-pins.json').read_text())
assert all(hashlib.sha256((frozen/n).read_bytes()).hexdigest()==h for n,h in base['files'].items())
root=Path('/home/draco/work/gotreesitter')
runner='scripts/run_randomized_benchmarks.sh'
assert (root/runner).read_bytes()==subprocess.check_output(['git','show',base['head']+':'+runner],cwd=root)
out={'baseline':base['head'],'status':'complete focused correctness and paired timing; full fleet pending','protocol':{'Go':'go1.25.14','GOMAXPROCS':1,'cpuset':6,'memory':'6g','pairs':20,'benchtime':'750ms','count_per_process':1,'benchmem':True,'shuffle_seeds':{'go':'29601..29620','yaml':'29701..29720'}},'source_pins_verified':{'baseline':len(base['files']),'candidate_and_overlay':len(inputs)},'benchmarks':{},'ledger':{},'limits':['Go/YAML timing only; no all-grammar timing claim','no independent review, main merge or graduation claim yet']}
for family,start,count in [('go',29601,8),('yaml',29701,3)]:
    data={}
    for role in ['base','head']:
        s=(p/f'{family}-{role}.txt').read_text()
        assert s.rfind('# status: complete')>s.rfind('# status: incomplete')
        seeds=re.findall(r'^# seed: (\d+); position: (\d+)',s,re.M)
        assert sorted(int(a) for a,b in seeds)==list(range(start,start+20))
        expected_start=1 if role=='base' else 2
        for seed,pos in seeds:
            expected=expected_start if (int(seed)-start)%2==0 else 3-expected_start
            assert int(pos)==expected,(family,role,seed,pos,expected)
        rows={}
        for line in s.splitlines():
            if not line.startswith('Benchmark'):continue
            a=line.split();metrics={a[i+1]:float(a[i]) for i in range(2,len(a)-1,2)}
            assert all(k in metrics for k in ['ns/op','B/op','allocs/op'])
            rows.setdefault(a[0],[]).append(metrics)
        assert len(rows)==count and all(len(x)==20 for x in rows.values())
        data[role]=rows
    for name,b in data['base'].items():
        h=data['head'][name]
        for k in set(b[0])-{'ns/op','B/op','allocs/op','MB/s'}:
            assert {x[k] for x in b}=={x[k] for x in h},(name,k,'work/route mismatch')
        before={k:median(x[k] for x in b) for k in b[0]}
        after={k:median(x[k] for x in h) for k in h[0]}
        for k in ['B/op','allocs/op']:
            assert after[k]<=before[k]*1.10,(name,k,'memory ratchet')
        assert after['ns/op']<=before['ns/op']*1.10,(name,'time ratchet')
        out['benchmarks'][family+'/'+name]={'before':before,'after':after,'time_percent':100*(after['ns/op']/before['ns/op']-1)}
for kind,n in [('ledger',412),('owned-ledger',2)]:
    a=json.loads((p/f'base-{kind}.json').read_text());b=json.loads((p/f'head-{kind}.json').read_text())
    assert a==b and len(b['rows'])==n
    out['ledger'][kind]={'rows':n,'exact':True,'sha256':hashlib.sha256((p/f'base-{kind}.json').read_bytes()).hexdigest()}
(p/'audit.json').write_text(json.dumps(out,indent=2)+'\n')
for name,row in out['benchmarks'].items():print(name,round(row['time_percent'],3),'%',row['before']['B/op'],'->',row['after']['B/op'],'B;',row['before']['allocs/op'],'->',row['after']['allocs/op'],'objects')
print('440 timed rows, alternating orders, source/work/reuse/route pins and median hard gates verified')
