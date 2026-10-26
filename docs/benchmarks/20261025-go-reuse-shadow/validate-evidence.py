#!/usr/bin/env python3
"""Validate shadow correctness/counter evidence without running the prototype."""
from pathlib import Path,PurePosixPath
import argparse,gzip,hashlib,json,re,tarfile

def sha(b):return hashlib.sha256(b).hexdigest()

def source_fixture():
 source=b'package main\n\nimport "fmt"\n\n'
 index=0
 while len(source)<137*1024:
  source+=f'func f{index}(a int, b int) int {{\n\tx := a + b\n\tfmt.Println("f{index}", x)\n\treturn x\n}}\n\n'.encode()
  index+=1
 assert len(source)==140368 and index==1760 and sha(source)=='5434575bb249338937331c3756e10dcec34237d107f523ea315996fb6de0ef8f'
 return source

def clean_session_proof(files,obj,text,guards,passes):
 stage='trusted-go-shadow-clean-sessions-v1';guards(stage,0)
 runtime='controls/trusted-go-shadow-session-v1/overlay.runtime.json'
 assert sha(files[runtime])=='b8516549c515222d8aeb5d4b4a2609b50d8abbcbd4b691e091d73aff7f40f49d'
 original=source_fixture();sites=[0,117,235,352,469,587,704,821,939,1056,1173,1291,1408,1525,1643,1759]
 operations=[(i,phase<2) for phase in range(4) for i in range(16)]+[(i,grow) for grow in (True,False) for i in (0,5,10,15)]
 assert len(operations)==72
 sessions=[];chains={}
 for api in ('plain','profiled'):
  s=passes(stage,f'clean-{api}-session.log',1,2)
  rows=[json.loads(line.split('TRUSTED_GO_SESSION_STEP ',1)[1]) for line in s.splitlines() if 'TRUSTED_GO_SESSION_STEP ' in line]
  assert len(rows)==72
  current=original;lengths=[1]*16;events=would=shared=0
  for step,(row,(site,grow)) in enumerate(zip(rows,operations)):
   prefix=f'func f{sites[site]}(a int, b int) int {{\n\t'.encode();at=current.index(prefix)+len(prefix)
   assert current[at:].startswith(b'x'*lengths[site]+b' := a + b\n')
   after=current[:at]+(b'x'+current[at:] if grow else current[at+1:]);lengths[site]+=1 if grow else -1
   assert lengths[site]>=1 and abs(len(after)-len(current))==1
   expected={'API':api,'Step':step,'Function':sites[site],'Kind':'grow' if grow else 'shrink','EditByte':at,'BeforeBytes':len(current),'AfterBytes':len(after),'BeforeSourceSHA':sha(current),'AfterSourceSHA':sha(after),'ProducerKey':8195,'VerifierEvents':1,'WouldCertify':1,'NotWouldCertify':0,'SharedTopLevel':1761,'FreshVerified':True,'BypassCertified':False,'NoEditAllocs':0}
   for key,value in expected.items():assert row[key]==value,(api,step,key,row[key],value)
   if api=='profiled':assert row['ReusedSubtrees']>0 and row['ReusedBytes']>0
   else:assert row['ReusedSubtrees']==row['ReusedBytes']==0
   events+=row['VerifierEvents'];would+=row['WouldCertify'];shared+=row['SharedTopLevel'];current=after
  assert current==original and lengths==[1]*16
  summary=re.findall(r'TRUSTED_GO_SESSION_SUMMARY api=(\S+) steps=(\d+) functions=(\d+) initial_bytes=(\d+) final_bytes=(\d+) final_sha256=([a-f0-9]+) verifier_events=(\d+) would_certify=(\d+) shared_top_level=(\d+) noedit_allocs=(\d+)',s)
  assert summary==[(api,'72','16','140368','140368',sha(original),str(events),str(would),str(shared),'0')]
  session={'api':api,'steps':72,'functions':16,'actual_verifier_events':events,'would_certify':would,'shared_top_level':shared,'minimum_profiled_reused_bytes':min(x['ReusedBytes'] for x in rows) if api=='profiled' else None,'no_edit_allocations':0,'restored_original_source':True,'steps_data':rows}
  sessions.append(session);chains[api]=[(x['Function'],x['Kind'],x['EditByte'],x['BeforeSourceSHA'],x['AfterSourceSHA']) for x in rows]
 assert chains['plain']==chains['profiled']
 negative=passes(stage,'inherited-producer-negatives.log',1,7)
 for kind in ('copied','transformed-factory','identity-wrapper','compact-producer','no-artifact'):assert re.search('producer='+kind+r' verifier_events=\d+ would_certify=0 noedit_allocs=0',negative)
 proof={'schema':'gts-trusted-go-shadow-clean-sessions/v1','sessions':sessions,'total_steps':144,'actual_verifier_events':144,'would_certify':144,'fresh_verifier_active':True,'runtime_source_changed':False,'counter_or_timing_gate_passed':False,'runtime_promoted':False,'main_merges_held':True}
 assert proof==obj('outputs/'+stage+'/validated-summary.json')
 return proof

