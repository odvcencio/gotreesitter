//go:build !gts_recovery_telemetry

package gotreesitter

// parserColdState shares the Parser's existing lazy sidecar slot between
// uncommon features. It preserves the hot Parser layout for ordinary parses.
type parserColdState struct {
	forestDeclineMemoState
	cNodeMemoRetainedCache          []cNodeMemoCacheEntry
	pendingForkStackReserve         []glrStack
	pendingFrontierForkStackReserve []glrStack
	cNodeMemoCollisions             uint64
	recoveryRuntime                 recoveryRuntimeTelemetry
	// memoryBudgetBytes is the SetMemoryBudgetBytes value. Zero keeps the
	// default budget. A negative value turns the per-parse budget off.
	memoryBudgetBytes int64
}
