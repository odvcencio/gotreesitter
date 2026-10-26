#!/usr/bin/env python3
"""Validate this receipt using Python's standard library; never run Go or benchstat."""
from pathlib import Path, PurePosixPath
import argparse
import gzip
import hashlib
import json
import re
import statistics
import tarfile


RUNTIME_SHA = "aef67f5f76465be98c9587a27d44ff242db8e59a555fb622aa413e381fb14ca8"


def digest(data):
    return hashlib.sha256(data).hexdigest()


def text(files, name):
    return files[name].decode("utf-8")


def obj(files, name):
    return json.loads(text(files, name))


def selected(files, folder, expected, exit_code=0):
    prefix = "outputs/" + folder + "/"
    assert obj(files, prefix + "finished.json")["exit_code"] == exit_code, folder
    for name in ("source-before.log", "source-after.log", "controls-before.log", "controls-after.log"):
        assert text(files, prefix + name) == "", (folder, name)
    result = {}
    for name, top_count in expected.items():
        value = text(files, prefix + name)
        tops = re.findall(r"^--- PASS: (\S+)", value, re.M)
        assert len(tops) == top_count, (folder, name, tops)
        assert not re.search(r"--- (FAIL|SKIP):|WARNING: DATA RACE|^FAIL\b|panic:", value, re.M), (folder, name)
        result[name] = {"top_level_passes": len(tops), "all_pass_records": len(re.findall(r"^\s*--- PASS:", value, re.M)), "selected_families": tops}
    return result


def validate_controls(files):
    checked = 0
    for name, data in files.items():
        path = PurePosixPath(name)
        if not name.startswith("controls/") or path.name not in ("controls.sha256", "SHA256SUMS"):
            continue
        for line in data.decode().splitlines():
            expected, relative = line.split("  ", 1)
            if relative.startswith("/controls/"):
                target = "controls/" + relative.removeprefix("/controls/")
            else:
                assert not relative.startswith("/") and ".." not in PurePosixPath(relative).parts, (name, relative)
                target = str(path.parent / relative)
            assert target in files and digest(files[target]) == expected, (name, target)
            checked += 1
    runtime_name = "controls/sealed-reduction-cell-v1/overlay.json"
    assert digest(files[runtime_name]) == RUNTIME_SHA
    original = obj(files, runtime_name)["Replace"]
    measured = obj(files, "controls/sealed-reduction-perf-v1/overlay.container.json")["Replace"]
    assert len(original) == 10 and len(measured) == 12
    assert all(measured[key] == value for key, value in original.items())
    added = set(measured) - set(original)
    assert added == {"/candidate/benchmark_admission_candidate_route_test.go", "/baseline/benchmark_admission_candidate_route_test.go"}
    assert len({measured[key] for key in added}) == 1
    assert not any("activation" in value or "native-syntax" in value for value in measured.values())
    manifest = obj(files, "controls/sealed-reduction-cell-v1/manifest.json")
    for row in manifest["files"]:
        assert digest(files["controls/sealed-reduction-cell-v1/" + row["backing"]]) == row["candidate_sha256"], row["backing"]
    return {"checksum_entries_verified": checked, "frozen_measured_runtime_sha256": RUNTIME_SHA, "runtime_targets": 10, "activation_instrumentation_in_measured_runtime": False}


