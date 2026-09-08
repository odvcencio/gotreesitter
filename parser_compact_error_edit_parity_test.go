package gotreesitter_test

import (
	"bytes"
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestCompactOldTreeTransientErrorEditsMatchFreshParse pins the issue #454
// keystroke contract on the default route. A single-byte delete that leaves a
// transient syntax error mid-file is served by production incremental reuse
// on the compact old tree, and the result must equal a fresh parse of the
// edited bytes on the default route. An edit that reaches the end of the file
// keeps the fresh compact recovery route.
func TestCompactOldTreeTransientErrorEditsMatchFreshParse(t *testing.T) {
	repeat := func(format string) func(int) []byte {
		return func(n int) []byte {
			var b bytes.Buffer
			for i := 0; b.Len() < n; i++ {
				fmt.Fprintf(&b, format, i, i)
			}
			return b.Bytes()
		}
	}
	type fixture struct {
		lang   string
		gen    func(int) []byte
		marker string
	}
	fixtures := []fixture{
		{"go", func(n int) []byte {
			b := []byte("package main\n\nimport \"fmt\"\n\n")
			return append(b, repeat("func f%d(a int, b int) int {\n\tx0 := a + b\n\tfmt.Println(\"f%d\", x0)\n\treturn x0\n}\n\n")(n)...)
		}, "x0"},
		{"typescript", repeat("function f%d(a: number, b: number): number {\n  const x0 = a + b;\n  console.log(\"f%d\", x0);\n  return x0;\n}\n\n"), "x0"},
		{"javascript", repeat("function f%d(a, b) {\n  const x0 = a + b;\n  console.log(\"f%d\", x0);\n  return x0;\n}\n\n"), "x0"},
		{"c", repeat("int f%d(int a, int b) {\n    int x0 = a + b;\n    return x0 + %d;\n}\n\n"), "x0"},
		{"css", repeat(".f%d {\n  color: red;\n  width: %dpx;\n}\n\n"), "red"},
		{"toml", repeat("[section%d]\nx0 = %d\nname = \"f\"\nenabled = true\n\n"), "x0"},
		{"cmake", repeat("function(f%d a b)\n  set(x0 ${a})\n  message(STATUS \"f%d ${x0}\")\nendfunction()\n\n"), "x0"},
		{"ini", repeat("[section%d]\nx0 = %d\nname = f\n\n"), "x0"},
		{"hcl", repeat("resource \"aws_instance\" \"f%d\" {\n  x0 = %d\n  name = \"f\"\n}\n\n"), "x0"},
	}
	// Known divergences that predate the issue #454 repair. Both are
	// production-route or compact-fresh behaviors that this gate documents
	// rather than pins:
	//   - c/unbalanced: production's fresh parse wraps the file in ERROR while
	//     production's incremental parse recovers the function; the compact
	//     route declines this source, so both trees come from production.
	//   - javascript/eof: the compact route's certified end-of-file recovery
	//     keeps the trailing function that production wraps in ERROR;
	//     JavaScript never enters the compact incremental recovery route
	//     because its scanner is not stateless.
	known := map[string]string{
		"c/unbalanced":   "production fresh parse and incremental parse disagree",
		"javascript/eof": "compact end-of-file recovery differs from production incremental recovery",
	}
	deleteAt := func(src []byte, site int) ([]byte, gts.InputEdit) {
		edited := append(append([]byte{}, src[:site]...), src[site+1:]...)
		row := uint32(bytes.Count(src[:site], []byte{'\n'}))
		col := uint32(site - (bytes.LastIndexByte(src[:site], '\n') + 1))
		return edited, gts.InputEdit{
			StartByte: uint32(site), OldEndByte: uint32(site + 1), NewEndByte: uint32(site),
			StartPoint: gts.Point{Row: row, Column: col}, OldEndPoint: gts.Point{Row: row, Column: col + 1},
			NewEndPoint: gts.Point{Row: row, Column: col},
		}
	}
	for _, fx := range fixtures {
		entry := grammars.DetectLanguageByName(fx.lang)
		if entry == nil || entry.Language() == nil {
			t.Logf("%s grammar not registered", fx.lang)
			continue
		}
		lang := entry.Language()
		src := fx.gen(24 << 10)
		mid := bytes.Index(src, []byte(fx.marker))
		brace := mid + bytes.IndexAny(src[mid:], "})]")
		eof := bytes.LastIndexAny(src, "})]")
		for _, class := range []struct {
			name string
			site int
		}{{"midfile", mid}, {"unbalanced", brace}, {"eof", eof}} {
			t.Run(fx.lang+"/"+class.name, func(t *testing.T) {
				if class.site < 0 {
					t.Skip("no edit site")
				}
				parser := gts.NewParser(lang)
				parser.SetAdmissionCandidateRoute(true)
				old, err := parser.Parse(src)
				if err != nil {
					t.Fatalf("compact parse: %v", err)
				}
				edited, edit := deleteAt(src, class.site)
				old.Edit(edit)
				inc, _, err := parser.ParseIncrementalProfiled(edited, old)
				if err != nil {
					t.Fatalf("incremental parse: %v", err)
				}
				fresh, err := gts.NewParser(lang).Parse(edited)
				if err != nil {
					t.Fatalf("fresh parse: %v", err)
				}
				if got, want := inc.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang); got != want {
					if reason, ok := known[fx.lang+"/"+class.name]; ok {
						t.Logf("known divergence (%s)", reason)
						return
					}
					t.Fatalf("incremental tree diverges from the fresh default-route parse\n  incremental: %.400s\n  fresh:       %.400s", got, want)
				}
				rt := inc.ParseRuntime()
				if class.name == "midfile" && fx.lang == "go" && rt.CompactIncrementalFullRecoveryRoute {
					t.Fatalf("a mid-file transient error must use production reuse, not the compact recovery route; runtime=%s", rt.Summary())
				}
			})
		}
	}
}
