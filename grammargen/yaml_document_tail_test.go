package grammargen

import (
	"slices"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// Pinned tree-sitter-yaml a1c4812a reduces _bgn_imp_doc after the initial
// _exp_doc_tal goto, before consuming the document-end marker.
func TestYAMLDocumentTailReducesBeforeEnd(t *testing.T) {
	grammar := YAMLCoreGrammar()
	externals := grammar.Externals
	lang, err := GenerateLanguage(grammar)
	if err != nil {
		t.Fatal(err)
	}
	symbol := func(name string) gotreesitter.Symbol {
		t.Helper()
		i := slices.Index(lang.SymbolNames, name)
		if i < 0 {
			t.Fatalf("missing symbol %q", name)
		}
		return gotreesitter.Symbol(i)
	}

	tail := symbol("_exp_doc_tal")
	state := gotreesitter.StateID(lookupActionIndexForLanguage(lang, lang.InitialState, tail))
	if wide, ok := lang.LargeStateGotos[uint64(lang.InitialState)<<32|uint64(tail)]; ok {
		state = wide
	}
	if state == 0 {
		t.Fatal("missing initial goto on _exp_doc_tal")
	}
	// The scanner uses the grammar's external-token order even when the
	// generated symbol has a default display alias ("..." for _s_doc_end).
	end := slices.IndexFunc(externals, func(r *Rule) bool { return r.Value == "_s_doc_end" })
	if end < 0 || end >= len(lang.ExternalSymbols) {
		t.Fatal("missing external _s_doc_end")
	}
	group := lookupActionIndexForLanguage(lang, state, lang.ExternalSymbols[end])
	actions := lang.ParseActions[group].Actions
	want := symbol("_bgn_imp_doc")
	if len(actions) == 1 {
		a := actions[0]
		if a.Type == gotreesitter.ParseActionReduce && a.Symbol == want && a.ChildCount == 1 {
			return
		}
		if a.Type == gotreesitter.ParseActionShift {
			t.Fatalf("after _exp_doc_tal on _s_doc_end: got shift to state %d, want reduce _bgn_imp_doc (1 child)", a.State)
		}
	}
	t.Fatalf("after _exp_doc_tal on _s_doc_end: got actions %+v, want one reduce _bgn_imp_doc (1 child)", actions)
}
