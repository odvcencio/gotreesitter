//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// The COBOL scanner picks tokens by code-point column, so every COBOL string
// token records C tree-sitter's Subtree.depends_on_column bit. These cases
// pin the two guards in C's ts_subtree_edit (lib/src/subtree.c) against the
// locked C oracle:
//
//   - the child guard, exercised by an insert that moves byte columns;
//   - the parent guard, exercised by a replacement that keeps every byte
//     column but changes the code-point column.
//
// The test checks node by node that every subtree C marks changed is also
// marked changed here, and then compares the incremental result against a
// fresh parse in both runtimes.
const cobolColumnParityPrefix = "       IDENTIFICATION DIVISION.\n" +
	"       PROGRAM-ID. A.\n" +
	"       PROCEDURE DIVISION.\n"

type cobolColumnParityCase struct {
	name         string
	originalTail string
	editedTail   string
	startColumn  uint
	oldEndColumn uint
	newEndColumn uint
}

func TestCobolColumnDependencyIncrementalParity(t *testing.T) {
	goLang := grammars.CobolLanguage()
	cLang, err := ParityCLanguage("cobol")
	if err != nil {
		t.Skipf("C parser unavailable: %v", err)
	}

	cases := []cobolColumnParityCase{
		{
			name:         "same_line_insert_moves_byte_columns",
			originalTail: "AAAAA* DISPLAY \"Z\".",
			editedTail:   "AAAAAA* DISPLAY \"Z\".",
			startColumn:  0,
			oldEndColumn: 0,
			newEndColumn: 1,
		},
		{
			name:         "same_line_replacement_keeps_byte_columns",
			originalTail: "éAAAA* DISPLAY \"Z\".",
			editedTail:   "ABAAAA* DISPLAY \"Z\".",
			startColumn:  0,
			oldEndColumn: 2,
			newEndColumn: 2,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			original := []byte(cobolColumnParityPrefix + tc.originalTail + "\n")
			edited := []byte(cobolColumnParityPrefix + tc.editedTail + "\n")
			base := uint(len(cobolColumnParityPrefix))
			const editRow = 3

			goParser := gotreesitter.NewParser(goLang)
			goOld, err := goParser.Parse(original)
			if err != nil {
				t.Fatalf("go parse original: %v", err)
			}
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatalf("C SetLanguage: %v", err)
			}
			cOld := cParser.Parse(original, nil)
			if cOld == nil || cOld.RootNode() == nil {
				t.Fatal("C parse original returned nil")
			}
			defer cOld.Close()

			var shapeErrs []string
			compareNodes(goOld.RootNode(), goLang, cOld.RootNode(), "original", &shapeErrs)
			if len(shapeErrs) > 0 {
				t.Fatalf("original trees already differ:\n%s", joinTopErrors(shapeErrs))
			}

			goOld.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(base + tc.startColumn),
				OldEndByte:  uint32(base + tc.oldEndColumn),
				NewEndByte:  uint32(base + tc.newEndColumn),
				StartPoint:  gotreesitter.Point{Row: editRow, Column: uint32(tc.startColumn)},
				OldEndPoint: gotreesitter.Point{Row: editRow, Column: uint32(tc.oldEndColumn)},
				NewEndPoint: gotreesitter.Point{Row: editRow, Column: uint32(tc.newEndColumn)},
			})
			cOld.Edit(&sitter.InputEdit{
				StartByte:      base + tc.startColumn,
				OldEndByte:     base + tc.oldEndColumn,
				NewEndByte:     base + tc.newEndColumn,
				StartPosition:  sitter.Point{Row: editRow, Column: tc.startColumn},
				OldEndPosition: sitter.Point{Row: editRow, Column: tc.oldEndColumn},
				NewEndPosition: sitter.Point{Row: editRow, Column: tc.newEndColumn},
			})

			var changeErrs []string
			compareHasChanges(goOld.RootNode(), goLang, cOld.RootNode(), "root", &changeErrs)
			if len(changeErrs) > 0 {
				t.Fatalf("this runtime under-invalidates against the C oracle after the edit:\n%s", joinTopErrors(changeErrs))
			}

			goIncremental, err := goParser.ParseIncremental(edited, goOld)
			if err != nil {
				t.Fatalf("go incremental parse: %v", err)
			}
			cIncremental := cParser.Parse(edited, cOld)
			if cIncremental == nil || cIncremental.RootNode() == nil {
				t.Fatal("C incremental parse returned nil")
			}
			defer cIncremental.Close()

			var incrErrs []string
			compareNodes(goIncremental.RootNode(), goLang, cIncremental.RootNode(), "incremental", &incrErrs)
			if len(incrErrs) > 0 {
				t.Fatalf("incremental trees differ:\n%s\n\ngo:\n%s\n\nc:\n%s",
					joinTopErrors(incrErrs),
					dumpGoTree(goIncremental.RootNode(), goLang, 0),
					dumpCTree(cIncremental.RootNode(), 0))
			}

			goFresh, err := goParser.Parse(edited)
			if err != nil {
				t.Fatalf("go fresh parse: %v", err)
			}
			if got, want := goIncremental.RootNode().SExpr(goLang), goFresh.RootNode().SExpr(goLang); got != want {
				t.Fatalf("go incremental parse differs from a fresh parse\n incremental: %s\n fresh:       %s", got, want)
			}
		})
	}
}

