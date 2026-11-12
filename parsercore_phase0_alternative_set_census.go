//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"fmt"
	"os"
	"sort"
	"sync"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// The converged-alternative-set proofs and the stage 2 census.
//
// Through stage 2a, a converged-split no-action drop was proved admissible
// by a scalar (rank, lineage) pair recorded on each header
// (diagnosticParserCoreSelectedLineageDrops). Stage 2b flips
// dropGenericNoActionHeads to decide with the v2 predicate instead
// (spec.b4b-alternative-set.v2 section 8), after the section 7 gate passed
// (zero class-1 differing-tree cases; the Kotlin witness declines under v2;
// stage 1's gates re-passed). This file carries three proofs:
//
//   - v1 (event-only, superseded): a drop is admissible when one surviving
//     header's recorded set contains, for every recorded (event, branch)
//     member of the dropped header's set, some member of that same event --
//     ignoring branch (spec.b4b-alternative-set.v1 section 5).
//   - v2 ((event, branch), blended-witness veto, the decider): a drop is
//     admissible when one surviving header's recorded set contains every
//     recorded (event, branch) member of the dropped header's set exactly,
//     and the surviving header is not blended (spec.b4b-alternative-set.v2
//     section 5, the revised theorem).
//   - scalar (rank, lineage), retired: the pre-B4b proof dropGenericNoActionHeads
//     decided with through stage 2a. Evaluated only inside this file's
//     census now, as an ongoing regression check while more languages
//     decertify in stage 3.
//
// dropGenericNoActionHeads decides with v2 unconditionally for uncertified
// languages and evaluates all three proofs together for the census whenever
// diagnosticParserCoreShadowCensusEnabled reports true. The census never
// influences routing.
//
// Mirrors admission_census.go's pattern: gated behind an env var, cached via
// sync.Once, zero cost when off.

// diagnosticParserCoreConvergedCoverageDropsV1 proves converged-split
// no-action drops by event-only alternative-set containment
// (spec.b4b-alternative-set.v1 section 5, superseded by v2): a drop is
// admissible when one surviving header's recorded set contains, event-wise,
// every recorded member of the dropped header's set, and the dropped set is
// complete (non-empty, not overflowed, with a resolvable spill reference).
// It never walks the derivation DAG and never allocates once the shared
// spill arena has warmed.
func (s *diagnosticParserCoreGenericScheduler) diagnosticParserCoreConvergedCoverageDropsV1(
	indices []int,
) (uint64, bool) {
	var proved uint64
	for _, index := range indices {
		if index < 0 || index >= len(s.headers) {
			return 0, false
		}
		dropped := s.headers[index]
		if !dropped.convergedReductionSplit {
			continue
		}
		droppedMembers, ok := s.compact.AlternativeSetMembers(dropped.altSet)
		if !ok || len(droppedMembers) == 0 || dropped.altSet.Overflowed() {
			return 0, false
		}
		matched := false
		for survivorIndex, survivor := range s.headers {
			dropIndex := sort.SearchInts(indices, survivorIndex)
			if dropIndex < len(indices) && indices[dropIndex] == survivorIndex {
				continue
			}
			survivorMembers, ok := s.compact.AlternativeSetMembers(survivor.altSet)
			if !ok {
				continue
			}
			if alternativeSetEventOnlyContainsAll(droppedMembers, survivorMembers) {
				matched = true
				break
			}
		}
		if !matched {
			return 0, false
		}
		proved++
	}
	return proved, true
}

// diagnosticParserCoreConvergedCoverageDropsV2 proves converged-split
// no-action drops by (event, branch) alternative-set containment with the
// blended-witness veto (spec.b4b-alternative-set.v2 section 5, the revised
// theorem): a drop is admissible when one surviving, non-blended header's
// recorded set contains every recorded (event, branch) member of the
// dropped header's set exactly, and the dropped set is complete (non-empty,
// not overflowed, with a resolvable spill reference). It never walks the
// derivation DAG and never allocates once the shared spill arena has
// warmed. blendedVetoFired reports whether at least one candidate witness
// would have matched by exact-member containment but was rejected solely
// because it was blended (spec section 7 class 4, the census's veto-rate
// counter).
func (s *diagnosticParserCoreGenericScheduler) diagnosticParserCoreConvergedCoverageDropsV2(
	indices []int,
) (proved uint64, ok bool) {
	_, proved, ok, _ = s.diagnosticParserCoreConvergedCoverageDropsV2Detail(indices)
	return proved, ok
}

