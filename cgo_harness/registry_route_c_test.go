//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/ccore/certified"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// registryRouteErrorInputs are small inputs with syntax errors for grammars
// whose registry entry has a token-source factory. The C inputs are the ones
// the runtime audit of 24 Nov 2026 probed.
var registryRouteErrorInputs = map[string][]string{
	"c": {
		"int main( { return 0; }\n",
		"a * b;\nT (x);\n",
		"void f() { x = ; y = 1 }\n",
		"#define X(a) a +\nint y = X(1) 2;\nstruct s { int a b; };\n",
		"int f(void) { if (a) { b(); else c(); }\n",
	},
	"authzed": {
		"definition document {\n\trelation viewer: \n}\n",
		"definition user {\ndefinition document {}\n",
		"definition document {\n\trelation viewer: user\n\tpermission view = viewer +\n}\n",
		"caveat c(x int) {\n\tx == \n}\n",
		"definition document {\n\trelation owner: user | \n\tpermission edit = owner->\n}\n",
	},
}

// registryRoutePinnedInputs are valid inputs whose tree a test in the
// grammars package pins for the registry route.
var registryRoutePinnedInputs = map[string][]string{
	"c": {
		"typedef int local_t;\nvoid f(void) { x = (local_t)(y); }\n", // TestParseFileCLocalTypedefCastStaysCastExpression
		"void f(int content_size) { x = (off_t)content_size; }\n",    // TestParseFileCHeaderTypeCastWithoutArgumentListStaysCastExpression
	},
}

// registryRouteEdit inserts one space before the first newline of src.
func registryRouteEdit(src []byte) ([]byte, g.InputEdit) {
	at := strings.IndexByte(string(src), '\n')
	if at < 0 {
		at = len(src)
	}
	out := append(append(append([]byte(nil), src[:at]...), ' '), src[at:]...)
	p := g.Point{Row: 0, Column: uint32(at)}
	return out, g.InputEdit{StartByte: uint32(at), OldEndByte: uint32(at), NewEndByte: uint32(at + 1),
		StartPoint: p, OldEndPoint: p, NewEndPoint: g.Point{Row: 0, Column: uint32(at + 1)}}
}

// registryRouteTotals is what one route did over the inputs of one grammar.
type registryRouteTotals struct {
	name                          string
	inputs, served, equalC, diffC int
	first                         string
	differing                     map[string]bool // input ids whose tree differs from C
}

// The registry routes compared with pinned C, input by input. For each grammar
// in REGISTRY_ROUTE_LANGS (default "c,authzed") the test parses the error
// inputs above and the grammar's certification inputs (gradInputs) through
// Parser.Parse and through the routes that use the entry's token-source factory,
// and compares every tree with the tree pinned C gives for the same bytes. The
// incremental routes reparse a one-byte edit of their own old tree and are
// compared with C's incremental reparse of the same edit. It prints one REGROUTE
// line per route. For a grammar whose certificate lists the table-v1 provider,
// every factory route must be served by the core and differ from C on no input
// where its built-in-lexer counterpart (Parser.Parse, Parser.ParseIncremental)
// does not.
func TestRegistryRoutesMatchPinnedC(t *testing.T) {
	langs := os.Getenv("REGISTRY_ROUTE_LANGS")
	if langs == "" {
		langs = "c,authzed"
	}
	for _, name := range strings.Split(langs, ",") {
		name = strings.TrimSpace(name)
		t.Run(name, func(t *testing.T) { registryRoutesAgainstC(t, name) })
	}
}

