package gotreesitter

import (
	"runtime"
	"runtime/debug"
	"testing"
)

// This file regression-tests the 2026-07-12 gocompat-walk-containment-gap
// fix: walkGoCompatSubtree used to poll only for timeout/cancellation
// (parseStopReasonIsActive explicitly
// excludes ParseStopMemoryBudget), so a C-recovery-widened Go tree's compat
// walk could balloon heap growth during result finalization independent of
// whatever the parse loop itself already enforced. The fix threads
// (*Parser).compatRuntimeMemoryBudgetStopReason (renamed from
// goCompatRuntimeMemoryBudgetStopReason once the JS/TS fused walk started
// sharing it — see parser_result_javascript_typescript_memory_budget_test.go)
// into the walk's poller
// (parseStopPoller.memoryBudgetParser) and into per-stage boundary checks in
// normalizeGoReturnedTreeCompatibilityWithCensus (parser_result_go.go), and latches
// compatMemoryBudgetTripped so a trip detected there is reliably surfaced by
// resultMaterializationStopReason afterward (parser_result.go).
//
// buildGoCompatWideContainerTree below models the trigger shape without an
// end-to-end multi-hundred-MB reproduction: a wide, flat, C-recovery-shaped
// tree (many real, distinct semicolon-container nodes, e.g. what a truncated
// giant const/table-literal input widens into) whose compat-walk cost is
// genuinely proportional to tree size, calibrated so unbounded work is
// clearly (10x+) larger than a small configured budget while staying well
// under ~1-2GiB for test safety.

const (
	testGoCompatIdentSym Symbol = 100
	testGoCompatSemiSym  Symbol = 101
	testGoCompatDeclSym  Symbol = 102
	testGoCompatRootSym  Symbol = 103
)

// buildGoCompatWideContainerTree builds a root with numContainers distinct
// semicolon-container ("const_declaration"-shaped) children, each holding
// containerWidth alternating ident/droppable-semicolon leaves. Every
// container is visited exactly once by the walk (there is no node sharing
// here — unlike the historical DAG/cycle defect, this is an ordinary tree),
// so its per-container work (normalizeGoSemicolonContainer's kept-slice
// filter, sized to the container's real width) is real, non-idempotent,
// unavoidable work: exactly the shape a legitimately huge recovered tree
// produces, just made large enough here to make the containment gap
// observable without needing multi-hundred-MB inputs.
func buildGoCompatWideContainerTree(numContainers, containerWidth int) (*Node, []byte, goCompatibilitySymbols) {
	arena := newNodeArena(arenaClassFull)
	source := make([]byte, numContainers*containerWidth)
	containers := make([]*Node, numContainers)
	for c := 0; c < numContainers; c++ {
		base := c * containerWidth
		children := make([]*Node, containerWidth)
		for i := 0; i < containerWidth; i++ {
			off := uint32(base + i)
			if i%2 == 0 {
				source[base+i] = 'x'
				children[i] = newLeafNodeInArena(arena, testGoCompatIdentSym, true, off, off+1, Point{}, Point{})
			} else {
				source[base+i] = '\n'
				children[i] = newLeafNodeInArena(arena, testGoCompatSemiSym, false, off, off+1, Point{}, Point{})
			}
		}
		containers[c] = newParentNodeInArena(arena, testGoCompatDeclSym, true, cloneNodeSliceInArena(arena, children), nil, 0)
	}
	root := newParentNodeInArena(arena, testGoCompatRootSym, true, cloneNodeSliceInArena(arena, containers), nil, 0)

	syms := goCompatibilitySymbols{semicolon: testGoCompatSemiSym}
	syms.semiContainers[0] = testGoCompatDeclSym
	syms.semiContainerLen = 1
	return root, source, syms
}

// containerStillHasSemicolon reports whether container c of root (as built by
// buildGoCompatWideContainerTree) still carries its droppable semicolon leaf
// — i.e. normalizeGoSemicolonContainer has not (yet) run on it.
func containerStillHasSemicolon(root *Node, c int) bool {
	container := resultChildAt(root, c)
	if container == nil {
		return false
	}
	for i := 0; i < resultChildCount(container); i++ {
		if child := resultChildAt(container, i); child != nil && child.symbol == testGoCompatSemiSym {
			return true
		}
	}
	return false
}

