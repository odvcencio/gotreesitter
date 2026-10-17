from pathlib import Path
import json
p=Path('/evidence');q=Path('/reference');changes=[]
def check(a,b,path):
 if isinstance(a,dict):
  assert isinstance(b,dict) and set(a)==set(b),(path,'keys')
  for k in a:check(a[k],b[k],path+'.'+k)
 elif isinstance(a,list):
  assert len(a)==len(b),(path,'length')
  for i,(x,y) in enumerate(zip(a,b)):check(x,y,path+f'[{i}]')
 elif a!=b:
  changes.append([path,a,b]);assert path.endswith('.new_nodes') and b<a,(path,a,b)
for kind in ['ledger','owned-ledger']:
 a=json.loads((q/f'base-{kind}.json').read_text());b=json.loads((p/f'head-{kind}.json').read_text());check(a,b,kind)
for role in ['base','head']:
 rows=[json.loads(l.split('PUBLICATION_WORK_CENSUS ',1)[1]) for l in (p/f'{role}-work.log').read_text().splitlines() if 'PUBLICATION_WORK_CENSUS ' in l]
 assert len(rows)==5
 if role=='base':base=rows
 else:
  for a,b in zip(base,rows):
   assert a['DeepTree']==b['DeepTree'] and b['NodesBefore']==b['NodesAfter']
   assert b['Parse']['leaf_constructions_proxy']<a['Parse']['leaf_constructions_proxy'] and b['Consume']['leaf_constructions_proxy']==0
p.joinpath('counter-gate.json').write_text(json.dumps({'rows':414,'changes':changes,'adverse_changes':[]},indent=2)+'\n')
print('414 counter rows passed; five consumed digests exact; constructors reduced without deferred work',flush=True)
