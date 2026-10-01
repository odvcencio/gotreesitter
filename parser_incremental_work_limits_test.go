package gotreesitter

import (
	"strings"
	"testing"
)

func TestIncrementalWorkLimitsMatchFreshSchedule(t *testing.T) {
	lang := loadBlobForDecode(t, "json")
	source := []byte("[" + strings.Repeat("1,", 200) + "1]")
	edited := append([]byte(nil), source...)
	edited[1] = '2'
	initial := NewParser(lang)
	initial.SetAdmissionCandidateRoute(false)
	old, err := initial.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	old.Edit(InputEdit{StartByte: 1, OldEndByte: 2, NewEndByte: 2, StartPoint: Point{Column: 1}, OldEndPoint: Point{Column: 2}, NewEndPoint: Point{Column: 2}})
	for _, limit := range []struct {
		name   string
		limits ParseWorkLimits
		stop   ParseStopReason
	}{
		{"iterations", ParseWorkLimits{IterationLimit: 20}, ParseStopIterationLimit},
		{"nodes", ParseWorkLimits{NodeLimit: 20}, ParseStopNodeLimit},
		{"depth", ParseWorkLimits{StackDepthLimit: 2}, ParseStopStackDepthLimit},
	} {
		for _, api := range []string{"plain", "profiled", "token_source", "token_source_profiled"} {
			t.Run(limit.name+"/"+api, func(t *testing.T) {
				p := NewParser(lang)
				p.SetAdmissionCandidateRoute(false)
				p.SetParseWorkLimits(limit.limits)
				var next *Tree
				var profile IncrementalParseProfile
				var err error
				switch api {
				case "plain":
					next, err = p.ParseIncremental(edited, old)
				case "profiled":
					next, profile, err = p.ParseIncrementalProfiled(edited, old)
				default:
					ts := p.acquireParserDFATokenSource(edited)
					defer ts.Close()
					if api == "token_source" {
						next, err = p.ParseIncrementalWithTokenSource(edited, old, ts)
					} else {
						next, profile, err = p.ParseIncrementalWithTokenSourceProfiled(edited, old, ts)
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				defer next.Release()
				fresh, err := p.Parse(edited)
				if err != nil {
					t.Fatal(err)
				}
				defer fresh.Release()
				if next.ParseStopReason() != limit.stop || fresh.ParseStopReason() != limit.stop {
					t.Fatalf("incremental stop=%s fresh stop=%s want=%s", next.ParseStopReason(), fresh.ParseStopReason(), limit.stop)
				}
				assertReleaseRootTreeEqual(t, next.RootNode(), fresh.RootNode(), lang)
				if next.ParseRuntime().Iterations != fresh.ParseRuntime().Iterations || next.ParseRuntime().NodesAllocated != fresh.ParseRuntime().NodesAllocated {
					t.Fatal("bounded incremental work differs from the fresh schedule")
				}
				if strings.Contains(api, "profiled") && (profile.ReusedSubtrees != 0 || profile.ReuseUnsupportedReason != "configured_work_limits") {
					t.Fatalf("bounded request reused old work: %+v", profile)
				}
			})
		}
	}
}
