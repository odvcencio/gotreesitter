#!/usr/bin/env python3
"""Exercise W5's failure paths using complete synthetic paired receipts."""
import copy
import contextlib
import io
import json
from pathlib import Path
import tempfile
import unittest

import editor_latency_gate as gate


class GateTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.out = Path(self.temp.name)
        self.manifest = Path(__file__).resolve().parents[1] / "cgo_harness/editor_latency/fixtures.json"
        self.m = gate.fixture_manifest(self.manifest)
        self.base = "b" * 40
        self.head = "c" * 40
        gate.write_json(self.out / "environment.json", {"base_revision": self.base, "head_revision": self.head})
        self.admissions = {}
        for role, revision in (("base", self.base), ("head", self.head)):
            (self.out / "admission" / role).mkdir(parents=True)
            for f in self.m["fixtures"]:
                cells = []
                for workload in gate.WORKLOADS.values():
                    count = len(f["typing_text"].encode()) if workload == "typing" else 2
                    cells.append({"workload": workload, "steps": [{"source_sha256": "d" * 64, "tree_sha256": "e" * 64, "has_error": False,
                        "counters": {"tokens": 100, "nodes": 100, "max_stacks": 1, "reused_subtrees": 100, "reused_bytes": 1000}} for _ in range(count)]})
                a = {"schema": "w5-edit-admission-v1", "revision": revision, "language": f["language"],
                     "fixture_sha256": f["sha256"], "manifest_sha256": gate.sha(self.manifest.read_bytes()),
                     "initial_tree_sha256": "f" * 64, "no_edit_allocs": 0,
                     "oracle": {"language": f["language"], "contract": "tree-sitter-c-v1", "binding_module": "github.com/tree-sitter/go-tree-sitter",
                        "binding_version": "v0.25.0", "binding_commit": "adc13ffd8b2c0b01b878fda9f7c422ce0df5fad3", "runtime_version": "0.25.1",
                        "runtime_commit": "f5afe475deb7c0bae6407fb776c76824f717bb61", "grammar_commit": "a" * 40,
                        "grammar_artifact_sha256": "a" * 64, "grammar_compile_flags": "-std=c11 -fPIC -O2 -I ."}, "cells": cells}
                self.admissions[role, f["language"]] = a
                gate.write_json(self.admission_path(role, f["language"]), a)
        (self.out / "timing").mkdir()
        for f in self.m["fixtures"]:
            for role in ("base", "head"):
                self.write_bench(f, role)

    def admission_path(self, role, language):
        return self.out / "admission" / role / (language + ".json")

    def write_bench(self, f, role, multiplier=1, c_multiplier=1):
        lines = ["# status: incomplete", "# protocol: paired-alternating-seeds", "# role: " + ("baseline" if role == "base" else "head"),
                 "# GOMAXPROCS: 1", "# count per process: 1", "# benchtime: 750ms", "# build tags: treesitter_c_parity",
                 "# required benchmarks: " + ",".join(gate.NAMES)]
        for seed in range(1, 21):
            position = 1 if (seed % 2 == 1) == (role == "base") else 2
            lines.append(f"# seed: {seed}; position: {position}")
            for benchmark, workload in gate.WORKLOADS.items():
                edits = len(f["typing_text"].encode()) if workload == "typing" else 2
                for runtime in gate.BACKENDS:
                    ns = (1000 * multiplier if runtime.startswith("Go") else 100 * c_multiplier) * edits
                    lines.append(f"{benchmark}/{runtime} 10 {ns} ns/op {edits} edits/op 20 B/op 4 allocs/op")
            lines.append(f"# completed seed: {seed}; at: 2026-09-30T00:00:00Z")
        lines += ["# completed runs: 20", "# status: complete"]
        (self.out / "timing" / (f["language"] + "-" + role + ".txt")).write_text("\n".join(lines) + "\n")

    def compare(self):
        with contextlib.redirect_stdout(io.StringIO()):
            return gate.compare_timing(self.out, self.manifest, self.base, self.head)

    def test_complete_campaign_and_exact_five_percent_boundary(self):
        f = self.m["fixtures"][0]
        self.write_bench(f, "head", 1.05)
        result = self.compare()
        self.assertEqual(result["status"], "passed")
        self.assertEqual(len(result["cells"]), len(self.m["fixtures"]) * 3)

    def test_any_cell_above_five_percent_fails_even_if_c_slows(self):
        f = self.m["fixtures"][-1]
        self.write_bench(f, "head", 1.0501, c_multiplier=2)
        result = self.compare()
        self.assertEqual(result["status"], "failed")
        self.assertEqual(len([r for r in result["cells"] if not r["passed"]]), 3)

    def test_missing_duplicate_nan_and_incomplete_benchmarks_fail(self):
        f = self.m["fixtures"][0]
        path = self.out / "timing" / (f["language"] + "-head.txt")
        original = path.read_text()
        row = next(line for line in original.splitlines() if line.startswith("BenchmarkW5"))
        for broken in (original.replace(row + "\n", "", 1), original.replace(row + "\n", row + "\n" + row + "\n", 1),
                       original.replace("2000 ns/op", "nan ns/op", 1), original.replace("# status: complete", "# status: incomplete"),
                       original.replace("# completed seed: 20;", "# completed seed: 19;"), original.replace("# seed: 20;", "# seed: 19;"),
                       original.replace("750ms", "100ms"), original.replace("2 edits/op", "1 edits/op", 1)):
            with self.subTest(broken=broken[-100:]):
                path.write_text(broken)
                with self.assertRaises(ValueError): self.compare()
        path.write_text(original)

    def test_counter_ledger_and_correctness_identity_fail_closed(self):
        language = self.m["fixtures"][0]["language"]
        original = self.admissions["head", language]
        changes = [lambda a: a["cells"][0]["steps"][0]["counters"].update(tokens=103),
                   lambda a: a["cells"][0]["steps"][0]["counters"].update(reused_subtrees=97),
                   lambda a: a["cells"][0]["steps"][0].update(tree_sha256="a"*64),
                   lambda a: a["cells"].pop(), lambda a: a["cells"][2]["steps"].pop(),
                   lambda a: a.update(no_edit_allocs=1), lambda a: a.update(revision=self.base),
                   lambda a: a["oracle"].update(runtime_commit="a"*40)]
        for change in changes:
            a = copy.deepcopy(original); change(a)
            gate.write_json(self.admission_path("head", language), a)
            with self.assertRaises(ValueError): self.compare()

    def test_pairing_uses_matching_seeds(self):
        f = self.m["fixtures"][0]
        # A huge slowdown in one head sample cannot be hidden by sorting times
        # before pairing. Here the other 19 matching seeds remain equal.
        path = self.out / "timing" / (f["language"] + "-head.txt")
        text = path.read_text().replace("2000 ns/op", "200000 ns/op", 1)
        path.write_text(text)
        result = self.compare()
        row = result["cells"][0]
        self.assertEqual(row["paired_ratio"], 1)
        self.assertGreater(row["paired_max"], 1)


if __name__ == "__main__":
    unittest.main()
