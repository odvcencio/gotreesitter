import copy
import unittest

from seal_grammar_receipts import validate_receipt


class SealReceiptTests(unittest.TestCase):
    def setUp(self):
        self.args = ("go", "b" * 40, "c" * 40, "d" * 64, "a" * 64,
                     ["https://example.invalid/grammar", "e" * 40], "f" * 64)
        self.receipt = {
            "schema": "gts-grammar-receipt/v1",
            "grammar": {"name": "go", "blob_sha256": "a" * 64},
            "gotreesitter_commit": "b" * 40,
            "git_tree_dirty": False,
            "c_oracle": {
                "runtime_version": "0.27.0", "runtime_commit": "c" * 40,
                "sources_manifest_sha256": "d" * 64,
                "grammar_artifact": {"repository": self.args[5][0], "commit": "e" * 40, "sha256": "0" * 64},
            },
            "corpus": {"lock_sha256": "f" * 64, "selection": {"order": "largest", "max_files": 4, "max_file_bytes": 1024}, "files": []},
            "compact_route": {"status": "accepted"},
            "fresh_parity": {"status": "fail"},
            "incremental_parity": {"status": "fail", "reference": "fresh_c", "steps": []},
            "invariant_gate": {"status": "fail"},
        }

    def validate(self, receipt):
        validate_receipt(receipt, *self.args)

    def test_failed_qualification_is_still_complete_evidence(self):
        self.validate(self.receipt)

    def test_rejects_identity_and_workload_drift(self):
        for section, key, value in [
            (None, "git_tree_dirty", True),
            (None, "gotreesitter_commit", "0" * 40),
            ("c_oracle", "runtime_commit", "0" * 40),
            ("c_oracle", "sources_manifest_sha256", "0" * 64),
            ("corpus", "lock_sha256", "0" * 64),
            ("corpus", "selection", {"order": "largest", "max_files": 4, "max_file_bytes": 2048}),
            ("incremental_parity", "reference", None),
        ]:
            with self.subTest(key=key):
                receipt = copy.deepcopy(self.receipt)
                (receipt if section is None else receipt[section])[key] = value
                with self.assertRaises(ValueError):
                    self.validate(receipt)

    def test_preserves_c_disagreement_without_failing_go_step(self):
        step = {"step": 1, "pass": True, "invariant_pass": True,
                "go_incremental_equals_fresh": True, "locked_c_parity": True,
                "c_incremental_equals_fresh": False, "locked_c_incremental_parity": False,
                "c_incremental_tree_sha256": "1" * 64,
                "c_incremental_failure": {"category": "c-incremental-fresh-mismatch"},
                "locked_c_incremental_failure": {"category": "go-c-incremental-mismatch"}}
        self.receipt["incremental_parity"]["steps"] = [step]
        self.validate(self.receipt)
        step["pass"] = False
        with self.assertRaises(ValueError):
            self.validate(self.receipt)

    def test_rejects_lost_c_disagreement_and_incomplete_pass(self):
        self.receipt["incremental_parity"]["status"] = "pass"
        with self.assertRaises(ValueError):
            self.validate(self.receipt)
        self.receipt["incremental_parity"]["status"] = "fail"
        self.receipt["incremental_parity"]["steps"] = [{
            "step": 1, "pass": False, "invariant_pass": False,
            "go_incremental_equals_fresh": False, "locked_c_parity": False,
            "c_incremental_tree_sha256": "1" * 64, "c_incremental_equals_fresh": False,
        }]
        with self.assertRaises(ValueError):
            self.validate(self.receipt)


if __name__ == "__main__":
    unittest.main()
