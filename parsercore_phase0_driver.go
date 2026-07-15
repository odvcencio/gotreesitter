//go:build gts_parsercorephase0

package gotreesitter

import (
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"
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
)

type DiagnosticParserCorePrefixOptions struct {
	Recovery       bool
	Retry          bool
	Incremental    bool
	IncludedRanges bool
	MaxDispatches  uint64
	MaxTokens      uint64
	Limits         core.Limits
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
// fork and its frozen oracle-condense continuation. It stops before executing
// the next conflict or at an earlier typed unsupported boundary. It never
// calls the production parser.
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

// executeDiagnosticParserCoreConflict executes one complete conflict cell
// transactionally. Returned headers are ordered primary first, then secondary
// clones in action-ordinal order. The caller inserts pre-existing siblings
// between those groups to preserve production scheduler order.
func executeDiagnosticParserCoreConflict(
	compact *core.Core,
	incoming diagnosticParserCoreHeader,
	headerIndex int,
	token Token,
	actions []core.Action,
	branchOrder uint64,
	nextSeq uint64,
) ([]diagnosticParserCoreHeader, DiagnosticParserCoreDispatchRound, uint64, uint64, error) {
	before, err := diagnosticParserCoreHeaderReceipt(compact, incoming)
	if err != nil {
		return nil, DiagnosticParserCoreDispatchRound{}, branchOrder, nextSeq, err
	}
	if err := validateDiagnosticParserCoreCell(token, actions); err != nil {
		return nil, DiagnosticParserCoreDispatchRound{}, branchOrder, nextSeq, err
	}
	if len(actions) < 2 {
		return nil, DiagnosticParserCoreDispatchRound{}, branchOrder, nextSeq, errors.New("parser-core phase zero: conflict executor requires multiple actions")
	}

	trialOrder, trialSeq := branchOrder, nextSeq
	var primary diagnosticParserCoreHeader
	var secondaries []diagnosticParserCoreHeader
	var receipts []DiagnosticParserCoreRoundAction
	err = compact.ApplyAtomic(func() error {
		for ordinal := 1; ordinal < len(actions); ordinal++ {
			trialOrder++
			heads, applyErr := applyParserCorePrefixAction(compact, incoming.head, token, actions[ordinal], ordinal, core.ForkOrder{Present: true, Value: trialOrder})
			if applyErr != nil {
				return applyErr
			}
			if len(heads) != 1 {
				return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "multi-boundary conflict arm requires frontier-version scheduling"}
			}
			secondaries = append(secondaries, diagnosticParserCoreHeader{
				head: heads[0], creationSeq: trialSeq, shifted: actions[ordinal].Type == core.ActionShift,
				checkpoint: incoming.checkpoint,
			})
			receipts = append(receipts, DiagnosticParserCoreRoundAction{
				HeaderIndex: headerIndex, State: before.State, ByteOffset: before.ByteOffset,
				Ordinal: ordinal, Action: rootParserCoreAction(actions[ordinal]), BranchOrder: trialOrder,
			})
			trialSeq++
		}
		heads, applyErr := applyParserCorePrefixAction(compact, incoming.head, token, actions[0], 0, core.ForkOrder{})
		if applyErr != nil {
			return applyErr
		}
		if len(heads) != 1 {
			return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRoute, detail: "multi-boundary primary reduction requires frontier-version scheduling"}
		}
		primary = incoming
		primary.head = heads[0]
		primary.shifted = actions[0].Type == core.ActionShift
		receipts = append(receipts, DiagnosticParserCoreRoundAction{
			HeaderIndex: headerIndex, State: before.State, ByteOffset: before.ByteOffset,
			Ordinal: 0, Action: rootParserCoreAction(actions[0]),
		})
		return nil
	})
	if err != nil {
		return nil, DiagnosticParserCoreDispatchRound{}, branchOrder, nextSeq, err
	}

	headers := append([]diagnosticParserCoreHeader{primary}, secondaries...)
	after, err := diagnosticParserCoreHeaderReceipts(compact, headers)
	if err != nil {
		return nil, DiagnosticParserCoreDispatchRound{}, branchOrder, nextSeq, err
	}
	return headers, DiagnosticParserCoreDispatchRound{
		Before: []DiagnosticParserCoreHeaderReceipt{before}, Actions: receipts, After: after,
	}, trialOrder, trialSeq, nil
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
