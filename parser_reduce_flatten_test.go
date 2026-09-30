package gotreesitter

import "testing"

func buildDeepHiddenChain(depth, leaves int) *Node {
	leafParent := &Node{
		symbol:   1,
		children: make([]*Node, leaves),
	}
	for i := 0; i < leaves; i++ {
		leafParent.children[i] = &Node{
			symbol: 2,
			flags:  nodeFlagNamed,
		}
	}
	root := leafParent
	for i := 0; i < depth; i++ {
		root = &Node{
			symbol:   1,
			children: []*Node{root},
		}
	}
	return root
}

func TestFlattenHiddenChildrenHandlesDeepInvisibleChains(t *testing.T) {
	symbolMeta := []SymbolMetadata{
		{Name: "EOF", Visible: false},
		{Name: "_hidden", Visible: false},
		{Name: "leaf", Visible: true, Named: true},
	}
	root := buildDeepHiddenChain(20000, 512)

	if got, want := countFlattenedHiddenChildren(root, symbolMeta, nil), 512; got != want {
		t.Fatalf("countFlattenedHiddenChildren() = %d, want %d", got, want)
	}

	dst := make([]*Node, 512)
	out := appendFlattenedHiddenChildren(dst, 0, root, symbolMeta, nil)
	if got, want := out, 512; got != want {
		t.Fatalf("appendFlattenedHiddenChildren() out = %d, want %d", got, want)
	}
	for i := 0; i < out; i++ {
		if dst[i] == nil {
			t.Fatalf("flattened child %d is nil", i)
		}
		if got, want := dst[i].symbol, Symbol(2); got != want {
			t.Fatalf("flattened child %d symbol = %d, want %d", i, got, want)
		}
	}
}

func TestFlattenHiddenChildrenPreservesMarkedHiddenNamedTokenWrapper(t *testing.T) {
	symbolMeta := []SymbolMetadata{
		{Name: "EOF", Visible: false},
		{Name: "_terminator", Visible: false, Named: true},
		{Name: ";", Visible: true, Named: false},
	}
	semi := &Node{symbol: 2}
	wrapper := &Node{
		symbol:   1,
		flags:    nodeFlagNamed,
		children: []*Node{semi},
	}
	preserved := []bool{false, true, false}

	if got, want := countFlattenedHiddenChildren(wrapper, symbolMeta, nil), 1; got != want {
		t.Fatalf("unmarked countFlattenedHiddenChildren() = %d, want %d", got, want)
	}
	unmarked := make([]*Node, 1)
	appendFlattenedHiddenChildren(unmarked, 0, wrapper, symbolMeta, nil)
	if unmarked[0] != semi {
		t.Fatalf("unmarked flatten child = %p, want token %p", unmarked[0], semi)
	}

	if got, want := countFlattenedHiddenChildren(wrapper, symbolMeta, preserved), 1; got != want {
		t.Fatalf("marked countFlattenedHiddenChildren() = %d, want %d", got, want)
	}
	marked := make([]*Node, 1)
	appendFlattenedHiddenChildren(marked, 0, wrapper, symbolMeta, preserved)
	if marked[0] != wrapper {
		t.Fatalf("marked flatten child = %p, want wrapper %p", marked[0], wrapper)
	}
}

func TestFlattenHiddenChildrenScratchHandlesInputDepth(t *testing.T) {
	meta := []SymbolMetadata{{Name: "EOF"}, {Name: "_hidden"}, {Name: "leaf", Visible: true, Named: true}}
	lang := &Language{SymbolMetadata: meta, SymbolNames: []string{"EOF", "_hidden", "leaf"}, SupertypeSymbols: []Symbol{1}}
	arena := newNodeArena(arenaClassFull)
	root := buildDeepHiddenChain(20000, 3)
	bottom := root
	for len(bottom.children) == 1 {
		bottom = bottom.children[0]
	}
	for i := range bottom.children {
		bottom.children[i] = newLeafNodeInArena(arena, 2, true, 0, 1, Point{}, Point{Column: 1})
	}
	root.setFieldMetadata([]FieldID{7}, []uint8{fieldSourceDirect})
	if !hiddenTreeHasFieldIDs(root) || !hiddenTreeHasFieldIDs(root) {
		t.Fatal("deep field presence was lost")
	}
	var scratch reduceBuildScratch
	appendFlattenedHiddenChildrenWithFieldScratch(&scratch, root, meta, nil, lang, arena, 2)
	if len(scratch.nodes) != 3 || len(scratch.fieldIDs) != 3 {
		t.Fatalf("deep flatten produced %d nodes, %d fields", len(scratch.nodes), len(scratch.fieldIDs))
	}
	for i, child := range scratch.nodes {
		if child.symbol != 2 || child.supertypeMask() != 3 || scratch.fieldIDs[i] != 7 {
			t.Fatalf("child %d lost symbol, supertype or field: symbol=%d mask=%d field=%d", i, child.symbol, child.supertypeMask(), scratch.fieldIDs[i])
		}
	}
	scratch.reset()
	appendFlattenedHiddenChildrenToScratch(&scratch, root, meta, nil, lang, arena, 2)
	if len(scratch.nodes) != 3 || scratch.trackFields {
		t.Fatalf("fieldless flatten produced %d nodes, trackFields=%t", len(scratch.nodes), scratch.trackFields)
	}
	plain := buildDeepHiddenChain(20000, 1)
	if hiddenTreeHasFieldIDs(plain) || hiddenTreeHasFieldIDs(plain) {
		t.Fatal("fieldless deep chain gained a field")
	}
}
