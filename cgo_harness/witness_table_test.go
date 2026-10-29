//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"cmp"
	"os"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// witness is one minimal source that is locked against the C reference.
// An empty name means the source names itself. lang overrides the set's
// language for sets that mix grammars.
type witness struct{ name, source, lang string }

// witnessSet is a list of witnesses for one language. Each witness runs
// through runParityCase as a subtest.
type witnessSet struct {
	name     string // subtest that wraps the set; empty runs the rows directly
	lang     string
	label    string // runParityCase label; empty uses the witness name
	routes   bool   // run each witness with the candidate route off, then on
	fixture  bool   // witness source is a path to read, not source text
	scavenge bool   // release memory after each witness
	setup    func(*testing.T)
	rows     []witness
}

func w(name, source string) witness { return witness{name: name, source: source} }

// srcs makes witnesses that take their names from their sources.
func srcs(sources ...string) []witness {
	rows := make([]witness, len(sources))
	for i, source := range sources {
		rows[i] = witness{source: source}
	}
	return rows
}

func runWitnessSet(t *testing.T, s witnessSet) {
	t.Helper()
	if s.name != "" {
		name := s.name
		s.name = ""
		t.Run(name, func(t *testing.T) { runWitnessSet(t, s) })
		return
	}
	if s.setup != nil {
		s.setup(t)
	}
	for _, row := range s.rows {
		name := cmp.Or(row.name, row.source)
		label := cmp.Or(s.label, name)
		lang := cmp.Or(row.lang, s.lang)
		t.Run(name, func(t *testing.T) {
			skipKnownCOracleDifference(t)
			if s.scavenge {
				scheduleParityMemoryScavenge(t)
			}
			src := []byte(row.source)
			if s.fixture {
				var err error
				if src, err = os.ReadFile(row.source); err != nil {
					t.Fatalf("read %s witness: %v", lang, err)
				}
			}
			if !s.routes {
				runParityCase(t, parityCase{name: lang}, label, src)
				return
			}
			for _, candidate := range []bool{false, true} {
				route := "default"
				if candidate {
					route = "candidate"
				}
				t.Run(route, func(t *testing.T) {
					runParityCase(t, parityCase{name: lang, candidateRoute: &candidate}, label, src)
				})
			}
		})
	}
}

// recoveryStackVersionOrder turns on the recovery stack version order for one
// language for the test, and optionally selects the matching GOT_C_RECOVERY mode.
func recoveryStackVersionOrder(language string, recoveryEnv bool) func(*testing.T) {
	return func(t *testing.T) {
		lang := grammars.DetectLanguageByName(language).Language()
		previous := lang.RecoveryStackVersionOrderEnabled
		lang.RecoveryStackVersionOrderEnabled = true
		t.Cleanup(func() { lang.RecoveryStackVersionOrderEnabled = previous })
		if recoveryEnv {
			t.Setenv("GOT_C_RECOVERY", language)
			gotreesitter.ResetParseEnvConfigCacheForTests()
			t.Cleanup(gotreesitter.ResetParseEnvConfigCacheForTests)
		}
	}
}

