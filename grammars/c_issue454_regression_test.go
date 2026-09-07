package grammars_test

import (
	"bytes"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// TestIssue454CIncrementalDeleteMatchesFresh is the named regression case for
// issue #454's C incremental-delete defect: ParseIncremental on a ~137 KiB
// repeated-function C fixture, after a single-byte delete at the first "x0"
// identifier (a transient-error edit: "x0" loses its leading letter and
// becomes the bare integer literal "0") that defeats incremental reuse,
// returned a 1-node ERROR tree against a fresh parse of tens of thousands of
// nodes.
//
// Root cause: the incremental GLR loop explored pathological ambiguity near
// the malformed declaration (observed ~3.08M allocated nodes for a ~64K-node
// file -- about 48x a clean parse) and tripped ParseStopMemoryBudget. The
// plain (DFA) ParseIncremental / ParseIncrementalProfiled entry points had no
// fail-closed fallback for that stop reason at all; the TokenSource entry
// points had a retry-as-full ladder (shouldRetryIncrementalParseAsFull) but
// it never triggers for ParseStopMemoryBudget either, since neither
// fullParseRetryMaxStacksOverrideForOrigin nor fullParseRetryNodeLimitOverride
// key off that stop reason (and widening the GLR stack/node cap is the wrong
// direction for a runaway-allocation failure regardless). The fix
// (shouldRetryIncrementalMemoryBudgetAsPlainFull and its DFA/TokenSource
// fallbacks, parser_retry.go) discards a memory-budget-aborted incremental
// attempt and substitutes exactly one plain, default-budget full parse. This
// test proves the result converges to a byte-identical tree (full deep-digest
// comparison, the same digest stream the cgo oracle uses) and that the other
// two single-byte edit classes at the same site keep genuine old-tree reuse
// (this fix is scoped narrower than c_sharp/php's blanket reuse decline).
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
		// wantFallbackReason is non-empty only for the edit class expected to
		// trip a fail-closed plain full retry (the reuse budget, or the memory
		// budget behind it).
		wantFallbackReason string
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
			// The incremental reuse budget now stops this attempt before the
			// memory budget does; both take the same plain full retry.
			wantFallbackReason: "incremental_parse_reuse_budget_full_retry",
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

			if test.wantFallbackReason != "" {
				if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != test.wantFallbackReason {
					t.Fatalf("%s: expected the fail-closed plain full retry %q, profile = %+v", test.name, test.wantFallbackReason, profile)
				}
			} else if profile.ReusedSubtrees == 0 {
				t.Fatalf("%s: expected genuine old-tree reuse, got none: profile = %+v", test.name, profile)
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
