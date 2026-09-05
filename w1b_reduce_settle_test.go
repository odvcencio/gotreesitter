package gotreesitter_test

// Campaign O(edit) workstream W1b (spec.campaign.oedit): settle the pending
// eager-default (unconditional) reduce chain before the incremental reuse
// check, so Go (and similar languages) can take the non-leaf top-level sibling
// splice that W1 unlocked. tryReuseSubtree runs before the dispatch loop's
// reduce chain, so the live top-of-stack state was a short chain of pending
// unconditional reduces "behind" the state the old tree recorded as a
// candidate's PreGotoState; every trailing non-leaf sibling was therefore
// rejected with ReuseRejectRootNonLeafChanged. Settling closes that gap.
//
// These tests are real-parse and enforce the campaign's non-negotiable oracle
// invariant: the incremental tree must be structurally identical to a fresh
// parse of the same final text. They also prove the two behaviors a reviewer
// must be able to trust:
//
//   - Settling drives ReuseRejectRootNonLeafChanged from O(file) to a small
//     bound and lifts non-leaf reuse for Go's top-level declaration list
//     (TestW1BSettleUnlocksGoTopLevelSiblingSplice, and its scaling control
//     TestW1BSettleReuseIsBoundedNotFileSized).
//   - Settling is scoped by hasNonLeafCandidateAt so it can never perturb an
//     intra-item parse trajectory -- in particular a trailing comment/extra
//     between top-level items keeps the exact attachment a fresh parse gives
//     (TestW1BSettlePreservesTrailingCommentAttachment). Without that scope the
//     invariant gate fails on JavaScript's `}\n/** ... */\nfunction` boundary;
//     Go's `}\n// ...\nfunc` is the same hazard class.

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// w1bStructuralDiff walks fresh and incr in lockstep and returns the first
// structural difference (type, span, named, missing, childCount), or "" when
// the two subtrees are identical. This is the same oracle contract the
// incremental invariant gate enforces, scoped to one edit site.
func w1bStructuralDiff(lang *gts.Language, fresh, incr *gts.Node, path string) string {
	if fresh == nil || incr == nil {
		if fresh == incr {
			return ""
		}
		return fmt.Sprintf("%s: nil mismatch fresh=%v incr=%v", path, fresh != nil, incr != nil)
	}
	ft, it := fresh.Type(lang), incr.Type(lang)
	if ft != it {
		return fmt.Sprintf("%s: type %s != %s", path, ft, it)
	}
	cur := path + ">" + ft
	if fresh.StartByte() != incr.StartByte() || fresh.EndByte() != incr.EndByte() {
		return fmt.Sprintf("%s: span [%d,%d) != [%d,%d)", cur, fresh.StartByte(), fresh.EndByte(), incr.StartByte(), incr.EndByte())
	}
	if fresh.IsNamed() != incr.IsNamed() {
		return fmt.Sprintf("%s: named %v != %v", cur, fresh.IsNamed(), incr.IsNamed())
	}
	if fresh.IsMissing() != incr.IsMissing() {
		return fmt.Sprintf("%s: missing %v != %v", cur, fresh.IsMissing(), incr.IsMissing())
	}
	if fresh.ChildCount() != incr.ChildCount() {
		return fmt.Sprintf("%s: childCount %d != %d", cur, fresh.ChildCount(), incr.ChildCount())
	}
	for i := 0; i < fresh.ChildCount(); i++ {
		if d := w1bStructuralDiff(lang, fresh.Child(i), incr.Child(i), cur); d != "" {
			return d
		}
	}
	return ""
}

func w1bPointAt(src []byte, off int) gts.Point {
	row, col := 0, 0
	for i := 0; i < off && i < len(src); i++ {
		if src[i] == '\n' {
			row++
			col = 0
		} else {
			col++
		}
	}
	return gts.Point{Row: uint32(row), Column: uint32(col)}
}

// w1bInsertByte builds the edited buffer and InputEdit for a single-byte insert
// (duplicating the byte at off), the same near-top keystroke the #380 repro
// uses.
func w1bInsertByte(src []byte, off int) ([]byte, gts.InputEdit) {
	edited := make([]byte, 0, len(src)+1)
	edited = append(edited, src[:off]...)
	edited = append(edited, src[off])
	edited = append(edited, src[off:]...)
	p := w1bPointAt(src, off)
	return edited, gts.InputEdit{
		StartByte: uint32(off), OldEndByte: uint32(off), NewEndByte: uint32(off + 1),
		StartPoint: p, OldEndPoint: p, NewEndPoint: w1bPointAt(edited, off+1),
	}
}

