//go:build !grammar_subset || grammar_subset_lua

package grammars

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// TestLuaExternalSymbolConstants pins the hardcoded scanner symbols to the
// blob's ExternalSymbols table so a regenerated blob cannot silently skew
// the mapping.
func TestLuaExternalSymbolConstants(t *testing.T) {
	lang := LuaLanguage()
	want := []gotreesitter.Symbol{
		luaSymBlockCommentStart,
		luaSymBlockCommentContent,
		luaSymBlockCommentEnd,
		luaSymBlockStringStart,
		luaSymBlockStringContent,
		luaSymBlockStringEnd,
	}
	if len(lang.ExternalSymbols) != len(want) {
		t.Fatalf("ExternalSymbols len = %d, want %d", len(lang.ExternalSymbols), len(want))
	}
	for i, sym := range want {
		if lang.ExternalSymbols[i] != sym {
			t.Fatalf("ExternalSymbols[%d] = %d, want %d", i, lang.ExternalSymbols[i], sym)
		}
	}
}

func luaParse(t *testing.T, src string) string {
	t.Helper()
	lang := LuaLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse(%q) failed: %v", src, err)
	}
	t.Cleanup(tree.Release)
	root := tree.RootNode()
	if root == nil {
		t.Fatalf("Parse(%q) returned nil root", src)
	}
	return root.SExpr(lang)
}

// TestLuaBlockScannerMatchesC exercises the LuaExternalScanner port of the
// pinned upstream scanner.c (tree-sitter-lua @ 10fe0054): long-bracket block
// strings and comments with level counting.
func TestLuaBlockScannerMatchesC(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "block string",
			src:  "s = [[hello]]\n",
			want: `(chunk (assignment_statement (variable_list (identifier)) (expression_list (string (string_content)))))`,
		},
		{
			name: "leveled block string",
			src:  "s = [==[a]] b]==]\n",
			want: `(chunk (assignment_statement (variable_list (identifier)) (expression_list (string (string_content)))))`,
		},
		{
			name: "empty block string",
			src:  "s = [[]]\n",
			want: `(chunk (assignment_statement (variable_list (identifier)) (expression_list (string (string_content)))))`,
		},
		{
			name: "block comment",
			src:  "--[[ hi ]]\nx = 1\n",
			want: `(chunk (comment (comment_content)) (assignment_statement (variable_list (identifier)) (expression_list (number))))`,
		},
		{
			name: "leveled block comment with inner brackets",
			src:  "--[=[ a ]] b ]=]\nx = 1\n",
			want: `(chunk (comment (comment_content)) (assignment_statement (variable_list (identifier)) (expression_list (number))))`,
		},
		{
			name: "line comment unaffected",
			src:  "-- plain comment\nx = 1\n",
			want: `(chunk (comment (comment_content)) (assignment_statement (variable_list (identifier)) (expression_list (number))))`,
		},
		{
			name: "multiline block string",
			src:  "s = [[\nline1\nline2\n]]\n",
			want: `(chunk (assignment_statement (variable_list (identifier)) (expression_list (string (string_content)))))`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := luaParse(t, tc.src)
			if got != tc.want {
				t.Fatalf("SExpr mismatch for %q:\n got  %s\n want %s", tc.src, got, tc.want)
			}
		})
	}
}

func TestLuaStringEscapesMatchCShape(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{name: "decimal_escape", src: "a = '\\027'\n"},
		{name: "backslash_escape", src: "a = '\\\\'\n"},
		{name: "newline_escape", src: "a = '\\n'\n"},
	}
	want := `(chunk (assignment_statement (variable_list (identifier)) (expression_list (string (string_content (escape_sequence))))))`
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := luaParse(t, tc.src)
			if got != want {
				t.Fatalf("SExpr mismatch for %q:\n got  %s\n want %s", tc.src, got, want)
			}
		})
	}
}
