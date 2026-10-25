package grammargen

import (
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestImportGrammarJSONReservedWordSets(t *testing.T) {
	// Preserve declared sets in source order. RESERVED wrappers retain their
	// existing structural handling independently of the global declaration.
	data := []byte(`{
		"name": "reserved_import",
		"rules": {
			"source_file": {"type": "SYMBOL", "name": "stmt"},
			"stmt": {"type": "RESERVED", "context_name": "global", "content": {"type": "SYMBOL", "name": "identifier"}},
			"identifier": {"type": "PATTERN", "value": "[a-z]+"}
		},
		"extras": [],
		"conflicts": [],
		"externals": [],
		"inline": [],
		"supertypes": [],
		"word": "identifier",
		"reserved": {
			"global": [
				{"type": "STRING", "value": "if"}
			],
			"property": []
		},
		"precedences": []
	}`)

	g, err := ImportGrammarJSON(data)
	if err != nil {
		t.Fatalf("ImportGrammarJSON failed: %v", err)
	}

	if len(g.ReservedWordSets) != 2 {
		t.Fatalf("len(ReservedWordSets) = %d, want 2", len(g.ReservedWordSets))
	}
	if g.ReservedWordSets[0].Name != "global" {
		t.Fatalf("first reserved set = %q, want %q", g.ReservedWordSets[0].Name, "global")
	}
	if len(g.ReservedWordSets[0].Rules) != 1 || g.ReservedWordSets[0].Rules[0].Kind != RuleString || g.ReservedWordSets[0].Rules[0].Value != "if" {
		t.Fatalf("global reserved rules = %#v, want single STRING(\"if\")", g.ReservedWordSets[0].Rules)
	}
	if g.ReservedWordSets[1].Name != "property" {
		t.Fatalf("second reserved set = %q, want %q", g.ReservedWordSets[1].Name, "property")
	}
}

func TestImportGrammarJSONReservedWordSetsPreservedWithoutRESERVED(t *testing.T) {
	// An explicit global reserved list applies without context-specific wrappers.
	data := []byte(`{
		"name": "reserved_import_bare",
		"rules": {
			"source_file": {"type": "SYMBOL", "name": "identifier"},
			"identifier": {"type": "PATTERN", "value": "[a-z]+"}
		},
		"extras": [],
		"conflicts": [],
		"externals": [],
		"inline": [],
		"supertypes": [],
		"word": "identifier",
		"reserved": {
			"global": [
				{"type": "STRING", "value": "if"}
			]
		},
		"precedences": []
	}`)

	g, err := ImportGrammarJSON(data)
	if err != nil {
		t.Fatalf("ImportGrammarJSON failed: %v", err)
	}
	if len(g.ReservedWordSets) != 1 || g.ReservedWordSets[0].Name != "global" {
		t.Fatalf("reserved sets without wrappers = %+v, want one global set", g.ReservedWordSets)
	}
	rules := g.ReservedWordSets[0].Rules
	if len(rules) != 1 || rules[0] == nil || rules[0].Kind != RuleString || rules[0].Value != "if" {
		t.Fatalf("global reserved rules without wrappers = %+v, want Str(\"if\")", rules)
	}
}

func TestGenerateLanguageReservedWordSets(t *testing.T) {
	lang, err := GenerateLanguage(reservedWordGrammar())
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	if lang.LanguageVersion < 15 {
		t.Fatalf("LanguageVersion = %d, want >= 15", lang.LanguageVersion)
	}
	if lang.MaxReservedWordSetSize == 0 || len(lang.ReservedWords) == 0 {
		t.Fatalf("reserved words missing: stride=%d len=%d", lang.MaxReservedWordSetSize, len(lang.ReservedWords))
	}

	wordSym, ok := lang.SymbolByName("identifier")
	if !ok {
		t.Fatal("missing identifier symbol")
	}
	kwIfSym, ok := lang.SymbolByName("if")
	if !ok {
		t.Fatal("missing if symbol")
	}

	var reservedState gotreesitter.StateID
	var keywordState gotreesitter.StateID
	for state := gotreesitter.StateID(1); int(state) < len(lang.LexModes); state++ {
		wordAction := lookupActionIndexForLanguage(lang, state, wordSym)
		ifAction := lookupActionIndexForLanguage(lang, state, kwIfSym)
		if reservedState == 0 && wordAction != 0 && ifAction == 0 {
			reservedState = state
		}
		if keywordState == 0 && ifAction != 0 {
			keywordState = state
		}
	}

	if reservedState == 0 {
		t.Fatal("did not find a state where identifier is valid and if is reserved")
	}
	if lang.LexModes[reservedState].ReservedWordSetID == 0 {
		t.Fatalf("state %d ReservedWordSetID = 0, want non-zero", reservedState)
	}
	if !reservedSetContains(lang, reservedState, kwIfSym) {
		t.Fatalf("state %d reserved set does not contain if", reservedState)
	}

	if keywordState == 0 {
		t.Fatal("did not find a state where if is a valid keyword token")
	}
	if reservedSetContains(lang, keywordState, kwIfSym) {
		t.Fatalf("state %d reserves if even though it is explicitly valid there", keywordState)
	}
}

func TestEmitCReservedWords(t *testing.T) {
	lang, err := GenerateLanguageForC(reservedWordGrammar())
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}

	csrc, err := EmitC("reserved_emit", lang)
	if err != nil {
		t.Fatalf("EmitC failed: %v", err)
	}

	for _, snippet := range []string{
		"#define MAX_RESERVED_WORD_SET_SIZE",
		"static const TSSymbol ts_reserved_words",
		".reserved_word_set_id = ",
		".reserved_words = &ts_reserved_words[0][0],",
		".max_reserved_word_set_size = ",
	} {
		if !strings.Contains(csrc, snippet) {
			t.Fatalf("generated C missing %q", snippet)
		}
	}
}

func reservedWordGrammar() *Grammar {
	g := NewGrammar("reserved_word_generation")
	g.Define("source_file", Sym("statement"))
	g.Define("statement", Choice(
		Seq(Str("x"), Sym("identifier")),
		Seq(Str("y"), Str("if")),
	))
	g.Define("identifier", Pat("[a-zA-Z_][a-zA-Z0-9_]*"))
	g.Word = "identifier"
	g.ReservedWordSets = []ReservedWordSet{{
		Name:  "global",
		Rules: []*Rule{Str("if")},
	}}
	return g
}

func reservedSetContains(lang *gotreesitter.Language, state gotreesitter.StateID, sym gotreesitter.Symbol) bool {
	if lang == nil || int(state) >= len(lang.LexModes) || lang.MaxReservedWordSetSize == 0 {
		return false
	}
	setID := lang.LexModes[state].ReservedWordSetID
	if setID == 0 {
		return false
	}
	stride := int(lang.MaxReservedWordSetSize)
	start := int(setID) * stride
	end := start + stride
	if start < 0 || start >= len(lang.ReservedWords) {
		return false
	}
	if end > len(lang.ReservedWords) {
		end = len(lang.ReservedWords)
	}
	for _, entry := range lang.ReservedWords[start:end] {
		if entry == 0 {
			break
		}
		if entry == sym {
			return true
		}
	}
	return false
}
