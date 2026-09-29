#!/usr/bin/env python3
"""Checks for the receipt-only oracle audit; no parser execution is needed."""

import copy
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

SCRIPT = Path(__file__).with_name("audit_grammar_receipt_oracles.py")
SPEC = importlib.util.spec_from_file_location("receipt_oracle_audit", SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def receipt():
    return {
        "schema": "gts-grammar-receipt/v1",
        "grammar": {"name": "sample", "blob_sha256": "a" * 64},
        "gotreesitter_commit": "b" * 40, "git_tree_dirty": False,
        "c_oracle": {"runtime_commit": "c" * 40},
        "corpus": {"manifest_sha256": "d" * 64},
        "incremental_parity": {"status": "fail", "steps": [{
            "step": 1, "edit_class": "insert", "site": "sample:site-00",
            "source_sha256": "e" * 64,
            "c_incremental_tree_sha256": "1" * 64,
            "c_fresh_tree_sha256": "2" * 64,
            "go_incremental_tree_sha256": "2" * 64,
            "go_fresh_tree_sha256": "2" * 64, "pass": False,
        }]},
    }


class OracleAuditTest(unittest.TestCase):
    def test_reports_incompatible_oracles_without_changing_a_verdict(self):
        original = receipt()
        before = copy.deepcopy(original)
        report = MODULE.audit([original])
        self.assertEqual(report["counts"]["c_incremental_differs_from_fresh"], 1)
        self.assertEqual(report["counts"]["go_incremental_equals_go_fresh_equals_c_fresh"], 1)
        self.assertFalse(report["witnesses"][0]["recorded_step_pass"])
        self.assertEqual(original, before)

    def test_missing_digests_are_unmeasured_rather_than_disagreements(self):
        original = receipt()
        del original["incremental_parity"]["steps"][0]["c_incremental_tree_sha256"]
        report = MODULE.audit([original])
        self.assertEqual(report["counts"]["unmeasured_c_steps"], 1)
        self.assertEqual(report["counts"]["c_incremental_differs_from_fresh"], 0)
        self.assertEqual(report["witnesses"], [])

    def test_equal_c_digests_ignore_a_stale_summary_boolean(self):
        original = receipt()
        step = original["incremental_parity"]["steps"][0]
        step["c_incremental_tree_sha256"] = step["c_fresh_tree_sha256"]
        step["c_incremental_equals_fresh"] = False
        report = MODULE.audit([original])
        self.assertEqual(report["counts"]["c_incremental_equals_fresh"], 1)
        self.assertEqual(report["witnesses"], [])

    def test_duplicate_evidence_does_not_inflate_counts(self):
        with self.assertRaisesRegex(ValueError, "duplicate receipt"):
            MODULE.audit([receipt(), receipt()])

    def test_cli_preserves_inputs_and_refuses_to_overwrite_evidence(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / "sample.json"
            source.write_text(json.dumps(receipt()))
            before = source.read_bytes()
            result = subprocess.run([sys.executable, str(SCRIPT), "--report-dir", str(root)],
                                    capture_output=True, text=True, check=True)
            self.assertEqual(json.loads(result.stdout)["counts"]["receipts"], 1)
            rejected = subprocess.run([sys.executable, str(SCRIPT), "--report-dir", str(root),
                                       "--out", str(source)], capture_output=True, text=True)
            self.assertNotEqual(rejected.returncode, 0)
            self.assertEqual(source.read_bytes(), before)


if __name__ == "__main__":
    unittest.main()
