# Go edit proof integration

Twenty paired local runs of the unchanged Go CI benchmarks show **single-byte edit time 137.18 → 92.92 µs (−32.27%, p<.001)**. Allocations remain five per edit. This integrates two existing proof optimizations from [PR #1509](https://github.com/odvcencio/gotreesitter/pull/1509) into the compact-output branch; it does not claim a new independent invention or import that PR's receipt-publication, read-history, or allocator changes. PR #1512 remains draft and all main merges remain held.

## Source and review

The baseline is `450e66015075aebc601574993d02240f3792953f`, whose runtime matches `9a0b6d7c`. All five integrated source/test files exactly match `8450a068f2a66b8c66793d416fc6e748b3f54c49` on #1509. The proof result originated in `45eb11137`; mode membership originated in `dba13461f0df4dc8eaf626a80d143a7cf0e8e5ab`. The archive authenticates their complete contents and the baseline overlays.

- Return the six cursor coordinates used by the proof in `incr.DFAProbeState`: 32 bytes instead of a 240-byte `Lexer` on this amd64 host. Each probe still scans a full configured private lexer. Successful and failed cursor coordinates, tokens, read frontiers, byte/scan charges, projection failures, and rollback behavior remain checked.
- Use a 512-byte stack bitmap to recognize already-seen state IDs below 4,096. Wider IDs retain ordered-slice lookup. The ordered mode list, fallback state zero, after-whitespace modes, validity checks, 32,768-row limit, 1,024-mode limit, scanner checks, and proof budgets remain unchanged.

No language-name comparison, route change, verifier removal, pool adjustment, ledger exception, or memory-limit change is introduced. Review confirmed exact correspondence with the previous six comparisons and ordered membership semantics. Contract tests exercise accepted/failed scans, skipped whitespace, UTF-8, invalid bytes, EOF, budget charges, invalid modes, bounds, wide IDs, and allocation-free membership. A separate private cursor implementation was stopped without timing after locating #1509; its status and patch are retained as superseded evidence.

## Original CI regression

The [original CI job](https://github.com/odvcencio/gotreesitter/actions/runs/40146769046/job/121029780878) tested `c5695bc4` against synthetic merge `d41e71d5`, using Go 1.22.0, default legacy routing, and seeds 1–20. On **AMD EPYC 9V45**, edit time rose **115.23 → 127.46 µs (+10.61%, p<.001)**, exceeding the unchanged +8% gate. Its green advisory status did not mean performance passed.

The [later run](https://github.com/odvcencio/gotreesitter/actions/runs/40152069466/job/121045752142) used the same runtime, compiler, route, and seeds on **Intel Xeon Platinum 8573C**. It measured **174.86 → 178.08 µs (+1.84%, p<.001)**. This is below the gate but remains a directional regression. The two CPU models prevent treating the later result as an exact hardware reproduction or dismissing the original loss as noise. Both artifacts and analyses are preserved.

Profiling the original CI binaries locally attributes about **97% of edit CPU time** to the primitive token-equivalence proof. This benchmark shares the complete root after proving token equivalence; rebuilding allocates no nodes. The source of the original cross-revision slowdown remains unassigned. The integration targets the dominant cost directly.

The CI workflow now records separate 20-seed edit attribution and sampled profiles after the unchanged uninstrumented gate. Its runner label and Go version are unchanged. GitHub-hosted VMs are ephemeral: the runner class can be repeated, but the original VM or CPU model cannot be selected through that label. Hosted qualification of this new integration follows its Buckley push and must be read from that run, not inferred from these local numbers.

## Paired local performance

Go **1.22.0 linux/amd64**, Intel Core Ultra 9 285, pinned CPU 6, `GOMAXPROCS=1`, one process per seed, count 1, 750 ms, benchmem. The shared randomized runner alternates baseline/head across **seeds 1–20** and verifies identical child order. The original benchmark and prepared-suite hashes exactly match CI. Both sides use the ordinary default legacy route. The baseline is frozen through source overlays in the canonical checkout. Go 1.22's virtual-source vet limitation requires `-vet=off` for these local benchmark builds; separate correctness gates use Go 1.26.6, and native CI retains vet.

| Workload | Before | After | Result |
| --- | ---: | ---: | --- |
| Full parse | 5.341 ms | 5.055 ms | −5.35%, inconclusive, p=.565 |
| Single-byte edit | 137.18 µs | 92.92 µs | **−32.27%, p<.001** |
| No edit | 3.615 ns | 3.771 ns | +4.33%, inconclusive, p=.265 |

Allocations remain **6 / 5 / 0** respectively. Bytes are 1,164→1,162.5 for full parse, 395→394 for edit, and zero for no-edit. All unchanged timing/allocation gates pass. See [benchstat](benchstat-primary.txt) and [complete measurements](performance.json). The full/no-edit directions are recorded without claiming their timing differences as improvements or regressions established by this local experiment.

Separate instrumented runs retain all 20 pairs. These timers explain costs and are not interchangeable with ordinary benchmark timings:

| Phase, median per edit | Before | After |
| --- | ---: | ---: |
| `Tree.Edit` | 0.462 µs | 0.411 µs |
| Reuse selection and proof | 107.313 µs | 70.840 µs |
| Reparse/rebuild | 0 | 0 |

Every sample reuses one subtree and 19,294 bytes and allocates zero new nodes. Source preparation, release, profiling overhead, and work outside the parser timers are not assigned to rebuilding. The ordinary-entry CPU profiles corroborate the proof hotspot; its copy fraction falls, while actual scans and proof-loop work remain the next costs.

The CI-shaped large-file RSS probes complete at **64,004 → 64,140 KiB (+0.21%)**. These are single diagnostic samples, not a new memory-equivalence or graduation qualification. All 20 memory-contract tests pass.

## Correctness and deterministic counters

Before timing, isolated Go 1.26.6 runs pass **412 invariant sessions / 29,664 edit steps**, **1,236 standard locked-C checks without skips**, **824 identical work rows**, and **206 identical selected-ledger comparisons**. Each language/route runs separately. The existing CSV/Enforce grammar-smoke fixtures remain; other languages use R4. Internal and focused root race checks, explicit mode-bound tests, and layout checks pass: **756 root Go files, 12 API tag sets**. See [gate summary](correctness.json).

| Node work | Full before → after | First edit before → after |
| --- | ---: | ---: |
| Go candidate | 892 → 892 | 1,553 → 1,553 |
| Go default | 1,922 → 1,922 | 3,810 → 3,810 |
| YAML candidate | 232 → 232 | 232 → 232 |
| YAML default | 288 → 288 | 480 → 480 |

All token, version, reuse, splice, route, completion, coverage, and error metadata are unchanged. No-edit reparses allocate nothing. This numeric-edit improvement does not resolve Cognociente's identifier-edit fresh-verification cost or known malformed-delete C mismatch. It does not graduate compact, qualify Windows/ARM64 on this branch, or clear #1509's separate RSS hold.

## Receipt

[receipts.tar.gz](receipts.tar.gz) contains frozen source, commands, raw paired samples, profiles, original CI samples, counter comparisons, correctness logs, and analyses. Reconstruct from the pinned baseline and integrated files; adapt the recorded mount paths to the same canonical checkout and shared caches. Timing headers identify the physical checkout, while source manifests authenticate the virtual baseline. Correctness uses the harness-compatible Go 1.26.6 image; timing uses pinned Go 1.22.0.

The archive is **500,540 bytes**, SHA-256 `2947c59482a1b126695133182e2e809584c38b52c8fb59b17faaa78a4f107228`; all **2,589 members** were verified after writing. No executable, credential, or unrelated source is included.
