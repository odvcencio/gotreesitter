//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"errors"
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type failingCRebuilder struct {
	*grammars.CTokenSource
	err error
}

func (ts *failingCRebuilder) RebuildTokenSource([]byte, *gts.Language) (gts.TokenSource, error) {
	return nil, ts.err
}

func TestTokenSourceRecoveryVerificationFailure(t *testing.T) {
	before := []byte("/**/#e\nenum{L,/**/C,/**/T,E};r e[]{}")
	after := append([]byte{'x'}, before...)
	lang := grammars.DetectLanguageByName("c").Language()
	want := errors.New("verification stream unavailable")
	for _, mode := range []string{"factory", "rebuilder", "profiled", "options"} {
		t.Run(mode, func(t *testing.T) {
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(false)
			initial, err := grammars.NewCTokenSource(before, lang)
			if err != nil {
				t.Fatal(err)
			}
			old, err := p.ParseWithTokenSource(before, initial)
			if err != nil {
				t.Fatal(err)
			}
			defer old.Release()
			old.Edit(canonicalGoInputEdit(before, after, 0, 0, 1))
			base, err := grammars.NewCTokenSource(after, lang)
			if err != nil {
				t.Fatal(err)
			}
			ts := &failingCRebuilder{base, want}
			var next *gts.Tree
			switch mode {
			case "factory":
				calls := 0
				next, err = p.ParseIncrementalWithTokenSourceFactory(after, old, func([]byte) (gts.TokenSource, error) {
					calls++
					if calls == 1 {
						return ts, nil
					}
					return nil, want
				})
			case "rebuilder":
				next, err = p.ParseIncrementalWithTokenSource(after, old, ts)
			case "profiled":
				next, _, err = p.ParseIncrementalWithTokenSourceProfiled(after, old, ts)
			case "options":
				var result gts.ParseResult
				result, err = p.ParseWith(after, gts.WithOldTree(old), gts.WithTokenSource(ts), gts.WithProfiling())
				next = result.Tree
			}
			if next != nil {
				defer next.Release()
			}
			if !errors.Is(err, want) || next != nil {
				t.Fatalf("failed verifier published a tree: tree=%v error=%v", next, err)
			}
			// A failed operation must leave the caller-owned old tree usable.
			if old.RootNode() == nil {
				t.Fatal("old tree was released")
			}
		})
	}
}

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
		for _, mode := range []string{"factory", "token_source", "profiled", "options", "token_source_no_rebuilder", "profiled_no_rebuilder", "options_no_rebuilder"} {
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
				case "token_source_no_rebuilder":
					next, err = p.ParseIncrementalWithTokenSource(after, old, newCSourceWithoutRebuilder(t, after, lang))
				case "profiled_no_rebuilder":
					next, profile, err = p.ParseIncrementalWithTokenSourceProfiled(after, old, newCSourceWithoutRebuilder(t, after, lang))
				case "options_no_rebuilder":
					var result gts.ParseResult
					result, err = p.ParseWith(after, gts.WithOldTree(old),
						gts.WithTokenSource(newCSourceWithoutRebuilder(t, after, lang)), gts.WithProfiling())
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
				if mode == "token_source_no_rebuilder" || mode == "profiled_no_rebuilder" || mode == "options_no_rebuilder" {
					unrebuildableFresh, err := p.ParseWithTokenSource(after, newCSourceWithoutRebuilder(t, after, lang))
					if err != nil {
						t.Fatal(err)
					}
					defer unrebuildableFresh.Release()
					assertLockedCTreeExactWithErrors(t, "fresh without rebuilder", unrebuildableFresh, lang, cAfter)
				}
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
				if mode == "profiled" || mode == "options" || mode == "profiled_no_rebuilder" || mode == "options_no_rebuilder" {
					t.Logf("tokens=%d nodes=%d reused_subtrees=%d reused_bytes=%d fallback=%s no_edit_allocs=%g",
						profile.TokensConsumed, profile.NewNodesAllocated, profile.ReusedSubtrees, profile.ReusedBytes,
						profile.ReuseUnsupportedReason, allocs)
				}
			})
		}
	}
}

// Shadow only the optional rebuilder. Every other C lexer capability, including
// stable token boundaries and deterministic skipping, remains available.
type cSourceWithoutRebuilder struct {
	*grammars.CTokenSource
	RebuildTokenSource struct{}
}

func newCSourceWithoutRebuilder(t testing.TB, source []byte, lang *gts.Language) gts.TokenSource {
	t.Helper()
	ts, err := grammars.NewCTokenSource(source, lang)
	if err != nil {
		t.Fatal(err)
	}
	wrapped := &cSourceWithoutRebuilder{CTokenSource: ts}
	if _, ok := any(wrapped).(gts.TokenSourceRebuilder); ok {
		t.Fatal("test stream must not provide a rebuilder")
	}
	return wrapped
}

func BenchmarkCTokenSourceWithoutRebuilderRecovery(b *testing.B) {
	before := []byte("/**/#e\nenum{L,/**/C,/**/T,E};r e[]{}")
	after := append([]byte{'x'}, before...)
	lang := grammars.DetectLanguageByName("c").Language()
	p := gts.NewParser(lang)
	p.SetAdmissionCandidateRoute(false)
	edit := canonicalGoInputEdit(before, after, 0, 0, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		old, err := p.ParseWithTokenSource(before, newCSourceWithoutRebuilder(b, before, lang))
		if err != nil {
			b.Fatal(err)
		}
		old.Edit(edit)
		next, err := p.ParseIncrementalWithTokenSource(after, old, newCSourceWithoutRebuilder(b, after, lang))
		if err != nil {
			b.Fatal(err)
		}
		next.Release()
		old.Release()
	}
}
