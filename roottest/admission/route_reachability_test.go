package admission_test

// Route reachability: which engine serves each public entry point for each
// certified grammar, read from the route counters and from the tree that came
// back, not inferred from the code.
//
// For every grammar whose blob has a certificate (internal/ccore/certified), the
// test runs each entry point named in routeEntryPoints on a committed sample and
// reads three things around the call:
//
//   - the core's route counters (bind.RouteCounters, bind.IncrFallbacks): the
//     fresh parses and reparses it served, the reasons it declined, and the
//     reasons a reparse became a fresh core parse;
//   - the compact scheduler's counter (gts.AdmissionCandidateCounters);
//   - whether the returned tree has a core tree behind it (bind.CompactRecords
//     reports rows only for such a tree).
//
// The counters and the tree must agree. A cell that has no tree (a highlight
// call that returns ranges) uses the counters alone. The result is a table, kept
// as a golden file so that a later change shows which cells moved. Run
//
//	go test ./roottest/admission -run TestRouteReachabilityTable -update-route-reachability
//
// to rewrite the golden after an intended change, and read the diff.
//
// The test fails when: the instrumentation reads nothing (the controls), a
// certificate has no loadable grammar, an entry point lists no measured grammar,
// the tree and the counters disagree, a public parse method of Parser,
// LangEntry or Highlighter is neither measured nor waived with a reason, or a
// waiver names a method that no longer exists.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"unicode/utf16"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/ccore/certified"
)

var updateRouteReachability = flag.Bool("update-route-reachability", false, "rewrite testdata/route_reachability.golden")

const routeReachabilityGolden = "testdata/route_reachability.golden"

// routeEntryPoints lists the measured entry points in table order.
var routeEntryPoints = []string{
	"parser_parse",
	"parser_parse_strict",
	"parser_parse_utf16",
	"parser_parse_token_source",
	"parser_parse_token_source_factory",
	"parser_parse_forest_experimental",
	"parser_incremental_clean_edit",
	"parser_incremental_with_old_tree_option",
	"parser_incremental_token_source",
	"parser_incremental_edit_to_error",
	"parser_incremental_edit_from_error",
	"parser_included_ranges_parse",
	"parser_included_ranges_incremental",
	"lang_entry_parse",
	"lang_entry_parse_context",
	"highlighter_highlight",
	"highlighter_incremental",
}

// routeWaivers lists the public parse methods that the table does not measure,
// by receiver and method, each with the reason. A waived method is a gap in the
// table; the reasons that say "not measured yet" are the backlog.
var routeWaivers = map[string]string{
	"Parser.ParseNoTreeBenchmarkOnly":                         "benchmark-only entry (the name says so)",
	"Parser.ParseNoTreeWithExternalCheckpointsBenchmarkOnly":  "benchmark-only entry (the name says so)",
	"Parser.ParseNoResultCompatibilityBenchmarkOnly":          "benchmark-only entry (the name says so)",
	"Parser.ParseWorkLimits":                                  "getter, not a parse",
	"Parser.ParseUTF16Bytes":                                  "decodes the bytes and calls the ParseUTF16 path (parser_api.go:2068-2075)",
	"Parser.ParseUTF16WithTokenSourceFactory":                 "encodes to UTF-8 and calls the factory path (parser_api.go:2087-2094)",
	"Parser.ParseUTF16BytesWithTokenSourceFactory":            "decodes and calls the UTF-16 factory path (parser_api.go:2107-2115)",
	"Parser.ParseWithTokenSourceStrict":                       "strict error wrapper over the token-source path",
	"Parser.ParseWithTokenSourceFactoryStrict":                "strict error wrapper over the factory path",
	"Parser.ParseIncrementalStrict":                           "strict error wrapper over parseIncremental (parser_api.go:2430-2434)",
	"Parser.ParseIncrementalUTF16":                            "not measured yet: needs UTF-16 edits; encodes and calls parseIncremental (parser_api.go:2445-2452)",
	"Parser.ParseIncrementalUTF16Bytes":                       "not measured yet: decodes and calls the UTF-16 incremental path (parser_api.go:2465-2472)",
	"Parser.ParseIncrementalUTF16WithTokenSourceFactory":      "not measured yet: UTF-16 incremental with a factory",
	"Parser.ParseIncrementalUTF16BytesWithTokenSourceFactory": "not measured yet: UTF-16 byte incremental with a factory",
	"Parser.ParseIncrementalWithTokenSourceStrict":            "strict error wrapper over the token-source incremental path",
	"Parser.ParseIncrementalWithTokenSourceFactory":           "not measured yet: factory variant of ParseIncrementalWithTokenSource",
	"Parser.ParseIncrementalWithTokenSourceFactoryStrict":     "strict error wrapper over the factory incremental path",
	"Parser.ParseIncrementalProfiled":                         "not measured yet: profiling variant of ParseIncremental",
	"Parser.ParseIncrementalWithTokenSourceProfiled":          "not measured yet: profiling variant of the token-source incremental path",
	"Parser.ParseWithStrict":                                  "strict error wrapper over ParseWith",
	"LangEntry.ParseStrict":                                   "strict error wrapper over the entry parse",
	"Highlighter.HighlightIncrementalStrict":                  "strict error wrapper over HighlightIncremental",
	"Highlighter.HighlightIncrementalUTF16":                   "not measured yet: UTF-16 highlight reparse",
	"Highlighter.HighlightIncrementalUTF16Bytes":              "not measured yet: UTF-16 byte highlight reparse",
	"Highlighter.HighlightUTF16":                              "not measured yet: UTF-16 highlight",
	"Highlighter.HighlightTreeUTF16":                          "takes a finished tree; it parses nothing",
	"Highlighter.HighlightUTF16Bytes":                         "not measured yet: UTF-16 byte highlight",
}

