#!/usr/bin/env python3
"""Registry cross-check for the A1 live-arm inventory (campaign/a1-live-arm-inventory.md).

Reproduces the inventory's denominator and language-label counts from
testdata/result_compat_ownership_v1.json and cross-checks them against the
dispatcher arm identifiers referenced in the Go sources.

Usage (from repository root):
    python3 campaign/a1_registry_check.py

Exits 0 when every check passes, 1 otherwise.
"""

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
REGISTRY = ROOT / "testdata" / "result_compat_ownership_v1.json"
GO_SOURCES = ROOT / "parser_result_compat.go"

failures = []


def check(name, actual, expected):
    if actual == expected:
        print(f"ok   {name}: {actual}")
    else:
        failures.append(name)
        print(f"FAIL {name}: got {actual!r}, want {expected!r}")


def main():
    data = json.loads(REGISTRY.read_text())
    entries = data["entries"]
    denom = data["denominator"]

    live = [e for e in entries if e.get("status") == "live"]
    retired = [e for e in entries if e.get("status") != "live"]
    arms = [e for e in live if e.get("kind") == "dispatcher_arm"]

    check("schema", data.get("schema"), "gotreesitter/result-compat-ownership/v1")
    check("live_entries", denom.get("live_entries"), len(live))
    check("dispatcher_arms", denom.get("dispatcher_arms"), len(arms))
    # Count language labels over dispatcher-arm entries only. The one live
    # predicate entry carries cobol/COBOL labels that are not part of the
    # 33; tsx/typescript and c/cpp each share an arm and stay inside the 33.
    check("dispatcher_languages", denom.get("dispatcher_languages"),
          len({lang for e in arms for lang in e.get("languages", [])}))
    check("retired_entries", denom.get("retired_entries"), len(retired))

    # Cross-check against the Go dispatcher source: every live dispatcher arm
    # id should appear as a registered census identifier.
    src = GO_SOURCES.read_text() if GO_SOURCES.exists() else ""
    if not src:
        print(f"skip dispatcher-source cross-check ({GO_SOURCES} missing)")
        return 1 if failures else 0
    for arm in arms:
        if f'"{arm["id"]}"' not in src:
            failures.append(arm["id"])
            print(f"FAIL arm {arm['id']} not found in {GO_SOURCES.name}")
    print(f"{len(failures)} failure(s)")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
