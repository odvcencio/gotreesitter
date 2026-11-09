//go:build gts_workcount

package gotreesitter

import "math"

// DiagnosticWorkCount is the diagnostic-only Go GLR counter contract. Direct
// counters describe parser actions or selected-tree events with a common
// semantic definition. Fields ending in Proxy describe implementation work;
// their values must not be presented as if Go and C used identical machinery.
type DiagnosticWorkCount struct {
	Contract string `json:"contract"`
	Overflow bool   `json:"overflow"`
	DiagnosticWorkCountValues

	Attempts       []DiagnosticWorkCountAttempt   `json:"attempts"`
	OutsideAttempt DiagnosticWorkCountValues      `json:"outside_attempt"`
	Convergence    DiagnosticWorkCountConvergence `json:"convergence_frontier"`
	boardDirect    DiagnosticWorkCountBoardDirect
}

// DiagnosticRetryTrace records parse-attempt boundaries without enabling the
// work counters or the convergence observer. Use it when an observer must not
// walk or allocate from the live graph.
type DiagnosticRetryTrace struct {
	Attempts []DiagnosticWorkCountAttempt `json:"attempts"`
}

// DiagnosticWorkCountBoardDirect contains board events whose definitions
// are shared with the locked C diagnostic oracle. It is emitted beside, not
// inside, the immutable gts-work-count/v2 counter object.
type DiagnosticWorkCountBoardDirect struct {
	Schema                                 string `json:"schema"`
	FrontierLexerElectionsAvailable        bool   `json:"frontier_lexer_elections_available"`
	FrontierLexerElections                 uint64 `json:"frontier_lexer_elections"`
	PerVersionLexRequestsAvailable         bool   `json:"per_version_lex_requests_available"`
	PerVersionLexRequests                  uint64 `json:"per_version_lex_requests"`
	RawMainLexerInvocations                uint64 `json:"raw_main_lexer_invocations"`
	ResolvedActionCellsExamined            uint64 `json:"resolved_action_cells_examined"`
	RawActionEntriesBeyondFirst            uint64 `json:"raw_action_entries_beyond_first"`
	ConflictActionArmsAdmitted             uint64 `json:"conflict_action_arms_admitted"`
	CausalConflictForks                    uint64 `json:"causal_conflict_forks"`
	PredecessorLinkUnionAttempts           uint64 `json:"predecessor_link_union_attempts"`
	PredecessorLinkUnionDuplicateNoop      uint64 `json:"predecessor_link_union_duplicate_noop"`
	PredecessorLinkUnionPrecedenceReplaced uint64 `json:"predecessor_link_union_precedence_replaced"`
	PredecessorLinkUnionRecursiveChanged   uint64 `json:"predecessor_link_union_recursive_changed"`
	PredecessorLinkUnionAlternateAppended  uint64 `json:"predecessor_link_union_alternate_appended"`
	PredecessorLinkUnionRejected           uint64 `json:"predecessor_link_union_rejected"`
	AlternatePredecessorLinksAppended      uint64 `json:"alternate_predecessor_links_appended"`
	RawSelectedInternalNodes               uint64 `json:"raw_selected_internal_nodes"`
	RawSelectedInternalParents             uint64 `json:"raw_selected_internal_parent_occurrences"`
	RawSelectedInternalLeaves              uint64 `json:"raw_selected_internal_leaf_occurrences"`
	Overflow                               bool   `json:"overflow"`
}

