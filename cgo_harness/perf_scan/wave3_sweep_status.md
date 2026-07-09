# Wave 3 Perf Sweep Status

- generated_at: `2026-07-09T10:50:28Z`
- budget: `perf_scan/perf_ratio_budgets.json`
- fleet catalog: `tier_scan/exts.tsv`
- budget_generated_at: `2026-07-09T10:38:34Z`
- budget_generated_by: `wave-3 fleet perf sweep ratchet pass, branch wave3/scoped-heldout-ratchets, extending the wave-2b budget with batch-1 through batch-7, assisted fleet gap-close measurements, held-out exact-file RCA probes, and the scoped Groovy heldout ratchet`

## Coverage

| metric | value |
|---|---:|
| fleet languages | 206 |
| budgeted languages | 204 |
| held out languages | 2 |
| known budget class gaps | 4 |
| wave2b pending budget rows | 15 |
| scoped heldout budget rows | 1 |
| measured-today budget rows | 194 |
| partial measured-today notes | 1 |

Measurement basis: `reps=5`, `warmup=1`, `file_budget_ms=10000`, `max_files=8`, `order=largest`, `axes=full,noedit`.

Excluded paths: `groovy/subprojects/performance/src/files/pleac11_15.groovy`.

Held out of the ratchet: `d`, `fsharp`.

## Budget Status Counts

| status | languages |
|---|---:|
| `green` | 50 |
| `green_with_caveat` | 143 |
| `wave2b_pending` | 11 |

## Known Gap Ledger

| key | file | action |
|---|---|---|
| `d_expressionsem_go_rss_blowup` | d/compiler/src/dmd/expressionsem.d (685384 bytes; largest D corpus file, first selected file under largest-order probes) | D remains held out of the ratchet. The prior Go timeout/OOM class is contained under default settings, but excluding expressionsem.d is not enough: largest-order D next hits C timeouts and a dsymbolsem.d C noedit-base RSS watchdog. A ratchetable row needs a principled smaller-workload policy or an explicit D C-reference high-RSS witness exclusion set. |
| `fsharp_providedtypes_c_reference_memory_blowup` | fsharp/examples/FSharp.Compiler/tests/EndToEndBuildTests/ProvidedTypes/ProvidedTypes.fs (755275 bytes; first active selected file after largest-8 selection) | F# remains held out. Do not rerun broad F# sweeps without a disposable hard RSS envelope. A ratchetable row needs either a principled corpus-selection policy for multi-MiB F# fixtures plus a default-budget truncation fix, or an explicit decision that these C-reference high-RSS giants are excluded workload witnesses rather than normal ratio samples. |
| `groovy_pleac11_15_memory_blowup` | groovy/subprojects/performance/src/files/pleac11_15.groovy (102960 bytes, largest-file selection hit during the assisted fleet pass) | Groovy is now budgeted only under a scoped measurement basis that excludes the named pleac11_15.groovy witness. The exact witness remains a tracked correctness/perf gap: default policy contains the OOM, but the file is still C-shape divergent and ~60x C on full parse. |
| `webworker_generated_d_ts` | typescript/src/lib/webworker.generated.d.ts (786262 bytes, largest .d.ts in the corpus sample) | typescript's full_axis budget above is intentionally NOT tightened to reflect a 'fixed' webworker.generated.d.ts; GOT_FAITHFUL_CONDENSE (or an equivalent default-budget-aware condense path) remains a real wave-2b item. |

## Seed Sources

- `after_cliffs_20260706T210143Z`
- `authoritative_20260706T145520Z`
- `d_expressionsem_default_20260709T083649Z`
- `fleet_gap_close_assist_20260708T232019Z_to_20260709T003453Z`
- `fsharp_providedtypes_exact_default_20260709T094157Z`
- `fsharp_providedtypes_exact_full30s_20260709T094341Z`
- `fsharp_providedtypes_exact_scale3_rss4096_20260709T094500Z`
- `groovy_ceiling_exact_20260709T075435Z`
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
- `wave3_scoped_d_exclude_expressionsem_20260709T103336Z`
- `wave3_scoped_fsharp_exclude_providedtypes_20260709T103723Z`
- `wave3_scoped_groovy_exclude_pleac11_20260709T103612Z`
- `webworker_oracle_spotcheck`

## Caveats

- The perf ratio budget is a ratchet and evidence ledger, not a universal near-C claim; >2x and cliff rows remain explicit backlog.
- d, fsharp are intentionally held out of the language ratchet until their memory/C-reference RCA rows are resolved.
- 1 budget row(s) use scoped heldout exclusions; see measurement_basis.exclude_paths and the known-gap ledger before treating those rows as whole-language claims.
- The TypeScript webworker generated-file entry remains a correctness cross-check caveat even though TypeScript has a timing budget row.
