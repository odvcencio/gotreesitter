//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestIssue1274DjangoCorpus reports exact C parity after the documented
// Python module wrapper normalization. Set GTS_ISSUE1274_DJANGO_CORPUS to
// the Django checkout at 951d13c before running this diagnostic test.
func TestIssue1274DjangoCorpus(t *testing.T) {
	root := os.Getenv("GTS_ISSUE1274_DJANGO_CORPUS")
	if root == "" {
		t.Skip("set GTS_ISSUE1274_DJANGO_CORPUS to the Django checkout")
	}
	language := grammars.PythonLanguage()
	goParser := gotreesitter.NewParser(language)
	cLanguage, err := ParityCLanguage("python")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}

	var total, differing, parseFailures int
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".py") {
			return nil
		}
		total++
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		cTree := cParser.Parse(source, nil)
		if cTree == nil || cTree.RootNode() == nil {
			parseFailures++
			return fmt.Errorf("C parse returned no tree for %s", path)
		}
		goTree, err := goParser.Parse(source)
		if err != nil || goTree == nil || goTree.RootNode() == nil {
			cTree.Close()
			parseFailures++
			return fmt.Errorf("Go parse failed for %s: %v", path, err)
		}
		diff := issue1274PythonModuleDivergence(goTree.RootNode(), language, cTree.RootNode())
		if diff != nil {
			differing++
			if differing <= 15 {
				t.Logf("different %s: %+v", path, diff)
			}
		}
		goTree.Release()
		cTree.Close()
		if total%32 == 0 {
			runtime.GC()
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Django Python C parity: differing=%d total=%d parse_failures=%d", differing, total, parseFailures)
}

func issue1274PythonModuleDivergence(goRoot *gotreesitter.Node, language *gotreesitter.Language, cRoot *sitter.Node) *DumpV1Divergence {
	goStart, goEnd := goRoot.StartPoint(), goRoot.EndPoint()
	cStart, cEnd := cRoot.StartPosition(), cRoot.EndPosition()
	if goRoot.Type(language) != "module" || cRoot.Kind() != "module" ||
		goRoot.IsNamed() != cRoot.IsNamed() ||
		goRoot.IsExtra() != cRoot.IsExtra() ||
		goRoot.IsMissing() != cRoot.IsMissing() ||
		goRoot.StartByte() != uint32(cRoot.StartByte()) ||
		goRoot.EndByte() != uint32(cRoot.EndByte()) ||
		goStart.Row != uint32(cStart.Row) || goStart.Column != uint32(cStart.Column) ||
		goEnd.Row != uint32(cEnd.Row) || goEnd.Column != uint32(cEnd.Column) ||
		goRoot.HasError() != cRoot.HasError() ||
		goRoot.ChildCount() != int(cRoot.ChildCount()) {
		return FirstDivergenceDumpV1(goRoot, language, cRoot)
	}
	for i := 0; i < goRoot.ChildCount(); i++ {
		goField := goRoot.FieldNameForChild(i, language)
		cField := cRoot.FieldNameForChild(uint32(i))
		if goField != cField {
			return &DumpV1Divergence{Path: "/module", Category: "field", GoValue: goField, CValue: cField}
		}
		cChild := cRoot.Child(uint(i))
		for cChild != nil && cChild.ChildCount() == 1 &&
			(cChild.Kind() == "_simple_statements" || cChild.Kind() == "expression_statement" ||
				cChild.Kind() == "expression" || cChild.Kind() == "primary_expression") {
			child := cChild.Child(0)
			if child == nil || !child.IsNamed() {
				break
			}
			cChild = child
		}
		if diff := FirstDivergenceDumpV1(goRoot.Child(i), language, cChild); diff != nil {
			return diff
		}
	}
	return nil
}
