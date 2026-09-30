Real-source incremental edits lose reuse or pay for fresh verification and discarded retries. This change adds compile-time opt-in reuse decisions, exact guard outcomes, dispatch barriers, dependency proof timing, and a locked real-source census. Default-off parsing results, work counters and retained identities remain unchanged.

| Language | Nodes retained | Ranked edit costs: time % / lost-node % | Observer time |
|---|---:|---|---:|
| Go | 2.1% | multiple GLR stacks **67.6/97.4**; fresh recovery verification **15.3/0.0**; reparse and rebuild **4.0/0.0** | 10.3% |
| JavaScript | 82.6% | reparse and rebuild **16.9/0.0**; reuse selection **14.3/0.0**; fresh recovery verification **11.2/0.0** | 40.9% |
| TypeScript | 96.4% | reparse and rebuild **17.1/0.0**; reuse selection **12.0/0.0**; fresh recovery verification **11.3/0.0** | 37.7% |
| Python | 73.7% | fresh recovery verification **46.8/53.7**; reparse and rebuild **13.1/0.0**; reuse selection **5.6/0.0** | 21.3% |
| Rust | 57.9% | full-parse retry **26.6/0.0**; reparse and rebuild **16.8/0.0**; reuse selection **7.5/0.0** | 31.2% |
| Java | 4.1% | full-parse retry **77.6/0.0**; multiple GLR stacks **19.1/97.9**; reparse and rebuild **1.2/0.0** | 1.7% |
| C# | 0.0% | unsupported scanner → fresh parse **100.0/100.0**; Tree.Edit **0.0/0.0**; missing-node dependency proof **0.0/0.0** | 0.0% |
| PowerShell* | 66.8% | fresh recovery verification **69.2/0.0**; full-parse retry **16.6/0.0**; multiple GLR stacks **2.6/15.1** | 5.9% |

Time/loss shares cover the requested 144 edits; 48 numeric controls are separate. PowerShell* uses existing fix `4afda6f2d` only in isolated measurement checkouts because original before/after edit sessions time out in exponential dependency recursion.

See [the report](REPORT.md), [workload rows](WORKLOADS.md), and [validation receipts](validation.json) for the full ranked table, guard origins, correctness protection, C structural counterparts, exact counters, baseline failures and source identities. Nine existing incremental/fresh Go failures and 104 fresh-Go/C failures remain in the inventory. This is not graduation evidence.

Validation: recorder unit/race tests; eight language-scoped observation checks; production symbol erasure; Docker C smoke parity; invariant gates before/after (unchanged JavaScript failure, original PowerShell timeout); ledger and core no-edit suites; strict results/work/identity comparison on 168 original-engine edits plus 24 matched PowerShell dependency controls.

The locked C runtime has one incremental/fresh disagreement on the TypeScript middle splice, unchanged before/after. Go/C ratios on unequal outputs are diagnostic costs.

Before/after returned work and retained identities (24 edits/language; PowerShell controls include the dependency):

| Language | Tokens | Nodes allocated | Retained nodes |
|---|---:|---:|---:|
| Go | 327,088 → 327,088 | 1,820,563 → 1,820,563 | 8,989 → 8,989 |
| JavaScript | 15,609 → 15,609 | 59,351 → 59,351 | 159,532 → 159,532 |
| TypeScript | 23,115 → 23,115 | 286,129 → 286,129 | 1,240,309 → 1,240,309 |
| Python | 115,374 → 115,374 | 322,939 → 322,939 | 448,363 → 448,363 |
| Rust | 105,811 → 105,811 | 381,529 → 381,529 | 396,701 → 396,701 |
| Java | 292,300 → 292,300 | 3,136,790 → 3,136,790 | 17,645 → 17,645 |
| C# | 223,364 → 223,364 | 3,387,195 → 3,387,195 | 0 → 0 |
| PowerShell* | 95,529 → 95,529 | 858,448 → 858,448 | 356,225 → 356,225 |

