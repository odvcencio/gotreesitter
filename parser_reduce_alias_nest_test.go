package gotreesitter

import "testing"

// TestAliasedHiddenChildedWrapperNestsUnderAlias guards the single-iteration
// repeat1(alias($._x, $.y)) case. When a hidden node flattens to exactly ONE
// visible descendant that is itself an internal node (it has children), aliasing
// the hidden node to an outer name must NEST the inner wrapper under the new
// alias rather than rename-through (collapse) it. Mirrors upstream tree-sitter:
// a single list_item produced by repeat1(alias($._list_item, $.list_item)) and
// then aliased to (section) yields (section (list_item ...)), not a renamed
// (section ...) with the list_item layer evaporated.
func TestAliasedHiddenChildedWrapperNestsUnderAlias(t *testing.T) {
	lang := &Language{
		SymbolCount: 5,
		SymbolMetadata: []SymbolMetadata{
			{Visible: false, Named: false}, // 0: hidden container (_list)
			{Visible: true, Named: false},  // 1: anon leaf (list_marker)
			{Visible: true, Named: true},   // 2: named leaf (paragraph)
			{Visible: true, Named: true},   // 3: visible wrapper (list_item)
			{Visible: true, Named: true},   // 4: outer alias (section)
		},
	}
	arena := acquireNodeArena(arenaClassFull)

	marker := newLeafNodeInArena(arena, 1, false, 0, 1, Point{Column: 0}, Point{Column: 1})
	para := newLeafNodeInArena(arena, 2, true, 2, 6, Point{Column: 2}, Point{Column: 6})
	// list_item: a VISIBLE internal node with two children.
	listItem := newParentNodeInArena(arena, 3, true, []*Node{marker, para}, nil, 11)
	// hidden _list holding the single list_item.
	hidden := newParentNodeInArena(arena, 0, false, []*Node{listItem}, nil, 12)

	before := arena.used
	aliased := aliasedNodeInArena(arena, lang, hidden, 4)
	if aliased == nil {
		t.Fatal("expected aliased node")
	}
	if got, want := aliased.symbol, Symbol(4); got != want {
		t.Fatalf("symbol = %d, want %d (section)", got, want)
	}
	if got := arena.used - before; got != 1 {
		t.Fatalf("alias allocated %d nodes, want one materialized wrapper", got)
	}
	if hidden.symbol != 0 || aliased == hidden {
		t.Fatal("alias changed the source node")
	}
	// Must NEST: section has exactly one child, the list_item wrapper.
	if got, want := aliased.ChildCount(), 1; got != want {
		t.Fatalf("child count = %d, want %d (list_item must be nested, not collapsed)", got, want)
	}
	inner := aliased.children[0]
	if got, want := inner.symbol, Symbol(3); got != want {
		t.Fatalf("inner symbol = %d, want %d (list_item)", got, want)
	}
	if got, want := inner.ChildCount(), 2; got != want {
		t.Fatalf("inner child count = %d, want %d (list_item's marker+paragraph)", got, want)
	}
}

func TestAliasedHiddenAnonymousLeafToNamedNonterminalNestsUnderAlias(t *testing.T) {
	lang := &Language{
		SymbolCount: 3,
		TokenCount:  1,
		SymbolMetadata: []SymbolMetadata{
			{Visible: true, Named: false}, // 0: anonymous token
			{Visible: false, Named: true}, // 1: hidden wrapper
			{Visible: true, Named: true},  // 2: visible named nonterminal alias
		},
	}
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	leaf := newLeafNodeInArena(arena, 0, false, 0, 1, Point{}, Point{Column: 1})
	hidden := newParentNodeInArena(arena, 1, false, []*Node{leaf}, nil, 0)

	aliased := aliasedNodeInArena(arena, lang, hidden, 2)
	if aliased == nil {
		t.Fatal("expected aliased node")
	}
	if got, want := aliased.symbol, Symbol(2); got != want {
		t.Fatalf("symbol = %d, want %d", got, want)
	}
	if got, want := aliased.ChildCount(), 1; got != want {
		t.Fatalf("child count = %d, want %d", got, want)
	}
	child := aliased.Child(0)
	if child == nil {
		t.Fatal("aliased child = nil")
	}
	if got, want := child.symbol, Symbol(0); got != want {
		t.Fatalf("child symbol = %d, want %d", got, want)
	}
	if child.IsNamed() {
		t.Fatal("anonymous token child became named")
	}
}

func TestAliasedHiddenAnonymousLeafToSameAnonymousTokenRenamesThroughAlias(t *testing.T) {
	lang := &Language{
		SymbolCount: 3,
		TokenCount:  2,
		SymbolMetadata: []SymbolMetadata{
			{},                            // 0: no alias / EOF
			{Visible: true, Named: false}, // 1: anonymous token and alias
			{Visible: false, Named: true}, // 2: hidden token wrapper
		},
	}
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	leaf := newLeafNodeInArena(arena, 1, false, 12, 13, Point{Column: 12}, Point{Column: 13})
	hidden := newParentNodeInArena(arena, 2, false, []*Node{leaf}, nil, 0)

	aliased := aliasedNodeInArena(arena, lang, hidden, 1)
	if aliased == nil {
		t.Fatal("expected aliased node")
	}
	if got, want := aliased.symbol, Symbol(1); got != want {
		t.Fatalf("symbol = %d, want %d", got, want)
	}
	if aliased.IsNamed() {
		t.Fatal("same anonymous-token alias became named")
	}
	if got, want := aliased.ChildCount(), 0; got != want {
		t.Fatalf("child count = %d, want %d", got, want)
	}
	if got, want := aliased.StartByte(), uint32(12); got != want {
		t.Fatalf("start byte = %d, want %d", got, want)
	}
	if got, want := aliased.EndByte(), uint32(13); got != want {
		t.Fatalf("end byte = %d, want %d", got, want)
	}
}

