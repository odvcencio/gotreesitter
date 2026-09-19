package highlight_test

import (
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// injectionSnapshot is a comparable, tree-pointer-independent description of
// one detected injection. It lets a test assert that ParseIncremental's
// nested injection tree has the same shape as a fresh Parse of the same
// text, without depending on *Tree object identity.
type injectionSnapshot struct {
	Language  string
	StartByte uint32
	EndByte   uint32
	RootType  string
}

func snapshotInjections(injections []gotreesitter.Injection, langs map[string]*gotreesitter.Language) []injectionSnapshot {
	out := make([]injectionSnapshot, 0, len(injections))
	for _, inj := range injections {
		snap := injectionSnapshot{Language: inj.Language}
		if len(inj.Ranges) > 0 {
			snap.StartByte = inj.Ranges[0].StartByte
			snap.EndByte = inj.Ranges[len(inj.Ranges)-1].EndByte
		}
		if inj.Tree != nil {
			if root := inj.Tree.RootNode(); root != nil {
				if lang := langs[inj.Language]; lang != nil {
					snap.RootType = root.Type(lang)
				}
			}
		}
		out = append(out, snap)
	}
	return out
}

// pointAt returns the row/column position of a byte offset within source,
// using the source's own newline layout. It exists only to build InputEdit
// values in this test; it does not need to be fast.
func pointAt(source []byte, offset int) gotreesitter.Point {
	row := uint32(0)
	col := uint32(0)
	for i := 0; i < offset && i < len(source); i++ {
		if source[i] == '\n' {
			row++
			col = 0
		} else {
			col++
		}
	}
	return gotreesitter.Point{Row: row, Column: col}
}

// TestInjectionParserIncrementalTwoLevelMatchesFreshParse builds a
// Markdown-in-Markdown-in-Go document: the top-level document has a fenced
// code block whose language is "markdown"; reparsing that block's raw
// content as its own Markdown document reveals a second, nested fenced code
// block whose language is "go". This gives two levels of injection using
// only grammars and injection queries the registry already supports.
//
// It then edits the document twice — once inside the innermost injected Go
// block (so every injection level overlaps the changed range) and once
// after the whole fenced block (so no injection level does) — and asserts
// ParseIncremental's resulting injection tree matches a fresh Parse of the
// same edited text both times, and that the untouched-region edit reuses
// both levels' old child trees.
func TestInjectionParserIncrementalTwoLevelMatchesFreshParse(t *testing.T) {
	mdEntry := grammars.DetectLanguage("sample.md")
	if mdEntry == nil {
		t.Skip("Markdown grammar not available")
	}
	goEntry := grammars.DetectLanguage("main.go")
	if goEntry == nil {
		t.Skip("Go grammar not available")
	}
	mdLang := mdEntry.Language()
	goLang := goEntry.Language()
	langs := map[string]*gotreesitter.Language{"markdown": mdLang, "go": goLang}

	const mdInjectionQuery = `
(fenced_code_block
  (info_string (language) @injection.language)
  (code_fence_content) @injection.content)
`

	newParser := func() *gotreesitter.InjectionParser {
		ip := gotreesitter.NewInjectionParser()
		ip.RegisterLanguage("markdown", mdLang)
		ip.RegisterLanguage("go", goLang)
		if err := ip.RegisterInjectionQuery("markdown", mdInjectionQuery); err != nil {
			t.Fatalf("RegisterInjectionQuery: %v", err)
		}
		return ip
	}

	// A 4-backtick outer fence keeps the top-level Markdown parser from
	// treating the inner 3-backtick fence as its own closing delimiter: the
	// whole nested block is one opaque code_fence_content, and the
	// injection mechanism reparses it as an independent Markdown document,
	// where the inner fence is discovered fresh.
	source1 := []byte("# Doc\n\n````markdown\n## Nested\n\n```go\nfunc f0() int { return 0 }\n```\n````\n")

	ip := newParser()
	result1, err := ip.Parse(source1, "markdown")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result1.Injections) != 2 {
		t.Fatalf("expected 2 injections (two levels), got %d: %+v", len(result1.Injections), snapshotInjections(result1.Injections, langs))
	}
	if result1.Injections[0].Language != "markdown" || result1.Injections[1].Language != "go" {
		t.Fatalf("unexpected injection languages: %+v", snapshotInjections(result1.Injections, langs))
	}

	// Step 1: edit inside the innermost injected Go block. Both injection
	// levels' byte ranges overlap the changed range, so both must be
	// reparsed, and the resulting nested structure must still match a fresh
	// Parse of the edited text.
	idx := strings.Index(string(source1), "f0")
	if idx < 0 {
		t.Fatal("test source missing marker f0")
	}
	source2 := append(append(append([]byte{}, source1[:idx]...), []byte("f1")...), source1[idx+2:]...)
	startPoint := pointAt(source1, idx)
	edit1 := gotreesitter.InputEdit{
		StartByte:   uint32(idx),
		OldEndByte:  uint32(idx + 2),
		NewEndByte:  uint32(idx + 2),
		StartPoint:  startPoint,
		OldEndPoint: gotreesitter.Point{Row: startPoint.Row, Column: startPoint.Column + 2},
		NewEndPoint: gotreesitter.Point{Row: startPoint.Row, Column: startPoint.Column + 2},
	}
	result1.Tree.Edit(edit1)

	result2, err := ip.ParseIncremental(source2, "markdown", result1)
	if err != nil {
		t.Fatalf("ParseIncremental (step 1): %v", err)
	}

	freshIP := newParser()
	fresh2, err := freshIP.Parse(source2, "markdown")
	if err != nil {
		t.Fatalf("fresh Parse (step 1): %v", err)
	}

	assertSameInjectionShape(t, "step 1", snapshotInjections(result2.Injections, langs), snapshotInjections(fresh2.Injections, langs))

	// Step 2: edit the top-level heading text (same length, so the document's
	// top-level child count and every node's byte range below the heading
	// stay identical). Neither injection's absolute byte range overlaps the
	// changed range, so both should reuse their old child trees, and the
	// resulting structure must still match a fresh Parse.
	idx2 := strings.Index(string(source2), "Doc")
	if idx2 < 0 {
		t.Fatal("test source missing marker Doc")
	}
	source3 := append(append(append([]byte{}, source2[:idx2]...), []byte("DOC")...), source2[idx2+3:]...)
	headingPoint := pointAt(source2, idx2)
	edit2 := gotreesitter.InputEdit{
		StartByte:   uint32(idx2),
		OldEndByte:  uint32(idx2 + 3),
		NewEndByte:  uint32(idx2 + 3),
		StartPoint:  headingPoint,
		OldEndPoint: gotreesitter.Point{Row: headingPoint.Row, Column: headingPoint.Column + 3},
		NewEndPoint: gotreesitter.Point{Row: headingPoint.Row, Column: headingPoint.Column + 3},
	}
	result2.Tree.Edit(edit2)

	result3, err := ip.ParseIncremental(source3, "markdown", result2)
	if err != nil {
		t.Fatalf("ParseIncremental (step 2): %v", err)
	}

	fresh3, err := freshIP.Parse(source3, "markdown")
	if err != nil {
		t.Fatalf("fresh Parse (step 2): %v", err)
	}

	assertSameInjectionShape(t, "step 2", snapshotInjections(result3.Injections, langs), snapshotInjections(fresh3.Injections, langs))

	if len(result3.Injections) == len(result2.Injections) {
		for i := range result3.Injections {
			if result3.Injections[i].Tree != result2.Injections[i].Tree {
				t.Errorf("injection[%d] tree was reparsed instead of reused for an edit outside every injection", i)
			}
		}
	}
}

func assertSameInjectionShape(t *testing.T, label string, got, want []injectionSnapshot) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: injection count mismatch: got %d %+v, want %d %+v", label, len(got), got, len(want), want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("%s: injection[%d]: got %+v, want %+v", label, i, got[i], want[i])
		}
	}
}