// w1bCleanGo builds a valid Go file of unambiguous, NON-fragile single-typed-
// parameter functions, so the only blocker to non-leaf top-level sibling
// splice is the pending-reduce settling gap W1b closes (no fragility, no
// comments).
func w1bCleanGo(n int) []byte {
	var b strings.Builder
	b.WriteString("package p\n\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "func G%d(a int) int {\n\tx := a + %d\n\treturn x\n}\n\n", i, i)
	}
	return []byte(b.String())
}

func w1bParseProfiled(t *testing.T, lang *gts.Language, src []byte, off int) (fresh, incr *gts.Node, profile gts.IncrementalParseProfile, edited []byte) {
	return w1bParseProfiledWithParser(t, lang, src, off, gts.NewParser)
}

func w1bParseProfiledWithParser(t *testing.T, lang *gts.Language, src []byte, off int, newParser func(*gts.Language) *gts.Parser) (fresh, incr *gts.Node, profile gts.IncrementalParseProfile, edited []byte) {
	t.Helper()
	parser := newParser(lang)
	oldTree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("base parse: %v", err)
	}
	if oldTree.RootNode().HasError() {
		t.Fatal("fixture invariant: base parse must be clean")
	}

	edited, edit := w1bInsertByte(src, off)

	freshParser := newParser(lang)
	freshTree, err := freshParser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh parse of edited: %v", err)
	}

	oldTree.Edit(edit)
	incrTree, prof, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	t.Cleanup(func() {
		freshTree.Release()
		incrTree.Release()
		oldTree.Release()
	})
	return freshTree.RootNode(), incrTree.RootNode(), prof, edited
}

// TestW1BSettleUnlocksGoTopLevelSiblingSplice is the core W1b proof: a near-top
// single-char insert into a clean Go file of many simple top-level functions
// must (1) produce a tree byte-identical to a fresh parse, and (2) actually
// reuse the trailing sibling functions as non-leaf subtrees -- driving
// ReuseRejectRootNonLeafChanged to a small bound rather than one-per-trailing-
// sibling. Before W1b the whole trailing run was rejected (RootNonLeafChanged
// scaled with the file); the settling makes the splice fire.
func TestW1BSettleUnlocksGoTopLevelSiblingSplice(t *testing.T) {
	lang := grammars.GoLanguage()
	src := w1bCleanGo(300)
	// Insert inside G0's parameter name, well past the package clause, so a
	// long run of trailing unchanged siblings exists.
	off := bytes.Index(src, []byte("func G0(a")) + len("func G0(")
	fresh, incr, profile, edited := w1bParseProfiled(t, lang, src, off)

	if incr.HasError() {
		t.Fatal("incremental tree unexpectedly has errors")
	}
	if int(incr.EndByte()) != len(edited) {
		t.Fatalf("incremental root span %d does not cover edited buffer %d", incr.EndByte(), len(edited))
	}
	if d := w1bStructuralDiff(lang, fresh, incr, "root"); d != "" {
		t.Fatalf("oracle divergence (incremental != fresh): %s", d)
	}

	if profile.ReusedSubtrees == 0 {
		t.Fatal("W1b regression: no subtrees reused -- the top-level sibling splice did not fire")
	}
	// The edited item plus a small constant of boundary bookkeeping is the only
	// place non-leaf reuse can be legitimately refused; it must NOT scale with
	// the 300 trailing siblings. A generous bound guards the O(edit) property
	// without being brittle.
	if profile.ReuseRejectRootNonLeafChanged > 64 {
		t.Fatalf("W1b regression: ReuseRejectRootNonLeafChanged=%d is not bounded (settling gap re-opened; expected a small constant, not one per trailing sibling)",
			profile.ReuseRejectRootNonLeafChanged)
	}
}

