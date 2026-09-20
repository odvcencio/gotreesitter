package grammarruntime

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestExternalLexStatesRegisteredExactlyOnce statically scans every source
// file in this package for RegisterExternalLexStates("name", ...) calls and
// confirms each language name is registered exactly once. A grammar with a
// hand-edited support_<lang>.go that duplicates the call its
// <lang>_external_lex_states_gen.go sidecar already makes (as yaml, bash,
// and python once did) would register the same table twice; a grammar with
// a sidecar file but no registration call at all would silently carry an
// empty external lex states table. Both are static wiring bugs this test
// catches without needing to run any parser.
func TestExternalLexStatesRegisteredExactlyOnce(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed to resolve this test file's path")
	}
	dir := filepath.Dir(thisFile)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}

	registrationCount := map[string]int{}
	sidecarLanguages := map[string]bool{}

	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if strings.HasSuffix(name, "_external_lex_states_gen.go") {
			lang := strings.TrimSuffix(name, "_external_lex_states_gen.go")
			sidecarLanguages[lang] = true
		}

		path := filepath.Join(dir, name)
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			fn, ok := call.Fun.(*ast.Ident)
			if !ok || fn.Name != "RegisterExternalLexStates" || len(call.Args) == 0 {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			langName, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatalf("%s: unquote RegisterExternalLexStates argument %s: %v", path, lit.Value, err)
			}
			registrationCount[langName]++
			return true
		})
	}

	for lang, count := range registrationCount {
		if count > 1 {
			t.Errorf("RegisterExternalLexStates(%q, ...) called %d times; want exactly 1", lang, count)
		}
	}
	for lang := range sidecarLanguages {
		if registrationCount[lang] == 0 {
			t.Errorf("%s_external_lex_states_gen.go exists but nothing calls RegisterExternalLexStates(%q, ...)", lang, lang)
		}
	}
}
