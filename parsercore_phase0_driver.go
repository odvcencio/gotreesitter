//go:build gts_parsercorephase0

package gotreesitter

import (
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

type DiagnosticParserCoreBoundaryKind string

const (
	DiagnosticParserCoreFirstFork                  DiagnosticParserCoreBoundaryKind = "first_fork"
	DiagnosticParserCoreExtra                      DiagnosticParserCoreBoundaryKind = "extra"
	DiagnosticParserCoreExtraChain                 DiagnosticParserCoreBoundaryKind = "extra_chain"
	DiagnosticParserCoreNoAction                   DiagnosticParserCoreBoundaryKind = "no_action"
	DiagnosticParserCoreRecovery                   DiagnosticParserCoreBoundaryKind = "recovery"
	DiagnosticParserCoreAccept                     DiagnosticParserCoreBoundaryKind = "accept_without_materialization"
	DiagnosticParserCoreCap                        DiagnosticParserCoreBoundaryKind = "cap"
	DiagnosticParserCoreIdentity                   DiagnosticParserCoreBoundaryKind = "identity"
	DiagnosticParserCoreRoute                      DiagnosticParserCoreBoundaryKind = "unsupported_route"
	DiagnosticParserCoreElectionBarrier            DiagnosticParserCoreBoundaryKind = "multi_state_re_election"
	DiagnosticParserCoreSingleStateContinuation    DiagnosticParserCoreBoundaryKind = "single_state_continuation_before_dispatch"
	DiagnosticParserCoreSubsequentConflictBoundary DiagnosticParserCoreBoundaryKind = "subsequent_conflict_before_execution"
	DiagnosticParserCoreCohortCondensed            DiagnosticParserCoreBoundaryKind = "cohort_condensed_before_election"
	DiagnosticParserCoreDotConflictFanoutBoundary  DiagnosticParserCoreBoundaryKind = "dot_conflict_fanout_before_cached_shift"
	DiagnosticParserCoreCachedDotClosureBoundary   DiagnosticParserCoreBoundaryKind = "cached_dot_closed_before_edits_shift"
)

type DiagnosticParserCorePrefixOptions struct {
	Recovery         bool
	Retry            bool
	Incremental      bool
	IncludedRanges   bool
	GenericScheduler bool
	MaxDispatches    uint64
	MaxTokens        uint64
	Limits           core.Limits
}

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

type DiagnosticParserCoreForkAction struct {
	Ordinal     int
	Action      ParseAction
	BranchOrder uint64
}

type DiagnosticParserCoreForkBoundary struct {
	State      StateID
	ByteOffset uint32
	ExactPaths uint64
}

type DiagnosticParserCoreHeaderReceipt struct {
	CreationSeq uint64
	State       StateID
	ByteOffset  uint32
	Shifted     bool
	Accepted    bool
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

// DiagnosticParserCoreOracleCondenseResolution records the one C-oracle-pinned
// comparison that phase zero may apply at the frozen state-254 no-action cell.
// C charges the paused version one skipped-tree error cost, keeps the zero-cost
// shifted sibling, and does not resume the paused version. The paused graph
// remains immutable; only the diagnostic scheduler agenda drops it.
type DiagnosticParserCoreOracleCondenseResolution struct {
	Paused                 DiagnosticParserCoreHeaderReceipt
	Preserved              DiagnosticParserCoreHeaderReceipt
	Lookahead              Token
	PrecedingScannerAfter  DiagnosticParserCoreScannerCheckpoint
	PausedEffectiveCost    uint32
	PreservedEffectiveCost uint32
	PausedDropped          bool
	PausedResumed          bool
	OraclePinned           bool
}

// DiagnosticParserCoreContinuationElection records the single authenticated
// production-DFA election admitted after the frozen no-action resolution.
type DiagnosticParserCoreContinuationElection struct {
	State                StateID
	ByteOffset           uint32
	ElectionIndex        int
	ExpectedBefore       DiagnosticParserCoreScannerCheckpoint
	ActualBefore         DiagnosticParserCoreScannerCheckpoint
	CheckpointContinuous bool
	Token                Token
	// SchedulerHeader is the sole agenda header for the new token epoch. Its
	// Shifted reset is scheduler state only; phase zero does not migrate or
	// rewrite the persistent compact graph head.
	SchedulerHeader DiagnosticParserCoreHeaderReceipt
	HandoffBoundary DiagnosticParserCoreBoundaryKind
}

// DiagnosticParserCoreSubsequentConflict records a later multi-action cell
// without executing it or replacing the authenticated first-fork evidence.
type DiagnosticParserCoreSubsequentConflictReceipt struct {
	State          StateID
	ByteOffset     uint32
	Header         DiagnosticParserCoreHeaderReceipt
	Token          Token
	Actions        []ParseAction
	ElectionIndex  int
	Score          int64
	BranchOrder    uint64
	HasBranchOrder bool
}

type DiagnosticParserCoreLaterForkBoundary struct {
	Header         DiagnosticParserCoreHeaderReceipt
	Score          int64
	BranchOrder    uint64
	HasBranchOrder bool
}

// DiagnosticParserCoreLaterForkExecution records the second authenticated
// conflict independently from the immutable first-fork receipt.
type DiagnosticParserCoreLaterForkExecution struct {
	Token                 Token
	ElectionIndex         int
	BranchOrderBefore     uint64
	BranchOrderAfter      uint64
	NextCreationSeqBefore uint64
	NextCreationSeqAfter  uint64
	Round                 DiagnosticParserCoreDispatchRound
	Boundaries            []DiagnosticParserCoreLaterForkBoundary
	ExactPaths            uint64
	ClosedBoundaries      []DiagnosticParserCoreLaterForkBoundary
	ClosedExactPaths      uint64
}

type DiagnosticParserCoreOrderedElection struct {
	Index                int
	Before               []DiagnosticParserCoreHeaderReceipt
	States               []StateID
	ExpectedBefore       DiagnosticParserCoreScannerCheckpoint
	ActualBefore         DiagnosticParserCoreScannerCheckpoint
	CheckpointContinuous bool
	Token                Token
	Reset                []DiagnosticParserCoreHeaderReceipt
	Boundaries           []DiagnosticParserCoreLaterForkBoundary
	BranchOrder          uint64
	NextCreationSeq      uint64
}

type DiagnosticParserCoreCohortCondense struct {
	Before                 []DiagnosticParserCoreHeaderReceipt
	BeforeBoundaries       []DiagnosticParserCoreLaterForkBoundary
	Paused                 DiagnosticParserCoreHeaderReceipt
	Preserved              DiagnosticParserCoreHeaderReceipt
	After                  []DiagnosticParserCoreHeaderReceipt
	AfterBoundaries        []DiagnosticParserCoreLaterForkBoundary
	Token                  Token
	ElectionIndex          int
	PausedOpenRecoveryCost uint32
	PausedSkippedTreeCost  uint32
	PausedEffectiveCost    uint32
	PreservedEffectiveCost uint32
	PausedDropped          bool
	PausedResumed          bool
	OraclePinned           bool
	Executed               bool
}

type DiagnosticParserCorePostCondenseContinuation struct {
	Before                     DiagnosticParserCoreHeaderReceipt
	BeforeBoundary             DiagnosticParserCoreLaterForkBoundary
	ContinuationElectionIndex  int
	ContinuationExpectedBefore DiagnosticParserCoreScannerCheckpoint
	ContinuationElection       DiagnosticParserCoreElection
	ContinuationReset          DiagnosticParserCoreHeaderReceipt
	ShiftRound                 DiagnosticParserCoreDispatchRound
	ShiftedBoundary            DiagnosticParserCoreLaterForkBoundary
	ConflictElectionIndex      int
	ConflictExpectedBefore     DiagnosticParserCoreScannerCheckpoint
	ConflictElection           DiagnosticParserCoreElection
	ConflictReset              DiagnosticParserCoreHeaderReceipt
	Conflict                   DiagnosticParserCoreSubsequentConflictReceipt
	GlobalBranchOrder          uint64
	NextCreationSeq            uint64
	Dispatches                 uint64
}

type diagnosticParserCorePostCondenseExecution struct {
	receipt     *DiagnosticParserCorePostCondenseContinuation
	finalHeader diagnosticParserCoreHeader
}

type diagnosticParserCoreDotConflictExecution struct {
	receipt *DiagnosticParserCoreDotConflictFanout
	headers []diagnosticParserCoreHeader
}

type diagnosticParserCoreCachedDotFaultPoint uint8

const (
	diagnosticParserCoreCachedDotFaultAfterCohort diagnosticParserCoreCachedDotFaultPoint = iota + 1
	diagnosticParserCoreCachedDotFaultAfterElection
	diagnosticParserCoreCachedDotFaultAfterRollback
)

var diagnosticParserCoreCachedDotFaultHook func(
	diagnosticParserCoreCachedDotFaultPoint,
	*core.Core,
	[]diagnosticParserCoreHeader,
	[]diagnosticParserCoreHeader,
	*dfaTokenSource,
) error

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
	Terminal          bool
}

type DiagnosticParserCorePackedConvergence struct {
	ConflictRound              DiagnosticParserCoreDispatchRound
	PostConflictBoundaries     []DiagnosticParserCoreLaterForkBoundary
	SameTokenRounds            []DiagnosticParserCoreDispatchRound
	ClosedParenBoundaries      []DiagnosticParserCoreLaterForkBoundary
	ElectionIndex              int
	ElectionExpectedBefore     DiagnosticParserCoreScannerCheckpoint
	Election                   DiagnosticParserCoreElection
	ElectionBefore             []DiagnosticParserCoreHeaderReceipt
	ElectionBeforeBoundaries   []DiagnosticParserCoreLaterForkBoundary
	ElectionReset              []DiagnosticParserCoreHeaderReceipt
	ShiftRounds                []DiagnosticParserCoreDispatchRound
	BeforeCanonical            []DiagnosticParserCoreHeaderReceipt
	Packed                     DiagnosticParserCoreHeaderReceipt
	Derivations                []DiagnosticParserCorePackedDerivation
	TerminalSubtreesBefore     uint32
	TerminalSubtreesAfter      uint32
	TerminalNodesBefore        uint32
	TerminalNodesAfter         uint32
	TerminalLinksBefore        uint32
	TerminalLinksAfter         uint32
	TerminalPayloadAllocations uint32
	TerminalPayloadsPerCohort  uint32
	TerminalPayloadViews       []DiagnosticParserCoreTerminalPayloadView
	SemanticTerminalIdentities uint32
	DistinctTerminalPayloads   uint32
	DuplicateTerminalPayloads  uint32
	GlobalBranchOrder          uint64
	NextCreationSeq            uint64
	Dispatches                 uint64
}

type DiagnosticParserCoreHeaderPathReceipt struct {
	Header      DiagnosticParserCoreHeaderReceipt
	Derivations []DiagnosticParserCorePackedDerivation
}

type DiagnosticParserCoreDotConflictFanout struct {
	ElectionIndex             int
	ElectionExpectedBefore    DiagnosticParserCoreScannerCheckpoint
	Election                  DiagnosticParserCoreElection
	ElectionBefore            DiagnosticParserCoreHeaderPathReceipt
	ElectionReset             DiagnosticParserCoreHeaderReceipt
	ConflictRound             DiagnosticParserCoreDispatchRound
	Headers                   []DiagnosticParserCoreHeaderPathReceipt
	LogicalPaths              uint64
	NodesBefore               uint32
	NodesAfter                uint32
	LinksBefore               uint32
	LinksAfter                uint32
	SubtreesBefore            uint32
	SubtreesAfter             uint32
	ChildrenBefore            uint32
	ChildrenAfter             uint32
	NewPayloadViews           []DiagnosticParserCoreTerminalPayloadView
	ReductionParentPayloadIDs []uint32
	SemanticReductionParents  uint32
	DistinctReductionParents  uint32
	DuplicateReductionParents uint32
	GlobalBranchOrder         uint64
	NextCreationSeq           uint64
	Dispatches                uint64
}

// DiagnosticParserCoreCachedDotClosure records the partial-cohort closure of
// the elected dot. Only the two unshifted primary headers are dispatched; the
// already-consumed secondary header is retained while the complete frontier
// is canonicalized and authenticated before the next election is published.
type DiagnosticParserCoreCachedDotClosure struct {
	Before                     []DiagnosticParserCoreHeaderPathReceipt
	RunnableBefore             []DiagnosticParserCoreHeaderReceipt
	RetainedBefore             DiagnosticParserCoreHeaderPathReceipt
	ShiftRound                 DiagnosticParserCoreDispatchRound
	BeforeCanonical            []DiagnosticParserCoreHeaderReceipt
	Headers                    []DiagnosticParserCoreHeaderPathReceipt
	LogicalPaths               uint64
	NodesBefore                uint32
	NodesAfter                 uint32
	LinksBefore                uint32
	LinksAfter                 uint32
	SubtreesBefore             uint32
	SubtreesAfter              uint32
	ChildrenBefore             uint32
	ChildrenAfter              uint32
	TerminalPayload            DiagnosticParserCoreTerminalPayloadView
	PrimaryTerminalPayloadIDs  []uint32
	RetainedTerminalPayloadIDs []uint32
	GlobalBranchOrder          uint64
	NextCreationSeq            uint64
	Dispatches                 uint64
	ElectionIndex              int
	ElectionExpectedBefore     DiagnosticParserCoreScannerCheckpoint
	ElectionBefore             []DiagnosticParserCoreHeaderPathReceipt
	Election                   DiagnosticParserCoreElection
	NextActions                []DiagnosticParserCoreRoundAction
}

// DiagnosticParserCoreGenericWork records semantic scheduler work separately
// from the compact core's physical arena storage.
type DiagnosticParserCoreGenericWork struct {
	Passes            uint64
	ActionLookups     uint64
	Dispatches        uint64
	Conflicts         uint64
	ConflictActions   uint64
	Forks             uint64
	ConflictHeads     uint64
	Reductions        uint64
	OrdinaryShifts    uint64
	OrdinaryCohorts   uint64
	NoActionDrops     uint64
	Elections         uint64
	Canonicalizations uint64
	PeakHeaders       uint64
}

// DiagnosticParserCoreGenericConflict records one table-driven conflict cell.
// Actions preserve execution order: secondary ordinals first, then primary.
type DiagnosticParserCoreGenericConflictArm struct {
	Ordinal     int
	BranchOrder uint64
	Outputs     []DiagnosticParserCoreHeaderReceipt
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

// DiagnosticParserCoreGenericScheduler records the committed suffix beginning
// with the already-elected edits token. Elections contains only elections made
// by this scheduler; the edits election remains owned by CachedDotClosure.
type DiagnosticParserCoreGenericScheduler struct {
	StartElectionIndex int
	StartToken         Token
	StartHeaders       []DiagnosticParserCoreHeaderPathReceipt
	Rounds             []DiagnosticParserCoreDispatchRound
	Conflicts          []DiagnosticParserCoreGenericConflict
	Elections          []DiagnosticParserCoreElection
	NoActionDrops      []DiagnosticParserCoreGenericNoActionDrop
	Stop               DiagnosticParserCoreGenericStop
	Tokens             uint64
	Dispatches         uint64
	GlobalBranchOrder  uint64
	NextCreationSeq    uint64
}

type DiagnosticParserCoreExtraShift struct {
	State          StateID
	Token          Token
	Action         ParseAction
	EffectiveState StateID
}

type DiagnosticParserCoreReductionAttempt struct {
	State     StateID
	Lookahead Token
	Action    ParseAction
	Applied   bool
}

type DiagnosticParserCorePrefixResult struct {
	Boundary                 DiagnosticParserCoreBoundaryKind
	Detail                   string
	Dispatches               uint64
	Tokens                   uint64
	State                    StateID
	Lookahead                Token
	ForkActions              []DiagnosticParserCoreForkAction
	ForkBoundaryReceipts     []DiagnosticParserCoreForkBoundary
	ForkBoundaries           int
	ForkLogicalPaths         uint64
	ExtraShifts              []DiagnosticParserCoreExtraShift
	ReductionAttempts        []DiagnosticParserCoreReductionAttempt
	SameTokenRounds          []DiagnosticParserCoreDispatchRound
	LastBranchOrder          uint64
	OracleCondenseResolution *DiagnosticParserCoreOracleCondenseResolution
	ContinuationElection     *DiagnosticParserCoreContinuationElection
	SubsequentConflict       *DiagnosticParserCoreSubsequentConflictReceipt
	LaterForkExecution       *DiagnosticParserCoreLaterForkExecution
	OrderedElections         []DiagnosticParserCoreOrderedElection
	CohortRounds             []DiagnosticParserCoreDispatchRound
	CohortCondense           *DiagnosticParserCoreCohortCondense
	PostCondenseContinuation *DiagnosticParserCorePostCondenseContinuation
	PackedConvergence        *DiagnosticParserCorePackedConvergence
	DotConflictFanout        *DiagnosticParserCoreDotConflictFanout
	CachedDotClosure         *DiagnosticParserCoreCachedDotClosure
	GenericScheduler         *DiagnosticParserCoreGenericScheduler
	Elections                []DiagnosticParserCoreElection
	SourceSHA256             [32]byte
	GrammarBlobSHA256        [32]byte
	Grammar                  string
	ExactRootDFA             bool
	Materialized             bool
}

type diagnosticParserCoreDecline struct {
	boundary DiagnosticParserCoreBoundaryKind
	detail   string
}

//go:embed grammars/grammar_blobs/go.bin
var parserCoreCertifiedGoBlob []byte

func (e *diagnosticParserCoreDecline) Error() string { return string(e.boundary) + ": " + e.detail }

type parserCoreRootTables struct{ parser *Parser }

func (a parserCoreRootTables) Actions(state core.StateID, symbol core.Symbol) ([]core.Action, error) {
	p := a.parser
	index := p.lookupActionIndex(StateID(state), Symbol(symbol))
	if index == 0 {
		return nil, nil
	}
	if int(index) >= len(p.language.ParseActions) {
		return nil, errors.New("parser-core phase zero: canonical action index out of range")
	}
	actions := p.language.ParseActions[index].Actions
	out := make([]core.Action, len(actions))
	for i, action := range actions {
		converted, err := parserCoreAction(action)
		if err != nil {
			return nil, err
		}
		out[i] = converted
	}
	return out, nil
}

func (a parserCoreRootTables) Goto(state core.StateID, symbol core.Symbol) (core.StateID, error) {
	return core.StateID(a.parser.lookupGoto(StateID(state), Symbol(symbol))), nil
}

func (a parserCoreRootTables) ProductionFields(productionID uint16, childCount int) ([]core.FieldMapEntry, error) {
	p := a.parser
	fieldIDs, inherited := buildFieldPlanForProduction(p.language, childCount, productionID)
	var out []core.FieldMapEntry
	for index, fieldID := range fieldIDs {
		if fieldID == 0 {
			continue
		}
		out = append(out, core.FieldMapEntry{FieldID: core.FieldID(fieldID), ChildIndex: uint8(index), Inherited: inherited[index]})
	}
	return out, nil
}

func (a parserCoreRootTables) ProductionAliases(productionID uint16, childCount int) ([]core.Symbol, error) {
	lang := a.parser.language
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

func parserCoreCheckpoint(bytes []byte) DiagnosticParserCoreScannerCheckpoint {
	return DiagnosticParserCoreScannerCheckpoint{Length: len(bytes), SHA256: sha256.Sum256(bytes)}
}

// DiagnosticParseParserCorePrefix independently schedules the compact core
// from exact production DFA/scanner elections through the first authenticated
// fork and its frozen oracle-condense continuation. The authenticated route
// currently closes the later cached-dot primary cohort, elects the following
// ordered token, and stops before dispatch. Earlier unsupported boundaries
// remain fail-closed. It never calls the production parser.
func DiagnosticParseParserCorePrefix(scanner ExternalScanner, source []byte, options DiagnosticParserCorePrefixOptions) (DiagnosticParserCorePrefixResult, error) {
	result := DiagnosticParserCorePrefixResult{SourceSHA256: sha256.Sum256(source)}
	lang, err := authenticatedParserCoreGoLanguage(scanner)
	if err != nil {
		result.Boundary, result.Detail = DiagnosticParserCoreIdentity, err.Error()
		return result, &diagnosticParserCoreDecline{boundary: result.Boundary, detail: result.Detail}
	}
	result.Grammar = lang.Name
	result.ExactRootDFA = true
	result.GrammarBlobSHA256 = sha256.Sum256(parserCoreCertifiedGoBlob)
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
	tables := parserCoreRootTables{parser: parser}
	compact, err := core.New(tables, options.Limits)
	if err != nil {
		return result, err
	}
	head, err := compact.Seed(core.StateID(lang.InitialState), 0)
	if err != nil {
		return result, err
	}
	state := lang.InitialState
	tokenSource := parser.acquireParserDFATokenSource(source)
	if tokenSource == nil {
		return result, errors.New("parser-core phase zero: production DFA unavailable")
	}
	defer tokenSource.Close()
	var token Token
	haveToken := false
	outerCreationSeq := uint64(0)
	outerBranchOrder := uint64(0)
	outerNextCreationSeq := uint64(1)
	var scannerScratch []byte
	for result.Dispatches < options.MaxDispatches {
		if !haveToken {
			tokenSource.SetParserState(state)
			tokenSource.SetGLRStates(nil)
			before := append([]byte(nil), tokenSource.captureExternalScannerStateInto(&scannerScratch)...)
			token = tokenSource.Next()
			after := append([]byte(nil), tokenSource.captureExternalScannerStateInto(&scannerScratch)...)
			current, currentStart, currentEnd, currentValid := currentExternalScannerCheckpoint(tokenSource)
			result.Tokens++
			if result.Tokens > options.MaxTokens {
				result.Boundary, result.Detail = DiagnosticParserCoreCap, "token cap"
				return result, &diagnosticParserCoreDecline{boundary: result.Boundary, detail: result.Detail}
			}
			result.Elections = append(result.Elections, DiagnosticParserCoreElection{
				States: []StateID{state}, Token: token,
				ScannerBefore: parserCoreCheckpoint(before), ScannerAfter: parserCoreCheckpoint(after),
				CurrentCheckpointValid: currentValid,
				CurrentCheckpointStart: parserCoreCheckpoint(current.start),
				CurrentCheckpointEnd:   parserCoreCheckpoint(current.end),
				CurrentCheckpointBytes: [2]uint32{currentStart, currentEnd},
			})
			if err := compact.BeginFrontier(); err != nil {
				return result, err
			}
			compact.SetPhaseCheckpoint(sha256.Sum256(after))
			haveToken = true
		}
		result.Dispatches++
		result.State, result.Lookahead = state, token
		actions, err := compact.Actions(core.StateID(state), core.Symbol(token.Symbol))
		if err != nil {
			return result, err
		}
		if err := validateDiagnosticParserCoreCell(token, actions); err != nil {
			if setDiagnosticParserCoreBoundaryError(&result, err) {
				return result, &diagnosticParserCoreDecline{boundary: result.Boundary, detail: result.Detail}
			}
			return result, err
		}
		if len(actions) > 1 {
			if len(result.ForkActions) != 0 {
				if result.LaterForkExecution != nil {
					receipt, receiptErr := diagnosticParserCoreSubsequentConflictReceipt(
						compact, head, outerCreationSeq, token, actions, len(result.Elections)-1,
						result.Elections[len(result.Elections)-1].ScannerAfter.SHA256,
					)
					if receiptErr != nil {
						return result, receiptErr
					}
					result.SubsequentConflict = receipt
					result.Boundary = DiagnosticParserCoreSubsequentConflictBoundary
					result.Detail = "third multi-action cell reached before execution"
					return result, &diagnosticParserCoreDecline{boundary: result.Boundary, detail: result.Detail}
				}

				checkpoint := result.Elections[len(result.Elections)-1].ScannerAfter.SHA256
				initial := diagnosticParserCoreHeader{head: head, creationSeq: outerCreationSeq, checkpoint: checkpoint}
				headers, round, nextOrder, nextSeq, conflictErr := executeDiagnosticParserCoreConflict(
					compact, initial, 0, token, actions, outerBranchOrder, outerNextCreationSeq,
				)
				if conflictErr != nil {
					if setDiagnosticParserCoreBoundaryError(&result, conflictErr) {
						return result, &diagnosticParserCoreDecline{boundary: result.Boundary, detail: result.Detail}
					}
					return result, conflictErr
				}
				round.Index = 0
				boundaries, exactPaths, receiptErr := diagnosticParserCoreLaterForkBoundaries(compact, headers)
				if receiptErr != nil {
					return result, receiptErr
				}
				result.LaterForkExecution = &DiagnosticParserCoreLaterForkExecution{
					Token: token, ElectionIndex: len(result.Elections) - 1,
					BranchOrderBefore: outerBranchOrder, BranchOrderAfter: nextOrder,
					NextCreationSeqBefore: outerNextCreationSeq, NextCreationSeqAfter: nextSeq,
					Round: round, Boundaries: boundaries, ExactPaths: exactPaths,
				}
				recordDiagnosticParserCoreAppliedReductions(&result, token, round.Actions)
				result.LastBranchOrder = nextOrder
				var resume diagnosticParserCoreOuterResume
				if continueErr := continueDiagnosticParserCoreSameToken(
					compact, tokenSource, &scannerScratch, token, headers, nextOrder, nextSeq, options, &result, &resume,
				); continueErr != nil {
					if setDiagnosticParserCoreBoundaryError(&result, continueErr) {
						return result, &diagnosticParserCoreDecline{boundary: result.Boundary, detail: result.Detail}
					}
					return result, continueErr
				}
				if !resume.ready {
					return result, errors.New("parser-core phase zero: later conflict continuation returned without boundary or resume")
				}
				head, state, token, haveToken = resume.head, resume.state, resume.token, true
				outerCreationSeq, outerBranchOrder, outerNextCreationSeq = resume.creationSeq, resume.branchOrder, resume.nextCreationSeq
				continue
			}
			checkpoint := result.Elections[len(result.Elections)-1].ScannerAfter.SHA256
			initial := diagnosticParserCoreHeader{head: head, creationSeq: 0, checkpoint: checkpoint}
			headers, round, branchOrder, nextSeq, err := executeDiagnosticParserCoreConflict(compact, initial, 0, token, actions, 0, 1)
			if err != nil {
				if setDiagnosticParserCoreBoundaryError(&result, err) {
					return result, &diagnosticParserCoreDecline{boundary: result.Boundary, detail: result.Detail}
				}
				return result, err
			}
			result.SameTokenRounds = append(result.SameTokenRounds, round)
			result.SameTokenRounds[len(result.SameTokenRounds)-1].Index = 0
			recordDiagnosticParserCoreAppliedReductions(&result, token, round.Actions)
			result.LastBranchOrder = branchOrder
			result.ForkActions = make([]DiagnosticParserCoreForkAction, len(round.Actions))
			for index, action := range round.Actions {
				result.ForkActions[index] = DiagnosticParserCoreForkAction{Ordinal: action.Ordinal, Action: action.Action, BranchOrder: action.BranchOrder}
			}
			if err := recordDiagnosticParserCoreFirstFork(compact, headers, &result); err != nil {
				return result, err
			}
			var resume diagnosticParserCoreOuterResume
			if err := continueDiagnosticParserCoreSameToken(compact, tokenSource, &scannerScratch, token, headers, branchOrder, nextSeq, options, &result, &resume); err != nil {
				if setDiagnosticParserCoreBoundaryError(&result, err) {
					return result, &diagnosticParserCoreDecline{boundary: result.Boundary, detail: result.Detail}
				}
				return result, err
			}
			if !resume.ready {
				return result, errors.New("parser-core phase zero: same-lookahead scheduler returned without a boundary or outer resume")
			}
			head, state, token, haveToken = resume.head, resume.state, resume.token, true
			outerCreationSeq, outerBranchOrder, outerNextCreationSeq = resume.creationSeq, resume.branchOrder, resume.nextCreationSeq
			continue
		}
		action := actions[0]
		switch action.Type {
		case core.ActionShift:
			beforeState := state
			head, err = compact.Shift(head, core.Symbol(token.Symbol), 0, core.Token{Symbol: core.Symbol(token.Symbol), StartByte: token.StartByte, EndByte: token.EndByte, Extra: action.Extra}, core.ForkOrder{})
			if err != nil {
				return result, err
			}
			compactState, _, boundaryErr := compact.Boundary(head)
			if boundaryErr != nil {
				return result, boundaryErr
			}
			state = StateID(compactState)
			if action.Extra {
				result.ExtraShifts = append(result.ExtraShifts, DiagnosticParserCoreExtraShift{
					State: beforeState, Token: token, Action: rootParserCoreAction(action), EffectiveState: state,
				})
			}
			haveToken = false
		case core.ActionReduce:
			result.ReductionAttempts = append(result.ReductionAttempts, DiagnosticParserCoreReductionAttempt{
				State: state, Lookahead: token, Action: rootParserCoreAction(action),
			})
			frontier, reduceErr := compact.Reduce(head, core.Symbol(token.Symbol), 0, core.ForkOrder{})
			if reduceErr != nil {
				if core.IsDecline(reduceErr, core.DeclineExtras) {
					result.Boundary, result.Detail = DiagnosticParserCoreExtra, reduceErr.Error()
					return result, &diagnosticParserCoreDecline{boundary: result.Boundary, detail: result.Detail}
				}
				return result, reduceErr
			}
			if len(frontier) != 1 {
				return result, errors.New("parser-core phase zero: clean prefix reduction produced multiple boundaries")
			}
			result.ReductionAttempts[len(result.ReductionAttempts)-1].Applied = true
			head = frontier[0]
			compactState, _, boundaryErr := compact.Boundary(head)
			if boundaryErr != nil {
				return result, boundaryErr
			}
			state = StateID(compactState)
		case core.ActionRecover:
			result.Boundary, result.Detail = DiagnosticParserCoreRecovery, "recover action"
			return result, &diagnosticParserCoreDecline{boundary: result.Boundary, detail: result.Detail}
		case core.ActionAccept:
			result.Boundary, result.Detail = DiagnosticParserCoreAccept, "compact tree is not materialized as a public tree"
			return result, &diagnosticParserCoreDecline{boundary: result.Boundary, detail: result.Detail}
		default:
			return result, errors.New("parser-core phase zero: unknown action")
		}
	}
	result.Boundary, result.Detail = DiagnosticParserCoreCap, "dispatch cap"
	return result, &diagnosticParserCoreDecline{boundary: result.Boundary, detail: result.Detail}
}

type diagnosticParserCoreHeader struct {
	head        core.Head
	creationSeq uint64
	shifted     bool
	accepted    bool
	checkpoint  [32]byte
}

type diagnosticParserCoreOuterResume struct {
	head            core.Head
	state           StateID
	token           Token
	creationSeq     uint64
	branchOrder     uint64
	nextCreationSeq uint64
	ready           bool
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
	return DiagnosticParserCoreHeaderReceipt{
		CreationSeq: header.creationSeq,
		State:       StateID(state),
		ByteOffset:  byteOffset,
		Shifted:     header.shifted,
		Accepted:    header.accepted,
		ExactPaths:  stats.CurrentExactPaths,
		Checkpoint:  header.checkpoint,
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

func validateDiagnosticParserCoreCell(token Token, actions []core.Action) error {
	if token.NoLookahead {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "no-lookahead tokens require production recovery semantics"}
	}
	if len(actions) == 0 {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreNoAction, detail: "canonical cell has no action"}
	}
	for _, action := range actions {
		if action.Repetition {
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
	primaries     []diagnosticParserCoreHeader
	secondaries   []diagnosticParserCoreHeader
	secondaryArms [][]diagnosticParserCoreHeader
	round         DiagnosticParserCoreDispatchRound
	branchOrder   uint64
	nextSeq       uint64
}

// executeDiagnosticParserCoreConflict executes one complete conflict cell
// transactionally. Returned headers are ordered primary frontier first, then
// secondary clones in action/output order. The first primary retains the
// incoming creation sequence; every secondary output and every additional
// primary boundary receives a deterministic new sequence.
func executeDiagnosticParserCoreConflict(
	compact *core.Core,
	incoming diagnosticParserCoreHeader,
	headerIndex int,
	token Token,
	actions []core.Action,
	branchOrder uint64,
	nextSeq uint64,
) ([]diagnosticParserCoreHeader, DiagnosticParserCoreDispatchRound, uint64, uint64, error) {
	execution, err := executeDiagnosticParserCoreConflictDetailed(
		compact, incoming, headerIndex, token, actions, branchOrder, nextSeq,
	)
	if err != nil {
		return nil, DiagnosticParserCoreDispatchRound{}, branchOrder, nextSeq, err
	}
	headers := make([]diagnosticParserCoreHeader, 0, len(execution.primaries)+len(execution.secondaries))
	headers = append(headers, execution.primaries...)
	headers = append(headers, execution.secondaries...)
	return headers, execution.round, execution.branchOrder, execution.nextSeq, nil
}

func executeDiagnosticParserCoreConflictDetailed(
	compact *core.Core,
	incoming diagnosticParserCoreHeader,
	headerIndex int,
	token Token,
	actions []core.Action,
	branchOrder uint64,
	nextSeq uint64,
) (diagnosticParserCoreConflictExecution, error) {
	before, err := diagnosticParserCoreHeaderReceipt(compact, incoming)
	if err != nil {
		return diagnosticParserCoreConflictExecution{}, err
	}
	if err := validateDiagnosticParserCoreCell(token, actions); err != nil {
		return diagnosticParserCoreConflictExecution{}, err
	}
	if len(actions) < 2 {
		return diagnosticParserCoreConflictExecution{}, errors.New("parser-core phase zero: conflict executor requires multiple actions")
	}
	secondaryCount := uint64(len(actions) - 1)
	if secondaryCount > math.MaxUint64-branchOrder {
		return diagnosticParserCoreConflictExecution{}, errors.New("parser-core phase zero: conflict branch order overflow")
	}
	if secondaryCount > math.MaxUint64-nextSeq {
		return diagnosticParserCoreConflictExecution{}, errors.New("parser-core phase zero: conflict creation sequence overflow")
	}

	trialOrder, trialSeq := branchOrder, nextSeq
	var primaries []diagnosticParserCoreHeader
	var secondaries []diagnosticParserCoreHeader
	var secondaryArms [][]diagnosticParserCoreHeader
	var receipts []DiagnosticParserCoreRoundAction
	err = compact.ApplyAtomic(func() error {
		for ordinal := 1; ordinal < len(actions); ordinal++ {
			trialOrder++
			heads, applyErr := applyParserCorePrefixAction(compact, incoming.head, token, actions[ordinal], ordinal, core.ForkOrder{Present: true, Value: trialOrder})
			if applyErr != nil {
				return applyErr
			}
			if len(heads) == 0 {
				return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "conflict secondary produced no scheduler boundary"}
			}
			arm := make([]diagnosticParserCoreHeader, 0, len(heads))
			for _, head := range heads {
				if trialSeq == math.MaxUint64 {
					return errors.New("parser-core phase zero: conflict creation sequence overflow")
				}
				arm = append(arm, diagnosticParserCoreHeader{
					head: head, creationSeq: trialSeq, shifted: actions[ordinal].Type == core.ActionShift,
					checkpoint: incoming.checkpoint,
				})
				trialSeq++
			}
			secondaryArms = append(secondaryArms, arm)
			secondaries = append(secondaries, arm...)
			receipts = append(receipts, DiagnosticParserCoreRoundAction{
				HeaderIndex: headerIndex, State: before.State, ByteOffset: before.ByteOffset,
				Ordinal: ordinal, Action: rootParserCoreAction(actions[ordinal]), BranchOrder: trialOrder,
			})
		}
		heads, applyErr := applyParserCorePrefixAction(compact, incoming.head, token, actions[0], 0, core.ForkOrder{})
		if applyErr != nil {
			return applyErr
		}
		if len(heads) == 0 {
			return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "conflict primary produced no scheduler boundary"}
		}
		primaries = make([]diagnosticParserCoreHeader, len(heads))
		for index, head := range heads {
			primary := incoming
			primary.head = head
			primary.shifted = actions[0].Type == core.ActionShift
			if index > 0 {
				if trialSeq == math.MaxUint64 {
					return errors.New("parser-core phase zero: conflict creation sequence overflow")
				}
				primary.creationSeq = trialSeq
				trialSeq++
			}
			primaries[index] = primary
		}
		receipts = append(receipts, DiagnosticParserCoreRoundAction{
			HeaderIndex: headerIndex, State: before.State, ByteOffset: before.ByteOffset,
			Ordinal: 0, Action: rootParserCoreAction(actions[0]),
		})
		return nil
	})
	if err != nil {
		return diagnosticParserCoreConflictExecution{}, err
	}

	headers := append(primaries, secondaries...)
	after, err := diagnosticParserCoreHeaderReceipts(compact, headers)
	if err != nil {
		return diagnosticParserCoreConflictExecution{}, err
	}
	return diagnosticParserCoreConflictExecution{
		primaries: primaries, secondaries: secondaries, secondaryArms: secondaryArms,
		round: DiagnosticParserCoreDispatchRound{
			Before: []DiagnosticParserCoreHeaderReceipt{before}, Actions: receipts, After: after,
		},
		branchOrder: trialOrder, nextSeq: trialSeq,
	}, nil
}

func recordDiagnosticParserCoreAppliedReductions(result *DiagnosticParserCorePrefixResult, token Token, actions []DiagnosticParserCoreRoundAction) {
	for _, dispatched := range actions {
		if dispatched.Action.Type != ParseActionReduce {
			continue
		}
		result.ReductionAttempts = append(result.ReductionAttempts, DiagnosticParserCoreReductionAttempt{
			State: dispatched.State, Lookahead: token, Action: dispatched.Action, Applied: true,
		})
	}
}

func recordDiagnosticParserCoreFirstFork(compact *core.Core, headers []diagnosticParserCoreHeader, result *DiagnosticParserCorePrefixResult) error {
	receipts, err := diagnosticParserCoreHeaderReceipts(compact, headers)
	if err != nil {
		return err
	}
	result.ForkBoundaryReceipts = result.ForkBoundaryReceipts[:0]
	result.ForkLogicalPaths = 0
	for _, receipt := range receipts {
		result.ForkLogicalPaths += receipt.ExactPaths
		result.ForkBoundaryReceipts = append(result.ForkBoundaryReceipts, DiagnosticParserCoreForkBoundary{
			State: receipt.State, ByteOffset: receipt.ByteOffset, ExactPaths: receipt.ExactPaths,
		})
	}
	sort.Slice(result.ForkBoundaryReceipts, func(i, j int) bool {
		if result.ForkBoundaryReceipts[i].State != result.ForkBoundaryReceipts[j].State {
			return result.ForkBoundaryReceipts[i].State < result.ForkBoundaryReceipts[j].State
		}
		return result.ForkBoundaryReceipts[i].ByteOffset < result.ForkBoundaryReceipts[j].ByteOffset
	})
	result.ForkBoundaries = len(result.ForkBoundaryReceipts)
	return nil
}

func diagnosticParserCoreSubsequentConflictReceipt(
	compact *core.Core,
	head core.Head,
	creationSeq uint64,
	token Token,
	actions []core.Action,
	electionIndex int,
	checkpoint [32]byte,
) (*DiagnosticParserCoreSubsequentConflictReceipt, error) {
	converted := make([]ParseAction, len(actions))
	for index, action := range actions {
		converted[index] = rootParserCoreAction(action)
	}
	headerReceipt, err := diagnosticParserCoreHeaderReceipt(compact, diagnosticParserCoreHeader{
		head: head, creationSeq: creationSeq, checkpoint: checkpoint,
	})
	if err != nil {
		return nil, err
	}
	derivations, err := compact.Derivations(head)
	if err != nil {
		return nil, err
	}
	if len(derivations) != 1 {
		return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "subsequent conflict requires one exact resumed derivation"}
	}
	derivation := derivations[0]
	return &DiagnosticParserCoreSubsequentConflictReceipt{
		State: headerReceipt.State, ByteOffset: headerReceipt.ByteOffset, Header: headerReceipt,
		Token: token, Actions: converted, ElectionIndex: electionIndex,
		Score: derivation.Score, BranchOrder: derivation.BranchOrder,
		HasBranchOrder: derivation.HasBranchOrder,
	}, nil
}

func diagnosticParserCoreLaterForkBoundaries(compact *core.Core, headers []diagnosticParserCoreHeader) ([]DiagnosticParserCoreLaterForkBoundary, uint64, error) {
	out := make([]DiagnosticParserCoreLaterForkBoundary, 0, len(headers))
	var exactPaths uint64
	for _, header := range headers {
		receipt, err := diagnosticParserCoreHeaderReceipt(compact, header)
		if err != nil {
			return nil, 0, err
		}
		derivations, err := compact.Derivations(header.head)
		if err != nil {
			return nil, 0, err
		}
		if len(derivations) != 1 || receipt.ExactPaths != 1 {
			return nil, 0, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "later fork boundary requires one exact derivation"}
		}
		derivation := derivations[0]
		out = append(out, DiagnosticParserCoreLaterForkBoundary{
			Header: receipt, Score: derivation.Score,
			BranchOrder: derivation.BranchOrder, HasBranchOrder: derivation.HasBranchOrder,
		})
		exactPaths += receipt.ExactPaths
	}
	return out, exactPaths, nil
}

func canonicalizeDiagnosticParserCoreHeaders(compact *core.Core, headers []diagnosticParserCoreHeader) ([]diagnosticParserCoreHeader, error) {
	type phaseHead struct {
		head       core.Head
		shifted    bool
		accepted   bool
		checkpoint [32]byte
	}
	out := make([]diagnosticParserCoreHeader, 0, len(headers))
	seen := make(map[phaseHead]bool, len(headers))
	for _, header := range headers {
		state, byteOffset, err := compact.Boundary(header.head)
		if err != nil {
			return nil, err
		}
		if canonical, ok := compact.CanonicalBoundary(state, byteOffset, header.shifted, header.checkpoint); ok {
			header.head = canonical
		}
		key := phaseHead{head: header.head, shifted: header.shifted, accepted: header.accepted, checkpoint: header.checkpoint}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, header)
	}
	return out, nil
}

