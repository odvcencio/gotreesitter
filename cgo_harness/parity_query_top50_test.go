//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"strings"
	"testing"
	"unicode"

	sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/odvcencio/gotreesitter/grammars"
)

type generatedExactQuery struct {
	name  string
	query string
}

func TestParityQueryExactTop50Generated(t *testing.T) {
	parityRequireTop50(t, "TestParityQueryExactTop50Generated")
	for _, name := range top50ParityLanguages {
		if parityLanguageExcluded(name) {
			continue
		}
		report, ok := paritySupportForName(name)
		if !ok || report.Backend == grammars.ParseBackendUnsupported {
			continue
		}
		if !hasDedicatedSample[name] {
			continue
		}

		tc := parityCase{name: name, source: grammars.ParseSmokeSample(name)}
		t.Run(name, func(t *testing.T) {
			scheduleParityMemoryScavenge(t)
			if reason := paritySkipReason(tc.name); reason != "" {
				t.Skipf("known mismatch: %s", reason)
			}
			queries := generatedExactQueriesForTop50Smoke(t, tc)
			if len(queries) == 0 {
				t.Skip("no generated exact query cases")
			}
			for _, qc := range queries {
				qc := qc
				t.Run(qc.name, func(t *testing.T) {
					skipKnownCOracleDifference(t)
					assertExactQueryParity(t, exactQueryCase{
						name:   qc.name,
						lang:   tc.name,
						source: tc.source,
						query:  qc.query,
					})
				})
			}
		})
	}
}

func generatedExactQueriesForTop50Smoke(t *testing.T, tc parityCase) []generatedExactQuery {
	t.Helper()
	src := normalizedSource(tc.name, tc.source)
	cLang, err := ParityCLanguage(tc.name)
	if err != nil {
		if skipReason := parityReferenceSkipReason(err); skipReason != "" {
			t.Skipf("skip C reference parser: %s", skipReason)
		}
		t.Fatalf("load C parser: %v", err)
	}

	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		if skipReason := parityReferenceSkipReason(err); skipReason != "" {
			t.Skipf("skip C reference parser SetLanguage: %s", skipReason)
		}
		t.Fatalf("C SetLanguage: %v", err)
	}
	cTree := cParser.Parse(src, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("C parser returned nil tree")
	}
	defer cTree.Close()

	return generatedExactQueriesFromCTree(cTree.RootNode())
}

func generatedExactQueriesFromCTree(root *sitter.Node) []generatedExactQuery {
	if root == nil {
		return nil
	}

	var queries []generatedExactQuery
	add := func(name, query string) {
		for _, existing := range queries {
			if existing.query == query {
				return
			}
		}
		queries = append(queries, generatedExactQuery{name: name, query: query})
	}

	rootKind := root.Kind()
	if queryNodeTypeUsable(rootKind) {
		add("root_capture", fmt.Sprintf("(%s) @root", rootKind))
	}
	add("named_wildcard", "(_) @node")

	firstNamed := firstNamedDescendant(root)
	firstNamedKind := ""
	if firstNamed != nil && queryNodeTypeUsable(firstNamed.Kind()) {
		firstNamedKind = firstNamed.Kind()
		add("first_named_type", fmt.Sprintf("(%s) @node", firstNamedKind))
		add("first_named_plus", fmt.Sprintf("(%s)+ @node", firstNamedKind))
		if countCNodes(root) <= 120 {
			add("first_named_star", fmt.Sprintf("(%s)* @node", firstNamedKind))
		}
	}

	firstChild := firstNamedDirectChild(root)
	if firstChild != nil && queryNodeTypeUsable(rootKind) && queryNodeTypeUsable(firstChild.Kind()) {
		childKind := firstChild.Kind()
		add("root_child", fmt.Sprintf("(%s (%s) @child)", rootKind, childKind))
		add("root_child_plus", fmt.Sprintf("(%s (%s)+ @child)", rootKind, childKind))
		add("root_child_star", fmt.Sprintf("(%s (%s)* @child)", rootKind, childKind))
	}

	parent, runKind := firstAdjacentNamedSiblingRun(root)
	if parent != nil && queryNodeTypeUsable(parent.Kind()) && queryNodeTypeUsable(runKind) {
		add("adjacent_sibling_run_plus", fmt.Sprintf("(%s (%s)+ @run)", parent.Kind(), runKind))
		add("adjacent_sibling_run_star", fmt.Sprintf("(%s (%s)* @run)", parent.Kind(), runKind))
	}

	return queries
}

func queryNodeTypeUsable(kind string) bool {
	if kind == "" || kind == "ERROR" {
		return false
	}
	for i, r := range kind {
		switch {
		case i == 0 && !(unicode.IsLetter(r) || r == '_'):
			return false
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '_', r == '.', r == '-', r == '/':
			continue
		default:
			return false
		}
	}
	return !strings.HasPrefix(kind, "MISSING")
}

func firstNamedDescendant(node *sitter.Node) *sitter.Node {
	if node == nil {
		return nil
	}
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		if child.IsNamed() {
			return child
		}
		if found := firstNamedDescendant(child); found != nil {
			return found
		}
	}
	return nil
}

func firstNamedDirectChild(node *sitter.Node) *sitter.Node {
	if node == nil {
		return nil
	}
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child != nil && child.IsNamed() && queryNodeTypeUsable(child.Kind()) {
			return child
		}
	}
	return nil
}

func firstAdjacentNamedSiblingRun(root *sitter.Node) (*sitter.Node, string) {
	var visit func(*sitter.Node) (*sitter.Node, string)
	visit = func(node *sitter.Node) (*sitter.Node, string) {
		if node == nil {
			return nil, ""
		}
		prevKind := ""
		for i := uint(0); i < node.ChildCount(); i++ {
			child := node.Child(i)
			if child == nil {
				continue
			}
			if child.IsNamed() && queryNodeTypeUsable(child.Kind()) {
				if child.Kind() == prevKind {
					return node, child.Kind()
				}
				prevKind = child.Kind()
			}
		}
		for i := uint(0); i < node.ChildCount(); i++ {
			if parent, kind := visit(node.Child(i)); parent != nil {
				return parent, kind
			}
		}
		return nil, ""
	}
	return visit(root)
}

func countCNodes(root *sitter.Node) int {
	if root == nil {
		return 0
	}
	total := 1
	for i := uint(0); i < root.ChildCount(); i++ {
		total += countCNodes(root.Child(i))
	}
	return total
}
