#!/usr/bin/env python3
"""Exercise W5's failure paths using complete synthetic paired receipts."""
import copy
import contextlib
import io
import json
import os
import re
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

import editor_latency_gate as gate


class HostedCampaignTests(unittest.TestCase):
    def test_lane_cache_scratch_and_unique_image(self):
        environment = {"GOCACHE": "/cache", "GOTMPDIR": "/go-tmp", "TMPDIR": "/tmp-dir", "GTS_EDITOR_LATENCY_IMAGE": "gotreesitter/test:unique"}
        with mock.patch.dict(os.environ, environment, clear=True):
            command = gate.campaign_docker_command(Path("/repo"), Path("/base"), Path("/out"), 1)
            self.assertIn("gotreesitter/test:unique", command)
            for mount in ("/cache:/lane-go-cache", "/go-tmp:/lane-go-tmp", "/tmp-dir:/lane-tmp"):
                self.assertIn(mount, command)
            self.assertIn("--no-chown", command)
            self.assertEqual(gate.campaign_go_environment(), "export GOCACHE=/lane-go-cache; export GOTMPDIR=/lane-go-tmp; export TMPDIR=/lane-tmp")

    def test_hosted_and_dedicated_limits_apply_to_same_paired_container(self):
        for environment, memory, gomemlimit in (
            ({}, "8g", "6GiB"),
            ({"GTS_EDITOR_LATENCY_MEMORY_LIMIT": "", "GOMEMLIMIT": ""}, "8g", "6GiB"),
            ({"GTS_EDITOR_LATENCY_MEMORY_LIMIT": "6g", "GOMEMLIMIT": "4GiB"}, "6g", "4GiB"),
        ):
            with self.subTest(environment=environment), mock.patch.dict(os.environ, environment, clear=True):
                command = gate.campaign_docker_command(Path("/repo"), Path("/base"), Path("/out"), 1)
                self.assertEqual(command, ["bash", Path("/repo/cgo_harness/docker/run_parity_in_docker.sh"),
                    "--out-root", Path("/out/harness"),
                    "--no-build", "--memory", memory, "--gomemlimit", gomemlimit,
                    "--cpuset-cpus", "1", "--cpus", "1", "--mount", "/base:/baseline:ro", "--mount", "/out:/campaign"])

    def test_build_only_keeps_artifacts_outside_checkout(self):
        command = gate.campaign_harness_command(Path("/repo"), Path("/out")) + ["--build-only"]
        self.assertEqual(command, ["bash", Path("/repo/cgo_harness/docker/run_parity_in_docker.sh"),
            "--out-root", Path("/out/harness"), "--build-only"])


class WorkExemptionHistoryTests(unittest.TestCase):
    def setUp(self):
        self.rows = [{"fixture_sha256": "digest", "cell": "cell", "class": "delete", "value": 30, "ceiling": 20, "expires_at_certificate": 2}]

    def test_unchanged_and_removals(self):
        gate.check_work_exemptions_shrink(self.rows, copy.deepcopy(self.rows))
        gate.check_work_exemptions_shrink(self.rows, [])

    def test_additions_readditions_and_substitutions_fail(self):
        for before, after in (([], self.rows), (self.rows, self.rows + [dict(self.rows[0], cell="other")]),
                              (self.rows, [dict(self.rows[0], cell="other")])):
            with self.subTest(before=before, after=after), self.assertRaises(ValueError):
                gate.check_work_exemptions_shrink(before, after)

    def test_every_receipt_field_is_immutable(self):
        for field in self.rows[0]:
            changed = copy.deepcopy(self.rows)
            changed[0][field] = "changed"
            with self.subTest(field=field), self.assertRaises(ValueError):
                gate.check_work_exemptions_shrink(self.rows, changed)

    def test_duplicates_fail(self):
        with self.assertRaises(ValueError):
            gate.check_work_exemptions_shrink(self.rows, self.rows * 2)


class CampaignLockTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name) / "campaign.lock"

    def assert_locked(self):
        # A separate process must contend on the same inode. Do not substitute
        # a per-user or per-job lock to work around the permission failure.
        result = subprocess.run(["flock", "-n", str(self.path), "true"], capture_output=True)
        self.assertEqual(result.returncode, 1, result.stderr.decode())

    def test_creates_shared_readable_lock_and_releases_on_error(self):
        old_umask = os.umask(0o077)
        try:
            with self.assertRaisesRegex(RuntimeError, "campaign failed"):
                with gate.campaign_lock(self.path):
                    self.assertEqual(self.path.stat().st_mode & 0o777, 0o644)
                    self.assert_locked()
                    raise RuntimeError("campaign failed")
        finally:
            os.umask(old_umask)
        inode = self.path.stat().st_ino
        with gate.campaign_lock(self.path):
            self.assert_locked()
        self.assertEqual(self.path.stat().st_ino, inode)

    def test_existing_read_only_lock_serializes_processes(self):
        self.path.write_text("existing shared lock\n")
        self.path.chmod(0o444)
        inode = self.path.stat().st_ino
        code = """import sys
from pathlib import Path
import editor_latency_gate as gate
print('waiting', flush=True)
with gate.campaign_lock(Path(sys.argv[1])):
    print('acquired', flush=True)
"""
        with gate.campaign_lock(self.path):
            child = subprocess.Popen([sys.executable, "-c", code, str(self.path)],
                cwd=Path(gate.__file__).parent, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            try:
                self.assertEqual(child.stdout.readline().strip(), "waiting")
                with self.assertRaises(subprocess.TimeoutExpired):
                    child.communicate(timeout=0.2)
            except BaseException:
                child.kill()
                child.communicate()
                raise
        try:
            out, err = child.communicate(timeout=5)
            self.assertEqual(child.returncode, 0, err)
            self.assertEqual(out.strip(), "acquired")
        finally:
            if child.poll() is None:
                child.kill()
                child.communicate()
        self.assertEqual(self.path.stat().st_ino, inode)
        self.assertEqual(self.path.read_text(), "existing shared lock\n")
        self.assertEqual(self.path.stat().st_mode & 0o777, 0o444)


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
                        "binding_version": "v0.25.0", "binding_commit": "adc13ffd8b2c0b01b878fda9f7c422ce0df5fad3", "runtime_version": "0.27.1",
                        "runtime_commit": "10eb5ac119eec9cb4267a8e035066473983f5f3a", "grammar_commit": "a" * 40,
                        "grammar_artifact_path": "parser.so", "grammar_artifact_sha256": "a" * 64,
                        "grammar_compile_flags": "-std=c11 -fPIC -O2 -I .", "compiler_path": "cc", "compiler_version": "cc test",
                        "grammar_repo": "https://github.com/example/grammar", "grammar_linkage": "shared_dlopen",
                        "runtime_linkage": "static_cgo_test_binary", "transport": "cgo_parity_binding"}, "cells": cells}
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

    def test_seed_fragments_preserve_all_samples_and_fail_closed(self):
        fixture = self.m["fixtures"][0]
        for role in ("base", "head"):
            source = self.out / "timing" / (fixture["language"] + "-" + role + ".txt")
            original = gate.parse_bench(source, role)
            text = source.read_text()
            header = text.split("# seed: ", 1)[0]
            fragments = []
            for seed in range(1, gate.RUNS + 1):
                start = text.index(f"# seed: {seed};")
                end = text.index("\n", text.index(f"# completed seed: {seed};", start))
                fragment = self.out / f"{role}-{seed}.txt"
                fragment.write_text(header + text[start:end + 1] + "# completed runs: 1\n# status: complete\n")
                fragments.append(fragment)
            combined = self.out / f"combined-{role}.txt"
            gate.combine_timing_seeds(fragments, combined, role)
            self.assertEqual(gate.parse_bench(combined, role), original)
            for broken in (fragments[:-1], fragments[::-1], fragments[:-1] + [fragments[0]]):
                with self.assertRaises(ValueError):
                    gate.combine_timing_seeds(broken, combined, role)
            last = fragments[-1]
            last.write_text(last.read_text().replace("# status: complete", "# status: incomplete"))
            with self.assertRaises(ValueError):
                gate.combine_timing_seeds(fragments, combined, role)

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

    def test_cold_and_warm_oracle_paths_preserve_content_identity_gate(self):
        language = self.m["fixtures"][0]["language"]
        a = copy.deepcopy(self.admissions["head", language])
        a["oracle"]["grammar_artifact_path"] = language + "-0123456789abcdef.so"
        gate.write_json(self.admission_path("head", language), a)
        result = self.compare()
        self.assertEqual(result["status"], "passed")
        self.assertEqual(result["oracles"][0]["oracle"], a["oracle"])
        # Cache location must never excuse different bytes or build provenance.
        for key in gate.oracle_identity(a["oracle"]):
            with self.subTest(key=key):
                broken = copy.deepcopy(a)
                broken["oracle"][key] = "b" + broken["oracle"][key][1:]
                gate.write_json(self.admission_path("head", language), broken)
                with self.assertRaises(ValueError):
                    self.compare()


    def prepare_w5_undo_exception(self):
        for role, count in (("base", 39), ("head", 35)):
            admission = self.admissions[role, "go"]
            step = admission["cells"][0]["steps"][1]
            step.update(source_sha256=gate.GO_1512_FIXTURE, tree_sha256=gate.GO_1512_TREE)
            step["counters"]["reused_subtrees"] = count
            gate.write_json(self.admission_path(role, "go"), admission)
        base = gate.canonical_go_admission(self.admissions["base", "go"])
        head = gate.canonical_go_admission(self.admissions["head", "go"])
        return base, head

    def test_w5_exception_is_strict_by_default_and_exact_when_simulated(self):
        base_hash, head_hash = self.prepare_w5_undo_exception()
        with self.assertRaisesRegex(ValueError, "deterministic counter gate failed"):
            gate.compare_counters(self.out, self.manifest, self.base, self.head)
        with mock.patch.object(gate, "GO_1512_BASE_ADMISSION", base_hash), mock.patch.object(gate, "GO_1512_HEAD_ADMISSION", head_hash):
            result = gate.compare_counters(self.out, self.manifest, self.base, self.head, gate.GO_1512_UNDO_EXCEPTION)
            self.assertEqual(len(result["applied_exceptions"]), 1)
            changed = self.admissions["head", "go"]
            changed["cells"][0]["steps"][0]["counters"]["nodes"] = 999
            gate.write_json(self.admission_path("head", "go"), changed)
            with self.assertRaisesRegex(ValueError, "receipt changed"):
                gate.compare_counters(self.out, self.manifest, self.base, self.head, gate.GO_1512_UNDO_EXCEPTION)

    def test_w5_exception_requires_matching_actual_compiler_and_artifact(self):
        base_hash, head_hash = self.prepare_w5_undo_exception()
        for role in ("base", "head"):
            a = self.admissions[role, "go"]
            a["oracle"]["compiler_version"] = "a different paired compiler"
            a["oracle"]["grammar_artifact_sha256"] = "1" * 64
            gate.write_json(self.admission_path(role, "go"), a)
        with mock.patch.object(gate, "GO_1512_BASE_ADMISSION", base_hash), mock.patch.object(gate, "GO_1512_HEAD_ADMISSION", head_hash):
            gate.compare_counters(self.out, self.manifest, self.base, self.head, gate.GO_1512_UNDO_EXCEPTION)
            a = self.admissions["head", "go"]
            a["oracle"]["grammar_artifact_sha256"] = "2" * 64
            gate.write_json(self.admission_path("head", "go"), a)
            with self.assertRaisesRegex(ValueError, "C oracle differs"):
                gate.compare_counters(self.out, self.manifest, self.base, self.head, gate.GO_1512_UNDO_EXCEPTION)

    def test_w5_exception_requires_real_pinned_approval_and_clean_checkout(self):
        with mock.patch.object(gate, "GO_1512_APPROVAL_SHA256", ""):
            with self.assertRaisesRegex(ValueError, "no approved receipt"):
                gate.validate_1512_undo_exception(self.out, gate.GO_1512_BASE, gate.GO_1512_REVIEWED, self.out / "missing")
        path = self.out / "simulated-owner-decision.json"
        path.write_text(json.dumps({"owner_approved": True, "base": gate.GO_1512_BASE, "reviewed_source": gate.GO_1512_REVIEWED, "exception": gate.GO_1512_UNDO_EXCEPTION}))
        with mock.patch.object(gate, "GO_1512_APPROVAL_SHA256", gate.sha(path.read_bytes())), mock.patch.object(gate, "git", return_value=gate.GO_1512_REVIEWED), mock.patch.object(gate.subprocess, "run"), mock.patch.object(gate.subprocess, "check_output", side_effect=[b"!! ignored-user-work\n"]):
            with self.assertRaisesRegex(ValueError, "clean checkout"):
                gate.validate_1512_undo_exception(self.out, gate.GO_1512_BASE, gate.GO_1512_REVIEWED, path)

    def test_b7_exception_requires_approval_and_binds_clean_exact_source(self):
        decision = self.out / "prospective-decision.json"
        decision.write_text(json.dumps(gate.GO_1512_B7_DECISION, separators=(",", ":")) + "\n")
        with mock.patch.object(gate, "GO_1512_B7_OWNER_APPROVED", False):
            with self.assertRaisesRegex(ValueError, "no owner approval"):
                gate.validate_b7_undo_exception(self.out, gate.GO_1512_B7_BASE, gate.GO_1512_B7_REVIEWED, decision)
        with mock.patch.object(gate, "GO_1512_B7_OWNER_APPROVED", True), mock.patch.object(gate, "git", return_value=gate.GO_1512_B7_REVIEWED), mock.patch.object(gate.subprocess, "run"):
            with self.assertRaisesRegex(ValueError, "base mismatch"):
                gate.validate_b7_undo_exception(self.out, "0" * 40, gate.GO_1512_B7_REVIEWED, decision)
            with mock.patch.object(gate.subprocess, "check_output", side_effect=[b"!! ignored\n"]):
                with self.assertRaisesRegex(ValueError, "clean checkout"):
                    gate.validate_b7_undo_exception(self.out, gate.GO_1512_B7_BASE, gate.GO_1512_B7_REVIEWED, decision)
            with mock.patch.object(gate.subprocess, "check_output", side_effect=[b"", b"parser.go\0"]):
                with self.assertRaisesRegex(ValueError, "production inputs changed"):
                    gate.validate_b7_undo_exception(self.out, gate.GO_1512_B7_BASE, gate.GO_1512_B7_REVIEWED, decision)
            with mock.patch.object(gate.subprocess, "check_output", side_effect=[b"", b"scripts/editor_latency_gate.py\0"]):
                self.assertEqual(gate.validate_b7_undo_exception(self.out, gate.GO_1512_B7_BASE, gate.GO_1512_B7_REVIEWED, decision), gate.GO_1512_B7_UNDO_EXCEPTION)
            decision.write_text("tampered")
            with mock.patch.object(gate.subprocess, "check_output", side_effect=[b"", b""]):
                with self.assertRaisesRegex(ValueError, "decision digest mismatch"):
                    gate.validate_b7_undo_exception(self.out, gate.GO_1512_B7_BASE, gate.GO_1512_B7_REVIEWED, decision)
            forged = copy.deepcopy(gate.GO_1512_B7_DECISION)
            forged["row"]["after"] = 34
            decision.write_text(json.dumps(forged, separators=(",", ":")) + "\n")
            with mock.patch.object(gate, "GO_1512_B7_APPROVAL_SHA256", gate.sha(decision.read_bytes())), mock.patch.object(gate.subprocess, "check_output", side_effect=[b"", b""]):
                with self.assertRaisesRegex(ValueError, "decision content mismatch"):
                    gate.validate_b7_undo_exception(self.out, gate.GO_1512_B7_BASE, gate.GO_1512_B7_REVIEWED, decision)

    def test_b7_exception_allows_only_reviewed_counter_row(self):
        base_hash, head_hash = self.prepare_w5_undo_exception()
        with mock.patch.object(gate, "GO_1512_BASE_ADMISSION", base_hash), mock.patch.object(gate, "GO_1512_HEAD_ADMISSION", head_hash):
            with self.assertRaisesRegex(ValueError, "counter gate failed"):
                gate.compare_counters(self.out, self.manifest, self.base, self.head)
            result = gate.compare_counters(self.out, self.manifest, self.base, self.head, gate.GO_1512_B7_UNDO_EXCEPTION)
            self.assertEqual(len(result["applied_exceptions"]), 1)
            a = self.admissions["head", "go"]
            a["cells"][0]["steps"][1]["counters"]["reused_subtrees"] = 34
            gate.write_json(self.admission_path("head", "go"), a)
            with self.assertRaises(ValueError):
                gate.compare_counters(self.out, self.manifest, self.base, self.head, gate.GO_1512_B7_UNDO_EXCEPTION)

