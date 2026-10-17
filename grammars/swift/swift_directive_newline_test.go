package swift_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars/swift"
)

// Issue #1399 regression witnesses contributed by AdamMagued in PR #1442.
func TestSwiftDirectiveNewlineRepros(t *testing.T) {
	lang := swift.Language()

	cases := []struct {
		name string
		src  string
	}{
		{"warning", "let x = 1\n#warning(\"w\")\n"},
		{"if", "let x = 1\n#if compiler(>=5.9)\nlet y = 2\n#endif\n"},
		{"if-import", "#if FOO\nimport A\n#else\nimport B\n#endif\n"},
		{"preview", "struct A {}\n#Preview { Text(\"hi\") }\n"},
		{"expect", "func f() {\n  let x = 1\n  #expect(x == 1)\n}\n"},
		{"explicit-separator", "let x = 1;\n#warning(\"w\")\n"},
		{"line-comment", "let x = 1 // comment\n#warning(\"w\")\n"},
		{"block-comment", "let x = 1 /* comment */\n#warning(\"w\")\n"},
		{"intervening-comment", "let x = 1\n// comment\n#warning(\"w\")\n"},
		{"empty-type-body", "struct S {\n#if FOO\nvar x: Int\n#else\nvar y: Int\n#endif\n}\n"},
		{"type-member-separator", "struct S {\nvar x: Int\n#if FOO\nvar y: Int\n#else\nvar z: Int\n#endif\n}\n"},
		{"function-body", "func f() {\nlet x = 1\n#if FOO\nprint(x)\n#elseif BAR\nprint(2)\n#else\nprint(3)\n#endif\n}\n"},
		{"nested-directives", "#if FOO\n#if BAR\nimport A\n#else\nimport B\n#endif\n#else\nimport C\n#endif\n"},
		{"consecutive-directives", "#warning(\"a\")\n#warning(\"b\")\n"},
		{"raw-string", "let x = #\"#if FOO\"#\n#warning(\"w\")\n"},
		{"no-trailing-newline", "let x = 1\n#warning(\"w\")"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, candidate := range []bool{false, true} {
				t.Run(candidateRouteName(candidate), func(t *testing.T) {
					p := gotreesitter.NewParser(lang)
					p.SetAdmissionCandidateRoute(candidate)
					tree, err := p.Parse([]byte(tc.src))
					if err != nil {
						t.Fatalf("parse error: %v", err)
					}
					defer tree.Release()
					root := tree.RootNode()
					if root.HasError() {
						t.Errorf("expected clean parse, got error: %s", root.SExpr(lang))
					}
				})
			}
		})
	}
}

func TestSwiftDirectiveSyntaxErrors(t *testing.T) {
	lang := swift.Language()

	negativeCases := []struct {
		name string
		src  string
	}{
		{"invalid-identifier", "let 123 = 1\n#warning(\"w\")\n"},
		{"unmatched-paren", "func f( {\n#warning(\"w\")\n}\n"},
	}

	for _, tc := range negativeCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, candidate := range []bool{false, true} {
				t.Run(candidateRouteName(candidate), func(t *testing.T) {
					p := gotreesitter.NewParser(lang)
					p.SetAdmissionCandidateRoute(candidate)
					tree, err := p.Parse([]byte(tc.src))
					if err != nil {
						t.Fatalf("parse error: %v", err)
					}
					defer tree.Release()
					root := tree.RootNode()
					if !root.HasError() {
						t.Errorf("expected syntax error, got clean parse: %s", root.SExpr(lang))
					}
				})
			}
		})
	}
}

func candidateRouteName(candidate bool) string {
	if candidate {
		return "candidate"
	}
	return "production"
}
