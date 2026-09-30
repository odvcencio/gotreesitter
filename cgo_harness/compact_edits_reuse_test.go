//go:build cgo && treesitter_c_parity && gts_engine_ceiling

package cgoharness

import (
	"bytes"
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Run one language per process. Every direction compares full public trees,
// including fields, flags and points, rather than only their S-expressions.
func TestCompactEditsReuse(t *testing.T) {
	name := ceilingLanguage(t)
	runCompactEditsReuseInputs(t, name, ceilingInputs(t))
}

func runCompactEditsReuseInputs(t *testing.T, name string, inputs []ceilingInput) {
	lang := grammars.DetectLanguageByName(name).Language()
	cl, err := COracleLanguage(name)
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	for _, input := range inputs {
		if input.mode == "fresh" {
			continue
		}
		t.Run(input.size+"/"+input.mode, func(t *testing.T) {
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(true)
			gts.ResetAdmissionCandidateCounters()
			tree, err := p.Parse(input.source[0])
			ceilingGoTree(t, tree, input.source[0], err)
			defer func() { tree.Release() }()
			served, declined := gts.AdmissionCandidateCounters()
			if served != 1 || declined != 0 {
				t.Fatalf("initial compact declined: %s", gts.AdmissionCandidateLastFallbackReason())
			}
			for step := 0; step < 4; step++ {
				direction := step % 2
				source := input.source[1-direction]
				cReuseBeginWorkCount()
				tree.Edit(input.edit[direction])
				next, profile, err := p.ParseIncrementalProfiled(source, tree)
				cReuseEndWorkCount(t)
				ceilingGoTree(t, next, source, err)
				tree.Release()
				tree = next
				fresh, err := p.Parse(source)
				ceilingGoTree(t, fresh, source, err)
				ct := cp.Parse(source, nil)
				if ct == nil {
					t.Fatal("C returned no tree")
				}
				got, err := benchfixtures.InspectGoTree(next.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				want, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				oracle, err := COracleDeepDigest(ct)
				if err != nil {
					t.Fatal(err)
				}
				if got.SHA256 != want.SHA256 || got.SHA256 != oracle {
					t.Logf("first difference: %s", compactEditsFirstDifference(next.RootNode(), fresh.RootNode(), lang, "root"))
					fresh.Release()
					ct.Close()
					t.Fatalf("step=%d incremental=%s fresh=%s C=%s", step, got.SHA256, want.SHA256, oracle)
				}
				fresh.Release()
				ct.Close()
				rt := next.ParseRuntime()
				if !rt.CompactIncrementalReuseRoute || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
					t.Fatalf("step=%d compact declined: %s profile=%+v", step, rt.CompactIncrementalFallbackReason, profile)
				}
				t.Logf("COUNTERS %s/%s/%s step=%d tokens=%d nodes=%d reused=%d/%d digest=%s", name, input.size, input.mode, step, profile.TokensConsumed, profile.NewNodesAllocated, profile.ReusedSubtrees, profile.ReusedBytes, got.SHA256)
			}
			allocations := testing.AllocsPerRun(3, func() {
				next, err := p.ParseIncremental(input.source[0], tree)
				if err != nil {
					panic(err)
				}
				next.Release()
			})
			if allocations != 0 {
				t.Fatal(fmt.Sprintf("no-edit allocations=%g", allocations))
			}
		})
	}
}

// Raw delimiters, multiline strings and indentation exercise nonempty scanner
// snapshots; Rust documentation probes also exercise prefix-read abstention.
func TestCompactEditsScannerStates(t *testing.T) {
	name := ceilingLanguage(t)
	units := map[string]string{
		"python": "def item():\n    text = \"\"\"long\nstring\"\"\"\n    if text:\n        return text\n    return \"\"\n",
		"rust":   "/// item documentation\nfn item() -> &'static str { r##\"long\nstring\"## }\n",
		"cpp":    "const char *item() { return R\"delimiter(long\nstring)delimiter\"; }\n",
	}
	unit, ok := units[name]
	if !ok {
		t.Fatalf("scanner fixture unavailable for %s", name)
	}
	source := bytes.Repeat([]byte(unit), 512)
	at := len(source)/2 + bytes.Index([]byte(unit), []byte("item"))
	runCompactEditsReuseInputs(t, name, ceilingEditedInputs(name, "scanner", source, at))
}

// The fixed 72-step session includes syntax errors. Both the incremental
// result and the fresh candidate result must equal the fresh locked C tree.
func TestCompactEditsSession(t *testing.T) {
	name := ceilingLanguage(t)
	lang := grammars.DetectLanguageByName(name).Language()
	cl, err := COracleLanguage(name)
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	for _, input := range ceilingInputs(t) {
		if input.mode != "fresh" {
			continue
		}
		t.Run(input.size, func(t *testing.T) {
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(true)
			tree, err := p.Parse(input.source[0])
			ceilingGoTree(t, tree, input.source[0], err)
			defer func() { tree.Release() }()
			for step, edit := range benchfixtures.EditingSession(input.source[0]) {
				tree.Edit(edit.Edit)
				next, profile, err := p.ParseIncrementalProfiled(edit.Source, tree)
				ceilingGoTree(t, next, edit.Source, err)
				tree.Release()
				tree = next
				fresh, err := p.Parse(edit.Source)
				ceilingGoTree(t, fresh, edit.Source, err)
				ct := cp.Parse(edit.Source, nil)
				got, err := benchfixtures.InspectGoTree(next.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				want, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				oracle, err := COracleDeepDigest(ct)
				if err != nil {
					t.Fatal(err)
				}
				fresh.Release()
				ct.Close()
				if got.SHA256 != want.SHA256 {
					t.Fatalf("step=%d incremental=%s fresh=%s C=%s tokens=%d reused=%d/%d", step+1, got.SHA256, want.SHA256, oracle, profile.TokensConsumed, profile.ReusedSubtrees, profile.ReusedBytes)
				}
				if got.SHA256 != oracle {
					// Keep the C gate failing while checking D8 at the later
					// steps, rather than hiding an incremental divergence
					// behind an earlier fresh-C recovery discrepancy.
					t.Errorf("step=%d fresh-C mismatch incremental=%s fresh=%s C=%s", step+1, got.SHA256, want.SHA256, oracle)
				}
			}
		})
	}
}

func compactEditsFirstDifference(a, b *gts.Node, lang *gts.Language, path string) string {
	if a == nil || b == nil {
		if a != b {
			return path + ": nil mismatch"
		}
		return ""
	}
	describe := func(n *gts.Node) string {
		return fmt.Sprintf("%s %d..%d %+v..%+v named=%t extra=%t missing=%t error=%t/%t children=%d", n.Type(lang), n.StartByte(), n.EndByte(), n.StartPoint(), n.EndPoint(), n.IsNamed(), n.IsExtra(), n.IsMissing(), n.IsError(), n.HasError(), n.ChildCount())
	}
	if x, y := describe(a), describe(b); x != y {
		return fmt.Sprintf("%s: incremental=%s fresh=%s", path, x, y)
	}
	for i := 0; i < a.ChildCount(); i++ {
		if x, y := a.FieldNameForChild(i, lang), b.FieldNameForChild(i, lang); x != y {
			return fmt.Sprintf("%s[%d]: field %q != %q", path, i, x, y)
		}
		if diff := compactEditsFirstDifference(a.Child(i), b.Child(i), lang, fmt.Sprintf("%s[%d]", path, i)); diff != "" {
			return diff
		}
	}
	return ""
}
