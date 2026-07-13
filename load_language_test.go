package gotreesitter

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"testing"
)

func TestFullParseAcceptedErrorRetryProfileLanguageBlobRoundTrip(t *testing.T) {
	want := FullParseAcceptedErrorRetryProfile{
		SkipCompleteAcceptedErrorRetry: true,
		SkipCompleteMinSourceBytes:     2 * 1024,
		ReuseCleanWideForWideRetry:     true,
		ReuseCleanWideMinSourceBytes:   128 * 1024,
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
