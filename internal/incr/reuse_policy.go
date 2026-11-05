package incr

// FreshReuseBasis supplies complete local inputs to the fresh reuse schedule.
// Final publication separately requires complete current-source reads and the
// accepted-result guards. Source size and language name cannot establish them.
type FreshReuseBasis struct {
	DefaultOperation           bool
	SupportedTokenSource       bool
	AcceptedCleanOldResult     bool
	CompleteOldReads           bool
	TrustedOldProducer         bool
	CompleteOldStateProjection bool
}

func (basis FreshReuseBasis) ScheduleEligible() bool {
	return basis.DefaultOperation && basis.SupportedTokenSource &&
		basis.AcceptedCleanOldResult && basis.CompleteOldReads && basis.TrustedOldProducer && basis.CompleteOldStateProjection
}

// SmallFreshReusePublicationEligible restricts the transferred large clean
// contract to a clean result without entered recovery or dropped-error markers.
// Complete source/state/dependency and schedule proofs remain caller guards;
// a clean root alone cannot establish them. Routine policy pruning is permitted
// by the existing large clean contract and does not supply those proofs either.
func SmallFreshReusePublicationEligible(cleanResult, enteredRecovery, droppedError bool) bool {
	return cleanResult && !enteredRecovery && !droppedError
}

// DefaultProducerFacts keeps input identity separate from complete read spans
// and from the actual fresh dispatch schedule. Unknown or nondefault contexts
// cannot manufacture a positive receipt from a clean accepted tree.
type DefaultProducerFacts struct {
	ArtifactCapability                                                 bool
	ExactDFA, ParserOwnedLexer, SourceIdentity, SameLanguage, NoRanges bool
	DefaultControls, DefaultPolicy                                     bool
	MaxStacks, MergePerKey                                             int
}

// ArtifactCapability is a separate exact-artifact permission; it never supplies
// any producer/source/default-context fact.
// ScheduleKey uses existing Reads padding. Zero means unknown. Wider policies
// abstain instead of truncating the receipt or changing the parser's limits.
func (facts DefaultProducerFacts) ScheduleKey() uint16 {
	if !facts.ArtifactCapability || !facts.ExactDFA || !facts.ParserOwnedLexer || !facts.SourceIdentity || !facts.SameLanguage || !facts.NoRanges || !facts.DefaultControls || !facts.DefaultPolicy ||
		facts.MaxStacks <= 0 || facts.MaxStacks > 255 || facts.MergePerKey <= 0 || facts.MergePerKey > 255 {
		return 0
	}
	return uint16(facts.MaxStacks)<<8 | uint16(facts.MergePerKey)
}

// TransferNativeReuseProducer carries authenticated producer policy across a
// complete clean reuse proof. Unknown, changed, incomplete or partial output
// never acquires authority merely because its public root looks clean.
func TransferNativeReuseProducer(previous, current uint16, completeProof, sourceCovered bool) uint16 {
	if previous == 0 || previous != current || !completeProof || !sourceCovered {
		return 0
	}
	return current
}
