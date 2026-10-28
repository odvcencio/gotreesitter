//go:build !grammar_subset

package grammars

import (
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

// emptyInputRootCases are the inputs for which tree-sitter C always returns a
// root node. Go used to return a Tree whose RootNode() was nil for some
// grammars (those whose parse table answers EOF in state 0 with a recover
// action) because the zero-node accept path built a Tree with no root.
var emptyInputRootCases = []string{"", " ", "\n"}

// emptyInputCRootSExpr pins the root tree-sitter C returns for the grammar and
// input pairs that used to have a nil Go root. The values come from the locked
// C oracle (cgo_harness TestParityEmptyInputRoot), where C's recover_eof step
// publishes a childless ERROR root.
var emptyInputCRootSExpr = map[string]map[string]string{
	"doxygen":         {"": "(ERROR)", " ": "(ERROR)", "\n": "(ERROR)"},
	"tlaplus":         {"": "(ERROR)", " ": "(ERROR)", "\n": "(ERROR)"},
	"norg":            {"": "(ERROR)"},
	"markdown_inline": {"": "(ERROR)"},
	"xml":             {"": "(ERROR)"},
}

// TestEmptyInputRootNonNilAllLanguages asserts that every registered grammar
// returns a non-nil root for empty and whitespace-only input.
func TestEmptyInputRootNonNilAllLanguages(t *testing.T) {
	for _, entry := range AllLanguages() {
		entry := entry
		t.Run(entry.Name, func(t *testing.T) {
			lang := entry.Language()
			if lang == nil {
				t.Skip("language did not load")
			}
			for _, input := range emptyInputRootCases {
				src := []byte(input)
				parser := gts.NewParser(lang)
				var (
					tree *gts.Tree
					err  error
				)
				if entry.TokenSourceFactory != nil {
					tree, err = parser.ParseWithTokenSource(src, entry.TokenSourceFactory(src, lang))
				} else {
					tree, err = parser.Parse(src)
				}
				if err != nil {
					t.Fatalf("parse %q: %v", input, err)
				}
				if tree == nil {
					t.Fatalf("parse %q returned a nil tree", input)
				}
				root := tree.RootNode()
				if root == nil {
					tree.Release()
					t.Errorf("RootNode() is nil for %q", input)
					continue
				}
				if root.IsError() && !root.HasError() {
					t.Errorf("ERROR root for %q does not report HasError", input)
				}
				if want, ok := emptyInputCRootSExpr[entry.Name][input]; ok {
					if got := root.SExpr(lang); got != want {
						t.Errorf("root for %q = %s, want C root %s", input, got, want)
					}
				}
				tree.Release()
			}
		})
	}
}

// TestEmptyInputEditSessionMatchesFresh walks the grammars that used to return
// a nil root through edits between empty and whitespace-only text. At every
// step the incremental result must equal a fresh parse, an ERROR root must
// report HasError, the root must reach the end of the input, and a reparse
// with no edit must allocate nothing.
func TestEmptyInputEditSessionMatchesFresh(t *testing.T) {
	type step struct {
		from, to string
	}
	steps := []step{
		{"", "\n"},
		{"\n", ""},
		{"", " "},
		{" ", "\n"},
		{"\n", " "},
		{" ", ""},
	}
	wanted := map[string]bool{"doxygen": true, "tlaplus": true, "norg": true, "markdown_inline": true, "xml": true}
	pointAfter := func(s string) gts.Point {
		var p gts.Point
		for i := 0; i < len(s); i++ {
			if s[i] == '\n' {
				p.Row++
				p.Column = 0
			} else {
				p.Column++
			}
		}
		return p
	}
	for _, entry := range AllLanguages() {
		if !wanted[entry.Name] {
			continue
		}
		entry := entry
		t.Run(entry.Name, func(t *testing.T) {
			lang := entry.Language()
			parser := gts.NewParser(lang)
			parse := func(src []byte, old *gts.Tree) *gts.Tree {
				t.Helper()
				var (
					tree *gts.Tree
					err  error
				)
				switch {
				case entry.TokenSourceFactory != nil && old == nil:
					tree, err = parser.ParseWithTokenSource(src, entry.TokenSourceFactory(src, lang))
				case entry.TokenSourceFactory != nil:
					tree, err = parser.ParseIncrementalWithTokenSource(src, old, entry.TokenSourceFactory(src, lang))
				case old == nil:
					tree, err = parser.Parse(src)
				default:
					tree, err = parser.ParseIncremental(src, old)
				}
				if err != nil {
					t.Fatalf("parse %q: %v", src, err)
				}
				if tree == nil || tree.RootNode() == nil {
					t.Fatalf("parse %q returned no root", src)
				}
				return tree
			}
			check := func(label, src string, tree *gts.Tree) {
				t.Helper()
				root := tree.RootNode()
				if root.IsError() && !root.HasError() {
					t.Errorf("%s: ERROR root does not report HasError", label)
				}
				stop := tree.ParseStopReason()
				if root.EndByte() != uint32(len(src)) && (stop == gts.ParseStopAccepted || stop == gts.ParseStopNone) {
					t.Errorf("%s: root ends at %d of %d with stop reason %v", label, root.EndByte(), len(src), stop)
				}
			}
			old := parse(nil, nil)
			defer func() { old.Release() }()
			check("initial", "", old)
			for i, s := range steps {
				old.Edit(gts.InputEdit{
					StartByte:   0,
					OldEndByte:  uint32(len(s.from)),
					NewEndByte:  uint32(len(s.to)),
					StartPoint:  gts.Point{},
					OldEndPoint: pointAfter(s.from),
					NewEndPoint: pointAfter(s.to),
				})
				next := parse([]byte(s.to), old)
				fresh := parse([]byte(s.to), nil)
				label := fmt.Sprintf("step %d to %q", i, s.to)
				if got, want := next.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang); got != want {
					t.Errorf("%s: incremental %s, fresh %s", label, got, want)
				}
				if next.RootNode().StartByte() != fresh.RootNode().StartByte() || next.RootNode().EndByte() != fresh.RootNode().EndByte() {
					t.Errorf("%s: incremental span [%d,%d], fresh span [%d,%d]", label,
						next.RootNode().StartByte(), next.RootNode().EndByte(),
						fresh.RootNode().StartByte(), fresh.RootNode().EndByte())
				}
				check(label, s.to, next)
				fresh.Release()
				old.Release()
				old = next
			}
			src := []byte(steps[len(steps)-1].to)
			allocs := testing.AllocsPerRun(3, func() {
				next, err := parser.ParseIncremental(src, old)
				if err != nil {
					t.Fatal(err)
				}
				next.Release()
			})
			if allocs != 0 {
				t.Errorf("no-edit reparse allocations = %g, want 0", allocs)
			}
		})
	}
}