// lockedCWitnessSets holds the witnesses that no workflow selects by name.
var lockedCWitnessSets = []witnessSet{
	{
		name: "CInitializerAfterTransientRecovery", lang: "c", label: "initializer-after-transient-recovery",
		rows: []witness{w("table", "static const element_t table[] = {\n"+strings.Repeat("  {{ 0x1, 0x2, 0x3, 0x4, }},\n", 40)+"};\n")},
	},
	// These witnesses were reduced from cumulative edits of Git's ctype.c.
	{
		name: "CEditSessionFreshRecoveryWitness", lang: "c", routes: true, setup: recoveryStackVersionOrder("c", false),
		rows: []witness{
			w("enumerator-recovery-order", "enum{,G//\nL E}"),
			w("initializer-action-transaction", "r s={A,A,A,x G,U,"),
			w("closed-error-packed-links", "r s\tA,"),
			w("recovery-competitor-position", "X=GITT_CNTRL x|"),
			w("paused-keyword-lookahead", "L\tt for]"),
			w("recovered-delete-missing-semicolon-cost", `"",`),
			w("error-mode-keyword-promotion", "}if>n;"),
			w("error-mode-keyword-at-eof", "}if"),
		},
	},
	{
		name: "CppEditSessionFreshRecoveryWitness", lang: "cpp", routes: true, setup: recoveryStackVersionOrder("cpp", true),
		rows: []witness{
			w("literal-suffix-relex", `""t`),
			w("hidden-missing-newline-cost", ".#i"),
			w("eof-missing-token-trial", ",;e"),
			w("pause-progress-baseline", ", ;\n se"),
			w("packed-eof-error-ownership", "class D{d e({}}\nd A::b("),
			w("packed-eof-error-ownership-minimal", "class{d{}}d A::b("),
		},
	},
	{
		name: "ParityCaddyRecoveredStringLiteralRepetition", lang: "caddy", label: "recovered-string-literal-repetition", fixture: true,
		rows: []witness{w("security-header", "../testdata/caddy/security-header.Caddyfile")},
	},
	{
		name: "CollapsedTokenRetirementCOracle", label: "collapsed-token-retirement", scavenge: true,
		rows: []witness{
			{"hcl_wrappers", "a {\n b = true\n c = [false]\n d = {}\n}\n", "hcl"},
			{"cpon_null", "[null,true,false]\n", "cpon"},
			{"c_sharp_wrappers", "public class C { public bool F = false; void M() { var x = global::System.String.Empty; } }\n", "c_sharp"},
			{"powershell_assignment_operators", "$a = 1\n$a += 2\n", "powershell"},
		},
	},
	{
		name: "CSharp", lang: "c_sharp",
		rows: []witness{
			w("invocation-statement-vs-local-declaration", "class C { void F(){ newLines.Add(line); } }\n"),
			w("read-to-end-member-access", `using System.Diagnostics;

var filePath = "";

string GetOutput()
{
    var process = new Process
    {
        StartInfo = new ProcessStartInfo
        {
            Arguments = $"test --filter skip-all-corpus-tests",

        }
    };
    var output = process.StandardOutput.ReadToEnd();
    process.WaitForExit();
    return output;
}
`),
			w("switch-tuple-case", "class C { void M(){ switch (a, a) { case (1, 1): break; } } }\n"),
			w("interpolated-string-argument", "class C { void M(){ newLines.Add($\"{leadingWhitespaces}// <- {variable}\"); } }\n"),
			w("nested-member-access-qualified-left", "class C { void M(){ if (match.Success && match.Groups.Count == 3) { } } }\n"),
			w("implicit-object-creation-argument", `class C { void M(System.Action<object, string> onUpdate){ onUpdate(new(null, "Deployment failed")); } }
`),
			w("is-pattern-conditional-expression", `class C { string M(string outputDir, string jsonPath, object context){ var path = outputDir is null ? PathHelper.GetBicepparamOutputPath(jsonPath) : FileHelper.GetResultFilePath(context, outputDir); return path; } }
`),
			w("generic-base-list", `public class CliResultAssertions : ReferenceTypeAssertions<CliResult, CliResultAssertions>
{
}
`),
		},
	},
	{
		name: "FSharpUnaryWrapperRetirement", lang: "fsharp", label: "unary-wrapper-retirement",
		rows: []witness{w("open-then-let", "open Namespace.Module\nlet simple = Alpha\n")},
	},
	{
		name: "ObjcNativeAlternativeSelection", lang: "objc", label: "objc-native-selection",
		rows: []witness{
			w("encode_type", `void f(id coder) { [coder encodeValueOfObjCType: @encode(NSUInteger) at: 0]; }
`),
			w("function_pointers", `void f() {
  NSComparisonResult (*imp)(id, SEL, id);
  NSComparisonResult (*comp)(id, SEL, id) = 0;
}
`),
			w("at_string", `int main() { NSLog(@"one"); }
`),
			w("at_concatenated_string", `int main() { NSLog(@"one" @"two"); }
`),
		},
	},
	{
		name: "OCamlCommentEOF", lang: "ocaml", label: "comment-eof", routes: true,
		rows: srcs(
			"(**)", "(**)\n", "(**)\t", " (* outer (* inner *) *) \n",
			"(**) (**)\n", "(**)\n(**)\n", "\n(**)\n",
			"let x = 1\n(**)\n", "(* a *)\nlet x = 1\n(* b *)\n",
		),
	},
	{
		name: "ParityJavaScriptStandaloneBlockBeforeSimpleAssignment", lang: "javascript",
		rows: []witness{w("issue111", "{a}b=c")},
	},
	{
		name: "ParitySwiftAdjacentGenericClosers", lang: "swift",
		rows: []witness{
			w("nested-generic-call", "let v = X<Y<Z>>()\n"),
			w("right-shift", "let v = x >> y\n"),
		},
	},
	{
		name: "ParitySwiftOptionalBindingActionSelection", lang: "swift",
		rows: []witness{
			w("if-let", "if let x = y {\n}\n"),
			w("if-var", "if var x = y {\n}\n"),
			w("trailing-closure", "let value = transform(y) { item in item }\n"),
		},
	},
	{
		name: "PerlDeclaredShift", lang: "perl", label: "declared-shift", routes: true,
		rows: srcs(
			"{l$s, }", "{f$s, }", "{l$s,$t}", "{l$s,$t, }",
			"{(p())if@;}", "{(f())if@a;}", "{(f($x))if@a;}", "{(f(1,2))if@a;}", "{(p())unless@a;}",
			"push @found, $_;\n", "push @found, $a, $b;\n",
			"{a(())if@_}",
			"opmask_add(invert_opset opset(@_)) if @_;\n",
			"opmask_add(opset(@_)) if @_;\n",
		),
	},
	// This is the C-oracle twin for the languages.lock bump from da65ba3acc93 to
	// e7bd348c49fd (airbus-cert/tree-sitter-powershell@1ffcd63, "Handle simple
	// quote handler"). It compares a command name that continues straight into a
	// single-quoted run. TestPowerShellCommandNameImmediateSingleQuoteParsesCleanly
	// (root package) pins the same construct on the host side.
	{
		name: "PowerShellCommandNameImmediateSingleQuote", lang: "powershell", label: "command-name-immediate-quote",
		rows: []witness{w("single-quote-run", "foo'bar' baz\n")},
	},
	{
		name: "TerminalLeafNative", lang: "go", label: "terminal-leaf-native",
		rows: []witness{w("generic-constraint", `package p

type Number interface {
	~int | ~string
}

func F[T Number](value T) {
	_ = value
	_ = true
}
`)},
	},
	{
		name: "CleanTrailingExtraRoot", label: "clean-trailing-extra-root",
		rows: []witness{
			{"rst", "Title\n=====\n\nbody\n", "rst"},
			{"comment", "hello\n", "comment"},
		},
	},
}

