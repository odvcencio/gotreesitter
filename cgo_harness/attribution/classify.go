package main

import "strings"

// Component is one leaf of the campaign v7 C0 attribution tree. The set is
// mutually exclusive and collectively exhaustive: classify assigns exactly
// one component to every CPU-profile sample. See the perf-attribution record (Hyphae space m31labs/gotreesitter) for
// the prose methodology this table implements.
type Component string

const (
	ComponentSchedulerDispatch Component = "scheduler-dispatch"
	ComponentElections         Component = "elections"
	ComponentReductionsAndPops Component = "reductions-and-pops"
	ComponentCanonicalization  Component = "canonicalization"
	ComponentLexing            Component = "lexing"
	ComponentMaterialization   Component = "materialization"
	ComponentCompatTail        Component = "compat-tail"
	// ComponentRecovery is the compact-core native recovery mechanism (B3
	// campaign tranche): error cost, missing-token insertion, retry, and
	// recovery election over compact subtrees. Added in tranche B3 stage S1,
	// before any recovery engine code exists, per gate G5 ("classifier
	// first... so recovery cost can never hide in other",
	// the perf-attribution record (Hyphae space m31labs/gotreesitter)). It owns no functions yet -- compact has no
	// recovery implementation (B3 stages S2-S5 add error-cost, absorb/
	// condense-resume, election, and missing-token code class by class) -- so
	// it reads 0.0% on every clean canonical fixture today, exactly like
	// compat-tail's conditional-work components do before their trigger
	// fires. The whole-file rule for internal/parsercorephase0/recovery_cost.go
	// below (stage S2's planned file) is forward-declared now so that landing
	// it does not also require touching this table.
	ComponentRecovery Component = "recovery"
	ComponentOther    Component = "other"
)

// ComponentOrder is the fixed display and receipt order for all nine
// components.
var ComponentOrder = []Component{
	ComponentSchedulerDispatch,
	ComponentElections,
	ComponentReductionsAndPops,
	ComponentCanonicalization,
	ComponentLexing,
	ComponentMaterialization,
	ComponentCompatTail,
	ComponentRecovery,
	ComponentOther,
}

