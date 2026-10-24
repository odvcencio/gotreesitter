# Recovered Mac experiment: memory improvement with a Go timing regression

This experiment compares held incremental runtime `4de229c4b1718989917c6212f92cf74df28cf229` with the same runtime plus compact graph-link storage. It is not a main-only comparison. The Core production bytes match the draft storage implementation; the later test-helper and CI changes do not alter runtime bytes.

The frozen run completed before recovery. Its 20 matched alternating pairs per language use one process per shuffle seed, GOMAXPROCS=1, count=1, 750 ms and benchmem. Go seeds are 160001–160020; YAML seeds are 161001–161020. Required workloads, shuffle order, work metrics and no-edit allocation checks pass. Source and checkout hashes remain unchanged. The [raw archive](mac-performance-experiment.tar.gz) includes all samples, full benchstat output, RSS probes, source hashes and controls.

The smaller links reduce allocation and compact-Go RSS, but the current layout regresses Go throughput. It is not a throughput win or a compact graduation receipt.

| Workload | Before | After | Time change | B/op change | allocs/op before → after |
| --- | ---: | ---: | ---: | ---: | ---: |
| go/fresh_one_mib | 969.019 ms | 1048.288 ms | +8.18% | -14.55% | 267.5 → 276 |
| go/full_dfa | 13.019 ms | 14.263 ms | +9.56% | +0.00% | 37 → 37 |
| go/full_dfa_3500 | 95.196 ms | 100.834 ms | +5.92% | +0.00% | 76 → 76 |
| go/grammargen_lr | 143.599 ms | 149.393 ms | +4.03% | +0.00% | 1709 → 1709 |
| go/no_edit_dfa | 6.070 ns | 6.068 ns | -0.03% | 0.00% | 0 → 0 |
| go/query_compile | 13.768 ms | 14.456 ms | +4.99% | +0.00% | 121 → 121 |
| go/single_byte_edit_dfa | 113.412 us | 113.625 us | +0.19% | +0.00% | 5 → 5 |
| yaml/eof8k_legacy | 677.152 us | 669.725 us | -1.10% | +0.00% | 13 → 13 |
| yaml/eof8k_ordinary | 755.361 us | 748.307 us | -0.93% | -8.96% | 34 → 34 |
| yaml/eof8k_owned | 1.899 ms | 1.936 ms | +1.92% | +0.00% | 191 → 191 |
| yaml/r4_legacy | 182.830 us | 181.718 us | -0.61% | +0.00% | 16 → 16 |
| yaml/r4_ordinary | 242.759 us | 242.399 us | -0.15% | -7.29% | 37 → 37 |
| yaml/r4_owned | 492.788 us | 500.940 us | +1.65% | +0.00% | 72 → 72 |

The time column reports median changes; the archived benchstat reports significance and uncertainty. Go full parse, 3500-function full parse, 1 MiB fresh parse, query input and LR input regress significantly. Standard edit and no-edit cells show no significant time change. Ordinary YAML r4 shows no significant time change; EOF ordinary YAML improves 0.93%. Owned YAML regresses 1.65% and 1.92%. All medians remain inside the existing 10% ratchet.

| 1 MiB RSS case | Median before | Median after | Change |
| --- | ---: | ---: | ---: |
| go | 276.87 MiB | 266.96 MiB | -3.58% |
| yaml_ordinary | 69.38 MiB | 69.81 MiB | +0.62% |
| yaml_owned | 112.40 MiB | 112.13 MiB | -0.24% |

All 126 RSS processes, including initial probes, are retained. No probe exceeds the source-size ceiling; no manual GC runs in the measured process. These are compact/ordinary single-parse probes. The separate warmed default-Go RSS failure holding #1509 remains open.

CPU attribution found that central graph-link publication lost inlining (compiler cost 31 → 91). A private follow-up returns the allocated link ID directly and restores inlining at cost 80; its Core suite passes. Its full correctness and randomized timing qualification is still in progress. This prototype is not included in the draft runtime.

Archive SHA-256: `bb74f80d0f35dd4dbf86717776ec2ea76d6c9663ca9462fc7578a93ef6ecbe03`.
