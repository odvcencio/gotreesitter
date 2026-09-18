//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter_test

import (
	"bytes"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestCompactMissingBraceRecoveryPreservesLockedCDigest(t *testing.T) {
	source := []byte("package p\nfunc a() { _ = 1 }\nfunc b() { _ = 2 }\n")
	start := bytes.IndexByte(source, '}')
	edited := bytes.Replace(source, []byte("}"), nil, 1)
	// Both locked C fresh and incremental parses produce this digest.
	const want = "fbaa5356fbcbc44db292d802215f3c0a86f83cb916fe138991a101b79ba1e09d"
	for _, incremental := range []bool{false, true} {
		name := "fresh"
		if incremental {
			name = "compact_old"
		}
		t.Run(name, func(t *testing.T) {
			language := grammars.GoLanguage()
			parser := gts.NewParser(language)
			parser.SetAdmissionCandidateRoute(true)
			var tree *gts.Tree
			var err error
			if incremental {
				old, parseErr := parser.Parse(source)
				if parseErr != nil {
					t.Fatal(parseErr)
				}
				defer old.Release()
				column := uint32(start - len("package p\n"))
				old.Edit(gts.InputEdit{
					StartByte: uint32(start), OldEndByte: uint32(start + 1), NewEndByte: uint32(start),
					StartPoint: gts.Point{Row: 1, Column: column}, OldEndPoint: gts.Point{Row: 1, Column: column + 1},
					NewEndPoint: gts.Point{Row: 1, Column: column},
				})
				tree, err = parser.ParseIncremental(edited, old)
			} else {
				routedBefore, fallbackBefore := gts.AdmissionCandidateCounters()
				tree, err = parser.Parse(edited)
				routed, fallback := gts.AdmissionCandidateCounters()
				if routed != routedBefore+1 || fallback != fallbackBefore {
					if tree != nil {
						tree.Release()
					}
					t.Fatalf("fresh compact admission failed: %q", gts.AdmissionCandidateLastFallbackReason())
				}
			}
			if err != nil || tree == nil {
				t.Fatalf("parse tree=%v err=%v", tree, err)
			}
			defer tree.Release()
			runtime := tree.ParseRuntime()
			if incremental && (!runtime.CompactIncrementalFullRecoveryRoute || runtime.CompactIncrementalReuseRoute || runtime.IncrementalAcceptedErrorRetryAttempts != 0) {
				t.Fatalf("compact recovery route lacks proof: %s", runtime.Summary())
			}
			if runtime.Truncated || tree.RootNode().EndByte() != uint32(len(edited)) {
				t.Fatalf("incomplete recovery: %s", runtime.Summary())
			}
			inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
			if err != nil || inspection.SHA256 != want {
				t.Fatalf("digest=%s want=%s err=%v", inspection.SHA256, want, err)
			}
		})
	}
}
