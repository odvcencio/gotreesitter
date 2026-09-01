//go:build !gts_merge_census

package gotreesitter

// Shipped build of the merge-event census (stage M0 of
// spec.merge-time-election.v1): the census is absent.
//
// merge_event_census.go carries the whole instrument behind the
// gts_merge_census build tag. This file supplies the same names with empty
// bodies and, above all, mergeCensusEnabled as a false CONSTANT. Every census
// block in glr.go is written as `if mergeCensusEnabled { ... }`, so the
// compiler removes the block and its argument evaluation from the default
// build. That is stronger than an env-var read: the shipped binary keeps the
// exact instruction stream it had before the census existed, which
// merge_event_census_inert_test.go ratchets.

// mergeCensusEnabled is a constant false in the shipped build.
const mergeCensusEnabled = false

// MergeEventCensusBuilt reports whether this binary carries the merge-event
// census. The shipped build never does.
func MergeEventCensusBuilt() bool { return false }

func mergeCensusRecordAttempt()                                           {}
func mergeCensusRecordSuccess()                                           {}
func mergeCensusRecordMergeFailed()                                       {}
func mergeCensusRecordDistinctShapes()                                    {}
func mergeCensusRecordErrorCost()                                         {}
func mergeCensusRecordScorePreflight()                                    {}
func mergeCensusAttributeForParserRefusal(*Parser, *glrStack, *glrStack)  {}
func mergeCensusRecordGateRefusal(*glrMergeScratch, *glrStack, *glrStack) {}
func mergeCensusRecordLinkPayload(stackEntry, stackEntry, bool)           {}
func mergeCensusRecordCompactLinkUnion(
	uint64, uint64, uint64, uint64, uint64, uint64,
	uint64, uint64, uint64,
) {
}
