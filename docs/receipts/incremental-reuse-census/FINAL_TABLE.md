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
