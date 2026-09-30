# Tree-scoped navigation receipt, 2026-09-30

The parser baseline is `f9828512c`. The first view implementation is
`1be979554926bddf052566f5a5f1bd5edba490cb`; the first slab candidate is
`96c9867d853d36c28c6840be5b3269b9e0db0401`; the final cache is
`3192383b7f9b6751d8d60865a183630bf603bc46`. The measured parser/view
implementations correspond to those revisions. Test-only completion checks and
comment changes were added after some timings; benchmark source metadata is
informational rather than an authenticated full-source snapshot.

The C oracle is runtime 0.27.0,
`6070dbfefd326bd735e5683eb128cc1b57dad0c0`, from migration `3a0e7df5`.
It was used in a separate measurement checkout. The repository runtime pin
and all gate thresholds remain unchanged.

| C grammar | Commit | Loaded artifact SHA-256 |
| --- | --- | --- |
| Go | `2346a3ab1bb3857b48b29d779a1ef9799a248cd7` | `f17d674be6e3e7a0edb521defa31d3174c877b01414fd758a4988673423573f7` |
| C# | `9150f7d56bb47f1a809fa23623f1ba1413e93fa9` | `92c48c8fd45048d9e2923a97828c8a6336d0f7650e4086be4afe080d8d76fb82` |

METRIC: wrong parent edges, Go old/new views | 1771 -> 0 | f9828512c -> 3192383b7 | GeneratedSource(go, 140288), last a + b changed to a - b
METRIC: wrong parent edges, C# old/new views | 0 -> 0 | f9828512c -> 3192383b7 | GeneratedSource(c_sharp, 140288), last a + b changed to a - b
METRIC: no-edit allocations with views | 0 -> 0 | f9828512c -> 3192383b7 | retained Go root view, 100 reparses
METRIC: complete full navigation Go/C time | 2.533x -> 2.405x | 96c9867d8 -> 3192383b7, C 6070dbfe | GeneratedSource(go, 32768), parse + all child/parent edges + release
METRIC: complete edit navigation Go/C time | 3.262x -> 3.351x | 96c9867d8 -> 3192383b7, C 6070dbfe | GeneratedSource(go, 32768), edit + parse + both live graphs + release
METRIC: complete full navigation allocations | 106 -> 74 | 96c9867d8 -> 3192383b7 | GeneratedSource(go, 32768), 20-seed median allocations/op

## Parent and reuse witnesses

The 137 KiB Go source has 140,368 bytes and SHA-256
`5434575bb249338937331c3756e10dcec34237d107f523ea315996fb6de0ef8f`;
edited SHA-256 is
`993ffc4b3efa6ffac2ab508bfaad0ecb404f9b6605d50f78b497d4bfb78a6e20`.
Each tree has 80,971 nodes; 80,934 payloads are shared. Legacy wrong edges
are 1,771 in the old tree and zero in the new tree. Both view graphs have zero
wrong edges, with distinct views over shared payloads.

The 137 KiB C# source has 140,356 bytes and SHA-256
`e4fc17e581a43b830ada353815eec8b12715f13801076d21edb1af411b231f33`;
edited SHA-256 is
`030e7093fad73f03f96967b07b105381ebce1b730a37099feb60cf276012dece`.
Each tree has 63,645 nodes. C# uses the existing
`external_scanner_unsupported` fresh fallback. This witness establishes
navigation correctness, but does not establish C# payload reuse.

The 1 MiB Go witness has 1,048,591 bytes and 593,457 nodes; its source SHA-256
is `76c70b62127085d3c791ba98a24cba9f22bd343e40afb43522bbb5041cede524`.
Its old legacy graph has 12,912 wrong parent edges; both view graphs have zero.
The parser reports the existing `recovery_frontier_unproven` fresh fallback
at this size, with zero shared payloads. The C# witness has 1,048,645 bytes
and 460,245 nodes; its source SHA-256 is
`76d9bcb2b06576f9f85ce7f38acb0c721e46bcfc0ee73c71cc436d97fd1aa853`.
It uses the scanner fallback and has zero wrong parent edges. Both languages
accept the full clean input and match a fresh Go parse after the edit.