func continueDiagnosticParserCoreSameToken(
	compact *core.Core,
	tokenSource *dfaTokenSource,
	scannerScratch *[]byte,
	token Token,
	headers []diagnosticParserCoreHeader,
	branchOrder uint64,
	nextSeq uint64,
	options DiagnosticParserCorePrefixOptions,
	result *DiagnosticParserCorePrefixResult,
	resume *diagnosticParserCoreOuterResume,
) error {
	for roundIndex := 1; ; roundIndex++ {
		if result.Dispatches >= options.MaxDispatches {
			return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreCap, detail: "same-lookahead dispatch cap"}
		}
		runnable := -1
		for index, header := range headers {
			if header.accepted || header.shifted {
				continue
			}
			if runnable != -1 {
				return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "multiple runnable headers require cohort scheduling"}
			}
			runnable = index
		}
		if runnable == -1 {
			for _, header := range headers {
				if header.accepted {
					continue
				}
				_, byteOffset, err := compact.Boundary(header.head)
				if err != nil {
					return err
				}
				if !header.shifted || byteOffset != token.EndByte {
					return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "same-lookahead heads did not close at token end"}
				}
			}
			if result.LaterForkExecution != nil {
				closed, exactPaths, err := diagnosticParserCoreLaterForkBoundaries(compact, headers)
				if err != nil {
					return err
				}
				result.LaterForkExecution.ClosedBoundaries = closed
				result.LaterForkExecution.ClosedExactPaths = exactPaths
				return continueDiagnosticParserCoreOrderedCohort(
					compact, tokenSource, scannerScratch, headers, branchOrder, nextSeq, options, result,
				)
			}
			result.LastBranchOrder = branchOrder
			result.Boundary = DiagnosticParserCoreElectionBarrier
			result.Detail = fmt.Sprintf("same lookahead closed at byte %d before multi-state election", token.EndByte)
			return &diagnosticParserCoreDecline{boundary: result.Boundary, detail: result.Detail}
		}

		beforeAll, err := diagnosticParserCoreHeaderReceipts(compact, headers)
		if err != nil {
			return err
		}
		active := headers[runnable]
		state, byteOffset, err := compact.Boundary(active.head)
		if err != nil {
			return err
		}
		if byteOffset != token.StartByte {
			return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "runnable header is not at cached lookahead start"}
		}
		result.State, result.Lookahead = StateID(state), token
		result.Dispatches++
		actions, err := compact.Actions(state, core.Symbol(token.Symbol))
		if err != nil {
			return err
		}
		if len(actions) == 0 {
			return continueDiagnosticParserCoreFrozenOracleCondense(
				compact, tokenSource, scannerScratch, token, headers, runnable, branchOrder, nextSeq, options, result, resume,
			)
		}
		if err := validateDiagnosticParserCoreCell(token, actions); err != nil {
			return err
		}
		if len(actions) > 1 && result.LaterForkExecution != nil {
			receipt, receiptErr := diagnosticParserCoreSubsequentConflictReceipt(
				compact, active.head, active.creationSeq, token, actions,
				len(result.Elections)-1, active.checkpoint,
			)
			if receiptErr != nil {
				return receiptErr
			}
			result.SubsequentConflict = receipt
			result.Boundary = DiagnosticParserCoreSubsequentConflictBoundary
			result.Detail = "third multi-action cell reached before execution"
			return &diagnosticParserCoreDecline{boundary: result.Boundary, detail: result.Detail}
		}

		var round DiagnosticParserCoreDispatchRound
		if len(actions) > 1 {
			outputs, conflictRound, nextOrder, nextCreationSeq, err := executeDiagnosticParserCoreConflict(
				compact, active, runnable, token, actions, branchOrder, nextSeq,
			)
			if err != nil {
				return err
			}
			if len(outputs) < 2 {
				return errors.New("parser-core phase zero: conflict did not create a secondary header")
			}
			headers[runnable] = outputs[0]
			headers = append(headers, outputs[1:]...)
			branchOrder, nextSeq = nextOrder, nextCreationSeq
			round = conflictRound
		} else {
			action := actions[0]
			var output []core.Head
			err := compact.ApplyAtomic(func() error {
				var applyErr error
				output, applyErr = applyParserCorePrefixAction(compact, active.head, token, action, 0, core.ForkOrder{})
				if applyErr != nil {
					return applyErr
				}
				if len(output) != 1 {
					return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "single action produced multiple scheduler boundaries"}
				}
				return nil
			})
			if err != nil {
				return err
			}
			headers[runnable].head = output[0]
			headers[runnable].shifted = action.Type == core.ActionShift
			round.Actions = []DiagnosticParserCoreRoundAction{{
				HeaderIndex: runnable, State: StateID(state), ByteOffset: byteOffset,
				Ordinal: 0, Action: rootParserCoreAction(action),
			}}
		}

		headers, err = canonicalizeDiagnosticParserCoreHeaders(compact, headers)
		if err != nil {
			return err
		}
		afterAll, err := diagnosticParserCoreHeaderReceipts(compact, headers)
		if err != nil {
			return err
		}
		round.Index = roundIndex
		round.Before = beforeAll
		round.After = afterAll
		if result.LaterForkExecution == nil {
			result.SameTokenRounds = append(result.SameTokenRounds, round)
		}
		recordDiagnosticParserCoreAppliedReductions(result, token, round.Actions)
		result.LastBranchOrder = branchOrder
	}
}

