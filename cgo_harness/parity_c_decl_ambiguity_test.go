//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

// TestParityCTopLevelDeclAmbiguity is the adversarial safety gate for the C
// translation_unit_repeat1 fork collapse (parser.go cRepetitionShiftConflictChoice,
// state 43). Collapsing the top-level list-continuation fork must NOT change how
// the deeper declaration-vs-expression-statement ambiguity resolves — C's
// hardest parsing case, declared in tree-sitter-c's conflicts: block. Each
// snippet is parsed by gotreesitter and the C reference and compared
// node-by-node by runParityCase; any divergence fails.
func TestParityCTopLevelDeclAmbiguity(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			// The classic ambiguity: `A * b;` — A-as-type (declaration of pointer b)
			// vs A-as-value (multiplication expression statement). Repeated to drive
			// the state-43 list boundary between each.
			name: "star_ambiguity",
			src:  "A * b;\nc * d;\nE * f;\n",
		},
		{
			// Many heterogeneous top-level items back to back — every boundary hits
			// state 43 with a different declaration-starter lookahead.
			name: "heterogeneous_items",
			src: "typedef int T;\n" +
				"T x;\n" +
				"int y = 1;\n" +
				"void f(void) { return; }\n" +
				"struct S { int m; };\n" +
				"enum E { X, Y };\n" +
				"static const char *s = \"hi\";\n" +
				"#define MAC 1\n" +
				"int arr[3] = {1, 2, 3};\n",
		},
		{
			// K&R old-style function definition followed by more items — the
			// _old_style_parameter_list conflict lives near the top level.
			name: "knr_function",
			src:  "int g(a, b)\nint a;\nint b;\n{\n  return a + b;\n}\nint h;\n",
		},
		{
			// Call-vs-declaration: `T (x);` is a declaration of x; `foo(x);` is a
			// call expression statement. Both at top level across state 43.
			name: "paren_decl_vs_call",
			src:  "T (x);\nfoo(y);\nint z;\n",
		},
		{
			// Function definitions interleaved with declarations — the most common
			// real-C shape, many state-43 boundaries.
			name: "funcs_and_decls",
			src: "int a;\n" +
				"void f(int p) { p++; }\n" +
				"long b, c;\n" +
				"static int g(void) { return 0; }\n" +
				"unsigned d;\n",
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			tc := parityCase{name: "c", source: c.src}
			runParityCase(t, tc, "fresh", normalizedSource("c", c.src))
		})
	}
}
