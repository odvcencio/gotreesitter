package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestCppConsecutiveStorageClassSpecifiersProductionRouteMatchesCompact pins an
// issue #454 follow-up. Pull request #709 added a per-stack re-lex to the plain
// multi-stack dispatch loop. That re-lex let the doomed constructor-specifier
// fork of `static inline void f(...) {}` read `void` as an identifier, and the
// previous-shift recovery then kept that fork alive next to an error-free
// sibling. The recovered fork won result selection with an ERROR node.
// Tree-sitter C halts a version when a better one exists, so the production
// route must return the error-free tree and match the compact route.
func TestCppConsecutiveStorageClassSpecifiersProductionRouteMatchesCompact(t *testing.T) {
	lang := grammars.CppLanguage()
	if lang == nil {
		t.Skip("cpp grammar not registered")
	}
	for _, src := range []string{
		"static inline void f(int *v) {}\n",
		"extern inline void f(Foo<Rule> *v) {}\n",
		"static inline void f(std::vector<Rule> &v) {}\n",
		"static inline int g(std::vector<Rule> *v) { return 0; }\n",
		"#include <vector>\nstruct Rule {};\nstatic inline void f(std::vector<Rule> *v) {}\n",
	} {
		production := gts.NewParser(lang)
		production.SetAdmissionCandidateRoute(false)
		productionTree, err := production.Parse([]byte(src))
		if err != nil {
			t.Fatalf("production parse %q: %v", src, err)
		}
		compact := gts.NewParser(lang)
		compact.SetAdmissionCandidateRoute(true)
		compactTree, err := compact.Parse([]byte(src))
		if err != nil {
			t.Fatalf("compact parse %q: %v", src, err)
		}
		got := productionTree.RootNode().SExpr(lang)
		want := compactTree.RootNode().SExpr(lang)
		if productionTree.RootNode().HasError() {
			t.Errorf("production route reported an error for %q:\n  %s", src, got)
		}
		if got != want {
			t.Errorf("production route diverges from compact route for %q:\n  production: %s\n  compact:    %s", src, got, want)
		}
	}
}
