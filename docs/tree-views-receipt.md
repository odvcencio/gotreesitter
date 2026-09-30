# Tree-scoped navigation receipt, 2026-09-30

Base: `f9828512c`. C oracle: runtime 0.27.0,
`6070dbfefd326bd735e5683eb128cc1b57dad0c0`, from oracle migration
`3a0e7df5`. The oracle migration is used in an isolated measurement checkout;
this change does not move the repository's runtime pin.

METRIC: wrong parent edges, Go old/new views | 1771 -> 0 | f9828512c + tree-scoped views | GeneratedSource(go, 140288), last a + b changed to a - b
METRIC: wrong parent edges, C# old/new views | 0 -> 0 | f9828512c + tree-scoped views | GeneratedSource(c_sharp, 140288), last a + b changed to a - b
METRIC: no-edit allocations with views | 0 -> 0 | f9828512c + tree-scoped views | retained Go root view, 100 reparses

The Go source has 140,368 bytes and SHA-256
`5434575bb249338937331c3756e10dcec34237d107f523ea315996fb6de0ef8f`;
edited SHA-256 is
`993ffc4b3efa6ffac2ab508bfaad0ecb404f9b6605d50f78b497d4bfb78a6e20`.
Each tree has 80,971 nodes; 80,934 payloads are shared. Legacy wrong edges
are 1,771 in the old tree and zero in the new tree. Both view graphs have zero
wrong edges, with distinct views over shared payloads.

The C# source has 140,356 bytes and SHA-256
`e4fc17e581a43b830ada353815eec8b12715f13801076d21edb1af411b231f33`;
edited SHA-256 is
`030e7093fad73f03f96967b07b105381ebce1b730a37099feb60cf276012dece`.
Each tree has 63,645 nodes. C# uses the existing
`external_scanner_unsupported` fresh fallback, so this witness does not
establish C# payload reuse.

## Deterministic counters

| Witness, one edit | Base | Views |
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

## Default parser performance

The standard Go trio used 20 paired alternating seeds, one process per seed,
`GOMAXPROCS=1`, `-count=1`, `-benchtime=750ms`, and `-benchmem`, through
`scripts/run_randomized_benchmarks.sh`. The existing 500-function fixture
and default parse route were unchanged.

| Operation | Base | Views implementation, API unused | Bytes / allocations |
| --- | ---: | ---: | --- |
| Full DFA | 8.763 ms | 8.777 ms, statistically unchanged | unchanged, 8 allocations |
| Single-byte edit DFA | 180.1 us | 179.3 us, -0.45% | 388 B / 5, unchanged |
| No-edit DFA | 8.418 ns | 8.549 ns, +1.56% | 0 B / 0, unchanged |

The no-edit timing regression is below the 2% ledger threshold and is
reported rather than treated as a speed improvement.

## Correctness and limits

Docker checks passed for Go and C# old/new graphs, edit/undo/error/repair
sessions on both routes, release orders, repeated payload occurrences,
cursor and query-capture adapters, concurrent creation under `-race`, and
zero-allocation no-edit reparses. Locked C 0.27 agrees on all node types,
byte and point ranges, fields, flags, child counts, and parent edges on the
32 KiB generated witnesses. Both Go and C# R4 invariant gates passed all
72 session steps with zero no-edit allocations. `go vet .` passed.

The JavaScript R4 gate fails at step 2 on both the base and the candidate
with the same incremental digest
`f48ed499a10ad9efd0905bdd208fea3d50fe0ded51d25526677e60a1912271bc`
and fresh digest
`50dd63f1e9ff5efecc93b602f2cf2fa5cecdf78d20207bb97c63b88c3d6fb291`.
Its existing 96-step ledger and no-edit allocation tests pass on both.
This pre-existing parser divergence is not changed or exempted here.

The original external audit bundle was unavailable. These deterministic
fixtures were rebuilt with the committed generator. The authenticated corpus
lock was fetched outside the repository and verified against committed
SHA-256 `41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea`.
Legacy query matching and incremental membership still use existing payload
relations; the experimental adapters bind their returned nodes to views.
