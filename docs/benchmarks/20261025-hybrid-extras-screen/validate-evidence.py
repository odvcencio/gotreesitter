"""Offline archive, correctness and complete paired timing validation."""
from pathlib import Path
import copy,gzip,hashlib,json,re,statistics,tarfile
root=Path(__file__).resolve().parent
index=json.loads(gzip.decompress((root/'contents.json.gz').read_bytes()))['files']
with tarfile.open(root/'evidence.tar.gz','r:gz')as archive:
 members=archive.getmembers();assert len(members)==len({m.name for m in members})==len(index)
 assert all(m.isfile()and m.mode==0o644 and not m.name.startswith('/')and '..'not in Path(m.name).parts for m in members)
 data={m.name:archive.extractfile(m).read()for m in members}
for n,b in data.items():
 assert len(b)==index[n]['bytes']and hashlib.sha256(b).hexdigest()==index[n]['sha256'],n
 assert not b.startswith((b'\x7fELF',b'MZ'))and not n.endswith('.csv'),n
prefix='harness_out/docker/go-ci-blockers-20261025/'
def raw(n):return data[prefix+n]
def text(n):return raw(n).decode()
def obj(n):return json.loads(raw(n))
q='out/dense-hybrid-extras-qualification-v2/'
assert obj('out/dense-hybrid-extras-qualification-v1/finished.json')['exit_code']==1
v1=text('out/dense-hybrid-extras-qualification-v1/core.log')
assert re.findall(r'^--- FAIL: ([^ ]+)',v1,re.M)==['TestDenseHybridGraphExtraDeclinesBeforeGoto']
assert obj(q+'finished.json')['exit_code']==0 and raw(q+'completed.txt')
counts={}
for n,wanted in [('extras-core.log',5),('core.log',24),('region-boundaries.log',10),('lifetime.log',4),('memory.log',15),('go-cgo.log',10),('yaml-cgo.log',8)]:
 log=text(q+n);assert not re.search(r'--- (FAIL|SKIP):|^panic:|^fatal error:',log,re.M),n
 counts[n]=len(re.findall(r'^--- PASS:',log,re.M));assert counts[n]==wanted,(n,counts[n])
assert 'frame_bytes=96 dense_header_bytes=1928 path_payload_bytes=32 trailing_window_bytes=1024 empty_retained_bytes=1928'in text(q+'extras-core.log')
for n in ['source-before.log','source-after.log','controls-before.log','controls-after.log']:assert raw(q+n)==b''
for language in ['go','yaml']:
 for route in ['0','1']:
  log=text(q+f'invariants-{language}-{route}.log')
  assert f'language={language}'in log and 'session_steps=72 no_edit_allocs=0'in log
  assert len(re.findall(r'^--- PASS:',log,re.M))==1 and not re.search(r'--- (FAIL|SKIP):',log)
assert 'DENSE_PROMOTION_BUDGET_DIAGNOSTICS {Regions:48 Checks:481 Fallbacks:0 Refreshes:0}'in text(q+'lifetime.log')
assertion='if ok || d.lastRefusalCode != HybridRefusalExtra || counting.calls != 0 || c.SubtreeCount() != before || c.Work() != work || d.anchor != anchor {'
assert assertion in text('dense-hybrid-extras-v1/hybrid_graph_pop_test.go.txt')
assert assertion in text('dense-hybrid-extras-v1/v2/hybrid_graph_pop_test.go.txt')
assert 'c.popPaths(anchor.Node, 1)'in text('dense-hybrid-extras-v1/v2/hybrid_graph_pop_test.go.txt')
assert '--- PASS: TestDenseHybridGraphExtraWithStructuralPredecessorMatchesGeneric'in text(q+'core.log')
rows=obj(q+'entry-rows.json');assert len(rows['off'])==len(rows['on'])==8
for aa,bb in zip(rows['off'],rows['on']):
 x,y=copy.deepcopy(aa),copy.deepcopy(bb);assert all(x[k]==y[k]for k in ['fixture','entry','candidate'])
 oa,ob=x.pop('dense_private_observations'),y.pop('dense_private_observations')
 if x['candidate']:
  assert len(oa)==len(ob)==1;u,v=oa[0],ob[0];u.pop('Dense');v.pop('Dense')
  for k in ['CoreNodes','CoreLinks']:assert v.pop(k)<u.pop(k)
  assert u.pop('IdleDenseBytes')==0 and 0<v.pop('IdleDenseBytes')<=16384
  assert v['CoreWork'].pop('GraphLinkAdditionsProxy')<u['CoreWork'].pop('GraphLinkAdditionsProxy');assert u==v
  for k in ['last_idle_core_footprint_bytes','last_idle_core_nodes']:assert y['publication'].pop(k)<x['publication'].pop(k)
  assert y['publication']['last_idle_core_work'].pop('GraphLinkAdditionsProxy')<x['publication']['last_idle_core_work'].pop('GraphLinkAdditionsProxy')
 else:assert oa==ob
 assert x==y
