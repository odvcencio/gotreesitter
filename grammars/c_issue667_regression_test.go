package grammars_test

import (
	"reflect"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestIssue667CEnumListsRemainHealthy(t *testing.T) {
	lang := grammars.CLanguage()
	for _, test := range issue667CEnumCases() {
		test := test
		t.Run(test.name, func(t *testing.T) {
			for run := 1; run <= 2; run++ {
				tree, err := gotreesitter.NewParser(lang).Parse([]byte(test.source))
				if err != nil {
					t.Fatalf("Parse run %d: %v", run, err)
				}
				defer tree.Release()
				issue667RequireHealthyTree(t, lang, tree, []byte(test.source))
				if got := issue667Enumerators(tree.RootNode(), lang, []byte(test.source)); !reflect.DeepEqual(got, test.enumerators) {
					t.Fatalf("enumerators = %q, want %q; tree=%s", got, test.enumerators, tree.RootNode().SExpr(lang))
				}
			}
		})
	}
}

func TestIssue667CEnumTokenSourceRouteRemainsHealthy(t *testing.T) {
	lang := grammars.CLanguage()
	for _, test := range issue667CEnumCases() {
		test := test
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source)
			tree, err := gotreesitter.NewParser(lang).ParseWithTokenSource(source, grammars.NewCTokenSourceOrEOF(source, lang))
			if err != nil {
				t.Fatalf("ParseWithTokenSource: %v", err)
			}
			defer tree.Release()
			issue667RequireHealthyTree(t, lang, tree, source)
			if got := issue667Enumerators(tree.RootNode(), lang, source); !reflect.DeepEqual(got, test.enumerators) {
				t.Fatalf("enumerators = %q, want %q; tree=%s", got, test.enumerators, tree.RootNode().SExpr(lang))
			}
		})
	}
}

func TestIssue667CEnumForestCandidateRemainsHealthy(t *testing.T) {
	lang := grammars.CLanguage()
	for _, test := range issue667CEnumCases() {
		test := test
		t.Run(test.name, func(t *testing.T) {
			tree, ok := gotreesitter.NewParser(lang).ParseForestExperimental([]byte(test.source))
			if !ok || tree == nil {
				t.Fatal("forest candidate declined")
			}
			defer tree.Release()
			issue667RequireHealthyTree(t, lang, tree, []byte(test.source))
			if got := issue667Enumerators(tree.RootNode(), lang, []byte(test.source)); !reflect.DeepEqual(got, test.enumerators) {
				t.Fatalf("enumerators = %q, want %q; tree=%s", got, test.enumerators, tree.RootNode().SExpr(lang))
			}
		})
	}
}

func TestIssue667CEnumIncrementalMatchesFresh(t *testing.T) {
	lang := grammars.CLanguage()
	cases := []struct {
		name        string
		before      string
		after       string
		enumerators []string
	}{
		{
			name:        "two to three",
			before:      "enum E { A, B };\n",
			after:       "enum E { A, B, C };\n",
			enumerators: []string{"A", "B", "C"},
		},
		{
			name:        "three to four",
			before:      "enum E { A, B, C };\n",
			after:       "enum E { A, B, C, D };\n",
			enumerators: []string{"A", "B", "C", "D"},
		},
		{
			name:        "add trailing comma",
			before:      "enum E { A, B, C };\n",
			after:       "enum E { A, B, C, };\n",
			enumerators: []string{"A", "B", "C"},
		},
		{
			name:        "remove trailing comma",
			before:      "enum E { A, B, C, };\n",
			after:       "enum E { A, B, C };\n",
			enumerators: []string{"A", "B", "C"},
		},
		{
			name:        "add explicit values",
			before:      "enum E { A = 1, B = 2 };\n",
			after:       "enum E { A = 1, B = 2, C = 3 };\n",
			enumerators: []string{"A", "B", "C"},
		},
	}

	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			oldTree, err := gotreesitter.NewParser(lang).Parse([]byte(test.before))
			if err != nil {
				t.Fatalf("base Parse: %v", err)
			}
			defer oldTree.Release()
			issue667ApplyEdit(t, oldTree, []byte(test.before), []byte(test.after))

			incremental, _, err := gotreesitter.NewParser(lang).ParseIncrementalProfiled([]byte(test.after), oldTree)
			if err != nil {
				t.Fatalf("ParseIncrementalProfiled: %v", err)
			}
			defer incremental.Release()
			issue667RequireHealthyTree(t, lang, incremental, []byte(test.after))
			if got := issue667Enumerators(incremental.RootNode(), lang, []byte(test.after)); !reflect.DeepEqual(got, test.enumerators) {
				t.Fatalf("incremental enumerators = %q, want %q", got, test.enumerators)
			}
			fresh, err := gotreesitter.NewParser(lang).Parse([]byte(test.after))
			if err != nil {
				t.Fatalf("fresh Parse: %v", err)
			}
			defer fresh.Release()
			issue667RequireHealthyTree(t, lang, fresh, []byte(test.after))
			if got, want := incremental.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang); got != want {
				t.Fatalf("incremental tree = %s, want fresh tree = %s", got, want)
			}
		})
	}
}

