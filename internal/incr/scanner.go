package incr

// StatelessReadScanner certifies read dependencies for legacy subtree reuse.
// Scan retains no cross-token state and observes source only through the lexer
// read APIs. The observer still declines backward and column dependencies.
// This certificate does not admit the scanner to compact incremental scheduling.
type StatelessReadScanner interface {
	SupportsStatelessReadDependencies() bool
}

// CheckpointReadScanner certifies native subtree read dependencies in addition
// to serialized scanner state. Checkpoint support alone does not certify this
// stricter first-leaf and reduction proof.
type CheckpointReadScanner interface {
	SupportsCheckpointReadDependencies() bool
}

// LargeCheckpointReadScanner enables complete read receipts for the certified
// large-edit path while preserving the established small-input frontier.
type LargeCheckpointReadScanner interface {
	SupportsLargeCheckpointReadDependencies() bool
}

// LargeStatelessReadScanner is the corresponding capability for empty state.
type LargeStatelessReadScanner interface {
	SupportsLargeStatelessReadDependencies() bool
}
