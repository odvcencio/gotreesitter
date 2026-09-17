//go:build !grammar_subset || grammar_subset_python

package grammars

import (
	"slices"
	"testing"
)

func TestPythonExternalScannerSpec(t *testing.T) {
	spec, ok := LookupExternalScannerSpec("python")
	if !ok {
		t.Fatal("missing python external scanner spec")
	}
	if got, want := spec.UpstreamRepo, "https://github.com/tree-sitter/tree-sitter-python"; got != want {
		t.Fatalf("python repo = %q, want %q", got, want)
	}
	if got, want := spec.Externals, []string{
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
	}; !slices.Equal(got, want) {
		t.Fatalf("python externals = %v, want %v", got, want)
	}
}
