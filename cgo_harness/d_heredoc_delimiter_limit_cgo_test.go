//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestDHeredocDelimiterLimitCOracle pins the scanner fix ported from upstream
// tree-sitter-d commit 7c8c31c ("prevent heredoc delimiter stack buffer
// overflow (#64)"), part of the 64f27931b4e6 grammar bump, against the real
// C tree-sitter oracle.
//
// The upstream C scanner caps a heredoc string's identifier delimiter
// (q"IDENT ... IDENT") at 256 characters and rejects a still-unterminated
// delimiter at that length, instead of the pre-fix behavior of writing past
// a fixed-size C array for a long enough delimiter (CWE-787). This test
// confirms the C oracle at the new commit accepts a 256-character delimiter
// cleanly and rejects a 257-character delimiter, and that gotreesitter's
// ported Go scanner agrees at both boundaries.
func TestDHeredocDelimiterLimitCOracle(t *testing.T) {
	tests := []struct {
		name      string
		delimLen  int
		wantError bool
	}{
		{name: "at limit", delimLen: 256, wantError: false},
		{name: "one over limit", delimLen: 257, wantError: true},
	}

	cLanguage, err := ParityCLanguage("d")
	if err != nil {
		t.Skipf("C d oracle unavailable: %v", err)
	}
	goLanguage := grammars.DLanguage()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ident := strings.Repeat("a", test.delimLen)
			source := []byte(fmt.Sprintf("void f() { auto s = q\"%s\nhello\n%s\"; }\n", ident, ident))

			cTree := compactT3ParseC(t, cLanguage, source)
			defer cTree.Close()
			cRoot := cTree.RootNode()
			if cRoot == nil {
				t.Fatal("d C oracle returned no root")
			}
			if got := cRoot.HasError(); got != test.wantError {
				t.Fatalf("d C oracle delimLen=%d has_error=%v, want %v:\n%s", test.delimLen, got, test.wantError, dumpCTree(cRoot, 0))
			}

			goParser := gotreesitter.NewParser(goLanguage)
			goTree, err := goParser.Parse(source)
			if err != nil {
				t.Fatalf("d Go parse: %v", err)
			}
			defer goTree.Release()
			goRoot := goTree.RootNode()
			if goRoot == nil {
				t.Fatal("d Go parse returned no root")
			}
			if got := goRoot.HasError(); got != test.wantError {
				t.Fatalf("d Go delimLen=%d has_error=%v, want %v:\n%s", test.delimLen, got, test.wantError, dumpGoTree(goRoot, goLanguage, 0))
			}

			if test.wantError {
				// Error-recovery structure legitimately differs in shape
				// between the two engines once an error is present (see
				// the ocaml and r C-oracle twins for the same convention);
				// the has_error agreement above is the scanner-level claim
				// this port makes.
				return
			}

			var errs []string
			compareNodes(goRoot, goLanguage, cRoot, "root", &errs)
			if len(errs) != 0 {
				t.Fatalf(
					"d Go/C heredoc-delimiter tree diverged at delimLen=%d:\n%s\n\nGo: %s\n%s\nC: %s\n%s",
					test.delimLen,
					joinTopErrors(errs),
					goRoot.SExpr(goLanguage),
					dumpGoTree(goRoot, goLanguage, 0),
					cRoot.ToSexp(),
					dumpCTree(cRoot, 0),
				)
			}
		})
	}
}