type diagnosticParserCoreOrderedElectionStage struct {
	beforeHeaders     []DiagnosticParserCoreHeaderReceipt
	states            []StateID
	expectedBefore    DiagnosticParserCoreScannerCheckpoint
	actualBefore      DiagnosticParserCoreScannerCheckpoint
	election          DiagnosticParserCoreElection
	orderedBoundaries []DiagnosticParserCoreLaterForkBoundary
}

type diagnosticParserCoreCohortExecution struct {
	headers    []diagnosticParserCoreHeader
	dispatches uint64
	reset      []DiagnosticParserCoreHeaderReceipt
	rounds     []DiagnosticParserCoreDispatchRound
	condense   *DiagnosticParserCoreCohortCondense
}

func continueDiagnosticParserCoreOrderedCohort(
	compact *core.Core,
	tokenSource *dfaTokenSource,
	scannerScratch *[]byte,
	headers []diagnosticParserCoreHeader,
	branchOrder uint64,
	nextSeq uint64,
	options DiagnosticParserCorePrefixOptions,
	result *DiagnosticParserCorePrefixResult,
) error {
	if tokenSource == nil || scannerScratch == nil || len(headers) != 2 || result.LaterForkExecution == nil {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "ordered cohort requires the exact closed later-fork frontier"}
	}
	for electionIndex := 0; electionIndex < 2; electionIndex++ {
		if len(result.Elections) == 0 || result.Tokens >= options.MaxTokens {
			return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreCap, detail: "ordered cohort election token cap"}
		}
		stage, err := authenticateDiagnosticParserCoreOrderedElection(
			compact, tokenSource, scannerScratch, headers, electionIndex,
			result.Elections[len(result.Elections)-1].ScannerAfter,
		)
		if err != nil {
			return err
		}
		execution, err := executeDiagnosticParserCoreOrderedCohort(
			compact, headers, stage.election, len(result.Elections),
			result.Dispatches, options.MaxDispatches, len(result.CohortRounds),
		)
		if err != nil {
			return err
		}
		headers = execution.headers
		result.Elections = append(result.Elections, stage.election)
		result.Tokens++
		result.OrderedElections = append(result.OrderedElections, DiagnosticParserCoreOrderedElection{
			Index: electionIndex, Before: stage.beforeHeaders, States: stage.states,
			ExpectedBefore: stage.expectedBefore, ActualBefore: stage.actualBefore,
			CheckpointContinuous: true, Token: stage.election.Token, Reset: execution.reset,
			Boundaries: stage.orderedBoundaries, BranchOrder: branchOrder, NextCreationSeq: nextSeq,
		})
		result.Dispatches = execution.dispatches
		for _, round := range execution.rounds {
			result.CohortRounds = append(result.CohortRounds, round)
			recordDiagnosticParserCoreAppliedReductions(result, stage.election.Token, round.Actions)
		}
		if execution.condense != nil {
			result.CohortCondense = execution.condense
			result.State, result.Lookahead = execution.condense.Preserved.State, execution.condense.Token
			postExecution, postErr := executeDiagnosticParserCorePostCondenseContinuation(
				compact, tokenSource, scannerScratch, execution.headers[0],
				stage.election.ScannerAfter, len(result.Elections), result.Dispatches,
				options, branchOrder, nextSeq,
			)
			if postErr != nil {
				return postErr
			}
			postCondense := postExecution.receipt
			result.Elections = append(result.Elections, postCondense.ContinuationElection, postCondense.ConflictElection)
			result.Tokens += 2
			result.Dispatches = postCondense.Dispatches
			result.PostCondenseContinuation = postCondense
			conflict := postCondense.Conflict
			result.SubsequentConflict = &conflict
			result.State, result.Lookahead = postCondense.Conflict.State, postCondense.Conflict.Token
			result.Boundary = DiagnosticParserCoreSubsequentConflictBoundary
			result.Detail = "post-condense continuation reached authenticated multi-action cell before execution"
			packed, packedErr := executeDiagnosticParserCorePackedConvergence(
				compact, tokenSource, scannerScratch, postExecution.finalHeader,
				postCondense.ConflictElection, len(result.Elections), result.Dispatches,
				options, branchOrder, nextSeq,
			)
			if packedErr != nil {
				return packedErr
			}
			result.Elections = append(result.Elections, packed.Election)
			result.Tokens++
			result.Dispatches = packed.Dispatches
			result.LastBranchOrder = packed.GlobalBranchOrder
			result.PackedConvergence = packed
			result.State, result.Lookahead = packed.Packed.State, packed.Election.Token
			result.Boundary = DiagnosticParserCoreElectionBarrier
			result.Detail = "packed convergence closed before the next scanner election"
			dotExecution, dotErr := executeDiagnosticParserCoreDotConflictFanout(
				compact, tokenSource, scannerScratch, packed, len(result.Elections), result.Dispatches,
				options, packed.GlobalBranchOrder, packed.NextCreationSeq,
			)
			if dotErr != nil {
				return dotErr
			}
			dot := dotExecution.receipt
			result.Elections = append(result.Elections, dot.Election)
			result.Tokens++
			result.Dispatches = dot.Dispatches
			result.LastBranchOrder = dot.GlobalBranchOrder
			result.DotConflictFanout = dot
			result.State, result.Lookahead = dot.Headers[0].Header.State, dot.Election.Token
			result.Boundary = DiagnosticParserCoreDotConflictFanoutBoundary
			result.Detail = "dot conflict fanout closed before cached-lookahead primary shifts"
			closure, closureHeaders, closureErr := executeDiagnosticParserCoreCachedDotClosure(
				compact, tokenSource, scannerScratch, dotExecution.headers, dot,
				len(result.Elections), result.Dispatches, result.Tokens, options,
			)
			if closureErr != nil {
				return closureErr
			}
			result.Elections = append(result.Elections, closure.Election)
			result.Tokens++
			result.Dispatches = closure.Dispatches
			result.CachedDotClosure = closure
			result.State, result.Lookahead = closure.Headers[0].Header.State, closure.Election.Token
			result.Boundary = DiagnosticParserCoreCachedDotClosureBoundary
			result.Detail = "cached dot closure authenticated before edits shifts"
			if options.GenericScheduler {
				generic, genericErr := executeDiagnosticParserCoreGenericScheduler(
					compact, tokenSource, scannerScratch, closureHeaders, closure,
					result.Dispatches, result.Tokens, options,
				)
				if genericErr != nil {
					return genericErr
				}
				result.GenericScheduler = generic
				result.Elections = append(result.Elections, generic.Elections...)
				result.Tokens = generic.Tokens
				result.Dispatches = generic.Dispatches
				result.LastBranchOrder = generic.GlobalBranchOrder
				result.State, result.Lookahead = generic.Stop.State, generic.Stop.Token
				result.Boundary, result.Detail = generic.Stop.Boundary, generic.Stop.Detail
			}
			return &diagnosticParserCoreDecline{boundary: result.Boundary, detail: result.Detail}
		}
	}
	return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "ordered cohort exceeded frozen two-election route"}
}

func authenticateDiagnosticParserCoreOrderedElection(
	compact *core.Core,
	tokenSource *dfaTokenSource,
	scannerScratch *[]byte,
	headers []diagnosticParserCoreHeader,
	electionIndex int,
	expectedBefore DiagnosticParserCoreScannerCheckpoint,
) (diagnosticParserCoreOrderedElectionStage, error) {
	beforeHeaders, err := diagnosticParserCoreHeaderReceipts(compact, headers)
	if err != nil {
		return diagnosticParserCoreOrderedElectionStage{}, err
	}
	states := make([]StateID, len(beforeHeaders))
	for index, header := range beforeHeaders {
		if header.Accepted || !header.Shifted || header.ExactPaths != 1 {
			return diagnosticParserCoreOrderedElectionStage{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "ordered cohort election requires shifted single-path headers"}
		}
		states[index] = header.State
		if header.Checkpoint != expectedBefore.SHA256 {
			return diagnosticParserCoreOrderedElectionStage{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "ordered cohort header checkpoint mismatch"}
		}
	}
	if err := validateDiagnosticParserCoreOrderedFrontier(electionIndex, beforeHeaders, states); err != nil {
		return diagnosticParserCoreOrderedElectionStage{}, err
	}
	tokenSource.SetParserState(states[0])
	tokenSource.SetGLRStates(append([]StateID(nil), states...))
	actualBefore := parserCoreCheckpoint(append([]byte(nil), tokenSource.captureExternalScannerStateInto(scannerScratch)...))
	if actualBefore != expectedBefore {
		return diagnosticParserCoreOrderedElectionStage{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "ordered cohort scanner checkpoint continuity failed"}
	}
	token := tokenSource.Next()
	after := append([]byte(nil), tokenSource.captureExternalScannerStateInto(scannerScratch)...)
	current, currentStart, currentEnd, currentValid := currentExternalScannerCheckpoint(tokenSource)
	election := DiagnosticParserCoreElection{
		States: append([]StateID(nil), states...), Token: token,
		ScannerBefore:          actualBefore,
		ScannerAfter:           parserCoreCheckpoint(after),
		CurrentCheckpointValid: currentValid,
		CurrentCheckpointStart: parserCoreCheckpoint(current.start),
		CurrentCheckpointEnd:   parserCoreCheckpoint(current.end),
		CurrentCheckpointBytes: [2]uint32{currentStart, currentEnd},
	}
	if err := validateDiagnosticParserCoreOrderedScannerReceipt(electionIndex, election); err != nil {
		return diagnosticParserCoreOrderedElectionStage{}, err
	}
	orderedBoundaries, exactPaths, err := diagnosticParserCoreLaterForkBoundaries(compact, headers)
	if err != nil {
		return diagnosticParserCoreOrderedElectionStage{}, err
	}
	if exactPaths != 2 || len(orderedBoundaries) != 2 ||
		orderedBoundaries[0].Score != -10 || orderedBoundaries[0].BranchOrder != 1 || !orderedBoundaries[0].HasBranchOrder ||
		orderedBoundaries[1].Score != -10 || orderedBoundaries[1].BranchOrder != 2 || !orderedBoundaries[1].HasBranchOrder {
		return diagnosticParserCoreOrderedElectionStage{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "ordered cohort score/order identity mismatch"}
	}
	return diagnosticParserCoreOrderedElectionStage{
		beforeHeaders: beforeHeaders, states: states,
		expectedBefore: expectedBefore, actualBefore: actualBefore,
		election: election, orderedBoundaries: orderedBoundaries,
	}, nil
}

func validateDiagnosticParserCoreOrderedFrontier(electionIndex int, headers []DiagnosticParserCoreHeaderReceipt, states []StateID) error {
	wantStates := []StateID{164, 194}
	wantByte := uint32(725)
	if electionIndex == 1 {
		wantStates = []StateID{410, 444}
		wantByte = 730
	}
	if electionIndex < 0 || electionIndex > 1 || !reflect.DeepEqual(states, wantStates) || len(headers) != 2 ||
		headers[0].ByteOffset != wantByte || headers[1].ByteOffset != wantByte ||
		headers[0].CreationSeq != 1 || headers[1].CreationSeq != 2 {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "ordered cohort does not match its frozen frontier"}
	}
	return nil
}

func validateDiagnosticParserCoreOrderedScannerReceipt(electionIndex int, election DiagnosticParserCoreElection) error {
	token := election.Token
	validToken := electionIndex == 0 && token.Symbol == 86 && token.Text == "edits" && token.StartByte == 725 && token.EndByte == 730
	validToken = validToken || electionIndex == 1 && token.Symbol == 10 && token.Text == "=" && token.StartByte == 731 && token.EndByte == 732
	emptyCheckpoint := parserCoreCheckpoint(nil)
	if !validToken || token.Missing || token.NoLookahead || token.ExternalScannerToken ||
		election.ScannerBefore != emptyCheckpoint || election.ScannerAfter != emptyCheckpoint ||
		election.CurrentCheckpointValid || election.CurrentCheckpointStart != emptyCheckpoint ||
		election.CurrentCheckpointEnd != emptyCheckpoint || election.CurrentCheckpointBytes != [2]uint32{} {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "ordered cohort scanner receipt is not the frozen ordinary-DFA identity"}
	}
	return nil
}

func executeDiagnosticParserCoreOrderedCohort(
	compact *core.Core,
	headers []diagnosticParserCoreHeader,
	election DiagnosticParserCoreElection,
	electionIndex int,
	dispatches uint64,
	maxDispatches uint64,
	roundBase int,
) (diagnosticParserCoreCohortExecution, error) {
	execution := diagnosticParserCoreCohortExecution{
		headers:    append([]diagnosticParserCoreHeader(nil), headers...),
		dispatches: dispatches,
	}
	err := compact.ApplyAtomic(func() error {
		if err := compact.BeginFrontier(); err != nil {
			return err
		}
		compact.SetPhaseCheckpoint(election.ScannerAfter.SHA256)
		for index := range execution.headers {
			execution.headers[index].shifted = false
			execution.headers[index].checkpoint = election.ScannerAfter.SHA256
		}
		var err error
		execution.reset, err = diagnosticParserCoreHeaderReceipts(compact, execution.headers)
		if err != nil {
			return err
		}
		return runDiagnosticParserCoreCohortPasses(
			compact, election.Token, electionIndex, maxDispatches, roundBase, &execution,
		)
	})
	if err != nil {
		return diagnosticParserCoreCohortExecution{}, err
	}
	return execution, nil
}

func runDiagnosticParserCoreCohortPasses(
	compact *core.Core,
	token Token,
	electionIndex int,
	maxDispatches uint64,
	roundBase int,
	execution *diagnosticParserCoreCohortExecution,
) error {
	for pass := 0; ; pass++ {
		beforePass, err := diagnosticParserCoreHeaderReceipts(compact, execution.headers)
		if err != nil {
			return err
		}
		passActions, err := runDiagnosticParserCoreCohortPass(
			compact, token, electionIndex, maxDispatches, execution,
		)
		if err != nil {
			return err
		}
		afterPass, err := diagnosticParserCoreHeaderReceipts(compact, execution.headers)
		if err != nil {
			return err
		}
		if len(passActions) != 0 {
			execution.rounds = append(execution.rounds, DiagnosticParserCoreDispatchRound{
				Index: roundBase + len(execution.rounds), Before: beforePass,
				Actions: passActions, After: afterPass,
			})
		}
		if execution.condense != nil || diagnosticParserCoreHeadersConsumed(execution.headers) {
			return nil
		}
		if len(passActions) == 0 {
			return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: fmt.Sprintf("ordered cohort pass %d made no progress", pass)}
		}
	}
}

