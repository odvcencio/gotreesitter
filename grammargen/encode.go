package grammargen

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/gob"
	"fmt"
	"io"

	"github.com/odvcencio/gotreesitter"
)

// Generate compiles a Grammar definition into a binary blob that
// gotreesitter can load via DecodeLanguageBlob / loadEmbeddedLanguage.
// LR(1) state splitting is always attempted; a rollback guard reverts to the
// plain LALR table if splitting does not reduce GLR conflicts.
func Generate(g *Grammar) ([]byte, error) {
	report, err := generateWithReport(g, reportBuildOptions{includeLanguage: true, includeBlob: true})
	if err != nil {
		return nil, err
	}
	return report.Blob, nil
}

// GenerateLanguage compiles a Grammar into a Language struct without encoding.
// LR(1) state splitting is always attempted; a rollback guard reverts to the
// plain LALR table if splitting does not reduce GLR conflicts.
func GenerateLanguage(g *Grammar) (*gotreesitter.Language, error) {
	return GenerateLanguageWithContext(context.Background(), g)
}

// GenerateLanguageAndBlob compiles a Grammar into both a Language and its
// serialized blob representation in a single generation pass.
func GenerateLanguageAndBlob(g *Grammar) (*gotreesitter.Language, []byte, error) {
	return GenerateLanguageAndBlobWithContext(context.Background(), g)
}

// GenerateLanguageWithContext is like GenerateLanguage but accepts a context
// for cancellation. When the context is cancelled, LR table construction and
// DFA building abort promptly, allowing the caller to reclaim memory that
// would otherwise be held by an orphaned goroutine.
func GenerateLanguageWithContext(ctx context.Context, g *Grammar) (*gotreesitter.Language, error) {
	report, err := generateWithReportCtx(ctx, g, reportBuildOptions{includeLanguage: true})
	if err != nil {
		return nil, err
	}
	return report.Language, nil
}

// GenerateLanguageForC compiles a Grammar into a Language struct suitable
// for C emission (EmitC / GenerateC). It disables buildLexDFA's cross-mode
// LexState minimization (see dfa_minimize.go): the C emitter's modeStartOf
// requires each lex mode's states to occupy a contiguous, increasing index
// block, an invariant minimization does not preserve. Every other Language
// consumer (the pure-Go runtime, the encoded blob) does not have that
// constraint and should keep using GenerateLanguage / Generate, which
// return the smaller, minimized tables.
func GenerateLanguageForC(g *Grammar) (*gotreesitter.Language, error) {
	report, err := generateWithReportCtx(context.Background(), g, reportBuildOptions{
		includeLanguage:         true,
		disableLexStateMinimize: true,
	})
	if err != nil {
		return nil, err
	}
	return report.Language, nil
}

// GenerateLanguageAndBlobWithContext is like GenerateLanguageAndBlob but
// accepts a context for cancellation.
func GenerateLanguageAndBlobWithContext(ctx context.Context, g *Grammar) (*gotreesitter.Language, []byte, error) {
	report, err := generateWithReportCtx(ctx, g, reportBuildOptions{includeLanguage: true, includeBlob: true})
	if err != nil {
		return nil, nil, err
	}
	return report.Language, report.Blob, nil
}

// allSymbolsSet returns a set containing all symbol IDs from the patterns.
func allSymbolsSet(patterns []TerminalPattern) map[int]bool {
	s := make(map[int]bool, len(patterns))
	for _, p := range patterns {
		s[p.SymbolID] = true
	}
	return s
}

// computeSkipExtras returns the set of extra symbol IDs that should be
// silently consumed (Skip=true in the DFA). Only invisible/anonymous extras
// are skipped. Visible extras like `comment` produce tree nodes.
func computeSkipExtras(ng *NormalizedGrammar) map[int]bool {
	skip := make(map[int]bool)
	for _, e := range ng.ExtraSymbols {
		if e > 0 && e < len(ng.Symbols) && !ng.Symbols[e].Visible {
			skip[e] = true
		}
	}
	return skip
}

// encodeLanguageBlob keeps grammargen's internal call sites stable while the
// root package owns the single blob-format encoder shared with ts2go.
func encodeLanguageBlob(lang *gotreesitter.Language) ([]byte, error) {
	return gotreesitter.EncodeLanguageBlob(lang)
}

