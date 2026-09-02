package gotreesitter_test

import (
	"bytes"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestCheckpointedScannerPrefixFrontierFallback covers production trees.
// Checkpoint bytes prove scanner state, but they do not prove the reduction
// frontier after an edit inside the first structural top-level child.
func TestCheckpointedScannerPrefixFrontierFallback(t *testing.T) {
	cases := []struct {
		name    string
		lang    func() *gts.Language
		source  []byte
		needle  []byte
		replace []byte
	}{
		{
			name:    "python_insert_first_child",
			lang:    grammars.PythonLanguage,
			source:  []byte("def first():\n    return 1\n\ndef second():\n    return 2\n"),
			needle:  []byte("return 1"),
			replace: []byte("return 19"),
		},
		{
			name:    "python_delete_first_child",
			lang:    grammars.PythonLanguage,
			source:  []byte("def first():\n    return 111\n\ndef second():\n    return 2\n"),
			needle:  []byte("return 111"),
			replace: []byte("return 11"),
		},
		{
			name:    "python_same_length_indent_state",
			lang:    grammars.PythonLanguage,
			source:  []byte("def first():\n        value = 1\n        return value\n\ndef second():\n    return 2\n"),
			needle:  []byte("        return value"),
			replace: []byte("\t       return value"),
		},
		{
			name:    "python_leading_comment_extra",
			lang:    grammars.PythonLanguage,
			source:  []byte("# leading extra\n\ndef first():\n    return 1\n\ndef second():\n    return 2\n"),
			needle:  []byte("return 1"),
			replace: []byte("return 19"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lang := tc.lang()
			parser := gts.NewParser(lang)
			parser.SetAdmissionCandidateRoute(false)

			oldTree, err := parser.Parse(tc.source)
			if err != nil {
				t.Fatalf("old parse: %v", err)
			}
			defer oldTree.Release()
			requireCompleteParse(t, oldTree, tc.source, lang, "old")

			edited, edit := replacePrefixFrontierWitness(t, tc.source, tc.needle, tc.replace)
			oldTree.Edit(edit)
			incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			defer incremental.Release()
			requireCompleteParse(t, incremental, edited, lang, "incremental")
			if profile.ReuseUnsupportedReason != "external_scanner_prefix_frontier_unproven" {
				t.Fatalf("fallback reason = %q, want external_scanner_prefix_frontier_unproven: %+v", profile.ReuseUnsupportedReason, profile)
			}
			if !profile.ReuseUnsupported || profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
				t.Fatalf("prefix fallback reported reuse: %+v", profile)
			}
			if profile.TokensConsumed == 0 || profile.NewNodesAllocated == 0 {
				t.Fatalf("prefix fallback did not report fresh parse work: %+v", profile)
			}
			if want := uint64(incremental.ParseRuntime().NodesAllocated); profile.NewNodesAllocated != want {
				t.Fatalf("prefix fallback node attribution=%d, want fresh tree node count %d: %+v", profile.NewNodesAllocated, want, profile)
			}

			freshParser := gts.NewParser(lang)
			freshParser.SetAdmissionCandidateRoute(false)
			fresh, err := freshParser.Parse(edited)
			if err != nil {
				t.Fatalf("fresh parse: %v", err)
			}
			defer fresh.Release()
			requireCompleteParse(t, fresh, edited, lang, "fresh")
			requireIncrementalDeepTreeMatchesFresh(t, incremental, fresh, lang)
		})
	}
}

