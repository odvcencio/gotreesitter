import os,tempfile,unittest
from pathlib import Path
from unittest.mock import patch
import editor_latency_gate as g

class ParentCacheTests(unittest.TestCase):
 def setUp(self):
  self.tmp=tempfile.TemporaryDirectory();self.root=Path(self.tmp.name);self.cache=self.root/'cache';self.cache.mkdir()
  self.src=self.root/'source';self.src.write_text('source');self.art=self.cache/'a.so';self.art.write_text('artifact')
  self.identity=self.root/'identity.json';self.identity.write_text('{}');(self.root/'compiler.before').write_text('/usr/bin/cc '+('c'*64)+'\n/usr/bin/c++ '+('d'*64)+'\n'); (self.root/'compiler.after').write_bytes((self.root/'compiler.before').read_bytes()); (self.root/'environment.before').write_text('f'*64); (self.root/'environment.after').write_text('f'*64)
  self.expected={'sources':g.source_fingerprints([self.src]),'filename':'a.so','artifact':g.digest(self.art),'identity':g.digest(self.identity),'compilers':[['/usr/bin/cc','c'*64],['/usr/bin/c++','d'*64]],'build_env':'f'*64,'image':'image','image_id':'sha256:image'}
  self.docker=patch.object(g.subprocess,'check_output',return_value='sha256:image\n');self.docker.start()
  self.env=patch.dict(os.environ,{},clear=True);self.env.start()
 def tearDown(self): self.env.stop();self.docker.stop();self.tmp.cleanup()
 def verify(self): return g.verify_private_oracle(self.root,[self.src],self.expected)
 def test_old_owner_exception_rejects_new_driver(self):
  with patch.object(g,'git',return_value=g.GO_1512_REVIEWED), patch.object(g.subprocess,'run'), patch.object(g.subprocess,'check_output',side_effect=[b'',b'cgo_harness/editor_latency/editor_latency_test.go\0']):
   with self.assertRaisesRegex(ValueError,'production inputs changed'):
    g.validate_1512_undo_exception(self.root,g.GO_1512_BASE,g.GO_1512_REVIEWED,self.root/'unused')
 def test_clean_private_inputs(self):
  with patch.object(g.subprocess,'check_output',return_value=b''): g.clean_private_inputs(self.root)
  for status in (b' M parser.go', b'?? new.go', b'!! hidden.go'):
   with patch.object(g.subprocess,'check_output',return_value=status):
    with self.assertRaises(ValueError): g.clean_private_inputs(self.root)
 def test_pass(self): self.verify()
 def test_tampered_artifact(self):
  self.art.write_text('changed')
  with self.assertRaises(ValueError): self.verify()
 def test_changed_source(self):
  self.src.write_text('changed')
  with self.assertRaises(ValueError): self.verify()
 def test_changed_compiler_environment(self):
  (self.root/'environment.after').write_text('e'*64)
  with self.assertRaises(ValueError): self.verify()
 def test_changed_cxx_receipt(self):
  (self.root/'compiler.after').write_text('/usr/bin/c++ '+('e'*64))
  with self.assertRaises(ValueError): self.verify()
 def test_changed_identity(self):
  self.identity.write_text('changed')
  with self.assertRaises(ValueError): self.verify()
 def test_symlink_artifact(self):
  self.art.unlink(); self.art.symlink_to(self.src)
  with self.assertRaises(ValueError): self.verify()
 def test_changed_compiler_receipt(self):
  (self.root/'compiler.before').write_text('modified')
  with self.assertRaises(ValueError): self.verify()
 def test_extra_artifact(self):
  (self.cache/'extra').write_text('changed')
  with self.assertRaises(ValueError): self.verify()
 def test_override(self):
  os.environ['GTS_PARITY_REPO_ROOT']='/untrusted'
  with self.assertRaises(ValueError): self.verify()
 def test_changed_image(self):
  self.docker.stop()
  with patch.object(g.subprocess,'check_output',return_value='sha256:other\n'):
   with self.assertRaises(ValueError): self.verify()
  self.docker.start()
if __name__=='__main__': unittest.main(verbosity=2)
