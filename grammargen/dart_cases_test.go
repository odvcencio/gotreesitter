package grammargen

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
)

// dartParityLanguages imports the Dart grammar, generates a language with
// generate, and returns it with the shipped reference language. The test skips
// when the Dart grammar source is unavailable.
func dartParityLanguages(t *testing.T, generate func(*Grammar, time.Duration) (*gotreesitter.Language, error), timeout time.Duration) (genLang, refLang *gotreesitter.Language) {
	t.Helper()
	spec, ok := importParityGrammarByName("dart")
	if !ok {
		t.Fatal("missing dart import parity grammar")
	}
	gram, err := importParityGrammarSource(spec)
	if err != nil {
		t.Skipf("Dart grammar source unavailable: %v", err)
	}
	genLang, err = generate(gram, timeout)
	if err != nil {
		t.Fatalf("generate Dart: %v", err)
	}
	refLang = spec.blobFunc()
	adaptExternalScanner(refLang, genLang)
	return genLang, refLang
}

func importParityGrammarByName(name string) (importParityGrammar, bool) {
	for _, spec := range importParityGrammars {
		if spec.name == name {
			return spec, true
		}
	}
	return importParityGrammar{}, false
}

// parseDartClean parses src and fails the test when the parse errors, has no
// root, or has an error node.
func parseDartClean(t *testing.T, lang *gotreesitter.Language, src []byte, label string) *gotreesitter.Tree {
	t.Helper()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("%s parse failed: %v", label, err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatalf("%s parse missing root node", label)
	}
	if root.HasError() {
		t.Fatalf("%s parse has error: %s", label, root.SExpr(lang))
	}
	return tree
}

// assertDartDeepParity parses src with both languages and fails on any deep
// tree divergence. what names the case in the failure message.
func assertDartDeepParity(t *testing.T, genLang, refLang *gotreesitter.Language, src []byte, what string) {
	t.Helper()
	genTree := parseDartClean(t, genLang, src, "generated")
	refTree := parseDartClean(t, refLang, src, "reference")
	divs := compareTreesDeep(genTree.RootNode(), genLang, refTree.RootNode(), refLang, "root", 20)
	if len(divs) > 0 {
		t.Fatalf("generated/reference Dart %s mismatch: %s\ngen=%s\nref=%s",
			what, divs[0], genTree.RootNode().SExpr(genLang), refTree.RootNode().SExpr(refLang))
	}
}

func TestGeneratedDartTypeCastPrecedenceMatchesReference(t *testing.T) {
	genLang, refLang := dartParityLanguages(t, generateDartParityLanguageWithTimeout, 2*time.Minute)
	src := []byte(`void main() {
  a < b['json'] as BigB;
  a == b as BigB;
  a && b as BigB;
  a as BigB || b as BigB;
  if (a['json'] as BigB < b as BigB) {}
  a as BigB | b as BigB;
}
`)
	assertDartDeepParity(t, genLang, refLang, src, "cast precedence")
}

func TestGeneratedDartSwitchObjectPatternTypeIdentifierMatchesReference(t *testing.T) {
	genLang, refLang := dartParityLanguages(t, generateDartParityLanguageWithTimeout, 2*time.Minute)
	src := []byte(`void f() {
  switch (m) {
    case C(:final double x):
      break;
  }
}
`)
	assertDartDeepParity(t, genLang, refLang, src, "switch object pattern")
}

func TestGeneratedDartForLoopRelationalConditionMatchesReference(t *testing.T) {
	genLang, refLang := dartParityLanguages(t, generateWithTimeout, 90*time.Second)
	src := []byte("void main() { for (var i = 0; i < 10; i++) {} }\n")
	parseDartClean(t, refLang, src, "reference")
	parseDartClean(t, genLang, src, "generated")
}

func TestGeneratedDartSequentialRelationalStatementsMatchReference(t *testing.T) {
	genLang, refLang := dartParityLanguages(t, generateWithTimeout, 90*time.Second)
	for _, tc := range []struct {
		name string
		src  []byte
	}{
		{name: "empty body", src: []byte("main() {}\n")},
		{name: "one statement", src: []byte("main() {\n  a > b;\n}\n")},
		{name: "two statements", src: []byte("main() {\n  a > b;\n  a < b;\n}\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parseDartClean(t, refLang, tc.src, "reference")
			parseDartClean(t, genLang, tc.src, "generated")
		})
	}
}

