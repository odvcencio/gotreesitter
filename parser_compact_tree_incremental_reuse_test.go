package gotreesitter_test

import (
	"bytes"
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestCompactOldTreeKeepsTopLevelIncrementalReuse pins the issue #454 reuse
// repair. A fresh parse on the compact route must leave an old tree that the
// production incremental path reuses as well as a production old tree: the
// compatible-goto contract applies to compact candidates, and a synthesized
// root (trailing extras after the accepted root payload, as in INI files that
// end with a blank line) does not disable reuse for the whole tree.
func TestCompactOldTreeKeepsTopLevelIncrementalReuse(t *testing.T) {
	type fixture struct {
		lang   string
		gen    func(n int) []byte
		marker string
	}
	repeat := func(format string) func(int) []byte {
		return func(n int) []byte {
			var b bytes.Buffer
			for i := 0; b.Len() < n; i++ {
				fmt.Fprintf(&b, format, i, i)
			}
			return b.Bytes()
		}
	}
	fixtures := []fixture{
		{"toml", repeat("[section%d]\nx0 = %d\nname = \"f\"\nenabled = true\n\n"), "x0"},
		{"ini", repeat("[section%d]\nx0 = %d\nname = f\n\n"), "x0"},
		{"make", repeat("f%d: x0.o\n\t$(CC) -o f%d x0.o\n\n"), "x0"},
		{"diff", repeat("diff --git a/f%d.txt b/f%d.txt\n--- a/f.txt\n+++ b/f.txt\n@@ -1,2 +1,2 @@\n-x0 old\n+x0 new\n"), "x0"},
		{"typescript", repeat("function f%d(a: number, b: number): number {\n  const x0 = a + b;\n  console.log(\"f%d\", x0);\n  return x0;\n}\n\n"), "x0"},
	}
	for _, fx := range fixtures {
		t.Run(fx.lang, func(t *testing.T) {
			entry := grammars.DetectLanguageByName(fx.lang)
			if entry == nil || entry.Language() == nil {
				t.Skipf("%s grammar not registered", fx.lang)
			}
			lang := entry.Language()
			src := fx.gen(48 << 10)
			site := bytes.Index(src, []byte(fx.marker))
			if site < 0 {
				t.Fatalf("marker %q missing", fx.marker)
			}
			edited := append(append(append([]byte{}, src[:site]...), src[site]), src[site:]...)
			row := uint32(bytes.Count(src[:site], []byte{'\n'}))
			col := uint32(site - (bytes.LastIndexByte(src[:site], '\n') + 1))
			edit := gts.InputEdit{
				StartByte: uint32(site), OldEndByte: uint32(site), NewEndByte: uint32(site + 1),
				StartPoint: gts.Point{Row: row, Column: col}, OldEndPoint: gts.Point{Row: row, Column: col},
				NewEndPoint: gts.Point{Row: row, Column: col + 1},
			}

			parser := gts.NewParser(lang)
			parser.SetAdmissionCandidateRoute(true)
			old, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("compact parse: %v", err)
			}
			routed, fallback := gts.AdmissionCandidateCounters()
			_ = fallback
			old.Edit(edit)
			inc, prof, err := parser.ParseIncrementalProfiled(edited, old)
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			fresh, err := parser.Parse(edited)
			if err != nil {
				t.Fatalf("fresh parse: %v", err)
			}
			if got, want := inc.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang); got != want {
				t.Fatalf("incremental tree diverges from fresh parse\n  incremental: %.300s\n  fresh:       %.300s", got, want)
			}
			if prof.ReuseUnsupported {
				t.Fatalf("reuse unsupported on a compact old tree: %q (routed=%d)", prof.ReuseUnsupportedReason, routed)
			}
			if 10*prof.ReusedBytes < 9*uint64(len(edited)) {
				t.Fatalf("reused %d of %d bytes (%.1f%%), want at least 90%%; rootNonLeaf rejects=%d",
					prof.ReusedBytes, len(edited), 100*float64(prof.ReusedBytes)/float64(len(edited)), prof.ReuseRejectRootNonLeafChanged)
			}
			if prof.ReuseRejectRootNonLeafChanged > 64 {
				t.Fatalf("rootNonLeaf rejects = %d, want at most 64", prof.ReuseRejectRootNonLeafChanged)
			}
		})
	}
}
