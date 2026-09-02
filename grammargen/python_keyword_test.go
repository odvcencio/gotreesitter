package grammargen

import (
	"os"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestPythonKeywordIdentificationIncludesSoftKeywords(t *testing.T) {
	gram := loadPythonGrammarJSONForTest(t)

	ng, err := Normalize(gram)
	if err != nil {
		t.Fatalf("normalize Python grammar: %v", err)
	}

	want := map[string]bool{
		"match":  false,
		"case":   false,
		"except": false,
	}
	for _, symID := range ng.KeywordSymbols {
		if symID >= 0 && symID < len(ng.Symbols) {
			if _, ok := want[ng.Symbols[symID].Name]; ok {
				want[ng.Symbols[symID].Name] = true
			}
		}
	}

	for name, found := range want {
		if !found {
			t.Fatalf("keyword %q missing from normalized keyword set", name)
		}
	}
}

func TestPythonGeneratedSoftKeywordCallParsesAsCall(t *testing.T) {
	gram := loadPythonGrammarJSONForTest(t)
	lang, err := GenerateLanguage(gram)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}
	adaptExternalScanner(grammars.PythonLanguage(), lang)

	source := []byte("match(r1, r2, r3)\n")
	tree, err := gotreesitter.NewParser(lang).Parse(source)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()

	root := tree.RootNode()
	sexpr := root.SExpr(lang)
	if root.HasError() {
		t.Fatalf("parse has error: %s", sexpr)
	}
	if !strings.Contains(sexpr, "(call ") {
		t.Fatalf("parse did not produce a call: %s", sexpr)
	}
}

func TestPythonReservedWordsSurviveNormalization(t *testing.T) {
	// Python's grammar.json contains a top-level reserved.global block but
	// never uses RESERVED wrappers. As of the Go-parity fix, ImportGrammarJSON
	// now drops reserved word sets in that case because the runtime
	// buildReservedWordTables path mismatches tree-sitter's semantic and
	// actively harms parsing when every reserved word is a hard keyword that
	// the grammar already lexes directly. To keep this test exercising the
	// normalization path, re-attach a synthetic reserved word set that mirrors
	// what grammar.json advertises.
	gram := loadPythonGrammarJSONForTest(t)
	if len(gram.ReservedWordSets) == 0 {
		gram.ReservedWordSets = []ReservedWordSet{{
			Name: "global",
			Rules: []*Rule{
				Str("if"),
				Str("except"),
				Str("await"),
			},
		}}
	}

	ng, err := Normalize(gram)
	if err != nil {
		t.Fatalf("normalize Python grammar: %v", err)
	}
	if len(ng.ReservedWordSets) == 0 {
		t.Fatal("normalized grammar dropped reserved word sets")
	}
	if len(ng.ReservedWordSets[0]) == 0 {
		t.Fatal("normalized global reserved word set is empty")
	}

	want := map[string]bool{
		"if":     false,
		"except": false,
		"await":  false,
	}
	for _, symID := range ng.ReservedWordSets[0] {
		if symID >= 0 && symID < len(ng.Symbols) {
			if _, ok := want[ng.Symbols[symID].Name]; ok {
				want[ng.Symbols[symID].Name] = true
			}
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("reserved word %q missing from normalized global set", name)
		}
	}
}

func TestPythonGenerateLanguageEmitsReservedWords(t *testing.T) {
	// See TestPythonReservedWordsSurviveNormalization: re-attach a synthetic
	// reserved set so generator coverage is retained after ImportGrammarJSON
	// auto-drops the sets in the absence of RESERVED wrappers.
	gram := loadPythonGrammarJSONForTest(t)
	if len(gram.ReservedWordSets) == 0 {
		gram.ReservedWordSets = []ReservedWordSet{{
			Name: "global",
			Rules: []*Rule{
				Str("if"),
				Str("except"),
				Str("await"),
			},
		}}
	}

	lang, err := GenerateLanguage(gram)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}
	if lang.LanguageVersion < 15 {
		t.Fatalf("LanguageVersion = %d, want >= 15", lang.LanguageVersion)
	}
	if lang.MaxReservedWordSetSize == 0 || len(lang.ReservedWords) == 0 {
		t.Fatalf("reserved words missing from generated language: stride=%d len=%d", lang.MaxReservedWordSetSize, len(lang.ReservedWords))
	}

	nonZeroSetIDs := 0
	for _, mode := range lang.LexModes {
		if mode.ReservedWordSetID > 0 {
			nonZeroSetIDs++
		}
	}
	if nonZeroSetIDs == 0 {
		t.Fatal("generated language has no lex modes with reserved word sets")
	}
}