func registryRoutesAgainstC(t *testing.T, name string) {
	gl, cl := ccoreLanguages(t, name)
	entry := grammars.DetectLanguageByName(name)
	if entry == nil || entry.TokenSourceFactory == nil {
		t.Fatalf("%s has no registry token-source factory", name)
	}
	sum, _ := gl.GrammarBlobSHA256()
	cert, hasCert := certified.Blobs[sum]
	granted := hasCert && cert.HasProvider("table-v1")
	var inputs []ccoreInput
	for i, s := range registryRouteErrorInputs[name] {
		inputs = append(inputs, ccoreInput{ID: fmt.Sprintf("%s/error/%d", name, i+1), Src: []byte(s)})
	}
	for i, s := range registryRoutePinnedInputs[name] {
		inputs = append(inputs, ccoreInput{ID: fmt.Sprintf("%s/pinned/%d", name, i+1), Src: []byte(s)})
	}
	inputs = append(inputs, gradInputs(t, name)...)

	served := func() int64 {
		_, f, i := bind.RouteCounters()
		return f + i
	}
	filename := ""
	for _, ext := range entry.Extensions {
		if d := grammars.DetectLanguage("x" + ext); d != nil && d.Name == entry.Name {
			filename = "x" + ext
			break
		}
	}
	hl, err := entry.NewHighlighter()
	if err != nil {
		t.Fatal(err)
	}
	type route struct {
		name string
		incr bool
		run  func(src []byte) (*g.Node, func())
	}
	reparseServed := int64(0) // what the reparse of an incremental route alone was served
	reparse := func(f func() (*g.Tree, error)) (*g.Tree, error) {
		before := served()
		tr, err := f()
		reparseServed = served() - before
		return tr, err
	}
	treeOf := func(tr *g.Tree, err error) (*g.Node, func()) {
		if err != nil || tr == nil {
			t.Fatalf("parse: tree %v, error %v", tr != nil, err)
		}
		return tr.RootNode(), tr.Release
	}
	routes := []route{
		{name: "Parser.Parse", run: func(src []byte) (*g.Node, func()) { return treeOf(g.NewParser(gl).Parse(src)) }},
		{name: "Parser.ParseIncremental", incr: true, run: func(src []byte) (*g.Node, func()) {
			p := g.NewParser(gl)
			old, err := p.Parse(src)
			if err != nil || old == nil {
				t.Fatalf("old tree: %v", err)
			}
			defer old.Release()
			edited, e := registryRouteEdit(src)
			old.Edit(e)
			return treeOf(reparse(func() (*g.Tree, error) { return p.ParseIncremental(edited, old) }))
		}},
		{name: "LangEntry.Parse", run: func(src []byte) (*g.Node, func()) { return treeOf(entry.Parse(src)) }},
		{name: "LangEntry.ParseContext", run: func(src []byte) (*g.Node, func()) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			return treeOf(entry.ParseContext(ctx, src))
		}},
		{name: "grammars.ParseFile", run: func(src []byte) (*g.Node, func()) {
			bt, err := grammars.ParseFile(filename, src)
			if err != nil || bt == nil {
				t.Fatalf("ParseFile: %v", err)
			}
			return bt.RootNode(), bt.Release
		}},
		{name: "Highlighter.fresh", run: func(src []byte) (*g.Node, func()) {
			_, tr := hl.HighlightIncremental(src, nil)
			return treeOf(tr, nil)
		}},
		{name: "ParseIncrementalWithTokenSource", incr: true, run: func(src []byte) (*g.Node, func()) {
			p := g.NewParser(gl)
			old, err := p.ParseWithTokenSource(src, entry.TokenSourceFactory(src, gl))
			if err != nil || old == nil {
				t.Fatalf("old tree: %v", err)
			}
			defer old.Release()
			edited, e := registryRouteEdit(src)
			old.Edit(e)
			return treeOf(reparse(func() (*g.Tree, error) {
				return p.ParseIncrementalWithTokenSource(edited, old, entry.TokenSourceFactory(edited, gl))
			}))
		}},
	}
	totals := make([]*registryRouteTotals, len(routes))
	for i, r := range routes {
		totals[i] = &registryRouteTotals{name: r.name, differing: map[string]bool{}}
	}
	for _, in := range inputs {
		if len(in.Src) == 0 {
			continue // the highlighter parses no empty source
		}
		edited, edit := registryRouteEdit(in.Src)
		cFresh := compactT3ParseC(t, cl, in.Src)
		cEdited := registryRouteCIncremental(t, cl, in.Src, edited, edit)
		for i, r := range routes {
			tot := totals[i]
			before := served()
			root, release := r.run(in.Src)
			delta := served() - before
			c := cFresh
			if r.incr {
				c = cEdited
				delta = reparseServed // the route also parses its old tree
			}
			tot.inputs++
			if delta >= 1 {
				tot.served++
			}
			divs := compactT3StructuralDivergences(root, gl, c.RootNode())
			n := len(divs)
			why := ""
			if n > 0 {
				why = compactT3FormatDivergence(divs[0])
			} else if root.HasError() != c.RootNode().HasError() {
				n, why = 1, "HasError differs"
			}
			if n == 0 {
				tot.equalC++
			} else {
				tot.diffC++
				tot.differing[in.ID] = true
				if tot.first == "" {
					tot.first = fmt.Sprintf("%s: %d differences, first %s", in.ID, n, why)
				}
			}
			release()
		}
		cFresh.Close()
		cEdited.Close()
	}
	for _, tot := range totals {
		fmt.Printf("REGROUTE lang=%s granted=%t route=%s inputs=%d served=%d equal_c=%d differ_c=%d first=%q\n",
			name, granted, tot.name, tot.inputs, tot.served, tot.equalC, tot.diffC, tot.first)
	}
	if !granted {
		return
	}
	fresh, incr := totals[0], totals[1]
	for i, tot := range totals {
		if tot.served != tot.inputs {
			t.Errorf("%s/%s: core served %d of %d inputs", name, tot.name, tot.served, tot.inputs)
		}
		base := fresh
		if routes[i].incr {
			base = incr
		}
		for id := range tot.differing {
			if !base.differing[id] {
				t.Errorf("%s/%s: %s differs from pinned C where %s does not", name, tot.name, id, base.name)
			}
		}
	}
}

// registryRouteCIncremental is pinned C's incremental reparse of edited after
// the edit of src.
func registryRouteCIncremental(t *testing.T, cl *sitter.Language, src, edited []byte, e g.InputEdit) *sitter.Tree {
	t.Helper()
	p := sitter.NewParser()
	defer p.Close()
	if err := p.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	old := p.Parse(src, nil)
	if old == nil {
		t.Fatal("C oracle parse returned no tree")
	}
	defer old.Close()
	old.Edit(gradSitterEdit(e))
	tree := p.Parse(edited, old)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("C oracle incremental parse returned no root")
	}
	return tree
}
