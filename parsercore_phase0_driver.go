//go:build !gts_no_parsercorephase0

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
	// freshSchedulerSession is set only by the reusable fresh-full runner,
	// which resets before every call and never exposes a declined core.
	freshSchedulerSession bool
	// These acceptance options require exact artifact certification. Diagnostic
	// callers retain the conservative false defaults.
	allowEOFAcceptNoActionSiblings bool
	allowPrimaryAcceptDerivation   bool
	noLookaheadRootSymbol          Symbol
	hasNoLookaheadRootSymbol       bool
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
	Passes                     uint64
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
	RepetitionFolds              uint64
	Reductions                   uint64
	OrdinaryShifts               uint64
	OrdinaryCohorts              uint64
	ExtraShifts                  uint64
	ExtraCohorts                 uint64
	Accepts                      uint64
	ReductionPauses              uint64
	NoActionDrops                uint64
	Elections                    uint64
	Canonicalizations            uint64
	PeakHeaders                  uint64
	Overflow                     bool
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
	acceptanceBacking DiagnosticParserCoreGenericAcceptance
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
		rows[index] = core.NewActionRow(converted)
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

func certifyParserCoreExternalPayloadQuiescence(compact *core.Core, lang *Language) {
	if compact != nil && classifyExternalScannerQuiescence(lang) == scannerQuiescenceProven {
		compact.CertifyExternalPayloadsQuiescent()
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
	certifyParserCoreExternalPayloadQuiescence(compact, lang)
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
		return materializeDiagnosticParserCoreAcceptedSelection(compact, head, scheduler.acceptedPayloads, parser, source, nil, false)
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
	creationSeq             uint64
	head                    core.Head
	checkpoint              core.CheckpointID
	freshness               core.ReductionFreshness
	shifted                 bool
	accepted                bool
	paused                  bool
	convergedReductionSplit bool
}

func diagnosticParserCoreCheckpointDigest(compact *core.Core, id core.CheckpointID) ([32]byte, error) {
	_, digest, ok := compact.CheckpointReceipt(id)
	if !ok {
		return [32]byte{}, errors.New("parser-core phase zero: header references unknown checkpoint identity")
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
	head      core.Head
	freshness core.ReductionFreshness
}

func executeDiagnosticParserCoreGenericConflictDetailed(
	compact *core.Core,
	owner core.SchedulerTransactionToken,
	incoming diagnosticParserCoreHeader,
	headerIndex int,
	token Token,
	classified core.ClassifiedBoundary,
	branchOrder uint64,
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
	if err := validateDiagnosticParserCoreCell(token, actions); err != nil {
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
				action, ordinal, core.ForkOrder{Present: true, Value: trialOrder},
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
			primaryAction, 0, core.ForkOrder{},
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
	head       core.Head
	checkpoint core.CheckpointID
	shifted    bool
	accepted   bool
}

type diagnosticParserCoreCanonicalScratch struct {
	headerBuffers [2][]diagnosticParserCoreHeader
	inlineHeaders [2][diagnosticParserCoreLinearCanonicalLimit]diagnosticParserCoreHeader
	nextBuffer    uint8
	keys          []diagnosticParserCorePhaseHead
	inlineKeys    [diagnosticParserCoreLinearCanonicalLimit]diagnosticParserCorePhaseHead
	groups        map[diagnosticParserCorePhaseHead]diagnosticParserCoreCanonicalGroup
}

type diagnosticParserCoreCanonicalGroup struct {
	winner                  int
	runnable                bool
	convergedReductionSplit bool
}

const diagnosticParserCoreLinearCanonicalLimit = 8

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
		if len(headers) <= len(s.inlineHeaders[target]) {
			normalized = s.inlineHeaders[target][:len(headers)]
		} else {
			normalized = make([]diagnosticParserCoreHeader, len(headers))
		}
	} else {
		normalized = normalized[:len(headers)]
	}
	copy(normalized, headers)
	s.headerBuffers[target] = normalized
	if cap(s.keys) < len(headers) {
		if len(headers) <= len(s.inlineKeys) {
			s.keys = s.inlineKeys[:len(headers)]
		} else {
			s.keys = make([]diagnosticParserCorePhaseHead, len(headers))
		}
	} else {
		s.keys = s.keys[:len(headers)]
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
	}
	var out []diagnosticParserCoreHeader
	switch {
	case len(normalized) == 0:
		out = normalized
	case len(normalized) == 1:
		normalized[0].freshness = 0
		out = normalized
	case len(normalized) <= diagnosticParserCoreLinearCanonicalLimit:
		out = s.canonicalizeLinear(normalized)
	default:
		out = s.canonicalizeMapped(normalized)
	}
	s.headerBuffers[target] = out
	s.nextBuffer = uint8(target ^ 1)
	return out, nil
}

func (s *diagnosticParserCoreCanonicalScratch) canonicalizeLinear(normalized []diagnosticParserCoreHeader) []diagnosticParserCoreHeader {
	type linearGroup struct {
		keyIndex int
		diagnosticParserCoreCanonicalGroup
	}
	var groups [diagnosticParserCoreLinearCanonicalLimit]linearGroup
	groupCount := 0
	for index, header := range normalized {
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
					winner: index, runnable: !header.paused,
					convergedReductionSplit: header.convergedReductionSplit,
				},
			}
			groupCount++
			continue
		}
		group := &groups[groupIndex].diagnosticParserCoreCanonicalGroup
		group.runnable = group.runnable || !header.paused
		group.convergedReductionSplit = group.convergedReductionSplit || header.convergedReductionSplit
		if diagnosticParserCoreCanonicalCandidateWins(normalized[group.winner], header) {
			group.winner = index
		}
	}
	write := 0
	for index, header := range normalized {
		for groupIndex := 0; groupIndex < groupCount; groupIndex++ {
			group := groups[groupIndex].diagnosticParserCoreCanonicalGroup
			if group.winner != index {
				continue
			}
			header.paused = !group.runnable
			header.freshness = 0
			header.convergedReductionSplit = group.convergedReductionSplit
			normalized[write] = header
			write++
			break
		}
	}
	return normalized[:write]
}

