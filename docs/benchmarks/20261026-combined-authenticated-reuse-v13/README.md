# Combined authenticated reuse V13: rejected

V13 is a combined-branch experiment, not a promoted runtime. Its Go report-fixture full parse rose from **6 to 7 allocations/op (+16.67%, p=0.000)**, exceeding the applicable 10% allocation-count ratchet on this comparison. Bytes rose from **1.371 to 7.414 KiB/op (+440.93%, p=0.000)**. The candidate is rejected despite the edit wins; the existing ARM64 RSS hold and all main-merge holds remain.

The baseline was `e079b642fa1c11a3c9bd939d367e05598fc8162f`. V13 used a dirty source snapshot on the local two-parent integration checkpoint `9c9d5b2065497a3ecfe2279a6d340cb5082a4ecc`; its complete input manifest has SHA-256 `2a6e0b1d940fc74ba24745a3a5054a5b753974395473ad9b89219c4eeac56e6a`. The checkpoint is not yet published. The archive retains the source patch, new files, baseline controls, source hashes, commands, raw logs and measurements. Source snapshot and baseline must be revalidated before reproduction.

| Linux amd64, 20 paired seeds | Baseline | V13 | Result |
| --- | ---: | ---: | --- |
| Primary numeric edit | 68.10 µs | 50.69 µs | -25.57%, p=0.000 |
| Report-fixture edit | 73.351 ms | 6.460 ms | -91.19%, p=0.000 |
| Report-fixture full parse | 62.15 ms | 65.14 ms | Inconclusive, p=0.086 |
| Report-fixture full bytes | 1.371 KiB/op | 7.414 KiB/op | +440.93%, p=0.000 |
| Report-fixture full allocations | 6/op | 7/op | +16.67%, p=0.000; gate failed |

The exact snapshot passed 412 fleet invariant routes, 1,236 C fixture checks, 127 matching pinned-C oracle cases, race checks and 27 memory tests. All 206 languages' two route ledger exports matched baseline, and the 24-operation work and reuse comparison passed. Separate 20-pair YAML timing showed inconclusive full/edit timings and unchanged bytes and allocations. These checks do not establish full v1 graduation or Windows latency.

The proposed ARM64 run was not performed. The old ARM64 RSS result is not cleared by this Linux measurement; any later transfer requires approval for its specific source bundle.

Run `python3 docs/benchmarks/20261026-combined-authenticated-reuse-v13/validate-evidence.py` to validate the [archived results](evidence.tar.gz), indexed hashes, paired runs and correctness receipt. The archive excludes large binaries; their recorded hashes and execution logs remain. Retained failed pilot runs are diagnostic and are not qualification evidence.
