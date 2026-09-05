//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

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

func TestReleaseTokenInvariantGoNumericLookaheadLockedC(t *testing.T) {
	lang := grammars.GoLanguage()
	assertExact := func(t *testing.T, label string, tree *gts.Tree, oracle *sitter.Tree) {
		t.Helper()
		if diff := FirstDivergenceDumpV1(tree.RootNode(), lang, oracle.RootNode()); diff != nil {
			t.Fatalf("%s node or field divergence: %+v", label, diff)
		}
		if diff := firstLockedCTreeFlagDivergence(tree.RootNode(), lang, oracle.RootNode(), "/"); diff != nil {
			t.Fatal(diff)
		}
		actual, err := benchfixtures.InspectGoTree(tree.RootNode(), lang)
		if err != nil {
			t.Fatal(err)
		}
		want, err := COracleDeepDigest(oracle)
		if err != nil {
			t.Fatal(err)
		}
		if actual.SHA256 != want {
			t.Fatalf("%s deep digest Go=%s C=%s", label, actual.SHA256, want)
		}
	}
	cl, err := ParityCLanguage("go")
	if err != nil {
		t.Fatal(err)
	}
	for _, compact := range []bool{false, true} {
		t.Run(fmt.Sprintf("compact_%t", compact), func(t *testing.T) {
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(compact)
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cl); err != nil {
				t.Fatal(err)
			}
			source := []byte("package p\nfunc f(){_=0eX}\n")
			offset := bytes.IndexByte(source, 'X')
			old, err := p.Parse(source)
			if err != nil || old == nil {
				t.Fatalf("initial Go parse: %v", err)
			}
			defer func() { old.Release() }()
			co := cp.Parse(source, nil)
			if co == nil {
				t.Fatal("initial C tree is nil")
			}
			defer func() { co.Close() }()
			assertExact(t, "initial numeric error", old, co)
			for epoch, replacement := range []byte{'1', 'X', '1'} {
				nextSource := bytes.Clone(source)
				nextSource[offset] = replacement
				edit := gts.InputEdit{
					StartByte: uint32(offset), OldEndByte: uint32(offset + 1), NewEndByte: uint32(offset + 1),
					StartPoint: pointAtOffset(source, offset), OldEndPoint: pointAtOffset(source, offset+1),
					NewEndPoint: pointAtOffset(nextSource, offset+1),
				}
				oldRoot := old.RootNode()
				old.Edit(edit)
				ce := realCorpusCInputEdit(edit)
				co.Edit(&ce)
				next, profile, err := p.ParseIncrementalProfiled(nextSource, old)
				if err != nil || next == nil {
					t.Fatalf("incremental Go parse: %v", err)
				}
				old.Release()
				old = next
				ci := cp.Parse(nextSource, co)
				if ci == nil {
					t.Fatal("incremental C tree is nil")
				}
				co.Close()
				co = ci
				func() {
					cf := cp.Parse(nextSource, nil)
					if cf == nil {
						t.Fatal("fresh C tree is nil")
					}
					defer cf.Close()
					fresh, err := p.Parse(nextSource)
					if err != nil || fresh == nil {
						t.Fatalf("fresh Go parse: %v", err)
					}
					defer fresh.Release()
					for _, oracle := range []*sitter.Tree{cf, ci} {
						assertExact(t, "incremental numeric lookahead", next, oracle)
						assertExact(t, "fresh numeric lookahead", fresh, oracle)
					}
					if next.RootNode().HasError() != (replacement == 'X') {
						t.Fatal("numeric edit did not change the error state")
					}
				}()
				if profile.ReparseNanos <= 0 || profile.NewNodesAllocated == 0 || next.RootNode() == oldRoot {
					t.Fatalf("epoch %d used the unsafe whole-tree shortcut: %+v", epoch, profile)
				}
				t.Logf("epoch=%d compact_reuse=%t reparse_ns=%d new_nodes=%d", epoch,
					next.ParseRuntime().CompactIncrementalReuseRoute, profile.ReparseNanos, profile.NewNodesAllocated)
				source = nextSource
			}
		})
	}
}