// classifyFunction maps one fully-qualified pprof function name (as written
// by the Go runtime, e.g. "github.com/odvcencio/gotreesitter.(*Type).Method")
// to a component, or returns ("", false) when the function is either a shared
// primitive (transparent -- the walk continues to its caller) or genuinely
// unrecognized (the walk keeps looking further up the stack; if nothing
// matches anywhere, the sample lands in ComponentOther).
//
// This function is the single source of truth for the attribution boundary
// rules. the perf-attribution record (Hyphae space m31labs/gotreesitter) names the primary anchors in prose; this
// table is the byte-exact implementation.
func classifyFunction(fn pbFrame) (Component, bool) {
	name := fn.Function
	file := fn.File

	// --- Shared / unclassified primitives (explicit, checked first) ---
	// These functions exist to serve more than one component (generic
	// transaction bookkeeping, checkpoint identity, or pure counters). A
	// sample landing in one of these frames is NOT attributed here -- the
	// walk continues to the next frame up the stack, which is always the
	// specific call site that decided to use the shared primitive.
	if isSharedPrimitive(name) {
		return "", false
	}

	// --- Whole-file components (a file with exactly one job) ---
	switch {
	case strings.HasSuffix(file, "/parser_dfa_token_source.go"), strings.HasSuffix(file, "/lexer.go"):
		return ComponentLexing, true
	case strings.HasSuffix(file, "/admission_switch.go"), strings.HasSuffix(file, "/admission_switch_candidate.go"):
		return ComponentCompatTail, true
	case strings.HasSuffix(file, "/parsestate_replay_compact.go"),
		strings.HasSuffix(file, "/materialization_postorder_scratch.go"),
		strings.HasSuffix(file, "/materialization_replay.go"),
		strings.HasSuffix(file, "/selected_store.go"),
		strings.HasSuffix(file, "/selected_store_builder_onepass.go"),
		strings.HasSuffix(file, "/selected_store_builder_staged.go"),
		strings.HasSuffix(file, "/phase0a_diagnostic_run.go"),
		strings.HasSuffix(file, "/phase0a_factor_provenance.go"),
		strings.HasSuffix(file, "/phase0a_identity.go"),
		strings.HasSuffix(file, "/phase0a_observer_off.go"),
		strings.HasSuffix(file, "/phase0a_routes.go"),
		strings.HasSuffix(file, "/phase0a_selected_occurrences.go"):
		// Not on the profiled diagnostic-lane or shipped-route call paths for
		// this receipt (Phase0AEnabled is false; SelectedStore is a separate
		// entry point runner.parse never calls). Classified conservatively as
		// materialization in case a shared helper is ever reached; expected
		// near-zero.
		return ComponentMaterialization, true
	case strings.HasSuffix(file, "/recovery_cost.go"):
		// B3 stage S2's file (internal/parsercorephase0/recovery_cost.go):
		// compact equivalents of cNodeErrorCostLang, cSymbolVisibleLang, and
		// cVersionStatus over immutable compact subtrees. Dead code on the
		// clean path by construction (S2's own no-outside-call-sites
		// ratchet), so this arm still reads 0.0% on every clean canonical
		// fixture; it exists so a later stage that does call into it needs
		// no further table change here.
		return ComponentRecovery, true
	case strings.HasSuffix(file, "/error_region.go"):
		// B3 stage S3's file (internal/parsercorephase0/error_region.go):
		// publishes the ERROR container and absorbed leaves a native
		// strategy-2 recovery region accumulates (ErrorRegionLeaf,
		// ErrorRegionResume). Reachable only from the driver-side S3 helpers
		// below, themselves reachable only when a genuine no-table-action
		// point is hit under an admitted, certified recovery region -- never
		// on a clean parse.
		return ComponentRecovery, true
	}

	// --- Explicit function-name overrides (the two mixed files: the driver's
	// scheduler and the core package's transactional structures, plus a
	// handful of specific cross-file exceptions). Matching is by Contains
	// against the full pprof symbol (package path + receiver + method), using
	// the longest matching entry so a more specific override (e.g. an
	// "...Owned" variant) beats a shorter one that happens to also match.
	if component, ok := lookupFunctionOverride(name); ok {
		return component, true
	}

	return "", false
}

func lookupFunctionOverride(name string) (Component, bool) {
	bestLen := -1
	var best Component
	found := false
	for _, entry := range functionOverrideTable {
		if strings.Contains(name, entry.substr) && len(entry.substr) > bestLen {
			bestLen = len(entry.substr)
			best = entry.component
			found = true
		}
	}
	return best, found
}

// isSharedPrimitive reports whether name is a generic building block reused
// by more than one component, so a sample resting in its own frames should be
// attributed to whichever specific caller reached it instead.
//
// Worked examples (see the perf-attribution record (Hyphae space m31labs/gotreesitter)):
//   - condense / condenseWithOutcome / condenseWithOutcomeAtomic run from both
//     Shift (scheduler-dispatch) and reduction-output application
//     (reductions-and-pops); the walk finds the caller's frame instead.
//   - ApplySchedulerAtomic / RunSchedulerOwned wrap shift, reduce, conflict,
//     and extra-shift appliers alike; same rule.
//   - checkpoint interning and external-scanner-state capture run both nested
//     inside dfaTokenSource.Next (lexing) and directly from elect() for
//     checkpoint-continuity proof (elections); the walk finds whichever is
//     the nearer ancestor on that particular stack.
func isSharedPrimitive(name string) bool {
	for _, prefix := range sharedPrimitivePrefixes {
		if strings.Contains(name, prefix) {
			return true
		}
	}
	return false
}

