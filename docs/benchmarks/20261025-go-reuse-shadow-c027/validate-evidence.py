#!/usr/bin/env python3
"""Validate the recorded C027 supplement and its immutable source archives."""
from pathlib import Path, PurePosixPath
import hashlib
import json
import re
import tarfile

ROOT = Path(__file__).resolve().parent
sha = lambda b: hashlib.sha256(b).hexdigest()


def read_archive(path, expected):
    assert sha(path.read_bytes()) == expected, (path, "archive hash")
    entries = {}
    with tarfile.open(path) as archive:
        for member in archive:
            name = PurePosixPath(member.name)
            assert member.isfile() and not name.is_absolute() and ".." not in name.parts
            assert member.name not in entries, (path, "duplicate", member.name)
            entries[member.name] = archive.extractfile(member).read()
    return entries


def main():
    summary = json.loads((ROOT / "summary.json").read_text())
    actual = read_archive(ROOT / "evidence.tar.gz", summary["supplement_sha256"])
    index = json.loads((ROOT / "contents.json").read_text())
    assert set(actual) == set(index)
    for name, data in actual.items():
        assert index[name] == {"bytes": len(data), "sha256": sha(data)}, name
    all_entries = {}
    for name, expected in summary["inherited_archives"].items():
        inherited = read_archive(ROOT / name, expected)
        for key, data in inherited.items():
            # The archives intentionally carry different test maps; source
            # collisions are resolved by looking up the expected checksum.
            all_entries.setdefault(key, []).append(data)
    for name, data in actual.items():
        all_entries.setdefault(name, []).append(data)

    def bound(virtual, expected=None):
        key = virtual.lstrip("/")
        choices = all_entries.get(key, [])
        if expected is not None:
            choices = [data for data in choices if sha(data) == expected]
        assert choices, (virtual, "missing source binding", expected)
        assert all(data == choices[0] for data in choices), (virtual, "ambiguous binding")
        return choices[0]

    prefix = "out/trusted-go-shadow-c027-v1/"
    assert json.loads(actual[prefix + "finished.json"])["exit_code"] == 0
    for stem in ("source", "controls", "oracle", "binding"):
        for phase in ("before", "after"):
            assert actual[prefix + f"{stem}-{phase}.log"] == b""
    assert actual[prefix + "cgo-build.log"] == actual[prefix + "container.log"] == b""
    for name, counts in (("runtime-probe.log", (1, 1)), ("existing-strong-C-Go.log", (3, 127))):
        text = actual[prefix + name].decode()
        assert not re.search(r"--- (FAIL|SKIP):|^FAIL\b|panic:|WARNING: DATA RACE", text, re.M)
        observed = (len(re.findall(r"^--- PASS:", text, re.M)), len(re.findall(r"^\s*--- PASS:", text, re.M)))
        assert observed == counts, (name, observed)
    assert summary["runtime_commit"] in actual[prefix + "runtime-probe.log"].decode()
    for phase in ("before", "after"):
        stage = json.loads(actual[prefix + f"staged-{phase}.json"])
        assert stage["cgo_files"] == 14 and stage["bodies_byte_identical_to_frozen_adaptation"]
        assert not stage["module_cache_changed"] and not stage["project_dependencies_changed"]
    controls = "controls/trusted-go-shadow-c027-v1/"
    for line in actual[controls + "controls.sha256"].decode().splitlines():
        digest, virtual = line.split(None, 1)
        bound(virtual.strip(), digest)
    runtime_bytes = actual["controls/trusted-go-shadow-v1/overlay.runtime.json"]
    assert sha(runtime_bytes) == summary["runtime_map_sha256"] == "b8516549c515222d8aeb5d4b4a2609b50d8abbcbd4b691e091d73aff7f40f49d"
    runtime = json.loads(runtime_bytes)["Replace"]
    composed = json.loads(actual[controls + "overlay.c027.json"])["Replace"]
    assert len(runtime) == 11 and len(composed) == 27
    assert all(composed.get(target) == backing for target, backing in runtime.items())
    for backing in composed.values():
        bound(backing)
    assert summary["fresh_verifier_active"] and summary["main_merges_held"]
    assert not any(summary[key] for key in ("dependency_changed", "runtime_promoted", "performance_qualified", "counter_gate_passed", "graduation_claim"))
    print(json.dumps({"validated": True, "C_families": 3, "C_records": 127,
                      "runtime_probe": True, "runtime_targets": 11,
                      "composed_targets": 27, "fresh_verifier_active": True,
                      "performance_qualified": False, "main_merges_held": True}))


if __name__ == "__main__":
    main()
