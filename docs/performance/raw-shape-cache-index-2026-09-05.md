# Use slab identity in the raw-shape hash cache

Incremental arenas allocate 1,024 raw-shape headers per slab. References store slab identity above bit 19.
The previous cache index selected the low 15 bits after multiplication. These bits cannot depend on slab identity.
Equal offsets from every slab therefore competed for the same 1,024 slots in a 32,768-slot cache.
Eviction can trigger recursive hash recomputation during conflict reduction.

The candidate selects the high 15 product bits. It preserves cache capacity, reference validation, and the content hash.
The unsuccessful parser-state shortcut is excluded.

## Correctness

The slab-identity regression fails on the baseline and passes on the candidate.
The eviction test forces a collision and verifies exact hash recomputation.
The transient checkpoint test records nested hashes before materialization.
It then reuses the original storage and clears the cache before each hash comparison.
All focused raw-shape and transient tests pass.

Both versions pass the C replacement, insertion, and deletion comparisons against fresh Go trees.
Both versions pass the scoped C parity controls.
The known locked-C error-flag difference remains unchanged. This is not complete C parity.
All correctness containers exited successfully without OOM failures or timeouts.

## Performance method

Run the repository randomized benchmark script with 20 alternating baseline/candidate pairs.
Use seeds 1 through 20, GOMAXPROCS=1, count=1, 750ms, and allocation measurements.
Pin one CPU and set a 4 GiB container memory limit.
Use identical test overlays in both worktrees. Only raw_shape.go differs.
Verify source hashes after all measurements complete.

The C benchmark excludes old-tree construction and Tree.Edit.
It measures incremental parsing and result release after deleting the first character of the first local identifier.
The old tree uses the legacy parser. This separates the workload from compact-parser graduation.
Include the Go full-parse, single-byte edit, and no-edit benchmarks as controls.

All 20 pairs completed without OOM failures or timeouts. All recorded source hashes remained unchanged.

| Benchmark | Baseline | Candidate | Change |
| --- | ---: | ---: | --- |
| C, 256 functions | 484.1 ms | 242.1 ms | -49.99%, p < 0.001 |
| C, 16 functions | 3.705 ms | 3.686 ms | No significant change |
| Go full parse | 22.76 ms | 22.31 ms | No significant change |
| Go single-byte edit | 201.2 us | 201.7 us | No significant change |
| Go no edit | 4.133 ns | 4.127 ns | No significant change |

The large C case used 91.62 MiB/op before and 83.56 MiB/op after the change.
Allocation counts changed from 1,130 to 1,120 per operation.
Different benchmark iteration counts can change pool amortization. These results do not prove lower intrinsic storage requirements.
Some large C samples used one iteration. Throughput output also rounded one baseline sample to zero.
Use time per operation for the performance conclusion; do not infer throughput from that rounded sample.

This result supports the index change for the tested recovery workload. It does not establish universal performance relative to C.

## Evidence and reproduction

The baseline commit is `332fcdd9dcd03f2a169e6d4418f91bd6e53e64f7`.
The evidence directory contains the complete benchmark logs, benchstat output, controls, and launch scripts.
Create two worktrees from that baseline. Apply this change only to the candidate.
Copy the three changed test files into the baseline so both test overlays match.
Update the absolute worktree and evidence paths in `launch-paired.sh` before running it.
The full source manifest is retained in the external evidence archive.


## Memory limitation

The baseline correctness process reached 840,408 KiB maximum resident size. The candidate reached 972,804 KiB.
These single measurements include compilation and all three edits. They do not isolate parser retention.
Record the increase; do not claim a resident-memory improvement from unchanged cache capacity.
Use the randomized allocation results as separate evidence.