// routeMeasured maps each measured public method to the entry point that
// measures it. ParseForestExperimental and the highlighter calls are measured
// through their own cells.
var routeMeasured = map[string]string{
	"Parser.Parse":                           "parser_parse",
	"Parser.ParseStrict":                     "parser_parse_strict",
	"Parser.ParseUTF16":                      "parser_parse_utf16",
	"Parser.ParseWithTokenSource":            "parser_parse_token_source",
	"Parser.ParseWithTokenSourceFactory":     "parser_parse_token_source_factory",
	"Parser.ParseForestExperimental":         "parser_parse_forest_experimental",
	"Parser.ParseIncremental":                "parser_incremental_clean_edit",
	"Parser.ParseWith":                       "parser_incremental_with_old_tree_option",
	"Parser.ParseIncrementalWithTokenSource": "parser_incremental_token_source",
	"Parser.SetIncludedRanges":               "parser_included_ranges_parse",
	"LangEntry.Parse":                        "lang_entry_parse",
	"LangEntry.ParseContext":                 "lang_entry_parse_context",
	"Highlighter.Highlight":                  "highlighter_highlight",
	"Highlighter.HighlightIncremental":       "highlighter_incremental",
}

// ---- reading the route counters ----

type routeReading struct {
	fresh, incr int64
	routed      uint64 // the compact route's served count
	forestAcc   uint64
	declines    map[string]int64
	fallbacks   map[string]int64
}

func readRoute() routeReading {
	declines, fresh, incr := bind.RouteCounters()
	routed, _ := gts.AdmissionCandidateCounters()
	acc, _, _ := bind.ForestCounters()
	r := routeReading{fresh: fresh, incr: incr, routed: routed, forestAcc: acc,
		declines: map[string]int64{}, fallbacks: map[string]int64{}}
	for k, v := range declines {
		r.declines[k] = v
	}
	for k, v := range bind.IncrFallbacks() {
		r.fallbacks[k] = v
	}
	return r
}

type routeDelta struct {
	fresh, incr int64
	routed      int64
	forestAcc   int64
	declines    []string
	fallbacks   []string
}

func (after routeReading) since(before routeReading) routeDelta {
	d := routeDelta{
		fresh: after.fresh - before.fresh, incr: after.incr - before.incr,
		routed: int64(after.routed) - int64(before.routed), forestAcc: int64(after.forestAcc) - int64(before.forestAcc),
	}
	for k, v := range after.declines {
		if v > before.declines[k] {
			d.declines = append(d.declines, k)
		}
	}
	for k, v := range after.fallbacks {
		if v > before.fallbacks[k] {
			d.fallbacks = append(d.fallbacks, k)
		}
	}
	sort.Strings(d.declines)
	sort.Strings(d.fallbacks)
	return d
}