func TestCheckpointedScannerPrefixFrontierAllowsLaterSiblingReuse(t *testing.T) {
	cases := []struct {
		name    string
		lang    func() *gts.Language
		source  []byte
		needle  []byte
		replace []byte
	}{
		{
			name:    "python_insert_second_child",
			lang:    grammars.PythonLanguage,
			source:  []byte("def first():\n    return 1\n\ndef second():\n    return 2\n"),
			needle:  []byte("return 2"),
			replace: []byte("return 29"),
		},
		{
			name:    "python_delete_second_child_pre_edit_coordinates",
			lang:    grammars.PythonLanguage,
			source:  []byte("def first():\n    return 1\n\ndef second():\n    return 222\n"),
			needle:  []byte("return 222"),
			replace: []byte("return 22"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lang := tc.lang()
			parser := gts.NewParser(lang)
			parser.SetAdmissionCandidateRoute(false)
			oldTree, err := parser.Parse(tc.source)
			if err != nil {
				t.Fatalf("old parse: %v", err)
			}
			defer oldTree.Release()
			requireCompleteParse(t, oldTree, tc.source, lang, "old")

			edited, edit := replacePrefixFrontierWitness(t, tc.source, tc.needle, tc.replace)
			oldTree.Edit(edit)
			incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			defer incremental.Release()
			requireCompleteParse(t, incremental, edited, lang, "incremental")
			if profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "" || !profile.OldTreeReuseRoute {
				t.Fatalf("later-sibling edit did not stay on reuse route: %+v", profile)
			}
			if profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
				t.Fatalf("later-sibling edit lost all reuse: %+v", profile)
			}

			freshParser := gts.NewParser(lang)
			freshParser.SetAdmissionCandidateRoute(false)
			fresh, err := freshParser.Parse(edited)
			if err != nil {
				t.Fatalf("fresh parse: %v", err)
			}
			defer fresh.Release()
			requireCompleteParse(t, fresh, edited, lang, "fresh")
			requireIncrementalDeepTreeMatchesFresh(t, incremental, fresh, lang)
		})
	}
}

// TestCheckpointedScannerWithoutPrefixFrontierRequirementKeepsReuse proves
// that Python's ownership guard does not reduce other scanner certifications.
func TestCheckpointedScannerWithoutPrefixFrontierRequirementKeepsReuse(t *testing.T) {
	source := []byte("SELECT $q$first$q$ AS first;\nSELECT $q$second$q$ AS second;\n")
	lang := grammars.SqlLanguage()
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	oldTree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("old SQL parse: %v", err)
	}
	defer oldTree.Release()
	requireSQLAcceptedEOF(t, oldTree, source, "old")

	edited, edit := replacePrefixFrontierWitness(t, source, []byte("first"), []byte("firstx"))
	oldTree.Edit(edit)
	incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("incremental SQL parse: %v", err)
	}
	defer incremental.Release()
	if profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "" || !profile.OldTreeReuseRoute ||
		profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
		t.Fatalf("SQL prefix edit lost certified reuse: %+v", profile)
	}
	requireSQLAcceptedEOF(t, incremental, edited, "incremental")

	freshParser := gts.NewParser(lang)
	freshParser.SetAdmissionCandidateRoute(false)
	fresh, err := freshParser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh SQL parse: %v", err)
	}
	defer fresh.Release()
	requireSQLAcceptedEOF(t, fresh, edited, "fresh")
	requireIncrementalDeepTreeMatchesFresh(t, incremental, fresh, lang)
}

// TestCheckpointedScannerPrefixFrontierCatchesSuffixDelete covers the
// post-edit boundary case where a deletion moves the first child end to the
// edit start. The old raw-coordinate check treated that boundary as clean.
func TestCheckpointedScannerPrefixFrontierCatchesSuffixDelete(t *testing.T) {
	source := []byte("def first():\n    return 111\n\ndef second():\n    return 2\n")
	parser := gts.NewParser(grammars.PythonLanguage())
	parser.SetAdmissionCandidateRoute(false)
	oldTree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("old parse: %v", err)
	}
	defer oldTree.Release()
	first := oldTree.RootNode().Child(0)
	if first == nil || first.EndByte() == 0 {
		t.Fatal("fixture has no first top-level child")
	}
	offset := int(first.EndByte()) - 1
	if source[offset] < '0' || source[offset] > '9' {
		t.Fatalf("first-child suffix byte=%q, want a digit", source[offset])
	}
	edited := append(append([]byte(nil), source[:offset]...), source[offset+1:]...)
	edit := gts.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(offset + 1),
		NewEndByte:  uint32(offset),
		StartPoint:  pointForOffset(source, offset),
		OldEndPoint: pointForOffset(source, offset+1),
		NewEndPoint: pointForOffset(edited, offset),
	}
	oldTree.Edit(edit)
	incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("suffix-delete incremental parse: %v", err)
	}
	defer incremental.Release()
	if profile.ReuseUnsupportedReason != "external_scanner_prefix_frontier_unproven" ||
		!profile.ReuseUnsupported || profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
		t.Fatalf("suffix deletion bypassed frontier fallback: %+v", profile)
	}
	freshParser := gts.NewParser(grammars.PythonLanguage())
	freshParser.SetAdmissionCandidateRoute(false)
	fresh, err := freshParser.Parse(edited)
	if err != nil {
		t.Fatalf("suffix-delete fresh parse: %v", err)
	}
	defer fresh.Release()
	requireIncrementalDeepTreeMatchesFresh(t, incremental, fresh, grammars.PythonLanguage())
	if incremental.RootNode().HasError() != fresh.RootNode().HasError() {
		t.Fatalf("suffix-delete error state differs: incremental=%t fresh=%t", incremental.RootNode().HasError(), fresh.RootNode().HasError())
	}
}

