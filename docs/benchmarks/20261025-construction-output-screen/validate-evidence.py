from pathlib import Path
import gzip,hashlib,json,re,statistics,tarfile
p=Path(__file__).resolve().parent
for line in (p/'SHA256SUMS').read_text().splitlines():
 digest,name=line.split('  ',1);assert hashlib.sha256((p/name).read_bytes()).hexdigest()==digest,name
index=json.loads(gzip.decompress((p/'contents.json.gz').read_bytes()))
with tarfile.open(p/'evidence.tar.gz','r:gz') as t:
 members=t.getmembers();assert len(members)==len(index)
 data={}
 for m,row in zip(members,index):
  assert m.isfile() and m.name==row['path'] and m.size==row['bytes'] and not m.name.startswith('/') and '..' not in Path(m.name).parts
  value=t.extractfile(m).read();assert hashlib.sha256(value).hexdigest()==row['sha256'];data[m.name]=value
s=json.loads((p/'summary.json').read_text());v=json.loads((p/'verification.json').read_text())
assert s['pairs']==60 and s['processes']==120 and s['decision'].startswith('HOLD') and not s['production_runtime_change'] and not s['graduation_claim'] and s['merges_held']
assert v['primary500_activated'] and v['ordinary_ledger_rows_equal']==4 and v['R4_edit_steps']==288 and v['no_edit_allocs']==0
assert v['table_lookup_proxy']==[41515,26010]
for series,pattern in [('go-primary','primary-{}.txt'),('go-r4','r4-{}-go.txt'),('yaml-r4','r4-{}-yaml.txt')]:
 orders={}
 for arm in ('before','after'):
  text=data['outputs/native-syntax-perf-v1/'+pattern.format(arm)].decode();assert text.rstrip().endswith('# status: complete')
  assert list(map(int,re.findall(r'^# completed seed: (\d+);',text,re.M)))==list(range(1,21))
  orders[arm]=re.findall(r'^# workload order: (.*)',text,re.M);assert len(orders[arm])==20
  by={}
  for line in text.splitlines():
   if not line.startswith('Benchmark'):continue
   name=line.split()[0];metrics={unit:float(value) for value,unit in re.findall(r'([0-9.]+) (ns/op|B/op|allocs/op|compact-routes/op|compact-fallbacks/op)',line)};by.setdefault(name,[]).append(metrics)
   if series!='go-primary':assert metrics['compact-fallbacks/op']==0 and metrics['compact-routes/op']==int(name.endswith('/compact'))
   elif name.endswith('/NoEdit'):assert metrics['B/op']==metrics['allocs/op']==0
  assert set(by)==set(s['results'][series])
  for name,rows in by.items():
   assert len(rows)==20
   for metric in ('ns/op','B/op','allocs/op'):assert statistics.median(row[metric] for row in rows)==s['results'][series][name][metric][arm]
 assert orders['before']==orders['after']
for route in ('default','candidate'):
 for lang in ('go','yaml'):
  name=f'{route}-{lang}.json';assert json.loads(data['outputs/native-syntax-focused-v1/'+name])==json.loads(data['baselines/'+name])
for variant in ('native-syntax-v4-qualification-v1','native-syntax-focused-v1','native-syntax-cgo-qualification-v2','native-syntax-coordinates-race-v1','native-syntax-api-qualification-v2','native-syntax-r4-audit-v2'):
 assert json.loads(data['outputs/'+variant+'/finished.json'])['exit_code']==0
 for f in ('source-before.log','source-after.log','controls-before.log','controls-after.log'):assert data['outputs/'+variant+'/'+f]==b''
selections={
 'native-syntax-v4-qualification-v1':{'core-release.log':8,'core-workcount.log':8,'core-shadow-clone.log':1,'root.log':2,'accounting.log':1,'actual-entries.log':1},
 'native-syntax-focused-v1':{'core-reference.log':55,'focused.log':36,'go-public-api.log':3},
 'native-syntax-cgo-qualification-v2':{'primary-cgo.log':1,'primary-edits-cgo.log':1,'go-cgo.log':10,'yaml-cgo.log':8},
 'native-syntax-coordinates-race-v1':{'coordinates-cgo.log':1,'core-race.log':8,'public-concurrent-race.log':1},
 'native-syntax-api-qualification-v2':{'api-query-workcount.log':7,'public-race.log':2},
 'native-syntax-r4-audit-v2':{'r4-decline-audit.log':1}}
for folder,logs in selections.items():
 for name,count in logs.items():
  text=data['outputs/'+folder+'/'+name].decode();assert len(re.findall(r'^--- PASS:',text,re.M))==count
  assert not re.search(r'--- (FAIL|SKIP):|WARNING: DATA RACE|^FAIL\b|panic:',text,re.M)
assert data['outputs/native-syntax-copy-probe-v1/test-exit.txt'].strip()==b'1'
copy=data['outputs/native-syntax-copy-probe-v1/copy-diagnostic.log'].decode();rows=[json.loads(line.split('COPY_RECEIPT_DIAGNOSTIC ',1)[1]) for line in copy.splitlines() if 'COPY_RECEIPT_DIAGNOSTIC ' in line];assert len(rows)==2
for row in rows:row.pop('mode');assert row['first_copy_root_word']==2 and row['second_copy_root_word']==0 and not row['second_copy_has_certified_reuse']
assert rows[0]==rows[1]
profiles='outputs/native-syntax-profile-r4-audit-v1/'
for arm in ('baseline','candidate'):assert data[profiles+arm+'-build.log']==b'' and data[profiles+arm+'-profile.log'].decode().rstrip().endswith('PASS')
assert 'import cycle' in data[profiles+'r4-decline-audit.log'].decode()
audit=data['outputs/native-syntax-r4-audit-v2/r4-decline-audit.log'].decode();assert 'extra replay-position scope' in audit and 'ambiguous or reused reduction' in audit
print(json.dumps({'validated_members':len(index),'paired_processes':120,'decision':'HOLD','runtime_promoted':False,'graduation_claim':False,'merges_held':True}))
