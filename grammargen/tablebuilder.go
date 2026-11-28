package grammargen

import (
	"fmt"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/tablebuild"
)

// TableBuilderTreeSitter selects internal/tablebuild, which builds the tables
// the way the tree-sitter generator (release v0.27.1) builds them, in place
// of the legacy passes (Normalize, the LR builder and the lexer builder).
// The grammar's JSON text (ExportGrammarJSON) is the whole input: the
// builder reads no other field of Grammar, and a build never falls back to the
// legacy passes.
const TableBuilderTreeSitter = "tree-sitter"

// tableBuilderABI is the language ABI that the tree-sitter builder writes.
const tableBuilderABI = tablebuild.ABIVersionMax

// generateWithTableBuilder is generateWithReportCtx for a grammar whose
// TableBuilder is set. It fails on a builder that it does not know.
func generateWithTableBuilder(g *Grammar, opts reportBuildOptions) (*GenerateReport, error) {
	if g.TableBuilder != TableBuilderTreeSitter {
		return nil, fmt.Errorf("unknown table builder %q", g.TableBuilder)
	}
	text, err := ExportGrammarJSON(g)
	if err != nil {
		return nil, fmt.Errorf("export grammar.json: %w", err)
	}
	lang, err := tablebuild.BuildLanguage(text, g.Name, tableBuilderABI)
	if err != nil {
		return nil, fmt.Errorf("tablebuild: %w", err)
	}
	lang.WantsForest = g.WantsForest
	lang.Metadata = gotreesitter.LanguageMetadata{MajorVersion: g.Version[0], MinorVersion: g.Version[1], PatchVersion: g.Version[2]}
	finishBlobLanguage(lang)
	// The runtime's recovery gate reads the tables. The blob records only the
	// capability, as cmd/ts2go does for a parser.c: the default-on decision
	// depends on the scanner that the registry attaches, and the embedded
	// loader makes it again at load time (grammars/runtime/embedded_loader.go).
	gotreesitter.CertifyCRecoveryCostCompetition(lang)
	lang.CRecoveryCostCompetitionEnabledByDefault = false
	report := &GenerateReport{
		Language:    lang,
		SymbolCount: int(lang.SymbolCount),
		StateCount:  int(lang.StateCount),
		TokenCount:  int(lang.TokenCount),
	}
	if opts.includeBlob {
		blob, err := encodeLanguageBlob(lang)
		if err != nil {
			return nil, fmt.Errorf("encode: %w", err)
		}
		report.Blob = blob
	}
	return report, nil
}

// finishBlobLanguage gives the Language of a table build the shape that
// cmd/ts2go gives the Language of a parser.c before it encodes a blob, so
// that a blob built from the same tables decodes to the same Language:
//
//   - adjacent lex ranges with one target merge (cmd/ts2go/compact.go: fn
//     compactTransitions; the C text spells each range apart, and the merge
//     changes no step);
//   - generated repeat helpers are marked (InferGeneratedRepeatAuxMetadata,
//     which the loader also runs);
//   - the primary state ids are left out: no runtime code reads them (the C
//     emitter reads them from a Language that holds them, and the layout
//     check of internal/tablebuild compares them before this step).
func finishBlobLanguage(lang *gotreesitter.Language) {
	for i := range lang.LexStates {
		lang.LexStates[i].Transitions = mergeLexRanges(lang.LexStates[i].Transitions)
	}
	for i := range lang.KeywordLexStates {
		lang.KeywordLexStates[i].Transitions = mergeLexRanges(lang.KeywordLexStates[i].Transitions)
	}
	gotreesitter.InferGeneratedRepeatAuxMetadata(lang)
	lang.PrimaryStateIDs = nil
}

func mergeLexRanges(ts []gotreesitter.LexTransition) []gotreesitter.LexTransition {
	if len(ts) == 0 {
		return ts
	}
	merged := make([]gotreesitter.LexTransition, 0, len(ts))
	for _, tr := range ts {
		if n := len(merged); n > 0 {
			prev := merged[n-1]
			if prev.NextState == tr.NextState && prev.Skip == tr.Skip && prev.Hi < tr.Lo && prev.Hi+1 == tr.Lo {
				merged[n-1].Hi = tr.Hi
				continue
			}
		}
		merged = append(merged, tr)
	}
	return merged
}
