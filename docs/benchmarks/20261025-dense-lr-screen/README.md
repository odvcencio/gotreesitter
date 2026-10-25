# Held dense LR V2 screen

**HELD.** This private Go experiment reduces persistent graph construction on a single-version parser path. It improves generated full-parse time, but increases allocations and does not establish an R4 throughput improvement. It changes no production source, default route, graduation allowlist or policy. V3 is a separate experiment and is excluded here.

The baseline is source `9580313574e273cd0e920cd120c6afe5dc357c8e`; the candidate uses the same source plus the frozen `dense-lr-v2-root` sparse overlays. All 3,813 saved Go, grammar-bin and module source hashes match at packaging. Later commits change documentation and ledger metadata, with no Go/bin/module change. The Go blob is `9ad1f8d7d0f2b9aa32c61bd8295b56177caabb0ae2fb8dd4617559ee0f71fed4`. The generated fixture is `makeGoBenchmarkSource(500)`, 19,294 bytes, SHA-256 `2e0f9d4676549c8560cebc3c53877932f921ab9d77c88518227f3018573af4b7`; R4 is the existing 2,047-byte Go sample, SHA-256 `b37389f6dbac4e417f19db04909783b0264dfec4e3c93774f0557c3e91b8f6c5`.

The unchanged randomized runner executes seeds 1–20, 750 ms per benchmark, one process per seed, count 1, benchmem, matched explicit child order, alternating baseline/candidate order. There are 20 primary pairs and 20 R4 pairs: 80 benchmark processes, all complete. Both sides use `gts_parsercorephase0` and the identical seeded benchmark wrapper, with no observer installed during timing. Results are Linux/amd64 with Go 1.26.6, Intel Core Ultra 9 285, one pinned CPU, GOMAXPROCS=1, a 4 GiB container and GOMEMLIMIT=3GiB. This is a comparison under those build flags, not ordinary uninstrumented production qualification.

| Measurement | Baseline | Dense V2 | Result |
| --- | ---: | ---: | --- |
| Generated full parse | 7.526 ms | 6.684 ms | −11.19%, p<0.001 |
| Generated full allocations | 22/op | 28/op | +27.27%; B/op +1.24% |
| Generated edit | 67.95 µs | 67.78 µs | Inconclusive, p=0.738; 5 allocations/op unchanged |
| Generated no edit | 3.182 ns | 3.191 ns | Inconclusive, p=0.713; zero allocations/bytes |
| R4 compact full parse | 1.014 ms | 1.042 ms | Inconclusive, p=0.165 |
| R4 compact allocations | 14/op | 21/op | +50.00%; B/op +296.40% |
| R4 median paired compact/legacy time ratio | 1.19452 | 1.22285 | Current-legacy diagnostic; not E-A qualification |

Physical graph-link additions fall from 15,505 to 2 on generated input and 1,381 to 1,333 on R4. All eight matched entry/route pairs retain the same public deep digest; candidate observations retain identical scheduler work, other Core work, payload/scanner hashes and dependency hashes. The two entry labels exercise ordinary Parse and the registered-entry harness; Go has no registered token-source factory here, so the latter also calls Parse. The private observer's source fields are unusable: eight observations report zero bytes and the empty-input hash because the source binding has already cleared. Input authentication comes from the external `ENTRY_WORK` rows and frozen fixture hashes, not those fields.

Focused correctness passes: five core top-level tests (nine records including subtests), ten Go C top-level tests (47 records), and both routes' unchanged 72-step R4 invariant sessions. Exact C assertions retain symbols, fields, spans, points, flags and deep digests, including malformed semicolons and grouped bindings. The ordinary candidate routes the two benchmark witnesses once without fallback and preserves zero-allocation no-edit parsing. These tests use the repository's existing locked C oracle; this experiment does not adopt C 0.27.1.

