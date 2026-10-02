//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// Run one named subtest per process. Syntax-breaking edits still have to equal
// fresh Go; the dedicated locked-C witnesses cover the native reuse boundary.
func TestIncrementalCReuseFleetInvariant(t *testing.T) {
	engine := os.Getenv("GTS_C_REUSE_FLEET_ENGINE")
	if engine != "" && engine != "legacy" && engine != "compact" {
		t.Fatal("GTS_C_REUSE_FLEET_ENGINE must be legacy or compact")
	}
	testIncrementalCReuseFleetInvariant(t, engine == "compact", false)
}

func TestIncrementalCReuseFleetCandidateInvariant(t *testing.T) {
	testIncrementalCReuseFleetInvariant(t, true, false)
}

func TestIncrementalCReuseObservedFleetInvariant(t *testing.T) {
	testIncrementalCReuseFleetInvariant(t, true, true)
}

func testIncrementalCReuseFleetInvariant(t *testing.T, candidate, observed bool) {

	for _, tc := range parityCases {
		t.Run(tc.name, func(t *testing.T) {
			entry := parityEntriesByName[tc.name]
			lang := entry.Language()
			report := grammars.EvaluateParseSupport(entry, lang)
			t.Logf("backend=%v candidate=%t", report.Backend, candidate)
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(candidate)
			if observed {
				p.SetLogger(func(gts.ParserLogType, string) {})
			}

			var profile gts.IncrementalParseProfile
			parse := func(source []byte, old *gts.Tree) (*gts.Tree, error) {
				var tree *gts.Tree
				var err error
				if report.Backend == grammars.ParseBackendTokenSource {
					if old == nil {
						return p.ParseWithTokenSource(source, entry.TokenSourceFactory(source, lang))
					}
					tree, profile, err = p.ParseIncrementalWithTokenSourceProfiled(source, old, entry.TokenSourceFactory(source, lang))
					return tree, err
				}
				if old == nil {
					return p.Parse(source)
				}
				tree, profile, err = p.ParseIncrementalProfiled(source, old)
				return tree, err
			}
			source := normalizedSource(tc.name, tc.source)
			old, err := parse(source, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { old.Release() }()
			for cycle, at := range []int{min(1, len(source)), len(source) / 3, len(source) / 2, len(source)} {
				for _, added := range []byte{' ', '/', '"'} {
					broken := append(bytes.Clone(source[:at]), added)
					broken = append(broken, source[at:]...)
					for step, to := range [][]byte{broken, source} {
						from := source
						if step != 0 {
							from = broken
						}
						old.Edit(canonicalGoInputEdit(from, to, at, at+len(from)-len(source), at+len(to)-len(source)))
						next, err := parse(to, old)
						if err != nil {
							t.Fatal(err)
						}
						fresh, err := parse(to, nil)
						if err != nil {
							t.Fatal(err)
						}
						got, err := benchfixtures.InspectGoTree(next.RootNode(), lang)
						if err != nil {
							t.Fatal(err)
						}
						want, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
						if err != nil {
							t.Fatal(err)
						}
						if got.SHA256 != want.SHA256 {
							t.Fatalf("cycle=%d at=%d inserted=%q step=%d incremental=%s fresh=%s", cycle, at, added, step, next.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang))
						}
						root := next.RootNode()
						if root.IsError() && !root.HasError() {
							t.Fatal("ERROR root does not report HasError")
						}
						// C's public root excludes leading lexer-skipped padding;
						// its visible start is already checked by the deep digest.
						if root.EndByte() != uint32(len(to)) && (next.ParseStopReason() == gts.ParseStopAccepted || next.ParseStopReason() == gts.ParseStopNone) {
							t.Fatalf("unexplained coverage gap: %d..%d of %d", root.StartByte(), root.EndByte(), len(to))
						}
						fresh.Release()
						old.Release()
						old = next
					}
				}
			}
			// Same-width replacements exercise the native certificates rather
			// than the retained moving-boundary proof. Include edits that turn
			// a clean token into recovery, then restore it on the next step.
			for _, at := range []int{min(1, len(source)-1), len(source) / 3, len(source) / 2, len(source) - 1} {
				for _, replacement := range []byte{'x', ' ', '"'} {
					changed := bytes.Clone(source)
					changed[at] = replacement
					for step, to := range [][]byte{changed, source} {
						from := source
						if step != 0 {
							from = changed
						}
						old.Edit(canonicalGoInputEdit(from, to, at, at+1, at+1))
						next, err := parse(to, old)
						if err != nil {
							t.Fatal(err)
						}
						fresh, err := parse(to, nil)
						if err != nil {
							t.Fatal(err)
						}
						got, err := benchfixtures.InspectGoTree(next.RootNode(), lang)
						if err != nil {
							t.Fatal(err)
						}
						want, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
						if err != nil {
							t.Fatal(err)
						}
						if got.SHA256 != want.SHA256 {
							t.Fatalf("replacement at=%d byte=%q step=%d source=%q backend=%v profile=%+v incremental=%s fresh=%s", at, replacement, step, to, report.Backend, profile, next.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang))
						}
						root := next.RootNode()
						if root.IsError() && !root.HasError() {
							t.Fatal("replacement ERROR root does not report HasError")
						}
						if root.EndByte() != uint32(len(to)) && (next.ParseStopReason() == gts.ParseStopAccepted || next.ParseStopReason() == gts.ParseStopNone) {
							t.Fatalf("replacement coverage gap: %d..%d of %d", root.StartByte(), root.EndByte(), len(to))
						}
						fresh.Release()
						old.Release()
						old = next
					}
				}
			}
			allocs := testing.AllocsPerRun(3, func() {
				next, err := p.ParseIncremental(source, old)
				if err != nil {
					t.Fatal(err)
				}
				next.Release()
			})
			if allocs != 0 {
				t.Fatalf("no-edit allocations=%g", allocs)
			}
			t.Logf("48 edit-session steps; no-edit allocations=%g", allocs)
		})
	}
}
