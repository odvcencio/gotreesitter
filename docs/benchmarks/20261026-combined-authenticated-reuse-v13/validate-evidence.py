#!/usr/bin/env python3
"""Validate archived V13 rejection evidence without executing archived code."""
import gzip, hashlib, json, re, tarfile
from pathlib import Path, PurePosixPath

def sha(b): return hashlib.sha256(b).hexdigest()
d=Path(__file__).resolve().parent
files={}
with tarfile.open(d/'evidence.tar.gz') as tar:
 for member in tar:
  name=PurePosixPath(member.name)
  assert member.isfile() and not name.is_absolute() and '..' not in name.parts and member.name not in files
  files[member.name]=tar.extractfile(member).read()
index=json.loads(gzip.decompress((d/'contents.json.gz').read_bytes()))
assert files.keys()==index.keys()
for name,data in files.items(): assert index[name]=={'bytes':len(data),'sha256':sha(data)}
complete=json.loads(files['combined-correctness/v13/completed.json'])
assert complete['exit_code']==0 and complete['combined_inputs_sha256']=='2a6e0b1d940fc74ba24745a3a5054a5b753974395473ad9b89219c4eeac56e6a'
assert sha(files['combined-v13/inputs.sha256'])==complete['combined_inputs_sha256']
assert (complete['fleet_languages'],complete['fleet_route_runs'],complete['c_fixture_checks'],complete['oracle_matching_cases'],complete['memory_test_count'])==(206,412,1236,127,27)
for name,digest in complete['files_sha256'].items(): assert sha(files['combined-correctness/v13/'+name])==digest,name
counter=json.loads(files['out-combined-v13-counters/summary.json'])
assert counter['languages']==206 and counter['routes_per_language']==2 and counter['ledger_exports_byte_identical'] and counter['work_and_reuse_direction_passed']
bench=files['out-combined-v13-perf-v4/benchstat.txt'].decode()
assert '+16.67%' in bench and '+440.93%' in bench and '-25.57%' in bench
for folder in ('out-combined-v13-perf-v4','out-combined-v13-yaml'):
 arms=[]
 for arm in ('before','after'):
  raw=files[folder+'/'+arm+'.txt'].decode(); rows=re.findall(r'# seed: (\d+); position: (\d+)',raw)
  assert len(rows)==20 and raw.count('\nPASS\n')==20
  arms.append({int(s):int(p) for s,p in rows})
 assert arms[0].keys()==arms[1].keys() and all(arms[0][s]+arms[1][s]==3 for s in arms[0])
for name in files:
 if name.startswith('container-states/'):
  state=json.loads(files[name]); assert state['state']['ExitCode']==0 and not state['state']['OOMKilled']
print(json.dumps({'validated':True,'status':'rejected','members':len(files),'fleet_routes':412,'c_checks':1236,'go_paired_seeds':20,'yaml_paired_seeds':20,'full_report_allocations_change_percent':16.67},indent=2))