No existing pins, thresholds, graduation lists or default routes changed. NEEDS-APPROVAL: none. Integration dependency: PowerShell needs the existing `4afda6f2d` fix; it is outside this branch.

Same-width one-byte replacement: change the nearest numeric leaf digit at each of three sites, over both real-source sizes (48 additional edits). Each cost cell is exclusive wall time % / lost-node %. The original insertion/splice table remains separate. PowerShell* uses the existing dependency fix in both controls.

| Language | Nodes retained | Ranked costs: time % / lost-node % | Observer time |
|---|---:|---|---:|
| Go | 1.2% | multiple GLR stacks **51.5/99.6**; fresh verification **35.7/0.0**; reparse/rebuild **3.8/0.0** | 8.2% |
| JavaScript | 90.7% | fresh verification **45.0/0.0**; reparse/rebuild **13.6/0.0**; reuse selection **9.8/0.0** | 19.6% |
| TypeScript | 99.7% | fresh verification **58.6/0.0**; lexical proof **10.1/0.0**; reparse/rebuild **5.0/0.0** | 16.5% |
| Python | 76.6% | fresh verification **46.1/60.4**; reparse/rebuild **13.3/0.0**; reuse selection **6.1/0.0** | 23.2% |
| Rust | 99.4% | fresh verification **90.2/0.0**; EOF dispatch **3.9/0.0**; reparse/rebuild **2.5/0.0** | 1.7% |
| Java | 3.4% | multiple GLR stacks **82.0/99.1**; reparse/rebuild **10.9/0.0**; reuse selection **0.3/0.0** | 6.5% |
| C# | 0.0% | unsupported scanner → fresh parse **100.0/100.0**; missing-node dependency proof **0.0/0.0**; reparse/rebuild **0.0/0.0** | 0.0% |
| PowerShell* | 17.3% | fresh verification **63.1/95.5**; full-parse retry **28.8/0.0**; reparse/rebuild **1.3/0.0** | 3.8% |

Paired randomized performance receipts: [PERF.md](PERF.md), [machine-readable medians](benchmark-summary.json), and [raw runs with benchstat](perf/).

