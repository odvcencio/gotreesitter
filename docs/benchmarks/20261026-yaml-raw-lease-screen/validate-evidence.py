#!/usr/bin/env python3
"""Check the held YAML raw-lease receipt without executing archived code."""
from pathlib import Path, PurePosixPath
import argparse, gzip, hashlib, json, re, statistics, tarfile

BASE_SHA = '73fe52b4ef05a02a6741265da9b447ed3e2610338880592a6d1ff57c5f8c90fa'
def sha(data): return hashlib.sha256(data).hexdigest()
def archive(path):
    files = {}
    with tarfile.open(path) as tar:
        for member in tar:
            name = PurePosixPath(member.name)
            assert member.isfile() and not name.is_absolute() and '..' not in name.parts
            assert member.name not in files
            files[member.name] = tar.extractfile(member).read()
    return files

def validate(directory, source_root=None):
    base = directory.parent/'20261025-compact-raw-screen/evidence.tar.gz'
    assert sha(base.read_bytes()) == BASE_SHA
    inherited, delta = archive(base), archive(directory/'evidence.tar.gz')
    index = json.loads(gzip.decompress((directory/'contents.json.gz').read_bytes()))
    assert len(index) == len(delta) and {row['path'] for row in index} == delta.keys()
    for row in index:
        data = delta[row['path']]
        assert len(data) == row['bytes'] and sha(data) == row['sha256']
    files = dict(inherited)
    for name, data in delta.items():
        assert name not in files or files[name] == data
        files[name] = data
    def obj(name): return json.loads(files[name])
    def out(stage, name): return files[f'outputs/{stage}/{name}'].decode()
    def resolve(name):
        return 'outputs/'+name.removeprefix('/controls/out/') if name.startswith('/controls/out/') else name.lstrip('/')
    checks = 0
    for name, data in delta.items():
        if name.startswith('controls/') and PurePosixPath(name).name in ('controls.sha256','checksums.container.sha256'):
            for line in data.decode().splitlines():
                expected, path = line.split('  ', 1)
                if path.startswith('/controls/'):
                    assert sha(files[resolve(path)]) == expected, (name,path)
                    checks += 1
    def stage(name, logs, code=0):
        assert json.loads(out(name,'finished.json'))['exit_code'] == code, name
        for guard in ('source-before.log','source-after.log','controls-before.log','controls-after.log'):
            assert out(name,guard) == '', (name,guard)
        for log, (top, total) in logs.items():
            text = out(name,log)
            assert len(re.findall(r'^--- PASS:',text,re.M)) == top, (name,log)
            assert len(re.findall(r'^\s*--- PASS:',text,re.M)) == total, (name,log)
            assert not re.search(r'--- (FAIL|SKIP):|^FAIL\b|WARNING: DATA RACE|panic:',text,re.M)
    final = 'raw-summary-yaml-replay-v32'
    stage(final, {'scanner.log':(5,10),'core-release.log':(93,205),'core-workcount.log':(29,66),'core-race.log':(29,66),'value-identity.log':(48,72),'yaml-lifetime.log':(5,10),'yaml-authority-budget.log':(17,41),'go-preserved.log':(1,3),'yaml-race.log':(13,45),'invariants-yaml-0.log':(1,1),'invariants-yaml-1.log':(1,1),'yaml-raw-locked-c.log':(2,6),'yaml-existing-locked-c.log':(8,186),'reference-formation-work.log':(1,1),'candidate-formation-work.log':(1,1)})
    for route in (0,1): assert 'session_steps=72 no_edit_allocs=0' in out(final,f'invariants-yaml-{route}.log')
    state = out(final,'yaml-lifetime.log')
    assert 'records=231 state_known=230 pre_known=209 state_unknown=1 pre_unknown=22' in state and 'unproved=0' in state
    assert 'raw=296 public=231 checkpoints=105 provenance=109 cp_bytes=16384 owned=true public_cp_complete=false' in state
    census = out('raw-summary-yaml-all-state-diagnostic-v24','diagnosis.log')
    assert 'records231 nonextra_pre16 extra_pre21 post0 private_production1 public_structure_fields_queries_equal=true' in census
    # Both older ownership interpretations are preserved, with the corrected
    # contract checked alongside the original unchanged baseline assertion.
    assert json.loads(out('raw-summary-yaml-full-v25','finished.json'))['exit_code'] == 1
    assert 'published scanner transfer lost its proof' in out('raw-summary-yaml-full-v25','yaml-existing-locked-c.log')
    assert json.loads(out('raw-summary-yaml-ownership-v26','finished.json'))['exit_code'] == 1
    assert 'native projection would hide a scanner checkpoint' in out('raw-summary-yaml-ownership-v26','yaml-existing-locked-c.log')
    stage('raw-summary-yaml-ownership-v27',{'baseline-original-ownership.log':(1,1),'yaml-existing-locked-c.log':(8,186),'yaml-raw-locked-c.log':(1,3)})
    cstage = 'yaml-c027-profile-v33'
    stage(cstage,{'runtime-probe.log':(1,1),'yaml-raw-c027.log':(2,6),'yaml-existing-c027.log':(8,186),'identity-compatibility.log':(17,17)})
    for label in ('oracle-before','oracle-after','binding-before','binding-after'): assert out(cstage,label+'.log') == ''
    assert '"runtime_commit":"10eb5ac119eec9cb4267a8e035066473983f5f3a"' in out(cstage,'runtime-probe.log')
    assert out(cstage,'yaml-raw-c027.log').count('actual_lease=true public_checkpoint_transfer=false') == 2
    def digests(text): return re.findall(r'deep digest ([0-9a-f]{64})',text)
    assert digests(out(cstage,'yaml-raw-c027.log')) == digests(out(final,'yaml-raw-locked-c.log'))
    assert digests(out(cstage,'yaml-existing-c027.log')) == digests(out(final,'yaml-existing-locked-c.log'))
    # C027 v30 stopped at a missing checksum file before test compilation.
    assert json.loads(out('yaml-c027-profile-v30','finished.json'))['exit_code'] == 1
    assert 'controls.sha256: No such file' in out('yaml-c027-profile-v30','controls-before.log')
    assert 'outputs/yaml-c027-profile-v30/cgo-build.log' not in files
    summary = json.loads((directory/'summary.json').read_text())
    for route in ('default','candidate'):
        a=json.loads(out(final,f'baseline-{route}-yaml.json')); b=json.loads(out(final,f'candidate-{route}-yaml.json'))
        if route == 'candidate':
            assert a[0]['full']['new_nodes'] == a[0]['edits'][0]['counters']['new_nodes'] == 232
            assert b[0]['full']['new_nodes'] == b[0]['edits'][0]['counters']['new_nodes'] == 1
            a[0]['full']['new_nodes'] = a[0]['edits'][0]['counters']['new_nodes'] = 1
        assert a == b
    work={}
    for arm in ('reference','candidate'):
        work[arm]=[json.loads(line.split('YAML_FORMATION_WORK ',1)[1]) for line in out(final,f'{arm}-formation-work.log').splitlines() if 'YAML_FORMATION_WORK ' in line]
        assert [row['Bytes'] for row in work[arm]] == [1998,8012,32048]
        for row in work[arm]: assert row['After']==row['Warm'] and row['Before']['Formation']==row['After']['Formation']
    changes=[]
    for before,after in zip(work['reference'],work['candidate']):
        delta_work={k:{'before':v,'after':after['Before']['Formation'][k]} for k,v in before['Before']['Formation'].items() if v!=after['Before']['Formation'][k]}
        assert set(delta_work)=={'FormationRawEdges','FormationLocalRootRefs','FormationHiddenSteps'}
        assert all(v['after']<v['before'] for v in delta_work.values())
        changes.append({'bytes':before['Bytes'],'changes':delta_work})
        for row in (before,after):
            for point in ('Before','After','Warm'): del row[point]['Formation']
        assert before == after
    assert changes == summary['gate']['formation_changes']
    runtime = obj(summary['runtime_map'])['Replace']
    assert sha(files[summary['runtime_map']]) == summary['runtime_sha256'] and len(runtime)==35
    timing = {}
    names = ['BenchmarkCompactGraduationR4/legacy','BenchmarkCompactGraduationR4/compact']
    for campaign in ('yaml-raw-perf-v1','yaml-raw-perf-v2','yaml-raw-perf-v3'):
        stage(campaign,{})
        assert 'matched YAML shuffle pairs complete' in out(campaign,'completed.txt')
        arms={};orders={}
        for arm in ('before','after'):
            text=out(campaign,f'r4-{arm}-yaml.txt')
            for header in ('# protocol: paired-alternating-seeds','# seeds: 1..20','# benchtime: 750ms','# GOMAXPROCS: 1','# count per process: 1','# build tags: gts_parsercorephase0'): assert header in text
            assert text.rstrip().endswith('# status: complete')
            assert [int(x) for x in re.findall(r'^# completed seed: (\d+);',text,re.M)] == list(range(1,21))
            starts=list(re.finditer(r'^# seed: (\d+); position: (\d+)\n',text,re.M));assert len(starts)==20
            rows={};order=[]
            for i,m in enumerate(starts):
                seed,position=map(int,m.groups());assert seed==i+1
                assert position == ((1 if seed%2 else 2) if arm=='before' else (2 if seed%2 else 1))
                chunk=text[m.end():starts[i+1].start() if i+1<20 else len(text)]
                sequence=re.findall(r'^# workload order: (.+)',chunk,re.M);assert len(sequence)==1 and sorted(sequence[0].split(','))==sorted(names);order+=sequence
                points={}
                for line in chunk.splitlines():
                    if not line.startswith('Benchmark'):continue
                    name=line.split()[0];assert name in names and name not in points
                    values={unit:float(v) for v,unit in re.findall(r'([0-9.]+) (ns/op|B/op|allocs/op|compact-routes/op|compact-fallbacks/op)',line)}
                    assert values['compact-fallbacks/op']==0 and values['compact-routes/op']==int(name.endswith('/compact'))
                    points[name]=values
                assert set(points)==set(names);rows[seed]=points
            arms[arm]=rows;orders[arm]=order
        assert orders['before']==orders['after']
        saved=summary['screens'][campaign]['results']['yaml-r4']['results']
        for name in names:
            for metric in ('ns/op','B/op','allocs/op'):
                for arm in ('before','after'):
                    assert saved[name][metric][arm+'_median']==statistics.median(arms[arm][s][name][metric] for s in range(1,21))
        timing[campaign]=saved
    timed=obj('controls/yaml-raw-perf-v3/overlay.container.json')['Replace']
    assert all(timed[k]==v for k,v in runtime.items()) and len(timed)==len(runtime)+2
    assert timed['/baseline/benchmark_admission_candidate_route_test.go']==timed['/candidate/benchmark_admission_candidate_route_test.go']
    verified_source=0
    if source_root:
        for line in files['controls/yaml-raw-perf-v3/source.sha256'].decode().splitlines():
            digest,name=line.split('  ',1);path=PurePosixPath(name);assert not path.is_absolute() and '..' not in path.parts
            assert sha((Path(source_root)/name).read_bytes()) == digest, name
            verified_source+=1
    assert summary['archive_members']==len(delta) and not summary['production_promoted'] and not summary['graduated'] and summary['merges_held']
    return {'schema':'gts-yaml-raw-lease-verification/v1','archive_members':len(delta),'inherited_members':len(inherited),'checksum_bindings':checks,'canonical_source_files':verified_source,'actual_lease_bytes':[1998,8012,32048],'actual_state_proof':{'records':231,'known_post':230,'known_pre':209,'unproved':0},'D8_steps':144,'no_edit_allocations':0,'C027_exact_final_source':True,'work_changes':changes,'pairs':60,'processes':120,'results':timing,'production_promoted':False,'graduated':False,'merges_held':True}

if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('--source-root',type=Path);args=parser.parse_args()
    print(json.dumps(validate(Path(__file__).resolve().parent,args.source_root),indent=2))
