# Across-token ownership experiment: archived

**Decision: archive the prototype.** None of the four prespecified YAML parse/traversal cells reaches the 5% improvement target. The runtime and API snapshots return to `9a0b6d7cc07a973df122699d1197a066a14688c2`. This bucket publishes evidence only. PR #1512 stays draft; main merges and compact graduation remain held.

The prototype kept one fresh scheduler operation open across token elections, bounded by 256 shifts/reductions. It advanced the real scanner and frontier, retained ordinary recovery/ambiguity handoffs, and left each refused token unconsumed. The loop still performs each real election, checkpoint capture, reduction, and publication. Reducing scheduler re-entry did not remove enough elapsed time to meet the target. A separate direct-shift publication idea was excluded without timing it.

## Fixed hypothesis and source

The baseline is frozen `9a0b6d7c`. The candidate is the three Go source/test overlays in `source-manifest.json`, with API snapshots regenerated for the new internal adapter method. The promotion hypothesis was registered before the first timing run: **at least 5% lower median time, with p<.05, in all four ordinary YAML corpus/8 KiB parse and full-traversal cells**, with all existing gates preserved. Go, query, edits, and no-edit remain controls. `hypothesis-order.json` records registration time and the subsequent run starts.

Both sides use **Go 1.26.6, linux/amd64**, the pinned Docker image in `command.json`, one pinned CPU, `GOMAXPROCS=1`, 4 GiB container memory, and `GOMEMLIMIT=3GiB`. `scripts/run_randomized_benchmarks.sh` runs 20 paired, alternating seeds at 750 ms, count 1, with matched shuffled child order and `-benchmem`. Public suites use seeds 266001–266020; the primary trio uses 267001–267020. The four-pair screening runs are retained separately. All completed samples remain in the receipt.

Public workloads explicitly request the candidate route. YAML uses its ordinary eager publisher; the experimental YAML node-view publisher is not enabled. Parse-plus-edit includes the existing fallback and fresh-verifier behavior. Default and candidate routes both receive invariant, counter and C checks. The primary Go trio explicitly enables the candidate route. This experiment does not change any route default.

## Public-operation results

Median time change relative to the frozen baseline; negative values are faster. These are 20 pairs for each of 19 final cells, or 760 benchmark rows across both sides.

| Fixture | Parse | Full traversal | Query | Parse plus edit |
| --- | ---: | ---: | ---: | ---: |
| YAML corpus (1,998 B) | -1.48% | -1.07% | -0.10% | -0.61% |
| YAML EOF witness (8,012 B) | -0.84% | -0.14% | -2.20% | +0.66% |
| Go query compiler | -2.40% | -1.39% | -1.32% | +0.44% |
| Go grammar generator | -0.20% | -1.23% | -3.44% | +0.64% |

Only the 8 KiB YAML parse (p=.049), 8 KiB YAML query (p=.028), and large Go query (p=.011) reach p<.05. The other public timing comparisons are inconclusive. Ancillary query gains do not satisfy the primary hypothesis. See [Go benchstat](benchstat-go.txt) and [YAML benchstat](benchstat-yaml.txt).

The primary Go trio measures full parse **−0.51%**, single-byte edit **+0.80%**, and no-edit **−1.18%**; all are inconclusive. The candidate full-parse interval is wide (±28%), and all samples are retained. No-edit remains **0 B/op, 0 allocs/op**. See [primary benchstat](benchstat-primary.txt).

Allocation-count medians are unchanged in all 19 final cells. Byte medians are effectively unchanged: the largest increase is 3.5 B/op on the 18.74 MB small-Go parse-plus-edit case (+0.00002%); primary edit falls 424→422 B/op. All final cells pass the existing 10% relative timing/allocation ratchets. No threshold is changed.

## Counters and correctness

All eight Go/YAML full/edit work rows and both per-language ledgers match the baseline, including route decisions, reuse, completion, coverage, and error metadata. Node counters are:

| Language / route | Full, before → after | First edit, before → after |
| --- | ---: | ---: |
| Go candidate | 892 → 892 | 1,553 → 1,553 |
| Go default | 1,922 → 1,922 | 3,810 → 3,810 |
| YAML candidate | 232 → 232 | 232 → 232 |
| YAML default | 288 → 288 | 480 → 480 |

