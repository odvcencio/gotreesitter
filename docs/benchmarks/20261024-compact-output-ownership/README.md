# Compact result ownership

This bucket removes the producing parser graph from a compact result's lifetime. It qualifies the internal read-only output route introduced by the preceding single-version/output bucket. It does not enable compact output through the public `*Node` API or graduate a language.

Baseline: `377beec2d9035ad3d9b91c58162273dffedb9ddd`, on draft PR #1512. The [source manifest](qualified-source.json) pins the candidate files. These are branch-relative results, not main-based net qualification or C speed ratios. **All merges to main remain held.**

## Change

`SelectedStore` previously retained `*Core` solely to recycle three pointer-free backing arrays. Consequently, keeping a result alive kept the parse graph, table adapter, and parser scratch reachable. A small independent pool now owns recycling. It has no reference to the core or grammar adapter. Retiring core storage detaches the pool generation, so late result releases cannot refill the retired core's current pool.

Pool capacity accounting now includes the provenance array and synchronizes with concurrent sibling releases. Publication overwrites every used output slot, so release no longer clears the pointer-free arrays. A poisoned-buffer regression checks that old flags, parent IDs, fields, spans, and child IDs cannot leak into a new tree. No action, reduction, replay, recovery, or reuse rule changes.

## Retained memory

Twenty matched process pairs per language keep one compact result alive after discarding its parser, force collection, validate the complete result against a public tree, then release it and collect again. Each process warms grammar metadata first; input and grammar remain alive throughout. Reported retained heap is `HeapAlloc(result alive) − HeapAlloc(before parse)`, excluding warmed grammar/input memory. Both roles use identical controls and alternate first/second order.

| Generated source | Nodes, before → after | Result backing, unchanged | Retained heap before | Retained heap after | Change |
| --- | ---: | ---: | ---: | ---: | ---: |
| Go, 1,048,591 bytes | 593,457 → 593,457 | 46,137,344 B | 152,211,712 B | 46,137,520 B | −69.69% |
| YAML, 1,049,572 bytes | 121,045 → 121,045 | 5,767,168 B | 27,673,720 B | 5,767,392 B | −79.16% |

Every pair has identical source hashes, full output digests, node counts, backing capacities, and work counters. All 80 processes return within 1 KiB of pre-parse heap usage after result release. A separate regression keeps the result alive while detecting collection of its producing core: it fails on the baseline and passes on the candidate.

Median peak RSS for the complete validation/retention workload is Go **341,712 → 335,198 KiB (−1.91%)** and YAML **96,964 → 96,916 KiB (−0.05%)**. This includes warm-up and simultaneous public/compact tree validation; it is not a single-parse RSS comparison. The retained-heap reduction must not be presented as a corresponding peak-RSS or throughput reduction.

## Throughput

Twenty randomized paired seeds per suite, one process per seed, `GOMAXPROCS=1`, `-count=1`, `-benchtime=750ms`, and `-benchmem`, using the repository runner. All 520 rows have matched workload order and alternating roles. **Every timing difference is inconclusive; this bucket makes no throughput-win claim.**

| Workload | Before | After | Median time change | p |
| --- | ---: | ---: | ---: | ---: |
| Go query source, parse | 8.863 ms | 8.982 ms | +1.34% | 0.445 |
| Go LR source, parse | 94.083 ms | 94.768 ms | +0.73% | 0.127 |
| Go query source, full traversal | 8.918 ms | 8.945 ms | +0.30% | 0.758 |
| Go LR source, full traversal | 94.541 ms | 94.723 ms | +0.19% | 0.799 |
| YAML R4, parse | 180.418 µs | 178.620 µs | -1.00% | 0.883 |
| YAML EOF 8 KiB, parse | 710.907 µs | 714.021 µs | +0.44% | 0.820 |
| YAML R4, full traversal | 181.643 µs | 183.825 µs | +1.20% | 0.327 |
| YAML EOF 8 KiB, full traversal | 721.457 µs | 719.016 µs | -0.34% | 0.862 |
| Go 1 MiB, full traversal | 555.615 ms | 551.394 ms | -0.76% | 0.461 |
| Go 1 MiB, parse | 544.697 ms | 543.623 ms | -0.20% | 0.445 |
| Public Go edit control | 88.036 µs | 88.421 µs | +0.44% | 0.183 |
| Public Go full parse control | 4.025 ms | 4.040 ms | +0.38% | 0.779 |
| Public Go no-edit control | 3.083 ns | 3.082 ns | -0.03% | 0.952 |

All allocation-count medians are unchanged; no-edit remains **0 B/op, 0 allocs/op**. The public full-parse control's B/op median rises 0.043% (p=.751); the other allocation-byte changes round to zero. Every timing/allocation median ratchet passes. The independent pool adds one small allocation when a core first requests selected output; the timing rows above measure warmed parsing. The complete [performance analysis](performance-analysis.json) and [Go](benchstat-go.txt), [YAML](benchstat-yaml.txt), [large Go](benchstat-large.txt), and [public control](benchstat-primary.txt) benchstat tables retain all directional changes.

## Correctness and counters

- All 206 grammars: 412 invariant sessions, 29,664 edit steps, and allocation-free no-edit reparses.
- 1,236 standard locked-C fixture checks, with no skips; focused YAML C checks also pass.
- All 824 complete-operation work rows are exactly equal; all 206 selected work/reuse/route ledgers are exactly equal. No counter exception or baseline reset.
- Core, selected one-pass builder, shadow, 386, focused race, public-output race, and 20 memory-contract tests pass.
- The exact latest Cognociente 137 KiB witness still has 12 independent incremental/fresh agreements and eight clean C agreements. All four malformed-delete cases retain the same known C shape difference at `/source_file/function_declaration[2]/block[4]/ERROR[1]`: Go has one child and C has two. These remain failures, not approved parity exceptions. The small-source fresh verifier stays enabled.

Representative selected counters (all rows are unchanged):

| R4 route / operation | Tokens before → after | New nodes before → after | Reused bytes before → after | Splices before → after |
| --- | ---: | ---: | ---: | ---: |
| Go candidate / full | 589 → 589 | 893 → 893 | 0 → 0 | 0 → 0 |
| Go candidate / first edit, existing legacy fallback | 374 → 374 | 1,553 → 1,553 | 1,784 → 1,784 | 266 → 266 |
| YAML candidate / full | 107 → 107 | 232 → 232 | 0 → 0 | 0 → 0 |
| YAML candidate / first edit, existing fallback | 107 → 107 | 232 → 232 | 0 → 0 | 0 → 0 |

The first candidate check failed to compile because one test constructor still used the former owner type. It was corrected before qualification; the failed log is retained. The expected baseline lifetime-regression failure is also retained.

## Reproduce and limits

The [receipt archive](receipts.tar.gz) contains frozen overlays, controls, input, command manifests, all raw results, and a SHA-256 manifest. Extract it into `harness_out/docker/compact-output-ownership-v1`. The command JSON files record every Docker mount, environment variable, CPU/memory limit, and the pinned Go 1.26.6 image. `qualification.sh` serializes grammar checks and counters; `measure.sh` refuses to run before that gate completes. `verify-correctness.py` and `analyze.py` validate completeness, matched order, source identity, no-edit allocations, and ratchets. Generated executables are excluded from the archive and removed after qualification.

The output still uses the existing array-of-records store. Public lazy node views, persistent replay/checkpoint/reuse metadata, edit integration, and the structure-of-arrays output layout remain unfinished. A live result may retain one bounded recycling pool; this change removes its reference to the producing core. No Windows or ARM64 run, route change, allowlist change, or compact graduation is claimed.
