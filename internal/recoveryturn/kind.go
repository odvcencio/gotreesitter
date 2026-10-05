// Package recoveryturn distinguishes versions advanced before their native
// physical dispatch turn by the shared-lookahead recovery loop.
package recoveryturn

// Kind fits in the recovery stack header's existing one-byte marker.
type Kind uint8

const (
	None Kind = iota
	Missing
	Resync
)

// ClosedHistoriesCertified keeps checkpoint-backed scanners on their established
// recovery path until physical recovery order, not just conflict order, is proven.
func ClosedHistoriesCertified(conflictOrder, recoveryOrder, scannerCheckpoints bool) bool {
	return conflictOrder && (!scannerCheckpoints || recoveryOrder)
}

// DefersRecoveryCompetition keeps advanced versions out of the recovery
// election until their physical dispatch. Native version order supplies the
// same scheduling proof as a dedicated missing-version certification.
func (k Kind) DefersRecoveryCompetition(missingCertified, nativeOrderActive bool) bool {
	switch k {
	case Resync:
		return true
	case Missing:
		return missingCertified || nativeOrderActive
	default:
		return false
	}
}
