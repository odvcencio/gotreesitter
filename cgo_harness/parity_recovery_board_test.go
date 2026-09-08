//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// parityRecoveryCase is one malformed source for one language. The board
// parses it on the C oracle and on the Go routes and compares the trees
// node by node.
type parityRecoveryCase struct {
	lang   string
	source string
}

var parityRecoveryCases = []parityRecoveryCase{
	{"javascript", "let x = ;\n"},
	{"javascript", "foo(1 2);\n"},
	{"javascript", "let x = 1 +;\n"},
	{"javascript", "if (x {\n}\n"},
	{"javascript", "x = ;\n"},
	{"javascript", "function f( { return 1; }\n"},
	{"javascript", "let x = [1, 2;\n"},
	{"javascript", "a b c;\n"},
	{"javascript", "let x = 1;\n)\nlet y = 2;\n"},
	{"javascript", "let x = 1; @ let y = 2;\n"},
	{"javascript", "let x = 1;\n}\nlet y = 2;\n"},
	{"javascript", "class A { foo( { } }\n"},
	{"javascript", "const o = { a: 1, b };\nfunction g() {\n"},
	{"javascript", "for (let i = 0; i < 3 i++) {}\n"},
	{"javascript", "return 1;\n"},
	{"javascript", "x.\n"},
	{"python", "def f(:\n    return 1\n"},
	{"python", "x = \n"},
	{"python", "x = (1 +\n"},
	{"python", "return\n)\n"},
	{"python", "x = 1\n)\ny = 2\n"},
	{"python", "x = 1 +\ny = 2\n"},
	{"python", "def f():\n    return 1 2\n"},
	{"python", "x = [1, 2\ny = 3\n"},
	{"python", "if x\n    y = 1\n"},
	{"python", "class A:\n    def m(self:\n        pass\n"},
	{"python", "import\n"},
	{"python", "for i in range(3) print(i)\n"},
	{"python", "print(1, 2\n"},
	{"python", "x = {1: 2, 3}\n"},
	{"go", "package main\nfunc f() { x := }\n"},
	{"go", "package main\nfunc f( { }\n"},
	{"go", "package main\nvar x = 1 +\n"},
	{"go", "package main\nfunc f() {\n\tif x {\n}\n"},
	{"go", "package main\nfunc f() { return 1 2 }\n"},
	{"go", "package main\ntype T struct { a int b }\n"},
	{"go", "package main\nfunc f() { for i := 0; i < 3 i++ {} }\n"},
	{"go", "package main\nfunc f() { x := []int{1, 2 }\n"},
	{"go", "package main\n)\nfunc g() {}\n"},
	{"go", "package main\nfunc f() { a.b. }\n"},
	{"rust", "fn f() { let x = ; }\n"},
	{"rust", "fn f( { }\n"},
	{"rust", "fn f() { let x = 1 + }\n"},
	{"rust", "fn f() { if x { }\n"},
	{"rust", "fn f() { foo(1 2); }\n"},
	{"rust", "struct S { a: i32 b: i32 }\n"},
	{"rust", "fn f() { let v = vec![1, 2; }\n"},
	{"rust", "fn f() {}\n)\nfn g() {}\n"},
	{"rust", "fn f() { x. }\n"},
	{"rust", "impl S { fn m(&self { } }\n"},
	{"typescript", "let x: = 1;\n"},
	{"typescript", "function f(a: number { return a; }\n"},
	{"typescript", "interface I { a: number b: string }\n"},
	{"typescript", "let x = <T>(y;\n"},
	{"typescript", "class A { m(: number {} }\n"},
	{"typescript", "type T = { a: ;\n"},
	{"json", "{\"a\": 1,}\n"},
	{"json", "{\"a\" 1}\n"},
	{"json", "[1, 2,\n"},
	{"json", "{\"a\": [1, 2}\n"},
	{"json", "{\"a\": 1 \"b\": 2}\n"},
	{"json", "1 2\n"},
	{"c", "int f( { return 1; }\n"},
	{"c", "int x = ;\n"},
	{"c", "int f() { if (x { } }\n"},
	{"c", "int f() { return 1 2; }\n"},
	{"c", "struct S { int a int b; };\n"},
	{"c", "int f() { foo(1 2); }\n"},
	{"c", "int f() {\n"},
	{"c", "int f() { x = [1]; }\n"},
	{"java", "class A { void m( { } }\n"},
	{"java", "class A { int x = ; }\n"},
	{"java", "class A { void m() { if (x { } } }\n"},
	{"java", "class A { void m() { foo(1 2); } }\n"},
	{"java", "class A { int a int b; }\n"},
	{"java", "class A {\n"},
	{"java", "class A { void m() { return 1 2; } }\n"},
	{"java", "import\nclass A {}\n"},
}

