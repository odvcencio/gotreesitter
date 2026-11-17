//go:build cgo && treesitter_c_parity

package cgoharness

// Graduation gate checks that need the pinned C runtime. scripts/graduation_gate.sh
// runs one grammar per process; GRAD_LANG names it. Census tests certify the
// grammar and select the candidate route. Other runs use the shipped route.
// Comparisons use pinned C and the present engine.
//
// A test prints one result line
//
//	GRADRESULT item=<n> status=<pass|fail|na> <numbers> first=<input or reason>
//
// and exits normally whether the item passed or not; the script decides what a
// failure does. A test fails (non-zero exit) only when it cannot run. A check
// that did not run prints status=na with the reason. The decline counters of a
// test are printed as GRADDECL so that the script can check them against
// cgo_harness/testdata/graduation_deny.txt.
//
// Items: 1 fresh equality with C, 2 invariants, 3 incremental equality with C
// (public tree and the core's records, which hold the token lookahead bytes),
// 5 reuse against C, 6 tokens against C and other counters against the present engine,
// 7 query results, 9 peak memory (child process), 10 time.

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"testing"
	"time"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/cgo_harness/internal/corecords"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type gradCtx struct {
	name  string
	entry *grammars.LangEntry
	lang  *g.Language
	cLang *sitter.Language
	blob  string // SHA-256 of the grammar blob, hex
}

func gradCensus() bool { return os.Getenv("GRAD_CENSUS") == "1" }

func gradCertify(t *testing.T) func() {
	t.Helper()
	if gradCensus() {
		return ccoreRouteCertify(t)
	}
	if _, set := os.LookupEnv("CCORE_VIEWS"); set {
		t.Fatal("CCORE_VIEWS is for --report --census; the gate measures the default views switch")
	}
	return func() {}
}

func gradRun(t *testing.T) {
	fmt.Printf("GRADRUN test=%s census=%t views=%t\n", t.Name(), gradCensus(), ccoreViewsEnabled())
}

func gradLoad(t *testing.T) *gradCtx {
	t.Helper()
	name := os.Getenv("GRAD_LANG")
	if name == "" {
		gradRun(t)
		t.Skip("set GRAD_LANG")
	}
	t.Cleanup(gradCertify(t))
	gradRun(t)
	gl, cl := ccoreLanguages(t, name)
	sum, _ := gl.GrammarBlobSHA256()
	c := &gradCtx{name: name, entry: grammars.DetectLanguageByName(name), lang: gl, cLang: cl, blob: fmt.Sprintf("%x", sum)}
	gradAssertDefaultArm(t, c)
	return c
}

// gradNeg reports whether the negative check for an item is on. GRAD_NEGATIVE=<item>
// makes one comparison of that item use a corrupted expectation; the item must then
// fail. The script's --negative option sets it.
func gradNeg(item int) bool { return os.Getenv("GRAD_NEGATIVE") == fmt.Sprint(item) }

func gradResult(item int, status, numbers, first string) {
	fmt.Printf("GRADRESULT item=%d status=%s views=%t %s first=%q\n", item, status, ccoreViewsEnabled(), numbers, first)
}

// gradReparse reparses and reports whether the core served the request. A route
// that serves a reparse as a fresh parse counts it in the fresh counter, one
// that reuses the old tree in the incremental counter; either is a served reparse.
func gradReparse(p *g.Parser, src []byte, old *g.Tree) (*g.Tree, bool, error) {
	_, _, i0 := bind.RouteCounters()
	tree, err := p.ParseIncremental(src, old)
	_, _, i1 := bind.RouteCounters()
	return tree, i1 > i0, err
}

// gradService records the route's decision for each edited reparse.
type gradService struct {
	incremental, fresh, invalid int
	reasons                     map[string]int64
	first                       string
}

func (s *gradService) begin() (map[string]int64, int64, int64) {
	_, f, i := bind.RouteCounters()
	return bind.IncrFallbacks(), f, i
}
func (s *gradService) end(id string, before map[string]int64, f0, i0 int64, oldError bool, next *g.Tree) bool {
	_, f1, i1 := bind.RouteCounters()
	after := bind.IncrFallbacks()
	if s.reasons == nil {
		s.reasons = make(map[string]int64)
	}
	var count int64
	reason := ""
	for k, v := range after {
		n := v - before[k]
		if n != 0 {
			s.reasons[k] += n
			count += n
			reason = k
		}
	}
	incr := i1-i0 == 1 && f1-f0 == 0 && count == 0
	fresh := i1-i0 == 0 && f1-f0 == 1 && count == 1
	if incr {
		s.incremental++
	}
	if fresh {
		s.fresh++
	}
	valid := incr || (fresh && ((reason == "old_has_error" && oldError) || (reason == "result_has_error" && !oldError && next != nil && next.RootNode().HasError())))
	if !valid {
		s.invalid++
		if s.first == "" {
			s.first = fmt.Sprintf("%s: fresh=%d incremental=%d reason=%s count=%d old_error=%t", id, f1-f0, i1-i0, reason, count, oldError)
		}
	}
	return incr
}
func (s *gradService) numbers() string {
	keys := make([]string, 0, len(s.reasons))
	for k := range s.reasons {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", k, s.reasons[k]))
	}
	if len(parts) == 0 {
		parts = append(parts, "none")
	}
	return fmt.Sprintf("served_incremental=%d served_fresh=%d fresh_reasons=%s invalid_service=%d", s.incremental, s.fresh, strings.Join(parts, ","), s.invalid)
}

// gradDefault selects the present engine for the reference arm: the engine of the
// build under test with the route off on the parser. It is not a pinned release of
// that engine. gradServedBy verifies that this parser does not use the core.
func gradDefault(lang *g.Language) *g.Parser {
	p := g.NewParser(lang)
	p.SetAdmissionCandidateRoute(false)
	return p
}

// gradServedBy parses src once with p and reports whether the core served that parse
// (the route's fresh counter rose). The decision comes from the counters, not from
// the environment.
func gradServedBy(p *g.Parser, src []byte) (bool, error) {
	_, f0, _ := bind.RouteCounters()
	tree, err := p.Parse(src)
	_, f1, _ := bind.RouteCounters()
	if tree != nil {
		tree.Release()
	}
	return f1 > f0, err
}

// gradAssertDefaultArm stops a test when the present-engine arm is served by the core.
// It prints GRADARM with the counters of both arms.
func gradAssertDefaultArm(t *testing.T, c *gradCtx) {
	t.Helper()
	src, _ := gradSample(t, c.name)
	rs, err := gradServedBy(gradRoute(c.lang), src)
	if err != nil {
		t.Fatalf("arm check: %v", err)
	}
	ds, err := gradServedBy(gradDefault(c.lang), src)
	if err != nil {
		t.Fatalf("arm check: %v", err)
	}
	fmt.Printf("GRADARM route_served=%v default_served=%v\n", rs, ds)
	if ds {
		t.Fatalf("the present-engine arm was served by the core; the comparison would be the core against itself")
	}
}

// gradRoute uses the candidate route in a census and the default parser otherwise.
func gradRoute(lang *g.Language) *g.Parser {
	p := g.NewParser(lang)
	if gradCensus() {
		p.SetAdmissionCandidateRoute(true)
	}
	return p
}

func gradDeclTotal(m map[string]int64) int64 {
	var n int64
	for _, v := range m {
		n += v
	}
	return n
}

// gradPrintDecl prints the decline counters since the test began.
func gradPrintDecl(item int, base map[string]int64, f0, i0 int64) {
	d, f1, i1 := bind.RouteCounters()
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		if v := d[k] - base[k]; v != 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", k, v))
		}
	}
	fmt.Printf("GRADDECL item=%d served_fresh=%d served_incr=%d declines=%s\n", item, f1-f0, i1-i0, strings.Join(parts, ","))
}

