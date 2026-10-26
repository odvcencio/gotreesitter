# Ordinary YAML ownership and compact storage experiment

**Unpromoted: correctness and compact memory checks pass, but throughput and allocation ratchets fail.** This receipt preserves a 77-Go-file prototype against main `c5695bc4c3a155cc1af490e18c6a0684c8317945`. The prototype remains in [prototype.patch.gz](prototype.patch.gz); it is not installed in this PR's runtime. Apply the decompressed patch to that exact main commit to reproduce it. `git apply --check` passes there.

The prototype admits ordinary YAML using version-owned lexer state and a single lexer prefix. It combines 16-byte common graph links with full-width overflow, 12-byte checkpoint records with authenticated receipt reconstruction, bounded observed-density reservation, and selected-tree publication that omits an unused source map. It excludes held #1509 incremental changes and preserves main's hidden-newline/NUL fix. No allowlist or global-default change is included.

## Correctness and deterministic work

Both routes complete all 206-language gates: **412 invariant sessions, 29,664 edit steps and 1,236 locked-C fixture checks**. No-edit reparses allocate zero. All 206 counter exports are unchanged except the expected ordinary-YAML route change; the other 205 exports are byte-identical. Fifty focused work/reuse comparisons pass. Core, shadow, focused race, memory-budget and actual 386 tests also pass. Node remains 104 bytes on amd64 and 80 bytes on 386. The source set grows from 3,542 to 3,557 Go files, with no new root production file.

| YAML witness / work | Main | Prototype |
| --- | ---: | ---: |
| Ordinary R4 full / edit nodes | 288 / 288 | 232 / 232 |
| Ordinary EOF 8 KiB nodes | 1,156 | 925 |
| Ordinary R4 tokens including declined work | 124 | 107 |
| Ordinary EOF tokens including declined work | 441 | 423 |
| Owned R4 nodes | 484 | 232 |
| Owned EOF nodes | 1,936 | 925 |

Default R4 full/edit nodes stay 288/480. Selected token and reuse counters stay unchanged. Ordinary maximum stack count stays two. No counter exception is requested.

## Memory

Matched cold-process tests use twenty alternating pairs per case, seeds 173001–173020, the same 1 MiB fixtures and no forced GC. Candidate-to-main median RSS ratios are Go **0.9440**, ordinary YAML **1.0410**, and owned YAML **0.8170**. All cold per-source ceilings pass.

The separate warm compact-Go test passes all forty process ceilings: median RSS **391,563,264 → 362,543,104 bytes (−7.41%)**, unchanged 55,328,236 B/op and 51 allocations. Its fixed-iteration samples establish memory behavior only.

The forced-legacy warm Go test retains an existing failure: both main and prototype exceed the 419,436,400-byte ceiling in all twenty processes. Medians are **443,488,256 → 447,318,016 bytes**, with unchanged 175,922,160 B/op and 96 allocations. This failure is neither fixed nor waived by the compact experiment.

## Randomized timing: failed

Twenty alternating shuffled pairs per suite use Go 1.26.6 linux/amd64, GOMAXPROCS=1, count=1, 750 ms and benchmem. Seeds are 169001–169020 for YAML, 170001–170020 for primary Go and 171001–171020 for compact Go. All 300 benchmark rows per role are retained and their orders independently checked. Compare [YAML](benchstat-yaml.txt), [primary Go](benchstat-go.txt), and [compact Go](benchstat-compact-go.txt).

| Failed median ratchet | Main | Prototype | Change |
| --- | ---: | ---: | ---: |
| Ordinary YAML R4 time | 267.3 µs | 310.7 µs | +16.21%, p=.015 |
| Ordinary YAML EOF time | 826.0 µs | 1,137.0 µs | +37.65%, p<.001 |
| Ordinary YAML EOF allocations | 34 | 71 | +108.82% |
| Owned YAML R4 B/op | 11,848 | 13,072 | +10.33% |
| Compact Go query-compile time | 16.59 ms | 19.21 ms | +15.84%, p=.461 |

The Go query timing is statistically inconclusive but still misses the median ratchet. Other Go timing cells show no significant improvement. Owned YAML timing also shows no significant change. This experiment does not establish a Go throughput win or satisfy compact graduation.

CPU profiles place ordinary-YAML cost in corridor dispatch, lexer election and publication. Allocation profiles show recurring immutable version-state wrappers and scanner snapshots. Compilation and table-building allocations are setup costs and are not counted as per-parse publication work.

## Cognociente report and limits

The exact [October 21 report](https://github.com/odvcencio/gotreesitter/issues/454#issuecomment-6311551850) supplies the 140,368-byte Go fixture, SHA-256 `5434575bb249338937331c3756e10dcec34237d107f523ea315996fb6de0ef8f`. Both routes and both plain/profiled APIs pass all twelve independent incremental/fresh comparisons. Eight insert/replace cases match C. Four malformed-delete comparisons still fail at `/source_file/function_declaration[2]/block[4]/ERROR[1]`: Go has one child and C has two. The bounded C trace is retained. This known C mismatch remains a correctness blocker.

This 77-file prototype has not been rerun on native Windows or the Mac. The separately approved Mac checkpoint-layout run is documented in #1509 and must not be treated as qualification for this larger source set.

## Reproduction evidence

[qualification.json](qualification.json) records all source hashes, gates, failed cells and archive members. [receipts.tar.gz](receipts.tar.gz) contains 3,552 regular members: frozen controls, raw successful and failed runs, source manifests, the complete patch and profiles. Archive SHA-256: `3a73c12ad7909eb9a991f2ab67f6eaf641c0ad2b291117fda5e2ef819cfede90`; size: 10,703,404 bytes. Every member was extracted and checked against its SHA-256 manifest. Failed or interrupted experiments remain recorded. Source hashes are unchanged across the qualifying runs.
