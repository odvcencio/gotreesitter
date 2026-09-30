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

## Third increment and complete-operation C receipt

Root scalar repetitions now collect one run iteratively instead of copying
every prefix. Duplicate alternatives use exact interned capture sequences.
Output storage is bounded at 4,096 states and 1,000,000 capture cells per
attempt, independently of the work limit. Exhaustion returns an explicit
incomplete status. A cursor reuses its attempt budget.

`NextCapture` merges streams by source position, pattern index, and match
sequence, preserving capture order within each stream. It also preserves C's
unfinished greedy-prefix capture events. The queue is bounded and traversal
maintains a source-position watermark without rescanning the worklist. A
32-bit child index keeps each traversal entry at 32 bytes.

METRIC: quantified-query complete-operation Go/C | 88104 -> 0.1848 | f9828512c + bounded capture merge | 314-byte, 32-comment failed-suffix witness; locked C 6070dbfe

The original revision and final candidate each ran 20 explicit shuffle seeds
through the benchmark wrapper, using Go-C-C-Go cycles and the standard 750 ms
settings. Parsing and compilation are outside the measurement. Every timed
operation creates a cursor and consumes all matches; Go must report complete.
The baseline and candidate were pinned to different CPUs on the same busy VM.
The C control moved from 81.46 to 57.56 us, so small absolute timing differences
between these two runs should not be attributed to code. The failure removal
and paired Go/C improvement are much larger than that variation.

| Complete match operation | Original Go | Final Go | Final C | Median Go/C |
| --- | ---: | ---: | ---: | ---: |
| Failed suffix, 32 comments | 7.199 s | 10.64 us | 57.56 us | 0.1848 |
| Successful child run, 32 comments | — | 21.76 us | 66.58 us | 0.3264 |
| Successful child run, 4,096 comments | — | 3.079 ms | 287.9 ms | 0.01043 |
| Successful root run, 4,096 comments | — | 2.606 ms | 47.12 ms | 0.05445 |

Native allocation hooks count C malloc, calloc, and realloc requests. The
receipt also counts the binding's Go allocations and its libc match records,
which bypass the runtime hooks. This measures requested allocation volume,
not peak live memory or allocator rounding. One Go grammar runs per process.

| Complete match operation | Go bytes / allocations | C total requested bytes / allocations |
| --- | ---: | ---: |
| Failed suffix, 32 comments | 3,880 / 4 | 7,336 / 18 |
| Successful child run, 4,096 comments | 2,060,440 / 50 | 1,655,471 / 64 |
| Successful root run, 4,096 comments | 921,792 / 4,124 | 1,654,616 / 60 |

The successful child run adds three bookkeeping allocations over the second
increment's 47, a 6.38% increase, while adding about 0.02% allocated bytes.
It supplies exact output deduplication and storage accounting. Work counters
remain 33, 257, and 4,097 for child runs, zero for failed suffixes, and zero
before/after for the scalar root path. No work or reuse ledger threshold moves.

Rejected candidate: adding a source watermark initially grew traversal entries
from 32 to 40 bytes. It increased failed-witness bytes at width 32 by 3.67%
and at width 64 by 100.49%. Narrowing the child index restored the original
entry size. The rejected 20-seed receipt is retained separately.

Locked C has an additional defect: `QueryState.consumed_capture_count` is a
12-bit field. For a match with more than 4,096 captures, `NextCapture` wraps
from index 4,095 to zero and never finishes. Initial unbounded oracle collectors
hit their isolated 8 GiB limit. The narrowed C-only probe reproduced the wrap
without growing memory. Wide fixtures now assert that exact wrap, compare its
finite prefix, then use the remaining captures from the complete C match as
the expected finite tail. This keeps the C source and pin fixed; it does not
claim that the original C capture iterator completes on those inputs. Both
8,192-comment fixtures pass, and their bounded oracle process peaks at
65,236 KiB RSS. This oracle workaround needs owner review.

The range oracle previously compared Go captures against C matches. Comparing
against C captures proves that a zero-width capture exactly at the range start
is excluded; traversal and match intersection still include that node. The
expectation refresh is a separate `update(pins)` commit (2 -> 1 captures).

Focused query, selected-store, directive, output-bound, mixed-iterator, and
concurrent-cursor tests pass. Docker race checks pass. All 50 generated-query
grammar runs pass against locked C, one language per process. Go and JavaScript
semantics, overlapping capture streams, and TypeScript empty-node ranges pass.
The Go R4 invariant session passes all 72 steps and reports zero no-edit
allocations; existing compact and legacy no-edit tests also pass. Parser
scheduling and incremental logic remain unchanged.

An existing overlapping-pattern `NextMatch` ordering difference remains:
Go returns a parent's matches before descendant matches, while C can interleave
them by completion. The new `NextCapture` ordering agrees with C on that
fixture. This work does not claim complete query-language equivalence.

Raw complete-operation Go/C receipt SHA-256:

- Original: `d3e4c1a961ba9ed784fa71da5422ce216a423269b0ccb277c1085c35643cbdbd`.
- Final: `ce1d5c3821a9684e080594519786eae2eb4b5c2a7d9950b6e56ccb4d7a0de48f`.

The final paired Go-only comparison uses revision `92a3dfc8b` as its baseline,
20 alternating seeds on CPU 2, and the same wrapper settings:

METRIC: root quantified-query latency | 387.111 ms -> 2.526 ms | 92a3dfc8b + iterative root matcher | 4096-comment Go complete ExecuteInto operation

| Complete operation | Before time | After time | Before / after allocations |
| --- | ---: | ---: | ---: |
| Root run, 4,096 comments | 387.111 ms | 2.526 ms | 94,348 / 4,124 |
| Child run, 4,096 comments | 2.433 ms | 2.523 ms | 47 / 50 |
| Failed suffix, 32 comments | 13.14 us | 12.03 us | 8 / 4 |
| Failed suffix, 64 comments | 20.89 us | 18.06 us | 9 / 4 |
| Compiled Go highlight query | 24.92 ms | 24.36 ms | 35,031 / 13,524 |

Root timing improves with `p=0.000`; width-64 failed-suffix timing improves with
`p=0.001`. The other timing differences are not significant. Ordinary-query
bytes decrease 20.77% and allocations decrease 61.39%. The final candidate
stays inside the existing 10% allocation and timing ratchets. Deterministic
work and reuse counters do not regress, and no threshold changes.

Process-only peak RSS for the 314-byte witness is 54,164 KiB at `f9828512c`,
26,668 KiB for the final Go candidate, and 15,232 KiB for locked C. These runs
include parser/query setup and process startup, exclude compiler processes,
and execute one complete operation. Their cold single-operation times are
not used as performance-comparison evidence.

The audit directory was unavailable, so the fixture was recovered from the
recorded overseer revision. Source SHA-256:
`7923df8b87acab7c9897dbab3fe0076cce9349eefedca03946f0a0dffe1af521`.
Query SHA-256:
`bdd1453728db98db4c6e036531b89e639ab2d2b90de38b57d970b8fd6f698874`.

Raw final paired Go receipt SHA-256:

- Before: `97cdb441eaaaaab89cb4ee4b76cce82ad161b7ec2e9193c40a548e7ff8ef3507`.
- After: `32d4b345dc83f915a836da9fbb8304a76e3b241f4b0ef41991cddd1cb4d8dc6b`.
