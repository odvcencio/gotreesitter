//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Package-two adversarial lane witnesses (issue #1063; supports #1057 and
// #1053). These tests lock the smallest physical GSS merge cases through a
// nullable recovery discontinuity edge. All four witnesses require the
// compact route to publish the locked C result. All four use locked Scala.
//
// Every witness runs the same four checks, in order:
//
//  1. The locked C oracle tree: exact S-expression, root range, error flag,
//     the missing-leaf position the issue names, and the deep digest.
//  2. The Go production route (candidate route forced off) against the
//     same fields.
//  3. The Go compact route (candidate route forced on): an exact match for
//     strict witnesses, or an explicit fallback for transitional witnesses.
//  4. One incremental edit: insert one space at byte 1, apply the edit to
//     the old Go tree, reparse incrementally, and compare against a fresh
//     Go parse and a fresh C parse of the edited source.

// package2ScalaFalsifierWitness pins one locked-C regression witness.
type package2ScalaFalsifierWitness struct {
	// name identifies the witness in log lines and failure messages.
	name string
	// source is the exact byte sequence under test.
	source string
	// sourceSHA256 pins source so the witness cannot drift silently.
	sourceSHA256 string

	// wantCSExpr is the exact named-node S-expression the locked C oracle
	// must publish.
	wantCSExpr string
	// wantRootType and wantRootHasError pin the published root shape. The
	// root range is always 0..len(source) for these witnesses.
	wantRootType     string
	wantRootHasError bool
	// wantMissingKind and wantMissingByte pin the position the issue names
	// for the missing leaf C inserts during recovery.
	wantMissingKind string
	wantMissingByte uint32
	// wantNoErrorNode asserts C published no visible ERROR node even though
	// the root carries HasError -- see #1053's true EOF composition
	// witness.
	wantNoErrorNode bool
	// wantDeepDigest pins the C oracle's deep tree digest.
	wantDeepDigest string

	// wantProductionMatchesC records whether the Go production route (the
	// candidate route forced off) is expected to match C exactly today. If
	// this is false, the test records the first divergence with t.Skipf
	// instead of weakening the assertion -- that is a finding for #1056.
	wantProductionMatchesC bool

	// wantCompactExact records whether the Go compact route (the candidate
	// route forced on) must match C exactly. The certified package-two
	// witnesses are strict: generic recovery fallback is not accepted.
	wantCompactExact bool
	// wantCompactFallbackReasonPart, when wantCompactExact is false, must be
	// a substring of the recorded fallback reason. Silent divergence
	// (neither an exact match nor a recorded fallback) fails the test.
	wantCompactFallbackReasonPart string

	// wantEditedMissingByte pins the missing-leaf byte after the byte-1
	// space-insertion edit exercised in step 4.
	wantEditedMissingByte uint32
	// wantIncrementalReuseUnsupported records whether ParseIncrementalProfiled
	// is expected to report ReuseUnsupported for the edit. The Scala owned
	// scanner does not support reuse yet (see #1054), so every locked
	// witness here expects true.
	wantIncrementalReuseUnsupported bool
}

// TestPackage2ScalaFalsifierPhysicalMergeMinimal locks `(y; `, the smallest
// physical-merge falsifier from #1053/#1057. Locked C selects the
// missing-closing-parenthesis tree at cost 610; five physical merges retain
// six absorber paths; there is no ancestor or EOF recovery.
func TestPackage2ScalaFalsifierPhysicalMergeMinimal(t *testing.T) {
	runPackage2ScalaFalsifierWitness(t, package2ScalaFalsifierWitness{
		name:                            "physical_merge_minimal",
		source:                          "(y; ",
		sourceSHA256:                    "80bfe1eb9ff4fb73fa504f6210d5c628431913736b1093c4c70f354ecd408e64",
		wantCSExpr:                      "(compilation_unit (parenthesized_expression (identifier)))",
		wantRootType:                    "compilation_unit",
		wantRootHasError:                true,
		wantMissingKind:                 ")",
		wantMissingByte:                 2,
		wantDeepDigest:                  "06d2d9f6b02599ec5be0808330bf1c62e49b8cb0b146d094ceaa28b9185c79b6",
		wantProductionMatchesC:          true,
		wantCompactExact:                true,
		wantEditedMissingByte:           3,
		wantIncrementalReuseUnsupported: true,
	})
}

// TestPackage2ScalaFalsifierOwnedLexerComposition locks `((y)->; `, the
// owned-lexer composition witness from #1053: two lexer widths and one
// viability drop before the same recovery obligation as the minimal
// witness.
func TestPackage2ScalaFalsifierOwnedLexerComposition(t *testing.T) {
	runPackage2ScalaFalsifierWitness(t, package2ScalaFalsifierWitness{
		name:                            "owned_lexer_composition",
		source:                          "((y)->; ",
		sourceSHA256:                    "2645c366cd69f43fb554d3556dbe127f0d1ff834d6197ea21cb9b0228a54e0a5",
		wantCSExpr:                      "(compilation_unit (parenthesized_expression (postfix_expression (parenthesized_expression (identifier)) (operator_identifier))))",
		wantRootType:                    "compilation_unit",
		wantRootHasError:                true,
		wantMissingKind:                 ")",
		wantMissingByte:                 6,
		wantDeepDigest:                  "c0de79617c3e1bdac71c1686e5d6a3d4b2fb375aca9f69902aee974c5ca9f852",
		wantProductionMatchesC:          true,
		wantCompactExact:                true,
		wantEditedMissingByte:           7,
		wantIncrementalReuseUnsupported: true,
	})
}

