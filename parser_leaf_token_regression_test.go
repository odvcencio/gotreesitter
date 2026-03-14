package gotreesitter_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func parseLanguageSample(t *testing.T, name, src string) (*gotreesitter.Tree, *gotreesitter.Language) {
	t.Helper()

	var entry grammars.LangEntry
	var report grammars.ParseSupport
	found := false
	for _, e := range grammars.AllLanguages() {
		if e.Name == name {
			entry = e
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("%s language entry not found", name)
	}
	found = false
	for _, r := range grammars.AuditParseSupport() {
		if r.Name == name {
			report = r
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("%s parse support entry not found", name)
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
		t.Fatalf("unsupported %s backend: %s", name, report.Backend)
	}
	if err != nil {
		t.Fatalf("%s parse failed: %v", name, err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatalf("%s parse returned nil tree/root", name)
	}
	if tree.RootNode().HasError() {
		t.Fatalf("%s parse has error: %s", name, tree.ParseRuntime().Summary())
	}
	return tree, lang
}

func TestParseAsmImmediateIntStaysInt(t *testing.T) {
	src := grammars.ParseSmokeSample("asm")
	tree, lang := parseLanguageSample(t, "asm", src)
	t.Cleanup(tree.Release)

	node := tree.RootNode().NamedDescendantForByteRange(19, 20)
	if node == nil {
		t.Fatal("missing named descendant for asm immediate")
	}
	if got, want := node.Type(lang), "int"; got != want {
		t.Fatalf("asm immediate type = %q, want %q", got, want)
	}
}

func TestParseFennelImmediateNumberStaysNumber(t *testing.T) {
	src := grammars.ParseSmokeSample("fennel")
	tree, lang := parseLanguageSample(t, "fennel", src)
	t.Cleanup(tree.Release)

	node := tree.RootNode().NamedDescendantForByteRange(8, 9)
	if node == nil {
		t.Fatal("missing named descendant for fennel number")
	}
	if got, want := node.Type(lang), "number"; got != want {
		t.Fatalf("fennel binding value type = %q, want %q", got, want)
	}
}

func TestParseForthBuiltinOperatorBeatsWord(t *testing.T) {
	src := grammars.ParseSmokeSample("forth")
	tree, lang := parseLanguageSample(t, "forth", src)
	t.Cleanup(tree.Release)

	node := tree.RootNode().NamedDescendantForByteRange(13, 14)
	if node == nil {
		t.Fatal("missing named descendant for forth operator")
	}
	if got, want := node.Type(lang), "operator"; got != want {
		t.Fatalf("forth operator type = %q, want %q", got, want)
	}
}

func TestParseMesonCommandArgumentPrefersVariableunit(t *testing.T) {
	src := grammars.ParseSmokeSample("meson")
	tree, lang := parseLanguageSample(t, "meson", src)
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if got, want := root.ChildCount(), 1; got != want {
		t.Fatalf("meson root child count = %d, want %d", got, want)
	}
	cmd := root.Child(0)
	if cmd == nil {
		t.Fatal("meson root child is nil")
	}
	if got, want := cmd.Type(lang), "normal_command"; got != want {
		t.Fatalf("meson root child type = %q, want %q", got, want)
	}
	arg := cmd.Child(2)
	if arg == nil {
		t.Fatal("meson command argument child is nil")
	}
	if got, want := arg.Type(lang), "variableunit"; got != want {
		t.Fatalf("meson command argument type = %q, want %q", got, want)
	}
}