// sharedPrimitivePrefixes match on unqualified method/function names (using
// Contains against the full symbol, which already includes the package and
// receiver) so one entry covers a method regardless of receiver formatting.
var sharedPrimitivePrefixes = []string{
	// Scheduler transaction bookkeeping (core.go): wraps shift, reduce,
	// conflict, and extra-shift appliers uniformly.
	").ApplySchedulerAtomic",
	").RunSchedulerOwned",
	").RunFreshSchedulerSession",
	").ApplyAtomic",
	").markInto",
	").mark(",
	").restoreCheckpoint",
	").restore(",
	").commit(",
	").assertTopTransaction",
	").finishTransaction",
	").completeTransaction",
	").validateSchedulerTransaction",
	").poisonSchedulerTransaction",
	").runSchedulerMaybeLiveScopedOwned",
	// GSS graph condense/merge: shared by shift and reduce application.
	").condense(",
	").condenseWithOutcome(",
	").condenseWithOutcomeAtomic",
	").condenseInputExists",
	// Checkpoint identity: shared by lexing-nested capture (inside Next) and
	// direct election-time capture (elect's continuity proof).
	"diagnosticParserCoreInternCheckpoint",
	").InternCheckpoint",
	").CheckpointReceipt",
	").CheckpointInternerStats",
	"checkpointInterner)",
	"parserCoreCheckpoint(",
	"diagnosticParserCoreCheckpointDigest",
	// Boundary hash index: shared by shift's and condense's boundary lookups.
	"boundaryIndex)",
	"boundaryIdentityFromKey",
	"boundaryIdentityHash",
	// Pure bookkeeping/counters: negligible cost, no domain meaning of their
	// own, invoked from every component.
	").Work(",
	").addWork(",
	").recordLinkUnion",
	").publishTotals",
	// Arena/session construction: once per parse, outside any one dispatch
	// action; New/Reset overlap the fixed per-parse entry cost that this
	// receipt intentionally treats as unattributed rather than guessing which
	// downstream component "owns" allocator setup.
	").Reset(",
	"parsercorephase0.New(",
}

// functionOverrideTable lists specific functions in the two mixed-purpose
// files (parsercore_phase0_driver.go in the root package, and core.go /
// scheduler_owned.go in internal/parsercorephase0) plus a small number of
// named cross-file exceptions. Matching is by Contains against the pprof
// function symbol (see lookupFunctionOverride), so receiver formatting
// differences do not matter.
type functionOverrideEntry struct {
	substr    string
	component Component
}

var functionOverrideTable = buildFunctionOverride()

