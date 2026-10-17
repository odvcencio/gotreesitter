from pathlib import Path
from statistics import median
import hashlib,json,os,random,re,subprocess
p=Path('/evidence');f=p/'fleet';out=p/'rss-matched'
assert (f/'complete.txt').read_text().strip()=='complete'
assert os.environ.get('GOMAXPROCS')=='1'
out.mkdir(exist_ok=False)
def read_row(log,rss,role,case,stage,seed=None,position=None):
 text=log.read_text();assert '--- PASS:' in text and '--- SKIP:' not in text
 size=int(re.search(r'bytes=(\d+)',text)[1]);peak=int(re.search(r'Maximum resident set size \(kbytes\): (\d+)',rss.read_text())[1])*1024
 assert size==(1048591 if case=='go' else 1049572)
 assert 'stopReason=accepted' in text
 route='compact=0 fallback=1' if case=='yaml_ordinary' else 'compact=1 fallback=0'
 assert route in text
 if case=='go':assert 'nodes=903086/' in text and 'tokens=425742 ' in text
 return dict(role=role,case=case,stage=stage,seed=seed,position=position,input_bytes=size,peak_rss_bytes=peak,rss_bytes_per_source_byte=peak/size)
initial=[read_row(f/f'{role}-standalone-{case}.log',f/f'{role}-standalone-{case}.rss',role,case,'initial') for role in ['baseline','candidate'] for case in ['go','yaml_ordinary','yaml_owned']]
pins={role:hashlib.sha256((f/f'{role}-rss-root.test').read_bytes()).hexdigest() for role in ['baseline','candidate']}
rows=[]
for seed in range(31001,31021):
 cases=['go','yaml_ordinary','yaml_owned'];random.Random(seed).shuffle(cases)
 order=['baseline','candidate'] if seed%2 else ['candidate','baseline']
 for case in cases:
  for position,role in enumerate(order,1):
   log=out/f'{seed}-{case}-{role}.log';rss=out/f'{seed}-{case}-{role}.rss'
   args=['/usr/bin/time','-v','-o',str(rss),str(f/f'{role}-rss-root.test'),'-test.run',f'^TestCompactLineageRSS$/^{case}$','-test.count=1','-test.v',f'-test.shuffle={seed}','-test.timeout=4m']
   with log.open('w') as stream:subprocess.run(args,cwd='/baseline' if role=='baseline' else '/workspace',check=True,stdout=stream,stderr=subprocess.STDOUT)
   row=read_row(log,rss,role,case,'matched',seed,position);rows.append(row)
   print(seed,case,role,round(row['rss_bytes_per_source_byte'],3),flush=True)
   (out/'rows.json').write_text(json.dumps(rows,indent=2)+'\n')
assert all(hashlib.sha256((f/f'{role}-rss-root.test').read_bytes()).hexdigest()==h for role,h in pins.items())
summary={};ratios={}
for case in ['go','yaml_ordinary','yaml_owned']:
 summary[case]={}
 for role in ['baseline','candidate']:
  v=[x['rss_bytes_per_source_byte'] for x in rows if x['case']==case and x['role']==role]
  assert len(v)==20
  summary[case][role]=dict(count=len(v),median=median(v),maximum=max(v),ceiling_failures=sum(x>400 for x in v))
 ratios[case]=summary[case]['candidate']['median']/summary[case]['baseline']['median']
failures=[x for x in initial+rows if x['role']=='candidate' and x['rss_bytes_per_source_byte']>400]
report=dict(protocol='20 matched alternating pairs per case; case order shuffled per explicit seed; one grammar/process; safety probes only, no timing claim',seeds='31001..31020',binary_sha256=pins,environment={k:os.environ.get(k) for k in ['GOMAXPROCS','GOMEMLIMIT','GOGC','GODEBUG','GOT_PARSE_NODE_LIMIT_SCALE','GOT_GLR_MAX_STACKS']},initial=initial,matched=rows,summary=summary,candidate_baseline_median_ratios=ratios,candidate_ceiling_failures=failures,all_initial_probes_retained=True,candidate_safety_pass=not failures and all(v<=1.10 for v in ratios.values()))
(out/'audit.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps({k:v for k,v in report.items() if k not in ['initial','matched']},indent=2),flush=True)
if not report['candidate_safety_pass']:raise SystemExit('candidate RSS ceiling or median ratchet failed; retain all probes and do not promote')
(out/'complete.txt').write_text('complete\n')
