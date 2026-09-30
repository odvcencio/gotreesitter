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