func TestLockedCWitnesses(t *testing.T) {
	for _, s := range lockedCWitnessSets {
		runWitnessSet(t, s)
	}
}

// TestAdmissionCandidateDirectCOracle routes each fixture through the
// admission candidate once and requires zero fallbacks.
func TestAdmissionCandidateDirectCOracle(t *testing.T) {
	for _, tc := range []struct{ lang, fixture string }{
		{"graphql", "graphql_root_fields.graphql"},
		{"svelte", "svelte_button.svelte"},
	} {
		t.Run(tc.lang, func(t *testing.T) {
			gotreesitter.ResetAdmissionCandidateCounters()
			source, err := os.ReadFile("../testdata/admission_direct/" + tc.fixture)
			if err != nil {
				t.Fatalf("read %s admission fixture: %v", tc.lang, err)
			}

			candidateRoute := true
			beforeRouted, beforeFallback := gotreesitter.AdmissionCandidateCounters()
			runParityCase(t, parityCase{
				name:           tc.lang,
				source:         string(source),
				candidateRoute: &candidateRoute,
			}, "admission-direct", source)
			afterRouted, afterFallback := gotreesitter.AdmissionCandidateCounters()

			routed := afterRouted - beforeRouted
			fallback := afterFallback - beforeFallback
			if routed != 1 || fallback != 0 {
				t.Fatalf(
					"candidate route counters = %d/%d, want 1/0; reason=%s",
					routed,
					fallback,
					gotreesitter.AdmissionCandidateLastFallbackReason(),
				)
			}
		})
	}
}

