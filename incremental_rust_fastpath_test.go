package gotreesitter_test

import (
	"bytes"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	grm "github.com/odvcencio/gotreesitter/grammars"
)

func TestRustLineCommentTextEditUsesInvariantReuse(t *testing.T) {
	lang := grm.RustLanguage()
	oldSource := []byte("// Copyright 2012-2014\nfn main() {}\n")
	offset := bytes.Index(oldSource, []byte("2012"))
	if offset < 0 {
		t.Fatal("fixture missing edit marker")
	}
	offset += len("201")
	newSource := append([]byte(nil), oldSource...)
	newSource[offset] = '3'

	for _, route := range []string{"legacy", "compact"} {
		t.Run(route, func(t *testing.T) {
			parser := gts.NewParser(lang)
			parser.SetAdmissionCandidateRoute(route == "compact")
			fresh, err := parser.Parse(newSource)
			if err != nil {
				t.Fatalf("fresh parse: %v", err)
			}
			defer fresh.Release()
			routedBefore, _ := gts.AdmissionCandidateCounters()
			oldTree, err := parser.Parse(oldSource)
			if err != nil {
				t.Fatalf("old parse: %v", err)
			}
			defer oldTree.Release()
			routedAfter, _ := gts.AdmissionCandidateCounters()
			compactRouted := routedAfter > routedBefore
			if route == "legacy" && compactRouted {
				t.Fatal("legacy fixture entered the compact route")
			}

			edit := gts.InputEdit{
				StartByte:   uint32(offset),
				OldEndByte:  uint32(offset + 1),
				NewEndByte:  uint32(offset + 1),
				StartPoint:  pointForOffset(oldSource, offset),
				OldEndPoint: pointForOffset(oldSource, offset+1),
				NewEndPoint: pointForOffset(newSource, offset+1),
			}
			oldTree.Edit(edit)
			result, err := parser.ParseWith(newSource, gts.WithOldTree(oldTree), gts.WithProfiling())
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			defer result.Tree.Release()
			if got, want := result.Tree.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang); got != want {
				t.Fatalf("incremental tree mismatch\n got: %s\nwant: %s", got, want)
			}
			if !result.ProfileAvailable {
				t.Fatal("profile unavailable")
			}
			requireReleaseSameWidthReparse(t, result.Profile)
			requireIncrementalDeepTreeMatchesFresh(t, result.Tree, fresh, lang)
			if compactRouted {
				if !result.Profile.ReuseUnsupported || result.Profile.ReuseUnsupportedReason != "old tree was compact-materialized without a scanner-quiescence proof" ||
					result.Profile.ReusedSubtrees != 0 || result.Profile.ReusedBytes != 0 {
					t.Fatalf("compact Rust tree did not use the scanner-proof fallback: %+v", result.Profile)
				}
				return
			}
			if result.Profile.ReuseUnsupported {
				t.Fatalf("reuse unsupported: %s", result.Profile.ReuseUnsupportedReason)
			}
			if result.Profile.ReusedSubtrees == 0 {
				t.Fatal("ordinary Rust reuse lost unchanged subtrees")
			}
		})
	}
}