The full core suite passes, including 36 operation-bound/refusal combinations, unbound external-token handoff, and invalid-frontier/error/panic poisoning. Focused corridor, public-tree/query/edit/copy, race, emergency-build, and layout checks pass. Four invariant sessions cover **288 edit steps**, with zero-allocation no-edit reparses; **12 standard locked-C fixture checks** pass. Go and YAML C/invariant runs use separate processes. All **20 memory-contract tests** pass. Layout remains 756 root Go files and 12 API snapshots.

This is focused experiment evidence. The prototype was not promoted or run through the 206-language fleet. Cognociente’s known malformed-delete C mismatch remains open and was not rerun here; the small-source fresh verifier stays enabled. No native Windows/ARM64 or new C-speed claim is made.

## Bounded memory observations

Two pairs for each language/operation give eight matched 1 MiB probes. These are diagnostic observations, not a memory-equivalence or graduation qualification. Tree digests match; RSS remains inside the existing relative and 400-bytes/source-byte limits. Every release returns within 1 MiB of its warm baseline. Total live heap is reported alongside peak RSS because the retained delta alone excludes the warm pool.

| Case | Peak RSS MiB, before → after | Total live heap MiB, before → after |
| --- | ---: | ---: |
| go-root | 244.16 → 237.16 | 141.67 → 141.67 |
| go-walk | 242.47 → 243.15 | 213.68 → 213.68 |
| yaml-root | 72.25 → 69.46 | 45.00 → 45.06 |
| yaml-walk | 69.49 → 69.32 | 45.10 → 45.10 |

## Advisory CI loss: preserved, not explained

The green-status advisory [performance job](https://github.com/odvcencio/gotreesitter/actions/runs/40146769046/job/121029780878) reports **DEFINITIVE_LOSS**: default-route Go single-byte edit rises **115,228→127,457 ns/op (+10.61%)**, exceeding its unchanged +8% gate. It compares `c5695bc4c3a155cc1af490e18c6a0684c8317945` against synthetic merge `d41e71d53771e59a77a4716b9aecbbb87b84bafe`, using Go 1.22.0. That merge contains main `6a4dea10` plus branch `9a0b6d7c`; its only source differences from the branch head are the YAML indentation-frame contribution. The advisory benchmark uses the default legacy route, with an empty admission allowlist and no candidate override.

A separate local comparison freezes those exact two revisions through source overlays and uses **Go 1.22.0, linux/amd64**, with the same default route. Twenty matched pairs (seeds 265001–265020) measure full parse **+0.25%** (p=.841), edit **182,178→182,431 ns/op, +0.14%** (p=.883), and no-edit **−1.02%** (p=.110). It passes the unchanged +8% time/+5% byte/+5% allocation gate. No-edit remains allocation-free. See [matched-compiler benchstat](benchstat-ci-go122.txt).

The original CI loss is **not reproduced locally and is not treated as fixed**. The host and seed set differ from CI’s seeds 1–20. No cause is assigned. The first local attempt stopped before timing because Go 1.22 vet read host files instead of the frozen overlays; its failed log is retained. The diagnostic retry disables vet only for those benchmark invocations. Compilation and benchmark tree/completion checks remain enabled; the experiment’s separate static and correctness gates are unchanged.

At the recorded current-source CI snapshot, completed correctness jobs—including the broad SQL suite—pass, while `editor_latency` is still running. The earlier SQL memory discrepancy remains unexplained despite the newer pass. This archive changes no runtime source and makes no claim that either historical observation has been fixed.

The next recommendation is a same-runner, seeds-1–20 Go 1.22 comparison with `Tree.Edit`, reuse selection, and reparse/rebuild attribution. That is the smallest causal follow-up to the remaining CI observation; no speculative optimization is included here.

## Receipt and reproduction

[receipts.tar.gz](receipts.tar.gz) contains the frozen source overlays and hashes, commands, controls, all successful and failed run logs, raw timing samples, counter comparisons, memory observations, analysis scripts, and the decision. Executables are excluded; their hashes and sizes are recorded. The original CI log and the local overlay/vet limitation remain included.

Reconstruct against the recorded Git revisions with the frozen overlays and Docker commands. `prepare.py` records the original capture and is not a rerun step on a clean checkout. The command manifests retain the original mount/cache paths; the source manifests authenticate the overlays even though benchmark headers show the canonical checkout’s Git revision. The CI diagnostic has its own revision manifest and must not be interpreted as a reset of the stream experiment’s baseline.

Archive: 866804 bytes; SHA-256 `0fd4fef22a22d9e585566f015ba847c2564e47611f594a2a56d16842b891b73f`. All 278 members were verified after writing.
