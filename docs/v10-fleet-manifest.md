# V10 fleet manifest

This manifest is derived from the accepted V10 scoreboard. It does not change parser admission or routing.

- Source revision: `5003ffba01e2aee44043e71360c00f5aa93e6e8b`
- Source scoreboard SHA-256: `620b4e98d532b0589adead4af4cdf1eccc3d7b85f5ada7a9e72137caa09137d5`
- Source epoch: `2026-08-08T21:04:47Z`
- Axis: `full`
- Manifest generated at: `2026-08-11T22:20:00Z`
- Validation: `true` (206 languages, 1435 rows)

## Class and timing denominator

The manifest keeps semantic classes and timing hygiene separate.

| Predicate | Count |
|---|---:|
| Clean rows | 876 |
| Error rows | 534 |
| Stopped rows | 25 |
| Measured rows | 1317 |
| Non-measured rows | 118 |
| Raw C median present | 1412 |
| Raw C median missing | 23 |
| Measured rows with raw C median | 1317 |
| Measured rows missing raw C median | 0 |
| Non-measured rows with raw C median | 95 |
| Ratio-noninterpretable rows | 2 |
| Ratio-interpretable rows | 1315 |
| Clean rows above 3.0x | 808 |
| Clean rows above 3.0x after hygiene | 806 |

The fixed F0 hygiene threshold is `1000 ns`. The threshold applies to reporting only.

## Gate findings

The 386 gate findings are predicates. They can overlap the same row.

| Finding kind | Records |
|---|---:|
| `coverage` | 110 |
| `go_stop` | 25 |
| `oracle` | 1 |
| `ratio` | 250 |

## Source gate

- Status: `fail`
- Files expected: 1435
- Files measured: 1435
- Full files evaluated: 1317
- Failure records: 386

## Reproduce

Run `go run ./cgo_harness/cmd/perf_scan_manifest -scoreboard <v10-scoreboard.json> -expected-languages 206 -expected-rows 1435 -timer-threshold-ns 1000 -clean-ratio-threshold 3.0 -out-json docs/v10-fleet-manifest.json -out-md docs/v10-fleet-manifest.md`.
