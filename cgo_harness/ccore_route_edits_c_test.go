//go:build cgo && treesitter_c_parity

package cgoharness

// Facts about the edit sessions that the byte-for-byte tree comparison does not see:
// the changed ranges after an edit and a reparse, and the edited flag (Node.HasChanges)
// of every node. Each is compared three ways: the route (core, converted tree), the
// present engine with the route off, and pinned C.

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// ccoreSpan is a half-open byte interval.
type ccoreSpan struct{ s, e uint32 }

// ccoreSpansOf returns the merged, sorted, non-empty byte intervals of ranges.
func ccoreSpansOf(ranges [][2]uint32) []ccoreSpan {
	var in []ccoreSpan
	for _, r := range ranges {
		if r[1] > r[0] {
			in = append(in, ccoreSpan{r[0], r[1]})
		}
	}
	sort.Slice(in, func(i, j int) bool { return in[i].s < in[j].s })
	var out []ccoreSpan
	for _, r := range in {
		if n := len(out); n > 0 && r.s <= out[n-1].e {
			if r.e > out[n-1].e {
				out[n-1].e = r.e
			}
			continue
		}
		out = append(out, r)
	}
	return out
}

func ccoreGoRanges(rs []g.Range) [][2]uint32 {
	out := make([][2]uint32, len(rs))
	for i, r := range rs {
		out[i] = [2]uint32{r.StartByte, r.EndByte}
	}
	return out
}

func ccoreCRanges(rs []sitter.Range) [][2]uint32 {
	out := make([][2]uint32, len(rs))
	for i, r := range rs {
		out[i] = [2]uint32{uint32(r.StartByte), uint32(r.EndByte)}
	}
	return out
}

func ccoreSpanBytes(sp []ccoreSpan) (n uint32) {
	for _, r := range sp {
		n += r.e - r.s
	}
	return n
}

// ccoreIntersectBytes is the number of bytes in both span lists.
func ccoreIntersectBytes(a, b []ccoreSpan) (n uint32) {
	for _, x := range a {
		for _, y := range b {
			lo, hi := max(x.s, y.s), min(x.e, y.e)
			if hi > lo {
				n += hi - lo
			}
		}
	}
	return n
}

// Relations of the bytes of ranges A to the bytes of ranges B.
const (
	ccoreRelSame   = "same_bytes"          // the same bytes
	ccoreRelLarger = "a_covers_b_and_more" // A holds every byte of B and more
	ccoreRelMisses = "a_misses_b_bytes"    // B has a byte that A lacks
)

func ccoreRelation(a, b [][2]uint32) string {
	sa, sb := ccoreSpansOf(a), ccoreSpansOf(b)
	common := ccoreIntersectBytes(sa, sb)
	switch {
	case common < ccoreSpanBytes(sb):
		return ccoreRelMisses
	case ccoreSpanBytes(sa) > common:
		return ccoreRelLarger
	}
	return ccoreRelSame
}

func ccoreShowRanges(rs [][2]uint32) string {
	var b strings.Builder
	for i, r := range rs {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%d-%d", r[0], r[1])
	}
	return b.String()
}

// ccoreRangeFacts counts, over the steps of a session, the relation of each pair.
type ccoreRangeFacts struct {
	steps                            int
	identical                        map[string]int // pair -> steps whose range lists are identical
	rel                              map[string]int // "pair/relation" -> steps
	first                            map[string]string
	missSteps                        []string // steps where the route misses a byte C reports
	missRouteEqPresent               bool     // at every such step the route\'s ranges equal the present engine\'s
	emptyRoute, emptyPresent, emptyC int
}

func newCcoreRangeFacts() *ccoreRangeFacts {
	return &ccoreRangeFacts{missRouteEqPresent: true, identical: map[string]int{}, rel: map[string]int{}, first: map[string]string{}}
}