// TestCheckpointedScannerPrefixFrontierMapsMultiEditHistory applies edits in
// successive source coordinate spaces. It proves the first-child suffix
// deletion cannot bypass the fallback after an earlier prefix deletion.
func TestCheckpointedScannerPrefixFrontierMapsMultiEditHistory(t *testing.T) {
	source := []byte("\n\ndef first():\n    return 111\n\ndef second():\n    return 2\n")
	parser := gts.NewParser(grammars.PythonLanguage())
	parser.SetAdmissionCandidateRoute(false)
	oldTree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("multi-edit old parse: %v", err)
	}
	defer oldTree.Release()

	withoutLeadingNewline := append([]byte(nil), source[1:]...)
	leadingEdit := gts.InputEdit{
		StartByte:   0,
		OldEndByte:  1,
		NewEndByte:  0,
		StartPoint:  pointForOffset(source, 0),
		OldEndPoint: pointForOffset(source, 1),
		NewEndPoint: pointForOffset(withoutLeadingNewline, 0),
	}
	oldTree.Edit(leadingEdit)
	first := oldTree.RootNode().Child(0)
	if first == nil || first.EndByte() == 0 {
		t.Fatal("multi-edit fixture has no first top-level child")
	}
	offset := int(first.EndByte()) - 1
	if withoutLeadingNewline[offset] < '0' || withoutLeadingNewline[offset] > '9' {
		t.Fatalf("multi-edit suffix byte=%q, want a digit", withoutLeadingNewline[offset])
	}
	edited := append(append([]byte(nil), withoutLeadingNewline[:offset]...), withoutLeadingNewline[offset+1:]...)
	suffixEdit := gts.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(offset + 1),
		NewEndByte:  uint32(offset),
		StartPoint:  pointForOffset(withoutLeadingNewline, offset),
		OldEndPoint: pointForOffset(withoutLeadingNewline, offset+1),
		NewEndPoint: pointForOffset(edited, offset),
	}
	oldTree.Edit(suffixEdit)
	incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("multi-edit incremental parse: %v", err)
	}
	defer incremental.Release()
	if profile.ReuseUnsupportedReason != "external_scanner_prefix_frontier_unproven" ||
		!profile.ReuseUnsupported || profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
		t.Fatalf("multi-edit suffix deletion bypassed frontier fallback: %+v", profile)
	}
	freshParser := gts.NewParser(grammars.PythonLanguage())
	freshParser.SetAdmissionCandidateRoute(false)
	fresh, err := freshParser.Parse(edited)
	if err != nil {
		t.Fatalf("multi-edit fresh parse: %v", err)
	}
	defer fresh.Release()
	requireIncrementalDeepTreeMatchesFresh(t, incremental, fresh, grammars.PythonLanguage())
	if incremental.RootNode().HasError() != fresh.RootNode().HasError() {
		t.Fatalf("multi-edit error state differs: incremental=%t fresh=%t", incremental.RootNode().HasError(), fresh.RootNode().HasError())
	}
}

func replacePrefixFrontierWitness(t *testing.T, source, oldText, replacement []byte) ([]byte, gts.InputEdit) {
	t.Helper()
	offset := bytes.Index(source, oldText)
	if offset < 0 {
		t.Fatalf("fixture missing edit text %q", oldText)
	}
	oldEnd := offset + len(oldText)
	edited := make([]byte, 0, len(source)-len(oldText)+len(replacement))
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
