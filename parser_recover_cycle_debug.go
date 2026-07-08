package gotreesitter

// parser_recover_cycle_debug.go — construction-time back-edge (cycle) tracing
// for the error-recovery engine, env-gated via GOT_DEBUG_RECOVERY_CYCLES=1.
//
// The fundamental tree invariant is that no node is reachable from its own
// descendants. Recovery-path construction historically violated it (issue
// #110; the go zerrors_windows.go truncation hang): wiring eager parent links
// into TRANSIENT reduce-parent children corrupted the result-time
// materializer's parent-field sentinel, and transientReplacement() then linked
// a recovery wrapper under itself. The mechanism fix is
// newRecoveryParentNodeInArena (parser_recover_c.go); the checks here remain
// as the debug-mode acyclicity sweep: with the env var set, every recovery
// construction and every condense pass re-verifies the transient child view
// (dense children, arena final child refs, pending parent shells) WITHOUT
// materializing anything, so running them cannot perturb the parse.
//
// Zero overhead when the env var is unset beyond one package-var bool check
// per recovery construction site.

import (
	"fmt"
	"os"
	"unsafe"
)

// The package-level debug state below is single-parse/sequential-use-only.
// GOT_DEBUG_RECOVERY_CYCLES is a debug-mode tracing aid, not a
// concurrency-safe facility: debugRecoveryCheckSitesSeen is a plain map and
// the counters are plain (non-atomic) integers. Running multiple *Parser
// instances concurrently under this flag can race the site map and undercount
// or corrupt these values. No mutex is added here deliberately — this path is
// debug-only and opt-in (zero cost when the flag is unset), and documenting
// the single-parse constraint is sufficient; callers who need concurrent
// debug tracing must serialize their own parses.
var debugRecoveryCycleChecks = os.Getenv("GOT_DEBUG_RECOVERY_CYCLES") == "1"

// debugRecoveryCycleReportsLeft caps the loud per-cycle output;
// debugRecoveryCyclesFound keeps counting past the cap.
var debugRecoveryCycleReportsLeft = 8
var debugRecoveryCyclesFound uint64
var debugRecoveryCheckSitesSeen = map[string]bool{}

// debugRecoveryTraceSite prints one line per static site the first time it is
// checked, so a sweep run proves which recovery paths actually engaged.
// Per-call variability (indices, byte offsets) is trimmed at the first digit.
func debugRecoveryTraceSite(site string) {
	if !debugRecoveryCycleChecks {
		// Direct detector calls (unit tests) stay quiet; only env-gated sweep
		// runs report site coverage.
		return
	}
	key := site
	for j := 0; j < len(key); j++ {
		if key[j] >= '0' && key[j] <= '9' {
			key = key[:j]
			break
		}
	}
	if !debugRecoveryCheckSitesSeen[key] {
		debugRecoveryCheckSitesSeen[key] = true
		fmt.Fprintf(os.Stderr, "RECOVERY-CYCLE-TRACE first check at site=%s\n", key)
	}
}

// debugRecoveryChildEntries appends e's child payload entries (transient view,
// no materialization) to out and returns it. Field-only pending entries have a
// nil payload pointer and are skipped.
func debugRecoveryChildEntries(arena *nodeArena, e stackEntry, out []stackEntry) []stackEntry {
	switch e.kind {
	case stackEntryKindNode:
		n := (*Node)(e.node)
		if n == nil {
			return out
		}
		if n.ownerArena != nil {
			if cr, ok := n.ownerArena.finalChildRange(n); ok {
				for _, ref := range cr.refs(n.ownerArena) {
					se := ref.stackEntry()
					if se.node != nil {
						out = append(out, se)
					}
				}
				return out
			}
		}
		for _, c := range n.children {
			if c != nil {
				out = append(out, newStackEntryNode(c.parseState, c))
			}
		}
	case stackEntryKindPendingParent:
		pp := (*pendingParent)(e.node)
		if pp == nil || arena == nil {
			return out
		}
		for _, ref := range pp.childRange.refs(arena) {
			se := ref.stackEntry()
			if se.node != nil {
				out = append(out, se)
			}
		}
	}
	// noTreeNode / compactFullLeaf payloads are leaves.
	return out
}