func (s *diagnosticParserCoreCanonicalScratch) canonicalizeMapped(normalized []diagnosticParserCoreHeader) []diagnosticParserCoreHeader {
	if s.groups == nil {
		s.groups = make(map[diagnosticParserCorePhaseHead]diagnosticParserCoreCanonicalGroup, len(normalized))
	} else {
		clear(s.groups)
	}
	for index, header := range normalized {
		key := s.keys[index]
		group, duplicate := s.groups[key]
		if !duplicate {
			group.winner = index
		} else if diagnosticParserCoreCanonicalCandidateWins(normalized[group.winner], header) {
			group.winner = index
		}
		group.runnable = group.runnable || !header.paused
		group.convergedReductionSplit = group.convergedReductionSplit || header.convergedReductionSplit
		s.groups[key] = group
	}
	write := 0
	for index, header := range normalized {
		group := s.groups[s.keys[index]]
		if group.winner != index {
			continue
		}
		header.paused = !group.runnable
		header.freshness = 0
		header.convergedReductionSplit = group.convergedReductionSplit
		normalized[write] = header
		write++
	}
	return normalized[:write]
}

func diagnosticParserCoreCanonicalCandidateWins(incumbent, candidate diagnosticParserCoreHeader) bool {
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

type diagnosticParserCoreGenericScheduler struct {
	compact                       *core.Core
	tokenSource                   *dfaTokenSource
	scannerScratch                *[]byte
	headers                       []diagnosticParserCoreHeader
	token                         Token
	checkpoint                    DiagnosticParserCoreScannerCheckpoint
	checkpointID                  core.CheckpointID
	currentElection               DiagnosticParserCoreElection
	electionIndex                 int
	noLookaheadSteps              uint8
	tokens                        uint64
	dispatches                    uint64
	branchOrder                   uint64
	nextSeq                       uint64
	options                       DiagnosticParserCorePrefixOptions
	receiptBacking                DiagnosticParserCoreGenericScheduler
	receipt                       *DiagnosticParserCoreGenericScheduler
	summaryHeaderScratch          []DiagnosticParserCoreHeaderReceipt
	headerRollbackScratch         diagnosticParserCoreHeaderRollbackScratch
	canonicalScratch              diagnosticParserCoreCanonicalScratch
	dispatchScratch               diagnosticParserCoreDispatchScratch
	conflictScratch               diagnosticParserCoreConflictScratch
	reductionOutputs              []core.ReductionOutput
	reductionReplacements         []diagnosticParserCoreHeader
	classifiedBoundaries          []core.ClassifiedBoundary
	electStates                   []StateID
	electGLRStates                []StateID
	work                          DiagnosticParserCoreGenericWork
	epochProgress                 bool
	acceptedHead                  core.Head
	acceptedPayloads              []core.SubtreeID
	conflictPostExecutionFault    func() error
	extraPostExecutionFault       func() error
	freshSessionOwner             *core.SchedulerTransactionToken
	observer                      diagnosticParserCoreSeedObserver
	stoppedAfterElection          bool
	requireEOFPostNoLookaheadRoot bool
	seedHeaders                   [1]diagnosticParserCoreHeader
}

const maxDiagnosticParserCoreNoLookaheadSteps = 64

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
	checkpointID core.CheckpointID,
	checkpoint DiagnosticParserCoreScannerCheckpoint,
	observer diagnosticParserCoreSeedObserver,
	options DiagnosticParserCorePrefixOptions,
) (*diagnosticParserCoreGenericScheduler, error) {
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
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact, tokenSource: tokenSource, scannerScratch: scannerScratch,
		checkpoint: checkpoint, checkpointID: checkpointID,
		electionIndex: -1, nextSeq: 1,
		options: options, observer: observer,
	}
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
	headerIndex             int
	boundary                core.ClassifiedBoundary
	conflictPolicy          bool
	conflictPolicyOrdinal   int
	repetitionFold          bool
	repetitionReduceOrdinal int
}

