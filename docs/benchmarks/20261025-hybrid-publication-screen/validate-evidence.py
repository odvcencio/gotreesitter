"""Offline checks only: archive identities and retained raw proof cardinality."""
from pathlib import Path
import copy, gzip, hashlib, json, re, statistics, tarfile
root=Path(__file__).resolve().parent
index=json.loads(gzip.decompress((root/'contents.json.gz').read_bytes()))['files']
with tarfile.open(root/'evidence.tar.gz','r:gz') as archive:
 entries=archive.getmembers()
 assert len(entries)==len({x.name for x in entries})==len(index)
 data={}
 for entry in entries:
  assert entry.isfile() and entry.mode==0o644 and not entry.name.startswith('/') and '..' not in Path(entry.name).parts
  content=archive.extractfile(entry).read();record=index[entry.name]
  assert len(content)==record['bytes'] and hashlib.sha256(content).hexdigest()==record['sha256'],entry.name
  assert not content.startswith(b'\x7fELF') and not content.startswith(b'MZ') and not entry.name.endswith('.csv')
  data[entry.name]=content
prefix='harness_out/docker/go-ci-blockers-20261025/'
def raw(name):return data[prefix+name]
def text(name):return raw(name).decode()
def parsed(name):return json.loads(raw(name))
summary=json.loads(data['package/summary.json']);reports=summary['latest_perf'];assert reports['pairs']==60 and reports['processes']==120
processes=0
for series,pattern,wanted in [('go-r4','r4-{}-go.txt',['BenchmarkCompactGraduationR4/legacy','BenchmarkCompactGraduationR4/compact']),('yaml-r4','r4-{}-yaml.txt',['BenchmarkCompactGraduationR4/legacy','BenchmarkCompactGraduationR4/compact']),('go-primary','primary-{}.txt',['BenchmarkGraduationGoSuite/Full','BenchmarkGraduationGoSuite/Edit','BenchmarkGraduationGoSuite/NoEdit'])]:
 orders={}
 for arm in ['before','after']:
  log=text('out/dense-hybrid-budget-perf-v1/'+pattern.format(arm));assert log.rstrip().endswith('# status: complete')
  assert list(map(int,re.findall(r'^# completed seed: (\d+);',log,re.M)))==list(range(1,21))
  orders[arm]=re.findall(r'^# workload order: (.*)',log,re.M);assert len(orders[arm])==20 and all(set(x.split(','))==set(wanted) for x in orders[arm])
  positions=re.findall(r'^# seed: (\d+); position: (\d+)',log,re.M);assert len(positions)==20;processes+=len(positions)
  for seed,pos in positions:assert int(pos)==((1 if int(seed)%2 else 2) if arm=='before' else (2 if int(seed)%2 else 1))
  samples={k:[] for k in wanted}
  for line in log.splitlines():
   if not line.startswith('Benchmark'):continue
   name=line.split()[0];metrics={unit:float(value) for value,unit in re.findall(r'([0-9.]+) (ns/op|B/op|allocs/op|compact-routes/op|compact-fallbacks/op)',line)};samples[name].append(metrics)
   if series!='go-primary':assert metrics['compact-fallbacks/op']==0 and metrics['compact-routes/op']==(1 if name.endswith('/compact') else 0)
   if name.endswith('/NoEdit'):assert metrics['B/op']==metrics['allocs/op']==0
  assert all(len(values)==20 for values in samples.values())
  for name,values in samples.items():
   for metric in ['ns/op','B/op','allocs/op']:assert statistics.median(v[metric] for v in values)==reports['results'][series][name][metric][arm]
 assert orders['before']==orders['after']
assert processes==120
q='out/dense-hybrid-budget-qualification-v1/'
for name,want in [('core.log',18),('region-boundaries.log',10),('lifetime.log',4),('memory.log',15),('go-cgo.log',10),('yaml-cgo.log',8)]:
 log=text(q+name);assert len(re.findall(r'^--- PASS:',log,re.M))==want
 assert not re.search(r'--- (FAIL|SKIP):|^panic:|^fatal error:',log,re.M)
