package gotreesitter

import (
	"fmt"
	"sort"
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
// fallback movements in isolation.
func ResetAdmissionCandidateCountersForTest() {
	resetAdmissionCandidateCounters()
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

// AdmissionCandidateCompactStorageBytesForTest exposes the cached
// admission-candidate runner's current compact-core StorageBytes(): live
// record length only, always 0 immediately after any Reset regardless of
// retained capacity. It returns 0 when no runner is cached yet (including
// under -tags gts_no_parsercorephase0, where the engine never runs). Use
// AdmissionCandidateCompactFootprintBytesForTest to check the tranche B9
// storage-RELEASE gate specifically; this one alone cannot.
func AdmissionCandidateCompactStorageBytesForTest(p *Parser) uint64 {
	return admissionCandidateCompactStorageBytes(p)
}

// AdmissionCandidateCompactFootprintBytesForTest exposes the cached
// admission-candidate runner's current compact-core FootprintBytes(): real
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
	return p.beginParseOperationBudget()
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
