# C0e/C0f attribution boards

Status: **partially gated**. This report is evidence-only. It does not run a scan or change routing.

## Authority

R1 is authoritative. C0e and C0f remain independent instruments.

## C0e sealed equal-fixture board

Epoch `strictboundary-20260802T062212Z-v9`, commit `492cd600cfd2bfd0bd3e8b3233fc2f477ff0e887`. Compact Go/C geomean: **3.986x**.

| Fixture | Compact Go/C |
|---|---:|
| `rewrite.go` | 4.348x |
| `query_compile.go` | 4.030x |
| `language.go` | 4.126x |
| `grammargen/lr.go` | 3.493x |

The C0e target is geomean <= 2.0x and every fixture <= 3.0x. Result: geomean pass **false**, fixture pass **false**. Four fixtures exceed 3.0x.

The accepted local C0 profile records a separate noise floor: 7.367% to 10.803% p95 absolute A/A delta on a shared WSL2 host. It is not a sealed-v9 noise floor.

## C0f fleet board

Signal definition: `axis.status == "ok" and predicates.ratio_interpretable == true; timer threshold 1000 ns`.

Signal rows: **1315**; Go time: **436257955255 ns**; C time: **39840435812 ns**; ratio-by-total: **10.950130x**.

| Class | Rows | Go ns | C ns | Ratio by total | Go share | Local Go gap |
|---|---:|---:|---:|---:|---:|---:|
| clean | 871 | 121024265439 | 16610014247 | 7.286223x | 27.7414% | 86.2755% |
| error | 444 | 315233689816 | 23230421565 | 13.569865x | 72.2586% | 92.6307% |

F0 counts: 1,435 rows; 876 clean; 534 error; 25 stopped; 1,317 measured-ok; 2 hygiene rows; 1,315 ratio-eligible; 808 clean rows above 3.0x before hygiene and 806 after hygiene.

## Weighted cohort ceilings

The recovery row is an observable error-class proxy. The other cohorts lack runtime facts in V10. Their 100% values are upper bounds, not performance credit.

| Cohort | Observed | Rows | Go share | Local gap | Ceiling |
|---|---:|---:|---:|---:|---:|
| recovery | true | 444 | 72.2586% | 92.6307% | 72.3% |
| alternative_lifetime | false | 0 | 0.0000% | 0.0000% | 100.0% |
| scanner_boundary | false | 0 | 0.0000% | 0.0000% | 100.0% |
| materialization | false | 0 | 0.0000% | 0.0000% | 100.0% |

Selection formula: `projected_saved_go_ns / sum(go_median_ns over measured signal rows) >= 0.02`. C0f cannot evaluate the numerator for the three unobserved cohorts.

## Findings and conflicts

- **legacy-share-boundary — resolved.** The accepted C0 profile assigns different named boundaries. Wide scheduler-family shares are 80.3-84.8%; dispatch plus reductions are 47.6-56.2%. Do not use any legacy percentage without its boundary.
- **sealed-share-join — unresolved.** The sealed board contains ratios and A/A nulls, but no component profile. The local C0 profile is a different host and diagnostic instrument. Keep C0e ratios and C0e noise evidence separate from C0f fleet shares.
- **fleet-mechanism-facts — unresolved.** The V10 scoreboard and F0 manifest contain class, stop, timing, recovery memo, and oracle fields. They contain no retry count, recovery-cost counter, live-version lifetime, scanner-call density, or materialization-work counter. Do not admit a mechanism or claim the 2% selection ceiling until a trace lane supplies these facts.
- **hygiene-provenance — unresolved.** F0 records 1000 ns as a fixed reporting policy, but the V10 metadata has no timer calibration or pre-run threshold record. Treat the F0 hygiene denominator as queryable evidence, not sealed C6f acceptance proof, until threshold provenance is attached.
- **error-byte-denominator — unresolved.** F0 signal bytes give 84,094,345 / 260,884,819 = 32.2343%. R1 does not define the 39.0% byte denominator, so the values cannot be reconciled from this artifact. Publish both denominators and do not blend them.
