from pathlib import Path
import hashlib,json,re,statistics,sys,tarfile
p=Path(sys.argv[1])
r=p/'mac-retrieved'
pin=json.loads((p/'receipt-archive.json').read_text())
archive=p/'receipts.tar.gz'
assert hashlib.sha256(archive.read_bytes()).hexdigest()==pin['sha256'] and archive.stat().st_size==pin['bytes']
m=json.loads((r/'receipt-manifest.json').read_text())
for name,v in m.items():
 f=r/name
 assert hashlib.sha256(f.read_bytes()).hexdigest()==v['sha256'] and f.stat().st_size==v['bytes'],name
assert len(m)==pin['manifest_members']
base=json.loads((r/'baseline-ledger.json').read_text())
assert base==json.loads((r/'candidate-ledger.json').read_text()) and len(base['rows'])==412
main=json.loads((r/'main-source-audit.json').read_text())
assert main['main_go_files']==main['snapshot_go_files']==3541 and main['snapshot_differences']==[]
initial=json.loads((r/'parent-audit.json').read_text())
assert all(initial[k] for k in ['original_unchanged','baseline_unchanged','candidate_unchanged','checkout_unchanged'])
gate=json.loads((r/'full-gates/gate-audit.json').read_text())
assert gate['passed'] and gate['locked_C_checks']==1236 and gate['invariant_sessions']==412 and gate['edit_steps']==29664 and gate['no_edit_allocations']==0 and gate['memory_contracts']==20 and gate['root_race_cases']==60
final=json.loads((r/'paired-perf/parent-audit.json').read_text())
assert final['exit_code']==0 and all(final[k] for k in ['baseline_original_unchanged','candidate_original_unchanged','baseline_bench_source_unchanged','candidate_bench_source_unchanged','checkout_unchanged'])
for k in ['cgo_state','perf_state']:
 assert final[k]['ExitCode']==0 and not final[k]['OOMKilled'] and not final[k]['Running']
for lang in ['go','yaml']:
 a=json.loads((r/('paired-perf/perf/'+lang+'-pair-audit.json')).read_text())
 assert a['pairing_passed'] and a['all_median_ratchets_pass'] and a['all_reported_work_metrics_equal']
c=json.loads((r/'paired-perf/cgo/gate-audit.json').read_text())
assert c['baseline_candidate_digest_and_counter_pairs_equal'] and c['clean_report_C_leaves_per_variant']==8 and c['report_D8_leaves_per_variant']==12 and c['known_delete_C_failures_per_variant']==4 and c['delete_C_is_not_a_pass'] and c['numeric_replacements']==12
work={}
rss={}
for role in ['baseline','candidate']:
 values=[];work[role]={}
 for seed in range(154001,154021):
  f=r/('paired-perf/perf/rss/%s-%d.json'%(role,seed))
  record=json.loads(f.read_text())
  assert record['exit_code']==0 and record['compiler_outside_measured_process'] and not record['manual_GC']
  argv=record['argv']
  assert '-test.benchtime=2x' in argv and '-test.count=1' in argv and '-test.shuffle=%d'%seed in argv
  text=f.with_suffix('.log').read_text()
  lines=[line for line in text.splitlines() if line.startswith('BenchmarkFreshMemory/go/1024KiB')]
  assert len(lines)==1 and re.search(r'^PASS$',text,re.M) and not re.search(r'^FAIL',text,re.M)
  parts=lines[0].split()
  assert parts[1]=='2' and (len(parts)-2)%2==0
  metrics={parts[i+1]:float(parts[i]) for i in range(2,len(parts),2)}
  selected={k:metrics[k] for k in ['nodes/op','tokens/op','stacks/op']}
  assert selected=={'nodes/op':993391.,'tokens/op':425742.,'stacks/op':2.}
  work[role][str(seed)]=selected
  values.append(record['max_rss_bytes'])
 rss[role]={'values':values,'median_bytes':statistics.median(values),'maximum_bytes':max(values),'above_400_gate':sum(v>400*1048591 for v in values)}
assert work['baseline']==work['candidate']
ratio=rss['candidate']['median_bytes']/rss['baseline']['median_bytes']
assert ratio<=1.10
rss_audit={'fixed_iterations':2,'pairs':20,'source_bytes':1048591,'source_sha256':'76c70b62127085d3c791ba98a24cba9f22bd343e40afb43522bbb5041cede524','reported_work_metrics_equal':True,'reported_work_metrics':work,'measurement':rss,'median_RSS_ratio':ratio,'inside_10pct_ratchet':True,'absolute_400_gate_passed':rss['candidate']['above_400_gate']==0,'no_manual_GC':True,'compiler_outside_measured_process':True,'throughput_claim':False}
(p/'rss-work-audit.json').write_text(json.dumps(rss_audit,indent=2)+'\n')
audit={'archive_sha256':pin['sha256'],'archive_bytes':pin['bytes'],'all_member_hashes_verified':len(m),'main_sha':main['main_sha'],'main_source_files_authenticated':3541,'ledger_rows_equal':412,'all_source_post_audits_pass':True,'all_median_perf_ratchets_pass':True,'known_delete_C_gap_retained':True,'reported_RSS_work_metrics_equal':True,'RSS_median_ratio':ratio,'large_file_RSS_qualified':rss_audit['absolute_400_gate_passed'],'draft_and_RSS_hold_remain':True,'exact_head_CI_still_required':True}
(p/'collection-audit.json').write_text(json.dumps(audit,indent=2)+'\n')
print(json.dumps(audit))
