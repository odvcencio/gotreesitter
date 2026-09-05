from pathlib import Path
import json, re, statistics
root = Path(__file__).resolve().parent
names = ["BenchmarkGoParseFullDFA", "BenchmarkGoParseIncrementalSingleByteEditDFA", "BenchmarkGoParseIncrementalNoEditDFA"]
data = {}
for role in ["baseline", "candidate"]:
    raw = (root / (role + "-trio.txt")).read_text()
    assert raw.rstrip().endswith("# status: complete"), role
    assert "# completed runs: 20" in raw, role
    assert ("# dirty: " + ("true" if role == "candidate" else "false")) in raw and ("# dirty at end: " + ("true" if role == "candidate" else "false")) in raw, role
    samples = []
    seed = position = None
    for line in raw.splitlines():
        if match := re.match(r"# seed: (\d+); position: (\d+)$", line):
            seed, position = map(int, match.groups())
            expected = 1 if (seed % 2 == 1) == (role == "baseline") else 2
            assert position == expected, (role, seed, position)
        if not line.startswith("Benchmark"):
            continue
        tokens = line.split()
        name = re.sub(r"-\d+$", "", tokens[0])
        assert name in names, name
        row = {"seed": seed, "position": position, "benchmark": name, "iterations": int(tokens[1])}
        for index in range(2, len(tokens), 2):
            row[tokens[index + 1]] = float(tokens[index])
        assert all(metric in row for metric in ["ns/op", "B/op", "allocs/op"])
        samples.append(row)
    assert len(samples) == 60, (role, len(samples))
    for name in names:
        assert sorted(row["seed"] for row in samples if row["benchmark"] == name) == list(range(1, 21))
    data[role] = samples
summary = {"protocol": "20 alternating paired seeds; 750ms; count1 per process; GOMAXPROCS1", "metric_summary": {}}
for name in names:
    rows = {}
    for metric in ["ns/op", "B/op", "allocs/op"]:
        values = {role: [r[metric] for r in data[role] if r["benchmark"] == name] for role in data}
        medians = {role: statistics.median(v) for role,v in values.items()}
        delta = (medians["candidate"] / medians["baseline"] - 1) * 100 if medians["baseline"] else None
        rows[metric] = {"medians": medians, "median_change_percent": delta, "ranges": {role: [min(v), max(v)] for role,v in values.items()}}
    summary["metric_summary"][name] = rows
(root / "trio-samples.json").write_text(json.dumps(data, indent=2) + "\n")
(root / "trio-summary.json").write_text(json.dumps(summary, indent=2) + "\n")
print(json.dumps(summary, indent=2))
