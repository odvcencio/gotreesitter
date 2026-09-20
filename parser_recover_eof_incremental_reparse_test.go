package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestPublishedRecoverEOFRootDoesNotPoisonNextIncrementalRoot is review
// finding B1/M2's regression receipt. A published recover_eof root
// (tryPublishCRecoverEOFRoot, parser_result_root_build.go) carries
// nodeFlagCompactRecoverEOF but never sets tree.compactMaterialized, because
// the classic GLR C-recovery port (cRecoverEOFAccept) produced it. Before
// this fix, newResultRootBuild's guard at parser_result_root_build.go:58
// checked compactRecoverEOFTreeMarked, which requires compactMaterialized
// and therefore never recognized this root: an edit that fixed the broken
// source then reparsed incrementally with the transient ERROR symbol framed
// as the expected root, silently nesting the correct grammar root under a
// stale "(ERROR (<grammar root> ...))" wrapper. HasError() on that wrapper
// reports false, so the corruption carries no error signal at all.
//
// dtd and powershell are both capable and on by default today
// (testdata/c_recovery_gate_fleet.json), and both are among the five
// grammars review finding B2 lists as reaching the recover_eof publish path
// for a short malformed input.
func TestPublishedRecoverEOFRootDoesNotPoisonNextIncrementalRoot(t *testing.T) {
	cases := []struct {
		name           string
		lang           func() *gts.Language
		broken         string
		fixed          string
		wantRootSymbol string
	}{
		{
			name:           "dtd",
			lang:           grammars.DtdLanguage,
			broken:         "{",
			fixed:          "<!ELEMENT a EMPTY>",
			wantRootSymbol: "extSubset",
		},
		{
			name:           "powershell",
			lang:           grammars.PowershellLanguage,
			broken:         "\"",
			fixed:          "$a = 1",
			wantRootSymbol: "program",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lang := tc.lang()

			firstParser := gts.NewParser(lang)
			brokenTree, err := firstParser.Parse([]byte(tc.broken))
			if err != nil {
				t.Fatalf("parse broken source: %v", err)
			}
			t.Cleanup(brokenTree.Release)
			brokenRoot := brokenTree.RootNode()
			if brokenRoot.Type(lang) != "ERROR" || brokenRoot.ChildCount() != 0 {
				t.Fatalf("broken-source root = type %q children %d, want a published childless ERROR root; sexp=%s",
					brokenRoot.Type(lang), brokenRoot.ChildCount(), brokenRoot.SExpr(lang))
			}

			brokenTree.Edit(gts.InputEdit{
				StartByte:  0,
				OldEndByte: uint32(len(tc.broken)),
				NewEndByte: uint32(len(tc.fixed)),
			})
			secondParser := gts.NewParser(lang)
			incrementalTree, err := secondParser.ParseIncremental([]byte(tc.fixed), brokenTree)
			if err != nil {
				t.Fatalf("incremental reparse: %v", err)
			}
			t.Cleanup(incrementalTree.Release)
			incrementalRoot := incrementalTree.RootNode()

			freshParser := gts.NewParser(lang)
			freshTree, err := freshParser.Parse([]byte(tc.fixed))
			if err != nil {
				t.Fatalf("fresh parse of fixed source: %v", err)
			}
			t.Cleanup(freshTree.Release)
			freshRoot := freshTree.RootNode()

			if freshRoot.Type(lang) != tc.wantRootSymbol {
				t.Fatalf("fresh parse root = %q, want %q; the fixture no longer matches the grammar",
					freshRoot.Type(lang), tc.wantRootSymbol)
			}
			if incrementalRoot.Type(lang) != tc.wantRootSymbol {
				t.Fatalf("incremental reparse root = %q, want %q (matching a fresh parse); sexp=%s",
					incrementalRoot.Type(lang), tc.wantRootSymbol, incrementalRoot.SExpr(lang))
			}
			if incrementalRoot.Type(lang) != freshRoot.Type(lang) {
				t.Fatalf("incremental reparse root %q does not match fresh parse root %q",
					incrementalRoot.Type(lang), freshRoot.Type(lang))
			}
		})
	}
}