The first memory selector used the wrong tags and ran no tests; its empty selection is preserved. The next 29-test memory/lifetime run has five top-level failures at exact `GraphLinkAdditionsProxy` golden pins. A separate four-fixture probe proves unchanged canonical digests and every other Core work field, with lower link counts `2995→2977`, `14662→14592`, `15073→15041`, and `148367→148321`. A private test overlay changes only those exact physical pins, including their repeated query/warm references. Every original assertion and memory threshold stays intact. With that overlay, all 29 memory/lifetime top-level tests (47 records) pass. The original failures and old pins remain in the archive; this is not a public pin update or a large-file RSS qualification.

The original R4 edit schedule is unchanged: source/session hashes and all 72 edits are pinned. On both baseline and dense builds, all 146 route rows / 73 pairs pass fresh equality and zero-allocation no-edit checks, and deterministic profiles are identical. Both tests intentionally fail their final clean-reuse coverage assertion because there are **zero clean edited states**. This does not prove positive reuse on clean edits. A separate additive control applies the benchmark's one-byte `0↔1` replacement at byte 35 repeatedly: all 72 edited pairs are clean, have positive candidate reused subtrees and bytes at least as high as legacy, satisfy fresh equality, and retain zero-allocation no-edit parsing. That control covers one replacement site only; it does not cover insertions, deletions, 16 sites, C parity at every edit, or the complete graduation gate.

All failed attempts remain visible: the first reuse build stops at vet's redundant-condition diagnostic before execution; the original R4 reuse runs retain their intentional coverage failures; the first memory run retains five physical-pin failures; the first perf job stops at the failed qualification-status check without timing. The later 40 timing pairs complete. CPU profiles and top/cumulative reports are attribution only; their fixed-order benchmark runs are not timing comparison evidence.

The [verification](verification.json), [source identities](source-identities.json), [archive](evidence.tar.gz) and [checksum](evidence.sha256) retain exact V2 source/test overlays, original relevant tests, unchanged randomized runner, commands, workers, per-file authentication, all eight raw run groups, benchmark samples/benchstat, process statuses and CPU profiles. Executable test binaries, caches, credentials and private CSV/Enforce payloads are excluded. The source checkout, pinned public dependency modules and authenticated C-ref cache remain replay prerequisites. This screen makes no fleet, YAML, M1, C-speed, cross-platform, Max RSS, production promotion, E-A/E-G or graduation claim.

To inspect or reproduce, extract into a new directory and verify the payload. The replay verifies the 3,813 source hashes and exact runner, remaps the recorded Docker bindings, prints the command, and executes only with `--execute`, under the shared measurement lock. Use a new output directory for every group and keep the original failed evidence. The perf replay's prerequisites point at the frozen successful C/D8 and memory receipts included under `controls/out`; they remain evidence of this source snapshot, not a new qualification run.

```sh
sha256sum -c evidence.sha256
mkdir /path/to/new-dense-v2-receipt
tar -xzf evidence.tar.gz -C /path/to/new-dense-v2-receipt
cd /path/to/new-dense-v2-receipt
sha256sum -c payload.sha256
python3 reproduce.py --group dense-perf-v2 --repo /path/to/gotreesitter --module-cache /path/to/go-mod --c-cache /path/to/c-cache --runtime /home/draco/work/.runtime --output /path/to/new-results --cpu 6
```

The replay accepts each of the eight original run names as `--group`; the archive contains their exact scripts/selectors. `dense-memory-reuse-v1`, `dense-memory-reuse-v2` and `dense-perf-v1` are expected to reproduce failures. It does not disable vet or weaken a threshold. The source commit plus sparse overlay hashes identify this experiment; the receipt does not authorize a merge.

## Rejected V3 reentry supplement

**REJECTED.** V3 adds authenticated reentry at single-path graph anchors and bounded detached scratch retention. It reduces more graph work but regresses the measured R4 workload. No V3 runtime source, default route or graduation change is promoted. The original V2 archive, checksum, verification and source identities remain unchanged.

V3 uses the same authenticated source snapshot and fixture bytes as V2, with its own frozen sparse overlay. All eight entry/route pairs retain equal public deep digests and equal scheduler work, other Core work, payload/scanner hashes and dependency hashes. Graph-link additions fall from `15505→2` on generated input and `1381→752` on R4. The R4 ordinary candidate records 45 entries, including 44 reentries. Corrected observer source fields match the external 19,294-byte / 2,047-byte input identities in all eight observations. Ownership is detached after parsing, while bounded idle scratch remains retained: 3,608 bytes generated and 4,408 bytes R4; a released owner does not imply zero retained storage.

