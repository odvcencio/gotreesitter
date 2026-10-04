#!/usr/bin/env python3
"""W5 paired edit gate. Fail closed on incomplete or incomparable evidence."""

import argparse
import contextlib
import datetime
import difflib
import fcntl
import hashlib
import json
import math
import os
from pathlib import Path
import platform
import re
import shlex
import shutil
import statistics
import subprocess
import sys
import tempfile
import urllib.request


WORKLOADS = {"BenchmarkW5OneByte": "one_byte", "BenchmarkW5HundredByte": "hundred_byte", "BenchmarkW5Typing": "typing"}
BACKENDS = ("GoA", "CA", "CB", "GoB")
NAMES = [f"{benchmark}/{backend}" for benchmark in WORKLOADS for backend in BACKENDS]
WORK_COUNTERS = ("tokens", "nodes", "max_stacks")
REUSE_COUNTERS = ("reused_subtrees", "reused_bytes")
EDIT_LIMIT = 1.05
LEDGER_LIMIT = 1.02
RUNS = 20
BENCHSTAT_MODULE = "golang.org/x/perf/cmd/benchstat@v0.0.0-20260929162123-406019bb8b68"


def correctness_test_command(role, revision, language):
    # Resolve only the campaign's compiled imports. `go mod tidy` also walks
    # the current harness's tests, whose new internal packages may be absent
    # from the older parser revision being measured.
    return (f"GTS_EDIT_REVISION={revision} GTS_EDIT_ADMISSION_OUT=/campaign/admission/{role}/{language}.json "
            "go test -mod=mod -tags treesitter_c_parity . -run '^TestW5RealCodeEdits$' -count=1 -v -timeout 15m")


def baseline_verification_accounting(base, out):
    """Restore omitted verification work without changing baseline parsing."""
    path = base / "incremental_tree_equal.go"
    original = path.read_text()
    require("func (p *Parser) verifyIncrementalFreshResult" in original,
            "unrecognized baseline verification accounting; counter scopes must match")
    old = """\t} else if fresh != nil {
\t\tfresh.Release()
\t\tif timing != nil {
\t\t\ttiming.totalNanos += freshNanos
\t\t}
"""
    new = """\t} else if fresh != nil {
\t\tif timing != nil {
\t\t\ttiming.totalNanos += freshNanos
\t\t\tattempt := incrementalParseTimingFromRuntime(*fresh.rawParseRuntime())
\t\t\ttiming.addAttempt(&attempt)
\t\t}
\t\tfresh.Release()
"""
    result = {"file": path.name, "original_sha256": sha(original.encode()),
              "scope": "include retained-result verification, fresh arena clones, and lazy comparison views in profiled counters"}
    corrected = original
    if old in original:
        require(original.count(old) == 1, "ambiguous baseline verification accounting")
        corrected = original.replace(old, new, 1)
    else:
        retained = """\t} else {
\t\ttree.setIncrementalFreshVerified(true)
\t\ttree.eofExtraTokenSourceProofID = fresh.eofExtraTokenSourceProofID
\t\tfresh.Release()
\t\tif timing != nil {
\t\t\ttiming.totalNanos += freshNanos
\t\t}
"""
        if retained in original:
            require(original.count(retained) == 1, "ambiguous baseline verification accounting")
            charged = retained.replace("\t\tfresh.Release()\n", "").replace(
                "\t\t\ttiming.totalNanos += freshNanos\n",
                "\t\t\ttiming.totalNanos += freshNanos\n\t\t\tattempt := incrementalParseTimingFromRuntime(*fresh.rawParseRuntime())\n\t\t\ttiming.addAttempt(&attempt)\n") + "\t\tfresh.Release()\n"
            corrected = original.replace(retained, charged, 1)
        verifier = corrected.split("func (p *Parser) verifyIncrementalFreshResult", 1)
        require(len(verifier) == 2 and "timing.addAttempt(&attempt)" in verifier[1],
                "unrecognized baseline verification accounting; counter scopes must match")
    comparison = "incrementalTreesStructurallyEqual(tree, fresh, p.language)"
    if comparison in corrected:
        require(corrected.count(comparison) == 1, "ambiguous baseline comparison accounting")
        line = next(line for line in corrected.splitlines(True) if comparison in line)
        allocation_accounting = """\t// The old loop counter omits cloned nodes already constructed in the
\t// fresh arena. Include them before comparison or release changes it.
\tif timing != nil && fresh != nil && fresh.arena != nil {
\t\tif extra := fresh.arena.used - fresh.rawParseRuntime().NodesAllocated; extra > 0 {
\t\t\ttiming.newNodes += uint64(extra)
\t\t}
\t}
"""
        corrected = corrected.replace(line, allocation_accounting + line, 1)
        corrected = corrected.replace(comparison, "baselineVerificationTreesEqual(tree, fresh, p.language, timing)", 1)
        corrected += """
// Account for lazy views built by the existing comparison. Parser decisions
// and the unprofiled comparison retain their original implementation.
func baselineVerificationTreesEqual(a, b *Tree, lang *Language, timing *incrementalParseTiming) bool {
	if timing == nil {
		return incrementalTreesStructurallyEqual(a, b, lang)
	}
	type frame struct {
		arena *nodeArena
		used int
	}
	var local [8]frame
	frames := local[:0]
	add := func(arena *nodeArena) {
		if arena == nil { return }
		for _, item := range frames {
			if item.arena == arena { return }
		}
		frames = append(frames, frame{arena, arena.used})
	}
	for _, tree := range []*Tree{a, b} {
		if tree != nil {
			add(tree.arena)
			for _, arena := range tree.borrowedArena { add(arena) }
		}
	}
	equal := incrementalTreesStructurallyEqual(a, b, lang)
	for _, item := range frames {
		if added := item.arena.used - item.used; added > 0 {
			timing.newNodes += uint64(added)
		}
	}
	return equal
}
"""
    if corrected != original:
        path.write_text(corrected)
        patch = "".join(difflib.unified_diff(original.splitlines(True), corrected.splitlines(True),
                                            fromfile="a/" + path.name, tofile="b/" + path.name))
        (out / "baseline-counter-accounting.patch").write_text(patch)
        result.update(applied=True, patch_sha256=sha(patch.encode()), instrumented_sha256=sha(corrected.encode()))
    else:
        result.update(applied=False, instrumented_sha256=result["original_sha256"])
    return result


