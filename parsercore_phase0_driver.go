//go:build gts_parsercorephase0

package gotreesitter

import (
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"
	"reflect"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

type DiagnosticParserCoreBoundaryKind string

const (
	DiagnosticParserCoreFirstFork DiagnosticParserCoreBoundaryKind = "first_fork"
	DiagnosticParserCoreExtra     DiagnosticParserCoreBoundaryKind = "extra"
	DiagnosticParserCoreNoAction  DiagnosticParserCoreBoundaryKind = "no_action"
	DiagnosticParserCoreRecovery  DiagnosticParserCoreBoundaryKind = "recovery"
	DiagnosticParserCoreAccept    DiagnosticParserCoreBoundaryKind = "accept_without_materialization"
	DiagnosticParserCoreCap       DiagnosticParserCoreBoundaryKind = "cap"
	DiagnosticParserCoreIdentity  DiagnosticParserCoreBoundaryKind = "identity"
	DiagnosticParserCoreRoute     DiagnosticParserCoreBoundaryKind = "unsupported_route"
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

type DiagnosticParserCorePrefixResult struct {
	Boundary          DiagnosticParserCoreBoundaryKind
	Detail            string
	Dispatches        uint64
	Tokens            uint64
	State             StateID
	Lookahead         Token
	ForkActions       []DiagnosticParserCoreForkAction
	Elections         []DiagnosticParserCoreElection
	SourceSHA256      [32]byte
	GrammarBlobSHA256 [32]byte
	Grammar           string
	ExactRootDFA      bool
	Materialized      bool
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
			haveToken = true
		}
		result.Dispatches++
		result.State, result.Lookahead = state, token
		actions, err := compact.Actions(core.StateID(state), core.Symbol(token.Symbol))
		if err != nil {
			return result, err
		}
		if len(actions) == 0 {
			result.Boundary, result.Detail = DiagnosticParserCoreNoAction, "canonical cell has no action"
			return result, &diagnosticParserCoreDecline{boundary: result.Boundary, detail: result.Detail}
		}
		for _, action := range actions {
			if action.Extra || action.ExtraChain {
				result.Boundary, result.Detail = DiagnosticParserCoreExtra, "extra action requires production extra ownership semantics"
				return result, &diagnosticParserCoreDecline{boundary: result.Boundary, detail: result.Detail}
			}
		}
		if err := compact.BeginFrontier(); err != nil {
			return result, err
		}
		if len(actions) > 1 {
			var branchOrder uint64
			for ordinal := 1; ordinal < len(actions); ordinal++ {
				branchOrder++
				if err := applyParserCorePrefixAction(compact, head, token, actions[ordinal], ordinal, core.ForkOrder{Present: true, Value: branchOrder}); err != nil {
					return result, err
				}
				result.ForkActions = append(result.ForkActions, DiagnosticParserCoreForkAction{Ordinal: ordinal, Action: rootParserCoreAction(actions[ordinal]), BranchOrder: branchOrder})
			}
			if err := applyParserCorePrefixAction(compact, head, token, actions[0], 0, core.ForkOrder{}); err != nil {
				return result, err
			}
			result.ForkActions = append(result.ForkActions, DiagnosticParserCoreForkAction{Ordinal: 0, Action: rootParserCoreAction(actions[0])})
			result.Boundary, result.Detail = DiagnosticParserCoreFirstFork, "independently decoded and scheduled first conflict"
			return result, nil
		}
		action := actions[0]
		switch action.Type {
		case core.ActionShift:
			head, err = compact.Shift(head, core.Symbol(token.Symbol), 0, core.Token{Symbol: core.Symbol(token.Symbol), StartByte: token.StartByte, EndByte: token.EndByte}, core.ForkOrder{})
			if err != nil {
				return result, err
			}
			state = StateID(action.State)
			haveToken = false
		case core.ActionReduce:
			frontier, reduceErr := compact.Reduce(head, core.Symbol(token.Symbol), 0, core.ForkOrder{})
			if reduceErr != nil {
				return result, reduceErr
			}
			if len(frontier) != 1 {
				return result, errors.New("parser-core phase zero: clean prefix reduction produced multiple boundaries")
			}
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

func applyParserCorePrefixAction(compact *core.Core, head core.Head, token Token, action core.Action, ordinal int, fork core.ForkOrder) error {
	switch action.Type {
	case core.ActionShift:
		_, err := compact.Shift(head, core.Symbol(token.Symbol), ordinal, core.Token{Symbol: core.Symbol(token.Symbol), StartByte: token.StartByte, EndByte: token.EndByte}, fork)
		return err
	case core.ActionReduce:
		_, err := compact.Reduce(head, core.Symbol(token.Symbol), ordinal, fork)
		return err
	case core.ActionRecover:
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreRecovery, detail: "recover action in first conflict"}
	case core.ActionAccept:
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreAccept, detail: "accept action in first conflict"}
	default:
		return errors.New("parser-core phase zero: unknown conflict action")
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
