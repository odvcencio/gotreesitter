package parse_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestJavaScriptBlockThenAssignmentParsesClean is a regression guard for #111.
//
// A standalone block statement `{…}` immediately followed by a SIMPLE assignment
// statement (ASI, no semicolon) collapses the GLR parse into a root ERROR node.
// Minifiers emit `}x=y` constantly (`if(c){…}next=…`), so this silently nukes
// real-world minified bundles — mixpanel/amplitude parse to a root ERROR and lose
// nearly every string, which breaks JS recon, code-intel, and highlighting.
//
// Root cause: only simple `=` permits a destructuring-pattern LHS (`{x}=obj`), so
// after a bare block `{a}` the parser cannot disambiguate statement_block vs
// object/object_pattern assignment target. The distinguishing cases:
//
//	{a}b=c   -> ERROR (bug)
//	{a}b+=c  -> clean (compound assignment has no pattern-LHS ambiguity)
//	{a};b=c  -> clean (explicit semicolon removes the ambiguity)
//	{a}b()   -> clean (call expression statement, not an assignment)
//	if(x){}y=z -> clean (the `{}` binds to the `if`; only a standalone block breaks)
//
// Minimal reproducer: `{a}b=c`.
func TestJavaScriptBlockThenAssignmentParsesClean(t *testing.T) {
	assertBlockThenAssignmentParsesClean(t, grammars.JavascriptLanguage(), []struct {
		name string
		src  string
	}{
		// Currently broken (the bug):
		{"minimal", `{a}b=c`},
		{"member_target", `{a}b.c=d`},
		{"path_value", `{a}b="/v1/users"`},
		{"sequence", `{a}b=c,d=e`},
		{"realworld_fn", `function S(){if(a){return}f=new e}`},
		// Controls that already parse clean today — must stay clean after the fix:
		{"compound_assign", `{a}b+=c`},
		{"explicit_semicolon", `{a};b=c`},
		{"if_block", `if(x){}y=z`},
		{"call_after_block", `{a}b()`},
	})
}

func TestTypeScriptBlockThenAssignmentParsesClean(t *testing.T) {
	assertBlockThenAssignmentParsesClean(t, grammars.TypescriptLanguage(), []struct {
		name string
		src  string
	}{
		{"minimal", `{a}b=c`},
		{"member_target", `{a}b.c=d`},
		{"path_value", `{a}b="/v1/users"`},
	})
}

func TestTSXBlockThenAssignmentParsesClean(t *testing.T) {
	assertBlockThenAssignmentParsesClean(t, grammars.TsxLanguage(), []struct {
		name string
		src  string
	}{
		{"minimal", `{a}b=c`},
		{"member_target", `{a}b.c=d`},
		{"path_value", `{a}b="/v1/users"`},
	})
}

func assertBlockThenAssignmentParsesClean(t *testing.T, lang *gts.Language, cases []struct {
	name string
	src  string
}) {
	t.Helper()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree, err := gts.NewParser(lang).Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", tc.src, err)
			}
			defer tree.Release()
			if root := tree.RootNode(); root.HasError() {
				t.Fatalf("Parse(%q) produced an ERROR tree; want clean parse:\n%s", tc.src, root.SExpr(lang))
			}
		})
	}
}
