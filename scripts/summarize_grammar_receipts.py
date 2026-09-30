#!/usr/bin/env python3
"""Write the O4 one-table summary and aggregate local Docker CPU metrics."""

import argparse
import json
from collections import Counter
from pathlib import Path


def load_receipts(report_dir: Path):
    receipts = []
    for path in sorted(report_dir.glob("*.json")):
        try:
            receipt = json.loads(path.read_text())
        except (OSError, json.JSONDecodeError):
            continue
        if receipt.get("schema") == "gts-grammar-receipt/v1":
            receipts.append(receipt)
    return receipts


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--report-dir", required=True, type=Path)
    args = parser.parse_args()
    report_dir = args.report_dir
    receipts = load_receipts(report_dir)
    route_counts = Counter(receipt["compact_route"]["status"] for receipt in receipts)
    cohort_counts = Counter(receipt["cohort"] for receipt in receipts)
    fresh_pass = sum(receipt["fresh_parity"]["status"] == "pass" for receipt in receipts)
    incremental_pass = sum(receipt["incremental_parity"]["status"] == "pass" for receipt in receipts)
    both_parity_pass = sum(
        receipt["fresh_parity"]["status"] == "pass" and receipt["incremental_parity"]["status"] == "pass"
        for receipt in receipts
    )
    c_incremental_disagreements = sum(
        not step["c_incremental_equals_fresh"]
        for receipt in receipts for step in receipt["incremental_parity"].get("steps", [])
        if step.get("c_incremental_tree_sha256") and step.get("c_fresh_tree_sha256")
    )
    go_c_incremental_disagreements = sum(
        not step["locked_c_incremental_parity"]
        for receipt in receipts for step in receipt["incremental_parity"].get("steps", [])
        if step.get("go_incremental_tree_sha256") and step.get("c_incremental_tree_sha256")
    )
    unavailable_count = sum("unavailable" in (receipt["fresh_parity"]["status"], receipt["incremental_parity"]["status"]) for receipt in receipts)
    revisions = sorted({receipt["gotreesitter_commit"] for receipt in receipts})
    runtimes = sorted({receipt["c_oracle"]["runtime_commit"] for receipt in receipts})
    invariant_pass = sum(receipt["invariant_gate"]["status"] == "pass" for receipt in receipts)
    timeout_count = sum(receipt.get("execution", {}).get("status") == "timeout" for receipt in receipts)
    error_lines = (report_dir / "generator-errors.txt").read_text(errors="replace").splitlines() if (report_dir / "generator-errors.txt").exists() else []
    receipt_names = {receipt.get("grammar", {}).get("name") for receipt in receipts}
    error_count = len({
        line.split("\t", 1)[0].split(":", 1)[0]
        for line in error_lines
        if line.strip() and line.split("\t", 1)[0].split(":", 1)[0] not in receipt_names
    })

    lines = [
        "# O4 grammar receipt run",
        "",
        f"Receipts: {len(receipts)} (target 207). These record current state; they do not graduate grammars.",
        f"Fresh locked-C parity: {fresh_pass}/{len(receipts)} pass; incremental locked-C parity: {incremental_pass}/{len(receipts)} pass; both: {both_parity_pass}/{len(receipts)} pass.",
        f"Invariant gate: {invariant_pass}/{len(receipts)} pass.",
        f"Recorded timeouts: {timeout_count}; unavailable samples: {unavailable_count}; generator errors: {error_count}.",
        "Go revisions: " + ", ".join(f"`{revision}`" for revision in revisions),
        "C runtime revisions: " + ", ".join(f"`{revision}`" for revision in runtimes),
        f"C incremental/fresh disagreements: {c_incremental_disagreements} steps; Go/C incremental disagreements: {go_c_incremental_disagreements} steps. Receipts with reference=fresh_c keep these axes diagnostic.",
        "",
        "Compact route counts: " + ", ".join(f"{name}={route_counts[name]}" for name in ("accepted", "declined", "forest_route")),
        "Cohort counts: " + ", ".join(f"{name}={cohort_counts[name]}" for name in ("1a", "1b", "2", "3", "4-A", "4-B", "4-C", "4-D", "4-E", "4-F", "Lean 4")),
        "",
        "| Grammar | Cohort | Route today | Decline reason | Fresh C | Go incremental vs fresh C | Go invariants | C incremental vs fresh | Go vs C incremental | Unavailable reason |",
        "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |",
    ]
    for receipt in receipts:
        route = receipt["compact_route"]
        reason = route.get("decline_reason", "") or "; ".join(route.get("decline_reasons", []))
        execution_status = receipt.get("execution", {}).get("status")
        if execution_status == "timeout":
            parity = invariant = f"timeout ({receipt['execution'].get('time_limit', 'limit unspecified')})"
        else:
            fresh_status = receipt["fresh_parity"]["status"]
            incremental_status = receipt["incremental_parity"]["status"]
            if fresh_status == "pass" and incremental_status == "pass":
                parity = "pass"
            elif "unavailable" in (fresh_status, incremental_status):
                parity = "unavailable"
            else:
                parity = "fail"
            invariant = receipt["invariant_gate"]["status"]
        escaped_reason = reason.replace("|", "\\|")
        steps = receipt["incremental_parity"].get("steps", [])
        c_steps = [step for step in steps if step.get("c_incremental_tree_sha256") and step.get("c_fresh_tree_sha256")]
        paired_steps = [step for step in steps if step.get("go_incremental_tree_sha256") and step.get("c_incremental_tree_sha256")]
        c_axis = f"{sum(step['c_incremental_equals_fresh'] for step in c_steps)}/{len(c_steps)} steps" if c_steps else "unavailable"
        paired_axis = f"{sum(step['locked_c_incremental_parity'] for step in paired_steps)}/{len(paired_steps)} steps" if paired_steps else "unavailable"
        unavailable_reason = ""
        if "unavailable" in (receipt["fresh_parity"]["status"], receipt["incremental_parity"]["status"]):
            failure = receipt["fresh_parity"].get("first_failure") or receipt["incremental_parity"].get("first_failure") or {}
            unavailable_reason = failure.get("error") or failure.get("category", "sample unavailable")
        lines.append(f"| {receipt['grammar']['name']} | {receipt['cohort']} | {route['status']} | {escaped_reason} | {receipt['fresh_parity']['status']} | {receipt['incremental_parity']['status']} | {invariant} | {c_axis} | {paired_axis} | {unavailable_reason.replace('|', r'\|')} |")
    (report_dir / "summary.md").write_text("\n".join(lines) + "\n")

    failures = {}
    for receipt in receipts:
        grammar = receipt["grammar"]["name"]
        for section in ("fresh_parity", "incremental_parity"):
            result = receipt[section]
            if result["status"] == "pass":
                continue
            candidates = [result.get("first_failure")]
            candidates.extend(file.get("first_divergence") for file in result.get("files", []))
            candidates.extend(step.get("first_failure") for step in result.get("steps", []))
            candidates = [failure for failure in candidates if failure]
            if not candidates:
                candidates = [{"category": "unspecified", "error": "failed without a first failure record"}]
            seen = set()
            for failure in candidates:
                category = failure.get("category", "unspecified")
                identity = (grammar, section, category)
                if identity in seen:
                    continue
                seen.add(identity)
                failures.setdefault(category, []).append((grammar, section, failure))
    finding_lines = ["# Receipt parity failures", ""]
    if not failures:
        finding_lines.append("No fresh or incremental locked-C parity failures were recorded.")
    else:
        finding_lines.append("Failures are grouped by recorded class. This list is evidence only; it does not decide whether a failure is already tracked.")
        for category in sorted(failures):
            finding_lines.extend(["", f"## {category}", ""])
            for grammar, section, failure in failures[category]:
                details = failure.get("path", "")
                if failure.get("error"):
                    details = (details + ": " if details else "") + failure["error"]
                values = ""
                if failure.get("go_value") or failure.get("c_value"):
                    values = f" (Go `{failure.get('go_value', '')}`, C `{failure.get('c_value', '')}`)"
                finding_lines.append(f"- `{grammar}` {section}: {details}{values}")
    (report_dir / "parity-failures.md").write_text("\n".join(finding_lines) + "\n")

    user_seconds = system_seconds = generator_elapsed_seconds = 0.0
    timed = 0
    for path in report_dir.glob(".*.time"):
        values = {}
        for line in path.read_text(errors="replace").splitlines():
            if "=" in line:
                key, value = line.split("=", 1)
                try:
                    values[key] = float(value)
                except ValueError:
                    continue
        if "receipt_user_seconds" in values and "receipt_system_seconds" in values:
            user_seconds += values["receipt_user_seconds"]
            system_seconds += values["receipt_system_seconds"]
            generator_elapsed_seconds += values.get("receipt_elapsed_seconds", 0.0)
            timed += 1
    runner_elapsed_seconds = 0.0
    runner_timed = 0
    for path in report_dir.glob(".*.runner-time"):
        for line in path.read_text(errors="replace").splitlines():
            if line.startswith("runner_elapsed_seconds="):
                try:
                    runner_elapsed_seconds += float(line.split("=", 1)[1])
                    runner_timed += 1
                except ValueError:
                    pass
    metrics = {
        "receipt_count": len(receipts),
        "timed_grammar_count": timed,
        "generator_user_cpu_seconds": round(user_seconds, 3),
        "generator_system_cpu_seconds": round(system_seconds, 3),
        "generator_total_cpu_seconds": round(user_seconds + system_seconds, 3),
        "generator_total_cpu_hours": round((user_seconds + system_seconds) / 3600, 4),
        "generator_elapsed_seconds": round(generator_elapsed_seconds, 3),
        "runner_timed_grammar_count": runner_timed,
        "runner_elapsed_seconds": round(runner_elapsed_seconds, 3),
        "runner_elapsed_hours": round(runner_elapsed_seconds / 3600, 4),
        "full_run_elapsed_seconds": int((report_dir / ".run-wall-seconds").read_text()) if (report_dir / ".run-wall-seconds").exists() else None,
        "route_counts": dict(route_counts),
        "cohort_counts": dict(cohort_counts),
        "fresh_parity_pass": fresh_pass,
        "incremental_parity_pass": incremental_pass,
        "fresh_and_incremental_parity_pass": both_parity_pass,
        "invariant_pass": invariant_pass,
        "unavailable_count": unavailable_count,
        "c_incremental_fresh_disagreement_steps": c_incremental_disagreements,
        "go_c_incremental_disagreement_steps": go_c_incremental_disagreements,
        "gotreesitter_revisions": revisions,
        "c_runtime_revisions": runtimes,
        "timeout_count": timeout_count,
        "generator_error_count": error_count,
    }
    (report_dir / "run-metrics.json").write_text(json.dumps(metrics, indent=2) + "\n")
    summary_path = report_dir / "summary.md"
    summary_path.write_text(
        summary_path.read_text()
        + f"\nGenerator CPU: {metrics['generator_total_cpu_seconds']:.3f} s ({metrics['generator_total_cpu_hours']:.4f} h).\n"
        + f"Summed per-grammar runner time: {metrics['runner_elapsed_seconds']:.3f} s ({metrics['runner_elapsed_hours']:.4f} h; {runner_timed} timed attempts for {len(receipts)} receipts).\n"
        + (f"Full local batch wall time: {metrics['full_run_elapsed_seconds']} s.\n" if metrics["full_run_elapsed_seconds"] is not None else "")
    )


if __name__ == "__main__":
    main()