// The tests below keep their names because workflows or runtime profile
// comments select them.

func TestCExternCWrapperParity(t *testing.T) {
	runWitnessSet(t, witnessSet{lang: "c", rows: []witness{
		w("extern-c-wrapper", "#ifdef __cplusplus\nextern \"C\" {\n#endif\n\nint x;\n\n#ifdef __cplusplus\n}\n#endif\n"),
	}})
}

func TestIssue667CEnumListsMatchCReference(t *testing.T) {
	runWitnessSet(t, witnessSet{lang: "c", rows: []witness{
		w("issue667-one", "enum E { A };\n"),
		w("issue667-two", "enum E { A, B };\n"),
		w("issue667-three", "enum E { A, B, C };\n"),
		w("issue667-four", "enum E { A, B, C, D };\n"),
		w("issue667-five", "enum E { A, B, C, D, E };\n"),
		w("issue667-trailing comma", "enum E { A, B, C, };\n"),
		w("issue667-explicit values", "enum E { A = 1, B = 2, C = 3 };\n"),
		w("issue667-typedef", "typedef enum { RED, GREEN, BLUE } Colour;\n"),
		w("issue667-comment before close", "enum E { A, B, C /* close */\n};\n"),
		w("issue667-neighboring declarations", "enum First { A, B, C };\nenum Second { D, E, F };\n"),
	}})
}

func TestCommonLispReadTimeFieldLockedCParity(t *testing.T) {
	runWitnessSet(t, witnessSet{
		lang: "commonlisp", label: "read-time-field", routes: true,
		rows: srcs("(#.())", "(#.()()(((()))))", "(#.(a)b)", "(#.(+ 1 2) other)", "('a `b ,c ,@d)", "(()()(((()))))"),
	})
}

func TestCppTemplateTypeParameterParity(t *testing.T) {
	runWitnessSet(t, witnessSet{lang: "cpp", rows: []witness{
		w("template-type-parameter", "void f(Vector<Rule> *elements) {}\n"),
	}})
}

func TestCppCollapsedKeywordCompatibilityParity(t *testing.T) {
	runWitnessSet(t, witnessSet{lang: "cpp", rows: []witness{
		w("collapsed-keyword-compatibility", `namespace tree_sitter {
namespace rules {
struct Rule {
  Rule(Rule &&other) noexcept;
  bool operator==(const Rule &other) const;
};
Rule::Rule(Rule &&other) noexcept {}
bool Rule::operator==(const Rule &other) const { return true; }
void f() {
  match([&](auto rule) { return; }, [=](auto rule) { return; });
}
}
}
`),
	}})
}

func TestCppMalformedClassFunctionDefinitionRecoveryParity(t *testing.T) {
	runWitnessSet(t, witnessSet{lang: "cpp", rows: []witness{
		w("malformed-class-function-definition-recovery", `int main() {
  a<T>();
  // <- function

  a::b();
  // ^ function

  a::b<C, D>();
  // ^ function

  this->b<C, D>();
  //    ^ function

  auto x = y;
  // <- type

  vector<T> a;
  // <- type

  std::vector<T> a;
  //   ^ type
}

class C : D{
  A();
  // <- function

  void efg() {
    // ^ function
  }
}

void A::b() {
  //    ^ function
}
`),
	}})
}

func TestCrystalBraceOpenerLockedCParity(t *testing.T) {
	runWitnessSet(t, witnessSet{lang: "crystal", rows: []witness{
		w("inline_hash", "x = {1 => 2}\n"),
		w("multiline_hash", "x = {\n  1 => 2,\n}\n"),
		w("inline_named_tuple", "x = {a: 1}\n"),
		w("multiline_named_tuple", "x = {\n  a: 1,\n}\n"),
	}})
}

