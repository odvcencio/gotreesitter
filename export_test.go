package gotreesitter

import (
	"fmt"
	"sort"
	"sync"
	"time"
	"unsafe"

	"github.com/odvcencio/gotreesitter/internal/sched"
)

// This file exposes unexported ParseState-by-table-replay machinery to the
// external gotreesitter_test package so the differential corpus test
// (parsestate_replay_diff_test.go) can compare replay-reconstructed states
// against production-recorded states with full access to node internals.

// ReplayMismatchSample is a single node whose reconstructed state disagreed
// with the production-recorded state.
type ReplayMismatchSample struct {
	Symbol         Symbol
	SymbolName     string
	Class          string
	PreGotoRec     StateID
	PreGotoReplay  StateID
	ParseStateRec  StateID
	ParseStateRep  StateID
	PreGotoDiff    bool
	ParseStateDiff bool
	Depth          int
}

// ReplayClassStat aggregates match/mismatch counts for one node class.
type ReplayClassStat struct {
	Class      string
	Total      int
	Mismatched int
}

// ReplayDiffReport is the result of comparing replay-reconstructed
// (preGotoState, parseState) against production-recorded values over one tree.
type ReplayDiffReport struct {
	TotalNodes          int
	Mismatched          int
	PreGotoMismatched   int
	ParseStateMismatch  int
	RootPreGotoRecorded StateID
	RootPreGotoSeed     StateID
	Classes             map[string]*ReplayClassStat
	Samples             []ReplayMismatchSample
}

// SetTypeScriptCapOneStructurePreferenceForTests selects variant B for
// head-to-head evaluation against the shipping cap-two policy: it keeps the
// TypeScript full-parse cap at one and prefers the structurally-richer fork
// before score at the cap-one discard site. It returns a restore function and
// is never set in production.
func SetTypeScriptCapOneStructurePreferenceForTests(enabled bool) func() {
	prev := typeScriptCapOneStructurePreference.Swap(enabled)
	return func() { typeScriptCapOneStructurePreference.Store(prev) }
}

// SetForceFullResultNormalizationWalk forces the full-tree result-normalization
// walk, disabling the incremental range-limited walk (campaign O(edit),
// spec.campaign.oedit). It exists ONLY so the byte-sweep differential can
// compare the range-limited walk against the full walk on the SAME Parser and
// prove they produce identical trees. It lives in export_test.go, so it is
// compiled only in test builds and is never part of the public API.
func (p *Parser) SetForceFullResultNormalizationWalk(v bool) {
	if p == nil {
		return
	}
	p.forceFullResultNormalizationWalk = v
}

// SetDisableLeadingRunSplice controls the test-only leading-splice
// differential seam. Production has no exported switch and always enables the
// generic path when its safety gates admit it.
func (p *Parser) SetDisableLeadingRunSplice(v bool) {
	if p == nil {
		return
	}
	p.disableLeadingRunSplice = v
}

// SetResultCompatibilityElisionForceDisabledForTest forces every language
// ineligible for the compact fresh path's compat-tail elision (spec.campaign.v7
// tranche A5/C1, result_compat_elision.go), regardless of the registry-computed
// eligibility set. It exists ONLY so the elision-equivalence regression tests
// (admission_compat_tail_elision_test.go) can drive the SAME source through
// the SAME compact route with elision on and off and diff the two resulting
// trees. It returns a restore function and lives in export_test.go, so it is
// compiled only in test builds and is never part of the public API.
func SetResultCompatibilityElisionForceDisabledForTest(disabled bool) func() {
	prev := resultCompatibilityElisionForceDisabledForTest.Swap(disabled)
	return func() { resultCompatibilityElisionForceDisabledForTest.Store(prev) }
}

