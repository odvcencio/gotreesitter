package grammars_test

import (
	"os"
	"path/filepath"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestRecoveryDoxygenSkippedInputCoversEOF(t *testing.T) {
	lang := grammars.DetectLanguageByName("doxygen").Language()
	for _, file := range []string{"medium__CMakeLists.txt", "medium__metrics.py", "small__example.cfg"} {
		t.Run(file, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join("..", "testdata", "dispatcher_census_a0", "doxygen", file))
			if err != nil {
				t.Fatal(err)
			}
			parser := gotreesitter.NewParser(lang)
			old, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { old.Release() }()
			for _, nextSource := range [][]byte{append(append([]byte{}, source...), ' '), source} {
				old.Edit(spliceTestEdit(source, nextSource))
				next, err := parser.ParseIncremental(nextSource, old)
				if err != nil {
					t.Fatal(err)
				}
				fresh, err := gotreesitter.NewParser(lang).Parse(nextSource)
				if err != nil {
					t.Fatal(err)
				}
				if diff := spliceTreeDiff(next.RootNode(), fresh.RootNode(), lang, ""); diff != "" {
					t.Fatal(diff)
				}
				root := next.RootNode()
				if next.ParseStopReason() != gotreesitter.ParseStopAccepted || root.StartByte() != 0 || root.EndByte() != uint32(len(nextSource)) || !root.HasError() {
					t.Fatalf("stop=%s span=%d..%d input=%d error=%t", next.ParseStopReason(), root.StartByte(), root.EndByte(), len(nextSource), root.HasError())
				}
				fresh.Release()
				old.Release()
				old, source = next, nextSource
				if allocations := testing.AllocsPerRun(100, func() {
					unchanged, err := parser.ParseIncremental(source, old)
					if err != nil {
						panic(err)
					}
					unchanged.Release()
				}); allocations != 0 {
					t.Fatalf("no-edit reparse allocated %.2f times", allocations)
				}
			}
		})
	}
}

func TestRecoveryLonePipeKeepsLiveStack(t *testing.T) {
	lang := grammars.DetectLanguageByName("fsharp").Language()
	for _, source := range []string{"|", "|>"} {
		t.Run(source, func(t *testing.T) {
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse([]byte(source))
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			root := tree.RootNode()
			if tree.ParseStopReason() != gotreesitter.ParseStopAccepted || root.StartByte() != 0 || root.EndByte() != uint32(len(source)) || !root.HasError() || root.SExpr(lang) != "(file (ERROR))" {
				t.Fatalf("stop=%s tree=%s span=%d..%d error=%v", tree.ParseStopReason(), root.SExpr(lang), root.StartByte(), root.EndByte(), root.HasError())
			}
		})
	}
}

func TestRecoverySkippedEOFIncrementalMatchesFresh(t *testing.T) {
	lang := grammars.DetectLanguageByName("fsharp").Language()
	parser := gotreesitter.NewParser(lang)
	source := []byte("|")
	old, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { old.Release() }()
	for _, text := range []string{"|>", "||", "", "|", "let x = 1\n|", "let x = 1\n|>"} {
		nextSource := []byte(text)
		old.Edit(spliceTestEdit(source, nextSource))
		next, err := parser.ParseIncremental(nextSource, old)
		if err != nil {
			t.Fatal(err)
		}
		fresh, err := gotreesitter.NewParser(lang).Parse(nextSource)
		if err != nil {
			t.Fatal(err)
		}
		if diff := spliceTreeDiff(next.RootNode(), fresh.RootNode(), lang, ""); diff != "" {
			t.Fatalf("input %q: %s", text, diff)
		}
		if next.ParseStopReason() != gotreesitter.ParseStopAccepted || next.RootNode().StartByte() != 0 || next.RootNode().EndByte() != uint32(len(nextSource)) {
			t.Fatalf("input %q: stop=%s span=%d..%d", text, next.ParseStopReason(), next.RootNode().StartByte(), next.RootNode().EndByte())
		}
		if next.RootNode().IsError() && !next.RootNode().HasError() {
			t.Fatalf("input %q: ERROR root does not report HasError", text)
		}
		fresh.Release()
		old.Release()
		old, source = next, nextSource
		allocations := testing.AllocsPerRun(100, func() {
			unchanged, err := parser.ParseIncremental(source, old)
			if err != nil {
				panic(err)
			}
			unchanged.Release()
		})
		if allocations != 0 {
			t.Fatalf("input %q: no-edit reparse allocated %.2f times", text, allocations)
		}
	}
}
