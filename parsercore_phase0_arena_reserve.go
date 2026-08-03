//go:build !gts_no_parsercorephase0

package gotreesitter

// compactArenaReserveCapBytes bounds how much record-arena capacity one parse
// may reserve up front (Core.ReserveRecordArenas,
// internal/parsercorephase0/arena_reserve.go).
//
// It is half the compact core's own retention cap (coreRetentionCapBytes, 48
// MiB, internal/parsercorephase0/storage_bytes.go). Staying strictly below
// that cap matters: releaseOversizedRetention compares the core's TOTAL
// FootprintBytes against it, so a core born at a full 48 MiB reserve would
// already be oversized by the project's own standard and would lose its
// arenas on the next decline. Retention would then stop being monotonic in
// source length -- a 400 KB source would keep its arenas while a 600 KB
// source lost them. At 24 MiB a maximum reserve leaves room for the boundary
// index, the checkpoint interner, and the scratch families before the
// retention cap can trip on capacity the reserve took.
//
// 24 MiB also covers every canonical fixture whole: the largest, at 235,626
// bytes, estimates 21.3 MiB. The cap starts to bind at roughly 265 KB of
// source. Past that the reserve stays flat, so a large source starts from a
// bounded base and grows the rest, which is still far cheaper than growing
// the whole arena from zero.
const compactArenaReserveCapBytes = 24 << 20

// compactArenaReserveBudgetDivisor bounds the reserve as a fraction of every
// memory limit the scheduler polls. Core.FootprintBytes gauges capacity, not
// live length, and the scheduler's stop-control poll compares that one gauge
// against BOTH the caller's soft budget and production's absolute hard
// ceiling (stopControlMemoryBudgetReason, parsercore_phase0_driver.go). A
// reserve therefore raises the gauge immediately, at construction, before
// the parse publishes one record.
//
// Dividing by eight bounds that effect against each armed limit
// independently: whatever the configured budget and ceiling are, the reserve
// can add at most one eighth of the smaller one to the gauge, so a parse
// that completes today still has at least seven eighths of every limit left
// after the reserve. At the shipped defaults (512 MB budget, 2 GB ceiling)
// the 24 MiB cap binds first and the real share is 4.7% of the budget.
const compactArenaReserveBudgetDivisor = 8

// compactArenaReserveBytes reports the record-arena reserve ceiling for a
// parse whose soft memory budget is budgetBytes and whose absolute hard
// ceiling is hardCeilingBytes.
//
// Both limits are honored, and each independently, because the scheduler
// arms them independently: parseMemoryHardCeilingBytes stays on even when a
// caller sets GOT_PARSE_MEMORY_BUDGET_MB to zero
// (parsercore_phase0_fresh_full_runner.go), and stopControlMemoryBudgetReason
// tests the same footprint gauge against each. A limit at or below zero is
// not armed and does not constrain the reserve; when neither is armed (the
// diagnostic and benchmark runners arm neither) only the absolute cap
// applies.
func compactArenaReserveBytes(budgetBytes, hardCeilingBytes int64) uint64 {
	reserve := uint64(compactArenaReserveCapBytes)
	for _, limit := range [2]int64{budgetBytes, hardCeilingBytes} {
		if limit <= 0 {
			continue
		}
		if share := uint64(limit) / compactArenaReserveBudgetDivisor; share < reserve {
			reserve = share
		}
	}
	return reserve
}

// sourceLength reports the byte length of the source this token source lexes,
// or zero when it has none. It is the reserve estimator's only input.
func (d *dfaTokenSource) sourceLength() int {
	if d == nil || d.lexer == nil {
		return 0
	}
	return len(d.lexer.source)
}
