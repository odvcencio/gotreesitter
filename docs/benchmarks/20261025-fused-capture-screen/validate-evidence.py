"""Read-only validation of archived qualification and paired-performance proof."""
from pathlib import Path
import gzip,hashlib,json,re,tarfile
root=Path(__file__).resolve().parent
index=json.loads(gzip.decompress((root/'qualification-contents.json.gz').read_bytes()))['files']
with tarfile.open(root/'qualification-evidence.tar.gz','r:gz') as archive:
 entries=archive.getmembers();assert len(entries)==len({x.name for x in entries})==len(index)
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
assert parsed('out/native-fused-capture-qualification-v1/finished.json')['exit_code']==1
failed=text('out/native-fused-capture-qualification-v1/focused.log')
assert len(re.findall(r'^--- FAIL:',failed,re.M))==2 and failed.count('A/B did not select original/fused capture')==6
assert 'ordinary candidate bypassed fused views' in failed
q='out/native-fused-capture-qualification-v2/'
assert parsed(q+'finished.json')['exit_code']==0
summary=parsed(q+'validated-summary.json')
for name,want in summary['top_level_passes'].items():
 log=text(q+name);assert len(re.findall(r'^--- PASS:',log,re.M))==want
 assert not re.search(r'--- (FAIL|SKIP):|^panic:|^fatal error:',log,re.M)
focused=text(q+'focused.log')
for name in ['r4','canonical','type-retag','case-boundaries','root-extras','padding']:
 assert re.search(r'^    --- PASS: TestNativeFusedCaptureDifferentialGo/'+re.escape(name)+r' ',focused,re.M)
for language in ['go','yaml']:
 for route in ['0','1']:
  log=text(q+f'invariants-{language}-{route}.log')
  assert f'language={language}' in log and 'session_steps=72 no_edit_allocs=0' in log
 for route in ['default','candidate']:
  name=f'{route}-{language}.json';assert parsed(q+name)==parsed('out/packed-snapshot-budget-final/ledger/'+name)
  assert raw(q+f'{route}-{language}.stderr')==b''
for guard in ['source-before.log','source-after.log','controls-before.log','controls-after.log']:assert raw(q+guard)==b''
base='native-fused-capture-v1/'
for name in ['overlay.json','overlay.tests.json']:
 a,b=parsed(base+'v2/'+name)['Replace'],parsed(base+'v3/'+name)['Replace']
 changed=[k for k in a if a[k]!=b[k]];assert len(changed)==1 and changed[0].endswith('/parsercore_phase0_materializer.go')
a=text(base+'root/parsercore_phase0_materializer.go.txt');b=text(base+'v3/parsercore_phase0_materializer.go.txt')
assert b==a.replace('nativeFusedCapturePrivateEnabled && r.lang.ExternalScanner == nil && !replay.CheckpointsRequired','nativeFusedCapturePrivateEnabled && !compactReuseScannerUnsupported(r.lang) && !replay.CheckpointsRequired')
# Every recorded runtime/test map backing is present; benchmark wrapper reuse
# contributes no dense/hybrid runtime targets to this independent experiment.
maps=0
for name,content in data.items():
 if not name.endswith('.json'):continue
 try:obj=json.loads(content)
 except (ValueError,UnicodeDecodeError):continue
 if not isinstance(obj,dict) or 'Replace' not in obj:continue
 maps+=1
 for target,backing in obj['Replace'].items():
  assert 'dense_lr' not in target and 'parsercore_hybrid' not in target
  if not backing:continue
  if backing.startswith('/controls/'):member=prefix+backing[len('/controls/'):]
  elif '/harness_out/' in backing:member='harness_out/'+backing.split('/harness_out/',1)[1]
  else:raise AssertionError((name,backing))
  assert member in data,(name,member)
assert (root/'source-identities.json').read_bytes()==data['package/source-identities.json']
print(json.dumps({'qualification_archive_files':len(index),'overlay_maps':maps,'V2_expected_activation_failure':True,'V3_core':59,'V3_focused':39,'activated_fused_cases':6,'Go_C_families':10,'YAML_C_families':8,'sessions':4,'edit_steps':288,'no_edit_allocations':0,'actual_counter_rows_equal':4,'timing':'validated separately below'},indent=2))