// ResultCompatibilityElisionEligibleForTest exposes the registry-computed
// compat-tail elision eligibility set (result_compat_elision.go) to the
// external gotreesitter_test package, so a corpus-wide differential test can
// filter to eligible languages without duplicating the registry parse.
func ResultCompatibilityElisionEligibleForTest(lang *Language) bool {
	return resultCompatibilityElisionEligible(lang)
}

// ReplayDiffTree replays the LR tables over the tree rooted at root and
// compares the reconstructed states against the recorded ones on every node.
// It mutates nothing. maxSamples bounds the retained mismatch examples.
func ReplayDiffTree(p *Parser, root *Node, maxSamples int) ReplayDiffReport {
	rep := ReplayDiffReport{
		Classes: map[string]*ReplayClassStat{},
	}
	if p == nil || root == nil {
		return rep
	}
	rep.RootPreGotoRecorded = root.preGotoState
	rep.RootPreGotoSeed = p.replayRootPreGotoState()

	// depth tracking: recompute via the same worklist shape is awkward, so
	// track depth by walking parents lazily only for samples.
	var scratch []replayFrame
	p.replayParseStates(root, rep.RootPreGotoSeed, func(n *Node, preGoto, parseState StateID) {
		rep.TotalNodes++
		class := p.replayNodeClass(n)
		st := rep.Classes[class]
		if st == nil {
			st = &ReplayClassStat{Class: class}
			rep.Classes[class] = st
		}
		st.Total++

		preDiff := preGoto != n.preGotoState
		psDiff := parseState != n.parseState
		if !preDiff && !psDiff {
			return
		}
		rep.Mismatched++
		st.Mismatched++
		if preDiff {
			rep.PreGotoMismatched++
		}
		if psDiff {
			rep.ParseStateMismatch++
		}
		if len(rep.Samples) < maxSamples {
			rep.Samples = append(rep.Samples, ReplayMismatchSample{
				Symbol:         n.symbol,
				SymbolName:     p.symbolNameForReport(n.symbol),
				Class:          class,
				PreGotoRec:     n.preGotoState,
				PreGotoReplay:  preGoto,
				ParseStateRec:  n.parseState,
				ParseStateRep:  parseState,
				PreGotoDiff:    preDiff,
				ParseStateDiff: psDiff,
				Depth:          nodeDepth(n),
			})
		}
	}, &scratch)
	return rep
}

// SortedClassStats returns the per-class stats in a stable, mismatch-first order
// for reporting.
func (r ReplayDiffReport) SortedClassStats() []ReplayClassStat {
	out := make([]ReplayClassStat, 0, len(r.Classes))
	for _, st := range r.Classes {
		out = append(out, *st)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Mismatched != out[j].Mismatched {
			return out[i].Mismatched > out[j].Mismatched
		}
		return out[i].Class < out[j].Class
	})
	return out
}

// replayNodeClass produces a stable multi-dimensional class key for a node,
// used to localize which node classes are/are not replay-derivable.
func (p *Parser) replayNodeClass(n *Node) string {
	kind := "internal"
	if p.replaySymbolIsTerminal(n.symbol) {
		kind = "terminal"
	}
	if len(n.children) == 0 && kind == "internal" {
		kind = "empty-nonterminal"
	}
	var mods string
	if n.symbol == errorSymbol {
		mods += "+error"
	}
	if n.isMissing() {
		mods += "+missing"
	}
	if n.isExtra() {
		mods += "+extra"
	}
	if n.isExternalScannerToken() {
		mods += "+extscan"
	}
	if n.hasError() && n.symbol != errorSymbol {
		mods += "+haserr"
	}
	if mods == "" {
		return kind
	}
	return kind + mods
}

// symbolNameForReport returns a human-readable symbol name for reporting.
func (p *Parser) symbolNameForReport(sym Symbol) string {
	if sym == errorSymbol {
		return "ERROR"
	}
	if p == nil || p.language == nil {
		return fmt.Sprintf("sym#%d", sym)
	}
	if int(sym) < len(p.language.SymbolNames) {
		if name := p.language.SymbolNames[sym]; name != "" {
			return name
		}
	}
	return fmt.Sprintf("sym#%d", sym)
}