// parityRecoveryRouteResult is one route's verdict on one case.
type parityRecoveryRouteResult struct {
	agree bool
	err   error
	diff  *DumpV1Divergence
}

func parityRecoveryParseRoute(tc parityRecoveryCase, src []byte, route *bool, cRoot *sitter.Node) parityRecoveryRouteResult {
	goTree, goLang, err := parseWithGo(parityCase{name: tc.lang, source: tc.source, candidateRoute: route}, src, nil)
	if err != nil {
		return parityRecoveryRouteResult{err: err}
	}
	defer releaseGoTree(goTree)
	diff := FirstDivergenceDumpV1(goTree.RootNode(), goLang, cRoot)
	return parityRecoveryRouteResult{agree: diff == nil, diff: diff}
}

// TestParityRecoveryBoard parses malformed sources on the C oracle and on
// the Go routes: the default route, the compact candidate route, and the
// production route. It reports per-language agreement counts. The board is
// informational; GTS_PARITY_RECOVERY_STRICT=1 fails it on any divergence
// of the default route.
func TestParityRecoveryBoard(t *testing.T) {
	strict := parityEnvBool("GTS_PARITY_RECOVERY_STRICT", false)
	type tally struct{ total, def, compact, production int }
	byLang := map[string]*tally{}
	var langs []string
	var divergent []string
	compact, production := true, false
	for _, tc := range parityRecoveryCases {
		src := []byte(tc.source)
		cLang, err := ParityCLanguage(tc.lang)
		if err != nil {
			if reason := parityReferenceSkipReason(err); reason != "" {
				t.Logf("%s: skip C reference: %s", tc.lang, reason)
				continue
			}
			t.Fatalf("%s: load C parser: %v", tc.lang, err)
		}
		cParser := sitter.NewParser()
		if err := cParser.SetLanguage(cLang); err != nil {
			cParser.Close()
			t.Fatalf("%s: C SetLanguage: %v", tc.lang, err)
		}
		cTree := cParser.Parse(src, nil)
		if cTree == nil {
			cParser.Close()
			t.Fatalf("%s: C parser returned nil tree", tc.lang)
		}
		cRoot := cTree.RootNode()
		if _, ok := byLang[tc.lang]; !ok {
			byLang[tc.lang] = &tally{}
			langs = append(langs, tc.lang)
		}
		tl := byLang[tc.lang]
		tl.total++
		def := parityRecoveryParseRoute(tc, src, nil, cRoot)
		comp := parityRecoveryParseRoute(tc, src, &compact, cRoot)
		prod := parityRecoveryParseRoute(tc, src, &production, cRoot)
		if def.agree {
			tl.def++
		}
		if comp.agree {
			tl.compact++
		}
		if prod.agree {
			tl.production++
		}
		if !def.agree {
			detail := ""
			if def.err != nil {
				detail = "parse error: " + def.err.Error()
			} else if def.diff != nil {
				detail = fmt.Sprintf("%+v", *def.diff)
			}
			divergent = append(divergent, fmt.Sprintf("%s %q compact=%t production=%t\n    C: %s\n    %s", tc.lang, tc.source, comp.agree, prod.agree, cRoot.ToSexp(), detail))
		}
		cTree.Close()
		cParser.Close()
	}
	sort.Strings(langs)
	var lines []string
	total, agree := 0, 0
	for _, lang := range langs {
		tl := byLang[lang]
		total += tl.total
		agree += tl.def
		gate := ""
		if entry, ok := parityEntriesByName[lang]; ok {
			diag := gotreesitter.DiagnoseCRecoveryGate(entry.Language())
			gate = fmt.Sprintf("%+v", diag)
		}
		lines = append(lines, fmt.Sprintf("%-11s cases=%2d default=%2d compact=%2d production=%2d gate=%s", lang, tl.total, tl.def, tl.compact, tl.production, gate))
	}
	t.Logf("recovery board: %d of %d malformed sources build the C tree on the default route\n%s", agree, total, strings.Join(lines, "\n"))
	for _, d := range divergent {
		t.Logf("diverge: %s", d)
	}
	if strict && len(divergent) > 0 {
		t.Fatalf("%d malformed sources diverge from C on the default route", len(divergent))
	}
}
