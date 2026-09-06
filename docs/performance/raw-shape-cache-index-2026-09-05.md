# Raw-shape cache indexing

Incremental slabs contain 1,024 headers. The old index discarded slab identity, forcing every slab into the same 1,024 of 32,768 slots.
The new index selects the high product bits. Cache capacity, reference validation, and content hashes remain unchanged.

Across 20 randomized pairs, the 256-function C recovery benchmark fell from 484.1 ms to 242.1 ms (49.99%, p < 0.001).
The 16-function C case and Go benchmark trio showed no significant timing change.
The C benchmark excludes Tree.Edit and old-tree construction. It measures incremental parsing and result release.
These results do not establish universal performance relative to C.

Focused raw-shape, eviction, and transient materialization tests passed.
Both versions passed C edit comparisons against fresh Go trees and the scoped C parity controls.
The known locked-C error-flag divergence remained unchanged. All containers completed without OOM failures or timeouts.

C allocations changed from 91.62 to 83.56 MiB/op and from 1,130 to 1,120 allocations/op.
Iteration counts affect pool amortization; some large samples used one iteration.
Single correctness processes reached 840,408 and 972,804 KiB maximum RSS, respectively.
These measurements include compilation and do not isolate parser retention.

## Reproduce

Create a baseline worktree at `332fcdd9dcd03f2a169e6d4418f91bd6e53e64f7` and a candidate with this change.
Copy the candidate benchmark source into the baseline.
Mount the baseline at `/baseline` and an output directory at `/evidence` in the repository Docker runner.
Use one pinned CPU, 4 GiB memory, and GOMAXPROCS=1. Run this command inside the container:

```sh
cd /workspace
GOT_BENCH_FUNC_COUNT=500 bash scripts/run_randomized_benchmarks.sh \
  --output /evidence/candidate.txt \
  --baseline-root /baseline --baseline-output /evidence/baseline.txt \
  --runs 20 --seed-start 1 --benchtime 750ms \
  --tags gts_parsercorephase0 \
  --bench-regex '^(BenchmarkGoParse(FullDFA|IncrementalSingleByteEditDFA|IncrementalNoEditDFA)|BenchmarkCIncrementalRecoveryGrowth)$'
benchstat /evidence/baseline.txt /evidence/candidate.txt
```

The measured source snapshots remained unchanged during all pairs.
Raw logs and source manifests remain in the external archive:
`/home/draco/work/gts-authenticated-edit-optimization-evidence-20260905/cache-index`.
