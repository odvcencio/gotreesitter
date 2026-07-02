package gotreesitter_test

import (
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// countCSharpDecls walks the tree and counts class/method declaration nodes.
func countCSharpDecls(t *testing.T, lang *gotreesitter.Language, root *gotreesitter.Node) (classes, methods int) {
	t.Helper()
	var walk func(n *gotreesitter.Node)
	walk = func(n *gotreesitter.Node) {
		if n == nil {
			return
		}
		switch n.Type(lang) {
		case "class_declaration":
			classes++
		case "method_declaration":
			methods++
		}
		for i := 0; i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	return classes, methods
}

// TestCSharpNamespaceWithInternalErrorRecoversDeclarations is the regression
// test for issue #115: a namespace whose body has an internal parse error used
// to collapse the whole file into a single ERROR node with zero recoverable
// declarations. Recovery should now surface the namespace's type declarations.
func TestCSharpNamespaceWithInternalErrorRecoversDeclarations(t *testing.T) {
	lang := grammars.CSharpLanguage()

	// A namespace containing a class with a deliberately broken member among
	// well-formed ones. The braces are balanced so the namespace span resolves,
	// but the body does not parse cleanly end-to-end.
	src := []byte(`namespace N
{
    public class C
    {
        public void A() {}
        public int @@@ Broken {}
        public int B() { return 0; }
    }
}`)

	tree, err := gotreesitter.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	classes, methods := countCSharpDecls(t, lang, root)
	if classes < 1 {
		t.Fatalf("recovered classes = %d, want >= 1 (root type %s)", classes, root.Type(lang))
	}
	if methods < 1 {
		t.Fatalf("recovered methods = %d, want >= 1 (root type %s)", methods, root.Type(lang))
	}
}

// TestCSharpNamespaceWithCharLiteralBracesRecovers exercises the trivia-aware
// brace matcher: char-literal braces ('{' / '}') in a member body must not
// truncate the namespace span during recovery.
func TestCSharpNamespaceWithCharLiteralBracesRecovers(t *testing.T) {
	lang := grammars.CSharpLanguage()

	src := []byte(`namespace N
{
    public class C
    {
        char Open = '{';
        char Close = '}';
        public int @@@ Broken {}
        public bool IsBrace(char c) { return c == '{' || c == '}'; }
    }
}`)

	tree, err := gotreesitter.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if got, want := root.EndByte(), uint32(len(src)); got != want {
		t.Fatalf("root end = %d, want %d (span truncated by char-literal braces)", got, want)
	}
	classes, _ := countCSharpDecls(t, lang, root)
	if classes < 1 {
		t.Fatalf("recovered classes = %d, want >= 1 (root type %s)", classes, root.Type(lang))
	}
}

// TestCSharpNamespaceWithVerbatimStringBracesRecovers ensures a verbatim string
// (@"...") containing braces and "" escapes does not throw off the trivia-aware
// brace matcher during recovery: the braces inside the string must be ignored so
// the namespace/class span is not truncated.
func TestCSharpNamespaceWithVerbatimStringBracesRecovers(t *testing.T) {
	lang := grammars.CSharpLanguage()

	src := []byte(`namespace N
{
    public class C
    {
        string S = @"obj }} with ""q"" and { brace";
        public int @@@ Broken {}
        public int B() { return 0; }
    }
}`)

	tree, err := gotreesitter.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if got, want := root.EndByte(), uint32(len(src)); got != want {
		t.Fatalf("root end = %d, want %d (span truncated by verbatim-string braces)", got, want)
	}
	classes, methods := countCSharpDecls(t, lang, root)
	if classes < 1 {
		t.Fatalf("recovered classes = %d, want >= 1 (root type %s)", classes, root.Type(lang))
	}
	if methods < 1 {
		t.Fatalf("recovered methods = %d, want >= 1 (root type %s)", methods, root.Type(lang))
	}
}

// TestCSharpCleanNamespaceUnaffected guards against the recovery path altering a
// namespace that already parses cleanly.
func TestCSharpCleanNamespaceUnaffected(t *testing.T) {
	lang := grammars.CSharpLanguage()

	src := []byte(`namespace N
{
    public class C
    {
        public void A() {}
        public int B() { return 0; }
    }
    public class D
    {
        public void E() {}
    }
}`)

	tree, err := gotreesitter.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("clean namespace reported an error")
	}
	if got, want := root.Type(lang), "compilation_unit"; got != want {
		t.Fatalf("root type = %s, want %s", got, want)
	}
	classes, methods := countCSharpDecls(t, lang, root)
	if classes != 2 {
		t.Fatalf("classes = %d, want 2", classes)
	}
	if methods != 3 {
		t.Fatalf("methods = %d, want 3", methods)
	}
}

// TestCSharpLargeShreddedNamespaceRecoversMethods is the regression test for
// issue #136. A large real-world file (a trimmed Newtonsoft.Json JsonTextReader
// excerpt) whose class body is shredded by a cumulative GLR failure used to
// recover the namespace/class shell but zero method_declaration nodes, because
// the source-based method reconstruction was gated off above 4096 bytes. The
// per-member bounded source recovery should now surface the methods.
func TestCSharpLargeShreddedNamespaceRecoversMethods(t *testing.T) {
	if raceEnabled {
		// The per-member bounded recovery in csharpRecoverNamespaceBodyMembersFromSource
		// (parser_result_csharp_method_recovery.go) reparses each class member as its
		// own small GLR parse; on this ~12KB real-world fixture that's fast enough to
		// finish well inside the parser's SetTimeoutMicros budget normally (~1.1s), but
		// the race detector's per-access instrumentation slows the same work enough
		// (5s+) to trip the parser's own internal timeout, which is a wall-clock
		// checkpoint and therefore inherently race/CPU-speed sensitive. Skip under
		// -race; non-race coverage keeps the full recovery assertions. Mirrors
		// TestScalaPathResolverRecoversTopLevelObjectAndClass in
		// parser_result_test/parser_result_scala_realworld_test.go, which skips its
		// own heavyweight realworld recovery parse under -race for the same reason.
		t.Skip("skip heavyweight C# JsonTextReader realworld recovery parse under -race: race instrumentation overhead trips the parser's internal wall-clock timeout; non-race coverage keeps the full recovery assertions")
	}
	lang := grammars.CSharpLanguage()
	src, err := os.ReadFile("testdata/parser_result/csharp/jsontextreader_excerpt.cs")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if len(src) <= 4096 {
		t.Fatalf("fixture must exceed the 4096-byte gate to exercise #136, got %d bytes", len(src))
	}

	parser := gotreesitter.NewParser(lang)
	// Generous budget: the recovery is bounded per member and must finish well
	// within this, so the parse must not stop early.
	parser.SetTimeoutMicros(5_000_000)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()

	if tree.ParseStoppedEarly() {
		t.Fatalf("parse stopped early: %s", tree.ParseRuntime().Summary())
	}
	root := tree.RootNode()
	if got, want := root.EndByte(), uint32(len(src)); got != want {
		t.Fatalf("root end = %d, want %d (span not byte-faithful)", got, want)
	}
	classes, methods := countCSharpDecls(t, lang, root)
	if classes < 1 {
		t.Fatalf("class_declaration count = %d, want >= 1", classes)
	}
	if methods < 8 {
		t.Fatalf("method_declaration count = %d, want >= 8 (was 0 before #136)", methods)
	}
}
