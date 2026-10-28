package grammargen

import (
	"os"
	"testing"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// rustParityLanguages imports the Rust grammar.json, generates a language from
// it, and returns it with the shipped reference language. The test skips when
// grammar.json is unavailable.
func rustParityLanguages(t *testing.T) (genLang, refLang *gotreesitter.Language) {
	t.Helper()
	source, err := os.ReadFile(rustGrammarJSONPathForTest(t))
	if err != nil {
		t.Skipf("Rust grammar.json not available: %v", err)
	}
	gram, err := ImportGrammarJSON(source)
	if err != nil {
		t.Fatalf("import Rust grammar.json: %v", err)
	}
	genLang, err = generateWithTimeout(gram, 90*time.Second)
	if err != nil {
		t.Fatalf("generate Rust language: %v", err)
	}
	refLang = grammars.RustLanguage()
	adaptExternalScanner(refLang, genLang)
	return genLang, refLang
}

type rustParityCase struct {
	name string
	src  string
}

// runRustParityCases generates the Rust language once and checks each case for
// deep parity with the reference parser.
func runRustParityCases(t *testing.T, cases []rustParityCase) {
	t.Helper()
	genLang, refLang := rustParityLanguages(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertGeneratedAndReferenceDeepParity(t, genLang, refLang, tc.src)
		})
	}
}

func TestRustContinueBreakLabelParity(t *testing.T) {
	runRustParityCases(t, []rustParityCase{
		{name: "continue_label", src: "fn f() { loop { continue 'outer; } }\n"},
		{name: "break_label", src: "fn f() { loop { break 'outer; } }\n"},
		{name: "labeled_loop_continue", src: "fn f() { 'outer: loop { continue 'outer; } }\n"},
		{name: "labeled_loop_break", src: "fn f() { 'outer: loop { break 'outer; } }\n"},
	})
}

func TestRustClosureOrPatternParity(t *testing.T) {
	runRustParityCases(t, []rustParityCase{
		{name: "let_tuple_closure", src: "fn f() { let (|x| x) = y; }\n"},
		{name: "let_tuple_captured", src: "fn f() { let (|__@_|__) = y; }\n"},
		{name: "expr_tuple_captured", src: "fn f() { let val = ((|__@_|__)); }\n"},
		{name: "tuple_param_nested", src: "fn f() { let val = ((|(..):(_,_),(|__@_|__)|__)); }\n"},
	})
}

func TestRustDocCommentContentParity(t *testing.T) {
	implBlock := "impl fmt::Debug for Lifetime {\n" +
		"    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {\n" +
		"        write!(\n" +
		"            f,\n" +
		"            \"lifetime({}: {})\",\n" +
		"            self.id,\n" +
		"            pprust::lifetime_to_string(self)\n" +
		"        )\n" +
		"    }\n" +
		"}\n"
	structFull := "pub struct LifetimeDef {\n" +
		"    pub attrs: ThinVec<Attribute>,\n" +
		"    pub lifetime: Lifetime,\n" +
		"    pub bounds: Vec<Lifetime>,\n" +
		"}\n"

	runRustParityCases(t, []rustParityCase{
		{name: "plain", src: "/// A lifetime definition\npub struct X {}\n"},
		{name: "lifetime", src: "/// A lifetime definition, e.g. 'a: 'b+'c+'d\npub struct X {}\n"},
		{name: "backtick_lifetime", src: "/// A lifetime definition, e.g. `'a: 'b+'c+'d`\npub struct X {}\n"},
		{name: "inner", src: "//! Module docs with 'a: 'b+'c+'d\nmod x {}\n"},
		{name: "after_impl", src: implBlock + "\n/// A lifetime definition, e.g. `'a: 'b+'c+'d`\n" + structFull},
		{name: "after_impl_before_attribute", src: implBlock + "\n/// A lifetime definition, e.g. `'a: 'b+'c+'d`\n#[derive(Clone, PartialEq, Eq, RustcEncodable, RustcDecodable, Hash, Debug)]\n" + structFull},
	})
}

func TestRustDocCommentGLRNoLookaheadParity(t *testing.T) {
	genLang, refLang := rustParityLanguages(t)

	src, err := os.ReadFile("/tmp/grammar_parity/rust/examples/ast.rs")
	if err != nil {
		t.Skipf("Rust ast.rs not available: %v", err)
	}
	if len(src) < 1600 {
		t.Fatalf("Rust ast.rs fixture too short: %d", len(src))
	}
	src = src[:1600]

	genParser := gotreesitter.NewParser(genLang)
	refParser := gotreesitter.NewParser(refLang)
	genTree, _ := genParser.Parse(src)
	refTree, _ := refParser.Parse(src)
	defer genTree.Release()
	defer refTree.Release()

	genRoot := genTree.RootNode()
	refRoot := refTree.RootNode()
	if genRoot.EndByte() != refRoot.EndByte() ||
		genRoot.HasError() != refRoot.HasError() ||
		genRoot.ChildCount() != refRoot.ChildCount() ||
		genTree.ParseRuntime().TokensConsumed != refTree.ParseRuntime().TokensConsumed {
		t.Fatalf("generated Rust doc-comment prefix diverged: gen end=%d err=%t children=%d runtime=%s ref end=%d err=%t children=%d runtime=%s",
			genRoot.EndByte(), genRoot.HasError(), genRoot.ChildCount(), genTree.ParseRuntime().Summary(),
			refRoot.EndByte(), refRoot.HasError(), refRoot.ChildCount(), refTree.ParseRuntime().Summary())
	}
}
