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
	"encoding/json"
	"fmt"
	"io"
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
	"github.com/odvcencio/gotreesitter/internal/ccore/certified"
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
	gradErrReuseOn = gradErrReuse(gl)
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

// gradErrReuseOn says that the certificate under test carries the ErrReuse mode
// (in a census run, that --err-reuse certified the grammar with it). Set by gradLoad.
var gradErrReuseOn bool

// gradErrReuse reads the ErrReuse mode of the language: the certificate's own in a
// gate run, the census's GRAD_ERR_REUSE=1 in a census run.
func gradErrReuse(lang *g.Language) bool {
	if gradCensus() {
		return os.Getenv("GRAD_ERR_REUSE") == "1"
	}
	sha, ok := lang.GrammarBlobSHA256()
	return ok && certified.Blobs[sha].Modes&certified.ErrReuse != 0
}

// gradServiceValid is the rule for how the route may serve one edited reparse.
// A reparse served incrementally is always valid. A fresh parse is valid only as
// the declared fallback for an error tree: the old tree has an error
// (old_has_error), or the clean old tree gave an error result (result_has_error).
// With the ErrReuse mode there is no such fallback: an error step is served
// incrementally, as C serves it, so any fresh parse is invalid.
func gradServiceValid(errReuse, oldError, resultError, incr, fresh bool, reason string) bool {
	if incr {
		return true
	}
	if !fresh || errReuse {
		return false
	}
	return (reason == "old_has_error" && oldError) || (reason == "result_has_error" && !oldError && resultError)
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
	resultError := next != nil && next.RootNode().HasError()
	valid := gradServiceValid(gradErrReuseOn, oldError, resultError, incr, fresh, reason)
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

const (
	gradFreshTreeDifference    = "fresh_tree"
	gradCIncrementalDifference = "c_incremental_vs_fresh"
)

func gradKnownDifferences(t *testing.T) map[[4]string]string {
	t.Helper()
	const path = "testdata/graduation_known_differences.txt"
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out, err := gradReadKnownDifferences(f)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return out
}

func gradReadKnownDifferences(r io.Reader) (map[[4]string]string, error) {
	out := map[[4]string]string{}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 5 || strings.TrimSpace(fields[4]) == "" {
			return nil, fmt.Errorf("line %d: an entry needs a comparison, language, blob, witness and approval reference", n)
		}
		switch fields[0] {
		case gradFreshTreeDifference, gradCIncrementalDifference:
		default:
			return nil, fmt.Errorf("line %d: unknown comparison %q", n, fields[0])
		}
		out[[4]string{fields[0], fields[1], fields[2], fields[3]}] = fields[4]
	}
	return out, sc.Err()
}

func TestGraduationKnownDifferences(t *testing.T) {
	gradRun(t)
	const entry = "c_incremental_vs_fresh\tdart\tblob\twitness\tapproved"
	known, err := gradReadKnownDifferences(strings.NewReader(entry))
	if err != nil {
		t.Fatal(err)
	}
	if known[[4]string{gradCIncrementalDifference, "dart", "blob", "witness"}] != "approved" {
		t.Fatal("missing approved C incremental/fresh difference")
	}
	if known[[4]string{gradFreshTreeDifference, "dart", "blob", "witness"}] != "" {
		t.Fatal("C incremental/fresh approval leaked into fresh parity")
	}
	for _, invalid := range []string{"dart\tblob\twitness\tapproved", "unknown\tdart\tblob\twitness\tapproved", "fresh_tree\tdart\tblob\twitness\t"} {
		if _, err := gradReadKnownDifferences(strings.NewReader(invalid)); err == nil {
			t.Fatalf("invalid entry accepted: %q", invalid)
		}
	}
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
		case !gradNeg(1) && known[[4]string{gradFreshTreeDifference, c.name, c.blob, gradSHA(in.Src)}] != "":
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
	// With the ErrReuse mode, a step on an error tree follows the split rule: the
	// incremental tree equals C's incremental tree and the fresh tree equals C's
	// fresh tree. C itself can give two different trees at such a step.
	errReuse := gradErrReuseOn
	cpar := gradCParser(t, c, nil)
	defer cpar.Close()
	var cTree *sitter.Tree
	var stepOldErr bool
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
		errStep := errReuse && (stepOldErr || next.RootNode().HasError() || cTree.RootNode().HasError())
		eq := ccoreLegacyDigest(next, c.lang) == ccoreLegacyDigest(fresh, c.lang) && next.RootNode().HasError() == fresh.RootNode().HasError()
		if errStep {
			freshC := cpar.Parse(src, nil)
			nIncr, _ := gradFirstDiv(next, c, cTree)
			nFresh, _ := gradFirstDiv(fresh, c, freshC)
			freshC.Close()
			eq = nIncr == 0 && nFresh == 0
		}
		if !gradNeg(2) && eq {
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
				if cTree != nil {
					cTree.Close()
				}
				next, err = p.Parse(st.src)
				cTree = cpar.Parse(st.src, nil)
			} else {
				oldError := tree.RootNode().HasError()
				stepOldErr = oldError
				cTree.Edit(gradSitterEdit(*st.edit))
				cn := cpar.Parse(st.src, cTree)
				cTree.Close()
				cTree = cn
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
		if cTree != nil {
			cTree.Close()
			cTree = nil
		}
	}
	chain, sites := gradChain(sample), gradSites(sample)
	run("session", chain)
	// Sites: each starts from the unedited sample.
	for _, st := range sites {
		run("site", []gradStep{{id: "base", src: sample}, st})
	}
	oldSteps, oldEq, oldErr, oldCov, oldAlloc := steps, okEq, okErr, okCov, okAlloc
	oldService := service
	service = gradService{}
	cleanSource, cleanName := gradCleanSource(t, c, 4<<10)
	cleanChain := gradCleanEdits(t, c, cleanSource, 48, 4242)
	if cleanSource != nil {
		run("clean", append([]gradStep{{id: "base", src: cleanSource}}, cleanChain...))
	}
	status := "pass"
	switch {
	case oldService.invalid != 0:
		status, first = "fail", oldService.first
	case service.invalid != 0:
		status, first = "fail", service.first
	case okEq != steps || okErr != steps || okCov != steps || okAlloc != steps:
		status = "fail"
	case len(chain) != 73:
		status, first = "na", fmt.Sprintf("session_steps=%d", len(chain)-1)
	case service.incremental < 24:
		status, first = "na", gradCleanShortReason(cleanSource, len(cleanChain), service.incremental)
	}
	gradResult(2, status, fmt.Sprintf("steps=%d incr_equals_fresh=%d error_root_has_error=%d root_covers_input=%d no_edit_zero_alloc=%d %s clean_source=%q clean_steps=%d clean_incremental=%d", oldSteps, oldEq, oldErr, oldCov, oldAlloc, oldService.numbers(), cleanName, len(cleanChain), service.incremental), first)
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
	case c.equalTrees != c.steps || c.equalRecords != c.recordSteps:
		return "fail", "tree or record differs from C"
	case c.steps == 0 || c.edits == 0 || c.recordSteps == 0:
		return "na", fmt.Sprintf("steps=%d edits=%d record_steps=%d", c.steps, c.edits, c.recordSteps)
	default:
		return "pass", ""
	}
}

