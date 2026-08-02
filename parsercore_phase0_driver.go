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
	MaxDispatches           uint64
	MaxTokens               uint64
	Limits                  core.Limits
	// freshSchedulerSession is set only by the reusable fresh-full runner,
	// which resets before every call and never exposes a declined core.
	freshSchedulerSession bool
	// These acceptance options require exact artifact certification. Diagnostic
	// callers retain the conservative false defaults.
	allowEOFAcceptNoActionSiblings  bool
	allowPrimaryAcceptDerivation    bool
	allowConvergedSplitDropArtifact bool
	// allowCompactStrategy2ErrorRegion permits the generic scheduler to
	// attempt native S3 recovery (error-region absorb and condense-resume)
	// at a true no-table-action point instead of declining. Set only from
	// Language.CompactStrategy2ErrorRegionCertified (grammar-blob-keyed, not
	// name-keyed -- design section 7). Recovery must also be true: this
	// option alone does not admit the fresh-full runner's recovery guard.
	allowCompactStrategy2ErrorRegion bool
	noLookaheadRootSymbol            Symbol
	hasNoLookaheadRootSymbol         bool
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
	ReductionPauses        uint64
	NoActionDrops          uint64
	Elections              uint64
	Canonicalizations      uint64
	PeakHeaders            uint64
	Overflow               bool
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
// field (head, checkpoint, altSet, lastPersistedHead, lastPersistedAltSet),
// then cleanPathLineage (2-byte aligned), then every remaining byte-sized
// field. This is layout-only: every construction site across the package
// and its tests uses keyed fields (grep-verified), so declaration order
// changes memory footprint, never behavior. It restores this struct to its
// pre-b4b-v2 64 bytes (unsafe.Sizeof-verified, parsercore_phase0_canonical_scratch_internal_test.go)
// despite carrying two full (event, branch) alternative sets plus three
// bools v1 never had (b4b-width-repair audit, 2026-08): the widened
// AlternativeSet's own inline-capacity reduction (core.go) supplies most of
// the recovered space, and this reorder folds the three new bools into
// padding a naive append-at-the-end declaration order would otherwise pay
// for separately.
type diagnosticParserCoreHeader struct {
	creationSeq uint64
	head        core.Head
	checkpoint  core.CheckpointID
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
	lastPersistedHead       core.Head
	lastPersistedAltSet     core.AlternativeSet
	cleanPathLineage        uint16
	freshness               core.ReductionFreshness
	shifted                 bool
	accepted                bool
	paused                  bool
	convergedReductionSplit bool
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
	// s3Region marks a header carrying an open native strategy-2 recovery
	// region (campaign v7 tranche B3 stage S3: error-region absorb and
	// condense-resume). nil for every header outside recovery, mirroring
	// glrStack.cRec's nil-for-clean-stacks discipline (glr.go) -- the S3
	// zero-cost clean-path gate (design section 8, G2). Never mutated in
	// place: s3AdvanceErrorRegion and s3TryOpenErrorRegion always publish a
	// fresh *diagnosticParserCoreS3Region and reassign this field, so a
	// header snapshot taken by diagnosticParserCoreHeaderRollbackScratch
	// (a plain by-value struct copy) restores a correct, independent region
	// state on rollback without aliasing the live one.
	s3Region *diagnosticParserCoreS3Region
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
			return err
		}
		if !header.convergedReductionSplit {
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
			header.blended != header.lastPersistedBlended
		if err := s.compact.RecordHeadLineageOwned(
			owner,
			header.head,
			header.cleanPathRank,
			header.cleanPathLineage,
			header.altSet,
			header.blended,
			setDirty,
		); err != nil {
			return err
		}
		header.lastPersistedHead = header.head
		header.lastPersistedAltSet = header.altSet
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

// Field order groups winner (8-byte aligned int) and altSet (4-byte
// aligned) up front, then the byte/uint16-sized fields; the sole
// construction site (canonicalizeLinear/canonicalizeMapped) uses keyed
// fields, so this reorder is layout-only (b4b-width-repair audit, 2026-08).
type diagnosticParserCoreCanonicalGroup struct {
	winner                  int
	altSet                  core.AlternativeSet
	cleanPathLineage        uint16
	runnable                bool
	convergedReductionSplit bool
	resurrectionUnproved    bool
	cleanPathRank           core.CleanPathRankSelection
	blended                 bool
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
	// Single-head frontiers never reach canonicalizeLinear/canonicalizeMapped,
	// so the per-header canonical group key they group by carries no
	// semantics here -- only the canonical remap (the compact.Boundary +
	// CanonicalBoundary probe) and the freshness reset do. Skip the key-slice
	// sizing and key-struct build entirely on this path; the double-buffer
	// copy above still runs unchanged, since the aliasing check above and on
	// the next call depends on the returned slice living in s.headerBuffers.
	if len(normalized) == 1 {
		header := normalized[0]
		state, byteOffset, err := compact.Boundary(header.head)
		if err != nil {
			return nil, err
		}
		if canonical, ok := compact.CanonicalBoundary(state, byteOffset, header.shifted, header.checkpoint); ok {
			header.head = canonical
		}
		header.freshness = 0
		normalized[0] = header
		s.nextBuffer = uint8(target ^ 1)
		return normalized, nil
	}
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
	case len(normalized) <= diagnosticParserCoreLinearCanonicalLimit:
		out = s.canonicalizeLinear(compact, normalized)
	default:
		out = s.canonicalizeMapped(compact, normalized)
	}
	s.headerBuffers[target] = out
	s.nextBuffer = uint8(target ^ 1)
	return out, nil
}

