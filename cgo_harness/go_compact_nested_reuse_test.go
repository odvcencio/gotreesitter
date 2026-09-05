//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"bytes"
	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
	"testing"
)

func TestGoCompactNestedReuseLockedC(t *testing.T) {
	source := []byte("func a(){_=1}")
	edited := []byte("func a(){_=x}")
	edit := gts.InputEdit{StartByte: 11, OldEndByte: 12, NewEndByte: 12, StartPoint: pointAtOffset(source, 11), OldEndPoint: pointAtOffset(source, 12), NewEndPoint: pointAtOffset(edited, 12)}
	lang := grammars.GoLanguage()
	p := gts.NewParser(lang)
	p.SetAdmissionCandidateRoute(true)
	old, err := p.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	parameters := findGoNodeByTypeAndStart(old.RootNode(), lang, "parameter_list", 6)
	if parameters == nil || parameters.EndByte() != 8 {
		old.Release()
		t.Fatal("missing parameter list at bytes 6..8")
	}
	old.Edit(edit)
	next, err := p.ParseIncremental(edited, old)
	old.Release()
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release()
	cl, err := ParityCLanguage("go")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	co := cp.Parse(source, nil)
	if co == nil {
		t.Fatal("C old tree is nil")
	}
	ce := realCorpusCInputEdit(edit)
	co.Edit(&ce)
	ci := cp.Parse(edited, co)
	co.Close()
	cf := cp.Parse(edited, nil)
	if ci == nil || cf == nil {
		t.Fatal("C result is nil")
	}
	defer ci.Close()
	defer cf.Close()
	for _, oracle := range []*sitter.Tree{cf, ci} {
		assertLockedCTreeExact(t, "nested parameter list", next, lang, oracle)
	}
	runtime := next.ParseRuntime()
	if !runtime.CompactIncrementalReuseRoute || runtime.CompactIncrementalReusedSubtrees < 1 || runtime.CompactIncrementalReusedBytes < 2 {
		t.Fatalf("missing compact nonterminal reuse: decline=%q", runtime.CompactIncrementalFallbackReason)
	}
	got := findGoNodeByTypeAndStart(next.RootNode(), lang, "parameter_list", 6)
	if got != parameters || got.Parent() != next.RootNode().Child(0) || got.Text(edited) != "()" {
		t.Fatal("borrowed parameter list lost its identity or navigation")
	}
}

func TestGoCompactNestedReuseBoundaryLockedC(t *testing.T) {
	for _, tc := range []struct {
		name, source, before, after string
	}{
		{"right_boundary", "func a(){_=1}", "){", ")int{"},
		{"leading_identifier_width", "func a(){_=1}", "a", "longName"},
		{"inside_parameters", "func a(){_=1}", "()", "(x int)"},
		{"parameter_separator", "func a(x int){_=1}", "x int", "x,y int"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := []byte(tc.source)
			start := bytes.Index(source, []byte(tc.before))
			edited := bytes.Replace(source, []byte(tc.before), []byte(tc.after), 1)
			edit := gts.InputEdit{StartByte: uint32(start), OldEndByte: uint32(start + len(tc.before)), NewEndByte: uint32(start + len(tc.after)), StartPoint: pointAtOffset(source, start), OldEndPoint: pointAtOffset(source, start+len(tc.before)), NewEndPoint: pointAtOffset(edited, start+len(tc.after))}
			if tc.name == "right_boundary" {
				// Insert at the exact end of the old parameter list.
				edit = gts.InputEdit{StartByte: 8, OldEndByte: 8, NewEndByte: 11, StartPoint: pointAtOffset(source, 8), OldEndPoint: pointAtOffset(source, 8), NewEndPoint: pointAtOffset(edited, 11)}
			}
			lang := grammars.GoLanguage()
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(true)
			old, err := p.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			parameters := findGoNodeByTypeAndStart(old.RootNode(), lang, "parameter_list", 6)
			if parameters == nil {
				old.Release()
				t.Fatal("old parameter list is absent")
			}
			old.Edit(edit)
			next, err := p.ParseIncremental(edited, old)
			old.Release()
			if err != nil {
				t.Fatal(err)
			}
			defer next.Release()
			cl, err := ParityCLanguage("go")
			if err != nil {
				t.Fatal(err)
			}
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cl); err != nil {
				t.Fatal(err)
			}
			co := cp.Parse(source, nil)
			if co == nil {
				t.Fatal("C old tree is nil")
			}
			ce := realCorpusCInputEdit(edit)
			co.Edit(&ce)
			ci := cp.Parse(edited, co)
			co.Close()
			cf := cp.Parse(edited, nil)
			if ci == nil || cf == nil {
				t.Fatal("C result is nil")
			}
			defer ci.Close()
			defer cf.Close()
			for _, oracle := range []*sitter.Tree{cf, ci} {
				assertLockedCTreeExact(t, tc.name, next, lang, oracle)
			}
			var contains func(*gts.Node) bool
			contains = func(n *gts.Node) bool {
				if n == parameters {
					return true
				}
				for i := 0; i < n.ChildCount(); i++ {
					if contains(n.Child(i)) {
						return true
					}
				}
				return false
			}
			// A wider preceding name can preserve the untouched parameter list.
			if tc.name != "leading_identifier_width" && contains(next.RootNode()) {
				t.Fatal("edited parameter boundary reused the old parameter list")
			}
		})
	}
}

func TestGoCompactNestedReuseChangedLookaheadLockedC(t *testing.T) {
	for _, prefix := range []string{"", "package p\n"} {
		source := []byte(prefix + "var x = a + b ;")
		edited := bytes.Replace(source, []byte(";"), []byte("* c ;"), 1)
		start := len(source) - 1
		edit := gts.InputEdit{StartByte: uint32(start), OldEndByte: uint32(start + 1), NewEndByte: uint32(start + 5), StartPoint: pointAtOffset(source, start), OldEndPoint: pointAtOffset(source, start+1), NewEndPoint: pointAtOffset(edited, start+5)}
		lang := grammars.GoLanguage()
		p := gts.NewParser(lang)
		p.SetAdmissionCandidateRoute(true)
		old, err := p.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		old.Edit(edit)
		next, err := p.ParseIncremental(edited, old)
		old.Release()
		if err != nil {
			t.Fatal(err)
		}
		defer next.Release()
		cl, err := ParityCLanguage("go")
		if err != nil {
			t.Fatal(err)
		}
		cp := sitter.NewParser()
		defer cp.Close()
		if err := cp.SetLanguage(cl); err != nil {
			t.Fatal(err)
		}
		co := cp.Parse(source, nil)
		if co == nil {
			t.Fatal("C old tree is nil")
		}
		ce := realCorpusCInputEdit(edit)
		co.Edit(&ce)
		ci := cp.Parse(edited, co)
		co.Close()
		cf := cp.Parse(edited, nil)
		if ci == nil || cf == nil {
			t.Fatal("C result is nil")
		}
		defer ci.Close()
		defer cf.Close()
		for _, oracle := range []*sitter.Tree{cf, ci} {
			assertLockedCTreeExact(t, "changed lookahead after whitespace", next, lang, oracle)
		}
	}
}