Focused gates pass with no skips or failures: nine core top-level tests (13 records including subtests), two lifetime tests, 15 directly selected memory tests (18 records), ten Go C top-level tests (47 records), and both unchanged 72-step Go invariant sessions with zero no-edit allocations. The warm-allocation unit covers generated input only and reports `22=22`; it does not qualify R4 allocations. These selections do not rerun the full canonical memory-pin suite or establish fleet, large-file RSS, YAML, platform or complete reuse/graduation qualification. V2's separate one-site clean replacement receipt is not new V3 coverage.

The randomized protocol again completes 20 primary and 20 R4 pairs, 80 processes, with matched child order and the same diagnostic build flags on both sides. Each V3 result below compares against its own paired baseline; the separate V2 and V3 timing sessions are not a direct comparison.

| Measurement | V3 paired baseline | V3 candidate | Result |
| --- | ---: | ---: | --- |
| Generated full parse | 9.293 ms | 9.025 ms | Inconclusive, p=0.659; 22 allocations/op both |
| Generated edit | 89.52 µs | 87.01 µs | Inconclusive, p=0.157; 5 allocations/op both |
| Generated no edit | 4.058 ns | 3.854 ns | −5.05%, p=0.021; zero allocations/bytes |
| R4 compact full parse | 1.020 ms | 1.126 ms | **+10.40%, p<0.001** |
| R4 compact allocations | 14/op | 37/op | **+164.29%; B/op +132.37%** |
| R4 median of within-seed compact/legacy ratios | 1.1821749043 | 1.2932265545 | Current-legacy diagnostic; not E-A qualification |
| R4 ratio of median compact time to median legacy time | 1.1720775490 | 1.3036322735 | A distinct statistic from the paired median above |

The nanosecond no-edit difference makes no claim about a faster core execution mechanism: no-edit parsing bypasses that work. R4 compact routing remains one route per operation with zero fallbacks.

The terminal allocation profile proves a specific cause of the R4 allocation increase. `tryDenseLRPrivate`, line 357 of the frozen overlay, concatenates `"core refused action: " + LastRefusal()` before calling the promotion helper even when no observer is installed. Raw pprof locations 156 and 448 attribute **51,451 objects and 3,292,864 bytes** directly to that line. These equal `2237×23` objects and `2237×1472` bytes, matching the entire extra **23 allocations and 1,472 bytes per parse** in the randomized R4 results. The profile reports 2,133 timed benchmark iterations; its allocation totals also include calibration/warm calls, so 2,237 is not the timed-iteration count. The archive includes a Python protobuf reader that independently verifies the exact function/line totals and frozen source expression. This identifies diagnostic formatting as the allocation cause; the measured V3 regression and rejection remain intact. No unexecuted corrected variant is included or claimed.

The profile includes initialization and table compilation, so its overall percentages and profiler-perturbed benchmark time are attribution data rather than comparison evidence. The [V3 verification](v3-verification.json), [V3 source identities](v3-source-identities.json), [paired ratios](v3-paired-r4-ratios.json), [supplemental archive](v3-evidence.tar.gz) and [checksum](v3-evidence.sha256) retain all V3 controls, combined test overlays, exact commands/workers, raw qualification and 40 timing pairs, benchstat, input/binary authentication and the raw allocation profile. Executable binaries, caches, credentials and private CSV/Enforce payloads remain excluded.

Extract `evidence.tar.gz` first, then `v3-evidence.tar.gz` into the same new directory. Their members do not overlap. Verify both payload manifests. The separate replay supports `dense-root-qualification-v3`, `dense-perf-v3` and `dense-alloc-v3`; it prints by default and acquires the shared lock only with `--execute`. The perf replay uses the frozen V3 qualification receipt under `controls/out`, as the original command did.

