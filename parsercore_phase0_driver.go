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
	DiagnosticParserCoreExtra         DiagnosticParserCoreBoundaryKind = "extra"
	DiagnosticParserCoreExtraChain    DiagnosticParserCoreBoundaryKind = "extra_chain"
	DiagnosticParserCoreNoAction      DiagnosticParserCoreBoundaryKind = "no_action"
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
	MaxDispatches           uint64
	MaxTokens               uint64
	Limits                  core.Limits
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
	ExtraShifts       uint64
	ExtraCohorts      uint64
	Accepts           uint64
	ReductionPauses   uint64
	NoActionDrops     uint64
	Elections         uint64
	Canonicalizations uint64
	PeakHeaders       uint64
}

// DiagnosticParserCoreGenericAcceptance records an authenticated EOF accept
// after the compact frontier has converged to one exact derivation. Payloads
// are the selected bottom-to-top compact stack; materialization does not
// mutate that graph.
type DiagnosticParserCoreGenericAcceptance struct {
	ElectionIndex   int
	Token           Token
	Header          DiagnosticParserCoreHeaderPathReceipt
	Payloads        []uint32
	Score           int64
	BranchOrder     uint64
	HasBranchOrder  bool
	CoreWork        core.Work
	Accepts         uint64
	SelectedNodes   uint64
	SelectedParents uint64
	SelectedLeaves  uint64
	Stats           core.Stats
	Work            DiagnosticParserCoreGenericWork
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
	ReceiptMode       DiagnosticParserCoreReceiptMode
	StartCheckpoint   DiagnosticParserCoreScannerCheckpoint
	StartHeaders      []DiagnosticParserCoreHeaderPathReceipt
	Rounds            []DiagnosticParserCoreDispatchRound
	Conflicts         []DiagnosticParserCoreGenericConflict
	ExternalShifts    []DiagnosticParserCoreGenericExternalShift
	Elections         []DiagnosticParserCoreElection
	NoActionDrops     []DiagnosticParserCoreGenericNoActionDrop
	Completion        *DiagnosticParserCoreGenericCompletion
	Acceptance        *DiagnosticParserCoreGenericAcceptance
	Stop              DiagnosticParserCoreGenericStop
	Tokens            uint64
	Dispatches        uint64
	GlobalBranchOrder uint64
	NextCreationSeq   uint64
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
	// one-shot compact-tree materialization succeed. Compact phase zero does not
	// retain the production parser's per-node reuse state or scanner checkpoints,
	// so the returned tree is explicitly barred from incremental reuse; passing
	// it to ParseIncremental takes the production parser's fresh-parse fallback.
	MaterializedTree *Tree
}

type diagnosticParserCoreDecline struct {
	boundary DiagnosticParserCoreBoundaryKind
	detail   string
}

//go:embed grammars/grammar_blobs/go.bin
var parserCoreCertifiedGoBlob []byte

func (e *diagnosticParserCoreDecline) Error() string { return string(e.boundary) + ": " + e.detail }

type parserCoreRootTables struct {
	parser     *Parser
	actionRows []core.ActionRow
}

