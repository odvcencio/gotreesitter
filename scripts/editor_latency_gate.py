#!/usr/bin/env python3
"""W5 paired edit gate. Fail closed on incomplete or incomparable evidence."""

import argparse
import contextlib
import datetime
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

from w5_private_oracle import check_env, digest, identity_fields, source_fingerprints


WORKLOADS = {"BenchmarkW5OneByte": "one_byte", "BenchmarkW5HundredByte": "hundred_byte", "BenchmarkW5Typing": "typing"}
BACKENDS = ("GoA", "CA", "CB", "GoB")
NAMES = [f"{benchmark}/{backend}" for benchmark in WORKLOADS for backend in BACKENDS]
WORK_COUNTERS = ("tokens", "nodes", "max_stacks")
REUSE_COUNTERS = ("reused_subtrees", "reused_bytes")
EDIT_LIMIT = 1.05
LEDGER_LIMIT = 1.02
RUNS = 20
BENCHSTAT_MODULE = "golang.org/x/perf/cmd/benchstat@v0.0.0-20260929162123-406019bb8b68"

# The owner approved only this exact failure multiset, after strict CI
# reproduced it for the refreshed source pair. This is not a CLI opt-in.
POWERSHELL_W5_DECISION_SHA256 = "485d80df01502acad8299f8177a581b1b044ef0fe61ef746899c914e0665890c"
POWERSHELL_W5_ROWS_SHA256 = "7533831c95279998d53b274c99150fbbbd9d8ac7a324fe0f17c61db93a9d3f02"
POWERSHELL_W5_POLICY = {
    "fixture_manifest_sha256": "110352e799c54aced0f169e3852d1f58d6ff7d33abf4412a4cc21842cdf7bd37",
    "fixture_sha256": "7210a67d7bc3039211067e0a5a9a3731cb340477673fed80b0531d2c0a94c3e2",
    "oracle_repo": "https://github.com/airbus-cert/tree-sitter-powershell",
    "oracle_base_commit": "e7bd348c49fdfd5c853a146a670965ba516a6239",
    "oracle_head_commit": "b03e6b0e39692037a98e37a26f0a969b2c46fbc1",
    "oracle_subdir": "src", "oracle_runtime_commit": "10eb5ac119eec9cb4267a8e035066473983f5f3a",
    "oracle_binding_commit": "adc13ffd8b2c0b01b878fda9f7c422ce0df5fad3",
    "oracle_sources_sha256": "242c4f6b72d2d24421f84a505aa20164e43f144eb6401e876e6d24a167f06f51",
    "reviewed_patch_id": "3449af63add2da88dea7d58e0f41649bace9ee74",
}
POWERSHELL_W5_RECORD = dict(
    POWERSHELL_W5_POLICY,
    schema="w5-powershell-owner-decision-v1", exception="powershell-command-parameter-b03", owner_approved=True,
    decision_sha256=POWERSHELL_W5_DECISION_SHA256, failed_row_count=63, failed_rows_sha256=POWERSHELL_W5_ROWS_SHA256,
    base="d0838609c0d0f0c5f6d6fc892183e4f968ec77e7", base_tree="021617ec1d820f5fc9f98455d2efd6c70e377137",
    reviewed_source="8c217d53bbffeee34bf932a4d2f741efc5794bdc", reviewed_tree="0c5f50a28af863801afdb751b7439b8fa2ce4dac",
    original_evidence="https://github.com/odvcencio/gotreesitter/actions/runs/42095615414/job/127484861358",
    refreshed_evidence="https://github.com/odvcencio/gotreesitter/actions/runs/42186712785/job/127788798535",
)
POWERSHELL_W5_SUCCESSOR_PATHS = frozenset(("scripts/editor_latency_gate.py", "scripts/test_editor_latency_gate.py"))


def powershell_rowset_digest(rows):
    ordered = sorted(rows, key=lambda r: (r["language"], r["workload"], r["step"], r["counter"]))
    keys = [(r["language"], r["workload"], r["step"], r["counter"]) for r in ordered]
    require(len(keys) == len(set(keys)), "duplicate PowerShell failure row")
    return sha(json.dumps(ordered, sort_keys=True, separators=(",", ":")).encode())


def validate_powershell_source(root, base, head, record):
    require(record is not None and record.get("schema") == "w5-powershell-owner-decision-v1" and record.get("exception") == "powershell-command-parameter-b03" and record.get("owner_approved") is True and record.get("decision_sha256") == POWERSHELL_W5_DECISION_SHA256, "PowerShell owner decision missing")
    require(record.get("failed_row_count") == 63 and record.get("failed_rows_sha256") == POWERSHELL_W5_ROWS_SHA256, "PowerShell owner rowset changed")
    require(all(record.get(k) == v for k, v in POWERSHELL_W5_POLICY.items()), "PowerShell owner policy changed")
    require(all(isinstance(value, str) and re.fullmatch(r"[0-9a-f]{40}", value) for value in (base, head, record.get("reviewed_source"), record.get("base_tree"), record.get("reviewed_tree"))), "PowerShell source identity malformed")
    require(base == record["base"] and git(root, "rev-parse", "HEAD") == head, "PowerShell source pair mismatch")
    reviewed = record["reviewed_source"]
    require(git(root, "rev-parse", base + "^{tree}") == record["base_tree"] and git(root, "rev-parse", reviewed + "^{tree}") == record["reviewed_tree"], "PowerShell source tree mismatch")
    subprocess.run(["git", "-C", str(root), "merge-base", "--is-ancestor", reviewed, head], check=True)
    lineage = subprocess.check_output(["git", "-C", str(root), "rev-list", "--parents", reviewed + ".." + head]).decode().splitlines()
    expected = head
    for line in lineage:
        commit = line.split()
        require(len(commit) == 2 and commit[0] == expected, "PowerShell successor history is not linear")
        changed = subprocess.check_output(["git", "-C", str(root), "diff", "--no-renames", "--name-only", "-z", commit[1], commit[0]]).decode().split("\0")
        require((set(changed) - {""}) <= POWERSHELL_W5_SUCCESSOR_PATHS, "PowerShell successor changed production inputs")
        expected = commit[1]
    require(expected == reviewed, "PowerShell successor history is incomplete")
    delta = subprocess.check_output(["git", "-C", str(root), "diff", "--binary", "--no-ext-diff", base, reviewed])
    patch_id = subprocess.check_output(["git", "patch-id", "--stable"], input=delta).decode().split()
    require(len(patch_id) >= 1 and patch_id[0] == record["reviewed_patch_id"], "PowerShell reviewed patch changed")
    return record


