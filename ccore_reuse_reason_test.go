package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// A tree built by the certified core, given to a parser pinned to the legacy
// route, is reparsed fresh. The reported reason names the certified core, not
// the GSS forest fast path.
func TestLegacyReparseOfCertifiedCoreTreeReportsCoreReason(t *testing.T) {
	lang := grammars.JsonLanguage()
	oldTree, err := gts.NewParser(lang).Parse([]byte(`{"a": 1}`))
	if err != nil {
		t.Fatal(err)
	}
	defer oldTree.Release()
	if !gts.CcoreTestFromCore(oldTree) {
		t.Skip("the default route did not build this tree with the certified core")
	}
	oldTree.Edit(gts.InputEdit{StartByte: 6, OldEndByte: 7, NewEndByte: 8,
		StartPoint: gts.Point{Row: 0, Column: 6}, OldEndPoint: gts.Point{Row: 0, Column: 7}, NewEndPoint: gts.Point{Row: 0, Column: 8}})

	legacy := gts.NewParser(lang)
	legacy.SetAdmissionCandidateRoute(false)
	newTree, prof, err := legacy.ParseIncrementalProfiled([]byte(`{"a": 12}`), oldTree)
	if err != nil {
		t.Fatal(err)
	}
	defer newTree.Release()
	const want = "old tree was built by the certified core and is reparsed fresh by the legacy engine"
	if !prof.ReuseUnsupported || prof.ReuseUnsupportedReason != want {
		t.Fatalf("reuse unsupported = %t, reason = %q; want %q", prof.ReuseUnsupported, prof.ReuseUnsupportedReason, want)
	}
}
