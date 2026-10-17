# Numeric scanner fingerprint encoding: no measured YAML gain

Evidence date: 2026-10-16 UTC. Baseline: `86879374e37ef0028443121d1bf12214377653a6`.

A private numeric sequence encoder preserves the existing identity bytes and
passes scoped checks, but its YAML timing differences are inconclusive. It is
not promoted. A subsequent warm-loop census shows that these YAML parses use
pointer identities and bypass the numeric encoder. Main merges remain held
for 0.56.0.

## Experiment and qualification

The two-file internal prototype replaces recursive per-element encoding for
integer sequences with typed reads for numeric slices and fixed-kind reflective
reads for boxed arrays. Kind, nil, length, native-width signed/unsigned values
and little-endian encoding remain unchanged. Unusual values keep their existing
fallback. No identity caching, language dispatch or callback-check removal occurs.

Tests compare complete encoded bytes and final SHA-256 values against the old
recursive encoder, including prefixed destination buffers, varied capacity,
named types, unexported fields, nil/empty slices, signed/unsigned limits and
65,537-element cases. Normal/race/386 checks, core tests, root binding/mutation,
lifecycle and budget checks, focused YAML locked-C cases and Go/YAML 48-step
edit sessions pass. No-edit reparses allocate nothing. All **414 ledger rows**
and canonical Go digests/`Core.Work()` stay exact; raw counters do not increase.

## Twenty randomized YAML pairs

| Case | Median time before → after | Direction | p | B/op before → after | Objects |
| --- | ---: | ---: | ---: | ---: | ---: |
| Ordinary attempt/fallback | 193.685 → 193.104 µs | -0.30% | .525 | 16,760 → 16,760 | 37 → 37 |
| Owned R4 | 350.106 → 349.0385 µs | -0.30% | .862 | 12,872 → 12,872 | 109 → 109 |
| Owned 8 KiB stream | 1.3605095 → 1.3574355 ms | -0.23% | .086 | 44,312 → 44,313 | 336 → 336 |

All timing differences are inconclusive. The stream's one-byte median rounding
increase is retained. Route/completion columns stay exact. These measurements
qualify no YAML throughput improvement.

Runs use the repository randomized runner, **20 alternating shuffled pairs**,
seeds **22301–22320**, one process per seed, `GOMAXPROCS=1`, `-count=1`, 750 ms,
`-benchmem`, CPU 6, 6 GiB Docker and both locks. All samples complete without
OOM or timeout; source hashes, 3,783 native input hashes, order and deterministic
columns pass the audit. No Go timing, full-fleet, strict-census or RSS certificate
is claimed. The earlier slice-only variant is scoped-tested and counter-checked,
but remains untimed and unpromoted; its evidence does not replace final checks.

## Warm-loop census changes the next target

A separate observer runs exactly one parse after benchmark setup/warmup for
each case. It records **one**, **675** and **2,522** fingerprint calls for
ordinary, owned R4 and owned stream respectively. Every call uses
`*grammarruntime.YamlExternalScanner` through the existing pointer identity path.
It observes **zero** structured-value, formatted-fallback, numeric-slice or
numeric-array encoding calls during those loops. Route checks pass.

The observer does not alter identities or retain scanner references. Its hooks
and benchmark edits remain outside the checkout. This census is diagnostic
evidence, not timing, allocation, C or fleet certification. It explains why
the numeric encoder is outside the measured YAML hot path and directs the next
investigation toward the pointer fingerprint's stack frame. No validation
calls are waived.

Buckley's first full-diff review returns `invalid_completion` and is incomplete.
A smaller review of the complete production source establishes no concrete
defect, with explicit limits: the old implementation, tests, callers and runtime
are outside that report. Exact old bytes are established separately by tests.
Workspace review packets are removed. No prototype source is promoted.

Evidence: `gotreesitter-evidence/20261016-scanner-slice-encoding`, especially
`sequences/{pins.json,qualification-audit.json,precheck/audit.json,perf-audit.json,
review-audit.json,decision.json,census/audit.json}`. The qualified Go runtime and
ordinary YAML routing remain unchanged.