func (s *diagnosticParserCoreGenericScheduler) diagnosticParserCoreConvergedCoverageDropsV2Detail(
	indices []int,
) (blendedVetoFired bool, proved uint64, ok bool, overflowDeclined bool) {
	for _, index := range indices {
		if index < 0 || index >= len(s.headers) {
			return blendedVetoFired, 0, false, overflowDeclined
		}
		dropped := s.headers[index]
		if !dropped.convergedReductionSplit {
			continue
		}
		droppedMembers, memOK := s.compact.AlternativeSetMembers(dropped.altSet)
		if !memOK || len(droppedMembers) == 0 || dropped.altSet.Overflowed() {
			if dropped.altSet.Overflowed() {
				overflowDeclined = true
			}
			return blendedVetoFired, 0, false, overflowDeclined
		}
		matched := false
		blendedCandidate := false
		for survivorIndex, survivor := range s.headers {
			dropIndex := sort.SearchInts(indices, survivorIndex)
			if dropIndex < len(indices) && indices[dropIndex] == survivorIndex {
				continue
			}
			survivorMembers, survivorOK := s.compact.AlternativeSetMembers(survivor.altSet)
			if !survivorOK {
				continue
			}
			if !alternativeSetContainsAll(droppedMembers, survivorMembers) {
				continue
			}
			if survivor.blended {
				blendedCandidate = true
				continue
			}
			matched = true
			break
		}
		if !matched {
			if blendedCandidate {
				blendedVetoFired = true
			}
			return blendedVetoFired, 0, false, overflowDeclined
		}
		proved++
	}
	return blendedVetoFired, proved, true, overflowDeclined
}

// alternativeSetContainsAll reports whether every (event, branch) member of
// dropped is present in survivor, by exact packed-value match (spec.b4b-
// alternative-set.v2 section 5). Both slices are sorted ascending
// (core.AlternativeSet's invariant), so this is a single merge-scan, never
// allocating: O(len(dropped)+len(survivor)), each side bounded by
// alternativeSetHardCap.
func alternativeSetContainsAll(dropped, survivor []uint32) bool {
	if len(dropped) > len(survivor) {
		return false
	}
	survivorIndex := 0
	for _, member := range dropped {
		for survivorIndex < len(survivor) && survivor[survivorIndex] < member {
			survivorIndex++
		}
		if survivorIndex >= len(survivor) || survivor[survivorIndex] != member {
			return false
		}
		survivorIndex++
	}
	return true
}

// alternativeSetEventOnlyContainsAll reports whether every event named by
// dropped also appears, on any branch, in survivor (spec.b4b-alternative-
// set.v1 section 5, event-only identity). Both slices are sorted ascending
// event-major, branch-minor (packAlternativeSetMember's invariant), so a
// merge-scan over the event half alone still proves event-only containment:
// repeated dropped members sharing one event re-match the same survivor
// anchor without rescanning backward, and advancing survivorIndex on a
// value strictly less than the current dropped event can never skip past an
// as-yet-unconsumed equal-event survivor entry. Unlike
// alternativeSetContainsAll's exact-match merge scan, there is no valid
// len(dropped) > len(survivor) fast-fail here: two branches of one event
// (spec.b4b-alternative-set.v2 section 3.1) can make dropped's raw member
// count exceed survivor's while every dropped event is still, individually,
// present in survivor -- true v1 never observed this shape (its member was
// the bare event, deduplicated at insertion), but this predicate answers
// "what would v1's event-only identity prove about this v2-recorded
// history", which must reason about deduplicated event sets, not raw packed
// member counts.
func alternativeSetEventOnlyContainsAll(dropped, survivor []uint32) bool {
	survivorIndex := 0
	for _, member := range dropped {
		event := member & 0xFFFF0000
		for survivorIndex < len(survivor) && survivor[survivorIndex]&0xFFFF0000 < event {
			survivorIndex++
		}
		if survivorIndex >= len(survivor) || survivor[survivorIndex]&0xFFFF0000 != event {
			return false
		}
	}
	return true
}

