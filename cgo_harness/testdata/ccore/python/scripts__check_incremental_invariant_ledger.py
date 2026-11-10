#!/usr/bin/env python3
"""Reject an increase in the incremental invariant ledger on a pull request."""

import json
import subprocess
import sys
from pathlib import Path


LEDGER = "testdata/incremental_invariant_ledger.json"


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: check_incremental_invariant_ledger.py BASE_REVISION", file=sys.stderr)
        return 2
    current = json.loads(Path(LEDGER).read_text())
    previous = subprocess.run(
        ["git", "show", f"{sys.argv[1]}:{LEDGER}"], capture_output=True, text=True
    )
    if previous.returncode != 0:
        # The first PR adds this ledger. A missing base file is valid only then.
        exists = subprocess.run(
            ["git", "cat-file", "-e", f"{sys.argv[1]}^{{commit}}"], capture_output=True
        )
        if exists.returncode != 0:
            print(previous.stderr, file=sys.stderr)
            return 1
        print("base has no incremental invariant ledger; checking new entries in Go tests")
        return 0
    baseline = json.loads(previous.stdout)
    if current["version"] != baseline["version"]:
        print("invariant ledger version changed", file=sys.stderr)
        return 1
    failures = []
    for section in ("sessions", "probes"):
        for name, old in baseline[section].items():
            new = current[section].get(name)
            if new is None:
                failures.append(f"{section}/{name}: entry removed")
                continue
            for key in ("steps", "reported_baseline"):
                if new.get(key) != old.get(key):
                    failures.append(f"{section}/{name}: {key} changed")
            for route in ("production", "compact"):
                if new[route] > old[route]:
                    failures.append(f"{section}/{name}/{route}: {old[route]} -> {new[route]}")
    if failures:
        print("incremental invariant ledger increased:\n" + "\n".join(failures), file=sys.stderr)
        return 1
    print("incremental invariant ledger did not increase")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
