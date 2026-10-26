# Compact raw-output screen

**Hold the runtime.** The owned raw-body output path passes the scoped
correctness gates, but generated Go full parsing regresses 18.96% and ordinary
Go regresses 16.33%. No runtime, routing, dependency or graduation change is
included. Main merges remain held.

The prototype transfers immutable raw records and closed replay-context facts
to a producer-free result. Public nodes and local child plans are created on
demand. It avoids constructing a SelectedStore and running final table replay.
Unlike earlier output experiments, it activates on unchanged ordinary Go with
comments and ambiguity. This establishes a working representation, not a speed
win.

## Complete randomized comparison

The unchanged runner completes 60 matched pairs in 120 processes: 20 seeds per
suite, alternating execution order, matched workload order, 750 ms, count 1,
GOMAXPROCS 1 and memory metrics. Linux amd64, Intel Core Ultra 9 285, Go 1.26.6;
Docker uses one pinned CPU, 4 GiB and no network. Both arms use the same wrapper;
the candidate contains no diagnostic/work-counter instrumentation.

| Workload | Before | After | Time change |
| --- | ---: | ---: | ---: |
| Generated Go full | 7.0852 ms | 8.4288 ms | +18.96%, p<.001 |
| Generated Go edit | 67.488 µs | 64.768 µs | −4.03%, p<.001 |
| Generated Go no edit | 3.088 ns | 2.9595 ns | −4.16%, p<.001 |
| Ordinary Go compact | 774.143 µs | 900.598 µs | +16.33%, p<.001 |
| Ordinary Go legacy control | 644.165 µs | 639.314 µs | inconclusive, p=.183 |
| Ordinary YAML compact | 193.973 µs | 193.321 µs | inconclusive, p=.779 |
| Ordinary YAML legacy control | 135.989 µs | 134.037 µs | −1.44%, p<.001 |

Generated full-parse bytes fall 261,543→139,397 B/op and allocations 22→16.
Ordinary Go bytes rise 1,112→1,784 B/op while allocations fall 14→10. Edit
bytes are 407→406 B/op with five allocations. No-edit bytes/allocations remain
zero. YAML allocation metrics are unchanged. Reduced allocated bytes do not
establish reduced retained memory or RSS.

## Correctness and deterministic work

Before traversal, both original Go sources require the actual raw lease,
one public root header, zero Core-owned raw records and no SelectedStore.
The 19,294-byte generated source has 15,505 raw records and 11,004 public nodes;
ordinary R4 has 1,283 raw records, 1,383 replay contexts and 892 public nodes.
Both retain exact public trees, states, fields, queries and cold receipts.
Copy/reset/reuse lifetime, source-authority attacks, capacity accounting and
concurrent reads/query/copy checks pass.

The four actual Go/YAML ledger files have one change: candidate Go full
`new_nodes` **892→1**. All other fields, including edit work and reuse, are
identical. No counter exception is used. Four invariant sessions pass 288 edit
steps with no-edit allocations zero. Existing Go/YAML C suites, the original
Cognociente witness and all 96 assignment edit cases remain green.

Exact C comparisons use the original generated/R4 sources in both reference
and lease arms. They pass against repository C 0.25.1 and the separately pinned
private C 0.27.1 runtime `10eb5ac119eec9cb4267a8e035066473983f5f3a`.
The C027 selection uses only the previously qualified default parse/close
binding adaptation; it does not establish general binding compatibility or
change the repository dependency.

The compact summary header is 68 bytes, down from 112. The unchanged 1,024-item
synthetic stress case fits its original byte cap. Actual ordinary generated
formation edges are `51*n+8` and closure edges `31*n+4` for n=32/128/512/1024.
Exceptional plans have a checked raw-edge bound before traversal. All five
ordinary work witnesses require real activation; a repeated full API walk adds
no plan/cache work, and formation counters remain unchanged after publication.

## Attribution and retained failures

Four exact-input CPU profiles are diagnostic attribution only. Candidate
generated Go spends 28.02% cumulative CPU in summary capture, including 11.06%
in composable-summary construction. Raw-child/root classification and repeated
pool accounting contribute further nested costs. Baseline publication totals
18.06% cumulative CPU. These overlapping percentages must not be added.
The removed publication work has been replaced by more expensive construction.

The archive retains the original zero-end history test expectation failure,
ordinary-entry refusal at external semicolons, cleanup overwriting refusal and
transfer receipts, the first linear draft's 1,024-item byte-cap failure, and
the C027 read-only submount preparation failure before container execution.
Repairs preserve the original sources, hard caps and strict comparators.

Full 206-language qualification, large-file RSS, public-consumer timing,
native ARM/Windows and E-A/M1 gates remain unqualified. YAML checkpoint-transfer
and fused payload/summary construction are separate follow-up experiments.
Fresh verification stays enabled. The owner-approved assignment exception in
commit `dd7d1d1` covers a different correctness receipt.

[summary.json](summary.json), [verification.json](verification.json),
[source-identities.json](source-identities.json), [evidence.tar.gz](evidence.tar.gz)
and [contents.json.gz](contents.json.gz) retain the measured inputs and outputs.
Validate without compiling or rerunning workloads:

```sh
python3 docs/benchmarks/20261025-compact-raw-screen/validate-evidence.py --source-root /home/draco/work/gotreesitter
```
