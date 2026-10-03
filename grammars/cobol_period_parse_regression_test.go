package grammars

import (
	"fmt"
	"testing"

	ts "github.com/odvcencio/gotreesitter"
)

func TestCobolHiddenMissingTokenPreservesRootError(t *testing.T) {
	for _, candidate := range []bool{false, true} {
		t.Run(fmt.Sprintf("candidate=%t", candidate), func(t *testing.T) {
			parser := ts.NewParser(CobolLanguage())
			parser.SetAdmissionCandidateRoute(candidate)
			for _, tc := range []struct {
				source    string
				wantError bool
			}{
				{"       identification division.\n       program-id. .\n", true},
				{"       identification division.\n       program-id. a.\n", false},
			} {
				tree, err := parser.Parse([]byte(tc.source))
				if err != nil {
					t.Fatal(err)
				}
				root := tree.RootNode()
				gotError := root != nil && root.HasError()
				if root == nil || gotError != tc.wantError {
					tree.Release()
					t.Fatalf("HasError() = %v, want %t for %q", gotError, tc.wantError, tc.source)
				}
				tree.Release()
			}
		})
	}
}

// TestCobolPeriodKeepsDotTokenChildViaEngine proves that the reduce engine
// restores the anonymous `.` token child of `period` on a real parse, without
// help from the (now removed) cobol/COBOL resultCollapsedNamedLeafRules
// rows. shouldKeepVisibleAnonymousTokenChild keeps different-named
// single-token-wrapper anonymous children unconditionally, so `period`
// (named) wrapping `.` (anonymous) is never collapsed to a childless leaf in
// the first place.
func TestCobolPeriodKeepsDotTokenChildViaEngine(t *testing.T) {
	lang := CobolLanguage()
	parser := ts.NewParser(lang)
	src := []byte("       IDENTIFICATION DIVISION.\n       PROGRAM-ID. HELLO.\n       PROCEDURE DIVISION.\n           DISPLAY \"HI\".\n           STOP RUN.\n")
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("missing root node")
	}
	if tree.ParseStopReason() != ts.ParseStopAccepted {
		t.Fatalf("stop=%s runtime=%s", tree.ParseStopReason(), tree.ParseRuntime().Summary())
	}
	if root.HasError() {
		t.Fatalf("expected minimal cobol program to parse cleanly, got %s", root.SExpr(lang))
	}

	var periods []*ts.Node
	var walk func(*ts.Node)
	walk = func(n *ts.Node) {
		if n == nil {
			return
		}
		if n.IsNamed() && n.Type(lang) == "period" {
			periods = append(periods, n)
		}
		for i := 0; i < n.NamedChildCount(); i++ {
			walk(n.NamedChild(i))
		}
	}
	walk(root)
	if len(periods) == 0 {
		t.Fatalf("no period nodes found; tree=%s", root.SExpr(lang))
	}
	for _, period := range periods {
		if got := period.ChildCount(); got != 1 {
			t.Fatalf("period child count = %d, want 1; node=%s tree=%s", got, period.SExpr(lang), root.SExpr(lang))
		}
		child := period.Child(0)
		if child == nil {
			t.Fatalf("period missing dot child; node=%s", period.SExpr(lang))
		}
		if child.Type(lang) != "." || child.IsNamed() {
			t.Fatalf("period child type/named = %q/%v, want ./false; node=%s", child.Type(lang), child.IsNamed(), period.SExpr(lang))
		}
		if child.StartByte() != period.StartByte() || child.EndByte() != period.EndByte() {
			t.Fatalf("dot child byte range = [%d,%d), want [%d,%d) to match parent", child.StartByte(), child.EndByte(), period.StartByte(), period.EndByte())
		}
	}
}