// DiagnosticWorkCountValues is one additive counter vector. The aggregate
// vector on DiagnosticWorkCount must equal the sum of Attempts plus
// OutsideAttempt for every field.
type DiagnosticWorkCountValues struct {
	Shifts                 uint64 `json:"shifts"`
	Reductions             uint64 `json:"reductions"`
	AcceptActions          uint64 `json:"accept_actions"`
	ExplicitRecoverActions uint64 `json:"explicit_recover_actions"`
	ReductionPopRequests   uint64 `json:"reduction_pop_requests"`
	EmittedPopPaths        uint64 `json:"emitted_pop_paths"`
	EmittedPopPayloads     uint64 `json:"emitted_pop_payloads"`
	SelectedNodes          uint64 `json:"selected_nodes"`
	SelectedParentNodes    uint64 `json:"selected_parent_nodes"`
	SelectedLeafNodes      uint64 `json:"selected_leaf_nodes"`

	TableLookupsProxy               uint64 `json:"table_lookups_proxy"`
	ActionEntriesExaminedProxy      uint64 `json:"action_entries_examined_proxy"`
	LexerFrontDoorCallsProxy        uint64 `json:"lexer_front_door_calls_proxy"`
	StackVersionCreationsProxy      uint64 `json:"stack_version_creations_proxy"`
	MergeAttemptsProxy              uint64 `json:"merge_attempts_proxy"`
	MergeSuccessesProxy             uint64 `json:"merge_successes_proxy"`
	GraphLinkAdditionsProxy         uint64 `json:"graph_link_additions_proxy"`
	LeafConstructionsProxy          uint64 `json:"leaf_constructions_proxy"`
	ParentConstructionsProxy        uint64 `json:"parent_constructions_proxy"`
	PendingParentConstructionsProxy uint64 `json:"pending_parent_constructions_proxy"`
	NoTreeParentConstructionsProxy  uint64 `json:"no_tree_parent_constructions_proxy"`
}

// DiagnosticWorkCountAttempt attributes one parseInternal invocation to the
// retry ladder that scheduled it. LogicalRung describes scheduler intent;
// ResolvedRetryPass records the independent cap-derived parser mode.
type DiagnosticWorkCountAttempt struct {
	Index          uint32 `json:"index"`
	LogicalRung    string `json:"logical_rung"`
	OperationCause string `json:"operation_cause"`

	RequestedMaxStacks       int  `json:"requested_max_stacks"`
	RequestedMaxNodes        int  `json:"requested_max_nodes"`
	RequestedMaxMergePerKey  int  `json:"requested_max_merge_per_key"`
	CapsResolved             bool `json:"caps_resolved"`
	ResolvedMaxStacks        int  `json:"resolved_max_stacks"`
	ResolvedRetryPass        bool `json:"resolved_retry_pass"`
	ResolvedMaxMergePerKey   int  `json:"resolved_max_merge_per_key"`
	ResolvedStackCullTrigger int  `json:"resolved_stack_cull_trigger"`
	ResolvedMaxIterations    int  `json:"resolved_max_iterations"`
	ResolvedMaxNodes         int  `json:"resolved_max_nodes"`

	StopReason   ParseStopReason `json:"stop_reason"`
	RootHasError bool            `json:"root_has_error"`
	RootEndByte  uint32          `json:"root_end_byte"`

	EntryToCaps    DiagnosticWorkCountValues `json:"entry_to_caps"`
	CapsToFinalize DiagnosticWorkCountValues `json:"caps_to_finalize"`
	Finalize       DiagnosticWorkCountValues `json:"finalize"`
	Counters       DiagnosticWorkCountValues `json:"counters"`

	entrySnapshot    DiagnosticWorkCountValues
	capsSnapshot     DiagnosticWorkCountValues
	finalizeSnapshot DiagnosticWorkCountValues
	finalized        bool
}

const DiagnosticWorkCountContract = "gts-work-count/v2"

const workCountInstrumentationEnabled = true

var activeDiagnosticWorkCount *DiagnosticWorkCount

var activeDiagnosticRetryTrace *DiagnosticWorkCount

var pendingDiagnosticWorkCountAttempt struct {
	logicalRung    string
	operationCause string
}

// BeginDiagnosticWorkCount starts one diagnostic parse. The work-count child
// protocol runs one parse with GOMAXPROCS=1, so a process-local pointer avoids
// adding instrumentation state to every production Parser or stack record.
func BeginDiagnosticWorkCount() {
	if activeDiagnosticWorkCount != nil || activeDiagnosticRetryTrace != nil {
		panic("gotreesitter: diagnostic work-count parse already active")
	}
	workCountNestedStart()
	activeDiagnosticWorkCount = &DiagnosticWorkCount{Contract: DiagnosticWorkCountContract, boardDirect: DiagnosticWorkCountBoardDirect{
		Schema: "gts-work-count-board-direct/v3", FrontierLexerElectionsAvailable: true,
	}}
	workCountBeginConvergence(activeDiagnosticWorkCount)
	pendingDiagnosticWorkCountAttempt = struct {
		logicalRung    string
		operationCause string
	}{}
}

