# Primitive-edit DFA snapshot receipts, 2026-10-21

Status: runtime qualification is held. The speed result is measured; direct parser-only probes confirm a 1 MiB warm-process RSS ceiling failure in both baseline and candidate. Do not merge or graduate a language from this receipt.

The primitive-edit proof still clones and scans the full configured lexer. It now returns the six cursor/failure coordinates that callers compare, instead of returning and projecting the entire lexer. The returned state is 32 rather than 240 bytes on tested 64-bit targets, and 24 rather than 136 bytes on actual Linux 386. Scans, mode enumeration, byte/scan budgets, rollback, verification policy and reuse selection are unchanged.

## Measured throughput

Each row uses 20 matched alternating pairs, one process per explicit seed, `GOMAXPROCS=1`, count 1, 750 ms and benchmem. Baseline is D64 runtime `d30639dd95af74197305ca76de645ea8d9c29c07`; the review base is `7c940633eb977fb8122caff422166aa1257f6257`, which changes only receipts after D64.

| Host / primary Go workload | Before | After | Result |
| --- | ---: | ---: | --- |
| Linux amd64 / single-byte edit | 154.2 us | 143.6 us | -6.85%, p=.020 |
| Linux amd64 / full | 6.779 ms | 7.000 ms | inconclusive, p=.968 |
| Linux amd64 / no edit | 4.550 ns | 4.649 ns | inconclusive, p=.529 |
| M1 Max Docker Linux arm64 / single-byte edit | 209.6 us | 133.3 us | -36.40%, p<.001 |
| M1 Max Docker Linux arm64 / full | 5.950 ms | 5.920 ms | inconclusive, p=.602 |
| M1 Max Docker Linux arm64 / no edit | 6.207 ns | 6.211 ns | inconclusive, p=.292 |

Allocations stay at 5/edit, 6/full and zero/no-edit. The ARM64 edit B/op median is 397 to 395; this small amortized difference is not a separate allocation claim. Raw before/after files, benchstat tables and collection audits are adjacent.

