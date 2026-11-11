package gotreesitter_test

// Campaign post-admission-frontier T2a (spec.campaign.post-admission-frontier):
// the LEADING-run block-splice. A mid-file or bottom edit no longer pays
// O(preceding top-level items): the whole run of byte-identical top-level items
// BEFORE the edit is spliced back as a block from the initial parser state, the
// mirror of the trailing block-splice (roottest/parse/w1_block_splice_test.go). These tests are
// real-parse and enforce the campaign invariants for the leading path:
//
//   - Oracle equality: the incremental tree is structurally identical to a fresh
//     parse of the same final text, for edits at the top, middle, and bottom of
//     the file, and for the boundary cases (item 0, comments between items,
//     extras before the first item, an edit in the last item with no trailing
//     run).
//   - The leading splice fires and is O(edit): ReuseRejectRootNonLeafChanged
//     stays a small size-independent constant for a MIDDLE edit, not O(leading
//     item count) as it was before T2a.
//   - Determinism: two independent fresh Parsers on the same (source, edit)
//     produce identical trees and identical counters.
//   - The untyped-parameter ambiguity divergence (tracked, pre-existing) is not
//     widened: a clean untyped-param function sitting in the spliced leading run
//     stays fresh-identical.

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// leadingParseAt runs a single-byte edit of the given class at off and returns
// the fresh root, incremental root, profile, and edited buffer. The base parse
// must be clean.
func leadingParseAt(t *testing.T, lang *gts.Language, src []byte, off int, class string) (fresh, incr *gts.Node, prof gts.IncrementalParseProfile, edited []byte) {
	t.Helper()
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	oldTree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("base parse: %v", err)
	}
	if oldTree.RootNode().HasError() {
		t.Fatal("fixture invariant: base parse must be clean")
	}
	edited, edit := incrGateBuildEdit(src, off, class)

	freshParser := gts.NewParser(lang)
	freshParser.SetAdmissionCandidateRoute(false)
	freshTree, err := freshParser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh parse of edited: %v", err)
	}
	if e, m := incrGateTreeStats(freshTree.RootNode()); e != 0 || m != 0 {
		t.Fatalf("fixture invariant: fresh parse of edited must be clean (err=%d miss=%d)", e, m)
	}

	oldTree.Edit(edit)
	incrTree, p, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	t.Cleanup(func() {
		freshTree.Release()
		incrTree.Release()
		oldTree.Release()
	})
	return freshTree.RootNode(), incrTree.RootNode(), p, edited
}

func leadingCleanGo(n int) []byte {
	var b strings.Builder
	b.WriteString("package p\n\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "func G%d(a int) int {\n\tx := a + %d\n\treturn x\n}\n\n", i, i)
	}
	return []byte(b.String())
}

// TestLeadingSpliceOracleEqualityMiddleBottom is the non-negotiable oracle: an
// edit deep in the file (a large leading run precedes it) must produce a tree
// structurally identical to a fresh parse, even though the whole leading run is
// spliced back as a block.
func TestLeadingSpliceOracleEqualityMiddleBottom(t *testing.T) {
	lang := grammars.GoLanguage()
	src := leadingCleanGo(400)
	for _, pos := range []string{"middle", "bottom"} {
		var marker string
		switch pos {
		case "middle":
			marker = "func G200("
		case "bottom":
			marker = "func G396("
		}
		off := bytes.Index(src, []byte(marker)) + len("func G200(")
		if off < len("func G200(") {
			t.Fatalf("marker %q not found", marker)
		}
		fresh, incr, prof, edited := leadingParseAt(t, lang, src, off, "insert")
		if incr.HasError() {
			t.Fatalf("%s: incremental tree unexpectedly has errors", pos)
		}
		if int(incr.EndByte()) != len(edited) {
			t.Fatalf("%s: incremental root span %d does not cover edited buffer %d", pos, incr.EndByte(), len(edited))
		}
		if d := w1bStructuralDiff(lang, fresh, incr, "root"); d != "" {
			t.Fatalf("%s: oracle divergence (incremental != fresh) with leading splice: %s", pos, d)
		}
		if prof.BlockSpliceSteps == 0 {
			t.Fatalf("%s: leading splice did not fire (no block splice steps)", pos)
		}
	}
}

