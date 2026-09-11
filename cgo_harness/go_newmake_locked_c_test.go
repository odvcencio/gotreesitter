//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestGoNewMakeLockedC(t *testing.T) {
	forms := []string{
		"new(pkg.Type)", "new(*T)", "new(*pkg.Type)", "new(**T)", "new(a.b)",
		"new((T))", "new((pkg.Type))", "new((*T))", "make(pkg.Type, 0)", "make(*T, 0)",
		"new(T)", "new(dirInfo)", "make(T)", "new(a, b)", "make(a, b, c)",
		"new([]T)", "new(*[]T)", "new([]*T)", "new(map[K]V)", "new(*map[K]V)",
		"new(chan T)", "new(*chan T)", "new(struct{})", "new(interface{})", "new(func())",
		"new([3]T)", "new(pkg.Type[int])", "make([]pkg.Type, 0)", "new(a.b.C)", "new(&T)",
	}
	for _, form := range forms {
		t.Run(form, func(t *testing.T) {
			source := []byte("package p\n\nfunc f() {\n\t_ = " + form + "\n}\n")
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(loadCanonicalGoCLanguage(t)); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil {
				t.Fatal("C returned no tree")
			}
			defer cTree.Close()
			wantError := form == "new(a.b.C)" || form == "new(&T)"
			if cTree.RootNode().HasError() != wantError {
				t.Fatalf("C root error=%v, want %v", cTree.RootNode().HasError(), wantError)
			}
			language := grammars.GoLanguage()
			for _, route := range []string{"compact", "production"} {
				t.Run(route, func(t *testing.T) {
					parser := gts.NewParser(language)
					parser.SetAdmissionCandidateRoute(route == "compact")
					beforeRoute, beforeFallback := gts.AdmissionCandidateCounters()
					tree, err := parser.Parse(source)
					afterRoute, afterFallback := gts.AdmissionCandidateCounters()
					if tree != nil {
						defer tree.Release()
					}
					if err != nil {
						t.Fatal(err)
					}
					if tree == nil || tree.RootNode() == nil {
						t.Fatal("Go returned no tree")
					}
					defer func() {
						if t.Failed() {
							runtime := tree.ParseRuntime()
							t.Logf("route=%d/%d reason=%q recovery=%v normalization checked=%d run=%d rewritten=%d", afterRoute-beforeRoute, afterFallback-beforeFallback, gts.AdmissionCandidateLastFallbackReason(), runtime.CRecoveryEnteredErrorState, runtime.NormalizationPassesChecked, runtime.NormalizationPassesRun, runtime.NormalizationNodesRewritten)
							t.Logf("C=%s\nGo=%s", cTree.RootNode().ToSexp(), tree.RootNode().SExpr(language))
						}
					}()
					if route == "compact" && (afterRoute-beforeRoute != 1 || afterFallback != beforeFallback) {
						t.Fatalf("compact route=%d/%d, want 1/0", afterRoute-beforeRoute, afterFallback-beforeFallback)
					}
					assertG18LockedCExact(t, form+" "+route, tree, language, cTree)
				})
			}
		})
	}
}
