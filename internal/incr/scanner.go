package incr

// StatelessReadScanner certifies read dependencies for legacy subtree reuse.
// Scan retains no cross-token state and observes source only through the lexer
// read APIs. The observer still declines backward and column dependencies.
// This certificate does not admit the scanner to compact incremental scheduling.
type StatelessReadScanner interface {
	SupportsStatelessReadDependencies() bool
}
