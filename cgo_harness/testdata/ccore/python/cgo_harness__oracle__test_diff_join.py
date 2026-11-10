import json, pathlib, subprocess, sys, tempfile, unittest
SCRIPT = pathlib.Path(__file__).with_name('diff_join.py')
class JoinTests(unittest.TestCase):
 def run_join(self,a,b):
  with tempfile.TemporaryDirectory() as d:
   p=pathlib.Path(d)
   for n,rows in [('a',a),('b',b)]: (p/n).write_text(''.join(json.dumps(r)+'\n' for r in rows))
   r=subprocess.run([sys.executable,str(SCRIPT),str(p/'a'),str(p/'b'),str(p/'out'),str(p/'summary')],capture_output=True,text=True)
   return r, json.loads((p/'summary').read_text()) if (p/'summary').exists() else None
 def row(self,**kw):
  return dict(grammar='go', **{'class':'smoke'}, id='x', input_sha256='abc', c_digest='c', go_digest='c', go_equals_c=True, c_sexp_sha256='s', **kw)
 def test_duplicate_rejected(self):
  r,_=self.run_join([self.row(),self.row()],[self.row()]); self.assertNotEqual(r.returncode,0)
 def test_changed_input_rejected(self):
  b=self.row(); b['input_sha256']='def'; r,_=self.run_join([self.row()],[b]); self.assertNotEqual(r.returncode,0)
 def test_failed_side_not_parity_or_change(self):
  b=self.row(); b.update(c_digest='',c_error='timeout',go_equals_c=False)
  r,s=self.run_join([self.row()],[b]); self.assertEqual(r.returncode,0); self.assertEqual(s['parity'],{'unavailable':1}); self.assertEqual(s['c_digest_changed'],0)
 def test_valid_difference(self):
  b=self.row(); b.update(c_digest='d',go_equals_c=False)
  r,s=self.run_join([self.row()],[b]); self.assertEqual(r.returncode,0); self.assertEqual(s['parity'],{'newly_differing':1}); self.assertEqual(s['c_span_or_flag_only_change'],1)
if __name__=='__main__': unittest.main()
