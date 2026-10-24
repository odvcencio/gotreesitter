# Reduction pop construction: not adopted

The prototype constructs a one-child reduction result directly instead of staging and reversing three temporary vectors. Longer single-link reductions reuse the already authenticated leftmost child's geometry instead of reading every subtree again during the reverse copy. It preserves forward score-overflow checks, fork order, extras, recovery markers, graph validation, and path limits.

Correctness and counters pass, but twenty paired samples per suite establish **no throughput or allocation improvement**. The three-file patch is archived here and is not installed in the runtime. The retained runtime improvement is [fused native publication](../20261023-native-fused-publication/README.md).

| Linux amd64 workload | Before | Candidate | Timing result |
| --- | ---: | ---: | --- |
| Compact Go, full DFA | 11.04 ms | 11.09 ms | Inconclusive, p=0.925 |
| Compact Go, fresh 1 MiB | 961.1 ms | 921.4 ms | Inconclusive, p=0.445 |
| Compact Go, LR source | 132.2 ms | 126.0 ms | Inconclusive, p=0.369 |
| Compact Go, query source | 11.66 ms | 12.09 ms | Inconclusive, p=0.327 |
| Ordinary YAML, R4 | 228.1 µs | 225.7 µs | Inconclusive, p=0.698 |
| Ordinary YAML, 8 KiB EOF | 871.8 µs | 866.5 µs | Inconclusive, p=0.925 |

Allocation counts remain unchanged. All 206 work/reuse/routing ledgers and timed work vectors are identical. Go R4 public nodes remain 893→893 and large Go 593,457→593,457; YAML R4 remains 232→232 and EOF 925→925. No-edit reparses allocate zero. Every 10% median ratchet passes. The complete tables retain all directional changes: [compact Go](benchstat-compact-go.txt), [primary Go](benchstat-go.txt), and [YAML](benchstat-yaml.txt).

Before timing, both source variants pass the full core suites in normal and shadow configurations. The candidate passes core and focused-root races, actual 386 execution, 412 invariant sessions / 29,664 edit steps, 1,236 standard locked-C checks, 12 large-Go C comparisons, YAML ownership/EOF probes, and 20 memory checks. The exact Cognociente witness preserves 12 independent incremental/fresh agreements and eight clean C agreements; four known malformed-delete C shape failures remain unresolved.

All 126 cold-RSS probes pass the 400 bytes/source-byte ceiling. Median changes are Go −0.40%, ordinary YAML +0.65%, and owned YAML −0.39%. These probes do not clear the existing warmed forced-legacy Go failure.

The comparison is independent of fused publication: both roles use the existing frozen `native-record-receipts-v2` source, and only the candidate receives the three-file reduction overlay. No checkout copies were created. The primary Go trio uses the repository's unchanged seeded wrapper and original benchmark functions, with its test overlay combined with the candidate source overlay. Its output retains the wrapper prefix. All original/prepared benchmark hashes are recorded.

The repository randomized driver runs 20 explicit seeds per suite, one process per seed, matching workload order and alternating role order, GOMAXPROCS=1, count=1, 750 ms, and benchmem. Seeds are 211001–211020 for YAML, 212001–212020 for primary Go, and 213001–213020 for compact Go. Timing and RSS run separately. The pinned Go 1.26.6 container has one CPU, 4 GiB memory, GOMEMLIMIT=3GiB, no network, and the shared workload lock. Source hashes remain unchanged; the container exits zero without OOM and is removed after audit.

[Qualification](qualification.json), [complete source patch](source.patch.gz), and [receipts](receipts.tar.gz) preserve the source pins, controls, raw timing, counters, tests, RSS, and profiles. Every source-overlay backing file is included. This experiment has no ARM64 qualification and does not qualify a combination with fused publication. Main-based qualification and existing correctness, RSS, platform, and graduation holds remain unchanged.
