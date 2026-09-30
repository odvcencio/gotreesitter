#!/usr/bin/env python3
"""Seal one complete qualification run; reject mixed or unauthenticated runs.

The archive contains receipts, never the corpus lock or corpus source bytes.
Its SHA-256 filename and the manifest's per-receipt hashes bind the evidence.
All repository pins are read at the measured Git revision, not from HEAD.
"""

import argparse
import gzip
import hashlib
import io
import json
import re
import subprocess
import tarfile
from collections import Counter
from pathlib import Path


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def git_bytes(revision, path):
    return subprocess.check_output(["git", "show", f"{revision}:{path}"])


def lock_entries(data):
    return {parts[0]: parts[1:3] for line in data.decode().splitlines()
            if (parts := line.split()) and not line.startswith("#")}


def validate_receipt(receipt, name, revision, runtime, sources_sha, blob_sha, grammar_pin, lock_sha):
    def require(condition, message):
        if not condition:
            raise ValueError(f"{name}: {message}")

    require(receipt["schema"] == "gts-grammar-receipt/v1", "unknown receipt schema")
    require(receipt["grammar"] == {"name": name, "blob_sha256": blob_sha}, "grammar artifact differs from measured revision")
    require(receipt["gotreesitter_commit"] == revision and receipt["git_tree_dirty"] is False, "mixed revision or dirty worktree")
    oracle = receipt["c_oracle"]
    require(oracle["runtime_version"] == "0.27.0" and oracle["runtime_commit"] == runtime, "wrong C runtime")
    require(oracle.get("sources_manifest_sha256") == sources_sha, "C source manifest is not authenticated")
    require([oracle["grammar_artifact"]["repository"], oracle["grammar_artifact"]["commit"]] == grammar_pin, "wrong C grammar pin")
    require(re.fullmatch(r"[0-9a-f]{64}", oracle["grammar_artifact"]["sha256"]) is not None, "C grammar artifact is not authenticated")
    require(receipt["corpus"]["lock_sha256"] == lock_sha, "wrong corpus lock digest")
    require(receipt["corpus"]["selection"] == {"order": "largest", "max_files": 4, "max_file_bytes": 1024}, "workload selection changed")
    require(receipt["compact_route"]["status"] in ("accepted", "declined", "forest_route"), "missing route")
    if receipt["compact_route"]["status"] == "declined":
        require(bool(receipt["compact_route"].get("decline_reason")), "decline without reason")
    for section in ("fresh_parity", "incremental_parity", "invariant_gate"):
        result = receipt[section]
        require(result["status"] in ("pass", "fail", "unavailable", "timeout"), f"{section} did not run")
        if result["status"] == "unavailable":
            require(receipt["fresh_parity"].get("first_failure") is not None, "unavailable result without explanation")
    if receipt["incremental_parity"]["status"] != "timeout":
        require(receipt["incremental_parity"].get("reference") == "fresh_c", "incremental axes have not been separated")
    steps = receipt["incremental_parity"].get("steps", [])
    for step in steps:
        expected = step["invariant_pass"] and step["go_incremental_equals_fresh"] and step["locked_c_parity"] and not step.get("first_failure")
        require(step["pass"] == expected, f"step {step['step']} mixes C diagnostics into Go pass")
        if step.get("c_incremental_tree_sha256") and not step["c_incremental_equals_fresh"]:
            require(bool(step.get("c_incremental_failure")), "C disagreement lost its evidence")
        if step.get("c_incremental_tree_sha256") and not step["locked_c_incremental_parity"]:
            require(bool(step.get("locked_c_incremental_failure")), "Go/C incremental disagreement lost its evidence")
    if receipt["incremental_parity"]["status"] == "pass":
        require(len(steps) == 72 and all(step["pass"] for step in steps), "incomplete passing edit session")
    if receipt["invariant_gate"]["status"] == "pass":
        gate = receipt["invariant_gate"]
        require(gate["steps_passed"] == 72 and gate["no_edit_reparse_pass"] and gate["no_edit_reparse_allocs_per_run"] == 0, "incomplete passing invariant gate")
        require(all(gate[key] == 0 for key in ("root_coverage_failures", "error_root_failures", "incremental_fresh_failures")), "passing invariant gate has failures")
    for file in receipt["corpus"].get("files") or []:
        require(0 <= file["bytes"] <= 1024, "file outside recorded workload")
        require(re.fullmatch(r"[0-9a-f]{64}", file["sha256"]) is not None, "file without source digest")


