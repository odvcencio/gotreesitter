# Packed graph-link storage screen — 2026-10-17

**Rejected: smaller graph records slow compact throughput.** The private
integration loses 6.60–7.99% on the five compact Go parses and 3.82–4.05% on
owned YAML. The active runtime remains
`5e52cf2a2644f3d38785513eebc39f8cf8a506b2`; no runtime, route, allowlist or
ledger baseline changes are promoted.

## Representation

This integrates the [component codec proof](compact-packed-link-codec-proof-20261017.md)
into a private overlay. The main graph arena uses 16-byte records with all
three references still 32 bits. Zero scores and 30-bit orders fit one inline
format; signed 16-bit scores and 13-bit orders fit another. Logical scratch
records remain 32 bytes.

Other values retain their complete score, order and flags in a 24-byte cold
record, in addition to the 16-byte main record. A large-index escape searches
cold records by full logical ID instead of imposing a new graph-size limit.
Cold metadata is append-only. Rollback derives its retained prefix from the
existing logical-link checkpoint; reset and release track both arenas.
Storage and footprint include cold metadata, and the diagnostic EOF clone
copies, accounts for and checks the disjointness of that metadata.

## Randomized comparison

Positive values are slower. All compact-parse differences below have
p<.001; ordinary YAML has p=.049. Results are separate from earlier screens.

| Workload | Time direction and p |
| --- | ---: |
| Go R4 | +6.60% (p<.001) |
| Go rewrite | +7.19% (p<.001) |
| Go query | +6.75% (p<.001) |
| Go language | +7.99% (p<.001) |
| Go large | +6.75% (p<.001) |
| Ordinary YAML attempt + fallback | -0.66% (p=0.049) |
| Owned YAML R4 | +4.05% (p<.001) |
| Owned YAML 8 KiB | +3.82% (p<.001) |

All five full-tree-consumption workloads also slow, by **5.78–7.04%**
(all p<.001). Their preflight digests/checksums match, and every timed parse
must preserve compact routing, input coverage, field/parent reads and checksum.

Ordinary YAML allocates **15,608→14,072 B/op (-9.84%, p<.001)**, with the same
allocation count. Its timing improves **0.66% (p=.049)**. This does not offset
the supported compact Go and owned-YAML regressions. Other byte medians are
exact except a one-byte Go-consumption rounding movement; all allocation
counts are exact. The 16-byte representation supplies no RSS or general
retained-memory saving claim.

Default Go full/edit/no-edit directions are **-0.73% / +1.36% / -0.49%**
(p=.001 / <.001 / .279). These remain unattributed; the no-edit direction is
inconclusive and it stays **0 B/op / zero allocations**.

## Correctness and work

All **414 ledger rows** are exact, including routes, node/token work, peak
versions and reuse. All five Go parse-and-consume vectors and deep digests are
exact. No ledger exception is needed or applied.

The candidate passes full core tests in normal and phase-zero-A builds,
focused race/386 tests, root publication/lifecycle/memory checks and root race,
Go/YAML locked-C fresh/incremental/error suites, and Go/YAML/owned-YAML
72-step invariant sessions with zero no-edit allocations. New storage tests
cover complete signed/unsigned values, unknown/recovery flags, missing or
misbound cold metadata, the large-index escape, rollback, ID reuse, reset,
retention release and diagnostic-clone isolation. The component tests retain
their boundary and 10,000 generated cases.

Initial build failures (one missing frozen-overlay path and one unconverted
summary reader) and a stale recovery byte assertion are retained. The assertion
now uses the packed arena width: its operation accounts for 136 instead of
152 bytes. No parser correctness assertion is weakened. The full all-language
invariant fleet, full C fleet, strict census and RSS qualification were not run
after the timing rejection.

## Attribution and next target

The compiler reports `graphLink` at **194** and `appendGraphLink` at **140**
inlining cost against a budget of **80**; the inline packer costs **94**.
The candidate's sampled Go R4 profile attributes **4.00% cumulative** time to
`graphLink`. Copying also appears prominently. These observations point to
accessor and encoding overhead; they do not establish a complete causal
breakdown or validate a replacement.

The next revision should preserve the tested storage semantics while keeping
the common access/append path small enough to inline, then repeat the complete
counter-first randomized screen. Profiles use separate three-second runs for
attribution only; their fixed-order timings are not comparison evidence.

## Evidence and reproduction

[Result and scope](benchmarks/20261017-packed-link-storage/result.json),
[all measurements](benchmarks/20261017-packed-link-storage/measurement.json),
[construction vectors](benchmarks/20261017-packed-link-storage/construction.json),
[counter gate](benchmarks/20261017-packed-link-storage/counter-gate.json),
[source pins](benchmarks/20261017-packed-link-storage/source-pins.json),
and the [complete private patch](benchmarks/20261017-packed-link-storage/runtime.patch)
retain the screen. Raw rows, benchmark sources, scripts, validation logs,
compiler excerpts and CPU profiles are beside them.

The comparison uses twenty alternating shuffled pairs, one process per seed,
Go 1.25.14, CPU 6, `GOMAXPROCS=1`, `-count=1`, `750ms`, `-benchmem`,
serialized 6 GiB Docker and both benchmark locks. Go seeds are
**32301–32320** and YAML seeds are **32401–32420**; all **640 timed rows** are
retained. Apply the patch with `git apply --unidiff-zero` in an isolated baseline
checkout and install the diagnostic `.go.txt` files under their scripted test
names. The archived scripts show the original frozen-overlay lab. Keep parser
correctness and counters ahead of timing. Single-iteration consumption
preflights and profiling runs supply no before-and-after comparison evidence.