func TestDartH6GeneratedCollapsedLeafMetadataAndMaterialization(t *testing.T) {
	genLang, refLang := dartParityLanguages(t, generateWithTimeout, 2*time.Minute)
	if got, want := genLang.Name, "dart"; got != want {
		t.Fatalf("generated language name = %q, want %q", got, want)
	}
	assertAnonymousVisibleSymbol(t, genLang, "?")
	assertAnonymousVisibleSymbol(t, genLang, "null")

	src := []byte(`
class Parser {
  Tree parse(String program, {int? encoding}) {
    if (program == null) {
      return Tree();
    }
    return Tree();
  }
}
`)
	genTree := parseDartClean(t, genLang, src, "generated")
	refTree := parseDartClean(t, refLang, src, "reference")
	assertDartH6CollapsedLeafShape(t, genLang, genTree.RootNode(), "generated")
	assertDartH6CollapsedLeafShape(t, refLang, refTree.RootNode(), "reference")

	divs := compareTreesDeep(genTree.RootNode(), genLang, refTree.RootNode(), refLang, "root", 20)
	if len(divs) > 0 {
		t.Fatalf("generated/reference Dart mismatch after collapsed-leaf repair: %s\ngen=%s\nref=%s",
			divs[0], genTree.RootNode().SExpr(genLang), refTree.RootNode().SExpr(refLang))
	}
}

func TestDartH5EagerDefaultReduceParserProof(t *testing.T) {
	t.Setenv("GOT_EAGER_DEFAULT_REDUCE", "1")

	genLang, refLang := dartParityLanguages(t, generateWithTimeout, 2*time.Minute)

	sample12 := dartH5ParsePair(t, "sample12", genLang, refLang, dartH5Sample12Source())
	sample21 := dartH5ParsePair(t, "sample21", genLang, refLang, dartH5Sample21Source())
	t.Logf("eager-default-reduce runtime=%d sample12=%s sample21=%s", genLang.StateCount, sample12.summary(), sample21.summary())
	if !sample12.ok() {
		t.Fatalf("sample12 mismatch with eager default reduce: %s", sample12.summary())
	}
	if !sample21.ok() {
		t.Fatalf("sample21 mismatch with eager default reduce: %s", sample21.summary())
	}
}

func assertAnonymousVisibleSymbol(t *testing.T, lang *gotreesitter.Language, name string) {
	t.Helper()
	sym, ok := lang.SymbolByName(name)
	if !ok {
		t.Fatalf("SymbolByName(%q) missing", name)
	}
	if int(sym) >= len(lang.SymbolMetadata) {
		t.Fatalf("SymbolByName(%q) = %d outside metadata len %d", name, sym, len(lang.SymbolMetadata))
	}
	meta := lang.SymbolMetadata[sym]
	if meta.Name != name || !meta.Visible || meta.Named {
		t.Fatalf("SymbolByName(%q) metadata = {Name:%q Visible:%v Named:%v}, want anonymous visible",
			name, meta.Name, meta.Visible, meta.Named)
	}
}

