//go:build !grammar_subset || grammar_subset_python

package grammars

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars/internal/pythonruntime"
)

var pythonExternalScannerSpec = ExternalScannerSpec{
	Language:     "python",
	UpstreamRepo: "https://github.com/tree-sitter/tree-sitter-python",
	Externals: []string{
		"_newline",
		"_indent",
		"_dedent",
		"string_start",
		"_string_content",
		"escape_interpolation",
		"string_end",
		"comment",
		"]",
		")",
		"}",
		"except",
	},
}

func init() {
	RegisterExternalScannerSpec(pythonExternalScannerSpec)
}

// PythonExternalScanner shares its implementation with the standalone Python package.
type PythonExternalScanner struct {
	pythonruntime.PythonExternalScanner
}

func (PythonExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	scanner := pythonruntime.Bind(lang, func(lang *gotreesitter.Language, setSymbol func(int, gotreesitter.Symbol)) []int {
		return bindExternalScannerSpec(lang, pythonExternalScannerSpec, setSymbol)
	})
	return PythonExternalScanner{PythonExternalScanner: scanner}
}
