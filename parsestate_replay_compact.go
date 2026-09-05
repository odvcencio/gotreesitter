//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"errors"
	"os"
	"sync"
	"sync/atomic"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// parserCoreReplayParseStatesEnabled reports whether the compact route should
// reconstruct and stamp per-node parser states by table replay. It is gated by
// GTS_REPLAY_PARSESTATE so the states-free compact route can be A/B'd against
// the replay-materialized route. The value is latched once for stable A/B runs.
var (
	parserCoreReplayParseStatesOnce  int32
	parserCoreReplayParseStatesValue int32
)

func parserCoreReplayParseStatesEnabled() bool {
	if atomic.LoadInt32(&parserCoreReplayParseStatesOnce) == 0 {
		v := int32(0)
		switch os.Getenv("GTS_REPLAY_PARSESTATE") {
		case "1", "true", "on", "yes":
			v = 1
		}
		atomic.StoreInt32(&parserCoreReplayParseStatesValue, v)
		atomic.StoreInt32(&parserCoreReplayParseStatesOnce, 1)
	}
	return atomic.LoadInt32(&parserCoreReplayParseStatesValue) == 1
}

// setParserCoreReplayParseStatesForTest overrides the gate for differential
// tests without mutating process environment mid-run.
func setParserCoreReplayParseStatesForTest(on bool) {
	v := int32(0)
	if on {
		v = 1
	}
	atomic.StoreInt32(&parserCoreReplayParseStatesValue, v)
	atomic.StoreInt32(&parserCoreReplayParseStatesOnce, 1)
}

// ParseState-by-table-replay over the compact derivation (Phase-3 Lane 2).
//
// The visible production tree cannot reconstruct parseState because
// materialization erases the two things replay needs: hidden-node goto
// transitions (repeat/supertype aux symbols carry state between visible
// siblings) and the pre-alias grammar symbol (aliased leaves store the alias,
// not the terminal that was shifted). The compact derivation retains BOTH --
// every reduce keeps one record per RHS symbol, including hidden ones, with the
// real grammar symbol and an authoritative terminal flag -- so replay over the
// derivation, before hidden elision and aliasing, is exact.
//
// The pass is a single push-shaped top-down sweep over the derivation DAG,
// threading the LR state chain exactly as the parser did: root seeded from
// InitialState, first child inherits the parent's exposed state, each later
// child advances from the previous sibling's state. Results are stored per
// SubtreeID; the postorder materialization then stamps psByID/preByID onto the
// surviving public nodes (hidden ids are dropped, aliased ids keep the state
// computed from their real symbol).

// compactReplayFrame is one entry on the explicit worklist used by
// replayCompactDerivation to avoid deep Go recursion on pathologically deep
// derivations.
type compactReplayFrame struct {
	cursor    StateID
	children  core.MaterializationReplayChildren
	nextChild uint32
}

// compactReplayStates holds the reconstructed per-derivation-node parser
// states, indexed 1-based by SubtreeID (index 0 unused). The three per-id
// backing slices AND the traversal worklist (frames) are pooled across parses,
// so once the pool is warm the top-down pass adds no steady-state allocation to
// the materialization phase; release() returns them once stamping is done.
type compactReplayStates struct {
	parseState   []StateID
	preGotoState []StateID
	// psKnown[id] is true only when the top-down replay found an authoritative
	// shift/goto transition for id's parseState. preKnown[id] additionally
	// requires that id's preGotoState is authoritative: it is false for extra
	// (comment) nodes, whose tree position floats away from their live-parse
	// lex-time stack state, so their inherited preGoto is not reconstructable
	// even though their parseState is exact. When a flag is false the
	// corresponding state is NOT written (abstained): get() reports it so the
	// materializer leaves the node's state at its zero value
	// ("unknown -> recompute") rather than stamping a known-wrong but trusted
	// non-zero state (Phase-3 Lane 3 review amendment 1).
	psKnown  []bool
	preKnown []bool
	// frames is the pooled traversal worklist backing store, reused across
	// parses so the top-down pass is allocation-free once the pool is warm
	// (Phase-3 Lane 3 review amendment 6).
	frames []compactReplayFrame
}

var compactReplayStatePool = sync.Pool{New: func() any { return &compactReplayStates{} }}

func acquireCompactReplayStates(n int) *compactReplayStates {
	s := compactReplayStatePool.Get().(*compactReplayStates)
	if cap(s.parseState) < n {
		s.parseState = make([]StateID, n)
		s.preGotoState = make([]StateID, n)
		s.psKnown = make([]bool, n)
		s.preKnown = make([]bool, n)
	} else {
		s.parseState = s.parseState[:n]
		s.preGotoState = s.preGotoState[:n]
		s.psKnown = s.psKnown[:n]
		s.preKnown = s.preKnown[:n]
		clear(s.parseState)
		clear(s.preGotoState)
		clear(s.psKnown)
		clear(s.preKnown)
	}
	return s
}