// TestElixirExtrasAndPairsRetirementLockedCParity locks the C-equivalent
// shape for the two Elixir constructs the retired result-compatibility arm
// used to repair after the fact: the external scanner's hidden
// `_newline_before_comment` synchronization token never surfacing as a tree
// node, and map keyword-pair/update-operator grouping. See
// grammars/elixir_extras_and_pairs_native_regression_test.go for the
// compatibility-free and four-route receipts.
func TestElixirExtrasAndPairsRetirementLockedCParity(t *testing.T) {
	runWitnessSet(t, witnessSet{lang: "elixir", label: "extras-and-pairs-retirement", rows: []witness{
		w("root_comments_separated_by_newline", "# one\n# two\n\n[]\n"),
		w("do_block_comment_before_keyword", "fun x\n# comment\ndo\n  x\nend\n"),
		w("map_consecutive_keyword_pairs", "%{a: 1, b: 2}\n"),
		w("map_keyword_pairs_split_by_comment", "%{\n  a: 1,\n  # comment\n  b: 2\n}\n"),
		w("map_update_syntax_wraps_binary_operator", "%{map | name: \"Silly\"}\n"),
		w("map_arrow_entries_wrap_binary_operator", "%{1 => 2, 3 => 4}\n"),
		w("map_mixed_arrow_and_keyword_entries", "%{\"a\" => 1, b: 2, c: 3}\n"),
	}})
}

// TestEnforceConstParamRetirementLockedCParity locks the C-equivalent shape
// for the Enforce construct the retired result-compatibility arm used to
// repair after the fact: a `const int` formal parameter, with or without a
// default value, parsing as `(formal_parameter_modifier) (type_int)
// (identifier) ...` rather than a misread type/name pair that drops the
// parameter's own name. See
// grammars/enforce_const_param_native_regression_test.go for the
// compatibility-free and four-route receipts.
func TestEnforceConstParamRetirementLockedCParity(t *testing.T) {
	runWitnessSet(t, witnessSet{lang: "enforce", label: "const-param-retirement", rows: []witness{
		w("const_int_default_value", "class A { void m(const int x = 69) {} }\n"),
		w("const_int_no_default", "class A { void m(const int x) {} }\n"),
		w("const_int_default_among_other_modifiers", "override void foo(const int x = 69, inout int y, out int y) {}\n"),
		w("const_int_default_expression", "void foo(const int x = 1 + 2) {}\n"),
		w("const_int_forward_declaration", "void foo(const int x = 69);\n"),
	}})
}

func TestHurlExpectedRootFallbackLockedCParity(t *testing.T) {
	runWitnessSet(t, witnessSet{lang: "hurl", rows: []witness{
		w("recovered_entry", "GET http://localhost:8000/hello\n\nxxx"),
		w("file_delimiter", "POST http://localhost:8000/data\nfile,"),
		w("clean", "GET http://localhost:8000/hello"),
	}})
}

func TestIniExpectedRootFallbackLockedCParity(t *testing.T) {
	runWitnessSet(t, witnessSet{lang: "ini", rows: []witness{
		w("malformed_files", "[mypy]\nfiles =\n    one.py\n    two.py\nstrict = true\n"),
		w("error_code_continuation", "[mypy]\nenable_error_code = \n    ignore-without-code,\n    unused-ignore\n"),
		w("clean", "[mypy]\nstrict = true\n"),
	}})
}

// TestFIDLVersionedLayoutModifiersLockedCParity checks the same
// versioned-layout-modifier witnesses grammars/fidl_versioned_layout_modifier_native_regression_test.go
// locks against a native C tree-sitter reference parser, node by node.
func TestFIDLVersionedLayoutModifiersLockedCParity(t *testing.T) {
	runWitnessSet(t, witnessSet{lang: "fidl", rows: []witness{
		w("two_modifiers_with_args", "library test;\ntype Color = strict(removed=2) flexible(added=2) enum {\n    RED = 1;\n};"),
		w("single_modifier_with_args", "library test;\ntype Color = strict(removed=2) enum {\n    RED = 1;\n};"),
		w("three_modifiers_with_args", "library test;\ntype Color = strict(removed=2) flexible(added=2) resource(added=3) enum {\n    RED = 1;\n};"),
	}})
}

