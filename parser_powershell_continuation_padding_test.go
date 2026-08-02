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
//
// Runs under both the production route and the compact-candidate route.
// PowerShell's compact route declines this snippet's real-corpus siblings
// (large__packaging.psm1, medium__Install-PowerShellRemoting.ps1 both
// decline per the bisect), so this is primarily a route-equality check: the
// candidate route either serves its own tree (verified equal to production
// by the compact engine's own admission contract) or declines and falls
// back to production transparently -- either way this test's caller-visible
// assertions must hold identically. Route equality for this fix was also
// verified by hand across 7 continuation witnesses.
func TestPowerShellBacktickContinuationIsParserPadding(t *testing.T) {
	src := []byte("New-RpmSpec `\n    -Name $Name `\n    -Version $Version\n")
	lang := grammars.PowershellLanguage()

	for _, route := range []struct {
		name  string
		force bool
	}{
		{"production", false},
		{"compact-candidate", true},
	} {
		t.Run(route.name, func(t *testing.T) {
			parser := gts.NewParser(lang)
			parser.SetAdmissionCandidateRoute(route.force)
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
		})
	}
}

// TestPowerShellTrailingContinuationDoesNotForceDegradedFallback pins a
// second, distinct defect this same repair closes: a trailing backtick
// continuation at (or near) the very end of a file. Before threading
// p.lineContinuationEscapeByte() into parserTailAllowsCleanAcceptance and
// its chain (cleanAcceptedStackSelectableAtEOF, cleanAcceptedTreeLeavesRealTail,
// extendRootToAcceptedCleanTail, recordParseRuntimeRootStats,
// finalizeReturnedTreeRootSpan, retryTreeCoversExpectedEOF,
// parserCoreFreshFullAcceptedTailIsClean), this exact class of input hit a
// two-fix interaction: removing materializeSkippedGapAsExtraError's spurious
// mid-file ERROR (the first half of this repair) also removed the non-zero
// stackResultErrorRank that used to short-circuit
// cleanAcceptedStackSelectableAtEOF before it ever reached the tail check.
// With the ERROR gone, a clean accepted stack legitimately reached
// parserTailAllowsCleanAcceptance, which -- hardcoded to continuationEscape
// 0 -- called an ordinary trailing backtick+newline a real, uncovered tail
// and forced production to decline the clean accept, falling back to a
// degraded flat ERROR-wrapped tree instead of the properly nested shape.
//
// This witness's root does carry one pre-existing, unrelated PowerShell
// divergence from the C oracle around a trailing statement terminator
// (present identically on main before any of today's changes; out of scope
// here), so this pin checks the structural invariant the tail-acceptance fix
// actually restores -- a full-span, properly nested tree, not a collapsed
// ERROR wrapper -- rather than asserting byte-for-byte C parity.
func TestPowerShellTrailingContinuationDoesNotForceDegradedFallback(t *testing.T) {
	src := []byte("Get-Item a `\n | Out-Null `\n")
	lang := grammars.PowershellLanguage()

	for _, route := range []struct {
		name  string
		force bool
	}{
		{"production", false},
		{"compact-candidate", true},
	} {
		t.Run(route.name, func(t *testing.T) {
			parser := gts.NewParser(lang)
			parser.SetAdmissionCandidateRoute(route.force)
			tree, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("PowerShell parse error: %v", err)
			}
			defer tree.Release()
			root := requireCompleteParse(t, tree, src, lang, "PowerShell trailing continuation")

			if got, want := root.Type(lang), "program"; got != want {
				t.Fatalf("root.Type() = %q, want %q; tree=%s", got, want, root.SExpr(lang))
			}
			if root.ChildCount() != 1 || root.Child(0).IsError() {
				t.Fatalf("root is a degraded ERROR wrapper, want a properly nested program>statement_list tree; tree=%s", root.SExpr(lang))
			}
			if got, want := root.Child(0).Type(lang), "statement_list"; got != want {
				t.Fatalf("root's only child = %q, want %q (degraded fallback wraps the tree in ERROR instead); tree=%s", got, want, root.SExpr(lang))
			}
		})
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
// half of the repair: only backtick immediately followed by a newline is
// PowerShell's declared continuation escape (Language.LineContinuationEscapeByte).
// A bare backtick at the SAME structural position as
// TestPowerShellBacktickContinuationIsParserPadding's positive witness --
// immediately after the command name's separator, before the next parameter
// -- but followed by "x" instead of a newline, is not the declared escape
// sequence and must still surface as a real, HasError()==true parse, exactly
// as it did before this change. Unlike the incomplete-"$" witness this
// replaces, this one shares byte-for-byte structural position with the
// positive witness, so it actually exercises the padding classification: if
// bytesAreParserPadding's continuationEscape case ever widened to accept a
// bare escape byte without requiring a following newline (or matched an
// undeclared byte), this witness would flip from erroring to clean and catch
// it. See TestBytesAreParserPaddingLineContinuationEscapeComposes
// (parser_shift_gap_test.go) for the same composition property pinned
// directly at the bytesAreParserPadding predicate.
//
// The backtick+"x" sequence here defeats the parser badly enough that the
// GLR frontier runs out of live stacks before reaching EOF (unrelated to
// this fix -- PowerShell's command-argument tokenizer is permissive enough
// that few byte sequences are lexer-skipped "gaps" at all; this is one of
// the few that still is), so the returned tree is honestly truncated rather
// than full-span. HasError() is still the right, and only, invariant to
// check: requireCompleteParse's full-span requirement does not apply to a
// witness whose whole point is that the parse must NOT complete cleanly.
func TestPowerShellNonContinuationErrorsStillMaterialize(t *testing.T) {
	src := []byte("New-RpmSpec `x -Name $Name\n")
	lang := grammars.PowershellLanguage()
	for _, route := range []struct {
		name  string
		force *bool
	}{
		{"production", boolPtr(false)},
		{"compact-candidate", boolPtr(true)},
	} {
		t.Run(route.name, func(t *testing.T) {
			parser := gts.NewParser(lang)
			parser.SetAdmissionCandidateRoute(*route.force)
			tree, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("PowerShell parse error: %v", err)
			}
			defer tree.Release()
			root := tree.RootNode()
			if root == nil {
				t.Fatal("PowerShell parse returned a nil root")
			}

			if !root.HasError() {
				t.Fatalf("root.HasError() = false, want true for a genuine syntax error; tree=%s", root.SExpr(lang))
			}
		})
	}
}

func boolPtr(b bool) *bool { return &b }

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
