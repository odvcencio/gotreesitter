//go:build cgo && treesitter_c_parity && !gts_no_parsercorephase0

package cgoharness

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestStage5CompactPythonScannerCheckpointReuse proves the first Stage 5
// package with the smallest locked-C indentation edit. The edit changes four
// spaces before the second top-level function's final return into eight
// spaces, so the return moves inside the if statement. The compact tree must
// retain scanner checkpoints, reject stale boundaries, and preserve reuse on
// unaffected leaves in both directions.
func TestStage5CompactPythonScannerCheckpointReuse(t *testing.T) {
	source, err := os.ReadFile("testdata/canonical_incremental_python_scanner.py")
	if err != nil {
		t.Fatal(err)
	}
	source = append(source, []byte("\n\ndef classify_again(value):\n    if value:\n        return \"present\"\n    return \"missing\"\n")...)
	const marker = "\n    return \"missing\"\n"
	offset := bytes.LastIndex(source, []byte(marker)) + 1
	if offset <= 0 {
		t.Fatalf("locked Python scanner witness has no second return marker %q", marker)
	}
	const inserted = "    "
	if source[offset] != ' ' {
		t.Fatalf("locked Python scanner witness changed at byte %d: got %q", offset, source[offset])
	}
	offsetByte := uint32(offset)
	insertedBytes := uint32(len(inserted))
	edited := make([]byte, 0, len(source)+len(inserted))
	edited = append(edited, source[:offset]...)
	edited = append(edited, inserted...)
	edited = append(edited, source[offset:]...)
	forwardEdit := gotreesitter.InputEdit{
		StartByte:   offsetByte,
		OldEndByte:  offsetByte,
		NewEndByte:  offsetByte + insertedBytes,
		StartPoint:  pointAtOffset(source, offset),
		OldEndPoint: pointAtOffset(source, offset),
		NewEndPoint: pointAtOffset(edited, offset+len(inserted)),
	}
	backEdit := gotreesitter.InputEdit{
		StartByte:   offsetByte,
		OldEndByte:  offsetByte + insertedBytes,
		NewEndByte:  offsetByte,
		StartPoint:  pointAtOffset(edited, offset),
		OldEndPoint: pointAtOffset(edited, offset+len(inserted)),
		NewEndPoint: pointAtOffset(source, offset),
	}

	goLang := grammars.PythonLanguage()
	cLang, err := ParityCLanguage("python")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	cInitial := cParser.Parse(source, nil)
	if cInitial == nil || cInitial.RootNode() == nil {
		t.Fatal("locked C parser returned no initial tree")
	}
	t.Cleanup(cInitial.Close)
	cEditedFresh := cParser.Parse(edited, nil)
	if cEditedFresh == nil || cEditedFresh.RootNode() == nil {
		t.Fatal("locked C parser returned no edited tree")
	}
	t.Cleanup(cEditedFresh.Close)
	cOld := cParser.Parse(source, nil)
	if cOld == nil {
		t.Fatal("locked C parser returned no incremental base tree")
	}
	cOldEdit := realCorpusCInputEdit(forwardEdit)
	cOld.Edit(&cOldEdit)
	cEditedIncremental := cParser.Parse(edited, cOld)
	cOld.Close()
	if cEditedIncremental == nil || cEditedIncremental.RootNode() == nil {
		t.Fatal("locked C parser returned no forward incremental tree")
	}
	t.Cleanup(cEditedIncremental.Close)
	cReverseOld := cParser.Parse(edited, nil)
	if cReverseOld == nil || cReverseOld.RootNode() == nil {
		t.Fatal("locked C parser returned no reverse incremental base tree")
	}
	cReverseOldEdit := realCorpusCInputEdit(backEdit)
	cReverseOld.Edit(&cReverseOldEdit)
	cReverseIncremental := cParser.Parse(source, cReverseOld)
	cReverseOld.Close()
	if cReverseIncremental == nil || cReverseIncremental.RootNode() == nil {
		t.Fatal("locked C parser returned no reverse incremental tree")
	}
	t.Cleanup(cReverseIncremental.Close)

	routedBefore, fallbacksBefore := gotreesitter.AdmissionCandidateCounters()
	parser := gotreesitter.NewParser(goLang)
	parser.SetAdmissionCandidateRoute(true)
	base, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("compact base parse: %v", err)
	}
	t.Cleanup(base.Release)
	routedAfter, fallbacksAfter := gotreesitter.AdmissionCandidateCounters()
	if routedAfter-routedBefore != 1 || fallbacksAfter-fallbacksBefore != 0 {
		t.Fatalf("compact base route counter delta=%d/%d, want 1/0; reason=%q", routedAfter-routedBefore, fallbacksAfter-fallbacksBefore, gotreesitter.AdmissionCandidateLastFallbackReason())
	}
	assertStage5PythonGoCParity(t, base, goLang, cInitial.RootNode(), "compact initial")
	baseRuntime := base.ParseRuntime()
	if baseRuntime.ExternalScannerCheckpointRecords == 0 ||
		baseRuntime.ExternalScannerCheckpointLeafNodes == 0 ||
		baseRuntime.ExternalScannerSnapshotBytesAllocated == 0 ||
		!baseRuntime.CompactExternalScannerCheckpointTransferProven {
		t.Fatalf("compact base did not publish scanner checkpoint accounting: %+v", baseRuntime)
	}

	untouched := base.Copy()
	if untouched == nil {
		t.Fatal("compact base copy returned nil")
	}
	t.Cleanup(untouched.Release)
	untouchedRuntime := untouched.ParseRuntime()
	if untouchedRuntime.ExternalScannerCheckpointRecords != baseRuntime.ExternalScannerCheckpointRecords ||
		untouchedRuntime.ExternalScannerCheckpointSlotsAllocated != baseRuntime.ExternalScannerCheckpointSlotsAllocated ||
		untouchedRuntime.ExternalScannerCheckpointBytesAllocated != baseRuntime.ExternalScannerCheckpointBytesAllocated ||
		untouchedRuntime.ExternalScannerCheckpointLeafNodes != baseRuntime.ExternalScannerCheckpointLeafNodes ||
		untouchedRuntime.ExternalScannerSnapshotBytesAllocated != baseRuntime.ExternalScannerSnapshotBytesAllocated {
		t.Fatalf("compact copy changed checkpoint accounting: base=%+v copy=%+v", baseRuntime, untouchedRuntime)
	}

	forwardBase := base.Copy()
	if forwardBase == nil {
		t.Fatal("compact forward copy returned nil")
	}
	t.Cleanup(forwardBase.Release)
	forwardBase.Edit(forwardEdit)
	forward, profile, err := parser.ParseIncrementalProfiled(edited, forwardBase)
	if err != nil {
		t.Fatalf("compact forward incremental parse: %v", err)
	}
	if forward != forwardBase {
		t.Cleanup(forward.Release)
	}
	if profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "" || !profile.OldTreeReuseRoute {
		t.Fatalf("compact forward reuse was not admitted: %+v", profile)
	}
	if profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
		t.Fatalf("compact forward lost all reuse: %+v", profile)
	}
	if profile.ReuseRejectScannerUnquiescent == 0 {
		t.Fatalf("compact forward did not report invalidated scanner boundaries: %+v", profile)
	}
	assertStage5PythonGoCParity(t, forward, goLang, cEditedFresh.RootNode(), "compact forward fresh C")
	assertStage5PythonGoCParity(t, forward, goLang, cEditedIncremental.RootNode(), "compact forward incremental C")
	forwardRuntime := forward.ParseRuntime()
	if forwardRuntime.StopReason != gotreesitter.ParseStopAccepted ||
		forwardRuntime.Truncated || forwardRuntime.TokenSourceEOFEarly ||
		forwardRuntime.ExpectedEOFByte != uint32(len(edited)) ||
		forwardRuntime.RootEndByte != uint32(len(edited)) ||
		!forwardRuntime.IncrementalOldTreeReuseRoute {
		t.Fatalf("compact forward runtime is incomplete: %+v", forwardRuntime)
	}
	if forwardRuntime.ExternalScannerCheckpointRecords == 0 ||
		forwardRuntime.ExternalScannerCheckpointSlotsAllocated == 0 ||
		forwardRuntime.ExternalScannerCheckpointBytesAllocated == 0 ||
		forwardRuntime.ExternalScannerCheckpointLeafNodes == 0 ||
		forwardRuntime.ExternalScannerSnapshotBytesAllocated == 0 {
		t.Fatalf("compact forward lost scanner checkpoint accounting: %+v", forwardRuntime)
	}

	// Tree.Copy must isolate the source branch. The untouched copy remains the
	// original tree while the forward branch edits and reparses independently.
	assertStage5PythonGoCParity(t, untouched, goLang, cInitial.RootNode(), "untouched copy")
	if got := untouched.ParseRuntime().ExternalScannerCheckpointRecords; got != untouchedRuntime.ExternalScannerCheckpointRecords {
		t.Fatalf("untouched copy checkpoint records changed: before=%d after=%d", untouchedRuntime.ExternalScannerCheckpointRecords, got)
	}
	if got := untouched.ParseRuntime().ExternalScannerCheckpointSlotsAllocated; got != untouchedRuntime.ExternalScannerCheckpointSlotsAllocated {
		t.Fatalf("untouched copy checkpoint slots changed: before=%d after=%d", untouchedRuntime.ExternalScannerCheckpointSlotsAllocated, got)
	}
	if got := untouched.ParseRuntime().ExternalScannerCheckpointBytesAllocated; got != untouchedRuntime.ExternalScannerCheckpointBytesAllocated {
		t.Fatalf("untouched copy checkpoint bytes changed: before=%d after=%d", untouchedRuntime.ExternalScannerCheckpointBytesAllocated, got)
	}
	if got := untouched.ParseRuntime().ExternalScannerSnapshotBytesAllocated; got != untouchedRuntime.ExternalScannerSnapshotBytesAllocated {
		t.Fatalf("untouched copy snapshot bytes changed: before=%d after=%d", untouchedRuntime.ExternalScannerSnapshotBytesAllocated, got)
	}
	if got := base.ParseRuntime().ExternalScannerCheckpointRecords; got != baseRuntime.ExternalScannerCheckpointRecords {
		t.Fatalf("compact base checkpoint records changed through copy: before=%d after=%d", baseRuntime.ExternalScannerCheckpointRecords, got)
	}

	if forward == nil {
		t.Fatal("compact forward tree is nil")
	}
	forward.Edit(backEdit)
	reverse, reverseProfile, err := parser.ParseIncrementalProfiled(source, forward)
	if err != nil {
		t.Fatalf("compact reverse incremental parse: %v", err)
	}
	if reverse != forward {
		t.Cleanup(reverse.Release)
	}
	if reverseProfile.ReuseUnsupported || reverseProfile.ReuseUnsupportedReason != "" || !reverseProfile.OldTreeReuseRoute ||
		reverseProfile.ReusedSubtrees == 0 || reverseProfile.ReusedBytes == 0 {
		t.Fatalf("compact reverse did not preserve later-child checkpoint reuse: %+v", reverseProfile)
	}
	assertStage5PythonGoCParity(t, reverse, goLang, cInitial.RootNode(), "compact reverse fresh C")
	assertStage5PythonGoCParity(t, reverse, goLang, cReverseIncremental.RootNode(), "compact reverse incremental C")
	reverseRuntime := reverse.ParseRuntime()
	if reverseRuntime.StopReason != gotreesitter.ParseStopAccepted ||
		reverseRuntime.Truncated || reverseRuntime.TokenSourceEOFEarly ||
		reverseRuntime.ExpectedEOFByte != uint32(len(source)) ||
		reverseRuntime.RootEndByte != uint32(len(source)) ||
		!reverseRuntime.IncrementalOldTreeReuseRoute {
		t.Fatalf("compact reverse runtime is incomplete: %+v", reverseRuntime)
	}
	if reverseRuntime.ExternalScannerCheckpointRecords == 0 ||
		reverseRuntime.ExternalScannerCheckpointSlotsAllocated == 0 ||
		reverseRuntime.ExternalScannerCheckpointBytesAllocated == 0 ||
		reverseRuntime.ExternalScannerCheckpointLeafNodes == 0 ||
		reverseRuntime.ExternalScannerSnapshotBytesAllocated == 0 {
		t.Fatalf("compact reverse lost scanner checkpoint accounting: %+v", reverseRuntime)
	}
	t.Logf("stage5 python checkpoint witness: forward reuse=%d/%d reject_scanner=%d; reverse reuse=%d/%d reject_scanner=%d; checkpoints=%d/%d leaves=%d/%d", profile.ReusedSubtrees, profile.ReusedBytes, profile.ReuseRejectScannerUnquiescent, reverseProfile.ReusedSubtrees, reverseProfile.ReusedBytes, reverseProfile.ReuseRejectScannerUnquiescent, forwardRuntime.ExternalScannerCheckpointRecords, reverseRuntime.ExternalScannerCheckpointRecords, forwardRuntime.ExternalScannerCheckpointLeafNodes, reverseRuntime.ExternalScannerCheckpointLeafNodes)
}

