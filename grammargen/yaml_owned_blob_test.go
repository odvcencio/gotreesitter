package grammargen

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestYAMLOwnedGrammarGeneratesCompactBlob(t *testing.T) {
	grammar := YAMLGrammar()
	if !grammar.PreferPreciseExternalLexStates {
		t.Fatal("YAML grammar must preserve precise external lexer states")
	}
	if !grammar.CompactParseStates {
		t.Fatal("YAML grammar must minimize equivalent parser states")
	}

	lang, blob, err := GenerateLanguageAndBlob(grammar)
	if err != nil {
		t.Fatalf("generate owned YAML grammar: %v", err)
	}
	if !lang.GeneratedByGrammargen {
		t.Fatal("generated YAML language lacks grammargen provenance")
	}
	if lang.StateCount > 4000 {
		t.Fatalf("parser state count = %d, want at most 4000", lang.StateCount)
	}
	if got := len(lang.ExternalSymbols); got != 113 {
		t.Fatalf("external symbol count = %d, want 113", got)
	}
	if got := len(lang.ExternalLexStates); got < 100 {
		t.Fatalf("external lexer state count = %d, want at least 100", got)
	}

	baselineGrammar := YAMLGrammar()
	baselineGrammar.CompactParseStates = false
	baselineLang, baselineBlob, err := GenerateLanguageAndBlob(baselineGrammar)
	if err != nil {
		t.Fatalf("generate unminimized YAML grammar: %v", err)
	}
	if lang.StateCount >= baselineLang.StateCount {
		t.Fatalf("parser state count = %d, want fewer than baseline %d", lang.StateCount, baselineLang.StateCount)
	}
	if len(blob) >= len(baselineBlob) {
		t.Fatalf("blob size = %d, want smaller than baseline %d", len(blob), len(baselineBlob))
	}
	t.Logf("YAML compact states %d -> %d; blob bytes %d -> %d", baselineLang.StateCount, lang.StateCount, len(baselineBlob), len(blob))

	_, repeatedBlob, err := GenerateLanguageAndBlob(YAMLGrammar())
	if err != nil {
		t.Fatalf("regenerate owned YAML grammar: %v", err)
	}
	if !bytes.Equal(blob, repeatedBlob) {
		t.Fatal("owned YAML grammar generated a non-deterministic blob")
	}

	// Compare decoded tables, not raw bytes. encoding/gob assigns each
	// concrete struct type's wire type ID from a process-global counter the
	// first time that type crosses any Encoder in this test binary, so the
	// shipped yaml.bin (encoded by a past, unrelated process) can differ
	// byte-for-byte from a freshly regenerated blob even when every decoded
	// field is identical. See TestGrammargenOwnedBlobsAreReproducible's
	// doc comment in blob_reproducibility_test.go for the confirmed repro.
	shippedBytes, err := os.ReadFile(filepath.Join("..", "grammars", "grammar_blobs", "yaml.bin"))
	if err != nil {
		t.Fatalf("read shipped YAML blob: %v", err)
	}
	gotLang, err := decodeLanguageBlob(blob)
	if err != nil {
		t.Fatalf("decode regenerated YAML blob: %v", err)
	}
	wantLang, err := decodeLanguageBlob(shippedBytes)
	if err != nil {
		t.Fatalf("decode shipped YAML blob: %v", err)
	}
	if !reflect.DeepEqual(gotLang, wantLang) {
		t.Fatal("shipped YAML blob does not match the compact generated blob (decoded Language differs)")
	}
}
