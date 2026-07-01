package grammargen

import "testing"

// TestGrammarWantsForestBlobRoundTrip verifies that Grammar.WantsForest is
// plumbed onto the assembled Language and survives gob-blob encode/decode
// (grammargen.Generate / decodeLanguageBlob) — consumer grammars set this
// flag to opt a grammargen-generated Language into the GSS-forest fast path
// without forking gotreesitter's curated builtinForestDefaults allowlist.
func TestGrammarWantsForestBlobRoundTrip(t *testing.T) {
	g := CalcGrammar()
	g.WantsForest = true

	lang, blob, err := GenerateLanguageAndBlob(g)
	if err != nil {
		t.Fatalf("GenerateLanguageAndBlob failed: %v", err)
	}
	if !lang.WantsForest {
		t.Errorf("directly generated Language.WantsForest = false, want true")
	}

	decoded, err := decodeLanguageBlob(blob)
	if err != nil {
		t.Fatalf("decodeLanguageBlob failed: %v", err)
	}
	if !decoded.WantsForest {
		t.Errorf("blob round-trip Language.WantsForest = false, want true")
	}
}

// TestGrammarWantsForestDefaultFalse verifies the flag defaults to false when
// unset, so existing consumer grammars are unaffected until they opt in.
func TestGrammarWantsForestDefaultFalse(t *testing.T) {
	g := CalcGrammar()

	lang, blob, err := GenerateLanguageAndBlob(g)
	if err != nil {
		t.Fatalf("GenerateLanguageAndBlob failed: %v", err)
	}
	if lang.WantsForest {
		t.Errorf("unset Grammar.WantsForest should default to false, got true")
	}

	decoded, err := decodeLanguageBlob(blob)
	if err != nil {
		t.Fatalf("decodeLanguageBlob failed: %v", err)
	}
	if decoded.WantsForest {
		t.Errorf("blob round-trip of unset WantsForest should default to false, got true")
	}
}
