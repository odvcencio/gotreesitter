//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Shrunk from the first receipt-style edit of Git's ctype.c. The factory
// entry deferred recovery verification to a DFA retry it never ran, leaving
// the unchanged enum marked HasError even though fresh Go and C keep it clean.
func TestTokenSourceRecoveryFrontierMatchesFreshC(t *testing.T) {
	before := []byte("/**/#e\nenum{L,/**/C,/**/T,E};r e[]{}")
	after := append([]byte{'x'}, before...)
	entry := grammars.DetectLanguageByName("c")
	lang := entry.Language()
	factory := func(source []byte) (gts.TokenSource, error) {
		return entry.TokenSourceFactory(source, lang), nil
	}
	cl, err := COracleLanguage("c")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	cBefore, cAfter := cp.Parse(before, nil), cp.Parse(after, nil)
	defer cBefore.Close()
	defer cAfter.Close()
	if cBefore.RootNode().HasError() || !cAfter.RootNode().HasError() {
		t.Fatal("locked C must accept the original and recover the inserted prefix")
	}
	for _, candidate := range []bool{false, true} {
		for _, mode := range []string{"factory", "token_source", "profiled", "options"} {
			t.Run(fmt.Sprintf("candidate=%t/%s", candidate, mode), func(t *testing.T) {
				p := gts.NewParser(lang)
				p.SetAdmissionCandidateRoute(candidate)
				old, err := p.ParseWithTokenSourceFactory(before, factory)
				if err != nil {
					t.Fatal(err)
				}
				defer old.Release()
				assertLockedCTreeExact(t, "original", old, lang, cBefore)
				old.Edit(canonicalGoInputEdit(before, after, 0, 0, 1))
				var next *gts.Tree
				var profile gts.IncrementalParseProfile
				switch mode {
				case "factory":
					next, err = p.ParseIncrementalWithTokenSourceFactory(after, old, factory)
				case "token_source":
					next, err = p.ParseIncrementalWithTokenSource(after, old, entry.TokenSourceFactory(after, lang))
				case "profiled":
					next, profile, err = p.ParseIncrementalWithTokenSourceProfiled(after, old, entry.TokenSourceFactory(after, lang))
				case "options":
					var result gts.ParseResult
					result, err = p.ParseWith(after, gts.WithOldTree(old),
						gts.WithTokenSource(entry.TokenSourceFactory(after, lang)), gts.WithProfiling())
					next, profile = result.Tree, result.Profile
				}
				if err != nil {
					t.Fatal(err)
				}
				defer next.Release()
				fresh, err := p.ParseWithTokenSourceFactory(after, factory)
				if err != nil {
					t.Fatal(err)
				}
				defer fresh.Release()
				assertLockedCTreeExactWithErrors(t, "fresh", fresh, lang, cAfter)
				assertLockedCTreeExactWithErrors(t, "incremental", next, lang, cAfter)
				got, err := benchfixtures.InspectGoTree(next.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				want, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
				if err != nil || got.SHA256 != want.SHA256 {
					t.Fatalf("incremental/fresh digest mismatch: %s / %s, %v", got.SHA256, want.SHA256, err)
				}
				root := next.RootNode()
				if root.EndByte() != uint32(len(after)) || root.IsError() && !root.HasError() {
					t.Fatal("incremental tree violates coverage or ERROR-root reporting")
				}
				allocs := testing.AllocsPerRun(5, func() {
					unchanged, err := p.ParseIncremental(after, next)
					if err != nil {
						t.Fatal(err)
					}
					unchanged.Release()
				})
				if allocs != 0 {
					t.Fatalf("no-edit reparse allocations=%g, want 0", allocs)
				}
				if mode == "profiled" || mode == "options" {
					t.Logf("tokens=%d nodes=%d reused_subtrees=%d reused_bytes=%d fallback=%s no_edit_allocs=%g",
						profile.TokensConsumed, profile.NewNodesAllocated, profile.ReusedSubtrees, profile.ReusedBytes,
						profile.ReuseUnsupportedReason, allocs)
				}
			})
		}
	}
}
