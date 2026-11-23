package grammargen

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

func TestYAMLCoreExternalOnlyLexer(t *testing.T) {
	g := YAMLCoreGrammar()
	lang, blob, err := GenerateLanguageAndBlob(g)
	if err != nil {
		t.Fatal(err)
	}
	for _, obj := range []*gts.Language{lang, mustLoadOwnedYAML(t, blob)} {
		if len(obj.LexStates) != 1 || !obj.LexerStateAcceptsEndBeforeEOF(0) || len(obj.LexStates[0].Transitions) != 0 {
			t.Fatalf("expected the scanner alone to own whitespace: %+v", obj.LexStates)
		}
	}
	c, err := GenerateC(g)
	if err != nil {
		t.Fatal(err)
	}
	_, body, ok := strings.Cut(c, "static bool ts_lex(")
	if !ok {
		t.Fatal("missing internal lexer")
	}
	def, _, _ := strings.Cut(body, "static const")
	if !strings.Contains(def, "\n      ACCEPT_TOKEN(ts_builtin_sym_end);") || strings.Contains(def, "SKIP(") {
		t.Fatalf("C emitted the wrong explicit end lexer: %s", def)
	}
}

func mustLoadOwnedYAML(t *testing.T, blob []byte) *gts.Language {
	t.Helper()
	lang, err := gts.LoadLanguage(blob)
	if err != nil {
		t.Fatal(err)
	}
	return lang
}
