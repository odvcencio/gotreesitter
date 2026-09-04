//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"sort"
	"unsafe"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

type DiagnosticParserCoreBoundaryKind string

const (
	DiagnosticParserCoreExtra      DiagnosticParserCoreBoundaryKind = "extra"
	DiagnosticParserCoreExtraChain DiagnosticParserCoreBoundaryKind = "extra_chain"
	DiagnosticParserCoreNoAction   DiagnosticParserCoreBoundaryKind = "no_action"
	// DiagnosticParserCoreRecovery marks every dispatch shape where only
	// locked-C production's recovery semantics can continue: an explicit
	// ActionRecover cell, an unexpected recover action inside a generic
	// conflict, and (B3 stage S1) the pure no-table-action frontier that
	// mirrors C's cPaused trigger (glr.go: "the stack hit a no-action
	// point"). Dispatch classification only -- every one of these shapes
	// still declines and falls back to production unchanged.
	DiagnosticParserCoreRecovery      DiagnosticParserCoreBoundaryKind = "recovery"
	DiagnosticParserCoreAccept        DiagnosticParserCoreBoundaryKind = "accept_without_materialization"
	DiagnosticParserCoreCap           DiagnosticParserCoreBoundaryKind = "cap"
	DiagnosticParserCoreIdentity      DiagnosticParserCoreBoundaryKind = "identity"
	DiagnosticParserCoreRoute         DiagnosticParserCoreBoundaryKind = "unsupported_route"
	DiagnosticParserCoreGenericClosed DiagnosticParserCoreBoundaryKind = "generic_scheduler_closed"
)

// DiagnosticParserCoreReceiptMode controls diagnostic observation only. It
// never changes parser-core scheduling or selection semantics. The zero value
// preserves the complete historical receipt; summary mode retains only the
// authenticated result and aggregate work needed for larger-fixture study.
type DiagnosticParserCoreReceiptMode uint8

const (
	DiagnosticParserCoreReceiptFull DiagnosticParserCoreReceiptMode = iota
	DiagnosticParserCoreReceiptSummary
)

type DiagnosticParserCorePrefixOptions struct {
	Recovery       bool
	Retry          bool
	Incremental    bool
	IncludedRanges bool
	// GenericStopAtClosedByte publishes a successful closed-frontier receipt
	// when every authenticated scheduler head closes at this byte. Nil is
	// unbounded. The boundary is checked before another scanner election.
	GenericStopAtClosedByte *uint32
	ReceiptMode             DiagnosticParserCoreReceiptMode
	// DisablePerHeaderSpanUnlockedRelex restores relexTokenForState's
	// pre-D2-1 span-locked probe (only an exact-span relex is eligible; a
	// relex whose EndByte differs from the shared election is declined the
	// same way a scan failure is, never routed through owned lexer
	// activation). The zero value keeps the span-unlocked probe on. This is a
	// test-and-diagnostic lever only: no production caller sets it, and it
	// has no operator-facing surface (config flag, CLI flag, or similar).
	DisablePerHeaderSpanUnlockedRelex bool
	// recordDropCohortCertificates keeps the Slice C certificate stores inert
	// until a focused authentic producer test enables the later activation.
	recordDropCohortCertificates bool
	// recordDropCohortFrontiers enables only the D6a frontier producer. It does
	// not enable historical certificate authentication or admission behavior.
	recordDropCohortFrontiers bool
	// verifyDropCohortFrontiers enables the D6b consumer only for a private
	// focused verifier run. The default remains false.
	verifyDropCohortFrontiers bool
	MaxDispatches             uint64
	MaxTokens                 uint64
	Limits                    core.Limits
	// freshSchedulerSession is set only by the reusable fresh-full runner,
	// which resets before every call and never exposes a declined core.
	freshSchedulerSession bool
	// These acceptance options require exact artifact certification. Diagnostic
	// callers retain the conservative false defaults.
	allowEOFAcceptNoActionSiblings bool
	allowMetadataEOFAcceptRecovery bool
	allowPrimaryAcceptDerivation   bool
	// allowCompactAcceptanceStructuralElection permits C's raw subtree
	// comparator for a clean, tied acceptance frontier. The admission route
	// binds it only from an exact grammar-artifact capability.
	allowCompactAcceptanceStructuralElection bool
	allowConvergedSplitDropArtifact          bool
	// captureLexerSkippedPrefixProvenance preserves exact DFA evidence only
	// for an artifact that consumes it during accepted-tree tiling.
	captureLexerSkippedPrefixProvenance bool
	// allowCompactStrategy2ErrorRegion permits the generic scheduler to
	// attempt native S3 recovery (error-region absorb and condense-resume)
	// at a true no-table-action point instead of declining. Set only from
	// Language.CompactStrategy2ErrorRegionCertified (grammar-blob-keyed, not
	// name-keyed -- design section 7). Recovery must also be true: this
	// option alone does not admit the fresh-full runner's recovery guard.
	allowCompactStrategy2ErrorRegion bool
	// allowCompactRecoverEOF permits one exact, authenticated EOF no-action
	// lineage to publish the locked-C recover_eof ERROR root. It is separate
	// from strategy-2 recovery and never enables S3, S4, or S5.
	allowCompactRecoverEOF bool
	// allowCompactStackSummaryRecovery permits the scheduler to fork one
	// no-action head into ancestor-recovered and error-absorb lineages.
	// Language.CompactStackSummaryRecoveryCertified is its artifact gate.
	allowCompactStackSummaryRecovery bool
	// allowCompactMissingTokenInsertion permits the scheduler to fork one
	// no-action head into missing-token and error-absorb recovery lineages.
	// Language.CompactMissingTokenInsertionCertified is its artifact gate.
	allowCompactMissingTokenInsertion bool
	// allowCompactFaithfulS5Recovery selects the complete S5 physical
	// graph-head scan. Uncertified artifacts retain the bounded legacy scan.
	allowCompactFaithfulS5Recovery bool
	// allowCompactRecoveryLineageSelection permits completeAcceptance to
	// resolve MORE THAN ONE accepted head by pricing the competing recovery
	// lineages and publishing the cheaper tree, instead of declining.
	//
	// This is the acceptance half of C's version competition. C does not pick
	// between a missing-token insertion and an error-region absorb where they
	// diverge; it carries both versions to the end and keeps the cheaper tree
	// (ts_parser__select_tree). A compact route that requires a sole accepted
	// head cannot express that outcome at all.
	//
	// The admission route sets this only when one artifact certifies strategy 2
	// and at least one competing recovery mechanism. Other parses retain the
	// conservative false default.
	allowCompactRecoveryLineageSelection bool
	// allowCompactRecoveryTrailingLineageRetirement permits C's condense-tail
	// removal for one exact two-version S5 frontier. The admission route sets
	// this only from an exact grammar-artifact capability.
	allowCompactRecoveryTrailingLineageRetirement bool
	// allowCompactRecoveryErrorModeKeywordCapture permits s3ErrorModeRelex to
	// run the keyword lexer in ERROR_STATE after the main lexer returns the
	// grammar's keyword-capture token. The admission route sets this only from
	// an exact grammar-artifact capability.
	allowCompactRecoveryErrorModeKeywordCapture bool
	noLookaheadRootSymbol                       Symbol
	hasNoLookaheadRootSymbol                    bool
	// stopControlParser, when non-nil, arms the scheduler's stop-control poll
	// (spec.campaign.v7 tranche B8): once per dispatch-pass-loop iteration,
	// diagnosticParserCoreGenericScheduler.run checks this Parser's deadline
	// (timeoutMicros) and cancellation flag through the exact production
	// check (activeParseStopReason), plus the deterministic compact-arena
	// memory budget below. Nil (the default for every diagnostic and
	// benchmark caller) disables the whole poll, matching prior behavior
	// byte-for-byte -- only the admission-candidate route sets this field.
	stopControlParser *Parser
	// stopControlMemoryBudgetBytes is the production engine's own soft
	// per-parse byte budget (parseMemoryBudgetForParser), recomputed from the
	// caller's source length immediately before each scheduler run. Zero
	// disables the memory-budget half of the poll, mirroring production's
	// own "budget disabled" contract (parseMemoryBudget's mb<=0 case).
	stopControlMemoryBudgetBytes int64
	// stopControlHardCeilingBytes is production's own absolute, decoupled
	// hard ceiling (parseMemoryHardCeilingBytes, parser_config.go),
	// independent of the soft budget above: it stays armed even when a
	// caller explicitly disables the soft budget (GOT_PARSE_MEMORY_BUDGET_MB=0),
	// the same independence production's own runtime-heap watchdog keeps
	// (runtimeMemoryHardCeilingEnabled, parser_memory_budget_runtime.go).
	// Without this the candidate route had no backstop at all when a caller
	// zeroed the soft budget (tranche B9 honest-accounting gate). Zero
	// disables it, mirroring the production contract's own 0=off escape
	// hatch.
	stopControlHardCeilingBytes int64
	// materializationParser, materializationSource,
	// materializationForceReplayParseStates, and materializationContextSet
	// back the compact acceptance-election materiality gate
	// (completeAcceptance -> compactAcceptanceElectionIsVacuous): when a
	// certified primary derivation election has more than one live
	// candidate, the gate re-materializes every candidate through the same
	// public-tree pipeline the accepted parse itself uses, so it can require
	// every candidate's tree to match the primary before admitting it.
	//
	// materializationForceReplayParseStates must match the value the
	// caller's own post-acceptance materialization will use, not just be
	// "some" value: it decides whether replayCompactDerivation stamps
	// ParseState/PreGotoState on the materialized nodes at all, and the
	// gate's comparator reads those stamps. A mismatched value would compare
	// an attribute the published tree does not actually carry.
	//
	// All fields are set by the two production seed entry points
	// (DiagnosticParseParserCorePrefix, parserCoreFreshFullRunner's
	// executeSchedulerOpen) from state those callers already hold:
	// DiagnosticParseParserCorePrefix always forces false (its own
	// materialization call does the same); executeSchedulerOpen forwards the
	// runner's own replayParseStates field, which is true only for the
	// admission-candidate route (admission_switch_candidate.go).
	//
	// materializationContextSet distinguishes "no caller has set this
	// context" (fail closed: the gate cannot materialize at all) from "the
	// caller set it, and the source legitimately empty" (a zero-length
	// source is a normal, if degenerate, parse input) -- len(source) == 0
	// alone cannot tell those apart.
	materializationParser                 *Parser
	materializationSource                 []byte
	materializationForceReplayParseStates bool
	materializationContextSet             bool
}

func diagnosticParserCoreLexerSkippedPrefixLength(token Token, capture bool) uint16 {
	if !capture || !token.lexerSkippedPrefix || token.lexerSkippedPrefixStart >= token.StartByte {
		return 0
	}
	length := token.StartByte - token.lexerSkippedPrefixStart
	if length > math.MaxUint16 {
		return 0
	}
	return uint16(length)
}

type compactEOFRecoveryAdmissionState uint8

const (
	compactEOFRecoveryAdmissionEmpty compactEOFRecoveryAdmissionState = iota
	compactEOFRecoveryAdmissionProduced
	compactEOFRecoveryAdmissionPreApplyValidated
	compactEOFRecoveryAdmissionAcceptApplied
	compactEOFRecoveryAdmissionRecoveryDropped
	compactEOFRecoveryAdmissionCompleted
	compactEOFRecoveryAdmissionSchedulerReturned
	compactEOFRecoveryAdmissionConsumed
	compactEOFRecoveryAdmissionInvalid compactEOFRecoveryAdmissionState = 255
)

type compactEOFRecoveryAdmissionEventKind uint8

const (
	compactEOFRecoveryAdmissionEventNormal compactEOFRecoveryAdmissionEventKind = iota + 1
	compactEOFRecoveryAdmissionEventRecover
)

type compactEOFRecoveryAdmissionRoute uint8

const (
	compactEOFRecoveryAdmissionRoutePublicTree compactEOFRecoveryAdmissionRoute = iota
	compactEOFRecoveryAdmissionRouteSelectedStore
	compactEOFRecoveryAdmissionRouteNone compactEOFRecoveryAdmissionRoute = 255
)

type compactEOFRecoveryAdmissionEvent struct {
	ordinal           int
	kind              compactEOFRecoveryAdmissionEventKind
	cost              uint32
	dynamicPrecedence int64
}

type compactEOFRecoveryAdmissionWork struct {
	polls                      uint64
	sourceChunks               uint64
	childGroups                uint64
	pathsVisited               uint64
	linksVisited               uint64
	payloadRecordsVisited      uint64
	maxDepth                   uint64
	bytesInspected             uint64
	maxSourceChunk             uint64
	maxChildGroup              uint64
	checkedArithmetic          uint64
	publicationAttempts        uint64
	parserConstructions        uint64
	treeConstructions          uint64
	selectedStoreConstructions uint64
	overflow                   bool
}

type compactEOFRecoveryAdmissionReceipt struct {
	active              bool
	valid               bool
	state               compactEOFRecoveryAdmissionState
	seal                [32]byte
	transitionSeals     [7][32]byte
	transitions         [7]compactEOFRecoveryAdmissionState
	transitionCount     uint8
	declineReason       string
	coreGeneration      uint64
	electionIndex       int
	token               Token
	sourceLength        uint32
	sourceSHA256        [32]byte
	normalHead          core.Head
	recoveryHead        core.Head
	normalCreationSeq   uint64
	recoveryCreationSeq uint64
	normalLineage       uint16
	recoveryLineage     uint16
	normalFingerprint   [32]byte
	recoveryFingerprint [32]byte
	normalPayloads      uint32
	recoveryPayloads    uint32
	normalOccurrences   uint32
	recoveryOccurrences uint32
	normalFrontier      uint32
	recoveryFrontier    uint32
	events              [2]compactEOFRecoveryAdmissionEvent
	selectedEvent       int
	metadataOnly        bool
	consumptionCount    uint64
	constructionRoute   compactEOFRecoveryAdmissionRoute
	observedErrorCost   uint32
	work                compactEOFRecoveryAdmissionWork
}

var (
	compactEOFRecoveryAdmissionFaultHook    func(*diagnosticParserCoreGenericScheduler, string) error
	compactEOFRecoveryAdmissionOverflowHook func(*diagnosticParserCoreGenericScheduler, string) bool
	compactEOFRecoveryAdmissionCensusHook   func(compactEOFRecoveryAdmissionReceipt)
)

type DiagnosticParserCoreScannerCheckpoint struct {
	Length int
	SHA256 [32]byte
}

type DiagnosticParserCoreElection struct {
	States                 []StateID
	Token                  Token
	ScannerBefore          DiagnosticParserCoreScannerCheckpoint
	ScannerAfter           DiagnosticParserCoreScannerCheckpoint
	CurrentCheckpointValid bool
	CurrentCheckpointStart DiagnosticParserCoreScannerCheckpoint
	CurrentCheckpointEnd   DiagnosticParserCoreScannerCheckpoint
	CurrentCheckpointBytes [2]uint32
}

// DiagnosticParserCoreVersionLexerRequest records one full owned lexer
// request. The pair is the scanner state before and after the token, not a
// cursor-only approximation. Full receipts retain these records so a caller
// can audit straddled token spans and their authenticated checkpoints.
type DiagnosticParserCoreVersionLexerRequest struct {
	ElectionIndex     int
	HeaderCreationSeq uint64
	State             StateID
	Token             Token
	InternalDFAToken  bool
	ScannerBefore     DiagnosticParserCoreScannerCheckpoint
	ScannerAfter      DiagnosticParserCoreScannerCheckpoint
}

type DiagnosticParserCoreHeaderReceipt struct {
	CreationSeq uint64
	State       StateID
	ByteOffset  uint32
	Shifted     bool
	Accepted    bool
	Paused      bool
	ExactPaths  uint64
	Checkpoint  [32]byte
}

type DiagnosticParserCoreRoundAction struct {
	HeaderIndex int
	State       StateID
	ByteOffset  uint32
	Ordinal     int
	Action      ParseAction
	BranchOrder uint64
}

type DiagnosticParserCoreDispatchRound struct {
	Index   int
	Before  []DiagnosticParserCoreHeaderReceipt
	Actions []DiagnosticParserCoreRoundAction
	After   []DiagnosticParserCoreHeaderReceipt
}

type DiagnosticParserCorePackedDerivation struct {
	Score          int64
	BranchOrder    uint64
	HasBranchOrder bool
}

type DiagnosticParserCoreTerminalPayloadView struct {
	ID                uint32
	Symbol            Symbol
	ProductionID      uint16
	DynamicPrecedence int16
	StartByte         uint32
	EndByte           uint32
	Children          []uint32
	Fields            []FieldMapEntry
	Aliases           []Symbol
	Extra             bool
	External          bool
	Terminal          bool
}

type DiagnosticParserCoreHeaderPathReceipt struct {
	Header               DiagnosticParserCoreHeaderReceipt
	Derivations          []DiagnosticParserCorePackedDerivation
	DerivationsTruncated bool
}

// DiagnosticParserCoreGenericWork records semantic scheduler work separately
// from the compact core's physical arena storage.
type DiagnosticParserCoreGenericWork struct {
	Passes uint64
	// PotentialReductionActions counts C-style any-terminal reduction actions
	// examined by the staged S5 frontier.
	PotentialReductionActions uint64
	// PotentialReductionOutputs counts physical outputs returned by staged S5
	// reduction actions.
	PotentialReductionOutputs uint64
	// ReductionPromotions counts outputs promoted into an earlier local source
	// slot after a positive-child reduction.
	ReductionPromotions uint64
	// MissingTokenTrials counts viable missing-terminal trial candidates.
	MissingTokenTrials uint64
	// MissingTokenCommits counts committed missing-terminal versions.
	MissingTokenCommits uint64
	// RecoveryDiscontinuityMerges counts committed recovery-head merges.
	RecoveryDiscontinuityMerges uint64
	// RecoveryCeilingDeclines counts staged recovery searches that hit a
	// configured ceiling and then declined.
	RecoveryCeilingDeclines uint64
	// StackSummaryRecoveryForks counts S4 forks that retain both the
	// ancestor-recovered lineage and the error-absorb lineage.
	StackSummaryRecoveryForks uint64
	// RecoveryLineageSelections counts accepting frontiers resolved by pricing
	// competing recovery lineages instead of declining. Every other
	// head-removal site in this scheduler accounts for itself; without this
	// counter a parse that arbitrated three competitors is indistinguishable
	// in the receipt from one that never competed, and no differential harness
	// could confirm the port picks what C picks.
	RecoveryLineageSelections uint64
	// RecoveryLineageRetirements counts C-condense-tail transitions that remove
	// one trailing no-action recovery version after an earlier version shifts.
	RecoveryLineageRetirements uint64
	// RecoveryAmbiguityForks counts ordinary grammar forks whose source already
	// belongs to a recovery competition. The outputs retain recovery cost.
	RecoveryAmbiguityForks uint64
	// RecoveryCondensePasses counts completed C-style recovery condensations.
	RecoveryCondensePasses uint64
	// RecoveryVersionCapDrops counts histories removed because their distinct
	// C merge key ranked after the six-version recovery limit.
	RecoveryVersionCapDrops uint64
	// SingleHeaderPasses counts dispatch passes executed against a
	// single-header frontier (spec.c4-bytecode-isa.v1 section 5, obligation
	// R6). It is count-only and published on the gts_workcount board so
	// corridor coverage is a committed board row rather than a
	// profile-derived figure. It is a strict subset of Passes.
	SingleHeaderPasses uint64
	// CorridorPasses counts the subset of SingleHeaderPasses the C4 bytecode
	// corridor executed. It is zero on every build and every parse with the
	// corridor lane off, so it never perturbs the pinned board.
	CorridorPasses             uint64
	ActionLookups              uint64
	Dispatches                 uint64
	Conflicts                  uint64
	ConflictActions            uint64
	Forks                      uint64
	ConflictActionArmsAdmitted uint64
	CausalConflictForks        uint64
	ConflictHeads              uint64
	// ConvergedReductionSplitDrops counts no-action drops descended from a
	// reduction that split multiple compact predecessor paths into live heads.
	ConvergedReductionSplitDrops uint64
	// ConvergedCoverageDrops counts converged split drops whose dropped
	// header's recorded alternative set was contained in one surviving,
	// non-blended header's recorded set (spec.b4b-alternative-set.v2 section
	// 5, the revised theorem). Renamed from SelectedLineageDrops at stage
	// 2b, when the v2 containment predicate replaced the scalar (rank,
	// lineage) proof as the deciding proof.
	ConvergedCoverageDrops uint64
	RepetitionFolds        uint64
	Reductions             uint64
	OrdinaryShifts         uint64
	OrdinaryCohorts        uint64
	ExtraShifts            uint64
	ExtraCohorts           uint64
	Accepts                uint64
	// RecoverEOFAccepts counts one certified live publication of C's
	// recover_eof ERROR root. It is distinct from ordinary ActionAccept work.
	RecoverEOFAccepts uint64
	ReductionPauses   uint64
	NoActionDrops     uint64
	Elections         uint64
	// PerVersionLexRequests counts lexer calls issued for an owned parser
	// version. The shared lexer election does not contribute to this counter.
	PerVersionLexRequests uint64
	// PerVersionLexRestores counts exact DFA and scanner snapshot restores
	// before an owned request or a state-dependent re-request.
	PerVersionLexRestores uint64
	// PerVersionLexPublications counts immutable snapshots published to a
	// parser header or its scheduler sidecar.
	PerVersionLexPublications uint64
	// PerVersionLexAcceptedRaggedSpans counts different-width token views that
	// the owned scheduler accepted instead of declining at a shared cursor.
	PerVersionLexAcceptedRaggedSpans uint64
	// PerVersionLexViabilityDrops counts owned versions removed only after
	// their own tokens exhausted reductions without an action and a sibling's
	// token shifted from the same byte. These are not grammar-ambiguity drops.
	PerVersionLexViabilityDrops uint64
	// PeakLiveVersions records the largest live owned-header frontier.
	PeakLiveVersions  uint64
	Canonicalizations uint64
	PeakHeaders       uint64
	Overflow          bool
}

func (w *DiagnosticParserCoreGenericWork) add(counter *uint64, delta uint64) {
	if math.MaxUint64-*counter < delta {
		*counter = math.MaxUint64
		w.Overflow = true
		return
	}
	*counter += delta
}

// DiagnosticParserCoreGenericAcceptance records an authenticated EOF accept
// after the compact frontier has selected one derivation. A materiality-
// certified selection may represent more than one live derivation when every
// candidate publishes the same public tree. Payloads are the selected
// bottom-to-top compact stack; materialization does not mutate that graph.
type DiagnosticParserCoreGenericAcceptance struct {
	ElectionIndex  int
	Token          Token
	Header         DiagnosticParserCoreHeaderPathReceipt
	Payloads       []uint32
	Score          int64
	BranchOrder    uint64
	HasBranchOrder bool
	// MaterialityCertified records the bounded public-tree comparison that
	// makes a multi-derivation selection safe without a certified primary.
	MaterialityCertified bool
	// StructuralElectionCertified records an exact artifact's C-order proof.
	// It makes a clean multi-derivation selection safe without a primary proof.
	StructuralElectionCertified bool
	CoreWork                    core.Work
	Accepts                     uint64
	SelectedNodes               uint64
	SelectedParents             uint64
	SelectedLeaves              uint64
	Stats                       core.Stats
	Work                        DiagnosticParserCoreGenericWork
}

// DiagnosticParserCoreGenericConflict records one table-driven conflict cell.
// Actions preserve execution order: secondary ordinals first, then primary.
type DiagnosticParserCoreGenericConflictArm struct {
	Ordinal     int
	BranchOrder uint64
	Outputs     []DiagnosticParserCoreHeaderReceipt
	Paused      bool
	Adopted     bool
}

type DiagnosticParserCoreGenericConflict struct {
	ElectionIndex            int
	Token                    Token
	HeaderIndex              int
	BranchOrderBefore        uint64
	BranchOrderAfter         uint64
	NextCreationSeqBefore    uint64
	NextCreationSeqAfter     uint64
	Round                    DiagnosticParserCoreDispatchRound
	Prefix                   []DiagnosticParserCoreHeaderReceipt
	PrimaryOutput            DiagnosticParserCoreHeaderReceipt
	PrimaryPaused            bool
	PrimaryAdopted           bool
	OriginalSuffix           []DiagnosticParserCoreHeaderReceipt
	SecondaryArms            []DiagnosticParserCoreGenericConflictArm
	AdditionalPrimaryOutputs []DiagnosticParserCoreHeaderReceipt
	After                    []DiagnosticParserCoreHeaderReceipt
}

// DiagnosticParserCoreGenericNoActionDrop records a paused scheduler head
// removed only after a sibling made real progress in the same token epoch.
type DiagnosticParserCoreGenericNoActionDrop struct {
	ElectionIndex int
	Token         Token
	Header        DiagnosticParserCoreHeaderPathReceipt
}

// DiagnosticParserCoreGenericExternalShift ties every compact external
// terminal payload created by one generic scheduler round to its
// scanner-authenticated election without embedding scanner state in the
// compact graph. The round may be an ordinary or extra shift cohort, or a
// conflict with one or more shift arms.
type DiagnosticParserCoreGenericExternalShift struct {
	ElectionIndex int
	Token         Token
	ScannerBefore DiagnosticParserCoreScannerCheckpoint
	ScannerAfter  DiagnosticParserCoreScannerCheckpoint
	RoundIndex    int
	Payloads      []DiagnosticParserCoreTerminalPayloadView
}

// DiagnosticParserCoreGenericStop is the first semantic the table-driven
// clean scheduler deliberately does not implement.
type DiagnosticParserCoreGenericStop struct {
	Boundary      DiagnosticParserCoreBoundaryKind
	Detail        string
	ElectionIndex int
	HeaderIndex   int
	State         StateID
	ByteOffset    uint32
	Token         Token
	Headers       []DiagnosticParserCoreHeaderPathReceipt
	Stats         core.Stats
	Work          DiagnosticParserCoreGenericWork
}

// DiagnosticParserCoreGenericCompletion is a caller-selected, successfully
// closed scheduler frontier. LastToken is consumed; no pending lookahead has
// been read.
type DiagnosticParserCoreGenericCompletion struct {
	TargetByte    uint32
	ElectionIndex int
	LastToken     Token
	State         StateID
	Headers       []DiagnosticParserCoreHeaderPathReceipt
	Stats         core.Stats
	Work          DiagnosticParserCoreGenericWork
}

// DiagnosticParserCoreGenericScheduler records one committed compact scheduler
// run from the sole authenticated seed lifecycle before its first election.
type DiagnosticParserCoreGenericScheduler struct {
	ReceiptMode                      DiagnosticParserCoreReceiptMode
	StartCheckpoint                  DiagnosticParserCoreScannerCheckpoint
	StartHeaders                     []DiagnosticParserCoreHeaderPathReceipt
	Rounds                           []DiagnosticParserCoreDispatchRound
	Conflicts                        []DiagnosticParserCoreGenericConflict
	ExternalShifts                   []DiagnosticParserCoreGenericExternalShift
	Elections                        []DiagnosticParserCoreElection
	VersionLexerRequests             []DiagnosticParserCoreVersionLexerRequest
	NoActionDrops                    []DiagnosticParserCoreGenericNoActionDrop
	Completion                       *DiagnosticParserCoreGenericCompletion
	Acceptance                       *DiagnosticParserCoreGenericAcceptance
	acceptanceBacking                DiagnosticParserCoreGenericAcceptance
	Stop                             DiagnosticParserCoreGenericStop
	Tokens                           uint64
	Dispatches                       uint64
	GlobalBranchOrder                uint64
	NextCreationSeq                  uint64
	PerVersionLexRequests            uint64
	PerVersionLexRestores            uint64
	PerVersionLexPublications        uint64
	PerVersionLexAcceptedRaggedSpans uint64
	PerVersionLexViabilityDrops      uint64
	PeakLiveVersions                 uint64
	PotentialReductionActions        uint64
	PotentialReductionOutputs        uint64
	ReductionPromotions              uint64
	MissingTokenTrials               uint64
	MissingTokenCommits              uint64
	RecoveryDiscontinuityMerges      uint64
	RecoveryCeilingDeclines          uint64
}

type DiagnosticParserCorePrefixResult struct {
	Boundary          DiagnosticParserCoreBoundaryKind
	Detail            string
	Dispatches        uint64
	Tokens            uint64
	State             StateID
	Lookahead         Token
	LastBranchOrder   uint64
	GenericScheduler  *DiagnosticParserCoreGenericScheduler
	Completed         bool
	Elections         []DiagnosticParserCoreElection
	SourceSHA256      [32]byte
	GrammarBlobSHA256 [32]byte
	Grammar           string
	ExactRootDFA      bool
	Materialized      bool
	// MaterializedTree is a structural diagnostic owned by the caller and must
	// be released. It is set only after authenticated EOF acceptance and
	// one-shot compact-tree materialization succeed. The diagnostic runner does
	// not force parser-state replay, so its default tree retains the hard
	// incremental-reuse bar. The production admission runner separately forces
	// replay and may clear that bar only when materialization proves the required
	// states and scanner quiescence per tree.
	MaterializedTree *Tree
}

type diagnosticParserCoreDecline struct {
	boundary DiagnosticParserCoreBoundaryKind
	detail   string
}

//go:embed grammars/grammar_blobs/go.bin
var parserCoreCertifiedGoBlob []byte

func (e *diagnosticParserCoreDecline) Error() string { return string(e.boundary) + ": " + e.detail }

// parserCoreLanguageTables holds the immutable, language-derived compact-parser
// action and reduction tables. Its content depends only on the language's
// ParseActions plus its field and alias metadata, so every Parser of the same
// *Language shares one instance through the per-language cache below. Sharing
// the converted tables removes a full action-table rebuild (about 2.5 MB and
// 3.1k allocations for the Go grammar) from every fresh-Parser candidate parse.
type parserCoreLanguageTables struct {
	actionRows          []core.ActionRow
	reductionPlans      []core.ReductionPlan
	reductionPlanIndex  []uint16
	reductionPlanStride int
}

// parserCoreRootTables binds one Parser to the shared, immutable language
// tables. The Parser back-reference resolves only language-derived lookups
// (action index, goto, and root symbol), which every Parser of the same
// *Language resolves identically, so the wrapper stays per-Parser while the
// heavy converted tables stay shared.
type parserCoreRootTables struct {
	parser *Parser
	*parserCoreLanguageTables
}

// TableIdentity binds compact replay to the immutable Language that produced
// the cached action and reduction tables. The parser back-reference is read at
// validation time, so a language change cannot reuse stale state numbers.
func (a *parserCoreRootTables) TableIdentity() ([32]byte, bool) {
	if a == nil || a.parser == nil {
		return [32]byte{}, false
	}
	return a.parser.language.parserCoreTableIdentity()
}

// acquireParserCoreLanguageTables returns the converted tables for the parser's
// language, building them once and caching them on the Language itself.
//
// Retention decision -- the cache lives on the Language, so it lives and dies
// with the Language and pins nothing extra. Each cached table set retains only
// the converted action rows, reduction plans, and the reduction pair index:
// about 95 KiB for the Go grammar (measured by
// TestParserCoreLanguageTablesFootprint). A process-wide identity-keyed map was
// rejected: its strong key would pin every routed Language, and each Language
// holds a multi-megabyte decoded grammar and lex tables, so a caller that
// builds many transient languages would leak hundreds of megabytes. The
// sync.Once builds the tables exactly once per Language, even under concurrent
// first use, without holding a process-wide lock across the build.
func acquireParserCoreLanguageTables(parser *Parser) (*parserCoreLanguageTables, error) {
	if parser == nil || parser.language == nil {
		return nil, errors.New("parser-core phase zero: cannot cache actions without a parser language")
	}
	lang := parser.language
	lang.compactTablesOnce.Do(func() {
		lang.compactTables, lang.compactTablesErr = buildParserCoreLanguageTables(parser)
	})
	if lang.compactTablesErr != nil {
		return nil, lang.compactTablesErr
	}
	tables, _ := lang.compactTables.(*parserCoreLanguageTables)
	return tables, nil
}

// newParserCoreRootTables binds parser to the shared language tables. It builds
// the converted tables once per *Language and reuses them for every later
// Parser of the same language.
func newParserCoreRootTables(parser *Parser) (*parserCoreRootTables, error) {
	langTables, err := acquireParserCoreLanguageTables(parser)
	if err != nil {
		return nil, err
	}
	return &parserCoreRootTables{parser: parser, parserCoreLanguageTables: langTables}, nil
}

// buildParserCoreLanguageTables converts the immutable language action table
// into the compact-parser representation. It reads only language data, so the
// tables it returns are correct for every Parser of that language.
func buildParserCoreLanguageTables(parser *Parser) (*parserCoreLanguageTables, error) {
	if parser == nil || parser.language == nil {
		return nil, errors.New("parser-core phase zero: cannot cache actions without a parser language")
	}
	lang := parser.language
	rows := make([]core.ActionRow, len(lang.ParseActions))
	for index, entry := range lang.ParseActions {
		converted := make([]core.Action, len(entry.Actions))
		for ordinal, action := range entry.Actions {
			var err error
			converted[ordinal], err = parserCoreAction(action)
			if err != nil {
				return nil, fmt.Errorf("parser-core phase zero: convert action row %d ordinal %d: %w", index, ordinal, err)
			}
		}
		rows[index] = core.NewActionRow(converted, entry.Reusable)
	}
	tables := &parserCoreLanguageTables{actionRows: rows}
	maxProductionID, maxChildCount := 0, 0
	for _, row := range rows {
		for ordinal := 0; ordinal < row.Len(); ordinal++ {
			action := row.At(ordinal)
			if action.Type != core.ActionReduce {
				continue
			}
			maxProductionID = max(maxProductionID, int(action.ProductionID))
			maxChildCount = max(maxChildCount, int(action.ChildCount))
		}
	}
	tables.reductionPlanStride = maxChildCount + 1
	if tables.reductionPlanStride > 0 {
		if maxProductionID > (math.MaxInt/tables.reductionPlanStride)-1 {
			return nil, errors.New("parser-core phase zero: reduction plan pair index overflow")
		}
		tables.reductionPlanIndex = make([]uint16, (maxProductionID+1)*tables.reductionPlanStride)
	}
	for _, row := range rows {
		for ordinal := 0; ordinal < row.Len(); ordinal++ {
			action := row.At(ordinal)
			if action.Type != core.ActionReduce {
				continue
			}
			pairIndex := int(action.ProductionID)*tables.reductionPlanStride + int(action.ChildCount)
			if tables.reductionPlanIndex[pairIndex] != 0 {
				continue
			}
			fields, err := parserCoreProductionFields(lang, action.ProductionID, int(action.ChildCount))
			if err != nil {
				return nil, err
			}
			aliases, err := parserCoreProductionAliases(lang, action.ProductionID, int(action.ChildCount))
			if err != nil {
				return nil, err
			}
			plan, err := core.NewReductionPlan(action.ProductionID, int(action.ChildCount), fields, aliases)
			if err != nil {
				return nil, err
			}
			if len(tables.reductionPlans) >= math.MaxUint16 {
				return nil, errors.New("parser-core phase zero: reduction plan count exceeds uint16")
			}
			tables.reductionPlans = append(tables.reductionPlans, plan)
			tables.reductionPlanIndex[pairIndex] = uint16(len(tables.reductionPlans))
		}
	}
	return tables, nil
}

func buildParserCoreSelectedStorePolicy(parser *Parser) (core.SelectedStorePolicy, error) {
	if parser == nil || parser.language == nil || !parser.hasRootSymbol {
		return core.SelectedStorePolicy{}, errors.New("parser-core phase zero: selected-store policy requires an authenticated parser root")
	}
	lang := parser.language
	width := max(len(lang.SymbolMetadata), len(lang.SymbolNames), int(lang.SymbolCount))
	symbols := make([]core.SelectedSymbolPolicy, width)
	for index := range symbols {
		visible, named := true, false
		if index < len(lang.SymbolMetadata) {
			visible = lang.SymbolMetadata[index].Visible
			named = lang.SymbolMetadata[index].Named
		}
		symbols[index] = core.SelectedSymbolPolicy{Visible: visible, Named: named}
	}
	if width != 0 && width > math.MaxInt/width {
		return core.SelectedStorePolicy{}, errors.New("parser-core phase zero: selected-store unary policy overflow")
	}
	unary := make([]core.SelectedUnaryRule, width*width)
	for parent := 0; parent < width; parent++ {
		for child := 0; child < width; child++ {
			parentSymbol, childSymbol := Symbol(parent), Symbol(child)
			rule := core.SelectedUnaryKeep
			switch {
			case parentSymbol == childSymbol && !parser.isSharedVisibleAnonymousToken(childSymbol):
				rule = core.SelectedUnaryPass
			case parser.canCollapseInvisibleUnaryWrapperSymbol(parentSymbol):
				rule = core.SelectedUnaryPass
			case parser.canCollapseNamedLeafWrapper(parentSymbol, childSymbol) &&
				!parser.shouldPreserveVisibleUnaryTokenWrapper(parentSymbol) &&
				!parser.shouldKeepVisibleAnonymousTokenChild(parentSymbol, childSymbol):
				rule = core.SelectedUnaryRenameLeaf
			}
			unary[parent*width+child] = rule
		}
	}
	policy, err := core.NewSelectedStorePolicy(symbols, unary, core.Symbol(parser.rootSymbol))
	if err != nil {
		return core.SelectedStorePolicy{}, err
	}
	retainedAliases := make([]core.SelectedAliasChildPair, 0, len(parser.collapsedChildOccurrencePairs))
	for _, pair := range parser.collapsedChildOccurrencePairs {
		retainedAliases = append(retainedAliases, core.SelectedAliasChildPair{Alias: core.Symbol(pair.parent), Child: core.Symbol(pair.child)})
	}
	policy.SetRetainedAliasChildren(retainedAliases)
	syms, _ := goCompatibilitySymbolsForLanguage(lang)
	containers := make([]bool, width)
	for _, symbol := range syms.semiContainers[:syms.semiContainerLen] {
		if int(symbol) < width {
			containers[symbol] = true
		}
	}
	cases := make([]bool, width)
	for _, symbol := range [...]Symbol{syms.expressionCase, syms.defaultCase, syms.typeCase, syms.communicationCase} {
		if symbol != 0 && int(symbol) < width {
			cases[symbol] = true
		}
	}
	statementLists := make([]bool, width)
	for _, symbol := range [...]Symbol{syms.statementList, syms.statementListTail} {
		if symbol != 0 && int(symbol) < width {
			statementLists[symbol] = true
		}
	}
	if err := policy.SetGoCompatibility(core.Symbol(syms.semicolon), core.Symbol(syms.semicolonSentinel), containers, cases, statementLists); err != nil {
		return core.SelectedStorePolicy{}, err
	}
	return policy, nil
}

func (a *parserCoreRootTables) SelectedStorePolicy() (core.SelectedStorePolicy, error) {
	if a == nil || a.parser == nil || !a.parser.hasRootSymbol {
		return core.SelectedStorePolicy{}, nil
	}
	return buildParserCoreSelectedStorePolicy(a.parser)
}

func (a *parserCoreRootTables) Actions(state core.StateID, symbol core.Symbol) (core.ActionRow, error) {
	if a == nil || a.parser == nil || a.parser.language == nil {
		return core.ActionRow{}, errors.New("parser-core phase zero: incomplete cached action tables")
	}
	p := a.parser
	index := p.lookupActionIndex(StateID(state), Symbol(symbol))
	if index == 0 {
		return core.ActionRow{}, nil
	}
	if int(index) >= len(a.actionRows) {
		return core.ActionRow{}, errors.New("parser-core phase zero: canonical action index out of range")
	}
	return a.actionRows[index], nil
}

func (a *parserCoreRootTables) Goto(state core.StateID, symbol core.Symbol) (core.StateID, error) {
	return core.StateID(a.parser.lookupGoto(StateID(state), Symbol(symbol))), nil
}

func (a *parserCoreRootTables) ProductionFields(productionID uint16, childCount int) ([]core.FieldMapEntry, error) {
	return parserCoreProductionFields(a.parser.language, productionID, childCount)
}

// parserCoreProductionFields converts one production's field plan into compact
// field-map entries. It reads only language data, so it serves both the shared
// table build and the per-Parser TableView fallback.
func parserCoreProductionFields(lang *Language, productionID uint16, childCount int) ([]core.FieldMapEntry, error) {
	fieldIDs, inherited, _ := buildFieldPlanForProduction(lang, childCount, productionID)
	var out []core.FieldMapEntry
	for index, fieldID := range fieldIDs {
		if fieldID == 0 {
			continue
		}
		out = append(out, core.FieldMapEntry{FieldID: core.FieldID(fieldID), ChildIndex: uint8(index), Inherited: inherited[index]})
	}
	return out, nil
}

func (a *parserCoreRootTables) ProductionAliases(productionID uint16, childCount int) ([]core.Symbol, error) {
	return parserCoreProductionAliases(a.parser.language, productionID, childCount)
}

// parserCoreProductionAliases converts one production's alias sequence into
// compact symbols. It reads only language data, so it serves both the shared
// table build and the per-Parser TableView fallback.
func parserCoreProductionAliases(lang *Language, productionID uint16, childCount int) ([]core.Symbol, error) {
	if int(productionID) >= len(lang.AliasSequences) || childCount <= 0 || !languageProductionHasAliasSequence(lang, productionID, childCount) {
		return nil, nil
	}
	out := make([]core.Symbol, childCount)
	for i, symbol := range lang.AliasSequences[productionID] {
		if i >= childCount {
			break
		}
		out[i] = core.Symbol(symbol)
	}
	return out, nil
}

func (a *parserCoreRootTables) ReductionPlan(productionID uint16, childCount int) (core.ReductionPlan, error) {
	if a == nil || childCount < 0 || childCount >= a.reductionPlanStride {
		return core.ReductionPlan{}, errors.New("parser-core phase zero: reduction plan pair is outside authenticated index")
	}
	pairIndex := int(productionID)*a.reductionPlanStride + childCount
	if pairIndex < 0 || pairIndex >= len(a.reductionPlanIndex) {
		return core.ReductionPlan{}, errors.New("parser-core phase zero: reduction plan production is outside authenticated index")
	}
	planID := a.reductionPlanIndex[pairIndex]
	if planID == 0 || int(planID) > len(a.reductionPlans) {
		return core.ReductionPlan{}, errors.New("parser-core phase zero: reduction plan pair was not authenticated from an action row")
	}
	return a.reductionPlans[planID-1], nil
}

func parserCoreAction(action ParseAction) (core.Action, error) {
	var actionType core.ActionType
	switch action.Type {
	case ParseActionShift:
		actionType = core.ActionShift
	case ParseActionReduce:
		actionType = core.ActionReduce
	case ParseActionAccept:
		actionType = core.ActionAccept
	case ParseActionRecover:
		actionType = core.ActionRecover
	default:
		return core.Action{}, fmt.Errorf("parser-core phase zero: unknown root action type %d", action.Type)
	}
	return core.Action{
		Type: actionType, State: core.StateID(action.State), Symbol: core.Symbol(action.Symbol),
		ChildCount: action.ChildCount, DynamicPrecedence: action.DynamicPrecedence,
		ProductionID: action.ProductionID, Extra: action.Extra,
		ExtraChain: action.ExtraChain, Repetition: action.Repetition,
	}, nil
}

var parserCoreEmptyCheckpoint = DiagnosticParserCoreScannerCheckpoint{SHA256: sha256.Sum256(nil)}

func parserCoreCheckpoint(bytes []byte) DiagnosticParserCoreScannerCheckpoint {
	if len(bytes) == 0 {
		return parserCoreEmptyCheckpoint
	}
	return DiagnosticParserCoreScannerCheckpoint{Length: len(bytes), SHA256: sha256.Sum256(bytes)}
}

// parserCoreExternalScannerIdentityFingerprint binds one election snapshot to
// the scanner and grammar identity that produced its serialized state. Length
// prefixes keep distinct identifier pairs distinct without allocating on the
// election hot path.
func parserCoreExternalScannerIdentityFingerprint(identity ExternalScannerCheckpointIdentity) [32]byte {
	if !identity.complete() {
		return [32]byte{}
	}
	var encoded [8 + 2*externalScannerCheckpointIdentityMaxBytes]byte
	offset := 0
	binary.LittleEndian.PutUint16(encoded[offset:], uint16(len(identity.Scanner)))
	offset += 2
	copy(encoded[offset:], identity.Scanner)
	offset += len(identity.Scanner)
	binary.LittleEndian.PutUint16(encoded[offset:], uint16(len(identity.Grammar)))
	offset += 2
	copy(encoded[offset:], identity.Grammar)
	offset += len(identity.Grammar)
	return sha256.Sum256(encoded[:offset])
}

func diagnosticParserCoreInternCheckpoint(compact *core.Core, bytes []byte) (core.CheckpointID, DiagnosticParserCoreScannerCheckpoint, error) {
	id, err := compact.InternCheckpoint(bytes)
	if err != nil {
		return 0, DiagnosticParserCoreScannerCheckpoint{}, err
	}
	length, digest, ok := compact.CheckpointReceipt(id)
	if !ok {
		return 0, DiagnosticParserCoreScannerCheckpoint{}, errors.New("parser-core phase zero: interned checkpoint identity is unavailable")
	}
	return id, DiagnosticParserCoreScannerCheckpoint{Length: int(length), SHA256: digest}, nil
}

func configureParserCoreScannerProvenance(compact *core.Core, lang *Language) {
	if compact == nil {
		return
	}
	if classifyExternalScannerQuiescence(lang) == scannerQuiescenceProven {
		compact.CertifyExternalPayloadsQuiescent()
	}
	if languageUsesExternalScannerCheckpoints(lang) {
		compact.EnableTerminalScannerCheckpointProvenance()
	}
}

// DiagnosticParseParserCorePrefix independently schedules one compact seed
// against the complete production DFA/scanner election stream. Unsupported
// boundaries remain fail-closed. It never calls the production parser.
func DiagnosticParseParserCorePrefix(scanner ExternalScanner, source []byte, options DiagnosticParserCorePrefixOptions) (DiagnosticParserCorePrefixResult, error) {
	result := DiagnosticParserCorePrefixResult{SourceSHA256: sha256.Sum256(source)}
	if options.ReceiptMode != DiagnosticParserCoreReceiptFull && options.ReceiptMode != DiagnosticParserCoreReceiptSummary {
		result.Boundary, result.Detail = DiagnosticParserCoreRoute, "unknown diagnostic receipt mode"
		return result, &diagnosticParserCoreDecline{boundary: result.Boundary, detail: result.Detail}
	}
	lang, err := authenticatedParserCoreGoLanguage(scanner)
	if err != nil {
		result.Boundary, result.Detail = DiagnosticParserCoreIdentity, err.Error()
		return result, &diagnosticParserCoreDecline{boundary: result.Boundary, detail: result.Detail}
	}
	result.Grammar = lang.Name
	result.ExactRootDFA = true
	result.GrammarBlobSHA256 = sha256.Sum256(parserCoreCertifiedGoBlob)
	options.allowConvergedSplitDropArtifact = lang.CompactConvergedReductionSplitDropsCertified
	if options.Recovery || options.Retry || options.Incremental || options.IncludedRanges {
		result.Boundary, result.Detail = DiagnosticParserCoreRoute, "recovery/retry/incremental/included-range routes decline"
		return result, &diagnosticParserCoreDecline{boundary: result.Boundary, detail: result.Detail}
	}
	if options.MaxDispatches == 0 {
		options.MaxDispatches = 100000
	}
	if options.MaxTokens == 0 {
		options.MaxTokens = 100000
	}
	parser := NewParser(lang)
	options.noLookaheadRootSymbol = parser.rootSymbol
	options.hasNoLookaheadRootSymbol = parser.hasRootSymbol
	tables, err := newParserCoreRootTables(parser)
	if err != nil {
		return result, err
	}
	compact, err := core.New(tables, options.Limits)
	if err != nil {
		return result, err
	}
	configureParserCoreScannerProvenance(compact, lang)
	tokenSource := parser.acquireParserDFATokenSource(source)
	if tokenSource == nil {
		return result, errors.New("parser-core phase zero: production DFA unavailable")
	}
	defer tokenSource.Close()
	var scannerScratch []byte
	var observedRun core.Phase0ADiagnosticRun
	if core.Phase0AEnabled {
		observedRun, err = core.BeginPhase0ADiagnosticRun(compact)
		if err != nil {
			return result, err
		}
	}
	parsed, parseErr := diagnosticParseParserCoreGenericFromSeed(
		result, compact, tokenSource, &scannerScratch, parser, lang.InitialState, source, options,
	)
	if core.Phase0AEnabled {
		if endErr := core.EndPhase0ADiagnosticRun(observedRun); parseErr == nil && endErr != nil {
			return parsed, endErr
		}
	}
	return parsed, parseErr
}

func diagnosticParseParserCoreGenericFromSeed(
	result DiagnosticParserCorePrefixResult,
	compact *core.Core,
	tokenSource *dfaTokenSource,
	scannerScratch *[]byte,
	parser *Parser,
	initialState StateID,
	source []byte,
	options DiagnosticParserCorePrefixOptions,
) (DiagnosticParserCorePrefixResult, error) {
	options.materializationParser = parser
	options.materializationSource = source
	// The diagnostic prefix route's own post-acceptance materialization
	// (below, and publishDiagnosticParserCoreGenericResult's callback) always
	// forces false; match it so the gate compares the same ParseState/
	// PreGotoState presence the published tree actually carries.
	options.materializationForceReplayParseStates = false
	options.materializationContextSet = true
	scheduler, runErr := executeDiagnosticParserCoreGenericSchedulerFromSeed(
		compact, tokenSource, scannerScratch, initialState, options, diagnosticParserCoreSeedObserver{},
	)
	if runErr != nil {
		var decline *diagnosticParserCoreDecline
		if errors.As(runErr, &decline) {
			result.Boundary, result.Detail = decline.boundary, decline.detail
		}
		return result, runErr
	}
	if scheduler == nil || scheduler.receipt == nil {
		return result, errors.New("parser-core phase zero: seed scheduler returned no receipt")
	}
	generic := scheduler.receipt
	if generic.Stop.Boundary != "" {
		result.Boundary, result.Detail = generic.Stop.Boundary, generic.Stop.Detail
		return result, &diagnosticParserCoreDecline{boundary: result.Boundary, detail: result.Detail}
	}
	return publishDiagnosticParserCoreGenericResult(result, scheduler, func(head core.Head) (*Tree, error) {
		return materializeDiagnosticParserCoreAcceptedSelection(compact, head, scheduler.acceptedPayloads, parser, source, nil, false, options.Recovery && options.allowCompactStrategy2ErrorRegion)
	})
}

func publishDiagnosticParserCoreGenericResult(
	result DiagnosticParserCorePrefixResult,
	scheduler *diagnosticParserCoreGenericScheduler,
	materialize func(core.Head) (*Tree, error),
) (DiagnosticParserCorePrefixResult, error) {
	if scheduler == nil || scheduler.receipt == nil {
		return result, errors.New("parser-core phase zero: cannot publish an empty generic scheduler")
	}
	generic := scheduler.receipt
	if generic.Completion != nil {
		if generic.Dispatches != generic.Completion.Work.Dispatches {
			return result, errors.New("parser-core phase zero: seed scheduler completion dispatch totals diverged")
		}
		result.Tokens = generic.Tokens
		result.Dispatches = generic.Dispatches
		result.LastBranchOrder = generic.GlobalBranchOrder
		result.GenericScheduler = generic
		result.Elections = append([]DiagnosticParserCoreElection(nil), generic.Elections...)
		result.Completed = true
		result.State = generic.Completion.State
		result.Lookahead = Token{}
		result.Boundary = DiagnosticParserCoreGenericClosed
		result.Detail = "seed-owned generic scheduler reached the requested closed byte without reading another lookahead"
		return result, nil
	}
	if generic.Acceptance == nil {
		return result, errors.New("parser-core phase zero: seed scheduler ended without completion, acceptance, or stop")
	}
	if generic.Dispatches != generic.Acceptance.Work.Dispatches {
		return result, errors.New("parser-core phase zero: seed scheduler acceptance dispatch totals diverged")
	}
	if materialize == nil {
		return result, errors.New("parser-core phase zero: accepted seed scheduler requires a materializer")
	}
	tree, materializeErr := materialize(scheduler.acceptedHead)
	if materializeErr != nil {
		return result, materializeErr
	}
	if tree == nil {
		return result, errors.New("parser-core phase zero: accepted seed scheduler materializer returned no tree")
	}
	selected := diagnosticParserCoreSelectedNodeCensus(tree.root)
	generic.Acceptance.SelectedNodes = selected.total
	generic.Acceptance.SelectedParents = selected.parents
	generic.Acceptance.SelectedLeaves = selected.leaves
	result.Tokens = generic.Tokens
	result.Dispatches = generic.Dispatches
	result.LastBranchOrder = generic.GlobalBranchOrder
	result.GenericScheduler = generic
	result.Elections = append([]DiagnosticParserCoreElection(nil), generic.Elections...)
	result.Completed = true
	result.Materialized = true
	result.MaterializedTree = tree
	result.State = generic.Acceptance.Header.Header.State
	result.Lookahead = generic.Acceptance.Token
	result.Boundary = DiagnosticParserCoreGenericClosed
	result.Detail = "seed-owned generic scheduler accepted EOF and materialized one exact compact derivation"
	return result, nil
}

type diagnosticParserCoreSelectedCensus struct {
	total   uint64
	parents uint64
	leaves  uint64
}

func diagnosticParserCoreSelectedNodeCensus(root *Node) diagnosticParserCoreSelectedCensus {
	if root == nil {
		return diagnosticParserCoreSelectedCensus{}
	}
	var census diagnosticParserCoreSelectedCensus
	stack := []*Node{root}
	for len(stack) != 0 {
		last := len(stack) - 1
		node := stack[last]
		stack = stack[:last]
		if node == nil {
			continue
		}
		census.total++
		if len(node.children) == 0 {
			census.leaves++
		} else {
			census.parents++
		}
		stack = append(stack, node.children...)
	}
	return census
}

// Field order groups creationSeq (8-byte aligned), then every 4-byte-aligned
// field (head, drop-cohort refs, checkpoint, altSet, lastPersistedHead,
// lastPersistedAltSet, lastPersistedDropCohortRefs),
// then cleanPathLineage (2-byte aligned), then every remaining byte-sized
// field. This is layout-only: every construction site across the package
// and its tests uses keyed fields (grep-verified), so declaration order
// changes memory footprint, never behavior. The exact current size is 224
// bytes (unsafe.Sizeof-verified, parsercore_phase0_canonical_scratch_internal_test.go).
// despite carrying two full (event, branch) alternative sets plus three
// bools v1 never had (b4b-width-repair audit, 2026-08): the widened
// AlternativeSet's own inline-capacity reduction (core.go) supplies most of
// the recovered space, and this reorder folds the three new bools into
// padding a naive append-at-the-end declaration order would otherwise pay
// for separately.
type diagnosticParserCoreHeader struct {
	creationSeq    uint64
	head           core.Head
	dropCohortRefs core.DropCohortRefSet
	checkpoint     core.CheckpointID
	// altSet mirrors cleanPathRank/cleanPathLineage but by union rather than
	// overwrite: it accumulates every converged-split (event, branch) member
	// this derivation thread has passed through and is never invalidated
	// (carried unchanged through an external shift that zeroes
	// cleanPathLineage; see markDiagnosticParserCoreExternalLineage). The
	// scalar (rank, lineage) pair remains the live decider
	// (dropGenericNoActionHeads); altSet and blended are read by the v1/v2
	// containment census only (spec.b4b-alternative-set.v2 section 7).
	altSet core.AlternativeSet
	// lastPersistedHead and lastPersistedAltSet record the (head, altSet)
	// pair persistHeaderLineageOwned last actually wrote to a node. Both are
	// plain comparable values, so persistHeaderLineageOwned can detect a
	// no-op persist (same node, same set already recorded there) with two
	// equality checks instead of re-entering the scheduler-owned set-union
	// machinery every dispatch. A rolled-back dispatch reverts these fields
	// along with the rest of the header (diagnosticParserCoreHeaderRollbackScratch
	// snapshots the whole struct by value), so they never claim a persist
	// that Core itself undid. lastPersistedBlended extends the same no-op
	// detection to blended: persistHeaderLineageOwned must also re-persist
	// when only blended changed (spec.b4b-alternative-set.v2 section 10).
	lastPersistedHead           core.Head
	lastPersistedAltSet         core.AlternativeSet
	lastPersistedDropCohortRefs core.DropCohortRefSet
	frontierSequence            uint32
	cleanPathLineage            uint16
	freshness                   core.ReductionFreshness
	shifted                     bool
	accepted                    bool
	paused                      bool
	convergedReductionSplit     bool
	// resurrectionUnproved marks a header descended from a
	// HistoricalBoundaryUnproved dead-node import: a non-deterministic,
	// non-converged historical boundary with no recorded provenance to prove
	// (spec.b4b-alternative-set.v2 section 5, F4 disposition). It carries no
	// alternative-set members, so containment can never prove it; it fails a
	// no-action drop closed independently of the live proof, waived by the
	// same certified-language artifact escape that waives the proof itself.
	resurrectionUnproved bool
	cleanPathRank        core.CleanPathRankSelection
	// blended records whether altSet was ever produced by folding two
	// incomparable recorded sets together (spec.b4b-alternative-set.v2
	// section 3.4). A blended header can never serve as a v2 containment
	// witness (section 5).
	blended              bool
	lastPersistedBlended bool
	// recoveryFlags records recovery competition and permanent cost provenance.
	// It sits before versionState in the padding byte at offset 215. Placing it
	// after the pointer would grow each header from 224 to 232 bytes.
	//
	// A frontier that merely forked on ordinary grammar ambiguity must NOT be
	// marked. Error cost answers "which recovery is cheaper", which is not the
	// question that frontier asks.
	//
	// The open-recovery segment count is deliberately NOT stored beside this.
	// It is derived from live header state at pricing time -- see
	// recoveryOpenSegments. Ordinary paths write paused, while region paths
	// publish versionState. A stored copy could drift from either value.
	recoveryFlags diagnosticParserCoreRecoveryFlags
	// versionState carries optional state owned by this parser version. It is
	// nil on the single-version fast path. The pointed-to state is immutable.
	// Accessors publish a fresh wrapper or clear the pointer. A by-value header
	// snapshot can therefore restore its prior state on rollback.
	versionState *diagnosticParserCoreVersionState
}

// diagnosticParserCoreVersionState holds immutable state for one header.
// Keep it separate from the fixed-size header so lexer ownership can grow
// without changing the header layout. Region updates publish a new wrapper
// while preserving the lexer snapshot, and snapshot updates do the reverse.
type diagnosticParserCoreVersionState struct {
	s3Region      *diagnosticParserCoreS3Region
	relexSnapshot *diagnosticParserCoreVersionLexerSnapshot
	// lexerRequest is a one-based scheduler sidecar reference. Reductions
	// preserve it because they do not consume lookahead. Shifts clear it when
	// they publish the request's after snapshot.
	lexerRequest uint32
	// recoveryGroup identifies one live C error-state group. missingGroup ties
	// a missing-token version to that group for C's S5 ordering rule.
	recoveryGroup uint64
	missingGroup  uint64
	// recoveryNodeBaseline is C's cumulative visible-node count at the last
	// error entry. Current counts come from the live graph during condensation.
	recoveryNodeBaseline    uint32
	recoveryNodeBaselineSet bool
}

type diagnosticParserCoreRecoveryFlags uint8

const (
	diagnosticParserCoreRecoveryCompetitorFlag diagnosticParserCoreRecoveryFlags = 1 << iota
	diagnosticParserCoreRecoveryCostedFlag
)

// recoveryRegion returns the optional open strategy-2 region.
func (h diagnosticParserCoreHeader) recoveryRegion() *diagnosticParserCoreS3Region {
	if h.versionState == nil {
		return nil
	}
	return h.versionState.s3Region
}

// versionLexerSnapshot returns the immutable DFA/scanner state owned by this
// parser version. The nil value is the single-version fast-path state.
func (h diagnosticParserCoreHeader) versionLexerSnapshot() *diagnosticParserCoreVersionLexerSnapshot {
	if h.versionState == nil {
		return nil
	}
	return h.versionState.relexSnapshot
}

func (h diagnosticParserCoreHeader) versionLexerRequestReference() uint32 {
	if h.versionState == nil {
		return 0
	}
	return h.versionState.lexerRequest
}

func (h diagnosticParserCoreHeader) recoveryGroupIdentity() uint64 {
	if h.versionState == nil {
		return 0
	}
	return h.versionState.recoveryGroup
}

func (h diagnosticParserCoreHeader) recoveryMissingGroupIdentity() uint64 {
	if h.versionState == nil {
		return 0
	}
	return h.versionState.missingGroup
}

func (h diagnosticParserCoreHeader) recoveryNodeBaseline() (uint32, bool) {
	if h.versionState == nil {
		return 0, false
	}
	return h.versionState.recoveryNodeBaseline, h.versionState.recoveryNodeBaselineSet
}

func (h *diagnosticParserCoreHeader) publishVersionState(
	region *diagnosticParserCoreS3Region,
	snapshot *diagnosticParserCoreVersionLexerSnapshot,
	request uint32,
	recoveryGroup, missingGroup uint64,
	nodeBaseline uint32,
	nodeBaselineSet bool,
) {
	if h == nil {
		return
	}
	if region == nil && snapshot == nil && request == 0 && recoveryGroup == 0 &&
		missingGroup == 0 && !nodeBaselineSet {
		h.versionState = nil
		return
	}
	h.versionState = &diagnosticParserCoreVersionState{
		s3Region: region, relexSnapshot: snapshot, lexerRequest: request,
		recoveryGroup: recoveryGroup, missingGroup: missingGroup,
		recoveryNodeBaseline: nodeBaseline, recoveryNodeBaselineSet: nodeBaselineSet,
	}
}

func (h *diagnosticParserCoreHeader) publishRecoveryCondenseState(
	recoveryGroup, missingGroup uint64,
	nodeBaseline uint32,
	nodeBaselineSet bool,
) {
	if h == nil {
		return
	}
	h.publishVersionState(
		h.recoveryRegion(), h.versionLexerSnapshot(), h.versionLexerRequestReference(),
		recoveryGroup, missingGroup, nodeBaseline, nodeBaselineSet,
	)
}

func (h diagnosticParserCoreHeader) isRecoveryCosted() bool {
	return h.recoveryFlags&diagnosticParserCoreRecoveryCostedFlag != 0
}

// openRecoveryRegion publishes an open strategy-2 region for this header.
func (h *diagnosticParserCoreHeader) openRecoveryRegion(region *diagnosticParserCoreS3Region) {
	h.setRecoveryRegion(region)
}

// setRecoveryRegion publishes a fresh immutable wrapper for the region.
func (h *diagnosticParserCoreHeader) setRecoveryRegion(region *diagnosticParserCoreS3Region) {
	if h == nil {
		return
	}
	if region == nil {
		h.closeRecoveryRegion()
		return
	}
	baseline, baselineSet := h.recoveryNodeBaseline()
	h.publishVersionState(
		cloneDiagnosticParserCoreS3Region(region), h.versionLexerSnapshot(),
		h.versionLexerRequestReference(), h.recoveryGroupIdentity(),
		h.recoveryMissingGroupIdentity(), baseline, baselineSet,
	)
}

// closeRecoveryRegion clears the header's open strategy-2 region.
func (h *diagnosticParserCoreHeader) closeRecoveryRegion() {
	if h != nil {
		baseline, baselineSet := h.recoveryNodeBaseline()
		if !h.isRecoveryLineage() {
			baseline, baselineSet = 0, false
		}
		h.publishVersionState(
			nil, h.versionLexerSnapshot(), h.versionLexerRequestReference(),
			0, h.recoveryMissingGroupIdentity(), baseline, baselineSet,
		)
	}
}

// isRecoveryLineage reports whether this header may compete at acceptance.
func (h *diagnosticParserCoreHeader) isRecoveryLineage() bool {
	return h != nil && h.recoveryFlags&diagnosticParserCoreRecoveryCompetitorFlag != 0
}

// markRecoveryLineage marks a competing recovery lineage with cost provenance.
func (h *diagnosticParserCoreHeader) markRecoveryLineage() {
	if h == nil {
		return
	}
	h.recoveryFlags |= diagnosticParserCoreRecoveryCompetitorFlag | diagnosticParserCoreRecoveryCostedFlag
	if _, set := h.recoveryNodeBaseline(); !set {
		h.publishRecoveryCondenseState(
			h.recoveryGroupIdentity(), h.recoveryMissingGroupIdentity(), 0, true,
		)
	}
}

// markRecoveryCosted records recovery work without creating a competition.
func (h *diagnosticParserCoreHeader) markRecoveryCosted() {
	if h != nil {
		h.recoveryFlags |= diagnosticParserCoreRecoveryCostedFlag
	}
}

// clearRecoveryLineage removes this header from a finished competition.
//
// Do not call it at an ordinary ambiguity fork. C copies the complete stack
// head there, including its recovery cost. Each ambiguity arm must therefore
// retain the competitor flag until C's cost and precedence election runs.
func (h *diagnosticParserCoreHeader) clearRecoveryLineage() {
	if h == nil || !h.isRecoveryLineage() {
		return
	}
	h.recoveryFlags &^= diagnosticParserCoreRecoveryCompetitorFlag
	h.publishRecoveryCondenseState(0, 0, 0, false)
}

// competingRecoveryFrontier reports whether every live version belongs to
// one recovery competition. Such versions can accept independently at EOF.
func (s *diagnosticParserCoreGenericScheduler) competingRecoveryFrontier() bool {
	if s == nil || !s.recoveryIsolation || !s.options.allowCompactRecoveryLineageSelection || len(s.headers) < 2 {
		return false
	}
	for index := range s.headers {
		if !s.headers[index].isRecoveryLineage() {
			return false
		}
	}
	return true
}

// invalidateVerifierHeaderBinding drops the test-only pointer whenever a
// frontier mutation can replace or compact its backing array.
func (s *diagnosticParserCoreGenericScheduler) invalidateVerifierHeaderBinding() {
	if s == nil {
		return
	}
	s.verifierHeaderPtr = nil
	s.verifierBound = 0
}

// retireTrailingRecoveryNoActionLineage ports one exact C condense-tail
// transition. S5 orders the absorbing version before the missing version. If
// the absorbing version consumes the elected token and the later missing
// version cannot act on it, C keeps the earlier unpaused version and removes
// the later paused version without another recovery election.
//
// Keep the proof deliberately narrow. Both versions must share the elected
// checkpoint, carry no open error region, and preserve creation order. The
// survivor must end exactly at the elected token boundary. Every other
// recovery frontier continues to fail closed.
func (s *diagnosticParserCoreGenericScheduler) retireTrailingRecoveryNoActionLineage(indices []int) (bool, error) {
	if s == nil || !s.options.allowCompactRecoveryTrailingLineageRetirement ||
		!s.competingRecoveryFrontier() || len(s.headers) != 2 || len(indices) != 1 || indices[0] != 1 ||
		!diagnosticParserCoreGenericNoActionDropEligible(s.headers, indices, s.epochProgress) {
		return false, nil
	}
	survivor := &s.headers[0]
	loser := &s.headers[1]
	if !survivor.shifted || survivor.accepted || survivor.paused || loser.shifted || loser.accepted || loser.paused ||
		survivor.recoveryRegion() != nil || loser.recoveryRegion() != nil || survivor.creationSeq >= loser.creationSeq ||
		survivor.checkpoint != loser.checkpoint || survivor.checkpoint != s.checkpointID ||
		s.token.Symbol == 0 || s.token.EndByte <= s.token.StartByte {
		return false, nil
	}
	_, survivorByte, err := s.compact.Boundary(survivor.head)
	if err != nil {
		return false, err
	}
	_, loserByte, err := s.compact.Boundary(loser.head)
	if err != nil {
		return false, err
	}
	if survivorByte != s.token.EndByte || loserByte > s.token.StartByte {
		return false, nil
	}

	survivor.clearRecoveryLineage()
	s.invalidateVerifierHeaderBinding()
	clear(s.headers[1:])
	s.headers = s.headers[:1]
	s.recoveryIsolation = false
	s.work.add(&s.work.RecoveryLineageRetirements, 1)
	return true, nil
}

// recoveryOpenSegments returns the open-recovery segment count pricing charges
// RecoveryCostPerRecovery for, DERIVED from live header state rather than
// stored.
//
// It ports cStackOpenRecoveryCost's predicate (parser_recover_c.go:2475-2489),
// `s.cPaused || (s.cRec != nil && s.cRec.openErr == nil)`: a paused head, or
// an error region opened but still empty, carries one segment that no
// published subtree accounts for.
//
// Deriving rather than storing is the point. paused is written by the
// ordinary reduce and pause paths (they set it false on a fresh reduction and
// true on a reduction pause) and versionState is replaced wholesale by the region
// advance, none of which would update a stored count. A stale count
// mis-prices the lineage by 500, which is ten times the margin this
// arbitration turns on.
//
// C's extraRecoveries term has no compact analogue yet: stage S3 does not
// track unlexable-run re-pauses. When it does, add it here.
func (h *diagnosticParserCoreHeader) recoveryOpenSegments() int {
	region := h.recoveryRegion()
	if h.paused || (region != nil && len(region.children) == 0) {
		return 1
	}
	return 0
}

// diagnosticParserCoreS3Region is the open ERROR container a native S3
// recovery region accumulates on its owning header -- the compact analogue
// of glrStack.cRec.openErr (glr.go), living on the header rather than the
// arena until s3AdvanceErrorRegion resolves it (design section 4, restating
// the S2 doc comment for S3). state is the pre-error state probed for resume
// each pass (depth-0 resume only; see s3RegionResumeAction).
type diagnosticParserCoreS3Region struct {
	state     core.StateID
	startByte uint32
	endByte   uint32
	children  []core.SubtreeID
}

// cloneDiagnosticParserCoreS3Region makes the header's recovery region
// independent from the caller's region and child backing array.
func cloneDiagnosticParserCoreS3Region(region *diagnosticParserCoreS3Region) *diagnosticParserCoreS3Region {
	if region == nil {
		return nil
	}
	owned := *region
	if region.children != nil {
		owned.children = make([]core.SubtreeID, len(region.children))
		copy(owned.children, region.children)
	}
	return &owned
}

func nextDiagnosticParserCoreCleanPathLineage(next *uint16) (uint16, error) {
	if next == nil || *next == 0 {
		return 0, &diagnosticParserCoreDecline{
			boundary: DiagnosticParserCoreCap,
			detail:   "clean multi-pop lineage identity cap",
		}
	}
	lineage := *next
	if lineage == math.MaxUint16 {
		*next = 0
	} else {
		(*next)++
	}
	return lineage, nil
}

func mergeDiagnosticParserCoreCleanPathLineage(
	leftRank core.CleanPathRankSelection,
	leftLineage uint16,
	rightRank core.CleanPathRankSelection,
	rightLineage uint16,
) (core.CleanPathRankSelection, uint16) {
	if leftRank == core.CleanPathRankUnknown || rightRank == core.CleanPathRankUnknown {
		return core.CleanPathRankUnknown, 0
	}
	if leftRank == core.CleanPathRankNotApplicable || leftLineage == 0 {
		return rightRank, rightLineage
	}
	if rightRank == core.CleanPathRankNotApplicable || rightLineage == 0 {
		return leftRank, leftLineage
	}
	if leftLineage != rightLineage {
		return core.CleanPathRankUnknown, 0
	}
	if leftRank == core.CleanPathRankSelected || rightRank == core.CleanPathRankSelected {
		return core.CleanPathRankSelected, leftLineage
	}
	return core.CleanPathRankUnselected, leftLineage
}

func applyDiagnosticParserCoreCleanPathOutput(
	header *diagnosticParserCoreHeader,
	rank core.CleanPathRankSelection,
	lineage uint16,
) {
	if header == nil || header.cleanPathRank == core.CleanPathRankUnknown {
		return
	}
	if rank == core.CleanPathRankNotApplicable || lineage == 0 {
		return
	}
	header.cleanPathRank = rank
	header.cleanPathLineage = lineage
}

func markDiagnosticParserCoreExternalLineage(
	header *diagnosticParserCoreHeader,
	token Token,
) {
	if header == nil || !token.ExternalScannerToken ||
		header.cleanPathRank == core.CleanPathRankNotApplicable {
		return
	}
	header.cleanPathRank = core.CleanPathRankUnknown
	header.cleanPathLineage = 0
}

func mergeDiagnosticParserCoreFrontier(left, right uint32) uint32 {
	if left == 0 {
		return right
	}
	if right == 0 || left == right {
		return left
	}
	// A canonical group must not claim two different producer frontiers.
	// Clear the compact sequence and let the authenticated producer state fail
	// closed if a later consumer requests a frontier from this group.
	return 0
}

func (s *diagnosticParserCoreGenericScheduler) persistHeaderLineageOwned(
	owner core.SchedulerTransactionToken,
) error {
	for index := range s.headers {
		header := &s.headers[index]
		if header.creationSeq >= math.MaxUint32 {
			return errors.New("parser-core phase zero: scheduler lineage overflow")
		}
		if err := s.compact.RecordHeadOwnerOwned(
			owner,
			header.head,
			uint32(header.creationSeq)+1,
		); err != nil {
			return fmt.Errorf(
				"parser-core phase zero: persist header %d head=%d owner=%d lexer_owned=%t recovery_region=%t: %w",
				index, header.head.Node, header.creationSeq+1,
				header.versionLexerSnapshot() != nil, header.recoveryRegion() != nil, err,
			)
		}
		if !header.convergedReductionSplit && header.dropCohortRefs.Empty() &&
			!header.dropCohortRefs.Overflowed() && !header.dropCohortRefs.Blended() {
			continue
		}
		// The scalar pair is re-merged unconditionally: recordNodeLineage
		// already no-ops cheaply when nothing changed, and rank can flip
		// (Unselected -> Selected on the same lineage id) without altSet
		// gaining a member, so scalar dirtiness can't be inferred from set
		// dirtiness alone. The set union is the expensive, and far more
		// often redundant, half (persistHeaderLineageOwned runs every
		// dispatch for every still-active header, not only the one that
		// dispatch actually touched): skip it when this exact (head, altSet,
		// blended) triple is already what was last persisted for this header
		// (spec.b4b-alternative-set.v2 section 10: the dirtiness check must
		// also compare blended, or conservatively persist when it changes).
		setDirty := header.head != header.lastPersistedHead ||
			header.altSet != header.lastPersistedAltSet ||
			header.blended != header.lastPersistedBlended ||
			header.dropCohortRefs != header.lastPersistedDropCohortRefs
		if err := s.compact.RecordHeadLineageOwned(
			owner,
			header.head,
			header.cleanPathRank,
			header.cleanPathLineage,
			header.altSet,
			header.blended,
			setDirty,
			header.dropCohortRefs,
		); err != nil {
			return err
		}
		header.lastPersistedHead = header.head
		header.lastPersistedAltSet = header.altSet
		header.lastPersistedDropCohortRefs = header.dropCohortRefs
		header.lastPersistedBlended = header.blended
	}
	return nil
}

// errDiagnosticParserCoreUnknownCheckpointIdentity is the shared sentinel for
// a header (or a cold-path identity-gate reject) that names a checkpoint the
// compact core never interned. Both callers below return it unwrapped, so
// callers may compare with errors.Is.
var errDiagnosticParserCoreUnknownCheckpointIdentity = errors.New("parser-core phase zero: header references unknown checkpoint identity")

func diagnosticParserCoreCheckpointDigest(compact *core.Core, id core.CheckpointID) ([32]byte, error) {
	_, digest, ok := compact.CheckpointReceipt(id)
	if !ok {
		return [32]byte{}, errDiagnosticParserCoreUnknownCheckpointIdentity
	}
	return digest, nil
}

func diagnosticParserCoreHeaderReceipt(compact *core.Core, header diagnosticParserCoreHeader) (DiagnosticParserCoreHeaderReceipt, error) {
	state, byteOffset, err := compact.Boundary(header.head)
	if err != nil {
		return DiagnosticParserCoreHeaderReceipt{}, err
	}
	stats, err := compact.Stats(header.head)
	if err != nil {
		return DiagnosticParserCoreHeaderReceipt{}, err
	}
	checkpoint, err := diagnosticParserCoreCheckpointDigest(compact, header.checkpoint)
	if err != nil {
		return DiagnosticParserCoreHeaderReceipt{}, err
	}
	return DiagnosticParserCoreHeaderReceipt{
		CreationSeq: header.creationSeq,
		State:       StateID(state),
		ByteOffset:  byteOffset,
		Shifted:     header.shifted,
		Accepted:    header.accepted,
		Paused:      header.paused,
		ExactPaths:  stats.CurrentExactPaths,
		Checkpoint:  checkpoint,
	}, nil
}

func diagnosticParserCoreHeaderSummary(compact *core.Core, header diagnosticParserCoreHeader) (DiagnosticParserCoreHeaderReceipt, error) {
	state, byteOffset, err := compact.Boundary(header.head)
	if err != nil {
		return DiagnosticParserCoreHeaderReceipt{}, err
	}
	checkpoint, err := diagnosticParserCoreCheckpointDigest(compact, header.checkpoint)
	if err != nil {
		return DiagnosticParserCoreHeaderReceipt{}, err
	}
	return DiagnosticParserCoreHeaderReceipt{
		CreationSeq: header.creationSeq,
		State:       StateID(state),
		ByteOffset:  byteOffset,
		Shifted:     header.shifted,
		Accepted:    header.accepted,
		Paused:      header.paused,
		Checkpoint:  checkpoint,
	}, nil
}

func diagnosticParserCoreHeaderReceipts(compact *core.Core, headers []diagnosticParserCoreHeader) ([]DiagnosticParserCoreHeaderReceipt, error) {
	out := make([]DiagnosticParserCoreHeaderReceipt, len(headers))
	for index, header := range headers {
		receipt, err := diagnosticParserCoreHeaderReceipt(compact, header)
		if err != nil {
			return nil, err
		}
		out[index] = receipt
	}
	return out, nil
}

func validateDiagnosticParserCoreCell(token Token, actions core.ActionRow) error {
	return validateDiagnosticParserCoreCellWithRepetitionFork(token, actions, false)
}

func validateDiagnosticParserCoreCellWithRepetitionFork(token Token, actions core.ActionRow, allowRepetitionFork bool) error {
	if token.NoLookahead {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "no-lookahead tokens require production recovery semantics"}
	}
	if actions.Len() == 0 {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreNoAction, detail: "canonical cell has no action"}
	}
	for ordinal := 0; ordinal < actions.Len(); ordinal++ {
		action := actions.At(ordinal)
		if action.Repetition {
			if _, ok := diagnosticParserCoreSingleReduceRepetitionShiftOrdinal(actions); allowRepetitionFork && ok {
				continue
			}
			return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "repetition shifts require production frontier suppression semantics"}
		}
		if action.ExtraChain {
			return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreExtraChain, detail: "extra-chain shift requires distinct nonterminal-chain semantics"}
		}
		if action.Extra && action.Type != core.ActionShift {
			return errors.New("parser-core phase zero: decoded extra action is not a shift")
		}
		if action.Type == core.ActionRecover {
			return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRecovery, detail: "recovery is unsupported in same-lookahead closure"}
		}
		if action.Type == core.ActionAccept {
			return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreAccept, detail: "accept requires authenticated EOF selection"}
		}
	}
	return nil
}

type diagnosticParserCoreConflictExecution struct {
	outputs     []diagnosticParserCoreHeader
	armRanges   []diagnosticParserCoreConflictArmRange
	round       DiagnosticParserCoreDispatchRound
	branchOrder uint64
	nextSeq     uint64
}

func (e diagnosticParserCoreConflictExecution) arm(ordinal int) []diagnosticParserCoreHeader {
	if ordinal < 0 || ordinal >= len(e.armRanges) {
		return nil
	}
	arm := e.armRanges[ordinal]
	return e.outputs[arm.start:arm.end]
}

type diagnosticParserCoreConflictArmRange struct {
	start int
	end   int
}

type diagnosticParserCoreConflictScratch struct {
	busy             bool
	actionOutputs    []diagnosticParserCoreActionOutput
	reductionOutputs []core.ReductionOutput
	outputs          []diagnosticParserCoreHeader
	armRanges        []diagnosticParserCoreConflictArmRange
	adopted          []int
	headerAssembly   []diagnosticParserCoreHeader
}

func (s *diagnosticParserCoreConflictScratch) begin(actionCount int) error {
	if s == nil {
		return errors.New("parser-core phase zero: nil conflict scratch")
	}
	if s.busy {
		return errors.New("parser-core phase zero: reentrant conflict scratch")
	}
	s.busy = true
	s.actionOutputs = s.actionOutputs[:0]
	s.reductionOutputs = s.reductionOutputs[:0]
	clear(s.outputs)
	s.outputs = s.outputs[:0]
	if cap(s.armRanges) < actionCount {
		s.armRanges = make([]diagnosticParserCoreConflictArmRange, actionCount)
	} else {
		s.armRanges = s.armRanges[:actionCount]
		clear(s.armRanges)
	}
	if cap(s.adopted) < actionCount {
		s.adopted = make([]int, actionCount)
	} else {
		s.adopted = s.adopted[:actionCount]
		clear(s.adopted)
	}
	clear(s.headerAssembly)
	s.headerAssembly = s.headerAssembly[:0]
	return nil
}

func (s *diagnosticParserCoreConflictScratch) finish() {
	if s == nil {
		return
	}
	clear(s.actionOutputs)
	s.actionOutputs = s.actionOutputs[:0]
	clear(s.reductionOutputs)
	s.reductionOutputs = s.reductionOutputs[:0]
	clear(s.outputs)
	s.outputs = s.outputs[:0]
	clear(s.armRanges)
	s.armRanges = s.armRanges[:0]
	clear(s.adopted)
	s.adopted = s.adopted[:0]
	clear(s.headerAssembly)
	s.headerAssembly = s.headerAssembly[:0]
	s.busy = false
}

type diagnosticParserCoreActionOutput struct {
	head             core.Head
	freshness        core.ReductionFreshness
	cleanPathRank    core.CleanPathRankSelection
	cleanPathLineage uint16
	cleanPathSet     core.AlternativeSet
	cleanPathBlended bool
	dropCohortRefs   core.DropCohortRefSet
}

func executeDiagnosticParserCoreGenericConflictDetailed(
	compact *core.Core,
	owner core.SchedulerTransactionToken,
	incoming diagnosticParserCoreHeader,
	headerIndex int,
	token Token,
	classified core.ClassifiedBoundary,
	branchOrder uint64,
	nextCleanPathLineage *uint16,
	captureLexerSkippedPrefixProvenance bool,
	reductionCost core.ReductionOutputCostFunc,
	allowRepetitionFork bool,
	collectReceipts bool,
	scratch *diagnosticParserCoreConflictScratch,
) (diagnosticParserCoreConflictExecution, error) {
	actions := classified.Actions()
	if scratch == nil || !scratch.busy || len(scratch.armRanges) != actions.Len() {
		return diagnosticParserCoreConflictExecution{}, errors.New("parser-core phase zero: conflict scratch is not initialized")
	}
	var before DiagnosticParserCoreHeaderReceipt
	if collectReceipts {
		var err error
		before, err = diagnosticParserCoreHeaderReceipt(compact, incoming)
		if err != nil {
			return diagnosticParserCoreConflictExecution{}, err
		}
	}
	if err := validateDiagnosticParserCoreCellWithRepetitionFork(token, actions, allowRepetitionFork); err != nil {
		return diagnosticParserCoreConflictExecution{}, err
	}
	if actions.Len() < 2 {
		return diagnosticParserCoreConflictExecution{}, errors.New("parser-core phase zero: conflict executor requires multiple actions")
	}
	// See Core.reduceConflictContext (core.go): every arm applied below --
	// both the fork.Present secondaries and the fork.Present==false primary
	// -- runs while this dispatch point had more than one viable action, so
	// every subtreeRecord any of them reduce is fragile. Reset unconditionally
	// on every exit path, including error returns from RunSchedulerOwned.
	compact.SetReduceConflictContext(true)
	defer compact.SetReduceConflictContext(false)
	if err := compact.SetDropCohortSelectionContextOwned(owner, core.DropCohortSelectionConflictPolicy); err != nil {
		return diagnosticParserCoreConflictExecution{}, err
	}
	defer func() {
		_ = compact.SetDropCohortSelectionContextOwned(owner, core.DropCohortSelectionNone)
	}()
	if token.NoLookahead {
		compact.SetReduceNoLookaheadContext(true)
		defer compact.SetReduceNoLookaheadContext(false)
	}
	secondaryCount := uint64(actions.Len() - 1)
	if secondaryCount > math.MaxUint64-branchOrder {
		return diagnosticParserCoreConflictExecution{}, errors.New("parser-core phase zero: conflict branch order overflow")
	}
	trialOrder := branchOrder
	var receipts []DiagnosticParserCoreRoundAction
	err := compact.RunSchedulerOwned(owner, func() error {
		for ordinal := 1; ordinal < actions.Len(); ordinal++ {
			action := actions.At(ordinal)
			trialOrder++
			var applyErr error
			scratch.actionOutputs, scratch.reductionOutputs, applyErr = applyParserCoreConflictActionInto(
				scratch.actionOutputs[:0], scratch.reductionOutputs[:0], compact, owner, classified, token,
				action, ordinal, core.ForkOrder{Present: true, Value: trialOrder}, nextCleanPathLineage,
				captureLexerSkippedPrefixProvenance,
				reductionCost,
			)
			if applyErr != nil {
				return applyErr
			}
			start := len(scratch.outputs)
			for _, output := range scratch.actionOutputs {
				secondary := incoming
				secondary.head = output.head
				secondary.shifted = action.Type == core.ActionShift
				secondary.freshness = output.freshness
				secondary.convergedReductionSplit = secondary.convergedReductionSplit || output.cleanPathLineage != 0
				applyDiagnosticParserCoreCleanPathOutput(&secondary, output.cleanPathRank, output.cleanPathLineage)
				if output.cleanPathSet.Len() != 0 {
					// Conflict-arm application is fold-class (spec.b4b-
					// alternative-set.v2 section 3.4): secondary starts as a
					// copy of incoming's own already-accumulated history,
					// and output.cleanPathSet is this mutually exclusive
					// arm's own independently established set -- two
					// separately tracked histories, not one popped cone's
					// uniform extension.
					incomparable := compact.AlternativeSetIncomparable(secondary.altSet, output.cleanPathSet)
					compact.UnionAlternativeSet(&secondary.altSet, output.cleanPathSet)
					secondary.blended = secondary.blended || output.cleanPathBlended || incomparable
				}
				if !output.dropCohortRefs.Empty() || output.dropCohortRefs.Overflowed() || output.dropCohortRefs.Blended() {
					if _, err := compact.UnionDropCohortRefsChecked(&secondary.dropCohortRefs, output.dropCohortRefs); err != nil {
						return err
					}
				}
				if action.Type == core.ActionShift {
					markDiagnosticParserCoreExternalLineage(&secondary, token)
				}
				scratch.outputs = append(scratch.outputs, secondary)
			}
			scratch.armRanges[ordinal] = diagnosticParserCoreConflictArmRange{start: start, end: len(scratch.outputs)}
			if collectReceipts {
				receipts = append(receipts, DiagnosticParserCoreRoundAction{
					HeaderIndex: headerIndex, State: before.State, ByteOffset: before.ByteOffset,
					Ordinal: ordinal, Action: rootParserCoreAction(action), BranchOrder: trialOrder,
				})
			}
		}
		primaryAction := actions.At(0)
		var applyErr error
		scratch.actionOutputs, scratch.reductionOutputs, applyErr = applyParserCoreConflictActionInto(
			scratch.actionOutputs[:0], scratch.reductionOutputs[:0], compact, owner, classified, token,
			primaryAction, 0, core.ForkOrder{}, nextCleanPathLineage,
			captureLexerSkippedPrefixProvenance,
			reductionCost,
		)
		if applyErr != nil {
			return applyErr
		}
		start := len(scratch.outputs)
		for _, output := range scratch.actionOutputs {
			primary := incoming
			primary.head = output.head
			primary.shifted = primaryAction.Type == core.ActionShift
			primary.freshness = output.freshness
			primary.convergedReductionSplit = primary.convergedReductionSplit || output.cleanPathLineage != 0
			applyDiagnosticParserCoreCleanPathOutput(&primary, output.cleanPathRank, output.cleanPathLineage)
			if output.cleanPathSet.Len() != 0 {
				// See the secondary loop's identical fold-class comment above.
				incomparable := compact.AlternativeSetIncomparable(primary.altSet, output.cleanPathSet)
				compact.UnionAlternativeSet(&primary.altSet, output.cleanPathSet)
				primary.blended = primary.blended || output.cleanPathBlended || incomparable
			}
			if !output.dropCohortRefs.Empty() || output.dropCohortRefs.Overflowed() || output.dropCohortRefs.Blended() {
				if _, err := compact.UnionDropCohortRefsChecked(&primary.dropCohortRefs, output.dropCohortRefs); err != nil {
					return err
				}
			}
			if primaryAction.Type == core.ActionShift {
				markDiagnosticParserCoreExternalLineage(&primary, token)
			}
			scratch.outputs = append(scratch.outputs, primary)
		}
		scratch.armRanges[0] = diagnosticParserCoreConflictArmRange{start: start, end: len(scratch.outputs)}
		if collectReceipts {
			receipts = append(receipts, DiagnosticParserCoreRoundAction{
				HeaderIndex: headerIndex, State: before.State, ByteOffset: before.ByteOffset,
				Ordinal: 0, Action: rootParserCoreAction(primaryAction),
			})
		}
		return nil
	})
	if err != nil {
		return diagnosticParserCoreConflictExecution{}, err
	}

	var round DiagnosticParserCoreDispatchRound
	if collectReceipts {
		round.Actions = receipts
	}
	return diagnosticParserCoreConflictExecution{
		outputs: scratch.outputs, armRanges: scratch.armRanges,
		round: round, branchOrder: trialOrder,
	}, nil
}

type diagnosticParserCorePhaseHead struct {
	head         core.Head
	checkpoint   core.CheckpointID
	shifted      bool
	accepted     bool
	versionState *diagnosticParserCoreVersionState
}

// clearDiagnosticParserCoreHeaderSuffix clears header values that no longer
// belong to a logical slice. Keep the range bounded by the prior length.
func clearDiagnosticParserCoreHeaderSuffix(headers []diagnosticParserCoreHeader, keep int) {
	if keep < 0 {
		keep = 0
	}
	if keep >= len(headers) {
		return
	}
	clear(headers[keep:])
}

type diagnosticParserCoreCanonicalScratch struct {
	headerBuffers       [2][]diagnosticParserCoreHeader
	inlineHeaders       [2][diagnosticParserCoreLinearCanonicalLimit]diagnosticParserCoreHeader
	nextBuffer          uint8
	keys                []diagnosticParserCorePhaseHead
	inlineKeys          [diagnosticParserCoreLinearCanonicalLimit]diagnosticParserCorePhaseHead
	groups              map[diagnosticParserCorePhaseHead]diagnosticParserCoreCanonicalGroup
	groupsBucketCount   uint64
	groupsBucketBytes   uint64
	groupsRetainedBytes uint64
	// versionStateEqual selects a prior semantic representative for each
	// canonical key. The representative keeps the comparable key compact and
	// avoids hashing mutable snapshot slices.
	versionStateEqual func(left, right *diagnosticParserCoreVersionState) bool
}

const (
	diagnosticParserCoreMapBucketCount = 8
	diagnosticParserCoreMapLoadNum     = 13
	diagnosticParserCoreMapLoadDen     = 2
)

func diagnosticParserCoreCanonicalGroupsBucketBytes(entries int) (uint64, uint64) {
	if entries <= 0 {
		return 0, 0
	}
	buckets := uint64(1)
	for buckets*diagnosticParserCoreMapLoadNum < uint64(entries)*diagnosticParserCoreMapLoadDen {
		buckets <<= 1
	}
	keyBytes := uint64(unsafe.Sizeof(diagnosticParserCorePhaseHead{})) * diagnosticParserCoreMapBucketCount
	valueBytes := uint64(unsafe.Sizeof(diagnosticParserCoreCanonicalGroup{})) * diagnosticParserCoreMapBucketCount
	metadataBytes := uint64(diagnosticParserCoreMapBucketCount) // tophash
	overflowPointerBytes := uint64(unsafe.Sizeof(uintptr(0)))
	base := metadataBytes + keyBytes + valueBytes + overflowPointerBytes
	alignment := uint64(unsafe.Alignof(uintptr(0)))
	base = (base + alignment - 1) / alignment * alignment
	// Estimate each bucket as eight tophash bytes, eight keys, eight values,
	// one overflow pointer, and alignment padding. Round capacity by the
	// documented load-factor target, then reserve one overflow bucket for each
	// two primary buckets. This conservative overcount avoids runtime map
	// internals and covers overflow chains and metadata retained during growth.
	overflowBuckets := (buckets + 1) / 2
	bucketBytes := base * (buckets + overflowBuckets)
	return buckets, bucketBytes
}

func (s *diagnosticParserCoreCanonicalScratch) observeGroupsInsertion(entries int) {
	if s == nil || entries <= 0 {
		return
	}
	buckets, estimate := diagnosticParserCoreCanonicalGroupsBucketBytes(entries)
	if estimate == 0 {
		return
	}
	if buckets > s.groupsBucketCount {
		// A grow retains the old bucket array while the new array is live.
		retained := estimate + s.groupsBucketBytes
		if retained > s.groupsRetainedBytes {
			s.groupsRetainedBytes = retained
		}
		s.groupsBucketCount = buckets
		s.groupsBucketBytes = estimate
	} else if estimate > s.groupsRetainedBytes {
		s.groupsRetainedBytes = estimate
	}
}

// Field order groups winner (8-byte aligned int) and altSet (4-byte
// aligned) up front, then the byte/uint16-sized fields; the sole
// construction site (canonicalizeLinear/canonicalizeMapped) uses keyed
// fields, so this reorder is layout-only (b4b-width-repair audit, 2026-08).
type diagnosticParserCoreCanonicalGroup struct {
	winner           int
	dropCohortRefs   core.DropCohortRefSet
	altSet           core.AlternativeSet
	frontierSequence uint32
	cleanPathLineage uint16
	cleanPathRank    core.CleanPathRankSelection
	flags            uint8
}

const (
	diagnosticParserCoreCanonicalGroupRunnable uint8 = 1 << iota
	diagnosticParserCoreCanonicalGroupConvergedReductionSplit
	diagnosticParserCoreCanonicalGroupResurrectionUnproved
	diagnosticParserCoreCanonicalGroupBlended
)

func (group *diagnosticParserCoreCanonicalGroup) setFlag(flag uint8, value bool) {
	if value {
		group.flags |= flag
	} else {
		group.flags &^= flag
	}
}

func (group diagnosticParserCoreCanonicalGroup) hasFlag(flag uint8) bool {
	return group.flags&flag != 0
}

func diagnosticParserCoreCanonicalGroupFlags(header *diagnosticParserCoreHeader) uint8 {
	var flags uint8
	if !header.paused {
		flags |= diagnosticParserCoreCanonicalGroupRunnable
	}
	if header.convergedReductionSplit {
		flags |= diagnosticParserCoreCanonicalGroupConvergedReductionSplit
	}
	if header.resurrectionUnproved {
		flags |= diagnosticParserCoreCanonicalGroupResurrectionUnproved
	}
	if header.blended {
		flags |= diagnosticParserCoreCanonicalGroupBlended
	}
	return flags
}

const diagnosticParserCoreLinearCanonicalLimit = 8

func (s *diagnosticParserCoreCanonicalScratch) headerBufferFor(target, length int) []diagnosticParserCoreHeader {
	previous := s.headerBuffers[target]
	if length < len(previous) {
		clearDiagnosticParserCoreHeaderSuffix(previous, length)
	}
	if cap(previous) < length {
		// The previous logical contents become unreachable from this buffer when
		// it grows into a different backing array.
		clear(previous)
		if length <= len(s.inlineHeaders[target]) {
			return s.inlineHeaders[target][:length]
		}
		return make([]diagnosticParserCoreHeader, length)
	}
	return previous[:length]
}

func (s *diagnosticParserCoreCanonicalScratch) canonicalize(compact *core.Core, headers []diagnosticParserCoreHeader) ([]diagnosticParserCoreHeader, error) {
	out, _, err := s.canonicalizeWithMutation(compact, headers)
	return out, err
}

func (s *diagnosticParserCoreCanonicalScratch) canonicalizeWithMutation(
	compact *core.Core, headers []diagnosticParserCoreHeader,
) ([]diagnosticParserCoreHeader, core.DropCohortProducerMutation, error) {
	if s == nil {
		return nil, 0, errors.New("parser-core phase zero: nil canonicalization scratch")
	}
	if s.groups != nil {
		clear(s.groups)
	}
	clear(s.keys)
	s.keys = s.keys[:0]
	target := int(s.nextBuffer & 1)
	if len(headers) != 0 && cap(s.headerBuffers[target]) != 0 && &headers[0] == &s.headerBuffers[target][:1][0] {
		target ^= 1
	}
	normalized := s.headerBufferFor(target, len(headers))
	copy(normalized, headers)
	s.headerBuffers[target] = normalized
	// Single-head frontiers never reach canonicalizeLinear/canonicalizeMapped,
	// so the per-header canonical group key they group by carries no
	// semantics here -- only the canonical remap (the compact.Boundary +
	// CanonicalBoundary probe) and the freshness reset do. Skip the key-slice
	// sizing and key-struct build entirely on this path; the double-buffer
	// copy above still runs unchanged, since the aliasing check above and on
	// the next call depends on the returned slice living in s.headerBuffers.
	if len(normalized) == 1 {
		clear(s.keys)
		s.keys = s.keys[:0]
		header := &normalized[0]
		state, byteOffset, err := compact.Boundary(header.head)
		if err != nil {
			clear(normalized)
			s.headerBuffers[target] = normalized[:0]
			return nil, 0, err
		}
		if canonical, ok := compact.CanonicalBoundary(state, byteOffset, header.shifted, header.checkpoint); ok {
			header.head = canonical
		}
		header.freshness = 0
		s.nextBuffer = uint8(target ^ 1)
		return normalized, 0, nil
	}
	previousKeys := s.keys
	if cap(s.keys) < len(headers) {
		clear(previousKeys)
		if len(headers) <= len(s.inlineKeys) {
			s.keys = s.inlineKeys[:len(headers)]
		} else {
			s.keys = make([]diagnosticParserCorePhaseHead, len(headers))
		}
	} else {
		if len(previousKeys) > len(headers) {
			clear(previousKeys[len(headers):])
		}
		s.keys = s.keys[:len(headers)]
	}
	for index := range normalized {
		header := &normalized[index]
		state, byteOffset, err := compact.Boundary(header.head)
		if err != nil {
			clear(normalized)
			clear(s.keys)
			s.headerBuffers[target] = normalized[:0]
			s.keys = s.keys[:0]
			return nil, 0, err
		}
		if canonical, ok := compact.CanonicalBoundary(state, byteOffset, header.shifted, header.checkpoint); ok {
			header.head = canonical
		}
		versionState := header.versionState
		if header.versionLexerSnapshot() == nil {
			// Preserve the pre-ownership canonical key. Recovery isolation keeps
			// region-bearing versions separate when that distinction matters.
			versionState = nil
		}
		if versionState != nil && s.versionStateEqual != nil {
			for prior := 0; prior < index; prior++ {
				representative := s.keys[prior].versionState
				if representative != nil && s.versionStateEqual(representative, versionState) {
					versionState = representative
					break
				}
			}
		}
		key := diagnosticParserCorePhaseHead{
			head: header.head, shifted: header.shifted, accepted: header.accepted,
			checkpoint: header.checkpoint, versionState: versionState,
		}
		s.keys[index] = key
	}
	var out []diagnosticParserCoreHeader
	var err error
	var mutation core.DropCohortProducerMutation
	switch {
	case len(normalized) == 0:
		out = normalized
	case len(normalized) <= diagnosticParserCoreLinearCanonicalLimit:
		out, mutation, err = s.canonicalizeLinearCheckedWithMutation(compact, normalized)
	default:
		out, mutation, err = s.canonicalizeMappedCheckedWithMutation(compact, normalized)
	}
	if err != nil {
		clear(normalized)
		clear(s.keys)
		if s.groups != nil {
			clear(s.groups)
		}
		s.headerBuffers[target] = normalized[:0]
		s.keys = s.keys[:0]
		return nil, 0, err
	}
	s.headerBuffers[target] = out
	s.nextBuffer = uint8(target ^ 1)
	return out, mutation, nil
}

func (s *diagnosticParserCoreCanonicalScratch) canonicalizeRecovery(
	compact *core.Core, headers []diagnosticParserCoreHeader,
) ([]diagnosticParserCoreHeader, error) {
	out, _, err := s.canonicalizeRecoveryWithMutation(compact, headers)
	return out, err
}

// canonicalizeRecoveryWithMutation preserves each recovery version until
// acceptance can price it. It does not remap or merge any live header.
// A mixed marked and unmarked frontier therefore remains fail-closed.
func (s *diagnosticParserCoreCanonicalScratch) canonicalizeRecoveryWithMutation(
	compact *core.Core, headers []diagnosticParserCoreHeader,
) ([]diagnosticParserCoreHeader, core.DropCohortProducerMutation, error) {
	if s == nil || compact == nil {
		return nil, 0, errors.New("parser-core phase zero: recovery canonicalization requires scratch and core")
	}
	if s.groups != nil {
		clear(s.groups)
	}
	clear(s.keys)
	s.keys = s.keys[:0]
	target := int(s.nextBuffer & 1)
	if len(headers) != 0 && cap(s.headerBuffers[target]) != 0 && &headers[0] == &s.headerBuffers[target][:1][0] {
		target ^= 1
	}
	normalized := s.headerBufferFor(target, len(headers))
	copy(normalized, headers)
	for index := range normalized {
		if _, _, err := compact.Boundary(normalized[index].head); err != nil {
			clear(normalized)
			s.headerBuffers[target] = normalized[:0]
			return nil, 0, err
		}
		normalized[index].freshness = 0
	}
	s.headerBuffers[target] = normalized
	s.nextBuffer = uint8(target ^ 1)
	return normalized, 0, nil
}

func (s *diagnosticParserCoreCanonicalScratch) canonicalizeLinear(compact *core.Core, normalized []diagnosticParserCoreHeader) []diagnosticParserCoreHeader {
	out, _ := s.canonicalizeLinearChecked(compact, normalized)
	return out
}

func (s *diagnosticParserCoreCanonicalScratch) canonicalizeLinearChecked(compact *core.Core, normalized []diagnosticParserCoreHeader) ([]diagnosticParserCoreHeader, error) {
	out, _, err := s.canonicalizeLinearCheckedWithMutation(compact, normalized)
	return out, err
}

func (s *diagnosticParserCoreCanonicalScratch) canonicalizeLinearCheckedWithMutation(compact *core.Core, normalized []diagnosticParserCoreHeader) ([]diagnosticParserCoreHeader, core.DropCohortProducerMutation, error) {
	type linearGroup struct {
		keyIndex int
		diagnosticParserCoreCanonicalGroup
	}
	var groups [diagnosticParserCoreLinearCanonicalLimit]linearGroup
	groupCount := 0
	merged := false
	for index := range normalized {
		header := &normalized[index]
		groupIndex := -1
		for candidate := 0; candidate < groupCount; candidate++ {
			if s.keys[groups[candidate].keyIndex] == s.keys[index] {
				groupIndex = candidate
				break
			}
		}
		if groupIndex < 0 {
			groups[groupCount] = linearGroup{
				keyIndex: index,
				diagnosticParserCoreCanonicalGroup: diagnosticParserCoreCanonicalGroup{
					winner: index, frontierSequence: header.frontierSequence,
					cleanPathRank: header.cleanPathRank, cleanPathLineage: header.cleanPathLineage,
					altSet:         header.altSet,
					dropCohortRefs: header.dropCohortRefs,
					flags:          diagnosticParserCoreCanonicalGroupFlags(header),
				},
			}
			groupCount++
			continue
		}
		merged = true
		group := &groups[groupIndex].diagnosticParserCoreCanonicalGroup
		group.setFlag(diagnosticParserCoreCanonicalGroupRunnable, group.hasFlag(diagnosticParserCoreCanonicalGroupRunnable) || !header.paused)
		group.setFlag(diagnosticParserCoreCanonicalGroupConvergedReductionSplit, group.hasFlag(diagnosticParserCoreCanonicalGroupConvergedReductionSplit) || header.convergedReductionSplit)
		group.setFlag(diagnosticParserCoreCanonicalGroupResurrectionUnproved, group.hasFlag(diagnosticParserCoreCanonicalGroupResurrectionUnproved) || header.resurrectionUnproved)
		group.cleanPathRank, group.cleanPathLineage = mergeDiagnosticParserCoreCleanPathLineage(
			group.cleanPathRank,
			group.cleanPathLineage,
			header.cleanPathRank,
			header.cleanPathLineage,
		)
		// Union, never poison: the group's alternative set accumulates every
		// member any header folding into this canonical group carries, even
		// when the scalar pair above disagrees and resets to Unknown/0
		// (spec.b4b-alternative-set.v1 section 4). Fold-class union
		// (spec.b4b-alternative-set.v2 section 3.4): the group's blended mark
		// becomes true when header was already blended, or when the group's
		// accumulated set and header's set are incomparable under
		// containment -- computed before the union mutates group.altSet.
		if header.altSet.Len() != 0 {
			incomparable := compact.AlternativeSetIncomparable(group.altSet, header.altSet)
			compact.UnionAlternativeSet(&group.altSet, header.altSet)
			group.setFlag(diagnosticParserCoreCanonicalGroupBlended, group.hasFlag(diagnosticParserCoreCanonicalGroupBlended) || header.blended || incomparable)
		}
		if !header.dropCohortRefs.Empty() || header.dropCohortRefs.Overflowed() || header.dropCohortRefs.Blended() {
			if _, err := compact.UnionDropCohortRefsChecked(&group.dropCohortRefs, header.dropCohortRefs); err != nil {
				return nil, 0, err
			}
		}
		group.frontierSequence = mergeDiagnosticParserCoreFrontier(group.frontierSequence, header.frontierSequence)
		if diagnosticParserCoreCanonicalCandidateWins(&normalized[group.winner], header) {
			group.winner = index
		}
	}
	write := 0
	for index := range normalized {
		header := &normalized[index]
		for groupIndex := 0; groupIndex < groupCount; groupIndex++ {
			group := groups[groupIndex].diagnosticParserCoreCanonicalGroup
			if group.winner != index {
				continue
			}
			header.paused = !group.hasFlag(diagnosticParserCoreCanonicalGroupRunnable)
			header.freshness = 0
			header.convergedReductionSplit = group.hasFlag(diagnosticParserCoreCanonicalGroupConvergedReductionSplit)
			header.resurrectionUnproved = group.hasFlag(diagnosticParserCoreCanonicalGroupResurrectionUnproved)
			header.cleanPathRank = group.cleanPathRank
			header.cleanPathLineage = group.cleanPathLineage
			header.altSet = group.altSet
			header.dropCohortRefs = group.dropCohortRefs
			header.frontierSequence = group.frontierSequence
			header.blended = group.hasFlag(diagnosticParserCoreCanonicalGroupBlended)
			if write != index {
				normalized[write] = *header
			}
			write++
			break
		}
	}
	clearDiagnosticParserCoreHeaderSuffix(normalized, write)
	if write < len(s.keys) {
		clear(s.keys[write:])
	}
	s.keys = s.keys[:write]
	var mutation core.DropCohortProducerMutation
	if merged {
		mutation = core.DropCohortProducerLinearCanonicalization
	}
	return normalized[:write], mutation, nil
}

func (s *diagnosticParserCoreCanonicalScratch) canonicalizeMapped(compact *core.Core, normalized []diagnosticParserCoreHeader) []diagnosticParserCoreHeader {
	out, _ := s.canonicalizeMappedChecked(compact, normalized)
	return out
}

func (s *diagnosticParserCoreCanonicalScratch) canonicalizeMappedChecked(compact *core.Core, normalized []diagnosticParserCoreHeader) ([]diagnosticParserCoreHeader, error) {
	out, _, err := s.canonicalizeMappedCheckedWithMutation(compact, normalized)
	return out, err
}

func (s *diagnosticParserCoreCanonicalScratch) canonicalizeMappedCheckedWithMutation(compact *core.Core, normalized []diagnosticParserCoreHeader) ([]diagnosticParserCoreHeader, core.DropCohortProducerMutation, error) {
	// The capacity hint can reserve buckets before unique insertions occur.
	s.observeGroupsInsertion(len(normalized))
	if s.groups == nil {
		s.groups = make(map[diagnosticParserCorePhaseHead]diagnosticParserCoreCanonicalGroup, len(normalized))
	} else {
		clear(s.groups)
	}
	merged := false
	for index := range normalized {
		header := &normalized[index]
		key := s.keys[index]
		group, duplicate := s.groups[key]
		if !duplicate {
			group.winner = index
			group.frontierSequence = header.frontierSequence
			group.flags = diagnosticParserCoreCanonicalGroupFlags(header)
			s.observeGroupsInsertion(len(s.groups) + 1)
		} else {
			merged = true
			group.frontierSequence = mergeDiagnosticParserCoreFrontier(group.frontierSequence, header.frontierSequence)
		}
		group.setFlag(diagnosticParserCoreCanonicalGroupRunnable, group.hasFlag(diagnosticParserCoreCanonicalGroupRunnable) || !header.paused)
		group.setFlag(diagnosticParserCoreCanonicalGroupConvergedReductionSplit, group.hasFlag(diagnosticParserCoreCanonicalGroupConvergedReductionSplit) || header.convergedReductionSplit)
		group.setFlag(diagnosticParserCoreCanonicalGroupResurrectionUnproved, group.hasFlag(diagnosticParserCoreCanonicalGroupResurrectionUnproved) || header.resurrectionUnproved)
		group.cleanPathRank, group.cleanPathLineage = mergeDiagnosticParserCoreCleanPathLineage(
			group.cleanPathRank,
			group.cleanPathLineage,
			header.cleanPathRank,
			header.cleanPathLineage,
		)
		// See canonicalizeLinear's identical fold-class comment.
		if header.altSet.Len() != 0 {
			incomparable := compact.AlternativeSetIncomparable(group.altSet, header.altSet)
			compact.UnionAlternativeSet(&group.altSet, header.altSet)
			group.setFlag(diagnosticParserCoreCanonicalGroupBlended, group.hasFlag(diagnosticParserCoreCanonicalGroupBlended) || header.blended || incomparable)
		}
		if !header.dropCohortRefs.Empty() || header.dropCohortRefs.Overflowed() || header.dropCohortRefs.Blended() {
			if _, err := compact.UnionDropCohortRefsChecked(&group.dropCohortRefs, header.dropCohortRefs); err != nil {
				return nil, 0, err
			}
		}
		if diagnosticParserCoreCanonicalCandidateWins(&normalized[group.winner], header) {
			group.winner = index
		}
		s.groups[key] = group
	}
	write := 0
	for index := range normalized {
		header := &normalized[index]
		group := s.groups[s.keys[index]]
		if group.winner != index {
			continue
		}
		header.paused = !group.hasFlag(diagnosticParserCoreCanonicalGroupRunnable)
		header.freshness = 0
		header.convergedReductionSplit = group.hasFlag(diagnosticParserCoreCanonicalGroupConvergedReductionSplit)
		header.resurrectionUnproved = group.hasFlag(diagnosticParserCoreCanonicalGroupResurrectionUnproved)
		header.cleanPathRank = group.cleanPathRank
		header.cleanPathLineage = group.cleanPathLineage
		header.altSet = group.altSet
		header.dropCohortRefs = group.dropCohortRefs
		header.frontierSequence = group.frontierSequence
		header.blended = group.hasFlag(diagnosticParserCoreCanonicalGroupBlended)
		if write != index {
			normalized[write] = *header
		}
		write++
	}
	clearDiagnosticParserCoreHeaderSuffix(normalized, write)
	if write < len(s.keys) {
		clear(s.keys[write:])
	}
	s.keys = s.keys[:write]
	var mutation core.DropCohortProducerMutation
	if merged {
		mutation = core.DropCohortProducerMappedCanonicalization
	}
	return normalized[:write], mutation, nil
}

func diagnosticParserCoreCanonicalCandidateWins(incumbent, candidate *diagnosticParserCoreHeader) bool {
	incumbentFresh := incumbent.freshness != 0
	candidateFresh := candidate.freshness != 0
	return incumbentFresh && !candidateFresh ||
		incumbentFresh == candidateFresh && incumbent.paused && !candidate.paused
}

func canonicalizeDiagnosticParserCoreHeaders(compact *core.Core, headers []diagnosticParserCoreHeader) ([]diagnosticParserCoreHeader, error) {
	var scratch diagnosticParserCoreCanonicalScratch
	return scratch.canonicalize(compact, headers)
}

func diagnosticParserCoreTerminalPayloadView(id uint32, view core.SubtreeView) DiagnosticParserCoreTerminalPayloadView {
	converted := DiagnosticParserCoreTerminalPayloadView{
		ID: id, Symbol: Symbol(view.Symbol), ProductionID: view.ProductionID,
		DynamicPrecedence: view.DynamicPrecedence, StartByte: view.StartByte, EndByte: view.EndByte,
		Extra: view.Extra, External: view.External, Terminal: view.Terminal,
	}
	for _, child := range view.Children {
		converted.Children = append(converted.Children, uint32(child))
	}
	for _, field := range view.Fields {
		converted.Fields = append(converted.Fields, FieldMapEntry{
			FieldID: FieldID(field.FieldID), ChildIndex: field.ChildIndex, Inherited: field.Inherited,
		})
	}
	for _, alias := range view.Aliases {
		converted.Aliases = append(converted.Aliases, Symbol(alias))
	}
	return converted
}

func diagnosticParserCoreHeaderPaths(compact *core.Core, header diagnosticParserCoreHeader) (DiagnosticParserCoreHeaderPathReceipt, error) {
	receipt, err := diagnosticParserCoreHeaderReceipt(compact, header)
	if err != nil {
		return DiagnosticParserCoreHeaderPathReceipt{}, err
	}
	out := DiagnosticParserCoreHeaderPathReceipt{Header: receipt}
	paths, err := compact.Derivations(header.head)
	if errors.Is(err, core.ErrDerivationEnumerationCap) {
		out.DerivationsTruncated = true
		return out, nil
	}
	if err != nil {
		return DiagnosticParserCoreHeaderPathReceipt{}, err
	}
	for _, path := range paths {
		out.Derivations = append(out.Derivations, DiagnosticParserCorePackedDerivation{
			Score: path.Score, BranchOrder: path.BranchOrder, HasBranchOrder: path.HasBranchOrder,
		})
	}
	sort.Slice(out.Derivations, func(i, j int) bool {
		if out.Derivations[i].Score != out.Derivations[j].Score {
			return out.Derivations[i].Score < out.Derivations[j].Score
		}
		return out.Derivations[i].BranchOrder < out.Derivations[j].BranchOrder
	})
	return out, nil
}

func diagnosticParserCoreHeaderPathReceipts(compact *core.Core, headers []diagnosticParserCoreHeader) ([]DiagnosticParserCoreHeaderPathReceipt, error) {
	out := make([]DiagnosticParserCoreHeaderPathReceipt, len(headers))
	for index, header := range headers {
		receipt, err := diagnosticParserCoreHeaderPaths(compact, header)
		if err != nil {
			return nil, err
		}
		out[index] = receipt
	}
	return out, nil
}

// diagnosticParserCoreTokenCell caches the epoch's elected token identity for
// the forced-reuse tranche: the token, the lexer-mode identity it was lexed
// under, and its scanner checkpoints. One cell per election; nothing reads it
// yet.
type diagnosticParserCoreTokenCell struct {
	token            Token
	state            StateID
	byteOffset       uint32
	beforeCheckpoint core.CheckpointID
	afterCheckpoint  core.CheckpointID
	valid            bool
}

// diagnosticParserCoreVersionLexerRequest owns one header's current
// lookahead. The scheduler keeps this sidecar outside the pinned header so a
// different token width never changes the compact cell layout.
type diagnosticParserCoreVersionLexerRequest struct {
	electionIndex     int
	headerCreationSeq uint64
	state             StateID
	token             Token
	before            *diagnosticParserCoreVersionLexerSnapshot
	after             *diagnosticParserCoreVersionLexerSnapshot
	beforeCheckpoint  DiagnosticParserCoreScannerCheckpoint
	afterCheckpoint   DiagnosticParserCoreScannerCheckpoint
	beforeID          core.CheckpointID
	afterID           core.CheckpointID
	// raggedSpan marks the activation request whose token ends at a
	// different byte than the shared election. Count it only after the
	// owning header shifts it.
	raggedSpan bool
	valid      bool
}

type diagnosticParserCoreGenericScheduler struct {
	compact                         *core.Core
	tokenSource                     *dfaTokenSource
	scannerScratch                  *[]byte
	headers                         []diagnosticParserCoreHeader
	token                           Token
	checkpoint                      DiagnosticParserCoreScannerCheckpoint
	checkpointBeforeID              core.CheckpointID
	checkpointID                    core.CheckpointID
	currentElection                 DiagnosticParserCoreElection
	tokenCell                       diagnosticParserCoreTokenCell
	versionLexerBefore              dfaRelexSnapshot
	versionLexerBeforeScratch       dfaRelexSnapshotScratch
	versionLexerBeforeValid         bool
	versionLexerBeforeIdentity      [32]byte
	versionLexerBeforeIdentityValid bool
	versionLexerBeforeElection      int
	versionLexerBeforeCheckpoint    core.CheckpointID
	versionLexerRequests            []diagnosticParserCoreVersionLexerRequest
	// versionLexerOwnershipActive marks the ragged election that switched this
	// scheduler from its shared-token fast path to owned per-header cursors.
	// versionLexerOwnershipActive keeps each live header on its own DFA and
	// scanner cursor until one version remains and rejoins shared election.
	versionLexerOwnershipActive bool
	// versionLexerNoActionProof is true only during one exact C-style drop:
	// owned versions shared a token start, at least one shifted, and the
	// remaining versions exhausted reductions without an action.
	versionLexerNoActionProof bool
	electionIndex             int
	noLookaheadSteps          uint8
	// recoveryIsolation becomes true only after S4 or S5 publishes two versions.
	// Clean parses retain the canonical scheduler fast path.
	recoveryIsolation bool
	// s3RegionOpened records one S3 recovery episode across this parse. A later
	// episode starts a new C recovery election that this route cannot model.
	s3RegionOpened bool
	// s3ResumeState and s3ResumeSymbol record the sole strategy-2 resume. The
	// accepted-tree audit uses them only with an exact artifact rule.
	s3ResumeState  StateID
	s3ResumeSymbol Symbol
	s3ResumeCount  uint32
	// selectedRecoveryAbsorbLineage records that EOF pricing selected S5's
	// original, earlier absorb version. Materialization uses this outcome to
	// prevent the later missing version from borrowing an absorb-resume rule.
	selectedRecoveryAbsorbLineage bool
	// s5MissingInsertions counts recovery forks created by S5. The counter
	// bounds zero-width progress across one parse.
	s5MissingInsertions   uint32
	tokens                uint64
	dispatches            uint64
	branchOrder           uint64
	nextSeq               uint64
	nextCleanPathLineage  uint16
	options               DiagnosticParserCorePrefixOptions
	receiptBacking        DiagnosticParserCoreGenericScheduler
	receipt               *DiagnosticParserCoreGenericScheduler
	summaryHeaderScratch  []DiagnosticParserCoreHeaderReceipt
	headerRollbackScratch diagnosticParserCoreHeaderRollbackScratch
	canonicalScratch      diagnosticParserCoreCanonicalScratch
	// footprintRefs is reusable poll scratch. It is cleared after every
	// footprint calculation so the retained backing array owns no state.
	footprintRefs                []diagnosticParserCoreFootprintRef
	dispatchScratch              diagnosticParserCoreDispatchScratch
	conflictScratch              diagnosticParserCoreConflictScratch
	reductionOutputs             []core.ReductionOutput
	reductionReplacements        []diagnosticParserCoreHeader
	recoveryCondenseScratch      []diagnosticParserCoreRecoveryCondenseEntry
	recoveryCondenseOrderScratch []int
	classifiedBoundaries         []core.ClassifiedBoundary
	condenseCandidates           []core.CondenseCandidate
	electStates                  []StateID
	electGLRStates               []StateID
	work                         DiagnosticParserCoreGenericWork
	epochProgress                bool
	acceptedHead                 core.Head
	acceptedPayloads             []core.SubtreeID
	// acceptedRootFinalization is a scheduler sidecar. Keeping it outside the
	// fixed header preserves the 224-byte scheduler-header contract.
	acceptedRootFinalization   diagnosticParserCoreRootFinalization
	eofRecoveryAdmission       compactEOFRecoveryAdmissionReceipt
	conflictPostExecutionFault func() error
	extraPostExecutionFault    func() error
	freshSessionOwner          *core.SchedulerTransactionToken
	verifierHeads              [32]core.Head
	verifierRefs               [32]core.DropCohortRef
	verifierBound              int
	verifierHeaderPtr          *diagnosticParserCoreHeader
	verifierInvocation         uint64
	// frontierProofVerified binds one successful D6b proof to the immediately
	// following no-action drop. The state is reset with the scheduler and is
	// consumed by dropGenericNoActionHeads.
	frontierProofVerified         bool
	frontierProofDropCount        uint8
	frontierProofDropIndices      [diagnosticParserCoreFrontierParticipantCap]int
	observer                      diagnosticParserCoreSeedObserver
	stoppedAfterElection          bool
	requireEOFPostNoLookaheadRoot bool
	seedHeaders                   [1]diagnosticParserCoreHeader
	// corridor is the compiled C4 bytecode program for this parse's language,
	// or nil when the corridor lane is off or the grammar did not compile
	// (spec.c4-bytecode-isa.v1 section 6.2). corridorCells is the lane's own
	// singleton dispatch-cell buffer, so the corridor never touches the
	// generic pass's dispatch scratch.
	corridor *ParserCoreCorridorProgram
	// corridorRows is the shared converted action-row table, indexed by the
	// action-row index every executable corridor body carries. It is the same
	// immutable slice the compact core's TableView reads, so the corridor and
	// the generic lane resolve one cell to one row.
	corridorRows  []core.ActionRow
	corridorCells [1]diagnosticParserCoreGenericCell
	capPressure   diagnosticParserCoreCapPressurePrediction
}

const (
	compactEOFRecoveryAdmissionSourceChunkBytes = 4096
	compactEOFRecoveryAdmissionChildGroupSize   = core.EOFAdmissionMetadataGroupSize
	compactEOFRecoveryAdmissionMaxTopPayloads   = core.EOFAdmissionMaxTopPayloads
	compactEOFRecoveryAdmissionMaxDepth         = 256
	compactEOFRecoveryAdmissionMaxOccurrences   = 65536
)

func (state compactEOFRecoveryAdmissionState) String() string {
	switch state {
	case compactEOFRecoveryAdmissionEmpty:
		return "empty"
	case compactEOFRecoveryAdmissionProduced:
		return "produced"
	case compactEOFRecoveryAdmissionPreApplyValidated:
		return "pre_apply_validated"
	case compactEOFRecoveryAdmissionAcceptApplied:
		return "accept_applied"
	case compactEOFRecoveryAdmissionRecoveryDropped:
		return "recovery_dropped"
	case compactEOFRecoveryAdmissionCompleted:
		return "completed"
	case compactEOFRecoveryAdmissionSchedulerReturned:
		return "scheduler_returned"
	case compactEOFRecoveryAdmissionConsumed:
		return "consumed"
	case compactEOFRecoveryAdmissionInvalid:
		return "invalid"
	default:
		return "unknown"
	}
}

func (kind compactEOFRecoveryAdmissionEventKind) String() string {
	switch kind {
	case compactEOFRecoveryAdmissionEventNormal:
		return "normal"
	case compactEOFRecoveryAdmissionEventRecover:
		return "recover_eof"
	default:
		return "unknown"
	}
}

func (route compactEOFRecoveryAdmissionRoute) String() string {
	switch route {
	case compactEOFRecoveryAdmissionRoutePublicTree:
		return "public_tree"
	case compactEOFRecoveryAdmissionRouteSelectedStore:
		return "selected_store"
	default:
		return "none"
	}
}

func compactEOFRecoveryAdmissionCheckedAdd(left, right uint64) (uint64, bool) {
	if math.MaxUint64-left < right {
		return 0, false
	}
	return left + right, true
}

func compactEOFRecoveryAdmissionCheckedMul(left, right uint64) (uint64, bool) {
	if left != 0 && right > math.MaxUint64/left {
		return 0, false
	}
	return left * right, true
}

func compactEOFRecoveryAdmissionSeal(receipt *compactEOFRecoveryAdmissionReceipt, previous [32]byte) [32]byte {
	if receipt == nil {
		return [32]byte{}
	}
	hasher := sha256.New()
	var scratch [8]byte
	writeUint64 := func(value uint64) {
		binary.LittleEndian.PutUint64(scratch[:], value)
		_, _ = hasher.Write(scratch[:])
	}
	writeBool := func(value bool) {
		if value {
			writeUint64(1)
			return
		}
		writeUint64(0)
	}
	_, _ = hasher.Write(previous[:])
	writeUint64(uint64(receipt.state))
	writeUint64(uint64(receipt.transitionCount))
	writeBool(receipt.active)
	writeBool(receipt.valid)
	writeUint64(receipt.coreGeneration)
	writeUint64(uint64(receipt.electionIndex))
	writeUint64(uint64(receipt.token.Symbol))
	writeUint64(uint64(receipt.token.StartByte))
	writeUint64(uint64(receipt.token.EndByte))
	writeBool(receipt.token.Missing)
	writeBool(receipt.token.NoLookahead)
	writeBool(receipt.token.ExternalScannerToken)
	writeUint64(uint64(receipt.sourceLength))
	_, _ = hasher.Write(receipt.sourceSHA256[:])
	writeUint64(uint64(receipt.normalHead.Node))
	writeUint64(uint64(receipt.recoveryHead.Node))
	writeUint64(receipt.normalCreationSeq)
	writeUint64(receipt.recoveryCreationSeq)
	writeUint64(uint64(receipt.normalLineage))
	writeUint64(uint64(receipt.recoveryLineage))
	_, _ = hasher.Write(receipt.normalFingerprint[:])
	_, _ = hasher.Write(receipt.recoveryFingerprint[:])
	writeUint64(uint64(receipt.normalPayloads))
	writeUint64(uint64(receipt.recoveryPayloads))
	writeUint64(uint64(receipt.normalOccurrences))
	writeUint64(uint64(receipt.recoveryOccurrences))
	writeUint64(uint64(receipt.normalFrontier))
	writeUint64(uint64(receipt.recoveryFrontier))
	for _, event := range receipt.events {
		writeUint64(uint64(event.ordinal))
		writeUint64(uint64(event.kind))
		writeUint64(uint64(event.cost))
		writeUint64(uint64(event.dynamicPrecedence))
	}
	writeUint64(uint64(receipt.selectedEvent))
	writeBool(receipt.metadataOnly)
	writeUint64(receipt.consumptionCount)
	writeUint64(uint64(receipt.constructionRoute))
	writeUint64(uint64(receipt.observedErrorCost))
	work := receipt.work
	for _, value := range [...]uint64{
		work.polls,
		work.sourceChunks,
		work.childGroups,
		work.pathsVisited,
		work.linksVisited,
		work.payloadRecordsVisited,
		work.maxDepth,
		work.bytesInspected,
		work.maxSourceChunk,
		work.maxChildGroup,
		work.checkedArithmetic,
		work.publicationAttempts,
		work.parserConstructions,
		work.treeConstructions,
		work.selectedStoreConstructions,
	} {
		writeUint64(value)
	}
	writeBool(work.overflow)
	var sum [32]byte
	copy(sum[:], hasher.Sum(nil))
	return sum
}

func compactEOFRecoveryAdmissionSealIsValid(receipt *compactEOFRecoveryAdmissionReceipt) bool {
	if receipt == nil || receipt.transitionCount == 0 ||
		int(receipt.transitionCount) > len(receipt.transitionSeals) {
		return false
	}
	var previous [32]byte
	if receipt.transitionCount > 1 {
		previous = receipt.transitionSeals[receipt.transitionCount-2]
	}
	want := compactEOFRecoveryAdmissionSeal(receipt, previous)
	return receipt.seal != ([32]byte{}) && receipt.seal == want &&
		receipt.transitionSeals[receipt.transitionCount-1] == want
}

func compactEOFRecoveryAdmissionTransition(
	receipt *compactEOFRecoveryAdmissionReceipt,
	next compactEOFRecoveryAdmissionState,
) error {
	if receipt == nil || int(receipt.transitionCount) >= len(receipt.transitions) {
		return errors.New("parser-core phase zero: EOF recovery receipt transition overflow")
	}
	previous := receipt.seal
	index := receipt.transitionCount
	receipt.state = next
	receipt.transitions[index] = next
	receipt.transitionCount++
	receipt.seal = compactEOFRecoveryAdmissionSeal(receipt, previous)
	receipt.transitionSeals[index] = receipt.seal
	return nil
}

func compactEOFRecoveryAdmissionInvalidate(
	receipt *compactEOFRecoveryAdmissionReceipt,
	reason string,
) {
	if receipt == nil {
		return
	}
	receipt.valid = false
	receipt.state = compactEOFRecoveryAdmissionInvalid
	receipt.selectedEvent = -1
	receipt.declineReason = reason
}

func compactEOFRecoveryAdmissionAddWork(
	scheduler *diagnosticParserCoreGenericScheduler,
	receipt *compactEOFRecoveryAdmissionReceipt,
	target *uint64,
	amount uint64,
) error {
	if receipt == nil || target == nil {
		return errors.New("parser-core phase zero: EOF recovery work counter is nil")
	}
	if compactEOFRecoveryAdmissionOverflowHook != nil &&
		compactEOFRecoveryAdmissionOverflowHook(scheduler, "counter") {
		receipt.work.overflow = true
		compactEOFRecoveryAdmissionInvalidate(receipt, "EOF recovery admission counter overflow")
		return errors.New("parser-core phase zero: EOF recovery admission counter overflow")
	}
	checked, ok := compactEOFRecoveryAdmissionCheckedAdd(receipt.work.checkedArithmetic, 1)
	if !ok {
		receipt.work.overflow = true
		compactEOFRecoveryAdmissionInvalidate(receipt, "EOF recovery admission arithmetic counter overflow")
		return errors.New("parser-core phase zero: EOF recovery admission arithmetic counter overflow")
	}
	receipt.work.checkedArithmetic = checked
	value, ok := compactEOFRecoveryAdmissionCheckedAdd(*target, amount)
	if !ok {
		receipt.work.overflow = true
		compactEOFRecoveryAdmissionInvalidate(receipt, "EOF recovery admission counter overflow")
		return errors.New("parser-core phase zero: EOF recovery admission counter overflow")
	}
	*target = value
	return nil
}

func compactEOFRecoveryAdmissionAddCost(
	scheduler *diagnosticParserCoreGenericScheduler,
	receipt *compactEOFRecoveryAdmissionReceipt,
	target *uint64,
	amount uint64,
) error {
	if compactEOFRecoveryAdmissionOverflowHook != nil &&
		compactEOFRecoveryAdmissionOverflowHook(scheduler, "cost") {
		receipt.work.overflow = true
		compactEOFRecoveryAdmissionInvalidate(receipt, "EOF recovery admission cost overflow")
		return errors.New("parser-core phase zero: EOF recovery admission cost overflow")
	}
	return compactEOFRecoveryAdmissionAddWork(scheduler, receipt, target, amount)
}

func compactEOFRecoveryAdmissionMultiplyCost(
	receipt *compactEOFRecoveryAdmissionReceipt,
	left uint64,
	right uint64,
) (uint64, error) {
	if receipt == nil {
		return 0, errors.New("parser-core phase zero: EOF recovery cost receipt is nil")
	}
	checked, ok := compactEOFRecoveryAdmissionCheckedAdd(receipt.work.checkedArithmetic, 1)
	if !ok {
		receipt.work.overflow = true
		compactEOFRecoveryAdmissionInvalidate(receipt, "EOF recovery admission arithmetic counter overflow")
		return 0, errors.New("parser-core phase zero: EOF recovery admission arithmetic counter overflow")
	}
	receipt.work.checkedArithmetic = checked
	value, ok := compactEOFRecoveryAdmissionCheckedMul(left, right)
	if !ok {
		receipt.work.overflow = true
		compactEOFRecoveryAdmissionInvalidate(receipt, "EOF recovery admission cost overflow")
		return 0, errors.New("parser-core phase zero: EOF recovery admission cost overflow")
	}
	return value, nil
}

func compactEOFRecoveryAdmissionPoll(
	scheduler *diagnosticParserCoreGenericScheduler,
	receipt *compactEOFRecoveryAdmissionReceipt,
	poll func() error,
) error {
	if err := compactEOFRecoveryAdmissionAddWork(scheduler, receipt, &receipt.work.polls, 1); err != nil {
		return err
	}
	if poll == nil {
		return nil
	}
	if err := poll(); err != nil {
		compactEOFRecoveryAdmissionInvalidate(receipt, "EOF recovery admission poll failed")
		return err
	}
	return nil
}

type compactEOFRecoveryAdmissionPath struct {
	fingerprint       [32]byte
	payloadCount      uint32
	occurrenceCount   uint32
	visibleFrontier   uint32
	metadataGroups    uint32
	polls             uint32
	maxDepth          uint32
	dynamicPrecedence int64
}

type compactEOFRecoveryAdmissionFrame struct {
	payload            core.SubtreeID
	path               [32]byte
	incomingAlias      core.Symbol
	parentStart        uint32
	parentEnd          uint32
	hasParent          bool
	frontierBlocked    bool
	descendantsBlocked bool
	startByte          uint32
	endByte            uint32
	childCount         uint32
	nextChild          uint32
	entered            bool
}

func compactEOFRecoveryAdmissionWriteUint64(hasher interface{ Write([]byte) (int, error) }, value uint64) {
	var scratch [8]byte
	binary.LittleEndian.PutUint64(scratch[:], value)
	_, _ = hasher.Write(scratch[:])
}

func compactEOFRecoveryAdmissionWriteBool(hasher interface{ Write([]byte) (int, error) }, value bool) {
	if value {
		compactEOFRecoveryAdmissionWriteUint64(hasher, 1)
		return
	}
	compactEOFRecoveryAdmissionWriteUint64(hasher, 0)
}

func compactEOFRecoveryAdmissionRootPath(role uint64, ordinal uint32) [32]byte {
	var input [16]byte
	binary.LittleEndian.PutUint64(input[:8], role)
	binary.LittleEndian.PutUint64(input[8:], uint64(ordinal))
	return sha256.Sum256(input[:])
}

func compactEOFRecoveryAdmissionChildPath(parent [32]byte, ordinal uint32) [32]byte {
	var input [40]byte
	copy(input[:32], parent[:])
	binary.LittleEndian.PutUint64(input[32:], uint64(ordinal))
	return sha256.Sum256(input[:])
}

func (s *diagnosticParserCoreGenericScheduler) inspectCompactEOFRecoveryAdmissionSource(
	receipt *compactEOFRecoveryAdmissionReceipt,
	source []byte,
	poll func() error,
) (uint64, error) {
	hasher := sha256.New()
	var newlineCount uint64
	for start := 0; start < len(source); start += compactEOFRecoveryAdmissionSourceChunkBytes {
		if err := compactEOFRecoveryAdmissionPoll(s, receipt, poll); err != nil {
			return 0, err
		}
		end := start + compactEOFRecoveryAdmissionSourceChunkBytes
		if end > len(source) {
			end = len(source)
		}
		chunk := source[start:end]
		if err := compactEOFRecoveryAdmissionAddWork(s, receipt, &receipt.work.sourceChunks, 1); err != nil {
			return 0, err
		}
		if err := compactEOFRecoveryAdmissionAddWork(s, receipt, &receipt.work.bytesInspected, uint64(len(chunk))); err != nil {
			return 0, err
		}
		if uint64(len(chunk)) > receipt.work.maxSourceChunk {
			receipt.work.maxSourceChunk = uint64(len(chunk))
		}
		for _, value := range chunk {
			if value == '\n' {
				newlineCount++
			}
		}
		_, _ = hasher.Write(chunk)
	}
	copy(receipt.sourceSHA256[:], hasher.Sum(nil))
	return newlineCount, nil
}

func (s *diagnosticParserCoreGenericScheduler) inspectCompactEOFRecoveryAdmissionPath(
	receipt *compactEOFRecoveryAdmissionReceipt,
	header diagnosticParserCoreHeader,
	language *Language,
	role uint64,
	sourceLength uint32,
	poll func() error,
) (compactEOFRecoveryAdmissionPath, string, error) {
	var result compactEOFRecoveryAdmissionPath
	if s == nil || s.compact == nil || receipt == nil || language == nil {
		return result, "EOF recovery admission audit context is incomplete", nil
	}
	generation := receipt.coreGeneration
	var topPayloads [compactEOFRecoveryAdmissionMaxTopPayloads]core.SubtreeID
	path, err := s.compact.VisitEOFAdmissionExactPath(
		header.head,
		generation,
		func() error { return compactEOFRecoveryAdmissionPoll(s, receipt, poll) },
		func(ordinal uint32, payload core.SubtreeID) error {
			if ordinal >= compactEOFRecoveryAdmissionMaxTopPayloads {
				return core.ErrEOFAdmissionTopPayloadCap
			}
			topPayloads[ordinal] = payload
			return nil
		},
	)
	if errors.Is(err, core.ErrEOFAdmissionInexactPath) {
		return result, "EOF recovery admission requires one exact derivation", nil
	}
	if errors.Is(err, core.ErrEOFAdmissionTopPayloadCap) {
		return result, "EOF recovery admission top payload cap", nil
	}
	if errors.Is(err, core.ErrEOFAdmissionMalformed) {
		return result, "EOF recovery admission path is malformed", nil
	}
	if err != nil {
		return result, "", err
	}
	if path.Payloads == 0 || path.Payloads > compactEOFRecoveryAdmissionMaxTopPayloads {
		return result, "EOF recovery admission requires one nonempty exact derivation", nil
	}
	result.payloadCount = path.Payloads
	result.dynamicPrecedence = path.Score
	result.polls = path.Polls
	if err := compactEOFRecoveryAdmissionAddWork(s, receipt, &receipt.work.pathsVisited, 1); err != nil {
		return result, "", err
	}
	if err := compactEOFRecoveryAdmissionAddWork(s, receipt, &receipt.work.linksVisited, uint64(path.Links)); err != nil {
		return result, "", err
	}
	topGroups := (uint64(path.Payloads) + compactEOFRecoveryAdmissionChildGroupSize - 1) /
		compactEOFRecoveryAdmissionChildGroupSize
	if err := compactEOFRecoveryAdmissionAddWork(s, receipt, &receipt.work.childGroups, topGroups); err != nil {
		return result, "", err
	}
	result.metadataGroups = uint32(topGroups)
	if group := uint64(path.Payloads); group > compactEOFRecoveryAdmissionChildGroupSize {
		group = compactEOFRecoveryAdmissionChildGroupSize
		if group > receipt.work.maxChildGroup {
			receipt.work.maxChildGroup = group
		}
	} else if group > receipt.work.maxChildGroup {
		receipt.work.maxChildGroup = group
	}

	hasher := sha256.New()
	compactEOFRecoveryAdmissionWriteUint64(hasher, role)
	compactEOFRecoveryAdmissionWriteUint64(hasher, uint64(header.head.Node))
	compactEOFRecoveryAdmissionWriteUint64(hasher, uint64(path.Payloads))
	compactEOFRecoveryAdmissionWriteUint64(hasher, uint64(path.Score))
	compactEOFRecoveryAdmissionWriteUint64(hasher, path.BranchOrder)
	compactEOFRecoveryAdmissionWriteBool(hasher, path.HasBranchOrder)
	for _, capValue := range [...]uint64{
		compactEOFRecoveryAdmissionSourceChunkBytes,
		compactEOFRecoveryAdmissionChildGroupSize,
		compactEOFRecoveryAdmissionMaxTopPayloads,
		compactEOFRecoveryAdmissionMaxDepth,
		compactEOFRecoveryAdmissionMaxOccurrences,
	} {
		compactEOFRecoveryAdmissionWriteUint64(hasher, capValue)
	}

	var frames [compactEOFRecoveryAdmissionMaxDepth]compactEOFRecoveryAdmissionFrame
	for rootOrdinal := uint32(0); rootOrdinal < path.Payloads; rootOrdinal++ {
		frames[0] = compactEOFRecoveryAdmissionFrame{
			payload: topPayloads[rootOrdinal],
			path:    compactEOFRecoveryAdmissionRootPath(role, rootOrdinal),
		}
		depth := 1
		for depth != 0 {
			frame := &frames[depth-1]
			if !frame.entered {
				if result.occurrenceCount >= compactEOFRecoveryAdmissionMaxOccurrences {
					return result, "EOF recovery admission occurrence cap", nil
				}
				if err := compactEOFRecoveryAdmissionAddWork(
					s,
					receipt,
					&receipt.work.payloadRecordsVisited,
					1,
				); err != nil {
					return result, "", err
				}
				result.occurrenceCount++
				if uint32(depth) > result.maxDepth {
					result.maxDepth = uint32(depth)
				}
				if uint64(depth) > receipt.work.maxDepth {
					receipt.work.maxDepth = uint64(depth)
				}

				decline := ""
				err := s.compact.VisitEOFAdmissionSubtree(
					frame.payload,
					generation,
					func(view core.EOFAdmissionSubtreeView) error {
						if view.Identity != frame.payload || view.Generation != generation ||
							!view.MetadataAuthenticated {
							decline = "EOF recovery admission subtree identity is unauthenticated"
							return nil
						}
						if view.Missing {
							decline = "EOF recovery admission rejects a missing subtree"
							return nil
						}
						if view.Extra || view.External {
							decline = "EOF recovery admission rejects extra or external closure"
							return nil
						}
						if view.Symbol == core.RecoveryErrorSymbol || frame.incomingAlias == core.RecoveryErrorSymbol {
							span := uint64(view.EndByte - view.StartByte)
							observed := uint64(core.RecoveryCostPerRecovery + core.RecoveryCostPerSkippedTree)
							if value, ok := compactEOFRecoveryAdmissionCheckedAdd(observed, span); ok {
								observed = value
							} else {
								observed = math.MaxUint32
							}
							if observed > math.MaxUint32 {
								observed = math.MaxUint32
							}
							receipt.observedErrorCost = uint32(observed)
							decline = "EOF recovery admission rejects an existing ERROR payload"
							return nil
						}
						if view.EndByte > sourceLength ||
							frame.hasParent && (view.StartByte < frame.parentStart || view.EndByte > frame.parentEnd) {
							decline = "EOF recovery admission subtree range is malformed"
							return nil
						}
						if view.Terminal && (len(view.Children) != 0 || len(view.Fields) != 0 || len(view.Aliases) != 0) {
							decline = "EOF recovery admission terminal metadata is malformed"
							return nil
						}
						if len(view.Aliases) != 0 && len(view.Aliases) != len(view.Children) {
							decline = "EOF recovery admission alias width is malformed"
							return nil
						}
						if int(view.Symbol) >= len(language.SymbolMetadata) {
							decline = "EOF recovery admission subtree symbol is outside metadata"
							return nil
						}
						aliasedBoundary := frame.incomingAlias != 0
						if aliasedBoundary {
							if int(frame.incomingAlias) >= len(language.SymbolMetadata) {
								decline = "EOF recovery admission alias symbol is outside metadata"
								return nil
							}
						}
						visibleBoundary := aliasedBoundary || language.SymbolMetadata[view.Symbol].Visible
						contributes := !frame.frontierBlocked && visibleBoundary
						if contributes {
							frontier, ok := compactEOFRecoveryAdmissionCheckedAdd(uint64(result.visibleFrontier), 1)
							if !ok || frontier > math.MaxUint32 {
								receipt.work.overflow = true
								decline = "EOF recovery admission visible frontier overflow"
								return nil
							}
							result.visibleFrontier = uint32(frontier)
						}
						frame.descendantsBlocked = frame.frontierBlocked || visibleBoundary
						frame.startByte, frame.endByte = view.StartByte, view.EndByte
						frame.childCount = uint32(len(view.Children))

						_, _ = hasher.Write(frame.path[:])
						for _, value := range [...]uint64{
							uint64(view.Identity),
							uint64(frame.incomingAlias),
							uint64(view.Symbol),
							uint64(view.ProductionID),
							uint64(uint16(view.DynamicPrecedence)),
							uint64(view.StartByte),
							uint64(view.EndByte),
							uint64(len(view.Children)),
							uint64(len(view.Fields)),
							uint64(len(view.Aliases)),
						} {
							compactEOFRecoveryAdmissionWriteUint64(hasher, value)
						}
						compactEOFRecoveryAdmissionWriteBool(hasher, view.Extra)
						compactEOFRecoveryAdmissionWriteBool(hasher, view.External)
						compactEOFRecoveryAdmissionWriteBool(hasher, view.Terminal)
						compactEOFRecoveryAdmissionWriteBool(hasher, view.Fragile)
						compactEOFRecoveryAdmissionWriteBool(hasher, view.Missing)
						compactEOFRecoveryAdmissionWriteBool(hasher, visibleBoundary)
						compactEOFRecoveryAdmissionWriteBool(hasher, contributes)

						for start := 0; start < len(view.Children); start += compactEOFRecoveryAdmissionChildGroupSize {
							if err := compactEOFRecoveryAdmissionPoll(s, receipt, poll); err != nil {
								return err
							}
							result.polls++
							end := start + compactEOFRecoveryAdmissionChildGroupSize
							if end > len(view.Children) {
								end = len(view.Children)
							}
							if err := compactEOFRecoveryAdmissionAddWork(s, receipt, &receipt.work.childGroups, 1); err != nil {
								return err
							}
							result.metadataGroups++
							if group := uint64(end - start); group > receipt.work.maxChildGroup {
								receipt.work.maxChildGroup = group
							}
							for ordinal := start; ordinal < end; ordinal++ {
								child := view.Children[ordinal]
								if child == 0 || child >= view.Identity {
									decline = "EOF recovery admission child identity is malformed"
									return nil
								}
								alias := core.Symbol(0)
								if len(view.Aliases) != 0 {
									alias = view.Aliases[ordinal]
									if alias == core.RecoveryErrorSymbol ||
										alias != 0 && int(alias) >= len(language.SymbolMetadata) {
										decline = "EOF recovery admission alias metadata is malformed"
										return nil
									}
								}
								compactEOFRecoveryAdmissionWriteUint64(hasher, uint64(ordinal))
								compactEOFRecoveryAdmissionWriteUint64(hasher, uint64(child))
								compactEOFRecoveryAdmissionWriteUint64(hasher, uint64(alias))
							}
						}
						for start := 0; start < len(view.Fields); start += compactEOFRecoveryAdmissionChildGroupSize {
							if err := compactEOFRecoveryAdmissionPoll(s, receipt, poll); err != nil {
								return err
							}
							result.polls++
							end := start + compactEOFRecoveryAdmissionChildGroupSize
							if end > len(view.Fields) {
								end = len(view.Fields)
							}
							if err := compactEOFRecoveryAdmissionAddWork(s, receipt, &receipt.work.childGroups, 1); err != nil {
								return err
							}
							result.metadataGroups++
							if group := uint64(end - start); group > receipt.work.maxChildGroup {
								receipt.work.maxChildGroup = group
							}
							for _, field := range view.Fields[start:end] {
								if uint32(field.ChildIndex) >= uint32(len(view.Children)) {
									decline = "EOF recovery admission field metadata is malformed"
									return nil
								}
								compactEOFRecoveryAdmissionWriteUint64(hasher, uint64(field.FieldID))
								compactEOFRecoveryAdmissionWriteUint64(hasher, uint64(field.ChildIndex))
								compactEOFRecoveryAdmissionWriteBool(hasher, field.Inherited)
							}
						}
						return nil
					},
				)
				if errors.Is(err, core.ErrEOFAdmissionMalformed) {
					return result, "EOF recovery admission subtree record is malformed", nil
				}
				if err != nil {
					return result, "", err
				}
				if decline != "" {
					return result, decline, nil
				}
				frame.entered = true
				continue
			}
			if frame.nextChild >= frame.childCount {
				frames[depth-1] = compactEOFRecoveryAdmissionFrame{}
				depth--
				continue
			}
			ordinal := frame.nextChild
			frame.nextChild++
			var child core.SubtreeID
			var alias core.Symbol
			err := s.compact.VisitEOFAdmissionSubtree(
				frame.payload,
				generation,
				func(view core.EOFAdmissionSubtreeView) error {
					if ordinal >= uint32(len(view.Children)) {
						return core.ErrEOFAdmissionMalformed
					}
					child = view.Children[ordinal]
					if len(view.Aliases) != 0 {
						if len(view.Aliases) != len(view.Children) {
							return core.ErrEOFAdmissionMalformed
						}
						alias = view.Aliases[ordinal]
					}
					return nil
				},
			)
			if errors.Is(err, core.ErrEOFAdmissionMalformed) {
				return result, "EOF recovery admission child metadata is malformed", nil
			}
			if err != nil {
				return result, "", err
			}
			if depth >= compactEOFRecoveryAdmissionMaxDepth {
				return result, "EOF recovery admission depth cap", nil
			}
			frames[depth] = compactEOFRecoveryAdmissionFrame{
				payload:         child,
				path:            compactEOFRecoveryAdmissionChildPath(frame.path, ordinal),
				incomingAlias:   alias,
				parentStart:     frame.startByte,
				parentEnd:       frame.endByte,
				hasParent:       true,
				frontierBlocked: frame.descendantsBlocked,
			}
			depth++
		}
	}
	for _, value := range [...]uint64{
		uint64(result.payloadCount),
		uint64(result.occurrenceCount),
		uint64(result.visibleFrontier),
		uint64(result.metadataGroups),
		uint64(result.polls),
		uint64(result.maxDepth),
	} {
		compactEOFRecoveryAdmissionWriteUint64(hasher, value)
	}
	copy(result.fingerprint[:], hasher.Sum(nil))
	return result, "", nil
}

func (s *diagnosticParserCoreGenericScheduler) produceCompactEOFRecoveryAdmission(
	source []byte,
	poll func() error,
) (receipt compactEOFRecoveryAdmissionReceipt, err error) {
	receipt.selectedEvent = -1
	receipt.constructionRoute = compactEOFRecoveryAdmissionRouteNone
	receipt.metadataOnly = true
	defer func() { s.eofRecoveryAdmission = receipt }()
	if s == nil || !s.options.allowMetadataEOFAcceptRecovery {
		return receipt, nil
	}
	if s.token.Symbol != 0 {
		return receipt, nil
	}
	receipt.active = true
	receipt.state = compactEOFRecoveryAdmissionInvalid
	if s.compact == nil || s.tokenSource == nil || s.tokenSource.language == nil {
		compactEOFRecoveryAdmissionInvalidate(&receipt, "EOF recovery admission context is incomplete")
		return receipt, nil
	}
	language := s.tokenSource.language
	if language.ExternalScanner != nil || language.ExternalTokenCount != 0 {
		compactEOFRecoveryAdmissionInvalidate(&receipt, "EOF recovery admission requires scanner quiescence")
		return receipt, nil
	}
	if s.token.StartByte != s.token.EndByte || s.token.Missing || s.token.NoLookahead ||
		s.token.ExternalScannerToken || uint64(len(source)) > math.MaxUint32 ||
		s.token.EndByte != uint32(len(source)) {
		compactEOFRecoveryAdmissionInvalidate(&receipt, "EOF recovery admission requires authenticated source EOF")
		return receipt, nil
	}
	if len(s.headers) != 2 || s.headers[0].head == s.headers[1].head {
		compactEOFRecoveryAdmissionInvalidate(&receipt, "EOF recovery admission requires two distinct heads")
		return receipt, nil
	}
	for _, header := range s.headers {
		if header.recoveryRegion() != nil {
			compactEOFRecoveryAdmissionInvalidate(&receipt, "EOF recovery admission rejects an open strategy-three region")
			return receipt, nil
		}
	}

	normalIndex := -1
	recoveryIndex := -1
	for index, header := range s.headers {
		boundary, boundaryErr := s.compact.ClassifyBoundary(header.head, 0)
		if boundaryErr != nil {
			return receipt, boundaryErr
		}
		actions := boundary.Actions()
		switch {
		case actions.Len() == 1 && actions.At(0).Type == core.ActionAccept:
			if normalIndex >= 0 {
				compactEOFRecoveryAdmissionInvalidate(&receipt, "EOF recovery admission found multiple accepting heads")
				return receipt, nil
			}
			normalIndex = index
		case actions.Len() == 0:
			if recoveryIndex >= 0 {
				compactEOFRecoveryAdmissionInvalidate(&receipt, "EOF recovery admission found multiple no-action heads")
				return receipt, nil
			}
			recoveryIndex = index
		default:
			compactEOFRecoveryAdmissionInvalidate(&receipt, "EOF recovery admission found an unsupported action row")
			return receipt, nil
		}
	}
	if normalIndex < 0 || recoveryIndex < 0 {
		compactEOFRecoveryAdmissionInvalidate(&receipt, "EOF recovery admission requires one accept and one no-action head")
		return receipt, nil
	}

	receipt.coreGeneration = s.compact.AuthenticationGeneration()
	receipt.electionIndex = s.electionIndex
	receipt.token = s.token
	receipt.sourceLength = uint32(len(source))
	newlineCount, inspectErr := s.inspectCompactEOFRecoveryAdmissionSource(&receipt, source, poll)
	if inspectErr != nil {
		return receipt, inspectErr
	}
	normalHeader := s.headers[normalIndex]
	recoveryHeader := s.headers[recoveryIndex]
	normalPath, decline, inspectErr := s.inspectCompactEOFRecoveryAdmissionPath(
		&receipt,
		normalHeader,
		language,
		uint64(compactEOFRecoveryAdmissionEventNormal),
		uint32(len(source)),
		poll,
	)
	if inspectErr != nil {
		return receipt, inspectErr
	}
	if decline != "" {
		compactEOFRecoveryAdmissionInvalidate(&receipt, decline)
		return receipt, nil
	}
	recoveryPath, decline, inspectErr := s.inspectCompactEOFRecoveryAdmissionPath(
		&receipt,
		recoveryHeader,
		language,
		uint64(compactEOFRecoveryAdmissionEventRecover),
		uint32(len(source)),
		poll,
	)
	if inspectErr != nil {
		return receipt, inspectErr
	}
	if decline != "" {
		compactEOFRecoveryAdmissionInvalidate(&receipt, decline)
		return receipt, nil
	}

	spanCost, multiplyErr := compactEOFRecoveryAdmissionMultiplyCost(
		&receipt,
		uint64(len(source)),
		core.RecoveryCostPerSkippedChar,
	)
	if multiplyErr != nil {
		return receipt, multiplyErr
	}
	lineCost, multiplyErr := compactEOFRecoveryAdmissionMultiplyCost(
		&receipt,
		newlineCount,
		core.RecoveryCostPerSkippedLine,
	)
	if multiplyErr != nil {
		return receipt, multiplyErr
	}
	frontierCost, multiplyErr := compactEOFRecoveryAdmissionMultiplyCost(
		&receipt,
		uint64(recoveryPath.visibleFrontier),
		core.RecoveryCostPerSkippedTree,
	)
	if multiplyErr != nil {
		return receipt, multiplyErr
	}
	var recoveryCost uint64
	for _, amount := range [...]uint64{
		core.RecoveryCostPerRecovery,
		spanCost,
		lineCost,
		frontierCost,
	} {
		if err := compactEOFRecoveryAdmissionAddCost(s, &receipt, &recoveryCost, amount); err != nil {
			return receipt, err
		}
	}
	if recoveryCost > math.MaxUint32 {
		receipt.work.overflow = true
		compactEOFRecoveryAdmissionInvalidate(&receipt, "EOF recovery admission cost overflow")
		return receipt, errors.New("parser-core phase zero: EOF recovery admission cost overflow")
	}

	receipt.normalHead = normalHeader.head
	receipt.recoveryHead = recoveryHeader.head
	receipt.normalCreationSeq = normalHeader.creationSeq
	receipt.recoveryCreationSeq = recoveryHeader.creationSeq
	receipt.normalLineage = normalHeader.cleanPathLineage
	receipt.recoveryLineage = recoveryHeader.cleanPathLineage
	receipt.normalFingerprint = normalPath.fingerprint
	receipt.recoveryFingerprint = recoveryPath.fingerprint
	receipt.normalPayloads = normalPath.payloadCount
	receipt.recoveryPayloads = recoveryPath.payloadCount
	receipt.normalOccurrences = normalPath.occurrenceCount
	receipt.recoveryOccurrences = recoveryPath.occurrenceCount
	receipt.normalFrontier = normalPath.visibleFrontier
	receipt.recoveryFrontier = recoveryPath.visibleFrontier
	receipt.events = [2]compactEOFRecoveryAdmissionEvent{
		{ordinal: 0, kind: compactEOFRecoveryAdmissionEventNormal, dynamicPrecedence: normalPath.dynamicPrecedence},
		{ordinal: 1, kind: compactEOFRecoveryAdmissionEventRecover, cost: uint32(recoveryCost), dynamicPrecedence: recoveryPath.dynamicPrecedence},
	}
	if receipt.events[0].cost >= receipt.events[1].cost {
		compactEOFRecoveryAdmissionInvalidate(&receipt, "EOF recovery admission requires a strictly lower normal error cost")
		return receipt, nil
	}
	receipt.selectedEvent = 0
	receipt.valid = true
	if err := compactEOFRecoveryAdmissionTransition(&receipt, compactEOFRecoveryAdmissionProduced); err != nil {
		compactEOFRecoveryAdmissionInvalidate(&receipt, err.Error())
		return receipt, err
	}
	return receipt, nil
}

func (s *diagnosticParserCoreGenericScheduler) compactEOFRecoveryAdmissionHeader(
	head core.Head,
	creationSeq uint64,
	lineage uint16,
) (diagnosticParserCoreHeader, bool) {
	if s == nil {
		return diagnosticParserCoreHeader{}, false
	}
	for _, header := range s.headers {
		if header.head == head && header.creationSeq == creationSeq &&
			header.cleanPathLineage == lineage {
			return header, true
		}
	}
	return diagnosticParserCoreHeader{}, false
}

func (s *diagnosticParserCoreGenericScheduler) validateCompactEOFRecoveryAdmission(
	source []byte,
	wantState compactEOFRecoveryAdmissionState,
) error {
	if s == nil || s.compact == nil {
		return errors.New("parser-core phase zero: EOF recovery receipt has no Core")
	}
	receipt := &s.eofRecoveryAdmission
	if !receipt.active || !receipt.valid || receipt.state != wantState ||
		receipt.transitionCount == 0 || !compactEOFRecoveryAdmissionSealIsValid(receipt) {
		return errors.New("parser-core phase zero: EOF recovery receipt is not authentic")
	}
	if receipt.coreGeneration == 0 || receipt.coreGeneration != s.compact.AuthenticationGeneration() ||
		receipt.electionIndex != s.electionIndex || receipt.token != s.token ||
		uint64(len(source)) > math.MaxUint32 || receipt.sourceLength != uint32(len(source)) ||
		receipt.sourceSHA256 != sha256.Sum256(source) {
		return errors.New("parser-core phase zero: EOF recovery receipt binding changed")
	}
	if !receipt.metadataOnly || receipt.selectedEvent != 0 ||
		receipt.events[0].ordinal != 0 || receipt.events[0].kind != compactEOFRecoveryAdmissionEventNormal ||
		receipt.events[1].ordinal != 1 || receipt.events[1].kind != compactEOFRecoveryAdmissionEventRecover ||
		receipt.events[0].cost >= receipt.events[1].cost || receipt.work.overflow ||
		receipt.work.publicationAttempts != 0 || receipt.work.parserConstructions != 0 {
		return errors.New("parser-core phase zero: EOF recovery receipt policy changed")
	}
	wantTransitions := [...]compactEOFRecoveryAdmissionState{
		compactEOFRecoveryAdmissionProduced,
		compactEOFRecoveryAdmissionPreApplyValidated,
		compactEOFRecoveryAdmissionAcceptApplied,
		compactEOFRecoveryAdmissionRecoveryDropped,
		compactEOFRecoveryAdmissionCompleted,
		compactEOFRecoveryAdmissionSchedulerReturned,
		compactEOFRecoveryAdmissionConsumed,
	}
	for index := uint8(0); index < receipt.transitionCount; index++ {
		if receipt.transitions[index] != wantTransitions[index] ||
			receipt.transitionSeals[index] == ([32]byte{}) {
			return errors.New("parser-core phase zero: EOF recovery receipt transition changed")
		}
	}

	normal, normalOK := s.compactEOFRecoveryAdmissionHeader(
		receipt.normalHead,
		receipt.normalCreationSeq,
		receipt.normalLineage,
	)
	recovery, recoveryOK := s.compactEOFRecoveryAdmissionHeader(
		receipt.recoveryHead,
		receipt.recoveryCreationSeq,
		receipt.recoveryLineage,
	)
	switch wantState {
	case compactEOFRecoveryAdmissionProduced, compactEOFRecoveryAdmissionPreApplyValidated:
		if len(s.headers) != 2 || !normalOK || !recoveryOK || normal.accepted || recovery.accepted {
			return errors.New("parser-core phase zero: EOF recovery pre-accept frontier changed")
		}
	case compactEOFRecoveryAdmissionAcceptApplied:
		if len(s.headers) != 2 || !normalOK || !recoveryOK || !normal.accepted || recovery.accepted {
			return errors.New("parser-core phase zero: EOF recovery accepted frontier changed")
		}
	case compactEOFRecoveryAdmissionRecoveryDropped,
		compactEOFRecoveryAdmissionCompleted,
		compactEOFRecoveryAdmissionSchedulerReturned,
		compactEOFRecoveryAdmissionConsumed:
		if len(s.headers) != 1 || !normalOK || recoveryOK || !normal.accepted {
			return errors.New("parser-core phase zero: EOF recovery selected frontier changed")
		}
	default:
		return errors.New("parser-core phase zero: EOF recovery receipt state is unsupported")
	}
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) compactEOFRecoveryAdmissionFault(stage string) error {
	if compactEOFRecoveryAdmissionFaultHook == nil {
		return nil
	}
	return compactEOFRecoveryAdmissionFaultHook(s, stage)
}

func (s *diagnosticParserCoreGenericScheduler) compactEOFRecoveryAdmissionDropMatches(indices []int) bool {
	if s == nil || len(indices) != 1 || len(s.headers) != 2 ||
		s.eofRecoveryAdmission.state != compactEOFRecoveryAdmissionAcceptApplied ||
		!compactEOFRecoveryAdmissionSealIsValid(&s.eofRecoveryAdmission) {
		return false
	}
	dropIndex := indices[0]
	if dropIndex < 0 || dropIndex >= len(s.headers) {
		return false
	}
	dropped := s.headers[dropIndex]
	if dropped.head != s.eofRecoveryAdmission.recoveryHead ||
		dropped.creationSeq != s.eofRecoveryAdmission.recoveryCreationSeq ||
		dropped.cleanPathLineage != s.eofRecoveryAdmission.recoveryLineage || dropped.accepted {
		return false
	}
	survivor := s.headers[1-dropIndex]
	return survivor.head == s.eofRecoveryAdmission.normalHead &&
		survivor.creationSeq == s.eofRecoveryAdmission.normalCreationSeq &&
		survivor.cleanPathLineage == s.eofRecoveryAdmission.normalLineage && survivor.accepted
}

func (s *diagnosticParserCoreGenericScheduler) applyCompactEOFRecoveryAdmission(
	before []DiagnosticParserCoreHeaderReceipt,
	cell diagnosticParserCoreGenericCell,
) (handled bool, err error) {
	if s == nil || !s.options.allowMetadataEOFAcceptRecovery ||
		s.options.allowEOFAcceptNoActionSiblings {
		return false, nil
	}
	if len(s.headers) != 2 || len(s.acceptedPayloads) != 0 {
		return false, nil
	}
	var headersBefore [2]diagnosticParserCoreHeader
	copy(headersBefore[:], s.headers)
	dispatchesBefore, workBefore := s.dispatches, s.work
	epochProgressBefore := s.epochProgress
	acceptedHeadBefore := s.acceptedHead
	acceptedPayloadsBefore := s.acceptedPayloads
	roundsBefore := s.receipt.Rounds
	noActionDropsBefore := s.receipt.NoActionDrops
	receiptBefore := s.eofRecoveryAdmission
	rollback := func(cause error) (bool, error) {
		s.invalidateVerifierHeaderBinding()
		current := s.headers
		if cap(current) < len(headersBefore) {
			clear(current)
			current = make([]diagnosticParserCoreHeader, len(headersBefore))
		} else {
			clearDiagnosticParserCoreHeaderSuffix(current, len(headersBefore))
			current = current[:len(headersBefore)]
		}
		copy(current, headersBefore[:])
		s.headers = current
		s.dispatches, s.work = dispatchesBefore, workBefore
		s.epochProgress = epochProgressBefore
		s.acceptedHead = acceptedHeadBefore
		s.acceptedPayloads = acceptedPayloadsBefore
		s.receipt.Rounds = roundsBefore
		s.receipt.NoActionDrops = noActionDropsBefore
		s.eofRecoveryAdmission = receiptBefore
		compactEOFRecoveryAdmissionInvalidate(&s.eofRecoveryAdmission, cause.Error())
		return true, cause
	}

	receipt, produceErr := s.produceCompactEOFRecoveryAdmission(
		s.options.materializationSource,
		s.pollStopControl,
	)
	if produceErr != nil {
		return rollback(produceErr)
	}
	if !receipt.active || !receipt.valid {
		return false, nil
	}
	if faultErr := s.compactEOFRecoveryAdmissionFault("after_produce"); faultErr != nil {
		return rollback(faultErr)
	}
	if validateErr := s.validateCompactEOFRecoveryAdmission(
		s.options.materializationSource,
		compactEOFRecoveryAdmissionProduced,
	); validateErr != nil {
		return rollback(validateErr)
	}
	siblings := make([]int, 0, len(s.headers)-1)
	for index := range s.headers {
		if index != int(cell.headerIndex) {
			siblings = append(siblings, index)
		}
	}
	if transitionErr := compactEOFRecoveryAdmissionTransition(
		&s.eofRecoveryAdmission,
		compactEOFRecoveryAdmissionPreApplyValidated,
	); transitionErr != nil {
		return rollback(transitionErr)
	}
	// Census the complete frontier at the safe pre-apply point. Applying the
	// accept can mutate the compact head and retire its sibling, so later reads
	// cannot reconstruct this admission history without observing live state.
	s.censusEOFAcceptHistoryFrontier(int(cell.headerIndex), siblings)
	if applyErr := s.applyGenericAccept(before, cell); applyErr != nil {
		return rollback(applyErr)
	}
	if transitionErr := compactEOFRecoveryAdmissionTransition(
		&s.eofRecoveryAdmission,
		compactEOFRecoveryAdmissionAcceptApplied,
	); transitionErr != nil {
		return rollback(transitionErr)
	}
	if faultErr := s.compactEOFRecoveryAdmissionFault("after_apply"); faultErr != nil {
		return rollback(faultErr)
	}
	if validateErr := s.validateCompactEOFRecoveryAdmission(
		s.options.materializationSource,
		compactEOFRecoveryAdmissionAcceptApplied,
	); validateErr != nil {
		return rollback(validateErr)
	}

	indices := s.dispatchScratch.noActionIndices[:0]
	accepted := 0
	for index, header := range s.headers {
		if header.accepted {
			accepted++
			continue
		}
		indices = append(indices, index)
	}
	if accepted != 1 || len(indices) != 1 {
		return rollback(errors.New("parser-core phase zero: EOF recovery admission did not preserve one accepting head"))
	}
	if s.options.recordDropCohortFrontiers {
		if publishErr := s.publishDropCohortFrontierOwned(indices); publishErr != nil {
			return rollback(publishErr)
		}
	}
	if consumeErr := s.consumeDropCohortFrontierOwned(indices); consumeErr != nil {
		return rollback(consumeErr)
	}
	if dropErr := s.dropGenericNoActionHeads(indices); dropErr != nil {
		return rollback(dropErr)
	}
	if transitionErr := compactEOFRecoveryAdmissionTransition(
		&s.eofRecoveryAdmission,
		compactEOFRecoveryAdmissionRecoveryDropped,
	); transitionErr != nil {
		return rollback(transitionErr)
	}
	if faultErr := s.compactEOFRecoveryAdmissionFault("after_drop"); faultErr != nil {
		return rollback(faultErr)
	}
	if validateErr := s.validateCompactEOFRecoveryAdmission(
		s.options.materializationSource,
		compactEOFRecoveryAdmissionRecoveryDropped,
	); validateErr != nil {
		return rollback(validateErr)
	}
	return true, nil
}

func (s *diagnosticParserCoreGenericScheduler) markCompactEOFRecoverySchedulerReturned(source []byte) error {
	if s == nil || !s.eofRecoveryAdmission.active {
		return nil
	}
	if err := s.validateCompactEOFRecoveryAdmission(
		source,
		compactEOFRecoveryAdmissionCompleted,
	); err != nil {
		compactEOFRecoveryAdmissionInvalidate(&s.eofRecoveryAdmission, err.Error())
		return err
	}
	if err := compactEOFRecoveryAdmissionTransition(
		&s.eofRecoveryAdmission,
		compactEOFRecoveryAdmissionSchedulerReturned,
	); err != nil {
		compactEOFRecoveryAdmissionInvalidate(&s.eofRecoveryAdmission, err.Error())
		return err
	}
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) beginCompactEOFRecoveryConstruction(
	source []byte,
	route compactEOFRecoveryAdmissionRoute,
) (bool, error) {
	if s == nil || !s.eofRecoveryAdmission.active {
		return false, nil
	}
	if route != compactEOFRecoveryAdmissionRoutePublicTree &&
		route != compactEOFRecoveryAdmissionRouteSelectedStore {
		compactEOFRecoveryAdmissionInvalidate(
			&s.eofRecoveryAdmission,
			"EOF recovery admission rejects a live publication route",
		)
		return false, errors.New("parser-core phase zero: EOF recovery receipt route is unsupported")
	}
	if err := s.validateCompactEOFRecoveryAdmission(
		source,
		compactEOFRecoveryAdmissionSchedulerReturned,
	); err != nil {
		compactEOFRecoveryAdmissionInvalidate(&s.eofRecoveryAdmission, err.Error())
		return false, err
	}
	receipt := &s.eofRecoveryAdmission
	if receipt.consumptionCount != 0 ||
		receipt.constructionRoute != compactEOFRecoveryAdmissionRouteNone ||
		receipt.work.treeConstructions != 0 ||
		receipt.work.selectedStoreConstructions != 0 {
		compactEOFRecoveryAdmissionInvalidate(receipt, "EOF recovery admission receipt was already consumed")
		return false, errors.New("parser-core phase zero: EOF recovery receipt was already consumed")
	}
	var constructionCounter *uint64
	switch route {
	case compactEOFRecoveryAdmissionRoutePublicTree:
		constructionCounter = &receipt.work.treeConstructions
	case compactEOFRecoveryAdmissionRouteSelectedStore:
		constructionCounter = &receipt.work.selectedStoreConstructions
	}
	value, ok := compactEOFRecoveryAdmissionCheckedAdd(*constructionCounter, 1)
	if !ok {
		receipt.work.overflow = true
		compactEOFRecoveryAdmissionInvalidate(receipt, "EOF recovery admission construction counter overflow")
		return false, errors.New("parser-core phase zero: EOF recovery construction counter overflow")
	}
	*constructionCounter = value
	receipt.consumptionCount = 1
	receipt.constructionRoute = route
	if err := compactEOFRecoveryAdmissionTransition(
		receipt,
		compactEOFRecoveryAdmissionConsumed,
	); err != nil {
		compactEOFRecoveryAdmissionInvalidate(receipt, err.Error())
		return false, err
	}
	if compactEOFRecoveryAdmissionCensusHook != nil {
		compactEOFRecoveryAdmissionCensusHook(*receipt)
	}
	return true, nil
}

func (s *diagnosticParserCoreGenericScheduler) failCompactEOFRecoveryConstruction(err error) {
	if s == nil || err == nil || !s.eofRecoveryAdmission.active {
		return
	}
	compactEOFRecoveryAdmissionInvalidate(
		&s.eofRecoveryAdmission,
		"EOF recovery admission construction failed",
	)
}

const (
	diagnosticParserCoreCapPressureFirstDivisor  = 8
	diagnosticParserCoreCapPressureSecondDivisor = 4
	diagnosticParserCoreCapPressureMarginNum     = 17
	diagnosticParserCoreCapPressureMarginDen     = 16
)

// diagnosticParserCoreCapPressurePrediction samples node growth at one
// eighth and one quarter of the compact node arena. It declines only when
// both linear projections exceed the cap by 6.25 percent. The source floor
// suppresses polling on small parses. Source length alone never declines a
// route. In the 408-file V10 census, 43 successful routes reached the second
// sample. Their maximum projection was 0.988x. All 12 cap failures exceeded
// 1.108x.
type diagnosticParserCoreCapPressurePrediction struct {
	samples             uint8
	priorProjectedNodes uint64
}

func diagnosticParserCoreProjectedNodes(nodes, progressBytes, sourceBytes uint32) uint64 {
	if nodes == 0 || progressBytes == 0 || sourceBytes == 0 {
		return 0
	}
	product := uint64(nodes) * uint64(sourceBytes)
	projected := product / uint64(progressBytes)
	if product%uint64(progressBytes) != 0 {
		projected++
	}
	return projected
}

func diagnosticParserCoreCapPressureSourceEligible(sourceBytes, maxNodes uint32) bool {
	return maxNodes >= diagnosticParserCoreCapPressureFirstDivisor &&
		sourceBytes >= maxNodes/diagnosticParserCoreCapPressureFirstDivisor
}

func (p *diagnosticParserCoreCapPressurePrediction) nextThreshold(maxNodes uint32) uint32 {
	if p == nil {
		return 0
	}
	switch p.samples {
	case 0:
		return maxNodes / diagnosticParserCoreCapPressureFirstDivisor
	case 1:
		return maxNodes / diagnosticParserCoreCapPressureSecondDivisor
	default:
		return 0
	}
}

func (p *diagnosticParserCoreCapPressurePrediction) observe(
	nodes, progressBytes, sourceBytes, maxNodes uint32,
) (decline bool, projected uint64) {
	if p == nil || !diagnosticParserCoreCapPressureSourceEligible(sourceBytes, maxNodes) {
		return false, 0
	}
	threshold := p.nextThreshold(maxNodes)
	if threshold == 0 || nodes < threshold {
		return false, 0
	}
	projected = diagnosticParserCoreProjectedNodes(nodes, progressBytes, sourceBytes)
	if projected == 0 {
		return false, 0
	}
	if p.samples == 0 {
		p.priorProjectedNodes = projected
		p.samples = 1
		return false, projected
	}
	p.samples = 2
	margin := (uint64(maxNodes)*diagnosticParserCoreCapPressureMarginNum + diagnosticParserCoreCapPressureMarginDen - 1) /
		diagnosticParserCoreCapPressureMarginDen
	return p.priorProjectedNodes > margin && projected > margin, projected
}

// Keep only small scheduler scratch buffers between fresh full parses. This
// bound prevents one wide frontier from retaining disproportionate memory.
const diagnosticParserCoreRetainedScratchCapacity = 64

func resetDiagnosticParserCoreRetainedSlice[T any](items []T) []T {
	if cap(items) == 0 {
		return nil
	}
	if cap(items) > diagnosticParserCoreRetainedScratchCapacity {
		return nil
	}
	clear(items[:cap(items)])
	return items[:0]
}

func resetDiagnosticParserCoreRetainedScannerBytes(items []byte) []byte {
	if cap(items) == 0 {
		return nil
	}
	if cap(items) > externalScannerSerializationBufferSize {
		return nil
	}
	clear(items[:cap(items)])
	return items[:0]
}

func resetDiagnosticParserCoreDFARelexSnapshotScratch(scratch dfaRelexSnapshotScratch) dfaRelexSnapshotScratch {
	if cap(scratch.externalPayload) == externalScannerSerializationBufferSize {
		clear(scratch.externalPayload[:cap(scratch.externalPayload)])
		scratch.externalPayload = scratch.externalPayload[:0]
	} else {
		if cap(scratch.externalPayload) != 0 {
			clear(scratch.externalPayload[:cap(scratch.externalPayload)])
		}
		scratch.externalPayload = nil
	}
	scratch.externalTokenStart = resetDiagnosticParserCoreRetainedScannerBytes(scratch.externalTokenStart)
	scratch.externalTokenEnd = resetDiagnosticParserCoreRetainedScannerBytes(scratch.externalTokenEnd)
	scratch.extZeroTried = resetDiagnosticParserCoreRetainedSlice(scratch.extZeroTried)
	return scratch
}

func clearDiagnosticParserCoreHeaderBacking(headers []diagnosticParserCoreHeader) {
	if cap(headers) == 0 {
		return
	}
	clear(headers[:cap(headers)])
}

func clearDiagnosticParserCorePhaseHeadBacking(heads []diagnosticParserCorePhaseHead) {
	if cap(heads) == 0 {
		return
	}
	clear(heads[:cap(heads)])
}

// clearDiagnosticParserCoreGenericSchedulerVersionState drops every retained
// header and phase-key reference before the scheduler struct is replaced.
// Callers can retain aliases to these backing arrays, so dropping the fields
// alone does not release the immutable per-version state they point to.
func clearDiagnosticParserCoreGenericSchedulerVersionState(scheduler *diagnosticParserCoreGenericScheduler) {
	if scheduler == nil {
		return
	}
	clearDiagnosticParserCoreHeaderBacking(scheduler.headers)
	clearDiagnosticParserCoreHeaderBacking(scheduler.seedHeaders[:])
	clearDiagnosticParserCoreHeaderBacking(scheduler.headerRollbackScratch.headers)
	clearDiagnosticParserCoreHeaderBacking(scheduler.headerRollbackScratch.inline[:])
	for _, headers := range scheduler.canonicalScratch.headerBuffers {
		clearDiagnosticParserCoreHeaderBacking(headers)
	}
	for index := range scheduler.canonicalScratch.inlineHeaders {
		clearDiagnosticParserCoreHeaderBacking(scheduler.canonicalScratch.inlineHeaders[index][:])
	}
	clearDiagnosticParserCoreHeaderBacking(scheduler.conflictScratch.outputs)
	clearDiagnosticParserCoreHeaderBacking(scheduler.conflictScratch.headerAssembly)
	clearDiagnosticParserCoreHeaderBacking(scheduler.reductionReplacements)
	if cap(scheduler.recoveryCondenseScratch) != 0 {
		clear(scheduler.recoveryCondenseScratch[:cap(scheduler.recoveryCondenseScratch)])
	}
	if cap(scheduler.recoveryCondenseOrderScratch) != 0 {
		clear(scheduler.recoveryCondenseOrderScratch[:cap(scheduler.recoveryCondenseOrderScratch)])
	}
	clearDiagnosticParserCorePhaseHeadBacking(scheduler.canonicalScratch.keys)
	clearDiagnosticParserCorePhaseHeadBacking(scheduler.canonicalScratch.inlineKeys[:])
	clear(scheduler.canonicalScratch.groups)
	scheduler.canonicalScratch.versionStateEqual = nil
	if cap(scheduler.versionLexerRequests) != 0 {
		clear(scheduler.versionLexerRequests[:cap(scheduler.versionLexerRequests)])
	}
	scheduler.versionLexerBefore = dfaRelexSnapshot{}
	scheduler.versionLexerBeforeValid = false
	scheduler.versionLexerBeforeIdentity = [32]byte{}
	scheduler.versionLexerBeforeIdentityValid = false
	scheduler.versionLexerBeforeElection = 0
	scheduler.versionLexerBeforeCheckpoint = 0
	scheduler.versionLexerOwnershipActive = false
	scheduler.versionLexerNoActionProof = false
}

func resetDiagnosticParserCoreGenericScheduler(scheduler *diagnosticParserCoreGenericScheduler) error {
	if scheduler.dispatchScratch.busy || scheduler.conflictScratch.busy {
		return errors.New("parser-core phase zero: seed scheduler scratch is active")
	}
	clearDiagnosticParserCoreGenericSchedulerVersionState(scheduler)
	summaryHeaders := resetDiagnosticParserCoreRetainedSlice(scheduler.summaryHeaderScratch)
	dispatchCells := resetDiagnosticParserCoreRetainedSlice(scheduler.dispatchScratch.cells)
	noActionIndices := resetDiagnosticParserCoreRetainedSlice(scheduler.dispatchScratch.noActionIndices)
	conflictActionOutputs := resetDiagnosticParserCoreRetainedSlice(scheduler.conflictScratch.actionOutputs)
	conflictReductionOutputs := resetDiagnosticParserCoreRetainedSlice(scheduler.conflictScratch.reductionOutputs)
	conflictOutputs := resetDiagnosticParserCoreRetainedSlice(scheduler.conflictScratch.outputs)
	conflictArmRanges := resetDiagnosticParserCoreRetainedSlice(scheduler.conflictScratch.armRanges)
	conflictAdopted := resetDiagnosticParserCoreRetainedSlice(scheduler.conflictScratch.adopted)
	conflictHeaderAssembly := resetDiagnosticParserCoreRetainedSlice(scheduler.conflictScratch.headerAssembly)
	reductionOutputs := resetDiagnosticParserCoreRetainedSlice(scheduler.reductionOutputs)
	reductionReplacements := resetDiagnosticParserCoreRetainedSlice(scheduler.reductionReplacements)
	recoveryCondenseScratch := resetDiagnosticParserCoreRetainedSlice(scheduler.recoveryCondenseScratch)
	recoveryCondenseOrderScratch := resetDiagnosticParserCoreRetainedSlice(scheduler.recoveryCondenseOrderScratch)
	footprintRefs := resetDiagnosticParserCoreRetainedSlice(scheduler.footprintRefs)
	classifiedBoundaries := resetDiagnosticParserCoreRetainedSlice(scheduler.classifiedBoundaries)
	condenseCandidates := resetDiagnosticParserCoreRetainedSlice(scheduler.condenseCandidates)
	electStates := resetDiagnosticParserCoreRetainedSlice(scheduler.electStates)
	electGLRStates := resetDiagnosticParserCoreRetainedSlice(scheduler.electGLRStates)
	acceptedPayloads := resetDiagnosticParserCoreRetainedSlice(scheduler.acceptedPayloads)
	versionLexerRequests := resetDiagnosticParserCoreRetainedSlice(scheduler.versionLexerRequests)
	versionLexerBeforeScratch := resetDiagnosticParserCoreDFARelexSnapshotScratch(scheduler.versionLexerBeforeScratch)
	*scheduler = diagnosticParserCoreGenericScheduler{
		summaryHeaderScratch: summaryHeaders,
		dispatchScratch: diagnosticParserCoreDispatchScratch{
			cells: dispatchCells, noActionIndices: noActionIndices,
		},
		conflictScratch: diagnosticParserCoreConflictScratch{
			actionOutputs: conflictActionOutputs, reductionOutputs: conflictReductionOutputs,
			outputs: conflictOutputs, armRanges: conflictArmRanges, adopted: conflictAdopted,
			headerAssembly: conflictHeaderAssembly,
		},
		reductionOutputs:             reductionOutputs,
		reductionReplacements:        reductionReplacements,
		recoveryCondenseScratch:      recoveryCondenseScratch,
		recoveryCondenseOrderScratch: recoveryCondenseOrderScratch,
		footprintRefs:                footprintRefs,
		classifiedBoundaries:         classifiedBoundaries,
		condenseCandidates:           condenseCandidates,
		electStates:                  electStates,
		electGLRStates:               electGLRStates,
		acceptedPayloads:             acceptedPayloads,
		versionLexerBeforeScratch:    versionLexerBeforeScratch,
		versionLexerRequests:         versionLexerRequests,
	}
	return nil
}

const maxDiagnosticParserCoreNoLookaheadSteps = 64

// maxDiagnosticParserCoreMissingInsertions bounds zero-width S5 forks.
const maxDiagnosticParserCoreMissingInsertions = 256

func (s *diagnosticParserCoreGenericScheduler) fullReceipts() bool {
	return s != nil && s.options.ReceiptMode == DiagnosticParserCoreReceiptFull
}

func (s *diagnosticParserCoreGenericScheduler) headerReceipt(header diagnosticParserCoreHeader) (DiagnosticParserCoreHeaderReceipt, error) {
	if s.fullReceipts() {
		return diagnosticParserCoreHeaderReceipt(s.compact, header)
	}
	return diagnosticParserCoreHeaderSummary(s.compact, header)
}

// electHeaderState resolves only what one election round needs from a
// header: its authentic StateID. Shifted/Accepted are read directly off the
// header by the caller, so this skips the checkpoint-digest lookup that
// diagnosticParserCoreHeaderSummary otherwise pays on every header on every
// election — that digest has no reader on this path. Full-receipt mode keeps
// paying it, since diagnosticParserCoreHeaderReceipt is also what tests and
// diagnostics observe and must stay byte-identical there.
func (s *diagnosticParserCoreGenericScheduler) electHeaderState(header diagnosticParserCoreHeader) (StateID, error) {
	if s.fullReceipts() {
		receipt, err := diagnosticParserCoreHeaderReceipt(s.compact, header)
		if err != nil {
			return 0, err
		}
		return receipt.State, nil
	}
	state, _, err := s.compact.Boundary(header.head)
	if err != nil {
		return 0, err
	}
	return StateID(state), nil
}

func (s *diagnosticParserCoreGenericScheduler) headerReceipts(headers []diagnosticParserCoreHeader) ([]DiagnosticParserCoreHeaderReceipt, error) {
	if s.fullReceipts() {
		return diagnosticParserCoreHeaderReceipts(s.compact, headers)
	}
	if cap(s.summaryHeaderScratch) < len(headers) {
		s.summaryHeaderScratch = make([]DiagnosticParserCoreHeaderReceipt, len(headers))
	} else {
		s.summaryHeaderScratch = s.summaryHeaderScratch[:len(headers)]
		clear(s.summaryHeaderScratch)
	}
	out := s.summaryHeaderScratch
	for index, header := range headers {
		receipt, err := diagnosticParserCoreHeaderSummary(s.compact, header)
		if err != nil {
			return nil, err
		}
		out[index] = receipt
	}
	return out, nil
}

// diagnosticParserCoreSeedObserver is a tagged, diagnostic-only probe seam.
// It can inspect closed frontiers before an election, after an election, or
// after a complete owned frontier seal and before header publication.
type diagnosticParserCoreSeedObserver struct {
	beforeElection    func(*diagnosticParserCoreGenericScheduler) error
	afterElection     func(*diagnosticParserCoreGenericScheduler) (bool, error)
	frontierPublished func(*diagnosticParserCoreGenericScheduler, core.SchedulerTransactionToken, []int) error
}

func newDiagnosticParserCoreGenericScheduler(
	compact *core.Core,
	tokenSource *dfaTokenSource,
	scannerScratch *[]byte,
	head core.Head,
	checkpointID core.CheckpointID,
	checkpoint DiagnosticParserCoreScannerCheckpoint,
	observer diagnosticParserCoreSeedObserver,
	options DiagnosticParserCorePrefixOptions,
) (*diagnosticParserCoreGenericScheduler, error) {
	return initializeDiagnosticParserCoreGenericScheduler(
		&diagnosticParserCoreGenericScheduler{},
		compact,
		tokenSource,
		scannerScratch,
		head,
		checkpointID,
		checkpoint,
		observer,
		options,
	)
}

func initializeDiagnosticParserCoreGenericScheduler(
	scheduler *diagnosticParserCoreGenericScheduler,
	compact *core.Core,
	tokenSource *dfaTokenSource,
	scannerScratch *[]byte,
	head core.Head,
	checkpointID core.CheckpointID,
	checkpoint DiagnosticParserCoreScannerCheckpoint,
	observer diagnosticParserCoreSeedObserver,
	options DiagnosticParserCorePrefixOptions,
) (*diagnosticParserCoreGenericScheduler, error) {
	if scheduler == nil {
		return nil, errors.New("parser-core phase zero: seed scheduler storage is nil")
	}
	if err := resetDiagnosticParserCoreGenericScheduler(scheduler); err != nil {
		return nil, err
	}
	if compact == nil || tokenSource == nil || scannerScratch == nil || head.Node == 0 {
		return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "generic scheduler requires a compact core, token source, scanner scratch, and seed head"}
	}
	state, byteOffset, err := compact.Boundary(head)
	if err != nil {
		return nil, err
	}
	if byteOffset != 0 {
		return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "generic seed scheduler head is not at byte zero"}
	}
	length, digest, ok := compact.CheckpointReceipt(checkpointID)
	if !ok || int(length) != checkpoint.Length || digest != checkpoint.SHA256 {
		return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "generic seed scanner checkpoint receipt does not match its exact identity"}
	}
	if canonical, ok := compact.CanonicalBoundary(state, byteOffset, false, checkpointID); !ok || canonical != head {
		return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "generic seed head was not created under its scanner checkpoint identity"}
	}
	header := diagnosticParserCoreHeader{head: head, checkpoint: checkpointID}
	scheduler.compact = compact
	scheduler.tokenSource = tokenSource
	scheduler.scannerScratch = scannerScratch
	scheduler.checkpoint = checkpoint
	scheduler.checkpointBeforeID = checkpointID
	scheduler.checkpointID = checkpointID
	scheduler.electionIndex = -1
	scheduler.nextSeq = 1
	scheduler.nextCleanPathLineage = 1
	scheduler.options = options
	scheduler.observer = observer
	// Public diagnostic results retain their receipt after the scheduler returns.
	// Embed only for a fresh runner, which never publishes its receipt.
	if options.freshSchedulerSession {
		scheduler.receiptBacking = DiagnosticParserCoreGenericScheduler{
			ReceiptMode:     options.ReceiptMode,
			StartCheckpoint: checkpoint,
		}
		scheduler.receipt = &scheduler.receiptBacking
	} else {
		scheduler.receipt = &DiagnosticParserCoreGenericScheduler{
			ReceiptMode:     options.ReceiptMode,
			StartCheckpoint: checkpoint,
		}
	}
	scheduler.seedHeaders[0] = header
	scheduler.headers = scheduler.seedHeaders[:]
	// C4 corridor: attach the compiled bytecode program for this language when
	// the lane is on. The program is memoized on the *Language, so this is one
	// atomic load per parse after the first (spec.c4-bytecode-isa.v1 section
	// 3.6). A grammar that does not compile keeps the generic lane.
	if parserCoreCorridorEnabled() {
		if program := acquireParserCoreCorridorProgram(tokenSource.language); program != nil {
			if rows, ok := tokenSource.language.compactTables.(*parserCoreLanguageTables); ok && rows != nil {
				scheduler.corridor = program
				scheduler.corridorRows = rows.actionRows
			}
		}
	}
	if scheduler.fullReceipts() {
		startHeaders, err := diagnosticParserCoreHeaderPathReceipts(compact, scheduler.headers)
		if err != nil {
			return nil, err
		}
		scheduler.receipt.StartHeaders = startHeaders
	}
	return scheduler, nil
}

type diagnosticParserCoreCellSelection uint8

const (
	diagnosticParserCoreCellSelectionNone diagnosticParserCoreCellSelection = iota
	diagnosticParserCoreCellSelectionConflictPolicy
	diagnosticParserCoreCellSelectionRepetitionFold
	diagnosticParserCoreCellSelectionRepetitionFork
)

type diagnosticParserCoreGenericCell struct {
	boundary        core.ClassifiedBoundary
	headerIndex     int32
	selectedOrdinal int32
	// versionLexerRequest is a one-based index into the scheduler's request
	// sidecar. Zero keeps the shared-election fast path.
	versionLexerRequest uint32
	// relexedSymbol is the symbol a same-span per-header relex
	// (diagnosticParserCoreSameSpanRelex) found for this header, or 0 when
	// this header dispatches the shared election unchanged. Kept as just
	// the Symbol, not the verified relex's full Token: diagnosticParserCoreGenericCell
	// has a certified <=64-byte size cap
	// (TestDiagnosticParserCoreClassifiedBoundaryAndReductionPlanShape) that
	// a 72-byte Token field alone would blow past, and dispatchToken can
	// reconstruct the exact same-span relexed token from the Symbol alone
	// (diagnosticParserCoreSameSpanRelex already proved every other field
	// matches the shared token, or is cleared, before returning true). A
	// different-span relex (D2-1's other new shape: a different EndByte,
	// wider or narrower) never reaches a cell at all: dispatchPassActive
	// declines that header fail-closed before cell construction.
	relexedSymbol            Symbol
	selectedBy               diagnosticParserCoreCellSelection
	corridorTrustedReduction bool
}

// diagnosticParserCoreS5Work keeps recovery-search work private until the
// complete S5 route commits. A declined route must publish no staged work.
type diagnosticParserCoreS5Work struct {
	potentialReductionActions   uint64
	potentialReductionOutputs   uint64
	reductionPromotions         uint64
	missingTokenTrials          uint64
	missingTokenCommits         uint64
	recoveryDiscontinuityMerges uint64
	recoveryCeilingDeclines     uint64
}

func (s *diagnosticParserCoreGenericScheduler) commitS5Work(staged diagnosticParserCoreS5Work) {
	if s == nil {
		return
	}
	s.work.add(&s.work.PotentialReductionActions, staged.potentialReductionActions)
	s.work.add(&s.work.PotentialReductionOutputs, staged.potentialReductionOutputs)
	s.work.add(&s.work.ReductionPromotions, staged.reductionPromotions)
	s.work.add(&s.work.MissingTokenTrials, staged.missingTokenTrials)
	s.work.add(&s.work.MissingTokenCommits, staged.missingTokenCommits)
	s.work.add(&s.work.RecoveryDiscontinuityMerges, staged.recoveryDiscontinuityMerges)
	s.work.add(&s.work.RecoveryCeilingDeclines, staged.recoveryCeilingDeclines)
}

func (cell *diagnosticParserCoreGenericCell) actions() core.ActionRow { return cell.boundary.Actions() }
func (cell *diagnosticParserCoreGenericCell) dispatchToken(shared Token) Token {
	if cell.relexedSymbol != 0 {
		shared.Symbol = cell.relexedSymbol
		shared.ExternalScannerToken = false
		shared.ExternalScannerStartByte = 0
		// isKeyword is a promotion-path artifact of the shared token's own
		// lex, not a property of the relexed symbol replacing it here;
		// clear it alongside the external-scanner fields above so a
		// promoted keyword token's dispatch view is judged on the relexed
		// symbol, not on which lex path produced the original.
		shared.isKeyword = false
	}
	return shared
}
func (cell *diagnosticParserCoreGenericCell) descriptor() core.ActionRowDescriptor {
	return cell.boundary.Actions().Descriptor()
}
func (cell *diagnosticParserCoreGenericCell) kind() core.ActionRowKind {
	if cell.selectedBy == diagnosticParserCoreCellSelectionConflictPolicy {
		switch cell.actions().At(int(cell.selectedOrdinal)).Type {
		case core.ActionShift:
			return core.ActionRowShift
		case core.ActionReduce:
			return core.ActionRowReduce
		}
	}
	if cell.selectedBy == diagnosticParserCoreCellSelectionRepetitionFold {
		return core.ActionRowReduce
	}
	if cell.selectedBy == diagnosticParserCoreCellSelectionRepetitionFork {
		return core.ActionRowConflict
	}
	return cell.descriptor().Kind()
}
func (cell *diagnosticParserCoreGenericCell) selectedActionOrdinal() int {
	if cell.selectedBy != diagnosticParserCoreCellSelectionNone {
		return int(cell.selectedOrdinal)
	}
	return 0
}

func (cell *diagnosticParserCoreGenericCell) selectsConflictReduction() bool {
	return cell.selectedBy != diagnosticParserCoreCellSelectionNone &&
		cell.selectedBy != diagnosticParserCoreCellSelectionRepetitionFork &&
		cell.actions().At(cell.selectedActionOrdinal()).Type == core.ActionReduce
}

func dropCohortSelectionClass(selected diagnosticParserCoreCellSelection) core.DropCohortSelectionClass {
	switch selected {
	case diagnosticParserCoreCellSelectionConflictPolicy:
		return core.DropCohortSelectionConflictPolicy
	case diagnosticParserCoreCellSelectionRepetitionFold:
		return core.DropCohortSelectionRepetitionFold
	case diagnosticParserCoreCellSelectionRepetitionFork:
		return core.DropCohortSelectionRepetitionFork
	default:
		return core.DropCohortSelectionNone
	}
}

// diagnosticParserCoreSameSpanRelex verifies a per-header relex that landed
// on exactly the shared election's span (same StartByte and EndByte): the
// only difference a same-span relex may legitimately carry is Symbol (and
// the external-scanner/isKeyword bits, which are provenance of the shared
// token's own lex path, not of tokenization identity). Every other field
// must already match the shared token exactly; this checks that assumption
// instead of trusting it. A different EndByte never reaches this function.
// dispatchPassActive activates owned per-header lexer requests for that shape.
func diagnosticParserCoreSameSpanRelex(shared, relexed Token) (Token, bool) {
	if relexed.Symbol == 0 || relexed.Symbol == shared.Symbol {
		return Token{}, false
	}
	candidate := shared
	candidate.Symbol = relexed.Symbol
	candidate.ExternalScannerToken = false
	candidate.ExternalScannerStartByte = 0
	// isKeyword is a promotion-path artifact of the DFA token source's
	// keyword re-lex (promoteKeyword), not a property probe.scan's raw
	// DFA-only relex can ever report; clear it here too, alongside the
	// external-scanner fields above, so a promoted keyword token's relex
	// probe is judged on tokenization identity, not on which lex path
	// produced it.
	candidate.isKeyword = false
	if candidate != relexed {
		return Token{}, false
	}
	return candidate, true
}

var diagnosticParserCoreRepetitionFoldOptOut = map[string]bool{
	// Most Markdown inline repetition rows are not certified for the compact
	// frontier. Exact runtime-profile policies admit the proved rows.
	"markdown_inline": true,
}

// diagnosticParserCoreRepetitionFoldOrdinal mirrors the production parser's
// certified single-reduce repetition fold for a clean compact lineage.
func diagnosticParserCoreRepetitionFoldOrdinal(language *Language, actions core.ActionRow) (int, bool) {
	if actions.Len() < 2 || language == nil || cRepetitionSkipOptOut[language.Name] ||
		diagnosticParserCoreRepetitionFoldOptOut[language.Name] {
		return 0, false
	}
	return diagnosticParserCoreSingleReduceRepetitionShiftOrdinal(actions)
}

// diagnosticParserCoreSingleReduceRepetitionShiftOrdinal identifies the exact
// two-arm row that production either folds or keeps as one conflict fork.
func diagnosticParserCoreSingleReduceRepetitionShiftOrdinal(actions core.ActionRow) (int, bool) {
	reduceOrdinal := -1
	shiftFound := false
	for ordinal := 0; ordinal < actions.Len(); ordinal++ {
		action := actions.At(ordinal)
		switch action.Type {
		case core.ActionReduce:
			if reduceOrdinal >= 0 {
				return 0, false
			}
			reduceOrdinal = ordinal
		case core.ActionShift:
			if !action.Repetition || shiftFound {
				return 0, false
			}
			shiftFound = true
		default:
			return 0, false
		}
	}
	if reduceOrdinal < 0 || !shiftFound {
		return 0, false
	}
	return reduceOrdinal, true
}

// diagnosticParserCoreConflictPolicyOrdinal mirrors the production parser's
// deterministic clean-lineage conflict policy before the generic repetition
// fold. Exact built-in policies remain bound to their certified blob identity.
func diagnosticParserCoreConflictPolicyOrdinal(
	language *Language,
	token Token,
	state core.StateID,
	actions core.ActionRow,
	frontierHeaders int,
) (int, bool) {
	if language == nil || len(language.ConflictPolicies) == 0 || actions.Len() < 2 {
		return 0, false
	}
	var inline [8]ParseAction
	decoded := inline[:0]
	if actions.Len() <= len(inline) {
		decoded = inline[:actions.Len()]
	} else {
		decoded = make([]ParseAction, actions.Len())
	}
	for ordinal := 0; ordinal < actions.Len(); ordinal++ {
		decoded[ordinal] = rootParserCoreAction(actions.At(ordinal))
	}
	selected, ok := conflictPolicyChoiceForCompact(language, token, StateID(state), decoded, frontierHeaders)
	if !ok {
		return 0, false
	}
	for ordinal, action := range decoded {
		if action == selected {
			return ordinal, true
		}
	}
	return 0, false
}

// relexTokenForState mirrors production's per-stack lexer: each no-action
// header re-lexes at its own byte position under its own current state's
// lex mode, exactly like a live C stack version, instead of trusting the
// shared election's tokenization. Internal-DFA tokens use a scratch lexer.
// Checkpointed external scanners use the election-start snapshot and compare
// the complete post-scan cursor state. The probe accepts any genuine token
// that starts where the shared election started; it may differ in Symbol,
// scanner state, or EndByte/EndPoint. The caller alone decides what to do
// with a different EndByte. dispatchPassActive switches that shape to owned
// per-header requests before any shared action executes. This probe only
// decides whether a same-start token exists at all.
//
// DisablePerHeaderSpanUnlockedRelex restores the pre-D2-1 span-locked
// probe: only an exact-span (same StartByte and EndByte) relex is eligible.
func (s *diagnosticParserCoreGenericScheduler) relexTokenForState(state StateID, tok Token) (Token, bool) {
	if s == nil || s.tokenSource == nil || s.tokenSource.lexer == nil {
		return tok, false
	}
	lang := s.tokenSource.language
	source := s.tokenSource.lexer.source
	if lang == nil || len(lang.LexStates) == 0 || int(state) >= len(lang.LexModes) {
		return tok, false
	}
	// An external scanner can only enter the per-version probe when its
	// checkpoint is complete and identity-bearing. Restore the election-start
	// payload before the scan, then restore the shared post-election payload on
	// every exit. A stateful scanner without this contract keeps the old,
	// internal-DFA-only behavior and fails closed.
	if lang.ExternalScanner != nil {
		if relexed, ok := s.relexExternalTokenForState(state, tok); ok {
			return relexed, true
		}
		if s.checkpoint.Length == 0 {
			return tok, false
		}
	}
	if tok.Symbol == 0 || tok.Symbol == errorSymbol || tok.Missing || tok.NoLookahead ||
		tok.StartByte >= tok.EndByte || int(tok.StartByte) >= len(source) {
		return tok, false
	}
	lexState := lang.LexModes[state].LexStateIndex()
	if lexState == noLookaheadLexState || int(lexState) >= len(lang.LexStates) {
		return tok, false
	}
	// Match Parser.relexTokenForStackLexState. Lexer.scan reads only these
	// DFA fields, so this probe does not need parser or scanner state.
	probe := s.tokenSource.relexProbeLexer
	if probe == nil {
		probe = &Lexer{}
		s.tokenSource.relexProbeLexer = probe
	}
	*probe = Lexer{
		states:          lang.LexStates,
		asciiTable:      lang.LexAsciiTable(),
		source:          source,
		pos:             int(tok.StartByte),
		row:             tok.StartPoint.Row,
		col:             tok.StartPoint.Column,
		immediateTokens: lang.ImmediateTokens,
		zeroWidthTokens: lang.ZeroWidthTokens,
	}
	relexed, ok := probe.scan(uint32(lexState), probe.pos, probe.row, probe.col)
	if !ok || relexed.Symbol == 0 {
		return tok, false
	}
	// A genuine per-header relex must start exactly where the shared
	// election started. A different start byte is a leading-skip
	// differential (this header's own lex mode treats intervening bytes as
	// extras the shared election did not skip) -- a distinct shape this
	// slice does not own; leave that header on the ordinary no-action path.
	if relexed.StartByte != tok.StartByte {
		return tok, false
	}
	if relexed.Symbol == tok.Symbol && relexed.EndByte == tok.EndByte {
		// True no-op: identical tokenization: nothing this probe can offer.
		return tok, false
	}
	if s.options.DisablePerHeaderSpanUnlockedRelex && relexed.EndByte != tok.EndByte {
		return tok, false
	}
	return relexed, true
}

// relexExternalTokenForState probes one parser state's external scanner from
// the shared election-start checkpoint. It returns a witness when the token
// identity or the serialized scanner state differs from the shared election.
// The token source is restored before the caller can dispatch another header.
func (s *diagnosticParserCoreGenericScheduler) relexExternalTokenForState(state StateID, shared Token) (Token, bool) {
	if s == nil || s.tokenSource == nil || s.tokenSource.lexer == nil || !s.versionLexerBeforeValid {
		return shared, false
	}
	d := s.tokenSource
	lang := d.language
	if lang == nil || lang.ExternalScanner == nil || !languageUsesExternalScannerCheckpoints(lang) {
		return shared, false
	}
	provider, ok := externalScannerCheckpointIdentityProviderForScanner(lang.ExternalScanner)
	if !ok {
		return shared, false
	}
	identity, ok := provider.CheckpointIdentity()
	if !ok || !identity.complete() || !s.versionLexerBefore.externalScannerPresent || len(s.versionLexerBefore.externalPayload) == 0 {
		return shared, false
	}
	if s.compact != nil {
		if !s.versionLexerBeforeIdentityValid || parserCoreExternalScannerIdentityFingerprint(identity) != s.versionLexerBeforeIdentity {
			return shared, false
		}
	}
	if int(state) >= len(lang.LexModes) {
		return shared, false
	}
	// Keep the source exactly as the shared election left it. The snapshot
	// restore below may call Deserialize, so capture the current state first.
	prior := d.snapshotRelexState()
	priorState := d.state
	priorGLRStates := d.glrStates
	defer func() {
		prior.restore(d)
		d.SetParserState(priorState)
		d.SetGLRStates(priorGLRStates)
	}()

	// A production scheduler has an authenticated compact core. The direct
	// helper tests use the same snapshot type without a core, so retain the
	// raw DFA restore as a test-only fallback after the capability checks above.
	if s.compact != nil {
		length, digest, ok := s.compact.CheckpointReceipt(s.checkpointBeforeID)
		beforeInfo := parserCoreCheckpoint(s.versionLexerBefore.externalPayload)
		if !ok || uint64(len(s.versionLexerBefore.externalPayload)) != uint64(length) ||
			digest != beforeInfo.SHA256 {
			return shared, false
		}
	}
	s.versionLexerBefore.restore(d)
	d.SetParserState(state)
	d.SetGLRStates(nil)
	candidate := d.Next()
	if candidate.Symbol == 0 || candidate.NoLookahead || candidate.Missing ||
		!candidate.ExternalScannerToken || candidate.StartByte != shared.StartByte {
		return shared, false
	}
	// Compare the scanner payload after the state-specific scan. Equal token
	// bytes can still leave different scanner states, which must also activate
	// ownership for the next election.
	candidateAfter := d.snapshotRelexState()
	sharedAfter := prior
	if tokensSameLex(candidate, shared) && candidateAfter.equal(sharedAfter) {
		return shared, false
	}
	return candidate, true
}

// withVersionLexerOwner runs one snapshot publication under the scheduler's
// authenticated owner. Fresh sessions already hold the owner; ordinary
// diagnostic calls acquire one short transaction for the publication only.
func (s *diagnosticParserCoreGenericScheduler) withVersionLexerOwner(fn func(core.SchedulerTransactionToken) error) error {
	if s == nil || s.compact == nil || fn == nil {
		return errors.New("parser-core phase zero: version lexer owner is unavailable")
	}
	if s.freshSessionOwner != nil {
		return fn(*s.freshSessionOwner)
	}
	return s.compact.ApplySchedulerAtomic(fn)
}

func (s *diagnosticParserCoreGenericScheduler) versionLexerSnapshotForHeader(index int) *diagnosticParserCoreVersionLexerSnapshot {
	if s == nil || index < 0 || index >= len(s.headers) {
		return nil
	}
	return s.headers[index].versionLexerSnapshot()
}

func (s *diagnosticParserCoreGenericScheduler) versionLexerRequestForHeader(index int) *diagnosticParserCoreVersionLexerRequest {
	if s == nil || index < 0 || index >= len(s.headers) {
		return nil
	}
	header := &s.headers[index]
	reference := header.versionLexerRequestReference()
	if reference == 0 || int(reference) > len(s.versionLexerRequests) {
		return nil
	}
	_, byteOffset, err := s.compact.Boundary(header.head)
	if err != nil {
		return nil
	}
	base := header.versionLexerSnapshot()
	request := &s.versionLexerRequests[reference-1]
	if request.valid && request.electionIndex == s.electionIndex &&
		diagnosticParserCoreVersionLexerRequestStartsAtOwnedSnapshot(request, byteOffset) &&
		s.versionLexerRequestSnapshotsValid(request) &&
		diagnosticParserCoreVersionLexerSnapshotEqual(request.before, base) {
		return request
	}
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) versionLexerRequestReferenceForHeader(index int) uint32 {
	if s == nil || index < 0 || index >= len(s.headers) {
		return 0
	}
	header := &s.headers[index]
	reference := header.versionLexerRequestReference()
	if reference == 0 || int(reference) > len(s.versionLexerRequests) {
		return 0
	}
	_, byteOffset, err := s.compact.Boundary(header.head)
	if err != nil {
		return 0
	}
	request := &s.versionLexerRequests[reference-1]
	if request.valid && request.electionIndex == s.electionIndex &&
		diagnosticParserCoreVersionLexerRequestStartsAtOwnedSnapshot(request, byteOffset) &&
		s.versionLexerRequestSnapshotsValid(request) &&
		diagnosticParserCoreVersionLexerSnapshotEqual(request.before, header.versionLexerSnapshot()) {
		return reference
	}
	return 0
}

func (s *diagnosticParserCoreGenericScheduler) versionLexerRequestForCell(
	cell diagnosticParserCoreGenericCell,
) (*diagnosticParserCoreVersionLexerRequest, error) {
	if cell.versionLexerRequest == 0 {
		return nil, nil
	}
	requestIndex := int(cell.versionLexerRequest - 1)
	if s == nil || requestIndex < 0 || requestIndex >= len(s.versionLexerRequests) {
		return nil, errors.New("parser-core phase zero: version lexer cell request is out of range")
	}
	if cell.headerIndex < 0 || int(cell.headerIndex) >= len(s.headers) {
		return nil, errors.New("parser-core phase zero: version lexer cell header is out of range")
	}
	request := &s.versionLexerRequests[requestIndex]
	header := &s.headers[cell.headerIndex]
	if !request.valid || request.electionIndex != s.electionIndex ||
		cell.versionLexerRequest != header.versionLexerRequestReference() ||
		!diagnosticParserCoreVersionLexerRequestStartsAtOwnedSnapshot(request, cell.boundary.ByteOffset()) ||
		!s.versionLexerRequestSnapshotsValid(request) ||
		cell.boundary.Head() != header.head ||
		!diagnosticParserCoreVersionLexerSnapshotEqual(request.before, header.versionLexerSnapshot()) {
		return nil, errors.New("parser-core phase zero: version lexer cell request is stale")
	}
	return request, nil
}

// diagnosticParserCoreVersionLexerRequestStartsAtOwnedSnapshot validates the
// token span against its owned cursor. A reduction preserves lookahead but can
// move the graph head to a node with an earlier start byte or a new parser
// state. The header-owned reference and snapshot authenticate the request.
func diagnosticParserCoreVersionLexerRequestStartsAtOwnedSnapshot(
	request *diagnosticParserCoreVersionLexerRequest,
	boundaryByte uint32,
) bool {
	if request == nil || request.before == nil || request.before.dfa.lexerPos < 0 ||
		uint64(request.before.dfa.lexerPos) > math.MaxUint32 ||
		request.token.StartByte < uint32(request.before.dfa.lexerPos) ||
		request.token.StartByte < boundaryByte ||
		request.token.EndByte < request.token.StartByte {
		return false
	}
	return true
}

func (s *diagnosticParserCoreGenericScheduler) versionLexerRequestSnapshotsValid(
	request *diagnosticParserCoreVersionLexerRequest,
) bool {
	if s == nil || s.compact == nil || s.tokenSource == nil || s.tokenSource.language == nil ||
		request == nil || request.before == nil || request.after == nil {
		return false
	}
	for _, snapshot := range []*diagnosticParserCoreVersionLexerSnapshot{request.before, request.after} {
		if err := snapshot.validateDestination(s.compact, s.tokenSource.language); err != nil {
			return false
		}
		if err := snapshot.validate(); err != nil {
			return false
		}
	}
	return request.beforeID == request.before.afterCheckpoint &&
		request.afterID == request.after.afterCheckpoint &&
		request.beforeCheckpoint == request.before.afterCheckpointInfo &&
		request.afterCheckpoint == request.after.afterCheckpointInfo
}

func (s *diagnosticParserCoreGenericScheduler) clearVersionLexerRequests() {
	if s == nil {
		return
	}
	// Every append extends from a reset or a prior logical clear. No pointer can
	// exist beyond len here. The scheduler reset performs the capacity sweep.
	clear(s.versionLexerRequests)
	s.versionLexerRequests = s.versionLexerRequests[:0]
}

// publishVersionLexerSnapshotOwned installs one immutable cursor snapshot on
// a header. The wrapper remains copy-on-write, so a header update never
// changes a by-value rollback copy or a sibling version.
func (s *diagnosticParserCoreGenericScheduler) publishVersionLexerSnapshotOwned(
	owner core.SchedulerTransactionToken,
	index int,
	snapshot *diagnosticParserCoreVersionLexerSnapshot,
) error {
	if s == nil || index < 0 || index >= len(s.headers) {
		return errors.New("parser-core phase zero: version lexer header index is out of range")
	}
	if err := snapshot.validateDestination(s.compact, s.tokenSource.language); err != nil {
		return err
	}
	if _, _, ok := s.compact.CheckpointReceiptOwned(owner, snapshot.beforeCheckpoint); !ok {
		return errDiagnosticParserCoreUnknownCheckpointIdentity
	}
	if _, _, ok := s.compact.CheckpointReceiptOwned(owner, snapshot.afterCheckpoint); !ok {
		return errDiagnosticParserCoreUnknownCheckpointIdentity
	}
	if err := s.installEquivalentVersionLexerState(&s.headers[index], snapshot, 0, nil); err != nil {
		return err
	}
	s.work.add(&s.work.PerVersionLexPublications, 1)
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) publishVersionLexerRequestOwned(
	owner core.SchedulerTransactionToken,
	index int,
	snapshot *diagnosticParserCoreVersionLexerSnapshot,
	requestReference uint32,
) error {
	if s == nil || index < 0 || index >= len(s.headers) || requestReference == 0 ||
		int(requestReference) > len(s.versionLexerRequests) {
		return errors.New("parser-core phase zero: version lexer request publication is out of range")
	}
	if err := snapshot.validateDestination(s.compact, s.tokenSource.language); err != nil {
		return err
	}
	if _, _, ok := s.compact.CheckpointReceiptOwned(owner, snapshot.beforeCheckpoint); !ok {
		return errDiagnosticParserCoreUnknownCheckpointIdentity
	}
	if _, _, ok := s.compact.CheckpointReceiptOwned(owner, snapshot.afterCheckpoint); !ok {
		return errDiagnosticParserCoreUnknownCheckpointIdentity
	}
	if err := s.installEquivalentVersionLexerState(
		&s.headers[index], snapshot, requestReference, nil,
	); err != nil {
		return err
	}
	s.work.add(&s.work.PerVersionLexPublications, 1)
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) installEquivalentVersionLexerState(
	header *diagnosticParserCoreHeader,
	snapshot *diagnosticParserCoreVersionLexerSnapshot,
	requestReference uint32,
	extra []diagnosticParserCoreHeader,
) error {
	if s == nil || header == nil {
		return errors.New("parser-core phase zero: version lexer state publication is incomplete")
	}
	region := header.recoveryRegion()
	recoveryGroup := header.recoveryGroupIdentity()
	missingGroup := header.recoveryMissingGroupIdentity()
	baseline, baselineSet := header.recoveryNodeBaseline()
	matches := func(state *diagnosticParserCoreVersionState) bool {
		return state != nil && state.s3Region == region &&
			diagnosticParserCoreVersionLexerSnapshotEqual(state.relexSnapshot, snapshot) &&
			state.lexerRequest == requestReference && state.recoveryGroup == recoveryGroup &&
			state.missingGroup == missingGroup && state.recoveryNodeBaseline == baseline &&
			state.recoveryNodeBaselineSet == baselineSet
	}
	for index := range s.headers {
		state := s.headers[index].versionState
		if matches(state) {
			header.versionState = state
			return nil
		}
	}
	for index := range extra {
		state := extra[index].versionState
		if matches(state) {
			header.versionState = state
			return nil
		}
	}
	if snapshot != nil {
		if err := snapshot.validateDestination(s.compact, s.tokenSource.language); err != nil {
			return err
		}
	}
	header.publishVersionState(
		region, snapshot, requestReference, recoveryGroup, missingGroup,
		baseline, baselineSet,
	)
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) equivalentVersionLexerSnapshot(
	dfa dfaRelexSnapshot,
	beforeID core.CheckpointID,
	afterID core.CheckpointID,
) *diagnosticParserCoreVersionLexerSnapshot {
	if s == nil || s.compact == nil || s.tokenSource == nil || s.tokenSource.language == nil {
		return nil
	}
	var identity [32]byte
	identityRequired := false
	identityValid := false
	if s.tokenSource != nil {
		identity, identityRequired, identityValid = diagnosticParserCoreVersionLexerCheckpointIdentity(s.tokenSource.language)
	}
	contract, contractErr := diagnosticParserCoreVersionLexerScannerContractForLanguage(s.tokenSource.language)
	if contractErr != nil {
		return nil
	}
	matches := func(snapshot *diagnosticParserCoreVersionLexerSnapshot) bool {
		return snapshot != nil && snapshot.compact == s.compact &&
			snapshot.coreGeneration == s.compact.ResetGeneration() &&
			snapshot.language == s.tokenSource.language &&
			snapshot.scanner.equal(contract) &&
			snapshot.beforeCheckpoint == beforeID &&
			snapshot.afterCheckpoint == afterID && snapshot.dfa.equal(dfa) &&
			snapshot.checkpointIdentityRequired == identityRequired &&
			snapshot.checkpointIdentityValid == identityValid &&
			(!identityRequired || (identityValid && snapshot.checkpointIdentity == identity)) &&
			snapshot.validate() == nil
	}
	for index := range s.headers {
		if snapshot := s.headers[index].versionLexerSnapshot(); matches(snapshot) {
			return snapshot
		}
	}
	for index := range s.versionLexerRequests {
		request := &s.versionLexerRequests[index]
		if matches(request.before) {
			return request.before
		}
		if matches(request.after) {
			return request.after
		}
	}
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) newVersionLexerSnapshot(
	owner core.SchedulerTransactionToken,
	dfa dfaRelexSnapshot,
	beforeID core.CheckpointID,
	afterID core.CheckpointID,
) (*diagnosticParserCoreVersionLexerSnapshot, error) {
	if s == nil || s.compact == nil || s.tokenSource == nil || s.tokenSource.language == nil {
		return nil, errors.New("parser-core phase zero: version lexer snapshot context is incomplete")
	}
	return newDiagnosticParserCoreVersionLexerSnapshot(
		s.compact, s.tokenSource.language, owner, dfa, beforeID, afterID,
	)
}

// seedVersionLexerOwnership creates a private cursor at the current shared
// election start. It runs only when a ragged token activates ownership; clean
// parses retain the zero-overhead shared path.
func (s *diagnosticParserCoreGenericScheduler) seedVersionLexerOwnership() error {
	return s.seedVersionLexerOwnershipMode(false)
}

// seedVersionLexerOwnershipMode can admit one exact mixed recovery frontier.
// Unshifted versions start at the shared election's before cursor. A shifted
// absorber owns the shared after cursor that it already consumed.
func (s *diagnosticParserCoreGenericScheduler) seedVersionLexerOwnershipMode(
	allowShiftedRecovery bool,
) error {
	if s == nil || !s.versionLexerBeforeValid || s.tokenSource == nil || s.tokenSource.language == nil {
		return errors.New("parser-core phase zero: version lexer seed lacks an election snapshot")
	}
	if s.versionLexerBeforeElection != s.electionIndex || s.versionLexerBeforeCheckpoint != s.checkpointBeforeID {
		return errors.New("parser-core phase zero: version lexer seed snapshot belongs to another election")
	}
	var beforeSeed *diagnosticParserCoreVersionLexerSnapshot
	var afterSeed *diagnosticParserCoreVersionLexerSnapshot
	var afterDFA dfaRelexSnapshot
	if allowShiftedRecovery {
		// The exact S5 mixed frontier stays inside one parser run. Its owned
		// snapshots bind the live language, scanner implementation, compact
		// generation, and serialized scanner payload. It does not require the
		// stronger cross-tree checkpoint identity used by incremental reuse.
		if s.tokenSource.lexer == nil || s.tokenSource.lexer.pos != int(s.token.EndByte) {
			return errors.New("parser-core phase zero: mixed recovery lexer seed lacks an exact shared after cursor")
		}
		afterDFA = s.tokenSource.snapshotRelexState()
	}
	err := s.withVersionLexerOwner(func(owner core.SchedulerTransactionToken) error {
		var seedErr error
		beforeSeed, seedErr = s.newVersionLexerSnapshot(owner, s.versionLexerBefore, s.checkpointBeforeID, s.checkpointBeforeID)
		if seedErr != nil || !allowShiftedRecovery {
			return seedErr
		}
		afterSeed = s.equivalentVersionLexerSnapshot(afterDFA, s.checkpointID, s.checkpointID)
		if afterSeed != nil {
			return nil
		}
		afterSeed, seedErr = s.newVersionLexerSnapshot(owner, afterDFA, s.checkpointID, s.checkpointID)
		return seedErr
	})
	if err != nil {
		return err
	}
	if err := beforeSeed.validateDestination(s.compact, s.tokenSource.language); err != nil {
		return err
	}
	if allowShiftedRecovery {
		if err := afterSeed.validateDestination(s.compact, s.tokenSource.language); err != nil {
			return err
		}
	}
	type seedStateKey struct {
		snapshot                *diagnosticParserCoreVersionLexerSnapshot
		region                  *diagnosticParserCoreS3Region
		recoveryGroup           uint64
		missingGroup            uint64
		recoveryNodeBaseline    uint32
		recoveryNodeBaselineSet bool
	}
	states := make(map[seedStateKey]*diagnosticParserCoreVersionState)
	for index := range s.headers {
		header := &s.headers[index]
		if header.accepted {
			continue
		}
		baseline, baselineSet := header.recoveryNodeBaseline()
		seed := beforeSeed
		if header.shifted {
			if !allowShiftedRecovery || !header.isRecoveryLineage() {
				return errors.New("parser-core phase zero: shifted lexer seed is not an owned recovery absorber")
			}
			seed = afterSeed
		}
		key := seedStateKey{
			snapshot: seed, region: header.recoveryRegion(), recoveryGroup: header.recoveryGroupIdentity(),
			missingGroup:         header.recoveryMissingGroupIdentity(),
			recoveryNodeBaseline: baseline, recoveryNodeBaselineSet: baselineSet,
		}
		state := states[key]
		if state == nil {
			header.publishVersionState(
				key.region, key.snapshot, 0, key.recoveryGroup, key.missingGroup,
				key.recoveryNodeBaseline, key.recoveryNodeBaselineSet,
			)
			state = header.versionState
			states[key] = state
		}
		header.versionState = state
		s.work.add(&s.work.PerVersionLexPublications, 1)
	}
	if uint64(len(s.headers)) > s.work.PeakLiveVersions {
		s.work.PeakLiveVersions = uint64(len(s.headers))
	}
	return nil
}

// requestVersionLexerHeader restores one exact owned cursor, sets its parser
// state, and performs exactly one lexer request. Interner writes occur before
// the short publication transaction on ordinary scheduler calls.
func (s *diagnosticParserCoreGenericScheduler) requestVersionLexerHeader(index int) error {
	if s == nil || s.tokenSource == nil || s.tokenSource.language == nil || index < 0 || index >= len(s.headers) {
		return errors.New("parser-core phase zero: version lexer request context is incomplete")
	}
	if existing := s.versionLexerRequestForHeader(index); existing != nil {
		return nil
	}
	header := &s.headers[index]
	base := header.versionLexerSnapshot()
	if base == nil {
		return errors.New("parser-core phase zero: version lexer header has no owned cursor snapshot")
	}
	// A request is a speculative read. Restore the shared source on every exit,
	// including snapshot validation, checkpoint, and publication failures.
	// Keep parser state and GLR state with the DFA cursor: the next shared
	// election must observe exactly the state that preceded this request.
	priorDFA := s.tokenSource.snapshotRelexState()
	priorState := s.tokenSource.state
	priorGLRStates := s.tokenSource.glrStates
	defer func() {
		priorDFA.restore(s.tokenSource)
		s.tokenSource.SetParserState(priorState)
		s.tokenSource.SetGLRStates(priorGLRStates)
	}()
	if err := base.restore(s.compact, s.tokenSource); err != nil {
		return err
	}
	s.work.add(&s.work.PerVersionLexRestores, 1)
	state, _, err := s.compact.Boundary(header.head)
	if err != nil {
		return err
	}
	s.tokenSource.SetParserState(StateID(state))
	s.tokenSource.SetGLRStates(nil)
	beforeID := base.afterCheckpoint
	token := s.tokenSource.Next()
	afterDFA := s.tokenSource.snapshotRelexState()
	afterBytes := s.tokenSource.captureExternalScannerStateInto(s.scannerScratch)
	afterID, _, err := diagnosticParserCoreInternCheckpoint(s.compact, afterBytes)
	if err != nil {
		return err
	}
	beforeSnapshot := base
	afterSnapshot := s.equivalentVersionLexerSnapshot(afterDFA, afterID, afterID)
	if err := s.withVersionLexerOwner(func(owner core.SchedulerTransactionToken) error {
		if afterSnapshot == nil {
			var snapshotErr error
			afterSnapshot, snapshotErr = s.newVersionLexerSnapshot(owner, afterDFA, afterID, afterID)
			if snapshotErr != nil {
				return snapshotErr
			}
		}
		return nil
	}); err != nil {
		return err
	}
	request := diagnosticParserCoreVersionLexerRequest{
		electionIndex:     s.electionIndex,
		headerCreationSeq: header.creationSeq,
		state:             StateID(state),
		token:             token,
		before:            beforeSnapshot,
		after:             afterSnapshot,
		beforeCheckpoint:  beforeSnapshot.afterCheckpointInfo,
		afterCheckpoint:   afterSnapshot.afterCheckpointInfo,
		beforeID:          beforeID,
		afterID:           afterID,
		raggedSpan: s.electionIndex == s.versionLexerBeforeElection &&
			(token.StartByte != s.token.StartByte || token.EndByte != s.token.EndByte),
		valid: true,
	}
	s.versionLexerRequests = append(s.versionLexerRequests, request)
	requestReference := uint32(len(s.versionLexerRequests))
	if err := s.withVersionLexerOwner(func(owner core.SchedulerTransactionToken) error {
		return s.publishVersionLexerRequestOwned(owner, index, beforeSnapshot, requestReference)
	}); err != nil {
		clear(s.versionLexerRequests[len(s.versionLexerRequests)-1:])
		s.versionLexerRequests = s.versionLexerRequests[:len(s.versionLexerRequests)-1]
		return err
	}
	// Header checkpoint identity follows the classified election's current
	// scanner phase. The request retains the separate before cursor.
	header.checkpoint = afterID
	s.work.add(&s.work.PerVersionLexPublications, 1) // the after snapshot is a sidecar publication
	s.work.add(&s.work.PerVersionLexRequests, 1)
	if uint64(len(s.headers)) > s.work.PeakLiveVersions {
		s.work.PeakLiveVersions = uint64(len(s.headers))
	}
	if s.fullReceipts() {
		s.receipt.VersionLexerRequests = append(s.receipt.VersionLexerRequests, DiagnosticParserCoreVersionLexerRequest{
			ElectionIndex: s.electionIndex, HeaderCreationSeq: request.headerCreationSeq,
			State: request.state, Token: request.token, InternalDFAToken: request.token.lexerInternalDFALexed,
			ScannerBefore: request.beforeCheckpoint, ScannerAfter: request.afterCheckpoint,
		})
	}
	return nil
}

// requestHeaderLexerToken names the scheduler-owned one-request operation.
// Keep requestVersionLexerHeader as the implementation name for callers that
// describe the operation by its target header rather than its lexer result.
func (s *diagnosticParserCoreGenericScheduler) requestHeaderLexerToken(index int) error {
	return s.requestVersionLexerHeader(index)
}

// captureSharedElectionSnapshot records the scanner state around the shared
// election. It does not change the shared path or publish a version.
func (s *diagnosticParserCoreGenericScheduler) captureSharedElectionSnapshot() error {
	if s == nil || s.tokenSource == nil {
		return errors.New("parser-core phase zero: shared lexer snapshot source is unavailable")
	}
	s.versionLexerBefore = s.tokenSource.snapshotRelexStateWithScratch(&s.versionLexerBeforeScratch)
	return s.finishSharedElectionSnapshotCapture()
}

// captureSharedElectionSnapshotFromExternalPayload uses checkpoint bytes that
// the election already serialized. This keeps one Serialize call per election.
func (s *diagnosticParserCoreGenericScheduler) captureSharedElectionSnapshotFromExternalPayload(externalPayload []byte) error {
	if s == nil || s.tokenSource == nil {
		return errors.New("parser-core phase zero: shared lexer snapshot source is unavailable")
	}
	s.versionLexerBefore = s.tokenSource.snapshotRelexStateWithScratchFromExternalPayload(
		&s.versionLexerBeforeScratch,
		externalPayload,
	)
	return s.finishSharedElectionSnapshotCapture()
}

func (s *diagnosticParserCoreGenericScheduler) finishSharedElectionSnapshotCapture() error {
	s.versionLexerBeforeValid = true
	s.versionLexerBeforeIdentity = [32]byte{}
	s.versionLexerBeforeIdentityValid = false
	if s.tokenSource != nil && s.tokenSource.language != nil && languageUsesExternalScannerCheckpoints(s.tokenSource.language) {
		if provider, ok := externalScannerCheckpointIdentityProviderForScanner(s.tokenSource.language.ExternalScanner); ok {
			if identity, ok := provider.CheckpointIdentity(); ok && identity.complete() {
				s.versionLexerBeforeIdentity = parserCoreExternalScannerIdentityFingerprint(identity)
				s.versionLexerBeforeIdentityValid = true
			}
		}
	}
	s.versionLexerBeforeElection = s.electionIndex + 1
	s.versionLexerBeforeCheckpoint = s.checkpointID
	return nil
}

// activateVersionLexerOwnershipAtRagged starts the owned-cursor tranche at
// the first different-span relex. Keep activation separate from action
// execution: the shared pass must not reduce a sibling after another header
// proves that its lexer view has a different width. The next scheduler pass
// dispatches the complete owned request set.
const diagnosticParserCoreOwnedDispatchPendingDetail = "generic scheduler owned dispatch pending"

// DiagnosticParserCoreOwnedDispatchPendingDetailForTest exposes the stable
// activation prefix without exposing scheduler internals.
func DiagnosticParserCoreOwnedDispatchPendingDetailForTest() string {
	return diagnosticParserCoreOwnedDispatchPendingDetail
}

func (s *diagnosticParserCoreGenericScheduler) activateVersionLexerOwnershipAtRagged(
	raggedHeaderIndex int,
) (stop *diagnosticParserCoreGenericUnsupported, err error) {
	if s == nil || raggedHeaderIndex < 0 || raggedHeaderIndex >= len(s.headers) {
		return nil, errors.New("parser-core phase zero: ragged lexer ownership header index is out of range")
	}
	if s.versionLexerOwnershipActive {
		return &diagnosticParserCoreGenericUnsupported{
			boundary:    DiagnosticParserCoreRoute,
			detail:      diagnosticParserCoreOwnedDispatchPendingDetail + ": ownership is already active",
			headerIndex: raggedHeaderIndex,
		}, nil
	}
	mixedRecovery := s5MissingLineageNeedsOwnedLexer(s, []int{raggedHeaderIndex}) ||
		s.recoveryAmbiguityNeedsOwnedLexer(raggedHeaderIndex)
	// An open recovery region owns its scanner and parser frontier as one
	// recovery lineage. Do not mix a new lexer-ownership tranche into any
	// sibling unless this is the exact S5 absorber/missing frontier.
	for index, header := range s.headers {
		if header.recoveryRegion() != nil && (!mixedRecovery || !header.shifted || index == raggedHeaderIndex) {
			return &diagnosticParserCoreGenericUnsupported{
				boundary:    DiagnosticParserCoreRoute,
				detail:      diagnosticParserCoreOwnedDispatchPendingDetail + ": ragged ownership has an open recovery region",
				headerIndex: index,
			}, nil
		}
	}
	// A shifted ordinary header cannot be seeded at this election's token
	// start. The exact S5 absorber instead receives the shared after cursor.
	for index, header := range s.headers {
		if header.shifted && !header.accepted && !mixedRecovery {
			return &diagnosticParserCoreGenericUnsupported{
				boundary:    DiagnosticParserCoreRoute,
				detail:      diagnosticParserCoreOwnedDispatchPendingDetail + ": ragged ownership has a shifted sibling",
				headerIndex: index,
			}, nil
		}
	}
	if err = s.headerRollbackScratch.begin(s.headers); err != nil {
		return nil, err
	}
	requestsBefore := len(s.versionLexerRequests)
	receiptRequestsBefore := 0
	if s.receipt != nil {
		receiptRequestsBefore = len(s.receipt.VersionLexerRequests)
	}
	workBefore := s.work
	rollback := true
	defer func() {
		s.headerRollbackScratch.finish(&s.headers, rollback)
		if !rollback {
			return
		}
		clear(s.versionLexerRequests[requestsBefore:])
		s.versionLexerRequests = s.versionLexerRequests[:requestsBefore]
		if s.receipt != nil {
			s.receipt.VersionLexerRequests = s.receipt.VersionLexerRequests[:receiptRequestsBefore]
		}
		s.work = workBefore
		s.versionLexerOwnershipActive = false
	}()
	if err = s.seedVersionLexerOwnershipMode(mixedRecovery); err != nil {
		return nil, err
	}
	for index := range s.headers {
		if s.headers[index].accepted || s.headers[index].shifted {
			continue
		}
		if err = s.requestHeaderLexerToken(index); err != nil {
			return nil, err
		}
	}
	s.versionLexerOwnershipActive = true
	rollback = false
	return nil, nil
}

// recoveryAmbiguityNeedsOwnedLexer admits a recovery frontier after an
// ordinary grammar fork. Dispatch classifies every header before it executes
// any shift, so each admitted header must still own the shared before cursor.
func (s *diagnosticParserCoreGenericScheduler) recoveryAmbiguityNeedsOwnedLexer(witnessIndex int) bool {
	if s == nil || witnessIndex < 0 || witnessIndex >= len(s.headers) ||
		s.work.RecoveryAmbiguityForks == 0 || !s.competingRecoveryFrontier() ||
		s.headers[witnessIndex].shifted || s.headers[witnessIndex].accepted ||
		s.headers[witnessIndex].recoveryRegion() != nil {
		return false
	}
	for index := range s.headers {
		header := &s.headers[index]
		if header.shifted || header.accepted || header.paused ||
			header.checkpoint != s.checkpointID || header.recoveryRegion() != nil {
			return false
		}
	}
	return true
}

func (s *diagnosticParserCoreGenericScheduler) bindVersionLexerRequest(
	request *diagnosticParserCoreVersionLexerRequest,
) error {
	if s == nil || request == nil || !request.valid || request.electionIndex != s.electionIndex {
		return errors.New("parser-core phase zero: cannot bind an invalid version lexer request")
	}
	if _, _, ok := s.compact.CheckpointReceipt(request.beforeID); !ok {
		return errDiagnosticParserCoreUnknownCheckpointIdentity
	}
	if _, _, ok := s.compact.CheckpointReceipt(request.afterID); !ok {
		return errDiagnosticParserCoreUnknownCheckpointIdentity
	}
	if err := s.compact.SetPhaseExternalTokenScannerCheckpoints(request.beforeID, request.afterID); err != nil {
		return err
	}
	s.token = request.token
	s.checkpointBeforeID = request.beforeID
	s.checkpointID = request.afterID
	s.checkpoint = request.afterCheckpoint
	s.currentElection = DiagnosticParserCoreElection{
		Token:                  request.token,
		ScannerBefore:          request.beforeCheckpoint,
		ScannerAfter:           request.afterCheckpoint,
		CurrentCheckpointValid: true,
		CurrentCheckpointStart: request.beforeCheckpoint,
		CurrentCheckpointEnd:   request.afterCheckpoint,
		CurrentCheckpointBytes: [2]uint32{request.token.StartByte, request.token.EndByte},
	}
	s.tokenCell = diagnosticParserCoreTokenCell{
		token: request.token, state: request.state, byteOffset: request.token.StartByte,
		beforeCheckpoint: request.beforeID, afterCheckpoint: request.afterID, valid: true,
	}
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) withVersionLexerRequest(
	cell diagnosticParserCoreGenericCell,
	fn func() error,
) (err error) {
	request, err := s.versionLexerRequestForCell(cell)
	if err != nil {
		return err
	}
	if request == nil {
		return fn()
	}
	tokenBefore := s.token
	checkpointBefore := s.checkpoint
	checkpointStartBefore, checkpointEndBefore := s.checkpointBeforeID, s.checkpointID
	electionBefore := s.currentElection
	tokenCellBefore := s.tokenCell
	phaseCheckpointBefore, phaseStartBefore, phaseEndBefore, phaseExactBefore := s.compact.PhaseScannerCheckpoints()
	restore := func() error {
		s.token = tokenBefore
		s.checkpoint = checkpointBefore
		s.checkpointBeforeID, s.checkpointID = checkpointStartBefore, checkpointEndBefore
		s.currentElection = electionBefore
		s.tokenCell = tokenCellBefore
		if phaseExactBefore {
			return s.compact.SetPhaseExternalTokenScannerCheckpoints(phaseStartBefore, phaseEndBefore)
		}
		return s.compact.SetPhaseCheckpoint(phaseCheckpointBefore)
	}
	if err = s.bindVersionLexerRequest(request); err != nil {
		return err
	}
	defer func() {
		panicValue := recover()
		restoreErr := restore()
		if panicValue != nil {
			if restoreErr != nil {
				panic(fmt.Errorf(
					"parser-core phase zero: version lexer callback panic=%v; restore: %w",
					panicValue,
					restoreErr,
				))
			}
			panic(panicValue)
		}
		if restoreErr != nil {
			wrapped := fmt.Errorf("parser-core phase zero: restore version lexer request: %w", restoreErr)
			if err != nil {
				err = errors.Join(err, wrapped)
			} else {
				err = wrapped
			}
		}
	}()
	err = fn()
	return err
}

func (s *diagnosticParserCoreGenericScheduler) publishVersionLexerShiftOnHeaderOwned(
	owner core.SchedulerTransactionToken,
	header *diagnosticParserCoreHeader,
	request *diagnosticParserCoreVersionLexerRequest,
) error {
	if s == nil || header == nil || request == nil || request.after == nil {
		return errors.New("parser-core phase zero: version lexer shift publication is incomplete")
	}
	if err := request.after.validateDestination(s.compact, s.tokenSource.language); err != nil {
		return err
	}
	if _, _, ok := s.compact.CheckpointReceiptOwned(owner, request.after.beforeCheckpoint); !ok {
		return errDiagnosticParserCoreUnknownCheckpointIdentity
	}
	if _, _, ok := s.compact.CheckpointReceiptOwned(owner, request.after.afterCheckpoint); !ok {
		return errDiagnosticParserCoreUnknownCheckpointIdentity
	}
	if err := s.installEquivalentVersionLexerState(header, request.after, 0, nil); err != nil {
		return err
	}
	header.checkpoint = request.afterID
	s.work.add(&s.work.PerVersionLexPublications, 1)
	if request.raggedSpan {
		s.work.add(&s.work.PerVersionLexAcceptedRaggedSpans, 1)
	}
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) beginNextVersionLexerElection() error {
	if s == nil || !s.versionLexerOwnershipActive || len(s.headers) < 2 {
		return errors.New("parser-core phase zero: next version lexer election requires multiple owned headers")
	}
	for index := range s.headers {
		header := s.headers[index]
		if header.accepted || !header.shifted || header.versionLexerSnapshot() == nil {
			return errors.New("parser-core phase zero: next version lexer election requires a closed owned frontier")
		}
		if err := header.versionLexerSnapshot().validateDestination(s.compact, s.tokenSource.language); err != nil {
			return err
		}
	}
	if err := s.headerRollbackScratch.begin(s.headers); err != nil {
		return err
	}
	requestsBefore := len(s.versionLexerRequests)
	receiptRequestsBefore := 0
	if s.receipt != nil {
		receiptRequestsBefore = len(s.receipt.VersionLexerRequests)
	}
	electionBefore := s.electionIndex
	workBefore := s.work
	noLookaheadStepsBefore := s.noLookaheadSteps
	requireEOFBefore := s.requireEOFPostNoLookaheadRoot
	s.electionIndex++
	rollback := true
	defer func() {
		s.headerRollbackScratch.finish(&s.headers, rollback)
		if !rollback {
			return
		}
		clear(s.versionLexerRequests[requestsBefore:])
		s.versionLexerRequests = s.versionLexerRequests[:requestsBefore]
		if s.receipt != nil {
			s.receipt.VersionLexerRequests = s.receipt.VersionLexerRequests[:receiptRequestsBefore]
		}
		s.electionIndex = electionBefore
		s.work = workBefore
		s.noLookaheadSteps = noLookaheadStepsBefore
		s.requireEOFPostNoLookaheadRoot = requireEOFBefore
	}()
	for index := range s.headers {
		header := &s.headers[index]
		header.shifted = false
		header.paused = false
		header.frontierSequence = 0
		if err := s.requestHeaderLexerToken(index); err != nil {
			return err
		}
	}
	requestNoLookahead := false
	requestNoLookaheadSet := false
	for index := range s.headers {
		reference := s.headers[index].versionLexerRequestReference()
		request := s.versionLexerRequestForHeader(index)
		if int(reference) <= requestsBefore || request == nil {
			return errors.New("parser-core phase zero: next version lexer request was not authenticated")
		}
		if !requestNoLookaheadSet {
			requestNoLookahead = request.token.NoLookahead
			requestNoLookaheadSet = true
		} else if request.token.NoLookahead != requestNoLookahead {
			return &diagnosticParserCoreDecline{
				boundary: DiagnosticParserCoreRoute,
				detail:   "generic scheduler owned lexer election mixed no-lookahead and ordinary tokens",
			}
		}
		if request.token.NoLookahead &&
			(request.token.Symbol != 0 || request.token.StartByte != request.token.EndByte ||
				request.token.Missing || request.token.ExternalScannerToken ||
				request.beforeCheckpoint != request.afterCheckpoint) {
			return &diagnosticParserCoreDecline{
				boundary: DiagnosticParserCoreRoute,
				detail:   "generic scheduler owned lexer no-lookahead token is not authenticated synthetic EOF",
			}
		}
		if requireEOFBefore &&
			(request.token.Symbol != 0 || request.token.StartByte != request.token.EndByte ||
				request.token.Missing || request.token.NoLookahead || request.token.ExternalScannerToken) {
			return &diagnosticParserCoreDecline{
				boundary: DiagnosticParserCoreRoute,
				detail:   "generic scheduler root reduction on no-lookahead was not followed by authenticated EOF",
			}
		}
	}
	if requestNoLookahead {
		if s.noLookaheadSteps == math.MaxUint8 {
			return &diagnosticParserCoreDecline{
				boundary: DiagnosticParserCoreCap,
				detail:   "generic scheduler no-lookahead re-election cap",
			}
		}
		s.noLookaheadSteps++
		if s.noLookaheadSteps > maxDiagnosticParserCoreNoLookaheadSteps {
			return &diagnosticParserCoreDecline{
				boundary: DiagnosticParserCoreCap,
				detail:   "generic scheduler no-lookahead re-election cap",
			}
		}
	} else {
		s.noLookaheadSteps = 0
	}
	// Build every owned request before advancing the compact frontier. A
	// request failure can then restore the scheduler without leaving the Core
	// in a new authentication generation.
	if err := s.compact.BeginFrontier(); err != nil {
		return err
	}
	newRequestCount := len(s.versionLexerRequests) - requestsBefore
	copy(s.versionLexerRequests, s.versionLexerRequests[requestsBefore:])
	clear(s.versionLexerRequests[newRequestCount:])
	s.versionLexerRequests = s.versionLexerRequests[:newRequestCount]
	for index := range s.headers {
		header := &s.headers[index]
		reference := header.versionLexerRequestReference()
		baseline, baselineSet := header.recoveryNodeBaseline()
		header.publishVersionState(
			header.recoveryRegion(), header.versionLexerSnapshot(),
			reference-uint32(requestsBefore), header.recoveryGroupIdentity(),
			header.recoveryMissingGroupIdentity(), baseline, baselineSet,
		)
	}
	s.epochProgress = false
	if requireEOFBefore {
		s.requireEOFPostNoLookaheadRoot = false
	}
	rollback = false
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) rejoinSharedLexerFromOwnedHeader() error {
	if s == nil || !s.versionLexerOwnershipActive || len(s.headers) != 1 {
		return errors.New("parser-core phase zero: shared lexer rejoin requires one owned header")
	}
	if s.tokenSource == nil || s.tokenSource.lexer == nil {
		return errors.New("parser-core phase zero: shared lexer rejoin token source is unavailable")
	}
	header := &s.headers[0]
	snapshot := header.versionLexerSnapshot()
	if !header.shifted || header.accepted || snapshot == nil {
		return errors.New("parser-core phase zero: shared lexer rejoin requires one shifted owned header")
	}
	if err := snapshot.validate(); err != nil {
		return err
	}
	state, _, err := s.compact.Boundary(header.head)
	if err != nil {
		return err
	}
	priorDFA := s.tokenSource.snapshotRelexState()
	priorState := s.tokenSource.state
	priorGLRStates := s.tokenSource.glrStates
	committed := false
	defer func() {
		if committed {
			return
		}
		priorDFA.restore(s.tokenSource)
		s.tokenSource.SetParserState(priorState)
		s.tokenSource.SetGLRStates(priorGLRStates)
	}()
	if err := snapshot.restore(s.compact, s.tokenSource); err != nil {
		return err
	}
	s.tokenSource.SetParserState(StateID(state))
	s.tokenSource.SetGLRStates(nil)
	if err := s.compact.SetPhaseExternalTokenScannerCheckpoints(snapshot.beforeCheckpoint, snapshot.afterCheckpoint); err != nil {
		return err
	}
	// Every remaining operation is an assignment or a bounded slice clear.
	// Commit the token source after the last operation that can return an error.
	committed = true
	s.checkpointBeforeID = snapshot.beforeCheckpoint
	s.checkpointID = snapshot.afterCheckpoint
	s.checkpoint = snapshot.afterCheckpointInfo
	header.checkpoint = snapshot.afterCheckpoint
	header.clearVersionLexerSnapshot()
	s.clearVersionLexerRequests()
	s.versionLexerOwnershipActive = false
	return nil
}

type diagnosticParserCoreDispatchScratch struct {
	busy            bool
	cells           []diagnosticParserCoreGenericCell
	noActionIndices []int
}

// diagnosticParserCoreHeaderRollbackScratch retains the pre-operation header
// frontier while one scheduler mutation is in flight. Scheduler operations are
// deliberately non-reentrant, so one bounded buffer can serve every accept,
// reduction, conflict, ordinary-shift, and extra-shift transaction in a parse.
// The inline buffer covers the common one-to-eight-header frontier. Wider
// frontiers retain the existing heap-backed growth path.
//
// Header values can own immutable per-version state. Clear the populated
// snapshot range when the scratch operation finishes.
type diagnosticParserCoreHeaderRollbackScratch struct {
	inline  [8]diagnosticParserCoreHeader
	busy    bool
	headers []diagnosticParserCoreHeader
}

func (scratch *diagnosticParserCoreHeaderRollbackScratch) begin(headers []diagnosticParserCoreHeader) error {
	if scratch == nil {
		return errors.New("parser-core phase zero: nil header rollback scratch")
	}
	if scratch.busy {
		return errors.New("parser-core phase zero: reentrant header rollback snapshot")
	}
	scratch.busy = true
	if cap(scratch.headers) < len(headers) {
		if len(headers) <= len(scratch.inline) {
			scratch.headers = scratch.inline[:len(headers)]
		} else {
			capacity := max(len(headers), cap(scratch.headers)*2)
			scratch.headers = make([]diagnosticParserCoreHeader, len(headers), capacity)
		}
	} else {
		scratch.headers = scratch.headers[:len(headers)]
	}
	copy(scratch.headers, headers)
	return nil
}

func (scratch *diagnosticParserCoreHeaderRollbackScratch) finish(headers *[]diagnosticParserCoreHeader, rollback bool) {
	if scratch == nil || !scratch.busy {
		return
	}
	snapshot := scratch.headers
	if rollback && headers != nil {
		restored := *headers
		aliasesSnapshot := len(restored) != 0 && len(snapshot) != 0 && &restored[0] == &snapshot[0]
		if cap(restored) < len(snapshot) {
			if !aliasesSnapshot {
				clear(restored)
			}
			restored = make([]diagnosticParserCoreHeader, len(scratch.headers))
		} else {
			clearDiagnosticParserCoreHeaderSuffix(restored, len(scratch.headers))
			restored = restored[:len(scratch.headers)]
		}
		copy(restored, scratch.headers)
		*headers = restored
	}
	clear(snapshot)
	scratch.headers = scratch.headers[:0]
	scratch.busy = false
}

func (scratch *diagnosticParserCoreHeaderRollbackScratch) reset() {
	if scratch == nil {
		return
	}
	clear(scratch.headers)
	scratch.headers = nil
	scratch.busy = false
}

func (scratch *diagnosticParserCoreDispatchScratch) begin() error {
	if scratch.busy {
		return errors.New("parser-core phase zero: reentrant generic scheduler dispatch")
	}
	scratch.busy = true
	scratch.cells = scratch.cells[:0]
	scratch.noActionIndices = scratch.noActionIndices[:0]
	return nil
}

func (scratch *diagnosticParserCoreDispatchScratch) finish() {
	clear(scratch.cells)
	scratch.cells = scratch.cells[:0]
	scratch.noActionIndices = scratch.noActionIndices[:0]
	scratch.busy = false
}

// executeDiagnosticParserCoreGenericSchedulerFromSeed owns the compact seed
// and the scheduler lifecycle before the first production DFA/scanner
// election. It intentionally does not wrap the parse in one arena-wide atomic
// transaction: each scheduler operation retains its own bounded publication
// contract, while this fresh diagnostic core has no caller state to restore.
func executeDiagnosticParserCoreGenericSchedulerFromSeed(
	compact *core.Core,
	tokenSource *dfaTokenSource,
	scannerScratch *[]byte,
	initialState StateID,
	options DiagnosticParserCorePrefixOptions,
	observer diagnosticParserCoreSeedObserver,
) (*diagnosticParserCoreGenericScheduler, error) {
	return executeDiagnosticParserCoreGenericSchedulerFromSeedInto(
		nil, compact, tokenSource, scannerScratch, initialState, options, observer,
	)
}

func executeDiagnosticParserCoreGenericSchedulerFromSeedInto(
	scheduler *diagnosticParserCoreGenericScheduler,
	compact *core.Core,
	tokenSource *dfaTokenSource,
	scannerScratch *[]byte,
	initialState StateID,
	options DiagnosticParserCorePrefixOptions,
	observer diagnosticParserCoreSeedObserver,
) (*diagnosticParserCoreGenericScheduler, error) {
	if compact == nil || tokenSource == nil || scannerScratch == nil {
		return nil, errors.New("parser-core phase zero: seed scheduler requires compact core and production token source")
	}
	tokenSource.SetParserState(initialState)
	tokenSource.SetGLRStates(nil)
	initialCheckpoint := tokenSource.captureExternalScannerStateInto(scannerScratch)
	initialCheckpointID, initialCheckpointReceipt, err := diagnosticParserCoreInternCheckpoint(compact, initialCheckpoint)
	if err != nil {
		return nil, err
	}
	if err := compact.SetPhaseCheckpoint(initialCheckpointID); err != nil {
		return nil, err
	}
	// Reserve the five record arenas from the source length before the seed
	// publishes the first node, so a cold parse starts at its expected
	// capacity instead of growing there from zero. The ceiling comes from
	// compactArenaReserveBytes (parsercore_phase0_arena_reserve.go); the
	// estimator is Core.ReserveRecordArenas
	// (internal/parsercorephase0/arena_reserve.go). This is the one seam
	// every compact route passes through: the shipped admission-candidate
	// route, the diagnostic prefix route, and the benchmark runners all reach
	// the scheduler here. Pure capacity -- no length, no record, and no Work
	// counter moves -- so it cannot change a parse result.
	compact.ReserveRecordArenas(
		tokenSource.sourceLength(),
		compactArenaReserveBytes(options.stopControlMemoryBudgetBytes, options.stopControlHardCeilingBytes),
	)
	head, err := compact.Seed(core.StateID(initialState), 0)
	if err != nil {
		return nil, err
	}
	if scheduler == nil {
		scheduler, err = newDiagnosticParserCoreGenericScheduler(
			compact, tokenSource, scannerScratch, head, initialCheckpointID, initialCheckpointReceipt, observer, options,
		)
	} else {
		scheduler, err = initializeDiagnosticParserCoreGenericScheduler(
			scheduler, compact, tokenSource, scannerScratch, head, initialCheckpointID, initialCheckpointReceipt, observer, options,
		)
	}
	if err != nil {
		return nil, err
	}
	defer scheduler.headerRollbackScratch.reset()
	run := scheduler.run
	if options.freshSchedulerSession {
		run = func() error {
			return compact.RunFreshSchedulerSession(func(owner core.SchedulerTransactionToken) error {
				scheduler.freshSessionOwner = &owner
				defer func() { scheduler.freshSessionOwner = nil }()
				if options.recordDropCohortCertificates {
					if err := compact.SetHistoricalCertificateAuthenticationOwned(owner, true); err != nil {
						return err
					}
				}
				if err := scheduler.persistHeaderLineageOwned(owner); err != nil {
					return err
				}
				return scheduler.run()
			})
		}
	}
	if err := run(); err != nil {
		return scheduler, err
	}
	return scheduler, nil
}

const diagnosticParserCorePointCacheSize = 16

type diagnosticParserCorePointCacheEntry struct {
	offset uint32
	point  Point
}

type diagnosticParserCorePointIndex struct {
	lineStarts []uint32
	cache      [diagnosticParserCorePointCacheSize]diagnosticParserCorePointCacheEntry
	valid      uint16
}

// diagnosticParserCoreMaterializationScratch retains parent-build storage for
// one accepted-tree materialization. Parent construction consumes entries
// synchronously and copies every surviving child/field slice into the result
// arena, so the next postorder parent may safely reuse both buffers.
type diagnosticParserCoreMaterializationScratch struct {
	entries []stackEntry
	reduce  reduceBuildScratch
}

func (scratch *diagnosticParserCoreMaterializationScratch) entriesFor(width int) []stackEntry {
	if width <= 0 {
		scratch.entries = scratch.entries[:0]
		return nil
	}
	if cap(scratch.entries) < width {
		capacity := max(width, cap(scratch.entries)*2)
		scratch.entries = make([]stackEntry, width, capacity)
		return scratch.entries
	}
	scratch.entries = scratch.entries[:width]
	return scratch.entries
}

func (scratch *diagnosticParserCoreMaterializationScratch) reset() {
	if scratch == nil {
		return
	}
	clear(scratch.entries[:cap(scratch.entries)])
	scratch.entries = scratch.entries[:0]
	// Clear the reduce node backing to its full capacity before the shared reset.
	// This scratch is retained on the runner across parses, so a stale *Node
	// beyond len (which reduceBuildScratch.reset leaves in place for the pooled
	// production path) would pin a released arena slab and defeat GC. Production
	// discards its per-parse scratch instead, so it never retains these pointers.
	if cap(scratch.reduce.nodes) > 0 {
		clear(scratch.reduce.nodes[:cap(scratch.reduce.nodes)])
	}
	scratch.reduce.reset()
}

func withDiagnosticParserCoreMaterializationScratch(parser *Parser, visit func(*diagnosticParserCoreMaterializationScratch) error) (err error) {
	var scratch diagnosticParserCoreMaterializationScratch
	return withProvidedMaterializationScratch(parser, &scratch, visit)
}

// withProvidedMaterializationScratch runs visit with a caller-owned
// materialization scratch installed as the parser's reduce scratch. It resets
// the scratch on return so a runner-held scratch is safe to reuse for the next
// parse. Passing a reused scratch keeps the warm steady state from allocating a
// fresh reduce-build buffer on every parse.
func withProvidedMaterializationScratch(parser *Parser, scratch *diagnosticParserCoreMaterializationScratch, visit func(*diagnosticParserCoreMaterializationScratch) error) (err error) {
	if parser == nil || visit == nil || scratch == nil {
		return errors.New("parser-core phase zero: materialization scratch requires a parser, scratch, and visitor")
	}
	previousReduceScratch := parser.reduceScratch
	parser.reduceScratch = &scratch.reduce
	defer func() {
		parser.reduceScratch = previousReduceScratch
		scratch.reset()
	}()
	return visit(scratch)
}

// parserCoreMaxRetainedNodeScratch caps the *Node scratch buffers the runner
// retains between parses so an unusually wide tree cannot pin a multi-megabyte
// backing array for the parser's whole lifetime. It mirrors production's
// maxRetainedNodeLinkStack bound in releaseParserScratch.
const parserCoreMaxRetainedNodeScratch = 256 * 1024

// parserCoreMaxRetainedLineStarts caps the retained line-start buffer.
const parserCoreMaxRetainedLineStarts = 256 * 1024

// parserCoreRunnerScratch retains the reusable per-Parser materialization
// buffers for the compact candidate route. The fresh-full runner is per-Parser
// and single-goroutine (see parserCoreFreshFullRunner), so retaining these
// buffers on it and resetting them per parse mirrors production's parser-held
// arena reuse: the warm steady state stops re-allocating the public-tree
// scratch on every parse.
type parserCoreRunnerScratch struct {
	materialization diagnosticParserCoreMaterializationScratch
	postorder       core.MaterializationPostorderScratch
	acceptedLeaves  diagnosticParserCoreAcceptedLeafCoverageScratch
	// recoveryTerminalAliasSymbol is valid only when the exact artifact rule
	// also set recoveryTerminalAliasCertified for this materialization.
	recoveryTerminalAliasSymbol    Symbol
	recoveryTerminalAliasCertified bool
	nodesByID                      []*Node
	hasErrorByID                   []bool
	nodes                          []*Node
	linkScratch                    []*Node
	lineStarts                     []uint32
	goCompatFrames                 []goCompatSubtreeFrame
}

// parserCoreMaxRetainedAcceptedLeafSpans bounds the caller-owned coverage
// storage retained by a parser after a wide accepted tree.
const parserCoreMaxRetainedAcceptedLeafSpans = 256 * 1024

type diagnosticParserCoreAcceptedLeafSpan struct {
	id        core.SubtreeID
	startByte uint32
	endByte   uint32
	hidden    bool
}

type diagnosticParserCoreAcceptedLeafCoverageScratch struct {
	spans                           []diagnosticParserCoreAcceptedLeafSpan
	authenticatedAliases            []*Node
	leadingLexerSkippedPrefixStarts []uint32
}

func (scratch *diagnosticParserCoreAcceptedLeafCoverageScratch) reset() {
	if scratch == nil {
		return
	}
	if cap(scratch.spans) > parserCoreMaxRetainedAcceptedLeafSpans {
		scratch.spans = nil
	} else {
		clear(scratch.spans)
		scratch.spans = scratch.spans[:0]
	}
	if cap(scratch.authenticatedAliases) > parserCoreMaxRetainedAcceptedLeafSpans {
		scratch.authenticatedAliases = nil
	} else {
		clear(scratch.authenticatedAliases)
		scratch.authenticatedAliases = scratch.authenticatedAliases[:0]
	}
	if cap(scratch.leadingLexerSkippedPrefixStarts) > parserCoreMaxRetainedAcceptedLeafSpans {
		scratch.leadingLexerSkippedPrefixStarts = nil
	} else {
		clear(scratch.leadingLexerSkippedPrefixStarts)
		scratch.leadingLexerSkippedPrefixStarts = scratch.leadingLexerSkippedPrefixStarts[:0]
	}
}

func (scratch *diagnosticParserCoreAcceptedLeafCoverageScratch) prepareLexerSkippedPrefixes(subtrees int) {
	if scratch == nil || subtrees <= 0 {
		return
	}
	if cap(scratch.leadingLexerSkippedPrefixStarts) < subtrees+1 {
		scratch.leadingLexerSkippedPrefixStarts = make([]uint32, subtrees+1)
		return
	}
	scratch.leadingLexerSkippedPrefixStarts = scratch.leadingLexerSkippedPrefixStarts[:subtrees+1]
	clear(scratch.leadingLexerSkippedPrefixStarts)
}

func (scratch *diagnosticParserCoreAcceptedLeafCoverageScratch) recordLexerSkippedPrefix(
	id core.SubtreeID,
	view core.MaterializationSubtreeView,
) {
	if scratch == nil || !view.Terminal || !view.LexerSkippedPrefix ||
		uint64(id) >= uint64(len(scratch.leadingLexerSkippedPrefixStarts)) {
		return
	}
	scratch.leadingLexerSkippedPrefixStarts[id] = view.LexerSkippedPrefixStart + 1
}

func (scratch *diagnosticParserCoreAcceptedLeafCoverageScratch) propagateLeadingLexerSkippedPrefix(
	id core.SubtreeID,
	startByte uint32,
	children []core.SubtreeID,
	nodesByID []*Node,
) {
	if scratch == nil || uint64(id) >= uint64(len(scratch.leadingLexerSkippedPrefixStarts)) {
		return
	}
	for _, childID := range children {
		if uint64(childID) >= uint64(len(nodesByID)) || uint64(childID) >= uint64(len(scratch.leadingLexerSkippedPrefixStarts)) {
			return
		}
		child := nodesByID[childID]
		if child == nil || child.startByte > startByte {
			return
		}
		if child.startByte < startByte {
			continue
		}
		if encoded := scratch.leadingLexerSkippedPrefixStarts[childID]; encoded != 0 {
			scratch.leadingLexerSkippedPrefixStarts[id] = encoded
			return
		}
		if child.endByte > startByte {
			return
		}
	}
}

func (scratch *diagnosticParserCoreAcceptedLeafCoverageScratch) childHasLexerSkippedPrefix(
	id core.SubtreeID,
	startByte, endByte uint32,
	nodesByID []*Node,
) bool {
	if scratch == nil || startByte >= endByte || uint64(id) >= uint64(len(scratch.leadingLexerSkippedPrefixStarts)) ||
		uint64(id) >= uint64(len(nodesByID)) || nodesByID[id] == nil || nodesByID[id].startByte != endByte {
		return false
	}
	encoded := scratch.leadingLexerSkippedPrefixStarts[id]
	return encoded != 0 && encoded-1 == startByte
}

func (scratch *diagnosticParserCoreAcceptedLeafCoverageScratch) append(
	id core.SubtreeID,
	view core.MaterializationSubtreeView,
	sourceLen uint32,
	hidden bool,
) error {
	if scratch == nil || !view.Terminal {
		return nil
	}
	if view.StartByte > view.EndByte || view.EndByte > sourceLen {
		return fmt.Errorf("terminal subtree span=%d..%d is outside source length %d", view.StartByte, view.EndByte, sourceLen)
	}
	if count := len(scratch.spans); count > 0 {
		previous := scratch.spans[count-1]
		if view.StartByte < previous.startByte {
			return fmt.Errorf("terminal subtree spans move backward: previous=%d..%d current=%d..%d", previous.startByte, previous.endByte, view.StartByte, view.EndByte)
		}
		if view.StartByte < previous.endByte {
			return fmt.Errorf("terminal subtree spans overlap: previous=%d..%d current=%d..%d", previous.startByte, previous.endByte, view.StartByte, view.EndByte)
		}
	}
	scratch.spans = append(scratch.spans, diagnosticParserCoreAcceptedLeafSpan{
		id:        id,
		startByte: view.StartByte,
		endByte:   view.EndByte,
		hidden:    hidden,
	})
	return nil
}

func (scratch *diagnosticParserCoreAcceptedLeafCoverageScratch) hasVisibleTerminalNode(startByte, endByte uint32, node *Node, nodesByID []*Node) bool {
	if scratch == nil {
		return false
	}
	index := sort.Search(len(scratch.spans), func(index int) bool {
		return scratch.spans[index].startByte >= startByte
	})
	for index < len(scratch.spans) && scratch.spans[index].startByte == startByte {
		span := scratch.spans[index]
		if !span.hidden && span.endByte == endByte && uint64(span.id) < uint64(len(nodesByID)) && nodesByID[span.id] == node {
			return true
		}
		index++
	}
	return false
}

func (scratch *diagnosticParserCoreAcceptedLeafCoverageScratch) hasAuthenticatedAlias(node *Node) bool {
	if scratch == nil {
		return false
	}
	for _, alias := range scratch.authenticatedAliases {
		if alias == node {
			return true
		}
	}
	return false
}

func (scratch *diagnosticParserCoreAcceptedLeafCoverageScratch) authenticateDirectTerminalAliases(
	parser *Parser,
	entries []stackEntry,
	children []*Node,
	productionID uint16,
	aliasSymbol Symbol,
	nodesByID []*Node,
) {
	if scratch == nil || parser == nil || parser.language == nil || aliasSymbol == 0 {
		return
	}
	aliases := parser.reduceAliasSequence(productionID)
	if len(aliases) == 0 {
		return
	}
	structuralChild := 0
	for _, entry := range entries {
		raw := stackEntryNode(entry)
		if raw == nil || raw.isExtra() {
			continue
		}
		alias := Symbol(0)
		if structuralChild < len(aliases) {
			alias = aliases[structuralChild]
		}
		structuralChild++
		if alias != aliasSymbol || raw.symbol == alias ||
			uint32(raw.symbol) >= parser.language.TokenCount ||
			nodeChildCountNoMaterialize(raw) != 0 ||
			!scratch.hasVisibleTerminalNode(raw.startByte, raw.endByte, raw, nodesByID) {
			continue
		}
		var match *Node
		for _, child := range children {
			if child == nil || child == raw || child.symbol != alias ||
				child.startByte != raw.startByte || child.endByte != raw.endByte ||
				nodeChildCountNoMaterialize(child) != 0 {
				continue
			}
			if match != nil {
				match = nil
				break
			}
			match = child
		}
		if match != nil {
			scratch.authenticatedAliases = append(scratch.authenticatedAliases, match)
		}
	}
}

// nodeSlice returns a zeroed length-n *Node slice, reusing buf's capacity when
// it fits. Clearing every entry prevents a stale node pointer from an earlier
// parse leaking into this one.
func parserCoreNodeSlice(buf []*Node, n int) []*Node {
	if n <= 0 {
		return buf[:0]
	}
	if cap(buf) < n {
		return make([]*Node, n)
	}
	buf = buf[:n]
	clear(buf)
	return buf
}

// clearNodeScratch clears a *Node scratch buffer to its full capacity and
// resets its length, dropping it when it grew past the retention cap. Clearing
// the full capacity (not [:len]) matters because tree-wiring leaves live node
// pointers in the backing array beyond len.
func clearNodeScratch(buf []*Node) []*Node {
	if cap(buf) > parserCoreMaxRetainedNodeScratch {
		return nil
	}
	if cap(buf) > 0 {
		clear(buf[:cap(buf)])
		return buf[:0]
	}
	return buf
}

// resetTreeBuffers clears the tree-materialization buffers after a parse so the
// runner never pins arena node pointers between parses. The line-start buffer
// holds no pointers, so it only resets its length (or drops past the cap). The
// Go-compatibility frame buffer holds node pointers, so it clears the full
// capacity before resetting the length, dropping it past its retention cap.
func (s *parserCoreRunnerScratch) resetTreeBuffers() {
	if s == nil {
		return
	}
	s.postorder.Reset()
	s.acceptedLeaves.reset()
	s.recoveryTerminalAliasSymbol = 0
	s.recoveryTerminalAliasCertified = false
	s.nodesByID = clearNodeScratch(s.nodesByID)
	if cap(s.hasErrorByID) > parserCoreMaxRetainedNodeScratch {
		s.hasErrorByID = nil
	} else {
		clear(s.hasErrorByID)
		s.hasErrorByID = s.hasErrorByID[:0]
	}
	s.nodes = clearNodeScratch(s.nodes)
	s.linkScratch = clearNodeScratch(s.linkScratch)
	if cap(s.lineStarts) > parserCoreMaxRetainedLineStarts {
		s.lineStarts = nil
	} else {
		s.lineStarts = s.lineStarts[:0]
	}
	if cap(s.goCompatFrames) > maxRetainedGoCompatFrames {
		s.goCompatFrames = nil
	} else if cap(s.goCompatFrames) > 0 {
		clear(s.goCompatFrames[:cap(s.goCompatFrames)])
		s.goCompatFrames = s.goCompatFrames[:0]
	}
}

func newDiagnosticParserCorePointIndex(source []byte, poll func() error) (diagnosticParserCorePointIndex, error) {
	return newDiagnosticParserCorePointIndexInto(source, poll, nil)
}

// newDiagnosticParserCorePointIndexInto builds the source point index, reusing
// buf as the line-start backing storage when it has capacity. Reusing the
// buffer keeps the warm steady state from allocating a fresh line-start slice
// on every parse.
func newDiagnosticParserCorePointIndexInto(source []byte, poll func() error, buf []uint32) (diagnosticParserCorePointIndex, error) {
	if uint64(len(source)) > math.MaxUint32 {
		return diagnosticParserCorePointIndex{}, errors.New("parser-core phase zero: materialization source exceeds uint32 offsets")
	}
	var starts []uint32
	if cap(buf) >= 1 {
		starts = buf[:1]
		starts[0] = 0
	} else {
		starts = make([]uint32, 1, min(1024, 1+len(source)/32))
	}
	for index, b := range source {
		if index&1023 == 0 {
			if err := poll(); err != nil {
				return diagnosticParserCorePointIndex{}, err
			}
		}
		if b == '\n' {
			starts = append(starts, uint32(index+1))
		}
	}
	if err := poll(); err != nil {
		return diagnosticParserCorePointIndex{}, err
	}
	return diagnosticParserCorePointIndex{lineStarts: starts}, nil
}

func (index *diagnosticParserCorePointIndex) point(offset uint32) Point {
	point, _ := index.pointCached(offset)
	return point
}

// pointCached returns exact source coordinates and whether the exact offset
// was already present in the materialization-local direct-mapped cache. The
// multiplicative slot keeps nearby byte boundaries from systematically
// colliding without adding a map, source-sized table, or shared state.
func (index *diagnosticParserCorePointIndex) pointCached(offset uint32) (Point, bool) {
	slot := uint32(offset*0x9e3779b1) >> 28
	mask := uint16(1) << slot
	entry := index.cache[slot]
	if index.valid&mask != 0 && entry.offset == offset {
		return entry.point, true
	}
	point := index.pointUncached(offset)
	index.cache[slot] = diagnosticParserCorePointCacheEntry{offset: offset, point: point}
	index.valid |= mask
	return point, false
}

func (index *diagnosticParserCorePointIndex) pointUncached(offset uint32) Point {
	line := sort.Search(len(index.lineStarts), func(i int) bool { return index.lineStarts[i] > offset }) - 1
	if line < 0 {
		return Point{Column: offset}
	}
	return Point{Row: uint32(line), Column: offset - index.lineStarts[line]}
}

func materializeDiagnosticParserCoreAcceptedTree(compact *core.Core, head core.Head, parser *Parser, source []byte, scratch *parserCoreRunnerScratch, forceReplayParseStates bool, allowErrorRoot bool) (*Tree, error) {
	if compact == nil || parser == nil || parser.language == nil || head.Node == 0 {
		return nil, errors.New("parser-core phase zero: incomplete accepted-tree materialization input")
	}
	derivations, err := compactDerivationsForAcceptance(compact, head)
	if err != nil {
		return nil, err
	}
	if len(derivations) != 1 {
		return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreAccept, detail: "materialization requires one exact accepted derivation"}
	}
	return materializeDiagnosticParserCoreAcceptedSelection(compact, head, derivations[0].Payloads, parser, source, scratch, forceReplayParseStates, allowErrorRoot)
}

// finalizeDiagnosticParserCoreAcceptedRootSpan requires a complete, clean
// root span by default: compact has no error recovery outside the B3 stage
// S3 certified shape, so an error/incomplete root anywhere else is a defect,
// not a legitimate result. allowErrorRoot, true only when this parse ran
// under an admitted native S3 recovery region (design section 4;
// s3ErrorRegionAdmitted's exact gate, threaded down from the caller), lifts
// the !root.IsError() && !root.HasError() bar so a genuinely recovered tree
// can complete -- the span-completeness half of the check (root must still
// cover the whole source) stays in force unconditionally either way.
func finalizeDiagnosticParserCoreAcceptedRootSpan(root *Node, source []byte, sourceLen uint32, allowErrorRoot bool, continuationEscape byte, poll func() error, tokenCount uint32, coverage *diagnosticParserCoreAcceptedLeafCoverageScratch, nodesByID []*Node) error {
	expectedStart := firstNonTriviaByteStart(source)
	clean := allowErrorRoot || (!root.IsError() && !root.HasError())
	if root.startByte == expectedStart && root.endByte < sourceLen && clean {
		extendRootToAcceptedCleanTail(root, source, sourceLen, nil, continuationEscape)
	}
	if root.startByte == expectedStart && root.endByte == sourceLen && clean {
		if allowErrorRoot {
			// B3 stage S3 fail-closed audit (adversarial review finding,
			// html "<!--c-->>"): allowErrorRoot lets a root whose OWN
			// HasError reads false past the ordinary !IsError()&&!HasError()
			// bar, because a native recovery region can legitimately accept
			// with an unlinked ERROR payload sitting beside the structural
			// root reduce rather than under it. That same relaxation also
			// let a genuinely hollow accept through once: the accepted
			// payload set held [comment(extra), ERROR(extra),
			// document(span 9..9, 0 children)] -- production's own
			// ts_parser__accept splices sibling extra payloads into the
			// root; this materializer's root-payload-only path does not, so
			// "document" reduced over zero real content, its 0 (raw and
			// public) children trivially satisfied
			// diagnosticParserCoreReduceChildrenTilingGap's per-reduce check
			// (isDerivationRootReduce exempts the root from that check
			// besides), and extendRootToAcceptedCleanTail above then
			// stretched the empty span to the full source with nothing
			// re-checking that the stretch was justified by covered
			// content: document[0:9], 0 children, HasError()==false --
			// total byte loss, silently reported clean. Independent of
			// which reduce claims it, or whether that reduce is exempt from
			// the per-reduce check, the FINAL PUBLIC TREE'S leaves must
			// still tile the accepted span; this is that closing check, and
			// it runs only in the one case that needs it (allowErrorRoot),
			// so every other language and every non-recovery compact parse
			// pays nothing.
			if err := poll(); err != nil {
				return err
			}
			if gapStart, gapEnd, gapped, err := diagnosticParserCoreAcceptedDerivationLeafCoverageGap(coverage, source, expectedStart, sourceLen, poll); err != nil {
				return fmt.Errorf("parser-core phase zero: accepted compact derivation leaf coverage: %w", err)
			} else if gapped {
				return fmt.Errorf(
					"parser-core phase zero: accepted compact root leaves do not tile the accepted span: gap=%d..%d root=%d..%d",
					gapStart, gapEnd, root.startByte, root.endByte,
				)
			}
			if gapStart, gapEnd, gapped, err := diagnosticParserCoreAcceptedTreeLeafCoverageGap(root, source, expectedStart, sourceLen, tokenCount, coverage, nodesByID, poll); err != nil {
				return fmt.Errorf("parser-core phase zero: accepted compact public-tree leaf coverage: %w", err)
			} else if gapped {
				return fmt.Errorf(
					"parser-core phase zero: accepted compact root leaves do not tile the accepted span: gap=%d..%d root=%d..%d",
					gapStart, gapEnd, root.startByte, root.endByte,
				)
			}
			// Round-2 adversarial review, accept-time splice gap: the leaf-
			// coverage audit above closed the BYTE-LOSS symptom of a related
			// gap; it cannot see this one, since every byte is still
			// covered -- only the ATTACHMENT point is wrong. See
			// diagnosticParserCoreAcceptedRootTrailingErrorExtraGap's own
			// doc comment.
			if err := poll(); err != nil {
				return err
			}
			if diagnosticParserCoreAcceptedRootTrailingErrorExtraGap(root) {
				return fmt.Errorf(
					"parser-core phase zero: accepted compact root carries an error-bearing trailing extra payload after its last non-extra child: root=%d..%d",
					root.startByte, root.endByte,
				)
			}
		}
		return nil
	}
	return fmt.Errorf(
		"parser-core phase zero: accepted compact root is incomplete or erroneous: span=%d..%d expected=%d..%d error=%t allowErrorRoot=%t",
		root.startByte,
		root.endByte,
		expectedStart,
		sourceLen,
		root.HasError(),
		allowErrorRoot,
	)
}

// diagnosticParserCoreAcceptedDerivationLeafCoverageGap checks the raw
// terminal spans collected during authenticated materialization. Hidden
// terminals remain available, while childless non-terminals contribute none.
func diagnosticParserCoreAcceptedDerivationLeafCoverageGap(coverage *diagnosticParserCoreAcceptedLeafCoverageScratch, source []byte, expectedStart, sourceLen uint32, poll func() error) (gapStart, gapEnd uint32, gapped bool, err error) {
	if coverage == nil {
		return 0, 0, false, errors.New("accepted derivation leaf coverage requires terminal spans")
	}
	if poll != nil {
		if err := poll(); err != nil {
			return 0, 0, false, err
		}
	}
	cur := expectedStart
	for index, span := range coverage.spans {
		if index&255 == 0 && poll != nil {
			if err := poll(); err != nil {
				return 0, 0, false, err
			}
		}
		if span.startByte > span.endByte || span.endByte > sourceLen {
			return 0, 0, false, fmt.Errorf("terminal subtree span=%d..%d is outside source length %d", span.startByte, span.endByte, sourceLen)
		}
		if span.startByte > cur {
			tolerated, gapErr := diagnosticParserCoreGapIsToleratedWithPoll(source[cur:span.startByte], poll)
			if gapErr != nil {
				return 0, 0, false, gapErr
			}
			if !tolerated {
				return cur, span.startByte, true, nil
			}
		}
		if span.endByte > cur {
			cur = span.endByte
		}
	}
	if cur < sourceLen {
		tolerated, gapErr := diagnosticParserCoreGapIsToleratedWithPoll(source[cur:sourceLen], poll)
		if gapErr != nil {
			return 0, 0, false, gapErr
		}
		if !tolerated {
			return cur, sourceLen, true, nil
		}
	}
	return 0, 0, false, nil
}

func diagnosticParserCoreAcceptedHiddenLeafCovers(coverage *diagnosticParserCoreAcceptedLeafCoverageScratch, source []byte, startByte, endByte uint32, nextSpan *int, poll func() error) (covered bool, err error) {
	if coverage == nil || startByte >= endByte || nextSpan == nil {
		return false, nil
	}
	if endByte > uint32(len(source)) {
		return false, fmt.Errorf("public leaf gap=%d..%d is outside source length %d", startByte, endByte, len(source))
	}
	for *nextSpan < len(coverage.spans) && coverage.spans[*nextSpan].endByte <= startByte {
		(*nextSpan)++
	}
	cur := startByte
	for index := *nextSpan; index < len(coverage.spans); index++ {
		if index&255 == 0 && poll != nil {
			if err := poll(); err != nil {
				return false, err
			}
		}
		span := coverage.spans[index]
		if span.startByte >= endByte {
			break
		}
		if !span.hidden {
			if span.endByte > cur {
				return false, nil
			}
			*nextSpan = index + 1
			continue
		}
		if span.startByte > cur {
			tolerated, gapErr := diagnosticParserCoreGapIsToleratedWithPoll(source[cur:min(span.startByte, endByte)], poll)
			if gapErr != nil {
				return false, gapErr
			}
			if !tolerated {
				return false, nil
			}
			cur = span.startByte
		}
		if span.endByte > cur {
			cur = min(span.endByte, endByte)
		}
		*nextSpan = index + 1
		if cur >= endByte {
			return true, nil
		}
	}
	if cur < endByte {
		tolerated, gapErr := diagnosticParserCoreGapIsToleratedWithPoll(source[cur:endByte], poll)
		if gapErr != nil {
			return false, gapErr
		}
		if tolerated {
			return true, nil
		}
	}
	return false, nil
}

func diagnosticParserCoreAcceptedTreeLeafCoverageGap(root *Node, source []byte, expectedStart, sourceLen, tokenCount uint32, coverage *diagnosticParserCoreAcceptedLeafCoverageScratch, nodesByID []*Node, poll func() error) (gapStart, gapEnd uint32, gapped bool, err error) {
	if coverage == nil {
		return 0, 0, false, errors.New("public leaf coverage requires terminal spans")
	}
	if poll != nil {
		if err := poll(); err != nil {
			return 0, 0, false, err
		}
	}
	cur := expectedStart
	nextHiddenSpan := 0
	acceptGap := func(startByte, endByte uint32) (bool, error) {
		if endByte <= startByte {
			return true, nil
		}
		if startByte > endByte || endByte > sourceLen {
			return false, fmt.Errorf("public leaf gap=%d..%d is outside source length %d", startByte, endByte, sourceLen)
		}
		tolerated, gapErr := diagnosticParserCoreGapIsToleratedWithPoll(source[startByte:endByte], poll)
		if gapErr != nil {
			return false, gapErr
		}
		if tolerated {
			cur = endByte
			return true, nil
		}
		covered, hiddenErr := diagnosticParserCoreAcceptedHiddenLeafCovers(coverage, source, startByte, endByte, &nextHiddenSpan, poll)
		if hiddenErr != nil {
			return false, hiddenErr
		}
		if covered {
			cur = endByte
			return true, nil
		}
		gapStart, gapEnd, gapped = startByte, endByte, true
		return false, nil
	}
	var walk func(n *Node) error
	var visited uint32
	walk = func(n *Node) error {
		if n == nil || gapped {
			return nil
		}
		visited++
		if visited&255 == 0 && poll != nil {
			if err := poll(); err != nil {
				return err
			}
		}
		count := n.ChildCount()
		if count == 0 {
			sym := n.Symbol()
			// A terminal alias publishes a cloned grammar nonterminal leaf.
			// Accept only a clone authenticated at its exact reduce site.
			if uint32(sym) >= tokenCount &&
				!coverage.hasVisibleTerminalNode(n.startByte, n.endByte, n, nodesByID) &&
				!coverage.hasAuthenticatedAlias(n) {
				return nil
			}
			if n.startByte > cur {
				if ok, gapErr := acceptGap(cur, n.startByte); gapErr != nil || !ok {
					return gapErr
				}
			}
			if n.endByte > cur {
				cur = n.endByte
			}
			return nil
		}
		for index := 0; index < count; index++ {
			if err := walk(n.Child(index)); err != nil {
				return err
			}
			if gapped {
				return nil
			}
		}
		return nil
	}
	if err = walk(root); err != nil || gapped {
		return gapStart, gapEnd, gapped, err
	}
	if cur < sourceLen {
		if ok, gapErr := acceptGap(cur, sourceLen); gapErr != nil || !ok {
			return gapStart, gapEnd, gapped, gapErr
		}
	}
	return 0, 0, false, nil
}

// diagnosticParserCoreAcceptedRootTrailingErrorExtraGap reports whether
// root's direct children end with an error-bearing extra payload trailing
// the last non-extra child -- the accept-time splice gap (adversarial
// review round 2, html "<html><body>x</body>\x00>"). C's ts_parser__accept
// rebuilds the last non-extra tree over the remaining stack contents,
// INCLUDING trailing extras, at the moment of accepting; this splices a
// trailing extra into whatever real content precedes it, extending that
// content's own span and propagating its own HasError up through ordinary
// ancestor propagation. This materializer's S3 accept path does not
// perform that splice: an ERROR region that resumes onto a head already
// past the last structural reduce (s3CloseInProgressProductions's own
// eager closure -- correct and necessary on its own terms -- can land
// there) ends up attached as the ROOT's own sibling instead, one level too
// shallow. Observed: document[0:22], 2 children (element[0:20]
// HasError=false, ERROR[20:22] extra) where the C oracle reports 1 child
// (element[0:22] HasError=true, the same byte-identical ERROR nested
// inside it) -- tokenization and the ERROR container itself are identical;
// only the attachment point differs, and it flips the enclosing element's
// own span and HasError, which callers read.
//
// This audit runs only under allowErrorRoot (S3-admitted parses), same as
// the leaf-coverage gap above, and closes a different symptom of a related
// class: that audit requires every byte to be COVERED by some leaf, which
// this shape already satisfies (nothing is lost, just misattached), so it
// cannot see this gap on its own.
//
// Deliberately narrower than "any trailing extra": an ordinary trailing
// comment or whitespace extra (IsError()==false, HasError()==false)
// legitimately sits beside a root's non-extra child in BOTH engines --
// confirmed necessary: html "<a></a><!--trailing-->" produces
// document[0:22], 2 children (element, comment), byte-identical between
// compact and production, and must not decline. Only an error-bearing
// trailing extra (the shape C would have spliced into the preceding
// content instead of leaving as the root's own sibling) trips this gap.
func diagnosticParserCoreAcceptedRootTrailingErrorExtraGap(root *Node) bool {
	count := root.ChildCount()
	if count < 2 {
		return false
	}
	lastNonExtra := -1
	for i := 0; i < count; i++ {
		if !root.Child(i).IsExtra() {
			lastNonExtra = i
		}
	}
	if lastNonExtra < 0 {
		return false
	}
	for i := lastNonExtra + 1; i < count; i++ {
		child := root.Child(i)
		if child.IsExtra() && (child.IsError() || child.HasError()) {
			return true
		}
	}
	return false
}

// diagnosticParserCoreReduceChildrenTilingGap is the B1 route-equality
// invariant (campaign v7, tranche B1): a compact subtree's declared span
// [startByte, endByte) is sound only if its own reduce children -- the RAW
// list the compact core popped for this subtree, before hidden-node elision
// -- actually tile it: contiguous coverage, no unaccounted-for non-trivia
// byte range. Called once per non-terminal from materializeVisit (below),
// on entries built from view.Children, immediately after they are resolved
// via nodesByID and before any hidden-node filtering
// (parser.buildReduceChildrenWithPath) or unary collapse runs.
//
// Root cause this closes: the scheduler's acceptance gate
// (finalizeDiagnosticParserCoreAcceptedRootSpan) checked only the root's own
// span and error flag, never that the accepted derivation's content actually
// justified that span. A derivation whose reduce silently skipped real input
// could still be accepted and materialized: the reduce's own view.StartByte/
// EndByte (set independently of any child -- see the view construction in
// materializeVisit) can claim a span wider than any entry actually covers,
// publishing HasError()==false while production and the locked C oracle
// both return an error tree for the same input. Reference witness: html
// `<a></a^>` -- the erroneous end-tag subtree claimed span 3..8 ("</a^>")
// while its three RAW children covered only 3..5, 5..6 and 7..8, leaving
// byte 6 (the stray '^') completely unaccounted for at every level, not
// just the final public tree (cgo_harness/testdata/
// compact_t3_oracle_witnesses_v2.json, witness "html_min_a"; verified by
// direct inspection of view.Children, not merely the post-materialization
// node tree -- see the next paragraph for why that distinction matters).
//
// Why RAW children, not the final public Node.children: an earlier version
// of this gate walked the finalized public tree after materialization and
// false-positived on legitimate, production-matching shapes -- the
// certified EOF-accept/primary-accept acceptance frontiers (http, robot,
// meson; grammars/runtime_profiles.go). Direct inspection proved why: a
// hidden grammar symbol (symbolMeta[...].Visible == false) can cover real
// source bytes -- Robot Framework's `keyword` node covers a leading byte its
// only NAMED child excludes; meson's `string` node covers its quoted
// content with no child at all -- and parser.buildReduceChildrenWithPath
// (shared by production and compact) elides that hidden entry from the
// node's final public children (occasionally repositioning a following
// ANONYMOUS sibling's start via flattenedHiddenEntryPadding, but never a
// named one), while the node's own declared span, set once at reduce time,
// still legitimately includes it. Production exhibits the identical
// "parent span wider than public children" shape for these inputs (verified
// by direct comparison), so it is not a defect -- but by the same token it
// is structurally indistinguishable from the html/js defect at the public-
// tree level: both are "parent span wider than the children the public API
// exposes". The only place the two are distinguishable is before hidden
// filtering, using the RAW popped children: robot's `keyword` and meson's
// `string` fully tile their own span once their hidden child is counted
// (verified directly: view.Children for robot's `keyword` [26,29) is two
// entries, [26,27) hidden + [27,29) name_chunk; meson's `string` [8,15) is
// three, [8,9) quote + [9,14) hidden content + [14,15) quote); html's
// erroneous-end-tag subtree does not -- its RAW view.Children already omit
// byte 6, so no filtering step is discarding evidence there at all. This
// function therefore runs on entries (RAW, pre-filter) rather than on any
// node's already-finalized public children.
//
// This is the identical defect class the GLR forest already guards against
// at reduce time: its own coverage rejection (glr_forest.go, "Coverage
// rejection: a reduction whose children leave a NON-TRIVIA hole skipped
// real input and is INVALID") scans a reduction's RAW children left to
// right -- before the same later hidden-filtering step -- and rejects any
// grouping with a real (non-trivia) gap, using bytesAreInterTokenTrivia to
// tell a dropped token from ordinary inter-token whitespace. This function
// is that same, already-proven definition and predicate, applied at the
// compact route's equivalent point in its pipeline (materialization-time
// reduce, since the compact scheduler has no forest reduce step of its
// own), checked uniformly at the leading edge (startByte to the first
// entry), between every pair of entries, and at the trailing edge (the last
// entry to endByte) -- one running coverage frontier, so no special case is
// needed for any position. Checking every non-terminal (not just the root)
// composes soundly by induction: if every node's own direct (raw) children
// tile that node's span modulo trivia, then transitively so do the leaves
// of the whole accepted derivation against the root's span, which is the
// property the tranche asks for.
//
// Deliberately out of scope here: the relationship between the root's own
// declared span and sourceLen (extendRootToAcceptedCleanTail, a few lines
// above, using the wider bytesAreParserPadding predicate for trailing
// padding beyond every reduce). That check runs later, once, only at the
// root, against a different boundary (the whole source, not a reduce's own
// declared span) and is unaffected by this one.
//
// The common path is O(children) and allocation-free. A certified non-trivia
// gap adds one indexed lookup for the aligned raw child. The lookup admits only
// an exact prefix whose start equals the prior coverage frontier and whose
// terminal lineage starts at the next child boundary.
func diagnosticParserCoreReduceChildrenTilingGap(startByte, endByte uint32, entries []stackEntry, source []byte) (gapStart, gapEnd uint32, gapped bool) {
	return diagnosticParserCoreReduceChildrenTilingGapWithLexerProvenance(
		startByte, endByte, entries, nil, source, nil, nil, false,
	)
}

func diagnosticParserCoreReduceChildrenTilingGapWithLexerProvenance(
	startByte, endByte uint32,
	entries []stackEntry,
	childIDs []core.SubtreeID,
	source []byte,
	coverage *diagnosticParserCoreAcceptedLeafCoverageScratch,
	nodesByID []*Node,
	allowLexerSkippedPrefix bool,
) (gapStart, gapEnd uint32, gapped bool) {
	lastEnd := startByte
	for index, entry := range entries {
		child := stackEntryNode(entry)
		if child == nil {
			continue
		}
		if child.startByte > lastEnd && !diagnosticParserCoreGapIsTolerated(source[lastEnd:child.startByte]) {
			var childID core.SubtreeID
			if index < len(childIDs) {
				childID = childIDs[index]
			}
			if !allowLexerSkippedPrefix || !coverage.childHasLexerSkippedPrefix(childID, lastEnd, child.startByte, nodesByID) {
				return lastEnd, child.startByte, true
			}
		}
		if child.endByte > lastEnd {
			lastEnd = child.endByte
		}
	}
	if lastEnd < endByte && !diagnosticParserCoreGapIsTolerated(source[lastEnd:endByte]) {
		return lastEnd, endByte, true
	}
	return 0, 0, false
}

// diagnosticParserCoreGapIsTolerated reports whether an apparent coverage
// gap is not, in fact, a real one: ordinary inter-token trivia
// (bytesAreInterTokenTrivia, matching the forest's own reduce-time coverage
// rejection).
//
// RETIRED (campaign v7 class-e closure, spore.2026-08-02.alder-e.js-false-
// clean): this gate used to also tolerate a single decoration byte strictly
// enclosed by trivia on both sides (bytesAreSingleByteDecorationTrivia),
// added to excuse a doxygen/jsdoc comment-continuation marker ("* ") this B1
// post-hoc auditor could not otherwise place. That predicate's own doctrine
// comment claimed the shape was not a plausible one for a real scheduler
// drop to produce; a stray byte injected between two spaces produces that
// exact shape. The measured false-clean sweep found 189 occurrences of it
// across javascript, haskell, html, and bash. Direct measurement (disabling
// only this predicate, no other change) closes the class: 0 residual
// divergences in a 624-input javascript-family probe, and only the
// pre-existing, separately tracked 25-input haskell residual (a different
// mechanism; compact may be correct there) surviving the 5,148-input probe
// restricted to the 9 curated languages (routeEqualityFuzzLanguages,
// fuzz_admission_route_equality_test.go). A wider, 45-language review sweep
// (11,884 inputs, unchanged before and after this predicate's retirement)
// separately observed two more residual members outside that curated set,
// hcl (12 instances) and doxygen (22 instances) -- both a different
// mechanism from this class-e closure, neither touched by this predicate's
// removal, and neither in scope here; they need their own adjudication,
// same as the haskell residual. Retiring this predicate first moved JSDoc from
// PASS to FALLBACK. The later exact-artifact route restores that case through
// lexer skipped-prefix provenance in the wrapper above. This source-byte
// predicate remains retired. Doxygen does not regress: its gap sits at the
// derivation root, which this predicate does not check.
//
// A companion dispatch-time byte-continuity guard in dispatchPassActive
// (declining before any shift crosses an unexplained gap, one site upstream
// of every compact shift call) was built and measured as a candidate
// replacement. It over-declines: production's own tolerance for this exact
// doxygen shape is decided only after full tree construction, by the
// language-general "result compatibility" normalization layer
// (finalizeResultRoot / normalizeResultCompatibility, parser_result_root_
// build.go and parser_result_compat.go), which drops the interior error
// production's own single-stack GLR run really does create for doxygen's
// smoke sample (verified with GLR tracing: production shifts the gap,
// tryMaterializeSkippedRealGap pushes a real, hasError=true ERROR node, the
// root reduce includes it as a raw child with hasError=true, and only the
// later result-compatibility pass removes it and clears the flag). No
// three-exemption mirror at the compact scheduler's dispatch point --
// necessarily earlier than materialization, let alone this later
// normalization pass -- can see that decision. The guard was reverted for
// this reason; this predicate retirement alone is the shipped fix. See
// spore.2026-08-02.hornbeam-e.byte-continuity for the full account.
func diagnosticParserCoreGapIsTolerated(gap []byte) bool {
	return bytesAreInterTokenTrivia(gap)
}

func diagnosticParserCoreGapIsToleratedWithPoll(gap []byte, poll func() error) (bool, error) {
	for index := 0; index < len(gap); index++ {
		if index&255 == 0 && poll != nil {
			if err := poll(); err != nil {
				return false, err
			}
		}
		switch gap[index] {
		case ' ', '\t', '\n', '\r', '\f':
			continue
		case '\\':
			if index+1 < len(gap) && (gap[index+1] == '\n' || gap[index+1] == '\r') {
				continue
			}
			return false, nil
		default:
			return false, nil
		}
	}
	return true, nil
}

// materializeCompactExternalScannerCheckpoint copies one terminal's core-owned
// scanner snapshots into the public arena. Core checkpoint IDs are not valid
// after materialization, so retain only byte copies in the node sidecar.
// It returns false unless the complete scanner provenance pair is attached.
func materializeCompactExternalScannerCheckpoint(compact *core.Core, arena *nodeArena, node *Node, view core.MaterializationSubtreeView) bool {
	if compact == nil || arena == nil || node == nil || node.ownerArena != arena || !view.ExternalScannerCheckpointExact ||
		view.ExternalScannerCheckpointStart == 0 || view.ExternalScannerCheckpointEnd == 0 {
		return false
	}
	start, startOK := compact.CopyCheckpointBytes(view.ExternalScannerCheckpointStart, nil)
	end, endOK := compact.CopyCheckpointBytes(view.ExternalScannerCheckpointEnd, nil)
	if !startOK || !endOK {
		return false
	}
	checkpoint := arena.recordExternalScannerCompactCheckpoint(start, end)
	if !externalScannerCheckpointRefComplete(checkpoint) {
		return false
	}
	if arena.setExternalScannerCheckpoint(node, checkpoint) {
		arena.externalScannerCheckpointLeafNodes++
		arena.externalScannerCheckpointRecords++
		return true
	}
	return false
}

// materializeDiagnosticParserCoreAcceptedSelection materializes the accepted
// compact derivation into a public tree. When scratch is non-nil the runner's
// reusable buffers back the transient materialization storage, so the warm
// steady state does not re-allocate the public-tree scratch on every parse.
// scratch is reset on return, so it is safe to reuse for the next parse.
func materializeDiagnosticParserCoreAcceptedSelection(compact *core.Core, head core.Head, payloads []core.SubtreeID, parser *Parser, source []byte, scratch *parserCoreRunnerScratch, forceReplayParseStates bool, allowErrorRoot bool) (*Tree, error) {
	return materializeDiagnosticParserCoreAcceptedSelectionWithRootFinalization(
		compact, head, payloads, parser, source, scratch,
		forceReplayParseStates, allowErrorRoot, diagnosticParserCoreFinalizeDefault,
	)
}

type diagnosticParserCoreRootFinalization uint8

const (
	diagnosticParserCoreFinalizeDefault diagnosticParserCoreRootFinalization = iota
	diagnosticParserCoreFinalizeRecoverEOF
)

// materializeDiagnosticParserCoreAcceptedSelectionWithRootFinalization keeps
// the default path unchanged. A tagged diagnostic caller can request the
// locked-C recover_eof root rule.
func materializeDiagnosticParserCoreAcceptedSelectionWithRootFinalization(compact *core.Core, head core.Head, payloads []core.SubtreeID, parser *Parser, source []byte, scratch *parserCoreRunnerScratch, forceReplayParseStates bool, allowErrorRoot bool, rootFinalization diagnosticParserCoreRootFinalization) (*Tree, error) {
	if compact == nil || parser == nil || parser.language == nil || head.Node == 0 || len(payloads) == 0 {
		return nil, errors.New("parser-core phase zero: incomplete accepted-tree selection input")
	}
	if rootFinalization != diagnosticParserCoreFinalizeDefault && rootFinalization != diagnosticParserCoreFinalizeRecoverEOF {
		return nil, errors.New("parser-core phase zero: unknown accepted-root finalization")
	}
	if scratch != nil {
		defer scratch.resetTreeBuffers()
	}
	stats, err := compact.Stats(head)
	if err != nil {
		return nil, err
	}

	arena := acquireNodeArena(arenaClassFull)
	// Compact external-token provenance is transferred into this arena below.
	// Set the language identity before publishing the first checkpoint so the
	// incremental reuse gate can authenticate the copied snapshots.
	scannerProvenanceTransferProven := true
	if languageUsesExternalScannerCheckpoints(parser.language) {
		_, identityRequired, identityValid := externalScannerCheckpointIdentityStatus(parser.language)
		if identityRequired {
			scannerProvenanceTransferProven = identityValid && arena.setExternalScannerCheckpointIdentityForLanguage(parser.language)
		}
	}
	owned := true
	defer func() {
		if owned {
			arena.Release()
		}
	}()
	poll := func() error {
		reason := parser.resultMaterializationStopReason(arena)
		if !resultMaterializationShouldStop(reason) {
			return nil
		}
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreCap, detail: "accepted-tree materialization stopped: " + string(reason)}
	}
	var lineStartsBuf []uint32
	if scratch != nil {
		lineStartsBuf = scratch.lineStarts
	}
	points, err := newDiagnosticParserCorePointIndexInto(source, poll, lineStartsBuf)
	if err != nil {
		return nil, err
	}
	if scratch != nil {
		scratch.lineStarts = points.lineStarts
	}
	// The visitor proves unique ownership, so this is a transient child-build
	// table rather than a memoization or sharing mechanism: every populated
	// compact ID owns exactly one public node in this tree.
	nodesByIDLen := uint64(stats.Subtrees) + 1
	var nodesByID []*Node
	var hasErrorByID []bool
	if scratch != nil && nodesByIDLen <= uint64(math.MaxInt) {
		scratch.nodesByID = parserCoreNodeSlice(scratch.nodesByID, int(nodesByIDLen))
		nodesByID = scratch.nodesByID
		if cap(scratch.hasErrorByID) < int(nodesByIDLen) {
			scratch.hasErrorByID = make([]bool, int(nodesByIDLen))
		} else {
			scratch.hasErrorByID = scratch.hasErrorByID[:int(nodesByIDLen)]
			clear(scratch.hasErrorByID)
		}
		hasErrorByID = scratch.hasErrorByID
	} else {
		nodesByID = make([]*Node, nodesByIDLen)
		hasErrorByID = make([]bool, nodesByIDLen)
	}
	if err := poll(); err != nil {
		return nil, err
	}

	// Phase-3 Lane 2: reconstruct parser states by top-down table replay over
	// the full derivation (real symbols + hidden nodes), before the postorder
	// pass elides hidden nodes and applies aliases. Gated so it can be A/B'd
	// against the states-free compact route.
	var replayStates *compactReplayStates
	if forceReplayParseStates || parserCoreReplayParseStatesEnabled() {
		replayStates, err = parser.replayCompactDerivation(compact, payloads)
		if err != nil {
			return nil, err
		}
		defer replayStates.release()
	}
	incrementalReuseProven := replayStates != nil && compactIncrementalReuseProvenForLanguage(parser.language)
	stamp := func(id core.SubtreeID, node *Node, terminal bool) {
		// Stamp the reconstructed state for THIS derivation id onto the node
		// that materializes it. For a unary collapse chain the driver visits the
		// ids inner-to-outer (postorder) and reuses one node object, so the last
		// (outermost) stamp wins -- mirroring production's collapse, which
		// overwrites parseState = goto(topState, outerSymbol) as each wrapper
		// reduce fires.
		//
		// replayStates.get returns ok=false when the top-down replay could not
		// find a table transition for this id (an extra/comment leaf whose
		// floated stack position does not match a live shift, or any node whose
		// production shape is not a plain shift/goto of its visible symbol). In
		// that case the reconstructed state is NOT authoritative, so we ABSTAIN:
		// leave parseState/preGotoState at their zero value. Downstream, a zero
		// parseState is the "unknown -> recompute" sentinel (incremental
		// self-healing), which is strictly safer than stamping a known-wrong but
		// trusted non-zero state (Phase-3 Lane 3 review amendment 1).
		if replayStates != nil && node != nil {
			pre, ps, preOk, psOk := replayStates.get(id)
			if terminal {
				incrementalReuseProven = incrementalReuseProven && psOk
			} else {
				incrementalReuseProven = incrementalReuseProven && preOk
			}
			if psOk {
				node.parseState = ps
			}
			if preOk {
				node.preGotoState = pre
			}
		} else {
			incrementalReuseProven = false
		}
		nodesByID[id] = node
	}
	// markFragile threads the compact record's ambiguity bit (subtreeRecord
	// .fragile, exposed on MaterializationSubtreeView.Fragile) onto the public
	// node so Lane-1's isFragile() reuse gate sees compact-materialized trees
	// the same as production-built ones (Phase-3 Lane 3 review amendment 7). The
	// compact record collapses production's fragileLeft/fragileRight into one
	// conservative flag, so both edges are set. Set-only (never clears), matching
	// the record's monotone contract on shared/deduped records.
	markFragile := func(node *Node, fragile bool) {
		if node == nil || !fragile {
			return
		}
		node.setFragileLeft(true)
		node.setFragileRight(true)
	}
	var acceptedLeavesLocal diagnosticParserCoreAcceptedLeafCoverageScratch
	acceptedLeaves := &acceptedLeavesLocal
	if scratch != nil {
		scratch.acceptedLeaves.reset()
		acceptedLeaves = &scratch.acceptedLeaves
	}
	allowLexerSkippedPrefix := parser.language.CompactLexerSkippedPrefixTilingCertified
	if !allowErrorRoot && !allowLexerSkippedPrefix {
		acceptedLeaves = nil
	} else if allowLexerSkippedPrefix {
		acceptedLeaves.prepareLexerSkippedPrefixes(int(stats.Subtrees))
	}
	recoveryTerminalAlias := Symbol(0)
	if scratch != nil && scratch.recoveryTerminalAliasCertified {
		recoveryTerminalAlias = scratch.recoveryTerminalAliasSymbol
	}
	materializeVisit := func(materializationScratch *diagnosticParserCoreMaterializationScratch) error {
		visit := func(id core.SubtreeID, view core.MaterializationSubtreeView) error {
			if view.EndByte < view.StartByte || view.EndByte > uint32(len(source)) {
				return errors.New("parser-core phase zero: compact subtree extent is outside source")
			}
			if acceptedLeaves != nil && view.Terminal {
				if allowLexerSkippedPrefix {
					acceptedLeaves.recordLexerSkippedPrefix(id, view)
				}
				if allowErrorRoot {
					hidden := !parser.isVisibleSymbol(Symbol(view.Symbol))
					if view.Symbol == core.RecoveryErrorSymbol {
						hidden = false
					}
					if err := acceptedLeaves.append(id, view, uint32(len(source)), hidden); err != nil {
						return err
					}
				}
			}
			named := parser.isNamedSymbol(Symbol(view.Symbol))
			// B3 stage S3: the built-in ERROR symbol (65535) sits outside
			// every real grammar's SymbolMetadata table, so isNamedSymbol's
			// bounds check above always reads false for it. Tree-sitter
			// treats ERROR as named unconditionally (visible in
			// S-expressions and named-child traversal, matching the pinned
			// C oracle's own "(ERROR ...)"/"(ERROR (UNEXPECTED 'x'))"
			// rendering for both the container and a raw unlexable-byte
			// leaf) -- force it here rather than teach the shared,
			// grammar-table-driven isNamedSymbol about a symbol that is
			// never a real grammar table entry.
			if Symbol(view.Symbol) == errorSymbol {
				named = true
			}
			if view.Terminal {
				node := newLeafNodeInArena(
					arena, Symbol(view.Symbol), named, view.StartByte, view.EndByte,
					points.point(view.StartByte), points.point(view.EndByte),
				)
				if languageUsesExternalScannerCheckpoints(parser.language) && view.Terminal &&
					!materializeCompactExternalScannerCheckpoint(compact, arena, node, view) {
					scannerProvenanceTransferProven = false
				}
				node.setExtra(view.Extra)
				node.setExternalScannerToken(view.External)
				// S5 recovery: a recovery-inserted MISSING terminal
				// (core.MissingLeaf) carries both public bits, matching the
				// pinned C oracle and production's own port
				// (parser.go's missing-shift path sets exactly this pair).
				// has-error belongs on the missing node ITSELF, not only on
				// its ancestors: C defines ts_node_has_error as
				// error_cost > 0 (node.c:520-522), and ts_subtree_error_cost
				// short-circuits on the missing bit to return
				// ERROR_COST_PER_MISSING_TREE + ERROR_COST_PER_RECOVERY
				// (subtree.h:331-337), which is 610, so C reports has-error
				// true on the leaf. For a VISIBLE missing leaf, ordinary
				// ancestor propagation (populateParentNode, tree.go) then ORs
				// the flag up through every enclosing reduce with no
				// additional code.
				//
				// Hidden missing leaves can disappear during parent construction.
				// hasErrorByID carries their error state through that collapse. This
				// matches production's explicit trackChildErrors signal.
				if view.Missing {
					node.setMissing(true)
					node.setHasError(true)
				}
				hasErrorByID[id] = view.Missing || Symbol(view.Symbol) == errorSymbol
				// No markFragile here: fragile is a reduce/conflict-arm property
				// (subtreeRecord.fragile is only ever set on reductions), so a
				// terminal record is never fragile. The reduce branches below
				// carry the bit.
				stamp(id, node, true)
				return nil
			}

			entries := materializationScratch.entriesFor(len(view.Children))
			subtreeHasError := Symbol(view.Symbol) == errorSymbol
			structuralChildren := 0
			for index, childID := range view.Children {
				if uint64(childID) >= uint64(len(nodesByID)) || nodesByID[childID] == nil {
					return errors.New("parser-core phase zero: compact materialization traversal omitted a child")
				}
				child := nodesByID[childID]
				if hasErrorByID[childID] {
					subtreeHasError = true
				}
				entries[index] = newStackEntryNode(0, child)
				if !child.isExtra() {
					structuralChildren++
				}
			}
			// isDerivationRootReduce is true only for the one reduce, per parse,
			// whose symbol is this language's own inferred grammar root symbol
			// (parser.rootSymbol / hasRootSymbol -- inferRootSymbol, parser.go: a
			// grammar-derived property, computed from the language's own tables,
			// not a per-language name check). It is exempted from this reduce's
			// OWN tiling requirement for the same reason
			// finalizeDiagnosticParserCoreAcceptedRootSpan already treats the
			// root-to-sourceLen boundary as a separately governed special case
			// (extendRootToAcceptedCleanTail, with its own, more lenient rule): the
			// root reduce is the one construct with no enclosing reduce to ever
			// re-validate its own declared span from the outside, so an over-wide
			// root span (still exactly [expectedStart, sourceLen), already pinned
			// by finalizeDiagnosticParserCoreAcceptedRootSpan's own checks) is a
			// materially different, narrower risk than an internal gap anywhere
			// below it, which every enclosing reduce's own tiling check still
			// catches. This closes a jsdoc residual the retired
			// bytesAreSingleByteDecorationTrivia predicate used to leave standing:
			// javadoc/doxygen-style comments that close with "*/" (no leading
			// space) put the decoration marker
			// on the trailing edge of the root reduce's own gap, indistinguishable
			// in isolation from a genuine drop. The exemption stays limited to the
			// grammar root. Every internal reduce still passes the ordinary tiling
			// check before materialization can publish it.
			isDerivationRootReduce := rootFinalization == diagnosticParserCoreFinalizeDefault &&
				parser.hasRootSymbol && Symbol(view.Symbol) == parser.rootSymbol
			if allowLexerSkippedPrefix {
				acceptedLeaves.propagateLeadingLexerSkippedPrefix(id, view.StartByte, view.Children, nodesByID)
			}
			if gapStart, gapEnd, gapped := diagnosticParserCoreReduceChildrenTilingGapWithLexerProvenance(
				view.StartByte, view.EndByte, entries, view.Children, source, acceptedLeaves, nodesByID, allowLexerSkippedPrefix,
			); !isDerivationRootReduce && gapped {
				return &diagnosticParserCoreDecline{
					boundary: DiagnosticParserCoreAccept,
					detail: fmt.Sprintf(
						"accepted-leaf-tiling-gap: compact subtree symbol=%d span=%d..%d has an unaccounted byte range %d..%d not covered by any child",
						view.Symbol, view.StartByte, view.EndByte, gapStart, gapEnd,
					),
				}
			}
			// B3 stage S3: an ERROR-symbol reduce is a native recovery region
			// (s3TryOpenErrorRegion/ErrorRegionResume), never a real grammar
			// production. It always bypasses unary self-reduction collapse
			// (errorSymbol's huge numeric value falls outside every real
			// grammar's SymbolMetadata table, so the collapse checks below
			// would either safely no-op or -- for the one case they would
			// not, a childless absorbed leaf sharing the ERROR symbol itself
			// -- wrongly elide the wrapper the C oracle keeps; skip them
			// outright instead of relying on that bound check), matching
			// production's own recovery construction (newRecoveryParentNodeInArena,
			// parser_recover_c.go), which never goes through the shared
			// collapsibleRawUnarySelfReduction/collapsibleUnarySelfReduction
			// path either.
			if Symbol(view.Symbol) == errorSymbol {
				children, fieldIDs, fieldSources, _ := parser.buildReduceChildrenWithPath(
					entries, 0, len(entries), structuralChildren,
					Symbol(view.Symbol), view.ProductionID, arena,
				)
				if recoveryTerminalAlias != 0 {
					acceptedLeaves.authenticateDirectTerminalAliases(
						parser, entries, children, view.ProductionID, recoveryTerminalAlias, nodesByID,
					)
				}
				parent := newParentNodeInArenaWithFieldSources(
					arena, Symbol(view.Symbol), named, children, fieldIDs, fieldSources, view.ProductionID,
				)
				parent.dynamicPrecedence += int32(view.DynamicPrecedence)
				parent.startByte = view.StartByte
				parent.endByte = view.EndByte
				parent.startPoint = points.point(view.StartByte)
				parent.endPoint = points.point(view.EndByte)
				parent.setExtra(view.Extra)
				// The ERROR container's own HasError is always true,
				// regardless of what populateParentNode's children-OR
				// propagation computed: matching the pinned C oracle, an
				// absorbed leaf's own HasError stays false even when the
				// leaf is itself an unlexable byte (ErrorRegionLeaf's doc
				// comment; finding production-recovery-structural-divergence),
				// so this explicit set is the only place HasError=true
				// originates for the whole region. Every enclosing ordinary
				// reduce above this one propagates it up for free through
				// populateParentNode's existing, unmodified OR-of-children
				// walk (tree.go) -- no further HasError code is needed
				// anywhere else in this file.
				parent.setHasError(true)
				hasErrorByID[id] = true
				markFragile(parent, view.Fragile)
				stamp(id, parent, false)
				return nil
			}
			action := ParseAction{
				Type: ParseActionReduce, Symbol: Symbol(view.Symbol), ChildCount: uint8(structuralChildren),
				DynamicPrecedence: view.DynamicPrecedence, ProductionID: view.ProductionID,
			}
			if child := parser.collapsibleRawUnarySelfReduction(action, Token{}, arena, entries, 0, len(entries)); child != nil {
				child.productionID = view.ProductionID
				child.dynamicPrecedence += int32(view.DynamicPrecedence)
				markFragile(child, view.Fragile)
				if subtreeHasError {
					child.setHasError(true)
				}
				hasErrorByID[id] = subtreeHasError
				stamp(id, child, false)
				return nil
			}
			children, fieldIDs, fieldSources, _ := parser.buildReduceChildrenWithPath(
				entries, 0, len(entries), structuralChildren,
				Symbol(view.Symbol), view.ProductionID, arena,
			)
			if recoveryTerminalAlias != 0 {
				acceptedLeaves.authenticateDirectTerminalAliases(
					parser, entries, children, view.ProductionID, recoveryTerminalAlias, nodesByID,
				)
			}
			if child := parser.collapsibleUnarySelfReduction(action, Token{}, arena, entries, 0, len(entries), children, fieldIDs); child != nil {
				child.productionID = view.ProductionID
				child.dynamicPrecedence += int32(view.DynamicPrecedence)
				markFragile(child, view.Fragile)
				if subtreeHasError {
					child.setHasError(true)
				}
				hasErrorByID[id] = subtreeHasError
				stamp(id, child, false)
				return nil
			}
			parent := newParentNodeInArenaWithFieldSources(
				arena, Symbol(view.Symbol), named, children, fieldIDs, fieldSources, view.ProductionID,
			)
			parent.dynamicPrecedence += int32(view.DynamicPrecedence)
			parent.startByte = view.StartByte
			parent.endByte = view.EndByte
			parent.startPoint = points.point(view.StartByte)
			parent.endPoint = points.point(view.EndByte)
			parent.setExtra(view.Extra)
			if subtreeHasError {
				parent.setHasError(true)
			}
			hasErrorByID[id] = subtreeHasError
			markFragile(parent, view.Fragile)
			stamp(id, parent, false)
			return nil
		}
		if scratch != nil {
			return compact.VisitMaterializationPostorderWithScratch(payloads, poll, &scratch.postorder, visit)
		}
		return compact.VisitMaterializationPostorder(payloads, poll, visit)
	}
	if scratch != nil {
		err = withProvidedMaterializationScratch(parser, &scratch.materialization, materializeVisit)
	} else {
		err = withDiagnosticParserCoreMaterializationScratch(parser, materializeVisit)
	}
	if err != nil {
		return nil, err
	}

	var nodes []*Node
	if scratch != nil {
		scratch.nodes = parserCoreNodeSlice(scratch.nodes, len(payloads))
		nodes = scratch.nodes
	} else {
		nodes = make([]*Node, len(payloads))
	}
	for index, payload := range payloads {
		if uint64(payload) >= uint64(len(nodesByID)) || nodesByID[payload] == nil {
			return nil, errors.New("parser-core phase zero: compact materialization order omitted an accepted payload")
		}
		nodes[index] = nodesByID[payload]
	}
	// acceptedRootSpanStart is each top-level node's OWN declared startByte
	// (view.StartByte, stamped verbatim at materializeVisit and never mutated
	// since), captured before buildResultFromNodes runs. It must be read here:
	// buildResultFromNodes's finalizeResultRoot calls normalizeRootSourceStart
	// (parser_result_root_build.go), which unconditionally pulls a wider root
	// back to source's first non-trivia byte on the assumption of a
	// legitimately elided leading extra. That pull-back overwrites this exact
	// field, so any check against the post-build root.startByte is tautological
	// (it always reads back whatever normalizeRootSourceStart just wrote). See
	// the leading-gap decline below, which compares this pre-normalization
	// value instead.
	acceptedRootSpanStart := nodes[0].startByte
	acceptedRootSpanEnd := nodes[0].endByte
	for _, n := range nodes[1:] {
		if n.startByte < acceptedRootSpanStart {
			acceptedRootSpanStart = n.startByte
		}
		if n.endByte > acceptedRootSpanEnd {
			acceptedRootSpanEnd = n.endByte
		}
	}
	if err := poll(); err != nil {
		return nil, err
	}
	linkScratch := new([]*Node)
	if scratch != nil {
		linkScratch = &scratch.linkScratch
	}
	// buildResultFromNodes runs the returned-tree compatibility normalization
	// (including the Go-compatibility walk). Install the runner's reusable frame
	// buffer for that walk so the warm steady state reuses the walk stack instead
	// of re-growing it every parse, mirroring production's parser-held scratch.
	// resetTreeBuffers restores and clears it on return.
	previousGoCompatFrames := parser.goCompatFrames
	if scratch != nil {
		parser.goCompatFrames = &scratch.goCompatFrames
		defer func() { parser.goCompatFrames = previousGoCompatFrames }()
	}
	var tree *Tree
	if rootFinalization == diagnosticParserCoreFinalizeRecoverEOF {
		if len(nodes) != 1 || nodes[0] == nil || !nodes[0].IsError() {
			return nil, errors.New("parser-core phase zero: recover_eof finalization requires one ERROR payload")
		}
		// C publishes the ERROR parent pushed by recover_eof before accept.
		// Do not apply the generic Go single-error grammar-root framing rule or
		// any language result-compatibility rewrite.
		builder := newResultRootBuild(parser, source, arena, nil, nil, linkScratch)
		tree = builder.finishRecoverEOFTree(nodes[0], builder.shouldWireParentLinks)
	} else {
		tree = parser.buildResultFromNodes(nodes, source, arena, nil, nil, linkScratch)
	}
	if tree != nil {
		owned = false // The result tree owns the materialization arena.
	}
	rejectTree := func(err error) (*Tree, error) {
		if tree != nil {
			tree.Release()
		}
		return nil, err
	}
	if tree == nil || tree.root == nil {
		return rejectTree(errors.New("parser-core phase zero: accepted compact derivation materialized no root"))
	}
	// Use the raw (parser-owned) runtime record for this internal sanity check
	// instead of the public ParseRuntime() accessor. ParseRuntime() fires the
	// tree's one-shot deferred-compatibility finalizer (ensureResultCompatibility,
	// which runs for deferred-compat languages such as typescript/tsx/ini via
	// shouldDeferResultCompatibility). Firing it here would be premature: a few
	// lines below, setParseRuntime full-struct-replaces the runtime record, which
	// would permanently erase the normalization counters the finalizer just wrote
	// (NormalizationPasses and friends), since the finalizer only ever runs once.
	// rawParseRuntime does not normalize StopReason the way the public accessor
	// does, so compare with rawParseStopReason, which treats an empty raw
	// StopReason the same as ParseStopNone.
	rawRuntime := tree.rawParseRuntime()
	if reason := tree.rawParseStopReason(); reason != ParseStopNone || rawRuntime.Truncated || rawRuntime.TokenSourceEOFEarly {
		return rejectTree(fmt.Errorf("parser-core phase zero: accepted-tree materialization returned an incomplete runtime: %s", rawRuntime.Summary()))
	}
	if err := poll(); err != nil {
		return rejectTree(err)
	}
	sourceLen := uint32(len(source))
	root := tree.root
	if rootFinalization == diagnosticParserCoreFinalizeRecoverEOF {
		expectedStart := firstNonTriviaByteStart(source)
		tailClean := root.endByte <= sourceLen && parserTailAllowsCleanAcceptance(
			source, root.endByte, sourceLen, nil, parser.lineContinuationEscapeByte(),
		)
		if !allowErrorRoot || !root.IsError() || !root.HasError() ||
			root.startByte != expectedStart || root.startByte != acceptedRootSpanStart ||
			root.endByte != acceptedRootSpanEnd || !tailClean {
			return rejectTree(fmt.Errorf(
				"parser-core phase zero: recover_eof root is not exact: span=%d..%d expected=%d..%d source-tail-clean=%t error=%t has-error=%t",
				root.startByte, root.endByte, expectedStart, acceptedRootSpanEnd, tailClean, root.IsError(), root.HasError(),
			))
		}
	} else {
		if err := finalizeDiagnosticParserCoreAcceptedRootSpan(root, source, sourceLen, allowErrorRoot, parser.lineContinuationEscapeByte(), poll, parser.language.TokenCount, acceptedLeaves, nodesByID); err != nil {
			return rejectTree(err)
		}
	}
	// accepted-root-leading-gap: the derivation's own root reduce is exempt
	// from diagnosticParserCoreReduceChildrenTilingGap (isDerivationRootReduce,
	// above), so a root whose declared span starts strictly after the source's
	// first non-trivia byte passes that check by construction -- no child ever
	// needed to cover the missing prefix because the root itself never claimed
	// it. finalizeDiagnosticParserCoreAcceptedRootSpan does not catch this
	// either: it runs after normalizeRootSourceStart has already pulled
	// root.startByte back to expectedStart, so its equality check is
	// tautological for this exact shape. acceptedRootSpanStart (captured
	// before that pull-back) is the only place this information still exists.
	// A raw start after the first real byte means at least one leading byte
	// was never represented by any node in the accepted derivation; decline
	// fail-closed rather than let normalizeRootSourceStart's legitimate-
	// elision assumption launder it into a clean tree. Conservative by
	// design: a genuine legitimately-elided leading extra also declines here
	// and falls back to production, which still serves it correctly.
	if expectedStart := firstNonTriviaByteStart(source); acceptedRootSpanStart > expectedStart {
		return rejectTree(&diagnosticParserCoreDecline{
			boundary: DiagnosticParserCoreAccept,
			detail: fmt.Sprintf(
				"accepted-root-leading-gap: accepted derivation's own root span started at byte %d, after source's first non-trivia byte %d",
				acceptedRootSpanStart, expectedStart,
			),
		})
	}
	// A recover_eof result carries an EOF runtime for the original source.
	// Never let an edited copy enter incremental reuse with that stale boundary.
	tree.incrementalReuseDisabled = rootFinalization == diagnosticParserCoreFinalizeRecoverEOF ||
		!incrementalReuseProven || !scannerProvenanceTransferProven
	tree.compactMaterialized = true
	tree.setParseRuntime(ParseRuntime{
		StopReason:                                     ParseStopAccepted,
		SourceLen:                                      sourceLen,
		ExpectedEOFByte:                                sourceLen,
		RootEndByte:                                    root.endByte,
		LastTokenEndByte:                               sourceLen,
		LastTokenSymbol:                                0,
		LastTokenWasEOF:                                true,
		ExternalScannerCheckpointRecords:               arena.externalScannerCheckpointRecords,
		ExternalScannerCheckpointSlotsAllocated:        arena.externalScannerCheckpointSlotsAllocated(),
		ExternalScannerCheckpointBytesAllocated:        arena.externalScannerCheckpointBytesAllocated(),
		ExternalScannerSnapshotBytesAllocated:          arena.externalScannerSnapshotPayloadBytes,
		ExternalScannerCheckpointLeafNodes:             arena.externalScannerCheckpointLeafNodes,
		CompactExternalScannerCheckpointTransferProven: scannerProvenanceTransferProven,
	})
	return tree, nil
}

// diagnosticParserCoreStopControlTripped renders a poll-detected stop-control
// trip (spec.campaign.v7 tranche B8: memory budget, deadline, or
// cancellation) as the same kind of decline every other scheduler cap uses.
// Returning a real error here -- not the graceful s.finish(...) receipt path
// -- matters: run executes inside compact.RunFreshSchedulerSession for the
// admission-candidate route (options.freshSchedulerSession), whose deferred
// cleanup resets the whole core on any non-nil error. That is what releases
// the compact arenas' accumulated storage before the caller's production
// fallback engages, with no extra cleanup call needed here.
func diagnosticParserCoreStopControlTripped(reason ParseStopReason) error {
	return &diagnosticParserCoreDecline{
		boundary: DiagnosticParserCoreCap,
		detail:   "scheduler stop-control tripped: " + string(reason),
	}
}

// stopControlFootprintChurnRatio documents a measured, deliberately UNUSED
// lever (tranche B9 honest-accounting gate). FootprintBytes gauges retained
// structure; it is blind to per-token ephemeral allocation (temporary
// values the dispatch/election hot path creates and discards -- boxed
// action results, scanner-state capture buffers, and similar -- that a live
// GC reclaims continuously in normal operation but that accumulate
// unbounded in any measurement that holds GC off for the whole parse,
// including the RCA-era production replica test this poll is compared
// against). On the giant-table-literal witness, a scheduler run whose
// tracked footprint reached 103.6 MB at decline had allocated 516.5 MB
// cumulative by then: a ~5x ratio.
//
// A runtime-heap-based soft stop is not an option here. The determinism
// contract at parser_memory_budget_runtime.go:162-172 (issue #454) bars a
// runtime.MemStats reading (HeapAlloc, Sys) from stopping a parse at
// anything but the absolute hard ceiling: both readings are process-global
// and shift run to run with GC timing, not with the input, so using either
// one for the SOFT per-parse budget would make this poll's trip point
// non-deterministic. FootprintBytes is the deterministic, capacity-based
// gauge that keeps the soft budget reproducible instead.
//
// Discounting the comparison threshold by a fixed divisor (tripping the
// poll at budget/divisor instead of budget) was tried and reverted: at
// divisor 2, the giant-table-literal replica still exceeded the 6x
// cumulative-allocation contract (6.15-6.70x measured, still over), and a
// realistic, currently-passing witness (a 140KB clean Go source, budget 48
// MB) started declining before completion, because ITS OWN legitimate
// footprint at completion (measured (34,36] MB) already exceeds budget/2. At
// divisor 3 the replica came inside the contract (5.0-5.9x) but the same
// 140KB/48MB witness still regressed. No tested divisor cleared the
// pathological witness without cutting into ordinary coverage, because the
// two witnesses need materially different discounts: the giant literal's
// churn ratio is a property of ITS shape (dense, repeated struct-literal
// reduction), not a universal constant every input pays.
//
// stopControlMemoryBudgetReason therefore compares FootprintBytes against
// the configured budget with NO discount (ratio effectively 1): the honest,
// cap()-based, structure-complete gauge alone, with its own measured
// improvement (11.51x to 9.14-9.56x cumulative allocation on the same
// replica, down from the pre-B9-honest-accounting baseline). That
// improvement is not free at low budgets: FootprintBytes reads higher than
// the length-only StorageBytes gauge tranche B9 replaced, so the minimum
// budget a realistic witness needs to still route (rather than decline)
// rose too -- measured +25-29% on two witnesses (the same 140KB clean Go
// source referenced above: 28 MB to 36 MB; a 238KB one: 48 MB to 60 MB).
// The cost is nil at the shipped 512 MB default budget, which clears both
// thresholds with wide margin; it only reaches a caller who configured a
// budget close to a witness's pre-B9 threshold. Closing the remaining gap
// to the 6x contract needs either an owner decision to accept the
// divisor-discount coverage cost described above, or a deeper change to
// reduce the scheduler's own per-token ephemeral allocation rate (out of
// this tranche's scope). See the tranche's PR for the full witness table.
const stopControlFootprintChurnRatio = 1

func diagnosticParserCoreSliceAliases[T any](items []T, inline []T) bool {
	if cap(items) == 0 || len(inline) == 0 {
		return false
	}
	return unsafe.Pointer(&items[:cap(items)][0]) == unsafe.Pointer(&inline[0])
}

type diagnosticParserCoreFootprintRef struct {
	pointer unsafe.Pointer
	kind    uint8
}

const (
	diagnosticParserCoreFootprintState uint8 = iota + 1
	diagnosticParserCoreFootprintRegion
	diagnosticParserCoreFootprintSnapshot
)

func appendDiagnosticParserCoreFootprintRef(
	refs []diagnosticParserCoreFootprintRef,
	kind uint8,
	pointer unsafe.Pointer,
) []diagnosticParserCoreFootprintRef {
	if pointer == nil {
		return refs
	}
	return append(refs, diagnosticParserCoreFootprintRef{pointer: pointer, kind: kind})
}

func appendDiagnosticParserCoreVersionFootprintRefs(
	refs []diagnosticParserCoreFootprintRef,
	state *diagnosticParserCoreVersionState,
) []diagnosticParserCoreFootprintRef {
	if state == nil {
		return refs
	}
	refs = appendDiagnosticParserCoreFootprintRef(refs, diagnosticParserCoreFootprintState, unsafe.Pointer(state))
	refs = appendDiagnosticParserCoreFootprintRef(refs, diagnosticParserCoreFootprintRegion, unsafe.Pointer(state.s3Region))
	return appendDiagnosticParserCoreFootprintRef(refs, diagnosticParserCoreFootprintSnapshot, unsafe.Pointer(state.relexSnapshot))
}

func appendDiagnosticParserCoreHeaderFootprintRefs(
	refs []diagnosticParserCoreFootprintRef,
	headers []diagnosticParserCoreHeader,
) []diagnosticParserCoreFootprintRef {
	for index := 0; index < cap(headers); index++ {
		refs = appendDiagnosticParserCoreVersionFootprintRefs(refs, headers[:cap(headers)][index].versionState)
	}
	return refs
}

func appendDiagnosticParserCoreCanonicalScratchFootprintRefs(
	refs []diagnosticParserCoreFootprintRef,
	scratch *diagnosticParserCoreCanonicalScratch,
) []diagnosticParserCoreFootprintRef {
	if scratch == nil {
		return refs
	}
	for _, key := range scratch.keys {
		refs = appendDiagnosticParserCoreVersionFootprintRefs(refs, key.versionState)
	}
	for key := range scratch.groups {
		refs = appendDiagnosticParserCoreVersionFootprintRefs(refs, key.versionState)
	}
	return refs
}

func appendDiagnosticParserCoreVersionLexerRequestFootprintRefs(
	refs []diagnosticParserCoreFootprintRef,
	requests []diagnosticParserCoreVersionLexerRequest,
) []diagnosticParserCoreFootprintRef {
	for index := range requests {
		refs = appendDiagnosticParserCoreFootprintRef(refs, diagnosticParserCoreFootprintSnapshot, unsafe.Pointer(requests[index].before))
		refs = appendDiagnosticParserCoreFootprintRef(refs, diagnosticParserCoreFootprintSnapshot, unsafe.Pointer(requests[index].after))
	}
	return refs
}

func diagnosticParserCoreDFARelexSnapshotRetainedBytes(snapshot dfaRelexSnapshot) uint64 {
	total := uint64(0)
	add := func(count int, size uintptr) {
		if count <= 0 || size == 0 || total == math.MaxUint64 {
			return
		}
		if uint64(count) > math.MaxUint64/uint64(size) {
			total = math.MaxUint64
			return
		}
		bytes := uint64(count) * uint64(size)
		if math.MaxUint64-total < bytes {
			total = math.MaxUint64
			return
		}
		total += bytes
	}
	add(cap(snapshot.externalPayload), 1)
	add(cap(snapshot.externalTokenStart), 1)
	add(cap(snapshot.externalTokenEnd), 1)
	add(cap(snapshot.extZeroTried), unsafe.Sizeof(bool(false)))
	return total
}

func diagnosticParserCoreDFARelexSnapshotAndScratchRetainedBytes(
	snapshot dfaRelexSnapshot,
	scratch dfaRelexSnapshotScratch,
) uint64 {
	total := uint64(0)
	addPair := func(left, right unsafe.Pointer, leftCap, rightCap int, size uintptr) {
		if size == 0 || total == math.MaxUint64 {
			return
		}
		count := leftCap + rightCap
		if left != nil && left == right {
			count = max(leftCap, rightCap)
		}
		if count <= 0 || uint64(count) > math.MaxUint64/uint64(size) {
			if count > 0 {
				total = math.MaxUint64
			}
			return
		}
		bytes := uint64(count) * uint64(size)
		if math.MaxUint64-total < bytes {
			total = math.MaxUint64
			return
		}
		total += bytes
	}
	bytePointer := func(items []byte) unsafe.Pointer {
		if cap(items) == 0 {
			return nil
		}
		return unsafe.Pointer(&items[:cap(items)][0])
	}
	boolPointer := func(items []bool) unsafe.Pointer {
		if cap(items) == 0 {
			return nil
		}
		return unsafe.Pointer(&items[:cap(items)][0])
	}
	addPair(bytePointer(snapshot.externalPayload), bytePointer(scratch.externalPayload), cap(snapshot.externalPayload), cap(scratch.externalPayload), 1)
	addPair(bytePointer(snapshot.externalTokenStart), bytePointer(scratch.externalTokenStart), cap(snapshot.externalTokenStart), cap(scratch.externalTokenStart), 1)
	addPair(bytePointer(snapshot.externalTokenEnd), bytePointer(scratch.externalTokenEnd), cap(snapshot.externalTokenEnd), cap(scratch.externalTokenEnd), 1)
	addPair(boolPointer(snapshot.extZeroTried), boolPointer(scratch.extZeroTried), cap(snapshot.extZeroTried), cap(scratch.extZeroTried), unsafe.Sizeof(bool(false)))
	return total
}

func compareDiagnosticParserCoreFootprintRefs(
	left, right diagnosticParserCoreFootprintRef,
) int {
	if left.kind < right.kind {
		return -1
	}
	if left.kind > right.kind {
		return 1
	}
	leftPointer, rightPointer := uintptr(left.pointer), uintptr(right.pointer)
	if leftPointer < rightPointer {
		return -1
	}
	if leftPointer > rightPointer {
		return 1
	}
	return 0
}

func diagnosticParserCoreSchedulerFootprintBytes(s *diagnosticParserCoreGenericScheduler) uint64 {
	if s == nil {
		return 0
	}
	total := uint64(0)
	addBytes := func(bytes uint64) {
		if total == math.MaxUint64 || bytes == 0 {
			return
		}
		if math.MaxUint64-total < bytes {
			total = math.MaxUint64
			return
		}
		total += bytes
	}
	add := func(count int, size uintptr) {
		if count <= 0 || size == 0 || total == math.MaxUint64 {
			return
		}
		countBytes := uint64(count)
		sizeBytes := uint64(size)
		if countBytes > math.MaxUint64/sizeBytes {
			total = math.MaxUint64
			return
		}
		addBytes(countBytes * sizeBytes)
	}
	// Header copies share immutable version-state pointers. Count each owned
	// wrapper, region, and lexer snapshot once across the active frontier,
	// canonical keys/groups, and retained header scratch. The scheduler-owned
	// buffer keeps repeated polls allocation-free while growing exactly for a
	// wider frontier.
	refs := s.footprintRefs[:0]
	defer func() {
		clear(refs)
		s.footprintRefs = refs[:0]
	}()
	refs = appendDiagnosticParserCoreHeaderFootprintRefs(refs, s.headers)
	refs = appendDiagnosticParserCoreHeaderFootprintRefs(refs, s.headerRollbackScratch.headers)
	refs = appendDiagnosticParserCoreHeaderFootprintRefs(refs, s.headerRollbackScratch.inline[:])
	for index := range s.canonicalScratch.inlineHeaders {
		refs = appendDiagnosticParserCoreHeaderFootprintRefs(refs, s.canonicalScratch.inlineHeaders[index][:])
		refs = appendDiagnosticParserCoreHeaderFootprintRefs(refs, s.canonicalScratch.headerBuffers[index])
	}
	refs = appendDiagnosticParserCoreHeaderFootprintRefs(refs, s.conflictScratch.outputs)
	refs = appendDiagnosticParserCoreHeaderFootprintRefs(refs, s.conflictScratch.headerAssembly)
	refs = appendDiagnosticParserCoreHeaderFootprintRefs(refs, s.reductionReplacements)
	refs = appendDiagnosticParserCoreHeaderFootprintRefs(refs, s.seedHeaders[:])
	refs = appendDiagnosticParserCoreCanonicalScratchFootprintRefs(refs, &s.canonicalScratch)
	refs = appendDiagnosticParserCoreVersionLexerRequestFootprintRefs(refs, s.versionLexerRequests)
	slices.SortFunc(refs, compareDiagnosticParserCoreFootprintRefs)
	for index := 0; index < len(refs); {
		ref := refs[index]
		next := index + 1
		for next < len(refs) && compareDiagnosticParserCoreFootprintRefs(ref, refs[next]) == 0 {
			next++
		}
		switch ref.kind {
		case diagnosticParserCoreFootprintState:
			addBytes(uint64(unsafe.Sizeof(diagnosticParserCoreVersionState{})))
		case diagnosticParserCoreFootprintRegion:
			addBytes(diagnosticParserCoreVersionS3RegionFootprintBytes((*diagnosticParserCoreS3Region)(ref.pointer)))
		case diagnosticParserCoreFootprintSnapshot:
			addBytes(diagnosticParserCoreVersionLexerSnapshotFootprintBytes((*diagnosticParserCoreVersionLexerSnapshot)(ref.pointer)))
		}
		index = next
	}
	add(cap(refs), unsafe.Sizeof(diagnosticParserCoreFootprintRef{}))
	add(cap(s.headers), unsafe.Sizeof(diagnosticParserCoreHeader{}))
	add(cap(s.summaryHeaderScratch), unsafe.Sizeof(DiagnosticParserCoreHeaderReceipt{}))
	add(len(s.headerRollbackScratch.inline), unsafe.Sizeof(diagnosticParserCoreHeader{}))
	if !diagnosticParserCoreSliceAliases(s.headerRollbackScratch.headers, s.headerRollbackScratch.inline[:]) {
		add(cap(s.headerRollbackScratch.headers), unsafe.Sizeof(diagnosticParserCoreHeader{}))
	}
	add(len(s.canonicalScratch.inlineHeaders[0]), unsafe.Sizeof(diagnosticParserCoreHeader{}))
	add(len(s.canonicalScratch.inlineHeaders[1]), unsafe.Sizeof(diagnosticParserCoreHeader{}))
	for index := range s.canonicalScratch.headerBuffers {
		aliasesInline := diagnosticParserCoreSliceAliases(s.canonicalScratch.headerBuffers[index], s.canonicalScratch.inlineHeaders[0][:]) ||
			diagnosticParserCoreSliceAliases(s.canonicalScratch.headerBuffers[index], s.canonicalScratch.inlineHeaders[1][:])
		if !aliasesInline {
			add(cap(s.canonicalScratch.headerBuffers[index]), unsafe.Sizeof(diagnosticParserCoreHeader{}))
		}
	}
	if !diagnosticParserCoreSliceAliases(s.canonicalScratch.keys, s.canonicalScratch.inlineKeys[:]) {
		add(cap(s.canonicalScratch.keys), unsafe.Sizeof(diagnosticParserCorePhaseHead{}))
	}
	add(len(s.canonicalScratch.inlineKeys), unsafe.Sizeof(diagnosticParserCorePhaseHead{}))
	addBytes(s.canonicalScratch.groupsRetainedBytes)
	add(cap(s.dispatchScratch.cells), unsafe.Sizeof(diagnosticParserCoreGenericCell{}))
	add(cap(s.dispatchScratch.noActionIndices), unsafe.Sizeof(int(0)))
	add(cap(s.conflictScratch.actionOutputs), unsafe.Sizeof(diagnosticParserCoreActionOutput{}))
	add(cap(s.conflictScratch.reductionOutputs), unsafe.Sizeof(core.ReductionOutput{}))
	add(cap(s.conflictScratch.outputs), unsafe.Sizeof(diagnosticParserCoreHeader{}))
	add(cap(s.conflictScratch.armRanges), unsafe.Sizeof(diagnosticParserCoreConflictArmRange{}))
	add(cap(s.conflictScratch.adopted), unsafe.Sizeof(int(0)))
	add(cap(s.conflictScratch.headerAssembly), unsafe.Sizeof(diagnosticParserCoreHeader{}))
	add(cap(s.reductionOutputs), unsafe.Sizeof(core.ReductionOutput{}))
	add(cap(s.reductionReplacements), unsafe.Sizeof(diagnosticParserCoreHeader{}))
	add(cap(s.recoveryCondenseScratch), unsafe.Sizeof(diagnosticParserCoreRecoveryCondenseEntry{}))
	add(cap(s.recoveryCondenseOrderScratch), unsafe.Sizeof(int(0)))
	add(cap(s.classifiedBoundaries), unsafe.Sizeof(core.ClassifiedBoundary{}))
	add(cap(s.condenseCandidates), unsafe.Sizeof(core.CondenseCandidate{}))
	add(cap(s.electStates), unsafe.Sizeof(StateID(0)))
	add(cap(s.electGLRStates), unsafe.Sizeof(StateID(0)))
	add(cap(s.acceptedPayloads), unsafe.Sizeof(core.SubtreeID(0)))
	add(cap(s.versionLexerRequests), unsafe.Sizeof(diagnosticParserCoreVersionLexerRequest{}))
	addBytes(diagnosticParserCoreDFARelexSnapshotAndScratchRetainedBytes(
		s.versionLexerBefore, s.versionLexerBeforeScratch,
	))
	add(len(s.seedHeaders), unsafe.Sizeof(diagnosticParserCoreHeader{}))
	add(len(s.corridorCells), unsafe.Sizeof(diagnosticParserCoreGenericCell{}))
	add(1, unsafe.Sizeof(s.tokenCell))
	if s.compact != nil {
		coreBytes := s.compact.FootprintBytes()
		if math.MaxUint64-total < coreBytes {
			total = math.MaxUint64
		} else {
			total += coreBytes
		}
	}
	return total
}

// stopControlMemoryBudgetReason compares the compact core's and scheduler's real
// retained-memory footprint against the production engine's soft per-parse
// byte budget (stopControlMemoryBudgetBytes, sourced from
// parseMemoryBudgetForParser so the same GOT_PARSE_MEMORY_BUDGET_MB
// configuration governs both engines) and, independently, against
// production's own absolute hard ceiling (stopControlHardCeilingBytes,
// armed even when the soft budget is disabled). Every input is
// Core.FootprintBytes(): already-tracked slice/map length and capacity
// reads times compile-time-constant record sizes, so this is pure
// deterministic integer arithmetic -- no wall clock, no GC-timing
// dependence, and (same input, same budget) the same trip point on every
// run, unlike the runtime heap/sys signal production's own hard ceiling
// poll uses (parser_memory_budget_runtime.go). The scheduler contribution
// counts retained and ephemeral slice capacities, while Core.FootprintBytes
// counts Core-owned spill only once. FootprintBytes, not
// StorageBytes, is deliberate here: StorageBytes counts live length only,
// so it reads near zero for a core whose arenas hold retained capacity from
// an earlier declined attempt on the same cached runner, and it never
// counted scratch, the boundary index, or checkpoint interning at all --
// either gap let a pathological input's real footprint clear the configured
// budget well before this poll noticed (tranche B9 honest-accounting gate).
// See stopControlFootprintChurnRatio's doc comment for the ephemeral-churn
// gap this gauge still has, and why closing it further is left as an owner
// decision rather than a silent default change.
func (s *diagnosticParserCoreGenericScheduler) stopControlMemoryBudgetReason() ParseStopReason {
	if s == nil {
		return ParseStopNone
	}
	footprint := uint64(0)
	haveFootprint := false
	footprintAtLeast := func(bytes int64) bool {
		if bytes <= 0 {
			return false
		}
		if !haveFootprint {
			footprint = diagnosticParserCoreSchedulerFootprintBytes(s)
			haveFootprint = true
		}
		ratio := uint64(stopControlFootprintChurnRatio)
		scaled := footprint
		if ratio != 0 && footprint > math.MaxUint64/ratio {
			scaled = math.MaxUint64
		} else {
			scaled = footprint * ratio
		}
		return scaled >= uint64(bytes)
	}
	if footprintAtLeast(s.options.stopControlMemoryBudgetBytes) {
		return ParseStopMemoryBudget
	}
	if footprintAtLeast(s.options.stopControlHardCeilingBytes) {
		return ParseStopMemoryBudget
	}
	return ParseStopNone
}

// pollStopControl is the bounded scheduler-boundary poll (spec.campaign.v7
// tranche B8): the memory-budget check above, then the exact production
// deadline and cancellation check. Admission candidates also run the node-cap
// predictor after those controls. The poll runs before the first election,
// once per dispatch loop, and during an S5 terminal scan. Diagnostic callers
// bind no Parser, so they do not run the predictor.
func (s *diagnosticParserCoreGenericScheduler) pollStopControl() error {
	if reason := s.stopControlMemoryBudgetReason(); reason != ParseStopNone {
		return diagnosticParserCoreStopControlTripped(reason)
	}
	parser := s.options.stopControlParser
	if parser == nil {
		return nil
	}
	if reason := parser.activeParseStopReason(); parseStopReasonIsActive(reason) {
		return diagnosticParserCoreStopControlTripped(reason)
	}
	return s.observeCapPressure()
}

// observeCapPressure stops a compact attempt that is on a stable path to the
// node cap. Production then parses the source once, without the doomed tail.
func (s *diagnosticParserCoreGenericScheduler) observeCapPressure() error {
	if s == nil || s.options.stopControlParser == nil || s.compact == nil || s.tokenSource == nil ||
		len(s.headers) == 0 || s.capPressure.samples >= 2 {
		return nil
	}
	maxNodes := s.options.Limits.MaxNodes
	sourceLen := s.tokenSource.sourceLength()
	if sourceLen <= 0 || maxNodes == 0 {
		return nil
	}
	if uint64(sourceLen) > uint64(^uint32(0)) {
		return errors.New("parser-core phase zero: cap-pressure source length exceeds uint32")
	}
	sourceBytes := uint32(sourceLen)
	if !diagnosticParserCoreCapPressureSourceEligible(sourceBytes, maxNodes) {
		return nil
	}
	stats, err := s.compact.Stats(s.headers[0].head)
	if err != nil {
		return err
	}
	if threshold := s.capPressure.nextThreshold(maxNodes); threshold == 0 || stats.Nodes < threshold {
		return nil
	}
	progress := s.token.EndByte
	for _, header := range s.headers {
		_, byteOffset, boundaryErr := s.compact.Boundary(header.head)
		if boundaryErr != nil {
			return boundaryErr
		}
		if byteOffset > progress {
			progress = byteOffset
		}
	}
	prior := s.capPressure.priorProjectedNodes
	decline, projected := s.capPressure.observe(stats.Nodes, progress, sourceBytes, maxNodes)
	if decline {
		return &diagnosticParserCoreDecline{
			boundary: DiagnosticParserCoreCap,
			detail: fmt.Sprintf(
				"scheduler projected node arena cap: nodes=%d progress=%d/%d projected=%d prior=%d cap=%d",
				stats.Nodes, progress, sourceLen, projected, prior, maxNodes,
			),
		}
	}
	return nil
}

var errDiagnosticParserCoreTerminalSchedulerResume = errors.New("parser-core phase zero: terminal scheduler cannot resume")

func (s *diagnosticParserCoreGenericScheduler) run() error {
	if s != nil && s.receipt != nil &&
		(s.receipt.Acceptance != nil || s.receipt.Completion != nil || s.receipt.Stop.Detail != "") {
		return errDiagnosticParserCoreTerminalSchedulerResume
	}
	if err := s.pollStopControl(); err != nil {
		return err
	}
	if err := s.elect(true); err != nil {
		return err
	}
	if s.stoppedAfterElection {
		s.publishTotals()
		return nil
	}
	for {
		if err := s.pollStopControl(); err != nil {
			return err
		}
		if uint64(len(s.headers)) > s.work.PeakHeaders {
			s.work.PeakHeaders = uint64(len(s.headers))
		}
		allClosed := true
		accepted := 0
		shifted := 0
		for _, header := range s.headers {
			if header.accepted {
				accepted++
			}
			if header.shifted {
				shifted++
			}
			if !header.shifted && !header.accepted {
				allClosed = false
				break
			}
		}
		if allClosed {
			if s.versionLexerOwnershipActive && accepted == 0 {
				if len(s.headers) == 1 {
					if err := s.rejoinSharedLexerFromOwnedHeader(); err != nil {
						return err
					}
					if err := s.elect(false); err != nil {
						return err
					}
					if s.stoppedAfterElection {
						s.publishTotals()
						return nil
					}
					continue
				}
				if err := s.beginNextVersionLexerElection(); err != nil {
					return err
				}
				continue
			}
			if accepted != 0 {
				if shifted != 0 || accepted != len(s.headers) {
					return s.finish(DiagnosticParserCoreRoute, "generic scheduler cannot mix accepted and shifted heads", 0)
				}
				return s.completeAcceptance()
			}
			if s.options.GenericStopAtClosedByte != nil {
				completed, err := s.completeAtClosedByte(*s.options.GenericStopAtClosedByte)
				if err != nil {
					return err
				}
				if completed {
					return nil
				}
			}
			if err := s.elect(false); err != nil {
				return err
			}
			if s.stoppedAfterElection {
				s.publishTotals()
				return nil
			}
			continue
		}
		// C4 corridor: when the frontier is the deterministic single-header
		// shape the bytecode lane owns, run the compiled program instead of
		// the interpreted dispatch pass. The corridor executes
		// election-to-election corridors and returns on any boundary opcode;
		// it never produces a decline of its own, so the generic pass below
		// still owns every boundary verbatim (spec.c4-bytecode-isa.v1
		// section 6.2).
		if s.corridorEligible() {
			progressed, err := s.dispatchCorridor()
			if err != nil {
				return err
			}
			if progressed {
				continue
			}
		}
		stop, err := s.dispatchPass()
		if err != nil {
			return err
		}
		if stop != nil {
			return s.finish(stop.boundary, stop.detail, stop.headerIndex)
		}
		if s.receipt != nil &&
			(s.receipt.Acceptance != nil || s.receipt.Completion != nil || s.receipt.Stop.Detail != "") {
			return nil
		}
	}
}

type diagnosticParserCoreGenericUnsupported struct {
	boundary    DiagnosticParserCoreBoundaryKind
	detail      string
	headerIndex int
}

const diagnosticParserCoreNoTableActionDetail = "generic scheduler has no table action for the elected token"

// DiagnosticParserCoreNoTableActionDetailForTest exposes
// diagnosticParserCoreNoTableActionDetail so the external test package can
// assert on the genuinely-empty-row decline's exact detail. This proves that
// DisablePerHeaderSpanUnlockedRelex restores the legacy route.
func DiagnosticParserCoreNoTableActionDetailForTest() string {
	return diagnosticParserCoreNoTableActionDetail
}

// diagnosticParserCoreContextualCloseAngleDeferralDetail is the decline
// detail for a no-action head that reached dispatchPassActive's no-action
// classification through the issue #983 contextual close-angle deferral,
// not a genuinely empty action row. Kept distinct from
// diagnosticParserCoreNoTableActionDetail so census, receipts, and tests can
// tell the two shapes apart even though both share the Recovery boundary.
const diagnosticParserCoreContextualCloseAngleDeferralDetail = "generic scheduler deferred a contextual close-angle action for the elected token"

// DiagnosticParserCoreContextualCloseAngleDeferralDetailForTest exposes
// diagnosticParserCoreContextualCloseAngleDeferralDetail so the external
// test package can assert on the issue #983 deferral's decline detail
// without duplicating the literal string.
func DiagnosticParserCoreContextualCloseAngleDeferralDetailForTest() string {
	return diagnosticParserCoreContextualCloseAngleDeferralDetail
}

// diagnosticParserCoreRaggedRelexDeclineDetail is the former shared-cursor
// decline detail prefix. Keep it for telemetry migrations and regression
// tests that must distinguish the pre-ownership fallback from activation.
// It describes a no-action head whose own-mode relex found a different end.
//
// The active scheduler now preempts the shared pass and publishes owned lexer
// requests at the first such witness. It does not emit this detail there.
//
// The different-span shape starts where the shared election started but ends
// at another byte. It can be wider or narrower.
//
// Historical detail format:
// for a no-action head whose own-mode per-header relex (relexTokenForState)
// found a genuine internal-DFA token starting where the shared election
// started but ending at a different byte (D2-1's different-span shape,
// wider or narrower -- not always wider despite the "ragged-end" name
// still used in a few surrounding identifiers and comments). This
// scheduler shares one token source's byte cursor across every header in a
// pass; shifting one header onto a wider or narrower span than every other
// header sees would desynchronize that shared cursor. Kept distinct
// from diagnosticParserCoreNoTableActionDetail and
// diagnosticParserCoreContextualCloseAngleDeferralDetail so census,
// receipts, and tests can tell the three shapes apart even though all three
// share the Recovery boundary. The full decline detail appends the
// offending relexed token's symbol and span (fmt.Sprintf), matching this
// file's existing dynamic-detail pattern (e.g. "accepted-leaf-tiling-gap").
const diagnosticParserCoreRaggedRelexDeclineDetail = "generic scheduler declined a per-header token with a different span"

// DiagnosticParserCoreRaggedRelexDeclineDetailForTest exposes
// diagnosticParserCoreRaggedRelexDeclineDetail so the external test package
// can assert on the ragged-end decline's detail prefix without duplicating
// the literal string.
func DiagnosticParserCoreRaggedRelexDeclineDetailForTest() string {
	return diagnosticParserCoreRaggedRelexDeclineDetail
}

// diagnosticParserCoreRaggedRelexDeclineDetailFor builds the full ragged-end
// decline detail: the fixed prefix plus the offending relexed token's own
// symbol and span, and the shared election's span it declined to widen or
// narrow onto. A small standalone function so the exact format is directly
// testable without driving a full multi-header dispatch pass.
func diagnosticParserCoreRaggedRelexDeclineDetailFor(relexedWitness, shared Token) string {
	return fmt.Sprintf(
		"%s: relexed symbol=%d span=%d..%d shared span=%d..%d",
		diagnosticParserCoreRaggedRelexDeclineDetail,
		relexedWitness.Symbol, relexedWitness.StartByte, relexedWitness.EndByte,
		shared.StartByte, shared.EndByte,
	)
}

// DiagnosticParserCoreRaggedRelexDeclineDetailFormatForTest exposes
// diagnosticParserCoreRaggedRelexDeclineDetailFor so the external test
// package can assert on the exact ragged-end decline detail a given
// (relexed, shared) token pair produces.
func DiagnosticParserCoreRaggedRelexDeclineDetailFormatForTest(relexedWitness, shared Token) string {
	return diagnosticParserCoreRaggedRelexDeclineDetailFor(relexedWitness, shared)
}

func (s *diagnosticParserCoreGenericScheduler) dispatchPass() (*diagnosticParserCoreGenericUnsupported, error) {
	if err := s.dispatchScratch.begin(); err != nil {
		return nil, err
	}
	// Keep panic-safe scratch release in this small wrapper. The action body is
	// intentionally separate so its large frame does not require a runtime
	// defer registration on every scheduler pass.
	defer s.dispatchScratch.finish()
	return s.dispatchPassActive()
}

func (s *diagnosticParserCoreGenericScheduler) classifyVersionLexerCell(
	index int,
	recordWork bool,
) (diagnosticParserCoreGenericCell, bool, *diagnosticParserCoreGenericUnsupported, error) {
	if s == nil || index < 0 || index >= len(s.headers) {
		return diagnosticParserCoreGenericCell{}, false, nil, errors.New("parser-core phase zero: version lexer classification index is out of range")
	}
	header := &s.headers[index]
	if header.shifted || header.accepted || header.paused {
		return diagnosticParserCoreGenericCell{}, false, nil, nil
	}
	if header.recoveryRegion() != nil {
		return diagnosticParserCoreGenericCell{}, false, &diagnosticParserCoreGenericUnsupported{
			boundary: DiagnosticParserCoreRoute, detail: "generic scheduler owned lexer dispatch reached an open recovery region", headerIndex: index,
		}, nil
	}
	if err := s.requestHeaderLexerToken(index); err != nil {
		return diagnosticParserCoreGenericCell{}, false, nil, err
	}
	requestReference := s.versionLexerRequestReferenceForHeader(index)
	if requestReference == 0 {
		return diagnosticParserCoreGenericCell{}, false, nil, fmt.Errorf(
			"parser-core phase zero: version lexer request was not published for header %d",
			index,
		)
	}
	request := &s.versionLexerRequests[requestReference-1]
	if err := s.bindVersionLexerRequest(request); err != nil {
		return diagnosticParserCoreGenericCell{}, false, nil, err
	}
	if unsupported := diagnosticParserCoreGenericUnsupportedToken(request.token); unsupported != nil {
		unsupported.headerIndex = index
		return diagnosticParserCoreGenericCell{}, false, unsupported, nil
	}
	boundary, err := s.compact.ClassifyBoundary(header.head, core.Symbol(request.token.Symbol))
	if err != nil {
		return diagnosticParserCoreGenericCell{}, false, nil, err
	}
	if recordWork {
		s.work.ActionLookups++
	}
	actions := boundary.Actions()
	if recordWork {
		workCountRecordResolvedActionCell(actions.Len())
	}
	if actions.Len() == 0 {
		return diagnosticParserCoreGenericCell{}, true, nil, nil
	}
	cell := diagnosticParserCoreGenericCell{
		headerIndex: int32(index), boundary: boundary, versionLexerRequest: requestReference,
	}
	if ordinal, ok := diagnosticParserCoreConflictPolicyOrdinal(
		s.tokenSource.language,
		request.token,
		boundary.State(),
		actions,
		len(s.headers),
	); ok {
		cell.selectedOrdinal = int32(ordinal)
		cell.selectedBy = diagnosticParserCoreCellSelectionConflictPolicy
	}
	if cell.selectedBy == diagnosticParserCoreCellSelectionNone &&
		actions.Descriptor().Kind() == core.ActionRowUnsupported {
		if ordinal, ok := diagnosticParserCoreRepetitionFoldOrdinal(s.tokenSource.language, actions); ok {
			cell.selectedOrdinal = int32(ordinal)
			cell.selectedBy = diagnosticParserCoreCellSelectionRepetitionFold
		} else if _, ok := diagnosticParserCoreSingleReduceRepetitionShiftOrdinal(actions); ok &&
			cRepetitionSkipOptOut[s.tokenSource.language.Name] {
			cell.selectedBy = diagnosticParserCoreCellSelectionRepetitionFork
		}
	}
	if cell.selectedBy == diagnosticParserCoreCellSelectionNone && !cell.descriptor().DispatchSupported() {
		if unsupported := diagnosticParserCoreGenericUnsupportedCellDescriptor(index, request.token, actions, cell.descriptor()); unsupported != nil {
			return diagnosticParserCoreGenericCell{}, false, unsupported, nil
		}
	}
	return cell, false, nil, nil
}

func (s *diagnosticParserCoreGenericScheduler) versionLexerNoActionDropEligible(indices []int) bool {
	if s == nil || !s.versionLexerOwnershipActive || s.recoveryIsolation ||
		!diagnosticParserCoreGenericNoActionDropEligible(s.headers, indices, s.epochProgress) {
		return false
	}
	drop := 0
	sharedStart := uint32(0)
	startSet := false
	for index := range s.headers {
		isDrop := drop < len(indices) && indices[drop] == index
		if isDrop {
			drop++
			request := s.versionLexerRequestForHeader(index)
			if request == nil || s.headers[index].shifted || s.headers[index].accepted {
				return false
			}
			if !startSet {
				sharedStart, startSet = request.token.StartByte, true
			} else if request.token.StartByte != sharedStart {
				return false
			}
			continue
		}
		header := &s.headers[index]
		if !header.shifted || header.accepted || header.paused ||
			header.versionLexerRequestReference() != 0 {
			return false
		}
		matchedShift := false
		for requestIndex := range s.versionLexerRequests {
			request := &s.versionLexerRequests[requestIndex]
			if !request.valid || request.electionIndex != s.electionIndex ||
				!diagnosticParserCoreVersionLexerSnapshotEqual(request.after, header.versionLexerSnapshot()) {
				continue
			}
			if !startSet {
				sharedStart, startSet = request.token.StartByte, true
			} else if request.token.StartByte != sharedStart {
				return false
			}
			matchedShift = true
			break
		}
		if !matchedShift {
			return false
		}
	}
	return startSet && drop == len(indices)
}

func (s *diagnosticParserCoreGenericScheduler) dispatchVersionLexerPassActive() (*diagnosticParserCoreGenericUnsupported, error) {
	s.work.Passes++
	var before []DiagnosticParserCoreHeaderReceipt
	if s.fullReceipts() {
		var err error
		before, err = s.headerReceipts(s.headers)
		if err != nil {
			return nil, err
		}
	}
	cells := s.dispatchScratch.cells[:0]
	noActionIndices := s.dispatchScratch.noActionIndices[:0]
	acceptCell, extraCell, reductionCell, conflictCell, shiftCell := -1, -1, -1, -1, -1
	for index := range s.headers {
		if s.headers[index].paused {
			if s.headers[index].shifted || s.headers[index].accepted ||
				s.versionLexerRequestForHeader(index) == nil {
				return &diagnosticParserCoreGenericUnsupported{
					boundary:    DiagnosticParserCoreRoute,
					detail:      "generic scheduler owned lexer paused head is not authenticated",
					headerIndex: index,
				}, nil
			}
			noActionIndices = append(noActionIndices, index)
			continue
		}
		cell, noAction, unsupported, err := s.classifyVersionLexerCell(index, true)
		if err != nil || unsupported != nil {
			return unsupported, err
		}
		if noAction {
			noActionIndices = append(noActionIndices, index)
			continue
		}
		if s.headers[index].shifted || s.headers[index].accepted || s.headers[index].paused {
			continue
		}
		cellIndex := len(cells)
		cells = append(cells, cell)
		switch cell.kind() {
		case core.ActionRowAccept:
			if acceptCell < 0 {
				acceptCell = cellIndex
			}
		case core.ActionRowExtraShift:
			if extraCell < 0 {
				extraCell = cellIndex
			}
		case core.ActionRowReduce:
			if reductionCell < 0 {
				reductionCell = cellIndex
			}
		case core.ActionRowConflict:
			if cell.descriptor().HasReduce() && reductionCell < 0 {
				reductionCell = cellIndex
			}
			if conflictCell < 0 {
				conflictCell = cellIndex
			}
		case core.ActionRowShift:
			if shiftCell < 0 {
				shiftCell = cellIndex
			}
		}
	}
	s.dispatchScratch.cells = cells
	s.dispatchScratch.noActionIndices = noActionIndices
	if len(cells) == 0 {
		if s.versionLexerNoActionDropEligible(noActionIndices) {
			s.versionLexerNoActionProof = true
			defer func() { s.versionLexerNoActionProof = false }()
			if s.options.recordDropCohortFrontiers {
				if err := s.publishDropCohortFrontierOwned(noActionIndices); err != nil {
					return nil, err
				}
			}
			if err := s.consumeDropCohortFrontierOwned(noActionIndices); err != nil {
				return nil, err
			}
			return nil, s.dropGenericNoActionHeads(noActionIndices)
		}
		if len(noActionIndices) != 0 {
			return &diagnosticParserCoreGenericUnsupported{
				boundary: DiagnosticParserCoreRecovery, detail: diagnosticParserCoreNoTableActionDetail, headerIndex: noActionIndices[0],
			}, nil
		}
		return &diagnosticParserCoreGenericUnsupported{
			boundary: DiagnosticParserCoreRoute, detail: "generic scheduler owned lexer dispatch has no runnable head",
		}, nil
	}
	selected := -1
	operation := core.ActionRowEmpty
	competingRecoveryAccept := false
	switch {
	case acceptCell >= 0:
		competingRecoveryAccept = s.competingRecoveryFrontier() &&
			s.headers[cells[acceptCell].headerIndex].isRecoveryLineage()
		if !competingRecoveryAccept && (len(s.headers) != 1 || len(cells) != 1 || len(noActionIndices) != 0) {
			return &diagnosticParserCoreGenericUnsupported{
				boundary: DiagnosticParserCoreAccept, detail: "generic scheduler owned lexer accept requires one live version", headerIndex: int(cells[acceptCell].headerIndex),
			}, nil
		}
		selected, operation = acceptCell, core.ActionRowAccept
	case extraCell >= 0:
		selected, operation = extraCell, core.ActionRowExtraShift
	case reductionCell >= 0:
		selected = reductionCell
		if cells[selected].kind() == core.ActionRowConflict {
			operation = core.ActionRowConflict
		} else {
			operation = core.ActionRowReduce
		}
	case conflictCell >= 0:
		selected, operation = conflictCell, core.ActionRowConflict
	case shiftCell >= 0:
		selected, operation = shiftCell, core.ActionRowShift
	default:
		return &diagnosticParserCoreGenericUnsupported{
			boundary: DiagnosticParserCoreRoute, detail: "generic scheduler owned lexer dispatch found no supported action",
		}, nil
	}
	selectedHeader := int(cells[selected].headerIndex)
	cell, noAction, unsupported, err := s.classifyVersionLexerCell(selectedHeader, false)
	if err != nil || unsupported != nil {
		return unsupported, err
	}
	if noAction {
		return nil, errors.New("parser-core phase zero: version lexer action changed before dispatch")
	}
	if unsupported := s.validateGenericNoLookaheadReduction(
		[]diagnosticParserCoreGenericCell{cell},
		noActionIndices,
	); unsupported != nil {
		return unsupported, nil
	}
	if operation == core.ActionRowExtraShift {
		if unsupported := s.zeroWidthExtraShiftWithoutProgress(
			[]diagnosticParserCoreGenericCell{cell},
		); unsupported != nil {
			return unsupported, nil
		}
	}
	err = s.withVersionLexerRequest(cell, func() error {
		switch operation {
		case core.ActionRowAccept:
			if err := s.applyGenericAccept(before, cell); err != nil {
				return err
			}
			if competingRecoveryAccept {
				return s.completeAcceptance()
			}
			return nil
		case core.ActionRowExtraShift:
			return s.applyGenericExtraShifts(before, []diagnosticParserCoreGenericCell{cell})
		case core.ActionRowReduce:
			return s.applyGenericReduction(before, cell)
		case core.ActionRowConflict:
			return s.applyGenericConflict(before, cell)
		case core.ActionRowShift:
			return s.applyGenericShifts(before, []diagnosticParserCoreGenericCell{cell})
		default:
			return errors.New("parser-core phase zero: invalid version lexer dispatch operation")
		}
	})
	return nil, err
}

func (s *diagnosticParserCoreGenericScheduler) dispatchPassActive() (*diagnosticParserCoreGenericUnsupported, error) {
	if s.versionLexerOwnershipActive {
		return s.dispatchVersionLexerPassActive()
	}
	s.work.Passes++
	// R6 pass-mix counter. A single-header pass is exactly the shape the C4
	// bytecode corridor is eligible for, so this count makes corridor coverage
	// measurable on the committed board.
	if len(s.headers) == 1 {
		s.work.SingleHeaderPasses++
	}
	if unsupported := diagnosticParserCoreGenericUnsupportedToken(s.token); unsupported != nil {
		return unsupported, nil
	}
	recoveryCompetition := s.competingRecoveryFrontier()
	if s.recoveryIsolation && !recoveryCompetition {
		return &diagnosticParserCoreGenericUnsupported{
			boundary: DiagnosticParserCoreRecovery,
			detail:   "recovery competition crossed ordinary grammar ambiguity",
		}, nil
	}
	for index, header := range s.headers {
		if header.accepted {
			if recoveryCompetition {
				continue
			}
			return &diagnosticParserCoreGenericUnsupported{
				boundary: DiagnosticParserCoreAccept, detail: "generic scheduler found an accepted head before sole-frontier completion", headerIndex: index,
			}, nil
		}
	}
	var before []DiagnosticParserCoreHeaderReceipt
	if s.fullReceipts() {
		var err error
		before, err = s.headerReceipts(s.headers)
		if err != nil {
			return nil, err
		}
	}
	var singletonCells [1]diagnosticParserCoreGenericCell
	cells := singletonCells[:0]
	scratchCells := s.dispatchScratch.cells
	pausedNoActionHeads := 0
	deferredNoActionHeads := 0
	raggedRelexNoActionHeads := 0
	var raggedRelexWitness Token // the first ragged-end relex this pass, for the decline detail
	raggedRelexHeaderIndex := 0  // that same relex's own header index, for the decline receipt
	for index := range s.headers {
		header := &s.headers[index]
		if header.shifted || header.accepted {
			continue
		}
		if header.paused {
			s.dispatchScratch.noActionIndices = append(s.dispatchScratch.noActionIndices, index)
			pausedNoActionHeads++
			continue
		}
		if region := header.recoveryRegion(); region != nil {
			// A header sitting on an open region is the compact analogue of
			// a live C stack in ERROR_STATE, which lexes with a completely
			// different (most-permissive, LexModes[0]) mode than the
			// ordinary per-state shared election every other header uses
			// this pass (cRecoverElectionLookaheadSymbol's own doc comment,
			// parser_recover_c.go). Prefer that error-mode view whenever it
			// disagrees with the shared token by still reporting this
			// position unlexable: s3ErrorModeRelex's doc comment records the
			// witness (html_log_8) that needs this to avoid resuming one
			// byte early.
			resumeToken := s.token
			relexDisagreesUnmodeled := false
			if relexed, relexOK := s.s3ErrorModeRelex(region.endByte); relexOK {
				sharedIsRealContent := s.token.StartByte != s.token.EndByte
				switch {
				case relexed.Symbol == errorSymbol:
					resumeToken = relexed
				case sharedIsRealContent && (relexed.Symbol != s.token.Symbol || relexed.EndByte != s.token.EndByte):
					// REQUIRED 2b (adversarial review, corpus witness
					// "a>[/>"): the error-mode lexer (the lex state a live C
					// ERROR_STATE stack actually uses) found a REAL terminal
					// here too, but a differently-classified or differently-
					// wide one than the ordinary shared election found for
					// every other header this pass. Silently keeping the
					// ordinary (here, narrower) token let this region resume
					// one byte-run short of where C's own error-mode lexer
					// would have kept absorbing, producing a resumed tree
					// this single-path model cannot prove matches C. Only
					// the already-handled errorSymbol case above is proven
					// safe (html_log_8); every other disagreement between
					// two REAL (non-zero-width) lex views is unmodeled and
					// must decline, not silently prefer one view over the
					// other.
					//
					// Guarding on sharedIsRealContent (confirmed necessary:
					// html_erroneous_end_tag/html_log_6's own resume point)
					// keeps this from over-firing when the ordinary shared
					// token is zero-width: a zero-width token is a pure
					// lookahead/existence marker at this exact byte offset,
					// not consumed content, so its own table entry (used by
					// s3RegionResumeAction just below, as the lookahead
					// symbol for the resume-action probe, not as something
					// this code shifts) answers "would resuming exactly here
					// work" on its own terms -- disagreeing with the error-
					// mode lexer's independent, wider real-content
					// classification a few bytes later is expected, not a
					// sign this single-path model might be wrong.
					relexDisagreesUnmodeled = true
				}
			}
			hasAction, actErr := s3RegionResumeAction(s.compact, region.state, Symbol(resumeToken.Symbol))
			if actErr != nil {
				return nil, actErr
			}
			if relexDisagreesUnmodeled {
				return &diagnosticParserCoreGenericUnsupported{
					boundary:    DiagnosticParserCoreRoute,
					detail:      "generic scheduler s3 error region error-mode lex disagrees with the ordinary shared election in an unmodeled way",
					headerIndex: index,
				}, nil
			}
			// REQUIRED 2b (adversarial review): a real live C stack would
			// scan its stack summary up to depth cRecoverMaxSummaryDepth for
			// a state that resumes with an action before ever falling
			// through to strategy 2's next absorb (cRecoverDispatchInError).
			// S3 owns only depth-0 resume; probing deeper here is a bounded
			// existence check (AncestorStateWithActionExists's own doc
			// comment), not an attempt to perform that deeper resume.
			deeperResumeExists := false
			if !hasAction && resumeToken.Symbol != 0 {
				deeperResumeExists, actErr = s.compact.AncestorStateWithActionExists(header.head, core.Symbol(resumeToken.Symbol), cRecoverMaxSummaryDepth)
				if actErr != nil {
					return nil, actErr
				}
			}
			switch {
			case hasAction:
				if s.s3ResumeCount == math.MaxUint32 {
					return nil, errors.New("parser-core phase zero: strategy-2 resume count overflow")
				}
				s.s3ResumeState = StateID(region.state)
				s.s3ResumeSymbol = Symbol(resumeToken.Symbol)
				s.s3ResumeCount++
				// Depth-0 resume: the pre-error state now accepts the current
				// token. Publish the ERROR container over the absorbed
				// children and condense it onto the pre-error head (the
				// compact equivalent of cRecoverToState's
				// pushStackNode(fork, goal, errNode, ...)), then fall through
				// to ordinary classification below using the refreshed head.
				recoveryCost, recoveryCostMemo, costErr := s.recoveryOutputCostFunc()
				if costErr != nil {
					return nil, costErr
				}
				defer recoveryCostMemo.Reset()
				var newHead core.Head
				var resumeErr error
				if s.recoveryIsolation {
					resume := func(owner core.SchedulerTransactionToken) error {
						newHead, resumeErr = s.compact.ErrorRegionResumeWithLiveCondenseCandidatesAndCostOwned(
							owner, s.collectCondenseCandidates(index), header.head, region.state,
							region.startByte, region.endByte, region.children, recoveryCost,
						)
						return resumeErr
					}
					if s.freshSessionOwner != nil {
						resumeErr = resume(*s.freshSessionOwner)
					} else {
						resumeErr = s.compact.ApplySchedulerAtomic(resume)
					}
				} else {
					newHead, resumeErr = s.compact.ErrorRegionResumeWithCost(
						header.head, region.state, region.startByte, region.endByte, region.children,
						recoveryCost,
					)
				}
				if resumeErr != nil {
					return nil, resumeErr
				}
				s.headers[index].head = newHead
				s.headers[index].closeRecoveryRegion()
			case resumeToken.Symbol == 0:
				// EOF while a region is open: cRecoverEOFAccept's whole-file
				// wrap is out of S3 scope (s3TryOpenErrorRegion's doc
				// comment). Fall through to ordinary classification
				// unchanged; it finds no action against the still-open head
				// and lands back in noActionIndices, where
				// s3TryOpenErrorRegion bails (s3Region already set) and the
				// existing decline applies -- fail-closed, not a guess.
			case deeperResumeExists:
				// A stack entry above depth 0 would accept resumeToken. C can
				// recover there instead of continuing to absorb. This S4 unit
				// owns only the first no-action fork, so decline here.
				return &diagnosticParserCoreGenericUnsupported{
					boundary:    DiagnosticParserCoreRoute,
					detail:      "generic scheduler s3 error region found a deeper stack-summary resume opportunity outside single-path depth-0 scope",
					headerIndex: index,
				}, nil
			default:
				tokenExtra, extraErr := s3TokenIsExtraShift(s.compact, resumeToken.Symbol)
				if extraErr != nil {
					return nil, extraErr
				}
				leafID, leafErr := s.compact.ErrorRegionLeaf(core.Symbol(resumeToken.Symbol), resumeToken.StartByte, resumeToken.EndByte, tokenExtra)
				if leafErr != nil {
					return nil, leafErr
				}
				grown := make([]core.SubtreeID, len(region.children)+1)
				copy(grown, region.children)
				grown[len(region.children)] = leafID
				s.headers[index].setRecoveryRegion(&diagnosticParserCoreS3Region{
					state: region.state, startByte: region.startByte, endByte: resumeToken.EndByte, children: grown,
				})
				s.headers[index].shifted = true
				if resumeToken.EndByte != s.token.EndByte {
					// The error-mode relex consumed a different span than
					// the shared election (a wider unlexable run, matching
					// C's error-mode lexer): resync the shared token
					// source's cursor so the next elect() call continues
					// from where this absorb actually left off, not from
					// the shared token's own (now-stale) end.
					s.tokenSource.SeekTokenFrontier(resumeToken.EndByte, resumeToken.EndPoint)
					// The lexer moved without an election: the cell's elected
					// token/checkpoints no longer describe the token source's
					// position, so invalidate it rather than leave it stale.
					s.tokenCell.valid = false
				}
				// Return this pass after one recovery absorb. A paired missing
				// version remains unshifted and dispatches on the next pass. A
				// standalone S3 version otherwise reaches the "no runnable head"
				// branch if this loop falls through without recording a cell.
				// Confirmed necessary:
				// html_erroneous_end_tag/html_log_8 needs a second
				// consecutive absorb (the region opened by
				// s3TryOpenErrorRegion for '>' does not resume until the
				// error-mode-relexed run through 'o' completes), and only
				// this direct return reaches that second absorb at all.
				return nil, nil
			}
		}
		cellToken := s.token
		var relexedSymbol Symbol
		boundary, err := s.compact.ClassifyBoundary(header.head, core.Symbol(cellToken.Symbol))
		if err != nil {
			return nil, err
		}
		s.work.ActionLookups++
		actions := boundary.Actions()
		// Production's per-stack contextualActionIndex (parser_dfa_token_source.go)
		// zeroes an action for this exact shared token shape when the header's own
		// lex mode reads a wider close-angle operator that itself carries a real
		// action here: a differently-lexing stack already claimed the elected
		// narrow prefix, so this header must not shift it either. Skipping this
		// check would let the compact route accept a derivation the production
		// route provably declines (issue #983). A header this defers falls into
		// the ordinary no-action machinery below -- it never gets a synthesized
		// token or an altered election. The work-count record below is skipped
		// for a deferred cell (recorded as zero instead, matching production's
		// zeroed action index) and deferredNoActionHeads is tracked separately
		// from pausedNoActionHeads so the no-action classification below can
		// tell a deferral apart from a genuinely empty action row.
		if actions.Len() != 0 && s.tokenSource != nil && s.tokenSource.lexer != nil {
			probe := s.tokenSource.relexProbeLexer
			if probe == nil {
				probe = &Lexer{}
				s.tokenSource.relexProbeLexer = probe
			}
			if deferContextualCloseAngleAction(
				s.tokenSource.language, s.tokenSource.lexer.source, StateID(boundary.State()), cellToken, nil, probe,
			) {
				workCountRecordResolvedActionCell(0)
				s.dispatchScratch.noActionIndices = append(s.dispatchScratch.noActionIndices, index)
				deferredNoActionHeads++
				continue
			}
		}
		workCountRecordResolvedActionCell(actions.Len())
		if actions.Len() == 0 {
			state := StateID(boundary.State())
			if len(s.headers) > 1 {
				relexed, ok := s.relexTokenForState(state, s.token)
				if ok {
					// External scanner output carries mutable state beyond the
					// token span. Keep the entire frontier on owned requests,
					// even when the scanner produced the same-width token, so a
					// later election cannot observe a sibling's payload.
					if relexed.ExternalScannerToken {
						return s.activateVersionLexerOwnershipAtRagged(index)
					}
					if relexed.EndByte != s.token.EndByte {
						if raggedRelexNoActionHeads == 0 {
							raggedRelexWitness = relexed
							raggedRelexHeaderIndex = index
							raggedRelexNoActionHeads++
							// A different span proves that this election cannot
							// continue on the shared cursor. Discard every cell
							// classified earlier in this pass before ownership
							// publishes its per-header requests.
							s.dispatchScratch.cells = s.dispatchScratch.cells[:0]
							s.dispatchScratch.noActionIndices = s.dispatchScratch.noActionIndices[:0]
							return s.activateVersionLexerOwnershipAtRagged(index)
						}
						// A second witness cannot occur in this activation pass,
						// but keep the legacy accounting if a future caller
						// continues after the first owned request.
						raggedRelexNoActionHeads++
						s.dispatchScratch.noActionIndices = append(s.dispatchScratch.noActionIndices, index)
						continue
					}
					if relexed.Symbol != 0 && relexed.Symbol != s.token.Symbol &&
						relexed.StartByte == s.token.StartByte && relexed.EndByte == s.token.EndByte &&
						s.recoveryAmbiguityNeedsOwnedLexer(index) {
						return s.activateVersionLexerOwnershipAtRagged(index)
					}
					if verified, ok := diagnosticParserCoreSameSpanRelex(s.token, relexed); ok {
						relexedSymbol = verified.Symbol
						cellToken = verified
						boundary, err = s.compact.ClassifyBoundary(header.head, core.Symbol(cellToken.Symbol))
						if err != nil {
							return nil, err
						}
						s.work.ActionLookups++
						actions = boundary.Actions()
						workCountRecordResolvedActionCell(actions.Len())
					}
				}
			}
			if actions.Len() == 0 {
				s.dispatchScratch.noActionIndices = append(s.dispatchScratch.noActionIndices, index)
				continue
			}
		}
		cell := diagnosticParserCoreGenericCell{
			headerIndex:   int32(index),
			boundary:      boundary,
			relexedSymbol: relexedSymbol,
		}
		if s.tokenSource != nil {
			if ordinal, ok := diagnosticParserCoreConflictPolicyOrdinal(
				s.tokenSource.language,
				cell.dispatchToken(s.token),
				boundary.State(),
				actions,
				len(s.headers),
			); ok {
				cell.selectedOrdinal = int32(ordinal)
				cell.selectedBy = diagnosticParserCoreCellSelectionConflictPolicy
			}
			if cell.selectedBy == diagnosticParserCoreCellSelectionNone &&
				actions.Descriptor().Kind() == core.ActionRowUnsupported {
				if ordinal, ok := diagnosticParserCoreRepetitionFoldOrdinal(s.tokenSource.language, actions); ok {
					cell.selectedOrdinal = int32(ordinal)
					cell.selectedBy = diagnosticParserCoreCellSelectionRepetitionFold
				} else if _, ok := diagnosticParserCoreSingleReduceRepetitionShiftOrdinal(actions); ok &&
					cRepetitionSkipOptOut[s.tokenSource.language.Name] {
					// Production keeps both arms for a language with a proven
					// fold counterexample. Use the existing conflict executor.
					cell.selectedBy = diagnosticParserCoreCellSelectionRepetitionFork
				}
			}
		}
		if len(s.headers) == 1 {
			cells = append(cells, cell)
		} else {
			scratchCells = append(scratchCells, cell)
		}
	}
	if len(s.headers) != 1 {
		s.dispatchScratch.cells = scratchCells
		cells = scratchCells
	}
	noActionIndices := s.dispatchScratch.noActionIndices
	if unsupported := s.validateGenericNoLookaheadReduction(cells, noActionIndices); unsupported != nil {
		return unsupported, nil
	}
	acceptCell := -1
	extraCells := 0
	reductionCell := -1
	reductionConflict := false
	conflictCell := -1
	for index := range cells {
		cell := &cells[index]
		descriptor := cell.descriptor()
		// descriptor.DispatchSupported() is table-derived and immutable once
		// this row was decoded (core.describeActionRow, once per distinct
		// action row -- not once per dispatch pass): it is true exactly when
		// diagnosticParserCoreGenericUnsupportedCellDescriptor's own kind
		// switch below would already return a nil decline without reading
		// its token argument (Shift, Reduce, and Conflict rows). Skipping
		// the call for those rows avoids re-deriving that same kind-only
		// fact, and the cell.dispatchToken(s.token) token-struct copy that
		// building its argument would cost, on every pass that dispatches an
		// already-proven-supported cell (spec.campaign.v7 tranche C0 item 4,
		// the "cell-array and descriptor validation" L1 sub-item). Rows that
		// still need the token (ExtraShift, Accept) or are never supported
		// (Empty, Unsupported) keep paying the full per-pass call unchanged.
		if cell.selectedBy == diagnosticParserCoreCellSelectionNone && !descriptor.DispatchSupported() {
			if unsupported := diagnosticParserCoreGenericUnsupportedCellDescriptor(int(cell.headerIndex), cell.dispatchToken(s.token), cell.actions(), descriptor); unsupported != nil {
				return unsupported, nil
			}
		}
		switch cell.kind() {
		case core.ActionRowAccept:
			if acceptCell < 0 {
				acceptCell = index
			}
		case core.ActionRowExtraShift:
			extraCells++
		case core.ActionRowReduce:
			if reductionCell < 0 {
				reductionCell = index
			}
		case core.ActionRowConflict:
			if descriptor.HasReduce() && reductionCell < 0 {
				reductionCell = index
				reductionConflict = true
			}
			if conflictCell < 0 {
				conflictCell = index
			}
		}
	}
	if len(cells) == 0 {
		if !s.recoveryIsolation && diagnosticParserCoreGenericNoActionDropEligible(s.headers, noActionIndices, s.epochProgress) {
			if s.options.recordDropCohortFrontiers {
				if err := s.publishDropCohortFrontierOwned(noActionIndices); err != nil {
					return nil, err
				}
			}
			if err := s.consumeDropCohortFrontierOwned(noActionIndices); err != nil {
				return nil, err
			}
			return nil, s.dropGenericNoActionHeads(noActionIndices)
		}
		if len(noActionIndices) != 0 {
			if s5MissingLineageNeedsOwnedLexer(s, noActionIndices) {
				return s.activateVersionLexerOwnershipAtRagged(noActionIndices[0])
			}
			// pausedNoActionHeads == 0, deferredNoActionHeads == 0, and
			// raggedRelexNoActionHeads == 0 mean every no-action head
			// reached this point through a genuinely empty action row (no
			// table action for the elected token), not through the
			// unrelated group-election pause tracked by header.paused
			// above, not through the issue #983 contextual close-angle
			// deferral tracked by deferredNoActionHeads above, and not
			// through the legacy ragged-span fallback tracked by
			// raggedRelexNoActionHeads above. The close-angle deferral has a
			// real, non-empty action row the header must not take this pass.
			// The legacy ragged-span row is empty too. That
			// emptiness is what triggers the per-header relex in the first
			// place), but a live C stack in this exact state may relex its
			// own version and shift that token instead of entering recovery
			// -- so this shape is not the C recovery-entry shape either,
			// even though its action row looks the same as one. That
			// exact genuinely-empty shape is the error-entry point locked-C
			// production pauses on for recovery (glr.go cPaused: "the stack
			// hit a no-action point"; the real-corpus matrix's 13 recovery-
			// handoff rows trigger here with "the elected token has no
			// table action at end-of-file"). Publish the typed recovery
			// boundary for that shape instead of the generic no-action
			// boundary so census and receipts can tell a recovery handoff
			// apart from an internal election pause. This is a dispatch
			// classification only: every one of these boundaries still
			// declines and falls back to production unchanged (B3 stage S1).
			if pausedNoActionHeads == 0 && deferredNoActionHeads == 0 && raggedRelexNoActionHeads == 0 {
				retired, retireErr := s.retireTrailingRecoveryNoActionLineage(noActionIndices)
				if retireErr != nil {
					return nil, retireErr
				}
				if retired {
					return nil, nil
				}
				// Try each certified recovery competition before standalone S3.
				// This foundation owns only the sole-header, sole-no-action shape.
				// Unmodeled ambiguity falls through to the existing decline.
				if len(s.headers) == 1 && len(noActionIndices) == 1 {
					handled, s5Err := s.s5TryMissingTokenInsertion(noActionIndices[0])
					if s5Err != nil {
						return nil, s5Err
					}
					if handled {
						return nil, nil
					}
					handled, s4Err := s.s4TryStackSummaryRecovery(noActionIndices[0])
					if s4Err != nil {
						return nil, s4Err
					}
					if handled {
						return nil, nil
					}
					handled, eofErr := s.tryRecoverEOFAccept(noActionIndices[0])
					if eofErr != nil {
						return nil, eofErr
					}
					if handled {
						return nil, nil
					}
					handled, s3Err := s.s3TryOpenErrorRegion(noActionIndices[0])
					if s3Err != nil {
						return nil, s3Err
					}
					if handled {
						return nil, nil
					}
				}
				return &diagnosticParserCoreGenericUnsupported{
					boundary:    DiagnosticParserCoreRecovery,
					detail:      diagnosticParserCoreNoTableActionDetail,
					headerIndex: noActionIndices[0],
				}, nil
			}
			if pausedNoActionHeads == 0 && deferredNoActionHeads == 0 {
				// raggedRelexNoActionHeads > 0: at least one no-action head
				// reached this dormant legacy path
				// (relexTokenForState found a genuine, same-start relex
				// whose EndByte differs from the shared election), not a
				// genuinely empty action row and not the close-angle
				// deferral. This exclusion is defence-in-depth, not a load-
				// bearing fence: s3TryOpenErrorRegion needs len(s.headers) ==
				// 1, and a ragged relex needs len(s.headers) > 1 (the guard
				// above), so the two conditions cannot both hold today. The
				// exclusion documents intent and stays safe if either guard
				// changes later, instead of silently letting a ragged-span
				// header reach the S3 resume path, which re-reads the raw
				// table without this decline's own reasoning and could
				// bounce forever. Keep the boundary class Recovery so routing is unchanged;
				// the detail carries the offending relexed token's own
				// symbol and span for census and receipts. Report
				// raggedRelexHeaderIndex, not noActionIndices[0]: when a
				// genuinely-empty no-action head and a ragged-end head share
				// this pass, noActionIndices[0] may name the genuinely-empty
				// head while raggedRelexWitness holds the other head's own
				// relexed token, so headerIndex must track the witness.
				return &diagnosticParserCoreGenericUnsupported{
					boundary:    DiagnosticParserCoreRecovery,
					detail:      diagnosticParserCoreRaggedRelexDeclineDetailFor(raggedRelexWitness, s.token),
					headerIndex: raggedRelexHeaderIndex,
				}, nil
			}
			if pausedNoActionHeads == 0 {
				// deferredNoActionHeads > 0: at least one no-action head
				// reached this point through the issue #983 contextual
				// close-angle deferral, not a genuinely empty action row.
				// Never route a deferred header through s3TryOpenErrorRegion:
				// the S3 resume path re-reads the raw table without the
				// deferral and can bounce forever if the certification gate
				// ever widens beyond html. Keep the boundary class Recovery
				// so routing is unchanged, but give the decline a distinct,
				// accurate detail instead of claiming the row had no action.
				return &diagnosticParserCoreGenericUnsupported{
					boundary:    DiagnosticParserCoreRecovery,
					detail:      diagnosticParserCoreContextualCloseAngleDeferralDetail,
					headerIndex: noActionIndices[0],
				}, nil
			}
			detail := "generic scheduler has only paused heads for the elected token"
			if pausedNoActionHeads != len(noActionIndices) {
				detail = "generic scheduler has only paused or no-action heads for the elected token"
			}
			return &diagnosticParserCoreGenericUnsupported{
				boundary:    DiagnosticParserCoreNoAction,
				detail:      detail,
				headerIndex: noActionIndices[0],
			}, nil
		}
		return &diagnosticParserCoreGenericUnsupported{
			boundary: DiagnosticParserCoreRoute, detail: "generic scheduler has no runnable head", headerIndex: 0,
		}, nil
	}
	if acceptCell >= 0 {
		cell := cells[acceptCell]
		if !recoveryCompetition && !s.options.allowEOFAcceptNoActionSiblings &&
			s.options.allowMetadataEOFAcceptRecovery &&
			len(s.headers) == 2 && len(cells) == 1 && len(noActionIndices) == 1 {
			handled, err := s.applyCompactEOFRecoveryAdmission(before, cell)
			if err != nil {
				return nil, err
			}
			if handled {
				return nil, nil
			}
			if s.eofRecoveryAdmission.active && !s.eofRecoveryAdmission.valid &&
				s.eofRecoveryAdmission.declineReason != "" {
				return &diagnosticParserCoreGenericUnsupported{
					boundary:    DiagnosticParserCoreAccept,
					detail:      s.eofRecoveryAdmission.declineReason,
					headerIndex: int(cell.headerIndex),
				}, nil
			}
		}
		soleAccept := len(s.headers) == 1 && len(cells) == 1 &&
			len(noActionIndices) == 0 && cell.headerIndex == 0
		certifiedAcceptWithDeadSiblings := s.options.allowEOFAcceptNoActionSiblings &&
			len(s.headers) > 1 && len(cells) == 1 &&
			len(noActionIndices) == len(s.headers)-1
		competingRecoveryAccept := recoveryCompetition && s.headers[cell.headerIndex].isRecoveryLineage()
		if !soleAccept && !certifiedAcceptWithDeadSiblings && !competingRecoveryAccept {
			return &diagnosticParserCoreGenericUnsupported{
				boundary: DiagnosticParserCoreAccept, detail: "generic scheduler requires a sole homogeneous accept frontier", headerIndex: int(cell.headerIndex),
			}, nil
		}
		if certifiedAcceptWithDeadSiblings {
			// The G2 EOF-history census observes the complete frontier before
			// acceptance canonicalization removes its no-action siblings. The
			// default build supplies an empty inlined stub.
			s.censusEOFAcceptHistoryFrontier(int(cell.headerIndex), noActionIndices)
		}
		if err := s.applyGenericAccept(before, cell); err != nil {
			return nil, err
		}
		if competingRecoveryAccept {
			return nil, nil
		}
		if !certifiedAcceptWithDeadSiblings {
			return nil, nil
		}
		// Canonicalization preserves the accepted marker. Rebuild the drop list
		// after acceptance so it cannot depend on stale frontier indices.
		noActionIndices = noActionIndices[:0]
		accepted := 0
		for index, header := range s.headers {
			if header.accepted {
				accepted++
				continue
			}
			noActionIndices = append(noActionIndices, index)
		}
		if accepted != 1 || len(noActionIndices)+1 != len(s.headers) {
			return nil, errors.New("parser-core phase zero: certified EOF accept did not preserve one accepted head")
		}
		if s.options.recordDropCohortFrontiers {
			if err := s.publishDropCohortFrontierOwned(noActionIndices); err != nil {
				return nil, err
			}
		}
		if err := s.consumeDropCohortFrontierOwned(noActionIndices); err != nil {
			return nil, err
		}
		return nil, s.dropGenericNoActionHeads(noActionIndices)
	}
	if extraCells != 0 {
		if extraCells != len(cells) || len(cells) != len(s.headers) || len(noActionIndices) != 0 {
			return &diagnosticParserCoreGenericUnsupported{
				boundary: DiagnosticParserCoreExtra, detail: "generic scheduler requires a homogeneous all-runnable extra cohort", headerIndex: int(cells[0].headerIndex),
			}, nil
		}
		for index := 1; index < len(cells); index++ {
			cell := &cells[index]
			if cell.dispatchToken(s.token) != cells[0].dispatchToken(s.token) {
				return &diagnosticParserCoreGenericUnsupported{
					boundary: DiagnosticParserCoreExtra, detail: "generic scheduler extra cohort requires one tokenization", headerIndex: int(cell.headerIndex),
				}, nil
			}
		}
		if unsupported := s.zeroWidthExtraShiftWithoutProgress(cells); unsupported != nil {
			return unsupported, nil
		}
		return nil, s.applyGenericExtraShifts(before, cells)
	}

	// One reduction-bearing cell is applied per pass. This deliberately
	// reclassifies the complete frontier before any shift is allowed.
	if reductionCell >= 0 {
		cell := cells[reductionCell]
		if reductionConflict {
			return nil, s.applyGenericConflict(before, cell)
		}
		return nil, s.applyGenericReduction(before, cell)
	}
	if conflictCell >= 0 {
		return nil, s.applyGenericConflict(before, cells[conflictCell])
	}
	return nil, s.applyGenericShifts(before, cells)
}

// ---------------------------------------------------------------------------
// B3 stage S3: native strategy-2 recovery (error-region absorb and
// condense-resume) over the sole no-action head. See spec.
// compact-recovery-ownership.v1 section 4 and internal/parsercorephase0/
// error_region.go's file doc comment for the mechanism this ports.
// ---------------------------------------------------------------------------

// recoverEOFAcceptAdmitted reports whether this scheduler may attempt the
// dedicated recover_eof route. It never follows from the S3 capability.
func (s *diagnosticParserCoreGenericScheduler) recoverEOFAcceptAdmitted() bool {
	return s != nil && s.options.Recovery && s.options.allowCompactRecoverEOF &&
		s.tokenSource != nil && compactRecoverEOFArtifactConfigured(s.tokenSource.language)
}

// recoverEOFAcceptHeaderTopologyEligible keeps the direct EOF package
// separate from every ambiguity and recovery-lineage path.
func recoverEOFAcceptHeaderTopologyEligible(header diagnosticParserCoreHeader) bool {
	return !header.accepted && !header.shifted && !header.paused &&
		header.recoveryRegion() == nil && !header.isRecoveryLineage() &&
		header.altSet.Len() == 0 && !header.convergedReductionSplit &&
		!header.resurrectionUnproved && !header.blended
}

// recoverEOFAcceptPayloadShapeEligible admits only a materializable terminal
// for the first direct recover_eof package. Fragile reductions stay closed.
func recoverEOFAcceptPayloadShapeEligible(compact *core.Core, payload core.SubtreeID) (core.SubtreeView, bool, error) {
	if compact == nil {
		return core.SubtreeView{}, false, nil
	}
	view, err := compact.Subtree(payload)
	if err != nil {
		return core.SubtreeView{}, false, err
	}
	if view.Extra || view.Missing || !view.Terminal || view.StartByte > view.EndByte {
		return view, false, nil
	}
	materializationView, err := compact.MaterializationView(payload)
	if err != nil {
		return core.SubtreeView{}, false, err
	}
	if materializationView.Fragile {
		return view, false, nil
	}
	return view, true, nil
}

// recoverEOFAcceptPriorityFree proves that the direct EOF package does not
// shadow an earlier C recovery mechanism. The probes are read-only; Core
// mutation starts only after this function returns true.
func (s *diagnosticParserCoreGenericScheduler) recoverEOFAcceptPriorityFree(head core.Head) (bool, error) {
	if s == nil || s.compact == nil || s.tokenSource == nil || s.tokenSource.language == nil {
		return false, nil
	}
	state, position, err := s.compact.Boundary(head)
	if err != nil {
		return false, err
	}
	tokenCount := Symbol(s.tokenSource.language.TokenCount)
	for symbol := Symbol(0); symbol < tokenCount; symbol++ {
		row, rowErr := s.compact.Actions(state, core.Symbol(symbol))
		if rowErr != nil {
			return false, rowErr
		}
		for ordinal := 0; ordinal < row.Len(); ordinal++ {
			if row.At(ordinal).Type == core.ActionReduce {
				return false, nil
			}
		}
	}
	missingOpportunity, missingErr := s.s3MissingTokenOpportunityExists(state)
	if missingErr != nil {
		return false, missingErr
	}
	if missingOpportunity {
		return false, nil
	}
	candidates, summaryErr := s.compact.StackSummaryCandidates(head, cRecoverMaxSummaryDepth)
	if summaryErr != nil {
		return false, summaryErr
	}
	for _, candidate := range candidates {
		if candidate.ByteOffset() >= position {
			continue
		}
		recoverable, recoverableErr := s.compact.StackSummaryCandidateRecoverable(candidate)
		if recoverableErr != nil {
			return false, recoverableErr
		}
		if !recoverable {
			continue
		}
		row, rowErr := s.compact.Actions(candidate.State(), core.Symbol(0))
		if rowErr != nil {
			return false, rowErr
		}
		if row.Len() != 0 {
			return false, nil
		}
	}
	return true, nil
}

func compactRecoverEOFRecoveryWork(s *diagnosticParserCoreGenericScheduler) uint64 {
	if s == nil {
		return math.MaxUint64
	}
	values := [...]uint64{
		s.work.StackSummaryRecoveryForks,
		s.work.RecoveryLineageSelections,
		s.work.RecoveryLineageRetirements,
		s.work.RecoveryAmbiguityForks,
		s.work.RecoveryCondensePasses,
		s.work.RecoveryVersionCapDrops,
		s.work.ReductionPauses,
		s.work.NoActionDrops,
		s.work.PerVersionLexRequests,
		s.work.PerVersionLexRestores,
		s.work.PerVersionLexPublications,
		s.work.PerVersionLexAcceptedRaggedSpans,
		s.work.PerVersionLexViabilityDrops,
		uint64(s.s5MissingInsertions),
		uint64(s.s3ResumeCount),
	}
	var total uint64
	for _, value := range values {
		if math.MaxUint64-total < value {
			return math.MaxUint64
		}
		total += value
	}
	for _, active := range []bool{
		s.s3RegionOpened,
		s.recoveryIsolation,
		s.selectedRecoveryAbsorbLineage,
	} {
		if active {
			if total == math.MaxUint64 {
				return total
			}
			total++
		}
	}
	return total
}

func (s *diagnosticParserCoreGenericScheduler) recoverEOFAcceptArtifactEligible(
	header diagnosticParserCoreHeader,
	view core.SubtreeView,
) (bool, error) {
	if s == nil || s.compact == nil || s.tokenSource == nil || s.tokenSource.language == nil {
		return false, nil
	}
	language := s.tokenSource.language
	receipt := language.CompactRecoverEOFArtifactReceipt
	blob, blobOK := language.GrammarBlobSHA256()
	if !blobOK || receipt.BlobSHA256 == ([32]byte{}) || receipt.BlobSHA256 != blob ||
		!compactRecoverEOFArtifactConfigured(language) || !s.compact.TableIdentityMatches() {
		return false, nil
	}
	state, offset, err := s.compact.Boundary(header.head)
	if err != nil {
		return false, err
	}
	if StateID(state) != receipt.EOFState || offset != receipt.EOFByteOffset ||
		Symbol(view.Symbol) != receipt.TerminalSymbol {
		return false, nil
	}
	work := s.work
	if work.Passes != receipt.Passes || work.Elections != receipt.Elections ||
		work.ActionLookups != receipt.ActionLookups || work.Dispatches != receipt.Dispatches ||
		work.OrdinaryShifts != receipt.OrdinaryShifts ||
		work.OrdinaryCohorts != receipt.OrdinaryCohorts || work.ExtraShifts != receipt.ExtraShifts ||
		work.ExtraCohorts != receipt.ExtraCohorts || work.Reductions != receipt.Reductions ||
		work.Conflicts != receipt.Conflicts || work.ConflictActions != receipt.ConflictActions ||
		work.Forks != receipt.Forks || work.RepetitionFolds != receipt.RepetitionFolds ||
		compactRecoverEOFRecoveryWork(s) != receipt.RecoveryWork ||
		work.NoActionDrops != receipt.NoActionDrops || work.ReductionPauses != receipt.ReductionPauses ||
		work.Accepts != receipt.Accepts || work.Canonicalizations != receipt.Canonicalizations ||
		work.PeakHeaders != receipt.PeakHeaders || work.Overflow {
		return false, nil
	}
	return true, nil
}

// recoverEOFAcceptPayloads validates the sole no-action path before it is
// replaced by a synthetic ERROR root. Keep extra and missing nodes out of
// this focused package. Core receives the original path payloads because EOF
// admission does not reconstruct reductions that the parser did not perform.
func (s *diagnosticParserCoreGenericScheduler) recoverEOFAcceptPayloads(
	index int,
) (core.Head, []core.SubtreeID, uint32, uint32, bool, error) {
	if s == nil || index < 0 || index >= len(s.headers) || len(s.headers) != 1 ||
		s.tokenSource == nil || s.tokenSource.language == nil ||
		s.options.materializationParser == nil || !s.options.materializationContextSet {
		return core.Head{}, nil, 0, 0, false, nil
	}
	if s.token.Symbol != 0 || s.token.StartByte != s.token.EndByte ||
		s.token.Missing || s.token.NoLookahead || s.token.ExternalScannerToken {
		return core.Head{}, nil, 0, 0, false, nil
	}
	header := s.headers[index]
	// Recover-eof admission owns one deterministic derivation only. Any
	// alternative-set, split, resurrection, or blended provenance means the
	// head came through a condensation or ambiguity path that this route does
	// not reproduce. Decline before reading or mutating the compact graph.
	if !recoverEOFAcceptHeaderTopologyEligible(header) {
		return core.Head{}, nil, 0, 0, false, nil
	}
	paths, err := compactDerivationsForAcceptance(s.compact, header.head)
	if err != nil {
		return core.Head{}, nil, 0, 0, false, nil
	}
	if len(paths) != 1 || len(paths[0].Payloads) == 0 ||
		len(paths[0].Payloads) > compactEOFRecoveryAdmissionMaxTopPayloads {
		return core.Head{}, nil, 0, 0, false, nil
	}
	payloads := paths[0].Payloads
	if len(payloads) != 1 {
		return core.Head{}, nil, 0, 0, false, nil
	}
	priorityFree, priorityErr := s.recoverEOFAcceptPriorityFree(header.head)
	if priorityErr != nil {
		return core.Head{}, nil, 0, 0, false, priorityErr
	}
	if !priorityFree {
		return core.Head{}, nil, 0, 0, false, nil
	}
	var firstStart, lastEnd uint32
	var terminalView core.SubtreeView
	for payloadIndex, payload := range payloads {
		view, shapeEligible, viewErr := recoverEOFAcceptPayloadShapeEligible(s.compact, payload)
		if viewErr != nil {
			return core.Head{}, nil, 0, 0, false, viewErr
		}
		terminalView = view
		if !shapeEligible {
			return core.Head{}, nil, 0, 0, false, nil
		}
		if payloadIndex == 0 {
			firstStart = view.StartByte
		} else {
			previous, previousErr := s.compact.Subtree(payloads[payloadIndex-1])
			if previousErr != nil {
				return core.Head{}, nil, 0, 0, false, previousErr
			}
			if view.StartByte < previous.EndByte {
				return core.Head{}, nil, 0, 0, false, nil
			}
		}
		lastEnd = view.EndByte
	}
	source := s.options.materializationSource
	if s.token.StartByte != uint32(len(source)) ||
		uint64(len(source)) > uint64(^uint32(0)) || firstStart > uint32(len(source)) ||
		lastEnd > uint32(len(source)) || firstStart != firstNonTriviaByteStart(source) ||
		lastEnd < firstStart ||
		!parserTailAllowsCleanAcceptance(source, lastEnd, uint32(len(source)), nil,
			s.options.materializationParser.lineContinuationEscapeByte()) {
		return core.Head{}, nil, 0, 0, false, nil
	}
	artifactEligible, artifactErr := s.recoverEOFAcceptArtifactEligible(header, terminalView)
	if artifactErr != nil {
		return core.Head{}, nil, 0, 0, false, artifactErr
	}
	if !artifactEligible {
		return core.Head{}, nil, 0, 0, false, nil
	}
	return header.head, payloads, firstStart, lastEnd, true, nil
}

// tryRecoverEOFAccept publishes one exact recover_eof ERROR root. The Core
// mutation and scheduler counters commit together; unsupported topologies
// return false so the existing S3 and final-decline ladder remains intact.
func (s *diagnosticParserCoreGenericScheduler) tryRecoverEOFAccept(index int) (bool, error) {
	if !s.recoverEOFAcceptAdmitted() {
		return false, nil
	}
	selectedHead, payloads, startByte, endByte, eligible, err := s.recoverEOFAcceptPayloads(index)
	if err != nil || !eligible {
		return false, err
	}
	dispatchesBefore, workBefore, epochProgressBefore := s.dispatches, s.work, s.epochProgress
	if err := s.reserveDispatches(1); err != nil {
		return false, err
	}
	recoveryCost, recoveryCostMemo, costErr := s.recoveryOutputCostFunc()
	if costErr != nil {
		return false, costErr
	}
	defer recoveryCostMemo.Reset()
	var recovered core.Head
	var root core.SubtreeID
	apply := func(owner core.SchedulerTransactionToken) error {
		var applyErr error
		recovered, root, applyErr = s.compact.RecoverEOFAcceptWithCostOwned(
			owner, selectedHead, payloads, startByte, endByte, recoveryCost,
		)
		return applyErr
	}
	var applyErr error
	if s.freshSessionOwner != nil {
		applyErr = apply(*s.freshSessionOwner)
	} else {
		applyErr = s.compact.ApplySchedulerAtomic(apply)
	}
	if applyErr != nil {
		s.dispatches, s.work, s.epochProgress = dispatchesBefore, workBefore, epochProgressBefore
		return false, applyErr
	}
	header := &s.headers[index]
	header.head = recovered
	header.accepted = true
	header.shifted = false
	header.paused = false
	s.acceptedHead = recovered
	s.acceptedPayloads = append(s.acceptedPayloads[:0], root)
	s.acceptedRootFinalization = diagnosticParserCoreFinalizeRecoverEOF
	s.epochProgress = true
	s.work.add(&s.work.Accepts, 1)
	s.work.add(&s.work.RecoverEOFAccepts, 1)
	s.work.add(&s.work.Dispatches, 1)
	return true, nil
}

// s3ErrorRegionAdmitted reports whether native S3 recovery may attempt to
// own a true no-action point for the current parse instead of declining.
// Both the caller-declared operation shape (Recovery) and the certified,
// grammar-blob-keyed capability (allowCompactStrategy2ErrorRegion, set only
// from Language.CompactStrategy2ErrorRegionCertified) must hold -- an
// uncertified grammar, or a caller that never asked for recovery, changes
// nothing here (design section 7: no grammar-name branches, gate on
// certified capability artifacts).
func (s *diagnosticParserCoreGenericScheduler) s3ErrorRegionAdmitted() bool {
	return s.options.Recovery && s.options.allowCompactStrategy2ErrorRegion
}

// s3TokenIsExtraShift reports whether symbol shifts as extra in state 1: the
// compact equivalent of cAbsorbTokenIntoError's own state-1 probe
// (parser_recover_c.go:3769, "if the token shifts as extra in state 1, mark
// it extra so it is not counted in error cost calculations"). Compact's
// generic-scheduler Token carries no Extra bit of its own (unlike the
// internal package's Token, lexer.go), so this reproduces the same table
// lookup production performs instead of trusting an absent field.
func s3TokenIsExtraShift(compact *core.Core, symbol Symbol) (bool, error) {
	row, err := compact.Actions(1, core.Symbol(symbol))
	if err != nil {
		return false, err
	}
	if row.Len() == 0 {
		return false, nil
	}
	last := row.At(row.Len() - 1)
	return last.Type == core.ActionShift && last.Extra, nil
}

// s3RegionResumeAction reports whether state has a genuine dispatchable
// action for lookahead: the compact equivalent of cRecoverDispatchInError's
// leading action-row check, restricted to depth-0 resume (state is always
// exactly the state the region opened at -- never a deeper stack-summary
// entry; scanning deeper is strategy-1 election, out of S3 scope per the
// stage stop rule).
func s3RegionResumeAction(compact *core.Core, state core.StateID, lookahead Symbol) (bool, error) {
	row, err := compact.Actions(state, core.Symbol(lookahead))
	if err != nil {
		return false, err
	}
	return row.Len() > 0, nil
}

// s3ErrorModeRelex re-lexes at startByte using the grammar's error-mode lex
// state (LexModes[0], the most permissive catch-all mode): the compact
// equivalent of cRecoverElectionLookaheadSymbol's own relex
// (parser_recover_c.go:3230-3275). A live C stack sitting in ERROR_STATE
// lexes with this mode, not the mode its pre-error state would use; a
// header holding an open S3 region is that same shape, so probing "would
// resuming work" against the ordinary shared election (s.token, elected
// once per pass for every header alike) is not faithful on its own --
// confirmed necessary: html_erroneous_end_tag/html_log_8 shows the ordinary
// shared election finding an immediately resumable "text" token for 'H'
// where the pinned C oracle's error-mode lex keeps 'H' (and the letters
// after it) inside the same open error run, one byte-run token wider than
// what plain per-state lexing would report. ok is false when this
// grammar has no distinct error-mode lex state (or startByte is past the
// source), in which case the caller falls back to the ordinary shared
// token unmodified -- the same conservative fallback
// cRecoverElectionLookaheadSymbol itself takes.
func (s *diagnosticParserCoreGenericScheduler) s3ErrorModeRelex(startByte uint32) (Token, bool) {
	if s.tokenSource == nil || s.tokenSource.language == nil || s.tokenSource.lexer == nil {
		return Token{}, false
	}
	lang := s.tokenSource.language
	if len(lang.LexModes) == 0 || len(lang.LexStates) == 0 {
		return Token{}, false
	}
	ls := lang.LexModes[0].LexStateIndex()
	if ls == noLookaheadLexState || int(ls) >= len(lang.LexStates) {
		return Token{}, false
	}
	source := s.tokenSource.lexer.source
	if int(startByte) >= len(source) {
		return Token{}, false
	}
	lx := Lexer{
		states:              lang.LexStates,
		asciiTable:          lang.LexAsciiTable(),
		source:              source,
		pos:                 int(startByte),
		immediateTokens:     lang.ImmediateTokens,
		zeroWidthTokens:     lang.ZeroWidthTokens,
		errorRunLexState:    ls,
		hasErrorRunLexState: true,
	}
	relexed := lx.NextWithErrorRuns(ls)
	if relexed.Symbol == 0 && relexed.StartByte == relexed.EndByte {
		return Token{}, false
	}
	if s.options.allowCompactRecoveryErrorModeKeywordCapture &&
		relexed.Symbol == lang.KeywordCaptureToken {
		savedState := s.tokenSource.state
		savedGLRStates := s.tokenSource.glrStates
		s.tokenSource.state = 0
		s.tokenSource.glrStates = nil
		promoted, demoted := s.tokenSource.promoteKeyword(relexed)
		s.tokenSource.state = savedState
		s.tokenSource.glrStates = savedGLRStates
		// C accepts only a keyword that owns the complete capture span. Keep the
		// raw capture token if the shared promoter did not prove that exact
		// result. The caller then reaches its existing disagreement decline.
		if !demoted && promoted.isKeyword && promoted.Symbol != relexed.Symbol &&
			promoted.StartByte == relexed.StartByte && promoted.EndByte == relexed.EndByte &&
			promoted.StartPoint == relexed.StartPoint && promoted.EndPoint == relexed.EndPoint {
			relexed = promoted
		}
	}
	return relexed, true
}

// s3MissingTokenOpportunityExists reports whether a synthetic missing-token
// insertion at state would let the current elected token proceed: the
// compact equivalent of cHandleError step 2's scan (parser_recover_c.go,
// mirroring cTerminalNextState/stateHasLeadingReduceAction). For every
// terminal ms, if state has a genuine shift for ms (Extra tokens keep the
// same state, matching cTerminalNextState) to some other state, and that
// state's leading action for the actual current token is a reduce, a
// missing-token insertion here would let the parse continue -- exactly the
// shape S5 owns. Standalone S3 declines. Paired S5 keeps both versions.
func (s *diagnosticParserCoreGenericScheduler) s3MissingTokenOpportunityExists(state core.StateID) (bool, error) {
	if s.tokenSource == nil || s.tokenSource.language == nil {
		return false, nil
	}
	tokenCount := Symbol(s.tokenSource.language.TokenCount)
	if tokenCount == 0 {
		return false, nil
	}
	for ms := Symbol(1); ms < tokenCount; ms++ {
		row, err := s.compact.Actions(state, core.Symbol(ms))
		if err != nil {
			return false, err
		}
		if row.Len() == 0 {
			continue
		}
		last := row.At(row.Len() - 1)
		if last.Type != core.ActionShift {
			continue
		}
		nextState := core.StateID(last.State)
		if last.Extra {
			nextState = state
		}
		if nextState == 0 || nextState == state {
			continue
		}
		nextRow, err := s.compact.Actions(nextState, core.Symbol(s.token.Symbol))
		if err != nil {
			return false, err
		}
		if nextRow.Len() == 0 {
			continue
		}
		if nextRow.At(0).Type == core.ActionReduce {
			return true, nil
		}
	}
	return false, nil
}

// s3CloseInProgressProductionsMaxSteps bounds the eager reduction closure
// below. Any real single-path closure chain in the certified witness class
// resolves in a handful of steps; a chain this long almost certainly means
// the walk stopped terminating for a reason S3 does not understand, so
// bailing out (a decline, not a guess) is the safe default.
const s3CloseInProgressProductionsMaxSteps = 64

// s3CloseInProgressProductions eagerly reduces head across every terminal
// symbol's action row until either some symbol yields a genuine (non-extra,
// non-repetition) shift/accept/recover action at the resulting state (a real
// dispatchable state -- stop here, nothing more to close) or no symbol
// yields any reduce action at all (a dead end -- also stop, keeping the
// pre-closure head, mirroring C's anyLookahead=true dead-end-stays-in-place
// rule). This is the compact equivalent of cDoAllPotentialReductions's
// "close in-progress productions" step (parser_recover_c.go:2523),
// restricted to the single deterministic path S3 owns: a state offering
// more than one distinct reduce candidate with no shift is genuine ambiguity
// (true strategy-1 territory), and this function reports ok=false rather
// than choosing among candidates.
//
// Two exclusions keep the single dispatchable-action test faithful to C's
// own has_shift_action (adversarial review finding, REQUIRED 2b): extra
// shifts (a comment/whitespace token is shiftable from nearly every state,
// in this grammar and most others, so treating that as "a real dispatchable
// action exists, stop closing" would end the walk almost immediately,
// everywhere, independent of whether the actual error the walk is trying to
// close resolves) and repetition shifts (a self-loop that does not
// represent grammatical progress out of the error) are excluded from
// setting hasShift, exactly as C's has_shift_action excludes them.
//
// A reduce action with ChildCount==0 (a nullable/epsilon production) is a
// real reduce this closure does not know how to fold into its single-path
// walk -- applying it would require reasoning this stage does not own, and
// silently ignoring it (treating the state as if that action did not exist)
// would let the walk report a stale, pre-reduce state as final, exactly the
// missing-token-insertion-detection gap REQUIRED 2a names. Either shape
// forces ok=false: an epsilon reduce is exactly the kind of reduce
// candidate "the single-path closure does not reproduce."
//
// changed reports whether at least one reduction actually ran (so the
// caller knows to adopt the returned head instead of discarding it).
func (s *diagnosticParserCoreGenericScheduler) s3CloseInProgressProductions(head core.Head) (out core.Head, changed bool, ok bool, err error) {
	if s.tokenSource == nil || s.tokenSource.language == nil {
		return head, false, false, nil
	}
	tokenCount := Symbol(s.tokenSource.language.TokenCount)
	if tokenCount == 0 {
		return head, false, false, nil
	}
	current := head
	for steps := 0; steps < s3CloseInProgressProductionsMaxSteps; steps++ {
		state, _, boundaryErr := s.compact.Boundary(current)
		if boundaryErr != nil {
			return core.Head{}, changed, false, boundaryErr
		}
		hasShift := false
		reduceCandidates := 0
		sawUnmodeledReduce := false
		var reduceLookahead Symbol
		var reduceOrdinal int
		var reduceKeySymbol core.Symbol
		var reduceKeyCount uint8
		haveKey := false
		for sym := Symbol(1); sym < tokenCount; sym++ {
			row, actionsErr := s.compact.Actions(state, core.Symbol(sym))
			if actionsErr != nil {
				return core.Head{}, changed, false, actionsErr
			}
			for i := 0; i < row.Len(); i++ {
				act := row.At(i)
				switch act.Type {
				case core.ActionShift:
					if !act.Extra && !act.Repetition {
						hasShift = true
					}
				case core.ActionAccept, core.ActionRecover:
					hasShift = true
				case core.ActionReduce:
					if act.ChildCount == 0 {
						sawUnmodeledReduce = true
						continue
					}
					if haveKey && act.Symbol == reduceKeySymbol && act.ChildCount == reduceKeyCount {
						continue // same production reachable on another symbol: not a new candidate
					}
					reduceCandidates++
					reduceLookahead, reduceOrdinal = sym, i
					reduceKeySymbol, reduceKeyCount, haveKey = act.Symbol, act.ChildCount, true
				}
			}
		}
		if hasShift {
			// A real dispatchable action exists here regardless of what else
			// this state also offers: stop closing, nothing more to do.
			return current, changed, true, nil
		}
		if sawUnmodeledReduce {
			// No real shift to fall back on, and at least one reduce
			// candidate here is a shape this single-path closure cannot
			// safely apply (see doc comment): decline rather than either
			// silently discarding it (the pre-fix bug) or guessing at how
			// to fold it into the walk.
			return current, changed, false, nil
		}
		if reduceCandidates == 0 {
			return current, changed, true, nil
		}
		if reduceCandidates > 1 {
			return current, changed, false, nil
		}
		frontier, reduceErr := s.compact.Reduce(current, core.Symbol(reduceLookahead), reduceOrdinal, core.ForkOrder{})
		if reduceErr != nil {
			return core.Head{}, changed, false, reduceErr
		}
		if len(frontier) != 1 {
			return current, changed, false, nil
		}
		current = frontier[0]
		changed = true
	}
	return current, changed, false, nil
}

// s5MissingTokenAdmitted reports whether the complete recovery competition is
// available. S5 needs the insertion, absorb, and acceptance capabilities.
func (s *diagnosticParserCoreGenericScheduler) s5MissingTokenAdmitted() bool {
	return s.options.Recovery &&
		s.options.allowCompactMissingTokenInsertion &&
		s.options.allowCompactStrategy2ErrorRegion &&
		s.options.allowCompactRecoveryLineageSelection
}

// s4StackSummaryRecoveryAdmitted reports whether the complete ancestor-
// recovery competition is available. S4 needs the summary, absorb, and
// acceptance capabilities.
func (s *diagnosticParserCoreGenericScheduler) s4StackSummaryRecoveryAdmitted() bool {
	return s.options.Recovery &&
		s.options.allowCompactStackSummaryRecovery &&
		s.options.allowCompactStrategy2ErrorRegion &&
		s.options.allowCompactRecoveryLineageSelection
}

// s4TryStackSummaryRecovery forks one no-action head into two versions. The
// original version absorbs the real token through S3. Its copied sibling
// recovers to the first actionable stack-summary entry. This order matches
// C's version list: strategy 1 appends its fork after the absorbing version.
func (s *diagnosticParserCoreGenericScheduler) s4TryStackSummaryRecovery(index int) (handled bool, err error) {
	if !s.s4StackSummaryRecoveryAdmitted() || len(s.headers) != 1 || index != 0 {
		return false, nil
	}
	original := s.headers[index]
	if original.recoveryRegion() != nil || s.token.Missing || s.token.NoLookahead ||
		s.token.Symbol == 0 || s.token.Symbol == errorSymbol {
		return false, nil
	}
	if s.nextSeq == math.MaxUint64 {
		return false, errors.New("parser-core phase zero: recovery fork creation sequence overflow")
	}

	// C closes productions before it records and scans the stack summary.
	// Keep the original header unchanged unless the complete fork succeeds.
	closedHead, closedChanged, closeOK, closeErr := s.s3CloseInProgressProductions(original.head)
	if closeErr != nil {
		return false, closeErr
	}
	if !closeOK {
		return false, nil
	}
	scanHead := original.head
	if closedChanged {
		scanHead = closedHead
	}
	_, position, boundaryErr := s.compact.Boundary(scanHead)
	if boundaryErr != nil {
		return false, boundaryErr
	}

	electionSymbol := s.token.Symbol
	if relexed, relexOK := s.s3ErrorModeRelex(s.token.StartByte); relexOK && relexed.Symbol != errorSymbol {
		electionSymbol = relexed.Symbol
	}
	candidates, summaryErr := s.compact.StackSummaryCandidates(scanHead, cRecoverMaxSummaryDepth)
	if summaryErr != nil {
		return false, summaryErr
	}
	var elected core.StackSummaryCandidate
	found := false
	for candidateIndex, candidate := range candidates {
		if candidateIndex&63 == 0 {
			if pollErr := s.pollStopControl(); pollErr != nil {
				return false, pollErr
			}
		}
		// C skips summary entries at the current group position. This first-fork
		// unit has no older competing version to apply C's better-version gate to.
		if candidate.ByteOffset() == position || candidate.ByteOffset() > position {
			continue
		}
		recoverable, recoverableErr := s.compact.StackSummaryCandidateRecoverable(candidate)
		if recoverableErr != nil {
			return false, recoverableErr
		}
		if !recoverable {
			continue
		}
		row, rowErr := s.compact.Actions(candidate.State(), core.Symbol(electionSymbol))
		if rowErr != nil {
			return false, rowErr
		}
		if row.Len() == 0 {
			continue
		}
		elected = candidate
		found = true
		break
	}
	if !found {
		return false, nil
	}
	recoveryBaseline, baselineOK, baselineErr := s.recoveryNodeBaselineForHead(scanHead)
	if baselineErr != nil {
		return false, baselineErr
	}
	if !baselineOK {
		return false, nil
	}
	recoveryGroup := s.nextSeq

	absorbHeader := original
	absorbHeader.head = scanHead
	absorbHeader.paused = false
	absorbHeader.shifted = false
	s.invalidateVerifierHeaderBinding()
	s.headers[index] = absorbHeader
	recoveredHeader := absorbHeader
	recoveredHeader.creationSeq = s.nextSeq
	restore := func() {
		s.invalidateVerifierHeaderBinding()
		clear(s.headers)
		s.headers = s.headers[:1]
		s.headers[0] = original
	}

	absorbed, absorbErr := s.s3TryOpenErrorRegionWithAlternatives(index, true)
	if absorbErr != nil {
		restore()
		return false, absorbErr
	}
	if !absorbed || s.headers[index].recoveryRegion() == nil || !s.headers[index].shifted {
		restore()
		return false, nil
	}
	recoveryCost, recoveryCostMemo, costErr := s.recoveryOutputCostFunc()
	if costErr != nil {
		restore()
		return false, costErr
	}
	defer recoveryCostMemo.Reset()

	var recoveredHead core.Head
	recover := func(owner core.SchedulerTransactionToken) error {
		var recoverErr error
		recoveredHead, recoverErr = s.compact.RecoverToAncestorStateWithCostOwned(owner, elected, recoveryCost)
		return recoverErr
	}
	if s.freshSessionOwner != nil {
		err = recover(*s.freshSessionOwner)
	} else {
		err = s.compact.ApplySchedulerAtomic(recover)
	}
	if err != nil {
		restore()
		return false, err
	}
	recoveredHeader.head = recoveredHead
	recoveredHeader.closeRecoveryRegion()
	recoveredHeader.shifted = false
	s.headers[index].publishRecoveryCondenseState(recoveryGroup, 0, recoveryBaseline, true)
	recoveredHeader.publishRecoveryCondenseState(0, 0, recoveryBaseline, true)
	s.headers[index].markRecoveryLineage()
	recoveredHeader.markRecoveryLineage()
	s.invalidateVerifierHeaderBinding()
	s.headers = append(s.headers, recoveredHeader)
	s.recoveryIsolation = true
	s.nextSeq++
	s.work.add(&s.work.StackSummaryRecoveryForks, 1)
	return true, nil
}

// s5TryMissingTokenInsertion selects the artifact-certified S5 implementation.
// Other artifacts retain their previously certified bounded recovery path.
func (s *diagnosticParserCoreGenericScheduler) s5TryMissingTokenInsertion(index int) (handled bool, err error) {
	if s.options.allowCompactFaithfulS5Recovery {
		return s.s5TryMissingTokenInsertionFaithful(index)
	}
	return s.s5TryMissingTokenInsertionLegacy(index)
}

// s3TryOpenErrorRegion attempts to open (and immediately begin absorbing
// into) a native S3 error region for the sole no-action header index.
// handled=true means this pass is fully accounted for: either closure alone
// resolved the no-action point (an LALR table gap, not malformed input -- no
// region needed, and the caller redispatches this same pass against the
// closed head) or a region was opened and the current token absorbed.
// handled=false means the caller must fall back to the existing decline path
// unchanged: recovery is not admitted, the shape is not a single deterministic
// path, or absorbing would require the EOF wrap S3 does not own.
func (s *diagnosticParserCoreGenericScheduler) s3TryOpenErrorRegion(index int) (handled bool, err error) {
	return s.s3TryOpenErrorRegionWithAlternatives(index, false)
}

// s3TryOpenErrorRegionWithAlternatives opens an absorb lineage after S4 and
// S5 have either preserved or precisely rejected their alternatives. When
// alternativesResolved is false, standalone S3 keeps its conservative
// missing-token and deeper-summary decline guards.
func (s *diagnosticParserCoreGenericScheduler) s3TryOpenErrorRegionWithAlternatives(index int, alternativesResolved bool) (handled bool, err error) {
	if !s.s3ErrorRegionAdmitted() {
		return false, nil
	}
	header := &s.headers[index]
	if header.recoveryRegion() != nil {
		// Already owned by the per-header advance hook (dispatchPassActive's
		// s3Region branch); that hook declined to widen absorption to EOF.
		// Fall through to the existing decline unchanged.
		return false, nil
	}
	// A no-action point at or before the source's first non-trivia byte is
	// the root-leading-gap shape (finalizeDiagnosticParserCoreAcceptedRootSpan's
	// sibling gate, diagnosticParserCoreReduceChildrenTilingGap's
	// isDerivationRootReduce exemption): one real byte at document start
	// that no node in ANY derivation ever represents, a pre-existing,
	// separately-owned decline path this stage must not intrude on. Every
	// committed html_erroneous_end_tag witness needs at least one real
	// shifted tag before its absorbed byte (structurally, an end-tag error
	// cannot exist with no preceding start tag), so this guard never blocks
	// the certified witness class -- confirmed necessary: without it,
	// TestCompactRouteRootLeadingGapDeclines's html cases ("&0", "&;", "&#",
	// ">0", "&000") get absorbed here instead of reaching the existing,
	// unverified-for-this-shape accepted-root-leading-gap decline.
	if s.tokenSource != nil && s.tokenSource.lexer != nil &&
		s.token.StartByte <= firstNonTriviaByteStart(s.tokenSource.lexer.source) {
		return false, nil
	}
	// Comment-accuracy note (adversarial review, MINOR item a): every decline
	// below this line runs AFTER s3CloseInProgressProductions, which -- when
	// it applies a reduce candidate while walking toward a dispatchable
	// state or a dead end -- has already performed a real reduction,
	// appending new node/subtree records to the compact arena. That ordering
	// is intentional, not an oversight to fix by moving these checks above
	// the closure: s3RegionResumeAction, the error-mode-lex-disagreement
	// check, the EOF check, the missing-token-opportunity check, and the
	// deeper-resume check all need the CLOSED head's own state, not the
	// pre-closure one, to answer their own question correctly, so they
	// cannot run any earlier than this. It is also safe: the compact arena
	// is append-only and immutable once published (core.go's own
	// documentation on this invariant), and every decline path here means
	// the caller falls back to production for the whole parse, not a
	// partial/local rollback -- so nothing downstream ever reads whatever
	// extra records this closure left behind. No dirty state escapes a
	// decline; only tree records that are already immutable, unreferenced
	// by anything the caller ultimately serves, and cheap.
	closedHead, changed, ok, closeErr := s.s3CloseInProgressProductions(header.head)
	if closeErr != nil {
		return false, closeErr
	}
	if !ok {
		return false, nil
	}
	if changed {
		header.head = closedHead
	}
	state, _, boundaryErr := s.compact.Boundary(header.head)
	if boundaryErr != nil {
		return false, boundaryErr
	}
	// REQUIRED 2b (adversarial review): the same error-mode-lex-disagreement
	// guard Hook A runs before every subsequent absorb (dispatchPassActive's
	// s3Region branch doc comment) applies here too, symmetrically, for the
	// very first token a brand-new region would absorb -- a live C stack
	// enters ERROR_STATE (and its permissive lex mode) at exactly this same
	// no-action point, not one absorb later, so the first token deserves the
	// identical disagreement check, not just every token after it.
	if relexed, relexOK := s.s3ErrorModeRelex(s.token.StartByte); relexOK && relexed.Symbol != errorSymbol {
		sharedIsRealContent := s.token.StartByte != s.token.EndByte
		if sharedIsRealContent && (relexed.Symbol != s.token.Symbol || relexed.EndByte != s.token.EndByte) {
			return false, nil
		}
	}
	hasAction, actionErr := s3RegionResumeAction(s.compact, state, Symbol(s.token.Symbol))
	if actionErr != nil {
		return false, actionErr
	}
	if hasAction {
		// Closure alone resolved it: this was never a real error. Let the
		// ordinary dispatch loop redispatch this pass against the closed head.
		return true, nil
	}
	if s.s3RegionOpened {
		return false, &diagnosticParserCoreDecline{
			boundary: DiagnosticParserCoreRecovery,
			detail:   "compact recovery permits one error region per parse",
		}
	}
	// EOF at the very first no-action point, nothing absorbed yet:
	// cRecoverEOFAccept's whole-file wrap is out of S3 scope. No committed
	// html_erroneous_end_tag witness needs it (verified against the pinned C
	// oracle: every native witness resumes before EOF).
	if s.token.Symbol == 0 {
		return false, nil
	}
	// C tries missing-token insertion before strategy 2 absorb. A standalone
	// S3 call must decline when an insertion opportunity exists. The paired S5
	// fork already preserved that alternative and can proceed with absorption.
	// Confirmed necessary: html_erroneous_end_tag/
	// html_log_7 (a dangling start_tag whose next real token is a valid "</"
	// it cannot use) needs a MISSING ">" here; absorbing "</" as an ordinary
	// error-region token instead produced a confirmed wrong tree.
	if !alternativesResolved {
		missingOpportunity, missingErr := s.s3MissingTokenOpportunityExists(state)
		if missingErr != nil {
			return false, missingErr
		}
		if missingOpportunity {
			return false, nil
		}
	}
	// A standalone S3 route declines when a deeper stack-summary resume exists.
	// It cannot model C's strategy-1 election by itself. The paired S5 route
	// retains the strategy-2 version because C keeps that version beside its
	// recovery forks. The artifact gate must prove that the carried versions
	// produce C's selected result.
	if !alternativesResolved {
		deeperResumeExists, deeperErr := s.compact.AncestorStateWithActionExists(header.head, core.Symbol(s.token.Symbol), cRecoverMaxSummaryDepth)
		if deeperErr != nil {
			return false, deeperErr
		}
		if deeperResumeExists {
			return false, nil
		}
	}
	tokenExtra, extraErr := s3TokenIsExtraShift(s.compact, s.token.Symbol)
	if extraErr != nil {
		return false, extraErr
	}
	recoveryBaseline, baselineOK, baselineErr := s.recoveryNodeBaselineForHead(header.head)
	if baselineErr != nil {
		return false, baselineErr
	}
	if !baselineOK {
		return false, nil
	}
	leafID, leafErr := s.compact.ErrorRegionLeaf(core.Symbol(s.token.Symbol), s.token.StartByte, s.token.EndByte, tokenExtra)
	if leafErr != nil {
		return false, leafErr
	}
	header.openRecoveryRegion(&diagnosticParserCoreS3Region{
		state:     state,
		startByte: s.token.StartByte,
		endByte:   s.token.EndByte,
		children:  []core.SubtreeID{leafID},
	})
	header.publishRecoveryCondenseState(
		header.recoveryGroupIdentity(), header.recoveryMissingGroupIdentity(),
		recoveryBaseline, true,
	)
	header.markRecoveryCosted()
	s.s3RegionOpened = true
	header.shifted = true
	return true, nil
}

func (s *diagnosticParserCoreGenericScheduler) zeroWidthExtraShiftWithoutProgress(cells []diagnosticParserCoreGenericCell) *diagnosticParserCoreGenericUnsupported {
	if s.currentElection.ScannerAfter != s.currentElection.ScannerBefore {
		return nil
	}
	for index := range cells {
		cell := &cells[index]
		token := cell.dispatchToken(s.token)
		if token.EndByte != token.StartByte {
			continue
		}
		action := cell.actions().At(0)
		target := action.State
		if target == 0 {
			target = cell.boundary.State()
		}
		if target == cell.boundary.State() &&
			token.EndByte <= cell.boundary.ByteOffset() {
			return &diagnosticParserCoreGenericUnsupported{
				boundary:    DiagnosticParserCoreRoute,
				detail:      "generic scheduler zero-width extra shift has no scanner or parser-state progress",
				headerIndex: int(cell.headerIndex),
			}
		}
	}
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) applyGenericAccept(before []DiagnosticParserCoreHeaderReceipt, cell diagnosticParserCoreGenericCell) (err error) {
	if s.freshSessionOwner != nil {
		return s.applyGenericAcceptOwned(*s.freshSessionOwner, before, cell)
	}
	if err := s.headerRollbackScratch.begin(s.headers); err != nil {
		return err
	}
	dispatchesBefore, workBefore, epochProgressBefore := s.dispatches, s.work, s.epochProgress
	tokenCellBefore := s.tokenCell
	roundsBefore := len(s.receipt.Rounds)
	defer func() {
		s.headerRollbackScratch.finish(&s.headers, err != nil)
		if err == nil {
			return
		}
		s.dispatches, s.work, s.epochProgress = dispatchesBefore, workBefore, epochProgressBefore
		s.tokenCell = tokenCellBefore
		s.receipt.Rounds = s.receipt.Rounds[:roundsBefore]
	}()
	return s.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		return s.applyGenericAcceptOwned(owner, before, cell)
	})
}

func (s *diagnosticParserCoreGenericScheduler) applyGenericAcceptOwned(owner core.SchedulerTransactionToken, before []DiagnosticParserCoreHeaderReceipt, cell diagnosticParserCoreGenericCell) (err error) {
	if cell.actions().Len() != 1 || cell.actions().At(0).Type != core.ActionAccept {
		return errors.New("parser-core phase zero: generic accept requires one accept action")
	}
	token := cell.dispatchToken(s.token)
	if token.Symbol != 0 || token.StartByte != token.EndByte || token.Missing || token.NoLookahead || token.ExternalScannerToken {
		return errors.New("parser-core phase zero: generic accept requires authenticated zero-width EOF")
	}
	if err := s.reserveDispatches(1); err != nil {
		return err
	}
	s.headers[cell.headerIndex].accepted = true
	s.headers[cell.headerIndex].paused = false
	s.epochProgress = true
	s.work.Accepts++
	s.work.Dispatches++
	if err := s.canonicalizeOwned(owner); err != nil {
		return err
	}
	if s.fullReceipts() {
		after, err := diagnosticParserCoreHeaderReceipts(s.compact, s.headers)
		if err != nil {
			return err
		}
		s.receipt.Rounds = append(s.receipt.Rounds, DiagnosticParserCoreDispatchRound{
			Index: len(s.receipt.Rounds), Before: before,
			Actions: []DiagnosticParserCoreRoundAction{{
				HeaderIndex: int(cell.headerIndex), State: StateID(cell.boundary.State()), ByteOffset: cell.boundary.ByteOffset(),
				Ordinal: 0, Action: rootParserCoreAction(cell.actions().At(0)),
			}},
			After: after,
		})
	}
	return nil
}

// collapseToRecoveryWinner keeps only the priced winner, the way C keeps one
// finished_tree and drops the losing versions.
//
// It clears the active frontier and both canonical buffers before it seats the
// winner. A plain reslice would retain each loser's region and reference set.
func (s *diagnosticParserCoreGenericScheduler) collapseToRecoveryWinner(winner int) {
	if winner < 0 || winner >= len(s.headers) {
		return
	}
	s.work.RecoveryLineageSelections++
	other := 1 - winner
	s.selectedRecoveryAbsorbLineage = len(s.headers) == 2 &&
		s.s5MissingInsertions == 1 && other >= 0 && other < len(s.headers) &&
		s.headers[winner].creationSeq < s.headers[other].creationSeq
	winnerHeader := s.headers[winner]
	winnerHeader.clearRecoveryLineage()
	s.invalidateVerifierHeaderBinding()
	clear(s.headers)
	for target := range s.canonicalScratch.headerBuffers {
		buffer := s.canonicalScratch.headerBuffers[target]
		if cap(buffer) != 0 {
			clear(buffer)
		}
	}
	clear(s.canonicalScratch.keys)
	s.canonicalScratch.keys = s.canonicalScratch.keys[:0]
	if s.canonicalScratch.groups != nil {
		clear(s.canonicalScratch.groups)
	}
	s.headers = s.headers[:1]
	s.headers[0] = winnerHeader
	s.recoveryIsolation = false
}

// selectCompetingRecoveryLineage resolves an accepting frontier that carries
// more than one head, by pricing every competitor and returning the index C
// would publish.
//
// resolved is false whenever this route must decline instead, and every one of
// those conditions is a deliberate fail-closed gate rather than an oversight:
//
//   - the capability is not armed;
//   - any competitor is NOT a recovery lineage, so the frontier is ordinary
//     grammar ambiguity and error cost is the wrong question to ask of it;
//   - the language or source needed to price a lineage is unavailable;
//   - pricing refuses a head (an ambiguous head, or one past the derivation
//     enumeration cap);
//   - the selection ladder cannot order the winner against some competitor,
//     which is C's structural comparison and out of this port's scope.
//
// A returned error means something is wrong with the compact state itself; a
// false resolved means the shape is simply not ours to decide.
func (s *diagnosticParserCoreGenericScheduler) selectCompetingRecoveryLineage() (int, bool, error) {
	indices := make([]int, len(s.headers))
	for index := range indices {
		indices[index] = index
	}
	return s.selectCompetingRecoveryLineageIndices(indices)
}

// selectCompetingRecoveryLineageIndices prices the supplied stack versions
// in frontier order. Acceptance passes only finished versions. Direct policy
// tests pass the complete frontier.
func (s *diagnosticParserCoreGenericScheduler) selectCompetingRecoveryLineageIndices(
	indices []int,
) (int, bool, error) {
	if s == nil || !s.options.allowCompactRecoveryLineageSelection {
		return 0, false, nil
	}
	if len(indices) < 2 {
		return 0, false, nil
	}
	prior := -1
	for _, index := range indices {
		if index <= prior || index < 0 || index >= len(s.headers) ||
			!s.headers[index].isRecoveryLineage() {
			return 0, false, nil
		}
		prior = index
	}
	if s.tokenSource == nil || s.tokenSource.language == nil {
		return 0, false, nil
	}
	// Non-EMPTY, not merely non-nil. rowAt answers from this text, so a zero
	// or short source silently drops RecoveryCostPerSkippedLine (30 per line)
	// from every ERROR region. A twenty-line region would lose 600, more than
	// an entire missing insertion costs, and the arbitration inverts.
	source := s.options.materializationSource
	if len(source) == 0 {
		return 0, false, nil
	}
	costSource, err := newDiagnosticParserCoreRecoveryCostSource(s.compact, source)
	if err != nil {
		// A source past uint32 is a shape this route cannot price, not a
		// broken parse. The sole-head path turns the same condition into a
		// counted decline, so match it rather than aborting the whole route.
		return 0, false, nil
	}
	var memo core.RecoveryCostMemo
	defer memo.Reset()
	symbols := diagnosticParserCoreRecoverySymbolPolicy(s.tokenSource.language)
	priced := make([]diagnosticParserCoreLineage, 0, len(indices))
	for _, index := range indices {
		entry, supported, priceErr := s.recoveryCondenseEntry(
			s.headers[index], symbols, costSource, &memo,
		)
		if priceErr != nil || !supported {
			// A pricing failure is a counted decline here. A raw error would
			// classify the same condition differently from sole-head pricing.
			return 0, false, nil
		}
		priced = append(priced, diagnosticParserCoreLineage{
			Head:  s.headers[index].head,
			Cost:  entry.status.Cost,
			Score: int64(entry.status.DynPrec),
		})
	}
	winner, err := diagnosticParserCoreSelectRecoveryLineage(priced)
	if err != nil {
		return 0, false, nil
	}
	return indices[winner], true, nil
}

func (s *diagnosticParserCoreGenericScheduler) completeAcceptance() (err error) {
	metadataAdmission := s != nil && s.eofRecoveryAdmission.active &&
		s.eofRecoveryAdmission.valid &&
		s.eofRecoveryAdmission.state == compactEOFRecoveryAdmissionRecoveryDropped
	defer func() {
		if err != nil && metadataAdmission && s.eofRecoveryAdmission.valid {
			compactEOFRecoveryAdmissionInvalidate(
				&s.eofRecoveryAdmission,
				"EOF recovery admission completion failed",
			)
		}
	}()
	if s.token.Symbol != 0 || s.token.StartByte != s.token.EndByte || s.token.Missing || s.token.NoLookahead || s.token.ExternalScannerToken {
		return s.finish(DiagnosticParserCoreAccept, "generic scheduler accept is not authenticated EOF", 0)
	}
	// Resolve the winner BEFORE collapsing anything, and collapse only after
	// the EOF-recovery-admission validation below has seen the real frontier.
	// That validation treats len(s.headers) == 1 as proof the sanctioned
	// recovery drop already happened, so truncating first would manufacture
	// the very shape it checks for.
	competitionWinner := 0
	competitionResolved := false
	if len(s.headers) != 1 {
		acceptedIndices := make([]int, 0, len(s.headers))
		for index := range s.headers {
			if s.headers[index].accepted {
				acceptedIndices = append(acceptedIndices, index)
			}
		}
		switch len(acceptedIndices) {
		case 0:
			return s.finish(DiagnosticParserCoreAccept, "generic scheduler requires one accepted compact head", 0)
		case 1:
			competitionWinner = acceptedIndices[0]
			competitionResolved = true
		default:
			var selectErr error
			competitionWinner, competitionResolved, selectErr =
				s.selectCompetingRecoveryLineageIndices(acceptedIndices)
			if selectErr != nil {
				return selectErr
			}
			if !competitionResolved {
				return s.finish(DiagnosticParserCoreAccept, "generic scheduler requires one accepted compact head", 0)
			}
		}
	}
	if metadataAdmission {
		if err := s.validateCompactEOFRecoveryAdmission(
			s.options.materializationSource,
			compactEOFRecoveryAdmissionRecoveryDropped,
		); err != nil {
			return err
		}
	}
	if competitionResolved {
		s.collapseToRecoveryWinner(competitionWinner)
	}
	paths, err := compactDerivationsForAcceptance(s.compact, s.headers[0].head)
	if err != nil {
		// Stage D0 instrument (spec.derivation-set-equivalence.v1): a capped
		// enumeration leaves D unknown, which the differential must not read
		// as an empty set. Compiled out of the shipped build; see
		// parsercore_phase0_derivation_set_census_disabled.go.
		s.censusAcceptanceDerivationSetTruncated()
		return err
	}
	selection, err := decideCompactAcceptanceElection(
		s.compact,
		paths,
		compactAcceptanceElectionPolicy{
			allowPrimary: s.options.allowPrimaryAcceptDerivation,
			allowCStructural: s.options.allowCompactAcceptanceStructuralElection &&
				!metadataAdmission && !s.s3RegionOpened && s.s5MissingInsertions == 0,
		},
	)
	if err != nil {
		return err
	}
	if !selection.selected {
		s.censusAcceptanceDerivationSet(paths, core.Derivation{}, false, false)
		return s.finish(DiagnosticParserCoreAccept, "generic scheduler requires one certified accepted derivation", 0)
	}
	path := selection.path
	materialityCertified := selection.materialityCertified
	// The R1 materiality gate remains the fallback for tied frontiers without
	// an exact structural-election capability. A certified primary or a
	// provisional path is safe only when all paths publish the same tree.
	//
	// COST, stated plainly, not special-cased away: Apex's class_literal_alias
	// witness ("Object t = RecordPage.class;", apexA3TiedElectionWitnesses,
	// apex_a3_certification_sweep_test.go) was already in the certified sweep
	// corpus before this gate existed, and on main its positional primary was
	// the C-exact derivation while production diverged (a sanctioned
	// adjudicated exception). This gate cannot tell that apart from a tied
	// election where the primary is wrong: it declines class_literal_alias
	// too, and the route now serves production's C-divergent tree instead of
	// compact's C-exact one -- public output got worse on this one input.
	// This restores the pre-compact result because production already served
	// every input. The C comparator now exists, but Apex still needs its own
	// exact artifact certification. This code does not special-case the witness.
	if len(paths) > 1 && !selection.cStructuralCertified {
		if detail := compactAcceptanceElectionMaterialityDeclineDetail(paths, s.options.materializationContextSet); detail != "" {
			// The cap and context guard keep comparison cost bounded and keep
			// callers without a materialization context fail-closed. Count
			// both outcomes under the material-acceptance-election census
			// mechanism with their distinct detail strings.
			s.censusAcceptanceDerivationSet(paths, path, true, true)
			return s.finish(DiagnosticParserCoreAccept, detail, 0)
		}
		if !compactAcceptanceElectionIsVacuous(
			s.compact, s.options.materializationParser, s.options.materializationSource,
			s.options.materializationForceReplayParseStates, s.options.materializationContextSet,
			s.headers[0].head, paths, path,
		) {
			s.censusAcceptanceDerivationSet(paths, path, true, true)
			return s.finish(DiagnosticParserCoreAccept, compactAcceptanceElectionMaterialDetail, 0)
		}
	}
	// Stage D0 instrument (spec.derivation-set-equivalence.v1): record the
	// accepted derivation set D. Compiled out of the shipped build.
	s.censusAcceptanceDerivationSet(paths, path, true, false)
	if core.Phase0AEnabled {
		if err := core.RecordPhase0ADiagnosticAcceptedRoots(s.compact, path.Payloads); err != nil {
			return err
		}
		capability, err := core.CapturePhase0ASelectionCapability(s.compact, s.headers[0].head)
		if err != nil {
			if !core.Phase0ADiagnosticRunManaged(s.compact) {
				return err
			}
		} else if err := core.ObservePhase0AAcceptedSelection(s.compact, capability); err != nil && !core.Phase0ADiagnosticRunManaged(s.compact) {
			return err
		}
	}
	stats, err := s.compact.Stats(s.headers[0].head)
	if err != nil {
		return err
	}
	var header DiagnosticParserCoreHeaderPathReceipt
	var payloads []uint32
	if s.fullReceipts() {
		headers, err := diagnosticParserCoreHeaderPathReceipts(s.compact, s.headers)
		if err != nil {
			return err
		}
		header = headers[0]
		payloads = make([]uint32, len(path.Payloads))
		for index, payload := range path.Payloads {
			payloads[index] = uint32(payload)
		}
	} else {
		receipt, err := diagnosticParserCoreHeaderReceipt(s.compact, s.headers[0])
		if err != nil {
			return err
		}
		header.Header = receipt
	}
	s.acceptedHead = s.headers[0].head
	s.acceptedPayloads = append(s.acceptedPayloads[:0], path.Payloads...)
	s.receipt.acceptanceBacking = DiagnosticParserCoreGenericAcceptance{
		ElectionIndex: s.electionIndex, Token: s.token, Header: header,
		Payloads: payloads, Score: path.Score, BranchOrder: path.BranchOrder,
		HasBranchOrder: path.HasBranchOrder, MaterialityCertified: materialityCertified,
		StructuralElectionCertified: selection.cStructuralCertified,
		CoreWork:                    s.compact.Work(),
		Accepts:                     s.work.Accepts, Stats: stats, Work: s.work,
	}
	s.receipt.Acceptance = &s.receipt.acceptanceBacking
	if mergeCensusEnabled {
		// Stage M0 instrument (spec.merge-time-election.v1): the compact
		// core's own always-on link-union counters, read from the snapshot
		// this function already took. Removed from the default build by the
		// constant guard.
		compactWork := s.receipt.acceptanceBacking.CoreWork
		mergeCensusRecordCompactLinkUnion(
			compactWork.PredecessorLinkUnionAttempts,
			compactWork.PredecessorLinkUnionDuplicateNoop,
			compactWork.PredecessorLinkUnionPrecedenceReplaced,
			compactWork.PredecessorLinkUnionRecursiveChanged,
			compactWork.PredecessorLinkUnionAlternateAppended,
			compactWork.PredecessorLinkUnionRejected,
			compactWork.PhysicalHeadMergeAttempts,
			compactWork.PhysicalHeadMergeSuccesses,
			compactWork.PhysicalHeadMergeInputLinks,
		)
	}
	s.publishTotals()
	if metadataAdmission {
		if err := compactEOFRecoveryAdmissionTransition(
			&s.eofRecoveryAdmission,
			compactEOFRecoveryAdmissionCompleted,
		); err != nil {
			return err
		}
	}
	return nil
}

// selectCompactAcceptanceDerivation keeps sole derivations unchanged. A
// certified ambiguous frontier may select exactly one primary conflict path.
// A higher-scoring secondary path always keeps the route closed.
func selectCompactAcceptanceDerivation(paths []core.Derivation, allowPrimary bool) (core.Derivation, bool) {
	if len(paths) == 1 {
		return paths[0], true
	}
	if !allowPrimary || len(paths) < 2 {
		return core.Derivation{}, false
	}
	primary := -1
	for index, path := range paths {
		if path.HasBranchOrder {
			continue
		}
		if primary >= 0 {
			return core.Derivation{}, false
		}
		primary = index
	}
	if primary < 0 {
		return core.Derivation{}, false
	}
	for index, path := range paths {
		if index == primary {
			continue
		}
		if !path.HasBranchOrder || path.Score > paths[primary].Score {
			return core.Derivation{}, false
		}
	}
	return paths[primary], true
}

// selectCompactAcceptanceDerivationWithMateriality keeps the certified
// primary rules unchanged. An uncertified multi-derivation frontier receives
// a provisional first path only for the bounded public-tree comparison.
func selectCompactAcceptanceDerivationWithMateriality(paths []core.Derivation, allowPrimary bool) (core.Derivation, bool, bool) {
	path, selected := selectCompactAcceptanceDerivation(paths, allowPrimary)
	if selected || allowPrimary || len(paths) < 2 {
		return path, selected, false
	}
	return paths[0], true, true
}

type compactAcceptanceElectionPolicy struct {
	allowPrimary     bool
	allowCStructural bool
}

type compactAcceptanceElectionDecision struct {
	path                 core.Derivation
	selected             bool
	materialityCertified bool
	cStructuralCertified bool
}

// decideCompactAcceptanceElection keeps the acceptance policy in one place.
// An artifact-certified C election takes precedence. Every unsupported shape
// returns to the existing primary or materiality rules and stays fail-closed.
func decideCompactAcceptanceElection(
	compact *core.Core,
	paths []core.Derivation,
	policy compactAcceptanceElectionPolicy,
) (compactAcceptanceElectionDecision, error) {
	if policy.allowCStructural && len(paths) > 1 {
		path, selected, err := selectCompactAcceptanceDerivationByCOrder(compact, paths)
		if err != nil {
			return compactAcceptanceElectionDecision{}, err
		}
		if selected {
			return compactAcceptanceElectionDecision{
				path: path, selected: true, cStructuralCertified: true,
			}, nil
		}
	}
	path, selected, materialityCertified := selectCompactAcceptanceDerivationWithMateriality(paths, policy.allowPrimary)
	return compactAcceptanceElectionDecision{
		path: path, selected: selected, materialityCertified: materialityCertified,
	}, nil
}

// selectCompactAcceptanceDerivationByCOrder ports the clean-tree portion of
// ts_parser__select_tree. Derivation Score is the compact path's cumulative
// dynamic precedence. CompareCSelectionSubtrees supplies C's final raw-tree
// ordering. A single payload is required because C rebuilds that accepted
// root without changing its symbol or children. Multi-payload acceptance needs
// a separate exact root reconstruction and therefore remains fail-closed.
func selectCompactAcceptanceDerivationByCOrder(
	compact *core.Core,
	paths []core.Derivation,
) (core.Derivation, bool, error) {
	if compact == nil || len(paths) < 2 || len(paths) > compactAcceptanceElectionMaxLiveDerivations {
		return core.Derivation{}, false, nil
	}
	for _, path := range paths {
		if len(path.Payloads) != 1 {
			return core.Derivation{}, false, nil
		}
	}
	winner := 0
	for candidate := 1; candidate < len(paths); candidate++ {
		if paths[candidate].Score > paths[winner].Score {
			winner = candidate
			continue
		}
		if paths[candidate].Score < paths[winner].Score {
			continue
		}
		comparison, err := compact.CompareCSelectionSubtrees(
			paths[winner].Payloads[0],
			paths[candidate].Payloads[0],
		)
		if errors.Is(err, core.ErrCSelectionComparisonBudget) {
			return core.Derivation{}, false, nil
		}
		if err != nil {
			return core.Derivation{}, false, err
		}
		if comparison > 0 {
			winner = candidate
		}
	}
	return paths[winner], true, nil
}

func compactDerivationsForAcceptance(compact *core.Core, head core.Head) ([]core.Derivation, error) {
	paths, err := compact.Derivations(head)
	if errors.Is(err, core.ErrDerivationEnumerationCap) {
		return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreAccept, detail: "accepted derivation enumeration cap"}
	}
	return paths, err
}

// compactAcceptanceElectionMaterialDetail is the counted decline reason FIX
// R1 uses when a tied compact acceptance election is material: more than one
// live derivation exists at the accepted head, and materializing every one of
// them does not prove they all publish the same tree. The compact route has
// no C-faithful tiebreak for this shape, so it declines instead of guessing
// which derivation the locked C runtime would have picked; production still
// serves the input. See admissionCensusClassify (admission_census.go) for the
// counted mechanism class this detail is matched into.
const compactAcceptanceElectionMaterialDetail = "material-acceptance-election: certified primary derivation is not proven byte-identical to every tied secondary derivation"

// compactAcceptanceElectionNoContextDetail is the counted decline reason for
// a tied election reached with no materialization context available: no
// caller in this parse set DiagnosticParserCorePrefixOptions
// .materializationContextSet, so compactAcceptanceElectionIsVacuous never
// ran, and no comparison happened at all. This is deliberately distinct
// wording from compactAcceptanceElectionMaterialDetail -- "not proven
// byte-identical" would overstate what happened when no comparison ran.
// completeAcceptance checks materializationContextSet itself and selects
// this detail before ever calling compactAcceptanceElectionIsVacuous, so
// that function's own internal contextSet check is a second, defensive
// fail-closed guard for any other caller, not the source of this wording.
// Classifies under the same material-acceptance-election census mechanism
// (admissionCensusClassify, admission_census.go) as the other two decline
// details in this family: the public contract is identical across all
// three -- the route could not prove this election vacuous, so it declines
// rather than guess.
const compactAcceptanceElectionNoContextDetail = "material-acceptance-election: materialization context unavailable; election cannot be proven vacuous"

// compactAcceptanceElectionMaxLiveDerivations bounds how many live
// derivations compactAcceptanceElectionIsVacuous will materialize and
// compare. Observed live-derivation counts at a tied accepted head top out
// at 8 (bounded in practice by MaxLinksPerBoundary); core.Limits.MaxDerivations
// itself permits up to 1<<16 exact paths, so an adversarial or pathological
// grammar/input combination could in principle present far more live
// candidates than any witness measured so far. Materializing and comparing
// every one of them is unbounded cost with no known witness anywhere near
// this bound; completeAcceptance declines outright above it instead.
const compactAcceptanceElectionMaxLiveDerivations = 8

// compactAcceptanceElectionCandidateCapDetail is the counted decline reason
// for a tied election whose live-derivation count exceeds
// compactAcceptanceElectionMaxLiveDerivations. It is a distinct string from
// compactAcceptanceElectionMaterialDetail (the gate never ran a comparison
// here) but classifies under the same material-acceptance-election census
// mechanism (admissionCensusClassify, admission_census.go), since both
// outcomes share the same public contract: the route could not prove this
// election vacuous, so it declines rather than guess.
const compactAcceptanceElectionCandidateCapDetail = "material-acceptance-election: tied derivation count exceeds the materiality gate's comparison cap"

// compactAcceptanceElectionMaterialityDeclineDetail returns the preconditions
// that prevent the bounded public-tree comparison from running.
func compactAcceptanceElectionMaterialityDeclineDetail(paths []core.Derivation, contextSet bool) string {
	if len(paths) > compactAcceptanceElectionMaxLiveDerivations {
		return compactAcceptanceElectionCandidateCapDetail
	}
	if !contextSet {
		return compactAcceptanceElectionNoContextDetail
	}
	return ""
}

// compactAcceptanceElectionIsVacuous decides whether every derivation in
// paths materializes to the same public tree as the selected reference.
// The reference may be a certified primary or a deterministic provisional
// path when no primary exists. The caller limits paths to
// compactAcceptanceElectionMaxLiveDerivations before this function runs.
//
// This is NOT a property every multi-derivation accept in the certified
// languages' own sweep corpora already had before this gate landed: Apex's
// class_literal_alias witness (apexA3TiedElectionWitnesses,
// apex_a3_certification_sweep_test.go) was already in that sweep corpus, and
// its two derivations are not byte-identical -- one assigns an "object"
// field the other does not. Before this gate, the route accepted it anyway
// (the positional primary, which happened to be the byte-identical-to-C one
// on this witness); this gate now declines it. See the cost note where
// completeAcceptance calls this function for that specific regression.
//
// forceReplayParseStates must equal the value the caller's own
// post-acceptance materialization will use (DiagnosticParserCorePrefixOptions
// .materializationForceReplayParseStates): it decides whether the
// materialized nodes carry ParseState/PreGotoState stamps at all, and this
// function's contract is to compare what the published tree actually
// carries, not some other configuration. contextSet distinguishes "no caller
// provided a materialization context" (fail closed) from "the caller
// provided one, and the source is legitimately empty". completeAcceptance
// checks contextSet itself before calling this function and picks
// compactAcceptanceElectionNoContextDetail instead of running a call that
// can only fail closed, so the contextSet check inside this function is a
// second, defensive guard for any other caller, not the source of the
// no-context detail text.
//
// A materialization failure on any candidate (a cap, a tiling-gap decline,
// or any other error) is treated as "not proven vacuous", matching the
// fail-closed contract every other compact decline in this file uses: the
// route never guesses.
//
// This redundantly re-materializes primary (once here for comparison, once
// again by the caller's normal post-acceptance materialization). That cost
// lands only on multi-derivation accepts, which are rare; a sole-derivation
// accept (the overwhelming majority) never reaches this function at all.
func compactAcceptanceElectionIsVacuous(
	compact *core.Core, parser *Parser, source []byte,
	forceReplayParseStates bool, contextSet bool,
	head core.Head, paths []core.Derivation, primary core.Derivation,
) bool {
	if len(paths) < 2 {
		return true
	}
	if !contextSet || compact == nil || parser == nil {
		return false
	}
	// allowErrorRoot is always false here (B3 stage S3, materializeDiagnosticParserCoreAcceptedSelection):
	// this gate only runs on a clean, non-recovery tied accept, and a
	// candidate that needs allowErrorRoot=true to materialize is not
	// something this gate is equipped to reason about -- it fails the trial
	// materialization, which the fail-closed contract below already treats
	// as "not proven vacuous". forceReplayParseStates is threaded through
	// (not hardcoded) so the trial materialization matches the caller's own
	// post-acceptance one -- see this function's doc comment.
	primaryTree, err := materializeDiagnosticParserCoreAcceptedSelection(compact, head, primary.Payloads, parser, source, nil, forceReplayParseStates, false)
	if err != nil {
		return false
	}
	defer primaryTree.Release()
	primaryRoot := primaryTree.RootNode()
	for _, candidate := range paths {
		if slices.Equal(candidate.Payloads, primary.Payloads) {
			continue
		}
		candidateTree, err := materializeDiagnosticParserCoreAcceptedSelection(compact, head, candidate.Payloads, parser, source, nil, forceReplayParseStates, false)
		if err != nil {
			return false
		}
		equal := compactAcceptanceDerivationTreesEqual(parser.language, primaryRoot, candidateTree.RootNode())
		candidateTree.Release()
		if !equal {
			return false
		}
	}
	return true
}

// compactAcceptanceDerivationTreesEqual reports whether two materialized
// derivation trees of the same accepted head publish the same PUBLIC tree
// shape: the same node symbol, byte span, named/extra/missing flags, the
// same field assignment per child, and the same children, recursively. This
// is deliberately narrower than "identical in every internal attribute a
// *Node carries" -- see the scope note below. Field assignment matters here
// even though it is invisible in an SExpr dump: a materiality census that
// only compared SExpr text would miss a tied election where both candidates
// share a symbol and shape but assign a field (for example "object") on only
// one side. HasError is not compared separately -- it is a pure function of
// the subtree below a node, so it is already implied once every descendant
// symbol and shape matches.
//
// Scope note (evaluated and rejected: ParseState, PreGotoState, the fragile
// flag, and the external-scanner-token flag). All four are gotreesitter's
// own internal construction bookkeeping, not part of the tree-sitter tree
// shape the C oracle can be compared against -- the C runtime has no
// equivalent of any of them. Adding any of the four to this comparator was
// measured directly against the five R1 target languages' full sweep
// corpora (perl, python, apex, ada, kotlin; certificates forced,
// GTS_ADMISSION_CENSUS=1). Only ParseState/PreGotoState decline a source
// that was otherwise a clean, C-exact accept: Perl push_two_args_real_corpus_
// witness, push_three_args, and Python assignment_bare_tuple_real_corpus_
// witness, assignment_bare_pair, assignment_bare_single_trailing_comma (5
// sources). In every one, the two derivations' PUBLIC trees are already
// byte-identical, so adding either attribute declines a correct accept for
// no benefit. That is the entire measured cost of this scope boundary.
//
// The other two measured effects are not an additional cost, because
// neither source involved was a clean pass to begin with:
//   - ParseState/PreGotoState also decline four already-tracked,
//     C-divergent sources: Perl join_assignment, return_list, and Python
//     fstring_interpolation_bare_tuple, fstring_interpolation_splat.
//     Declining them buys nothing for parity -- production already carries
//     the identical divergence -- but costs nothing new either.
//   - The fragile/external-scanner-token pair declines exactly one source,
//     Ada array_others_choice, which is itself a tracked Family M
//     divergence (adaA3KnownDivergences), not a clean pass. This pair's
//     measured effect is a wash: no fidelity change either way.
//
// None of the four are compared here; the measured, avoidable cost is
// confined to the five ParseState/PreGotoState witnesses named above.
// ParseState/PreGotoState trustworthiness for incremental reuse is guarded
// separately and already, per materialized tree, by
// incrementalReuseProven / Tree.incrementalReuseDisabled
// (materializeDiagnosticParserCoreAcceptedSelection) -- that mechanism does
// not depend on which of two shape-identical derivations was published, so
// this gate does not need to duplicate it.
func compactAcceptanceDerivationTreesEqual(lang *Language, a, b *Node) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Symbol() != b.Symbol() ||
		a.StartByte() != b.StartByte() || a.EndByte() != b.EndByte() ||
		a.IsNamed() != b.IsNamed() || a.IsExtra() != b.IsExtra() || a.IsMissing() != b.IsMissing() {
		return false
	}
	childCount := a.ChildCount()
	if childCount != b.ChildCount() {
		return false
	}
	for i := 0; i < childCount; i++ {
		if a.FieldNameForChild(i, lang) != b.FieldNameForChild(i, lang) {
			return false
		}
		if !compactAcceptanceDerivationTreesEqual(lang, a.Child(i), b.Child(i)) {
			return false
		}
	}
	return true
}

// diagnosticParserCoreGenericNoActionDropEligible reports whether at least one
// non-paused sibling head is still live (shifted or accepted). noActionIndices
// is ascending and unique (dispatch-pass order), so the paused set is matched
// with a two-pointer walk rather than an allocated map.
func diagnosticParserCoreGenericNoActionDropEligible(headers []diagnosticParserCoreHeader, noActionIndices []int, epochProgress bool) bool {
	if !epochProgress || len(noActionIndices) == 0 || len(noActionIndices) >= len(headers) {
		return false
	}
	prev := -1
	for _, index := range noActionIndices {
		if index <= prev || index >= len(headers) {
			return false
		}
		prev = index
	}
	next := 0
	for index := range headers {
		if next < len(noActionIndices) && noActionIndices[next] == index {
			next++
			continue
		}
		if headers[index].shifted || headers[index].accepted {
			return true
		}
	}
	return false
}

func diagnosticParserCoreSelectedLineageDrops(
	headers []diagnosticParserCoreHeader,
	indices []int,
) (uint64, bool) {
	var proved uint64
	for _, index := range indices {
		if index < 0 || index >= len(headers) {
			return 0, false
		}
		dropped := headers[index]
		if !dropped.convergedReductionSplit {
			continue
		}
		if dropped.cleanPathRank != core.CleanPathRankUnselected || dropped.cleanPathLineage == 0 {
			return 0, false
		}
		matched := false
		for survivorIndex, survivor := range headers {
			dropIndex := sort.SearchInts(indices, survivorIndex)
			if dropIndex < len(indices) && indices[dropIndex] == survivorIndex {
				continue
			}
			if survivor.cleanPathRank == core.CleanPathRankSelected &&
				survivor.cleanPathLineage == dropped.cleanPathLineage {
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

// publishDropCohortFrontierOwned records the complete current election before
// the no-action consumer sees its headers. Incomplete producer facts keep the
// existing v2 fallback unchanged.
const diagnosticParserCoreFrontierParticipantCap = 32

func (s *diagnosticParserCoreGenericScheduler) dropCohortFrontierTokenOwned(owner core.SchedulerTransactionToken) (core.DropCohortFrontierToken, bool) {
	if s == nil || s.compact == nil {
		return core.DropCohortFrontierToken{}, false
	}
	beforeLength, beforeDigest, beforeOK := s.compact.CheckpointReceiptOwned(owner, s.checkpointBeforeID)
	afterLength, afterDigest, afterOK := s.compact.CheckpointReceiptOwned(owner, s.checkpointID)
	if !beforeOK || !afterOK || beforeLength != uint32(s.currentElection.ScannerBefore.Length) || afterLength != uint32(s.currentElection.ScannerAfter.Length) {
		return core.DropCohortFrontierToken{}, false
	}
	return core.DropCohortFrontierToken{
		Symbol: core.Symbol(s.token.Symbol), StartByte: s.token.StartByte, EndByte: s.token.EndByte,
		StartRow: s.token.StartPoint.Row, StartColumn: s.token.StartPoint.Column,
		EndRow: s.token.EndPoint.Row, EndColumn: s.token.EndPoint.Column,
		NoLookahead: s.token.NoLookahead, Missing: s.token.Missing,
		ExternalScannerToken: s.token.ExternalScannerToken,
		ScannerBefore:        s.checkpointBeforeID, ScannerAfter: s.checkpointID,
		ScannerBeforeDigest: beforeDigest, ScannerAfterDigest: afterDigest,
	}, true
}

func (s *diagnosticParserCoreGenericScheduler) publishDropCohortFrontierMembersOwned(
	owner core.SchedulerTransactionToken,
	electionSequence uint64,
	heads []core.Head,
	refs []core.DropCohortRefSet,
	dropIndices []int,
) (uint32, bool, error) {
	if s == nil || !s.options.recordDropCohortFrontiers || electionSequence == 0 ||
		len(heads) == 0 || len(heads) > diagnosticParserCoreFrontierParticipantCap || len(heads) != len(refs) {
		return 0, false, nil
	}
	token, ok := s.dropCohortFrontierTokenOwned(owner)
	if !ok {
		return 0, false, nil
	}
	var branchOrders [diagnosticParserCoreFrontierParticipantCap]uint64
	for index := range heads {
		branchOrders[index] = uint64(index)
	}
	handle, complete, err := s.compact.PublishDropCohortFrontierOwned(
		owner, electionSequence, token, heads, branchOrders[:len(heads)], refs,
	)
	if err != nil || !complete {
		return 0, complete, err
	}
	if handle.Sequence > math.MaxUint32 {
		return 0, false, errors.New("parser-core phase zero: frontier sequence does not fit the header")
	}
	if s.observer.frontierPublished != nil {
		if err := s.observer.frontierPublished(s, owner, dropIndices); err != nil {
			return 0, false, err
		}
	}
	return uint32(handle.Sequence), true, nil
}

func (s *diagnosticParserCoreGenericScheduler) publishDropCohortFrontierOwned(dropContext ...[]int) error {
	if s == nil || !s.options.recordDropCohortFrontiers || s.freshSessionOwner == nil ||
		len(s.headers) == 0 || len(s.headers) > diagnosticParserCoreFrontierParticipantCap {
		return nil
	}
	var heads [diagnosticParserCoreFrontierParticipantCap]core.Head
	var refs [diagnosticParserCoreFrontierParticipantCap]core.DropCohortRefSet
	for index, header := range s.headers {
		heads[index], refs[index] = header.head, header.dropCohortRefs
	}
	var dropIndices []int
	if len(dropContext) != 0 {
		dropIndices = dropContext[0]
	}
	sequence, _, err := s.publishDropCohortFrontierMembersOwned(
		*s.freshSessionOwner, uint64(s.electionIndex+1), heads[:len(s.headers)], refs[:len(s.headers)], dropIndices,
	)
	if err != nil {
		return err
	}
	// Replace every current header sequence with this publication result. A
	// zero result must clear stale reduction state before the no-action drop.
	for index := range s.headers {
		s.headers[index].frontierSequence = sequence
	}
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) publishDropCohortFrontierForReductionOwned(
	owner core.SchedulerTransactionToken,
	cohort core.DropCohortHandle,
	outputs []core.ReductionOutput,
) (uint32, error) {
	if len(outputs) < 2 || len(outputs) > diagnosticParserCoreFrontierParticipantCap {
		return 0, nil
	}
	var heads [diagnosticParserCoreFrontierParticipantCap]core.Head
	var refs [diagnosticParserCoreFrontierParticipantCap]core.DropCohortRefSet
	for index, output := range outputs {
		ref, err := s.compact.DropCohortRefForBranchOwned(owner, cohort, uint16(index))
		if err != nil {
			return 0, nil
		}
		heads[index] = output.Head
		refs[index] = core.DropCohortRefSet{Inline: [2]core.DropCohortRef{ref}, Count: 1}
	}
	sequence, complete, err := s.publishDropCohortFrontierMembersOwned(
		owner, uint64(s.electionIndex+1), heads[:len(outputs)], refs[:len(outputs)], nil,
	)
	if err != nil || !complete {
		return 0, err
	}
	return sequence, nil
}

func (s *diagnosticParserCoreGenericScheduler) attachDiagnosticParserCoreFrontierToHead(head core.Head, sequence uint32) {
	if s == nil || !s.options.recordDropCohortFrontiers || sequence == 0 {
		return
	}
	for index := range s.headers {
		if s.headers[index].head == head {
			s.headers[index].frontierSequence = mergeDiagnosticParserCoreFrontier(s.headers[index].frontierSequence, sequence)
		}
	}
}

// consumeDropCohortFrontierOwned authenticates the current frontier before a
// no-action drop mutates the scheduler headers. The private option stays off.
func (s *diagnosticParserCoreGenericScheduler) consumeDropCohortFrontierOwned(indices []int) error {
	if s == nil {
		return nil
	}
	s.frontierProofVerified = false
	s.frontierProofDropCount = 0
	if !s.options.verifyDropCohortFrontiers {
		return nil
	}
	if s.compact == nil || s.freshSessionOwner == nil {
		return errors.New("parser-core phase zero: D6b frontier consumer requires an owned fresh session")
	}
	if len(s.headers) < 2 || len(s.headers) > diagnosticParserCoreFrontierParticipantCap {
		return errors.New("parser-core phase zero: D6b frontier consumer requires a bounded multi-participant frontier")
	}
	sequence := uint32(0)
	var heads [diagnosticParserCoreFrontierParticipantCap]core.Head
	var refs [diagnosticParserCoreFrontierParticipantCap]core.DropCohortRefSet
	for index, header := range s.headers {
		if header.frontierSequence == 0 {
			return errors.New("parser-core phase zero: D6b frontier consumer requires a nonzero frontier sequence")
		}
		if sequence == 0 {
			sequence = header.frontierSequence
		} else if header.frontierSequence != sequence {
			return errors.New("parser-core phase zero: D6b frontier consumer requires one common frontier sequence")
		}
		heads[index] = header.head
		refs[index] = header.dropCohortRefs
	}
	electionSequence := s.electionIndex + 1
	if electionSequence <= 0 {
		return errors.New("parser-core phase zero: D6b frontier consumer has no election sequence")
	}
	owner := *s.freshSessionOwner
	token, ok := s.dropCohortFrontierTokenOwned(owner)
	if !ok {
		return errors.New("parser-core phase zero: D6b frontier consumer could not rebuild the frontier token")
	}
	if len(indices) > len(s.frontierProofDropIndices) {
		return errors.New("parser-core phase zero: D6b frontier consumer drop set exceeds its bounded receipt")
	}
	if err := s.compact.ConsumeDropCohortFrontierSequenceOwned(
		owner,
		uint64(sequence),
		uint64(electionSequence),
		token,
		heads[:len(s.headers)],
		refs[:len(s.headers)],
		indices,
	); err != nil {
		if errors.Is(err, core.ErrDropCohortFrontierNoCommonProof) {
			// D6b declines this authenticated but heterogeneous frontier. Keep
			// the proof flag clear so the existing alternative-set gate runs.
			return nil
		}
		return err
	}
	copy(s.frontierProofDropIndices[:], indices)
	s.frontierProofDropCount = uint8(len(indices))
	s.frontierProofVerified = true
	return nil
}

// dropGenericNoActionHeads removes the paused/no-action heads named by indices.
// indices is produced in ascending, unique header order by the dispatch pass,
// so the surviving frontier is compacted in place with no allocation. The drop
// runs outside any rollback transaction, so mutating the s.headers backing is
// safe.
func (s *diagnosticParserCoreGenericScheduler) dropGenericNoActionHeads(indices []int) error {
	if s.recoveryIsolation {
		return &diagnosticParserCoreDecline{
			boundary: DiagnosticParserCoreNoAction,
			detail:   "recovery competition cannot use an ordinary no-action drop",
		}
	}
	frontierProofVerified := s.frontierProofMatchesDrop(indices)
	defer func() {
		s.frontierProofVerified = false
		s.frontierProofDropCount = 0
	}()
	if len(indices) == 0 || len(indices) >= len(s.headers) {
		return errors.New("parser-core phase zero: sibling-backed no-action drop removed the complete frontier")
	}
	metadataDrop := s.compactEOFRecoveryAdmissionDropMatches(indices)
	versionLexerDrop := s.versionLexerNoActionProof
	// F4 disposition (spec.b4b-alternative-set.v2 section 5): a resurrection
	// descended from a HistoricalBoundaryUnproved dead-node import carries no
	// recorded provenance to prove, so it fails closed independently of the
	// proof below -- waived by the same certified-language artifact escape
	// that waives the proof itself. The detail keeps the "converged-path
	// reduction split" substring every existing fallback-reason assertion
	// keys on (admission_switch_converged_path_test.go,
	// admission_switch_erlang_converged_split_probe_test.go), distinguished
	// by the trailing clause.
	if !metadataDrop && !s.options.allowConvergedSplitDropArtifact {
		for _, index := range indices {
			if index >= 0 && index < len(s.headers) && s.headers[index].resurrectionUnproved {
				return &diagnosticParserCoreDecline{
					boundary: DiagnosticParserCoreNoAction,
					detail:   "converged-path reduction split no-action drop descends from an unproved historical boundary resurrection",
				}
			}
		}
	}
	// Stage 2b (spec.b4b-alternative-set.v2 section 8): the v2 containment
	// predicate -- (event, branch) exact-member containment plus the
	// blended-witness veto -- is the deciding proof for uncertified
	// languages, replacing the scalar (rank, lineage) proof stage 2a kept
	// live while the re-census ran. The section 7 gate passed (zero class-1
	// differing-tree cases, the Kotlin witness declines under v2, stage 1's
	// gates re-passed); erlang's own class-3 probe re-proof miss is scoped
	// to its stage 3 certificate precondition, not this gate.
	convergedCoverageDrops, proved := uint64(0), metadataDrop || versionLexerDrop
	if !metadataDrop && !versionLexerDrop {
		if frontierProofVerified {
			convergedCoverageDrops = s.frontierProofConvergedDropCount(indices)
			proved = true
		} else {
			convergedCoverageDrops, proved = s.diagnosticParserCoreConvergedCoverageDropsV2(indices)
		}
	}
	if !metadataDrop && diagnosticParserCoreShadowCensusEnabled() {
		// The retired scalar proof is evaluated only here, next to the v2
		// decision above, for the ongoing three-proof regression check as
		// more languages decertify in stage 3. Never influences the
		// decision below.
		_, scalarProved := diagnosticParserCoreSelectedLineageDrops(s.headers, indices)
		s.diagnosticParserCoreRunThreeProofCensus(indices, scalarProved)
	}
	if !proved && !s.options.allowConvergedSplitDropArtifact {
		return &diagnosticParserCoreDecline{
			boundary: DiagnosticParserCoreNoAction,
			detail:   "converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor",
		}
	}
	if s.fullReceipts() {
		pathReceipts, err := diagnosticParserCoreHeaderPathReceipts(s.compact, s.headers)
		if err != nil {
			return err
		}
		for _, index := range indices {
			if index < 0 || index >= len(pathReceipts) {
				return errors.New("parser-core phase zero: no-action drop index is out of range")
			}
			token := s.token
			if request := s.versionLexerRequestForHeader(index); request != nil {
				token = request.token
			}
			s.receipt.NoActionDrops = append(s.receipt.NoActionDrops, DiagnosticParserCoreGenericNoActionDrop{
				ElectionIndex: s.electionIndex, Token: token, Header: pathReceipts[index],
			})
		}
	}
	var convergedReductionSplitDrops uint64
	if !versionLexerDrop {
		for _, index := range indices {
			if index >= 0 && index < len(s.headers) && s.headers[index].convergedReductionSplit {
				convergedReductionSplitDrops++
			}
		}
	}
	s.invalidateVerifierHeaderBinding()
	write := 0
	next := 0
	for read := range s.headers {
		if next < len(indices) && indices[next] == read {
			next++
			continue
		}
		if write != read {
			s.headers[write] = s.headers[read]
		}
		write++
	}
	if write == 0 {
		return errors.New("parser-core phase zero: sibling-backed no-action drop removed the complete frontier")
	}
	clear(s.headers[write:])
	s.headers = s.headers[:write]
	s.work.NoActionDrops += uint64(len(indices))
	if versionLexerDrop {
		s.work.add(&s.work.PerVersionLexViabilityDrops, uint64(len(indices)))
	}
	s.work.add(&s.work.ConvergedReductionSplitDrops, convergedReductionSplitDrops)
	s.work.add(&s.work.ConvergedCoverageDrops, convergedCoverageDrops)
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) frontierProofMatchesDrop(indices []int) bool {
	if s == nil || !s.frontierProofVerified || len(indices) != int(s.frontierProofDropCount) {
		return false
	}
	for index, drop := range indices {
		if s.frontierProofDropIndices[index] != drop {
			return false
		}
	}
	return true
}

func (s *diagnosticParserCoreGenericScheduler) frontierProofConvergedDropCount(indices []int) uint64 {
	var count uint64
	for _, index := range indices {
		if index >= 0 && index < len(s.headers) && s.headers[index].convergedReductionSplit {
			count++
		}
	}
	return count
}

// DiagnosticBindDropCohortReferencesForTest binds opaque certificate handles
// to the existing headers. It performs no validation and does not activate the
// production admission route.
func (s *diagnosticParserCoreGenericScheduler) DiagnosticBindDropCohortReferencesForTest(handles [][3]uint64, branches []uint16) error {
	if s == nil || s.compact == nil {
		return errors.New("parser-core phase zero: nil verifier scheduler")
	}
	if len(handles) != len(branches) || len(handles) != len(s.headers) {
		return errors.New("parser-core phase zero: verifier binding length mismatch")
	}
	if len(handles) > len(s.verifierRefs) {
		return errors.New("parser-core phase zero: verifier header cap")
	}
	for index := range s.headers {
		raw := handles[index]
		s.verifierRefs[index] = core.DropCohortRef{
			Owner: raw[0], Epoch: raw[1], Sequence: raw[2], Branch: branches[index],
		}
		s.headers[index].dropCohortRefs = core.DropCohortRefSet{
			Inline: [2]core.DropCohortRef{{
				Owner: raw[0], Epoch: raw[1], Sequence: raw[2], Branch: branches[index],
			}},
			Count: 1,
		}
	}
	s.verifierBound = len(handles)
	if len(s.headers) == 0 {
		s.verifierHeaderPtr = nil
	} else {
		s.verifierHeaderPtr = &s.headers[0]
	}
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) diagnosticDropCohortVerifierInputs() (int, error) {
	if s == nil || s.compact == nil {
		return 0, errors.New("parser-core phase zero: nil verifier scheduler")
	}
	if len(s.headers) > len(s.verifierHeads) {
		return 0, errors.New("parser-core phase zero: verifier header cap")
	}
	if s.verifierBound != len(s.headers) {
		return 0, errors.New("parser-core phase zero: verifier binding is absent")
	}
	if len(s.headers) != 0 && s.verifierHeaderPtr != &s.headers[0] {
		return 0, errors.New("parser-core phase zero: verifier binding is stale")
	}
	for index := range s.headers {
		s.verifierHeads[index] = s.headers[index].head
		bound := s.headers[index].dropCohortRefs
		if bound.Count != 1 || bound.Spilled() || bound.Inline[0] != s.verifierRefs[index] {
			return 0, errors.New("parser-core phase zero: verifier reference binding is stale")
		}
	}
	return len(s.headers), nil
}

// DiagnosticDropGenericNoActionHeadsForTest runs only the inert certificate
// verifier. It never compacts headers or changes the production drop route.
func (s *diagnosticParserCoreGenericScheduler) DiagnosticDropGenericNoActionHeadsForTest(indices []int) (string, error) {
	count, err := s.diagnosticDropCohortVerifierInputs()
	if err != nil {
		return "unknown_cohort", err
	}
	return s.compact.DiagnosticVerifyDropCohortRefsForTest(
		s.verifierHeads[:count], s.verifierRefs[:count], indices,
	)
}

// DiagnosticDropGenericNoActionHeadsNonDestructiveForTest evaluates the same
// certificate without telemetry, header mutation, or arena mutation.
func (s *diagnosticParserCoreGenericScheduler) DiagnosticDropGenericNoActionHeadsNonDestructiveForTest(indices []int) (string, uint64, [32]byte, error) {
	var zero [32]byte
	count, err := s.diagnosticDropCohortVerifierInputs()
	if err != nil {
		return "unknown_cohort", s.verifierInvocation, zero, err
	}
	if s.verifierInvocation != ^uint64(0) {
		s.verifierInvocation++
	}
	reason, verifyErr := s.compact.DiagnosticVerifyDropCohortRefsNonDestructiveForTest(
		s.verifierHeads[:count], s.verifierRefs[:count], indices,
	)
	return reason, s.verifierInvocation, s.compact.DiagnosticDropCohortVerifierStateDigestForTest(), verifyErr
}

func (s *diagnosticParserCoreGenericScheduler) DiagnosticDropGenericNoActionHeadsVerifierStateDigestForTest() [32]byte {
	if s == nil || s.compact == nil {
		return [32]byte{}
	}
	return s.compact.DiagnosticDropCohortVerifierStateDigestForTest()
}

func (s *diagnosticParserCoreGenericScheduler) applyGenericReduction(before []DiagnosticParserCoreHeaderReceipt, cell diagnosticParserCoreGenericCell) (err error) {
	if s.freshSessionOwner != nil {
		return s.applyGenericReductionOwned(*s.freshSessionOwner, before, cell)
	}
	if err := s.headerRollbackScratch.begin(s.headers); err != nil {
		return err
	}
	dispatchesBefore, nextSeqBefore := s.dispatches, s.nextSeq
	nextCleanPathLineageBefore := s.nextCleanPathLineage
	workBefore, epochProgressBefore := s.work, s.epochProgress
	tokenCellBefore := s.tokenCell
	roundsBefore := len(s.receipt.Rounds)
	defer func() {
		s.headerRollbackScratch.finish(&s.headers, err != nil)
		if err == nil {
			return
		}
		s.dispatches, s.nextSeq = dispatchesBefore, nextSeqBefore
		s.nextCleanPathLineage = nextCleanPathLineageBefore
		s.work, s.epochProgress = workBefore, epochProgressBefore
		s.tokenCell = tokenCellBefore
		s.receipt.Rounds = s.receipt.Rounds[:roundsBefore]
	}()
	return s.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		return s.applyGenericReductionOwned(owner, before, cell)
	})
}

// recoveryOutputCostFunc binds the row-aware recovery cost source for one
// scheduler operation. Core receives the complete prefix-plus-payload cost.
func (s *diagnosticParserCoreGenericScheduler) recoveryOutputCostFunc() (core.ReductionOutputCostFunc, *core.RecoveryCostMemo, error) {
	if s == nil || s.compact == nil || s.tokenSource == nil || s.tokenSource.language == nil {
		return nil, nil, errors.New("parser-core phase zero: recovery cost source is unavailable")
	}
	if len(s.options.materializationSource) == 0 {
		return nil, nil, errors.New("parser-core phase zero: recovery cost source requires non-empty materialization source")
	}
	source, err := newDiagnosticParserCoreRecoveryCostSource(s.compact, s.options.materializationSource)
	if err != nil {
		return nil, nil, err
	}
	symbols := diagnosticParserCoreRecoverySymbolPolicy(s.tokenSource.language)
	memo := new(core.RecoveryCostMemo)
	cost := func(prev core.NodeID, payload core.SubtreeID) (uint32, error) {
		prefix, prefixErr := s.compact.RecoveryStoredErrorCost(core.Head{Node: prev})
		if prefixErr != nil {
			return 0, prefixErr
		}
		payloadCost, payloadErr := core.RecoveryNodeErrorCostMemo(symbols, source, memo, payload)
		if payloadErr != nil {
			return 0, payloadErr
		}
		if math.MaxUint32-prefix < payloadCost {
			return 0, errors.New("parser-core phase zero: recovery output cost overflow")
		}
		return prefix + payloadCost, nil
	}
	return cost, memo, nil
}

func (s *diagnosticParserCoreGenericScheduler) applyGenericReductionOwned(owner core.SchedulerTransactionToken, before []DiagnosticParserCoreHeaderReceipt, cell diagnosticParserCoreGenericCell) error {
	header := s.headers[cell.headerIndex]
	recoveryAmbiguitySource := header.isRecoveryLineage()
	storedHeadCost, costErr := s.compact.RecoveryStoredErrorCost(cell.boundary.Head())
	if costErr != nil {
		return costErr
	}
	recoveryCostRequired := recoveryAmbiguitySource || header.recoveryRegion() != nil ||
		header.isRecoveryCosted() || storedHeadCost != 0
	if err := s.reserveDispatches(1); err != nil {
		return err
	}
	var candidates []core.CondenseCandidate
	if len(s.headers) == 1 {
		// The corridor owns only a single header, so this reduction has no live
		// sibling candidates. Preserve the empty scratch slice because the core
		// still needs its live scope and fresh-node boundary.
		s.condenseCandidates = s.condenseCandidates[:0]
		candidates = s.condenseCandidates
	} else {
		candidates = s.collectCondenseCandidates(int(cell.headerIndex))
	}
	ordinal := cell.selectedActionOrdinal()
	if cell.selectsConflictReduction() {
		s.compact.SetReduceConflictContext(true)
		defer s.compact.SetReduceConflictContext(false)
	}
	token := cell.dispatchToken(s.token)
	if token.NoLookahead {
		s.compact.SetReduceNoLookaheadContext(true)
		defer s.compact.SetReduceNoLookaheadContext(false)
	}
	if err := s.compact.SetDropCohortSelectionContextOwned(owner, dropCohortSelectionClass(cell.selectedBy)); err != nil {
		return err
	}
	defer func() {
		_ = s.compact.SetDropCohortSelectionContextOwned(owner, core.DropCohortSelectionNone)
	}()
	var reductionCost core.ReductionOutputCostFunc
	var reductionCostMemo *core.RecoveryCostMemo
	if recoveryCostRequired {
		var costErr error
		reductionCost, reductionCostMemo, costErr = s.recoveryOutputCostFunc()
		if costErr != nil {
			return costErr
		}
		defer reductionCostMemo.Reset()
	}
	var outputs []core.ReductionOutput
	var err error
	if cell.corridorTrustedReduction {
		if reductionCost == nil {
			outputs, err = s.compact.ReduceOutputsCorridorClassifiedIntoWithLiveCondenseCandidatesOwned(
				owner, candidates, s.reductionOutputs, cell.boundary, core.ForkOrder{},
			)
		} else {
			outputs, err = s.compact.ReduceOutputsCorridorClassifiedIntoWithLiveCondenseCandidatesAndCostOwned(
				owner, candidates, s.reductionOutputs, cell.boundary, core.ForkOrder{}, reductionCost,
			)
		}
	} else {
		if reductionCost == nil {
			outputs, err = s.compact.ReduceOutputsClassifiedIntoWithLiveCondenseCandidatesOwned(
				owner, candidates, s.reductionOutputs, cell.boundary, ordinal, core.ForkOrder{},
			)
		} else {
			outputs, err = s.compact.ReduceOutputsClassifiedIntoWithLiveCondenseCandidatesAndCostOwned(
				owner, candidates, s.reductionOutputs, cell.boundary, ordinal, core.ForkOrder{}, reductionCost,
			)
		}
	}
	if err != nil {
		return err
	}
	hasMultiplePopPaths := false
	for _, output := range outputs {
		if output.MultiplePopPaths {
			hasMultiplePopPaths = true
			break
		}
	}
	var reductionLineage uint16
	var reductionFrontierSequence uint32
	if hasMultiplePopPaths {
		reductionLineage, err = nextDiagnosticParserCoreCleanPathLineage(&s.nextCleanPathLineage)
		if err != nil {
			return err
		}
		if err := s.compact.RecordReductionLineageOwned(owner, outputs, reductionLineage); err != nil {
			return err
		}
		if s.options.recordDropCohortCertificates {
			identity := core.DropCohortActionIdentity{
				BoundaryState: core.StateID(cell.boundary.State()),
				Lookahead:     core.Symbol(token.Symbol),
				ActionOrdinal: int32(ordinal),
				Action:        cell.actions().At(ordinal),
				NoLookahead:   token.NoLookahead,
				Selection:     dropCohortSelectionClass(cell.selectedBy),
			}
			cohort, err := s.compact.BeginDropCohortOwned(owner, identity, len(outputs))
			if err != nil {
				return err
			}
			checkpoint := core.DropCohortSourceCheckpoint{
				StartByte:    token.StartByte,
				EndByte:      token.EndByte,
				StartRow:     token.StartPoint.Row,
				StartColumn:  token.StartPoint.Column,
				EndRow:       token.EndPoint.Row,
				EndColumn:    token.EndPoint.Column,
				ScannerStart: s.checkpointBeforeID,
				ScannerEnd:   s.checkpointID,
			}
			certificateSkipped := false
			for outputIndex := range outputs {
				derivation, skipped, buildErr := s.compact.TryBuildDropCohortDerivationOwned(owner, outputs[outputIndex].Head, checkpoint)
				if buildErr != nil {
					return buildErr
				}
				if skipped {
					certificateSkipped = true
					break
				}
				if writeErr := s.compact.WriteDropCohortMemberOwned(
					owner, cohort, outputs[outputIndex].Head, uint16(outputIndex), derivation,
				); writeErr != nil {
					return writeErr
				}
			}
			if certificateSkipped {
				if abandonErr := s.compact.AbandonDropCohortOwned(owner, cohort); abandonErr != nil {
					return abandonErr
				}
			} else {
				if _, err := s.compact.FinalizeDropCohortOwned(owner, cohort); err != nil {
					return err
				}
				if s.options.recordDropCohortFrontiers {
					if sequence, publishErr := s.publishDropCohortFrontierForReductionOwned(owner, cohort, outputs); publishErr != nil {
						return publishErr
					} else {
						reductionFrontierSequence = sequence
					}
				}
				for outputIndex := range outputs {
					ref, refErr := s.compact.DropCohortRefForBranchOwned(owner, cohort, uint16(outputIndex))
					if refErr != nil {
						return refErr
					}
					refs := outputs[outputIndex].DropCohortRefs
					if !s.compact.AddDropCohortRef(&refs, ref) {
						// A bounded reference set can reject publication after the
						// producer cohort is complete. Keep the ordinary reduction
						// output and let the current fallback route continue. The
						// verifier is fail-closed when no complete reference is present.
						continue
					}
					outputs[outputIndex].DropCohortRefs = refs
				}
			}
		}
	}
	s.reductionOutputs = outputs
	s.invalidateVerifierHeaderBinding()
	previousReplacements := s.reductionReplacements
	clear(previousReplacements)
	s.reductionReplacements = previousReplacements[:0]
	defer func() {
		clear(s.reductionReplacements)
		s.reductionReplacements = s.reductionReplacements[:0]
	}()
	replacements := s.reductionReplacements
	madeFreshProgress := false
	for outputIndex, output := range outputs {
		convergedHistory := output.MultiplePopPaths ||
			output.HistoricalBoundaryProvenance == core.HistoricalBoundaryConverged
		// resurrectionUnproved (spec.b4b-alternative-set.v2 section 5, F4
		// disposition): a HistoricalBoundaryUnproved dead-node import no
		// longer contributes to convergedReductionSplit -- it carries no
		// recorded alternative-set members, so containment could never prove
		// it -- but it is still tracked as its own independent, fail-closed
		// veto bit on whichever header inherits it (dropGenericNoActionHeads).
		resurrectionUnproved := output.HistoricalBoundaryProvenance == core.HistoricalBoundaryUnproved
		rank := output.CleanPathRank
		lineage := reductionLineage
		var outputSet core.AlternativeSet
		var outputSetBlended bool
		if output.MultiplePopPaths {
			// Establishment/extend (spec.b4b-alternative-set.v2 section 3.4):
			// the branch is this output's index within outputs, the
			// dispatch's stable first-boundary order -- the identical slice
			// RecordReductionLineageOwned above already iterated, so the
			// ordinals agree with no further synchronization.
			outputSet = core.NewAlternativeSetMember(reductionLineage, uint16(outputIndex))
		}
		if !output.MultiplePopPaths &&
			output.HistoricalBoundaryProvenance == core.HistoricalBoundaryConverged {
			rank = output.HistoricalCleanPathRank
			lineage = output.HistoricalCleanPathLineage
		}
		if output.HistoricalBoundaryProvenance == core.HistoricalBoundaryConverged &&
			output.HistoricalAlternativeSet.Len() != 0 {
			// Union unconditionally, unlike the !MultiplePopPaths-gated scalar
			// override above: historical ancestry is a fact regardless of
			// whether this same reduction also established a fresh split on
			// this output (spec.b4b-alternative-set.v1 section 4). Fold-class
			// union (spec.b4b-alternative-set.v2 section 3.4): this
			// dispatch's own fresh outputSet and the imported dead history
			// are two independently tracked sets.
			incomparable := s.compact.AlternativeSetIncomparable(outputSet, output.HistoricalAlternativeSet)
			s.compact.UnionAlternativeSet(&outputSet, output.HistoricalAlternativeSet)
			outputSetBlended = outputSetBlended || output.HistoricalBlended || incomparable
		}
		outputDropCohortRefs := output.DropCohortRefs
		switch output.Freshness {
		case core.ReductionUnchanged:
			// An unchanged output already names a live physical head. Reuse its
			// existing scheduler slot only when the complete immutable version
			// identity matches. Distinct owned cursors remain fail-closed until
			// the physical-head merge package.
			adopted, err := s.adoptUpdatedReductionSiblingOwned(
				owner,
				int(cell.headerIndex),
				output.Head,
				rank,
				lineage,
				outputSet,
				outputSetBlended,
				outputDropCohortRefs,
				convergedHistory,
				resurrectionUnproved,
				core.DropCohortProducerSiblingAdoption,
			)
			if err != nil {
				return err
			}
			if adopted {
				if reductionFrontierSequence != 0 {
					s.attachDiagnosticParserCoreFrontierToHead(output.Head, reductionFrontierSequence)
				}
				continue
			}
		case core.ReductionNew, core.ReductionUpdated:
		default:
			return errors.New("parser-core phase zero: reduction returned invalid freshness")
		}
		if output.Freshness != core.ReductionUnchanged {
			madeFreshProgress = true
		}
		if output.Freshness == core.ReductionUpdated {
			adopted, err := s.adoptUpdatedReductionSiblingOwned(
				owner,
				int(cell.headerIndex),
				output.Head,
				rank,
				lineage,
				outputSet,
				outputSetBlended,
				outputDropCohortRefs,
				convergedHistory,
				resurrectionUnproved,
				core.DropCohortProducerSiblingAdoption,
			)
			if err != nil {
				return err
			}
			if adopted {
				if reductionFrontierSequence != 0 {
					s.attachDiagnosticParserCoreFrontierToHead(output.Head, reductionFrontierSequence)
				}
				continue
			}
		}
		replacement := s.headers[cell.headerIndex]
		replacement.head = output.Head
		replacement.frontierSequence = mergeDiagnosticParserCoreFrontier(replacement.frontierSequence, reductionFrontierSequence)
		replacement.paused = false
		replacement.shifted = token.NoLookahead
		replacement.convergedReductionSplit = replacement.convergedReductionSplit || convergedHistory
		replacement.resurrectionUnproved = replacement.resurrectionUnproved || resurrectionUnproved
		applyDiagnosticParserCoreCleanPathOutput(&replacement, rank, lineage)
		if outputSet.Len() != 0 {
			// Extend (spec.b4b-alternative-set.v2 section 3.4): this union
			// plants exactly this dispatch's own established (and already
			// fold-classified above) set onto its own uniformly extending
			// derivation thread. It never independently computes
			// incomparability; it only propagates outputSetBlended.
			s.compact.UnionAlternativeSet(&replacement.altSet, outputSet)
			replacement.blended = replacement.blended || outputSetBlended
		}
		if !outputDropCohortRefs.Empty() || outputDropCohortRefs.Overflowed() || outputDropCohortRefs.Blended() {
			if _, err := s.compact.UnionDropCohortRefsChecked(&replacement.dropCohortRefs, outputDropCohortRefs); err != nil {
				return err
			}
		}
		if len(replacements) > 0 {
			if s.nextSeq == math.MaxUint64 {
				return errors.New("parser-core phase zero: reduction creation sequence overflow")
			}
			replacement.creationSeq = s.nextSeq
			s.nextSeq++
		}
		replacements = append(replacements, replacement)
	}
	s.reductionReplacements = replacements
	if len(replacements) == 0 {
		// The canonical outputs already exist and have been processed in this
		// election. Keep this version paused until a sibling makes real progress;
		// the ordinary no-action drop then removes it under the same safety rule.
		supported, baselineErr := s.resetRecoveryNodeBaseline(&s.headers[cell.headerIndex])
		if baselineErr != nil {
			return baselineErr
		}
		if !supported && s.headers[cell.headerIndex].isRecoveryLineage() {
			return &diagnosticParserCoreDecline{
				boundary: DiagnosticParserCoreRecovery,
				detail:   "recovery pause has an ambiguous visible-node count",
			}
		}
		s.headers[cell.headerIndex].paused = true
		s.work.ReductionPauses++
	} else if len(replacements) == 1 {
		s.headers[cell.headerIndex] = replacements[0]
	} else {
		s.headers = replaceDiagnosticParserCoreHeader(s.headers, int(cell.headerIndex), replacements)
	}
	if madeFreshProgress {
		s.epochProgress = true
	}
	if cell.selectsConflictReduction() {
		s.work.add(&s.work.RepetitionFolds, 1)
	}
	if recoveryAmbiguitySource && len(replacements) > 1 {
		s.work.add(&s.work.RecoveryAmbiguityForks, 1)
	}
	s.work.Reductions++
	s.work.Dispatches++
	if err := s.canonicalizeOwned(owner); err != nil {
		return err
	}
	if err := s.persistHeaderLineageOwned(owner); err != nil {
		return err
	}
	if s.fullReceipts() {
		after, err := diagnosticParserCoreHeaderReceipts(s.compact, s.headers)
		if err != nil {
			return err
		}
		s.receipt.Rounds = append(s.receipt.Rounds, DiagnosticParserCoreDispatchRound{
			Index: len(s.receipt.Rounds), Before: before,
			Actions: []DiagnosticParserCoreRoundAction{{
				HeaderIndex: int(cell.headerIndex), State: StateID(cell.boundary.State()), ByteOffset: cell.boundary.ByteOffset(),
				Ordinal: ordinal, Action: rootParserCoreAction(cell.actions().At(ordinal)),
			}},
			After: after,
		})
	}
	if token.NoLookahead &&
		Symbol(cell.actions().At(ordinal).Symbol) == s.options.noLookaheadRootSymbol {
		s.requireEOFPostNoLookaheadRoot = true
	}
	return nil
}

// reindexCondenseCandidatesOwned retains only active sibling versions.
// Tree-sitter C does not merge a new reduction into its source version.
// Stage 2 also requires a never-recovered lineage. This makes every supplied
// ErrorCost zero an authenticated fact instead of a struct default.
func (s *diagnosticParserCoreGenericScheduler) reindexCondenseCandidatesOwned(owner core.SchedulerTransactionToken, source int) error {
	return s.compact.ReindexCondenseCandidatesOwned(owner, s.collectCondenseCandidates(source))
}

func (s *diagnosticParserCoreGenericScheduler) collectCondenseCandidates(source int) []core.CondenseCandidate {
	candidates := s.condenseCandidates[:0]
	if source >= 0 && source < len(s.headers) {
		sourceHeader := &s.headers[source]
		if sourceHeader.accepted || sourceHeader.paused ||
			sourceHeader.isRecoveryLineage() || sourceHeader.recoveryRegion() != nil ||
			sourceHeader.isRecoveryCosted() {
			s.condenseCandidates = candidates
			return candidates
		}
	}
	if !s.recoveryIsolation {
		for index, header := range s.headers {
			if index == source || header.accepted || header.paused ||
				header.isRecoveryLineage() || header.recoveryRegion() != nil ||
				header.isRecoveryCosted() {
				continue
			}
			candidates = append(candidates, core.CondenseCandidate{
				Head: header.head, DropCohortRefs: header.dropCohortRefs,
				Shifted: header.shifted, Checkpoint: header.checkpoint, ErrorCost: 0,
				MergeIdentity: s.condenseCandidateMergeIdentity(index),
			})
		}
		s.condenseCandidates = candidates
		return candidates
	}
	for index, header := range s.headers {
		if index == source || header.accepted || header.paused {
			continue
		}
		// Marked recovery versions must remain separate until acceptance can
		// price them. Ordinary unmarked versions retain normal condensation.
		if header.isRecoveryLineage() || header.recoveryRegion() != nil || header.isRecoveryCosted() {
			continue
		}
		candidates = append(candidates, core.CondenseCandidate{
			Head: header.head, DropCohortRefs: header.dropCohortRefs,
			Shifted: header.shifted, Checkpoint: header.checkpoint, ErrorCost: 0,
			MergeIdentity: s.condenseCandidateMergeIdentity(index),
		})
	}
	s.condenseCandidates = candidates
	return candidates
}

// condenseCandidateMergeIdentity partitions exact immutable lexer ownership
// without removing a candidate from reduction freshness classification.
func (s *diagnosticParserCoreGenericScheduler) condenseCandidateMergeIdentity(index int) uint16 {
	if s == nil || index < 0 || index >= len(s.headers) {
		return 0
	}
	state := s.headers[index].versionState
	if state == nil {
		return 0
	}
	for prior := 0; prior < index; prior++ {
		if s.versionLexerStateEqual(s.headers[prior].versionState, state) {
			return uint16(prior + 1)
		}
	}
	return uint16(index + 1)
}

// adoptUpdatedReductionSibling updates an already-active canonical sibling in
// place. The sibling keeps its scheduler slot and creation sequence; a paused
// copy becomes runnable because the canonical boundary materially changed.
func (s *diagnosticParserCoreGenericScheduler) adoptUpdatedReductionSibling(
	source int,
	head core.Head,
	rank core.CleanPathRankSelection,
	lineage uint16,
	set core.AlternativeSet,
	setBlended bool,
	compat ...interface{},
) (bool, error) {
	if source < 0 || source >= len(s.headers) {
		return false, errors.New("parser-core phase zero: sibling adoption source is out of range")
	}
	sourceFrontier := s.headers[source].frontierSequence
	sourceVersionState := s.headers[source].versionState
	var dropCohortRefs core.DropCohortRefSet
	var convergedReductionSplit, resurrectionUnproved bool
	switch len(compat) {
	case 2:
		var ok bool
		convergedReductionSplit, ok = compat[0].(bool)
		if !ok {
			return false, errors.New("parser-core phase zero: invalid sibling compatibility arguments")
		}
		resurrectionUnproved, ok = compat[1].(bool)
		if !ok {
			return false, errors.New("parser-core phase zero: invalid sibling compatibility arguments")
		}
	case 3:
		var ok bool
		dropCohortRefs, ok = compat[0].(core.DropCohortRefSet)
		if !ok {
			return false, errors.New("parser-core phase zero: invalid sibling reference set")
		}
		convergedReductionSplit, ok = compat[1].(bool)
		if !ok {
			return false, errors.New("parser-core phase zero: invalid sibling compatibility arguments")
		}
		resurrectionUnproved, ok = compat[2].(bool)
		if !ok {
			return false, errors.New("parser-core phase zero: invalid sibling compatibility arguments")
		}
	default:
		return false, errors.New("parser-core phase zero: invalid sibling compatibility arity")
	}
	for index := range s.headers {
		if index == source {
			continue
		}
		if s.recoveryIsolation &&
			(s.headers[source].isRecoveryLineage() || s.headers[index].isRecoveryLineage()) {
			continue
		}
		header := s.headers[index]
		if !s.versionLexerStateEqual(header.versionState, sourceVersionState) {
			// A canonical head does not prove equal version history. Keep forks
			// with distinct immutable state as separate versions.
			continue
		}
		state, byteOffset, err := s.compact.Boundary(header.head)
		if err != nil {
			return false, err
		}
		canonical, ok := s.compact.CanonicalBoundary(state, byteOffset, header.shifted, header.checkpoint)
		if !ok || canonical != head {
			continue
		}
		s.headers[index].head = head
		s.headers[index].paused = false
		s.headers[index].convergedReductionSplit =
			s.headers[index].convergedReductionSplit || convergedReductionSplit
		s.headers[index].resurrectionUnproved =
			s.headers[index].resurrectionUnproved || resurrectionUnproved
		s.headers[index].frontierSequence = mergeDiagnosticParserCoreFrontier(
			s.headers[index].frontierSequence,
			sourceFrontier,
		)
		applyDiagnosticParserCoreCleanPathOutput(&s.headers[index], rank, lineage)
		dst := &s.headers[index]
		if set.Len() != 0 {
			// Fold-class union (spec.b4b-alternative-set.v2 section 3.4):
			// index is a genuinely different, independently tracked header
			// from source -- this is a joint-resolution merge, not a single
			// thread's own uniform extension.
			incomparable := s.compact.AlternativeSetIncomparable(dst.altSet, set)
			s.compact.UnionAlternativeSet(&dst.altSet, set)
			dst.blended = dst.blended || setBlended || incomparable
		}
		if !dropCohortRefs.Empty() || dropCohortRefs.Overflowed() || dropCohortRefs.Blended() {
			if _, err := s.compact.UnionDropCohortRefsChecked(&dst.dropCohortRefs, dropCohortRefs); err != nil {
				return false, err
			}
		}
		return true, nil
	}
	return false, nil
}

func (s *diagnosticParserCoreGenericScheduler) adoptUpdatedReductionSiblingOwned(
	owner core.SchedulerTransactionToken,
	source int,
	head core.Head,
	rank core.CleanPathRankSelection,
	lineage uint16,
	set core.AlternativeSet,
	setBlended bool,
	dropCohortRefs core.DropCohortRefSet,
	convergedReductionSplit bool,
	resurrectionUnproved bool,
	mutation core.DropCohortProducerMutation,
) (bool, error) {
	adopted, err := s.adoptUpdatedReductionSibling(
		source, head, rank, lineage, set, setBlended, dropCohortRefs,
		convergedReductionSplit, resurrectionUnproved,
	)
	if err != nil || !adopted {
		return adopted, err
	}
	if err := s.compact.RecordDropCohortProducerMutation(owner, mutation); err != nil {
		return false, err
	}
	return true, nil
}

func (s *diagnosticParserCoreGenericScheduler) reconcileGenericConflictOutputs(first interface{}, compat ...interface{}) ([]diagnosticParserCoreHeader, int, error) {
	var owner core.SchedulerTransactionToken
	var source int
	var outputs []diagnosticParserCoreHeader
	switch value := first.(type) {
	case core.SchedulerTransactionToken:
		if len(compat) != 2 {
			return nil, 0, errors.New("parser-core phase zero: invalid conflict compatibility arity")
		}
		var ok bool
		source, ok = compat[0].(int)
		if !ok {
			return nil, 0, errors.New("parser-core phase zero: invalid conflict source")
		}
		outputs, ok = compat[1].([]diagnosticParserCoreHeader)
		if !ok {
			return nil, 0, errors.New("parser-core phase zero: invalid conflict outputs")
		}
		owner = value
	case int:
		if len(compat) != 1 {
			return nil, 0, errors.New("parser-core phase zero: invalid legacy conflict compatibility arity")
		}
		var ok bool
		outputs, ok = compat[0].([]diagnosticParserCoreHeader)
		if !ok {
			return nil, 0, errors.New("parser-core phase zero: invalid legacy conflict outputs")
		}
		source = value
	default:
		return nil, 0, errors.New("parser-core phase zero: invalid conflict owner")
	}
	if owner == (core.SchedulerTransactionToken{}) {
		var kept []diagnosticParserCoreHeader
		var adopted int
		err := s.compact.ApplySchedulerAtomic(func(token core.SchedulerTransactionToken) error {
			var innerErr error
			kept, adopted, innerErr = s.reconcileGenericConflictOutputs(token, source, outputs)
			return innerErr
		})
		return kept, adopted, err
	}
	kept := outputs[:0]
	adopted := 0
	for _, output := range outputs {
		if output.freshness == core.ReductionUpdated || output.freshness == core.ReductionUnchanged {
			// The conflict-arm path's diagnosticParserCoreActionOutput never
			// carries HistoricalBoundaryProvenance (applyParserCoreConflictActionInto
			// derives convergedReductionSplit from cleanPathLineage != 0
			// alone, pre-dating and orthogonal to the F4 resurrection
			// signal), so there is no resurrectionUnproved bit to thread
			// here.
			ok, err := s.adoptUpdatedReductionSiblingOwned(
				owner,
				source,
				output.head,
				output.cleanPathRank,
				output.cleanPathLineage,
				output.altSet,
				output.blended,
				output.dropCohortRefs,
				output.cleanPathLineage != 0,
				false,
				core.DropCohortProducerConflictReconciliation,
			)
			if err != nil {
				return nil, 0, err
			}
			if ok {
				adopted++
				continue
			}
		}
		kept = append(kept, output)
	}
	return kept, adopted, nil
}

func (s *diagnosticParserCoreGenericScheduler) reconcileGenericConflictOutputsOwnedWithMutation(
	owner core.SchedulerTransactionToken,
	source int,
	outputs []diagnosticParserCoreHeader,
	mutation core.DropCohortProducerMutation,
) ([]diagnosticParserCoreHeader, int, error) {
	if mutation != core.DropCohortProducerConflictReconciliation {
		return nil, 0, errors.New("parser-core phase zero: invalid conflict-reconciliation mutation class")
	}
	return s.reconcileGenericConflictOutputs(owner, source, outputs)
}

func (s *diagnosticParserCoreGenericScheduler) applyGenericConflict(before []DiagnosticParserCoreHeaderReceipt, cell diagnosticParserCoreGenericCell) (err error) {
	if s.freshSessionOwner != nil {
		return s.applyGenericConflictOwned(*s.freshSessionOwner, before, cell)
	}
	if err := s.headerRollbackScratch.begin(s.headers); err != nil {
		return err
	}
	dispatchesBefore, branchOrderBefore, nextSeqBefore := s.dispatches, s.branchOrder, s.nextSeq
	nextCleanPathLineageBefore := s.nextCleanPathLineage
	workBefore, epochProgressBefore := s.work, s.epochProgress
	tokenCellBefore := s.tokenCell
	roundsBefore, conflictsBefore := len(s.receipt.Rounds), len(s.receipt.Conflicts)
	externalShiftsBefore := len(s.receipt.ExternalShifts)
	defer func() {
		s.headerRollbackScratch.finish(&s.headers, err != nil)
		if err == nil {
			return
		}
		s.dispatches, s.branchOrder, s.nextSeq = dispatchesBefore, branchOrderBefore, nextSeqBefore
		s.nextCleanPathLineage = nextCleanPathLineageBefore
		s.work, s.epochProgress = workBefore, epochProgressBefore
		s.tokenCell = tokenCellBefore
		s.receipt.Rounds = s.receipt.Rounds[:roundsBefore]
		s.receipt.Conflicts = s.receipt.Conflicts[:conflictsBefore]
		s.receipt.ExternalShifts = s.receipt.ExternalShifts[:externalShiftsBefore]
	}()
	return s.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		return s.applyGenericConflictOwned(owner, before, cell)
	})
}

func (s *diagnosticParserCoreGenericScheduler) applyGenericConflictOwned(owner core.SchedulerTransactionToken, before []DiagnosticParserCoreHeaderReceipt, cell diagnosticParserCoreGenericCell) (err error) {
	branchOrderBefore, nextSeqBefore := s.branchOrder, s.nextSeq
	header := s.headers[cell.headerIndex]
	recoveryAmbiguitySource := header.isRecoveryLineage()
	storedHeadCost, costErr := s.compact.RecoveryStoredErrorCost(cell.boundary.Head())
	if costErr != nil {
		return costErr
	}
	recoveryCostRequired := recoveryAmbiguitySource || header.recoveryRegion() != nil ||
		header.isRecoveryCosted() || storedHeadCost != 0
	var reductionCost core.ReductionOutputCostFunc
	var reductionCostMemo *core.RecoveryCostMemo
	if recoveryCostRequired {
		reductionCost, reductionCostMemo, costErr = s.recoveryOutputCostFunc()
		if costErr != nil {
			return costErr
		}
		defer reductionCostMemo.Reset()
	}
	token := cell.dispatchToken(s.token)
	scannerBefore, scannerAfter := s.currentElection.ScannerBefore, s.currentElection.ScannerAfter
	affectedHead := cell.boundary.Head()
	var versionLexerRequest *diagnosticParserCoreVersionLexerRequest
	if cell.versionLexerRequest != 0 {
		versionLexerRequest, err = s.versionLexerRequestForCell(cell)
		if err != nil {
			return err
		}
	}
	if err = s.reserveDispatches(1); err != nil {
		return err
	}
	if err = s.reindexCondenseCandidatesOwned(owner, int(cell.headerIndex)); err != nil {
		return err
	}
	externalStatsBefore, err := s.genericExternalStats(affectedHead, token)
	if err != nil {
		return err
	}
	actions := cell.actions()
	if err := s.conflictScratch.begin(actions.Len()); err != nil {
		return err
	}
	defer s.conflictScratch.finish()
	execution, err := executeDiagnosticParserCoreGenericConflictDetailed(
		s.compact, owner, s.headers[cell.headerIndex], int(cell.headerIndex), token, cell.boundary,
		s.branchOrder, &s.nextCleanPathLineage,
		s.options.captureLexerSkippedPrefixProvenance,
		reductionCost,
		cell.selectedBy == diagnosticParserCoreCellSelectionRepetitionFork,
		s.fullReceipts(), &s.conflictScratch,
	)
	if err != nil {
		return err
	}
	for ordinal := range execution.armRanges {
		for _, output := range execution.arm(ordinal) {
			if output.shifted {
				affectedHead = output.head
				break
			}
		}
		if affectedHead != cell.boundary.Head() {
			break
		}
	}
	if s.conflictPostExecutionFault != nil {
		if err := s.conflictPostExecutionFault(); err != nil {
			return err
		}
	}
	for ordinal := range execution.armRanges {
		arm := execution.arm(ordinal)
		kept, adopted, reconcileErr := s.reconcileGenericConflictOutputs(owner, int(cell.headerIndex), arm)
		if reconcileErr != nil {
			return reconcileErr
		}
		execution.armRanges[ordinal].end = execution.armRanges[ordinal].start + len(kept)
		s.conflictScratch.adopted[ordinal] = adopted
	}
	trialSeq := nextSeqBefore
	for ordinal := 1; ordinal < len(execution.armRanges); ordinal++ {
		arm := execution.arm(ordinal)
		for output := range arm {
			if trialSeq == math.MaxUint64 {
				return errors.New("parser-core phase zero: conflict creation sequence overflow")
			}
			arm[output].creationSeq = trialSeq
			trialSeq++
		}
	}
	primaries := execution.arm(0)
	if len(primaries) != 0 {
		primaries[0].creationSeq = s.headers[cell.headerIndex].creationSeq
		for index := 1; index < len(primaries); index++ {
			if trialSeq == math.MaxUint64 {
				return errors.New("parser-core phase zero: conflict creation sequence overflow")
			}
			primaries[index].creationSeq = trialSeq
			trialSeq++
		}
	}
	execution.nextSeq = trialSeq
	prefix := s.headers[:cell.headerIndex]
	suffix := s.headers[cell.headerIndex+1:]
	outputCount := 0
	for ordinal := range execution.armRanges {
		outputCount += len(execution.arm(ordinal))
	}
	assemblySize := outputCount + len(prefix) + len(suffix)
	if outputCount == 0 {
		assemblySize++
	}
	if cap(s.conflictScratch.headerAssembly) < assemblySize {
		s.conflictScratch.headerAssembly = make([]diagnosticParserCoreHeader, 0, assemblySize)
	} else {
		s.conflictScratch.headerAssembly = s.conflictScratch.headerAssembly[:0]
	}
	headers := s.conflictScratch.headerAssembly
	headers = append(headers, prefix...)
	if len(primaries) != 0 {
		headers = append(headers, primaries[0])
	}
	headers = append(headers, suffix...)
	for ordinal := 1; ordinal < len(execution.armRanges); ordinal++ {
		headers = append(headers, execution.arm(ordinal)...)
	}
	if len(primaries) > 1 {
		headers = append(headers, primaries[1:]...)
	}
	if outputCount == 0 {
		paused := s.headers[cell.headerIndex]
		supported, baselineErr := s.resetRecoveryNodeBaseline(&paused)
		if baselineErr != nil {
			return baselineErr
		}
		if !supported && paused.isRecoveryLineage() {
			return &diagnosticParserCoreDecline{
				boundary: DiagnosticParserCoreRecovery,
				detail:   "recovery conflict pause has an ambiguous visible-node count",
			}
		}
		paused.paused = true
		headers = headers[:len(prefix)]
		headers = append(headers, paused)
		headers = append(headers, suffix...)
	}
	s.conflictScratch.headerAssembly = headers
	s.invalidateVerifierHeaderBinding()
	s.headers = headers
	s.branchOrder, s.nextSeq = execution.branchOrder, execution.nextSeq
	adoptedCount := 0
	for _, count := range s.conflictScratch.adopted {
		adoptedCount += count
	}
	if outputCount != 0 || adoptedCount != 0 {
		s.epochProgress = true
	}
	s.work.Conflicts++
	s.work.ConflictActions += uint64(actions.Len())
	s.work.Forks += uint64(actions.Len() - 1)
	s.work.add(&s.work.ConflictActionArmsAdmitted, uint64(actions.Len()))
	s.work.add(&s.work.CausalConflictForks, uint64(actions.Len()-1))
	s.work.ConflictHeads += uint64(outputCount)
	if recoveryAmbiguitySource && outputCount > 1 {
		s.work.add(&s.work.RecoveryAmbiguityForks, 1)
	}
	s.work.Dispatches++
	if err := s.canonicalizeOwned(owner); err != nil {
		return err
	}
	if cell.versionLexerRequest != 0 {
		for index := range s.headers {
			header := &s.headers[index]
			if !header.shifted || header.versionLexerRequestReference() != cell.versionLexerRequest ||
				!diagnosticParserCoreVersionLexerSnapshotEqual(header.versionLexerSnapshot(), versionLexerRequest.before) {
				continue
			}
			if err := s.publishVersionLexerShiftOnHeaderOwned(owner, header, versionLexerRequest); err != nil {
				return err
			}
		}
	}
	if err := s.persistHeaderLineageOwned(owner); err != nil {
		return err
	}
	roundIndex := -1
	if s.fullReceipts() {
		primaryReceipts, err := diagnosticParserCoreHeaderReceipts(s.compact, primaries)
		if err != nil {
			return err
		}
		prefixReceipts, err := diagnosticParserCoreHeaderReceipts(s.compact, prefix)
		if err != nil {
			return err
		}
		suffixReceipts, err := diagnosticParserCoreHeaderReceipts(s.compact, suffix)
		if err != nil {
			return err
		}
		secondaryArms := make([]DiagnosticParserCoreGenericConflictArm, actions.Len()-1)
		for ordinal := 1; ordinal < actions.Len(); ordinal++ {
			arm := execution.arm(ordinal)
			outputs, receiptErr := diagnosticParserCoreHeaderReceipts(s.compact, arm)
			if receiptErr != nil {
				return receiptErr
			}
			secondaryArms[ordinal-1] = DiagnosticParserCoreGenericConflictArm{
				Ordinal: ordinal, BranchOrder: execution.round.Actions[ordinal-1].BranchOrder,
				Outputs: outputs, Paused: len(outputs) == 0 && s.conflictScratch.adopted[ordinal] == 0,
				Adopted: s.conflictScratch.adopted[ordinal] != 0,
			}
		}
		after, err := diagnosticParserCoreHeaderReceipts(s.compact, s.headers)
		if err != nil {
			return err
		}
		round := execution.round
		round.Index = len(s.receipt.Rounds)
		round.Before = before
		round.After = after
		roundIndex = round.Index
		s.receipt.Rounds = append(s.receipt.Rounds, round)
		conflict := DiagnosticParserCoreGenericConflict{
			ElectionIndex: s.electionIndex, Token: token, HeaderIndex: int(cell.headerIndex),
			BranchOrderBefore: branchOrderBefore, BranchOrderAfter: s.branchOrder,
			NextCreationSeqBefore: nextSeqBefore, NextCreationSeqAfter: s.nextSeq,
			Round: round, Prefix: prefixReceipts,
			PrimaryPaused: len(primaryReceipts) == 0 && s.conflictScratch.adopted[0] == 0, PrimaryAdopted: s.conflictScratch.adopted[0] != 0,
			OriginalSuffix: suffixReceipts,
			SecondaryArms:  secondaryArms, After: after,
		}
		if len(primaryReceipts) != 0 {
			conflict.PrimaryOutput = primaryReceipts[0]
			conflict.AdditionalPrimaryOutputs = primaryReceipts[1:]
		}
		s.receipt.Conflicts = append(s.receipt.Conflicts, conflict)
	}
	return s.recordGenericExternalShift(
		externalStatsBefore,
		roundIndex,
		token,
		scannerBefore,
		scannerAfter,
		affectedHead,
	)
}

func (s *diagnosticParserCoreGenericScheduler) applyGenericShifts(before []DiagnosticParserCoreHeaderReceipt, cells []diagnosticParserCoreGenericCell) (err error) {
	if s.freshSessionOwner != nil {
		return s.applyGenericShiftsOwned(*s.freshSessionOwner, before, cells)
	}
	if err := s.headerRollbackScratch.begin(s.headers); err != nil {
		return err
	}
	dispatchesBefore, workBefore, epochProgressBefore := s.dispatches, s.work, s.epochProgress
	tokenCellBefore := s.tokenCell
	roundsBefore, externalBefore := len(s.receipt.Rounds), len(s.receipt.ExternalShifts)
	defer func() {
		s.headerRollbackScratch.finish(&s.headers, err != nil)
		if err == nil {
			return
		}
		s.dispatches, s.work, s.epochProgress = dispatchesBefore, workBefore, epochProgressBefore
		s.tokenCell = tokenCellBefore
		s.receipt.Rounds = s.receipt.Rounds[:roundsBefore]
		s.receipt.ExternalShifts = s.receipt.ExternalShifts[:externalBefore]
	}()
	return s.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		return s.applyGenericShiftsOwned(owner, before, cells)
	})
}

func (s *diagnosticParserCoreGenericScheduler) applyGenericShiftsOwned(owner core.SchedulerTransactionToken, before []DiagnosticParserCoreHeaderReceipt, cells []diagnosticParserCoreGenericCell) error {
	if len(cells) == 0 {
		return errors.New("parser-core phase zero: empty ordinary shift cohort")
	}
	if err := s.reserveDispatches(uint64(len(cells))); err != nil {
		return err
	}
	telemetryCell := 0
	for index := range cells {
		if cells[index].dispatchToken(s.token).ExternalScannerToken {
			telemetryCell = index
			break
		}
	}
	telemetryToken := cells[telemetryCell].dispatchToken(s.token)
	scannerBefore, scannerAfter := s.currentElection.ScannerBefore, s.currentElection.ScannerAfter
	affectedHead := cells[telemetryCell].boundary.Head()
	externalStatsBefore, err := s.genericExternalStats(affectedHead, telemetryToken)
	if err != nil {
		return err
	}
	ordinaryCohort := len(cells) > 1 && !s.recoveryIsolation
	if ordinaryCohort {
		for index := range cells {
			cell := &cells[index]
			if cell.selectedActionOrdinal() != 0 || cell.dispatchToken(s.token) != cells[0].dispatchToken(s.token) {
				ordinaryCohort = false
				break
			}
		}
	}
	if ordinaryCohort {
		s.classifiedBoundaries = s.classifiedBoundaries[:0]
		for index := range cells {
			cell := &cells[index]
			s.classifiedBoundaries = append(s.classifiedBoundaries, cell.boundary)
		}
		token := cells[0].dispatchToken(s.token)
		heads, err := s.compact.ShiftOrdinaryClassifiedCohortWithLiveCondenseCandidatesOwned(owner, nil, s.classifiedBoundaries, core.Token{
			Symbol: core.Symbol(token.Symbol), StartByte: token.StartByte, EndByte: token.EndByte, External: token.ExternalScannerToken,
			LexerSkippedPrefixLength: diagnosticParserCoreLexerSkippedPrefixLength(token, s.options.captureLexerSkippedPrefixProvenance),
		})
		if err != nil {
			return err
		}
		affectedHead = heads[telemetryCell]
		for index := range cells {
			cell := &cells[index]
			s.headers[cell.headerIndex].head = heads[index]
			s.headers[cell.headerIndex].shifted = true
			markDiagnosticParserCoreExternalLineage(&s.headers[cell.headerIndex], token)
		}
		s.work.OrdinaryCohorts++
	} else {
		for index := range cells {
			cell := &cells[index]
			var versionLexerRequest *diagnosticParserCoreVersionLexerRequest
			if cell.versionLexerRequest != 0 {
				versionLexerRequest, err = s.versionLexerRequestForCell(*cell)
				if err != nil {
					return err
				}
			}
			ordinal := cell.selectedActionOrdinal()
			action := cell.actions().At(ordinal)
			if action.Type != core.ActionShift || action.Extra {
				return errors.New("parser-core phase zero: ordinary shift selection is not an ordinary shift")
			}
			token := cell.dispatchToken(s.token)
			shifted := core.Token{
				Symbol: core.Symbol(token.Symbol), StartByte: token.StartByte, EndByte: token.EndByte, External: token.ExternalScannerToken,
				LexerSkippedPrefixLength: diagnosticParserCoreLexerSkippedPrefixLength(token, s.options.captureLexerSkippedPrefixProvenance),
			}
			head, err := s.compact.ShiftClassifiedWithLiveCondenseCandidatesOwned(
				owner, s.collectCondenseCandidates(int(cell.headerIndex)),
				cell.boundary, ordinal, shifted, core.ForkOrder{},
			)
			if err != nil {
				return err
			}
			if index == telemetryCell {
				affectedHead = head
			}
			s.headers[cell.headerIndex].head = head
			s.headers[cell.headerIndex].shifted = true
			markDiagnosticParserCoreExternalLineage(&s.headers[cell.headerIndex], token)
			if versionLexerRequest != nil {
				if err := s.publishVersionLexerShiftOnHeaderOwned(owner, &s.headers[cell.headerIndex], versionLexerRequest); err != nil {
					return err
				}
			}
		}
	}
	s.epochProgress = true
	s.work.OrdinaryShifts += uint64(len(cells))
	s.work.Dispatches += uint64(len(cells))
	if err := s.canonicalizeOwned(owner); err != nil {
		return err
	}
	if err := s.persistHeaderLineageOwned(owner); err != nil {
		return err
	}
	roundIndex := -1
	if s.fullReceipts() {
		actions := make([]DiagnosticParserCoreRoundAction, len(cells))
		for index := range cells {
			cell := &cells[index]
			ordinal := cell.selectedActionOrdinal()
			actions[index] = DiagnosticParserCoreRoundAction{
				HeaderIndex: int(cell.headerIndex), State: StateID(cell.boundary.State()), ByteOffset: cell.boundary.ByteOffset(),
				Ordinal: ordinal, Action: rootParserCoreAction(cell.actions().At(ordinal)),
			}
		}
		after, err := diagnosticParserCoreHeaderReceipts(s.compact, s.headers)
		if err != nil {
			return err
		}
		round := DiagnosticParserCoreDispatchRound{
			Index: len(s.receipt.Rounds), Before: before, Actions: actions, After: after,
		}
		roundIndex = round.Index
		s.receipt.Rounds = append(s.receipt.Rounds, round)
	}
	return s.recordGenericExternalShift(
		externalStatsBefore,
		roundIndex,
		telemetryToken,
		scannerBefore,
		scannerAfter,
		affectedHead,
	)
}

func (s *diagnosticParserCoreGenericScheduler) applyGenericExtraShifts(before []DiagnosticParserCoreHeaderReceipt, cells []diagnosticParserCoreGenericCell) (err error) {
	if err := s.headerRollbackScratch.begin(s.headers); err != nil {
		return err
	}
	dispatchesBefore, workBefore, epochProgressBefore := s.dispatches, s.work, s.epochProgress
	tokenCellBefore := s.tokenCell
	roundsBefore, externalShiftsBefore := len(s.receipt.Rounds), len(s.receipt.ExternalShifts)
	defer func() {
		s.headerRollbackScratch.finish(&s.headers, err != nil)
		if err == nil {
			return
		}
		s.dispatches, s.work, s.epochProgress = dispatchesBefore, workBefore, epochProgressBefore
		s.tokenCell = tokenCellBefore
		s.receipt.Rounds = s.receipt.Rounds[:roundsBefore]
		s.receipt.ExternalShifts = s.receipt.ExternalShifts[:externalShiftsBefore]
	}()
	return s.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		if len(cells) == 0 {
			return errors.New("parser-core phase zero: empty extra shift cohort")
		}
		for index := range cells {
			cell := &cells[index]
			if cell.actions().Len() != 1 || cell.actions().At(0).Type != core.ActionShift || !cell.actions().At(0).Extra {
				return errors.New("parser-core phase zero: extra cohort requires one decoded extra action per head")
			}
		}
		if err := s.reserveDispatches(uint64(len(cells))); err != nil {
			return err
		}
		token := cells[0].dispatchToken(s.token)
		telemetryCell := 0
		for index := range cells {
			if cells[index].dispatchToken(s.token).ExternalScannerToken {
				telemetryCell = index
				break
			}
		}
		telemetryToken := cells[telemetryCell].dispatchToken(s.token)
		scannerBefore, scannerAfter := s.currentElection.ScannerBefore, s.currentElection.ScannerAfter
		affectedHead := cells[telemetryCell].boundary.Head()
		externalStatsBefore, err := s.genericExternalStats(affectedHead, telemetryToken)
		if err != nil {
			return err
		}
		isolatedRecovery := s.recoveryIsolation
		if isolatedRecovery {
			for index := range cells {
				cell := &cells[index]
				var versionLexerRequest *diagnosticParserCoreVersionLexerRequest
				if cell.versionLexerRequest != 0 {
					versionLexerRequest, err = s.versionLexerRequestForCell(*cell)
					if err != nil {
						return err
					}
				}
				head, shiftErr := s.compact.ShiftClassifiedWithLiveCondenseCandidatesOwned(
					owner, s.collectCondenseCandidates(int(cell.headerIndex)), cell.boundary, 0,
					core.Token{
						Symbol: core.Symbol(token.Symbol), StartByte: token.StartByte, EndByte: token.EndByte,
						Extra: true, External: token.ExternalScannerToken,
						LexerSkippedPrefixLength: diagnosticParserCoreLexerSkippedPrefixLength(token, s.options.captureLexerSkippedPrefixProvenance),
					},
					core.ForkOrder{},
				)
				if shiftErr != nil {
					return shiftErr
				}
				if index == telemetryCell {
					affectedHead = head
				}
				s.headers[cell.headerIndex].head = head
				s.headers[cell.headerIndex].shifted = true
				markDiagnosticParserCoreExternalLineage(&s.headers[cell.headerIndex], token)
				if versionLexerRequest != nil {
					if err := s.publishVersionLexerShiftOnHeaderOwned(owner, &s.headers[cell.headerIndex], versionLexerRequest); err != nil {
						return err
					}
				}
			}
		} else {
			s.classifiedBoundaries = s.classifiedBoundaries[:0]
			for index := range cells {
				s.classifiedBoundaries = append(s.classifiedBoundaries, cells[index].boundary)
			}
			heads, shiftErr := s.compact.ShiftExtraClassifiedCohortWithLiveCondenseCandidatesOwned(owner, nil, s.classifiedBoundaries, core.Token{
				Symbol: core.Symbol(token.Symbol), StartByte: token.StartByte, EndByte: token.EndByte,
				Extra: true, External: token.ExternalScannerToken,
				LexerSkippedPrefixLength: diagnosticParserCoreLexerSkippedPrefixLength(token, s.options.captureLexerSkippedPrefixProvenance),
			})
			if shiftErr != nil {
				return shiftErr
			}
			affectedHead = heads[telemetryCell]
			for index := range cells {
				cell := &cells[index]
				var versionLexerRequest *diagnosticParserCoreVersionLexerRequest
				if cell.versionLexerRequest != 0 {
					versionLexerRequest, err = s.versionLexerRequestForCell(*cell)
					if err != nil {
						return err
					}
				}
				s.headers[cell.headerIndex].head = heads[index]
				s.headers[cell.headerIndex].shifted = true
				markDiagnosticParserCoreExternalLineage(&s.headers[cell.headerIndex], token)
				if versionLexerRequest != nil {
					if err := s.publishVersionLexerShiftOnHeaderOwned(owner, &s.headers[cell.headerIndex], versionLexerRequest); err != nil {
						return err
					}
				}
			}
		}
		if s.extraPostExecutionFault != nil {
			if err := s.extraPostExecutionFault(); err != nil {
				return err
			}
		}
		s.epochProgress = true
		s.work.ExtraShifts += uint64(len(cells))
		if len(cells) > 1 && !isolatedRecovery {
			s.work.ExtraCohorts++
		}
		s.work.Dispatches += uint64(len(cells))
		if err := s.canonicalizeOwned(owner); err != nil {
			return err
		}
		if err := s.persistHeaderLineageOwned(owner); err != nil {
			return err
		}
		roundIndex := -1
		if s.fullReceipts() {
			after, err := diagnosticParserCoreHeaderReceipts(s.compact, s.headers)
			if err != nil {
				return err
			}
			actions := make([]DiagnosticParserCoreRoundAction, len(cells))
			for index := range cells {
				cell := &cells[index]
				actions[index] = DiagnosticParserCoreRoundAction{
					HeaderIndex: int(cell.headerIndex), State: StateID(cell.boundary.State()), ByteOffset: cell.boundary.ByteOffset(),
					Ordinal: 0, Action: rootParserCoreAction(cell.actions().At(0)),
				}
			}
			round := DiagnosticParserCoreDispatchRound{
				Index: len(s.receipt.Rounds), Before: before, Actions: actions, After: after,
			}
			roundIndex = round.Index
			s.receipt.Rounds = append(s.receipt.Rounds, round)
		}
		return s.recordGenericExternalShift(
			externalStatsBefore,
			roundIndex,
			telemetryToken,
			scannerBefore,
			scannerAfter,
			affectedHead,
		)
	})
}

func (s *diagnosticParserCoreGenericScheduler) genericExternalStats(
	head core.Head,
	token Token,
) (core.Stats, error) {
	if !s.fullReceipts() || !token.ExternalScannerToken {
		return core.Stats{}, nil
	}
	return s.compact.Stats(head)
}

func (s *diagnosticParserCoreGenericScheduler) recordGenericExternalShift(
	before core.Stats,
	roundIndex int,
	token Token,
	scannerBefore DiagnosticParserCoreScannerCheckpoint,
	scannerAfter DiagnosticParserCoreScannerCheckpoint,
	affectedHead core.Head,
) error {
	if !s.fullReceipts() || !token.ExternalScannerToken {
		return nil
	}
	after, err := s.compact.Stats(affectedHead)
	if err != nil {
		return err
	}
	external := DiagnosticParserCoreGenericExternalShift{
		ElectionIndex: s.electionIndex, Token: token,
		ScannerBefore: scannerBefore, ScannerAfter: scannerAfter,
		RoundIndex: roundIndex,
	}
	for id := before.Subtrees + 1; id <= after.Subtrees; id++ {
		view, err := s.compact.Subtree(core.SubtreeID(id))
		if err != nil {
			return err
		}
		if !view.Terminal || !view.External {
			continue
		}
		external.Payloads = append(external.Payloads, diagnosticParserCoreTerminalPayloadView(id, view))
	}
	if len(external.Payloads) != 0 {
		s.receipt.ExternalShifts = append(s.receipt.ExternalShifts, external)
	}
	return nil
}

func diagnosticParserCoreGenericUnsupportedCell(headerIndex int, token Token, actions core.ActionRow) *diagnosticParserCoreGenericUnsupported {
	return diagnosticParserCoreGenericUnsupportedCellDescriptor(headerIndex, token, actions, actions.Descriptor())
}

func diagnosticParserCoreGenericUnsupportedCellDescriptor(headerIndex int, token Token, actions core.ActionRow, descriptor core.ActionRowDescriptor) *diagnosticParserCoreGenericUnsupported {
	unsupported := func(boundary DiagnosticParserCoreBoundaryKind, detail string) *diagnosticParserCoreGenericUnsupported {
		return &diagnosticParserCoreGenericUnsupported{boundary: boundary, detail: detail, headerIndex: headerIndex}
	}
	switch descriptor.Kind() {
	case core.ActionRowEmpty:
		return unsupported(DiagnosticParserCoreNoAction, "generic scheduler reached an empty action cell")
	case core.ActionRowShift:
		return nil
	case core.ActionRowExtraShift:
		if token.EndByte < token.StartByte || token.EndByte == token.StartByte && !token.ExternalScannerToken {
			return unsupported(DiagnosticParserCoreRoute, "generic scheduler extra shift has invalid token geometry")
		}
		return nil
	case core.ActionRowReduce:
		return nil
	case core.ActionRowAccept:
		if token.Symbol != 0 || token.StartByte != token.EndByte || token.Missing || token.NoLookahead || token.ExternalScannerToken {
			return unsupported(DiagnosticParserCoreAccept, "generic scheduler accept requires one authenticated EOF action")
		}
		return nil
	case core.ActionRowConflict:
		return nil
	}

	// Unsupported rows retain the ordinal scan so the first failure and its
	// diagnostic remain byte-for-byte ordered as before descriptor compilation.
	for ordinal := 0; ordinal < actions.Len(); ordinal++ {
		action := actions.At(ordinal)
		if action.Repetition {
			return unsupported(DiagnosticParserCoreRoute, "generic scheduler does not support repetition shifts")
		}
		if action.ExtraChain {
			return unsupported(DiagnosticParserCoreExtraChain, "generic scheduler does not support extra-chain shifts")
		}
		if action.Extra && (actions.Len() != 1 || action.Type != core.ActionShift) {
			return unsupported(DiagnosticParserCoreExtra, "generic scheduler extra action is not one sole shift")
		}
		switch action.Type {
		case core.ActionReduce:
		case core.ActionShift:
		case core.ActionRecover:
			return unsupported(DiagnosticParserCoreRecovery, "generic scheduler reached recovery")
		case core.ActionAccept:
			if actions.Len() != 1 || token.Symbol != 0 || token.StartByte != token.EndByte || token.Missing || token.NoLookahead || token.ExternalScannerToken {
				return unsupported(DiagnosticParserCoreAccept, "generic scheduler accept requires one authenticated EOF action")
			}
		default:
			return unsupported(DiagnosticParserCoreRoute, "generic scheduler reached an unknown action")
		}
	}
	return nil
}

func diagnosticParserCoreGenericUnsupportedToken(token Token) *diagnosticParserCoreGenericUnsupported {
	switch {
	case token.Missing:
		return &diagnosticParserCoreGenericUnsupported{
			boundary: DiagnosticParserCoreRoute, detail: "generic scheduler does not support missing tokens",
		}
	default:
		return nil
	}
}

// validateGenericNoLookaheadReduction admits the narrow production-equivalent
// synthetic-EOF shape. One closed head may apply one reduction, then it must
// re-elect at the same byte. A root reduction also authenticates the next EOF.
func (s *diagnosticParserCoreGenericScheduler) validateGenericNoLookaheadReduction(
	cells []diagnosticParserCoreGenericCell,
	noActionIndices []int,
) *diagnosticParserCoreGenericUnsupported {
	if !s.token.NoLookahead {
		return nil
	}
	unsupported := func(detail string) *diagnosticParserCoreGenericUnsupported {
		return &diagnosticParserCoreGenericUnsupported{
			boundary: DiagnosticParserCoreRoute, detail: detail, headerIndex: 0,
		}
	}
	if s.token.Symbol != 0 || s.token.StartByte != s.token.EndByte ||
		s.token.Missing || s.token.ExternalScannerToken {
		return unsupported("generic scheduler no-lookahead token is not authenticated synthetic EOF")
	}
	if s.currentElection.ScannerBefore != s.currentElection.ScannerAfter {
		return unsupported("generic scheduler no-lookahead election changed scanner state")
	}
	if len(s.headers) != 1 || len(cells) != 1 || len(noActionIndices) != 0 ||
		cells[0].headerIndex != 0 {
		return unsupported("generic scheduler no-lookahead reduction requires one runnable head")
	}
	actions := cells[0].actions()
	if actions.Len() != 1 || cells[0].descriptor().Kind() != core.ActionRowReduce {
		return unsupported("generic scheduler no-lookahead token requires one sole reduction")
	}
	if !s.options.hasNoLookaheadRootSymbol {
		return unsupported("generic scheduler no-lookahead reduction requires an authenticated root symbol")
	}
	return nil
}

// replaceDiagnosticParserCoreHeader replaces headers[index] with replacements.
// It reuses the headers backing array when its capacity allows, so multi-output
// reductions no longer allocate a fresh frontier slice on every reduction. The
// replacements slice is a distinct scheduler buffer, so it never aliases
// headers. Reusing the headers backing is safe: canonicalization always copies
// its input before use, and the rollback scratch snapshots a separate copy, so
// no other frontier owner observes the reused storage. Go's copy is memmove-
// safe, so the overlapping tail shift is correct for both growth and shrink.
func replaceDiagnosticParserCoreHeader(headers []diagnosticParserCoreHeader, index int, replacements []diagnosticParserCoreHeader) []diagnosticParserCoreHeader {
	oldLen := len(headers)
	newLen := oldLen - 1 + len(replacements)
	if newLen <= cap(headers) {
		headers = headers[:newLen]
		copy(headers[index+len(replacements):], headers[index+1:oldLen])
		copy(headers[index:index+len(replacements)], replacements)
		// A shrink leaves the old tail inside the retained backing array. Clear
		// only the previously populated tail because headers can own snapshots.
		clearDiagnosticParserCoreHeaderSuffix(headers[:oldLen], newLen)
		return headers
	}
	out := make([]diagnosticParserCoreHeader, newLen, max(newLen, 2*cap(headers)))
	copy(out, headers[:index])
	copy(out[index:index+len(replacements)], replacements)
	copy(out[index+len(replacements):], headers[index+1:oldLen])
	return out
}

func (s *diagnosticParserCoreGenericScheduler) canonicalize() error {
	if s.freshSessionOwner == nil {
		return errors.New("parser-core phase zero: canonicalization requires an authenticated scheduler token")
	}
	return s.canonicalizeOwned(*s.freshSessionOwner)
}

func (s *diagnosticParserCoreGenericScheduler) canonicalizeOwned(owner core.SchedulerTransactionToken) error {
	return s.canonicalizeOwnedWithMutation(owner, 0)
}

func (s *diagnosticParserCoreGenericScheduler) canonicalizeOwnedWithMutation(owner core.SchedulerTransactionToken, expected core.DropCohortProducerMutation) error {
	previousVersionStateEqual := s.canonicalScratch.versionStateEqual
	s.canonicalScratch.versionStateEqual = s.versionLexerStateEqual
	defer func() {
		s.canonicalScratch.versionStateEqual = previousVersionStateEqual
	}()
	var headers []diagnosticParserCoreHeader
	var mutation core.DropCohortProducerMutation
	var err error
	if s.recoveryIsolation {
		headers, mutation, err = s.canonicalScratch.canonicalizeRecoveryWithMutation(s.compact, s.headers)
	} else {
		headers, mutation, err = s.canonicalScratch.canonicalizeWithMutation(s.compact, s.headers)
	}
	if err != nil {
		return err
	}
	if s.recoveryIsolation {
		var drops uint64
		var applied bool
		headers, drops, applied, err = s.condenseRecoveryVersions(owner, headers)
		if err != nil {
			// The recovery canonicalizer copied header-owned pointers into its
			// alternate buffer. Release that failed frontier before rollback.
			clear(headers)
			return err
		}
		if applied {
			s.work.add(&s.work.RecoveryCondensePasses, 1)
			s.work.add(&s.work.RecoveryVersionCapDrops, drops)
			// C ends recovery competition when pairwise condensation leaves
			// one active version. Clear the compact marker before the next
			// dispatch, or the sole winner rejects itself as mixed ambiguity.
			if len(headers) == 1 && !headers[0].accepted {
				headers[0].clearRecoveryLineage()
				s.recoveryIsolation = false
			}
		}
	}
	if expected != 0 && mutation != expected {
		return fmt.Errorf("parser-core phase zero: canonicalizer mutation class mismatch: got %d want %d", mutation, expected)
	}
	if mutation != 0 {
		if err := s.compact.RecordDropCohortProducerMutation(owner, mutation); err != nil {
			return err
		}
	}
	s.invalidateVerifierHeaderBinding()
	s.headers = headers
	s.work.Canonicalizations++
	if uint64(len(headers)) > s.work.PeakHeaders {
		s.work.PeakHeaders = uint64(len(headers))
	}
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) reserveDispatches(count uint64) error {
	if count > s.options.MaxDispatches || s.dispatches > s.options.MaxDispatches-count {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreCap, detail: "generic scheduler dispatch cap"}
	}
	s.dispatches += count
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) elect(first bool) error {
	if s.eofRecoveryAdmission.active && s.eofRecoveryAdmission.valid &&
		s.eofRecoveryAdmission.state != compactEOFRecoveryAdmissionEmpty &&
		s.eofRecoveryAdmission.state != compactEOFRecoveryAdmissionInvalid &&
		s.eofRecoveryAdmission.state != compactEOFRecoveryAdmissionConsumed {
		compactEOFRecoveryAdmissionInvalidate(&s.eofRecoveryAdmission, "new election")
	}
	if s.tokens >= s.options.MaxTokens {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreCap, detail: "generic scheduler token cap"}
	}
	// states is scheduler-owned scratch, rebuilt every election. It feeds
	// SetParserState, a separate reused GLR buffer, and currentElection.States
	// (read only within this round for summary receipts; cloned below when full
	// receipts retain the election). This collapses one slice allocation per
	// election without changing the frontier order or work graph.
	states := s.electStates[:0]
	if cap(states) < len(s.headers) {
		states = make([]StateID, 0, max(len(s.headers), 2*cap(states)))
	}
	for _, header := range s.headers {
		state, err := s.electHeaderState(header)
		if err != nil {
			return err
		}
		shiftIdentity := header.shifted || first && !header.shifted
		// Precondition: s.checkpointID always holds a value produced by
		// diagnosticParserCoreInternCheckpoint, set only at its two writer sites
		// (the scheduler seed above and the election afterID assignment below), so
		// this raw identity comparison is a sound substitute for a digest lookup.
		if !shiftIdentity || header.accepted || header.checkpoint != s.checkpointID {
			// Full receipts already validated the checkpoint while reading the
			// header. Summary mode skips that digest lookup on the healthy hot
			// path, but keeps the legacy invalid-checkpoint error when this cold
			// identity gate rejects a malformed header.
			if !s.fullReceipts() {
				if _, _, ok := s.compact.CheckpointReceipt(header.checkpoint); !ok {
					return errDiagnosticParserCoreUnknownCheckpointIdentity
				}
			}
			return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "generic scheduler election frontier is not closed and checkpoint-continuous"}
		}
		states = append(states, state)
	}
	s.electStates = states
	if s.observer.beforeElection != nil {
		if err := s.observer.beforeElection(s); err != nil {
			return err
		}
	}
	s.tokenSource.SetParserState(states[0])
	if len(states) == 1 {
		s.tokenSource.SetGLRStates(nil)
	} else {
		// The token source retains the passed slice only until the next
		// election reassigns it, so a second reused buffer keeps the copy
		// semantics without allocating.
		glr := append(s.electGLRStates[:0], states...)
		s.electGLRStates = glr
		s.tokenSource.SetGLRStates(glr)
	}
	beforeBytes := s.tokenSource.captureExternalScannerStateInto(s.scannerScratch)
	// Retain the exact DFA and scanner state at this election's token start.
	// A ragged per-header relex may activate ownership before any shared cell
	// executes. Reuse the checkpoint serialization instead of serializing the
	// scanner twice at the same cursor.
	if err := s.captureSharedElectionSnapshotFromExternalPayload(beforeBytes); err != nil {
		return err
	}
	beforeID, before, err := diagnosticParserCoreInternCheckpoint(s.compact, beforeBytes)
	if err != nil {
		return err
	}
	if beforeID != s.checkpointID {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "generic scheduler scanner checkpoint continuity failed"}
	}
	s.checkpointBeforeID = beforeID
	workCountRecordFrontierLexerElection()
	token := s.tokenSource.Next()
	afterBytes := s.tokenSource.captureExternalScannerStateInto(s.scannerScratch)
	afterID, after, err := diagnosticParserCoreInternCheckpoint(s.compact, afterBytes)
	if err != nil {
		return err
	}
	current, currentStart, currentEnd, currentValid := currentExternalScannerCheckpoint(s.tokenSource)
	if s.requireEOFPostNoLookaheadRoot {
		if token.Symbol != 0 || token.StartByte != token.EndByte ||
			token.Missing || token.NoLookahead || token.ExternalScannerToken {
			return &diagnosticParserCoreDecline{
				boundary: DiagnosticParserCoreRoute,
				detail:   "generic scheduler root reduction on no-lookahead was not followed by authenticated EOF",
			}
		}
		s.requireEOFPostNoLookaheadRoot = false
	}
	if token.NoLookahead {
		s.noLookaheadSteps++
		if s.noLookaheadSteps > maxDiagnosticParserCoreNoLookaheadSteps {
			return &diagnosticParserCoreDecline{
				boundary: DiagnosticParserCoreCap,
				detail:   "generic scheduler no-lookahead re-election cap",
			}
		}
	} else {
		s.noLookaheadSteps = 0
	}
	if err := s.compact.BeginFrontier(); err != nil {
		return err
	}
	if err := s.compact.SetPhaseExternalTokenScannerCheckpoints(beforeID, afterID); err != nil {
		return err
	}
	for index := range s.headers {
		s.headers[index].shifted = false
		s.headers[index].paused = false
		s.headers[index].frontierSequence = 0
		s.headers[index].checkpoint = afterID
	}
	s.electionIndex++
	s.tokens++
	s.work.Elections++
	s.token = token
	s.checkpoint = after
	s.checkpointID = afterID
	// tokenCell mirrors this election's freshly elected token identity for the
	// forced-reuse tranche (substrate only; nothing reads it yet). state is
	// the primary election state passed to SetParserState above;
	// beforeCheckpoint/afterCheckpoint are the same interned checkpoint IDs
	// just assigned to checkpointBeforeID/checkpointID.
	s.tokenCell = diagnosticParserCoreTokenCell{
		token: token, state: states[0], byteOffset: token.StartByte,
		beforeCheckpoint: beforeID, afterCheckpoint: afterID, valid: true,
	}
	s.epochProgress = false
	// Summary receipts read currentElection.States only within this round, so
	// the reused scratch is safe. Full receipts retain the election, so clone
	// the states into an owned slice before appending it.
	electionStates := states
	if s.fullReceipts() {
		electionStates = append([]StateID(nil), states...)
	}
	election := DiagnosticParserCoreElection{
		States: electionStates, Token: token, ScannerBefore: before, ScannerAfter: after,
		CurrentCheckpointValid: currentValid,
		CurrentCheckpointStart: parserCoreCheckpoint(current.start),
		CurrentCheckpointEnd:   parserCoreCheckpoint(current.end),
		CurrentCheckpointBytes: [2]uint32{currentStart, currentEnd},
	}
	s.currentElection = election
	if s.fullReceipts() {
		s.receipt.Elections = append(s.receipt.Elections, election)
	}
	if s.observer.afterElection != nil {
		stop, err := s.observer.afterElection(s)
		if err != nil {
			return err
		}
		s.stoppedAfterElection = stop
	}
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) completeAtClosedByte(target uint32) (bool, error) {
	receipts, err := s.headerReceipts(s.headers)
	if err != nil {
		return false, err
	}
	allBelow := true
	for index, header := range receipts {
		if !header.Shifted || header.Accepted || s.headers[index].checkpoint != s.checkpointID {
			return false, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "generic completion frontier is not shifted, nonaccepted, and checkpoint-continuous"}
		}
		if header.ByteOffset >= target {
			allBelow = false
		}
	}
	if allBelow {
		return false, nil
	}
	for _, header := range receipts {
		if header.ByteOffset != target {
			return false, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "generic completion frontier straddled or passed the requested byte"}
		}
	}
	stats, err := s.compact.Stats(s.headers[0].head)
	if err != nil {
		return false, err
	}
	completion := &DiagnosticParserCoreGenericCompletion{
		TargetByte: target, ElectionIndex: s.electionIndex, LastToken: s.token,
		State: receipts[0].State, Stats: stats, Work: s.work,
	}
	if s.fullReceipts() {
		paths, err := diagnosticParserCoreHeaderPathReceipts(s.compact, s.headers)
		if err != nil {
			return false, err
		}
		completion.Headers = paths
	}
	s.receipt.Completion = completion
	s.publishTotals()
	return true, nil
}

func (s *diagnosticParserCoreGenericScheduler) finish(boundary DiagnosticParserCoreBoundaryKind, detail string, headerIndex int) error {
	if s != nil && s.eofRecoveryAdmission.active && s.eofRecoveryAdmission.valid &&
		s.eofRecoveryAdmission.state != compactEOFRecoveryAdmissionConsumed {
		compactEOFRecoveryAdmissionInvalidate(&s.eofRecoveryAdmission, detail)
	}
	if headerIndex < 0 || headerIndex >= len(s.headers) {
		return errors.New("parser-core phase zero: generic stop header index out of range")
	}
	header, err := s.headerReceipt(s.headers[headerIndex])
	if err != nil {
		return err
	}
	stats, err := s.compact.Stats(s.headers[headerIndex].head)
	if err != nil {
		return err
	}
	stop := DiagnosticParserCoreGenericStop{
		Boundary: boundary, Detail: detail, ElectionIndex: s.electionIndex,
		HeaderIndex: headerIndex, State: header.State, ByteOffset: header.ByteOffset,
		Token: s.token, Stats: stats, Work: s.work,
	}
	if s.fullReceipts() {
		paths, err := diagnosticParserCoreHeaderPathReceipts(s.compact, s.headers)
		if err != nil {
			return err
		}
		stop.Headers = paths
	}
	s.receipt.Stop = stop
	s.publishTotals()
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) publishTotals() {
	s.receipt.Tokens = s.tokens
	s.receipt.Dispatches = s.dispatches
	s.receipt.GlobalBranchOrder = s.branchOrder
	s.receipt.NextCreationSeq = s.nextSeq
	s.receipt.PerVersionLexRequests = s.work.PerVersionLexRequests
	s.receipt.PerVersionLexRestores = s.work.PerVersionLexRestores
	s.receipt.PerVersionLexPublications = s.work.PerVersionLexPublications
	s.receipt.PerVersionLexAcceptedRaggedSpans = s.work.PerVersionLexAcceptedRaggedSpans
	s.receipt.PerVersionLexViabilityDrops = s.work.PerVersionLexViabilityDrops
	s.receipt.PeakLiveVersions = s.work.PeakLiveVersions
	s.receipt.PotentialReductionActions = s.work.PotentialReductionActions
	s.receipt.PotentialReductionOutputs = s.work.PotentialReductionOutputs
	s.receipt.ReductionPromotions = s.work.ReductionPromotions
	s.receipt.MissingTokenTrials = s.work.MissingTokenTrials
	s.receipt.MissingTokenCommits = s.work.MissingTokenCommits
	s.receipt.RecoveryDiscontinuityMerges = s.work.RecoveryDiscontinuityMerges
	s.receipt.RecoveryCeilingDeclines = s.work.RecoveryCeilingDeclines
}

func authenticatedParserCoreGoLanguage(scanner ExternalScanner) (*Language, error) {
	const goBlobSHA256 = "9cf914d26d962d1a62e7954f8b20b302337a44cb7d4a07218eec482c45a57a08"
	if fmt.Sprintf("%x", sha256.Sum256(parserCoreCertifiedGoBlob)) != goBlobSHA256 {
		return nil, errors.New("parser-core phase zero: certified Go grammar identity mismatch")
	}
	scannerType := reflect.TypeOf(scanner)
	if scannerType == nil || scannerType.Kind() != reflect.Struct || scannerType.PkgPath() != "github.com/odvcencio/gotreesitter/grammars" || scannerType.Name() != "GoExternalScanner" {
		return nil, errors.New("parser-core phase zero: certified Go external scanner identity mismatch")
	}
	decoded, err := LoadLanguage(parserCoreCertifiedGoBlob)
	if err != nil {
		return nil, fmt.Errorf("parser-core phase zero: decode embedded Go blob: %w", err)
	}
	decoded.Name = "go"
	decoded.ExternalScanner = scanner
	decoded.CompactConvergedReductionSplitDropsCertified = true
	CertifyCRecoveryCostCompetition(decoded)
	return decoded, nil
}

func applyParserCorePrefixAction(compact *core.Core, head core.Head, token Token, action core.Action, ordinal int, fork core.ForkOrder) ([]core.Head, error) {
	switch action.Type {
	case core.ActionShift:
		out, err := compact.Shift(head, core.Symbol(token.Symbol), ordinal, core.Token{
			Symbol: core.Symbol(token.Symbol), StartByte: token.StartByte, EndByte: token.EndByte,
			Extra: action.Extra, External: token.ExternalScannerToken,
		}, fork)
		return []core.Head{out}, err
	case core.ActionReduce:
		return compact.Reduce(head, core.Symbol(token.Symbol), ordinal, fork)
	case core.ActionRecover:
		return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRecovery, detail: "unexpected recover action in generic conflict"}
	case core.ActionAccept:
		return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreAccept, detail: "unexpected accept action in generic conflict"}
	default:
		return nil, errors.New("parser-core phase zero: unknown conflict action")
	}
}

func applyParserCoreConflictActionInto(
	dst []diagnosticParserCoreActionOutput,
	reductionDst []core.ReductionOutput,
	compact *core.Core,
	owner core.SchedulerTransactionToken,
	classified core.ClassifiedBoundary,
	token Token,
	action core.Action,
	ordinal int,
	fork core.ForkOrder,
	nextCleanPathLineage *uint16,
	captureLexerSkippedPrefixProvenance bool,
	reductionCost core.ReductionOutputCostFunc,
) ([]diagnosticParserCoreActionOutput, []core.ReductionOutput, error) {
	if action.Type != core.ActionReduce {
		switch action.Type {
		case core.ActionShift:
			head, err := compact.ShiftClassifiedOwned(owner, classified, ordinal, core.Token{
				Symbol: core.Symbol(token.Symbol), StartByte: token.StartByte, EndByte: token.EndByte,
				Extra: action.Extra, External: token.ExternalScannerToken,
				LexerSkippedPrefixLength: diagnosticParserCoreLexerSkippedPrefixLength(token, captureLexerSkippedPrefixProvenance),
			}, fork)
			if err != nil {
				return nil, reductionDst, err
			}
			return append(dst, diagnosticParserCoreActionOutput{head: head, freshness: core.ReductionNew}), reductionDst, nil
		case core.ActionRecover:
			return nil, reductionDst, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRecovery, detail: "unexpected recover action in generic conflict"}
		case core.ActionAccept:
			return nil, reductionDst, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreAccept, detail: "unexpected accept action in generic conflict"}
		default:
			return nil, reductionDst, errors.New("parser-core phase zero: unknown conflict action")
		}
	}
	var outputs []core.ReductionOutput
	var err error
	if reductionCost == nil {
		outputs, err = compact.ReduceOutputsClassifiedIntoOwned(owner, reductionDst, classified, ordinal, fork)
	} else {
		outputs, err = compact.ReduceOutputsClassifiedIntoWithCostOwned(
			owner, reductionDst, classified, ordinal, fork, reductionCost,
		)
	}
	if err != nil {
		return nil, outputs, err
	}
	var lineage uint16
	if len(outputs) != 0 && outputs[0].MultiplePopPaths {
		lineage, err = nextDiagnosticParserCoreCleanPathLineage(nextCleanPathLineage)
		if err != nil {
			return nil, outputs, err
		}
		if err := compact.RecordReductionLineageOwned(owner, outputs, lineage); err != nil {
			return nil, outputs, err
		}
	}
	for outputIndex, output := range outputs {
		var set core.AlternativeSet
		var setBlended bool
		if lineage != 0 {
			// Establishment/extend (spec.b4b-alternative-set.v2 section
			// 3.4): branch is this output's index within outputs, agreeing
			// with RecordReductionLineageOwned's identical iteration above.
			set = core.NewAlternativeSetMember(lineage, uint16(outputIndex))
		}
		if output.HistoricalBoundaryProvenance == core.HistoricalBoundaryConverged &&
			output.HistoricalAlternativeSet.Len() != 0 {
			// Fold-class union (spec.b4b-alternative-set.v2 section 3.4):
			// see applyGenericReductionOwned's identical dead-node-import site.
			incomparable := compact.AlternativeSetIncomparable(set, output.HistoricalAlternativeSet)
			compact.UnionAlternativeSet(&set, output.HistoricalAlternativeSet)
			setBlended = setBlended || output.HistoricalBlended || incomparable
		}
		switch output.Freshness {
		case core.ReductionUnchanged:
			dst = append(dst, diagnosticParserCoreActionOutput{
				head: output.Head, freshness: output.Freshness,
				cleanPathRank: output.CleanPathRank, cleanPathLineage: lineage, cleanPathSet: set,
				cleanPathBlended: setBlended, dropCohortRefs: output.DropCohortRefs,
			})
		case core.ReductionNew, core.ReductionUpdated:
			dst = append(dst, diagnosticParserCoreActionOutput{
				head: output.Head, freshness: output.Freshness,
				cleanPathRank: output.CleanPathRank, cleanPathLineage: lineage, cleanPathSet: set,
				cleanPathBlended: setBlended, dropCohortRefs: output.DropCohortRefs,
			})
		default:
			return nil, outputs, errors.New("parser-core phase zero: reduction returned invalid freshness")
		}
	}
	return dst, outputs, nil
}

func rootParserCoreAction(action core.Action) ParseAction {
	var actionType ParseActionType
	switch action.Type {
	case core.ActionShift:
		actionType = ParseActionShift
	case core.ActionReduce:
		actionType = ParseActionReduce
	case core.ActionAccept:
		actionType = ParseActionAccept
	case core.ActionRecover:
		actionType = ParseActionRecover
	default:
		panic("parser-core phase zero: impossible compact action type")
	}
	return ParseAction{
		Type: actionType, State: StateID(action.State), Symbol: Symbol(action.Symbol),
		ChildCount: action.ChildCount, DynamicPrecedence: action.DynamicPrecedence,
		ProductionID: action.ProductionID, Extra: action.Extra,
		ExtraChain: action.ExtraChain, Repetition: action.Repetition,
	}
}