func (s *compactReplayStates) release() {
	if s == nil {
		return
	}
	compactReplayStatePool.Put(s)
}

// get returns the reconstructed states for id and whether each is authoritative.
// preOk / psOk are independent: an extra leaf has an exact parseState (psOk) but
// a non-reconstructable, floated preGotoState (preOk=false).
func (s *compactReplayStates) get(id core.SubtreeID) (pre, ps StateID, preOk, psOk bool) {
	if s == nil || uint64(id) >= uint64(len(s.parseState)) {
		return 0, 0, false, false
	}
	return s.preGotoState[id], s.parseState[id], s.preKnown[id], s.psKnown[id]
}

// replayCompactDerivation reconstructs parser states for every node in the
// accepted compact derivation rooted at roots. roots are the accepted payload
// ids in tree order; each is seeded with the parser's InitialState (the state
// exposed below the final reduce). The traversal uses an explicit worklist over
// SubtreeIDs to avoid deep recursion.
func (p *Parser) replayCompactDerivation(compact *core.Core, roots []core.SubtreeID) (*compactReplayStates, error) {
	if p == nil || p.language == nil || compact == nil {
		return nil, errors.New("parser-core phase zero: replay requires parser and core")
	}
	if !compact.TableIdentityMatches() {
		return nil, &diagnosticParserCoreDecline{
			boundary: DiagnosticParserCoreIdentity,
			detail:   "compact parser table identity does not match the materialization parser",
		}
	}
	n := compact.SubtreeArenaLen() + 1
	states := acquireCompactReplayStates(n)
	seed := p.replayRootPreGotoState()

	stack := states.frames[:0]
	fail := func(err error) (*compactReplayStates, error) {
		clear(stack)
		states.frames = stack[:0]
		states.release()
		return nil, err
	}
	// Process one root at a time. This bounds the worklist by derivation depth
	// instead of the width of every sibling cohort.
	for rootIndex := len(roots) - 1; rootIndex >= 0; rootIndex-- {
		root := roots[rootIndex]
		view, err := compact.MaterializationReplayView(root)
		if err != nil {
			return fail(err)
		}
		rootParseState, rootStateKnown, err := p.replayCompactMaterializationTransition(seed, view)
		if err != nil {
			return fail(err)
		}
		if rootStateKnown && uint64(root) < uint64(len(states.parseState)) {
			states.parseState[root] = rootParseState
			states.psKnown[root] = true
			if !view.Extra {
				states.preGotoState[root] = seed
				states.preKnown[root] = true
			}
		}
		stack = append(stack, compactReplayFrame{
			cursor: seed, children: view.Children,
		})

		for len(stack) > 0 {
			topIndex := len(stack) - 1
			top := &stack[topIndex]
			if top.nextChild >= top.children.Len() {
				// Clear the borrowed child slice before the frame returns to the
				// global pool. This prevents a transient Core from being retained.
				stack[topIndex] = compactReplayFrame{}
				stack = stack[:topIndex]
				continue
			}

			child, ok := compact.MaterializationReplayChild(top.children, top.nextChild)
			if !ok {
				return fail(errors.New("parser-core phase zero: replay child index is invalid"))
			}
			childPreGoto := top.cursor
			top.nextChild++
			cview, err := compact.MaterializationReplayView(child)
			if err != nil {
				return fail(err)
			}
			childParseState, childStateKnown, err := p.replayCompactMaterializationTransition(childPreGoto, cview)
			if err != nil {
				return fail(err)
			}
			top.cursor = childParseState
			if childStateKnown && uint64(child) < uint64(len(states.parseState)) {
				// Record only authoritative transitions. A failed transition
				// returns childPreGoto as a traversal fallback.
				states.parseState[child] = childParseState
				states.psKnown[child] = true
				if !cview.Extra {
					states.preGotoState[child] = childPreGoto
					states.preKnown[child] = true
				}
			}
			stack = append(stack, compactReplayFrame{
				cursor: childPreGoto, children: cview.Children,
			})
		}
	}
	// Retain the cleared worklist backing store for the next parse.
	states.frames = stack[:0]
	return states, nil
}

func (p *Parser) replayCompactMaterializationTransition(pre StateID, view core.MaterializationReplayView) (StateID, bool, error) {
	state, known := p.replayTransition(pre, Symbol(view.Symbol), view.Terminal)
	if view.ReusedKey == 0 {
		return state, known, nil
	}
	if view.Terminal || view.Extra || view.Children.Len() != 0 || !known ||
		pre != StateID(view.ReusedPreGotoState) || state != StateID(view.ReusedState) {
		return 0, false, compactIncrementalMaterializationDecline("borrowed replay transition does not match the parser tables")
	}
	return StateID(view.ReusedState), true, nil
}
