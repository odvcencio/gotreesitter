package grammars_test

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"testing"
)

func TestRecoveryLonePipeKeepsLiveStack(t *testing.T) {
	lang := grammars.DetectLanguageByName("fsharp").Language()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse([]byte("|"))
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if tree.ParseStopReason() != gotreesitter.ParseStopAccepted || root.EndByte() != 1 || !root.HasError() || root.SExpr(lang) != "(file (ERROR))" {
		t.Fatalf("stop=%s tree=%s span=%d..%d error=%v", tree.ParseStopReason(), root.SExpr(lang), root.StartByte(), root.EndByte(), root.HasError())
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
	for _, text := range []string{"||", "", "|", "let x = 1\n|"} {
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
		fresh.Release()
		old.Release()
		old, source = next, nextSource
	}
	allocations := testing.AllocsPerRun(100, func() {
		next, err := parser.ParseIncremental(source, old)
		if err != nil {
			panic(err)
		}
		next.Release()
	})
	if allocations != 0 {
		t.Fatalf("no-edit reparse allocated %.2f times", allocations)
	}
}