links={}
for language,expected in [('go',(1381,577)),('yaml',(304,117))]:
 rows=[json.loads(line.split('DENSE_EXTRAS_ORDINARY ',1)[1])for line in text(q+f'extras-ordinary-{language}.log').splitlines()if 'DENSE_EXTRAS_ORDINARY 'in line]
 assert len(rows)==2 and [r['enabled']for r in rows]==[False,True]
 x,y=rows
 for k in ['digest','payload_scanner','dependencies','scheduler_work','source_bytes','source_sha256','no_edit_allocations']:assert x[k]==y[k]
 assert x['no_edit_allocations']==0
 u,v=x['core_work'],y['core_work'];links[language]=(u.pop('GraphLinkAdditionsProxy'),v.pop('GraphLinkAdditionsProxy'));assert links[language]==expected and u==v
 assert y['dense']['HybridRefusals'][3]==0
c='out/dense-hybrid-extras-counters-v2/'
assert obj(c+'finished.json')['exit_code']==0
for n in ['source-before.log','source-after.log','controls-before.log','controls-after.log']:assert raw(c+n)==b''
for language in ['go','yaml']:
 for route in ['default','candidate']:
  n=f'{route}-{language}.json';assert obj(c+n)==obj('out/packed-snapshot-budget-final/ledger/'+n)
  assert raw(c+f'{route}-{language}.stderr')==b''
# Every retained overlay has all of its exact backing files in this package.
maps=0
for n,b in data.items():
 if not n.endswith('.json'):continue
 try:m=json.loads(b)
 except(ValueError,UnicodeDecodeError):continue
 if not isinstance(m,dict)or not isinstance(m.get('Replace'),dict):continue
 maps+=1
 for backing in m['Replace'].values():
  if not backing:continue
  if backing.startswith('/controls/'):member=prefix+backing.removeprefix('/controls/')
  elif '/harness_out/'in backing:member='harness_out/'+backing.split('/harness_out/',1)[1]
  else:raise AssertionError((n,backing))
  assert member in data,(n,member)
