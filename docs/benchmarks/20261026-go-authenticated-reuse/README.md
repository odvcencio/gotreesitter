# Go authenticated reuse: V1 rejected

The V1 frozen overlay bypassed verification for the tested native ASCII identifier edits, but **did not qualify**: the primary numeric single-byte edit benchmark regressed from 86.78 µs to 105.67 µs (+21.77%, benchstat p=0.000, 20 paired seeds). The overlay was not promoted, no language was graduated, and merges remain held. A later candidate needs its own source-bound correctness and performance evidence.

The baseline is `04367dd2d3366dc4f6691cebd283f42be595a402`; the tested candidate is the 14-file runtime overlay and accompanying controls in [evidence.tar.gz](evidence.tar.gz). The archive includes both arms, source and control hashes, commands, logs, raw measurements, and container state. [summary.json](summary.json) and [verification.json](verification.json) give machine-readable results. Validate the archive and its inherited inputs with `python3 docs/benchmarks/20261026-go-authenticated-reuse/validate-evidence.py`; add `--source-root .` to check the current checkout against source guards.

The actual bypass was exercised in 144 release and 144 race session steps, with independent fresh structure, state, and precedence comparisons. The work ledger compared 24 operations: 20 were unchanged and four clean identifier insert/replace operations reduced attempts from two to one. Selected reuse stayed equal; arena profile fields differed. The tested no-edit paths allocated zero. These are results for this source snapshot and scenarios, not proof for every grammar or edit.

| Linux amd64 measurement | Baseline | V1 | Result |
| --- | ---: | ---: | --- |
| Original report fixture, plain edit | 94.375 ms | 9.028 ms | -90.43% |
| Original report fixture, profiled edit | 89.008 ms | 9.918 ms | -88.86% |
| Primary numeric edit | 86.78 µs | 105.67 µs | +21.77%, gate failed |
| Primary full parse | 5.389 ms | 5.527 ms | Inconclusive |
| Primary no-edit | 3.925 ns | 3.882 ns | Inconclusive; zero allocations |
| RSS median, 140288-byte workload | 108514 KiB | 59156 KiB | -45.49% |
| RSS median, 1 MiB workload | 214252 KiB | 214316 KiB | +0.03% |

Timing uses the required randomized benchmark script with 20 paired seeds and 750 ms per benchmark; the archive retains the benchstat report and samples. RSS uses 20 pairs per size. The 140368-byte original report fixture has SHA-256 `5434575bb249338937331c3756e10dcec34237d107f523ea315996fb6de0ef8f`. Runs used a 4 GiB, one-CPU, network-disabled Docker container on an Intel Core Ultra 9 285. These Linux measurements do not establish Windows latency for the [reported issue](https://github.com/odvcencio/gotreesitter/issues/454#issuecomment-6311551850).

## V2 diagnostic pilot

A field-layout variant was rejected after four exploratory pairs (seeds 21–24). Median numeric-edit time rose from 71.567 µs to 93.831 µs (+31.11%); median report-fixture edit time fell from 75.782 ms to 7.752 ms (-89.77%). These four pairs are diagnostic, not a qualification run. The archive also retains the variant's source and overlay maps, container state, and the V1 baseline/candidate CPU profiles and attribution. No winner has been promoted; the merge hold remains in force.

A subsequent negative authority test also rejected V1: a copied language, one with independently modified symbol names, and a forged receipt each acquired producer key 8195. The archive retains the test sources, overlay, and failing output. This demonstrates an authority-scope violation; it did not demonstrate an incorrect tree. The proposed fix is owner-bound decoded-artifact permission under the existing immutable-language contract, and must be tested together with any performance winner. No claim is made that it detects arbitrary in-place table mutation.

## Combined V11: scoped comparative checks passed

The combined candidate binds the owner-bound authority fix and performance change to the same `v11/inputs.sha256` manifest in the archive. Focused correctness, 412 fleet route runs across 206 languages, 144 release and 144 race bypass steps, both pinned C comparisons (127 matching cases), and focused race tests passed. All 206 languages' two route ledger exports match their baseline. Complete-operation work and selected reuse match V1, whose four identifier operations reduce work compared with baseline. The authority contract remains the existing immutable-language contract.

The full 20-seed comparison measured numeric edit 76.44 → 59.51 µs (-22.15%, p=0.000), plain report-fixture edit 83.682 → 8.290 ms (-90.09%, p=0.000), and profiled edit 79.960 → 8.637 ms (-89.20%, p=0.000). Primary full and no-edit timing were inconclusive; no-edit remained zero allocations. RSS medians were 108460 → 59660 KiB at 140288 bytes and 214284 → 214036 KiB at 1 MiB.

Profiled reuse selection rose from 446.9 to 690.6 µs (+54.54%, p=0.000), while reparse fell from 73.324 to 6.020 ms (-91.79%, p=0.000). **A measured allocation regression remains:** report-fixture full parse rose from 1.402 to 4.956 KiB/op (+253.38%, p=0.017) and 6 to 6.5 allocs/op (+8.33%, p=0.002). A separate 10-second-per-arm allocation profile measured about 1182 B/op and six allocations in both arms; it does not negate the 20-seed result. Raw samples, benchstat, profile and container settings are archived. V11 passed the scoped comparative checks; runtime promotion and main merges remain held. Porting to the existing incremental draft requires fresh qualification on its combined branch. The earlier V3 and V5 pilots are rejected diagnostic variants; V9 is exploratory. An authority test setup failure is archived with reconstructed inputs explicitly labeled because its original overlay was overwritten.

The applicability of any prior fixture-specific B/op ratchet is unverified; this receipt does not claim that all v1 ratchets or language graduation gates pass.