def baseline_forest_accounting(base, out):
    """Count existing forest and verifier attempts without changing decisions."""
    paths = {name: base / name for name in (
        "parser.go", "parser_api.go", "glr_forest.go", "incremental_tree_equal.go")}
    originals = {name: path.read_text() for name, path in paths.items()}
    forest = originals["glr_forest.go"]
    result = {"scope": "include accepted/discarded forest work and all legacy verifier attempts in profiled counters",
              "original_sha256": {name: sha(source.encode()) for name, source in originals.items()}}
    if "p.recordOperationAttempt(" in forest or "baselineForestAccountingEnabled" in originals["parser.go"]:
        return dict(result, applied=False)
    corrected = dict(originals)

    def replace(name, old, new):
        require(corrected[name].count(old) == 1,
                f"unrecognized baseline forest accounting in {name}; counter scopes must match")
        corrected[name] = corrected[name].replace(old, new, 1)

    replace("parser.go", "type Parser struct {", """type Parser struct {
	baselineForestAccountingEnabled bool
	baselineForestTokens uint64
	baselineForestNodes uint64
	baselineLastForestNodes uint64
	baselineLegacyAccountingEnabled bool
	baselineLegacyTokens uint64
	baselineLegacyNodes uint64
	baselineLegacyArenaNodes map[*nodeArena]uint64
	baselineAccountingParent *Parser
	baselineRecoveryTokens uint64
	baselineRecoveryNodes uint64""")
    replace("parser.go", "\tparseRuntime := ParseRuntime{", """
	if p.baselineLegacyAccountingEnabled {
		defer func() {
			accounting := p
			if p.baselineAccountingParent != nil {
				accounting = p.baselineAccountingParent
				if !accounting.baselineLegacyAccountingEnabled {
					accounting.baselineRecoveryTokens += perfTokensConsumed
					accounting.baselineRecoveryNodes += uint64(arena.used)
					return
				}
			}
			accounting.baselineLegacyTokens += perfTokensConsumed
			accounting.baselineLegacyNodes += uint64(arena.used)
			if accounting.baselineLegacyArenaNodes == nil {
				accounting.baselineLegacyArenaNodes = make(map[*nodeArena]uint64)
			}
			accounting.baselineLegacyArenaNodes[arena] = uint64(arena.used)
		}()
	}
	parseRuntime := ParseRuntime{""")
    signature = "func (p *Parser) parseForestWithMode(arena *nodeArena, source []byte, captureExternalCheckpoints bool, memoryBudget int64, lexicalReadSpan *uint32, cleanOnly bool) (*Node, bool) {"
    replace("glr_forest.go", signature, signature + """
	var baselineTokens uint64
	if p.baselineForestAccountingEnabled {
		defer func() {
			accounting := p
			if p.baselineAccountingParent != nil { accounting = p.baselineAccountingParent }
			accounting.baselineForestTokens += baselineTokens
			accounting.baselineLastForestNodes = uint64(arena.used)
			accounting.baselineForestNodes += accounting.baselineLastForestNodes
		}()
	}""")
    # Read the existing token counter at return, before the caller releases the
    # forest arena. No lexer, worklist, acceptance, or budget condition changes.
    replace("glr_forest.go", "\titer := 0\n\tvar tokens uint64\n",
            "\titer := 0\n\tvar tokens uint64\n\tif p.baselineForestAccountingEnabled { defer func() { baselineTokens = tokens }() }\n")
    replace("parser_api.go", "\tparser.skipRecoveryReparse = true\n", """
	if p.baselineForestAccountingEnabled || p.baselineLegacyAccountingEnabled {
		previousParent := parser.baselineAccountingParent
		previousForest, previousLegacy := parser.baselineForestAccountingEnabled, parser.baselineLegacyAccountingEnabled
		parser.baselineAccountingParent = p
		parser.baselineForestAccountingEnabled, parser.baselineLegacyAccountingEnabled = true, true
		defer func() {
			parser.baselineAccountingParent = previousParent
			parser.baselineForestAccountingEnabled, parser.baselineLegacyAccountingEnabled = previousForest, previousLegacy
		}()
	}
	parser.skipRecoveryReparse = true
""")
    replace("incremental_tree_equal.go", "\tverifier := p.newIncrementalFreshVerifier()\n",
            "\tverifier := p.newIncrementalFreshVerifier()\n\tverifier.baselineForestAccountingEnabled = timing != nil\n\tverifier.baselineLegacyAccountingEnabled = timing != nil\n")
    replace("incremental_tree_equal.go", "\tfreshNanos := time.Since(started).Nanoseconds()\n", """
	if timing != nil {
		timing.tokensConsumed += verifier.baselineForestTokens
		forestNodes := verifier.baselineForestNodes
		// The verification adapter already counts the selected forest arena
		// as omitted clones. Add only the other forest arenas here.
		if fresh != nil && fresh.forestFastPath {
			forestNodes -= verifier.baselineLastForestNodes
		}
		timing.newNodes += forestNodes
		legacyTokens, legacyNodes := verifier.baselineLegacyTokens, verifier.baselineLegacyNodes
		if fresh != nil {
			if selectedNodes, ok := verifier.baselineLegacyArenaNodes[fresh.arena]; ok {
				// The selected attempt is already charged through its runtime
				// and the existing fresh-arena adapter. Charge discarded attempts.
				legacyTokens -= fresh.rawParseRuntime().TokensConsumed
				legacyNodes -= selectedNodes
			}
		}
		timing.tokensConsumed += legacyTokens
		timing.newNodes += legacyNodes
	}
	freshNanos := time.Since(started).Nanoseconds()
""")
    for method, args in (
            ("parseIncrementalProfiledChangedSource", "source []byte, oldTree *Tree"),
            ("parseIncrementalWithTokenSourceProfiled", "source []byte, oldTree *Tree, ts TokenSource")):
        old = f"func (p *Parser) {method}({args}) (*Tree, IncrementalParseProfile, error) {{"
        new = f"func (p *Parser) {method}({args}) (baselineTree *Tree, baselineProfile IncrementalParseProfile, baselineErr error) {{" + """
	p.baselineForestAccountingEnabled = true
	p.baselineForestTokens, p.baselineForestNodes, p.baselineLastForestNodes = 0, 0, 0
	p.baselineRecoveryTokens, p.baselineRecoveryNodes = 0, 0
	defer func() {
		baselineProfile.TokensConsumed += p.baselineForestTokens + p.baselineRecoveryTokens
		baselineProfile.NewNodesAllocated += p.baselineForestNodes + p.baselineRecoveryNodes
		p.baselineForestAccountingEnabled = false
	}()
"""
        replace("parser_api.go", old, new)
    patches = []
    for name, source in corrected.items():
        paths[name].write_text(source)
        patches.extend(difflib.unified_diff(originals[name].splitlines(True), source.splitlines(True),
                                            fromfile="a/" + name, tofile="b/" + name))
    patch = "".join(patches)
    (out / "baseline-forest-counter-accounting.patch").write_text(patch)
    return dict(result, applied=True, patch_sha256=sha(patch.encode()),
                instrumented_sha256={name: sha(source.encode()) for name, source in corrected.items()})


