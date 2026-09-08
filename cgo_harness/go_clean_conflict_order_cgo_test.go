//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestGoCleanConflictOrderLockedC(t *testing.T) {
	runGoCleanConflictOrderLockedC(t, false)
}

func runGoCleanConflictOrderLockedC(t *testing.T, requireNativeGeneric bool) {
	full, err := os.ReadFile("../testdata/incremental_gate/go_print.go")
	if err != nil {
		t.Fatal(err)
	}
	fixtures := []struct {
		name   string
		source []byte
	}{
		{"minimal", []byte("package p\nfunc f(){ g(reflect.ValueOf(v)) }\n")},
		{"generic_minimal", []byte("package p;var _=F[int](a)\n")},
		{"generic_nested", []byte("package p;var _=F[F[int]](a)\n")},
		{"generic_instantiation", []byte("package p\n\ntype Foo[T any] struct {\n\tV T\n}\n\nfunc f() {\n\ta := Foo[int]{}\n\tb := Foo[int](a)\n\t_ = a\n\t_ = b\n}\n")},
		{"full", full},
	}
	for _, member := range benchfixtures.GoGenericConflictFamily() {
		fixtures = append(fixtures, struct {
			name   string
			source []byte
		}{"family_" + member.Name, member.Source()})
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(loadCanonicalGoCLanguage(t)); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(fixture.source, nil)
			if cTree == nil {
				t.Fatal("C returned no tree")
			}
			defer cTree.Close()
			if cTree.RootNode().HasError() {
				t.Fatal("C fixture has errors")
			}
			if fixture.name == "minimal" {
				t.Logf("C tree: %s", cTree.RootNode().ToSexp())
			}
			for _, route := range []string{"production", "compact", "forest"} {
				t.Run(route, func(t *testing.T) {
					language := grammars.GoLanguage()
					parser := gts.NewParser(language)
					parser.SetAdmissionCandidateRoute(route == "compact")
					routedBefore, fallbackBefore := gts.AdmissionCandidateCounters()
					var tree *gts.Tree
					if route == "forest" {
						var ok bool
						tree, ok = parser.ParseForestExperimental(fixture.source)
						if tree != nil {
							defer tree.Release()
						}
						if !ok {
							t.Fatal("forest parse declined")
						}
					} else {
						var parseErr error
						tree, parseErr = parser.Parse(fixture.source)
						if tree != nil {
							defer tree.Release()
						}
						if parseErr != nil {
							t.Fatal(parseErr)
						}
					}
					if route == "compact" {
						routedAfter, fallbackAfter := gts.AdmissionCandidateCounters()
						t.Logf("compact routed=%d fallback=%d", routedAfter-routedBefore, fallbackAfter-fallbackBefore)
						if fallbackAfter != fallbackBefore {
							t.Logf("compact fallback: %s", gts.AdmissionCandidateLastFallbackReason())
						}
						if routedAfter-routedBefore+fallbackAfter-fallbackBefore != 1 {
							t.Fatal("compact parse must report one route decision")
						}
						if requireNativeGeneric && (fixture.name == "generic_minimal" || fixture.name == "generic_nested") && (routedAfter-routedBefore != 1 || fallbackAfter != fallbackBefore) {
							t.Fatal("generic parse must use native compact execution")
						}
					}
					if tree == nil || tree.RootNode() == nil {
						t.Fatal("Go returned no tree")
					}
					if tree.ParseRuntime().CRecoveryEnteredErrorState {
						t.Fatal("clean conflict fixture entered recovery")
					}
					assertLockedCTreeExact(t, fixture.name+" "+route, tree, language, cTree)
					if route != "forest" && (fixture.name == "generic_minimal" || fixture.name == "generic_nested") {
						at := bytes.LastIndex(fixture.source, []byte("(a)")) + 1
						if at == 0 {
							t.Fatal("generic fixture has no argument edit site")
						}
						edited := bytes.Clone(fixture.source)
						edited[at] = 'b'
						edit := gts.InputEdit{
							StartByte: uint32(at), OldEndByte: uint32(at + 1), NewEndByte: uint32(at + 1),
							StartPoint:  pointAtOffset(fixture.source, at),
							OldEndPoint: pointAtOffset(fixture.source, at+1), NewEndPoint: pointAtOffset(edited, at+1),
						}
						tree.Edit(edit)
						next, profile, err := parser.ParseIncrementalProfiled(edited, tree)
						if next != nil && next != tree {
							defer next.Release()
						}
						if err != nil || next == nil {
							t.Fatalf("incremental generic parse: %v", err)
						}
						cOld := cParser.Parse(fixture.source, nil)
						if cOld == nil {
							t.Fatal("C returned no edit baseline")
						}
						defer cOld.Close()
						cEdit := realCorpusCInputEdit(edit)
						cOld.Edit(&cEdit)
						cNext := cParser.Parse(edited, cOld)
						if cNext == nil {
							t.Fatal("C returned no incremental tree")
						}
						defer cNext.Close()
						cFresh := cParser.Parse(edited, nil)
						if cFresh == nil {
							t.Fatal("C returned no fresh edited tree")
						}
						defer cFresh.Close()
						assertLockedCTreeExact(t, "generic incremental C", next, language, cNext)
						assertLockedCTreeExact(t, "generic fresh C", next, language, cFresh)
						t.Logf("generic edit reused=%d unsupported=%t reason=%s", profile.ReusedSubtrees, profile.ReuseUnsupported, profile.ReuseUnsupportedReason)
					}
				})
			}
		})
	}
}