func runDiagnosticParserCoreCohortPass(
	compact *core.Core,
	token Token,
	electionIndex int,
	maxDispatches uint64,
	execution *diagnosticParserCoreCohortExecution,
) ([]DiagnosticParserCoreRoundAction, error) {
	var passActions []DiagnosticParserCoreRoundAction
	reducedThisPass := false
	for headerIndex := 0; headerIndex < len(execution.headers); headerIndex++ {
		for !execution.headers[headerIndex].accepted && !execution.headers[headerIndex].shifted {
			active := execution.headers[headerIndex]
			state, byteOffset, err := compact.Boundary(active.head)
			if err != nil {
				return nil, err
			}
			cell, err := compact.Actions(state, core.Symbol(token.Symbol))
			if err != nil {
				return nil, err
			}
			if len(cell) == 0 {
				if reducedThisPass {
					break
				}
				if execution.dispatches >= maxDispatches {
					return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreCap, detail: "ordered cohort dispatch cap"}
				}
				execution.dispatches++
				condense, kept, err := validateDiagnosticParserCoreState22Condense(compact, token, execution.headers, headerIndex, electionIndex)
				if err != nil {
					return nil, err
				}
				execution.headers, execution.condense = kept, &condense
				break
			}
			if err := validateDiagnosticParserCoreCell(token, cell); err != nil {
				return nil, err
			}
			if len(cell) != 1 {
				return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreSubsequentConflictBoundary, detail: "ordered cohort encountered an unauthenticated conflict cell"}
			}
			if cell[0].Type == core.ActionShift && reducedThisPass {
				break
			}
			if execution.dispatches >= maxDispatches {
				return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreCap, detail: "ordered cohort dispatch cap"}
			}
			output, err := applyParserCorePrefixAction(compact, active.head, token, cell[0], 0, core.ForkOrder{})
			if err != nil {
				return nil, err
			}
			if len(output) != 1 {
				return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "ordered cohort action produced multiple boundaries"}
			}
			execution.dispatches++
			execution.headers[headerIndex].head = output[0]
			execution.headers[headerIndex].shifted = cell[0].Type == core.ActionShift
			passActions = append(passActions, DiagnosticParserCoreRoundAction{
				HeaderIndex: headerIndex, State: StateID(state), ByteOffset: byteOffset,
				Ordinal: 0, Action: rootParserCoreAction(cell[0]),
			})
			if cell[0].Type == core.ActionReduce {
				reducedThisPass = true
				continue
			}
			break
		}
		if execution.condense != nil {
			break
		}
	}
	return passActions, nil
}

func diagnosticParserCoreHeadersConsumed(headers []diagnosticParserCoreHeader) bool {
	for _, header := range headers {
		if !header.shifted && !header.accepted {
			return false
		}
	}
	return true
}

func executeDiagnosticParserCorePostCondenseContinuation(
	compact *core.Core,
	tokenSource *dfaTokenSource,
	scannerScratch *[]byte,
	header diagnosticParserCoreHeader,
	expectedBefore DiagnosticParserCoreScannerCheckpoint,
	electionBase int,
	dispatches uint64,
	options DiagnosticParserCorePrefixOptions,
	globalBranchOrder uint64,
	nextCreationSeq uint64,
) (*diagnosticParserCorePostCondenseExecution, error) {
	if electionBase != 98 || globalBranchOrder != 2 || nextCreationSeq != 3 {
		return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "post-condense continuation allocator identity mismatch"}
	}
	if uint64(electionBase)+2 > options.MaxTokens {
		return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreCap, detail: "post-condense continuation token cap"}
	}
	before, err := diagnosticParserCoreHeaderReceipt(compact, header)
	if err != nil {
		return nil, err
	}
	boundaries, exactPaths, err := diagnosticParserCoreLaterForkBoundaries(compact, []diagnosticParserCoreHeader{header})
	if err != nil {
		return nil, err
	}
	if before.State != 248 || before.ByteOffset != 732 || before.CreationSeq != 1 || !before.Shifted || before.Accepted || before.ExactPaths != 1 ||
		before.Checkpoint != expectedBefore.SHA256 || exactPaths != 1 || len(boundaries) != 1 ||
		boundaries[0].Score != -10 || boundaries[0].BranchOrder != 1 || !boundaries[0].HasBranchOrder {
		return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "post-condense continuation header identity mismatch"}
	}
	continuationElection, err := readDiagnosticParserCorePostCondenseElection(
		tokenSource, scannerScratch, 248, expectedBefore, 86, "append", 733, 739,
	)
	if err != nil {
		return nil, err
	}
	receipt := &DiagnosticParserCorePostCondenseContinuation{
		Before: before, BeforeBoundary: boundaries[0],
		ContinuationElectionIndex:  electionBase,
		ContinuationExpectedBefore: expectedBefore,
		ContinuationElection:       continuationElection,
		ConflictElectionIndex:      electionBase + 1,
		GlobalBranchOrder:          globalBranchOrder, NextCreationSeq: nextCreationSeq,
	}
	trialHeader := header
	err = compact.ApplyAtomic(func() error {
		if err := applyDiagnosticParserCorePostCondenseShift(
			compact, continuationElection, options.MaxDispatches,
			&trialHeader, &dispatches, receipt,
		); err != nil {
			return err
		}
		return authenticateDiagnosticParserCorePostCondenseConflict(
			compact, tokenSource, scannerScratch, continuationElection.ScannerAfter,
			electionBase+1, options.MaxDispatches, &trialHeader, &dispatches, receipt,
		)
	})
	if err != nil {
		return nil, err
	}
	return &diagnosticParserCorePostCondenseExecution{receipt: receipt, finalHeader: trialHeader}, nil
}

func applyDiagnosticParserCorePostCondenseShift(
	compact *core.Core,
	election DiagnosticParserCoreElection,
	maxDispatches uint64,
	header *diagnosticParserCoreHeader,
	dispatches *uint64,
	receipt *DiagnosticParserCorePostCondenseContinuation,
) error {
	if err := compact.BeginFrontier(); err != nil {
		return err
	}
	compact.SetPhaseCheckpoint(election.ScannerAfter.SHA256)
	header.shifted = false
	header.checkpoint = election.ScannerAfter.SHA256
	reset, err := diagnosticParserCoreHeaderReceipt(compact, *header)
	if err != nil {
		return err
	}
	receipt.ContinuationReset = reset
	state, byteOffset, err := compact.Boundary(header.head)
	if err != nil {
		return err
	}
	actions, err := compact.Actions(state, core.Symbol(election.Token.Symbol))
	if err != nil {
		return err
	}
	if state != 248 || byteOffset != 732 || !reflect.DeepEqual(actions, []core.Action{{Type: core.ActionShift, State: 186}}) {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "post-condense state248 continuation cell mismatch"}
	}
	if *dispatches >= maxDispatches {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreCap, detail: "post-condense continuation dispatch cap"}
	}
	output, err := applyParserCorePrefixAction(compact, header.head, election.Token, actions[0], 0, core.ForkOrder{})
	if err != nil {
		return err
	}
	if len(output) != 1 {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "post-condense continuation shift produced multiple boundaries"}
	}
	*dispatches++
	header.head, header.shifted = output[0], true
	afterShift, err := diagnosticParserCoreHeaderReceipt(compact, *header)
	if err != nil {
		return err
	}
	receipt.ShiftRound = DiagnosticParserCoreDispatchRound{
		Index: 0, Before: []DiagnosticParserCoreHeaderReceipt{reset},
		Actions: []DiagnosticParserCoreRoundAction{{
			HeaderIndex: 0, State: 248, ByteOffset: 732,
			Ordinal: 0, Action: rootParserCoreAction(actions[0]),
		}},
		After: []DiagnosticParserCoreHeaderReceipt{afterShift},
	}
	shifted, paths, err := diagnosticParserCoreLaterForkBoundaries(compact, []diagnosticParserCoreHeader{*header})
	if err != nil {
		return err
	}
	if paths != 1 || len(shifted) != 1 || shifted[0].Score != -10 || shifted[0].BranchOrder != 1 || !shifted[0].HasBranchOrder {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "post-condense shifted boundary score/order mismatch"}
	}
	receipt.ShiftedBoundary = shifted[0]
	return nil
}

func authenticateDiagnosticParserCorePostCondenseConflict(
	compact *core.Core,
	tokenSource *dfaTokenSource,
	scannerScratch *[]byte,
	expectedBefore DiagnosticParserCoreScannerCheckpoint,
	electionIndex int,
	maxDispatches uint64,
	header *diagnosticParserCoreHeader,
	dispatches *uint64,
	receipt *DiagnosticParserCorePostCondenseContinuation,
) error {
	election, err := readDiagnosticParserCorePostCondenseElection(
		tokenSource, scannerScratch, 186, expectedBefore, 6, "(", 739, 740,
	)
	if err != nil {
		return err
	}
	receipt.ConflictExpectedBefore = expectedBefore
	receipt.ConflictElection = election
	if err := compact.BeginFrontier(); err != nil {
		return err
	}
	compact.SetPhaseCheckpoint(election.ScannerAfter.SHA256)
	header.shifted = false
	header.checkpoint = election.ScannerAfter.SHA256
	reset, err := diagnosticParserCoreHeaderReceipt(compact, *header)
	if err != nil {
		return err
	}
	receipt.ConflictReset = reset
	state, byteOffset, err := compact.Boundary(header.head)
	if err != nil {
		return err
	}
	actions, err := compact.Actions(state, core.Symbol(election.Token.Symbol))
	if err != nil {
		return err
	}
	want := []core.Action{
		{Type: core.ActionReduce, Symbol: 121, ChildCount: 1, DynamicPrecedence: -1, ProductionID: 44},
		{Type: core.ActionReduce, Symbol: 171, ChildCount: 1},
	}
	if state != 186 || byteOffset != 739 || !reflect.DeepEqual(actions, want) {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "post-condense state186 conflict cell mismatch"}
	}
	if *dispatches >= maxDispatches {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreCap, detail: "post-condense conflict dispatch cap"}
	}
	*dispatches++
	conflict, err := diagnosticParserCoreSubsequentConflictReceipt(
		compact, header.head, header.creationSeq, election.Token, actions, electionIndex, header.checkpoint,
	)
	if err != nil {
		return err
	}
	receipt.Conflict = *conflict
	receipt.Dispatches = *dispatches
	return nil
}

func executeDiagnosticParserCorePackedConvergence(
	compact *core.Core,
	tokenSource *dfaTokenSource,
	scannerScratch *[]byte,
	header diagnosticParserCoreHeader,
	conflictElection DiagnosticParserCoreElection,
	electionIndex int,
	dispatches uint64,
	options DiagnosticParserCorePrefixOptions,
	branchOrder uint64,
	nextCreationSeq uint64,
) (*DiagnosticParserCorePackedConvergence, error) {
	if electionIndex != 100 || branchOrder != 2 || nextCreationSeq != 3 {
		return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "packed convergence allocator identity mismatch"}
	}
	if uint64(electionIndex) >= options.MaxTokens {
		return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreCap, detail: "packed convergence election token cap"}
	}
	receipt := &DiagnosticParserCorePackedConvergence{}
	err := compact.ApplyAtomic(func() error {
		conflict, err := authenticateDiagnosticParserCorePackedConflict(
			compact, header, conflictElection.Token, branchOrder, nextCreationSeq, receipt,
		)
		if err != nil {
			return err
		}
		paren, err := closeDiagnosticParserCorePackedParenthesis(
			compact, conflictElection.Token, electionIndex, options.MaxDispatches,
			conflict.headers, dispatches, receipt,
		)
		if err != nil {
			return err
		}
		return electAndPackDiagnosticParserCoreIdentifier(
			compact, tokenSource, scannerScratch, conflictElection.ScannerAfter,
			electionIndex, options.MaxDispatches, paren, conflict, receipt,
		)
	})
	if err != nil {
		return nil, err
	}
	return receipt, nil
}

type diagnosticParserCorePackedConflictStage struct {
	headers              []diagnosticParserCoreHeader
	branchOrder, nextSeq uint64
}

func authenticateDiagnosticParserCorePackedConflict(
	compact *core.Core,
	header diagnosticParserCoreHeader,
	token Token,
	branchOrder uint64,
	nextCreationSeq uint64,
	receipt *DiagnosticParserCorePackedConvergence,
) (diagnosticParserCorePackedConflictStage, error) {
	wantConflict := []core.Action{
		{Type: core.ActionReduce, Symbol: 121, ChildCount: 1, DynamicPrecedence: -1, ProductionID: 44},
		{Type: core.ActionReduce, Symbol: 171, ChildCount: 1},
	}
	headers, round, nextOrder, nextSeq, err := executeDiagnosticParserCoreConflict(
		compact, header, 0, token, wantConflict, branchOrder, nextCreationSeq,
	)
	if err != nil {
		return diagnosticParserCorePackedConflictStage{}, err
	}
	if nextOrder != 3 || nextSeq != 4 || len(headers) != 2 {
		return diagnosticParserCorePackedConflictStage{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "packed conflict allocation identity mismatch"}
	}
	round.Index = 0
	wantRound := []DiagnosticParserCoreRoundAction{
		{HeaderIndex: 0, State: 186, ByteOffset: 739, Ordinal: 1, Action: rootParserCoreAction(wantConflict[1]), BranchOrder: 3},
		{HeaderIndex: 0, State: 186, ByteOffset: 739, Ordinal: 0, Action: rootParserCoreAction(wantConflict[0])},
	}
	if !reflect.DeepEqual(round.Actions, wantRound) {
		return diagnosticParserCorePackedConflictStage{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "packed conflict execution order mismatch"}
	}
	receipt.ConflictRound = round
	boundaries, paths, err := diagnosticParserCoreLaterForkBoundaries(compact, headers)
	if err != nil {
		return diagnosticParserCorePackedConflictStage{}, err
	}
	if paths != 2 || len(boundaries) != 2 {
		return diagnosticParserCorePackedConflictStage{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "packed conflict did not retain two exact derivations"}
	}
	if boundaries[0].Header.State != 101 || boundaries[0].Header.CreationSeq != 1 || boundaries[0].Score != -11 || boundaries[0].BranchOrder != 1 || !boundaries[0].HasBranchOrder ||
		boundaries[1].Header.State != 253 || boundaries[1].Header.CreationSeq != 3 || boundaries[1].Score != -10 || boundaries[1].BranchOrder != 3 || !boundaries[1].HasBranchOrder {
		return diagnosticParserCorePackedConflictStage{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "packed conflict post-reduce/GOTO identity mismatch"}
	}
	receipt.PostConflictBoundaries = boundaries
	return diagnosticParserCorePackedConflictStage{headers: headers, branchOrder: nextOrder, nextSeq: nextSeq}, nil
}

type diagnosticParserCorePackedParenthesisStage struct {
	headers    []diagnosticParserCoreHeader
	dispatches uint64
}

func closeDiagnosticParserCorePackedParenthesis(
	compact *core.Core,
	token Token,
	electionIndex int,
	maxDispatches uint64,
	headers []diagnosticParserCoreHeader,
	dispatches uint64,
	receipt *DiagnosticParserCorePackedConvergence,
) (diagnosticParserCorePackedParenthesisStage, error) {
	execution := diagnosticParserCoreCohortExecution{headers: headers, dispatches: dispatches}
	if err := runDiagnosticParserCoreCohortPasses(compact, token, electionIndex-1, maxDispatches, 0, &execution); err != nil {
		return diagnosticParserCorePackedParenthesisStage{}, err
	}
	if execution.condense != nil || !diagnosticParserCoreHeadersConsumed(execution.headers) {
		return diagnosticParserCorePackedParenthesisStage{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "packed conflict parenthesis closure declined"}
	}
	receipt.SameTokenRounds = execution.rounds
	wantRound := []DiagnosticParserCoreRoundAction{
		{HeaderIndex: 0, State: 101, ByteOffset: 739, Ordinal: 0, Action: ParseAction{Type: ParseActionShift, State: 276}},
		{HeaderIndex: 1, State: 253, ByteOffset: 739, Ordinal: 0, Action: ParseAction{Type: ParseActionShift, State: 163}},
	}
	if len(receipt.SameTokenRounds) != 1 || !reflect.DeepEqual(receipt.SameTokenRounds[0].Actions, wantRound) {
		return diagnosticParserCorePackedParenthesisStage{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "packed parenthesis dispatch order mismatch"}
	}
	boundaries, paths, err := diagnosticParserCoreLaterForkBoundaries(compact, execution.headers)
	if err != nil {
		return diagnosticParserCorePackedParenthesisStage{}, err
	}
	if paths != 2 || len(boundaries) != 2 {
		return diagnosticParserCorePackedParenthesisStage{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "packed conflict parenthesis paths drifted"}
	}
	if boundaries[0].Header.State != 276 || boundaries[0].Header.ByteOffset != 740 || boundaries[0].Header.CreationSeq != 1 || boundaries[0].Score != -11 || boundaries[0].BranchOrder != 1 || !boundaries[0].HasBranchOrder ||
		boundaries[1].Header.State != 163 || boundaries[1].Header.ByteOffset != 740 || boundaries[1].Header.CreationSeq != 3 || boundaries[1].Score != -10 || boundaries[1].BranchOrder != 3 || !boundaries[1].HasBranchOrder {
		return diagnosticParserCorePackedParenthesisStage{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "packed parenthesis closure identity mismatch"}
	}
	receipt.ClosedParenBoundaries = boundaries
	return diagnosticParserCorePackedParenthesisStage{headers: execution.headers, dispatches: execution.dispatches}, nil
}

func electAndPackDiagnosticParserCoreIdentifier(
	compact *core.Core,
	tokenSource *dfaTokenSource,
	scannerScratch *[]byte,
	expectedBefore DiagnosticParserCoreScannerCheckpoint,
	electionIndex int,
	maxDispatches uint64,
	paren diagnosticParserCorePackedParenthesisStage,
	conflict diagnosticParserCorePackedConflictStage,
	receipt *DiagnosticParserCorePackedConvergence,
) error {
	election, beforeHeaders, beforeBoundaries, err := readDiagnosticParserCorePackedElection(
		compact, tokenSource, scannerScratch, paren.headers, expectedBefore,
	)
	if err != nil {
		return err
	}
	receipt.ElectionIndex = electionIndex
	receipt.ElectionExpectedBefore = expectedBefore
	receipt.Election = election
	receipt.ElectionBefore = beforeHeaders
	receipt.ElectionBeforeBoundaries = beforeBoundaries
	if err := compact.BeginFrontier(); err != nil {
		return err
	}
	compact.SetPhaseCheckpoint(election.ScannerAfter.SHA256)
	for index := range paren.headers {
		paren.headers[index].shifted = false
		paren.headers[index].checkpoint = election.ScannerAfter.SHA256
	}
	receipt.ElectionReset, err = diagnosticParserCoreHeaderReceipts(compact, paren.headers)
	if err != nil {
		return err
	}
	packed, err := shiftDiagnosticParserCorePackedHeaders(
		compact, election.Token, paren.headers, paren.dispatches, maxDispatches, receipt,
	)
	if err != nil {
		return err
	}
	if len(receipt.BeforeCanonical) != 2 ||
		receipt.BeforeCanonical[0].State != 186 || receipt.BeforeCanonical[0].ByteOffset != 741 || receipt.BeforeCanonical[0].CreationSeq != 1 || !receipt.BeforeCanonical[0].Shifted || receipt.BeforeCanonical[0].ExactPaths != 1 ||
		receipt.BeforeCanonical[1].State != 186 || receipt.BeforeCanonical[1].ByteOffset != 741 || receipt.BeforeCanonical[1].CreationSeq != 3 || !receipt.BeforeCanonical[1].Shifted || receipt.BeforeCanonical[1].ExactPaths != 2 {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "packed raw post-shift boundary identity mismatch"}
	}
	canonical, err := canonicalizeDiagnosticParserCoreHeaders(compact, packed)
	if err != nil {
		return err
	}
	if len(canonical) != 1 {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "joint identifier shifts did not form one packed scheduler head"}
	}
	receipt.Packed, err = diagnosticParserCoreHeaderReceipt(compact, canonical[0])
	if err != nil {
		return err
	}
	derivations, err := compact.Derivations(canonical[0].head)
	if err != nil {
		return err
	}
	if len(derivations) != 2 || receipt.Packed.ExactPaths != 2 {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "packed scheduler head did not retain two derivations"}
	}
	if receipt.Packed.State != 186 || receipt.Packed.ByteOffset != 741 || receipt.Packed.CreationSeq != 1 || !receipt.Packed.Shifted {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "packed scheduler boundary identity mismatch"}
	}
	for _, derivation := range derivations {
		receipt.Derivations = append(receipt.Derivations, DiagnosticParserCorePackedDerivation{
			Score: derivation.Score, BranchOrder: derivation.BranchOrder, HasBranchOrder: derivation.HasBranchOrder,
		})
	}
	sort.Slice(receipt.Derivations, func(i, j int) bool { return receipt.Derivations[i].BranchOrder < receipt.Derivations[j].BranchOrder })
	if !reflect.DeepEqual(receipt.Derivations, []DiagnosticParserCorePackedDerivation{
		{Score: -11, BranchOrder: 1, HasBranchOrder: true},
		{Score: -10, BranchOrder: 3, HasBranchOrder: true},
	}) {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "packed derivation score/order identity mismatch"}
	}
	receipt.GlobalBranchOrder = conflict.branchOrder
	receipt.NextCreationSeq = conflict.nextSeq
	return nil
}

