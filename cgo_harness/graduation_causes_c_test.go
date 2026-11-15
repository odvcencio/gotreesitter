//go:build cgo && treesitter_c_parity

package cgoharness

// Diagnostic tests behind the failing items of the graduation gate. They
// print GRADCAUSE lines and write the raw logs to GRAD_CAUSE_DIR (when set);
// they fail only when they cannot run. GRAD_LANG names the grammar.
//
//	TestGraduationCauseTokens   tokens the present engine, the core and C lex on the sample
//	TestGraduationCauseQueries  the failing query comparisons, reduced, with C's own query engine

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func gradDump(name string, lines []string) {
	dir := os.Getenv("GRAD_CAUSE_DIR")
	if dir == "" {
		return
	}
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, name), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func gradCParser(t *testing.T, c *gradCtx, log *[]string) *sitter.Parser {
	cp := sitter.NewParser()
	if err := cp.SetLanguage(c.cLang); err != nil {
		t.Fatal(err)
	}
	if log != nil {
		cp.SetLogger(func(kind sitter.LogType, msg string) {
			if kind == sitter.LogTypeParse {
				*log = append(*log, msg)
			}
		})
	}
	return cp
}

func gradCount(lines []string, prefix string) int {
	n := 0
	for _, l := range lines {
		if strings.HasPrefix(l, prefix) {
			n++
		}
	}
	return n
}

func TestGraduationCauseTokens(t *testing.T) {
	c := gradLoad(t)
	src, kind := gradSample(t, c.name)
	lt, err := gradDefault(c.lang).Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	lr := lt.ParseRuntime()
	var clog, slog []string
	cp := gradCParser(t, c, &clog)
	defer cp.Close()
	cp.Parse(src, nil)
	core := ccoreNew(c.lang)
	core.Parse(src, func(s string) { slog = append(slog, s) })
	_, ctok, mv, multi, _ := core.Stats()
	// distinct (position, symbol) lexed by C
	distinct := map[string]int{}
	var events []string
	row, col := "", ""
	re := regexp.MustCompile(`(?:lex_internal|lex_external) state:\d+, row:(\d+), column:(\d+)`)
	for _, l := range clog {
		if m := re.FindStringSubmatch(l); m != nil {
			row, col = m[1], m[2]
		}
		if strings.HasPrefix(l, "lexed_lookahead") {
			ev := row + ":" + col + " " + l
			distinct[row+":"+col+" "+strings.Fields(l)[1]]++
			events = append(events, ev)
		}
	}
	gradDump(c.name+".c-lex-events.txt", events)
	gradDump(c.name+".c.log", clog)
	gradDump(c.name+".core.log", slog)
	multiLex := []string{}
	keys := make([]string, 0)
	for k, n := range distinct {
		if n > 1 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		multiLex = append(multiLex, fmt.Sprintf("%s x%d", k, distinct[k]))
	}
	gradDump(c.name+".lexed-more-than-once.txt", multiLex)
	extra := 0
	for _, k := range keys {
		extra += distinct[k] - 1
	}
	fmt.Printf("GRADCAUSE tokens lang=%s fixture=%s bytes=%d present_engine_tokens=%d core_tokens=%d core_max_versions=%d core_tokens_with_more_than_one_version=%d c_lexed_lookahead=%d c_lex_internal=%d c_lex_external=%d c_distinct_position_symbol=%d c_extra_lexes_same_position_symbol=%d core_lexed_lookahead_lines=%d\n",
		c.name, kind, len(src), lr.TokensConsumed, ctok, mv, multi, gradCount(clog, "lexed_lookahead"), gradCount(clog, "lex_internal"), gradCount(clog, "lex_external"), len(distinct), extra, gradCount(slog, "lexed_lookahead"))
	for i, k := range keys {
		if i < 5 {
			fmt.Printf("GRADCAUSE lexed-more-than-once %s x%d\n", k, distinct[k])
		}
	}
	lt.Release()
}

