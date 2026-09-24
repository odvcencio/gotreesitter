//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestParitySwiftConditionFieldFamily(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "issue-590-condition-list",
			source: "func f() {\n" +
				"  if let first = first, let second = second {\n" +
				"    return\n" +
				"  }\n" +
				"}\n",
		},
		{
			name: "issue-591-else-if-binding",
			source: "func f(_ i: Index, _ limit: Index) -> Index? {\n" +
				"  if (limit < i) {\n" +
				"    if let idx = make(i) {\n" +
				"      return idx\n" +
				"    } else {\n" +
				"      return nil\n" +
				"    }\n" +
				"  } else if let idx = make(i) {\n" +
				"    return idx\n" +
				"  } else {\n" +
				"    return nil\n" +
				"  }\n" +
				"}\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source)
			cLang, err := ParityCLanguage("swift")
			if err != nil {
				t.Fatalf("load locked Swift C parser: %v", err)
			}
			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatalf("set locked Swift C language: %v", err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked Swift C parser returned no tree")
			}
			t.Cleanup(cTree.Close)

			for _, route := range []struct {
				name      string
				candidate bool
			}{
				{name: "production", candidate: false},
				{name: "compact", candidate: true},
			} {
				route := route
				t.Run("fresh-"+route.name, func(t *testing.T) {
					candidate := route.candidate
					goTree, goLang, err := parseWithGo(parityCase{
						name:           "swift",
						source:         test.source,
						candidateRoute: &candidate,
					}, source, nil)
					if err != nil {
						t.Fatalf("parse Swift with Go: %v", err)
					}
					t.Cleanup(func() { releaseGoTree(goTree) })
					assertSwiftConditionTreeExact(t, goTree, goLang, cTree)
				})

				t.Run("incremental-"+route.name, func(t *testing.T) {
					oldSource := append(append([]byte(nil), source...), '\n')
					candidate := route.candidate
					oldTree, goLang, err := parseWithGo(parityCase{
						name:           "swift",
						source:         string(oldSource),
						candidateRoute: &candidate,
					}, oldSource, nil)
					if err != nil {
						t.Fatalf("parse old Swift tree with Go: %v", err)
					}
					oldTree.Edit(gotreesitter.InputEdit{
						StartByte:   uint32(len(source)),
						OldEndByte:  uint32(len(oldSource)),
						NewEndByte:  uint32(len(source)),
						StartPoint:  pointAtOffset(source, len(source)),
						OldEndPoint: pointAtOffset(oldSource, len(oldSource)),
						NewEndPoint: pointAtOffset(source, len(source)),
					})
					goTree, _, err := parseWithGo(parityCase{
						name:           "swift",
						source:         test.source,
						candidateRoute: &candidate,
					}, source, oldTree)
					releaseGoTree(oldTree)
					if err != nil {
						t.Fatalf("parse Swift incrementally with Go: %v", err)
					}
					t.Cleanup(func() { releaseGoTree(goTree) })
					assertSwiftConditionTreeExact(t, goTree, goLang, cTree)
				})
			}
		})
	}
}

func assertSwiftConditionTreeExact(t *testing.T, goTree *gotreesitter.Tree, goLang *gotreesitter.Language, cTree *sitter.Tree) {
	t.Helper()
	assertLockedCTreeExact(t, "Swift condition witness", goTree, goLang, cTree)
}

func assertLockedCTreeExact(t *testing.T, label string, goTree *gotreesitter.Tree, goLang *gotreesitter.Language, cTree *sitter.Tree) {
	t.Helper()
	goRoot := goTree.RootNode()
	cRoot := cTree.RootNode()
	if goRoot.HasError() || cRoot.HasError() {
		t.Fatalf("%s has an error node: Go=%v C=%v", label, goRoot.HasError(), cRoot.HasError())
	}
	assertLockedCTreeExactWithErrors(t, label, goTree, goLang, cTree)
}

func assertLockedCTreeExactWithErrors(t *testing.T, label string, goTree *gotreesitter.Tree, goLang *gotreesitter.Language, cTree *sitter.Tree) {
	t.Helper()
	goRoot := goTree.RootNode()
	cRoot := cTree.RootNode()
	if diff := FirstDivergenceDumpV1(goRoot, goLang, cRoot); diff != nil {
		t.Fatalf("%s node or field divergence: %+v", label, diff)
	}
	if diff := firstLockedCTreeFlagDivergence(goRoot, goLang, cRoot, "/"+goRoot.Type(goLang)); diff != nil {
		t.Fatal(diff)
	}
	goInspection, err := benchfixtures.InspectGoTree(goRoot, goLang)
	if err != nil {
		t.Fatalf("inspect Go deep tree: %v", err)
	}
	cDigest, err := COracleDeepDigest(cTree)
	if err != nil {
		t.Fatalf("inspect C deep tree: %v", err)
	}
	if goInspection.SHA256 != cDigest {
		t.Fatalf("deep digest Go=%s C=%s", goInspection.SHA256, cDigest)
	}
	t.Logf("%s: exact symbols, fields, spans, points, flags, and deep digest %s", label, goInspection.SHA256)
}

func firstLockedCTreeFlagDivergence(goNode *gotreesitter.Node, goLang *gotreesitter.Language, cNode *sitter.Node, path string) error {
	if goNode == nil || cNode == nil {
		return fmt.Errorf("%s: nil mismatch Go=%v C=%v", path, goNode == nil, cNode == nil)
	}
	if goNode.IsMissing() != cNode.IsMissing() {
		return fmt.Errorf("%s: missing Go=%v C=%v", path, goNode.IsMissing(), cNode.IsMissing())
	}
	if goNode.IsError() != cNode.IsError() {
		return fmt.Errorf("%s: error Go=%v C=%v", path, goNode.IsError(), cNode.IsError())
	}
	for i := 0; i < goNode.ChildCount(); i++ {
		goChild := goNode.Child(i)
		cChild := cNode.Child(uint(i))
		childPath := fmt.Sprintf("%s/%s[%d]", path, goChild.Type(goLang), i)
		if err := firstLockedCTreeFlagDivergence(goChild, goLang, cChild, childPath); err != nil {
			return err
		}
	}
	return nil
}