func readDiagnosticParserCorePackedElection(
	compact *core.Core,
	tokenSource *dfaTokenSource,
	scannerScratch *[]byte,
	headers []diagnosticParserCoreHeader,
	expectedBefore DiagnosticParserCoreScannerCheckpoint,
) (DiagnosticParserCoreElection, []DiagnosticParserCoreHeaderReceipt, []DiagnosticParserCoreLaterForkBoundary, error) {
	beforeHeaders, err := diagnosticParserCoreHeaderReceipts(compact, headers)
	if err != nil {
		return DiagnosticParserCoreElection{}, nil, nil, err
	}
	states := make([]StateID, len(headers))
	for index, header := range beforeHeaders {
		if !header.Shifted || header.Accepted || header.ExactPaths != 1 || header.ByteOffset != 740 || header.Checkpoint != expectedBefore.SHA256 {
			return DiagnosticParserCoreElection{}, nil, nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "packed election header identity mismatch"}
		}
		states[index] = header.State
	}
	boundaries, paths, err := diagnosticParserCoreLaterForkBoundaries(compact, headers)
	if err != nil {
		return DiagnosticParserCoreElection{}, nil, nil, err
	}
	if paths != 2 || len(boundaries) != 2 {
		return DiagnosticParserCoreElection{}, nil, nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "packed election derivation count mismatch"}
	}
	if !reflect.DeepEqual(states, []StateID{276, 163}) ||
		beforeHeaders[0].CreationSeq != 1 || boundaries[0].Score != -11 || boundaries[0].BranchOrder != 1 || !boundaries[0].HasBranchOrder ||
		beforeHeaders[1].CreationSeq != 3 || boundaries[1].Score != -10 || boundaries[1].BranchOrder != 3 || !boundaries[1].HasBranchOrder {
		return DiagnosticParserCoreElection{}, nil, nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "packed election ordered frontier mismatch"}
	}
	tokenSource.SetParserState(states[0])
	tokenSource.SetGLRStates(append([]StateID(nil), states...))
	before := parserCoreCheckpoint(append([]byte(nil), tokenSource.captureExternalScannerStateInto(scannerScratch)...))
	if before != expectedBefore {
		return DiagnosticParserCoreElection{}, nil, nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "packed election scanner continuity failed"}
	}
	token := tokenSource.Next()
	after := parserCoreCheckpoint(append([]byte(nil), tokenSource.captureExternalScannerStateInto(scannerScratch)...))
	current, currentStart, currentEnd, currentValid := currentExternalScannerCheckpoint(tokenSource)
	election := DiagnosticParserCoreElection{
		States: append([]StateID(nil), states...), Token: token,
		ScannerBefore: before, ScannerAfter: after,
		CurrentCheckpointValid: currentValid,
		CurrentCheckpointStart: parserCoreCheckpoint(current.start),
		CurrentCheckpointEnd:   parserCoreCheckpoint(current.end),
		CurrentCheckpointBytes: [2]uint32{currentStart, currentEnd},
	}
	empty := parserCoreCheckpoint(nil)
	if token.Symbol != 86 || token.Text != "r" || token.StartByte != 740 || token.EndByte != 741 ||
		token.Missing || token.NoLookahead || token.ExternalScannerToken || before != empty || after != empty ||
		currentValid || election.CurrentCheckpointStart != empty || election.CurrentCheckpointEnd != empty || election.CurrentCheckpointBytes != [2]uint32{} {
		return DiagnosticParserCoreElection{}, nil, nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "packed election token identity mismatch"}
	}
	return election, beforeHeaders, boundaries, nil
}

func shiftDiagnosticParserCorePackedHeaders(
	compact *core.Core,
	token Token,
	headers []diagnosticParserCoreHeader,
	dispatches uint64,
	maxDispatches uint64,
	receipt *DiagnosticParserCorePackedConvergence,
) ([]diagnosticParserCoreHeader, error) {
	plan, err := authenticateDiagnosticParserCorePackedShift(compact, token, headers, dispatches, maxDispatches)
	if err != nil {
		return nil, err
	}
	shifted, err := compact.ShiftOrdinaryCohort(plan.cohort, core.Symbol(token.Symbol), core.Token{
		Symbol: core.Symbol(token.Symbol), StartByte: token.StartByte, EndByte: token.EndByte,
	})
	if err != nil {
		return nil, err
	}
	if len(shifted) != len(headers) {
		return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "packed identifier cohort cardinality mismatch"}
	}
	for index := range headers {
		headers[index].head, headers[index].shifted = shifted[index], true
	}
	dispatches += uint64(len(headers))
	afterRound, err := diagnosticParserCoreHeaderReceipts(compact, headers)
	if err != nil {
		return nil, err
	}
	if err := measureDiagnosticParserCorePackedPayloads(compact, shifted, plan.beforeStats, receipt); err != nil {
		return nil, err
	}
	receipt.BeforeCanonical = afterRound
	receipt.ShiftRounds = []DiagnosticParserCoreDispatchRound{{Index: 0, Before: plan.beforeRound, Actions: plan.actions, After: afterRound}}
	receipt.Dispatches = dispatches
	return headers, nil
}

type diagnosticParserCorePackedShiftPlan struct {
	beforeRound []DiagnosticParserCoreHeaderReceipt
	beforeStats core.Stats
	cohort      []core.OrdinaryCohortShiftInput
	actions     []DiagnosticParserCoreRoundAction
}

func authenticateDiagnosticParserCorePackedShift(
	compact *core.Core,
	token Token,
	headers []diagnosticParserCoreHeader,
	dispatches uint64,
	maxDispatches uint64,
) (diagnosticParserCorePackedShiftPlan, error) {
	beforeRound, err := diagnosticParserCoreHeaderReceipts(compact, headers)
	if err != nil {
		return diagnosticParserCorePackedShiftPlan{}, err
	}
	stats, err := compact.Stats(headers[0].head)
	if err != nil {
		return diagnosticParserCorePackedShiftPlan{}, err
	}
	plan := diagnosticParserCorePackedShiftPlan{
		beforeRound: beforeRound,
		beforeStats: stats,
		cohort:      make([]core.OrdinaryCohortShiftInput, len(headers)),
	}
	wantStates := []StateID{276, 163}
	for index := range headers {
		state, byteOffset, err := compact.Boundary(headers[index].head)
		if err != nil {
			return diagnosticParserCorePackedShiftPlan{}, err
		}
		cell, err := compact.Actions(state, core.Symbol(token.Symbol))
		if err != nil {
			return diagnosticParserCorePackedShiftPlan{}, err
		}
		if state != core.StateID(wantStates[index]) || len(cell) != 1 ||
			!reflect.DeepEqual(cell[0], core.Action{Type: core.ActionShift, State: 186}) {
			return diagnosticParserCorePackedShiftPlan{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "packed identifier cell is not one ordinary shift"}
		}
		plan.cohort[index] = core.OrdinaryCohortShiftInput{Head: headers[index].head, ActionOrdinal: 0}
		plan.actions = append(plan.actions, DiagnosticParserCoreRoundAction{
			HeaderIndex: index, State: StateID(state), ByteOffset: byteOffset,
			Ordinal: 0, Action: rootParserCoreAction(cell[0]),
		})
	}
	if token.Missing || token.NoLookahead || token.ExternalScannerToken || token.StartByte >= token.EndByte || token.Symbol != 86 || token.Text != "r" {
		return diagnosticParserCorePackedShiftPlan{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "packed identifier is not one ordinary DFA terminal"}
	}
	if dispatches > maxDispatches || uint64(len(headers)) > maxDispatches-dispatches {
		return diagnosticParserCorePackedShiftPlan{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreCap, detail: "packed identifier dispatch cap"}
	}
	return plan, nil
}

func measureDiagnosticParserCorePackedPayloads(
	compact *core.Core,
	shifted []core.Head,
	before core.Stats,
	receipt *DiagnosticParserCorePackedConvergence,
) error {
	receipt.TerminalSubtreesBefore = before.Subtrees
	receipt.TerminalNodesBefore = before.Nodes
	receipt.TerminalLinksBefore = before.Links
	after, err := compact.Stats(shifted[0])
	if err != nil {
		return err
	}
	receipt.TerminalSubtreesAfter = after.Subtrees
	receipt.TerminalNodesAfter = after.Nodes
	receipt.TerminalLinksAfter = after.Links
	receipt.TerminalPayloadAllocations = after.Subtrees - before.Subtrees
	receipt.TerminalPayloadsPerCohort = receipt.TerminalPayloadAllocations
	for id := receipt.TerminalSubtreesBefore + 1; id <= receipt.TerminalSubtreesAfter; id++ {
		view, err := compact.Subtree(core.SubtreeID(id))
		if err != nil {
			return err
		}
		receipt.TerminalPayloadViews = append(receipt.TerminalPayloadViews, diagnosticParserCoreTerminalPayloadView(id, view))
	}
	sharedPayloads, err := diagnosticParserCoreShiftPayloadIDs(compact, shifted)
	if err != nil {
		return err
	}
	receipt.SemanticTerminalIdentities = 1
	receipt.DistinctTerminalPayloads = uint32(len(sharedPayloads))
	if receipt.DistinctTerminalPayloads > receipt.TerminalPayloadAllocations {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "packed identifier payload accounting underflow"}
	}
	receipt.DuplicateTerminalPayloads = receipt.TerminalPayloadAllocations - receipt.DistinctTerminalPayloads
	if receipt.TerminalSubtreesBefore != 194 || receipt.TerminalSubtreesAfter != 195 ||
		receipt.TerminalNodesAfter-receipt.TerminalNodesBefore != 2 ||
		receipt.TerminalLinksAfter-receipt.TerminalLinksBefore != 2 ||
		receipt.TerminalPayloadAllocations != 1 || receipt.TerminalPayloadsPerCohort != 1 ||
		receipt.SemanticTerminalIdentities != 1 || receipt.DistinctTerminalPayloads != 1 || receipt.DuplicateTerminalPayloads != 0 {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "packed identifier physical payload measurement mismatch"}
	}
	terminal := receipt.TerminalPayloadViews[0]
	if terminal.Symbol != 86 || terminal.ProductionID != 0 || terminal.DynamicPrecedence != 0 ||
		terminal.StartByte != 740 || terminal.EndByte != 741 || terminal.Extra || !terminal.Terminal ||
		len(terminal.Children) != 0 || len(terminal.Fields) != 0 || len(terminal.Aliases) != 0 {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "packed identifier terminal payload identity mismatch"}
	}
	return nil
}

func diagnosticParserCoreTerminalPayloadView(id uint32, view core.SubtreeView) DiagnosticParserCoreTerminalPayloadView {
	converted := DiagnosticParserCoreTerminalPayloadView{
		ID: id, Symbol: Symbol(view.Symbol), ProductionID: view.ProductionID,
		DynamicPrecedence: view.DynamicPrecedence, StartByte: view.StartByte, EndByte: view.EndByte,
		Extra: view.Extra, Terminal: view.Terminal,
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

func diagnosticParserCoreShiftPayloadIDs(compact *core.Core, shifted []core.Head) (map[core.SubtreeID]struct{}, error) {
	payloads := make(map[core.SubtreeID]struct{})
	for _, shiftedHead := range shifted {
		derivations, err := compact.Derivations(shiftedHead)
		if err != nil {
			return nil, err
		}
		for _, derivation := range derivations {
			if len(derivation.Payloads) == 0 {
				return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "packed identifier shift omitted terminal payload"}
			}
			payloads[derivation.Payloads[len(derivation.Payloads)-1]] = struct{}{}
		}
	}
	return payloads, nil
}

func executeDiagnosticParserCoreDotConflictFanout(
	compact *core.Core,
	tokenSource *dfaTokenSource,
	scannerScratch *[]byte,
	packed *DiagnosticParserCorePackedConvergence,
	electionIndex int,
	dispatches uint64,
	options DiagnosticParserCorePrefixOptions,
	branchOrder uint64,
	nextCreationSeq uint64,
) (*diagnosticParserCoreDotConflictExecution, error) {
	if electionIndex != 101 || branchOrder != 3 || nextCreationSeq != 4 || packed == nil {
		return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "dot conflict allocator identity mismatch"}
	}
	if uint64(electionIndex) >= options.MaxTokens {
		return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreCap, detail: "dot conflict election token cap"}
	}
	receipt := &DiagnosticParserCoreDotConflictFanout{}
	var finalHeaders []diagnosticParserCoreHeader
	err := compact.ApplyAtomic(func() error {
		header, before, election, err := readDiagnosticParserCoreDotElection(
			compact, tokenSource, scannerScratch, packed,
		)
		if err != nil {
			return err
		}
		receipt.ElectionIndex = electionIndex
		receipt.ElectionExpectedBefore = packed.Election.ScannerAfter
		receipt.Election = election
		receipt.ElectionBefore = before
		if err := compact.BeginFrontier(); err != nil {
			return err
		}
		compact.SetPhaseCheckpoint(election.ScannerAfter.SHA256)
		header.shifted = false
		header.checkpoint = election.ScannerAfter.SHA256
		receipt.ElectionReset, err = diagnosticParserCoreHeaderReceipt(compact, header)
		if err != nil {
			return err
		}
		beforeStats, err := compact.Stats(header.head)
		if err != nil {
			return err
		}
		actions, err := compact.Actions(core.StateID(receipt.ElectionReset.State), core.Symbol(election.Token.Symbol))
		if err != nil {
			return err
		}
		wantActions := []core.Action{
			{Type: core.ActionReduce, Symbol: 171, ChildCount: 1},
			{Type: core.ActionShift, State: 194},
		}
		if !reflect.DeepEqual(actions, wantActions) {
			return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "dot conflict action cell mismatch"}
		}
		if dispatches >= options.MaxDispatches {
			return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreCap, detail: "dot conflict dispatch cap"}
		}
		dispatches++
		headers, round, nextOrder, nextSeq, err := executeDiagnosticParserCoreConflict(
			compact, header, 0, election.Token, actions, branchOrder, nextCreationSeq,
		)
		if err != nil {
			return err
		}
		round.Index = 0
		receipt.ConflictRound = round
		if err := authenticateDiagnosticParserCoreDotFanout(compact, headers, nextOrder, nextSeq, receipt); err != nil {
			return err
		}
		if err := measureDiagnosticParserCoreDotFanout(compact, headers, beforeStats, receipt); err != nil {
			return err
		}
		receipt.GlobalBranchOrder = nextOrder
		receipt.NextCreationSeq = nextSeq
		receipt.Dispatches = dispatches
		finalHeaders = headers
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &diagnosticParserCoreDotConflictExecution{receipt: receipt, headers: finalHeaders}, nil
}

func readDiagnosticParserCoreDotElection(
	compact *core.Core,
	tokenSource *dfaTokenSource,
	scannerScratch *[]byte,
	packed *DiagnosticParserCorePackedConvergence,
) (diagnosticParserCoreHeader, DiagnosticParserCoreHeaderPathReceipt, DiagnosticParserCoreElection, error) {
	if packed.Packed.State != 186 || packed.Packed.ByteOffset != 741 || packed.Packed.CreationSeq != 1 || !packed.Packed.Shifted || packed.Packed.ExactPaths != 2 ||
		packed.GlobalBranchOrder != 3 || packed.NextCreationSeq != 4 || !reflect.DeepEqual(packed.Derivations, []DiagnosticParserCorePackedDerivation{
		{Score: -11, BranchOrder: 1, HasBranchOrder: true},
		{Score: -10, BranchOrder: 3, HasBranchOrder: true},
	}) {
		return diagnosticParserCoreHeader{}, DiagnosticParserCoreHeaderPathReceipt{}, DiagnosticParserCoreElection{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "dot election packed-header identity mismatch"}
	}
	head, ok := compact.CanonicalBoundary(186, 741, true, packed.Packed.Checkpoint)
	if !ok {
		return diagnosticParserCoreHeader{}, DiagnosticParserCoreHeaderPathReceipt{}, DiagnosticParserCoreElection{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "dot election packed head is not canonical"}
	}
	header := diagnosticParserCoreHeader{
		head: head, creationSeq: 1, shifted: true, checkpoint: packed.Packed.Checkpoint,
	}
	before, err := diagnosticParserCoreHeaderPaths(compact, header)
	if err != nil {
		return diagnosticParserCoreHeader{}, DiagnosticParserCoreHeaderPathReceipt{}, DiagnosticParserCoreElection{}, err
	}
	if !reflect.DeepEqual(before.Derivations, packed.Derivations) {
		return diagnosticParserCoreHeader{}, DiagnosticParserCoreHeaderPathReceipt{}, DiagnosticParserCoreElection{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "dot election packed derivations drifted"}
	}
	tokenSource.SetParserState(186)
	tokenSource.SetGLRStates(nil)
	beforeCheckpoint := parserCoreCheckpoint(append([]byte(nil), tokenSource.captureExternalScannerStateInto(scannerScratch)...))
	if beforeCheckpoint != packed.Election.ScannerAfter {
		return diagnosticParserCoreHeader{}, DiagnosticParserCoreHeaderPathReceipt{}, DiagnosticParserCoreElection{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "dot election scanner continuity failed"}
	}
	token := tokenSource.Next()
	afterCheckpoint := parserCoreCheckpoint(append([]byte(nil), tokenSource.captureExternalScannerStateInto(scannerScratch)...))
	current, currentStart, currentEnd, currentValid := currentExternalScannerCheckpoint(tokenSource)
	election := DiagnosticParserCoreElection{
		States: []StateID{186}, Token: token,
		ScannerBefore: beforeCheckpoint, ScannerAfter: afterCheckpoint,
		CurrentCheckpointValid: currentValid,
		CurrentCheckpointStart: parserCoreCheckpoint(current.start),
		CurrentCheckpointEnd:   parserCoreCheckpoint(current.end),
		CurrentCheckpointBytes: [2]uint32{currentStart, currentEnd},
	}
	empty := parserCoreCheckpoint(nil)
	if token.Symbol != 4 || token.Text != "." || token.StartByte != 741 || token.EndByte != 742 ||
		token.Missing || token.NoLookahead || token.ExternalScannerToken || beforeCheckpoint != empty || afterCheckpoint != empty ||
		currentValid || election.CurrentCheckpointStart != empty || election.CurrentCheckpointEnd != empty || election.CurrentCheckpointBytes != [2]uint32{} {
		return diagnosticParserCoreHeader{}, DiagnosticParserCoreHeaderPathReceipt{}, DiagnosticParserCoreElection{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "dot election token identity mismatch"}
	}
	return header, before, election, nil
}

