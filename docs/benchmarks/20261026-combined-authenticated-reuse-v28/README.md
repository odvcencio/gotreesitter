# Combined native Go reuse: V28 relative checks; RSS held

V28 combines authenticated native Go identifier reuse and direct token proof with grouped read-history reservation and a bounded parent identity set. The authority belongs to its decoded language instance under the immutable-language contract; unsupported edits retain fresh verification. Parent identity storage is charged to scratch accounting, and checked arithmetic covers 32-bit builds. The tested snapshot is identified by input-manifest SHA-256 `835ba71dedcec7b284547d78acf40da944c08588ebbc2f82ba668a3e47d10d82`, on local integration checkpoint `9c9d5b2065497a3ecfe2279a6d340cb5082a4ecc`, against baseline `e079b642fa1c11a3c9bd939d367e05598fc8162f`.

The relative checks below pass for this snapshot. **The existing absolute warm RSS hold remains:** on Linux amd64, all 20 paired samples in each arm exceed the 419,436,400-byte ceiling; median peak RSS is 446,396,416 → 434,923,520 bytes. The earlier native ARM64 hold also remains. No language is graduated, no default is changed and main merges remain held. Linux results do not establish Windows latency for the [downstream report](https://github.com/odvcencio/gotreesitter/issues/454#issuecomment-6311551850).

| Linux amd64, 20 paired randomized seeds | Baseline | V28 | Result |
| --- | ---: | ---: | --- |
| Primary full parse | 4.059 ms | 4.113 ms | Inconclusive, p=0.277 |
| Primary numeric edit | 66.42 µs | 50.70 µs | -23.67%, p=0.000 |
| Primary no-edit | 3.160 ns | 3.031 ns | -4.07%, p=0.000; zero allocations |
| Report-fixture edit | 67.976 ms | 5.077 ms | -92.53%, p=0.000 |
| Report-fixture profiled edit | 68.680 ms | 5.590 ms | -91.86%, p=0.000 |
| Report-fixture full | 58.003 ms | 59.491 ms | +2.57% median, inconclusive p=0.060 |
| Report-fixture full allocations | 1394 B / 6 allocations | 1394 B / 6 allocations | Unchanged |
| Profiled reuse selection | 402.9 µs | 529.8 µs | +31.47%, p=0.000 |
| Warm 1 MiB fresh parse | 557.2 ms | 568.8 ms | +2.07%, p=0.040 |
| Warm 1 MiB allocations | 96/op | 99/op | +3.12%; bytes/op -3.20% |

The report fixture is 140368 bytes (140288-byte generator target), SHA-256 `5434575bb249338937331c3756e10dcec34237d107f523ea315996fb6de0ef8f`. The warm witness is 1,048,591 bytes (1 MiB target), SHA-256 `76c70b62127085d3c791ba98a24cba9f22bd343e40afb43522bbb5041cede524`. YAML's 20-pair comparison measured full -1.39%, edit -1.80% and no-edit -4.33%; bytes and allocations were unchanged. These results come from pinned Linux amd64 Docker runs on an Intel Core Ultra 9 285 with GOMAXPROCS=1. The Go and YAML comparisons use one process per seed and 750 ms benchmark time. The warm 1 MiB comparison uses 20 paired seeds and two measured iterations; its direct-child RSS measurements are separate.

| Deterministic comparison | Baseline | V28 |
| --- | ---: | ---: |
| Go/YAML and fleet ledger exports | 412 routes | Identical |
| Clean identifier insert/replace shifts | 72,200 | 1,792 |
| Clean identifier insert/replace reductions | 75,729 | 1,803 |
| Clean identifier insert/replace table lookups | 345,393 | 12,541 |
| Complete operations | 24 per arm | Work/reuse direction passed |

Memory capacity/accounting gauges differ in 48 records: scratch baseline and allocated bytes each rise in 12 records by 524,288 or 1,048,576 bytes; arena baseline rises in eight and falls in four, while arena allocated rises in ten and falls in two. These gauges are not primitive work counters. The archive retains the complete machine-readable records and the observed heap/RSS data.

Correctness includes 412 fleet invariant routes, 1,236 C fixture checks, 127 matching C 0.25.1/0.27.1 cases, focused and edit-chain races, 27 memory tests and 32-bit checks. The [V13 rejection](../20261026-combined-authenticated-reuse-v13/README.md) is retained separately. The archive's `rss-next-target/notes.md` records the 109,053,464-byte node reservation and 128 MiB full-arena eviction as a future architecture question; it does not waive the RSS hold.

[Evidence archive](evidence.tar.gz) includes source patch and new files, source/control manifests, commands, raw samples, counter/gauge records, correctness receipts and container states. Run `python3 docs/benchmarks/20261026-combined-authenticated-reuse-v28/validate-evidence.py` to validate indexed hashes, correctness records, matched pairs and the RSS hold. The local integration checkpoint must be published for the source patch to be independently reconstructed.
