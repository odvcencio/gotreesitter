# Packed scanner-page budget accounting

The scheduler now counts both current scanner-page allocations, including unused and unpublished packed slots, once per physical allocation. Published aliases do not add a second charge; retired pages remain charged while a live reference owns them. Budgets, thresholds, polling, scanner bytes and reuse rules are unchanged.

The original regression run preserved the expected baseline failure (exit **1**), corrected pass (exit **0**), and existing-memory pass. Measured page capacity was **936 B packed**, **1,192 B small**, and **2,128 B combined**.

| Control | Old footprint | Corrected footprint |
|---|---:|---:|
| Unpublished packed page | 2,280 B | 3,216 B |
| Unpublished small page, passing control | 3,472 B | 3,472 B |
| Both current page shapes | 3,472 B | 4,408 B |
| Retired live + unpublished current packed page | 3,872 B | 4,808 B |

The omitted packed capacity also caused the old exact soft/hard limit assertions to fail. Shared-page deduplication and immutable-record controls passed before and after.

The final saved-source gate passed **56 focused tests**, **412 sessions / 29,664 edit steps / zero no-edit allocations**, and **618 C fixture checks**. Sessions comprised 408 R4 and four grammar-smoke controls (`csv` and `enforce`, both routes). Focused race coverage passed nine top-level tests, with 12 PASS records and no race warning.

The following totals come from retained **actual before/after process rows**, not tracked ledger ratchets. Every complete JSON row, including fixture/session identities, decline reasons, outputs and counters, is equal.

| Metric, before = after | Default | Candidate |
|---|---:|---:|
| Actual rows compared | 206 | 206 |
| Rows with counters / declined rows | 206 / 0 | 115 / 91 |
| Full tokens | 79,127 | 40,065 |
| Full new nodes | 257,215 | 79,759 |
| Edit tokens | 53,830 | 27,887 |
| Edit new nodes | 226,508 | 64,768 |
| Edit reused bytes | 158,120 | 76,816 |
| Edit block splices | 10,173 | 3,021 |

[summary.json](summary.json) and [actual-work-reuse-summary.json](actual-work-reuse-summary.json) describe these checks. [source.patch](source.patch) is the exact tested two-file patch; [source-identities.json](source-identities.json) pins before/after bytes and the complete final source manifest. Production accounting SHA256 is `7c47d7074c1bf25c471a29a76ef7d1d1557f43f3d0d1f0324f8e0fb7952cf2ae`.

[evidence.tar.gz](evidence.tar.gz) retains original failed/passed logs, all actual ledger rows, per-language invariant/C output, race output, source guards, pinned Docker commands, workers and the original validator. Historical prepared manifests remain unchanged; terminal outputs record their later execution. `package/original-controls-reconstruction.json` reconstructs the original private test from the appended patch block, with its exact source hash. No runtime source copies, binaries, caches, credentials or private CSV are bundled; archived scripts have no executable permission bits.

Run `sha256sum -c SHA256SUMS` and `python3 validate-evidence.py` in this directory to check the retained package without building or running parser tests. Replaying runtime checks requires the pinned source/dependencies and the recorded serialized Docker commands.

This is an accounting-correctness receipt. It makes no throughput, language-graduation or main-merge claim. No allowlist or default changed, and main merges remain held.