class DetailedArtifactPolicyTests(unittest.TestCase):
    def test_detailed_outputs_stay_local_and_verdicts_remain(self):
        path = os.environ.get("GTS_TEST_GATE_WORKFLOW")
        workflow = Path(path) if path else Path(__file__).resolve().parents[1] / ".github/workflows/ci.yml"
        text = workflow.read_text()
        def section(name):
            found = re.search(r"(?ms)^  " + re.escape(name) + r":\n(.*?)(?=^  [A-Za-z_][A-Za-z0-9_-]*:\n|\Z)", text)
            self.assertIsNotNone(found, name)
            return found.group(1)
        for name in ("race_root_shards", "race_root_isolated", "v1_edit_sessions", "perf-regression"):
            with self.subTest(job=name):
                self.assertNotIn("actions/upload-artifact@", section(name))
        v1 = section("v1_edit_sessions")
        self.assertIn("Last session: route=", v1)
        self.assertIn('exit "$status"', v1)
        self.assertIn('"$evidence/summary.json"', v1)
        self.assertIn("GITHUB_STEP_SUMMARY", section("perf-regression"))
        self.assertIn("actions/upload-artifact@", section("parity_report"))
        build = section("build")
        for gate in ("race_root", "race_packages", "v1_edit_sessions"):
            self.assertIn("- " + gate, build)


if __name__ == "__main__":
    unittest.main()