// TestPackage2ScalaFalsifierNoSummaryEOF locks `(y;` (no trailing space),
// the no-summary EOF rung from #1053. It separates multi-path recover_eof
// from group-wide summary recovery.
func TestPackage2ScalaFalsifierNoSummaryEOF(t *testing.T) {
	runPackage2ScalaFalsifierWitness(t, package2ScalaFalsifierWitness{
		name:                            "no_summary_eof",
		source:                          "(y;",
		sourceSHA256:                    "cbc573a6f4346115614483aeb27146bbcbcdaf1dae24fec7a4d0968a6f57e61a",
		wantCSExpr:                      "(compilation_unit (parenthesized_expression (identifier)))",
		wantRootType:                    "compilation_unit",
		wantRootHasError:                true,
		wantMissingKind:                 ")",
		wantMissingByte:                 2,
		wantDeepDigest:                  "c32aa60988eb48fd8de48b59f40125eb648ecf9507cc7cf06651e3e153d59b1b",
		wantProductionMatchesC:          true,
		wantCompactExact:                true,
		wantEditedMissingByte:           3,
		wantIncrementalReuseUnsupported: true,
	})
}

// TestPackage2ScalaFalsifierTrueEOFComposition locks `((y)->` (6 bytes),
// the true EOF composition witness from #1053. C publishes
// `(compilation_unit (parenthesized_expression (postfix_expression
// (parenthesized_expression (identifier)) (operator_identifier))))` with
// one missing `)` at byte 6; the root carries HasError, but C publishes no
// visible ERROR node.
func TestPackage2ScalaFalsifierTrueEOFComposition(t *testing.T) {
	runPackage2ScalaFalsifierWitness(t, package2ScalaFalsifierWitness{
		name:                            "true_eof_composition",
		source:                          "((y)->",
		sourceSHA256:                    "66568f2d2cf310e447ed512da7a3fad0be45158327905c4ffd0dd857bd90934c",
		wantCSExpr:                      "(compilation_unit (parenthesized_expression (postfix_expression (parenthesized_expression (identifier)) (operator_identifier))))",
		wantRootType:                    "compilation_unit",
		wantRootHasError:                true,
		wantMissingKind:                 ")",
		wantMissingByte:                 6,
		wantNoErrorNode:                 true,
		wantDeepDigest:                  "03a458abd5832f6326e03b833a3890ea8d48ca43eb91ada1951bb020871f49a6",
		wantProductionMatchesC:          true,
		wantCompactExact:                true,
		wantEditedMissingByte:           7,
		wantIncrementalReuseUnsupported: true,
	})
}

// TestPackage2ScalaIncrementalCleanMissingTransitions proves both invalidation
// directions around the smallest package-two recovery point. The certified
// scanner fails closed because each edit changes width.
func TestPackage2ScalaIncrementalCleanMissingTransitions(t *testing.T) {
	runPackage2ScalaIncrementalCleanMissingTransitions(t, []byte("(y); "), []byte("(y; "), 2, "", "")
}

// TestPackage2ScalaIncrementalEOFCompositionCleanMissingTransitions locks the
// clean control and both edit directions for the true EOF composition witness.
func TestPackage2ScalaIncrementalEOFCompositionCleanMissingTransitions(t *testing.T) {
	runPackage2ScalaIncrementalCleanMissingTransitions(
		t,
		[]byte("((y)->)"),
		[]byte("((y)->"),
		6,
		"5e44dd6eed19285e0430c21c6ff85828340ed44e89534bde49f6e2e336e9acf4",
		"03a458abd5832f6326e03b833a3890ea8d48ca43eb91ada1951bb020871f49a6",
	)
}

