#!/usr/bin/env python3
"""Summarize opt-in ceiling benchmarks; preserve noisy and missing cells.

Example: summarize_engine_ceiling.py /tmp/main-go.txt --source main
Outputs CSV. CV is sample standard deviation / mean, in percent. Ratios use
paired seed measurements. Native C bytes/allocations are allocator requests;
Go bytes/allocations are Go heap charges. Neither is peak live memory.
Optional --allocation-log arguments must use the same authenticated source
snapshot as the timing files. Independent count windows expose allocation
ranges and repeatability; one window does not establish stability.
time_C compares complete operations. operation_time_native_parse_C compares
the complete Go operation to native parse alone, excluding C edits/releases.
"""

import argparse
import csv
import json
import re
import statistics
import sys
from collections import defaultdict


def read_samples(paths):
    cells = defaultdict(dict)
    for path in paths:
        seed = None
        with open(path, encoding="utf-8") as file:
            for line in file:
                match = re.match(r"# seed: (\d+);", line)
                if match:
                    seed = int(match[1])
                fields = line.split()
                if len(fields) < 4 or not fields[0].startswith("BenchmarkEngineCeiling/"):
                    continue
                name = re.sub(r"-\d+$", "", fields[0])
                parts = name.split("/")
                if len(parts) != 5 or not fields[1].isdigit():
                    continue
                if seed is None:
                    raise ValueError(f"{path}: benchmark has no explicit seed")
                metrics = {fields[i + 1]: float(fields[i]) for i in range(2, len(fields) - 1, 2)}
                if not {"ns/op", "B/op", "allocs/op"}.issubset(metrics):
                    raise ValueError(f"{path}: incomplete metrics: {name}")
                key = tuple(parts[1:])
                if seed in cells[key]:
                    raise ValueError(f"{path}: duplicate cell/seed: {name}/{seed}")
                cells[key][seed] = metrics
    return cells


def summarize(cells, expected):
    for key in sorted(cells):
        samples = cells[key]
        times = [s["ns/op"] for s in samples.values()]
        cv = 100 * statistics.stdev(times) / statistics.mean(times) if len(times) > 1 else None
        row = dict(zip(("language", "size", "mode", "engine"), key))
        row.update(seeds=len(samples), median_ns=statistics.median(times), cv_percent=cv if cv is not None else "NA",
                   noisy=cv > 5 if cv is not None else "NA", complete=len(samples) == expected)
        parse_times = [s["c-parse-ns/op"] for s in samples.values() if "c-parse-ns/op" in s]
        row["native_parse_median_ns"] = statistics.median(parse_times) if parse_times else "NA"
        row["native_parse_cv_percent"] = (
            100 * statistics.stdev(parse_times) / statistics.mean(parse_times)
            if len(parse_times) > 1 else "NA")
        for metric, label in (("B/op", "bytes"), ("allocs/op", "allocs")):
            values = [s[metric] for s in samples.values()]
            row[label] = statistics.median(values)
            row[label + "_min"] = min(values)
            row[label + "_max"] = max(values)
        reference = cells.get(key[:3] + ("C",), {})
        paired = sorted(set(samples) & set(reference))
        for metric, label in (("ns/op", "time_C"), ("B/op", "bytes_C"), ("allocs/op", "allocs_C")):
            ratios = [samples[s][metric] / reference[s][metric] for s in paired if reference[s][metric]]
            row[label] = statistics.median(ratios) if ratios else "NA"
        row["paired_seeds"] = len(paired)
        parse_ratios = [samples[s]["ns/op"] / reference[s]["c-parse-ns/op"]
                        for s in paired if reference[s].get("c-parse-ns/op")]
        row["operation_time_native_parse_C"] = statistics.median(parse_ratios) if parse_ratios else "NA"
        yield row


def apply_controlled_allocations(rows, paths):
    """Join repeatable allocation probes without hiding unstable windows."""
    counts = defaultdict(list)
    for path in dict.fromkeys(paths):
        with open(path, encoding="utf-8") as file:
            for line in file:
                if line.startswith("CEILING_NATIVE_ALLOCATION "):
                    sample = json.loads(line.split(" ", 1)[1])
                    values = sample["bytes"], sample["allocs"]
                elif line.startswith("CEILING_ALLOCATION "):
                    sample = json.loads(line.split(" ", 1)[1])
                    if sample["gc_during_count"] != 0:
                        raise ValueError(f"{path}: GC ran during controlled allocation count")
                    values = sample["bytes_per_op"], sample["allocs_per_op"]
                else:
                    continue
                key = tuple(sample[k] for k in ("language", "size", "mode", "engine"))
                counts[key].append(values)
    for row in rows:
        key = tuple(row[k] for k in ("language", "size", "mode", "engine"))
        samples = counts.get(key, [])
        row["allocation_windows"] = len(samples)
        # One window cannot establish repeatability.
        row["allocation_stable"] = len(set(samples)) == 1 if len(samples) > 1 else "NA"
        for column, index in (("bytes", 0), ("allocs", 1)):
            for suffix in ("", "_min", "_max"):
                row["timing_" + column + suffix] = row[column + suffix]
            if samples:
                values = [sample[index] for sample in samples]
                row[column] = statistics.median(values)
                row[column + "_min"] = min(values)
                row[column + "_max"] = max(values)
    reference = {(r["language"], r["size"], r["mode"]): r for r in rows if r["engine"] == "C"}
    for row in rows:
        ref = reference.get((row["language"], row["size"], row["mode"]))
        for column in ("bytes", "allocs"):
            row[column + "_C"] = row[column] / ref[column] if ref and ref[column] else "NA"
    return rows


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("paths", nargs="+")
    parser.add_argument("--source", required=True)
    parser.add_argument("--expected-seeds", type=int, default=20)
    parser.add_argument("--allocation-log", action="append", default=[],
                        help="native or Go allocation log; repeat to include independent count windows")
    args = parser.parse_args()
    rows = list(summarize(read_samples(args.paths), args.expected_seeds))
    if args.allocation_log:
        apply_controlled_allocations(rows, args.allocation_log)
    if not rows:
        parser.error("no ceiling benchmark samples")
    writer = csv.DictWriter(sys.stdout, fieldnames=["source", *rows[0]])
    writer.writeheader()
    for row in rows:
        writer.writerow({"source": args.source, **row})
    if any(not row["complete"] for row in rows):
        print("incomplete cells remain visible in CSV", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