def seal(report_dir, output_dir, revision):
    if re.fullmatch(r"[0-9a-f]{40}", revision) is None:
        raise ValueError("revision must be a full immutable Git SHA")
    pins = lock_entries(git_bytes(revision, "grammars/languages.lock"))
    if len(pins) != 206:
        raise ValueError("expected 206 default artifacts")
    pins.update(lock_entries(git_bytes(revision, "cgo_harness/grammar_receipts/lean_c_oracle.lock")))
    if len(pins) != 207:
        raise ValueError("expected 207 artifacts including Lean")
    source_data = git_bytes(revision, "cgo_harness/internal/coracle/upstream.json")
    source_manifest = json.loads(source_data)
    runtime = source_manifest["pins"]["RuntimeCommit"]
    lock_sha = git_bytes(revision, "cgo_harness/perf_scan/corpus_sources.lock.sha256").decode().split()[0]
    lean_sha = git_bytes(revision, "cgo_harness/grammar_receipts/lean_corpus_sources.lock.sha256").decode().split()[0]
    files, receipts, rows = {}, [], []
    for name, grammar_pin in sorted(pins.items()):
        data = (report_dir / f"{name}.json").read_bytes()
        receipt = json.loads(data)
        blob_path = f"grammars/grammar_blobs/{name}.bin" if name != "lean" else "grammars/lean/lean.bin"
        validate_receipt(receipt, name, revision, runtime, sha256(source_data), sha256(git_bytes(revision, blob_path)), grammar_pin, lean_sha if name == "lean" else lock_sha)
        files[f"receipts/{name}.json"] = data
        receipts.append(receipt)
        rows.append({"grammar": name, "receipt_sha256": sha256(data), "blob_sha256": receipt["grammar"]["blob_sha256"], "route": receipt["compact_route"]["status"], "fresh": receipt["fresh_parity"]["status"], "incremental": receipt["incremental_parity"]["status"], "invariant": receipt["invariant_gate"]["status"]})
    counts = {section: dict(Counter(receipt[section]["status"] for receipt in receipts)) for section in ("fresh_parity", "incremental_parity", "invariant_gate")}
    manifest = {"schema": "gts-qualification-baseline/v1", "gotreesitter_commit": revision, "c_runtime_commit": runtime, "c_sources_manifest_sha256": sha256(source_data), "corpus_lock_sha256": lock_sha, "lean_corpus_lock_sha256": lean_sha, "artifact_count": len(rows), "results": counts, "routes": dict(Counter(row["route"] for row in rows)), "artifacts": rows}
    # Normalize archive headers and gzip timestamp for reproducible bytes.
    buffer = io.BytesIO()
    with gzip.GzipFile(fileobj=buffer, mode="wb", filename="", mtime=0) as zipped:
        with tarfile.open(fileobj=zipped, mode="w", format=tarfile.USTAR_FORMAT) as archive:
            for name, data in sorted(files.items()):
                info = tarfile.TarInfo(name)
                info.size, info.mode = len(data), 0o644
                archive.addfile(info, io.BytesIO(data))
    archive_data = buffer.getvalue()
    archive_name = f"receipts-{sha256(archive_data)}.tar.gz"
    manifest["archive"] = {"path": archive_name, "sha256": sha256(archive_data), "bytes": len(archive_data)}
    output_dir.mkdir(parents=True, exist_ok=True)
    (output_dir / archive_name).write_bytes(archive_data)
    (output_dir / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    print(f"sealed {len(rows)} artifacts at {revision}; archive SHA-256 {sha256(archive_data)}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--report-dir", required=True, type=Path)
    parser.add_argument("--output-dir", required=True, type=Path)
    parser.add_argument("--revision", required=True)
    args = parser.parse_args()
    seal(args.report_dir, args.output_dir, args.revision)


if __name__ == "__main__":
    main()