for language in ['go','yaml']:
 for route in ['0','1']:
  log=text(q+f'invariants-{language}-{route}.log');assert f'language={language}' in log and 'session_steps=72 no_edit_allocs=0' in log
  assert not re.search(r'--- (FAIL|SKIP):',log)
rows=parsed(q+'entry-rows.json');assert len(rows['off'])==len(rows['on'])==8
for aa,bb in zip(rows['off'],rows['on']):
 a,b=copy.deepcopy(aa),copy.deepcopy(bb)
 assert all(a[k]==b[k] for k in ['fixture','entry','candidate'])
 oa,ob=a.pop('dense_private_observations'),b.pop('dense_private_observations')
 if a['candidate']:
  assert len(oa)==len(ob)==1;x,y=oa[0],ob[0];x.pop('Dense');y.pop('Dense')
  for dim in ['CoreNodes','CoreLinks']:assert y.pop(dim)<x.pop(dim)
  assert x.pop('IdleDenseBytes')==0 and 0<y.pop('IdleDenseBytes')<=16384
  assert y['CoreWork'].pop('GraphLinkAdditionsProxy')<x['CoreWork'].pop('GraphLinkAdditionsProxy')
  assert x==y
  for dim in ['last_idle_core_footprint_bytes','last_idle_core_nodes']:assert b['publication'].pop(dim)<a['publication'].pop(dim)
  assert b['publication']['last_idle_core_work'].pop('GraphLinkAdditionsProxy')<a['publication']['last_idle_core_work'].pop('GraphLinkAdditionsProxy')
 else:assert oa==ob
 assert a==b
qual=parsed(q+'differential-summary.json');assert (qual['budget_diagnostic_checks'],qual['budget_diagnostic_regions'],qual['budget_diagnostic_fallbacks'])==(561,56,0)
for language in ['go','yaml']:
 for route in ['default','candidate']:
  name=f'{route}-{language}.json';assert parsed('out/dense-hybrid-budget-counters-v1/'+name)==parsed('out/packed-snapshot-budget-final/ledger/'+name)
supp=[json.loads(x.split('YAML_DENSE_ATTRIBUTION ',1)[1]) for x in text('out/dense-hybrid-yaml-attribution-v1/yaml-attribution.log').splitlines() if 'YAML_DENSE_ATTRIBUTION ' in x];assert len(supp)==2
for key in ['digest','payload_scanner','dependencies','scheduler_work']:assert supp[0][key]==supp[1][key]
a,b=supp[0]['core_work'].copy(),supp[1]['core_work'].copy();assert (a.pop('GraphLinkAdditionsProxy'),b.pop('GraphLinkAdditionsProxy'))==(304,168) and a==b
# Every retained overlay backing is present with its original bytes.
maps=0
for name,content in data.items():
 if not name.endswith('.json'):continue
 try:obj=json.loads(content)
 except (ValueError,UnicodeDecodeError):continue
 if not isinstance(obj,dict) or 'Replace' not in obj:continue
 maps+=1
 for backing in obj['Replace'].values():
  if not backing:continue
  if backing.startswith('/controls/'):member=prefix+backing[len('/controls/'):]
  elif '/harness_out/' in backing:member='harness_out/'+backing.split('/harness_out/',1)[1]
  else:raise AssertionError((name,backing))
  assert member in data,(name,member)
for variant in ['dense-hybrid-budget-perf-v1','dense-hybrid-budget-qualification-v1','dense-hybrid-budget-counters-v1','dense-hybrid-perf-v1','dense-hybrid-qualification-v1','dense-hybrid-profile-v1','yaml-current-profile-v1','dense-hybrid-yaml-attribution-v1']:
 assert parsed('out/'+variant+'/finished.json')['exit_code']==0
 for guard in ['source-before.log','source-after.log','controls-before.log','controls-after.log']:
  member=prefix+'out/'+variant+'/'+guard
  if member in data:assert data[member]==b''
assert (root/'summary.json').read_bytes()==data['package/summary.json']
print(json.dumps({'archive_files':len(index),'overlay_maps':maps,'pairs':60,'processes':processes,'entry_pairs':8,'actual_counter_rows_equal':4,'budget_checks':561,'regions':56,'supplemental_YAML':'exact untimed receipts;304→168graph links','decision':'HOLD; YAML regression retained'},indent=2))