def validate_performance(files):
    folder = "outputs/sealed-reduction-perf-v1/"
    assert obj(files, folder + "finished.json")["exit_code"] == 0
    assert folder + "completed.txt" in files
    for name in ("source-before.log", "source-after.log", "controls-before.log", "controls-after.log"):
        assert text(files, folder + name) == "", name
    assert obj(files, folder + "gates-before.json")["selected_semantic_gates_passed"]
    reports = {}
    for series, pattern, wanted in (
        ("go-primary", "primary-{}.txt", ["BenchmarkGraduationGoSuite/Full", "BenchmarkGraduationGoSuite/Edit", "BenchmarkGraduationGoSuite/NoEdit"]),
        ("go-r4", "r4-{}-go.txt", ["BenchmarkCompactGraduationR4/legacy", "BenchmarkCompactGraduationR4/compact"]),
        ("yaml-r4", "r4-{}-yaml.txt", ["BenchmarkCompactGraduationR4/legacy", "BenchmarkCompactGraduationR4/compact"]),
    ):
        samples, orders, raw_ids = {}, {}, {}
        for arm in ("before", "after"):
            name = folder + pattern.format(arm)
            value = text(files, name)
            raw_ids[arm] = digest(files[name])
            assert value.rstrip().endswith("# status: complete"), name
            assert not re.search(r"^FAIL|--- (FAIL|SKIP):|panic:|WARNING: DATA RACE", value, re.M), name
            for header in ("# protocol: paired-alternating-seeds", "# seeds: 1..20", "# benchtime: 750ms", "# GOMAXPROCS: 1", "# count per process: 1", "# build tags: gts_parsercorephase0"):
                assert header in value, (name, header)
            assert list(map(int, re.findall(r"^# completed seed: (\d+);", value, re.M))) == list(range(1, 21)), name
            starts = list(re.finditer(r"^# seed: (\d+); position: (\d+)\n", value, re.M))
            assert len(starts) == 20, name
            by_seed, order_by_seed = {}, {}
            for index, match in enumerate(starts):
                seed, position = map(int, match.groups())
                assert seed == index + 1
                expected = (1 if seed % 2 else 2) if arm == "before" else (2 if seed % 2 else 1)
                assert position == expected, (name, seed, position)
                chunk = value[match.end():starts[index + 1].start() if index + 1 < len(starts) else len(value)]
                order = re.findall(r"^# workload order: (.*)", chunk, re.M)
                assert len(order) == 1 and sorted(order[0].split(",")) == sorted(wanted), (name, seed, order)
                order_by_seed[seed] = order[0]
                rows = {}
                for line in chunk.splitlines():
                    if not line.startswith("Benchmark"):
                        continue
                    benchmark = line.split()[0]
                    assert benchmark in wanted and benchmark not in rows, (name, seed, line)
                    iterations = re.search(r"\s(\d+)\s+([0-9.]+) ns/op", line)
                    assert iterations and int(iterations[1]) > 0
                    metrics = {unit: float(number) for number, unit in re.findall(r"([0-9.]+) (ns/op|B/op|allocs/op|compact-routes/op|compact-fallbacks/op)", line)}
                    assert {"ns/op", "B/op", "allocs/op"} <= metrics.keys() and metrics["ns/op"] > 0
                    if series != "go-primary":
                        assert metrics.get("compact-fallbacks/op") == 0 and metrics.get("compact-routes/op") == (1 if benchmark.endswith("/compact") else 0)
                    elif benchmark.endswith("/NoEdit"):
                        assert metrics["B/op"] == metrics["allocs/op"] == 0
                    rows[benchmark] = metrics
                assert set(rows) == set(wanted), (name, seed, rows)
                by_seed[seed] = rows
            samples[arm], orders[arm] = by_seed, order_by_seed
        assert orders["before"] == orders["after"], series
        result = {}
        for benchmark in wanted:
            result[benchmark] = {}
            for metric in ("ns/op", "B/op", "allocs/op"):
                before = [samples["before"][seed][benchmark][metric] for seed in range(1, 21)]
                after = [samples["after"][seed][benchmark][metric] for seed in range(1, 21)]
                bmed, amed = statistics.median(before), statistics.median(after)
                ratios = [a / b for a, b in zip(after, before)] if all(before) else None
                result[benchmark][metric] = {"before_median": bmed, "after_median": amed, "change_percent": None if bmed == 0 else (amed / bmed - 1) * 100, "median_paired_after_before_ratio": None if ratios is None else statistics.median(ratios), "ratio_of_medians": None if bmed == 0 else amed / bmed}
        route_ratios = {}
        if series != "go-primary":
            legacy, compact = wanted
            for arm in ("before", "after"):
                ratios = [samples[arm][seed][compact]["ns/op"] / samples[arm][seed][legacy]["ns/op"] for seed in range(1, 21)]
                route_ratios[arm] = {"median_paired_compact_legacy_ratio": statistics.median(ratios), "ratio_of_medians": statistics.median(samples[arm][seed][compact]["ns/op"] for seed in range(1, 21)) / statistics.median(samples[arm][seed][legacy]["ns/op"] for seed in range(1, 21))}
        reports[series] = {"paired_samples": 20, "raw_sha256": raw_ids, "results": result, "route_ratios": route_ratios}
    assert reports == obj(files, folder + "validated-summary.json")["results"], "saved medians or ratios disagree with raw samples"
    return {"pairs": 60, "processes": 120, "failed_or_incomplete_processes": 0, "no_edit_allocations": 0, "compact_fallbacks": 0, "results": reports, "significance_validation": "benchstat reports retained; p-values are not recomputed by this standard-library validator"}