perf='out/dense-hybrid-extras-perf-v1/'
assert obj(perf+'finished.json')['exit_code']==0 and raw(perf+'completed.txt')
for n in ['source-before.log','source-after.log','controls-before.log','controls-after.log']:assert raw(perf+n)==b''
summary=json.loads(data['package/summary.json']);report=summary['latest_perf'];assert report['pairs']==60 and report['processes']==120
assert summary['decision'].startswith('HOLD')and not summary['private_prototype_promoted']and not summary['graduation_claim']and summary['merges_held']
ratio_report=json.loads(data['package/route-ratios.json'])
processes=0
for series,pattern,wanted in [('go-r4','r4-{}-go.txt',['BenchmarkCompactGraduationR4/legacy','BenchmarkCompactGraduationR4/compact']),('yaml-r4','r4-{}-yaml.txt',['BenchmarkCompactGraduationR4/legacy','BenchmarkCompactGraduationR4/compact']),('go-primary','primary-{}.txt',['BenchmarkGraduationGoSuite/Full','BenchmarkGraduationGoSuite/Edit','BenchmarkGraduationGoSuite/NoEdit'])]:
 orders={}
 for arm in ['before','after']:
  log=text(perf+pattern.format(arm));assert log.rstrip().endswith('# status: complete')
  assert list(map(int,re.findall(r'^# completed seed: (\d+);',log,re.M)))==list(range(1,21))
  orders[arm]=re.findall(r'^# workload order: (.*)',log,re.M);assert len(orders[arm])==20 and all(set(x.split(','))==set(wanted)for x in orders[arm])
  positions=re.findall(r'^# seed: (\d+); position: (\d+)',log,re.M);assert len(positions)==20;processes+=20
  for seed,pos in positions:assert int(pos)==((1 if int(seed)%2 else 2)if arm=='before'else(2 if int(seed)%2 else 1))
  samples={k:[]for k in wanted};byseed={};seed=None
  for line in log.splitlines():
   m=re.match(r'# seed: (\d+); position:',line)
   if m:seed=int(m[1]);byseed[seed]={}
   if not line.startswith('Benchmark'):continue
   name=line.split()[0];metrics={unit:float(value)for value,unit in re.findall(r'([0-9.]+) (ns/op|B/op|allocs/op|compact-routes/op|compact-fallbacks/op)',line)};samples[name].append(metrics);byseed[seed][name]=metrics['ns/op']
   if series!='go-primary':assert metrics['compact-fallbacks/op']==0 and metrics['compact-routes/op']==(1 if name.endswith('/compact')else 0)
   if name.endswith('/NoEdit'):assert metrics['B/op']==metrics['allocs/op']==0
  assert all(len(v)==20 for v in samples.values())
  for name,values in samples.items():
   for metric in ['ns/op','B/op','allocs/op']:assert statistics.median(v[metric]for v in values)==report['results'][series][name][metric][arm]
  if series!='go-primary':
   language=series.split('-')[0];r=ratio_report['languages'][language][arm];pairs=r['pairs'];assert [x['seed']for x in pairs]==list(range(1,21))
   for x in pairs:
    assert x['legacy_ns']==byseed[x['seed']]['BenchmarkCompactGraduationR4/legacy']and x['compact_ns']==byseed[x['seed']]['BenchmarkCompactGraduationR4/compact']
    assert x['ratio']==x['compact_ns']/x['legacy_ns']
   assert statistics.median(x['ratio']for x in pairs)==r['median_of_paired_ratios']
   assert statistics.median(x['compact_ns']for x in pairs)/statistics.median(x['legacy_ns']for x in pairs)==r['ratio_of_medians']
 assert orders['before']==orders['after']
assert processes==120
for name in ['summary.json','source-identities.json','route-ratios.json']:assert(root/name).read_bytes()==data['package/'+name]
assert summary['timing_pvalues']=={'go-primary':{'Edit':.495,'NoEdit':.904,'Full':.211},'go-r4':{'compact':.904,'legacy':.738},'yaml-r4':{'compact':.001,'legacy':.779}}
assert '+11.19% (p=0.001 n=20)'in text(perf+'benchstat-yaml-r4.txt')
print(json.dumps({'archive_files_authenticated':len(data),'overlay_maps_complete':maps,'core_top_tests':24,'extras_top_families_rerun':5,'original_failure_preserved':True,'original_negative_assertion_unchanged':True,'ordinary_entry_pairs':8,'invariant_sessions':4,'edit_steps':288,'actual_ledger_rows_equal':4,'graph_links':links,'budget_regions':48,'budget_checks':481,'pairs':60,'processes':processes,'route_fallbacks':0,'decision':'HOLD; significant YAML regression, other timing inconclusive','production_promotion':False,'graduation':False,'merges_held':True},indent=2))
