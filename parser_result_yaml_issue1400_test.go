package gotreesitter_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestYAMLResultCompatibilityKeepsSyntaxErrors(t *testing.T) {
	lang := grammars.YamlLanguage()
	for _, name := range []string{"missing-colon", "indentation", "unclosed-flow"} {
		t.Run(name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join("testdata", "yaml_issue1400", name+".yaml"))
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join("testdata", "yaml_issue1400", name+".tree"))
			if err != nil {
				t.Fatal(err)
			}
			tree, err := gts.NewParser(lang).Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			root := tree.RootNode()
			if !root.HasError() || !yamlTreeContainsType(root, lang, "ERROR") {
				t.Fatalf("syntax error was hidden: %s", root.SExpr(lang))
			}
			for _, typ := range []string{"block_mapping_pair", "flow_node", "integer_scalar"} {
				if !yamlTreeContainsType(root, lang, typ) {
					t.Errorf("recovered %s nodes were dropped: %s", typ, root.SExpr(lang))
				}
			}
			if got := yamlIssue1400TreeSnapshot(root, lang); got != string(want) {
				t.Fatalf("tree differs from locked C:\ngot:\n%s\nwant:\n%s", got, want)
			}
			if tree.ParseStoppedEarly() {
				t.Fatalf("parse stopped early: %s", tree.ParseRuntime().Summary())
			}
		})
	}
}

func TestYAMLResultCompatibilityKeepsLexicalErrorBoundary(t *testing.T) {
	lang := grammars.YamlLanguage()
	for _, source := range []string{"root: a\\\nb\\\nc\"\n", "a: 1\nb\nc: 2\"", "\n,", "\n]", "\"\n"} {
		t.Run(source, func(t *testing.T) {
			tree, err := gts.NewParser(lang).Parse([]byte(source))
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			var firstError func(*gts.Node) *gts.Node
			firstError = func(node *gts.Node) *gts.Node {
				if node.IsError() || node.IsMissing() {
					return node
				}
				for i := 0; i < node.ChildCount(); i++ {
					if found := firstError(node.Child(i)); found != nil {
						return found
					}
				}
				return nil
			}
			first := firstError(tree.RootNode())
			if first == nil || first.StartPoint().Row != 0 || !tree.RootNode().HasError() {
				t.Fatalf("lost C's first-line error boundary: %s", tree.RootNode().SExpr(lang))
			}
			if tree.ParseStoppedEarly() || tree.RootNode().EndByte() != uint32(len(source)) {
				t.Fatalf("error root does not cover input: %s", tree.ParseRuntime().Summary())
			}
		})
	}
}

func yamlIssue1400TreeSnapshot(root *gts.Node, lang *gts.Language) string {
	var result strings.Builder
	var visit func(*gts.Node, int, string)
	visit = func(node *gts.Node, depth int, field string) {
		sp, ep := node.StartPoint(), node.EndPoint()
		fmt.Fprintf(&result, "%s%s named=%t extra=%t error=%t missing=%t has_error=%t bytes=%d:%d points=%d:%d-%d:%d field=%q\n", strings.Repeat("  ", depth), node.Type(lang), node.IsNamed(), node.IsExtra(), node.IsError(), node.IsMissing(), node.HasError(), node.StartByte(), node.EndByte(), sp.Row, sp.Column, ep.Row, ep.Column, field)
		for i := 0; i < node.ChildCount(); i++ {
			visit(node.Child(i), depth+1, node.FieldNameForChild(i, lang))
		}
	}
	visit(root, 0, "")
	return result.String()
}

func TestYAMLResultCompatibilityErrorEditsMatchFresh(t *testing.T) {
	lang := grammars.YamlLanguage()
	parser := gts.NewParser(lang)
	source := []byte("a: 1\nb: 2\nc: 2\n")
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { tree.Release() }()
	for _, edited := range []string{"a: 1\nb\nc: 2\n", "a: 1\n b: 2\n", "a: [1, 2\nb: 3\n", "a: [1, 2]\nb: 3\n"} {
		nextSource := []byte(edited)
		start := 0
		for start < len(source) && start < len(nextSource) && source[start] == nextSource[start] {
			start++
		}
		tree.Edit(gts.InputEdit{
			StartByte: uint32(start), OldEndByte: uint32(len(source)), NewEndByte: uint32(len(nextSource)),
			StartPoint: yamlIssue1400Point(source[:start]), OldEndPoint: yamlIssue1400Point(source), NewEndPoint: yamlIssue1400Point(nextSource),
		})
		next, err := parser.ParseIncremental(nextSource, tree)
		if err != nil {
			t.Fatal(err)
		}
		tree.Release()
		tree = next
		fresh, err := gts.NewParser(lang).Parse(nextSource)
		if err != nil {
			t.Fatal(err)
		}
		got, want := yamlIssue1400TreeSnapshot(tree.RootNode(), lang), yamlIssue1400TreeSnapshot(fresh.RootNode(), lang)
		fresh.Release()
		if got != want {
			t.Fatalf("incremental parse of %q differs from fresh:\ngot:\n%s\nwant:\n%s", edited, got, want)
		}
		source = nextSource
	}
	for index, step := range benchfixtures.EditingSession(source) {
		tree.Edit(step.Edit)
		next, err := parser.ParseIncremental(step.Source, tree)
		if err != nil {
			t.Fatal(err)
		}
		tree.Release()
		tree = next
		fresh, err := gts.NewParser(lang).Parse(step.Source)
		if err != nil {
			t.Fatal(err)
		}
		got, want := yamlIssue1400TreeSnapshot(tree.RootNode(), lang), yamlIssue1400TreeSnapshot(fresh.RootNode(), lang)
		fresh.Release()
		if got != want {
			t.Fatalf("session step %d differs from fresh:\ngot:\n%s\nwant:\n%s", index, got, want)
		}
		if tree.RootNode().IsError() && !tree.RootNode().HasError() {
			t.Fatalf("session step %d: ERROR root reports HasError false", index)
		}
		source = step.Source
	}
	if allocs := testing.AllocsPerRun(10, func() {
		unchanged, err := parser.ParseIncremental(source, tree)
		if err != nil || unchanged != tree {
			t.Fatalf("no-edit parse changed the tree: %v", err)
		}
	}); allocs != 0 {
		t.Fatalf("no-edit parse allocated %g times", allocs)
	}
}

func yamlIssue1400Point(source []byte) gts.Point {
	var point gts.Point
	for _, b := range source {
		if b == '\n' {
			point.Row++
			point.Column = 0
		} else {
			point.Column++
		}
	}
	return point
}
