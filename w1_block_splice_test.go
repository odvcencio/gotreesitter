package gotreesitter_test

// These tests explicitly select the legacy engine because BlockSpliceSteps measures its W1 loop.
// The compact incremental execution tests verify compact reuse separately.

// Campaign O(edit) workstream W1 block-splice composition (spec.campaign.oedit):
// after the edited item settles, the whole run of following unchanged top-level
// siblings is spliced inside one parse-loop iteration instead of one iteration
// per sibling. These tests are real-parse and enforce the campaign invariants a
// reviewer must be able to trust for the block path:
//
//   - Oracle equality: the incremental tree is structurally identical to a
//     fresh parse of the same final text (TestW1BlockSpliceOracleEquality).
//   - The block path actually fires and its step count is O(edit): it scales
//     with the reused run, and rootNonLeafChanged stays a small constant
//     (TestW1BlockSpliceFiresAndIsBounded).
//   - Determinism: two independent fresh Parsers on the same (source, edit)
//     produce identical trees AND identical BlockSpliceSteps counters
//     (TestW1BlockSpliceIsDeterministic). The loop reads only the live single
//     stack and the old tree, never per-instance history.

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func w1BlockCleanGo(n int) []byte {
	var b strings.Builder
	b.WriteString("package p\n\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "func B%d(a int) int {\n\tx := a + %d\n\treturn x\n}\n\n", i, i)
	}
	return []byte(b.String())
}

// TestW1BlockSpliceOracleEquality is the non-negotiable oracle: a near-top
// single-byte insert into a clean Go file of many simple top-level functions
// must produce a tree structurally identical to a fresh parse, even though the
// trailing run is spliced back as a block.
func TestW1BlockSpliceOracleEquality(t *testing.T) {
	lang := grammars.GoLanguage()
	src := w1BlockCleanGo(400)
	off := bytes.Index(src, []byte("func B0(a")) + len("func B0(")
	fresh, incr, profile, edited := w1bParseProfiledWithParser(t, lang, src, off, w1BlockLegacyParser)

	if incr.HasError() {
		t.Fatal("incremental tree unexpectedly has errors")
	}
	if int(incr.EndByte()) != len(edited) {
		t.Fatalf("incremental root span %d does not cover edited buffer %d", incr.EndByte(), len(edited))
	}
	if d := w1bStructuralDiff(lang, fresh, incr, "root"); d != "" {
		t.Fatalf("oracle divergence (incremental != fresh) with block splice: %s", d)
	}
	if profile.BlockSpliceSteps == 0 {
		t.Fatal("W1 block splice did not fire on a long clean trailing run")
	}
}

// TestW1BlockSpliceFiresAndIsBounded proves the block path scales with the
// reused run (it is the whole-run splice, not a one-off) while
// ReuseRejectRootNonLeafChanged stays a small constant (O(edit), not O(file)).
func TestW1BlockSpliceFiresAndIsBounded(t *testing.T) {
	lang := grammars.GoLanguage()
	small := w1BlockCleanGo(150)
	large := w1BlockCleanGo(600)
	offS := bytes.Index(small, []byte("func B0(a")) + len("func B0(")
	offL := bytes.Index(large, []byte("func B0(a")) + len("func B0(")

	_, incrS, profS, editedS := w1bParseProfiledWithParser(t, lang, small, offS, w1BlockLegacyParser)
	_, incrL, profL, editedL := w1bParseProfiledWithParser(t, lang, large, offL, w1BlockLegacyParser)

	if incrS.HasError() || incrL.HasError() {
		t.Fatal("incremental tree unexpectedly has errors")
	}
	if int(incrS.EndByte()) != len(editedS) || int(incrL.EndByte()) != len(editedL) {
		t.Fatal("incremental root span does not cover the edited buffer")
	}
	if profS.BlockSpliceSteps == 0 || profL.BlockSpliceSteps == 0 {
		t.Fatalf("block splice did not fire (small=%d large=%d)", profS.BlockSpliceSteps, profL.BlockSpliceSteps)
	}
	// The block spliced the whole trailing run: 4x the siblings means far more
	// block steps, but the rejection counter stays flat (a small constant).
	if profL.BlockSpliceSteps <= profS.BlockSpliceSteps {
		t.Fatalf("block splice did not scale with the reused run (small=%d large=%d)",
			profS.BlockSpliceSteps, profL.BlockSpliceSteps)
	}
	if profL.ReuseRejectRootNonLeafChanged > profS.ReuseRejectRootNonLeafChanged+16 {
		t.Fatalf("W1 regression: ReuseRejectRootNonLeafChanged grew with file size (small=%d large=%d) -- reuse is O(file), not O(edit)",
			profS.ReuseRejectRootNonLeafChanged, profL.ReuseRejectRootNonLeafChanged)
	}
}

