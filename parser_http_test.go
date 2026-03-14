package gotreesitter_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func parseHTTP(t *testing.T, src string) (*gotreesitter.Tree, *gotreesitter.Language) {
	t.Helper()

	var entry grammars.LangEntry
	var report grammars.ParseSupport
	found := false
	for _, e := range grammars.AllLanguages() {
		if e.Name == "http" {
			entry = e
			found = true
			break
		}
	}
	if !found {
		t.Fatal("http language entry not found")
	}
	found = false
	for _, r := range grammars.AuditParseSupport() {
		if r.Name == "http" {
			report = r
			found = true
			break
		}
	}
	if !found {
		t.Fatal("http parse support entry not found")
	}

	lang := entry.Language()
	parser := gotreesitter.NewParser(lang)
	srcBytes := []byte(src)
	var (
		tree *gotreesitter.Tree
		err  error
	)
	switch report.Backend {
	case grammars.ParseBackendTokenSource:
		tree, err = parser.ParseWithTokenSource(srcBytes, entry.TokenSourceFactory(srcBytes, lang))
	case grammars.ParseBackendDFA, grammars.ParseBackendDFAPartial:
		tree, err = parser.Parse(srcBytes)
	default:
		t.Fatalf("unsupported http backend: %s", report.Backend)
	}
	if err != nil {
		t.Fatalf("http parse failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil tree/root")
	}
	return tree, lang
}

func TestParseHTTPSmokeBuildsSectionRoot(t *testing.T) {
	src := grammars.ParseSmokeSample("http")
	tree, lang := parseHTTP(t, src)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if got, want := root.Type(lang), "document"; got != want {
		t.Fatalf("root type = %q, want %q", got, want)
	}
	if got, want := root.EndByte(), uint32(len(src)); got != want {
		t.Fatalf("root end = %d, want %d (%s)", got, want, tree.ParseRuntime().Summary())
	}
	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	child := root.Child(0)
	if child == nil {
		t.Fatal("root child is nil")
	}
	if got, want := child.Type(lang), "section"; got != want {
		t.Fatalf("root child type = %q, want %q", got, want)
	}
	if got, want := child.EndByte(), uint32(len(src)); got != want {
		t.Fatalf("section end = %d, want %d", got, want)
	}
	if root.HasError() {
		t.Fatalf("root has error: %s", tree.ParseRuntime().Summary())
	}
}