// DiagnosticParserCoreShadowCensusTotals tallies the comparison between the
// retired scalar rank/lineage proof (diagnosticParserCoreSelectedLineageDrops)
// and the v2 alternative-set containment proof
// (diagnosticParserCoreConvergedCoverageDropsV2, the live decider since
// stage 2b) across every converged-split no-action drop election observed
// while the census is enabled. Retained under its stage 1 name for callers
// already reading it; see DiagnosticParserCoreThreeProofCensusTotals for the
// full v1/v2/scalar breakdown stage 2a adds.
//
// Deprecated: this name belongs to the compact scheduler and will be removed together with it. It has no replacement.
type DiagnosticParserCoreShadowCensusTotals struct {
	// Agree: both proofs reached the same verdict (both proved or both
	// declined).
	Agree uint64
	// OldProvedNewUnproved is a design falsifier candidate (spec.b4b-
	// alternative-set.v2 section 7): the scalar proof proved a drop v2 could
	// not. Under the theorem this is expected and safe (v2 is strictly more
	// conservative, never less), so it is not itself a stop rule; class 2
	// below is its typed name.
	OldProvedNewUnproved uint64
	// NewProvedOldUnproved is the expected, desired direction: the v2 proof
	// is strictly more capable than the scalar proof on this drop.
	NewProvedOldUnproved uint64
	// NeitherProved: both proofs declined.
	NeitherProved uint64
}

// DiagnosticParserCoreThreeProofCensusTotals tallies scalar, v1, and v2
// verdicts together across every converged-split no-action drop election
// observed while the census is enabled (spec.b4b-alternative-set.v2 section
// 7). Every election increments exactly one of ScalarProved/false and one of
// V1Proved/false and one of V2Proved/false, plus the derived classes below.
//
// Deprecated: this name belongs to the compact scheduler and will be removed together with it. It has no replacement.
type DiagnosticParserCoreThreeProofCensusTotals struct {
	// Elections is the number of converged-split no-action drop elections
	// observed (one call to dropGenericNoActionHeads with a non-empty
	// indices batch).
	Elections uint64
	// ScalarProved / V1Proved / V2Proved count how many elections each
	// proof, evaluated independently, proved.
	ScalarProved uint64
	V1Proved     uint64
	V2Proved     uint64
	// Class1V2AdmitsScalarDeclines: v2 proves where the retired scalar proof
	// declines -- the class the stage 1 census structurally could not see,
	// and the Kotlin class detector (spec section 7 class 1). The stage 2b
	// gate required this class to carry zero differing-tree cases
	// (adjudicated against the C oracle, not raw production equality;
	// campaign amendment 2026-08-02) across the corpora in scope; it stays a
	// live regression watch now that v2 is the decider.
	Class1V2AdmitsScalarDeclines uint64
	// Class2ScalarProvesV2Declines: the retired scalar proof proves where v2
	// (the live decider) declines -- expected non-zero (rank-preference
	// drops, the coincidental-premise class v1's own design indicted; spec
	// section 7 class 2). No gate; these are the newly-declined drops v2
	// falls back on.
	Class2ScalarProvesV2Declines uint64
	// Class3V1ProvesV2Declines: v1 (event-only) proves where v2 declines --
	// the branch-discrimination accepted capability cost (spec section 7
	// class 3). No gate; the Kotlin witness is the canonical member.
	Class3V1ProvesV2Declines uint64
	// BlendedVetoFirings: elections where v2 found an exact-member-
	// containing candidate witness but rejected it solely because that
	// witness was blended (spec section 7 class 4 rate; open question 2).
	BlendedVetoFirings uint64
	// OverflowDeclines: elections where v1 or v2 declined because the
	// dropped header's own recorded set had overflowed the hard cap (spec
	// section 7 class 4 rate).
	OverflowDeclines uint64
	// SpillElections / SpillObserved: elections examined, and how many of
	// them touched at least one spilled (beyond-inline) recorded set --
	// re-checks the 1-percent spill-rate gate at uint32 member width (v1
	// open question 5; spec section 7 class 4 rate).
	SpillElections uint64
	SpillObserved  uint64
	// MaxBranchOrdinalObserved: the largest branch ordinal recorded on any
	// member examined by the census (spec section 7 class 4 rate).
	MaxBranchOrdinalObserved uint16
}

// DiagnosticParserCoreShadowCensusFalsifier captures one old-proved/new-
// unproved drop context for root-causing a stop-rule hit.
//
// Deprecated: this name belongs to the compact scheduler and will be removed together with it. It has no replacement.
type DiagnosticParserCoreShadowCensusFalsifier struct {
	Detail string
}

