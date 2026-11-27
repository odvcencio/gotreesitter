//go:build cgo && treesitter_c_parity

package cgoharness

// B7: edits served by the core incrementally, through the public API, against
// the present engine and pinned C.
//
//	CCORE_B7_LANG=<lang> go test -run '^TestCcoreB7Counters$'   work and reuse counters of the clean edit classes at 3 sizes
//	CCORE_B7_LANG=<lang> go test -run '^TestCcoreB7Timing$'     timing of the five edit classes, three arms alternating
//
// Edit classes: noop (the same source, no edit), same (one digit replaced by
// another), insdel (a digit typed, then deleted), breakrep (the syntax broken,
// then repaired). The measured path is what a caller does: Tree.Edit, the
// reparse, then node access (walk A: the root and its direct children; walk B:
// a cursor walk of every node with kind, byte range and field name).

import (
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

var b7Sizes = []struct {
	name string
	n    int
}{{"32KiB", 32 << 10}, {"137KiB", 137 << 10}, {"1MiB", 1 << 20}}

func b7Doc(lang string, target int) []byte {
	var b strings.Builder
	switch lang {
	case "json":
		b.WriteString("[\n")
		for i := 0; b.Len() < target; i++ {
			if i > 0 {
				b.WriteString(",\n")
			}
			fmt.Fprintf(&b, "  {\"id\": %d, \"name\": \"item%d\", \"tags\": [1, 2, \"x\"], \"ok\": true}", 100000+i%899999, i)
		}
		b.WriteString("\n]\n")
	case "python":
		for i := 0; b.Len() < target; i++ {
			fmt.Fprintf(&b, "def f%d(a):\n    return a + %d\n\n", i, 100+i%900)
		}
	case "c":
		for i := 0; b.Len() < target; i++ {
			fmt.Fprintf(&b, "int f%d(int a) { return a + %d; }\n", i, 100+i%900)
		}
	case "typescript":
		for i := 0; b.Len() < target; i++ {
			fmt.Fprintf(&b, "function f%d(a: number): number { return a + %d; }\n", i, 100+i%900)
		}
	default:
		panic("b7Doc: " + lang)
	}
	return []byte(b.String())
}

// b7Pair is the base source and the changed source of one edit class, with the
// edit from base to changed and the edit back.
type b7Pair struct {
	class     string
	base, alt []byte
	fwd, back *bindtest.Edit
	fwdName   string // class of the step base to alt
	backName  string // class of the step alt to base
}

func b7Replace(src []byte, at int, b byte) ([]byte, *bindtest.Edit) {
	out, e := ccoreReplaceEdit(src, at, b)
	return out, &e
}

func b7Insert(src []byte, at int, b byte) ([]byte, *bindtest.Edit) {
	out := append(append(append([]byte(nil), src[:at]...), b), src[at:]...)
	r0, c0 := ccorePointAt(src, at)
	r1, c1 := ccorePointAt(out, at+1)
	return out, &bindtest.Edit{StartByte: uint32(at), OldEndByte: uint32(at), NewEndByte: uint32(at + 1),
		StartRow: r0, StartCol: c0, OldEndRow: r0, OldEndCol: c0, NewEndRow: r1, NewEndCol: c1}
}

func b7Delete(src []byte, at int) ([]byte, *bindtest.Edit) {
	out := append(append([]byte(nil), src[:at]...), src[at+1:]...)
	r0, c0 := ccorePointAt(src, at)
	r1, c1 := ccorePointAt(src, at+1)
	return out, &bindtest.Edit{StartByte: uint32(at), OldEndByte: uint32(at + 1), NewEndByte: uint32(at),
		StartRow: r0, StartCol: c0, OldEndRow: r1, OldEndCol: c1, NewEndRow: r0, NewEndCol: c0}
}

// b7Pairs builds the edit classes at the middle of the document.
func b7Pairs(t testing.TB, lang string, src []byte) []b7Pair {
	mid := len(src) / 2
	// a digit inside a number, at or after the middle
	dig := mid
	for dig < len(src)-2 && !(src[dig] >= '0' && src[dig] <= '8' && src[dig+1] >= '0' && src[dig+1] <= '9') {
		dig++
	}
	if dig >= len(src)-2 {
		t.Fatalf("%s: no digit found", lang)
	}
	var out []b7Pair
	out = append(out, b7Pair{class: "noop", base: src, alt: src, fwdName: "noop", backName: "noop"})
	alt, fe := b7Replace(src, dig, src[dig]+1)
	_, be := b7Replace(alt, dig, src[dig])
	out = append(out, b7Pair{class: "same", base: src, alt: alt, fwd: fe, back: be, fwdName: "same", backName: "same"})
	alt2, fe2 := b7Insert(src, dig+1, '7')
	_, be2 := b7Delete(alt2, dig+1)
	out = append(out, b7Pair{class: "insdel", base: src, alt: alt2, fwd: fe2, back: be2, fwdName: "insert", backName: "delete"})
	// break: the keyword "return" (python, c, typescript) or the colon after a json key
	var at int
	var ch byte
	if lang == "json" {
		at = strings.Index(string(src[mid:]), "\": ")
		at += mid + 1
		ch = '#'
	} else {
		at = strings.Index(string(src[mid:]), "return")
		at += mid
		ch = 'x'
	}
	if at < mid {
		t.Fatalf("%s: no break position", lang)
	}
	alt3, fe3 := b7Replace(src, at, ch)
	_, be3 := b7Replace(alt3, at, src[at])
	out = append(out, b7Pair{class: "breakrep", base: src, alt: alt3, fwd: fe3, back: be3, fwdName: "break", backName: "repair"})
	return out
}

func b7GoEdit(e *bindtest.Edit) g.InputEdit {
	return g.InputEdit{StartByte: e.StartByte, OldEndByte: e.OldEndByte, NewEndByte: e.NewEndByte,
		StartPoint:  g.Point{Row: e.StartRow, Column: e.StartCol},
		OldEndPoint: g.Point{Row: e.OldEndRow, Column: e.OldEndCol},
		NewEndPoint: g.Point{Row: e.NewEndRow, Column: e.NewEndCol}}
}

func b7LangFor(t testing.TB, name string) (*g.Language, *sitter.Language) {
	return ccoreLanguages(t, name)
}

// Walks. Each returns a number so the work is not optimised away.
func b7WalkAGo(tree *g.Tree, lang *g.Language) int {
	r := tree.RootNode()
	n := len(r.Type(lang)) + int(r.EndByte())
	for i := 0; i < r.ChildCount(); i++ {
		c := r.Child(i)
		n += len(c.Type(lang)) + int(c.StartByte()) + int(c.EndByte())
	}
	return n
}

func b7WalkBGo(tree *g.Tree, lang *g.Language) int {
	c := g.NewTreeCursor(tree.RootNode(), tree)
	n := 0
	visit := func() {
		node := c.CurrentNode()
		n += len(node.Type(lang)) + int(node.StartByte()) + int(node.EndByte()) + len(c.CurrentFieldName())
	}
	for done := false; !done; {
		visit()
		switch {
		case c.GotoFirstChild():
		case c.GotoNextSibling():
		default:
			for {
				if !c.GotoParent() {
					done = true
					break
				}
				if c.GotoNextSibling() {
					break
				}
			}
		}
	}
	return n
}

func b7WalkAC(tree *sitter.Tree) int {
	r := tree.RootNode()
	n := len(r.Kind()) + int(r.EndByte())
	for i := uint(0); i < r.ChildCount(); i++ {
		c := r.Child(i)
		n += len(c.Kind()) + int(c.StartByte()) + int(c.EndByte())
	}
	return n
}

func b7WalkBC(tree *sitter.Tree) int {
	c := tree.Walk()
	defer c.Close()
	n := 0
	visit := func() {
		node := c.Node()
		n += len(node.Kind()) + int(node.StartByte()) + int(node.EndByte()) + len(c.FieldName())
	}
	for done := false; !done; {
		visit()
		switch {
		case c.GotoFirstChild():
		case c.GotoNextSibling():
		default:
			for {
				if !c.GotoParent() {
					done = true
					break
				}
				if c.GotoNextSibling() {
					break
				}
			}
		}
	}
	return n
}

var b7Sink int

func b7Langs() []string {
	if v := os.Getenv("CCORE_B7_LANG"); v != "" {
		return strings.Split(v, ",")
	}
	return []string{"json", "python", "c", "typescript"}
}

// TestCcoreB7Counters compares, for the clean edit classes, the tokens lexed,
// the records reused and the bytes reused of the public API (route on) with C's
// parse log and with the core's own trace.
func TestCcoreB7Counters(t *testing.T) {
	if os.Getenv("CCORE_B7_LANG") == "" {
		t.Skip("set CCORE_B7_LANG")
	}
	defer ccoreRouteCertify(t)()
	cells := 0
	for _, lang := range b7Langs() {
		gl, cl := b7LangFor(t, lang)
		langCells := 0
		for _, sz := range b7Sizes {
			src := b7Doc(lang, sz.n)
			for _, pr := range b7Pairs(t, lang, src) {
				if pr.class == "noop" || pr.class == "breakrep" {
					continue
				}
				for step := 0; step < 2; step++ {
					// step 0: base -> alt; step 1: alt -> base (from a tree of alt)
					from, to, e, name := pr.base, pr.alt, pr.fwd, pr.fwdName
					if step == 1 {
						from, to, e, name = pr.alt, pr.base, pr.back, pr.backName
					}
					b7CountCell(t, lang, sz.name, name, gl, cl, from, to, e)
					langCells++
				}
			}
		}
		// Every cell must be there: a cell that was skipped would hide a difference.
		if langCells != b7ExpectedCells {
			t.Errorf("%s: %d counter cells compared, want %d", lang, langCells, b7ExpectedCells)
		}
		cells += langCells
		ccoreCount("TestCcoreB7Counters", lang, langCells)
	}
	for cell, why := range b7KnownCellDifferences {
		if !b7KnownSeen[cell] {
			t.Errorf("the known difference %s (%s) is gone: remove it from b7KnownCellDifferences", cell, why)
		}
	}
	_ = cells
}

// b7ExpectedCells is the number of cells of one language: three document sizes and, for each,
// the clean edit classes in both directions.
const b7ExpectedCells = 12

// b7KnownCellDifferences lists the cells (lang/size/class) whose counters cannot equal C's by
// design, with the reason. A cell in the list must differ, and the test fails when it no
// longer does. Empty: every cell of the four languages equals C.
var b7KnownCellDifferences = map[string]string{}

var b7KnownSeen = map[string]bool{}

func b7CountCell(t *testing.T, lang, size, class string, gl *g.Language, cl *sitter.Language, from, to []byte, e *bindtest.Edit) {
	// ours, through the public API
	p := g.NewParser(gl)
	p.SetAdmissionCandidateRoute(true)
	tree, err := p.Parse(from)
	if err != nil {
		t.Fatal(err)
	}
	tree.Edit(b7GoEdit(e))
	_, f0, i0 := bind.RouteCounters()
	_, i0u := f0, i0
	_, _, views0 := bind.ViewCounters()
	next, prof, err := p.ParseIncrementalProfiled(to, tree)
	if err != nil {
		t.Fatal(err)
	}
	_, f1, i1 := bind.RouteCounters()
	_, _, views1 := bind.ViewCounters()
	shortcutDelta := views1["shortcut_reparses"] - views0["shortcut_reparses"]
	shortcut := shortcutDelta == 1
	wantShortcut := ccoreViewsEnabled() && class == "same"
	if shortcutDelta < 0 || shortcutDelta > 1 || shortcut != wantShortcut {
		t.Errorf("%s/%s/%s: shortcut reparses=%d, want %t", lang, size, class, shortcutDelta, wantShortcut)
	}
	servedIncr := i1 - i0u
	tokOurs := next.ParseRuntime().TokensConsumed
	// the present engine on the same edit (route off): the counters the base has; reported, not asserted
	po := g.NewParser(gl)
	po.SetAdmissionCandidateRoute(false)
	offTree, err := po.Parse(from)
	if err != nil {
		t.Fatal(err)
	}
	offTree.Edit(b7GoEdit(e))
	offNext, offProf, err := po.ParseIncrementalProfiled(to, offTree)
	if err != nil {
		t.Fatal(err)
	}
	tokOff, reusedOff, bytesOff := offNext.ParseRuntime().TokensConsumed, offProf.ReusedSubtrees, offProf.ReusedBytes
	offNext.Release()
	if offTree != offNext {
		offTree.Release()
	}
	// C
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	var clog []string
	logOn := false
	cp.SetLogger(func(kind sitter.LogType, msg string) {
		if logOn && kind == sitter.LogTypeParse {
			clog = append(clog, msg)
		}
	})
	ct := cp.Parse(from, nil)
	ct.Edit(ccoreSitterEdit(e))
	logOn = true
	cn := cp.Parse(to, ct)
	logOn = false
	cTok, cReuse := ccoreCountLines(clog, "lexed_lookahead"), ccoreCountLines(clog, "reuse_node")
	// the core's own trace and reuse bytes
	core := ccoreNew(gl)
	core.Parse(from, nil)
	base := core.Tree()
	var slog []string
	edited := core.Edit(base, *e)
	_, stop := core.ParseIncremental(to, edited, func(s string) { slog = append(slog, s) })
	cn2, cb, _ := core.ReuseStats()
	first := -1
	for k := 0; k < len(clog) || k < len(slog); k++ {
		if k >= len(clog) || k >= len(slog) || clog[k] != slog[k] {
			first = k
			break
		}
	}
	fmt.Printf("CCB7CNT lang=%s size=%s class=%s shortcut=%t served_incr=%d fresh_served_delta=%d tokens_ours=%d tokens_c=%d tokens_core=%d records_ours=%d records_c=%d records_core=%d bytes_ours=%d bytes_core=%d bytes_c=trace_equal:%t trace_first_diff=%d stop=%q tokens_off=%d records_off=%d bytes_off=%d\n",
		lang, size, class, shortcut, servedIncr, f1-f0, tokOurs, cTok, ccoreCountLines(slog, "lexed_lookahead"), prof.ReusedSubtrees, cReuse, cn2, prof.ReusedBytes, cb, first < 0, first, stop, tokOff, reusedOff, bytesOff)
	if os.Getenv("CCORE_MUTATE_COUNTERS") != "" { // mutation check: one counter of ours is changed; the test must fail
		tokOurs++
	}
	cell := lang + "/" + size + "/" + class
	var diffs []string
	if servedIncr != 1 {
		diffs = append(diffs, fmt.Sprintf("the core served %d incremental parses, want 1", servedIncr))
	}
	if shortcut {
		if tokOurs < 1 || tokOurs > uint64(cTok) {
			diffs = append(diffs, fmt.Sprintf("tokens lexed: ours %d, C %d", tokOurs, cTok))
		}
		if prof.ReusedBytes < cb {
			diffs = append(diffs, fmt.Sprintf("bytes reused: ours %d, core %d", prof.ReusedBytes, cb))
		}
		if divs := compactT3StructuralDivergences(next.RootNode(), gl, cn.RootNode()); len(divs) != 0 {
			diffs = append(diffs, fmt.Sprintf("tree differs from C: %s", compactT3FormatDivergence(divs[0])))
		}
	} else {
		if int64(tokOurs) != int64(cTok) || int64(cTok) != int64(ccoreCountLines(slog, "lexed_lookahead")) {
			diffs = append(diffs, fmt.Sprintf("tokens lexed: ours %d, C %d, core %d", tokOurs, cTok, ccoreCountLines(slog, "lexed_lookahead")))
		}
		if int64(prof.ReusedSubtrees) != int64(cReuse) || int64(cReuse) != int64(cn2) {
			diffs = append(diffs, fmt.Sprintf("records reused: ours %d, C %d, core %d", prof.ReusedSubtrees, cReuse, cn2))
		}
		if int64(prof.ReusedBytes) != int64(cb) {
			diffs = append(diffs, fmt.Sprintf("bytes reused: ours %d, core %d", prof.ReusedBytes, cb))
		}
	}
	if first >= 0 {
		diffs = append(diffs, fmt.Sprintf("the core's trace differs from C's parse log at line %d", first))
	}
	if stop != "" {
		diffs = append(diffs, fmt.Sprintf("the core stopped: %q", stop))
	}
	if why, known := b7KnownCellDifferences[cell]; known {
		if len(diffs) > 0 {
			b7KnownSeen[cell] = true
			t.Logf("%s: known difference (%s): %v", cell, why, diffs)
		}
	} else if len(diffs) > 0 {
		t.Errorf("%s: %s", cell, strings.Join(diffs, "; "))
	}
	core.Release(edited)
	core.Release(base)
	cn.Close()
	ct.Close()
	next.Release()
	tree.Release()
}

// ---- timing ----

type b7Rec struct {
	lang, size, class, arm, part string
	ns                           []int64 // one value per round: the mean per step
}

type b7Parts struct{ edit, parse, walkA, walkB, core, conv int64 }

func b7Median(v []int64) int64 {
	s := append([]int64(nil), v...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[len(s)/2]
}

func b7Dockerps() string { return "" }

// TestCcoreB7Timing measures the three arms alternating inside this process.
func TestCcoreB7Timing(t *testing.T) {
	if os.Getenv("CCORE_B7_LANG") == "" {
		t.Skip("set CCORE_B7_LANG")
	}
	rounds := 7
	if v, err := strconv.Atoi(os.Getenv("CCORE_B7_ROUNDS")); err == nil && v > 0 {
		rounds = v
	}
	budget := 200 * time.Millisecond // target time of one arm in one round of one group
	defer ccoreRouteCertify(t)()
	only := os.Getenv("CCORE_B7_SIZES")
	for _, lang := range b7Langs() {
		gl, cl := b7LangFor(t, lang)
		cp := sitter.NewParser()
		if err := cp.SetLanguage(cl); err != nil {
			t.Fatal(err)
		}
		for _, sz := range b7Sizes {
			if only != "" && !strings.Contains(only, sz.name) {
				continue
			}
			src := b7Doc(lang, sz.n)
			for _, pr := range b7Pairs(t, lang, src) {
				b7Cell(t, lang, sz.name, gl, cp, pr, rounds, budget)
			}
		}
		cp.Close()
	}
}

func b7Cell(t *testing.T, lang, size string, gl *g.Language, cp *sitter.Parser, pr b7Pair, rounds int, budget time.Duration) {
	present := g.NewParser(gl)
	ours := g.NewParser(gl)
	ours.SetAdmissionCandidateRoute(true)
	arms := []string{"present", "tree", "c"}
	recs := map[string]*b7Rec{}
	key := func(class, arm, part string) *b7Rec {
		k := class + "|" + arm + "|" + part
		if recs[k] == nil {
			recs[k] = &b7Rec{lang: lang, size: size, class: class, arm: arm, part: part}
		}
		return recs[k]
	}
	steps := 2                                 // steps per arm per round; fixed after the warm-up round
	for round := -1; round < rounds; round++ { // round -1 is the warm-up
		for _, arm := range arms {
			var p *g.Parser
			var tree *g.Tree
			var ctree *sitter.Tree
			switch arm {
			case "present":
				p = present
			case "tree":
				p = ours
			}
			if arm == "c" {
				ctree = cp.Parse(pr.base, nil)
			} else {
				var err error
				if tree, err = p.Parse(pr.base); err != nil {
					t.Fatal(err)
				}
			}
			bind.RouteNanos(true)
			sums := map[string]*b7Parts{}
			counts := map[string]int{}
			var elapsed time.Duration
			n := steps
			for i := 0; i < n; i++ {
				forward := i%2 == 0
				to, e, class := pr.alt, pr.fwd, pr.fwdName
				if !forward {
					to, e, class = pr.base, pr.back, pr.backName
				}
				if pr.class == "noop" {
					to, e = pr.base, nil
				}
				var pt b7Parts
				t0 := time.Now()
				if arm == "c" {
					if e != nil {
						ctree.Edit(ccoreSitterEdit(e))
					}
					t1 := time.Now()
					cn := cp.Parse(to, ctree)
					t2 := time.Now()
					b7Sink += b7WalkAC(cn)
					t3 := time.Now()
					b7Sink += b7WalkBC(cn)
					t4 := time.Now()
					ctree.Close()
					ctree = cn
					pt = b7Parts{edit: int64(t1.Sub(t0)), parse: int64(t2.Sub(t1)), walkA: int64(t3.Sub(t2)), walkB: int64(t4.Sub(t3))}
				} else {
					if e != nil {
						tree.Edit(b7GoEdit(e))
					}
					t1 := time.Now()
					bind.RouteNanos(true)
					next, err := p.ParseIncremental(to, tree)
					t2 := time.Now()
					if err != nil || next == nil {
						t.Fatalf("%s %s %s %s: %v", lang, size, pr.class, arm, err)
					}
					cNs, vNs, _, _ := bind.RouteNanos(true)
					b7Sink += b7WalkAGo(next, gl)
					t3 := time.Now()
					b7Sink += b7WalkBGo(next, gl)
					t4 := time.Now()
					if next != tree {
						tree.Release()
						tree = next
					} else {
						next.Release() // the extra handle of an unchanged tree
					}
					pt = b7Parts{edit: int64(t1.Sub(t0)), parse: int64(t2.Sub(t1)), walkA: int64(t3.Sub(t2)), walkB: int64(t4.Sub(t3)), core: cNs, conv: vNs}
				}
				elapsed += time.Since(t0)
				s := sums[class]
				if s == nil {
					s = &b7Parts{}
					sums[class] = s
				}
				s.edit += pt.edit
				s.parse += pt.parse
				s.walkA += pt.walkA
				s.walkB += pt.walkB
				s.core += pt.core
				s.conv += pt.conv
				counts[class]++
			}
			if arm == "c" {
				ctree.Close()
			} else {
				tree.Release()
			}
			if round == -1 && arm == "present" {
				// choose the steps per round from the slowest-looking arm (the present engine) so all arms run the same count
				per := elapsed / time.Duration(n)
				if per <= 0 {
					per = time.Microsecond
				}
				steps = int(budget / per)
				if steps < 2 {
					steps = 2
				}
				if steps > 32 {
					steps = 32
				}
				steps &^= 1
			}
			if round < 0 {
				continue
			}
			for class, s := range sums {
				c := int64(counts[class])
				add := func(part string, v int64) { r := key(class, arm, part); r.ns = append(r.ns, v/c) }
				add("edit", s.edit)
				add("reparse", s.parse)
				add("walkA", s.walkA)
				add("walkB", s.walkB)
				add("total", s.edit+s.parse)
				add("total_walkA", s.edit+s.parse+s.walkA)
				add("total_walkB", s.edit+s.parse+s.walkB)
				if arm == "tree" {
					add("core_parsewith", s.core)
					add("convert", s.conv)
				}
			}
		}
	}
	keys := make([]string, 0, len(recs))
	for k := range recs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		r := recs[k]
		med := b7Median(r.ns)
		lo, hi := r.ns[0], r.ns[0]
		for _, v := range r.ns {
			if v < lo {
				lo = v
			}
			if v > hi {
				hi = v
			}
		}
		// TSV: language, size, edit class, arm, part, median ns, spread (max-min) ns, rounds, steps per round
		fmt.Printf("CCB7TSV\t%s\t%s\t%s\t%s\t%s\t%d\t%d\t%d\t%d\n", r.lang, r.size, r.class, r.arm, r.part, med, hi-lo, len(r.ns), steps)
	}
}

// TestCcoreB7Session is a 144-step edit session per language through the public
// API on a 32 KiB document whose edits keep the syntax valid (a digit replaced,
// a digit typed, a digit deleted), with a break and its repair every 12th pair of
// steps. Each step is compared with a fresh parse of the same bytes and, when the
// old tree has no error, with C's incremental result.
func TestCcoreB7Session(t *testing.T) {
	if os.Getenv("CCORE_B7_LANG") == "" {
		t.Skip("set CCORE_B7_LANG")
	}
	defer ccoreRouteCertify(t)()
	for _, lang := range b7Langs() {
		b7Session(t, lang)
	}
}

func b7FirstTreeDifference(a, b *g.Node, lang *g.Language, path string) string {
	if a == nil || b == nil {
		return fmt.Sprintf("path=%s incremental=%v fresh=%v", path, a, b)
	}
	av := fmt.Sprintf("type=%s bytes=%d-%d points=%v-%v named=%t extra=%t missing=%t children=%d", a.Type(lang), a.StartByte(), a.EndByte(), a.StartPoint(), a.EndPoint(), a.IsNamed(), a.IsExtra(), a.IsMissing(), a.ChildCount())
	bv := fmt.Sprintf("type=%s bytes=%d-%d points=%v-%v named=%t extra=%t missing=%t children=%d", b.Type(lang), b.StartByte(), b.EndByte(), b.StartPoint(), b.EndPoint(), b.IsNamed(), b.IsExtra(), b.IsMissing(), b.ChildCount())
	if av != bv {
		return fmt.Sprintf("path=%s incremental={%s} fresh={%s}", path, av, bv)
	}
	for i := 0; i < a.ChildCount(); i++ {
		if diff := b7FirstTreeDifference(a.Child(i), b.Child(i), lang, fmt.Sprintf("%s/%d", path, i)); diff != "" {
			return diff
		}
	}
	return ""
}

func b7Session(t *testing.T, lang string) {
	gl, cl := b7LangFor(t, lang)
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	// A second C parser for fresh parses: on a step with an error the route parses
	// fresh by its own rule, so C's fresh tree is the reference there.
	cfp := sitter.NewParser()
	defer cfp.Close()
	if err := cfp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	p := g.NewParser(gl)
	p.SetAdmissionCandidateRoute(true)
	fp := g.NewParser(gl)
	fp.SetAdmissionCandidateRoute(true)
	src := b7Doc(lang, 32<<10)
	cTree := cp.Parse(src, nil)
	tree, err := p.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(7))
	_, f0, i0 := bind.RouteCounters()
	steps, eqFresh, cmpC, eqC, diffNewErr, oldErrSteps, cleanClean := 0, 0, 0, 0, 0, 0, 0
	errSteps, eqCFresh := 0, 0
	var breakAt = -1
	var breakByte byte
	for i := 0; i < 144; i++ {
		var next []byte
		var e *bindtest.Edit
		if breakAt >= 0 { // repair
			next, e = b7Replace(src, breakAt, breakByte)
			breakAt = -1
		} else if i%12 == 5 { // break
			from := rng.Intn(len(src) / 2)
			var at int
			var ch byte
			if lang == "json" {
				at = strings.Index(string(src[from:]), "\": ") + from + 1
				ch = '#'
			} else {
				at = strings.Index(string(src[from:]), "return") + from
				ch = 'x'
			}
			breakAt, breakByte = at, src[at]
			next, e = b7Replace(src, at, ch)
		} else {
			at := rng.Intn(len(src) - 10)
			for at < len(src)-3 && !(src[at] >= '0' && src[at] <= '8' && src[at+1] >= '0' && src[at+1] <= '9') {
				at++
			}
			switch i % 3 {
			case 0:
				next, e = b7Replace(src, at, src[at]+1)
			case 1:
				next, e = b7Insert(src, at+1, byte('0'+rng.Intn(10)))
			default: // the second digit of a pair: the number stays valid (deleting the first could leave a leading zero)
				next, e = b7Delete(src, at+1)
			}
		}
		oldErr := tree.RootNode().HasError()
		cTree.Edit(ccoreSitterEdit(e))
		cn := cp.Parse(next, cTree)
		tree.Edit(b7GoEdit(e))
		nt, err := p.ParseIncremental(next, tree)
		if err != nil || nt == nil {
			t.Fatalf("%s step %d: %v", lang, i, err)
		}
		fresh, err := fp.Parse(next)
		if err != nil {
			t.Fatal(err)
		}
		steps++
		if ccoreLegacyDigest(nt, gl) == ccoreLegacyDigest(fresh, gl) {
			eqFresh++
		} else {
			ccoreExpect(t, false, "%s input=b7 step=%d: incremental differs from fresh: %s root_has_error_incremental=%t root_has_error_fresh=%t", lang, i, b7FirstTreeDifference(nt.RootNode(), fresh.RootNode(), gl, "root"), nt.RootNode().HasError(), fresh.RootNode().HasError())
		}
		divs := compactT3StructuralDivergences(nt.RootNode(), gl, cn.RootNode())
		newErr := nt.RootNode().HasError()
		switch {
		case oldErr:
			oldErrSteps++
		case len(divs) == 0:
			cmpC++
			eqC++
			if !newErr {
				cleanClean++
			}
		case newErr:
			cmpC++
			diffNewErr++
		default:
			cmpC++
			ccoreExpect(t, false, "%s input=b7 step=%d: %d divergences from C; first %s", lang, i, len(divs), compactT3FormatDivergence(divs[0]))
		}
		if oldErr || newErr {
			// The route does not reuse a tree with an error and does not keep an
			// incremental result with an error: it parses fresh. C's incremental tree
			// may differ from C's fresh tree on such a step, so the comparison above
			// cannot judge it. C's fresh parse of the same text can, and it does not
			// share a mistake with the Go parse used for the fresh comparison.
			errSteps++
			cf := cfp.Parse(next, nil)
			if d := compactT3StructuralDivergences(nt.RootNode(), gl, cf.RootNode()); len(d) == 0 {
				eqCFresh++
			} else {
				ccoreExpect(t, false, "%s input=b7 step=%d: %d divergences from C's fresh parse on a step with an error (old_error=%t new_error=%t); first %s", lang, i, len(d), oldErr, newErr, compactT3FormatDivergence(d[0]))
			}
			cf.Close()
		}
		fresh.Release()
		tree.Release()
		cTree.Close()
		tree, cTree, src = nt, cn, next
	}
	tree.Release()
	cTree.Close()
	decl, f1, i1 := bind.RouteCounters()
	fmt.Printf("CCB7SESSION lang=%s steps=%d equal_fresh=%d compared_with_c=%d equal_c=%d clean_old_clean_new_equal_c=%d old_clean_new_error_differs_from_c_incr=%d old_error_steps=%d error_steps=%d error_steps_equal_c_fresh=%d served_incr=%d served_fresh=%d declines=%v\n",
		lang, steps, eqFresh, cmpC, eqC, cleanClean, diffNewErr, oldErrSteps, errSteps, eqCFresh, i1-i0, f1-f0, decl)
	ccoreCount("TestCcoreB7Session", lang, steps)
	ccoreExpect(t, steps == 144 && eqFresh == steps && cmpC > 0 && eqC+diffNewErr == cmpC && errSteps > 0 && eqCFresh == errSteps && len(decl) == 0, "%s: session counts", lang)
}