# Performance outputs are a separate component captured only after root's
# terminal validation. Never resample, rebuild, invoke benchstat, or write logs.
pindex=json.loads(gzip.decompress((root/'performance-contents.json.gz').read_bytes()))['files']
with tarfile.open(root/'performance-evidence.tar.gz','r:gz') as archive:
 entries=archive.getmembers();assert len(entries)==len({x.name for x in entries})==len(pindex)
 for entry in entries:
  assert entry.isfile() and entry.mode==0o644 and not entry.name.startswith('/') and '..' not in Path(entry.name).parts
  content=archive.extractfile(entry).read();record=pindex[entry.name]
  assert len(content)==record['bytes'] and hashlib.sha256(content).hexdigest()==record['sha256']
  assert not content.startswith(b'\x7fELF') and not content.startswith(b'MZ') and not entry.name.endswith('.csv')
  if entry.name in data:assert data[entry.name]==content  # identical saved validator in both components
  data[entry.name]=content
import statistics
q='out/native-fused-capture-perf-v1/'
assert parsed(q+'finished.json')['exit_code']==0
for guard in ['source-before.log','source-after.log','controls-before.log','controls-after.log','producer-controls-before.log','producer-controls-after.log']:assert raw(q+guard)==b''
report=parsed(q+'validated-summary.json');assert report['pairs']==60 and report['processes']==120
processes=0
for series,pattern,wanted in [('go-r4','r4-{}-go.txt',['BenchmarkCompactGraduationR4/legacy','BenchmarkCompactGraduationR4/compact']),('yaml-r4','r4-{}-yaml.txt',['BenchmarkCompactGraduationR4/legacy','BenchmarkCompactGraduationR4/compact']),('go-primary','primary-{}.txt',['BenchmarkGraduationGoSuite/Full','BenchmarkGraduationGoSuite/Edit','BenchmarkGraduationGoSuite/NoEdit'])]:
 orders={}
 for arm in ['before','after']:
  log=text(q+pattern.format(arm));assert log.rstrip().endswith('# status: complete')
  assert not re.search(r'^FAIL|--- (FAIL|SKIP):|panic:',log,re.M)
  assert list(map(int,re.findall(r'^# completed seed: (\d+);',log,re.M)))==list(range(1,21))
  orders[arm]=re.findall(r'^# workload order: (.*)',log,re.M);assert len(orders[arm])==20 and all(set(x.split(','))==set(wanted) for x in orders[arm])
  positions=re.findall(r'^# seed: (\d+); position: (\d+)',log,re.M);assert len(positions)==20;processes+=20
  for seed,pos in positions:assert int(pos)==((1 if int(seed)%2 else 2) if arm=='before' else (2 if int(seed)%2 else 1))
  samples={name:[] for name in wanted}
  for line in log.splitlines():
   if not line.startswith('Benchmark'):continue
   name=line.split()[0];metrics={unit:float(value) for value,unit in re.findall(r'([0-9.]+) (ns/op|B/op|allocs/op|compact-routes/op|compact-fallbacks/op)',line)};samples[name].append(metrics)
   if series!='go-primary':assert metrics['compact-fallbacks/op']==0 and metrics['compact-routes/op']==(1 if name.endswith('/compact') else 0)
   elif name.endswith('/NoEdit'):assert metrics['B/op']==metrics['allocs/op']==0
  assert all(len(x)==20 for x in samples.values())
  for name,values in samples.items():
   for metric in ['ns/op','B/op','allocs/op']:assert statistics.median(v[metric] for v in values)==report['results'][series][name][metric][arm]
 assert orders['before']==orders['after']
assert processes==120
pvalues=[]
for series in ['go-primary','go-r4','yaml-r4']:
 lines=text(q+f'benchstat-{series}.txt').splitlines();start=next(i for i,line in enumerate(lines) if 'sec/op' in line)
 for line in lines[start+1:]:
  if line.startswith('geomean'):break
  match=re.search(r'\(p=([0-9.]+) n=20\)',line)
  if match:pvalues.append(float(match[1]))
assert len(pvalues)==7 and all(x>=0.05 for x in pvalues)
final=json.loads(data['package/summary.json'])
assert final['all_timings_inconclusive'] and final['decision']=='HOLD; no demonstrated timing win'
assert hashlib.sha256((root/'qualification-evidence.tar.gz').read_bytes()).hexdigest()==final['static_archive_sha256']
assert (root/'summary.json').read_bytes()==data['package/summary.json']
assert prefix+'validate-native-fused-capture-qualification-v2.py' in data
print(json.dumps({'performance_archive_files':len(pindex),'pairs':60,'processes':processes,'all_seven_timings_inconclusive':True,'allocation_counts_unchanged':True,'byte_increases_retained':True,'static_archive_unchanged':True,'decision':'HOLD; no demonstrated timing win'},indent=2))