func TestPackage2ScalaIncrementalScannerStateEdits(t *testing.T) {
	tests := []struct {
		name               string
		before             []byte
		oldText            []byte
		newText            []byte
		wantFallbackReason string
	}{
		{
			name:               "token invariant number",
			before:             []byte("object Prelude:\n  val fixed = 0\n\nobject A:\n  val value = 2\n  val sibling = 3\n"),
			oldText:            []byte("2"),
			newText:            []byte("4"),
			wantFallbackReason: "",
		},
		{
			name:               "before indentation state",
			before:             []byte("object A:\n  def value =\n    1\n  def sibling = 2\n"),
			oldText:            []byte("A"),
			newText:            []byte("B"),
			wantFallbackReason: "external_scanner_unsupported",
		},
		{
			name:               "before interpolation state",
			before:             []byte("object A:\n  val prefix = 1\n  val name = \"x\"\n  val greeting = s\"hello $name\"\n"),
			oldText:            []byte("1"),
			newText:            []byte("2"),
			wantFallbackReason: "",
		},
		{
			name:               "indentation width",
			before:             []byte("object A:\n  def value =\n    1\n  def sibling = 2\n"),
			oldText:            []byte("    1"),
			newText:            []byte("      1"),
			wantFallbackReason: "external_scanner_unsupported",
		},
		{
			name:               "interpolation identifier",
			before:             []byte("object Prelude:\n  val fixed = 0\n\nobject A:\n  val name = \"x\"\n  val greeting = s\"hello $name\"\n  val sibling = 2\n"),
			oldText:            []byte("$name"),
			newText:            []byte("$game"),
			wantFallbackReason: "external_scanner_unsupported",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			forward := replacePackage2ScalaEditText(t, test.before, test.oldText, test.newText)
			directions := []struct {
				name, oldText, newText string
				before, after          []byte
			}{
				{name: "forward", oldText: string(test.oldText), newText: string(test.newText), before: test.before, after: forward},
				{name: "reverse", oldText: string(test.newText), newText: string(test.oldText), before: forward, after: test.before},
			}
			for _, direction := range directions {
				t.Run(direction.name, func(t *testing.T) {
					for _, compactOldTree := range []bool{false, true} {
						routeName := "production-old-tree"
						if compactOldTree {
							routeName = "compact-old-tree"
						}
						t.Run(routeName, func(t *testing.T) {
							oldText := []byte(direction.oldText)
							newText := []byte(direction.newText)
							offset := bytes.Index(direction.before, oldText)
							if offset < 0 {
								t.Fatal("Scala scanner-state edit site is missing")
							}
							edit := gotreesitter.InputEdit{
								StartByte:   uint32(offset),
								OldEndByte:  uint32(offset + len(oldText)),
								NewEndByte:  uint32(offset + len(newText)),
								StartPoint:  pointAtOffset(direction.before, offset),
								OldEndPoint: pointAtOffset(direction.before, offset+len(oldText)),
								NewEndPoint: pointAtOffset(direction.after, offset+len(newText)),
							}
							runPackage2ScalaScannerStateEdit(
								t, direction.before, direction.after, edit,
								test.wantFallbackReason, compactOldTree,
							)
						})
					}
				})
			}
		})
	}
}

func replacePackage2ScalaEditText(t *testing.T, source, oldText, newText []byte) []byte {
	t.Helper()
	offset := bytes.Index(source, oldText)
	if offset < 0 {
		t.Fatal("Scala scanner-state edit site is missing")
	}
	after := make([]byte, 0, len(source)-len(oldText)+len(newText))
	after = append(after, source[:offset]...)
	after = append(after, newText...)
	after = append(after, source[offset+len(oldText):]...)
	return after
}

func runPackage2ScalaScannerStateEdit(
	t *testing.T,
	before, after []byte,
	edit gotreesitter.InputEdit,
	wantFallbackReason string,
	compactOldTree bool,
) {
	t.Helper()
	goLanguage := grammars.ScalaLanguage()
	cLanguage, err := ParityCLanguage("scala")
	if err != nil {
		t.Fatalf("load locked Scala C language: %v", err)
	}
	assertPackage2ScalaCIncrementalTransition(t, cLanguage, before, after, edit)

	parser := gotreesitter.NewParser(goLanguage)
	parser.SetAdmissionCandidateRoute(compactOldTree)
	beforeRouted, beforeFallback := gotreesitter.AdmissionCandidateCounters()
	oldTree, err := parser.Parse(before)
	if err != nil {
		t.Fatalf("parse Scala scanner-state source: %v", err)
	}
	t.Cleanup(oldTree.Release)
	if oldTree.RootNode() == nil || oldTree.RootNode().HasError() {
		t.Fatalf("Scala scanner-state source is not clean: %v", oldTree.RootNode())
	}
	if compactOldTree {
		routed, fallback := gotreesitter.AdmissionCandidateCounters()
		if routed-beforeRouted != 1 || fallback-beforeFallback != 0 ||
			!oldTree.ParseRuntime().CompactExternalScannerCheckpointTransferProven {
			t.Fatalf("compact Scala old-tree route=%d/%d transfer=%t, want 1/0/true",
				routed-beforeRouted, fallback-beforeFallback,
				oldTree.ParseRuntime().CompactExternalScannerCheckpointTransferProven)
		}
	}
	assertPackage2ScalaTransitionEndpoint(t, goLanguage, cLanguage, before, oldTree, false, 0, "")
	oldTree.Edit(edit)
	incremental, profile, err := parser.ParseIncrementalProfiled(after, oldTree)
	if err != nil {
		t.Fatalf("incremental Scala scanner-state parse: %v", err)
	}
	t.Cleanup(incremental.Release)
	effectiveFallbackReason := wantFallbackReason
	if compactOldTree && effectiveFallbackReason == "" {
		effectiveFallbackReason = "external_scanner_unsupported"
	}
	if effectiveFallbackReason != "" {
		if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != effectiveFallbackReason ||
			profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
			t.Fatalf("Scala scanner-state fallback profile=%+v", profile)
		}
	} else if profile.ReuseUnsupported || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
		t.Fatalf("Scala scanner-state edit did not reuse authenticated syntax: %+v", profile)
	}

	fresh, err := gotreesitter.NewParser(goLanguage).Parse(after)
	if err != nil {
		t.Fatalf("fresh Scala scanner-state parse: %v", err)
	}
	t.Cleanup(fresh.Release)
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatalf("set locked Scala C language: %v", err)
	}
	cFresh := cParser.Parse(after, nil)
	if cFresh == nil || cFresh.RootNode() == nil {
		t.Fatal("locked Scala C parser returned no scanner-state tree")
	}
	t.Cleanup(cFresh.Close)
	for _, candidate := range []struct {
		name string
		tree *gotreesitter.Tree
	}{{"incremental", incremental}, {"fresh", fresh}} {
		root := candidate.tree.RootNode()
		if diff := FirstDivergenceDumpV1(root, goLanguage, cFresh.RootNode()); diff != nil {
			t.Fatalf("%s Scala scanner-state tree diverges from locked C: %+v", candidate.name, *diff)
		}
		if diff := firstLockedCTreeFlagDivergence(root, goLanguage, cFresh.RootNode(), "/"+root.Type(goLanguage)); diff != nil {
			t.Fatalf("%s Scala scanner-state flags diverge from locked C: %v", candidate.name, diff)
		}
		inspection, err := benchfixtures.InspectGoTree(root, goLanguage)
		if err != nil {
			t.Fatalf("inspect %s Scala scanner-state tree: %v", candidate.name, err)
		}
		cDigest, err := COracleDeepDigest(cFresh)
		if err != nil {
			t.Fatalf("inspect locked C Scala scanner-state tree: %v", err)
		}
		if inspection.SHA256 != cDigest {
			t.Fatalf("%s Scala scanner-state digest=%s, want locked C %s", candidate.name, inspection.SHA256, cDigest)
		}
	}
}

