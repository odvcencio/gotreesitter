//go:build cgo && treesitter_c_parity

package cgoharness

// The public API served by internal/ccore, against pinned C. The route is on
// (the per-parser candidate switch) and every bound language is certified by
// bind.CertifyForTests. Each language runs in its own process:
//
//	CCORE_LANG=<name> go test -run '^TestCcoreRoutePublic$'   one fresh parse of the R4 sample (or the smoke sample)
//	CCORE_INC_SET=<lang>-session go test -run '^TestCcoreRouteSession$'   the 72-step edit session
//	go test -run '^TestCcoreRouteList$'   prints the language names

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func ccoreRouteCertify(t testing.TB) func() {
	restore, err := bindtest.CertifyForTests(1|2|4|8, []string{"ImmediateTokens", "ZeroWidthTokens", "LayoutFallbackLexState", "AfterWhitespaceLexState"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	on := os.Getenv("CCORE_VIEWS") == "1"
	previous := bind.ViewsOn(on)
	trees0, made0, conversions0 := bind.ViewCounters()
	return func() {
		trees, made, conversions := bind.ViewCounters()
		arm := "eager"
		if on {
			arm = "views"
		}
		for reason, count := range conversions {
			conversions[reason] = count - conversions0[reason]
		}
		fmt.Printf("CCARM test=%s arm=%s trees=%d views=%d conversions=%v\n", t.Name(), arm, trees-trees0, made-made0, conversions)
		if on && trees <= trees0 {
			t.Errorf("%s: views arm made no view tree", t.Name())
		}
		bind.ViewsOn(previous)
		restore()
	}
}

func TestCcoreRouteList(t *testing.T) {
	if os.Getenv("CCORE_ROUTE_LIST") == "" {
		t.Skip("set CCORE_ROUTE_LIST")
	}
	for _, e := range grammars.AllLanguages() {
		if _, err := ParityCLanguage(e.Name); err == nil {
			fmt.Printf("ROUTELANG %s\n", e.Name)
		}
	}
}

// ccoreRouteSource returns the R4 sample of a language, or its smoke sample.
func ccoreRouteSource(t testing.TB, name string) ([]byte, string) {
	raw, err := os.ReadFile("../internal/benchfixtures/real_corpus.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Entries []struct {
			Language, Role, SHA256 string
			Bytes                  int
			Path                   string `json:"committed_path"`
		}
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, row := range manifest.Entries {
		if row.Language == name && row.Role == "sample" && row.Path != "" {
			src, err := os.ReadFile(filepath.Join("../internal/benchfixtures", row.Path))
			if err != nil {
				t.Fatal(err)
			}
			if len(src) != row.Bytes || fmt.Sprintf("%x", sha256.Sum256(src)) != row.SHA256 {
				t.Fatal("R4 sample identity changed")
			}
			return src, "r4"
		}
	}
	return []byte(grammars.ParseSmokeSample(name)), "smoke"
}

func TestCcoreRoutePublic(t *testing.T) {
	name := os.Getenv("CCORE_LANG")
	if name == "" {
		t.Skip("set CCORE_LANG")
	}
	defer ccoreRouteCertify(t)()
	src, kind := ccoreRouteSource(t, name)
	ccoreRouteOne(t, name, kind, src)
}

// ccoreRouteOne parses src through the route, the legacy default and the core
// itself, and compares each with C.
func ccoreRouteOne(t *testing.T, name, kind string, src []byte) {
	ccoreRouteCheck(t, name, kind, src, true)
}

// ccoreRouteCheck is ccoreRouteOne; with assert false it only reports.
var ccoreRouteAsserted int // inputs asserted equal to C in this process

func ccoreRouteCheck(t *testing.T, name, kind string, src []byte, assert bool) {
	if assert {
		ccoreRouteAsserted++
	}
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		t.Fatalf("no language %q", name)
	}
	lang := entry.Language()
	cLang, err := ParityCLanguage(name)
	if err != nil {
		t.Fatal(err)
	}
	cTree := compactT3ParseC(t, cLang, src)
	defer cTree.Close()
	_, f0, _ := bind.RouteCounters()
	p := g.NewParser(lang)
	p.SetAdmissionCandidateRoute(true)
	tree, err := p.Parse(src)
	if err != nil || tree == nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	decl, f1, _ := bind.RouteCounters()
	divs := compactT3StructuralDivergences(tree.RootNode(), lang, cTree.RootNode())
	root := tree.RootNode()
	lp := g.NewParser(lang)
	lt, lerr := lp.Parse(src)
	legacyDivs := -1
	if lerr == nil && lt != nil {
		legacyDivs = len(compactT3StructuralDivergences(lt.RootNode(), lang, cTree.RootNode()))
		lt.Release()
	}
	coreEq := "n/a"
	if len(divs) != 0 {
		core := ccoreNew(lang)
		if stop := core.Parse(src, nil); stop == "" {
			coreEq = fmt.Sprint(core.Digest() == ccoreCDigest(cTree))
		} else {
			coreEq = "stop:" + stop
		}
	}
	fmt.Printf("CCROUTE lang=%s input=%s bytes=%d served=%t equal=%t core_digest_equal_c=%s divergences=%d legacy_divergences=%d root_only=%v declines=%v root_end=%d has_error=%t\n",
		name, kind, len(src), f1 > f0, len(divs) == 0, coreEq, len(divs), legacyDivs, bind.RootOnlyFields(lang), decl, root.EndByte(), root.HasError())
	for i, d := range divs {
		if i < 5 {
			fmt.Printf("CCROUTEDIV %s %s\n", name, compactT3FormatDivergence(d))
		}
	}
	if len(divs) != 0 || legacyDivs != 0 {
		trunc := func(s string) string {
			if len(s) > 700 {
				return s[:700] + "..."
			}
			return s
		}
		fmt.Printf("CCTREE %s input=%s who=route %s\n", name, kind, trunc(tree.RootNode().SExpr(lang)))
		if lt2, err := g.NewParser(lang).Parse(src); err == nil && lt2 != nil {
			fmt.Printf("CCTREE %s input=%s who=legacy %s\n", name, kind, trunc(lt2.RootNode().SExpr(lang)))
			lt2.Release()
		}
		fmt.Printf("CCTREE %s input=%s who=c %s\n", name, kind, trunc(cTree.RootNode().ToSexp()))
	}
	if len(divs) == 0 {
		fc := ccoreCompareFields(lang, root, tree, cTree.RootNode())
		fmt.Printf("CCFIELDS lang=%s lookups=%d lookup_differs=%d cursor_steps=%d cursor_differs=%d first_lookup=%q first_cursor=%q\n",
			name, fc.lookups, fc.lookupDiffs, fc.steps, fc.stepDiffs, fc.firstLookup, fc.firstCursor)
		if assert {
			ccoreRequirePositive(t, name+" cursor steps", fc.steps)
			ccoreExpect(t, fc.steps > 0 && fc.lookupDiffs == 0 && fc.stepDiffs == 0, "%s: field lookups differ from C on %d of %d, cursor fields on %d of %d steps (first lookup %s; first cursor %s)",
				name, fc.lookupDiffs, fc.lookups, fc.stepDiffs, fc.steps, fc.firstLookup, fc.firstCursor)
		}
	}
	nodesChecked := 0
	var count func(n *g.Node)
	count = func(n *g.Node) {
		nodesChecked++
		for i := 0; i < n.ChildCount(); i++ {
			count(n.Child(i))
		}
	}
	count(tree.RootNode())
	if as := os.Getenv("CCORE_COUNT_AS"); as != "" {
		fmt.Printf("CCCOUNT test=%s set=%s checked=%d\n", as, name, nodesChecked)
	}
	if assert {
		ccoreExpect(t, f1 > f0, "%s: the core did not serve the parse (declines %v); the tree compared is the present engine's", name, decl)
		if len(divs) > 0 {
			t.Errorf("%s input=%s: %d divergences; first %s", name, kind, len(divs), compactT3FormatDivergence(divs[0]))
		}
	}
}

// TestCcoreRouteInputs runs inputs that root tests with the route on named as failing.
func TestCcoreRouteInputs(t *testing.T) {
	if os.Getenv("CCORE_ROUTE_INPUTS") == "" {
		t.Skip("set CCORE_ROUTE_INPUTS")
	}
	defer ccoreRouteCertify(t)()
	frag, err := os.ReadFile("../testdata/issue490_go_fragment.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	src := append([]byte("package p\n"), frag...)
	for i := 1; i < 8; i++ {
		src = append(append(src, '\n'), frag...)
	}
	// Compare every input with C.
	ccoreRouteCheck(t, "go", "issue490", src, true)
	ccoreRouteCheck(t, "go", "issue490-1x", append([]byte("package p\n"), frag...), true)
	for i, in := range []string{
		"package example\n\ntype Reader interface {\n    Read(dst []byte) (int, error)\n}\n",
		"package p\n\ntype R interface {\n    Read(dst []byte)\n}\n",
		"package p\n\nfunc f(dst []byte) (int, error) {}\n",
		"package p\n\nfunc f(a, b string) {}\n",
		"package p\n\nvar x []byte\n",
		"package p\n\ntype R interface{ Read() }\n",
	} {
		ccoreRouteCheck(t, "go", fmt.Sprintf("go-small-%d", i), []byte(in), true)
	}
	// The four inputs of root tests that were skipped because the route and the present engine disagree.
	ccoreRouteOne(t, "bash", "bash-clean-url-ret", []byte("url=`(curl -SsL https://registry.npmjs.org/npm/$t; echo \"\") \\\n     | sed -e 's/^.*tarball\":\"//' \\\n     | sed -e 's/\".*$//'`\n\nret=$?\n"))
	ccoreRouteOne(t, "rust", "rust-fn-main-open", []byte("fn main() { let x = 1; \n"))
	ccoreRouteOne(t, "rust", "rust-fn-main-closed", []byte("fn main() { let x = 1; }\n"))
	html, sites := ccoreHTMLCertificationSource(4<<10, true)
	ccoreRouteOne(t, "html", "html-recovered-original", html)
	ccoreRouteOne(t, "html", "html-recovered-edited", append(append(append([]byte(nil), html[:sites[1]]...), '9'), html[sites[1]:]...))
	jsSrc := ccoreJSFixture(24 << 10)
	mid := bytes.Index(jsSrc, []byte("x0"))
	brace := mid + bytes.IndexAny(jsSrc[mid:], "})]")
	for _, site := range []struct {
		name string
		at   int
	}{{"midfile", mid}, {"unbalanced", brace}} {
		ccoreRouteOne(t, "javascript", "javascript-"+site.name, append(append([]byte(nil), jsSrc[:site.at]...), jsSrc[site.at+1:]...))
	}
	ccoreRouteOne(t, "javascript", "javascript-original", jsSrc)
	ccoreRouteOne(t, "fidl", "fidl-versioned-layout", []byte("library test;\ntype Color = strict(removed=2) flexible(added=2) enum {\n    RED = 1;\n};\n"))
	ccoreRouteOne(t, "julia", "julia-trailing-comma", []byte("function f()\n    minarg, maxarg, = T_IFUNC[iidx]\nend\n"))
	for _, n := range []string{"missing-colon", "indentation", "unclosed-flow"} {
		ys, err := os.ReadFile("../testdata/yaml_issue1400/" + n + ".yaml")
		if err != nil {
			t.Fatal(err)
		}
		ccoreRouteCheck(t, "yaml", "yaml-"+n, ys, true)
	}
	defer func() {
		if os.Getenv("CCORE_COUNT_AS") != "" {
			fmt.Printf("CCCOUNT test=%s set=inputs checked=%d\n", os.Getenv("CCORE_COUNT_AS"), ccoreRouteAsserted)
		}
	}()
	ccoreRouteOne(t, "kdl", "kdl-package", []byte("package {\n    name kdl\n    version \"0.0.0\"\n    description \"The kdl document language\"\n    authors \"Kat Marchan\"\n    license-file LICENSE.md\n    edition \"2018\"\n}\n\ndependencies {\n    nom \"6.0.1\"\n    thiserror \"1.0.22\"\n}\n"))
}

// TestCcoreRouteGoUndo is the Go undo regression (140,026 bytes, byte 2959 E to Z and back)
// through the public API, step by step against C and against the public fresh parse.
func TestCcoreRouteGoUndo(t *testing.T) {
	if os.Getenv("CCORE_ROUTE_INPUTS") == "" {
		t.Skip("set CCORE_ROUTE_INPUTS")
	}
	defer ccoreRouteCertify(t)()
	zr, err := os.Open("../internal/benchfixtures/testdata/cliffs/go_elf.go.gz")
	if err != nil {
		t.Fatal(err)
	}
	gz, err := gzip.NewReader(zr)
	if err != nil {
		t.Fatal(err)
	}
	source, err := io.ReadAll(gz)
	if err != nil {
		t.Fatal(err)
	}
	entry := grammars.DetectLanguageByName("go")
	lang := entry.Language()
	cLang, err := ParityCLanguage("go")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	p := g.NewParser(lang)
	p.SetAdmissionCandidateRoute(true)
	tree, err := p.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	cTree := cp.Parse(source, nil)
	cur := source
	for si, b := range []byte{'Z', 'E'} {
		next := append([]byte(nil), cur...)
		next[2959] = b
		e := g.InputEdit{StartByte: 2959, OldEndByte: 2960, NewEndByte: 2960}
		pt := func(at int) g.Point {
			var row, col uint32
			for _, c := range cur[:at] {
				if c == '\n' {
					row, col = row+1, 0
				} else {
					col++
				}
			}
			return g.Point{Row: row, Column: col}
		}
		e.StartPoint, e.OldEndPoint, e.NewEndPoint = pt(2959), pt(2960), pt(2960)
		tree.Edit(e)
		cTree.Edit(&sitter.InputEdit{StartByte: 2959, OldEndByte: 2960, NewEndByte: 2960,
			StartPosition: sitter.Point{Row: uint(e.StartPoint.Row), Column: uint(e.StartPoint.Column)}, OldEndPosition: sitter.Point{Row: uint(e.OldEndPoint.Row), Column: uint(e.OldEndPoint.Column)}, NewEndPosition: sitter.Point{Row: uint(e.NewEndPoint.Row), Column: uint(e.NewEndPoint.Column)}})
		inc, err := p.ParseIncremental(next, tree)
		if err != nil {
			t.Fatal(err)
		}
		cn := cp.Parse(next, cTree)
		cTree.Close()
		cTree = cn
		fp := g.NewParser(lang)
		fp.SetAdmissionCandidateRoute(true)
		fresh, _ := fp.Parse(next)
		dC := compactT3StructuralDivergences(inc.RootNode(), lang, cTree.RootNode())
		dF := ccoreLegacyDigest(inc, lang) == ccoreLegacyDigest(fresh, lang)
		cFreshTree := cp.Parse(next, nil)
		dCF := compactT3StructuralDivergences(fresh.RootNode(), lang, cFreshTree.RootNode())
		cFreshTree.Close()
		fmt.Printf("CCGOUNDO step=%d incremental_vs_c_divergences=%d incremental_equals_public_fresh=%t fresh_vs_c_divergences=%d\n", si+1, len(dC), dF, len(dCF))
		for i, d := range dC {
			if i < 3 {
				fmt.Printf("CCGOUNDODIV %s\n", compactT3FormatDivergence(d))
			}
		}
		ccoreExpect(t, len(dC) == 0 && dF && len(dCF) == 0, "go undo step %d: incremental vs C %d divergences, equals public fresh %t, fresh vs C %d divergences", si+1, len(dC), dF, len(dCF))
		tree.Release()
		tree, cur = inc, next
		fresh.Release()
	}
	tree.Release()
	cTree.Close()

}

// TestCcoreRouteSession runs a session chain through Parser.Parse, Tree.Edit and
// Parser.ParseIncremental, and checks each step against C's incremental tree and
// against the public fresh parse of the same bytes. It also runs the same chain on
// the present engine (the route off) and compares the changed ranges and the edited
// flags of the route, of the present engine and of C (ccore_route_edits_c_test.go).
func TestCcoreRouteSession(t *testing.T) {
	set := os.Getenv("CCORE_INC_SET")
	if set == "" {
		t.Skip("set CCORE_INC_SET")
	}
	defer ccoreRouteCertify(t)()
	langName := strings.TrimSuffix(set, "-session")
	entry := grammars.DetectLanguageByName(langName)
	if entry == nil {
		t.Fatalf("no language %q", langName)
	}
	lang := entry.Language()
	cLang, err := ParityCLanguage(langName)
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	p := g.NewParser(lang)
	p.SetAdmissionCandidateRoute(true)
	fp := g.NewParser(lang)
	fp.SetAdmissionCandidateRoute(true)
	pe := g.NewParser(lang) // the present engine: the route off
	pe.SetAdmissionCandidateRoute(false)
	_, f0, i0 := bind.RouteCounters()
	steps, editSteps, sameTree, eqC, eqFresh, inv := 0, 0, 0, 0, 0, 0
	recoveryCmp, recoveryCandidates := 0, 0                 // steps whose old tree has an error, or whose new tree has one and differs from C's incremental result: compared with a FRESH C parse of the new bytes
	cmpC, diffNewErr, oldErrSteps, cleanSteps := 0, 0, 0, 0 // steps compared with C; old clean/new error steps that differ from C's incremental result; steps whose old tree has an error; clean-to-clean edit steps equal to C
	ranges := newCcoreRangeFacts()
	type flagKey struct{ pair, when string }
	flags := map[flagKey]*ccoreFlagFacts{}
	flagOf := func(pair, when string) *ccoreFlagFacts {
		k := flagKey{pair, when}
		if flags[k] == nil {
			flags[k] = newCcoreFlagFacts()
		}
		return flags[k]
	}
	chains := ccoreIncChains(t, set)
	ccoreRequirePositive(t, set+" chains", len(chains))
	for _, chain := range chains {
		var cTree *sitter.Tree
		var tree, ptree *g.Tree
		var prevSrc []byte
		for _, st := range chain {
			var next, pnext *g.Tree
			var perr error
			oldErr := tree != nil && tree.RootNode().HasError() // the old tree has an error: the core parses afresh, C does not
			if st.edit == nil {
				cTree = cp.Parse(st.src, nil)
				next, err = p.Parse(st.src)
				pnext, perr = pe.Parse(st.src)
			} else {
				e := st.edit
				cTree.Edit(ccoreSitterEdit(e))
				cn := cp.Parse(st.src, cTree)
				gedit := g.InputEdit{
					StartByte: e.StartByte, OldEndByte: e.OldEndByte, NewEndByte: e.NewEndByte,
					StartPoint:  g.Point{Row: e.StartRow, Column: e.StartCol},
					OldEndPoint: g.Point{Row: e.OldEndRow, Column: e.OldEndCol},
					NewEndPoint: g.Point{Row: e.NewEndRow, Column: e.NewEndCol},
				}
				tree.Edit(gedit)
				ptree.Edit(gedit)
				// The edited flags of every node, after the edit and before the reparse.
				ccoreFlagsGoC(flagOf("route_vs_c", "after_edit"), tree.RootNode(), cTree.RootNode(), e)
				ccoreFlagsGoC(flagOf("present_vs_c", "after_edit"), ptree.RootNode(), cTree.RootNode(), e)
				ccoreFlagsGoGo(flagOf("route_vs_present", "after_edit"), tree.RootNode(), ptree.RootNode(), e)
				next, err = p.ParseIncremental(st.src, tree)
				pnext, perr = pe.ParseIncremental(st.src, ptree)
				if err == nil && perr == nil && next != nil && pnext != nil && next != tree && pnext != ptree {
					editSteps++
					fo := cp.Parse(prevSrc, nil) // C with a history-free old tree: a fresh parse of the old bytes
					fo.Edit(ccoreSitterEdit(e))
					fn := cp.Parse(st.src, fo)
					cFresh := ccoreCRanges(fo.ChangedRanges(fn))
					fn.Close()
					fo.Close()
					ranges.add(st.id, ccoreGoRanges(g.DiffChangedRanges(tree, next)), ccoreGoRanges(g.DiffChangedRanges(ptree, pnext)), ccoreCRanges(cTree.ChangedRanges(cn)), cFresh)
				} else if err == nil && perr == nil {
					sameTree++ // the parser returned the old tree: no changed ranges to compare
				}
				cTree.Close()
				cTree = cn
			}
			if err != nil || next == nil || perr != nil || pnext == nil {
				t.Fatalf("%s %s: parse: %v / %v", set, st.id, err, perr)
			}
			steps++
			if st.edit != nil { // the flags of the new tree
				ccoreFlagsGoC(flagOf("route_vs_c", "after_reparse"), next.RootNode(), cTree.RootNode(), st.edit)
				ccoreFlagsGoC(flagOf("present_vs_c", "after_reparse"), pnext.RootNode(), cTree.RootNode(), st.edit)
				ccoreFlagsGoGo(flagOf("route_vs_present", "after_reparse"), next.RootNode(), pnext.RootNode(), st.edit)
			}
			divs := compactT3StructuralDivergences(next.RootNode(), lang, cTree.RootNode())
			newErr := next.RootNode().HasError()
			// Where C's incremental result legitimately differs from C's fresh parse (an old tree with
			// an error, or a recovered new tree), the routed result is compared with a FRESH C parse of
			// the new bytes: comparing it with the Go fresh parse could not see a recovery or conversion
			// defect that both Go paths share.
			recoveryCompare := func() {
				recoveryCandidates++
				ref := st.src
				if os.Getenv("CCORE_MUTATE_RECOVERY") != "" && len(ref) > 0 { // mutation check: the C reference differs from the routed result in one node
					ref = append([]byte(nil), ref...)
					ref[len(ref)/2] ^= 0x01
				}
				cf := cp.Parse(ref, nil)
				fd := compactT3StructuralDivergences(next.RootNode(), lang, cf.RootNode())
				cf.Close()
				recoveryCmp++
				if len(fd) != 0 {
					ccoreExpect(t, false, "%s %s: routed result differs from a fresh C parse of the new bytes (%d divergences, first %s)", set, st.id, len(fd), compactT3FormatDivergence(fd[0]))
				}
			}
			switch {
			case st.edit != nil && oldErr: // C's incremental result of an error tree is not the fresh parse (v56 chain-steps.md)
				oldErrSteps++
				recoveryCompare()
			case len(divs) == 0:
				cmpC++
				eqC++
				if st.edit != nil && !newErr {
					cleanSteps++
				}
			case newErr:
				cmpC++
				diffNewErr++ // old clean, new error: C's incremental result may differ from its fresh parse
				recoveryCompare()
			default:
				cmpC++
				ccoreExpect(t, false, "%s %s: %d divergences from C, first %s", set, st.id, len(divs), compactT3FormatDivergence(divs[0]))
			}
			fresh, ferr := fp.Parse(st.src)
			if ferr != nil || fresh == nil {
				t.Fatalf("%s %s: fresh parse: %v", set, st.id, ferr)
			}
			if ccoreLegacyDigest(next, lang) == ccoreLegacyDigest(fresh, lang) {
				eqFresh++
			} else {
				ccoreExpect(t, false, "%s %s: incremental differs from fresh", set, st.id)
			}
			fresh.Release()
			root := next.RootNode()
			okInv := !(root.Type(lang) == "ERROR" && !root.HasError()) && (root.EndByte() >= uint32(len(st.src)) || (next.ParseRuntime().StopReason != "" && next.ParseRuntime().StopReason != g.ParseStopAccepted))
			if okInv {
				inv++
			} else {
				ccoreExpect(t, false, "%s %s: invariant (error root, coverage) fails", set, st.id)
			}
			if tree != nil && tree != next {
				tree.Release()
			}
			if ptree != nil && ptree != pnext {
				ptree.Release()
			}
			tree, ptree = next, pnext
			prevSrc = st.src
		}
		if tree != nil {
			tree.Release()
		}
		if ptree != nil {
			ptree.Release()
		}
		cTree.Close()
	}
	decl, f1, i1 := bind.RouteCounters()
	fmt.Printf("CCROUTESESSION set=%s steps=%d edit_steps=%d returned_old_tree=%d compared_with_c=%d equal_c=%d old_clean_new_error_differs_from_c_incr=%d old_error_not_compared=%d clean_edit_steps_equal_c=%d equal_fresh=%d invariants=%d served_fresh=%d served_incr=%d declines=%v\n", set, steps, editSteps, sameTree, cmpC, eqC, diffNewErr, oldErrSteps, cleanSteps, eqFresh, inv, f1-f0, i1-i0, decl)
	fmt.Printf("CCROUTERECOVERY set=%s candidates=%d compared_with_fresh_c=%d\n", set, recoveryCandidates, recoveryCmp)
	ccoreRequirePositive(t, set+" recovery comparisons with a fresh C parse", recoveryCmp)
	ranges.report(set)
	var fk []flagKey
	for k := range flags {
		fk = append(fk, k)
	}
	sort.Slice(fk, func(i, j int) bool { return fk[i].when+fk[i].pair < fk[j].when+fk[j].pair })
	for _, k := range fk {
		flags[k].report(set, k.pair, k.when)
	}
	// Each comparison class must have compared something, also in a report-only run.
	ccoreRequirePositive(t, set+" steps", steps)
	ccoreRequirePositive(t, set+" edit steps (changed ranges)", editSteps)
	ccoreRequirePositive(t, set+" edited-flag nodes", flagOf("route_vs_c", "after_edit").nodes)
	if os.Getenv("CCORE_COUNT_AS") != "" {
		fmt.Printf("CCCOUNT test=%s set=%s checked=%d\n", os.Getenv("CCORE_COUNT_AS"), set, eqC)
		fmt.Printf("CCCOUNT test=%s set=%s:ranges checked=%d\n", os.Getenv("CCORE_COUNT_AS"), set, editSteps)
		fmt.Printf("CCCOUNT test=%s set=%s:flags checked=%d\n", os.Getenv("CCORE_COUNT_AS"), set, flagOf("route_vs_c", "after_edit").nodes)
	}
	// Changed ranges and edited flags must match C.
	ccoreCheckListed(t, "ranges", langName, ranges.counts())
	ccoreExpect(t, len(ranges.missSteps) == 0 || ranges.missRouteEqPresent, "%s: the route's ranges differ from the present engine's at a step where they miss C (steps %v)", set, ranges.missSteps)
	fr, fa := flagOf("route_vs_c", "after_edit"), flagOf("route_vs_c", "after_reparse")
	ccoreCheckListed(t, "flags", langName, map[string]string{
		"after_edit_nodes": fmt.Sprint(fr.nodes), "set_only_in_c": fmt.Sprint(fr.bOnly), "set_only_in_route": fmt.Sprint(fr.aOnly),
		"after_reparse_nodes": fmt.Sprint(fa.nodes), "after_reparse_set": fmt.Sprint(fa.aSet + fa.bSet),
	})
	// The route parser and the fresh parser
	// each serve one request per step, so 2*steps requests are served and none is declined.
	ccoreExpect(t, steps > 0 && cmpC > 0 && eqC+diffNewErr == cmpC && eqC+diffNewErr+oldErrSteps == steps && recoveryCmp == diffNewErr+oldErrSteps && recoveryCmp > 0 && eqFresh == steps && inv == steps && (f1-f0)+(i1-i0) == int64(2*steps) && len(decl) == 0, "%s: %d steps, compared with C %d, equal to C %d, old clean/new error differ %d, old error %d, equal to fresh %d, invariants %d, served fresh %d incr %d, declines %v", set, steps, cmpC, eqC, diffNewErr, oldErrSteps, eqFresh, inv, f1-f0, i1-i0, decl)
	_ = benchfixtures.EditSessionSteps
}

// ccoreHTMLCertificationSource is makeHTMLCertificationSource of the root tests.
func ccoreHTMLCertificationSource(target int, recovered bool) ([]byte, []int) {
	var source strings.Builder
	source.WriteString("<!doctype html><html><body><main>\n")
	var sites []int
	for i := 0; source.Len() < target || len(sites) < 4; i++ {
		source.WriteString("<section><custom-widget data-value=\"")
		sites = append(sites, source.Len())
		fmt.Fprintf(&source, "%06d\">payload_%06d</custom-widget></section>\n", i, i)
	}
	if recovered {
		source.WriteString("<section><></section>\n")
	}
	source.WriteString("</main></body></html>\n")
	return []byte(source.String()), sites
}

// ccoreJSFixture is the javascript fixture of the root test TestCompactOldTreeTransientErrorEditsMatchFreshParse.
func ccoreJSFixture(n int) []byte {
	var b bytes.Buffer
	for i := 0; b.Len() < n; i++ {
		fmt.Fprintf(&b, "function f%d(a, b) {\n  const x0 = a + b;\n  console.log(\"f%d\", x0);\n  return x0;\n}\n\n", i, i)
	}
	return b.Bytes()
}

// TestCcoreRouteStates compares Node.PreGotoState, Node.ParseState and Node.Symbol of a
// tree built by the core with pinned C's ts_node_parse_state, ts_node_next_parse_state
// and ts_node_symbol, on every node of the R4 sample (CCORE_LANG=<name>), where the
// route's tree equals C.
//
// A different tree, numbering, state or symbol makes the comparison fail.
func TestCcoreRouteStates(t *testing.T) {
	name := os.Getenv("CCORE_LANG")
	if name == "" {
		t.Skip("set CCORE_LANG")
	}
	defer ccoreRouteCertify(t)()
	entry := grammars.DetectLanguageByName(name)
	lang := entry.Language()
	cLang, err := ParityCLanguage(name)
	if err != nil {
		t.Fatal(err)
	}

	src, kind := ccoreRouteSource(t, name)
	cTree := compactT3ParseC(t, cLang, src)
	defer cTree.Close()
	rp := g.NewParser(lang)
	rp.SetAdmissionCandidateRoute(true)
	_, f0, _ := bind.RouteCounters()
	rt, err := rp.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Release()
	_, f1, _ := bind.RouteCounters()
	ccoreExpect(t, f1 > f0, "%s: the core did not serve the parse; the states compared are the present engine's", name)
	if divs := compactT3StructuralDivergences(rt.RootNode(), lang, cTree.RootNode()); len(divs) != 0 {
		fmt.Printf("CCSTATES lang=%s input=%s compared=false served=%t route_differs_from_c=true\n", name, kind, f1 > f0)
		fmt.Printf("CCSTATESLINE @states %s unavailable=tree\n", name)
		t.Errorf("%s input=%s: tree differs from C: %s", name, kind, compactT3FormatDivergence(divs[0]))
		ccoreCount("TestCcoreRouteStates", name, 1)
		return
	}
	// The blob may number the states differently from the pinned C table; then the numbers
	// cannot match although the trees do. Prove it from the tables.
	cfg, cerr := bind.Config(lang)
	if cerr != nil {
		t.Fatal(cerr)
	}
	states, cstates := int(cfg.Tables.StateCount), int(cLang.ParseStateCount())
	numberingDiffers := states != cstates
	if !numberingDiffers {
		syms := min(int(lang.SymbolCount), int(cLang.NodeKindCount()))
	pairs:
		for s := 0; s < states; s++ {
			for sym := 0; sym < syms; sym++ {
				if cfg.Tables.NextState(uint16(s), ccore.Symbol(sym)) != cLang.NextState(uint16(s), uint16(sym)) {
					numberingDiffers = true
					break pairs
				}
			}
		}
	}
	if numberingDiffers {
		fmt.Printf("CCSTATES lang=%s numbering_differs_from_c=true (state numbers cannot be compared; blob states %d, C states %d)\n", name, states, cstates)
		fmt.Printf("CCSTATESLINE @states %s unavailable=numbering\n", name)
		t.Errorf("%s: state numbering differs from C", name)
		ccoreCount("TestCcoreRouteStates", name, 1)
		return
	}
	// C's "none" state (65535) against the package's "no state" (0): counted, not hidden.
	total, nonePre, noneNext, otherPre, otherNext, symbolIDs := 0, 0, 0, 0, 0, 0
	var first []string
	var walkC func(a *g.Node, c *sitter.Node, path string)
	walkC = func(a *g.Node, c *sitter.Node, path string) {
		total++
		switch cs := c.ParseState(); {
		case uint16(a.PreGotoState()) == cs:
		case cs == 65535 && a.PreGotoState() == 0:
			nonePre++
			if len(first) < 3 {
				first = append(first, fmt.Sprintf("path=%s pre route=%d C=%d", path, a.PreGotoState(), cs))
			}
		default:
			otherPre++
			if len(first) < 3 {
				first = append(first, fmt.Sprintf("path=%s %s[%d,%d) pre route %d C %d", path, a.Type(lang), a.StartByte(), a.EndByte(), a.PreGotoState(), cs))
			}
		}
		switch cs := c.NextParseState(); {
		case uint16(a.ParseState()) == cs:
		case cs == 65535 && a.ParseState() == 0:
			noneNext++
			if len(first) < 3 {
				first = append(first, fmt.Sprintf("path=%s next route=%d C=%d", path, a.ParseState(), cs))
			}
		default:
			otherNext++
			if len(first) < 3 {
				first = append(first, fmt.Sprintf("path=%s %s[%d,%d) next route %d C %d", path, a.Type(lang), a.StartByte(), a.EndByte(), a.ParseState(), cs))
			}
		}
		if uint16(a.Symbol()) != c.KindId() {
			symbolIDs++
			if len(first) < 3 {
				first = append(first, fmt.Sprintf("path=%s symbol route=%d C=%d", path, a.Symbol(), c.KindId()))
			}
		}
		for i := 0; i < a.ChildCount(); i++ {
			walkC(a.Child(i), c.Child(uint(i)), fmt.Sprintf("%s/%d", path, i))
		}
	}
	walkC(rt.RootNode(), cTree.RootNode(), "root")
	// report only: the present engine's values, where its tree equals C too
	presentDiff, presentCompared := -1, false
	if lt, err := g.NewParser(lang).Parse(src); err == nil && lt != nil {
		if len(compactT3StructuralDivergences(lt.RootNode(), lang, cTree.RootNode())) == 0 {
			presentCompared, presentDiff = true, 0
			var walk func(a, b *g.Node)
			walk = func(a, b *g.Node) {
				if a.ParseState() != b.ParseState() || a.PreGotoState() != b.PreGotoState() {
					presentDiff++
				}
				for i := 0; i < a.ChildCount() && i < b.ChildCount(); i++ {
					walk(a.Child(i), b.Child(i))
				}
			}
			walk(lt.RootNode(), rt.RootNode())
		}
		lt.Release()
	}
	fmt.Printf("CCSTATES lang=%s input=%s compared=true served=%t nodes=%d none_pre=%d none_next=%d other_pre=%d other_next=%d symbol_ids=%d present_engine_compared=%t present_engine_differs=%d first=%v\n",
		name, kind, f1 > f0, total, nonePre, noneNext, otherPre, otherNext, symbolIDs, presentCompared, presentDiff, first)
	fmt.Printf("CCSTATESLINE @states %s none_pre=%d none_next=%d symbol_ids=%d\n", name, nonePre, noneNext, symbolIDs)
	ccoreRequirePositive(t, name+" states (nodes)", total)
	ccoreExpect(t, otherPre == 0 && otherNext == 0, "%s: %d nodes differ from C in ParseState/PreGotoState beyond the documented none mapping (%d pre, %d next) of %d; first %v", name, otherPre+otherNext, otherPre, otherNext, total, first)
	for k, got := range map[string]int{"none_pre": nonePre, "none_next": noneNext, "symbol_ids": symbolIDs} {
		ccoreExpect(t, got == 0, "%s input=%s: %s differs from C on %d nodes; first %v", name, kind, k, got, first)
	}
	if as := os.Getenv("CCORE_COUNT_AS"); as != "" {
		fmt.Printf("CCCOUNT test=%s set=%s checked=%d\n", as, name, total)
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TestCcoreRouteLookahead: NewLookaheadIterator(lang, node.ParseState()) on a
// core tree lists the symbols that C's lookahead iterator lists for
// ts_node_next_parse_state (CCORE_ROUTE_LOOKAHEAD=1).
func TestCcoreRouteLookahead(t *testing.T) {
	if os.Getenv("CCORE_ROUTE_LOOKAHEAD") == "" {
		t.Skip("set CCORE_ROUTE_LOOKAHEAD")
	}
	defer ccoreRouteCertify(t)()
	names := []string{"json", "python"}
	if l := os.Getenv("CCORE_LANG"); l != "" {
		names = []string{l}
	}
	for _, name := range names {
		entry := grammars.DetectLanguageByName(name)
		lang := entry.Language()
		cLang, err := ParityCLanguage(name)
		if err != nil {
			t.Fatal(err)
		}
		src, _ := ccoreRouteSource(t, name)
		cTree := compactT3ParseC(t, cLang, src)
		rp := g.NewParser(lang)
		rp.SetAdmissionCandidateRoute(true)
		_, lf0, _ := bind.RouteCounters()
		rt, err := rp.Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		_, lf1, _ := bind.RouteCounters()
		ccoreExpect(t, lf1 > lf0, "%s: the core did not serve the parse; the symbols compared are the present engine's", name)
		if divs := compactT3StructuralDivergences(rt.RootNode(), lang, cTree.RootNode()); len(divs) != 0 {
			t.Errorf("%s input=sample: tree differs from C: %s", name, compactT3FormatDivergence(divs[0]))
			rt.Release()
			cTree.Close()
			continue
		}
		checked, equal := 0, 0
		var walk func(a *g.Node, c *sitter.Node, path string)
		walk = func(a *g.Node, c *sitter.Node, path string) {
			if checked < 400 && a.ParseState() != 0 && checked%1 == 0 {
				ci := cLang.LookaheadIterator(c.NextParseState())
				want := map[string]bool{}
				for _, n := range ci.IterNames() {
					want[n] = true
				}
				ci.Close()
				it, err := g.NewLookaheadIterator(lang, a.ParseState())
				got := map[string]bool{}
				if err == nil {
					for it.Next() {
						got[it.CurrentSymbolName()] = true
					}
				}
				checked++
				same := err == nil && len(got) == len(want)
				for n := range want {
					same = same && got[n]
				}
				if same {
					equal++
				} else if checked-equal <= 3 {
					fmt.Printf("CCLOOKAHEADDIFF lang=%s input=sample path=%s node=%s[%d,%d) state=%d route=%v C=%v err=%v\n", name, path, a.Type(lang), a.StartByte(), a.EndByte(), a.ParseState(), got, want, err)
				}
			}
			for i := 0; i < a.ChildCount(); i++ {
				walk(a.Child(i), c.Child(uint(i)), fmt.Sprintf("%s/%d", path, i))
			}
		}
		walk(rt.RootNode(), cTree.RootNode(), "root")
		fmt.Printf("CCLOOKAHEAD lang=%s nodes_checked=%d symbol_set_equal_to_c=%d\n", name, checked, equal)
		if os.Getenv("CCORE_COUNT_AS") != "" {
			fmt.Printf("CCCOUNT test=%s set=%s checked=%d\n", os.Getenv("CCORE_COUNT_AS"), name, checked)
		}
		ccoreExpect(t, checked > 0 && equal == checked, "%s: %d of %d nodes list another symbol set than C", name, checked-equal, checked)
		rt.Release()
		cTree.Close()
	}
}

// TestCcoreRouteStatesMinimize shrinks the R4 sample of a language (by lines,
// then by single bytes) to a small input on which the route and the present
// engine both equal pinned C and Node.ParseState or Node.PreGotoState still
// differ (with CCORE_STATES_MIN_SKIPROOT=1 the root is left out) (CCORE_LANG=<name>, CCORE_STATES_MIN=1).
func TestCcoreRouteStatesMinimize(t *testing.T) {
	name := os.Getenv("CCORE_LANG")
	if name == "" || os.Getenv("CCORE_STATES_MIN") == "" {
		t.Skip("set CCORE_LANG and CCORE_STATES_MIN")
	}
	defer ccoreRouteCertify(t)()
	entry := grammars.DetectLanguageByName(name)
	lang := entry.Language()
	cLang, err := ParityCLanguage(name)
	if err != nil {
		t.Fatal(err)
	}
	var detail string
	differs := func(src []byte) bool {
		cTree := compactT3ParseC(t, cLang, src)
		defer cTree.Close()
		rp := g.NewParser(lang)
		rp.SetAdmissionCandidateRoute(true)
		rt, err := rp.Parse(src)
		if err != nil || rt == nil {
			return false
		}
		defer rt.Release()
		lt, err := g.NewParser(lang).Parse(src)
		if err != nil || lt == nil {
			return false
		}
		defer lt.Release()
		if os.Getenv("CCORE_STATES_MIN_CLEAN") != "" && rt.RootNode().HasError() {
			return false
		}
		if len(compactT3StructuralDivergences(rt.RootNode(), lang, cTree.RootNode())) != 0 || len(compactT3StructuralDivergences(lt.RootNode(), lang, cTree.RootNode())) != 0 {
			return false
		}
		found := ""
		var walk func(a, b *g.Node)
		walk = func(a, b *g.Node) {
			if found == "" && (a.ParseState() != b.ParseState() || a.PreGotoState() != b.PreGotoState()) {
				found = fmt.Sprintf("%s[%d,%d) leaf=%t extra=%t present %d/%d route %d/%d", a.Type(lang), a.StartByte(), a.EndByte(), a.ChildCount() == 0, a.IsExtra(), a.PreGotoState(), a.ParseState(), b.PreGotoState(), b.ParseState())
			}
			for i := 0; i < a.ChildCount() && i < b.ChildCount(); i++ {
				walk(a.Child(i), b.Child(i))
			}
		}
		if os.Getenv("CCORE_STATES_MIN_SKIPROOT") != "" { // look past the root, whose states differ in every file
			for i := 0; i < lt.RootNode().ChildCount() && i < rt.RootNode().ChildCount(); i++ {
				walk(lt.RootNode().Child(i), rt.RootNode().Child(i))
			}
		} else {
			walk(lt.RootNode(), rt.RootNode())
		}
		detail = found
		return found != ""
	}
	src, _ := ccoreRouteSource(t, name)
	if !differs(src) {
		fmt.Printf("CCSTATESMIN lang=%s nothing to minimize\n", name)
		return
	}
	// ddmin over lines, then over bytes
	shrink := func(units [][]byte) [][]byte {
		n := 2
		for len(units) >= 2 {
			chunk := (len(units) + n - 1) / n
			reduced := false
			for i := 0; i < len(units); i += chunk {
				end := i + chunk
				if end > len(units) {
					end = len(units)
				}
				rest := append(append([][]byte(nil), units[:i]...), units[end:]...)
				if len(rest) > 0 && differs(bytes.Join(rest, nil)) {
					units, reduced = rest, true
					if n > 2 {
						n--
					}
					break
				}
			}
			if !reduced {
				if n >= len(units) {
					break
				}
				n = min(n*2, len(units))
			}
		}
		return units
	}
	var lines [][]byte
	for _, l := range bytes.SplitAfter(src, []byte("\n")) {
		if len(l) > 0 {
			lines = append(lines, l)
		}
	}
	lines = shrink(lines)
	cur := bytes.Join(lines, nil)
	var bs [][]byte
	for i := range cur {
		bs = append(bs, cur[i:i+1])
	}
	cur = bytes.Join(shrink(bs), nil)
	differs(cur)
	fmt.Printf("CCSTATESMIN lang=%s bytes=%d input=%q first=%s\n", name, len(cur), cur, detail)
}
