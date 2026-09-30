Requested edits by language, size and edit class. Each row pools the quarter, middle and three-quarter sites. Cost cells are exclusive wall time % / lost-node %. Observer overhead remains separate. PowerShell* includes the existing dependency fix in both isolated controls; its original edit sessions did not finish. Numeric controls are excluded.

| Language | Target / actual bytes | Edit | Retained nodes | Three largest edit costs (time % / loss %) | Largest node-loss reason | Observer |
|---|---|---|---:|---|---|---:|
| Go | 32768 / 32686 | one_byte | 4.9% | multiple GLR stacks 71.8/97.3; fresh verification 19.8/0.0; reparse/rebuild 1.7/0.0 | multiple GLR stacks 97.3% | 5.3% |
| Go | 32768 / 32686 | 100_byte | 4.9% | multiple GLR stacks 67.4/97.3; fresh verification 23.0/0.0; reparse/rebuild 2.4/0.0 | multiple GLR stacks 97.3% | 5.7% |
| Go | 32768 / 32686 | splice | 5.4% | multiple GLR stacks 79.2/93.9; reparse/rebuild 4.0/0.0; reuse selection 1.2/0.0 | multiple GLR stacks 93.9% | 13.4% |
| Go | 140288 / 140026 | one_byte | 1.0% | multiple GLR stacks 59.4/98.6; fresh verification 26.6/0.0; reparse/rebuild 3.6/0.0 | multiple GLR stacks 98.6% | 8.4% |
| Go | 140288 / 140026 | 100_byte | 1.0% | multiple GLR stacks 60.4/98.6; fresh verification 24.9/0.0; reparse/rebuild 2.3/0.0 | multiple GLR stacks 98.6% | 10.3% |
| Go | 140288 / 140026 | splice | 2.1% | multiple GLR stacks 74.4/95.6; reparse/rebuild 7.0/0.0; accepted-error retry 1.5/0.0 | multiple GLR stacks 95.6% | 14.0% |
| JavaScript | 32768 / 32689 | one_byte | 85.6% | fresh verification 22.2/0.0; reparse/rebuild 13.1/0.0; reuse selection 10.6/0.0 | token/state compatibility 50.6% | 39.3% |
| JavaScript | 32768 / 32689 | 100_byte | 85.6% | fresh verification 28.5/0.0; reparse/rebuild 15.6/0.0; reuse selection 9.3/0.0 | token/state compatibility 50.6% | 36.1% |
| JavaScript | 32768 / 32689 | splice | 84.1% | reparse/rebuild 17.4/0.0; reuse selection 16.7/0.0; dispatch/eof_lookahead: reuse not offered 7.1/0.0 | token/state compatibility 52.8% | 46.0% |
| JavaScript | 140288 / 141043 | one_byte | 80.5% | reparse/rebuild 22.2/0.0; reuse selection 18.2/0.0; token/state compatibility 4.0/44.0 | token/state compatibility 44.0% | 39.7% |
| JavaScript | 140288 / 141043 | 100_byte | 80.5% | reuse selection 19.4/0.0; reparse/rebuild 19.0/0.0; token/state compatibility 4.2/44.0 | token/state compatibility 44.0% | 40.8% |
| JavaScript | 140288 / 141043 | splice | 78.3% | reparse/rebuild 20.7/0.0; reuse selection 19.3/0.0; token/state compatibility 4.3/46.3 | token/state compatibility 46.3% | 40.2% |
| TypeScript | 32768 / 32652 | one_byte | 97.5% | reparse/rebuild 17.2/0.0; reuse selection 9.7/0.0; fresh verification 8.2/0.0 | token/state compatibility 57.4% | 48.5% |
| TypeScript | 32768 / 32652 | 100_byte | 97.5% | reparse/rebuild 20.0/0.0; reuse selection 11.5/0.0; fresh verification 10.5/0.0 | token/state compatibility 57.4% | 39.6% |
| TypeScript | 32768 / 32652 | splice | 55.5% | full-parse retry 41.7/74.9; reparse/rebuild 14.2/0.0; multiple GLR stacks 7.7/5.3 | full-parse retry 74.9% | 21.2% |
| TypeScript | 140288 / 140212 | one_byte | 100.0% | fresh verification 22.9/0.0; reparse/rebuild 15.7/0.0; reuse selection 12.1/0.0 | source-byte / ancestor edit guard 44.4% | 34.0% |
| TypeScript | 140288 / 140212 | 100_byte | 100.0% | fresh verification 25.3/0.0; reparse/rebuild 14.5/0.0; reuse selection 12.3/0.0 | source-byte / ancestor edit guard 44.4% | 30.9% |
| TypeScript | 140288 / 140212 | splice | 91.6% | reparse/rebuild 19.2/0.0; reuse selection 12.4/0.0; multiple GLR stacks 7.5/99.1 | multiple GLR stacks 99.1% | 43.6% |
| Python | 32768 / 32768 | one_byte | 0.0% | fresh verification 47.8/100.0; reparse/rebuild 11.1/0.0; reuse selection 7.2/0.0 | fresh verification 100.0% | 24.1% |
| Python | 32768 / 32768 | 100_byte | 0.0% | fresh verification 47.9/100.0; reparse/rebuild 13.3/0.0; reuse selection 7.1/0.0 | fresh verification 100.0% | 20.3% |
| Python | 32768 / 32768 | splice | 0.0% | fresh verification 33.3/100.0; reparse/rebuild 16.6/0.0; multiple GLR stacks 9.5/0.0 | fresh verification 100.0% | 21.8% |
| Python | 140288 / 139106 | one_byte | 86.9% | fresh verification 47.1/0.0; reparse/rebuild 13.9/0.0; reuse selection 5.1/0.0 | token/state compatibility 46.2% | 20.8% |
| Python | 140288 / 139106 | 100_byte | 86.9% | fresh verification 50.8/0.0; reparse/rebuild 10.8/0.0; reuse selection 5.1/0.0 | token/state compatibility 46.2% | 21.0% |
| Python | 140288 / 139106 | splice | 83.8% | fresh verification 48.4/0.0; reparse/rebuild 13.7/0.0; reuse selection 5.7/0.0 | token/state compatibility 38.9% | 20.8% |
| Rust | 32768 / 32765 | one_byte | 96.6% | reparse/rebuild 26.2/0.0; reuse selection 11.6/0.0; other subtree eligibility / resume guard 4.8/0.0 | token/state compatibility 43.2% | 45.5% |
| Rust | 32768 / 32765 | 100_byte | 96.6% | reparse/rebuild 24.9/0.0; reuse selection 10.4/0.0; other subtree eligibility / resume guard 6.8/0.0 | token/state compatibility 43.2% | 45.4% |
| Rust | 32768 / 32765 | splice | 90.3% | reparse/rebuild 21.9/0.0; multiple GLR stacks 14.6/73.3; reuse selection 9.0/0.0 | multiple GLR stacks 73.3% | 39.7% |
| Rust | 140288 / 140019 | one_byte | 46.7% | full-parse retry 31.3/0.0; reparse/rebuild 15.1/0.0; reuse selection 6.4/0.0 | token/state compatibility 42.9% | 29.4% |
| Rust | 140288 / 140019 | 100_byte | 46.7% | full-parse retry 32.6/0.0; reparse/rebuild 13.6/0.0; reuse selection 6.8/0.0 | token/state compatibility 42.9% | 26.9% |
| Rust | 140288 / 140019 | splice | 48.7% | full-parse retry 28.8/0.0; reparse/rebuild 18.1/0.0; reuse selection 8.0/0.0 | token/state compatibility 52.0% | 31.3% |
| Java | 32768 / 32728 | one_byte | 17.7% | multiple GLR stacks 43.9/89.0; full-parse retry 37.1/0.0; reparse/rebuild 2.9/0.0 | multiple GLR stacks 89.0% | 10.9% |
| Java | 32768 / 32728 | 100_byte | 17.7% | full-parse retry 37.9/0.0; multiple GLR stacks 34.1/89.0; reparse/rebuild 15.3/0.0 | multiple GLR stacks 89.0% | 8.1% |
| Java | 32768 / 32728 | splice | 17.6% | full-parse retry 38.3/0.0; multiple GLR stacks 35.2/88.6; reparse/rebuild 8.0/0.0 | multiple GLR stacks 88.6% | 14.0% |
| Java | 140288 / 144870 | one_byte | 2.2% | full-parse retry 76.4/0.0; multiple GLR stacks 20.8/99.5; reparse/rebuild 1.1/0.0 | multiple GLR stacks 99.5% | 1.5% |
| Java | 140288 / 144870 | 100_byte | 2.2% | full-parse retry 70.9/0.0; multiple GLR stacks 26.0/99.5; reparse/rebuild 1.0/0.0 | multiple GLR stacks 99.5% | 1.9% |
| Java | 140288 / 144870 | splice | 3.1% | full-parse retry 85.4/0.0; multiple GLR stacks 12.4/97.1; reparse/rebuild 0.8/0.0 | multiple GLR stacks 97.1% | 1.1% |
| C# | 32768 / 32846 | one_byte | 0.0% | unsupported scanner → fresh 100.0/100.0; Tree.Edit 0.0/0.0; missing-node dependency proof 0.0/0.0 | unsupported scanner → fresh 100.0% | 0.0% |
| C# | 32768 / 32846 | 100_byte | 0.0% | unsupported scanner → fresh 100.0/100.0; Tree.Edit 0.0/0.0; missing-node dependency proof 0.0/0.0 | unsupported scanner → fresh 100.0% | 0.0% |
| C# | 32768 / 32846 | splice | 0.0% | unsupported scanner → fresh 100.0/100.0; Tree.Edit 0.0/0.0; missing-node dependency proof 0.0/0.0 | unsupported scanner → fresh 100.0% | 0.0% |
| C# | 140288 / 134787 | one_byte | 0.0% | unsupported scanner → fresh 100.0/100.0; Tree.Edit 0.0/0.0; missing-node dependency proof 0.0/0.0 | unsupported scanner → fresh 100.0% | 0.0% |
| C# | 140288 / 134787 | 100_byte | 0.0% | unsupported scanner → fresh 100.0/100.0; Tree.Edit 0.0/0.0; missing-node dependency proof 0.0/0.0 | unsupported scanner → fresh 100.0% | 0.0% |
| C# | 140288 / 134787 | splice | 0.0% | unsupported scanner → fresh 100.0/100.0; Tree.Edit 0.0/0.0; missing-node dependency proof 0.0/0.0 | unsupported scanner → fresh 100.0% | 0.0% |
| PowerShell* | 32768 / 32666 | one_byte | 81.7% | fresh verification 31.5/0.0; reparse/rebuild 9.2/0.0; dispatch/eof_lookahead: reuse not offered 6.7/0.0 | top-level splice scope 42.2% | 33.7% |
| PowerShell* | 32768 / 32666 | 100_byte | 81.7% | fresh verification 36.2/0.0; reparse/rebuild 12.1/0.0; reuse selection 6.3/0.0 | top-level splice scope 42.2% | 29.4% |
| PowerShell* | 32768 / 32666 | splice | 80.9% | fresh verification 29.6/0.0; reparse/rebuild 9.1/0.0; dispatch/eof_lookahead: reuse not offered 7.0/0.0 | top-level splice scope 40.6% | 36.0% |
| PowerShell* | 140288 / 104415 | one_byte | 62.9% | fresh verification 70.3/0.0; full-parse retry 16.3/0.0; multiple GLR stacks 2.7/16.6 | top-level splice scope 25.4% | 5.6% |
| PowerShell* | 140288 / 104415 | 100_byte | 62.9% | fresh verification 69.4/0.0; full-parse retry 18.5/0.0; multiple GLR stacks 2.6/16.6 | top-level splice scope 25.4% | 5.0% |
| PowerShell* | 140288 / 104415 | splice | 62.9% | fresh verification 71.1/0.0; full-parse retry 16.5/0.0; multiple GLR stacks 2.7/16.6 | top-level splice scope 25.3% | 4.7% |
