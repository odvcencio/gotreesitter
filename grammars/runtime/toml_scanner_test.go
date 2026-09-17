//go:build !grammar_subset || grammar_subset_toml

package grammarruntime

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// TestTomlScannerSymbolIDs guards the hardcoded external symbol ids in
// toml_scanner.go against grammar table drift.
func TestTomlScannerSymbolIDs(t *testing.T) {
	lang := TomlLanguage()
	want := map[gotreesitter.Symbol]string{
		tomlSymLineEndingOrEOF:            "_line_ending_or_eof",
		tomlSymMultilineBasicStrContent:   "_multiline_basic_string_content",
		tomlSymMultilineBasicStrEnd:       "_multiline_basic_string_end",
		tomlSymMultilineLiteralStrContent: "_multiline_literal_string_content",
		tomlSymMultilineLiteralStrEnd:     "_multiline_literal_string_end",
	}
	for sym, name := range want {
		if int(sym) >= len(lang.SymbolNames) {
			t.Fatalf("symbol %d out of range (%d symbols)", sym, len(lang.SymbolNames))
		}
		if got := lang.SymbolNames[sym]; got != name {
			t.Errorf("symbol %d: got %q want %q", sym, got, name)
		}
	}
	if got := len(lang.ExternalSymbols); got != 5 {
		t.Fatalf("external symbol count: got %d want 5", got)
	}
	order := []gotreesitter.Symbol{
		tomlSymLineEndingOrEOF,
		tomlSymMultilineBasicStrContent,
		tomlSymMultilineBasicStrEnd,
		tomlSymMultilineLiteralStrContent,
		tomlSymMultilineLiteralStrEnd,
	}
	for i, sym := range order {
		if lang.ExternalSymbols[i] != sym {
			t.Errorf("ExternalSymbols[%d]: got %d want %d", i, lang.ExternalSymbols[i], sym)
		}
	}
}

// TestTomlMultilineStrings exercises the multiline string paths end-to-end:
// the scanner must terminate `”'`/`"""` strings, treat embedded single and
// double delimiter runs as content, and keep simple documents error-free.
func TestTomlMultilineStrings(t *testing.T) {
	lang := TomlLanguage()
	for name, src := range map[string]string{
		"literal":           "a = '''\nline1\nline2\n'''\n",
		"literal_quotes":    "a = '''it's, ''nested''\n'''\n",
		"basic":             "b = \"\"\"\nq\\t\n\"\"\"\n",
		"basic_quotes":      "b = \"\"\"say \"hi\" twice \"\"\n\"\"\"\n",
		"black_style":       "x = '''\n(\n  third-party/\n)\n'''\n",
		"inline_then_multi": "t = { v = \"3.8\" }\ns = '''\nbody\n'''\n",
	} {
		p := gotreesitter.NewParser(lang)
		tree, err := p.Parse([]byte(src))
		if err != nil {
			t.Fatalf("%s: parse error: %v", name, err)
		}
		root := tree.RootNode()
		if root.Type(lang) != "document" || root.HasError() {
			t.Errorf("%s: root=%s hasError=%v (want clean document)\nsrc: %q",
				name, root.Type(lang), root.HasError(), src)
		}
		tree.Release()
	}
}