func diagnosticParserCoreHeaderPaths(compact *core.Core, header diagnosticParserCoreHeader) (DiagnosticParserCoreHeaderPathReceipt, error) {
	receipt, err := diagnosticParserCoreHeaderReceipt(compact, header)
	if err != nil {
		return DiagnosticParserCoreHeaderPathReceipt{}, err
	}
	paths, err := compact.Derivations(header.head)
	if err != nil {
		return DiagnosticParserCoreHeaderPathReceipt{}, err
	}
	out := DiagnosticParserCoreHeaderPathReceipt{Header: receipt}
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

func authenticateDiagnosticParserCoreDotFanout(
	compact *core.Core,
	headers []diagnosticParserCoreHeader,
	branchOrder uint64,
	nextSeq uint64,
	receipt *DiagnosticParserCoreDotConflictFanout,
) error {
	if branchOrder != 4 || nextSeq != 6 || len(headers) != 3 {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "dot conflict allocator/cardinality mismatch"}
	}
	wantActions := []DiagnosticParserCoreRoundAction{
		{HeaderIndex: 0, State: 186, ByteOffset: 741, Ordinal: 1, Action: ParseAction{Type: ParseActionShift, State: 194}, BranchOrder: 4},
		{HeaderIndex: 0, State: 186, ByteOffset: 741, Ordinal: 0, Action: ParseAction{Type: ParseActionReduce, Symbol: 171, ChildCount: 1}},
	}
	if !reflect.DeepEqual(receipt.ConflictRound.Actions, wantActions) {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "dot conflict execution order mismatch"}
	}
	wantHeaders := []DiagnosticParserCoreHeaderReceipt{
		{CreationSeq: 1, State: 520, ByteOffset: 741, ExactPaths: 1, Checkpoint: receipt.Election.ScannerAfter.SHA256},
		{CreationSeq: 5, State: 407, ByteOffset: 741, ExactPaths: 1, Checkpoint: receipt.Election.ScannerAfter.SHA256},
		{CreationSeq: 4, State: 194, ByteOffset: 742, Shifted: true, ExactPaths: 2, Checkpoint: receipt.Election.ScannerAfter.SHA256},
	}
	if !reflect.DeepEqual(receipt.ConflictRound.After, wantHeaders) {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "dot conflict header order/identity mismatch"}
	}
	wantDerivations := [][]DiagnosticParserCorePackedDerivation{
		{{Score: -11, BranchOrder: 1, HasBranchOrder: true}},
		{{Score: -10, BranchOrder: 3, HasBranchOrder: true}},
		{
			{Score: -11, BranchOrder: 4, HasBranchOrder: true},
			{Score: -10, BranchOrder: 4, HasBranchOrder: true},
		},
	}
	for index, header := range headers {
		pathReceipt, err := diagnosticParserCoreHeaderPaths(compact, header)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(pathReceipt.Header, wantHeaders[index]) || !reflect.DeepEqual(pathReceipt.Derivations, wantDerivations[index]) {
			return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "dot conflict derivation identity mismatch"}
		}
		receipt.Headers = append(receipt.Headers, pathReceipt)
		receipt.LogicalPaths += pathReceipt.Header.ExactPaths
	}
	if receipt.LogicalPaths != 4 {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "dot conflict logical path count mismatch"}
	}
	return nil
}

func measureDiagnosticParserCoreDotFanout(
	compact *core.Core,
	headers []diagnosticParserCoreHeader,
	before core.Stats,
	receipt *DiagnosticParserCoreDotConflictFanout,
) error {
	after, err := compact.Stats(headers[0].head)
	if err != nil {
		return err
	}
	receipt.NodesBefore, receipt.NodesAfter = before.Nodes, after.Nodes
	receipt.LinksBefore, receipt.LinksAfter = before.Links, after.Links
	receipt.SubtreesBefore, receipt.SubtreesAfter = before.Subtrees, after.Subtrees
	receipt.ChildrenBefore, receipt.ChildrenAfter = before.Children, after.Children
	for _, header := range headers[:2] {
		paths, err := compact.Derivations(header.head)
		if err != nil {
			return err
		}
		if len(paths) != 1 || len(paths[0].Payloads) == 0 {
			return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "dot reduction parent path identity mismatch"}
		}
		receipt.ReductionParentPayloadIDs = append(receipt.ReductionParentPayloadIDs, uint32(paths[0].Payloads[len(paths[0].Payloads)-1]))
	}
	for id := before.Subtrees + 1; id <= after.Subtrees; id++ {
		view, err := compact.Subtree(core.SubtreeID(id))
		if err != nil {
			return err
		}
		receipt.NewPayloadViews = append(receipt.NewPayloadViews, diagnosticParserCoreTerminalPayloadView(id, view))
	}
	if after.Nodes-before.Nodes != 3 || after.Links-before.Links != 3 || after.Subtrees-before.Subtrees != 2 || after.Children-before.Children != 1 ||
		before.Subtrees != 195 || after.Subtrees != 197 || len(receipt.NewPayloadViews) != 2 {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "dot conflict physical work mismatch"}
	}
	dot, parent := receipt.NewPayloadViews[0], receipt.NewPayloadViews[1]
	if dot.ID != 196 || dot.Symbol != 4 || dot.StartByte != 741 || dot.EndByte != 742 || dot.Extra || !dot.Terminal ||
		len(dot.Children) != 0 || len(dot.Fields) != 0 || len(dot.Aliases) != 0 {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "dot terminal payload identity mismatch"}
	}
	if !reflect.DeepEqual(receipt.ReductionParentPayloadIDs, []uint32{197, 197}) || parent.ID != 197 || parent.Symbol != 171 || parent.ProductionID != 0 || parent.DynamicPrecedence != 0 ||
		parent.StartByte != 740 || parent.EndByte != 741 || parent.Extra || parent.Terminal || !reflect.DeepEqual(parent.Children, []uint32{195}) || len(parent.Fields) != 0 || len(parent.Aliases) != 0 {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "dot reduction parent payload identity mismatch"}
	}
	receipt.SemanticReductionParents = 1
	receipt.DistinctReductionParents = 1
	receipt.DuplicateReductionParents = 0
	return nil
}

func executeDiagnosticParserCoreCachedDotClosure(
	compact *core.Core,
	tokenSource *dfaTokenSource,
	scannerScratch *[]byte,
	headers []diagnosticParserCoreHeader,
	dot *DiagnosticParserCoreDotConflictFanout,
	electionIndex int,
	dispatches uint64,
	tokens uint64,
	options DiagnosticParserCorePrefixOptions,
) (*DiagnosticParserCoreCachedDotClosure, []diagnosticParserCoreHeader, error) {
	if dot == nil || electionIndex != 102 || tokens != 102 || dispatches != 196 ||
		dot.GlobalBranchOrder != 4 || dot.NextCreationSeq != 6 || len(headers) != 3 {
		return nil, nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "cached-dot closure allocator/frontier identity mismatch"}
	}
	trialHeaders := append([]diagnosticParserCoreHeader(nil), headers...)
	receipt := &DiagnosticParserCoreCachedDotClosure{}
	var tokenSnapshot dfaRelexSnapshot
	var tokenState StateID
	var tokenGLRStates []StateID
	tokenSnapshotValid := false
	err := compact.ApplyAtomic(func() error {
		plan, err := planDiagnosticParserCoreCachedDotClosure(compact, trialHeaders, dot, dispatches, options.MaxDispatches)
		if err != nil {
			return err
		}
		trialHeaders, err = applyDiagnosticParserCoreCachedDotClosure(compact, trialHeaders, dot, plan, receipt)
		if err != nil {
			return err
		}
		if err := runDiagnosticParserCoreCachedDotFault(diagnosticParserCoreCachedDotFaultAfterCohort, compact, headers, trialHeaders, tokenSource); err != nil {
			return err
		}
		if tokens >= options.MaxTokens {
			return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreCap, detail: "cached-dot next-election token cap"}
		}
		electionBefore, err := diagnosticParserCoreHeaderPathReceipts(compact, trialHeaders)
		if err != nil {
			return err
		}
		tokenSnapshot = tokenSource.snapshotRelexState()
		tokenState = tokenSource.state
		tokenGLRStates = append([]StateID(nil), tokenSource.glrStates...)
		tokenSnapshotValid = true
		election, nextActions, err := readDiagnosticParserCoreCachedDotElection(
			compact, tokenSource, scannerScratch, trialHeaders, dot.Election.ScannerAfter,
		)
		if err != nil {
			return err
		}
		if err := runDiagnosticParserCoreCachedDotFault(diagnosticParserCoreCachedDotFaultAfterElection, compact, headers, trialHeaders, tokenSource); err != nil {
			return err
		}
		receipt.ElectionIndex = electionIndex
		receipt.ElectionExpectedBefore = dot.Election.ScannerAfter
		receipt.ElectionBefore = electionBefore
		receipt.Election = election
		receipt.NextActions = nextActions
		return nil
	})
	if err != nil {
		restoreDiagnosticParserCoreCachedDotTokenSource(tokenSource, tokenSnapshotValid, tokenSnapshot, tokenState, tokenGLRStates)
		_ = runDiagnosticParserCoreCachedDotFault(diagnosticParserCoreCachedDotFaultAfterRollback, compact, headers, trialHeaders, tokenSource)
		return nil, nil, err
	}
	return receipt, trialHeaders, nil
}

func runDiagnosticParserCoreCachedDotFault(
	point diagnosticParserCoreCachedDotFaultPoint,
	compact *core.Core,
	callerHeaders []diagnosticParserCoreHeader,
	trialHeaders []diagnosticParserCoreHeader,
	tokenSource *dfaTokenSource,
) error {
	if diagnosticParserCoreCachedDotFaultHook == nil {
		return nil
	}
	return diagnosticParserCoreCachedDotFaultHook(point, compact, callerHeaders, trialHeaders, tokenSource)
}

func restoreDiagnosticParserCoreCachedDotTokenSource(
	tokenSource *dfaTokenSource,
	valid bool,
	snapshot dfaRelexSnapshot,
	state StateID,
	glrStates []StateID,
) {
	if !valid {
		return
	}
	snapshot.restore(tokenSource)
	tokenSource.state = state
	tokenSource.glrStates = append([]StateID(nil), glrStates...)
}

type diagnosticParserCoreCachedDotPlan struct {
	before      []DiagnosticParserCoreHeaderPathReceipt
	beforeStats core.Stats
	cohort      []core.OrdinaryCohortShiftInput
	actions     []DiagnosticParserCoreRoundAction
	dispatches  uint64
}

func planDiagnosticParserCoreCachedDotClosure(
	compact *core.Core,
	headers []diagnosticParserCoreHeader,
	dot *DiagnosticParserCoreDotConflictFanout,
	dispatches uint64,
	maxDispatches uint64,
) (diagnosticParserCoreCachedDotPlan, error) {
	before, err := diagnosticParserCoreHeaderPathReceipts(compact, headers)
	if err != nil {
		return diagnosticParserCoreCachedDotPlan{}, err
	}
	if !reflect.DeepEqual(before, dot.Headers) {
		return diagnosticParserCoreCachedDotPlan{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "cached-dot closure input drifted from committed fanout"}
	}
	if dispatches+2 > maxDispatches {
		return diagnosticParserCoreCachedDotPlan{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreCap, detail: "cached-dot closure dispatch cap"}
	}
	stats, err := compact.Stats(headers[0].head)
	if err != nil {
		return diagnosticParserCoreCachedDotPlan{}, err
	}
	cohort := make([]core.OrdinaryCohortShiftInput, 2)
	actions := make([]DiagnosticParserCoreRoundAction, 2)
	for index, wantState := range []StateID{520, 407} {
		if err := authenticateDiagnosticParserCoreCachedDotRunnable(compact, headers[index], before[index].Header, dot.Election.Token.Symbol, wantState); err != nil {
			return diagnosticParserCoreCachedDotPlan{}, err
		}
		cohort[index] = core.OrdinaryCohortShiftInput{Head: headers[index].head, ActionOrdinal: 0}
		actions[index] = DiagnosticParserCoreRoundAction{
			HeaderIndex: index, State: wantState, ByteOffset: 741,
			Action: ParseAction{Type: ParseActionShift, State: 164},
		}
	}
	if !diagnosticParserCoreCachedDotRetained(before[2].Header) {
		return diagnosticParserCoreCachedDotPlan{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "cached-dot retained header identity mismatch"}
	}
	return diagnosticParserCoreCachedDotPlan{
		before: before, beforeStats: stats, cohort: cohort, actions: actions, dispatches: dispatches + 2,
	}, nil
}

func authenticateDiagnosticParserCoreCachedDotRunnable(
	compact *core.Core,
	header diagnosticParserCoreHeader,
	receipt DiagnosticParserCoreHeaderReceipt,
	lookahead Symbol,
	wantState StateID,
) error {
	if receipt.State != wantState || receipt.ByteOffset != 741 || receipt.Shifted || receipt.Accepted || receipt.ExactPaths != 1 {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "cached-dot runnable header identity mismatch"}
	}
	cell, err := compact.Actions(core.StateID(receipt.State), core.Symbol(lookahead))
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(cell, []core.Action{{Type: core.ActionShift, State: 164}}) {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "cached-dot runnable action cell mismatch"}
	}
	if header.head.Node == 0 {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "cached-dot runnable compact head missing"}
	}
	return nil
}

func diagnosticParserCoreCachedDotRetained(header DiagnosticParserCoreHeaderReceipt) bool {
	return header.State == 194 && header.ByteOffset == 742 && header.CreationSeq == 4 && header.Shifted && !header.Accepted && header.ExactPaths == 2
}

func applyDiagnosticParserCoreCachedDotClosure(
	compact *core.Core,
	headers []diagnosticParserCoreHeader,
	dot *DiagnosticParserCoreDotConflictFanout,
	plan diagnosticParserCoreCachedDotPlan,
	receipt *DiagnosticParserCoreCachedDotClosure,
) ([]diagnosticParserCoreHeader, error) {
	receipt.Before = plan.before
	receipt.RunnableBefore = []DiagnosticParserCoreHeaderReceipt{plan.before[0].Header, plan.before[1].Header}
	receipt.RetainedBefore = plan.before[2]
	shifted, err := compact.ShiftOrdinaryCohort(plan.cohort, core.Symbol(dot.Election.Token.Symbol), core.Token{
		Symbol: core.Symbol(dot.Election.Token.Symbol), StartByte: dot.Election.Token.StartByte, EndByte: dot.Election.Token.EndByte,
	})
	if err != nil {
		return nil, err
	}
	if len(shifted) != 2 {
		return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "cached-dot runnable cohort cardinality mismatch"}
	}
	for index := range shifted {
		headers[index].head = shifted[index]
		headers[index].shifted = true
	}
	beforeCanonical, err := diagnosticParserCoreHeaderReceipts(compact, headers)
	if err != nil {
		return nil, err
	}
	receipt.BeforeCanonical = beforeCanonical
	receipt.ShiftRound = DiagnosticParserCoreDispatchRound{
		Index: 0, Before: append([]DiagnosticParserCoreHeaderReceipt(nil), receipt.RunnableBefore...), Actions: plan.actions,
		After: append([]DiagnosticParserCoreHeaderReceipt(nil), beforeCanonical[:2]...),
	}
	headers, err = canonicalizeDiagnosticParserCoreHeaders(compact, headers)
	if err != nil {
		return nil, err
	}
	if err := authenticateDiagnosticParserCoreCachedDotHeaders(compact, headers, receipt); err != nil {
		return nil, err
	}
	if err := measureDiagnosticParserCoreCachedDotClosure(compact, headers, plan.beforeStats, receipt); err != nil {
		return nil, err
	}
	receipt.GlobalBranchOrder = dot.GlobalBranchOrder
	receipt.NextCreationSeq = dot.NextCreationSeq
	receipt.Dispatches = plan.dispatches
	return headers, nil
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

func authenticateDiagnosticParserCoreCachedDotHeaders(
	compact *core.Core,
	headers []diagnosticParserCoreHeader,
	receipt *DiagnosticParserCoreCachedDotClosure,
) error {
	if len(headers) != 2 {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "cached-dot canonical frontier cardinality mismatch"}
	}
	wantHeaders := []DiagnosticParserCoreHeaderReceipt{
		{CreationSeq: 1, State: 164, ByteOffset: 742, Shifted: true, ExactPaths: 2, Checkpoint: sha256.Sum256(nil)},
		{CreationSeq: 4, State: 194, ByteOffset: 742, Shifted: true, ExactPaths: 2, Checkpoint: sha256.Sum256(nil)},
	}
	wantDerivations := [][]DiagnosticParserCorePackedDerivation{
		{{Score: -11, BranchOrder: 1, HasBranchOrder: true}, {Score: -10, BranchOrder: 3, HasBranchOrder: true}},
		{{Score: -11, BranchOrder: 4, HasBranchOrder: true}, {Score: -10, BranchOrder: 4, HasBranchOrder: true}},
	}
	for index, header := range headers {
		pathReceipt, err := diagnosticParserCoreHeaderPaths(compact, header)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(pathReceipt.Header, wantHeaders[index]) || !reflect.DeepEqual(pathReceipt.Derivations, wantDerivations[index]) {
			return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "cached-dot canonical derivation identity mismatch"}
		}
		receipt.Headers = append(receipt.Headers, pathReceipt)
		receipt.LogicalPaths += pathReceipt.Header.ExactPaths
	}
	if receipt.LogicalPaths != 4 {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "cached-dot logical path count mismatch"}
	}
	return nil
}

func measureDiagnosticParserCoreCachedDotClosure(
	compact *core.Core,
	headers []diagnosticParserCoreHeader,
	before core.Stats,
	receipt *DiagnosticParserCoreCachedDotClosure,
) error {
	after, err := compact.Stats(headers[0].head)
	if err != nil {
		return err
	}
	receipt.NodesBefore, receipt.NodesAfter = before.Nodes, after.Nodes
	receipt.LinksBefore, receipt.LinksAfter = before.Links, after.Links
	receipt.SubtreesBefore, receipt.SubtreesAfter = before.Subtrees, after.Subtrees
	receipt.ChildrenBefore, receipt.ChildrenAfter = before.Children, after.Children
	if before.Nodes != 206 || after.Nodes != 208 || before.Links != 205 || after.Links != 207 ||
		before.Subtrees != 197 || after.Subtrees != 198 || before.Children != 184 || after.Children != 184 {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "cached-dot closure physical work mismatch"}
	}
	if err := authenticateDiagnosticParserCoreCachedDotTerminal(compact, receipt); err != nil {
		return err
	}
	return collectDiagnosticParserCoreCachedDotTerminalIDs(compact, headers, receipt)
}

func authenticateDiagnosticParserCoreCachedDotTerminal(
	compact *core.Core,
	receipt *DiagnosticParserCoreCachedDotClosure,
) error {
	view, err := compact.Subtree(198)
	if err != nil {
		return err
	}
	receipt.TerminalPayload = diagnosticParserCoreTerminalPayloadView(198, view)
	terminal := receipt.TerminalPayload
	if terminal.ID != 198 || terminal.Symbol != 4 || terminal.ProductionID != 0 || terminal.DynamicPrecedence != 0 ||
		terminal.StartByte != 741 || terminal.EndByte != 742 || terminal.Extra || !terminal.Terminal ||
		len(terminal.Children) != 0 || len(terminal.Fields) != 0 || len(terminal.Aliases) != 0 {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "cached-dot cohort terminal identity mismatch"}
	}
	retainedView, err := compact.Subtree(196)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(view, retainedView) {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "cached-dot election-wide terminal identity mismatch"}
	}
	return nil
}

func collectDiagnosticParserCoreCachedDotTerminalIDs(
	compact *core.Core,
	headers []diagnosticParserCoreHeader,
	receipt *DiagnosticParserCoreCachedDotClosure,
) error {
	for index, header := range headers {
		derivations, err := compact.Derivations(header.head)
		if err != nil {
			return err
		}
		for _, derivation := range derivations {
			if len(derivation.Payloads) == 0 {
				return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "cached-dot terminal path identity missing"}
			}
			id := uint32(derivation.Payloads[len(derivation.Payloads)-1])
			if index == 0 {
				receipt.PrimaryTerminalPayloadIDs = append(receipt.PrimaryTerminalPayloadIDs, id)
			} else {
				receipt.RetainedTerminalPayloadIDs = append(receipt.RetainedTerminalPayloadIDs, id)
			}
		}
	}
	if !reflect.DeepEqual(receipt.PrimaryTerminalPayloadIDs, []uint32{198, 198}) || !reflect.DeepEqual(receipt.RetainedTerminalPayloadIDs, []uint32{196, 196}) {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "cached-dot primary/retained terminal ownership mismatch"}
	}
	return nil
}

func readDiagnosticParserCoreCachedDotElection(
	compact *core.Core,
	tokenSource *dfaTokenSource,
	scannerScratch *[]byte,
	headers []diagnosticParserCoreHeader,
	expectedBefore DiagnosticParserCoreScannerCheckpoint,
) (DiagnosticParserCoreElection, []DiagnosticParserCoreRoundAction, error) {
	states := []StateID{164, 194}
	if err := authenticateDiagnosticParserCoreCachedDotElectionHeaders(compact, headers, states, expectedBefore); err != nil {
		return DiagnosticParserCoreElection{}, nil, err
	}
	election, err := electDiagnosticParserCoreCachedDotToken(tokenSource, scannerScratch, states, expectedBefore)
	if err != nil {
		return DiagnosticParserCoreElection{}, nil, err
	}
	actions, err := authenticateDiagnosticParserCoreCachedDotNextActions(compact, states, election.Token.Symbol)
	if err != nil {
		return DiagnosticParserCoreElection{}, nil, err
	}
	return election, actions, nil
}

