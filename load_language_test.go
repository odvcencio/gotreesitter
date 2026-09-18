package gotreesitter

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"testing"
)

func TestLoadLanguageLegacyLexState(t *testing.T) {
	// This shape predates AcceptEOF. Gob must leave the new field false.
	type legacyLexState struct {
		AcceptToken  Symbol
		Default, EOF int
		Transitions  []LexTransition
	}
	legacy := struct{ LexStates []legacyLexState }{
		LexStates: []legacyLexState{
			{Default: -1, EOF: 1, Transitions: []LexTransition{{Lo: 'a', Hi: 'a', NextState: 2}}},
			{Default: -1, EOF: -1},
			{AcceptToken: 1, Default: -1, EOF: -1},
		},
	}
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	if err := gob.NewEncoder(gzw).Encode(legacy); err != nil {
		t.Fatal(err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadLanguage(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	for i, state := range loaded.LexStates {
		if state.AcceptEOF {
			t.Fatalf("legacy state %d acquired end acceptance", i)
		}
	}
	lex := NewLexer(loaded.LexStates, []byte("a"))
	if token := lex.Next(0); token.Symbol != 1 || token.EndByte != 1 {
		t.Fatalf("legacy text token=%+v", token)
	}
	if token := lex.Next(0); token.Symbol != 0 || token.StartByte != 1 || token.EndByte != 1 {
		t.Fatalf("legacy end token=%+v", token)
	}
}

func TestFullParseAcceptedErrorRetryProfileLanguageBlobRoundTrip(t *testing.T) {
	want := FullParseAcceptedErrorRetryProfile{
		SkipCompleteAcceptedErrorRetry:         true,
		SkipCompleteMinSourceBytes:             2 * 1024,
		ReuseCleanWideForWideRetry:             true,
		ReuseCleanWideMinSourceBytes:           128 * 1024,
		GSSConvergenceAcceptedErrorMergePerKey: 12,
		SkipFreshCompleteAcceptedErrorRetry:    true,
	}
	lang := &Language{
		Name:                               "accepted_error_retry_profile_round_trip",
		FullParseAcceptedErrorRetryProfile: want,
	}

	blob, err := EncodeLanguageBlob(lang)
	if err != nil {
		t.Fatalf("EncodeLanguageBlob: %v", err)
	}
	decoded, err := LoadLanguage(blob)
	if err != nil {
		t.Fatalf("LoadLanguage: %v", err)
	}
	if got := decoded.FullParseAcceptedErrorRetryProfile; got != want {
		t.Fatalf("FullParseAcceptedErrorRetryProfile = %+v, want %+v", got, want)
	}
}

func TestFullParseGSSConvergenceLanguageBlobRoundTrip(t *testing.T) {
	lang := &Language{
		Name:                           "gss_convergence_round_trip",
		FullParseGSSConvergenceEnabled: true,
	}

	blob, err := EncodeLanguageBlob(lang)
	if err != nil {
		t.Fatalf("EncodeLanguageBlob: %v", err)
	}
	decoded, err := LoadLanguage(blob)
	if err != nil {
		t.Fatalf("LoadLanguage: %v", err)
	}
	if !decoded.FullParseGSSConvergenceEnabled {
		t.Fatal("FullParseGSSConvergenceEnabled = false, want true")
	}
}

func TestLoadLanguageInfersGeneratedRepeatAuxMetadata(t *testing.T) {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	lang := &Language{
		TokenCount:  2,
		SymbolNames: []string{"end", "token_repeat1", "module_repeat1", "visible_repeat2", "named_repeat3"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Named: true},
			{},
			{},
			{Visible: true},
			{Named: true},
		},
	}
	if err := gob.NewEncoder(gzw).Encode(lang); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	loaded, err := LoadLanguage(buf.Bytes())
	if err != nil {
		t.Fatalf("LoadLanguage: %v", err)
	}
	if !loaded.SymbolMetadata[2].GeneratedRepeatAux {
		t.Fatal("LoadLanguage did not infer GeneratedRepeatAux for invisible anonymous module_repeat1")
	}
	if loaded.SymbolMetadata[1].GeneratedRepeatAux {
		t.Fatal("LoadLanguage marked terminal repeat-like symbol GeneratedRepeatAux")
	}
	if loaded.SymbolMetadata[3].GeneratedRepeatAux {
		t.Fatal("LoadLanguage marked visible repeat-like symbol GeneratedRepeatAux")
	}
	if loaded.SymbolMetadata[4].GeneratedRepeatAux {
		t.Fatal("LoadLanguage marked named repeat-like symbol GeneratedRepeatAux")
	}
}
