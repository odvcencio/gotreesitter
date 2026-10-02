#!/usr/bin/env python3
"""Exercise W5's failure paths using complete synthetic paired receipts."""
import copy
import contextlib
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

import editor_latency_gate as gate


class HostedCampaignTests(unittest.TestCase):
    def test_baseline_forest_counts_selected_and_discarded_arenas_once(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            base.joinpath("go.mod").write_text("module example.com/baseline\n\ngo 1.23\n")
            base.joinpath("parser.go").write_text('''package baseline
type Node struct{}
type nodeArena struct { used int }
type Tree struct { arena *nodeArena; forestFastPath bool; tokens uint64 }
type TokenSource interface{}
type IncrementalParseProfile struct { TokensConsumed, NewNodesAllocated uint64 }
type incrementalParseTiming struct { tokensConsumed, newNodes uint64 }
type Parser struct {
 failed, own bool
}
func (p *Parser) newIncrementalFreshVerifier() *Parser { return &Parser{failed:p.failed} }
func (p *Parser) parseFresh() *Tree {
 arena := &nodeArena{}
 _, ok := p.parseForestWithMode(arena,nil,false,0,nil,false)
 if ok { return &Tree{arena:arena,forestFastPath:true} }
 return &Tree{arena:&nodeArena{used:7},tokens:5}
}
''')
            base.joinpath("glr_forest.go").write_text('''package baseline
func (p *Parser) parseForestWithMode(arena *nodeArena, source []byte, captureExternalCheckpoints bool, memoryBudget int64, lexicalReadSpan *uint32, cleanOnly bool) (*Node, bool) {
	iter := 0
	var tokens uint64
 for iter < 2 { iter++; tokens++ }
 arena.used = 3
 if p.failed { return nil,false }
 return &Node{},true
}
''')
            base.joinpath("incremental_tree_equal.go").write_text('''package baseline
import "time"
func (p *Parser) verifyIncrementalFreshResult(timing *incrementalParseTiming) *Tree {
 started := time.Now()
	verifier := p.newIncrementalFreshVerifier()
 fresh := verifier.parseFresh()
 if timing != nil {
  timing.tokensConsumed += fresh.tokens
  // The older verification adapter already counts the selected arena.
  timing.newNodes += uint64(fresh.arena.used)
 }
	freshNanos := time.Since(started).Nanoseconds()
 _ = freshNanos
 return fresh
}
''')
            base.joinpath("parser_api.go").write_text('''package baseline
func (p *Parser) parseIncrementalProfiledChangedSource(source []byte, oldTree *Tree) (*Tree, IncrementalParseProfile, error) {
 if p.own {
  arena := &nodeArena{}
  p.parseForestWithMode(arena,nil,false,0,nil,false)
 }
 timing := &incrementalParseTiming{tokensConsumed:11,newNodes:13}
 tree := p.verifyIncrementalFreshResult(timing)
 return tree,IncrementalParseProfile{timing.tokensConsumed,timing.newNodes},nil
}
func (p *Parser) parseIncrementalWithTokenSourceProfiled(source []byte, oldTree *Tree, ts TokenSource) (*Tree, IncrementalParseProfile, error) {
 timing := &incrementalParseTiming{tokensConsumed:11,newNodes:13}
 tree := p.verifyIncrementalFreshResult(timing)
 return tree,IncrementalParseProfile{timing.tokensConsumed,timing.newNodes},nil
}
''')
            base.joinpath("accounting_test.go").write_text('''package baseline
import "testing"
func TestForestWork(t *testing.T) {
 for _, failed := range []bool{false,true} {
  for _, own := range []bool{false,true} {
   p := &Parser{failed:failed,own:own}
   tree,profile,err := p.parseIncrementalProfiledChangedSource(nil,nil)
   tokens,nodes := uint64(13),uint64(16)
   if failed { tokens,nodes = 18,23 }
   if own { tokens+=2;nodes+=3 }
   if err!=nil || profile.TokensConsumed!=tokens || profile.NewNodesAllocated!=nodes {
    t.Fatalf("failed=%t own=%t profile=%+v want tokens=%d nodes=%d err=%v",failed,own,profile,tokens,nodes,err)
   }
   if tree.forestFastPath==failed || p.baselineForestAccountingEnabled {t.Fatal("accounting changed result or leaked profiling scope")}
   plain := p.verifyIncrementalFreshResult(nil)
   if plain.forestFastPath!=tree.forestFastPath {t.Fatal("profiled and plain selected different results")}
   // Independent operations cannot retain counters from the last call.
   _,again,_ := p.parseIncrementalProfiledChangedSource(nil,nil)
   if again!=profile {t.Fatalf("warm counters changed: %+v != %+v",again,profile)}
  }
 }
}
''')
            receipt = gate.baseline_forest_accounting(base, base)
            self.assertTrue(receipt["applied"])
            env = {**os.environ, "GOWORK": "off", "GOMAXPROCS": "1", "CGO_ENABLED": "0",
                   "GOPROXY": "off", "GOSUMDB": "off"}
            result = subprocess.run(["go", "test", ".", "-count=1"], cwd=base, env=env,
                                    text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertFalse(gate.baseline_forest_accounting(base, base)["applied"])

    def test_baseline_accounting_preserves_result_and_counts_before_release(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            base, out = root / "base", root / "out"
            base.mkdir()
            out.mkdir()
            base.joinpath("go.mod").write_text("module example.com/baseline\n\ngo 1.23\n")
            source = '''package baseline
type ParseRuntime struct { tokens uint64; NodesAllocated int }
type nodeArena struct { used int }
type Language struct{}
type Tree struct { runtime ParseRuntime; released bool; arena *nodeArena; borrowedArena []*nodeArena }
func (t *Tree) rawParseRuntime() *ParseRuntime { return &t.runtime }
func (t *Tree) Release() { t.released = true; t.runtime = ParseRuntime{} }
type incrementalParseTiming struct { totalNanos int64; tokens uint64; newNodes uint64 }
func incrementalParseTimingFromRuntime(r ParseRuntime) incrementalParseTiming { return incrementalParseTiming{tokens:r.tokens} }
func (t *incrementalParseTiming) addAttempt(a *incrementalParseTiming) { t.tokens += a.tokens }
func incrementalTreesStructurallyEqual(a, b *Tree, lang *Language) bool { a.arena.used += 3; b.arena.used += 2; return true }
type Parser struct { fresh *Tree; language *Language }
func (p *Parser) verifyIncrementalFreshResult(tree *Tree, timing *incrementalParseTiming) *Tree {
\tfresh := p.fresh
\tfreshNanos := int64(1)
\tif fresh != nil && !incrementalTreesStructurallyEqual(tree, fresh, p.language) {
\t} else if fresh != nil {
\t\tfresh.Release()
\t\tif timing != nil {
\t\t\ttiming.totalNanos += freshNanos
\t\t}
\t}
\treturn tree
}
'''
            base.joinpath("incremental_tree_equal.go").write_text(source)
            base.joinpath("accounting_test.go").write_text('''package baseline
import "testing"
func TestProfileAndPlainKeepSameResult(t *testing.T) {
 for _, profiled := range []bool{false,true} {
  shared := &nodeArena{used:4}
  selected := &Tree{runtime:ParseRuntime{tokens:5}, arena:shared, borrowedArena:[]*nodeArena{shared}}
  fresh := &Tree{runtime:ParseRuntime{tokens:2, NodesAllocated:2}, arena:shared, borrowedArena:[]*nodeArena{shared}}
  parser := Parser{fresh:fresh}
  var timing *incrementalParseTiming
  if profiled {timing=&incrementalParseTiming{tokens:5}}
  if got:=parser.verifyIncrementalFreshResult(selected,timing);got!=selected || selected.released || !fresh.released {t.Fatal("accounting changed the selected result or release")}
  if profiled && timing.tokens!=7 {t.Fatalf("tokens=%d want all 7",timing.tokens)}
  if profiled && timing.newNodes!=7 {t.Fatalf("nodes=%d want 2 omitted clones and 5 views without double counting shared arenas",timing.newNodes)}
 }
}
''')
            receipt = gate.baseline_verification_accounting(base, out)
            self.assertTrue(receipt["applied"])
            self.assertEqual(receipt["patch_sha256"], gate.sha((out / "baseline-counter-accounting.patch").read_bytes()))
            env = {**os.environ, "GOWORK": "off", "GOPROXY": "off", "GOSUMDB": "off"}
            result = subprocess.run(["go", "test", ".", "-count=1"], cwd=base, env=env,
                                    text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            again = gate.baseline_verification_accounting(base, out)
            self.assertFalse(again["applied"])
            self.assertEqual(again["instrumented_sha256"], receipt["instrumented_sha256"])

    def test_unknown_baseline_accounting_fails_closed(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            root.joinpath("incremental_tree_equal.go").write_text("package unknown\n")
            with self.assertRaisesRegex(ValueError, "counter scopes must match"):
                gate.baseline_verification_accounting(root, root)

    def test_baseline_does_not_resolve_unrelated_current_harness_tests(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            harness = root / "harness"
            campaign = root / "campaign"
            harness.mkdir()
            campaign.mkdir()
            harness.joinpath("go.mod").write_text("module example.com/harness\n\ngo 1.23\n")
            harness.joinpath("oracle.go").write_text("package harness\nfunc Oracle() bool { return true }\n")
            harness.joinpath("unrelated_test.go").write_text(
                'package harness\nimport _ "example.com/new-parser/internal/graduation"\n')
            campaign.joinpath("go.mod").write_text(
                "module example.com/campaign\n\ngo 1.23\n"
                "require example.com/harness v0.0.0\n"
                f"replace example.com/harness => {harness}\n")
            campaign.joinpath("editor_latency_test.go").write_text(
                'package campaign\nimport ("testing"; "example.com/harness")\n'
                'func TestW5RealCodeEdits(t *testing.T) { if !harness.Oracle() { t.Fatal("missing oracle") } }\n')
            env = {**os.environ, "GOWORK": "off", "GOPROXY": "off", "GOSUMDB": "off"}
            tidy = subprocess.run(["go", "mod", "tidy"], cwd=campaign, env=env,
                                  text=True, capture_output=True)
            self.assertNotEqual(tidy.returncode, 0)
            self.assertIn("internal/graduation", tidy.stderr)
            result = subprocess.run(gate.correctness_test_command("base", "b" * 40, "typescript"),
                                    shell=True, cwd=campaign, env=env, text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertIn("PASS: TestW5RealCodeEdits", result.stdout)

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
                        "binding_version": "v0.25.0", "binding_commit": "adc13ffd8b2c0b01b878fda9f7c422ce0df5fad3", "runtime_version": "0.25.1",
                        "runtime_commit": "f5afe475deb7c0bae6407fb776c76824f717bb61", "grammar_commit": "a" * 40,
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


if __name__ == "__main__":
    unittest.main()
