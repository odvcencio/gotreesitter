package gotreesitter

import "testing"

func collapsedChildPolicyTestLanguage(rule collapsedChildOccurrenceRule) (*Language, Symbol, Symbol) {
	lang := &Language{
		Name:                      rule.languageName,
		TokenCount:                4,
		NativeResultCompatibility: ResultCompatibilityNativeCollapsedChildren,
		SymbolNames:               []string{"EOF", "root", rule.parentName, rule.childName},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "root", Visible: true, Named: true},
			{Name: rule.parentName, Visible: true, Named: true},
			{Name: rule.childName, Visible: true, Named: rule.childNamed},
		},
	}
	return lang, 2, 3
}

func TestCollapsedChildOccurrencePolicyNativelyRetainsAllLedgerRows(t *testing.T) {
	if got, want := len(collapsedChildOccurrenceRules), 24; got != want {
		t.Fatalf("collapsed-child ledger rows = %d, want %d", got, want)
	}
	for _, rule := range collapsedChildOccurrenceRules {
		rule := rule
		t.Run(rule.languageName+"/"+rule.parentName, func(t *testing.T) {
			lang, parentSymbol, childSymbol := collapsedChildPolicyTestLanguage(rule)
			parser := NewParser(lang)
			if !parser.retainsCollapsedChildOccurrence(parentSymbol, childSymbol) {
				t.Fatal("compiled occurrence policy omitted ledger pair")
			}
			arena := newNodeArena(arenaClassFull)
			end := uint32(len(rule.childName))
			original := newLeafNodeInArena(arena, childSymbol, symbolIsNamed(lang, childSymbol), 0, end, Point{}, Point{Column: end})

			var parent *Node
			switch rule.languageName {
			case "ruby", "kotlin":
				lang.AliasSequences = [][]Symbol{{parentSymbol}}
				parser = NewParser(lang)
				children, _, _ := parser.buildReduceChildren(
					[]stackEntry{newStackEntryNode(0, original)}, 0, 1, 1, 1, 0, arena,
				)
				if len(children) != 1 {
					t.Fatalf("native alias path children = %d", len(children))
				}
				parent = children[0]
				if parent == original || parent == nil || parent.symbol != parentSymbol || parent.ChildCount() != 1 {
					t.Fatalf("native alias retention = %#v", parent)
				}
				child := parent.Child(0)
				if child == nil || child == original || child.symbol != childSymbol {
					t.Fatalf("retained alias child = %#v, original=%p", child, original)
				}
				if original.parent != nil {
					t.Fatal("alias retention mutated borrowed child parent")
				}
			default:
				action := ParseAction{Type: ParseActionReduce, Symbol: parentSymbol, ChildCount: 1}
				entries := []stackEntry{newStackEntryNode(0, original)}
				if collapsed := parser.collapsibleRawUnarySelfReduction(action, Token{}, arena, entries, 0, 1); collapsed != nil {
					t.Fatalf("native unary retention collapsed to %p", collapsed)
				}
				lang.KeywordCaptureToken = 1
				if collapsed := parser.forestCollapsibleNamedKeywordLeaf(action, Token{}, arena, entries, 0, 1); collapsed != nil {
					t.Fatalf("forest native unary retention collapsed to %p", collapsed)
				}
				parent = newParentNodeInArena(arena, parentSymbol, true, []*Node{original}, nil, 0)
			}

			if parent.ChildCount() != 1 || parent.Child(0).symbol != childSymbol {
				t.Fatalf("native shape parent=%d children=%d child=%v", parent.symbol, parent.ChildCount(), parent.Child(0))
			}
		})
	}
}

