# Pointer fingerprint frame split: no measured throughput gain

Evidence date: 2026-10-16 UTC. Baseline: `86879374e37ef0028443121d1bf12214377653a6`.

Moving generic scanner encoding into a helper shrinks the pointer wrapper's
compiled stack allocation, but the twenty-pair YAML screen finds no measurable
throughput improvement. The one-file prototype is not promoted. Main merges
remain held for 0.56.0.

The [warm-loop census](compact-scanner-encoding-screen-20261016.md) records
675 / 2,522 pointer fingerprints in owned R4 / stream parses. The native
`ScannerFingerprint` function also contains a 4 KiB scratch array for value
encoding. This experiment outlines that unchanged generic body; pointer/nil
identity, value encoding, fallback and every caller validation remain unchanged.
No unsafe sequence encoder, observer hook or new test carries into this version.

On the pinned amd64 compiler, disassembly shows the wrapper's explicit SP
allocation **4,304→152 bytes**. Both wrappers remain outside the inline budget:
cost **429→227**, budget 80; the generic helper costs 264. These are compiler
facts, not runtime speed or RSS savings. A source audit confirms the generic
body is textually unchanged.

Before timing, existing binding/value/pointer/mutation tests pass normally,
under race and on 386. Core, root ownership/lifecycle/budget checks, focused
YAML locked-C cases and Go/YAML 48-step edit sessions pass; no-edit allocations
stay zero. All **414 ledger rows**, canonical Go digests and `Core.Work()`
vectors remain exact; raw counters do not increase.

| Case | Median time before → after | Direction | p | B/op | Objects |
| --- | ---: | ---: | ---: | ---: | ---: |
| Ordinary attempt/fallback | 194.1225 → 194.4735 µs | +0.18% | .092 | 16,760 → identical | 37 → identical |
| Owned R4 | 350.397 → 350.322 µs | -0.02% | .478 | 12,872 → identical | 109 → identical |
| Owned 8 KiB stream | 1.3631895 → 1.3633285 ms | +0.01% | .904 | 44,313 → identical | 336 → identical |

All timing differences are inconclusive. Allocation medians and timed route/
completion columns remain exact. These results do not qualify a YAML win.

The repository randomized runner completes **20 alternating shuffled pairs**,
seeds **22501–22520**, one process per seed, `GOMAXPROCS=1`, `-count=1`, 750 ms,
`-benchmem`, CPU 6, 6 GiB Docker and both locks. Source hashes, 3,783 native
input hashes, sample/order and work audits pass; no OOM or timeout occurs.
No Go timing, full-fleet, strict-census or RSS qualification is claimed.

Buckley's completed source-only review establishes no concrete defect in the
supplied diff and candidate. Callers and execution/allocation claims are outside
that report; it is not promotion approval. The workspace packet is removed.
Evidence: `gotreesitter-evidence/20261016-scanner-pointer-frame`, especially
`compiler-audit.json`, `qualification-audit.json`, `precheck/audit.json`,
`perf-audit.json`, `review-audit.json` and `decision.json`. Runtime source,
ordinary YAML routing and graduation status remain unchanged.
