from pathlib import Path
import gzip,hashlib,json,re,statistics,tarfile
p=Path(__file__).resolve().parent
for line in (p/'SHA256SUMS').read_text().splitlines():
 digest,name=line.split('  ',1);assert hashlib.sha256((p/name).read_bytes()).hexdigest()==digest,name
index=json.loads(gzip.decompress((p/'contents.json.gz').read_bytes()))
with tarfile.open(p/'evidence.tar.gz','r:gz') as t:
 members=t.getmembers();assert len(members)==len(index);d={}
 for m,row in zip(members,index):
  assert m.isfile() and m.name==row['path'] and m.size==row['bytes'] and not m.name.startswith('/') and '..' not in Path(m.name).parts
  value=t.extractfile(m).read();assert hashlib.sha256(value).hexdigest()==row['sha256'];d[m.name]=value
s=json.loads((p/'summary.json').read_text());v=json.loads((p/'verification.json').read_text())
assert s['pairs']==60 and s['processes']==120 and s['decision'].startswith('HOLD') and not s['production_runtime_change'] and not s['graduation_claim'] and s['merges_held']
assert v['actual_primary500_constructed_cold'] and v['original_capture_skipped_on_primary'] and v['ordinary_actual_rows_equal']==4 and v['R4_steps']==288 and v['no_edit_allocs']==0
for series,pattern in [('go-primary','primary-{}.txt'),('go-r4','r4-{}-go.txt'),('yaml-r4','r4-{}-yaml.txt')]:
 orders={}
 for arm in ('before','after'):
  text=d['outputs/native-syntax-v5-perf-v1/'+pattern.format(arm)].decode();assert text.rstrip().endswith('# status: complete')
  assert list(map(int,re.findall(r'^# completed seed: (\d+);',text,re.M)))==list(range(1,21))
  orders[arm]=re.findall(r'^# workload order: (.*)',text,re.M);assert len(orders[arm])==20;by={}
  for line in text.splitlines():
   if not line.startswith('Benchmark'):continue
   name=line.split()[0];row={u:float(x) for x,u in re.findall(r'([0-9.]+) (ns/op|B/op|allocs/op|compact-routes/op|compact-fallbacks/op)',line)};by.setdefault(name,[]).append(row)
   if series!='go-primary':assert row['compact-fallbacks/op']==0 and row['compact-routes/op']==int(name.endswith('/compact'))
   elif name.endswith('/NoEdit'):assert row['B/op']==row['allocs/op']==0
  assert set(by)==set(s['results'][series])
  for name,rows in by.items():
   assert len(rows)==20
   for metric in ('ns/op','B/op','allocs/op'):assert statistics.median(row[metric] for row in rows)==s['results'][series][name][metric][arm]
 assert orders['before']==orders['after']
selections={
 'native-syntax-v5-reads-qualification-v2':(0,{'reads-all.log':42,'reads-race.log':9}),
 'native-syntax-v5-initial-qualification-v2':(1,{'core-release.log':16,'core-workcount.log':16,'core-shadow-clone.log':2}),
 'native-syntax-v5-initial-qualification-v4':(0,{'root-receipts.log':8,'accounting.log':1,'actual-entries.log':1}),
 'native-syntax-public-budget-v4':(0,{'public-budget.log':1}),
 'native-syntax-public-budget-v5':(0,{'public-budget.log':1,'core-race.log':16}),
 'native-syntax-v5-semantic-qualification-v1':(0,{'core-reference.log':55,'focused.log':36,'go-public-api.log':3,'new-api-budget.log':8,'constructed-cgo.log':3,'go-cgo.log':10,'yaml-cgo.log':8,**{f'invariants-{lang}-{mode}.log':1 for lang in ('go','yaml') for mode in (0,1)}}),
 'native-syntax-v5-public-race-v1':(0,{'public-race.log':9})}
for folder,(code,logs) in selections.items():
 assert json.loads(d['outputs/'+folder+'/finished.json'])['exit_code']==code
 for n in ('source-before.log','source-after.log','controls-before.log','controls-after.log'):assert d['outputs/'+folder+'/'+n]==b''
 for n,count in logs.items():
  text=d['outputs/'+folder+'/'+n].decode();assert len(re.findall(r'^--- PASS:',text,re.M))==count and not re.search(r'--- (FAIL|SKIP):|WARNING: DATA RACE|^FAIL\b',text,re.M)
rows=[json.loads(x.split('SYNTAX_ACTUAL_ENTRY ',1)[1]) for x in d['outputs/native-syntax-v5-initial-qualification-v4/actual-entries.log'].decode().splitlines() if 'SYNTAX_ACTUAL_ENTRY ' in x]
a,b=[x for x in rows if x['fixture']=='primary-500'];assert a['digest']==b['digest'] and a['board']==b['board'];q=b['publication'];assert q['constructed_cold'] and q['borrowed_cold'] and not q['original_cold_capture'] and not q['selected_store']
for route in ('default','candidate'):
 for lang in ('go','yaml'):
  n=f'{route}-{lang}.json';assert json.loads(d['outputs/native-syntax-v5-semantic-qualification-v1/'+n])==json.loads(d['baselines/'+n])
profile='outputs/native-syntax-v5-profile-v1/';assert json.loads(d[profile+'finished.json'])['exit_code']==0
for arm in ('baseline','candidate'):assert d[profile+arm+'-build.log']==b'' and d[profile+arm+'-profile.log'].decode().rstrip().endswith('PASS')
assert b'e redeclared' in d['outputs/native-syntax-v5-initial-qualification-v1/core-release.log']
assert b'unproved final read history' in d['outputs/native-syntax-v5-initial-qualification-v3/root-receipts.log']
print(json.dumps({'validated_members':len(index),'paired_processes':120,'decision':'HOLD','constructed_receipts_activated':True,'runtime_promoted':False,'graduation_claim':False,'merges_held':True}))
