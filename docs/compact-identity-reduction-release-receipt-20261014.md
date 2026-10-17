# Identity reduction metadata — release qualification, 2026-10-14

Bucket: measured diagnostic-owned YAML throughput win. No ordinary YAML activation or graduation is claimed.

A complete, non-recovery pop path with exactly the production's structural child count contains no interstitial extras. Its field and alias positions already equal those in the immutable, authenticated reduction plan. The core now borrows that plan metadata for this case; `appendSubtreeRecord` still copies every published sidecar into core-owned storage. Recovery discontinuities, extras and partial alias vectors retain the general remapper. The regression compares against that original remapper at widths 0, 1, 3 and 255, including inherited fields, aliases, extras and partial vectors.

The retained two-file patch applies cleanly to main after v0.56.0 and #1452. Its core implementation and test remain the same as the earlier qualified experiment. New release-based qualification passes: full core tests, focused root/reduction tests, canonical lifecycle, Go/YAML and owned-YAML edit invariants, **all 206** 72-step invariant sessions, **618** per-language C fixture checks, and strict R4 census **159 passes / 45 known failures**, with **120 compact-admitted C passes** and no new failure or lost admission. Core/root races and six memory-budget trials pass. The **1,049,572-byte**, 524-document YAML witness is structurally exact against locked C, consumes the full input without fallback/error, and peaks at **156,436 KiB RSS** (below 400 bytes per source byte).

All **412 ordinary ledger rows** match the pinned baseline byte-for-byte. Owned YAML full/edit nodes stay **484→484 / 484→484**, retaining the previously approved values. Default YAML full/edit nodes stay **288→288 / 480→480**. Tokens, versions, multi-version shares, reuse, splices, stop reasons, extents and errors are unchanged. Ordinary YAML retains its no-action alternative-set ownership decline.

Twenty shuffled alternating pairs use the repository runner with one process per seed, `GOMAXPROCS=1`, `-count=1`, `750ms`, `-benchmem`, a pinned CPU and 6 GiB Docker isolation. The shared host lock inodes are held inside the container throughout each run. The timing container exits zero without OOM/timeout.

| Benchmark | Before → after | p | Bucket |
| --- | --- | --- | --- |
| Owned YAML | 383.9 → 379.8 µs | <.001 | **−1.07% time**, supported |
| Small compact Go | 893.0 → 867.3 µs | .242 | Inconclusive |
| 20 KiB Go | 13.96 → 13.63 ms | .799 | Inconclusive |
| 235 KiB Go | 152.9 → 143.5 ms | .640 | Inconclusive |
| Legacy Go control | 692.9 → 670.8 µs | .925 | Inconclusive |
| Full DFA control | 4.156 → 4.047 ms | 1.000 | Inconclusive |
| Single-byte edit control | 102.5 → 101.6 µs | .620 | Inconclusive |
| No-edit control | 3.233 → 3.183 ns | .723 | Inconclusive |

YAML stays **12,936 B/op / 111 allocations**, small compact Go **1,696 B/op / 25 allocations**, larger Go **204 / 3,091 allocations**, and no-edit **0 B/op / 0 allocations**. The 20 KiB Go byte median changes **141,123→141,123.5** and the larger Go median **3,295,752.5→3,295,747**, with no supported memory claim. All timed work/completion metrics are exact and every 10% median ratchet passes. Baseline/unmodified hashes for **3,477** Go/module inputs, the exact candidate and five retained-storage alignment files are verified after timing.

The earlier independent receipt measured 562.8→534.0 µs, −5.12% (p=.020), on the pre-release baseline. The release-based measurement above establishes the same direction with a smaller effect. Keep the measurements separate; do not combine them with the inline-retry-mask memory result or imply a Go gain.

The strict-census comparison initially asserted 206 rows although the pinned real-corpus manifest contains 204. Every suite had already completed with the totals above; that receipt assertion is corrected, and only remaining races/memory gates are resumed. The excluded assertion attempt remains in the evidence. Final focused/full-fleet and continuation containers exit zero without OOM/timeout. Actual main-based source-surface guards retain L0 756/756 files and 12 tag sets, and R6 454 language uses / 101 environment reads.

Measurement baseline: release-integrated dependency head `a32d87b6d23b090fef2559380b44bfb2c86a800e`. The PR branches from main `8bfb2080874201485da7588a4a27a5f439803833`; it does not carry #1480's storage changes. Source-surface guards run on that actual two-file PR checkout. All routing, allowlists, caps and correctness rules remain unchanged. This change does not close the Go throughput or ordinary YAML ownership gaps.

Evidence: `/home/draco/.local/state/gotreesitter-evidence/20261014-release-yaml-identity-reduction`.
