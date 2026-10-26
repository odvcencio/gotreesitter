#!/usr/bin/env python3
"""Check the held construction screen; no Go, C, network or archive extraction."""
from pathlib import Path,PurePosixPath
import argparse,gzip,hashlib,importlib.util,json,re,sys,tarfile
sys.dont_write_bytecode=True

def sha(b):return hashlib.sha256(b).hexdigest()
def load_delta(d):
 files={};index=json.loads(gzip.decompress((d/'contents.json.gz').read_bytes()));wanted={x['path']:x for x in index};assert len(wanted)==len(index)
 with tarfile.open(d/'evidence.tar.gz','r:gz') as t:
  for m in t:
   p=PurePosixPath(m.name);assert m.isfile() and not p.is_absolute() and '..' not in p.parts and m.name not in files
   b=t.extractfile(m).read();row=wanted[m.name];assert m.size==len(b)==row['bytes'] and sha(b)==row['sha256'];files[m.name]=b
 assert files.keys()==wanted.keys();return files

def validate(d,source_root=None):
 base=d.parent/'20261025-compact-raw-screen'
 assert sha((base/'evidence.tar.gz').read_bytes())=='73fe52b4ef05a02a6741265da9b447ed3e2610338880592a6d1ff57c5f8c90fa'
 expected={}
 for line in (base/'SHA256SUMS').read_text().splitlines():
  digest,name=line.split('  ',1);assert PurePosixPath(name).name==name;assert sha((base/name).read_bytes())==digest;expected[name]=digest
 spec=importlib.util.spec_from_file_location('qualified_raw_screen',base/'validate-evidence.py');old=importlib.util.module_from_spec(spec);spec.loader.exec_module(old)
 inherited=old.read_archive(base);delta=load_delta(d);assert not (inherited.keys()&delta.keys())
 files={**inherited,**delta}
 checked=0
 for name,b in delta.items():
  if PurePosixPath(name).name not in ('controls.sha256','SHA256SUMS'):continue
  for line in b.decode().splitlines():
   digest,path=line.split('  ',1)
   if path.startswith('/controls/'):target='controls/'+path.removeprefix('/controls/')
   elif not path.startswith('/'):target=str(PurePosixPath(name).parent/path)
   else:continue
   assert target in files and sha(files[target])==digest,(name,target);checked+=1
 runtime_name='controls/raw-summary-output-v1/fused-commit-v1/overlay.runtime.json'
 assert sha(files[runtime_name])=='5ef7f2bf727ed3e01fce62ba6cc1edea31a5ab59ff8d6a0d826c16448dd7d436'
 runtime=json.loads(files[runtime_name])['Replace'];timed=json.loads(files['controls/raw-summary-perf-fused-v7/overlay.container.json'])['Replace']
 assert len(runtime)==26 and len(timed)==28
 for target,backing in runtime.items():assert timed[target]==backing and 'controls/'+backing.removeprefix('/controls/') in files
 wrapper='/controls/raw-summary-perf-fused-v7/benchmark-source.go.txt'
 assert timed['/baseline/benchmark_admission_candidate_route_test.go']==timed['/candidate/benchmark_admission_candidate_route_test.go']==wrapper
 assert sha(files['controls/'+wrapper.removeprefix('/controls/')])=='2d55591291b04e514b505b0671a7f912fd5bdc5d8c42eff8bf1c074d94e00cb4'
 stages={
 'raw-summary-fused-v7':{'core-release.log':(83,173),'core-workcount.log':(19,34),'core-race.log':(19,34),'ordinary-root.log':(6,16),'ordinary-cgo.log':(1,7)},
 'raw-summary-work-fused-v7':{f'work-{n}.log':(1,2) for n in ('primary32','primary128','primary512','primary1024','real-r4')},
 'raw-summary-semantic-fused-v7':old.GREEN['raw-summary-semantic-v6'],
 'raw-summary-public-race-fused-v7':{'public-race.log':(5,13)},
 'raw-summary-c027-fused-v7':{'runtime-probe.log':(1,1),'ordinary-cgo.log':(1,7)}}
 gates={name:old.stage(files,name,logs) for name,logs in stages.items()}
 ordinary=old.out(files,'raw-summary-fused-v7','ordinary-root.log')
 assert ordinary.count('lease:true')==ordinary.count('arena_nodes:1')==ordinary.count('core_owned_raw_records:0')==4
 c025=old.c_cases(files,'raw-summary-fused-v7');c027=old.c_cases(files,'raw-summary-c027-fused-v7');assert c025['full_tree_digests']==c027['full_tree_digests']
 assert '"runtime_commit":"10eb5ac119eec9cb4267a8e035066473983f5f3a"' in old.out(files,'raw-summary-c027-fused-v7','runtime-probe.log')
 for name in ('oracle-before','oracle-after','binding-before','binding-after'):assert old.out(files,'raw-summary-c027-fused-v7',name+'.log')==''
 # Reuse the exact previously reviewed counter checker with only its output
 # names rebound. The actual baseline remains the original ledger bytes.
 virtual=dict(files)
 prefix='outputs/raw-summary-semantic-fused-v7/'
 for name,b in files.items():
  if name.startswith(prefix):virtual['outputs/raw-summary-semantic-v6/'+name[len(prefix):]]=b
 counters=old.counters(virtual)
 old.PERF='raw-summary-perf-fused-v7';perf=old.perf_proof(files)
 baseline=json.loads(files['outputs/raw-summary-work-compact-v6/validated-summary.json'])['sources']
 current=json.loads(files['outputs/raw-summary-work-fused-v7/validated-summary.json'])['sources'];assert len(current)==5
 reductions=[]
 for a,b in zip(baseline,current):
  lines=[json.loads(line.split('RAW_SUMMARY_ORDINARY_WORK ',1)[1]) for line in old.out(files,'raw-summary-work-fused-v7',f"work-{b['Fixture']}.log").splitlines() if 'RAW_SUMMARY_ORDINARY_WORK ' in line]
  assert lines==[b] and a['Fixture']==b['Fixture'] and a['TreeSHA256']==b['TreeSHA256'] and b['After']==b['Warm']
  for key in ('RawRecords','ReplayContexts','PublicNodes','RegisteredContexts','ArenaNodes','RetainedBytes'):assert a['Before'][key]==b['Before'][key]
  assert b['Before']['RegisteredContexts']==b['Before']['ArenaNodes']==1 and b['Before']['Formation']==b['After']['Formation']
  for key,value in b['Before']['Formation'].items():assert value<=a['Before']['Formation'][key],(b['Fixture'],key)
  n=b['Functions'];f=b['Before']['Formation']
  if n:
   for key,value in {'ClosureRawEdges':31*n+4,'FormationRawEdges':41*n+6,'FormationLocalRootRefs':9*n+2,'FormationHiddenSteps':n}.items():assert f[key]==value
  reductions.append({'fixture':b['Fixture'],'formation_edges':[a['Before']['Formation']['FormationRawEdges'],f['FormationRawEdges']],'local_root_refs':[a['Before']['Formation']['FormationLocalRootRefs'],f['FormationLocalRootRefs']],'hidden_steps':[a['Before']['Formation']['FormationHiddenSteps'],f['FormationHiddenSteps']]})
 saved=json.loads((d/'summary.json').read_text());assert saved['results']==perf['results'] and saved['runtime_map_sha256']==sha(files[runtime_name]) and saved['base_evidence_archive_sha256']==sha((base/'evidence.tar.gz').read_bytes())
 source_checks=0
 if source_root:
  for line in files['controls/raw-summary-perf-fused-v7/source.sha256'].decode().splitlines():
   digest,path=line.split('  ',1);relative=PurePosixPath(path);assert not relative.is_absolute() and '..' not in relative.parts;p=Path(source_root)/path;assert sha(p.read_bytes())==digest,path;source_checks+=1
 return {'schema':'gts-compact-construction-screen-proof/v1','delta_members':len(delta),'inherited_members':len(inherited),'checksum_entries':checked,'source_files_verified':source_checks,'runtime_targets':len(runtime),'timing_instrumentation':False,'gates':gates,'C025':c025,'C027':c027,'counters_and_D8':counters,'formation_reductions':reductions,'paired_timing':perf,'runtime_promoted':False,'graduation_claim':False,'main_merges_held':True}

if __name__=='__main__':
 parser=argparse.ArgumentParser();parser.add_argument('--source-root');args=parser.parse_args();d=Path(__file__).resolve().parent

 if (d/'SHA256SUMS').exists():
  for line in (d/'SHA256SUMS').read_text().splitlines():
   digest,name=line.split('  ',1);assert PurePosixPath(name).name==name;assert sha((d/name).read_bytes())==digest,name
 result=validate(d,args.source_root)
 if (d/'verification.json').exists():
  expected=json.loads((d/'verification.json').read_text());assert {k:v for k,v in result.items() if k!='source_files_verified'}=={k:v for k,v in expected.items() if k!='source_files_verified'}
 print(json.dumps(result,indent=2))
