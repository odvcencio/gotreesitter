//go:build cgo && treesitter_c_parity && gts_parsercorephase0

package cgoharness

import (
	"bytes"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const compactRecoveryJavaScriptSource = "const f = (a) => a + 1;&\nclass A { m() { return 1 } }\n\n"

// TestCompactRecoveryJavaScriptIncrementalCOracle compares the deletion edit
// with fresh and incremental C trees. The class declaration is a clean
// sibling; the ERROR node must be rebuilt when the ampersand changes.
func TestCompactRecoveryJavaScriptIncrementalCOracle(t *testing.T) {
	goLanguage := grammars.JavascriptLanguage()
	if !goLanguage.CompactStrategy2ErrorRegionCertified {
		t.Fatal("JavaScript strategy-2 recovery is not certified")
	}
	cLanguage, err := ParityCLanguage("javascript")
	if err != nil {
		t.Fatalf("load JavaScript C oracle: %v", err)
	}

	original := []byte(compactRecoveryJavaScriptSource)
	ampersand := bytes.IndexByte(original, '&')
	if ampersand < 0 {
		t.Fatal("JavaScript recovery witness has no ampersand")
	}
	withoutAmpersand := append([]byte(nil), original[:ampersand]...)
	withoutAmpersand = append(withoutAmpersand, original[ampersand+1:]...)

	edit := gotreesitter.InputEdit{
		StartByte:   uint32(ampersand),
		OldEndByte:  uint32(ampersand + 1),
		NewEndByte:  uint32(ampersand),
		StartPoint:  pointAtOffset(original, ampersand),
		OldEndPoint: pointAtOffset(original, ampersand+1),
		NewEndPoint: pointAtOffset(withoutAmpersand, ampersand),
	}
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatalf("set JavaScript C oracle: %v", err)
	}
	cInitial := cParser.Parse(original, nil)
	if cInitial == nil || cInitial.RootNode() == nil {
		t.Fatal("C initial parse returned no root")
	}
	t.Cleanup(cInitial.Close)

	goParser := gotreesitter.NewParser(goLanguage)
	goParser.SetAdmissionCandidateRoute(true)
	initialRoutedBefore, initialFallbackBefore := gotreesitter.AdmissionCandidateCounters()
	goInitial, err := goParser.Parse(original)
	if err != nil {
		t.Fatalf("compact Go initial parse: %v", err)
	}
	t.Cleanup(goInitial.Release)
	initialRoutedAfter, initialFallbackAfter := gotreesitter.AdmissionCandidateCounters()
	if initialRoutedAfter-initialRoutedBefore != 1 || initialFallbackAfter-initialFallbackBefore != 0 {
		t.Fatalf("initial route counters=%d/%d, want 1/0; reason=%q", initialRoutedAfter-initialRoutedBefore, initialFallbackAfter-initialFallbackBefore, gotreesitter.AdmissionCandidateLastFallbackReason())
	}

	compactRecoveryAssertCParity(t, "compact initial", goInitial.RootNode(), goLanguage, cInitial)
	cleanSibling := compactRecoveryFindCleanDirectChild(goInitial.RootNode(), goLanguage, "class_declaration")
	if cleanSibling == nil {
		t.Fatalf("initial Go tree has no clean class sibling: %s", goInitial.RootNode().SExpr(goLanguage))
	}
	recoveryNode := compactRecoveryFindProblemNode(goInitial.RootNode())
	if recoveryNode == nil {
		t.Fatalf("initial Go tree has no recovery node: %s", goInitial.RootNode().SExpr(goLanguage))
	}

	cFresh := cParser.Parse(withoutAmpersand, nil)
	if cFresh == nil || cFresh.RootNode() == nil {
		t.Fatal("C fresh edited parse returned no root")
	}
	t.Cleanup(cFresh.Close)

	cOld := cParser.Parse(original, nil)
	if cOld == nil || cOld.RootNode() == nil {
		t.Fatal("C old parse returned no root")
	}
	cEdit := realCorpusCInputEdit(edit)
	cOld.Edit(&cEdit)
	cRecovery := compactRecoveryFindCProblemNode(cOld.RootNode())
	if cRecovery == nil || !cRecovery.HasChanges() {
		t.Fatal("C edit did not invalidate the recovery node")
	}
	cIncremental := cParser.Parse(withoutAmpersand, cOld)
	cOld.Close()
	if cIncremental == nil || cIncremental.RootNode() == nil {
		t.Fatal("C incremental parse returned no root")
	}
	t.Cleanup(cIncremental.Close)

	cFreshDigest, err := COracleDeepDigest(cFresh)
	if err != nil {
		t.Fatalf("digest C fresh edited tree: %v", err)
	}
	cIncrementalDigest, err := COracleDeepDigest(cIncremental)
	if err != nil {
		t.Fatalf("digest C incremental edited tree: %v", err)
	}
	if cIncrementalDigest != cFreshDigest {
		t.Fatalf("C incremental digest=%s, want fresh C digest=%s", cIncrementalDigest, cFreshDigest)
	}

	initialRootSExpr := goInitial.RootNode().SExpr(goLanguage)
	initialRootRange := goInitial.RootNode().Range()
	initialEditCount := len(goInitial.Edits())
	incrementalBase := goInitial.Copy()
	if incrementalBase == nil || incrementalBase == goInitial || incrementalBase.RootNode() == goInitial.RootNode() {
		t.Fatal("Tree.Copy did not isolate the JavaScript recovery tree")
	}
	if got := incrementalBase.RootNode().SExpr(goLanguage); got != initialRootSExpr {
		t.Fatalf("Tree.Copy changed the initial tree: got=%q want=%q", got, initialRootSExpr)
	}
	cleanSibling = compactRecoveryFindCleanDirectChild(incrementalBase.RootNode(), goLanguage, "class_declaration")
	recoveryNode = compactRecoveryFindProblemNode(incrementalBase.RootNode())
	if cleanSibling == nil || recoveryNode == nil {
		t.Fatal("Tree.Copy lost the clean sibling or recovery node")
	}
	t.Cleanup(incrementalBase.Release)
	incrementalBase.Edit(edit)
	if !recoveryNode.HasChanges() {
		t.Fatal("Go edit did not invalidate the recovery node")
	}

	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	goIncremental, profile, err := goParser.ParseIncrementalProfiled(withoutAmpersand, incrementalBase)
	if err != nil {
		t.Fatalf("Go incremental parse: %v", err)
	}
	if goIncremental != incrementalBase {
		t.Cleanup(goIncremental.Release)
	}
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	if routedAfter != routedBefore || fallbackAfter != fallbackBefore {
		t.Fatalf("incremental parse changed admission counters: before=%d/%d after=%d/%d", routedBefore, fallbackBefore, routedAfter, fallbackAfter)
	}
	if profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "" || !profile.OldTreeReuseRoute {
		t.Fatalf("incremental reuse route was not admitted: %+v", profile)
	}
	if profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
		t.Fatalf("incremental route reused no clean sibling: %+v", profile)
	}
	if profile.ReuseRejectHasError == 0 && profile.ReuseRejectFragileNonLeaf == 0 {
		t.Fatalf("incremental route recorded no recovery rejection: %+v", profile)
	}
	if compactRecoveryContainsNode(goIncremental.RootNode(), recoveryNode) {
		t.Fatal("incremental tree reused the edited recovery node")
	}
	if !compactRecoveryContainsNode(goIncremental.RootNode(), cleanSibling) {
		t.Fatal("incremental tree did not reuse the clean class sibling")
	}
	compactRecoveryAssertCParity(t, "incremental versus fresh C", goIncremental.RootNode(), goLanguage, cFresh)
	compactRecoveryAssertCParity(t, "incremental versus incremental C", goIncremental.RootNode(), goLanguage, cIncremental)

	if got := goInitial.RootNode().SExpr(goLanguage); got != initialRootSExpr || goInitial.RootNode().Range() != initialRootRange || len(goInitial.Edits()) != initialEditCount {
		t.Fatalf("editing Tree.Copy changed the original: sexpr=%q range=%+v edits=%d", got, goInitial.RootNode().Range(), len(goInitial.Edits()))
	}
}

