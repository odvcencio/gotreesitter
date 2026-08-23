# P13 incremental attribution report

Disposition: NO-GO for one cross-case performance gate.

The candidate remains suitable for one targeted follow-up on `early_newline`.
The evidence does not support a gate for all four representative cases.

## Evidence identity

- Worktree: `/home/draco/work/gts-p13-attribution-20260821`
- State: detached evidence worktree
- Base commit: `de4fe455d3a9778b8a9b09347908e860a1f6b7f8`
- Base subject: `Merge pull request #752 from odvcencio/codex/perf-p11-pagepool-20260821`
- Base date: `2026-08-21T13:01:10-07:00`
- Source tree: clean at worktree creation
- Production edits: none
- Evidence-only edits: the benchmark hook, two evidence benchmarks, and one differential test

The worktree started from `de4fe455` after inspection of the uncommitted P12 worktree.
The P12 worktree contained uncommitted harness changes, so this report does not use them.

## Method

Run each command in Docker with one pinned CPU, one Go process, and an eight-gigabyte limit.
Use `GOMAXPROCS=1`, `-count=1`, and explicit shuffle seeds.

Admit each representative case before timing.
Perform the initial parse before timing.
Start CPU sampling and the wall timer immediately before `ParseIncrementalProfiled`.
Stop both immediately after it returns.
Capture the runtime profile before validation and reporting.
Capture allocation space before validation on the final iteration.
Run `runtime.GC()` before the final live-heap snapshot.

The timer excludes semantic admission, the initial parse, `Tree.Edit`, and validation.
The CPU profile excludes validation and the benchmark report path.
The allocation profile includes only the timed parse calls after the admission snapshot.

The first matrix probes failed because they exercised a Python full-parse fallback.
The successful matrix uses a temporary benchmark that selects only the four Go representatives.
The failed probes remain under `evidence/docker` for audit.

## Case disposition

Use a conservative credibility rule for this report.
Require both directions to provide at least 0.5 seconds of CPU samples, 10 percent profile coverage, and 20 percent direct state-restore samples.

| Case | Iterations | Timed seconds | CPU samples, forward/reverse | Direct state-restore share, forward/reverse | Decision |
| --- | ---: | ---: | ---: | ---: | --- |
| `token_class_change` | 20 | 0.764796060 | 0.450 s / 0.360 s | 11.11% / 5.56% | No credible gate |
| `same_line_length_change` | 20 | 1.046878004 | 0.520 s / 0.570 s | 1.92% / 3.51% | No credible gate |
| `early_newline` | 4 | 2.647591267 | 1.240 s / 1.420 s | 36.29% / 22.54% | Targeted gate is credible |
| `recovery_deletion` | 100 | 0.446196562 | 0.270 s / 0.100 s | not observed / not observed | No credible gate |

The direct state-restore share names `stackEntryNodeParseState` flat samples.
The candidate removes that state restore from the content-hash loop.

The recovery profiles cover only 2.69 percent and 1.00 percent of their ten-second profile windows.
The missing raw-shape samples do not prove zero work.
They show that this isolated profile does not measure that work credibly.

## CPU attribution

The values below use cumulative samples from the merged direction profiles.
Nested percentages overlap and must not be added.

| Case and direction | Samples | ParseIncrementalProfiled | parseInternal | Reduce chain: apply / dispatch / GSS | captureRawShape | rawShapeHash | raw hash compute | child entry | parse state |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| token forward | 0.450 s | 86.67% | 86.67% | 22.22% / 22.22% / 22.22% | 8.89% | 11.11% | 11.11% | 11.11% | 11.11% |
| token reverse | 0.360 s | 94.44% | 94.44% | 38.89% / 47.22% / 47.22% | 11.11% | 5.56% | 5.56% | 5.56% | 5.56% |
| same-line forward | 0.520 s | 90.38% | 92.31% | 21.15% / 25.00% / 21.15% | 7.69% | 3.85% | 3.85% | 1.92% | 1.92% |
| same-line reverse | 0.570 s | 94.74% | 92.98% | 29.82% / 33.33% / 31.58% | 10.53% | 5.26% | 7.02% | 3.51% | 3.51% |
| early forward | 1.240 s | 45.16% | 45.97% | 16.94% / 16.94% / 16.94% | 8.06% | 59.68% | 59.68% | 37.10% | 36.29% |
| early reverse | 1.420 s | 55.63% | 57.04% | 21.83% / 23.24% / 22.54% | 9.15% | 43.66% | 43.66% | 23.24% | 22.54% |
| recovery forward | 0.270 s | 100.00% | 100.00% | 14.81% / 18.52% / 18.52% | not observed | not observed | not observed | not observed | not observed |
| recovery reverse | 0.100 s | 100.00% | 100.00% | 30.00% / 30.00% / 30.00% | not observed | not observed | not observed | not observed | not observed |

