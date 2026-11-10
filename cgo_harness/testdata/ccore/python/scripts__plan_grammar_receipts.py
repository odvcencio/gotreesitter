#!/usr/bin/env python3
"""Choose the grammars that the grammar-receipts workflow runs.

Modes, picked from the GitHub event:

* workflow_dispatch: the `grammars` input. "all", "smoke", or a comma-separated list.
* schedule, release: every grammar (206 default grammars plus opt-in Lean 4).
* pull_request: the grammars whose files the pull request changes, at most
  PR_MAX of them; a small smoke set when the pull request changes none.

The script writes `matrix`, `count`, `mode`, `parallel`, and `note` to the file
named by GITHUB_OUTPUT (or stdout when unset) and a short Markdown note to
GITHUB_STEP_SUMMARY.
"""

import argparse
import json
import os
import re
import sys
from pathlib import Path

SMOKE = ["go", "css", "elixir"]
PR_MAX = 8
NAME_RE = re.compile(r"^[a-z0-9_]+$")


def locked_names(lock_path: Path):
    names = []
    for line in lock_path.read_text().splitlines():
        line = line.strip()
        if line and not line.startswith("#"):
            names.append(line.split()[0])
    if len(names) != 206 or len(set(names)) != 206:
        raise SystemExit(f"expected 206 unique locked default grammars, got {len(names)}")
    return names


def grammar_for_file(path: str, ordered_names):
    """Return the grammar a changed file belongs to, or None."""
    parts = path.split("/")
    if len(parts) < 2 or parts[0] != "grammars":
        return None
    if len(parts) >= 3 and parts[1] != "grammar_blobs" and parts[1] in ordered_names:
        return parts[1]
    base = parts[-1] if parts[1] == "grammar_blobs" else parts[1]
    for name in ordered_names:  # longest first, so c_sharp wins over c
        if base.startswith(name + "_") or base.startswith(name + "."):
            return name
    return None


def grammars_from_lock_patch(patch: str, names):
    found = set()
    for line in patch.splitlines():
        if line.startswith("+") and not line.startswith("+++"):
            fields = line[1:].split()
            if fields and fields[0] in names:
                found.add(fields[0])
    return found


def changed_grammars(files, names):
    ordered = sorted(names, key=lambda n: (-len(n), n))
    nameset = set(names)
    found = set()
    for item in files:
        path = item.get("filename", "")
        if path == "grammars/languages.lock":
            found |= grammars_from_lock_patch(item.get("patch", ""), nameset)
            continue
        name = grammar_for_file(path, ordered)
        if name:
            found.add(name)
    return sorted(found)


def parse_requested(value: str, everything):
    value = (value or "smoke").strip()
    if value == "all":
        return list(everything)
    if value == "smoke":
        return list(SMOKE)
    requested = [item.strip() for item in value.split(",") if item.strip()]
    if not requested:
        raise SystemExit("the grammars input is empty")
    known = set(everything)
    for name in requested:
        if not NAME_RE.match(name) or name not in known:
            raise SystemExit(f"unknown grammar: {name!r}")
    return sorted(set(requested), key=requested.index)


def emit(path, pairs):
    text = "".join(f"{key}={value}\n" for key, value in pairs)
    if path:
        with open(path, "a", encoding="utf-8") as handle:
            handle.write(text)
    else:
        sys.stdout.write(text)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo-root", default=".", type=Path)
    parser.add_argument("--event", default=os.environ.get("GITHUB_EVENT_NAME", ""))
    parser.add_argument("--grammars", default="", help="workflow_dispatch input")
    parser.add_argument("--max-parallel", default="", help="workflow_dispatch input")
    parser.add_argument("--pr-files", type=Path, help="JSON list from the pull request files API")
    args = parser.parse_args()

    names = locked_names(args.repo_root / "grammars" / "languages.lock")
    everything = names + ["lean"]

    note = ""
    if args.event == "pull_request":
        files = json.loads(args.pr_files.read_text()) if args.pr_files else []
        touched = changed_grammars(files, names)
        if not touched:
            selected, mode = list(SMOKE), "pr-smoke"
            note = "The pull request changes no grammar files; running the smoke set."
        elif len(touched) > PR_MAX:
            selected, mode = touched[:PR_MAX], "pr-changed"
            skipped = ", ".join(touched[PR_MAX:])
            note = f"{len(touched)} grammars changed; running the first {PR_MAX}. Not run here: {skipped}. Use workflow_dispatch for them."
        else:
            selected, mode = touched, "pr-changed"
            note = "Grammars whose files the pull request changes."
        parallel = 3
    elif args.event == "workflow_dispatch":
        selected = parse_requested(args.grammars, everything)
        mode = "dispatch"
        try:
            parallel = int(args.max_parallel or 6)
        except ValueError:
            raise SystemExit(f"max_parallel is not a number: {args.max_parallel!r}")
        parallel = max(1, min(8, parallel))
    elif args.event in ("schedule", "release"):
        selected, mode, parallel = list(everything), args.event, 6
    else:
        raise SystemExit(f"unsupported event {args.event!r}")

    matrix = {"include": [{"grammar": name} for name in selected]}
    emit(
        os.environ.get("GITHUB_OUTPUT"),
        [
            ("matrix", json.dumps(matrix, separators=(",", ":"))),
            ("count", len(selected)),
            ("mode", mode),
            ("parallel", parallel),
        ],
    )
    summary = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary:
        with open(summary, "a", encoding="utf-8") as handle:
            handle.write(f"### Grammar receipts plan\n\nMode `{mode}`, {len(selected)} grammar(s), {parallel} at a time: {', '.join(selected)}\n")
            if note:
                handle.write(f"\n{note}\n")


if __name__ == "__main__":
    main()