// BeginDiagnosticRetryTrace starts an attempt-only trace. It does not enable
// work counters, convergence graph walks, or convergence event allocation.
func BeginDiagnosticRetryTrace() {
	if activeDiagnosticWorkCount != nil || activeDiagnosticRetryTrace != nil {
		panic("gotreesitter: diagnostic parse trace already active")
	}
	activeDiagnosticRetryTrace = &DiagnosticWorkCount{Contract: DiagnosticWorkCountContract}
	pendingDiagnosticWorkCountAttempt = struct {
		logicalRung    string
		operationCause string
	}{}
}

// BoardDirect returns the paired board-counter extension. The base v2
// counter contract remains byte-for-byte unchanged.
func (c DiagnosticWorkCount) BoardDirect() DiagnosticWorkCountBoardDirect {
	return c.boardDirect
}

// EndDiagnosticWorkCount returns the current parse-local counters and disables
// further recording. It is available only in gts_workcount builds.
func EndDiagnosticWorkCount() DiagnosticWorkCount {
	if activeDiagnosticWorkCount == nil {
		panic("gotreesitter: diagnostic work-count parse is not active")
	}
	for i := range activeDiagnosticWorkCount.Attempts {
		if !activeDiagnosticWorkCount.Attempts[i].finalized {
			panic("gotreesitter: diagnostic work-count parse attempt did not finalize")
		}
	}
	workCountNestedFinishSession(activeDiagnosticWorkCount)
	activeDiagnosticWorkCount.reconcileOutsideAttempt()
	out := *activeDiagnosticWorkCount
	workCountEndConvergence()
	activeDiagnosticWorkCount = nil
	return out
}

// EndDiagnosticRetryTrace returns the current attempt-only trace and disables
// further recording.
func EndDiagnosticRetryTrace() DiagnosticRetryTrace {
	if activeDiagnosticRetryTrace == nil {
		panic("gotreesitter: diagnostic retry trace is not active")
	}
	for i := range activeDiagnosticRetryTrace.Attempts {
		if !activeDiagnosticRetryTrace.Attempts[i].finalized {
			panic("gotreesitter: diagnostic retry attempt did not finalize")
		}
	}
	out := DiagnosticRetryTrace{
		Attempts: append([]DiagnosticWorkCountAttempt(nil), activeDiagnosticRetryTrace.Attempts...),
	}
	activeDiagnosticRetryTrace = nil
	return out
}

func diagnosticAttemptLedger() *DiagnosticWorkCount {
	if activeDiagnosticWorkCount != nil {
		return activeDiagnosticWorkCount
	}
	return activeDiagnosticRetryTrace
}

func workCountAttemptTraceActive() bool {
	return diagnosticAttemptLedger() != nil
}

func workCountValuesSnapshot(c *DiagnosticWorkCount) DiagnosticWorkCountValues {
	if c == nil {
		return DiagnosticWorkCountValues{}
	}
	return c.DiagnosticWorkCountValues
}

func workCountValuesDelta(c *DiagnosticWorkCount, after, before DiagnosticWorkCountValues) DiagnosticWorkCountValues {
	var out DiagnosticWorkCountValues
	workCountForEachValue(&out, func(dst *uint64, afterValue, beforeValue uint64) {
		if afterValue < beforeValue {
			if c != nil {
				c.Overflow = true
			}
			*dst = 0
			return
		}
		*dst = afterValue - beforeValue
	}, after, before)
	return out
}

func workCountValuesAdd(c *DiagnosticWorkCount, dst *DiagnosticWorkCountValues, src DiagnosticWorkCountValues) {
	workCountForEachValue(dst, func(slot *uint64, value, _ uint64) {
		diagnosticWorkCountSaturatingAdd(c, slot, value)
	}, src, DiagnosticWorkCountValues{})
}