// TestStage5PythonSameLengthScannerDelimiterParity proves the conservative
// same-length scanner-state lane against both C fresh and C incremental trees.
// It changes a regular string with literal braces into a real f-string
// interpolation without changing the byte span, then checks the following
// top-level child.
func TestStage5PythonSameLengthScannerDelimiterParity(t *testing.T) {
	source := []byte("def first(value):\n    return u\"{x}\"\n\ndef second(value):\n    return \"unchanged\"\n")
	oldText := []byte("u\"{x}\"")
	replacement := []byte("f'{x}'")
	offset := bytes.Index(source, oldText)
	if offset < 0 || len(oldText) != len(replacement) {
		t.Fatalf("same-length delimiter witness is malformed: offset=%d old=%d new=%d", offset, len(oldText), len(replacement))
	}
	edited := make([]byte, 0, len(source))
	edited = append(edited, source[:offset]...)
	edited = append(edited, replacement...)
	edited = append(edited, source[offset+len(oldText):]...)
	edit := gotreesitter.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(offset + len(oldText)),
		NewEndByte:  uint32(offset + len(replacement)),
		StartPoint:  pointAtOffset(source, offset),
		OldEndPoint: pointAtOffset(source, offset+len(oldText)),
		NewEndPoint: pointAtOffset(edited, offset+len(replacement)),
	}

	goLang := grammars.PythonLanguage()
	goParser := gotreesitter.NewParser(goLang)
	goParser.SetAdmissionCandidateRoute(false)
	goOld, err := goParser.Parse(source)
	if err != nil {
		t.Fatalf("Go same-length old parse: %v", err)
	}
	t.Cleanup(goOld.Release)
	goOld.Edit(edit)
	goIncremental, profile, err := goParser.ParseIncrementalProfiled(edited, goOld)
	if err != nil {
		t.Fatalf("Go same-length incremental parse: %v", err)
	}
	if goIncremental != goOld {
		t.Cleanup(goIncremental.Release)
	}
	goFreshParser := gotreesitter.NewParser(goLang)
	goFreshParser.SetAdmissionCandidateRoute(false)
	goFresh, err := goFreshParser.Parse(edited)
	if err != nil {
		t.Fatalf("Go same-length fresh parse: %v", err)
	}
	t.Cleanup(goFresh.Release)
	var goFreshDiff []string
	compareGoNodes(goIncremental.RootNode(), goLang, goFresh.RootNode(), "root", &goFreshDiff)
	if len(goFreshDiff) > 0 {
		t.Fatalf("Go same-length incremental differs from fresh: %s", goFreshDiff[0])
	}
	if goIncremental.RootNode().HasError() != goFresh.RootNode().HasError() {
		t.Fatalf("Go same-length error state differs from fresh: incremental=%t fresh=%t", goIncremental.RootNode().HasError(), goFresh.RootNode().HasError())
	}
	if profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "" || !profile.OldTreeReuseRoute ||
		profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
		t.Fatalf("Go same-length scanner-state edit did not use authenticated reuse: %+v", profile)
	}

	cLang, err := ParityCLanguage("python")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	cFresh := cParser.Parse(edited, nil)
	if cFresh == nil || cFresh.RootNode() == nil {
		t.Fatal("C same-length fresh parse returned no root")
	}
	t.Cleanup(cFresh.Close)
	cOld := cParser.Parse(source, nil)
	if cOld == nil || cOld.RootNode() == nil {
		t.Fatal("C same-length old parse returned no root")
	}
	cEdit := realCorpusCInputEdit(edit)
	cOld.Edit(&cEdit)
	cIncremental := cParser.Parse(edited, cOld)
	cOld.Close()
	if cIncremental == nil || cIncremental.RootNode() == nil {
		t.Fatal("C same-length incremental parse returned no root")
	}
	t.Cleanup(cIncremental.Close)
	if diff := FirstDivergenceDumpV1(goIncremental.RootNode(), goLang, cFresh.RootNode()); diff != nil {
		t.Fatalf("Go same-length incremental differs from C fresh: %+v", *diff)
	}
	if diff := FirstDivergenceDumpV1(goIncremental.RootNode(), goLang, cIncremental.RootNode()); diff != nil {
		t.Fatalf("Go same-length incremental differs from C incremental: %+v", *diff)
	}
	if goIncremental.RootNode().HasError() != cFresh.RootNode().HasError() || goIncremental.RootNode().HasError() != cIncremental.RootNode().HasError() {
		t.Fatalf("same-length error state differs across Go/C: go=%t c_fresh=%t c_incremental=%t", goIncremental.RootNode().HasError(), cFresh.RootNode().HasError(), cIncremental.RootNode().HasError())
	}
	t.Logf("same-length scanner delimiter parity: reuse=%t unsupported=%t reason=%q reused=%d bytes=%d", profile.OldTreeReuseRoute, profile.ReuseUnsupported, profile.ReuseUnsupportedReason, profile.ReusedSubtrees, profile.ReusedBytes)
}