def select_powershell_approval(root, base, head):
    if POWERSHELL_W5_RECORD is None or base != POWERSHELL_W5_RECORD.get("base"):
        return None
    try:
        record = validate_powershell_source(root, base, head, POWERSHELL_W5_RECORD)
        return {"record": record, "head": head}
    except (ValueError, KeyError, subprocess.CalledProcessError):
        return None

# Owner approval applies only to this source-bound counter row. Strict mode
# remains the default, and every opt-in requires the pinned decision receipt.
GO_1512_UNDO_EXCEPTION = "go-one-byte-undo-1512-20261029"
GO_1512_BASE = "9d14415d0a8d11632292310c578446b31554217b"
GO_1512_REVIEWED = "58ce5c00050815d361ec72723d68b87209b59ed4"
GO_1512_FIXTURE = "ff2f811d29a56303bad96c3c7cff1a83814a974503a51cee918a314a96eb476e"
GO_1512_TREE = "a675b5e8c8e688c21f45cad8c5217c06802c3fa5061fb2555ba346b4769f4c55"
GO_1512_BASE_ADMISSION = "4af75c271fee19a18a09caca22dba5fc3b0bb56142af480917f41f09ff69ef6a"
GO_1512_HEAD_ADMISSION = "4d7ae9309efb29d9a339d179f447aee788caee701d7d4c839ddbbcde53a863f0"
GO_1512_APPROVAL_SHA256 = "6c660fb0e91aeb3b515de27f263d12a30135ed1466b9f960f2778e6739a9e5b8"  # Owner-approved, single source-bound W5 row.
GO_1512_SUCCESSOR_PATHS = frozenset(("scripts/editor_latency_gate.py", "scripts/test_editor_latency_gate.py", ".github/workflows/ci.yml"))


# The owner approved only this exact base/source pair and row.
GO_1512_B7_OWNER_APPROVED = True
GO_1512_B7_APPROVAL_SHA256 = "ffb3bd9f774c899bbe1cabe07cb84cdfa61324e1b0fbc941d1938d34c840086b"
GO_1512_B7_DECISION = {'schema': 'gts-w5-owner-decision-v1', 'owner_approved': True, 'exception': 'go-one-byte-undo-b7-to-278', 'base': 'b7f1877f72c46ac2992e865ab636edad1d545060', 'reviewed_source': '2782879c97b4e28e9db7209ae7ddfbd6e9b69574', 'fixture_sha256': 'ff2f811d29a56303bad96c3c7cff1a83814a974503a51cee918a314a96eb476e', 'tree_sha256': 'a675b5e8c8e688c21f45cad8c5217c06802c3fa5061fb2555ba346b4769f4c55', 'base_admission_sha256': '4af75c271fee19a18a09caca22dba5fc3b0bb56142af480917f41f09ff69ef6a', 'head_admission_sha256': '4d7ae9309efb29d9a339d179f447aee788caee701d7d4c839ddbbcde53a863f0', 'oracle_runtime_commit': '10eb5ac119eec9cb4267a8e035066473983f5f3a', 'oracle_sources_sha256': '242c4f6b72d2d24421f84a505aa20164e43f144eb6401e876e6d24a167f06f51', 'oracle_go_grammar_commit': '2346a3ab1bb3857b48b29d779a1ef9799a248cd7', 'row': {'language': 'go', 'workload': 'one_byte', 'step': 1, 'counter': 'reused_subtrees', 'before': 39, 'after': 35}, 'context': {'reused_bytes_before': 2816, 'reused_bytes_after': 2853, 'nodes_before': 109036, 'nodes_after': 108999}, 'scope': 'This single counter row only; no timing, correctness, RSS, graduation or merge waiver.', 'owner_response_provenance': {'channel': 'direct human owner reply', 'question_item_id': ['request_user_input_async', 'call_3694a733fae64952bd5b34f85deeb359', 0], 'response': 'Approve this scoped exception and minimal CI record'}, 'review_evidence': 'https://github.com/odvcencio/gotreesitter/actions/runs/40949464539/job/123713570661'}
GO_1512_B7_BASE = "b7f1877f72c46ac2992e865ab636edad1d545060"
GO_1512_B7_REVIEWED = "2782879c97b4e28e9db7209ae7ddfbd6e9b69574"
GO_1512_B7_UNDO_EXCEPTION = "go-one-byte-undo-b7-to-278"
GO_1512_B7_SUCCESSOR_PATHS = frozenset(("scripts/editor_latency_gate.py", "scripts/test_editor_latency_gate.py", ".github/workflows/ci.yml"))