func newParserCoreRootTables(parser *Parser) (*parserCoreRootTables, error) {
	if parser == nil || parser.language == nil {
		return nil, errors.New("parser-core phase zero: cannot cache actions without a parser language")
	}
	rows := make([]core.ActionRow, len(parser.language.ParseActions))
	for index, entry := range parser.language.ParseActions {
		converted := make([]core.Action, len(entry.Actions))
		for ordinal, action := range entry.Actions {
			var err error
			converted[ordinal], err = parserCoreAction(action)
			if err != nil {
				return nil, fmt.Errorf("parser-core phase zero: convert action row %d ordinal %d: %w", index, ordinal, err)
			}
		}
		rows[index] = core.NewActionRow(converted)
	}
	return &parserCoreRootTables{parser: parser, actionRows: rows}, nil
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

func (a *parserCoreRootTables) ProductionAliases(productionID uint16, childCount int) ([]core.Symbol, error) {
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
	tables, err := newParserCoreRootTables(parser)
	if err != nil {
		return result, err
	}
	compact, err := core.New(tables, options.Limits)
	if err != nil {
		return result, err
	}
	tokenSource := parser.acquireParserDFATokenSource(source)
	if tokenSource == nil {
		return result, errors.New("parser-core phase zero: production DFA unavailable")
	}
	defer tokenSource.Close()
	var scannerScratch []byte
	return diagnosticParseParserCoreGenericFromSeed(
		result, compact, tokenSource, &scannerScratch, parser, lang.InitialState, source, options,
	)
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
		return materializeDiagnosticParserCoreAcceptedTree(compact, head, parser, source)
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

type diagnosticParserCoreHeader struct {
	head        core.Head
	creationSeq uint64
	shifted     bool
	accepted    bool
	paused      bool
	freshness   core.ReductionFreshness
	checkpoint  [32]byte
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
		Paused:      header.paused,
		ExactPaths:  stats.CurrentExactPaths,
		Checkpoint:  header.checkpoint,
	}, nil
}

func diagnosticParserCoreHeaderSummary(compact *core.Core, header diagnosticParserCoreHeader) (DiagnosticParserCoreHeaderReceipt, error) {
	state, byteOffset, err := compact.Boundary(header.head)
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

func validateDiagnosticParserCoreCell(token Token, actions core.ActionRow) error {
	if token.NoLookahead {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "no-lookahead tokens require production recovery semantics"}
	}
	if actions.Len() == 0 {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreNoAction, detail: "canonical cell has no action"}
	}
	for ordinal := 0; ordinal < actions.Len(); ordinal++ {
		action := actions.At(ordinal)
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

type diagnosticParserCoreActionOutput struct {
	head      core.Head
	freshness core.ReductionFreshness
}

func executeDiagnosticParserCoreGenericConflictDetailed(
	compact *core.Core,
	incoming diagnosticParserCoreHeader,
	headerIndex int,
	token Token,
	actions core.ActionRow,
	branchOrder uint64,
	collectReceipts bool,
) (diagnosticParserCoreConflictExecution, error) {
	var before DiagnosticParserCoreHeaderReceipt
	if collectReceipts {
		var err error
		before, err = diagnosticParserCoreHeaderReceipt(compact, incoming)
		if err != nil {
			return diagnosticParserCoreConflictExecution{}, err
		}
	}
	if err := validateDiagnosticParserCoreCell(token, actions); err != nil {
		return diagnosticParserCoreConflictExecution{}, err
	}
	if actions.Len() < 2 {
		return diagnosticParserCoreConflictExecution{}, errors.New("parser-core phase zero: conflict executor requires multiple actions")
	}
	secondaryCount := uint64(actions.Len() - 1)
	if secondaryCount > math.MaxUint64-branchOrder {
		return diagnosticParserCoreConflictExecution{}, errors.New("parser-core phase zero: conflict branch order overflow")
	}
	trialOrder := branchOrder
	var primaries []diagnosticParserCoreHeader
	var secondaries []diagnosticParserCoreHeader
	var secondaryArms [][]diagnosticParserCoreHeader
	var receipts []DiagnosticParserCoreRoundAction
	err := compact.ApplyAtomic(func() error {
		for ordinal := 1; ordinal < actions.Len(); ordinal++ {
			action := actions.At(ordinal)
			trialOrder++
			outputs, applyErr := applyParserCoreConflictAction(compact, incoming.head, token, action, ordinal, core.ForkOrder{Present: true, Value: trialOrder})
			if applyErr != nil {
				return applyErr
			}
			arm := make([]diagnosticParserCoreHeader, 0, len(outputs))
			if len(outputs) != 0 {
				for _, output := range outputs {
					arm = append(arm, diagnosticParserCoreHeader{
						head: output.head, shifted: action.Type == core.ActionShift,
						freshness: output.freshness, checkpoint: incoming.checkpoint,
					})
				}
			}
			secondaryArms = append(secondaryArms, arm)
			secondaries = append(secondaries, arm...)
			if collectReceipts {
				receipts = append(receipts, DiagnosticParserCoreRoundAction{
					HeaderIndex: headerIndex, State: before.State, ByteOffset: before.ByteOffset,
					Ordinal: ordinal, Action: rootParserCoreAction(action), BranchOrder: trialOrder,
				})
			}
		}
		primaryAction := actions.At(0)
		outputs, applyErr := applyParserCoreConflictAction(compact, incoming.head, token, primaryAction, 0, core.ForkOrder{})
		if applyErr != nil {
			return applyErr
		}
		primaries = make([]diagnosticParserCoreHeader, len(outputs))
		for index, output := range outputs {
			primary := incoming
			primary.head = output.head
			primary.shifted = primaryAction.Type == core.ActionShift
			primary.freshness = output.freshness
			primaries[index] = primary
		}
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
		headers := append(primaries, secondaries...)
		after, err := diagnosticParserCoreHeaderReceipts(compact, headers)
		if err != nil {
			return diagnosticParserCoreConflictExecution{}, err
		}
		round = DiagnosticParserCoreDispatchRound{
			Before: []DiagnosticParserCoreHeaderReceipt{before}, Actions: receipts, After: after,
		}
	}
	return diagnosticParserCoreConflictExecution{
		primaries: primaries, secondaries: secondaries, secondaryArms: secondaryArms,
		round:       round,
		branchOrder: trialOrder,
	}, nil
}

type diagnosticParserCorePhaseHead struct {
	head       core.Head
	shifted    bool
	accepted   bool
	checkpoint [32]byte
}

type diagnosticParserCoreCanonicalScratch struct {
	headerBuffers [2][]diagnosticParserCoreHeader
	nextBuffer    uint8
	keys          []diagnosticParserCorePhaseHead
	winners       map[diagnosticParserCorePhaseHead]int
	runnable      map[diagnosticParserCorePhaseHead]bool
}

func (s *diagnosticParserCoreCanonicalScratch) canonicalize(compact *core.Core, headers []diagnosticParserCoreHeader) ([]diagnosticParserCoreHeader, error) {
	if s == nil {
		return nil, errors.New("parser-core phase zero: nil canonicalization scratch")
	}
	target := int(s.nextBuffer & 1)
	if len(headers) != 0 && cap(s.headerBuffers[target]) != 0 && &headers[0] == &s.headerBuffers[target][:1][0] {
		target ^= 1
	}
	normalized := s.headerBuffers[target]
	if cap(normalized) < len(headers) {
		normalized = make([]diagnosticParserCoreHeader, len(headers))
	} else {
		normalized = normalized[:len(headers)]
	}
	copy(normalized, headers)
	s.headerBuffers[target] = normalized
	if cap(s.keys) < len(headers) {
		s.keys = make([]diagnosticParserCorePhaseHead, len(headers))
	} else {
		s.keys = s.keys[:len(headers)]
	}
	if s.winners == nil {
		s.winners = make(map[diagnosticParserCorePhaseHead]int, len(headers))
	} else {
		clear(s.winners)
	}
	if s.runnable == nil {
		s.runnable = make(map[diagnosticParserCorePhaseHead]bool, len(headers))
	} else {
		clear(s.runnable)
	}
	for index, header := range normalized {
		state, byteOffset, err := compact.Boundary(header.head)
		if err != nil {
			return nil, err
		}
		if canonical, ok := compact.CanonicalBoundary(state, byteOffset, header.shifted, header.checkpoint); ok {
			header.head = canonical
		}
		key := diagnosticParserCorePhaseHead{head: header.head, shifted: header.shifted, accepted: header.accepted, checkpoint: header.checkpoint}
		normalized[index] = header
		s.keys[index] = key
		if !header.paused {
			s.runnable[key] = true
		}
		if existing, duplicate := s.winners[key]; duplicate {
			incumbent := normalized[existing]
			incumbentFresh := incumbent.freshness != 0
			headerFresh := header.freshness != 0
			if (incumbentFresh && !headerFresh) ||
				(incumbentFresh == headerFresh && incumbent.paused && !header.paused) {
				s.winners[key] = index
			}
		} else {
			s.winners[key] = index
		}
	}
	write := 0
	for index, header := range normalized {
		if s.winners[s.keys[index]] != index {
			continue
		}
		header.paused = !s.runnable[s.keys[index]]
		header.freshness = 0
		normalized[write] = header
		write++
	}
	out := normalized[:write]
	s.headerBuffers[target] = out
	s.nextBuffer = uint8(target ^ 1)
	return out, nil
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

type diagnosticParserCoreGenericScheduler struct {
	compact                    *core.Core
	tokenSource                *dfaTokenSource
	scannerScratch             *[]byte
	headers                    []diagnosticParserCoreHeader
	token                      Token
	checkpoint                 DiagnosticParserCoreScannerCheckpoint
	currentElection            DiagnosticParserCoreElection
	electionIndex              int
	tokens                     uint64
	dispatches                 uint64
	branchOrder                uint64
	nextSeq                    uint64
	options                    DiagnosticParserCorePrefixOptions
	receipt                    *DiagnosticParserCoreGenericScheduler
	summaryHeaderScratch       []DiagnosticParserCoreHeaderReceipt
	canonicalScratch           diagnosticParserCoreCanonicalScratch
	work                       DiagnosticParserCoreGenericWork
	epochProgress              bool
	acceptedHead               core.Head
	conflictPostExecutionFault func() error
	extraPostExecutionFault    func() error
	observer                   diagnosticParserCoreSeedObserver
	stoppedAfterElection       bool
}

func (s *diagnosticParserCoreGenericScheduler) fullReceipts() bool {
	return s != nil && s.options.ReceiptMode == DiagnosticParserCoreReceiptFull
}

func (s *diagnosticParserCoreGenericScheduler) headerReceipt(header diagnosticParserCoreHeader) (DiagnosticParserCoreHeaderReceipt, error) {
	if s.fullReceipts() {
		return diagnosticParserCoreHeaderReceipt(s.compact, header)
	}
	return diagnosticParserCoreHeaderSummary(s.compact, header)
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
// It can inspect closed frontiers immediately before an election and stop a
// seed-owned run immediately after an election, before any action dispatch.
type diagnosticParserCoreSeedObserver struct {
	beforeElection func(*diagnosticParserCoreGenericScheduler) error
	afterElection  func(*diagnosticParserCoreGenericScheduler) (bool, error)
}

func newDiagnosticParserCoreGenericScheduler(
	compact *core.Core,
	tokenSource *dfaTokenSource,
	scannerScratch *[]byte,
	head core.Head,
	checkpoint DiagnosticParserCoreScannerCheckpoint,
	observer diagnosticParserCoreSeedObserver,
	options DiagnosticParserCorePrefixOptions,
) (*diagnosticParserCoreGenericScheduler, error) {
	if compact == nil || tokenSource == nil || scannerScratch == nil || head.Node == 0 {
		return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "generic scheduler requires a compact core, token source, scanner scratch, and seed head"}
	}
	_, byteOffset, err := compact.Boundary(head)
	if err != nil {
		return nil, err
	}
	if byteOffset != 0 {
		return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "generic seed scheduler head is not at byte zero"}
	}
	header := diagnosticParserCoreHeader{head: head, checkpoint: checkpoint.SHA256}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact, tokenSource: tokenSource, scannerScratch: scannerScratch,
		headers: []diagnosticParserCoreHeader{header}, checkpoint: checkpoint,
		electionIndex: -1, nextSeq: 1,
		options: options, observer: observer,
		receipt: &DiagnosticParserCoreGenericScheduler{
			ReceiptMode:     options.ReceiptMode,
			StartCheckpoint: checkpoint,
		},
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

type diagnosticParserCoreGenericCell struct {
	headerIndex int
	receipt     DiagnosticParserCoreHeaderReceipt
	actions     core.ActionRow
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
	if compact == nil || tokenSource == nil || scannerScratch == nil {
		return nil, errors.New("parser-core phase zero: seed scheduler requires compact core and production token source")
	}
	head, err := compact.Seed(core.StateID(initialState), 0)
	if err != nil {
		return nil, err
	}
	tokenSource.SetParserState(initialState)
	tokenSource.SetGLRStates(nil)
	initialCheckpoint := parserCoreCheckpoint(append([]byte(nil), tokenSource.captureExternalScannerStateInto(scannerScratch)...))
	scheduler, err := newDiagnosticParserCoreGenericScheduler(
		compact, tokenSource, scannerScratch, head, initialCheckpoint, observer, options,
	)
	if err != nil {
		return nil, err
	}
	if err := scheduler.run(); err != nil {
		return scheduler, err
	}
	return scheduler, nil
}

type diagnosticParserCorePointIndex struct {
	lineStarts []uint32
}

func newDiagnosticParserCorePointIndex(source []byte, poll func() error) (diagnosticParserCorePointIndex, error) {
	if uint64(len(source)) > math.MaxUint32 {
		return diagnosticParserCorePointIndex{}, errors.New("parser-core phase zero: materialization source exceeds uint32 offsets")
	}
	starts := make([]uint32, 1, min(1024, 1+len(source)/32))
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

func (index diagnosticParserCorePointIndex) point(offset uint32) Point {
	line := sort.Search(len(index.lineStarts), func(i int) bool { return index.lineStarts[i] > offset }) - 1
	if line < 0 {
		return Point{Column: offset}
	}
	return Point{Row: uint32(line), Column: offset - index.lineStarts[line]}
}

func materializeDiagnosticParserCoreAcceptedTree(compact *core.Core, head core.Head, parser *Parser, source []byte) (*Tree, error) {
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
	stats, err := compact.Stats(head)
	if err != nil {
		return nil, err
	}

	arena := acquireNodeArena(arenaClassFull)
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
	order, err := compact.MaterializationOrder(derivations[0].Payloads, poll)
	if err != nil {
		return nil, err
	}
	if uint64(len(order)) > uint64(stats.Subtrees) {
		return nil, errors.New("parser-core phase zero: materialization order exceeds compact subtree arena")
	}
	points, err := newDiagnosticParserCorePointIndex(source, poll)
	if err != nil {
		return nil, err
	}
	nodesByID := make([]*Node, uint64(stats.Subtrees)+1)
	if err := poll(); err != nil {
		return nil, err
	}
	for orderIndex, id := range order {
		if orderIndex&255 == 0 {
			if err := poll(); err != nil {
				return nil, err
			}
		}
		view, err := compact.Subtree(id)
		if err != nil {
			return nil, err
		}
		if view.EndByte < view.StartByte || view.EndByte > uint32(len(source)) {
			return nil, errors.New("parser-core phase zero: compact subtree extent is outside source")
		}
		named := parser.isNamedSymbol(Symbol(view.Symbol))
		if view.Terminal {
			node := newLeafNodeInArena(
				arena, Symbol(view.Symbol), named, view.StartByte, view.EndByte,
				points.point(view.StartByte), points.point(view.EndByte),
			)
			node.setExtra(view.Extra)
			node.setExternalScannerToken(view.External)
			nodesByID[id] = node
			continue
		}

		entries := make([]stackEntry, len(view.Children))
		structuralChildren := 0
		for index, childID := range view.Children {
			if uint64(childID) >= uint64(len(nodesByID)) || nodesByID[childID] == nil {
				return nil, errors.New("parser-core phase zero: compact materialization order omitted a child")
			}
			child := nodesByID[childID]
			entries[index] = newStackEntryNode(0, child)
			if !child.isExtra() {
				structuralChildren++
			}
		}
		action := ParseAction{
			Type: ParseActionReduce, Symbol: Symbol(view.Symbol), ChildCount: uint8(structuralChildren),
			DynamicPrecedence: view.DynamicPrecedence, ProductionID: view.ProductionID,
		}
		if child := parser.collapsibleRawUnarySelfReduction(action, Token{}, arena, entries, 0, len(entries)); child != nil {
			child.productionID = view.ProductionID
			child.dynamicPrecedence += int32(view.DynamicPrecedence)
			nodesByID[id] = child
			continue
		}
		children, fieldIDs, fieldSources, _ := parser.buildReduceChildrenWithPath(
			entries, 0, len(entries), structuralChildren,
			Symbol(view.Symbol), view.ProductionID, arena,
		)
		if child := parser.collapsibleUnarySelfReduction(action, Token{}, arena, entries, 0, len(entries), children, fieldIDs); child != nil {
			child.productionID = view.ProductionID
			child.dynamicPrecedence += int32(view.DynamicPrecedence)
			nodesByID[id] = child
			continue
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
		nodesByID[id] = parent
	}

	nodes := make([]*Node, len(derivations[0].Payloads))
	for index, payload := range derivations[0].Payloads {
		if uint64(payload) >= uint64(len(nodesByID)) || nodesByID[payload] == nil {
			return nil, errors.New("parser-core phase zero: compact materialization order omitted an accepted payload")
		}
		nodes[index] = nodesByID[payload]
	}
	if err := poll(); err != nil {
		return nil, err
	}
	var linkScratch []*Node
	tree := parser.buildResultFromNodes(nodes, source, arena, nil, nil, &linkScratch)
	if tree != nil {
		owned = false // buildResultFromNodes transfers arena ownership to tree.
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
	if runtime := tree.ParseRuntime(); runtime.StopReason != ParseStopNone || runtime.Truncated || runtime.TokenSourceEOFEarly {
		return rejectTree(fmt.Errorf("parser-core phase zero: accepted-tree materialization returned an incomplete runtime: %s", runtime.Summary()))
	}
	if err := poll(); err != nil {
		return rejectTree(err)
	}
	sourceLen := uint32(len(source))
	root := tree.root
	if root.startByte != 0 || root.endByte != sourceLen || root.IsError() || root.HasError() {
		return rejectTree(fmt.Errorf("parser-core phase zero: accepted compact root is incomplete or erroneous: span=%d..%d source=%d error=%t", root.startByte, root.endByte, sourceLen, root.HasError()))
	}
	tree.incrementalReuseDisabled = true
	tree.setParseRuntime(ParseRuntime{
		StopReason: ParseStopAccepted, SourceLen: sourceLen, ExpectedEOFByte: sourceLen,
		RootEndByte: root.endByte, LastTokenEndByte: sourceLen, LastTokenSymbol: 0, LastTokenWasEOF: true,
	})
	return tree, nil
}

func (s *diagnosticParserCoreGenericScheduler) run() error {
	if err := s.elect(true); err != nil {
		return err
	}
	if s.stoppedAfterElection {
		s.publishTotals()
		return nil
	}
	for {
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
	for index, header := range s.headers {
		if header.accepted {
			return &diagnosticParserCoreGenericUnsupported{
				boundary: DiagnosticParserCoreAccept, detail: "generic scheduler found an accepted head before sole-frontier completion", headerIndex: index,
			}, nil
		}
	}
	before, err := s.headerReceipts(s.headers)
	if err != nil {
		return nil, err
	}
	var cells []diagnosticParserCoreGenericCell
	var noActionIndices []int
	for index, header := range s.headers {
		if header.shifted || header.accepted {
			continue
		}
		if header.paused {
			noActionIndices = append(noActionIndices, index)
			continue
		}
		receipt := before[index]
		actions, err := s.compact.Actions(core.StateID(receipt.State), core.Symbol(s.token.Symbol))
		if err != nil {
			return nil, err
		}
		s.work.ActionLookups++
		if actions.Len() == 0 {
			noActionIndices = append(noActionIndices, index)
			continue
		}
		cells = append(cells, diagnosticParserCoreGenericCell{headerIndex: index, receipt: receipt, actions: actions})
	}
	for _, cell := range cells {
		if unsupported := diagnosticParserCoreGenericUnsupportedCell(cell.headerIndex, s.token, cell.actions); unsupported != nil {
			return unsupported, nil
		}
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
	acceptCell := -1
	for index, cell := range cells {
		if diagnosticParserCoreActionsContain(cell.actions, core.ActionAccept) {
			acceptCell = index
			break
		}
	}
	if acceptCell >= 0 {
		cell := cells[acceptCell]
		if len(s.headers) != 1 || len(cells) != 1 || len(noActionIndices) != 0 || cell.headerIndex != 0 || cell.actions.Len() != 1 || cell.actions.At(0).Type != core.ActionAccept {
			return &diagnosticParserCoreGenericUnsupported{
				boundary: DiagnosticParserCoreAccept, detail: "generic scheduler requires a sole homogeneous accept frontier", headerIndex: cell.headerIndex,
			}, nil
		}
		return nil, s.applyGenericAccept(before, cell)
	}
	extraCells := 0
	for _, cell := range cells {
		if cell.actions.At(0).Extra {
			extraCells++
		}
	}
	if extraCells != 0 {
		if extraCells != len(cells) || len(cells) != len(s.headers) || len(noActionIndices) != 0 {
			return &diagnosticParserCoreGenericUnsupported{
				boundary: DiagnosticParserCoreExtra, detail: "generic scheduler requires a homogeneous all-runnable extra cohort", headerIndex: cells[0].headerIndex,
			}, nil
		}
		return nil, s.applyGenericExtraShifts(before, cells)
	}

	// One reduction-bearing cell is applied per pass. This deliberately
	// reclassifies the complete frontier before any shift is allowed.
	for _, cell := range cells {
		if !diagnosticParserCoreActionsContain(cell.actions, core.ActionReduce) {
			continue
		}
		if cell.actions.Len() > 1 {
			return nil, s.applyGenericConflict(before, cell)
		}
		if cell.actions.At(0).Type == core.ActionReduce {
			return nil, s.applyGenericReduction(before, cell)
		}
	}
	for _, cell := range cells {
		if cell.actions.Len() > 1 {
			return nil, s.applyGenericConflict(before, cell)
		}
	}
	shiftCells := make([]diagnosticParserCoreGenericCell, 0, len(cells))
	for _, cell := range cells {
		if cell.actions.At(0).Type == core.ActionShift {
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

func (s *diagnosticParserCoreGenericScheduler) applyGenericAccept(before []DiagnosticParserCoreHeaderReceipt, cell diagnosticParserCoreGenericCell) (err error) {
	headersBefore := append([]diagnosticParserCoreHeader(nil), s.headers...)
	dispatchesBefore, workBefore, epochProgressBefore := s.dispatches, s.work, s.epochProgress
	roundsBefore := len(s.receipt.Rounds)
	defer func() {
		if err == nil {
			return
		}
		s.headers = headersBefore
		s.dispatches, s.work, s.epochProgress = dispatchesBefore, workBefore, epochProgressBefore
		s.receipt.Rounds = s.receipt.Rounds[:roundsBefore]
	}()
	if cell.actions.Len() != 1 || cell.actions.At(0).Type != core.ActionAccept {
		return errors.New("parser-core phase zero: generic accept requires one accept action")
	}
	if s.token.Symbol != 0 || s.token.StartByte != s.token.EndByte || s.token.Missing || s.token.NoLookahead || s.token.ExternalScannerToken {
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
	if err := s.canonicalize(); err != nil {
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
				HeaderIndex: cell.headerIndex, State: cell.receipt.State, ByteOffset: cell.receipt.ByteOffset,
				Ordinal: 0, Action: rootParserCoreAction(cell.actions.At(0)),
			}},
			After: after,
		})
	}
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) completeAcceptance() error {
	if s.token.Symbol != 0 || s.token.StartByte != s.token.EndByte || s.token.Missing || s.token.NoLookahead || s.token.ExternalScannerToken {
		return s.finish(DiagnosticParserCoreAccept, "generic scheduler accept is not authenticated EOF", 0)
	}
	if len(s.headers) != 1 {
		return s.finish(DiagnosticParserCoreAccept, "generic scheduler requires one accepted compact head", 0)
	}
	paths, err := compactDerivationsForAcceptance(s.compact, s.headers[0].head)
	if err != nil {
		return err
	}
	if len(paths) != 1 {
		return s.finish(DiagnosticParserCoreAccept, "generic scheduler requires one exact accepted derivation", 0)
	}
	stats, err := s.compact.Stats(s.headers[0].head)
	if err != nil {
		return err
	}
	path := paths[0]
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
	s.receipt.Acceptance = &DiagnosticParserCoreGenericAcceptance{
		ElectionIndex: s.electionIndex, Token: s.token, Header: header,
		Payloads: payloads, Score: path.Score, BranchOrder: path.BranchOrder,
		HasBranchOrder: path.HasBranchOrder, CoreWork: s.compact.Work(),
		Accepts: s.work.Accepts, Stats: stats, Work: s.work,
	}
	s.publishTotals()
	return nil
}

func compactDerivationsForAcceptance(compact *core.Core, head core.Head) ([]core.Derivation, error) {
	paths, err := compact.Derivations(head)
	if errors.Is(err, core.ErrDerivationEnumerationCap) {
		return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreAccept, detail: "accepted derivation enumeration cap"}
	}
	return paths, err
}

func diagnosticParserCoreActionsContain(actions core.ActionRow, actionType core.ActionType) bool {
	for ordinal := 0; ordinal < actions.Len(); ordinal++ {
		if actions.At(ordinal).Type == actionType {
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
	var pathReceipts []DiagnosticParserCoreHeaderPathReceipt
	if s.fullReceipts() {
		var err error
		pathReceipts, err = diagnosticParserCoreHeaderPathReceipts(s.compact, s.headers)
		if err != nil {
			return err
		}
	}
	kept := make([]diagnosticParserCoreHeader, 0, len(s.headers)-len(indices))
	for index, header := range s.headers {
		if _, drop := paused[index]; !drop {
			kept = append(kept, header)
			continue
		}
		if s.fullReceipts() {
			s.receipt.NoActionDrops = append(s.receipt.NoActionDrops, DiagnosticParserCoreGenericNoActionDrop{
				ElectionIndex: s.electionIndex, Token: s.token, Header: pathReceipts[index],
			})
		}
	}
	if len(kept) == 0 {
		return errors.New("parser-core phase zero: sibling-backed no-action drop removed the complete frontier")
	}
	s.headers = kept
	s.work.NoActionDrops += uint64(len(indices))
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) applyGenericReduction(before []DiagnosticParserCoreHeaderReceipt, cell diagnosticParserCoreGenericCell) (err error) {
	headersBefore := append([]diagnosticParserCoreHeader(nil), s.headers...)
	dispatchesBefore, nextSeqBefore := s.dispatches, s.nextSeq
	workBefore, epochProgressBefore := s.work, s.epochProgress
	roundsBefore := len(s.receipt.Rounds)
	defer func() {
		if err == nil {
			return
		}
		s.headers = headersBefore
		s.dispatches, s.nextSeq = dispatchesBefore, nextSeqBefore
		s.work, s.epochProgress = workBefore, epochProgressBefore
		s.receipt.Rounds = s.receipt.Rounds[:roundsBefore]
	}()
	return s.compact.ApplyAtomic(func() error {
		return s.applyGenericReductionAtomic(before, cell)
	})
}

func (s *diagnosticParserCoreGenericScheduler) applyGenericReductionAtomic(before []DiagnosticParserCoreHeaderReceipt, cell diagnosticParserCoreGenericCell) error {
	if err := s.reserveDispatches(1); err != nil {
		return err
	}
	outputs, err := s.compact.ReduceOutputs(s.headers[cell.headerIndex].head, core.Symbol(s.token.Symbol), 0, core.ForkOrder{})
	if err != nil {
		return err
	}
	replacements := make([]diagnosticParserCoreHeader, 0, len(outputs))
	madeFreshProgress := false
	for _, output := range outputs {
		switch output.Freshness {
		case core.ReductionUnchanged:
			continue
		case core.ReductionNew, core.ReductionUpdated:
		default:
			return errors.New("parser-core phase zero: reduction returned invalid freshness")
		}
		madeFreshProgress = true
		if output.Freshness == core.ReductionUpdated {
			adopted, err := s.adoptUpdatedReductionSibling(cell.headerIndex, output.Head)
			if err != nil {
				return err
			}
			if adopted {
				continue
			}
		}
		replacement := s.headers[cell.headerIndex]
		replacement.head = output.Head
		replacement.paused = false
		if len(replacements) > 0 {
			if s.nextSeq == math.MaxUint64 {
				return errors.New("parser-core phase zero: reduction creation sequence overflow")
			}
			replacement.creationSeq = s.nextSeq
			s.nextSeq++
		}
		replacements = append(replacements, replacement)
	}
	if len(replacements) == 0 {
		// The canonical outputs already exist and have been processed in this
		// election. Keep this version paused until a sibling makes real progress;
		// the ordinary no-action drop then removes it under the same safety rule.
		s.headers[cell.headerIndex].paused = true
		s.work.ReductionPauses++
	} else {
		s.headers = replaceDiagnosticParserCoreHeader(s.headers, cell.headerIndex, replacements)
	}
	if madeFreshProgress {
		s.epochProgress = true
	}
	s.work.Reductions++
	s.work.Dispatches++
	if err := s.canonicalize(); err != nil {
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
				HeaderIndex: cell.headerIndex, State: cell.receipt.State, ByteOffset: cell.receipt.ByteOffset,
				Ordinal: 0, Action: rootParserCoreAction(cell.actions.At(0)),
			}},
			After: after,
		})
	}
	return nil
}

// adoptUpdatedReductionSibling updates an already-active canonical sibling in
// place. The sibling keeps its scheduler slot and creation sequence; a paused
// copy becomes runnable because the canonical boundary materially changed.
func (s *diagnosticParserCoreGenericScheduler) adoptUpdatedReductionSibling(source int, head core.Head) (bool, error) {
	for index := range s.headers {
		if index == source {
			continue
		}
		header := s.headers[index]
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
		return true, nil
	}
	return false, nil
}

func (s *diagnosticParserCoreGenericScheduler) reconcileGenericConflictOutputs(source int, outputs []diagnosticParserCoreHeader) ([]diagnosticParserCoreHeader, int, error) {
	kept := make([]diagnosticParserCoreHeader, 0, len(outputs))
	adopted := 0
	for _, output := range outputs {
		if output.freshness == core.ReductionUpdated {
			ok, err := s.adoptUpdatedReductionSibling(source, output.head)
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

func (s *diagnosticParserCoreGenericScheduler) applyGenericConflict(before []DiagnosticParserCoreHeaderReceipt, cell diagnosticParserCoreGenericCell) (err error) {
	headersBefore := append([]diagnosticParserCoreHeader(nil), s.headers...)
	dispatchesBefore, branchOrderBefore, nextSeqBefore := s.dispatches, s.branchOrder, s.nextSeq
	workBefore, epochProgressBefore := s.work, s.epochProgress
	roundsBefore, conflictsBefore := len(s.receipt.Rounds), len(s.receipt.Conflicts)
	externalShiftsBefore := len(s.receipt.ExternalShifts)
	defer func() {
		if err == nil {
			return
		}
		s.headers = headersBefore
		s.dispatches, s.branchOrder, s.nextSeq = dispatchesBefore, branchOrderBefore, nextSeqBefore
		s.work, s.epochProgress = workBefore, epochProgressBefore
		s.receipt.Rounds = s.receipt.Rounds[:roundsBefore]
		s.receipt.Conflicts = s.receipt.Conflicts[:conflictsBefore]
		s.receipt.ExternalShifts = s.receipt.ExternalShifts[:externalShiftsBefore]
	}()
	return s.compact.ApplyAtomic(func() error {
		return s.applyGenericConflictAtomic(before, cell)
	})
}

func (s *diagnosticParserCoreGenericScheduler) applyGenericConflictAtomic(before []DiagnosticParserCoreHeaderReceipt, cell diagnosticParserCoreGenericCell) (err error) {
	branchOrderBefore, nextSeqBefore := s.branchOrder, s.nextSeq
	if err = s.reserveDispatches(1); err != nil {
		return err
	}
	externalStatsBefore, err := s.genericExternalStats()
	if err != nil {
		return err
	}
	execution, err := executeDiagnosticParserCoreGenericConflictDetailed(
		s.compact, s.headers[cell.headerIndex], cell.headerIndex, s.token, cell.actions,
		s.branchOrder, s.fullReceipts(),
	)
	if err != nil {
		return err
	}
	if s.conflictPostExecutionFault != nil {
		if err := s.conflictPostExecutionFault(); err != nil {
			return err
		}
	}
	primaryAdopted := 0
	execution.primaries, primaryAdopted, err = s.reconcileGenericConflictOutputs(cell.headerIndex, execution.primaries)
	if err != nil {
		return err
	}
	secondaryAdopted := make([]int, len(execution.secondaryArms))
	execution.secondaries = execution.secondaries[:0]
	for index, arm := range execution.secondaryArms {
		execution.secondaryArms[index], secondaryAdopted[index], err = s.reconcileGenericConflictOutputs(cell.headerIndex, arm)
		if err != nil {
			return err
		}
		execution.secondaries = append(execution.secondaries, execution.secondaryArms[index]...)
	}
	trialSeq := nextSeqBefore
	for index := range execution.secondaryArms {
		for output := range execution.secondaryArms[index] {
			if trialSeq == math.MaxUint64 {
				return errors.New("parser-core phase zero: conflict creation sequence overflow")
			}
			execution.secondaryArms[index][output].creationSeq = trialSeq
			trialSeq++
		}
	}
	if len(execution.primaries) != 0 {
		execution.primaries[0].creationSeq = s.headers[cell.headerIndex].creationSeq
		for index := 1; index < len(execution.primaries); index++ {
			if trialSeq == math.MaxUint64 {
				return errors.New("parser-core phase zero: conflict creation sequence overflow")
			}
			execution.primaries[index].creationSeq = trialSeq
			trialSeq++
		}
	}
	execution.secondaries = execution.secondaries[:0]
	for _, arm := range execution.secondaryArms {
		execution.secondaries = append(execution.secondaries, arm...)
	}
	execution.nextSeq = trialSeq
	prefix := s.headers[:cell.headerIndex]
	suffix := s.headers[cell.headerIndex+1:]
	outputCount := len(execution.primaries) + len(execution.secondaries)
	headers := make([]diagnosticParserCoreHeader, 0, outputCount+len(prefix)+len(suffix)+1)
	headers = append(headers, prefix...)
	if len(execution.primaries) != 0 {
		headers = append(headers, execution.primaries[0])
	}
	headers = append(headers, suffix...)
	headers = append(headers, execution.secondaries...)
	if len(execution.primaries) > 1 {
		headers = append(headers, execution.primaries[1:]...)
	}
	if outputCount == 0 {
		paused := s.headers[cell.headerIndex]
		paused.paused = true
		headers = append(append(append([]diagnosticParserCoreHeader(nil), prefix...), paused), suffix...)
	}
	s.headers = headers
	s.branchOrder, s.nextSeq = execution.branchOrder, execution.nextSeq
	adoptedCount := primaryAdopted
	for _, count := range secondaryAdopted {
		adoptedCount += count
	}
	if outputCount != 0 || adoptedCount != 0 {
		s.epochProgress = true
	}
	s.work.Conflicts++
	s.work.ConflictActions += uint64(cell.actions.Len())
	s.work.Forks += uint64(cell.actions.Len() - 1)
	s.work.ConflictHeads += uint64(outputCount)
	s.work.Dispatches++
	if err := s.canonicalize(); err != nil {
		return err
	}
	roundIndex := -1
	if s.fullReceipts() {
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
				Ordinal: index + 1, BranchOrder: execution.round.Actions[index].BranchOrder,
				Outputs: outputs, Paused: len(outputs) == 0 && secondaryAdopted[index] == 0,
				Adopted: secondaryAdopted[index] != 0,
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
			ElectionIndex: s.electionIndex, Token: s.token, HeaderIndex: cell.headerIndex,
			BranchOrderBefore: branchOrderBefore, BranchOrderAfter: s.branchOrder,
			NextCreationSeqBefore: nextSeqBefore, NextCreationSeqAfter: s.nextSeq,
			Round: round, Prefix: prefixReceipts,
			PrimaryPaused: len(primaryReceipts) == 0 && primaryAdopted == 0, PrimaryAdopted: primaryAdopted != 0,
			OriginalSuffix: suffixReceipts,
			SecondaryArms:  secondaryArms, After: after,
		}
		if len(primaryReceipts) != 0 {
			conflict.PrimaryOutput = primaryReceipts[0]
			conflict.AdditionalPrimaryOutputs = primaryReceipts[1:]
		}
		s.receipt.Conflicts = append(s.receipt.Conflicts, conflict)
	}
	return s.recordGenericExternalShift(externalStatsBefore, roundIndex)
}

func (s *diagnosticParserCoreGenericScheduler) applyGenericShifts(before []DiagnosticParserCoreHeaderReceipt, cells []diagnosticParserCoreGenericCell) (err error) {
	headersBefore := append([]diagnosticParserCoreHeader(nil), s.headers...)
	dispatchesBefore, workBefore, epochProgressBefore := s.dispatches, s.work, s.epochProgress
	roundsBefore, externalBefore := len(s.receipt.Rounds), len(s.receipt.ExternalShifts)
	defer func() {
		if err == nil {
			return
		}
		s.headers = headersBefore
		s.dispatches, s.work, s.epochProgress = dispatchesBefore, workBefore, epochProgressBefore
		s.receipt.Rounds = s.receipt.Rounds[:roundsBefore]
		s.receipt.ExternalShifts = s.receipt.ExternalShifts[:externalBefore]
	}()
	if err := s.reserveDispatches(uint64(len(cells))); err != nil {
		return err
	}
	externalStatsBefore, err := s.genericExternalStats()
	if err != nil {
		return err
	}
	if len(cells) == 1 {
		cell := cells[0]
		head, err := s.compact.Shift(s.headers[cell.headerIndex].head, core.Symbol(s.token.Symbol), 0, core.Token{
			Symbol: core.Symbol(s.token.Symbol), StartByte: s.token.StartByte, EndByte: s.token.EndByte, External: s.token.ExternalScannerToken,
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
			Symbol: core.Symbol(s.token.Symbol), StartByte: s.token.StartByte, EndByte: s.token.EndByte, External: s.token.ExternalScannerToken,
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
	s.work.OrdinaryShifts += uint64(len(cells))
	s.work.Dispatches += uint64(len(cells))
	if err := s.canonicalize(); err != nil {
		return err
	}
	roundIndex := -1
	if s.fullReceipts() {
		actions := make([]DiagnosticParserCoreRoundAction, len(cells))
		for index, cell := range cells {
			actions[index] = DiagnosticParserCoreRoundAction{
				HeaderIndex: cell.headerIndex, State: cell.receipt.State, ByteOffset: cell.receipt.ByteOffset,
				Ordinal: 0, Action: rootParserCoreAction(cell.actions.At(0)),
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
	return s.recordGenericExternalShift(externalStatsBefore, roundIndex)
}

func (s *diagnosticParserCoreGenericScheduler) applyGenericExtraShifts(before []DiagnosticParserCoreHeaderReceipt, cells []diagnosticParserCoreGenericCell) (err error) {
	headersBefore := append([]diagnosticParserCoreHeader(nil), s.headers...)
	dispatchesBefore, workBefore, epochProgressBefore := s.dispatches, s.work, s.epochProgress
	roundsBefore, externalShiftsBefore := len(s.receipt.Rounds), len(s.receipt.ExternalShifts)
	defer func() {
		if err == nil {
			return
		}
		s.headers = headersBefore
		s.dispatches, s.work, s.epochProgress = dispatchesBefore, workBefore, epochProgressBefore
		s.receipt.Rounds = s.receipt.Rounds[:roundsBefore]
		s.receipt.ExternalShifts = s.receipt.ExternalShifts[:externalShiftsBefore]
	}()
	return s.compact.ApplyAtomic(func() error {
		if len(cells) == 0 {
			return errors.New("parser-core phase zero: empty extra shift cohort")
		}
		for _, cell := range cells {
			if cell.actions.Len() != 1 || cell.actions.At(0).Type != core.ActionShift || !cell.actions.At(0).Extra {
				return errors.New("parser-core phase zero: extra cohort requires one decoded extra action per head")
			}
		}
		if err := s.reserveDispatches(uint64(len(cells))); err != nil {
			return err
		}
		externalStatsBefore, err := s.genericExternalStats()
		if err != nil {
			return err
		}
		inputs := make([]core.ExtraCohortShiftInput, len(cells))
		for index, cell := range cells {
			inputs[index] = core.ExtraCohortShiftInput{Head: s.headers[cell.headerIndex].head, ActionOrdinal: 0}
		}
		heads, err := s.compact.ShiftExtraCohort(inputs, core.Symbol(s.token.Symbol), core.Token{
			Symbol: core.Symbol(s.token.Symbol), StartByte: s.token.StartByte, EndByte: s.token.EndByte,
			Extra: true, External: s.token.ExternalScannerToken,
		})
		if err != nil {
			return err
		}
		for index, cell := range cells {
			s.headers[cell.headerIndex].head = heads[index]
			s.headers[cell.headerIndex].shifted = true
		}
		if s.extraPostExecutionFault != nil {
			if err := s.extraPostExecutionFault(); err != nil {
				return err
			}
		}
		s.epochProgress = true
		s.work.ExtraShifts += uint64(len(cells))
		if len(cells) > 1 {
			s.work.ExtraCohorts++
		}
		s.work.Dispatches += uint64(len(cells))
		if err := s.canonicalize(); err != nil {
			return err
		}
		roundIndex := -1
		if s.fullReceipts() {
			after, err := diagnosticParserCoreHeaderReceipts(s.compact, s.headers)
			if err != nil {
				return err
			}
			actions := make([]DiagnosticParserCoreRoundAction, len(cells))
			for index, cell := range cells {
				actions[index] = DiagnosticParserCoreRoundAction{
					HeaderIndex: cell.headerIndex, State: cell.receipt.State, ByteOffset: cell.receipt.ByteOffset,
					Ordinal: 0, Action: rootParserCoreAction(cell.actions.At(0)),
				}
			}
			round := DiagnosticParserCoreDispatchRound{
				Index: len(s.receipt.Rounds), Before: before, Actions: actions, After: after,
			}
			roundIndex = round.Index
			s.receipt.Rounds = append(s.receipt.Rounds, round)
		}
		return s.recordGenericExternalShift(externalStatsBefore, roundIndex)
	})
}

func (s *diagnosticParserCoreGenericScheduler) genericExternalStats() (core.Stats, error) {
	if !s.fullReceipts() || !s.token.ExternalScannerToken {
		return core.Stats{}, nil
	}
	if len(s.headers) == 0 {
		return core.Stats{}, errors.New("parser-core phase zero: external shift receipt requires a scheduler head")
	}
	return s.compact.Stats(s.headers[0].head)
}

func (s *diagnosticParserCoreGenericScheduler) recordGenericExternalShift(before core.Stats, roundIndex int) error {
	if !s.fullReceipts() || !s.token.ExternalScannerToken {
		return nil
	}
	if len(s.headers) == 0 {
		return errors.New("parser-core phase zero: external shift receipt requires a scheduler head")
	}
	after, err := s.compact.Stats(s.headers[0].head)
	if err != nil {
		return err
	}
	external := DiagnosticParserCoreGenericExternalShift{
		ElectionIndex: s.electionIndex, Token: s.token,
		ScannerBefore: s.currentElection.ScannerBefore, ScannerAfter: s.currentElection.ScannerAfter,
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
	unsupported := func(boundary DiagnosticParserCoreBoundaryKind, detail string) *diagnosticParserCoreGenericUnsupported {
		return &diagnosticParserCoreGenericUnsupported{boundary: boundary, detail: detail, headerIndex: headerIndex}
	}
	if actions.Len() == 0 {
		return unsupported(DiagnosticParserCoreNoAction, "generic scheduler reached an empty action cell")
	}
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
			if token.EndByte <= token.StartByte {
				return unsupported(DiagnosticParserCoreRoute, "generic scheduler ordinary shift is not positive-width")
			}
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
	case token.NoLookahead:
		return &diagnosticParserCoreGenericUnsupported{
			boundary: DiagnosticParserCoreRoute, detail: "generic scheduler does not support no-lookahead tokens",
		}
	case token.Missing:
		return &diagnosticParserCoreGenericUnsupported{
			boundary: DiagnosticParserCoreRoute, detail: "generic scheduler does not support missing tokens",
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
	headers, err := s.canonicalScratch.canonicalize(s.compact, s.headers)
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

func (s *diagnosticParserCoreGenericScheduler) elect(first bool) error {
	if s.tokens >= s.options.MaxTokens {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreCap, detail: "generic scheduler token cap"}
	}
	states := make([]StateID, len(s.headers))
	for index, header := range s.headers {
		receipt, err := s.headerReceipt(header)
		if err != nil {
			return err
		}
		shiftIdentity := receipt.Shifted || first && !receipt.Shifted
		if !shiftIdentity || receipt.Accepted || receipt.Checkpoint != s.checkpoint.SHA256 {
			return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "generic scheduler election frontier is not closed and checkpoint-continuous"}
		}
		states[index] = receipt.State
	}
	if s.observer.beforeElection != nil {
		if err := s.observer.beforeElection(s); err != nil {
			return err
		}
	}
	s.tokenSource.SetParserState(states[0])
	if len(states) == 1 {
		s.tokenSource.SetGLRStates(nil)
	} else {
		s.tokenSource.SetGLRStates(append([]StateID(nil), states...))
	}
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
		s.headers[index].paused = false
		s.headers[index].checkpoint = after.SHA256
	}
	s.electionIndex++
	s.tokens++
	s.work.Elections++
	s.token = token
	s.checkpoint = after
	s.epochProgress = false
	election := DiagnosticParserCoreElection{
		States: states, Token: token, ScannerBefore: before, ScannerAfter: after,
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
	for _, header := range receipts {
		if !header.Shifted || header.Accepted || header.Checkpoint != s.checkpoint.SHA256 {
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
		out, err := compact.Shift(head, core.Symbol(token.Symbol), ordinal, core.Token{Symbol: core.Symbol(token.Symbol), StartByte: token.StartByte, EndByte: token.EndByte, Extra: action.Extra, External: token.ExternalScannerToken}, fork)
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

func applyParserCoreConflictAction(compact *core.Core, head core.Head, token Token, action core.Action, ordinal int, fork core.ForkOrder) ([]diagnosticParserCoreActionOutput, error) {
	if action.Type != core.ActionReduce {
		heads, err := applyParserCorePrefixAction(compact, head, token, action, ordinal, fork)
		if err != nil {
			return nil, err
		}
		outputs := make([]diagnosticParserCoreActionOutput, len(heads))
		for index, output := range heads {
			outputs[index] = diagnosticParserCoreActionOutput{head: output, freshness: core.ReductionNew}
		}
		return outputs, nil
	}
	outputs, err := compact.ReduceOutputs(head, core.Symbol(token.Symbol), ordinal, fork)
	if err != nil {
		return nil, err
	}
	filtered := make([]diagnosticParserCoreActionOutput, 0, len(outputs))
	for _, output := range outputs {
		switch output.Freshness {
		case core.ReductionUnchanged:
		case core.ReductionNew, core.ReductionUpdated:
			filtered = append(filtered, diagnosticParserCoreActionOutput{head: output.Head, freshness: output.Freshness})
		default:
			return nil, errors.New("parser-core phase zero: reduction returned invalid freshness")
		}
	}
	return filtered, nil
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
