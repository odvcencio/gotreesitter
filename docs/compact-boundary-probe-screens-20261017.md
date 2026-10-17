# Boundary-probe screens — 2026-10-17

**Both sources are rejected.** Caller-owned probe results establish no
supported compact gain. A separate first-slot inline path slows four Go
workloads **0.89–1.42%** and both owned YAML workloads **0.62–0.77%**.
No runtime or test file is promoted.

Both comparisons start at `5e52cf2a2644f3d38785513eebc39f8cf8a506b2`, the
qualified paired-terminal-cursor runtime. The earlier `a03d0842994a85aa26cf494c8c67404e1e64e999`
Go profile attributes about **320 ms / 12.91 s** to the condensation
boundary-probe call. Its Core and boundary-index files are byte-identical at
the comparison baseline. That
motivates testing result transport and call overhead; it does not predict a
measured gain. Separate run percentages are not combined.

## Caller-owned result

The first source prepares the full key and hash in caller-owned scratch. One
shared search loop writes the result instead of returning a complete probe
through intervening calls. Full state/offset/checkpoint/flag equality,
generation checks, growth re-probes and transaction journaling remain exact.
No cache, retained field or table-layout change is introduced. The new
collision/reuse test checks independent expected IDs, replacement counts,
generation changes and lookup immutability.

Go 1.25.14 confirms the initializer's **inline cost 53**, actual hot-call-site
inlining, and non-escaping scratch. The full search remains out of line at
**cost 104**. The [source proof](benchmarks/20261017-boundary-probe-screens/sink/probe-proof.json)
and compiler excerpts retain those observations.

| Path | Before → after | Time direction | p |
| --- | ---: | ---: | ---: |
| Go R4 | 799.091 → 808.348 µs | **+1.16%** | **.038** |
| Go rewrite | 1.795766 → 1.779690 ms | −0.90% | .355 |
| Go query | 8.250418 → 8.318927 ms | +0.83% | .327 |
| Go language | 8.690469 → 9.024985 ms | +3.85% | .355 |
| Go large | 90.008853 → 89.898567 ms | −0.12% | .583 |
| Ordinary YAML attempt plus fallback | 196.891 → 192.089 µs | −2.44% | .072 |
| Owned YAML R4 | 346.916 → 344.862 µs | −0.59% | .718 |
| Owned YAML 8 KiB | 1.355456 → 1.328569 ms | −1.98% | .698 |

Only the bold slowdown is supported. Several Go confidence intervals reach
12–14%; this source establishes no compact improvement. A post-run host
snapshot shows no current CPU or memory pressure, but it cannot establish
what caused spread during sampling. No cause is assigned.

## Inline first slot

The second independent source prepares the same probe, inlines the first
slot's generation and complete-key checks, and enters the remaining scan only
after a collision. It checks offsets 1 through n−1 once, in the original
masked order; a full-table miss restores the original zero slot. Reused
scratch still resets before lookup. No table representation, ownership,
checkpoint or journal rule changes.

The compiler confirms **first-slot cost 58**, actual condensation-call-site
inlining, and no escaping scratch. The initializer still inlines at cost 53.
The [bounded-order proof](benchmarks/20261017-boundary-probe-screens/first/first-probe-proof.json)
and compiler excerpts retain the exact scope. Those observations do not
translate into faster parsing.

| Path | Before → after | Time direction | p |
| --- | ---: | ---: | ---: |
| Go R4 | 782.764 → 791.818 µs | **+1.16%** | **.018** |
| Go rewrite | 1.707670 → 1.731956 ms | **+1.42%** | **.005** |
| Go query | 8.108269 → 8.201549 ms | **+1.15%** | **.002** |
| Go language | 8.469483 → 8.544544 ms | **+0.89%** | **.018** |
| Go large | 86.913145 → 87.018448 ms | +0.12% | .327 |
| Ordinary YAML attempt plus fallback | 189.762 → 190.067 µs | +0.16% | .301 |
| Owned YAML R4 | 334.872 → 336.954 µs | **+0.62%** | **.037** |
| Owned YAML 8 KiB | 1.299485 → 1.309437 ms | **+0.77%** | **.010** |

Only bold regressions are supported. This comparison has much narrower
timing intervals, generally 0–2%. It supplies no source promotion.

## Controls and deterministic gates

| Default Go control | Caller-owned result direction / p | Inline-first direction / p |
| --- | ---: | ---: |
| Full | −1.41% / .947 | −0.25% / .414 |
| First edit | −0.75% / .620 | +0.17% / .445 |
| No edit | **+1.70% / .038** | **−4.00% / <.001** |

Default-control movements remain unattributed. No-edit stays **0 B/op / zero
allocations**. All other allocation counts remain exact. Caller-owned Go
language bytes round **150,418 → 150,418.5 B/op**; inline-first rounds
**150,418 → 150,419.5 B/op**, and its owned-stream bytes round
**44,313 → 44,312.5 B/op**. These changes supply no memory claim. Complete
metrics, controls and raw samples are retained separately for the
[caller-owned](benchmarks/20261017-boundary-probe-screens/sink/measurement.json)
and [inline-first](benchmarks/20261017-boundary-probe-screens/first/measurement.json)
sources.

Each source preserves **412 ordinary plus two owned-YAML ledger rows**
byte-for-byte. Counter references link the complete published identical
values. Core normal, phase-zero-A, race and 386 tests, focused scheduler and
callback checks, Go/YAML/owned-YAML edit invariants, locked-C fresh/edit/error
checks, and YAML EOF/ownership and contextual-Go regressions pass. Baseline
boundary/fold/history/ownership/precedence/rollback tests run before candidate
checks. No counter exception or threshold change is used.

## Reproduction and limits

Both runs use the repository randomized runner, Go **1.25.14**, CPU 6,
`GOMAXPROCS=1`, 6 GiB Docker, `GOMEMLIMIT=5GiB`, `-count=1`, `750ms`, and
`-benchmem`. Each seed uses its own process; baseline/candidate order
alternates and both benchmark locks serialize the runs. Caller-owned Go/YAML
seeds are **31501–31520 / 31601–31620**; inline-first seeds are
**31701–31720 / 31801–31820**. All **880 timed rows** are retained.

Each source reference pins the shared 3,508-file baseline and all four
candidate files. Zero-context patches reproduce each measured source with
`git apply --unidiff-zero`. Runner/check scripts, input hashes, compiler
evidence, counter references and gate-log hashes/tails remain in the bundle.
Archived Go diagnostic shims retain `.go.txt` suffixes and must be restored
when reconstructing external overlays.

Neither rejected source receives full-fleet, strict-census or RSS
qualification. Qualification of their unchanged baseline is not transferred
to them. Ordinary YAML ownership, the Go throughput gap, E-A and graduation
remain open. Runtime, routing, allowlists and the global default stay unchanged.
