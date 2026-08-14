package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// TestTypeScriptShiftOperatorNotSplitAsCloseAngle guards the signed right-shift
// operator against the compact close-angle split.
//
// splitCompactCloseAngleToken narrows a `>>` token to a single `>` so nested
// generic closers such as `Array<Array<string>>` parse. Java gates that split
// behind hasUnclosedAngleBefore, because `>>` is also a shift operator there.
// TypeScript and TSX carried no such gate, so `a >> (b)` split the shift
// operator into two generic closers and failed to parse. esbuild accepts the
// same source. Found while migrating the GoSX browser runtime to TypeScript:
// bootstrap-src/11a-scene-decompress.ts holds
// `indices[i] = (src[i >> 3] >> (i & 7)) & 1;`.
//
// The split only misfired when the byte after `>>` was one of the openers and
// punctuation listed in shouldSplitCompactCloseAngleToken, which is why
// `a >> b` parsed while `a >> (b)` did not.
func TestTypeScriptShiftOperatorNotSplitAsCloseAngle(t *testing.T) {
	langs := []struct {
		name string
		lang *gotreesitter.Language
	}{
		{"typescript", TypescriptLanguage()},
		{"tsx", TsxLanguage()},
	}

	mustParse := []struct {
		name   string
		source string
	}{
		// Shift operators whose right operand starts with a gated byte.
		{"shift_paren_rhs", "x = a >> (b);"},
		{"shift_paren_rhs_stmt", "a[i] >> (b);"},
		{"shift_indexed_both", "indices[i] = (src[i >> 3] >> (i & 7)) & 1;"},
		{"shift_bracket_rhs", "x = a >> [0][0];"},
		{"shift_brace_rhs_in_call", "f(a >> (b));"},
		{"shift_assign_paren", "x >>= (b);"},

		// Nested generic closers must still split, or these regress.
		{"nested_generic_type", "let v: Array<Array<string>> = [];"},
		{"nested_generic_call", "f<Map<string, Array<number>>>();"},
		{"nested_generic_new", "const m = new Map<string, Array<number>>();"},
	}

	for _, l := range langs {
		if l.lang == nil {
			t.Fatalf("%s grammar unavailable", l.name)
		}
		for _, tc := range mustParse {
			t.Run(l.name+"/"+tc.name, func(t *testing.T) {
				tree, err := gotreesitter.NewParser(l.lang).Parse([]byte(tc.source))
				if err != nil {
					t.Fatalf("parse error: %v\nsource: %s", err, tc.source)
				}
				if tree == nil {
					t.Fatalf("nil tree\nsource: %s", tc.source)
				}
				root := tree.RootNode()
				if root == nil {
					t.Fatalf("nil root\nsource: %s", tc.source)
				}
				if root.IsError() || root.HasError() {
					t.Fatalf("grammar rejected valid source\nsource: %s", tc.source)
				}
			})
		}
	}
}