## Deterministic counters

Counters were compared before timing. The slab optimization preserves the
first implementation's counts and navigation result.

| 137 KiB witness, one edit | Parser baseline | Final views |
| --- | ---: | ---: |
| Go tokens consumed | 1,785 | 1,785 |
| Go new parser nodes | 3,598 | 3,598 |
| Go reused subtrees | 1,771 | 1,771 |
| Go reused bytes | 136,786 | 136,786 |
| C# tokens consumed | 42,961 | 42,961 |
| C# new parser nodes | 273,567 | 273,567 |
| C# reused subtrees / bytes | 0 / 0 | 0 / 0 |

No counter, digest, census, expectation, or threshold was refreshed.
`Node`, `Tree`, `Parser`, and `ParseRuntime` layout gates stay unchanged.

## Complete-operation performance

The full operation includes parsing, lazy creation of every visited view,
all child/parent edges, and release. An edit includes `Tree.Edit`, incremental
parsing or its existing fresh fallback, navigation of both live tree versions,
and old-tree release. Only the initial retained edit tree is warmed outside
the timed region. Every measured view and C operation has zero wrong parents.
The legacy Go edit benchmark still measures 429 wrong parent edges per operation.

The first implementation allocated a wrapper for every visited occurrence.
The first slab candidate embedded views and improved complete Go full/edit time
by 14.61%/19.60% in its paired campaign, but regressed sparse requests. A
20-seed API microbenchmark measured root-only storage 384 -> 736 B, root time
539.8 -> 649.7 ns, and a 16-node path 2,872 -> 14,448 B and
3.798 -> 8.391 us. That layout is superseded. Its
[before](receipts/tree-views/rejected-sparse-before.txt) and
[after](receipts/tree-views/rejected-sparse-after.txt) samples are retained.

The final cache starts with one record, grows small blocks, then uses larger
slabs. Passing the immutable payload child count removes a redundant record
field, reducing each amd64 occurrence record from 48 to 40 bytes.

The paired comparison uses `scripts/run_randomized_benchmarks.sh`, 20 seeds,
alternating baseline/head order, shuffled top-level benchmarks,
`GOMAXPROCS=1`, `-count=1`, `-benchtime=750ms`, and `-benchmem` on Linux amd64,
Intel Xeon Platinum 8481C. Inputs are the committed 32 KiB generator fixtures.
Native C is measured in the same campaign with its own 750 ms timer, excluding
process startup, grammar loading, and source loading. C bytes count requested
allocation sizes, including realloc requests; allocations count native calls.
They are not Go wrapper allocations or retained memory.

| Complete operation | Prior slab ms/op | Final ms/op | Go/C, prior / final |
| --- | ---: | ---: | ---: |
| Go full | 29.359 | 28.027 | 2.533 / 2.405 |
| Go edit | 35.519 | 35.168 | 3.262 / 3.351 |
| C# full | 73.248 | 66.732 | 5.440 / 5.200 |
| C# edit | 71.231 | 68.209 | 7.240 / 6.992 |

All four timing distributions, native C, and legacy payload timings are
statistically unchanged. Go edit's median Go/C ratio increases 2.71% because
native C's edit median decreases more than Go's; both timing distributions
are unchanged and the ratio stays within the 10% ratchet. These ratios divide
corresponding 20-seed medians. Confidence intervals are wide under shared VM
load; the receipt does not establish a new release speed target.

