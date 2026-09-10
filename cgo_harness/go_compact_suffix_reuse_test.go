//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"bytes"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestGoCompactSuffixReuseUnderFragileFunctionLockedC(t *testing.T) {
	source := []byte("package p\nfunc f(x,y int){a:=1;_=a}\n")
	edited := bytes.Replace(source, []byte("func f"), []byte("func g"), 1)
	start := bytes.Index(source, []byte("f("))
	edit := canonicalGoInputEdit(source, edited, start, start+1, start+1)
	lang := canonicalIncrementalGoLanguage(t, "go")
	p := gts.NewParser(lang)
	p.SetAdmissionCandidateRoute(true)
	old, err := p.Parse(source)
	requireCanonicalGoIncrementalTree(t, old, source, "initial", err)
	candidates := map[*gts.Node]bool{}
	var collect func(*gts.Node)
	collect = func(n *gts.Node) {
		if n.ChildCount() > 0 {
			candidates[n] = true
		}
		for i := 0; i < n.ChildCount(); i++ {
			collect(n.Child(i))
		}
	}
	function := findGoNodeByTypeAndStart(old.RootNode(), lang, "function_declaration", uint32(bytes.Index(source, []byte("func"))))
	if function == nil {
		old.Release()
		t.Fatal("missing function fixture")
	}
	collect(function)
	before := compactNativeLegacyEntries(t, p)
	old.Edit(edit)
	next, profile, err := p.ParseIncrementalProfiled(edited, old)
	old.Release()
	requireCanonicalGoIncrementalTree(t, next, edited, "incremental", err)
	defer next.Release()
	if compactNativeLegacyEntries(t, p) != before || !next.ParseRuntime().CompactIncrementalReuseRoute || profile.ReusedBytes == 0 {
		t.Fatalf("native reuse missing: %+v", profile)
	}
	borrowed := 0
	var check func(*gts.Node)
	check = func(n *gts.Node) {
		if candidates[n] {
			borrowed++
			t.Logf("borrowed %s %d:%d", n.Type(lang), n.StartByte(), n.EndByte())
		}
		for i := 0; i < n.ChildCount(); i++ {
			check(n.Child(i))
		}
	}
	check(next.RootNode())
	if borrowed == 0 {
		t.Fatal("no retained nonterminal identity")
	}
	verifyCompactNativeParentLinks(t, next.RootNode())
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(canonicalIncrementalCLanguage(t, "go")); err != nil {
		t.Fatal(err)
	}
	fresh := cp.Parse(edited, nil)
	if fresh == nil {
		t.Fatal("nil C tree")
	}
	defer fresh.Close()
	assertLockedCTreeExact(t, "fragile function suffix", next, lang, fresh)
}