def validate_b7_undo_exception(root, base_revision, head_revision, decision_path):
    require(GO_1512_B7_OWNER_APPROVED, "W5 b7 exception has no owner approval")
    require(base_revision == GO_1512_B7_BASE, "W5 b7 exception base mismatch")
    require(re.fullmatch(r"[0-9a-f]{40}", head_revision) is not None, "W5 b7 exception head must be a commit")
    require(git(root, "rev-parse", "HEAD") == head_revision, "W5 b7 exception checkout mismatch")
    require(not subprocess.check_output(["git", "-C", str(root), "status", "--porcelain", "--untracked-files=all", "--ignored"]), "W5 b7 exception requires a clean checkout")
    subprocess.run(["git", "-C", str(root), "merge-base", "--is-ancestor", GO_1512_B7_REVIEWED, head_revision], check=True)
    diff = subprocess.check_output(["git", "-C", str(root), "diff", "--no-renames", "--name-only", "-z", GO_1512_B7_REVIEWED, head_revision])
    require((set(diff.decode().split("\0")) - {""}) <= GO_1512_B7_SUCCESSOR_PATHS, "W5 b7 exception production inputs changed")
    decision_bytes = Path(decision_path).read_bytes()
    require(sha(decision_bytes) == GO_1512_B7_APPROVAL_SHA256, "W5 b7 owner decision digest mismatch")
    require(read_json(decision_path) == GO_1512_B7_DECISION, "W5 b7 owner decision content mismatch")
    return GO_1512_B7_UNDO_EXCEPTION


def canonical_go_admission(admission):
    # Revision and ephemeral cache filename do not change the parsed content.
    # Compiler-specific fields are compared across the paired run; source,
    # tree, counter, oracle version, grammar pin, and flags remain bound.
    data = json.loads(json.dumps(admission))
    data.pop("revision", None)
    for key in ("grammar_artifact_path", "grammar_artifact_sha256", "compiler_path", "compiler_version", "grammar_subdir", "grammar_patch_identity"):
        data["oracle"].pop(key, None)
    return sha(json.dumps(data, sort_keys=True, separators=(",", ":"), ensure_ascii=True).encode())


def validate_1512_undo_exception(root, base_revision, head_revision, decision_path):
    require(GO_1512_APPROVAL_SHA256 != "", "W5 exception has no approved receipt")
    require(base_revision == GO_1512_BASE, "W5 exception base mismatch")
    require(re.fullmatch(r"[0-9a-f]{40}", head_revision) is not None, "W5 exception head must be a commit")
    require(git(root, "rev-parse", "HEAD") == head_revision, "W5 exception checkout mismatch")
    require(not subprocess.check_output(["git", "-C", str(root), "status", "--porcelain", "--untracked-files=all", "--ignored"]), "W5 exception requires a clean checkout, including ignored files")
    subprocess.run(["git", "-C", str(root), "merge-base", "--is-ancestor", GO_1512_REVIEWED, head_revision], check=True)
    diff = subprocess.check_output(["git", "-C", str(root), "diff", "--no-renames", "--name-only", "-z", GO_1512_REVIEWED, head_revision])
    changed = set(diff.decode().split("\0")) - {""}
    require(changed <= GO_1512_SUCCESSOR_PATHS, "W5 exception production inputs changed")
    decision_bytes = Path(decision_path).read_bytes()
    require(sha(decision_bytes) == GO_1512_APPROVAL_SHA256, "W5 approval receipt hash mismatch")
    decision = read_json(decision_path)
    require(decision.get("owner_approved") is True and decision.get("base") == GO_1512_BASE and decision.get("reviewed_source") == GO_1512_REVIEWED and decision.get("exception") == GO_1512_UNDO_EXCEPTION, "W5 owner decision mismatch")
    return GO_1512_UNDO_EXCEPTION


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
    require(oracle["runtime_version"] == "0.27.1" and oracle["runtime_commit"] == "10eb5ac119eec9cb4267a8e035066473983f5f3a", "C runtime is not locked")
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


def verify_fresh_proof(proof, oracle):
    require(isinstance(proof, dict), "changed grammar lacks fresh committed-parser proof")
    require(proof.get("schema") == "w5-fresh-source-v1" and proof.get("mode") == "committed-parser-no-patch-no-regeneration", "unqualified fresh source proof")
    for field, oracle_field in (("language", "language"), ("repo", "grammar_repo"), ("commit", "grammar_commit"), ("subdir", "grammar_subdir"), ("artifact_sha256", "grammar_artifact_sha256")):
        require(proof.get(field) == oracle.get(oracle_field), "fresh source proof does not match oracle")
    require(oracle.get("grammar_patch_identity") == "none", "patched grammar cannot use committed-parser proof")
    subdir = proof["subdir"]
    require(isinstance(subdir, str) and subdir and not Path(subdir).is_absolute() and ".." not in Path(subdir).parts, "invalid proof subdir")
    parser = str(Path(subdir) / "parser.c")
    require(proof.get("expected_parser") == parser and proof.get("actual_parser") == parser, "fresh source parser path mismatch")
    require(type(proof.get("abi")) is int and 13 <= proof["abi"] <= 15, "fresh source parser ABI is unknown or incompatible")
    sources = proof.get("sources")
    allowed = {str(Path(subdir) / name) for name in ("parser.c", "scanner.c", "scanner.cc", "scanner.cpp", "scanner.cxx")}
    require(isinstance(sources, dict) and parser in sources and set(sources) <= allowed and all(isinstance(v, str) and re.fullmatch(r"[0-9a-f]{64}", v) for v in sources.values()), "invalid fresh compiler sources")
    require(proof.get("parser_sha256") == sources[parser], "fresh parser digest mismatch")