| Complete operation | Prior slab B/op -> final | Prior allocations/op -> final | Final/C bytes / allocations |
| --- | ---: | ---: | ---: |
| Go full | 1,360,877 -> 1,157,510.5 | 106 -> 74 | 0.583 / 0.00347 |
| Go edit | 2,262,998.5 -> 2,052,749 | 139 -> 107 | 48.444 / 0.243 |
| C# full | 2,338,521 -> 2,180,489 | 5,857 -> 5,833 | 1.056 / 0.259 |
| C# edit | 2,338,240 -> 2,180,208 | 5,856 -> 5,832 | 53.368 / 13.690 |

Bytes and allocations decrease for all four operations (`p<0.001`). Go full
bytes decrease 14.94% and allocations decrease 30.19%; C# bytes decrease 6.76%.
Complete operations include every view creation, both live edit graphs, and
release. The parser-only legacy measurements do not regress significantly.

The final paired [before](receipts/tree-views/final-complete-before.txt) and
[after](receipts/tree-views/final-complete-after.txt) samples retain all 20
processes and native metrics. The earlier first-wrapper to fixed-slab
[before](receipts/tree-views/complete-before.txt) and
[after](receipts/tree-views/complete-after.txt) campaign is also retained.
The final Go full operation has 74 allocations, versus about 19,284 in the
first view implementation, on the same fixed workload across these campaigns.

The final sparse cache microbenchmark creates a tree over an existing manual
16-node payload chain, visits only the requested path, and releases the tree.
Payload construction is outside this API microbenchmark; it is separate from
complete parse/edit timing. The final cache uses 368 B for the root, 448 B for
one child, and 2,160 B for a 16-edge path. Those counts are below the first
correct implementation's 384 / 472 / 2,872 B, with 3 / 6 / 16 allocations
versus 5 / 8 / 30. Relative to the rejected fixed slab, small paths allocate
more blocks (one child 4 -> 6, deep path 9 -> 16), paying those allocations
only as views are visited. Complete traversal allocates fewer blocks overall.
The direct 20-seed first-implementation to final comparison measures root
535.7 -> 439.3 ns (-18.00%, `p=0.003`), one child
755.5 -> 634.7 ns (-15.98%, `p=0.009`), and path
3.796 -> 2.781 us (-26.74%, `p=0.005`). All three use fewer bytes and
allocations. [Before](receipts/tree-views/sparse-before.txt) and
[after](receipts/tree-views/sparse-after.txt) samples are retained.

## Large-file RSS

Standalone Go test binaries ran inside Docker, one grammar at a time, with
`GOMAXPROCS=1`, one complete full operation, and `/usr/bin/time -v`.
Compiler RSS is excluded. These are cold-operation RSS probes, not randomized
timing evidence. Native C performs its normal initial warm parse and one
measured full operation in the RSS process, so its live RSS scope is explicit.

| Input / grammar | First views KiB | Slabs KiB | C KiB | RSS Go/C, first -> slabs |
| --- | ---: | ---: | ---: | ---: |
| 137 KiB / Go | 67,636 | 57,864 | 10,552 | 6.410 -> 5.484 |
| 137 KiB / C# | 104,532 | 96,012 | 12,760 | 8.192 -> 7.525 |
| 1 MiB / Go | 326,000 | 246,068 | 63,544 | 5.130 -> 3.872 |
| 1 MiB / C# | 522,368 | 453,056 | 67,036 | 7.792 -> 6.758 |

No probe crashed, timed out, or ran out of memory. The C# 1 MiB RSS remains
above the design's 400-bytes-per-source-byte floor. The same generated input
already fails that floor on `f9828512c` through the legacy API: 428,604 KiB,
418,942,472 allocated bytes, and 173,164 allocations. The first view revision's
legacy API measures the identical bytes/allocations and 429,516 KiB.
The optimized view operation uses 447,564,480 bytes and 174,099 allocations.
This remaining parser/output memory limit is recorded; no exemption or gate
threshold was added or changed.

## Default parser performance

A separate final 20-seed paired run compares `f9828512c` with `3192383b7`,
with the API unused and the existing 500-function Go fixture.