def require(ok, message):
    if not ok:
        raise ValueError(message)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def read_json(path):
    def unique(pairs):
        result = {}
        for key, value in pairs:
            require(key not in result, f"duplicate JSON key: {key}")
            result[key] = value
        return result
    return json.loads(Path(path).read_text(), object_pairs_hook=unique)


def write_json(path, data):
    Path(path).write_text(json.dumps(data, indent=2, allow_nan=False) + "\n")


def fixture_manifest(path):
    m = read_json(path)
    require(m["schema"] == "w5-editor-fixtures-v1", "wrong fixture schema")
    require(len(m["languages"]) == 8, "the top-eight surface must have eight languages")
    names = m["languages"] + m["supplemental_languages"]
    require(len(set(names)) == len(names), "duplicate language")
    require([f["language"] for f in m["fixtures"]] == names, "fixture surface/order mismatch")
    for f in m["fixtures"]:
        require(re.fullmatch(r"[a-z_]+", f["language"]), "unsafe language")
        require(re.fullmatch(r"[0-9a-f]{40}", f["commit"]), "fixture commit must be full SHA")
        require(re.fullmatch(r"[0-9a-f]{64}", f["sha256"]), "fixture digest must be full SHA-256")
        require(f["repo"].startswith("https://github.com/") and len(f["repo"].split("/")) == 5, "fixture must name a GitHub repository")
        require(not Path(f["path"]).is_absolute() and ".." not in Path(f["path"]).parts, "unsafe fixture path")
        require(len(f["hundred_byte_text"].encode()) == 100, "hundred-byte edit changed size")
        require(len(f["typing_text"].encode()) >= 8 and f["typing_text"].isascii(), "typing must contain at least eight single-byte keystrokes")
    return m