func nodeDepth(n *Node) int {
	d := 0
	for n != nil && n.parent != nil {
		d++
		n = n.parent
	}
	return d
}

// ReplayResyncReport isolates the two error sources in visible-tree replay:
// (1) whether advance() reproduces parseState GIVEN the recorded preGotoState
// (tests the goto/shift model in isolation), and (2) whether the first child's
// preGotoState equals its parent's recorded preGotoState (tests the straight
// line intra-production chain). preGoto threading across hidden-node boundaries
// is deliberately NOT threaded here: each node is seeded from its own recorded
// preGotoState.
type ReplayResyncReport struct {
	TotalNodes             int
	ParseStateExact        int // advance(recordedPre, n) == recordedParseState
	ParseStateExactByClass map[string]*ReplayClassStat
	FirstChildPreExact     int // child0.recordedPre == parent.recordedPre
	FirstChildTotal        int
	SiblingChainExact      int // child[i].recordedPre == advance(child[i-1].recordedPre, child[i-1])
	SiblingChainTotal      int
}

// ReplayDiffResync walks the tree and, for every node, checks whether
// advance(recordedPreGoto, node) reproduces the recorded parseState, plus the
// two structural chain invariants. It reveals that parseState IS exactly a
// table transition of the (correct) preGotoState, and that the ONLY thing the
// visible tree cannot reconstruct is preGotoState across hidden-node edges.
func ReplayDiffResync(p *Parser, root *Node) ReplayResyncReport {
	rep := ReplayResyncReport{ParseStateExactByClass: map[string]*ReplayClassStat{}}
	if p == nil || root == nil {
		return rep
	}
	var walk func(n *Node)
	walk = func(n *Node) {
		rep.TotalNodes++
		class := p.replayNodeClass(n)
		st := rep.ParseStateExactByClass[class]
		if st == nil {
			st = &ReplayClassStat{Class: class}
			rep.ParseStateExactByClass[class] = st
		}
		st.Total++
		adv, _ := p.replayAdvance(n.preGotoState, n)
		if adv == n.parseState {
			rep.ParseStateExact++
		} else {
			st.Mismatched++
		}
		if len(n.children) > 0 {
			rep.FirstChildTotal++
			if n.children[0].preGotoState == n.preGotoState {
				rep.FirstChildPreExact++
			}
			for i := 1; i < len(n.children); i++ {
				rep.SiblingChainTotal++
				prevAdv, _ := p.replayAdvance(n.children[i-1].preGotoState, n.children[i-1])
				if n.children[i].preGotoState == prevAdv {
					rep.SiblingChainExact++
				}
			}
		}
		for _, c := range n.children {
			walk(c)
		}
	}
	walk(root)
	return rep
}

func (r ReplayResyncReport) SortedClassStats() []ReplayClassStat {
	out := make([]ReplayClassStat, 0, len(r.ParseStateExactByClass))
	for _, st := range r.ParseStateExactByClass {
		out = append(out, *st)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Mismatched != out[j].Mismatched {
			return out[i].Mismatched > out[j].Mismatched
		}
		return out[i].Class < out[j].Class
	})
	return out
}

// ResetAdmissionCandidateCountersForTest zeroes the Phase-3 admission switch
// counters so the external gotreesitter_test package can assert routing and
// fallback movements in isolation. It delegates to the exported
// ResetAdmissionCandidateCounters, which other packages can also call.
func ResetAdmissionCandidateCountersForTest() {
	ResetAdmissionCandidateCounters()
}

// AdmissionCandidateEnvEnabledForTest exposes the GTS_ADMISSION_CANDIDATE
// parsing that init uses to seed the process-wide default, so the external test
// package can assert the environment contract deterministically.
func AdmissionCandidateEnvEnabledForTest() bool {
	return admissionCandidateEnvEnabled()
}

