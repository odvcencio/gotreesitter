from pathlib import Path
import json,re,collections,hashlib
p=Path(__file__).parent;q=p.parent
assert (p/'complete.txt').read_text().strip()=='complete'
for n,h in json.loads((p/'input-pins.json').read_text()).items():assert hashlib.sha256((q/n).read_bytes()).hexdigest()==h,n
langs=(p/'languages.txt').read_text().splitlines();assert len(langs)==len(set(langs))==206
for lang in langs:
 s=(p/f'invariant-{lang}.log').read_text()
 assert '--- PASS: TestV1InvariantGateR4EditSession' in s and 'session_steps=72 no_edit_allocs=0' in s and '--- SKIP:' not in s,lang
 s=(p/f'c-{lang}.log').read_text()
 for name in ['TestParityFreshParse','TestParityIncrementalParse','TestParityHasNoErrors']:
  assert '--- PASS: '+name in s,(lang,name)
 assert '--- SKIP:' not in s,lang
before=json.loads((p/'baseline-strict-census.json').read_text());after=json.loads((p/'candidate-strict-census.json').read_text())
assert len(before)==len(after)==204 and before==after
census=json.loads((p/'canonical-work.json').read_text());assert census['baseline']==census['candidate'] and len(census['candidate'])==4
rss=json.loads((p/'standalone-rss.json').read_text());assert len(rss)==6
for row in rss:assert row['rss_bytes_per_source_byte']<=400,row
for n in ['memory-budgets.log','core-onepass.log','go-invariant-race.log','yaml-invariant-race.log','owned-yaml-invariant-race.log','owned-yaml-invariant.log']:
 s=(p/n).read_text();assert ('\nPASS\n' in s or s.startswith('ok  ')) and '--- SKIP:' not in s,n
for role in ['baseline','candidate']:
 for n in ['go-1m','yaml-ordinary-1m','yaml-owned-1m']:
  s=(p/f'{role}-{n}.log').read_text();assert '--- PASS:' in s and '--- SKIP:' not in s,(role,n)
out={'status':'complete','invariant_sessions':206,'steps_per_session':72,'no_edit_allocations':0,'locked_C_fixture_checks':618,'strict_rows':204,'strict_counts':dict(collections.Counter(r['status'] for r in after)),'actual_compact_C_exact':sum(r['compact']==1 and r['status']=='PASS' for r in after),'passing_legacy_routes':sum(r['compact']==0 and r['status']=='PASS' for r in after),'passing_recorded_fallbacks':sum(r['fallback']>0 and r['status']=='PASS' for r in after),'passing_before_attempt_legacy_routes':sum(r['compact']==0 and r['fallback']==0 and r['status']=='PASS' for r in after),'existing_failing_legacy_routes':sum(r['compact']==0 and r['status']=='FAIL' for r in after),'existing_failing_recorded_fallbacks':sum(r['fallback']>0 and r['status']=='FAIL' for r in after),'existing_failing_before_attempt_legacy_routes':sum(r['compact']==0 and r['fallback']==0 and r['status']=='FAIL' for r in after),'all_strict_rows_exact':True,'canonical_full_work_vectors_and_pinned_deep_digests':4,'standalone_RSS_probes':rss,'large_C_checks':'one-MiB Go, ordinary and owned YAML, baseline and candidate all pass','scope':'all six RSS numbers are standalone safety probes, not statistical RSS improvements; combined C+Go+compatibility-walk process RSS has a different scope','other_gates':['memory budgets','one-pass core tests','Go/YAML/owned-YAML invariant races','owned-YAML 72-step invariant']}
(p/'audit.json').write_text(json.dumps(out,indent=2)+'\n')
print(json.dumps({k:v for k,v in out.items() if k!='standalone_RSS_probes'},indent=2))
