//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestUnfinishedFinalLineMatchesLockedC pins that the default Go route and
// pinned C agree on documents that end in an unfinished line: equal trees and
// equal has-error. The test asserts equality only. The has-error value that
// pinned C gives for each row is recorded in a comment for the reader.
func TestUnfinishedFinalLineMatchesLockedC(t *testing.T) {
	const (
		plain = "--- a/f\n+++ b/f\n@@ -1 +1 @@\n-a\n+b\n"
		hunk  = "@@ -1 +1 @@\n-a\n+b\n"
		git   = "diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1 +1 @@\n-a\n+b\n"
	)
	cases := []struct{ lang, name, source string }{
		{"diff", "file-header/newline-and-one-space", plain + " "},        // C has-error: false
		{"diff", "file-header/added-line-no-newline", plain + "+c"},       // C has-error: false
		{"diff", "hunk-only/newline-and-one-space", hunk + " "},           // C has-error: false
		{"diff", "hunk-only/added-line-no-newline", hunk + "+c"},          // C has-error: false
		{"diff", "bare/added-line", "+c"},                                 // C has-error: false
		{"diff", "bare/newline-and-one-space", "\n "},                     // C has-error: false
		{"diff", "git-header/added-line-no-newline", git + "+c"},          // C has-error: true
		{"diff", "git-header/newline-and-one-space", git + " "},           // C has-error: true
		{"make", "rule-without-prerequisite", "all: dep\n\techo hi\ng:"},  // C has-error: true
		{"make", "rule-with-prerequisite", "all: dep\n\techo hi\ng: dep"}, // C has-error: true
	}
	for _, tc := range cases {
		t.Run(tc.lang+"/"+tc.name, func(t *testing.T) {
			goLang := grammars.DetectLanguageByName(tc.lang).Language()
			cLang, err := ParityCLanguage(tc.lang)
			if err != nil {
				t.Fatalf("C %s oracle unavailable: %v", tc.lang, err)
			}
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			cTree := cp.Parse([]byte(tc.source), nil)
			if cTree == nil {
				t.Fatal("C tree is nil")
			}
			defer cTree.Close()
			goTree, err := gotreesitter.NewParser(goLang).Parse([]byte(tc.source))
			if err != nil {
				t.Fatal(err)
			}
			defer goTree.Release()
			cErr, goErr := cTree.RootNode().HasError(), goTree.RootNode().HasError()
			t.Logf("has-error: C=%v Go=%v", cErr, goErr)
			if cErr != goErr {
				t.Fatalf("has-error differs: C=%v Go=%v", cErr, goErr)
			}
			assertLockedCTreeExactWithErrors(t, tc.name, goTree, goLang, cTree)
		})
	}
}