func workCountForEachValue(dst *DiagnosticWorkCountValues, fn func(*uint64, uint64, uint64), a, b DiagnosticWorkCountValues) {
	fn(&dst.Shifts, a.Shifts, b.Shifts)
	fn(&dst.Reductions, a.Reductions, b.Reductions)
	fn(&dst.AcceptActions, a.AcceptActions, b.AcceptActions)
	fn(&dst.ExplicitRecoverActions, a.ExplicitRecoverActions, b.ExplicitRecoverActions)
	fn(&dst.ReductionPopRequests, a.ReductionPopRequests, b.ReductionPopRequests)
	fn(&dst.EmittedPopPaths, a.EmittedPopPaths, b.EmittedPopPaths)
	fn(&dst.EmittedPopPayloads, a.EmittedPopPayloads, b.EmittedPopPayloads)
	fn(&dst.SelectedNodes, a.SelectedNodes, b.SelectedNodes)
	fn(&dst.SelectedParentNodes, a.SelectedParentNodes, b.SelectedParentNodes)
	fn(&dst.SelectedLeafNodes, a.SelectedLeafNodes, b.SelectedLeafNodes)
	fn(&dst.TableLookupsProxy, a.TableLookupsProxy, b.TableLookupsProxy)
	fn(&dst.ActionEntriesExaminedProxy, a.ActionEntriesExaminedProxy, b.ActionEntriesExaminedProxy)
	fn(&dst.LexerFrontDoorCallsProxy, a.LexerFrontDoorCallsProxy, b.LexerFrontDoorCallsProxy)
	fn(&dst.StackVersionCreationsProxy, a.StackVersionCreationsProxy, b.StackVersionCreationsProxy)
	fn(&dst.MergeAttemptsProxy, a.MergeAttemptsProxy, b.MergeAttemptsProxy)
	fn(&dst.MergeSuccessesProxy, a.MergeSuccessesProxy, b.MergeSuccessesProxy)
	fn(&dst.GraphLinkAdditionsProxy, a.GraphLinkAdditionsProxy, b.GraphLinkAdditionsProxy)
	fn(&dst.LeafConstructionsProxy, a.LeafConstructionsProxy, b.LeafConstructionsProxy)
	fn(&dst.ParentConstructionsProxy, a.ParentConstructionsProxy, b.ParentConstructionsProxy)
	fn(&dst.PendingParentConstructionsProxy, a.PendingParentConstructionsProxy, b.PendingParentConstructionsProxy)
	fn(&dst.NoTreeParentConstructionsProxy, a.NoTreeParentConstructionsProxy, b.NoTreeParentConstructionsProxy)
}

func (c *DiagnosticWorkCount) reconcileOutsideAttempt() {
	if c == nil {
		return
	}
	var attributed DiagnosticWorkCountValues
	for i := range c.Attempts {
		workCountValuesAdd(c, &attributed, c.Attempts[i].Counters)
	}
	c.OutsideAttempt = workCountValuesDelta(c, c.DiagnosticWorkCountValues, attributed)
}

func workCountSetNextParseAttempt(logicalRung, operationCause string) {
	if diagnosticAttemptLedger() == nil {
		return
	}
	pendingDiagnosticWorkCountAttempt.logicalRung = logicalRung
	pendingDiagnosticWorkCountAttempt.operationCause = operationCause
}

type workCountAttemptToken uint32

func workCountBeginParseAttempt(maxStacks, maxNodes, maxMergePerKey int) workCountAttemptToken {
	workCountTopologyBeginAttempt()
	c := diagnosticAttemptLedger()
	if c == nil {
		return 0
	}
	rung := pendingDiagnosticWorkCountAttempt.logicalRung
	cause := pendingDiagnosticWorkCountAttempt.operationCause
	pendingDiagnosticWorkCountAttempt.logicalRung = ""
	pendingDiagnosticWorkCountAttempt.operationCause = ""
	if rung == "" {
		rung = "unspecified"
	}
	if cause == "" {
		cause = "direct_parse_internal"
	}
	index := uint32(len(c.Attempts) + 1)
	c.Attempts = append(c.Attempts, DiagnosticWorkCountAttempt{
		Index:                   index,
		LogicalRung:             rung,
		OperationCause:          cause,
		RequestedMaxStacks:      maxStacks,
		RequestedMaxNodes:       maxNodes,
		RequestedMaxMergePerKey: maxMergePerKey,
		entrySnapshot:           workCountValuesSnapshot(c),
	})
	if activeDiagnosticWorkCount != nil {
		workCountNestedBegin(activeDiagnosticWorkCount, index)
		workCountResetConvergenceAttempt(index)
	}
	return workCountAttemptToken(index)
}

func workCountAttempt(c *DiagnosticWorkCount, token workCountAttemptToken) *DiagnosticWorkCountAttempt {
	if c == nil || token == 0 || int(token) > len(c.Attempts) {
		return nil
	}
	return &c.Attempts[int(token)-1]
}

