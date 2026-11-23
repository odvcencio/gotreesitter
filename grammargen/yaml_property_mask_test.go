package grammargen

import (
	"slices"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

// After "- &a !t", an implicit key may continue with a colon. Same-indent
// property and mapping-key starts belong to other scanner contexts.
func TestYAMLCoreAnchorThenTagLookaheads(t *testing.T) {
	grammar := YAMLCoreGrammar()
	normalized, err := Normalize(grammar)
	if err != nil {
		t.Fatal(err)
	}
	lang, err := GenerateLanguage(grammar)
	if err != nil {
		t.Fatal(err)
	}
	external := func(name string) int {
		t.Helper()
		for i, r := range grammar.Externals {
			if r.Value == name {
				return i
			}
		}
		t.Fatalf("unknown external %q", name)
		return 0
	}
	shift := func(state gts.StateID, name string) gts.StateID {
		t.Helper()
		token := lang.ExternalSymbols[external(name)]
		actions := lang.ParseActions[lookupActionIndexForLanguage(lang, state, token)].Actions
		if len(actions) != 1 || actions[0].Type != gts.ParseActionShift {
			t.Fatalf("expected a single shift on %s, got %+v", name, actions)
		}
		return actions[0].State
	}
	start := shift(lang.InitialState, "_r_blk_seq_bgn")
	for _, anchor := range []string{"_r_acr", "_br_acr"} {
		t.Run(anchor, func(t *testing.T) {
			// Default display aliases rename these generated symbols to
			// "anchor". Use the normalized grammar's pre-display identities.
			i := slices.IndexFunc(normalized.Symbols, func(symbol SymbolInfo) bool {
				return symbol.Name == anchor
			})
			if i < 0 {
				t.Fatalf("unknown symbol %q", anchor)
			}
			gotoState := lookupActionIndexForLanguage(lang, start, gts.Symbol(i))
			if gotoState == 0 {
				t.Fatalf("missing goto on %s", anchor)
			}
			afterTag := shift(gts.StateID(gotoState), "_r_tag")
			mask := lang.ExternalLexStates[lang.LexModes[afterTag].ExternalLexState]
			for _, name := range []string{"_r_blk_imp_bgn", "_b_blk_seq_bgn", "_br_tag", "comment"} {
				if !mask[external(name)] {
					t.Errorf("missing required %s", name)
				}
			}
			for _, name := range []string{"_b_tag", "_b_blk_key_bgn", "_b_sgl_pln_str_blk"} {
				if mask[external(name)] {
					t.Errorf("unexpected %s", name)
				}
			}
		})
	}
}