// reuseEvents lists (symbol, row, col) of every reuse_node line of a parse log.
func gradReuseEvents(log []string) []string { return gradReuseEventsAt(log, nil) }

// gradReuseEventsAt is gradReuseEvents with the byte offset of each position when src is given.
func gradReuseEventsAt(log []string, src []byte) []string {
	return gradEventsAt(log, src, "reuse_node ")
}

// gradEventsAt lists the log lines that start with prefix (all of them when
// prefix is "cant_reuse"), each with the position of the version that was processed.
func gradEventsAt(log []string, src []byte, prefix string) []string {
	lineStart := []int{0}
	for i, b := range src {
		if b == '\n' {
			lineStart = append(lineStart, i+1)
		}
	}
	var out []string
	re := regexp.MustCompile(`process version:\d+, version_count:\d+, state:\d+, row:(\d+), col:(\d+)`)
	row, col := "?", "?"
	for _, l := range log {
		if m := re.FindStringSubmatch(l); m != nil {
			row, col = m[1], m[2]
		}
		if strings.HasPrefix(l, prefix) {
			at := row + ":" + col
			if src != nil {
				r, _ := strconv.Atoi(row)
				cc, _ := strconv.Atoi(col)
				if r < len(lineStart) {
					at = fmt.Sprintf("byte%d", lineStart[r]+cc)
				}
			}
			out = append(out, strings.TrimPrefix(strings.TrimPrefix(l, "reuse_node symbol:"), "")+"@"+at)
		}
	}
	return out
}

func gradNodeSet(n *g.Node, set map[*g.Node]bool) {
	if n == nil {
		return
	}
	set[n] = true
	for i := 0; i < n.ChildCount(); i++ {
		gradNodeSet(n.Child(i), set)
	}
}

// gradTopReused lists the outermost nodes of the new tree that are the same
// node objects as in the old tree.
func gradTopReused(n *g.Node, old map[*g.Node]bool, lang *g.Language, out *[]string) {
	if n == nil {
		return
	}
	if old[n] {
		*out = append(*out, fmt.Sprintf("%s[%d-%d]", n.Type(lang), n.StartByte(), n.EndByte()))
		return
	}
	for i := 0; i < n.ChildCount(); i++ {
		gradTopReused(n.Child(i), old, lang, out)
	}
}

// gradQueryLists returns the capture lists of one pattern on the route tree,
// the present engine's tree and C's tree (C's own query engine).
func gradQueryLists(t *testing.T, c *gradCtx, src []byte, pat string) (route, present, cl []string) {
	rt, err := gradRoute(c.lang).Parse(src)
	if err != nil || rt == nil {
		t.Fatal(err)
	}
	defer rt.Release()
	lt, err := gradDefault(c.lang).Parse(src)
	if err != nil || lt == nil {
		t.Fatal(err)
	}
	defer lt.Release()
	cTree := compactT3ParseC(t, c.cLang, src)
	defer cTree.Close()
	return gradCaps(t, c.lang, rt, pat), gradCaps(t, c.lang, lt, pat), gradCapsC(t, c.cLang, cTree, pat, src)
}

func gradSetDiff(a, b []string) []string {
	in := map[string]bool{}
	for _, x := range b {
		in[x] = true
	}
	var out []string
	for _, x := range a {
		if !in[x] {
			out = append(out, x)
		}
	}
	return out
}

