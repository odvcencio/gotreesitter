# Public compact node views

This change connects the compact selected store to the public `*Tree` and `*Node` APIs. A clean admitted parse publishes one stable root header. Children and parent views are created when requested; queries walk compact records and create headers for captures. The result owns its replay and scanner receipts independently of the producing parser.

Baseline: `42baf92651eb70dc1a173e07e019cf9209599ad7`, on draft PR #1512. Comparisons are relative to that stacked branch, not main. Go uses the public publisher within its existing candidate route; this changes neither the admission allowlist nor the global routing default. YAML enrollment requires `gts_compact_node_views` while its throughput is evaluated separately. The final relative timing, allocation, and memory gates pass; the native-C diagnostic is complete. All merges remain held.

## Behavior and ownership

- Only the two authenticated Go and YAML table identities are enrolled. Recovery and unsupported projection provenance retain their existing publisher. No engine language-name condition or graduation allowlist changes.
- Views preserve symbols, fields, byte and point ranges, flags, parser states, production IDs, dynamic precedence, supertype provenance, parent/sibling relationships, and pointer identity. Copies receive independent storage. Concurrent reads and copies do not convert the source tree.
- Editing first restores the complete arena representation, including scanner checkpoints, legacy lookahead receipts, and compact reuse dependencies. The Tree-level incremental publication boundary also restores it for same-width changes made without `Tree.Edit`. Existing node pointers remain valid. The incremental reuse and fresh-verification policies are unchanged.
- Fresh-result verification compares the full compact witness directly: symbols, ranges, points, named/extra/missing/error flags, child structure, and field names. It does not skip the verifier or replace it with a digest.
- View headers and metadata use separate bounded pools. Metadata recycling clears every public-node reference and retains neither an arena nor a producing core. At most four metadata entries of at most 4 MiB each are cached. `DrainArenaPools` drains these caches too. Snapshot recycling retains the existing selected-store byte limit. Full materialization allocates missing cache pages in one block. A cold large-output reserve uses existing raw symbol and alias metadata, with checked growth as fallback; repeated parses reuse its observation and backing. The final public-node slab stops at the known occurrence count. Near-root reuse dependencies use sorted sparse entries, and single-root native publication allocates no unused precedence plane.

## Correctness evidence

V10 repeats all 412 route/language invariant sessions across 206 grammars, totaling 29,664 edit steps, plus 1,236 standard locked-C checks. Every unchanged reparse allocates zero objects. Public API, independent-copy, concurrent-read, detached-ownership, leading-trivia, extra-root, and verifier-mutation tests pass, as do 20 memory-contract tests and focused race checks. The additional YAML C suite includes ordinary and owned document streams through 1 MiB.

V14 repeats Go and YAML's focused C, race, invariant, work and selected-ledger gates after changes to the Tree-level incremental mutation boundary, cold capacity census, and receipt storage. The other 204 grammars inherit V10's unchanged public paths. The mutation regression covers same-width changes without `Tree.Edit`; capacity regressions exclude empty alias slots and the unused single-root precedence plane. Exact receipt comparisons cover materialized sparse dependencies. Go's public parser/edit tests and C checks also run without the experimental output tag. Source manifests and the inheritance receipt separate these scopes.

All 824 complete-operation work rows and 206 selected work/reuse/route ledgers pass without an adverse change. Logical syntax construction remains counted when compact records are published; lazily allocated public headers are separately exposed by the public-operation allocation measurements. The only changed rows are:

| Counter and case | Before | After |
| --- | ---: | ---: |
| New nodes, Go candidate full | 893 | 892 |
| New nodes, YAML candidate full | 232 | 231 |
| New nodes, YAML candidate first edit | 232 | 231 |
| Parent construction proxy, Go compact full | 330 | 329 |
| Parent construction proxy, YAML compact full | 141 | 140 |
| Parent construction proxy, YAML compact edit | 141 | 140 |

The difference removes one redundant published parent. All reuse, completion, coverage, error, and route values remain unchanged. No counter exception or baseline reset is used.

## Public-operation timing

The final Go public-operation suite has 20 matched, shuffled pairs per workload. On the 235,626-byte `grammargen_lr` fixture, parse time falls **99.66 → 91.38 ms (−8.31%)**, allocated bytes fall **59.09%**, and allocation count falls **12.01%**. Parsing plus full traversal improves **2.96%**, parsing plus highlighting **3.95%**, and parsing plus one edit **3.04%** (all p<.001). The 20,168-byte `query_compile` parse improves **3.42%**; its other timing changes are inconclusive.

Go query bytes increase **7.51% / 6.68%** for the small/large fixtures. Parse-plus-edit bytes increase **5.15% / 6.91%**; small-file edit allocations increase **8.37%**, while large-file edit allocations fall **7.82%**. These costs remain below the relative ratchets and are not hidden by parse-only measurements. Complete medians and significance tests are in `benchstat-go.txt`.

