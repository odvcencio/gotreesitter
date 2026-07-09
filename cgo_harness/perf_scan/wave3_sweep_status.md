# Wave 3 Perf Sweep Status

- generated_at: `2026-07-09T04:59:41Z`
- budget: `perf_scan/perf_ratio_budgets.json`
- fleet catalog: `tier_scan/exts.tsv`
- budget_generated_at: `2026-07-09T01:17:47Z`
- budget_generated_by: `wave-3 fleet perf sweep ratchet pass, branch wave3/perf-assisted-ratchets, extending the wave-2b budget with batch-1 through batch-7 plus the assisted fleet gap-close measurements`

## Coverage

| metric | value |
|---|---:|
| fleet languages | 206 |
| budgeted languages | 203 |
| held out languages | 3 |
| known budget class gaps | 4 |
| wave2b pending budget rows | 15 |
| measured-today budget rows | 193 |
| partial measured-today notes | 1 |

Measurement basis: `reps=5`, `warmup=1`, `file_budget_ms=10000`, `max_files=8`, `order=largest`, `axes=full,noedit`.

Held out of the ratchet: `d`, `fsharp`, `groovy`.

## Budget Status Counts

| status | languages |
|---|---:|
| `green` | 50 |
| `green_with_caveat` | 142 |
| `wave2b_pending` | 11 |

## Known Gap Ledger

| key | file | action |
|---|---|---|
| `d_docker_oom` | d corpus largest-file selection from wave3_batch6 | dedicated one-language D RCA under a hard disposable memory limit before creating any budget row |
| `fsharp_providedtypes_c_reference_memory_blowup` | fsharp/examples/FSharp.Compiler/tests/EndToEndBuildTests/ProvidedTypes/ProvidedTypes.fs (755275 bytes; first active selected file after largest-8 selection) | dedicated isolated C-reference containment or corpus-selection RCA on ProvidedTypes.fs; do not rerun fsharp broad sweeps without a disposable hard memory bound and process-level watchdog |
| `groovy_pleac11_15_memory_blowup` | groovy/subprojects/performance/src/files/pleac11_15.groovy (102960 bytes, largest-file selection hit during the assisted fleet pass) | dedicated RCA on pleac11_15.groovy in an isolated hard-bounded container; do not sweep this file again as part of broad fleet runs until containment is understood |
| `webworker_generated_d_ts` | typescript/src/lib/webworker.generated.d.ts (786262 bytes, largest .d.ts in the corpus sample) | typescript's full_axis budget above is intentionally NOT tightened to reflect a 'fixed' webworker.generated.d.ts; GOT_FAITHFUL_CONDENSE (or an equivalent default-budget-aware condense path) remains a real wave-2b item. |

## Seed Sources

- `after_cliffs_20260706T210143Z`
- `authoritative_20260706T145520Z`
- `fleet_gap_close_assist_20260708T232019Z_to_20260709T003453Z`
- `pr142_evidence_and_wave2a_closeout_spore`
- `wave2b_green_20260707T0100Z`
- `wave2b_js_20260707T0000Z`
- `wave3_batch1_20260708T202820Z`
- `wave3_batch2_20260708T211248Z`
- `wave3_batch3_20260708T213644Z`
- `wave3_batch4_20260708T215550Z`
- `wave3_batch5_20260708T222227Z`
- `wave3_batch6_20260708T224850Z`
- `wave3_batch6_dot_20260708T225933Z`
- `wave3_batch6_doxygen_20260708T230104Z`
- `wave3_batch7_20260708T232544Z`
- `webworker_oracle_spotcheck`

## Caveats

- The perf ratio budget is a ratchet and evidence ledger, not a universal near-C claim; >2x and cliff rows remain explicit backlog.
- d, fsharp, groovy are intentionally held out of the language ratchet until their memory/C-reference RCA rows are resolved.
- The TypeScript webworker generated-file entry remains a correctness cross-check caveat even though TypeScript has a timing budget row.
