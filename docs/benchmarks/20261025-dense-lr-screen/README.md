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