func runPackage2ScalaIncrementalCleanMissingTransitions(
	t *testing.T,
	clean, missing []byte,
	closeByte int,
	wantCleanDigest, wantMissingDigest string,
) {
	t.Helper()
	entry := grammars.DetectLanguageByName("scala")
	if entry == nil || entry.Language() == nil {
		t.Fatal("Scala Go grammar is unavailable")
	}
	goLanguage := entry.Language()
	cLanguage, err := ParityCLanguage("scala")
	if err != nil {
		t.Fatalf("load locked Scala C language: %v", err)
	}

	parser := gotreesitter.NewParser(goLanguage)
	parser.SetAdmissionCandidateRoute(false)
	cleanTree, err := parser.Parse(clean)
	if err != nil {
		t.Fatalf("parse clean source: %v", err)
	}
	t.Cleanup(cleanTree.Release)

	deleteClose := gotreesitter.InputEdit{
		StartByte: uint32(closeByte), OldEndByte: uint32(closeByte + 1), NewEndByte: uint32(closeByte),
		StartPoint:  pointAtOffset(clean, closeByte),
		OldEndPoint: pointAtOffset(clean, closeByte+1),
		NewEndPoint: pointAtOffset(missing, closeByte),
	}
	assertPackage2ScalaCIncrementalTransition(t, cLanguage, clean, missing, deleteClose)
	cleanTree.Edit(deleteClose)
	missingTree, missingProfile, err := parser.ParseIncrementalProfiled(missing, cleanTree)
	if err != nil {
		t.Fatalf("delete closing parenthesis: %v", err)
	}
	t.Cleanup(missingTree.Release)
	if !missingProfile.ReuseUnsupported ||
		missingProfile.ReuseUnsupportedReason != "external_scanner_unsupported" {
		t.Fatalf("delete profile=%+v, want the general scanner fallback", missingProfile)
	}
	assertPackage2ScalaTransitionEndpoint(t, goLanguage, cLanguage, missing, missingTree, true, 1, wantMissingDigest)

	insertClose := gotreesitter.InputEdit{
		StartByte: uint32(closeByte), OldEndByte: uint32(closeByte), NewEndByte: uint32(closeByte + 1),
		StartPoint:  pointAtOffset(missing, closeByte),
		OldEndPoint: pointAtOffset(missing, closeByte),
		NewEndPoint: pointAtOffset(clean, closeByte+1),
	}
	assertPackage2ScalaCIncrementalTransition(t, cLanguage, missing, clean, insertClose)
	missingTree.Edit(insertClose)
	restoredTree, restoredProfile, err := parser.ParseIncrementalProfiled(clean, missingTree)
	if err != nil {
		t.Fatalf("restore closing parenthesis: %v", err)
	}
	t.Cleanup(restoredTree.Release)
	if !restoredProfile.ReuseUnsupported ||
		restoredProfile.ReuseUnsupportedReason != "external_scanner_error_tree_unsupported" {
		t.Fatalf("restore profile=%+v, want the recovery-tree fallback", restoredProfile)
	}
	assertPackage2ScalaTransitionEndpoint(t, goLanguage, cLanguage, clean, restoredTree, false, 0, wantCleanDigest)
}

