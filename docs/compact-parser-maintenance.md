# Compact parser maintenance

Use these source boundaries when you change the compact parser.
All compact implementation files use the `!gts_no_parsercorephase0` build constraint.

| Responsibility | Source owner |
| --- | --- |
| Schedule elections and dispatch parser actions | `parsercore_phase0_driver.go` |
| Measure retained memory and poll stop conditions | `parsercore_phase0_stop_control.go` |
| Project symbol metadata for recovery and materialization | `parsercore_phase0_symbol_policy.go` |
| Read compact subtrees for recovery pricing | `parsercore_phase0_recovery_cost_source.go` |
| Compute recovery costs and measure their memo | `internal/parsercorephase0/recovery_cost.go` |
| Measure compact-core storage | `internal/parsercorephase0/storage_bytes.go` |

## Add retained state

Before you add a slice, map, or cache, identify its owner and lifetime.

1. Specify whether reset releases the allocation or clears its contents.
2. Add its retained capacity to the owner's footprint method.
3. Count shared storage once, even when several headers reference it.
4. Test growth, reset, and memory-limit enforcement.
5. Preserve overflow protection in the aggregate calculation.

The scheduler retains the recovery-cost memo across parses and clears its entries at reset.
Its allocated capacity still counts after reset.
The scheduler discards the recovery-symbol projection at reset and rebuilds it when the next parse needs it.

## Change symbol metadata

Use `parserCoreSymbolPolicy` for recovery and selected-store materialization.
The projection covers the maximum width of metadata, symbol names, and the declared symbol count.
Symbols without metadata remain visible and unnamed.
Changing either default changes recovery costs and can change the selected tree.

Keep parser-specific unary rules in `buildParserCoreSelectedStorePolicy`.
Do not build the quadratic unary table merely to obtain recovery visibility.

## Change stop control

Keep the existing order: memory accounting, active parser stops, then node-cap prediction.
Keep the existing eager-materialization polling frequency.
Use retained capacities for the deterministic scheduler budget.
Do not substitute process-wide heap measurements for that budget.

Run focused tests in bounded Docker containers. Use one grammar per parity run.
Cover these contracts when the change affects their owner:

- Memory growth and retained capacity after reset.
- Deadline and cancellation receipts.
- Recovery costs and selected-store symbol metadata.
- Fresh and incremental trees against the pinned C parser.
- The emergency build with `gts_no_parsercorephase0`.

Use the randomized benchmark runner for performance comparisons.
Do not describe source-file extraction as a performance improvement.