// TestW1BSettleReuseIsBoundedNotFileSized is the scaling control: doubling the
// number of trailing siblings must NOT roughly double ReuseRejectRootNonLeaf
// Changed. A settling regression re-couples that counter to file size; this
// asserts it stays flat (O(edit), not O(file)).
func TestW1BSettleReuseIsBoundedNotFileSized(t *testing.T) {
	lang := grammars.GoLanguage()
	small := w1bCleanGo(150)
	large := w1bCleanGo(600)
	offS := bytes.Index(small, []byte("func G0(a")) + len("func G0(")
	offL := bytes.Index(large, []byte("func G0(a")) + len("func G0(")

	_, incrS, profS, editedS := w1bParseProfiled(t, lang, small, offS)
	_, incrL, profL, editedL := w1bParseProfiled(t, lang, large, offL)

	if incrS.HasError() || incrL.HasError() {
		t.Fatal("incremental tree unexpectedly has errors")
	}
	if int(incrS.EndByte()) != len(editedS) || int(incrL.EndByte()) != len(editedL) {
		t.Fatal("incremental root span does not cover the edited buffer")
	}

	// 4x the siblings must not produce anywhere near 4x the rejections. Allow a
	// small absolute slack so a flat-plus-noise counter passes.
	if profL.ReuseRejectRootNonLeafChanged > profS.ReuseRejectRootNonLeafChanged+16 {
		t.Fatalf("W1b regression: ReuseRejectRootNonLeafChanged grew with file size (small=%d large=%d) -- reuse is O(file), not O(edit)",
			profS.ReuseRejectRootNonLeafChanged, profL.ReuseRejectRootNonLeafChanged)
	}
}

// TestW1BSettlePreservesTrailingCommentAttachment guards the hasNonLeafCandidate
// At scope on the settling. A comment between two top-level functions is an
// `extra` whose attachment (trailing child of the preceding item vs. sibling of
// the parent) depends on the exact reduce/shift interleaving. Settling
// indiscriminately would absorb such a comment into the preceding function on
// an edit inside the comment; scoping settling to positions that actually have
// a non-leaf sibling candidate prevents that. The edit here lands inside the
// comment, and the incremental tree must still match a fresh parse exactly.
func TestW1BSettlePreservesTrailingCommentAttachment(t *testing.T) {
	lang := grammars.GoLanguage()
	var b strings.Builder
	b.WriteString("package p\n\n")
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "func H%d(a int) int {\n\treturn a\n}\n\n// doc comment number %d describing the following function\nfunc J%d(a int) int {\n\treturn a\n}\n\n", i, i, i)
	}
	src := []byte(b.String())

	// Edit inside one of the doc comments, well past the top.
	marker := []byte("// doc comment number 5 ")
	ci := bytes.Index(src, marker)
	if ci < 0 {
		t.Fatal("fixture invariant: comment marker not found")
	}
	off := ci + len("// doc comment ") // inside the comment word "number"

	fresh, incr, _, edited := w1bParseProfiled(t, lang, src, off)
	if int(incr.EndByte()) != len(edited) {
		t.Fatalf("incremental root span %d does not cover edited buffer %d", incr.EndByte(), len(edited))
	}
	if d := w1bStructuralDiff(lang, fresh, incr, "root"); d != "" {
		t.Fatalf("oracle divergence on trailing-comment boundary (settling perturbed extra attachment): %s", d)
	}
}

// TestW1BSettleIsDeterministicAcrossFreshParsers runs the identical
// (source, edit) on two independent, freshly-constructed Parser instances and
// requires structurally identical incremental trees. The settling reads only
// p.eagerDefaultReduces (built once at construction) and live stack state, so
// no behavior may depend on a Parser instance's unrelated history -- the
// campaign's determinism invariant. A regression that made settling consult
// mutable per-instance state would diverge here.
func TestW1BSettleIsDeterministicAcrossFreshParsers(t *testing.T) {
	lang := grammars.GoLanguage()
	src := w1bCleanGo(200)
	off := bytes.Index(src, []byte("func G0(a")) + len("func G0(")
	edited, edit := w1bInsertByte(src, off)

	run := func() *gts.Node {
		p := gts.NewParser(lang)
		old, err := p.Parse(src)
		if err != nil {
			t.Fatalf("base parse: %v", err)
		}
		old.Edit(edit)
		incr, err := p.ParseIncremental(edited, old)
		if err != nil {
			t.Fatalf("incremental parse: %v", err)
		}
		t.Cleanup(func() {
			incr.Release()
			old.Release()
		})
		return incr.RootNode()
	}

	a := run()
	b := run()
	if d := w1bStructuralDiff(lang, a, b, "root"); d != "" {
		t.Fatalf("W1b non-determinism: two fresh Parsers produced different trees for the same edit: %s", d)
	}
}