func TestGleamIntegerSelfConflictLockedCParity(t *testing.T) {
	runWitnessSet(t, witnessSet{
		lang: "gleam", label: "integer-self-conflict", routes: true,
		rows: srcs(
			"{-1}", "{1}", "{-0}", "{0}", "{-123_456}", "{123_456}",
			"{-0xff}", "{0xff}", "{-0b1010}", "{0b1010}",
			"{-1.5}", "{1.5}", "{-x}", "{1 - 2}", "{1 - -2}",
			"{f(-1, 2)}", "{[-1, 0, 1]}", "{#(-1, 2)}", "{(-1)}",
			"fn main() { -1 }\n", "fn main() { 1 }\n",
		),
	})
}

func TestHaskellLayoutEndLockedCParity(t *testing.T) {
	runWitnessSet(t, witnessSet{
		lang: "haskell", label: "layout-end", routes: true,
		rows: srcs(
			"a=do\"\"[\"\"]\n",
			"a=do\"\"[\"\"]",
			"a = do\n  f [\"\"]\n",
			"a = do\n  f [\"\"]\n\nb = 1\n",
			"a = do\n  f [\"\"]\n  -- trailing comment\n",
		),
	})
}

// TestHLSLSubscriptAssignmentLockedCParity checks the same
// subscript-assignment witnesses grammars/hlsl_subscript_assignment_native_regression_test.go
// locks against a native C tree-sitter reference parser, node by node.
func TestHLSLSubscriptAssignmentLockedCParity(t *testing.T) {
	runWitnessSet(t, witnessSet{lang: "hlsl", rows: []witness{
		w("single_index_in_function", "void f() {\n    Foo[bar] = value;\n}\n"),
		w("top_level", "Foo[bar] = value;\n"),
		w("two_indices", "void f() {\n    Foo[bar, baz] = value;\n}\n"),
		w("numeric_index", "void f() {\n    Foo[42] = value;\n}\n"),
		w("call_value", "void f() {\n    Foo[bar] = compute(baz);\n}\n"),
	}})
}

func TestAngularInertRecoveryMaterializationLockedCParity(t *testing.T) {
	runWitnessSet(t, witnessSet{lang: "angular", rows: []witness{
		w("let_non_null", "<div>@let itemLabel = item.label!;</div>"),
		w("binary_non_null", "<div>@if (item.level! > 1) { ok }</div>"),
		w("ampersand_text", "<strong>Opinionated & versatile,</strong>"),
	}})
}

func TestBibtexInertRecoveryMaterializationLockedCParity(t *testing.T) {
	runWitnessSet(t, witnessSet{lang: "bibtex", rows: []witness{
		w("missing_comma_after_key", "@article{test\n    title     = {title-from-embedded-bibtex-file},\n    author    = {author-from-embedded-bibtex-file},\n}\n"),
		w("missing_key", "@techreport{\n  journal = {Wirtschaftsinformatik},\n  author = {Sam and jason},\n}\n"),
	}})
}

func TestEDSInertRecoveryMaterializationLockedCParity(t *testing.T) {
	runWitnessSet(t, witnessSet{lang: "eds", rows: []witness{
		w("embedded_section", "[A]\nEmpty=\n\n[B]\nX=1\n"),
		w("error_lines", "[1003sub0]\nParameterName=Number of errors\nObjectType=0x7\n;StorageLocation=RAM\nDataType=0x0005\nAccessType=rw\nDefaultValue=\nPDOMapping=0\n"),
	}})
}

func TestChatitoInertRecoveryMaterializationLockedCParity(t *testing.T) {
	runWitnessSet(t, witnessSet{lang: "chatito", rows: []witness{
		w("trailing_alias_word", "~[minute]\n    58\n    59"),
	}})
}

func TestDartInheritedFieldProjectionLockedCParity(t *testing.T) {
	runWitnessSet(t, witnessSet{lang: "dart", rows: []witness{
		w("switch_expression_body", "int f(Object value) => switch (value) { int n => n, _ => 0 };\n"),
	}})
}