// ParseRuntimeMemoryMinSourceBytesForTest exposes the source-length floor
// where the production route arms its own automatic runtime memory budget
// (parser_memory_budget_runtime.go), so the external test package can pin
// production-budget tests to the single source of truth instead of copying
// the 64 KiB literal. Tranche B9 retired this floor's former second use as
// the compact-route admission size gate; it no longer bounds candidate-route
// eligibility, only when production's own runtime-heap watchdog arms.
func ParseRuntimeMemoryMinSourceBytesForTest() int {
	return parseRuntimeMemoryMinSourceBytes
}

// TryCompactFullParseRouteForTest exposes the admission-candidate engine seam
// directly (bypassing the public Parser.Parse eligibility checks) so external
// test code can drive the compact route's own accept-or-decline outcome,
// including its decline detail string, without going through the public
// Parse route. Both build configurations define tryCompactFullParseRoute (the
// real engine under the default build, a fail-closed stub under -tags
// gts_no_parsercorephase0), so this resolves in either build.
func TryCompactFullParseRouteForTest(p *Parser, source []byte) (tree *Tree, ok bool, reason string) {
	return p.tryCompactFullParseRoute(source)
}

// AdmissionCandidateCompactStorageBytesForTest exposes the owned runtime,
// or the language's idle runtime, through Core.StorageBytes(): live
// record length only, always 0 immediately after any Reset regardless of
// retained capacity. It returns 0 when no runner is cached yet (including
// under -tags gts_no_parsercorephase0, where the engine never runs). Use
// AdmissionCandidateCompactFootprintBytesForTest to check the tranche B9
// storage-RELEASE gate specifically; this one alone cannot.
func AdmissionCandidateCompactStorageBytesForTest(p *Parser) uint64 {
	return admissionCandidateCompactStorageBytes(p)
}

// AdmissionCandidateCompactFootprintBytesForTest exposes the owned runtime,
// or the language's idle runtime, through Core.FootprintBytes(): real
// retained capacity, not live length. External test code uses this to prove
// the tranche B9 storage-release gate -- a declined parse must not leave
// compact storage retained while the caller's production fallback runs --
// because unlike StorageBytes, this can actually detect a decline path that
// reset logical length but left a large backing array retained. It returns
// 0 when no runner is cached yet (including under -tags
// gts_no_parsercorephase0).
func AdmissionCandidateCompactFootprintBytesForTest(p *Parser) uint64 {
	return admissionCandidateCompactFootprintBytes(p)
}

// BeginParseOperationBudgetForTest opens the same outer parse-budget scope
// Parse itself opens (beginParseOperationBudget), so external test code can
// pin a deadline via SetTimeoutMicros, sleep past it, and then call Parse:
// the nested budget scope Parse opens internally (including inside the
// tranche B8 admission-candidate route) inherits this already-expired
// deadline instead of computing a fresh one, giving a deterministic --
// not call-overhead-dependent -- expired-timeout witness. Mirrors the
// technique TestParseWithSnippetParserInheritsExpiredParentDeadline already
// uses from inside the package.
func (p *Parser) BeginParseOperationBudgetForTest() func() {
	state := p.beginParseOperationBudget()
	return func() {
		p.endParseOperationBudget(state)
	}
}

// AdmissionSubParserProbe bundles internal sub-parser construction so the
// external test package can prove recovery, snippet, and injection sub-parsers
// are born pinned to the production route.
func AcquireSnippetParserForTest(lang *Language) *Parser { return acquireSnippetParser(lang) }

// ReleaseSnippetParserForTest returns a snippet parser to its pool.
func ReleaseSnippetParserForTest(p *Parser) { releaseSnippetParser(p) }

// InjectionChildParserForTest returns a freshly constructed injection child
// parser via the same getParser path the injection engine uses.
func InjectionChildParserForTest(lang *Language) *Parser {
	ip := NewInjectionParser()
	return ip.getParser("test", lang)
}

