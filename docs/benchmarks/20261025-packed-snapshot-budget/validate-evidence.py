"""Validate retained evidence only; no builds, parser execution, or writes."""
from pathlib import Path
from collections import Counter
import hashlib, json, re, tarfile

root = Path(__file__).resolve().parent
index = json.loads((root / "contents.json").read_text())["files"]
with tarfile.open(root / "evidence.tar.gz", "r:gz") as archive:
    entries = archive.getmembers()
    assert len(entries) == len({e.name for e in entries}) == len(index)
    data = {}
    for entry in entries:
        assert entry.isfile() and entry.mode == 0o644
        assert entry.name in index and not entry.name.startswith("/") and ".." not in Path(entry.name).parts
        assert not entry.name.endswith((".csv", ".test", ".so", ".a", ".pprof", ".go.txt"))
        content = archive.extractfile(entry).read()
        record = index[entry.name]
        assert len(content) == record["bytes"] and hashlib.sha256(content).hexdigest() == record["sha256"], entry.name
        data[entry.name] = content
prefix = "harness_out/docker/go-ci-blockers-20261025/"
def raw(name):
    return data[prefix + name]
def text(name):
    return raw(name).decode()
def parsed(name):
    return json.loads(raw(name))
assert parsed("out/packed-snapshot-budget-v1/process-status.json") == {"baseline_exit": 1, "candidate_exit": 0, "existing_memory_exit": 0}
for variant in ["v1", "final", "race"]:
    base = "out/packed-snapshot-budget-" + variant + "/"
    assert parsed(base + "finished.json")["exit_code"] == 0
    assert raw(base + "source-before.log") == raw(base + "source-after.log") == b""
langs = text("fleet-controls/languages.txt").splitlines()
assert len(langs) == len(set(langs)) == 206
fixtures, steps, c_checks, equal_rows = Counter(), 0, 0, 0
for route in ["0", "1"]:
    for lang in langs:
        log = text(f"out/packed-snapshot-budget-final/invariants/{route}-{lang}.log")
        assert not re.search(r"--- (FAIL|SKIP):|^panic:|^fatal error:", log, re.M)
        match = re.search(r"language=(\S+) fixture=(\S+) sample_bytes=(\d+) session_steps=(\d+) no_edit_allocs=(\d+)", log)
        assert match and match[1] == lang and match[4] == "72" and match[5] == "0"
        fixtures[match[2]] += 1
        steps += int(match[4])
for lang in langs:
    log = text(f"out/packed-snapshot-budget-final/cgo/{lang}.log")
    assert not re.search(r"--- (FAIL|SKIP):|^panic:|^fatal error:", log, re.M)
    for kind in ["FreshParse", "IncrementalParse", "HasNoErrors"]:
        assert len(re.findall(r"^    --- PASS: TestParity" + kind + "/" + re.escape(lang) + r" ", log, re.M)) == 1
        c_checks += 1
for route in ["default", "candidate"]:
    for lang in langs:
        name = route + "-" + lang
        assert parsed("out/ci-followup-final/ledger/" + name + ".json") == parsed("out/packed-snapshot-budget-final/ledger/" + name + ".json")
        assert raw("out/packed-snapshot-budget-final/ledger/" + name + ".stderr") == b""
        equal_rows += 1
assert fixtures == {"r4": 408, "grammar-smoke": 4} and steps == 29664 and c_checks == 618 and equal_rows == 412
race_top, race_all = 0, 0
for name in ["race-root.log", "race-pages.log"]:
    log = text("out/packed-snapshot-budget-race/" + name)
    assert not re.search(r"--- (FAIL|SKIP):|WARNING: DATA RACE|^panic:|^fatal error:", log, re.M)
    race_top += len(re.findall(r"^--- PASS:", log, re.M))
    race_all += len(re.findall(r"--- PASS:", log))
assert (race_top, race_all) == (9, 12)
recipe = json.loads(data["package/original-controls-reconstruction.json"])
patch = data["package/source.patch"].decode()
section = patch.split("diff --git a/parsercore_phase0_per_version_lexer_snapshot_internal_test.go", 1)[1].split("diff --git ", 1)[0]
added = "".join(line[1:] for line in section.splitlines(True) if line.startswith("+") and not line.startswith("+++"))
body = added[added.index(recipe["appended_block_starts_with"]):]
for before, after in recipe["rename_identifiers"].items():
    body = body.replace(before, after)
reconstructed = (recipe["prefix"] + body).encode()
assert hashlib.sha256(reconstructed).hexdigest() == recipe["expected_private_test_sha256"]
assert data["package/source.patch"] == (root / "source.patch").read_bytes()
identities = json.loads(data["package/source-identities.json"])
assert hashlib.sha256(raw("packed-snapshot-budget-final/source.sha256")).hexdigest() == identities["final_full_source_manifest_sha256"]
print(json.dumps({"archive_files": len(index), "baseline_exit": 1, "corrected_exit": 0, "sessions": 412, "steps": steps, "no_edit_allocations": 0, "C_checks": c_checks, "actual_rows_equal": equal_rows, "race_top_level": race_top, "race_pass_records": race_all, "private_test_reconstruction": "exact"}, indent=2))