The primary candidate-route Go trio measures full parse **8.037 → 7.053 ms (−12.25%)**, with **39.11% fewer bytes** and **36 → 22 allocations**. Single-byte edit time improves **1.33%**, trading **399 → 412 B/op (+3.26%)** at the same five allocations. No-edit remains **0 B/op and 0 allocs/op**, but takes **2.977 → 3.209 ns (+7.81%)**. All changes are significant; see `benchstat-primary.txt` for exact tests.

YAML does **not** qualify as a throughput improvement. Parse and query timings are inconclusive on both fixtures. Full traversal slows **8.86% / 7.13%**, and parse-plus-edit slows **4.65% / 4.55%** for the R4/8 KiB inputs (all p<.001). Every byte-allocation median falls, but the 8 KiB parse/walk each add one allocation, its query adds two, and its edit adds two (**+1.42–2.41%**). Ordinary R4 allocation counts fall **7.69–12.82%**. These results keep YAML behind the experimental output tag. See `benchstat-yaml.txt`.

All **1,040** before/after timing and allocation rows pass the relative ratchets. Standalone legacy Go/YAML walks improve **21.14% / 25.25%**, with zero allocations; their query controls slow **1.25% / 1.98%** with unchanged allocations. The legacy primary full-parse difference is inconclusive, its single-byte edit improves **1.10%**, and no-edit slows **5.39%** (3.005 → 3.167 ns). Legacy primary allocations are unchanged. The six `benchstat-*.txt` files and `performance-analysis.json` retain every result and directional tradeoff. 

Measurements use pinned Docker on Linux amd64, Go 1.26.6, one CPU, `GOMAXPROCS=1`, 20 seeds, `-count=1`, `-benchtime=750ms`, and allocation reporting. The repository randomized driver checks identical workload order within each pair and alternates which revision runs first. The frozen source manifest authenticates the overlay; the driver's dirty-checkout label alone does not identify the candidate source.

## Locked native-C comparison

All **640 timing rows** are complete: 20 shuffled Go–C–C–Go cycles per language and workload. Exact structural digests agree before timing, and every timed Go parse uses the candidate route without fallback. The loop includes parsing, root validation, and release; traversal rows also visit every node. Ratios below are medians of paired cycle ratios, not ratios rounded from the displayed times.

| Language and operation | Go median | C median | Go / C |
| --- | ---: | ---: | ---: |
| go `grammargen_lr_consume` | 97.960 ms | 37.071 ms | 2.634× |
| go `grammargen_lr_parse` | 91.713 ms | 33.800 ms | 2.720× |
| go `query_compile_consume` | 9.656 ms | 3.636 ms | 2.657× |
| go `query_compile_parse` | 9.082 ms | 3.211 ms | 2.819× |
| yaml `eof8k_consume` | 792.670 µs | 417.873 µs | 1.896× |
| yaml `eof8k_parse` | 723.145 µs | 377.500 µs | 1.916× |
| yaml `r4_consume` | 207.393 µs | 104.609 µs | 1.984× |
| yaml `r4_parse` | 190.269 µs | 94.350 µs | 2.019× |

These fixtures remain below the 10× hard ceiling. Go has not reached the 2× stretch target; YAML straddles it. The locked C 0.25.1 runtime is linked into the cgo harness and its pinned grammar is a shared object compiled with `-O2`. Parsing and cursor traversal stay inside C, with one cgo crossing per sample. Go allocation metrics exclude C heap allocation and cannot establish a native-C memory comparison. This is a diagnostic, not the standalone-static publication protocol, a C 0.27.x result, or graduation evidence.

## Allocation and memory results

Twenty first-open-plus-edit pairs keep first-use costs separate from the warmed throughput suite. The parser is warm, but public arena/view pools are drained before each measured open.

| Workload | Open allocated bytes | Edit allocated bytes | Combined bytes | Combined allocations |
| --- | ---: | ---: | ---: | ---: |
| Go `grammargen_lr` | 13,605,304 → 1,529,728 (−88.76%) | 145,494,392 → 158,203,064 (+8.73%) | 159,099,696 → 159,732,800 (+0.40%) | 5,336 → 4,940 (−7.42%) |
| YAML 8 KiB document stream | 2,972,208 → 40,640 (−98.63%) | 2,965,968 → 973,816 (−67.17%) | 5,938,176 → 1,014,456 (−82.92%) | 178 → 176 (−1.12%) |

The 1 MiB result probe uses 20 alternating baseline/candidate pairs per language and consumption mode, including the initial screening pairs. All source hashes, output digests, and completion checks agree. Release returns live heap to within 1 MiB of the warm baseline in every process. Peak RSS passes both the 10% relative gate and the 400-bytes/source-byte ceiling.

