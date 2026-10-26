#!/usr/bin/env python3
"""Verify one held compact raw-output screen from raw evidence; never execute Go/C."""
from pathlib import Path, PurePosixPath
import argparse, gzip, hashlib, json, re, statistics, tarfile

FROZEN = 'controls/raw-summary-output-v1/screen-package-v1/frozen-controls.json'
TEMPLATE = 'controls/raw-summary-output-v1/screen-package-v1/validate-evidence.py.txt'
OLD_PREFIX = 'controls/raw-summary-output-v1/qualification-package-v1/'
RUNTIME = 'controls/raw-summary-output-v1/lease-root/compact-core-v5/overlay.runtime.json'
PERF = 'raw-summary-perf-compact-v6'
C027 = '10eb5ac119eec9cb4267a8e035066473983f5f3a'
GREEN = {
 'raw-summary-compact-v6': {'core-release.log':(79,164),'core-workcount.log':(15,25),'core-race.log':(15,25),'ordinary-root.log':(6,16),'ordinary-cgo.log':(1,7)},
 'raw-summary-work-compact-v6': {f'work-{n}.log':(1,2) for n in ('primary32','primary128','primary512','primary1024','real-r4')},
 'raw-summary-c027-compact-v6': {'runtime-probe.log':(1,1),'ordinary-cgo.log':(1,7)},
 'raw-summary-semantic-v6': {'focused.log':(36,68),'go-public-api.log':(3,9),'go-cgo.log':(10,41),'go-report-cgo.log':(3,127),'yaml-cgo.log':(8,184),**{f'invariants-{n}-{r}.log':(1,1) for n in ('go','yaml') for r in (0,1)}},
 'raw-summary-public-race-v6': {'public-race.log':(5,13)},
}
OLD_VARIANTS = ['raw-summary-history-v1','raw-summary-history-v2','raw-summary-history-transfer-v3','raw-summary-core-v1','raw-summary-core-reference-v1','raw-summary-root-v2','raw-summary-root-debug-v3','raw-summary-ordinary-repair-v4','raw-summary-public-lifetime-v4','raw-summary-linear-v5']
PENDING = ['all206_engine_fleet','large_file_RSS','API_consumer_timing','ARM64_and_other_platforms','E_A_authenticated_M1_baseline','Go_graduation','YAML_checkpoint_raw_lease']

def sha(data): return hashlib.sha256(data).hexdigest()
def txt(files,name): return files[name].decode()
def obj(files,name): return json.loads(txt(files,name))
def out(files,stage,name): return txt(files,f'outputs/{stage}/{name}')
def pass_names(text): return re.findall(r'^\s*--- PASS: (\S+)',text,re.M)

def controls(files):
 frozen={**obj(files,FROZEN)['files'],**obj(files,'controls/raw-summary-output-v1/screen-package-v1/profile-controls.json')['files']}
 for name,row in frozen.items():
  assert sha(files[name])==row['sha256'] and len(files[name])==row['bytes'],name
 checked=0; external=[]
 for name,data in files.items():
  p=PurePosixPath(name)
  if not name.startswith('controls/') or p.name not in ('controls.sha256','SHA256SUMS','checksums.container.sha256'): continue
  for line in data.decode().splitlines():
   if not line.strip(): continue
   expected,relative=line.split('  ',1)
   if relative.startswith('/controls/'): target='controls/'+relative.removeprefix('/controls/')
   elif not relative.startswith('/'):
    assert '..' not in PurePosixPath(relative).parts
    target=str(p.parent/relative)
   else:
    external.append({'checksum_file':name,'external_path':relative,'sha256':expected});continue
   assert target in files and sha(files[target])==expected,(name,target)
   checked+=1
 # Runtime map is exactly the measured compact composition, with telemetry off.
 assert sha(files[RUNTIME])=='25e9017c7210eb13766ebdd8f5bd9dd63b9c1bc31d12cf3023416ff786dd837c'
 assert len(obj(files,RUNTIME)['Replace'])==25
 perfmap=obj(files,'controls/raw-summary-perf-compact-v6/overlay.container.json')['Replace']
 runtime=obj(files,RUNTIME)['Replace']
 for target,backing in runtime.items(): assert perfmap[target]==backing
 assert len(perfmap)==len(runtime)+2
 return {'frozen_files':len(frozen),'checksum_entries_verified':checked,'external_readonly_inputs_authenticated_by_prior_provenance':external,'measured_runtime_targets':25,'work_debug_instrumentation_in_timing':False}

