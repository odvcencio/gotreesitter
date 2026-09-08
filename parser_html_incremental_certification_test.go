package gotreesitter_test

import (
	"fmt"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestHTMLIncrementalScannerCertification proves that HTML's exact tag-stack
// checkpoints support changed-length edits without sacrificing fresh-parse
// equality. The ordinary lane crosses a clean 4 KiB document at three
// positions; the 137 KiB lane guards admission and useful reuse at the macro
// size without claiming O(edit) work. Error trees have a separate fail-closed
// witness below.
func TestHTMLIncrementalScannerCertification(t *testing.T) {
	plans := []struct {
		name      string
		size      int
		positions []string
		classes   []htmlCertificationEditClass
	}{
		{
			name:      "4KiB",
			size:      4 << 10,
			positions: []string{"start", "middle", "end"},
			classes:   []htmlCertificationEditClass{htmlCertificationInsert, htmlCertificationDelete, htmlCertificationReplace},
		},
		{
			name:      "137KiB",
			size:      137 << 10,
			positions: []string{"middle"},
			classes:   []htmlCertificationEditClass{htmlCertificationReplace},
		},
	}

	for _, plan := range plans {
		source, sites := makeHTMLCertificationSource(plan.size, false)
		for _, position := range plan.positions {
			var site int
			switch position {
			case "start":
				site = sites[1]
			case "middle":
				site = sites[len(sites)/2]
			case "end":
				site = sites[len(sites)-2]
			default:
				t.Fatalf("unknown HTML certification position %q", position)
			}
			for _, class := range plan.classes {
				t.Run(fmt.Sprintf("%s/%s/%s", plan.name, class, position), func(t *testing.T) {
					edited, edit := applyHTMLCertificationEdit(source, site, class)
					profile := runHTMLCertificationSample(t, source, edited, edit)
					if plan.size >= 137<<10 && profile.ReusedBytes*100 < uint64(len(edited))*25 {
						t.Fatalf("HTML certification reused only %d/%d bytes (<25%%): %+v", profile.ReusedBytes, len(edited), profile)
					}
				})
			}
		}
	}
}

type htmlCertificationEditClass uint8

const (
	htmlCertificationInsert htmlCertificationEditClass = iota
	htmlCertificationDelete
	htmlCertificationReplace
)

func (c htmlCertificationEditClass) String() string {
	switch c {
	case htmlCertificationInsert:
		return "insert"
	case htmlCertificationDelete:
		return "delete"
	case htmlCertificationReplace:
		return "replace"
	default:
		return "unknown"
	}
}

func makeHTMLCertificationSource(target int, recovered bool) ([]byte, []int) {
	var source strings.Builder
	source.Grow(target + 256)
	source.WriteString("<!doctype html><html><body><main>\n")
	sites := make([]int, 0, target/96)
	for i := 0; source.Len() < target || len(sites) < 4; i++ {
		source.WriteString("<section><custom-widget data-value=\"")
		sites = append(sites, source.Len())
		fmt.Fprintf(&source, "%06d\">payload_%06d</custom-widget></section>\n", i, i)
	}
	if recovered {
		// An empty tag name forces a local recovery after the stateful scanner
		// has carried the non-empty html/body/main stack across every edit site.
		source.WriteString("<section><></section>\n")
	}
	source.WriteString("</main></body></html>\n")
	return []byte(source.String()), sites
}

func applyHTMLCertificationEdit(source []byte, offset int, class htmlCertificationEditClass) ([]byte, gts.InputEdit) {
	oldEnd := offset
	replacement := []byte{'9'}
	switch class {
	case htmlCertificationDelete:
		oldEnd = offset + 1
		replacement = nil
	case htmlCertificationReplace:
		oldEnd = offset + 1
		replacement = []byte("88")
	}
	edited := make([]byte, 0, len(source)-(oldEnd-offset)+len(replacement))
	edited = append(edited, source[:offset]...)
	edited = append(edited, replacement...)
	edited = append(edited, source[oldEnd:]...)
	return edited, gts.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(oldEnd),
		NewEndByte:  uint32(offset + len(replacement)),
		StartPoint:  pointForOffset(source, offset),
		OldEndPoint: pointForOffset(source, oldEnd),
		NewEndPoint: pointForOffset(edited, offset+len(replacement)),
	}
}