// decodeLanguageBlob deserializes a legacy gob+gzip Language blob, a
// version-enveloped blob containing the LargeStateGotos trailer, or a
// version-header-wrapped blob (all written by encodeLanguageBlob). Envelope
// and header detection are cheap no-ops for a legacy blob that has neither.
//
// This intentionally does not delegate to gotreesitter.LoadLanguage: this
// package's reproducibility tests (TestGrammargenOwnedBlobsAreReproducible,
// TestYAMLOwnedGrammarGeneratesCompactBlob) compare two independently
// decoded *gotreesitter.Language values field-for-field to prove a
// regenerated blob's *content* matches a checked-in one. LoadLanguage stamps
// each decode with per-blob identity (GrammarBlobSHA256, BlobInfo) that
// legitimately differs between two separately encoded blobs of identical
// content -- gob's process-global type-ID counter alone can change a
// re-encoded blob's byte length -- which would make that comparison fail on
// identity metadata having nothing to do with grammar content. Keeping this
// decoder scoped to exactly what it decoded before (content, not identity)
// keeps that comparison meaningful. It still uses the same envelope and
// version-header unwrap functions LoadLanguage uses, so the wire format
// itself cannot drift between the two decoders.
func decodeLanguageBlob(data []byte) (*gotreesitter.Language, error) {
	enveloped, _, err := gotreesitter.UnwrapLanguageBlobVersionHeader(data)
	if err != nil {
		return nil, fmt.Errorf("decode language blob: %w", err)
	}
	compressed, expectsTrailer, err := gotreesitter.UnwrapLanguageBlobEnvelope(enveloped)
	if err != nil {
		return nil, fmt.Errorf("decode language blob: %w", err)
	}
	gzr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("open gzip: %w", err)
	}
	defer gzr.Close()

	// Decode from a fully-buffered *bytes.Reader, not gzr directly: gob's
	// Decoder can read ahead past its own message boundary on a streaming
	// reader, which would silently swallow the trailer bytes below before we
	// get a chance to read them.
	raw, err := io.ReadAll(gzr)
	if err != nil {
		return nil, fmt.Errorf("read gzip: %w", err)
	}

	var lang gotreesitter.Language
	br := bytes.NewReader(raw)
	if err := gob.NewDecoder(br).Decode(&lang); err != nil {
		return nil, fmt.Errorf("decode language blob: %w", err)
	}
	trailer, err := gotreesitter.DecodeLargeStateGotosTrailer(br)
	if err != nil {
		return nil, fmt.Errorf("decode language blob: %w", err)
	}
	if expectsTrailer && len(trailer) == 0 {
		return nil, fmt.Errorf("decode language blob: envelope requires a non-empty large-state-gotos trailer")
	}
	if !expectsTrailer && len(trailer) != 0 {
		return nil, fmt.Errorf("decode language blob: large-state-gotos trailer requires a versioned envelope")
	}
	if trailer != nil {
		lang.LargeStateGotos = trailer
	}
	return &lang, nil
}

func repairGeneratedCompatibilitySymbols(lang *gotreesitter.Language) {
	if lang == nil {
		return
	}
	switch lang.Name {
	case "dart":
		repairGeneratedCollapsedLeafTokenSymbol(lang, "nullable_type", "?")
		repairGeneratedCollapsedLeafTokenSymbol(lang, "null_literal", "null")
	}
}

func repairGeneratedCollapsedLeafTokenSymbol(lang *gotreesitter.Language, parentName, childName string) {
	if !generatedLanguageHasSymbolName(lang, parentName) || generatedLanguageHasSymbolName(lang, childName) {
		return
	}
	for len(lang.SymbolMetadata) < len(lang.SymbolNames) {
		lang.SymbolMetadata = append(lang.SymbolMetadata, gotreesitter.SymbolMetadata{})
	}
	lang.SymbolNames = append(lang.SymbolNames, childName)
	lang.SymbolMetadata = append(lang.SymbolMetadata, gotreesitter.SymbolMetadata{
		Name:    childName,
		Visible: true,
		Named:   false,
	})
	lang.SymbolCount = uint32(len(lang.SymbolNames))
}

func generatedLanguageHasSymbolName(lang *gotreesitter.Language, name string) bool {
	for _, symbolName := range lang.SymbolNames {
		if symbolName == name {
			return true
		}
	}
	return false
}
