package grammars_test

import (
	"fmt"
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// A reused string leaf used to bypass the exp_list shift/reduce conflict,
// losing the list reduction before an edit later in the same statement.
func TestAWKIncrementalInsertMatchesFresh(t *testing.T) {
	reported, err := os.ReadFile("../internal/benchfixtures/testdata/real/awk")
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		name   string
		source []byte
		offset int
	}{
		{"shrunk", []byte(`{ a[(1),(2),""]=value }`), 18},
		{"reported", reported, 438},
	} {
		for _, candidate := range []bool{false, true} {
			for _, profiled := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/candidate=%t/profiled=%t", fixture.name, candidate, profiled), func(t *testing.T) {
					lang := grammars.AwkLanguage()
					parser := gts.NewParser(lang)
					parser.SetAdmissionCandidateRoute(candidate)
					source := fixture.source
					tree, err := parser.Parse(source)
					if err != nil {
						t.Fatal(err)
					}
					defer func() { tree.Release() }()
					for step, replacement := range []string{"x", "y", ""} {
						oldEnd := fixture.offset
						if step > 0 {
							oldEnd++
						}
						edited := append([]byte(nil), source[:fixture.offset]...)
						edited = append(edited, replacement...)
						edited = append(edited, source[oldEnd:]...)
						tree.Edit(gts.InputEdit{
							StartByte: uint32(fixture.offset), OldEndByte: uint32(oldEnd), NewEndByte: uint32(fixture.offset + len(replacement)),
							StartPoint: spliceTestPoint(source, fixture.offset), OldEndPoint: spliceTestPoint(source, oldEnd), NewEndPoint: spliceTestPoint(edited, fixture.offset+len(replacement)),
						})
						var next *gts.Tree
						if profiled {
							var profile gts.IncrementalParseProfile
							next, profile, err = parser.ParseIncrementalProfiled(edited, tree)
							if profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
								t.Fatalf("step %d lost subtree reuse: %+v", step+1, profile)
							}
							if profile.ReuseObservedPreGotoStateMismatch != 0 {
								t.Fatalf("step %d misclassified a proven leaf frontier as unproven top-level ownership: %+v", step+1, profile)
							}
						} else {
							next, err = parser.ParseIncremental(edited, tree)
						}
						if err != nil {
							t.Fatal(err)
						}
						tree.Release()
						tree = next
						freshParser := gts.NewParser(lang)
						freshParser.SetAdmissionCandidateRoute(candidate)
						fresh, err := freshParser.Parse(edited)
						if err != nil {
							t.Fatal(err)
						}
						func() {
							defer fresh.Release()
							for _, parsed := range []*gts.Tree{tree, fresh} {
								root := parsed.RootNode()
								if root == nil || root.HasError() || root.StartByte() != 0 || root.EndByte() != uint32(len(edited)) || parsed.ParseRuntime().StopReason != gts.ParseStopAccepted {
									t.Fatalf("step %d did not accept the complete clean input: %s", step+1, parsed.ParseRuntime().Summary())
								}
							}
							incrementalDigest, err := benchfixtures.InspectGoTree(tree.RootNode(), lang)
							if err != nil {
								t.Fatal(err)
							}
							freshDigest, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
							if err != nil {
								t.Fatal(err)
							}
							if incrementalDigest.SHA256 != freshDigest.SHA256 {
								t.Fatalf("step %d differs from fresh: incremental=%s fresh=%s\nincremental: %s\nfresh: %s", step+1, incrementalDigest.SHA256, freshDigest.SHA256, tree.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang))
							}
						}()
						source = edited
						if allocations := testing.AllocsPerRun(5, func() {
							unchanged, err := parser.ParseIncremental(source, tree)
							if err != nil {
								panic(err)
							}
							unchanged.Release()
						}); allocations != 0 {
							t.Fatalf("step %d no-edit reparse allocated %g times", step+1, allocations)
						}
					}
				})
			}
		}
	}
}