def validate_payloads(files):
    controls = validate_controls(files)
    historical = "outputs/sealed-reduction-initial-qualification-v1/"
    assert obj(files, historical + "finished.json")["exit_code"] == 1
    for name in ("source-before.log", "source-after.log", "controls-before.log", "controls-after.log"):
        assert text(files, historical + name) == ""
    failures = re.findall(r"^--- FAIL: (\S+)", text(files, historical + "core-release.log"), re.M)
    assert failures == ["TestSealedReductionExecutionCollisionConsumesExactGotoMemo", "TestSealedReductionExecutionGotoMutationPoisonsFreshOwner"]
    assert not re.search(r"--- SKIP:|WARNING: DATA RACE|panic:", text(files, historical + "core-release.log"))
    adjudication = selected(files, "sealed-reduction-adjudication-v1", {"adjudication.log": 1})
    proof = text(files, "outputs/sealed-reduction-adjudication-v1/adjudication.log")
    assert proof.count("SEALED_WITNESS collision bound=") == 2 and proof.count("SEALED_WITNESS frontier bound=") == 2
    assert proof.count("old_equal=false callback_equal=true") == 2
    assert text(files, "outputs/sealed-reduction-adjudication-v1/test-exit.txt").strip() == "0"
    initial = selected(files, "sealed-reduction-initial-qualification-v2", {"core-release.log": 23, "core-workcount.log": 23, "core-clone.log": 1, "ordinary-go.log": 1, "ordinary-yaml.log": 1})
    selected(files, "sealed-reduction-activation-v1", {"activation-go.log": 1, "activation-yaml.log": 1})
    activation = {}
    for language in ("go", "yaml"):
        value = text(files, f"outputs/sealed-reduction-activation-v1/activation-{language}.log")
        rows = re.findall(r"SEALED_PROVIDER_ACTIVATION grammar=(\w+) enabled=(false|true) actual_plan_provider_calls=(\d+) actual_goto_provider_calls=(\d+) Core_reductions=(\d+)", value)
        assert len(rows) == 2 and all(row[0] == language for row in rows)
        arms = {row[1]: tuple(map(int, row[2:])) for row in rows}
        bp, bg, br = arms["false"]
        ap, ag, ar = arms["true"]
        assert bp > ap and bg == ag > 0 and br == ar > 0
        activation[language] = {"avoided": bp - ap, "before_plans": bp, "after_plans": ap, "equal_gotos": bg, "equal_core_reductions": br}
    saved = obj(files, "outputs/sealed-reduction-activation-v1/validated-summary.json")
    assert saved["actual_activation_proved"] and all(saved[key] == value for key, value in activation.items())
    semantic = selected(files, "sealed-reduction-semantic-v1", {"core-reference.log": 55, "core-race.log": 23, "focused.log": 36, "go-public-api.log": 3, "go-cgo.log": 10, "go-report-cgo.log": 3, "yaml-cgo.log": 8, **{f"invariants-{language}-{route}.log": 1 for language in ("go", "yaml") for route in (0, 1)}})
    counters = {}
    for language in ("go", "yaml"):
        for route in ("default", "candidate"):
            name = f"{route}-{language}.json"
            baseline, actual = obj(files, "baselines/" + name), obj(files, "outputs/sealed-reduction-semantic-v1/" + name)
            assert json.dumps(baseline, sort_keys=True) == json.dumps(actual, sort_keys=True), name
            counters[name] = {"exact_equal": True, "baseline_sha256": digest(files["baselines/" + name]), "actual_sha256": digest(files["outputs/sealed-reduction-semantic-v1/" + name])}
        for route in (0, 1):
            value = text(files, f"outputs/sealed-reduction-semantic-v1/invariants-{language}-{route}.log")
            assert "session_steps=72 no_edit_allocs=0" in value
    memory = selected(files, "sealed-reduction-memory-race-v1", {"cold-go.log": 1, "cold-yaml.log": 1, "existing-cold.log": 2, "public-race.log": 3})
    cold = {}
    for language in ("go", "yaml"):
        value = text(files, f"outputs/sealed-reduction-memory-race-v1/cold-{language}.log")
        record = re.findall(r"SEALED_COLD_TABLE (.*)", value)
        assert len(record) == 1
        fields = dict(re.findall(r"(\w+)=([^\s]+)", record[0]))
        assert fields["grammar"] == language
        for key in ("total_rows", "populated_rows", "sealed_rows", "set_slot_bytes", "set_record_bytes", "row_slot_bytes", "authority_bytes_each", "authorities_bytes", "unique_increment_bytes", "complete_heap_delta_bytes", "heap_guard_bytes", "shared_existing_plan_buffers_charged_again"):
            fields[key] = int(fields[key])
        assert fields["total_rows"] >= fields["populated_rows"] >= fields["sealed_rows"] > 0
        assert fields["row_slot_bytes"] == fields["populated_rows"] * fields["set_slot_bytes"]
        assert fields["authorities_bytes"] == fields["sealed_rows"] * fields["authority_bytes_each"]
        assert fields["unique_increment_bytes"] == sum(fields[key] for key in ("set_slot_bytes", "set_record_bytes", "row_slot_bytes", "authorities_bytes"))
        assert fields["shared_existing_plan_buffers_charged_again"] == 0
        assert fields["complete_heap_delta_bytes"] <= fields["heap_guard_bytes"] == 4 << 20
        cold[language] = fields
    return {"controls": controls, "historical_failed_families_preserved": failures, "adjudication": adjudication, "repaired_initial": initial, "activation": activation, "semantic": semantic, "counter_rows": counters, "D8_sessions": 4, "D8_steps": 288, "no_edit_allocations": 0, "memory_and_public_race": memory, "cold_tables": cold, "performance": validate_performance(files), "fleet_qualified": False, "E_A_qualified": False, "runtime_promoted": False, "merges_held": True}


