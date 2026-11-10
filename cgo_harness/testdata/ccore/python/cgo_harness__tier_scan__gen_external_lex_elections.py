#!/usr/bin/env python3
"""Generate the Wave 4 external-lex-state election ledger.

The tier table answers "is this grammar byte-clean against C?" This ledger
answers the narrower Wave 4 question: for each published grammar, is there an
external scanner whose C-recovery election depends on a precise
ExternalLexStates table, and if so is that table default-elected, staged, or
still missing?
"""

import argparse
import json
import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
TIER_DIR = ROOT / "cgo_harness" / "tier_scan"
GRAMMARS_DIR = ROOT / "grammars"

EXTS_TSV = TIER_DIR / "exts.tsv"
CLASS_TSV = TIER_DIR / "tier_classification.tsv"
DEFAULT_JSON = TIER_DIR / "external_lex_elections.json"
DEFAULT_MD = TIER_DIR / "external_lex_elections.md"

EXPECTED_GRAMMAR_COUNT = 206

STATUS_ORDER = {
    "default_elected": 0,
    "staged_precise_els": 1,
    "blocked_missing_precise_els": 2,
    "not_applicable_no_external_scanner": 3,
}

STATUS_LABELS = {
    "default_elected": "default elected",
    "staged_precise_els": "staged precise ELS",
    "blocked_missing_precise_els": "blocked: missing precise ELS",
    "not_applicable_no_external_scanner": "not applicable: no external scanner",
}

C_RECOVERY_DEFAULT_OPT_OUT = {
    # Precise ELS is registered by default, but C recovery election is staged
    # because focused regressions show the faithful C recovery port changes
    # accepted trees for these grammars.
    "cpp",
    "html",
    "javascript",
    "julia",
}

RECEIPTS = {
    "default_elected": [
        "Docker: wave4-external-lex-election-inventory-test-v2",
        "TestExternalLexStatesDefaultElectionInventory",
        "Docker: wave4-cobol-default-precise-els",
        "TestCobolExternalLexStatesDefaultElection",
    ],
    "staged_precise_els": [
        "Docker: javascript-asi-comment-deep-review",
        "TestJavascriptExternalLexStatesRegression",
        "TestJavascriptExternalLexStatesDefaultWithoutRecoveryElection",
        "TestExternalLexStatesRecoveryElectionOptOutInventory",
    ],
    "sample_c_oracle_smoke": [
        "Docker: wave4-external-lex-smoke-20260707T1928",
        "angular/python/yaml clean; scss/wgsl classified recovery/error-shape IV",
    ],
    "wave3_inventory": [
        "Docker: wave3-tier-plan-206",
        "206 visited; 202 planned files; 4 planned-empty",
    ],
}


def read_exts():
    rows = {}
    with EXTS_TSV.open(encoding="utf-8") as f:
        for line in f:
            line = line.rstrip("\n")
            if not line:
                continue
            parts = line.split("\t", 1)
            rows[parts[0]] = parts[1] if len(parts) > 1 else ""
    return rows


def read_classification():
    rows = {}
    with CLASS_TSV.open(encoding="utf-8") as f:
        for lineno, line in enumerate(f, 1):
            parts = line.rstrip("\n").split("\t")
            if lineno == 1 and parts and parts[0] == "grammar":
                continue
            if len(parts) < 2 or not parts[0]:
                continue
            rows[parts[0]] = {
                "tier": parts[1],
                "parity": parts[2] if len(parts) > 2 else "",
                "notes": parts[3] if len(parts) > 3 else "",
            }
    return rows


def registered_scanners():
    names = set()
    for path in GRAMMARS_DIR.glob("z_subset_scanner_register_*.go"):
        text = path.read_text(encoding="utf-8")
        names.update(re.findall(r'RegisterExternalScanner\("([^"]+)"', text))

    zzz = GRAMMARS_DIR / "zzz_scanner_attachments.go"
    if zzz.exists():
        text = zzz.read_text(encoding="utf-8")
        names.update(
            re.findall(r'^\s*"([^"]+)":\s+[A-Za-z0-9_]+ExternalScanner\{\}', text, re.M)
        )
    return names