// TestCompactRecoveryJavaScriptInsertionRegression compares compact insertion
// with fresh Go production parsing. Fresh Go has a pre-existing C parity gap
// for this recovery witness, so insertion is not part of the C certification.
func TestCompactRecoveryJavaScriptInsertionRegression(t *testing.T) {
	lang := grammars.JavascriptLanguage()
	if !lang.CompactStrategy2ErrorRegionCertified {
		t.Fatal("JavaScript strategy-2 recovery is not certified")
	}
	source := []byte(compactRecoveryJavaScriptSource)
	ampersand := bytes.IndexByte(source, '&')
	if ampersand < 0 {
		t.Fatal("JavaScript recovery witness has no ampersand")
	}
	withoutAmpersand := append([]byte(nil), source[:ampersand]...)
	withoutAmpersand = append(withoutAmpersand, source[ampersand+1:]...)
	edit := gotreesitter.InputEdit{
		StartByte:   uint32(ampersand),
		OldEndByte:  uint32(ampersand),
		NewEndByte:  uint32(ampersand + 1),
		StartPoint:  pointAtOffset(withoutAmpersand, ampersand),
		OldEndPoint: pointAtOffset(withoutAmpersand, ampersand),
		NewEndPoint: pointAtOffset(source, ampersand+1),
	}

	parser := gotreesitter.NewParser(lang)
	parser.SetAdmissionCandidateRoute(true)
	oldTree, err := parser.Parse(withoutAmpersand)
	if err != nil {
		t.Fatalf("compact Go insertion base parse: %v", err)
	}
	defer oldTree.Release()
	oldTree.Edit(edit)
	incremental, profile, err := parser.ParseIncrementalProfiled(source, oldTree)
	if err != nil {
		t.Fatalf("compact Go insertion incremental parse: %v", err)
	}
	if incremental != oldTree {
		t.Cleanup(incremental.Release)
	}
	if profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "" || !profile.OldTreeReuseRoute ||
		profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
		t.Fatalf("compact Go insertion did not preserve the production reuse route: %+v", profile)
	}

	freshParser := gotreesitter.NewParser(lang)
	freshParser.SetAdmissionCandidateRoute(false)
	fresh, err := freshParser.Parse(source)
	if err != nil {
		t.Fatalf("fresh Go insertion parse: %v", err)
	}
	defer fresh.Release()
	incrementalDigest, err := benchfixtures.InspectGoTree(incremental.RootNode(), lang)
	if err != nil {
		t.Fatalf("inspect compact Go insertion tree: %v", err)
	}
	freshDigest, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
	if err != nil {
		t.Fatalf("inspect fresh Go insertion tree: %v", err)
	}
	if incrementalDigest.SHA256 != freshDigest.SHA256 {
		t.Fatalf("compact Go insertion digest=%s, want fresh Go digest=%s; incremental=%s fresh=%s",
			incrementalDigest.SHA256, freshDigest.SHA256, incremental.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang))
	}
}