def validate_powershell_rows(record, rows, manifest_path, out, authenticated):
    require(record is not None and authenticated is not None, "PowerShell exception requires authenticated private oracles")
    require(record.get("schema") == "w5-powershell-owner-decision-v1" and record.get("exception") == "powershell-command-parameter-b03" and record.get("owner_approved") is True and record.get("decision_sha256") == POWERSHELL_W5_DECISION_SHA256, "PowerShell owner decision missing")
    require(all(record.get(k) == v for k, v in POWERSHELL_W5_POLICY.items()), "PowerShell owner policy changed")
    require(record.get("failed_row_count") == 63 and record.get("failed_rows_sha256") == POWERSHELL_W5_ROWS_SHA256, "PowerShell owner rowset changed")
    require(sha(Path(manifest_path).read_bytes()) == record["fixture_manifest_sha256"] and digest(out / "fixtures/powershell") == record["fixture_sha256"], "PowerShell fixture identity changed")
    require(len(rows) == 63 and all(r.get("language") == "powershell" for r in rows) and powershell_rowset_digest(rows) == POWERSHELL_W5_ROWS_SHA256, "PowerShell failure rowset changed")
    receipts = authenticated["powershell"]
    for role in ("base", "head"):
        expected = receipts[role][2]
        oracle = expected["oracle"]
        wanted = {"language": "powershell", "grammar_repo": record["oracle_repo"], "grammar_commit": record["oracle_" + role + "_commit"],
                  "grammar_subdir": record["oracle_subdir"], "grammar_patch_identity": "none", "runtime_commit": record["oracle_runtime_commit"],
                  "binding_commit": record["oracle_binding_commit"], "sources_manifest_sha256": record["oracle_sources_sha256"]}
        require(all(oracle.get(k) == v for k, v in wanted.items()), "PowerShell role oracle differs from owner decision")
        verify_fresh_proof(expected.get("fresh_proof"), oracle)
    return {"name": "powershell-command-parameter-b03", "row_count": 63, "rows_sha256": POWERSHELL_W5_ROWS_SHA256}


def compare_counters(out, manifest_path, base_revision, head_revision, exception=None, authenticated=None, powershell_approval=None):
    m = fixture_manifest(manifest_path)
    manifest_sha = sha(Path(manifest_path).read_bytes())
    changes = []
    identities = []
    failures = []
    applied = []
    semantic_changes = []
    for f in m["fixtures"]:
        base = admission(out, "base", f, manifest_sha, base_revision)
        head = admission(out, "head", f, manifest_sha, head_revision)
        changed_grammar = False
        if authenticated is not None:
            receipts = authenticated[f["language"]]
            require(receipts["base"][2]["compilers"] == receipts["head"][2]["compilers"] and receipts["base"][2]["build_env"] == receipts["head"][2]["build_env"] and receipts["base"][2]["image_id"] == receipts["head"][2]["image_id"], f"{f['language']}: C compiler, environment or image differs")
            for role, actual in (("base", base), ("head", head)):
                expected_oracle = dict(receipts[role][2]["oracle"])
                expected_oracle["compiler_path"] = Path(expected_oracle["compiler_path"]).name
                require(oracle_identity(actual["oracle"]) == oracle_identity(expected_oracle), f"{f['language']}: {role} oracle differs from authenticated build")
            grammar_keys = ("grammar_repo", "grammar_commit", "grammar_subdir", "grammar_patch_identity")
            changed_grammar = any(base["oracle"][key] != head["oracle"][key] for key in grammar_keys)
            if changed_grammar:
                ignored = set(grammar_keys) | {"grammar_artifact_path", "grammar_artifact_sha256"}
                require({k:v for k,v in base["oracle"].items() if k not in ignored} == {k:v for k,v in head["oracle"].items() if k not in ignored}, f"{f['language']}: C runtime/compiler differs between revisions")
                for role in ("base", "head"):
                    verify_fresh_proof(receipts[role][2]["fresh_proof"], receipts[role][2]["oracle"])
        if not changed_grammar:
            require(oracle_identity(base["oracle"]) == oracle_identity(head["oracle"]), f"{f['language']}: C oracle differs between revisions")
        if exception is not None and f["language"] == "go":
            require(exception in (GO_1512_UNDO_EXCEPTION, GO_1512_B7_UNDO_EXCEPTION), "unknown W5 exception")
            require(canonical_go_admission(base) == GO_1512_BASE_ADMISSION and canonical_go_admission(head) == GO_1512_HEAD_ADMISSION, "W5 approved source/tree/oracle/counter receipt changed")
        if base["initial_tree_sha256"] != head["initial_tree_sha256"]:
            require(changed_grammar, "initial tree digest differs")
            semantic_changes.append({"language": f["language"], "step": "initial", "base": base["initial_tree_sha256"], "head": head["initial_tree_sha256"]})
        identities.append({"language": f["language"], "base_oracle": base["oracle"], "oracle": head["oracle"]})
        for bc, hc in zip(base["cells"], head["cells"]):
            for index, (bs, hs) in enumerate(zip(bc["steps"], hc["steps"])):
                require(bs["source_sha256"] == hs["source_sha256"], "edit source differs between revisions")
                if (bs["tree_sha256"], bs["has_error"]) != (hs["tree_sha256"], hs["has_error"]):
                    require(changed_grammar, "edit source/tree digest differs between revisions")
                    semantic_changes.append({"language": f["language"], "workload": bc["workload"], "step": index, "base": [bs["tree_sha256"], bs["has_error"]], "head": [hs["tree_sha256"], hs["has_error"]]})
                for key in WORK_COUNTERS + REUSE_COUNTERS:
                    before, after = bs["counters"][key], hs["counters"][key]
                    bad = after > before * LEDGER_LIMIT
                    # The reuse ledger tolerates a fall of 2% of the base value.
                    if key in REUSE_COUNTERS:
                        bad = after < before * (2 - LEDGER_LIMIT)
                    row = {"language": f["language"], "workload": bc["workload"], "step": index, "counter": key, "before": before, "after": after}
                    changes.append(row)
                    if bad:
                        if exception in (GO_1512_UNDO_EXCEPTION, GO_1512_B7_UNDO_EXCEPTION) and f["sha256"] == GO_1512_FIXTURE and (bs["source_sha256"], bs["tree_sha256"], bs["has_error"]) == (GO_1512_FIXTURE, GO_1512_TREE, False) and row == {"language":"go", "workload":"one_byte", "step":1, "counter":"reused_subtrees", "before":39, "after":35}:
                            applied.append({"name":exception, "row":row})
                        else:
                            failures.append(row)
    require(exception is None or len(applied) == 1, "W5 approved row did not match exactly once")
    if powershell_approval is not None and exception is None:
        try:
            record = powershell_approval["record"]
            require(record["base"] == base_revision and powershell_approval["head"] == head_revision, "PowerShell source pair mismatch")
            approved = validate_powershell_rows(record, failures, manifest_path, out, authenticated)
        except (ValueError, KeyError, TypeError, OSError):
            approved = None
        if approved is not None:
            applied.append(approved)
            failures = []
    result = {"schema": "w5-edit-counters-v1", "passed": not failures, "rows": changes, "failures": failures, "applied_exceptions":applied, "oracles": identities, "semantic_changes": semantic_changes}
    write_json(out / "counters.json", result)
    if failures:
        print(f"W5 counter failure rows: {len(failures)}", flush=True)
        for row in failures:
            print("W5 counter failure: " + json.dumps(row, sort_keys=True, separators=(",", ":")), flush=True)
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