func assertDartH6CollapsedLeafShape(t *testing.T, lang *gotreesitter.Language, root *gotreesitter.Node, label string) {
	t.Helper()
	var nullableNodes []*gotreesitter.Node
	var nullNodes []*gotreesitter.Node
	var walk func(*gotreesitter.Node)
	walk = func(n *gotreesitter.Node) {
		if n == nil {
			return
		}
		switch n.Type(lang) {
		case "nullable_type":
			nullableNodes = append(nullableNodes, n)
		case "null_literal":
			nullNodes = append(nullNodes, n)
		}
		for i := 0; i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	if got, want := len(nullableNodes), 1; got != want {
		t.Fatalf("%s nullable_type count = %d, want %d; tree=%s", label, got, want, root.SExpr(lang))
	}
	assertDartH6SingleAnonymousChild(t, lang, nullableNodes[0], "?", label)
	if got, want := len(nullNodes), 1; got != want {
		t.Fatalf("%s null_literal count = %d, want %d; tree=%s", label, got, want, root.SExpr(lang))
	}
	assertDartH6SingleAnonymousChild(t, lang, nullNodes[0], "null", label)
}

func assertDartH6SingleAnonymousChild(t *testing.T, lang *gotreesitter.Language, node *gotreesitter.Node, wantType, label string) {
	t.Helper()
	if got := node.ChildCount(); got != 1 {
		t.Fatalf("%s %s child count = %d, want 1; node=%s", label, node.Type(lang), got, node.SExpr(lang))
	}
	child := node.Child(0)
	if child == nil {
		t.Fatalf("%s %s missing child", label, node.Type(lang))
	}
	if child.Type(lang) != wantType || child.IsNamed() {
		t.Fatalf("%s %s child type/named = %q/%v, want %q/false; node=%s",
			label, node.Type(lang), child.Type(lang), child.IsNamed(), wantType, node.SExpr(lang))
	}
}

type dartH5PairResult struct {
	genType  string
	genStart uint32
	genEnd   uint32
	genErr   bool
	refType  string
	refStart uint32
	refEnd   uint32
	refErr   bool
	deep     int
}

func (r dartH5PairResult) ok() bool {
	return !r.genErr && !r.refErr && r.deep == 0 &&
		r.genType == r.refType &&
		r.genStart == r.refStart &&
		r.genEnd == r.refEnd
}

func (r dartH5PairResult) summary() string {
	return fmt.Sprintf("gen=%s[%d:%d]/err=%v ref=%s[%d:%d]/err=%v deep=%d",
		r.genType, r.genStart, r.genEnd, r.genErr, r.refType, r.refStart, r.refEnd, r.refErr, r.deep)
}

func dartH5ParsePair(t *testing.T, name string, genLang, refLang *gotreesitter.Language, src []byte) dartH5PairResult {
	t.Helper()
	genParser := gotreesitter.NewParser(genLang)
	if name == "sample12" && os.Getenv("DART_H5_TRACE") == "1" && os.Getenv("DART_H5_TRACE_VARIANT") == "eager-default-reduce" {
		genParser.SetGLRTrace(true)
	}
	refParser := gotreesitter.NewParser(refLang)
	genTree, err := genParser.Parse(src)
	if err != nil {
		t.Fatalf("%s generated parse: %v", name, err)
	}
	refTree, err := refParser.Parse(src)
	if err != nil {
		t.Fatalf("%s reference parse: %v", name, err)
	}
	genRoot, refRoot := genTree.RootNode(), refTree.RootNode()
	divs := compareTreesDeep(genRoot, genLang, refRoot, refLang, "root", 20)
	for i, div := range divs {
		t.Logf("%s div[%d]=%s", name, i, div.String())
	}
	return dartH5PairResult{
		genType:  genRoot.Type(genLang),
		genStart: genRoot.StartByte(),
		genEnd:   genRoot.EndByte(),
		genErr:   genRoot.HasError(),
		refType:  refRoot.Type(refLang),
		refStart: refRoot.StartByte(),
		refEnd:   refRoot.EndByte(),
		refErr:   refRoot.HasError(),
		deep:     len(divs),
	}
}

func dartH5Sample12Source() []byte {
	return []byte(`bool? _boolAttribute(
    String resourceId,
    String name,
    Map<String, Object?> attributes,
    String attributeName,
    ) {
  final Object? value = attributes[attributeName];
  if (value == null) {
    return null;
  }
  if (value != 'true' && value != 'false') {
    throw L10nException(
      'The "$attributeName" value of the "$name" placeholder in message $resourceId '
          'must be a boolean value.',
    );
  }
  return value == 'true';
}
`)
}

func dartH5Sample21Source() []byte {
	return []byte(`bool f() {
  assert(textColor != null
      && style != null
      && margin != null
      && _position != null
      && _position.isFinite
      && _opacity != null
      && _opacity >= 0.0
      && _opacity <= 1.0);
  return true;
}
`)
}
