//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestJavaScriptStrayQuoteKeepsDeclarations(t *testing.T) {
	for _, tc := range []struct {
		name         string
		declarations int
	}{{"single", 1}, {"reported", 40}} {
		t.Run(tc.name, func(t *testing.T) {
			var source strings.Builder
			for i := 0; i < tc.declarations; i++ {
				fmt.Fprintf(&source, "function fn%d(a, b) {\n\tvar x%d = a + b;\n\treturn x%d;\n}\n\n", i, i, i)
			}
			broken := []byte(strings.Replace(source.String(), "x0", "\"x0", 1))
			lang := grammars.JavascriptLanguage()
			got, err := gts.NewParser(lang).Parse(broken)
			if err != nil || got == nil {
				t.Fatalf("public Parse: %v", err)
			}
			defer got.Release()
			want := parseLockedC(t, "javascript", broken)
			actual, err := benchfixtures.InspectGoTree(got.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			expected, err := COracleDeepDigest(want)
			if err != nil {
				t.Fatal(err)
			}
			if actual.SHA256 != expected {
				t.Fatalf("tree differs from pinned C: %+v", FirstDivergenceDumpV1(got.RootNode(), lang, want.RootNode()))
			}
		})
	}
}
