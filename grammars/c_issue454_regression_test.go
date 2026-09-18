package grammars_test

import (
	"bytes"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// TestIssue454CIncrementalDeleteMatchesFresh checks the original C deletion and its clean controls.
// The deletion previously exhausted the reuse budget and required a full retry.
// It now reuses the unchanged suffix. Require complete fresh-tree equality and genuine reuse for all three edits.
// Direct budget tests separately preserve the allocation limit and full-retry rules.
func TestIssue454CIncrementalDeleteMatchesFresh(t *testing.T) {
	lang := grammars.CLanguage()
	source := benchfixtures.Issue454CSource()
	if got := len(source); got != benchfixtures.Issue454CFixtureBytes {
		t.Fatalf("fixture bytes = %d, want %d", got, benchfixtures.Issue454CFixtureBytes)
	}

	baseShape := issue454CFreshShape(t, lang, source)
	if baseShape.hasError {
		t.Fatal("base fresh parse has an error")
	}
	for run := 2; run <= 3; run++ {
		got := issue454CFreshShape(t, lang, source)
		if got != baseShape {
			t.Fatalf("fresh base parse %d changed: got %+v, want %+v", run, got, baseShape)
		}
	}

	site := bytes.Index(source, []byte("x0"))
	if site < 0 {
		t.Fatal("C edit marker is absent")
	}

	tests := []struct {
		name    string
		edited  []byte
		oldEnd  int
		newEnd  int
		oldCols uint32
		newCols uint32
	}{
		{
			name:    "replace",
			edited:  append([]byte(nil), source...),
			oldEnd:  site + 1,
			newEnd:  site + 1,
			oldCols: 1,
			newCols: 1,
		},
		{
			name:    "insert",
			edited:  append(append(append([]byte(nil), source[:site]...), 'x'), source[site:]...),
			oldEnd:  site,
			newEnd:  site + 1,
			oldCols: 0,
			newCols: 1,
		},
		{
			name:    "delete",
			edited:  append(append([]byte(nil), source[:site]...), source[site+1:]...),
			oldEnd:  site + 1,
			newEnd:  site,
			oldCols: 1,
			newCols: 0,
		},
	}
	tests[0].edited[site] = 'y'

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			oldTree, err := gotreesitter.NewParser(lang).Parse(source)
			if err != nil {
				t.Fatalf("old Parse: %v", err)
			}
			defer oldTree.Release()
			point := issue454PointAt(source, site)
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(site),
				OldEndByte:  uint32(test.oldEnd),
				NewEndByte:  uint32(test.newEnd),
				StartPoint:  point,
				OldEndPoint: gotreesitter.Point{Row: point.Row, Column: point.Column + test.oldCols},
				NewEndPoint: gotreesitter.Point{Row: point.Row, Column: point.Column + test.newCols},
			})

			incremental, profile, err := gotreesitter.NewParser(lang).ParseIncrementalProfiled(test.edited, oldTree)
			if err != nil {
				t.Fatalf("ParseIncrementalProfiled: %v", err)
			}
			defer incremental.Release()
			if incremental.ParseStoppedEarly() {
				t.Fatalf("incremental parse stopped early: %s", incremental.ParseRuntime().Summary())
			}

			fresh, err := gotreesitter.NewParser(lang).Parse(test.edited)
			if err != nil {
				t.Fatalf("fresh edited Parse: %v", err)
			}
			defer fresh.Release()

			got := issue454CTreeShape(t, lang, incremental)
			want := issue454CTreeShape(t, lang, fresh)
			if got != want {
				t.Fatalf("incremental shape = %+v, fresh shape = %+v", got, want)
			}
			t.Logf("exact fresh digest=%s nodes=%d reused=%d/%d unsupported=%v", got.digest, got.nodes, profile.ReusedBytes, len(test.edited), profile.ReuseUnsupported)
			if profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "" || !profile.OldTreeReuseRoute || profile.ReusedSubtrees == 0 {
				t.Fatalf("%s: expected genuine old-tree reuse, profile = %+v", test.name, profile)
			}
			if profile.ReusedBytes == 0 || profile.ReusedBytes > uint64(len(test.edited)) {
				t.Fatalf("reused bytes = %d, source bytes = %d", profile.ReusedBytes, len(test.edited))
			}
		})
	}
}

type issue454CShape struct {
	digest   string
	nodes    uint64
	rootType string
	hasError bool
	endByte  uint32
}

func issue454CFreshShape(t *testing.T, lang *gotreesitter.Language, source []byte) issue454CShape {
	t.Helper()
	tree, err := gotreesitter.NewParser(lang).Parse(source)
	if err != nil {
		t.Fatalf("fresh Parse: %v", err)
	}
	defer tree.Release()
	return issue454CTreeShape(t, lang, tree)
}

func issue454CTreeShape(
	t *testing.T,
	lang *gotreesitter.Language,
	tree *gotreesitter.Tree,
) issue454CShape {
	t.Helper()
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned no root")
	}
	if tree.ParseStoppedEarly() {
		t.Fatalf("parse stopped early: %s", tree.ParseRuntime().Summary())
	}
	root := tree.RootNode()
	if got, want := root.EndByte(), uint32(len(tree.Source())); got != want {
		t.Fatalf("root end = %d, want %d", got, want)
	}
	inspection, err := benchfixtures.InspectGoTree(root, lang)
	if err != nil {
		t.Fatalf("inspect tree: %v", err)
	}
	var nodes uint64
	for _, count := range inspection.NodeKinds {
		nodes += count
	}
	return issue454CShape{
		digest:   inspection.SHA256,
		nodes:    nodes,
		rootType: root.Type(lang),
		hasError: root.HasError(),
		endByte:  root.EndByte(),
	}
}