// newGoCompatBudgetTestParser returns a *Parser configured with a runtime
// memory budget baselined at the current live heap, mirroring how
// (*Parser).enterRuntimeMemoryBudget configures a real parse.
//
// The hard ceiling is armed at the same size as the soft budget. Since the
// issue #454 determinism contract, only the hard ceiling may stop a parse from
// a runtime.MemStats reading, so these compat-walk propagation tests need it
// armed to have any runtime-sourced stop to propagate at all.
func newGoCompatBudgetTestParser(budgetBytes int64) *Parser {
	debug.FreeOSMemory()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return &Parser{
		parseRuntimeMemoryBudgetBytes:      budgetBytes,
		parseRuntimeMemoryHardCeilingBytes: budgetBytes,
		parseRuntimeMemoryBaselineBytes:    stats.HeapAlloc,
		parseRuntimeMemoryBaselineSys:      stats.Sys,
		parseMemoryBudgetDiagActive:        true,
	}
}

// TestWalkGoCompatSubtreeUnboundedBaselineCompletes establishes the baseline:
// with no memory-budget parser wired into the poller (the pre-fix shape —
// parseStopPoller{} carries no way to express memory-budget awareness at
// all), the walk runs to completion over every container regardless of size.
// This is the control for TestWalkGoCompatSubtreeStopsOnMemoryBudget below.
func TestWalkGoCompatSubtreeUnboundedBaselineCompletes(t *testing.T) {
	const numContainers, containerWidth = 400, 2000
	root, source, syms := buildGoCompatWideContainerTree(numContainers, containerWidth)

	poller := parseStopPoller{}
	reason := normalizeGoCompatibilitySubtreeWithStopAndScratch(root, source, syms, goCompatibilitySourceFlags{}, nil, &poller, nil)
	if reason != ParseStopNone {
		t.Fatalf("reason = %q, want %q (unbounded walk must not stop)", reason, ParseStopNone)
	}
	for c := 0; c < numContainers; c++ {
		if containerStillHasSemicolon(root, c) {
			t.Fatalf("container %d still has a droppable semicolon after an unbounded walk", c)
		}
	}
}

// TestWalkGoCompatSubtreeStopsOnMemoryBudget is the core regression: wiring a
// tiny-budget *Parser into the poller (parseStopPoller.memoryBudgetParser,
// the walkGoCompatSubtree-side fix) must make the walk (a) return
// ParseStopMemoryBudget, (b) stop well before the last container is reached
// (proving it bailed out mid-walk rather than finishing then reporting the
// budget as an afterthought), and (c) keep total heap growth within a small
// multiple of the configured budget — even though an unbounded walk over the
// identical tree (see the baseline test above) would keep going regardless.
func TestWalkGoCompatSubtreeStopsOnMemoryBudget(t *testing.T) {
	const numContainers, containerWidth = 400, 2000
	root, source, syms := buildGoCompatWideContainerTree(numContainers, containerWidth)

	const budgetBytes = 1 << 20 // 1MiB: far smaller than the ~400*2000*8B = 6.4MB
	// this walk would allocate (in kept-slice filtering alone) if it ran to
	// completion, so the trip must land partway through, not at the end.
	parser := newGoCompatBudgetTestParser(budgetBytes)

	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	poller := parseStopPoller{memoryBudgetParser: parser}
	reason := normalizeGoCompatibilitySubtreeWithStopAndScratch(root, source, syms, goCompatibilitySourceFlags{}, nil, &poller, nil)

	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	if reason != ParseStopMemoryBudget {
		t.Fatalf("reason = %q, want %q", reason, ParseStopMemoryBudget)
	}
	if !parser.compatMemoryBudgetTripped {
		t.Fatal("compatMemoryBudgetTripped = false, want true (sticky latch for resultMaterializationStopReason's later recheck)")
	}

	processed, unprocessed := 0, 0
	for c := 0; c < numContainers; c++ {
		if containerStillHasSemicolon(root, c) {
			unprocessed++
		} else {
			processed++
		}
	}
	if unprocessed == 0 {
		t.Fatalf("all %d containers were processed before the budget trip fired; want a partial walk (processed=%d unprocessed=%d)", numContainers, processed, unprocessed)
	}
	if processed == 0 {
		t.Fatalf("no containers were processed at all; want some real work done before the trip (processed=%d unprocessed=%d)", processed, unprocessed)
	}
	// The last container must still be untouched: pushChild processes root's
	// children in original order (0..numContainers-1), so if the walk had run
	// to completion the final container would already be normalized.
	if !containerStillHasSemicolon(root, numContainers-1) {
		t.Fatal("last container was processed; want the walk to have stopped before reaching it")
	}

	growth := int64(0)
	if after.HeapAlloc > before.HeapAlloc {
		growth = int64(after.HeapAlloc - before.HeapAlloc)
	}
	const maxOvershoot = 6 * budgetBytes
	t.Logf("processed=%d/%d unprocessed=%d heap growth=%d bytes (budget=%d, %.2fx)",
		processed, numContainers, unprocessed, growth, int64(budgetBytes), float64(growth)/float64(budgetBytes))
	if growth > maxOvershoot {
		t.Fatalf("heap growth = %d bytes, want <= %d (6x budget=%d)", growth, maxOvershoot, int64(budgetBytes))
	}
}

