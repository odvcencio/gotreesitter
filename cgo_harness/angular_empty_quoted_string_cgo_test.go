//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestAngularEmptyQuotedStringCOracle pins the scanner fix ported from
// upstream tree-sitter-angular commit 6a31043 ("fix: empty quoted attribute
// values on multiline no longer brakes parsing"), part of the 38a8014ed545
// grammar bump, against the real C tree-sitter oracle.
//
// The upstream C scanner adds an EMPTY_QUOTED_STRING external token so a
// property binding written as `[property]=""` on its own line lexes as one
// token, instead of two adjacent double-quote tokens with nothing between
// them. This test confirms:
//
//   - the C oracle at the OLD pinned commit (f0d0685701b7) still reports an
//     ERROR node for the multiline construct (negative control: the bug is
//     real and this test would have caught a no-op "fix");
//   - the C oracle at the NEW pinned commit (38a8014ed545, read from
//     grammars/languages.lock) parses it cleanly;
//   - gotreesitter's ported Go scanner agrees with the new C oracle, both on
//     the absence of an error and on full tree shape.
func TestAngularEmptyQuotedStringCOracle(t *testing.T) {
	const source = "<span\n  [property]=\"\"\n></span>\n"

	t.Run("old_ref_has_error", func(t *testing.T) {
		oldEntry := parityLockEntry{
			Name:    "angular_old_f0d0685",
			RepoURL: "https://github.com/dlvandenberg/tree-sitter-angular",
			Commit:  "f0d0685701b70883fa2dfe94ee7dc27965cab841",
			Subdir:  "src",
		}
		ref, err := buildParityCRef(t.TempDir(), "", oldEntry)
		if err != nil {
			t.Skipf("build old-ref angular C oracle: %v", err)
		}
		cTree := compactT3ParseC(t, ref.lang, []byte(source))
		defer cTree.Close()
		if !cTree.RootNode().HasError() {
			t.Fatalf("old-ref (f0d0685701b7) angular C oracle has_error=false for multiline empty-quoted attribute, want true (negative control failed):\n%s", dumpCTree(cTree.RootNode(), 0))
		}
	})

	t.Run("new_ref_clean_and_matches_go", func(t *testing.T) {
		cLanguage, err := ParityCLanguage("angular")
		if err != nil {
			t.Skipf("C angular oracle unavailable: %v", err)
		}
		goLanguage := grammars.AngularLanguage()

		cTree := compactT3ParseC(t, cLanguage, []byte(source))
		defer cTree.Close()
		cRoot := cTree.RootNode()
		if cRoot.HasError() {
			t.Fatalf("angular C oracle (38a8014ed545) has_error=true for multiline empty-quoted attribute, want false:\n%s", dumpCTree(cRoot, 0))
		}

		goParser := gotreesitter.NewParser(goLanguage)
		goTree, err := goParser.Parse([]byte(source))
		if err != nil {
			t.Fatalf("angular Go parse: %v", err)
		}
		defer goTree.Release()
		goRoot := goTree.RootNode()
		if goRoot == nil {
			t.Fatal("angular Go parse returned no root")
		}
		if goRoot.HasError() {
			t.Fatalf("angular Go has_error=true for multiline empty-quoted attribute, want false:\n%s", dumpGoTree(goRoot, goLanguage, 0))
		}

		var errs []string
		compareNodes(goRoot, goLanguage, cRoot, "root", &errs)
		if len(errs) != 0 {
			t.Fatalf(
				"angular Go/C empty-quoted-string tree diverged:\n%s\n\nGo: %s\n%s\nC: %s\n%s",
				joinTopErrors(errs),
				goRoot.SExpr(goLanguage),
				dumpGoTree(goRoot, goLanguage, 0),
				cRoot.ToSexp(),
				dumpCTree(cRoot, 0),
			)
		}
	})
}
