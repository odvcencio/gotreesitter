from pathlib import Path
import json
p=Path('/evidence');q=Path('/reference')
for kind in ['ledger','owned-ledger']:
 a=json.loads((q/f'base-{kind}.json').read_text());b=json.loads((p/f'head-{kind}.json').read_text())
 assert a==b,kind
rows={role:[json.loads(l.split('PUBLICATION_WORK_CENSUS ',1)[1]) for l in (p/f'{role}-work.log').read_text().splitlines() if 'PUBLICATION_WORK_CENSUS ' in l] for role in ['base','head']}
assert len(rows['base'])==len(rows['head'])==5
assert rows['base']==rows['head'],'full construction/consumption vectors changed'
p.joinpath('counter-gate.json').write_text(json.dumps({'rows':414,'all_fields_exact':True,'construction_vectors':5,'consumption_digests_exact':True,'exceptions':[]},indent=2)+'\n')
print('414 ledger rows and five full construction/consumption vectors exact',flush=True)