// TestW1BlockSpliceIsDeterministic runs the identical (source, edit) on two
// independent fresh Parsers and requires structurally identical trees AND
// identical BlockSpliceSteps counters -- the campaign determinism invariant.
func TestW1BlockSpliceIsDeterministic(t *testing.T) {
	lang := grammars.GoLanguage()
	src := w1BlockCleanGo(250)
	off := bytes.Index(src, []byte("func B0(a")) + len("func B0(")
	edited, edit := w1bInsertByte(src, off)

	run := func() (*gts.Node, uint64) {
		p := w1BlockLegacyParser(lang)
		old, err := p.Parse(src)
		if err != nil {
			t.Fatalf("base parse: %v", err)
		}
		old.Edit(edit)
		incr, prof, err := p.ParseIncrementalProfiled(edited, old)
		if err != nil {
			t.Fatalf("incremental parse: %v", err)
		}
		t.Cleanup(func() {
			incr.Release()
			old.Release()
		})
		return incr.RootNode(), prof.BlockSpliceSteps
	}

	aNode, aSteps := run()
	bNode, bSteps := run()
	if d := w1bStructuralDiff(lang, aNode, bNode, "root"); d != "" {
		t.Fatalf("W1 block-splice non-determinism: two fresh Parsers produced different trees: %s", d)
	}
	if aSteps != bSteps {
		t.Fatalf("W1 block-splice non-determinism: BlockSpliceSteps differ across runs (%d != %d)", aSteps, bSteps)
	}
	if aSteps == 0 {
		t.Fatal("block splice did not fire")
	}
}

// TestW1BlockSpliceConfinesUntypedParamAmbiguity guards the boundary the W4
// lane (PR #407) flagged: a byte delete that turns Go's `func h(a, b int)` into
// the untyped-parameter-list-ambiguous `func h(a, int)` makes the incremental
// parse of the EDITED function diverge from a fresh parse (a pre-existing bug,
// tracked separately -- NOT fixed here). This test proves the W1 block splice
// does not WIDEN that hole: the edited function is dirty, so the per-member
// gates (byte-range equality, fragility, has-error) bar it from block reuse, and
// every OTHER top-level sibling -- the clean run the block splices back -- stays
// structurally identical to a fresh parse. The divergence, if present, is
// confined to the edited function. If a future change let the block sweep past
// this construct, a trailing sibling would diverge and this test fails.
func TestW1BlockSpliceConfinesUntypedParamAmbiguity(t *testing.T) {
	lang := grammars.GoLanguage()
	var b strings.Builder
	b.WriteString("package p\n\n")
	b.WriteString("func A0(x int) int {\n\treturn x\n}\n\n")
	b.WriteString("func h(a, b int) int {\n\treturn a\n}\n\n")
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&b, "func C%d(x int) int {\n\treturn x\n}\n\n", i)
	}
	src := []byte(b.String())

	mi := bytes.Index(src, []byte("func h(a, b int)"))
	if mi < 0 {
		t.Fatal("fixture invariant: marker not found")
	}
	// Delete the "b " (2 bytes) inside h so its parameter list becomes the
	// ambiguous `(a, int)`.
	bOff := mi + bytes.Index(src[mi:], []byte("b int"))
	edited := make([]byte, 0, len(src))
	edited = append(edited, src[:bOff]...)
	edited = append(edited, src[bOff+2:]...)
	edit := gts.InputEdit{
		StartByte: uint32(bOff), OldEndByte: uint32(bOff + 2), NewEndByte: uint32(bOff),
		StartPoint: w1bPointAt(src, bOff), OldEndPoint: w1bPointAt(src, bOff+2), NewEndPoint: w1bPointAt(src, bOff),
	}

	parser := w1BlockLegacyParser(lang)
	old, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("base parse: %v", err)
	}
	old.Edit(edit)
	incr, prof, err := parser.ParseIncrementalProfiled(edited, old)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	freshTree, err := w1BlockLegacyParser(lang).Parse(edited)
	if err != nil {
		t.Fatalf("fresh parse: %v", err)
	}
	t.Cleanup(func() {
		incr.Release()
		old.Release()
		freshTree.Release()
	})

	fresh, incrRoot := freshTree.RootNode(), incr.RootNode()
	if fresh.ChildCount() != incrRoot.ChildCount() {
		t.Fatalf("block splice widened the hole: top-level child count %d (incr) != %d (fresh)",
			incrRoot.ChildCount(), fresh.ChildCount())
	}
	if prof.BlockSpliceSteps == 0 {
		t.Fatal("block splice did not fire on the trailing clean run")
	}
	// Every top-level sibling EXCEPT the edited function h must match fresh.
	editedFn := "func h("
	for i := 0; i < fresh.ChildCount(); i++ {
		fc, ic := fresh.Child(i), incrRoot.Child(i)
		if fc == nil || ic == nil {
			continue
		}
		isEdited := int(fc.StartByte()) <= bOff && bOff < int(fc.EndByte()) &&
			bytes.HasPrefix(edited[fc.StartByte():], []byte(editedFn))
		d := w1bStructuralDiff(lang, fc, ic, "child")
		if d != "" && !isEdited {
			t.Fatalf("block splice widened the untyped-param hole into a trailing sibling [%d %s @%d-%d]: %s",
				i, fc.Type(lang), fc.StartByte(), fc.EndByte(), d)
		}
	}
}

func w1BlockLegacyParser(lang *gts.Language) *gts.Parser {
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	return parser
}
