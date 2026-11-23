//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"strings"
	"testing"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestCcoreIncludedRangesExternalScannersMatchPinnedC(t *testing.T) {
	type sample struct {
		name   string
		gap    string
		chunks []string
	}
	for _, grammar := range []struct {
		name  string
		load  func() *g.Language
		tests []sample
	}{
		{"javascript", grammars.JavascriptLanguage, []sample{
			{"automatic semicolon at the range start", " OMIT ", []string{"let a=1", "let b=2"}},
			{"template content across a gap", "OMIT", []string{"const t = \x60ab", "cd${1}ef\x60;"}},
			{"template content across rows", "\nOMIT\n", []string{"const t = \x60ab", "cd\x60;"}},
			{"initial BOM through a gap", "OMIT", []string{"\ufeff", "const t = \x60ab\x60;"}},
			{"unterminated template at ranged EOF", "OMIT", []string{"const t = \x60ab", "cd"}},
			{"template crosses invalid UTF-8", "OMIT", []string{"const t = \x60ab", "\xffcd\x60;"}},
		}},
		{"python", grammars.PythonLanguage, []sample{
			{"indents and dedents across rows", "omitted\n", []string{"if True:\n", "    x = 1\n", "y = 2\n"}},
			{"indentation spaces across a gap", "OMIT", []string{"if True:\n  ", "  pass\n", "y = 2\n"}},
			{"serialized triple quoted string", "OMIT", []string{"x = '''ab", "cd'''\n", "y = 2\n"}},
			{"stateful f-string", "OMIT", []string{"x = f'''ab", "cd{1}ef'''\n"}},
		}},
		{"cpp", grammars.CppLanguage, []sample{
			{"raw string through excluded text", "OMIT", []string{"auto s = R\"tag(ab", "cd)tag\";"}},
		}},
		{"bash", grammars.BashLanguage, []sample{
			{"heredoc column across a gap", "OMIT", []string{"cat <<EOF\n\u00e9", "b\nEOF\n"}},
		}},
		{"html", grammars.HtmlLanguage, []sample{
			{"raw script text through excluded text", "OMIT", []string{"<script>let a=1;", "let b=2;</script>"}},
			{"comment through excluded text", "OMIT", []string{"<!--ab", "cd--><p>hi</p>"}},
		}},
	} {
		t.Run(grammar.name, func(t *testing.T) {
			core, err := bind.New(grammar.load())
			if err != nil {
				t.Fatal(err)
			}
			core.SetStopAfter(1024, false)
			cl, err := COracleLanguage(grammar.name)
			if err != nil {
				t.Fatal(err)
			}
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cl); err != nil {
				t.Fatal(err)
			}
			for _, test := range grammar.tests {
				t.Run(test.name, func(t *testing.T) {
					source, spans := selectedScannerChunks(test.gap, test.chunks)
					goRanges, cRanges := coreAndCIncludedRanges(source, spans)
					if err := cp.SetIncludedRanges(cRanges); err != nil {
						t.Fatal(err)
					}
					want := cp.Parse(source, nil)
					if want == nil {
						t.Fatal("pinned C returned no tree")
					}
					defer want.Close()
					got, stop := core.ParseIncluded(source, goRanges)
					if got == nil || stop != "" {
						t.Fatalf("ccore stopped: %q", stop)
					}
					defer got.Release()
					assertIncludedTreeMatchesC(t, core, got, want)
				})
			}
		})
	}
}

func selectedScannerChunks(gap string, chunks []string) ([]byte, [][2]int) {
	var source strings.Builder
	spans := make([][2]int, 0, len(chunks))
	for i, chunk := range chunks {
		if i > 0 {
			source.WriteString(gap)
		}
		start := source.Len()
		source.WriteString(chunk)
		spans = append(spans, [2]int{start, source.Len()})
	}
	return []byte(source.String()), spans
}