def stage(files,name,logs):
 assert obj(files,f'outputs/{name}/finished.json')['exit_code']==0,name
 for guard in ('source-before.log','source-after.log','controls-before.log','controls-after.log'):assert out(files,name,guard)=='',(name,guard)
 result={}
 for log,(tops,total) in logs.items():
  text=out(files,name,log);names=pass_names(text)
  assert len(re.findall(r'^--- PASS:',text,re.M))==tops and len(names)==total,(name,log)
  assert not re.search(r'--- (FAIL|SKIP):|^FAIL\b|WARNING: DATA RACE|panic:',text,re.M),(name,log)
  result[log]={'top_level_passes':tops,'all_pass_records':total,'selected_names':names}
 return result

def historical(files):
 frozen=obj(files,OLD_PREFIX+'frozen-controls.json')['files']
 subset={name:data for name,data in files.items() if name in {'controls/'+p for p in frozen} or any(name.startswith('outputs/'+v+'/') for v in OLD_VARIANTS)}
 subset[OLD_PREFIX+'frozen-controls.json']=files[OLD_PREFIX+'frozen-controls.json']
 namespace={'__name__':'historical_proof'}
 exec(compile(files[OLD_PREFIX+'validate-evidence.py.txt'],OLD_PREFIX+'validate-evidence.py.txt','exec'),namespace)
 value=namespace['validate_payloads'](subset)
 linear=out(files,'raw-summary-linear-v5','core-release.log')
 assert 'raw summary: draft byte cap' in linear and 'TestRawSummaryLinearHiddenPrefixScaling/1024' in linear
 assert value['linear_v5']['qualified'] is False
 assert obj(files,'outputs/raw-summary-c027-v4/finished.json')['exit_code']==125
 text=out(files,'raw-summary-c027-v4','container.log')
 assert 'error mounting' in text and 'read-only file system' in text
 value['C027_initial_infrastructure_failure']={'exit_code':125,'test_process_started':False,'reason':'readonly /controls nested mount could not create mountpoint'}
 return value,namespace

def c_cases(files,stage_name):
 text=out(files,stage_name,'ordinary-cgo.log')
 rows=re.findall(r'source_bytes=(\d+) source_sha256=([a-f0-9]{64}) actual_lease=(false|true) lease_nodes=(\d+) candidate_routes=1 fallback=0',text)
 expected={'19294':('2e0f9d4676549c8560cebc3c53877932f921ab9d77c88518227f3018573af4b7',11004),'2047':('b37389f6dbac4e417f19db04909783b0264dfec4e3c93774f0557c3e91b8f6c5',892)}
 assert len(rows)==4 and {(size,arm) for size,_,arm,_ in rows}=={(size,arm) for size in expected for arm in ('false','true')}
 for size,digest,arm,nodes in rows:assert digest==expected[size][0] and int(nodes)==(expected[size][1] if arm=='true' else 0)
 digests=re.findall(r'exact symbols, fields, spans, points, flags, and deep digest ([0-9a-f]{64})',text)
 assert len(digests)==4 and digests[0]==digests[1] and digests[2]==digests[3]
 return {'fixture_count':2,'strict_full_axis_comparisons':4,'actual_lease_both_original_sources':True,'source_sha256':{k:v[0] for k,v in expected.items()},'full_tree_digests':digests}

def work_proof(files):
 saved=obj(files,'outputs/raw-summary-work-compact-v6/validated-summary.json')
 assert saved['ordinary_lease_required'] and saved['warm_additional_work']==0 and len(saved['sources'])==5
 result=[]
 for x in saved['sources']:
  lines=[json.loads(line.split('RAW_SUMMARY_ORDINARY_WORK ',1)[1]) for line in out(files,'raw-summary-work-compact-v6',f"work-{x['Fixture']}.log").splitlines() if 'RAW_SUMMARY_ORDINARY_WORK ' in line]
  assert lines==[x] and x['After']==x['Warm'] and x['VisitedNodes']>0 and x['VisitedEdges']==x['VisitedNodes']-1
  before=x['Before'];formation=before['Formation']
  assert before['RegisteredContexts']==before['ArenaNodes']==1 and formation==x['After']['Formation']
  assert formation['ExceptionalRefusals']==formation['RefusedRootBound']==formation['RefusedPlanEdgeBound']==0
  if x['Functions']:
   n=x['Functions']
   exact={'ClosureRawEdges':31*n+4,'FormationRawEdges':51*n+8,'FormationLocalRootRefs':18*n+4,'FormationHiddenSteps':2*n,'ComposedSummaries':28*n+4,'ExceptionalSummaries':3*n+1,'Contexts':31*n+5}
   for k,v in exact.items():assert formation[k]==v,(x['Fixture'],k)
  result.append(x)
 assert {x['Fixture'] for x in result}=={'primary32','primary128','primary512','primary1024','real-r4'}
 return {'actual_sources':result,'warm_additional_work':0,'generated_formation_affine_counts_verified':True,'real_R4_measured_separately':True}