// TestStage5PythonWideIndentBoundaryParity proves the uint16 indentation
// counter used by the locked Python commit 26855eab. Each bounded case
// compares fresh, forward-incremental, and reverse-incremental Go trees with
// the locked C trees, including ranges and error state.
func TestStage5PythonWideIndentBoundaryParity(t *testing.T) {
	for _, width := range []int{65535, 65536, 131072} {
		width := width
		t.Run(fmt.Sprintf("spaces_%d", width), func(t *testing.T) {
			source := stage5PythonWideIndentSource(width)
			const oldReturn = "return 2"
			offset := bytes.LastIndex(source, []byte(oldReturn))
			if offset < 0 {
				t.Fatalf("wide-indent fixture has no second return marker")
			}
			edited := append([]byte(nil), source...)
			edited[offset+len(oldReturn)-1] = '3'
			edit := gotreesitter.InputEdit{
				StartByte:   uint32(offset + len(oldReturn) - 1),
				OldEndByte:  uint32(offset + len(oldReturn)),
				NewEndByte:  uint32(offset + len(oldReturn)),
				StartPoint:  pointAtOffset(source, offset+len(oldReturn)-1),
				OldEndPoint: pointAtOffset(source, offset+len(oldReturn)),
				NewEndPoint: pointAtOffset(edited, offset+len(oldReturn)),
			}
			reverseEdit := gotreesitter.InputEdit{
				StartByte:   edit.StartByte,
				OldEndByte:  edit.OldEndByte,
				NewEndByte:  edit.NewEndByte,
				StartPoint:  edit.StartPoint,
				OldEndPoint: edit.NewEndPoint,
				NewEndPoint: edit.OldEndPoint,
			}

			goLang := grammars.PythonLanguage()
			goParser := gotreesitter.NewParser(goLang)
			goParser.SetAdmissionCandidateRoute(false)
			goBase, err := goParser.Parse(source)
			if err != nil {
				t.Fatalf("Go wide-indent fresh parse: %v", err)
			}
			t.Cleanup(goBase.Release)
			goBaseRuntime := goBase.ParseRuntime()
			if goBaseRuntime.Truncated || goBaseRuntime.RootEndByte != uint32(len(source)) {
				t.Fatalf("Go wide-indent fresh parse incomplete: %+v", goBaseRuntime)
			}

			cLang, err := ParityCLanguage("python")
			if err != nil {
				t.Fatal(err)
			}
			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			cBase := cParser.Parse(source, nil)
			if cBase == nil || cBase.RootNode() == nil {
				t.Fatal("locked C wide-indent fresh parse returned no root")
			}
			t.Cleanup(cBase.Close)
			assertStage5PythonGoCParity(t, goBase, goLang, cBase.RootNode(), "wide-indent fresh")
			assertStage5PythonRootRange(t, goBase, cBase.RootNode(), "wide-indent fresh")

			cForwardFresh := cParser.Parse(edited, nil)
			if cForwardFresh == nil || cForwardFresh.RootNode() == nil {
				t.Fatal("locked C wide-indent forward fresh parse returned no root")
			}
			t.Cleanup(cForwardFresh.Close)
			cForwardOld := cParser.Parse(source, nil)
			if cForwardOld == nil {
				t.Fatal("locked C wide-indent forward base parse returned no tree")
			}
			cForwardOldEdit := realCorpusCInputEdit(edit)
			cForwardOld.Edit(&cForwardOldEdit)
			cForward := cParser.Parse(edited, cForwardOld)
			cForwardOld.Close()
			if cForward == nil || cForward.RootNode() == nil {
				t.Fatal("locked C wide-indent forward incremental parse returned no root")
			}
			t.Cleanup(cForward.Close)

			goForwardBase := goBase.Copy()
			if goForwardBase == nil {
				t.Fatal("Go wide-indent forward base copy returned nil")
			}
			t.Cleanup(goForwardBase.Release)
			goForwardBase.Edit(edit)
			goForward, forwardProfile, err := goParser.ParseIncrementalProfiled(edited, goForwardBase)
			if err != nil {
				t.Fatalf("Go wide-indent forward incremental parse: %v", err)
			}
			if goForward != goForwardBase {
				t.Cleanup(goForward.Release)
			}
			assertStage5PythonGoCParity(t, goForward, goLang, cForwardFresh.RootNode(), "wide-indent forward fresh C")
			assertStage5PythonGoCParity(t, goForward, goLang, cForward.RootNode(), "wide-indent forward incremental C")
			assertStage5PythonRootRange(t, goForward, cForward.RootNode(), "wide-indent forward")

			cReverseOld := cParser.Parse(edited, nil)
			if cReverseOld == nil {
				t.Fatal("locked C wide-indent reverse base parse returned no tree")
			}
			cReverseOldEdit := realCorpusCInputEdit(reverseEdit)
			cReverseOld.Edit(&cReverseOldEdit)
			cReverse := cParser.Parse(source, cReverseOld)
			cReverseOld.Close()
			if cReverse == nil || cReverse.RootNode() == nil {
				t.Fatal("locked C wide-indent reverse incremental parse returned no root")
			}
			t.Cleanup(cReverse.Close)

			goForward.Edit(reverseEdit)
			goReverse, reverseProfile, err := goParser.ParseIncrementalProfiled(source, goForward)
			if err != nil {
				t.Fatalf("Go wide-indent reverse incremental parse: %v", err)
			}
			if goReverse != goForward {
				t.Cleanup(goReverse.Release)
			}
			assertStage5PythonGoCParity(t, goReverse, goLang, cBase.RootNode(), "wide-indent reverse fresh C")
			assertStage5PythonGoCParity(t, goReverse, goLang, cReverse.RootNode(), "wide-indent reverse incremental C")
			assertStage5PythonRootRange(t, goReverse, cReverse.RootNode(), "wide-indent reverse")
			t.Logf("wide-indent spaces=%d source=%d base_stop=%s base_end=%d forward_profile=%+v reverse_profile=%+v", width, len(source), goBaseRuntime.StopReason, goBaseRuntime.RootEndByte, forwardProfile, reverseProfile)
		})
	}
}

