# Compact output integration

This bucket repairs correctness and integration gaps exposed by CI on the owned execution loop and compact public publisher. Go builtin type arguments now receive the same symbols and fields as the C tree directly in compact records. A failed native byte-coverage proof remains a typed decline, allowing the existing recovery retry. Synthetic grammars without an authenticated root retain their supported publication path.

The direct-reduction GOTO memo moves out of every classified cell into one authenticated core-owned slot. It is consumed only by the same classified boundary and predecessor. Classified boundaries shrink from 56 to 48 bytes, restoring the 64-byte scheduler-cell limit. A regression test rejects reuse across a new frontier.

Eight added root files are consolidated into existing files with their complete bodies unchanged; `relocation-audit.json` records the exact matches. The experimental view switch moves into `internal/parsercorephase0`. The root returns to its existing 756-file allowance, and all 12 API tag snapshots pass. No root allowance, counter tolerance, memory limit, admission allowlist, or global default is relaxed.

Baseline: `6e9460228d50a113233fe081cfe05ffe50379f1e`. This is a comparison against the preceding stacked bucket, not main or C. All merges to main remain held. Public-output benchmarks explicitly enable the candidate route. YAML uses its ordinary eager publisher; the experimental `gts_compact_node_views` publisher remains separate. The default route is unchanged.

## Deterministic evidence

All 824 full/edit work rows and 206 per-language selected-ledger comparisons are unchanged. Completion, source coverage, error flags, route decisions and reuse are also unchanged. Selected node-work examples:

| Language and route | Full before → after | First edit before → after |
| --- | ---: | ---: |
| Go candidate | 892 → 892 | 1,553 → 1,553 |
| Go default | 1,922 → 1,922 | 3,810 → 3,810 |
| YAML candidate | 232 → 232 | 232 → 232 |
| YAML default | 288 → 288 | 480 → 480 |

The checked YAML candidate ledger previously recorded an unsupported route without numeric counters. It now records the existing admitted route's first measurements: 107 tokens, 232 nodes, two maximum live versions for full parsing, and 107 tokens/232 nodes for the first edit. The edit retains its documented candidate-incremental fallback. This does not replace a measured counter with a larger value. The prior experimental YAML-view receipt's 231-node result uses a different output flag; it is not the ordinary row above.

The GitHub Actions YAML depth witness now requires the route it already takes, preserving its existing deep-tree digest. Go's qualified-name cases retain their existing ambiguity fallback where needed; the C test logs the route for each form. The admitted builtin cases additionally check lazy root publication, complete structural equality, independent copies, and edits against a fresh parse.

## Qualification

The isolated gate passes 412 invariant sessions across all 206 grammars: 29,664 edit steps, exact incremental/fresh equality, and zero no-edit allocations. CSV and Enforce use the existing grammar-smoke fixtures; the other 204 use R4. All 1,236 standard locked-C checks pass. Focused Go builtin, YAML EOF/document/recovery, and JavaScript recovery-oracle checks also pass, along with core/public-view races, 20 memory-contract tests, layout checks and the emergency build.

The final follow-up changes only source-byte comparisons inside the authenticated Go type-argument normalizer. Go repeats its invariant, C, edit/copy, race, work-counter and ledger checks on that source. Other language paths do not invoke that normalizer and inherit the complete fleet gate. `qualification.json` records this boundary explicitly.

The first complete performance run passed the relative gates but exposed avoidable temporary token-name strings: the large Go parse rose from 1,590 to 1,706 allocations. That version was not selected. The final version compares bytes in place. Both complete sample sets are retained; final Go timing and memory results come from the repeated run.

## Performance and memory

All **760 final timing/allocation rows** pass the 10% relative ratchets. All eleven Go timing comparisons are inconclusive. Their intervals are wide in this run; observed directional medians remain listed below. Allocation-count medians are unchanged, including 1,590 for the large Go parse, 22 for the primary full parse, five for its single-byte edit, and zero for no-edit. No-edit also allocates zero bytes. No new Go throughput gain is claimed.

The eight YAML comparisons improve **0.84–1.52%** (p≤.011), with unchanged bytes and allocations. These are modest branch-relative measurements; they do not qualify the experimental YAML view publisher or establish graduation.