// TestCompactRecoveryYAMLRecoverEOFIncrementalCOracle compares the certified
// recover_eof tree and proves that its uncertified scanner fails closed.
func TestCompactRecoveryYAMLRecoverEOFIncrementalCOracle(t *testing.T) {
	goLanguage := grammars.YamlLanguage()
	if !goLanguage.CompactRecoverEOFCertified {
		t.Fatal("YAML recover_eof is not certified")
	}
	cLanguage, err := ParityCLanguage("yaml")
	if err != nil {
		t.Fatalf("load YAML C oracle: %v", err)
	}
	source := []byte("[\n")
	edited := []byte("[]\n")
	edit := gotreesitter.InputEdit{
		StartByte:   1,
		OldEndByte:  1,
		NewEndByte:  2,
		StartPoint:  gotreesitter.Point{Row: 0, Column: 1},
		OldEndPoint: gotreesitter.Point{Row: 0, Column: 1},
		NewEndPoint: gotreesitter.Point{Row: 0, Column: 2},
	}

	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatalf("set YAML C oracle: %v", err)
	}
	cInitial := cParser.Parse(source, nil)
	if cInitial == nil || cInitial.RootNode() == nil {
		t.Fatal("YAML C initial parse returned no root")
	}
	t.Cleanup(cInitial.Close)

	goParser := gotreesitter.NewParser(goLanguage)
	goParser.SetAdmissionCandidateRoute(true)
	initialRoutedBefore, initialFallbackBefore := gotreesitter.AdmissionCandidateCounters()
	goInitial, err := goParser.Parse(source)
	if err != nil {
		t.Fatalf("YAML compact initial parse: %v", err)
	}
	t.Cleanup(goInitial.Release)
	initialRoutedAfter, initialFallbackAfter := gotreesitter.AdmissionCandidateCounters()
	if initialRoutedAfter-initialRoutedBefore != 1 || initialFallbackAfter-initialFallbackBefore != 0 {
		t.Fatalf("YAML initial route counters=%d/%d, want 1/0; reason=%q", initialRoutedAfter-initialRoutedBefore, initialFallbackAfter-initialFallbackBefore, gotreesitter.AdmissionCandidateLastFallbackReason())
	}
	compactRecoveryAssertCParity(t, "YAML compact initial", goInitial.RootNode(), goLanguage, cInitial)

	cFresh := cParser.Parse(edited, nil)
	if cFresh == nil || cFresh.RootNode() == nil {
		t.Fatal("YAML C fresh edited parse returned no root")
	}
	t.Cleanup(cFresh.Close)
	cOld := cParser.Parse(source, nil)
	if cOld == nil || cOld.RootNode() == nil {
		t.Fatal("YAML C old parse returned no root")
	}
	cEdit := realCorpusCInputEdit(edit)
	cOld.Edit(&cEdit)
	cIncremental := cParser.Parse(edited, cOld)
	cOld.Close()
	if cIncremental == nil || cIncremental.RootNode() == nil {
		t.Fatal("YAML C incremental parse returned no root")
	}
	t.Cleanup(cIncremental.Close)
	cFreshDigest, err := COracleDeepDigest(cFresh)
	if err != nil {
		t.Fatalf("digest YAML C fresh tree: %v", err)
	}
	cIncrementalDigest, err := COracleDeepDigest(cIncremental)
	if err != nil {
		t.Fatalf("digest YAML C incremental tree: %v", err)
	}
	if cIncrementalDigest != cFreshDigest {
		t.Fatalf("YAML C incremental digest=%s, want fresh C digest=%s", cIncrementalDigest, cFreshDigest)
	}

	goInitial.Edit(edit)
	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	goIncremental, profile, err := goParser.ParseIncrementalProfiled(edited, goInitial)
	if err != nil {
		t.Fatalf("YAML Go incremental parse: %v", err)
	}
	if goIncremental != goInitial {
		t.Cleanup(goIncremental.Release)
	}
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	if routedAfter != routedBefore || fallbackAfter != fallbackBefore {
		t.Fatalf("YAML incremental parse changed admission counters: before=%d/%d after=%d/%d", routedBefore, fallbackBefore, routedAfter, fallbackAfter)
	}
	if profile.ReuseUnsupportedReason == "old tree carried a compact recover_eof EOF runtime" {
		t.Fatalf("YAML recover_eof reported the obsolete permanent bar: %+v", profile)
	}
	if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "external_scanner_unsupported" ||
		profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
		t.Fatalf("YAML recover_eof did not fail closed for the uncertified scanner: %+v", profile)
	}
	compactRecoveryAssertCParity(t, "YAML incremental versus fresh C", goIncremental.RootNode(), goLanguage, cFresh)
	compactRecoveryAssertCParity(t, "YAML incremental versus incremental C", goIncremental.RootNode(), goLanguage, cIncremental)
	finalRuntime := goIncremental.ParseRuntime()
	if finalRuntime.StopReason != gotreesitter.ParseStopAccepted || finalRuntime.Truncated ||
		!finalRuntime.LastTokenWasEOF || finalRuntime.LastTokenEndByte != uint32(len(edited)) ||
		finalRuntime.ExpectedEOFByte != uint32(len(edited)) || finalRuntime.RootEndByte != uint32(len(edited)) ||
		finalRuntime.RootEndByte != goIncremental.RootNode().EndByte() {
		t.Fatalf("YAML recover_eof fallback runtime=%+v, want fresh EOF at %d", finalRuntime, len(edited))
	}
}

