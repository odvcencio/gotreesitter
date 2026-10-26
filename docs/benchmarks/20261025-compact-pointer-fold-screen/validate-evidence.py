#!/usr/bin/env python3
"""Validate the held pointer-fold screen without executing parser code."""
from pathlib import Path, PurePosixPath
import argparse
import hashlib
import importlib.util
import json
import re
import sys

sys.dont_write_bytecode = True
sha = lambda data: hashlib.sha256(data).hexdigest()


def module(path, name):
    spec = importlib.util.spec_from_file_location(name, path)
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


def validate(directory, source_root=None):
    raw = directory.parent / "20261025-compact-raw-screen"
    construction = directory.parent / "20261025-compact-construction-screen"
    assert sha((raw / "evidence.tar.gz").read_bytes()) == "73fe52b4ef05a02a6741265da9b447ed3e2610338880592a6d1ff57c5f8c90fa"
    assert sha((construction / "evidence.tar.gz").read_bytes()) == "f21fcaa7c7a558f32dc351a2d58a5ef04696faf344b158abf7713a6d9d1d518b"
    old = module(raw / "validate-evidence.py", "pointer_fold_raw_base")
    held = module(construction / "validate-evidence.py", "pointer_fold_construction_base")
    files = old.read_archive(raw)
    prior = held.load_delta(construction)
    delta = held.load_delta(directory)
    assert not (files.keys() & prior.keys())
    files.update(prior)
    assert not (files.keys() & delta.keys())
    files.update(delta)
    checked = 0
    for name, data in delta.items():
        if PurePosixPath(name).name != "controls.sha256":
            continue
        for line in data.decode().splitlines():
            digest, virtual = line.split("  ", 1)
            if not virtual.startswith("/controls/"):
                continue
            target = virtual.lstrip("/")
            if target not in files and virtual.startswith("/controls/out/"):
                target = "outputs/" + virtual.removeprefix("/controls/out/")
            assert target in files and sha(files[target]) == digest, (name, target)
            checked += 1
    runtime_name = "controls/raw-summary-output-v1/pure-gap-fused-v2/overlay.runtime.json"
    assert sha(files[runtime_name]) == "6f2d518d0051e520644a32cf2991ada86231cc7d40cc952109956428172070fe"
    runtime = json.loads(files[runtime_name])["Replace"]
    timed = json.loads(files["controls/raw-summary-perf-pure-gap-v2/overlay.container.json"])["Replace"]
    assert len(runtime) == 28 and len(timed) == 30
    for target, backing in runtime.items():
        assert timed[target] == backing and backing.lstrip("/") in files
    wrapper = "/controls/raw-summary-perf-pure-gap-v2/benchmark-source.go.txt"
    assert timed["/baseline/benchmark_admission_candidate_route_test.go"] == timed["/candidate/benchmark_admission_candidate_route_test.go"] == wrapper
    assert sha(files[wrapper.lstrip("/")]) == "2d55591291b04e514b505b0671a7f912fd5bdc5d8c42eff8bf1c074d94e00cb4"
    stages = {
        "raw-summary-pure-gap-v2b": {"core-release.log": (83, 173), "core-workcount.log": (19, 34), "core-race.log": (19, 34), "ordinary-root.log": (6, 16), "ordinary-cgo.log": (1, 7)},
        "raw-summary-work-pure-gap-v2b": {f"work-{name}.log": (1, 2) for name in ("primary32", "primary128", "primary512", "primary1024", "real-r4")},
        "raw-summary-semantic-pure-gap-v2b": old.GREEN["raw-summary-semantic-v6"],
        "raw-summary-public-race-pure-gap-v2b": {"public-race.log": (5, 13)},
        "raw-summary-c027-pure-gap-v2b": {"runtime-probe.log": (1, 1), "ordinary-cgo.log": (1, 7)},
        "raw-summary-pure-gap-focused-v2b": {name: (12, 71) for name in ("focused.log", "workcount.log", "race.log")},
        "raw-summary-pure-gap-refusals-v3": {name: (1, 9) for name in ("focused.log", "workcount.log", "race.log")},
    }
    gates = {name: old.stage(files, name, logs) for name, logs in stages.items()}
    ordinary = old.out(files, "raw-summary-pure-gap-v2b", "ordinary-root.log")
    assert ordinary.count("lease:true") == ordinary.count("arena_nodes:1") == ordinary.count("core_owned_raw_records:0") == 4
    c025 = old.c_cases(files, "raw-summary-pure-gap-v2b")
    c027 = old.c_cases(files, "raw-summary-c027-pure-gap-v2b")
    assert c025["full_tree_digests"] == c027["full_tree_digests"]
    assert '"runtime_commit":"10eb5ac119eec9cb4267a8e035066473983f5f3a"' in old.out(files, "raw-summary-c027-pure-gap-v2b", "runtime-probe.log")
    for name in ("oracle-before", "oracle-after", "binding-before", "binding-after"):
        assert old.out(files, "raw-summary-c027-pure-gap-v2b", name + ".log") == ""
    virtual = dict(files)
    prefix = "outputs/raw-summary-semantic-pure-gap-v2b/"
    for name, data in files.items():
        if name.startswith(prefix):
            virtual["outputs/raw-summary-semantic-v6/" + name[len(prefix):]] = data
    counters = old.counters(virtual)
    before = json.loads(files["outputs/raw-summary-work-fused-v7/validated-summary.json"])["sources"]
    after = json.loads(files["outputs/raw-summary-work-pure-gap-v2b/validated-summary.json"])["sources"]
    assert before == after and len(after) == 5
    for row in after:
        text = old.out(files, "raw-summary-work-pure-gap-v2b", f"work-{row['Fixture']}.log")
        actual = [json.loads(line.split("RAW_SUMMARY_ORDINARY_WORK ", 1)[1]) for line in text.splitlines() if "RAW_SUMMARY_ORDINARY_WORK " in line]
        assert actual == [row] and row["After"] == row["Warm"]
    old.stage(files, "raw-summary-pure-gap-focused-v1", {name: (12, 71) for name in ("focused.log", "workcount.log", "race.log")})
    text = old.out(files, "raw-summary-pure-gap-focused-v1", "workcount.log")
    assert re.search(r"candidate=\{ClosureRawEdges:2 FormationRawEdges:1 .*control=\{ClosureRawEdges:2 FormationRawEdges:0 ", text)
    text = old.out(files, "raw-summary-pure-gap-focused-v2b", "workcount.log")
    assert text.count("strict closure-first physical work equality: candidate={ClosureRawEdges:2 FormationRawEdges:0") == 2
    assert "outputs/raw-summary-perf-pure-gap-v1/started.json" not in files
    assert json.loads(files["outputs/raw-summary-pure-gap-focused-v2/finished.json"])["exit_code"] == 126
    assert "permission denied" in old.out(files, "raw-summary-pure-gap-focused-v2", "container.log")
    assert json.loads(files["outputs/raw-summary-pure-gap-v2/finished.json"])["exit_code"] == 1
    assert "No such file or directory" in old.out(files, "raw-summary-pure-gap-v2", "controls-before.log")
    assert "outputs/raw-summary-pure-gap-v2/core-release.log" not in files
    old.PERF = "raw-summary-perf-pure-gap-v2"
    perf = old.perf_proof(files)
    saved = json.loads((directory / "summary.json").read_text())
    assert saved["results"] == perf["results"] and saved["archive_members"] == len(delta)
    assert saved["actual_counter_rows"] == counters["actual_rows"]
    assert not saved["V1_timing_executed"] and not saved["private_prototype_promoted"]
    source_checks = 0
    if source_root:
        for line in files["controls/raw-summary-perf-pure-gap-v2/source.sha256"].decode().splitlines():
            digest, name = line.split("  ", 1)
            path = PurePosixPath(name)
            assert not path.is_absolute() and ".." not in path.parts
            assert sha((Path(source_root) / name).read_bytes()) == digest, name
            source_checks += 1
    return {"schema": "gts-compact-pointer-fold-proof/v1", "delta_members": len(delta),
            "inherited_members": len(files) - len(delta), "checksum_entries": checked,
            "source_files_verified": source_checks, "runtime_targets": 28,
            "gates": gates, "C025": c025, "C027": c027, "counters_and_D8": counters,
            "ordinary_work_equal_to_fused_v7": True, "V1_refusal_increase_preserved": True,
            "V1_timing_executed": False, "V2_refusal_work_equal": True,
            "paired_timing": perf, "runtime_promoted": False,
            "graduation_claim": False, "main_merges_held": True}


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--source-root")
    args = parser.parse_args()
    directory = Path(__file__).resolve().parent
    if (directory / "SHA256SUMS").exists():
        for line in (directory / "SHA256SUMS").read_text().splitlines():
            digest, name = line.split("  ", 1)
            assert PurePosixPath(name).name == name
            assert sha((directory / name).read_bytes()) == digest, name
    result = validate(directory, args.source_root)
    if (directory / "verification.json").exists():
        expected = json.loads((directory / "verification.json").read_text())
        assert {k: v for k, v in result.items() if k != "source_files_verified"} == {k: v for k, v in expected.items() if k != "source_files_verified"}
    print(json.dumps(result, indent=2))
