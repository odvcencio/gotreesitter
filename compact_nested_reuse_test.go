//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter

import "testing"

func TestCompactNestedReuseLifetime(t *testing.T) {
	p := newAdmissionCandidateGoParser(t)
	p.SetAdmissionCandidateRoute(true)
	source := []byte("func a(){_=1}")
	old, err := p.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { old.Release() }()
	parameters := compactExecutionNode(old.RootNode(), p.language, "parameter_list", 6)
	if parameters == nil || parameters.EndByte() != 8 {
		t.Fatal("missing parameter list at bytes 6..8")
	}
	for _, replacement := range []string{"x", "2", "y"} {
		edited, edit := compactExecutionEdit(source, 11, 12, replacement)
		old.Edit(edit)
		runner := p.admissionCandidateRunner.(*parserCoreFreshFullRunner)
		legacy := runner.legacyParseRuns
		next, profile, err := p.ParseIncrementalProfiled(edited, old)
		if err != nil {
			t.Fatal(err)
		}
		old.Release()
		old = next
		runtime := next.ParseRuntime()
		if !runtime.CompactIncrementalReuseRoute || !next.compactMaterialized {
			t.Fatalf("nested reuse declined: replacement=%q reason=%q", replacement, runtime.CompactIncrementalFallbackReason)
		}
		if runner.legacyParseRuns != legacy {
			t.Fatal("nested compact reuse invoked the legacy parser")
		}
		got := compactExecutionNode(next.RootNode(), p.language, "parameter_list", 6)
		if got != parameters {
			t.Fatal("parameter list lost its identity after the old tree was released")
		}
		if got.Parent() != next.RootNode().Child(0) || got.Parent().Type(p.language) != "function_declaration" || got.Text(edited) != "()" {
			t.Fatal("borrowed parameter list lost its navigation or text")
		}
		if runtime.CompactIncrementalReusedSubtrees < 1 || runtime.CompactIncrementalReusedBytes < 2 || profile.ReusedSubtrees != runtime.CompactIncrementalReusedSubtrees || profile.ReusedBytes != runtime.CompactIncrementalReusedBytes {
			t.Fatalf("missing nonterminal reuse: runtime=%+v profile=%+v", runtime, profile)
		}
		copyTree := next.Copy()
		before := got.Range()
		_, copyEdit := compactExecutionEdit(edited, 6, 6, "x int")
		copyTree.Edit(copyEdit)
		if got.Range() != before || len(next.Edits()) != 0 {
			t.Fatal("editing a copy changed the borrowed parameter list")
		}
		copyTree.Release()
		source = edited
	}
}
