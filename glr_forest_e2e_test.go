package gotreesitter

import "testing"

// TestParseForestJSONEndToEnd drives the GSS-forest GLR loop with real JSON
// tokens and compares the resulting tree to the production parser. JSON has no
// external scanner and state-independent structural lexing, so single-lead-state
// lexing is faithful — a clean first end-to-end exercise of coalesce +
// reduce-over-DAG + the reused node-builder, no deep-equivalence anywhere.
// TestParseForestEndToEnd drives the GSS-forest GLR parser (coalesce +
// reduce-over-DAG, no deep equivalence) through the production token source and
// asserts byte-identical trees vs the production parser across three real
// grammars. Extras (comments) are the next layer and are intentionally absent.
func TestParseForestEndToEnd(t *testing.T) {
	// want, when non-empty, overrides the production oracle for cases where the
	// production parser is itself wrong — the forest must match the value, which
	// was verified byte-for-byte against the real C tree-sitter (cgo oracle).
	cases := []struct{ lang, src, want string }{
		{lang: "json", src: `[1, 2]`},
		{lang: "json", src: `[]`},
		{lang: "json", src: `[1, [2, [3, 4]], 5]`},
		{lang: "json", src: `{"a": 1, "b": [true, false, null]}`},
		{lang: "json", src: `{"x": {"y": {"z": -3.5}}, "w": "str"}`},
		{lang: "json", src: `[{"k": [1, 2]}, {"k": []}]`},
		{lang: "go", src: "package main\n"},
		{lang: "go", src: "var x = 1\n"},
		{lang: "go", src: "func f() { return }\n"},
		{lang: "go", src: "func add(a, b int) int { return a + b }\n"},
		{lang: "c", src: "int x;\n"},
		{lang: "c", src: "int f(void) { return 0; }\n"},
		{lang: "c", src: "struct S { int a; };\n"},
		{lang: "c", src: "struct S { long b; };\n"}, // Stage-3 disambiguation
		{lang: "c", src: "struct S { int a; long b; unsigned c; };\n"},
		{lang: "c", src: "int g(int a, char *b) { return a + *b; }\n"},
		// Deeper expressions, control flow, multiple statements.
		{lang: "c", src: "int m(void) { int x = 1 + 2 * 3; if (x > 4) return x; else return 0; }\n"},
		{lang: "c", src: "int *p[10]; void h(int n) { for (int i = 0; i < n; i++) p[i] = &n; }\n"},
		{lang: "c", src: "typedef struct { int x, y; } Point; Point mk(int a, int b) { Point q = {a, b}; return q; }\n"},
		// Production errors on an enumerator with an explicit value (B = 3); the
		// forest parses it correctly (matches the cgo C tree-sitter oracle), so we
		// assert against the known-correct tree rather than the buggy oracle.
		{lang: "c", src: "enum E { A, B = 3, C }; int v = A | B & C;\n",
			want: "(translation_unit (enum_specifier (type_identifier) (enumerator_list (enumerator (identifier)) (enumerator (identifier) (number_literal)) (enumerator (identifier)))) (declaration (primitive_type) (init_declarator (identifier) (binary_expression (identifier) (binary_expression (identifier) (identifier))))))"},
		{lang: "go", src: "package p\nfunc g(xs []int) int { s := 0; for _, x := range xs { s += x }; return s }\n"},
		{lang: "go", src: "package p\ntype T struct { A int; B string }\nfunc (t *T) M() int { return t.A }\n"},
		{lang: "go", src: "package p\nvar m = map[string]int{\"a\": 1, \"b\": 2}\n"},
		{lang: "json", src: `{"nested": {"a": [1, {"b": [2, 3]}], "c": null}, "d": [[[]]]}`},
		{lang: "json", src: `[true, false, null, -1.5e10, "with \"escapes\" and \\ slashes"]`},
	}
	for _, c := range cases {
		lang := loadBlobForDecode(t, c.lang)
		want := c.want
		if want == "" {
			want = mustParseSExpr(t, lang, []byte(c.src))
		}
		got, ok := forestParseSExpr(t, lang, []byte(c.src))
		if !ok {
			t.Errorf("%s %q: forest parse failed", c.lang, c.src)
			continue
		}
		if got != want {
			t.Errorf("%s %q: mismatch\n forest=%s\n want  =%s", c.lang, c.src, got, want)
			continue
		}
		t.Logf("OK  %-5s %q", c.lang, c.src)
	}
}

// TestParseForestExtras exercises extra (comment) handling across every
// position relative to the tree: interior to a node, between siblings, leading
// the root, trailing at EOF, and combinations. Each must match the production
// parser byte-for-byte — extras are state-transparent shifts, excluded from a
// production's child count, trimmed when trailing a reduced node and re-pushed
// to the surrounding context, and folded into the root when they lead or trail
// the whole file.
func TestParseForestExtras(t *testing.T) {
	cases := []struct{ lang, src string }{
		{"c", "int /* c */ x;\n"},                                   // interior to a declaration
		{"c", "int x; // c\nint y;\n"},                              // between two declarations
		{"c", "int x; /* a */ /* b */ int y;\n"},                    // two adjacent extras
		{"c", "int f(void) { return 0; /* done */ }\n"},             // trailing a statement
		{"c", "// leading\nint x;\n"},                               // leading the root
		{"c", "int x;\n// trailing\n"},                              // trailing at EOF
		{"c", "/* lead */ int x; // mid\nint y; /* tail */\n"},      // leading + interior + trailing
		{"c", "int f(void) {\n  // body comment\n  return 0;\n}\n"}, // inside a block
		{"go", "package p // pkg\nfunc f() {}\n"},
		{"go", "// header\npackage p\nvar x = 1 // trailing\n"},
	}
	for _, c := range cases {
		lang := loadBlobForDecode(t, c.lang)
		want := mustParseSExpr(t, lang, []byte(c.src))
		got, ok := forestParseSExpr(t, lang, []byte(c.src))
		if !ok {
			t.Errorf("%s %q: forest parse failed", c.lang, c.src)
			continue
		}
		if got != want {
			t.Errorf("%s %q: mismatch\n forest=%s\n normal=%s", c.lang, c.src, got, want)
			continue
		}
		t.Logf("OK  %-4s %q", c.lang, c.src)
	}
}

func mustParseSExpr(t *testing.T, lang *Language, src []byte) string {
	tree, err := NewParser(lang).Parse(src)
	if err != nil {
		t.Fatalf("normal parse %q: %v", src, err)
	}
	defer tree.Release()
	return tree.RootNode().SExpr(lang)
}

func forestParseSExpr(t *testing.T, lang *Language, src []byte) (string, bool) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()
	parser := NewParser(lang)
	root, ok := parser.parseForest(arena, src, true, parseMemoryBudgetForParser(parser, len(src)), nil)
	if !ok || root == nil {
		return "", false
	}
	return root.SExpr(lang), true
}