func ccoreSameRangeLists(a, b [][2]uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// add records one step. route, present and c are the byte ranges of the three engines; cFresh
// is C's answer when its old tree is a fresh parse of the old bytes (C's answer with the
// history of the session is c).
func (f *ccoreRangeFacts) add(id string, route, present, c, cFresh [][2]uint32) {
	f.steps++
	if ccoreRelation(route, c) == ccoreRelMisses {
		f.missSteps = append(f.missSteps, id)
		f.missRouteEqPresent = f.missRouteEqPresent && ccoreSameRangeLists(route, present)
	}
	if len(route) == 0 {
		f.emptyRoute++
	}
	if len(present) == 0 {
		f.emptyPresent++
	}
	if len(c) == 0 {
		f.emptyC++
	}
	for _, p := range []struct {
		name string
		a, b [][2]uint32
	}{{"route_vs_c", route, c}, {"route_vs_present", route, present}, {"present_vs_c", present, c}, {"route_vs_c_fresh_old", route, cFresh}, {"present_vs_c_fresh_old", present, cFresh}} {
		if ccoreSameRangeLists(p.a, p.b) {
			f.identical[p.name]++
		}
		rel := p.name + "/" + ccoreRelation(p.a, p.b)
		f.rel[rel]++
		if _, ok := f.first[rel]; !ok {
			f.first[rel] = fmt.Sprintf("%s route=[%s] present=[%s] c=[%s]", id, ccoreShowRanges(route), ccoreShowRanges(present), ccoreShowRanges(c))
		}
	}
}

func (f *ccoreRangeFacts) report(set string) {
	fmt.Printf("CCRANGEFACTS set=%s steps=%d empty_route=%d empty_present=%d empty_c=%d", set, f.steps, f.emptyRoute, f.emptyPresent, f.emptyC)
	for _, pair := range []string{"route_vs_c", "route_vs_present", "present_vs_c", "route_vs_c_fresh_old", "present_vs_c_fresh_old"} {
		fmt.Printf(" %s.identical=%d", pair, f.identical[pair])
		for _, rel := range []string{ccoreRelSame, ccoreRelLarger, ccoreRelMisses} {
			fmt.Printf(" %s.%s=%d", pair, rel, f.rel[pair+"/"+rel])
		}
	}
	fmt.Println()
	keys := make([]string, 0, len(f.first))
	for k := range f.first {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("CCRANGEFIRST set=%s %s first=%s\n", set, k, f.first[k])
	}
}

// ccoreEditClass places a node [s,e) relative to the edited region [start,newEnd) of the
// edited tree (a node of the edited tree is in the new coordinates).
func ccoreEditClass(s, e, start, newEnd uint32) string {
	empty := newEnd <= start
	switch {
	case !empty && s <= start && e >= newEnd, empty && s < start && e > start:
		return "contains_edit"
	case !empty && s < newEnd && e > start:
		return "overlaps_edit"
	case e == start:
		return "ends_at_edit_start"
	case s == newEnd:
		return "starts_at_edit_end"
	case e < start:
		return "before_edit"
	}
	return "after_edit"
}

// ccoreFlagFacts counts the nodes whose edited flag differs between two trees.
type ccoreFlagFacts struct {
	nodes, shapeMismatch int
	aOnly, bOnly         int            // flag set only in the first (route or present) / only in the second
	aSet, bSet           int            // nodes with the flag set in each
	byClass              map[string]int // "<direction>/<class>" -> nodes
}

func newCcoreFlagFacts() *ccoreFlagFacts { return &ccoreFlagFacts{byClass: map[string]int{}} }

func (f *ccoreFlagFacts) note(a, b bool, s, e uint32, ed *bindtest.Edit) {
	f.nodes++
	if a {
		f.aSet++
	}
	if b {
		f.bSet++
	}
	if a == b {
		return
	}
	dir := "first_only"
	if b {
		dir = "second_only"
		f.bOnly++
	} else {
		f.aOnly++
	}
	f.byClass[dir+"/"+ccoreEditClass(s, e, ed.StartByte, ed.NewEndByte)]++
}

// ccoreFlagsGoC compares Node.HasChanges of a public tree (first) with C (second).
func ccoreFlagsGoC(f *ccoreFlagFacts, gn *g.Node, cn *sitter.Node, ed *bindtest.Edit) {
	f.note(gn.HasChanges(), cn.HasChanges(), gn.StartByte(), gn.EndByte(), ed)
	n := gn.ChildCount()
	if n != int(cn.ChildCount()) {
		f.shapeMismatch++
		return
	}
	for i := 0; i < n; i++ {
		ccoreFlagsGoC(f, gn.Child(i), cn.Child(uint(i)), ed)
	}
}

// ccoreFlagsGoGo compares Node.HasChanges of two public trees (first, second).
func ccoreFlagsGoGo(f *ccoreFlagFacts, a, b *g.Node, ed *bindtest.Edit) {
	f.note(a.HasChanges(), b.HasChanges(), a.StartByte(), a.EndByte(), ed)
	n := a.ChildCount()
	if n != b.ChildCount() {
		f.shapeMismatch++
		return
	}
	for i := 0; i < n; i++ {
		ccoreFlagsGoGo(f, a.Child(i), b.Child(i), ed)
	}
}

func (f *ccoreFlagFacts) report(set, pair, when string) {
	fmt.Printf("CCFLAGFACTS set=%s pair=%s when=%s nodes=%d shape_mismatch=%d first_set=%d second_set=%d first_only=%d second_only=%d", set, pair, when, f.nodes, f.shapeMismatch, f.aSet, f.bSet, f.aOnly, f.bOnly)
	keys := make([]string, 0, len(f.byClass))
	for k := range f.byClass {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf(" %s=%d", strings.ReplaceAll(k, "/", "."), f.byClass[k])
	}
	fmt.Println()
}

// ccoreChainProbe plays the versions as one edit chain (a fresh parse of versions[0], then
// an edit and an incremental reparse per version) on C, on the route and on the present
// engine, and returns the changed ranges of the last step. ok is false when a parse has an
// error or fails, (a version with a syntax error is allowed: the sessions have them).
func ccoreChainProbe(cp *sitter.Parser, lang *g.Language, route, present *g.Parser, versions [][]byte) (routeR, presentR, cR, cFreshR [][2]uint32, ok bool) {
	last := len(versions) - 1
	cTree := cp.Parse(versions[0], nil)
	var trees [2]*g.Tree
	for i, p := range []*g.Parser{route, present} {
		tr, err := p.Parse(versions[0])
		if err != nil || tr == nil {
			cTree.Close()
			return nil, nil, nil, nil, false
		}
		trees[i] = tr
	}
	for k := 1; k <= last; k++ {
		e := ccoreDiffEdit(versions[k-1], versions[k])
		gedit := g.InputEdit{
			StartByte: e.StartByte, OldEndByte: e.OldEndByte, NewEndByte: e.NewEndByte,
			StartPoint:  g.Point{Row: e.StartRow, Column: e.StartCol},
			OldEndPoint: g.Point{Row: e.OldEndRow, Column: e.OldEndCol},
			NewEndPoint: g.Point{Row: e.NewEndRow, Column: e.NewEndCol},
		}
		cTree.Edit(ccoreSitterEdit(e))
		var cFresh [][2]uint32
		if k == last { // C with a history-free old tree: a fresh parse of the previous version
			fo := cp.Parse(versions[k-1], nil)
			fo.Edit(ccoreSitterEdit(e))
			fn := cp.Parse(versions[k], fo)
			cFresh = ccoreCRanges(fo.ChangedRanges(fn))
			fn.Close()
			fo.Close()
		}
		cn := cp.Parse(versions[k], cTree)
		if k == last {
			cR, cFreshR = ccoreCRanges(cTree.ChangedRanges(cn)), cFresh
		}
		cTree.Close()
		cTree = cn
		for i, p := range []*g.Parser{route, present} {
			trees[i].Edit(gedit)
			next, err := p.ParseIncremental(versions[k], trees[i])
			if err != nil || next == nil {
				cTree.Close()
				return nil, nil, nil, nil, false
			}
			if k == last {
				r := ccoreGoRanges(g.DiffChangedRanges(trees[i], next))
				if i == 0 {
					routeR = r
				} else {
					presentR = r
				}
			}
			if next != trees[i] {
				trees[i].Release()
			}
			trees[i] = next
		}
	}
	cTree.Close()
	trees[0].Release()
	trees[1].Release()
	return routeR, presentR, cR, cFreshR, true
}

// TestCcoreRouteRangeMissReduce finds the steps of a session where the route's changed
// ranges miss a byte that C reports, and reduces the first such step: first the history (the
// latest step j such that the chain from version j still misses at the step), then the lines
// before the first and after the last edited line (CCORE_RANGE_REDUCE=<lang>-session).
func TestCcoreRouteRangeMissReduce(t *testing.T) {
	set := os.Getenv("CCORE_RANGE_REDUCE")
	if set == "" {
		t.Skip("set CCORE_RANGE_REDUCE")
	}
	defer ccoreRouteCertify(t)()
	langName := strings.TrimSuffix(set, "-session")
	lang := grammars.DetectLanguageByName(langName).Language()
	cLang, err := ParityCLanguage(langName)
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	route := g.NewParser(lang)
	route.SetAdmissionCandidateRoute(true)
	present := g.NewParser(lang)
	present.SetAdmissionCandidateRoute(false)
	misses := func(vs [][]byte) bool {
		r, _, c, _, ok := ccoreChainProbe(cp, lang, route, present, vs)
		return ok && ccoreRelation(r, c) == ccoreRelMisses
	}
	found := 0
	for _, chain := range ccoreIncChains(t, set) {
		var versions [][]byte
		for _, st := range chain {
			versions = append(versions, st.src)
		}
		for i := 1; i < len(chain); i++ {
			if !misses(versions[:i+1]) {
				continue
			}
			found++
			r, pr, c, cf, _ := ccoreChainProbe(cp, lang, route, present, versions[:i+1])
			e := chain[i].edit
			fmt.Printf("CCRANGEMISS set=%s step=%s edit=[%d,%d)->[%d,%d) route=[%s] present=[%s] c=[%s] c_with_fresh_old_tree=[%s] route_vs_c_fresh=%s\n", set, chain[i].id, e.StartByte, e.OldEndByte, e.StartByte, e.NewEndByte, ccoreShowRanges(r), ccoreShowRanges(pr), ccoreShowRanges(c), ccoreShowRanges(cf), ccoreRelation(r, cf))
			if found > 1 {
				continue
			}
			// 1. the shortest history
			j := i - 1
			for j-1 >= 0 && !misses(versions[j:i+1]) {
				j--
			}
			hist := versions[j : i+1]
			fmt.Printf("CCRANGEHISTORY set=%s step=%s shortest_history_start=version%d edits_in_history=%d\n", set, chain[i].id, j, len(hist)-1)
			// 2. the lines before the first and after the last edit, common to all versions in the history
			base := strings.SplitAfter(string(hist[0]), "\n")
			pre, suf := len(base), len(base)
			for _, v := range hist[1:] {
				ls := strings.SplitAfter(string(v), "\n")
				k := 0
				for k < len(base) && k < len(ls) && base[k] == ls[k] {
					k++
				}
				pre = min(pre, k)
				k = 0
				for k < len(base)-pre && k < len(ls)-pre && base[len(base)-1-k] == ls[len(ls)-1-k] {
					k++
				}
				suf = min(suf, k)
			}
			total := pre + suf
			keep := make([]bool, total)
			for k := range keep {
				keep[k] = true
			}
			build := func() [][]byte {
				out := make([][]byte, len(hist))
				for vi, v := range hist {
					ls := strings.SplitAfter(string(v), "\n")
					var b strings.Builder
					for k := 0; k < pre; k++ {
						if keep[k] {
							b.WriteString(ls[k])
						}
					}
					for k := pre; k < len(ls)-suf; k++ {
						b.WriteString(ls[k])
					}
					for k := 0; k < suf; k++ {
						if keep[pre+k] {
							b.WriteString(ls[len(ls)-suf+k])
						}
					}
					out[vi] = []byte(b.String())
				}
				return out
			}
			for size := (total + 1) / 2; size >= 1; size /= 2 {
				for start := 0; start < total; start += size {
					saved := append([]bool(nil), keep...)
					for k := start; k < start+size && k < total; k++ {
						keep[k] = false
					}
					if !misses(build()) {
						copy(keep, saved)
					}
				}
				if size == 1 {
					break
				}
			}
			vs := build()
			r2, pr2, c2, cf2, _ := ccoreChainProbe(cp, lang, route, present, vs)
			fmt.Printf("CCRANGEMINIMAL set=%s step=%s versions=%d first_version_bytes=%d route=[%s] present=[%s] c=[%s] c_with_fresh_old_tree=[%s]\n", set, chain[i].id, len(vs), len(vs[0]), ccoreShowRanges(r2), ccoreShowRanges(pr2), ccoreShowRanges(c2), ccoreShowRanges(cf2))
			for vi, v := range vs {
				fmt.Printf("CCRANGEMINIMAL-VERSION %d %q\n", vi, v)
			}
		}
	}
	fmt.Printf("CCRANGEMISSCOUNT set=%s steps_missing=%d\n", set, found)
}

// counts summarizes the range comparisons for a session.
func (f *ccoreRangeFacts) counts() map[string]string {
	n := func(k string) string { return fmt.Sprint(f.rel[k]) }
	miss := "none"
	if len(f.missSteps) > 0 {
		miss = strings.Join(f.missSteps, "+")
	}
	return map[string]string{
		"steps":  fmt.Sprint(f.steps),
		"c_same": n("route_vs_c/" + ccoreRelSame), "c_larger": n("route_vs_c/" + ccoreRelLarger), "c_misses": n("route_vs_c/" + ccoreRelMisses),
		"c_miss_steps": miss,
		"p_same":       n("route_vs_present/" + ccoreRelSame), "p_larger": n("route_vs_present/" + ccoreRelLarger), "p_misses": n("route_vs_present/" + ccoreRelMisses),
		"pc_same": n("present_vs_c/" + ccoreRelSame), "pc_larger": n("present_vs_c/" + ccoreRelLarger), "pc_misses": n("present_vs_c/" + ccoreRelMisses),
	}
}

// ccoreCheckListed requires the route's answers to match C.
func ccoreCheckListed(t testing.TB, kind, lang string, got map[string]string) {
	t.Helper()
	keys := []string{"c_larger", "c_misses"}
	if kind == "flags" {
		keys = []string{"set_only_in_c", "set_only_in_route", "after_reparse_set"}
	}
	for _, key := range keys {
		if got[key] != "0" {
			t.Errorf("%s %s: %s=%s differs from C", kind, lang, key, got[key])
		}
	}
}

// Compare changed ranges on an input with an indentation edit.
func TestCcoreKnownDifferenceDiffChangedRangesMissesBytesThatCReports(t *testing.T) {
	if os.Getenv("CCORE_LANG") != "python" {
		t.Skip("set CCORE_LANG=python")
	}
	defer ccoreRouteCertify(t)()
	lang := grammars.DetectLanguageByName("python").Language()
	cLang, err := ParityCLanguage("python")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	route := g.NewParser(lang)
	route.SetAdmissionCandidateRoute(true)
	present := g.NewParser(lang)
	present.SetAdmissionCandidateRoute(false)
	oldSrc := []byte("        writer = DecodeMapWriter(fpx \"cp949ext\", uhcdecmap)\n        writer.updat_decodxe_map(UHCL1_C1, UHCL1_C2)\n")
	newSrc := []byte("        writer = DecodeMapWriter(fpx \"cp949ext\", uhcdecmap)\n       xwriter.updat_decodxe_map(UHCL1_C1, UHCL1_C2)\n")
	r, p, c, _, ok := ccoreChainProbe(cp, lang, route, present, [][]byte{oldSrc, newSrc})
	if !ok {
		t.Fatal("probe failed")
	}
	ccoreRequirePositive(t, "ranges on the 113-byte input", len(c))
	fmt.Printf("CCRANGEDOC route=[%s] present=[%s] c=[%s]\n", ccoreShowRanges(r), ccoreShowRanges(p), ccoreShowRanges(c))
	if !ccoreSameRangeLists(r, p) {
		t.Errorf("the route's ranges [%s] differ from the present engine's [%s]: the miss is no longer shared", ccoreShowRanges(r), ccoreShowRanges(p))
	}
	if ccoreRelation(r, c) != ccoreRelSame {
		t.Errorf("the route's ranges [%s] differ from C's [%s]", ccoreShowRanges(r), ccoreShowRanges(c))
	}
	ccoreCount("TestCcoreKnownDifferenceDiffChangedRangesMissesBytesThatCReports", "python-known-range", 1)
}
