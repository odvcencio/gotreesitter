//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	g "github.com/odvcencio/gotreesitter"

	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
)

// Item 0 for a language whose registry entry has a TokenSourceFactory: the
// entry points that use the factory must be served by the core, not only
// Parser.Parse. Each route reports how many requests the core served and how
// many it declined, and every tree equals Parser.Parse on the same bytes.
//
// GRAD_CANDIDATE=1 (with --report --census) stands for a certificate that is not
// in the generated list yet: the test certification covers the language, and
// GRAD_CANDIDATE_PROVIDERS (comma-separated, for example table-v1) names the scan
// providers its record would list. Without a provider the routes must fail.

func gradCandidate() bool { return os.Getenv("GRAD_CANDIDATE") == "1" }

func gradCandidateProviders() []string {
	var out []string
	for _, p := range strings.Split(os.Getenv("GRAD_CANDIDATE_PROVIDERS"), ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// gradCandidateCertify installs the candidate certification and the process
// default for the candidate route, so that the parsers that LangEntry and the
// highlighter make are eligible.
func gradCandidateCertify(t *testing.T) func() {
	t.Helper()
	restore, err := bindtest.CertifyForTestsProviders(1|2|4|8, []string{"ImmediateTokens", "ZeroWidthTokens", "LayoutFallbackLexState", "AfterWhitespaceLexState"}, nil, gradCandidateProviders())
	if err != nil {
		t.Fatal(err)
	}
	prev := g.AdmissionCandidateRouteDefault()
	g.SetAdmissionCandidateRouteDefault(true)
	return func() {
		g.SetAdmissionCandidateRouteDefault(prev)
		restore()
	}
}

// gradTreeDigest is a digest of the visible tree: type, range, flags and field names.
func gradTreeDigest(tree *g.Tree, lang *g.Language) string {
	if tree == nil || tree.RootNode() == nil {
		return "nil"
	}
	h := sha256.New()
	var visit func(n *g.Node, field string)
	visit = func(n *g.Node, field string) {
		s, e := n.StartPoint(), n.EndPoint()
		fmt.Fprintf(h, "%s|%d|%d|%d.%d|%d.%d|%v|%v|%v|%s;", n.Type(lang), n.StartByte(), n.EndByte(), s.Row, s.Column, e.Row, e.Column, n.IsNamed(), n.IsMissing(), n.HasError(), field)
		for i := 0; i < n.ChildCount(); i++ {
			visit(n.Child(i), n.FieldNameForChild(i, lang))
		}
	}
	visit(tree.RootNode(), "")
	return fmt.Sprintf("%x", h.Sum(nil))
}

// gradRouteTotals is what one route did over all its requests.
type gradRouteTotals struct {
	name              string
	requests          int
	served, declined  int64
	firstFailure      string
	declineReasonSums map[string]int64
}

func (r *gradRouteTotals) numbers() string {
	return fmt.Sprintf("route_%s=requests:%d,served:%d,declined:%d", r.name, r.requests, r.served, r.declined)
}

// gradMeasure runs one request of a route and records what the core did with it.
func gradMeasure(r *gradRouteTotals, id string, want string, lang *g.Language, run func() (*g.Tree, error)) {
	d0, f0, i0 := bind.RouteCounters()
	tree, err := run()
	d1, f1, i1 := bind.RouteCounters()
	served := (f1 - f0) + (i1 - i0)
	declined := gradDeclTotal(d1) - gradDeclTotal(d0)
	r.requests++
	r.served += served
	r.declined += declined
	if r.declineReasonSums == nil {
		r.declineReasonSums = map[string]int64{}
	}
	for k, v := range d1 {
		r.declineReasonSums[k] += v - d0[k]
	}
	fail := ""
	switch {
	case tree == nil:
		fail = fmt.Sprintf("no tree (error %v)", err)
	case served < 1:
		fail = fmt.Sprintf("not served by the core (declined %d)", declined)
	case gradTreeDigest(tree, lang) != want:
		fail = "tree differs from Parser.Parse"
	}
	if tree != nil {
		tree.Release()
	}
	if fail != "" && r.firstFailure == "" {
		r.firstFailure = fmt.Sprintf("route=%s input=%s: %s", r.name, id, fail)
	}
}

// gradFactoryRoutes runs every route that uses the registered factory over the
// item 1 inputs and over the first step of the edit chain of the sample. It
// returns the totals per route, in a fixed order.
func gradFactoryRoutes(t *testing.T, entry *grammars.LangEntry, lang *g.Language, inputs []ccoreInput, sample []byte) []*gradRouteTotals {
	t.Helper()
	ctx := context.Background()
	names := []string{"LangEntry.Parse", "LangEntry.ParseStrict", "LangEntry.ParseContext", "Highlighter.fresh", "Highlighter.incremental", "ParseIncrementalWithTokenSource"}
	totals := map[string]*gradRouteTotals{}
	var ordered []*gradRouteTotals
	for _, n := range names {
		totals[n] = &gradRouteTotals{name: n}
		ordered = append(ordered, totals[n])
	}
	want := func(src []byte) string {
		tree, err := g.NewParser(lang).Parse(src)
		if err != nil || tree == nil {
			t.Fatalf("Parser.Parse: %v", err)
		}
		defer tree.Release()
		return gradTreeDigest(tree, lang)
	}
	hl, err := entry.NewHighlighter()
	if err != nil {
		t.Fatalf("NewHighlighter: %v", err)
	}
	for _, in := range inputs {
		if len(in.Src) == 0 {
			continue // the highlighter parses no empty source
		}
		w := want(in.Src)
		gradMeasure(totals["LangEntry.Parse"], in.ID, w, lang, func() (*g.Tree, error) { return entry.Parse(in.Src) })
		gradMeasure(totals["LangEntry.ParseStrict"], in.ID, w, lang, func() (*g.Tree, error) { return entry.ParseStrict(ctx, in.Src) })
		gradMeasure(totals["LangEntry.ParseContext"], in.ID, w, lang, func() (*g.Tree, error) { return entry.ParseContext(ctx, in.Src) })
		gradMeasure(totals["Highlighter.fresh"], in.ID, w, lang, func() (*g.Tree, error) {
			_, tree := hl.HighlightIncremental(in.Src, nil)
			return tree, nil
		})
	}
	// The first step of the edit chain.
	chain := gradChain(sample)
	if len(chain) < 2 || chain[1].edit == nil {
		t.Fatalf("no edit chain for %d bytes", len(sample))
	}
	step := chain[1]
	w := want(step.src)
	id := "chain/step1"
	h := hl
	_, hOld := h.HighlightIncremental(chain[0].src, nil)
	hOld.Edit(*step.edit)
	gradMeasure(totals["Highlighter.incremental"], id, w, lang, func() (*g.Tree, error) {
		_, tree := h.HighlightIncremental(step.src, hOld)
		return tree, nil
	})
	hOld.Release()
	p := g.NewParser(lang)
	old, err := p.Parse(chain[0].src)
	if err != nil || old == nil {
		t.Fatalf("old tree: %v", err)
	}
	old.Edit(*step.edit)
	gradMeasure(totals["ParseIncrementalWithTokenSource"], id, w, lang, func() (*g.Tree, error) {
		return p.ParseIncrementalWithTokenSource(step.src, old, entry.TokenSourceFactory(step.src, lang))
	})
	old.Release()
	return ordered
}

// gradRoutesVerdict joins the totals into the numbers of the result line and the
// first failure, if any, which names its route.
func gradRoutesVerdict(routes []*gradRouteTotals) (numbers, first string) {
	var parts []string
	reasons := map[string]int64{}
	for _, r := range routes {
		parts = append(parts, r.numbers())
		if first == "" && r.firstFailure != "" {
			first = r.firstFailure
		}
		if first == "" && r.requests == 0 {
			first = fmt.Sprintf("route=%s: no request ran", r.name)
		}
		for k, v := range r.declineReasonSums {
			reasons[k] += v
		}
	}
	if len(reasons) > 0 {
		var ks []string
		for k, v := range reasons {
			if v != 0 {
				ks = append(ks, fmt.Sprintf("%s:%d", k, v))
			}
		}
		sort.Strings(ks)
		parts = append(parts, "decline_reasons="+strings.Join(ks, ","))
	}
	return strings.Join(parts, " "), first
}

// TestGraduationRoutesVerdict checks that a route that served nothing fails and is named.
func TestGraduationRoutesVerdict(t *testing.T) {
	ok := &gradRouteTotals{name: "A", requests: 2, served: 2}
	bad := &gradRouteTotals{name: "B", requests: 1, declined: 1, firstFailure: "route=B input=x: not served by the core (declined 1)"}
	if _, first := gradRoutesVerdict([]*gradRouteTotals{ok}); first != "" {
		t.Fatalf("a served route failed: %q", first)
	}
	if n, first := gradRoutesVerdict([]*gradRouteTotals{ok, bad}); !strings.Contains(first, "route=B") || !strings.Contains(n, "route_B=requests:1,served:0,declined:1") {
		t.Fatalf("numbers %q first %q", n, first)
	}
	if _, first := gradRoutesVerdict([]*gradRouteTotals{{name: "C"}}); !strings.Contains(first, "route=C") {
		t.Fatalf("an empty route passed: %q", first)
	}
}