// TestGraduationServiceRule pins gradServiceValid: a failing and a passing case for
// each of the two certificate states. Without ErrReuse a fresh parse is valid as the
// declared fallback for an error tree. With ErrReuse the same fresh parse is invalid,
// and an error step must be served incrementally.
func TestGraduationServiceRule(t *testing.T) {
	gradRun(t)
	for _, tc := range []struct {
		name                                     string
		errReuse, oldErr, resultErr, incr, fresh bool
		reason                                   string
		want                                     bool
	}{
		{"no bit, clean step served incrementally", false, false, false, true, false, "", true},
		{"no bit, old error, fresh fallback", false, true, true, false, true, "old_has_error", true},
		{"no bit, result error, fresh fallback", false, false, true, false, true, "result_has_error", true},
		{"no bit, fresh parse with the wrong reason", false, true, true, false, true, "result_has_error", false},
		{"no bit, fresh parse for another reason", false, false, false, false, true, "edits_not_carried", false},
		{"bit, error step served incrementally (passing case)", true, true, true, true, false, "", true},
		{"bit, clean step to error served incrementally", true, false, true, true, false, "", true},
		{"bit, old error, fresh fallback (failing case)", true, true, true, false, true, "old_has_error", false},
		{"bit, result error, fresh fallback (failing case)", true, false, true, false, true, "result_has_error", false},
		{"bit, nothing served", true, true, true, false, false, "", false},
	} {
		if got := gradServiceValid(tc.errReuse, tc.oldErr, tc.resultErr, tc.incr, tc.fresh, tc.reason); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
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
		{"tree mismatch with no records", gradIncrementalCounts{steps: 1, edits: 1}, "fail", "tree or record differs from C"},
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
	run := func(chain []gradStep) {
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
					gh, gn := bind.CompactRecords(next, nil)
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
					witness := gradSHA([]byte(strings.Join([]string{COracleRuntimeCommit, gradSHA(st.src), incrementalDigest, freshDigest}, "\n")))
					if _, ok := known[[4]string{gradCIncrementalDifference, c.name, c.blob, witness}]; !ok {
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
		tree, cTree, cStale = nil, nil, nil
	}
	run(chain)
	oldSteps, oldEq, oldRecords, oldRecordEqual, oldCDiff, oldCUnapproved := steps, eqC, recordSteps, recordEqual, cDiff, cUnapproved
	oldService := service
	oldService.reasons = gradCopyMap(service.reasons)
	cleanSource, cleanName := gradCleanSource(t, c, 4<<10)
	var cleanChain []gradStep
	if cleanSource != nil {
		cleanChain = gradCleanEdits(t, c, cleanSource, 48, 4242)
		run(append([]gradStep{{id: "clean-base", src: cleanSource}}, cleanChain...))
	}
	cleanIncremental := service.incremental - oldService.incremental
	status, reason := gradIncrementalVerdict(gradIncrementalCounts{steps, edits, recordSteps, eqC, recordEqual, service.invalid, cDiff, cUnapproved})
	if service.invalid != 0 {
		first = service.first
	} else if status == "na" {
		first = reason
	}
	if status != "fail" && cleanIncremental < 24 {
		status, first = "na", gradCleanShortReason(cleanSource, len(cleanChain), cleanIncremental)
	}
	if cUnapproved != 0 {
		detail := firstUnapproved + ": C incremental tree differs from fresh without approval"
		if first == "" {
			first = detail
		} else {
			first += "; " + detail
		}
	}
	gradResult(3, status, fmt.Sprintf("steps=%d tree_equals_c=%d record_steps=%d records_equal_c=%d c_incremental_differs_from_fresh=%d c_incremental_differs_unapproved=%d %s clean_source=%q clean_steps=%d clean_incremental=%d", oldSteps, oldEq, oldRecords, oldRecordEqual, oldCDiff, oldCUnapproved, oldService.numbers(), cleanName, len(cleanChain), cleanIncremental), first)
	gradPrintDecl(3, base, f0, i0)
}

// Item 5: reuse per step of the session. The core-served arm is compared with C: the
// number of subtrees the route reuses at a step must equal the number of reuse_node
// events in C after the same history of edits and shortcuts. An independent C
// chain reparses every edit and checks the shortcut's public result.
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
	cp := gradCParser(t, c, nil)
	defer cp.Close()
	var mirrorLog []string
	mirror := gradCParser(t, c, &mirrorLog)
	defer mirror.Close()
	var rt, lt *g.Tree
	var cTree, mirrorTree *sitter.Tree
	steps, compared, differ, shortcutSteps, noReuse := 0, 0, 0, 0, 0
	var service gradService
	var rSum, lSum, rNodes, cNodes, shortcutNodes uint64
	first := ""
	run := func(chain []gradStep) {
		for _, st := range chain {
			if st.edit == nil {
				var err error
				cTree = cp.Parse(st.src, nil)
				if cTree == nil {
					t.Fatalf("item 5 input=%s: C parse failed", st.id)
				}
				mirrorTree = cTree.Clone()
				if rt, err = rp.Parse(st.src); err != nil {
					t.Fatal(err)
				}
				if lt, err = lp.Parse(st.src); err != nil {
					t.Fatal(err)
				}
				continue
			}
			edit := gradSitterEdit(*st.edit)
			cTree.Edit(edit)
			cn := cp.Parse(st.src, cTree)
			cTree.Close()
			cTree = cn
			if cTree == nil {
				t.Fatalf("item 5 input=%s: C incremental parse failed", st.id)
			}
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
			var cReused int
			switch {
			case incr && shortcutDelta == 0:
				mirrorLog = mirrorLog[:0]
				mirrorTree.Edit(edit)
				next := mirror.Parse(st.src, mirrorTree)
				mirrorTree.Close()
				mirrorTree = next
				cReused = gradCount(mirrorLog, "reuse_node ")
			case !incr:
				// The route reparsed from scratch. Match its input history
				// for subsequent edits while leaving the C chain above intact.
				mirrorLog = mirrorLog[:0]
				mirrorTree.Close()
				mirrorTree = mirror.Parse(st.src, nil)
				// A shortcut kept the pre-edit root. Keep the mirror tree too.
			}
			if mirrorTree == nil {
				t.Fatalf("item 5 input=%s: C parse with matching history failed", st.id)
			}
			if incr || shortcutDelta != 0 {
				compared++
				rSum += rprof.ReusedBytes
				lSum += lprof.ReusedBytes
				problem := ""
				if shortcutDelta != 0 {
					shortcutSteps++
					shortcutNodes += rprof.ReusedSubtrees
					wantBytes := uint64(len(st.src))
					if gradNeg(5) {
						wantBytes++
					}
					validShortcut := shortcutDelta == 1 && incr && ccoreViewsEnabled() &&
						st.edit.OldEndByte == st.edit.NewEndByte && st.edit.OldEndPoint == st.edit.NewEndPoint
					switch {
					case gradReuseStepRule(true, validShortcut, rprof.ReusedSubtrees, uint64(cReused)) != "":
						problem = fmt.Sprintf("invalid shortcut: delta=%d incremental=%t views=%t edit=%+v", shortcutDelta, incr, ccoreViewsEnabled(), *st.edit)
					case rprof.ReusedSubtrees != 1 || rprof.ReusedBytes != wantBytes:
						problem = fmt.Sprintf("shortcut reused %d subtrees and %d bytes, want 1 subtree and %d bytes", rprof.ReusedSubtrees, rprof.ReusedBytes, wantBytes)
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
					wantSubtrees := uint64(cReused)
					if gradNeg(5) {
						wantSubtrees = rprof.ReusedSubtrees + 1
					}
					problem = gradReuseStepRule(false, false, rprof.ReusedSubtrees, wantSubtrees)
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
		mirrorTree.Close()
		if rt != nil {
			rt.Release()
		}
		if lt != nil {
			lt.Release()
		}
		rt, lt, cTree, mirrorTree = nil, nil, nil, nil
	}
	run(chain)
	oldNumbers := fmt.Sprintf("steps=%d compared=%d shortcut_steps=%d shortcut_subtrees_route=%d clean_steps_without_reuse=%d steps_differing_from_c=%d reused_subtrees_route=%d reused_subtrees_c=%d reused_bytes_route=%d reused_bytes_default_engine=%d %s", steps, compared, shortcutSteps, shortcutNodes, noReuse, differ, rNodes, cNodes, rSum, lSum, service.numbers())
	oldDiffer, oldInvalid, oldFirst := differ, service.invalid, first
	oldServiceFirst := service.first
	steps, compared, differ, shortcutSteps, noReuse = 0, 0, 0, 0, 0
	rSum, lSum, rNodes, cNodes, shortcutNodes = 0, 0, 0, 0, 0
	service, first = gradService{}, ""
	cleanSource, cleanName := gradCleanSource(t, c, 4<<10)
	var cleanChain []gradStep
	if cleanSource != nil {
		cleanChain = gradCleanEdits(t, c, cleanSource, 48, 4242)
		run(append([]gradStep{{id: "clean-base", src: cleanSource}}, cleanChain...))
	}
	noShortcutDiffer, noShortcutReason := gradCleanNoShortcut(t, c, cleanSource, cleanChain)
	status := "pass"
	switch {
	case oldInvalid != 0:
		status, first = "fail", oldServiceFirst
	case service.invalid != 0:
		status, first = "fail", service.first
	case oldDiffer > 0:
		status, first = "fail", oldFirst
	case differ > 0:
		status = "fail"
	case len(cleanChain) < 48 || service.incremental < 24:
		status, first = "na", gradCleanShortReason(cleanSource, len(cleanChain), service.incremental)
	case noShortcutDiffer < 0:
		status, first = "fail", "shortcut-off replay: "+noShortcutReason
	case noShortcutDiffer > 0:
		status, first = "fail", fmt.Sprintf("shortcut-off replay differed from C on %d steps", noShortcutDiffer)
	}
	gradResult(5, status, fmt.Sprintf("%s clean_source=%q clean_steps=%d clean_incremental=%d clean_compared=%d clean_shortcut_steps=%d clean_steps_differing_from_c=%d clean_reused_subtrees_route=%d clean_reused_subtrees_c=%d clean_noshortcut_steps_differing_from_c=%d clean_noshortcut_reason=%q", oldNumbers, cleanName, len(cleanChain), service.incremental, compared, shortcutSteps, differ, rNodes, cNodes, noShortcutDiffer, noShortcutReason), first)
}

// Item 6: the core against pinned C on the same history. On the ledger
// fixture and its first pinned edit, the core
//
//	(a) lexes exactly the tokens C lexes (C's lexed_lookahead events);
//	(b) has a processed version width equal to C's largest version_count at a
//	    "process version" event;
//	(c) makes at most 1.10 x C's subtree creations + 16 new heap records
//	    (core: leaves, nodes and copies; C: heap leaves, new nodes without the
//	    comparison trees of ts_parser__select_children, and clones, counted by
//	    the test-only counters of the oracle build with the tag
//	    gts_subtree_counters, reset before each parse and read right after it).
//
// The default engine's tokens, new nodes and maximum live versions are
// printed, not gated. A same-length shortcut that returns the old tree keeps
// its own rule: tokens between 1 and C's, and a tree equal to C's.
// The core is read through the binding on its own parser.
const (
	gradCountersSlackNum = 110 // records <= 1.10 x C's creations ...
	gradCountersSlackDen = 100
	gradCountersSlackAdd = 16 // ... + 16 (fixed by the owner's rule)
)

// gradCounterRow is one parse's numbers: the core, pinned C and the shortcut flag.
type gradCounterRow struct {
	name         string
	shortcut     bool
	cLex         int    // C lexed_lookahead events
	cWidth       int    // C largest version_count at a process version event
	cCreated     uint64 // C subtree creations (leaves + nodes - scratch + clones)
	cCountersOn  bool   // the oracle was built with gts_subtree_counters
	coreTokens   uint64
	coreRecords  uint64 // leaves + nodes + copies
	coreWidth    uint64
	coreSelected uint64 // selectChildren comparisons (printed only)
}

// gradCountersVerdict applies the rule to one row.
func gradCountersVerdict(r gradCounterRow) (status, first string) {
	if r.cLex <= 0 {
		return "na", fmt.Sprintf("input=%s no C lexed_lookahead events", r.name)
	}
	if r.shortcut {
		if r.coreTokens == 0 || r.coreTokens > uint64(r.cLex) {
			return "fail", fmt.Sprintf("input=%s shortcut tokens core=%d C=%d", r.name, r.coreTokens, r.cLex)
		}
		return "pass", ""
	}
	if !r.cCountersOn {
		return "na", fmt.Sprintf("input=%s the oracle was built without gts_subtree_counters", r.name)
	}
	if r.cWidth <= 0 || r.cCreated == 0 {
		return "na", fmt.Sprintf("input=%s no C width or creations (width=%d creations=%d)", r.name, r.cWidth, r.cCreated)
	}
	if r.coreTokens == 0 || r.coreWidth == 0 {
		return "na", fmt.Sprintf("input=%s missing core counters tokens=%d width=%d", r.name, r.coreTokens, r.coreWidth)
	}
	if r.coreTokens != uint64(r.cLex) {
		return "fail", fmt.Sprintf("input=%s tokens core=%d C=%d", r.name, r.coreTokens, r.cLex)
	}
	if r.coreWidth != uint64(r.cWidth) {
		return "fail", fmt.Sprintf("input=%s processed width core=%d C=%d", r.name, r.coreWidth, r.cWidth)
	}
	if r.coreRecords*gradCountersSlackDen > r.cCreated*gradCountersSlackNum+gradCountersSlackAdd*gradCountersSlackDen {
		return "fail", fmt.Sprintf("input=%s new records core=%d C creations=%d bound=%d", r.name, r.coreRecords, r.cCreated, (r.cCreated*gradCountersSlackNum+gradCountersSlackAdd*gradCountersSlackDen)/gradCountersSlackDen)
	}
	return "pass", ""
}

// gradCMaxWidth is the largest version_count of the "process version" lines
// of a C parse log (as cParseVersionCount reads it).
func gradCMaxWidth(lines []string) int {
	max := 0
	for _, l := range lines {
		if !strings.HasPrefix(l, "process version:") {
			continue
		}
		for _, f := range strings.Split(l, ", ") {
			var n int
			if _, err := fmt.Sscanf(f, "version_count:%d", &n); err == nil && n > max {
				max = n
			}
		}
	}
	return max
}

// gradCounterNegative perturbs one number of a row for the negative check of
// item 6. GRAD_NEGATIVE_KIND picks it: tokens (default), width or records.
func gradCounterNegative(r *gradCounterRow) {
	switch os.Getenv("GRAD_NEGATIVE_KIND") {
	case "width":
		r.coreWidth++
	case "records":
		r.coreRecords = (r.cCreated*gradCountersSlackNum+gradCountersSlackAdd*gradCountersSlackDen)/gradCountersSlackDen + 1
	default:
		r.coreTokens = uint64(r.cLex) + 1
	}
}

// TestGraduationCountersRule checks the rule on made-up rows: each of the
// three parts alone fails a row that is otherwise equal, the records bound is
// exact, a shortcut keeps its rule, and missing data is n/a.
func TestGraduationCountersRule(t *testing.T) {
	base := gradCounterRow{name: "x", cLex: 50, cWidth: 2, cCreated: 100, cCountersOn: true, coreTokens: 50, coreRecords: 126, coreWidth: 2}
	// 1.10 x 100 + 16 = 126.
	cases := []struct {
		name   string
		edit   func(*gradCounterRow)
		status string
		reason string
	}{
		{"equal", func(r *gradCounterRow) {}, "pass", ""},
		{"records at the bound", func(r *gradCounterRow) { r.coreRecords = 126 }, "pass", ""},
		{"records over the bound only", func(r *gradCounterRow) { r.coreRecords = 127 }, "fail", "new records"},
		{"width only", func(r *gradCounterRow) { r.coreWidth = 3 }, "fail", "processed width"},
		{"width below C", func(r *gradCounterRow) { r.coreWidth = 1 }, "fail", "processed width"},
		{"tokens only", func(r *gradCounterRow) { r.coreTokens = 51 }, "fail", "tokens core"},
		{"fewer tokens", func(r *gradCounterRow) { r.coreTokens = 49 }, "fail", "tokens core"},
		{"fewer records pass", func(r *gradCounterRow) { r.coreRecords = 1 }, "pass", ""},
		{"zero creations is n/a", func(r *gradCounterRow) { r.cCreated = 0 }, "na", "no C width or creations"},
		{"no counters build is n/a", func(r *gradCounterRow) { r.cCountersOn = false }, "na", "without gts_subtree_counters"},
		{"no C width is n/a", func(r *gradCounterRow) { r.cWidth = 0 }, "na", "no C width"},
		{"core width 0 is n/a", func(r *gradCounterRow) { r.coreWidth = 0 }, "na", "missing core counters"},
		{"no C tokens is n/a", func(r *gradCounterRow) { r.cLex = 0 }, "na", "no C lexed_lookahead"},
		{"shortcut inside C's tokens", func(r *gradCounterRow) { r.shortcut, r.coreTokens, r.coreWidth, r.coreRecords = true, 3, 0, 0 }, "pass", ""},
		{"shortcut with no token", func(r *gradCounterRow) { r.shortcut, r.coreTokens = true, 0 }, "fail", "shortcut tokens"},
		{"shortcut over C's tokens", func(r *gradCounterRow) { r.shortcut, r.coreTokens = true, 51 }, "fail", "shortcut tokens"},
	}
	for _, c := range cases {
		r := base
		c.edit(&r)
		status, first := gradCountersVerdict(r)
		if status != c.status || !strings.Contains(first, c.reason) {
			t.Errorf("%s: status=%s first=%q, want %s containing %q", c.name, status, first, c.status, c.reason)
		}
	}
	for _, kind := range []string{"", "width", "records"} {
		t.Setenv("GRAD_NEGATIVE_KIND", kind)
		r := base
		gradCounterNegative(&r)
		if status, _ := gradCountersVerdict(r); status != "fail" {
			t.Errorf("negative kind %q: status=%s, want fail", kind, status)
		}
	}
	if got := gradCMaxWidth([]string{"lexed_lookahead sym:x", "process version:0, version_count:1, state:1, row:0, col:0", "process version:1, version_count:3, state:2, row:0, col:0", "process version:0, version_count:2, state:1, row:0, col:1"}); got != 3 {
		t.Errorf("gradCMaxWidth = %d, want 3", got)
	}
}

func TestGraduationCounters(t *testing.T) {
	c := gradLoad(t)
	src, kind := gradSample(t, c.name)
	chain := gradChain(src)
	if len(chain) < 2 || chain[1].edit == nil {
		gradResult(6, "na", "inputs=0", "no pinned edit")
		return
	}
	st := chain[1]
	countersOn := sitter.SubtreeCountersEnabled()
	// Pinned C on the same history: parse, edit that tree, parse the edited
	// text against it. The counters are reset right before each parse and read
	// right after it, so the edit, tree inspection and teardown are outside.
	var clog []string
	cp := gradCParser(t, c, &clog)
	defer cp.Close()
	sitter.SubtreeCountersReset()
	ct := cp.Parse(src, nil)
	cFresh := sitter.SubtreeCountersRead()
	fullLex, fullWidth := gradCount(clog, "lexed_lookahead"), gradCMaxWidth(clog)
	clog = nil
	ct.Edit(gradSitterEdit(*st.edit))
	sitter.SubtreeCountersReset()
	cn := cp.Parse(st.src, ct)
	cEdit := sitter.SubtreeCountersRead()
	editLex, editWidth := gradCount(clog, "lexed_lookahead"), gradCMaxWidth(clog)
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
	defaultFreshForest := lr.ForestFastPath
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
	type coreRead struct {
		tokens, records, width, selected uint64
		leaves, nodes, copies            int
	}
	read := func() coreRead {
		_, tok, _, _, _ := core.Stats()
		leaves, nodes, copies := core.RecordSources()
		return coreRead{uint64(tok), uint64(leaves + nodes + copies), uint64(core.ProcessedWidth()), uint64(core.SelectCandidates()), leaves, nodes, copies}
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
	rows := []gradCounterRow{
		{name: kind, cLex: fullLex, cWidth: fullWidth, cCreated: cFresh.Total(), cCountersOn: countersOn, coreTokens: full.tokens, coreRecords: full.records, coreWidth: full.width, coreSelected: full.selected},
		{name: st.id, shortcut: editShortcut, cLex: editLex, cWidth: editWidth, cCreated: cEdit.Total(), cCountersOn: countersOn, coreTokens: inc.tokens, coreRecords: inc.records, coreWidth: inc.width, coreSelected: inc.selected},
	}
	if gradNeg(6) {
		gradCounterNegative(&rows[0])
	}
	status, first := "pass", ""
	for _, row := range rows {
		s, f := gradCountersVerdict(row)
		if s == "fail" || (s == "na" && status == "pass") {
			status, first = s, f
			if s == "fail" {
				break
			}
		}
	}
	if status == "na" && rows[0].coreWidth == 0 && defaultFreshForest && lFull == [3]uint64{} {
		first += "; default engine reports no counters on this path: forest fast path"
	}
	if status == "pass" && (shortcutDelta < 0 || shortcutDelta > 1 || coreShortcut != editShortcut || (editShortcut && (route1-route0 != 1 || !ccoreViewsEnabled() || st.edit.OldEndByte-st.edit.StartByte != st.edit.NewEndByte-st.edit.StartByte || runtime.TokensConsumed != inc.tokens || uint64(runtime.MaxStacksSeen) != inc.width))) {
		status, first = "fail", fmt.Sprintf("input=%s invalid shortcut delta=%d core_shortcut=%t route_incremental=%d route_tokens=%d route_versions=%d core_tokens=%d core_width=%d", st.id, shortcutDelta, coreShortcut, route1-route0, runtime.TokensConsumed, runtime.MaxStacksSeen, inc.tokens, inc.width)
	}
	if status == "pass" && editShortcut {
		if divs := compactT3StructuralDivergences(rn.RootNode(), c.lang, cn.RootNode()); len(divs) != 0 {
			status, first = "fail", fmt.Sprintf("input=%s shortcut tree differs from C: %s", st.id, compactT3FormatDivergence(divs[0]))
		}
	}
	line := func(label string, core coreRead, cl, cw int, cc sitter.SubtreeCreations, def [3]uint64) string {
		return fmt.Sprintf("%s_core=[tokens=%d records=%d width=%d] %s_core_records=[leaves=%d nodes=%d copies=%d select_candidates=%d] %s_c=[tokens=%d creations=%d width=%d] %s_c_creations=[leaves=%d nodes=%d clones=%d scratch=%d] %s_default=[tokens=%d nodes=%d versions=%d]",
			label, core.tokens, core.records, core.width, label, core.leaves, core.nodes, core.copies, core.selected,
			label, cl, cc.Total(), cw, label, cc.Leaves, cc.Nodes, cc.Clones, cc.Scratch, label, def[0], def[1], def[2])
	}
	gradResult(6, status, fmt.Sprintf("inputs=2 %s %s edit_shortcut=%t counters=%t", line("fresh", full, fullLex, fullWidth, cFresh, lFull), line("edit", inc, editLex, editWidth, cEdit, lEdit), editShortcut, countersOn), first)
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

// gradMatchesGo preserves capture order and duplicate matches.
func gradMatchesGo(t *testing.T, lang *g.Language, tree *g.Tree, text string) []string {
	q, err := g.NewQuery(text, lang)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, m := range q.Execute(tree) {
		row := fmt.Sprintf("%d", m.PatternIndex)
		for _, cp := range m.Captures {
			row += fmt.Sprintf(" %q:%d-%d", cp.Name, cp.Node.StartByte(), cp.Node.EndByte())
		}
		out = append(out, row)
	}
	sort.Strings(out)
	return out
}

func gradMatchesC(t *testing.T, lang *sitter.Language, tree *sitter.Tree, text string, src []byte) []string {
	q, err := sitter.NewQuery(lang, text)
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	cur := sitter.NewQueryCursor()
	defer cur.Close()
	iter := cur.Matches(q, tree.RootNode(), src)
	names := q.CaptureNames()
	var out []string
	for m := iter.Next(); m != nil; m = iter.Next() {
		if !cQueryMatchSatisfiesGeneralPredicates(m, q, src) {
			continue
		}
		row := fmt.Sprintf("%d", m.PatternIndex)
		for _, cp := range m.Captures {
			row += fmt.Sprintf(" %q:%d-%d", names[cp.Index], cp.Node.StartByte(), cp.Node.EndByte())
		}
		out = append(out, row)
	}
	sort.Strings(out)
	return out
}

func gradFirstMatchDifference(got, want []string) string {
	i := 0
	for i < len(got) && i < len(want) && got[i] == want[i] {
		i++
	}
	if i < len(got) && (i == len(want) || got[i] < want[i]) {
		return "route=" + got[i]
	}
	return "expected=" + want[i]
}

// Item 7: highlight and tags query results on the item 1 inputs. The expected
// output is C's own query output on C's tree. Two extra patterns
// cover MISSING nodes and ERROR nodes; each supertype of the grammar gets a
// pattern too.
// Match differences are reported for review and do not change the verdict.
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
	matchChecked, matchDiffs, defaultMatchDiffs := 0, 0, 0
	matchFirst := ""
	first := ""
	names := make([]string, 0, len(queries))
	for k := range queries {
		names = append(names, k)
	}
	sort.Strings(names)
	loaded := make(map[string]bool)
	loadErrors, firstLoadError := 0, ""
	for _, name := range names {
		status, detail := "loaded", ""
		if _, err := g.NewQuery(queries[name], c.lang); err != nil {
			status, detail = "go_error", err.Error()
		} else if cq, err := sitter.NewQuery(c.cLang, queries[name]); err != nil {
			status, detail = "c_error", err.Error()
		} else {
			cq.Close()
			loaded[name] = true
		}
		fmt.Printf("GRADQUERY lang=%s query=%s status=%s detail=%q\n", c.name, name, status, detail)
		if status != "loaded" {
			loadErrors++
			if firstLoadError == "" {
				firstLoadError = fmt.Sprintf("query %s %s: %s", name, status, detail)
			}
		}
	}
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
			if !loaded[qn] {
				continue
			}
			q := queries[qn]
			got := gradCaps(t, c.lang, rt, q)
			// C's own query engine on C's tree is the judge. The default engine's result is
			// counted when it differs from C's: its tree can equal C's in structure and still
			// lack the hidden supertype ancestors that a supertype pattern reads.
			want := gradCapsC(t, c.cLang, cTree, q, in.Src)
			wantMatches := gradMatchesC(t, c.cLang, cTree, q, in.Src)
			gotMatches := gradMatchesGo(t, c.lang, rt, q)
			matchChecked++
			if !gradEqual(gradMatchesGo(t, c.lang, lt, q), wantMatches) {
				defaultMatchDiffs++
			}
			if !gradEqual(gotMatches, wantMatches) {
				matchDiffs++
				if matchFirst == "" {
					diff := gradFirstMatchDifference(gotMatches, wantMatches)
					row := strings.SplitN(diff, "=", 2)[1]
					pattern := strings.Fields(row)[0]
					matchFirst = fmt.Sprintf("query=%s input=%s pattern=%s first_difference=%q", qn, in.ID, pattern, diff)
				}
			}
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
	case diffs > 0:
		status = "fail"
	case loadErrors > 0:
		status, first = "na", firstLoadError
	case checked == 0:
		status, first = "na", fmt.Sprintf("input=%s comparisons=%d highlight=%t tags=%t", c.name, checked, queries["highlight"] != "", queries["tags"] != "")
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
	if matchFirst != "" {
		fmt.Printf("GRADNOTE item=7 %s\n", matchFirst)
	}
	gradResult(7, status, fmt.Sprintf("omitted=%s queries=%s load_errors=%d inputs=%d comparisons=%d differences=%d default_engine_differs_from_c=%d expected_from_c=%d match_comparisons=%d match_differences=%d default_engine_match_differences=%d", strings.Join(omitted, "+"), strings.Join(names, "+"), loadErrors, len(inputs), checked, diffs, viaLegacy, viaC, matchChecked, matchDiffs, defaultMatchDiffs), first)
}

// gradSized builds a source of about target bytes. JSON uses an array and
// Protobuf uses a single header with several message declarations.
func gradSized(t *testing.T, name string, target int) []byte {
	if name == "proto" {
		var b strings.Builder
		b.WriteString("syntax = \"proto3\";\npackage graduation;\n\n")
		for n := 0; b.Len() < target; n++ {
			fmt.Fprintf(&b, `message WorkItem%d {
  string id = 1;
  repeated int64 values = 2;
  map<string, string> labels = 3;
  oneof result {
    string text = 4;
    bytes data = 5;
  }
}

`, n)
		}
		return []byte(b.String())
	}
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
	if err := gradClearPeakRSS(); err != nil {
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

// gradRSSEdits is the file of item 9's prepared edits: the SHA-256 of the
// source they apply to and the edits in order.
type gradRSSEdits struct {
	SourceSHA256 string
	Edits        []g.InputEdit
}

// gradWriteRSSEdits writes the prepared edits of a source.
func gradWriteRSSEdits(path string, src []byte, edits []g.InputEdit) error {
	b, err := json.Marshal(gradRSSEdits{SourceSHA256: gradSHA(src), Edits: edits})
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// gradReadRSSEdits reads prepared edits and checks that they were made for src.
func gradReadRSSEdits(path string, src []byte) ([]g.InputEdit, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f gradRSSEdits
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	if f.SourceSHA256 != gradSHA(src) {
		return nil, fmt.Errorf("prepared for source %s, not %s", f.SourceSHA256, gradSHA(src))
	}
	if len(f.Edits) != 1000 {
		return nil, fmt.Errorf("%d prepared edits, want 1000", len(f.Edits))
	}
	return f.Edits, nil
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
	src, sourceName := gradCleanSource(t, c, 1<<20)
	if src == nil {
		fmt.Printf("GRADRSS arm=%s work=%s status=na source=%q bytes=0 reason=%q\n", arm, work, sourceName, "no clean source")
		return
	}
	sourceBytes := len(src)
	var prepared []g.InputEdit
	editsFile, prepare := os.Getenv("GRAD_RSS_EDITS_FILE"), os.Getenv("GRAD_RSS_PREPARE") == "1"
	if work == "edits1000" && editsFile != "" && !prepare {
		// The edits were prepared once, outside the timed runs.
		var err error
		if prepared, err = gradReadRSSEdits(editsFile, src); err != nil {
			t.Fatalf("item 9 arm=%s work=%s prepared edits: %v", arm, work, err)
		}
	} else if work == "edits1000" {
		current := src
		used := make(map[uint32]bool)
		var gen benchfixtures.CleanEditGenerator
		for step := 0; step < 1000; step++ {
			found := false
			for _, candidate := range gen.Candidates(current, "space+", 40, uint64(step)*7919) {
				proposal, err := gen.Apply(current, candidate)
				if err != nil || used[proposal.Edit.StartPoint.Row] || !gradCleanC(c, proposal.Source) {
					continue
				}
				used[proposal.Edit.StartPoint.Row] = true
				gen.Accept(candidate)
				prepared = append(prepared, proposal.Edit)
				current, found = proposal.Source, true
				break
			}
			if !found {
				break
			}
		}
		if len(prepared) != 1000 {
			fmt.Printf("GRADRSS arm=%s work=%s status=na source=%q bytes=%d reason=%q\n", arm, work, sourceName, len(src), fmt.Sprintf("only %d distinct C-clean line ends", len(prepared)))
			return
		}
		if prepare {
			if editsFile == "" {
				t.Fatalf("item 9 GRAD_RSS_PREPARE needs GRAD_RSS_EDITS_FILE")
			}
			if err := gradWriteRSSEdits(editsFile, src, prepared); err != nil {
				t.Fatalf("item 9 prepared edits: %v", err)
			}
			fmt.Printf("GRADRSSPREP status=pass source=%q bytes=%d edits=%d\n", sourceName, len(src), len(prepared))
			return
		}
	}
	runtime.GC()
	setupRSS, err := gradResetPeakRSS()
	if err != nil {
		t.Fatalf("item 9 arm=%s work=%s cannot reset the peak resident set size: %v", arm, work, err)
	}
	tree, err := p.Parse(src)
	if err != nil || tree == nil {
		t.Fatalf("parse: %v", err)
	}
	if work == "edits1000" {
		// A run killed before its 25th edit still leaves a lower bound: the
		// peak after the first parse.
		if peak, _, err := gradProcRSS(); err == nil {
			fmt.Printf("GRADPROG arm=%s work=%s edits=0 maxrss_kb=%d setup_rss_kb=%d\n", arm, work, peak, setupRSS)
		}
		for i := 0; i < 1000; i++ {
			e := prepared[i]
			at := int(e.StartByte)
			next := make([]byte, 0, len(src)+1)
			next = append(next, src[:at]...)
			next = append(next, ' ')
			next = append(next, src[at:]...)
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
	fmt.Printf("GRADRSS arm=%s work=%s status=pass source=%q maxrss_kb=%d setup_rss_kb=%d bytes=%d core_fresh_served=%d core_incremental_served=%d\n", arm, work, sourceName, peakRSS, setupRSS, sourceBytes, servedDelta, incrDelta)
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
	const rounds = 20
	status, first := "pass", ""
	var parts, unavailable []string
	for _, size := range []int{32 << 10, 137 << 10, 1 << 20} {
		src, sourceName := gradCleanSource(t, c, size)
		if src == nil {
			unavailable = append(unavailable, fmt.Sprintf("%dKiB: no clean source (%s)", size>>10, sourceName))
			parts = append(parts, fmt.Sprintf("fresh@%dKiB source=%q bytes=0 status=na", size>>10, sourceName))
			continue
		}
		var sites []gradTypedSite
		for _, kind := range []struct{ label, candidate string }{{"edit", "space+"}, {"typed", "type"}, {"deleted", "del"}, {"same_length", "same"}} {
			var gen benchfixtures.CleanEditGenerator
			candidates := gen.Candidates(src, kind.candidate, 40, 0)
			middle := len(src) / 2
			distance := func(at int) int {
				if at < middle {
					return middle - at
				}
				return at - middle
			}
			sort.SliceStable(candidates, func(i, j int) bool { return distance(candidates[i].At) < distance(candidates[j].At) })
			found := false
			for _, candidate := range candidates {
				proposal, err := gen.Apply(src, candidate)
				if err == nil && gradCleanC(c, proposal.Source) {
					sites = append(sites, gradTypedSite{label: kind.label, text: proposal.Source, edit: proposal.Edit})
					found = true
					break
				}
			}
			if !found {
				unavailable = append(unavailable, fmt.Sprintf("%s@%dKiB: no C-clean edit", kind.label, size>>10))
			}
		}
		arms := []struct {
			name string
			p    *g.Parser
		}{{"route", gradRoute(c.lang)}, {"default", gradDefault(c.lang)}}
		fresh := [2][]float64{}
		totals, edits, reparses := map[string]*[2][]float64{}, map[string]*[2][]float64{}, map[string]*[2][]float64{}
		for _, st := range sites {
			totals[st.label], edits[st.label], reparses[st.label] = &[2][]float64{}, &[2][]float64{}, &[2][]float64{}
		}
		for _, arm := range arms {
			for _, st := range sites {
				base, err := arm.p.Parse(src)
				if err != nil || base == nil {
					t.Fatalf("warm-up parse: %v", err)
				}
				base.Edit(st.edit)
				next, err := arm.p.ParseIncremental(st.text, base)
				if err != nil || next == nil {
					t.Fatalf("warm-up reparse: %v", err)
				}
				if base != next {
					base.Release()
				}
				next.Release()
			}
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
				tree, err := p.Parse(src)
				duration := time.Since(t0)
				if err != nil || tree == nil {
					t.Fatalf("fresh parse: %v", err)
				}
				fresh[ai] = append(fresh[ai], float64(duration.Nanoseconds())/1e6)
				tree.Release()
				for _, st := range sites {
					base, err := p.Parse(src)
					if err != nil || base == nil {
						t.Fatalf("edit base: %v", err)
					}
					runtime.GC()
					start := time.Now()
					base.Edit(st.edit)
					editDuration := time.Since(start)
					reparseStart := time.Now()
					next, err := p.ParseIncremental(st.text, base)
					reparseDuration := time.Since(reparseStart)
					totalDuration := time.Since(start)
					if err != nil || next == nil {
						t.Fatalf("edit reparse: %v", err)
					}
					totals[st.label][ai] = append(totals[st.label][ai], float64(totalDuration.Nanoseconds())/1e6)
					edits[st.label][ai] = append(edits[st.label][ai], float64(editDuration.Nanoseconds())/1e6)
					reparses[st.label][ai] = append(reparses[st.label][ai], float64(reparseDuration.Nanoseconds())/1e6)
					if base != next {
						base.Release()
					}
					next.Release()
				}
				_, afterFresh, afterIncr := bind.RouteCounters()
				wantFresh, wantIncr := int64(0), int64(0)
				if ai == 0 {
					wantFresh, wantIncr = int64(1+len(sites)), int64(len(sites))
				}
				if afterFresh-beforeFresh != wantFresh || afterIncr-beforeIncr != wantIncr {
					t.Fatalf("item 10 input=%s size=%d round=%d fresh=%d want=%d incremental=%d want=%d", arms[ai].name, size, seed, afterFresh-beforeFresh, wantFresh, afterIncr-beforeIncr, wantIncr)
				}
			}
		}
		compare := func(label string, values [2][]float64) {
			mr, ml := gradMedian(values[0]), gradMedian(values[1])
			if gradNeg(10) {
				mr = ml + 1
			}
			lo0, hi0 := gradRange(values[0])
			lo1, hi1 := gradRange(values[1])
			part := fmt.Sprintf("%s@%dKiB source=%q bytes=%d route=%.2fms[%.2f-%.2f] default=%.2fms[%.2f-%.2f]", label, size>>10, sourceName, len(src), mr, lo0, hi0, ml, lo1, hi1)
			if label != "fresh" {
				part += fmt.Sprintf(" route_edit=%.2fms route_reparse=%.2fms default_edit=%.2fms default_reparse=%.2fms", gradMedian(edits[label][0]), gradMedian(reparses[label][0]), gradMedian(edits[label][1]), gradMedian(reparses[label][1]))
			}
			parts = append(parts, part)
			if mr > ml {
				status = "fail"
				if first == "" {
					first = fmt.Sprintf("%s at %d KiB: route median %.2f ms, default %.2f ms", label, size>>10, mr, ml)
				}
			}
		}
		compare("fresh", fresh)
		for _, st := range sites {
			compare(st.label, *totals[st.label])
		}
	}
	if status == "pass" && len(unavailable) > 0 {
		status, first = "na", strings.Join(unavailable, "; ")
	}
	gradResult(10, status, fmt.Sprintf("rounds=%d directional=true cells: %s", rounds, strings.Join(parts, " ")), first)
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
