package gotreesitter_test

import (
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestCIncrementalReductionChainMatchesFresh(t *testing.T) {
	entry := grammars.DetectLanguageByName("c")
	lang := entry.Language()
	source, err := os.ReadFile("testdata/incremental_gate/c_repeated_functions.c")
	if err != nil {
		t.Fatal(err)
	}
	parseFresh := func(source []byte) *gts.Tree {
		tree, err := gts.NewParser(lang).ParseWithTokenSource(source, entry.TokenSourceFactory(source, lang))
		if err != nil {
			t.Fatal(err)
		}
		return tree
	}
	baseline := parseFresh(source)
	defer baseline.Release()
	for _, editClass := range []string{"delete", "replace"} {
		t.Run(editClass, func(t *testing.T) {
			// This edit needs more than 256 reductions of the reused prefix.
			edited, edit := incrGateBuildEdit(source, 23995, editClass)
			fresh := parseFresh(edited)
			defer fresh.Release()
			if errors, missing := incrGateTreeStats(fresh.RootNode()); errors != 0 || missing != 0 {
				t.Fatalf("fresh fixture has %d error nodes and %d missing nodes", errors, missing)
			}
			for _, profiled := range []bool{false, true} {
				name := "ordinary"
				if profiled {
					name = "profiled"
				}
				t.Run(name, func(t *testing.T) {
					old := baseline.Copy()
					defer old.Release()
					old.Edit(edit)
					parser := gts.NewParser(lang)
					tokens := entry.TokenSourceFactory(edited, lang)
					var tree *gts.Tree
					var err error
					if profiled {
						tree, _, err = parser.ParseIncrementalWithTokenSourceProfiled(edited, old, tokens)
					} else {
						tree, err = parser.ParseIncrementalWithTokenSource(edited, old, tokens)
					}
					if err != nil {
						t.Fatal(err)
					}
					defer tree.Release()
					if runtime := tree.ParseRuntime(); runtime.Truncated || runtime.StopReason != gts.ParseStopAccepted || !runtime.IncrementalOldTreeReuseRoute {
						t.Fatalf("incremental parse did not finish: %s", runtime.Summary())
					}
					if difference := incrGateFirstDivergence(lang, fresh.RootNode(), tree.RootNode(), nil); difference != nil {
						t.Fatalf("incremental tree differs from fresh: %+v", difference)
					}
				})
			}
		})
	}
}