func TestCcoreIncrementalIncludedRangesExternalScannersMatchPinnedC(t *testing.T) {
	for _, grammar := range []struct {
		name   string
		load   func() *g.Language
		chunks []string
	}{
		{"javascript", grammars.JavascriptLanguage, []string{"let kept = 1; const t = \x60ab", "cd\x60; let tail = 2;"}},
		{"python", grammars.PythonLanguage, []string{"kept = 1\nx = \x27\x27\x27ab", "cd\x27\x27\x27\ny = 2\n"}},
	} {
		t.Run(grammar.name, func(t *testing.T) {
			config, err := bind.Config(grammar.load())
			if err != nil {
				t.Fatal(err)
			}
			core, err := ccore.NewParser(config)
			if err != nil {
				t.Fatal(err)
			}
			core.SetStopAfter(1024, false)
			cl, err := COracleLanguage(grammar.name)
			if err != nil {
				t.Fatal(err)
			}
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cl); err != nil {
				t.Fatal(err)
			}
			source, spans := selectedScannerChunks("OMIT", grammar.chunks)
			oldRanges, oldCRanges := coreAndCIncludedRanges(source, [][2]int{{0, len(source)}})
			if err := cp.SetIncludedRanges(oldCRanges); err != nil {
				t.Fatal(err)
			}
			oldC := cp.Parse(source, nil)
			if oldC == nil {
				t.Fatal("pinned C returned no initial tree")
			}
			defer oldC.Close()
			old, stop := core.ParseIncluded(source, oldRanges)
			if old == nil || stop != "" {
				t.Fatalf("initial ccore parse: %q", stop)
			}
			defer old.Release()
			assertIncludedTreeMatchesC(t, core, old, oldC)

			newRanges, newCRanges := coreAndCIncludedRanges(source, spans)
			if err := cp.SetIncludedRanges(newCRanges); err != nil {
				t.Fatal(err)
			}
			newC := cp.Parse(source, oldC)
			if newC == nil {
				t.Fatal("pinned C returned no incremental tree")
			}
			defer newC.Close()
			next, stop := core.ParseWithIncluded(source, old, newRanges)
			if next == nil || stop != "" {
				t.Fatalf("incremental ccore parse: %q", stop)
			}
			defer next.Release()
			assertIncludedTreeMatchesC(t, core, next, newC)
			reused, _, _ := core.ReuseStats()
			if reused == 0 {
				t.Fatal("incremental parse reused no nodes")
			}

			changes, ok := ccore.ChangedRanges(old, next, config.Tables, old.IncludedRanges(), next.IncludedRanges())
			if !ok {
				t.Fatal("ccore changed ranges declined")
			}
			assertIncludedRangesEqual(t, changes, oldC.ChangedRanges(newC))

			// Put a newline inside the external string token, shifting both
			// its end point and the points of later nodes in the range.
			start := spans[1][0]
			row, col := includedRangesPointAt(source, start)
			endRow, endCol := includedRangesPointAt(source, start+1)
			editedSource := append([]byte(nil), source...)
			editedSource[start] = '\n'
			newEndRow, newEndCol := includedRangesPointAt(editedSource, start+1)
			next.Edit(ccore.Edit{
				StartByte: uint32(start), OldEndByte: uint32(start + 1), NewEndByte: uint32(start + 1),
				StartRow: uint32(row), StartCol: uint32(col), OldEndRow: uint32(endRow), OldEndCol: uint32(endCol),
				NewEndRow: uint32(newEndRow), NewEndCol: uint32(newEndCol),
			})
			newC.Edit(&sitter.InputEdit{
				StartByte: uint(start), OldEndByte: uint(start + 1), NewEndByte: uint(start + 1),
				StartPosition: sitter.Point{Row: row, Column: col}, OldEndPosition: sitter.Point{Row: endRow, Column: endCol},
				NewEndPosition: sitter.Point{Row: newEndRow, Column: newEndCol},
			})
			editedRanges, editedCRanges := coreAndCIncludedRanges(editedSource, spans)
			if err := cp.SetIncludedRanges(editedCRanges); err != nil {
				t.Fatal(err)
			}
			editedC := cp.Parse(editedSource, newC)
			if editedC == nil {
				t.Fatal("pinned C returned no source-edited tree")
			}
			defer editedC.Close()
			edited, stop := core.ParseWithIncluded(editedSource, next, editedRanges)
			if edited == nil || stop != "" {
				t.Fatalf("source-edited ccore parse: %q", stop)
			}
			defer edited.Release()
			assertIncludedTreeMatchesC(t, core, edited, editedC)

			if err := cp.SetIncludedRanges(nil); err != nil {
				t.Fatal(err)
			}
			fullC := cp.Parse(source, nil)
			if fullC == nil {
				t.Fatal("pinned C returned no whole-input tree")
			}
			defer fullC.Close()
			full, stop := core.Parse(source)
			if full == nil || stop != "" {
				t.Fatalf("whole-input ccore parse: %q", stop)
			}
			defer full.Release()
			assertIncludedTreeMatchesC(t, core, full, fullC)
		})
	}
}

