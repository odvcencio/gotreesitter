# Construction-time compact output — held

**HOLD: this version regresses Go throughput.** The private V4 prototype constructs indexed public topology during authenticated shifts/reductions and transfers an immutable, Core-free raw-body lease at acceptance. It removes SelectedStore construction and publication replay on supported inputs. No runtime code was promoted, no language graduated, no counter baseline changed, and all main merges remain held.

The actual 19,294-byte, 500-function Go benchmark activates this path: 15,505 raw records produce 11,004 public occurrences and 11,003 edges, with no SelectedStore. Its full tree and cold reuse receipts match the original publisher. Exact pinned-C comparisons cover symbols, fields, byte spans, points, flags and digest. Direct parser work and core work are unchanged; `table_lookups_proxy` falls **41,515→26,010**. This proxy decrease does not establish a throughput improvement.

The completed standard runner comparison contains **60 matched pairs / 120 processes**, with 20 explicit shuffle seeds per series, alternating process order, GOMAXPROCS=1, count 1, 750 ms and benchmem. The run used network-disabled, one-CPU, 4-GiB Docker on Linux amd64, Intel Core Ultra 9 285, Go 1.26.6. Source and control guards pass before and after; all R4 compact samples report one route and zero fallbacks.

| Workload | Production → V4 | Change | p |
|---|---:|---:|---:|
| Generated Go full | 7.5157825 → 7.731357 ms | +2.87% | <.001 |
| Go edit | 70.9465 → 71.007 µs | +0.09% | .883 |
| Go no-edit | 3.2205 → 3.114 ns | −3.31% | <.001 |
| Real Go R4 compact | 763.736 → 790.5775 µs | +3.51% | <.001 |
| YAML R4 compact | 196.030 → 196.550 µs | +0.27% | .277 |
| Go R4 legacy control | 638.219 → 640.495 µs | +0.36% | .583 |
| YAML R4 legacy control | 136.9425 → 134.367 µs | −1.88% | <.001 |

Full-parse allocations decrease **22→21**, while bytes increase **261,546→261,678 B**. Edit remains 407 B / 5 allocations; no-edit remains zero. Go/YAML R4 compact allocations remain 14/39 and bytes remain 1,112/5,456. The small no-edit gain and allocation decrease do not offset the two significant Go throughput regressions. Extra parse-plus-query/traversal timing controls were prepared privately but were not executed after the primary screen failed.

Qualification includes eight producer families in release and instrumented builds, a diagnostic clone control, 55 existing core families, 36 focused API/memory families, three Go public-API families, seven new lease-positive API/query families, eight core race families and two public lifetime/race families. New queries cover fields, supertypes, predicates, directives, disabled patterns/captures and highlights. Three lease-positive coordinate cases match C for leading whitespace, UTF-8 and CRLF. Each of three independent edits begins with an asserted lease, then passes incremental=fresh Go and full-axis fresh-C comparisons, including malformed deletion, with no-edit allocations zero. Recovery fallback and fresh verification remain enabled.

Ten existing Go and eight YAML C families also pass. Four actual ordinary Go/YAML default/candidate counter rows equal corrected production exactly. Four R4 edit sessions pass all 288 steps with zero no-edit allocations. These R4 sessions exercise the original publisher: they are fallback-safety evidence, not lease activation evidence.

## Applicability and attribution

The original real-Go R4 fixture declines the new publisher at its first comment (`extra replay-position scope`). A separately pinned diagnostic masks exactly seven Go comments to spaces, preserving all 2,047 bytes and newlines; it then declines after 55 raw records with `ambiguous or reused reduction`. This modified input is not the original corpus and supplies no real-corpus throughput claim. Extras alone cannot establish R4 applicability; fork ownership and compatibility remain independent constraints. Checkpoint-bearing YAML keeps its existing publisher.

Exact-source CPU profiles are retained as function attribution, not randomized comparison evidence. In the candidate profile, final cold-receipt capture consumes **13.12%** cumulative CPU, syntax completion **6.77%**, and producer-footprint calculation **4.37%**, including **4.09%** in the locked recycle-pool census. Nested percentages must not be added. The next architectural targets are to synthesize cold receipts during authenticated construction and avoid repeated locked memory census, with exact accounting and callback proofs intact.

## Failures retained

- V3's live lease included a mutable shared pool in its retained-byte charge. The unchanged public regression observed arena bytes **11,240→11,240**, while retained bytes changed **4,584→241,104**. V4 gives Core sole strong cache ownership, leases weak recycling references, stable lease charges, locked retirement and complete pre-transfer budget admission. The same regression passes unchanged.
- The first C wrapper passed its tests but failed its post-check because relative source checksums ran from the C harness directory. The corrected wrapper reran all selected C cases and passed both guards.
- The first copy control compared original receipts with a second-generation copy. An unchanged strict diagnostic fails identically for both publishers: first copy retains root read word 2; the ordinary second copy abstains with word 0 and no certified reuse. Corrected controls compare corresponding copies, preserve the positive first-copy receipt, require the same conservative second-copy abstention and verify lifetime/query results after original release and Core reset. No engine behavior changed to make this test pass.
- The first R4 diagnostic failed to compile because a root-package test imported a package that imports the root. Its external-package replacement passes. Both preceding profiles completed successfully and were not rerun.

The archive retains these source versions, red outputs and corrected controls. V4 remains a rejected throughput experiment despite its focused correctness results. Full 206-grammar qualification, large-file RSS gates, E-A graduation, Go 1.22/1.23 fallback builds, Windows and ARM64 are unqualified here. No Mac execution or C-oracle upgrade is claimed.

[summary.json](summary.json) contains exact paired medians; [verification.json](verification.json) records scope; [source-identities.json](source-identities.json) pins sources. [evidence.tar.gz](evidence.tar.gz) and its [member index](contents.json.gz) contain frozen overlays, raw outputs, benchmark reports, profiles and replay controls. They contain no native executables, caches, credentials, private CSV or unrelated prototype runtime. Replay uses the pinned source checkout and dependencies with the recorded bounded Docker commands. Run `sha256sum -c SHA256SUMS` and `python3 validate-evidence.py` for read-only archive validation.
