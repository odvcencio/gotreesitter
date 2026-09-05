# v0.52.0 release performance

Date: 2026-09-05.

The temporary correctness mitigation makes the measured single-byte edit much slower.
Its median rises from 1.706 microseconds to 3,350.460 microseconds, approximately 1,964 times the baseline.
The increase is 196,292.73 percent, with `p<0.001` and twenty samples per revision.
The owner approved disabling the unsafe shortcut before these measurements.
Issue [#1087](https://github.com/odvcencio/gotreesitter/issues/1087) remains open until complete lexical dependency proofs permit restoration.

Ordinary incremental reuse remains enabled. The no-edit route also remains enabled.
These results do not establish compact parser graduation.

## Paired results

The table reports medians. Benchmark names omit the common `Benchmark` prefix.

| Benchmark | Baseline time | Release time | Timing result |
| --- | ---: | ---: | --- |
| `GoParseIncrementalSingleByteEditDFA` | 1.706 us | 3,350.460 us | +196,292.73%, p<0.001 |
| `GoParseFullDFA` | 19.79 ms | 19.52 ms | No significant change, p=0.883 |
| `GoParseIncrementalNoEditDFA` | 3.576 ns | 3.619 ns | No significant change, p=1.000 |

| Benchmark | Baseline bytes per operation | Release bytes per operation | Baseline allocations | Release allocations |
| --- | ---: | ---: | ---: | ---: |
| `GoParseIncrementalSingleByteEditDFA` | 0 | 184.4 KiB | 0 | 95 |
| `GoParseFullDFA` | 556.1 KiB | 317.5 KiB | 15,549 | 43 |
| `GoParseIncrementalNoEditDFA` | 0 | 0 | 0 | 0 |

Full parsing reduces allocated bytes by 42.90 percent and allocation count by 99.72 percent.
Both reductions have `p<0.001`. The candidate includes both optimizations from pull request #1090.
This comparison measures their combined effect with the shortcut mitigation, not the mitigation alone.
The lexical error-leaf correction remains deferred and is not part of this release scope.

## Sources and method

- Baseline commit: `7db6e20ad88ac7f79228979899d5c55578818f49`.
- Candidate commit: `c85abaf6e21b94dc81704068e857389368927196`.
- Benchmark driver commit: `4e8d780c0dfecfff2b14850bc750e22b8ab15fa0`.
- Both source checkouts were clean, as recorded in `source.json`.
- The driver ran twenty alternating same-seed pairs, with seeds 1 through 20.
- Each process used `GOMAXPROCS=1`, `-count=1`, `-benchtime=750ms`, and allocation reporting.
- Docker limited the run to one CPU, pinned to logical CPU 2, and 4 GiB of memory.
- The Go memory limit was 3 GiB. The build tag was `gts_parsercorephase0`.
- The shared host lock excluded competing benchmark runs.

The host used Windows Subsystem for Linux (WSL). Shared-host scheduling and development activity limit timing precision.
The paired protocol reduces execution-order bias but does not remove that limitation.
No C implementation was timed. Do not interpret these results as a Go/C comparison.

Both raw files finish with twenty completed runs and `status: complete`.
Their initial `status: incomplete` markers remain intact as part of the publication protocol.
The mounted snapshots lack Git metadata inside Docker; the driver reports those identities as unavailable.
The separate source manifest records the supplied identities. The driver does not authenticate source snapshots itself.

The container completed successfully without an out-of-memory kill or wall timeout.

## Bounded memory observation

A separate process measured maximum resident set size at **150,940 KiB**, with zero swaps and exit status zero.
The Docker run also completed without an out-of-memory kill or wall timeout.
The probe used the canonical `grammargen_lr` fixture, exact preflight, and one benchmark iteration.
It reported 109,769 nodes and 49,912 tokens.
Compilation occurred before `/usr/bin/time` started and is excluded from the memory measurement.

This single observation includes process setup and preflight. It is not a statistical memory comparison.
Do not use its elapsed time or single-iteration latency as comparative performance evidence.
The production Go sources are unchanged between measured commit `c85abaf6e21b94dc81704068e857389368927196`
and validation commit `7bc3a8a4e0a93b9bc82f90436f5f051df459e261`.
The later Go changes affect tests, not runtime behavior.

## Evidence

- [Baseline samples](release-v0.52.0-2026-09-05/release-base.txt).
- [Release samples](release-v0.52.0-2026-09-05/release-head.txt).
- [Benchstat comparison](release-v0.52.0-2026-09-05/benchstat.txt).
- [Source and environment manifest](release-v0.52.0-2026-09-05/source.json).
- [Paired command](release-v0.52.0-2026-09-05/run-release-paired.sh).
- [Container completion metadata](release-v0.52.0-2026-09-05/metadata.txt).
- [Maximum resident memory output](release-v0.52.0-2026-09-05/release-large-rss.txt).
- [Memory probe command](release-v0.52.0-2026-09-05/run-release-rss.sh).
- [Memory probe result](release-v0.52.0-2026-09-05/rss-container.log).
- [Memory probe completion metadata](release-v0.52.0-2026-09-05/rss-metadata.txt).

The command expects `/baseline`, `/workspace`, `/paired-driver.sh`, `/evidence`, and `/bench.lock` mounts.
Use the recorded commits and driver when reproducing the comparison.