func workCountResolveParseAttempt(token workCountAttemptToken, maxStacks int, retryPass bool, maxMergePerKey, stackCullTrigger, maxIterations, maxNodes int) {
	c := diagnosticAttemptLedger()
	attempt := workCountAttempt(c, token)
	if attempt == nil {
		return
	}
	attempt.CapsResolved = true
	attempt.ResolvedMaxStacks = maxStacks
	attempt.ResolvedRetryPass = retryPass
	attempt.ResolvedMaxMergePerKey = maxMergePerKey
	attempt.ResolvedStackCullTrigger = stackCullTrigger
	attempt.ResolvedMaxIterations = maxIterations
	attempt.ResolvedMaxNodes = maxNodes
	attempt.capsSnapshot = workCountValuesSnapshot(c)
	attempt.EntryToCaps = workCountValuesDelta(c, attempt.capsSnapshot, attempt.entrySnapshot)
	if activeDiagnosticWorkCount != nil {
		workCountNestedSetPhase(c, uint32(token), 1)
	}
}

func workCountBeginFinalizeParseAttempt(token workCountAttemptToken) {
	c := diagnosticAttemptLedger()
	attempt := workCountAttempt(c, token)
	if attempt == nil {
		return
	}
	if !attempt.CapsResolved {
		attempt.capsSnapshot = attempt.entrySnapshot
	}
	attempt.finalizeSnapshot = workCountValuesSnapshot(c)
	attempt.CapsToFinalize = workCountValuesDelta(c, attempt.finalizeSnapshot, attempt.capsSnapshot)
	if activeDiagnosticWorkCount != nil {
		workCountNestedSetPhase(c, uint32(token), 2)
	}
}

func workCountEndFinalizeParseAttempt(token workCountAttemptToken, stopReason ParseStopReason, tree *Tree) {
	c := diagnosticAttemptLedger()
	attempt := workCountAttempt(c, token)
	if attempt == nil {
		return
	}
	after := workCountValuesSnapshot(c)
	attempt.Finalize = workCountValuesDelta(c, after, attempt.finalizeSnapshot)
	attempt.Counters = workCountValuesDelta(c, after, attempt.entrySnapshot)
	attempt.StopReason = stopReason
	if root := rawRootOrNil(tree); root != nil {
		attempt.RootHasError = root.hasError()
		attempt.RootEndByte = root.endByte
	}
	attempt.finalized = true
	if activeDiagnosticWorkCount != nil {
		workCountNestedEnd(c, uint32(token), attempt)
	}
}

func diagnosticWorkCountSaturatingAdd(counts *DiagnosticWorkCount, slot *uint64, delta uint64) {
	if counts == nil || slot == nil || delta == 0 {
		return
	}
	if math.MaxUint64-*slot < delta {
		*slot = math.MaxUint64
		counts.Overflow = true
		return
	}
	*slot += delta
}

func workCountSaturatingAdd(slot *uint64, delta uint64) {
	diagnosticWorkCountSaturatingAdd(activeDiagnosticWorkCount, slot, delta)
}

// AddDiagnosticSelectedNode records one node from the returned selected tree.
// It is available only in tagged diagnostic builds and uses the same saturating
// arithmetic and overflow bit as the parser-event counters.
func (c *DiagnosticWorkCount) AddDiagnosticSelectedNode(parent bool) {
	diagnosticWorkCountSaturatingAdd(c, &c.SelectedNodes, 1)
	diagnosticWorkCountSaturatingAdd(c, &c.OutsideAttempt.SelectedNodes, 1)
	if parent {
		diagnosticWorkCountSaturatingAdd(c, &c.SelectedParentNodes, 1)
		diagnosticWorkCountSaturatingAdd(c, &c.OutsideAttempt.SelectedParentNodes, 1)
		return
	}
	diagnosticWorkCountSaturatingAdd(c, &c.SelectedLeafNodes, 1)
	diagnosticWorkCountSaturatingAdd(c, &c.OutsideAttempt.SelectedLeafNodes, 1)
}

func workCountRecordLexerFrontDoor() {
	if c := activeDiagnosticWorkCount; c != nil {
		workCountSaturatingAdd(&c.LexerFrontDoorCallsProxy, 1)
	}
}

