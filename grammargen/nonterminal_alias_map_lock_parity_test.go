package grammargen

import (
	"os"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// Alias metadata depends on normalized productions, before LR and lexer table
// construction. Comparing it directly keeps these locked-source regressions
// small while checking every row and alias target extracted from C.
func lockedNonTerminalAliasMapParity(t *testing.T, name, subdir string) {
	t.Helper()
	path := aliasMapGrammarJSONPathForTest(t, name, name, subdir)
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	grammar, err := ImportGrammarJSON(source)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := Normalize(grammar)
	if err != nil {
		t.Fatal(err)
	}
	generated := &gotreesitter.Language{SymbolCount: uint32(len(normalized.Symbols))}
	for _, symbol := range normalized.Symbols {
		generated.SymbolNames = append(generated.SymbolNames, symbol.Name)
		generated.SymbolMetadata = append(generated.SymbolMetadata, gotreesitter.SymbolMetadata{
			Name: symbol.Name, Named: symbol.Named, Visible: symbol.Visible,
		})
	}
	compactProductionIDs(normalized)
	aliases := buildAliasSequences(generated, normalized)
	buildNonTerminalAliasMap(generated, normalized, aliases)
	reference := nonTerminalAliasMapByName(loadShippedBlob(t, name))
	if len(reference) == 0 {
		t.Fatal("locked C alias map is empty")
	}
	derived := nonTerminalAliasMapByName(generated)
	if diffs := diffNonTerminalAliasMapEntries(reference, derived); len(diffs) > 0 {
		t.Fatalf("generated %s alias map differs from locked C:\n%s", name, strings.Join(diffs, "\n"))
	}
}

func TestArduinoNonTerminalAliasMapParity(t *testing.T) {
	lockedNonTerminalAliasMapParity(t, "arduino", "src")
}
func TestCSharpNonTerminalAliasMapParity(t *testing.T) {
	lockedNonTerminalAliasMapParity(t, "c_sharp", "src")
}
func TestDNonTerminalAliasMapParity(t *testing.T) { lockedNonTerminalAliasMapParity(t, "d", "src") }
func TestKotlinNonTerminalAliasMapParity(t *testing.T) {
	lockedNonTerminalAliasMapParity(t, "kotlin", "src")
}
func TestOCamlNonTerminalAliasMapParity(t *testing.T) {
	lockedNonTerminalAliasMapParity(t, "ocaml", "grammars/ocaml/src")
}
func TestOrgNonTerminalAliasMapParity(t *testing.T) { lockedNonTerminalAliasMapParity(t, "org", "src") }
func TestPerlNonTerminalAliasMapParity(t *testing.T) {
	lockedNonTerminalAliasMapParity(t, "perl", "src")
}
func TestYAMLNonTerminalAliasMapParity(t *testing.T) {
	lockedNonTerminalAliasMapParity(t, "yaml", "src")
}
