# Quantified query execution

The cursor and `ExecuteInto` now use the same reader matcher as `Execute` and
`ExecuteNode`. A required sibling that cannot occur rejects a quantified run
before enumerating its capture combinations. This preserves exact empty results
and avoids reporting budget exhaustion for a proven failure.

METRIC: quantified-query ExecuteInto latency | 3.230 s -> 10.51 us | f9828512c + shared reader matcher | 32-comment Go failed-suffix witness
METRIC: quantified-query matcher states | 1000000 -> 0 | f9828512c + shared reader matcher | 32-identifier synthetic failed-suffix witness

## Workload and oracle

The witness was recovered from `programAuditQueries` on overseer revision
`392dc8e29`. The source is `package audit\n`, 32 repetitions of `// audit\n`,
and `func F() {}\n` (314 bytes). The query is:

```scheme
(source_file (comment)+ @comment . (type_declaration) @type)
```

Parsing and query compilation are outside the timed operation. Each operation
creates a cursor, enumerates to completion, and returns all matches (zero).
No work is deferred to a later operation.

The exact-result fixtures run against tree-sitter C 0.27.0 commit
`6070dbfefd326bd735e5683eb128cc1b57dad0c0`, from oracle revision `3a0e7df5d`.
All 102 source-manifest file hashes were verified. The harness replaces only its
Go dependency with the candidate; the oracle sources and grammar locks stay
fixed. Fixtures cover 0, 1, 16, 32, and 64 comments, `*`, `+`, and `?`, failed and
successful suffixes, parent captures, exact capture order, and appending to a
nonempty destination. All 30 cases agree with C through all four public methods.
The fixtures also caught an existing anchor error after an omitted first optional
child, now corrected in both execution paths.

## First increment

Twenty processes per revision, shuffle seeds 1–20, `GOWORK=off`, `GOMAXPROCS=1`,
`-count=1`, `-benchtime=750ms`, and `-benchmem`, using
`scripts/run_randomized_benchmarks.sh`. The host CPU is an Intel Xeon Platinum
8481C. These initial runs were not CPU-pinned; later paired runs should be used
for smaller timing comparisons.

| Complete `ExecuteInto` operation | Before | After |
| --- | ---: | ---: |
| Median time | 3.230 s | 10.51 us |
| Allocated bytes | 5,403,291,824 | 4,416 |
| Allocations | 3,362,956 | 9 |

`benchstat`: time, bytes, and allocations improve with `p=0.000`, `n=20`.
The old audit's 6.26-second observation is not substituted for this fresh baseline.

Deterministic failed-suffix counters, checked before candidate timing:

| Synthetic identifiers | Before states | After states | Matches before/after |
| --- | ---: | ---: | ---: |
| 16 | 131,071 | 0 | 0 / 0 |
| 32 | 1,000,000 | 0 | 0 / 0 |
| 64 | 1,000,000 | 0 | 0 / 0 |

The 32- and 64-child results change from budget-exceeded to complete. The budget
test expectation refresh is a separate `update(pins)` commit. A stale Go
supertype known-difference note is also removed separately: the baseline and
candidate each return the same zero captures as locked C on that fixture.

Focused root query tests, lazy-child query tests, Go query semantics, Go generated
exact queries, and all 30 public-surface fixtures passed in Docker. There were
no crashes or container OOMs. Parser scheduling and incremental logic did not
change. Broader grammar queries, explicit completion reporting, nested budget
propagation, C timing ratios, and process-only RSS measurements remain for the
next increment.