// workCountRecordFrontierLexerElection records one production parser request
// for a fresh lookahead after its current runnable frontier is established.
// Reusing or re-dispatching an already-held lookahead is not an election.
func workCountRecordFrontierLexerElection() {
	if c := activeDiagnosticWorkCount; c != nil {
		diagnosticWorkCountSaturatingAdd(c, &c.boardDirect.FrontierLexerElections, 1)
		c.boardDirect.Overflow = c.Overflow
	}
}

// workCountRecordRawMainLexerInvocation records one raw scan of the grammar's
// main DFA with one lex state. External-scanner and keyword-only calls are
// deliberately outside this counter.
func workCountRecordRawMainLexerInvocation() {
	if c := activeDiagnosticWorkCount; c != nil {
		diagnosticWorkCountSaturatingAdd(c, &c.boardDirect.RawMainLexerInvocations, 1)
		c.boardDirect.Overflow = c.Overflow
	}
}

func workCountRecordTableLookup() {
	if c := activeDiagnosticWorkCount; c != nil {
		workCountSaturatingAdd(&c.TableLookupsProxy, 1)
	}
}

func workCountRecordResolvedActionCell(actionCount int) {
	if c := activeDiagnosticWorkCount; c != nil {
		diagnosticWorkCountSaturatingAdd(c, &c.boardDirect.ResolvedActionCellsExamined, 1)
		if actionCount > 1 {
			diagnosticWorkCountSaturatingAdd(c, &c.boardDirect.RawActionEntriesBeyondFirst, uint64(actionCount-1))
		}
		c.boardDirect.Overflow = c.Overflow
	}
}

func workCountRecordAlternatePredecessorLinkAppended() {
	if c := activeDiagnosticWorkCount; c != nil {
		diagnosticWorkCountSaturatingAdd(c, &c.boardDirect.AlternatePredecessorLinksAppended, 1)
		c.boardDirect.Overflow = c.Overflow
	}
}

func workCountAddActionEntries(n int) {
	if c := activeDiagnosticWorkCount; c != nil && n > 0 {
		workCountSaturatingAdd(&c.ActionEntriesExaminedProxy, uint64(n))
	}
}

func workCountRecordShift() {
	if c := activeDiagnosticWorkCount; c != nil {
		workCountSaturatingAdd(&c.Shifts, 1)
	}
}

func workCountRecordReduce() {
	if c := activeDiagnosticWorkCount; c != nil {
		workCountSaturatingAdd(&c.Reductions, 1)
		workCountSaturatingAdd(&c.ReductionPopRequests, 1)
	}
}

func workCountRecordAccept() {
	if c := activeDiagnosticWorkCount; c != nil {
		workCountSaturatingAdd(&c.AcceptActions, 1)
	}
}

func workCountRecordExplicitRecover() {
	if c := activeDiagnosticWorkCount; c != nil {
		workCountSaturatingAdd(&c.ExplicitRecoverActions, 1)
	}
}

// workCountObserveReductionPop records direct pop-path output for stack forms
// that have exactly one possible path. Non-linear GSS paths are recorded at
// the point selectedReduceWindowsFromGSSWithBudget emits each concrete path.
func workCountObserveReductionPop(s *glrStack, childCount int) {
	if activeDiagnosticWorkCount == nil || s == nil || childCount < 0 {
		if activeDiagnosticTopology == nil || s == nil || childCount < 0 {
			return
		}
	}
	workCountTopologyRecordDirectPop(s, childCount)
	if activeDiagnosticWorkCount == nil {
		return
	}
	if childCount == 0 {
		workCountAddPopPaths(1, 0)
		return
	}
	if s.entries != nil {
		payloads, remaining := 0, childCount
		for i := len(s.entries) - 1; i >= 0; i-- {
			entry := s.entries[i]
			if !stackEntryHasNode(entry) {
				continue
			}
			payloads++
			if !stackEntryNodeIsExtra(entry) {
				remaining--
				if remaining == 0 {
					workCountAddPopPaths(1, uint64(payloads))
					return
				}
			}
		}
		return
	}
	if s.gss.head == nil || !gssSpanIsLinear(s.gss.head, childCount) {
		return
	}
	payloads, remaining := 0, childCount
	for n := s.gss.head; n != nil; n = n.prev {
		entry := n.entry
		if !stackEntryHasNode(entry) {
			continue
		}
		payloads++
		if !stackEntryNodeIsExtra(entry) {
			remaining--
			if remaining == 0 {
				workCountAddPopPaths(1, uint64(payloads))
				return
			}
		}
	}
}