// ParserPinnedToProductionForTest reports whether p carries the production-only
// override that pinToProductionRoute installs.
func ParserPinnedToProductionForTest(p *Parser) bool {
	return p != nil && p.admissionCandidateRoute == admissionRouteProductionForced
}

// ParserAdmissionEligibleForTest reports whether p would route a fresh DFA full
// parse through the compact candidate right now.
func ParserAdmissionEligibleForTest(p *Parser) bool {
	return p.admissionCandidateFullParseEligible(nil, true)
}

// SchedRequestForTest returns the internal/sched request that a public parse
// method on p builds, with its implied modes resolved.
func (p *Parser) SchedRequestForTest(entry sched.Mode, oldTree *Tree) sched.Request {
	return sched.Request{Modes: p.schedCall(entry, oldTree).All()}
}

// ParserRetainsCollapsedChildOccurrenceForTest exposes the exact native
// occurrence predicate to external-package artifact tests.
func ParserRetainsCollapsedChildOccurrenceForTest(p *Parser, parent, child Symbol) bool {
	return p != nil && p.retainsCollapsedChildOccurrence(parent, child)
}

// SetNoResultCompatibilityBenchmarkOnlyForTest sets p's
// noResultCompatibilityBenchmarkOnly flag directly, without also suppressing
// the admission candidate route the way the public
// ParseNoResultCompatibilityBenchmarkOnly wrapper does (it defers
// suppressAdmissionCandidateRoute, so it can never serve as a compact-route
// probe). Combined with SetAdmissionCandidateRoute(true), this lets the A3
// certification-workstream arm no-op receipt (spec.campaign.v7, finding
// tied-election-family-compact-retirement) dump the compact route's raw,
// pre-compat-tail runner tree for comparison against the normal tailed
// parse. It returns a restore function.
func SetNoResultCompatibilityBenchmarkOnlyForTest(p *Parser, enabled bool) func() {
	prev := p.noResultCompatibilityBenchmarkOnly
	p.noResultCompatibilityBenchmarkOnly = enabled
	return func() { p.noResultCompatibilityBenchmarkOnly = prev }
}

// ParserPoolCheckoutForTest checks a parser out of the pool (applying defaults).
func ParserPoolCheckoutForTest(pp *ParserPool) *Parser { return pp.checkout() }

// ParserPoolReleaseForTest returns a parser to the pool (applying defaults).
func ParserPoolReleaseForTest(pp *ParserPool, p *Parser) { pp.release(p) }

// SetParseMemoryBudgetFloorMBForTest replaces the default budget floor and
// keeps the size scaling on. It returns a function that restores the
// environment-derived value.
func SetParseMemoryBudgetFloorMBForTest(mb int) func() {
	parseMemoryBudgetOnce.Do(func() {})
	prevVal, prevEnv := parseMemoryBudgetMBVal, parseMemoryBudgetMBFromEnv
	parseMemoryBudgetMBVal, parseMemoryBudgetMBFromEnv = mb, false
	return func() {
		parseMemoryBudgetMBVal, parseMemoryBudgetMBFromEnv = prevVal, prevEnv
	}
}

// NormalizeCobolCompatibilityForTest re-applies the cobol result-compatibility
// pass to an already-parsed root. It exists so the external gotreesitter_test
// package can prove normalizeCobolCompatibility is convergent: a real cobol
// witness parsed through the public API reaches its first application via
// Parse, and this wrapper lets the test apply the pass again directly and
// assert the shape and HasError stay stable (spec.campaign.cobol,
// exec_cics_tail_after_clean_prefix period-2 oscillation regression).
func NormalizeCobolCompatibilityForTest(root *Node, source []byte, lang *Language) {
	normalizeCobolCompatibility(root, source, lang)
}