`rawShapeHash` and `rawShapeComputeContentHash` include recursive child work.
Treat their percentages as overlapping inclusive paths.
Use the child-entry and parse-state rows to estimate the removable state restore.

The raw timer statistics are:

| Case | Count | Mean milliseconds | Minimum milliseconds | Maximum milliseconds |
| --- | ---: | ---: | ---: | ---: |
| `token_class_change` | 20 | 38.239803 | 25.231950 | 65.518470 |
| `same_line_length_change` | 20 | 52.343900 | 35.202436 | 81.042412 |
| `early_newline` | 4 | 661.897817 | 572.547384 | 727.226406 |
| `recovery_deletion` | 100 | 4.461966 | 2.722947 | 10.292689 |

## Allocation space and live heap

Allocation space is the delta from `alloc_before.pb` to `alloc_after_timed.pb`.
Live heap is the total in `heap_after_forced_gc.pb` after `runtime.GC()`.

| Case | Allocation space | Allocation per timed call | Heap before | Heap after forced garbage collection |
| --- | ---: | ---: | ---: | ---: |
| `token_class_change` | 221.94 MB | 11.097 MB | 16,275.31 kB | 26,434.52 kB |
| `same_line_length_change` | 365.16 MB | 18.258 MB | 33,850.08 kB | 35,112.51 kB |
| `early_newline` | 361.85 MB | 90.463 MB | 145.25 MB | 74.87 MB |
| `recovery_deletion` | 96.72 MB | 0.967 MB | 8,736.46 kB | 36,852.36 kB |

Allocation space is dominated by arena growth and recovery work.
The candidate changes no allocation shape and adds no cache.

The raw allocation and heap reports use the matching Docker-built binary.
The CPU reports use their matching CPU-profile binary.

## Canonical matrix

Keep generated controls separate from representative dispositions.

Generated controls from the locked manifest:

- `unchanged_snapshot_identity`
- `same_length_leaf_validation`
- `python_scanner_dedent`

Representatives in this report:

- `token_class_change`
- `same_line_length_change`
- `early_newline`
- `recovery_deletion`

The raw matrix contains 20 shuffled outputs for each Go and C row of every representative.
It contains 160 benchmark rows and 20 `PASS` results.
The command used an eight-gigabyte container, one CPU, CPU 19, and 750-millisecond runs.
The matrix container reached 365,964 kilobytes of maximum resident set size.

The `benchstat` summaries show the following median values and 95 percent confidence ranges:

| Case | Go seconds per operation | C seconds per operation | Go bytes per operation | C bytes per operation | Go allocations per operation | C allocations per operation |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `token_class_change` | 42.77 ms ± 15% | 86.48 us ± 14% | 13.58 MiB | 20.27 KiB | 96.50 | 9 |
| `same_line_length_change` | 54.24 ms ± 11% | 101.2 us ± 11% | 20.29 MiB | 18.27 KiB | 295.0 | 9 |
| `early_newline` | 825.6 ms ± 10% | 669.7 us ± 8% | 73.27 MiB | 232.3 KiB | 1,565 | 9 |
| `recovery_deletion` | 5.930 ms ± 10% | 63.80 us ± 15% | 1.291 MiB | 3.344 KiB | 119.0 | 9 |

The raw file is `matrix/canonical-go-c-20-seeds-v3.txt`.
Do not use the aggregate table in place of the raw file.

## Static chain evidence

Canopy traced the chain in scoped, no-cache queries.
The query outputs are retained under `meta/`.

| Edge | Source location |
| --- | --- |
| `benchmarkCanonicalGoIncremental` to `ParseIncrementalProfiled` | `cgo_harness/benchmark_go_canonical_incremental_test.go:886`, call at `:912` |
| `ParseIncrementalProfiled` to incremental parse and `parseInternal` | `parser_api.go:1689`, incremental route begins at `:1700`; `parseInternal` at `parser.go:4512` |
| `parseInternal` to `applyActionWithReduceChain` | `parser.go:6538` to `parser_reduce.go:590` |
| `applyActionWithReduceChain` to `applyReduceActionDispatch` | `parser_reduce.go:598` to `:2669` |
| `applyReduceActionDispatch` to `applyReduceActionFromGSS` | `parser_reduce.go:2683` to `:3918` |
| `applyReduceActionFromGSS` to `captureRawShape` | `parser_reduce.go:3996` to `raw_shape.go:365` |
| `captureRawShape` to content hash | `raw_shape.go:408` to `:422` |
| content hash to `nodeArena.rawShapeHash` | `raw_shape.go:442` to `:273`; this is the recursive child hash edge |
| content hash to `rawShapeChild.entry` | `raw_shape.go:431` to `:49` |
| `rawShapeChild.entry` to `stackEntryNodeParseState` | `raw_shape.go:51` to `no_tree_node.go:328` |

