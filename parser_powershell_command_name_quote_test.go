package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestPowerShellCommandNameImmediateSingleQuoteParsesCleanly pins the
// airbus-cert/tree-sitter-powershell@1ffcd63 fix ("Handle simple quote
// handler"), carried by the languages.lock bump from da65ba3acc93 to
// e7bd348c49fd. That commit added a `command_name` alternative
// (`token.immediate("'")` + a new hidden `_string_literal_immediate` rule)
// mirroring the existing double-quote handling, so a command name may
// immediately continue into a single-quoted run without breaking the token.
//
// C-oracle-equivalent evidence: rebuilding the pre-fix blob (upstream
// da65ba3acc93) from cmd/ts2go and parsing this same source produces
// `(program (ERROR (ERROR) (simple_name) (ERROR) (simple_name) (ERROR)))`
// with HasError() == true. The post-fix blob (this test) must parse the
// command name as one clean command_name with zero ERROR nodes.
func TestPowerShellCommandNameImmediateSingleQuoteParsesCleanly(t *testing.T) {
	src := []byte("foo'bar' baz\n")
	lang := grammars.PowershellLanguage()

	for _, route := range []struct {
		name  string
		force bool
	}{
		{"production", false},
		{"compact-candidate", true},
	} {
		t.Run(route.name, func(t *testing.T) {
			parser := gts.NewParser(lang)
			parser.SetAdmissionCandidateRoute(route.force)
			tree, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("PowerShell parse error: %v", err)
			}
			defer tree.Release()
			root := requireCompleteParse(t, tree, src, lang, "PowerShell immediate single-quote command name")

			if root.HasError() {
				t.Fatalf("root.HasError() = true, want false; tree=%s", root.SExpr(lang))
			}
			if errs := collectNodesByType(root, lang, "ERROR"); len(errs) != 0 {
				t.Fatalf("found %d ERROR node(s), want 0: %v; tree=%s", len(errs), errs, root.SExpr(lang))
			}

			names := collectNodesByType(root, lang, "command_name")
			if len(names) != 1 {
				t.Fatalf("command_name count = %d, want 1; tree=%s", len(names), root.SExpr(lang))
			}
			name := names[0]
			if name.StartByte() != 0 || name.EndByte() != 8 {
				t.Fatalf("command_name span = [%d,%d), want [0,8) (\"foo'bar'\"); tree=%s", name.StartByte(), name.EndByte(), root.SExpr(lang))
			}
		})
	}
}