func authenticateDiagnosticParserCoreCachedDotElectionHeaders(
	compact *core.Core,
	headers []diagnosticParserCoreHeader,
	states []StateID,
	expectedBefore DiagnosticParserCoreScannerCheckpoint,
) error {
	if len(headers) != 2 {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "cached-dot election frontier cardinality mismatch"}
	}
	for index, header := range headers {
		receipt, err := diagnosticParserCoreHeaderReceipt(compact, header)
		if err != nil {
			return err
		}
		if receipt.State != states[index] || receipt.ByteOffset != 742 || !receipt.Shifted || receipt.Accepted || receipt.ExactPaths != 2 || receipt.Checkpoint != expectedBefore.SHA256 {
			return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "cached-dot election ordered frontier mismatch"}
		}
	}
	return nil
}

func electDiagnosticParserCoreCachedDotToken(
	tokenSource *dfaTokenSource,
	scannerScratch *[]byte,
	states []StateID,
	expectedBefore DiagnosticParserCoreScannerCheckpoint,
) (DiagnosticParserCoreElection, error) {
	tokenSource.SetParserState(states[0])
	tokenSource.SetGLRStates(append([]StateID(nil), states...))
	before := parserCoreCheckpoint(append([]byte(nil), tokenSource.captureExternalScannerStateInto(scannerScratch)...))
	if before != expectedBefore {
		return DiagnosticParserCoreElection{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "cached-dot election scanner continuity failed"}
	}
	token := tokenSource.Next()
	after := parserCoreCheckpoint(append([]byte(nil), tokenSource.captureExternalScannerStateInto(scannerScratch)...))
	current, currentStart, currentEnd, currentValid := currentExternalScannerCheckpoint(tokenSource)
	election := DiagnosticParserCoreElection{
		States: append([]StateID(nil), states...), Token: token,
		ScannerBefore: before, ScannerAfter: after,
		CurrentCheckpointValid: currentValid,
		CurrentCheckpointStart: parserCoreCheckpoint(current.start),
		CurrentCheckpointEnd:   parserCoreCheckpoint(current.end),
		CurrentCheckpointBytes: [2]uint32{currentStart, currentEnd},
	}
	empty := parserCoreCheckpoint(nil)
	if token.Symbol != 86 || token.Text != "edits" || token.StartByte != 742 || token.EndByte != 747 ||
		token.Missing || token.NoLookahead || token.ExternalScannerToken || before != empty || after != empty || currentValid ||
		election.CurrentCheckpointStart != empty || election.CurrentCheckpointEnd != empty || election.CurrentCheckpointBytes != [2]uint32{} {
		return DiagnosticParserCoreElection{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "cached-dot next-election token identity mismatch"}
	}
	return election, nil
}

func authenticateDiagnosticParserCoreCachedDotNextActions(
	compact *core.Core,
	states []StateID,
	lookahead Symbol,
) ([]DiagnosticParserCoreRoundAction, error) {
	wantTargets := []StateID{410, 444}
	actions := make([]DiagnosticParserCoreRoundAction, len(states))
	for index, state := range states {
		cell, err := compact.Actions(core.StateID(state), core.Symbol(lookahead))
		if err != nil {
			return nil, err
		}
		want := []core.Action{{Type: core.ActionShift, State: core.StateID(wantTargets[index])}}
		if !reflect.DeepEqual(cell, want) {
			return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "cached-dot next-election action cell mismatch"}
		}
		actions[index] = DiagnosticParserCoreRoundAction{
			HeaderIndex: index, State: state, ByteOffset: 742,
			Action: ParseAction{Type: ParseActionShift, State: wantTargets[index]},
		}
	}
	return actions, nil
}

type diagnosticParserCoreGenericScheduler struct {
	compact        *core.Core
	tokenSource    *dfaTokenSource
	scannerScratch *[]byte
	headers        []diagnosticParserCoreHeader
	token          Token
	checkpoint     DiagnosticParserCoreScannerCheckpoint
	electionIndex  int
	tokens         uint64
	dispatches     uint64
	branchOrder    uint64
	nextSeq        uint64
	options        DiagnosticParserCorePrefixOptions
	receipt        *DiagnosticParserCoreGenericScheduler
	work           DiagnosticParserCoreGenericWork
	epochProgress  bool
}

type diagnosticParserCoreGenericCell struct {
	headerIndex int
	receipt     DiagnosticParserCoreHeaderReceipt
	actions     []core.Action
}

func executeDiagnosticParserCoreGenericScheduler(
	compact *core.Core,
	tokenSource *dfaTokenSource,
	scannerScratch *[]byte,
	headers []diagnosticParserCoreHeader,
	closure *DiagnosticParserCoreCachedDotClosure,
	dispatches uint64,
	tokens uint64,
	options DiagnosticParserCorePrefixOptions,
) (*DiagnosticParserCoreGenericScheduler, error) {
	if closure == nil || len(headers) == 0 {
		return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "generic scheduler requires a closed elected frontier"}
	}
	tokenSnapshot := tokenSource.snapshotRelexState()
	tokenState := tokenSource.state
	tokenGLRStates := append([]StateID(nil), tokenSource.glrStates...)
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact, tokenSource: tokenSource, scannerScratch: scannerScratch,
		headers: append([]diagnosticParserCoreHeader(nil), headers...),
		token:   closure.Election.Token, checkpoint: closure.Election.ScannerAfter,
		electionIndex: closure.ElectionIndex, tokens: tokens, dispatches: dispatches,
		branchOrder: closure.GlobalBranchOrder, nextSeq: closure.NextCreationSeq,
		options: options,
		receipt: &DiagnosticParserCoreGenericScheduler{
			StartElectionIndex: closure.ElectionIndex, StartToken: closure.Election.Token,
		},
	}
	// The cached-dot route elected edits but deliberately did not start its
	// compact frontier or reset scheduler consumption flags.
	for index := range scheduler.headers {
		scheduler.headers[index].shifted = false
		scheduler.headers[index].accepted = false
		scheduler.headers[index].checkpoint = closure.Election.ScannerAfter.SHA256
	}
	err := compact.ApplyAtomic(func() error {
		if err := compact.BeginFrontier(); err != nil {
			return err
		}
		compact.SetPhaseCheckpoint(closure.Election.ScannerAfter.SHA256)
		start, err := diagnosticParserCoreHeaderPathReceipts(compact, scheduler.headers)
		if err != nil {
			return err
		}
		scheduler.receipt.StartHeaders = start
		return scheduler.run()
	})
	if err != nil {
		restoreDiagnosticParserCoreCachedDotTokenSource(tokenSource, true, tokenSnapshot, tokenState, tokenGLRStates)
		return nil, err
	}
	return scheduler.receipt, nil
}

func (s *diagnosticParserCoreGenericScheduler) run() error {
	for {
		if uint64(len(s.headers)) > s.work.PeakHeaders {
			s.work.PeakHeaders = uint64(len(s.headers))
		}
		allClosed := true
		for _, header := range s.headers {
			if !header.shifted && !header.accepted {
				allClosed = false
				break
			}
		}
		if allClosed {
			if err := s.elect(); err != nil {
				return err
			}
			continue
		}
		stop, err := s.dispatchPass()
		if err != nil {
			return err
		}
		if stop != nil {
			return s.finish(stop.boundary, stop.detail, stop.headerIndex)
		}
	}
}

type diagnosticParserCoreGenericUnsupported struct {
	boundary    DiagnosticParserCoreBoundaryKind
	detail      string
	headerIndex int
}

func (s *diagnosticParserCoreGenericScheduler) dispatchPass() (*diagnosticParserCoreGenericUnsupported, error) {
	s.work.Passes++
	if unsupported := diagnosticParserCoreGenericUnsupportedToken(s.token); unsupported != nil {
		return unsupported, nil
	}
	before, err := diagnosticParserCoreHeaderReceipts(s.compact, s.headers)
	if err != nil {
		return nil, err
	}
	var cells []diagnosticParserCoreGenericCell
	var noActionIndices []int
	for index, header := range s.headers {
		if header.shifted || header.accepted {
			continue
		}
		receipt := before[index]
		actions, err := s.compact.Actions(core.StateID(receipt.State), core.Symbol(s.token.Symbol))
		if err != nil {
			return nil, err
		}
		s.work.ActionLookups++
		if len(actions) == 0 {
			noActionIndices = append(noActionIndices, index)
			continue
		}
		if unsupported := diagnosticParserCoreGenericUnsupportedCell(index, s.token, actions); unsupported != nil {
			return unsupported, nil
		}
		cells = append(cells, diagnosticParserCoreGenericCell{headerIndex: index, receipt: receipt, actions: actions})
	}
	if len(cells) == 0 {
		if diagnosticParserCoreGenericNoActionDropEligible(s.headers, noActionIndices, s.epochProgress) {
			return nil, s.dropGenericNoActionHeads(noActionIndices)
		}
		if len(noActionIndices) != 0 {
			return &diagnosticParserCoreGenericUnsupported{
				boundary:    DiagnosticParserCoreNoAction,
				detail:      "generic scheduler has only paused no-action heads for the elected token",
				headerIndex: noActionIndices[0],
			}, nil
		}
		return &diagnosticParserCoreGenericUnsupported{
			boundary: DiagnosticParserCoreRoute, detail: "generic scheduler has no runnable head", headerIndex: 0,
		}, nil
	}

	// One reduction-bearing cell is applied per pass. This deliberately
	// reclassifies the complete frontier before any shift is allowed.
	for _, cell := range cells {
		if !diagnosticParserCoreActionsContain(cell.actions, core.ActionReduce) {
			continue
		}
		if len(cell.actions) > 1 {
			return nil, s.applyGenericConflict(before, cell)
		}
		if cell.actions[0].Type == core.ActionReduce {
			return nil, s.applyGenericReduction(before, cell)
		}
	}
	for _, cell := range cells {
		if len(cell.actions) > 1 {
			return nil, s.applyGenericConflict(before, cell)
		}
	}

	shiftCells := make([]diagnosticParserCoreGenericCell, 0, len(cells))
	for _, cell := range cells {
		if cell.actions[0].Type == core.ActionShift {
			shiftCells = append(shiftCells, cell)
		}
	}
	if len(shiftCells) == 0 {
		return &diagnosticParserCoreGenericUnsupported{
			boundary: DiagnosticParserCoreRoute, detail: "generic scheduler pass made no progress", headerIndex: cells[0].headerIndex,
		}, nil
	}
	return nil, s.applyGenericShifts(before, shiftCells)
}

func diagnosticParserCoreActionsContain(actions []core.Action, actionType core.ActionType) bool {
	for _, action := range actions {
		if action.Type == actionType {
			return true
		}
	}
	return false
}

func diagnosticParserCoreGenericNoActionDropEligible(headers []diagnosticParserCoreHeader, noActionIndices []int, epochProgress bool) bool {
	if !epochProgress || len(noActionIndices) == 0 || len(noActionIndices) >= len(headers) {
		return false
	}
	noAction := make(map[int]struct{}, len(noActionIndices))
	for _, index := range noActionIndices {
		if index < 0 || index >= len(headers) {
			return false
		}
		noAction[index] = struct{}{}
	}
	for index, header := range headers {
		if _, paused := noAction[index]; paused {
			continue
		}
		if header.shifted || header.accepted {
			return true
		}
	}
	return false
}

func (s *diagnosticParserCoreGenericScheduler) dropGenericNoActionHeads(indices []int) error {
	paused := make(map[int]struct{}, len(indices))
	for _, index := range indices {
		paused[index] = struct{}{}
	}
	pathReceipts, err := diagnosticParserCoreHeaderPathReceipts(s.compact, s.headers)
	if err != nil {
		return err
	}
	kept := make([]diagnosticParserCoreHeader, 0, len(s.headers)-len(indices))
	for index, header := range s.headers {
		if _, drop := paused[index]; !drop {
			kept = append(kept, header)
			continue
		}
		s.receipt.NoActionDrops = append(s.receipt.NoActionDrops, DiagnosticParserCoreGenericNoActionDrop{
			ElectionIndex: s.electionIndex, Token: s.token, Header: pathReceipts[index],
		})
	}
	if len(kept) == 0 {
		return errors.New("parser-core phase zero: sibling-backed no-action drop removed the complete frontier")
	}
	s.headers = kept
	s.work.NoActionDrops += uint64(len(indices))
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) applyGenericReduction(before []DiagnosticParserCoreHeaderReceipt, cell diagnosticParserCoreGenericCell) error {
	if err := s.reserveDispatches(1); err != nil {
		return err
	}
	heads, err := s.compact.Reduce(s.headers[cell.headerIndex].head, core.Symbol(s.token.Symbol), 0, core.ForkOrder{})
	if err != nil {
		return err
	}
	if len(heads) == 0 {
		return errors.New("parser-core phase zero: generic reduction produced no boundary")
	}
	replacements := make([]diagnosticParserCoreHeader, len(heads))
	for index, head := range heads {
		replacement := s.headers[cell.headerIndex]
		replacement.head = head
		if index > 0 {
			replacement.creationSeq = s.nextSeq
			s.nextSeq++
		}
		replacements[index] = replacement
	}
	s.headers = replaceDiagnosticParserCoreHeader(s.headers, cell.headerIndex, replacements)
	s.epochProgress = true
	s.work.Reductions++
	s.work.Dispatches++
	if err := s.canonicalize(); err != nil {
		return err
	}
	after, err := diagnosticParserCoreHeaderReceipts(s.compact, s.headers)
	if err != nil {
		return err
	}
	s.receipt.Rounds = append(s.receipt.Rounds, DiagnosticParserCoreDispatchRound{
		Index: len(s.receipt.Rounds), Before: before,
		Actions: []DiagnosticParserCoreRoundAction{{
			HeaderIndex: cell.headerIndex, State: cell.receipt.State, ByteOffset: cell.receipt.ByteOffset,
			Ordinal: 0, Action: rootParserCoreAction(cell.actions[0]),
		}},
		After: after,
	})
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) applyGenericConflict(before []DiagnosticParserCoreHeaderReceipt, cell diagnosticParserCoreGenericCell) (err error) {
	headersBefore := append([]diagnosticParserCoreHeader(nil), s.headers...)
	dispatchesBefore, branchOrderBefore, nextSeqBefore := s.dispatches, s.branchOrder, s.nextSeq
	workBefore, epochProgressBefore := s.work, s.epochProgress
	roundsBefore, conflictsBefore := len(s.receipt.Rounds), len(s.receipt.Conflicts)
	defer func() {
		if err == nil {
			return
		}
		s.headers = headersBefore
		s.dispatches, s.branchOrder, s.nextSeq = dispatchesBefore, branchOrderBefore, nextSeqBefore
		s.work, s.epochProgress = workBefore, epochProgressBefore
		s.receipt.Rounds = s.receipt.Rounds[:roundsBefore]
		s.receipt.Conflicts = s.receipt.Conflicts[:conflictsBefore]
	}()
	if err = s.reserveDispatches(1); err != nil {
		return err
	}
	execution, err := executeDiagnosticParserCoreConflictDetailed(
		s.compact, s.headers[cell.headerIndex], cell.headerIndex, s.token, cell.actions,
		s.branchOrder, s.nextSeq,
	)
	if err != nil {
		return err
	}
	prefix := s.headers[:cell.headerIndex]
	suffix := s.headers[cell.headerIndex+1:]
	primaryReceipts, err := diagnosticParserCoreHeaderReceipts(s.compact, execution.primaries)
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
	secondaryArms := make([]DiagnosticParserCoreGenericConflictArm, len(execution.secondaryArms))
	for index, arm := range execution.secondaryArms {
		outputs, receiptErr := diagnosticParserCoreHeaderReceipts(s.compact, arm)
		if receiptErr != nil {
			return receiptErr
		}
		secondaryArms[index] = DiagnosticParserCoreGenericConflictArm{
			Ordinal: index + 1, BranchOrder: branchOrderBefore + uint64(index) + 1, Outputs: outputs,
		}
	}
	headers := make([]diagnosticParserCoreHeader, 0, len(execution.primaries)+len(prefix)+len(suffix)+len(execution.secondaries))
	headers = append(headers, prefix...)
	headers = append(headers, execution.primaries[0])
	headers = append(headers, suffix...)
	headers = append(headers, execution.secondaries...)
	headers = append(headers, execution.primaries[1:]...)
	s.headers = headers
	s.branchOrder, s.nextSeq = execution.branchOrder, execution.nextSeq
	s.epochProgress = true
	s.work.Conflicts++
	s.work.ConflictActions += uint64(len(cell.actions))
	s.work.Forks += uint64(len(cell.actions) - 1)
	s.work.ConflictHeads += uint64(len(execution.primaries) + len(execution.secondaries))
	s.work.Dispatches++
	if err := s.canonicalize(); err != nil {
		return err
	}
	after, err := diagnosticParserCoreHeaderReceipts(s.compact, s.headers)
	if err != nil {
		return err
	}
	round := execution.round
	round.Index = len(s.receipt.Rounds)
	round.Before = before
	round.After = after
	s.receipt.Rounds = append(s.receipt.Rounds, round)
	s.receipt.Conflicts = append(s.receipt.Conflicts, DiagnosticParserCoreGenericConflict{
		ElectionIndex: s.electionIndex, Token: s.token, HeaderIndex: cell.headerIndex,
		BranchOrderBefore: branchOrderBefore, BranchOrderAfter: s.branchOrder,
		NextCreationSeqBefore: nextSeqBefore, NextCreationSeqAfter: s.nextSeq,
		Round: round, Prefix: prefixReceipts, PrimaryOutput: primaryReceipts[0], OriginalSuffix: suffixReceipts,
		SecondaryArms: secondaryArms, AdditionalPrimaryOutputs: primaryReceipts[1:], After: after,
	})
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) applyGenericShifts(before []DiagnosticParserCoreHeaderReceipt, cells []diagnosticParserCoreGenericCell) error {
	if err := s.reserveDispatches(uint64(len(cells))); err != nil {
		return err
	}
	if len(cells) == 1 {
		cell := cells[0]
		head, err := s.compact.Shift(s.headers[cell.headerIndex].head, core.Symbol(s.token.Symbol), 0, core.Token{
			Symbol: core.Symbol(s.token.Symbol), StartByte: s.token.StartByte, EndByte: s.token.EndByte,
		}, core.ForkOrder{})
		if err != nil {
			return err
		}
		s.headers[cell.headerIndex].head = head
		s.headers[cell.headerIndex].shifted = true
	} else {
		inputs := make([]core.OrdinaryCohortShiftInput, len(cells))
		for index, cell := range cells {
			inputs[index] = core.OrdinaryCohortShiftInput{Head: s.headers[cell.headerIndex].head, ActionOrdinal: 0}
		}
		heads, err := s.compact.ShiftOrdinaryCohort(inputs, core.Symbol(s.token.Symbol), core.Token{
			Symbol: core.Symbol(s.token.Symbol), StartByte: s.token.StartByte, EndByte: s.token.EndByte,
		})
		if err != nil {
			return err
		}
		for index, cell := range cells {
			s.headers[cell.headerIndex].head = heads[index]
			s.headers[cell.headerIndex].shifted = true
		}
		s.work.OrdinaryCohorts++
	}
	s.epochProgress = true
	actions := make([]DiagnosticParserCoreRoundAction, len(cells))
	for index, cell := range cells {
		actions[index] = DiagnosticParserCoreRoundAction{
			HeaderIndex: cell.headerIndex, State: cell.receipt.State, ByteOffset: cell.receipt.ByteOffset,
			Ordinal: 0, Action: rootParserCoreAction(cell.actions[0]),
		}
	}
	s.work.OrdinaryShifts += uint64(len(cells))
	s.work.Dispatches += uint64(len(cells))
	if err := s.canonicalize(); err != nil {
		return err
	}
	after, err := diagnosticParserCoreHeaderReceipts(s.compact, s.headers)
	if err != nil {
		return err
	}
	s.receipt.Rounds = append(s.receipt.Rounds, DiagnosticParserCoreDispatchRound{
		Index: len(s.receipt.Rounds), Before: before, Actions: actions, After: after,
	})
	return nil
}

