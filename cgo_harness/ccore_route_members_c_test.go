//go:build cgo && treesitter_c_parity

package cgoharness

// Class members with modifiers, comments and generators, each wrapped as
// `class Box { <member> }`. A core tree is final (the present engine's
// normalization pass is skipped), so the route must give C's tree by itself:
// the public tree and the name, type_parameters and return_type fields are
// compared with C's.
//
//	CCORE_MEMBERS=1 go test -run '^TestCcoreRouteClassMembers$'

import (
	"fmt"
	"os"
	"testing"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
)

var ccoreClassMembers = []string{
	"private async *stream(value: string): AsyncGenerator<string> { yield value; }",
	"protected async *generic /* type docs */ <T>(value: T): AsyncGenerator<T> { yield value; }",
	"public static async *events() { yield 1; }",
	"private async*compact() { yield 1; }",
	"static async #pending() {}",
	"public async /* name docs */ commented() {}",
	"public async ordinary<T>(value: T): Promise<T> { return value; }",
	"public async () {}",
	"public get size () { return 1; }",
	"public set size (value: number) {}",
	"public value /* c */ (): void {}",
	"private x // c\n: number",
	"public async /* c */ () {}",
	"public isA(x: unknown): x is A { return true; }",
}

func TestCcoreRouteClassMembers(t *testing.T) {
	if os.Getenv("CCORE_MEMBERS") == "" {
		t.Skip("set CCORE_MEMBERS")
	}
	defer ccoreRouteCertify(t)()
	for _, name := range []string{"typescript", "tsx"} {
		entry := grammars.DetectLanguageByName(name)
		if entry == nil {
			t.Fatalf("no language %q", name)
		}
		lang := entry.Language()
		cLang, err := ParityCLanguage(name)
		if err != nil {
			t.Fatal(err)
		}
		equal, first := 0, ""
		for i, m := range ccoreClassMembers {
			src := []byte("class Box { " + m + " }")
			cTree := compactT3ParseC(t, cLang, src)
			_, f0, _ := bind.RouteCounters()
			p := g.NewParser(lang)
			p.SetAdmissionCandidateRoute(true)
			tree, err := p.Parse(src)
			if err != nil || tree == nil {
				t.Fatalf("%s member %d: %v", name, i, err)
			}
			_, f1, _ := bind.RouteCounters()
			divs := compactT3StructuralDivergences(tree.RootNode(), lang, cTree.RootNode())
			ok := f1 > f0 && len(divs) == 0
			if ok {
				fc := ccoreCompareFields(lang, tree.RootNode(), tree, cTree.RootNode())
				ok = fc.lookupDiffs == 0 && fc.stepDiffs == 0
				if !ok && first == "" {
					first = fmt.Sprintf("member %d fields: lookup %s; cursor %s", i, fc.firstLookup, fc.firstCursor)
				}
			} else if first == "" {
				if f1 <= f0 {
					first = fmt.Sprintf("member %d: not served by the core", i)
				} else {
					first = fmt.Sprintf("member %d: %s", i, compactT3FormatDivergence(divs[0]))
				}
			}
			if ok {
				equal++
			}
			tree.Release()
			cTree.Close()
		}
		fmt.Printf("CCMEMBERS lang=%s equal_to_c=%d of=%d first_difference=%q\n", name, equal, len(ccoreClassMembers), first)
		ccoreExpect(t, equal == len(ccoreClassMembers), "%s: %d of %d member fixtures equal C (first: %s)", name, equal, len(ccoreClassMembers), first)
	}
}