func (cell diagnosticParserCoreGenericCell) actions() core.ActionRow { return cell.boundary.Actions() }
func (cell diagnosticParserCoreGenericCell) descriptor() core.ActionRowDescriptor {
	return cell.boundary.Actions().Descriptor()
}
func (cell diagnosticParserCoreGenericCell) kind() core.ActionRowKind {
	if cell.conflictPolicy {
		switch cell.actions().At(cell.conflictPolicyOrdinal).Type {
		case core.ActionShift:
			return core.ActionRowShift
		case core.ActionReduce:
			return core.ActionRowReduce
		}
	}
	if cell.repetitionFold {
		return core.ActionRowReduce
	}
	return cell.descriptor().Kind()
}
func (cell diagnosticParserCoreGenericCell) selectedActionOrdinal() int {
	if cell.conflictPolicy {
		return cell.conflictPolicyOrdinal
	}
	if cell.repetitionFold {
		return cell.repetitionReduceOrdinal
	}
	return 0
}

func (cell diagnosticParserCoreGenericCell) selectsConflictReduction() bool {
	return (cell.conflictPolicy || cell.repetitionFold) &&
		cell.actions().At(cell.selectedActionOrdinal()).Type == core.ActionReduce
}

var diagnosticParserCoreRepetitionFoldOptOut = map[string]bool{
	// The compact reduction frontier splits an attribute-bearing HTML tag.
	// TestMarkdownInlineAttributeHTMLTagStaysWhole protects the production shape.
	"markdown_inline": true,
}