func TestCcoreTSXOrdinaryEditPreservesParseState(t *testing.T) {
	lang, cLang := ccoreLanguages(t, "tsx")
	source, edited := []byte("a<T>y"), []byte("a<T>z")
	edit := g.InputEdit{
		StartByte: 4, OldEndByte: 5, NewEndByte: 5,
		StartPoint: g.Point{Column: 4}, OldEndPoint: g.Point{Column: 5}, NewEndPoint: g.Point{Column: 5},
	}

	parser := g.NewParser(lang)
	old, err := parser.Parse(source)
	if err != nil || old == nil {
		t.Fatalf("initial TSX parse: %v", err)
	}
	defer old.Release()

	reference := sitter.NewParser()
	defer reference.Close()
	if err := reference.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	oldC := reference.Parse(source, nil)
	if oldC == nil {
		t.Fatal("pinned C returned no initial TSX tree")
	}
	defer oldC.Close()

	oldOperator := oldC.RootNode().DescendantForByteRange(1, 2)
	if oldOperator == nil || oldOperator.Kind() != "<" {
		t.Fatal("initial C tree lacks the less-than token")
	}
	initialState := oldOperator.ParseState()
	old.Edit(edit)
	oldC.Edit(gradSitterEdit(edit))
	_, freshBefore, incrBefore := bind.RouteCounters()
	next, err := parser.ParseIncremental(edited, old)
	if err != nil || next == nil {
		t.Fatalf("incremental TSX parse: %v", err)
	}
	defer next.Release()
	_, freshAfter, incrAfter := bind.RouteCounters()
	if freshAfter != freshBefore || incrAfter != incrBefore+1 {
		t.Fatalf("expected one ccore incremental parse, got fresh=%d incremental=%d",
			freshAfter-freshBefore, incrAfter-incrBefore)
	}

	nextC := reference.Parse(edited, oldC)
	if nextC == nil {
		t.Fatal("pinned C returned no edited TSX tree")
	}
	defer nextC.Close()
	op := next.RootNode().DescendantForByteRange(1, 2)
	opC := nextC.RootNode().DescendantForByteRange(1, 2)
	if op == nil || opC == nil || op.Type(lang) != "<" || opC.Kind() != "<" {
		t.Fatal("incremental tree lacks the less-than token")
	}
	if want := opC.ParseState(); want == initialState || uint16(op.PreGotoState()) != want {
		t.Fatalf("less-than pre-goto state %d, pinned incremental C %d, initial C %d",
			op.PreGotoState(), want, initialState)
	}
}