def complete_work_proof(files,obj,text,guards):
 stage='trusted-go-shadow-complete-work-v1';guards(stage,0)
 source='prior-controls/recovery-work-main.go'
 assert sha(files[source])=='d1382a4bfdae12522929c54c018b9f2edb70556d008ef4db657d82255254f684'
 assert 'old.Edit(' in files[source].decode() and 'gts.BeginDiagnosticWorkCount()' in files[source].decode() and 'gts.EndDiagnosticWorkCount()' in files[source].decode()
 expected_keys={(assignment,candidate,kind,phase) for assignment in (False,True) for candidate in (False,True) for kind in ('insert','replace','delete') for phase in ('fresh','edit')}
 arms={}
 counter_names=None
 for arm in ('before','after'):
  arms[arm]={}
  assert text(stage,arm+'-build.log')==''
  for assignment in (0,1):
   raw=text(stage,f'{arm}-work-{assignment}.log')
   assert not re.search(r'panic:|WARNING: DATA RACE|^FAIL\b',raw,re.M)
   records=[json.loads(line[5:]) for line in raw.splitlines() if line.startswith('WORK ')]
   assert len(records)==12
   for row in records:
    key=(row['assignment'],row['candidate'],row['kind'],row['phase']);assert key not in arms[arm] and row['assignment']==bool(assignment)
    assert row.keys()=={'assignment','candidate','kind','phase','selected','work','has_error','end_byte'}
    w=row['work'];assert w['contract']=='gts-work-count/v2' and w['overflow'] is False
    names=set(w['outside_attempt'])
    if counter_names is None:counter_names=names
    assert names==counter_names
    assert w['attempts'] is None or isinstance(w['attempts'],list)
    attempts=w['attempts'] or []
    for name in names:
     assert type(w[name])==int and w[name]>=0 and w[name]==w['outside_attempt'][name]+sum(a['counters'][name] for a in attempts),(arm,key,name)
    for attempt in attempts:
     assert all(set(attempt[phase])==names for phase in ('entry_to_caps','caps_to_finalize','finalize','counters'))
     for name in names:assert attempt['counters'][name]==sum(attempt[phase][name] for phase in ('entry_to_caps','caps_to_finalize','finalize')),(arm,key,name)
    if row['phase']=='fresh':assert row['selected'] is None
    else:assert isinstance(row['selected'],dict) and not any(k.endswith('Nanos') for k in row['selected'])
    arms[arm][key]=row
  assert arms[arm].keys()==expected_keys
 def differences(a,b,path=''):
  assert type(a)==type(b),(path,type(a),type(b))
  if isinstance(a,dict):
   assert a.keys()==b.keys();return [x for k in a for x in differences(a[k],b[k],path+'/'+k)]
  if isinstance(a,list):
   assert len(a)==len(b);return [x for i in range(len(a)) for x in differences(a[i],b[i],path+'/'+str(i))]
  if a==b:return []
  return [{'path':path,'before':a,'after':b,'change_percent':(b/a-1)*100 if type(a)==int and a!=0 else None}]
 rows=[];groups=[];totals=[];phases=[]
 for key,a in arms['before'].items():
  b=arms['after'][key]
  for name in ('assignment','candidate','kind','phase','has_error','end_byte'):assert a[name]==b[name],(key,name)
  changes=differences(a,b)
  row={'assignment':key[0],'candidate':key[1],'kind':key[2],'phase':key[3],'exact_equal':not changes,'work_exact_equal':a['work']==b['work'],'differences':changes};rows.append(row)
  if not row['work_exact_equal']:
   assert key[1] is False and key[3]=='edit'
   overall=[x for x in changes if x['path'].startswith('/work/') and x['path'].count('/')==2]
   phase=[x for x in changes if x['path']=='/work/attempts/0/counters/table_lookups_proxy'];assert len(phase)==1
   lookup=[x for x in overall if x['path']=='/work/table_lookups_proxy'];assert len(lookup)==1
   totals.append(lookup[0]['change_percent']);phases.append(phase[0]['change_percent'])
   if key[2]=='delete':
    delta=2 if key[0] else 4
    for name in ('shifts','action_entries_examined_proxy','lexer_front_door_calls_proxy','leaf_constructions_proxy'):assert b['work'][name]-a['work'][name]==delta,(key,name)
   else:assert [x['path'] for x in overall]==['/work/table_lookups_proxy']
   groups.append({'assignment':key[0],'route':'default','edit':key[2],'overall_event_changes':overall,'attempt_and_phase_changes':[x for x in changes if x['path'].startswith('/work/attempts/')],'selected_changes':[x for x in changes if x['path'].startswith('/selected/')]})
 comparison={'schema':'gts-trusted-go-shadow-complete-work/v1','records_per_arm':24,'matching_output_error_and_extent':True,'exact_records':sum(x['exact_equal'] for x in rows),'work_exact_records':sum(x['work_exact_equal'] for x in rows),'rows':rows,'owner_exception_inferred':False,'new_timing_claim':False,'ledger_changed':False,'runtime_promoted':False,'merges_held':True}
 assert comparison==obj('outputs/'+stage+'/comparison.json')
 assert comparison['exact_records']==12 and comparison['work_exact_records']==18 and len(groups)==6
 assert all(x['work_exact_equal'] for x in rows if x['phase']=='fresh' or x['candidate'])
 review=obj('outputs/'+stage+'/review-summary.json')
 assert review['records_per_arm']==24 and review['identical_complete_work_records']==18 and review['full_operations_identical']==12 and review['changed_default_edit_operations']==6 and review['groups']==groups
 assert review['overall_table_lookup_increase_percent_range']==[min(totals),max(totals)] and review['counter_gate_passed'] is False and not review['approved_assignment_exception_used'] and not review['timing_ran'] and not review['promotion'] and review['main_merges_held']
 return {'records_per_arm':24,'attempt_and_phase_partitions_exact':True,'root_error_and_extent_equal':True,'work_identical_records':18,'entire_record_identical':12,'full_work_identical':12,'candidate_edit_work_identical':6,'changed_default_edits':6,'overall_table_lookup_percent_range':[min(totals),max(totals)],'incremental_phase_table_lookup_percent_range':[min(phases),max(phases)],'comparison':comparison,'review':review,'counter_gate_passed':False,'owner_exception_used':False,'timing_ran':False}
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
 original_index=files['provenance/original-contents.json.gz']
 assert sha(original_index)=='2f7caa5a559c57ce64f3d92f32c2b365a969a165a14afd1d9b56f9b272c79169'
 original_rows=json.loads(gzip.decompress(original_index));assert len(original_rows)==145
 for row in original_rows:assert sha(files[row['path']])==row['sha256'] and len(files[row['path']])==row['bytes'],row['path']
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
   elif path.startswith('/prior-controls/'):key='prior-controls/'+path.removeprefix('/prior-controls/')
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
 clean_sessions=clean_session_proof(files,obj,text,guards,passes)
 complete_operations=complete_work_proof(files,obj,text,guards)
 saved=json.loads((d/'summary.json').read_text());assert saved['runtime_map_sha256']==sha(files[runtime]) and saved['actual_counters']==summary
 assert saved['original_members_preserved']==145 and saved['archive_members']==len(files) and saved['original_archive_sha256']=='0fe58d4864d042a4a92cf67231ee39c924b93828cc51f36f3ca6cef3742a1100'
 assert saved['clean_sessions']==clean_sessions and saved['complete_operations']==complete_operations['review']
 assert not saved['new_timing_claim'] and not saved['runtime_promoted'] and saved['main_merges_held']
 count=0
 if source_root:
  for line in files['baselines/source.sha256'].decode().splitlines():
   digest,name=line.split('  ',1);p=PurePosixPath(name);assert not p.is_absolute() and '..' not in p.parts;assert sha((Path(source_root)/name).read_bytes())==digest,name;count+=1
 return {'schema':'gts-trusted-go-shadow-review-proof/v2','archive_members':len(files),'original_members_preserved':145,'checksum_entries':checked,'source_files_verified':count,'runtime_targets':11,'original_native_chain_would_certify':2,'original_actual_fresh_verifiers':2,'clean_sessions':clean_sessions,'complete_operations':complete_operations,'unsupported_producers_qualified':0,'strict_failures_preserved':True,'preexisting_internal_precedence_only':7037,'parse_or_pre_goto_differences':0,'C_families':3,'C_records':127,'R4_D8_sessions':2,'R4_D8_steps':144,'additional_clean_steps':144,'no_edit_allocations':0,'actual_counter_rows':rows,'unapproved_selected_adverse_fields':['tokens','reused_bytes','block_splices'],'counter_gate_passed':False,'new_timing_claim':False,'runtime_promoted':False,'graduation_claim':False,'main_merges_held':True}

if __name__=='__main__':
 p=argparse.ArgumentParser();p.add_argument('--source-root');args=p.parse_args();d=Path(__file__).resolve().parent;result=validate(d,args.source_root)
 if (d/'verification.json').exists():
  expected=json.loads((d/'verification.json').read_text());assert {k:v for k,v in result.items() if k!='source_files_verified'}=={k:v for k,v in expected.items() if k!='source_files_verified'}
 print(json.dumps(result,indent=2))
