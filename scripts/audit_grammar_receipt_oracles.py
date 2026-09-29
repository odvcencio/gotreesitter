#!/usr/bin/env python3
"""Report C incremental/fresh disagreements without changing parity verdicts.

Run only on demand. This reads gts-grammar-receipt/v1 files and emits evidence;
it does not parse source, rewrite receipts, or decide graduation policy.
"""

import argparse
import json
import re
from collections import Counter
from pathlib import Path


def digest(value):
    return isinstance(value, str) and re.fullmatch(r"[0-9a-f]{64}", value) is not None


def audit(receipts):
    counts = Counter()
    witnesses = []
    identities = set()
    seen = set()
    for receipt in receipts:
        grammar = receipt["grammar"]["name"]
        commit = receipt["gotreesitter_commit"]
        oracle = receipt["c_oracle"]
        identity = (grammar, commit, receipt["grammar"]["blob_sha256"],
                    oracle["runtime_commit"], receipt["corpus"]["manifest_sha256"])
        if identity in seen:
            raise ValueError(f"duplicate receipt for {grammar} at {commit}")
        seen.add(identity)
        identities.add((commit, oracle["runtime_commit"]))
        counts["receipts"] += 1
        for step in receipt["incremental_parity"].get("steps", []):
            counts["recorded_steps"] += 1
            c_inc = step.get("c_incremental_tree_sha256")
            c_fresh = step.get("c_fresh_tree_sha256")
            if not digest(c_inc) or not digest(c_fresh):
                counts["unmeasured_c_steps"] += 1
                continue
            counts["measured_c_steps"] += 1
            if c_inc == c_fresh:
                counts["c_incremental_equals_fresh"] += 1
                continue
            counts["c_incremental_differs_from_fresh"] += 1
            go_inc = step.get("go_incremental_tree_sha256")
            go_fresh = step.get("go_fresh_tree_sha256")
            go_matches_c_fresh = digest(go_inc) and go_inc == go_fresh == c_fresh
            if go_matches_c_fresh:
                counts["go_incremental_equals_go_fresh_equals_c_fresh"] += 1
            witnesses.append({
                "grammar": grammar, "gotreesitter_commit": commit,
                "git_tree_dirty": receipt["git_tree_dirty"],
                "grammar_blob_sha256": receipt["grammar"]["blob_sha256"],
                "c_oracle": oracle,
                "corpus_manifest_sha256": receipt["corpus"]["manifest_sha256"],
                "step": step["step"], "edit_class": step["edit_class"],
                "site": step["site"], "source_sha256": step["source_sha256"],
                "c_incremental_tree_sha256": c_inc, "c_fresh_tree_sha256": c_fresh,
                "go_incremental_tree_sha256": go_inc, "go_fresh_tree_sha256": go_fresh,
                "go_incremental_equals_go_fresh_equals_c_fresh": go_matches_c_fresh,
                "recorded_step_pass": step["pass"],
            })
    witnesses.sort(key=lambda row: (row["grammar"], row["gotreesitter_commit"], row["step"]))
    keys = ("receipts", "recorded_steps", "unmeasured_c_steps", "measured_c_steps",
            "c_incremental_equals_fresh", "c_incremental_differs_from_fresh",
            "go_incremental_equals_go_fresh_equals_c_fresh")
    return {
        "schema": "gts-receipt-oracle-consistency/v1",
        "scope": "recorded receipt steps; original verdicts are unchanged",
        "identities": [{"gotreesitter_commit": commit, "c_runtime_commit": runtime}
                       for commit, runtime in sorted(identities)],
        "counts": {key: counts[key] for key in keys},
        "affected_grammars": dict(sorted(Counter(row["grammar"] for row in witnesses).items())),
        "witnesses": witnesses,
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--report-dir", type=Path, required=True)
    parser.add_argument("--out", type=Path, help="write a new JSON artifact instead of stdout")
    args = parser.parse_args()
    receipts = []
    for path in sorted(args.report_dir.glob("*.json")):
        value = json.loads(path.read_text())
        if value.get("schema") == "gts-grammar-receipt/v1":
            receipts.append(value)
    if not receipts:
        parser.error("no gts-grammar-receipt/v1 files found")
    encoded = json.dumps(audit(receipts), indent=2, sort_keys=True) + "\n"
    if args.out is None:
        print(encoded, end="")
    else:
        # Do not overwrite an input receipt or an existing evidence artifact.
        with args.out.open("x") as output:
            output.write(encoded)


if __name__ == "__main__":
    main()