```sh
sha256sum -c v3-evidence.sha256
tar -xzf v3-evidence.tar.gz -C /path/to/new-dense-v2-receipt
cd /path/to/new-dense-v2-receipt
sha256sum -c payload.sha256
sha256sum -c v3-payload.sha256
python3 analyze-v3-allocation.py
python3 reproduce-v3.py --group dense-perf-v3 --repo /path/to/gotreesitter --module-cache /path/to/go-mod --c-cache /path/to/c-cache --runtime /home/draco/work/.runtime --output /path/to/new-v3-results --cpu 6
```

## Rejected V4 diagnostic correction

**REJECTED: the allocation artifact is repaired, but throughput still regresses.** V4 changes one V3 hook: it constructs the concatenated refusal reason only when a private observer is installed. Observed diagnostic text remains identical. The exact reverse-diff guard restores the frozen V3 code; every other implementation and private test source is byte-identical. V2 and V3 archives and verification files remain unchanged.

Focused qualification again passes without skips or failures: nine core top-level tests (13 records), two lifetime tests, 15 directly selected memory tests (18 records), ten Go C top-level tests (47 records), and both unchanged 72-step Go invariant sessions with zero no-edit allocations. All eight matched entry/route pairs retain equal public digests, work records, scheduler work, other Core work, and payload/scanner/dependency receipts. The graph reductions remain `15505→2` generated and `1381→752` R4. This retains the same bounded correctness scope as V3, without a full canonical memory-pin, fleet or large-file RSS qualification.

The **Go R4-only** timing run completes 20 matched seed pairs / 40 processes using the same randomized protocol and diagnostic build flags. Compact routing remains one per operation with zero fallbacks. Primary and YAML timing were not rerun and receive no V4 performance claim.

| Go R4 measurement | V4 paired baseline | V4 candidate | Result |
| --- | ---: | ---: | --- |
| Compact time | 844.1 µs | 913.6 µs | **+8.24%, p=0.009** |
| Compact bytes | 1,112/op | 1,112/op | Equal |
| Compact allocations | 14/op | 14/op | Equal |
| Legacy time | 713.9 µs | 700.0 µs | Inconclusive, p=0.779 |
| Median of within-seed compact/legacy ratios | 1.1753284727 | 1.2984818166 | Current-legacy diagnostic |
| Ratio of median compact time to median legacy time | 1.1822617560 | 1.3050856174 | Separate statistic |

Removing unconditional diagnostic formatting eliminates the V3 allocation increase in this corrected paired run. It does not establish a throughput win: the V4 candidate still runs slower than its own paired baseline. The separately collected V3/V4 timings are not a matched comparison against each other. No production source, default route, graduation decision or merge is promoted.

The [V4 verification](v4-verification.json), [source identities](v4-source-identities.json), [paired ratios](v4-paired-r4-ratios.json), [archive](v4-evidence.tar.gz) and [checksum](v4-evidence.sha256) retain the exact one-hook diff guard, controls/test overlays, commands/workers, all raw qualification and timing output, and source/control/binary authentication. No executable binaries, caches, credentials or private CSV/Enforce payloads are included. The archive has no overlapping members with either earlier archive; it uses ordinary sparse-overlay files and adds no materialization protocol.

Extract all three archives, in order, into the same new directory; verify all three payload manifests. The V4 replay prints by default and executes only with `--execute`, under the shared lock. It accepts `dense-root-qualification-v4` and `dense-perf-v4`; the perf command uses the frozen V4 qualification receipt as its prerequisite.

```sh
sha256sum -c v4-evidence.sha256
tar -xzf v4-evidence.tar.gz -C /path/to/new-dense-v2-receipt
cd /path/to/new-dense-v2-receipt
sha256sum -c payload.sha256
sha256sum -c v3-payload.sha256
sha256sum -c v4-payload.sha256
python3 reproduce-v4.py --group dense-perf-v4 --repo /path/to/gotreesitter --module-cache /path/to/go-mod --c-cache /path/to/c-cache --runtime /home/draco/work/.runtime --output /path/to/new-v4-results --cpu 6
```