def fetch_fixtures(root, manifest_path, lock, out):
    m = fixture_manifest(manifest_path)
    wanted = (root / "cgo_harness/perf_scan/corpus_sources.lock.sha256").read_text().split()[0]
    require(sha(lock.read_bytes()) == wanted, "unauthenticated corpus lock")
    rows = {}
    for line in lock.read_text().splitlines():
        if line.strip() and not line.startswith("#"):
            parts = line.split()
            require(parts[0] not in rows, "duplicate corpus language")
            rows[parts[0]] = parts
    priorities = [line for line in (root / "grammars/update_tier1_top50.txt").read_text().splitlines() if line and not line.startswith("#")]
    require(m["languages"] == priorities[:8], "W5 languages drifted from the top-eight priority list")
    out.mkdir()
    for f in m["fixtures"]:
        row = rows[f["language"]]
        require(row[1:3] == [f["repo"], f["commit"]], "fixture does not match authenticated corpus lock")
        url = f["repo"].replace("https://github.com/", "https://raw.githubusercontent.com/") + "/" + f["commit"] + "/" + f["path"]
        request = urllib.request.Request(url, headers={"User-Agent": "gotreesitter-w5"})
        with urllib.request.urlopen(request, timeout=60) as response:
            data = response.read(f["bytes"] + 1)
        require(len(data) == f["bytes"] and sha(data) == f["sha256"], "downloaded fixture hash/size mismatch")
        (out / f["language"]).write_bytes(data)
    return m


def admission(out, role, f, manifest_sha, revision):
    a = read_json(out / "admission" / role / (f["language"] + ".json"))
    require(a["schema"] == "w5-edit-admission-v1", "wrong admission schema")
    require(a["revision"] == revision and a["language"] == f["language"], "admission revision/language mismatch")
    require(a["fixture_sha256"] == f["sha256"] and a["manifest_sha256"] == manifest_sha, "admission fixture/manifest mismatch")
    require(a["no_edit_allocs"] == 0, "no-edit allocations must be zero")
    oracle = a["oracle"]
    require(oracle["language"] == f["language"] and oracle["contract"] == "tree-sitter-c-v1", "wrong C oracle contract/language")
    require(oracle["binding_module"] == "github.com/tree-sitter/go-tree-sitter" and oracle["binding_version"] == "v0.25.0" and oracle["binding_commit"] == "adc13ffd8b2c0b01b878fda9f7c422ce0df5fad3", "C binding is not locked")
    require(oracle["runtime_version"] == "0.25.1" and oracle["runtime_commit"] == "f5afe475deb7c0bae6407fb776c76824f717bb61", "C runtime is not locked")
    require(re.fullmatch(r"[0-9a-f]{40}", oracle["grammar_commit"]) and re.fullmatch(r"[0-9a-f]{64}", oracle["grammar_artifact_sha256"]), "C grammar/artifact identity is missing")
    require(oracle["grammar_compile_flags"] == "-std=c11 -fPIC -O2 -I .", "C grammar flags differ from the locked oracle")
    require([c["workload"] for c in a["cells"]] == list(WORKLOADS.values()), "missing or duplicate admission cell")
    for cell in a["cells"]:
        count = len(f["typing_text"].encode()) if cell["workload"] == "typing" else 2
        require(len(cell["steps"]) == count, "incomplete edit session")
        for step in cell["steps"]:
            for key in ("source_sha256", "tree_sha256"):
                require(re.fullmatch(r"[0-9a-f]{64}", step[key]), "invalid admission digest")
            for key in WORK_COUNTERS + REUSE_COUNTERS:
                value = step["counters"][key]
                require(type(value) is int and value >= 0, "counter must be a nonnegative integer")
    return a


def oracle_identity(oracle):
    # A cold build loads parser.so; a warm load uses the cache's keyed name.
    # The path is diagnostic. Compare the exact artifact SHA-256 and every
    # runtime, grammar, compiler and flag field, regardless of cache location.
    return {key: value for key, value in oracle.items() if key != "grammar_artifact_path"}


def compare_counters(out, manifest_path, base_revision, head_revision):
    m = fixture_manifest(manifest_path)
    manifest_sha = sha(Path(manifest_path).read_bytes())
    changes = []
    identities = []
    failures = []
    for f in m["fixtures"]:
        base = admission(out, "base", f, manifest_sha, base_revision)
        head = admission(out, "head", f, manifest_sha, head_revision)
        require(oracle_identity(base["oracle"]) == oracle_identity(head["oracle"]), f"{f['language']}: C oracle differs between revisions")
        require(base["initial_tree_sha256"] == head["initial_tree_sha256"], "initial tree digest differs")
        identities.append({"language": f["language"], "oracle": head["oracle"]})
        for bc, hc in zip(base["cells"], head["cells"]):
            for index, (bs, hs) in enumerate(zip(bc["steps"], hc["steps"])):
                require((bs["source_sha256"], bs["tree_sha256"], bs["has_error"]) == (hs["source_sha256"], hs["tree_sha256"], hs["has_error"]), "edit source/tree digest differs between revisions")
                for key in WORK_COUNTERS + REUSE_COUNTERS:
                    before, after = bs["counters"][key], hs["counters"][key]
                    bad = after > before * LEDGER_LIMIT
                    # The reuse ledger tolerates a fall of 2% of the base value.
                    if key in REUSE_COUNTERS:
                        bad = after < before * (2 - LEDGER_LIMIT)
                    row = {"language": f["language"], "workload": bc["workload"], "step": index, "counter": key, "before": before, "after": after}
                    changes.append(row)
                    if bad:
                        failures.append(row)
    result = {"schema": "w5-edit-counters-v1", "passed": not failures, "rows": changes, "failures": failures, "oracles": identities}
    write_json(out / "counters.json", result)
    require(not failures, f"deterministic counter gate failed: {failures[:8]}")
    return result


