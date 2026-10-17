# Owned-only retry flags: reject activation after throughput checks

Evidence date: 2026-10-16 UTC. Baseline: `86879374e37ef0028443121d1bf12214377653a6`.

Restricting packed flags to owned snapshots cuts owned YAML allocation bytes
**11.19–12.78%** and objects **13.76–17.56%**, but slows all three YAML cases.
The prototype is rejected; no runtime source, route, allowlist or default is
promoted. Main merges remain held for 0.56.0.

## Boundary and scoped correctness

The earlier [global packed-mask screen](compact-packed-retry-screen-20261016.md)
changed live DFA flags and regressed ordinary YAML. This experiment leaves the
live DFA and legacy rollback snapshot source unchanged. A new internal immutable
mask belongs only to owned snapshots: two scalar words for at most 128 flags,
private opaque backing for larger masks, and no mutation API. Capture copies
the original values; restore writes caller-owned boolean storage.

The final boundary adds a separate 24-byte mask to the existing owned record,
growing its amd64 logical size **400→424 bytes**. The full record and retained
private backing remain charged. The legacy retry slice is nil in owned records;
validation rejects mutable-slice injection. Active masks still affect equality,
while stale masks remain ignored. Checkpoint, scanner binding, generation and
callback checks remain intact. New engine representation code starts in
`internal/parsercorephase0`; root changes adapt capture, restore, equality and
footprint accounting.

Before timing, the exact final boundary passes mask reference/ownership tests,
concurrent immutable-copy tests, normal/race/386 checks, core tests, focused root
ownership and rollback checks, lifecycle and memory-budget checks, and a 386
library build. Full flag lengths include **65,537**, and mutable restore output
cannot change a frozen mask. Focused locked-C YAML cases and Go/YAML **48-step**
edit sessions pass, with no-edit reparses at zero allocations.

All **414 ledger rows** match baseline: 412 ordinary rows and two owned YAML
rows. Four canonical Go deep digests and complete `Core.Work()` vectors stay
exact; raw counters do not increase. All timed route/completion columns stay
exact. No counter exception is requested.

## Twenty randomized pairs

| Case | Median time before → after | Time direction | p | B/op before → after | Objects before → after |
| --- | ---: | ---: | ---: | ---: | ---: |
| Ordinary attempt/fallback | 193.0365 → 198.1715 µs | +2.66% | <.001 | 16,760 → 16,760 | 37 → 37 |
| Owned R4 | 348.6385 → 356.9165 µs | +2.37% | .001 | 12,872 → 11,432 | 109 → 94 |
| Owned 8 KiB stream | 1.368303 → 1.386054 ms | +1.30% | <.001 | 44,313 → 38,648 | 336 → 277 |

These are complete-candidate measurements. Unchanged live lexer source does
not establish unchanged runtime cost; this screen supplies no causal attribution
for the ordinary-path regression. Owned memory savings do not qualify any of
the throughput cases or remove the ordinary YAML graduation blocker.

The repository randomized runner uses **20 alternating shuffled pairs**, seeds
**22101–22120**, one process per seed, `GOMAXPROCS=1`, `-count=1`, 750 ms and
`-benchmem`, CPU 6, 6 GiB Docker and both locks. All 60 samples per role complete
without OOM or timeout. Audits verify source hashes, 3,783 native input hashes,
pair orders and deterministic columns. No Go timing, full 206-session fleet,
strict 204-file census or large-file RSS qualification is claimed.

## Review and excluded prototype

Buckley's completed source-only review establishes no concrete defect in the
supplied complete diff. It does not review surrounding repository code or
independently run checks, and is not approval for promotion. The frozen packet
and report remain in evidence; the ignored workspace packet is removed.

An earlier private version duplicated the full 27-field DFA record in the root.
It passed scoped checks but conflicted with the internal-first direction in D9.
It was superseded before counters or timing. Its results are not transferred to
the final boundary; the latter runs fresh checks. Neither version is promoted.

The next investigation targets fingerprint encoding cost while retaining exact
identity bytes and mutation detection. Callback authentication is not waived.
Evidence: `gotreesitter-evidence/20261016-owned-retry-flags`, especially
`boundary/{pins.json,qualification-audit.json,precheck/audit.json,perf-audit.json,
review-audit.json,decision.json}`. The qualified Go runtime remains unchanged.
