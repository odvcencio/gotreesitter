# Compact suffix reuse: correctness and cost

Starting compact revision: 45dcc1fa5454299be11f0728d6dafd56ef4a43b9.
Repaired legacy revision: 90bdc698d898f7ebb5e41e6bb54844224e480f16.

**Status: HOLD for general replacement. The required lifecycle cost gates now pass.**


## What changed

Compact rejected clean descendants when an ancestor carried ambiguity fragility.
The C parser rejects the fragile candidate, then considers its clean descendants.
A fresh C probe confirmed that grouped Go parameters make the function fragile.
Clearing that function flag would remove real ambiguity evidence.

The new proof authenticates the common source suffix through EOF.
Reverse edit mapping binds each candidate to the same old source interval.
Exact pre-goto state, target state, and the fresh left token remain required.
Own-node fragility, missing nodes, explicit recovery ancestors, and changed projections still prevent borrowing.

The source scan runs once per incremental attempt and checks cancellation every 4 KiB.
Its worst-case work is linear in source length, not edit length.
The proof stores two positions and two flags. It allocates no per-node map.
Forward-only lexer behavior is required. Stateful scanners, contextual prefix wrappers, included ranges, and UTF-16 cannot use this proof.

An uncertified active head can decline a nested candidate without poisoning its owned transaction.
The decline publishes no borrowed payload. A later certified head can continue in the same transaction.
Cancellation, invalid metadata, corrupted payloads, and invalidated certificates remain fatal.

## Correctness

The full compact core suite passed: 794 test and subtest outcomes.
The final focused root suite passed 144 outcomes.
Cancellation, memory-budget release, and the build without compact parsing also passed.
Focused tests cover changed later lookahead, shifted and repeated source, multiple edits, EOF, cancellation, and cache reuse.
Negative cases retain scanner, range, encoding, state, token, missing-node, and recovery barriers.

The three required histories passed all 60 native edits against fresh C.
Length change also passed all 20 edits natively.
Parent navigation passed after releasing every prior tree in those four histories.
A small grouped-parameter fixture retains clean descendant identities beneath the fragile function.

The first broader comparison matched all starting outcomes except the former fallback-accounting fixture.
Length change now stays native, so that fixture could no longer test a decline.
The accounting test now uses deletion recovery and passes with the same strict accounting assertions.
The final broader Go run records 423 passing and 30 shared failing outcomes.
It adds two passing outcomes and changes no existing outcome after the accounting-fixture update.
JavaScript changed-lookahead reuse also matched the starting result.
The strengthened descendant-identity fixture fails at 45dcc1fa and passes with this change.

Deletion still falls back. Its existing parent-link failure remains.
Shared forest, production, probe, and range failures remain outside this change.

## Cost against the starting compact candidate

Each operation includes compact initial parsing, four alternating edits, and every tree release.
Parser and grammar construction remain outside timing.
The protocol uses 20 paired, alternating shuffle seeds and a fresh process for each lane and seed.
Each benchmark uses 750 ms, GOMAXPROCS=1, one Docker CPU, and CPU 18.
The host is chi-1, with an Intel Core Ultra 9 285 and Go 1.25.14.
The container has an 8 GiB limit and a 6 GiB Go memory limit.
The host is shared. These intervals describe this pinned process protocol, not cross-machine uncertainty.

| History | Starting ms | New ms | Paired time ratio [95% CI] | Allocated-byte ratio | Allocation-count ratio |
| --- | ---: | ---: | ---: | ---: | ---: |
| Token-class change | 27.59 | 18.18 | 0.661 [0.630, 0.692] | 0.242 | 0.744 |
| Length change | 25.95 | 20.59 | 0.791 [0.752, 0.830] | 0.038 | 1.115 |
| Early newline | 249.27 | 188.62 | 0.746 [0.699, 0.793] | 0.673 | 0.416 |
| Prefix call/conversion | 66.41 | 45.28 | 0.672 [0.633, 0.705] | 0.390 | 0.355 |
| Deletion recovery | 10.18 | 9.90 | 0.937 [0.858, 1.014] | 0.996 | 1.000 |

Ratios are geometric means of paired measurements. Displayed lane costs are sample medians.
Confidence intervals use 50,000 paired bootstrap samples with a fixed random seed.
Benchstat confirms significant time improvements for the four native histories.
Deletion timing does not differ significantly.
Length change uses about 11.5% more allocations, despite much lower allocated bytes and wall time.

## Parser work across four edits

| History | Starting tokens | New tokens | Starting new nodes | New nodes |
| --- | ---: | ---: | ---: | ---: |
| Token-class change | 2,520 | 544 | 5,900 | 900 |
| Length change | 1,109 | 1,128 | 1,940 | 1,620 |
| Early newline | 13,940 | 3,323 | 29,976 | 4,684 |
| Prefix call/conversion | 5,692 | 1,271 | 12,216 | 1,720 |
| Deletion recovery | 1,323 | 1,323 | 4,919 | 4,919 |

Initial parsing is included in wall time but excluded from this work table.
Token and node counts remain constant across measured operations.
Length change now performs four native edits instead of four legacy edits.
Its slightly higher token count is a measured tradeoff.

