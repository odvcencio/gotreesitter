#!/usr/bin/env python3
"""Build or check an external measured receipt; never write measurements into Git."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import statistics
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
LANGUAGES = ["go", "java", "javascript", "typescript", "python", "rust", "c", "cpp", "c_sharp", "ruby", "php", "bash"]
SIZES = {"32k": 32 << 10, "137k": 137 << 10, "1m": 1 << 20}
OPERATIONS = ["fresh", "byte"]
MATRIX = REPORT = BENCHSTAT = None
BENCHSTAT_VERSION = "golang.org/x/perf v0.0.0-20260929162123-406019bb8b68"
CORPUS_SHA = "41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea"
HARNESS_FILES = ["engine_ceiling_test.go", "engine_ceiling_cgo.go", "compact_graduation_test.go", "compact_edits_work_test.go", "compact_edits_rss_test.go", "parity_c_loader_cgo.go", "go.mod", "go.sum"]


def sha(data):
    return hashlib.sha256(data).hexdigest()


def source_fingerprint(harness=False):
    if harness:
        paths = [ROOT / "cgo_harness" / name for name in HARNESS_FILES]
    else:
        paths = [path for path in ROOT.iterdir() if path.is_file()]
        paths += [path for folder in ["internal", "grammars"] for path in (ROOT / folder).rglob("*") if path.is_file()]
        paths = [path for path in paths if (path.suffix == ".go" and not path.name.endswith("_test.go") or path.suffix == ".bin") and not path.relative_to(ROOT).as_posix().startswith(("internal/graduation/", "internal/benchfixtures/"))]
    result = hashlib.sha256()
    for path in sorted(paths, key=lambda p: p.relative_to(ROOT).as_posix()):
        result.update(path.relative_to(ROOT).as_posix().encode())
        result.update(b"\0")
        result.update(hashlib.sha256(path.read_bytes()).digest())
    return result.hexdigest()


def observations(path, marker):
    return [json.loads(line.split(marker, 1)[1]) for line in path.read_text().splitlines() if marker + "{" in line]


def timing(path):
    text = path.read_text()
    if not text.rstrip().endswith("# status: complete"):
        raise ValueError(f"incomplete randomized campaign: {path.name}")
    seed = None
    rows = {}
    for line in text.splitlines():
        match = re.match(r"# seed: (\d+);", line)
        if match:
            seed = int(match[1])
        match = re.match(r"BenchmarkCompactGraduation/([^/]+)/([^/]+)/([^/]+)/([^/]+)/pass(\d+)(?:-\d+)?\s+\d+\s+(.*)", line)
        if not match:
            continue
        language, size, operation, engine, pass_text, metrics = match.groups()
        values = dict((unit, float(value)) for value, unit in re.findall(r"([\d.eE+\-]+)\s+(ns/op|B/op|allocs/op|compact-served/op|compact-declined/op)", metrics))
        if seed is None or not all(unit in values for unit in ["ns/op", "B/op", "allocs/op"]):
            raise ValueError("benchmark has no seed or standard metrics")
        rows.setdefault((language, size, operation, engine), []).append({
            "seed": seed, "pass": int(pass_text), "ns_per_op": values["ns/op"],
            "bytes_per_op": values["B/op"], "allocs_per_op": values["allocs/op"],
            "compact_served_per_op": values.get("compact-served/op", 0),
            "compact_declined_per_op": values.get("compact-declined/op", 0),
        })
    return rows


def per_seed(samples, metric="ns_per_op"):
    result = {}
    for row in samples:
        result.setdefault(row["seed"], []).append(row[metric])
    if set(result) != set(range(1, 21)) or any(len(values) != 2 for values in result.values()):
        raise ValueError("each engine cell needs two passes for every one of twenty seeds")
    return {seed: statistics.median(values) for seed, values in result.items()}


def spread(values):
    values = list(values)
    return {"median": statistics.median(values), "min": min(values), "max": max(values)}


def summary(cell):
    result = {}
    seeded = {}
    for engine in ["legacy", "compact", "C"]:
        seeded[engine] = per_seed(cell["timing"][engine])
        result[engine] = {metric: spread(per_seed(cell["timing"][engine], metric).values()) for metric in ["ns_per_op", "bytes_per_op", "allocs_per_op"]}
    for engine in ["legacy", "C"]:
        ratios = [seeded["compact"][seed] / seeded[engine][seed] for seed in range(1, 21)]
        result["compact_over_" + engine] = {**spread(ratios), "seed_ratios": ratios}
    return result


def gate(name, evidence):
    return {"name": name, "status": "not_established", "evidence": evidence}


def build(evidence, revision):
    matrix = {
        "schema": "compact-graduation/v1", "candidate_revision": revision,
        "legacy_revision": "58e5bec8c", "corpus_lock_sha256": CORPUS_SHA,
        "runtime_source_sha256": source_fingerprint(), "harness_source_sha256": source_fingerprint(True),
        "fixture_manifest_sha256": sha((ROOT / "internal/benchfixtures/generated.json").read_bytes()),
        "protocol": {"seeds": list(range(1, 21)), "benchtime": "750ms", "gomaxprocs": 1, "count_per_process": 1, "paired_cycle": "Go-C-C-Go", "warmup_operations": 2,
            "notes": "GOWORK=off; Go 1.25.14, linux/amd64, native grammar compiler GCC 12.2.0; one grammar per process in a busy shared VM. Tree checks and counters finished before timing. Timing workers overlap on distinct CPU affinities: go/rust=1; typescript/python=0; java/ruby=2; javascript/cpp=3; c_sharp=4; bash=5; c=6; php=7. The C#, Bash, C and PHP workers started later. Containers on CPU 0-3 have an 8 GiB cap and 6 GiB Go soft limit; CPU 4-7 have a 4 GiB cap and 3 GiB Go soft limit. Each RSS observation uses a separate process on the language's CPU. The measured tooling revision includes the bracketed benchmark; the empty checked-default wiring was applied in the worktree before timing, and its complete runtime fingerprint is pinned here. Seed summaries average the two bracketed passes; report the median and min-max of twenty seed summaries. Native C includes edit, parse and release; input buffers and parser setup are outside timing. Compact includes declines, fallback and hidden verification. Deltas under 5% are not claimed as wins. Work profiling is separate from timing; compact peak-version and complete shift/reduction counters remain unavailable. This generated clean fixture matrix does not replace the full v1 corpus/session/query gates."},
        "prerequisites": [
            gate("E-A_exit", "The required all-grammar typical-file compact/legacy exit receipt is not established by this generated matrix."),
            gate("M1_frozen_baseline", "Comparison freezes the current branch engine's explicit legacy route; an M1 release baseline has not been established."),
            gate("M1_C_oracle", "This lane uses the existing locked C 0.25.1 runtime, not the future M1 C 0.27.x receipt."),
        ], "languages": [],
    }
    affinity_path = evidence / "seed-affinities.json"
    if affinity_path.exists():
        affinities = json.loads(affinity_path.read_text())
        matrix["protocol"]["notes"] += " Remaining Python, C# and C++ seeds were distributed over freed workers after completed-seed boundaries. Each seed still has one process, identical warmups and bracket order, and the original per-language memory limits. Their seed-to-CPU assignments are " + json.dumps(affinities, sort_keys=True, separators=(",", ":")) + ". Final RSS observations for those languages and Ruby also use separate processes over freed workers with the original per-language limits."
    matrix["legacy_revision"] = subprocess.check_output(["git", "rev-parse", "58e5bec8c"], cwd=ROOT, text=True).strip()
    manifest = json.loads((ROOT / "internal/benchfixtures/generated.json").read_text())
    pins = {(row["language"], row["target_bytes"]): row for row in manifest["entries"]}
    for name in LANGUAGES:
        shape = "sh" if name == "bash" else name
        checks = []
        identities = []
        for size in SIZES:
            path = evidence / f"correctness-{name}-{size}.log"
            checks += observations(path, "GRADUATION_CORRECTNESS ")
            identities += observations(path, "GRADUATION_ORACLE ")
        identity = identities[0]
        for other in identities[1:]:
            for key in ["binding_commit", "runtime_commit", "grammar_commit", "grammar_artifact_sha256"]:
                if identity[key] != other[key]:
                    raise ValueError(f"{name}: locked C identity changed across processes")
        identity = {key: value for key, value in identity.items() if not key.endswith("_path")}
        work = observations(evidence / f"work-{name}.log", "WHOLE_WORK ")
        samples = timing(evidence / f"timing-{name}.txt")
        language = {"language": name, "fixture_shape": shape, "oracle_identity": identity, "cells": [], "rss": [],
            "gates": [
                gate("locked_real_corpus", "Only the generated clean sources were measured here; the locked real-file fresh parity receipt remains required."),
                gate("edit_session_72", "Four clean alternating replacements are checked per generated size; the 72-step recovery session is a separate required receipt."),
                gate("edit_sites_16", "This matrix measures the pinned first identifier replacement; sixteen sites and insert/delete/replace classes remain required."),
                gate("cliff", "No paired C version logger receipt is supplied by this matrix."),
                gate("race", "Focused routing/checker races are separate from a full per-grammar safety receipt."),
                gate("memory_budget", "Default budgets remain enabled; the full pathological and retained-memory safety receipt remains required."),
                gate("highlight_tags", "The compact highlight/tags supertype and MISSING query receipt remains required."),
            ], "graduated": False, "blockers": []}
        for size, target in SIZES.items():
            pin = pins[(shape, target)]
            for operation in OPERATIONS:
                selected = [r for r in checks if r["size"] == size and r["mode"] == operation]
                source_sha = pin["sha256"]
                edited_sha = next((r["source_sha256"] for r in selected if operation == "byte" and r["step"] == 0), source_sha)
                cell = {"size": size, "operation": operation, "source_bytes": pin["source_bytes"], "source_sha256": source_sha,
                    "edited_source_sha256": edited_sha, "correctness": {}, "counters": {}, "timing": {}, "summary": {}}
                for engine in ["legacy", "compact"]:
                    rows = sorted([r for r in selected if r["engine"] == engine], key=lambda r: r["step"])
                    cell["correctness"][engine] = [{key: value for key, value in row.items() if key not in ["language", "size", "mode", "engine", "source_bytes"]} for row in rows]
                    counted = sorted([r for r in work if r["size"] == size and r["mode"] == operation and r["engine"] == engine], key=lambda r: r["direction"])
                    cell["counters"][engine] = []
                    for row in counted:
                        flat = {key: value for key, value in row["work"].items() if isinstance(value, (int, float, str, bool))}
                        runtime = rows[row["direction"] if operation == "byte" else 0]["runtime"]
                        cell["counters"][engine].append({"direction": row["direction"], "whole_table_lookups_proxy": flat["table_lookups_proxy"],
                            "tokens": row["profile_tokens"] if operation == "byte" else runtime["TokensConsumed"],
                            "nodes": row["profile_nodes"] if operation == "byte" else runtime["NodesAllocated"],
                            "max_live_versions": runtime["MaxStacksSeen"], "reused_subtrees": row["reused_subtrees"], "reused_bytes": row["reused_bytes"],
                            "whole_counters_complete": engine == "legacy", "raw_work": flat})
                for engine in ["legacy", "compact", "C"]:
                    cell["timing"][engine] = samples[(name, size, operation, engine)]
                cell["summary"] = summary(cell)
                language["cells"].append(cell)
        for operation in OPERATIONS:
            for engine in ["legacy", "compact", "C"]:
                resident = []
                probes = []
                source_bytes = None
                for repeat in range(1, 4):
                    path = evidence / f"rss-{name}-{operation}-{engine}-{repeat}.log"
                    row, = observations(path, "RSS ")
                    time_match = re.search(r"Maximum resident set size \(kbytes\): (\d+)", path.read_text())
                    if not time_match or int(time_match[1]) * 1024 < row["max_rss_bytes"]:
                        raise ValueError(f"{path.name}: final /usr/bin/time peak is absent or below the in-process probe")
                    resident.append(int(time_match[1]) * 1024)
                    probes.append(row["max_rss_bytes"])
                    if source_bytes is not None and source_bytes != row["source_bytes"]:
                        raise ValueError("RSS source identity changed")
                    source_bytes = row["source_bytes"]
                language["rss"].append({"engine": engine, "operation": operation, "source_bytes": source_bytes, "max_rss_bytes": resident, "probe_max_rss_bytes": probes})
        matrix["languages"].append(language)
    return matrix


def display(matrix):
    lines = ["# Compact graduation matrix", "", "The matrix covers all twelve requested languages at 32 KiB, 137 KiB and 1 MiB. Default routing is checked against the external receipt. A correct legacy fallback cannot graduate compact.", "",
        f"Measured tooling revision: `{matrix['candidate_revision']}`. Legacy comparison: `{matrix['legacy_revision']}`, forced off on the same engine. Runtime and harness SHA-256 identities are in the external checked receipt. The comparison does not establish an M1 baseline or the E-A exit gate.", "",
        "The measured toolchain is Go 1.25.14 on linux/amd64, with GCC 12.2.0 for the native grammars. Each cell has twenty explicit shuffle seeds, two passes per engine in Go-C-C-Go order, 750 ms, `GOMAXPROCS=1`, `GOWORK=off`, `-count=1` and `-benchmem`. Inputs, grammar loading and initial edit trees are outside timing. Fresh includes parse and release; edits include `Tree.Edit`, parse, verification and old-tree release. C uses native buffers and the locked 0.25.1 runtime (`f5afe475deb7c0bae6407fb776c76824f717bb61`). Compact timings include declines and legacy retry.", "",
        "The VM is shared and busy. Timing workers overlap on distinct CPU affinities: Go/Rust on CPU 1, TypeScript/Python on CPU 0, Java/Ruby on CPU 2, JavaScript/C++ on CPU 3, C# on CPU 4, Bash on CPU 5, C on CPU 6, and PHP on CPU 7. The C#, Bash, C and PHP workers started later. CPU 0–3 containers have an 8 GiB cap and 6 GiB Go soft limit; CPU 4–7 have a 4 GiB cap and 3 GiB Go soft limit. Tree checks and counters completed before timing. The empty checked-default wiring was applied before timing; the runtime fingerprint records those sources as well as the measured tooling revision. Times below are medians of the twenty per-seed averages of two passes; brackets show the minimum and maximum seed averages. Ratios are medians of paired per-seed ratios. Deltas under 5% are descriptive, not claimed wins. Bytes/op and allocations/op, every raw timing sample and deterministic counter observation remain in the JSON. The external benchstat comparison uses twenty seed averages, rather than treating the two passes as independent observations.", "",
        "| Language | Size | Fresh compact / legacy / C ms [min–max] | Edit compact / legacy / C ms [min–max] | Compact/legacy fresh / edit | Compact/C fresh / edit | Default |",
        "| --- | --- | --- | --- | --- | --- | --- |"]
    for language in matrix["languages"]:
        for size in SIZES:
            fresh = next(c for c in language["cells"] if c["size"] == size and c["operation"] == "fresh")
            edit = next(c for c in language["cells"] if c["size"] == size and c["operation"] == "byte")
            def durations(cell):
                return " / ".join(f"{cell['summary'][e]['ns_per_op']['median']/1e6:.3f} [{cell['summary'][e]['ns_per_op']['min']/1e6:.3f}–{cell['summary'][e]['ns_per_op']['max']/1e6:.3f}]" for e in ["compact", "legacy", "C"])
            def ratios(engine):
                return " / ".join(f"{c['summary']['compact_over_'+engine]['median']:.2f}x" for c in [fresh, edit])
            lines.append(f"| {language['language']} | {size} | {durations(fresh)} | {durations(edit)} | {ratios('legacy')} | {ratios('C')} | {'compact' if language['graduated'] else 'legacy'} |")
    if "seed-to-CPU assignments" in matrix["protocol"]["notes"]:
        lines += ["", "After completed-seed boundaries, the remaining Python, C# and C++ seeds ran on freed workers. Every seed still has one process, the same bracket order and two warmups, and the original per-language memory limits. Their complete seed-to-CPU assignments are recorded in the artifact's protocol notes. Their final RSS processes, and Ruby’s final RSS observations, also use those original language limits on freed CPUs."]
    lines += ["", "Fresh-C and incremental-equals-fresh use full public tree digests, including fields, flags, ranges and points. Each route has one fresh check and four alternating edit checks per size, with three no-edit allocation runs after every check. These clean steps establish the generated matrix's equivalence; they do not certify the full 72-step recovery session or sixteen edit sites.", "",
        "| Language | Compact cells served / 6 | Fresh-C checks | Incremental=fresh edit steps | No-edit allocations | Graduation blockers |", "| --- | ---: | ---: | ---: | ---: | --- |"]
    for language in matrix["languages"]:
        checks = [r for c in language["cells"] for r in c["correctness"]["compact"]]
        edits = [r for c in language["cells"] if c["operation"] == "byte" for r in c["correctness"]["compact"]]
        served = sum(all(r["requested_served"] for r in c["correctness"]["compact"]) for c in language["cells"])
        reasons = sorted(set(reason.split(": ", 1)[-1] for reason in language["blockers"] if not reason.endswith("not_established")))
        lines.append(f"| {language['language']} | {served}/6 | {sum(r['fresh_C_equal'] for r in checks)}/{len(checks)} | {sum(r['incremental_fresh_equal'] for r in edits)}/{len(edits)} | {sum(r['no_edit_allocations'] for r in checks):g} | {'; '.join(reasons)} |")
    lines += ["", "Every language also retains unestablished E-A, M1 baseline/oracle, real-corpus, 72-step session, sixteen-site, cliff logger, full per-grammar race/safety and query gates. They stay blocking. Compact's always-on peak-version and full shift/reduction counters are incomplete; their observed zeros never certify less work. The complete table-lookup proxy and selected-pass token/node/reuse observations are recorded separately. The existing 2% ledger threshold and all safety limits are unchanged.", "",
        "The owner's correctness alternative remains available: a compact tree equal to C while the legacy result differs can justify the timing trade. These generated clean cells contain no such difference. Go's historical parser-cliff correctness reason remains in the [historical incremental receipt](https://github.com/odvcencio/gotreesitter/blob/58e5bec8c/docs/compact-incremental-edit-receipt.md); this generated matrix does not re-certify or revoke that separate witness.", "",
        "| Language / size | Whole lookup proxy legacy → compact, directions 0 / 1 | Edit tokens legacy → compact | Edit new nodes legacy → compact | Reused subtrees legacy → compact | Reused bytes legacy → compact |",
        "| --- | --- | --- | --- | --- | --- |"]
    for language in matrix["languages"]:
        for cell in language["cells"]:
            if cell["operation"] != "byte":
                continue
            def counter(key):
                return " / ".join(f"{cell['counters']['legacy'][direction][key]} → {cell['counters']['compact'][direction][key]}" for direction in range(2))
            lines.append(f"| {language['language']} / {cell['size']} | {counter('whole_table_lookups_proxy')} | {counter('tokens')} | {counter('nodes')} | {counter('reused_subtrees')} | {counter('reused_bytes')} |")
    lines += ["", "Both edit directions are shown. Tokens, new nodes and reuse are selected-pass profile observations; the lookup proxy covers the complete request. All fresh counters and the remaining direct/proxy observations are in the JSON. No existing ledger or threshold was refreshed.", "",
        "| Language | 1 MiB fresh RSS compact / legacy / C bytes per source byte [min–max] | 1 MiB edit RSS compact / legacy / C [min–max] |", "| --- | --- | --- |"]
    for language in matrix["languages"]:
        def rss(operation):
            values = []
            for engine in ["compact", "legacy", "C"]:
                row = next(r for r in language["rss"] if r["engine"] == engine and r["operation"] == operation)
                measured = [v / row["source_bytes"] for v in row["max_rss_bytes"]]
                values.append(f"{statistics.median(measured):.1f} [{min(measured):.1f}–{max(measured):.1f}]")
            return " / ".join(values)
        lines.append(f"| {language['language']} | {rss('fresh')} | {rss('byte')} |")
    lines += ["", "RSS uses the final process peak from `/usr/bin/time -v`. The JSON also retains each in-process probe; logging after that probe can raise the final peak. The larger final measurement controls the unchanged safety gate.", "", "Reproduce inside Docker, one language per process:", "", "```sh", "bash cgo_harness/docker/run_parity_in_docker.sh --no-build \\", "  --cpuset-cpus 1 --mount /tmp/graduation-evidence:/evidence -- \\", "  'cd /workspace && bash scripts/run_compact_graduation.sh /evidence'", f"GOWORK=off go install golang.org/x/perf/cmd/benchstat@{BENCHSTAT_VERSION.split()[-1]}", "GOWORK=off python3 scripts/compact_graduation.py --evidence-dir /tmp/graduation-evidence --output-dir /tmp/graduation-receipt --benchstat \"$(command -v benchstat)\"", "GOWORK=off go test ./internal/graduation", "GOWORK=off go run ./cmd/compactgraduation --matrix /tmp/graduation-receipt/matrix.json", "```", "",
        "The measurement runner preserves failed test exits and still collects the remaining cells. The checker rejects incomplete samples, stale engine/harness inputs, missing sizes, unproved C identities, forged tree equality, and a routing list that differs from the measured decisions. Running the artifact gate successfully means the recorded decisions are justified; it does not turn a blocked language's correctness, memory or performance gate green.", "", "```text"]
    for language in matrix["languages"]:
        for cell in language["cells"]:
            s = cell["summary"]
            lines.append(f"METRIC: {language['language']}/{cell['size']}/{cell['operation']} complete ns/op (legacy -> compact) | {s['legacy']['ns_per_op']['median']:.1f} -> {s['compact']['ns_per_op']['median']:.1f} | {matrix['candidate_revision']} | pinned generated {cell['source_bytes']} bytes; native C {s['C']['ns_per_op']['median']:.1f} ns/op")
    lines += ["```", ""]
    return "\n".join(lines)


def check(matrix):
    if matrix["runtime_source_sha256"] != source_fingerprint() or matrix["harness_source_sha256"] != source_fingerprint(True):
        raise ValueError("runtime or measurement harness changed since measurement")
    data = (ROOT / "internal/benchfixtures/generated.json").read_bytes()
    if matrix["fixture_manifest_sha256"] != sha(data):
        raise ValueError("generated fixture manifest changed")
    pins = {(r["language"], r["target_bytes"]): r for r in json.loads(data)["entries"]}
    for language in matrix["languages"]:
        for cell in language["cells"]:
            pin = pins[(language["fixture_shape"], SIZES[cell["size"]])]
            if cell["source_bytes"] != pin["source_bytes"] or cell["source_sha256"] != pin["sha256"]:
                raise ValueError("timed source differs from the pinned generator")
            if cell["summary"] != summary(cell):
                raise ValueError("display summary differs from raw samples")
    if REPORT.read_text() != display(matrix):
        raise ValueError("Markdown matrix differs from checked measurements")
    if BENCHSTAT.exists() and f"# matrix-sha256: {sha(MATRIX.read_bytes())}" not in BENCHSTAT.read_text().splitlines():
        raise ValueError("benchstat comparison is stale; rebuild with --benchstat")
    print("compact graduation inputs and display match the measured artifact")


def benchstat(matrix, executable):
    # Treat each seed as one observation; its two bracketed passes are paired,
    # rather than forty independent samples that would overstate confidence.
    with tempfile.TemporaryDirectory(prefix="compact-benchstat-") as folder:
        inputs = []
        for engine in ["legacy", "compact", "C"]:
            lines = []
            for language in matrix["languages"]:
                for cell in language["cells"]:
                    ns = per_seed(cell["timing"][engine])
                    allocated = per_seed(cell["timing"][engine], "bytes_per_op")
                    allocs = per_seed(cell["timing"][engine], "allocs_per_op")
                    for seed in range(1, 21):
                        lines.append(f"BenchmarkCompactGraduation/{language['language']}/{cell['size']}/{cell['operation']} 1 {ns[seed]:.6f} ns/op {allocated[seed]:.6f} B/op {allocs[seed]:.6f} allocs/op")
            path = Path(folder) / engine
            path.write_text("\n".join(lines) + "\n")
            inputs.append(f"{engine}={path}")
        result = subprocess.run([str(executable), *inputs], text=True, capture_output=True, check=True)
    BENCHSTAT.write_text(f"# {BENCHSTAT_VERSION}\n# matrix-sha256: {sha(MATRIX.read_bytes())}\n# Twenty seed averages per cell; two bracketed passes per seed. Shared busy VM; changes under 5% are descriptive.\n\n" + result.stdout + result.stderr)


def main():
    global MATRIX, REPORT, BENCHSTAT
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--evidence-dir", type=Path)
    parser.add_argument("--output-dir", type=Path, required=True, help="receipt directory outside the repository")
    parser.add_argument("--revision", help="revision of the measured benchmark tooling")
    parser.add_argument("--check", action="store_true")
    parser.add_argument("--benchstat", type=Path, help="also write the seed-paired benchstat comparison with this executable")
    args = parser.parse_args()
    output = args.output_dir.resolve()
    if output == ROOT or ROOT in output.parents:
        parser.error("--output-dir must be outside the repository")
    output.mkdir(parents=True, exist_ok=True)
    MATRIX, REPORT, BENCHSTAT = (output / name for name in ["matrix.json", "matrix.md", "benchstat.txt"])
    env = {**os.environ, "GOWORK": "off"}
    if args.check:
        subprocess.run(["go", "run", "./cmd/compactgraduation", "--matrix", str(MATRIX)], cwd=ROOT, env=env, check=True)
        check(json.loads(MATRIX.read_text()))
        return
    if args.evidence_dir is None:
        parser.error("--evidence-dir is required to build a receipt")
    revision = args.revision or subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
    matrix = build(args.evidence_dir, revision)
    MATRIX.write_text(json.dumps(matrix, indent=2) + "\n")
    subprocess.run(["go", "run", "./cmd/compactgraduation", "--matrix", str(MATRIX), "--write"], cwd=ROOT, env=env, check=True)
    matrix = json.loads(MATRIX.read_text())
    REPORT.write_text(display(matrix))
    if args.benchstat:
        benchstat(matrix, args.benchstat)
    check(matrix)


if __name__ == "__main__":
    main()
