package grammars_test

import (
	"bytes"
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestMakeLargeFileTypingAtEndMatchesFresh(t *testing.T) {
	entry := grammars.DetectLanguageByName("make")
	if entry == nil || entry.Language() == nil {
		t.Fatal("make language unavailable")
	}
	lang := entry.Language()
	for _, tc := range []struct {
		name     string
		children int
	}{
		{"large", 4200},
		{"probe_4000", 4000},
		{"probe_4090", 4090},
		{"probe_4094", 4094},
		{"below_boundary", 4095},
		{"at_boundary", 4096},
		{"above_boundary", 4097},
		{"control", 3000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var source bytes.Buffer
			for i := 0; i < tc.children; i++ {
				fmt.Fprintf(&source, "t%d: d%d\n\t@echo %d\n\n", i, i, i)
			}
			before := source.Bytes()
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse(before)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { tree.Release() }()
			if got := tree.RootNode().ChildCount(); got < tc.children {
				t.Fatalf("root has %d children, want at least %d", got, tc.children)
			}
			t.Logf("source bytes=%d root children=%d runtime=%s", len(before), tree.RootNode().ChildCount(), tree.ParseRuntime().Summary())
			for i, b := range []byte("g: dep\n\t@echo g\n") {
				after := append(append([]byte(nil), before...), b)
				tree.Edit(spliceTestEdit(before, after))
				incremental, err := parser.ParseIncremental(after, tree)
				if err != nil {
					t.Fatalf("keystroke %d: incremental parse: %v", i+1, err)
				}
				fresh, err := gotreesitter.NewParser(lang).Parse(after)
				if err != nil {
					incremental.Release()
					t.Fatalf("keystroke %d: fresh parse: %v", i+1, err)
				}
				diff := spliceTreeDiff(incremental.RootNode(), fresh.RootNode(), lang, "")
				if i == 0 {
					t.Logf("keystroke 1 incremental=%s fresh=%s", incremental.ParseRuntime().Summary(), fresh.ParseRuntime().Summary())
				}
				if diff != "" {
					t.Errorf("keystroke %d (%q), bytes=%d, base children=%d: %s\nincremental tail: %s\nfresh tail: %s\nincremental runtime: %s (retry attempts=%d adopted=%v cause=%d old-tree-reuse=%v)\nfresh runtime: %s",
						i+1, b, len(after), tc.children, diff,
						makeLargeFileLastChildren(incremental.RootNode(), lang),
						makeLargeFileLastChildren(fresh.RootNode(), lang), incremental.ParseRuntime().Summary(), incremental.ParseRuntime().IncrementalAcceptedErrorRetryAttempts, incremental.ParseRuntime().IncrementalAcceptedErrorRetryAdopted, incremental.ParseRuntime().IncrementalAcceptedErrorRetryCause, incremental.ParseRuntime().IncrementalOldTreeReuseRoute, fresh.ParseRuntime().Summary())
					fresh.Release()
					incremental.Release()
					return
				}
				fresh.Release()
				tree.Release()
				tree, before = incremental, after
			}
		})
	}
}

func makeLargeFileLastChildren(root *gotreesitter.Node, lang *gotreesitter.Language) string {
	if root == nil {
		return "<nil>"
	}
	var tail []string
	for i := max(0, root.ChildCount()-2); i < root.ChildCount(); i++ {
		tail = append(tail, root.Child(i).SExpr(lang))
	}
	return fmt.Sprint(tail)
}