These are the standard Go DFA benchmarks through the shared seeded wrapper. They are not the 137 KiB standalone timings in [Cogno's latest October 21 report](https://github.com/odvcencio/gotreesitter/issues/454#issuecomment-6311551850). The larger trusted-small-source candidate and its counter exceptions remain held in the [separate receipt](../20261021-trusted-small-source-results/README.md). Neither run validates native Windows timing or compares v0.55.1 directly with v0.56.0.

## Correctness and deterministic counters

All 412 selected ledger rows are identical to baseline: 320 available full rows and 92 unchanged declines with unavailable full counters. Unavailable counters are not zeroes. `counter-equality.json` preserves the Go/YAML before/after vectors; `candidate-ledger.json.gz` holds all rows. No ledger reset or owner exception is requested.

Linux qualification passes 1,236 standard locked-C fixture checks across 206 grammars and both routes; eight exact Cogno clean insert/replace C cases; twelve numeric C edit cases; two Djot/TOML context cases; 412 canonical invariant sessions of 72 edits (29,664 edit steps); twenty memory contracts; 47 focused unit tests; 21 focused race tests; and eleven actual 386 primitive tests. All invariant sessions report zero no-edit allocations. C sources, overlays and raw logs are retained.

An independent M1 Max Docker Linux arm64 run passes another 412 sessions / 29,664 edit steps, 47 focused units, twenty memory contracts and 21 focused race tests. `arm64-receipts.tar.gz` contains all session logs and its hash manifest. The worker and an independent remote check verified input files after execution; the user's Mac checkout stayed unchanged. No native macOS or Windows claim is made.

The existing 1 MiB Go four-edit test also passes incremental/fresh/locked-C structure comparisons. Its raw log is `large-Go-invariant-C.log.txt`.

## Memory hold and excluded YAML fixture

Twenty additional matched pairs on the generated 1,048,591-byte Go fixture show no full-parse timing difference (644.5 to 651.2 ms, p=.820). Full tokens (425,742), nodes (993,391), max stacks (2), arena/source ratio (160.7) and route counters agree before timing and throughout the run.

Process RSS medians are 458,928,128 baseline and 458,780,672 candidate bytes (ratio .99968, within the 10% ratchet). The hard 400-RSS-bytes/source-byte ceiling is nevertheless exceeded in 15 baseline and 18 candidate samples. Over-limit samples have benchmark N=2; N=1 samples remain under the ceiling. GNU time wraps `go test`, so this includes toolchain and benchmark calibration. Subsequent direct parser-only probes exclude the compiler and confirm the failure, as recorded below. Do not claim that the hard RSS gate passed, lower the workload to hide it, or change GC/pool/budget settings to obtain a pass. The raw RSS process files and complete audit are retained.

Direct immutable benchmark binaries at fixed N=2 report plain-process RSS of 458,366,976 baseline and 458,457,088 candidate bytes, with no compiler in the process. Default-sampled post-GC heap profiles retain 53.73 and 55.63 MiB; transient-parent allocation is the largest retained category at 30.04 MiB each. These are attribution probes, not randomized timing comparisons or measurements of the instantaneous RSS peak by allocation category. `parser-only-RSS-audit.json` and its archive retain the exact commands, input binary hashes, logs and profiles. Runtime promotion stays held. The next investigation concerns transient-parent construction/materialization and scratch lifetime.

Initial one-iteration process samples were under the hard ceiling but varied; they do not override the 20-pair result. An earlier paired setup attempt lacked the language environment and emitted zero valid benchmark rows. Its output remains in the local evidence directory and supplies no timing evidence.

The generated 1 MiB YAML fixture fails the benchmark's clean-input precondition in baseline. Locked C also reports an error: source SHA-256 `516b6fd69924a93d8122c653acf9ce6bb345eda0116c3d8ab833c56c97499e76`, YAML grammar `a1c4812a73ec5e089de8e441fdea3a921e8d5079`, artifact SHA-256 `4313f545180b69d962454979111200273d2ac81516050f0329cf9ba305fb4373`. Both baseline and candidate Go match C's error status and full end coverage on both routes. This is not a passing clean YAML memory gate or a full structural comparison of that invalid/error fixture. The two C logs and exact witness source are retained. An optional later CPU-profile step omitted its required seed and failed; those profiles are not used as evidence.

Ordinary YAML routing, compact allowlists, fresh verification and the known Go delete lexical-context mismatch remain unchanged.

## Reproduction and input identity

`reproduction-audit.json` verifies every byte and SHA-256 of 4,331 baseline and 4,332 candidate curated inputs using private Git indices. Start from review base `7c940633eb977fb8122caff422166aa1257f6257`; apply this PR's three Go source changes for the candidate, and the [existing baseline benchmark harness](../20261021-trusted-small-source-results/baseline-benchmark-harness.patch.gz) to both inputs. The two compressed ARM64 source manifests pin the full curated closures; `source-go-sha256.json.gz` pins all 3,542 candidate Go files.

The shared wrapper and seed shim are identical on both sides. The [repository randomized runner](../../../scripts/run_randomized_benchmarks.sh) is the comparison runner. Primary seeds are 134001..134020 on amd64 and 135001..135020 on arm64. Large-file seeds are 136001..136020. The large harness adds the same explicit per-parser route and input/route metrics on both sides; it changes no runtime source.

Linux uses pinned image `sha256:187907888b4bdc6f326c87b045718339e5f665baceb27db6b74633ca41e996df`, CPU 6, 6 GiB memory and a 5 GiB Go soft limit. ARM64 uses `golang@sha256:3b4a11519ad929d1e1d261a12cff056f0c85b735253d7d861346b9c6f8b36437`, CPUs 2,3, 4 GiB memory and a 3 GiB Go soft limit. Both report Go 1.25.14. Sources mount read-only. C parity and correctness runs remain separate from performance. No process crashed or was OOM-killed.

`manifest.json` pins every receipt file except itself. Private commit logs, executables and scratch source trees are excluded.
