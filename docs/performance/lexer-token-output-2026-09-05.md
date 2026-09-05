# Shared lexer token output performance

Date: 2026-09-05.
Runtime commit: `236ca848239679221715b386115fc2910f6cf4e1`.
Baseline: `da1150c6d6a2d581ce31f44ba4c5b8241ec431ae`.

The shared lexer change reduces the measured generated Go edit time by **14.20 percent**, with p<0.001.
Retain this change as an edit optimization.
Neither full-parse workload shows a statistically significant timing change.
These results establish no universal language improvement, C speed ratio, or compact parser graduation.

## Measured results

Each row uses twenty samples per source.
The human-authored fixture uses a separate paired campaign and the legacy parser.
Keep its results separate from the generated benchmark trio.

| Workload | Baseline | Candidate | Timing result |
| --- | ---: | ---: | --- |
| Generated Go single-byte edit | 210.8305 microseconds | 180.8945 microseconds | -14.20%, p<0.001 |
| Generated Go full parse | 19.931740 milliseconds | 19.348169 milliseconds | Not significant, p=0.989 |
| Generated Go no edit | 3.776 nanoseconds | 3.9185 nanoseconds | Not significant, p=0.534 |
| Human-authored Go full parse | 246.4183135 milliseconds | 240.1643955 milliseconds | Not significant, p=0.265 |

| Workload | Baseline bytes/op | Candidate bytes/op | Baseline allocations/op | Candidate allocations/op |
| --- | ---: | ---: | ---: | ---: |
| Single-byte edit | 124 | 122 | 3 | 3 |
| Generated full parse | 325,124.5 | 321,309 | 43 | 43 |
| No edit | 0 | 0 | 0 | 0 |
| Human-authored full parse | 9,821,702 | 9,821,702 | 169 | 169 |

The two-byte edit difference has p=0.003; it does not establish a smaller parser allocation object.
Other byte and allocation-count differences are not significant.
Read the [trio comparison](lexer-token-output-2026-09-05/benchstat.txt) and [human-authored comparison](lexer-token-output-2026-09-05/macro-benchstat.txt).
The adjacent raw outputs preserve all 160 timing rows.

## Change and correctness

Contiguous scans now write into caller-owned token storage.
Ordinary tokenization, error probes, and authenticated edit proofs use that path.
Value-returning wrappers remain available. The included-range scanner retains its existing implementation.
The patch adds no persistent cache or grammar-specific branch.
It excludes both earlier inconclusive experiments.

Independent assembly inspection confirms five fewer full-token transfers on successful contiguous scans, removing 440 copied bytes per scan.
The compiler also adds conditional write barriers and increases the contiguous scanner frame.
Read the [compiler record](lexer-token-output-2026-09-05/compiler-result.md) for the exact tradeoffs.
Its instruction counts do not establish speed by themselves.

Focused Docker controls passed: 57 baseline tests and 60 candidate tests.
New controls cover poisoned output, output reuse, failed probes, Unicode frontiers, and included-range gaps.
Separate C-oracle containers validated Go, JavaScript Object Notation (JSON), and JavaScript.
Go repair and reuse checks passed on both parser routes.
JSON and JavaScript passed fresh and incremental structural parity without skips.
These cases do not establish universal language parity.

The candidate preflight uses the exact generated workload: 500 functions, 19,294 bytes, and an edit at byte 35.
All five edits matched fresh deep trees and reused the root without reparsing or allocating parser nodes.
Each edit performed one authenticated dependency check.
The initial parse used the default compact route without fallback.
The preflight inspects trees between edits; the timed lifecycle does not.
This campaign does not repeat the baseline preflight.

## Memory observations

The frozen human-authored fixture contains 235,626 bytes and uses the legacy parser.
Process peak resident set size (RSS) includes admission, validation, warm-up, and one full parse.
One kibibyte (KiB) equals 1,024 bytes.

| Observation | Baseline RSS, KiB | Candidate RSS, KiB |
| --- | ---: | ---: |
| Pair 1 | 166,976 | 170,652 |
| Pair 2 | 149,640 | 168,448 |
| Pair 3 | 168,512 | 168,800 |

All six probes completed without failure.
The highest observations were 168,512 KiB and 170,652 KiB.
These observations establish neither memory improvement nor long-run retention.
They exercise the shared lexer but do not isolate proof stack space or compact parser memory.

## Reproduction and next work

Both timing campaigns used the repository randomized benchmark driver and shared lock.
Each ran twenty alternating pairs, seeds 1 through 20, without discarded samples or significance-seeking reruns.
Each process used 750 milliseconds, count=1, GOMAXPROCS=1, and allocation reporting.
Timing excluded the preflight overlay and diagnostic statistics.

Docker used Go 1.25.14, one processor pinned to logical processor 2, four GiB of memory, and GOMEMLIMIT=3GiB.
The shared Windows Subsystem for Linux host has an Intel Core Ultra 9 285 processor.
Shared-host scheduling limits timing precision.
All eight Docker stages completed without a memory failure or timeout.
Every tracked file hash matched afterward: 2,955 baseline files and 2,956 candidate files.
The runtime commit reproduces the measured patch exactly.

Read the [recorded commands](lexer-token-output-2026-09-05/commands.md), source identities, and compact stage receipts for the exact settings.
Use a fresh artifact directory and update worktree paths before reproduction.
Run `benchstat` on each corresponding pair of raw outputs.
Verify the published evidence with `sha256sum -c SHA256SUMS`.
Compiled binaries and the C reference build cache are excluded.
The [archive record](lexer-token-output-2026-09-05/ARCHIVE.md) identifies the full durable evidence, including source inventories, preflight source, and disassembly.
The compact bundle omits expanded samples and duplicate logs.

The human-authored workload reports 47,976 of 49,912 tokens on the path for multiple parser stacks, or 96.12 percent.
Both sources report the same work counters across every sample.
Profile that parsing and tree-construction work next, before choosing another shared optimization.
The counters establish work volume, not elapsed-time attribution.
A matched C timing campaign remains necessary to measure progress toward the two-times-C target.

## Integration on merged main

The publication branch applies this runtime change to main `40e93f74`, which includes the compact sibling-adoption correction.
All three changed Go files remain byte-identical to the measured source.
Separate Docker gates passed the Go lexer and C checks, then the TypeScript proof, adoption, and C checks.
The TypeScript gate includes numeric reuse, syntax rejection, and clean sibling adoption without skips.
Read the [integration receipt](lexer-token-output-2026-09-05/integration.json) for tested identities and counts.
These integration checks establish correctness on that main revision; they do not repeat the original timing campaign.