func diagnosticParserCoreGenericUnsupportedCell(headerIndex int, token Token, actions []core.Action) *diagnosticParserCoreGenericUnsupported {
	unsupported := func(boundary DiagnosticParserCoreBoundaryKind, detail string) *diagnosticParserCoreGenericUnsupported {
		return &diagnosticParserCoreGenericUnsupported{boundary: boundary, detail: detail, headerIndex: headerIndex}
	}
	if len(actions) == 0 {
		return unsupported(DiagnosticParserCoreNoAction, "generic scheduler reached an empty action cell")
	}
	for _, action := range actions {
		if action.Repetition {
			return unsupported(DiagnosticParserCoreRoute, "generic scheduler does not support repetition shifts")
		}
		if action.ExtraChain {
			return unsupported(DiagnosticParserCoreExtraChain, "generic scheduler does not support extra-chain shifts")
		}
		if action.Extra {
			return unsupported(DiagnosticParserCoreExtra, "generic scheduler does not yet apply extra shifts")
		}
		switch action.Type {
		case core.ActionReduce:
		case core.ActionShift:
			if token.EndByte <= token.StartByte {
				return unsupported(DiagnosticParserCoreRoute, "generic scheduler ordinary shift is not positive-width")
			}
		case core.ActionRecover:
			return unsupported(DiagnosticParserCoreRecovery, "generic scheduler reached recovery")
		case core.ActionAccept:
			return unsupported(DiagnosticParserCoreAccept, "generic scheduler reached accept before materialization")
		default:
			return unsupported(DiagnosticParserCoreRoute, "generic scheduler reached an unknown action")
		}
	}
	return nil
}

func diagnosticParserCoreGenericUnsupportedToken(token Token) *diagnosticParserCoreGenericUnsupported {
	switch {
	case token.NoLookahead:
		return &diagnosticParserCoreGenericUnsupported{
			boundary: DiagnosticParserCoreRoute, detail: "generic scheduler does not support no-lookahead tokens",
		}
	case token.Missing:
		return &diagnosticParserCoreGenericUnsupported{
			boundary: DiagnosticParserCoreRoute, detail: "generic scheduler does not support missing tokens",
		}
	case token.ExternalScannerToken:
		return &diagnosticParserCoreGenericUnsupported{
			boundary: DiagnosticParserCoreRoute, detail: "generic scheduler does not yet carry external-token checkpoint identity",
		}
	default:
		return nil
	}
}

func replaceDiagnosticParserCoreHeader(headers []diagnosticParserCoreHeader, index int, replacements []diagnosticParserCoreHeader) []diagnosticParserCoreHeader {
	out := make([]diagnosticParserCoreHeader, 0, len(headers)-1+len(replacements))
	out = append(out, headers[:index]...)
	out = append(out, replacements...)
	out = append(out, headers[index+1:]...)
	return out
}

func (s *diagnosticParserCoreGenericScheduler) canonicalize() error {
	headers, err := canonicalizeDiagnosticParserCoreHeaders(s.compact, s.headers)
	if err != nil {
		return err
	}
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

func (s *diagnosticParserCoreGenericScheduler) elect() error {
	if s.tokens >= s.options.MaxTokens {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreCap, detail: "generic scheduler token cap"}
	}
	states := make([]StateID, len(s.headers))
	for index, header := range s.headers {
		receipt, err := diagnosticParserCoreHeaderReceipt(s.compact, header)
		if err != nil {
			return err
		}
		if !receipt.Shifted || receipt.Accepted || receipt.Checkpoint != s.checkpoint.SHA256 {
			return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "generic scheduler election frontier is not closed and checkpoint-continuous"}
		}
		states[index] = receipt.State
	}
	s.tokenSource.SetParserState(states[0])
	s.tokenSource.SetGLRStates(append([]StateID(nil), states...))
	before := parserCoreCheckpoint(append([]byte(nil), s.tokenSource.captureExternalScannerStateInto(s.scannerScratch)...))
	if before != s.checkpoint {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "generic scheduler scanner checkpoint continuity failed"}
	}
	token := s.tokenSource.Next()
	after := parserCoreCheckpoint(append([]byte(nil), s.tokenSource.captureExternalScannerStateInto(s.scannerScratch)...))
	current, currentStart, currentEnd, currentValid := currentExternalScannerCheckpoint(s.tokenSource)
	if err := s.compact.BeginFrontier(); err != nil {
		return err
	}
	s.compact.SetPhaseCheckpoint(after.SHA256)
	for index := range s.headers {
		s.headers[index].shifted = false
		s.headers[index].checkpoint = after.SHA256
	}
	s.electionIndex++
	s.tokens++
	s.work.Elections++
	s.token = token
	s.checkpoint = after
	s.epochProgress = false
	s.receipt.Elections = append(s.receipt.Elections, DiagnosticParserCoreElection{
		States: states, Token: token, ScannerBefore: before, ScannerAfter: after,
		CurrentCheckpointValid: currentValid,
		CurrentCheckpointStart: parserCoreCheckpoint(current.start),
		CurrentCheckpointEnd:   parserCoreCheckpoint(current.end),
		CurrentCheckpointBytes: [2]uint32{currentStart, currentEnd},
	})
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) finish(boundary DiagnosticParserCoreBoundaryKind, detail string, headerIndex int) error {
	paths, err := diagnosticParserCoreHeaderPathReceipts(s.compact, s.headers)
	if err != nil {
		return err
	}
	if headerIndex < 0 || headerIndex >= len(paths) {
		return errors.New("parser-core phase zero: generic stop header index out of range")
	}
	stats, err := s.compact.Stats(s.headers[headerIndex].head)
	if err != nil {
		return err
	}
	header := paths[headerIndex].Header
	s.receipt.Stop = DiagnosticParserCoreGenericStop{
		Boundary: boundary, Detail: detail, ElectionIndex: s.electionIndex,
		HeaderIndex: headerIndex, State: header.State, ByteOffset: header.ByteOffset,
		Token: s.token, Headers: paths, Stats: stats, Work: s.work,
	}
	s.receipt.Tokens = s.tokens
	s.receipt.Dispatches = s.dispatches
	s.receipt.GlobalBranchOrder = s.branchOrder
	s.receipt.NextCreationSeq = s.nextSeq
	return nil
}

func readDiagnosticParserCorePostCondenseElection(
	tokenSource *dfaTokenSource,
	scannerScratch *[]byte,
	state StateID,
	expectedBefore DiagnosticParserCoreScannerCheckpoint,
	wantSymbol Symbol,
	wantText string,
	wantStart uint32,
	wantEnd uint32,
) (DiagnosticParserCoreElection, error) {
	// Token does not carry Tree-sitter's extra bit; that property belongs to
	// the selected parse action. Both callers separately authenticate exact
	// action rows whose Extra fields are false.
	tokenSource.SetParserState(state)
	tokenSource.SetGLRStates(nil)
	before := parserCoreCheckpoint(append([]byte(nil), tokenSource.captureExternalScannerStateInto(scannerScratch)...))
	if before != expectedBefore {
		return DiagnosticParserCoreElection{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "post-condense scanner checkpoint continuity failed"}
	}
	token := tokenSource.Next()
	after := parserCoreCheckpoint(append([]byte(nil), tokenSource.captureExternalScannerStateInto(scannerScratch)...))
	current, currentStart, currentEnd, currentValid := currentExternalScannerCheckpoint(tokenSource)
	election := DiagnosticParserCoreElection{
		States: []StateID{state}, Token: token,
		ScannerBefore: before, ScannerAfter: after,
		CurrentCheckpointValid: currentValid,
		CurrentCheckpointStart: parserCoreCheckpoint(current.start),
		CurrentCheckpointEnd:   parserCoreCheckpoint(current.end),
		CurrentCheckpointBytes: [2]uint32{currentStart, currentEnd},
	}
	emptyCheckpoint := parserCoreCheckpoint(nil)
	if token.Symbol != wantSymbol || token.Text != wantText || token.StartByte != wantStart || token.EndByte != wantEnd ||
		token.Missing || token.NoLookahead || token.ExternalScannerToken || before != emptyCheckpoint || after != emptyCheckpoint ||
		currentValid || election.CurrentCheckpointStart != emptyCheckpoint || election.CurrentCheckpointEnd != emptyCheckpoint ||
		election.CurrentCheckpointBytes != [2]uint32{} {
		return DiagnosticParserCoreElection{}, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "post-condense ordinary-DFA token identity mismatch"}
	}
	return election, nil
}

// validateDiagnosticParserCoreState22Condense admits only the C-oracle-pinned
// competition reached after the second ordered cohort pass. The paused state22
// version owns one open recovery region and one skipped tree (500+100); its
// shifted state248 sibling is clean. Dropping the paused agenda entry is the
// complete phase-zero operation: the immutable compact graph is not rewritten
// and the paused version is never resumed.
func validateDiagnosticParserCoreState22Condense(
	compact *core.Core,
	token Token,
	headers []diagnosticParserCoreHeader,
	pausedIndex int,
	electionIndex int,
) (DiagnosticParserCoreCohortCondense, []diagnosticParserCoreHeader, error) {
	before, err := diagnosticParserCoreHeaderReceipts(compact, headers)
	if err != nil {
		return DiagnosticParserCoreCohortCondense{}, nil, err
	}
	if len(headers) != 2 || pausedIndex != 1 || electionIndex != 97 ||
		token.Symbol != 10 || token.Text != "=" || token.StartByte != 731 || token.EndByte != 732 {
		return DiagnosticParserCoreCohortCondense{}, nil, &diagnosticParserCoreDecline{
			boundary: DiagnosticParserCoreRoute,
			detail:   "state22 condense requires the exact second ordered-cohort token",
		}
	}
	preserved, paused := before[0], before[1]
	if preserved.State != 248 || preserved.ByteOffset != 732 || preserved.CreationSeq != 1 ||
		!preserved.Shifted || preserved.Accepted || preserved.ExactPaths != 1 ||
		paused.State != 22 || paused.ByteOffset != 730 || paused.CreationSeq != 2 ||
		paused.Shifted || paused.Accepted || paused.ExactPaths != 1 ||
		preserved.Checkpoint != paused.Checkpoint || preserved.Checkpoint != sha256.Sum256(nil) {
		return DiagnosticParserCoreCohortCondense{}, nil, &diagnosticParserCoreDecline{
			boundary: DiagnosticParserCoreRoute,
			detail:   "state22 condense frontier identity mismatch",
		}
	}
	beforeBoundaries, exactPaths, err := diagnosticParserCoreLaterForkBoundaries(compact, headers)
	if err != nil {
		return DiagnosticParserCoreCohortCondense{}, nil, err
	}
	if exactPaths != 2 || len(beforeBoundaries) != 2 ||
		beforeBoundaries[0].Score != -10 || beforeBoundaries[0].BranchOrder != 1 || !beforeBoundaries[0].HasBranchOrder ||
		beforeBoundaries[1].Score != -10 || beforeBoundaries[1].BranchOrder != 2 || !beforeBoundaries[1].HasBranchOrder {
		return DiagnosticParserCoreCohortCondense{}, nil, &diagnosticParserCoreDecline{
			boundary: DiagnosticParserCoreRoute,
			detail:   "state22 condense score/order identity mismatch",
		}
	}
	kept := []diagnosticParserCoreHeader{headers[0]}
	after, err := diagnosticParserCoreHeaderReceipts(compact, kept)
	if err != nil {
		return DiagnosticParserCoreCohortCondense{}, nil, err
	}
	afterBoundaries, afterPaths, err := diagnosticParserCoreLaterForkBoundaries(compact, kept)
	if err != nil {
		return DiagnosticParserCoreCohortCondense{}, nil, err
	}
	if afterPaths != 1 || len(afterBoundaries) != 1 || afterBoundaries[0].Score != -10 ||
		afterBoundaries[0].BranchOrder != 1 || !afterBoundaries[0].HasBranchOrder {
		return DiagnosticParserCoreCohortCondense{}, nil, &diagnosticParserCoreDecline{
			boundary: DiagnosticParserCoreRoute,
			detail:   "state22 condense preserved score/order identity mismatch",
		}
	}
	openRecoveryCost := uint32(cErrCostPerRecovery)
	skippedTreeCost := uint32(cErrCostPerSkippedTree)
	return DiagnosticParserCoreCohortCondense{
		Before: before, BeforeBoundaries: beforeBoundaries,
		Paused: paused, Preserved: preserved,
		After: after, AfterBoundaries: afterBoundaries,
		Token: token, ElectionIndex: electionIndex,
		PausedOpenRecoveryCost: openRecoveryCost, PausedSkippedTreeCost: skippedTreeCost,
		PausedEffectiveCost:    openRecoveryCost + skippedTreeCost,
		PreservedEffectiveCost: 0,
		PausedDropped:          true, PausedResumed: false, OraclePinned: true, Executed: true,
	}, kept, nil
}

func validateDiagnosticParserCoreFrozenOracleCondense(
	compact *core.Core,
	token Token,
	headers []diagnosticParserCoreHeader,
	runnable int,
	precedingScannerAfter DiagnosticParserCoreScannerCheckpoint,
) (DiagnosticParserCoreOracleCondenseResolution, diagnosticParserCoreHeader, error) {
	// diagnosticParserCoreHeader intentionally has no prior recovery/error
	// payload. Recovery routes are rejected before this scheduler is entered;
	// this validator applies the pinned C pause-and-cost decision only to the
	// two previously unrecovered lineages named below.
	decline := func(detail string) (DiagnosticParserCoreOracleCondenseResolution, diagnosticParserCoreHeader, error) {
		return DiagnosticParserCoreOracleCondenseResolution{}, diagnosticParserCoreHeader{}, &diagnosticParserCoreDecline{
			boundary: DiagnosticParserCoreNoAction, detail: detail,
		}
	}
	if compact == nil {
		return decline("frozen no-action resolution requires a compact core")
	}
	if token.Symbol != 20 || token.StartByte != 579 || token.EndByte != 580 || token.NoLookahead || token.ExternalScannerToken {
		return decline("no-action cell does not match the authenticated rewrite lookahead")
	}
	if len(headers) != 2 || runnable != 0 {
		return decline("oracle condense requires exactly one no-action primary and one shifted sibling")
	}
	receipts, err := diagnosticParserCoreHeaderReceipts(compact, headers)
	if err != nil {
		return DiagnosticParserCoreOracleCondenseResolution{}, diagnosticParserCoreHeader{}, err
	}
	paused, preserved := receipts[0], receipts[1]
	if paused.State != 254 || paused.ByteOffset != 579 || paused.CreationSeq != 0 || paused.Shifted || paused.Accepted || paused.ExactPaths != 1 {
		return decline("primary no-action header does not match authenticated state 254 at byte 579")
	}
	if preserved.State != 193 || preserved.ByteOffset != 580 || preserved.CreationSeq != 1 || !preserved.Shifted || preserved.Accepted || preserved.ExactPaths != 1 {
		return decline("shifted sibling does not match authenticated clean state 193 at byte 580")
	}
	if paused.Checkpoint != preserved.Checkpoint || paused.Checkpoint == ([32]byte{}) || precedingScannerAfter.SHA256 != preserved.Checkpoint {
		return decline("oracle-condense headers do not match the complete preceding scanner checkpoint")
	}
	if precedingScannerAfter.Length != 0 {
		return decline("frozen Go oracle-condense checkpoint length is not zero")
	}
	return DiagnosticParserCoreOracleCondenseResolution{
		Paused: paused, Preserved: preserved, Lookahead: token,
		PrecedingScannerAfter: precedingScannerAfter,
		PausedEffectiveCost:   cErrCostPerSkippedTree, PreservedEffectiveCost: 0,
		PausedDropped: true, PausedResumed: false, OraclePinned: true,
	}, headers[1], nil
}

func continueDiagnosticParserCoreFrozenOracleCondense(
	compact *core.Core,
	tokenSource *dfaTokenSource,
	scannerScratch *[]byte,
	token Token,
	headers []diagnosticParserCoreHeader,
	runnable int,
	branchOrder uint64,
	nextSeq uint64,
	options DiagnosticParserCorePrefixOptions,
	result *DiagnosticParserCorePrefixResult,
	resume *diagnosticParserCoreOuterResume,
) error {
	if resume == nil {
		return errors.New("parser-core phase zero: nil outer resume")
	}
	if len(result.Elections) == 0 {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "oracle condense requires a preceding scanner election"}
	}
	precedingScannerAfter := result.Elections[len(result.Elections)-1].ScannerAfter
	resolution, preserved, err := validateDiagnosticParserCoreFrozenOracleCondense(compact, token, headers, runnable, precedingScannerAfter)
	if err != nil {
		return err
	}
	if tokenSource == nil || scannerScratch == nil {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "frozen continuation requires the authenticated production DFA"}
	}
	if result.Tokens >= options.MaxTokens {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreCap, detail: "token cap before frozen continuation election"}
	}

	// Validate continuity before advancing the production token source. The
	// paused agenda entry is not dropped from the caller-visible receipt until
	// every guard succeeds.
	tokenSource.SetParserState(193)
	tokenSource.SetGLRStates(nil)
	before := append([]byte(nil), tokenSource.captureExternalScannerStateInto(scannerScratch)...)
	beforeReceipt := parserCoreCheckpoint(before)
	checkpointContinuous := beforeReceipt == resolution.PrecedingScannerAfter
	if !checkpointContinuous {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "scanner checkpoint continuity failed before frozen continuation election"}
	}

	// Next mutates the production lexer cursor. Every rejectable shape and
	// checkpoint guard is therefore checked above; failures after this point
	// are terminal diagnostic declines and never resume this token source.
	next := tokenSource.Next()
	after := append([]byte(nil), tokenSource.captureExternalScannerStateInto(scannerScratch)...)
	current, currentStart, currentEnd, currentValid := currentExternalScannerCheckpoint(tokenSource)
	election := DiagnosticParserCoreElection{
		States: []StateID{193}, Token: next,
		ScannerBefore: beforeReceipt, ScannerAfter: parserCoreCheckpoint(after),
		CurrentCheckpointValid: currentValid,
		CurrentCheckpointStart: parserCoreCheckpoint(current.start),
		CurrentCheckpointEnd:   parserCoreCheckpoint(current.end),
		CurrentCheckpointBytes: [2]uint32{currentStart, currentEnd},
	}
	if next.StartByte != resolution.Preserved.ByteOffset {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "frozen continuation election did not start at the preserved header"}
	}
	if err := compact.ApplyAtomic(func() error {
		if err := compact.BeginFrontier(); err != nil {
			return err
		}
		compact.SetPhaseCheckpoint(election.ScannerAfter.SHA256)
		return nil
	}); err != nil {
		return err
	}

	result.OracleCondenseResolution = &resolution
	result.Elections = append(result.Elections, election)
	result.Tokens++
	result.State, result.Lookahead = 193, next
	schedulerHeader := resolution.Preserved
	schedulerHeader.Shifted = false
	schedulerHeader.Checkpoint = election.ScannerAfter.SHA256
	result.ContinuationElection = &DiagnosticParserCoreContinuationElection{
		State: 193, ByteOffset: resolution.Preserved.ByteOffset,
		ElectionIndex:  len(result.Elections) - 1,
		ExpectedBefore: resolution.PrecedingScannerAfter,
		ActualBefore:   beforeReceipt, CheckpointContinuous: checkpointContinuous, Token: next,
		SchedulerHeader: schedulerHeader,
		HandoffBoundary: DiagnosticParserCoreSingleStateContinuation,
	}
	*resume = diagnosticParserCoreOuterResume{
		head: preserved.head, state: 193, token: next,
		creationSeq: preserved.creationSeq, branchOrder: branchOrder,
		nextCreationSeq: nextSeq, ready: true,
	}
	return nil
}

func setDiagnosticParserCoreBoundaryError(result *DiagnosticParserCorePrefixResult, err error) bool {
	var decline *diagnosticParserCoreDecline
	if errors.As(err, &decline) {
		result.Boundary, result.Detail = decline.boundary, decline.detail
		return true
	}
	if core.IsDecline(err, core.DeclineExtras) {
		result.Boundary, result.Detail = DiagnosticParserCoreExtra, err.Error()
		return true
	}
	return false
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
	CertifyCRecoveryCostCompetition(decoded)
	return decoded, nil
}

func applyParserCorePrefixAction(compact *core.Core, head core.Head, token Token, action core.Action, ordinal int, fork core.ForkOrder) ([]core.Head, error) {
	switch action.Type {
	case core.ActionShift:
		out, err := compact.Shift(head, core.Symbol(token.Symbol), ordinal, core.Token{Symbol: core.Symbol(token.Symbol), StartByte: token.StartByte, EndByte: token.EndByte, Extra: action.Extra}, fork)
		return []core.Head{out}, err
	case core.ActionReduce:
		return compact.Reduce(head, core.Symbol(token.Symbol), ordinal, fork)
	case core.ActionRecover:
		return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRecovery, detail: "recover action in first conflict"}
	case core.ActionAccept:
		return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreAccept, detail: "accept action in first conflict"}
	default:
		return nil, errors.New("parser-core phase zero: unknown conflict action")
	}
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