| Operation | Baseline | Final | Bytes / allocations |
| --- | ---: | ---: | --- |
| Full DFA | 14.51 ms | 14.40 ms, statistically unchanged | statistically unchanged, 8 allocations |
| Single-byte edit DFA | 284.7 us | 247.4 us, -13.08% median | 390 -> 389.5 B / 5 |
| No-edit DFA | 14.71 ns | 13.96 ns, -5.13% median | 0 B / 0, unchanged |

Edit and no-edit timing differences have `p<0.001`; full time has `p=0.355`.
The view cache is unused in this workload. These are observed final-check
results under variable VM load, not a claim that cache growth speeds parsing.
The final [baseline](receipts/tree-views/final-default-before.txt) and
[head](receipts/tree-views/final-default-after.txt) samples retain all 20 seeds.

The earlier paired `f9828512c` -> `96c9867d8` campaign measured full
15.42 -> 15.39 ms (unchanged), edit 284.2 -> 291.3 us (+2.50%, `p=0.512`),
and no-edit 15.18 -> 15.67 ns (+3.23%, `p=0.110`). Its
[baseline](receipts/tree-views/default-before.txt) and
[slab](receipts/tree-views/default-after.txt) samples are retained. Those
median increases stayed within the 10% timing ratchet. The first API-only
campaign measured full 8.763 -> 8.777 ms (unchanged), edit
180.1 -> 179.3 us (-0.45%), and no-edit 8.418 -> 8.549 ns (+1.56%).
The 2% deterministic ledger tolerance applies to work/reuse counters,
which do not move. Absolute timing differs across campaigns under VM load.

An accidental wrapper-default run that included unrelated benchmarks was
stopped after two seeds and excluded from comparison evidence. Both explicitly
scoped campaigns above completed all 20 seeds.

## Correctness and limits

Docker checks passed for Go and C# old/new graphs at 32 KiB, 137 KiB and 1 MiB,
edit/undo/error/repair sessions on both routes, release orders, repeated payload
occurrences, cursor and query-capture adapters, retained-record payload clearing,
concurrent creation under `-race`, and zero-allocation no-edit reparses.
All 206 registered grammars passed isolated navigation, root/error invariants,
and no-edit smoke checks. Their [per-grammar results](receipts/tree-views/smoke.txt)
and the six [navigation witnesses](receipts/tree-views/navigation.txt) are retained.
Locked C 0.27 agrees on all node types, byte and point ranges, fields, flags,
child counts, and parent edges on all three sizes of generated Go and C#
witnesses, before and after the edit and after newer-tree release.

Both Go and C# R4 invariant gates passed all 72 session steps with zero no-edit
allocations. Python, TypeScript, and TSX R4 sessions and their applicable ledger
and no-edit gates passed. `go vet .` and pinned hot-layout checks passed.

The JavaScript R4 gate fails at step 2 on both the base and the candidate
with the same incremental digest
`f48ed499a10ad9efd0905bdd208fea3d50fe0ded51d25526677e60a1912271bc`
and fresh digest
`50dd63f1e9ff5efecc93b602f2cf2fa5cecdf78d20207bb97c63b88c3d6fb291`.
Its existing 96-step ledger and no-edit allocation tests pass on both.
This pre-existing parser divergence is unchanged and has no new exemption.

This focused receipt does not certify language graduation, all-language edit
sessions, the real-corpus performance matrix, or the full Go-C-C-Go release
performance protocol. The original external audit bundle was unavailable.
Witnesses were rebuilt with the committed generator. The authenticated corpus
lock was fetched outside the repository and verified against committed
SHA-256 `41c744279c8b1b8e26fba733423e77cd48efea46927309c22d163ea`.
The lock is not committed. Legacy query matching and incremental membership
still use existing payload relations; the experimental adapters bind their
returned nodes to views.