// TestLeadingSpliceIsBoundedNotFileSized proves the T2a win: for a MIDDLE edit,
// ReuseRejectRootNonLeafChanged stays a small constant as the leading item count
// grows 4x, instead of scaling with it (the pre-T2a O(preceding items) cost).
func TestLeadingSpliceIsBoundedNotFileSized(t *testing.T) {
	lang := grammars.GoLanguage()
	small := leadingCleanGo(150)
	large := leadingCleanGo(600)
	// Edit the middle item of each file, so a large leading run precedes it.
	offS := bytes.Index(small, []byte("func G75(")) + len("func G75(")
	offL := bytes.Index(large, []byte("func G300(")) + len("func G300(")

	_, incrS, profS, editedS := leadingParseAt(t, lang, small, offS, "insert")
	_, incrL, profL, editedL := leadingParseAt(t, lang, large, offL, "insert")

	if incrS.HasError() || incrL.HasError() {
		t.Fatal("incremental tree unexpectedly has errors")
	}
	if int(incrS.EndByte()) != len(editedS) || int(incrL.EndByte()) != len(editedL) {
		t.Fatal("incremental root span does not cover the edited buffer")
	}
	if profS.BlockSpliceSteps == 0 || profL.BlockSpliceSteps == 0 {
		t.Fatalf("leading splice did not fire (small=%d large=%d)", profS.BlockSpliceSteps, profL.BlockSpliceSteps)
	}
	// The counter must stay a small constant even though the leading run
	// quadrupled. Before T2a it scaled ~1:1 with the leading item count
	// (thousands at this size). A generous constant bound with headroom.
	const bound = 64
	if profS.ReuseRejectRootNonLeafChanged > bound || profL.ReuseRejectRootNonLeafChanged > bound {
		t.Fatalf("leading reuse is O(preceding items), not O(edit): rootNonLeafChanged small=%d large=%d (bound %d)",
			profS.ReuseRejectRootNonLeafChanged, profL.ReuseRejectRootNonLeafChanged, bound)
	}
	// And it must not grow materially with the 4x-larger leading run.
	if profL.ReuseRejectRootNonLeafChanged > profS.ReuseRejectRootNonLeafChanged+16 {
		t.Fatalf("leading reuse counter grew with file size (small=%d large=%d) -- O(file), not O(edit)",
			profS.ReuseRejectRootNonLeafChanged, profL.ReuseRejectRootNonLeafChanged)
	}
}

// TestLeadingSpliceIsDeterministic runs the identical (source, edit) on two
// independent fresh Parsers and requires identical trees AND identical counters.
func TestLeadingSpliceIsDeterministic(t *testing.T) {
	lang := grammars.GoLanguage()
	src := leadingCleanGo(250)
	off := bytes.Index(src, []byte("func G125(")) + len("func G125(")

	run := func() (*gts.Node, uint64, uint64) {
		fresh, incr, prof, _ := leadingParseAt(t, lang, src, off, "insert")
		_ = fresh
		return incr, prof.BlockSpliceSteps, prof.ReuseRejectRootNonLeafChanged
	}
	aNode, aSteps, aRej := run()
	bNode, bSteps, bRej := run()
	if d := w1bStructuralDiff(lang, aNode, bNode, "root"); d != "" {
		t.Fatalf("leading splice non-determinism: two fresh Parsers produced different trees: %s", d)
	}
	if aSteps != bSteps || aRej != bRej {
		t.Fatalf("leading splice non-determinism: counters differ (steps %d!=%d, rej %d!=%d)", aSteps, bSteps, aRej, bRej)
	}
	if aSteps == 0 {
		t.Fatal("leading splice did not fire")
	}
}

// TestLeadingSpliceBoundaryCases covers the boundary cases the leading splice
// must handle: an edit into item 0 (no leading run), comments between items,
// leading extras before the first item, and an edit in the LAST item (a leading
// run with no trailing run). Every one must stay fresh-identical.
func TestLeadingSpliceBoundaryCases(t *testing.T) {
	goLang := grammars.GoLanguage()

	item0 := leadingCleanGo(60)
	comments := []byte("package p\n\n" +
		"// leading file comment\n" +
		"func A0(a int) int { return a }\n\n" +
		"// between A0 and A1\n" +
		"func A1(a int) int { return a }\n\n" +
		"/* block comment */\n" +
		"func A2(a int) int { return a }\n\n" +
		"func A3(a int) int { return a }\n\n" +
		"func A4(a int) int { return a }\n")

	cases := []struct {
		name   string
		lang   *gts.Language
		src    []byte
		marker string // substring; edit lands just after it
		class  string
	}{
		{"go-item0-insert", goLang, item0, "func G0(", "insert"},
		{"go-item0-delete", goLang, item0, "func G0(a", "delete"},
		{"go-last-item-insert", goLang, item0, "func G59(", "insert"},
		{"go-comments-middle", goLang, comments, "func A2(", "insert"},
		{"go-comments-between-edit", goLang, comments, "func A3(", "delete"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mi := bytes.Index(tc.src, []byte(tc.marker))
			if mi < 0 {
				t.Fatalf("marker %q not found", tc.marker)
			}
			off := mi + len(tc.marker)
			fresh, incr, _, edited := leadingParseAt(t, tc.lang, tc.src, off, tc.class)
			if int(incr.EndByte()) != len(edited) {
				t.Fatalf("incremental root span %d does not cover edited buffer %d", incr.EndByte(), len(edited))
			}
			if d := w1bStructuralDiff(tc.lang, fresh, incr, "root"); d != "" {
				t.Fatalf("oracle divergence (incremental != fresh): %s", d)
			}
		})
	}
}

