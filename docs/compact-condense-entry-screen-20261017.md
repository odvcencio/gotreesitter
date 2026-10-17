# Condensation entry split screen — 2026-10-17

**Rejected:** a smaller entry frame slows four compact Go workloads
**0.79–1.64%** and owned YAML R4 **1.04%** in twenty shuffled pairs. No runtime
source is promoted. The frame reduction is a compiler observation, not a
throughput or RSS improvement.

Baseline: `5e52cf2a2644f3d38785513eebc39f8cf8a506b2`. The candidate moves the
325-line existing-boundary fold/history body into an internal helper. That
body is byte-identical, and every entry validation, boundary probe,
independent-owner branch and direct-append branch remains byte-identical.
The [extraction proof](benchmarks/20261017-condense-hot-entry/extraction-proof.json)
records the body hash and parameter boundary. Error order, history retirement,
precedence comparisons, publication, rollback and observers remain intact.

Linux amd64 Go **1.25.14** compilation reduces the entry's `SUBQ` stack
reservation **1,032 → 272 bytes**. These numbers exclude fixed BP/return
slots. The [frame evidence](benchmarks/20261017-condense-hot-entry/frame-size.json)
retains the disassembly prologues. Existing-boundary calls also enter the new
helper; a smaller entry does not establish less total work for those calls.
The throughput comparison below rejects the combined result.

## Twenty-pair result

The repository randomized runner uses `GOMAXPROCS=1`, CPU 6, 6 GiB Docker,
`GOMEMLIMIT=5GiB`, `-count=1`, `750ms` and `-benchmem`. Each seed runs in its
own process; baseline/candidate order alternates and both benchmark locks
stay held. Go seeds are **31301–31320**, YAML **31401–31420**. All **440
timed rows** remain in the evidence.

| Compact path | Before → after | Time direction | p |
| --- | ---: | ---: | ---: |
| Go R4 | 780.388 → 793.161 µs | **+1.64%** | **<.001** |
| Go rewrite | 1.701178 → 1.723420 ms | **+1.31%** | **<.001** |
| Go query | 8.105179 → 8.191453 ms | **+1.06%** | **<.001** |
| Go language | 8.443414 → 8.510533 ms | **+0.79%** | **.001** |
| Go large | 86.731155 → 87.062323 ms | +0.38% | .149 |
| Ordinary YAML attempt plus fallback | 190.429 → 190.366 µs | −0.03% | .862 |
| Owned YAML R4 | 335.139 → 338.615 µs | **+1.04%** | **.017** |
| Owned YAML 8 KiB | 1.303099 → 1.311982 ms | +0.68% | .056 |

Only the bold regressions are supported. Allocation counts stay exact: Go
**22 / 21 / 120 / 239 / 1,705**, YAML **37 / 109 / 336**, in table order.
All byte medians stay exact, including **150,418 B/op** for Go language and
**44,313 B/op** for the owned stream.

| Default Go control | Before → after | Time direction | p |
| --- | ---: | ---: | ---: |
| Full | 3.925005 → 3.867734 ms | −1.46% | <.001 |
| First edit | 98.764 → 98.979 µs | +0.22% | .076 |
| No edit | 3.1250 → 3.2235 ns | +3.15% | <.001 |

Default-control movements remain unattributed. No-edit stays **0 B/op / zero
allocations**. These controls do not rescue the compact regressions. Every
metric and raw sample is in the [measurement bundle](benchmarks/20261017-condense-hot-entry/measurement.json).

## Correctness and limits

All **412 ordinary plus two owned-YAML ledger rows** are byte-identical
before/after, preserving routes and work/reuse counters. The
[counter reference](benchmarks/20261017-condense-hot-entry/counter-reference.json)
links the complete published values. Core normal, phase-zero-A, race and
386 tests pass, as do focused scheduler/callback checks, Go/YAML and
owned-YAML edit invariants, locked-C fresh/edit/error checks, YAML EOF and
version-ownership regressions, and contextual-Go C checks. Existing fold,
history, ownership, precedence and rollback tests also run against baseline.

No counter exception, threshold change, full-fleet, strict-census or RSS
qualification is claimed for this rejected source. The
[source reference](benchmarks/20261017-condense-hot-entry/source-reference.json)
pins both candidate files and the 3,508-file baseline through the preceding
paired-cursor source reference. Apply the reconstruction patch with
`git apply --unidiff-zero`. Check scripts, gate-log hashes/tails, raw timings,
source proof and compiler evidence remain available. Ordinary YAML routing,
allowlists and graduation are unchanged.
