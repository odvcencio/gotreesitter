// Package scannerpolicy resolves immutable external scanner capabilities.
// It has no dependency on scanner-facing types or either parser engine.
package scannerpolicy

// Flags describes which scanner snapshots can be omitted safely.
type Flags struct {
	Stateless        bool
	PreservesFailure bool
	RetainsFailure   bool
	Checkpoints      bool
}

// Resolve reads capabilities once for a scanner binding. Missing capabilities
// keep transactional scanning and state capture enabled.
func Resolve(scanner any) Flags {
	var flags Flags
	if capability, ok := scanner.(interface{ ExternalScannerIsStateless() bool }); ok {
		flags.Stateless = capability.ExternalScannerIsStateless()
	}
	if capability, ok := scanner.(interface{ PreservesStateOnScanFailure() bool }); ok {
		flags.PreservesFailure = capability.PreservesStateOnScanFailure()
	}
	if capability, ok := scanner.(interface{ RetainsStateOnScanFailure() bool }); ok {
		flags.RetainsFailure = capability.RetainsStateOnScanFailure()
	}
	if capability, ok := scanner.(interface{ UsesExternalScannerCheckpoints() bool }); ok {
		flags.Checkpoints = capability.UsesExternalScannerCheckpoints()
	}
	// A retained failure mutation contradicts statelessness. Keep state
	// capture enabled when a binding supplies those conflicting claims.
	flags.Stateless = flags.Stateless && !flags.RetainsFailure
	// Statelessness also certifies failure preservation.
	flags.PreservesFailure = flags.PreservesFailure || flags.Stateless
	return flags
}