func gradCopyMap(m map[string]int64) map[string]int64 {
	out := map[string]int64{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

// gradKnownDifferences reads cgo_harness/testdata/graduation_known_differences.txt:
// one line per entry, tab separated: language, blob SHA-256, input SHA-256,
// approval reference. An entry without an approval reference fails the run.
func gradKnownDifferences(t *testing.T) map[[3]string]string {
	t.Helper()
	out := map[[3]string]string{}
	f, err := os.Open("testdata/graduation_known_differences.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 4 || strings.TrimSpace(fields[3]) == "" {
			t.Fatalf("graduation_known_differences.txt:%d: an entry needs an approval reference", n)
		}
		out[[3]string{fields[0], fields[1], fields[2]}] = fields[3]
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("item 1 known differences: %v", err)
	}
	return out
}

func gradSHA(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }

// gradSample is the committed real-code sample of the language, or its smoke sample.
func gradSample(t *testing.T, name string) ([]byte, string) {
	return ccoreRouteSource(t, name)
}

// gradInputs lists the fresh-parse inputs of a grammar: the committed sample,
// the other committed files and the end-of-input cases the equality tests use,
// and the files under GRAD_CORPUS_DIR/<language>/ (the parity corpus) when
// that folder exists.
func gradInputs(t *testing.T, name string) []ccoreInput {
	var out []ccoreInput
	seen := map[string]bool{}
	add := func(id string, src []byte) {
		h := gradSHA(src)
		if seen[h] {
			return
		}
		seen[h] = true
		out = append(out, ccoreInput{ID: id, Src: src})
	}
	switch name {
	case "json", "python", "c", "typescript":
		for _, set := range []string{name, name + "-eof"} {
			for _, in := range ccoreInputs(t, set, false) {
				add(in.ID, in.Src)
			}
		}
	default:
		src, kind := gradSample(t, name)
		add(name+"/"+kind, src)
		for _, in := range ccoreEOFInputs(name) {
			add(in.ID, in.Src)
		}
		for _, f := range ccoreExtraFiles(name) {
			b, err := os.ReadFile(filepath.Join("..", f))
			if err != nil {
				t.Fatalf("input=%s: %v", f, err)
			}
			add(name+"/file/"+filepath.ToSlash(f), b)
		}
	}
	if dir := os.Getenv("GRAD_CORPUS_DIR"); dir != "" {
		ents, err := os.ReadDir(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("corpus input=%s: %v", filepath.Join(dir, name), err)
		}
		for _, e := range ents {
			if e.IsDir() {
				continue
			}
			path := filepath.Join(dir, name, e.Name())
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("corpus input=%s: %v", path, err)
			}
			add(name+"/corpus/"+e.Name(), b)
		}
	}
	return out
}

func gradSitterEdit(e g.InputEdit) *sitter.InputEdit {
	return &sitter.InputEdit{
		StartByte: uint(e.StartByte), OldEndByte: uint(e.OldEndByte), NewEndByte: uint(e.NewEndByte),
		StartPosition:  sitter.Point{Row: uint(e.StartPoint.Row), Column: uint(e.StartPoint.Column)},
		OldEndPosition: sitter.Point{Row: uint(e.OldEndPoint.Row), Column: uint(e.OldEndPoint.Column)},
		NewEndPosition: sitter.Point{Row: uint(e.NewEndPoint.Row), Column: uint(e.NewEndPoint.Column)},
	}
}

func gradFirstDiv(tree *g.Tree, c *gradCtx, cTree *sitter.Tree) (int, string) {
	divs := compactT3StructuralDivergences(tree.RootNode(), c.lang, cTree.RootNode())
	n := len(divs)
	first := ""
	if n > 0 {
		first = compactT3FormatDivergence(divs[0])
	} else if tree.RootNode().HasError() != cTree.RootNode().HasError() {
		n, first = 1, "HasError differs"
	}
	return n, first
}

// Item 1: fresh parse of every input through the route against pinned C.
func TestGraduationFresh(t *testing.T) {
	c := gradLoad(t)
	known := gradKnownDifferences(t)
	base, f0, i0 := bind.RouteCounters()
	base = gradCopyMap(base)
	inputs := gradInputs(t, c.name)
	corpusDir := os.Getenv("GRAD_CORPUS_DIR")
	corpusFiles := 0
	if corpusDir != "" {
		entries, err := os.ReadDir(filepath.Join(corpusDir, c.name))
		if err != nil {
			t.Fatalf("item 1 corpus input=%s: %v", filepath.Join(corpusDir, c.name), err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				corpusFiles++
			}
		}
	}
	served, equal, declined, diffs, approved := 0, 0, 0, 0, 0
	first := ""
	for _, in := range inputs {
		cTree := compactT3ParseC(t, c.cLang, in.Src)
		_, fa, _ := bind.RouteCounters()
		rp := gradRoute(c.lang)
		tree, err := rp.Parse(in.Src)
		if err != nil || tree == nil {
			t.Fatalf("%s: parse: %v", in.ID, err)
		}
		_, fb, _ := bind.RouteCounters()
		if fb == fa {
			declined++
			tree.Release()
			cTree.Close()
			continue
		}
		served++
		n, why := gradFirstDiv(tree, c, cTree)
		if gradNeg(1) {
			n, why = n+1, "corrupted expectation"
		}
		switch {
		case n == 0:
			equal++
		case !gradNeg(1) && known[[3]string{c.name, c.blob, gradSHA(in.Src)}] != "":
			approved++
		default:
			diffs++
			if first == "" {
				first = fmt.Sprintf("%s (%d bytes): %d differences, first %s", in.ID, len(in.Src), n, why)
			}
		}
		tree.Release()
		cTree.Close()
	}
	status := "pass"
	switch {
	case served == 0:
		status, first = "na", "the route declined every input (see GRADDECL)"
	case diffs > 0:
		status = "fail"
	}
	corpus := "present"
	if corpusFiles == 0 {
		corpus = "none"
	}
	gradResult(1, status, fmt.Sprintf("inputs=%d served=%d equal=%d declined=%d differences=%d approved_known=%d corpus=%s corpus_files=%d", len(inputs), served, equal, declined, diffs, approved, corpus, corpusFiles), first)
	gradPrintDecl(1, base, f0, i0)
}

// gradChain is the edit session of the sample: step 0 is the sample, steps
// 1..72 are the pinned edits.
type gradStep struct {
	id   string
	src  []byte
	edit *g.InputEdit
}

func gradChain(sample []byte) []gradStep {
	chain := []gradStep{{id: "step0", src: sample}}
	for i, st := range benchfixtures.EditingSession(sample) {
		e := st.Edit
		chain = append(chain, gradStep{id: fmt.Sprintf("step%d", i+1), src: st.Source, edit: &e})
	}
	return chain
}

func gradCovered(tree *g.Tree, src []byte) bool {
	root := tree.RootNode()
	if root.EndByte() >= uint32(len(src)) {
		return true
	}
	r := tree.ParseRuntime().StopReason
	return r != "" && r != g.ParseStopAccepted
}

// gradSites builds 16 sites per edit class (insert, delete, replace), spread over the sample.
func gradSites(src []byte) []gradStep {
	var out []gradStep
	if len(src) < 64 {
		return out
	}
	for _, class := range []string{"insert", "delete", "replace"} {
		for k := 0; k < 16; k++ {
			at := (len(src)-2)*k/15 + 1
			if at >= len(src) {
				at = len(src) - 1
			}
			var next []byte
			e := g.InputEdit{StartByte: uint32(at), StartPoint: gradPoint(src, at)}
			switch class {
			case "insert":
				next = append(append(append([]byte(nil), src[:at]...), 'x'), src[at:]...)
				e.OldEndByte, e.NewEndByte = uint32(at), uint32(at+1)
			case "delete":
				next = append(append([]byte(nil), src[:at]...), src[at+1:]...)
				e.OldEndByte, e.NewEndByte = uint32(at+1), uint32(at)
			default:
				next = append([]byte(nil), src...)
				if next[at] == 'x' {
					next[at] = 'y'
				} else {
					next[at] = 'x'
				}
				e.OldEndByte, e.NewEndByte = uint32(at+1), uint32(at+1)
			}
			e.OldEndPoint, e.NewEndPoint = gradPoint(src, int(e.OldEndByte)), gradPoint(next, int(e.NewEndByte))
			out = append(out, gradStep{id: fmt.Sprintf("%s@%d", class, at), src: next, edit: &e})
		}
	}
	return out
}

