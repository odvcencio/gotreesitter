package gotreesitter_test

import (
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestIncrementalFreshVerificationDoesNotRetryAgain(t *testing.T) {
	before := []byte("t\nfrom s import(eg,)")
	after := []byte("t\nfrom s import(e(,)")
	lang := grammars.DetectLanguageByName("python").Language()
	for _, candidate := range []bool{false, true} {
		for _, profiled := range []bool{false, true} {
			t.Run(fmt.Sprintf("candidate=%t/profiled=%t", candidate, profiled), func(t *testing.T) {
				p := gts.NewParser(lang)
				p.SetAdmissionCandidateRoute(candidate)
				old, err := p.Parse(before)
				if err != nil {
					t.Fatal(err)
				}
				defer old.Release()
				if old.RootNode().HasError() {
					t.Fatal("original must parse cleanly")
				}
				old.Edit(gts.InputEdit{StartByte: 17, OldEndByte: 18, NewEndByte: 18, StartPoint: gts.Point{Row: 1, Column: 15}, OldEndPoint: gts.Point{Row: 1, Column: 16}, NewEndPoint: gts.Point{Row: 1, Column: 16}})
				var next *gts.Tree
				var profile gts.IncrementalParseProfile
				if profiled {
					next, profile, err = p.ParseIncrementalProfiled(after, old)
				} else {
					next, err = p.ParseIncremental(after, old)
				}
				if err != nil {
					t.Fatal(err)
				}
				defer next.Release()
				freshParser := gts.NewParser(lang)
				freshParser.SetAdmissionCandidateRoute(candidate)
				fresh, err := freshParser.Parse(after)
				if err != nil {
					t.Fatal(err)
				}
				defer fresh.Release()
				got, err := benchfixtures.InspectGoTree(next.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				want, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				if got.SHA256 != want.SHA256 {
					t.Fatalf("incremental=%s fresh=%s", next.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang))
				}
				if !next.RootNode().HasError() || next.RootNode().EndByte() != uint32(len(after)) {
					t.Fatal("replacement must report the syntax error and cover the input")
				}
				allocs := testing.AllocsPerRun(5, func() {
					same, err := p.ParseIncremental(after, next)
					if err != nil {
						t.Fatal(err)
					}
					same.Release()
				})
				if allocs != 0 {
					t.Fatalf("no-edit allocations=%g", allocs)
				}
				if profiled {
					t.Logf("tokens=%d nodes=%d reused_bytes=%d fallback=%s", profile.TokensConsumed, profile.NewNodesAllocated, profile.ReusedBytes, profile.ReuseUnsupportedReason)
				}
			})
		}
	}
}
