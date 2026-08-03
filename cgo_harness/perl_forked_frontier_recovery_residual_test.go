//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// perlResidualTailBody builds a long, valid Perl tail. The residual shapes
// below only lose a few bytes on a one-line file, which is what made the first
// measurement of this lane read "max 32 bytes". That number was an artifact of
// the mutation corpus using short bases. With a realistic tail behind the
// defect, the SAME shapes lose essentially the whole file, because the version
// that survives the fork resyncs into a state that cannot make progress and
// nothing after the defect is ever parsed.
func perlResidualTailBody() string {
	var b strings.Builder
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&b, "my $v%d = %d;\nsub f%d { return $v%d + 1; }\n", i, i, i, i)
	}
	return b.String()
}

// TestPerlForkedFrontierRecoveryResiduals pins the two shapes that the
// live-sibling recovery gates (parser.go, anotherLiveParseStackRemains) still
// get wrong, so the cost of those gates is on the record and so the
// merge-election lane has an acceptance instrument.
//
// MECHANISM. Every one of these sources reaches a frontier holding two
// versions at the same state, byte offset, and depth, differing only in a
// buried subtree. C collapses that pair at ts_stack_merge and elects one
// subtree with ts_parser__select_tree, so C's frontier returns to one version
// immediately. Go refuses the merge because the shapes differ
// (gssStacksHaveDistinctMaterializingShapes), so the duplicate frontier
// persists. Every gate in this lane is then a heuristic about how long to keep
// a version alive, which is a proxy for the merge-time election Go never
// performs. When a real syntax error arrives, the surviving version resyncs
// into a state with no forward progress and the rest of the file is lost.
//
// SCOPE OF THE RESIDUAL. These are NOT caused by the zero-width external-token
// rescue. On "try { A(;) } ..." the reference runtime emits `_NONASSOC` at the
// same byte with no rescue at all; the divergence is introduced purely by the
// recovery gates. Measured against origin/main at 4347068b, production route.
//
// Each case asserts that the defect is STILL PRESENT. If one stops failing,
// the merge-election lane has repaired it: delete the entry and record the
// receipt.
func TestPerlForkedFrontierRecoveryResiduals(t *testing.T) {
	goLang := grammars.PerlLanguage()
	cLang, err := COracleLanguage("perl")
	if err != nil {
		t.Fatal(err)
	}
	tail := perlResidualTailBody()

	tests := []struct {
		name string
		// source is the defect line plus, for the "long" variants, a valid tail.
		source string
		// baseEnd is the end byte origin/main@4347068b reaches on this source.
		baseEnd int
		// baseStop is the stop reason origin/main reports.
		baseStop string
	}{
		{
			// Unclosed call with a `;` inside, one line.
			name:     "unclosed_call_semicolon_short",
			source:   "try { A(;) } catch($e) { B(); }\n",
			baseEnd:  32,
			baseStop: "accepted",
		},
		{
			// The same defect with a realistic tail behind it. This is the
			// case the short measurement hid: 2563 of 2572 bytes lost.
			name:     "unclosed_call_semicolon_long",
			source:   "try { A(;) } catch($e) { B(); }\n" + tail,
			baseEnd:  2572,
			baseStop: "accepted",
		},
		{
			// Missing open paren after `catch`, one line.
			name:     "catch_missing_open_paren_short",
			source:   "try { A(); } catch$e) { B(); }\n",
			baseEnd:  31,
			baseStop: "accepted",
		},
		{
			// The same defect with a realistic tail: 2545 of 2571 bytes lost.
			// The earlier note called this "a reduce-chain cycle"; that is this
			// mechanism, and on a real file it costs the whole file, not five
			// bytes.
			name:     "catch_missing_open_paren_long",
			source:   "try { A(); } catch$e) { B(); }\n" + tail,
			baseEnd:  2571,
			baseStop: "accepted",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			scheduleParityMemoryScavenge(t)
			src := []byte(test.source)

			goParser := gotreesitter.NewParser(goLang)
			goParser.SetAdmissionCandidateRoute(false)
			tree, err := goParser.Parse(src)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			defer tree.Release()

			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(src, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C parse returned a nil tree")
			}
			defer cTree.Close()

			goEnd := int(tree.RootNode().EndByte())
			cEnd := int(cTree.RootNode().EndByte())
			stop := string(tree.ParseStopReason())

			if goEnd >= cEnd {
				t.Fatalf(
					"%q now reaches the end of the source (go=%d c=%d of %d, stop=%s). The "+
						"merge-election lane has repaired this residual: delete this entry and "+
						"record the receipt.",
					test.name, goEnd, cEnd, len(src), stop)
			}
			t.Skipf(
				"known recovery-gate residual: go stops at %d of %d (stop=%s), C reaches %d; "+
					"origin/main@4347068b reached %d (stop=%s), so this lane loses %d byte(s) (%d%%)",
				goEnd, len(src), stop, cEnd, test.baseEnd, test.baseStop,
				test.baseEnd-goEnd, (test.baseEnd-goEnd)*100/test.baseEnd)
		})
	}
}

// TestPerlForkedFrontierRecoveryStopReasonCensus records the stop-reason shift
// the recovery gates cause, because coverage alone hid it. Over a 1170-case
// deterministic Perl mutation corpus, origin/main@4347068b reports 1073
// `accepted` and 97 `no_stacks_alive`; this lane reports 1044 and 126, with 47
// cases moving from `accepted` into `no_stacks_alive` and 18 the other way.
// The net coverage number (5 truncations against 24 repairs) is therefore NOT
// the whole cost: a case can keep its byte coverage and still change how the
// parse terminated.
//
// This test pins the smallest source that makes the shift visible, so the
// census is reproducible without the corpus generator.
func TestPerlForkedFrontierRecoveryStopReasonCensus(t *testing.T) {
	goLang := grammars.PerlLanguage()

	// origin/main@4347068b reports "accepted" for this source and reaches the
	// end. This lane reports no_stacks_alive and stops early.
	source := []byte("try { A(;) } catch($e) { B(); }\n")

	scheduleParityMemoryScavenge(t)
	p := gotreesitter.NewParser(goLang)
	p.SetAdmissionCandidateRoute(false)
	tree, err := p.Parse(source)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	defer tree.Release()

	stop := string(tree.ParseStopReason())
	if stop == "accepted" && int(tree.RootNode().EndByte()) >= len(source) {
		t.Fatalf(
			"stop reason is back to %q with full coverage; the recovery-gate residual is "+
				"repaired -- delete this witness and record the receipt", stop)
	}
	t.Skipf("known recovery-gate residual: stop=%s end=%d of %d (origin/main@4347068b: accepted, %d)",
		stop, tree.RootNode().EndByte(), len(source), len(source))
}