type compactRecoveryErrorSummary struct {
	isError  int
	missing  int
	hasError int
}

func compactRecoveryAssertCParity(t *testing.T, label string, goRoot *gotreesitter.Node, goLanguage *gotreesitter.Language, cTree *sitter.Tree) {
	t.Helper()
	if cTree == nil {
		t.Fatalf("%s has a nil C tree", label)
	}
	cRoot := cTree.RootNode()
	if goRoot == nil || cRoot == nil {
		t.Fatalf("%s has a nil root: Go=%v C=%v", label, goRoot == nil, cRoot == nil)
	}
	if divergences := compactT3StructuralDivergences(goRoot, goLanguage, cRoot); len(divergences) != 0 {
		t.Fatalf("%s has %d structural divergences; first=%s", label, len(divergences), compactT3FormatDivergence(divergences[0]))
	}
	goDigest, err := benchfixtures.InspectGoTree(goRoot, goLanguage)
	if err != nil {
		t.Fatalf("%s inspect Go tree: %v", label, err)
	}
	cDigest, err := COracleDeepDigest(cTree)
	if err != nil {
		t.Fatalf("%s inspect C tree: %v", label, err)
	}
	if goDigest.SHA256 != cDigest {
		t.Fatalf("%s deep digest Go=%s C=%s", label, goDigest.SHA256, cDigest)
	}
	goSummary := compactRecoveryGoErrorSummary(goRoot)
	cSummary := compactRecoveryCErrorSummary(cRoot)
	if goSummary != cSummary {
		t.Fatalf("%s error summary Go=%+v C=%+v", label, goSummary, cSummary)
	}
}