func gradPoint(src []byte, at int) g.Point {
	var p g.Point
	for _, b := range src[:at] {
		if b == '\n' {
			p.Row, p.Column = p.Row+1, 0
		} else {
			p.Column++
		}
	}
	return p
}

// Item 2: the invariant gate through the route: the 72-step session and 16
// sites per edit class. Four lines per step.
func TestGraduationInvariants(t *testing.T) {
	c := gradLoad(t)
	base, f0, i0 := bind.RouteCounters()
	base = gradCopyMap(base)
	sample, _ := gradSample(t, c.name)
	p, fp := gradRoute(c.lang), gradRoute(c.lang)
	steps, okEq, okErr, okCov, okAlloc := 0, 0, 0, 0, 0
	var service gradService
	first := ""
	note := func(id, what string) {
		if first == "" {
			first = id + ": " + what
		}
	}
	check := func(id string, tree *g.Tree, next *g.Tree, src []byte) {
		steps++
		fresh, err := fp.Parse(src)
		if err != nil || fresh == nil {
			t.Fatalf("%s: fresh parse: %v", id, err)
		}
		if !gradNeg(2) && ccoreLegacyDigest(next, c.lang) == ccoreLegacyDigest(fresh, c.lang) && next.RootNode().HasError() == fresh.RootNode().HasError() {
			okEq++
		} else {
			note(id, "incremental tree differs from fresh parse")
		}
		fresh.Release()
		root := next.RootNode()
		if !(root.Type(c.lang) == "ERROR" && !root.HasError()) {
			okErr++
		} else {
			note(id, "ERROR root without HasError()")
		}
		if gradCovered(next, src) {
			okCov++
		} else {
			note(id, fmt.Sprintf("root ends at %d of %d bytes, stop reason %q", root.EndByte(), len(src), next.ParseRuntime().StopReason))
		}
		var perr error
		allocs := testing.AllocsPerRun(3, func() {
			again, e := p.ParseIncremental(src, next)
			if e != nil || again == nil {
				perr = e
				return
			}
			if again != next {
				again.Release()
			}
		})
		if perr == nil && allocs == 0 {
			okAlloc++
		} else {
			note(id, fmt.Sprintf("reparse without edits allocates %g", allocs))
		}
		_ = tree
	}
	run := func(label string, chain []gradStep) {
		var tree *g.Tree
		for _, st := range chain {
			var next *g.Tree
			var err error
			if st.edit == nil || tree == nil {
				if tree != nil {
					tree.Release()
				}
				next, err = p.Parse(st.src)
			} else {
				oldError := tree.RootNode().HasError()
				tree.Edit(*st.edit)
				before, f0, i0 := service.begin()
				next, err = p.ParseIncremental(st.src, tree)
				service.end(label+"/"+st.id, before, f0, i0, oldError, next)
			}
			if err != nil || next == nil {
				t.Fatalf("%s %s: parse: %v", label, st.id, err)
			}
			if st.edit != nil {
				check(label+"/"+st.id, tree, next, st.src)
			}
			if tree != nil && tree != next {
				tree.Release()
			}
			tree = next
		}
		if tree != nil {
			tree.Release()
		}
	}
	chain, sites := gradChain(sample), gradSites(sample)
	run("session", chain)
	// Sites: each starts from the unedited sample.
	for _, st := range sites {
		run("site", []gradStep{{id: "base", src: sample}, st})
	}
	status := "pass"
	switch {
	case len(chain) != 73 || len(sites) != 48 || steps != 120:
		status, first = "na", fmt.Sprintf("session_steps=%d sites=%d steps=%d", len(chain)-1, len(sites), steps)
	case service.invalid != 0:
		status, first = "fail", service.first
	case okEq != steps || okErr != steps || okCov != steps || okAlloc != steps:
		status = "fail"
	}
	gradResult(2, status, fmt.Sprintf("steps=%d incr_equals_fresh=%d error_root_has_error=%d root_covers_input=%d no_edit_zero_alloc=%d %s", steps, okEq, okErr, okCov, okAlloc, service.numbers()), first)
	gradPrintDecl(2, base, f0, i0)
}

// Item 3: the session through the route against C's incremental parse, and the
// core's own parse against C's records (kind, range, flags, token lookahead bytes).
//
// The record comparison follows C's own chain: a core parser that the test drives
// beside the route applies every edit and reparses incrementally, as C does, with
// the shortcut off. The route's tree for a step can have another history (a fresh
// parse on a step with an error, an old tree returned by the shortcut), and its
// records then differ from C's by design; the route's tree is compared with C's
// tree, not record by record.
type gradIncrementalCounts struct {
	steps, edits, recordSteps, equalTrees, equalRecords, invalidService, cDifferences, unapproved int
}

func gradIncrementalVerdict(c gradIncrementalCounts) (string, string) {
	switch {
	case c.invalidService != 0:
		return "fail", "invalid route service"
	case c.unapproved != 0:
		return "fail", "C incremental tree differs from fresh without approval"
	case c.steps == 0 || c.edits == 0 || c.recordSteps == 0:
		return "na", fmt.Sprintf("steps=%d edits=%d record_steps=%d", c.steps, c.edits, c.recordSteps)
	case c.equalTrees != c.steps || c.equalRecords != c.recordSteps:
		return "fail", "tree or record differs from C"
	default:
		return "pass", ""
	}
}

func TestGraduationIncrementalVerdict(t *testing.T) {
	gradRun(t)
	good := gradIncrementalCounts{steps: 2, edits: 1, recordSteps: 1, equalTrees: 2, equalRecords: 1}
	for _, tc := range []struct {
		name         string
		counts       gradIncrementalCounts
		want, reason string
	}{
		{"pass", good, "pass", ""},
		{"unapproved C difference", gradIncrementalCounts{steps: 2, edits: 1, recordSteps: 1, equalTrees: 2, equalRecords: 1, cDifferences: 1, unapproved: 1}, "fail", "C incremental tree differs from fresh without approval"},
		{"approved C differences", gradIncrementalCounts{steps: 2, edits: 1, recordSteps: 1, equalTrees: 2, equalRecords: 1, cDifferences: 2}, "pass", ""},
		{"unapproved with no records", gradIncrementalCounts{steps: 1, edits: 1, equalTrees: 1, cDifferences: 1, unapproved: 1}, "fail", "C incremental tree differs from fresh without approval"},
		{"no steps", gradIncrementalCounts{}, "na", "steps=0 edits=0 record_steps=0"},
		{"no edits", gradIncrementalCounts{steps: 1, equalTrees: 1}, "na", "steps=1 edits=0 record_steps=0"},
		{"no records", gradIncrementalCounts{steps: 1, edits: 1, equalTrees: 1}, "na", "steps=1 edits=1 record_steps=0"},
		{"invalid service", gradIncrementalCounts{invalidService: 1}, "fail", "invalid route service"},
		{"tree mismatch", gradIncrementalCounts{steps: 2, edits: 1, recordSteps: 1, equalTrees: 1, equalRecords: 1}, "fail", "tree or record differs from C"},
		{"record mismatch", gradIncrementalCounts{steps: 2, edits: 1, recordSteps: 1, equalTrees: 2}, "fail", "tree or record differs from C"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, reason := gradIncrementalVerdict(tc.counts)
			if status != tc.want || reason != tc.reason {
				t.Fatalf("got (%q, %q), want (%q, %q)", status, reason, tc.want, tc.reason)
			}
		})
	}
}

