# Native receipt publication: qualified on Linux amd64 and ARM64

Native publication previously walked the emitted records separately to save original symbols, authenticate aliases, validate geometry, publish lookahead receipts, and clear rejection markers. This seven-file change combines the post-construction receipt work into one walk and saves original symbols when their incoming field slots are consumed. It validates each record against its exact arena slot, then addresses receipt words directly instead of searching the slabs for every node. No persistent buffer or language-name special case is added. The generic publisher retains its duplicate-projection checks.

Twenty matched, alternating pairs establish these improvements over fused publication at `aced544fc3027e33e5486631846fd232ec10f406`:

| Linux amd64 workload | Before | Candidate | Result |
| --- | ---: | ---: | --- |
| Compact Go fresh 1 MiB | 850.8 ms | 785.4 ms | **−7.68%, p<.001** |
| Compact Go LR | 117.6 ms | 112.9 ms | **−4.02%, p=.028** |
| Compact Go full DFA | 10.015 ms | 9.644 ms | Inconclusive, p=.063 |
| Compact Go query source | 10.95 ms | 10.70 ms | Inconclusive, p=.096 |
| Primary Go full DFA | 4.350 ms | 4.208 ms | Inconclusive, p=.429 |
| Primary Go edit | 90.59 µs | 97.20 µs | **+7.29% median**, inconclusive, p=.081 |
| Ordinary YAML R4 | 292.4 µs | 292.3 µs | Inconclusive, p=.883 |
| Ordinary YAML 8 KiB EOF | 1.101 ms | 1.114 ms | +1.18% median, inconclusive, p=.414 |

All other timing comparisons are inconclusive, including compact no-edit's +4.85% median (p=.337). No allocation comparison establishes a change. Both no-edit cases retain zero bytes and allocations. Every 10% median time, byte, and allocation ratchet passes. The change is adopted for its large-Go and LR gains; it does not establish a YAML speedup. The complete [compact Go](benchstat-compact-go.txt), [primary Go](benchstat-go.txt), and [YAML](benchstat-yaml.txt) tables retain every directional result and control.

All **206 work/reuse/routing ledgers remain exact**: Go R4 public nodes 893→893, large Go 593,457→593,457, YAML R4 232→232, and EOF 925→925. No counter exception is required. Before timing, the candidate passes 412 invariant sessions / 29,664 edit steps, 1,236 standard locked-C checks, 12 large-Go C comparisons, YAML ownership/EOF probes, 20 memory checks, core and focused-root races, and actual 386 execution. The receipt-storage package also passes units, race checks, and 386 execution. New regressions cover slab boundaries, duplicate/reordered/foreign records, changed points/symbols/state/source, cancellation, alias identity, retained capacity, and rejected budget growth.

