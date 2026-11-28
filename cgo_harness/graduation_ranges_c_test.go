//go:build cgo && treesitter_c_parity

package cgoharness

// Item 11 of the graduation gate: included ranges against pinned C.
//
// The Ranges bit of a certificate (certified.Ranges) lets the core serve a parse
// that has included ranges. No shipped certificate grants it, so every ranged
// parse of a certified grammar runs on the present engine. This item is what a
// grant would have to pass. It runs only for a grammar whose certificate carries
// the bit, or under --report --census, which certifies the grammar for the item
// with the bit added.
//
// For each range set below the test compares, with pinned C:
//
//	a. the fresh parse with ranges: the public tree and every subtree record;
//	b. a range-only reparse (the ranges change, the bytes do not) of that tree:
//	   the public tree and every subtree record;
//	c. a reparse after an edit inside the first range, with the ranges moved by
//	   the edit: the public tree and every subtree record.
//
// It also requires that the core serves each of these (counters), because a set
// that declines proves nothing about the core.
//
// The sets, from the committed sample of n bytes (offsets rounded up to the next
// line start unless the name says otherwise):
//
//	injection_two_ranges   [n/4, n/2) and [5n/8, 7n/8): both start at a positive offset
//	single_interior        [n/4, 3n/4)
//	prefix                 [0, n/2)
//	suffix_to_eof          [n/2, n)
//	three_ranges           three ranges with gaps
//	contiguous_two_ranges  [0, n/2) and [n/2, n): together the whole text
//	gap_of_blanks          two ranges around a run of blank bytes
//	mid_line_boundaries    raw offsets [n/5, 2n/5) and [3n/5, 4n/5), inside lines
//
// A scan provider (the table-v1 token source of c, cpp and java) is not used:
// the item measures the core's own lexer. A grammar whose ranges are reached
// through a token-source factory needs its own set of cases before it can claim
// the bit.

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/cgo_harness/internal/corecords"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
	"github.com/odvcencio/gotreesitter/internal/ccore/certified"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type gradRangeSet struct {
	name          string
	before, after [][2]int
}

// gradRangeSets builds the sets for a source. A set is left out when the source
// is too short for it to have non-empty, ordered ranges.
func gradRangeSets(src []byte) []gradRangeSet {
	n := len(src)
	align := func(off int) int {
		for i := off; i < n; i++ {
			if src[i] == '\n' {
				return i + 1
			}
		}
		return n
	}
	valid := func(spans [][2]int) bool {
		prev := 0
		for _, s := range spans {
			if s[0] < prev || s[0] >= s[1] || s[1] > n {
				return false
			}
			prev = s[1]
		}
		return len(spans) > 0
	}
	grow := func(spans [][2]int) [][2]int {
		out := append([][2]int(nil), spans...)
		last := len(out) - 1
		if end := out[last][1] + 32; end <= n {
			out[last][1] = end
			return out
		}
		// No room to grow at the end of the text: trim the last range instead.
		if trim := (out[last][1] - out[last][0]) / 4; trim > 0 {
			out[last][1] -= trim
		}
		return out
	}
	var out []gradRangeSet
	add := func(name string, spans [][2]int) {
		if valid(spans) {
			out = append(out, gradRangeSet{name: name, before: spans, after: grow(spans)})
		}
	}
	add("injection_two_ranges", [][2]int{{align(n / 4), align(n / 2)}, {align(5 * n / 8), align(7 * n / 8)}})
	add("single_interior", [][2]int{{align(n / 4), align(3 * n / 4)}})
	add("prefix", [][2]int{{0, align(n / 2)}})
	add("suffix_to_eof", [][2]int{{align(n / 2), n}})
	add("three_ranges", [][2]int{{align(n / 8), align(2 * n / 8)}, {align(3 * n / 8), align(4 * n / 8)}, {align(5 * n / 8), align(6 * n / 8)}})
	// Two ranges that cover the whole text between them: the tree is the
	// document's, so it is clean when the sample is.
	add("contiguous_two_ranges", [][2]int{{0, align(n / 2)}, {align(n / 2), n}})
	// Two ranges around a gap of blanks: the omitted bytes are whitespace only.
	if i := gradBlankRun(src); i > 0 {
		add("gap_of_blanks", [][2]int{{0, i}, {i + gradBlankLen(src, i), n}})
	}
	add("mid_line_boundaries", [][2]int{{n / 5, 2 * n / 5}, {3 * n / 5, 4 * n / 5}})
	return out
}

