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

## Second increment

All public match methods now share the cursor traversal and reader matcher.
`ExecuteWithStatus`, `ExecuteIntoWithStatus`, and `QueryCursor.Status` distinguish
pending, complete, output-limit, and work-budget results. Nested alternatives
share the outer work allowance; recursive states have a separate 4,096-state
bound. A final successful scalar repetition accumulates captures iteratively.
Directives run on owned output, preserving the reusable capture prefix.

The matcher now enumerates every nested-alternative result and every contiguous
sibling run. Locked C confirms the corrected run counts: 1 -> 2 matches across
a named separator, and 1 -> 3 across anonymous separators. These expectations
are refreshed separately. The compiler also checks virtual-supertype children
before wildcard-root optimization, matching C's rejected JavaScript patterns.

METRIC: successful quantified-query latency | 1.909089 s -> 4.651 ms | a604824e6 + second increment | 4096-comment Go complete ExecuteInto operation
METRIC: ordinary-query allocated bytes | 5365136 -> 2403813 B/op | a604824e6 + second increment | BenchmarkQueryExecCompiled, 500-function Go source

Twenty paired seeds, alternating baseline/head order, CPU 2, and the standard
750 ms settings. The deterministic successful-run counters remain 33, 257,
and 4,097 states at widths 32, 256, and 4,096; all matches and captures remain
exact. The failed-suffix counters remain zero.

| Complete operation | Before time | After time | Before / after bytes | Before / after allocations |
| --- | ---: | ---: | ---: | ---: |
| Successful run, 32 comments | 181.65 us | 43.78 us | 85,480 / 12,480 | 100 / 18 |
| Successful run, 256 comments | 4.4098 ms | 228.8 us | 2,950,888 / 87,360 | 575 / 27 |
| Successful run, 4,096 comments | 1.909089 s | 4.651 ms | 628,211,184 / 2,059,969 | 8,309 / 47 |
| Failed suffix, 32 comments, ExecuteInto | 24.72 us | 23.29 us | 4,416 / 4,360 | 9 / 8 |
| Compiled Go highlight query | 52.01 ms | 46.85 ms | 5,365,136 / 2,403,813 | 94,055 / 35,031 |

Successful-run timing and all allocation improvements have `p=0.000`, `n=20`.
Ordinary-query and witness timing differences are not significant. Shared VM
load produced 33–54% timing intervals; these runs establish the large successful
run improvement, not a small ordinary-query timing win.

Rejected candidate: traversal unification initially copied patterns and
allocated recursive closures for scalar alternatives. The preceding 20-seed
run regressed the ordinary query from 23.22 to 30.43 ms (+31.08%), from
5.117 to 9.295 MiB (+81.66%), and from 94,055 to 129,564 allocations (+37.75%).
The accepted candidate uses compiled pattern pointers and handles scalar
alternatives without a recursive continuation. The rejected results are retained
as evidence, not used as the accepted comparison baseline.

Focused query and selected-store query/directive tests passed in Docker.
All 50 generated-query grammar runs passed against the locked C runtime, one
grammar per process. Go and JavaScript semantics, the nested-alternative fixtures,
and an 8,192-comment complete-result fixture also passed. No gate was relaxed.

Raw paired receipt SHA-256:

- Before: `7e54bc184c557e4fc34f69b99237a58b4567c5de6292df278b5c0330eb000566`.
- After: `d209b123d1b5c1684b1bf92a9cee536c622cf444eda220d74d08edafac099b85`.
