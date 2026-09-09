# Performance and efficiency evidence

Use 20 paired seeds and the geometric mean of their candidate/legacy ratios.
Estimate a 95% confidence interval with 50,000 percentile-bootstrap resamples.
Resample complete seed pairs. Set the random seed to 20260909.
This interval describes the pinned host and process protocol.
It does not estimate cross-machine uncertainty.

The initial candidate, arena-sizing candidate, and final candidate each received a separate 20-pair comparison.
The final candidate is the prefix-enumeration directory. Earlier rounds are attribution controls.
Do not compare unpaired medians across rounds as a measured speedup.

Each operation reuses a parser across benchmark iterations. Grammar/parser construction is outside the timer.
Initial parsing, four edits, and releases are inside the timer. Allocation counts include those operations.
The fixture loader verifies canonical source and edited SHA-256 hashes in every process.
fixtures.json records the derived prefix witness. No C parsing occurs inside timed or heap measurements.

| History | Legacy allocated MiB | Candidate allocated MiB | Byte ratio [95% CI] | Legacy allocations | Candidate allocations |
| --- | ---: | ---: | ---: | ---: | ---: |
| Token-class change | 2.228 | 0.760 | 0.347 [0.343, 0.352] | 998 | 983 |
| Early newline | 35.938 | 20.923 | 0.609 [0.593, 0.629] | 21,805 | 28,576 |
| Prefix call/conversion | 8.905 | 4.130 | 0.478 [0.466, 0.492] | 8,577 | 11,913 |
| Length change | 3.769 | 16.754 | 4.433 [4.422, 4.442] | 2,401 | 1,242 |
| Deletion recovery | 1.774 | 2.586 | 1.457 [1.454, 1.459] | 3,216 | 2,473 |

Length-change and deletion allocation costs include unsuccessful compact work before legacy.
The allocation-count improvements do not make these histories native.

Reported edit work excludes the identical initial parse in both lanes.
New-node metrics count public nodes. Private graph and certificate visits are separate.
Successful certificate units count one payload, child-edge, or graph-node visit.
Declined attempts reset their prefix; their certificate totals are unavailable, not zero.

| History | Legacy edit tokens | Candidate edit tokens | Legacy new nodes | Candidate new nodes | Candidate certificate units |
| --- | ---: | ---: | ---: | ---: | ---: |
| Token-class change | 712 | 2,520 | 2,670 | 5,900 | 22016 |
| Early newline | 4,693 | 13,940 | 13,284 | 29,976 | 117268 |
| Prefix call/conversion | 1,993 | 5,692 | 5,408 | 12,216 | 48500 |
| Length change | 872 | 1,109 | 1,940 | 1,940 | unavailable |
| Deletion recovery | 1,257 | 1,323 | 4,919 | 4,919 | unavailable |

The required histories rebuild approximately 2.2 times as many public nodes per edit history.
They consume roughly 2.9–3.5 times as many edit tokens.
Initial parse work is identical and is recorded separately in every benchmark row.
The cached proof visits new records; the 512/1024-borrow tests reject repeated prefix scanning.
Rollback, ID reuse, mutation, generation reset, and cancellation retain their rejection tests.

The untimed attribution control separates Tree.Edit, reuse-cursor work, and reparse/rebuild work.
Those cold single-history durations are diagnostic, not confidence estimates.
Raw profile structures are in attribution-baseline.txt and attribution-candidate.txt.

CPU profiles attribute only 0.19–0.46% of sampled CPU time to validateReusedHead in the first candidate.
Scheduler dispatch, exact memory accounting, reductions, and materialization dominate the remaining work.
These cumulative categories overlap. Do not sum their percentages.
The deep-prefix allocation test is a deterministic control: baseline 4,155 objects, candidate 20.
The candidate uses the existing single-path reader inside a general fork; it preserves validation and independent payload ownership.

Heap measurements use five fresh processes per lane and alternating lane order.
Each history runs three small/history/small cycles with four edits per workload.
Every heap snapshot follows two GCs. Sources, grammar, and global caches remain live.
The following values are median HeapAlloc in MiB at the third cycle.
Retained pool capacity is counted; the primary comparison does not drain pools.

| History | Legacy old+new overlap | Candidate old+new overlap | Legacy released/parser held | Candidate released/parser held | Legacy parser dropped | Candidate parser dropped |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Token-class change | 14.111 | 10.257 | 13.635 | 9.990 | 8.680 | 6.497 |
| Early newline | 68.251 | 58.266 | 64.869 | 56.440 | 25.590 | 23.478 |
| Prefix call/conversion | 41.246 | 35.070 | 39.551 | 34.283 | 25.379 | 23.478 |
| Length change | 16.969 | 9.314 | 16.087 | 8.432 | 8.681 | 7.336 |
| Deletion recovery | 34.699 | 33.404 | 34.692 | 33.397 | 33.334 | 32.840 |

Small-after-release checkpoints and HeapInuse/HeapObjects ranges are in memory-summary.json.
Parser-held minus parser-dropped heap is an observational delta; global pool changes can also contribute.
The 20-cycle follow-up isolates token change and deletion in fresh processes.
Both plateau before the final cycles. Explicit pool drain returns both lanes to approximately 3.28 MiB.
The initial deletion growth was bounded strong-pool fill, not continuing released-tree ownership growth.
Incremental arena sizing removes most of that unnecessary retained capacity.

Peak resident memory is recorded per fresh process in memory/*.process.txt.
Full-run process timing includes builds; it must not be presented as parser lifecycle timing.

Correctness is independent of timing. All required native tests passed C and parent-link checks.
The broader affected comparison has 447 identical test/subtest statuses.
The all-history parent test remains red for shared legacy failures: 20 length-change and 10 deletion edits.
Those failures are preserved, not reclassified as native compact successes.

Runtime changes remained frozen during each comparison.
Source manifests matched before and after every campaign. Binary hashes were checked by the runner.
Executable test binaries remain on chi-1; the portable archive retains hashes and profiling evidence.


The final 20-pair campaign, including compilation, took 4 minutes 59.82 seconds.
Its process receipt records 279.18 seconds of user CPU time and 7.36 seconds of system time.
The campaign completed with exit status zero and no swapping.
That campaign-level maximum resident size includes compilation and is not a parser peak.

The five fresh memory processes had these maximum resident sizes:

| Lane | Median MiB | Observed range MiB |
| --- | ---: | ---: |
| Repaired legacy | 107.54 | 104.26–107.66 |
| Final candidate | 98.49 | 98.32–98.56 |

MiB means 1,048,576 bytes.
The permanent native-history regression also passed after experimental harness files were removed.
Those removed files remain in the archive, with exact source hashes and reproduction patches.

CPU and heap profiles also include process setup and benchmark calibration.
Use timed B/op measurements for allocation comparisons.
Use sampled profiles for attribution, not exact per-operation percentages.