// DiagnosticParserCoreClass1Candidate captures one v2-admits/scalar-declines
// election (spec.b4b-alternative-set.v2 section 7 class 1): the census
// records the drop and best-candidate-survivor context so a differential
// harness (or manual investigation) can re-run the owning file with v2 as
// the opt-in decider and compare the resulting tree against production and,
// per the campaign's C-oracle adjudication amendment, the locked C oracle.
//
// Deprecated: this name belongs to the compact scheduler and will be removed together with it. It has no replacement.
type DiagnosticParserCoreClass1Candidate struct {
	Detail string
}

var (
	diagnosticParserCoreShadowCensusEnabledOnce sync.Once
	diagnosticParserCoreShadowCensusEnabledVal  bool

	diagnosticParserCoreShadowCensusMu    sync.Mutex
	diagnosticParserCoreShadowCensusTotal DiagnosticParserCoreShadowCensusTotals
	diagnosticParserCoreShadowFalsifiers  []DiagnosticParserCoreShadowCensusFalsifier
	diagnosticParserCoreThreeProofTotal   DiagnosticParserCoreThreeProofCensusTotals
	diagnosticParserCoreClass1Candidates  []DiagnosticParserCoreClass1Candidate
)

// diagnosticParserCoreShadowCensusEnabled reports whether
// GTS_B4B_SHADOW_CENSUS requests the alternative-set proof comparison
// census. The result is cached after the first read (sync.Once), so every
// later drop attempt pays only one cached boolean read when the census is
// off -- the same zero-cost-when-off shape as admissionCensusEnabled
// (admission_census.go).
func diagnosticParserCoreShadowCensusEnabled() bool {
	diagnosticParserCoreShadowCensusEnabledOnce.Do(func() {
		switch os.Getenv("GTS_B4B_SHADOW_CENSUS") {
		case "1", "true", "TRUE", "True", "on", "ON", "yes", "YES":
			diagnosticParserCoreShadowCensusEnabledVal = true
		}
	})
	return diagnosticParserCoreShadowCensusEnabledVal
}

// diagnosticParserCoreRunThreeProofCensus evaluates the v1 and v2
// containment predicates next to the scalar decision dropGenericNoActionHeads
// already computed, and tallies every pairwise comparison plus the rate
// counters (spec.b4b-alternative-set.v2 section 7). Diagnostic-only: it
// never influences dropGenericNoActionHeads' actual decision. Called only
// when diagnosticParserCoreShadowCensusEnabled is true.
func (s *diagnosticParserCoreGenericScheduler) diagnosticParserCoreRunThreeProofCensus(indices []int, scalarProved bool) {
	_, v1Proved := s.diagnosticParserCoreConvergedCoverageDropsV1(indices)
	blendedVetoFired, _, v2Proved, overflowDeclined := s.diagnosticParserCoreConvergedCoverageDropsV2Detail(indices)

	diagnosticParserCoreShadowCensusMu.Lock()
	defer diagnosticParserCoreShadowCensusMu.Unlock()

	diagnosticParserCoreThreeProofTotal.Elections++
	if scalarProved {
		diagnosticParserCoreThreeProofTotal.ScalarProved++
	}
	if v1Proved {
		diagnosticParserCoreThreeProofTotal.V1Proved++
	}
	if v2Proved {
		diagnosticParserCoreThreeProofTotal.V2Proved++
	}
	if blendedVetoFired {
		diagnosticParserCoreThreeProofTotal.BlendedVetoFirings++
	}
	if overflowDeclined {
		diagnosticParserCoreThreeProofTotal.OverflowDeclines++
	}
	if !scalarProved && v2Proved {
		diagnosticParserCoreThreeProofTotal.Class1V2AdmitsScalarDeclines++
		diagnosticParserCoreClass1Candidates = append(
			diagnosticParserCoreClass1Candidates,
			DiagnosticParserCoreClass1Candidate{
				Detail: s.diagnosticParserCoreShadowCensusFalsifierDetail(indices),
			},
		)
	}
	if scalarProved && !v2Proved {
		diagnosticParserCoreThreeProofTotal.Class2ScalarProvesV2Declines++
	}
	if v1Proved && !v2Proved {
		diagnosticParserCoreThreeProofTotal.Class3V1ProvesV2Declines++
	}

	spilled, maxBranch := s.diagnosticParserCoreThreeProofSpillAndBranchSample(indices)
	diagnosticParserCoreThreeProofTotal.SpillElections++
	if spilled {
		diagnosticParserCoreThreeProofTotal.SpillObserved++
	}
	if maxBranch > diagnosticParserCoreThreeProofTotal.MaxBranchOrdinalObserved {
		diagnosticParserCoreThreeProofTotal.MaxBranchOrdinalObserved = maxBranch
	}

	// Stage 1 shadow census stays populated too (scalar vs. v2): the
	// original two-proof comparison is a strict subset of the three-proof
	// one above and keeps every existing caller of the stage 1 type working
	// unmodified.
	switch {
	case scalarProved && v2Proved:
		diagnosticParserCoreShadowCensusTotal.Agree++
	case scalarProved && !v2Proved:
		diagnosticParserCoreShadowCensusTotal.OldProvedNewUnproved++
		diagnosticParserCoreShadowFalsifiers = append(
			diagnosticParserCoreShadowFalsifiers,
			DiagnosticParserCoreShadowCensusFalsifier{Detail: s.diagnosticParserCoreShadowCensusFalsifierDetail(indices)},
		)
	case !scalarProved && v2Proved:
		diagnosticParserCoreShadowCensusTotal.NewProvedOldUnproved++
	default:
		diagnosticParserCoreShadowCensusTotal.NeitherProved++
	}
}