func assertPackage2ScalaCIncrementalTransition(
	t *testing.T,
	cLanguage *sitter.Language,
	before, after []byte,
	edit gotreesitter.InputEdit,
) {
	t.Helper()
	parser := sitter.NewParser()
	t.Cleanup(parser.Close)
	if err := parser.SetLanguage(cLanguage); err != nil {
		t.Fatalf("set locked Scala C language: %v", err)
	}
	oldTree := parser.Parse(before, nil)
	if oldTree == nil || oldTree.RootNode() == nil {
		t.Fatal("locked Scala C parser returned no old tree")
	}
	cEdit := realCorpusCInputEdit(edit)
	oldTree.Edit(&cEdit)
	incremental := parser.Parse(after, oldTree)
	oldTree.Close()
	if incremental == nil || incremental.RootNode() == nil {
		t.Fatal("locked Scala C parser returned no incremental tree")
	}
	t.Cleanup(incremental.Close)
	fresh := parser.Parse(after, nil)
	if fresh == nil || fresh.RootNode() == nil {
		t.Fatal("locked Scala C parser returned no fresh tree")
	}
	t.Cleanup(fresh.Close)
	incrementalDigest, err := COracleDeepDigest(incremental)
	if err != nil {
		t.Fatalf("inspect locked Scala C incremental tree: %v", err)
	}
	freshDigest, err := COracleDeepDigest(fresh)
	if err != nil {
		t.Fatalf("inspect locked Scala C fresh tree: %v", err)
	}
	if incrementalDigest != freshDigest {
		t.Fatalf("locked Scala C incremental digest=%s, want fresh %s", incrementalDigest, freshDigest)
	}
}

func assertPackage2ScalaTransitionEndpoint(
	t *testing.T,
	goLanguage *gotreesitter.Language,
	cLanguage *sitter.Language,
	source []byte,
	incremental *gotreesitter.Tree,
	wantError bool,
	wantMissing int,
	wantDigest string,
) {
	t.Helper()
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatalf("set locked Scala C language: %v", err)
	}
	cTree := cParser.Parse(source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("locked Scala C parser returned no endpoint tree")
	}
	t.Cleanup(cTree.Close)
	cRoot := cTree.RootNode()
	if cRoot.HasError() != wantError || len(findMissingCNodes(cRoot)) != wantMissing {
		t.Fatalf("locked C endpoint error/missing=%t/%d, want %t/%d", cRoot.HasError(), len(findMissingCNodes(cRoot)), wantError, wantMissing)
	}

	assertGoEndpoint := func(label string, tree *gotreesitter.Tree) {
		t.Helper()
		root := tree.RootNode()
		if diff := FirstDivergenceDumpV1(root, goLanguage, cRoot); diff != nil {
			t.Fatalf("%s endpoint diverges from locked C: %+v", label, *diff)
		}
		if diff := firstLockedCTreeFlagDivergence(root, goLanguage, cRoot, "/"+root.Type(goLanguage)); diff != nil {
			t.Fatalf("%s endpoint flags diverge from locked C: %v", label, diff)
		}
		if root.HasError() != wantError || len(findMissingGoNodes(root, goLanguage)) != wantMissing {
			t.Fatalf("%s endpoint error/missing=%t/%d, want %t/%d", label, root.HasError(), len(findMissingGoNodes(root, goLanguage)), wantError, wantMissing)
		}
	}
	assertGoEndpoint("incremental", incremental)

	freshParser := gotreesitter.NewParser(goLanguage)
	freshParser.SetAdmissionCandidateRoute(false)
	freshTree, err := freshParser.Parse(source)
	if err != nil {
		t.Fatalf("fresh endpoint parse: %v", err)
	}
	t.Cleanup(freshTree.Release)
	assertGoEndpoint("fresh production", freshTree)

	beforeRouted, beforeFallback := gotreesitter.AdmissionCandidateCounters()
	compactParser := gotreesitter.NewParser(goLanguage)
	compactParser.SetAdmissionCandidateRoute(true)
	compactTree, err := compactParser.Parse(source)
	if err != nil {
		t.Fatalf("compact endpoint parse: %v", err)
	}
	t.Cleanup(compactTree.Release)
	routed, fallback := gotreesitter.AdmissionCandidateCounters()
	if routed-beforeRouted != 1 || fallback-beforeFallback != 0 {
		t.Fatalf("compact endpoint routed/fallback=%d/%d, want 1/0; reason=%q", routed-beforeRouted, fallback-beforeFallback, gotreesitter.AdmissionCandidateLastFallbackReason())
	}
	assertGoEndpoint("fresh compact", compactTree)

	cDigest, err := COracleDeepDigest(cTree)
	if err != nil {
		t.Fatalf("inspect locked C endpoint: %v", err)
	}
	if wantDigest != "" && cDigest != wantDigest {
		t.Fatalf("locked C endpoint digest=%s, want %s", cDigest, wantDigest)
	}
	for _, candidate := range []struct {
		name string
		tree *gotreesitter.Tree
	}{{"incremental", incremental}, {"fresh production", freshTree}, {"fresh compact", compactTree}} {
		inspection, err := benchfixtures.InspectGoTree(candidate.tree.RootNode(), goLanguage)
		if err != nil {
			t.Fatalf("inspect %s endpoint: %v", candidate.name, err)
		}
		if inspection.SHA256 != cDigest {
			t.Fatalf("%s endpoint digest=%s, want locked C %s", candidate.name, inspection.SHA256, cDigest)
		}
	}
}