// diagnosticParserCoreRepetitionFoldOrdinal mirrors the production parser's
// certified single-reduce repetition fold for a clean compact lineage.
func diagnosticParserCoreRepetitionFoldOrdinal(language *Language, actions core.ActionRow) (int, bool) {
	if actions.Len() < 2 || language == nil || cRepetitionSkipOptOut[language.Name] ||
		diagnosticParserCoreRepetitionFoldOptOut[language.Name] {
		return 0, false
	}
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
	selected, ok := conflictPolicyChoice(language, token, StateID(state), decoded)
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

type diagnosticParserCoreDispatchScratch struct {
	busy            bool
	cells           []diagnosticParserCoreGenericCell
	noActionIndices []int
}

// diagnosticParserCoreHeaderRollbackScratch retains the pre-operation header
// frontier while one scheduler mutation is in flight. Scheduler operations are
// deliberately non-reentrant, so one bounded buffer can serve every accept,
// reduction, conflict, ordinary-shift, and extra-shift transaction in a parse.
//
// diagnosticParserCoreHeader is pointer-free today. reset nevertheless clears
// the retained capacity at the end of the scheduler lifecycle so adding a
// pointer-bearing field later cannot make this scratch retain parse state.
type diagnosticParserCoreHeaderRollbackScratch struct {
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
		capacity := max(len(headers), cap(scratch.headers)*2)
		scratch.headers = make([]diagnosticParserCoreHeader, len(headers), capacity)
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
	if rollback && headers != nil {
		restored := *headers
		if cap(restored) < len(scratch.headers) {
			restored = make([]diagnosticParserCoreHeader, len(scratch.headers))
		} else {
			restored = restored[:len(scratch.headers)]
		}
		copy(restored, scratch.headers)
		*headers = restored
	}
	scratch.headers = scratch.headers[:0]
	scratch.busy = false
}

func (scratch *diagnosticParserCoreHeaderRollbackScratch) reset() {
	if scratch == nil {
		return
	}
	clear(scratch.headers[:cap(scratch.headers)])
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
	head, err := compact.Seed(core.StateID(initialState), 0)
	if err != nil {
		return nil, err
	}
	scheduler, err := newDiagnosticParserCoreGenericScheduler(
		compact, tokenSource, scannerScratch, head, initialCheckpointID, initialCheckpointReceipt, observer, options,
	)
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
	nodesByID       []*Node
	nodes           []*Node
	linkScratch     []*Node
	lineStarts      []uint32
	goCompatFrames  []goCompatSubtreeFrame
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
	s.nodesByID = clearNodeScratch(s.nodesByID)
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

func materializeDiagnosticParserCoreAcceptedTree(compact *core.Core, head core.Head, parser *Parser, source []byte, scratch *parserCoreRunnerScratch, forceReplayParseStates bool) (*Tree, error) {
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
	return materializeDiagnosticParserCoreAcceptedSelection(compact, head, derivations[0].Payloads, parser, source, scratch, forceReplayParseStates)
}

func finalizeDiagnosticParserCoreAcceptedRootSpan(root *Node, source []byte, sourceLen uint32) error {
	expectedStart := firstNonTriviaByteStart(source)
	if root.startByte == expectedStart && root.endByte < sourceLen &&
		!root.IsError() && !root.HasError() {
		extendRootToAcceptedCleanTail(root, source, sourceLen, nil)
	}
	if root.startByte == expectedStart && root.endByte == sourceLen &&
		!root.IsError() && !root.HasError() {
		return nil
	}
	return fmt.Errorf(
		"parser-core phase zero: accepted compact root is incomplete or erroneous: span=%d..%d expected=%d..%d error=%t",
		root.startByte,
		root.endByte,
		expectedStart,
		sourceLen,
		root.HasError(),
	)
}

// materializeDiagnosticParserCoreAcceptedSelection materializes the accepted
// compact derivation into a public tree. When scratch is non-nil the runner's
// reusable buffers back the transient materialization storage, so the warm
// steady state does not re-allocate the public-tree scratch on every parse.
// scratch is reset on return, so it is safe to reuse for the next parse.
func materializeDiagnosticParserCoreAcceptedSelection(compact *core.Core, head core.Head, payloads []core.SubtreeID, parser *Parser, source []byte, scratch *parserCoreRunnerScratch, forceReplayParseStates bool) (*Tree, error) {
	if compact == nil || parser == nil || parser.language == nil || head.Node == 0 || len(payloads) == 0 {
		return nil, errors.New("parser-core phase zero: incomplete accepted-tree selection input")
	}
	if scratch != nil {
		defer scratch.resetTreeBuffers()
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
	if scratch != nil && nodesByIDLen <= uint64(math.MaxInt) {
		scratch.nodesByID = parserCoreNodeSlice(scratch.nodesByID, int(nodesByIDLen))
		nodesByID = scratch.nodesByID
	} else {
		nodesByID = make([]*Node, nodesByIDLen)
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
	incrementalReuseProven := replayStates != nil && classifyExternalScannerQuiescence(parser.language) == scannerQuiescenceProven
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
	materializeVisit := func(materializationScratch *diagnosticParserCoreMaterializationScratch) error {
		visit := func(id core.SubtreeID, view core.MaterializationSubtreeView) error {
			if view.EndByte < view.StartByte || view.EndByte > uint32(len(source)) {
				return errors.New("parser-core phase zero: compact subtree extent is outside source")
			}
			named := parser.isNamedSymbol(Symbol(view.Symbol))
			if view.Terminal {
				node := newLeafNodeInArena(
					arena, Symbol(view.Symbol), named, view.StartByte, view.EndByte,
					points.point(view.StartByte), points.point(view.EndByte),
				)
				node.setExtra(view.Extra)
				node.setExternalScannerToken(view.External)
				// No markFragile here: fragile is a reduce/conflict-arm property
				// (subtreeRecord.fragile is only ever set on reductions), so a
				// terminal record is never fragile. The reduce branches below
				// carry the bit.
				stamp(id, node, true)
				return nil
			}

			entries := materializationScratch.entriesFor(len(view.Children))
			structuralChildren := 0
			for index, childID := range view.Children {
				if uint64(childID) >= uint64(len(nodesByID)) || nodesByID[childID] == nil {
					return errors.New("parser-core phase zero: compact materialization traversal omitted a child")
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
				markFragile(child, view.Fragile)
				stamp(id, child, false)
				return nil
			}
			children, fieldIDs, fieldSources, _ := parser.buildReduceChildrenWithPath(
				entries, 0, len(entries), structuralChildren,
				Symbol(view.Symbol), view.ProductionID, arena,
			)
			if child := parser.collapsibleUnarySelfReduction(action, Token{}, arena, entries, 0, len(entries), children, fieldIDs); child != nil {
				child.productionID = view.ProductionID
				child.dynamicPrecedence += int32(view.DynamicPrecedence)
				markFragile(child, view.Fragile)
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
	tree := parser.buildResultFromNodes(nodes, source, arena, nil, nil, linkScratch)
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
	if err := finalizeDiagnosticParserCoreAcceptedRootSpan(root, source, sourceLen); err != nil {
		return rejectTree(err)
	}
	tree.incrementalReuseDisabled = !incrementalReuseProven
	tree.compactMaterialized = true
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

const diagnosticParserCoreNoTableActionDetail = "generic scheduler has no table action for the elected token"

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

func (s *diagnosticParserCoreGenericScheduler) dispatchPassActive() (*diagnosticParserCoreGenericUnsupported, error) {
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
	for index, header := range s.headers {
		if header.shifted || header.accepted {
			continue
		}
		if header.paused {
			s.dispatchScratch.noActionIndices = append(s.dispatchScratch.noActionIndices, index)
			pausedNoActionHeads++
			continue
		}
		boundary, err := s.compact.ClassifyBoundary(header.head, core.Symbol(s.token.Symbol))
		if err != nil {
			return nil, err
		}
		s.work.ActionLookups++
		actions := boundary.Actions()
		workCountRecordResolvedActionCell(actions.Len())
		if actions.Len() == 0 {
			s.dispatchScratch.noActionIndices = append(s.dispatchScratch.noActionIndices, index)
			continue
		}
		cell := diagnosticParserCoreGenericCell{headerIndex: index, boundary: boundary}
		if s.tokenSource != nil {
			cell.conflictPolicyOrdinal, cell.conflictPolicy = diagnosticParserCoreConflictPolicyOrdinal(
				s.tokenSource.language,
				s.token,
				boundary.State(),
				actions,
			)
			if !cell.conflictPolicy && actions.Descriptor().Kind() == core.ActionRowUnsupported {
				cell.repetitionReduceOrdinal, cell.repetitionFold = diagnosticParserCoreRepetitionFoldOrdinal(s.tokenSource.language, actions)
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
	for index, cell := range cells {
		descriptor := cell.descriptor()
		if !cell.conflictPolicy && !cell.repetitionFold {
			if unsupported := diagnosticParserCoreGenericUnsupportedCellDescriptor(cell.headerIndex, s.token, cell.actions(), descriptor); unsupported != nil {
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
		if diagnosticParserCoreGenericNoActionDropEligible(s.headers, noActionIndices, s.epochProgress) {
			return nil, s.dropGenericNoActionHeads(noActionIndices)
		}
		if len(noActionIndices) != 0 {
			detail := diagnosticParserCoreNoTableActionDetail
			if pausedNoActionHeads == len(noActionIndices) {
				detail = "generic scheduler has only paused heads for the elected token"
			} else if pausedNoActionHeads != 0 {
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
		soleAccept := len(s.headers) == 1 && len(cells) == 1 &&
			len(noActionIndices) == 0 && cell.headerIndex == 0
		certifiedAcceptWithDeadSiblings := s.options.allowEOFAcceptNoActionSiblings &&
			len(s.headers) > 1 && len(cells) == 1 &&
			len(noActionIndices) == len(s.headers)-1
		if !soleAccept && !certifiedAcceptWithDeadSiblings {
			return &diagnosticParserCoreGenericUnsupported{
				boundary: DiagnosticParserCoreAccept, detail: "generic scheduler requires a sole homogeneous accept frontier", headerIndex: cell.headerIndex,
			}, nil
		}
		if err := s.applyGenericAccept(before, cell); err != nil {
			return nil, err
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
		return nil, s.dropGenericNoActionHeads(noActionIndices)
	}
	if extraCells != 0 {
		if extraCells != len(cells) || len(cells) != len(s.headers) || len(noActionIndices) != 0 {
			return &diagnosticParserCoreGenericUnsupported{
				boundary: DiagnosticParserCoreExtra, detail: "generic scheduler requires a homogeneous all-runnable extra cohort", headerIndex: cells[0].headerIndex,
			}, nil
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

func (s *diagnosticParserCoreGenericScheduler) zeroWidthExtraShiftWithoutProgress(cells []diagnosticParserCoreGenericCell) *diagnosticParserCoreGenericUnsupported {
	if s.token.EndByte != s.token.StartByte ||
		s.currentElection.ScannerAfter != s.currentElection.ScannerBefore {
		return nil
	}
	for _, cell := range cells {
		action := cell.actions().At(0)
		target := action.State
		if target == 0 {
			target = cell.boundary.State()
		}
		if target == cell.boundary.State() &&
			s.token.EndByte <= cell.boundary.ByteOffset() {
			return &diagnosticParserCoreGenericUnsupported{
				boundary:    DiagnosticParserCoreRoute,
				detail:      "generic scheduler zero-width extra shift has no scanner or parser-state progress",
				headerIndex: cell.headerIndex,
			}
		}
	}
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) applyGenericAccept(before []DiagnosticParserCoreHeaderReceipt, cell diagnosticParserCoreGenericCell) (err error) {
	if err := s.headerRollbackScratch.begin(s.headers); err != nil {
		return err
	}
	dispatchesBefore, workBefore, epochProgressBefore := s.dispatches, s.work, s.epochProgress
	roundsBefore := len(s.receipt.Rounds)
	defer func() {
		s.headerRollbackScratch.finish(&s.headers, err != nil)
		if err == nil {
			return
		}
		s.dispatches, s.work, s.epochProgress = dispatchesBefore, workBefore, epochProgressBefore
		s.receipt.Rounds = s.receipt.Rounds[:roundsBefore]
	}()
	if cell.actions().Len() != 1 || cell.actions().At(0).Type != core.ActionAccept {
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
				HeaderIndex: cell.headerIndex, State: StateID(cell.boundary.State()), ByteOffset: cell.boundary.ByteOffset(),
				Ordinal: 0, Action: rootParserCoreAction(cell.actions().At(0)),
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
	path, selected := selectCompactAcceptanceDerivation(paths, s.options.allowPrimaryAcceptDerivation)
	if !selected {
		return s.finish(DiagnosticParserCoreAccept, "generic scheduler requires one certified accepted derivation", 0)
	}
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
		HasBranchOrder: path.HasBranchOrder, CoreWork: s.compact.Work(),
		Accepts: s.work.Accepts, Stats: stats, Work: s.work,
	}
	s.receipt.Acceptance = &s.receipt.acceptanceBacking
	s.publishTotals()
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

func compactDerivationsForAcceptance(compact *core.Core, head core.Head) ([]core.Derivation, error) {
	paths, err := compact.Derivations(head)
	if errors.Is(err, core.ErrDerivationEnumerationCap) {
		return nil, &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreAccept, detail: "accepted derivation enumeration cap"}
	}
	return paths, err
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

// dropGenericNoActionHeads removes the paused/no-action heads named by indices.
// indices is produced in ascending, unique header order by the dispatch pass,
// so the surviving frontier is compacted in place with no allocation. The drop
// runs outside any rollback transaction, so mutating the s.headers backing is
// safe.
func (s *diagnosticParserCoreGenericScheduler) dropGenericNoActionHeads(indices []int) error {
	if len(indices) == 0 || len(indices) >= len(s.headers) {
		return errors.New("parser-core phase zero: sibling-backed no-action drop removed the complete frontier")
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
			s.receipt.NoActionDrops = append(s.receipt.NoActionDrops, DiagnosticParserCoreGenericNoActionDrop{
				ElectionIndex: s.electionIndex, Token: s.token, Header: pathReceipts[index],
			})
		}
	}
	var convergedReductionSplitDrops uint64
	for _, index := range indices {
		if index >= 0 && index < len(s.headers) && s.headers[index].convergedReductionSplit {
			convergedReductionSplitDrops++
		}
	}
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
	s.work.add(&s.work.ConvergedReductionSplitDrops, convergedReductionSplitDrops)
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) applyGenericReduction(before []DiagnosticParserCoreHeaderReceipt, cell diagnosticParserCoreGenericCell) (err error) {
	if s.freshSessionOwner != nil {
		return s.applyGenericReductionOwned(*s.freshSessionOwner, before, cell)
	}
	if err := s.headerRollbackScratch.begin(s.headers); err != nil {
		return err
	}
	dispatchesBefore, nextSeqBefore := s.dispatches, s.nextSeq
	workBefore, epochProgressBefore := s.work, s.epochProgress
	roundsBefore := len(s.receipt.Rounds)
	defer func() {
		s.headerRollbackScratch.finish(&s.headers, err != nil)
		if err == nil {
			return
		}
		s.dispatches, s.nextSeq = dispatchesBefore, nextSeqBefore
		s.work, s.epochProgress = workBefore, epochProgressBefore
		s.receipt.Rounds = s.receipt.Rounds[:roundsBefore]
	}()
	return s.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		return s.applyGenericReductionOwned(owner, before, cell)
	})
}

func (s *diagnosticParserCoreGenericScheduler) applyGenericReductionOwned(owner core.SchedulerTransactionToken, before []DiagnosticParserCoreHeaderReceipt, cell diagnosticParserCoreGenericCell) error {
	if err := s.reserveDispatches(1); err != nil {
		return err
	}
	ordinal := cell.selectedActionOrdinal()
	if cell.selectsConflictReduction() {
		s.compact.SetReduceConflictContext(true)
		defer s.compact.SetReduceConflictContext(false)
	}
	if s.token.NoLookahead {
		s.compact.SetReduceNoLookaheadContext(true)
		defer s.compact.SetReduceNoLookaheadContext(false)
	}
	outputs, err := s.compact.ReduceOutputsClassifiedIntoOwned(owner, s.reductionOutputs, cell.boundary, ordinal, core.ForkOrder{})
	if err != nil {
		return err
	}
	convergedReductionSplit := len(outputs) > 1 && outputs[0].MultiplePopPaths
	s.reductionOutputs = outputs
	s.reductionReplacements = s.reductionReplacements[:0]
	replacements := s.reductionReplacements
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
		replacement.shifted = s.token.NoLookahead
		replacement.convergedReductionSplit = replacement.convergedReductionSplit || convergedReductionSplit
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
		s.headers[cell.headerIndex].paused = true
		s.work.ReductionPauses++
	} else if len(replacements) == 1 {
		s.headers[cell.headerIndex] = replacements[0]
	} else {
		s.headers = replaceDiagnosticParserCoreHeader(s.headers, cell.headerIndex, replacements)
	}
	if madeFreshProgress {
		s.epochProgress = true
	}
	if cell.selectsConflictReduction() {
		s.work.add(&s.work.RepetitionFolds, 1)
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
				HeaderIndex: cell.headerIndex, State: StateID(cell.boundary.State()), ByteOffset: cell.boundary.ByteOffset(),
				Ordinal: ordinal, Action: rootParserCoreAction(cell.actions().At(ordinal)),
			}},
			After: after,
		})
	}
	if s.token.NoLookahead &&
		Symbol(cell.actions().At(ordinal).Symbol) == s.options.noLookaheadRootSymbol {
		s.requireEOFPostNoLookaheadRoot = true
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
	kept := outputs[:0]
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
	if s.freshSessionOwner != nil {
		return s.applyGenericConflictOwned(*s.freshSessionOwner, before, cell)
	}
	if err := s.headerRollbackScratch.begin(s.headers); err != nil {
		return err
	}
	dispatchesBefore, branchOrderBefore, nextSeqBefore := s.dispatches, s.branchOrder, s.nextSeq
	workBefore, epochProgressBefore := s.work, s.epochProgress
	roundsBefore, conflictsBefore := len(s.receipt.Rounds), len(s.receipt.Conflicts)
	externalShiftsBefore := len(s.receipt.ExternalShifts)
	defer func() {
		s.headerRollbackScratch.finish(&s.headers, err != nil)
		if err == nil {
			return
		}
		s.dispatches, s.branchOrder, s.nextSeq = dispatchesBefore, branchOrderBefore, nextSeqBefore
		s.work, s.epochProgress = workBefore, epochProgressBefore
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
	if err = s.reserveDispatches(1); err != nil {
		return err
	}
	externalStatsBefore, err := s.genericExternalStats()
	if err != nil {
		return err
	}
	actions := cell.actions()
	if err := s.conflictScratch.begin(actions.Len()); err != nil {
		return err
	}
	defer s.conflictScratch.finish()
	execution, err := executeDiagnosticParserCoreGenericConflictDetailed(
		s.compact, owner, s.headers[cell.headerIndex], cell.headerIndex, s.token, cell.boundary,
		s.branchOrder, s.fullReceipts(), &s.conflictScratch,
	)
	if err != nil {
		return err
	}
	if s.conflictPostExecutionFault != nil {
		if err := s.conflictPostExecutionFault(); err != nil {
			return err
		}
	}
	for ordinal := range execution.armRanges {
		arm := execution.arm(ordinal)
		kept, adopted, reconcileErr := s.reconcileGenericConflictOutputs(cell.headerIndex, arm)
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
		paused.paused = true
		headers = headers[:len(prefix)]
		headers = append(headers, paused)
		headers = append(headers, suffix...)
	}
	s.conflictScratch.headerAssembly = headers
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
	s.work.Dispatches++
	if err := s.canonicalize(); err != nil {
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
			ElectionIndex: s.electionIndex, Token: s.token, HeaderIndex: cell.headerIndex,
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
	return s.recordGenericExternalShift(externalStatsBefore, roundIndex)
}

func (s *diagnosticParserCoreGenericScheduler) applyGenericShifts(before []DiagnosticParserCoreHeaderReceipt, cells []diagnosticParserCoreGenericCell) (err error) {
	if s.freshSessionOwner != nil {
		return s.applyGenericShiftsOwned(*s.freshSessionOwner, before, cells)
	}
	if err := s.headerRollbackScratch.begin(s.headers); err != nil {
		return err
	}
	dispatchesBefore, workBefore, epochProgressBefore := s.dispatches, s.work, s.epochProgress
	roundsBefore, externalBefore := len(s.receipt.Rounds), len(s.receipt.ExternalShifts)
	defer func() {
		s.headerRollbackScratch.finish(&s.headers, err != nil)
		if err == nil {
			return
		}
		s.dispatches, s.work, s.epochProgress = dispatchesBefore, workBefore, epochProgressBefore
		s.receipt.Rounds = s.receipt.Rounds[:roundsBefore]
		s.receipt.ExternalShifts = s.receipt.ExternalShifts[:externalBefore]
	}()
	return s.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		return s.applyGenericShiftsOwned(owner, before, cells)
	})
}

func (s *diagnosticParserCoreGenericScheduler) applyGenericShiftsOwned(owner core.SchedulerTransactionToken, before []DiagnosticParserCoreHeaderReceipt, cells []diagnosticParserCoreGenericCell) error {
	if err := s.reserveDispatches(uint64(len(cells))); err != nil {
		return err
	}
	externalStatsBefore, err := s.genericExternalStats()
	if err != nil {
		return err
	}
	ordinaryCohort := len(cells) > 1
	for _, cell := range cells {
		if cell.selectedActionOrdinal() != 0 {
			ordinaryCohort = false
			break
		}
	}
	if ordinaryCohort {
		s.classifiedBoundaries = s.classifiedBoundaries[:0]
		for _, cell := range cells {
			s.classifiedBoundaries = append(s.classifiedBoundaries, cell.boundary)
		}
		heads, err := s.compact.ShiftOrdinaryClassifiedCohortOwned(owner, s.classifiedBoundaries, core.Token{
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
	} else {
		for _, cell := range cells {
			ordinal := cell.selectedActionOrdinal()
			action := cell.actions().At(ordinal)
			if action.Type != core.ActionShift || action.Extra {
				return errors.New("parser-core phase zero: ordinary shift selection is not an ordinary shift")
			}
			head, err := s.compact.ShiftClassifiedOwned(owner, cell.boundary, ordinal, core.Token{
				Symbol: core.Symbol(s.token.Symbol), StartByte: s.token.StartByte, EndByte: s.token.EndByte, External: s.token.ExternalScannerToken,
			}, core.ForkOrder{})
			if err != nil {
				return err
			}
			s.headers[cell.headerIndex].head = head
			s.headers[cell.headerIndex].shifted = true
		}
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
			ordinal := cell.selectedActionOrdinal()
			actions[index] = DiagnosticParserCoreRoundAction{
				HeaderIndex: cell.headerIndex, State: StateID(cell.boundary.State()), ByteOffset: cell.boundary.ByteOffset(),
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
	return s.recordGenericExternalShift(externalStatsBefore, roundIndex)
}

func (s *diagnosticParserCoreGenericScheduler) applyGenericExtraShifts(before []DiagnosticParserCoreHeaderReceipt, cells []diagnosticParserCoreGenericCell) (err error) {
	if err := s.headerRollbackScratch.begin(s.headers); err != nil {
		return err
	}
	dispatchesBefore, workBefore, epochProgressBefore := s.dispatches, s.work, s.epochProgress
	roundsBefore, externalShiftsBefore := len(s.receipt.Rounds), len(s.receipt.ExternalShifts)
	defer func() {
		s.headerRollbackScratch.finish(&s.headers, err != nil)
		if err == nil {
			return
		}
		s.dispatches, s.work, s.epochProgress = dispatchesBefore, workBefore, epochProgressBefore
		s.receipt.Rounds = s.receipt.Rounds[:roundsBefore]
		s.receipt.ExternalShifts = s.receipt.ExternalShifts[:externalShiftsBefore]
	}()
	return s.compact.ApplySchedulerAtomic(func(owner core.SchedulerTransactionToken) error {
		if len(cells) == 0 {
			return errors.New("parser-core phase zero: empty extra shift cohort")
		}
		for _, cell := range cells {
			if cell.actions().Len() != 1 || cell.actions().At(0).Type != core.ActionShift || !cell.actions().At(0).Extra {
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
		s.classifiedBoundaries = s.classifiedBoundaries[:0]
		for _, cell := range cells {
			s.classifiedBoundaries = append(s.classifiedBoundaries, cell.boundary)
		}
		heads, err := s.compact.ShiftExtraClassifiedCohortOwned(owner, s.classifiedBoundaries, core.Token{
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
					HeaderIndex: cell.headerIndex, State: StateID(cell.boundary.State()), ByteOffset: cell.boundary.ByteOffset(),
					Ordinal: 0, Action: rootParserCoreAction(cell.actions().At(0)),
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
		return headers
	}
	out := make([]diagnosticParserCoreHeader, newLen, max(newLen, 2*cap(headers)))
	copy(out, headers[:index])
	copy(out[index:index+len(replacements)], replacements)
	copy(out[index+len(replacements):], headers[index+1:oldLen])
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
		receipt, err := s.headerReceipt(header)
		if err != nil {
			return err
		}
		shiftIdentity := receipt.Shifted || first && !receipt.Shifted
		if !shiftIdentity || receipt.Accepted || header.checkpoint != s.checkpointID {
			return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "generic scheduler election frontier is not closed and checkpoint-continuous"}
		}
		states = append(states, receipt.State)
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
	beforeID, before, err := diagnosticParserCoreInternCheckpoint(s.compact, beforeBytes)
	if err != nil {
		return err
	}
	if beforeID != s.checkpointID {
		return &diagnosticParserCoreDecline{boundary: DiagnosticParserCoreIdentity, detail: "generic scheduler scanner checkpoint continuity failed"}
	}
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
	if err := s.compact.SetPhaseCheckpoint(afterID); err != nil {
		return err
	}
	for index := range s.headers {
		s.headers[index].shifted = false
		s.headers[index].paused = false
		s.headers[index].checkpoint = afterID
	}
	s.electionIndex++
	s.tokens++
	s.work.Elections++
	s.token = token
	s.checkpoint = after
	s.checkpointID = afterID
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
) ([]diagnosticParserCoreActionOutput, []core.ReductionOutput, error) {
	if action.Type != core.ActionReduce {
		switch action.Type {
		case core.ActionShift:
			head, err := compact.ShiftClassifiedOwned(owner, classified, ordinal, core.Token{Symbol: core.Symbol(token.Symbol), StartByte: token.StartByte, EndByte: token.EndByte, Extra: action.Extra, External: token.ExternalScannerToken}, fork)
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
	outputs, err := compact.ReduceOutputsClassifiedIntoOwned(owner, reductionDst, classified, ordinal, fork)
	if err != nil {
		return nil, outputs, err
	}
	for _, output := range outputs {
		switch output.Freshness {
		case core.ReductionUnchanged:
		case core.ReductionNew, core.ReductionUpdated:
			dst = append(dst, diagnosticParserCoreActionOutput{head: output.Head, freshness: output.Freshness})
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