func debugRecoveryEntryDesc(p *Parser, e stackEntry) string {
	kind := "?"
	sym := Symbol(0)
	var start, end uint32
	switch e.kind {
	case stackEntryKindNode:
		n := (*Node)(e.node)
		kind = "node"
		if n != nil {
			sym = n.symbol
			start, end = n.startByte, n.endByte
		}
	case stackEntryKindPendingParent:
		pp := (*pendingParent)(e.node)
		kind = "pending"
		if pp != nil {
			sym = pp.symbol
			start, end = pp.startByte, pp.endByte
		}
	case stackEntryKindNoTreeNode:
		kind = "notree"
	case stackEntryKindCompactFullLeaf:
		kind = "leaf"
	}
	name := ""
	if p != nil && p.language != nil && int(sym) < len(p.language.SymbolNames) {
		name = p.language.SymbolNames[sym]
	}
	return fmt.Sprintf("%s@%p sym=%d(%s) [%d-%d]", kind, e.node, sym, name, start, end)
}

// debugRecoveryCheckAcyclic runs an iterative 3-color DFS from root over the
// transient child view. On finding a back-edge it prints the enclosing cycle
// path (up to the report budget) and bumps debugRecoveryCyclesFound. Returns
// true when the reachable graph is acyclic.
func debugRecoveryCheckAcyclic(p *Parser, arena *nodeArena, site string, root stackEntry) bool {
	return debugRecoveryCheckAcyclicShared(p, arena, site, root, make(map[unsafe.Pointer]int, 64))
}

func debugRecoveryCheckAcyclicShared(p *Parser, arena *nodeArena, site string, root stackEntry, color map[unsafe.Pointer]int) bool {
	if root.node == nil {
		return true
	}
	debugRecoveryTraceSite(site)
	const (
		colorGray  = 1
		colorBlack = 2
	)
	if color[root.node] == colorBlack {
		return true
	}
	type frame struct {
		e        stackEntry
		children []stackEntry
		idx      int
	}
	stack := []frame{{e: root, children: debugRecoveryChildEntries(arena, root, nil)}}
	color[root.node] = colorGray
	for len(stack) > 0 {
		f := &stack[len(stack)-1]
		if f.idx >= len(f.children) {
			color[f.e.node] = colorBlack
			stack = stack[:len(stack)-1]
			continue
		}
		child := f.children[f.idx]
		f.idx++
		switch color[child.node] {
		case colorGray:
			// Back-edge: child is an ancestor on the current DFS path.
			debugRecoveryCyclesFound++
			if debugRecoveryCycleReportsLeft > 0 {
				debugRecoveryCycleReportsLeft--
				fmt.Fprintf(os.Stderr, "RECOVERY-CYCLE site=%s back-edge from %s to ancestor %s\n",
					site, debugRecoveryEntryDesc(p, f.e), debugRecoveryEntryDesc(p, child))
				for i := range stack {
					fmt.Fprintf(os.Stderr, "  path[%d] %s\n", i, debugRecoveryEntryDesc(p, stack[i].e))
				}
			}
			return false
		case colorBlack:
			continue
		default:
			color[child.node] = colorGray
			stack = append(stack, frame{e: child, children: debugRecoveryChildEntries(arena, child, nil)})
		}
	}
	return true
}

// debugRecoveryCheckNodeAcyclic is the *Node convenience wrapper.
func debugRecoveryCheckNodeAcyclic(p *Parser, arena *nodeArena, site string, n *Node) bool {
	if n == nil {
		return true
	}
	return debugRecoveryCheckAcyclic(p, arena, site, newStackEntryNode(n.parseState, n))
}

// debugRecoveryCheckSpineAcyclic checks every payload entry of a stack spine
// with one shared color map, so shared subtrees are only walked once.
func debugRecoveryCheckSpineAcyclic(p *Parser, arena *nodeArena, site string, entries []stackEntry) bool {
	color := make(map[unsafe.Pointer]int, 256)
	ok := true
	for ei := range entries {
		if entries[ei].node == nil {
			continue
		}
		if !debugRecoveryCheckAcyclicShared(p, arena, fmt.Sprintf("%s-spine-%d", site, ei), entries[ei], color) {
			ok = false
		}
	}
	return ok
}
