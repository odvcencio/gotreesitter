//go:build cgo && treesitter_c_parity && gts_parsercorephase0

package cgoharness

import (
	"bytes"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammargen"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// C lexes Go's line terminators internally. Model that terminal surface with
// the owned grammar so the empty EOF alternative cannot replace a real newline.
func TestGoGeneratedNULPreservesHiddenNewlineLockedC(t *testing.T) {
	grammar := grammargen.GoGrammar()
	var inlineTerminators func(*grammargen.Rule) *grammargen.Rule
	inlineTerminators = func(rule *grammargen.Rule) *grammargen.Rule {
		if rule == nil {
			return nil
		}
		if rule.Kind == grammargen.RuleSymbol && rule.Value == "_automatic_semicolon" {
			return grammargen.Choice(grammargen.Pat(`\n`), grammargen.Str("\x00"))
		}
		copyRule := *rule
		copyRule.Children = make([]*grammargen.Rule, len(rule.Children))
		for i, child := range rule.Children {
			copyRule.Children[i] = inlineTerminators(child)
		}
		return &copyRule
	}
	for name, rule := range grammar.Rules {
		grammar.Rules[name] = inlineTerminators(rule)
	}
	grammar.Externals = nil
	lang, err := grammargen.GenerateLanguage(grammar)
	if err != nil {
		t.Fatal(err)
	}
	cLang, err := ParityCLanguage("go")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err = cp.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	sources := []string{
		"package main\n\nfunc f(a int, b int) int {\n\tx := a + b\n\treturn x\n}\n",
		"package main\n\nfunc f() {\n\treturn\n}\n",
		"package main\n\nfunc f() {\n\treturn;\n}\n",
	}
	for _, compact := range []bool{false, true} {
		name := "default"
		if compact {
			name = "candidate"
		}
		t.Run(name, func(t *testing.T) {
			for _, source := range sources {
				data := []byte(source)
				parser := gts.NewParser(lang)
				parser.SetAdmissionCandidateRoute(compact)
				tree, err := parser.Parse(data)
				if err != nil {
					t.Fatal(err)
				}
				if tree == nil {
					t.Fatal("nil generated Go tree")
				}
				ct := cp.Parse(data, nil)
				if ct == nil {
					tree.Release()
					t.Fatal("nil C tree")
				}
				assertLockedCTreeExact(t, "generated Go newline ownership", tree, lang, ct)
				ct.Close()
				var unchanged *gts.Tree
				var noEditErr error
				allocs := testing.AllocsPerRun(10, func() { unchanged, noEditErr = parser.ParseIncremental(data, tree) })
				if noEditErr != nil || unchanged != tree || allocs != 0 {
					tree.Release()
					t.Fatalf("no-edit parse: same=%v allocs=%g err=%v", unchanged == tree, allocs, noEditErr)
				}
				if marker := bytes.Index(data, []byte("return x")); marker >= 0 {
					site := marker + len("return ")
					nextSource := append([]byte(nil), data...)
					nextSource[site] = 'y'
					point := func(pos int) gts.Point {
						var p gts.Point
						for _, b := range data[:pos] {
							if b == '\n' {
								p.Row++
								p.Column = 0
							} else {
								p.Column++
							}
						}
						return p
					}
					tree.Edit(gts.InputEdit{StartByte: uint32(site), OldEndByte: uint32(site + 1), NewEndByte: uint32(site + 1), StartPoint: point(site), OldEndPoint: point(site + 1), NewEndPoint: point(site + 1)})
					next, err := parser.ParseIncremental(nextSource, tree)
					if err != nil {
						tree.Release()
						t.Fatal(err)
					}
					if next == nil {
						tree.Release()
						t.Fatal("nil incremental tree")
					}
					if next != tree {
						tree.Release()
					}
					tree = next
					cFresh := cp.Parse(nextSource, nil)
					if cFresh == nil {
						tree.Release()
						t.Fatal("nil edited C tree")
					}
					assertLockedCTreeExact(t, "incremental generated Go newline ownership", tree, lang, cFresh)
					freshParser := gts.NewParser(lang)
					freshParser.SetAdmissionCandidateRoute(compact)
					fresh, err := freshParser.Parse(nextSource)
					if err != nil {
						cFresh.Close()
						tree.Release()
						t.Fatal(err)
					}
					if fresh == nil {
						cFresh.Close()
						tree.Release()
						t.Fatal("nil independent fresh Go tree")
					}
					assertLockedCTreeExact(t, "independent fresh generated Go newline ownership", fresh, lang, cFresh)
					fresh.Release()
					cFresh.Close()
				}
				tree.Release()
			}
		})
	}
}
