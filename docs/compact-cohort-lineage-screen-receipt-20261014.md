# Cohort lineage publication: Go gain does not justify the control regression

The cohort-publication prototypes stay outside the runtime. Twenty paired
samples measure roughly 1% less public large-Go parse time, with exact work and
reuse counters, but the default full-DFA control becomes slower. A separate
twenty-pair control-only run confirms a **3.68% regression**. This trade is not
promoted. The qualified YAML identity-reduction change in
[PR #1485](https://github.com/odvcencio/gotreesitter/pull/1485) remains unchanged.

Baseline for these trials: `70b27618117e60eecd6e1aaa26552aa81b37f22a`.
The earlier per-header fusion screen in evidence directory
`20261014-fused-lineage-publication` used an older baseline and established no
gain. This experiment batches multiple headers; its results are independent.

## Scope and correctness

The initial owner-only batch passes focused correctness but reaches just
**8 / 191,884** large-Go head publications. Its partial timing run is deliberately
stopped at Docker exit 143 and excluded. No speed claim follows from it.

The expanded batch publishes up to eight headers under one scheduler-session
check, retaining each header's owner, scalar-rank, alternative-set and reference
operations in their original order. Failure returns its exact index and stage;
the root caller preserves completed-prefix persistence caches and owner-error
context. Single-header, non-fresh, wider and creation-sequence overflow cases
retain the scalar path. No scheduling, ownership, pruning, route or cap rule is
relaxed.

The first full version shares a newly extracted scalar implementation. The
direct version leaves the original scalar API untouched and fuses the same
operations inside the batch. Both pass the full core package, focused root,
footprint, recovery and canonical-lifecycle tests, Go/YAML and owned-YAML edit
invariants, Go C fixtures and strict R4 comparison, and the YAML document-stream,
ownership and recovery C witnesses. Added regressions cover rollback, ignored
errors, expired/foreign owners, bad references, owner collisions, bounded input,
zero warm allocations and exact failure-prefix caches. One owner-only test's
expected diagnostic text was corrected; the failed expectation is excluded.

| Deterministic counter | Before → after, all three trials |
| --- | --- |
| Ordinary work/reuse ledger | All 412 rows identical |
| Candidate Go full tokens / nodes / versions | 589 / 1,251 / 3 → identical |
| Candidate Go edit tokens / nodes / reused bytes / splices | 374 / 1,553 / 1,784 / 266 → identical |
| Owned YAML full/edit nodes | 484 / 484 → identical |
| Default YAML full/edit nodes | 288 / 480 → identical |
| No-edit allocations | 0 B/op / 0 allocations → identical |
| Ordinary YAML ownership decline | Unchanged |

The public-path census records 51,302 large-Go batches carrying 122,533 head
bindings and 122,525 provenance publications, with 69,351 scalar owner calls and
69,242 scalar lineage calls remaining. This roughly halves session-entry calls
for metadata publication. YAML R4 uses 74 batches for 148 head bindings; the
8 KiB witness uses 274 batches for 548. These are implementation counts, not
published work-counter reductions or a throughput estimate.

## Twenty paired samples per screen

Each comparison uses the repository randomized runner, alternating role order,
one process per explicit shuffle seed, `GOMAXPROCS=1`, `-count=1`,
`-benchtime=750ms` and `-benchmem`. Go and YAML run separately. All timing roles
complete twenty samples, source hashes remain exact, and completed timing
containers exit zero without OOM or timeout. Neither campaign's gains are added
to the other or to #1485.

| Benchmark | Shared-helper version | Direct version |
| --- | --- | --- |
| Public Go 20 KiB | 9.953→9.903 ms, inconclusive, p=.183 | 10.014→9.965 ms, inconclusive, p=.142 |
| Public Go 235 KiB | 106.7→105.5 ms, **−1.18%, p=.001** | 107.7→106.6 ms, **−1.00%, p=.023** |
| Compact Go R4 | 830.3→826.1 µs, −0.51%, p=.043 | 840.4→835.4 µs, −0.60%, p=.046 |
| Default full-DFA control | 3.876→4.029 ms, **+3.95%, p<.001** | 3.902→4.073 ms, **+4.38%, p<.001** |
| Default edit control | 98.87→98.60 µs, inconclusive, p=.221 | 99.53→99.63 µs, inconclusive, p=.409 |
| Default no-edit control | 3.134→3.200 ns, **+2.14%, p<.001** | 3.145→3.108 ns, −1.21%, p=.001 |
| Owned YAML R4 | 377.0→379.3 µs, **+0.60%, p=.021** | 380.2→379.0 µs, inconclusive, p=.718 |
| Owned YAML 8 KiB | 1.464→1.468 ms, inconclusive, p=.841 | 1.471→1.465 ms, inconclusive, p=.149 |

Allocation medians stay unchanged throughout. Public Go remains at
141,139 / 3,295,728 B and 205 / 3,092 allocations; owned YAML remains at
12,936 / 44,601 B and 111 / 345 allocations. In the direct version, full-DFA
bytes rise **1,159→1,160 B/op (+0.09%, p<.001)** with six allocations unchanged.
The shared-helper full-DFA byte medians are equal, with a distribution difference
at p=.005. Public-node construction counts stay 11,326 / 107,964.

## Default-parser control isolation

The full-DFA increase appears across mixed-suite positions, including samples
where it runs first. The independent control-only comparison removes compact
benchmarks from execution while keeping the same pinned runtime:

| Control | Before → after | Result |
| --- | --- | --- |
| Full DFA | 3.887→4.030 ms | **+3.68%, p<.001** |
| Single-byte edit | 98.93→99.03 µs | Inconclusive, p=.265 |
| No edit | 3.145→3.109 ns | −1.14%, p<.001 |

Full-DFA bytes again rise 1,159→1,160 (+0.09%, p=.005); allocations remain six.
No-edit remains allocation-free. Benchmark source and the unset global route
switch keep these controls on the default path. The control regression is
measured; its cause is not established and is not dismissed as noise.

These changes stay below the 10% median ratchets, but that alone does not make
the overall trade worthwhile. Full fleet/strict-C-corpus, race, large-RSS and
source-guard qualification is withheld. A further two-head spilled/overflowed
reference differential and qualification scripts were prepared but not run;
they supply no passing evidence. No counter exception is requested.

Evidence under `/home/draco/.local/state/gotreesitter-evidence/`:

- `20261014-head-owner-batch`: low-coverage candidate, focused checks, census
  and explicitly excluded partial timing.
- `20261014-head-lineage-batch`: shared-helper candidate, focused regressions,
  complete paired samples, census, source pins and benchstat.
- `20261014-cohort-lineage-publication`: direct candidate, completed paired
  screens, isolated control comparison, exact runtime pins, source patches and
  qualification limits.

Both complete prototypes remain external. Ordinary YAML ownership, the Go
throughput gap and compact graduation remain open. No merge occurs.
