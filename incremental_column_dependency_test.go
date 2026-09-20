package gotreesitter_test

import (
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// The COBOL scanner selects tokens by code-point column: a line comment at
// column 6, a line prefix comment below column 6, and a line suffix comment
// at column 72 or beyond (grammars/runtime/cobol_scanner.go). The string
// scanner reads the column too, so every COBOL string token depends on its
// column.
//
// These tests pin C tree-sitter's depends_on_column contract
// (lib/src/subtree.c, ts_subtree_edit): an edit earlier on the same line
// must mark a column-dependent token changed, even when that token begins
// after the edited bytes. Without the contract the edit walk shifts the
// later token and leaves it clean, so an incremental reparse may reuse a
// token whose column, and therefore whose identity, has moved.
const cobolColumnDependencyPrefix = "       IDENTIFICATION DIVISION.\n" +
	"       PROGRAM-ID. A.\n" +
	"       PROCEDURE DIVISION.\n"

// findNodeWithSpan returns the first node of the given type whose span
// matches, searching the whole tree.
func findNodeWithSpan(t *testing.T, lang *gotreesitter.Language, n *gotreesitter.Node, nodeType string) *gotreesitter.Node {
	t.Helper()
	if n == nil {
		return nil
	}
	if n.Type(lang) == nodeType {
		return n
	}
	for i := 0; i < n.ChildCount(); i++ {
		if found := findNodeWithSpan(t, lang, n.Child(i), nodeType); found != nil {
			return found
		}
	}
	return nil
}

// runCobolColumnDependencyCase parses origLine and editedLine as the fourth
// line of a COBOL program, applies edit to the original tree, and checks both
// the column-dependent node's changed bit and the incremental result.
func runCobolColumnDependencyCase(t *testing.T, origLine, editedLine string, edit gotreesitter.InputEdit) {
	t.Helper()
	lang := grammars.CobolLanguage()
	original := []byte(cobolColumnDependencyPrefix + origLine + "\n")
	edited := []byte(cobolColumnDependencyPrefix + editedLine + "\n")
	if len(origLine) != len(editedLine)-int(edit.NewEndByte-edit.OldEndByte) {
		t.Fatalf("edit delta does not match the edited line length")
	}

	parser := gotreesitter.NewParser(lang)
	oldTree, err := parser.Parse(original)
	if err != nil {
		t.Fatalf("parse original: %v", err)
	}
	freshTree, err := parser.Parse(edited)
	if err != nil {
		t.Fatalf("parse edited: %v", err)
	}
	originalSExpr := oldTree.RootNode().SExpr(lang)
	freshSExpr := freshTree.RootNode().SExpr(lang)
	if originalSExpr == freshSExpr {
		t.Fatalf("the edit must change the tree shape, got %s for both", freshSExpr)
	}

	base := uint32(len(cobolColumnDependencyPrefix))
	edit.StartByte += base
	edit.OldEndByte += base
	edit.NewEndByte += base
	oldTree.Edit(edit)

	columnDependent := findNodeWithSpan(t, lang, oldTree.RootNode(), "string")
	if columnDependent == nil {
		t.Fatalf("no string node in the original tree: %s", originalSExpr)
	}
	if columnDependent.StartByte() < edit.OldEndByte {
		t.Fatalf("string node starts at %d, which is not after the edit end %d",
			columnDependent.StartByte(), edit.OldEndByte)
	}
	if !columnDependent.HasChanges() {
		t.Fatalf("string node [%d,%d) reports no changes after a same-line edit at [%d,%d)",
			columnDependent.StartByte(), columnDependent.EndByte(), edit.StartByte, edit.OldEndByte)
	}

	incremental, err := parser.ParseIncremental(edited, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	if got := incremental.RootNode().SExpr(lang); got != freshSExpr {
		t.Fatalf("incremental parse differs from a fresh parse\n incremental: %s\n fresh:       %s", got, freshSExpr)
	}
}

// TestIncrementalColumnDependentTokenInvalidatedBySameLineInsert covers C's
// child guard: the inserted byte moves every byte column on the line, so a
// later column-dependent token on that line must be marked changed. The
// insert moves the '*' indicator into column 6, which turns the whole line
// into a comment.
func TestIncrementalColumnDependentTokenInvalidatedBySameLineInsert(t *testing.T) {
	runCobolColumnDependencyCase(t,
		"AAAAA* DISPLAY \"Z\".",
		"AAAAAA* DISPLAY \"Z\".",
		gotreesitter.InputEdit{
			StartByte:   0,
			OldEndByte:  0,
			NewEndByte:  1,
			StartPoint:  gotreesitter.Point{Row: 3, Column: 0},
			OldEndPoint: gotreesitter.Point{Row: 3, Column: 0},
			NewEndPoint: gotreesitter.Point{Row: 3, Column: 1},
		})
}

// TestIncrementalColumnDependentTokenInvalidatedBySameLineByteEdit covers C's
// parent guard. The replacement keeps every byte offset and every byte
// column, so the child guard cannot fire. It swaps one two-byte rune for two
// ASCII bytes, which adds one code point to the line and moves the '*'
// indicator into code-point column 6.
func TestIncrementalColumnDependentTokenInvalidatedBySameLineByteEdit(t *testing.T) {
	runCobolColumnDependencyCase(t,
		"éAAAA* DISPLAY \"Z\".",
		"ABAAAA* DISPLAY \"Z\".",
		gotreesitter.InputEdit{
			StartByte:   0,
			OldEndByte:  2,
			NewEndByte:  2,
			StartPoint:  gotreesitter.Point{Row: 3, Column: 0},
			OldEndPoint: gotreesitter.Point{Row: 3, Column: 2},
			NewEndPoint: gotreesitter.Point{Row: 3, Column: 2},
		})
}

// TestIncrementalHaskellLayoutIndentEditMatchesFreshParse covers the second
// column-sensitive scanner family. The Haskell layout scanner reads the
// column to place virtual braces and semicolons
// (grammars/runtime/haskell_scanner.go). One inserted space changes the
// indent of the first statement in a do block, which changes the block
// structure. An incremental reparse must agree with a fresh parse.
func TestIncrementalHaskellLayoutIndentEditMatchesFreshParse(t *testing.T) {
	lang := grammars.HaskellLanguage()
	original := []byte("module M where\nf = do\n  x\n  y\n")
	edited := []byte("module M where\nf = do\n   x\n  y\n")
	parser := gotreesitter.NewParser(lang)
	oldTree, err := parser.Parse(original)
	if err != nil {
		t.Fatalf("parse original: %v", err)
	}
	freshTree, err := parser.Parse(edited)
	if err != nil {
		t.Fatalf("parse edited: %v", err)
	}
	freshSExpr := freshTree.RootNode().SExpr(lang)
	if oldTree.RootNode().SExpr(lang) == freshSExpr {
		t.Fatalf("the indent edit must change the tree shape, got %s for both", freshSExpr)
	}

	at := uint32(len("module M where\nf = do\n"))
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte:   at,
		OldEndByte:  at,
		NewEndByte:  at + 1,
		StartPoint:  gotreesitter.Point{Row: 2, Column: 0},
		OldEndPoint: gotreesitter.Point{Row: 2, Column: 0},
		NewEndPoint: gotreesitter.Point{Row: 2, Column: 1},
	})
	incremental, err := parser.ParseIncremental(edited, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	if got := incremental.RootNode().SExpr(lang); got != freshSExpr {
		t.Fatalf("incremental parse differs from a fresh parse\n incremental: %s\n fresh:       %s", got, freshSExpr)
	}
}

// cobolColumnDependencyInsertEdit is the one-byte insert both the copy tests
// and the insert case use. It moves the '*' indicator into column 6.
var cobolColumnDependencyInsertEdit = gotreesitter.InputEdit{
	StartByte:   0,
	OldEndByte:  0,
	NewEndByte:  1,
	StartPoint:  gotreesitter.Point{Row: 3, Column: 0},
	OldEndPoint: gotreesitter.Point{Row: 3, Column: 0},
	NewEndPoint: gotreesitter.Point{Row: 3, Column: 1},
}

const (
	cobolColumnDependencyOriginalLine = "AAAAA* DISPLAY \"Z\"."
	cobolColumnDependencyEditedLine   = "AAAAAA* DISPLAY \"Z\"."
)

// TestCopyBeforeEditKeepsColumnDependency pins Tree.Copy against a lost
// column dependency. Copy clones nodes into a fresh arena. The clone carries
// each node's recorded bit, but not the arena span list the fold reads, so a
// copy taken before the first edit would answer false for every node unless
// Copy folds first.
func TestCopyBeforeEditKeepsColumnDependency(t *testing.T) {
	lang := grammars.CobolLanguage()
	original := []byte(cobolColumnDependencyPrefix + cobolColumnDependencyOriginalLine + "\n")
	edited := []byte(cobolColumnDependencyPrefix + cobolColumnDependencyEditedLine + "\n")
	parser := gotreesitter.NewParser(lang)
	oldTree, err := parser.Parse(original)
	if err != nil {
		t.Fatalf("parse original: %v", err)
	}
	freshTree, err := parser.Parse(edited)
	if err != nil {
		t.Fatalf("parse edited: %v", err)
	}
	freshSExpr := freshTree.RootNode().SExpr(lang)

	copied := oldTree.Copy()
	if copied == nil {
		t.Fatal("Copy returned nil")
	}
	stringNode := findNodeWithSpan(t, lang, copied.RootNode(), "string")
	if stringNode == nil {
		t.Fatalf("no string node in the copy: %s", copied.RootNode().SExpr(lang))
	}

	edit := cobolColumnDependencyInsertEdit
	base := uint32(len(cobolColumnDependencyPrefix))
	edit.StartByte += base
	edit.OldEndByte += base
	edit.NewEndByte += base
	copied.Edit(edit)

	if !stringNode.HasChanges() {
		t.Fatalf("copy lost the column dependency: string node [%d,%d) reports no changes",
			stringNode.StartByte(), stringNode.EndByte())
	}
	incremental, err := parser.ParseIncremental(edited, copied)
	if err != nil {
		t.Fatalf("incremental parse from the copy: %v", err)
	}
	if got := incremental.RootNode().SExpr(lang); got != freshSExpr {
		t.Fatalf("incremental parse from the copy differs from a fresh parse\n incremental: %s\n fresh:       %s", got, freshSExpr)
	}
}

// TestCopyAfterEditKeepsColumnDependency pins the other Copy order. Copy
// carries the source tree's pending edits, and the fold declines on a tree
// that already carries edits, so Copy must also carry the flag that says the
// fold already ran.
func TestCopyAfterEditKeepsColumnDependency(t *testing.T) {
	lang := grammars.CobolLanguage()
	original := []byte(cobolColumnDependencyPrefix + cobolColumnDependencyOriginalLine + "\n")
	edited := []byte(cobolColumnDependencyPrefix + cobolColumnDependencyEditedLine + "\n")
	parser := gotreesitter.NewParser(lang)
	oldTree, err := parser.Parse(original)
	if err != nil {
		t.Fatalf("parse original: %v", err)
	}
	freshTree, err := parser.Parse(edited)
	if err != nil {
		t.Fatalf("parse edited: %v", err)
	}
	freshSExpr := freshTree.RootNode().SExpr(lang)

	edit := cobolColumnDependencyInsertEdit
	base := uint32(len(cobolColumnDependencyPrefix))
	edit.StartByte += base
	edit.OldEndByte += base
	edit.NewEndByte += base
	oldTree.Edit(edit)

	copied := oldTree.Copy()
	if copied == nil {
		t.Fatal("Copy returned nil")
	}
	stringNode := findNodeWithSpan(t, lang, copied.RootNode(), "string")
	if stringNode == nil {
		t.Fatalf("no string node in the copy: %s", copied.RootNode().SExpr(lang))
	}
	if !stringNode.HasChanges() {
		t.Fatalf("copy dropped the changed bit: string node [%d,%d) reports no changes",
			stringNode.StartByte(), stringNode.EndByte())
	}
	incremental, err := parser.ParseIncremental(edited, copied)
	if err != nil {
		t.Fatalf("incremental parse from the copy: %v", err)
	}
	if got := incremental.RootNode().SExpr(lang); got != freshSExpr {
		t.Fatalf("incremental parse from the copy differs from a fresh parse\n incremental: %s\n fresh:       %s", got, freshSExpr)
	}
}

// TestColumnFreeLanguageNeverGainsColumnDependency pins the span record
// against arena reuse. A borrowed arena keeps the byte offsets of the parse
// that filled it, so a later tree must never match a reused and shifted leaf
// against them. The fold drops the list as soon as it runs, so many edit
// rounds on a language that never reads a column leave every node clean.
func TestColumnFreeLanguageNeverGainsColumnDependency(t *testing.T) {
	lang := grammars.GoLanguage()
	source := []byte("package p\n\nfunc f() int {\n\treturn 1\n}\n")
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	at := uint32(len("package p\n\nfunc f() int {\n\treturn "))
	for round := 0; round < 8; round++ {
		next := append(append([]byte(nil), source[:at]...), source[at:]...)
		next = append(next[:at], append([]byte{'1'}, next[at:]...)...)
		tree.Edit(gotreesitter.InputEdit{
			StartByte:   at,
			OldEndByte:  at,
			NewEndByte:  at + 1,
			StartPoint:  gotreesitter.Point{Row: 3, Column: 8},
			OldEndPoint: gotreesitter.Point{Row: 3, Column: 8},
			NewEndPoint: gotreesitter.Point{Row: 3, Column: 9},
		})
		source = next
		tree, err = parser.ParseIncremental(source, tree)
		if err != nil {
			t.Fatalf("round %d incremental parse: %v", round, err)
		}
		if n := countChangedNodes(tree.RootNode()); n != 0 {
			t.Fatalf("round %d: %d nodes report changes on a freshly parsed tree", round, n)
		}
	}
}

// countChangedNodes returns the number of nodes that report changes.
func countChangedNodes(n *gotreesitter.Node) int {
	if n == nil {
		return 0
	}
	total := 0
	if n.HasChanges() {
		total++
	}
	for i := 0; i < n.ChildCount(); i++ {
		total += countChangedNodes(n.Child(i))
	}
	return total
}

// TestColumnIndependentEditKeepsLaterTokensClean pins the other side of the
// contract: an edit on an earlier line must not invalidate a
// column-dependent token further down the file. The guards compare rows, so
// a line break between the edit and the token stops the walk.
func TestColumnIndependentEditKeepsLaterTokensClean(t *testing.T) {
	lang := grammars.CobolLanguage()
	original := []byte(cobolColumnDependencyPrefix +
		"       DISPLAY 1.\n" +
		"       DISPLAY \"Z\".\n")
	parser := gotreesitter.NewParser(lang)
	oldTree, err := parser.Parse(original)
	if err != nil {
		t.Fatalf("parse original: %v", err)
	}
	stringNode := findNodeWithSpan(t, lang, oldTree.RootNode(), "string")
	if stringNode == nil {
		t.Fatalf("no string node: %s", oldTree.RootNode().SExpr(lang))
	}
	// Insert one space at the start of line 3, two lines above the string.
	at := uint32(strings.Index(string(original), "       DISPLAY 1."))
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte:   at,
		OldEndByte:  at,
		NewEndByte:  at + 1,
		StartPoint:  gotreesitter.Point{Row: 3, Column: 0},
		OldEndPoint: gotreesitter.Point{Row: 3, Column: 0},
		NewEndPoint: gotreesitter.Point{Row: 3, Column: 1},
	})
	if stringNode.HasChanges() {
		t.Fatalf("string node on a later line was invalidated by an edit two lines above")
	}
}