// SetRecordReadFrontierObserverForTest installs fn as
// recordReadFrontierObserverForTest (external_lexer.go, lever 3's lazy read
// frontier), so an external differential test can independently replay every
// position recordReadFrontier is called at -- including calls the lazy skip
// short-circuits -- through its own eager oracle. Pass nil to remove it.
func SetRecordReadFrontierObserverForTest(fn func(pos int)) {
	recordReadFrontierObserverForTest = fn
}

// ExternalLexerFrontierAtForTest exposes lookaheadEndByteAtCursor's pure
// (source, pos) formula without needing a live scan, so a differential test's
// eager oracle can replay it exactly: frontier is pos+1, plus 4 when pos
// lands on an invalid UTF-8 lead byte (ts_lexer_finish's rule).
func ExternalLexerFrontierAtForTest(source []byte, pos int) uint32 {
	l := ExternalLexer{source: source, pos: pos}
	return l.lookaheadEndByteAtCursor()
}

// TokenInvariantExaminedEndForTest exposes tokenInvariantExaminedEnd, the
// frontier-to-examined-end correction that includes UTF-8 continuation bytes
// beyond a frontier that lands mid-rune (#1093).
func TokenInvariantExaminedEndForTest(source []byte, frontier uint32) uint32 {
	return tokenInvariantExaminedEnd(source, frontier)
}

// ExternalReadFrontierValuesForTest is the (lookahead, examined) pair lever
// 3's lazy recordReadFrontier maintains for one scan attempt
// (externalReadFrontier, external_lexer.go).
type ExternalReadFrontierValuesForTest struct {
	Lookahead uint32
	Examined  uint32
}

// RecordReadFrontierForTest drives the real, unmodified
// (*ExternalLexer).recordReadFrontier -- the exact lazy implementation lever
// 3 ships, lazy skip included -- for a lexer positioned at pos over source,
// folding the result into rf in place. This lets a differential test replay
// a captured position sequence through the production lazy path directly,
// one position at a time, with no live scan required.
func RecordReadFrontierForTest(rf *ExternalReadFrontierValuesForTest, source []byte, pos int) {
	observer := &externalReadFrontier{lookahead: rf.Lookahead, examined: rf.Examined}
	l := ExternalLexer{source: source, pos: pos, readFrontier: observer}
	l.recordReadFrontier()
	rf.Lookahead, rf.Examined = observer.lookahead, observer.examined
}

// SnippetRootMemoProbe reports one parseSnippetRootWithOffset round trip.
type SnippetRootMemoProbe struct {
	First, Second, Direct *Node
	// Memoized reports that the first call stored the snippet root, so the
	// second call cloned it instead of parsing again.
	Memoized bool
	// ClearedAfterOperation reports that the memo was empty once the parse
	// operation ended.
	ClearedAfterOperation bool
	release               func()
}

// Release frees the direct reference tree.
func (p SnippetRootMemoProbe) Release() {
	if p.release != nil {
		p.release()
	}
}

// SnippetRootMemoProbeForTest calls parseSnippetRootWithOffset twice inside one
// parse operation of parser. Direct is the snippet root from an independent
// snippet parse, shifted with Tree.rootNodeWithRecoveryOffset (the internal twin of RootNodeWithOffset that keeps the recovery point rule) as callers did before
// the memo existed.
func SnippetRootMemoProbeForTest(parser *Parser, source []byte, start, end uint32) SnippetRootMemoProbe {
	var probe SnippetRootMemoProbe
	operation := parser.beginParseOperationBudget()
	probe.First, _ = parseSnippetRootWithOffset(parser.language, source, start, end, parser)
	if cold := parser.forestDeclineMemo; cold != nil {
		probe.Memoized = cold.snippetRootMemo[string(source[start:end])] != nil
	}
	probe.Second, _ = parseSnippetRootWithOffset(parser.language, source, start, end, parser)
	parser.endParseOperationBudget(operation)
	probe.ClearedAfterOperation = parser.forestDeclineMemo == nil || len(parser.forestDeclineMemo.snippetRootMemo) == 0
	tree, err := parseWithSnippetParser(parser.language, source[start:end])
	if err == nil && tree != nil {
		probe.Direct = tree.rootNodeWithRecoveryOffset(start, advancePointByBytes(Point{}, source[:start]))
		probe.release = tree.Release
	}
	return probe
}

