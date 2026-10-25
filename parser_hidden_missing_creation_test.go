package gotreesitter

import "testing"

func hiddenMissingCreationShift(t *testing.T, p *Parser, arena *nodeArena, stack *glrStack, extra, missing bool, offset uint32) stackEntry {
	t.Helper()
	tok := Token{Symbol: 2, StartByte: offset, EndByte: offset, StartPoint: Point{Column: offset}, EndPoint: Point{Column: offset}, Missing: missing}
	act := ParseAction{Type: ParseActionShift, State: 1, Extra: extra}
	trackErrors := false
	if extra {
		var scratch parserScratch
		p.applyExtraShiftAction(stack, stack.top().state, act, tok, arena, &scratch, &trackErrors)
	} else {
		nodeCount := 0
		p.applyShiftAction(stack, act, tok, &nodeCount, arena, nil, nil, &trackErrors)
	}
	entry := stack.top()
	if !stackEntryHasNode(entry) || stackEntryNodeIsMissing(entry) != missing {
		t.Fatalf("shift did not publish expected payload: missing=%t entry=%+v", missing, entry)
	}
	if missing && (!stackEntryNodeHasError(entry) || !trackErrors || stackEntryNode(entry) == nil) {
		t.Fatal("genuine MISSING did not take the materialized error-leaf path")
	}
	return entry
}

func TestHiddenMissingCreationEnablesRawErrorPropagation(t *testing.T) {
	for _, mode := range []string{"node", "compact-full-configured", "compact-no-tree-configured"} {
		for _, extra := range []bool{false, true} {
			name := mode + "/ordinary"
			if extra {
				name = mode + "/extra"
			}
			t.Run(name, func(t *testing.T) {
				arena := acquireNodeArena(arenaClassFull)
				defer arena.Release()
				p := testRawShapeParser()
				p.compactFullShiftLeaves = mode == "compact-full-configured"
				p.noTreeBenchmarkOnly = mode == "compact-no-tree-configured"
				p.compactNoTreeShiftLeaves = p.noTreeBenchmarkOnly
				stack := newGLRStack(0)
				missing := hiddenMissingCreationShift(t, p, arena, &stack, extra, true, 1)
				if !p.retainExternalFallbackMissingFlags {
					t.Fatal("actual MISSING creation did not retain its hidden history")
				}
				leaf := newLeafNodeInArena(arena, 3, true, 0, 1, Point{}, Point{Column: 1})
				hidden := newParentNodeInArenaNoLinksWithFieldSources(arena, 2, false, []*Node{leaf}, nil, nil, 0, false)
				hidden.rawShape = p.captureRawShape(nil, arena, 2, 0, []stackEntry{newStackEntryNode(0, leaf), missing}, 0, 2)
				var scratch gssScratch
				p.pushStackNode(&stack, 1, hidden, nil, &scratch)
				root := newParentNodeInArenaNoLinksWithFieldSources(arena, 1, true, []*Node{leaf}, nil, nil, 0, false)
				root.rawShape = p.captureRawShape(nil, arena, 1, 0, []stackEntry{newStackEntryNode(0, hidden)}, 0, 1)
				p.pushStackNode(&stack, 1, root, nil, &scratch)
				if !hidden.HasError() || !root.HasError() {
					t.Fatal("hidden MISSING did not propagate through flattened ancestors")
				}
				if !reconcileStaleHasErrorFlagsWithMissing(root, 0, p.retainExternalFallbackMissingFlags) || !root.HasError() {
					t.Fatal("selected-root reconciliation erased genuine hidden MISSING")
				}
			})
		}
	}
}

func TestHiddenMissingDiscardedTrialPreservesCleanSiblingAndOldTree(t *testing.T) {
	lang := buildArithmeticLanguage()
	p := NewParser(lang)
	source := []byte("1+2")
	oldTree := mustParse(t, p, source)
	defer oldTree.Release()
	oldRoot := oldTree.RootNode()
	before := oldRoot.SExpr(lang)
	if oldRoot.HasError() {
		t.Fatal("clean old-tree control already has an error")
	}
	assertReachableRawShapeRefsCleared(t, oldRoot)
	flags := map[*Node]nodeFlags{}
	parents := map[*Node]*Node{}
	var remember func(*Node)
	remember = func(n *Node) {
		if n == nil {
			return
		}
		flags[n], parents[n] = n.flags, n.parent
		for _, child := range n.children {
			remember(child)
		}
	}
	remember(oldRoot)

	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	var scratch gssScratch
	base := newGLRStack(0)
	p.pushStackNode(&base, 1, oldRoot, nil, &scratch)
	cleanSibling := base.clone()
	trial := base.clone()
	// A speculative version inserts MISSING and is then discarded. The
	// parser latch stays on, but it must not become a tree-wide error bit.
	hiddenMissingCreationShift(t, p, arena, &trial, false, true, oldRoot.EndByte())
	trial = glrStack{}
	if !p.retainExternalFallbackMissingFlags {
		t.Fatal("discarded genuine MISSING did not exercise the retained latch")
	}
	clean := newParentNodeInArenaNoLinksWithFieldSources(arena, oldRoot.symbol, true, []*Node{oldRoot}, nil, nil, 0, false)
	clean.rawShape = p.captureRawShape(nil, arena, clean.symbol, 0, []stackEntry{newStackEntryNode(1, oldRoot)}, 0, 1)
	p.pushStackNode(&cleanSibling, 1, clean, nil, &scratch)
	if clean.HasError() {
		t.Fatal("discarded MISSING version contaminated the clean sibling")
	}
	clean.setHasError(true) // A stale recovery flag still needs to clear.
	if reconcileStaleHasErrorFlagsWithMissing(clean, 0, p.retainExternalFallbackMissingFlags) || clean.HasError() {
		t.Fatal("retained latch prevented clearing an unsupported error claim")
	}
	for node, flag := range flags {
		if node.flags != flag || node.parent != parents[node] || node.rawShape != 0 {
			t.Fatal("speculative MISSING/clean publication mutated the old tree")
		}
	}
	if oldRoot.SExpr(lang) != before || oldRoot.HasError() {
		t.Fatal("old tree changed after the discarded MISSING trial")
	}
	// parseInternal starts a fresh retention scope, including no-edit reuse.
	reparsed := mustParseIncremental(t, p, source, oldTree)
	defer reparsed.Release()
	if reparsed.RootNode().HasError() || reparsed.RootNode().SExpr(lang) != before || oldRoot.SExpr(lang) != before {
		t.Fatal("a later clean reparse inherited the speculative MISSING")
	}
}

func TestHiddenMissingZeroWidthAutomaticTokenDoesNotLatch(t *testing.T) {
	for _, compact := range []bool{false, true} {
		arena := acquireNodeArena(arenaClassFull)
		p := testRawShapeParser()
		p.compactFullShiftLeaves = compact
		stack := newGLRStack(0)
		hiddenMissingCreationShift(t, p, arena, &stack, false, false, 1)
		if p.retainExternalFallbackMissingFlags {
			t.Fatal("ordinary zero-width automatic token was treated as MISSING")
		}
		arena.Release()
	}
}
