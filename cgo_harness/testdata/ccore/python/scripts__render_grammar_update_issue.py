#!/usr/bin/env python3
"""Render the read-only grammar plan and scanner guard as an issue body."""

import argparse
import json
from pathlib import Path


def cell(value):
    return str(value).replace("|", "\\|").replace("\n", " ").replace("\r", " ")


def public_reason(reason):
    # Failed checks can contain temporary paths or network diagnostics.
    # Keep the cause in the public issue; detailed errors stay in artifacts.
    if reason.startswith("fetch old ref:"):
        return "Could not fetch current ref; scanner check incomplete"
    if reason.startswith("fetch new ref:"):
        return "Could not fetch new ref; scanner check incomplete"
    if reason.startswith("read ") and ":" in reason:
        return "Could not read " + reason.split(":", 1)[0][5:] + "; check incomplete"
    return reason


def render(plan, guard):
    if plan.get("error_count", 0) or plan.get("write_applied"):
        raise ValueError("expected a successful read-only update plan")
    updates = sorted(
        (row for row in plan["results"] if row["status"] == "available"),
        key=lambda row: row["name"],
    )
    checks = {row["name"]: row for row in guard["results"]}
    lines = [
        "Upstream grammar updates are available. The weekly check leaves pinned refs and generated blobs unchanged.",
        "",
        "Each grammar update needs its own PR with regenerated blobs, CI results, and per-grammar locked-C parity results. Scanner-facing changes also need a reviewed Go scanner port before the update can proceed.",
        "",
        "Scope: `grammars/update_tier1_core100.txt`.",
        "",
        f"Available updates: {len(updates)}.",
        "",
        "| Grammar | Current ref | New ref | Upstream compare | Scanner-facing changes | Held back reason |",
        "| --- | --- | --- | --- | --- | --- |",
    ]
    for update in updates:
        check = checks.get(update["name"])
        if check is None or any(
            check.get(key) != update.get(key) for key in ("old_ref", "new_ref", "repo_url")
        ):
            raise ValueError(f"missing or mismatched scanner check for {update['name']}")
        reasons = check.get("reasons", [])
        changed = any(row["changed"] for row in check.get("source_files", [])) or (
            check.get("expected_externals", []) != check.get("actual_externals", [])
        )
        scanner = "Yes" if changed else "Unknown" if check["blocked"] else "No"
        held_back = "; ".join(public_reason(reason) for reason in reasons) if check["blocked"] else "None from scanner check"
        repo = update["repo_url"].removesuffix(".git").rstrip("/")
        compare = f"{repo}/compare/{update['old_ref']}...{update['new_ref']}"
        fields = [
            update["name"],
            f"`{update['old_ref']}`",
            f"`{update['new_ref']}`",
            f"[Compare]({compare})",
            scanner,
            held_back or "Scanner check blocked the update",
        ]
        lines.append("| " + " | ".join(cell(field) for field in fields) + " |")
    if not updates:
        lines = ["No upstream grammar updates are pending in the checked scope."]
    lines += [
        "",
        "The workflow artifact contains the update plan, scanner check, held-back list, and this report. This report does not regenerate blobs or run parity checks.",
    ]
    return "\n".join(lines) + "\n"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--plan", type=Path, required=True)
    parser.add_argument("--guard", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    args.output.write_text(render(json.loads(args.plan.read_text()), json.loads(args.guard.read_text())))


if __name__ == "__main__":
    main()