def parse_bench(path, role, *, expected_seeds=None):
    expected_seeds = list(range(1, RUNS + 1)) if expected_seeds is None else list(expected_seeds)
    seeds = {}
    completed = []
    current = None
    status = None
    metadata = {}
    order = []
    for line in Path(path).read_text().splitlines():
        if line.startswith("# status: "):
            status = line.split(": ", 1)[1]
        match = re.fullmatch(r"# seed: (\d+); position: ([12])", line)
        if match:
            current, position = map(int, match.groups())
            require(current not in seeds, "duplicate seed")
            seeds[current] = {"position": position, "rows": {}}
            order.append(current)
        if line.startswith("# completed seed: "):
            completed.append(int(line.split()[3].rstrip(";")))
        if line.startswith("# ") and ": " in line:
            key, value = line[2:].split(": ", 1)
            metadata[key] = value
        if line.startswith("BenchmarkW5"):
            fields = line.split()
            name = re.sub(r"-\d+$", "", fields[0])
            require(current is not None and name in NAMES and name not in seeds[current]["rows"], "unexpected/duplicate benchmark row")
            require(len(fields) >= 10 and int(fields[1]) > 0 and len(fields[2:]) % 2 == 0, "invalid benchmark row")
            metrics = {fields[i+1]: float(fields[i]) for i in range(2, len(fields), 2)}
            for key in ("ns/op", "B/op", "allocs/op", "edits/op"):
                require(key in metrics and math.isfinite(metrics[key]) and metrics[key] >= 0, "missing/nonfinite metric")
            require(metrics["ns/op"] > 0 and metrics["edits/op"] > 0, "nonpositive timing/session size")
            seeds[current]["rows"][name] = metrics
    require(status == "complete" and metadata.get("completed runs") == str(len(expected_seeds)), "incomplete randomized run")
    require(order == expected_seeds and completed == order, "incomplete or reordered seeds")
    require(metadata.get("protocol") == "paired-alternating-seeds" and metadata.get("role") == ("baseline" if role == "base" else "head"), "not paired baseline/head evidence")
    require(metadata.get("GOMAXPROCS") == "1" and metadata.get("count per process") == "1" and metadata.get("benchtime") == "750ms", "unstable benchmark settings")
    require(metadata.get("build tags") == "treesitter_c_parity" and metadata.get("required benchmarks") == ",".join(NAMES), "benchmark surface changed")
    for seed, data in seeds.items():
        expected = 1 if (seed % 2 == 1) == (role == "base") else 2
        require(data["position"] == expected and set(data["rows"]) == set(NAMES), "paired order or benchmark completeness mismatch")
        for benchmark in WORKLOADS:
            require([name.split("/")[1] for name in data["rows"] if name.startswith(benchmark + "/")] == list(BACKENDS), "C timing is not a Go-C-C-Go cycle")
    return seeds


def combine_timing_seeds(fragments, destination, role):
    require(len(fragments) == RUNS, "incomplete timing fragments")
    combined = []
    for seed, fragment in enumerate(fragments, 1):
        parse_bench(fragment, role, expected_seeds=[seed])
        lines = Path(fragment).read_text().splitlines()
        start = next(i for i, line in enumerate(lines) if line.startswith("# seed: "))
        end = next(i for i, line in enumerate(lines) if line.startswith("# completed seed: "))
        if seed == 1:
            combined.extend(line for line in lines[:start] if not line.startswith("# seeds: "))
            combined.append(f"# seeds: 1..{RUNS}")
        combined.extend(lines[start:end + 1])
    combined.extend([f"# completed runs: {RUNS}", "# status: complete"])
    Path(destination).write_text("\n".join(combined) + "\n")
    parse_bench(destination, role)


def sample(rows, benchmark, runtime, metric):
    names = ("GoA", "GoB") if runtime == "Go" else ("CA", "CB")
    return statistics.mean(rows[benchmark + "/" + n][metric] / rows[benchmark + "/" + n]["edits/op"] for n in names)


