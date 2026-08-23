# C2 hygiene provenance plan

Evidence-only plan. It states exactly what must attach to the F0 hygiene
denominator before it is sealed-acceptable, per the unresolved conflict
`hygiene-provenance` in `docs/c0e-c0f-attribution.md` (line 55):

> F0 records 1000 ns as a fixed reporting policy, but the V10 metadata has no
> timer calibration or pre-run threshold record. Treat the F0 hygiene
> denominator as queryable evidence, not sealed C6f acceptance proof, until
> threshold provenance is attached.

## What is missing today

1. **No timer-calibration record.** The V10 scoreboard metadata carries no
   calibration of the Go and C timers used for `go_median_ns` /
   `c_median_ns` (`docs/v10-fleet-manifest.md` lines 5–12 record only source
   revision, scoreboard SHA-256, axis, and validation counts).
2. **No pre-run threshold declaration.** The 1000 ns hygiene threshold appears
   only as a fixed reporting policy at manifest generation time
   (`docs/v10-fleet-manifest.md` line 33: "The fixed F0 hygiene threshold is
   `1000 ns`. The threshold applies to reporting only."), i.e. after the fact,
   not as a pre-registered run parameter.
3. **Uncontrolled host.** The only published noise evidence is a local shared
   WSL2 A/A floor of 7.367%–10.803% p95 (`docs/c0e-c0f-attribution.md`
   line 22), explicitly "not a sealed-v9 noise floor". The fleet host's noise
   floor is unpublished.

## What must attach to make the denominator sealed-acceptable

| # | Artifact | Required content |
|---:|---|---|
| 1 | Timer calibration record | Per-timer (Go and C) calibration against a reference clock on the fleet measurement host, with method, sample count, and drift bound; taken in the same epoch as the scoreboard |
| 2 | Pre-run threshold declaration | The `timer_threshold_ns = 1000` value recorded as a run parameter *before* measurement, with author and timestamp, hash-bound into the scoreboard metadata |
| 3 | Host noise-floor receipt | An A/A null distribution from the fleet host itself (the local WSL2 floor cannot substitute — `docs/c0e-c0f-attribution.md` line 22) |
| 4 | Threshold-sensitivity statement | Which rows move across the 1000 ns boundary under plausible timer error; today 2 hygiene rows and 1,315 ratio-eligible rows depend on this cut (`docs/v10-fleet-manifest.md` lines 28–29) |

## Receipt skeleton

```json
{
  "schema": "gts-f0-hygiene-provenance/v1",
  "subject_scoreboard_sha256": "<sha256 of v10 scoreboard>",
  "threshold": {
    "timer_threshold_ns": 1000,
    "declared_before_run": true,
    "declared_at": "<ISO-8601 timestamp preceding epoch start>",
    "declared_by": "<author identity>"
  },
  "timer_calibration": {
    "host_id": "<fleet measurement host identifier>",
    "epoch": "<measurement epoch id matching the scoreboard>",
    "go_timer": { "reference_clock": "<clock>", "method": "<method>",
                  "samples": <n>, "drift_bound_pct": <pct> },
    "c_timer":  { "reference_clock": "<clock>", "method": "<method>",
                  "samples": <n>, "drift_bound_pct": <pct> }
  },
  "host_noise_floor": {
    "aa_null_runs": <n>,
    "p95_abs_delta_pct": { "min": <pct>, "max": <pct> }
  },
  "threshold_sensitivity": {
    "rows_at_cut": { "hygiene_below_1000ns": 2, "ratio_eligible": 1315 },
    "rows_moving_under_drift_bound": <n>
  },
  "seal": { "receipt_sha256": "<self-hash>", "signed_by": "<signer>" }
}
```

Fixed values already citable: `hygiene_below_1000ns = 2` and
`ratio_eligible = 1315` come from `docs/c0e-c0f-attribution.md` line 35
("2 hygiene rows; 1,315 ratio-eligible"). All other fields are unfilled until
the calibration lane runs.

## Acceptance rule

The F0 hygiene denominator becomes sealed-acceptable when items 1–4 above are
attached and hash-bound to the scoreboard they describe. Until then, any
number that depends on the 1000 ns cut (including the C0f signal row count and
the selection-formula denominator) remains queryable evidence, not sealed proof.