// classifyRoute turns one call's counter delta, and the tree it returned, into a
// table cell. haveTree is false for a call that returns no tree; treeCore says
// whether the tree has a core tree behind it. It returns an error when the tree
// and the counters disagree: then one of the two readings is wrong and the table
// would be a guess.
func classifyRoute(d routeDelta, haveTree, treeCore bool) (string, error) {
	coreCounted := d.fresh+d.incr+d.forestAcc > 0
	if haveTree && treeCore != coreCounted {
		return "", fmt.Errorf("tree has core link %v but the core counters say served=%v (fresh %+d, incr %+d, forest accepted %+d, declines %v)",
			treeCore, coreCounted, d.fresh, d.incr, d.forestAcc, d.declines)
	}
	if coreCounted {
		var kinds []string
		if d.fresh > 0 {
			kinds = append(kinds, "fresh")
		}
		if d.incr > 0 {
			kinds = append(kinds, "incr")
		}
		if d.forestAcc > 0 {
			kinds = append(kinds, "forest")
		}
		out := "core:" + strings.Join(kinds, "+")
		if len(d.fallbacks) > 0 {
			out += "<" + strings.Join(d.fallbacks, "+")
		}
		return out, nil
	}
	if d.routed > 0 {
		return "compact:" + orDash(d.declines), nil
	}
	return "legacy:" + orDash(d.declines), nil
}

func orDash(s []string) string {
	if len(s) == 0 {
		return "-"
	}
	return strings.Join(s, "+")
}