func compactRecoveryFindCleanDirectChild(root *gotreesitter.Node, language *gotreesitter.Language, kind string) *gotreesitter.Node {
	if root == nil || language == nil {
		return nil
	}
	for index := 0; index < root.ChildCount(); index++ {
		child := root.Child(index)
		if child != nil && child.Type(language) == kind && !child.IsError() && !child.IsMissing() && !child.HasError() && !child.IsExtra() {
			return child
		}
	}
	return nil
}

func compactRecoveryFindProblemNode(root *gotreesitter.Node) *gotreesitter.Node {
	if root == nil {
		return nil
	}
	if root.IsError() || root.IsMissing() {
		return root
	}
	for index := 0; index < root.ChildCount(); index++ {
		if problem := compactRecoveryFindProblemNode(root.Child(index)); problem != nil {
			return problem
		}
	}
	return nil
}

func compactRecoveryFindCProblemNode(root *sitter.Node) *sitter.Node {
	if root == nil {
		return nil
	}
	if root.IsError() || root.IsMissing() {
		return root
	}
	for index := uint(0); index < root.ChildCount(); index++ {
		if problem := compactRecoveryFindCProblemNode(root.Child(index)); problem != nil {
			return problem
		}
	}
	return nil
}

func compactRecoveryContainsNode(root, target *gotreesitter.Node) bool {
	if root == nil || target == nil {
		return false
	}
	if root == target {
		return true
	}
	for index := 0; index < root.ChildCount(); index++ {
		if compactRecoveryContainsNode(root.Child(index), target) {
			return true
		}
	}
	return false
}

func compactRecoveryGoErrorSummary(root *gotreesitter.Node) compactRecoveryErrorSummary {
	if root == nil {
		return compactRecoveryErrorSummary{}
	}
	summary := compactRecoveryErrorSummary{}
	if root.IsError() {
		summary.isError++
	}
	if root.IsMissing() {
		summary.missing++
	}
	if root.HasError() {
		summary.hasError++
	}
	for index := 0; index < root.ChildCount(); index++ {
		child := compactRecoveryGoErrorSummary(root.Child(index))
		summary.isError += child.isError
		summary.missing += child.missing
		summary.hasError += child.hasError
	}
	return summary
}

func compactRecoveryCErrorSummary(root *sitter.Node) compactRecoveryErrorSummary {
	if root == nil {
		return compactRecoveryErrorSummary{}
	}
	summary := compactRecoveryErrorSummary{}
	if root.IsError() {
		summary.isError++
	}
	if root.IsMissing() {
		summary.missing++
	}
	if root.HasError() {
		summary.hasError++
	}
	for index := uint(0); index < root.ChildCount(); index++ {
		child := compactRecoveryCErrorSummary(root.Child(index))
		summary.isError += child.isError
		summary.missing += child.missing
		summary.hasError += child.hasError
	}
	return summary
}