// diagnosticParserCoreThreeProofSpillAndBranchSample scans every header
// touched by this election (dropped and live survivor candidates) for
// spill-arena usage and the maximum recorded branch ordinal, for the rate
// counters only (spec.b4b-alternative-set.v2 section 7 class 4). Never
// influences any proof outcome.
func (s *diagnosticParserCoreGenericScheduler) diagnosticParserCoreThreeProofSpillAndBranchSample(indices []int) (spilled bool, maxBranch uint16) {
	// alternativeSetInlineSampleCapacity mirrors core.AlternativeSet's own
	// inline capacity (internal/parsercorephase0/core.go's unexported
	// alternativeSetInlineCapacity, currently 2 as of the b4b-width-repair
	// audit, 2026-08): a recorded set longer than this has spilled into the
	// shared arena. Census-only proxy; never used for a proof decision, so a
	// stale value here cannot desync any drop decision -- only this rate
	// counter (SpillObserved/SpillElections).
	const alternativeSetInlineSampleCapacity = 2
	for _, header := range s.headers {
		members, ok := s.compact.AlternativeSetMembers(header.altSet)
		if !ok {
			continue
		}
		if len(members) > alternativeSetInlineSampleCapacity {
			spilled = true
		}
		for _, member := range members {
			if branch := core.AlternativeSetMemberBranch(member); branch > maxBranch {
				maxBranch = branch
			}
		}
	}
	_ = indices // every live header is sampled, not only the dropped/survivor subset
	return spilled, maxBranch
}

// diagnosticParserCoreShadowCensusFalsifierDetail formats the exact dropped-
// header and best-candidate-survivor context for a captured falsifier or
// class-1 candidate, so a stop-rule hit or differential-harness flag can be
// root-caused without rerunning under a debugger.
func (s *diagnosticParserCoreGenericScheduler) diagnosticParserCoreShadowCensusFalsifierDetail(indices []int) string {
	var b []byte
	for _, index := range indices {
		if index < 0 || index >= len(s.headers) {
			continue
		}
		header := s.headers[index]
		members, _ := s.compact.AlternativeSetMembers(header.altSet)
		state, byteOffset, boundaryErr := s.compact.Boundary(header.head)
		b = fmt.Appendf(b, "[drop idx=%d state=%d byte=%d boundaryErr=%v rank=%v lineage=%d converged=%t blended=%t altSet=%v overflowed=%t] ",
			index, state, byteOffset, boundaryErr, header.cleanPathRank, header.cleanPathLineage,
			header.convergedReductionSplit, header.blended, members, header.altSet.Overflowed())
	}
	for survivorIndex, survivor := range s.headers {
		dropIndex := sort.SearchInts(indices, survivorIndex)
		if dropIndex < len(indices) && indices[dropIndex] == survivorIndex {
			continue
		}
		members, _ := s.compact.AlternativeSetMembers(survivor.altSet)
		b = fmt.Appendf(b, "[survivor idx=%d rank=%v lineage=%d blended=%t altSet=%v] ",
			survivorIndex, survivor.cleanPathRank, survivor.cleanPathLineage, survivor.blended, members)
	}
	return string(b)
}