// stackScratchPool is a deterministic scratchPool: Get pops the last object
// that Put pushed and returns nil when the pool is empty. It never drops
// objects, unlike sync.Pool.
type stackScratchPool struct {
	mu    sync.Mutex
	items []any
	gets  int
}

// Gets returns how many times Get was called.
func (p *stackScratchPool) Gets() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.gets
}

func (p *stackScratchPool) Get() any {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.gets++
	if len(p.items) == 0 {
		return nil
	}
	x := p.items[len(p.items)-1]
	p.items = p.items[:len(p.items)-1]
	return x
}

func (p *stackScratchPool) Put(x any) {
	p.mu.Lock()
	p.items = append(p.items, x)
	p.mu.Unlock()
}

// Items returns the pooled objects, oldest first.
func (p *stackScratchPool) Items() []any {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]any(nil), p.items...)
}

func (p *stackScratchPool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.items)
}

// swapScratchPoolsForTest replaces the pools that tiny-source hand-out reads
// with empty deterministic stacks. The returned function restores the previous
// pools. Release and the path for larger sources use the real pools.
func swapScratchPoolsForTest() (shared, large, heavy *stackScratchPool, restore func()) {
	shared, large, heavy = &stackScratchPool{}, &stackScratchPool{}, &stackScratchPool{}
	previous := tinyScratchPools
	tinyScratchPools = &tinyScratchPoolSet{shared: shared, large: large, heavy: heavy}
	return shared, large, heavy, func() { tinyScratchPools = previous }
}

// ScratchPoolProbe gives the external test package access to the swapped
// scratch pools.
type ScratchPoolProbe struct {
	shared *stackScratchPool
}

// SwapScratchPoolsForTest replaces the scratch pools with empty deterministic
// stacks and returns a probe and a function that restores the previous pools.
func SwapScratchPoolsForTest() (*ScratchPoolProbe, func()) {
	shared, _, _, restore := swapScratchPoolsForTest()
	return &ScratchPoolProbe{shared: shared}, restore
}

// PutSharedWithAccountedBytes adds a scratch object that holds more than target
// accounted bytes to the shared pool and returns its accounted bytes.
func (p *ScratchPoolProbe) PutSharedWithAccountedBytes(target int64) int64 {
	s := newScratchWithAccountedBytesForTest(target)
	p.shared.Put(s)
	return s.allocatedBytes()
}

// SharedLen returns the number of objects in the shared pool.
func (p *ScratchPoolProbe) SharedLen() int { return p.shared.Len() }

// KeepOnlyLargestShared drops every shared-pool object except the one with the
// most accounted bytes and returns that object's accounted bytes.
func (p *ScratchPoolProbe) KeepOnlyLargestShared() int64 {
	p.shared.mu.Lock()
	defer p.shared.mu.Unlock()
	var best *parserScratch
	for _, x := range p.shared.items {
		if s := x.(*parserScratch); best == nil || s.allocatedBytes() > best.allocatedBytes() {
			best = s
		}
	}
	p.shared.items = nil
	if best == nil {
		return 0
	}
	p.shared.items = append(p.shared.items, best)
	return best.allocatedBytes()
}

func newScratchWithGSSNodesForTest(nodes int) *parserScratch {
	s := &parserScratch{}
	s.gss.slabs = []gssNodeSlab{{data: make([]gssNode, nodes), reachMarks: make([]uint32, nodes)}}
	s.gss.recomputeAllocatedBytes()
	return s
}

