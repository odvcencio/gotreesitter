import tempfile
import unittest
from pathlib import Path
import w5_private_oracle as guard

class PrivateOracleHelpersTests(unittest.TestCase):
 def test_empty_environment_and_all_source_overrides(self):
  guard.check_env({})
  for key in guard.OVERRIDES:
   with self.subTest(key=key), self.assertRaises(ValueError): guard.check_env({key:'/untrusted'})
 def test_file_digest_rejects_symlink_and_directory(self):
  with tempfile.TemporaryDirectory() as tmp:
   root=Path(tmp); regular=root/'source'; regular.write_text('verified')
   self.assertEqual(len(guard.digest(regular)),64)
   link=root/'link';link.symlink_to(regular)
   for path in (link,root):
    with self.subTest(path=path),self.assertRaises(ValueError):guard.digest(path)
 def test_incomplete_identity_fails_closed(self):
  for identity in ({},{'contract':'tree-sitter-c-v1'}):
   with self.assertRaises(ValueError): guard.identity_fields(identity)
 def test_source_fingerprints_detect_change(self):
  with tempfile.TemporaryDirectory() as tmp:
   path=Path(tmp)/'source';path.write_text('old');before=guard.source_fingerprints([path]);path.write_text('new')
   self.assertNotEqual(before,guard.source_fingerprints([path]))

if __name__=='__main__':unittest.main(verbosity=2)
