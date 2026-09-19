package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestRelexReservedWordGuardBlocksReservedWordAsIdentifier is a regression
// guard for Fix B (parser_recover_c.go relexTokenForStackLexState).
//
// relexTokenForStackLexState had no reserved-word guard: under
// GOT_C_RECOVERY=all it relexed the reserved word "if" to a plain identifier
// in the state that expects a binding name after "var", so
// "var if = 1;" parsed with no error at all. Tree-sitter C blocks this at
// ts_language_is_reserved_word (parser.c); this guard uses the same
// reserved-word set (keywordReservedInState) to keep the parse from
// silently accepting a reserved word as a binding identifier.
func TestRelexReservedWordGuardBlocksReservedWordAsIdentifier(t *testing.T) {
	t.Setenv("GOT_C_RECOVERY", "all")
	lang := grammars.JavascriptLanguage()
	if lang == nil {
		t.Skip("javascript grammar not registered")
	}
	src := []byte("var if = 1;\n")
	tree, err := gts.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q) returned error: %v", src, err)
	}
	defer tree.Release()
	if !tree.RootNode().HasError() {
		t.Fatalf("expected HasError=true: the reserved word \"if\" must not silently relex to an identifier binding name:\n%s", tree.RootNode().SExpr(lang))
	}
}