| Result consumption | Arena capacity, MB | Total live heap, MB | Peak RSS, KiB |
| --- | ---: | ---: | ---: |
| Go, root only | 83.34 → 37.48 (−55.02%) | 195.45 → 148.55 (−24.00%) | 228,624 → 248,344 (+8.63%) |
| Go, every node | 83.34 → 112.35 (+34.81%) | 195.45 → 224.06 (+14.64%) | 226,986 → 248,032 (+9.27%) |
| YAML, root only | 18.87 → 7.03 (−62.76%) | 47.21 → 35.59 (−24.61%) | 73,676 → 76,548 (+3.90%) |
| YAML, every node | 18.87 → 21.62 (+14.58%) | 47.25 → 50.30 (+6.46%) | 74,540 → 77,364 (+3.79%) |

MB means decimal megabytes. Full traversal retains compact records and public headers, so its live heap increases despite smaller parse allocations. The peak includes warm full-tree preflight and subsequent validation even in root-only mode. See `retention-analysis.json` and `extra-analysis.json` for every median and ratio.

## Rejected screens and measurement scope

V3 passed correctness but failed allocation ratchets because every result allocated new receipt planes and view-cache pages. V4 reused detached metadata and wrote final query captures directly, but its query reader still increased query bytes above the ratchet. V5 passed the reader by pointer; its complete 20-pair run then exposed a large pure legacy traversal regression that parse timings had masked. V6's marker alone did not fix it. V7 split ordinary inlinable child reads from backed dispatch and removed that regression in the initial screen.

V7's complete large-file run then failed peak-RSS ratchets: Go rose about 21%, and one YAML mode rose slightly over 10%. V8 grouped the first edit's missing cache pages. V9 reduced geometric growth but increased transient copying and failed the RSS screen more severely. V10 replaced that experiment with a single initial capacity hint and bounded the final public-header slab, passing the initial RSS screen. V11 added the explicit incremental publication boundary, but its complete 20-pair Go traversal RSS ratio was 1.101379 and failed the 1.10 gate. V12 excluded empty alias slots from the reserve hint but still failed its early traversal screen. V13 stores near-root dependencies sparsely and removes the unused single-root precedence plane, but also fails its early screen. V14 counts visible records and aliases that can add occurrences; its small RSS screen is mixed. Extending the same sample set to all 20 pairs, including the initial pairs, passes every RSS gate. The extension decision and both analyses remain in the archive. Every completed sample and failed check remains in the receipt; failed versions were not promoted.

The large-file probes warm the complete operation first. The language's idle compact-runner pool remains populated even after the public parser is discarded. Consequently, `LiveHeap - BeforeHeap` measures additional retained memory above that cached state, not the complete result's memory. The receipt includes total live heap, reported arena capacity, release heap, and peak RSS. A fully traversed result owns both compact records and public headers; parse allocation savings do not imply a smaller fully traversed live tree.

The initial parse-plus-edit control warmed parsing, traversal, and queries, but did not warm the edit operation. V5 warms the complete operation three times, identically for both roles. Separate first-open-plus-edit allocation probes retain the initial cost. Timings cover parse, full public traversal, public query execution, and parse-plus-edit for two real Go files and two YAML witnesses. The primary Go trio is measured on both routes; separate legacy read controls expose shared Node API overhead.

The latest downstream report is [Cognociente's v0.56.0 report](https://github.com/odvcencio/gotreesitter/issues/454#issuecomment-6311551850). Its small-source verifier remains enabled. The known malformed-delete C mismatch is not an approved exception and remains a graduation blocker. Native C measurements use the locked C 0.25.1 runtime and pinned grammars, with a native parse/consume/release loop and Go–C–C–Go pairing. They are diagnostics, not the standalone-static publication protocol or a C 0.27.x graduation receipt. This bucket does not establish native Windows or ARM64 qualification, main-based net performance, or compact graduation.

## Remaining execution cost

The final candidate profiles isolate the remaining cost; cumulative entries overlap and must not be added.

| Operation | Go cumulative CPU | YAML cumulative CPU |
| --- | ---: | ---: |
| Scheduler token election | 17.39% | 31.08% |
| Native projection | 9.32% | 10.12% |
| Public receipt capture | 3.83% | 5.03% |

The owned loop currently batches reductions through one elected token and returns to the scheduler after a shift. Extending ownership across token elections is the next architectural target, with scanner snapshots, frontier boundaries, and recovery exits preserved. These profiles are attribution, not evidence of a further speedup.

## Reproducing and auditing

[The receipt archive](receipts.tar.gz) contains the frozen baseline/candidate overlays, source hashes, Docker commands, all completed and failed measurements, and analysis scripts for V1–V14. `source-manifest.json` pins baseline `42baf926` and all 25 overlay files; `inherited-qualification.json` identifies unchanged fleet evidence. `memory-extension.json` records the decision to extend the same two RSS screening pairs to 20, without dropping samples. The archive includes a hash/size manifest and was verified after writing. Generated executables are excluded; their hashes remain in the receipts. The prepared benchmark controls and pinned module/grammar identities permit reconstruction from the recorded baseline.