func runPackage2ScalaFalsifierWitness(t *testing.T, w package2ScalaFalsifierWitness) {
	t.Helper()
	source := []byte(w.source)
	if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != w.sourceSHA256 {
		t.Fatalf("%s source SHA-256=%s, want %s (witness source drifted)", w.name, got, w.sourceSHA256)
	}

	cLanguage, err := ParityCLanguage("scala")
	if err != nil {
		t.Fatalf("%s: load locked Scala C language: %v", w.name, err)
	}
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatalf("%s: set locked Scala C language: %v", w.name, err)
	}
	cTree := cParser.Parse(source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatalf("%s: locked C parser returned no root", w.name)
	}
	t.Cleanup(cTree.Close)
	cRoot := cTree.RootNode()

	// Step 1: the locked C oracle tree, exactly.
	if got := formatCNodeSExpr(cRoot); got != w.wantCSExpr {
		t.Fatalf("%s: locked C tree=%q, want %q", w.name, got, w.wantCSExpr)
	}
	if cRoot.Kind() != w.wantRootType || cRoot.StartByte() != 0 || cRoot.EndByte() != uint(len(source)) || cRoot.HasError() != w.wantRootHasError {
		t.Fatalf("%s: locked C root type/range/error=%q/%d..%d/%t, want %s/0..%d/%t",
			w.name, cRoot.Kind(), cRoot.StartByte(), cRoot.EndByte(), cRoot.HasError(),
			w.wantRootType, len(source), w.wantRootHasError)
	}
	cMissing := findMissingCNodes(cRoot)
	if len(cMissing) != 1 || cMissing[0].kind != w.wantMissingKind || cMissing[0].byteOffset != w.wantMissingByte {
		t.Fatalf("%s: locked C missing leaves=%+v, want exactly one %q at byte %d", w.name, cMissing, w.wantMissingKind, w.wantMissingByte)
	}
	if w.wantNoErrorNode {
		if path := findErrorCNode(cRoot, "/"+cRoot.Kind()); path != "" {
			t.Fatalf("%s: locked C tree has a visible ERROR node at %s, want none", w.name, path)
		}
	}
	cDigest, err := COracleDeepDigest(cTree)
	if err != nil {
		t.Fatalf("%s: inspect locked C tree: %v", w.name, err)
	}
	if cDigest != w.wantDeepDigest {
		t.Fatalf("%s: locked C deep digest=%s, want %s", w.name, cDigest, w.wantDeepDigest)
	}
	t.Logf("%s: locked C oracle exact -- sexpr, root 0..%d/error=%t, missing %q@%d, digest %s",
		w.name, len(source), w.wantRootHasError, w.wantMissingKind, w.wantMissingByte, cDigest)

	entry := grammars.DetectLanguageByName("scala")
	if entry == nil || entry.Language() == nil {
		t.Fatalf("%s: Scala Go grammar is unavailable", w.name)
	}
	goLanguage := entry.Language()

	// Step 2: the Go production route (candidate route forced off).
	prodParser := gotreesitter.NewParser(goLanguage)
	prodParser.SetAdmissionCandidateRoute(false)
	prodTree, err := prodParser.Parse(source)
	if err != nil {
		t.Fatalf("%s: production parse: %v", w.name, err)
	}
	t.Cleanup(prodTree.Release)
	prodRoot := prodTree.RootNode()

	if diff := FirstDivergenceDumpV1(prodRoot, goLanguage, cRoot); diff != nil {
		if !w.wantProductionMatchesC {
			t.Skipf("%s: PACKAGE-2 FINDING FOR #1056 -- Go production route diverges from locked C at %s: category=%s go=%q c=%q",
				w.name, diff.Path, diff.Category, diff.GoValue, diff.CValue)
		}
		t.Fatalf("%s: production tree diverges from locked C: %+v", w.name, *diff)
	}
	if diff := firstLockedCTreeFlagDivergence(prodRoot, goLanguage, cRoot, "/"+prodRoot.Type(goLanguage)); diff != nil {
		if !w.wantProductionMatchesC {
			t.Skipf("%s: PACKAGE-2 FINDING FOR #1056 -- Go production route flags diverge from locked C: %v", w.name, diff)
		}
		t.Fatalf("%s: production flags diverge from locked C: %v", w.name, diff)
	}
	if !w.wantProductionMatchesC {
		t.Fatalf("%s: witness declared a known production divergence but none was observed -- update wantProductionMatchesC", w.name)
	}
	prodMissing := findMissingGoNodes(prodRoot, goLanguage)
	if len(prodMissing) != 1 || prodMissing[0].kind != w.wantMissingKind || prodMissing[0].byteOffset != w.wantMissingByte {
		t.Fatalf("%s: production missing leaves=%+v, want exactly one %q at byte %d", w.name, prodMissing, w.wantMissingKind, w.wantMissingByte)
	}
	prodInspection, err := benchfixtures.InspectGoTree(prodRoot, goLanguage)
	if err != nil {
		t.Fatalf("%s: inspect production tree: %v", w.name, err)
	}
	if prodInspection.SHA256 != cDigest {
		t.Fatalf("%s: production deep digest=%s, want locked C %s", w.name, prodInspection.SHA256, cDigest)
	}
	t.Logf("%s: Go production route matches locked C exactly (digest %s)", w.name, prodInspection.SHA256)

	// Step 3: the Go compact route, with the candidate route forced on. The
	// certified package-two witnesses require an exact route. Silent divergence
	// fails.
	beforeRouted, beforeFallback := gotreesitter.AdmissionCandidateCounters()
	compactParser := gotreesitter.NewParser(goLanguage)
	compactParser.SetAdmissionCandidateRoute(true)
	compactTree, err := compactParser.Parse(source)
	if err != nil {
		t.Fatalf("%s: compact candidate parse: %v", w.name, err)
	}
	t.Cleanup(compactTree.Release)
	routed, fallback := gotreesitter.AdmissionCandidateCounters()
	routedDelta := routed - beforeRouted
	fallbackDelta := fallback - beforeFallback
	reason := gotreesitter.AdmissionCandidateLastFallbackReason()

	switch {
	case routedDelta == 1 && fallbackDelta == 0:
		compactRoot := compactTree.RootNode()
		if diff := FirstDivergenceDumpV1(compactRoot, goLanguage, cRoot); diff != nil {
			t.Fatalf("%s: compact route reported an exact route but the tree diverges from locked C: %+v", w.name, *diff)
		}
		if diff := firstLockedCTreeFlagDivergence(compactRoot, goLanguage, cRoot, "/"+compactRoot.Type(goLanguage)); diff != nil {
			t.Fatalf("%s: compact route reported an exact route but flags diverge from locked C: %v", w.name, diff)
		}
		compactInspection, err := benchfixtures.InspectGoTree(compactRoot, goLanguage)
		if err != nil {
			t.Fatalf("%s: inspect compact tree: %v", w.name, err)
		}
		if compactInspection.SHA256 != cDigest {
			t.Fatalf("%s: compact route reported an exact route but deep digest=%s, want locked C %s", w.name, compactInspection.SHA256, cDigest)
		}
		if !w.wantCompactExact {
			t.Logf("%s: PACKAGE-2 OUTCOME: compact route now matches locked C exactly (package two appears implemented) -- update wantCompactExact", w.name)
		} else {
			t.Logf("%s: compact route matches locked C exactly (digest %s)", w.name, compactInspection.SHA256)
		}
	case routedDelta == 0 && fallbackDelta == 1:
		if w.wantCompactExact {
			t.Fatalf("%s: compact route used generic recovery fallback, want the exact compact route (routed=%d fallback=%d reason=%q)",
				w.name, routedDelta, fallbackDelta, reason)
		}
		if w.wantCompactFallbackReasonPart != "" && !strings.Contains(reason, w.wantCompactFallbackReasonPart) {
			t.Fatalf("%s: compact route fallback reason=%q, want substring %q", w.name, reason, w.wantCompactFallbackReasonPart)
		}
		t.Logf("%s: PACKAGE-2 OUTCOME: compact route fell back explicitly, reason=%q", w.name, reason)
	default:
		t.Fatalf("%s: compact route admission counters=%d routed/%d fallback, want exactly one of (1,0) or (0,1) -- silent divergence risk; reason=%q",
			w.name, routedDelta, fallbackDelta, reason)
	}

	// Step 4: one incremental edit -- insert one space at byte 1.
	editParser := gotreesitter.NewParser(goLanguage)
	editParser.SetAdmissionCandidateRoute(false)
	oldTree, err := editParser.Parse(source)
	if err != nil {
		t.Fatalf("%s: build old tree for incremental edit: %v", w.name, err)
	}
	t.Cleanup(oldTree.Release)

	edited := make([]byte, 0, len(source)+1)
	edited = append(edited, source[:1]...)
	edited = append(edited, ' ')
	edited = append(edited, source[1:]...)

	edit := gotreesitter.InputEdit{
		StartByte:   1,
		OldEndByte:  1,
		NewEndByte:  2,
		StartPoint:  pointAtOffset(source, 1),
		OldEndPoint: pointAtOffset(source, 1),
		NewEndPoint: pointAtOffset(edited, 2),
	}
	oldTree.Edit(edit)
	incTree, profile, err := editParser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("%s: incremental parse: %v", w.name, err)
	}
	t.Cleanup(incTree.Release)

	if profile.ReuseUnsupported != w.wantIncrementalReuseUnsupported {
		t.Fatalf("%s: incremental ReuseUnsupported=%t, want %t (reason=%q)", w.name, profile.ReuseUnsupported, w.wantIncrementalReuseUnsupported, profile.ReuseUnsupportedReason)
	}
	if w.wantIncrementalReuseUnsupported && profile.ReuseUnsupportedReason == "" {
		t.Fatalf("%s: incremental ReuseUnsupported=true but ReuseUnsupportedReason is empty", w.name)
	}
	t.Logf("%s: incremental ReuseUnsupported=%t reason=%q reusedSubtrees=%d reusedBytes=%d",
		w.name, profile.ReuseUnsupported, profile.ReuseUnsupportedReason, profile.ReusedSubtrees, profile.ReusedBytes)

	freshEditedParser := gotreesitter.NewParser(goLanguage)
	freshEditedParser.SetAdmissionCandidateRoute(false)
	freshEditedTree, err := freshEditedParser.Parse(edited)
	if err != nil {
		t.Fatalf("%s: fresh Go parse of edited source: %v", w.name, err)
	}
	t.Cleanup(freshEditedTree.Release)
	freshEditedRoot := freshEditedTree.RootNode()

	cEditedParser := sitter.NewParser()
	t.Cleanup(cEditedParser.Close)
	if err := cEditedParser.SetLanguage(cLanguage); err != nil {
		t.Fatalf("%s: set locked Scala C language for edited parse: %v", w.name, err)
	}
	cEditedTree := cEditedParser.Parse(edited, nil)
	if cEditedTree == nil || cEditedTree.RootNode() == nil {
		t.Fatalf("%s: locked C parser returned no root for edited source", w.name)
	}
	t.Cleanup(cEditedTree.Close)
	cEditedRoot := cEditedTree.RootNode()

	cEditedMissing := findMissingCNodes(cEditedRoot)
	if len(cEditedMissing) != 1 || cEditedMissing[0].kind != w.wantMissingKind || cEditedMissing[0].byteOffset != w.wantEditedMissingByte {
		t.Fatalf("%s: locked C edited missing leaves=%+v, want exactly one %q at byte %d", w.name, cEditedMissing, w.wantMissingKind, w.wantEditedMissingByte)
	}

	incRoot := incTree.RootNode()
	if diff := FirstDivergenceDumpV1(incRoot, goLanguage, cEditedRoot); diff != nil {
		t.Fatalf("%s: incremental tree diverges from locked C fresh-edited parse: %+v", w.name, *diff)
	}
	if diff := firstLockedCTreeFlagDivergence(incRoot, goLanguage, cEditedRoot, "/"+incRoot.Type(goLanguage)); diff != nil {
		t.Fatalf("%s: incremental flags diverge from locked C fresh-edited parse: %v", w.name, diff)
	}
	if diff := FirstDivergenceDumpV1(freshEditedRoot, goLanguage, cEditedRoot); diff != nil {
		t.Fatalf("%s: fresh Go edited parse diverges from locked C fresh-edited parse: %+v", w.name, *diff)
	}
	if diff := firstLockedCTreeFlagDivergence(freshEditedRoot, goLanguage, cEditedRoot, "/"+freshEditedRoot.Type(goLanguage)); diff != nil {
		t.Fatalf("%s: fresh Go edited parse flags diverge from locked C fresh-edited parse: %v", w.name, diff)
	}

	cEditedDigest, err := COracleDeepDigest(cEditedTree)
	if err != nil {
		t.Fatalf("%s: inspect locked C edited tree: %v", w.name, err)
	}
	incInspection, err := benchfixtures.InspectGoTree(incRoot, goLanguage)
	if err != nil {
		t.Fatalf("%s: inspect incremental Go tree: %v", w.name, err)
	}
	if incInspection.SHA256 != cEditedDigest {
		t.Fatalf("%s: incremental deep digest=%s, want locked C edited %s", w.name, incInspection.SHA256, cEditedDigest)
	}
	freshEditedInspection, err := benchfixtures.InspectGoTree(freshEditedRoot, goLanguage)
	if err != nil {
		t.Fatalf("%s: inspect fresh edited Go tree: %v", w.name, err)
	}
	if freshEditedInspection.SHA256 != cEditedDigest {
		t.Fatalf("%s: fresh edited deep digest=%s, want locked C edited %s", w.name, freshEditedInspection.SHA256, cEditedDigest)
	}
	t.Logf("%s: incremental edit at byte 1 == fresh Go parse == locked C fresh parse of edited source (digest %s)", w.name, cEditedDigest)
}