func (s *diagnosticParserCoreCanonicalScratch) canonicalizeLinear(compact *core.Core, normalized []diagnosticParserCoreHeader) []diagnosticParserCoreHeader {
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
					resurrectionUnproved:    header.resurrectionUnproved,
					cleanPathRank:           header.cleanPathRank, cleanPathLineage: header.cleanPathLineage,
					altSet: header.altSet, blended: header.blended,
				},
			}
			groupCount++
			continue
		}
		group := &groups[groupIndex].diagnosticParserCoreCanonicalGroup
		group.runnable = group.runnable || !header.paused
		group.convergedReductionSplit = group.convergedReductionSplit || header.convergedReductionSplit
		group.resurrectionUnproved = group.resurrectionUnproved || header.resurrectionUnproved
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
			group.blended = group.blended || header.blended || incomparable
		}
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
			header.resurrectionUnproved = group.resurrectionUnproved
			header.cleanPathRank = group.cleanPathRank
			header.cleanPathLineage = group.cleanPathLineage
			header.altSet = group.altSet
			header.blended = group.blended
			normalized[write] = header
			write++
			break
		}
	}
	return normalized[:write]
}

func (s *diagnosticParserCoreCanonicalScratch) canonicalizeMapped(compact *core.Core, normalized []diagnosticParserCoreHeader) []diagnosticParserCoreHeader {
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
		group.resurrectionUnproved = group.resurrectionUnproved || header.resurrectionUnproved
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
			group.blended = group.blended || header.blended || incomparable
		}
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
		header.resurrectionUnproved = group.resurrectionUnproved
		header.cleanPathRank = group.cleanPathRank
		header.cleanPathLineage = group.cleanPathLineage
		header.altSet = group.altSet
		header.blended = group.blended
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
	nextCleanPathLineage          uint16
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
	condenseCandidates            []core.CondenseCandidate
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

func resetDiagnosticParserCoreGenericScheduler(scheduler *diagnosticParserCoreGenericScheduler) error {
	if scheduler.dispatchScratch.busy || scheduler.conflictScratch.busy {
		return errors.New("parser-core phase zero: seed scheduler scratch is active")
	}
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
	classifiedBoundaries := resetDiagnosticParserCoreRetainedSlice(scheduler.classifiedBoundaries)
	condenseCandidates := resetDiagnosticParserCoreRetainedSlice(scheduler.condenseCandidates)
	electStates := resetDiagnosticParserCoreRetainedSlice(scheduler.electStates)
	electGLRStates := resetDiagnosticParserCoreRetainedSlice(scheduler.electGLRStates)
	acceptedPayloads := resetDiagnosticParserCoreRetainedSlice(scheduler.acceptedPayloads)
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
		reductionOutputs:      reductionOutputs,
		reductionReplacements: reductionReplacements,
		classifiedBoundaries:  classifiedBoundaries,
		condenseCandidates:    condenseCandidates,
		electStates:           electStates,
		electGLRStates:        electGLRStates,
		acceptedPayloads:      acceptedPayloads,
	}
	return nil
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
	headerIndex     int
	boundary        core.ClassifiedBoundary
	selectedOrdinal int
	relexedSymbol   Symbol
	selectedBy      diagnosticParserCoreCellSelection
}