func runHTMLCertificationSample(t *testing.T, source, edited []byte, edit gts.InputEdit) gts.IncrementalParseProfile {
	t.Helper()
	lang := grammars.HtmlLanguage()
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	oldTree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("initial HTML parse: %v", err)
	}
	defer oldTree.Release()
	initialRoot := requireCompleteParse(t, oldTree, source, lang, "initial HTML certification")
	requireHTMLAcceptedFullSource(t, oldTree, source, "initial HTML certification")
	if initialRoot.HasError() {
		t.Fatalf("initial HTML parse produced ERROR: %s", initialRoot.SExpr(lang))
	}

	oldTree.Edit(edit)
	incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("incremental HTML parse: %v", err)
	}
	defer incremental.Release()
	incrementalRoot := requireCompleteParse(t, incremental, edited, lang, "incremental HTML certification")
	requireHTMLAcceptedFullSource(t, incremental, edited, "incremental HTML certification")
	if profile.ReuseUnsupported {
		t.Fatalf("certified HTML scanner fell back: reason=%q", profile.ReuseUnsupportedReason)
	}
	if !profile.OldTreeReuseRoute || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
		t.Fatalf("certified HTML scanner did not reuse old syntax: %+v", profile)
	}
	if incrementalRoot.HasError() {
		t.Fatalf("incremental HTML parse produced ERROR: %s", incrementalRoot.SExpr(lang))
	}

	freshParser := gts.NewParser(lang)
	freshParser.SetAdmissionCandidateRoute(false)
	fresh, err := freshParser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh HTML parse: %v", err)
	}
	defer fresh.Release()
	requireCompleteParse(t, fresh, edited, lang, "fresh HTML certification")
	requireHTMLAcceptedFullSource(t, fresh, edited, "fresh HTML certification")
	requireIncrementalDeepTreeMatchesFresh(t, incremental, fresh, lang)
	return profile
}

func TestHTMLErrorTreeIncrementalReuseFailsClosed(t *testing.T) {
	source, sites := makeHTMLCertificationSource(4<<10, true)
	edited, edit := applyHTMLCertificationEdit(source, sites[1], htmlCertificationInsert)
	lang := grammars.HtmlLanguage()
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	oldTree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("initial recovered HTML parse: %v", err)
	}
	defer oldTree.Release()
	oldRoot := requireCompleteParse(t, oldTree, source, lang, "initial recovered HTML")
	if !oldRoot.HasError() {
		t.Fatalf("recovery witness did not produce an error tree: %s", oldRoot.SExpr(lang))
	}
	oldTree.Edit(edit)

	incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("recovered HTML incremental parse: %v", err)
	}
	defer incremental.Release()
	if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "external_scanner_error_tree_unsupported" ||
		profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
		t.Fatalf("HTML error tree did not fail closed: %+v", profile)
	}

	freshParser := gts.NewParser(lang)
	freshParser.SetAdmissionCandidateRoute(false)
	fresh, err := freshParser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh recovered HTML parse: %v", err)
	}
	defer fresh.Release()
	requireCompleteParse(t, incremental, edited, lang, "incremental recovered HTML fallback")
	requireCompleteParse(t, fresh, edited, lang, "fresh recovered HTML")
	requireIncrementalDeepTreeMatchesFresh(t, incremental, fresh, lang)
}

func requireHTMLAcceptedFullSource(t *testing.T, tree *gts.Tree, source []byte, phase string) {
	t.Helper()
	rt := tree.ParseRuntime()
	if tree.ParseStoppedEarly() || rt.StopReason != gts.ParseStopAccepted || rt.Truncated ||
		rt.ExpectedEOFByte != uint32(len(source)) {
		t.Fatalf("%s: parse was not accepted, non-truncated, and full-source: %s", phase, rt.Summary())
	}
}
