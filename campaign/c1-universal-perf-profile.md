# C1 universal perf profile

Evidence only. This profile ranks mechanism cohorts by their
`projected_saved_go_ns` treatment under the C0f selection formula. It claims no
performance credit: per `docs/c0e-c0f-attribution.md` (conflict
`fleet-mechanism-facts`, line 54), no mechanism may be admitted and no 2%
selection ceiling claimed until a trace lane supplies runtime facts.

## Selection formula and threshold

- Formula: `projected_saved_go_ns / sum(go_median_ns over measured signal rows) >= 0.02`
  (`docs/c0e-c0f-attribution.md` line 48; `docs/c0e-c0f-attribution.json`
  `formulas[4]`).
- Denominator (measured signal Go time): **436,257,955,255 ns** over **1,315**
  signal rows (`docs/c0e-c0f-attribution.md` line 28;
  `docs/c0e-c0f-attribution.json` `c0f.signal_go_median_ns`,
  `c0f.signal_rows`).
- Threshold numerator at 0.02: **8,725,159,105 ns** (computed from the two
  values above).
- Signal definition: `axis.status == "ok" and predicates.ratio_interpretable ==
  true`; timer threshold 1000 ns (`docs/c0e-c0f-attribution.md` line 26).

## Ranked cohort table

Ranked by admissible `projected_saved_go_ns` treatment. "Gap-bound projection"
is `(1 - local_go_gap) applied in reverse`: `go_median_ns * local_go_gap`, i.e.
the error-class excess Go time. It is an upper bound, not credit.

| Rank | Cohort | Languages | Fleet ratio contribution | projected_saved_go_ns treatment | Evidence that admits it | Evidence that blocks it |
|---:|---|---|---|---|---|---|
| 1 | `recovery` | Fleet-wide error-class rows (444 rows across the V10 fleet's 206 validated languages, `docs/v10-fleet-manifest.md` lines 5–10). Memo-tier witnesses include Python, Elixir, Scala, Solidity, Enforce, Liquid, PureScript (`docs/t0-tail-cards.md` top-20 census, ranks 1–20) — witnesses, not code selectors | Error class ratio-by-total **13.569865x**; Go share of signal **72.2586%**; excess share of signal Go **66.9336%** (`docs/c0e-c0f-attribution.md` line 33; JSON `c0f.classes.error`) | Gap-bound projection **292,003,268,251 ns** = 66.93% of signal ≥ 0.02 threshold (**passes as upper bound only**) | Observable error-class proxy: `semantic_class == error` over 444 ratio-eligible rows (JSON `c0f.cohorts.recovery`) | No recovery-cost counter or retry count exists in V10/F0 (`docs/c0e-c0f-attribution.md` line 54). The proxy is "not a causal recovery counter" (JSON `c0f.cohorts.recovery.note`). No C0f receipt exists |
| 2 | `alternative_lifetime` | None attributable; no per-row live-version lifetime exists for any of the 206 languages | **0 observed rows**; weight of signal Go **0.0000%** (`docs/c0e-c0f-attribution.md` line 44; JSON `c0f.cohorts.alternative_lifetime`) | Numerator **unevaluable**: C0f cannot evaluate it (`docs/c0e-c0f-attribution.md` line 48); evidence bound is 0..100% of signal Go time, no credit (JSON note) | Nothing yet | Missing runtime fact: live-version lifetime counter (`docs/c0e-c0f-attribution.md` line 54). Cannot pass or fail the 0.02 formula |
| 3 | `scanner_boundary` | None attributable; no per-row scanner-call density exists | **0 observed rows**; weight of signal Go **0.0000%** (`docs/c0e-c0f-attribution.md` line 45; JSON `c0f.cohorts.scanner_boundary`) | Numerator **unevaluable**, same treatment as rank 2 | Nothing yet | Missing runtime fact: scanner-call density / scanner boundary predicate (`docs/c0e-c0f-attribution.md` line 54) |
| 4 | `materialization` | None attributable; no materialization-work counter exists | **0 observed rows**; weight of signal Go **0.0000%** (`docs/c0e-c0f-attribution.md` line 46; JSON `c0f.cohorts.materialization`) | Numerator **unevaluable**, same treatment as rank 2 | Nothing yet | Missing runtime fact: materialization-work counter (`docs/c0e-c0f-attribution.md` line 54) |

Context row (not a mechanism cohort): the clean class contributes ratio-by-total
**7.286223x** and Go share **27.7414%**, with a gap-bound projection of
104,414,251,192 ns = 23.93% of signal (`docs/c0e-c0f-attribution.md` lines
32–33; JSON `c0f.classes.clean`). It is background against which mechanism
cohorts are measured, not a selectable cohort.

## Ranking rules and caveats

1. Only `recovery` has an observable proxy, so only it can be ranked with a
   number today; its number is a ceiling, not a claim.
2. Ranks 2–4 tie at "unevaluable". Their order is alphabetical from the board,
   not evidentiary.
3. Cohort ceilings are non-exclusive: the three unobserved cohorts each carry a
   100%-of-signal upper bound, so ceilings must never be summed
   (`docs/c0e-c0f-attribution.md` line 39: "Their 100% values are upper bounds").
4. Hygiene rows (2 rows below the 1000 ns timer threshold) stay outside the
   signal denominator (`docs/c0e-c0f-attribution.md` line 35;
   `docs/v10-fleet-manifest.md` lines 28–31).
5. The Perl tail card stays not-actionable pending a size series
   (`docs/t0-tail-cards.md` card 6); zero-byte HTTP rows are hygiene-excluded
   from ratio credit (`docs/t0-tail-cards.md` census ranks 8–9).
6. Next evidence gate: a trace lane supplying retry counts, recovery-cost
   counters, live-version lifetimes, scanner-call density, and
   materialization-work counters, after which the numerator becomes evaluable
   and a C0f receipt may be authored.
