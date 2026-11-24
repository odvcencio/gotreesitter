package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/odvcencio/gotreesitter/internal/tablebuild/ref"
)

// RunTablebuildRecords reads a generated parser.c with the strict reader and
// writes the record lines of internal/tablebuild/ref for it, one per line.
// tables is "L4" for a parser.c (layers L1, L2, L4, L5, L6) or "L3" for a
// parser-noopt.c (layer L3 only). The Language is built the way the blob path
// builds it (BuildLanguage, certified conflict policies, compaction), so the
// L6 records compare with a shipped blob.
func RunTablebuildRecords(inputPath, outputPath, name, tables string) error {
	source, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}
	g, err := ExtractGrammarStrict(string(source))
	if err != nil {
		return err
	}
	if name != "" {
		g.Name = name
	}
	lang := BuildLanguage(g)
	if err := applyCertifiedConflictPolicyProfiles(source, g, lang); err != nil {
		return fmt.Errorf("certified conflict policy: %w", err)
	}
	NewLanguageCompactor().CompactLanguage(lang)
	lines, err := ref.LanguageRecords(lang, ref.Extra{
		SymbolMap:              g.SymbolMap,
		PrimaryStateIDs:        g.PrimaryStateIDs,
		MaxAliasSequenceLength: g.MaxAliasSeqLength,
	}, tables)
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}