func TestGraduationCauseQueries(t *testing.T) {
	c := gradLoad(t)
	defer ccoreRouteCertify(t)()
	pats := map[string]string{"missing": "(MISSING) @missing\n", "error": "(ERROR) @error\n"}
	for _, s := range c.lang.SupertypeSymbols {
		if int(s) < len(c.lang.SymbolNames) {
			pats["supertype:"+c.lang.SymbolNames[s]] = fmt.Sprintf("(%s) @supertype\n", c.lang.SymbolNames[s])
		}
	}
	names := make([]string, 0, len(pats))
	for k := range pats {
		names = append(names, k)
	}
	sort.Strings(names)
	inputs := gradInputs(t, c.name)
	if len(inputs) > 12 {
		inputs = inputs[:12]
	}
	reported := 0
	for _, in := range inputs {
		for _, pn := range names {
			route, present, cl := gradQueryLists(t, c, in.Src, pats[pn])
			if gradEqual(route, cl) && gradEqual(route, present) {
				continue
			}
			fmt.Printf("GRADCAUSE query input=%s bytes=%d pattern=%q route=%d present=%d c=%d route_not_in_c=%v c_not_in_route=%v present_not_in_c=%v c_not_in_present=%v\n",
				in.ID, len(in.Src), pats[pn], len(route), len(present), len(cl), gradHead(gradSetDiff(route, cl)), gradHead(gradSetDiff(cl, route)), gradHead(gradSetDiff(present, cl)), gradHead(gradSetDiff(cl, present)))
			if reported < 2 {
				reported++
				pat := pats[pn]
				// keep: the route equals C, the present engine's query result differs from C's
				// on a tree whose structure equals C's.
				fails := func(src []byte) bool {
					r, p, k := gradQueryLists(t, c, src, pat)
					if !gradEqual(r, k) || gradEqual(p, k) {
						return false
					}
					cTree := compactT3ParseC(t, c.cLang, src)
					defer cTree.Close()
					lt, _ := gradDefault(c.lang).Parse(src)
					defer lt.Release()
					return len(compactT3StructuralDivergences(lt.RootNode(), c.lang, cTree.RootNode())) == 0
				}
				if !fails(in.Src) {
					fmt.Printf("GRADCAUSE query-note input=%s: the present engine's tree differs from C's structurally, so the difference is not a query difference on equal trees\n", in.ID)
					continue
				}
				min := gradMinimize(in.Src, fails)
				route, present, cl := gradQueryLists(t, c, min, pat)
				fmt.Printf("GRADCAUSE query-minimal input=%s pattern=%q source=%q\nGRADCAUSE query-minimal route=%v\nGRADCAUSE query-minimal present=%v\nGRADCAUSE query-minimal c=%v\n", in.ID, pat, min, route, present, cl)
				rt, _ := gradRoute(c.lang).Parse(min)
				lt, _ := gradDefault(c.lang).Parse(min)
				cTree := compactT3ParseC(t, c.cLang, min)
				fmt.Printf("GRADCAUSE query-minimal tree route=%s\nGRADCAUSE query-minimal tree present=%s\nGRADCAUSE query-minimal tree c=%s\n", rt.RootNode().SExpr(c.lang), lt.RootNode().SExpr(c.lang), cTree.RootNode().ToSexp())
				for _, cap := range append(gradSetDiff(cl, present), gradSetDiff(present, cl)...) {
					var a, b uint32
					fmt.Sscanf(cap[strings.Index(cap, ":")+1:], "%d-%d", &a, &b)
					cn := cTree.RootNode().DescendantForByteRange(uint(a), uint(b))
					cchain := []string{}
					for n := cn; n != nil; n = n.Parent() {
						cchain = append(cchain, fmt.Sprintf("%s[%d-%d]", n.Kind(), n.StartByte(), n.EndByte()))
					}
					gchain := []string{}
					for n := lt.RootNode().DescendantForByteRange(a, b); n != nil; n = n.Parent() {
						gchain = append(gchain, fmt.Sprintf("%s[%d-%d]", n.Type(c.lang), n.StartByte(), n.EndByte()))
					}
					fmt.Printf("GRADCAUSE query-minimal capture=%s C ancestors=%v present ancestors=%v\n", cap, cchain, gchain)
				}
				cTree.Close()
			}
		}
	}
	if reported == 0 {
		fmt.Printf("GRADCAUSE query lang=%s no difference among %d inputs and %d patterns\n", c.name, len(inputs), len(names))
	}
}

