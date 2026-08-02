package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestPowerShellBacktickContinuationIsParserPadding pins the repair for the
// PR #633 regression (materializeSkippedGapAsExtraError minting a spurious
// EXTRA ERROR leaf over PowerShell's backtick line continuation): a
// command_argument_sep that contains a backtick immediately followed by a
// newline must keep its C-sided two-child shape (the two surrounding
// whitespace runs) rather than gain a third, ERROR child.
//
// The source mirrors the exact shape found at byte 51746 and byte 51784 of
// cgo_harness/corpus_real/powershell/large__packaging.psm1 (bisect
// spore.2026-08-02.birch-g.powershell-bisect, "Sample 1"/"Sample 2" C
// verdicts): a bare command name, then a parameter continued across a
// backtick-newline line break. C-oracle receipt for this exact snippet
// (locked C reference, tree-sitter-powershell @ da65ba3a, verified directly
// against cgo_harness's ParityCLanguage/compareNodes):
//
//	command_argument_sep [11,18) 2 children: [11,12) [14,18)  (no ERROR)
//	command_argument_sep [29,36) 2 children: [29,30) [32,36)  (no ERROR)
//	root.HasError() == false, zero ERROR nodes in the whole tree
//
// Pre-fix (main HEAD before this change), production Go instead produced 3
// children at each site -- (" ", ERROR, " ") -- with a 2-byte EXTRA ERROR
// leaf spanning exactly the backtick+newline, diverging from C.
func TestPowerShellBacktickContinuationIsParserPadding(t *testing.T) {
	src := []byte("New-RpmSpec `\n    -Name $Name `\n    -Version $Version\n")
	lang := grammars.PowershellLanguage()
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("PowerShell parse error: %v", err)
	}
	defer tree.Release()
	root := requireCompleteParse(t, tree, src, lang, "PowerShell backtick continuation")

	if root.HasError() {
		t.Fatalf("root.HasError() = true, want false; tree=%s", root.SExpr(lang))
	}
	if errs := collectNodesByType(root, lang, "ERROR"); len(errs) != 0 {
		t.Fatalf("found %d ERROR node(s), want 0: %v; tree=%s", len(errs), errs, root.SExpr(lang))
	}

	seps := collectNodesByType(root, lang, "command_argument_sep")
	type wantSep struct {
		start, end     uint32
		c0Start, c0End uint32
		c1Start, c1End uint32
	}
	wantSeps := []wantSep{
		{start: 11, end: 18, c0Start: 11, c0End: 12, c1Start: 14, c1End: 18},
		{start: 29, end: 36, c0Start: 29, c0End: 30, c1Start: 32, c1End: 36},
	}

	var gapSeps []*gts.Node
	for _, n := range seps {
		if n.ChildCount() > 1 {
			gapSeps = append(gapSeps, n)
		}
	}
	if len(gapSeps) != len(wantSeps) {
		t.Fatalf("multi-child command_argument_sep count = %d, want %d; all seps=%v; tree=%s",
			len(gapSeps), len(wantSeps), describeNodes(seps), root.SExpr(lang))
	}
	for i, n := range gapSeps {
		want := wantSeps[i]
		if n.StartByte() != want.start || n.EndByte() != want.end {
			t.Fatalf("command_argument_sep[%d] span = [%d,%d), want [%d,%d)", i, n.StartByte(), n.EndByte(), want.start, want.end)
		}
		if got := n.ChildCount(); got != 2 {
			t.Fatalf("command_argument_sep[%d] ChildCount = %d, want 2 (C matches this exact shape; #633 minted a third ERROR child here); tree=%s", i, got, root.SExpr(lang))
		}
		c0, c1 := n.Child(0), n.Child(1)
		if c0.IsError() || c1.IsError() {
			t.Fatalf("command_argument_sep[%d] has an ERROR child: c0=%s c1=%s", i, describeNode(c0, lang), describeNode(c1, lang))
		}
		if c0.StartByte() != want.c0Start || c0.EndByte() != want.c0End {
			t.Fatalf("command_argument_sep[%d] child0 span = [%d,%d), want [%d,%d)", i, c0.StartByte(), c0.EndByte(), want.c0Start, want.c0End)
		}
		if c1.StartByte() != want.c1Start || c1.EndByte() != want.c1End {
			t.Fatalf("command_argument_sep[%d] child1 span = [%d,%d), want [%d,%d)", i, c1.StartByte(), c1.EndByte(), want.c1Start, want.c1End)
		}
	}
}

// TestPowerShellLineContinuationEscapeByteIsBacktick pins the declaration
// itself: PowerShell's built-in runtime profile (grammars/runtime_profiles.go)
// must attach exactly the backtick byte, and only for a real loaded
// PowerShell Language (never a zero-value or caller-constructed one, which
// keeps the conservative default of 0 -- no declared continuation escape).
func TestPowerShellLineContinuationEscapeByteIsBacktick(t *testing.T) {
	lang := grammars.PowershellLanguage()
	if got, want := lang.LineContinuationEscapeByte, byte('`'); got != want {
		t.Fatalf("PowershellLanguage().LineContinuationEscapeByte = %q, want %q", got, want)
	}
	if got := (&gts.Language{}).LineContinuationEscapeByte; got != 0 {
		t.Fatalf("zero-value Language.LineContinuationEscapeByte = %q, want 0 (conservative default)", got)
	}
}

// TestPowerShellNonContinuationErrorsStillMaterialize pins the composition
// half of the repair: declaring PowerShell's backtick-newline continuation
// as padding must not disable PowerShell's ordinary error materialization.
// This snippet's error has nothing to do with line continuation (an
// incomplete "$" variable reference); it still must surface as a real ERROR
// with HasError() set, exactly as before the continuation-padding change.
// See TestBytesAreParserPaddingLineContinuationEscapeComposes
// (parser_shift_gap_test.go) for the same composition property pinned
// directly at the bytesAreParserPadding predicate.
func TestPowerShellNonContinuationErrorsStillMaterialize(t *testing.T) {
	src := []byte("New-RpmSpec $ -Name $Name\n")
	lang := grammars.PowershellLanguage()
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("PowerShell parse error: %v", err)
	}
	defer tree.Release()
	root := requireCompleteParse(t, tree, src, lang, "PowerShell non-continuation error control")

	if !root.HasError() {
		t.Fatalf("root.HasError() = false, want true for a genuine syntax error; tree=%s", root.SExpr(lang))
	}
	if errs := collectNodesByType(root, lang, "ERROR"); len(errs) == 0 {
		t.Fatalf("found 0 ERROR nodes, want at least 1 for a genuine syntax error; tree=%s", root.SExpr(lang))
	}
}

func collectNodesByType(n *gts.Node, lang *gts.Language, typ string) []*gts.Node {
	if n == nil {
		return nil
	}
	var out []*gts.Node
	if n.Type(lang) == typ {
		out = append(out, n)
	}
	for i := 0; i < n.ChildCount(); i++ {
		out = append(out, collectNodesByType(n.Child(i), lang, typ)...)
	}
	return out
}

func describeNode(n *gts.Node, lang *gts.Language) string {
	if n == nil {
		return "<nil>"
	}
	return n.Type(lang)
}

func describeNodes(nodes []*gts.Node) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		if n == nil {
			out[i] = "<nil>"
			continue
		}
		out[i] = n.SExpr(nil)
	}
	return out
}
