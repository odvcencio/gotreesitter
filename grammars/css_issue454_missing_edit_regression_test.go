package grammars_test

import (
	"bytes"
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestIssue454MissingEditForLengthChangeFallsBackToFreshParse is issue
// #454's §8 finding: an old tree with NO recorded Tree.Edit call, handed to
// ParseIncremental alongside a new source of a DIFFERENT length, used to
// proceed silently instead of failing. Tree.Edit is what tells the reuse
// cursor how old byte positions map onto the new source; with zero edits
// nothing remaps them, so old positions were read as-is against the new
// bytes. The incremental path also runs with different GLR stack/merge caps
// than a fresh Parse, so the result was not merely stale -- it could
// genuinely differ from a fresh parse of the same bytes, and neither the
// returned error nor Tree.ParseStoppedEarly() said so.
//
// This is the caller's bug (Tree.Edit must be called for every edit), but a
// forgotten call must not silently corrupt the tree. ParseIncremental now
// detects the length mismatch with no recorded edit and falls back to an
// ordinary fresh parse, the same way it already does for an old tree it
// cannot trust for any other reason (a language or included-ranges
// mismatch).
//
// Reproduction is the downstream report's own CSS fixture and edit site
// (issue #454 §3's genCSS, "012345" insert), which is what exposed the
// original one-node divergence (70,713 incremental vs 70,714 fresh nodes,
// with HasError() true on the incremental tree and false on the fresh one).
func TestIssue454MissingEditForLengthChangeFallsBackToFreshParse(t *testing.T) {
	lang := grammars.CssLanguage()
	src := issue454GenCSS(137 << 10)
	site := bytes.Index(src, []byte("012345"))
	if site < 0 {
		t.Fatal("fixture has no \"012345\" marker")
	}
	// Duplicate the byte at site: an ordinary one-byte insert, same shape as
	// the downstream report's §3 harness. Length changes; no Tree.Edit call
	// follows, on purpose -- this is the bug under test.
	edited := append(append(append([]byte(nil), src[:site]...), src[site]), src[site:]...)
	if len(edited) == len(src) {
		t.Fatal("edited fixture must have a different length than src")
	}

	old, err := gotreesitter.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatalf("old Parse: %v", err)
	}
	defer old.Release()
	// Deliberately no old.Edit(...) call here: this is the exact caller
	// mistake issue #454 §8 describes.

	incremental, err := gotreesitter.NewParser(lang).ParseIncremental(edited, old)
	if err != nil {
		t.Fatalf("ParseIncremental: %v", err)
	}
	defer incremental.Release()

	fresh, err := gotreesitter.NewParser(lang).Parse(edited)
	if err != nil {
		t.Fatalf("fresh Parse: %v", err)
	}
	defer fresh.Release()

	incRoot, freshRoot := incremental.RootNode(), fresh.RootNode()
	if incRoot == nil || freshRoot == nil {
		t.Fatal("parse returned no root")
	}
	if got, want := incRoot.SExpr(lang), freshRoot.SExpr(lang); got != want {
		t.Fatalf("ParseIncremental with no recorded edit and a length-changed source diverged from a fresh parse:\nincHasError=%v freshHasError=%v",
			incRoot.HasError(), freshRoot.HasError())
	}
	if incRoot.HasError() != freshRoot.HasError() {
		t.Fatalf("HasError mismatch: incremental=%v fresh=%v", incRoot.HasError(), freshRoot.HasError())
	}
}

// TestIssue454MissingEditForLengthChangeProfiledReportsReason exercises the
// same scenario through ParseIncrementalProfiled, which surfaces the fallback
// reason directly instead of only through tree correctness.
func TestIssue454MissingEditForLengthChangeProfiledReportsReason(t *testing.T) {
	lang := grammars.CssLanguage()
	src := issue454GenCSS(137 << 10)
	site := bytes.Index(src, []byte("012345"))
	if site < 0 {
		t.Fatal("fixture has no \"012345\" marker")
	}
	edited := append(append(append([]byte(nil), src[:site]...), src[site]), src[site:]...)

	old, err := gotreesitter.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatalf("old Parse: %v", err)
	}
	defer old.Release()
	// No old.Edit(...) call: same caller mistake as above.

	incremental, profile, err := gotreesitter.NewParser(lang).ParseIncrementalProfiled(edited, old)
	if err != nil {
		t.Fatalf("ParseIncrementalProfiled: %v", err)
	}
	defer incremental.Release()

	if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "missing_edit_for_length_change" {
		t.Fatalf("reuse unsupported reason = %q (unsupported=%v), want \"missing_edit_for_length_change\"",
			profile.ReuseUnsupportedReason, profile.ReuseUnsupported)
	}

	fresh, err := gotreesitter.NewParser(lang).Parse(edited)
	if err != nil {
		t.Fatalf("fresh Parse: %v", err)
	}
	defer fresh.Release()

	if got, want := incremental.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang); got != want {
		t.Fatal("ParseIncrementalProfiled tree does not match a fresh parse")
	}
}

func issue454GenCSS(n int) []byte {
	var b bytes.Buffer
	for i := 0; b.Len() < n; i++ {
		fmt.Fprintf(&b, ".c%d { color: #012345; margin: 0px; padding: 1px; }\n", i)
	}
	return b.Bytes()
}