// TestHaskellLayoutColumnDependencyIncrementalParity covers the second
// column-sensitive scanner family. The Haskell layout scanner reads the
// column to place virtual braces, so one inserted space at the start of a do
// block statement changes the block structure. The test pins the changed
// bits and the incremental result against the locked C oracle.
func TestHaskellLayoutColumnDependencyIncrementalParity(t *testing.T) {
	goLang := grammars.HaskellLanguage()
	cLang, err := ParityCLanguage("haskell")
	if err != nil {
		t.Skipf("C parser unavailable: %v", err)
	}
	original := []byte("module M where\nf = do\n  x\n  y\n")
	edited := []byte("module M where\nf = do\n   x\n  y\n")
	at := uint(len("module M where\nf = do\n"))

	goParser := gotreesitter.NewParser(goLang)
	goOld, err := goParser.Parse(original)
	if err != nil {
		t.Fatalf("go parse original: %v", err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatalf("C SetLanguage: %v", err)
	}
	cOld := cParser.Parse(original, nil)
	if cOld == nil || cOld.RootNode() == nil {
		t.Fatal("C parse original returned nil")
	}
	defer cOld.Close()

	var shapeErrs []string
	compareNodes(goOld.RootNode(), goLang, cOld.RootNode(), "original", &shapeErrs)
	if len(shapeErrs) > 0 {
		t.Fatalf("original trees already differ:\n%s", joinTopErrors(shapeErrs))
	}

	goOld.Edit(gotreesitter.InputEdit{
		StartByte:   uint32(at),
		OldEndByte:  uint32(at),
		NewEndByte:  uint32(at + 1),
		StartPoint:  gotreesitter.Point{Row: 2, Column: 0},
		OldEndPoint: gotreesitter.Point{Row: 2, Column: 0},
		NewEndPoint: gotreesitter.Point{Row: 2, Column: 1},
	})
	cOld.Edit(&sitter.InputEdit{
		StartByte:      at,
		OldEndByte:     at,
		NewEndByte:     at + 1,
		StartPosition:  sitter.Point{Row: 2, Column: 0},
		OldEndPosition: sitter.Point{Row: 2, Column: 0},
		NewEndPosition: sitter.Point{Row: 2, Column: 1},
	})

	var changeErrs []string
	compareHasChanges(goOld.RootNode(), goLang, cOld.RootNode(), "root", &changeErrs)
	if len(changeErrs) > 0 {
		t.Fatalf("this runtime under-invalidates against the C oracle after the edit:\n%s", joinTopErrors(changeErrs))
	}

	goIncremental, err := goParser.ParseIncremental(edited, goOld)
	if err != nil {
		t.Fatalf("go incremental parse: %v", err)
	}
	cIncremental := cParser.Parse(edited, cOld)
	if cIncremental == nil || cIncremental.RootNode() == nil {
		t.Fatal("C incremental parse returned nil")
	}
	defer cIncremental.Close()

	var incrErrs []string
	compareNodes(goIncremental.RootNode(), goLang, cIncremental.RootNode(), "incremental", &incrErrs)
	if len(incrErrs) > 0 {
		t.Fatalf("incremental trees differ:\n%s\n\ngo:\n%s\n\nc:\n%s",
			joinTopErrors(incrErrs),
			dumpGoTree(goIncremental.RootNode(), goLang, 0),
			dumpCTree(cIncremental.RootNode(), 0))
	}
}

// compareHasChanges walks both trees in lockstep and reports every node that
// C marks changed while this runtime does not.
//
// The check runs in one direction on purpose. C decides has_changes on its
// own subtree hierarchy, which keeps the hidden wrappers this runtime folds
// away, and it rebases the edit into every frame. This runtime therefore
// marks a superset of C's nodes on the edited line. A superset costs reparse
// work and never costs correctness; a subset would let a stale
// column-dependent token survive, which is the defect these tests guard.
func compareHasChanges(goNode *gotreesitter.Node, goLang *gotreesitter.Language, cNode *sitter.Node, path string, errs *[]string) {
	if goNode == nil || cNode == nil {
		if (goNode == nil) != (cNode == nil) {
			*errs = append(*errs, fmt.Sprintf("%s: nil mismatch go=%v c=%v", path, goNode == nil, cNode == nil))
		}
		return
	}
	if cNode.HasChanges() && !goNode.HasChanges() {
		*errs = append(*errs, fmt.Sprintf("%s: C marks changed, go does not (type=%q bytes=[%d-%d])",
			path, goNode.Type(goLang), goNode.StartByte(), goNode.EndByte()))
	}
	goChildren := goNode.ChildCount()
	cChildren := int(cNode.ChildCount())
	if goChildren != cChildren {
		*errs = append(*errs, fmt.Sprintf("%s: ChildCount go=%d c=%d", path, goChildren, cChildren))
		return
	}
	for i := 0; i < goChildren; i++ {
		compareHasChanges(goNode.Child(i), goLang, cNode.Child(uint(i)), fmt.Sprintf("%s[%d]", path, i), errs)
	}
}