func gradHead(l []string) []string {
	if len(l) > 5 {
		return append(append([]string(nil), l[:5]...), "...+"+strconv.Itoa(len(l)-5))
	}
	return l
}

// gradMinimize shrinks src by deleting lines, then bytes, while fails stays true.
func gradMinimize(src []byte, fails func([]byte) bool) []byte {
	lines := strings.SplitAfter(string(src), "\n")
	join := func(l []string) []byte { return []byte(strings.Join(l, "")) }
	for n := 2; len(lines) >= 2; {
		chunk := (len(lines) + n - 1) / n
		removed := false
		for i := 0; i < len(lines); i += chunk {
			end := i + chunk
			if end > len(lines) {
				end = len(lines)
			}
			cand := append(append([]string(nil), lines[:i]...), lines[end:]...)
			if len(cand) > 0 && fails(join(cand)) {
				lines, removed = cand, true
				n = max(n-1, 2)
				break
			}
		}
		if !removed {
			if n >= len(lines) {
				break
			}
			n = min(len(lines), n*2)
		}
	}
	cur := join(lines)
	for size := len(cur) / 2; size >= 1; size /= 2 {
		for i := 0; i+size <= len(cur); {
			cand := append(append([]byte(nil), cur[:i]...), cur[i+size:]...)
			if len(cand) > 0 && fails(cand) {
				cur = cand
			} else {
				i += size
			}
		}
	}
	return cur
}

// TestGraduationCauseDeclines reports why a session's reparses do or do not
// reach the core: the scanner's incremental-reuse capability (the present
// engine's table that ParseIncremental consults before the route) and the
// route's own counters.
func TestGraduationCauseDeclines(t *testing.T) {
	c := gradLoad(t)
	defer ccoreRouteCertify(t)()
	sample, _ := gradSample(t, c.name)
	sc := c.lang.ExternalScanner
	reuse, hasReuse := sc.(g.IncrementalReuseExternalScanner)
	errReuse, hasErr := sc.(g.ErrorTreeIncrementalReuseExternalScanner)
	reuseVal, errVal := "n/a", "n/a"
	if hasReuse {
		reuseVal = fmt.Sprint(reuse.SupportsIncrementalReuse())
	}
	if hasErr {
		errVal = fmt.Sprint(errReuse.SupportsIncrementalReuseFromErrorTree())
	}
	fmt.Printf("GRADCAUSE scanner lang=%s scanner_type=%T implements_IncrementalReuseExternalScanner=%t supports_incremental_reuse=%s implements_ErrorTreeIncrementalReuseExternalScanner=%t supports_reuse_from_error_tree=%s\n", c.name, sc, hasReuse, reuseVal, hasErr, errVal)
	chain := gradChain(sample)
	p := gradRoute(c.lang)
	var tree *g.Tree
	steps, servedIncr, oldErrNotServed, oldCleanNotServed, oldErr := 0, 0, 0, 0, 0
	for _, st := range chain {
		if st.edit == nil {
			tree, _ = p.Parse(st.src)
			continue
		}
		hadErr := tree.RootNode().HasError()
		if hadErr {
			oldErr++
		}
		tree.Edit(*st.edit)
		nt, served, err := gradReparse(p, st.src, tree)
		if err != nil {
			t.Fatal(err)
		}
		steps++
		switch {
		case served:
			servedIncr++
		case hadErr:
			oldErrNotServed++
		default:
			oldCleanNotServed++
		}
		if nt != tree {
			tree.Release()
		}
		tree = nt
	}
	d, f, i := bind.RouteCounters()
	fmt.Printf("GRADCAUSE reparses lang=%s steps=%d served_by_core=%d not_served_old_tree_clean=%d not_served_old_tree_has_error=%d old_trees_with_error=%d route_served_fresh_total=%d route_declines=%v\n", c.name, steps, servedIncr, oldCleanNotServed, oldErrNotServed, oldErr, f, d)
	_ = i
}