func TestGraduationIncremental(t *testing.T) {
	c := gradLoad(t)
	base, f0, i0 := bind.RouteCounters()
	base = gradCopyMap(base)
	sample, _ := gradSample(t, c.name)
	chain := gradChain(sample)
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(c.cLang); err != nil {
		t.Fatal(err)
	}
	p := gradRoute(c.lang)
	var tree *g.Tree
	var cTree *sitter.Tree
	var cStale *sitter.Tree
	steps, eqC, edits, cDiff, cUnapproved := 0, 0, 0, 0, 0
	known := gradKnownDifferences(t)
	firstUnapproved := ""
	var service gradService
	lastIncr := 0
	recordSteps, recordEqual := 0, 0
	core := ccoreNew(c.lang)
	var coreTree bindtest.Tree
	first := ""
	note := func(id, what string) {
		if first == "" {
			first = id + ": " + what
		}
	}
	for _, st := range chain {
		var next *g.Tree
		var err error
		if st.edit == nil {
			cTree = cp.Parse(st.src, nil)
			if stop := core.Parse(st.src, nil); stop != "" {
				t.Fatalf("item 3 input=%s core stop=%s", st.id, stop)
			}
			coreTree = core.Tree()
			_, freshBefore, _ := bind.RouteCounters()
			next, err = p.Parse(st.src)
			_, freshAfter, _ := bind.RouteCounters()
			if freshAfter-freshBefore != 1 {
				service.invalid++
				if service.first == "" {
					service.first = st.id + ": fresh parse was not served by the core"
				}
			}
		} else {
			cTree.Edit(gradSitterEdit(*st.edit))
			cn := cp.Parse(st.src, cTree)
			cTree.Close()
			cTree = cn
			e := st.edit
			ce := bindtest.Edit{StartByte: e.StartByte, OldEndByte: e.OldEndByte, NewEndByte: e.NewEndByte, StartRow: e.StartPoint.Row, StartCol: e.StartPoint.Column, OldEndRow: e.OldEndPoint.Row, OldEndCol: e.OldEndPoint.Column, NewEndRow: e.NewEndPoint.Row, NewEndCol: e.NewEndPoint.Column}
			edited := core.Edit(coreTree, ce)
			newCore, stop := core.ParseIncremental(st.src, edited, nil)
			core.Release(edited)
			core.Release(coreTree)
			if stop != "" || !newCore.Valid() {
				t.Fatalf("item 3 input=%s core stop=%q", st.id, stop)
			}
			coreTree = newCore
			oldError := tree.RootNode().HasError()
			tree.Edit(*st.edit)
			before, f0, i0 := service.begin()
			next, err = p.ParseIncremental(st.src, tree)
			incr := service.end(st.id, before, f0, i0, oldError, next)
			edits++
			if incr {
				ch, ccount, rc := corecords.Hash(cTree, "")
				gh, gn := core.RecordsHash("")
				recordSteps++
				if rc == 0 && ccount > 0 && gn > 0 && ch == gh && ccount == gn {
					recordEqual++
				} else {
					note(st.id, fmt.Sprintf("records route=%x/%d C=%x/%d C_status=%d", gh, gn, ch, ccount, rc))
				}
			}
		}
		if err != nil || next == nil {
			t.Fatalf("%s: parse: %v", st.id, err)
		}
		steps++
		cmp := cTree
		var freshC *sitter.Tree
		if st.edit != nil && service.incremental == lastIncr {
			freshC = cp.Parse(st.src, nil)
			incrementalDigest, err1 := COracleDeepDigest(cTree)
			freshDigest, err2 := COracleDeepDigest(freshC)
			if err1 != nil || err2 != nil {
				t.Fatalf("item 3 input=%s C tree digest: %v %v", st.id, err1, err2)
			}
			if incrementalDigest != freshDigest {
				cDiff++
				if _, ok := known[[3]string{c.name, c.blob, gradSHA(st.src)}]; !ok {
					cUnapproved++
					if firstUnapproved == "" {
						firstUnapproved = st.id
					}
				}
			}
			cmp = freshC
		}
		lastIncr = service.incremental
		if gradNeg(3) && cStale != nil {
			cmp = cStale // corrupted expectation: C's tree of the step before
		}
		if st.edit != nil && gradNeg(3) && cStale == nil {
			cStale = cp.Parse(chain[0].src, nil)
		}
		n, why := gradFirstDiv(next, c, cmp)
		if gradNeg(3) && st.edit != nil {
			n, why = n+1, "corrupted expectation"
		}
		if n == 0 {
			eqC++
		} else {
			note(st.id, fmt.Sprintf("route tree differs from C: %d differences, first %s", n, why))
		}
		if freshC != nil {
			freshC.Close()
		}
		if tree != nil && tree != next {
			tree.Release()
		}
		tree = next
	}
	if tree != nil {
		tree.Release()
	}
	cTree.Close()
	if cStale != nil {
		cStale.Close()
	}
	core.Release(coreTree)
	status, reason := gradIncrementalVerdict(gradIncrementalCounts{steps, edits, recordSteps, eqC, recordEqual, service.invalid, cDiff, cUnapproved})
	if service.invalid != 0 {
		first = service.first
	} else if status == "na" {
		first = reason
	}
	if cUnapproved != 0 {
		detail := firstUnapproved + ": C incremental tree differs from fresh without approval"
		if first == "" {
			first = detail
		} else {
			first += "; " + detail
		}
	}
	gradResult(3, status, fmt.Sprintf("steps=%d tree_equals_c=%d record_steps=%d records_equal_c=%d c_incremental_differs_from_fresh=%d c_incremental_differs_unapproved=%d %s", steps, eqC, recordSteps, recordEqual, cDiff, cUnapproved, service.numbers()), first)
	gradPrintDecl(3, base, f0, i0)
}

// Item 5: reuse per step of the session. The core-served arm is compared with C: the
// number of subtrees the route reuses at a step must equal the number of reuse_node
// events in C's parse log for the same step. The default engine's reused bytes are
// reported, not judged.
// gradReuseStepRule checks a compared step after shortcut validation.
func gradReuseStepRule(shortcut, validShortcut bool, route, c uint64) string {
	if shortcut {
		if !validShortcut {
			return "invalid shortcut"
		}
		return ""
	}
	if route == 0 {
		return "no subtree reused on a clean step"
	}
	if route != c {
		return fmt.Sprintf("route reused %d subtrees, C %d", route, c)
	}
	return ""
}

