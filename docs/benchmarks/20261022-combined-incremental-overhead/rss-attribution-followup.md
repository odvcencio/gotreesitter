# RSS attribution and rejected warm-reservation experiment

PR #1509 stays draft. Honoring a smaller learned node-capacity hint lowers the median RSS on the held 1 MiB default-Go witness, but 9 of 20 candidate processes still exceed the hard ceiling. The runtime change is not promoted. These results do not qualify a throughput win or compact graduation.

## Source and workload

The allocation profiles compare main `afd16867b666dfec05f3eb28e9eee86036474532` with PR #1509 at `f10cc738aad78d1da6130717f140e14363bb842b` (runtime `4de229c4b1718989917c6212f92cf74df28cf229`). All 3,541/3,544 tracked Go files in the read-only source trees match their pinned Git trees after the run. The benchmark overlay and its hash are retained. Profiles use Go 1.26.6 on native Linux amd64 in Docker, with no forced GC. Instrumentation affects allocation and timing; profile results identify allocation sites only.

The separate reservation experiment compares the same held runtime with one private `parser_limits.go` overlay, SHA-256 `57b9ecb656a352e76bfd9ac80749c5a4e5d4e1745db3b195a6a25683175c7ad0`. The source is 1,048,591 bytes, SHA-256 `76c70b62127085d3c791ba98a24cba9f22bd343e40afb43522bbb5041cede524`. Every RSS process reports the same warmed default-route work: 993,391 nodes, 425,742 tokens, and at most two stacks. Both variants use the same fixed two measured iterations, including the benchmark framework's warm-up and calibration behavior.

## Allocation finding

The baseline allocation profile attributes 59.85% of sampled cumulative allocation to `ensureExactNodeCapacity`; the live profile attributes 62.92% to it. The candidate profile still attributes 60.35% of cumulative allocation to the same site. A primary array reserves 1,048,591 public Node headers, 109,053,464 bytes, while the inspected arena uses 928,887 slots. The public Node remains 104 bytes on this architecture. These sampled percentages and live capacities are diagnostic evidence, not a before-and-after performance comparison.

The cold byte-length estimate overrides a smaller learned usage hint. The private experiment lets a hint within 25% of the cold estimate reduce that reservation, leaving overflow handling, pool policy, retention limits, budgets, Node layout and work limits unchanged. On the C-preflight witness, the warmed default-route arena reservation changes from 167,007,344 to 160,102,656 bytes. Compact's separate capacity policy is unchanged.

## Qualification and rejection

Before RSS measurement, twelve full public-tree comparisons against locked C pass, including three warm rounds on both configured routes for each variant. The candidate passes four Go/YAML R4 sessions (288 edit steps; zero no-edit allocations), twenty memory-contract tests and focused arena/density tests. The Go/YAML deterministic ledger exports are byte-identical. The Mac repeats the native unit, invariant, memory and ledger gates before measurement.

The unprofiled RSS run uses native Linux arm64 on the M1 Max, Go 1.25.14, GOMAXPROCS=1, a precompiled parser test binary, and direct-child `wait4`/rusage. Twenty matched seeds 155001–155020 alternate baseline/candidate order. Compilers run outside the measured process; there is no manual GC. This is a peak-RSS experiment with a fixed iteration count, not a randomized throughput comparison.

| RSS, bytes | Held runtime | Smaller warm reservation |
| --- | ---: | ---: |
| Median | 431,073,280 | 399,409,152 |
| Maximum | 462,716,928 | 462,479,360 |
| Processes above 419,436,400-byte ceiling | 10 / 20 | 9 / 20 |

The median falls 7.35%, but the maximum barely changes and the absolute gate fails. Source manifests are identical before/after and match the pinned held runtime plus the one-file overlay. Every measured parse passes its completion assertions, and the containers report no OOM. The RSS benchmark reports counters from its warm parse; it does not separately assert per-iteration work. The independent C preflight checks all three full-parse rounds against the expected work. Retain the result as an unsuccessful attempt to clear the existing RSS hold. Investigate representation and allocation lifetime; do not loosen the ceiling or pool contracts.

The [archive metadata](rss-attribution-followup.json) binds every member of the [raw evidence archive](rss-attribution-followup.tar.gz), including profiles, controls, source hashes, C logs, deterministic counters, native gates and all forty RSS samples. The established full correctness and timing receipts remain in the parent [README](README.md). Cognociente's October 21 malformed-delete C mismatch and Windows ARM64 performance report remain unresolved.
