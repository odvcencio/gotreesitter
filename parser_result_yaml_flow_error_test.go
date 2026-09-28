package gotreesitter_test

import (
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestYAMLUnclosedFlowSequenceKeepsErrorRoot is the regression test for task
// #96. The yaml result-compatibility pass rebuilt a document around whatever
// node came first in a recovered ERROR root, even a bare "[" token, and then
// cleared the root's error flag. C reports these inputs as an ERROR root:
//
//	"[a"  -> (ERROR "[" (flow_node (plain_scalar (string_scalar))))
//	"[\n" -> (ERROR "[")
//
// Go's fresh parse of "[a" published (stream (document)) with HasError false
// and lost the flow_node; an incremental reparse of "[" into "[\n" did the
// same for the bare bracket. Both must keep the error. The remaining frame
// difference (stream root around the ERROR on the classic GLR route, bare
// ERROR root on the fresh compact route) is task #76 and is not asserted.
func TestYAMLUnclosedFlowSequenceKeepsErrorRoot(t *testing.T) {
	lang := grammars.YamlLanguage()

	// wantErrorRoot lists the inputs whose bare ERROR root Go already
	// publishes like C. "[a" and "[a\n" keep C's ERROR content but Go still
	// frames it in the expected stream root; that frame is task #76, not
	// this test's concern.
	wantErrorRoot := map[string]bool{"[": true, "[\n": true}
	for _, src := range []string{"[", "[\n", "[a", "[a\n"} {
		tree, err := gotreesitter.NewParser(lang).Parse([]byte(src))
		if err != nil {
			t.Fatalf("Parse(%q): %v", src, err)
		}
		root := tree.RootNode()
		if !root.HasError() {
			t.Fatalf("Parse(%q): root HasError() = false, want true:\n%s", src, root.SExpr(lang))
		}
		if got := root.Type(lang); wantErrorRoot[src] && got != "ERROR" {
			t.Fatalf("Parse(%q): root type = %q, want ERROR:\n%s", src, got, root.SExpr(lang))
		}
		if src == "[a" || src == "[a\n" {
			// The flow_node content must survive; the old pass dropped it.
			if !yamlTreeContainsType(root, lang, "flow_node") {
				t.Fatalf("Parse(%q): flow_node content was dropped:\n%s", src, root.SExpr(lang))
			}
		}
		tree.Release()
	}

	// Incremental: "[" then a newline appended must agree with the fresh
	// parse of the edited source.
	p := gotreesitter.NewParser(lang)
	tree, err := p.Parse([]byte("["))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tree.Edit(gotreesitter.InputEdit{
		StartByte: 1, OldEndByte: 1, NewEndByte: 2,
		StartPoint:  gotreesitter.Point{Row: 0, Column: 1},
		OldEndPoint: gotreesitter.Point{Row: 0, Column: 1},
		NewEndPoint: gotreesitter.Point{Row: 1, Column: 0},
	})
	edited := []byte("[\n")
	incremental, err := p.ParseIncremental(edited, tree)
	if err != nil {
		t.Fatalf("ParseIncremental: %v", err)
	}
	defer incremental.Release()
	fresh, err := gotreesitter.NewParser(lang).Parse(edited)
	if err != nil {
		t.Fatalf("Parse(edited): %v", err)
	}
	defer fresh.Release()
	if !incremental.RootNode().HasError() {
		t.Fatalf("incremental root HasError() = false, want true:\n%s", incremental.RootNode().SExpr(lang))
	}
	if !fresh.RootNode().HasError() {
		t.Fatalf("fresh root HasError() = false, want true:\n%s", fresh.RootNode().SExpr(lang))
	}
	// The incremental reparse takes the classic GLR route, which still frames
	// the recovered ERROR under the stream root (task #76); the fresh parse
	// publishes C's bare ERROR root. Both must keep the ERROR and neither may
	// manufacture the old clean (stream (document)) shape.
	if !yamlTreeContainsType(incremental.RootNode(), lang, "ERROR") {
		t.Fatalf("incremental reparse lost the ERROR node:\n%s", incremental.RootNode().SExpr(lang))
	}
	if yamlTreeContainsType(incremental.RootNode(), lang, "document") {
		t.Fatalf("incremental reparse manufactured a document for an unclosed flow sequence:\n%s", incremental.RootNode().SExpr(lang))
	}
}

func yamlTreeContainsType(n *gotreesitter.Node, lang *gotreesitter.Language, typ string) bool {
	if n == nil {
		return false
	}
	if n.Type(lang) == typ {
		return true
	}
	for i := 0; i < n.ChildCount(); i++ {
		if yamlTreeContainsType(n.Child(i), lang, typ) {
			return true
		}
	}
	return false
}

// Recovering an incomplete mapping must not publish only its first scalar and
// silently discard the colon, open collection, or subsequent fields.
func TestYAMLIncompleteMappingKeepsRecoveredTokens(t *testing.T) {
	lang := grammars.YamlLanguage()
	for _, tc := range []struct {
		source string
		leaves []string
	}{
		{"server: {ho", []string{"server", ":", "{", "ho"}},
		{"regions: [east, we", []string{"regions", ":", "[", "east", ",", "we"}},
		{"server: {tls: tr", []string{"server", ":", "{", "tls", "tr"}},
		{"mode: \"pr", []string{"mode", ":", "\""}},
		{"servers:\n- host: localhost\n- {ho", []string{"servers", ":", "-", "host", "localhost", "{", "ho"}},
		{"enabled: true\r\nser", []string{"enabled", ":", "true", "ser"}},
		{"name: [\n", []string{"name", ":", "["}},
		{"enabled: true\nserver: [", []string{"enabled", ":", "true", "server", "["}},
		{"- one\n- [", []string{"-", "one", "["}},
	} {
		t.Run(tc.source, func(t *testing.T) {
			tree, err := gotreesitter.NewParser(lang).ParseStrict([]byte(tc.source))
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			root := tree.RootNode()
			if !root.HasError() || !yamlTreeContainsType(root, lang, "ERROR") {
				t.Fatalf("incomplete YAML lost its error: %s", root.SExpr(lang))
			}
			if root.StartByte() != 0 || int(root.EndByte()) != len(tc.source) {
				t.Fatalf("root spans [%d,%d), want [0,%d)", root.StartByte(), root.EndByte(), len(tc.source))
			}
			leaves := make(map[string]bool)
			var visit func(*gotreesitter.Node)
			visit = func(node *gotreesitter.Node) {
				if node.ChildCount() == 0 {
					leaves[node.Text([]byte(tc.source))] = true
				}
				for i := 0; i < node.ChildCount(); i++ {
					visit(node.Child(i))
				}
			}
			visit(root)
			for _, token := range tc.leaves {
				if !leaves[token] {
					t.Errorf("lost token %q in %s", token, root.SExpr(lang))
				}
			}
		})
	}
}

func TestYAMLIncompleteMappingIncrementalMatchesFresh(t *testing.T) {
	lang := grammars.YamlLanguage()
	parser := gotreesitter.NewParser(lang)
	source := "server: {}"
	tree, err := parser.ParseStrict([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { tree.Release() }()
	for _, edited := range []string{"server: {", "server: {ho", "server: {host: localhost", "server: {host: localhost}"} {
		start := 0
		for start < len(source) && start < len(edited) && source[start] == edited[start] {
			start++
		}
		tree.Edit(gotreesitter.InputEdit{
			StartByte: uint32(start), OldEndByte: uint32(len(source)), NewEndByte: uint32(len(edited)),
			StartPoint:  gotreesitter.Point{Column: uint32(start)},
			OldEndPoint: gotreesitter.Point{Column: uint32(len(source))},
			NewEndPoint: gotreesitter.Point{Column: uint32(len(edited))},
		})
		next, err := parser.ParseIncrementalStrict([]byte(edited), tree)
		if err != nil {
			t.Fatal(err)
		}
		tree.Release()
		tree = next
		fresh, err := gotreesitter.NewParser(lang).ParseStrict([]byte(edited))
		if err != nil {
			t.Fatal(err)
		}
		got, want := yamlRecoveredTreeShape(tree.RootNode(), lang), yamlRecoveredTreeShape(fresh.RootNode(), lang)
		fresh.Release()
		if got != want {
			t.Fatalf("incremental parse of %q differs from fresh:\ngot %s\nwant %s", edited, got, want)
		}
		source = edited
	}
	unchangedSource := []byte(source)
	allocs := testing.AllocsPerRun(10, func() {
		unchanged, err := parser.ParseIncremental(unchangedSource, tree)
		if err != nil || unchanged != tree {
			t.Fatalf("no-edit parse changed the tree: err=%v", err)
		}
	})
	if allocs != 0 {
		t.Fatalf("no-edit parse allocated %g times", allocs)
	}

}

func yamlRecoveredTreeShape(root *gotreesitter.Node, lang *gotreesitter.Language) string {
	var result strings.Builder
	var visit func(*gotreesitter.Node)
	visit = func(node *gotreesitter.Node) {
		fmt.Fprintf(&result, "(%s %d:%d error=%t missing=%t", node.Type(lang), node.StartByte(), node.EndByte(), node.HasError(), node.IsMissing())
		for i := 0; i < node.ChildCount(); i++ {
			visit(node.Child(i))
		}
		result.WriteByte(')')
	}
	visit(root)
	return result.String()
}