func TestGraduationReuseStepRule(t *testing.T) {
	gradRun(t)
	for _, tc := range []struct {
		name            string
		shortcut, valid bool
		route, c        uint64
		want            string
	}{
		{"equal positive", false, false, 2, 2, ""},
		{"unequal", false, false, 2, 1, "route reused 2 subtrees, C 1"},
		{"zero", false, false, 0, 0, "no subtree reused on a clean step"},
		{"valid shortcut", true, true, 0, 0, ""},
		{"invalid shortcut", true, false, 0, 0, "invalid shortcut"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := gradReuseStepRule(tc.shortcut, tc.valid, tc.route, tc.c); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGraduationReuse(t *testing.T) {
	c := gradLoad(t)
	sample, _ := gradSample(t, c.name)
	chain := gradChain(sample)
	rp, lp := gradRoute(c.lang), gradDefault(c.lang)
	var clog []string
	cp := gradCParser(t, c, &clog)
	defer cp.Close()
	var rt, lt *g.Tree
	var cTree *sitter.Tree
	steps, compared, differ, shortcutSteps, noReuse := 0, 0, 0, 0, 0
	var service gradService
	var rSum, lSum, rNodes, cNodes, shortcutNodes uint64
	first := ""
	for _, st := range chain {
		if st.edit == nil {
			var err error
			cTree = cp.Parse(st.src, nil)
			if rt, err = rp.Parse(st.src); err != nil {
				t.Fatal(err)
			}
			if lt, err = lp.Parse(st.src); err != nil {
				t.Fatal(err)
			}
			continue
		}
		clog = clog[:0]
		cTree.Edit(gradSitterEdit(*st.edit))
		cn := cp.Parse(st.src, cTree)
		cTree.Close()
		cTree = cn
		cReused := gradCount(clog, "reuse_node ")
		oldError := rt.RootNode().HasError()
		rt.Edit(*st.edit)
		lt.Edit(*st.edit)
		before, f0, i0 := service.begin()
		_, _, v0 := bind.ViewCounters()
		rn, rprof, err := rp.ParseIncrementalProfiled(st.src, rt)
		_, _, v1 := bind.ViewCounters()
		shortcutDelta := v1["shortcut_reparses"] - v0["shortcut_reparses"]
		incr := service.end(st.id, before, f0, i0, oldError, rn)
		if err != nil {
			t.Fatal(err)
		}
		ln, lprof, err := lp.ParseIncrementalProfiled(st.src, lt)
		if err != nil {
			t.Fatal(err)
		}
		steps++
		if incr || shortcutDelta != 0 {
			compared++
			rSum += rprof.ReusedBytes
			lSum += lprof.ReusedBytes
			problem := ""
			wantBytes := rprof.ReusedBytes
			if shortcutDelta != 0 {
				wantBytes = uint64(len(st.src))
			}
			if gradNeg(5) {
				wantBytes = rprof.ReusedBytes + 1
			}
			if rprof.ReusedBytes != wantBytes {
				problem = fmt.Sprintf("route reused %d bytes, want %d", rprof.ReusedBytes, wantBytes)
			}
			if shortcutDelta != 0 {
				shortcutSteps++
				shortcutNodes += rprof.ReusedSubtrees
				validShortcut := shortcutDelta == 1 && incr && ccoreViewsEnabled() && st.edit.OldEndByte-st.edit.StartByte == st.edit.NewEndByte-st.edit.StartByte
				switch {
				case gradReuseStepRule(true, validShortcut, rprof.ReusedSubtrees, uint64(cReused)) != "":
					problem = fmt.Sprintf("invalid shortcut: delta=%d incremental=%t views=%t edit=%+v", shortcutDelta, incr, ccoreViewsEnabled(), *st.edit)
				default:
					if divs := compactT3StructuralDivergences(rn.RootNode(), c.lang, cTree.RootNode()); len(divs) != 0 {
						problem = "shortcut tree differs from C: " + compactT3FormatDivergence(divs[0])
					}
				}
			} else {
				rNodes += rprof.ReusedSubtrees
				cNodes += uint64(cReused)
				if rprof.ReusedSubtrees == 0 {
					noReuse++
				}
				if stepProblem := gradReuseStepRule(false, false, rprof.ReusedSubtrees, uint64(cReused)); problem == "" {
					problem = stepProblem
				}
			}
			if problem != "" {
				differ++
				if first == "" {
					first = st.id + ": " + problem
				}
			}
		}
		if rt != rn {
			rt.Release()
		}
		if lt != ln {
			lt.Release()
		}
		rt, lt = rn, ln
	}
	cTree.Close()
	status := "pass"
	switch {
	case service.invalid != 0:
		status, first = "fail", service.first
	case compared == 0:
		status, first = "na", "no incremental step compared"
	case differ > 0:
		status = "fail"
	}
	gradResult(5, status, fmt.Sprintf("steps=%d compared=%d shortcut_steps=%d shortcut_subtrees_route=%d clean_steps_without_reuse=%d steps_differing_from_c=%d reused_subtrees_route=%d reused_subtrees_c=%d reused_bytes_route=%d reused_bytes_default_engine=%d %s", steps, compared, shortcutSteps, shortcutNodes, noReuse, differ, rNodes, cNodes, rSum, lSum, service.numbers()), first)
}

// Item 6: tokens, new nodes and maximum live versions of the core against the
// default engine, on the ledger fixture and its first pinned edit. The core
// is read through the binding (the route reports tokens and versions but not
// new nodes); new nodes are the heap records the core allocated (leaves,
// internal nodes and copies).
func TestGraduationCounters(t *testing.T) {
	c := gradLoad(t)
	src, kind := gradSample(t, c.name)
	chain := gradChain(src)
	if len(chain) < 2 || chain[1].edit == nil {
		gradResult(6, "na", "inputs=0", "no pinned edit")
		return
	}
	st := chain[1]
	var clog []string
	cp := gradCParser(t, c, &clog)
	defer cp.Close()
	ct := cp.Parse(src, nil)
	fullLex := gradCount(clog, "lexed_lookahead")
	clog = nil
	ct.Edit(gradSitterEdit(*st.edit))
	cn := cp.Parse(st.src, ct)
	editLex := gradCount(clog, "lexed_lookahead")
	ct.Close()
	defer cn.Close()
	rp := gradRoute(c.lang)
	rt, err := rp.Parse(src)
	if err != nil || rt == nil {
		t.Fatalf("item 6 input=%s route parse error=%v", kind, err)
	}
	rt.Edit(*st.edit)
	_, _, v0 := bind.ViewCounters()
	_, _, route0 := bind.RouteCounters()
	rn, err := rp.ParseIncremental(st.src, rt)
	_, _, route1 := bind.RouteCounters()
	_, _, v1 := bind.ViewCounters()
	if err != nil || rn == nil {
		t.Fatalf("item 6 input=%s route edit error=%v", st.id, err)
	}
	if rt != rn {
		rt.Release()
	}
	defer rn.Release()
	shortcutDelta := v1["shortcut_reparses"] - v0["shortcut_reparses"]
	editShortcut := shortcutDelta == 1
	runtime := rn.ParseRuntime()
	lp := gradDefault(c.lang)
	lt, err := lp.Parse(src)
	if err != nil || lt == nil {
		t.Fatalf("item 6 input=%s default parse error=%v", kind, err)
	}
	lr := lt.ParseRuntime()
	lFull := [3]uint64{lr.TokensConsumed, uint64(lr.NodesAllocated), uint64(lr.MaxStacksSeen)}
	lt.Edit(*st.edit)
	ln, err := lp.ParseIncremental(st.src, lt)
	if err != nil || ln == nil {
		t.Fatalf("item 6 input=%s default edit error=%v", st.id, err)
	}
	lr = ln.ParseRuntime()
	lEdit := [3]uint64{lr.TokensConsumed, uint64(lr.NodesAllocated), uint64(lr.MaxStacksSeen)}
	if ln != lt {
		lt.Release()
	}
	ln.Release()
	core := ccoreNew(c.lang)
	core.SetShortcut(editShortcut)
	if stop := core.Parse(src, nil); stop != "" {
		gradResult(6, "na", "input="+kind, "core stop="+stop)
		return
	}
	read := func() [3]uint64 {
		_, tok, mv, _, _ := core.Stats()
		leaves, nodes, copies := core.RecordSources()
		return [3]uint64{uint64(tok), uint64(leaves + nodes + copies), uint64(mv)}
	}
	full := read()
	old := core.Tree()
	e := st.edit
	ce := bindtest.Edit{StartByte: e.StartByte, OldEndByte: e.OldEndByte, NewEndByte: e.NewEndByte, StartRow: e.StartPoint.Row, StartCol: e.StartPoint.Column, OldEndRow: e.OldEndPoint.Row, OldEndCol: e.OldEndPoint.Column, NewEndRow: e.NewEndPoint.Row, NewEndCol: e.NewEndPoint.Column}
	edited := core.Edit(old, ce)
	nt, stop := core.ParseIncremental(st.src, edited, nil)
	core.Release(edited)
	core.Release(old)
	if stop != "" || !nt.Valid() {
		gradResult(6, "na", "input="+st.id, "core stop="+stop)
		return
	}
	inc := read()
	coreShortcut := core.ShortcutTaken()
	core.Release(nt)
	if gradNeg(6) {
		full[0] = uint64(fullLex) + 1
	}
	status, first := "pass", ""
	for _, row := range []struct {
		name         string
		core, legacy [3]uint64
		lex          int
		shortcut     bool
	}{{kind, full, lFull, fullLex, false}, {st.id, inc, lEdit, editLex, editShortcut}} {
		if row.lex <= 0 || (!row.shortcut && (row.core[0] == 0 || row.core[2] == 0)) || row.legacy[2] == 0 {
			status, first = "na", fmt.Sprintf("input=%s missing counters core=%v default=%v C_tokens=%d", row.name, row.core, row.legacy, row.lex)
			break
		}
		if (!row.shortcut && row.core[0] != uint64(row.lex)) || (row.shortcut && (row.core[0] == 0 || row.core[0] > uint64(row.lex))) || row.core[1] > row.legacy[1] || row.core[2] > row.legacy[2] {
			status, first = "fail", fmt.Sprintf("input=%s core=%v default=%v C_tokens=%d", row.name, row.core, row.legacy, row.lex)
			break
		}
	}
	if status == "pass" && (shortcutDelta < 0 || shortcutDelta > 1 || coreShortcut != editShortcut || (editShortcut && (route1-route0 != 1 || !ccoreViewsEnabled() || st.edit.OldEndByte-st.edit.StartByte != st.edit.NewEndByte-st.edit.StartByte || runtime.TokensConsumed != inc[0] || uint64(runtime.MaxStacksSeen) != inc[2]))) {
		status, first = "fail", fmt.Sprintf("input=%s invalid shortcut delta=%d core_shortcut=%t route_incremental=%d route_tokens=%d route_versions=%d core=%v", st.id, shortcutDelta, coreShortcut, route1-route0, runtime.TokensConsumed, runtime.MaxStacksSeen, inc)
	}
	if status == "pass" && editShortcut {
		if divs := compactT3StructuralDivergences(rn.RootNode(), c.lang, cn.RootNode()); len(divs) != 0 {
			status, first = "fail", fmt.Sprintf("input=%s shortcut tree differs from C: %s", st.id, compactT3FormatDivergence(divs[0]))
		}
	}
	gradResult(6, status, fmt.Sprintf("inputs=2 fresh_core=%v fresh_default=%v fresh_c_tokens=%d edit_core=%v edit_default=%v edit_c_tokens=%d edit_shortcut=%t", full, lFull, fullLex, inc, lEdit, editLex, editShortcut), first)
}

// gradCaps is the sorted capture list (name, start, end) of a query on a Go tree.
func gradCaps(t *testing.T, lang *g.Language, tree *g.Tree, q string) []string {
	query, err := g.NewQuery(q, lang)
	if err != nil {
		t.Fatalf("Go query: %v", err)
	}
	set := map[string]bool{}
	for _, m := range query.Execute(tree) {
		for _, cp := range m.Captures {
			set[fmt.Sprintf("%s:%d-%d", cp.Name, cp.Node.StartByte(), cp.Node.EndByte())] = true
		}
	}
	return gradKeys(set)
}

func gradCapsC(t *testing.T, cl *sitter.Language, tree *sitter.Tree, q string, src []byte) []string {
	cq, qerr := sitter.NewQuery(cl, q)
	if qerr != nil {
		t.Fatalf("C query: %v", qerr)
	}
	defer cq.Close()
	cur := sitter.NewQueryCursor()
	defer cur.Close()
	matches := cur.Matches(cq, tree.RootNode(), src)
	names := cq.CaptureNames()
	set := map[string]bool{}
	for {
		m := matches.Next()
		if m == nil {
			break
		}
		if !cQueryMatchSatisfiesGeneralPredicates(m, cq, src) {
			continue
		}
		for _, cp := range m.Captures {
			set[fmt.Sprintf("%s:%d-%d", names[cp.Index], cp.Node.StartByte(), cp.Node.EndByte())] = true
		}
	}
	return gradKeys(set)
}

func gradKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func gradEqual(a, b []string) bool {
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

// Item 7: highlight and tags query results on the item 1 inputs. The expected
// output is C's own query output on C's tree. Two extra patterns
// cover MISSING nodes and ERROR nodes; each supertype of the grammar gets a
// pattern too.
func TestGraduationQueries(t *testing.T) {
	c := gradLoad(t)
	queries := map[string]string{}
	if c.entry.HighlightQuery != "" {
		queries["highlight"] = c.entry.HighlightQuery
	}
	if tq := grammars.ResolveTagsQuery(*c.entry); tq != "" {
		queries["tags"] = tq
	}
	extra := "(MISSING) @missing\n(ERROR) @error\n"
	for _, s := range c.lang.SupertypeSymbols {
		if int(s) < len(c.lang.SymbolNames) {
			extra += fmt.Sprintf("(%s) @supertype\n", c.lang.SymbolNames[s])
		}
	}
	queries["missing-error-supertype"] = extra
	inputs := gradInputs(t, c.name)
	checked, diffs, viaLegacy, viaC := 0, 0, 0, 0
	first := ""
	names := make([]string, 0, len(queries))
	for k := range queries {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, in := range inputs {
		cTree := compactT3ParseC(t, c.cLang, in.Src)
		_, before, _ := bind.RouteCounters()
		rt, err := gradRoute(c.lang).Parse(in.Src)
		_, after, _ := bind.RouteCounters()
		if err != nil || rt == nil || after-before != 1 {
			t.Fatalf("item 7 input=%s served=%d error=%v", in.ID, after-before, err)
		}
		lt, err := gradDefault(c.lang).Parse(in.Src)
		if err != nil || lt == nil {
			t.Fatalf("%s: legacy: %v", in.ID, err)
		}
		for _, qn := range names {
			q := queries[qn]
			got := gradCaps(t, c.lang, rt, q)
			// C's own query engine on C's tree is the judge. The default engine's result is
			// counted when it differs from C's: its tree can equal C's in structure and still
			// lack the hidden supertype ancestors that a supertype pattern reads.
			want := gradCapsC(t, c.cLang, cTree, q, in.Src)
			viaC++
			if !gradEqual(gradCaps(t, c.lang, lt, q), want) {
				viaLegacy++
			}
			checked++
			if gradNeg(7) {
				got = append(append([]string(nil), want...), "corrupted:0-0")
			}
			if !gradEqual(got, want) {
				diffs++
				if first == "" {
					first = fmt.Sprintf("%s %s: route %d captures, expected %d; %s", in.ID, qn, len(got), len(want), gradSymDiff(got, want))
				}
			}
		}
		rt.Release()
		lt.Release()
		cTree.Close()
	}
	status := "pass"
	switch {
	case checked == 0:
		status, first = "na", fmt.Sprintf("input=%s comparisons=%d highlight=%t tags=%t", c.name, checked, queries["highlight"] != "", queries["tags"] != "")
	case diffs > 0:
		status = "fail"
	}
	omitted := []string{}
	for _, name := range []string{"highlight", "tags"} {
		if queries[name] == "" {
			omitted = append(omitted, name)
		}
	}
	if len(omitted) == 0 {
		omitted = append(omitted, "none")
	}
	gradResult(7, status, fmt.Sprintf("omitted=%s queries=%s inputs=%d comparisons=%d differences=%d default_engine_differs_from_c=%d expected_from_c=%d", strings.Join(omitted, "+"), strings.Join(names, "+"), len(inputs), checked, diffs, viaLegacy, viaC), first)
}

// gradSized builds a source of about target bytes from the sample (a JSON
// array of the sample for json; the sample repeated otherwise).
func gradSized(t *testing.T, name string, target int) []byte {
	sample, _ := gradSample(t, name)
	if name == "json" {
		var b strings.Builder
		b.WriteString("[\n")
		for b.Len() < target {
			if b.Len() > 2 {
				b.WriteString(",\n")
			}
			b.WriteString(strings.TrimSpace(string(sample)))
		}
		b.WriteString("\n]\n")
		return []byte(b.String())
	}
	var out []byte
	for len(out) < target {
		out = append(out, sample...)
		if len(out) > 0 && out[len(out)-1] != '\n' {
			out = append(out, '\n')
		}
	}
	return out
}

// gradEditAt returns the source with one space inserted at the first space
// or newline at or after at, and the edit.
func gradEditAt(src []byte, at int) ([]byte, g.InputEdit) {
	for at < len(src)-1 && src[at] != ' ' && src[at] != '\n' {
		at++
	}
	next := append(append(append([]byte(nil), src[:at]...), ' '), src[at:]...)
	p0 := gradPoint(src, at)
	return next, g.InputEdit{StartByte: uint32(at), OldEndByte: uint32(at), NewEndByte: uint32(at + 1),
		StartPoint: p0, OldEndPoint: p0, NewEndPoint: gradPoint(next, at+1)}
}

// gradProcRSS returns the peak and the current resident set size of this process in
// kB, as /proc/self/status reports them (VmHWM and VmRSS).
func gradProcRSS() (peak, cur uint64, err error) {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, 0, err
	}
	found := 0
	for _, line := range strings.Split(string(b), "\n") {
		var v *uint64
		switch {
		case strings.HasPrefix(line, "VmHWM:"):
			v = &peak
		case strings.HasPrefix(line, "VmRSS:"):
			v = &cur
		default:
			continue
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			return 0, 0, fmt.Errorf("unreadable line %q", line)
		}
		if _, err := fmt.Sscan(f[1], v); err != nil {
			return 0, 0, fmt.Errorf("unreadable line %q: %v", line, err)
		}
		found++
	}
	if found != 2 {
		return 0, 0, fmt.Errorf("/proc/self/status has no VmHWM and VmRSS lines")
	}
	return peak, cur, nil
}

// gradRSSHeadroom is how far above its size after setup the child's heap may grow
// before the collector must run.
const gradRSSHeadroom = 32 << 20

// gradResetPeakRSS makes the peak resident set size start again from the current
// size, after a collection that returns freed memory to the system. It returns the
// size the peak starts from. The setup of the child (loading the grammar and the C
// library, building the input) reaches a peak of its own that is the same for both
// arms and can be higher than the workload's; without the reset a comparison of
// two peaks compares that setup, not the parses.
//
// It also sets a soft memory limit gradRSSHeadroom above the memory the Go runtime
// holds now. The setup leaves a large live heap, and with the default pacing the
// collector would not run again until the heap had doubled: a short workload would
// then never be collected, and its peak would count every byte it allocated instead
// of the bytes it held. With the limit the collector runs as soon as the workload
// has allocated the headroom, in both arms alike.
func gradResetPeakRSS() (uint64, error) {
	runtime.GC()
	debug.FreeOSMemory()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	debug.SetMemoryLimit(int64(ms.Sys-ms.HeapReleased) + gradRSSHeadroom)
	if err := os.WriteFile("/proc/self/clear_refs", []byte("5"), 0); err != nil {
		return 0, err
	}
	peak, cur, err := gradProcRSS()
	if err != nil {
		return 0, err
	}
	// The peak may have grown a little between the reset and the read, not by much.
	if peak > cur+(16<<10) {
		return 0, fmt.Errorf("the peak was not reset: peak %d kB, current %d kB", peak, cur)
	}
	return peak, nil
}

// Item 9, child process: one run of one workload; prints the peak resident set size
// that the workload reaches (maxrss_kb), measured from a peak that is reset after
// the setup (setup_rss_kb is the size it starts from).
//
//	GRAD_RSS_ARM=route|default GRAD_RSS_WORK=fresh|edits1000
func TestGraduationRSSChild(t *testing.T) {
	c := gradLoad(t)
	arm, work := os.Getenv("GRAD_RSS_ARM"), os.Getenv("GRAD_RSS_WORK")
	if arm == "" {
		t.Skip("set GRAD_RSS_ARM")
	}
	var p *g.Parser
	if arm != "route" && arm != "default" {
		t.Fatalf("item 9 arm=%q work=%q invalid arm", arm, work)
	}
	if work != "fresh" && work != "edits1000" {
		t.Fatalf("item 9 arm=%q work=%q invalid workload", arm, work)
	}
	_, served0, incr0 := bind.RouteCounters()
	if arm == "route" {
		p = gradRoute(c.lang)
	} else {
		p = gradDefault(c.lang)
	}
	src := gradSized(t, c.name, 1<<20)
	setupRSS, err := gradResetPeakRSS()
	if err != nil {
		t.Fatalf("item 9 arm=%s work=%s cannot reset the peak resident set size: %v", arm, work, err)
	}
	tree, err := p.Parse(src)
	if err != nil || tree == nil {
		t.Fatalf("parse: %v", err)
	}
	if work == "edits1000" {
		for i := 0; i < 1000; i++ {
			next, e := gradEditAt(src, (len(src)/1001)*(i+1))
			tree.Edit(e)
			nt, err := p.ParseIncremental(next, tree)
			if err != nil || nt == nil {
				t.Fatalf("edit %d: %v", i, err)
			}
			if nt != tree {
				tree.Release()
			}
			tree, src = nt, next
			if i%25 == 24 { // progress lines survive a kill by the container's memory limit
				if peak, _, err := gradProcRSS(); err == nil {
					fmt.Printf("GRADPROG arm=%s work=%s edits=%d maxrss_kb=%d setup_rss_kb=%d\n", arm, work, i+1, peak, setupRSS)
				}
			}
		}
	}
	tree.Release()
	peakRSS, _, err := gradProcRSS()
	if err != nil {
		t.Fatalf("item 9 arm=%s work=%s cannot read the peak resident set size: %v", arm, work, err)
	}
	_, f, i := bind.RouteCounters()
	servedDelta, incrDelta := f-served0, i-incr0
	expectedIncr := int64(0)
	if work == "edits1000" {
		expectedIncr = 1000
	}
	if (arm == "route" && (servedDelta != 1 || incrDelta != expectedIncr)) || (arm == "default" && (servedDelta != 0 || incrDelta != 0)) {
		t.Fatalf("item 9 arm=%s work=%s fresh=%d incremental=%d expected_incremental=%d", arm, work, servedDelta, incrDelta, expectedIncr)
	}
	fmt.Printf("GRADRSS arm=%s work=%s maxrss_kb=%d setup_rss_kb=%d bytes=%d core_fresh_served=%d core_incremental_served=%d\n", arm, work, peakRSS, setupRSS, len(src), servedDelta, incrDelta)
}

// Item 10: 20 paired rounds; the arm that goes first alternates with the seed.
// gradTypedSite is an edit people type: one letter inserted inside an identifier, or one
// character of an identifier deleted, in code that stays valid.
type gradTypedSite struct {
	label string
	text  []byte
	edit  g.InputEdit
}

func gradIsIdent(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// gradTypedSites finds, from the middle of src on, the first place where inserting the
// letter x and the first where deleting one character leaves a text that the default
// engine parses without error. It returns none when src itself has an error.
func gradTypedSites(t *testing.T, c *gradCtx, src []byte) []gradTypedSite {
	clean := func(text []byte) bool {
		tr, err := gradDefault(c.lang).Parse(text)
		if err != nil || tr == nil {
			return false
		}
		defer tr.Release()
		root := tr.RootNode()
		return root != nil && !root.HasError() && root.EndByte() == uint32(len(text))
	}
	if !clean(src) {
		return nil
	}
	var out []gradTypedSite
	for _, kind := range []string{"typed", "deleted", "same_length"} {
		tries := 0
		for at := len(src) / 2; at < len(src)-1 && tries < 40; at++ {
			if kind == "same_length" {
				if src[at] < '0' || src[at] > '9' {
					continue
				}
			} else if !gradIsIdent(src[at-1]) || !gradIsIdent(src[at]) || !gradIsIdent(src[at+1]) {
				continue
			}
			var text []byte
			e := g.InputEdit{StartByte: uint32(at), StartPoint: gradPoint(src, at)}
			if kind == "same_length" { // one digit replaced by another: the edit the repository's benchmarks time
				text = append([]byte(nil), src...)
				text[at] = '0' + (src[at]-'0'+1)%10
				e.OldEndByte, e.NewEndByte = uint32(at+1), uint32(at+1)
			} else if kind == "typed" {
				text = append(append(append([]byte(nil), src[:at]...), 'x'), src[at:]...)
				e.OldEndByte, e.NewEndByte = uint32(at), uint32(at+1)
			} else {
				text = append(append([]byte(nil), src[:at]...), src[at+1:]...)
				e.OldEndByte, e.NewEndByte = uint32(at+1), uint32(at)
			}
			tries++
			if !clean(text) {
				continue
			}
			e.OldEndPoint, e.NewEndPoint = gradPoint(src, int(e.OldEndByte)), gradPoint(text, int(e.NewEndByte))
			out = append(out, gradTypedSite{label: kind, text: text, edit: e})
			break
		}
	}
	return out
}

func TestGraduationTime(t *testing.T) {
	c := gradLoad(t)
	rounds := 20
	status, first := "pass", ""
	var parts, newParts, newNA []string
	newStatus := "pass"
	for _, size := range []int{32 << 10, 137 << 10, 1 << 20} {
		src := gradSized(t, c.name, size)
		next, edit := gradEditAt(src, len(src)/2)
		arms := []struct {
			name string
			p    *g.Parser
		}{{"route", gradRoute(c.lang)}, {"default", gradDefault(c.lang)}}
		fresh := [2][]float64{}
		editT := [2][]float64{}
		sites := gradTypedSites(t, c, src)
		siteT := map[string]*[2][]float64{}
		for _, st := range sites {
			siteT[st.label] = &[2][]float64{}
		}
		if len(sites) != 3 {
			newNA = append(newNA, fmt.Sprintf("%dKiB: no valid typed-letter or deleted-character site", size>>10))
		}
		for _, a := range arms { // warm-up
			tr, _ := a.p.Parse(src)
			tr.Edit(edit)
			nt, _ := a.p.ParseIncremental(next, tr)
			if tr != nt {
				tr.Release()
			}
			nt.Release()
		}
		for seed := 1; seed <= rounds; seed++ {
			order := []int{0, 1}
			if rand.New(rand.NewSource(int64(seed))).Intn(2) == 1 {
				order = []int{1, 0}
			}
			for _, ai := range order {
				p := arms[ai].p
				_, beforeFresh, beforeIncr := bind.RouteCounters()
				runtime.GC()
				t0 := time.Now()
				tr, err := p.Parse(src)
				d := time.Since(t0)
				if err != nil || tr == nil {
					t.Fatal(err)
				}
				fresh[ai] = append(fresh[ai], float64(d.Microseconds())/1000)
				tr.Edit(edit)
				runtime.GC()
				t1 := time.Now()
				nt, err := p.ParseIncremental(next, tr)
				d = time.Since(t1)
				if err != nil || nt == nil {
					t.Fatal(err)
				}
				editT[ai] = append(editT[ai], float64(d.Microseconds())/1000)
				if tr != nt {
					tr.Release()
				}
				nt.Release()
				for _, st := range sites { // the edits people type
					base, err := p.Parse(src)
					if err != nil || base == nil {
						t.Fatal(err)
					}
					base.Edit(st.edit)
					runtime.GC()
					t2 := time.Now()
					nt, err := p.ParseIncremental(st.text, base)
					d := time.Since(t2)
					if err != nil || nt == nil {
						t.Fatal(err)
					}
					siteT[st.label][ai] = append(siteT[st.label][ai], float64(d.Microseconds())/1000)
					if base != nt {
						base.Release()
					}
					nt.Release()
				}
				_, afterFresh, afterIncr := bind.RouteCounters()
				wantFresh, wantIncr := int64(0), int64(0)
				if ai == 0 {
					wantFresh, wantIncr = int64(1+len(sites)), int64(1+len(sites))
				}
				if afterFresh-beforeFresh != wantFresh || afterIncr-beforeIncr != wantIncr {
					t.Fatalf("item 10 input=%s size=%d round=%d fresh=%d want=%d incremental=%d want=%d", arms[ai].name, size, seed, afterFresh-beforeFresh, wantFresh, afterIncr-beforeIncr, wantIncr)
				}
			}
		}
		if gradNeg(10) {
			for i := range fresh[0] {
				fresh[0][i] *= 3 // route times tripled
				editT[0][i] *= 3
			}
		}
		for _, w := range []struct {
			label string
			v     [2][]float64
		}{{"fresh", fresh}, {"edit", editT}} {
			mr, ml := gradMedian(w.v[0]), gradMedian(w.v[1])
			if gradNeg(10) {
				mr = ml + 1
			}
			lo0, hi0 := gradRange(w.v[0])
			lo1, hi1 := gradRange(w.v[1])
			parts = append(parts, fmt.Sprintf("%s@%dKiB route=%.2fms[%.2f-%.2f] default=%.2fms[%.2f-%.2f]", w.label, size>>10, mr, lo0, hi0, ml, lo1, hi1))
			if mr > ml {
				status = "fail"
				if first == "" {
					first = fmt.Sprintf("%s at %d KiB: route median %.2f ms, default %.2f ms", w.label, size>>10, mr, ml)
				}
			}
		}
		for _, st := range sites {
			v := siteT[st.label]
			if gradNeg(10) {
				for i := range v[0] {
					v[0][i] *= 3
				}
			}
			mr, ml := gradMedian(v[0]), gradMedian(v[1])
			lo0, hi0 := gradRange(v[0])
			lo1, hi1 := gradRange(v[1])
			newParts = append(newParts, fmt.Sprintf("%s@%dKiB route=%.2fms[%.2f-%.2f] default=%.2fms[%.2f-%.2f]", st.label, size>>10, mr, lo0, hi0, ml, lo1, hi1))
			if mr > ml {
				newStatus = "fail"
				if first == "" {
					first = fmt.Sprintf("%s at %d KiB: route median %.2f ms, default %.2f ms", st.label, size>>10, mr, ml)
				}
			}
		}
	}
	if status == "pass" && newStatus == "fail" {
		status = "fail"
	}
	if status == "pass" && len(newNA) > 0 {
		status, first = "na", "typed, deleted or same-length cells missing: "+strings.Join(newNA, "; ")
	}
	gradResult(10, status, fmt.Sprintf("rounds=%d directional=true fresh_and_whitespace_cells: %s typed_deleted_and_same_length_cells(%s): %s", rounds, strings.Join(parts, " "), newStatus, strings.Join(newParts, " ")), first)
}

func gradMedian(v []float64) float64 {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return s[len(s)/2]
}

func gradRange(v []float64) (float64, float64) {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return s[0], s[len(s)-1]
}

// gradSymDiff names the first capture that only one side has.
func gradSymDiff(got, want []string) string {
	in := func(l []string, x string) bool {
		for _, y := range l {
			if y == x {
				return true
			}
		}
		return false
	}
	for _, x := range got {
		if !in(want, x) {
			return "only in route: " + x
		}
	}
	for _, x := range want {
		if !in(got, x) {
			return "only in expected: " + x
		}
	}
	return "same set"
}
