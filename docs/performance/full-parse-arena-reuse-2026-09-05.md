# Full-parse arena reuse

Date: 2026-09-05. Runtime: `14f5bd4c238c07151b944f36cb20723992d62b68`.
Baseline: `236ca848239679221715b386115fc2910f6cf4e1`.

Reusing retained node slabs improved the measured human-authored Go full parse by **6.47 percent**, with p=0.004.
Retain this general allocation change outside compact parser graduation.
The standard Go trio showed no significant timing change.
These results establish no universal language improvement or speed ratio against C.

## Results

The frozen `grammargen_lr` fixture contains 235,626 bytes and forces the legacy parser.
Each timing campaign used twenty alternating pairs with seeds 1 through 20.
Each process used 750 milliseconds, count=1, GOMAXPROCS=1, and allocation reporting.
The generated trio and human-authored fixture used separate campaigns without reruns or discarded samples.

| Human-authored full parse | Baseline median | Candidate median | Result |
| --- | ---: | ---: | --- |
| Time | 220.0190805 ms | 205.782642 ms | -6.47%, p=0.004 |
| Allocated bytes/op | 9,821,702 | 23,872 | -99.76%, p<0.001 |
| Allocations/op | 169 | 166 | -1.78%, p<0.001 |
| Arena capacity, bytes | 56,434,672 | 56,654,840 | +0.39% |

Read the [macro comparison](full-parse-arena-reuse-2026-09-05/macro-benchstat.txt) and [generated trio comparison](full-parse-arena-reuse-2026-09-05/benchstat.txt).
The generated full, edit, and no-edit timing tests have p=0.968, p=0.081, and p=0.268 respectively.
The edit allocation median changed from 124 to 123 bytes, with p=0.017 and three allocations on both sources.
That one-byte difference does not establish a smaller allocation object.
Every macro sample preserved the same node, token, stack, and normalization counts.

## Mechanism and limits

A warm parse can populate overflow slabs before the parser records its size hint.
The next reservation previously replaced primary storage and discarded sufficient overflow storage.
The new reservation reuses those slabs when their total capacity satisfies the hint.
Exact contiguous reservation remains available for final tree cloning.
Allocation budgets, retention limits, and release behavior remain unchanged.

The allocation benefit concerns growth after warm-up, not a 99.76 percent reduction in live-tree memory.
Longer runs amortize the original replacement cost; this campaign does not establish a sustained 6.47 percent speedup.
The candidate retains 220,168 more arena bytes on this fixture.

Process peak resident set size (RSS) includes admission, warm-up, validation, and one measured parse.
One kibibyte (KiB) equals 1,024 bytes.

| Pair | Baseline RSS, KiB | Candidate RSS, KiB |
| --- | ---: | ---: |
| 1 | 176,344 | 143,728 |
| 2 | 166,652 | 117,736 |
| 3 | 151,708 | 121,260 |

These six probes passed. They do not establish long-run retention behavior.

## Correctness and reproduction

The corrected Docker gate passed 48 baseline tests and 52 candidate tests.
The initial baseline run exposed a stale ownership assertion; preserve that failure in the full archive.
Both sources then used the exact merged test correction through an identical test-only overlay.
Read the [correctness record](full-parse-arena-reuse-2026-09-05/correctness-summary.md) for its scope and provenance.
New controls cover fragmented capacity, insufficient capacity, clearing, accounting, live arena rejection, and repeated parser ownership.
Separate Go, JSON, and JavaScript containers passed the selected C comparisons without skips.
The timing and RSS runs excluded the correctness overlay.

Docker used Go 1.25.14, one processor pinned to logical processor 2, four GiB memory, and GOMEMLIMIT=3GiB.
The shared host uses an Intel Core Ultra 9 285 processor; scheduling limits timing precision.
All nine containers avoided memory failures and timeouts. One stopped on the documented stale baseline assertion.
All 2,956 baseline and 2,958 candidate file hashes matched after measurement.
The runtime commit reproduces the measured patch exactly.

Read the [recorded commands](full-parse-arena-reuse-2026-09-05/commands.md), raw paired outputs, source identities, and compact receipts.
Run `sha256sum -c SHA256SUMS` inside the evidence directory to verify this sixteen-file bundle.
The durable archive retains full logs, profiles, and source inventories outside the repository.

## Integration on current main

Integration baseline: `b3a5f9d4129cdb255e01053e6ebf5a7d188fe4b5`. Candidate: `6b988f74`.
All four runtime and test files match the measured candidate exactly.
The original fifteen evidence hashes verify.

Fresh Docker checks passed 48 baseline controls and 52 candidate controls without a test overlay.
Separate Go, JSON, and JavaScript containers passed the selected locked-C comparisons without skips.
All four containers completed without memory failures or timeouts.
These integration checks establish correctness on the new baseline.
They do not update the historical timing comparison or establish universal parity.

Read the [integration receipts](full-parse-arena-reuse-integration-2026-09-05/summary.json).
The adjacent scripts, logs, container metadata, and SHA256SUMS preserve reproduction details.
