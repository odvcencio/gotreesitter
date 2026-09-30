#!/usr/bin/env python3
"""Check receipt planning and coverage without running a parser or Docker."""

import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent


def run_script(name, *args):
    env = dict(os.environ)
    env.pop("GITHUB_OUTPUT", None)
    env.pop("GITHUB_STEP_SUMMARY", None)
    return subprocess.run(
        [sys.executable, str(ROOT / "scripts" / name), *args],
        cwd=ROOT, env=env, text=True, capture_output=True,
    )


class ReceiptPlanningTest(unittest.TestCase):
    def test_nightly_and_release_cover_every_locked_grammar(self):
        locked = [line.split()[0] for line in (ROOT / "grammars/languages.lock").read_text().splitlines()
                  if line.strip() and not line.startswith("#")]
        self.assertEqual(len(locked), 206)
        for event in ("schedule", "release"):
            with self.subTest(event=event):
                result = run_script("plan_grammar_receipts.py", "--event", event)
                self.assertEqual(result.returncode, 0, result.stderr)
                output = dict(line.split("=", 1) for line in result.stdout.splitlines())
                matrix = json.loads(output["matrix"])["include"]
                self.assertEqual([entry["grammar"] for entry in matrix], locked + ["lean"])
                self.assertEqual(output["count"], "207")
                self.assertEqual(len({entry["grammar"] for entry in matrix}), 207)


class ReceiptCoverageTest(unittest.TestCase):
    def test_summary_requires_exactly_the_planned_receipts(self):
        cases = [
            ("complete", [("go", "pass"), ("yaml", "pass")], True, "complete"),
            ("empty", [], False, "missing: go, yaml"),
            ("missing", [("go", "pass")], False, "missing: yaml"),
            ("duplicate", [("go", "pass"), ("go", "pass"), ("yaml", "pass")], False, "duplicates: go"),
            ("unplanned", [("go", "pass"), ("yaml", "pass"), ("css", "pass")], False, "unexpected: css"),
            ("failed parity", [("go", "fail"), ("yaml", "fail")], True, "complete"),
            ("unavailable", [("go", "unavailable"), ("yaml", "unavailable")], True, "complete"),
            ("timeouts", [("go", "timeout"), ("yaml", "timeout")], True, "complete"),
        ]
        matrix = json.dumps({"include": [{"grammar": "go"}, {"grammar": "yaml"}]})
        for name, entries, success, message in cases:
            with self.subTest(name=name), tempfile.TemporaryDirectory() as tmp:
                report_dir = Path(tmp)
                for index, (grammar, status) in enumerate(entries):
                    receipt = {
                        "schema": "gts-grammar-receipt/v1",
                        "grammar": {"name": grammar},
                        "cohort": "1a",
                        "compact_route": {"status": "accepted"},
                        "fresh_parity": {"status": status},
                        "incremental_parity": {"status": status},
                        "invariant_gate": {"status": status},
                        "execution": {"status": "timeout" if status == "timeout" else "completed"},
                    }
                    (report_dir / f"{index}.json").write_text(json.dumps(receipt))
                result = run_script("summarize_grammar_receipts.py", "--report-dir", tmp, "--expected-matrix", matrix)
                self.assertEqual(result.returncode == 0, success, result.stderr)
                summary = (report_dir / "summary.md").read_text()
                self.assertIn(message, summary)
                self.assertIn("target 2", summary)
                self.assertTrue((report_dir / "parity-failures.md").exists())
                metrics = json.loads((report_dir / "run-metrics.json").read_text())
                self.assertEqual(metrics["coverage"]["expected_count"], 2)
                if not success:
                    self.assertIn(message, result.stderr)

    def test_c_incremental_diagnostics_do_not_change_canonical_parity(self):
        with tempfile.TemporaryDirectory() as tmp:
            receipt = {
                "schema": "gts-grammar-receipt/v1", "grammar": {"name": "go"},
                "cohort": "1a", "compact_route": {"status": "accepted"},
                "fresh_parity": {"status": "pass"}, "invariant_gate": {"status": "pass"},
                "incremental_parity": {"status": "pass", "steps": [{
                    "site": "example:site-00", "pass": True,
                    "c_incremental_failure": {"category": "c-incremental-fresh-mismatch", "go_value": "old", "c_value": "fresh"},
                }]},
            }
            (Path(tmp) / "go.json").write_text(json.dumps(receipt))
            matrix = json.dumps({"include": [{"grammar": "go"}]})
            result = run_script("summarize_grammar_receipts.py", "--report-dir", tmp, "--expected-matrix", matrix)
            self.assertEqual(result.returncode, 0, result.stderr)
            summary = (Path(tmp) / "summary.md").read_text()
            self.assertIn("incremental locked-C parity: 1/1 pass", summary)
            self.assertIn("Separate C incremental diagnostics: 1 steps", summary)
            self.assertIn("c-incremental-fresh-mismatch", (Path(tmp) / "c-incremental-diagnostics.md").read_text())

    def test_malformed_or_wrong_schema_receipt_cannot_fill_a_missing_job(self):
        matrix = json.dumps({"include": [{"grammar": "go"}]})
        for contents in ("{", json.dumps({"schema": "unknown", "grammar": {"name": "go"}})):
            with self.subTest(contents=contents), tempfile.TemporaryDirectory() as tmp:
                (Path(tmp) / "go.json").write_text(contents)
                result = run_script("summarize_grammar_receipts.py", "--report-dir", tmp, "--expected-matrix", matrix)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("missing: go", result.stderr)


if __name__ == "__main__":
    unittest.main()