func TestLegacyTSXDeferredSelectedEOF(t *testing.T) {
	lang, oracle := ccoreLanguages(t, "tsx")
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(oracle); err != nil {
		t.Fatal(err)
	}

	const prefix = "const outsideBefore = false;\n"
	const one = "const left = <a />;\n"
	const gap = "const hidden = false;\n"
	const two = "const right = <b />;\n"
	const suffix = "const outsideAfter = false;\n"
	source := []byte(prefix + one + gap + two + suffix)
	twoStart := len(prefix + one + gap)
	twoEnd := twoStart + len(two)
	at := strings.Index(string(source), "right")
	point, endPoint := gradPoint(source, at), gradPoint(source, at+1)
	edit := g.InputEdit{
		StartByte: uint32(at), OldEndByte: uint32(at + 1), NewEndByte: uint32(at + 1),
		StartPoint: point, OldEndPoint: endPoint, NewEndPoint: endPoint,
	}
	nextSource := append([]byte(nil), source...)
	nextSource[at] = 's'

	for _, test := range []struct {
		name string
		end  int
	}{
		{"included-newline", twoEnd},
		{"excluded-newline", twoEnd - 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			parser := g.NewParser(lang)
			parser.SetAdmissionCandidateRoute(false)
			ranges := []g.Range{
				{StartByte: uint32(len(prefix)), EndByte: uint32(len(prefix + one)), StartPoint: gradPoint(source, len(prefix)), EndPoint: gradPoint(source, len(prefix+one))},
				{StartByte: uint32(twoStart), EndByte: uint32(test.end), StartPoint: gradPoint(source, twoStart), EndPoint: gradPoint(source, test.end)},
			}
			parser.SetIncludedRanges(ranges)
			_, cRanges := coreAndCIncludedRanges(source, [][2]int{{len(prefix), len(prefix + one)}, {twoStart, test.end}})
			if err := cp.SetIncludedRanges(cRanges); err != nil {
				t.Fatal(err)
			}

			old, err := parser.ParseStrict(source)
			if err != nil || old == nil {
				t.Fatalf("fresh parse: %v", err)
			}
			defer old.Release()
			oldC := cp.Parse(source, nil)
			if oldC == nil {
				t.Fatal("pinned C returned no fresh tree")
			}
			defer oldC.Close()
			assertLockedCTreeExact(t, "fresh bounded root", old, lang, oldC)
			if got := old.ParseRuntime().RootEndByte; got != uint32(test.end) {
				t.Fatalf("fresh diagnostic root end = %d, want %d", got, test.end)
			}

			old.Edit(edit)
			oldC.Edit(gradSitterEdit(edit))
			next, err := parser.ParseIncrementalStrict(nextSource, old)
			if err != nil || next == nil {
				t.Fatalf("incremental parse: %v", err)
			}
			if next != old {
				defer next.Release()
			}
			nextC := cp.Parse(nextSource, oldC)
			if nextC == nil {
				t.Fatal("pinned C returned no incremental tree")
			}
			defer nextC.Close()
			assertLockedCTreeExact(t, "incremental bounded root", next, lang, nextC)
			if got := next.ParseRuntime().RootEndByte; got != uint32(test.end) {
				t.Fatalf("incremental diagnostic root end = %d, want %d", got, test.end)
			}
		})
	}
}