## Cost against repaired legacy

The same paired protocol compares the new candidate with repaired legacy at 90bdc698.
Both lanes still start each operation with compact initial parsing.
Only the four incremental edits select the lane implementation.

| History | Legacy ms | New ms | Paired time ratio [95% CI] | Allocated-byte ratio | Allocation-count ratio |
| --- | ---: | ---: | ---: | ---: | ---: |
| Token-class change | 22.84 | 21.25 | 0.926 [0.882, 0.973] | 0.087 | 0.734 |
| Length change | 22.97 | 23.19 | 1.016 [0.965, 1.064] | 0.172 | 0.577 |
| Early newline | 251.27 | 223.82 | 0.938 [0.892, 0.987] | 0.477 | 0.545 |
| Prefix call/conversion | 58.33 | 49.73 | 0.861 [0.809, 0.913] | 0.195 | 0.493 |
| Deletion recovery | 10.96 | 11.41 | 1.009 [0.961, 1.062] | 1.449 | 0.769 |

All three required time intervals remain below the 1.10 ceiling.
Their upper bounds also remain below 1.00 in this run.
Length change meets the same timing ceiling and has lower allocation bytes and counts than legacy.
Deletion timing meets the ceiling, but allocated bytes remain about 45% above legacy.
Deletion still fails native execution and public parent navigation.

The two rounds have different lane medians. Pairing controls each comparison within its own round.
Do not compare absolute medians across rounds as another performance result.

## Memory and retention

Five fresh processes per lane ran three small/history/small cycles for each history.
Each workload used four edits. Every heap snapshot followed two full garbage collections.
These heap values include the grammar, fixtures, and process caches.

| History | Legacy released MiB | New released MiB | Ratio | Legacy overlap MiB | New overlap MiB |
| --- | ---: | ---: | ---: | ---: | ---: |
| Token-class change | 13.635 | 9.766 | 0.716 | 14.111 | 9.774 |
| Length change | 16.087 | 13.068 | 0.812 | 16.970 | 13.075 |
| Early newline | 64.869 | 55.255 | 0.852 | 68.251 | 55.387 |
| Prefix call/conversion | 39.551 | 33.163 | 0.838 | 41.246 | 33.170 |
| Deletion recovery | 34.692 | 33.298 | 0.960 | 34.699 | 33.305 |

Released means all trees were released while the parser remained live.
Overlap means both old and new trees remained live after the fourth edit.
All required released-heap ratios remain below 1.10.

Baseline parser-process peak RSS: median 107.68 MiB; range 107.40–107.80 MiB.
Candidate parser-process peak RSS: median 84.82 MiB; range 84.55–85.82 MiB.

These process RSS values exclude compilation. Docker runner peaks include build activity and are reported separately.

A separate 20-cycle run reached a plateau in both lanes for all five histories.
The last five released-heap measurements were identical within each lane and history.
Draining pools returned each process to approximately 3.28 MiB.
This longer run uses one process per lane and drains pools between histories.
Its absolute heap levels must not be combined with the five-process run.

## Profiling limits and next cost targets

CPU and allocation profiles cover complete four-edit lifecycles, including initial parsing.
Scheduler footprint accounting, dispatch, reductions, tree normalization, and reused-tree traversal remain visible costs.
The larger reuse gain does not remove those costs.
The source proof adds one bounded scan per edit; future measurements can assess whether a retained source index is worthwhile.
No profile percentage is a standalone incremental-only timing result.

## Remaining gates

The required wall-time, allocation-byte, and released-heap gates pass in this run.
Graduation still requires recovery, scanner checkpoint, range, public API, and retirement evidence.
No legacy entry point was removed.

## Reproduction and evidence

harness_out/reuse-eligibility-20260909 contains both performance rounds, memory runs, profiles, and correctness receipts.
control/run-perf.sh compares against 45dcc1fa. legacy/run-perf.sh compares against repaired legacy at 90bdc698.
Both scripts use scripts/run_randomized_benchmarks.sh with authenticated build-once binaries.
The runtime source hashes match before and after all measurements.
The archive includes source copies, the patch, analyzer scripts, raw samples, and Docker metadata.
Temporary diagnostics and the temporary control checkout were removed after their source copies were verified.

The maintained BenchmarkGoCompactFourEditLifecycle now covers four histories and two explicit lanes.
Its eight one-iteration checks validate routes and counters. Those checks are not timing comparisons.
The maintained native-edit test now requires 80 fresh-C and parent-navigation checks.

## Next graduation work

1. Fix deletion recovery through borrowed compact subtrees. Verify 20 native edits and parent navigation.
2. Prove scanner checkpoint transfer before admitting stateful incremental scanners.
3. Complete included-range recovery and resolve the shared range fixtures.
4. Resolve the shared forest, production, and probe failures.
5. Complete the public API and broader language replacement matrix.
6. Re-run campaign costs before retiring the legacy implementation.

The required three-history performance gap is closed by this candidate in this protocol.
That result supports continued graduation work. It does not prove general parser replacement.