func workCountAddPopPaths(paths, payloads uint64) {
	if c := activeDiagnosticWorkCount; c != nil {
		workCountSaturatingAdd(&c.EmittedPopPaths, paths)
		workCountSaturatingAdd(&c.EmittedPopPayloads, payloads)
	}
}

func workCountObservePopWindow(window []stackEntry) {
	payloads := uint64(0)
	for _, entry := range window {
		if stackEntryHasNode(entry) {
			payloads++
		}
	}
	workCountAddPopPaths(1, payloads)
}

func workCountRecordVersionCreation() {
	if c := activeDiagnosticWorkCount; c != nil {
		workCountSaturatingAdd(&c.StackVersionCreationsProxy, 1)
	}
}

func workCountRecordMergeAttempt() {
	if c := activeDiagnosticWorkCount; c != nil {
		workCountSaturatingAdd(&c.MergeAttemptsProxy, 1)
	}
}

func workCountRecordMergeSuccess() {
	if c := activeDiagnosticWorkCount; c != nil {
		workCountSaturatingAdd(&c.MergeSuccessesProxy, 1)
	}
}

func workCountRecordGraphLinkAddition() {
	if c := activeDiagnosticWorkCount; c != nil {
		workCountSaturatingAdd(&c.GraphLinkAdditionsProxy, 1)
	}
}

func workCountRecordLeafConstruction() {
	if c := activeDiagnosticWorkCount; c != nil {
		workCountSaturatingAdd(&c.LeafConstructionsProxy, 1)
	}
}

func workCountRecordParentConstruction() {
	if c := activeDiagnosticWorkCount; c != nil {
		workCountSaturatingAdd(&c.ParentConstructionsProxy, 1)
	}
}

func workCountRecordPendingParentConstruction() {
	if c := activeDiagnosticWorkCount; c != nil {
		workCountSaturatingAdd(&c.PendingParentConstructionsProxy, 1)
		workCountSaturatingAdd(&c.ParentConstructionsProxy, 1)
	}
}

func workCountRecordNoTreeParentConstruction() {
	if c := activeDiagnosticWorkCount; c != nil {
		workCountSaturatingAdd(&c.NoTreeParentConstructionsProxy, 1)
		workCountSaturatingAdd(&c.ParentConstructionsProxy, 1)
	}
}

// Nested attempts share aggregate counters. Track only open attempts so each
// direct child is subtracted once from its parent’s active phase. Unsupported
// depth, lifetime, or arithmetic sets the existing overflow flag.
const workCountNestedDepthLimit = 512

type workCountNestedAttempt struct {
	id          uint32
	parent      uint32
	parentPhase uint8
	phase       uint8
	child       [3]DiagnosticWorkCountValues
}

var workCountNestedAttempts [workCountNestedDepthLimit]workCountNestedAttempt
var workCountNestedDepth int
var workCountNestedCount uint32
var workCountNestedInvalid bool

