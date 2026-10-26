# Compact pointer-based forest construction: held

The private candidate reduces allocation bytes but still misses ordinary Go
throughput. It remains unpromoted. Production runtime, routing, dependency pins,
ledger rows and graduation status are unchanged; all main merges remain held.

It retains the [held construction changes](../20261025-compact-construction-screen/README.md),
uses an explicit built-in trivia contract, and composes forest summaries through
pointers and scalars instead of copying large forest values for each child.
Arbitrary callbacks, aliases, fields and existing contexts retain their guarded
paths. Runtime map SHA is
`6f2d518d0051e520644a32cf2991ada86231cc7d40cc952109956428172070fe`.

## Paired timing

The unchanged randomized runner completed **60 matched pairs / 120 processes**:
20 explicit shuffle seeds per cohort, 750 ms, count=1, GOMAXPROCS=1 and benchmem.
Both arms use the same benchmark wrapper. Only the candidate receives the
28-target runtime overlay. Docker uses pinned Go 1.26.6, one CPU, 4 GiB, no
network and the shared measurement lock. No work/diagnostic instrumentation is
enabled in measured binaries.

| Workload | Baseline median | Candidate median | Time change | Allocation bytes | Allocations |
| --- | ---: | ---: | ---: | ---: | ---: |
| Generated Go full | 7.145 ms | 7.181 ms | +0.51%, inconclusive (p=.174) | 261,543→139,396 | 22→16 |
| Generated Go edit | 67.484 µs | 64.662 µs | −4.18% (p<.001) | 407→406 | 5→5 |
| Go no-edit | 3.114 ns | 2.988 ns | −4.05% (p<.001) | 0→0 | 0→0 |
| Ordinary Go compact | 892.467 µs | 929.515 µs | **+4.15% (p<.001)** | 1,112→1,784 | 14→10 |
| Ordinary Go legacy control | 753.309 µs | 747.251 µs | −0.80%, inconclusive (p=.142) | 2,712→2,712 | 11→11 |
| Ordinary YAML compact control | 224.930 µs | 221.396 µs | −1.57%, inconclusive (p=.478) | 5,456→5,456 | 39→39 |
| Ordinary YAML legacy control | 157.111 µs | 155.877 µs | −0.79%, inconclusive (p=.565) | 1,440→1,440 | 16→16 |

Generated full-parse bytes fall **46.70%**, but ordinary Go bytes rise **60.43%**.
The ordinary Go throughput regression keeps this architecture out of production.
The YAML rows measure the unchanged control route, not the separate checkpoint
lease prototype. These are fresh paired comparisons against canonical source;
different historical screens are not paired against each other.

## Correctness and counters precede timing

Core release coverage passes 83 families / 173 records; raw-summary work/race
coverage passes 19 / 34 each. Public ownership/lifetime checks require genuine
leases on both original Go sources, one initial arena node and no Core-owned raw
records. Both sources match locked C 0.25.1 and the private C 0.27.1 oracle across
all public tree axes. Broader Go C checks pass 41 records plus the 127-record
original-report/recovery suite; unchanged YAML C controls pass 184 records.
Four 72-step Go/YAML edit sessions pass D8 and no-edit allocation remains zero.
Selected memory checks and the 13-record public reader/Copy/reset race run pass.

| Actual deterministic ledger row | Baseline | Candidate |
| --- | ---: | ---: |
| Candidate Go full new nodes | 892 | 1 |
| All other fields, four Go/YAML route rows | unchanged | unchanged |

All five real-source construction/read records exactly match the preceding
held construction screen, including tree digests, capacities, formation work
and warm traversal. No counter exception or reset is used.

The earlier V1 fused closure/formation loop performed one provisional formation
edge before a later child-certificate refusal: **0→1**. Its selected ledgers
passed, but strict failure-path work did not. **V1 was never timed.** V2 restores
closure-first validation and retains pointer-based composition. Focused release,
work and race runs each pass 12 families / 71 records, including exhaustive byte
pair trivia checks and exact refusal/state/ownership work equality. A separate
eight-witness same-policy frozen-reference suite passes in all three builds;
it covers actual trivia refusals, malformed extent/symbol, overflow and depth.
The observed late-certificate case now performs **0→0** formation edges.

The archive preserves the initial Docker socket denial and an overbroad guard
that listed unmounted C-oracle files. Neither attempt executed tests; repaired
control-only attempts retain the exact candidate runtime and pass. The optional
V1 strict physical-work adjudication remains unexecuted, not an observed failure.

## Evidence and remaining work

The archive contains exact source overlays, commands, guards, raw benchmark
rows, benchstat output and executed checks. It adds an explicit allowlist to the
two immutable source archives referenced by [summary.json](summary.json).
Validate offline:

```sh
python3 docs/benchmarks/20261025-compact-pointer-fold-screen/validate-evidence.py --source-root /home/draco/work/gotreesitter
```

No C speed ratio, large-file RSS, full-fleet, Windows/ARM, E-A exit or graduation
is qualified here. Consumer traversal was checked for correctness and work, not
timed as part of these parse benchmarks. The next construction experiment must
remove repeated closure/replay work at its actual construction seam while
preserving state provenance, memory charging and the refusal contract.
