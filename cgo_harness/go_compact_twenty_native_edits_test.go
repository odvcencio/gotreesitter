//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"reflect"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func compactNativeCanonicalCases(t testing.TB) []canonicalGoIncrementalCase {
	cases := loadCanonicalGoIncrementalCases(t)
	var selected []canonicalGoIncrementalCase
	for _, tc := range cases {
		if tc.spec.Language != "go" || tc.spec.Role != "representative" {
			continue
		}
		if tc.spec.Name != "token_class_change" && tc.spec.Name != "early_newline" && tc.spec.Name != "same_line_length_change" {
			continue
		}
		selected = append(selected, tc)
		if tc.spec.Name == "early_newline" {
			tc.spec.Name = "newline_prefix_call_conversion"
			tc.source = tc.source[:65953]
			tc.edited = append(append(append([]byte(nil), tc.source[:19]...), '\n'), tc.source[19:]...)
			tc.forward = canonicalGoInputEdit(tc.source, tc.edited, 19, 19, 20)
			tc.reverse = canonicalGoInputEdit(tc.edited, tc.source, 19, 20, 19)
			selected = append(selected, tc)
		}
	}
	return selected
}

// Read the existing private entry counter without adding a public parser API.
// Missing instrumentation is a test failure, not permission to infer native execution.
func compactNativeLegacyEntries(t testing.TB, p *gts.Parser) uint64 {
	t.Helper()
	runner := reflect.ValueOf(p).Elem().FieldByName("admissionCandidateRunner")
	if !runner.IsValid() || runner.IsNil() {
		t.Fatal("missing compact runner")
	}
	value := runner.Elem().Elem().FieldByName("legacyParseRuns")
	if !value.IsValid() || value.Kind() != reflect.Uint64 {
		t.Fatal("missing legacy entry counter")
	}
	return value.Uint()
}

func TestGoCompactTwentyNativeEditsLockedC(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "")
	gts.ResetParseEnvConfigCacheForTests()
	t.Cleanup(gts.ResetParseEnvConfigCacheForTests)
	for _, tc := range compactNativeCanonicalCases(t) {
		t.Run(tc.spec.Name, func(t *testing.T) {
			lang := canonicalIncrementalGoLanguage(t, "go")
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(true)
			tree, err := p.Parse(tc.source)
			requireCanonicalGoIncrementalTree(t, tree, tc.source, "initial", err)
			defer func() { releaseCanonicalGoTree(tree) }()
			materialized := reflect.ValueOf(tree).Elem().FieldByName("compactMaterialized")
			if !materialized.IsValid() || !materialized.Bool() {
				t.Fatal("initial tree was not compact-produced")
			}
			initialLegacy := compactNativeLegacyEntries(t, p)
			if initialLegacy != 0 {
				t.Fatalf("initial parse entered legacy %d times", initialLegacy)
			}
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(canonicalIncrementalCLanguage(t, "go")); err != nil {
				t.Fatal(err)
			}
			dirs := tc.directions()
			native := 0
			for i := 0; i < 20; i++ {
				d := dirs[i%2]
				beforeLegacy := compactNativeLegacyEntries(t, p)
				_, beforeFallback := gts.AdmissionCandidateCounters()
				tree.Edit(d.goEdit)
				next, profile, err := p.ParseIncrementalProfiled(d.to, tree)
				if next == tree {
					t.Fatal("changed edit returned unchanged tree")
				}
				tree.Release()
				tree = next
				requireCanonicalGoIncrementalTree(t, tree, d.to, "incremental", err)
				verifyCompactNativeParentLinks(t, tree.RootNode())
				// The new tree must remain usable after the old owner is released.
				oracle := cp.Parse(d.to, nil)
				requireCanonicalCIncrementalTree(t, oracle, d.to, "fresh C")
				got := canonicalGoTreeDigest(t, tree, lang, "incremental")
				want := canonicalCTreeDigest(t, oracle, "fresh C")
				oracle.Close()
				if got != want {
					t.Fatalf("edit=%d C mismatch: got=%s want=%s", i, got, want)
				}
				receipt := tree.ParseRuntime()
				legacy := compactNativeLegacyEntries(t, p) - beforeLegacy
				_, afterFallback := gts.AdmissionCandidateCounters()
				isNative := receipt.CompactIncrementalReuseRoute && !profile.ReuseUnsupported && legacy == 0 && afterFallback == beforeFallback
				if isNative {
					native++
				}
				t.Logf("edit=%d native=%t legacy_entries=%d fallback=%q reused_subtrees=%d reused_bytes=%d new_nodes=%d tokens=%d", i, isNative, legacy, receipt.CompactIncrementalFallbackReason, profile.ReusedSubtrees, profile.ReusedBytes, profile.NewNodesAllocated, profile.TokensConsumed)
				if !isNative || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 || !profile.OldTreeReuseRoute {
					t.Fatalf("required history did not execute natively at edit=%d", i)
				}
			}
			t.Logf("history=%s native_edits=%d total_edits=20", tc.spec.Name, native)
		})
	}
}

// Verify public navigation after the prior tree owner has been released.
func verifyCompactNativeParentLinks(t *testing.T, root *gts.Node) {
	t.Helper()
	if root.Parent() != nil {
		t.Fatal("result root has a parent")
	}
	stack := []*gts.Node{root}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for i := 0; i < n.ChildCount(); i++ {
			child := n.Child(i)
			if child == nil {
				t.Fatal("nil child inside declared child count")
			}
			if child.Parent() != n {
				t.Errorf("parent link points outside the returned tree: child=[%d,%d) parent=[%d,%d)", child.StartByte(), child.EndByte(), n.StartByte(), n.EndByte())
				return
			}
			stack = append(stack, child)
		}
	}
}