| Workload | Before | After | Time change |
| --- | ---: | ---: | ---: |
| Go `grammargen_lr_walk` | 107.693 ms | 110.420 ms | +2.53% |
| Go `query_compile_parse_edit` | 50.156 ms | 52.810 ms | +5.29% |
| Go `grammargen_lr_parse` | 103.417 ms | 101.140 ms | -2.20% |
| Go `query_compile_walk` | 10.589 ms | 10.583 ms | -0.05% |
| Go `grammargen_lr_parse_edit` | 444.812 ms | 447.550 ms | +0.62% |
| Go `query_compile_parse` | 10.068 ms | 10.143 ms | +0.75% |
| Go `grammargen_lr_query` | 149.086 ms | 149.116 ms | +0.02% |
| Go `query_compile_query` | 15.328 ms | 15.027 ms | -1.96% |
| Go primary `IncrementalNoEditDFA` | 3.716 ns | 3.559 ns | -4.24% |
| Go primary `FullDFA` | 8.701 ms | 8.876 ms | +2.01% |
| Go primary `IncrementalSingleByteEditDFA` | 105.059 µs | 105.049 µs | -0.01% |
| YAML `eof8k_parse_edit` | 1.438 ms | 1.421 ms | -1.18% |
| YAML `eof8k_walk` | 722.876 µs | 713.239 µs | -1.33% |
| YAML `r4_parse_edit` | 373.964 µs | 370.837 µs | -0.84% |
| YAML `eof8k_query` | 741.293 µs | 730.011 µs | -1.52% |
| YAML `eof8k_parse` | 717.706 µs | 710.984 µs | -0.94% |
| YAML `r4_query` | 193.306 µs | 191.119 µs | -1.13% |
| YAML `r4_walk` | 188.369 µs | 185.904 µs | -1.31% |
| YAML `r4_parse` | 186.913 µs | 185.007 µs | -1.02% |

All samples, uncertainty estimates and significance tests are in the three `benchstat-*.txt` files. The final Go suites use seeds 262001–262020 and 263001–263020; the unchanged YAML path inherits seeds 260001–260020. The first version's 760 rows remain in the archive and are not substituted for the final Go samples.

All **80 matched memory pairs** pass: 20 pairs for each language/consumption mode. Every release returns live heap to within 1 MiB of its warm baseline, output digests agree, and peak RSS stays below both the 10% relative gate and 400 bytes/source byte. Go uses the repeated final run; YAML inherits the unchanged path's complete run.

| 1 MiB result | Peak RSS before → after, KiB | Change |
| --- | ---: | ---: |
| go-root | 249,092 → 246,888 | -0.88% |
| go-walk | 249,284 → 248,200 | -0.43% |
| yaml-root | 74,816 → 73,332 | -1.98% |
| yaml-walk | 75,312 → 74,724 | -0.78% |

Reported retained deltas are above warmed parser pools, not total tree storage. `retention-analysis.json` includes total live heap and arena capacity; the archive retains every process log and GNU time report.

The protocol uses pinned Docker, Go 1.26.6 on Linux amd64, one CPU, `GOMAXPROCS=1`, 20 explicit shuffle seeds, one process per seed, `750ms`, and allocation reporting. The repository randomized driver alternates revision order and verifies identical workload order within each pair. Correctness and deterministic counters precede timing. The 1 MiB memory probes retain every matched pair and measure peak RSS, live heap, output geometry and release behavior.

## Remaining limits

The broader CI SQL certification reported 69,757,496 bytes against its 67,764,224-byte limit. Isolated baseline and candidate certification suites both pass twice, with identical memory ranges of 4,715,248–24,860,248 bytes. A test-only 52 MiB transient-parent history is trimmed before the small SQL parse and also passes identically; it rejects that specific explanation. The broader failure is unresolved, and its limit is unchanged.

[Cognociente's latest reviewed report](https://github.com/odvcencio/gotreesitter/issues/454#issuecomment-6311551850) still governs the downstream check: the malformed Go deletion differs from C, and the small-source fresh verifier remains enabled. This bucket does not establish compact graduation, main-based net performance, native Windows/ARM64 qualification, or a new C-speed result. The newer main-only YAML indentation-frame contribution is outside this frozen comparison.

The receipt archive contains source overlays and hashes, exact container commands, correctness logs, paired raw benchmarks, analyses, and the unsuccessful SQL history hypothesis. Reproduction requires the pinned image and locked C/module caches recorded in those commands. Paths may be remapped, but both roles must use the same corpus, compiler, environment and controls.

[Raw receipt archive](receipts.tar.gz), SHA-256 `79c7fb2e2a167103f4ffe1b3b1fdd9c3b4fca8c229427836b4541896836ae580` (1293003 bytes). The [receipt manifest](receipt-manifest.json) records the archive identity and content summary; all 3,208 files were read back and verified before publication.