// package2MissingLeaf records one missing-leaf node found during a tree
// walk: its symbol kind and byte offset.
type package2MissingLeaf struct {
	kind       string
	byteOffset uint32
}

func findMissingCNodes(n *sitter.Node) []package2MissingLeaf {
	var out []package2MissingLeaf
	var walk func(node *sitter.Node)
	walk = func(node *sitter.Node) {
		if node == nil {
			return
		}
		if node.IsMissing() {
			out = append(out, package2MissingLeaf{kind: node.Kind(), byteOffset: uint32(node.StartByte())})
		}
		count := node.ChildCount()
		for i := uint(0); i < count; i++ {
			walk(node.Child(i))
		}
	}
	walk(n)
	return out
}

func findMissingGoNodes(n *gotreesitter.Node, lang *gotreesitter.Language) []package2MissingLeaf {
	var out []package2MissingLeaf
	var walk func(node *gotreesitter.Node)
	walk = func(node *gotreesitter.Node) {
		if node == nil {
			return
		}
		if node.IsMissing() {
			out = append(out, package2MissingLeaf{kind: node.Type(lang), byteOffset: node.StartByte()})
		}
		for i := 0; i < node.ChildCount(); i++ {
			walk(node.Child(i))
		}
	}
	walk(n)
	return out
}

// findErrorCNode returns the path of the first ERROR-kind node found in the
// tree, or "" if none exists.
func findErrorCNode(n *sitter.Node, path string) string {
	if n == nil {
		return ""
	}
	if n.IsError() {
		return path
	}
	count := n.ChildCount()
	for i := uint(0); i < count; i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		if found := findErrorCNode(child, fmt.Sprintf("%s/%s[%d]", path, child.Kind(), i)); found != "" {
			return found
		}
	}
	return ""
}
