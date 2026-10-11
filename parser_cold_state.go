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
	// Retokenized paused heads own their lookahead until condense resumes them.
	// Keep these uncommon tokens outside every hot stack version.
	cPausedLookaheads map[*gssNode]Token
	recoveryRuntime   recoveryRuntimeTelemetry
	// memoryBudgetBytes is the SetMemoryBudgetBytes value. Zero keeps the
	// default budget. A negative value turns the per-parse budget off.
	memoryBudgetBytes int64
	// Hidden verification keeps routing but emits no public admission event.
	admissionCountersSuppressed bool
	// Nonadvancing stateless markers require C recovery ordering and GSS paths.
	crecoveryEmptyExternal bool
	admissionConvergedWork [3]uint64
}
