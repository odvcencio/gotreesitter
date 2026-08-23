# C2 hygiene provenance plan

Evidence-only plan. It states exactly what must attach to the F0 hygiene
denominator before it is sealed-acceptable, per the unresolved conflict
`hygiene-provenance` in `docs/c0e-c0f-attribution.md` (line 55):

> F0 records 1000 ns as a fixed reporting policy, but the V10 metadata has no
> timer calibration or pre-run threshold record. Treat the F0 hygiene
> denominator as queryable evidence, not sealed C6f acceptance proof, until
> threshold provenance is attached.

## Epoch split: what V10 can still acquire vs what needs a future epoch

The V10 epoch has already run. That constrains what its hygiene denominator can
still acquire:

**Still acquirable for the already-run V10 epoch (post-hoc but epoch-bound):**

1. **Timer calibration record.** Per-timer (Go and C) calibration of the
   timers used for `go_median_ns` / `c_median_ns` against a reference clock on
   the fleet measurement host, with method, sample count, and drift bound.
2. **Host noise-floor receipt.** An A/A null distribution from the fleet host
   itself. The only published noise evidence today is a local shared WSL2 A/A
   floor of 7.367%–10.803% p95 (`docs/c0e-c0f-attribution.md` line 22),
   explicitly "not a sealed-v9 noise floor". The interleaved A/A method to
   replicate is documented at `docs/perf-attribution.md` lines 2900–2934
   ("Noise floor" protocol).
3. **Threshold-sensitivity statement.** Which rows move across the 1000 ns
   boundary under plausible timer error; today 2 hygiene rows and 1,315
   ratio-eligible rows depend on this cut (`docs/v10-fleet-manifest.md`
   lines 28–29).

**Only a future epoch can carry:**

- **Pre-run threshold declaration.** The 1000 ns hygiene threshold appears only
  as a fixed reporting policy at manifest generation time
  (`docs/v10-fleet-manifest.md` line 33: "The fixed F0 hygiene threshold is
  `1000 ns`. The threshold applies to reporting only."), i.e. after the fact,
  not as a pre-registered run parameter. Declaring it *before* measurement —
  with author and timestamp, hash-bound into the scoreboard metadata — cannot
  be done retroactively; it belongs to the next measurement epoch. The
  existing hook for this is the set-before-provisioning identity step in
  `docs/v10-gcp-fleet-measurement-runbook.md` line 26 ("Set these values
  before provisioning the machine").

## Hygiene denominator status

V10's hygiene denominator stays queryable evidence permanently. It can never
become sealed C6f acceptance proof for the pre-run-threshold property, because
that property is unattainable after the epoch has run; at most the post-hoc
artifacts above can bound how much the missing declaration could have moved.

## What must attach

| # | Artifact | Required content | Epoch |
|---:|---|---|---|
| 1 | Timer calibration record | Per-timer (Go and C) calibration against a reference clock on the fleet measurement host, with method, sample count, and drift bound; bound to the V10 epoch's scoreboard | V10 (post-hoc) |
| 2 | Pre-run threshold declaration | The `timer_threshold_ns = 1000` value recorded as a run parameter *before* measurement, with author and timestamp, hash-bound into the scoreboard metadata | future epoch only |
| 3 | Host noise-floor receipt | An A/A null distribution from the fleet host itself using the interleaved A/B method (`docs/perf-attribution.md` lines 2900–2934); the local WSL2 floor cannot substitute (`docs/c0e-c0f-attribution.md` line 22) | V10 (post-hoc) |
| 4 | Threshold-sensitivity statement | Which rows move across the 1000 ns boundary under plausible timer error; today 2 hygiene rows and 1,315 ratio-eligible rows depend on this cut (`docs/v10-fleet-manifest.md` lines 28–29) | V10 (post-hoc) |

Existing repo hooks to build on:

- Pre-run parameter recording: `docs/v10-gcp-fleet-measurement-runbook.md`
  line 26 — values are exported before provisioning the machine.
- Host-identity receipt: `docs/v10-gcp-fleet-measurement-runbook.md` line 343 —
  the receipt is stored with "the commit, lock digest, host identity, and
  regression summary".
- Interleaved A/A null method: `docs/perf-attribution.md` lines 2900–2934.
- Hash binding precedent: `docs/perf-attribution-receipt.json` binds its
  content to a source revision via the `git_commit` field
  (`f91e9c8cc7d2c8830c973cfa28b658c70dd0d0ba`).

## Receipt skeleton

```json
{
  "schema": "gts-f0-hygiene-provenance/v1",
  "subject_scoreboard_sha256": "620b4e98d532b0589adead4af4cdf1eccc3d7b85f5ada7a9e72137caa09137d5",
  "threshold": {
    "timer_threshold_ns": 1000,
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

Notes on the skeleton:

- `subject_scoreboard_sha256` is filled from `docs/v10-fleet-manifest.md`
  line 6 ("Source scoreboard SHA-256"). It applies to the already-run V10
  epoch.
- There is no `declared_before_run` field: that property is unattainable for
  the V10 epoch and would be false if asserted here. The `declared_at` /
  `declared_by` fields apply only when the skeleton is instantiated in a
  future epoch that declares the threshold before measurement.
- Fixed values already citable: `hygiene_below_1000ns = 2` and
  `ratio_eligible = 1315` come from `docs/c0e-c0f-attribution.md` line 35
  ("2 hygiene rows; 1,315 ratio-eligible"). All other fields are unfilled until
  the calibration lane runs.

## Two coexisting signal definitions

Two definitions of the C0f signal are in circulation:

1. `docs/c0e-c0f-attribution.md` line 26:
   `axis.status == "ok" and predicates.ratio_interpretable == true;
   timer threshold 1000 ns`.
2. The JSON selection formula (`formulas[0]`):
   `c_median_ns >= timer_threshold_ns`.

These are not identical: the first gates on the manifest's precomputed
`predicates.ratio_interpretable` flag, the second recomputes eligibility from
the raw `c_median_ns` against the threshold. The threshold-provenance work
depends on definition 2 (`formulas[0]`, `c_median_ns >= timer_threshold_ns`),
because it is the one whose output actually varies with
`timer_threshold_ns`; a sensitivity analysis against the threshold is only
meaningful under the raw-value comparison.

## Acceptance rule

The F0 hygiene denominator becomes sealed-acceptable when items 1, 3, and 4
above are attached and hash-bound to the V10 scoreboard they describe, and item
2 is carried by a future epoch. Until then, any number that depends on the
1000 ns cut (including the C0f signal row count and the selection-formula
denominator) remains queryable evidence, not sealed proof.
