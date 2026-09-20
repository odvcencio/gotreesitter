//go:build !grammar_subset || grammar_subset_toml

package grammarruntime

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// The hardcoded external symbol ids this test guarded were removed when
// toml_scanner.go moved to load-time symbol binding (ExternalScannerForLanguage);
// see TestTomlExternalScannerSpecMatchesBlob in load_bound_scanner_order_test.go
// for the equivalent positional-binding pin against the shipped blob.

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
