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
	DiagnosticParserCoreFirstFork       DiagnosticParserCoreBoundaryKind = "first_fork"
	DiagnosticParserCoreExtra           DiagnosticParserCoreBoundaryKind = "extra"
	DiagnosticParserCoreExtraChain      DiagnosticParserCoreBoundaryKind = "extra_chain"
	DiagnosticParserCoreNoAction        DiagnosticParserCoreBoundaryKind = "no_action"
	DiagnosticParserCoreRecovery        DiagnosticParserCoreBoundaryKind = "recovery"
	DiagnosticParserCoreAccept          DiagnosticParserCoreBoundaryKind = "accept_without_materialization"
	DiagnosticParserCoreCap             DiagnosticParserCoreBoundaryKind = "cap"
	DiagnosticParserCoreIdentity        DiagnosticParserCoreBoundaryKind = "identity"
	DiagnosticParserCoreRoute           DiagnosticParserCoreBoundaryKind = "unsupported_route"
	DiagnosticParserCoreElectionBarrier DiagnosticParserCoreBoundaryKind = "multi_state_re_election"
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
	Boundary             DiagnosticParserCoreBoundaryKind
	Detail               string
	Dispatches           uint64
	Tokens               uint64
	State                StateID
	Lookahead            Token
	ForkActions          []DiagnosticParserCoreForkAction
	ForkBoundaryReceipts []DiagnosticParserCoreForkBoundary
	ForkBoundaries       int
	ForkLogicalPaths     uint64
	ExtraShifts          []DiagnosticParserCoreExtraShift
	ReductionAttempts    []DiagnosticParserCoreReductionAttempt
	SameTokenRounds      []DiagnosticParserCoreDispatchRound
	LastBranchOrder      uint64
	Elections            []DiagnosticParserCoreElection
	SourceSHA256         [32]byte
	GrammarBlobSHA256    [32]byte
	Grammar              string
	ExactRootDFA         bool
	Materialized         bool
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
// from the exact production DFA/scanner election until the first authenticated
// fork or a typed unsupported boundary. It never calls the production parser.
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
			if err := continueDiagnosticParserCoreSameToken(compact, token, headers, branchOrder, nextSeq, options, &result); err != nil {
				if setDiagnosticParserCoreBoundaryError(&result, err) {
					return result, &diagnosticParserCoreDecline{boundary: result.Boundary, detail: result.Detail}
				}
				return result, err
			}
			return result, &diagnosticParserCoreDecline{boundary: result.Boundary, detail: result.Detail}
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
	token Token,
	headers []diagnosticParserCoreHeader,
	branchOrder uint64,
	nextSeq uint64,
	options DiagnosticParserCorePrefixOptions,
	result *DiagnosticParserCorePrefixResult,
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
		if err := validateDiagnosticParserCoreCell(token, actions); err != nil {
			return err
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
		result.SameTokenRounds = append(result.SameTokenRounds, round)
		recordDiagnosticParserCoreAppliedReductions(result, token, round.Actions)
		result.LastBranchOrder = branchOrder
	}
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
