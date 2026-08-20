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

The current board is `c0f-b09e3d9-r2-20260813T022827Z`. It uses revision
`b09e3d997690ec2b5d34a4a84310b5ebe06e14c6`, the 206-language corpus lock, and
bundle digest `67871ab7e453d8848e7f0265725fc54dbf6cd7cba0b8902bd1a2c2d6940c4ef8`.
It is partially gated and grants no performance credit.

Signal definition: `full.status == "ok" and full.c_median_ns >= 1000`.

Signal rows: **1,325**; Go time: **353011159791 ns**; C time:
**41704371680 ns**; ratio-by-total: **8.464608x**.

| Class | Rows | Go ns | C ns | Ratio by total | Go share | Local Go gap |
|---|---:|---:|---:|---:|---:|---:|
| clean | 871 | 76310525415 | 16827373226 | 4.534905x | 21.6170% | 77.9894% |
| error | 454 | 276700634376 | 24876998454 | 11.122750x | 78.3830% | 92.2688% |

The run measured 1,428 files and evaluated 1,327 full parses. Runtime facts
cover 1,374 of 1,428 rows and 99.0691% of signal Go time. The seven partial
rows used automatic forest results during diagnostic capture.

The targeted correction used revision `5067343eae3d2c3291bbbfbe918a71e4e3d871ea`.
It covered 24 files, produced 24 complete runtime records, and produced zero
forest records. Its bundle digest is
`ce1fa135c9e350543bc2c7fd8ceffb62eef349141e72602178541c08a7fc6a15`.

This confirmation proves the defect class and the diagnostic remedy. It does
not approve the public route-control implementation.

## Weighted cohort ceilings

The current run supplies runtime facts for the listed cohorts. Presence shares
are upper bounds. Projected timed equivalents remain estimates, not credit.

| Cohort | Rows | Languages | Go share | Decision |
|---|---:|---:|---:|---|
| recovery entry | 434 | 97 | 73.5438% | admit B16 evidence |
| retry attempt | 156 | 45 | 62.2130% | admit retry transcript |
| selected retry | 36 | 15 | 19.8017% | admit retry selection study |
| retained initial result | 120 | 44 | 42.4113% | admit retry waste study |
| multiple live versions | 423 | 93 | 72.8857% | admit lifetime study |
| scanner resynchronization | 9 | 2 | 0.4752% | reject broad scanner work |
| materialization present | 1,271 | 190 | 99.0691% | use timed estimate only |

| Projected mechanism | Signal Go share | Decision |
|---|---:|---|
| retry outer remainder | 48.6510% | instrumentation only |
| recovery cost walk | 15.3249% | admit B16 mechanism search |
| alternative lifetime | 16.8853% | admit lifetime investigation |
| scanner boundary | 0.0208% | reject broad scanner work |
| materialization | 11.4409% | admit grouped study |

The retry remainder includes outer parse work and losing attempts. Do not treat
it as a pure retry-cost estimate.

## Findings and conflicts

- **legacy-share-boundary — resolved.** The accepted C0 profile assigns different named boundaries. Wide scheduler-family shares are 80.3-84.8%; dispatch plus reductions are 47.6-56.2%. Do not use any legacy percentage without its boundary.
- **sealed-share-join — unresolved.** The sealed board contains ratios and A/A nulls, but no component profile. The local C0 profile is a different host and diagnostic instrument. Keep C0e ratios and C0e noise evidence separate from C0f fleet shares.
- **fleet-mechanism-facts — partially resolved.** The R2 runtime lane supplies retry, recovery, lifetime, scanner, and materialization facts for 99.0691% of signal Go time. The seven partial records require the private diagnostic forest control and a repeated 24-file confirmation at the final pull-request head. Do not award performance credit.
- **hygiene-provenance — unresolved.** F0 records 1000 ns as a fixed reporting policy, but the V10 metadata has no timer calibration or pre-run threshold record. Treat the F0 hygiene denominator as queryable evidence, not sealed C6f acceptance proof, until threshold provenance is attached.
- **error-byte-denominator — unresolved.** F0 signal bytes give 84,094,345 / 260,884,819 = 32.2343%. R1 does not define the 39.0% byte denominator, so the values cannot be reconciled from this artifact. Publish both denominators and do not blend them.

## Closure gates

Close this board only after the following gates pass:

- restore the admission-switch contract;
- add a private diagnostic control for automatic forest routes;
- repeat the 24-file confirmation at the final pull-request head;
- pass the affected focused tests and required Continuous Integration checks;
- amend the analyzer receipt with zero present partial rows.

C0e remains a separate Phase-1 obligation. Its sealed board remains 3.986x with
four fixtures above 3.0x.

## Final-head closure status

The current open campaign pull request is [#683](https://github.com/odvcencio/gotreesitter/pull/683)
at head `1095aff7aba4a7c7df85da6e7cf9be918063c269`. No final-head C0f fleet
receipt exists for that commit.

The current board at `b09e3d997690ec2b5d34a4a84310b5ebe06e14c6` remains the
latest retained diagnostic board. It measured 1,428 files, 1,327 full parses,
and 99.0691% of signal Go time with runtime facts. Its clean and error ratios
were 4.534905x and 11.122750x.

The separate candidate comment for commit
`fa4cf65b89a794f71d3af354a360c5c616a1bce1` reported 1,428 of 1,435 files,
341 hard-gate findings, clean aggregate and median of 4.4992x and 3.1448x,
and error aggregate and median of 11.1664x and 4.0921x. Keep those values as
candidate evidence only; do not merge them into the `b09e3d9` board.

Run the final-head fleet only after authenticated GCP access succeeds. Require
the unchanged 206-language corpus lock, complete file coverage, route-control
proof, runtime attribution, and hygiene provenance. Until then, C0f remains
open, the grant freeze remains active, and no performance credit is allowed.