// TestLeadingSpliceConfinesUntypedParamAmbiguity guards the tracked untyped-
// parameter ambiguity divergence (a Go construct, pre-existing, NOT fixed here).
// A CLEAN untyped-param function sitting in the LEADING run that the block
// splices back must stay structurally identical to fresh -- the leading splice
// reuses it byte-for-byte, never re-deriving the ambiguity. The edit lands in a
// LATER function so the untyped-param function is a spliced leading item.
func TestLeadingSpliceConfinesUntypedParamAmbiguity(t *testing.T) {
	lang := grammars.GoLanguage()
	var b strings.Builder
	b.WriteString("package p\n\n")
	// An untyped-parameter-list function early in the file (the tracked construct).
	b.WriteString("func h(a, b int) int {\n\treturn a\n}\n\n")
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&b, "func C%d(x int) int {\n\treturn x\n}\n\n", i)
	}
	src := []byte(b.String())

	// Edit a function far AFTER h, so h is in the spliced leading run.
	off := bytes.Index(src, []byte("func C60(")) + len("func C60(")
	fresh, incr, prof, edited := leadingParseAt(t, lang, src, off, "insert")

	if int(incr.EndByte()) != len(edited) {
		t.Fatalf("incremental root span %d does not cover edited buffer %d", incr.EndByte(), len(edited))
	}
	if d := w1bStructuralDiff(lang, fresh, incr, "root"); d != "" {
		t.Fatalf("leading splice widened the untyped-param hole: %s", d)
	}
	if prof.BlockSpliceSteps == 0 {
		t.Fatal("leading splice did not fire on the clean leading run")
	}
}

// TestLeadingSpliceCSSProductionRoute proves the production (non-forest) css
// route -- which the campaign's evidence base describes as already having
// top-level sibling reuse -- gets the leading win: a middle-rule edit is
// fresh-identical and splices the leading run.
func TestLeadingSpliceCSSProductionRoute(t *testing.T) {
	lang := grammars.CssLanguage()
	var b strings.Builder
	for i := 0; i < 120; i++ {
		fmt.Fprintf(&b, ".item-%d {\n  color: #112233;\n  margin: %dpx;\n}\n\n", i, i%16)
	}
	src := []byte(b.String())
	off := bytes.Index(src, []byte(".item-60 ")) + len(".item-6")
	fresh, incr, prof, edited := leadingParseAt(t, lang, src, off, "insert")

	if int(incr.EndByte()) != len(edited) {
		t.Fatalf("incremental root span %d does not cover edited buffer %d", incr.EndByte(), len(edited))
	}
	if d := w1bStructuralDiff(lang, fresh, incr, "root"); d != "" {
		t.Fatalf("css leading splice oracle divergence: %s", d)
	}
	if prof.BlockSpliceSteps == 0 {
		t.Fatal("css leading splice did not fire")
	}
	if prof.ReuseRejectRootNonLeafChanged > 32 {
		t.Fatalf("css leading reuse is not O(edit): rootNonLeafChanged=%d", prof.ReuseRejectRootNonLeafChanged)
	}
	// This is a real parser-produced ownership mismatch, not a fabricated Node:
	// CSS still reaches a top-level candidate from a live state different from
	// the candidate's recorded PreGotoState. The existing fragility+goto proof
	// remains the admission contract, so the parse must stay fresh-identical and
	// performant while the mismatch is observable for #432's ownership work.
	if prof.ReuseObservedPreGotoStateMismatch == 0 {
		t.Fatal("expected a real PreGotoState mismatch to be recorded")
	}
}