def read_archive(directory):
    checked = set()
    for line in (directory / "SHA256SUMS").read_text().splitlines():
        expected, name = line.split("  ", 1)
        assert PurePosixPath(name).name == name and digest((directory / name).read_bytes()) == expected, name
        assert name not in checked
        checked.add(name)
    assert checked == {"evidence.tar.gz", "contents.json.gz", "source-identities.json", "verification.json", "summary.json", "validate-evidence.py"}
    index = json.loads(gzip.decompress((directory / "contents.json.gz").read_bytes()))
    expected = {row["path"]: row for row in index}
    assert len(expected) == len(index)
    files = {}
    with tarfile.open(directory / "evidence.tar.gz", "r:gz") as archive:
        for member in archive:
            path = PurePosixPath(member.name)
            assert not path.is_absolute() and ".." not in path.parts and member.isfile() and member.name not in files
            data = archive.extractfile(member).read()
            row = expected[member.name]
            assert len(data) == row["bytes"] == member.size and digest(data) == row["sha256"], member.name
            files[member.name] = data
    assert set(files) == set(expected)
    return files


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source-root", type=Path, help="optionally verify the complete archived canonical source census against a local checkout")
    args = parser.parse_args()
    directory = Path(__file__).resolve().parent
    files = read_archive(directory)
    assert Path(__file__).read_bytes() == files["controls/sealed-reduction-screen-package-v1/validate-evidence.py.txt"]
    verified = validate_payloads(files)
    assert verified == json.loads((directory / "verification.json").read_text()), "published verification differs from raw evidence"
    summary = json.loads((directory / "summary.json").read_text())
    assert summary["performance"] == verified["performance"] and summary["root_decision"]["decision"]
    identities = json.loads((directory / "source-identities.json").read_text())
    assert identities["measured_runtime_overlay_sha256"] == RUNTIME_SHA
    assert identities["canonical_source_manifest_sha256"] == digest(files["controls/sealed-reduction-perf-v1/source.sha256"])
    for name, expected in identities["artifact_controls"].items():
        assert digest(files[name]) == expected, name
    if args.source_root:
        for line in text(files, "controls/sealed-reduction-perf-v1/source.sha256").splitlines():
            expected, name = line.split("  ", 1)
            path = PurePosixPath(name)
            assert not path.is_absolute() and ".." not in path.parts
            assert digest((args.source_root / name).read_bytes()) == expected, name
    print(json.dumps({"validated": True, "members": len(files), "pairs": 60, "processes": 120, "activation": verified["activation"], "exact_counter_rows": 4, "D8_sessions": 4, "D8_steps": 288, "no_edit_allocations": 0, "runtime_promoted": False, "fleet_qualified": False, "E_A_qualified": False, "merges_held": True}, indent=2))


if __name__ == "__main__":
    main()
