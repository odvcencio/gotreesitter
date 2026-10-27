#!/usr/bin/env python3
import hashlib,io,json,tarfile,zipfile
from pathlib import Path,PurePosixPath
p=Path(__file__).resolve().parent; sha=lambda b:hashlib.sha256(b).hexdigest(); files={}
with tarfile.open(p/'w5-go-undo-evidence.tar.gz') as t:
 for m in t:
  n=PurePosixPath(m.name); assert m.isfile() and not n.is_absolute() and '..' not in n.parts and m.name not in files
  files[m.name]=t.extractfile(m).read()
idx=json.loads((p/'w5-go-undo-index.json').read_text()); assert files.keys()==idx.keys()
for n,b in files.items(): assert idx[n]=={'bytes':len(b),'sha256':sha(b)}
assert sha(files['ci-artifact.zip'])=='98020ed133aabab1369edb4df4f69719f2adbcfbfef4a2a09e1c08ec59ce914f'
with zipfile.ZipFile(io.BytesIO(files['ci-artifact.zip'])) as z:
 assert sha(z.read('fixtures/go'))=='ff2f811d29a56303bad96c3c7cff1a83814a974503a51cee918a314a96eb476e'
 x=json.loads(z.read('counters.json')); assert not x['passed']; rows=[r for r in x['rows'] if r['language']=='go' and r['workload']=='one_byte' and r['step']==1]; v={r['counter']:(r['before'],r['after']) for r in rows}
 assert v=={'tokens':(22393,22385),'nodes':(109018,98946),'max_stacks':(18,9),'reused_subtrees':(39,35),'reused_bytes':(2816,2853)}
 assert b'--- PASS: TestW5RealCodeEdits' in z.read('correctness-go.log')
assert sha(files['trace-v3/replay.log'])=='e5bb781e95157620aee1f93c6704026a522780c7f295f13e0ddde0a175beac18'
assert sha(files['strict-v4/replay.log'])=='33c4b137e66c535ae48df0718ee78511ac9658d505917f0f2d5da113c9efa25e'
assert files['strict-v4/replay.log'].count(b'--- PASS: TestW5GoStrictUndo')==6
print('validated: W5 Go undo failure 39 -> 35; owner review pending')