func TestPythonGeneratedBooleanKeywordsAreLeafTokens(t *testing.T) {
	gram := loadPythonGrammarJSONForTest(t)
	ng, err := Normalize(gram)
	if err != nil {
		t.Fatalf("normalize Python grammar: %v", err)
	}
	for _, name := range []string{"true", "false", "none"} {
		if got := symbolKind(t, ng, name); got != SymbolNamedToken {
			t.Fatalf("%s kind = %v, want SymbolNamedToken", name, got)
		}
	}

	lang, err := GenerateLanguage(gram)
	if err != nil {
		t.Fatalf("GenerateLanguage: %v", err)
	}
	tree, err := gotreesitter.NewParser(lang).Parse([]byte("True\nFalse\nNone\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root.HasError() || root.ChildCount() != 3 {
		t.Fatalf("tree = %s, want three clean children", root.SExpr(lang))
	}
	for i := 0; i < root.ChildCount(); i++ {
		child := root.Child(i)
		if got := child.ChildCount(); got != 0 {
			t.Fatalf("child %d (%s) has %d children, want leaf; tree=%s", i, child.Type(lang), got, root.SExpr(lang))
		}
	}
}

func TestPythonGeneratedLanguageEnablesCRecoveryCostCompetitionByDefault(t *testing.T) {
	gram := loadPythonGrammarJSONForTest(t)
	lang, err := GenerateLanguage(gram)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}
	if !lang.CRecoveryCostCompetitionCapable {
		t.Fatal("generated Python language did not mark C recovery capability")
	}
	if lang.CRecoveryCostCompetitionEnabledByDefault {
		t.Fatal("generated Python language default-certified before scanner lex-state attachment")
	}
	adaptExternalScanner(grammars.PythonLanguage(), lang)
	if !lang.CRecoveryCostCompetitionEnabledByDefault {
		t.Fatal("generated Python language was not default-certified after scanner lex-state attachment")
	}
	if !parserCRecoveryEnabledForTest(gotreesitter.NewParser(lang)) {
		t.Fatal("generated Python language did not enable C recovery in parser")
	}
}

func TestPythonGeneratedLanguageMarksImportedRepeatAux(t *testing.T) {
	gram := loadPythonGrammarJSONForTest(t)

	lang, err := GenerateLanguage(gram)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}
	sym, ok := lang.SymbolByName("module_repeat1")
	if !ok {
		t.Fatal("generated Python language missing module_repeat1")
	}
	if int(sym) >= len(lang.SymbolMetadata) {
		t.Fatalf("module_repeat1 symbol %d outside metadata len %d", sym, len(lang.SymbolMetadata))
	}
	if !lang.SymbolMetadata[sym].GeneratedRepeatAux {
		t.Fatal("module_repeat1 GeneratedRepeatAux = false, want true")
	}
	if !hasGeneratedRepeatBoundaryConflictForTest(lang) {
		t.Fatal("generated Python language has no generated repeat boundary conflict")
	}
}

func hasGeneratedRepeatBoundaryConflictForTest(lang *gotreesitter.Language) bool {
	if lang == nil {
		return false
	}
	for _, entry := range lang.ParseActions {
		shiftFound := false
		reduceFound := false
		ok := len(entry.Actions) >= 2
		for _, act := range entry.Actions {
			switch act.Type {
			case gotreesitter.ParseActionShift:
				if !act.Repetition || act.Extra || shiftFound {
					ok = false
				}
				shiftFound = true
			case gotreesitter.ParseActionReduce:
				if int(act.Symbol) >= len(lang.SymbolMetadata) || !lang.SymbolMetadata[act.Symbol].GeneratedRepeatAux {
					ok = false
				}
				reduceFound = true
			default:
				ok = false
			}
		}
		if ok && shiftFound && reduceFound {
			return true
		}
	}
	return false
}

func loadPythonGrammarJSONForTest(t *testing.T) *Grammar {
	t.Helper()

	candidates := []string{
		"/tmp/python-locked-26855ea/src/grammar.json",
		"/tmp/grammar_parity/python/src/grammar.json",
		".parity_seed/python/src/grammar.json",
		"../.parity_seed/python/src/grammar.json",
	}
	jsonPath := ""
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			jsonPath = candidate
			break
		}
	}
	if jsonPath == "" {
		t.Skip("Python grammar.json not available")
	}

	source, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Skipf("Python grammar.json not available: %v", err)
	}

	gram, err := ImportGrammarJSON(source)
	if err != nil {
		t.Fatalf("import Python grammar.json: %v", err)
	}
	return gram
}
