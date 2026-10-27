#!/usr/bin/env python3
"""Validate CI-repair and historical V28 ARM evidence without running code."""
import gzip,hashlib,io,json,re,statistics,tarfile
from pathlib import Path,PurePosixPath
sha=lambda b:hashlib.sha256(b).hexdigest()
d=Path(__file__).resolve().parent; files={}
with tarfile.open(d/'evidence.tar.gz') as t:
 for m in t:
  p=PurePosixPath(m.name); assert m.isfile() and not p.is_absolute() and '..' not in p.parts and m.name not in files
  files[m.name]=t.extractfile(m).read()
index=json.loads(gzip.decompress((d/'contents.json.gz').read_bytes())); assert files.keys()==index.keys()
for n,b in files.items(): assert index[n]=={'bytes':len(b),'sha256':sha(b)}
assert sha(files['combined-repair/source.sha256'])=='b0412e082a458f46eb7c44df76f095dc54c349d44ac49cecb1ffbc504507e662'
c=json.loads(files['out-combined-repair-counters/summary.json']); assert c['languages']==206 and c['routes_per_language']==2 and c['ledger_exports_byte_identical'] and c['work_and_reuse_direction_passed'] and not c['changes']
raw=files['combined-repair/observer-focused.log'].decode(); steps=[json.loads(s.split('TRUSTED_GO_SESSION_STEP ',1)[1]) for s in raw.splitlines() if 'TRUSTED_GO_SESSION_STEP ' in s]; assert len(steps)==144 and all(x['VerifierEvents']==0 and x['BypassEvents']==1 and x['NoEditAllocs']==0 and x['BypassCertified'] and not x['FreshVerified'] for x in steps)
assert all([x['Step'] for x in steps if x['API']==api]==list(range(72)) for api in ('plain','profiled'))
assert re.search(r'^PASS$',raw,re.M)
orders=[]
for arm in ('before','after'):
 raw=files['out-combined-repair-perf/'+arm+'.txt'].decode(); pairs=re.findall(r'# seed: (\d+); position: (\d+)',raw); assert len(pairs)==20 and raw.count('\nPASS\n')==20; orders.append({int(s):int(p) for s,p in pairs})
assert orders[0].keys()==orders[1].keys() and all(orders[0][s]+orders[1][s]==3 for s in orders[0])
bench=files['out-combined-repair-perf/benchstat.txt'].decode(); assert '+4.38%' in bench and '+2.32%' in bench
assert files['test-fix/changed-paths.txt']==b'M\tincremental_test.go\n'
assert sha(files['test-fix/go122-vet-test.log'])=='30d40a919bdd84ebff8e75d96e1803ef62e9ed135942310bcb3bdd149aa4ea95'
fixlog=files['test-fix/go122-vet-test.log'].decode(); assert 'go1.22.0 linux/amd64' in fixlog and fixlog.count('\nPASS\n')==2
for name in ('TestLanguageBlobRoundTripDoesNotTransferAuthority','TestExistingGoBlobRequiresProfileForReuseAuthority','TestLanguageCopyDoesNotAuthenticate','TestScannerAdaptationRevokesProducer','TestAuthenticatedNativeReuseWitness'):
 assert '--- PASS: '+name in fixlog
fix2=files['test-fix-2/ci-pruning-layout-final.log']; assert sha(fix2)=='13c7395d5c41603bb207df697530bb4015c6b960cbd33e4eb1caef57c1cad87c'
assert b'--- PASS: TestPruningCertificateDiscards' in fix2 and b'--- PASS: TestRecoveryMemoTelemetryPreservesAMD64HotLayouts' in fix2
arm=json.loads(files['arm-v28-summary.json']); assert arm['exit_code']==0 and not arm['oom_killed'] and sha(files['arm-v28-evidence.tar.gz'])==arm['archive_sha256']
with tarfile.open(fileobj=io.BytesIO(files['arm-v28-evidence.tar.gz'])) as t:
 for role,expected in (('baseline',10),('candidate',15)):
  rows=[json.load(t.extractfile(f'evidence/rss/{role}-{s}.json')) for s in range(154001,154021)]; assert all(r['exit_code']==0 and not r['manual_GC'] for r in rows); vals=[r['max_rss_bytes'] for r in rows]; assert sum(v>419436400 for v in vals)==expected and statistics.median(vals)==arm['arms'][role]['median_bytes']
 for n in ('source-before','source-after','controls-before','controls-after'):
  b=t.extractfile('evidence/'+n+'.log').read(); expected=4466 if n.startswith('source') else 25
  assert b'FAILED' not in b and sum(line.endswith(b': OK') for line in b.splitlines())==expected
 assert b'go1.25.14 linux/arm64' in t.extractfile('evidence/go-version.txt').read()
 assert b'76c70b62127085d3c791ba98a24cba9f22bd343e40afb43522bbb5041cede524' in t.extractfile('evidence/fixture.log').read()
 for n,tests in (('focused.log',('TestAuthenticatedNativeReuseWitness','TestLanguageCopyDoesNotAuthenticate','TestScannerAdaptationRevokesProducer','TestPrimitiveProbeIntoContract','TestTransientIdentitySetBudgetAndRetention')),('incr-focused.log',('TestIdentitySetCollisionsGrowthAndClearing','TestGroupedReadProducerAuthorityLifecycle','TestEightPageReservationChecks32BitSliceArithmetic'))):
  log=t.extractfile('evidence/'+n).read().decode()
  for test in tests: assert '--- PASS: '+test in log
print(json.dumps({'validated':True,'members':len(files),'repair_paired_seeds':20,'actual_bypass_steps':144,'arm_v28_above_ceiling':{'baseline':10,'candidate':15},'main_merge_held':True},indent=2))