func workCountNestedStart() {
	workCountNestedDepth = 0
	workCountNestedCount = 0
	workCountNestedInvalid = false
}
func workCountNestedFail(c *DiagnosticWorkCount) {
	workCountNestedInvalid = true
	if c != nil {
		c.Overflow = true
	}
}
func workCountNestedBegin(c *DiagnosticWorkCount, id uint32) {
	if workCountNestedInvalid {
		if c != nil {
			c.Overflow = true
		}
		return
	}
	if id != workCountNestedCount+1 || workCountNestedDepth == workCountNestedDepthLimit {
		workCountNestedFail(c)
		return
	}
	var parent uint32
	var phase uint8
	if workCountNestedDepth > 0 {
		frame := &workCountNestedAttempts[workCountNestedDepth-1]
		parent = frame.id
		phase = frame.phase
	}
	workCountNestedAttempts[workCountNestedDepth] = workCountNestedAttempt{id: id, parent: parent, parentPhase: phase}
	workCountNestedCount = id
	workCountNestedDepth++
}
func workCountNestedSetPhase(c *DiagnosticWorkCount, id uint32, phase uint8) {
	if workCountNestedInvalid {
		if c != nil {
			c.Overflow = true
		}
		return
	}
	if workCountNestedDepth == 0 || id == 0 || phase > 2 {
		workCountNestedFail(c)
		return
	}
	frame := &workCountNestedAttempts[workCountNestedDepth-1]
	if frame.id != id || (phase != frame.phase+1 && !(phase == 2 && frame.phase == 0)) {
		workCountNestedFail(c)
		return
	}
	if phase == 2 && frame.phase == 0 { // No caps: the old public entry phase is empty.
		merged, ok := workCountNestedVectorAdd(frame.child[0], frame.child[1])
		if !ok {
			workCountNestedFail(c)
			return
		}
		frame.child[0] = DiagnosticWorkCountValues{}
		frame.child[1] = merged
	}
	frame.phase = phase
}
func workCountNestedVectorAdd(a, b DiagnosticWorkCountValues) (DiagnosticWorkCountValues, bool) {
	var out DiagnosticWorkCountValues
	valid := true
	workCountForEachValue(&out, func(dst *uint64, x, y uint64) {
		if ^uint64(0)-x < y {
			valid = false
			return
		}
		*dst = x + y
	}, a, b)
	return out, valid
}
func workCountNestedVectorSub(a, b DiagnosticWorkCountValues) (DiagnosticWorkCountValues, bool) {
	var out DiagnosticWorkCountValues
	valid := true
	workCountForEachValue(&out, func(dst *uint64, x, y uint64) {
		if x < y {
			valid = false
			return
		}
		*dst = x - y
	}, a, b)
	return out, valid
}
func workCountNestedEnd(c *DiagnosticWorkCount, id uint32, attempt *DiagnosticWorkCountAttempt) {
	if workCountNestedInvalid {
		if c != nil {
			c.Overflow = true
		}
		return
	}
	if id == 0 || id > workCountNestedCount || workCountNestedDepth == 0 || workCountNestedAttempts[workCountNestedDepth-1].id != id || attempt == nil {
		workCountNestedFail(c)
		return
	}
	rec := &workCountNestedAttempts[workCountNestedDepth-1]
	// Record a child's full inclusive interval exactly once in its parent's phase.
	inclusive := attempt.Counters
	totalChildren, ok := workCountNestedVectorAdd(rec.child[0], rec.child[1])
	if !ok {
		workCountNestedFail(c)
		return
	}
	totalChildren, ok = workCountNestedVectorAdd(totalChildren, rec.child[2])
	if !ok {
		workCountNestedFail(c)
		return
	}
	exclusive, ok := workCountNestedVectorSub(inclusive, totalChildren)
	if !ok {
		workCountNestedFail(c)
		return
	}
	entry, ok := workCountNestedVectorSub(attempt.EntryToCaps, rec.child[0])
	if !ok {
		workCountNestedFail(c)
		return
	}
	loop, ok := workCountNestedVectorSub(attempt.CapsToFinalize, rec.child[1])
	if !ok {
		workCountNestedFail(c)
		return
	}
	final, ok := workCountNestedVectorSub(attempt.Finalize, rec.child[2])
	if !ok {
		workCountNestedFail(c)
		return
	}
	phaseSum, ok := workCountNestedVectorAdd(entry, loop)
	if !ok {
		workCountNestedFail(c)
		return
	}
	phaseSum, ok = workCountNestedVectorAdd(phaseSum, final)
	if !ok || phaseSum != exclusive {
		workCountNestedFail(c)
		return
	}
	if rec.parent != 0 {
		if workCountNestedDepth < 2 || workCountNestedAttempts[workCountNestedDepth-2].id != rec.parent || rec.parentPhase > 2 {
			workCountNestedFail(c)
			return
		}
		parent := &workCountNestedAttempts[workCountNestedDepth-2]
		next, valid := workCountNestedVectorAdd(parent.child[rec.parentPhase], inclusive)
		if !valid {
			workCountNestedFail(c)
			return
		}
		parent.child[rec.parentPhase] = next
	}
	attempt.EntryToCaps = entry
	attempt.CapsToFinalize = loop
	attempt.Finalize = final
	attempt.Counters = exclusive
	workCountNestedDepth--
}
func workCountNestedFinishSession(c *DiagnosticWorkCount) {
	if workCountNestedInvalid || workCountNestedDepth != 0 || c == nil || int(workCountNestedCount) != len(c.Attempts) {
		workCountNestedFail(c)
	}
}