def compare_timing(out, manifest_path, base_revision, head_revision):
    counters = compare_counters(out, manifest_path, base_revision, head_revision)
    m = fixture_manifest(manifest_path)
    rows = []
    for f in m["fixtures"]:
        language = f["language"]
        base = parse_bench(out / "timing" / (language + "-base.txt"), "base")
        head = parse_bench(out / "timing" / (language + "-head.txt"), "head")
        for benchmark, workload in WORKLOADS.items():
            edits = len(f["typing_text"].encode()) if workload == "typing" else 2
            for seeds in (base, head):
                require(all(row["edits/op"] == edits for seed in seeds.values() for name, row in seed["rows"].items() if name.startswith(benchmark + "/")), "timed session differs from admitted session")
            go = {}; c = {}; allocations = {}
            for role, seeds in (("base", base), ("head", head)):
                go[role] = [sample(data["rows"], benchmark, "Go", "ns/op") for data in seeds.values()]
                c[role] = [sample(data["rows"], benchmark, "C", "ns/op") for data in seeds.values()]
                allocations[role] = {key: statistics.median(sample(data["rows"], benchmark, "Go", key) for data in seeds.values()) for key in ("B/op", "allocs/op")}
            pairs = [after / before for before, after in zip(go["base"], go["head"])]
            ratio = statistics.median(pairs)
            row = {"language": language, "workload": workload, "fixture_bytes": f["bytes"], "edits_per_session": edits,
                   "base_go_ns": statistics.median(go["base"]), "head_go_ns": statistics.median(go["head"]),
                   "base_c_ns": statistics.median(c["base"]), "head_c_ns": statistics.median(c["head"]),
                   "base_go_c": statistics.median(g / t for g, t in zip(go["base"], c["base"])),
                   "head_go_c": statistics.median(g / t for g, t in zip(go["head"], c["head"])),
                   "paired_ratio": ratio, "paired_min": min(pairs), "paired_max": max(pairs),
                   "c_paired_ratio": statistics.median(after / before for before, after in zip(c["base"], c["head"])),
                   "allocations": allocations, "passed": ratio <= EDIT_LIMIT,
                   "samples": {"base_go_ns": go["base"], "head_go_ns": go["head"], "base_c_ns": c["base"], "head_c_ns": c["head"]}}
            rows.append(row)
            print(f"METRIC: {language}/{workload} edit Go/C | {row['base_go_c']:.3f}x -> {row['head_go_c']:.3f}x | {base_revision[:12]}..{head_revision[:12]} | {f['bytes']}B; Go {row['base_go_ns']:.0f} -> {row['head_go_ns']:.0f} ns/edit; paired {(ratio-1)*100:+.2f}% {'PASS' if row['passed'] else 'FAIL'}", flush=True)
    environment = read_json(out / "environment.json")
    require(environment["base_revision"] == base_revision and environment["head_revision"] == head_revision, "campaign revision mismatch")
    result = {"schema": "w5-editor-latency-receipt-v1", "status": "passed" if all(row["passed"] for row in rows) else "failed",
              "base_revision": base_revision, "head_revision": head_revision, "manifest_sha256": sha(Path(manifest_path).read_bytes()),
              "threshold_percent": 5, "counter_threshold_percent": 2, "seeds": list(range(1, RUNS+1)), "benchtime": "750ms",
              "protocol": "alternating-base-head-seeds; shuffled-workloads; Go-C-C-Go; complete-sessions; ns-per-edit",
              "environment": environment, "cells": rows, "counter_rows": counters["rows"], "oracles": counters["oracles"]}
    write_json(out / "receipt.json", result)
    lines = [f"W5 editor latency: **{result['status']}**", "", f"Base `{base_revision}`; head `{head_revision}`. Twenty paired shuffle seeds, 750ms, one thread. A paired median above +5% fails any cell.", "",
             "| Language | Edit | Go base → head (µs/edit) | C base → head (µs/edit) | Go/C base → head | Paired Go change |", "|---|---|---:|---:|---:|---:|"]
    for row in rows:
        lines.append(f"| {row['language']} | {row['workload']} | {row['base_go_ns']/1000:.2f} → {row['head_go_ns']/1000:.2f} | {row['base_c_ns']/1000:.2f} → {row['head_c_ns']/1000:.2f} | {row['base_go_c']:.2f}x → {row['head_go_c']:.2f}x | {(row['paired_ratio']-1)*100:+.2f}% {'PASS' if row['passed'] else '**FAIL**'} |")
    lines += ["", "C timing and the range of paired Go samples are recorded in receipt.json. C drift is reported; it does not excuse a Go regression. Tree.Edit, reparse, and previous-tree release are timed. Go fresh resets and C snapshot restoration are excluded from edit timing. C typing allocations include the small snapshot clone; C allocations cover the Go binding only; Go allocations include parser work.", ""]
    (out / "summary.md").write_text("\n".join(lines))
    return result


def run_checked(args, **kwargs):
    subprocess.run([str(arg) for arg in args], check=True, **kwargs)


def normalized_bench(source, destination):
    """Keep raw session measurements; give benchstat metrics per edit."""
    lines = []
    for line in source.read_text().splitlines():
        if line.startswith("BenchmarkW5"):
            fields = line.split()
            metrics = {fields[i+1]: float(fields[i]) for i in range(2, len(fields), 2)}
            edits = metrics.pop("edits/op")
            line = " ".join(fields[:2]) + " " + " ".join(f"{value / edits:g} {unit}" for unit, value in metrics.items())
        lines.append(line)
    destination.write_text("\n".join(lines) + "\n")


def git(root, *args):
    return subprocess.check_output(["git", "-C", str(root), *args], text=True).strip()


def clean_revision(root, expected):
    require(git(root, "rev-parse", "HEAD") == expected, "revision moved during campaign")
    run_checked(["git", "-C", root, "diff", "--exit-code", "HEAD", "--"], stdout=subprocess.DEVNULL)


def load_sample():
    cpu = Path("/proc/stat").read_text().splitlines()[0].split()[1:]
    return {"utc": datetime.datetime.now(datetime.timezone.utc).isoformat(), "load": list(os.getloadavg()), "cpu_total_jiffies": sum(map(int,cpu[:8])), "cpu_steal_jiffies": int(cpu[7])}