func TestCollapsedChildOccurrencePolicyNegativeControls(t *testing.T) {
	t.Run("go-same-name-still-collapses", func(t *testing.T) {
		lang := &Language{
			Name: "go", TokenCount: 4,
			SymbolNames: []string{"EOF", "root", "true", "true"},
			SymbolMetadata: []SymbolMetadata{
				{Name: "EOF"}, {Name: "root", Visible: true, Named: true},
				{Name: "true", Visible: true, Named: true}, {Name: "true", Visible: true},
			},
		}
		parser := NewParser(lang)
		arena := newNodeArena(arenaClassFull)
		child := newLeafNodeInArena(arena, 3, false, 0, 4, Point{}, Point{Column: 4})
		action := ParseAction{Type: ParseActionReduce, Symbol: 2, ChildCount: 1}
		if collapsed := parser.collapsibleRawUnarySelfReduction(action, Token{}, arena, []stackEntry{newStackEntryNode(0, child)}, 0, 1); collapsed == nil {
			t.Fatal("unlisted Go same-name token stopped collapsing")
		}
	})

	t.Run("fielded-unary-remains-fielded", func(t *testing.T) {
		rule := collapsedChildOccurrenceRule{languageName: "apex", parentName: "super", childName: "super"}
		lang, parentSymbol, childSymbol := collapsedChildPolicyTestLanguage(rule)
		lang.FieldMapSlices = [][2]uint16{{0, 1}}
		lang.FieldMapEntries = []FieldMapEntry{{FieldID: 1, ChildIndex: 0}}
		parser := NewParser(lang)
		arena := newNodeArena(arenaClassFull)
		child := newLeafNodeInArena(arena, childSymbol, false, 0, 5, Point{}, Point{Column: 5})
		action := ParseAction{Type: ParseActionReduce, Symbol: parentSymbol, ChildCount: 1}
		if collapsed := parser.collapsibleRawUnarySelfReduction(action, Token{}, arena, []stackEntry{newStackEntryNode(0, child)}, 0, 1); collapsed != nil {
			t.Fatal("fielded unary occurrence collapsed")
		}
		parent := newParentNodeInArena(arena, parentSymbol, true, []*Node{child}, []FieldID{1}, 0)
		if parent.fieldIDs()[0] != 1 {
			t.Fatal("field metadata was not preserved")
		}
	})

	t.Run("inherited-field-is-not-promoted-to-direct", func(t *testing.T) {
		rule := collapsedChildOccurrenceRule{languageName: "ruby", parentName: "bare_string", childName: "string_content", childNamed: true}
		lang, parentSymbol, childSymbol := collapsedChildPolicyTestLanguage(rule)
		lang.FieldMapSlices = [][2]uint16{{0, 1}}
		lang.FieldMapEntries = []FieldMapEntry{{FieldID: 1, ChildIndex: 0, Inherited: true}}
		lang.AliasSequences = [][]Symbol{{parentSymbol}}
		parser := NewParser(lang)
		arena := newNodeArena(arenaClassFull)
		original := newLeafNodeInArena(arena, childSymbol, true, 0, 1, Point{}, Point{Column: 1})
		children, fields, sources := parser.buildReduceChildren(
			[]stackEntry{newStackEntryNode(0, original)}, 0, 1, 1, 1, 0, arena,
		)
		if len(children) != 1 || children[0] == original || children[0].symbol != parentSymbol || children[0].ChildCount() != 1 {
			t.Fatalf("retained inherited-field child = %#v", children)
		}
		if len(fields) != 1 || fields[0] != 0 || fieldSourceAt(sources, 0) != fieldSourceNone {
			t.Fatalf("inherited alias field was promoted: field=%v source=%v", fields, sources)
		}
		if original.parent != nil {
			t.Fatal("retained inherited-field alias mutated borrowed child")
		}
	})

	for _, test := range []struct {
		name  string
		mark  func(*Node)
		check func(*Node) bool
	}{
		{name: "missing", mark: func(n *Node) { n.setMissing(true) }, check: func(n *Node) bool { return n.isMissing() }},
		{name: "error", mark: func(n *Node) { n.setHasError(true) }, check: func(n *Node) bool { return n.hasError() }},
	} {
		t.Run("flagged-alias-fails-closed-"+test.name, func(t *testing.T) {
			rule := collapsedChildOccurrenceRule{languageName: "ruby", parentName: "bare_string", childName: "string_content", childNamed: true}
			lang, parentSymbol, childSymbol := collapsedChildPolicyTestLanguage(rule)
			lang.AliasSequences = [][]Symbol{{parentSymbol}}
			parser := NewParser(lang)
			arena := newNodeArena(arenaClassFull)
			original := newLeafNodeInArena(arena, childSymbol, true, 0, 1, Point{}, Point{Column: 1})
			test.mark(original)
			children, _, _ := parser.buildReduceChildren(
				[]stackEntry{newStackEntryNode(0, original)}, 0, 1, 1, 1, 0, arena,
			)
			if len(children) != 1 || children[0].symbol != parentSymbol || children[0].ChildCount() != 0 || !test.check(children[0]) {
				t.Fatalf("flagged alias did not fail closed: %#v", children)
			}
			if original.parent != nil {
				t.Fatal("borrowed flagged child was mutated")
			}
		})
	}

	t.Run("extra-skips-aliasing-on-live-child-build-path", func(t *testing.T) {
		rule := collapsedChildOccurrenceRule{languageName: "ruby", parentName: "bare_string", childName: "string_content", childNamed: true}
		lang, parentSymbol, childSymbol := collapsedChildPolicyTestLanguage(rule)
		lang.AliasSequences = [][]Symbol{{parentSymbol}}
		parser := NewParser(lang)
		arena := newNodeArena(arenaClassFull)
		original := newLeafNodeInArena(arena, childSymbol, true, 0, 1, Point{}, Point{Column: 1})
		original.setExtra(true)
		children, _, _ := parser.buildReduceChildren(
			[]stackEntry{newStackEntryNode(0, original)}, 0, 1, 1, 1, 0, arena,
		)
		if len(children) != 1 || children[0] != original || children[0].symbol != childSymbol || !children[0].isExtra() {
			t.Fatalf("extra alias path = %#v", children)
		}
	})

	t.Run("uncertified-exact-metadata-identities-do-not-opt-in", func(t *testing.T) {
		for _, rule := range []collapsedChildOccurrenceRule{
			{languageName: "apex", parentName: "super", childName: "super"},
			{languageName: "ruby", parentName: "bare_string", childName: "string_content", childNamed: true},
		} {
			lang, parentSymbol, childSymbol := collapsedChildPolicyTestLanguage(rule)
			lang.NativeResultCompatibility = 0
			parser := NewParser(lang)
			if parser.retainsCollapsedChildOccurrence(parentSymbol, childSymbol) {
				t.Fatalf("uncertified exact-metadata %s artifact compiled native policy", rule.languageName)
			}
		}
	})

	t.Run("registered-name-with-wrong-child-namedness-does-not-opt-in", func(t *testing.T) {
		lang := &Language{
			Name:                      "apex",
			TokenCount:                4,
			NativeResultCompatibility: ResultCompatibilityNativeCollapsedChildren,
			SymbolNames:               []string{"EOF", "root", "super", "super"},
			SymbolMetadata: []SymbolMetadata{
				{Name: "EOF"}, {Name: "root", Visible: true, Named: true},
				{Name: "super", Visible: true, Named: true},
				{Name: "super", Visible: true, Named: true},
			},
		}
		parser := NewParser(lang)
		if parser.retainsCollapsedChildOccurrence(2, 3) || parser.retainsCollapsedChildOccurrence(2, 2) {
			t.Fatal("named alias collision compiled anonymous-child ownership")
		}
	})

	t.Run("unregistered-custom-artifact-does-not-opt-in", func(t *testing.T) {
		lang := &Language{
			Name:        "custom",
			TokenCount:  4,
			SymbolNames: []string{"EOF", "root", "super", "super"},
			SymbolMetadata: []SymbolMetadata{
				{Name: "EOF"}, {Name: "root", Visible: true, Named: true},
				{Name: "super", Visible: true, Named: true}, {Name: "super", Visible: true},
			},
		}
		parser := NewParser(lang)
		if parser.retainsCollapsedChildOccurrence(2, 3) {
			t.Fatal("unregistered custom language compiled collapsed-child policy")
		}
	})
}

