# Frontier publication: not promoted

This eight-file experiment removes public-node readback and finalizes each emitted node's metadata once. It is **not promoted**: twenty paired samples establish a small ordinary-YAML EOF improvement, no compact-Go throughput gain, small primary-Go timing regressions, and higher Go allocation costs.

| Linux amd64 workload | Before | Candidate | Result |
| --- | ---: | ---: | --- |
| Ordinary YAML 8 KiB EOF | 806.0 µs | 793.8 µs | −1.51%, p=.004 |
| Ordinary YAML R4 | 212.7 µs | 211.0 µs | Inconclusive, p=.121 |
| Compact Go full DFA | 14.97 ms | 14.15 ms | Inconclusive, p=.289 |
| Compact Go fresh 1 MiB | 1.274 s | 1.276 s | Inconclusive, p=.799 |
| Compact Go query source | 16.84 ms | 16.54 ms | Inconclusive, p=.355 |
| Compact Go LR | 183.9 ms | 177.5 ms | Inconclusive, p=.583 |
| Primary Go full DFA | 3.938 ms | 3.969 ms | +0.79%, p=.035 |
| Primary Go edit | 85.76 µs | 86.18 µs | +0.49%, p=.006 |
| Primary Go no edit | 3.071 ns | 3.183 ns | +3.68%, p<.001 |

Large-Go allocated bytes rise 0.85% (p=.021). LR bytes rise 1.61% and allocations rise 1,807→1,817 (+0.55%, p<.001). Other allocation comparisons are inconclusive or equal. Both no-edit cases remain at zero bytes and allocations. All 10% median ratchets pass. Compact-Go timing dispersion is wide; the lower full/LR medians do not establish a win. Full tables retain all results and controls: [compact Go](benchstat-compact-go.txt), [primary Go](benchstat-go.txt), and [YAML](benchstat-yaml.txt).

All 206 strict work/reuse/routing ledgers remain exact: Go R4 public nodes 893→893, large Go 593,457→593,457, YAML R4 232→232, and EOF 925→925. Before timing, the candidate passes 412 invariant sessions / 29,664 edit steps, 1,236 standard locked-C checks, 12 large-Go C comparisons, YAML ownership/EOF probes, 20 memory checks, core and focused-root races, and actual 386 execution. New tests require final child metadata before attachment, reject repeated final writes, and cover hidden fields, extras, aliases, supertypes, score overflow, containment, and scratch accounting.

The exact latest Cognociente witness preserves all 12 independent incremental/fresh agreements and eight clean C comparisons. The same four malformed-delete C shape differences remain unresolved. All 126 cold-RSS probes pass the 400 bytes/source-byte ceiling. Median RSS changes are Go +2.60%, ordinary YAML −0.55%, and owned YAML −0.12%; these do not clear the warmed forced-legacy RSS hold.

The frontier publisher keeps unfinished node metadata in pointer-free traversal scratch. Hidden unary folds, fields, aliases, and supertypes update that metadata before a child is attached. Every emitted node receives one final metadata write; publication performs no public-node readback. Child containment and left-to-right checked precedence addition remain mandatory. The native constructor attaches finalized children and records fields, parent links, checkpoints, and audit counters in one loop. Scratch capacity remains part of memory accounting.

The comparison uses fused publication at `3d55d3fd7c85954249a8ec944a5025cb86733cf7`, reproduced with five source overlays over the existing frozen source. All 3,561 effective baseline Go hashes match that commit; the candidate has 3,565 Go files and an eight-file delta. This is not a main-based or Go/C timing comparison.

The repository randomized driver uses 20 explicit seeds per suite, one process per seed, matched workload order, alternating role order, GOMAXPROCS=1, count=1, 750 ms, and benchmem. Seeds are 225001–225020 for YAML, 226001–226020 for primary Go, and 227001–227020 for compact Go. The primary trio retains the unchanged test-only seeded wrapper. RSS uses separate processes and seeds 228001–228020 after timing. The pinned Go 1.26.6 container has one CPU, 4 GiB memory, GOMEMLIMIT=3GiB, no network, and the shared workload lock.

No merge or graduation is authorized. Main-based combined qualification, native platform coverage, existing Go allocation and warmed forced-legacy RSS holds, and the four malformed-delete C differences remain outstanding. This bucket does not qualify ARM64 or native Darwin.

The source hashes remain unchanged at completion. The container exits zero without OOM and is removed after audit. The [qualification](qualification.json), [source patch](source.patch.gz), and [complete receipts](receipts.tar.gz) retain the eight candidate source files, all source overlays and benchmark controls, raw samples, counters, test logs, RSS, and separate [Go](go-cpu-top.txt) and [YAML](yaml-cpu-top.txt) profiles. Profiles attribute work within this candidate; they are not comparative speed evidence. The subsequent receipt-publication experiment requires separate qualification.
