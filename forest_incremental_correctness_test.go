package gotreesitter_test

import (
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// byteToPoint computes the (row, column) for a byte offset, matching the
// tree-sitter convention (row = newlines before off, column = bytes since the
// last newline).
func byteToPoint(src []byte, off int) gts.Point {
	row, col := 0, 0
	for i := 0; i < off && i < len(src); i++ {
		if src[i] == '\n' {
			row++
			col = 0
		} else {
			col++
		}
	}
	return gts.Point{Row: uint32(row), Column: uint32(col)}
}

type forestEdit struct {
	desc   string
	edited []byte
	inEdit gts.InputEdit
}

func replaceEdit(src []byte, p int) forestEdit {
	nb := byte('z')
	if src[p] == 'z' {
		nb = 'q'
	}
	ed := append([]byte(nil), src...)
	ed[p] = nb
	return forestEdit{"replace", ed, gts.InputEdit{
		StartByte: uint32(p), OldEndByte: uint32(p + 1), NewEndByte: uint32(p + 1),
		StartPoint: byteToPoint(src, p), OldEndPoint: byteToPoint(src, p+1), NewEndPoint: byteToPoint(ed, p+1),
	}}
}

func insertEdit(src []byte, p int) forestEdit {
	ed := make([]byte, 0, len(src)+1)
	ed = append(ed, src[:p]...)
	ed = append(ed, 'x')
	ed = append(ed, src[p:]...)
	return forestEdit{"insert", ed, gts.InputEdit{
		StartByte: uint32(p), OldEndByte: uint32(p), NewEndByte: uint32(p + 1),
		StartPoint: byteToPoint(src, p), OldEndPoint: byteToPoint(src, p), NewEndPoint: byteToPoint(ed, p+1),
	}}
}

func deleteEdit(src []byte, p int) forestEdit {
	ed := make([]byte, 0, len(src))
	ed = append(ed, src[:p]...)
	ed = append(ed, src[p+1:]...)
	return forestEdit{"delete", ed, gts.InputEdit{
		StartByte: uint32(p), OldEndByte: uint32(p + 1), NewEndByte: uint32(p),
		StartPoint: byteToPoint(src, p), OldEndPoint: byteToPoint(src, p+1), NewEndPoint: byteToPoint(ed, p),
	}}
}

// TestForestIncrementalCorrectness is the edited-corpus matrix gate that
// languageAllowsForestIncrementalPath always required but never had. For each
// forest language it applies many varied edits (replace/insert/delete) across a
// real corpus file and asserts the incremental re-parse of the forest-built old
// tree is byte-for-byte identical (s-expr) to a fresh parse of the same edited
// source — only over edits that keep the source valid (an edit that breaks
// syntax routes fresh through production error recovery, a different-but-valid
// path). erlang + javascript pass via real forest-incremental reuse; scss/css/
// cmake are demoted from languageAllowsForestIncrementalPath (they FAILED this
// gate — wrong/truncated trees) and reach the same assertion via fresh-parse
// fallback. Re-adding a language to that list without it passing here regresses
// incremental correctness.
func TestForestIncrementalCorrectness(t *testing.T) {
	if os.Getenv("GTS_FOREST_INCR") == "" {
		t.Skip("set GTS_FOREST_INCR=1 to run the forest incremental correctness matrix (heavy)")
	}
	cases := []struct {
		name string
		file string
		lang func() *gts.Language
	}{
		// These are forest-default languages (builtinForestDefaults). erlang +
		// javascript do real forest-incremental reuse (must match fresh); scss,
		// css, cmake and go are forest for full parses but demoted from the
		// incremental path, so they reach the same assertion via fresh-parse
		// fallback. (python is NOT a forest language and its PRODUCTION incremental
		// reuse has its own pre-existing bug — tracked separately — so it is out of
		// scope here.)
		{"javascript", "cgo_harness/corpus_real/javascript/large__jquery.js", grammars.JavascriptLanguage},
		{"scss", "cgo_harness/corpus_real/scss/large__github.com.scss", grammars.ScssLanguage},
		{"css", "cgo_harness/corpus_real/css/large__github.com.css", grammars.CssLanguage},
		{"erlang", "cgo_harness/corpus_real/erlang/medium__attributes.erl", grammars.ErlangLanguage},
		{"cmake", "cgo_harness/corpus_real/cmake/medium__CMakeLists.txt", grammars.CmakeLanguage},
		{"go", "cgo_harness/corpus_real/go/medium__letter_test.go", grammars.GoLanguage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src, err := os.ReadFile(tc.file)
			if err != nil {
				t.Skipf("corpus missing: %v", err)
			}
			lang := tc.lang()

			// Positions spread across the file, each exercised with replace,
			// insert and delete.
			const sites = 10
			var edits []forestEdit
			for i := 1; i <= sites; i++ {
				p := i * len(src) / (sites + 2)
				if p <= 0 || p >= len(src)-1 {
					continue
				}
				edits = append(edits, replaceEdit(src, p), insertEdit(src, p), deleteEdit(src, p))
			}

			mismatches := 0
			tested := 0
			for i, e := range edits {
				parser := gts.NewParser(lang)
				oldTree, err := parser.Parse(src)
				if err != nil {
					t.Fatalf("edit %d: initial parse: %v", i, err)
				}
				if oldTree.RootNode().HasError() {
					oldTree.Release()
					t.Fatalf("edit %d: initial parse has error root", i)
				}
				oldTree.Edit(e.inEdit)
				incTree, err := parser.ParseIncremental(e.edited, oldTree)
				if err != nil {
					t.Fatalf("edit %d (%s): incremental parse: %v", i, e.desc, err)
				}

				fresh := gts.NewParser(lang)
				freshTree, err := fresh.Parse(e.edited)
				if err != nil {
					t.Fatalf("edit %d (%s): fresh parse: %v", i, e.desc, err)
				}

				// Only valid edited sources are a meaningful incremental==fresh
				// check: an edit that breaks syntax routes the fresh parse through
				// forest-decline -> production error recovery, a different (but
				// equally valid) path than incremental reuse, so a mismatch there
				// is not an incremental-reuse bug.
				if freshTree.RootNode().HasError() || incTree.RootNode().HasError() {
					incTree.Release()
					freshTree.Release()
					continue
				}
				tested++
				got := incTree.RootNode().SExpr(lang)
				want := freshTree.RootNode().SExpr(lang)
				if got != want {
					mismatches++
					if mismatches <= 3 {
						n := 240
						if len(got) < n {
							n = len(got)
						}
						m := 240
						if len(want) < m {
							m = len(want)
						}
						t.Errorf("edit %d (%s @ byte %d): incremental != fresh\n inc  =%s\n fresh=%s",
							i, e.desc, e.inEdit.StartByte, got[:n], want[:m])
					}
				}
				incTree.Release()
				freshTree.Release()
			}
			if mismatches > 0 {
				t.Errorf("%s: %d/%d VALID edits produced incremental != fresh", tc.name, mismatches, tested)
			} else {
				t.Logf("%s: %d/%d valid edits all incremental == fresh (rest skipped as error-producing)", tc.name, tested, len(edits))
			}
			if tested < 3 {
				t.Errorf("%s: only %d valid edits tested — sample too small to trust", tc.name, tested)
			}
		})
	}
}