// gradBlankRun returns the offset of the first run of at least two blank bytes
// (space, tab or newline) after a quarter of the source, or 0.
func gradBlankRun(src []byte) int {
	blank := func(b byte) bool { return b == ' ' || b == '\t' || b == '\n' }
	for i := len(src) / 4; i+1 < len(src); i++ {
		if blank(src[i]) && blank(src[i+1]) && (i == 0 || !blank(src[i-1])) {
			return i
		}
	}
	return 0
}

func gradBlankLen(src []byte, at int) int {
	n := 0
	for at+n < len(src) && (src[at+n] == ' ' || src[at+n] == '\t' || src[at+n] == '\n') {
		n++
	}
	return n
}

func gradGoRanges(src []byte, spans [][2]int) []g.Range {
	core, _ := coreAndCIncludedRanges(src, spans)
	out := make([]g.Range, len(core))
	for i, r := range core {
		out[i] = g.Range{StartByte: r.StartByte, EndByte: r.EndByte,
			StartPoint: g.Point{Row: r.StartRow, Column: r.StartCol}, EndPoint: g.Point{Row: r.EndRow, Column: r.EndCol}}
	}
	return out
}

// gradShift moves the spans after an insert of one byte at at (the end of the first span).
func gradShift(spans [][2]int, at int) [][2]int {
	out := make([][2]int, len(spans))
	for i, s := range spans {
		if s[0] >= at {
			s[0]++
		}
		if s[1] >= at {
			s[1]++
		}
		out[i] = s
	}
	return out
}

type gradRangeTally struct {
	steps, served, incr, equal, recordsEqual int
	declined                                 int
	first                                    string
}

func (r *gradRangeTally) note(id, what string) {
	if r.first == "" {
		r.first = id + ": " + what
	}
}

// compare checks one route tree against C's: public tree and subtree records.
func (r *gradRangeTally) compare(c *gradCtx, id string, tree *g.Tree, cTree *sitter.Tree) {
	n, why := gradFirstDiv(tree, c, cTree)
	if gradNeg(11) {
		n, why = n+1, "corrupted expectation"
	}
	if n == 0 {
		r.equal++
	} else {
		r.note(id, fmt.Sprintf("route tree differs from C: %d differences, first %s", n, why))
	}
	ch, ccount, rc := corecords.Hash(cTree, "")
	gh, gn := bind.CompactRecords(tree, nil)
	if rc == 0 && ccount > 0 && gn > 0 && ch == gh && ccount == gn && !gradNeg(11) {
		r.recordsEqual++
	} else {
		r.note(id, fmt.Sprintf("records route=%x/%d C=%x/%d C_status=%d", gh, gn, ch, ccount, rc))
	}
}

// gradRouteMoves names the decline and fallback counters that moved since before.
func gradRouteMoves(beforeDecl, beforeFall map[string]int64) string {
	var parts []string
	d, _, _ := bind.RouteCounters()
	for k, v := range d {
		if v > beforeDecl[k] {
			parts = append(parts, "decline "+k)
		}
	}
	for k, v := range bind.IncrFallbacks() {
		if v > beforeFall[k] {
			parts = append(parts, "fresh because "+k)
		}
	}
	sort.Strings(parts)
	if len(parts) == 0 {
		return "no counter named a reason"
	}
	return strings.Join(parts, ", ")
}

func gradSnapshot() (map[string]int64, map[string]int64) {
	d, _, _ := bind.RouteCounters()
	return gradCopyMap(d), gradCopyMap(bind.IncrFallbacks())
}

