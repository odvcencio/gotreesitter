# Compact storage census, 2026-10-22 UTC

Status: diagnostic evidence identifies the next storage experiment. No runtime changes, throughput improvement, peak-RSS qualification, routing change or graduation follows this receipt. PR #1509 retains its RSS hold. All workloads use its qualified Go source at `4de229c4b1718989917c6212f92cf74df28cf229`.

## Current shared reduction path

Native M1 Max Docker Linux ARM64, Go 1.25.14, one grammar per process, GOMAXPROCS=1, 4 GiB container memory and no manual GC. All five topology workloads accept the complete clean input. The owned YAML route is diagnostic; ordinary YAML still falls back after its compact attempt. These are construction censuses, not before/after timing comparisons.

| Workload | Reductions | One clean pop without extras | Share | Published graph links | Links with zero score/order/flags |
| --- | ---: | ---: | ---: | ---: | ---: |
| YAML R4 owned | 213 | 184 | 86.38% | 355 | 307 |
| YAML 8 KiB owned | 837 | 718 | 85.78% | 1,402 | 1,227 |
| YAML R4 ordinary attempt | 28 | 24 | 85.71% | 50 | 44 |
| Go grammargen_lr | 74,911 | 67,330 | 89.88% | 148,367 | 93,214 |
| Go 1 MiB | 516,046 | 490,244 | 95.00% | 1,032,094 | 748,271 |

The census defines that common path as a fresh reduction, one source link, one pop path, no recovery discontinuity, no cost callback, no trailing extras and exactly the grammar action's structural child count. It does not claim the entire parse is deterministic or that a new LR backend is proved. [Summary](summary.json), [complete counters](topology-projection.json).

Stored graph links currently occupy 32 bytes. A private prototype uses 20-byte common records with an exact full-width side table for large scores, large ordering values and unknown flag bits. This nominal slot reduction is not a measured allocation or throughput result. It needs complete rollback, wide-value, corruption, memory-accounting, C, D8 and counter qualification before timing or promotion. The observed link counts make storage a substantial target; they do not establish which encoding will win.

## Materialization volume and acceptance comparisons

The current Go 1 MiB parse visits **903,084 compact subtrees**, including **309,627 hidden subtrees**. Its physical arena has 903,086 node slots, while the identical-digest public census has 593,457 nodes. YAML R4 owned materializes 292 subtrees, including 200 hidden ones; the 8 KiB witness has 1,172 visits, including 803 hidden ones. The hidden counts are a projection target, not authorization to omit nodes: aliases can expose hidden symbols, and fields, spans, error propagation, coverage and reuse receipts still need their current proofs.

All five current fixtures have **zero tied acceptance-election comparisons**. Repeating the October 11 acceptance-trial optimization would miss these workloads. The first observer's workloads all passed, but its postcheck incorrectly required owned YAML to have a tied election. The [corrected audit](projection-corrected-audit.json) preserves that failed assumption and all original controls/logs. It changes the analyzer only; no workloads were rerun to hide a failure. The later topology observer confirms the zero-election result and counts all publication visits.

The first topology recorder failed to compile because it assumed the primary checkout's older contiguous node/subtree arenas; the current candidate uses blocked storage. Its failure is archived separately. The repaired recorder uses the candidate's actual accessors and passes all five workloads. This is a diagnostic-harness repair, not a runtime fix. All future source analysis for the experiment is pinned to the qualified candidate.

## Temporary parents: limited copy-elimination opportunity

Three parses per route of the exact 1,048,591-byte Go witness produce the same full public digest. The default route creates **232,220 temporary parents** but copies only **51,605 (22.22%)** into the final arena. Removing those copies could avoid at most **5,366,920 bytes** of Node copies. Whole-slab adoption adds lifetime/ownership complexity and cannot by itself explain the roughly 42 MB failed peak-RSS margin. The compact route uses no temporary parents. [Parent census](parents-census.json), [decision](parent-copy-decision.json).

The existing pending-parent mode, enabled only in a separate diagnostic process, also preserves this witness's public digest over three parses. It raises physical node slots **928,887→1,032,096**, while the reported logical construction counter remains 993,391. Reported arena plus scratch capacity moves **213,295,456→204,018,344 bytes**, about 4.35% lower. These capacity totals are neither peak RSS nor B/op. The mode has no C/D8/performance qualification here and is not enabled by this receipt. [Pending-mode census](pending-census.json).

This result is why the memory investigation records physical slots and retained capacity as well as logical node work. A constant logical counter alone does not establish constant physical allocation.

## Source and receipt integrity

The [parent archive](parents-receipts.tar.gz), [pending-mode archive](pending-receipts.tar.gz), [first projection archive](projection-receipts.tar.gz), [failed topology recorder](failed-topology-receipts.tar.gz) and [final topology archive](topology-receipts.tar.gz) retain controls, full logs, source manifests and terminal container audits. Adjacent archive JSON files pin byte counts and SHA-256; every internal manifest member is verified after retrieval. Executables, checkouts and caches are excluded.

The source closure has 3,544 Go files and 756 root Go files. Diagnostic overlays are listed and hashed in each scope file. Every original source, instrumented source and the user's Mac checkout remains unchanged after its run. All containers terminate with no OOM. The failed recorder's compile error and the projection observer's failed postcheck remain explicit; neither is reported as a product failure or performance pass.

Reproduce using the pinned source, the archives' exact scope overlays and Docker command records. Each worker uses the shared compute lock and image `golang@sha256:3b4a11519ad929d1e1d261a12cff056f0c85b735253d7d861346b9c6f8b36437` with explicit native `linux/arm64`. The [combined main-to-final performance receipt](../20261022-combined-incremental-overhead/README.md) remains the evidence for PR #1509's measured wins and failed absolute RSS gate. These censuses do not replace those gates or close the known Go-delete C mismatch or ordinary YAML ownership blocker.
