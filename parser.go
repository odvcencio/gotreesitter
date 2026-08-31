package gotreesitter

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Parser reads parse tables from a Language and produces a syntax tree.
// It supports GLR parsing: when a (state, symbol) pair maps to multiple
// actions, the parser forks the stack and explores all alternatives in
// parallel while preserving distinct parse paths. Duplicate stack
// versions are collapsed and ambiguities are resolved at selection time.
//
// Parser is not safe for concurrent use. Use one parser per goroutine, a
// ParserPool, or guard shared parser instances with external synchronization.
type Parser struct {
	language            *Language
	reuseCursor         reuseCursor
	reuseScratch        reuseScratch
	reuseMu             sync.Mutex
	reparseFactory      TokenSourceFactory
	recoveryParser      *Parser
	skipRecoveryReparse bool
	// recoveryInitialOnly suppresses all full-parse retry work for one nested
	// recovery probe. The caller accepts the initial tree or runs the legacy
	// recovery parse.
	recoveryInitialOnly bool
	// forceCleanRetryPass forces a single parseInternal call to behave as a
	// non-retry ("clean") pass even when the caller widened the GLR stack
	// budget via maxStacksOverride. A widened retry would normally also enable
	// the retry-pass error-recovery behavior (single-stack resurrection on
	// all-stacks-dead, and the associated degraded error handling), which turns
	// an otherwise-recoverable parse into a fragmented ERROR root. With this
	// set, the extra budget alone keeps a winning branch alive to the same
	// clean accepted forest a wider built-in budget (GOT_GLR_MAX_STACKS) would
	// have produced, matching tree-sitter C on bash for/while/case scripts.
	forceCleanRetryPass           bool
	retryStructuralTopLevelResync bool
	compatibilityBorrowedArenas   []*nodeArena
	fullArenaHint                 uint32
	pendingFullArenaHint          uint32
	compactFullArenaHint          uint32
	finalChildRefArenaHint        uint32
	incrementalArenaHint          uint32
	fullGSSHint                   uint32
	incrementalGSSHint            uint32
	rootSymbol                    Symbol
	hasRootSymbol                 bool
	collapsedChildOccurrencePairs []collapsedChildSymbolPair
	collapsedChildOccurrenceSet   map[uint32]struct{}
	unaryWrapperFlatteningSet     map[unaryWrapperFlatteningKey]Symbol

	// admissionCandidateRoute is the per-Parser override for the Phase-3
	// dual-route admission switch (see admission_switch.go). The zero value
	// follows the process-wide default.
	admissionCandidateRoute admissionRouteMode
	// admissionCandidateRunner caches the compact candidate route's reusable
	// state across full parses on this Parser. It is typed as any because the
	// concrete runner type only exists under the gts_parsercorephase0 build
	// tag; the default build never reads or writes it.
	admissionCandidateRunner any
	// admissionRouteSuppressed, when greater than zero, forces the production
	// route regardless of the switch. It is raised while a reuse-consuming call
	// or a production-only correctness reparse delegates to Parse, so a delegated
	// fresh parse can never publish a compact tree from a path that must stay on
	// production.
	admissionRouteSuppressed int

	// reduceMultiVersion/reduceActionConflict are transient, non-sticky
	// fragility signals for the reduce(s) about to run. Parser is documented
	// single-goroutine (see the doc comment above), so plain mutable fields
	// are safe here; unlike gssScratch.everForked (which latches permanently
	// once a parse ever forks), these are recomputed every dispatch pass /
	// conflict decision so fragility marking (tree.go markReduceFragility)
	// stays scoped to the reduces that actually happened under ambiguity,
	// mirroring C tree-sitter's live action_count/version_count checks
	// rather than a whole-parse latch.
	//
	// reduceMultiVersion mirrors C's "ts_stack_version_count > 1". The ordinary
	// path sets it once per token-dispatch pass. The packed-version transaction
	// path recomputes it before each same-round retry because that path can add
	// and promote physical versions while it scans one action cell.
	reduceMultiVersion bool
	// reduceActionConflict mirrors C's "action_count > 1": true only while
	// the "if len(actions) > 1" conflict-dispatch block (parseInternal) is
	// applying one of the conflicting actions (deterministic choice,
	// depth-cap fallback, or an explicit fork) and its immediate
	// conflict-reduce-frontier / pending-fork follow-through. Reset to false
	// at the top of every per-stack dispatch iteration.
	reduceActionConflict bool

	// Forest-decline diagnostics: the experimental forest fast path records
	// WHERE and WHY it last declined (fell back to production) so the language
	// burndown can triage dead-ends without re-instrumenting. Set on the parser
	// (not package globals) so concurrent parsers don't race. Cleared at the
	// start of each forest parse.
	forestDeclineByte   uint32
	forestDeclineSym    Symbol
	forestDeclineReason string
	forestDeclineStates []StateID
	// forestCapTieStats is Stage 0's cap-event instrument (see
	// ForestCapTieStats and glr_forest.go's forestCapReplacementIndex):
	// hidden-symbol cap-tie counts for the most recent forest parse. Reset
	// (along with forestCapTieDumpActive) by resetForestCapTieStats at the
	// top of every forest entry point, not only the decline diagnostics'
	// parseForest -- tryForestFastPath's early declines (included ranges,
	// the decline memo, ...) never reach parseForest at all, and without
	// their own reset a reused Parser would report a stale prior parse's
	// counts for a call that did no forest work this time.
	forestCapTieStats ForestCapTieStats
	// forestCapTieDumpActive is GOT_FOREST_CAP_TIE_DUMP's value latched once
	// per parse by resetForestCapTieStats; recordForestCapTie reads this
	// field on every hidden-symbol cap tie instead of calling os.Getenv
	// per-tie (see forestCapTieDumpEnabled's doc comment).
	forestCapTieDumpActive bool
	hasRecoverState        []bool
	hasRecoverSymbol       []bool
	recoverByState         [][]recoverSymbolAction
	hasKeywordState        []bool
	// lookupActionIndexFn caches the bound-method closure for
	// lookupActionIndex. Passing p.lookupActionIndex directly at token-source
	// construction sites allocates a fresh 16-byte closure per parse; the
	// bound method only captures p (tables are re-read at call time), so one
	// closure stays valid for the parser's whole lifetime, including language
	// changes. See lookupActionIndexFunc (parser_tables.go).
	lookupActionIndexFn func(state StateID, sym Symbol) uint16
	// activeParseStopCheckFn caches the bound stop-check method. Compact
	// full parses install this callback in a poller. Reusing one closure avoids
	// one allocation per parse.
	activeParseStopCheckFn              parseStopCheck
	typeScriptPropertyIdentifierSymbol  Symbol
	typeScriptIdentifierSymbol          Symbol
	typeScriptHasPropertyIdentifier     bool
	typeScriptHasIdentifier             bool
	typeScriptContextualPropertyKeyword map[string]Symbol
	// Scheme error-recovery: a datum that fails to lex (e.g. a bare backslash)
	// must be recovered as a `_datum` so the enclosing list keeps its opening
	// delimiter. schemeDatumSymbol is the `_datum` nonterminal used to take the
	// correct GOTO when an error node is pushed in a list that has no datum yet.
	isScheme             bool
	schemeDatumSymbol    Symbol
	schemeHasDatumSymbol bool
	// errorCostCompetition enables the faithful C error-recovery port
	// (parser_recover_c.go) for this grammar; see errorCostCompetitionLanguage.
	errorCostCompetition bool
	// relexProbeLexer is the reusable scratch lexer for
	// relexTokenForStackLexState's per-stack re-lex probe. One instance per
	// parser keeps the probe allocation-free at no-action points, which GLR
	// reaches constantly while pruning branches.
	relexProbeLexer Lexer
	// crecoveryEnteredErrorState is set once per Parse() call the first time
	// cHandleError actually runs (a no-action point was hit for the current
	// lookahead in some parser state). It is reset at the start of
	// parseInternal.
	//
	// IMPORTANT: cHandleError is NOT proof that the input is malformed.
	// LALR table limitations routinely drive well-formed, compiling input
	// into a momentary no-action dead end that cDoAllPotentialReductions
	// (step 1 of cHandleError) resolves losslessly via ordinary reduce
	// closure — this is exactly the same "ordinary GLR disambiguation"
	// mechanism eds's legitimate empty-value grammar production and a
	// measurable fraction (~2% in a real repo-file walk) of syntactically
	// valid Go source both exercise. So this field is only a cheap
	// pre-filter ("recovery machinery ran at all") for
	// resolveCRecoverySwallowedError, never the actual suspicion signal —
	// see crecoveryDroppedErrorForClean, which is scoped to the selected
	// result's own lineage and is what the fallback check actually keys on.
	crecoveryEnteredErrorState bool
	// crecoverySwallowedErrorCheckActive guards resolveCRecoverySwallowedError
	// against recursing into itself when it temporarily disables the C
	// recovery gate and re-invokes Parse for the fallback comparison parse.
	crecoverySwallowedErrorCheckActive bool
	// crecoveryDroppedErrorForClean is set exclusively in buildResultFromGLR
	// (parser_result.go), for the stack actually selected as the parse
	// result, when that stack carries cRecoveryUnvalidatedMarker (see
	// glr.go) and no unflagged sibling reaches the same final position (see
	// hasCleanSiblingAtSamePosition). That marker means the selected
	// lineage itself created a real ERROR node via cRecoverToState — for a
	// single-stack dead end, with a small recovered span — and was never
	// re-validated by another cost competition before reaching ACCEPT. Real
	// tree-sitter C's ts_parser__handle_error/ts_parser__recover never
	// produce a zero-marker result from a version that needed recovery
	// (recover_to_state, skip_token, and recover_eof all wrap something in
	// ERROR or MISSING) — this is the precise, narrow signature of the
	// C-recovery cost model swallowing an error signal that resync-based
	// recovery would have kept.
	//
	// This field is deliberately NOT set from "a drop happened somewhere in
	// the parse": an earlier version of this signal fired for any
	// recovery-owning-vs-clean condense drop anywhere, including on stacks
	// that never influenced the final result, and a later version tagged the
	// surviving side of such a drop regardless of stack count or span — both
	// versions triggered the (expensive, always-discarded) fallback re-parse
	// broadly on valid, compiling Go source in a real repo-file walk (the
	// condense-drop trigger specifically: thousands of times per large file,
	// ordinary LALR-table disambiguation, not error recovery). Scoping the
	// signal to cRecoverToState's single-stack, small-span trigger only, plus
	// the same-position sibling check, eliminated that false-fire rate while
	// keeping the confirmed defect-class fixes (java/php/gomod) working. It
	// also deliberately does NOT fire for ordinary "recovered cleanly, no
	// leftover error" resolutions (e.g. eds/ledger), whose selected lineage
	// never carries the marker.
	crecoveryDroppedErrorForClean bool
	// crecoveryHandleErrorSingleStack records whether the GLR engine had
	// exactly one live stack the moment the current cHandleError call began —
	// i.e. whether C-recovery currently owns the whole parse, as opposed to
	// being one of several unrelated ambiguity forks. Read by
	// cRecoverToState when deciding whether a freshly created ERROR node
	// makes its fork worth tracking via cRecoveryUnvalidatedMarker: a fork
	// born while a highly ambiguous grammar (e.g. kotlin's
	// generic-vs-comparison forks) was exploring several unrelated GLR
	// candidates routinely settles back to a legitimately clean tree without
	// ever being "re-validated", so it must not be treated as suspicious.
	crecoveryHandleErrorSingleStack bool
	// crecoveryReductionCandidateCeilingHits and
	// crecoveryMissingTokenCeilingHits count how many times this parse's
	// cDoAllPotentialReductions / cHandleError missing-token search hit the
	// cRecoverMaxReductionCandidateAttempts / cRecoverMaxMissingTokenTrials
	// Go-side backstop ceilings (parser_recover_c.go). Both stay at zero for
	// every currently-passing parse — see those constants' doc comment for
	// sizing rationale — and exist purely as a "counted reason" diagnostic
	// signal (surfaced via ParseRuntime) for the
	// spore.2026-08-02.walnut-e.memory-exhaustion fix: neither ceiling halts
	// the parse itself, so without a counter there would be no way to observe
	// that either one engaged.
	crecoveryReductionCandidateCeilingHits uint64
	crecoveryMissingTokenCeilingHits       uint64
	// crecoveryReductionCandidateAttemptsPeak and
	// crecoveryMissingTokenTrialAttemptsPeak record the single largest
	// candidateAttempts / missingTokenTrialAttempts value any ONE
	// cDoAllPotentialReductions call / cHandleError missing-token search
	// reached during this parse (not cumulative across calls, unlike the
	// ceiling-hit counters above). Diagnostic-only, parse-wide max, reset per
	// parse: lets a corpus walk report how close real, currently-passing
	// input gets to cRecoverMaxReductionCandidateAttempts /
	// cRecoverMaxMissingTokenTrials, the same way the ceiling constants'
	// sizing rationale is measured against.
	crecoveryReductionCandidateAttemptsPeak uint64
	crecoveryMissingTokenTrialAttemptsPeak  uint64
	// crecoveryCostCompetitionRelevant enables recovery convergence for the
	// active recovery frontier. It starts false for a
	// fresh full parse and flips true on the first event that can make costs
	// nonzero or unequal: a stack pausing (cPaused), cHandleError running, an
	// error/missing node being pushed, or — conservatively — any parse that can
	// REUSE subtrees from an old tree. Reused subtrees may carry error nodes
	// without a new pause in this pass. A clean condense clears the active cost
	// state.
	crecoveryCostCompetitionRelevant bool
	// crecoveryCostCompetitionWalkEnabled enables the expensive recovery-cost
	// walks for the current recovery episode. A clean condense clears this gate.
	crecoveryCostCompetitionWalkEnabled bool
	// Recovery-memo tier and operation fields occupy existing Parser padding.
	// Keep larger recovery-only state in the lazy cold sidecar at the tail.
	cNodeMemoPeakTier          RecoveryNodeMemoTier
	cNodeMemoOperationPeakTier RecoveryNodeMemoTier
	cNodeMemoOperationDepth    uint8
	// fullParseRetryPassesTaken counts retry-ladder passes run during the
	// current top-level parse operation (reset at the public parse entry
	// funnels). retryFullParse stops launching passes once it reaches
	// fullParseRetryMaxTotalPasses — a hard bound that, unlike the ladder's
	// wall-clock deadline, holds even when the caller never configured
	// SetTimeoutMicros.
	fullParseRetryPassesTaken int
	// cRecoverSharedTokenErrorModeLexed records whether the CURRENT shared
	// token was produced by C-equivalent error-mode lexing (the DFA token
	// source lexing with the ERROR-state primary because every live stack is
	// absorbing). When true, cRecoverElectionLookaheadSymbol trusts the shared
	// token's identity instead of re-lexing — the DFA source's error-mode lex
	// includes external-scanner and keyword handling the internal relex lacks.
	// Refreshed by updateParserStateTokenSource before each token acquisition.
	cRecoverSharedTokenErrorModeLexed bool
	// cRecoverCustomResyncActive/-Byte track a custom (non-DFA) token source
	// that has been bypassed by cRecoverInternalErrorModeToken while every
	// live stack absorbed in the C error state: once a normally-parsing stack
	// exists again, the source is resynchronized with SkipToByte to the end
	// of the last internally lexed token.
	cRecoverCustomResyncActive bool
	cRecoverCustomResyncByte   uint32
	// cRecoverCustomSourceEligible caches, per token acquisition, whether the
	// active source qualifies for engine-side error-mode substitution (see
	// cRecoverCustomSourceEligibleFor).
	cRecoverCustomSourceEligible bool
	// cNodeMemoEpoch makes pointer-keyed recovery memo invalidation O(1). Node
	// arenas and transient-parent slabs reuse addresses, so an entry is valid
	// only when its epoch matches this parser-owned generation. The generation
	// remains monotonic across parses and is cleared safely on uint16 wrap.
	cNodeMemoEpoch uint16
	// cNodeMemoCache caches per-subtree error cost and visible node count for
	// the gated recovery, keyed on (node pointer, equivVersion) — the engine
	// analogue of C's SubtreeHeapData.error_cost/visible_descendant_count
	// computed once in ts_subtree_summarize_children. A fixed-capacity
	// pointer-keyed 2-way set-associative cache (see cNodeMemoSlot,
	// parser_recover_c.go) rather than a map: invalidated by cNodeMemoEpoch at
	// parse start and transient-scratch checkpoints, empty (len 0) while the
	// gate is off.
	cNodeMemoCache []cNodeMemoCacheEntry
	// cNodeMemoThrash counts genuine 2-way-set collisions in cNodeMemoSlot
	// (the primary way already holding a live current-epoch entry for
	// another node) since the cache was last (re)sized -- reset to 0
	// whenever the cache is (re)initialized for a parse (see the len==0
	// branch in parseInternal) and whenever it grows. This is THIS PARSE's
	// own observed contention on its own memo cache, so crossing
	// cNodeMemoThrashGrowThreshold is a function of the input/edit being
	// parsed, not of any other parse this Parser instance has ever run.
	//
	// The raw counter value itself is NOT reproducible across processes/
	// hosts (cNodeMemoCacheIndex hashes node pointers, which move under
	// ASLR/allocator layout), so do not assume two runs of the same input
	// hit identical thrash counts. What IS reproducible -- and what
	// growCNodeMemoCache's determinism property (issue #380/#388) actually
	// rests on -- is the GROWTH DECISION: real workloads sit either at 0
	// contention (typical clean parses, see
	// TestCNodeMemoCacheStaysSmallForCleanFullParse) or several orders of
	// magnitude above cNodeMemoThrashGrowThreshold (the pathological #388
	// repro, ~235K-245K), so which side of the threshold a given (source,
	// edit) lands on is stable by a wide margin even though the exact count
	// is not.
	cNodeMemoThrash uint32
	// cPrefixPath is the reusable descent scratch for GSS prefix-aggregate
	// fills (cStackPrefixAgg, parser_recover_c.go). The aggregates themselves
	// live on gssNode (aggGen/aggCost/aggVis), the engine analogue of C
	// StackNode.error_cost / node_count maintained at push, making
	// ts_stack_error_cost-equivalent reads O(1) instead of O(stack depth).
	// Cleared and capacity-bounded at parse end so it cannot retain GSS slabs.
	cPrefixPath     []*gssNode
	forceRawSpanAll bool
	// leafInternByLang enables canonical leaf interning for this language even
	// when the global GOT_PARSE_INTERN_LEAVES_SUBSTITUTE flag is off. Limited to
	// languages whose GLR parses keep hundreds of stacks alive (bash, swift),
	// where the deep stack-equivalence merge dominates and shared-leaf pointer
	// identity short-circuits it (measured: swift 4.0x, bash 1.9x). Net-neutral
	// or slightly negative on fast languages (go +4.9%), so it stays per-language.
	leafInternByLang                   bool
	forceRawSpanTable                  []bool
	spanExtendingInvisibleSymbols      []bool
	nonSpanExtendingInvisibleSymbols   []bool
	aliasPreservedWrapperSymbols       []bool
	included                           []Range
	logger                             ParserLogger
	glrTrace                           bool // verbose GLR stack tracing
	ambiguityProfile                   *AmbiguityProfile
	maxConflictWidth                   int // widest N-way conflict in the parse table
	timeoutMicros                      uint64
	cancellationFlag                   *uint32
	parseBudgetDepth                   int
	parseDeadline                      time.Time
	parseStoppedReason                 ParseStopReason
	parseRuntimeMemoryBudgetBytes      int64
	parseRuntimeMemoryBaselineBytes    uint64
	parseRuntimeMemoryBaselineSys      uint64
	parseRuntimeMemoryPoll             uint64
	parseRuntimeMemoryVolumeAtPoll     uint64
	parseRuntimeMemoryHardCeilingBytes int64
	parseMemoryBudgetDiag              parseMemoryBudgetDiagnostic
	parseMemoryBudgetDiagActive        bool
	// compatMemoryBudgetTripped latches true the moment compat normalization
	// — Go's (normalizeGoReturnedTreeCompatibilityWithCensus / walkGoCompatSubtree's
	// poller) or JS/TS's fused walk (normalizeJavaScriptCompatibility /
	// normalizeTypeScriptTreeCompatibilityWithParser / rewriteJavaScriptTypeScriptStatementKeywordsCallPrecedenceAndBuildUnaryBinaryIndex's
	// poller) — observes a runtime memory-budget trip. The runtime heap/sys
	// budget check is inherently non-monotonic (GC can make heap growth
	// appear to recede), unlike arena.budgetExhausted(), which stays true for
	// the rest of the parse once tripped — so without this sticky flag, a
	// later, throttled resultMaterializationStopReason recheck (finalizeTree,
	// after compat normalization returns) could miss the trip entirely and
	// stamp the tree with ParseStopAccepted despite compat normalization
	// having already bailed out mid-walk. See resultMaterializationStopReason.
	compatMemoryBudgetTripped    bool
	denseLimit                   int
	smallBase                    int
	smallLookup                  [][]smallActionPair
	smallTokenLookup             [][]uint16
	externalValidByState         [][]uint16
	externalValidMaskByState     []uint64
	hasExtraChainActions         bool
	classifiedActions            []classifiedParseAction
	reduceChainHints             []reduceChainHint
	reduceChainHintByState       []int
	reduceAliasSeq               [][]Symbol
	aliasTargetSymbol            []bool
	keepSameNamedAnonChildSymbol []bool
	sharedAnonymousTokenSymbol   []bool
	reduceHasFields              []bool
	reduceFieldPlans             []reduceFieldPlan
	fieldIDScratch               []FieldID
	fieldInheritedScratch        []bool
	fieldConflictedScratch       []bool
	reduceScratch                *reduceBuildScratch
	// mergeScratch points at the active parse's scratch.merge for the duration
	// of parseInternal (set alongside reduceScratch, cleared on return). It lets
	// dispatch-time helpers that only carry *Parser — tryGSSMainMergeForParser
	// and the retargetStackEntryPayload reduce sites — invalidate the
	// materializing-shape prefix cache when they rewrite a spine node's
	// root->head prefix. nil outside a parse; bumpShapePrefixEpoch is nil-safe.
	mergeScratch *glrMergeScratch
	// budgetScratch points at the active parse's *parserScratch for the
	// duration of parseInternal (set alongside mergeScratch/reduceScratch,
	// cleared on return). resultMaterializationStopReason uses it to fold
	// parserScratch's own tracked allocation — dominated in pathological
	// cases by gssScratch.allocatedBytes — into the per-parse memory-budget
	// poll. gssScratch carries no budget state of its own (see gssScratch's
	// doc comment in glr_gss.go); only the owning parserScratch does
	// (parserScratch.budgetExhausted, already exercised by the main GLR loop
	// at several per-token checkpoints in parser.go). Before this field
	// existed, resultMaterializationStopReason — the only poll site
	// cDoAllPotentialReductions/cHandleError's C-recovery candidate search
	// reaches (parser_recover_c.go) — had no way to see that check at all,
	// because scratch is a local variable inside parseInternal, unreachable
	// from a *Parser-only call chain. nil outside a parse; budgetExhausted()
	// is itself nil-safe, but callers check budgetScratch != nil explicitly
	// to match this file's existing arena != nil && arena.budgetExhausted()
	// convention at the same call site (parser_result.go).
	budgetScratch *parserScratch
	// goCompatFrames points at the active parser scratch's reusable result-tree
	// traversal stack. It is nil outside parseInternal.
	goCompatFrames                     *[]goCompatSubtreeFrame
	noTreeBenchmarkOnly                bool
	noTreeCheckpointBenchmarkOnly      bool
	compactNoTreeShiftLeaves           bool
	compactFullShiftLeaves             bool
	eagerDefaultReduces                []eagerDefaultReduceAction
	pendingFullParents                 bool
	finalChildRefs                     bool
	skipInvisibleFullLeafCheckpoints   bool
	transientReduceChildren            bool
	transientReduceScratchNoAlias      bool
	transientChildren                  *transientChildScratch
	noResultCompatibilityBenchmarkOnly bool
	// forceFullResultNormalizationWalk disables incremental range-limited result
	// normalization for this Parser, forcing the full-tree walk. It exists only
	// so the campaign O(edit) byte-sweep differential can compare the
	// range-limited walk against the full walk on one Parser (see
	// SetForceFullResultNormalizationWalk). Production leaves it false.
	forceFullResultNormalizationWalk bool
	// disableLeadingRunSplice is a test-only differential seam for fixtures with
	// known incremental-vs-fresh residuals. Production always leaves it false.
	disableLeadingRunSplice             bool
	currentExternalTokenCheckpoint      externalScannerCheckpoint
	currentExternalTokenCheckpointStart uint32
	currentExternalTokenCheckpointEnd   uint32
	currentExternalTokenCheckpointValid bool
	normalizationStats                  normalizationStats
	materializationTiming               *parseMaterializationTiming
	reduceTiming                        *parseMaterializationTiming
	// forestConflictChoice backs forestSingletonActions: a single-element scratch
	// slice the GSS-forest path reuses when a scoped conflict rule collapses a
	// multi-action set to one C-preferred action, avoiding a per-node allocation.
	forestConflictChoice [1]ParseAction
	// pendingForkStacks buffers extra stacks produced by gated multi-link GSS
	// reductions. The dispatch loop drains them into stacks for same-token
	// re-dispatch.
	pendingForkStacks          []glrStack
	pendingFrontierForkStacks  []glrStack
	disablePostReduceForkMerge bool
	stopActionDiag             *parseStopActionDiagnostic
	// forestDeclineMemo is the lazy parser cold sidecar. It stores the forest
	// memo, recovery state, telemetry, and bounded pending-stack reserves.
	// Parsers that use none of these features pay no sidecar allocation.
	// Explicit ParseForestExperimental calls intentionally ignore this memo.
	forestDeclineMemo *parserColdState
	// cCondenseVersionKeyRanks is parser-owned scratch for the capped recovery
	// version window. Parser is not safe for concurrent use, so this map needs no
	// lock. cCondenseAndResume clears it before each qualifying pass and stores
	// only the first cRecoverMaxVersionCount keys. The bounded map remains
	// allocated after the first qualifying pass, but its key values are cleared
	// at parse and snippet-parser reset boundaries to avoid retaining recovery
	// groups from an earlier parse.
	cCondenseVersionKeyRanks map[cCondenseVersionKey]uint8
}

var snippetParserPools sync.Map

type parseStopActionDiagnostic struct {
	captured                     bool
	phase                        string
	stackState                   StateID
	stackByte                    uint32
	stackDepth                   int
	lookaheadSymbol              Symbol
	lookaheadStartByte           uint32
	lookaheadEndByte             uint32
	lookaheadNoLookahead         bool
	actionType                   ParseActionType
	actionState                  StateID
	actionSymbol                 Symbol
	actionChildCount             uint8
	actionProductionID           uint16
	actionDynamicPrecedence      int16
	actionCount                  int
	resultState                  StateID
	inReduceChain                bool
	reduceChainStep              int
	repeatedReduceSignatureCount int
	reduceChainCycle             bool
	forceAdvanceAfterReduce      bool
	postDispatchDefaultReduced   bool
	anyReduced                   bool
	consumedToken                bool
	dispatchShiftActions         int
	dispatchReduceActions        int
	dispatchAcceptActions        int
	dispatchRecoverActions       int
	dispatchOtherActions         int
	lastReduceCaptured           bool
	lastReducePhase              string
	lastReduceStackState         StateID
	lastReduceStackByte          uint32
	lastReduceStackDepth         int
	lastReduceSymbol             Symbol
	lastReduceChildCount         uint8
	lastReduceProductionID       uint16
	lastReduceDynamicPrecedence  int16
	lastReduceResultState        StateID
	lastReduceInChain            bool
	lastReduceChainStep          int
	lastReduceRepeatedSigCount   int
	lastReduceChainCycle         bool
}

type postDispatchStackStatus struct {
	liveUnaccepted            int
	shiftedLiveUnaccepted     int
	unshiftedLiveUnaccepted   int
	unshiftedPaused           int
	unshiftedActionable       int
	unshiftedActionEntries    int
	unshiftedShiftActions     int
	unshiftedReduceActions    int
	unshiftedAcceptActions    int
	unshiftedRecoverActions   int
	unshiftedOtherActions     int
	snapshotActionable        int
	snapshotActionEntries     int
	snapshotShiftActions      int
	snapshotReduceActions     int
	snapshotAcceptActions     int
	snapshotRecoverActions    int
	snapshotOtherActions      int
	appendedActionable        int
	appendedActionEntries     int
	appendedShiftActions      int
	appendedReduceActions     int
	appendedAcceptActions     int
	appendedRecoverActions    int
	appendedOtherActions      int
	firstUnshiftedActionable  bool
	firstUnshiftedActionState StateID
	firstUnshiftedActionDepth int
	firstUnshiftedActionByte  uint32
	accepted                  int
	dead                      int
}

type conflictFrontierResult struct {
	ran               bool
	reason            string
	step              int
	beforeState       StateID
	beforeByte        uint32
	beforeDepth       int
	afterState        StateID
	afterByte         uint32
	afterDepth        int
	afterShifted      bool
	afterDead         bool
	afterAccepted     bool
	lastReduceState   StateID
	lastReduceByte    uint32
	lastReduceDepth   int
	lastReduceSeen    bool
	terminalActionTyp ParseActionType
	terminalActionSet bool
	terminalCount     int
}

type dispatchStackSurvivorTrace struct {
	inSnapshot          bool
	visited             bool
	origin              string
	sourceIndex         int
	path                string
	actionIndex         int
	actionCount         int
	actionType          ParseActionType
	actionState         StateID
	actionSymbol        Symbol
	actionChildCount    uint8
	beforeState         StateID
	beforeByte          uint32
	beforeDepth         int
	afterPrimaryState   StateID
	afterPrimaryByte    uint32
	afterPrimaryDepth   int
	afterPrimaryShifted bool
	afterPrimaryDead    bool
	afterPrimaryAccept  bool
	afterFinalState     StateID
	afterFinalByte      uint32
	afterFinalDepth     int
	afterFinalShifted   bool
	afterFinalDead      bool
	afterFinalAccepted  bool
	frontier            conflictFrontierResult
}

func (s postDispatchStackStatus) progressString() string {
	result := fmt.Sprintf(
		"post_dispatch_live_unaccepted=%d post_dispatch_shifted_live=%d post_dispatch_unshifted_live=%d post_dispatch_unshifted_paused=%d post_dispatch_unshifted_actionable=%d post_dispatch_unshifted_action_entries=%d post_dispatch_unshifted_shift_actions=%d post_dispatch_unshifted_reduce_actions=%d post_dispatch_unshifted_accept_actions=%d post_dispatch_unshifted_recover_actions=%d post_dispatch_unshifted_other_actions=%d post_dispatch_accepted=%d post_dispatch_dead=%d",
		s.liveUnaccepted,
		s.shiftedLiveUnaccepted,
		s.unshiftedLiveUnaccepted,
		s.unshiftedPaused,
		s.unshiftedActionable,
		s.unshiftedActionEntries,
		s.unshiftedShiftActions,
		s.unshiftedReduceActions,
		s.unshiftedAcceptActions,
		s.unshiftedRecoverActions,
		s.unshiftedOtherActions,
		s.accepted,
		s.dead,
	)
	result += fmt.Sprintf(
		" post_dispatch_snapshot_actionable=%d post_dispatch_snapshot_action_entries=%d post_dispatch_snapshot_shift_actions=%d post_dispatch_snapshot_reduce_actions=%d post_dispatch_snapshot_accept_actions=%d post_dispatch_snapshot_recover_actions=%d post_dispatch_snapshot_other_actions=%d post_dispatch_appended_actionable=%d post_dispatch_appended_action_entries=%d post_dispatch_appended_shift_actions=%d post_dispatch_appended_reduce_actions=%d post_dispatch_appended_accept_actions=%d post_dispatch_appended_recover_actions=%d post_dispatch_appended_other_actions=%d",
		s.snapshotActionable,
		s.snapshotActionEntries,
		s.snapshotShiftActions,
		s.snapshotReduceActions,
		s.snapshotAcceptActions,
		s.snapshotRecoverActions,
		s.snapshotOtherActions,
		s.appendedActionable,
		s.appendedActionEntries,
		s.appendedShiftActions,
		s.appendedReduceActions,
		s.appendedAcceptActions,
		s.appendedRecoverActions,
		s.appendedOtherActions,
	)
	if s.firstUnshiftedActionable {
		result += fmt.Sprintf(
			" post_dispatch_first_unshifted_state=%d post_dispatch_first_unshifted_depth=%d post_dispatch_first_unshifted_byte=%d",
			s.firstUnshiftedActionState,
			s.firstUnshiftedActionDepth,
			s.firstUnshiftedActionByte,
		)
	}
	return result
}

func (p *Parser) classifyPostDispatchStacks(stacks []glrStack, tok Token, snapshotSize int) postDispatchStackStatus {
	var status postDispatchStackStatus
	if p == nil || p.language == nil {
		return status
	}
	parseActions := p.language.ParseActions
	for i := range stacks {
		s := &stacks[i]
		if s.dead {
			status.dead++
			continue
		}
		if s.accepted {
			status.accepted++
			continue
		}
		status.liveUnaccepted++
		if s.shifted {
			status.shiftedLiveUnaccepted++
			continue
		}
		status.unshiftedLiveUnaccepted++
		if s.cPaused {
			status.unshiftedPaused++
			continue
		}
		if s.depth() == 0 {
			continue
		}
		actionIdx := p.lookupActionIndex(s.top().state, tok.Symbol)
		if actionIdx == 0 || int(actionIdx) >= len(parseActions) {
			continue
		}
		actions := parseActions[actionIdx].Actions
		if len(actions) == 0 {
			continue
		}
		status.unshiftedActionable++
		status.unshiftedActionEntries += len(actions)
		inSnapshot := snapshotSize < 0 || i < snapshotSize
		if inSnapshot {
			status.snapshotActionable++
			status.snapshotActionEntries += len(actions)
		} else {
			status.appendedActionable++
			status.appendedActionEntries += len(actions)
		}
		if !status.firstUnshiftedActionable {
			status.firstUnshiftedActionable = true
			status.firstUnshiftedActionState = s.top().state
			status.firstUnshiftedActionDepth = s.depth()
			status.firstUnshiftedActionByte = s.byteOffset
		}
		for _, act := range actions {
			switch act.Type {
			case ParseActionShift:
				status.unshiftedShiftActions++
				if inSnapshot {
					status.snapshotShiftActions++
				} else {
					status.appendedShiftActions++
				}
			case ParseActionReduce:
				status.unshiftedReduceActions++
				if inSnapshot {
					status.snapshotReduceActions++
				} else {
					status.appendedReduceActions++
				}
			case ParseActionAccept:
				status.unshiftedAcceptActions++
				if inSnapshot {
					status.snapshotAcceptActions++
				} else {
					status.appendedAcceptActions++
				}
			case ParseActionRecover:
				status.unshiftedRecoverActions++
				if inSnapshot {
					status.snapshotRecoverActions++
				} else {
					status.appendedRecoverActions++
				}
			default:
				status.unshiftedOtherActions++
				if inSnapshot {
					status.snapshotOtherActions++
				} else {
					status.appendedOtherActions++
				}
			}
		}
	}
	return status
}

const (
	stopFrontierStackTableLimit = 16
	stopFrontierPairLimit       = 128
)

type stopFrontierDiagnostics struct {
	stackTable string
	actions    string
	sameHeader string
	survivors  string
}

type stopFrontierHeaderGroup struct {
	state   StateID
	offset  uint32
	indices []int
}

func (p *Parser) buildStopFrontierDiagnostics(stacks []glrStack, tok Token, snapshotSize int, traces []dispatchStackSurvivorTrace) stopFrontierDiagnostics {
	status := p.classifyPostDispatchStacks(stacks, tok, snapshotSize)
	return stopFrontierDiagnostics{
		stackTable: p.stopFrontierStackTable(stacks, snapshotSize),
		actions:    status.progressString(),
		sameHeader: p.stopFrontierSameHeaderSummary(stacks),
		survivors:  p.stopFrontierSurvivorTrace(stacks, tok, snapshotSize, traces),
	}
}

func parseActionTypeDiagName(t ParseActionType) string {
	switch t {
	case ParseActionShift:
		return "shift"
	case ParseActionReduce:
		return "reduce"
	case ParseActionAccept:
		return "accept"
	case ParseActionRecover:
		return "recover"
	default:
		return fmt.Sprintf("type%d", t)
	}
}

func stackTraceState(s *glrStack) (StateID, uint32, int) {
	if s == nil {
		return 0, 0, 0
	}
	depth := s.depth()
	state := StateID(0)
	if depth > 0 {
		state = s.top().state
	}
	return state, s.byteOffset, depth
}

func actionSetSummary(actions []ParseAction) (entries, shifts, reduces, accepts, recovers, others int) {
	entries = len(actions)
	for _, act := range actions {
		switch act.Type {
		case ParseActionShift:
			shifts++
		case ParseActionReduce:
			reduces++
		case ParseActionAccept:
			accepts++
		case ParseActionRecover:
			recovers++
		default:
			others++
		}
	}
	return entries, shifts, reduces, accepts, recovers, others
}

func (p *Parser) stopFrontierSurvivorTrace(stacks []glrStack, tok Token, snapshotSize int, traces []dispatchStackSurvivorTrace) string {
	if len(stacks) == 0 || p == nil || p.language == nil {
		return ""
	}
	var b strings.Builder
	count := 0
	emitted := 0
	const limit = 8
	for i := range stacks {
		s := &stacks[i]
		if s.dead || s.accepted || s.shifted || s.cPaused || s.depth() == 0 {
			continue
		}
		actionIdx := p.lookupActionIndex(s.top().state, tok.Symbol)
		if actionIdx == 0 || int(actionIdx) >= len(p.language.ParseActions) {
			continue
		}
		actions := p.language.ParseActions[actionIdx].Actions
		if len(actions) == 0 {
			continue
		}
		count++
		if emitted >= limit {
			continue
		}
		if emitted == 0 {
			b.WriteString("survivors={rows=[")
		} else {
			b.WriteByte(';')
		}
		entries, shifts, reduces, accepts, recovers, others := actionSetSummary(actions)
		trace := dispatchStackSurvivorTrace{
			inSnapshot: snapshotSize < 0 || i < snapshotSize,
			origin:     "unknown",
		}
		if i < len(traces) {
			trace = traces[i]
		}
		if trace.origin == "" {
			trace.origin = "preexisting"
		}
		state, byteOffset, depth := stackTraceState(s)
		fmt.Fprintf(&b, "%d:%d@%d/d%d snap=%t visited=%t origin=%q source=%d path=%q action_index=%d action_count=%d action=%s(state=%d,symbol=%d,children=%d) before=%d@%d/d%d after_primary=%d@%d/d%d shifted=%t dead=%t acc=%t after_final=%d@%d/d%d shifted=%t dead=%t acc=%t final_actions={entries=%d shift=%d reduce=%d accept=%d recover=%d other=%d}",
			i,
			state,
			byteOffset,
			depth,
			trace.inSnapshot,
			trace.visited,
			trace.origin,
			trace.sourceIndex,
			trace.path,
			trace.actionIndex,
			trace.actionCount,
			parseActionTypeDiagName(trace.actionType),
			trace.actionState,
			trace.actionSymbol,
			trace.actionChildCount,
			trace.beforeState,
			trace.beforeByte,
			trace.beforeDepth,
			trace.afterPrimaryState,
			trace.afterPrimaryByte,
			trace.afterPrimaryDepth,
			trace.afterPrimaryShifted,
			trace.afterPrimaryDead,
			trace.afterPrimaryAccept,
			trace.afterFinalState,
			trace.afterFinalByte,
			trace.afterFinalDepth,
			trace.afterFinalShifted,
			trace.afterFinalDead,
			trace.afterFinalAccepted,
			entries,
			shifts,
			reduces,
			accepts,
			recovers,
			others,
		)
		if trace.frontier.ran {
			frontierAction := "none"
			if trace.frontier.terminalActionSet {
				frontierAction = parseActionTypeDiagName(trace.frontier.terminalActionTyp)
			}
			fmt.Fprintf(&b, " frontier={reason=%q step=%d before=%d@%d/d%d after=%d@%d/d%d shifted=%t dead=%t acc=%t last_reduce_seen=%t last_reduce=%d@%d/d%d terminal_action=%s terminal_count=%d}",
				trace.frontier.reason,
				trace.frontier.step,
				trace.frontier.beforeState,
				trace.frontier.beforeByte,
				trace.frontier.beforeDepth,
				trace.frontier.afterState,
				trace.frontier.afterByte,
				trace.frontier.afterDepth,
				trace.frontier.afterShifted,
				trace.frontier.afterDead,
				trace.frontier.afterAccepted,
				trace.frontier.lastReduceSeen,
				trace.frontier.lastReduceState,
				trace.frontier.lastReduceByte,
				trace.frontier.lastReduceDepth,
				frontierAction,
				trace.frontier.terminalCount,
			)
		}
		emitted++
	}
	if count == 0 {
		return ""
	}
	if emitted == 0 {
		return fmt.Sprintf("survivors={count=%d rows=[]}", count)
	}
	if omitted := count - emitted; omitted > 0 {
		fmt.Fprintf(&b, ";omitted=%d", omitted)
	}
	fmt.Fprintf(&b, "] count=%d}", count)
	return b.String()
}

func (p *Parser) stopFrontierStackTable(stacks []glrStack, snapshotSize int) string {
	var b strings.Builder
	live := 0
	emitted := 0
	for i := range stacks {
		if !stacks[i].dead {
			live++
		}
	}
	fmt.Fprintf(&b, "stacks={total=%d live=%d limit=%d rows=[", len(stacks), live, stopFrontierStackTableLimit)
	for i := range stacks {
		if emitted >= stopFrontierStackTableLimit {
			break
		}
		s := &stacks[i]
		state := StateID(0)
		depth := s.depth()
		if depth > 0 {
			state = s.top().state
		}
		if emitted > 0 {
			b.WriteByte(';')
		}
		fmt.Fprintf(&b, "%d:%d@%d/d%d sh=%t dead=%t acc=%t pause=%t rec=%t score=%d br=%d snap=%t",
			i,
			state,
			s.byteOffset,
			depth,
			s.shifted,
			s.dead,
			s.accepted,
			s.cPaused,
			s.cRec != nil,
			s.score,
			s.branchOrder,
			snapshotSize < 0 || i < snapshotSize,
		)
		emitted++
	}
	if omitted := len(stacks) - emitted; omitted > 0 {
		fmt.Fprintf(&b, ";omitted=%d", omitted)
	}
	b.WriteString("]}")
	return b.String()
}

func (p *Parser) stopFrontierSameHeaderSummary(stacks []glrStack) string {
	groups := make([]stopFrontierHeaderGroup, 0, len(stacks))
	for i := range stacks {
		if stacks[i].dead || stacks[i].depth() == 0 {
			continue
		}
		key := mergeKeyForStack(&stacks[i])
		found := -1
		for gi := range groups {
			if groups[gi].state == key.state && groups[gi].offset == key.byteOffset {
				found = gi
				break
			}
		}
		if found < 0 {
			groups = append(groups, stopFrontierHeaderGroup{state: key.state, offset: key.byteOffset})
			found = len(groups) - 1
		}
		groups[found].indices = append(groups[found].indices, i)
	}
	sameGroups := 0
	maxGroup := 0
	pairs := 0
	deepRejected := 0
	costDiff := 0
	gssEligible := 0
	gssAttemptable := 0
	gssWouldMerge := 0
	pairBudgetHit := false
	scratch := glrMergeScratch{}
	p.syncCRecoveryMergeScratch(&scratch)
	if p.mergeScratch != nil {
		// stopFrontierSameHeaderSummary runs synchronously inside parseInternal;
		// borrow the active arena so pending-parent diagnostics use the same exact
		// equality as production merge decisions instead of failing closed.
		scratch.arena = p.mergeScratch.arena
		scratch.faithfulCapOne = p.mergeScratch.faithfulCapOne
		scratch.recoveryCapOneConvergence = p.mergeScratch.recoveryCapOneConvergence
	}
	for gi := range groups {
		size := len(groups[gi].indices)
		if size > maxGroup {
			maxGroup = size
		}
		if size < 2 {
			continue
		}
		sameGroups++
		for ai := 0; ai < size; ai++ {
			for bi := ai + 1; bi < size; bi++ {
				if pairs >= stopFrontierPairLimit {
					pairBudgetHit = true
					continue
				}
				a := &stacks[groups[gi].indices[ai]]
				b := &stacks[groups[gi].indices[bi]]
				pairs++
				if cRecoveryMergeCostsDiffer(&scratch, a, b) {
					costDiff++
				}
				if !stackEquivalentForMergeState(&scratch, p.language, groups[gi].state, a, b) {
					deepRejected++
				}
				if gssMainCanMergeWithScratch(&scratch, a, b) {
					gssEligible++
					gssAttemptable++
					if !gssStacksHaveDistinctMaterializingShapesWithScratch(&scratch, a, b) {
						gssWouldMerge++
					}
				}
			}
		}
	}
	return fmt.Sprintf(
		"sameHeader={groups=%d max=%d pairs=%d deepReject=%d costDiff=%d externalScannerDiffKnown=false gssEligible=%d gssAttemptable=%d gssWouldMerge=%d pairBudgetHit=%t}",
		sameGroups,
		maxGroup,
		pairs,
		deepRejected,
		costDiff,
		gssEligible,
		gssAttemptable,
		gssWouldMerge,
		pairBudgetHit,
	)
}

func cRecoveryRelevantStack(stacks []glrStack) bool {
	for i := range stacks {
		if stacks[i].cPaused || stacks[i].cRec != nil || stacks[i].cRecoverMissingGroup != nil {
			return true
		}
	}
	return false
}

func (p *Parser) markCRecoveryCostCompetitionRelevant() {
	if p == nil {
		return
	}
	p.crecoveryCostCompetitionRelevant = true
	p.crecoveryCostCompetitionWalkEnabled = true
}

// syncCRecoveryMergeScratch publishes parser recovery state to active merge
// scratch immediately before a multi-stack merge and after parse resets.
func (p *Parser) syncCRecoveryMergeScratch(scratch *glrMergeScratch) {
	if p == nil || scratch == nil {
		return
	}
	scratch.language = p.language
	scratch.trace = p.glrTrace
	scratch.cRecoveryCost = false
	if !p.errorCostCompetitionEnabled() {
		scratch.cRecoveryCostWalk = false
		scratch.cRecoveryConvergence = false
		scratch.cRecoveryFallbackSuppression = false
		return
	}
	scratch.cRecoveryCostWalk = p.crecoveryCostCompetitionWalkEnabled
	scratch.cRecoveryConvergence = p.crecoveryCostCompetitionRelevant
	scratch.cRecoveryFallbackSuppression = p.crecoveryCostCompetitionRelevant
}

func resetCRecoveryMergeScratch(scratch *glrMergeScratch) {
	if scratch == nil {
		return
	}
	scratch.cRecoveryCostWalk = false
	scratch.cRecoveryConvergence = false
	scratch.cRecoveryFallbackSuppression = false
	scratch.cRecoveryCost = false
}

func (p *Parser) resetCRecoveryCostCompetitionState() {
	if p == nil {
		return
	}
	p.crecoveryCostCompetitionRelevant = false
	p.crecoveryCostCompetitionWalkEnabled = false
	p.cRecoverSharedTokenErrorModeLexed = false
	p.cRecoverCustomResyncActive = false
	p.cRecoverCustomResyncByte = 0
	p.cRecoverCustomSourceEligible = false
	p.syncCRecoveryMergeScratch(p.mergeScratch)
}

// clearCRecoveryCostIfClean ends recovery-cost walks after condense removes a
// transient recovery branch. A false trackChildErrors value proves that this
// parse has not built an ERROR or MISSING node.
func (p *Parser) clearCRecoveryCostIfClean(stacks []glrStack, trackChildErrors *bool) {
	if p == nil || trackChildErrors == nil || *trackChildErrors {
		return
	}
	if p.crecoveryCostCompetitionRelevant {
		for i := range stacks {
			s := &stacks[i]
			if s.dead {
				continue
			}
			if s.cPaused || s.cRec != nil || s.cRecoverMissingGroup != nil {
				return
			}
		}
	}
	p.crecoveryCostCompetitionWalkEnabled = false
	p.crecoveryCostCompetitionRelevant = false
	p.syncCRecoveryMergeScratch(p.mergeScratch)
}

func stopCondenseGatingString(errorCostCompetitionEnabled, anyReduced, condenseRelevant, condenseRan, condenseResumed bool, stacks []glrStack) string {
	hasPaused := false
	hasCRec := false
	hasMissingGroup := false
	for i := range stacks {
		if stacks[i].dead {
			continue
		}
		if stacks[i].cPaused {
			hasPaused = true
		}
		if stacks[i].cRec != nil {
			hasCRec = true
		}
		if stacks[i].cRecoverMissingGroup != nil {
			hasMissingGroup = true
		}
	}
	return fmt.Sprintf(
		"condense={errorCostCompetitionEnabled=%t anyReduced=%t hasPaused=%t hasCRec=%t hasMissingGroup=%t condenseSkippedDueToAnyReduced=%t condenseRan=%t condenseResumed=%t}",
		errorCostCompetitionEnabled,
		anyReduced,
		hasPaused,
		hasCRec,
		hasMissingGroup,
		errorCostCompetitionEnabled && anyReduced && condenseRelevant && !condenseRan,
		condenseRan,
		condenseResumed,
	)
}

func (d *parseStopActionDiagnostic) progressString() string {
	if d == nil || !d.captured {
		return ""
	}
	s := fmt.Sprintf(
		"last_action_phase=%q last_action_stack=%d[%d]@%d last_action_token=%d[%d:%d] last_action_no_lookahead=%t last_action_type=%d last_action_state=%d last_action_symbol=%d last_action_child_count=%d last_action_production_id=%d last_action_dynamic_precedence=%d last_action_count=%d last_action_result_state=%d last_action_reduce_chain=%t last_action_chain_step=%d last_action_repeated_sig_count=%d last_action_chain_cycle=%t dispatch_shift_actions=%d dispatch_reduce_actions=%d dispatch_accept_actions=%d dispatch_recover_actions=%d dispatch_other_actions=%d force_advance_after_reduce=%t post_dispatch_default_reduce=%t",
		d.phase,
		d.stackState,
		d.stackDepth,
		d.stackByte,
		d.lookaheadSymbol,
		d.lookaheadStartByte,
		d.lookaheadEndByte,
		d.lookaheadNoLookahead,
		d.actionType,
		d.actionState,
		d.actionSymbol,
		d.actionChildCount,
		d.actionProductionID,
		d.actionDynamicPrecedence,
		d.actionCount,
		d.resultState,
		d.inReduceChain,
		d.reduceChainStep,
		d.repeatedReduceSignatureCount,
		d.reduceChainCycle,
		d.dispatchShiftActions,
		d.dispatchReduceActions,
		d.dispatchAcceptActions,
		d.dispatchRecoverActions,
		d.dispatchOtherActions,
		d.forceAdvanceAfterReduce,
		d.postDispatchDefaultReduced,
	)
	if d.lastReduceCaptured {
		s += fmt.Sprintf(
			" last_reduce_phase=%q last_reduce_stack=%d[%d]@%d last_reduce_symbol=%d last_reduce_child_count=%d last_reduce_production_id=%d last_reduce_dynamic_precedence=%d last_reduce_result_state=%d last_reduce_chain=%t last_reduce_chain_step=%d last_reduce_repeated_sig_count=%d last_reduce_chain_cycle=%t",
			d.lastReducePhase,
			d.lastReduceStackState,
			d.lastReduceStackDepth,
			d.lastReduceStackByte,
			d.lastReduceSymbol,
			d.lastReduceChildCount,
			d.lastReduceProductionID,
			d.lastReduceDynamicPrecedence,
			d.lastReduceResultState,
			d.lastReduceInChain,
			d.lastReduceChainStep,
			d.lastReduceRepeatedSigCount,
			d.lastReduceChainCycle,
		)
	}
	return s
}

func (d *parseStopActionDiagnostic) beginDispatch() {
	if d == nil {
		return
	}
	d.captured = false
	d.dispatchShiftActions = 0
	d.dispatchReduceActions = 0
	d.dispatchAcceptActions = 0
	d.dispatchRecoverActions = 0
	d.dispatchOtherActions = 0
	d.lastReduceCaptured = false
}

func (p *Parser) noteStopActionDiagnostic(phase string, s *glrStack, tok Token, act ParseAction, actionOrdinal, actionCount int, inReduceChain bool, chainStep int, repeatedCount int, cycle bool) {
	semanticPhaseTraceRecordActionExecution(p, s, tok, act, actionOrdinal, phase, cycle) // semantic-phase-assembly: action-execution seam
	if p == nil || p.stopActionDiag == nil || s == nil || s.dead || s.accepted || s.depth() == 0 {
		return
	}
	d := p.stopActionDiag
	d.captured = true
	d.phase = phase
	d.stackState = s.top().state
	d.stackByte = s.byteOffset
	d.stackDepth = s.depth()
	d.lookaheadSymbol = tok.Symbol
	d.lookaheadStartByte = tok.StartByte
	d.lookaheadEndByte = tok.EndByte
	d.lookaheadNoLookahead = tok.NoLookahead
	d.actionType = act.Type
	d.actionState = act.State
	d.actionSymbol = act.Symbol
	d.actionChildCount = act.ChildCount
	d.actionProductionID = act.ProductionID
	d.actionDynamicPrecedence = act.DynamicPrecedence
	d.actionCount = actionCount
	d.resultState = s.top().state
	d.inReduceChain = inReduceChain
	d.reduceChainStep = chainStep
	d.repeatedReduceSignatureCount = repeatedCount
	d.reduceChainCycle = cycle
	switch act.Type {
	case ParseActionShift:
		d.dispatchShiftActions++
	case ParseActionReduce:
		d.dispatchReduceActions++
		d.lastReduceCaptured = true
		d.lastReducePhase = phase
		d.lastReduceStackState = d.stackState
		d.lastReduceStackByte = d.stackByte
		d.lastReduceStackDepth = d.stackDepth
		d.lastReduceSymbol = act.Symbol
		d.lastReduceChildCount = act.ChildCount
		d.lastReduceProductionID = act.ProductionID
		d.lastReduceDynamicPrecedence = act.DynamicPrecedence
		d.lastReduceResultState = d.resultState
		d.lastReduceInChain = inReduceChain
		d.lastReduceChainStep = chainStep
		d.lastReduceRepeatedSigCount = repeatedCount
		d.lastReduceChainCycle = cycle
	case ParseActionAccept:
		d.dispatchAcceptActions++
	case ParseActionRecover:
		d.dispatchRecoverActions++
	default:
		d.dispatchOtherActions++
	}
}

func (p *Parser) noteStopActionResult(s *glrStack) {
	if workCountInstrumentationEnabled {
		workCountTopologyRecordActionResult(s) // work-count-assembly: topology action-result seam
	}
	if p == nil || p.stopActionDiag == nil || !p.stopActionDiag.captured || s == nil || s.depth() == 0 {
		return
	}
	p.stopActionDiag.resultState = s.top().state
	if p.stopActionDiag.actionType == ParseActionReduce {
		p.stopActionDiag.lastReduceResultState = s.top().state
	}
}

type smallActionPair struct {
	sym uint16
	val uint16
}

type recoverSymbolAction struct {
	sym    uint16
	action ParseAction
}

const (
	// maxForkCloneDepth limits GLR stack cloning for pathological ambiguity.
	// Above this depth, we execute only the first action to avoid runaway work.
	maxForkCloneDepth = 4 * 1024
	// maxConsecutivePrimaryReduces prevents infinite reduce loops on the
	// primary stack when no token advancement occurs.
	maxConsecutivePrimaryReduces = 256
	// maxConsecutiveNoTokenDispatches prevents multi-stack GLR reduce/merge
	// loops that keep re-dispatching the same concrete lookahead without
	// advancing the token source.
	maxConsecutiveNoTokenDispatches = 128
	// maxConsecutiveMissingSingleShifts prevents single-stack recovery from
	// cycling forever by repeatedly inserting the same missing token before
	// the same lookahead without advancing the parse position.
	maxConsecutiveMissingSingleShifts = 16
	// Allow a small temporary oversubscription on full parses before
	// triggering expensive global stack culling, mirroring the C runtime's
	// version overflow window.
	fullParseGLRStackOverflow = 4
)

// IncrementalParseProfile attributes incremental parse time into coarse buckets.
//
// ReuseCursorNanos includes reuse-cursor setup and subtree-candidate checks.
// ReparseNanos includes the remainder of incremental parsing/rebuild work.
type IncrementalParseProfile struct {
	ReuseCursorNanos                   int64
	ReparseNanos                       int64
	ReusedSubtrees                     uint64
	ReusedBytes                        uint64
	NewNodesAllocated                  uint64
	ReuseUnsupported                   bool
	ReuseUnsupportedReason             string
	AcceptedErrorRetryAttempts         uint8
	AcceptedErrorRetryAdopted          bool
	AcceptedErrorRetryMergePerKey      int
	AcceptedErrorRetryCause            IncrementalRetryCause
	OldTreeReuseRoute                  bool
	ReuseRejectDirty                   uint64
	ReuseRejectAncestorDirtyBeforeEdit uint64
	ReuseRejectHasError                uint64
	ReuseRejectInvalidSpan             uint64
	ReuseRejectOutOfBounds             uint64
	ReuseRejectRootNonLeafChanged      uint64
	// ReuseObservedPreGotoStateMismatch counts top-level block-splice candidates
	// observed at a live parser state different from the node's recorded
	// PreGotoState. This is diagnostic only: the established admission contract
	// remains goto-target compatibility plus fragility until #432 replaces it
	// with a complete ownership proof.
	ReuseObservedPreGotoStateMismatch uint64
	ReuseRejectLargeNonLeaf           uint64
	ReuseRejectStaleNonLeafBoundary   uint64
	// ReuseRejectFragileNonLeaf counts interior (non-leaf) reuse candidates
	// rejected because Node.isFragile() reported the candidate was built
	// under an ambiguous parse decision (LR-table conflict, GSS multi-pop, or
	// concurrent GLR stack versions) or is itself an ERROR/MISSING node -- see
	// markReduceFragility (parser_reduce.go) and reuseNonLeafTargetStateOnStack
	// (incremental.go). A nonzero count on a conflict-heavy grammar (e.g. js)
	// is expected and correct: it is exactly the unsound reuse this gate is
	// designed to prevent.
	ReuseRejectFragileNonLeaf uint64
	// BlockSpliceSteps is the number of top-level sibling reuses taken inside
	// the W1 block-splice composition loop (spec.campaign.oedit): one per
	// sibling spliced without a full main-loop round trip. It is O(edit) and
	// deterministic for a fixed (source, edit, language).
	BlockSpliceSteps uint64
	// ReuseRejectScannerUnquiescent counts reuse candidates the external
	// scanner checkpoint/quiescence gate rejected -- a checkpoint state
	// mismatch on a checkpoint language, or a refuted quiescence proof on an
	// opt-out scanner (campaign O(edit) workstream W4, spec.campaign.oedit).
	// It is 0 for stateless-scanner languages such as Go, whose ASI scanner
	// carries no cross-token state and is proven quiescent at every boundary
	// (external_scanner_quiescence.go). A nonzero count marks boundaries where
	// scanner state, not fragility or byte drift, is the binding constraint.
	ReuseRejectScannerUnquiescent       uint64
	RecoverSearches                     uint64
	RecoverStateChecks                  uint64
	RecoverStateSkips                   uint64
	RecoverSymbolSkips                  uint64
	RecoverLookups                      uint64
	RecoverHits                         uint64
	MaxStacksSeen                       int
	EntryScratchPeak                    uint64
	StopReason                          ParseStopReason
	TokensConsumed                      uint64
	LastTokenEndByte                    uint32
	ExpectedEOFByte                     uint32
	ArenaBytesAllocated                 int64
	ScratchBytesAllocated               int64
	EntryScratchBytesAllocated          int64
	GSSBytesAllocated                   int64
	SingleStackIterations               int
	MultiStackIterations                int
	SingleStackTokens                   uint64
	MultiStackTokens                    uint64
	SingleStackGSSNodes                 uint64
	MultiStackGSSNodes                  uint64
	GSSNodesAllocated                   uint64
	GSSNodesRetained                    uint64
	GSSNodesDroppedSameToken            uint64
	ParentNodesAllocated                uint64
	ParentNodesRetained                 uint64
	ParentNodesDroppedSameToken         uint64
	LeafNodesAllocated                  uint64
	LeafNodesRetained                   uint64
	LeafNodesDroppedSameToken           uint64
	MergeStacksIn                       uint64
	MergeStacksOut                      uint64
	MergeSlotsUsed                      uint64
	GlobalCullStacksIn                  uint64
	GlobalCullStacksOut                 uint64
	ParserLoopNanos                     int64
	TokenNextNanos                      int64
	ActionDispatchNanos                 int64
	ActionLookupNanos                   int64
	GLRMergeNanos                       int64
	GLRCullNanos                        int64
	ResultSelectionNanos                int64
	TransientParentMaterializationNanos int64
	ResultTreeBuildNanos                int64
	TransientChildMaterializationNanos  int64
	ResultPythonKeywordRepairNanos      int64
	ResultPythonRootRepairNanos         int64
	ResultFinalizeRootNanos             int64
	ResultExtendTrailingNanos           int64
	ResultNormalizeRootStartNanos       int64
	ResultCompatibilityNanos            int64
	ResultParentLinkNanos               int64
	ReduceRangeNanos                    int64
	ReducePendingParentNanos            int64
	ReduceChildBuildNanos               int64
	ReduceParentBuildNanos              int64
	ReduceSpanNanos                     int64
	ReduceStackPushNanos                int64
	ReduceNoTreeBuildNanos              int64
	ActionExtraShiftNanos               int64
	ActionNoActionNanos                 int64
	ActionNoActionRelexNanos            int64
	ActionNoActionMissingNanos          int64
	ActionNoActionRecoverNanos          int64
	ActionNoActionErrorNanos            int64
	ActionConflictChoiceNanos           int64
	ActionConflictForkNanos             int64
	ActionSingleShiftNanos              int64
	ActionSingleReduceNanos             int64
	ActionSingleAcceptNanos             int64
	ActionSingleRecoverNanos            int64
	ActionSingleOtherNanos              int64
	NormalizationNanos                  int64
}

type incrementalParseTiming struct {
	totalNanos                         int64
	reuseNanos                         int64
	reusedSubtrees                     uint64
	reusedBytes                        uint64
	newNodes                           uint64
	reuseUnsupported                   bool
	reuseUnsupportedReason             string
	acceptedErrorRetryAttempts         uint8
	acceptedErrorRetryAdopted          bool
	acceptedErrorRetryMergePerKey      int
	acceptedErrorRetryCause            IncrementalRetryCause
	oldTreeReuseRoute                  bool
	reuseRejectDirty                   uint64
	reuseRejectAncestorDirtyBeforeEdit uint64
	reuseRejectHasError                uint64
	reuseRejectInvalidSpan             uint64
	reuseRejectOutOfBounds             uint64
	reuseRejectRootNonLeafChanged      uint64
	reuseObservedPreGotoStateMismatch  uint64
	reuseRejectLargeNonLeaf            uint64
	reuseRejectStaleNonLeafBoundary    uint64
	reuseRejectFragileNonLeaf          uint64
	reuseRejectScannerUnquiescent      uint64
	// blockSpliceSteps counts top-level sibling reuses taken inside the W1
	// block-splice composition loop (spec.campaign.oedit) -- one per sibling
	// spliced without returning to the main parse-loop iteration. It is O(edit)
	// diagnostic evidence that the block path fired, and it is deterministic
	// for a fixed (source, edit, language): the loop reads only the live
	// single-stack frontier and the old tree, never per-instance history.
	blockSpliceSteps                    uint64
	recoverSearches                     uint64
	recoverStateChecks                  uint64
	recoverStateSkips                   uint64
	recoverSymbolSkips                  uint64
	recoverLookups                      uint64
	recoverHits                         uint64
	maxStacksSeen                       int
	entryScratchPeak                    uint64
	stopReason                          ParseStopReason
	tokensConsumed                      uint64
	lastTokenEndByte                    uint32
	expectedEOFByte                     uint32
	arenaBytesAllocated                 int64
	scratchBytesAllocated               int64
	entryScratchBytesAllocated          uint64
	gssBytesAllocated                   uint64
	singleStackIterations               int
	multiStackIterations                int
	singleStackTokens                   uint64
	multiStackTokens                    uint64
	singleStackGSSNodes                 uint64
	multiStackGSSNodes                  uint64
	gssNodesAllocated                   uint64
	gssNodesRetained                    uint64
	gssNodesDroppedSameToken            uint64
	parentNodesAllocated                uint64
	parentNodesRetained                 uint64
	parentNodesDroppedSameToken         uint64
	leafNodesAllocated                  uint64
	leafNodesRetained                   uint64
	leafNodesDroppedSameToken           uint64
	mergeStacksIn                       uint64
	mergeStacksOut                      uint64
	mergeSlotsUsed                      uint64
	globalCullStacksIn                  uint64
	globalCullStacksOut                 uint64
	parserLoopNanos                     int64
	tokenNextNanos                      int64
	actionDispatchNanos                 int64
	actionLookupNanos                   int64
	glrMergeNanos                       int64
	glrCullNanos                        int64
	resultSelectionNanos                int64
	transientParentMaterializationNanos int64
	resultTreeBuildNanos                int64
	transientChildMaterializationNanos  int64
	resultPythonKeywordRepairNanos      int64
	resultPythonRootRepairNanos         int64
	resultFinalizeRootNanos             int64
	resultExtendTrailingNanos           int64
	resultNormalizeRootStartNanos       int64
	resultCompatibilityNanos            int64
	resultParentLinkNanos               int64
	reduceRangeNanos                    int64
	reducePendingParentNanos            int64
	reduceChildBuildNanos               int64
	reduceParentBuildNanos              int64
	reduceSpanNanos                     int64
	reduceStackPushNanos                int64
	reduceNoTreeBuildNanos              int64
	actionExtraShiftNanos               int64
	actionNoActionNanos                 int64
	actionNoActionRelexNanos            int64
	actionNoActionMissingNanos          int64
	actionNoActionRecoverNanos          int64
	actionNoActionErrorNanos            int64
	actionConflictChoiceNanos           int64
	actionConflictForkNanos             int64
	actionSingleShiftNanos              int64
	actionSingleReduceNanos             int64
	actionSingleAcceptNanos             int64
	actionSingleRecoverNanos            int64
	actionSingleOtherNanos              int64
	normalizationNanos                  int64
}

type parseReuseState struct {
	reusedAny bool
	arenaRefs []*nodeArena
}

// NewParser creates a new Parser for the given language.
func NewParser(lang *Language) *Parser {
	p := &Parser{language: lang}
	p.activeParseStopCheckFn = p.activeParseStopReason
	if lang != nil {
		p.forceRawSpanAll = lang.Name == "yaml"
		p.leafInternByLang = languageWantsLeafInterning(lang.Name)
		for i, name := range lang.SymbolNames {
			if name != "statement_list" {
				continue
			}
			if p.forceRawSpanTable == nil {
				p.forceRawSpanTable = make([]bool, len(lang.SymbolNames))
			}
			p.forceRawSpanTable[i] = true
		}
		if isCobolLanguage(lang) {
			for i, name := range lang.SymbolNames {
				if !strings.HasSuffix(name, "_division") &&
					!strings.Contains(name, "_statement") &&
					!strings.HasSuffix(name, "_option") &&
					!strings.HasSuffix(name, "_clause") &&
					!strings.HasSuffix(name, "_section") &&
					!strings.HasSuffix(name, "_paragraph") {
					continue
				}
				if p.forceRawSpanTable == nil {
					p.forceRawSpanTable = make([]bool, len(lang.SymbolNames))
				}
				p.forceRawSpanTable[i] = true
			}
		}
		p.denseLimit = languageDenseLimit(lang)
		// Bind the cached action-lookup closure eagerly at language-config
		// time so lookupActionIndexFunc's lazy fallback never races, even in
		// the (unsupported) shared-Parser case.
		p.lookupActionIndexFn = p.lookupActionIndex
		p.smallBase = int(lang.LargeStateCount)
		derived := lang.acquireParserDerivedTables()
		p.smallTokenLookup = derived.smallTokenLookup
		p.smallLookup = derived.smallLookup
		p.externalValidByState = p.buildExternalValidByState()
		p.externalValidMaskByState = buildExternalValidMaskByState(p.externalValidByState, len(lang.ExternalSymbols))
		p.hasExtraChainActions = languageHasExtraChainActions(lang)
		p.classifiedActions = derived.classifiedActions
		p.eagerDefaultReduces = derived.eagerDefaultReduces
		p.reduceChainHints = buildReduceChainHints(lang)
		p.reduceChainHintByState = buildReduceChainHintIndex(p.reduceChainHints)
		p.reduceAliasSeq = buildReduceAliasSequences(lang)
		p.aliasTargetSymbol = buildAliasTargetSymbols(lang)
		p.keepSameNamedAnonChildSymbol = derived.keepSameNamedAnonChildSymbol
		p.sharedAnonymousTokenSymbol = derived.sharedAnonymousTokenSymbol
		p.reduceHasFields = buildReduceFieldPresence(lang)
		p.reduceFieldPlans = buildReduceFieldPlans(lang)
		p.recoverByState, p.hasRecoverState, p.hasRecoverSymbol = buildRecoverActionsByState(lang)
		p.hasKeywordState = buildKeywordStates(lang)
		p.spanExtendingInvisibleSymbols, p.nonSpanExtendingInvisibleSymbols = buildInvisibleSpanSymbolTables(lang.SymbolNames)
		p.aliasPreservedWrapperSymbols = buildAliasPreservedWrapperSymbols(lang)
		p.collapsedChildOccurrencePairs, p.collapsedChildOccurrenceSet = compileCollapsedChildOccurrencePolicy(lang)
		p.unaryWrapperFlatteningSet = compileUnaryWrapperFlatteningPolicy(lang)
		p.initTypeScriptContextualKeywordSymbols(lang)
		p.initSchemeErrorRecoverySymbols(lang)
		p.errorCostCompetition = errorCostCompetitionLanguage(lang)
		p.rootSymbol, p.hasRootSymbol = p.inferRootSymbol()
		p.maxConflictWidth = computeMaxConflictWidth(lang)
	}
	return p
}

func languageHasExtraChainActions(lang *Language) bool {
	if lang == nil {
		return false
	}
	for _, entry := range lang.ParseActions {
		for _, action := range entry.Actions {
			if action.ExtraChain {
				return true
			}
		}
	}
	return false
}

func snippetParserPool(lang *Language) *sync.Pool {
	if lang == nil {
		return nil
	}
	if pool, ok := snippetParserPools.Load(lang); ok {
		return pool.(*sync.Pool)
	}
	pool := &sync.Pool{
		New: func() any {
			return NewParser(lang)
		},
	}
	actual, _ := snippetParserPools.LoadOrStore(lang, pool)
	return actual.(*sync.Pool)
}

func acquireSnippetParser(lang *Language) *Parser {
	pool := snippetParserPool(lang)
	if pool == nil {
		return nil
	}
	parser, _ := pool.Get().(*Parser)
	if parser == nil {
		parser = NewParser(lang)
	}
	resetSnippetParser(parser)
	return parser
}

func releaseSnippetParser(parser *Parser) {
	if parser == nil || parser.language == nil {
		return
	}
	resetSnippetParser(parser)
	if pool := snippetParserPool(parser.language); pool != nil {
		pool.Put(parser)
	}
}

func resetSnippetParser(parser *Parser) {
	if parser == nil {
		return
	}
	parser.clearRecoveryRuntimeTelemetryDetailed()
	parser.finishCNodeMemoParse()
	parser.resetCRecoveryCostCompetitionState()
	resetGSSPrefixPath(&parser.cPrefixPath)
	parser.reparseFactory = nil
	parser.recoveryParser = nil
	parser.skipRecoveryReparse = false
	parser.recoveryInitialOnly = false
	parser.releaseCompatibilityBorrowedArenas()
	parser.fullArenaHint = 0
	parser.pendingFullArenaHint = 0
	parser.compactFullArenaHint = 0
	parser.finalChildRefArenaHint = 0
	parser.incrementalArenaHint = 0
	parser.fullGSSHint = 0
	parser.incrementalGSSHint = 0
	parser.included = nil
	parser.logger = nil
	parser.glrTrace = false
	parser.ambiguityProfile = nil
	parser.noTreeBenchmarkOnly = false
	parser.noTreeCheckpointBenchmarkOnly = false
	parser.compactNoTreeShiftLeaves = false
	parser.compactFullShiftLeaves = false
	parser.pendingFullParents = false
	parser.finalChildRefs = false
	parser.skipInvisibleFullLeafCheckpoints = false
	parser.noResultCompatibilityBenchmarkOnly = false
	parser.timeoutMicros = 0
	parser.cancellationFlag = nil
	parser.parseBudgetDepth = 0
	parser.cNodeMemoOperationDepth = 0
	parser.cNodeMemoPeakTier = RecoveryNodeMemoTierNone
	parser.cNodeMemoOperationPeakTier = RecoveryNodeMemoTierNone
	if parser.cCondenseVersionKeyRanks != nil {
		clear(parser.cCondenseVersionKeyRanks)
	}
	if cold := parser.forestDeclineMemo; cold != nil {
		cold.cNodeMemoRetainedCache = nil
		cold.cNodeMemoCollisions = 0
	}
	parser.parseDeadline = time.Time{}
	parser.parseStoppedReason = ParseStopNone
	parser.parseRuntimeMemoryBudgetBytes = 0
	parser.parseRuntimeMemoryBaselineBytes = 0
	parser.parseRuntimeMemoryBaselineSys = 0
	parser.parseRuntimeMemoryPoll = 0
	parser.parseRuntimeMemoryVolumeAtPoll = 0
	parser.parseRuntimeMemoryHardCeilingBytes = 0
	parser.parseMemoryBudgetDiag = parseMemoryBudgetDiagnostic{}
	parser.parseMemoryBudgetDiagActive = false
	parser.compatMemoryBudgetTripped = false
	// Recovery and snippet sub-parsers must never route through the compact
	// candidate: they parse fragments spliced into recovery and reuse.
	parser.pinToProductionRoute()
	// Release *Node refs so the arenas from the last incremental parse can be
	// collected by the GC. Without this, a Parser sitting in a sync.Pool keeps
	// its reuseCursor.topLevel/*Node alive, preventing arena reclamation.
	parser.reuseCursor.releaseNodeRefs()
	parser.reuseScratch.releaseNodeRefs()
	parser.resetPendingStackBuffersAtBoundary()
}

// InferredRootSymbol returns the root symbol inferred during parser
// construction, and whether inference succeeded.
func (p *Parser) InferredRootSymbol() (Symbol, bool) {
	return p.rootSymbol, p.hasRootSymbol
}

// computeMaxConflictWidth scans the parse action table and returns the
// widest N-way conflict (largest len(entry.Actions)). This determines the
// minimum GLR stack cap needed to keep all fork paths alive.
func computeMaxConflictWidth(lang *Language) int {
	maxWidth := 1
	for i := range lang.ParseActions {
		if n := len(lang.ParseActions[i].Actions); n > maxWidth {
			maxWidth = n
		}
	}
	return maxWidth
}

func (p *Parser) inferRootSymbol() (Symbol, bool) {
	if p == nil || p.language == nil {
		return 0, false
	}
	lang := p.language
	if lang.SymbolCount == 0 || lang.TokenCount >= lang.SymbolCount {
		return 0, false
	}
	// ts2go grammars use InitialState=1 (tree-sitter convention). Hand-built
	// test grammars often leave InitialState=0 and may not have a unique
	// start-symbol shape; skip inference there.
	if lang.InitialState == 0 {
		return 0, false
	}
	initial := lang.InitialState
	var candidate Symbol
	found := false
	for sym := Symbol(lang.TokenCount); uint32(sym) < lang.SymbolCount; sym++ {
		gotoState := p.lookupGoto(initial, sym)
		if gotoState == 0 {
			continue
		}
		if !p.stateHasAcceptOnEOF(gotoState) {
			continue
		}
		if !found {
			candidate = sym
			found = true
			continue
		}
		if p.preferRootSymbol(sym, candidate) {
			candidate = sym
		}
	}
	return candidate, found
}

func (p *Parser) stateHasAcceptOnEOF(state StateID) bool {
	if p == nil || p.language == nil {
		return false
	}
	idx := p.lookupActionIndex(state, 0)
	if idx == 0 || int(idx) >= len(p.language.ParseActions) {
		return false
	}
	actions := p.language.ParseActions[idx].Actions
	for i := range actions {
		if actions[i].Type == ParseActionAccept {
			return true
		}
	}
	return false
}

func (p *Parser) preferRootSymbol(candidate, current Symbol) bool {
	score := func(sym Symbol) int {
		s := 0
		if p != nil && p.language != nil && int(sym) < len(p.language.SymbolMetadata) {
			meta := p.language.SymbolMetadata[sym]
			if meta.Visible {
				s += 2
			}
			if meta.Named {
				s++
			}
		}
		if p != nil && p.language != nil && int(sym) < len(p.language.SymbolNames) {
			switch p.language.SymbolNames[sym] {
			case "source_file", "program", "module", "document", "file":
				s += 3
			}
		}
		return s
	}
	candidateScore := score(candidate)
	currentScore := score(current)
	if candidateScore != currentScore {
		return candidateScore > currentScore
	}
	return candidate < current
}

// tryRelexBroadDFA attempts to re-lex the current token position using the
// layout fallback lex state (state 0's DFA), which includes ALL terminal
// symbols. If it produces a different token that has valid actions in the
// current parser state, return it. This handles cases where the per-state
// lex mode's catch-all consumed input meant for a keyword/comment after
// reductions changed the parser state.
func (p *Parser) tryRelexBroadDFA(tok Token, parserState StateID, ts TokenSource) (Token, bool) {
	if p == nil || p.language == nil || ts == nil {
		return Token{}, false
	}
	dts, ok := ts.(*dfaTokenSource)
	if !ok || dts == nil || dts.lexer == nil {
		return Token{}, false
	}
	// Get the broad lex state (state 0's lex mode)
	if len(p.language.LexModes) == 0 {
		return Token{}, false
	}
	broadLS := p.language.LexModes[0].LexStateIndex()

	// Save lexer state
	savedPos, savedRow, savedCol := dts.lexer.pos, dts.lexer.row, dts.lexer.col

	type broadRelexCandidate struct {
		token      Token
		extraShift bool
	}

	tryAt := func(pos int, row, col uint32) (broadRelexCandidate, bool) {
		dts.lexer.pos, dts.lexer.row, dts.lexer.col = pos, row, col
		tok2 := dts.lexer.Next(broadLS)
		if tok2.Symbol == 0 {
			return broadRelexCandidate{}, false
		}
		// The broad lexer can skip extras before returning a token. Relexing is
		// only safe when any intentional layout skip is explicit at the call site.
		if int(tok2.StartByte) != pos {
			return broadRelexCandidate{}, false
		}
		actionIdx := p.lookupActionIndex(parserState, tok2.Symbol)
		if actionIdx == 0 || int(actionIdx) >= len(p.language.ParseActions) ||
			len(p.language.ParseActions[actionIdx].Actions) == 0 {
			return broadRelexCandidate{}, false
		}
		extraShift := false
		for _, action := range p.language.ParseActions[actionIdx].Actions {
			if action.Type == ParseActionShift && action.Extra {
				extraShift = true
				break
			}
		}
		if p.glrTrace {
			fmt.Printf("  RELEX: %s(%d) → %s(%d) in state=%d\n",
				p.language.SymbolNames[tok.Symbol], tok.Symbol,
				p.language.SymbolNames[tok2.Symbol], tok2.Symbol,
				parserState)
		}
		return broadRelexCandidate{token: tok2, extraShift: extraShift}, true
	}

	isImmediate := int(tok.Symbol) < len(p.language.ImmediateTokens) && p.language.ImmediateTokens[tok.Symbol]
	if isImmediate {
		if cand, ok := tryAt(int(tok.StartByte), tok.StartPoint.Row, tok.StartPoint.Column); ok {
			return cand.token, true
		}
	} else {
		if cand, ok := tryAt(int(tok.StartByte), tok.StartPoint.Row, tok.StartPoint.Column); ok &&
			(cand.extraShift || p.sameSurfaceRelexToken(tok, cand.token)) {
			return cand.token, true
		}
	}

	skipPos, skipCol := int(tok.StartByte), tok.StartPoint.Column
	for skipPos < len(dts.lexer.source) &&
		(dts.lexer.source[skipPos] == ' ' || dts.lexer.source[skipPos] == '\t') {
		skipPos++
		skipCol++
	}
	if skipPos > int(tok.StartByte) {
		if cand, ok := tryAt(skipPos, tok.StartPoint.Row, skipCol); ok && (isImmediate || cand.extraShift) {
			return cand.token, true
		}
	}

	isStringContent := int(tok.Symbol) < len(p.language.SymbolNames) && p.language.SymbolNames[tok.Symbol] == "string_content"
	if isStringContent {
		skipPos, skipRow, skipCol := int(tok.StartByte), tok.StartPoint.Row, tok.StartPoint.Column
		for skipPos < len(dts.lexer.source) {
			b := dts.lexer.source[skipPos]
			if b == ' ' || b == '\t' {
				skipPos++
				skipCol++
				continue
			}
			if b == '\n' {
				skipPos++
				skipRow++
				skipCol = 0
				continue
			}
			if b == '\r' {
				skipPos++
				skipCol = 0
				continue
			}
			break
		}
		if skipPos > int(tok.StartByte) {
			if cand, ok := tryAt(skipPos, skipRow, skipCol); ok &&
				p.allowStringContentWhitespaceBroadRelex(cand.token.Symbol) {
				return cand.token, true
			}
		}
	}

	// Restore lexer state
	dts.lexer.pos, dts.lexer.row, dts.lexer.col = savedPos, savedRow, savedCol
	return Token{}, false
}

func (p *Parser) sameSurfaceRelexToken(original, candidate Token) bool {
	if p == nil || p.language == nil {
		return false
	}
	if original.StartByte != candidate.StartByte || original.EndByte != candidate.EndByte {
		return false
	}
	if original.Symbol == 0 || candidate.Symbol == 0 || original.Symbol == candidate.Symbol {
		return false
	}
	return p.symbolSurfaceName(original.Symbol) != "" &&
		p.symbolSurfaceName(original.Symbol) == p.symbolSurfaceName(candidate.Symbol)
}

func (p *Parser) symbolSurfaceName(sym Symbol) string {
	if p == nil || p.language == nil {
		return ""
	}
	idx := int(sym)
	if idx < 0 {
		return ""
	}
	if idx < len(p.language.SymbolMetadata) && p.language.SymbolMetadata[idx].Name != "" {
		return p.language.SymbolMetadata[idx].Name
	}
	if idx < len(p.language.SymbolNames) {
		return p.language.SymbolNames[idx]
	}
	return ""
}

func (p *Parser) allowStringContentWhitespaceBroadRelex(candidate Symbol) bool {
	if p == nil || p.language == nil || int(candidate) >= len(p.language.SymbolNames) {
		return false
	}
	candidateName := p.language.SymbolNames[candidate]
	if candidateName != "b" && candidateName != "x" {
		return false
	}
	hasEscBlob, hasHexBlob := false, false
	for _, name := range p.language.SymbolNames {
		switch name {
		case "esc_blob":
			hasEscBlob = true
		case "hex_blob":
			hasHexBlob = true
		}
	}
	return hasEscBlob && hasHexBlob
}

// tryRelexCurrentStateDFA re-lexes from the current token start using the
// current parser state's DFA lex mode. This helps when a lookahead token was
// originally lexed under a pre-reduce state, but a reduce chain changes the
// parser state before the token is consumed.
func (p *Parser) tryRelexCurrentStateDFA(tok Token, parserState StateID, ts TokenSource) (Token, bool) {
	if p == nil || p.language == nil || ts == nil || tok.NoLookahead {
		return Token{}, false
	}
	dts, ok := ts.(*dfaTokenSource)
	if !ok || dts == nil || dts.lexer == nil || dts.language == nil {
		return Token{}, false
	}
	if tok.Symbol == 0 && tok.StartByte >= uint32(len(dts.lexer.source)) {
		return Token{}, false
	}
	if int(parserState) >= len(p.language.LexModes) {
		return Token{}, false
	}
	if dts.language.ExternalScanner != nil && dts.externalSymbolIndex(tok.Symbol) >= 0 &&
		p.language.LexModes[parserState].ExternalLexState != 0 &&
		!p.canRelexExternalTokenWithCurrentStateDFA(tok) {
		return Token{}, false
	}
	savedPos, savedRow, savedCol := dts.lexer.pos, dts.lexer.row, dts.lexer.col
	dts.lexer.pos = int(tok.StartByte)
	dts.lexer.row = tok.StartPoint.Row
	dts.lexer.col = tok.StartPoint.Column
	tok2, endPos, endRow, endCol := dts.scanPreferredTokenForState(parserState)
	if tok2.Symbol == 0 {
		dts.lexer.pos, dts.lexer.row, dts.lexer.col = savedPos, savedRow, savedCol
		return Token{}, false
	}
	if tok2.Symbol == tok.Symbol && tok2.StartByte == tok.StartByte && tok2.EndByte == tok.EndByte {
		dts.lexer.pos, dts.lexer.row, dts.lexer.col = savedPos, savedRow, savedCol
		return Token{}, false
	}
	actionIdx := p.lookupActionIndex(parserState, tok2.Symbol)
	if actionIdx == 0 || int(actionIdx) >= len(p.language.ParseActions) || len(p.language.ParseActions[actionIdx].Actions) == 0 {
		dts.lexer.pos, dts.lexer.row, dts.lexer.col = savedPos, savedRow, savedCol
		return Token{}, false
	}
	dts.lexer.pos, dts.lexer.row, dts.lexer.col = endPos, endRow, endCol
	return tok2, true
}

// peekZeroWidthExternalShiftForState asks an external scanner whether one
// JavaScript parser version needs a zero-width token before the already-lexed
// shared lookahead. JavaScript's external scanner has no serialized state, so
// the scanner and token-source state can be restored before return; callers
// apply the returned shift only to that version, then retry the real lookahead.
// This mirrors tree-sitter C's per-version lexing without replacing a real
// token that another live version may still consume. Keep this JavaScript-only
// until stateful scanners can transfer the probed post-scan checkpoint onto
// the shifted version instead of restoring it.
func (p *Parser) peekZeroWidthExternalShiftForState(tok Token, state StateID, ts TokenSource) (Token, ParseAction, bool) {
	if p == nil || p.language == nil || p.language.Name != "javascript" || tok.NoLookahead || tok.StartByte == tok.EndByte || p.language.ExternalScanner == nil {
		return Token{}, ParseAction{}, false
	}
	stateful, statefulOK := ts.(parserStateTokenSource)
	relexer, relexerOK := ts.(tokenSourceRelexer)
	dts := underlyingDFATokenSource(ts)
	if !statefulOK || !relexerOK || dts == nil || !relexer.CanRelexFromTokenStart(tok) {
		return Token{}, ParseAction{}, false
	}

	hasZeroWidthShiftCandidate := false
	for _, symbol := range p.language.ExternalSymbols {
		actionIndex := p.lookupActionIndex(state, symbol)
		if actionIndex == 0 || int(actionIndex) >= len(p.language.ParseActions) {
			continue
		}
		actions := p.language.ParseActions[actionIndex].Actions
		if len(actions) != 1 {
			continue
		}
		action := actions[0]
		if action.Type == ParseActionShift && !action.Extra && action.State != 0 && action.State != state {
			hasZeroWidthShiftCandidate = true
			break
		}
	}
	if !hasZeroWidthShiftCandidate {
		return Token{}, ParseAction{}, false
	}

	snapshot := dts.snapshotRelexState()
	savedState := dts.state
	savedGLRStates := dts.glrStates
	stateful.SetParserState(state)
	stateful.SetGLRStates(nil)
	next, relexed := relexer.RelexFromTokenStart(tok)
	snapshot.restore(dts)
	dts.state = savedState
	dts.glrStates = savedGLRStates

	if !relexed || !next.ExternalScannerToken || next.StartByte != tok.StartByte || next.EndByte != next.StartByte {
		return Token{}, ParseAction{}, false
	}
	actionIndex := p.lookupActionIndex(state, next.Symbol)
	if actionIndex == 0 || int(actionIndex) >= len(p.language.ParseActions) {
		return Token{}, ParseAction{}, false
	}
	actions := p.language.ParseActions[actionIndex].Actions
	if len(actions) != 1 {
		return Token{}, ParseAction{}, false
	}
	action := actions[0]
	if action.Type != ParseActionShift || action.Extra || action.State == 0 || action.State == state {
		return Token{}, ParseAction{}, false
	}
	return next, action, true
}

func (p *Parser) canRelexExternalTokenWithCurrentStateDFA(tok Token) bool {
	if p == nil || p.language == nil || int(tok.Symbol) >= len(p.language.SymbolNames) {
		return false
	}
	if p.language.Name != "kotlin" {
		return false
	}
	// Kotlin's LALR table reuses states between package headers and import
	// lists. The external scanner can therefore produce import-only tokens
	// before reductions reveal that the current branch needs an ordinary DFA
	// token such as "." or "import".
	switch p.language.SymbolNames[tok.Symbol] {
	case "_import_dot", "_import_list_delimiter":
		return true
	default:
		return false
	}
}

func (p *Parser) canFinalizeNoActionEOF(s *glrStack) bool {
	return p.canFinalizeNoActionEOFAt(s, 0, nil)
}

func (p *Parser) canFinalizeNoActionEOFAt(s *glrStack, expectedEOFByte uint32, source []byte) bool {
	if s == nil || s.dead {
		return false
	}
	top := s.top()
	if !stackEntryHasNode(top) {
		return true
	}

	tokenCount := uint32(0)
	if p != nil && p.language != nil {
		tokenCount = p.language.TokenCount
	}

	// Without an inferred root, the legacy behavior is still appropriate:
	// a single nonterminal at the top can serve as the final tree root.
	if p == nil || !p.hasRootSymbol {
		return p != nil && p.language != nil && uint32(stackEntryNodeSymbol(top)) >= tokenCount
	}

	nonExtraCount := 0
	onlyNonExtraSymbol := Symbol(0)
	onlyNonExtraEndByte := uint32(0)
	countEntry := func(e stackEntry) bool {
		if !stackEntryHasNode(e) || stackEntryNodeIsExtra(e) {
			return false
		}
		nonExtraCount++
		onlyNonExtraSymbol = stackEntryNodeSymbol(e)
		onlyNonExtraEndByte = stackEntryNodeEndByte(e)
		return nonExtraCount > 1
	}

	if len(s.entries) > 0 {
		for i := range s.entries {
			if countEntry(s.entries[i]) {
				return false
			}
		}
	} else {
		for n := s.gss.head; n != nil; n = n.prev {
			if countEntry(n.entry) {
				return false
			}
		}
	}

	if nonExtraCount == 0 {
		return true
	}
	if onlyNonExtraSymbol == errorSymbol {
		return false
	}
	if uint32(onlyNonExtraSymbol) < tokenCount {
		return false
	}
	if expectedEOFByte > onlyNonExtraEndByte {
		if int(expectedEOFByte) > len(source) || !isWhitespaceOnlySource(source[onlyNonExtraEndByte:expectedEOFByte]) {
			return false
		}
		if onlyNonExtraSymbol != p.rootSymbol {
			return false
		}
	}
	return true
}

func (p *Parser) tryInsertMissingSingleShift(source []byte, s *glrStack, tok Token, ts TokenSource, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, stateScratch *missingShiftStateScratch, trackChildErrors *bool) bool {
	if p == nil || p.language == nil || s == nil || s.dead || tok.NoLookahead {
		return false
	}
	if tok.Symbol == 0 || uint32(tok.Symbol) >= p.language.TokenCount {
		return false
	}

	state := s.top().state
	var (
		candidateSym Symbol
		candidateAct ParseAction
		candidateCnt int
	)
	p.forEachActionIndexInState(state, func(sym Symbol, idx uint16) bool {
		if sym == 0 || sym == tok.Symbol || uint32(sym) >= p.language.TokenCount {
			return true
		}
		if int(sym) >= len(p.language.SymbolMetadata) {
			return true
		}
		meta := p.language.SymbolMetadata[sym]
		if !meta.Visible || !meta.Named {
			return true
		}
		if int(idx) >= len(p.language.ParseActions) {
			return true
		}
		actions := p.language.ParseActions[idx].Actions
		if len(actions) != 1 {
			return true
		}
		act := actions[0]
		if act.Type != ParseActionShift || act.Extra {
			return true
		}
		if p.lookupActionIndex(act.State, tok.Symbol) == 0 {
			return true
		}
		candidateSym = sym
		candidateAct = act
		candidateCnt++
		return candidateCnt < 2
	})
	var reducePrefix []ParseAction
	if candidateCnt != 1 {
		localStateScratch := missingShiftStateScratch{}
		if stateScratch == nil {
			stateScratch = &localStateScratch
		}
		baseStates := p.collectStackStatesInto(s, stateScratch.baseStates[:0])
		stateScratch.baseStates = baseStates
		// Mirror tree-sitter's ts_parser__recover_with_missing: when the strict
		// single-shift heuristic above did not isolate a unique visible/named
		// candidate, fall back to the C runtime's exact algorithm. Scan terminal
		// symbols in ascending id order and pick the first whose shift target has
		// a reduce action on the real lookahead. C does this without any
		// visible/named filter and without a uniqueness requirement, taking the
		// lowest-id symbol that lets the parse make progress. This recovers
		// constructs such as Zig's empty container body `struct {}`, where the
		// grammar requires at least one `container_field` and the runtime inserts
		// a missing `_identifier` (an invisible terminal) at the `}`.
		//
		// The broadened scan is gated exactly like the C runtime: a candidate is
		// only accepted when, after inserting the missing terminal, performing all
		// possible reductions eventually reaches a state that can SHIFT the real
		// lookahead (ts_parser__do_all_potential_reductions returning
		// can_shift_lookahead_symbol). For grammars such as PHP's
		// `static function ...`, no missing terminal lets the offending `function`
		// lookahead be shifted, so this returns false and the parser falls through
		// to its established ERROR recovery; for Zig's `struct {}` the missing
		// `_identifier` reduces up to a state that can shift the closing `}`.
		if sym, act, ok := p.findRecoverWithMissingShiftAtStatesScratch(baseStates, state, tok.Symbol, &stateScratch.simStates); ok {
			candidateSym = sym
			candidateAct = act
		} else if reduces, sym, act, ok := p.findRecoverWithMissingAfterReductionsAtStates(baseStates, state, tok.Symbol, &stateScratch.reduceStates, &stateScratch.simStates); ok {
			// C's ts_parser__handle_error runs do_all_potential_reductions
			// BEFORE per-version missing insertion, so the missing terminal is
			// often only insertable after pending reductions land (jq: REDUCE
			// if_expression first, then `?` becomes shiftable and MISSING `?`
			// completes the pair). Apply the discovered reduce chain for real,
			// then insert the missing terminal at the reduced state.
			reducePrefix = reduces
			candidateSym = sym
			candidateAct = act
		} else {
			return false
		}
	}

	for _, reduceAct := range reducePrefix {
		p.applyAction(source, s, reduceAct, tok, new(bool), nodeCount, arena, entryScratch, gssScratch, nil, false, trackChildErrors)
		if p.rejectUndrainedPendingForkStacks(s) {
			return false
		}
		if s.dead {
			return false
		}
	}
	if externalTok, externalAct, ok := p.peekZeroWidthExternalShiftForState(tok, s.top().state, ts); ok {
		p.applyAction(source, s, externalAct, externalTok, new(bool), nodeCount, arena, entryScratch, gssScratch, nil, false, trackChildErrors)
		if p.rejectUndrainedPendingForkStacks(s) {
			return false
		}
		s.shifted = false
		return true
	}

	p.markCRecoveryCostCompetitionRelevant() // missing-token insertion makes costs relevant
	missingTok := Token{
		Symbol:     candidateSym,
		StartByte:  tok.StartByte,
		EndByte:    tok.StartByte,
		StartPoint: tok.StartPoint,
		EndPoint:   tok.StartPoint,
		Missing:    true,
	}
	if top := s.top(); stackEntryHasNode(top) && stackEntryNodeEndByte(top) <= tok.StartByte {
		missingTok.StartByte = stackEntryNodeEndByte(top)
		missingTok.EndByte = stackEntryNodeEndByte(top)
		missingTok.StartPoint = stackEntryNodeEndPoint(top)
		missingTok.EndPoint = stackEntryNodeEndPoint(top)
	}
	p.applyAction(source, s, candidateAct, missingTok, new(bool), nodeCount, arena, entryScratch, gssScratch, nil, false, trackChildErrors)
	if p.rejectUndrainedPendingForkStacks(s) {
		return false
	}
	s.shifted = false
	return true
}

func (p *Parser) tryRecoverPreviousShiftAsError(s *glrStack, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, trackChildErrors *bool) bool {
	if p == nil || p.language == nil || s == nil || s.dead || tok.NoLookahead || tok.Missing || tok.Symbol == 0 {
		return false
	}
	// Go has contextual identifier aliases after keywords such as package and
	// func; extra-chain grammars use synthetic states where replacing the prior
	// shift corrupts nested visible extras.
	if p.language.Name == "go" || p.hasExtraChainActions {
		return false
	}
	entries := s.ensureEntries(entryScratch)
	if len(entries) < 2 {
		return false
	}
	topIndex := len(entries) - 1
	topEntry := entries[topIndex]
	if !stackEntryHasNode(topEntry) || stackEntryNodeIsExtra(topEntry) || stackEntryNodeIsMissing(topEntry) {
		return false
	}
	topSymbol := stackEntryNodeSymbol(topEntry)
	if topSymbol == errorSymbol || uint32(topSymbol) >= p.language.TokenCount || stackEntryNodeChildCount(topEntry) != 0 {
		return false
	}
	topStartByte := stackEntryNodeStartByte(topEntry)
	topEndByte := stackEntryNodeEndByte(topEntry)
	if topEndByte == topStartByte || topEndByte > tok.StartByte {
		return false
	}
	prevState := entries[topIndex-1].state
	if prevState == topEntry.state {
		return false
	}
	actionIdx := p.lookupActionIndex(prevState, tok.Symbol)
	if actionIdx == 0 || int(actionIdx) >= len(p.language.ParseActions) || len(p.language.ParseActions[actionIdx].Actions) == 0 {
		return false
	}

	errNode := newLeafNodeInArena(arena, errorSymbol, true,
		topStartByte, topEndByte,
		stackEntryNodeStartPoint(topEntry), stackEntryNodeEndPoint(topEntry))
	errNode.setExtra(true)
	errNode.setHasError(true)
	errNode.parseState = prevState
	if perfCountersEnabled {
		perfRecordErrorNode()
	}
	if nodeCount != nil {
		*nodeCount = *nodeCount + 1
	}
	entries[topIndex] = newStackEntryNode(prevState, errNode)
	s.entries = entries
	s.gss = gssStack{}
	s.cacheEntries = true
	s.invalidateCEntryAgg()
	s.byteOffset = stackByteOffset(entries)
	s.shifted = false
	if s.recoverabilityKnown && !s.mayRecover && p.stateCanRecover(prevState) {
		s.mayRecover = true
	}
	if trackChildErrors != nil {
		*trackChildErrors = true
	}
	return true
}

func (p *Parser) rejectUndrainedPendingForkStacks(s *glrStack) bool {
	if p == nil || !faithfulCapOneMergeEnabled(p.mergeScratch) || len(p.pendingForkStacks) == 0 {
		return false
	}
	workCountRecordPendingTransition(p, &p.pendingForkStacks[0], len(p.pendingForkStacks), 0, workCountConvergenceOutcomePendingDiscarded, "undrained post-reduce candidates rejected")
	p.pendingForkStacks = p.pendingForkStacks[:0]
	if s != nil {
		s.dead = true
	}
	return true
}

// findRecoverWithMissingAfterReductions extends findRecoverWithMissingShift
// with C's handle_error ordering: ts_parser__do_all_potential_reductions runs
// BEFORE missing-token insertion, so a missing terminal that only becomes
// shiftable after pending reductions (jq's `?` after `if … end`) is still
// found. It chases the chain of reduce actions available for ANY lookahead at
// the current state (bounded), re-running the missing-shift scan after each
// step, and returns the reduce chain to apply plus the discovered candidate.
func (p *Parser) findRecoverWithMissingAfterReductions(s *glrStack, state StateID, lookahead Symbol) ([]ParseAction, Symbol, ParseAction, bool) {
	if p == nil || p.language == nil || s == nil {
		return nil, 0, ParseAction{}, false
	}
	baseStates := p.collectStackStates(s)
	return p.findRecoverWithMissingAfterReductionsAtStates(baseStates, state, lookahead, nil, nil)
}

func (p *Parser) findRecoverWithMissingAfterReductionsAtStates(baseStates []StateID, state StateID, lookahead Symbol, reduceScratch, simScratch *[]StateID) ([]ParseAction, Symbol, ParseAction, bool) {
	if len(baseStates) == 0 {
		return nil, 0, ParseAction{}, false
	}
	// C's ts_parser__handle_error performs exactly ONE round of
	// do_all_potential_reductions (one reduce per forked version) before
	// missing-token insertion — deeper chains find insertions C never takes
	// (PHP's `static function …` recovery is pinned on NOT inserting).
	const maxReduceChase = 1
	var states []StateID
	if reduceScratch != nil {
		states = resizeMissingShiftStateScratch((*reduceScratch)[:0], len(baseStates))
		defer func() {
			*reduceScratch = states[:0]
		}()
	} else {
		states = make([]StateID, len(baseStates))
	}
	copy(states, baseStates)
	var reduces []ParseAction
	cur := state
	for step := 0; step < maxReduceChase; step++ {
		reduceAct, ok := p.anyLookaheadReduceAction(cur)
		if !ok {
			return nil, 0, ParseAction{}, false
		}
		childCount := int(reduceAct.ChildCount)
		if childCount <= 0 || childCount >= len(states) {
			return nil, 0, ParseAction{}, false
		}
		states = states[:len(states)-childCount]
		gotoState := p.lookupGoto(states[len(states)-1], reduceAct.Symbol)
		if gotoState == 0 {
			return nil, 0, ParseAction{}, false
		}
		states = append(states, gotoState)
		reduces = append(reduces, reduceAct)
		cur = gotoState
		if sym, act, ok := p.findRecoverWithMissingShiftAtStatesScratch(states, cur, lookahead, simScratch); ok {
			return reduces, sym, act, true
		}
	}
	return nil, 0, ParseAction{}, false
}

// anyLookaheadReduceAction returns a deterministic single reduce action
// available in the state's table row regardless of lookahead — the first
// (lowest symbol id) entry whose sole action is a reduce. Conflicted rows are
// skipped so the chase never guesses between competing reductions.
func (p *Parser) anyLookaheadReduceAction(state StateID) (ParseAction, bool) {
	var found ParseAction
	var have bool
	p.forEachActionIndexInState(state, func(sym Symbol, idx uint16) bool {
		if have || int(idx) >= len(p.language.ParseActions) {
			return !have
		}
		actions := p.language.ParseActions[idx].Actions
		if len(actions) != 1 {
			return true
		}
		act := actions[0]
		if act.Type != ParseActionReduce || act.ChildCount <= 0 {
			return true
		}
		found = act
		have = true
		return false
	})
	return found, have
}

// findRecoverWithMissingShiftAtStates is findRecoverWithMissingShift's scan
// over an explicit (simulated) state chain instead of the live stack.
func (p *Parser) findRecoverWithMissingShiftAtStates(baseStates []StateID, state StateID, lookahead Symbol) (Symbol, ParseAction, bool) {
	return p.findRecoverWithMissingShiftAtStatesScratch(baseStates, state, lookahead, nil)
}

func (p *Parser) findRecoverWithMissingShiftAtStatesScratch(baseStates []StateID, state StateID, lookahead Symbol, simScratch *[]StateID) (Symbol, ParseAction, bool) {
	tokenCount := Symbol(p.language.TokenCount)
	var sim []StateID
	if simScratch != nil {
		sim = (*simScratch)[:0]
		defer func() {
			*simScratch = sim[:0]
		}()
	}
	for ms := Symbol(1); ms < tokenCount; ms++ {
		if ms == lookahead {
			continue
		}
		idx := p.lookupActionIndex(state, ms)
		if idx == 0 || int(idx) >= len(p.language.ParseActions) {
			continue
		}
		actions := p.language.ParseActions[idx].Actions
		if len(actions) == 0 {
			continue
		}
		act := actions[len(actions)-1]
		if act.Type != ParseActionShift || act.Extra {
			continue
		}
		nextState := act.State
		if nextState == 0 || nextState == state {
			continue
		}
		if !p.stateHasLeadingReduceAction(nextState, lookahead) {
			continue
		}
		sim = append(sim[:0], baseStates...)
		sim = append(sim, nextState)
		if p.canShiftAfterReductions(sim, lookahead) {
			return ms, act, true
		}
	}
	return 0, ParseAction{}, false
}

// findRecoverWithMissingShift mirrors tree-sitter's ts_parser__recover_with_missing
// (lib/src/parser.c). It walks terminal symbols in ascending id order; for each it
// computes the shift target (ts_language_next_state) and checks that the target
// state has a leading reduce action on the real lookahead (ts_language_has_reduce_action).
// The candidate is then confirmed by simulating ts_parser__do_all_potential_reductions:
// the missing terminal is only accepted when, after applying every available
// reduction, some reachable state can SHIFT the real lookahead. The first symbol
// that passes wins, exactly matching the C runtime's lowest-id selection.
func (p *Parser) findRecoverWithMissingShift(s *glrStack, state StateID, lookahead Symbol) (Symbol, ParseAction, bool) {
	if p == nil || p.language == nil || s == nil {
		return 0, ParseAction{}, false
	}
	// Materialize the current stack's state chain once; the per-candidate
	// reduction simulation works on a scratch copy so the real stack is never
	// mutated.
	baseStates := p.collectStackStates(s)
	if len(baseStates) == 0 {
		return 0, ParseAction{}, false
	}
	return p.findRecoverWithMissingShiftAtStates(baseStates, state, lookahead)
}

// collectStackStates returns the chain of parser states for s, bottom-to-top.
func (p *Parser) collectStackStates(s *glrStack) []StateID {
	return p.collectStackStatesInto(s, nil)
}

func (p *Parser) collectStackStatesInto(s *glrStack, dst []StateID) []StateID {
	if s == nil {
		return nil
	}
	if s.gss.head == nil && len(s.entries) > 0 {
		states := resizeMissingShiftStateScratch(dst, len(s.entries))
		for i := range s.entries {
			states[i] = s.entries[i].state
		}
		return states
	}
	n := s.gss.len()
	if n == 0 {
		return nil
	}
	states := resizeMissingShiftStateScratch(dst, n)
	i := n - 1
	for node := s.gss.head; node != nil && i >= 0; node = node.prev {
		states[i] = node.entry.state
		i--
	}
	if i >= 0 {
		panic("collectStackStatesInto: corrupt GSS depth metadata")
	}
	return states
}

func resizeMissingShiftStateScratch(dst []StateID, n int) []StateID {
	if n == 0 {
		return dst[:0]
	}
	if cap(dst) < n {
		capacity := cap(dst) * 2
		if capacity < 64 {
			capacity = 64
		}
		if capacity < n {
			capacity = n
		}
		dst = make([]StateID, n, capacity)
		return dst
	}
	return dst[:n]
}

// stateHasLeadingReduceAction mirrors ts_language_has_reduce_action: the first
// action for (state, sym) must be a reduce.
func (p *Parser) stateHasLeadingReduceAction(state StateID, sym Symbol) bool {
	entry := p.lookupAction(state, sym)
	if entry == nil || len(entry.Actions) == 0 {
		return false
	}
	return entry.Actions[0].Type == ParseActionReduce
}

// canShiftAfterReductions simulates ts_parser__do_all_potential_reductions for a
// single lookahead symbol over a linear copy of the state stack. It repeatedly
// applies reduce actions available for the lookahead (popping child states and
// taking the GOTO of the reduced nonterminal) and returns true as soon as a
// reachable state can shift (or recover on) the lookahead. The state slice is
// mutated in place; callers pass a scratch copy.
func (p *Parser) canShiftAfterReductions(states []StateID, lookahead Symbol) bool {
	const maxSimSteps = 1024
	for step := 0; step < maxSimSteps; step++ {
		if len(states) == 0 {
			return false
		}
		top := states[len(states)-1]
		entry := p.lookupAction(top, lookahead)
		if entry == nil || len(entry.Actions) == 0 {
			return false
		}
		var reduce ParseAction
		haveReduce := false
		for i := range entry.Actions {
			act := entry.Actions[i]
			switch act.Type {
			case ParseActionShift, ParseActionRecover:
				if !act.Extra && !act.Repetition {
					return true
				}
			case ParseActionReduce:
				if act.ChildCount > 0 && !haveReduce {
					reduce = act
					haveReduce = true
				}
			}
		}
		if !haveReduce {
			return false
		}
		// Apply the reduction: pop child_count states, then GOTO on the reduced
		// nonterminal from the new top state.
		childCount := int(reduce.ChildCount)
		if childCount > len(states) {
			return false
		}
		states = states[:len(states)-childCount]
		if len(states) == 0 {
			return false
		}
		gotoState := p.lookupGoto(states[len(states)-1], reduce.Symbol)
		if gotoState == 0 {
			return false
		}
		states = append(states, gotoState)
	}
	return false
}

func nodesFromStack(stack glrStack) []*Node {
	if len(stack.entries) > 0 {
		nodes := make([]*Node, 0, len(stack.entries))
		for _, entry := range stack.entries {
			if node := stackEntryNode(entry); node != nil {
				nodes = append(nodes, node)
			}
		}
		return nodes
	}
	return nodesFromGSS(stack.gss)
}

func trimTrailingRecoveryEOFErrors(nodes []*Node, eofByte uint32) []*Node {
	end := len(nodes)
	for end > 0 {
		n := nodes[end-1]
		if n == nil || n.symbol != errorSymbol || n.startByte != eofByte || n.endByte != eofByte {
			break
		}
		if len(n.children) == 0 {
			end--
			continue
		}
		if len(n.children) != 1 {
			break
		}
		child := n.children[0]
		if child == nil || child.symbol != errorSymbol || child.startByte != eofByte || child.endByte != eofByte {
			break
		}
		end--
	}
	return nodes[:end]
}

func trimRecoveryWhitespaceTail(n *Node, source []byte) {
	if n == nil {
		return
	}
	for _, child := range n.children {
		trimRecoveryWhitespaceTail(child, source)
	}
	if len(n.children) == 0 {
		return
	}
	last := n.children[len(n.children)-1]
	if last == nil || n.endByte <= last.endByte || int(n.endByte) > len(source) || int(last.endByte) > len(source) {
		return
	}
	if len(bytes.TrimSpace(source[last.endByte:n.endByte])) != 0 {
		return
	}
	n.endByte = last.endByte
	n.endPoint = last.endPoint
}

func findVisibleSymbolByName(lang *Language, name string, named bool) (Symbol, bool) {
	if lang == nil {
		return 0, false
	}
	return lang.visibleSymbolByNameAndNamed(name, named)
}

func normalizeSQLRecoveredMissingNull(root *Node, arena *nodeArena, lang *Language) {
	if root == nil || arena == nil || lang == nil || lang.Name != "sql" {
		return
	}
	nullParentSym, ok := findVisibleSymbolByName(lang, "NULL", true)
	if !ok {
		return
	}
	nullLeafSym, ok := findVisibleSymbolByName(lang, "NULL", false)
	if !ok {
		return
	}
	numberSym, ok := findVisibleSymbolByName(lang, "number", true)
	if !ok {
		return
	}
	var walk func(*Node)
	walk = func(parent *Node) {
		if parent == nil {
			return
		}
		for i, child := range parent.children {
			if child == nil {
				continue
			}
			walk(child)
			if parent.Type(lang) != "select_clause_body" || !child.isMissing() || child.symbol != numberSym {
				continue
			}
			leaf := newLeafNodeInArena(arena, nullLeafSym, false, child.startByte, child.endByte, child.startPoint, child.endPoint)
			leaf.setMissing(true)
			leaf.setHasError(true)
			repl := newParentNodeInArena(arena, nullParentSym, true, []*Node{leaf}, nil, 0)
			repl.setHasError(true)
			parent.children[i] = repl
		}
	}
	walk(root)
}

func (p *Parser) tryAdvanceEOFOnSingleStack(s *glrStack, tok Token, expectedEOFByte uint32, source []byte, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry) bool {
	if p == nil || p.language == nil || s == nil || s.dead || s.depth() == 0 {
		return false
	}
	parseActions := p.language.ParseActions
	anyReduced := false
	const maxEOFRecoverySteps = 256
	for steps := 0; steps < maxEOFRecoverySteps; steps++ {
		if s.accepted {
			return true
		}
		actionIdx := p.lookupActionIndex(s.top().state, 0)
		if actionIdx == 0 || int(actionIdx) >= len(parseActions) {
			return p.canFinalizeNoActionEOFAt(s, expectedEOFByte, source)
		}
		actions := parseActions[actionIdx].Actions
		if len(actions) != 1 {
			return false
		}
		act := actions[0]
		if semanticPhaseTraceActive() {
			semanticPhaseTraceRecordActionCell(p, s, s.top().state, tok, actions) // semantic-phase-assembly: EOF-prefix action-cell seam
		}
		switch act.Type {
		case ParseActionReduce:
			if semanticPhaseTraceActive() {
				semanticPhaseTraceRecordActionExecution(p, s, tok, act, 0, "eof-prefix-reduce", false) // semantic-phase-assembly: EOF-prefix action-execution seam
			}
			p.applyAction(nil, s, act, tok, &anyReduced, nodeCount, arena, entryScratch, gssScratch, tmpEntries, false, nil)
			if p.rejectUndrainedPendingForkStacks(s) {
				return false
			}
			if s.dead {
				return false
			}
		case ParseActionAccept:
			if semanticPhaseTraceActive() {
				semanticPhaseTraceRecordActionExecution(p, s, tok, act, 0, "eof-prefix-accept", false)
			}
			s.accepted = true
			return true
		default:
			return false
		}
	}
	return false
}

func (p *Parser) tryInsertMissingSingleShiftAtEOF(s *glrStack, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch) bool {
	if p == nil || p.language == nil || s == nil || s.dead || tok.NoLookahead || tok.Symbol != 0 || tok.StartByte != tok.EndByte {
		return false
	}

	state := s.top().state
	var (
		candidateSym Symbol
		candidateAct ParseAction
		candidateCnt int
	)
	p.forEachActionIndexInState(state, func(sym Symbol, idx uint16) bool {
		if sym == 0 || uint32(sym) >= p.language.TokenCount {
			return true
		}
		if int(sym) >= len(p.language.SymbolMetadata) {
			return true
		}
		meta := p.language.SymbolMetadata[sym]
		if !meta.Visible || !meta.Named {
			return true
		}
		if int(idx) >= len(p.language.ParseActions) {
			return true
		}
		actions := p.language.ParseActions[idx].Actions
		if len(actions) != 1 {
			return true
		}
		act := actions[0]
		if act.Type != ParseActionShift || act.Extra {
			return true
		}
		if p.lookupActionIndex(act.State, 0) == 0 {
			return true
		}
		candidateSym = sym
		candidateAct = act
		candidateCnt++
		return candidateCnt < 2
	})
	if candidateCnt != 1 {
		return false
	}

	p.markCRecoveryCostCompetitionRelevant() // missing-token insertion makes costs relevant
	missingTok := Token{
		Symbol:     candidateSym,
		StartByte:  tok.StartByte,
		EndByte:    tok.StartByte,
		StartPoint: tok.StartPoint,
		EndPoint:   tok.StartPoint,
		Missing:    true,
	}
	p.applyAction(nil, s, candidateAct, missingTok, new(bool), nodeCount, arena, entryScratch, gssScratch, nil, false, nil)
	if p.rejectUndrainedPendingForkStacks(s) {
		return false
	}
	s.shifted = false
	return true
}

func (p *Parser) tryRecoverTrailingEOFSuffix(s *glrStack, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry, source []byte) ([]*Node, bool) {
	if p == nil || s == nil || s.dead || tok.Symbol != 0 || tok.StartByte != tok.EndByte {
		return nil, false
	}
	entries := s.entries
	borrowed := false
	if entries == nil {
		tmp := []stackEntry(nil)
		if tmpEntries != nil {
			tmp = *tmpEntries
		}
		entries, borrowed = s.entriesForRead(tmp)
	}
	if borrowed && tmpEntries != nil {
		defer func() {
			*tmpEntries = entries[:0]
		}()
	}
	if len(entries) == 0 {
		return nil, false
	}

	for firstDrop := len(entries) - 1; firstDrop >= 0; firstDrop-- {
		node := stackEntryNode(entries[firstDrop])
		if node == nil || node.isExtra() {
			continue
		}
		if firstDrop == 0 {
			continue
		}
		for _, cut := range trailingEOFSuffixCuts(entries, firstDrop, node) {
			prefix := s.cloneWithScratch(gssScratch)
			if !prefix.truncate(cut) {
				continue
			}
			prefixEOF := eofTokenForTrailingCut(tok, entries, cut, node)
			insertedMissing, advanced := p.advanceTrailingEOFPrefix(&prefix, prefixEOF, prefixEOF.EndByte, source, nodeCount, arena, entryScratch, gssScratch, tmpEntries)
			if !advanced {
				continue
			}

			nodes := p.trailingEOFNodesFromPrefix(prefix)
			if insertedMissing || cut > firstDrop {
				nodes = trimTrailingRecoveryEOFErrors(nodes, tok.StartByte)
				for _, n := range nodes {
					trimRecoveryWhitespaceTail(n, source)
				}
			}
			nodes, recovered := p.appendTrailingEOFRecoveryNodes(nodes, entries, cut, tok, arena, nodeCount)
			if recovered || insertedMissing || cut > firstDrop {
				return nodes, true
			}
		}
	}
	return nil, false
}

func trailingEOFSuffixCuts(entries []stackEntry, firstDrop int, node *Node) []int {
	cuts := []int{firstDrop}
	if node == nil || node.isNamed() {
		return cuts
	}
	cut := firstDrop + 1
	for cut < len(entries) {
		trailing := stackEntryNode(entries[cut])
		if trailing == nil || !trailing.isExtra() {
			break
		}
		cut++
	}
	if cut > firstDrop && cut <= len(entries) {
		cuts = append(cuts, cut)
	}
	return cuts
}

func eofTokenForTrailingCut(tok Token, entries []stackEntry, cut int, fallback *Node) Token {
	prefixEOF := tok
	if cut > 0 {
		if last := stackEntryNode(entries[cut-1]); last != nil {
			prefixEOF.StartByte = last.endByte
			prefixEOF.EndByte = last.endByte
			prefixEOF.StartPoint = last.endPoint
			prefixEOF.EndPoint = last.endPoint
			return prefixEOF
		}
	}
	prefixEOF.StartByte = fallback.startByte
	prefixEOF.EndByte = fallback.startByte
	prefixEOF.StartPoint = fallback.startPoint
	prefixEOF.EndPoint = fallback.startPoint
	return prefixEOF
}

func (p *Parser) advanceTrailingEOFPrefix(prefix *glrStack, prefixEOF Token, expectedEOFByte uint32, source []byte, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, tmpEntries *[]stackEntry) (bool, bool) {
	if p.tryAdvanceEOFOnSingleStack(prefix, prefixEOF, expectedEOFByte, source, nodeCount, arena, entryScratch, gssScratch, tmpEntries) {
		return false, true
	}
	if !p.tryInsertMissingSingleShiftAtEOF(prefix, prefixEOF, nodeCount, arena, entryScratch, gssScratch) {
		return false, false
	}
	if !p.tryAdvanceEOFOnSingleStack(prefix, prefixEOF, expectedEOFByte, source, nodeCount, arena, entryScratch, gssScratch, tmpEntries) {
		return false, false
	}
	return true, true
}

func (p *Parser) trailingEOFNodesFromPrefix(prefix glrStack) []*Node {
	nodes := nodesFromStack(prefix)
	if p.hasRootSymbol && len(nodes) == 1 && nodes[0] != nil && nodes[0].symbol == p.rootSymbol {
		return append([]*Node(nil), nodes[0].children...)
	}
	return nodes
}

// appendTrailingEOFRecoveryNodes wraps the first dropped non-extra trailing
// stack payload in an ERROR node during trailing-EOF suffix recovery. This is
// issue #110's actual construction path: on a fresh parse `trailing` can be a
// TRANSIENT reduce parent (a transientParentScratch slab node), whose .parent
// field doubles as the result-time materializer's {nil: unvisited, self:
// in-progress, other: arena clone} sentinel. The old eager-link-wiring
// constructor (newParentNodeInArena → populateParentNode → setNodeParentLink)
// corrupted that sentinel, so materializeTransientParentNodes then linked this
// ERROR wrapper under itself. Route through newRecoveryParentNodeInArena, which
// skips eager wiring while transient parents are active (finalizeResultRoot wires
// links from the root anyway) and keeps the eager-wiring constructor for
// incremental parses. See parser_recover_c.go and
// parser_recover_cycle_internal_test.go for the mechanism and pins.
func (p *Parser) appendTrailingEOFRecoveryNodes(nodes []*Node, entries []stackEntry, cut int, tok Token, arena *nodeArena, nodeCount *int) ([]*Node, bool) {
	recovered := false
	for i := cut; i < len(entries); i++ {
		trailing := stackEntryNode(entries[i])
		if trailing == nil {
			continue
		}
		if trailing.symbol == errorSymbol && trailing.startByte == tok.StartByte && trailing.endByte == tok.EndByte {
			continue
		}
		if !recovered && !trailing.isExtra() {
			errNode := p.newRecoveryParentNodeInArena(arena, errorSymbol, true, []*Node{trailing}, 0)
			errNode.setHasError(true)
			errNode.setExtra(true)
			nodes = append(nodes, errNode)
			recovered = true
			if nodeCount != nil {
				*nodeCount = *nodeCount + 1
			}
			continue
		}
		nodes = append(nodes, trailing)
	}
	return nodes, recovered
}

func (p *Parser) parseIncrementalInternal(source []byte, oldTree *Tree, ts TokenSource, timing *incrementalParseTiming) *Tree {
	return p.parseIncrementalInternalWithMergePerKeyOverride(source, oldTree, ts, timing, 0)
}

// incrementalTokenSourceFreshFullParse performs a full fresh parse over the
// provided token source with the incremental origin's retry widening. It is the
// shared fallback for an incremental reparse that must not reuse the old tree:
// either the token source does not support subtree reuse, or the old tree
// disables reuse (forestFastPath / compact-materialized). It never touches
// oldTree, so no replayed/abstained state can leak into the result.
func (p *Parser) incrementalTokenSourceFreshFullParse(source []byte, ts TokenSource, timing *incrementalParseTiming) *Tree {
	deterministicExternalConflicts := fullParseUsesDeterministicExternalConflicts(p.language)
	initialMaxStacks := fullParseInitialMaxStacks(p.language, p.maxConflictWidth)
	workCountSetNextParseAttempt("initial_full", "incremental_token_source_fallback_full_parse")
	tree := p.parseInternal(source, ts, nil, nil, arenaClassFull, timing, initialMaxStacks, 0, 0, deterministicExternalConflicts)
	tree = p.retryFullParseWithTokenSourceForOrigin(source, ts, initialMaxStacks, deterministicExternalConflicts, tree, fullParseRetryOriginIncremental)
	if shouldRepeatExternalScannerFullParse(p.language, tree) {
		tree = p.retryFullParseWithTokenSourceForOrigin(source, ts, initialMaxStacks, deterministicExternalConflicts, tree, fullParseRetryOriginIncremental)
	}
	return tree
}

func (p *Parser) parseIncrementalInternalWithMergePerKeyOverride(source []byte, oldTree *Tree, ts TokenSource, timing *incrementalParseTiming, maxMergePerKeyOverride int) *Tree {
	// Fast path: unchanged source and no recorded edits.
	if canReuseUnchangedTree(source, oldTree, p.language) {
		return oldTree
	}
	if tree, ok := p.tryTokenInvariantLeafEdit(source, oldTree, ts, timing); ok {
		return tree
	}

	// One reuse bar for EVERY incremental entry (Phase-3 Lane 3 review). The DFA
	// entry (parseIncrementalChanged) already routes a reuse-disabled old tree to
	// a full fresh parse before reaching here, but the token-source entries
	// (parseIncrementalWithTokenSourceChanged and its Profiled twin) call in
	// directly. A reuse-disabled old tree -- forestFastPath, or a compact tree
	// whose replay/scanner proof is incomplete -- must never feed the reuseCursor
	// subtree splice below, which trusts old-tree parser states. The only reuse
	// it may take is the independently re-authenticated token-invariant leaf edit
	// attempted just above; once that declines, force a full fresh parse over the
	// provided token source.
	if oldTreeDisablesIncrementalReuse(oldTree) {
		if timing != nil {
			timing.reuseUnsupported = true
			timing.reuseUnsupportedReason = incrementalReuseUnsupportedReasonForTree(oldTree)
			if oldTree != nil && oldTree.compactMaterialized {
				if reason := incrementalReuseUnavailableReason(ts); reason != "" {
					timing.reuseUnsupportedReason = reason
				}
			}
		}
		return p.incrementalTokenSourceFreshFullParse(source, ts, timing)
	}
	if tokenSourceUsesLanguageExternalScanner(ts) && oldTree != nil && oldTree.RootNode() != nil && oldTree.RootNode().HasError() &&
		!languageSupportsIncrementalReuseFromErrorTree(p.language) {
		if timing != nil {
			timing.reuseUnsupported = true
			timing.reuseUnsupportedReason = "external_scanner_error_tree_unsupported"
		}
		return p.incrementalTokenSourceFreshFullParse(source, ts, timing)
	}

	// Subtree reuse is safe for DFA token sources without external scanners
	// and for custom token sources that explicitly opt in.
	if !tokenSourceSupportsIncrementalReuse(ts) {
		if timing != nil {
			timing.reuseUnsupported = true
			timing.reuseUnsupportedReason = incrementalReuseUnavailableReason(ts)
		}
		// When subtree reuse is unavailable, incremental reparses should behave
		// like ordinary full parses, including retry widening. This keeps
		// conservative fallback paths for external-scanner languages on the same
		// correctness footing as Parse.
		return p.incrementalTokenSourceFreshFullParse(source, ts, timing)
	}
	if oldTree != nil {
		oldTree.ensureParentLinks()
	}
	if oldTree != nil {
		if languageUsesExternalScannerCheckpoints(p.language) {
			oldTree.ensureExternalScannerCheckpoints()
		}
	}

	p.reuseMu.Lock()
	defer p.reuseMu.Unlock()

	var reuse *reuseCursor
	p.reuseCursor.disableLeadingSplice = p.disableLeadingRunSplice
	if timing != nil {
		reuseStart := time.Now()
		reuse = p.reuseCursor.reset(oldTree, source, &p.reuseScratch)
		timing.reuseNanos += time.Since(reuseStart).Nanoseconds()
	} else {
		reuse = p.reuseCursor.reset(oldTree, source, &p.reuseScratch)
	}
	arenaClass := incrementalArenaClassForSource(source)
	tree := p.parseInternal(source, ts, reuse, oldTree, arenaClass, timing, 0, 0, maxMergePerKeyOverride, false)
	if tree != nil && reuse != nil {
		tree.parseRuntime.IncrementalOldTreeReuseRoute = true
		if timing != nil {
			timing.oldTreeReuseRoute = true
		}
	}
	if reuse != nil {
		if timing != nil {
			timing.reuseRejectDirty += reuse.rejectDirty
			timing.reuseRejectAncestorDirtyBeforeEdit += reuse.rejectAncestorDirtyBeforeEdit
			timing.reuseRejectHasError += reuse.rejectHasError
			timing.reuseRejectInvalidSpan += reuse.rejectInvalidSpan
			timing.reuseRejectOutOfBounds += reuse.rejectOutOfBounds
			timing.reuseRejectRootNonLeafChanged += reuse.rejectRootNonLeafChanged
			timing.reuseObservedPreGotoStateMismatch += reuse.observedPreGotoStateMismatch
			timing.reuseRejectLargeNonLeaf += reuse.rejectLargeNonLeaf
			timing.reuseRejectStaleNonLeafBoundary += reuse.rejectStaleNonLeafBoundary
			timing.reuseRejectFragileNonLeaf += reuse.rejectFragileNonLeaf
			timing.reuseRejectScannerUnquiescent += reuse.rejectScannerUnquiescent
		}
		if timing != nil {
			reuseStart := time.Now()
			reuse.commitScratch(&p.reuseScratch)
			timing.reuseNanos += time.Since(reuseStart).Nanoseconds()
		} else {
			reuse.commitScratch(&p.reuseScratch)
		}
	}
	return tree
}

// Dart's external scanner is stateless enough for subtree reuse, but keep a
// bounded source-size guard so very large generated files fall back safely.
const dartIncrementalReuseMaxSourceBytes = 256 * 1024

func tokenSourceSupportsIncrementalReuse(ts TokenSource) bool {
	if ts == nil {
		return false
	}
	if dts, ok := ts.(*dfaTokenSource); ok {
		return dfaTokenSourceSupportsIncrementalReuse(dts)
	}
	if reusable, ok := ts.(IncrementalReuseTokenSource); ok {
		return reusable.SupportsIncrementalReuse()
	}
	return false
}

func dfaTokenSourceSupportsIncrementalReuse(dts *dfaTokenSource) bool {
	if dts == nil || !languageSupportsIncrementalReuse(dts.language) {
		return false
	}
	return !dfaTokenSourceIncrementalReuseBlockedBySource(dts)
}

func dfaTokenSourceIncrementalReuseBlockedBySource(dts *dfaTokenSource) bool {
	return dts != nil &&
		dts.language != nil &&
		dts.language.Name == "dart" &&
		dts.lexer != nil &&
		len(dts.lexer.source) > dartIncrementalReuseMaxSourceBytes
}

func languageSupportsIncrementalReuse(lang *Language) bool {
	if lang == nil {
		return false
	}
	if lang.ExternalScanner == nil {
		return true
	}
	if reusable, ok := lang.ExternalScanner.(IncrementalReuseExternalScanner); ok {
		return reusable.SupportsIncrementalReuse()
	}
	return false
}

func tokenSourceUsesLanguageExternalScanner(ts TokenSource) bool {
	switch source := ts.(type) {
	case *dfaTokenSource:
		return source != nil && source.hasExternalScanner
	case *includedRangeTokenSource:
		return source != nil && tokenSourceUsesLanguageExternalScanner(source.base)
	default:
		return false
	}
}

func languageSupportsIncrementalReuseFromErrorTree(lang *Language) bool {
	if lang == nil || lang.ExternalScanner == nil {
		return true
	}
	if reusable, ok := lang.ExternalScanner.(ErrorTreeIncrementalReuseExternalScanner); ok {
		return reusable.SupportsIncrementalReuseFromErrorTree()
	}
	return true
}

func incrementalReuseUnavailableReason(ts TokenSource) string {
	if ts == nil {
		return "token_source_nil"
	}
	if reasoner, ok := ts.(incrementalReuseUnsupportedReasoner); ok {
		if reason := reasoner.IncrementalReuseUnsupportedReason(); reason != "" {
			return reason
		}
	}
	if dts, ok := ts.(*dfaTokenSource); ok {
		if dts.language == nil {
			return "dfa_token_source_no_language"
		}
		if dfaTokenSourceIncrementalReuseBlockedBySource(dts) {
			return "dart_large_external_scanner_unsupported"
		}
		if languageSupportsIncrementalReuse(dts.language) {
			return ""
		}
		if dts.language.ExternalScanner != nil {
			return "external_scanner_unsupported"
		}
		return "dfa_token_source_no_reuse"
	}
	if _, ok := ts.(IncrementalReuseTokenSource); ok {
		return ""
	}
	return "token_source_no_incremental_reuse"
}

func incrementalArenaClassForSource(source []byte) arenaClass {
	arenaClass := arenaClassIncremental
	// Very large files can outgrow incremental defaults and trigger repeated
	// fallback allocations; use full-parse slab sizing only beyond this point.
	const incrementalUseFullArenaThreshold = 1 * 1024 * 1024
	if len(source) >= incrementalUseFullArenaThreshold {
		arenaClass = arenaClassFull
	}
	return arenaClass
}

func (p *Parser) clearCurrentExternalTokenCheckpoint() {
	if p == nil {
		return
	}
	p.currentExternalTokenCheckpoint = externalScannerCheckpoint{}
	p.currentExternalTokenCheckpointStart = 0
	p.currentExternalTokenCheckpointEnd = 0
	p.currentExternalTokenCheckpointValid = false
}

func (p *Parser) updateCurrentExternalTokenCheckpoint(ts TokenSource, tok Token) {
	if p == nil {
		return
	}
	p.clearCurrentExternalTokenCheckpoint()
	if p.noTreeBenchmarkOnly && !p.noTreeCheckpointBenchmarkOnly {
		return
	}
	cp, startByte, endByte, ok := currentExternalScannerCheckpoint(ts)
	if !ok || tok.Missing || tok.NoLookahead || tok.Symbol == 0 {
		return
	}
	if startByte != tok.StartByte || endByte != tok.EndByte {
		return
	}
	p.currentExternalTokenCheckpoint = cp
	p.currentExternalTokenCheckpointStart = startByte
	p.currentExternalTokenCheckpointEnd = endByte
	p.currentExternalTokenCheckpointValid = true
}

func (p *Parser) recordCurrentExternalLeafCheckpoint(node *Node, tok Token) {
	if p == nil || node == nil || !p.currentExternalTokenCheckpointValid {
		return
	}
	if p.noTreeBenchmarkOnly && !p.noTreeCheckpointBenchmarkOnly {
		return
	}
	if tok.Missing || tok.NoLookahead || tok.Symbol == 0 {
		return
	}
	if node.ownerArena == nil {
		return
	}
	if node.StartByte() != p.currentExternalTokenCheckpointStart || node.EndByte() != p.currentExternalTokenCheckpointEnd {
		return
	}
	arena := node.ownerArena
	if p.skipInvisibleFullLeafCheckpoints && !p.isVisibleSymbol(tok.Symbol) {
		return
	}
	if !arena.recordExternalScannerLeafCheckpoint(node, p.currentExternalTokenCheckpoint.start, p.currentExternalTokenCheckpoint.end) {
		return
	}
	arena.externalScannerCheckpointLeafNodes++
}

func (p *Parser) currentExternalNoTreeLeafCheckpointRef(arena *nodeArena, tok Token) (externalScannerCheckpointRef, bool) {
	if p == nil || arena == nil || !p.currentExternalTokenCheckpointValid {
		return externalScannerCheckpointRef{}, false
	}
	if !p.noTreeCheckpointBenchmarkOnly {
		return externalScannerCheckpointRef{}, false
	}
	if tok.Missing || tok.NoLookahead || tok.Symbol == 0 {
		return externalScannerCheckpointRef{}, false
	}
	if tok.StartByte != p.currentExternalTokenCheckpointStart || tok.EndByte != p.currentExternalTokenCheckpointEnd {
		return externalScannerCheckpointRef{}, false
	}
	cp := arena.recordExternalScannerCompactCheckpoint(
		p.currentExternalTokenCheckpoint.start,
		p.currentExternalTokenCheckpoint.end,
	)
	if !externalScannerCheckpointRefComplete(cp) {
		return externalScannerCheckpointRef{}, false
	}
	arena.compactFullLeafCreated++
	arena.checkpointLeafFullNodesAvoided++
	return cp, true
}

func (p *Parser) currentExternalCompactFullLeafCheckpointRef(arena *nodeArena, tok Token) (externalScannerCheckpointRef, bool) {
	if p == nil || arena == nil || !p.currentExternalTokenCheckpointValid {
		return externalScannerCheckpointRef{}, false
	}
	if tok.Missing || tok.NoLookahead || tok.Symbol == 0 {
		return externalScannerCheckpointRef{}, false
	}
	if tok.StartByte != p.currentExternalTokenCheckpointStart || tok.EndByte != p.currentExternalTokenCheckpointEnd {
		return externalScannerCheckpointRef{}, false
	}
	if p.skipInvisibleFullLeafCheckpoints && !p.isVisibleSymbol(tok.Symbol) {
		return externalScannerCheckpointRef{}, false
	}
	cp := arena.recordExternalScannerCompactCheckpoint(
		p.currentExternalTokenCheckpoint.start,
		p.currentExternalTokenCheckpoint.end,
	)
	if !externalScannerCheckpointRefComplete(cp) {
		return externalScannerCheckpointRef{}, false
	}
	arena.checkpointLeafFullNodesAvoided++
	return cp, true
}

func canReuseUnchangedTree(source []byte, oldTree *Tree, lang *Language) bool {
	if oldTree == nil || oldTree.language != lang || len(oldTree.edits) != 0 {
		return false
	}
	oldSource := oldTree.source
	if len(oldSource) != len(source) {
		return false
	}
	if len(source) == 0 {
		return true
	}
	// Common incremental no-edit case: caller passes the same source slice.
	// Pointer equality avoids memcmp on hot no-op reparses.
	if &oldSource[0] == &source[0] {
		return true
	}
	return bytes.Equal(oldSource, source)
}

func (p *Parser) logf(kind ParserLogType, format string, args ...any) {
	if p == nil || p.logger == nil {
		return
	}
	p.logger(kind, fmt.Sprintf(format, args...))
}

func resolveParseMaxStacks(configuredMaxStacks, maxStacksOverride, conflictWidth int) (maxStacks int, retryPass bool) {
	maxStacks = configuredMaxStacks
	if maxStacks <= 0 {
		maxStacks = maxGLRStacks
	}
	if maxStacksOverride > 0 {
		maxStacks = maxStacksOverride
		retryPass = maxStacksOverride > configuredMaxStacks
	}
	if conflictWidth > maxStacks {
		maxStacks = conflictWidth
	}
	return maxStacks, retryPass
}

func captureParseArenaStats(parseRuntime *ParseRuntime, arena *nodeArena, arenaBreakdown **ArenaBreakdown, preMaterializationFieldRejectCandidates, preMaterializationFieldRejectSameKeyCandidates, preMaterializationFieldRejectOverflowCandidates uint64) bool {
	if parseRuntime == nil || arena == nil {
		return false
	}
	arena.finalizeCompactFullLeafDropped()
	arena.finalizePendingParentDropped()
	parseRuntime.ArenaBytesAllocated = arena.allocatedBytes
	parseRuntime.ArenaBaselineBytes = arena.budgetBaselineBytes
	parseRuntime.MemoryBudgetBytes = arena.budgetBytes
	parseRuntime.ExternalScannerCheckpointRecords = arena.externalScannerCheckpointRecords
	parseRuntime.ExternalScannerCheckpointSlotsAllocated = arena.externalScannerCheckpointSlotsAllocated()
	parseRuntime.ExternalScannerCheckpointBytesAllocated = arena.externalScannerCheckpointBytesAllocated()
	parseRuntime.ExternalScannerSnapshotBytesAllocated = arena.externalScannerSnapshotPayloadBytes
	parseRuntime.ExternalScannerCheckpointLeafNodes = arena.externalScannerCheckpointLeafNodes
	parseRuntime.CompactFullLeafCreated = arena.compactFullLeafCreated
	parseRuntime.CompactFullLeafMaterialized = arena.compactFullLeafMaterialized
	parseRuntime.CompactFullLeafMaterializedForParentReduce = arena.compactFullLeafMaterializedForParentReduce
	parseRuntime.CompactFullLeafMaterializedForParentReject = arena.compactFullLeafMaterializedForParentReject
	parseRuntime.CompactFullLeafMaterializedForFinalTree = arena.compactFullLeafMaterializedForFinalTree
	parseRuntime.CompactFullLeafMaterializedForNormalization = arena.compactFullLeafMaterializedForNormalization
	parseRuntime.CompactFullLeafMaterializedForRecovery = arena.compactFullLeafMaterializedForRecovery
	parseRuntime.CompactFullLeafMaterializedForQuery = arena.compactFullLeafMaterializedForQuery
	parseRuntime.CompactFullLeafMaterializedForCursor = arena.compactFullLeafMaterializedForCursor
	parseRuntime.CompactFullLeafMaterializedForParentAPI = arena.compactFullLeafMaterializedForParentAPI
	parseRuntime.CompactFullLeafMaterializedForEdit = arena.compactFullLeafMaterializedForEdit
	parseRuntime.CompactFullLeafMaterializedForCheckpointRebuild = arena.compactFullLeafMaterializedForCheckpointRebuild
	parseRuntime.CompactFullLeafMaterializedForFieldRejectPayload = arena.compactFullLeafMaterializedForFieldRejectPayload
	parseRuntime.CompactFullLeafDropped = arena.compactFullLeafDropped
	parseRuntime.PendingParentCreated = arena.pendingParentCreated
	parseRuntime.PendingParentMaterialized = arena.pendingParentMaterialized
	parseRuntime.PendingParentMaterializedForParentReduce = arena.pendingParentMaterializedForParentReduce
	parseRuntime.PendingParentMaterializedForParentReject = arena.pendingParentMaterializedForParentReject
	parseRuntime.PendingParentMaterializedForFieldReject = arena.pendingParentMaterializedForFieldReject
	parseRuntime.PendingParentMaterializedForFieldRejectPayload = arena.pendingParentMaterializedForFieldRejectPayload
	parseRuntime.PendingParentMaterializedForFinalTree = arena.pendingParentMaterializedForFinalTree
	parseRuntime.PendingParentMaterializedForNormalization = arena.pendingParentMaterializedForNormalization
	parseRuntime.PendingParentMaterializedForRecovery = arena.pendingParentMaterializedForRecovery
	parseRuntime.PendingParentMaterializedForQuery = arena.pendingParentMaterializedForQuery
	parseRuntime.PendingParentMaterializedForCursor = arena.pendingParentMaterializedForCursor
	parseRuntime.PendingParentMaterializedForParentAPI = arena.pendingParentMaterializedForParentAPI
	parseRuntime.PendingParentMaterializedForEdit = arena.pendingParentMaterializedForEdit
	parseRuntime.PendingParentMaterializedForCheckpointRebuild = arena.pendingParentMaterializedForCheckpointRebuild
	parseRuntime.PendingParentDropped = arena.pendingParentDropped
	parseRuntime.PendingParentsFlattened = arena.pendingParentsFlattened
	parseRuntime.PendingChildRefsFlattened = arena.pendingChildRefsFlattened
	parseRuntime.PendingChildEntriesAllocated = arena.pendingChildEntriesAllocated
	parseRuntime.PendingChildEntryCapacity = arena.pendingChildEntryCapacity()
	parseRuntime.PendingChildEntryWaste = arena.pendingChildEntryWaste()
	parseRuntime.PendingParentCandidates = arena.pendingParentCandidates
	parseRuntime.PendingParentRejectedEmpty = arena.pendingParentRejectedEmpty
	parseRuntime.PendingParentRejectedChildLimit = arena.pendingParentRejectedChildLimit
	parseRuntime.PendingParentRejectedAlias = arena.pendingParentRejectedAlias
	parseRuntime.PendingParentRejectedRawSpan = arena.pendingParentRejectedRawSpan
	parseRuntime.PendingParentRejectedFields = arena.pendingParentRejectedFields
	parseRuntime.PendingParentRejectedFieldsParentHidden = arena.pendingParentRejectedFieldsParentHidden
	parseRuntime.PendingParentRejectedFieldsNoIDs = arena.pendingParentRejectedFieldsNoIDs
	parseRuntime.PendingParentRejectedFieldsInherited = arena.pendingParentRejectedFieldsInherited
	parseRuntime.PendingParentRejectedFieldsHiddenChild = arena.pendingParentRejectedFieldsHiddenChild
	parseRuntime.PendingParentRejectedFieldsHiddenChildPlain = arena.pendingParentRejectedFieldsHiddenChildPlain
	parseRuntime.PendingParentRejectedFieldsHiddenChildPlainEmpty = arena.pendingParentRejectedFieldsHiddenChildPlainEmpty
	parseRuntime.PendingParentRejectedFieldsHiddenChildPlainOne = arena.pendingParentRejectedFieldsHiddenChildPlainOne
	parseRuntime.PendingParentRejectedFieldsHiddenChildPlainMany = arena.pendingParentRejectedFieldsHiddenChildPlainMany
	parseRuntime.PendingParentRejectedFieldsHiddenChildWithFields = arena.pendingParentRejectedFieldsHiddenChildWithFields
	parseRuntime.PendingParentRejectedFieldsChild = arena.pendingParentRejectedFieldsChild
	parseRuntime.PendingParentRejectedFieldsAllVisibleDirect = arena.pendingParentRejectedFieldsAllVisibleDirect
	parseRuntime.PendingParentRejectedChild = arena.pendingParentRejectedChild
	parseRuntime.PendingParentRejectedSpan = arena.pendingParentRejectedSpan
	parseRuntime.PendingParentRejectedFill = arena.pendingParentRejectedFill
	parseRuntime.FinalChildRefParents = arena.finalChildRefParents
	parseRuntime.FinalChildRefs = arena.finalChildRefsCreated
	parseRuntime.FinalChildRefMaterializedParents = arena.finalChildRefsMaterializedParents
	parseRuntime.FinalChildRefMaterializedChildren = arena.finalChildRefsMaterializedChildren
	parseRuntime.FinalChildRefSingleChildAccesses = arena.finalChildRefsSingleChildAccesses
	parseRuntime.FinalChildRefSingleChildMaterializedChildren = arena.finalChildRefsSingleChildMaterializedChildren
	parseRuntime.PreMaterializationFieldRejectCandidates = preMaterializationFieldRejectCandidates
	parseRuntime.PreMaterializationFieldRejectSameKeyCandidates = preMaterializationFieldRejectSameKeyCandidates
	parseRuntime.PreMaterializationFieldRejectOverflowCandidates = preMaterializationFieldRejectOverflowCandidates
	parseRuntime.CheckpointLeafFullNodesAvoided = arena.checkpointLeafFullNodesAvoided
	parseRuntime.LeafNodesConstructed = arena.leafNodesConstructed
	parseRuntime.ParentNodesConstructed = arena.parentNodesConstructed
	parseRuntime.NoTreeReduceNodesConstructed = arena.noTreeReduceNodesConstructed
	parseRuntime.NoTreeLeafNodesConstructed = arena.noTreeLeafNodesConstructed
	if arena.breakdownEnabled && arenaBreakdown != nil {
		*arenaBreakdown = arena.collectArenaBreakdown()
	}
	return true
}

func captureParseScratchStats(parseRuntime *ParseRuntime, scratch *parserScratch, arena *nodeArena, arenaBreakdown **ArenaBreakdown) bool {
	if parseRuntime == nil || scratch == nil {
		return false
	}
	parseRuntime.ScratchBytesAllocated = scratch.allocatedBytes()
	parseRuntime.ScratchBaselineBytes = scratch.budgetBaselineBytes
	parseRuntime.EntryScratchBytesAllocated = scratch.entries.allocatedBytes
	parseRuntime.EntryScratchPeak = uint64(scratch.entries.peakEntriesUsed())
	parseRuntime.GSSBytesAllocated = scratch.gss.allocatedBytes
	parseRuntime.GSSBaselineBytes = scratch.gssBaselineBytes
	parseRuntime.GSSSlabCount = len(scratch.gss.slabs)
	parseRuntime.GSSNodesUsed = scratch.gss.peakUsed
	parseRuntime.GSSNodesCapacity = scratch.gss.capacityNodes()
	parseRuntime.GSSDemotions = scratch.gss.demotions
	parseRuntime.GSSNodesDemoted = scratch.gss.nodesDemoted
	parseRuntime.TransientScratchCheckpoints = scratch.transientCheckpoints
	parseRuntime.TransientChildSlicesAllocated = scratch.transientChildren.slicesAllocated
	parseRuntime.TransientChildPointersAllocated = scratch.transientChildren.pointersAllocated
	parseRuntime.TransientChildSlicesMaterialized = scratch.transientChildren.slicesMaterialized
	parseRuntime.TransientChildPointersMaterialized = scratch.transientChildren.pointersMaterialized
	parseRuntime.TransientParentNodesAllocated = scratch.transientParents.nodesAllocated
	parseRuntime.TransientParentNodesMaterialized = scratch.transientParents.nodesMaterialized
	if arena != nil && arena.breakdownEnabled && arenaBreakdown != nil {
		if *arenaBreakdown == nil {
			*arenaBreakdown = &ArenaBreakdown{}
		}
		(*arenaBreakdown).MergeScratchBytesAllocated = scratch.merge.allocatedBytes()
	}
	return true
}

func parseStopReasonWithTokenSourceEOF(stopReason ParseStopReason, tokenSourceEOFEarly bool) ParseStopReason {
	if tokenSourceEOFEarly && (stopReason == ParseStopAccepted || stopReason == ParseStopNone) {
		return ParseStopTokenSourceEOF
	}
	return stopReason
}

// rescueMaterializationStopReason preserves a stop reason that result
// materialization stamped onto a replacement error tree after the parse loop
// had already accepted. buildResultFromGLR can discard the accepted stacks and
// return a sentinel error tree carrying its own terminal reason (for example
// memory_budget when the budget trips during materialization); the loop-level
// stopReason still says accepted at that point, and stamping it over the tree
// would report a full-span sentinel ERROR root as a successful parse.
func rescueMaterializationStopReason(loopReason ParseStopReason, tree *Tree) ParseStopReason {
	if loopReason != ParseStopAccepted || tree == nil {
		return loopReason
	}
	already := tree.rawParseRuntime().StopReason
	if already == "" || already == ParseStopNone || already == ParseStopAccepted {
		return loopReason
	}
	return already
}

func recordParseRuntimeLoopStats(parseRuntime *ParseRuntime, scratch *parserScratch, iterationsUsed, nodeCount, peakStackDepth, maxStacksSeen, singleStackIterations, multiStackIterations int, singleStackTokens, multiStackTokens uint64) {
	if parseRuntime == nil {
		return
	}
	parseRuntime.Iterations = iterationsUsed
	parseRuntime.NodesAllocated = nodeCount
	parseRuntime.PeakStackDepth = peakStackDepth
	parseRuntime.MaxStacksSeen = maxStacksSeen
	parseRuntime.SingleStackIterations = singleStackIterations
	parseRuntime.MultiStackIterations = multiStackIterations
	parseRuntime.SingleStackTokens = singleStackTokens
	parseRuntime.MultiStackTokens = multiStackTokens
	if scratch == nil {
		return
	}
	parseRuntime.SingleStackGSSNodes = scratch.gss.singleStackAllocs
	parseRuntime.MultiStackGSSNodes = scratch.gss.multiStackAllocs
	parseRuntime.GSSNodesAllocated = scratch.audit.totalGSSAllocated
	parseRuntime.GSSNodesRetained = scratch.audit.totalGSSRetained
	parseRuntime.GSSNodesDroppedSameToken = scratch.audit.totalGSSDropped
	parseRuntime.ParentNodesAllocated = scratch.audit.totalParentAllocated
	parseRuntime.ParentNodesRetained = scratch.audit.totalParentRetained
	parseRuntime.ParentNodesDroppedSameToken = scratch.audit.totalParentDropped
	parseRuntime.LeafNodesAllocated = scratch.audit.totalLeafAllocated
	parseRuntime.LeafNodesRetained = scratch.audit.totalLeafRetained
	parseRuntime.LeafNodesDroppedSameToken = scratch.audit.totalLeafDropped
	parseRuntime.ChildSlicesAllocated = scratch.audit.totalChildSlicesAllocated
	parseRuntime.ChildSlicesRetained = scratch.audit.totalChildSlicesRetained
	parseRuntime.ChildSlicesDroppedSameToken = scratch.audit.totalChildSlicesDropped
	parseRuntime.ChildPointersAllocated = scratch.audit.totalChildPointersAllocated
	parseRuntime.ChildPointersRetained = scratch.audit.totalChildPointersRetained
	parseRuntime.ChildPointersDroppedSameToken = scratch.audit.totalChildPointersDropped
	parseRuntime.ReduceChildFastGSS = scratch.audit.reduceChildPathRuntime(reduceChildPathFastGSS)
	parseRuntime.ReduceChildAllVisible = scratch.audit.reduceChildPathRuntime(reduceChildPathAllVisible)
	parseRuntime.ReduceChildScratchGeneral = scratch.audit.reduceChildPathRuntime(reduceChildPathScratchGeneral)
	parseRuntime.ReduceChildScratchNoAlias = scratch.audit.reduceChildPathRuntime(reduceChildPathScratchNoAlias)
	parseRuntime.MergeStacksIn = scratch.audit.mergeStacksIn
	parseRuntime.MergeStacksOut = scratch.audit.mergeStacksOut
	parseRuntime.MergeSlotsUsed = scratch.audit.mergeSlotsUsed
	parseRuntime.GlobalCullStacksIn = scratch.audit.globalCullStacksIn
	parseRuntime.GlobalCullStacksOut = scratch.audit.globalCullStacksOut
	parseRuntime.StackEquivCalls = scratch.audit.stackEquivCalls
	parseRuntime.StackEquivTrue = scratch.audit.stackEquivTrue
	parseRuntime.StackEquivDepthMismatch = scratch.audit.stackEquivDepthMismatch
	parseRuntime.StackEquivHashMismatch = scratch.audit.stackEquivHashMismatch
	parseRuntime.StackEquivStateMismatch = scratch.audit.stackEquivStateMismatch
	parseRuntime.StackEquivPayloadMismatch = scratch.audit.stackEquivPayloadMismatch
	parseRuntime.StackEquivEntryCompares = scratch.audit.stackEquivEntryCompares
	parseRuntime.StackEquivStateMismatchDepthSum = scratch.audit.stackEquivStateMismatchDepthSum
	parseRuntime.StackEquivStateMismatchMaxDepth = scratch.audit.stackEquivStateMismatchMaxDepth
	parseRuntime.StackEquivStateMismatchDepthBuckets = scratch.audit.stackEquivStateMismatchDepthBuckets
	parseRuntime.StackEquivPayloadMismatchDepthSum = scratch.audit.stackEquivPayloadMismatchDepthSum
	parseRuntime.StackEquivPayloadMismatchMaxDepth = scratch.audit.stackEquivPayloadMismatchMaxDepth
	parseRuntime.StackEquivPayloadMismatchDepthBuckets = scratch.audit.stackEquivPayloadMismatchDepthBuckets
	parseRuntime.StackEquivPayloadHeaderSigDiff = scratch.audit.stackEquivPayloadHeaderSigDiff
	parseRuntime.StackEquivPayloadHeaderSigSame = scratch.audit.stackEquivPayloadHeaderSigSame
	parseRuntime.StackEquivPayloadShallowSigDiff = scratch.audit.stackEquivPayloadShallowSigDiff
	parseRuntime.StackEquivPayloadShallowSigSame = scratch.audit.stackEquivPayloadShallowSigSame
	parseRuntime.StackEquivPairKeyed = scratch.audit.stackEquivPairKeyed
	parseRuntime.StackEquivPairUnkeyed = scratch.audit.stackEquivPairUnkeyed
	parseRuntime.StackEquivPairRepeats = scratch.audit.stackEquivPairRepeats
	parseRuntime.StackEquivPairRepeatTrue = scratch.audit.stackEquivPairRepeatTrue
	parseRuntime.StackEquivPairRepeatFalse = scratch.audit.stackEquivPairRepeatFalse
	parseRuntime.StackEquivPairRepeatMismatch = scratch.audit.stackEquivPairRepeatMismatch
	parseRuntime.StackEquivPairStores = scratch.audit.stackEquivPairStores
	parseRuntime.MergeHeaderEqTotal = scratch.audit.mergeHeaderEqTotal
	parseRuntime.MergeDeepTrue = scratch.audit.mergeDeepTrue
	parseRuntime.MergeDeepFalse = scratch.audit.mergeDeepFalse
	parseRuntime.MergeHeaderDeepDivergent = scratch.audit.mergeHeaderDeepDivergent
	parseRuntime.EquivCacheLookups = scratch.audit.equivCacheLookups
	parseRuntime.EquivCacheHits = scratch.audit.equivCacheHits
	parseRuntime.EquivCacheStores = scratch.audit.equivCacheStores
	parseRuntime.EquivCacheMisses = scratch.audit.equivCacheMisses
	parseRuntime.EquivCacheTrueHits = scratch.audit.equivCacheTrueHits
	parseRuntime.EquivCacheFalseHits = scratch.audit.equivCacheFalseHits
	parseRuntime.EquivCacheEpochMisses = scratch.audit.equivCacheEpochMisses
	parseRuntime.EquivCacheKeyMisses = scratch.audit.equivCacheKeyMisses
	parseRuntime.EquivCacheVersionMisses = scratch.audit.equivCacheVersionMisses
	parseRuntime.EquivSkipError = scratch.audit.equivSkipError
	parseRuntime.EquivSkipLeaf = scratch.audit.equivSkipLeaf
	parseRuntime.EquivSkipFieldMismatch = scratch.audit.equivSkipFieldMismatch
	parseRuntime.EquivExactCalls = scratch.audit.equivExactCalls
	parseRuntime.EquivExactTrue = scratch.audit.equivExactTrue
	parseRuntime.EquivExactPointerTrue = scratch.audit.equivExactPointerTrue
	parseRuntime.EquivExactNilMismatch = scratch.audit.equivExactNilMismatch
	parseRuntime.EquivExactHeaderMismatch = scratch.audit.equivExactHeaderMismatch
	parseRuntime.EquivExactChildMismatch = scratch.audit.equivExactChildMismatch
	parseRuntime.EquivExactTerminalCalls = scratch.audit.equivExactTerminalCalls
	parseRuntime.EquivExactTerminalTrue = scratch.audit.equivExactTerminalTrue
	parseRuntime.EquivExactTerminalFalse = scratch.audit.equivExactTerminalFalse
	parseRuntime.EquivFrontierCalls = scratch.audit.equivFrontierCalls
	parseRuntime.EquivFrontierTrue = scratch.audit.equivFrontierTrue
	parseRuntime.EquivExactChildCompares = scratch.audit.equivExactChildCompares
	parseRuntime.EquivFrontierChildScans = scratch.audit.equivFrontierChildScans
	parseRuntime.EquivFrontierCandidateCompares = scratch.audit.equivFrontierCandidateCompares
	parseRuntime.EquivStateStats = scratch.audit.equivStateStats()
}

func recordParseRuntimeMaterializationTiming(parseRuntime *ParseRuntime, timingRef *parseMaterializationTiming, timing parseMaterializationTiming) {
	if parseRuntime == nil || timingRef == nil {
		return
	}
	parseRuntime.ResultSelectionNanos = timing.resultSelectionNanos
	parseRuntime.TransientParentMaterializationNanos = timing.transientParentMaterializeNanos
	parseRuntime.ResultTreeBuildNanos = timing.resultTreeBuildNanos
	parseRuntime.TransientChildMaterializationNanos = timing.transientChildMaterializationNanos
	parseRuntime.ResultPythonKeywordRepairNanos = timing.pythonKeywordRepairNanos
	parseRuntime.ResultPythonRootRepairNanos = timing.pythonRootRepairNanos
	parseRuntime.ResultFinalizeRootNanos = timing.resultFinalizeRootNanos
	parseRuntime.ResultExtendTrailingNanos = timing.resultExtendTrailingNanos
	parseRuntime.ResultNormalizeRootStartNanos = timing.resultNormalizeRootStartNanos
	parseRuntime.ResultCompatibilityNanos = timing.resultCompatibilityNanos
	parseRuntime.ResultParentLinkNanos = timing.resultParentLinkNanos
	if timing.reduceRangeNanos != 0 ||
		timing.reducePendingParentNanos != 0 ||
		timing.reduceChildBuildNanos != 0 ||
		timing.reduceParentBuildNanos != 0 ||
		timing.reduceSpanNanos != 0 ||
		timing.reduceStackPushNanos != 0 ||
		timing.reduceNoTreeBuildNanos != 0 {
		parseRuntime.ReduceTiming = &ParseReduceTiming{
			RangeNanos:         timing.reduceRangeNanos,
			PendingParentNanos: timing.reducePendingParentNanos,
			ChildBuildNanos:    timing.reduceChildBuildNanos,
			ParentBuildNanos:   timing.reduceParentBuildNanos,
			SpanNanos:          timing.reduceSpanNanos,
			StackPushNanos:     timing.reduceStackPushNanos,
			NoTreeBuildNanos:   timing.reduceNoTreeBuildNanos,
		}
	}
	if timing.actionExtraShiftNanos != 0 ||
		timing.actionNoActionNanos != 0 ||
		timing.actionNoActionRelexNanos != 0 ||
		timing.actionNoActionMissingNanos != 0 ||
		timing.actionNoActionRecoverNanos != 0 ||
		timing.actionNoActionErrorNanos != 0 ||
		timing.actionConflictChoiceNanos != 0 ||
		timing.actionConflictForkNanos != 0 ||
		timing.actionSingleShiftNanos != 0 ||
		timing.actionSingleReduceNanos != 0 ||
		timing.actionSingleAcceptNanos != 0 ||
		timing.actionSingleRecoverNanos != 0 ||
		timing.actionSingleOtherNanos != 0 {
		parseRuntime.ActionTiming = &ParseActionTiming{
			ExtraShiftNanos:      timing.actionExtraShiftNanos,
			NoActionNanos:        timing.actionNoActionNanos,
			NoActionRelexNanos:   timing.actionNoActionRelexNanos,
			NoActionMissingNanos: timing.actionNoActionMissingNanos,
			NoActionRecoverNanos: timing.actionNoActionRecoverNanos,
			NoActionErrorNanos:   timing.actionNoActionErrorNanos,
			ConflictChoiceNanos:  timing.actionConflictChoiceNanos,
			ConflictForkNanos:    timing.actionConflictForkNanos,
			SingleShiftNanos:     timing.actionSingleShiftNanos,
			SingleReduceNanos:    timing.actionSingleReduceNanos,
			SingleAcceptNanos:    timing.actionSingleAcceptNanos,
			SingleRecoverNanos:   timing.actionSingleRecoverNanos,
			SingleOtherNanos:     timing.actionSingleOtherNanos,
		}
	}
}

func recordParseRuntimePhaseTiming(parseRuntime *ParseRuntime, timingRef *parseMaterializationTiming, parseStart time.Time, parserLoopNanos, tokenNextNanos, actionDispatchNanos, actionLookupNanos, glrMergeNanos, glrCullNanos int64) {
	if parseRuntime == nil || timingRef == nil {
		return
	}
	parseRuntime.ParseWallNanos = time.Since(parseStart).Nanoseconds()
	parseRuntime.ParserLoopNanos = parserLoopNanos
	parseRuntime.TokenNextNanos = tokenNextNanos
	parseRuntime.ActionDispatchNanos = actionDispatchNanos
	parseRuntime.ActionLookupNanos = actionLookupNanos
	parseRuntime.GLRMergeNanos = glrMergeNanos
	parseRuntime.GLRCullNanos = glrCullNanos
}

func recordParseRuntimeTokenStats(parseRuntime *ParseRuntime, tokensConsumed uint64, lastTokenEndByte uint32, lastTokenSymbol Symbol, lastTokenWasEOF, tokenSourceEOFEarly bool) {
	if parseRuntime == nil {
		return
	}
	parseRuntime.TokensConsumed = tokensConsumed
	parseRuntime.LastTokenEndByte = lastTokenEndByte
	parseRuntime.LastTokenSymbol = lastTokenSymbol
	parseRuntime.LastTokenWasEOF = lastTokenWasEOF
	parseRuntime.TokenSourceEOFEarly = tokenSourceEOFEarly
}

func recordParseRuntimeRootStats(parseRuntime *ParseRuntime, tree *Tree, source []byte, expectedEOFByte uint32, included []Range, collectFinalStats bool, lang *Language) {
	if parseRuntime == nil {
		return
	}
	parseRuntime.RootEndByte = 0
	parseRuntime.Truncated = false
	root := rawRootOrNil(tree)
	if root == nil {
		return
	}
	parseRuntime.RootEndByte = root.EndByte()
	parseRuntime.Truncated = parseRuntime.RootEndByte < expectedEOFByte
	tailSource := source
	if tailSource == nil && tree != nil {
		tailSource = tree.Source()
	}
	tailStart := parseRuntime.RootEndByte
	if parseRuntime.LastTokenWasEOF && parseRuntime.LastTokenEndByte > tailStart && parseRuntime.LastTokenEndByte <= expectedEOFByte {
		tailStart = parseRuntime.LastTokenEndByte
	}
	if parseRuntime.Truncated && parserTailAllowsCleanAcceptance(tailSource, tailStart, expectedEOFByte, included, languageLineContinuationEscapeByte(lang)) {
		parseRuntime.Truncated = false
	}
	if !collectFinalStats {
		return
	}
	finalStats := collectFinalTreeMaterializationStats(root, lang)
	parseRuntime.FinalNodes = finalStats.nodes
	parseRuntime.FinalParentNodes = finalStats.parentNodes
	parseRuntime.FinalLeafNodes = finalStats.leafNodes
	parseRuntime.FinalFieldedParentNodes = finalStats.fieldedParentNodes
	parseRuntime.FinalUnfieldedParentNodes = finalStats.unfieldedParentNodes
	parseRuntime.FinalVisibleParentNodes = finalStats.visibleParentNodes
	parseRuntime.FinalHiddenParentNodes = finalStats.hiddenParentNodes
	parseRuntime.FinalCheckpointLeafNodes = finalStats.checkpointLeafNodes
	parseRuntime.FinalChildSlices = finalStats.childSlices
	parseRuntime.FinalChildPointers = finalStats.childPointers
	parseRuntime.FinalFieldIDElements = finalStats.fieldIDElements
	parseRuntime.FinalFieldSourceElements = finalStats.fieldSourceElements
}

func firstErrorNode(root *Node) *Node {
	if root == nil {
		return nil
	}
	if root.symbol == errorSymbol {
		return root
	}
	for i := 0; i < root.ChildCount(); i++ {
		if child := firstErrorNode(root.Child(i)); child != nil {
			return child
		}
	}
	return nil
}

func (p *Parser) materializeTransientChildrenForReturnedTree(tree *Tree, arena *nodeArena, scratch *parserScratch) ParseStopReason {
	if scratch == nil {
		return ParseStopNone
	}
	return scratch.transientChildren.materializeNodeUntil(rawRootOrNil(tree), arena, &scratch.nodeLinks, p)
}

func copyParseRuntimeToTiming(timing *incrementalParseTiming, parseRuntime ParseRuntime) {
	if timing == nil {
		return
	}
	timing.stopReason = parseRuntime.StopReason
	timing.tokensConsumed = parseRuntime.TokensConsumed
	timing.lastTokenEndByte = parseRuntime.LastTokenEndByte
	timing.expectedEOFByte = parseRuntime.ExpectedEOFByte
	timing.arenaBytesAllocated = parseRuntime.ArenaBytesAllocated
	timing.scratchBytesAllocated = parseRuntime.ScratchBytesAllocated
	timing.entryScratchBytesAllocated = uint64(parseRuntime.EntryScratchBytesAllocated)
	timing.gssBytesAllocated = uint64(parseRuntime.GSSBytesAllocated)
	timing.singleStackIterations = parseRuntime.SingleStackIterations
	timing.multiStackIterations = parseRuntime.MultiStackIterations
	timing.singleStackTokens = parseRuntime.SingleStackTokens
	timing.multiStackTokens = parseRuntime.MultiStackTokens
	timing.singleStackGSSNodes = parseRuntime.SingleStackGSSNodes
	timing.multiStackGSSNodes = parseRuntime.MultiStackGSSNodes
	timing.gssNodesAllocated = parseRuntime.GSSNodesAllocated
	timing.gssNodesRetained = parseRuntime.GSSNodesRetained
	timing.gssNodesDroppedSameToken = parseRuntime.GSSNodesDroppedSameToken
	timing.parentNodesAllocated = parseRuntime.ParentNodesAllocated
	timing.parentNodesRetained = parseRuntime.ParentNodesRetained
	timing.parentNodesDroppedSameToken = parseRuntime.ParentNodesDroppedSameToken
	timing.leafNodesAllocated = parseRuntime.LeafNodesAllocated
	timing.leafNodesRetained = parseRuntime.LeafNodesRetained
	timing.leafNodesDroppedSameToken = parseRuntime.LeafNodesDroppedSameToken
	timing.mergeStacksIn = parseRuntime.MergeStacksIn
	timing.mergeStacksOut = parseRuntime.MergeStacksOut
	timing.mergeSlotsUsed = parseRuntime.MergeSlotsUsed
	timing.globalCullStacksIn = parseRuntime.GlobalCullStacksIn
	timing.globalCullStacksOut = parseRuntime.GlobalCullStacksOut
	timing.parserLoopNanos = parseRuntime.ParserLoopNanos
	timing.tokenNextNanos = parseRuntime.TokenNextNanos
	timing.actionDispatchNanos = parseRuntime.ActionDispatchNanos
	timing.actionLookupNanos = parseRuntime.ActionLookupNanos
	timing.glrMergeNanos = parseRuntime.GLRMergeNanos
	timing.glrCullNanos = parseRuntime.GLRCullNanos
	timing.resultSelectionNanos = parseRuntime.ResultSelectionNanos
	timing.transientParentMaterializationNanos = parseRuntime.TransientParentMaterializationNanos
	timing.resultTreeBuildNanos = parseRuntime.ResultTreeBuildNanos
	timing.transientChildMaterializationNanos = parseRuntime.TransientChildMaterializationNanos
	timing.resultPythonKeywordRepairNanos = parseRuntime.ResultPythonKeywordRepairNanos
	timing.resultPythonRootRepairNanos = parseRuntime.ResultPythonRootRepairNanos
	timing.resultFinalizeRootNanos = parseRuntime.ResultFinalizeRootNanos
	timing.resultExtendTrailingNanos = parseRuntime.ResultExtendTrailingNanos
	timing.resultNormalizeRootStartNanos = parseRuntime.ResultNormalizeRootStartNanos
	timing.resultCompatibilityNanos = parseRuntime.ResultCompatibilityNanos
	timing.resultParentLinkNanos = parseRuntime.ResultParentLinkNanos
	if reduceTiming := parseRuntime.ReduceTiming; reduceTiming != nil {
		timing.reduceRangeNanos = reduceTiming.RangeNanos
		timing.reducePendingParentNanos = reduceTiming.PendingParentNanos
		timing.reduceChildBuildNanos = reduceTiming.ChildBuildNanos
		timing.reduceParentBuildNanos = reduceTiming.ParentBuildNanos
		timing.reduceSpanNanos = reduceTiming.SpanNanos
		timing.reduceStackPushNanos = reduceTiming.StackPushNanos
		timing.reduceNoTreeBuildNanos = reduceTiming.NoTreeBuildNanos
	}
	if actionTiming := parseRuntime.ActionTiming; actionTiming != nil {
		timing.actionExtraShiftNanos = actionTiming.ExtraShiftNanos
		timing.actionNoActionNanos = actionTiming.NoActionNanos
		timing.actionNoActionRelexNanos = actionTiming.NoActionRelexNanos
		timing.actionNoActionMissingNanos = actionTiming.NoActionMissingNanos
		timing.actionNoActionRecoverNanos = actionTiming.NoActionRecoverNanos
		timing.actionNoActionErrorNanos = actionTiming.NoActionErrorNanos
		timing.actionConflictChoiceNanos = actionTiming.ConflictChoiceNanos
		timing.actionConflictForkNanos = actionTiming.ConflictForkNanos
		timing.actionSingleShiftNanos = actionTiming.SingleShiftNanos
		timing.actionSingleReduceNanos = actionTiming.SingleReduceNanos
		timing.actionSingleAcceptNanos = actionTiming.SingleAcceptNanos
		timing.actionSingleRecoverNanos = actionTiming.SingleRecoverNanos
		timing.actionSingleOtherNanos = actionTiming.SingleOtherNanos
	}
	timing.normalizationNanos = parseRuntime.NormalizationNanos
}

// realTokenAttachmentGapIsParserPadding reports whether the gap between the
// stack's current byte offset and tok's start is trivia the parser may
// silently cross before attaching tok. included carries the parser's
// configured include ranges (nil when SetIncludedRanges was never called).
// When included is non-empty, the scan clips to the gap's overlap with those
// ranges and treats everything outside every included range as
// automatically crossable — see bytesAreParserPaddingInIncludedRanges.
// Without that clipping, a gap that straddles an excluded region between two
// included ranges scans real, non-included source text as if it had to be
// whitespace, which it almost never is, so the gap reads as "not padding"
// and the caller kills the stack: on the included-ranges route that forces
// recovery the C parser never enters for the same input. continuationEscape
// is the calling Parser's language-declared line-continuation escape byte (0
// if none — see Language.LineContinuationEscapeByte and
// (*Parser).lineContinuationEscapeByte), threaded through to
// bytesAreParserPadding so a language-declared escape+newline (for example
// PowerShell's backtick) counts as padding here exactly like the
// unconditional backslash+newline case.
func realTokenAttachmentGapIsParserPadding(source []byte, s *glrStack, tok Token, included []Range, continuationEscape byte) bool {
	if s == nil || tok.Missing || tok.NoLookahead || tok.StartByte <= s.byteOffset {
		return true
	}
	if tok.ExternalScannerToken && tok.ExternalScannerStartByte == s.byteOffset {
		return true
	}
	if tok.lexerSkippedPrefix && tok.lexerSkippedPrefixStart == s.byteOffset {
		return true
	}
	if int(s.byteOffset) > len(source) || int(tok.StartByte) > len(source) {
		return true
	}
	return bytesAreParserPaddingInIncludedRanges(source, s.byteOffset, tok.StartByte, included, continuationEscape)
}

// realShiftGapIsParserPadding is realTokenAttachmentGapIsParserPadding with
// no included ranges configured. Production always has a Parser in scope and
// passes its configured ranges (see (*Parser).guardRealShiftGap); this form
// exists for the direct, Parser-free callers that check a gap without one.
func realShiftGapIsParserPadding(source []byte, s *glrStack, tok Token, continuationEscape byte) bool {
	return realTokenAttachmentGapIsParserPadding(source, s, tok, nil, continuationEscape)
}

// skippedRealGapContinuesSeparatedList reports whether the sole active stack is
// mid-production immediately after an anonymous separator terminal (e.g. a comma
// in a separated list) and the real lookahead continues that production. In that
// position, covering a lexer-skipped stray with a STRUCTURAL error node — one
// that changes the automaton state (pushOrExtendErrorNode's
// schemeErrorRecoveryState target) or counts toward the enclosing production's
// ChildCount — would corrupt the pending reduction and could collapse the
// enclosing construct into a flat ERROR. When this returns true,
// tryMaterializeSkippedRealGap calls materializeSkippedGapAsExtraError instead:
// it covers the gap with a transparent EXTRA ERROR leaf pushed in the SAME
// state (not the older silent shift-across, which advanced past the gap with
// no node at all and left it in no leaf's span, HasError unset). See
// materializeSkippedGapAsExtraError's doc for the mechanism and its known
// remaining shape gap versus C tree-sitter's representation of the same stray.
func (p *Parser) skippedRealGapContinuesSeparatedList(s *glrStack, state StateID, tok Token) bool {
	if p == nil || s == nil || tok.Symbol == 0 || tok.Symbol == errorSymbol || tok.Missing || tok.NoLookahead {
		return false
	}
	top := stackEntryNode(s.top())
	if top == nil || top.symbol == errorSymbol || top.isMissing() || len(top.children) != 0 {
		return false
	}
	// The just-shifted top must be an anonymous (hidden) terminal with a real
	// span — the separator position where a structural error breaks the
	// enclosing production. Named leaves (identifiers, literals) and reduced
	// nonterminals are excluded so ordinary gap materialization is unaffected.
	if p.isNamedSymbol(top.symbol) || top.startByte >= top.endByte {
		return false
	}
	// The real lookahead must continue the production via a single, NON-extra
	// shift. Extra-shift lookaheads (trivia/comment tokens attached transparently)
	// and recover/ambiguous entries must still take ordinary gap materialization.
	return p.stateDeterministicNonExtraShift(state, tok.Symbol)
}

// stateDeterministicNonExtraShift reports whether state has exactly one action
// for sym and it is a plain (non-extra) shift.
func (p *Parser) stateDeterministicNonExtraShift(state StateID, sym Symbol) bool {
	if p == nil || p.language == nil {
		return false
	}
	idx := p.lookupActionIndex(state, sym)
	if idx == 0 || int(idx) >= len(p.language.ParseActions) {
		return false
	}
	actions := p.language.ParseActions[idx].Actions
	if len(actions) != 1 {
		return false
	}
	return actions[0].Type == ParseActionShift && !actions[0].Extra
}

// languageLineContinuationEscapeByte returns lang's declared line-continuation
// escape byte (see Language.LineContinuationEscapeByte), or 0 when lang is nil
// or declares none. This is the single accessor every padding-classification
// call site reads through — both callers that only have a *Language (a Tree
// already carries its language; some compact-engine call sites carry lang
// directly) and (*Parser).lineContinuationEscapeByte below, which is the same
// lookup for callers that have a *Parser instead. Keeping both behind the
// same function keeps the gap-padding and tail-acceptance classifications
// consistent (and default-off for every language that has not declared an
// escape) no matter which handle a given call site holds.
func languageLineContinuationEscapeByte(lang *Language) byte {
	if lang == nil {
		return 0
	}
	return lang.LineContinuationEscapeByte
}

// lineContinuationEscapeByte returns p's language-declared line-continuation
// escape byte (see Language.LineContinuationEscapeByte), or 0 when p or its
// language is nil, or the language declares none. Every production caller of
// realTokenAttachmentGapIsParserPadding / bytesAreParserPadding /
// parserTailAllowsCleanAcceptance's chain that has a Parser in scope reads the
// escape byte through this single accessor so the gap-padding and
// tail-acceptance classifications stay consistent (and default-off for every
// language that has not declared an escape) across every caller.
func (p *Parser) lineContinuationEscapeByte() byte {
	if p == nil {
		return 0
	}
	return languageLineContinuationEscapeByte(p.language)
}

// parserStackEndPoint returns the parser-owned point at a stack's current
// byte offset. An included-range parse starts with an empty stack at the
// first selected byte, so its point comes from the range rather than from a
// stack entry that does not exist yet.
func (p *Parser) parserStackEndPoint(s *glrStack) Point {
	if s == nil {
		return Point{}
	}
	top := s.top()
	if stackEntryHasNode(top) {
		return stackEntryNodeEndPoint(top)
	}
	if p != nil && len(p.included) > 0 {
		first := p.included[0]
		if s.byteOffset == first.StartByte {
			return first.StartPoint
		}
	}
	return stackEntryNodeEndPoint(top)
}

// materializeSkippedGapAsExtraError covers a lexer-skipped mid-production gap
// (a stray run of bytes immediately after an anonymous separator, where the
// real lookahead continues the production via a single deterministic shift,
// per skippedRealGapContinuesSeparatedList) with a transparent EXTRA ERROR
// leaf spanning exactly the gap, then advances the stack's byte offset across
// it. The leaf is pushed with parseState == state (the same state the caller
// already resolved the deterministic shift against, not
// schemeErrorRecoveryState's possibly-different recovery target), so the
// following action lookup for tok is unaffected: this mirrors
// pushLexErrorRunLeaf's "resume in the same state" contract. Because
// reduceWindowFromGSS only counts non-extra stack entries toward a
// production's ChildCount, the leaf is folded into whichever production
// later reduces over it without perturbing arity, while populateParentNode's
// HasError OR propagates the error through the reduce-built ancestors above
// it, all the way to the tree root. Python's root-level convention (the now-
// retired syntheticRootCanDropError / pythonModuleChildrenLookComplete pair,
// parser_result_root_build.go / parser_result_python.go) used to break
// exactly that propagation for EXTRA children: it skipped extra children
// before checking their HasError, so this leaf -- extra by construction, see
// leaf.setExtra(true) below -- could still be silently dropped at the python
// module root even though it correctly carries HasError=true. PR #636 fixed
// the check to look at HasError on every child, extra or not, before the
// extra-skip; the fix then proved the whole drop mechanism inert (measured:
// consulted thousands of times, actionable on a handful, and wrong every
// time it acted) and the mechanism was deleted outright
// (elm/synthetic-root-drop-retirement), so this leaf's error status now
// reaches the root unconditionally, like any other.
//
// This is an ACCOUNTING fix, not a shape fix: the skipped bytes now have a
// span and HasError=true, matching C tree-sitter's verdict that the
// construct is erroneous. The leaf's own shape still diverges from C's for
// the same stray in two ways that remain open follow-up work: the span
// covers the whole lexer-skipped gap (which can include trivia C would not
// attribute to the stray), and the leaf is childless where C typically wraps
// the stray token as a child of its own ERROR/error_repeat node. Closing that
// gap needs re-lexing the skipped bytes to find the stray token's true
// bounds and giving the leaf that token as a child, which needs its own
// verification pass and is out of scope here.
func (p *Parser) materializeSkippedGapAsExtraError(s *glrStack, state StateID, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) {
	// See pushOrExtendErrorNode: error content makes costs relevant. p is
	// never nil here: the only caller (tryMaterializeSkippedRealGap) reaches
	// this branch only after p.skippedRealGapContinuesSeparatedList already
	// returned true, and that function itself returns false for a nil p.
	p.markCRecoveryCostCompetitionRelevant()
	startPoint := p.parserStackEndPoint(s)
	leaf := newLeafNodeInArena(arena, errorSymbol, true, s.byteOffset, tok.StartByte, startPoint, tok.StartPoint)
	leaf.setHasError(true)
	leaf.setExtra(true)
	leaf.parseState = state
	if perfCountersEnabled {
		perfRecordErrorNode()
	}
	p.pushStackNode(s, state, leaf, entryScratch, gssScratch)
	if nodeCount != nil {
		*nodeCount = *nodeCount + 1
	}
	if trackChildErrors != nil {
		*trackChildErrors = true
	}
	s.byteOffset = tok.StartByte
}

func (p *Parser) tryMaterializeSkippedRealGap(source []byte, s *glrStack, state StateID, tok Token, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch, gssScratch *gssScratch, trackChildErrors *bool) bool {
	if s == nil || tok.StartByte <= s.byteOffset || realTokenAttachmentGapIsParserPadding(source, s, tok, p.included, p.lineContinuationEscapeByte()) {
		return false
	}
	// A stray run of bytes that the lexer skipped mid-production, immediately
	// after an anonymous separator terminal with a concrete deterministic
	// shift for the real lookahead, is covered by materializeSkippedGapAsExtraError
	// rather than by ordinary structural gap materialization below — see that
	// function's doc and skippedRealGapContinuesSeparatedList's doc for why a
	// structural node here would corrupt the pending reduction, and for the
	// accounting-vs-shape distinction in what this branch actually fixes.
	// parser_shift_gap_test.go's synthetic-language coverage of
	// skippedRealGapContinuesSeparatedList's guard clauses pins the shapes
	// this must keep matching.
	if p.skippedRealGapContinuesSeparatedList(s, state, tok) {
		if p.glrTrace {
			fmt.Printf("    MATERIALIZE-EXTRA skipped real gap (mid-list separator): gap=%d..%d before tok=%d..%d\n",
				s.byteOffset, tok.StartByte, tok.StartByte, tok.EndByte)
		}
		p.materializeSkippedGapAsExtraError(s, state, tok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
		return true
	}
	if p.tryExtendHiddenTrailingErrorAcrossSkippedRealGap(source, s, tok, nodeCount, arena, trackChildErrors) {
		return s.byteOffset == tok.StartByte
	}
	top := stackEntryNode(s.top())
	startPoint := p.parserStackEndPoint(s)
	if top != nil && top.symbol == errorSymbol {
		if top.isMissing() ||
			len(top.children) != 0 ||
			top.parseState != state ||
			top.endByte != s.byteOffset {
			return false
		}
		startPoint = top.endPoint
	}
	gapTok := Token{
		Symbol:     errorSymbol,
		StartByte:  s.byteOffset,
		EndByte:    tok.StartByte,
		StartPoint: startPoint,
		EndPoint:   tok.StartPoint,
	}
	p.pushOrExtendErrorNode(s, state, gapTok, nodeCount, arena, entryScratch, gssScratch, trackChildErrors)
	return s.byteOffset == tok.StartByte
}

func (p *Parser) tryExtendHiddenTrailingErrorAcrossSkippedRealGap(source []byte, s *glrStack, tok Token, nodeCount *int, arena *nodeArena, trackChildErrors *bool) bool {
	if p == nil || p.language == nil || s == nil || arena == nil {
		return false
	}
	top := stackEntryNode(s.top())
	if top == nil || top.symbol == errorSymbol || top.isMissing() || len(top.children) == 0 {
		return false
	}
	if symbolStructuralForHiddenFlattening(top.symbol, p.language.SymbolMetadata, nil) {
		return false
	}
	err, ok := trailingHiddenErrorLeaf(top, p.language.SymbolMetadata, s.byteOffset)
	if !ok || err == nil || err.startByte >= err.endByte || tok.StartByte <= err.endByte {
		return false
	}

	merged := newParentNodeInArena(arena, errorSymbol, true, nil, nil, 0)
	cSetNodeSpan(merged, err.startByte, tok.StartByte, err.startPoint, tok.StartPoint)
	merged.setHasError(true)
	merged.setExtra(err.isExtra())
	merged.preGotoState = err.preGotoState
	merged.parseState = err.parseState
	p.materializeAnonymousChildrenForRecoveredError(source, merged, arena)
	if len(merged.children) == 0 {
		return false
	}
	replaceTrailingHiddenErrorLeaf(top, p.language.SymbolMetadata, s.byteOffset, merged, tok.StartByte, tok.StartPoint)
	if s.byteOffset < tok.StartByte {
		s.byteOffset = tok.StartByte
	}
	if trackChildErrors != nil {
		*trackChildErrors = true
	}
	if nodeCount != nil {
		*nodeCount = *nodeCount + 1
	}
	if p.glrTrace {
		fmt.Printf("    MATERIALIZE skipped real gap into hidden ERROR: gap=%d..%d before tok=%d..%d\n",
			err.endByte, tok.StartByte, tok.StartByte, tok.EndByte)
	}
	return true
}

func trailingHiddenErrorLeaf(n *Node, symbolMeta []SymbolMetadata, end uint32) (*Node, bool) {
	if n == nil || len(n.children) == 0 {
		return nil, false
	}
	for i := len(n.children) - 1; i >= 0; i-- {
		child := n.children[i]
		if child == nil {
			continue
		}
		if symbolStructuralForHiddenFlattening(child.symbol, symbolMeta, nil) {
			if child.symbol == errorSymbol &&
				!child.isMissing() &&
				len(child.children) == 0 &&
				child.endByte == end {
				return child, true
			}
			if child.endByte == end && child.startByte == child.endByte {
				continue
			}
			return nil, false
		}
		if child.endByte > end || end < child.startByte {
			return nil, false
		}
		if err, ok := trailingHiddenErrorLeaf(child, symbolMeta, end); ok {
			return err, true
		}
		if child.endByte == end && child.startByte == child.endByte {
			continue
		}
		return nil, false
	}
	return nil, false
}

func replaceTrailingHiddenErrorLeaf(n *Node, symbolMeta []SymbolMetadata, oldEnd uint32, repl *Node, newEnd uint32, newEndPoint Point) bool {
	if n == nil || repl == nil || len(n.children) == 0 {
		return false
	}
	for i := len(n.children) - 1; i >= 0; i-- {
		child := n.children[i]
		if child == nil {
			continue
		}
		if symbolStructuralForHiddenFlattening(child.symbol, symbolMeta, nil) {
			if child.symbol == errorSymbol &&
				!child.isMissing() &&
				len(child.children) == 0 &&
				child.endByte == oldEnd {
				n.children[i] = repl
				extendHiddenErrorAncestorEnd(n, newEnd, newEndPoint)
				nodeBumpEquivVersion(n)
				return true
			}
			continue
		}
		if replaceTrailingHiddenErrorLeaf(child, symbolMeta, oldEnd, repl, newEnd, newEndPoint) {
			extendHiddenErrorAncestorEnd(n, newEnd, newEndPoint)
			nodeBumpEquivVersion(n)
			return true
		}
	}
	return false
}

func extendHiddenErrorAncestorEnd(n *Node, end uint32, point Point) {
	if n == nil || n.endByte > end {
		return
	}
	n.endByte = end
	n.endPoint = point
	n.setHasError(true)
}

// bytesAreParserPadding reports whether source[start:end] is entirely
// trivia a real parser skips silently: plain whitespace, backslash+newline
// (unconditional — see the field doc for why backslash needs no per-language
// gate), or, when continuationEscape is non-zero, that language-declared
// escape byte immediately followed by a newline (see
// Language.LineContinuationEscapeByte). Pass 0 for continuationEscape at any
// call site without a language-specific escape to declare; that reproduces
// this function's behavior before the parameter existed exactly.
func bytesAreParserPadding(source []byte, start, end uint32, continuationEscape byte) bool {
	if start > end || int(end) > len(source) {
		return false
	}
	i := int(start)
	if i == 0 && bytes.HasPrefix(source, utf8BOM) && int(end) >= len(utf8BOM) {
		i = len(utf8BOM)
	}
	for ; i < int(end); i++ {
		b := source[i]
		switch b {
		case ' ', '\t', '\n', '\r', '\f', '\v':
			continue
		case '\\':
			next := i + 1
			if next < int(end) && source[next] == '\n' {
				i = next
				continue
			}
			if next+1 < int(end) && source[next] == '\r' && source[next+1] == '\n' {
				i = next + 1
				continue
			}
			return false
		default:
			if continuationEscape != 0 && b == continuationEscape {
				next := i + 1
				if next < int(end) && source[next] == '\n' {
					i = next
					continue
				}
				if next+1 < int(end) && source[next] == '\r' && source[next+1] == '\n' {
					i = next + 1
					continue
				}
			}
			return false
		}
	}
	return true
}

// parserTailAllowsCleanAcceptance reports whether source[start:end] — the
// tail beyond an accepted stack's, or an accepted tree root's, own span — is
// entirely parser padding, so the caller may treat that tail as silently
// swallowed rather than a real, uncovered remainder. continuationEscape
// carries the same language-declared line-continuation escape byte
// bytesAreParserPadding accepts elsewhere (0 when the caller has no language
// in scope, or the language declares none): a trailing continuation escape
// immediately followed by a newline — for example a PowerShell file or
// incremental edit that ends mid-backtick-continuation — is exactly as much
// scanner-owned trivia at the tail as it is mid-file, and every caller here
// that reaches this function with a language in scope now passes its real
// escape byte instead of a hardcoded 0. Passing 0 for a language that HAS
// declared an escape (the historical shape of this function) is the actual
// defect this parameter closes: an accepted PowerShell stack with nothing but
// trailing padding used to still get declined here whenever
// materializeSkippedGapAsExtraError's spurious ERROR (removed for the
// backtick-continuation gap-classification fix, see
// realTokenAttachmentGapIsParserPadding) was gone and stackResultErrorRank
// read 0 — the accepted stack then genuinely reached this tail check, which,
// blind to the continuation escape, called an ordinary trailing
// backtick+newline a real tail and forced a degraded fallback despite the
// stack (and the C oracle) agreeing the parse was clean.
func parserTailAllowsCleanAcceptance(source []byte, start, end uint32, included []Range, continuationEscape byte) bool {
	if start >= end {
		return true
	}
	if start > end || int(end) > len(source) {
		return false
	}
	return bytesAreParserPaddingInIncludedRanges(source, start, end, included, continuationEscape)
}

// bytesAreParserPaddingInIncludedRanges reports whether source[start:end] is
// entirely parser padding once clipped to included. With no included ranges
// configured (included is empty, the overwhelming common case) this is
// exactly bytesAreParserPadding(source, start, end, continuationEscape): a
// caller with no active SetIncludedRanges call keeps its pre-clipping
// behavior unchanged. With included ranges configured, only the portions of
// [start,end) that actually fall inside an included range are scanned for
// padding; a gap between two included ranges is skipped entirely rather than
// scanned, because includedRangeTokenSource (included_ranges.go) already
// treats bytes outside every included range as invisible and never lexes
// them — this scan has to agree, or it ends up demanding that excluded,
// arbitrary source text be whitespace before the parser may cross it.
func bytesAreParserPaddingInIncludedRanges(source []byte, start, end uint32, included []Range, continuationEscape byte) bool {
	if len(included) == 0 {
		return bytesAreParserPadding(source, start, end, continuationEscape)
	}
	for _, r := range included {
		if r.EndByte <= start {
			continue
		}
		if r.StartByte >= end {
			break
		}
		overlapStart := start
		if r.StartByte > overlapStart {
			overlapStart = r.StartByte
		}
		overlapEnd := end
		if r.EndByte < overlapEnd {
			overlapEnd = r.EndByte
		}
		if overlapStart < overlapEnd && !bytesAreParserPadding(source, overlapStart, overlapEnd, continuationEscape) {
			return false
		}
	}
	return true
}

func cleanAcceptedStackSelectableAtEOF(source []byte, expectedEOFByte uint32, included []Range, arena *nodeArena, s *glrStack, continuationEscape byte) bool {
	if s == nil || !s.accepted {
		return false
	}
	if stackResultErrorRank(s, arena) != 0 {
		return true
	}
	return parserTailAllowsCleanAcceptance(source, s.byteOffset, expectedEOFByte, included, continuationEscape)
}

func cleanAcceptedTreeLeavesRealTail(tree *Tree, source []byte, expectedEOFByte uint32, included []Range, continuationEscape byte) bool {
	root := rawRootOrNil(tree)
	if root == nil || root.HasError() || root.EndByte() >= expectedEOFByte {
		return false
	}
	return !parserTailAllowsCleanAcceptance(source, root.EndByte(), expectedEOFByte, included, continuationEscape)
}

func (p *Parser) guardRealTokenAttachmentGap(source []byte, s *glrStack, tok Token, consumer string) bool {
	if realTokenAttachmentGapIsParserPadding(source, s, tok, p.included, p.lineContinuationEscapeByte()) {
		return true
	}
	if consumer == "" {
		consumer = "attachment"
	}
	if p != nil && p.glrTrace && s != nil {
		fmt.Printf("    KILL stale %s: stack_byte=%d tok=%d..%d gap=%q\n",
			consumer, s.byteOffset, tok.StartByte, tok.EndByte, string(source[s.byteOffset:tok.StartByte]))
	}
	if s != nil {
		s.dead = true
	}
	return false
}

func (p *Parser) guardRealShiftGap(source []byte, s *glrStack, tok Token) bool {
	return p.guardRealTokenAttachmentGap(source, s, tok, "shift")
}

// incrementalOldTreeMayCarryErrorCost decides the starting value of
// crecoveryCostCompetitionRelevant for a parse that may reuse subtrees from
// oldTree. Neither reuse nor oldTree set means an ordinary fresh full parse,
// which always starts clean (false), matching pre-existing behavior.
//
// When reuse is possible, the question is whether ANY subtree it could
// splice in already carries error/missing content. oldTree.root.hasError()
// answers that in O(1) as a conservative-when-set signal. It is the same
// cached, bottom-up-propagated bit that backs the public Node.HasError()
// API. IMPORTANT: the bit can be UNDER-SET. reconcileStaleHasErrorFlags
// (parser_result.go) is clear-only, and language normalizers call
// setHasError(false) on repaired regions, so a tree can report a clean
// root while a descendant is still IsMissing() (observed for python and
// wgsl). Do not treat a false root bit as an exactness proof.
//
// Why a false start stays safe despite under-set trees:
//  1. Reuse admission checks the per-node hasError()/zero-width gates
//     (incremental.go), so a MISSING leaf cannot be spliced directly.
//  2. The languages observed to produce under-set trees carry external
//     scanners that disable subtree reuse entirely, so their cost-bearing
//     content never enters this parse.
//  3. The pair-local backstop in glr.go (cost competition still runs when
//     either merge candidate is cPaused or has cRec != nil) guards the
//     residual case. Do not remove that guard on the strength of this
//     starting flag.
//
// A false result is therefore not "assume clean" -- it is "no reusable
// error content can enter, so let this pass's own explicit set-true sites
// (missing-token insertion, error-run leaves, resync recovery,
// cHandleError, ...) do exactly what they already do for a fresh full
// parse": the moment this parse constructs its own error/missing content,
// crecoveryCostCompetitionRelevant flips true from that call site, same as
// today. A true old tree (or a defensively unknown one) keeps the prior
// conservative behavior unchanged.
func incrementalOldTreeMayCarryErrorCost(reuse *reuseCursor, oldTree *Tree) bool {
	if reuse == nil && oldTree == nil {
		return false
	}
	if oldTree == nil || oldTree.root == nil {
		// Defensive: reuse should never be non-nil without a rooted oldTree
		// (reuseCursor.reset returns nil otherwise), but an unknown old tree
		// keeps the previously-conservative "assume relevant" answer.
		return true
	}
	return oldTree.root.hasError()
}

func compactPackedGSSActionCellRequiresTransaction(actions []ParseAction) bool {
	if len(actions) > 1 {
		return true
	}
	return len(actions) == 1 &&
		(actions[0].Type == ParseActionReduce ||
			(actions[0].Type == ParseActionShift && actions[0].Repetition))
}

func compactPackedGSSDispatchVersionCount(certified bool, roundVersionCount, currentVersionCount int) int {
	if certified {
		return currentVersionCount
	}
	return roundVersionCount
}

// parseInternal is the core GLR parsing loop shared by Parse and
// ParseWithTokenSource.
//
// It maintains a set of parse stacks. For unambiguous grammars (single
// action per table entry), there is exactly one stack and the algorithm
// reduces to standard LR parsing. When multiple actions exist for a
// (state, symbol) pair, the parser forks: one stack per alternative.
// Stacks that error out are dropped. Only duplicate stack versions are
// merged; distinct alternatives are preserved.
func (p *Parser) parseInternal(source []byte, ts TokenSource, reuse *reuseCursor, oldTree *Tree, arenaClass arenaClass, timing *incrementalParseTiming, maxStacksOverride int, maxNodesOverride int, maxMergePerKeyOverride int, deterministicExternalConflicts bool) *Tree {
	workCountAttempt := workCountBeginParseAttempt(maxStacksOverride, maxNodesOverride, maxMergePerKeyOverride)
	parseStart := time.Now()
	previousMemoryBudgetDiag := p.parseMemoryBudgetDiag
	previousMemoryBudgetDiagActive := p.parseMemoryBudgetDiagActive
	previousCompatMemoryBudgetTripped := p.compatMemoryBudgetTripped
	p.parseMemoryBudgetDiag = parseMemoryBudgetDiagnostic{}
	p.parseMemoryBudgetDiagActive = true
	p.compatMemoryBudgetTripped = false
	memoryBudgetDiag := &p.parseMemoryBudgetDiag
	defer func() {
		p.parseMemoryBudgetDiag = previousMemoryBudgetDiag
		p.parseMemoryBudgetDiagActive = previousMemoryBudgetDiagActive
		p.compatMemoryBudgetTripped = previousCompatMemoryBudgetTripped
	}()
	endParseBudget := p.enterParseBudgetAt(parseStart)
	defer endParseBudget()
	parseFlags := p.applyParseModeFlags(source, reuse, oldTree, arenaClass)
	defer p.restoreParseModeFlags(parseFlags)
	p.clearCurrentExternalTokenCheckpoint()
	p.resetNormalizationStats()
	p.resetPendingStackBuffersAtBoundary()
	if p.logger != nil {
		p.logf(ParserLogParse, "start len=%d incremental=%t", len(source), reuse != nil || oldTree != nil)
	}
	deferParentLinks := reuse == nil && oldTree == nil
	scratch := acquireParserScratch()
	transientReduceParents := p.configureParseScratch(scratch, source, reuse, oldTree, arenaClass, deferParentLinks)
	scratch.merge.gssOwner = &scratch.gss
	defer releaseParserScratch(scratch, deferParentLinks)
	p.reduceScratch = &scratch.reduce
	p.mergeScratch = &scratch.merge
	p.beginRecoveryRuntimeTelemetry()
	p.beginRecoveryRuntimeTelemetryDetailed()
	// budgetScratch is saved and restored, not just cleared, unlike its
	// siblings above: a nested parseInternal call (a retry or compat-
	// normalization sub-parse reachable from this call's own body, however
	// unlikely in practice) must leave the OUTER call's resultMaterializationStopReason
	// checks intact when the inner call returns. Clearing unconditionally to
	// nil would silently disable the scratch-budget check (parser_result.go)
	// for the rest of the outer parse instead of merely losing the inner
	// call's own coverage — a nested caller degrading to "no fix" is the
	// acceptable failure mode here, not the outer caller silently losing a
	// check several of its own materialization-boundary poll sites still
	// assume is active.
	prevBudgetScratch := p.budgetScratch
	p.budgetScratch = scratch
	p.goCompatFrames = &scratch.goCompatFrames
	if transientReduceParents {
		p.reduceScratch.transientParents = &scratch.transientParents
		p.reduceScratch.transientChildren = &scratch.transientChildren
	}
	defer func() {
		p.reduceScratch = nil
		p.mergeScratch = nil
		p.budgetScratch = prevBudgetScratch
		p.goCompatFrames = nil
		if p.cCondenseVersionKeyRanks != nil {
			clear(p.cCondenseVersionKeyRanks)
		}
		if p.cNodeMemoOperationDepth == 0 {
			p.finishCNodeMemoParse()
		}
	}()
	scratch.audit.beginParse()
	scratch.merge.audit = nil
	scratch.gss.audit = nil
	if p.cNodeMemoOperationDepth == 0 {
		p.cNodeMemoOperationPeakTier = RecoveryNodeMemoTierNone
		if cold := p.forestDeclineMemo; cold != nil {
			cold.cNodeMemoCollisions = 0
		}
	}
	cNodeMemoCollisionStart := p.cNodeMemoCollisionCount()
	p.cNodeMemoPeakTier = RecoveryNodeMemoTierNone
	// Reset all parse-episode recovery gates before applying this pass's
	// conservative starting state. This also protects parser reuse when the
	// recovery feature is disabled for one pass.
	p.resetCRecoveryCostCompetitionState()
	if p.errorCostCompetitionEnabled() {
		// Faithful C recovery port: arena nodes are pooled across parses, so
		// stale (pointer, version) memo hits from a previous parse must be
		// impossible. Advance the parser-owned memo epoch instead of clearing a
		// cache that may have grown to 16K entries. Start at the small default --
		// cCompareVersions
		// cost competition participates in ordinary GLR disambiguation on
		// well-formed input too, so this path is warm even on clean parses;
		// growCNodeMemoCache upgrades it to the full working-set size the
		// first time this parse actually enters C error handling, or the
		// first time this parse's own memo-set contention crosses
		// cNodeMemoThrashGrowThreshold (cNodeMemoSlot) -- either way, reset
		// the contention counter here so a parse's grow decision depends only
		// on ITS OWN observed load, never on carryover from an earlier,
		// unrelated parse on this same Parser instance (determinism, #380/#388).
		if len(p.cNodeMemoCache) == 0 {
			p.cNodeMemoCache = make([]cNodeMemoCacheEntry, cNodeMemoCacheInitialSize)
		}
		p.cNodeMemoThrash = 0
		p.cNodeMemoPeakTier = recoveryNodeMemoTierForEntries(len(p.cNodeMemoCache))
		if p.cNodeMemoPeakTier > p.cNodeMemoOperationPeakTier {
			p.cNodeMemoOperationPeakTier = p.cNodeMemoPeakTier
		}
		p.beginCNodeMemoEpoch()
		p.crecoveryEnteredErrorState = false
		p.crecoveryDroppedErrorForClean = false
		p.crecoveryReductionCandidateCeilingHits = 0
		p.crecoveryMissingTokenCeilingHits = 0
		p.crecoveryReductionCandidateAttemptsPeak = 0
		p.crecoveryMissingTokenTrialAttemptsPeak = 0
		// Cost walks stay gated off until this pass proves costs can be
		// nonzero. A fresh full parse starts clean (false). An incremental
		// parse over an old tree whose root bit is clean starts clean too.
		// The root bit can be under-set (see the safety chain documented on
		// incrementalOldTreeMayCarryErrorCost: per-node reuse-admission
		// gates + reuse-disabling scanners + the glr.go cPaused/cRec
		// backstop keep that safe), and any error this pass constructs on
		// its own goes through the same explicit set-true sites a fresh
		// parse uses. Only an old tree that is itself known to have
		// error/missing content anywhere starts the conservative true
		// (reused subtrees may carry it in without a new pause this pass).
		initialCostRelevant := incrementalOldTreeMayCarryErrorCost(reuse, oldTree)
		p.crecoveryCostCompetitionRelevant = initialCostRelevant
		p.crecoveryCostCompetitionWalkEnabled = initialCostRelevant
		p.cRecoverSharedTokenErrorModeLexed = false
		p.cRecoverCustomResyncActive = false
		p.cRecoverCustomResyncByte = 0
	}
	p.syncCRecoveryMergeScratch(p.mergeScratch)
	// Fresh full parses defer parent links and start with no error-bearing
	// payload. Besides controlling parent metadata propagation, this is a
	// sticky proof consumed by C-recovery condense: every path that inserts an
	// ERROR or MISSING payload must flip it before the next condense pass.
	// Incremental/reuse parses start true because old subtrees may carry errors.
	scratch.trackChildErrors = !deferParentLinks
	trackChildErrors := &scratch.trackChildErrors
	scratch.merge.childErrors = trackChildErrors

	arena := acquireNodeArena(arenaClass)
	arena.skipChildClear = reuse == nil && oldTree == nil
	arena.finalChildRefs = p.finalChildRefs
	arena.audit = nil
	scratch.merge.arena = arena
	scratch.merge.parser = p
	if scratch.audit.enabled || scratch.audit.equivEnabled {
		scratch.merge.audit = &scratch.audit
	}
	if scratch.audit.enabled {
		scratch.gss.audit = &scratch.audit
		arena.audit = &scratch.audit
	}
	if timing != nil {
		startUsed := arena.used
		defer func() {
			timing.totalNanos += time.Since(parseStart).Nanoseconds()
			if arena.used >= startUsed {
				timing.newNodes += uint64(arena.used - startUsed)
			}
			peak := uint64(scratch.entries.peakEntriesUsed())
			if peak > timing.entryScratchPeak {
				timing.entryScratchPeak = peak
			}
		}()
	}
	var materializationTiming parseMaterializationTiming
	var materializationTimingRef *parseMaterializationTiming
	if timing != nil || parseShouldCaptureMaterializationTiming(p, source, reuse, oldTree, arenaClass) || (p != nil && p.noTreeBenchmarkOnly && (parseReduceTimingEnabled() || parseActionTimingEnabled())) {
		materializationTimingRef = &materializationTiming
	}
	phaseTiming := materializationTimingRef != nil
	var actionTiming *parseMaterializationTiming
	if materializationTimingRef != nil && parseActionTimingEnabled() {
		actionTiming = materializationTimingRef
	}
	recordActionTiming := func(state StateID, lookahead Symbol, actions []ParseAction, kind ambiguityActionTimingKind, nanos int64) {
		if nanos <= 0 || p == nil || p.ambiguityProfile == nil {
			return
		}
		p.ambiguityProfile.recordActionTiming(state, lookahead, actions, kind, nanos)
	}
	var parserLoopNanos int64
	var tokenNextNanos int64
	var actionDispatchNanos int64
	var actionLookupNanos int64
	var glrMergeNanos int64
	var glrCullNanos int64
	type terminalFrontierAction struct {
		index  int
		action ParseAction
	}
	var terminalFrontierScratch [maxGLRStacks]terminalFrontierAction
	prevMaterializationTiming := p.materializationTiming
	prevReduceTiming := p.reduceTiming
	p.materializationTiming = materializationTimingRef
	if materializationTimingRef != nil && parseReduceTimingEnabled() {
		p.reduceTiming = materializationTimingRef
	} else {
		p.reduceTiming = nil
	}
	defer func() {
		p.materializationTiming = prevMaterializationTiming
		p.reduceTiming = prevReduceTiming
	}()
	defer p.recordParseArenaUsageOnReturn(arenaClass, arena, scratch)()
	p.ensureParseInitialCapacity(source, arenaClass, arena, scratch)
	memoryBudget := parseMemoryBudgetForParser(p, len(source))
	arena.setBudget(memoryBudget)
	arena.setExternalScannerCheckpointIdentityForLanguage(p.language)
	scratch.setBudget(memoryBudget)
	restoreRuntimeMemoryBudget := p.enterRuntimeMemoryBudget(memoryBudget, len(source))
	if restoreRuntimeMemoryBudget.parser != nil {
		defer restoreRuntimeMemoryBudget.restore()
	}
	var reuseState parseReuseState
	nodeCount := 0
	iterationsUsed := 0
	peakStackDepth := 0
	maxStacksSeen := 0
	var perfTokensConsumed uint64
	var lastTokenEndByte uint32
	var lastTokenSymbol Symbol
	var lastTokenWasEOF bool
	var tok Token
	tokenSourceEOFEarly := false
	singleStackIterations := 0
	multiStackIterations := 0
	var singleStackTokens uint64
	var multiStackTokens uint64
	preMaterializationDiag := parsePreMaterializationDiagEnabled()
	var preMaterializationFieldRejectCandidates uint64
	var preMaterializationFieldRejectSameKeyCandidates uint64
	var preMaterializationFieldRejectOverflowCandidates uint64
	expectedEOFByte := uint32(len(source))
	if len(p.included) > 0 {
		expectedEOFByte = p.included[len(p.included)-1].EndByte
	}
	progress := newParseProgressTelemetry(p, len(source), expectedEOFByte, parseStart)
	if progress.enabled {
		progress.emit(time.Now(), "start", 0, 0, Token{}, false, nil, 0, 0, 0, true, 0, 0, "")
	}
	var stopActionDiag parseStopActionDiagnostic
	var stopActionDiagRef *parseStopActionDiagnostic
	if !p.noResultCompatibilityBenchmarkOnly {
		stopActionDiagRef = &stopActionDiag
	}
	prevStopActionDiag := p.stopActionDiag
	p.stopActionDiag = stopActionDiagRef
	defer func() {
		p.stopActionDiag = prevStopActionDiag
	}()
	var stacks []glrStack
	var dispatchTrace []dispatchStackSurvivorTrace
	var pendingTraceOrigin string
	var pendingTraceSourceIndex int
	var pendingTraceActionIndex int
	var pendingTraceActionCount int
	var pendingTraceAction ParseAction
	drainPendingForkStacks := func() {
		if !faithfulCapOneMergeEnabled(p.mergeScratch) || len(p.pendingForkStacks) == 0 {
			return
		}
		pendingCount := len(p.pendingForkStacks)
		workCountRecordPendingTransition(p, &p.pendingForkStacks[0], pendingCount, 0, workCountConvergenceOutcomePendingDrained, "post-reduce pending queue drained into live frontier")
		if progress.enabled {
			for i := range p.pendingForkStacks {
				pending := &p.pendingForkStacks[i]
				state, byteOffset, depth := stackTraceState(pending)
				dispatchTrace = append(dispatchTrace, dispatchStackSurvivorTrace{
					inSnapshot:          false,
					visited:             false,
					origin:              "pending-reduce-fork",
					sourceIndex:         pendingTraceSourceIndex,
					path:                pendingTraceOrigin,
					actionIndex:         pendingTraceActionIndex,
					actionCount:         pendingTraceActionCount,
					actionType:          pendingTraceAction.Type,
					actionState:         pendingTraceAction.State,
					actionSymbol:        pendingTraceAction.Symbol,
					actionChildCount:    pendingTraceAction.ChildCount,
					afterPrimaryState:   state,
					afterPrimaryByte:    byteOffset,
					afterPrimaryDepth:   depth,
					afterPrimaryShifted: pending.shifted,
					afterPrimaryDead:    pending.dead,
					afterPrimaryAccept:  pending.accepted,
					afterFinalState:     state,
					afterFinalByte:      byteOffset,
					afterFinalDepth:     depth,
					afterFinalShifted:   pending.shifted,
					afterFinalDead:      pending.dead,
					afterFinalAccepted:  pending.accepted,
				})
			}
		}
		stacks = append(stacks, p.pendingForkStacks...)
		p.pendingForkStacks = p.pendingForkStacks[:0]
	}
	drainPendingFrontierForkStacks := func() {
		if len(p.pendingFrontierForkStacks) == 0 {
			return
		}
		pendingCount := len(p.pendingFrontierForkStacks)
		workCountRecordPendingTransition(p, &p.pendingFrontierForkStacks[0], pendingCount, 0, workCountConvergenceOutcomePendingDrained, "frontier pending queue drained into live frontier")
		if progress.enabled {
			for i := range p.pendingFrontierForkStacks {
				pending := &p.pendingFrontierForkStacks[i]
				state, byteOffset, depth := stackTraceState(pending)
				dispatchTrace = append(dispatchTrace, dispatchStackSurvivorTrace{
					inSnapshot:          false,
					visited:             false,
					origin:              "pending-frontier-fork",
					sourceIndex:         pendingTraceSourceIndex,
					path:                pendingTraceOrigin,
					actionIndex:         pendingTraceActionIndex,
					actionCount:         pendingTraceActionCount,
					actionType:          pendingTraceAction.Type,
					actionState:         pendingTraceAction.State,
					actionSymbol:        pendingTraceAction.Symbol,
					actionChildCount:    pendingTraceAction.ChildCount,
					afterPrimaryState:   state,
					afterPrimaryByte:    byteOffset,
					afterPrimaryDepth:   depth,
					afterPrimaryShifted: pending.shifted,
					afterPrimaryDead:    pending.dead,
					afterPrimaryAccept:  pending.accepted,
					afterFinalState:     state,
					afterFinalByte:      byteOffset,
					afterFinalDepth:     depth,
					afterFinalShifted:   pending.shifted,
					afterFinalDead:      pending.dead,
					afterFinalAccepted:  pending.accepted,
				})
			}
		}
		stacks = append(stacks, p.pendingFrontierForkStacks...)
		p.pendingFrontierForkStacks = p.pendingFrontierForkStacks[:0]
	}
	parseRuntime := ParseRuntime{
		StopReason:        ParseStopNone,
		SourceLen:         uint32(len(source)),
		ExpectedEOFByte:   expectedEOFByte,
		MemoryBudgetBytes: arena.budgetBytes,
	}
	stopDiagHaveStack := false
	stopDiagHaveToken := false
	stopDiagLastStackState := StateID(0)
	stopDiagLastStackByte := uint32(0)
	stopDiagLastStackDepth := 0
	stopDiagToken := Token{}
	stopDiagRecoverActionAvailable := false
	stopDiagFrontier := stopFrontierDiagnostics{}
	stopDiagCondenseGating := ""
	stopDiagCondenseErrorCostEnabled := false
	stopDiagCondenseAnyReduced := false
	stopDiagCondenseRelevant := false
	stopDiagCondenseRan := false
	stopDiagCondenseResumed := false
	noteStopDiagnosticStack := func(s *glrStack) {
		if s == nil || s.dead || s.accepted || s.depth() == 0 {
			return
		}
		stopDiagHaveStack = true
		stopDiagLastStackState = s.top().state
		stopDiagLastStackByte = s.byteOffset
		stopDiagLastStackDepth = s.depth()
	}
	noteStopDiagnosticStacks := func(stacks []glrStack) {
		for i := range stacks {
			noteStopDiagnosticStack(&stacks[i])
		}
	}
	noteStopDiagnosticToken := func(t Token) {
		stopDiagHaveToken = true
		stopDiagToken = t
	}
	recordCurrentLookahead := func(t Token) {
		noteStopDiagnosticToken(t)
		lastTokenEndByte = t.EndByte
		lastTokenSymbol = t.Symbol
		lastTokenWasEOF = t.Symbol == 0 && t.StartByte == t.EndByte && !t.NoLookahead
		if lastTokenWasEOF && t.EndByte < expectedEOFByte {
			tokenSourceEOFEarly = true
		}
	}
	computeStopDiagnosticRecoverAction := func() bool {
		if stopDiagRecoverActionAvailable || !stopDiagHaveToken {
			return stopDiagRecoverActionAvailable
		}
		for i := range stacks {
			s := &stacks[i]
			if s.accepted || s.depth() == 0 {
				continue
			}
			if _, _, ok := p.findRecoverActionOnStack(s, stopDiagToken.Symbol, nil); ok {
				stopDiagRecoverActionAvailable = true
				break
			}
		}
		return stopDiagRecoverActionAvailable
	}
	recordStopDiagnostic := func(reason ParseStopReason, tree *Tree) {
		if reason != ParseStopNoStacksAlive && reason != ParseStopIterationLimit {
			return
		}
		parseRuntime.StopDiagnosticCaptured = true
		parseRuntime.StopDiagnosticCRecoveryEnabled = p.errorCostCompetitionEnabled()
		if !parseRuntime.StopDiagnosticCRecoveryEnabled {
			parseRuntime.StopDiagnosticCRecoveryGateReason = p.cRecoveryGateReason()
		}
		parseRuntime.StopDiagnosticRecoverActionAvailable = computeStopDiagnosticRecoverAction()
		if stopDiagHaveStack {
			parseRuntime.StopDiagnosticLastStackState = stopDiagLastStackState
			parseRuntime.StopDiagnosticLastStackByte = stopDiagLastStackByte
			parseRuntime.StopDiagnosticLastStackDepth = stopDiagLastStackDepth
		}
		if stopDiagHaveToken {
			parseRuntime.StopDiagnosticTokenSymbol = stopDiagToken.Symbol
			parseRuntime.StopDiagnosticTokenStartByte = stopDiagToken.StartByte
			parseRuntime.StopDiagnosticTokenEndByte = stopDiagToken.EndByte
			parseRuntime.StopDiagnosticTokenNoLookahead = stopDiagToken.NoLookahead
		}
		root := rawRootOrNil(tree)
		if root == nil {
			return
		}
		parseRuntime.StopDiagnosticRootType = root.Type(p.language)
		parseRuntime.StopDiagnosticRootStartByte = root.StartByte()
		parseRuntime.StopDiagnosticRootEndByte = root.EndByte()
		parseRuntime.StopDiagnosticRootHasError = root.HasError()
		if errNode := firstErrorNode(root); errNode != nil {
			parseRuntime.StopDiagnosticFirstErrorFound = true
			parseRuntime.StopDiagnosticFirstErrorStartByte = errNode.StartByte()
			parseRuntime.StopDiagnosticFirstErrorEndByte = errNode.EndByte()
		}
		parseRuntime.StopDiagnosticFrontierStacks = stopDiagFrontier.stackTable
		parseRuntime.StopDiagnosticFrontierActions = stopDiagFrontier.actions
		parseRuntime.StopDiagnosticSameHeaderGroups = stopDiagFrontier.sameHeader
		if stopDiagCondenseGating == "" {
			stopDiagCondenseGating = stopCondenseGatingString(stopDiagCondenseErrorCostEnabled, stopDiagCondenseAnyReduced, stopDiagCondenseRelevant, stopDiagCondenseRan, stopDiagCondenseResumed, stacks)
		}
		parseRuntime.StopDiagnosticCondenseGating = stopDiagCondenseGating
		if stopActionDiag.captured {
			parseRuntime.StopDiagnosticActionCaptured = true
			parseRuntime.StopDiagnosticActionPhase = stopActionDiag.phase
			parseRuntime.StopDiagnosticActionStackState = stopActionDiag.stackState
			parseRuntime.StopDiagnosticActionStackByte = stopActionDiag.stackByte
			parseRuntime.StopDiagnosticActionStackDepth = stopActionDiag.stackDepth
			parseRuntime.StopDiagnosticActionTokenSymbol = stopActionDiag.lookaheadSymbol
			parseRuntime.StopDiagnosticActionTokenStartByte = stopActionDiag.lookaheadStartByte
			parseRuntime.StopDiagnosticActionTokenEndByte = stopActionDiag.lookaheadEndByte
			parseRuntime.StopDiagnosticActionTokenNoLookahead = stopActionDiag.lookaheadNoLookahead
			parseRuntime.StopDiagnosticActionType = stopActionDiag.actionType
			parseRuntime.StopDiagnosticActionState = stopActionDiag.actionState
			parseRuntime.StopDiagnosticActionSymbol = stopActionDiag.actionSymbol
			parseRuntime.StopDiagnosticActionChildCount = stopActionDiag.actionChildCount
			parseRuntime.StopDiagnosticActionProductionID = stopActionDiag.actionProductionID
			parseRuntime.StopDiagnosticActionDynamicPrecedence = stopActionDiag.actionDynamicPrecedence
			parseRuntime.StopDiagnosticActionCount = stopActionDiag.actionCount
			parseRuntime.StopDiagnosticActionResultState = stopActionDiag.resultState
			parseRuntime.StopDiagnosticActionInReduceChain = stopActionDiag.inReduceChain
			parseRuntime.StopDiagnosticActionReduceChainStep = stopActionDiag.reduceChainStep
			parseRuntime.StopDiagnosticActionRepeatedSignatureCount = stopActionDiag.repeatedReduceSignatureCount
			parseRuntime.StopDiagnosticActionReduceChainCycle = stopActionDiag.reduceChainCycle
			parseRuntime.StopDiagnosticActionForceAdvanceAfterReduce = stopActionDiag.forceAdvanceAfterReduce
			parseRuntime.StopDiagnosticActionPostDispatchDefaultReduce = stopActionDiag.postDispatchDefaultReduced
			parseRuntime.StopDiagnosticActionAnyReduced = stopActionDiag.anyReduced
			parseRuntime.StopDiagnosticActionConsumedToken = stopActionDiag.consumedToken
			parseRuntime.StopDiagnosticActionDispatchShiftActions = stopActionDiag.dispatchShiftActions
			parseRuntime.StopDiagnosticActionDispatchReduceActions = stopActionDiag.dispatchReduceActions
			parseRuntime.StopDiagnosticActionDispatchAcceptActions = stopActionDiag.dispatchAcceptActions
			parseRuntime.StopDiagnosticActionDispatchRecoverActions = stopActionDiag.dispatchRecoverActions
			parseRuntime.StopDiagnosticActionDispatchOtherActions = stopActionDiag.dispatchOtherActions
			parseRuntime.StopDiagnosticLastReduceCaptured = stopActionDiag.lastReduceCaptured
			parseRuntime.StopDiagnosticLastReducePhase = stopActionDiag.lastReducePhase
			parseRuntime.StopDiagnosticLastReduceStackState = stopActionDiag.lastReduceStackState
			parseRuntime.StopDiagnosticLastReduceStackByte = stopActionDiag.lastReduceStackByte
			parseRuntime.StopDiagnosticLastReduceStackDepth = stopActionDiag.lastReduceStackDepth
			parseRuntime.StopDiagnosticLastReduceSymbol = stopActionDiag.lastReduceSymbol
			parseRuntime.StopDiagnosticLastReduceChildCount = stopActionDiag.lastReduceChildCount
			parseRuntime.StopDiagnosticLastReduceProductionID = stopActionDiag.lastReduceProductionID
			parseRuntime.StopDiagnosticLastReduceDynamicPrecedence = stopActionDiag.lastReduceDynamicPrecedence
			parseRuntime.StopDiagnosticLastReduceResultState = stopActionDiag.lastReduceResultState
			parseRuntime.StopDiagnosticLastReduceInChain = stopActionDiag.lastReduceInChain
			parseRuntime.StopDiagnosticLastReduceChainStep = stopActionDiag.lastReduceChainStep
			parseRuntime.StopDiagnosticLastReduceRepeatedSigCount = stopActionDiag.lastReduceRepeatedSigCount
			parseRuntime.StopDiagnosticLastReduceChainCycle = stopActionDiag.lastReduceChainCycle
		}
	}
	arenaStatsCaptured := false
	var arenaBreakdown *ArenaBreakdown
	scratchStatsCaptured := false
	captureArenaStats := func() {
		if arenaStatsCaptured {
			return
		}
		if captureParseArenaStats(&parseRuntime, arena, &arenaBreakdown, preMaterializationFieldRejectCandidates, preMaterializationFieldRejectSameKeyCandidates, preMaterializationFieldRejectOverflowCandidates) {
			arenaStatsCaptured = true
		}
	}
	captureScratchStats := func() {
		if scratchStatsCaptured {
			return
		}
		if captureParseScratchStats(&parseRuntime, scratch, arena, &arenaBreakdown) {
			scratchStatsCaptured = true
		}
	}
	errorTreeWithOwnedArena := func(reason ParseStopReason) *Tree {
		tree := parseErrorTreeWithArena(source, p.language, arena)
		tree.setParseStopReason(reason)
		return tree
	}
	replaceWithOwnedErrorTree := func(tree *Tree, reason ParseStopReason) *Tree {
		if tree != nil {
			if tree.arena == arena {
				tree.arena = nil
			}
			tree.Release()
		}
		return errorTreeWithOwnedArena(reason)
	}
	finalizeTree := func(tree *Tree, stopReason ParseStopReason) *Tree {
		if workCountInstrumentationEnabled && workCountAttemptTraceActive() { // work-count-assembly: finalize-defer guard
			workCountBeginFinalizeParseAttempt(workCountAttempt)
			defer func() {
				workCountEndFinalizeParseAttempt(workCountAttempt, stopReason, tree)
			}()
		}
		if phaseTiming && parserLoopNanos == 0 {
			parserLoopNanos = time.Since(parseStart).Nanoseconds()
		}
		if !resultMaterializationShouldStop(stopReason) {
			if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
				stopReason = reason
			}
		}
		if p.transientReduceChildren && tree != nil {
			if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
				stopReason = reason
				tree = replaceWithOwnedErrorTree(tree, reason)
			} else {
				materializeStart := time.Time{}
				if materializationTimingRef != nil {
					materializeStart = time.Now()
				}
				if reason := p.materializeTransientChildrenForReturnedTree(tree, arena, scratch); resultMaterializationShouldStop(reason) {
					stopReason = reason
					tree = replaceWithOwnedErrorTree(tree, reason)
				}
				if materializationTimingRef != nil {
					materializationTimingRef.transientChildMaterializationNanos += time.Since(materializeStart).Nanoseconds()
				}
			}
		}
		if stopReason == ParseStopAccepted && cleanAcceptedTreeLeavesRealTail(tree, source, expectedEOFByte, p.included, p.lineContinuationEscapeByte()) {
			stopReason = ParseStopNoStacksAlive
			tree = replaceWithOwnedErrorTree(tree, stopReason)
		}
		// C-recovery wrap/unwrap cycles can leave stale hasError bits on
		// ancestors whose error content resolved losslessly (see
		// reconcileStaleHasErrorFlags). Repair before the runtime is stamped so
		// the retry ladder and callers see C-derived truth. Gated to passes
		// where the recovery machinery actually ran; the walk itself only
		// happens when the root still claims an error.
		//
		// SAFETY: only for ACCEPTED, EOF-covering trees. On a truncated or
		// stopped-early tree "no ERROR node inside" does not mean "no error"
		// — the error can be exactly the reason the parse stopped (the
		// _pydecimal.py swallowed-error class: Go truncates at 64K with
		// hasError=false where C reports hasError=true). Clearing there
		// would widen that class; an accepted tree spanning expected EOF
		// with zero ERROR/MISSING descendants is the only case where
		// hasError=false is definitionally C-correct.
		if tree != nil && p.crecoveryEnteredErrorState && stopReason == ParseStopAccepted {
			if root := tree.root; root != nil && root.hasError() && root.endByte >= expectedEOFByte {
				if reconcileStaleHasErrorFlags(root, 0) {
					tree.resultErrorSummary = resultErrorSummaryPresent
				} else {
					tree.resultErrorSummary = resultErrorSummaryClean
				}
			}
		}
		// Env-gated (GOT_DEBUG_RECOVERY_INCREMENTAL_COST=1) one-line summary of the
		// incremental cost/vis aggregate assertions run this parse; no-op when unset.
		debugRecoveryIncrementalCostReport()
		scratch.audit.finishParse(stacks)
		captureArenaStats()
		captureScratchStats()
		stopReason = rescueMaterializationStopReason(stopReason, tree)
		parseRuntime.StopReason = parseStopReasonWithTokenSourceEOF(stopReason, tokenSourceEOFEarly)
		parseRuntime.MemoryBudgetStopSource = memoryBudgetDiag.source
		parseRuntime.RuntimeHeapGrowthBytes = memoryBudgetDiag.runtimeHeapGrowthBytes
		parseRuntime.RuntimeSysGrowthBytes = memoryBudgetDiag.runtimeSysGrowthBytes
		parseRuntime.CRecoveryEnteredErrorState = p.crecoveryEnteredErrorState
		parseRuntime.CRecoveryDroppedErrorForClean = p.crecoveryDroppedErrorForClean
		memoRuntime := RecoveryNodeMemoRuntime{PeakTier: p.cNodeMemoPeakTier}
		collisions := p.cNodeMemoCollisionCount() - cNodeMemoCollisionStart
		if collisions > uint64(^uint32(0)) {
			memoRuntime.Collisions = ^uint32(0)
		} else {
			memoRuntime.Collisions = uint32(collisions)
		}
		parseRuntime.CRecoverReductionCandidateCeilingHits = p.crecoveryReductionCandidateCeilingHits
		parseRuntime.CRecoverMissingTokenCeilingHits = p.crecoveryMissingTokenCeilingHits
		parseRuntime.CRecoverReductionCandidateAttemptsPeak = p.crecoveryReductionCandidateAttemptsPeak
		parseRuntime.CRecoverMissingTokenTrialAttemptsPeak = p.crecoveryMissingTokenTrialAttemptsPeak
		recordParseRuntimeLoopStats(&parseRuntime, scratch, iterationsUsed, nodeCount, peakStackDepth, maxStacksSeen, singleStackIterations, multiStackIterations, singleStackTokens, multiStackTokens)
		recordParseRuntimePhaseTiming(&parseRuntime, materializationTimingRef, parseStart, parserLoopNanos, tokenNextNanos, actionDispatchNanos, actionLookupNanos, glrMergeNanos, glrCullNanos)
		recordParseRuntimeMaterializationTiming(&parseRuntime, materializationTimingRef, materializationTiming)
		recordParseRuntimeTokenStats(&parseRuntime, perfTokensConsumed, lastTokenEndByte, lastTokenSymbol, lastTokenWasEOF, tokenSourceEOFEarly)
		recordParseRuntimeRootStats(&parseRuntime, tree, source, expectedEOFByte, p.included, scratch.audit.enabled || (arena != nil && arena.breakdownEnabled), p.language)
		p.finishRecoveryRuntimeTelemetry(tree, stacks)
		p.finishRecoveryRuntimeTelemetryDetailed(tree, &parseRuntime)
		recordStopDiagnostic(parseRuntime.StopReason, tree)
		p.copyNormalizationStats(&parseRuntime)
		if tree != nil {
			tree.setIncludedRanges(p.included)
			tree.setParseRuntime(parseRuntime)
			tree.setRecoveryNodeMemoRuntime(memoRuntime)
			if arenaBreakdown != nil {
				tree.setArenaBreakdown(arenaBreakdown)
			}
		}
		copyParseRuntimeToTiming(timing, parseRuntime)
		if p.logger != nil {
			p.logf(
				ParserLogParse,
				"stop reason=%s truncated=%t tokens=%d max_stacks=%d",
				parseRuntime.StopReason,
				parseRuntime.Truncated,
				parseRuntime.TokensConsumed,
				parseRuntime.MaxStacksSeen,
			)
		}
		if progress.enabled {
			progress.emit(
				time.Now(),
				"stop",
				iterationsUsed,
				perfTokensConsumed,
				Token{},
				false,
				stacks,
				maxStacksSeen,
				nodeCount,
				peakStackDepth,
				false,
				singleStackIterations,
				multiStackIterations,
				fmt.Sprintf("stop_reason=%s last_token_symbol=%d last_token_end=%d last_token_eof=%t", parseRuntime.StopReason, lastTokenSymbol, lastTokenEndByte, lastTokenWasEOF),
			)
		}
		// Raw reduction shapes have served their final consumers by this point:
		// result selection, transient materialization, diagnostics, and arena
		// attribution are all complete. Do this after captureArenaStats so the
		// returned runtime/breakdown continue to describe parse-time allocation.
		arena.reclaimRawShapeStorage()
		resetGSSPrefixPath(&p.cPrefixPath)
		return tree
	}
	finalize := func(treeStacks []glrStack, stopReason ParseStopReason) *Tree {
		if phaseTiming && parserLoopNanos == 0 {
			parserLoopNanos = time.Since(parseStart).Nanoseconds()
		}
		workCountRecordFinalPendingDiscards(p, p.pendingForkStacks, p.pendingFrontierForkStacks)
		if p.noTreeBenchmarkOnly {
			rootEndByte := expectedEOFByte
			if stopReason != ParseStopAccepted && stopReason != ParseStopNone {
				rootEndByte = lastTokenEndByte
			}
			tree := p.buildNoTreeBenchmarkResult(source, arena, rootEndByte)
			return finalizeTree(tree, stopReason)
		}
		if len(treeStacks) == 0 {
			captureArenaStats()
		}
		tree := p.buildResultFromGLR(
			treeStacks,
			source,
			arena,
			oldTree,
			&reuseState,
			&scratch.nodeLinks,
			scratch.reduce.transientParents,
			scratch.reduce.transientChildren,
			!*trackChildErrors,
			materializationTimingRef,
		)
		if tree != nil && !parseStopReasonIsTerminal(stopReason) {
			if reason := tree.rawParseStopReason(); parseStopReasonIsTerminal(reason) {
				stopReason = reason
			}
		}
		return finalizeTree(tree, stopReason)
	}
	finalizeErrorTree := func(stopReason ParseStopReason) *Tree {
		if phaseTiming && parserLoopNanos == 0 {
			parserLoopNanos = time.Since(parseStart).Nanoseconds()
		}
		return finalizeTree(errorTreeWithOwnedArena(stopReason), stopReason)
	}
	finalizeRecoveredNodes := func(nodes []*Node) *Tree {
		if phaseTiming && parserLoopNanos == 0 {
			parserLoopNanos = time.Since(parseStart).Nanoseconds()
		}
		if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
			return finalizeTree(errorTreeWithOwnedArena(reason), reason)
		}
		if reason := materializeTransientParentNodes(nodes, arena, scratch.reduce.transientParents, scratch.reduce.transientChildren, p); resultMaterializationShouldStop(reason) {
			return finalizeTree(errorTreeWithOwnedArena(reason), reason)
		}
		if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
			return finalizeTree(errorTreeWithOwnedArena(reason), reason)
		}
		if invariantTree := recoveredResultInvariantErrorTree(nodes, source, p.language, arena); invariantTree != nil {
			return finalizeTree(invariantTree, ParseStopInvariantViolation)
		}
		tree := p.buildResultFromNodes(nodes, source, arena, oldTree, &reuseState, &scratch.nodeLinks)
		if root := rawRootOrNil(tree); root != nil {
			normalizeSQLRecoveredMissingNull(root, arena, p.language)
			for _, child := range root.children {
				trimRecoveryWhitespaceTail(child, source)
			}
			wireParentLinksWithScratch(root, &scratch.nodeLinks)
		}
		return finalizeTree(tree, ParseStopAccepted)
	}
	tryFinalizeTrailingEOFSuffix := func(s *glrStack, tok Token) (*Tree, bool) {
		if p.noTreeBenchmarkOnly {
			return nil, false
		}
		nodes, ok := p.tryRecoverTrailingEOFSuffix(s, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, source)
		if !ok {
			return nil, false
		}
		return finalizeRecoveredNodes(nodes), true
	}

	stacks, maxStacksSeen = p.newInitialParseStacks(scratch, reuse, timing, len(source))
	if workCountInstrumentationEnabled && len(stacks) != 0 {
		workCountTopologyRecordInitialVersion(&stacks[0]) // work-count-assembly: topology initial-version seam
	}
	caps := p.configureParseCaps(source, reuse, arenaClass, scratch, maxStacksOverride, maxNodesOverride, maxMergePerKeyOverride)
	workCountResolveParseAttempt(
		workCountAttempt,
		caps.maxStacks,
		caps.retryPass,
		caps.mergePerKeyCap,
		caps.maxStackCullTrigger,
		caps.maxIter,
		caps.maxNodes,
	)
	maxStacks := caps.maxStacks
	retryPass := caps.retryPass
	mergePerKeyCap := caps.mergePerKeyCap
	maxStackCullTrigger := caps.maxStackCullTrigger
	maxIter := caps.maxIter
	maxDepth := caps.maxDepth
	maxNodes := caps.maxNodes
	// Select the larger of the resolved cull trigger and full-parse overflow window.
	// Keep its historical zero-cap rule only when the trigger does not exceed
	// maxStacks.
	frontierForkPopulationCap := transientFrontierPopulationCap(maxStacks, maxStackCullTrigger, p.noResultCompatibilityBenchmarkOnly)
	parseRuntime.IterationLimit = maxIter
	parseRuntime.StackDepthLimit = maxDepth
	parseRuntime.NodeLimit = maxNodes
	parseRuntime.MemoryBudgetBytes = arena.budgetBytes

	needToken := true
	var nextBranchOrder uint64 = 1
	allocBranchOrder := func() uint64 {
		order := nextBranchOrder
		nextBranchOrder++
		return order
	}

	var lastReduceState StateID
	lastReduceDepth := -1
	var consecutiveReduces int
	var lastNoTokenProgressTok Token
	var lastNoTokenProgressTokens uint64
	var consecutiveNoTokenDispatches int
	noTokenProgressHaveLast := false
	missingShift := parseMissingShiftTracker{lastDepth: -1}
	tryMissingSingleShift := func(stackIndex int, s *glrStack, currentState StateID) bool {
		return missingShift.tryInsert(p, source, stackIndex, s, currentState, tok, ts, &nodeCount, arena, scratch, trackChildErrors)
	}

	for iter := 0; iter < maxIter; iter++ {
		reusedLookaheadThisPass := !needToken
		passStartTokensConsumed := perfTokensConsumed
		if reason := p.parseStopReasonNow(); parseStopReasonIsTerminal(reason) {
			// Timeout/cancellation are checked inside the parse loop so
			// long-running parses can terminate predictably under
			// caller-configured limits.
			return finalize(stacks, reason)
		}
		iterationsUsed = iter + 1
		workCountSetConvergenceIteration(iterationsUsed) // work-count-assembly: convergence iteration seam
		if perfCountersEnabled {
			perfRecordMaxConcurrentStacks(len(stacks))
		}
		if timing != nil && len(stacks) > timing.maxStacksSeen {
			timing.maxStacksSeen = len(stacks)
		}
		if len(stacks) > maxStacksSeen {
			maxStacksSeen = len(stacks)
		}
		noteStopDiagnosticStacks(stacks)
		if progress.enabled {
			progress.maybeLoop(time.Now(), iterationsUsed, perfTokensConsumed, tok, perfTokensConsumed > 0, stacks, maxStacksSeen, nodeCount, peakStackDepth, needToken, singleStackIterations, multiStackIterations)
		}
		if len(stacks) == 1 {
			if stacks[0].dead {
				return finalize(stacks, ParseStopNoStacksAlive)
			}
			p.tryDemoteSingleLinearGSS(stacks, scratch)
			scratch.gss.setSingleStackMode(true)
			clearParseStackEntryCaches(stacks)
		} else {
			if progress.enabled {
				progress.emit(time.Now(), "merge_cull_begin", iterationsUsed, perfTokensConsumed, tok, perfTokensConsumed > 0, stacks, maxStacksSeen, nodeCount, peakStackDepth, needToken, singleStackIterations, multiStackIterations, "")
			}
			prep := p.prepareParseStacksForIteration(stacks, scratch, arena, arenaClass, maxStacks, maxStackCullTrigger, phaseTiming, &glrMergeNanos, &glrCullNanos)
			stacks = prep.stacks
			if progress.enabled {
				progress.emit(time.Now(), "merge_cull_end", iterationsUsed, perfTokensConsumed, tok, perfTokensConsumed > 0, stacks, maxStacksSeen, nodeCount, peakStackDepth, needToken, singleStackIterations, multiStackIterations, fmt.Sprintf("stopped=%t", prep.stopped))
			}
			if prep.stopped {
				if prep.errorTree {
					return finalizeErrorTree(prep.stopReason)
				}
				return finalize(stacks, prep.stopReason)
			}
		}
		if scratch.gss.singleStackMode {
			singleStackIterations++
		} else {
			multiStackIterations++
		}
		if reason := p.checkpointTransientScratch(stacks, scratch, arena); resultMaterializationShouldStop(reason) {
			return finalize(stacks, reason)
		}

		// Safety: if the primary stack has grown beyond the depth cap,
		// or we've allocated too many nodes, return what we have.
		primaryDepth := stacks[0].depth()
		if primaryDepth > peakStackDepth {
			peakStackDepth = primaryDepth
		}
		if primaryDepth > maxDepth {
			return finalize(stacks, ParseStopStackDepthLimit)
		}
		if nodeCount > maxNodes {
			return finalize(stacks, ParseStopNodeLimit)
		}
		if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
			return finalize(stacks, reason)
		}

		p.updateParserStateTokenSource(ts, stacks, scratch)

		// --- Token acquisition and incremental reuse ---
		if needToken {
			scratch.audit.startToken(stacks)
			if len(stacks) == 1 {
				singleStackTokens++
			} else {
				multiStackTokens++
			}
			if progress.enabled {
				progress.emit(time.Now(), "token_begin", iterationsUsed, perfTokensConsumed, tok, perfTokensConsumed > 0, stacks, maxStacksSeen, nodeCount, peakStackDepth, needToken, singleStackIterations, multiStackIterations, "")
			}
			if phaseTiming {
				tokenStart := time.Now()
				tok = p.cRecoverAcquireToken(ts, stacks, source)
				tokenNextNanos += time.Since(tokenStart).Nanoseconds()
			} else {
				tok = p.cRecoverAcquireToken(ts, stacks, source)
			}
			if progress.enabled {
				progress.emit(time.Now(), "token_end", iterationsUsed, perfTokensConsumed+1, tok, true, stacks, maxStacksSeen, nodeCount, peakStackDepth, needToken, singleStackIterations, multiStackIterations, "")
			}
			p.updateCurrentExternalTokenCheckpoint(ts, tok)
			workCountSetConvergenceLookahead(tok)
			if p.logger != nil {
				p.logf(ParserLogLex, "token sym=%d start=%d end=%d", tok.Symbol, tok.StartByte, tok.EndByte)
			}
			perfTokensConsumed++
			recordCurrentLookahead(tok)
			for si := range stacks {
				stacks[si].shifted = false
			}
			missingShift.resetForToken()
		}

		if reuse != nil && len(stacks) == 1 && !stacks[0].dead && tok.Symbol != 0 {
			// Campaign O(edit) W1 block-splice composition (spec.campaign.oedit).
			// Once the edited item finishes reparsing, a whole run of following
			// top-level siblings is byte-identical to the old tree. W1b lifted
			// them one at a time, but each sibling re-entered the full main-loop
			// iteration (merge/cull branch, single-stack GSS preamble, token-
			// source state push, progress/audit, timeout poll) purely to reach
			// the same settle+reuse it took last time. This loop keeps the
			// splice inside one iteration: after a sibling reuses, it advances
			// directly to the next one while the frontier stays single-stack and
			// the next position is another non-leaf top-level candidate, running
			// only the cheap, internally-throttled safety checks between steps.
			//
			// Admission is UNCHANGED and per-sibling: every member still goes
			// through tryReuseCurrentParseSubtree (fragility bit, byte-range
			// equality, zero-width exclusion, external-scanner quiescence, the
			// #393 reuse bar) and the same W1b settle. The first member that
			// fails any gate ends the block at that member -- the loop breaks and
			// the main loop reparses it -- so the block never lifts content past
			// a would-be extra-shift boundary or any other gate. The block runs
			// ONLY in single-stack mode; a settle that forks drains and exits.
			reusedAnySibling := false
			blockForked := false
			blockStopReason := ParseStopNone
			blockStopped := false
			for {
				if target, required := reuse.requiredTopLevelOwnershipFrontier(tok.StartByte); required && stacks[0].top().state != target {
					forkedDuringOwnershipSettle := false
					_, reached := p.settleDeterministicReduceChainForReuse(source, &stacks[0], tok, target, maxStacksSeen, reuse, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, trackChildErrors, &forkedDuringOwnershipSettle)
					if forkedDuringOwnershipSettle {
						drainPendingForkStacks()
						blockForked = true
						break
					}
					if reached {
						// Count the mismatch even though scheduler replay repaired
						// it before tryReuseSubtree observed the aligned state.
						reuse.observedPreGotoStateMismatch++
					}
				}
				nextTok, ok := p.tryReuseCurrentParseSubtree(&stacks[0], tok, ts, reuse, scratch, arena, &reuseState, timing)
				if !ok && reuse.hasNonLeafCandidateAt(tok.StartByte) {
					// W1b settle (unchanged): reuse failed at the live top-of-
					// stack state, but a non-leaf sibling candidate begins right
					// here. Settle the pending eager-default (unconditional)
					// reduce chain and retry, so a candidate whose recorded
					// PreGotoState is only reachable after those reduces can
					// still splice. Settling is a FALLBACK, never a pre-empt: any
					// reuse the current state already admits is taken first and
					// unchanged. It is scoped to positions with a real non-leaf
					// candidate so it can only advance toward a splice, never
					// perturb an intra-item trajectory (for example a trailing
					// comment's attachment). The settled reduces are exactly what
					// the dispatch loop performs next regardless of lookahead, so
					// falling through to it below is sound when reuse still does
					// not apply.
					forkedDuringSettle := false
					settled := p.settleEagerDefaultReduceChainForReuse(source, &stacks[0], tok, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, trackChildErrors, &forkedDuringSettle)
					if forkedDuringSettle {
						// A settled reduce forked the stack through a GSS
						// multi-link. Drain the pending forks into the live
						// frontier (exactly as the dispatch loop does after its
						// own reduces) and end the block; the multi-stack
						// dispatch below handles it -- the block never continues
						// on a forked frontier (single-stack only).
						drainPendingForkStacks()
						blockForked = true
						break
					}
					if settled && len(stacks) == 1 && !stacks[0].dead && !stacks[0].accepted && !stacks[0].shifted && tok.Symbol != 0 {
						nextTok, ok = p.tryReuseCurrentParseSubtree(&stacks[0], tok, ts, reuse, scratch, arena, &reuseState, timing)
					}
				}
				if !ok {
					// This member did not pass the gates; the block splits here.
					break
				}
				tok = nextTok
				// Reuse advances the token source without returning through
				// ordinary acquisition. Publish that lookahead immediately so
				// EOF, fork, timeout, and block-stop exits all report the
				// frontier that actually governs the parse.
				recordCurrentLookahead(tok)
				reusedAnySibling = true
				if timing != nil {
					timing.blockSpliceSteps++
				}
				// The block continues only while the frontier stays a single
				// live, non-accepted stack and the next position begins another
				// non-leaf top-level splice candidate -- the exact case this
				// fast loop is proven for. EOF (Symbol 0) ends it. Any other
				// position falls back to the main loop so ordinary dispatch or
				// leaf reuse handles it with the full per-iteration machinery.
				if tok.Symbol == 0 || len(stacks) != 1 || stacks[0].dead || stacks[0].accepted || stacks[0].shifted {
					break
				}
				if !reuse.hasNonLeafCandidateAt(tok.StartByte) {
					break
				}
				// Per-step maintenance mirroring the main loop's single-stack
				// preamble, minus the multi-stack / merge-cull / progress / audit
				// machinery a pure single-stack reuse frontier never exercises.
				// updateParserStateTokenSource is intentionally skipped: reuseNode
				// already set the token source's parser state to each reused
				// node's goto and lexed the next lookahead with it, so the main
				// loop's SetParserState would only re-set the identical value.
				// Each callee below is internally throttled, so running it per
				// step stays cheap while preserving every depth/node/memory cap
				// and the timeout poll.
				if reason := p.parseStopReasonNow(); parseStopReasonIsTerminal(reason) {
					blockStopReason, blockStopped = reason, true
					break
				}
				p.tryDemoteSingleLinearGSS(stacks, scratch)
				scratch.gss.setSingleStackMode(true)
				clearParseStackEntryCaches(stacks)
				if reason := p.checkpointTransientScratch(stacks, scratch, arena); resultMaterializationShouldStop(reason) {
					blockStopReason, blockStopped = reason, true
					break
				}
				if d := stacks[0].depth(); d > maxDepth {
					blockStopReason, blockStopped = ParseStopStackDepthLimit, true
					break
				}
				if nodeCount > maxNodes {
					blockStopReason, blockStopped = ParseStopNodeLimit, true
					break
				}
				if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
					blockStopReason, blockStopped = reason, true
					break
				}
			}
			if blockStopped {
				return finalize(stacks, blockStopReason)
			}
			if blockForked {
				// The block ended on a settle that forked. Fall through to the
				// multi-stack dispatch below on the drained frontier, exactly as
				// the pre-block W1b code did. When earlier siblings in this block
				// already spliced, tok is their advanced (already-lexed)
				// lookahead, so mark it consumed and reset the no-progress
				// counters the reuse continue would have.
				if reusedAnySibling {
					needToken = false
					consecutiveReduces = 0
					consecutiveNoTokenDispatches = 0
					noTokenProgressHaveLast = false
				}
			} else if reusedAnySibling {
				workCountRefreshConvergenceLookahead(tok)
				needToken = false
				consecutiveReduces = 0
				consecutiveNoTokenDispatches = 0
				noTokenProgressHaveLast = false
				continue
			}
		}

		numStacks := len(stacks)
		// See the Parser.reduceMultiVersion doc comment: live, non-sticky
		// "ts_stack_version_count > 1" signal for every reduce that can run
		// against the current token, including the external-default-reduce
		// pre-dispatch step below (which runs before the per-stack loop).
		// Re-synced at every numStacks reassignment in that pre-dispatch step.
		p.reduceMultiVersion = numStacks > 1
		if progress.enabled {
			dispatchTrace = make([]dispatchStackSurvivorTrace, len(stacks))
			for i := range dispatchTrace {
				dispatchTrace[i] = dispatchStackSurvivorTrace{
					inSnapshot:  true,
					origin:      "preexisting",
					sourceIndex: i,
				}
			}
		} else {
			dispatchTrace = nil
		}
		pendingTraceOrigin = ""
		pendingTraceSourceIndex = -1
		pendingTraceActionIndex = -1
		pendingTraceActionCount = 0
		pendingTraceAction = ParseAction{}
		traceVisit := func(index int, s *glrStack, path string, actionIndex, actionCount int, act ParseAction) {
			if !progress.enabled || index < 0 || index >= len(dispatchTrace) {
				return
			}
			state, byteOffset, depth := stackTraceState(s)
			t := &dispatchTrace[index]
			t.visited = true
			t.path = path
			t.actionIndex = actionIndex
			t.actionCount = actionCount
			t.actionType = act.Type
			t.actionState = act.State
			t.actionSymbol = act.Symbol
			t.actionChildCount = act.ChildCount
			t.beforeState = state
			t.beforeByte = byteOffset
			t.beforeDepth = depth
		}
		traceAfterPrimary := func(index int, s *glrStack) {
			if !progress.enabled || index < 0 || index >= len(dispatchTrace) {
				return
			}
			state, byteOffset, depth := stackTraceState(s)
			t := &dispatchTrace[index]
			t.afterPrimaryState = state
			t.afterPrimaryByte = byteOffset
			t.afterPrimaryDepth = depth
			if s != nil {
				t.afterPrimaryShifted = s.shifted
				t.afterPrimaryDead = s.dead
				t.afterPrimaryAccept = s.accepted
			}
			t.afterFinalState = state
			t.afterFinalByte = byteOffset
			t.afterFinalDepth = depth
			if s != nil {
				t.afterFinalShifted = s.shifted
				t.afterFinalDead = s.dead
				t.afterFinalAccepted = s.accepted
			}
		}
		traceFrontier := func(index int, s *glrStack, frontier conflictFrontierResult) {
			if !progress.enabled || index < 0 || index >= len(dispatchTrace) {
				return
			}
			state, byteOffset, depth := stackTraceState(s)
			t := &dispatchTrace[index]
			t.frontier = frontier
			t.afterFinalState = state
			t.afterFinalByte = byteOffset
			t.afterFinalDepth = depth
			if s != nil {
				t.afterFinalShifted = s.shifted
				t.afterFinalDead = s.dead
				t.afterFinalAccepted = s.accepted
			}
		}
		traceAppend := func(origin string, sourceIndex, actionIndex, actionCount int, act ParseAction, s *glrStack, frontier conflictFrontierResult) {
			if !progress.enabled {
				return
			}
			state, byteOffset, depth := stackTraceState(s)
			dispatchTrace = append(dispatchTrace, dispatchStackSurvivorTrace{
				inSnapshot:          false,
				visited:             false,
				origin:              origin,
				sourceIndex:         sourceIndex,
				path:                origin,
				actionIndex:         actionIndex,
				actionCount:         actionCount,
				actionType:          act.Type,
				actionState:         act.State,
				actionSymbol:        act.Symbol,
				actionChildCount:    act.ChildCount,
				afterPrimaryState:   state,
				afterPrimaryByte:    byteOffset,
				afterPrimaryDepth:   depth,
				afterPrimaryShifted: s != nil && s.shifted,
				afterPrimaryDead:    s != nil && s.dead,
				afterPrimaryAccept:  s != nil && s.accepted,
				afterFinalState:     state,
				afterFinalByte:      byteOffset,
				afterFinalDepth:     depth,
				afterFinalShifted:   s != nil && s.shifted,
				afterFinalDead:      s != nil && s.dead,
				afterFinalAccepted:  s != nil && s.accepted,
				frontier:            frontier,
			})
		}
		traceFrontierResult := func(beforeState StateID, beforeByte uint32, beforeDepth int, s *glrStack) conflictFrontierResult {
			afterState, afterByte, afterDepth := stackTraceState(s)
			result := conflictFrontierResult{
				ran:           true,
				reason:        "returned_unshifted",
				beforeState:   beforeState,
				beforeByte:    beforeByte,
				beforeDepth:   beforeDepth,
				afterState:    afterState,
				afterByte:     afterByte,
				afterDepth:    afterDepth,
				afterShifted:  s != nil && s.shifted,
				afterDead:     s != nil && s.dead,
				afterAccepted: s != nil && s.accepted,
			}
			switch {
			case result.afterShifted:
				result.reason = "shifted"
			case result.afterDead:
				result.reason = "dead"
			case result.afterAccepted:
				result.reason = "accepted"
			}
			if stopActionDiag.lastReduceCaptured {
				result.lastReduceSeen = true
				result.lastReduceState = stopActionDiag.lastReduceResultState
				result.lastReduceByte = stopActionDiag.lastReduceStackByte
				result.lastReduceDepth = stopActionDiag.lastReduceStackDepth
			}
			if s != nil && !s.dead && !s.accepted && !s.shifted && s.depth() > 0 {
				actionIdx := p.contextualActionIndex(source, s.top().state, tok)
				if actionIdx != 0 && p.language != nil && int(actionIdx) < len(p.language.ParseActions) {
					frontierActions := p.language.ParseActions[actionIdx].Actions
					result.terminalCount = len(frontierActions)
					if len(frontierActions) == 1 {
						result.terminalActionSet = true
						result.terminalActionTyp = frontierActions[0].Type
					}
					if len(frontierActions) != 1 {
						result.reason = "multi_action_frontier"
					}
				} else {
					result.reason = "no_action_frontier"
				}
			}
			return result
		}
		setPendingTrace := func(origin string, sourceIndex, actionIndex, actionCount int, act ParseAction) {
			if !progress.enabled {
				return
			}
			pendingTraceOrigin = origin
			pendingTraceSourceIndex = sourceIndex
			pendingTraceActionIndex = actionIndex
			pendingTraceActionCount = actionCount
			pendingTraceAction = act
		}
		anyReduced := false
		forceAdvanceAfterReduce := false
		dispatchConsumedCurrentToken := false
		// consumeCurrentToken marks a stack as fully done processing the
		// current token, whether it got there via a real terminal shift or
		// via an error/recovery/extra-token placeholder path — all of these
		// are equally "this stack will not revisit tok again" signals. The
		// caller passes the stack it just finished dispatching so
		// allLiveUnacceptedStacksShifted (used below to gate whether it's
		// safe to advance to the next token) sees it as settled instead of
		// mistaking it for a still-mid-reduce-chain stack.
		consumeCurrentToken := func(s *glrStack) {
			if s != nil {
				s.shifted = true
			}
			dispatchConsumedCurrentToken = true
			needToken = true
		}
		dispatchStart := time.Time{}
		if phaseTiming {
			dispatchStart = time.Now()
		}
		if p.glrTrace {
			p.traceParseIteration(iter, tok, stacks, needToken)
		}
		stopActionDiag.beginDispatch()
		if progress.enabled {
			progress.emit(time.Now(), "dispatch_begin", iterationsUsed, perfTokensConsumed, tok, true, stacks, maxStacksSeen, nodeCount, peakStackDepth, needToken, singleStackIterations, multiStackIterations, fmt.Sprintf("num_stacks=%d", numStacks))
		}
		parseActions := p.language.ParseActions
		if !tok.NoLookahead && p.isExternalToken(tok.Symbol) {
			if relexer, canRelex := ts.(tokenSourceRelexer); canRelex && relexer.CanRelexFromTokenStart(tok) {
				preDispatchDefaultReduced := false
				seen := make(map[externalDefaultReduceSeenKey]int)
				for step := 0; step < maxConsecutivePrimaryReduces; step++ {
					if !p.applyExternalNoActionDefaultReduceStep(source, tok, stacks, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, trackChildErrors, seen) {
						break
					}
					preDispatchDefaultReduced = true
					anyReduced = true
					drainPendingForkStacks()
					numStacks = len(stacks)
					p.reduceMultiVersion = numStacks > 1
					if !p.canApplyExternalNoActionDefaultReduce(tok, stacks) {
						break
					}
				}
				if preDispatchDefaultReduced {
					numStacks = len(stacks)
					p.reduceMultiVersion = numStacks > 1
					if !p.externalNoActionDefaultReducesStable(tok, stacks) {
						if parseEagerDefaultReduceDebugEnabled() {
							fmt.Printf("  EXTERNAL-DEFAULT-RELEX-DEFER old=%d[%d-%d]\n",
								tok.Symbol, tok.StartByte, tok.EndByte)
						}
					} else if !p.updateCurrentRelexParserStateTokenSource(ts, stacks, scratch) {
						if parseEagerDefaultReduceDebugEnabled() {
							fmt.Printf("  EXTERNAL-DEFAULT-RELEX-STATE-FAILED old=%d[%d-%d]\n",
								tok.Symbol, tok.StartByte, tok.EndByte)
						}
						if p.logger != nil {
							p.logf(ParserLogLex, "relex state update failed sym=%d start=%d end=%d", tok.Symbol, tok.StartByte, tok.EndByte)
						}
					} else if nextTok, ok := relexer.RelexFromTokenStart(tok); !ok {
						if parseEagerDefaultReduceDebugEnabled() {
							fmt.Printf("  EXTERNAL-DEFAULT-RELEX-FAILED old=%d[%d-%d]\n",
								tok.Symbol, tok.StartByte, tok.EndByte)
						}
						if p.logger != nil {
							p.logf(ParserLogLex, "relex failed sym=%d start=%d end=%d", tok.Symbol, tok.StartByte, tok.EndByte)
						}
					} else {
						if parseEagerDefaultReduceDebugEnabled() {
							fmt.Printf("  EXTERNAL-DEFAULT-RELEX old=%d[%d-%d] new=%d[%d-%d]\n",
								tok.Symbol, tok.StartByte, tok.EndByte, nextTok.Symbol, nextTok.StartByte, nextTok.EndByte)
						}
						tok = nextTok
						workCountRefreshConvergenceLookahead(tok)
						recordCurrentLookahead(tok)
						p.updateCurrentExternalTokenCheckpoint(ts, tok)
						if p.logger != nil {
							p.logf(ParserLogLex, "relex token sym=%d start=%d end=%d", tok.Symbol, tok.StartByte, tok.EndByte)
						}
					}
				}
			}
		}
		// stackRelexRestoreTok holds the shared lookahead while one stack
		// dispatches on its own re-lexed tokenization of the same bytes (see
		// relexTokenForStackLexState). The re-lex is scoped to that stack: the
		// shared token is restored before the next stack dispatches, and again
		// after the loop, so a sibling that does accept the original symbol is
		// never handed a token it cannot use.
		stackRelexRestoreTok := Token{}
		stackRelexActive := false
		packedVersionOrder := p.language != nil && p.language.CompactPackedGSSVersionOrderCertified
		for si := 0; si < numStacks || (packedVersionOrder && si < len(stacks)); si++ {
			s := &stacks[si]
			if stackRelexActive {
				tok = stackRelexRestoreTok
				stackRelexActive = false
			}
			// reduceActionConflict is scoped to a single stack's dispatch; a
			// fresh iteration must not inherit the previous stack's conflict
			// context (see the "if len(actions) > 1" block below, which is
			// the only place this is set true).
			p.reduceActionConflict = false
			if s.dead || s.shifted {
				continue
			}
			// Faithful C recovery port (parser_recover_c.go): an accepted
			// version has left the version pool (C stashes its tree in
			// finished_tree and removes the version); it must not dispatch
			// again while other versions finish competing. Outside the gated
			// recovery, accepted stacks never survive an iteration (the loop
			// tail finalizes immediately), so this is unreachable there.
			if s.accepted {
				continue
			}
			// Faithful C recovery port (parser_recover_c.go): paused stacks
			// (C StackStatusPaused) wait for the condense step after this
			// dispatch pass to be resumed or removed.
			if s.cPaused {
				continue
			}
			// Faithful C recovery port (parser_recover_c.go): a stack already
			// in the C error state dispatches through ts_parser__recover
			// instead of the parse table, except for shiftable tokens.
			if s.cRec != nil && p.errorCostCompetitionEnabled() {
				outcome, redispatch, reason := p.cRecoverDispatchInError(&stacks, si, source, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, trackChildErrors)
				if resultMaterializationShouldStop(reason) {
					return finalize(stacks, reason)
				}
				s = &stacks[si]
				if redispatch {
					anyReduced = true
				}
				switch outcome {
				case cRecConsumed:
					continue
				case cRecHalted:
					s.dead = true
					continue
				}
			}
			currentState := s.top().state
			noteStopDiagnosticStack(s)
			packedVersionReductionSteps := 0
		retryAction:
			if packedVersionOrder {
				// A transaction can append reduction versions and then remove its
				// promoted source before this retry. Recompute both live C signals;
				// do not carry the prior action cell's conflict state forward.
				p.reduceMultiVersion = len(stacks) > 1
				p.reduceActionConflict = false
			}
			dispatchVersionCount := compactPackedGSSDispatchVersionCount(packedVersionOrder, numStacks, len(stacks))
			actionStart := time.Time{}
			if phaseTiming {
				actionStart = time.Now()
			}
			actionIdx := p.contextualActionIndex(source, currentState, tok)
			var actions []ParseAction
			if actionIdx != 0 && int(actionIdx) < len(parseActions) {
				actions = parseActions[actionIdx].Actions
			}
			semanticPhaseTraceRecordActionCell(p, s, currentState, tok, actions) // semantic-phase-assembly: action-cell seam
			workCountRecordResolvedActionCell(len(actions))                      // work-count-assembly: resolved action-cell seam
			workCountAddActionEntries(len(actions))
			if phaseTiming {
				actionLookupNanos += time.Since(actionStart).Nanoseconds()
			}
			if keywordSym, ok := p.typeScriptContextualPropertyKeywordSymbol(tok, source); ok && parseStacksShareState(stacks[:dispatchVersionCount], currentState) {
				if p.typeScriptContextualPropertyKeywordHasAction(keywordSym, currentState) {
					tok.Symbol = keywordSym
					workCountRefreshConvergenceLookahead(tok)
					needToken = false
					goto retryAction
				}
			}
			p.traceStackActions(si, currentState, tok.Symbol, actions)
			if p.ambiguityProfile != nil {
				p.ambiguityProfile.record(currentState, tok.Symbol, actions, dispatchVersionCount)
			}
			if packedVersionOrder && compactPackedGSSActionCellRequiresTransaction(actions) {
				p.reduceActionConflict = len(actions) > 1
				if len(actions) > 1 {
					scratch.gss.everForked = true
				}
				s.ensureGSS(&scratch.gss)
				var lastReductionVersion int
				var terminalOrdinal int
				var reason ParseStopReason
				stacks, lastReductionVersion, terminalOrdinal, reason = p.cAppendActionCellReductionVersions(
					source, stacks, si, actions, tok, &nodeCount, arena,
					&scratch.entries, &scratch.gss, &scratch.tmpEntries, trackChildErrors, &anyReduced,
				)
				if reason != ParseStopNone {
					return finalize(stacks, reason)
				}
				p.reduceMultiVersion = len(stacks) > 1
				s = &stacks[si]
				if terminalOrdinal >= 0 {
					terminal := actions[terminalOrdinal]
					if terminal.Type == ParseActionShift && !p.guardRealShiftGap(source, s, tok) {
						continue
					}
					if terminal.Type == ParseActionRecover && !p.guardRealTokenAttachmentGap(source, s, tok, "packed-version-recover") {
						continue
					}
					traceVisit(si, s, "packed-version-terminal", terminalOrdinal, len(actions), terminal)
					setPendingTrace("packed-version-terminal", si, terminalOrdinal, len(actions), terminal)
					p.noteStopActionDiagnostic("packed-version-terminal", s, tok, terminal, terminalOrdinal, len(actions), false, 0, 0, false)
					p.applyAction(source, s, terminal, tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, trackChildErrors)
					p.noteStopActionResult(s)
					traceAfterPrimary(si, s)
					if terminal.Type == ParseActionShift || terminal.Type == ParseActionRecover {
						consumeCurrentToken(s)
					}
					continue
				}
				if lastReductionVersion >= 0 {
					var renumbered bool
					stacks, renumbered = cRenumberReductionVersion(stacks, lastReductionVersion, si)
					if !renumbered {
						return finalize(stacks, ParseStopInvariantViolation)
					}
					packedVersionReductionSteps++
					if packedVersionReductionSteps > maxConsecutivePrimaryReduces {
						return finalize(stacks, ParseStopIterationLimit)
					}
					s = &stacks[si]
					currentState = s.top().state
					if tok.NoLookahead {
						// C sets needs_lex after it promotes a reduction made at
						// the end of a nonterminal extra. Do not dispatch the EOF
						// table again against the promoted state.
						needToken = true
						continue
					}
					needToken = false
					goto retryAction
				}
				if tok.NoLookahead {
					// C halts a nonterminal-extra version whose fixed reduction
					// produced no surviving physical version.
					s.dead = true
					continue
				}
				// A real lookahead remains invalid on the unchanged source slot.
				// Continue through the ordinary no-action path so keyword repair,
				// re-lexing, and recovery retain their existing ownership.
				p.reduceActionConflict = false
				actions = nil
			}
			if len(actions) > 0 && actions[0].Type == ParseActionShift && actions[0].Extra {
				actionKindStart := time.Time{}
				if actionTiming != nil {
					actionKindStart = time.Now()
				}
				if dispatchVersionCount == 1 && p.tryMaterializeSkippedRealGap(source, s, currentState, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, trackChildErrors) {
					anyReduced = true
					needToken = false
					goto retryAction
				}
				if !p.guardRealShiftGap(source, s, tok) {
					continue
				}
				traceVisit(si, s, "extra-shift", 0, len(actions), actions[0])
				semanticPhaseTraceRecordActionExecution(p, s, tok, actions[0], 0, "extra-shift", false) // semantic-phase-assembly: extra-shift-execution seam
				p.applyExtraShiftAction(s, currentState, actions[0], tok, arena, scratch, trackChildErrors)
				nodeCount++
				traceAfterPrimary(si, s)
				consumeCurrentToken(s)
				if actionTiming != nil {
					ns := time.Since(actionKindStart).Nanoseconds()
					actionTiming.actionExtraShiftNanos += ns
					recordActionTiming(currentState, tok.Symbol, actions, ambiguityActionExtraShift, ns)
				}
				continue
			}
			if len(actions) == 0 {
				noActionStart := time.Time{}
				if actionTiming != nil {
					noActionStart = time.Now()
				}
				recordNoActionTiming := func() int64 {
					if actionTiming == nil {
						return 0
					}
					ns := time.Since(noActionStart).Nanoseconds()
					actionTiming.actionNoActionNanos += ns
					recordActionTiming(currentState, tok.Symbol, actions, ambiguityActionNoAction, ns)
					return ns
				}
				sameState := parseStacksShareState(stacks, currentState)
				// Retirement of a per-language patch: this relex was gated to
				// javascript by name, but the condition it guards is grammar
				// independent. noLiveStackCanAcceptLookahead already proves no live
				// stack has an action for the lookahead, so re-lexing cannot steal a
				// token another version was going to consume, whatever the grammar.
				canRelexNoLiveAction := !sameState && p.language != nil && p.noLiveStackCanAcceptLookahead(stacks, tok)
				if tok.Symbol == errorSymbol && tok.StartByte != tok.EndByte && p.errorCostCompetitionEnabled() {
					// Faithful C recovery port: an unlexable-run lookahead has
					// no table actions in C either; the version pauses and the
					// condense step decides (ts_parser__handle_error skips the
					// strategy-1 scan for error lookaheads and absorbs it).
					s.cPaused = true
					p.markCRecoveryCostCompetitionRelevant()
					if actionTiming != nil {
						ns := recordNoActionTiming()
						actionTiming.actionNoActionErrorNanos += ns
					}
					continue
				}
				if tok.Symbol == errorSymbol && tok.StartByte != tok.EndByte && !p.isScheme {
					// Unlexable-run lookahead (mirrors C skipped-error lexing):
					// absorb it into this stack as an extra ERROR leaf the way
					// C ts_parser__recover does. It is never a reason to kill a
					// GLR stack, relex (the error lex state already failed at
					// this position), or pop into a resync. Scheme keeps its
					// dedicated recovery flow (schemeErrorRunToken + _datum
					// goto) further down this chain.
					if DebugDFA.Load() {
						fmt.Printf("  ABSORB-ERR tok=%d-%d state=%d stacks=%d\n", tok.StartByte, tok.EndByte, currentState, len(stacks))
					}
					if !p.guardRealTokenAttachmentGap(source, s, tok, "lex-error") {
						continue
					}
					p.pushLexErrorRunLeaf(s, currentState, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, trackChildErrors)
					consumeCurrentToken(s)
					if actionTiming != nil {
						ns := recordNoActionTiming()
						actionTiming.actionNoActionErrorNanos += ns
					}
					continue
				}
				if tok.Symbol == 0 {
					if sameState {
						if reTok, ok := p.tryRelexCurrentStateDFA(tok, currentState, ts); ok {
							tok = reTok
							workCountRefreshConvergenceLookahead(tok)
							needToken = false
							if actionTiming != nil {
								ns := recordNoActionTiming()
								actionTiming.actionNoActionRelexNanos += ns
							}
							goto retryAction
						}
					}
					if tok.StartByte != tok.EndByte {
						consumeCurrentToken(s)
						if actionTiming != nil {
							recordNoActionTiming()
						}
						continue
					}
					if p.errorCostCompetitionEnabled() {
						// Faithful C recovery port: EOF with no action pauses;
						// the condense step resumes via ts_parser__handle_error
						// whose recover_eof wraps the stack in an ERROR root.
						s.cPaused = true
						p.markCRecoveryCostCompetitionRelevant()
						if actionTiming != nil {
							ns := recordNoActionTiming()
							actionTiming.actionNoActionErrorNanos += ns
						}
						continue
					}
					if len(stacks) == 1 {
						if p.canFinalizeNoActionEOFAt(s, expectedEOFByte, source) {
							if actionTiming != nil {
								recordNoActionTiming()
							}
							if phaseTiming {
								actionDispatchNanos += time.Since(dispatchStart).Nanoseconds()
							}
							return finalize(stacks, ParseStopAccepted)
						}
						if tree, ok := tryFinalizeTrailingEOFSuffix(s, tok); ok {
							if actionTiming != nil {
								recordNoActionTiming()
							}
							if phaseTiming {
								actionDispatchNanos += time.Since(dispatchStart).Nanoseconds()
							}
							return tree
						}
					}
					s.dead = true
					if actionTiming != nil {
						ns := recordNoActionTiming()
						actionTiming.actionNoActionErrorNanos += ns
					}
					continue
				}
				if tok.StartByte == tok.EndByte {
					consumeCurrentToken(s)
					if actionTiming != nil {
						recordNoActionTiming()
					}
					continue
				}
				if canRelexNoLiveAction {
					if reTok, ok := p.tryRelexSingleParserState(tok, currentState, ts, stacks, scratch); ok {
						tok = reTok
						workCountRefreshConvergenceLookahead(tok)
						needToken = false
						if actionTiming != nil {
							ns := recordNoActionTiming()
							actionTiming.actionNoActionRelexNanos += ns
						}
						goto retryAction
					}
				}
				if sameState {
					if reTok, ok := p.tryRelexCurrentStateDFA(tok, currentState, ts); ok {
						tok = reTok
						workCountRefreshConvergenceLookahead(tok)
						needToken = false
						if actionTiming != nil {
							ns := recordNoActionTiming()
							actionTiming.actionNoActionRelexNanos += ns
						}
						goto retryAction
					}
					if reTok, ok := p.tryRelexBroadDFA(tok, currentState, ts); ok {
						tok = reTok
						workCountRefreshConvergenceLookahead(tok)
						needToken = false
						if actionTiming != nil {
							ns := recordNoActionTiming()
							actionTiming.actionNoActionRelexNanos += ns
						}
						goto retryAction
					}
				}
				// Languages gated into the faithful C error-recovery cost
				// competition (parser_recover_c.go) own single-stack no-action
				// dead ends themselves: pausing + condense + ts_parser__recover
				// resumes locally, matching tree-sitter C. The opportunistic
				// top-level resync below is a heuristic for languages WITHOUT
				// that port; letting it fire first for a C-recovery-gated
				// language can unwind the whole stack back to the grammar's
				// initial state (wrapping a huge, otherwise-locally-recoverable
				// span in one ERROR, or replaying it error-free) instead of the
				// narrow, tested local recovery the C-recovery gate would have
				// produced. Known tradeoff: this also removes the opportunistic
				// resync's ERROR-wrap safety net for the (currently small) set
				// of C-recovery-gated languages that relied on it to flag a
				// nested declaration-start construct as an error rather than
				// silently accepting it — see
				// TestOpportunisticTopLevelResyncDoesNotLiftNestedDeclarationStarts/java_import_inside_malformed_method.
				if len(stacks) == 1 && !p.resyncTopLevelLanguage() && !p.errorCostCompetitionEnabled() {
					switch p.tryOpportunisticTopLevelResyncRecovery(source, s, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, trackChildErrors) {
					case resyncRetry:
						currentState = s.top().state
						needToken = false
						if actionTiming != nil {
							ns := recordNoActionTiming()
							actionTiming.actionNoActionRecoverNanos += ns
						}
						goto retryAction
					case resyncAdvance:
						consumeCurrentToken(s)
						if actionTiming != nil {
							ns := recordNoActionTiming()
							actionTiming.actionNoActionRecoverNanos += ns
						}
						continue
					}
				}
				if p.errorCostCompetitionEnabled() {
					// C lexes once per version, so a version whose state needs
					// a different tokenization of these exact bytes gets it.
					// This engine shares one token across all stacks, so give
					// this stack its own lex mode before pausing it. See
					// relexTokenForStackLexState (issue #454): the re-lex is
					// span-exact, action-verified, and DFA-only, so the token
					// loop stays in lockstep and the external scanner is never
					// re-entered. Restored for the next stack at the top of the
					// dispatch loop so the shared token never leaks sideways.
					if reTok, ok := p.relexTokenForStackLexState(source, currentState, tok); ok {
						if p.glrTrace {
							fmt.Printf("  stack[%d] C-STACK-RELEX: sym=%d -> sym=%d [%d-%d] in state=%d\n",
								si, tok.Symbol, reTok.Symbol, reTok.StartByte, reTok.EndByte, currentState)
						}
						stackRelexRestoreTok = tok
						stackRelexActive = true
						tok = reTok
						if actionTiming != nil {
							ns := recordNoActionTiming()
							actionTiming.actionNoActionRelexNanos += ns
						}
						goto retryAction
					}
					// Faithful C recovery port: no action for the lookahead —
					// pause the version (C detect_error / ts_stack_pause) and
					// let the post-pass condense step resume or remove it.
					if p.glrTrace {
						fmt.Printf("  stack[%d] C-PAUSED: no action for sym=%d in state=%d\n", si, tok.Symbol, currentState)
					}
					s.cPaused = true
					p.markCRecoveryCostCompetitionRelevant()
					if actionTiming != nil {
						ns := recordNoActionTiming()
						actionTiming.actionNoActionErrorNanos += ns
					}
					continue
				}
				if _, _, hasRecoverAction := p.findRecoverActionOnStack(s, tok.Symbol, timing); !hasRecoverAction &&
					p.tryRecoverPreviousShiftAsError(s, tok, &nodeCount, arena, &scratch.entries, trackChildErrors) {
					anyReduced = true
					needToken = false
					consecutiveReduces = 0
					if actionTiming != nil {
						ns := recordNoActionTiming()
						actionTiming.actionNoActionRecoverNanos += ns
					}
					continue
				}
				if len(stacks) > 1 {
					// A GLR fork can leave sibling stacks in states that need
					// different tokenizations of the same bytes: a keyword
					// literal versus the grammar's generic word token, a
					// dedicated operator token versus a shared one, and so
					// on. This engine lexes one token per iteration for
					// every live stack (see updateParserStateTokenSource),
					// so a stack whose state needs the other reading is
					// starved unless it gets a chance at its own lex mode
					// first. relexTokenForStackLexState is the same
					// span-exact, action-verified DFA probe the faithful
					// C-recovery port already uses for this (issue #454)
					// and the compact route runs unconditionally
					// (relexTokenForState); it is a no-op whenever the
					// re-lex does not land a different, action-bearing
					// symbol at the identical byte span, so a stack that
					// genuinely has no other reading is killed exactly as
					// before.
					if reTok, ok := p.relexTokenForStackLexState(source, currentState, tok); ok {
						if p.glrTrace {
							fmt.Printf("  stack[%d] STACK-RELEX: sym=%d -> sym=%d [%d-%d] in state=%d\n",
								si, tok.Symbol, reTok.Symbol, reTok.StartByte, reTok.EndByte, currentState)
						}
						stackRelexRestoreTok = tok
						stackRelexActive = true
						tok = reTok
						if actionTiming != nil {
							ns := recordNoActionTiming()
							actionTiming.actionNoActionRelexNanos += ns
						}
						goto retryAction
					}
					if p.glrTrace {
						fmt.Printf("  stack[%d] KILLED: no action for sym=%d in state=%d (multiple stacks)\n", si, tok.Symbol, currentState)
					}
					s.dead = true
					if actionTiming != nil {
						ns := recordNoActionTiming()
						actionTiming.actionNoActionErrorNanos += ns
					}
					continue
				}
				if tryMissingSingleShift(si, s, currentState) {
					anyReduced = true
					needToken = false
					consecutiveReduces = 0
					if actionTiming != nil {
						ns := recordNoActionTiming()
						actionTiming.actionNoActionMissingNanos += ns
					}
					continue
				}
				if depth, recoverAct, ok := p.findRecoverActionOnStack(s, tok.Symbol, timing); ok {
					if !s.truncate(depth + 1) {
						s.dead = true
						if actionTiming != nil {
							ns := recordNoActionTiming()
							actionTiming.actionNoActionErrorNanos += ns
						}
						continue
					}
					if !p.guardRealTokenAttachmentGap(source, s, tok, "recover") {
						continue
					}
					p.noteStopActionDiagnostic("no-action-recover", s, tok, recoverAct, -1, 1, false, 0, 0, false)
					p.applyAction(source, s, recoverAct, tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, trackChildErrors)
					p.noteStopActionResult(s)
					drainPendingForkStacks()
					consumeCurrentToken(s)
					if actionTiming != nil {
						ns := recordNoActionTiming()
						actionTiming.actionNoActionRecoverNanos += ns
					}
					continue
				}
				if s.depth() == 0 {
					if actionTiming != nil {
						ns := recordNoActionTiming()
						actionTiming.actionNoActionErrorNanos += ns
					}
					if phaseTiming {
						actionDispatchNanos += time.Since(dispatchStart).Nanoseconds()
					}
					return finalize(stacks, ParseStopNoStacksAlive)
				}
				// In-context recovery (C ts_parser__recover candidate rule):
				// pop to the NEAREST stack state with an action on the
				// lookahead, wrap only the popped fragments into an extra
				// ERROR, and retry there. Runs before the top-level resync so
				// damage stays contained inside the enclosing construct.
				if p.tryNearestActionStateRecovery(s, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, trackChildErrors) {
					currentState = s.top().state
					needToken = false
					if actionTiming != nil {
						ns := recordNoActionTiming()
						actionTiming.actionNoActionRecoverNanos += ns
					}
					goto retryAction
				}
				// Panic-mode resync (mirrors C ts_parser__recover): before
				// appending a flat ERROR leaf at this dead-end state, try to pop
				// down to the grammar's top-level (initial) state, wrap the failed
				// region into one localized ERROR node (preserving any already
				// completed valid top-level siblings), and resume there. This keeps
				// subsequent valid top-level constructs nested under the real root
				// instead of shredding the rest of the file into flat fragments.
				switch p.tryResyncErrorRecovery(source, s, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, trackChildErrors) {
				case resyncRetry:
					currentState = s.top().state
					needToken = false
					if actionTiming != nil {
						ns := recordNoActionTiming()
						actionTiming.actionNoActionRecoverNanos += ns
					}
					goto retryAction
				case resyncAdvance:
					consumeCurrentToken(s)
					if actionTiming != nil {
						ns := recordNoActionTiming()
						actionTiming.actionNoActionRecoverNanos += ns
					}
					continue
				}
				if !p.guardRealTokenAttachmentGap(source, s, tok, "error") {
					continue
				}
				p.pushOrExtendErrorNode(s, currentState, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, trackChildErrors)
				consumeCurrentToken(s)
				if actionTiming != nil {
					ns := recordNoActionTiming()
					actionTiming.actionNoActionErrorNanos += ns
				}
				continue
			}
			if len(actions) > 1 {
				// A real grammar conflict can grow the live stack count (a
				// literal clone below, or a frontier/gated fork queued by
				// completeConflictReduceFrontier) before the top-of-loop
				// singleStackMode recompute runs on the next iteration. Latch
				// everForked here, at the earliest possible point, so raw-shape
				// capture resumes for every reduction from this token onward —
				// including the actions[0] continuation applied to the
				// original stack later in this same iteration.
				scratch.gss.everForked = true
				// C's action_count > 1: this dispatch point had more than one
				// viable parse-table action, so every node any of the paths
				// below build (deterministic choice, depth-cap fallback, or
				// an explicit fork, plus their conflict-reduce-frontier /
				// pending-fork follow-through) is born fragile. Reset to
				// false at the top of every per-stack iteration above.
				p.reduceActionConflict = true
				conflictStart := time.Time{}
				if actionTiming != nil {
					conflictStart = time.Now()
				}
				var chosen ParseAction
				chosenOrdinal := -1
				choice := false
				if next, ok := p.deterministicConflictChoiceForDispatch(source, s, tok, currentState, actions, maxStacksSeen, reuse); ok {
					chosen, choice = next, true
					// The diagnostic tagged seam may infer a unique ordinal. The
					// production build deliberately performs no reconstruction scan.
					chosenOrdinal = -2
				}
				if !choice && deterministicExternalConflicts && p.language != nil && p.language.Name == "yaml" && p.language.ExternalScanner != nil {
					chosen, chosenOrdinal, choice = deterministicExternalConflictActionAt(actions)
				}
				if choice {
					skipRepetitionShift := false
					if chosen.Type == ParseActionReduce {
						if cReduce, ok := singleReduceAgainstRepetitionShiftConflictChoice(actions); ok && cReduce == chosen {
							skipRepetitionShift = true
						}
					}
					if chosen.Type == ParseActionShift && !p.guardRealShiftGap(source, s, tok) {
						continue
					}
					if chosen.Type == ParseActionRecover && !p.guardRealTokenAttachmentGap(source, s, tok, "recover") {
						continue
					}
					traceVisit(si, s, "conflict-choice", -1, len(actions), chosen)
					setPendingTrace("conflict-choice", si, -1, len(actions), chosen)
					p.noteStopActionDiagnostic("conflict-choice", s, tok, chosen, chosenOrdinal, len(actions), false, 0, 0, false)
					actionBeforeState, actionBeforeByte, actionBeforeDepth := stackTraceState(s)
					p.applyAction(source, s, chosen, tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, trackChildErrors)
					p.noteStopActionResult(s)
					actionAfterState, actionAfterByte, actionAfterDepth := stackTraceState(s)
					traceAfterPrimary(si, s)
					if chosen.Type == ParseActionReduce {
						p.completeConflictReduceFrontier(source, s, tok, conflictReduceFrontierSeed{
							action:              chosen,
							beforeState:         actionBeforeState,
							beforeByte:          actionBeforeByte,
							beforeDepth:         actionBeforeDepth,
							afterState:          actionAfterState,
							afterByte:           actionAfterByte,
							afterDepth:          actionAfterDepth,
							skipRepetitionShift: skipRepetitionShift,
						}, len(stacks), frontierForkPopulationCap, allocBranchOrder, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, trackChildErrors)
						traceFrontier(si, s, traceFrontierResult(actionAfterState, actionAfterByte, actionAfterDepth, s))
					}
					drainPendingForkStacks()
					drainPendingFrontierForkStacks()
					if actionTiming != nil {
						ns := time.Since(conflictStart).Nanoseconds()
						actionTiming.actionConflictChoiceNanos += ns
						recordActionTiming(currentState, tok.Symbol, actions, ambiguityActionConflictChoice, ns)
					}
					continue
				}
				if perfCountersEnabled {
					rrConflict, rsConflict := classifyConflictShape(actions)
					switch {
					case rrConflict:
						perfRecordConflictRR()
					case rsConflict:
						perfRecordConflictRS()
					default:
						perfRecordConflictOther()
					}
					perfRecordFork(len(actions), perfTokensConsumed)
				}
				if preMaterializationDiag {
					candidates, sameKey, overflow := p.observePreMaterializationFieldRejectFork(s, actions, scratch.tmpEntries, mergePerKeyCap)
					preMaterializationFieldRejectCandidates += candidates
					preMaterializationFieldRejectSameKeyCandidates += sameKey
					preMaterializationFieldRejectOverflowCandidates += overflow
				}
				if s.depth() > maxForkCloneDepth {
					if actions[0].Type == ParseActionShift && !p.guardRealShiftGap(source, s, tok) {
						continue
					}
					if actions[0].Type == ParseActionRecover && !p.guardRealTokenAttachmentGap(source, s, tok, "recover") {
						continue
					}
					traceVisit(si, s, "conflict-depth-cap", 0, len(actions), actions[0])
					setPendingTrace("conflict-depth-cap", si, 0, len(actions), actions[0])
					p.noteStopActionDiagnostic("conflict-depth-cap", s, tok, actions[0], 0, len(actions), false, 0, 0, false)
					actionBeforeState, actionBeforeByte, actionBeforeDepth := stackTraceState(s)
					p.applyAction(source, s, actions[0], tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, trackChildErrors)
					p.noteStopActionResult(s)
					actionAfterState, actionAfterByte, actionAfterDepth := stackTraceState(s)
					traceAfterPrimary(si, s)
					if actions[0].Type == ParseActionReduce {
						p.completeConflictReduceFrontier(source, s, tok, conflictReduceFrontierSeed{
							action:      actions[0],
							beforeState: actionBeforeState,
							beforeByte:  actionBeforeByte,
							beforeDepth: actionBeforeDepth,
							afterState:  actionAfterState,
							afterByte:   actionAfterByte,
							afterDepth:  actionAfterDepth,
						}, len(stacks), frontierForkPopulationCap, allocBranchOrder, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, trackChildErrors)
						traceFrontier(si, s, traceFrontierResult(actionAfterState, actionAfterByte, actionAfterDepth, s))
					}
					drainPendingForkStacks()
					drainPendingFrontierForkStacks()
					if actionTiming != nil {
						ns := time.Since(conflictStart).Nanoseconds()
						actionTiming.actionConflictForkNanos += ns
						recordActionTiming(currentState, tok.Symbol, actions, ambiguityActionConflictFork, ns)
					}
					continue
				}
				base := *s
				if p.glrTrace {
					p.traceParseFork(currentState, actions)
				}
				for ai := 1; ai < len(actions); ai++ {
					fork := base.cloneWithScratch(&scratch.gss)
					fork.branchOrder = allocBranchOrder()
					if actions[ai].Type != ParseActionShift || p.guardRealShiftGap(source, &fork, tok) {
						if actions[ai].Type != ParseActionRecover || p.guardRealTokenAttachmentGap(source, &fork, tok, "recover") {
							if workCountInstrumentationEnabled {
								workCountTopologyPrepareVersionCopy(&base, &fork) // work-count-assembly: topology conflict-copy seam
							}
							setPendingTrace("conflict-fork", si, ai, len(actions), actions[ai])
							p.noteStopActionDiagnostic("conflict-fork", &fork, tok, actions[ai], ai, len(actions), false, 0, 0, false)
							actionBeforeState, actionBeforeByte, actionBeforeDepth := stackTraceState(&fork)
							p.applyAction(source, &fork, actions[ai], tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, trackChildErrors)
							p.noteStopActionResult(&fork)
							actionAfterState, actionAfterByte, actionAfterDepth := stackTraceState(&fork)
							if actions[ai].Type == ParseActionReduce {
								p.completeConflictReduceFrontier(source, &fork, tok, conflictReduceFrontierSeed{
									action:      actions[ai],
									beforeState: actionBeforeState,
									beforeByte:  actionBeforeByte,
									beforeDepth: actionBeforeDepth,
									afterState:  actionAfterState,
									afterByte:   actionAfterByte,
									afterDepth:  actionAfterDepth,
								}, len(stacks), frontierForkPopulationCap, allocBranchOrder, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, trackChildErrors)
								frontier := traceFrontierResult(actionAfterState, actionAfterByte, actionAfterDepth, &fork)
								traceAppend("conflict-fork", si, ai, len(actions), actions[ai], &fork, frontier)
							} else {
								traceAppend("conflict-fork", si, ai, len(actions), actions[ai], &fork, conflictFrontierResult{})
							}
						} else {
							traceAppend("conflict-fork-guard-recover", si, ai, len(actions), actions[ai], &fork, conflictFrontierResult{})
						}
					} else {
						traceAppend("conflict-fork-guard-shift", si, ai, len(actions), actions[ai], &fork, conflictFrontierResult{})
					}
					if p.glrTrace {
						fmt.Printf("[GLR] fork[%d] after action[%d]: st=%d dead=%v shift=%v dep=%d byte=%d\n",
							len(stacks), ai, fork.top().state, fork.dead, fork.shifted, fork.depth(), fork.byteOffset)
					}
					stacks = append(stacks, fork)
					if !progress.enabled {
						drainPendingForkStacks()
						drainPendingFrontierForkStacks()
					} else {
						drainPendingForkStacks()
						drainPendingFrontierForkStacks()
					}
				}
				s = &stacks[si]
				if actions[0].Type == ParseActionShift && !p.guardRealShiftGap(source, s, tok) {
					continue
				}
				if actions[0].Type == ParseActionRecover && !p.guardRealTokenAttachmentGap(source, s, tok, "recover") {
					continue
				}
				traceVisit(si, s, "conflict-original", 0, len(actions), actions[0])
				setPendingTrace("conflict-original", si, 0, len(actions), actions[0])
				p.noteStopActionDiagnostic("conflict-original", s, tok, actions[0], 0, len(actions), false, 0, 0, false)
				actionBeforeState, actionBeforeByte, actionBeforeDepth := stackTraceState(s)
				p.applyAction(source, s, actions[0], tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, trackChildErrors)
				p.noteStopActionResult(s)
				actionAfterState, actionAfterByte, actionAfterDepth := stackTraceState(s)
				traceAfterPrimary(si, s)
				if actions[0].Type == ParseActionReduce {
					p.completeConflictReduceFrontier(source, s, tok, conflictReduceFrontierSeed{
						action:      actions[0],
						beforeState: actionBeforeState,
						beforeByte:  actionBeforeByte,
						beforeDepth: actionBeforeDepth,
						afterState:  actionAfterState,
						afterByte:   actionAfterByte,
						afterDepth:  actionAfterDepth,
					}, len(stacks), frontierForkPopulationCap, allocBranchOrder, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, trackChildErrors)
					traceFrontier(si, s, traceFrontierResult(actionAfterState, actionAfterByte, actionAfterDepth, s))
				}
				if p.glrTrace {
					fmt.Printf("[GLR] orig[%d] after action[0]: st=%d dead=%v shift=%v dep=%d byte=%d\n",
						si, s.top().state, s.dead, s.shifted, s.depth(), s.byteOffset)
				}
				drainPendingForkStacks()
				drainPendingFrontierForkStacks()
				if actionTiming != nil {
					ns := time.Since(conflictStart).Nanoseconds()
					actionTiming.actionConflictForkNanos += ns
					recordActionTiming(currentState, tok.Symbol, actions, ambiguityActionConflictFork, ns)
				}
				continue
			}
			act := actions[0]
			actionKindStart := time.Time{}
			if actionTiming != nil {
				actionKindStart = time.Now()
			}
			disableBashReduceChain := p.language != nil && p.language.Name == "bash" && s.gss.head != nil
			if act.Type == ParseActionReduce && !disableBashReduceChain {
				traceVisit(si, s, "single-reduce", 0, len(actions), act)
				setPendingTrace("single-reduce", si, 0, len(actions), act)
				p.noteStopActionDiagnostic("single-reduce", s, tok, act, 0, len(actions), false, 0, 0, false)
				if p.applyActionWithReduceChain(source, s, act, tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, trackChildErrors) {
					forceAdvanceAfterReduce = true
				}
				p.noteStopActionResult(s)
				traceAfterPrimary(si, s)
				drainPendingForkStacks()
				if actionTiming != nil {
					ns := time.Since(actionKindStart).Nanoseconds()
					actionTiming.actionSingleReduceNanos += ns
					recordActionTiming(currentState, tok.Symbol, actions, ambiguityActionSingleReduce, ns)
				}
			} else {
				switch act.Type {
				case ParseActionShift:
					if dispatchVersionCount == 1 && p.tryMaterializeSkippedRealGap(source, s, currentState, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, trackChildErrors) {
						anyReduced = true
						needToken = false
						goto retryAction
					}
					if !p.guardRealShiftGap(source, s, tok) {
						continue
					}
					traceVisit(si, s, "single-shift", 0, len(actions), act)
					p.noteStopActionDiagnostic("single-shift", s, tok, act, 0, len(actions), false, 0, 0, false)
					p.applyShiftAction(s, act, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, trackChildErrors)
					p.noteStopActionResult(s)
					traceAfterPrimary(si, s)
					consumeCurrentToken(s)
					if actionTiming != nil {
						ns := time.Since(actionKindStart).Nanoseconds()
						actionTiming.actionSingleShiftNanos += ns
						recordActionTiming(currentState, tok.Symbol, actions, ambiguityActionSingleShift, ns)
					}
				case ParseActionAccept:
					traceVisit(si, s, "single-accept", 0, len(actions), act)
					p.noteStopActionDiagnostic("single-accept", s, tok, act, 0, len(actions), false, 0, 0, false)
					p.applyAcceptAction(s)
					p.noteStopActionResult(s)
					traceAfterPrimary(si, s)
					if actionTiming != nil {
						ns := time.Since(actionKindStart).Nanoseconds()
						actionTiming.actionSingleAcceptNanos += ns
						recordActionTiming(currentState, tok.Symbol, actions, ambiguityActionSingleAccept, ns)
					}
				case ParseActionRecover:
					if dispatchVersionCount == 1 && p.tryMaterializeSkippedRealGap(source, s, currentState, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, trackChildErrors) {
						anyReduced = true
						needToken = false
						goto retryAction
					}
					if !p.guardRealTokenAttachmentGap(source, s, tok, "recover") {
						continue
					}
					traceVisit(si, s, "single-recover", 0, len(actions), act)
					p.noteStopActionDiagnostic("single-recover", s, tok, act, 0, len(actions), false, 0, 0, false)
					p.applyRecoverAction(s, act, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, trackChildErrors)
					p.noteStopActionResult(s)
					traceAfterPrimary(si, s)
					consumeCurrentToken(s)
					if actionTiming != nil {
						ns := time.Since(actionKindStart).Nanoseconds()
						actionTiming.actionSingleRecoverNanos += ns
						recordActionTiming(currentState, tok.Symbol, actions, ambiguityActionSingleRecover, ns)
					}
				default:
					traceVisit(si, s, "single-other", 0, len(actions), act)
					setPendingTrace("single-other", si, 0, len(actions), act)
					p.noteStopActionDiagnostic("single-other", s, tok, act, 0, len(actions), false, 0, 0, false)
					p.applyAction(source, s, act, tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, trackChildErrors)
					p.noteStopActionResult(s)
					traceAfterPrimary(si, s)
					drainPendingForkStacks()
					if actionTiming != nil {
						ns := time.Since(actionKindStart).Nanoseconds()
						actionTiming.actionSingleOtherNanos += ns
						recordActionTiming(currentState, tok.Symbol, actions, ambiguityActionSingleOther, ns)
					}
				}
			}
		}
		// The last stack in the loop may have dispatched on its own re-lexed
		// tokenization; restore the shared lookahead before anything after the
		// dispatch loop reads it.
		if stackRelexActive {
			tok = stackRelexRestoreTok
			stackRelexActive = false
		}
		if phaseTiming {
			actionDispatchNanos += time.Since(dispatchStart).Nanoseconds()
		}
		if progress.enabled {
			progress.emit(time.Now(), "dispatch_end", iterationsUsed, perfTokensConsumed, tok, true, stacks, maxStacksSeen, nodeCount, peakStackDepth, needToken, singleStackIterations, multiStackIterations, fmt.Sprintf("any_reduced=%t consumed_token=%t", anyReduced, dispatchConsumedCurrentToken))
		}
		if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
			return finalize(stacks, reason)
		}

		postDispatchVersionCount := compactPackedGSSDispatchVersionCount(packedVersionOrder, numStacks, len(stacks))
		if postDispatchVersionCount > 1 && retryPass && allParseStacksDead(stacks) {
			bestIdx := bestRetryRecoveryStack(stacks)
			stacks[bestIdx].dead = false
			stacks[0] = stacks[bestIdx]
			stacks = stacks[:1]
			if p.glrTrace {
				fmt.Printf("[GLR] ALL-DEAD RECOVERY: resurrect stack (was [%d]) st=%d dep=%d byte=%d\n",
					bestIdx, stacks[0].top().state, stacks[0].depth(), stacks[0].byteOffset)
			}

			currentState := stacks[0].top().state
			if tryMissingSingleShift(bestIdx, &stacks[0], currentState) {
				anyReduced = true
				needToken = false
				consecutiveReduces = 0
			} else if _, _, hasRecoverAction := p.findRecoverActionOnStack(&stacks[0], tok.Symbol, timing); !hasRecoverAction &&
				p.tryRecoverPreviousShiftAsError(&stacks[0], tok, &nodeCount, arena, &scratch.entries, trackChildErrors) {
				anyReduced = true
				needToken = false
				consecutiveReduces = 0
			} else if depth, recoverAct, ok := p.findRecoverActionOnStack(&stacks[0], tok.Symbol, timing); ok {
				if stacks[0].truncate(depth + 1) {
					if !p.guardRealTokenAttachmentGap(source, &stacks[0], tok, "recover") {
						continue
					}
					p.noteStopActionDiagnostic("retry-recover", &stacks[0], tok, recoverAct, -1, 1, false, 0, 0, false)
					p.applyAction(source, &stacks[0], recoverAct, tok, &anyReduced, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, trackChildErrors)
					p.noteStopActionResult(&stacks[0])
					drainPendingForkStacks()
					consumeCurrentToken(&stacks[0])
				} else {
					stacks[0].dead = true
				}
			} else if stacks[0].depth() > 0 {
				if !p.guardRealTokenAttachmentGap(source, &stacks[0], tok, "error") {
					continue
				}
				p.pushOrExtendErrorNode(&stacks[0], currentState, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, trackChildErrors)
				consumeCurrentToken(&stacks[0])
			}
		}

		// Faithful C recovery port: ts_parser__condense_stack runs after each
		// completed dispatch pass — prune versions by error cost, resume the
		// best paused version (ts_parser__handle_error), remove the rest.
		// Only touches passes where some stack is paused or absorbing, so
		// clean parses are unaffected.
		condenseErrorCostEnabled := p.errorCostCompetitionEnabled()
		condenseAnyReduced := anyReduced
		condenseRelevant := condenseErrorCostEnabled && cRecoveryRelevantStack(stacks)
		condenseEOFRecovery := condenseRelevant && tok.Symbol == 0 && tok.StartByte == tok.EndByte && !tok.NoLookahead
		condenseShiftedRecovery := condenseRelevant && anyReduced && !tok.NoLookahead && allLiveUnacceptedStacksShifted(stacks)
		condenseRan := false
		condenseResumed := false
		if condenseErrorCostEnabled && (!anyReduced || condenseEOFRecovery || condenseShiftedRecovery) {
			var resumed bool
			condenseRan = true
			var reason ParseStopReason
			if recoveryRuntimeDetailedBuildEnabled {
				stacks, resumed, tok, reason = p.cCondenseAndResumeDetailed(stacks, source, ts, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, scratch, trackChildErrors)
			} else {
				stacks, resumed, tok, reason = p.cCondenseAndResume(stacks, source, ts, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, scratch, trackChildErrors)
			}
			workCountRefreshConvergenceLookahead(tok)
			if resultMaterializationShouldStop(reason) {
				return finalize(stacks, reason)
			}
			condenseResumed = resumed
			if resumed {
				anyReduced = true
			}
		}
		stopDiagCondenseErrorCostEnabled = condenseErrorCostEnabled
		stopDiagCondenseAnyReduced = condenseAnyReduced
		stopDiagCondenseRelevant = condenseRelevant
		stopDiagCondenseRan = condenseRan
		stopDiagCondenseResumed = condenseResumed
		stopDiagCondenseGating = ""

		postDispatchDefaultReduced := false
		if anyReduced && !tok.NoLookahead && parseEagerDefaultReduceEnabled() && p.isExternalToken(tok.Symbol) {
			if relexer, canRelex := ts.(tokenSourceRelexer); canRelex && relexer.CanRelexFromTokenStart(tok) {
				postDispatchDefaultReduced = p.applyEagerDefaultReduces(source, "post-dispatch", stacks, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, deferParentLinks, trackChildErrors)
				if postDispatchDefaultReduced {
					drainPendingForkStacks()
					if !p.updateCurrentRelexParserStateTokenSource(ts, stacks, scratch) {
						postDispatchDefaultReduced = false
					} else {
						nextTok, ok := relexer.RelexFromTokenStart(tok)
						if !ok {
							postDispatchDefaultReduced = false
							if parseEagerDefaultReduceDebugEnabled() {
								fmt.Printf("  EAGER-DEFAULT-RELEX-FAILED old=%d[%d-%d]\n",
									tok.Symbol, tok.StartByte, tok.EndByte)
							}
							if p.logger != nil {
								p.logf(ParserLogLex, "relex failed sym=%d start=%d end=%d", tok.Symbol, tok.StartByte, tok.EndByte)
							}
						} else {
							if parseEagerDefaultReduceDebugEnabled() {
								fmt.Printf("  EAGER-DEFAULT-RELEX old=%d[%d-%d] new=%d[%d-%d]\n",
									tok.Symbol, tok.StartByte, tok.EndByte, nextTok.Symbol, nextTok.StartByte, nextTok.EndByte)
							}
							tok = nextTok
							workCountRefreshConvergenceLookahead(tok)
							recordCurrentLookahead(tok)
							p.updateCurrentExternalTokenCheckpoint(ts, tok)
							if p.logger != nil {
								p.logf(ParserLogLex, "relex token sym=%d start=%d end=%d", tok.Symbol, tok.StartByte, tok.EndByte)
							}
						}
					}
				}
			}
		}

		if anyReduced && !dispatchConsumedCurrentToken && !tok.NoLookahead && !(tok.Symbol == 0 && tok.StartByte == tok.EndByte) {
			terminalFrontier := terminalFrontierScratch[:0]
			terminalFrontierConsumes := false
			terminalFrontierOK := true
			for i := range stacks {
				s := &stacks[i]
				if s.dead || s.accepted || s.shifted || s.cPaused || s.depth() == 0 {
					continue
				}
				actionIdx := p.contextualActionIndex(source, s.top().state, tok)
				if actionIdx == 0 || int(actionIdx) >= len(parseActions) {
					terminalFrontierOK = false
					break
				}
				actions := parseActions[actionIdx].Actions
				if len(actions) != 1 {
					terminalFrontierOK = false
					break
				}
				act := actions[0]
				switch act.Type {
				case ParseActionShift:
					if len(stacks) == 1 && p.tryMaterializeSkippedRealGap(source, s, s.top().state, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, trackChildErrors) {
						terminalFrontierOK = false
						terminalFrontier = terminalFrontier[:0]
						needToken = false
						break
					}
					if !p.guardRealShiftGap(source, s, tok) {
						terminalFrontierOK = false
						break
					}
					terminalFrontierConsumes = true
				case ParseActionRecover:
					if len(stacks) == 1 && p.tryMaterializeSkippedRealGap(source, s, s.top().state, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, trackChildErrors) {
						terminalFrontierOK = false
						terminalFrontier = terminalFrontier[:0]
						needToken = false
						break
					}
					if !p.guardRealTokenAttachmentGap(source, s, tok, "post-reduce-terminal-frontier-recover") {
						terminalFrontierOK = false
						break
					}
					terminalFrontierConsumes = true
				case ParseActionAccept:
				default:
					terminalFrontierOK = false
				}
				if !terminalFrontierOK {
					break
				}
				terminalFrontier = append(terminalFrontier, terminalFrontierAction{index: i, action: act})
			}
			if terminalFrontierOK && len(terminalFrontier) > 0 {
				for _, item := range terminalFrontier {
					s := &stacks[item.index]
					switch item.action.Type {
					case ParseActionShift:
						if len(stacks) == 1 && p.tryMaterializeSkippedRealGap(source, s, s.top().state, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, trackChildErrors) {
							terminalFrontierOK = false
							terminalFrontier = terminalFrontier[:0]
							needToken = false
							break
						}
						p.noteStopActionDiagnostic("post-reduce-terminal-frontier-shift", s, tok, item.action, 0, 1, false, 0, 0, false)
						p.applyShiftAction(s, item.action, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, trackChildErrors)
						p.noteStopActionResult(s)
					case ParseActionRecover:
						if len(stacks) == 1 && p.tryMaterializeSkippedRealGap(source, s, s.top().state, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, trackChildErrors) {
							terminalFrontierOK = false
							terminalFrontier = terminalFrontier[:0]
							needToken = false
							break
						}
						p.noteStopActionDiagnostic("post-reduce-terminal-frontier-recover", s, tok, item.action, 0, 1, false, 0, 0, false)
						p.applyRecoverAction(s, item.action, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, trackChildErrors)
						s.shifted = true
						p.noteStopActionResult(s)
					case ParseActionAccept:
						p.noteStopActionDiagnostic("post-reduce-terminal-frontier-accept", s, tok, item.action, 0, 1, false, 0, 0, false)
						p.applyAcceptAction(s)
						p.noteStopActionResult(s)
					}
				}
				if terminalFrontierConsumes && allLiveUnacceptedStacksShifted(stacks) {
					dispatchConsumedCurrentToken = true
					needToken = true
				}
			}
		}

		// C parity: ts_parser__condense_stack runs after EVERY advance round.
		// A strategy-1 recovery fork created during this round's dispatch
		// shifts the current token in the terminal-frontier block above —
		// AFTER the main condense gate already ran with allShifted=false — so
		// the post-shift cost competition must be re-checked here. This is the
		// round where C kills the absorbing version the moment the recovered
		// fork advances cleanly past the elected token (compare_versions
		// in-error vs clean → TakeRight; e.g. the FIDL versioned-layout
		// recovery chain). No stack is paused at this point (paused stacks
		// have shifted=false), so the resume tail inside is a no-op.
		if condenseErrorCostEnabled && !condenseRan && condenseRelevant &&
			anyReduced && !tok.NoLookahead &&
			allLiveUnacceptedStacksShifted(stacks) {
			var resumed bool
			condenseRan = true
			var reason ParseStopReason
			if recoveryRuntimeDetailedBuildEnabled {
				stacks, resumed, tok, reason = p.cCondenseAndResumeDetailed(stacks, source, ts, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, scratch, trackChildErrors)
			} else {
				stacks, resumed, tok, reason = p.cCondenseAndResume(stacks, source, ts, tok, &nodeCount, arena, &scratch.entries, &scratch.gss, &scratch.tmpEntries, scratch, trackChildErrors)
			}
			if resultMaterializationShouldStop(reason) {
				return finalize(stacks, reason)
			}
			condenseResumed = resumed
			if resumed {
				anyReduced = true
			}
			stopDiagCondenseRan = condenseRan
			stopDiagCondenseResumed = condenseResumed
		}

		p.clearCRecoveryCostIfClean(stacks, trackChildErrors)

		// After processing all stacks: determine whether to advance the
		// token. If any stack reduced, reuse the same token (the reducing
		// stacks have new top states and need to re-check the action for
		// the current lookahead). Otherwise, advance to next token.
		if anyReduced {
			needToken = tok.NoLookahead || forceAdvanceAfterReduce
			// dispatchConsumedCurrentToken is set as soon as ANY single stack
			// performs a terminal action (shift/recover/accept/error) for the
			// current token via consumeCurrentToken(). In a multi-stack GLR
			// dispatch pass, other live stacks may have only reduced this
			// pass (their new top state still needs to be re-checked against
			// the SAME lookahead before it's safe to advance). Advancing the
			// token here would strand those stacks mid-reduce-chain: the next
			// pass would probe their post-reduce state against the WRONG
			// (already-advanced) token instead of the one they were still
			// waiting to shift, killing an otherwise-viable parse (see e.g.
			// TypeScript's `foo<Bar>()` instantiation-expression vs.
			// binary_expression ambiguity: the type_arguments fork reduces
			// into a state that still needs to shift "(", but a sibling
			// fork's shift of "(" would force the token forward under it).
			// Only treat the token as fully consumed once every live,
			// unaccepted stack has actually shifted it.
			if dispatchConsumedCurrentToken && allLiveUnacceptedStacksShifted(stacks) {
				needToken = true
				lastReduceDepth = -1
				consecutiveReduces = 0
			} else if tok.NoLookahead {
				lastReduceDepth = -1
				consecutiveReduces = 0
			} else if postDispatchDefaultReduced {
				needToken = false
				lastReduceDepth = -1
				consecutiveReduces = 0
			} else if len(stacks) > 0 && !stacks[0].dead {
				topState := stacks[0].top().state
				topDepth := stacks[0].depth()
				if topState == lastReduceState && topDepth == lastReduceDepth {
					consecutiveReduces++
				} else {
					lastReduceState = topState
					lastReduceDepth = topDepth
					consecutiveReduces = 1
				}
				if consecutiveReduces > maxConsecutivePrimaryReduces {
					if tok.Symbol == 0 && tok.StartByte == tok.EndByte && len(stacks) == 1 {
						if tree, ok := tryFinalizeTrailingEOFSuffix(&stacks[0], tok); ok {
							return tree
						}
						if p.canFinalizeNoActionEOFAt(&stacks[0], expectedEOFByte, source) {
							return finalize(stacks, ParseStopAccepted)
						}
						return finalize(stacks, ParseStopNoStacksAlive)
					}
					if len(stacks) > 1 && !tok.NoLookahead && !(tok.Symbol == 0 && tok.StartByte == tok.EndByte) {
						needToken = false
					} else {
						needToken = true
						lastReduceDepth = -1
						consecutiveReduces = 0
					}
				}
			}
		} else {
			needToken = true
			lastReduceDepth = -1
			consecutiveReduces = 0
		}
		if anyReduced && !dispatchConsumedCurrentToken && !needToken && !tok.NoLookahead &&
			!(tok.Symbol == 0 && tok.StartByte == tok.EndByte) &&
			allLiveUnacceptedStacksShifted(stacks) {
			needToken = true
			lastReduceDepth = -1
			consecutiveReduces = 0
		}
		if stopActionDiag.captured {
			stopActionDiag.forceAdvanceAfterReduce = forceAdvanceAfterReduce
			stopActionDiag.postDispatchDefaultReduced = postDispatchDefaultReduced
			stopActionDiag.anyReduced = anyReduced
			stopActionDiag.consumedToken = dispatchConsumedCurrentToken
		}
		if reusedLookaheadThisPass && anyReduced && !dispatchConsumedCurrentToken && !needToken && !tok.NoLookahead &&
			!(tok.Symbol == 0 && tok.StartByte == tok.EndByte) &&
			perfTokensConsumed == passStartTokensConsumed {
			if noTokenProgressHaveLast &&
				tok.Symbol == lastNoTokenProgressTok.Symbol &&
				tok.StartByte == lastNoTokenProgressTok.StartByte &&
				tok.EndByte == lastNoTokenProgressTok.EndByte &&
				tok.NoLookahead == lastNoTokenProgressTok.NoLookahead &&
				perfTokensConsumed == lastNoTokenProgressTokens {
				consecutiveNoTokenDispatches++
			} else {
				lastNoTokenProgressTok = tok
				lastNoTokenProgressTokens = perfTokensConsumed
				consecutiveNoTokenDispatches = 1
				noTokenProgressHaveLast = true
			}
			if consecutiveNoTokenDispatches > maxConsecutiveNoTokenDispatches {
				stopDiagFrontier = p.buildStopFrontierDiagnostics(stacks, tok, postDispatchVersionCount, dispatchTrace)
				if progress.enabled {
					extra := fmt.Sprintf("count=%d cap=%d any_reduced=%t consumed_token=%t", consecutiveNoTokenDispatches, maxConsecutiveNoTokenDispatches, anyReduced, dispatchConsumedCurrentToken)
					if actionExtra := stopActionDiag.progressString(); actionExtra != "" {
						extra += " " + actionExtra
					}
					extra += " " + stopDiagFrontier.actions
					extra += " " + stopDiagFrontier.stackTable
					extra += " " + stopDiagFrontier.sameHeader
					if stopDiagFrontier.survivors != "" {
						extra += " " + stopDiagFrontier.survivors
					}
					stopDiagCondenseGating = stopCondenseGatingString(stopDiagCondenseErrorCostEnabled, stopDiagCondenseAnyReduced, stopDiagCondenseRelevant, stopDiagCondenseRan, stopDiagCondenseResumed, stacks)
					if stopDiagCondenseGating != "" {
						extra += " " + stopDiagCondenseGating
					}
					progress.emit(time.Now(), "no_token_progress_stop", iterationsUsed, perfTokensConsumed, tok, true, stacks, maxStacksSeen, nodeCount, peakStackDepth, needToken, singleStackIterations, multiStackIterations,
						extra)
				}
				return finalize(stacks, ParseStopIterationLimit)
			}
		} else {
			consecutiveNoTokenDispatches = 0
			noTokenProgressHaveLast = false
		}
		acceptedCount, liveUnaccepted := 0, 0
		for i := range stacks {
			if stacks[i].accepted {
				acceptedCount++
			} else if !stacks[i].dead {
				liveUnaccepted++
			}
		}
		if acceptedCount > 0 {
			// Faithful C recovery port: C keeps parsing the remaining
			// versions after one accepts (the accepted tree is stashed and
			// competes in ts_parser__select_tree); finalize only when every
			// version has accepted or halted. Without the gate, finalize on
			// the first accept exactly as before.
			if p.errorCostCompetitionEnabled() && tok.Symbol == 0 && tok.StartByte == tok.EndByte && !tok.NoLookahead && anyReduced && liveUnaccepted > 0 {
				needToken = false
				continue
			}
			if !p.errorCostCompetitionEnabled() || liveUnaccepted == 0 {
				accepted := compactAcceptedStacks(stacks)
				selectable := accepted[:0]
				continuationEscape := p.lineContinuationEscapeByte()
				for i := range accepted {
					if cleanAcceptedStackSelectableAtEOF(source, expectedEOFByte, p.included, arena, &accepted[i], continuationEscape) {
						selectable = append(selectable, accepted[i])
					}
				}
				if len(selectable) == 0 && liveUnaccepted > 0 {
					continue
				}
				if len(selectable) > 0 {
					accepted = selectable
				}
				if p.errorCostCompetitionEnabled() {
					if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
						return finalize(accepted, reason)
					}
					// Faithful C recovery port: ts_parser__accept rebuilds the
					// root around trailing extras before the tree competes.
					for i := range accepted {
						if reason := p.cAcceptRootRebuild(&accepted[i], arena, &scratch.entries, &scratch.gss); resultMaterializationShouldStop(reason) {
							return finalize(accepted, reason)
						}
					}
				}
				return finalize(accepted, ParseStopAccepted)
			}
		}
	}

	return finalize(stacks, ParseStopIterationLimit)
}

type parseModeFlags struct {
	compactNoTreeShiftLeaves         bool
	compactFullShiftLeaves           bool
	pendingFullParents               bool
	finalChildRefs                   bool
	skipInvisibleFullLeafCheckpoints bool
	transientReduceChildren          bool
	transientReduceScratchNoAlias    bool
	transientChildren                *transientChildScratch
}

func (p *Parser) applyParseModeFlags(source []byte, reuse *reuseCursor, oldTree *Tree, arenaClass arenaClass) parseModeFlags {
	prev := parseModeFlags{
		compactNoTreeShiftLeaves:         p.compactNoTreeShiftLeaves,
		compactFullShiftLeaves:           p.compactFullShiftLeaves,
		pendingFullParents:               p.pendingFullParents,
		finalChildRefs:                   p.finalChildRefs,
		skipInvisibleFullLeafCheckpoints: p.skipInvisibleFullLeafCheckpoints,
		transientReduceChildren:          p.transientReduceChildren,
		transientReduceScratchNoAlias:    p.transientReduceScratchNoAlias,
		transientChildren:                p.transientChildren,
	}
	p.compactNoTreeShiftLeaves = p.noTreeBenchmarkOnly && parseShouldCompactNoTreeShiftLeaves(len(source))
	p.compactFullShiftLeaves = parseShouldUseCompactFullShiftLeaves(p, source, reuse, oldTree, arenaClass)
	p.pendingFullParents = parseShouldUsePendingFullParents(p, source, reuse, oldTree, arenaClass)
	p.finalChildRefs = parseShouldUseFinalChildRefs(p, source, reuse, oldTree, arenaClass)
	p.skipInvisibleFullLeafCheckpoints = parseShouldSkipInvisibleFullLeafCheckpoints(p, source, reuse, oldTree, arenaClass)
	return prev
}

func (p *Parser) restoreParseModeFlags(prev parseModeFlags) {
	p.compactNoTreeShiftLeaves = prev.compactNoTreeShiftLeaves
	p.compactFullShiftLeaves = prev.compactFullShiftLeaves
	p.pendingFullParents = prev.pendingFullParents
	p.finalChildRefs = prev.finalChildRefs
	p.skipInvisibleFullLeafCheckpoints = prev.skipInvisibleFullLeafCheckpoints
	p.transientReduceChildren = prev.transientReduceChildren
	p.transientReduceScratchNoAlias = prev.transientReduceScratchNoAlias
	p.transientChildren = prev.transientChildren
}

func (p *Parser) configureParseScratch(scratch *parserScratch, source []byte, reuse *reuseCursor, oldTree *Tree, arenaClass arenaClass, deferParentLinks bool) bool {
	p.transientReduceChildren = p.shouldUseTransientReduceChildren(source, reuse, oldTree, arenaClass)
	if p.transientReduceChildren {
		p.transientChildren = &scratch.transientChildren
	} else {
		p.transientChildren = nil
	}
	scratch.merge.language = p.language
	scratch.merge.cErrorCostParser = nil
	scratch.merge.trace = p.glrTrace
	scratch.merge.beginEquivEpoch()
	scratch.merge.ensureMergeHotCaches()
	transientReduceParents := p.shouldUseTransientReduceParents(source, reuse, oldTree, arenaClass)
	if p.transientReduceChildren && transientReduceParents {
		scratch.transientCheckpointBytes = parseTransientReduceCheckpointBytes()
	} else {
		scratch.transientCheckpointBytes = 0
	}
	p.transientReduceScratchNoAlias = p.transientReduceChildren && transientReduceParents && parseShouldUseTransientReduceScratchNoAlias(len(source))
	scratch.merge.pythonShallow = p.language != nil && p.language.Name == "python" && len(source) <= 512*1024
	if deferParentLinks {
		scratch.gss.initialCap = p.fullGSSHintCapacity()
	} else {
		scratch.gss.initialCap = p.incrementalGSSHintCapacity()
	}
	return transientReduceParents
}

func (p *Parser) recordParseArenaUsageOnReturn(arenaClass arenaClass, arena *nodeArena, scratch *parserScratch) func() {
	if arenaClass == arenaClassFull {
		return func() {
			if !p.noTreeBenchmarkOnly {
				switch {
				case p.finalChildRefs:
					p.recordFinalChildRefArenaUsage(arena.used)
				case p.compactFullShiftLeaves:
					p.recordCompactFullArenaUsage(arena.used)
				case p.pendingFullParents:
					p.recordPendingFullArenaUsage(arena.used)
				default:
					p.recordFullArenaUsage(arena.used)
				}
			}
			p.recordFullGSSUsage(scratch.gss.peakUsed)
		}
	}
	return func() {
		p.recordIncrementalArenaUsage(arena.used)
		p.recordIncrementalGSSUsage(scratch.gss.peakUsed)
	}
}

func (p *Parser) ensureParseInitialCapacity(source []byte, arenaClass arenaClass, arena *nodeArena, scratch *parserScratch) {
	switch arenaClass {
	case arenaClassFull:
		p.ensureFullParseInitialCapacity(source, arena, scratch)
	case arenaClassIncremental:
		target := parseIncrementalArenaNodeCapacity(len(source), p.incrementalArenaHintCapacity())
		arena.ensureNodeCapacity(target)
		scratch.entries.ensureInitialCap(parseIncrementalEntryScratchCapacity(len(source)))
	}
}

func (p *Parser) ensureFullParseInitialCapacity(source []byte, arena *nodeArena, scratch *parserScratch) {
	target := parseFullArenaNodeCapacityForSource(source, p.language, p.fullArenaHintCapacity())
	checkpointCapacityTarget := target
	switch {
	case p.finalChildRefs:
		target = parseFinalChildRefArenaNodeCapacity(len(source), p.finalChildRefArenaHintCapacity())
		checkpointCapacityTarget = parseFullArenaInitialNodeCapacity(len(source))
	case p.compactFullShiftLeaves:
		target = parseCompactFullArenaNodeCapacity(len(source), p.compactFullArenaHintCapacity())
		checkpointCapacityTarget = parseFullArenaInitialNodeCapacity(len(source))
	case p.pendingFullParents:
		target = parsePendingFullArenaNodeCapacity(len(source), p.pendingFullArenaHintCapacity())
		checkpointCapacityTarget = target
	}
	if p.noTreeBenchmarkOnly {
		target = parseNoTreeArenaNodeCapacity(len(source))
		checkpointCapacityTarget = target
	}
	arena.ensureExactNodeCapacity(target)
	if p.noTreeBenchmarkOnly && !p.noTreeCheckpointBenchmarkOnly {
		arena.dropExternalScannerCheckpointStorage()
	}
	if !p.noTreeBenchmarkOnly && languageUsesExternalScannerCheckpoints(p.language) {
		arena.ensureExternalScannerCheckpointCapacity(parseFullExternalScannerCheckpointCapacity(len(source), checkpointCapacityTarget))
	}
	scratch.entries.ensureInitialCap(parseFullEntryScratchCapacity(len(source)))
}

func (p *Parser) newInitialParseStacks(scratch *parserScratch, reuse *reuseCursor, timing *incrementalParseTiming, sourceLen int) ([]glrStack, int) {
	var stacksBuf [4]glrStack
	stacks := stacksBuf[:1]
	initialStackCap := defaultStackEntrySlabCap
	if reuse == nil {
		initialStackCap = parseFullEntryScratchReservation(sourceLen)
	}
	stacks[0] = newGLRStackWithScratchCap(p.language.InitialState, &scratch.entries, initialStackCap)
	// Included-range parsing starts at the first selected byte. Keep the
	// initial stack offset aligned with the token source so a skipped prefix
	// cannot become a parser-owned ERROR span before the first token.
	if p != nil && len(p.included) > 0 {
		start := p.included[0].StartByte
		if uint64(start) > uint64(sourceLen) {
			start = uint32(sourceLen)
		}
		stacks[0].byteOffset = start
	}
	stacks[0].recoverabilityKnown = true
	stacks[0].mayRecover = p.stateCanRecover(p.language.InitialState)
	if timing != nil && timing.maxStacksSeen < len(stacks) {
		timing.maxStacksSeen = len(stacks)
	}
	return stacks, len(stacks)
}

type parseCaps struct {
	maxStacks           int
	retryPass           bool
	mergePerKeyCap      int
	maxStackCullTrigger int
	maxIter             int
	maxDepth            int
	maxNodes            int
}

func (p *Parser) configureParseCaps(source []byte, reuse *reuseCursor, arenaClass arenaClass, scratch *parserScratch, maxStacksOverride, maxNodesOverride, maxMergePerKeyOverride int) parseCaps {
	maxStacks, retryPass := resolveParseMaxStacks(parseMaxGLRStacksValue(), maxStacksOverride, p.maxConflictWidth)
	if p.forceCleanRetryPass {
		retryPass = false
	}
	mergePerKeyCap := p.resolveParseMergePerKeyCap(source, reuse, maxMergePerKeyOverride)
	scratch.merge.perKeyCap = mergePerKeyCap
	scratch.merge.faithfulCapOne = reuse == nil &&
		mergePerKeyCap == 1 &&
		((p.language != nil && p.language.FullParseGSSConvergenceEnabled) ||
			parseMaxMergePerKeyEnvConfigured() ||
			maxMergePerKeyOverride < 0)
	// C keeps equivalent cap-one recovery readings as links on one graph
	// stack. Preserve that convergence after recovery makes error cost
	// relevant. Otherwise, separate Go stacks fork each history at each
	// conflict in the valid suffix. This behavior can grow quadratically.
	scratch.merge.recoveryCapOneConvergence = reuse == nil &&
		mergePerKeyCap == 1 &&
		p.errorCostCompetitionEnabled()

	maxNodes := parseNodeLimitForLanguage(len(source), p.language)
	if maxNodesOverride > maxNodes {
		maxNodes = maxNodesOverride
	}
	return parseCaps{
		maxStacks:           maxStacks,
		retryPass:           retryPass,
		mergePerKeyCap:      mergePerKeyCap,
		maxStackCullTrigger: glrStackCullTrigger(maxStacks, languageName(p.language)),
		maxIter:             parseIterationsForLanguage(len(source), p.language),
		maxDepth:            parseStackDepth(len(source)),
		maxNodes:            maxNodes,
	}
}

// resolveParseMergePerKeyCap computes the final cap used by parseInternal,
// including source-sensitive grammar guards. Retry scheduling must use this
// same computation so an exact override never narrows below a fresh parse's
// required policy.
func (p *Parser) resolveParseMergePerKeyCap(source []byte, reuse *reuseCursor, maxMergePerKeyOverride int) int {
	mergePerKeyCap := effectiveParseMergePerKeyCap(p.language, parseMaxMergePerKeyValue(), reuse != nil, len(source))
	if javaFullParseNeedsAnnotationDeclarationMergeWidth(p.language, source, reuse) && mergePerKeyCap < javaFullParseRetryMaxMergePerKey {
		mergePerKeyCap = javaFullParseRetryMaxMergePerKey
	}
	// Certified languages keep clean alternatives in the graph. Use one survivor
	// per merge group for fresh full parses. Explicit settings take precedence.
	if reuse == nil && p.language != nil && p.language.FullParseGSSConvergenceEnabled &&
		!parseMaxMergePerKeyEnvConfigured() {
		mergePerKeyCap = 1
	}
	if maxMergePerKeyOverride < 0 {
		mergePerKeyCap = -maxMergePerKeyOverride
	} else if maxMergePerKeyOverride > mergePerKeyCap {
		mergePerKeyCap = maxMergePerKeyOverride
	}
	if mergePerKeyCap > maxStacksPerMergeKeyCeiling {
		mergePerKeyCap = maxStacksPerMergeKeyCeiling
	}
	return mergePerKeyCap
}

func javaFullParseNeedsAnnotationDeclarationMergeWidth(lang *Language, source []byte, reuse *reuseCursor) bool {
	return lang != nil &&
		lang.Name == "java" &&
		reuse == nil &&
		!parseMaxMergePerKeyEnvConfigured() &&
		bytes.Contains(source, []byte("@interface"))
}

func languageName(lang *Language) string {
	if lang == nil {
		return ""
	}
	return lang.Name
}

func languageDefersExactDedupe(lang *Language, noTreeBenchmarkOnly bool) bool {
	if noTreeBenchmarkOnly || lang == nil {
		return false
	}
	switch lang.Name {
	case "dart", "java", "typescript", "tsx", "rust":
		return true
	default:
		return false
	}
}

func (p *Parser) usesGenericFrontierMergeHash() bool {
	return p != nil &&
		p.language != nil &&
		p.language.Name == "perl" &&
		!p.noTreeBenchmarkOnly &&
		!p.compactFullShiftLeaves &&
		!p.pendingFullParents &&
		!p.finalChildRefs
}

type parseStackPrepResult struct {
	stacks     []glrStack
	stopReason ParseStopReason
	stopped    bool
	errorTree  bool
}

func (p *Parser) prepareParseStacksForIteration(stacks []glrStack, scratch *parserScratch, arena *nodeArena, arenaClass arenaClass, maxStacks, maxStackCullTrigger int, phaseTiming bool, glrMergeNanos, glrCullNanos *int64) parseStackPrepResult {
	result := parseStackPrepResult{stacks: stacks}
	resetCRecoveryMergeScratch(&scratch.merge)
	if len(stacks) == 1 {
		if stacks[0].dead {
			result.stop(ParseStopNoStacksAlive, false)
			return result
		}
		scratch.gss.setSingleStackMode(true)
		p.tryDemoteSingleLinearGSS(stacks, scratch)
		clearParseStackEntryCaches(stacks)
		return result
	}
	if reason := p.resultMaterializationStopReason(arena); resultMaterializationShouldStop(reason) {
		result.stop(reason, false)
		return result
	}
	if allParseStacksDead(stacks) {
		result.stop(ParseStopNoStacksAlive, false)
		return result
	}
	p.syncCRecoveryMergeScratch(&scratch.merge)
	scratch.merge.deferExactDedupe = languageDefersExactDedupe(p.language, p.noTreeBenchmarkOnly)
	scratch.merge.frontierMergeHash = p.usesGenericFrontierMergeHash()
	if p.ambiguityProfile != nil {
		p.ambiguityProfile.recordMergeBefore(stacks)
	}
	if p.glrTrace {
		p.traceCRecoverPrepareStacks("pre-merge", stacks)
	}
	if phaseTiming && glrMergeNanos != nil {
		mergeStart := time.Now()
		result.stacks = mergeStacksWithScratch(stacks, &scratch.merge)
		*glrMergeNanos += time.Since(mergeStart).Nanoseconds()
	} else {
		result.stacks = mergeStacksWithScratch(stacks, &scratch.merge)
	}
	// The large-cap merge path (mergeStacksWithScratchLargeCap) polls the
	// runtime memory budget itself at a coarse comparison stride, because its
	// O(survivors^2) grind can allocate multiple GB without returning to any
	// other poll site. Honor a stop it detected before doing anything further
	// with the (possibly partial) merge result.
	if scratch.merge.mergeBudgetStopReason == ParseStopMemoryBudget {
		result.stop(scratch.merge.mergeBudgetStopReason, false)
		return result
	}
	if p.glrTrace {
		p.traceCRecoverPrepareStacks("post-merge", result.stacks)
	}
	if p.ambiguityProfile != nil {
		p.ambiguityProfile.recordMergeAfter(result.stacks)
	}
	if len(result.stacks) == 0 {
		result.stop(ParseStopNoStacksAlive, true)
		return result
	}
	result.stacks = p.cullParseStacksForIteration(result.stacks, scratch, arenaClass, maxStacks, maxStackCullTrigger, phaseTiming, glrCullNanos)
	if p.glrTrace {
		p.traceCRecoverPrepareStacks("post-cull", result.stacks)
	}
	if len(result.stacks) > 1 {
		p.promotePrimaryStack(result.stacks)
	} else {
		p.tryDemoteSingleLinearGSS(result.stacks, scratch)
	}
	scratch.gss.setSingleStackMode(len(result.stacks) == 1)
	clearParseStackEntryCaches(result.stacks)
	return result
}

func (p *Parser) tryDemoteSingleLinearGSS(stacks []glrStack, scratch *parserScratch) bool {
	if p == nil || scratch == nil || len(stacks) != 1 || stacks[0].dead ||
		len(p.pendingForkStacks) != 0 || len(p.pendingFrontierForkStacks) != 0 {
		return false
	}
	depth := stacks[0].depth()
	if !stacks[0].demoteLinearGSS(&scratch.entries) {
		return false
	}
	scratch.gss.demotions++
	scratch.gss.nodesDemoted += uint64(depth)
	if !scratch.audit.enabled && !scratch.audit.equivEnabled {
		p.recycleDemotedGSS(stacks, scratch)
	}
	return true
}

// recycleDemotedGSS reclaims GSS slab slots only after the sole live stack has
// been materialized into entry scratch. The ordering is intentional: first
// invalidate address-keyed caches and clear stale stack holders, then overwrite
// the slab nodes. Runtime/equivalence audits stay on the append-only path
// because their attribution state deliberately retains node identity across
// token boundaries. C-recovery remains safe because its live version state is
// carried by glrStack/cRecoverState, while both GSS prefix paths are cleared
// here and the aggregate generation is advanced before address reuse.
func (p *Parser) recycleDemotedGSS(stacks []glrStack, scratch *parserScratch) {
	if p == nil || scratch == nil || len(stacks) != 1 || stacks[0].gss.head != nil ||
		len(stacks[0].entries) == 0 || len(p.pendingForkStacks) != 0 ||
		len(p.pendingFrontierForkStacks) != 0 || scratch.gss.usedTotal == 0 {
		return
	}

	live := stacks[0]
	scratch.merge.invalidateGSSPointersForReuse()
	resetGSSPrefixPath(&p.cPrefixPath)
	gssPrefixAggGen.Add(1)
	if cap(scratch.merge.result) > 0 {
		clear(scratch.merge.result[:cap(scratch.merge.result)])
		scratch.merge.result = scratch.merge.result[:0]
	}
	if cap(stacks) > 0 {
		clear(stacks[:cap(stacks)])
		stacks[0] = live
	}
	// Keep one bounded reserve for later fork bursts. Drop an oversized active
	// backing array without scanning it before the GSS slabs are recycled.
	p.resetPendingStackBuffersAfterDemotion()
	scratch.gss.recycleForParse()
}

func (p *Parser) checkpointTransientScratch(stacks []glrStack, scratch *parserScratch, arena *nodeArena) ParseStopReason {
	if p == nil || scratch == nil || arena == nil || scratch.transientCheckpointBytes <= 0 ||
		// Pending-parent payloads can retain transient nodes outside the
		// semantic/raw-shape graph walked below. Keep the checkpoint opt-in
		// disabled for that independently experimental representation until its
		// payload graph is materialized at the recycle boundary too.
		p.pendingFullParents ||
		len(stacks) != 1 || stacks[0].dead || stacks[0].gss.head != nil || len(stacks[0].entries) == 0 ||
		len(p.pendingForkStacks) != 0 || len(p.pendingFrontierForkStacks) != 0 ||
		scratch.reduce.transientParents == nil || scratch.reduce.transientChildren == nil ||
		scratch.audit.enabled || scratch.audit.equivEnabled || p.crecoveryEnteredErrorState {
		return ParseStopNone
	}
	used := scratch.transientParents.usedBytes() + scratch.transientChildren.usedBytes()
	if used < scratch.transientCheckpointBytes {
		return ParseStopNone
	}
	if reason := materializeTransientParentEntriesForRecycle(
		stacks[0].entries,
		arena,
		scratch.reduce.transientParents,
		scratch.reduce.transientChildren,
		p,
	); resultMaterializationShouldStop(reason) {
		return reason
	}
	// The materialized live stack, including its raw-shape-only sidecar graph,
	// no longer references transient slabs. Bump merge and recovery-memo epochs
	// before slab addresses are reused, so pointer-keyed results from discarded
	// alternatives cannot alias freshly allocated transient parents.
	scratch.merge.beginEquivEpoch()
	p.beginCNodeMemoEpoch()
	if cap(scratch.tmpEntries) > 0 {
		clear(scratch.tmpEntries[:cap(scratch.tmpEntries)])
		scratch.tmpEntries = scratch.tmpEntries[:0]
	}
	scratch.transientParents.recycleForParse()
	scratch.transientChildren.recycleForParse()
	scratch.transientCheckpoints++
	return ParseStopNone
}

func (p *Parser) traceCRecoverPrepareStacks(label string, stacks []glrStack) {
	interesting := false
	for i := range stacks {
		if stacks[i].cRec != nil || stacks[i].cRecoverMissingGroup != nil {
			interesting = true
			break
		}
	}
	if !interesting {
		return
	}
	fmt.Printf("[GLR] %s recovery stacks=%d\n", label, len(stacks))
	for i := range stacks {
		fmt.Printf("  %s[%d]: kind=%s st=%d dead=%v shift=%v dep=%d score=%d byte=%d\n",
			label,
			i,
			cRecoverStackTraceKind(&stacks[i]),
			stacks[i].top().state,
			stacks[i].dead,
			stacks[i].shifted,
			stacks[i].depth(),
			stacks[i].score,
			stacks[i].byteOffset,
		)
	}
}

func (r *parseStackPrepResult) stop(reason ParseStopReason, errorTree bool) {
	r.stopReason = reason
	r.stopped = true
	r.errorTree = errorTree
}

func allParseStacksDead(stacks []glrStack) bool {
	for i := range stacks {
		if !stacks[i].dead {
			return false
		}
	}
	return true
}

func allLiveUnacceptedStacksShifted(stacks []glrStack) bool {
	liveUnaccepted := 0
	for i := range stacks {
		if stacks[i].dead || stacks[i].accepted {
			continue
		}
		liveUnaccepted++
		if !stacks[i].shifted {
			return false
		}
	}
	return liveUnaccepted > 0
}

func (p *Parser) cullParseStacksForIteration(stacks []glrStack, scratch *parserScratch, arenaClass arenaClass, maxStacks, maxStackCullTrigger int, phaseTiming bool, glrCullNanos *int64) []glrStack {
	if len(stacks) <= maxStackCullTrigger {
		return stacks
	}
	if p.glrTrace {
		p.traceParseStackCull("pre-cull", stacks, maxStacks, maxStackCullTrigger)
	}
	if perfCountersEnabled {
		perfRecordGlobalCapCull(len(stacks), maxStacks)
	}
	cullIn := len(stacks)
	cullLang := stackCullLanguageForArena(p.language, arenaClass)
	if phaseTiming && glrCullNanos != nil {
		cullStart := time.Now()
		stacks = retainTopStacksForLanguageWithScratch(stacks, maxStacks, cullLang, &scratch.stackPick, &scratch.stackKeep, &scratch.stackCull)
		*glrCullNanos += time.Since(cullStart).Nanoseconds()
	} else {
		stacks = retainTopStacksForLanguageWithScratch(stacks, maxStacks, cullLang, &scratch.stackPick, &scratch.stackKeep, &scratch.stackCull)
	}
	scratch.audit.recordGlobalCull(cullIn, len(stacks))
	workCountRecordBoundaryCull(p, cullIn, len(stacks))
	if p.glrTrace {
		p.traceParseStackCull("kept", stacks, maxStacks, maxStackCullTrigger)
	}
	return stacks
}

func (p *Parser) traceParseStackCull(label string, stacks []glrStack, maxStacks, trigger int) {
	if label == "pre-cull" {
		fmt.Printf("[GLR] CAP CULL: %d stacks -> keep %d (trigger=%d)\n", len(stacks), maxStacks, trigger)
	} else {
		fmt.Printf("[GLR] after cull:\n")
	}
	for ci := range stacks {
		fmt.Printf("  %s[%d]: st=%d dead=%v shift=%v dep=%d score=%d byte=%d\n",
			label, ci, stacks[ci].top().state, stacks[ci].dead, stacks[ci].shifted, stacks[ci].depth(), stacks[ci].score, stacks[ci].byteOffset)
	}
}

func clearParseStackEntryCaches(stacks []glrStack) {
	for i := range stacks {
		stacks[i].cacheEntries = false
		if stacks[i].gss.head != nil {
			stacks[i].entries = nil
			stacks[i].invalidateCEntryAgg()
		}
	}
}

type parseMissingShiftTracker struct {
	lastState     StateID
	lastDepth     int
	lastSymbol    Symbol
	lastStartByte uint32
	lastEndByte   uint32
	consecutive   int
	stateScratch  missingShiftStateScratch
}

type missingShiftStateScratch struct {
	baseStates   []StateID
	reduceStates []StateID
	simStates    []StateID
}

func (s *missingShiftStateScratch) reset() {
	if s == nil {
		return
	}
	s.baseStates = s.baseStates[:0]
	s.reduceStates = s.reduceStates[:0]
	s.simStates = s.simStates[:0]
}

func (t *parseMissingShiftTracker) resetForToken() {
	t.lastDepth = -1
	t.consecutive = 0
	t.stateScratch.reset()
}

func (t *parseMissingShiftTracker) tryInsert(p *Parser, source []byte, stackIndex int, s *glrStack, currentState StateID, tok Token, ts TokenSource, nodeCount *int, arena *nodeArena, scratch *parserScratch, trackChildErrors *bool) bool {
	missingShiftDepth := s.depth()
	if t.matches(currentState, missingShiftDepth, tok) && t.consecutive >= maxConsecutiveMissingSingleShifts {
		if p.glrTrace {
			fmt.Printf("  stack[%d] SKIP missing-shift cycle state=%d sym=%d byte=%d..%d count=%d\n",
				stackIndex, currentState, tok.Symbol, tok.StartByte, tok.EndByte, t.consecutive)
		}
		return false
	}
	if !p.tryInsertMissingSingleShift(source, s, tok, ts, nodeCount, arena, &scratch.entries, &scratch.gss, &t.stateScratch, trackChildErrors) {
		return false
	}
	if t.matches(currentState, missingShiftDepth, tok) {
		t.consecutive++
		return true
	}
	t.lastState = currentState
	t.lastDepth = missingShiftDepth
	t.lastSymbol = tok.Symbol
	t.lastStartByte = tok.StartByte
	t.lastEndByte = tok.EndByte
	t.consecutive = 1
	return true
}

func (t *parseMissingShiftTracker) matches(state StateID, depth int, tok Token) bool {
	return t.lastState == state &&
		t.lastDepth == depth &&
		t.lastSymbol == tok.Symbol &&
		t.lastStartByte == tok.StartByte &&
		t.lastEndByte == tok.EndByte
}

func (p *Parser) tryReuseCurrentParseSubtree(s *glrStack, tok Token, ts TokenSource, reuse *reuseCursor, scratch *parserScratch, arena *nodeArena, reuseState *parseReuseState, timing *incrementalParseTiming) (Token, bool) {
	if timing == nil {
		nextTok, _, ok := p.tryReuseSubtree(s, tok, ts, reuse, &scratch.entries, &scratch.gss)
		if ok {
			reuseState.markReused(stackEntryNode(s.top()), arena)
		}
		return nextTok, ok
	}
	reuseStart := time.Now()
	nextTok, reusedBytes, ok := p.tryReuseSubtree(s, tok, ts, reuse, &scratch.entries, &scratch.gss)
	timing.reuseNanos += time.Since(reuseStart).Nanoseconds()
	if !ok {
		return nextTok, false
	}
	timing.reusedSubtrees++
	timing.reusedBytes += uint64(reusedBytes)
	reuseState.markReused(stackEntryNode(s.top()), arena)
	return nextTok, true
}

func (p *Parser) traceParseIteration(iter int, tok Token, stacks []glrStack, needToken bool) {
	symName := "?"
	if int(tok.Symbol) < len(p.language.SymbolNames) {
		symName = p.language.SymbolNames[tok.Symbol]
	}
	fmt.Printf("[GLR] iter=%d tok=%s(%d)[%d-%d] stacks=%d needTok=%v\n",
		iter, symName, tok.Symbol, tok.StartByte, tok.EndByte, len(stacks), needToken)
	for si := range stacks {
		fmt.Printf("  s[%d]: st=%d dead=%v shift=%v dep=%d byte=%d\n",
			si, stacks[si].top().state, stacks[si].dead, stacks[si].shifted, stacks[si].depth(), stacks[si].byteOffset)
	}
}

func parseStacksShareState(stacks []glrStack, state StateID) bool {
	if len(stacks) == 1 {
		return true
	}
	for i := range stacks {
		if stacks[i].dead {
			continue
		}
		if stacks[i].top().state != state {
			return false
		}
	}
	return true
}

func (p *Parser) actionsForParseState(state StateID, symbol Symbol, parseActions []ParseActionEntry) []ParseAction {
	actionIdx := p.lookupActionIndex(state, symbol)
	if actionIdx == 0 || int(actionIdx) >= len(parseActions) {
		return nil
	}
	return parseActions[actionIdx].Actions
}

// forestResolveConflict applies the same deterministic conflict tie-breaks the
// production GLR loop uses (the `switch p.language.Name` block) to the GSS-forest
// fast path. The forest disambiguates surviving alternatives purely by subtree
// score (dynamic precedence); when two interpretations tie at score 0 the forest
// keeps whichever coalesced first, which does not always match C tree-sitter's
// associativity-driven resolution. For the conflicts that C resolves at table
// generation via prec/associativity (not a runtime dynamic-precedence number),
// we collapse the action set to the single C-preferred action so the forest
// builds the matching shape. When no scoped rule applies, the full action set is
// returned unchanged and the forest's normal multi-action handling proceeds.
func (p *Parser) forestResolveConflict(state StateID, tok Token, actions []ParseAction) []ParseAction {
	if p == nil || p.language == nil || len(actions) < 2 {
		return actions
	}
	// The dot-only fallback that used to live here (dotRepetitionShiftConflictChoice)
	// is retired, not migrated: dot was never opted out of the engine-wide C
	// repetition-skip fold (cRepetitionSkipForestConflictChoice, applied by
	// the forest dispatch loop right after this function returns), which
	// already folds its stmt_list boundary with a flat (non-growing) parse
	// stack. Reviving the old helper's repetition-shift preference as a
	// certified policy instead grew the stack O(n) with statement count for
	// no behavioral benefit (see the "NOTE on dot" comment in
	// grammars/runtime_profiles.go).
	if chosen, ok := conflictPolicyChoice(p.language, tok, state, actions); ok {
		return p.forestSingletonActions(chosen)
	}
	if generatedRepeatBoundaryConflict(p.language, actions) {
		return actions
	}
	if p.language.GeneratedByGrammargen {
		return actions
	}
	return actions
}

// forestSingletonActions returns a reusable one-element action slice holding the
// chosen action, avoiding a per-call allocation in the forest hot loop.
func (p *Parser) forestSingletonActions(act ParseAction) []ParseAction {
	p.forestConflictChoice[0] = act
	return p.forestConflictChoice[:1]
}

func (p *Parser) traceStackActions(stackIndex int, state StateID, symbol Symbol, actions []ParseAction) {
	if !p.glrTrace {
		return
	}
	actionIdx := p.lookupActionIndex(state, symbol)
	fmt.Printf("  stack[%d] state=%d actionIdx=%d actions=%d\n", stackIndex, state, actionIdx, len(actions))
	for ai, action := range actions {
		fmt.Printf("    action[%d]: type=%d state=%d sym=%d cnt=%d prec=%d\n",
			ai, action.Type, action.State, action.Symbol, action.ChildCount, action.DynamicPrecedence)
	}
}

func deterministicExternalConflictAction(actions []ParseAction) ParseAction {
	chosen, _, _ := deterministicExternalConflictActionAt(actions)
	return chosen
}

func deterministicExternalConflictActionAt(actions []ParseAction) (ParseAction, int, bool) {
	if len(actions) == 0 {
		return ParseAction{}, -1, false
	}
	chosen := actions[0]
	chosenOrdinal := 0
	for ai := 1; ai < len(actions); ai++ {
		cand := actions[ai]
		if cand.Type == ParseActionShift {
			return cand, ai, true
		}
		if chosen.Type == ParseActionReduce && cand.Type == ParseActionReduce && cand.DynamicPrecedence > chosen.DynamicPrecedence {
			chosen = cand
			chosenOrdinal = ai
		}
	}
	return chosen, chosenOrdinal, true
}

func (p *Parser) traceParseFork(currentState StateID, actions []ParseAction) {
	fmt.Printf("[GLR] FORK: %d actions from state=%d\n", len(actions), currentState)
	for ai, action := range actions {
		symName := "?"
		if int(action.Symbol) < len(p.language.SymbolNames) {
			symName = p.language.SymbolNames[action.Symbol]
		}
		fmt.Printf("  action[%d]: type=%d state=%d sym=%s(%d) cnt=%d prec=%d\n",
			ai, action.Type, action.State, symName, action.Symbol, action.ChildCount, action.DynamicPrecedence)
	}
}

func bestRetryRecoveryStack(stacks []glrStack) int {
	bestIdx := 0
	for si := 1; si < len(stacks); si++ {
		if stacks[si].score > stacks[bestIdx].score {
			bestIdx = si
			continue
		}
		if stacks[si].score == stacks[bestIdx].score && stacks[si].depth() < stacks[bestIdx].depth() {
			bestIdx = si
		}
	}
	return bestIdx
}

func (p *Parser) updateParserStateTokenSource(ts TokenSource, stacks []glrStack, scratch *parserScratch) {
	p.cRecoverSharedTokenErrorModeLexed = false
	stateful, ok := ts.(parserStateTokenSource)
	if !ok || len(stacks) == 0 {
		return
	}
	// Faithful C recovery port (parser_recover_c.go): C versions each lex
	// with their own state's mode, so the merged error version's ERROR_STATE
	// mode — where every token, external tokens included, is valid — never
	// leaks into another version's lexing. This engine lexes once for all
	// stacks; keep absorbing stacks (head at the C error state) out of the
	// primary-state choice and the GLR lex union while any normally-parsing
	// stack is live. When only absorbing stacks remain, ERROR_STATE drives
	// the lex exactly like C's error-mode lexing.
	excludeAbsorbing := false
	primary := stacks[0].top().state
	if p.errorCostCompetitionEnabled() {
		for si := range stacks {
			if stacks[si].dead {
				continue
			}
			if stacks[si].cRec != nil && stacks[si].top().state == cErrorState {
				continue
			}
			primary = stacks[si].top().state
			excludeAbsorbing = true
			break
		}
		if primary == cErrorState {
			if em, ok := ts.(errorModeLexingTokenSource); ok && em.lexesErrorModeAtErrorState() {
				// Every live stack is absorbing and the source will lex this
				// token with the ERROR-state mode — C-equivalent error-mode
				// lookahead; the strategy-1 election can trust it directly.
				p.cRecoverSharedTokenErrorModeLexed = true
			}
		}
	}
	stateful.SetParserState(primary)
	if len(stacks) == 1 || p.usesPrimaryExternalScannerStateForGLR() {
		clearGLRStateTokenSource(stateful, scratch)
		return
	}
	glrBuf := scratch.glrStates[:0]
	if cap(glrBuf) < len(stacks) {
		glrBuf = make([]StateID, 0, len(stacks))
	}
	for si := range stacks {
		if stacks[si].dead {
			continue
		}
		if excludeAbsorbing && stacks[si].cRec != nil && stacks[si].top().state == cErrorState {
			continue
		}
		glrBuf = append(glrBuf, stacks[si].top().state)
	}
	scratch.glrStates = glrBuf
	stateful.SetGLRStates(glrBuf)
}

func (p *Parser) updateCurrentRelexParserStateTokenSource(ts TokenSource, stacks []glrStack, scratch *parserScratch) bool {
	stateful, ok := ts.(parserStateTokenSource)
	if !ok || len(stacks) == 0 {
		return false
	}

	primaryIdx := -1
	excludeAbsorbing := false
	if p.errorCostCompetitionEnabled() {
		for si := range stacks {
			if !currentRelexStateStackEligible(&stacks[si]) {
				continue
			}
			if stacks[si].cRec != nil && stacks[si].top().state == cErrorState {
				continue
			}
			primaryIdx = si
			excludeAbsorbing = true
			break
		}
	}
	if primaryIdx == -1 {
		for si := range stacks {
			if currentRelexStateStackEligible(&stacks[si]) {
				primaryIdx = si
				break
			}
		}
	}
	if primaryIdx == -1 {
		clearGLRStateTokenSource(stateful, scratch)
		return false
	}

	primary := stacks[primaryIdx].top().state
	stateful.SetParserState(primary)
	if p.usesPrimaryExternalScannerStateForGLR() {
		clearGLRStateTokenSource(stateful, scratch)
		return true
	}

	glrBuf := scratch.glrStates[:0]
	if cap(glrBuf) < len(stacks) {
		glrBuf = make([]StateID, 0, len(stacks))
	}
	for si := range stacks {
		if !currentRelexStateStackEligible(&stacks[si]) {
			continue
		}
		if excludeAbsorbing && stacks[si].cRec != nil && stacks[si].top().state == cErrorState {
			continue
		}
		glrBuf = append(glrBuf, stacks[si].top().state)
	}
	scratch.glrStates = glrBuf
	if len(glrBuf) <= 1 {
		clearGLRStateTokenSource(stateful, scratch)
		return true
	}
	stateful.SetGLRStates(glrBuf)
	return true
}

func (p *Parser) tryRelexSingleParserState(tok Token, state StateID, ts TokenSource, stacks []glrStack, scratch *parserScratch) (Token, bool) {
	stateful, statefulOK := ts.(parserStateTokenSource)
	relexer, relexerOK := ts.(tokenSourceRelexer)
	dts := underlyingDFATokenSource(ts)
	if !statefulOK || !relexerOK || dts == nil || !relexer.CanRelexFromTokenStart(tok) {
		return Token{}, false
	}
	snapshot, retainedSnapshot := scratch.snapshotDFARelexState(dts)
	savedState := dts.state
	savedGLRStates := dts.glrStates
	restoreRejectedProbe := func() {
		snapshot.restore(dts)
		scratch.releaseDFARelexSnapshot(retainedSnapshot)
		dts.state = savedState
		dts.glrStates = savedGLRStates
		p.updateCurrentRelexParserStateTokenSource(ts, stacks, scratch)
	}
	stateful.SetParserState(state)
	clearGLRStateTokenSource(stateful, scratch)
	var (
		next Token
		ok   bool
	)
	if ts == dts {
		// The outer snapshot already owns rollback for the direct source.
		// Do not start a redundant nested transaction.
		next, ok = dts.relexFromTokenStartInTransaction(tok)
	} else {
		next, ok = relexer.RelexFromTokenStart(tok)
	}
	actionIndex := p.lookupActionIndex(state, next.Symbol)
	if !ok || !next.ExternalScannerToken || next.StartByte != tok.StartByte || next.EndByte != next.StartByte || (next.Symbol == tok.Symbol && next.StartByte == tok.StartByte && next.EndByte == tok.EndByte) || actionIndex == 0 || int(actionIndex) >= len(p.language.ParseActions) {
		restoreRejectedProbe()
		return Token{}, false
	}
	actions := p.language.ParseActions[actionIndex].Actions
	if len(actions) != 1 || actions[0].Type != ParseActionShift || actions[0].Extra || actions[0].State == 0 || actions[0].State == state {
		restoreRejectedProbe()
		return Token{}, false
	}
	// The returned token and committed scanner checkpoint belong to this parser
	// state. Keep the source isolated through dispatch; the outer loop refreshes
	// the live frontier before its next read, and same-pass re-lex paths set it.
	scratch.releaseDFARelexSnapshot(retainedSnapshot)
	return next, true
}

func (p *Parser) noLiveStackCanAcceptLookahead(stacks []glrStack, tok Token) bool {
	for stackIndex := range stacks {
		candidate := &stacks[stackIndex]
		if candidate.dead || candidate.accepted || candidate.shifted || candidate.cPaused || candidate.depth() == 0 {
			continue
		}
		if p.lookupActionIndex(candidate.top().state, tok.Symbol) != 0 {
			return false
		}
	}
	return true
}

func currentRelexStateStackEligible(s *glrStack) bool {
	return s != nil && !s.dead && !s.shifted && s.depth() > 0
}

func (p *Parser) usesPrimaryExternalScannerStateForGLR() bool {
	if p == nil || p.language == nil || p.language.ExternalScanner == nil {
		return false
	}
	switch p.language.Name {
	case "yaml":
		return true
	case "c_sharp":
		// Pin to the primary stack only while the blob lacks a precise
		// ExternalLexStates table: the union path over-approximates external
		// validity and corrupts the stateful interpolation scanner. With the
		// precise table registered, the per-row scored path
		// (nextGLRScoredExternalToken) lexes each distinct external lex state
		// like C does per version, and MUST see all stack states — pinning
		// here starves interpolation-hole stacks of interpolation_close_brace
		// whenever the primary stack is a competing non-hole interpretation
		// (observed: DeclaredTypeManager.cs `{nameof(...)}` hole close at
		// byte 30694 arriving as a plain `}` and detonating recovery).
		return len(p.language.ExternalLexStates) == 0
	}
	return false
}

func clearGLRStateTokenSource(stateful parserStateTokenSource, scratch *parserScratch) {
	if scratch != nil && len(scratch.glrStates) > 0 {
		scratch.glrStates = scratch.glrStates[:0]
	}
	stateful.SetGLRStates(nil)
}

func (p *Parser) applyExtraShiftAction(s *glrStack, currentState StateID, act ParseAction, tok Token, arena *nodeArena, scratch *parserScratch, trackChildErrors *bool) {
	workCountRecordShift()
	named := p.isNamedSymbol(tok.Symbol)
	targetState := extraShiftTargetState(currentState, act)
	isMissing := p.shiftTokenIsMissingError(tok)
	if p.useCompactNoTreeShiftLeaf() && !isMissing {
		p.applyCompactExtraShiftAction(s, currentState, targetState, tok, named, arena, scratch)
		return
	}
	leaf := newLeafNodeInArena(arena, tok.Symbol, named, tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
	if isMissing {
		leaf.setMissing(true)
		leaf.setHasError(true)
		if trackChildErrors != nil {
			*trackChildErrors = true
		}
	}
	leaf.setExtra(true)
	leaf.setExternalScannerToken(tok.ExternalScannerToken)
	leaf.preGotoState = currentState
	leaf.parseState = targetState
	p.recordCurrentExternalLeafCheckpoint(leaf, tok)
	p.pushStackNode(s, targetState, leaf, &scratch.entries, &scratch.gss)
}

func (p *Parser) applyCompactExtraShiftAction(s *glrStack, currentState, targetState StateID, tok Token, named bool, arena *nodeArena, scratch *parserScratch) {
	if cp, ok := p.currentExternalNoTreeLeafCheckpointRef(arena, tok); ok {
		leaf := newCompactCheckpointLeafInArena(arena, tok.Symbol, named, tok.StartByte, tok.EndByte, cp)
		leaf.setExtra(true)
		leaf.setExternalScannerToken(tok.ExternalScannerToken)
		leaf.preGotoState = currentState
		leaf.parseState = targetState
		p.pushStackCompactCheckpointLeaf(s, targetState, leaf, &scratch.entries, &scratch.gss)
		return
	}
	leaf := newNoTreeLeafNodeInArena(arena, tok.Symbol, named, tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
	leaf.setExtra(true)
	leaf.setExternalScannerToken(tok.ExternalScannerToken)
	leaf.preGotoState = currentState
	leaf.parseState = targetState
	p.pushStackNoTreeNode(s, targetState, leaf, &scratch.entries, &scratch.gss)
}

func (p *Parser) shouldUseTransientReduceChildren(source []byte, reuse *reuseCursor, oldTree *Tree, arenaClass arenaClass) bool {
	return parseTransientReduceChildrenEnabled() &&
		p != nil &&
		p.language != nil &&
		parseTransientReduceChildrenLanguageEnabledForSource(p.language, len(source)) &&
		arenaClass == arenaClassFull &&
		reuse == nil &&
		oldTree == nil &&
		!p.noTreeBenchmarkOnly &&
		!p.noTreeCheckpointBenchmarkOnly &&
		!p.noResultCompatibilityBenchmarkOnly &&
		len(source) > 0
}

func (p *Parser) shouldUseTransientReduceParents(source []byte, reuse *reuseCursor, oldTree *Tree, arenaClass arenaClass) bool {
	return parseTransientReduceParentsEnabled() &&
		p != nil &&
		p.language != nil &&
		parseTransientReduceParentsLanguageEnabledForSource(p.language, len(source)) &&
		arenaClass == arenaClassFull &&
		reuse == nil &&
		oldTree == nil &&
		!p.noTreeBenchmarkOnly &&
		!p.noTreeCheckpointBenchmarkOnly &&
		!p.noResultCompatibilityBenchmarkOnly &&
		len(source) > 0
}

func (p *Parser) shouldUseTransientReduceScratchNoAlias() bool {
	return p != nil &&
		p.transientReduceScratchNoAlias &&
		p.transientChildren != nil
}

func repetitionShiftConflictChoice(actions []ParseAction) (ParseAction, bool) {
	if len(actions) < 2 {
		return ParseAction{}, false
	}
	var shift ParseAction
	shiftFound := false
	reduceFound := false
	for _, act := range actions {
		switch act.Type {
		case ParseActionShift:
			if !act.Repetition || shiftFound {
				return ParseAction{}, false
			}
			shift = act
			shiftFound = true
		case ParseActionReduce:
			reduceFound = true
		default:
			return ParseAction{}, false
		}
	}
	if !shiftFound || !reduceFound {
		return ParseAction{}, false
	}
	return shift, true
}

func (p *Parser) javaSwitchArrowConflictChoice(s *glrStack, tok Token, actions []ParseAction) (ParseAction, bool) {
	if p == nil || p.language == nil || p.language.Name != "java" || s == nil || !symbolHasName(p.language, tok.Symbol, "->") {
		return ParseAction{}, false
	}
	var primaryReduce ParseAction
	var reduceFound bool
	var shiftFound bool
	for _, act := range actions {
		switch act.Type {
		case ParseActionShift:
			shiftFound = true
		case ParseActionReduce:
			if act.ChildCount == 1 && symbolHasName(p.language, act.Symbol, "primary_expression") {
				if reduceFound {
					return ParseAction{}, false
				}
				primaryReduce = act
				reduceFound = true
			}
		default:
			return ParseAction{}, false
		}
	}
	if !shiftFound || !reduceFound {
		return ParseAction{}, false
	}
	// In switch rules, `case A ->` must reduce `A` as the label expression
	// before the arrow. Lambda parameters use the same state but have no
	// post-reduce goto that can consume `->`, so this keeps lambdas intact.
	predecessor, ok := reducePredecessorStateForStack(s, int(primaryReduce.ChildCount))
	if !ok {
		return ParseAction{}, false
	}
	gotoState := p.lookupGoto(predecessor, primaryReduce.Symbol)
	if gotoState == 0 || p.lookupActionIndex(gotoState, tok.Symbol) == 0 {
		return ParseAction{}, false
	}
	return primaryReduce, true
}

func reducePredecessorStateForStack(s *glrStack, childCount int) (StateID, bool) {
	if s == nil || childCount < 0 {
		return 0, false
	}
	if childCount == 0 {
		return s.top().state, true
	}
	if s.gss.head != nil {
		nonExtraFound := 0
		for n := s.gss.head; n != nil; n = n.prev {
			if !stackEntryHasNode(n.entry) || stackEntryNodeIsExtra(n.entry) {
				continue
			}
			nonExtraFound++
			if nonExtraFound != childCount {
				continue
			}
			if n.prev == nil {
				return 0, false
			}
			return n.prev.entry.state, true
		}
		return 0, false
	}
	rr, ok := computeReduceRangePayload(s.entries, childCount)
	if !ok {
		return 0, false
	}
	return rr.topState, true
}

func csharpRepetitionShiftConflictChoice(lang *Language, tok Token, actions []ParseAction) (ParseAction, bool) {
	if lang == nil {
		return ParseAction{}, false
	}
	kind := ""
	for _, act := range actions {
		if act.Type != ParseActionReduce {
			continue
		}
		name := ""
		if int(act.Symbol) < len(lang.SymbolNames) {
			name = lang.SymbolNames[act.Symbol]
		}
		nextKind := csharpRepeatConflictKind(name)
		if nextKind == "" || (kind != "" && kind != nextKind) {
			return ParseAction{}, false
		}
		kind = nextKind
	}
	switch kind {
	case "block":
		if !csharpCanShiftBlockRepetitionToken(lang, tok) {
			return ParseAction{}, false
		}
	case "declaration_list":
		if !csharpCanShiftDeclarationListRepetitionToken(lang, tok) {
			return ParseAction{}, false
		}
	default:
		return ParseAction{}, false
	}
	return repetitionShiftConflictChoice(actions)
}

// kotlinObjectLiteralConflictChoice resolves issue #93: the bundled Kotlin
// table admits a spurious bodiless `object_literal -> object` reduction
// (ChildCount==1, no class_body) that canonical tree-sitter-kotlin does not
// have. Completing it lets a top-level `object Foo { ... }` parse as
// infix_expression(object_literal, simple_identifier, lambda_literal) — the
// bare `object` becomes an anonymous-object expression, `Foo` an infix
// operator, and the body a trailing lambda — instead of object_declaration.
//
// At a shift/reduce conflict that offers that bodiless object_literal reduce
// alongside a shift, prefer the shift. Shifting keeps the parser on the
// declaration path (`object Foo { ... }` -> object_declaration) or, in value
// position, on the object_literal-plus-class_body path (`val x = object { }`
// -> object_literal(class_body)). It never completes a bodiless object_literal,
// so the infix misparse can't form. The anonymous-with-supertype and named
// forms already parse correctly and are unaffected, because this only fires
// when a ChildCount==1 object_literal reduce competes with a shift.
func kotlinObjectLiteralConflictChoice(lang *Language, actions []ParseAction) (ParseAction, bool) {
	if lang == nil {
		return ParseAction{}, false
	}
	// Cheap pre-filter (no symbol lookup): need both a ChildCount==1 reduce and
	// a shift in this conflict before it's worth resolving object_literal.
	var shift ParseAction
	haveShift := false
	haveBodilessReduce := false
	for _, a := range actions {
		switch a.Type {
		case ParseActionShift:
			if !haveShift {
				shift = a
				haveShift = true
			}
		case ParseActionReduce:
			if a.ChildCount == 1 {
				haveBodilessReduce = true
			}
		}
	}
	if !haveShift || !haveBodilessReduce {
		return ParseAction{}, false
	}
	olSym, ok := symbolByName(lang, "object_literal")
	if !ok {
		return ParseAction{}, false
	}
	for _, a := range actions {
		if a.Type == ParseActionReduce && a.ChildCount == 1 && a.Symbol == olSym {
			return shift, true
		}
	}
	return ParseAction{}, false
}

func swiftBraceTypeExpressionConflictChoice(lang *Language, tok Token, state StateID, actions []ParseAction) (ParseAction, bool) {
	if lang == nil {
		return ParseAction{}, false
	}
	if state == 72 && symbolHasName(lang, tok.Symbol, ".") {
		return singleReduceAgainstShiftConflictChoice(lang, actions, "_navigable_type_expression")
	}
	if state != 2815 || !symbolHasName(lang, tok.Symbol, "{") {
		return ParseAction{}, false
	}
	var simpleType ParseAction
	haveSimpleType := false
	haveExpression := false
	for _, act := range actions {
		if act.Type != ParseActionReduce || act.ChildCount != 1 {
			return ParseAction{}, false
		}
		switch {
		case symbolHasName(lang, act.Symbol, "_simple_user_type"):
			if haveSimpleType {
				return ParseAction{}, false
			}
			simpleType = act
			haveSimpleType = true
		case symbolHasName(lang, act.Symbol, "_expression"):
			if haveExpression {
				return ParseAction{}, false
			}
			haveExpression = true
		default:
			return ParseAction{}, false
		}
	}
	if !haveSimpleType || !haveExpression {
		return ParseAction{}, false
	}
	return simpleType, true
}

func singleReduceAgainstShiftConflictChoice(lang *Language, actions []ParseAction, reduceSymbol string) (ParseAction, bool) {
	if len(actions) < 2 {
		return ParseAction{}, false
	}
	var reduce ParseAction
	haveReduce := false
	haveShift := false
	for _, act := range actions {
		switch act.Type {
		case ParseActionReduce:
			if haveReduce || !symbolHasName(lang, act.Symbol, reduceSymbol) {
				return ParseAction{}, false
			}
			reduce = act
			haveReduce = true
		case ParseActionShift:
			if haveShift {
				return ParseAction{}, false
			}
			haveShift = true
		default:
			return ParseAction{}, false
		}
	}
	if !haveReduce || !haveShift {
		return ParseAction{}, false
	}
	return reduce, true
}

func singleReduceAgainstRepetitionShiftConflictChoice(actions []ParseAction) (ParseAction, bool) {
	if len(actions) < 2 {
		return ParseAction{}, false
	}
	var reduce ParseAction
	reduceFound := false
	shiftFound := false
	for _, act := range actions {
		switch act.Type {
		case ParseActionReduce:
			if reduceFound {
				return ParseAction{}, false
			}
			reduce = act
			reduceFound = true
		case ParseActionShift:
			if !act.Repetition || shiftFound {
				return ParseAction{}, false
			}
			shiftFound = true
		default:
			return ParseAction{}, false
		}
	}
	if !reduceFound || !shiftFound {
		return ParseAction{}, false
	}
	return reduce, true
}

func (p *Parser) initSchemeErrorRecoverySymbols(lang *Language) {
	if p == nil || lang == nil || lang.Name != "scheme" {
		return
	}
	p.isScheme = true
	p.schemeDatumSymbol, p.schemeHasDatumSymbol = lang.SymbolByName("_datum")
}

func (p *Parser) initTypeScriptContextualKeywordSymbols(lang *Language) {
	if p == nil || lang == nil {
		return
	}
	switch lang.Name {
	case "typescript", "tsx":
	default:
		return
	}
	p.typeScriptPropertyIdentifierSymbol, p.typeScriptHasPropertyIdentifier = lang.SymbolByName("property_identifier")
	p.typeScriptIdentifierSymbol, p.typeScriptHasIdentifier = lang.SymbolByName("identifier")
	keywords := make(map[string]Symbol, len(typeScriptContextualPropertyKeywordNames))
	for _, name := range typeScriptContextualPropertyKeywordNames {
		if sym, ok := lang.SymbolByName(name); ok {
			keywords[name] = sym
		}
	}
	if len(keywords) != 0 {
		p.typeScriptContextualPropertyKeyword = keywords
	}
}

func (p *Parser) typeScriptContextualPropertyKeywordSymbol(tok Token, source []byte) (Symbol, bool) {
	if p == nil || len(p.typeScriptContextualPropertyKeyword) == 0 || tok.Text == "" {
		return 0, false
	}
	if !(p.typeScriptHasPropertyIdentifier && tok.Symbol == p.typeScriptPropertyIdentifierSymbol) &&
		!(tok.Text == "readonly" && p.typeScriptHasIdentifier && tok.Symbol == p.typeScriptIdentifierSymbol) {
		return 0, false
	}
	keywordSym, ok := p.typeScriptContextualPropertyKeyword[tok.Text]
	if !ok || keywordSym == tok.Symbol {
		return 0, false
	}
	if !typeScriptContextualKeywordHasFollowingOperand(tok, source) {
		return 0, false
	}
	return keywordSym, true
}

func (p *Parser) typeScriptContextualPropertyKeywordHasAction(keywordSym Symbol, state StateID) bool {
	if p == nil || p.language == nil {
		return false
	}
	actionIdx := p.lookupActionIndex(state, keywordSym)
	if actionIdx == 0 || int(actionIdx) >= len(p.language.ParseActions) || len(p.language.ParseActions[actionIdx].Actions) == 0 {
		return false
	}
	return true
}

var typeScriptContextualPropertyKeywordNames = [...]string{
	"abstract", "accessor", "any", "as", "bigint", "boolean", "class", "const",
	"declare", "enum", "export", "extends", "function", "import", "in", "infer",
	"interface", "keyof", "let", "module", "namespace", "never", "new", "number",
	"object", "override", "private", "protected", "public", "readonly", "static",
	"string", "symbol", "type", "typeof", "undefined", "unknown", "void",
}

func typeScriptContextualKeywordHasFollowingOperand(tok Token, source []byte) bool {
	pos := int(tok.EndByte)
	for pos < len(source) {
		switch source[pos] {
		case ' ', '\t', '\n', '\r':
			pos++
			continue
		}
		break
	}
	if pos >= len(source) {
		return false
	}
	switch source[pos] {
	case '(', ')', '{', '}', ';', ',', ':':
		return false
	default:
		return true
	}
}

func allReducesHaveSymbol(lang *Language, actions []ParseAction, name string) bool {
	found := false
	for _, act := range actions {
		if act.Type != ParseActionReduce {
			continue
		}
		if !symbolHasName(lang, act.Symbol, name) {
			return false
		}
		found = true
	}
	return found
}

func symbolHasName(lang *Language, sym Symbol, name string) bool {
	return lang != nil && int(sym) >= 0 && int(sym) < len(lang.SymbolNames) && lang.SymbolNames[sym] == name
}

func csharpRepeatConflictKind(name string) string {
	switch {
	case strings.HasSuffix(name, "block_repeat1"):
		return "block"
	case strings.HasSuffix(name, "declaration_list_repeat1"):
		return "declaration_list"
	default:
		return ""
	}
}

func csharpCanShiftBlockRepetitionToken(lang *Language, tok Token) bool {
	if int(tok.Symbol) >= len(lang.SymbolNames) {
		return false
	}
	switch lang.SymbolNames[tok.Symbol] {
	case "this", "base":
		return true
	case "identifier":
		return tok.Text != "scoped"
	default:
		return false
	}
}

func csharpCanShiftDeclarationListRepetitionToken(lang *Language, tok Token) bool {
	if int(tok.Symbol) >= len(lang.SymbolNames) {
		return false
	}
	switch lang.SymbolNames[tok.Symbol] {
	case "abstract", "class", "const", "delegate", "enum", "event", "extern", "file",
		"fixed", "global", "implicit", "interface", "internal", "namespace", "new",
		"operator", "override", "partial", "private", "protected", "public", "readonly",
		"record", "ref", "sealed", "static", "struct", "unsafe", "using", "virtual",
		"volatile":
		return true
	default:
		return false
	}
}

func compactAcceptedStacks(stacks []glrStack) []glrStack {
	acceptedCount := 0
	for i := range stacks {
		if stacks[i].accepted {
			stacks[acceptedCount] = stacks[i]
			acceptedCount++
		}
	}
	return stacks[:acceptedCount]
}

func stackCullLanguageForArena(lang *Language, class arenaClass) *Language {
	if class != arenaClassFull && lang != nil && lang.Name == "bash" {
		// Incremental culling historically used the generic stack comparator
		// for Bash. Keep that tie-break order while still reusing scratch.
		return nil
	}
	return lang
}

// glrStackCullTrigger returns the live-stack population at which per-iteration
// culling engages. The slack window is a function of the language only: which
// memory pool the arena came from must never influence which GLR branches
// survive, or incremental parses select different trees than fresh parses of
// the same source (the early_newline canonical witness).
func glrStackCullTrigger(maxStacks int, langName string) int {
	if maxStacks <= 0 {
		return maxStacks
	}
	if langName == "c_sharp" {
		return maxStacks
	}
	maxInt := int(^uint(0) >> 1)
	if maxStacks > maxInt-fullParseGLRStackOverflow {
		return maxInt
	}
	return maxStacks + fullParseGLRStackOverflow
}

func maxTransientFrontierPopulationCap(maxStacks, maxStackCullTrigger int) int {
	if maxStacks <= 0 {
		return maxStackCullTrigger
	}
	maxInt := int(^uint(0) >> 1)
	overflowWindow := maxInt
	if maxStacks <= maxInt-fullParseGLRStackOverflow {
		overflowWindow = maxStacks + fullParseGLRStackOverflow
	}
	if maxStackCullTrigger > overflowWindow {
		return maxStackCullTrigger
	}
	return overflowWindow
}

func transientFrontierPopulationCap(maxStacks, maxStackCullTrigger int, noResultCompatibilityBenchmarkOnly bool) int {
	cap := maxTransientFrontierPopulationCap(maxStacks, maxStackCullTrigger)
	if noResultCompatibilityBenchmarkOnly && maxStackCullTrigger <= maxStacks {
		return 0
	}
	return cap
}

func (p *Parser) promotePrimaryStack(stacks []glrStack) {
	if len(stacks) <= 1 {
		return
	}
	best := 0
	for i := 1; i < len(stacks); i++ {
		if stackComparePtr(&stacks[i], &stacks[best]) > 0 {
			best = i
		}
	}
	if best != 0 {
		stacks[0], stacks[best] = stacks[best], stacks[0]
	}
}

type stackCullKey struct {
	state      StateID
	byteOffset uint32
	score      int
	hash       uint64
	depth      int
	branch     uint64
	errorRank  uint8
	flags      uint8
}

const (
	stackCullDeadFlag = 1 << iota
	stackCullAcceptedFlag
	stackCullShiftedFlag
)

func buildStackCullKeys(stacks []glrStack, lang *Language, buf *[]stackCullKey) []stackCullKey {
	if len(stacks) == 0 {
		if buf != nil {
			*buf = (*buf)[:0]
		}
		return nil
	}
	var keys []stackCullKey
	if buf != nil {
		if cap(*buf) < len(stacks) {
			*buf = make([]stackCullKey, len(stacks))
		}
		keys = (*buf)[:len(stacks)]
	} else {
		keys = make([]stackCullKey, len(stacks))
	}
	needHash := lang != nil && (lang.Name == "c_sharp" || lang.Name == "bash")
	for i := range stacks {
		s := &stacks[i]
		top := s.top()
		flags := uint8(0)
		if s.dead {
			flags |= stackCullDeadFlag
		}
		if s.accepted {
			flags |= stackCullAcceptedFlag
		}
		if s.shifted {
			flags |= stackCullShiftedFlag
		}
		errorRank := uint8(0)
		if stackEntryNodeHasError(top) {
			errorRank = 1
		}
		keys[i] = stackCullKey{
			state:      top.state,
			byteOffset: s.byteOffset,
			score:      s.score,
			depth:      s.depth(),
			branch:     s.branchOrder,
			errorRank:  errorRank,
			flags:      flags,
		}
		if needHash {
			keys[i].hash = stackHash(s)
		}
	}
	return keys
}

func compareStackCullKeys(lang *Language, a, b stackCullKey) int {
	useHashTieBreak := lang != nil && (lang.Name == "c_sharp" || lang.Name == "bash")
	aDead := a.flags&stackCullDeadFlag != 0
	bDead := b.flags&stackCullDeadFlag != 0
	if aDead != bDead {
		if aDead {
			return -1
		}
		return 1
	}
	aAccepted := a.flags&stackCullAcceptedFlag != 0
	bAccepted := b.flags&stackCullAcceptedFlag != 0
	if aAccepted != bAccepted {
		if aAccepted {
			return 1
		}
		return -1
	}
	if a.errorRank != b.errorRank {
		if a.errorRank < b.errorRank {
			return 1
		}
		return -1
	}
	if a.score != b.score {
		if a.score > b.score {
			return 1
		}
		return -1
	}
	if !useHashTieBreak {
		aShifted := a.flags&stackCullShiftedFlag != 0
		bShifted := b.flags&stackCullShiftedFlag != 0
		if aShifted != bShifted {
			if !aShifted {
				return 1
			}
			return -1
		}
	}
	if a.depth != b.depth {
		if lang != nil && lang.Name == "python" {
			if a.depth < b.depth {
				return 1
			}
			return -1
		}
		if a.depth > b.depth {
			return 1
		}
		return -1
	}
	if a.byteOffset != b.byteOffset {
		if a.byteOffset > b.byteOffset {
			return 1
		}
		return -1
	}
	if useHashTieBreak {
		aShifted := a.flags&stackCullShiftedFlag != 0
		bShifted := b.flags&stackCullShiftedFlag != 0
		if aShifted != bShifted {
			if !aShifted {
				return 1
			}
			return -1
		}
		if a.hash > b.hash {
			return 1
		}
		if a.hash < b.hash {
			return -1
		}
	}
	if a.branch != b.branch {
		if a.branch < b.branch {
			return 1
		}
		return -1
	}
	return 0
}

type preMaterializationFieldRejectKey struct {
	state      StateID
	byteOffset uint32
}

func (p *Parser) observePreMaterializationFieldRejectFork(s *glrStack, actions []ParseAction, tmp []stackEntry, perKeyCap int) (candidates, sameKeyCandidates, overflowCandidates uint64) {
	if p == nil || s == nil || p.noTreeBenchmarkOnly || !p.usePendingFullParents() || len(actions) <= 1 {
		return 0, 0, 0
	}
	if perKeyCap <= 0 {
		perKeyCap = maxStacksPerMergeKey
	}
	type keyCount struct {
		key   preMaterializationFieldRejectKey
		count int
	}
	var fixed [8]keyCount
	keys := fixed[:0]
	for _, act := range actions {
		if act.Type != ParseActionReduce || !p.reduceProductionHasEffectiveFields(int(act.ChildCount), act.ProductionID, nil) {
			continue
		}
		key, ok := p.preMaterializationFieldRejectKey(s, act, tmp)
		if !ok {
			continue
		}
		candidates++
		found := false
		for i := range keys {
			if keys[i].key == key {
				keys[i].count++
				found = true
				break
			}
		}
		if found {
			continue
		}
		if len(keys) == cap(keys) {
			next := make([]keyCount, len(keys), len(keys)*2)
			copy(next, keys)
			keys = next
		}
		keys = append(keys, keyCount{key: key, count: 1})
	}
	for i := range keys {
		if keys[i].count <= 1 {
			continue
		}
		sameKeyCandidates += uint64(keys[i].count)
		if keys[i].count > perKeyCap {
			overflowCandidates += uint64(keys[i].count - perKeyCap)
		}
	}
	return candidates, sameKeyCandidates, overflowCandidates
}

func (p *Parser) preMaterializationFieldRejectKey(s *glrStack, act ParseAction, tmp []stackEntry) (preMaterializationFieldRejectKey, bool) {
	if p == nil || s == nil {
		return preMaterializationFieldRejectKey{}, false
	}
	entries, _ := s.entriesForRead(tmp)
	window, ok := computeReduceRangeForFullPayloads(entries, int(act.ChildCount), p.usePendingFullParents())
	if !ok {
		return preMaterializationFieldRejectKey{}, false
	}
	targetState := window.topState
	if gotoState := p.lookupGoto(window.topState, act.Symbol); gotoState != 0 {
		targetState = gotoState
	}
	byteOffset := s.byteOffset
	if window.actualEnd > window.start {
		byteOffset = stackEntryNodeEndByte(entries[window.actualEnd-1])
	}
	return preMaterializationFieldRejectKey{
		state:      targetState,
		byteOffset: byteOffset,
	}, true
}

func retainTopStacks(stacks []glrStack, keep int) []glrStack {
	return retainTopStacksForLanguage(stacks, keep, nil)
}

func retainTopStacksForLanguage(stacks []glrStack, keep int, lang *Language) []glrStack {
	return retainTopStacksForLanguageWithScratch(stacks, keep, lang, nil, nil, nil)
}

func retainTopStacksForLanguageWithScratch(stacks []glrStack, keep int, lang *Language, selectedBuf *[]int, chosenBuf *[]bool, keyBuf *[]stackCullKey) []glrStack {
	if keep <= 0 {
		return stacks[:0]
	}
	if len(stacks) <= keep {
		return stacks
	}
	compareLang := lang
	if keyBuf == nil {
		// Preserve the former no-key fallback semantics. That path used the
		// C#-specific comparator, but all other languages followed the generic
		// stack comparator even if the keyed full-parse path has language
		// tie-breakers.
		if compareLang == nil || compareLang.Name != "c_sharp" {
			compareLang = nil
		}
	}
	keys := buildStackCullKeys(stacks, compareLang, keyBuf)
	return retainTopStacksByKeys(stacks, keep, compareLang, keys, selectedBuf, chosenBuf)
}

func retainTopStacksByKeys(stacks []glrStack, keep int, lang *Language, keys []stackCullKey, selectedBuf *[]int, chosenBuf *[]bool) []glrStack {
	// Preserve one strong representative per top state before filling the
	// remaining cap. Otherwise a burst of near-duplicate stacks from one state
	// can crowd out a shallower but semantically distinct branch.
	var selected []int
	if selectedBuf != nil {
		if cap(*selectedBuf) < len(stacks) {
			*selectedBuf = make([]int, 0, len(stacks))
		}
		selected = (*selectedBuf)[:0]
	} else {
		selected = make([]int, 0, len(stacks))
	}
	for i := range stacks {
		state := keys[i].state
		bestIdx := -1
		for j, selectedIdx := range selected {
			if keys[selectedIdx].state == state {
				bestIdx = j
				break
			}
		}
		if bestIdx >= 0 {
			if compareStackCullKeys(lang, keys[i], keys[selected[bestIdx]]) > 0 {
				selected[bestIdx] = i
			}
			continue
		}
		selected = append(selected, i)
	}
	for i := 0; i < len(selected); i++ {
		best := i
		for j := i + 1; j < len(selected); j++ {
			if compareStackCullKeys(lang, keys[selected[j]], keys[selected[best]]) > 0 {
				best = j
			}
		}
		if best != i {
			selected[i], selected[best] = selected[best], selected[i]
		}
	}
	if len(selected) > keep {
		selected = selected[:keep]
	}

	var chosen []bool
	if chosenBuf != nil {
		if cap(*chosenBuf) < len(stacks) {
			*chosenBuf = make([]bool, len(stacks))
		}
		chosen = (*chosenBuf)[:len(stacks)]
		clear(chosen)
	} else {
		chosen = make([]bool, len(stacks))
	}
	for _, idx := range selected {
		chosen[idx] = true
	}
	for len(selected) < keep {
		best := -1
		for i := range stacks {
			if chosen[i] {
				continue
			}
			if best < 0 || compareStackCullKeys(lang, keys[i], keys[best]) > 0 {
				best = i
			}
		}
		if best < 0 {
			break
		}
		chosen[best] = true
		selected = append(selected, best)
	}
	for i := 0; i < len(selected); i++ {
		idx := selected[i]
		if idx == i {
			continue
		}
		stacks[i], stacks[idx] = stacks[idx], stacks[i]
		keys[i], keys[idx] = keys[idx], keys[i]
		for j := i + 1; j < len(selected); j++ {
			if selected[j] == i {
				selected[j] = idx
				break
			}
		}
	}
	return stacks[:len(selected)]
}

func classifyConflictShape(actions []ParseAction) (rrConflict, rsConflict bool) {
	if len(actions) < 2 {
		return false, false
	}
	reduceCount := 0
	hasShift := false
	hasOther := false
	for i := range actions {
		switch actions[i].Type {
		case ParseActionReduce:
			reduceCount++
		case ParseActionShift:
			hasShift = true
		default:
			hasOther = true
		}
	}
	if hasOther || reduceCount == 0 {
		return false, false
	}
	if hasShift {
		return false, true
	}
	return reduceCount >= 2, false
}
