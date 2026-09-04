//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"bytes"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// jsLog6RecoverySource is the pinned js_log_6 witness from the same manifest.
const jsLog6RecoverySource = "const f = (a) => a + 1;&\nclass A { m() { return 1 } }\n\n"

// TestCompactJavaScriptRecoveryIncrementalReuse proves that Tree.Copy
// keeps recovery provenance private while retaining the incremental reuse proof.
func TestCompactJavaScriptRecoveryIncrementalReuse(t *testing.T) {
	lang := grammars.JavascriptLanguage()
	if !lang.CompactStrategy2ErrorRegionCertified {
		t.Fatal("JavaScript strategy-2 recovery is not certified")
	}
	source := []byte(jsLog6RecoverySource)
	editOffset := bytes.IndexByte(source, '&')
	if editOffset < 0 {
		t.Fatal("js_log_6 witness has no recovery edit byte")
	}
	edited := append([]byte(nil), source[:editOffset]...)
	edited = append(edited, source[editOffset+1:]...)

	gts.ResetAdmissionCandidateCountersForTest()
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(true)
	oldTree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("compact JavaScript parse: %v", err)
	}
	defer oldTree.Release()
	routed, fallback := gts.AdmissionCandidateCounters()
	if routed != 1 || fallback != 0 {
		t.Fatalf("compact JavaScript route counters=%d/%d, want 1/0; reason=%q", routed, fallback, gts.AdmissionCandidateLastFallbackReason())
	}
	if !oldTree.RootNode().HasErrorOrMissing() {
		t.Fatalf("JavaScript witness has no recovery node: %s", oldTree.RootNode().SExpr(lang))
	}

	originalRoot := oldTree.RootNode()
	originalSExpr := originalRoot.SExpr(lang)
	originalRange := originalRoot.Range()
	originalEdits := len(oldTree.Edits())
	copyTree := oldTree.Copy()
	if copyTree == nil {
		t.Fatal("Tree.Copy returned nil")
	}
	defer copyTree.Release()
	if copyTree == oldTree || copyTree.RootNode() == originalRoot {
		t.Fatal("Tree.Copy did not isolate the root node")
	}
	if got := copyTree.RootNode().SExpr(lang); got != originalSExpr {
		t.Fatalf("Tree.Copy changed the recovery tree: got %q want %q", got, originalSExpr)
	}

	cleanSibling := compactRecoveryCleanDirectChildOfType(copyTree.RootNode(), lang, "class_declaration")
	if cleanSibling == nil {
		t.Fatalf("JavaScript witness has no clean class sibling: %s", copyTree.RootNode().SExpr(lang))
	}
	recoveryNode := compactRecoveryFirstErrorOrMissing(copyTree.RootNode())
	if recoveryNode == nil {
		t.Fatalf("JavaScript witness has no explicit recovery node: %s", copyTree.RootNode().SExpr(lang))
	}

	routedBefore, fallbackBefore := gts.AdmissionCandidateCounters()
	copyTree.Edit(gts.InputEdit{
		StartByte:   uint32(editOffset),
		OldEndByte:  uint32(editOffset + 1),
		NewEndByte:  uint32(editOffset),
		StartPoint:  pointAtOffset(source, editOffset),
		OldEndPoint: pointAtOffset(source, editOffset+1),
		NewEndPoint: pointAtOffset(edited, editOffset),
	})
	if recoveryNode.StartByte() != recoveryNode.EndByte() {
		t.Fatalf("deleted recovery node span=%d..%d, want zero width", recoveryNode.StartByte(), recoveryNode.EndByte())
	}
	if len(copyTree.Edits()) != 1 || len(oldTree.Edits()) != originalEdits {
		t.Fatalf("Tree.Copy edit ownership changed: copy edits=%d original edits=%d", len(copyTree.Edits()), len(oldTree.Edits()))
	}
	if got := oldTree.RootNode().SExpr(lang); got != originalSExpr || oldTree.RootNode().Range() != originalRange {
		t.Fatalf("editing Tree.Copy changed the original: sexpr=%q range=%+v", got, oldTree.RootNode().Range())
	}

	incremental, profile, err := parser.ParseIncrementalProfiled(edited, copyTree)
	if err != nil {
		t.Fatalf("JavaScript recovery copy incremental parse: %v", err)
	}
	defer incremental.Release()
	routedAfter, fallbackAfter := gts.AdmissionCandidateCounters()
	if routedAfter != routedBefore || fallbackAfter != fallbackBefore {
		t.Fatalf("incremental parse changed admission counters: before=%d/%d after=%d/%d", routedBefore, fallbackBefore, routedAfter, fallbackAfter)
	}
	if profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "" || !profile.OldTreeReuseRoute {
		t.Fatalf("copied JavaScript recovery tree dropped the reuse proof: %+v", profile)
	}
	if profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
		t.Fatalf("copied JavaScript recovery edit reused no clean sibling: %+v", profile)
	}
	if profile.ReuseRejectHasError == 0 && profile.ReuseRejectFragileNonLeaf == 0 {
		t.Fatalf("copied JavaScript recovery edit did not reject an ERROR or fragile candidate: %+v", profile)
	}
	if compactRecoveryTreeContainsNode(incremental.RootNode(), recoveryNode) {
		t.Fatal("incremental tree reused the copied recovery node")
	}
	if !compactRecoveryTreeContainsNode(incremental.RootNode(), cleanSibling) {
		t.Fatal("incremental tree did not reuse the copied clean class sibling")
	}
	if got := oldTree.RootNode().SExpr(lang); got != originalSExpr || oldTree.RootNode().Range() != originalRange || len(oldTree.Edits()) != originalEdits {
		t.Fatalf("incremental copy parse changed the original tree: sexpr=%q range=%+v edits=%d", got, oldTree.RootNode().Range(), len(oldTree.Edits()))
	}

	freshParser := gts.NewParser(lang)
	freshParser.SetAdmissionCandidateRoute(false)
	fresh, err := freshParser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh JavaScript production parse: %v", err)
	}
	defer fresh.Release()
	requireIncrementalDeepTreeMatchesFresh(t, incremental, fresh, lang)

	// The first incremental result has a production root and a reused compact
	// class. Restore the recovery byte to prove that the marked descendant keeps
	// its exact frontier requirement after the tree-level compact marker clears.
	incremental.Edit(gts.InputEdit{
		StartByte:   uint32(editOffset),
		OldEndByte:  uint32(editOffset),
		NewEndByte:  uint32(editOffset + 1),
		StartPoint:  pointAtOffset(edited, editOffset),
		OldEndPoint: pointAtOffset(edited, editOffset),
		NewEndPoint: pointAtOffset(source, editOffset+1),
	})
	restored, restoredProfile, err := parser.ParseIncrementalProfiled(source, incremental)
	if err != nil {
		t.Fatalf("restore JavaScript recovery byte: %v", err)
	}
	defer restored.Release()
	if restoredProfile.ReuseUnsupported || !restoredProfile.OldTreeReuseRoute {
		t.Fatalf("second incremental generation did not use the reuse route: %+v", restoredProfile)
	}
	freshOriginal, err := freshParser.Parse(source)
	if err != nil {
		t.Fatalf("fresh restored JavaScript production parse: %v", err)
	}
	defer freshOriginal.Release()
	requireIncrementalDeepTreeMatchesFresh(t, restored, freshOriginal, lang)
}

func compactRecoveryCleanDirectChildOfType(root *gts.Node, lang *gts.Language, typ string) *gts.Node {
	if root == nil || lang == nil {
		return nil
	}
	for index := 0; index < root.ChildCount(); index++ {
		child := root.Child(index)
		if child != nil && !child.IsError() && !child.IsMissing() && !child.HasErrorOrMissing() && child.Type(lang) == typ {
			return child
		}
	}
	return nil
}

func compactRecoveryFirstErrorOrMissing(root *gts.Node) *gts.Node {
	var found *gts.Node
	var walk func(*gts.Node) bool
	walk = func(node *gts.Node) bool {
		if node.IsError() || node.IsMissing() {
			found = node
			return false
		}
		for index := 0; index < node.ChildCount(); index++ {
			if !walk(node.Child(index)) {
				return false
			}
		}
		return true
	}
	if root != nil {
		walk(root)
	}
	return found
}

func compactRecoveryTreeContainsNode(root, target *gts.Node) bool {
	if root == nil || target == nil {
		return false
	}
	if root == target {
		return true
	}
	for index := 0; index < root.ChildCount(); index++ {
		if compactRecoveryTreeContainsNode(root.Child(index), target) {
			return true
		}
	}
	return false
}