def compare_timing(out, manifest_path, base_revision, head_revision, exception=None, authenticated=None, powershell_approval=None):
    counters = compare_counters(out, manifest_path, base_revision, head_revision, exception, authenticated, powershell_approval)
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
              "environment": environment, "cells": rows, "counter_rows": counters["rows"], "applied_exceptions": counters["applied_exceptions"], "oracles": counters["oracles"], "semantic_changes": counters["semantic_changes"]}
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
    command = ["bash", root / "cgo_harness/docker/run_parity_in_docker.sh",
               "--out-root", out / "harness"]
    if os.environ.get("GTS_EDITOR_LATENCY_IMAGE"):
        command += ["--image", os.environ["GTS_EDITOR_LATENCY_IMAGE"]]
    return command


def campaign_docker_command(root, base, out, cpu):
    command = campaign_harness_command(root, out) + ["--no-build",
            "--memory", os.environ.get("GTS_EDITOR_LATENCY_MEMORY_LIMIT") or "8g",
            "--gomemlimit", os.environ.get("GOMEMLIMIT") or "6GiB",
            "--cpuset-cpus", str(cpu), "--cpus", "1",
            "--mount", f"{base}:/baseline:ro", "--mount", f"{out}:/campaign"]
    for variable, target in (("GOCACHE", "/lane-go-cache"), ("GOTMPDIR", "/lane-go-tmp"), ("TMPDIR", "/lane-tmp")):
        if os.environ.get(variable):
            command += ["--mount", f"{os.environ[variable]}:{target}"]
    if os.environ.get("GOCACHE"):
        command += ["--no-chown"]
    return command


def campaign_go_environment():
    return "; ".join(f"export {variable}={target}" for variable, target in
                     (("GOCACHE", "/lane-go-cache"), ("GOTMPDIR", "/lane-go-tmp"), ("TMPDIR", "/lane-tmp"))
                     if os.environ.get(variable)) or "true"


def check_work_exemptions_shrink(before, after):
    def keyed(entries):
        rows = {(e["fixture_sha256"], e["cell"], e["class"]): e for e in entries}
        require(len(rows) == len(entries), "duplicate W5 work exemption")
        return rows
    old, new = keyed(before), keyed(after)
    require(all(key in old and old[key] == entry for key, entry in new.items()),
            "W5 work exemptions may only shrink; additions and changed receipts are forbidden")


def check_work_exemptions_history(root, base_revision):
    path = "internal/benchfixtures/w5_work_exemptions.json"
    previous = subprocess.run(["git", "-C", root, "show", f"{base_revision}:{path}"], capture_output=True, text=True)
    if previous.returncode == 0:
        check_work_exemptions_shrink(json.loads(previous.stdout), read_json(root / path))


# Private cache is an optional optimization. Strict source and counter checks
# remain unchanged; all compiler/source/artifact checks fail closed.
def clean_private_inputs(root):
    status = subprocess.check_output(["git", "-C", str(root), "status", "--porcelain", "--untracked-files=all", "--ignored"])
    require(not status, "private oracle cache requires clean tracked, untracked and ignored source inputs")


def oracle_inputs(root):
    paths = subprocess.check_output(["git", "-C", str(root), "ls-files", "-z", "--", "cgo_harness/internal/coracle", "cgo_harness/parity_c_loader_cgo.go", "cgo_harness/editor_latency/editor_latency_test.go", "grammars/languages.lock", "internal/grammarpatch", "grammars/patches"]).decode().split("\0")
    require(any(paths), "missing tracked oracle inputs")
    return [root / p for p in paths if p]


def campaign_role_source(role, revision, checkout, expected=None):
    require(role in ("base", "head") and re.fullmatch(r"[0-9a-f]{40}", revision), "invalid campaign role/revision")
    lock = sha((checkout / "grammars/languages.lock").read_bytes())
    container_root = "/baseline" if role == "base" else "/workspace"
    values = dict(Role=role, Root=container_root, Revision=revision, LockSHA256=lock)
    if expected:
        values.update(IdentitySHA256=expected["identity"], CCPath=expected["compilers"][0][0], CCSHA256=expected["compilers"][0][1],
                      CXXPath=expected["compilers"][1][0], CXXSHA256=expected["compilers"][1][1], BuildEnvSHA256=expected["build_env"])
    fields = ", ".join(k + ": " + json.dumps(v) for k, v in values.items())
    source = (
        "// Code generated by the campaign; DO NOT EDIT.\n"
        "package editorlatency\n"
        "func init() { compiledCampaignRole = &campaignRole{" + fields + "} }\n")
    return lock, source


def write_campaign_role(module, role, revision, checkout, expected=None):
    """Compile the role and its complete grammar lock into the shared driver."""
    lock, source = campaign_role_source(role, revision, checkout, expected)
    (module / "campaign_role_test.go").write_text(source)
    return lock