def counters(files):
 saved=obj(files,'outputs/raw-summary-semantic-v6/validated-summary.json');rows=[]
 def diff(a,b,path=''):
  assert type(a)==type(b),(path,'type changed')
  if isinstance(a,dict):
   assert a.keys()==b.keys()
   return [d for k in a for d in diff(a[k],b[k],f'{path}/{k}' if path else k)]
  if isinstance(a,list):
   assert len(a)==len(b)
   return [d for i in range(len(a)) for d in diff(a[i],b[i],f'{path}/{i}' if path else str(i))]
  return [] if a==b else [{'path':path,'before':a,'after':b,'direction':'less_work'}]
 for route in ('default','candidate'):
  for lang in ('go','yaml'):
   name=f'{route}-{lang}.json'
   before=obj(files,'controls/out/packed-snapshot-budget-final/ledger/'+name);after=obj(files,'outputs/raw-summary-semantic-v6/'+name)
   changes=diff(before,after)
   expected=[{'path':'0/full/new_nodes','before':892,'after':1,'direction':'less_work'}] if (route,lang)==('candidate','go') else []
   assert changes==expected,(name,changes)
   row={'route':route,'language':lang,'exact_equal':not changes,'approved_work_exception_used':False,'changes':changes}
   rows.append(row)
 assert rows==saved['counter_rows'] and saved['adverse_counter_changes']==0
 assert saved['D8_sessions']==4 and saved['D8_steps']==288 and saved['no_edit_allocations']==0
 for lang in ('go','yaml'):
  for route in (0,1):assert 'session_steps=72 no_edit_allocs=0' in out(files,'raw-summary-semantic-v6',f'invariants-{lang}-{route}.log')
 return {'actual_rows':rows,'only_change':'candidate Go full/new_nodes 892 to 1','exceptions_used':False,'D8_sessions':4,'D8_steps':288,'no_edit_allocations':0}

def perf_proof(files):
 assert obj(files,f'outputs/{PERF}/finished.json')['exit_code']==0
 assert f'outputs/{PERF}/completed.txt' in files
 for guard in ('source-before.log','source-after.log','controls-before.log','controls-after.log'):assert out(files,PERF,guard)==''
 assert obj(files,f'outputs/{PERF}/gates-before.json')['selected_semantic_gates_passed']
 reports={}
 for series,pattern,wanted in [('go-primary','primary-{}.txt',['BenchmarkGraduationGoSuite/Full','BenchmarkGraduationGoSuite/Edit','BenchmarkGraduationGoSuite/NoEdit']),('go-r4','r4-{}-go.txt',['BenchmarkCompactGraduationR4/legacy','BenchmarkCompactGraduationR4/compact']),('yaml-r4','r4-{}-yaml.txt',['BenchmarkCompactGraduationR4/legacy','BenchmarkCompactGraduationR4/compact'])]:
  samples={};orders={};raw={}
  for arm in ('before','after'):
   name=f'outputs/{PERF}/'+pattern.format(arm);text=txt(files,name);raw[arm]=sha(files[name])
   assert text.rstrip().endswith('# status: complete') and not re.search(r'^FAIL|--- (FAIL|SKIP):|panic:|WARNING: DATA RACE',text,re.M)
   for header in ('# protocol: paired-alternating-seeds','# seeds: 1..20','# benchtime: 750ms','# GOMAXPROCS: 1','# count per process: 1','# build tags: gts_parsercorephase0'):assert header in text
   assert list(map(int,re.findall(r'^# completed seed: (\d+);',text,re.M)))==list(range(1,21))
   starts=list(re.finditer(r'^# seed: (\d+); position: (\d+)\n',text,re.M));assert len(starts)==20
   by_seed={};by_order={}
   for i,m in enumerate(starts):
    seed,position=map(int,m.groups());assert seed==i+1
    assert position==((1 if seed%2 else 2) if arm=='before' else (2 if seed%2 else 1))
    chunk=text[m.end():starts[i+1].start() if i+1<len(starts) else len(text)]
    order=re.findall(r'^# workload order: (.*)',chunk,re.M);assert len(order)==1 and sorted(order[0].split(','))==sorted(wanted)
    by_order[seed]=order[0];rows={}
    for line in chunk.splitlines():
     if not line.startswith('Benchmark'):continue
     bench=line.split()[0];assert bench in wanted and bench not in rows
     n=re.search(r'\s(\d+)\s+([0-9.]+) ns/op',line);assert n and int(n[1])>0
     metrics={unit:float(value) for value,unit in re.findall(r'([0-9.]+) (ns/op|B/op|allocs/op|compact-routes/op|compact-fallbacks/op)',line)}
     assert {'ns/op','B/op','allocs/op'}<=metrics.keys() and metrics['ns/op']>0
     if series!='go-primary':assert metrics.get('compact-fallbacks/op')==0 and metrics.get('compact-routes/op')==(1 if bench.endswith('/compact') else 0)
     elif bench.endswith('/NoEdit'):assert metrics['B/op']==metrics['allocs/op']==0
     rows[bench]=metrics
    assert set(rows)==set(wanted);by_seed[seed]=rows
   samples[arm]=by_seed;orders[arm]=by_order
  assert orders['before']==orders['after']
  results={}
  for bench in wanted:
   results[bench]={}
   for metric in ('ns/op','B/op','allocs/op'):
    b=[samples['before'][s][bench][metric] for s in range(1,21)];a=[samples['after'][s][bench][metric] for s in range(1,21)]
    bm,am=statistics.median(b),statistics.median(a);ratios=[x/y for x,y in zip(a,b)] if all(b) else None
    results[bench][metric]={'before_median':bm,'after_median':am,'change_percent':None if bm==0 else (am/bm-1)*100,'median_paired_after_before_ratio':None if ratios is None else statistics.median(ratios),'ratio_of_medians':None if bm==0 else am/bm}
  route_ratios={}
  if series!='go-primary':
   legacy,compact=wanted
   for arm in ('before','after'):
    route_ratios[arm]={'median_paired_compact_legacy_ratio':statistics.median(samples[arm][s][compact]['ns/op']/samples[arm][s][legacy]['ns/op'] for s in range(1,21)),'ratio_of_medians':statistics.median(samples[arm][s][compact]['ns/op'] for s in range(1,21))/statistics.median(samples[arm][s][legacy]['ns/op'] for s in range(1,21))}
  reports[series]={'paired_samples':20,'raw_sha256':raw,'results':results,'route_ratios':route_ratios}
 saved=obj(files,f'outputs/{PERF}/validated-summary.json')
 assert saved['results']==reports and saved['pairs']==60 and saved['processes']==120 and saved['failed_or_incomplete_processes']==0
 return saved