def extract_go_map_literal(text, marker):
    start = text.find(marker)
    if start < 0:
        return ""
    brace = text.find("{", start)
    if brace < 0:
        return ""
    depth = 0
    for pos in range(brace, len(text)):
        if text[pos] == "{":
            depth += 1
        elif text[pos] == "}":
            depth -= 1
            if depth == 0:
                return text[brace : pos + 1]
    raise SystemExit(f"unterminated Go map literal after marker {marker!r}")


def default_external_lex_states():
    names = set()
    for path in GRAMMARS_DIR.glob("*_external_lex_states*.go"):
        if path.name.endswith(("_test.go", "_staged.go")):
            continue
        text = path.read_text(encoding="utf-8")
        names.update(re.findall(r'RegisterExternalLexStates\("([^"]+)"', text))

    zzz = GRAMMARS_DIR / "zzz_scanner_attachments.go"
    if zzz.exists():
        text = zzz.read_text(encoding="utf-8")
        map_text = extract_go_map_literal(text, "externalLexStates := map[string][][]bool")
        names.update(re.findall(r'^\s*"([^"]+)":', map_text, re.M))
    return names


def staged_external_lex_states():
    names = set()
    for path in GRAMMARS_DIR.glob("*_external_lex_states_staged.go"):
        text = path.read_text(encoding="utf-8")
        found = re.findall(r'RegisterExternalLexStates\("([^"]+)"', text)
        if found:
            names.update(found)
        else:
            names.add(path.name.removesuffix("_external_lex_states_staged.go"))
    return names


def status_for(grammar, scanners, default_els, staged_els):
    if grammar not in scanners:
        return "not_applicable_no_external_scanner"
    if grammar in default_els:
        if grammar in C_RECOVERY_DEFAULT_OPT_OUT:
            return "staged_precise_els"
        return "default_elected"
    if grammar in staged_els:
        return "staged_precise_els"
    return "blocked_missing_precise_els"


def validate(doc, universe, scanners, default_els, staged_els):
    errors = []
    if len(universe) != EXPECTED_GRAMMAR_COUNT:
        errors.append(f"grammar universe has {len(universe)} rows, want {EXPECTED_GRAMMAR_COUNT}")
    for label, names in (
        ("external scanner", scanners),
        ("default ExternalLexStates", default_els),
        ("staged ExternalLexStates", staged_els),
    ):
        unknown = sorted(names - universe)
        if unknown:
            errors.append(f"{label} registrations outside exts.tsv: {', '.join(unknown)}")
    default_without_scanner = sorted(default_els - scanners)
    if default_without_scanner:
        errors.append(
            "default ExternalLexStates without scanner: " + ", ".join(default_without_scanner)
        )
    staged_without_scanner = sorted(staged_els - scanners)
    if staged_without_scanner:
        errors.append(
            "staged ExternalLexStates without scanner: " + ", ".join(staged_without_scanner)
        )
    overlap = sorted(default_els & staged_els)
    if overlap:
        errors.append("languages both default and staged: " + ", ".join(overlap))
    unknown_opt_out = sorted(C_RECOVERY_DEFAULT_OPT_OUT - universe)
    if unknown_opt_out:
        errors.append("C recovery default opt-outs outside exts.tsv: " + ", ".join(unknown_opt_out))
    opt_out_without_default_els = sorted(C_RECOVERY_DEFAULT_OPT_OUT - default_els)
    if opt_out_without_default_els:
        errors.append(
            "C recovery default opt-outs without default ExternalLexStates: "
            + ", ".join(opt_out_without_default_els)
        )

    row_names = {row["grammar"] for row in doc["grammars"]}
    if row_names != universe:
        missing = sorted(universe - row_names)
        extra = sorted(row_names - universe)
        if missing:
            errors.append("missing ledger rows: " + ", ".join(missing))
        if extra:
            errors.append("extra ledger rows: " + ", ".join(extra))

    histogram_total = sum(doc["histogram"].values())
    if histogram_total != len(universe):
        errors.append(f"histogram totals {histogram_total}, want {len(universe)}")

    if errors:
        raise SystemExit("external lex election ledger validation failed:\n- " + "\n- ".join(errors))