// TestStage5PythonWideIndentTransitionParity proves edits across the locked
// uint16 indentation boundary. The cases use bounded source sizes and compare
// fresh and incremental trees in both directions.
func TestStage5PythonWideIndentTransitionParity(t *testing.T) {
	for _, tc := range []struct {
		name               string
		oldWidth, newWidth int
	}{
		{name: "65535_to_65536", oldWidth: 65535, newWidth: 65536},
		{name: "65536_to_131072", oldWidth: 65536, newWidth: 131072},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			source, edited, edit, reverseEdit := stage5PythonWideIndentTransition(tc.oldWidth, tc.newWidth)

			goLang := grammars.PythonLanguage()
			goParser := gotreesitter.NewParser(goLang)
			goParser.SetAdmissionCandidateRoute(false)
			goOld, err := goParser.Parse(source)
			if err != nil {
				t.Fatalf("Go wide-indent transition old fresh parse: %v", err)
			}
			t.Cleanup(goOld.Release)
			goOldRuntime := goOld.ParseRuntime()
			if goOldRuntime.Truncated || goOldRuntime.RootEndByte != uint32(len(source)) {
				t.Fatalf("Go wide-indent transition old fresh parse incomplete: %+v", goOldRuntime)
			}

			goFreshParser := gotreesitter.NewParser(goLang)
			goFreshParser.SetAdmissionCandidateRoute(false)
			goNewFresh, err := goFreshParser.Parse(edited)
			if err != nil {
				t.Fatalf("Go wide-indent transition new fresh parse: %v", err)
			}
			t.Cleanup(goNewFresh.Release)
			goNewRuntime := goNewFresh.ParseRuntime()
			if goNewRuntime.Truncated || goNewRuntime.RootEndByte != uint32(len(edited)) {
				t.Fatalf("Go wide-indent transition new fresh parse incomplete: %+v", goNewRuntime)
			}

			cLang, err := ParityCLanguage("python")
			if err != nil {
				t.Fatal(err)
			}
			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			cOldFresh := cParser.Parse(source, nil)
			if cOldFresh == nil || cOldFresh.RootNode() == nil {
				t.Fatal("locked C wide-indent transition old fresh parse returned no root")
			}
			t.Cleanup(cOldFresh.Close)
			cNewFresh := cParser.Parse(edited, nil)
			if cNewFresh == nil || cNewFresh.RootNode() == nil {
				t.Fatal("locked C wide-indent transition new fresh parse returned no root")
			}
			t.Cleanup(cNewFresh.Close)

			assertStage5PythonGoCParity(t, goOld, goLang, cOldFresh.RootNode(), "wide-indent transition old fresh")
			assertStage5PythonRootRange(t, goOld, cOldFresh.RootNode(), "wide-indent transition old fresh")
			assertStage5PythonGoCParity(t, goNewFresh, goLang, cNewFresh.RootNode(), "wide-indent transition new fresh")
			assertStage5PythonRootRange(t, goNewFresh, cNewFresh.RootNode(), "wide-indent transition new fresh")

			cForwardOld := cParser.Parse(source, nil)
			if cForwardOld == nil {
				t.Fatal("locked C wide-indent transition forward base parse returned no tree")
			}
			cForwardEdit := realCorpusCInputEdit(edit)
			cForwardOld.Edit(&cForwardEdit)
			cForward := cParser.Parse(edited, cForwardOld)
			cForwardOld.Close()
			if cForward == nil || cForward.RootNode() == nil {
				t.Fatal("locked C wide-indent transition forward incremental parse returned no root")
			}
			t.Cleanup(cForward.Close)

			goForwardBase := goOld.Copy()
			if goForwardBase == nil {
				t.Fatal("Go wide-indent transition forward base copy returned nil")
			}
			t.Cleanup(goForwardBase.Release)
			goForwardBase.Edit(edit)
			goForward, forwardProfile, err := goParser.ParseIncrementalProfiled(edited, goForwardBase)
			if err != nil {
				t.Fatalf("Go wide-indent transition forward incremental parse: %v", err)
			}
			if goForward != goForwardBase {
				t.Cleanup(goForward.Release)
			}
			assertStage5PythonPrefixFallback(t, forwardProfile, "wide-indent transition forward")
			assertStage5PythonGoCParity(t, goForward, goLang, cNewFresh.RootNode(), "wide-indent transition forward fresh C")
			assertStage5PythonGoCParity(t, goForward, goLang, cForward.RootNode(), "wide-indent transition forward incremental C")
			assertStage5PythonRootRange(t, goForward, cForward.RootNode(), "wide-indent transition forward")

			cReverseOld := cParser.Parse(edited, nil)
			if cReverseOld == nil {
				t.Fatal("locked C wide-indent transition reverse base parse returned no tree")
			}
			cReverseEdit := realCorpusCInputEdit(reverseEdit)
			cReverseOld.Edit(&cReverseEdit)
			cReverse := cParser.Parse(source, cReverseOld)
			cReverseOld.Close()
			if cReverse == nil || cReverse.RootNode() == nil {
				t.Fatal("locked C wide-indent transition reverse incremental parse returned no root")
			}
			t.Cleanup(cReverse.Close)

			goForward.Edit(reverseEdit)
			goReverse, reverseProfile, err := goParser.ParseIncrementalProfiled(source, goForward)
			if err != nil {
				t.Fatalf("Go wide-indent transition reverse incremental parse: %v", err)
			}
			if goReverse != goForward {
				t.Cleanup(goReverse.Release)
			}
			assertStage5PythonPrefixFallback(t, reverseProfile, "wide-indent transition reverse")
			assertStage5PythonGoCParity(t, goReverse, goLang, cOldFresh.RootNode(), "wide-indent transition reverse fresh C")
			assertStage5PythonGoCParity(t, goReverse, goLang, cReverse.RootNode(), "wide-indent transition reverse incremental C")
			assertStage5PythonRootRange(t, goReverse, cReverse.RootNode(), "wide-indent transition reverse")
			t.Logf("wide-indent transition %d->%d old=%d new=%d forward_nodes=%d reverse_nodes=%d", tc.oldWidth, tc.newWidth, len(source), len(edited), forwardProfile.NewNodesAllocated, reverseProfile.NewNodesAllocated)
		})
	}
}