def bind_private_role(module, role, revision, checkout, inputs, expected):
    """Permit only the intended config rewrite after the cold-build snapshot."""
    config = module / "campaign_role_test.go"
    key = str(config)
    _, old_source = campaign_role_source(role, revision, checkout)
    require(expected["sources"].get(key) == sha(old_source.encode()), "cold role config does not match expected pre-build configuration")
    require(source_fingerprints(inputs) == expected["sources"], "oracle source inputs changed before role binding")
    _, new_source = campaign_role_source(role, revision, checkout, expected)
    config.write_text(new_source)
    updated = dict(expected["sources"])
    updated[key] = sha(new_source.encode())
    require(source_fingerprints(inputs) == updated, "oracle source inputs changed during role binding")
    expected["sources"] = updated


def lock_grammar(root, language):
    rows = [line.split() for line in (root / "grammars/languages.lock").read_text().splitlines()
            if line.strip() and not line.lstrip().startswith("#") and line.split()[0] == language]
    require(len(rows) == 1 and len(rows[0]) >= 3 and re.fullmatch(r"[0-9a-f]{40}", rows[0][2]), "invalid role grammar lock entry")
    row = rows[0]
    return {"grammar_repo": row[1], "grammar_commit": row[2], "grammar_subdir": row[3] if len(row) > 3 else "src"}


def verify_locked_oracle(root, language, cold):
    require(all(cold.get(k) == v for k, v in lock_grammar(root, language).items()), "cold oracle differs from role grammar lock")


def prepare_private_oracle(root, common, directory, language, inputs, image, image_id, role):
    check_env(os.environ)
    require(role in ("base", "head"), "invalid oracle role")
    directory.mkdir(parents=True); (directory / "cache").mkdir()
    private = "/private/" + role
    require(not list((directory / "cache").iterdir()), "private cache must start empty")
    before = source_fingerprints(inputs)
    command = "\n".join(["set -euo pipefail", campaign_go_environment(),
        "unset GTS_PARITY_REPO_ROOT GTS_PARITY_REPO_CACHE GTS_PARITY_EXTRA_LOCK GTS_W5_PRIVATE_ORACLE_IDENTITY GTS_W5_PRIVATE_ORACLE_SHA256 GTS_W5_CC_PATH GTS_W5_CC_SHA256 GTS_W5_CXX_PATH GTS_W5_CXX_SHA256",
        f"unset GTS_EDIT_REPO_ROOT GTS_EDIT_REVISION; export GOWORK=off GOMAXPROCS=1 GTS_PARITY_C_REF_BUILD_CACHE={private}/cache GTS_W5_PRIVATE_ORACLE_PREPARE={private}/cold.json",
        f"export GTS_EDIT_LANGUAGE={shlex.quote(language)}", f"cd /campaign/modules/{role}",
        'compiler_receipt() { for name in cc c++; do path=$(command -v "$name"); printf \'%s %s\\n\' "$path" "$(sha256sum "$path" | cut -d\' \' -f1)"; done; }',
        f"compiler_receipt > {private}/compiler.before",
        f"read -r GTS_W5_CC_PATH GTS_W5_CC_SHA256 < <(sed -n '1p' {private}/compiler.before); read -r GTS_W5_CXX_PATH GTS_W5_CXX_SHA256 < <(sed -n '2p' {private}/compiler.before); export GTS_W5_CC_PATH GTS_W5_CC_SHA256 GTS_W5_CXX_PATH GTS_W5_CXX_SHA256",
        "go test -tags treesitter_c_parity . -run '^$' -count=1 -timeout 15m",
        f"compiler_receipt > {private}/compiler.after; cmp {private}/compiler.before {private}/compiler.after; cmp {private}/environment.before {private}/environment.after"])
    run_checked(common + ["--mount", f"{directory}:{private}", "--label", "w5-oracle-" + role + "-" + language, "--wall-timeout", "20m", "--", command])
    require(source_fingerprints(inputs) == before, "oracle inputs changed during fresh build")
    require(subprocess.check_output(["docker", "image", "inspect", "--format", "{{.Id}}", image], text=True).strip() == image_id, "oracle Docker image changed")
    cold = read_json(directory / "cold.json"); identity_fields(cold)
    require(cold["language"] == language and cold["contract"] == "tree-sitter-c-v1", "cold identity mismatch")
    verify_locked_oracle(root, language, cold)
    artifacts = list((directory / "cache").iterdir())
    require(len(artifacts) == 1 and digest(artifacts[0]) == cold["grammar_artifact_sha256"], "cold artifact mismatch")
    proof = read_json(directory / "cold-source.json")
    if proof is not None: verify_fresh_proof(proof, cold)
    compiler_bytes = (directory / "compiler.before").read_bytes()
    require(compiler_bytes == (directory / "compiler.after").read_bytes(), "compiler changed during build")
    compilers = [line.split() for line in compiler_bytes.decode().splitlines()]
    require(len(compilers) == 2 and all(len(row) == 2 and Path(row[0]).is_absolute() and re.fullmatch(r"[0-9a-f]{64}", row[1]) for row in compilers), "compiler identity missing")
    require(compilers[0][0] == cold["compiler_path"], "cold C compiler path mismatch")
    build_env = (directory / "environment.before").read_text().strip()
    require(re.fullmatch(r"[0-9a-f]{64}", build_env) and (directory / "environment.after").read_text().strip() == build_env, "compiler environment changed during build")
    cold["grammar_artifact_path"] = private + "/cache/" + artifacts[0].name
    write_json(directory / "identity.json", cold)
    expected = {"sources":before, "artifact":digest(artifacts[0]), "identity":digest(directory / "identity.json"), "fresh_proof_sha256":digest(directory / "cold-source.json"), "fresh_proof":proof, "compilers":compilers, "build_env":build_env, "filename":artifacts[0].name, "image":image, "image_id":image_id, "oracle":cold}
    verify_private_oracle(directory, inputs, expected)
    return expected