// DiagnosticParserCoreShadowCensusSnapshotForTest returns the accumulated
// stage 1 (scalar vs. v2) shadow-census totals since the last reset. A
// harness resets around one language's corpus run to attribute totals to
// that language.
//
// Deprecated: this name belongs to the compact scheduler and will be removed together with it. It has no replacement.
func DiagnosticParserCoreShadowCensusSnapshotForTest() DiagnosticParserCoreShadowCensusTotals {
	diagnosticParserCoreShadowCensusMu.Lock()
	defer diagnosticParserCoreShadowCensusMu.Unlock()
	return diagnosticParserCoreShadowCensusTotal
}

// DiagnosticParserCoreThreeProofCensusSnapshotForTest returns the
// accumulated three-proof (scalar/v1/v2) census totals since the last
// reset.
//
// Deprecated: this name belongs to the compact scheduler and will be removed together with it. It has no replacement.
func DiagnosticParserCoreThreeProofCensusSnapshotForTest() DiagnosticParserCoreThreeProofCensusTotals {
	diagnosticParserCoreShadowCensusMu.Lock()
	defer diagnosticParserCoreShadowCensusMu.Unlock()
	return diagnosticParserCoreThreeProofTotal
}

// DiagnosticParserCoreShadowCensusResetForTest clears every accumulated
// census total (stage 1 and three-proof) and every captured falsifier or
// class-1 candidate.
//
// Deprecated: this name belongs to the compact scheduler and will be removed together with it. It has no replacement.
func DiagnosticParserCoreShadowCensusResetForTest() {
	diagnosticParserCoreShadowCensusMu.Lock()
	defer diagnosticParserCoreShadowCensusMu.Unlock()
	diagnosticParserCoreShadowCensusTotal = DiagnosticParserCoreShadowCensusTotals{}
	diagnosticParserCoreShadowFalsifiers = nil
	diagnosticParserCoreThreeProofTotal = DiagnosticParserCoreThreeProofCensusTotals{}
	diagnosticParserCoreClass1Candidates = nil
}

// DiagnosticParserCoreShadowCensusFalsifiersForTest returns every captured
// scalar-proved/v2-unproved drop context observed since the last reset.
// Under the theorem this direction is expected (v2 is strictly more
// conservative), so a non-empty result documents class 2, not a stop rule by
// itself; DiagnosticParserCoreClass1CandidatesForTest is the stage 2b stop
// rule's own class.
//
// Deprecated: this name belongs to the compact scheduler and will be removed together with it. It has no replacement.
func DiagnosticParserCoreShadowCensusFalsifiersForTest() []DiagnosticParserCoreShadowCensusFalsifier {
	diagnosticParserCoreShadowCensusMu.Lock()
	defer diagnosticParserCoreShadowCensusMu.Unlock()
	out := make([]DiagnosticParserCoreShadowCensusFalsifier, len(diagnosticParserCoreShadowFalsifiers))
	copy(out, diagnosticParserCoreShadowFalsifiers)
	return out
}

// DiagnosticParserCoreClass1CandidatesForTest returns every captured
// v2-admits/scalar-declines election context observed since the last reset
// (spec.b4b-alternative-set.v2 section 7 class 1). The stage 2b gate: zero
// of these may carry a differing tree from the C-oracle-adjudicated
// production route across the corpora in scope.
//
// Deprecated: this name belongs to the compact scheduler and will be removed together with it. It has no replacement.
func DiagnosticParserCoreClass1CandidatesForTest() []DiagnosticParserCoreClass1Candidate {
	diagnosticParserCoreShadowCensusMu.Lock()
	defer diagnosticParserCoreShadowCensusMu.Unlock()
	out := make([]DiagnosticParserCoreClass1Candidate, len(diagnosticParserCoreClass1Candidates))
	copy(out, diagnosticParserCoreClass1Candidates)
	return out
}

// SetDiagnosticParserCoreShadowCensusEnabledForTest overrides the
// GTS_B4B_SHADOW_CENSUS gate for one test process, mirroring
// setParserCoreReplayParseStatesForTest's override pattern
// (parsestate_replay_compact.go). Restore the previous value (the returned
// func) when done.
//
// Deprecated: this name belongs to the compact scheduler and will be removed together with it. It has no replacement.
func SetDiagnosticParserCoreShadowCensusEnabledForTest(on bool) func() {
	diagnosticParserCoreShadowCensusEnabledOnce.Do(func() {})
	previous := diagnosticParserCoreShadowCensusEnabledVal
	diagnosticParserCoreShadowCensusEnabledVal = on
	return func() { diagnosticParserCoreShadowCensusEnabledVal = previous }
}