func buildFunctionOverride() []functionOverrideEntry {
	var table []functionOverrideEntry
	add := func(component Component, names ...string) {
		for _, n := range names {
			table = append(table, functionOverrideEntry{substr: n, component: component})
		}
	}

	add(ComponentSchedulerDispatch,
		"GenericScheduler).run",
		"GenericScheduler).dispatchPass",
		"GenericScheduler).dispatchPassActive",
		"GenericScheduler).zeroWidthExtraShiftWithoutProgress",
		"GenericScheduler).applyGenericAccept",
		"GenericScheduler).applyGenericShifts",
		"GenericScheduler).applyGenericShiftsOwned",
		"GenericScheduler).applyGenericExtraShifts",
		"GenericScheduler).applyGenericExtraShiftsOwned",
		"GenericScheduler).genericExternalStats",
		"GenericScheduler).recordGenericExternalShift",
		"GenericScheduler).relexTokenForState",
		"diagnosticParserCoreSameSpanRelex",
		"diagnosticParserCoreRaggedRelexDeclineDetailFor",
		"GenericScheduler).validateGenericNoLookaheadReduction",
		"diagnosticParserCoreGenericUnsupportedCell",
		"diagnosticParserCoreGenericUnsupportedToken",
		"GenericScheduler).reserveDispatches",
		"GenericScheduler).finish",
		"GenericScheduler).completeAtClosedByte",
		"GenericScheduler).headerReceipt",
		"GenericScheduler).fullReceipts",
		"newDiagnosticParserCoreGenericScheduler",
		"initializeDiagnosticParserCoreGenericScheduler",
		"executeDiagnosticParserCoreGenericSchedulerFromSeed",
		"DispatchScratch).begin",
		"DispatchScratch).finish",
		"HeaderRollbackScratch).begin",
		"HeaderRollbackScratch).finish",
		"HeaderRollbackScratch).reset",
		// core.go / scheduler_owned.go: shift execution (GSS append for a
		// shifted token) is scheduler-dispatch, not a reduction.
		"Core).Shift(",
		"Core).ShiftClassified",
		"Core).ShiftOrdinaryCohort",
		"Core).ShiftOrdinaryClassifiedCohort",
		"Core).ShiftExtraCohort",
		"Core).ShiftExtraClassifiedCohort",
		"Core).Actions(",
		"Core).ClassifyBoundary",
		"validateClassification",
		"classifiedActionRef",
		"Core).appendDiagnosticPayload",
		"Core).Seed(",
		"Core).Boundary(",
		"Core).BeginFrontier",
		"Core).SetPhaseCheckpoint",
		"Core).SetPhaseExternalTokenScannerCheckpoints",
		"cohortHasDuplicateHead",
		"Core).cohortHeads",
		"Core).prepareOrdinaryClassifiedCohortInto",
		"Core).prepareExtraClassifiedCohortInto",
		"Core).shiftClassifiedMaybeLiveScopedOwned",
		"Core).shiftClassifiedUncheckpointed",
		"Core).shiftOrdinaryClassifiedCohortMaybeLiveScopedOwned",
		"Core).shiftOrdinaryClassifiedCohortUncheckpointed",
		"Core).shiftExtraClassifiedCohortMaybeLiveScopedOwned",
		"Core).shiftExtraClassifiedCohortUncheckpointed",
		"ShiftClassifiedOwned",
		"ShiftClassifiedWithLiveCondenseCandidatesOwned",
		"ShiftOrdinaryClassifiedCohortOwned",
		"ShiftOrdinaryClassifiedCohortWithLiveCondenseCandidatesOwned",
		"ShiftExtraClassifiedCohortOwned",
		"ShiftExtraClassifiedCohortWithLiveCondenseCandidatesOwned",
		// runner-level scheduler entry and its accepted-tail cleanliness
		// check (distinct from the compat-tail component -- see doc).
		"FreshFullRunner).executeSchedulerOpen",
		"requireParserCoreFreshFullAcceptance",
		"parserCoreFreshFullAcceptedTailIsClean",
		"parserTailAllowsCleanAcceptance",
		"FreshFullRunner).parse(",
		"FreshFullRunner).parseSelectedStore",
	)

	add(ComponentElections,
		"GenericScheduler).elect",
		"executeDiagnosticParserCoreGenericConflictDetailed",
		"GenericScheduler).applyGenericConflict",
		"GenericScheduler).applyGenericConflictOwned",
		"GenericScheduler).reconcileGenericConflictOutputs",
		"ConflictScratch).begin",
		"ConflictScratch).finish",
		"ConflictExecution).arm",
		"diagnosticParserCoreConflictPolicyOrdinal",
		"diagnosticParserCoreRepetitionFoldOrdinal",
		"diagnosticParserCoreSingleReduceRepetitionShiftOrdinal",
		"validateDiagnosticParserCoreCell",
	)

	add(ComponentReductionsAndPops,
		"GenericScheduler).applyGenericReduction",
		"GenericScheduler).applyGenericReductionOwned",
		"GenericScheduler).dropGenericNoActionHeads",
		"diagnosticParserCoreGenericNoActionDropEligible",
		"diagnosticParserCoreSelectedLineageDrops",
		"GenericScheduler).adoptUpdatedReductionSibling",
		"GenericScheduler).collectCondenseCandidates",
		"GenericScheduler).reindexCondenseCandidatesOwned",
		"Core).Reduce(",
		"Core).ReduceOutputs",
		"reductionParentForPath",
		"Core).reductionPlan",
		"remapReductionPlan",
		"Core).popPaths",
		"Core).popSingleLinkPath",
		"popEnumerationScratch)",
		"Core).findReductionBatchParent",
		"reductionParentIdentityEqual",
		"Core).appendPrivate",
		"Core).factorExactPredecessor",
		"factorExactPredecessorMerge",
		"Core).mergePredecessorsBounded",
		"Core).insertLinkBounded",
		"Core).replaceBoundaryLink",
		"Core).linkEqualInput",
		"Core).historicalForestIsDeterministic",
		"Core).graphVersionIsDeterministic",
		"Core).shallowPayloadClassEqual",
		"Core).effectivePayloadPrecedence",
		"Core).subtreeExternalProvenance",
		"Core).externalPayloadScannerProvenance",
		"Core).predecessorBoundariesMatch",
		"Core).nodeScannerCheckpoint",
		"Core).shallowPayloadsEqual",
		"Core).linkEdgeEqualInput",
		"Core).linkEdgesEqual",
		"Core).linkRecordsEqual",
		"Core).nodesAncestryRelated",
		"Core).nodeReaches",
		"Core).appendAdjacencyNode",
		"Core).subtreesStructurallyEqual",
		"Core).shallowPayloadClass",
		"Core).walkCleanPrefixRanks",
		"Core).markCleanProductionRank",
		"markCleanPathRankUnknown",
		"cleanPathRankAccumulator).observe",
		"compareCleanPathRank",
		"mergeCleanPathRank",
		"Core).cleanPathPayloadHasExternal",
		"Core).appendNode",
		"validatePublishedNodeDAG",
		"Core).appendSubtree",
		"Core).appendAuthenticatedTerminal",
		"Core).appendSubtreeRecord",
		"Core).nodeLinks",
		"Core).publishedNodeLinksInto",
		"Core).boundaryKey",
		"Core).shiftedBoundaryKey",
		"Core).CanonicalBoundary",
		"Core).condenseNodeIsLive",
		"Core).node(",
		"Core).nodeLineage",
		"Core).subtree(",
		"reductionOutputScratch)",
		"Core).reductionPlanForPair",
		// scheduler_owned.go reduce-side and condense-candidate core methods.
		"Core).reduceOutputsClassifiedIntoActive",
		"Core).reduceOutputsClassifiedIntoUncheckpointed",
		"Core).reduceOutputsClassifiedIntoMaybeLiveScopedOwned",
		"ReduceOutputsClassifiedIntoOwned",
		"ReduceOutputsClassifiedIntoWithLiveCondenseCandidatesOwned",
		"Core).runLiveCondenseCandidates",
		"Core).clearLiveCondenseCandidates",
		"Core).reindexCondenseCandidatesUncheckpointed",
		"ReindexCondenseCandidatesOwned",
	)

	add(ComponentCanonicalization,
		"GenericScheduler).canonicalize",
		"canonicalizeDiagnosticParserCoreHeaders",
		"CanonicalScratch).canonicalize",
		"CanonicalScratch).canonicalizeLinear",
		"CanonicalScratch).canonicalizeMapped",
		"diagnosticParserCoreCanonicalCandidateWins",
		"mergeDiagnosticParserCoreCleanPathLineage",
		"nextDiagnosticParserCoreCleanPathLineage",
		"applyDiagnosticParserCoreCleanPathOutput",
		"markDiagnosticParserCoreExternalLineage",
		"GenericScheduler).persistHeaderLineageOwned",
		"replaceDiagnosticParserCoreHeader",
		// scheduler_owned.go lineage/rank persistence backing canonicalize's
		// tie-breaking: same intrinsic job regardless of caller.
		"RecordReductionLineageOwned",
		"RecordHeadLineageOwned",
		"Core).recordNodeLineage",
		"RecordHeadOwnerOwned",
	)

	add(ComponentMaterialization,
		"GenericScheduler).completeAcceptance",
		"selectCompactAcceptanceDerivation",
		"compactDerivationsForAcceptance",
		"Core).Derivations",
		"saturatingAddPaths",
		"Core).Stats(",
		"materializeDiagnosticParserCoreAcceptedTree",
		"materializeDiagnosticParserCoreAcceptedSelection",
		"finalizeDiagnosticParserCoreAcceptedRootSpan",
		"Parser).replayCompactDerivation",
		"newDiagnosticParserCorePointIndex",
		"PointIndex).point",
		"parserCoreNodeSlice",
		"clearNodeScratch",
		"RunnerScratch).resetTreeBuffers",
		"withDiagnosticParserCoreMaterializationScratch",
		"withProvidedMaterializationScratch",
		"MaterializationScratch).entriesFor",
		"MaterializationScratch).reset",
		"Core).Subtree(",
		"Core).MaterializationOrder",
		"Core).VisitMaterializationPostorder",
		"Core).MaterializationView",
		"validateMaterializationMetadata",
		"validateGenericMaterializationMetadata",
		"Core).SubtreeArenaLen",
		"diagnosticParserCoreSelectedNodeCensus",
		"FreshFullRunner).materialize",
		"FreshFullRunner).materializeSelection",
		"Core).RawSelectedSubtreeCensus",
	)

	add(ComponentCompatTail,
		"Parser).normalizeReturnedTreeForParse",
		"Parser).resolveCRecoverySwallowedError",
		"Parser).maybeCompactReturnedFullTree",
		"Parser).tryCompactFullParseRoute",
		"Parser).attemptAdmissionCandidateFullParse",
		"Parser).admissionCandidateFullParseEligible",
		"Parser).acquireAdmissionCandidateRunner",
		"newAdmissionCandidateRunner",
		"Parser).hasActiveParseObservability",
		"admissionCandidateDeclineReason",
	)

	add(ComponentRecovery,
		// B3 stage S3: native strategy-2 recovery (error-region absorb and
		// condense-resume) driver-side helpers, parsercore_phase0_driver.go.
		// Reachable only from dispatchPassActive's s3Region branch and its
		// no-table-action decline branch, both gated on
		// s3ErrorRegionAdmitted (Recovery && allowCompactStrategy2ErrorRegion,
		// itself gated on the certified html grammar blob) -- every call
		// under this override is, by construction, work a clean parse or an
		// uncertified grammar never performs.
		"GenericScheduler).s3ErrorRegionAdmitted",
		"s3TokenIsExtraShift",
		"GenericScheduler).s3ErrorModeRelex",
		"s3RegionResumeAction",
		"GenericScheduler).s3CloseInProgressProductions",
		"GenericScheduler).s3MissingTokenOpportunityExists",
		"GenericScheduler).s3TryOpenErrorRegion",
		// The runner-level mirror of s3ErrorRegionAdmitted that
		// materialization consults (parsercore_phase0_fresh_full_runner.go);
		// same gate, same guarantee.
		"FreshFullRunner).s3AllowErrorRoot",
		// Adversarial-review revision (REQUIRED 1/2): the root leaf-coverage
		// audit only runs inside finalizeDiagnosticParserCoreAcceptedRootSpan's
		// allowErrorRoot branch, itself gated the same way as s3AllowErrorRoot
		// above; the deeper-resume existence probe only runs from the two S3
		// call sites listed above. Both are unreachable work for a clean
		// parse or an uncertified grammar, same as every other entry here.
		"diagnosticParserCoreAcceptedTreeLeafCoverageGap",
		"Core).AncestorStateWithActionExists",
	)

	return table
}