func TestIssue667CEnumTokenSourceIncrementalMatchesFresh(t *testing.T) {
	lang := grammars.CLanguage()
	before := []byte("enum E { A, B };\n")
	after := []byte("enum E { A, B, C };\n")
	oldTree, err := gotreesitter.NewParser(lang).ParseWithTokenSource(before, grammars.NewCTokenSourceOrEOF(before, lang))
	if err != nil {
		t.Fatalf("base ParseWithTokenSource: %v", err)
	}
	defer oldTree.Release()
	issue667ApplyEdit(t, oldTree, before, after)

	incremental, profile, err := gotreesitter.NewParser(lang).ParseIncrementalWithTokenSourceProfiled(after, oldTree, grammars.NewCTokenSourceOrEOF(after, lang))
	if err != nil {
		t.Fatalf("ParseIncrementalWithTokenSourceProfiled: %v", err)
	}
	defer incremental.Release()
	issue667RequireHealthyTree(t, lang, incremental, after)
	if incremental.UsedForestFastPath() {
		if got, want := profile.ReuseUnsupportedReason, "forest_recovery_fallback"; got != want {
			t.Fatalf("reuse unsupported reason = %q, want %q", got, want)
		}
	}

	fresh, err := gotreesitter.NewParser(lang).ParseWithTokenSource(after, grammars.NewCTokenSourceOrEOF(after, lang))
	if err != nil {
		t.Fatalf("fresh ParseWithTokenSource: %v", err)
	}
	defer fresh.Release()
	issue667RequireHealthyTree(t, lang, fresh, after)
	if got, want := incremental.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang); got != want {
		t.Fatalf("incremental tree = %s, want fresh tree = %s", got, want)
	}
}

func TestIssue667ForestFallbackRejectsIncompleteInput(t *testing.T) {
	lang := grammars.CLanguage()
	source := []byte("enum E { A, B, C\n")
	tree, err := gotreesitter.NewParser(lang).Parse(source)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	if !tree.RootNode().HasErrorOrMissing() {
		t.Fatalf("incomplete input returned a healthy tree: %s", tree.RootNode().SExpr(lang))
	}
	if tree.UsedForestFastPath() {
		t.Fatal("forest fallback accepted incomplete input")
	}
}

type issue667CEnumCase struct {
	name        string
	source      string
	enumerators []string
}

func issue667CEnumCases() []issue667CEnumCase {
	return []issue667CEnumCase{
		{name: "one", source: "enum E { A };\n", enumerators: []string{"A"}},
		{name: "two", source: "enum E { A, B };\n", enumerators: []string{"A", "B"}},
		{name: "three", source: "enum E { A, B, C };\n", enumerators: []string{"A", "B", "C"}},
		{name: "four", source: "enum E { A, B, C, D };\n", enumerators: []string{"A", "B", "C", "D"}},
		{name: "five", source: "enum E { A, B, C, D, E };\n", enumerators: []string{"A", "B", "C", "D", "E"}},
		{name: "trailing comma", source: "enum E { A, B, C, };\n", enumerators: []string{"A", "B", "C"}},
		{name: "explicit values", source: "enum E { A = 1, B = 2, C = 3 };\n", enumerators: []string{"A", "B", "C"}},
		{name: "typedef", source: "typedef enum { RED, GREEN, BLUE } Colour;\n", enumerators: []string{"RED", "GREEN", "BLUE"}},
		{name: "comment before close", source: "enum E { A, B, C /* close */\n};\n", enumerators: []string{"A", "B", "C"}},
		{name: "neighboring declarations", source: "enum First { A, B, C };\nenum Second { D, E, F };\n", enumerators: []string{"A", "B", "C", "D", "E", "F"}},
	}
}

func issue667RequireHealthyTree(t *testing.T, lang *gotreesitter.Language, tree *gotreesitter.Tree, source []byte) {
	t.Helper()
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned no root")
	}
	if tree.ParseStoppedEarly() {
		t.Fatalf("parse stopped early: %s", tree.ParseRuntime().Summary())
	}
	root := tree.RootNode()
	if got, want := root.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("root end = %d, want %d; tree=%s", got, want, root.SExpr(lang))
	}
	var errors, missing int
	issue667Walk(root, func(node *gotreesitter.Node) {
		if node.IsError() {
			errors++
		}
		if node.IsMissing() {
			missing++
		}
	})
	if errors != 0 || missing != 0 {
		t.Fatalf("recovery nodes: errors=%d missing=%d; rootHasError=%t runtime=%s tree=%s", errors, missing, root.HasError(), tree.ParseRuntime().Summary(), root.SExpr(lang))
	}
}

func issue667Enumerators(root *gotreesitter.Node, lang *gotreesitter.Language, source []byte) []string {
	var names []string
	issue667Walk(root, func(node *gotreesitter.Node) {
		if node.Type(lang) != "enumerator" {
			return
		}
		for i := 0; i < node.ChildCount(); i++ {
			child := node.Child(i)
			if child.Type(lang) == "identifier" {
				names = append(names, string(source[child.StartByte():child.EndByte()]))
				return
			}
		}
	})
	return names
}

func issue667Walk(node *gotreesitter.Node, visit func(*gotreesitter.Node)) {
	if node == nil {
		return
	}
	visit(node)
	for i := 0; i < node.ChildCount(); i++ {
		issue667Walk(node.Child(i), visit)
	}
}

func issue667ApplyEdit(t *testing.T, tree *gotreesitter.Tree, before, after []byte) {
	t.Helper()
	start := 0
	for start < len(before) && start < len(after) && before[start] == after[start] {
		start++
	}
	oldEnd := len(before)
	newEnd := len(after)
	for oldEnd > start && newEnd > start && before[oldEnd-1] == after[newEnd-1] {
		oldEnd--
		newEnd--
	}
	if start == oldEnd && start == newEnd {
		t.Fatal("edit has no changed bytes")
	}
	tree.Edit(gotreesitter.InputEdit{
		StartByte:   uint32(start),
		OldEndByte:  uint32(oldEnd),
		NewEndByte:  uint32(newEnd),
		StartPoint:  pointAtByte(before, start),
		OldEndPoint: pointAtByte(before, oldEnd),
		NewEndPoint: pointAtByte(after, newEnd),
	})
}