```text
METRIC: go/32768/100_byte edit Go/C | 372.500 -> 375.250 | 6f17ca1d6..592c8169d (default-off hooks) | 32686 bytes, middle-site 100_byte, 20 seeds
METRIC: go/32768/one_byte edit Go/C | 376.100 -> 378.800 | 6f17ca1d6..592c8169d (default-off hooks) | 32686 bytes, middle-site one_byte, 20 seeds
METRIC: go/32768/splice edit Go/C | 48.575 -> 43.635 | 6f17ca1d6..592c8169d (default-off hooks) | 32686 bytes, middle-site splice, 20 seeds
METRIC: go/140288/100_byte edit Go/C | 387.850 -> 385.650 | 6f17ca1d6..592c8169d (default-off hooks) | 140026 bytes, middle-site 100_byte, 20 seeds
METRIC: go/140288/one_byte edit Go/C | 387.650 -> 387.800 | 6f17ca1d6..592c8169d (default-off hooks) | 140026 bytes, middle-site one_byte, 20 seeds
METRIC: go/140288/splice edit Go/C | 2.983 -> 2.942 | 6f17ca1d6..592c8169d (default-off hooks) | 140026 bytes, middle-site splice, 20 seeds
METRIC: javascript/32768/100_byte edit Go/C | 38.845 -> 40.005 | 592c8169d..fea550641 (default-off hooks) | 32689 bytes, middle-site 100_byte, 20 seeds
METRIC: javascript/32768/one_byte edit Go/C | 38.205 -> 38.605 | 592c8169d..fea550641 (default-off hooks) | 32689 bytes, middle-site one_byte, 20 seeds
METRIC: javascript/32768/splice edit Go/C | 21.920 -> 22.105 | 592c8169d..fea550641 (default-off hooks) | 32689 bytes, middle-site splice, 20 seeds
METRIC: javascript/140288/100_byte edit Go/C | 19.950 -> 20.390 | 592c8169d..fea550641 (default-off hooks) | 141043 bytes, middle-site 100_byte, 20 seeds
METRIC: javascript/140288/one_byte edit Go/C | 20.470 -> 20.555 | 592c8169d..fea550641 (default-off hooks) | 141043 bytes, middle-site one_byte, 20 seeds
METRIC: javascript/140288/splice edit Go/C | 20.440 -> 20.505 | 592c8169d..fea550641 (default-off hooks) | 141043 bytes, middle-site splice, 20 seeds
METRIC: typescript/32768/100_byte edit Go/C | 6.865 -> 7.027 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32652 bytes, middle-site 100_byte, 20 seeds
METRIC: typescript/32768/one_byte edit Go/C | 6.889 -> 7.056 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32652 bytes, middle-site one_byte, 20 seeds
METRIC: typescript/32768/splice edit Go/C | 14.055 -> 13.970 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32652 bytes, middle-site splice, 20 seeds
METRIC: typescript/140288/100_byte edit Go/C | 4.118 -> 4.147 | fea550641ccc92cbc157c6c5545d77a44818a745 | 140212 bytes, middle-site 100_byte, 20 seeds
METRIC: typescript/140288/one_byte edit Go/C | 4.104 -> 4.095 | fea550641ccc92cbc157c6c5545d77a44818a745 | 140212 bytes, middle-site one_byte, 20 seeds
METRIC: typescript/140288/splice edit Go/C | 4.663 -> 4.581 | fea550641ccc92cbc157c6c5545d77a44818a745 | 140212 bytes, middle-site splice, 20 seeds
METRIC: python/32768/100_byte edit Go/C | 14.970 -> 14.870 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32768 bytes, middle-site 100_byte, 20 seeds
METRIC: python/32768/one_byte edit Go/C | 14.810 -> 14.700 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32768 bytes, middle-site one_byte, 20 seeds
METRIC: python/32768/splice edit Go/C | 18.180 -> 18.140 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32768 bytes, middle-site splice, 20 seeds
METRIC: python/140288/100_byte edit Go/C | 25.440 -> 25.345 | fea550641ccc92cbc157c6c5545d77a44818a745 | 139106 bytes, middle-site 100_byte, 20 seeds
METRIC: python/140288/one_byte edit Go/C | 25.400 -> 25.545 | fea550641ccc92cbc157c6c5545d77a44818a745 | 139106 bytes, middle-site one_byte, 20 seeds
METRIC: python/140288/splice edit Go/C | 13.970 -> 13.810 | fea550641ccc92cbc157c6c5545d77a44818a745 | 139106 bytes, middle-site splice, 20 seeds
METRIC: rust/32768/100_byte edit Go/C | 93.080 -> 93.015 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32765 bytes, middle-site 100_byte, 20 seeds
METRIC: rust/32768/one_byte edit Go/C | 95.095 -> 94.845 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32765 bytes, middle-site one_byte, 20 seeds
METRIC: rust/32768/splice edit Go/C | 89.205 -> 88.455 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32765 bytes, middle-site splice, 20 seeds
METRIC: rust/140288/100_byte edit Go/C | 484.600 -> 479.400 | fea550641ccc92cbc157c6c5545d77a44818a745 | 140019 bytes, middle-site 100_byte, 20 seeds
METRIC: rust/140288/one_byte edit Go/C | 489.900 -> 480.450 | fea550641ccc92cbc157c6c5545d77a44818a745 | 140019 bytes, middle-site one_byte, 20 seeds
METRIC: rust/140288/splice edit Go/C | 549.200 -> 549.050 | fea550641ccc92cbc157c6c5545d77a44818a745 | 140019 bytes, middle-site splice, 20 seeds
METRIC: java/32768/100_byte edit Go/C | 33.955 -> 33.930 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32728 bytes, middle-site 100_byte, 20 seeds
METRIC: java/32768/one_byte edit Go/C | 32.900 -> 33.680 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32728 bytes, middle-site one_byte, 20 seeds
METRIC: java/32768/splice edit Go/C | 33.075 -> 33.175 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32728 bytes, middle-site splice, 20 seeds
METRIC: java/140288/100_byte edit Go/C | 132.900 -> 135.750 | fea550641ccc92cbc157c6c5545d77a44818a745 | 144870 bytes, middle-site 100_byte, 20 seeds
METRIC: java/140288/one_byte edit Go/C | 135.800 -> 136.950 | fea550641ccc92cbc157c6c5545d77a44818a745 | 144870 bytes, middle-site one_byte, 20 seeds
METRIC: java/140288/splice edit Go/C | 746.150 -> 739.050 | fea550641ccc92cbc157c6c5545d77a44818a745 | 144870 bytes, middle-site splice, 20 seeds
METRIC: c_sharp/32768/100_byte edit Go/C | 592.550 -> 598.950 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32846 bytes, middle-site 100_byte, 20 seeds
METRIC: c_sharp/32768/one_byte edit Go/C | 588.700 -> 586.350 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32846 bytes, middle-site one_byte, 20 seeds
METRIC: c_sharp/32768/splice edit Go/C | 679.050 -> 681.450 | fea550641ccc92cbc157c6c5545d77a44818a745 | 32846 bytes, middle-site splice, 20 seeds
METRIC: c_sharp/140288/100_byte edit Go/C | 6571.500 -> 6520.000 | fea550641ccc92cbc157c6c5545d77a44818a745 | 134787 bytes, middle-site 100_byte, 20 seeds
METRIC: c_sharp/140288/one_byte edit Go/C | 6438.500 -> 6321.500 | fea550641ccc92cbc157c6c5545d77a44818a745 | 134787 bytes, middle-site one_byte, 20 seeds
METRIC: c_sharp/140288/splice edit Go/C | 425.950 -> 415.600 | fea550641ccc92cbc157c6c5545d77a44818a745 | 134787 bytes, middle-site splice, 20 seeds
METRIC: powershell/32768/100_byte edit Go/C | 238.500 -> 239.500 | fea550641ccc92cbc157c6c5545d77a44818a745 + matched dependency 4afda6f2d | 32666 bytes, middle-site 100_byte, 20 seeds
METRIC: powershell/32768/one_byte edit Go/C | 240.650 -> 239.650 | fea550641ccc92cbc157c6c5545d77a44818a745 + matched dependency 4afda6f2d | 32666 bytes, middle-site one_byte, 20 seeds
METRIC: powershell/32768/splice edit Go/C | 242.000 -> 239.900 | fea550641ccc92cbc157c6c5545d77a44818a745 + matched dependency 4afda6f2d | 32666 bytes, middle-site splice, 20 seeds
METRIC: powershell/140288/100_byte edit Go/C | 7174.500 -> 7067.500 | fea550641ccc92cbc157c6c5545d77a44818a745 + matched dependency 4afda6f2d | 104415 bytes, middle-site 100_byte, 20 seeds
METRIC: powershell/140288/one_byte edit Go/C | 7735.000 -> 7525.000 | fea550641ccc92cbc157c6c5545d77a44818a745 + matched dependency 4afda6f2d | 104415 bytes, middle-site one_byte, 20 seeds
METRIC: powershell/140288/splice edit Go/C | 6934.500 -> 7085.000 | fea550641ccc92cbc157c6c5545d77a44818a745 + matched dependency 4afda6f2d | 104415 bytes, middle-site splice, 20 seeds
```

Final frozen control: full parse 8.873 → 8.871 ms; single-byte edit 196.2 → 198.2 µs (**+1.03%, p<0.001**); no-edit 8.512 → 8.444 ns, 0 B/op and 0 allocs/op. The real-source matrix has no significant Go edit-time increase. The small control timing regression remains unexplained; no speedup or zero timing cost is claimed. All 14 returned work/reuse fields and physical identity counts remain exactly equal.