// newScratchWithAccountedBytesForTest returns scratch whose accounted capacity
// is above target, however large a GSS node is on the target architecture.
func newScratchWithAccountedBytesForTest(target int64) *parserScratch {
	nodes := int(target/int64(unsafe.Sizeof(gssNode{})+unsafe.Sizeof(uint32(0)))) + 1
	for {
		s := newScratchWithGSSNodesForTest(nodes)
		if s.allocatedBytes() > target {
			return s
		}
		nodes += nodes/64 + 1
	}
}

// SetRetryPassObserverForTests installs fn on p. It runs after each
// error-recovery retry pass and once after the first pass (rung "first_pass"),
// with the parser that ran the pass. The observer belongs to p, so tests that
// use different parsers do not share it.
func SetRetryPassObserverForTests(p *Parser, fn func(p *Parser, logicalRung string)) {
	p.retryPassObserver = fn
}

// ExpireParseDeadlineForTests moves the active parse deadline of p into the
// past, so the next stop check reports ParseStopTimeout. It does nothing
// outside a parse that has a timeout.
func ExpireParseDeadlineForTests(p *Parser) {
	if p != nil && !p.parseDeadline.IsZero() {
		p.parseDeadline = time.Now().Add(-time.Second)
	}
}

// SetPoolParserInitForTests runs init on every parser that pp creates later.
func SetPoolParserInitForTests(pp *ParserPool, init func(*Parser)) {
	create := pp.pool.New
	pp.pool.New = func() any {
		p := create().(*Parser)
		init(p)
		return p
	}
}

// RetryCutOperationForTests runs fn as one parse operation of p, with an
// open timeout scope, and stamps the operation's signal on the tree that fn
// returns, as the public parse entries do.
func RetryCutOperationForTests(p *Parser, fn func() *Tree) *Tree {
	p.retryCutBegin()
	defer p.enterParseBudget()()
	tree := fn()
	p.retryCutEnd(tree)
	return tree
}

// VerifyIncrementalFreshResultForTests runs the fresh verification of an
// incremental result inside an operation of p.
func VerifyIncrementalFreshResultForTests(p *Parser, source []byte, oldTree, tree *Tree) (*Tree, error) {
	ts := p.acquireParserDFATokenSource(source)
	defer ts.Close()
	return p.verifyIncrementalFreshResult(source, oldTree, p.wrapIncludedRanges(ts), tree, nil)
}

// RetryIncrementalAsFullForTests runs the fresh fallback of an incremental
// result inside an operation of p.
func RetryIncrementalAsFullForTests(p *Parser, source []byte, tree *Tree) *Tree {
	return p.retryIncrementalParseAsFullWithDFA(source, 8, tree, nil)
}

// ResolveSwallowedErrorForTests runs the swallowed-error check on a tree
// whose runtime is marked as the check requires.
func ResolveSwallowedErrorForTests(p *Parser, source []byte, tree *Tree) *Tree {
	rt := tree.ensureParseRuntime()
	rt.CRecoveryEnteredErrorState = true
	rt.CRecoveryDroppedErrorForClean = true
	p.errorCostCompetition = true
	return p.resolveCRecoverySwallowedError(source, tree)
}

// ParseWithExpiredDeadlineForTests parses source inside a timeout scope whose
// deadline is already over, so the first pass stops with ParseStopTimeout.
func ParseWithExpiredDeadlineForTests(p *Parser, source []byte) (*Tree, error) {
	p.parseBudgetDepth = 1
	p.parseDeadline = time.Now().Add(-time.Second)
	defer func() {
		p.parseBudgetDepth = 0
		p.parseDeadline = time.Time{}
		p.parseStoppedReason = ParseStopNone
	}()
	return p.Parse(source)
}

// ForestRescueForTests runs the optional forest rescue of a recovered tree.
func ForestRescueForTests(p *Parser, source []byte, tree *Tree) (*Tree, bool) {
	return p.maybeReplaceRecoveredTreeWithForest(source, tree)
}