func stage5PythonWideIndentSource(width int) []byte {
	source := make([]byte, 0, width+96)
	source = append(source, "def first():\n"...)
	source = append(source, bytes.Repeat([]byte{' '}, width)...)
	source = append(source, "return 1\n\n"...)
	source = append(source, "def second():\n    return 2\n"...)
	return source
}

func stage5PythonWideIndentTransition(oldWidth, newWidth int) ([]byte, []byte, gotreesitter.InputEdit, gotreesitter.InputEdit) {
	source := stage5PythonWideIndentSource(oldWidth)
	edited := stage5PythonWideIndentSource(newWidth)
	const indentStart = len("def first():\n")
	forward := gotreesitter.InputEdit{
		StartByte:   uint32(indentStart),
		OldEndByte:  uint32(indentStart + oldWidth),
		NewEndByte:  uint32(indentStart + newWidth),
		StartPoint:  pointAtOffset(source, indentStart),
		OldEndPoint: pointAtOffset(source, indentStart+oldWidth),
		NewEndPoint: pointAtOffset(edited, indentStart+newWidth),
	}
	reverse := gotreesitter.InputEdit{
		StartByte:   forward.StartByte,
		OldEndByte:  forward.NewEndByte,
		NewEndByte:  forward.OldEndByte,
		StartPoint:  forward.StartPoint,
		OldEndPoint: forward.NewEndPoint,
		NewEndPoint: forward.OldEndPoint,
	}
	return source, edited, forward, reverse
}

