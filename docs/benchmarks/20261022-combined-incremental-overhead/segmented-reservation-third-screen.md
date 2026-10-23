# Eight-MiB pieces with shared metadata: rejected

The third private reservation layout combines separately allocated 8 MiB node pieces with separately lazy, shared lookahead and alias metadata. It preserves total node capacity, first-overflow growth, budgets, public Node layout, retention ceilings and the primary-reservation discard policy. The baseline is PR #1509 runtime `dba13461f0df4dc8eaf626a80d143a7cf0e8e5ab`. No runtime source is promoted.

The corrected local preflight passes twelve warmed 1 MiB Go C comparisons (three rounds on each route, both revisions), four Go/YAML R4 sessions / 288 edits with zero no-edit allocations, focused reserve/metadata/retention tests, twenty memory contracts, and identical Go/YAML ledger exports. The first harness failed on a missing overlay control; that failed run remains recorded. A full language fleet is not run for this rejected source.

Twenty matched Mac Docker Linux ARM64 pairs use Go 1.25.14, GOMAXPROCS=1, seeds 163001–163020, precompiled parser binaries, direct-child rusage, exactly two measured iterations and no manual GC. The fixture is 1,048,591 bytes, SHA-256 `76c70b62127085d3c791ba98a24cba9f22bd343e40afb43522bbb5041cede524`. Sources stay unchanged and every measured parse passes completion checks. Warm counters remain 993,391 nodes, 425,742 tokens and two stacks; the stock benchmark does not separately report each measured iteration's work.

| Metric | Held runtime | Prototype |
| --- | ---: | ---: |
| Median peak RSS | 455,712,768 B | 367,069,184 B |
| Maximum peak RSS | 461,275,136 B | 449,273,856 B |
| Processes over 419,436,400-byte ceiling | 12/20 | 2/20 |
| Median allocs/op | 116 | 134 |

The absolute RSS gate still fails, and allocations rise **15.52%**, above the median ratchet. The lower median cannot waive either failure. No randomized throughput comparison follows this failed screen; fixed-iteration process timings are not throughput evidence. The PR's existing RSS hold remains.

The [archive](segmented-reservation-third-screen.tar.gz) and [member hashes / qualification](segmented-reservation-third-screen.json) retain the final source, controls, failed and corrected harnesses, source manifests, all eighty RSS log/JSON files, and terminal audits. Earlier separate-piece and grouped-piece experiments remain in the [preceding receipt](segmented-reservation-followup.md). This result stops promotion of this allocation-only combination; it does not qualify compact graduation or solve Cogno's small-document verifier blocker.