def profile_proof(files):
 name='raw-summary-profile-v6'
 assert obj(files,f'outputs/{name}/finished.json')['exit_code']==0
 for guard in ('source-before.log','source-after.log','controls-before.log','controls-after.log'):assert out(files,name,guard)==''
 entries={}
 for arm in ('baseline','candidate'):
  for fixture,length in (('primary500',19294),('real-r4',2047)):
   text=out(files,name,f'{arm}-{fixture}-entry.log')
   assert len(pass_names(text))==1 and not re.search(r'--- (FAIL|SKIP):|WARNING: DATA RACE|panic:',text)
   match=re.search(r'RAW_SUMMARY_PROFILE_ENTRY fixture=(\S+) bytes=(\d+) entry=(\{.*\})',text)
   assert match and match[1]==fixture and int(match[2])==length
   entry=json.loads(match[3]);assert entry['lease']==(arm=='candidate')
   if arm=='candidate':assert entry['arena_nodes']==1 and entry['core_owned_raw_records']==0 and entry['status']['Transferred'] and entry['status']['Ready']
   profile=out(files,name,f'{arm}-{fixture}-profile.log')
   assert re.search(r'^Benchmark\S+\s+\d+\s+[0-9.]+ ns/op',profile,re.M) and not re.search(r'^FAIL|--- (FAIL|SKIP):|panic:',profile,re.M)
   for suffix in ('cpu','mem'):
    assert len(files[f'outputs/{name}/{arm}-{fixture}.{suffix}'])>0
   for suffix in ('top.txt','cum.txt'):assert 'Total samples' in out(files,name,f'{arm}-{fixture}-{suffix}')
   entries[arm+'/'+fixture]=entry
 assert not any(k.startswith(f'outputs/{name}/') and ('large-root-only' in k or 'large-full-parity' in k) for k in files)
 return {'four_profiles_terminal':True,'actual_entry_by_arm':entries,'CPU_and_sampled_heap_for_attribution_only':True,'large_file_RSS_executed':False,'timing_qualification_from_profiles':False}