// TestNormalizeGoCompatibilityWithParserPropagatesMemoryBudgetStop exercises
// the full production entry point (normalizeGoCompatibilityWithParser, the
// exact function normalizeGoReturnedTreeCompatibilityWithCensus calls) instead of
// walkGoCompatSubtree directly, confirming the poller wiring done in
// normalizeGoCompatibilityInRangesWithStopAndScratch (parser_result_go_compat.go)
// reaches all the way from a *Parser's configured budget to the returned
// ParseStopReason.
func TestNormalizeGoCompatibilityWithParserPropagatesMemoryBudgetStop(t *testing.T) {
	const numContainers, containerWidth = 400, 2000
	root, source, _ := buildGoCompatWideContainerTree(numContainers, containerWidth)
	lang := goCompatMemoryBudgetTestLanguage()

	const budgetBytes = 1 << 20
	parser := newGoCompatBudgetTestParser(budgetBytes)
	parser.language = lang

	reason := normalizeGoCompatibilityWithParser(root, source, lang, parser)
	if reason != ParseStopMemoryBudget {
		t.Fatalf("normalizeGoCompatibilityWithParser reason = %q, want %q", reason, ParseStopMemoryBudget)
	}
	if !parser.compatMemoryBudgetTripped {
		t.Fatal("compatMemoryBudgetTripped = false, want true")
	}
	// resultMaterializationStopReason is what finalizeTree (parser.go) and
	// finalizeResultRoot (parser_result_root_build.go) consult afterward; the
	// sticky latch must make it report the trip reliably, matching how the
	// parse loop's own arena/scratch budget checks are surfaced.
	if got := parser.resultMaterializationStopReason(nil); got != ParseStopMemoryBudget {
		t.Fatalf("resultMaterializationStopReason() = %q, want %q after a Go compat memory-budget trip", got, ParseStopMemoryBudget)
	}
}

// goCompatMemoryBudgetTestLanguage is a minimal "go" Language whose symbol
// names match the ones goCompatibilitySymbolsForLanguage looks up by name, so
// normalizeGoCompatibilityWithParser resolves testGoCompatSemiSym /
// testGoCompatDeclSym the same way it would for a real grammar.
func goCompatMemoryBudgetTestLanguage() *Language {
	names := make([]string, 104)
	meta := make([]SymbolMetadata, 104)
	for i := range names {
		names[i] = ""
	}
	names[testGoCompatIdentSym] = "identifier"
	meta[testGoCompatIdentSym] = SymbolMetadata{Name: "identifier", Visible: true, Named: true}
	names[testGoCompatSemiSym] = ";"
	meta[testGoCompatSemiSym] = SymbolMetadata{Name: ";", Visible: true, Named: false}
	names[testGoCompatDeclSym] = "const_declaration"
	meta[testGoCompatDeclSym] = SymbolMetadata{Name: "const_declaration", Visible: true, Named: true}
	names[testGoCompatRootSym] = "source_file"
	meta[testGoCompatRootSym] = SymbolMetadata{Name: "source_file", Visible: true, Named: true}
	return &Language{
		Name:           "go",
		SymbolNames:    names,
		SymbolMetadata: meta,
	}
}