func TestElixirInheritedFieldProjectionLockedCParity(t *testing.T) {
	runWitnessSet(t, witnessSet{lang: "elixir", rows: []witness{
		w("nested_call_target", "def unquote(f)(x), do: nil\n"),
	}})
}

func TestOrgExpressionAliasLockedCParity(t *testing.T) {
	runWitnessSet(t, witnessSet{
		lang: "org", label: "expression-alias", routes: true,
		rows: srcs(
			"#+BEGIN_\"\n#+END_S",
			"#+TITLE: Hello\n",
			"#+X: a\n",
			"#+BEGIN_SRC sh\nprintf x\n#+END_SRC\n",
			"- [X] done\n",
			"plain text\n",
		),
	})
}

func TestParityJavaScriptAutomaticSemicolonBeforeTrailingComment(t *testing.T) {
	runWitnessSet(t, witnessSet{lang: "javascript", rows: []witness{
		w("javascript-asi-before-trailing-comment", "function f() {\n  if (!size || !pre) { i--; break; } // counter\n  var curr = 1;\n}\n"),
	}})
}

func TestParityJavaScriptAdjacentBlockCommentsDoNotConsumeFollowingToken(t *testing.T) {
	runWitnessSet(t, witnessSet{lang: "javascript", rows: []witness{
		w("javascript-adjacent-block-comments-before-else", "if (x) {} /* first *//* second */ else {}\n"),
	}})
}

func TestForthRecoveryActionMaterializationLockedCParity(t *testing.T) {
	runWitnessSet(t, witnessSet{lang: "forth", rows: []witness{
		w("body", ": foo 1 2\n"),
		w("empty", ": foo\n"),
		w("terminated", ": foo 1 2 ;\n"),
	}})
}

func TestLuauRecoveryActionMaterializationLockedCParity(t *testing.T) {
	runWitnessSet(t, witnessSet{lang: "luau", rows: []witness{
		w("end", "end"),
		w("declaration_end", "local x = 1\nend\n"),
		w("if_end", "if true then\nend\nend\n"),
	}})
}

// TestQLLoneIdentifierLockedCParity compares the error tree that a lone
// identifier produces, with and without a trailing newline, against the C oracle.
func TestQLLoneIdentifierLockedCParity(t *testing.T) {
	runWitnessSet(t, witnessSet{lang: "ql", rows: []witness{
		w("lone_identifier", "x"),
		w("lone_identifier_newline", "x\n"),
	}})
}

func TestRobotSkippedErrorLockedCParity(t *testing.T) {
	runWitnessSet(t, witnessSet{lang: "robot", rows: []witness{
		w("expression", `${tc['\${i}']}`),
	}})
}

func TestSchemeQuoteFamilySkippedErrorLockedCParity(t *testing.T) {
	runWitnessSet(t, witnessSet{lang: "scheme", rows: []witness{
		w("quote_adjacent_space", "'| x"),
		w("quote_separated_space", "' | x"),
		w("quote_adjacent", "'|x"),
		w("quote_separated", "' |x"),
		w("quasiquote", "`| x"),
		w("unquote", ",| x"),
		w("unquote_splicing", ",@| x"),
		w("syntax", "#'| x"),
		w("quasisyntax", "#`| x"),
		w("unsyntax", "#,| x"),
		w("unsyntax_splicing", "#,@| x"),
		w("list", "('| x)"),
	}})
}

func TestTealStringAliasLockedCParity(t *testing.T) {
	runWitnessSet(t, witnessSet{
		lang: "teal", label: "string-alias", routes: true,
		rows: srcs(
			`d{["%"]=""}`,
			`local t = { ["%"] = "" }`,
			`local s = "%"`,
			`local s = "a%b"`,
			`local s = [[%]]`,
			`local s = "plain"`,
		),
	})
}

func TestTypstNestedListLockedCParity(t *testing.T) {
	// This source comes from typst/list/list-nested in the locked
	// tree-sitter-typst corpus.
	runWitnessSet(t, witnessSet{lang: "typst", rows: []witness{
		w("locked_nested_list", `- First level.

  - Second level.
    There are multiple paragraphs.

    - Third level.

    Still the same bullet point.

  - Still level 2.

- At the top.

`),
	}})
}