func TestAliasedInvisibleTerminalLeafToVisibleAnonymousAliasSymbolNestsUnderAlias(t *testing.T) {
	lang := &Language{
		SymbolCount: 3,
		TokenCount:  2,
		SymbolMetadata: []SymbolMetadata{
			{},                            // 0: no alias / EOF
			{Visible: false, Named: true}, // 1: invisible terminal token
			{Visible: true, Named: false}, // 2: anonymous alias symbol outside TokenCount
		},
	}
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	leaf := newLeafNodeInArena(arena, 1, false, 12, 13, Point{Column: 12}, Point{Column: 13})

	aliased := aliasedNodeInArena(arena, lang, leaf, 2)
	if aliased == nil {
		t.Fatal("expected aliased node")
	}
	if aliased == leaf {
		t.Fatal("visible anonymous alias returned invisible leaf unchanged; want wrapper materialization")
	}
	if got, want := aliased.symbol, Symbol(2); got != want {
		t.Fatalf("symbol = %d, want %d", got, want)
	}
	if aliased.IsNamed() {
		t.Fatal("visible anonymous alias became named")
	}
	if got, want := aliased.ChildCount(), 1; got != want {
		t.Fatalf("child count = %d, want %d", got, want)
	}
	child := aliased.Child(0)
	if child == nil {
		t.Fatal("aliased child = nil")
	}
	if child == leaf {
		t.Fatal("aliased child reused invisible source leaf; want visible alias clone")
	}
	if got, want := child.symbol, Symbol(2); got != want {
		t.Fatalf("child symbol = %d, want %d", got, want)
	}
	if child.IsNamed() {
		t.Fatal("visible anonymous alias child became named")
	}
}

func TestAliasedSameAnonymousTokenLeafRemainsBareTerminalAlias(t *testing.T) {
	lang := &Language{
		SymbolCount: 2,
		TokenCount:  2,
		SymbolMetadata: []SymbolMetadata{
			{},                            // 0: no alias / EOF
			{Visible: true, Named: false}, // 1: anonymous token and alias
		},
	}
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	leaf := newLeafNodeInArena(arena, 1, false, 12, 13, Point{Column: 12}, Point{Column: 13})

	aliased := aliasedNodeInArena(arena, lang, leaf, 1)
	if aliased == nil {
		t.Fatal("expected aliased node")
	}
	if aliased != leaf {
		t.Fatalf("same anonymous-token alias = %p, want original leaf %p", aliased, leaf)
	}
	if got, want := aliased.symbol, Symbol(1); got != want {
		t.Fatalf("symbol = %d, want %d", got, want)
	}
	if aliased.IsNamed() {
		t.Fatal("same anonymous-token alias became named")
	}
	if got, want := aliased.ChildCount(), 0; got != want {
		t.Fatalf("child count = %d, want %d", got, want)
	}
	if got, want := aliased.StartByte(), uint32(12); got != want {
		t.Fatalf("start byte = %d, want %d", got, want)
	}
	if got, want := aliased.EndByte(), uint32(13); got != want {
		t.Fatalf("end byte = %d, want %d", got, want)
	}
}

// TestAliasedHiddenBareLeafStillRenamesThrough guards the companion side of the
// discriminator: a hidden node whose lone visible descendant is a childless LEAF
// (a token-shaped wrapper) must still rename-through (collapse) — the alias
// relabels it in place, e.g. alias($._hidden_token, $.name) -> (name).
func TestAliasedHiddenBareLeafStillRenamesThrough(t *testing.T) {
	lang := &Language{
		SymbolCount: 3,
		SymbolMetadata: []SymbolMetadata{
			{Visible: false, Named: false}, // 0: hidden container
			{Visible: true, Named: false},  // 1: visible leaf (childless)
			{Visible: true, Named: true},   // 2: outer alias
		},
	}
	arena := acquireNodeArena(arenaClassFull)

	leaf := newLeafNodeInArena(arena, 1, false, 4, 9, Point{Column: 4}, Point{Column: 9})
	hidden := newParentNodeInArena(arena, 0, false, []*Node{leaf}, nil, 17)

	aliased := aliasedNodeInArena(arena, lang, hidden, 2)
	if aliased == nil {
		t.Fatal("expected aliased node")
	}
	if got, want := aliased.symbol, Symbol(2); got != want {
		t.Fatalf("symbol = %d, want %d", got, want)
	}
	// Must rename-through: bare leaf collapses, childCount==0.
	if got, want := aliased.ChildCount(), 0; got != want {
		t.Fatalf("child count = %d, want %d (bare leaf must rename-through)", got, want)
	}
	if got, want := aliased.StartByte(), uint32(4); got != want {
		t.Fatalf("start byte = %d, want %d", got, want)
	}
	if got, want := aliased.EndByte(), uint32(9); got != want {
		t.Fatalf("end byte = %d, want %d", got, want)
	}
}