func assertStage5PythonRootRange(t *testing.T, tree *gotreesitter.Tree, cRoot *sitter.Node, label string) {
	t.Helper()
	if tree == nil || tree.RootNode() == nil || cRoot == nil {
		t.Fatalf("%s has no root", label)
	}
	goRange := tree.RootNode().Range()
	cStart := cRoot.StartPosition()
	cEnd := cRoot.EndPosition()
	if goRange.StartByte != uint32(cRoot.StartByte()) || goRange.EndByte != uint32(cRoot.EndByte()) ||
		goRange.StartPoint.Row != uint32(cStart.Row) || goRange.StartPoint.Column != uint32(cStart.Column) ||
		goRange.EndPoint.Row != uint32(cEnd.Row) || goRange.EndPoint.Column != uint32(cEnd.Column) {
		t.Fatalf("%s root range differs: Go=%+v C=[%d,%d) (%d,%d)..(%d,%d)", label, goRange, cRoot.StartByte(), cRoot.EndByte(), cStart.Row, cStart.Column, cEnd.Row, cEnd.Column)
	}
}

func assertStage5PythonGoCParity(t *testing.T, tree *gotreesitter.Tree, lang *gotreesitter.Language, cRoot *sitter.Node, label string) {
	t.Helper()
	if tree == nil || tree.RootNode() == nil {
		t.Fatalf("%s returned no Go root", label)
	}
	if cRoot == nil {
		t.Fatalf("%s received no C root", label)
	}
	if diff := FirstDivergenceDumpV1(tree.RootNode(), lang, cRoot); diff != nil {
		t.Fatalf("%s differs from locked C: %+v", label, *diff)
	}
	if got, want := stage5PythonGoErrorCount(tree.RootNode()), stage5PythonCErrorCount(cRoot); got != want {
		t.Fatalf("%s error-node count=%d, want %d", label, got, want)
	}
	if got, want := tree.RootNode().HasError(), cRoot.HasError(); got != want {
		t.Fatalf("%s root HasError=%t, want %t", label, got, want)
	}
}

func assertStage5PythonPrefixFallback(t *testing.T, profile gotreesitter.IncrementalParseProfile, label string) {
	t.Helper()
	if profile.ReuseUnsupportedReason != "external_scanner_prefix_frontier_unproven" ||
		!profile.ReuseUnsupported || profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 ||
		profile.ReusedBytes != 0 || profile.ReuseCursorNanos != 0 {
		t.Fatalf("%s did not fail closed before candidate scanning: %+v", label, profile)
	}
}

func stage5PythonGoErrorCount(node *gotreesitter.Node) uint64 {
	if node == nil {
		return 0
	}
	count := uint64(0)
	if node.IsError() {
		count++
	}
	for index := 0; index < node.ChildCount(); index++ {
		count += stage5PythonGoErrorCount(node.Child(index))
	}
	return count
}

func stage5PythonCErrorCount(node *sitter.Node) uint64 {
	if node == nil {
		return 0
	}
	count := uint64(0)
	if node.IsError() {
		count++
	}
	for index := uint(0); index < node.ChildCount(); index++ {
		count += stage5PythonCErrorCount(node.Child(index))
	}
	return count
}