func TestCollapsedNamedLeafCompatibilityTraversalIsRetired(t *testing.T) {
	lang := &Language{
		Name:       "apex",
		TokenCount: 4,
		SymbolNames: []string{
			"EOF", "root", "super", "super",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: "root", Visible: true, Named: true},
			{Name: "super", Visible: true, Named: true},
			{Name: "super", Visible: true},
		},
	}
	parser := NewParser(lang)
	parser.materializationTiming = &parseMaterializationTiming{}
	arena := newNodeArena(arenaClassFull)
	root := newLeafNodeInArena(arena, 2, true, 0, 5, Point{}, Point{Column: 5})

	normalizeResultCompatibility(root, []byte("super"), parser, nil)

	if root.ChildCount() != 0 {
		t.Fatalf("retired compatibility traversal synthesized %d children", root.ChildCount())
	}
	stats := parser.normalizationStats
	if stats.passesChecked != 0 || stats.passesRun != 0 || stats.nodesVisited != 0 || stats.nodesRewritten != 0 {
		t.Fatalf("retired compatibility traversal reported runtime counters: %+v", stats)
	}
}

func TestTerminalLeafCompatibilityMutationIsRetired(t *testing.T) {
	lang := &Language{
		TokenCount: 3,
		SymbolNames: []string{
			"EOF",
			".",
			"any_character",
			"root",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "EOF"},
			{Name: ".", Visible: true},
			{Name: "any_character", Visible: true, Named: true},
			{Name: "root", Visible: true, Named: true},
		},
	}
	parser := NewParser(lang)
	arena := newNodeArena(arenaClassFull)
	child := newLeafNodeInArena(arena, 1, false, 11, 12, Point{Column: 11}, Point{Column: 12})
	token := newParentNodeInArena(arena, 2, true, []*Node{child}, nil, 0)
	token.startByte = 10
	token.endByte = 12
	token.startPoint = Point{Column: 10}
	token.endPoint = Point{Column: 12}
	root := newParentNodeInArena(arena, 3, true, []*Node{token}, nil, 0)

	result := normalizeResultCompatibility(root, []byte("           ."), parser, nil)

	if result.stopReason != ParseStopNone {
		t.Fatalf("stop reason = %q, want none", result.stopReason)
	}
	if result.errorSummary != resultErrorSummaryClean {
		t.Fatalf("error summary = %d, want clean", result.errorSummary)
	}
	if token.ChildCount() != 1 || token.Child(0) != child {
		t.Fatalf("retired compatibility mutation changed terminal child: count=%d child=%p", token.ChildCount(), token.Child(0))
	}
	if token.StartByte() != 10 || token.EndByte() != 12 {
		t.Fatalf("retired compatibility mutation changed terminal span: %d..%d", token.StartByte(), token.EndByte())
	}
	if stats := parser.normalizationStats; stats.nodesVisited != 0 || stats.nodesRewritten != 0 {
		t.Fatalf("retired compatibility mutation reported counters: %+v", stats)
	}
}