def build():
    exts = read_exts()
    classification = read_classification()
    universe = set(exts)
    scanners = registered_scanners()
    default_els = default_external_lex_states()
    staged_els = staged_external_lex_states()

    rows = []
    histogram = {status: 0 for status in STATUS_ORDER}
    for grammar in sorted(universe):
        status = status_for(grammar, scanners, default_els, staged_els)
        histogram[status] += 1
        cls = classification.get(grammar, {})
        rows.append(
            {
                "grammar": grammar,
                "status": status,
                "status_label": STATUS_LABELS[status],
                "extensions": exts.get(grammar, ""),
                "has_external_scanner": grammar in scanners,
                "default_external_lex_states": grammar in default_els,
                "staged_external_lex_states": grammar in staged_els,
                "c_recovery_default_opt_out": grammar in C_RECOVERY_DEFAULT_OPT_OUT,
                "tier_classification": cls.get("tier", "IV-unassessed"),
                "parity": cls.get("parity", "unmeasured"),
                "tier_notes": cls.get("notes", ""),
            }
        )

    doc = {
        "schema_version": 1,
        "wave": "wave4-external-lex-state-elections",
        "summary": {
            "grammar_count": len(universe),
            "external_scanner_count": len(scanners & universe),
            "default_external_lex_state_count": len(default_els & universe),
            "staged_external_lex_state_count": len(staged_els & universe),
            "c_recovery_default_opt_out_count": len(C_RECOVERY_DEFAULT_OPT_OUT & universe),
        },
        "histogram": histogram,
        "status_definitions": {
            "default_elected": (
                "External scanner and precise ExternalLexStates table are registered "
                "by default; DiagnoseCRecoveryGate supports the language and C recovery "
                "cost competition is default-enabled."
            ),
            "staged_precise_els": (
                "A precise ExternalLexStates table exists behind an opt-in build tag "
                "or explicit staging policy; default C recovery election is intentionally "
                "disabled."
            ),
            "blocked_missing_precise_els": (
                "External scanner is registered, but no precise ExternalLexStates table "
                "is registered or staged yet. Wave 4 cannot default-elect C recovery for "
                "this grammar until the table is extracted and certified."
            ),
            "not_applicable_no_external_scanner": (
                "No Go external scanner is registered for this grammar, so there is no "
                "external-lex-state election to perform. Tier/parity status remains "
                "governed by the ordinary C-oracle classification."
            ),
        },
        "receipts": RECEIPTS,
        "source_files": [
            "cgo_harness/tier_scan/exts.tsv",
            "cgo_harness/tier_scan/tier_classification.tsv",
            "grammars/zzz_scanner_attachments.go",
            "grammars/*_external_lex_states*.go (excluding tests and staged files)",
            "grammars/*_external_lex_states_staged.go",
            "grammars/z_subset_scanner_register_*.go",
        ],
        "grammars": rows,
    }
    validate(doc, universe, scanners, default_els, staged_els)
    return doc


