//go:build gts_recovery_telemetry

package gotreesitter

import "github.com/odvcencio/gotreesitter/internal/retrybudget"

// parserColdState adds attempt storage only to diagnostic builds.
type parserColdState struct {
	forestDeclineMemoState
	cNodeMemoRetainedCache          []cNodeMemoCacheEntry
	pendingForkStackReserve         []glrStack
	pendingFrontierForkStackReserve []glrStack
	cNodeMemoCollisions             uint64
	recoveryRuntime                 recoveryRuntimeTelemetry
	// memoryBudgetBytes is the SetMemoryBudgetBytes value. Zero keeps the
	// default budget. A negative value turns the per-parse budget off.
	memoryBudgetBytes       int64
	recoveryRuntimeDetailed *recoveryRuntimeDetailedState
	retryBudgetStorage      retrybudget.Budget
	retryBudget             *retrybudget.Budget
}