func (cell *diagnosticParserCoreGenericCell) actions() core.ActionRow { return cell.boundary.Actions() }
func (cell *diagnosticParserCoreGenericCell) dispatchToken(shared Token) Token {
	if cell.relexedSymbol != 0 {
		shared.Symbol = cell.relexedSymbol
		shared.ExternalScannerToken = false
		shared.ExternalScannerStartByte = 0
	}
	return shared
}
func (cell *diagnosticParserCoreGenericCell) descriptor() core.ActionRowDescriptor {
	return cell.boundary.Actions().Descriptor()
}
func (cell *diagnosticParserCoreGenericCell) kind() core.ActionRowKind {
	if cell.selectedBy == diagnosticParserCoreCellSelectionConflictPolicy {
		switch cell.actions().At(cell.selectedOrdinal).Type {
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
		return cell.selectedOrdinal
	}
	return 0
}

func (cell *diagnosticParserCoreGenericCell) selectsConflictReduction() bool {
	return cell.selectedBy != diagnosticParserCoreCellSelectionNone &&
		cell.selectedBy != diagnosticParserCoreCellSelectionRepetitionFork &&
		cell.actions().At(cell.selectedActionOrdinal()).Type == core.ActionReduce
}

func diagnosticParserCoreRelexedSymbol(shared, relexed Token) (Symbol, bool) {
	if relexed.Symbol == 0 || relexed.Symbol == shared.Symbol {
		return 0, false
	}
	candidate := shared
	candidate.Symbol = relexed.Symbol
	candidate.ExternalScannerToken = false
	candidate.ExternalScannerStartByte = 0
	if candidate != relexed {
		return 0, false
	}
	return relexed.Symbol, true
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

// relexTokenForState mirrors the production parser's span-exact lexer probe.
// It gives one no-action header the tokenization required by its current state.
func (s *diagnosticParserCoreGenericScheduler) relexTokenForState(state StateID, tok Token) (Token, bool) {
	if s == nil || s.tokenSource == nil || s.tokenSource.lexer == nil {
		return tok, false
	}
	lang := s.tokenSource.language
	source := s.tokenSource.lexer.source
	if lang == nil || len(lang.LexStates) == 0 || int(state) >= len(lang.LexModes) {
		return tok, false
	}
	// A stateless external scanner has no checkpoint identity for this probe.
	// Keep that route unchanged until each header can own its scanner state.
	if lang.ExternalScanner != nil && s.checkpoint.Length == 0 {
		return tok, false
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
	if !ok || relexed.Symbol == 0 || relexed.Symbol == tok.Symbol {
		return tok, false
	}
	if relexed.StartByte != tok.StartByte || relexed.EndByte != tok.EndByte {
		return tok, false
	}
	return relexed, true
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
// diagnosticParserCoreHeader is pointer-free today. reset nevertheless clears
// the retained capacity at the end of the scheduler lifecycle so adding a
// pointer-bearing field later cannot make this scratch retain parse state.
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
func finalizeDiagnosticParserCoreAcceptedRootSpan(root *Node, source []byte, sourceLen uint32, allowErrorRoot bool, tokenCount uint32) error {
	expectedStart := firstNonTriviaByteStart(source)
	clean := allowErrorRoot || (!root.IsError() && !root.HasError())
	if root.startByte == expectedStart && root.endByte < sourceLen && clean {
		extendRootToAcceptedCleanTail(root, source, sourceLen, nil)
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
			if gapStart, gapEnd, gapped := diagnosticParserCoreAcceptedTreeLeafCoverageGap(root, source, expectedStart, sourceLen, tokenCount); gapped {
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

// diagnosticParserCoreAcceptedTreeLeafCoverageGap walks root's already-
// materialized, public tree (the same surface go-tree-sitter callers see) in
// byte order and reports the first non-trivia byte range no genuine terminal
// leaf covers -- the same predicate diagnosticParserCoreReduceChildrenTilingGap
// uses per reduce (diagnosticParserCoreGapIsTolerated), applied once, across
// the whole finalized tree, independent of which reduce's declared span
// claimed which bytes. See finalizeDiagnosticParserCoreAcceptedRootSpan's
// allowErrorRoot branch for why the per-reduce check alone is not sufficient
// here.
//
// A childless NODE only counts as covering its span when it is a genuine
// terminal: symbol < tokenCount (an ordinary lexed token, matching the exact
// bound s3CloseInProgressProductions already uses to enumerate terminals),
// or the built-in ERROR symbol (a leaf absorbed into an open S3 region,
// which is also structurally childless -- html_min_a's innermost
// "(ERROR (ERROR))", for one). A childless NON-terminal (symbol >=
// tokenCount, not ERROR) covers NOTHING: this is exactly the shape the
// adversarial finding exposed -- html "<!--c-->>" reduces "document" over
// zero real children, and treating that hollow reduce as if it were a leaf
// spanning its own (already-stretched) declared range let the accept
// through with the comment and the absorbed '>' entirely unrepresented in
// the public tree.
func diagnosticParserCoreAcceptedTreeLeafCoverageGap(root *Node, source []byte, expectedStart, sourceLen, tokenCount uint32) (gapStart, gapEnd uint32, gapped bool) {
	cur := expectedStart
	var walk func(n *Node) (uint32, uint32, bool)
	walk = func(n *Node) (uint32, uint32, bool) {
		if n == nil {
			return 0, 0, false
		}
		count := n.ChildCount()
		if count == 0 {
			sym := n.Symbol()
			if uint32(sym) >= tokenCount && sym != errorSymbol {
				// A hollow non-terminal: covers nothing, does not advance
				// cur. Any real content it claimed but does not itself
				// contain surfaces as a gap at the next genuine leaf (or at
				// the trailing check below, if it was the last thing in the
				// tree).
				return 0, 0, false
			}
			if n.startByte > cur && !diagnosticParserCoreGapIsTolerated(source[cur:n.startByte]) {
				return cur, n.startByte, true
			}
			if n.endByte > cur {
				cur = n.endByte
			}
			return 0, 0, false
		}
		for i := 0; i < count; i++ {
			if gs, ge, gapped := walk(n.Child(i)); gapped {
				return gs, ge, true
			}
		}
		return 0, 0, false
	}
	if gs, ge, gapped := walk(root); gapped {
		return gs, ge, true
	}
	if cur < sourceLen && !diagnosticParserCoreGapIsTolerated(source[cur:sourceLen]) {
		return cur, sourceLen, true
	}
	return 0, 0, false
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
// O(children) per call and allocation-free: entries are the slice
// materializeVisit already built this call from already-materialized nodes
// (nodesByID lookups done moments earlier); this only reads existing
// uint32 spans and does not walk further into any child's own subtree.
func diagnosticParserCoreReduceChildrenTilingGap(startByte, endByte uint32, entries []stackEntry, source []byte) (gapStart, gapEnd uint32, gapped bool) {
	lastEnd := startByte
	for _, entry := range entries {
		child := stackEntryNode(entry)
		if child == nil {
			continue
		}
		if child.startByte > lastEnd && !diagnosticParserCoreGapIsTolerated(source[lastEnd:child.startByte]) {
			return lastEnd, child.startByte, true
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
// gap is not, in fact, a real one: either ordinary inter-token trivia
// (bytesAreInterTokenTrivia, matching the forest's own reduce-time coverage
// rejection), or a single decoration byte strictly enclosed by trivia on
// both sides (bytesAreSingleByteDecorationTrivia).
func diagnosticParserCoreGapIsTolerated(gap []byte) bool {
	return bytesAreInterTokenTrivia(gap) || bytesAreSingleByteDecorationTrivia(gap)
}

// bytesAreSingleByteDecorationTrivia is the second, narrow trivia exception
// this gate needs, found by directly root-causing a currently-passing
// smoke-corpus collision (doxygen, jsdoc -- javadoc/doxygen-style
// "/** ... * @tag ... */" comment bodies): the continuation-line marker
// "* " that begins every interior comment line is not represented by any
// node -- hidden or public -- in either engine's tree. Verified by direct
// inspection of both compact's raw, pre-filter view.Children (the exact
// input this function receives, gap-for-gap) and production's own raw node
// children: production's root for the doxygen smoke fixture has no child at
// all for "/**" or the repeated "* ", only for the @tag content between
// them. It is legitimate, lexer-level filler that the scanner treats the
// same way it treats ordinary whitespace between tokens, just with a
// literal '*' inside it, so the fixed ASCII-whitespace set
// bytesAreInterTokenTrivia checks does not recognize it, and no
// per-language exception is available to ask (tree-sitter's compiled DFA
// transition tables have no queryable "is this byte skippable here"
// surface at this layer).
//
// The rule requires trivia (bytesAreTrivia) strictly BEFORE AND AFTER the
// one marker byte, never touching either of the gap's own edges. This is
// deliberately the strict form, not "buffered on at least one side": an
// earlier, one-sided version of this rule was built and measured against
// the full 206-language admission scorecard, and it silently re-admitted 5
// of the 8 already-fixed javascript witnesses (js_log_1, js_log_3, js_log_5,
// js_log_6, js_log_7 -- their real dropped-byte gaps also happen to touch
// one edge, e.g. immediately preceding the next real token with only
// leading trivia), which is exactly the false-clean escape this tranche
// exists to close. It was reverted; only the two-sided form ships. This
// fixes doxygen outright (its one gap is interior, buffered on both sides).
// jsdoc has two such gaps: an interior one this rule also fixes, and a
// second one where the comment's closing "*/" (no leading space) puts the
// decoration marker on the gap's own trailing edge -- correctly left
// unrecognized here, since that exact shape is indistinguishable from a
// genuine drop (js_log_3 and js_log_7 have the same trailing-edge shape and
// must stay rejected). jsdoc's remaining root-level gap is instead closed by
// the separate, narrower isDerivationRootReduce exemption in
// materializeDiagnosticParserCoreAcceptedSelection's reduce visitor, not by
// widening this predicate further.
//
// The general, language-independent shape that separates the tolerated
// case from a genuine dropped byte (the html/js defect class this gate
// exists to catch -- witness html_min_a's gap is the single byte "^",
// touching both of its own gap edges at once, directly adjacent to real
// content on both sides with no trivia buffer at all) is: exactly one
// non-trivia byte, with ordinary trivia on both sides of it within the same
// gap. A scheduler bug that truly skips real input skips a whole token or a
// recognizable fragment sitting flush against a neighbor on at least one
// edge; manufacturing an isolated, symmetrically trivia-padded punctuation
// byte is not a plausible shape for that defect class. All 18 fixed
// html/js witnesses were re-verified against this exact rule and none
// qualify for it (admission_route_equality_leaf_tiling_test.go).
func bytesAreSingleByteDecorationTrivia(gap []byte) bool {
	if len(gap) < 3 {
		return false
	}
	marker := -1
	for i := 0; i < len(gap); i++ {
		if bytesAreTrivia(gap[i : i+1]) {
			continue
		}
		if marker != -1 {
			return false
		}
		marker = i
	}
	return marker > 0 && marker < len(gap)-1
}

// materializeDiagnosticParserCoreAcceptedSelection materializes the accepted
// compact derivation into a public tree. When scratch is non-nil the runner's
// reusable buffers back the transient materialization storage, so the warm
// steady state does not re-allocate the public-tree scratch on every parse.
// scratch is reset on return, so it is safe to reuse for the next parse.
func materializeDiagnosticParserCoreAcceptedSelection(compact *core.Core, head core.Head, payloads []core.SubtreeID, parser *Parser, source []byte, scratch *parserCoreRunnerScratch, forceReplayParseStates bool, allowErrorRoot bool) (*Tree, error) {
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
			// catches. This closes the jsdoc residual left after
			// bytesAreSingleByteDecorationTrivia: javadoc/doxygen-style comments
			// that close with "*/" (no leading space) put the decoration marker
			// on the trailing edge of the root reduce's own gap, indistinguishable
			// in isolation from a genuine drop (js_log_3 and js_log_7 have the
			// identical trailing-edge shape and must still be rejected -- verified
			// directly, byte for byte: both keep declining under this exemption,
			// since their gaps are not at the root symbol). All 18 fixed html/js
			// witnesses were re-verified with this exemption active and none sit
			// at their language's root symbol, so none are affected by it.
			isDerivationRootReduce := parser.hasRootSymbol && Symbol(view.Symbol) == parser.rootSymbol
			if gapStart, gapEnd, gapped := diagnosticParserCoreReduceChildrenTilingGap(view.StartByte, view.EndByte, entries, source); !isDerivationRootReduce && gapped {
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
	for _, n := range nodes[1:] {
		if n.startByte < acceptedRootSpanStart {
			acceptedRootSpanStart = n.startByte
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
	if err := finalizeDiagnosticParserCoreAcceptedRootSpan(root, source, sourceLen, allowErrorRoot, parser.language.TokenCount); err != nil {
		return rejectTree(err)
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
	tree.incrementalReuseDisabled = !incrementalReuseProven
	tree.compactMaterialized = true
	tree.setParseRuntime(ParseRuntime{
		StopReason: ParseStopAccepted, SourceLen: sourceLen, ExpectedEOFByte: sourceLen,
		RootEndByte: root.endByte, LastTokenEndByte: sourceLen, LastTokenSymbol: 0, LastTokenWasEOF: true,
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

// stopControlMemoryBudgetReason compares the compact core's own live storage
// accounting against the production engine's soft per-parse byte budget
// (stopControlMemoryBudgetBytes, sourced from parseMemoryBudgetForParser so
// the same GOT_PARSE_MEMORY_BUDGET_MB configuration governs both engines).
// Every input is Core.StorageBytes(): six already-tracked slice lengths times
// a compile-time-constant record size, so this is pure deterministic integer
// arithmetic -- no wall clock, no GC-timing dependence, and (same input, same
// budget) the same trip point on every run, unlike the runtime heap/sys
// signal production's own hard ceiling uses (parser_memory_budget_runtime.go).
func (s *diagnosticParserCoreGenericScheduler) stopControlMemoryBudgetReason() ParseStopReason {
	if s == nil || s.options.stopControlMemoryBudgetBytes <= 0 {
		return ParseStopNone
	}
	if s.compact.StorageBytes() < uint64(s.options.stopControlMemoryBudgetBytes) {
		return ParseStopNone
	}
	return ParseStopMemoryBudget
}

// pollStopControl is the bounded scheduler-boundary poll (spec.campaign.v7
// tranche B8): the memory-budget check above, then -- only when the caller
// armed stop control by binding a Parser (stopControlParser, set only by the
// admission-candidate route) -- the exact production deadline/cancellation
// check (activeParseStopReason). It runs once before the scheduler's first
// election and once per iteration of run's dispatch-pass loop: every loop
// iteration is either one dispatch pass or one re-election round, so this
// granularity bounds both a runaway dispatch grind and a runaway election
// cycle. Nil stopControlParser and a zero budget make this two nil/zero
// comparisons -- the diagnostic and benchmark runners, which never set
// either, pay that and nothing else.
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
	return nil
}

func (s *diagnosticParserCoreGenericScheduler) run() error {
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
		if header.s3Region != nil {
			region := header.s3Region
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
				// Depth-0 resume: the pre-error state now accepts the current
				// token. Publish the ERROR container over the absorbed
				// children and condense it onto the pre-error head (the
				// compact equivalent of cRecoverToState's
				// pushStackNode(fork, goal, errNode, ...)), then fall through
				// to ordinary classification below using the refreshed head.
				newHead, resumeErr := s.compact.ErrorRegionResume(header.head, region.state, region.startByte, region.endByte, region.children)
				if resumeErr != nil {
					return nil, resumeErr
				}
				s.headers[index].head = newHead
				s.headers[index].s3Region = nil
				header = s.headers[index]
			case resumeToken.Symbol == 0:
				// EOF while a region is open: cRecoverEOFAccept's whole-file
				// wrap is out of S3 scope (s3TryOpenErrorRegion's doc
				// comment). Fall through to ordinary classification
				// unchanged; it finds no action against the still-open head
				// and lands back in noActionIndices, where
				// s3TryOpenErrorRegion bails (s3Region already set) and the
				// existing decline applies -- fail-closed, not a guess.
			case deeperResumeExists:
				// A stack entry above depth 0 would accept resumeToken: C's
				// own election could pick that deeper resume instead of
				// continuing to absorb (strategy 1, out of S3 scope). Keeping
				// this region growing here would risk swallowing bytes C's
				// oracle would instead have left outside the ERROR container
				// entirely -- decline rather than guess which one C picks.
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
				s.headers[index].s3Region = &diagnosticParserCoreS3Region{
					state: region.state, startByte: region.startByte, endByte: resumeToken.EndByte, children: grown,
				}
				s.headers[index].shifted = true
				if resumeToken.EndByte != s.token.EndByte {
					// The error-mode relex consumed a different span than
					// the shared election (a wider unlexable run, matching
					// C's error-mode lexer): resync the shared token
					// source's cursor so the next elect() call continues
					// from where this absorb actually left off, not from
					// the shared token's own (now-stale) end.
					s.tokenSource.SeekTokenFrontier(resumeToken.EndByte, resumeToken.EndPoint)
				}
				// Return this pass immediately: a header can only reach
				// s3Region!=nil through s3TryOpenErrorRegion, which requires
				// len(s.headers)==1 (S3 owns no forking), so this absorb is
				// the whole pass's work. Falling through to the per-header
				// loop's tail (as a bare `continue` would, once the sole
				// header's iteration ends) reaches the "len(cells)==0, no
				// runnable head" branch with nothing recorded in cells or
				// noActionIndices for this pass, an unsupported_route decline
				// this stage does not own -- confirmed necessary:
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
		workCountRecordResolvedActionCell(actions.Len())
		if actions.Len() == 0 {
			state := StateID(boundary.State())
			if len(s.headers) > 1 {
				relexed, ok := s.relexTokenForState(state, s.token)
				if ok {
					relexedSymbol, ok = diagnosticParserCoreRelexedSymbol(s.token, relexed)
					if ok {
						cellToken = relexed
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
			headerIndex:   index,
			boundary:      boundary,
			relexedSymbol: relexedSymbol,
		}
		if s.tokenSource != nil {
			if ordinal, ok := diagnosticParserCoreConflictPolicyOrdinal(
				s.tokenSource.language,
				cell.dispatchToken(s.token),
				boundary.State(),
				actions,
			); ok {
				cell.selectedOrdinal = ordinal
				cell.selectedBy = diagnosticParserCoreCellSelectionConflictPolicy
			}
			if cell.selectedBy == diagnosticParserCoreCellSelectionNone &&
				actions.Descriptor().Kind() == core.ActionRowUnsupported {
				if ordinal, ok := diagnosticParserCoreRepetitionFoldOrdinal(s.tokenSource.language, actions); ok {
					cell.selectedOrdinal = ordinal
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
			if unsupported := diagnosticParserCoreGenericUnsupportedCellDescriptor(cell.headerIndex, cell.dispatchToken(s.token), cell.actions(), descriptor); unsupported != nil {
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
			// pausedNoActionHeads == 0 means every no-action head reached this
			// point through a genuinely empty action row (no table action for
			// the elected token), not through the unrelated group-election
			// pause tracked by header.paused above. That exact shape is the
			// error-entry point locked-C production pauses on for recovery
			// (glr.go cPaused: "the stack hit a no-action point"; the
			// real-corpus matrix's 13 recovery-handoff rows trigger here with
			// "the elected token has no table action at end-of-file"). Publish
			// the typed recovery boundary for that shape instead of the
			// generic no-action boundary so census and receipts can tell a
			// recovery handoff apart from an internal election pause. This is
			// a dispatch classification only: both boundaries still decline
			// and fall back to production unchanged (B3 stage S1).
			if pausedNoActionHeads == 0 {
				// B3 stage S3: attempt native strategy-2 recovery for the
				// certified witness class instead of declining outright.
				// Scoped to the sole-header, sole-no-action-head shape only
				// (design section 4's "at most one fork" ceiling starts here
				// at zero forks: S3 owns no election and no forking at all).
				// Any other shape, and any ownership attempt that hits
				// genuine ambiguity, falls through unchanged to the existing
				// decline -- fail-closed, never a guess (design section 4's
				// fail-closed rule).
				if len(s.headers) == 1 && len(noActionIndices) == 1 {
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
		for index := 1; index < len(cells); index++ {
			cell := &cells[index]
			if cell.dispatchToken(s.token) != cells[0].dispatchToken(s.token) {
				return &diagnosticParserCoreGenericUnsupported{
					boundary: DiagnosticParserCoreExtra, detail: "generic scheduler extra cohort requires one tokenization", headerIndex: cell.headerIndex,
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
// shape S5 owns (design section 4's stop rule), so the caller must decline
// rather than absorb the real token that opportunity would have consumed.
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
	if !s.s3ErrorRegionAdmitted() {
		return false, nil
	}
	header := &s.headers[index]
	if header.s3Region != nil {
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
	// EOF at the very first no-action point, nothing absorbed yet:
	// cRecoverEOFAccept's whole-file wrap is out of S3 scope. No committed
	// html_erroneous_end_tag witness needs it (verified against the pinned C
	// oracle: every native witness resumes before EOF).
	if s.token.Symbol == 0 {
		return false, nil
	}
	// C's cHandleError tries missing-token insertion (step 2, "once across
	// the version set, in order") before it ever tries strategy 2 absorb.
	// This stage does not own missing-token insertion (S5) or strategy-1
	// election (S4; design section 4's stop rule: "if any html witness needs
	// strategy 1 or missing insertion, leave it fail-closed"), so it must
	// decline whenever a missing-token insertion opportunity exists here,
	// rather than silently absorbing the real token that opportunity would
	// have consumed instead. Confirmed necessary: html_erroneous_end_tag/
	// html_log_7 (a dangling start_tag whose next real token is a valid "</"
	// it cannot use) needs a MISSING ">" here; absorbing "</" as an ordinary
	// error-region token instead produced a confirmed wrong tree.
	missingOpportunity, missingErr := s.s3MissingTokenOpportunityExists(state)
	if missingErr != nil {
		return false, missingErr
	}
	if missingOpportunity {
		return false, nil
	}
	// REQUIRED 2b (adversarial review): before absorbing the first token
	// into a brand-new region, also rule out a deeper (depth 1..
	// cRecoverMaxSummaryDepth) stack-summary resume -- the same existence
	// probe Hook A runs before every subsequent absorb (dispatchPassActive's
	// s3Region branch). A live C stack would try that deeper resume
	// (strategy 1) before ever falling through to strategy 2's absorb; S3
	// owns only depth-0 resume, so finding one here means this shape is out
	// of scope and must decline instead of guessing which one C's election
	// would pick.
	deeperResumeExists, deeperErr := s.compact.AncestorStateWithActionExists(header.head, core.Symbol(s.token.Symbol), cRecoverMaxSummaryDepth)
	if deeperErr != nil {
		return false, deeperErr
	}
	if deeperResumeExists {
		return false, nil
	}
	tokenExtra, extraErr := s3TokenIsExtraShift(s.compact, s.token.Symbol)
	if extraErr != nil {
		return false, extraErr
	}
	leafID, leafErr := s.compact.ErrorRegionLeaf(core.Symbol(s.token.Symbol), s.token.StartByte, s.token.EndByte, tokenExtra)
	if leafErr != nil {
		return false, leafErr
	}
	header.s3Region = &diagnosticParserCoreS3Region{
		state:     state,
		startByte: s.token.StartByte,
		endByte:   s.token.EndByte,
		children:  []core.SubtreeID{leafID},
	}
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

// dropGenericNoActionHeads removes the paused/no-action heads named by indices.
// indices is produced in ascending, unique header order by the dispatch pass,
// so the surviving frontier is compacted in place with no allocation. The drop
// runs outside any rollback transaction, so mutating the s.headers backing is
// safe.
func (s *diagnosticParserCoreGenericScheduler) dropGenericNoActionHeads(indices []int) error {
	if len(indices) == 0 || len(indices) >= len(s.headers) {
		return errors.New("parser-core phase zero: sibling-backed no-action drop removed the complete frontier")
	}
	// F4 disposition (spec.b4b-alternative-set.v2 section 5): a resurrection
	// descended from a HistoricalBoundaryUnproved dead-node import carries no
	// recorded provenance to prove, so it fails closed independently of the
	// proof below -- waived by the same certified-language artifact escape
	// that waives the proof itself. The detail keeps the "converged-path
	// reduction split" substring every existing fallback-reason assertion
	// keys on (admission_switch_converged_path_test.go,
	// admission_switch_erlang_converged_split_probe_test.go), distinguished
	// by the trailing clause.
	if !s.options.allowConvergedSplitDropArtifact {
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
	convergedCoverageDrops, proved := s.diagnosticParserCoreConvergedCoverageDropsV2(indices)
	if diagnosticParserCoreShadowCensusEnabled() {
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
	s.work.add(&s.work.ConvergedCoverageDrops, convergedCoverageDrops)
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
	nextCleanPathLineageBefore := s.nextCleanPathLineage
	workBefore, epochProgressBefore := s.work, s.epochProgress
	roundsBefore := len(s.receipt.Rounds)
	defer func() {
		s.headerRollbackScratch.finish(&s.headers, err != nil)
		if err == nil {
			return
		}
		s.dispatches, s.nextSeq = dispatchesBefore, nextSeqBefore
		s.nextCleanPathLineage = nextCleanPathLineageBefore
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
	candidates := s.collectCondenseCandidates(cell.headerIndex)
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
	outputs, err := s.compact.ReduceOutputsClassifiedIntoWithLiveCondenseCandidatesOwned(
		owner, candidates, s.reductionOutputs, cell.boundary, ordinal, core.ForkOrder{},
	)
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
	if hasMultiplePopPaths {
		reductionLineage, err = nextDiagnosticParserCoreCleanPathLineage(&s.nextCleanPathLineage)
		if err != nil {
			return err
		}
		if err := s.compact.RecordReductionLineageOwned(owner, outputs, reductionLineage); err != nil {
			return err
		}
	}
	s.reductionOutputs = outputs
	s.reductionReplacements = s.reductionReplacements[:0]
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
		switch output.Freshness {
		case core.ReductionUnchanged:
			if convergedHistory || resurrectionUnproved {
				if _, err := s.adoptUpdatedReductionSibling(
					cell.headerIndex,
					output.Head,
					rank,
					lineage,
					outputSet,
					outputSetBlended,
					convergedHistory,
					resurrectionUnproved,
				); err != nil {
					return err
				}
			}
			continue
		case core.ReductionNew, core.ReductionUpdated:
		default:
			return errors.New("parser-core phase zero: reduction returned invalid freshness")
		}
		madeFreshProgress = true
		if output.Freshness == core.ReductionUpdated {
			adopted, err := s.adoptUpdatedReductionSibling(
				cell.headerIndex,
				output.Head,
				rank,
				lineage,
				outputSet,
				outputSetBlended,
				convergedHistory,
				resurrectionUnproved,
			)
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
				HeaderIndex: cell.headerIndex, State: StateID(cell.boundary.State()), ByteOffset: cell.boundary.ByteOffset(),
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
func (s *diagnosticParserCoreGenericScheduler) reindexCondenseCandidatesOwned(owner core.SchedulerTransactionToken, source int) error {
	return s.compact.ReindexCondenseCandidatesOwned(owner, s.collectCondenseCandidates(source))
}

func (s *diagnosticParserCoreGenericScheduler) collectCondenseCandidates(source int) []core.CondenseCandidate {
	candidates := s.condenseCandidates[:0]
	for index, header := range s.headers {
		if index == source || header.accepted || header.paused {
			continue
		}
		candidates = append(candidates, core.CondenseCandidate{
			Head: header.head, Shifted: header.shifted, Checkpoint: header.checkpoint,
		})
	}
	s.condenseCandidates = candidates
	return candidates
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
	convergedReductionSplit bool,
	resurrectionUnproved bool,
) (bool, error) {
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
		s.headers[index].convergedReductionSplit =
			s.headers[index].convergedReductionSplit || convergedReductionSplit
		s.headers[index].resurrectionUnproved =
			s.headers[index].resurrectionUnproved || resurrectionUnproved
		applyDiagnosticParserCoreCleanPathOutput(&s.headers[index], rank, lineage)
		if set.Len() != 0 {
			// Fold-class union (spec.b4b-alternative-set.v2 section 3.4):
			// index is a genuinely different, independently tracked header
			// from source -- this is a joint-resolution merge, not a single
			// thread's own uniform extension.
			dst := &s.headers[index]
			incomparable := s.compact.AlternativeSetIncomparable(dst.altSet, set)
			s.compact.UnionAlternativeSet(&dst.altSet, set)
			dst.blended = dst.blended || setBlended || incomparable
		}
		return true, nil
	}
	return false, nil
}

func (s *diagnosticParserCoreGenericScheduler) reconcileGenericConflictOutputs(source int, outputs []diagnosticParserCoreHeader) ([]diagnosticParserCoreHeader, int, error) {
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
			ok, err := s.adoptUpdatedReductionSibling(
				source,
				output.head,
				output.cleanPathRank,
				output.cleanPathLineage,
				output.altSet,
				output.blended,
				output.cleanPathLineage != 0,
				false,
			)
			if err != nil {
				return nil, 0, err
			}
			if ok {
				adopted++
				continue
			}
			if output.freshness == core.ReductionUnchanged {
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
	nextCleanPathLineageBefore := s.nextCleanPathLineage
	workBefore, epochProgressBefore := s.work, s.epochProgress
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
	if err = s.reindexCondenseCandidatesOwned(owner, cell.headerIndex); err != nil {
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
		s.compact, owner, s.headers[cell.headerIndex], cell.headerIndex, cell.dispatchToken(s.token), cell.boundary,
		s.branchOrder, &s.nextCleanPathLineage,
		cell.selectedBy == diagnosticParserCoreCellSelectionRepetitionFork,
		s.fullReceipts(), &s.conflictScratch,
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
			ElectionIndex: s.electionIndex, Token: cell.dispatchToken(s.token), HeaderIndex: cell.headerIndex,
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
	for index := range cells {
		cell := &cells[index]
		if cell.selectedActionOrdinal() != 0 || cell.dispatchToken(s.token) != cells[0].dispatchToken(s.token) {
			ordinaryCohort = false
			break
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
		})
		if err != nil {
			return err
		}
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
			ordinal := cell.selectedActionOrdinal()
			action := cell.actions().At(ordinal)
			if action.Type != core.ActionShift || action.Extra {
				return errors.New("parser-core phase zero: ordinary shift selection is not an ordinary shift")
			}
			token := cell.dispatchToken(s.token)
			shifted := core.Token{
				Symbol: core.Symbol(token.Symbol), StartByte: token.StartByte, EndByte: token.EndByte, External: token.ExternalScannerToken,
			}
			head, err := s.compact.ShiftClassifiedWithLiveCondenseCandidatesOwned(
				owner, s.collectCondenseCandidates(cell.headerIndex),
				cell.boundary, ordinal, shifted, core.ForkOrder{},
			)
			if err != nil {
				return err
			}
			s.headers[cell.headerIndex].head = head
			s.headers[cell.headerIndex].shifted = true
			markDiagnosticParserCoreExternalLineage(&s.headers[cell.headerIndex], token)
		}
	}
	s.epochProgress = true
	s.work.OrdinaryShifts += uint64(len(cells))
	s.work.Dispatches += uint64(len(cells))
	if err := s.canonicalize(); err != nil {
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
		for index := range cells {
			cell := &cells[index]
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
		for index := range cells {
			cell := &cells[index]
			s.classifiedBoundaries = append(s.classifiedBoundaries, cell.boundary)
		}
		token := cells[0].dispatchToken(s.token)
		heads, err := s.compact.ShiftExtraClassifiedCohortWithLiveCondenseCandidatesOwned(owner, nil, s.classifiedBoundaries, core.Token{
			Symbol: core.Symbol(token.Symbol), StartByte: token.StartByte, EndByte: token.EndByte,
			Extra: true, External: token.ExternalScannerToken,
		})
		if err != nil {
			return err
		}
		for index := range cells {
			cell := &cells[index]
			s.headers[cell.headerIndex].head = heads[index]
			s.headers[cell.headerIndex].shifted = true
			markDiagnosticParserCoreExternalLineage(&s.headers[cell.headerIndex], token)
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
	if err := s.compact.SetPhaseExternalTokenScannerCheckpoints(beforeID, afterID); err != nil {
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
	decoded.CompactConvergedReductionSplitDropsCertified = true
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
	nextCleanPathLineage *uint16,
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
				cleanPathBlended: setBlended,
			})
		case core.ReductionNew, core.ReductionUpdated:
			dst = append(dst, diagnosticParserCoreActionOutput{
				head: output.Head, freshness: output.Freshness,
				cleanPathRank: output.CleanPathRank, cleanPathLineage: lineage, cleanPathSet: set,
				cleanPathBlended: setBlended,
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