@contextlib.contextmanager
def campaign_lock(path=Path("/tmp/gotreesitter-editor-latency.lock")):
    # flock on Linux needs only a readable descriptor. Runner services sharing
    # a VM may use different users; appending requires the first user's write
    # permission and O_CREAT can also hit Linux's protected_regular guard.
    # Create exclusively, then open existing files without O_CREAT. Never unlink
    # the file: every runner must keep locking the same inode.
    try:
        fd = os.open(path, os.O_RDONLY | os.O_CREAT | os.O_EXCL, 0o644)
    except FileExistsError:
        fd = os.open(path, os.O_RDONLY)
    else:
        # Keep the shared lock readable even when the creator uses a strict umask.
        try:
            os.fchmod(fd, 0o644)
        except BaseException:
            os.close(fd)
            raise
    with os.fdopen(fd, "r") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        yield


def campaign_harness_command(root, out):
    return ["bash", root / "cgo_harness/docker/run_parity_in_docker.sh",
            "--out-root", out / "harness"]


def campaign_docker_command(root, base, out, cpu):
    return campaign_harness_command(root, out) + ["--no-build",
            "--memory", os.environ.get("GTS_EDITOR_LATENCY_MEMORY_LIMIT") or "8g",
            "--gomemlimit", os.environ.get("GOMEMLIMIT") or "6GiB",
            "--cpuset-cpus", str(cpu), "--cpus", "1",
            "--mount", f"{base}:/baseline:ro", "--mount", f"{out}:/campaign"]