The latest Cognociente report was rechecked and remains [comment 6311551850](https://github.com/odvcencio/gotreesitter/issues/454#issuecomment-6311551850). Its exact 137 KiB Go witness preserves all 12 independent incremental/fresh agreements and eight clean C comparisons. The same four malformed-delete C shape differences remain unresolved; this does not cover the report's entire Windows matrix.

All 126 cold-RSS probes pass the 400 bytes/source-byte ceiling. Median RSS changes are Go +0.35%, ordinary YAML +0.29%, and owned YAML +0.09%. The existing warmed forced-legacy Go RSS failure remains a hold. The container exits zero without OOM, and all source hashes remain unchanged at completion.

The baseline has 3,561 effective Go files, reproduced with five source overlays over the existing frozen directory; the candidate has 3,566. Module files match. The repository randomized driver uses 20 explicit seeds per suite, one process per seed, matched workload order, alternating role order, GOMAXPROCS=1, count=1, 750 ms, and benchmem. Seeds are 230001–230020 for YAML, 231001–231020 for primary Go, and 232001–232020 for compact Go. The primary trio retains its unchanged test-only seeded wrapper. Separate RSS processes use 233001–233020 after timing. The pinned Go 1.26.6 container has one CPU, 4 GiB memory, GOMEMLIMIT=3GiB, no network, and the shared workload lock.

The [qualification](qualification.json), [source patch](source.patch.gz), and [complete receipts](receipts.tar.gz) preserve candidate source files, every overlay backing and benchmark control, raw samples, counter comparisons, tests, RSS, and separate [Go](go-cpu-top.txt) and [YAML](yaml-cpu-top.txt) profiles. Profiles attribute work within the candidate; they are not comparative timing evidence. The container was removed after audit.

## Linux ARM64 qualification on Mac

The approved Mac run qualifies the exact runtime committed at `6d6ead01873d54d8cda839947e04b387474190ca`. Its baseline and candidate hashes match all effective amd64 source files, and the candidate matches all 3,566 tracked Go files at that commit. Source hashes remain unchanged at completion. This is Linux ARM64 in Docker on the Mac, not native Darwin.

Twenty matched, alternating pairs per suite establish three compact-Go gains over the same fused-publication baseline:

| Linux ARM64 workload | Before | Candidate | Result |
| --- | ---: | ---: | --- |
| Compact Go fresh 1 MiB | 1.171 s | 1.139 s | **−2.74%, p=.024** |
| Compact Go query source | 17.89 ms | 17.27 ms | **−3.46%, p=.001** |
| Compact Go LR source | 184.2 ms | 177.2 ms | **−3.80%, p=.003** |
| Compact Go full DFA | 16.49 ms | 16.25 ms | Inconclusive, p=.149 |
| Primary Go full DFA | 5.855 ms | 5.835 ms | Inconclusive, p=.820 |
| Primary Go edit | 523.0 µs | 524.1 µs | +0.21% median, inconclusive, p=.565 |
| Ordinary YAML R4 | 365.5 µs | 366.4 µs | +0.24% median, inconclusive, p=.542 |
| Ordinary YAML 8 KiB EOF | 1.374 ms | 1.376 ms | +0.13% median, inconclusive, p=.820 |

All remaining timing comparisons are inconclusive, including compact Go edit +0.06%, legacy YAML R4 +0.79%, and legacy YAML EOF +0.52%. No allocation comparison establishes a change. No-edit cases remain at zero bytes and allocations, every timed work/routing metric is exact, and all 10% median ratchets pass. Full tables retain every result and control: [compact Go](arm64-benchstat-compact-go.txt), [primary Go](arm64-benchstat-go.txt), and [YAML](arm64-benchstat-yaml.txt). This adds evidence for Go gains; it establishes no YAML throughput gain.

Before timing, ARM64 passes core and receipt-storage units, 42 focused root tests, four Go/YAML invariant sessions / 288 edit steps, 20 memory checks, and exact Go/YAML counter ledgers. It does not rerun locked-C, race, 386, or RSS checks; those remain the separate source-identical amd64 evidence above. Seeds are 235001–235020 for YAML, 236001–236020 for the primary Go trio, and 237001–237020 for compact Go, totaling 300 rows per role. The same repository runner uses matched orders, alternating roles, GOMAXPROCS=1, count=1, 750 ms, and benchmem. The container has one CPU, 4 GiB memory, GOMEMLIMIT=3GiB, no network, and the Mac's shared compute lock.

The [ARM64 receipts](arm64-receipts.tar.gz) preserve raw measurements, controls, every reused and new overlay backing, source pins, test logs, counter comparisons, analysis, and container audit. The container exits zero without OOM and is removed after audit. Its three executables are hashed before deletion, freeing 126,047,882 bytes; source trees, evidence, and shared caches remain intact. The original amd64 archive remains unchanged.

This is not a main-based or Go/C timing comparison. Native Windows and Darwin remain unqualified. Combined main-based qualification, earlier Go allocation and warmed-RSS holds, and the four malformed-delete differences remain outstanding. All merges and graduation stay on hold.
