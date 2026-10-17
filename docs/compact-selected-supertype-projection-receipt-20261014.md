# Selected-store supertype provenance qualification — 2026-10-14

Bucket: correctness prerequisite for direct projection. No throughput win or compact graduation is claimed.

The selected query reader previously returned zero for every supertype mask. On `package main\nfunc f() { _ = a + b }\n`, `(_expression) @e` returned no selected captures while the native and locked-C queries returned four: `_` at 24–25, `a + b` at 28–33, `a` at 28–29 and `b` at 32–33. The field-qualified query `(binary_expression left: (_expression) @l)` supplies a second C witness.

The policy now snapshots the grammar's singleton supertype bits. Both builders fold a hidden parent's bit onto each resulting logical occurrence. Visible and retained-alias boundaries stop propagation. Repeated payloads keep independent occurrence masks, including bit 31. The reader uses the stored mask. Reordering existing fields spends padding and keeps `SelectedNodeRecord` at **36 bytes**. Invalid/released store reads fail closed. The core footprint now charges the complete retained policy backing, which was previously omitted; ordinary parsing leaves the lazy policy absent.

Qualification passes: all core tests; staged and one-pass builders; all four canonical Go fixtures with unchanged counts, retained backing, tree digests and occurrence masks equal to native; Go/YAML 72-step edit invariants with zero no-edit allocations; focused core/root races; Go/YAML locked-C fixtures; and an independently emitted C query artifact compared with selected captures. The new portable query witness fails on the unmodified baseline. The actual main-based PR checkout independently passes selected/native and exact C capture checks, memory/Node layout tests, L0 **756/756 files and 12 tag sets**, and R6 **454 language uses / 101 environment reads**. One snapshot line names the private reader's newly used parameter; no API symbol is added.

All **412 ordinary ledger rows** are byte-for-byte unchanged. Selected timing work/completion/retained-backing metrics are also exact:

| Counter | 20 KiB Go before → after | 235 KiB Go before → after |
| --- | --- | --- |
| Compact leaves | 5,546 → 5,546 | 53,896 → 53,896 |
| Compact parents | 7,415 → 7,415 | 75,769 → 75,769 |
| Links | 14,662 → 14,662 | 148,367 → 148,367 |
| Pop paths | 7,981 → 7,981 | 82,259 → 82,259 |
| Pop payloads | 14,603 → 14,603 | 150,516 → 150,516 |
| Reductions / pop requests | 7,382 → 7,382 | 74,911 → 74,911 |
| Shifts | 6,685 → 6,685 | 66,115 → 66,115 |
| Selected nodes | 7,524 → 7,524 | 71,768 → 71,768 |
| Selected retained bytes | 448,156 → 448,156 | 4,328,596 → 4,328,596 |

Twenty shuffled alternating pairs use the repository runner, `GOMAXPROCS=1`, one process per seed, `-count=1`, `750ms`, `-benchmem`, and one pinned CPU in 6 GiB Docker isolation. Every timing difference below is inconclusive:

| Benchmark | Before → after | p |
| --- | --- | --- |
| Owned YAML | 537.0 → 542.4 µs | .820 |
| Small compact Go | 1.215 → 1.214 ms | .565 |
| Selected 20 KiB Go | 12.69 → 12.70 ms | .904 |
| Selected 235 KiB Go | 137.5 → 132.7 ms | .659 |
| Selected query | 4.593 → 4.832 ms | .640 |
| Selected parse + highlight | 16.59 → 15.63 ms | .565 |
| Legacy query control | 5.141 → 5.185 ms | .883 |
| Legacy parse + highlight control | 18.32 → 16.47 ms | .314 |
| Legacy small-Go control | 941.1 → 948.6 µs | .883 |
| Full DFA control | 5.533 → 5.419 ms | .565 |
| Single-byte edit control | 137.7 → 135.3 µs | .547 |
| No-edit control | 4.234 → 4.226 ns | .635 |

Record every directional increase; the selected-query median rises **5.20%** without statistical support and remains within the 10% median ratchet. Median allocation counts stay exact: small Go **25**, selected Go **209 / 2,987**, YAML **111**, selected/legacy query **67,961 / 67,984**, selected/legacy parse-highlight **68,172 / 68,206**, and no-edit **zero**. Selected backing is unchanged; warm selected parsing stays **142,304 / 3,301,332 B/op**. Query byte medians vary by only a few bytes without a supported change. YAML stays **12,936 B/op**; small Go **1,696 B/op**.

An outer timing wrapper exited 143 while its Docker child continued. A later layout check escaped the outer lock and overlapped the initial query timing set. That entire query set is excluded. The three earlier 20-pair families finished by 08:23:18 UTC, before the check started at 08:27:42 UTC. Only the affected query family was repeated; its final container exits zero without OOM/timeout. Both host lock-file inodes are now mounted under a non-sticky container directory and held by the container throughout a run, independently of the launching shell. A first mount under `/tmp` hit the host's protected-file rule before tests started; that attempt is excluded. The initial layout failure and corrected snapshot are retained in the evidence.

Timing and initial qualification use release-integrated dependency baseline `a32d87b6d23b090fef2559380b44bfb2c86a800e`. All **3,477** baseline and non-overlaid inputs, candidate hashes and the runner copy are verified. The PR branches from main `8bfb2080874201485da7588a4a27a5f439803833` and carries none of dependency PR #1480's five storage files. Main-based qualification passes separately.

This proves the selected supertype contract for the generic occurrence cases and tested Go/C witnesses. It is not an all-grammar selected-store C certificate. Missing-query patterns remain declined. Hidden-parent public construction, replay/scanner provenance transfer, ordinary YAML ownership and the Go throughput gap remain unresolved. Routing and allowlists are unchanged.

Evidence: `/home/draco/.local/state/gotreesitter-evidence/20261014-selected-supertype-projection`.
