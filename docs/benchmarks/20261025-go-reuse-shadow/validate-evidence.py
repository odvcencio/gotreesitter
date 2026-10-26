#!/usr/bin/env python3
"""Validate shadow correctness/counter evidence without running the prototype."""
from pathlib import Path,PurePosixPath
import argparse,gzip,hashlib,json,re,tarfile

def sha(b):return hashlib.sha256(b).hexdigest()
def validate(d,source_root=None):
 if (d/'SHA256SUMS').exists():
  for line in (d/'SHA256SUMS').read_text().splitlines():
   digest,name=line.split('  ',1);assert PurePosixPath(name).name==name;assert sha((d/name).read_bytes())==digest,name
 index=json.loads(gzip.decompress((d/'contents.json.gz').read_bytes()));wanted={x['path']:x for x in index};assert len(wanted)==len(index);files={}
 with tarfile.open(d/'evidence.tar.gz','r:gz') as t:
  for m in t:
   p=PurePosixPath(m.name);assert m.isfile() and not p.is_absolute() and '..' not in p.parts and m.name not in files
   b=t.extractfile(m).read();row=wanted[m.name];assert len(b)==m.size==row['bytes'] and sha(b)==row['sha256'];files[m.name]=b
 assert files.keys()==wanted.keys()
 def obj(n):return json.loads(files[n])
 def text(stage,n):return files[f'outputs/{stage}/{n}'].decode()
 def guards(stage,code):
  assert obj(f'outputs/{stage}/finished.json')['exit_code']==code
  for name in ['source-before','source-after','controls-before','controls-after']:assert text(stage,name+'.log')==''
 def passes(stage,name,tops,records):
  s=text(stage,name);assert len(re.findall(r'^--- PASS:',s,re.M))==tops and len(re.findall(r'^\s*--- PASS:',s,re.M))==records,(stage,name)
  assert not re.search(r'--- (FAIL|SKIP):|WARNING: DATA RACE|^FAIL\b|panic:',s,re.M);return s
 runtime='controls/trusted-go-shadow-v1/overlay.runtime.json';assert sha(files[runtime])=='b8516549c515222d8aeb5d4b4a2609b50d8abbcbd4b691e091d73aff7f40f49d'
 m=obj(runtime)['Replace'];assert len(m)==11
 for target,backing in m.items():assert files['controls/'+backing.removeprefix('/controls/')]
 checked=0
 for name,b in files.items():
  if PurePosixPath(name).name not in ['controls.sha256','SHA256SUMS']:continue
  for line in b.decode().splitlines():
   digest,path=line.split('  ',1)
   if path.startswith('/controls/'):key='controls/'+path.removeprefix('/controls/')
   elif not path.startswith('/'):key=str(PurePosixPath(name).parent/path)
   else:continue
   assert key in files and sha(files[key])==digest,(name,key);checked+=1
 v1='trusted-go-shadow-probe-v1';guards(v1,1)
 assert 'producer=native-chain verifier_events=2 would_certify=2 noedit_allocs=0' in text(v1,'original-Go-producer-shadow.log')
 assert 'state/precedence differs at source_file[0..140369]' in text(v1,'original-Go-producer-shadow.log')
 control='trusted-go-shadow-state-control-v1';guards(control,1);planes={}
 maps={arm:obj(f'controls/trusted-go-shadow-v2-adjudication/overlay.{arm}.tests.json')['Replace'] for arm in ['baseline','candidate','capability-off']}
 assert len(maps['baseline'])==2
 for target,backing in maps['baseline'].items():assert maps['candidate'][target]==maps['capability-off'][target]==backing
 for arm in maps:
  s=text(control,arm+'.log');assert text(control,arm+'.exit').strip()=='1'
  assert s.count('public_equal=true verified=true certified=false')==2
  planes[arm]=re.findall(r'STRICT_STATE nodes=(\d+) parse_diffs=(\d+) pre_diffs=(\d+) precedence_diffs=(\d+) got_state_sha256=([a-f0-9]+) fresh_state_sha256=([a-f0-9]+)',s)
  assert len(planes[arm])==2 and planes[arm][0][1:4]==('0','0','0') and planes[arm][0][4]==planes[arm][0][5]
  assert planes[arm][1][1:4]==('0','0','7037')
 assert planes['baseline']==planes['candidate']==planes['capability-off']
 probe='trusted-go-shadow-probe-v2';guards(probe,0)
 expected={'internal-policy.log':(4,23),'artifact-scope.log':(1,1),'actual-producer-guards.log':(1,20),'original-Go-producer-shadow.log':(1,7),'invariants-go-0.log':(1,1),'invariants-go-1.log':(1,1),'memory.log':(12,15),'internal-race.log':(4,23),'producer-race.log':(2,27)}
 for name,(tops,records) in expected.items():passes(probe,name,tops,records)
 for route in [0,1]:assert 'session_steps=72 no_edit_allocs=0' in text(probe,f'invariants-go-{route}.log')
 for name in ['original-Go-producer-shadow.log','producer-race.log']:
  s=text(probe,name)
  assert 'source_bytes=140368 source_sha256=5434575bb249338937331c3756e10dcec34237d107f523ea315996fb6de0ef8f producer=native-chain verifier_events=2 would_certify=2 noedit_allocs=0' in s
  for kind in ['copied','transformed-factory','identity-wrapper','compact-producer','no-artifact']:
   assert re.search('producer='+kind+r' verifier_events=\d+ would_certify=0 noedit_allocs=0',s),kind
 c='trusted-go-shadow-cgo-v1';guards(c,0);s=passes(c,'existing-strong-C-Go.log',3,127)
 for case in ['TestGoCognociente137KiBTopEditLockedC','TestGoMalformedDeclarationPrefixLockedC','TestGoAssignmentTargetEdits16SitesLockedC']:assert '\n--- PASS: '+case+' ' in s
 assert 'source_bytes=140368 edited_bytes=140369 edit_byte=57' in s
 ledger='trusted-go-shadow-ledger-v1';guards(ledger,0)
 def diff(a,b,path=''):
  assert type(a)==type(b)
  if isinstance(a,dict):
   assert a.keys()==b.keys();return [x for k in a for x in diff(a[k],b[k],path+'/'+k)]
  if isinstance(a,list):
   assert len(a)==len(b);return [x for i in range(len(a)) for x in diff(a[i],b[i],path+'/'+str(i))]
  return [] if a==b else [{'path':path,'before':a,'after':b}]
 rows=[]
 for route in ['default','candidate']:
  for lang in ['go','yaml']:
   n=f'{route}-{lang}.json';changes=diff(obj('baselines/ledger/'+n),obj('outputs/'+ledger+'/'+n));assert text(ledger,route+'-'+lang+'.stderr')==''
   expected_changes=[{'path':'/0/edits/0/counters/'+key,'before':a,'after':b} for key,a,b in [('tokens',572,614),('new_nodes',3810,3219),('max_live_versions',12,8),('multi_version_token_share_ppm',531468,495114),('reused_bytes',1166,1044),('block_splices',296,260)]] if (route,lang)==('default','go') else []
   assert changes==expected_changes;rows.append({'route':route,'language':lang,'changes':changes})
 summary=obj('outputs/'+ledger+'/actual-differences.json');assert summary['rows']==rows and not summary['owner_exception_inferred'] and not summary['ledger_updated']
 saved=json.loads((d/'summary.json').read_text());assert saved['runtime_map_sha256']==sha(files[runtime]) and saved['actual_counters']==summary
 assert not saved['new_timing_claim'] and not saved['runtime_promoted'] and saved['main_merges_held']
 count=0
 if source_root:
  for line in files['baselines/source.sha256'].decode().splitlines():
   digest,name=line.split('  ',1);p=PurePosixPath(name);assert not p.is_absolute() and '..' not in p.parts;assert sha((Path(source_root)/name).read_bytes())==digest,name;count+=1
 return {'schema':'gts-trusted-go-shadow-review-proof/v1','archive_members':len(files),'checksum_entries':checked,'source_files_verified':count,'runtime_targets':11,'native_chain_would_certify':2,'actual_fresh_verifiers':2,'unsupported_producers_qualified':0,'strict_failures_preserved':True,'preexisting_internal_precedence_only':7037,'parse_or_pre_goto_differences':0,'C_families':3,'C_records':127,'D8_sessions':2,'D8_steps':144,'no_edit_allocations':0,'actual_counter_rows':rows,'unapproved_adverse_fields':['tokens','reused_bytes','block_splices'],'counter_gate_passed':False,'new_timing_claim':False,'runtime_promoted':False,'graduation_claim':False,'main_merges_held':True}

if __name__=='__main__':
 p=argparse.ArgumentParser();p.add_argument('--source-root');args=p.parse_args();d=Path(__file__).resolve().parent;result=validate(d,args.source_root)
 if (d/'verification.json').exists():
  expected=json.loads((d/'verification.json').read_text());assert {k:v for k,v in result.items() if k!='source_files_verified'}=={k:v for k,v in expected.items() if k!='source_files_verified'}
 print(json.dumps(result,indent=2))