def validate_payloads(files):
 auth=controls(files);old,ns=historical(files)
 selected={name:stage(files,name,logs) for name,logs in GREEN.items()}
 ordinary=ns['actual_ordinary'](files,'raw-summary-compact-v6')
 for name in ('core-release.log','core-workcount.log','core-race.log'):assert set(ns['SCANNER_TESTS'])<=set(pass_names(out(files,'raw-summary-compact-v6',name)))
 for name in ('ordinary-root.log',):assert {'TestRawSummarySavedSourceAuthorityRejectsEOFMutation/'+x for x in ns['AUTHORITY_ATTACKS']}<=set(pass_names(out(files,'raw-summary-compact-v6',name)))
 assert {'TestRawSummarySavedSourceAuthorityRejectsEOFMutation/'+x for x in ns['AUTHORITY_ATTACKS']}<=set(pass_names(out(files,'raw-summary-public-race-v6','public-race.log')))
 c025=c_cases(files,'raw-summary-compact-v6');c027=c_cases(files,'raw-summary-c027-compact-v6');assert c025['full_tree_digests']==c027['full_tree_digests']
 probe=out(files,'raw-summary-c027-compact-v6','runtime-probe.log')
 identity=json.loads(re.search(r'UNPROMOTED_ORACLE0271 identity=(\{.*?\}) ABI=',probe)[1])
 assert identity['runtime_commit']==C027 and identity['runtime_version']=='0.27.1' and identity['binding_version']=='v0.25.0'
 assert 'ABI=15 minimum_ABI=13 new_symbol_linked=true' in probe
 for name in ('oracle-before.log','oracle-after.log','binding-before.log','binding-after.log'):assert out(files,'raw-summary-c027-compact-v6',name)==''
 return {'schema':'gts-compact-raw-output-screen-proof/v1','controls':auth,'historical_qualification':old,'compact_v6_selected':selected,'ordinary_actual_ownership':ordinary,'C025':c025,'C027':{**c027,'identity':identity,'API_scope':'default parse/close only; prior frozen private adapter','oracle_adopted':False,'general_binding_compatibility_claim':False},'real_source_formation_and_read_work':work_proof(files),'deterministic_ledger_and_D8':counters(files),'randomized_timing':perf_proof(files),'profile_attribution':profile_proof(files),'pending_qualification':PENDING,'runtime_promoted':False,'default_or_dependency_changed':False,'graduation_claim':False,'merges_to_main_held':True}

def read_archive(directory):
 names=set()
 for line in (directory/'SHA256SUMS').read_text().splitlines():
  expected,name=line.split('  ',1);assert PurePosixPath(name).name==name and name not in names
  assert sha((directory/name).read_bytes())==expected,name;names.add(name)
 assert names=={'evidence.tar.gz','contents.json.gz','source-identities.json','verification.json','summary.json','validate-evidence.py'}
 index=json.loads(gzip.decompress((directory/'contents.json.gz').read_bytes()));expected={x['path']:x for x in index};assert len(index)==len(expected)
 files={}
 with tarfile.open(directory/'evidence.tar.gz','r:gz') as archive:
  for member in archive:
   p=PurePosixPath(member.name);assert member.isfile() and not p.is_absolute() and '..' not in p.parts and member.name not in files
   data=archive.extractfile(member).read();row=expected[member.name]
   assert len(data)==member.size==row['bytes'] and sha(data)==row['sha256'];files[member.name]=data
 assert set(files)==set(expected)
 return files

def main():
 parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--source-root',type=Path);args=parser.parse_args();directory=Path(__file__).resolve().parent
 files=read_archive(directory);assert Path(__file__).read_bytes()==files[TEMPLATE]
 verified=validate_payloads(files);assert verified==json.loads((directory/'verification.json').read_text())
 summary=json.loads((directory/'summary.json').read_text());assert summary['root_decision']['decision'] and summary['root_decision']['rationale'] and summary['actual_screen']==verified
 identities=json.loads((directory/'source-identities.json').read_text());assert identities['runtime_map_sha256']==sha(files[RUNTIME])
 for name,expected in identities['archived_controls_sha256'].items():assert sha(files[name])==expected
 if args.source_root:
  for line in txt(files,'controls/raw-summary-perf-compact-v6/source.sha256').splitlines():
   expected,name=line.split('  ',1);p=PurePosixPath(name);assert not p.is_absolute() and '..' not in p.parts
   assert sha((args.source_root/name).read_bytes())==expected,name
 print(json.dumps({'validated':True,'root_decision':summary['root_decision'],'members':len(files),'pairs':60,'processes':120,'C025_original_sources':2,'C027_original_sources':2,'D8_steps':288,'only_counter_change':'candidate Go full new_nodes892→1','runtime_promoted':False,'pending_qualification':PENDING,'merges_to_main_held':True},indent=2))

if __name__=='__main__':main()
