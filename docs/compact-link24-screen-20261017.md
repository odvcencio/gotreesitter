# 24-byte graph-link screen — 2026-10-17

**Rejected: smaller records still slow compact throughput.** The private
24-byte format slows compact Go by 3.85–5.61% and owned YAML by 2.92–3.09%
against qualified runtime `5e52cf2a2644f3d38785513eebc39f8cf8a506b2`.
The active parser retains its 32-byte records. No engine, route, allowlist or
ledger baseline changes are promoted.

This follows the [direct 16-byte decoder screen](compact-packed-link-inline-screen-20261017.md).
Its single inline format stores a signed 32-bit score, a full 32-bit order,
flags and three full-width graph references. Wider values use an additional
24-byte cold record and retain all 64 score/order bits and all flags. The
large-index escape remains keyed by complete logical IDs. Unknown flags are
preserved; the existing graph-shape checks still reject invalid values.
Logical scratch records remain 32 bytes. Rollback, reset, retention, byte
accounting and diagnostic-clone isolation retain the tested storage contract.

The compiler inlines the encoder at cost 56 and decoder at cost 33, below its
budget of 80. Those smaller bodies do not establish a speed gain.

## Measurements

Positive values are slower. These are independent comparisons against the
qualified runtime; results from the 16-byte experiments are not combined.

| Workload | Time direction and p |
| --- | ---: |
| Go R4 | +5.06% (p=0.001) |
| Go rewrite | +5.50% (p<.001) |
| Go query | +4.97% (p<.001) |
| Go language | +5.61% (p<.001) |
| Go large | +3.85% (p<.001) |
| Ordinary YAML attempt + fallback | +0.13% (p=0.779) |
| Owned YAML R4 | +3.09% (p<.001) |
| Owned YAML 8 KiB | +2.92% (p<.001) |

Go R4 has p=.001, the four canonical Go fixtures p<.001, and both owned YAML
fixtures p<.001. Ordinary YAML's +0.13% direction is inconclusive (p=.779).
Its allocated bytes fall **15,608→14,840 B/op (-4.92%, p<.001)** with unchanged
allocation count. All other allocated-byte and allocation medians remain
exact. The saving does not offset the supported throughput regressions.
There is no RSS or retained-memory saving claim.

Default Go full/edit/no-edit directions are **-1.76% / -0.004% / +0.06%**
(p=.005 / .862 / .963). Only full parse is supported; the controls remain
unattributed. No-edit stays **0 B/op / zero allocations**.

## Correctness and work

All **414 ledger rows** and all five Go parse-and-consume vectors, including
deep digests, remain exact. The [equality evidence](benchmarks/20261017-link24/work-equality.json)
references the identical archived ledger/vector data and records hashes from
this run. Current native census logs are retained. No counter exception is
needed or applied.

Go/YAML locked-C fresh/incremental/error suites and Go/YAML/owned-YAML
72-step invariant sessions pass, with zero no-edit allocations. Full core
normal and phase-zero-A tests, focused core race/386, root publication/lifecycle/
memory and root race checks pass. Boundary tests cover both signed 32-bit
limits and their adjacent out-of-range values, 32-bit order limits, full
64-bit values, recovery and unknown flags, and maximum graph references.
The standalone codec and production append/decode path each pass 10,000
generated cases. Rollback, ID reuse, missing/misbound cold data, the index
escape, retention release and clone isolation tests also pass.

The all-language invariant fleet, full C fleet, strict census, RSS
qualification and full-consumption comparison timing were not run after this
rejection. Consumption is checked for exact vectors/digests and by untimed
single-iteration setup preflights.

## Evidence and next target

[Result and scope](benchmarks/20261017-link24/result.json),
[measurements](benchmarks/20261017-link24/measurement.json),
[source pins](benchmarks/20261017-link24/source-pins.json),
[complete private patch](benchmarks/20261017-link24/runtime.patch)
and [compiler excerpts](benchmarks/20261017-link24/compiler-inline-excerpts.txt)
retain the screen. Raw pairs, diagnostic source, scripts and validation logs
are beside them. Apply the patch with `git apply --unidiff-zero` in an isolated
baseline checkout and install the `.go.txt` diagnostics under the scripted
names. The archived scripts show the frozen-overlay lab.

Both language comparisons use 20 alternating shuffled pairs, one process per
seed, CPU 6, Go 1.25.14, `GOMAXPROCS=1`, `-count=1`, `750ms`, `-benchmem`,
serialized 6 GiB Docker and both benchmark locks. Go seeds are
**32901–32920**, YAML **33001–33020**. All **440 timed rows** are retained.

The next private experiment keeps the qualified 32-byte records and combines
a shift with its fresh-head owner publication in one owned operation. It
retains conflict and journal checks and keeps the original path for existing
heads, callbacks or canonical replacements. It has no throughput or
graduation claim in this receipt.