def campaign(root, base_revision, out):
    root = root.resolve(); out = out.resolve()
    head_revision = git(root, "rev-parse", "HEAD")
    base_revision = git(root, "rev-parse", base_revision + "^{commit}")
    clean_revision(root, head_revision)
    require(not out.exists(), "campaign output already exists; do not overwrite receipts")
    out.mkdir(parents=True)
    manifest_path = root / "cgo_harness/editor_latency/fixtures.json"
    lock_path = out / "corpus.lock"
    run_checked(["bash", root / "scripts/fetch_grammar_receipt_lock.sh", os.environ.get("GTS_CORPUS_LOCK_URL", ""), lock_path])
    try:
        m = fetch_fixtures(root, manifest_path, lock_path, out / "fixtures")
    finally:
        lock_path.unlink(missing_ok=True)
    cpu = max(os.sched_getaffinity(0))
    env = {"base_revision": base_revision, "head_revision": head_revision,
           "driver_sha256": sha((root / "cgo_harness/editor_latency/editor_latency_test.go").read_bytes()),
           "go_mod_sha256": sha((root / "cgo_harness/go.mod").read_bytes()),
           "go_sum_sha256": sha((root / "cgo_harness/go.sum").read_bytes()),
           "grammar_lock_sha256": sha((root / "grammars/languages.lock").read_bytes()),
           "corpus_lock_sha256": (root / "cgo_harness/perf_scan/corpus_sources.lock.sha256").read_text().split()[0],
           "system": platform.system(), "architecture": platform.machine(), "cpu": cpu, "started": load_sample(), "samples": []}
    cpuinfo = Path("/proc/cpuinfo").read_text()
    env["cpu_model"] = next((line.split(":", 1)[1].strip() for line in cpuinfo.splitlines() if line.startswith("model name")), "unknown")
    for role in ("base", "head"):
        (out / "admission" / role).mkdir(parents=True)
        module = out / "modules" / role
        module.mkdir(parents=True)
        shutil.copyfile(root / "cgo_harness/editor_latency/editor_latency_test.go", module / "editor_latency_test.go")
        shutil.copyfile(root / "cgo_harness/go.sum", module / "go.sum")
        source = "/baseline" if role == "base" else "/workspace"
        module.joinpath("go.mod").write_text(f"""module github.com/odvcencio/gotreesitter/w5campaign

go 1.25.0

require (
 github.com/odvcencio/gotreesitter v0.1.0
 github.com/odvcencio/gotreesitter/cgo_harness v0.0.0
 github.com/tree-sitter/go-tree-sitter v0.25.0
)
replace github.com/odvcencio/gotreesitter => {source}
replace github.com/odvcencio/gotreesitter/cgo_harness => /workspace/cgo_harness
replace github.com/tree-sitter/go-tree-sitter => github.com/tree-sitter/go-tree-sitter v0.25.0
""")
    (out / "timing").mkdir()
    (out / "tools").mkdir()
    tool_env = dict(os.environ, GOWORK="off", GOMAXPROCS="1", GOBIN=str(out / "tools"))
    run_checked(["go", "install", BENCHSTAT_MODULE], env=tool_env)
    env["benchstat_module"] = BENCHSTAT_MODULE
    env["benchstat_sha256"] = sha((out / "tools/benchstat").read_bytes())
    env["image"] = "gotreesitter/cgo-harness:go1.25-local"
    run_checked(campaign_harness_command(root, out) + ["--build-only"], stdout=subprocess.DEVNULL)
    env["image_id"] = subprocess.check_output(["docker", "image", "inspect", "--format", "{{.Id}}", env["image"]], text=True).strip()
    env["go_version"] = subprocess.check_output(["docker", "run", "--rm", env["image"], "go", "version"], text=True).strip()
    write_json(out / "environment.json", env)
    with tempfile.TemporaryDirectory(prefix="gts-w5-base-") as scratch:
        base = Path(scratch) / "source"
        run_checked(["git", "-C", root, "worktree", "add", "--detach", base, base_revision])
        try:
            env["baseline_counter_accounting"] = baseline_verification_accounting(base, out)
            env["baseline_forest_accounting"] = baseline_forest_accounting(base, out)
            common = campaign_docker_command(root, base, out, cpu)
            env["docker_memory_limit"] = common[common.index("--memory") + 1]
            env["go_memory_limit"] = common[common.index("--gomemlimit") + 1]
            write_json(out / "environment.json", env)
            for f in m["fixtures"]:
                language = f["language"]
                commands = ["set -euo pipefail", "export GOWORK=off GOMAXPROCS=1 GTS_EDIT_REPO_ROOT=/workspace GTS_EDIT_MANIFEST=/workspace/cgo_harness/editor_latency/fixtures.json GTS_EDIT_FIXTURES=/campaign/fixtures", f"export GTS_EDIT_LANGUAGE={language}"]
                for role, revision in (("base", base_revision), ("head", head_revision)):
                    commands += [f"cd /campaign/modules/{role}", correctness_test_command(role, revision, language)]
                with (out / ("correctness-" + language + ".log")).open("w") as log:
                    run_checked(common + ["--label", "w5-correctness-" + language, "--wall-timeout", "35m", "--", "\n".join(commands)], stdout=log, stderr=subprocess.STDOUT)
                print("W5 four-way correctness passed:", language, flush=True)
            compare_counters(out, manifest_path, base_revision, head_revision)
            print("W5 deterministic counter gate passed before timing", flush=True)
            for f in m["fixtures"]:
                language = f["language"]
                env["samples"].append({"language": language, "before": load_sample()})
                fragments = {role: [] for role in ("base", "head")}
                rss_samples = []
                for seed in range(1, RUNS + 1):
                    prefix = f"{language}-seed-{seed}"
                    for role in fragments:
                        fragments[role].append(out / "timing" / f"{prefix}-{role}.txt")
                    command = "\n".join(["set -euo pipefail", "export GOWORK=off GOMAXPROCS=1 GTS_EDIT_REPO_ROOT=/workspace GTS_EDIT_MANIFEST=/workspace/cgo_harness/editor_latency/fixtures.json GTS_EDIT_FIXTURES=/campaign/fixtures", f"export GTS_EDIT_LANGUAGE={language}", "cd /campaign/modules/head", "/usr/bin/time -v bash /workspace/scripts/run_randomized_benchmarks.sh " + " ".join(shlex.quote(x) for x in ["--runs", "1", "--seed-start", str(seed), "--baseline-root", "/campaign/modules/base", "--baseline-output", f"/campaign/timing/{prefix}-base.txt", "--output", f"/campaign/timing/{prefix}-head.txt", "--tags", "treesitter_c_parity", "--bench-regex", "^BenchmarkW5(OneByte|HundredByte|Typing)$", "--require-benchmarks", ",".join(NAMES)])])
                    log_path = out / f"timing-{prefix}.log"
                    with log_path.open("w") as log:
                        run_checked(common + ["--label", "w5-timing-" + prefix, "--wall-timeout", "90m", "--", command], stdout=log, stderr=subprocess.STDOUT)
                    rss = re.search(r"Maximum resident set size \(kbytes\): (\d+)", log_path.read_text())
                    require(rss is not None, "timing campaign omitted maximum RSS")
                    rss_samples.append(int(rss.group(1)))
                env["samples"][-1]["after"] = load_sample()
                env["samples"][-1]["max_rss_kib"] = max(rss_samples)
                for role in ("base", "head"):
                    combine_timing_seeds(fragments[role], out / "timing" / f"{language}-{role}.txt", role)
                    normalized_bench(out / "timing" / f"{language}-{role}.txt", out / "timing" / f"{language}-{role}-per-edit.txt")
                with (out / "timing" / (language + "-benchstat.txt")).open("w") as comparison:
                    run_checked([out / "tools/benchstat", out / "timing" / (language + "-base-per-edit.txt"), out / "timing" / (language + "-head-per-edit.txt")], stdout=comparison)
                print("W5 paired 20-seed timing complete:", language, flush=True)
            clean_revision(base, base_revision)
        finally:
            run_checked(["git", "-C", root, "worktree", "remove", "--force", base])
    clean_revision(root, head_revision)
    env["finished"] = load_sample()
    write_json(out / "environment.json", env)
    result = compare_timing(out, manifest_path, base_revision, head_revision)
    require(result["status"] == "passed", "one or more edit cells regressed by more than 5%")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    run = sub.add_parser("run")
    run.add_argument("--base", required=True)
    run.add_argument("--output", type=Path, required=True)
    compare = sub.add_parser("compare")
    compare.add_argument("--output", type=Path, required=True)
    compare.add_argument("--manifest", type=Path, required=True)
    compare.add_argument("--base", required=True)
    compare.add_argument("--head", required=True)
    args = parser.parse_args()
    try:
        if args.command == "run":
            root = Path(__file__).resolve().parents[1]
            with campaign_lock():
                campaign(root, args.base, args.output)
        else:
            result = compare_timing(args.output, args.manifest, args.base, args.head)
            require(result["status"] == "passed", "one or more edit cells regressed by more than 5%")
    except (ValueError, OSError, KeyError, subprocess.CalledProcessError) as error:
        print(f"W5 gate failed: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
