#!/usr/bin/env python3
"""Exercise report contents and issue reconciliation without GitHub writes."""

import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

from render_grammar_update_issue import render


SCRIPTS = Path(__file__).resolve().parent
TITLE = "Grammar updates available"
UPDATE = {
    "name": "example",
    "status": "available",
    "repo_url": "https://github.com/tree-sitter/tree-sitter-example.git",
    "old_ref": "a" * 40,
    "new_ref": "b" * 40,
}
CHECK = {**UPDATE, "blocked": False}


class ReportTests(unittest.TestCase):
    def body(self, check=None):
        return render({"results": [UPDATE]}, {"results": [check or CHECK]})

    def test_clear_update_has_full_refs_compare_and_requirements(self):
        body = self.body()
        self.assertIn("Available updates: 1.", body)
        self.assertIn(f"`{UPDATE['old_ref']}`", body)
        self.assertIn(f"`{UPDATE['new_ref']}`", body)
        self.assertIn(f"/compare/{UPDATE['old_ref']}...{UPDATE['new_ref']}", body)
        self.assertNotIn(".git/compare", body)
        self.assertIn("| No | None from scanner check |", body)
        self.assertIn("regenerated blobs, CI results, and per-grammar locked-C parity", body)

    def test_scanner_and_external_changes_are_reported(self):
        cases = [
            {"source_files": [{"changed": True}], "reasons": ["src/scanner.c changed"]},
            {"expected_externals": ["old"], "actual_externals": ["new"], "reasons": ["external token list changed"]},
        ]
        for case in cases:
            with self.subTest(case=case):
                body = self.body({**CHECK, "blocked": True, **case})
                self.assertIn("| Yes | " + case["reasons"][0], body)

    def test_failed_scanner_check_is_unknown_and_omits_diagnostics(self):
        body = self.body({**CHECK, "blocked": True, "reasons": ["fetch old ref: private diagnostic"]})
        self.assertIn("| Unknown | Could not fetch current ref; scanner check incomplete |", body)
        self.assertNotIn("private diagnostic", body)
        body = self.body({**CHECK, "blocked": True, "reasons": ["read src/grammar.json: private diagnostic"]})
        self.assertIn("Could not read src/grammar.json; check incomplete", body)
        self.assertNotIn("private diagnostic", body)

    def test_missing_or_stale_guard_rejects_publication(self):
        for checks in ([], [{**CHECK, "new_ref": "stale"}]):
            with self.subTest(checks=checks), self.assertRaises(ValueError):
                render({"results": [UPDATE]}, {"results": checks})

    def test_failed_or_mutating_plan_rejects_publication(self):
        for flags in ({"error_count": 1}, {"write_applied": True}):
            with self.subTest(flags=flags), self.assertRaises(ValueError):
                render({"results": [], **flags}, {"results": []})

    def test_no_updates_and_unchanged_entries(self):
        body = render({"results": [{**UPDATE, "status": "unchanged"}]}, {"results": []})
        self.assertIn("No upstream grammar updates are pending", body)
        self.assertNotIn("| example |", body)


class IssueTests(unittest.TestCase):
    def run_sync(self, pending=True, issues=None, error_count=0, list_failure=False):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            plan = root / "plan.json"
            plan.write_text(json.dumps({"results": [UPDATE] if pending else [], "error_count": error_count}))
            body = root / "body.md"
            body.write_text("Issue report\n")
            log = root / "calls.jsonl"
            gh = root / "gh"
            gh.write_text('''#!/usr/bin/env python3
import json, os, sys
with open(os.environ["CALL_LOG"], "a") as log:
    log.write(json.dumps(sys.argv[1:]) + "\\n")
if sys.argv[1:3] == ["issue", "list"]:
    if os.environ["LIST_FAILURE"] == "1":
        sys.exit(1)
    print(os.environ["ISSUES"])
''')
            gh.chmod(0o755)
            env = {
                **os.environ,
                "PATH": str(root) + os.pathsep + os.environ["PATH"],
                "GH_REPO": "tree-sitter/example",
                "ISSUES": json.dumps(issues or []),
                "CALL_LOG": str(log),
                "LIST_FAILURE": "1" if list_failure else "0",
            }
            result = subprocess.run(
                ["bash", str(SCRIPTS / "sync_grammar_update_issue.sh"), str(plan), str(body)],
                env=env, capture_output=True, text=True,
            )
            calls = [json.loads(line) for line in log.read_text().splitlines()] if log.exists() else []
            return result, calls

    def test_create_labeled_issue(self):
        result, calls = self.run_sync()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([call[:2] for call in calls], [["issue", "list"], ["label", "create"], ["issue", "create"]])
        self.assertIn("grammar-updates", calls[-1])
        self.assertIn(TITLE, calls[-1])
        self.assertIn("--body-file", calls[-1])

    def test_update_existing_issue_and_ignore_other_titles(self):
        result, calls = self.run_sync(issues=[{"number": 5, "title": "Other update"}, {"number": 7, "title": TITLE}])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([call[:3] for call in calls[1:]], [["issue", "edit", "7"]])

    def test_existing_duplicates_are_closed(self):
        result, calls = self.run_sync(issues=[{"number": 9, "title": TITLE}, {"number": 7, "title": TITLE}])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([call[:3] for call in calls[1:]], [["issue", "edit", "7"], ["issue", "close", "9"]])

    def test_no_updates_closes_all_matching_issues(self):
        result, calls = self.run_sync(pending=False, issues=[{"number": 7, "title": TITLE}, {"number": 9, "title": TITLE}])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([call[:3] for call in calls[1:]], [["issue", "close", "7"], ["issue", "close", "9"]])
        self.assertTrue(all(call[-1] == "completed" for call in calls[1:]))

    def test_empty_report_without_issue_does_nothing(self):
        result, calls = self.run_sync(pending=False)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(calls), 1)

    def test_plan_or_github_failure_does_not_write(self):
        for flags in ({"error_count": 1}, {"list_failure": True}):
            with self.subTest(flags=flags):
                result, calls = self.run_sync(pending=False, **flags)
                self.assertNotEqual(result.returncode, 0)
                self.assertTrue(all(call[:2] == ["issue", "list"] for call in calls))


if __name__ == "__main__":
    unittest.main()
