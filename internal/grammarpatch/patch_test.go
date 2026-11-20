package grammarpatch

import (
	"slices"
	"testing"
)

func TestLookup(t *testing.T) {
	tests := []struct {
		name           string
		wantFile       string
		wantExtra      string
		wantRegenerate bool
		wantGenerator  string
		wantOK         bool
	}{
		{name: "make", wantFile: "tree-sitter-make-function-variables.patch", wantRegenerate: true, wantGenerator: "0.24.7", wantOK: true},
		{name: "typescript", wantFile: "tree-sitter-typescript-import-type.patch", wantRegenerate: true, wantOK: true},
		{name: "TSX", wantFile: "tree-sitter-typescript-import-type.patch", wantExtra: "tree-sitter-tsx-jsx-text-ampersand.patch", wantRegenerate: true, wantOK: true},
		{name: " yaml ", wantFile: "tree-sitter-yaml-multiline-quoted-scalars.patch", wantOK: true},
		{name: "go"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := Lookup(test.name)
			if ok != test.wantOK {
				t.Fatalf("Lookup(%q) found = %t, want %t", test.name, ok, test.wantOK)
			}
			if !ok {
				return
			}
			want := []string{test.wantFile}
			if test.wantExtra != "" {
				want = append(want, test.wantExtra)
			}
			if !slices.Equal(got.PatchFiles(), want) || got.RegenerateParser != test.wantRegenerate || got.GeneratorVersion != test.wantGenerator {
				t.Fatalf("Lookup(%q) = %+v", test.name, got)
			}
		})
	}
}