The requested chain includes an inline incremental wrapper between `ParseIncrementalProfiled` and `parseInternal`.
The wrapper does not affect the timer boundary or the attribution decision.

## One candidate design

Evaluate only this design:

> In `rawShapeComputeContentHash`, read `children[i].packedEntry` directly.
> Keep `shapeRef()` for the packed reference.
> Avoid `rawShapeChild.entry()` during content hashing.

Make the following caller change in `raw_shape.go:431`:

```go
entry := children[i].packedEntry
```

Keep all other hash operations in their current order.
Do not add a cache, field, slab, or public accessor.

### Bit-identical proof

Prove equality before merge:

1. `rawShapeChild.entry()` copies `packedEntry` and replaces only `state`.
2. `stackEntryHasNode` reads only `node`.
3. Symbol, start byte, end byte, and child count accessors dispatch on `kind` and `node`.
4. The hash loop does not read `entry.state`.
5. `shapeRef()` still reads the packed reference from `packedEntry.state`.
6. The same seed, field order, sentinel, recursive hash, and child-count fallback remain.
7. The strict `ref != 0 && ref < parentRef` guard remains unchanged.

The evidence-only differential test covers Node, no-tree, compact-full-leaf, pending-parent, nil, recursive, and forward-reference entries.
The test passed in Docker run `20260821T213432Z-p13-candidate-hash-proof`.
The existing raw-shape and reclamation tests passed in Docker run `20260821T213512Z-p13-raw-shape-focused-baseline`.

### Privacy and state lifetime

Keep `rawShapeChild`, `packedEntry`, and `shapeRef` unexported.
Use `entry()` only at consumers that need the current parser state.
Keep packed references inside parser-owned raw-shape slabs.
Pass only value copies to state-independent payload accessors.
Do not return a packed entry to parser callers.
Do not call `stackEntryNodeParseState` from the hash helper.

The state nonescape proof requires a source review after implementation.
Search the hash helper for `entry()` and `stackEntryNodeParseState`.
Run escape analysis for the focused package.

### Reference guard and reclamation

Keep `ref != 0 && ref < parentRef`.
Treat zero, same-parent, and forward references as child-count fallbacks.
Preserve the existing direct-mapped hash cache and its eviction behavior.

Keep `reclaimRawShapeStorage` unchanged:

- Clear raw references in Node, no-tree, compact, pending, and checkpoint payloads.
- Reset raw-shape and raw-child slabs.
- Reset the existing hash cache.
- Recompute arena allocated bytes.
- Retain only the bounded warm slab prefix.

The existing raw-shape tests check stale-reference clearing, bounded retention, cache recomputation, and incremental reuse after reclamation.

### Memory accounting

Keep `unsafe.Sizeof(rawShapeChild{})` at 16 bytes.
Keep `rawShapeBytesAllocated`, `rawShapeChildBytesAllocated`, and `rawShapeHashCacheBytesAllocated` unchanged.
Keep `recomputeAllocatedBytes` unchanged.
The candidate should reduce CPU work without changing memory accounting.

### Parity, no-edit, and ownership invariants

- Preserve fresh Go and C tree parity in both edit directions.
- Preserve incremental Go and C tree parity in both edit directions.
- Preserve the no-edit identity result and zero incremental profile.
- Preserve same-length leaf validation.
- Preserve old-tree release only when the new tree differs.
- Preserve tree ownership after raw-shape reclamation.
- Preserve zero raw references in returned public trees.
- Preserve arena isolation for every raw-shape reference.

The timer harness admits and validates these invariants before timing.
The final implementation must run the validation path again after the hash change.

## Focused gates after implementation

Run each gate in Docker.
Keep correctness and performance runs separate.

1. Run the raw-shape differential and layout tests.
2. Run the raw-shape reclamation and incremental ownership tests.
3. Run `TestCanonicalGoIncrementalParity`.
4. Repeat the four-case matrix with 20 explicit shuffle seeds.
5. Run the standard randomized benchmark loop with the primary benchmark trio.
6. Run the focused race and ownership checks in the dedicated container.

Reject the candidate on any parity, no-edit identity, stale-reference, memory-budget, or ownership failure.
Do not approve the cross-case gate unless every representative meets the credibility rule.

## Artifact locations

- CPU profiles: `profiles/*_v2/*.pprof` and merged profiles
- Allocation profiles: `profiles/*_alloc_v2/alloc_*.pb`
- Forced-GC heap profiles: `profiles/*_alloc_v2/heap_after_forced_gc.pb`
- Profile manifests: `profiles/*/manifest.json`
- Symbolized reports: `pprof_text/`
- Raw matrix: `matrix/canonical-go-c-20-seeds-v3.txt`
- Docker logs and metadata: `docker/`
- Static Canopy traces: `meta/canopy_*.json`
- Artifact manifest: `p13_artifact_manifest.json`
- Signed receipt: `p13_signed_receipt.json`

The artifact manifest and receipt provide the final hashes and signature.
