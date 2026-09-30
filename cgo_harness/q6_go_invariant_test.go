//go:build linux && cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestQ6GoOriginalIncrementalInvariant(t *testing.T) {
	language := grammars.GoLanguage()
	source := q6GoOriginalSource(t)
	for _, candidate := range []bool{false, true} {
		t.Run(fmt.Sprintf("compact=%t", candidate), func(t *testing.T) {
			parser := gts.NewParser(language)
			parser.SetAdmissionCandidateRoute(candidate)
			old, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { old.Release() }()
			current := source
			for i, step := range benchfixtures.EditingSession(source) {
				old.Edit(step.Edit)
				next, err := parser.ParseIncremental(step.Source, old)
				if err != nil {
					t.Fatal(err)
				}
				freshParser := gts.NewParser(language)
				freshParser.SetAdmissionCandidateRoute(candidate)
				fresh, err := freshParser.Parse(step.Source)
				if err != nil {
					next.Release()
					t.Fatal(err)
				}
				incrementalDigest, err := benchfixtures.InspectGoTree(next.RootNode(), language)
				if err != nil {
					t.Fatal(err)
				}
				freshDigest, err := benchfixtures.InspectGoTree(fresh.RootNode(), language)
				fresh.Release()
				if err != nil {
					t.Fatal(err)
				}
				if incrementalDigest.SHA256 != freshDigest.SHA256 {
					t.Fatalf("step %d incremental=%s fresh=%s", i+1, incrementalDigest.SHA256, freshDigest.SHA256)
				}
				root := next.RootNode()
				if root.IsError() && !root.HasError() {
					t.Fatalf("step %d ERROR root has no error flag", i+1)
				}
				stop := next.ParseRuntime().StopReason
				if root.EndByte() < uint32(len(step.Source)) && (stop == gts.ParseStopAccepted || stop == gts.ParseStopNone) {
					t.Fatalf("step %d unexplained root gap", i+1)
				}
				old.Release()
				old = next
				current = step.Source
			}
			if allocations := testing.AllocsPerRun(5, func() {
				next, err := parser.ParseIncremental(current, old)
				if err != nil {
					panic(err)
				}
				next.Release()
			}); allocations != 0 {
				t.Fatalf("no-edit reparse allocated %g times", allocations)
			}
		})
	}
}