func TestGraduationRanges(t *testing.T) {
	c := gradLoad(t)
	base, f0, i0 := bind.RouteCounters()
	base = gradCopyMap(base)
	if gradCensus() {
		// The census certifies the grammar for this item with the Ranges bit
		// added. A certificate that already carries it needs no help.
		restore, err := bindtest.CertifyForTests(certified.Fresh|certified.Recover|certified.Scan|certified.Incr|certified.Ranges,
			[]string{"ImmediateTokens", "ZeroWidthTokens", "LayoutFallbackLexState", "AfterWhitespaceLexState"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer restore()
	} else {
		sha, _ := c.lang.GrammarBlobSHA256()
		if certified.Blobs[sha].Modes&certified.Ranges == 0 {
			gradResult(11, "na", "sets=0", "the certificate does not grant Ranges; run with --report --census to measure")
			return
		}
	}
	sample, _ := gradSample(t, c.name)
	sets := gradRangeSets(sample)
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(c.cLang); err != nil {
		t.Fatal(err)
	}
	setC := func(spans [][2]int) {
		t.Helper()
		_, cr := coreAndCIncludedRanges(sample, spans)
		if err := cp.SetIncludedRanges(cr); err != nil {
			t.Fatal(err)
		}
	}
	// tallies[phase][kind]: phases are the fresh parse, the range-only reparse and
	// the reparse after an edit; kind 0 is a step whose C trees (old and result) have
	// no error, kind 1 a step where either has one. A reparse of an error tree is a fresh parse on
	// the core today (the error-edit limit), so the incremental phases of kind 1 are
	// reported and do not gate; the fresh phase gates both kinds.
	var tallies [3][2]gradRangeTally
	used := 0
	moved := func(f0, i0 int64) (fresh, incr int64) {
		_, f1, i1 := bind.RouteCounters()
		return f1 - f0, i1 - i0
	}
	for _, set := range sets {
		setC(set.before)
		cOld := cp.Parse(sample, nil)
		if cOld == nil || cOld.RootNode() == nil {
			continue // C produced no tree for this set: it compares nothing
		}
		used++
		kind := 0
		if cOld.RootNode().HasError() {
			kind = 1
		}
		// a. The fresh parse with ranges.
		p := gradRoute(c.lang)
		p.SetIncludedRanges(gradGoRanges(sample, set.before))
		_, fa, ia := bind.RouteCounters()
		tree, err := p.Parse(sample)
		if err != nil || tree == nil {
			t.Fatalf("%s: parse: %v", set.name, err)
		}
		fr := &tallies[0][kind]
		fr.steps++
		if df, _ := moved(fa, ia); df != 1 {
			fr.declined++
			fr.note(set.name, "fresh parse with ranges was not served by the core")
			tree.Release()
			cOld.Close()
			continue
		}
		fr.served++
		fr.compare(c, set.name, tree, cOld)

		// b. Range-only reparse: same bytes, new ranges.
		setC(set.after)
		cNext := cp.Parse(sample, cOld)
		if cNext == nil {
			t.Fatalf("%s: C returned no range-only reparse", set.name)
		}
		p.SetIncludedRanges(gradGoRanges(sample, set.after))
		_, fa, ia = bind.RouteCounters()
		dBefore, fBefore := gradSnapshot()
		next, err := p.ParseIncremental(sample, tree)
		if err != nil || next == nil {
			t.Fatalf("%s: range-only reparse: %v", set.name, err)
		}
		rkind := kind
		if cNext.RootNode().HasError() {
			rkind = 1 // the result has an error: C reused, the route reparses fresh (result_has_error)
		}
		rr := &tallies[1][rkind]
		rr.steps++
		if df, di := moved(fa, ia); df+di != 1 {
			rr.declined++
			rr.note(set.name, "range-only reparse was not served by the core: "+gradRouteMoves(dBefore, fBefore))
		} else {
			rr.served++
			rr.incr += int(di)
			rr.compare(c, set.name+"/range-only", next, cNext)
		}
		next.Release()
		if tree != next {
			tree.Release()
		}
		cNext.Close()
		cOld.Close()

		// c. An edit inside the first range, ranges moved with the bytes.
		at := set.before[0][1] - 1
		if at < set.before[0][0] || at >= len(sample) {
			continue
		}
		edit := g.InputEdit{StartByte: uint32(at), OldEndByte: uint32(at), NewEndByte: uint32(at + 1),
			StartPoint: gradPoint(sample, at), OldEndPoint: gradPoint(sample, at)}
		after := append(append(append([]byte(nil), sample[:at]...), ' '), sample[at:]...)
		edit.NewEndPoint = gradPoint(after, at+1)
		shifted := gradShift(set.before, at)
		setC(set.before)
		cBase := cp.Parse(sample, nil)
		if cBase == nil {
			continue
		}
		p2 := gradRoute(c.lang)
		p2.SetIncludedRanges(gradGoRanges(sample, set.before))
		old, err := p2.Parse(sample)
		if err != nil || old == nil {
			t.Fatalf("%s: edit session base: %v", set.name, err)
		}
		cBase.Edit(gradSitterEdit(edit))
		_, mc := coreAndCIncludedRanges(after, shifted)
		if err := cp.SetIncludedRanges(mc); err != nil {
			t.Fatal(err)
		}
		cEdited := cp.Parse(after, cBase)
		old.Edit(edit)
		p2.SetIncludedRanges(gradGoRanges(after, shifted))
		_, fa, ia = bind.RouteCounters()
		dBefore, fBefore = gradSnapshot()
		eTree, err := p2.ParseIncremental(after, old)
		if err != nil || eTree == nil || cEdited == nil {
			t.Fatalf("%s: edit in range: %v", set.name, err)
		}
		ekind := kind
		if cEdited.RootNode().HasError() {
			ekind = 1
		}
		er := &tallies[2][ekind]
		er.steps++
		if df, di := moved(fa, ia); df+di != 1 {
			er.declined++
			er.note(set.name, "reparse after an edit in range was not served by the core: "+gradRouteMoves(dBefore, fBefore))
		} else {
			er.served++
			er.incr += int(di)
			er.compare(c, set.name+"/edit-in-range", eTree, cEdited)
		}
		eTree.Release()
		if old != eTree {
			old.Release()
		}
		cEdited.Close()
		cBase.Close()
	}

	// The verdict. Gating: the fresh phase of every set, and the two reparse
	// phases of the sets whose C tree is clean. Each must be served by the core
	// and equal C's tree and records. At least one clean set must have been
	// reparsed incrementally by the core, or the reuse of a ranged tree was not
	// measured.
	status, first := "pass", ""
	gate := []*gradRangeTally{&tallies[0][0], &tallies[0][1], &tallies[1][0], &tallies[2][0]}
	for _, r := range gate {
		if r.served != r.steps || r.equal != r.served || r.recordsEqual != r.served {
			status = "fail"
		}
		if first == "" {
			first = r.first
		}
	}
	cleanIncr := tallies[1][0].incr + tallies[2][0].incr
	switch {
	case used == 0:
		status, first = "na", "no range set produced a C tree"
	case status != "fail" && cleanIncr == 0:
		status, first = "na", "no clean range set was reparsed incrementally by the core"
	}
	nums := []string{fmt.Sprintf("sets=%d", used)}
	phases := []string{"fresh", "range_only", "edit_in_range"}
	kinds := []string{"clean", "error"}
	for i, ph := range phases {
		for k, kd := range kinds {
			r := tallies[i][k]
			nums = append(nums, fmt.Sprintf("%s_%s_steps=%d %s_%s_served=%d %s_%s_incremental=%d %s_%s_tree_equals_c=%d %s_%s_records_equal_c=%d",
				ph, kd, r.steps, ph, kd, r.served, ph, kd, r.incr, ph, kd, r.equal, ph, kd, r.recordsEqual))
		}
	}
	gradResult(11, status, strings.Join(nums, " "), first)
	gradPrintDecl(11, base, f0, i0)
}