def verify_private_oracle(directory, inputs, expected):
    check_env(os.environ)
    require(subprocess.check_output(["docker", "image", "inspect", "--format", "{{.Id}}", expected["image"]], text=True).strip() == expected["image_id"], "oracle Docker image changed")
    require(source_fingerprints(inputs) == expected["sources"], "oracle source inputs changed")
    artifacts = list((directory / "cache").iterdir())
    require(len(artifacts) == 1 and artifacts[0].name == expected["filename"] and digest(artifacts[0]) == expected["artifact"], "private oracle cache changed")
    require(digest(directory / "identity.json") == expected["identity"], "private oracle identity changed")
    require(digest(directory / "cold-source.json") == expected["fresh_proof_sha256"], "private oracle source proof changed")
    compiler_lines = [line.split() for line in (directory / "compiler.before").read_text().splitlines()]
    require(compiler_lines == expected["compilers"] and (directory / "compiler.before").read_bytes() == (directory / "compiler.after").read_bytes(), "compiler receipt changed")
    require((directory / "environment.before").read_text().strip() == expected["build_env"] and (directory / "environment.after").read_text().strip() == expected["build_env"], "compiler environment receipt changed")


def campaign(root, base_revision, out, approved_receipt=None, approved_b7_receipt=None):
    root = root.resolve(); out = out.resolve()
    private_setting = os.environ.get("GTS_W5_PRIVATE_C_CACHE", "")
    require(private_setting in ("", "1"), "invalid private C cache setting")
    if private_setting:
        check_env(os.environ)
        clean_private_inputs(root)
    head_revision = git(root, "rev-parse", "HEAD")
    base_revision = git(root, "rev-parse", base_revision + "^{commit}")
    clean_revision(root, head_revision)
    require(not (approved_receipt and approved_b7_receipt), "multiple W5 exceptions are forbidden")
    exception = validate_1512_undo_exception(root, base_revision, head_revision, approved_receipt) if approved_receipt else None
    if approved_b7_receipt: exception = validate_b7_undo_exception(root, base_revision, head_revision, approved_b7_receipt)
    powershell_approval = select_powershell_approval(root, base_revision, head_revision) if private_setting else None
    check_work_exemptions_history(root, base_revision)
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
replace github.com/tree-sitter/go-tree-sitter => /workspace/cgo_harness/internal/coracle
""")
    (out / "timing").mkdir()
    (out / "tools").mkdir()
    tool_env = dict(os.environ, GOWORK="off", GOMAXPROCS="1", GOBIN=str(out / "tools"))
    run_checked(["go", "install", BENCHSTAT_MODULE], env=tool_env)
    env["benchstat_module"] = BENCHSTAT_MODULE
    env["benchstat_sha256"] = sha((out / "tools/benchstat").read_bytes())
    env["image"] = os.environ.get("GTS_EDITOR_LATENCY_IMAGE") or "gotreesitter/cgo-harness:go1.25-local"
    run_checked(campaign_harness_command(root, out) + ["--build-only"], stdout=subprocess.DEVNULL)
    env["image_id"] = subprocess.check_output(["docker", "image", "inspect", "--format", "{{.Id}}", env["image"]], text=True).strip()
    env["go_version"] = subprocess.check_output(["docker", "run", "--rm", env["image"], "go", "version"], text=True).strip()
    write_json(out / "environment.json", env)
    with tempfile.TemporaryDirectory(prefix="gts-w5-base-") as scratch, tempfile.TemporaryDirectory(prefix="gts-w5-oracle-") as private_scratch:
        base = Path(scratch) / "source"
        run_checked(["git", "-C", root, "worktree", "add", "--detach", base, base_revision])
        try:
            if private_setting: clean_private_inputs(base)
            env["role_lock_sha256"] = {
                role: write_campaign_role(out / "modules" / role, role, revision, checkout)
                for role, revision, checkout in (("base", base_revision, base), ("head", head_revision, root))
            }
            common = campaign_docker_command(root, base, out, cpu)
            env["docker_memory_limit"] = common[common.index("--memory") + 1]
            env["go_memory_limit"] = common[common.index("--gomemlimit") + 1]
            write_json(out / "environment.json", env)
            private_receipts = {}
            if private_setting:
                setup = "\n".join(["set -euo pipefail", campaign_go_environment(), "export GOWORK=off GOMAXPROCS=1", "cd /campaign/modules/base; go mod tidy", "cd /campaign/modules/head; go mod tidy"])
                run_checked(common + ["--label", "w5-oracle-setup", "--wall-timeout", "20m", "--", setup])
            for f in m["fixtures"]:
                language = f["language"]
                roles = (("base", base_revision, base), ("head", head_revision, root))
                receipts = {}
                if private_setting:
                    for role, revision, checkout in roles:
                        module = out / "modules" / role
                        write_campaign_role(module, role, revision, checkout)
                        private_dir = Path(private_scratch) / role / language
                        private_inputs = oracle_inputs(root) + oracle_inputs(base) + [module / file for file in ("editor_latency_test.go", "campaign_role_test.go", "go.mod", "go.sum")]
                        expected = prepare_private_oracle(checkout, common, private_dir, language, private_inputs, env["image"], env["image_id"], role)
                        bind_private_role(module, role, revision, checkout, private_inputs, expected)
                        verify_private_oracle(private_dir, private_inputs, expected)
                        receipts[role] = (private_dir, private_inputs, expected)
                    private_receipts[language] = receipts
                    env.setdefault("private_oracles", {})[language] = {role: receipt[2] for role, receipt in receipts.items()}
                    write_json(out / "environment.json", env)
                worker_common = common + [item for role, (directory, _, _) in receipts.items() for item in ("--mount", f"{directory}:/private/{role}:ro")]
                commands = ["set -euo pipefail", campaign_go_environment(), "unset GTS_EDIT_REPO_ROOT GTS_EDIT_REVISION; export GOWORK=off GOMAXPROCS=1 GTS_EDIT_MANIFEST=/workspace/cgo_harness/editor_latency/fixtures.json GTS_EDIT_FIXTURES=/campaign/fixtures", f"export GTS_EDIT_LANGUAGE={language}"]
                if exception and not receipts:
                    # Fresh pinned clone/build: do not trust a shared .so or a
                    # caller-provided source directory/extra grammar lock.
                    commands.append("unset GTS_PARITY_REPO_ROOT GTS_PARITY_REPO_CACHE GTS_PARITY_EXTRA_LOCK; export GTS_PARITY_C_REF_BUILD_CACHE=off")
                for role, revision in (("base", base_revision), ("head", head_revision)):
                    commands += [f"cd /campaign/modules/{role}", "go mod tidy", f"GTS_EDIT_REVISION={revision} GTS_EDIT_ADMISSION_OUT=/campaign/admission/{role}/{language}.json go test -tags treesitter_c_parity . -run '^TestW5RealCodeEdits$' -count=1 -v -timeout 15m"]
                with (out / ("correctness-" + language + ".log")).open("w") as log:
                    run_checked(worker_common + ["--label", "w5-correctness-" + language, "--wall-timeout", "35m", "--", "\n".join(commands)], stdout=log, stderr=subprocess.STDOUT)
                for directory, inputs, expected in receipts.values(): verify_private_oracle(directory, inputs, expected)
                print("W5 four-way correctness passed:", language, flush=True)
            compare_counters(out, manifest_path, base_revision, head_revision, exception, private_receipts if private_setting else None, powershell_approval)
            print("W5 deterministic counter gate passed before timing", flush=True)
            for f in m["fixtures"]:
                language = f["language"]
                if private_setting:
                    # Reload only identity from an in-memory parent receipt; never trust a worker-written manifest.
                    receipts = private_receipts[language]
                    for role, revision, checkout in (("base", base_revision, base), ("head", head_revision, root)):
                        directory, inputs, expected = receipts[role]
                        write_campaign_role(out / "modules" / role, role, revision, checkout, expected)
                        verify_private_oracle(directory, inputs, expected)
                else: receipts = {}
                worker_common = common + [item for role, (directory, _, _) in receipts.items() for item in ("--mount", f"{directory}:/private/{role}:ro")]
                env["samples"].append({"language": language, "before": load_sample()})
                fragments = {role: [] for role in ("base", "head")}
                rss_samples = []
                for seed in range(1, RUNS + 1):
                    prefix = f"{language}-seed-{seed}"
                    for role in fragments:
                        fragments[role].append(out / "timing" / f"{prefix}-{role}.txt")
                    command = "\n".join(["set -euo pipefail", campaign_go_environment(), "unset GTS_EDIT_REPO_ROOT GTS_EDIT_REVISION; export GOWORK=off GOMAXPROCS=1 GTS_EDIT_MANIFEST=/workspace/cgo_harness/editor_latency/fixtures.json GTS_EDIT_FIXTURES=/campaign/fixtures", f"export GTS_EDIT_LANGUAGE={language}", "cd /campaign/modules/head", "/usr/bin/time -v bash /workspace/scripts/run_randomized_benchmarks.sh " + " ".join(shlex.quote(x) for x in ["--runs", "1", "--seed-start", str(seed), "--baseline-root", "/campaign/modules/base", "--baseline-output", f"/campaign/timing/{prefix}-base.txt", "--output", f"/campaign/timing/{prefix}-head.txt", "--tags", "treesitter_c_parity", "--bench-regex", "^BenchmarkW5(OneByte|HundredByte|Typing)$", "--require-benchmarks", ",".join(NAMES)])])
                    if exception and not receipts:
                        command = "unset GTS_PARITY_REPO_ROOT GTS_PARITY_REPO_CACHE GTS_PARITY_EXTRA_LOCK; export GTS_PARITY_C_REF_BUILD_CACHE=off\n" + command
                    log_path = out / f"timing-{prefix}.log"
                    with log_path.open("w") as log:
                        run_checked(worker_common + ["--label", "w5-timing-" + prefix, "--wall-timeout", "90m", "--", command], stdout=log, stderr=subprocess.STDOUT)
                    for directory, inputs, expected in receipts.values(): verify_private_oracle(directory, inputs, expected)
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
            if private_setting: clean_private_inputs(base)
        finally:
            run_checked(["git", "-C", root, "worktree", "remove", "--force", base])
    clean_revision(root, head_revision)
    if private_setting: clean_private_inputs(root)
    if approved_receipt:
        require(validate_1512_undo_exception(root, base_revision, head_revision, approved_receipt) == exception, "W5 exception identity changed")
    if approved_b7_receipt:
        require(validate_b7_undo_exception(root, base_revision, head_revision, approved_b7_receipt) == exception, "W5 b7 exception identity changed")
    if powershell_approval is not None:
        powershell_approval = select_powershell_approval(root, base_revision, head_revision)
    env["finished"] = load_sample()
    write_json(out / "environment.json", env)
    result = compare_timing(out, manifest_path, base_revision, head_revision, exception, private_receipts if private_setting else None, powershell_approval)
    require(result["status"] == "passed", "one or more edit cells regressed by more than 5%")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    run = sub.add_parser("run")
    run.add_argument("--base", required=True)
    run.add_argument("--output", type=Path, required=True)
    run.add_argument("--approved-go-undo-receipt", type=Path)
    run.add_argument("--approved-b7-undo-receipt", type=Path)
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
                campaign(root, args.base, args.output, args.approved_go_undo_receipt, args.approved_b7_undo_receipt)
        else:
            result = compare_timing(args.output, args.manifest, args.base, args.head)
            require(result["status"] == "passed", "one or more edit cells regressed by more than 5%")
    except (ValueError, OSError, KeyError, subprocess.CalledProcessError) as error:
        print(f"W5 gate failed: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
