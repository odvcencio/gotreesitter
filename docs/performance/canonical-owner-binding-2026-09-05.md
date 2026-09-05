# Remove canonicalization callback allocation

Date: 2026-09-05.
Baseline: `2f72bae21f30f17fb7638a236b2414fa499a1998`.
Candidate: `b471e9224cc66dc32068e4494ca658a29da77bd0`.
Benchmark driver: `4e8d780c0dfecfff2b14850bc750e22b8ab15fa0`.

The change reduces allocations without a significant timing change in the measured workloads.
Parsing the `query_compile` fixture falls from 13,731 to 1,360 allocations per parse, a 90.10 percent reduction.
Its allocated bytes fall by 62.43 percent.
The required generated Go trio also shows no significant timing change.

| Benchmark | Baseline time | Candidate time | Timing significance |
| --- | ---: | ---: | ---: |
| `AdmissionCandidateGoQueryCompileWarmRoute` | 19.18 ms | 19.60 ms | p=0.799 |
| `GoParseFullDFA` | 18.00 ms | 18.05 ms | p=0.495 |
| `GoParseIncrementalSingleByteEditDFA` | 1.623 us | 1.519 us | p=0.805 |
| `GoParseIncrementalNoEditDFA` | 3.401 ns | 3.084 ns | p=0.640 |

Benchmark names omit their common `Benchmark` prefix.
Times and allocation values are medians from twenty samples per side.

| Benchmark | Baseline bytes per parse | Candidate bytes per parse | Baseline allocations | Candidate allocations |
| --- | ---: | ---: | ---: | ---: |
| `AdmissionCandidateGoQueryCompileWarmRoute` | 309.6 KiB | 116.3 KiB | 13,731 | 1,360 |
| `GoParseFullDFA` | 547.5 KiB | 294.5 KiB | 15,548.5 | 42 |
| `GoParseIncrementalSingleByteEditDFA` | 0 | 0 | 0 | 0 |
| `GoParseIncrementalNoEditDFA` | 0 | 0 | 0 | 0 |

Both full-parse allocation reductions have p<0.001.
Every query benchmark sample reports one compact route per parse and zero fallbacks.
Read the [complete comparison](canonical-owner-binding-2026-09-05/trio-benchstat.txt) for confidence intervals and all metrics.

Each canonicalization previously stored a newly bound `versionLexerStateEqual` method in scratch.
The earlier allocation profile assigns all sampled canonicalization bytes to that assignment.
The change stores the scheduler pointer and calls the same equality method directly.
The existing deferred cleanup restores the previous pointer.

The change preserves:

- Request comparison, lexer snapshot identity, and recovery baseline equality.
- Boundary validation, header copies, mutation recording, and owner publication.
- Prior-owner restoration after success, errors, and panics.
- Pointer cleanup during version-state cleanup and scheduler reset.
- Rollback behavior and memory accounting, with no new backing storage.

Existing canonicalization, lexer ownership, footprint, and rollback controls pass on both revisions in Docker.
New candidate controls require zero steady-state wrapper allocations for one and two headers.
They also check semantic owner selection, mapped groups, failed publication, panic cleanup, and reset.
Independent source review found no actionable defect.

Separate Go, Python, and JavaScript certification runs pass against the locked C grammars.
The Go run also passes the canonical fixture preflight.

| Language | Direct compact cases | Required fallback cases | Smoke edit |
| --- | ---: | ---: | --- |
| Go | 3 | 1 recovery case | Exact tree and subtree reuse |
| Python | 4 | 1 recovery case | Exact tree and subtree reuse |
| JavaScript | 3, including recovery | 0 | Exact tree and subtree reuse |

These controls preserve the tested routes; they do not certify every grammar input or establish universal compact recovery coverage.
The [correctness receipts](canonical-owner-binding-2026-09-05/) include exact commands and container outcomes.
Each run exits successfully without a wall timeout or container memory failure.

The paired campaign uses:

- `GOMAXPROCS=1`, twenty seeds starting at one, and one process per seed.
- The same seed for each pair, alternating checkout order, and one campaign lock.
- `-count=1`, `-benchtime=750ms`, `-benchmem`, and the `gts_parsercorephase0` tag.
- Docker with four GiB memory, `GOMEMLIMIT=3GiB`, and CPU 2.
- Go 1.25.14 on Linux amd64 and an Intel Core Ultra 9 285.

Both raw outputs end with `# status: complete`.
The [source receipt](canonical-owner-binding-2026-09-05/source.json) records clean host checkouts because container Git metadata was unavailable.
The shared development host remains a measurement limitation.
The receipt directory includes the exact [campaign wrapper](canonical-owner-binding-2026-09-05/run-owner-paired.sh) and [paired driver](canonical-owner-binding-2026-09-05/run_randomized_benchmarks.sh).

Timing covers one real Go fixture, `query_compile`, plus the required generated Go trio.
This paired campaign has no timed C endpoint.
It does not establish new Go/C ratios or performance gains across other languages.

A separate `grammargen_lr` probe excludes compilation from maximum resident set size measurement.
Baseline memory is 151,888 KiB; candidate memory is 148,704 KiB.
Both processes exit successfully.
This single probe is a bounded memory observation, not evidence of a statistical improvement.
Its elapsed times are not timing comparison evidence.

The candidate query profile assigns 17.96 percent flat CPU time to `runtime.duffcopy`.
Scheduler footprint accounting accounts for 12.53 percent cumulative CPU time; active dispatch accounts for 50.46 percent.
These shares overlap and must not be added.
A [narrow allocation-profile query](canonical-owner-binding-2026-09-05/canonical-allocation-query.txt), with profile thresholds disabled, finds no sampled routine matching `canonicalizeOwnedWithMutation`.
Recovery symbol policy construction accounts for 31.94 percent of sampled allocated bytes.
The allocation profile includes setup; its percentages are not per-parse allocation totals.
Profiled timing is not comparison evidence.

Defer the next lineage-copy experiment.
Two calls to `invalidateReusedLineageProof` copy records before its guard checks whether reuse needs them.
The earlier query profile attributes 2.20 percent of sampled CPU time to those copies.
Recheck this attribution before changing the private helper to accept pointers.
Preserve proof invalidation, owner checks, rollback, and reset in that experiment.
This bounded opportunity does not establish a breakthrough-sized speedup.

The [receipt directory](canonical-owner-binding-2026-09-05/) contains raw samples, source identities, correctness logs, memory output, and CPU and allocation profiles.
Profile binaries are excluded.
Verify the receipt files with `sha256sum -c SHA256SUMS` from that directory.
