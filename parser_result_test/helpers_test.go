package parserresult_test

import (
	"os"
	"path/filepath"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func parseByLanguageName(t *testing.T, name, src string) (*gotreesitter.Tree, *gotreesitter.Language) {
	t.Helper()

	var entry grammars.LangEntry
	found := false
	for _, e := range grammars.AllLanguages() {
		if e.Name == name {
			entry = e
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing language entry %q", name)
	}

	var backend grammars.ParseBackend
	found = false
	for _, report := range grammars.AuditParseSupport() {
		if report.Name == name {
			backend = report.Backend
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing parse support report %q", name)
	}

	lang := entry.Language()
	parser := gotreesitter.NewParser(lang)
	srcBytes := []byte(src)

	var (
		tree *gotreesitter.Tree
		err  error
	)
	switch backend {
	case grammars.ParseBackendTokenSource:
		if entry.TokenSourceFactory == nil {
			t.Fatalf("%s: token source backend without factory", name)
		}
		tree, err = parser.ParseWithTokenSource(srcBytes, entry.TokenSourceFactory(srcBytes, lang))
	case grammars.ParseBackendDFA, grammars.ParseBackendDFAPartial:
		tree, err = parser.Parse(srcBytes)
	default:
		t.Fatalf("%s: unsupported backend %q", name, backend)
	}
	if err != nil {
		t.Fatalf("%s parse failed: %v", name, err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatalf("%s parse returned nil tree/root", name)
	}
	return tree, lang
}

func readRealworldCorpusOrSkip(t *testing.T, path string) []byte {
	t.Helper()

	for _, candidate := range []string{path, filepath.Join("..", path)} {
		src, err := os.ReadFile(candidate)
		if err == nil {
			return src
		}
		if !os.IsNotExist(err) {
			t.Fatalf("read %s: %v", candidate, err)
		}
	}
	t.Skipf("real-world corpus fixture unavailable: %s", path)
	return nil
}

func firstNode(root *gotreesitter.Node, pred func(*gotreesitter.Node) bool) *gotreesitter.Node {
	if root == nil {
		return nil
	}
	if pred(root) {
		return root
	}
	for i := 0; i < root.ChildCount(); i++ {
		child := root.Child(i)
		if found := firstNode(child, pred); found != nil {
			return found
		}
	}
	return nil
}

func countNodes(root *gotreesitter.Node, pred func(*gotreesitter.Node) bool) int {
	if root == nil {
		return 0
	}
	total := 0
	if pred(root) {
		total++
	}
	for i := 0; i < root.ChildCount(); i++ {
		total += countNodes(root.Child(i), pred)
	}
	return total
}

func forEachNode(root *gotreesitter.Node, visit func(*gotreesitter.Node)) {
	if root == nil {
		return
	}
	visit(root)
	for i := 0; i < root.ChildCount(); i++ {
		forEachNode(root.Child(i), visit)
	}
}

func trailingNewlineBoundary(source []byte, start, end uint32) uint32 {
	if start >= end || int(end) > len(source) {
		return start
	}
	lastNewline := -1
	for i, b := range source[start:end] {
		switch b {
		case ' ', '\t', '\r':
		case '\n':
			lastNewline = i
		default:
			return start
		}
	}
	if lastNewline < 0 {
		return start
	}
	return start + uint32(lastNewline+1)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