def to_markdown(doc):
    lines = [
        "# Wave 4 External Lex-State Election Ledger",
        "",
        "This ledger covers every grammar in `cgo_harness/tier_scan/exts.tsv`.",
        "It tracks whether each grammar's external-scanner recovery path is",
        "default-elected, staged, blocked on a missing precise ExternalLexStates",
        "table, or not applicable because the grammar has no registered Go external",
        "scanner.",
        "",
        "## Summary",
        "",
        "| metric | count |",
        "| --- | ---: |",
        f"| grammars | {doc['summary']['grammar_count']} |",
        f"| registered external scanners | {doc['summary']['external_scanner_count']} |",
        f"| default precise ExternalLexStates tables | {doc['summary']['default_external_lex_state_count']} |",
        f"| staged precise ExternalLexStates tables | {doc['summary']['staged_external_lex_state_count']} |",
        f"| C recovery default opt-outs | {doc['summary']['c_recovery_default_opt_out_count']} |",
        "",
        "| status | count |",
        "| --- | ---: |",
    ]
    for status in sorted(STATUS_ORDER, key=STATUS_ORDER.get):
        lines.append(f"| {STATUS_LABELS[status]} | {doc['histogram'].get(status, 0)} |")

    lines += [
        "",
        "## Verification Receipts",
        "",
    ]
    for key, receipts in doc["receipts"].items():
        lines.append(f"- `{key}`: " + "; ".join(receipts))

    lines += [
        "",
        "## Status Definitions",
        "",
    ]
    for status in sorted(STATUS_ORDER, key=STATUS_ORDER.get):
        lines.append(f"- `{status}`: {doc['status_definitions'][status]}")

    lines += [
        "",
        "## Grammar Ledger",
        "",
        "| grammar | status | tier row | parity | scanner | default ELS | staged ELS | recovery opt-out | extensions |",
        "| --- | --- | --- | --- | --- | --- | --- | --- | --- |",
    ]
    rows = sorted(
        doc["grammars"],
        key=lambda row: (STATUS_ORDER[row["status"]], row["grammar"]),
    )
    for row in rows:
        scanner = "yes" if row["has_external_scanner"] else "no"
        default = "yes" if row["default_external_lex_states"] else "no"
        staged = "yes" if row["staged_external_lex_states"] else "no"
        opt_out = "yes" if row["c_recovery_default_opt_out"] else "no"
        lines.append(
            f"| `{row['grammar']}` | {row['status_label']} | "
            f"{row['tier_classification']} | {row['parity']} | {scanner} | "
            f"{default} | {staged} | {opt_out} | `{row['extensions']}` |"
        )
    lines.append("")
    return "\n".join(lines)


def write_if_changed(path, data):
    old = path.read_text(encoding="utf-8") if path.exists() else None
    if old == data:
        return False
    path.write_text(data, encoding="utf-8")
    return True


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--out-json", default=str(DEFAULT_JSON))
    parser.add_argument("--out-md", default=str(DEFAULT_MD))
    parser.add_argument("--check", action="store_true", help="verify committed outputs are current")
    args = parser.parse_args()

    doc = build()
    json_data = json.dumps(doc, indent=1, sort_keys=True) + "\n"
    md_data = to_markdown(doc)

    json_path = Path(args.out_json)
    md_path = Path(args.out_md)
    if args.check:
        mismatches = []
        for path, data in ((json_path, json_data), (md_path, md_data)):
            if not path.exists() or path.read_text(encoding="utf-8") != data:
                mismatches.append(str(path))
        if mismatches:
            print("external lex election ledger is stale:", file=sys.stderr)
            for path in mismatches:
                print(f"  {path}", file=sys.stderr)
            return 1
        print("external lex election ledger is current")
        return 0

    changed = []
    if write_if_changed(json_path, json_data):
        changed.append(str(json_path))
    if write_if_changed(md_path, md_data):
        changed.append(str(md_path))
    h = doc["histogram"]
    print(
        "external-lex-elections: "
        + "  ".join(f"{status}={h.get(status, 0)}" for status in sorted(STATUS_ORDER, key=STATUS_ORDER.get))
    )
    if changed:
        print("wrote " + ", ".join(changed))
    else:
        print("outputs already current")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