// TestRouteClassifier pins the passing and the failing cases of the reading
// itself (AGENTS.md section 9, rule 5).
func TestRouteClassifier(t *testing.T) {
	cases := []struct {
		name      string
		d         routeDelta
		haveTree  bool
		treeCore  bool
		want      string
		wantError bool
	}{
		{"core fresh", routeDelta{fresh: 1}, true, true, "core:fresh", false},
		{"core incr", routeDelta{incr: 1}, true, true, "core:incr", false},
		{"core fresh after an incremental fallback", routeDelta{fresh: 1, fallbacks: []string{"result_has_error"}}, true, true, "core:fresh<result_has_error", false},
		{"legacy with a decline", routeDelta{declines: []string{"included_ranges"}}, true, false, "legacy:included_ranges", false},
		{"legacy not attempted", routeDelta{}, true, false, "legacy:-", false},
		{"compact", routeDelta{routed: 1}, true, false, "compact:-", false},
		{"no tree, counters only", routeDelta{fresh: 2}, false, false, "core:fresh", false},
		{"tree is core but counters did not move", routeDelta{}, true, true, "", true},
		{"counters moved but the tree is not core", routeDelta{fresh: 1}, true, false, "", true},
	}
	for _, tc := range cases {
		got, err := classifyRoute(tc.d, tc.haveTree, tc.treeCore)
		if (err != nil) != tc.wantError {
			t.Errorf("%s: error = %v, want error %v", tc.name, err, tc.wantError)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// ---- the grammars and their inputs ----

type routeGrammar struct {
	name  string
	entry grammars.LangEntry
	lang  *gts.Language
}

// certifiedRouteGrammars returns the grammar that carries each certificate, in
// name order. A certificate whose blob no registered grammar loads is a failure:
// the table would silently shrink.
func certifiedRouteGrammars(t *testing.T) []routeGrammar {
	t.Helper()
	if len(certified.Blobs) == 0 {
		t.Fatal("certified.Blobs is empty: the table would measure nothing")
	}
	byBlob := map[[32]byte]routeGrammar{}
	for _, e := range grammars.AllLanguages() {
		if e.Language == nil {
			continue
		}
		lang := e.Language()
		if lang == nil {
			continue
		}
		sha, ok := lang.GrammarBlobSHA256()
		if !ok {
			continue
		}
		if _, ok := certified.Blobs[sha]; !ok {
			continue
		}
		if prev, dup := byBlob[sha]; dup && prev.name <= e.Name {
			continue
		}
		byBlob[sha] = routeGrammar{name: e.Name, entry: e, lang: lang}
	}
	for sha := range certified.Blobs {
		if _, ok := byBlob[sha]; !ok {
			t.Errorf("certificate %x has no registered grammar that loads it", sha[:8])
		}
	}
	out := make([]routeGrammar, 0, len(byBlob))
	for _, g := range byBlob {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

type routeFixtureRow struct {
	Language, Role, SHA256 string
	Bytes                  int
	Path                   string `json:"committed_path"`
}

// routeBases returns the candidate base sources of each language: the R4 sample
// first, then the medians, then the largest file within a size cap. The order
// is part of the table, so it is fixed.
func routeBases(t *testing.T) map[string][][]byte {
	t.Helper()
	raw, err := os.ReadFile("../../internal/benchfixtures/real_corpus.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct{ Entries []routeFixtureRow }
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	order := []string{"sample", "median_1", "median_2", "median_3", "largest"}
	rank := map[string]int{}
	for i, r := range order {
		rank[r] = i
	}
	rows := map[string][]routeFixtureRow{}
	for _, row := range manifest.Entries {
		if _, ok := rank[row.Role]; ok && row.Path != "" && row.Bytes <= 64<<10 {
			rows[row.Language] = append(rows[row.Language], row)
		}
	}
	out := map[string][][]byte{}
	for lang, rs := range rows {
		sort.SliceStable(rs, func(i, j int) bool { return rank[rs[i].Role] < rank[rs[j].Role] })
		for _, row := range rs {
			src, err := os.ReadFile(filepath.Join("../../internal/benchfixtures", row.Path))
			if err != nil {
				t.Fatalf("fixture %s: %v", row.Path, err)
			}
			sum := sha256.Sum256(src)
			if len(src) != row.Bytes || hex.EncodeToString(sum[:]) != row.SHA256 {
				t.Fatalf("fixture %s changed identity", row.Path)
			}
			out[lang] = append(out[lang], src)
		}
	}
	return out
}

func routePointAt(src []byte, off int) gts.Point {
	var p gts.Point
	for i := 0; i < off && i < len(src); i++ {
		if src[i] == '\n' {
			p.Row++
			p.Column = 0
		} else {
			p.Column++
		}
	}
	return p
}

func routeReplace(src []byte, at, oldEnd int, text string) ([]byte, gts.InputEdit) {
	out := append(append(append([]byte(nil), src[:at]...), text...), src[oldEnd:]...)
	newEnd := at + len(text)
	return out, gts.InputEdit{
		StartByte: uint32(at), OldEndByte: uint32(oldEnd), NewEndByte: uint32(newEnd),
		StartPoint: routePointAt(src, at), OldEndPoint: routePointAt(src, oldEnd), NewEndPoint: routePointAt(out, newEnd),
	}
}

// routeParseClean reports whether a fresh parse of src has no error. It runs outside any measured window.
func routeParseClean(lang *gts.Language, src []byte) bool {
	tree, err := gts.NewParser(lang).Parse(src)
	if err != nil || tree == nil || tree.RootNode() == nil {
		return false
	}
	defer tree.Release()
	root := tree.RootNode()
	return !root.HasError()
}

func routeParseErrors(lang *gts.Language, src []byte) bool {
	tree, err := gts.NewParser(lang).Parse(src)
	if err != nil || tree == nil || tree.RootNode() == nil {
		return false
	}
	defer tree.Release()
	return tree.RootNode().HasError()
}

// routeSession is the inputs one grammar's cells use.
type routeSession struct {
	base       []byte
	cleanNew   []byte // base with a clean edit
	cleanEdit  gts.InputEdit
	cleanSkip  string
	errNew     []byte // base with an edit that leaves a syntax error
	errEdit    gts.InputEdit
	repairEdit gts.InputEdit // from errNew back to base
	errSkip    string
	ranges     []gts.Range
	rangesNew  []gts.Range // the same ranges after rangesEdit
	rangesSrc  []byte
	rangesEdit gts.InputEdit
	rangesNote string
}

func newlineOffsets(src []byte) []int {
	var out []int
	for i, b := range src {
		if b == '\n' {
			out = append(out, i)
		}
	}
	return out
}

// pickOffsets returns up to n offsets spread evenly over list.
func pickOffsets(list []int, n int) []int {
	if len(list) <= n {
		return list
	}
	out := make([]int, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, list[i*len(list)/n])
	}
	return out
}

func buildRouteSession(g routeGrammar, candidates [][]byte) (routeSession, string) {
	var s routeSession
	for _, c := range candidates {
		if routeParseClean(g.lang, c) {
			s.base = c
			break
		}
	}
	clean := s.base != nil
	if !clean {
		// Every candidate parses with an error: the plain parses still run on
		// the first one, and the edit sessions are skipped with that reason.
		s.base = candidates[0]
	}
	nl := newlineOffsets(s.base)
	positions := append(pickOffsets(nl, 12), len(s.base))

	// A clean edit: a space or a newline that leaves the tree free of errors.
	s.cleanSkip = "no_clean_edit"
	if !clean {
		s.cleanSkip = "base_has_error"
	}
	for _, pos := range positions {
		if !clean {
			break
		}
		for _, text := range []string{" ", "\n"} {
			next, edit := routeReplace(s.base, pos, pos, text)
			if routeParseClean(g.lang, next) {
				s.cleanNew, s.cleanEdit, s.cleanSkip = next, edit, ""
				break
			}
		}
		if s.cleanSkip == "" {
			break
		}
	}

	// An edit that leaves a syntax error.
	s.errSkip = "no_error_edit"
	if !clean {
		s.errSkip = "base_has_error"
	}
	junk := []string{"@@", ")", "}", "]", "\"", "<<", "`", "\\", "=", ",", "(", "{"}
search:
	for _, pos := range positions {
		if !clean {
			break
		}
		for _, j := range junk {
			next, edit := routeReplace(s.base, pos, pos, j)
			if routeParseErrors(g.lang, next) {
				s.errNew, s.errEdit = next, edit
				_, s.repairEdit = routeReplace(next, pos, pos+len(j), "")
				s.errSkip = ""
				break search
			}
		}
	}

	// Two included ranges that both start at a positive offset, the shape of an
	// injection (cgo_harness/included_ranges_go_root_parity_cgo_test.go).
	s.rangesNote = "source_too_short"
	n := len(s.base)
	align := func(off int) int {
		for i := off; i < n; i++ {
			if s.base[i] == '\n' {
				return i + 1
			}
		}
		return n
	}
	a0, a1, b0, b1 := align(n/4), align(n/2), align(5*n/8), align(7*n/8)
	if a0 > 0 && a0 < a1 && a1 <= b0 && b0 < b1 && b1 <= n {
		mk := func(src []byte, s0, e0 int) gts.Range {
			return gts.Range{StartByte: uint32(s0), EndByte: uint32(e0), StartPoint: routePointAt(src, s0), EndPoint: routePointAt(src, e0)}
		}
		s.ranges = []gts.Range{mk(s.base, a0, a1), mk(s.base, b0, b1)}
		// The edit puts a space at the end of the first range, so the second
		// range moves by one byte.
		next, edit := routeReplace(s.base, a1-1, a1-1, " ")
		s.rangesSrc, s.rangesEdit = next, edit
		s.rangesNew = []gts.Range{mk(next, a0, a1+1), mk(next, b0+1, b1+1)}
		s.rangesNote = ""
	}
	return s, ""
}

// ---- the cells ----

type routeCell struct {
	result string // engine:detail, or skip:<reason>
}

type routeStubSource struct{}

func (routeStubSource) Next() gts.Token { return gts.Token{} }

func routeTokenSource(g routeGrammar, src []byte) (gts.TokenSource, string) {
	if f := g.entry.TokenSourceFactory; f != nil {
		return f(src, g.lang), "registered"
	}
	return routeStubSource{}, "stub"
}

func routeFactory(g routeGrammar) gts.TokenSourceFactory {
	if f := g.entry.TokenSourceFactory; f != nil {
		return func(src []byte) (gts.TokenSource, error) { return f(src, g.lang), nil }
	}
	return func([]byte) (gts.TokenSource, error) { return routeStubSource{}, nil }
}

func routeTreeCore(tree *gts.Tree) bool {
	_, rows := bind.CompactRecords(tree, io.Discard)
	return rows > 0
}

// measure runs call between two counter readings and classifies it.
func measure(prepare func() func() (*gts.Tree, bool)) (string, error) {
	call := prepare() // setup (an old tree and its edit) runs outside the window
	if call == nil {
		return "skip:setup_failed", nil
	}
	before := readRoute()
	tree, haveTree := call()
	after := readRoute()
	core := false
	if tree != nil {
		core = routeTreeCore(tree)
		defer tree.Release()
	}
	return classifyRoute(after.since(before), haveTree && tree != nil, core)
}

func routeCells(t *testing.T, g routeGrammar, s routeSession) map[string]string {
	cells := map[string]string{}
	var setPrep func(name string, prepare func() func() (*gts.Tree, bool))
	set := func(name string, call func() (*gts.Tree, bool)) {
		setPrep(name, func() func() (*gts.Tree, bool) { return call })
	}
	setPrep = func(name string, prepare func() func() (*gts.Tree, bool)) {
		res, err := measure(prepare)
		if err != nil {
			t.Errorf("%s %s: %v", g.name, name, err)
			res = "error"
		}
		cells[name] = res
	}
	skip := func(name, why string) { cells[name] = "skip:" + why }
	// oldTree parses src and applies edit; it runs outside the measured window.
	oldTree := func(p *gts.Parser, src []byte, edit gts.InputEdit) *gts.Tree {
		tree, err := p.Parse(src)
		if err != nil || tree == nil {
			return nil
		}
		tree.Edit(edit)
		return tree
	}
	utf16Units := func(src []byte) []uint16 { return utf16.Encode([]rune(string(src))) }

	base := s.base

	set("parser_parse", func() (*gts.Tree, bool) { tree, _ := gts.NewParser(g.lang).Parse(base); return tree, true })
	set("parser_parse_strict", func() (*gts.Tree, bool) { tree, _ := gts.NewParser(g.lang).ParseStrict(base); return tree, true })
	set("parser_parse_utf16", func() (*gts.Tree, bool) {
		tree, _ := gts.NewParser(g.lang).ParseUTF16(utf16Units(base))
		return tree, true
	})
	set("parser_parse_token_source", func() (*gts.Tree, bool) {
		ts, _ := routeTokenSource(g, base)
		tree, _ := gts.NewParser(g.lang).ParseWithTokenSource(base, ts)
		return tree, true
	})
	set("parser_parse_token_source_factory", func() (*gts.Tree, bool) {
		tree, _ := gts.NewParser(g.lang).ParseWithTokenSourceFactory(base, routeFactory(g))
		return tree, true
	})
	set("parser_parse_forest_experimental", func() (*gts.Tree, bool) {
		tree, ok := gts.NewParser(g.lang).ParseForestExperimental(base)
		if !ok {
			if tree != nil {
				tree.Release()
			}
			return nil, false
		}
		return tree, true
	})

	// Edit sessions.
	if s.cleanSkip != "" {
		for _, name := range []string{"parser_incremental_clean_edit", "parser_incremental_with_old_tree_option", "parser_incremental_token_source", "highlighter_incremental"} {
			skip(name, s.cleanSkip)
		}
	} else {
		setPrep("parser_incremental_clean_edit", func() func() (*gts.Tree, bool) {
			p := gts.NewParser(g.lang)
			old := oldTree(p, base, s.cleanEdit)
			if old == nil {
				return nil
			}
			return func() (*gts.Tree, bool) { return incrementalAfter(p, s.cleanNew, old), true }
		})
		setPrep("parser_incremental_with_old_tree_option", func() func() (*gts.Tree, bool) {
			p := gts.NewParser(g.lang)
			old := oldTree(p, base, s.cleanEdit)
			if old == nil {
				return nil
			}
			return func() (*gts.Tree, bool) {
				res, _ := p.ParseWith(s.cleanNew, gts.WithOldTree(old))
				old.Release()
				return res.Tree, true
			}
		})
		setPrep("parser_incremental_token_source", func() func() (*gts.Tree, bool) {
			p := gts.NewParser(g.lang)
			old := oldTree(p, base, s.cleanEdit)
			if old == nil {
				return nil
			}
			return func() (*gts.Tree, bool) {
				ts, _ := routeTokenSource(g, s.cleanNew)
				tree, _ := p.ParseIncrementalWithTokenSource(s.cleanNew, old, ts)
				old.Release()
				return tree, true
			}
		})
	}
	if s.errSkip != "" {
		for _, name := range []string{"parser_incremental_edit_to_error", "parser_incremental_edit_from_error"} {
			skip(name, s.errSkip)
		}
	} else {
		setPrep("parser_incremental_edit_to_error", func() func() (*gts.Tree, bool) {
			p := gts.NewParser(g.lang)
			old := oldTree(p, base, s.errEdit)
			if old == nil {
				return nil
			}
			return func() (*gts.Tree, bool) { return incrementalAfter(p, s.errNew, old), true }
		})
		setPrep("parser_incremental_edit_from_error", func() func() (*gts.Tree, bool) {
			p := gts.NewParser(g.lang)
			old := oldTree(p, s.errNew, s.repairEdit)
			if old == nil {
				return nil
			}
			return func() (*gts.Tree, bool) { return incrementalAfter(p, base, old), true }
		})
	}

	// Included ranges.
	if s.rangesNote != "" {
		skip("parser_included_ranges_parse", s.rangesNote)
		skip("parser_included_ranges_incremental", s.rangesNote)
	} else {
		set("parser_included_ranges_parse", func() (*gts.Tree, bool) {
			p := gts.NewParser(g.lang)
			p.SetIncludedRanges(s.ranges)
			tree, _ := p.Parse(base)
			return tree, true
		})
		setPrep("parser_included_ranges_incremental", func() func() (*gts.Tree, bool) {
			p := gts.NewParser(g.lang)
			p.SetIncludedRanges(s.ranges)
			old, _ := p.Parse(base)
			if old == nil {
				return nil
			}
			old.Edit(s.rangesEdit)
			p.SetIncludedRanges(s.rangesNew)
			return func() (*gts.Tree, bool) {
				tree, _ := p.ParseIncremental(s.rangesSrc, old)
				old.Release()
				return tree, true
			}
		})
	}

	// The entry and the highlighter.
	e := g.entry
	set("lang_entry_parse", func() (*gts.Tree, bool) { tree, _ := e.Parse(base); return tree, true })
	set("lang_entry_parse_context", func() (*gts.Tree, bool) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		tree, _ := e.ParseContext(ctx, base)
		return tree, true
	})
	h, herr := e.NewHighlighter()
	if herr != nil || h == nil {
		why := "no_highlighter"
		skip("highlighter_highlight", why)
		skip("highlighter_incremental", why)
	} else {
		set("highlighter_highlight", func() (*gts.Tree, bool) { h.Highlight(base); return nil, false })
		if s.cleanSkip == "" {
			cells["highlighter_incremental"] = highlighterIncrementalCell(t, g, h, s)
		}
	}
	return cells
}

// incrementalAfter parses next incrementally against old inside a measured
// window that the caller opened before the call. It releases old.
func incrementalAfter(p *gts.Parser, next []byte, old *gts.Tree) *gts.Tree {
	if old == nil {
		return nil
	}
	tree, _ := p.ParseIncremental(next, old)
	old.Release()
	return tree
}

func highlighterIncrementalCell(t *testing.T, g routeGrammar, h *gts.Highlighter, s routeSession) string {
	_, old := h.HighlightIncremental(s.base, nil)
	if old == nil {
		return "skip:no_tree"
	}
	old.Edit(s.cleanEdit)
	before := readRoute()
	_, tree := h.HighlightIncremental(s.cleanNew, old)
	after := readRoute()
	core := false
	if tree != nil {
		core = routeTreeCore(tree)
		tree.Release()
	}
	old.Release()
	// The highlighter may also parse injected text, so only the counters speak
	// for the call; the tree is the document's.
	res, err := classifyRoute(after.since(before), false, core)
	if err != nil {
		t.Errorf("%s highlighter_incremental: %v", g.name, err)
		return "error"
	}
	return res
}

// ---- the table ----

func routeTable(t *testing.T) (string, map[string]int) {
	t.Helper()
	gs := certifiedRouteGrammars(t)
	bases := routeBases(t)
	var rows strings.Builder
	measuredBy := map[string]int{}
	for _, g := range gs {
		cands := bases[g.name]
		if len(cands) == 0 {
			t.Errorf("%s: no committed sample", g.name)
			continue
		}
		s, why := buildRouteSession(g, cands)
		_ = why
		cells := routeCells(t, g, s)
		for _, name := range routeEntryPoints {
			res, ok := cells[name]
			if !ok {
				t.Errorf("%s %s: no cell: the entry point stopped being measured", g.name, name)
				continue
			}
			if !strings.HasPrefix(res, "skip:") {
				measuredBy[name]++
			}
			fmt.Fprintf(&rows, "%s\t%s\t%s\n", g.name, name, res)
		}
	}
	return rows.String(), measuredBy
}

func routeSummary(rows string) string {
	type tally map[string]int
	byEntry := map[string]tally{}
	for _, line := range strings.Split(strings.TrimSpace(rows), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 3 {
			continue
		}
		kind := f[2]
		if i := strings.Index(kind, ":"); i >= 0 {
			kind = kind[:i]
		}
		if byEntry[f[1]] == nil {
			byEntry[f[1]] = tally{}
		}
		byEntry[f[1]][kind]++
	}
	var b strings.Builder
	b.WriteString("# entry_point\tcore\tcompact\tlegacy\tskip\terror\n")
	for _, name := range routeEntryPoints {
		c := byEntry[name]
		fmt.Fprintf(&b, "# %s\t%d\t%d\t%d\t%d\t%d\n", name, c["core"], c["compact"], c["legacy"], c["skip"], c["error"])
	}
	return b.String()
}

// TestRouteControls fails when the instrumentation reads nothing: a certified
// grammar's Parse must read as core, and a parse with included ranges must read
// as a decline, not as an unmoved counter.
func TestRouteControls(t *testing.T) {
	gs := certifiedRouteGrammars(t)
	if len(gs) == 0 {
		t.Fatal("no certified grammar")
	}
	bases := routeBases(t)
	for _, g := range gs {
		if g.name != "json" {
			continue
		}
		s, why := buildRouteSession(g, bases[g.name])
		if why != "" {
			t.Fatal(why)
		}
		cells := routeCells(t, g, s)
		if got := cells["parser_parse"]; got != "core:fresh" {
			t.Errorf("control parser_parse = %q, want core:fresh: the counters read nothing", got)
		}
		if got := cells["parser_included_ranges_parse"]; !strings.HasPrefix(got, "legacy:") && !strings.HasPrefix(got, "core:") {
			t.Errorf("control parser_included_ranges_parse = %q", got)
		}
		return
	}
	t.Fatal("json is not among the certified grammars; the control measured nothing")
}

func TestRouteReachabilityTable(t *testing.T) {
	rows, measuredBy := routeTable(t)
	for _, name := range routeEntryPoints {
		if measuredBy[name] == 0 {
			t.Errorf("entry point %s measured no grammar", name)
		}
	}
	got := "# Route reachability of every certified grammar, read from route counters.\n" +
		"# Cells: engine:detail. core:fresh|incr|forest, a '<reason' suffix names the reparse fallback;\n" +
		"# legacy:<decline reasons>, '-' when no decline counter moved (the core was not asked).\n" +
		"# Grammars without a registered token source use an end-of-input stub: the route is chosen before a token is read.\n" +
		routeSummary(rows) + rows
	if *updateRouteReachability {
		if err := os.WriteFile(routeReachabilityGolden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(routeReachabilityGolden)
	if err != nil {
		t.Fatalf("%v (run with -update-route-reachability)", err)
	}
	if !bytes.Equal(want, []byte(got)) {
		wl, gl := strings.Split(string(want), "\n"), strings.Split(got, "\n")
		shown := 0
		for i := 0; i < len(wl) || i < len(gl); i++ {
			var w, g string
			if i < len(wl) {
				w = wl[i]
			}
			if i < len(gl) {
				g = gl[i]
			}
			if w != g && shown < 40 {
				t.Errorf("golden line %d:\n  want %q\n  got  %q", i+1, w, g)
				shown++
			}
		}
		t.Fatal("route table differs from the golden; if the move is intended, rerun with -update-route-reachability and read the diff")
	}
}

// TestRouteEntryPointCoverage fails when a public parse method is neither
// measured nor waived, and when a waiver or measurement names a method that is
// gone.
func TestRouteEntryPointCoverage(t *testing.T) {
	known := map[string]bool{}
	check := func(recv string, typ reflect.Type, prefixes ...string) {
		for i := 0; i < typ.NumMethod(); i++ {
			name := typ.Method(i).Name
			match := false
			for _, p := range prefixes {
				if strings.HasPrefix(name, p) {
					match = true
				}
			}
			if !match {
				continue
			}
			key := recv + "." + name
			known[key] = true
			_, measured := routeMeasured[key]
			_, waived := routeWaivers[key]
			if !measured && !waived {
				t.Errorf("%s is neither measured nor waived: add a cell or a waiver with a reason", key)
			}
			if measured && waived {
				t.Errorf("%s is both measured and waived", key)
			}
		}
	}
	check("Parser", reflect.TypeOf(&gts.Parser{}), "Parse", "SetIncludedRanges")
	check("LangEntry", reflect.TypeOf(&grammars.LangEntry{}), "Parse")
	check("Highlighter", reflect.TypeOf(&gts.Highlighter{}), "Highlight")
	for key := range routeMeasured {
		if !known[key] {
			t.Errorf("measured method %s does not exist", key)
		}
	}
	for key := range routeWaivers {
		if !known[key] {
			t.Errorf("waiver for %s: the method does not exist", key)
		}
	}
	listed := map[string]bool{}
	for _, n := range routeEntryPoints {
		listed[n] = true
	}
	for key, entry := range routeMeasured {
		if !listed[entry] {
			t.Errorf("%s maps to %s, which is not in routeEntryPoints", key, entry)
		}
	}
}
