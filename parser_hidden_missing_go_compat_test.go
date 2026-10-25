package gotreesitter

import "testing"

func TestHiddenMissingGoCompatProjectionAndEvidence(t *testing.T) {
	for _, finalRefs := range []bool{false, true} {
		for _, tc := range []struct {
			name                   string
			source                 string
			symbol                 Symbol
			missing, stale         bool
			explicit, discarded    bool
			wantChildren           int
			wantError, wantRawOnly bool
		}{
			{name: "projected-missing-semicolon", source: "ab", symbol: 2, missing: true, wantChildren: 2, wantError: true, wantRawOnly: true},
			{name: "ordinary-automatic-semicolon", source: "ab", symbol: 2, wantChildren: 2},
			{name: "ordinary-newline", source: "a\nb", symbol: 2, explicit: true, wantChildren: 2},
			{name: "explicit-semicolon", source: "a;b", symbol: 2, explicit: true, wantChildren: 3},
			{name: "stale-error-clears", source: "ab", symbol: 2, stale: true, wantChildren: 2},
			{name: "discarded-missing-does-not-contaminate", source: "ab", symbol: 2, discarded: true, wantChildren: 2},
			{name: "visible-missing-nonseparator-stays-public", source: "ab", symbol: 4, missing: true, wantChildren: 3, wantError: true},
		} {
			layout := "children"
			if finalRefs {
				layout = "final-refs"
			}
			t.Run(layout+"/"+tc.name, func(t *testing.T) {
				arena := acquireNodeArena(arenaClassFull)
				defer arena.Release()
				lang := &Language{Name: "go", SymbolNames: []string{"EOF", "source_file", ";", "\x00", "identifier", "source_file_repeat1"}, SymbolMetadata: []SymbolMetadata{
					{}, {Visible: true, Named: true}, {Visible: true}, {Visible: true}, {Visible: true, Named: true}, {},
				}}
				p := &Parser{language: lang, retainExternalFallbackMissingFlags: true}
				left := newLeafNodeInArena(arena, 4, true, 0, 1, Point{}, Point{Column: 1})
				separatorEnd, rightStart := uint32(1), uint32(1)
				if tc.explicit {
					separatorEnd, rightStart = 2, 2
				}
				separator := newLeafNodeInArena(arena, tc.symbol, false, 1, separatorEnd, Point{Column: 1}, Point{Column: separatorEnd})
				separator.setMissing(tc.missing)
				separator.setHasError(tc.missing)
				right := newLeafNodeInArena(arena, 4, true, rightStart, uint32(len(tc.source)), Point{Column: rightStart}, Point{Column: uint32(len(tc.source))})
				entries := []stackEntry{newStackEntryNode(0, left), newStackEntryNode(0, separator), newStackEntryNode(0, right)}
				if tc.discarded {
					orphan := newLeafNodeInArena(arena, 2, false, 1, 1, Point{Column: 1}, Point{Column: 1})
					orphan.setMissing(true)
					orphan.setHasError(true)
				}
				var root *Node
				if finalRefs {
					childRange, refs := arena.allocPendingChildEntries(len(entries))
					for i, entry := range entries {
						refs[i] = newPendingChildEntry(entry)
					}
					root = newParentNodeInArenaWithFinalChildRefs(arena, 1, true, childRange, 0, false)
					root.endByte = uint32(len(tc.source))
					root.endPoint = Point{Column: root.endByte}
				} else {
					root = newParentNodeInArenaNoLinksWithFieldSources(arena, 1, true, []*Node{left, separator, right}, nil, nil, 0, false)
				}
				// Match the observed selected history: public children are flat,
				// while raw history retains the missing separator under a repeat.
				repeat := newParentNodeInArenaNoLinksWithFieldSources(arena, 5, false, []*Node{left, separator, right}, nil, nil, 0, false)
				repeat.rawShape = p.captureRawShape(nil, arena, 5, 0, entries, 0, len(entries))
				repeat.setHasError(tc.missing)
				root.rawShape = p.captureRawShape(nil, arena, 1, 0, []stackEntry{newStackEntryNode(0, repeat)}, 0, 1)
				root.setHasError(tc.missing || tc.stale)
				syms, _ := goCompatibilitySymbolsForLanguage(lang)
				normalizeGoSemicolonContainer(root, []byte(tc.source), syms)
				if resultChildCount(root) != tc.wantChildren {
					t.Fatalf("public projection children=%d want=%d", resultChildCount(root), tc.wantChildren)
				}
				if tc.wantRawOnly && !root.HasError() {
					t.Fatal("projecting genuine MISSING erased its container's error")
				}
				rawOnly := false
				hasError := reconcileStaleHasErrorFlagsWithMissingEvidence(root, 0, true, &rawOnly)
				if hasError != tc.wantError || root.HasError() != tc.wantError || rawOnly != tc.wantRawOnly {
					t.Fatalf("selected evidence error=%t root=%t raw-only=%t want=%t/%t", hasError, root.HasError(), rawOnly, tc.wantError, tc.wantRawOnly)
				}
				if tc.wantRawOnly && retainedHiddenMissingFlags(root, lang, arena) {
					t.Fatal("control unexpectedly relies on broadening the hidden-only visibility filter")
				}
			})
		}
	}
}

func TestHiddenMissingRepairedVisibleTokenDoesNotCreateEvidence(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	p := testRawShapeParser()
	missing := newLeafNodeInArena(arena, 3, true, 1, 1, Point{Column: 1}, Point{Column: 1})
	missing.setMissing(true)
	missing.setHasError(true)
	clean := newLeafNodeInArena(arena, 4, true, 0, 1, Point{}, Point{Column: 1})
	root := newParentNodeInArenaNoLinksWithFieldSources(arena, 1, true, []*Node{clean}, nil, nil, 0, false)
	root.rawShape = p.captureRawShape(nil, arena, 1, 0, []stackEntry{newStackEntryNode(0, clean), newStackEntryNode(0, missing)}, 0, 2)
	root.setHasError(false) // Existing visible-node repair already cleared it.
	rawOnly := false
	if reconcileStaleHasErrorFlagsWithMissingEvidence(root, 0, true, &rawOnly) || root.HasError() || rawOnly {
		t.Fatal("removed visible MISSING was resurrected without a retained error claim")
	}
}
