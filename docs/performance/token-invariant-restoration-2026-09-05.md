# Authenticated reuse performance against v0.52.0

Date: 2026-09-05.
Candidate: `da1150c6d6a2d581ce31f44ba4c5b8241ec431ae`.
Release: `2295871057f860598a006d6068588a6303fefb02`.

The candidate makes the generated Go single-byte edit benchmark **16.65 times faster** than v0.52.0.
Full parsing regresses **1.68 percent** in the same campaign.
Record that tradeoff when evaluating issue #1087 and pull request #1093.
This evidence establishes no C speed ratio, universal language improvement, or compact parser graduation.

## Paired results

These medians use twenty samples per commit.
Read the [raw release output](token-invariant-restoration-2026-09-05/release-trio.txt),
[raw candidate output](token-invariant-restoration-2026-09-05/candidate-trio.txt),
and [benchstat comparison](token-invariant-restoration-2026-09-05/benchstat.txt).

| Benchmark | Release ns/op | Candidate ns/op | Change | p-value |
| --- | ---: | ---: | ---: | ---: |
| Single-byte edit | 3,033,171 | 182,122 | -94.00% | <0.001 |
| Full parse | 16,395,747.5 | 16,671,558.5 | +1.68% | 0.021 |
| No edit | 3.149 | 3.084 | -2.06% | 0.002 |

| Benchmark | Release B/op | Candidate B/op | Release allocs/op | Candidate allocs/op |
| --- | ---: | ---: | ---: | ---: |
| Single-byte edit | 188,801 | 123 | 95 | 3 |
| Full parse | 297,360 | 300,144 | 42 | 42 |
| No edit | 0 | 0 | 0 | 0 |

Single-byte edits reduce bytes by 99.93 percent and allocations by 96.84 percent.
The full-parse byte difference is not statistically significant, with p=0.140.
The no-edit delta is only 0.065 nanoseconds; it is not a material project gain.
Zero new parser nodes does not mean zero heap allocations.
The candidate still allocates 123 bytes and three objects per edited operation.

## Workload validation

The unchanged trio uses 500 generated functions and 19,294 bytes.
It changes the numeric literal at byte 35.
This control does not represent the complete human-authored Go corpus.
The [source identity](token-invariant-restoration-2026-09-05/source.json) records its hash and both exact commits.

Both commits passed the frozen Go fixtures and longest-match fallback controls before timing.
The candidate also passed integer and float reuse checks on both parser routes.
A temporary test-only overlay checked the exact generated workload before timing.
Each initial parse used one default compact route without fallback.
All five edited trees matched fresh deep-tree digests; no-edit operations reused the same root.

The [candidate preflight](token-invariant-restoration-2026-09-05/probe/candidate-preflight.json) reports, per edit:

- One authenticated dependency check.
- Zero reparse time and zero new nodes.
- Reuse of the same root and all 19,294 bytes.

The release reparsed and allocated 1,026 nodes per edit.
The diagnostic inspected trees between edits, so its nanoseconds are not timing evidence.
Separate [untouched benchmark telemetry](token-invariant-restoration-2026-09-05/uninspected-telemetry-summary.json) confirmed the candidate path without those inspections.
The timed run used neither diagnostic statistics nor the overlay.

## Memory observations

The large-file probe used the frozen `grammargen_lr` fixture with 235,626 bytes.
It explicitly pins the legacy production parser.
Process peak resident set size (RSS) includes admission, validation, warm-up, and one full parse.
One kibibyte (KiB) equals 1,024 bytes.

| RSS observation | Release, KiB | Candidate, KiB |
| --- | ---: | ---: |
| Pair 1 | 169,088 | 167,668 |
| Pair 2 | 168,608 | 168,436 |
| Pair 3 | 150,112 | 169,300 |

All six probes completed without failure.
These observations establish neither long-run retention, statistical memory improvement, nor compact-route memory behavior.
The [RSS summary](token-invariant-restoration-2026-09-05/rss-summary.json) records the scope and observations.

## Protocol and reproduction

The repo benchmark script ran twenty alternating paired seeds, numbered 1 through 20, under the shared lock.
Each process used:

- A 750 millisecond duration and count=1.
- `GOMAXPROCS=1`, `-benchmem`, and the `gts_parsercorephase0` tag.
- The three required Go benchmark names and `GOT_BENCH_FUNC_COUNT=500`.
- No diagnostic statistics or parser overrides.

Docker used Go 1.25.14, one CPU pinned to CPU 2, four GiB of memory, and `GOMEMLIMIT=3GiB`.
The shared host has an Intel Core Ultra 9 285 processor.
All five Docker stages exited successfully without memory failure or timeout.
Both source checkouts remained clean, and all tracked file hashes matched after the run.
No samples were dropped.

Read the adjacent `run-*.sh` files for exact commands, mounts, and limits.
Update the recorded paths in `run-docker.sh` to clean worktrees and a fresh artifact directory.
The paired script rejects existing output paths.
Rebuild binaries with `run-controls.sh`; binary hashes are recorded, but binaries are excluded.
Regenerate the comparisons with `benchstat release-trio.txt candidate-trio.txt`.
Run `python3 summarize.py` to validate all sixty rows per commit and regenerate exact medians.
Verify the evidence directory with `sha256sum -c SHA256SUMS`.
The timed campaign ran from 14:22:05 through 14:26:26 UTC.
Shared host scheduling remains a limitation of this local measurement.
