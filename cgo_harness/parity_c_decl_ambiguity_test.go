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

// TestParityCPreprocConditional is the adversarial safety gate for collapsing
// the preproc_if_repeat1 fork (parser.go cRepetitionShiftConflictChoice). A
// preprocessor conditional body continues on every content token and closes
// only on #endif/#elif/#else (which carry no continuation shift), so collapsing
// the body's list-continuation fork must not change the parse. Each snippet is
// compared node-by-node against the C reference.
func TestParityCPreprocConditional(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "if_endif_decls",
			src:  "#if FOO\nint a;\nint b;\nvoid f(void) { return; }\n#endif\nint after;\n",
		},
		{
			name: "if_elif_else",
			src:  "#if X\nint p;\n#elif Y\nint q;\nlong q2;\n#else\nint r;\n#endif\n",
		},
		{
			name: "ifdef_else",
			src:  "#ifdef BAR\nvoid f(void) {}\n#else\nvoid g(void) {}\n#endif\n",
		},
		{
			name: "header_guard",
			src:  "#ifndef GUARD_H\n#define GUARD_H\nstruct S { int m; };\ntypedef int T;\n#endif\n",
		},
		{
			name: "nested_if",
			src:  "#if A\nint outer;\n#if B\nint inner;\nvoid h(void) {}\n#endif\nint outer2;\n#endif\n",
		},
		{
			name: "if_in_struct",
			src:  "struct S {\n  int x;\n#if FIELD\n  int y;\n  long z;\n#endif\n  int w;\n};\n",
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
