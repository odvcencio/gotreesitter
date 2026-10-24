# Native construction: not promoted alone

This five-file prototype combines child containment checks, checked precedence addition, child-pointer copying, direct fields, and parent links in one native constructor. Field and supertype updates avoid full public-node readback. Scanner checkpoints, replay metadata, aliases, ownership, and memory accounting retain their existing contracts.

The change is **not promoted as a standalone runtime improvement**. Twenty paired samples establish a compact Go LR gain, a primary Go full-parse regression, and no broader throughput or allocation gain. A subsequent frontier publication change needs its own qualification; these receipts do not cover it.

| Linux amd64 workload | Before | Candidate | Result |
| --- | ---: | ---: | --- |
| Compact Go LR | 129.6 ms | 123.8 ms | −4.45%, p=0.046 |
| Compact Go full DFA | 10.72 ms | 10.40 ms | Inconclusive, p=0.327 |
| Compact Go fresh 1 MiB | 908.7 ms | 898.2 ms | Inconclusive, p=0.383 |
| Compact Go query source | 11.75 ms | 11.39 ms | Inconclusive, p=0.265 |
| Primary Go full DFA | 4.207 ms | 4.481 ms | **+6.52%, p=0.014** |
| Ordinary YAML R4 | 244.6 µs | 251.3 µs | Inconclusive, p=0.989 |
| Ordinary YAML 8 KiB EOF | 933.7 µs | 928.8 µs | Inconclusive, p=0.369 |

Owned YAML, edits, and no-edit timings are also inconclusive. Primary Go full allocated bytes rise 0.09% (p=0.038). All other allocation comparisons are inconclusive or equal, and no-edit reparses stay at zero bytes and allocations. Every 10% median ratchet passes. Complete tables retain all directional results: [compact Go](benchstat-compact-go.txt), [primary Go](benchstat-go.txt), and [YAML](benchstat-yaml.txt).

All 206 work/reuse/routing ledgers are exact: Go R4 public nodes remain 893→893 and large Go 593,457→593,457; YAML R4 remains 232→232 and EOF 925→925. Before timing, the candidate passes 412 invariant sessions / 29,664 edit steps, 1,236 standard locked-C checks, 12 large-Go C comparisons, YAML ownership/EOF probes, 20 memory checks, core and focused-root races, and actual 386 execution. Unit regressions compare parent metadata with the general constructor and retain invalid-handle, child-containment, and intermediate score-overflow rejection.

The exact latest Cognociente witness preserves all 12 independent incremental/fresh agreements and eight clean C agreements. The same four malformed-delete C shape failures remain unresolved. All 126 cold-RSS probes pass the 400 bytes/source-byte ceiling; median RSS changes are Go +0.79%, ordinary YAML −0.39%, and owned YAML −1.86%. This does not clear the existing warmed forced-legacy Go RSS hold.

The baseline is fused publication at `2cfe957d717cb34169677ab516d836fa37db04fb`, reproduced through five source-overlay files over the existing frozen source directory. All 3,561 effective baseline Go hashes match the preceding qualification; module files match too. The candidate contains 3,564 Go files. Source hashes remain unchanged at completion. No checkout copies were created.

The repository randomized driver runs 20 explicit seeds per suite, one process per seed, matching workload order and alternating role order, GOMAXPROCS=1, count=1, 750 ms, and benchmem. Seeds are 220001–220020 for YAML, 221001–221020 for primary Go, and 222001–222020 for compact Go. The primary trio retains its unchanged test-only seeded wrapper. RSS uses 223001–223020 in separate processes after timing. The pinned Go 1.26.6 container has one CPU, 4 GiB memory, GOMEMLIMIT=3GiB, no network, and the shared workload lock. It exits zero without OOM and is removed after audit.

[Qualification](qualification.json), [source patch](source.patch.gz), and [complete receipts](receipts.tar.gz) preserve source pins, every overlay backing, controls, raw samples, counters, tests, RSS, and separate [Go](go-cpu-top.txt) and [YAML](yaml-cpu-top.txt) profiles. This is not a main-based or Go/C timing comparison. ARM64 and native platform gates remain unqualified; no merge or graduation is authorized.
