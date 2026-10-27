#!/usr/bin/env python3
"""Validate archived V28 qualification evidence without executing archived code."""
import gzip, hashlib, json, re, statistics, tarfile
from pathlib import Path, PurePosixPath

def sha(b): return hashlib.sha256(b).hexdigest()
d=Path(__file__).resolve().parent
files={}
with tarfile.open(d/'evidence.tar.gz') as tar:
 for member in tar:
  name=PurePosixPath(member.name)
  assert member.isfile() and not name.is_absolute() and '..' not in name.parts and member.name not in files
  files[member.name]=tar.extractfile(member).read()
index=json.loads(gzip.decompress((d/'contents.json.gz').read_bytes()))
assert files.keys()==index.keys()
for name,data in files.items(): assert index[name]=={'bytes':len(data),'sha256':sha(data)}
complete=json.loads(files['combined-correctness/v28/completed.json'])
assert complete['exit_code']==0 and complete['combined_inputs_sha256']=='835ba71dedcec7b284547d78acf40da944c08588ebbc2f82ba668a3e47d10d82'
assert sha(files['combined-v28/inputs.sha256'])==complete['combined_inputs_sha256']
assert (complete['fleet_languages'],complete['fleet_route_runs'],complete['c_fixture_checks'],complete['oracle_matching_cases'],complete['memory_test_count'])==(206,412,1236,127,27)
for name,digest in complete['files_sha256'].items(): assert sha(files['combined-correctness/v28/'+name])==digest,name
counter=json.loads(files['out-combined-v28-counters/summary.json'])
assert counter['languages']==206 and counter['routes_per_language']==2 and counter['ledger_exports_byte_identical'] and counter['work_and_reuse_direction_passed']
bench=files['out-combined-v28-perf/benchstat.txt'].decode()
assert '-23.67%' in bench and '-92.53%' in bench and '+31.47%' in bench
for folder in ('out-combined-v28-perf','out-combined-v28-yaml'):
 arms=[]
 for arm in ('before','after'):
  raw=files[folder+'/'+arm+'.txt'].decode(); rows=re.findall(r'# seed: (\d+); position: (\d+)',raw)
  assert len(rows)==20 and raw.count('\nPASS\n')==20
  arms.append({int(s):int(p) for s,p in rows})
 assert arms[0].keys()==arms[1].keys() and all(arms[0][s]+arms[1][s]==3 for s in arms[0])
for name in files:
 if name.startswith('container-states/'):
  state=json.loads(files[name]); assert state['state']['ExitCode']==0 and not state['state']['OOMKilled']
gauges=[c for c in counter['changes'] if c['counter'].startswith('selected.')]
assert len(gauges)==48
for name in ('selected.ArenaBaselineBytes','selected.ArenaBytesAllocated','selected.ScratchBaselineBytes','selected.ScratchBytesAllocated'):
 assert sum(c['counter']==name for c in gauges)==12
warm=[]
for arm in ('before','after'):
 raw=files['out-combined-v28-warm-1m/randomized-'+arm+'.txt'].decode()
 rows=re.findall(r'# seed: (\d+); position: (\d+)',raw)
 assert len(rows)==20 and raw.count('\nPASS\n')==20
 order={int(s):int(p) for s,p in rows}; assert set(order)==set(range(154001,154021))
 values=re.findall(r'^BenchmarkFreshMemory/go/1024KiB\s+2\s+.*?([0-9]+) B/op\s+([0-9]+) allocs/op',raw,re.M)
 assert len(values)==20
 warm.append((order,[(int(b),int(a)) for b,a in values]))
assert all(warm[0][0][s]+warm[1][0][s]==3 for s in warm[0][0])
assert statistics.median(a for b,a in warm[0][1])==96 and statistics.median(a for b,a in warm[1][1])==99
assert statistics.median(a for b,a in warm[1][1])/statistics.median(a for b,a in warm[0][1])<=1.10
assert statistics.median(b for b,a in warm[1][1])<statistics.median(b for b,a in warm[0][1])
for when in ('before','after'):
 for stem in ('source','controls','correctness-source'):
  assert files[f'out-combined-v28-warm-1m/{stem}-{when}.log']==b''
assert files['out-combined-v28-warm-1m/baseline-fixture.log']==files['out-combined-v28-warm-1m/candidate-fixture.log']
assert b'76c70b62127085d3c791ba98a24cba9f22bd343e40afb43522bbb5041cede524' in files['out-combined-v28-warm-1m/baseline-fixture.log']
rss={}
for arm in ('baseline','candidate'):
 vals=[]
 for seed in range(154001,154021):
  raw=files[f'out-combined-v28-warm-1m/{arm}-{seed}.time'].decode()
  assert 'Exit status: 0' in raw
  vals.append(int(re.search(r'Maximum resident set size \(kbytes\): (\d+)',raw)[1])*1024)
 assert all(v>419436400 for v in vals)
 rss[arm]=vals
print(json.dumps({'validated':True,'status':'relative checks passed; absolute RSS held','members':len(files),'fleet_routes':412,'c_checks':1236,'go_paired_seeds':20,'yaml_paired_seeds':20,'rss_samples_above_ceiling_per_arm':20},indent=2))
