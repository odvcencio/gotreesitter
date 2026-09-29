//go:build cgo && treesitter_c_parity

// Command gts_mismatch compares gotreesitter with the locked C tree-sitter
// oracle on one input, replays the grammar-receipt edit session, and shrinks
// disagreeing inputs to small reproducers.
//
// Modes:
//
//	incshow  Print Go's incremental and fresh trees and C's tree for the edit
//	         given by -before and -after.
//	show     Print the Go tree (-route, default "default") and the C tree of
//	         -in as S-expressions.
//	check    Parse -in with Go (default route and compact route) and with C.
//	         Print one JSON object with digests, the first divergence, the
//	         first differing leaf, and a triage class.
//	min      Shrink -in while Go on -route still differs from C with the same
//	         first-divergence signature and the same C error state. Write the
//	         smallest input to -out.
//	session  Replay the receipt edit session (72 single-character edits) on
//	         -in. Print one JSON line per step. With -dump, write the texts of
//	         the first steps where Go incremental differs from Go fresh, and
//	         of the first step where Go fresh differs from C fresh.
//	incmin   Shrink an edit given by -before and -after (one contiguous change)
//	         while Go's incremental tree still differs from Go's fresh tree.
//	chainmin Shrink the history-dependent edit chain ending at -step while the
//	         final incremental tree still differs from a fresh Go parse.
//	triage   Read one grammar receipt (-receipt) and the pinned corpus
//	         (-corpus). For every failing fresh-parity file and for the first
//	         failing session steps, check, shrink, and classify. Print JSON
//	         lines.
//
// Run it in the cgo harness image, for example:
//
//	cd cgo_harness && go run -tags treesitter_c_parity ./cmd/gts_mismatch \
//	  -mode min -grammar bash -in case.sh -out case.min.sh
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	gotreesitter "github.com/odvcencio/gotreesitter"
	cgoharness "github.com/odvcencio/gotreesitter/cgo_harness"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// The receipt edit session constants. They must match
// cmd/gts_grammar_receipt so a replayed step has the same text.
const (
	sessionStepsPerEditClass = 24
	sessionSitesPerEditClass = 16
)

type config struct {
	mode       string
	grammar    string
	in         string
	out        string
	before     string
	after      string
	route      string
	receipt    string
	corpus     string
	dumpDir    string
	maxTests   int
	timeout    time.Duration
	parseLimit time.Duration
	maxDumps   int
	step       int
	noPolicies bool
	reason     string
}

func main() {
	var cfg config
	flag.StringVar(&cfg.mode, "mode", "check", "check|show|incshow|min|session|incmin|chainmin|triage|receipt-check|minroute")
	flag.StringVar(&cfg.grammar, "grammar", "", "grammar name, for example bash")
	flag.StringVar(&cfg.in, "in", "", "input file")
	flag.StringVar(&cfg.out, "out", "", "output file for min and incmin, or PREFIX for chainmin artifacts")
	flag.StringVar(&cfg.before, "before", "", "incmin: text before the edit")
	flag.StringVar(&cfg.after, "after", "", "incmin: text after the edit")
	flag.StringVar(&cfg.route, "route", "auto", "Go route for min, session, incmin, and chainmin: default|compact|auto (auto picks default when it mismatches, else compact)")
	flag.StringVar(&cfg.receipt, "receipt", "", "triage: grammar receipt JSON")
	flag.StringVar(&cfg.corpus, "corpus", "", "triage: pinned corpus root (the directory that holds <grammar>/...)")
	flag.StringVar(&cfg.dumpDir, "dump", "", "session and triage: directory for step texts and shrunk inputs")
	flag.IntVar(&cfg.step, "step", 0, "chainmin: 1-based failing session step")
	flag.IntVar(&cfg.maxTests, "max-tests", 3000, "min, incmin, and chainmin: maximum predicate evaluations")
	flag.DurationVar(&cfg.timeout, "timeout", 3*time.Minute, "min, incmin, and chainmin: wall-clock budget per shrink")
	flag.DurationVar(&cfg.parseLimit, "parse-timeout", 5*time.Second, "Go parse timeout; a timed-out candidate is not interesting")
	flag.IntVar(&cfg.maxDumps, "max-dumps", 2, "session: invariant-failing steps to write")
	flag.StringVar(&cfg.reason, "reason", "did not accept EOF", "minroute: substring of the compact-route decline reason to keep")
	flag.BoolVar(&cfg.noPolicies, "no-conflict-policies", false, "clear the grammar's conflict policies before parsing (experiment: compare with C's plain GLR choice)")
	flag.Parse()

	if err := run(cfg, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "gts_mismatch:", err)
		os.Exit(1)
	}
}

func run(cfg config, stdout io.Writer) error {
	if (cfg.mode == "triage" || cfg.mode == "receipt-check") && cfg.grammar == "" && cfg.receipt != "" {
		name, err := receiptGrammarName(cfg.receipt)
		if err != nil {
			return err
		}
		cfg.grammar = name
	}
	if strings.TrimSpace(cfg.grammar) == "" {
		return errors.New("-grammar is required")
	}
	gotreesitter.SetGLRForestEnabled(true)
	h, err := newHarness(cfg.grammar, cfg.parseLimit)
	if err != nil {
		return err
	}
	defer h.close()
	if cfg.noPolicies {
		h.lang.ConflictPolicies = nil
	}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)

	switch cfg.mode {
	case "check":
		src, err := os.ReadFile(cfg.in)
		if err != nil {
			return err
		}
		return enc.Encode(h.check(src))
	case "incshow":
		before, err := os.ReadFile(cfg.before)
		if err != nil {
			return err
		}
		after, err := os.ReadFile(cfg.after)
		if err != nil {
			return err
		}
		return h.incShow(stdout, before, after, cfg.route)
	case "show":
		src, err := os.ReadFile(cfg.in)
		if err != nil {
			return err
		}
		return h.show(stdout, src, cfg.route)
	case "min":
		src, err := os.ReadFile(cfg.in)
		if err != nil {
			return err
		}
		res, err := h.minimizeFresh(src, cfg.route, cfg.maxTests, cfg.timeout)
		if err != nil {
			return err
		}
		if cfg.out != "" {
			if err := os.WriteFile(cfg.out, res.Input, 0o644); err != nil {
				return err
			}
		}
		return enc.Encode(res)
	case "session":
		src, err := os.ReadFile(cfg.in)
		if err != nil {
			return err
		}
		route := cfg.route
		if route == "auto" {
			route = h.receiptRoute()
		}
		steps := h.replaySession(src, route, cfg.dumpDir, cfg.maxDumps)
		for _, step := range steps {
			if err := enc.Encode(step); err != nil {
				return err
			}
		}
		return nil
	case "incmin":
		before, err := os.ReadFile(cfg.before)
		if err != nil {
			return err
		}
		after, err := os.ReadFile(cfg.after)
		if err != nil {
			return err
		}
		route := cfg.route
		if route == "auto" {
			route = h.receiptRoute()
		}
		res := h.minimizeIncremental(before, after, route, cfg.maxTests, cfg.timeout)
		if cfg.out != "" && res.Reproduced {
			if err := os.WriteFile(cfg.out+".before", res.Before, 0o644); err != nil {
				return err
			}
			if err := os.WriteFile(cfg.out+".after", res.After, 0o644); err != nil {
				return err
			}
		}
		return enc.Encode(res)
	case "chainmin":
		if cfg.step < 1 {
			return errors.New("chainmin requires -step N, where N is a 1-based session step")
		}
		src, err := os.ReadFile(cfg.in)
		if err != nil {
			return err
		}
		route := cfg.route
		if route == "auto" {
			route = h.receiptRoute()
		}
		res, err := h.minimizeChain(src, cfg.step, route, cfg.maxTests, cfg.timeout)
		if err != nil {
			return err
		}
		if cfg.out != "" && res.K > 0 {
			if err := os.WriteFile(cfg.out+".start", res.Start, 0o644); err != nil {
				return err
			}
			edits, err := json.MarshalIndent(res.Edits, "", "  ")
			if err != nil {
				return err
			}
			if err := os.WriteFile(cfg.out+".edits.json", append(edits, '\n'), 0o644); err != nil {
				return err
			}
		}
		return enc.Encode(res)
	case "triage":
		return h.triage(cfg, enc)
	case "minroute":
		src, err := os.ReadFile(cfg.in)
		if err != nil {
			return err
		}
		res := h.minimizeRouteDecline(src, cfg.reason, cfg.maxTests, cfg.timeout)
		if cfg.out != "" && res.Reproduced {
			if err := os.WriteFile(cfg.out, res.Input, 0o644); err != nil {
				return err
			}
		}
		return enc.Encode(res)
	case "receipt-check":
		return h.receiptCheck(cfg, enc)
	default:
		return fmt.Errorf("unknown -mode %q", cfg.mode)
	}
}

// harness holds one Go language, one C parser, and route-specific Go parsers.
type harness struct {
	name       string
	entry      grammars.LangEntry
	lang       *gotreesitter.Language
	cParser    *sitter.Parser
	parseLimit time.Duration
	external   map[string]bool
	forest     bool
}

func newHarness(name string, parseLimit time.Duration) (*harness, error) {
	var entry grammars.LangEntry
	found := false
	for _, candidate := range grammars.AllLanguages() {
		if candidate.Name == name && candidate.Language != nil {
			entry, found = candidate, true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("unknown grammar %q", name)
	}
	lang := entry.Language()
	if lang == nil {
		return nil, fmt.Errorf("grammar %q returned a nil language", name)
	}
	cLang, err := cgoharness.COracleLanguage(name)
	if err != nil {
		return nil, fmt.Errorf("load locked C grammar: %w", err)
	}
	cParser := sitter.NewParser()
	if err := cParser.SetLanguage(cLang); err != nil {
		cParser.Close()
		return nil, fmt.Errorf("set locked C grammar: %w", err)
	}
	external := make(map[string]bool, len(lang.ExternalSymbols))
	for _, sym := range lang.ExternalSymbols {
		if int(sym) < len(lang.SymbolNames) {
			external[lang.SymbolNames[sym]] = true
		}
	}
	return &harness{
		name: name, entry: entry, lang: lang, cParser: cParser, parseLimit: parseLimit,
		external: external, forest: gotreesitter.LanguageWantsForest(lang),
	}, nil
}

func (h *harness) close() {
	if h.cParser != nil {
		h.cParser.Close()
	}
}

// receiptRoute is the route the grammar receipt used: compact unless the
// grammar is on the forest route.
func (h *harness) receiptRoute() string {
	if h.forest {
		return "default"
	}
	return "compact"
}

func (h *harness) newParser(route string) *gotreesitter.Parser {
	parser := gotreesitter.NewParser(h.lang)
	switch route {
	case "compact":
		parser.SetAdmissionCandidateRoute(!h.forest)
	case "legacy":
		parser.SetAdmissionCandidateRoute(false)
	}
	if h.parseLimit > 0 {
		parser.SetTimeoutMicros(uint64(h.parseLimit / time.Microsecond))
	}
	return parser
}

func (h *harness) parseGo(parser *gotreesitter.Parser, src []byte, old *gotreesitter.Tree) (*gotreesitter.Tree, error) {
	if h.entry.TokenSourceFactory == nil {
		if old != nil {
			return parser.ParseIncremental(src, old)
		}
		return parser.Parse(src)
	}
	factory := func(input []byte) (gotreesitter.TokenSource, error) {
		ts := h.entry.TokenSourceFactory(input, h.lang)
		if ts == nil {
			return nil, fmt.Errorf("grammar %s token source factory returned nil", h.name)
		}
		return ts, nil
	}
	if old != nil {
		return parser.ParseIncrementalWithTokenSourceFactory(src, old, factory)
	}
	return parser.ParseWithTokenSourceFactory(src, factory)
}

func (h *harness) show(w io.Writer, src []byte, route string) error {
	if route == "auto" {
		route = "default"
	}
	parser := h.newParser(route)
	tree, err := h.parseGo(parser, src, nil)
	if err != nil || tree == nil {
		return fmt.Errorf("Go parse: %v", err)
	}
	defer tree.Release()
	c, err := h.parseC(src)
	if err != nil {
		return err
	}
	defer c.tree.Close()
	fmt.Fprintf(w, "input %q\n", src)
	fmt.Fprintf(w, "go/%s (%s): %s\n", route, tree.ParseStopReason(), tree.RootNode().SExpr(h.lang))
	fmt.Fprintf(w, "c:          %s\n", c.tree.RootNode().ToSexp())
	return nil
}

func (h *harness) incShow(w io.Writer, before, after []byte, route string) error {
	if route == "auto" {
		route = h.receiptRoute()
	}
	start, oldEnd, newEnd := diffEdit(before, after)
	parser := h.newParser(route)
	oldTree, err := h.parseGo(parser, before, nil)
	if err != nil || oldTree == nil {
		return fmt.Errorf("Go parse of before: %v", err)
	}
	fmt.Fprintf(w, "before %q\nafter  %q\nedit   start=%d old_end=%d new_end=%d route=%s\n", before, after, start, oldEnd, newEnd, route)
	fmt.Fprintf(w, "go fresh(before):  %s\n", oldTree.RootNode().SExpr(h.lang))
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte: uint32(start), OldEndByte: uint32(oldEnd), NewEndByte: uint32(newEnd),
		StartPoint: pointAt(before, start), OldEndPoint: pointAt(before, oldEnd),
		NewEndPoint: pointAt(after, newEnd),
	})
	incTree, err := h.parseGo(parser, after, oldTree)
	if err != nil || incTree == nil {
		return fmt.Errorf("Go incremental parse: %v", err)
	}
	fmt.Fprintf(w, "go inc(after):     %s  [%s]\n", incTree.RootNode().SExpr(h.lang), incTree.ParseStopReason())
	freshTree, err := h.parseGo(parser, after, nil)
	if err != nil || freshTree == nil {
		return fmt.Errorf("Go fresh parse: %v", err)
	}
	fmt.Fprintf(w, "go fresh(after):   %s  [%s]\n", freshTree.RootNode().SExpr(h.lang), freshTree.ParseStopReason())
	c, err := h.parseC(after)
	if err != nil {
		return err
	}
	defer c.tree.Close()
	fmt.Fprintf(w, "c fresh(after):    %s\n", c.tree.RootNode().ToSexp())
	if d := firstGoGoDivergence(incTree.RootNode(), freshTree.RootNode(), h.lang, ""); d != "" {
		fmt.Fprintf(w, "first inc/fresh divergence: %s\n", d)
	}
	return nil
}

// firstGoGoDivergence compares two Go trees by type, span, and child count.
func firstGoGoDivergence(a, b *gotreesitter.Node, lang *gotreesitter.Language, path string) string {
	if a == nil || b == nil {
		return fmt.Sprintf("%s: nil a=%v b=%v", path, a == nil, b == nil)
	}
	here := path + "/" + a.Type(lang)
	if a.Type(lang) != b.Type(lang) {
		return fmt.Sprintf("%s: type %s vs %s", here, a.Type(lang), b.Type(lang))
	}
	if a.StartByte() != b.StartByte() || a.EndByte() != b.EndByte() {
		return fmt.Sprintf("%s: span %d..%d vs %d..%d", here, a.StartByte(), a.EndByte(), b.StartByte(), b.EndByte())
	}
	if a.HasError() != b.HasError() {
		return fmt.Sprintf("%s: has_error %v vs %v", here, a.HasError(), b.HasError())
	}
	if a.ChildCount() != b.ChildCount() {
		return fmt.Sprintf("%s: children %d vs %d", here, a.ChildCount(), b.ChildCount())
	}
	for i := 0; i < a.ChildCount(); i++ {
		if d := firstGoGoDivergence(a.Child(i), b.Child(i), lang, fmt.Sprintf("%s[%d]", here, i)); d != "" {
			return d
		}
	}
	return ""
}

// cResult is one C oracle parse.
type cResult struct {
	tree     *sitter.Tree
	digest   string
	hasError bool
}

func (h *harness) parseC(src []byte) (*cResult, error) {
	tree := h.cParser.Parse(src, nil)
	if tree == nil || tree.RootNode() == nil {
		if tree != nil {
			tree.Close()
		}
		return nil, errors.New("locked C parse returned no tree")
	}
	digest, err := cgoharness.COracleDeepDigest(tree)
	if err != nil {
		tree.Close()
		return nil, err
	}
	return &cResult{tree: tree, digest: digest, hasError: tree.RootNode().HasError()}, nil
}

// LeafDiff is the first position where the Go and C leaf sequences differ.
// Leaves are nodes with no children, in document order.
type LeafDiff struct {
	Index      int    `json:"index"`
	GoLeaf     string `json:"go_leaf"`
	CLeaf      string `json:"c_leaf"`
	GoExternal bool   `json:"go_external,omitempty"`
	CExternal  bool   `json:"c_external,omitempty"`
	SameSpan   bool   `json:"same_span"`
}

// RouteOutcome is one Go parse compared with the C parse of the same text.
type RouteOutcome struct {
	Route         string                       `json:"route"`
	Digest        string                       `json:"digest,omitempty"`
	Match         bool                         `json:"match"`
	HasError      bool                         `json:"has_error"`
	StopReason    string                       `json:"stop_reason,omitempty"`
	CompactRoute  string                       `json:"compact_route,omitempty"`
	DeclineReason string                       `json:"decline_reason,omitempty"`
	Divergence    *cgoharness.DumpV1Divergence `json:"divergence,omitempty"`
	Signature     string                       `json:"signature,omitempty"`
	Leaf          *LeafDiff                    `json:"leaf,omitempty"`
	Error         string                       `json:"error,omitempty"`
}

// CheckResult compares both Go routes with C on one text.
type CheckResult struct {
	Grammar       string       `json:"grammar"`
	GrammarSource string       `json:"grammar_source,omitempty"`
	Bytes         int          `json:"bytes"`
	SourceSHA256  string       `json:"source_sha256"`
	CDigest       string       `json:"c_digest,omitempty"`
	CHasError     bool         `json:"c_has_error"`
	CError        string       `json:"c_error,omitempty"`
	Default       RouteOutcome `json:"default"`
	Compact       RouteOutcome `json:"compact"`
	Routes        string       `json:"routes"`
	Class         string       `json:"class"`
}

func (h *harness) check(src []byte) CheckResult {
	res := CheckResult{
		Grammar: h.name, GrammarSource: fmt.Sprint(h.entry.GrammarSource),
		Bytes: len(src), SourceSHA256: sha256Hex(src),
	}
	c, err := h.parseC(src)
	if err != nil {
		res.CError = err.Error()
		res.Class = "c-oracle-error"
		return res
	}
	defer c.tree.Close()
	res.CDigest, res.CHasError = c.digest, c.hasError
	res.Default = h.compareRoute(src, "default", c)
	res.Compact = h.compareRoute(src, "compact", c)
	res.Routes = routeSummary(res.Default, res.Compact)
	primary := res.Default
	if primary.Match {
		primary = res.Compact
	}
	res.Class = classify(res.CHasError, primary)
	return res
}

func routeSummary(def, compact RouteOutcome) string {
	switch {
	case def.Match && compact.Match:
		return "both-match"
	case !def.Match && !compact.Match:
		return "both-mismatch"
	case def.Match:
		return "compact-only"
	default:
		return "default-only"
	}
}

func (h *harness) compareRoute(src []byte, route string, c *cResult) RouteOutcome {
	out := RouteOutcome{Route: route}
	parser := h.newParser(route)
	gotreesitter.ResetAdmissionCandidateCounters()
	tree, err := h.parseGo(parser, src, nil)
	if err != nil || tree == nil || tree.RootNode() == nil {
		out.Error = "Go parse failed"
		if err != nil {
			out.Error = err.Error()
		}
		if tree != nil {
			tree.Release()
		}
		return out
	}
	defer tree.Release()
	out.StopReason = fmt.Sprint(tree.ParseStopReason())
	if route == "compact" {
		routed, declined := gotreesitter.AdmissionCandidateCounters()
		switch {
		case h.forest || tree.UsedForestFastPath():
			out.CompactRoute = "forest"
		case routed > 0:
			out.CompactRoute = "accepted"
		case declined > 0:
			out.CompactRoute = "declined"
			out.DeclineReason = gotreesitter.AdmissionCandidateLastFallbackReason()
		default:
			out.CompactRoute = "not-eligible"
		}
	}
	root := tree.RootNode()
	out.HasError = root.HasError()
	inspection, err := benchfixtures.InspectGoTree(root, h.lang)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	out.Digest = inspection.SHA256
	out.Match = out.Digest == c.digest
	if out.Match {
		return out
	}
	out.Divergence = cgoharness.FirstDivergenceDumpV1(root, h.lang, c.tree.RootNode())
	out.Signature = divergenceSignature(out.Divergence)
	out.Leaf = h.firstLeafDiff(root, c.tree.RootNode())
	return out
}

func (h *harness) firstLeafDiff(goRoot *gotreesitter.Node, cRoot *sitter.Node) *LeafDiff {
	goLeaves := leaves(cgoharness.DumpV1FromGo(goRoot, h.lang).Nodes)
	cLeaves := leaves(cgoharness.DumpV1FromC(cRoot).Nodes)
	n := len(goLeaves)
	if len(cLeaves) > n {
		n = len(cLeaves)
	}
	for i := 0; i < n; i++ {
		var g, c *cgoharness.DumpV1Node
		if i < len(goLeaves) {
			g = &goLeaves[i]
		}
		if i < len(cLeaves) {
			c = &cLeaves[i]
		}
		if g != nil && c != nil && g.Type == c.Type && g.StartByte == c.StartByte && g.EndByte == c.EndByte && g.IsExtra == c.IsExtra {
			continue
		}
		diff := &LeafDiff{Index: i, GoLeaf: leafString(g), CLeaf: leafString(c)}
		if g != nil {
			diff.GoExternal = h.external[g.Type]
		}
		if c != nil {
			diff.CExternal = h.external[c.Type]
		}
		diff.SameSpan = g != nil && c != nil && g.StartByte == c.StartByte && g.EndByte == c.EndByte
		return diff
	}
	return nil
}

func leaves(nodes []cgoharness.DumpV1Node) []cgoharness.DumpV1Node {
	out := make([]cgoharness.DumpV1Node, 0, len(nodes)/2)
	for _, node := range nodes {
		if node.ChildCount == 0 {
			out = append(out, node)
		}
	}
	return out
}

func leafString(node *cgoharness.DumpV1Node) string {
	if node == nil {
		return "<none>"
	}
	extra := ""
	if node.IsExtra {
		extra = " extra"
	}
	return fmt.Sprintf("%s@%d..%d%s", node.Type, node.StartByte, node.EndByte, extra)
}

// divergenceSignature identifies a divergence independently of byte offsets
// and child indexes, so a shrunk input can be checked for the same defect.
func divergenceSignature(d *cgoharness.DumpV1Divergence) string {
	if d == nil {
		return "digest-only"
	}
	last := lastPathType(d.Path)
	switch d.Category {
	case "range", "shape":
		return d.Category + "@" + last
	default:
		return d.Category + ":" + d.GoValue + "|" + d.CValue + "@" + last
	}
}

func lastPathType(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		path = path[i+1:]
	}
	if i := strings.LastIndex(path, "["); i > 0 {
		path = path[:i]
	}
	return path
}

// classify names the likely root-cause class of a mismatch. It is a
// heuristic: it reads the C error state, the first differing leaf, and the
// first structural divergence.
func classify(cHasError bool, out RouteOutcome) string {
	switch {
	case out.Error != "":
		return "go-parse-error"
	case out.Match:
		return "match"
	case out.StopReason != "" && out.StopReason != string(gotreesitter.ParseStopAccepted) && out.StopReason != "accepted":
		return "go-stopped:" + out.StopReason
	case cHasError && !out.HasError:
		return "recovery:c-error-go-clean"
	case cHasError:
		return "recovery:both-error"
	case out.HasError:
		return "valid:go-error"
	}
	if out.Leaf != nil {
		switch {
		case out.Leaf.GoExternal || out.Leaf.CExternal:
			return "valid:token-external-scanner"
		case out.Leaf.SameSpan:
			return "valid:token-type"
		default:
			return "valid:token-boundary"
		}
	}
	if out.Divergence == nil {
		return "valid:digest-only"
	}
	switch out.Divergence.Category {
	case "type", "shape", "range":
		return "valid:structure-" + out.Divergence.Category
	default:
		return "valid:" + out.Divergence.Category
	}
}

// ---------------------------------------------------------------------------
// Fresh-parse shrinking.

// MinResult reports one fresh-parse shrink.
type MinResult struct {
	Grammar      string      `json:"grammar"`
	Route        string      `json:"route"`
	Reproduced   bool        `json:"reproduced"`
	OrigBytes    int         `json:"orig_bytes"`
	MinBytes     int         `json:"min_bytes"`
	Tests        int         `json:"tests"`
	Elapsed      string      `json:"elapsed"`
	Signature    string      `json:"signature,omitempty"`
	CHasError    bool        `json:"c_has_error"`
	Check        CheckResult `json:"check"`
	Input        []byte      `json:"-"`
	InputText    string      `json:"input"`
	BudgetExceed bool        `json:"budget_exceeded,omitempty"`
}

func (h *harness) minimizeFresh(src []byte, route string, maxTests int, timeout time.Duration) (MinResult, error) {
	start := time.Now()
	orig := h.check(src)
	if route == "auto" {
		route = "default"
		if orig.Default.Match {
			route = "compact"
		}
	}
	outcome := orig.Default
	if route == "compact" {
		outcome = orig.Compact
	}
	res := MinResult{Grammar: h.name, Route: route, OrigBytes: len(src), CHasError: orig.CHasError}
	if orig.CError != "" || outcome.Match || outcome.Error != "" {
		res.Check = orig
		res.Input = src
		res.InputText = string(src)
		res.MinBytes = len(src)
		res.Elapsed = time.Since(start).String()
		return res, nil
	}
	res.Reproduced = true
	res.Signature = outcome.Signature
	wantCError := orig.CHasError
	wantSig := outcome.Signature
	deadline := start.Add(timeout)
	tests := 0
	interesting := func(candidate []byte) bool {
		if tests >= maxTests || time.Now().After(deadline) {
			res.BudgetExceed = true
			return false
		}
		tests++
		c, err := h.parseC(candidate)
		if err != nil {
			return false
		}
		defer c.tree.Close()
		if c.hasError != wantCError {
			return false
		}
		got := h.compareRoute(candidate, route, c)
		if got.Error != "" || got.Match {
			return false
		}
		if got.StopReason != outcome.StopReason {
			return false
		}
		return got.Signature == wantSig
	}
	current := shrinkBytes(src, interesting)
	res.Tests = tests
	res.Input = current
	res.InputText = string(current)
	res.MinBytes = len(current)
	res.Check = h.check(current)
	res.Elapsed = time.Since(start).String()
	return res, nil
}

// shrinkBytes runs ddmin over lines, then over UTF-8 characters.
func shrinkBytes(src []byte, interesting func([]byte) bool) []byte {
	lines := splitLines(src)
	lines = ddmin(lines, func(units [][]byte) bool { return interesting(bytes.Join(units, nil)) })
	chars := splitChars(bytes.Join(lines, nil))
	chars = ddmin(chars, func(units [][]byte) bool { return interesting(bytes.Join(units, nil)) })
	return bytes.Join(chars, nil)
}

func splitLines(src []byte) [][]byte {
	var out [][]byte
	for len(src) > 0 {
		i := bytes.IndexByte(src, '\n')
		if i < 0 {
			out = append(out, src)
			break
		}
		out = append(out, src[:i+1])
		src = src[i+1:]
	}
	return out
}

func splitChars(src []byte) [][]byte {
	out := make([][]byte, 0, len(src))
	for len(src) > 0 {
		_, size := utf8.DecodeRune(src)
		if size <= 0 {
			size = 1
		}
		out = append(out, src[:size])
		src = src[size:]
	}
	return out
}

// ddmin is the complement-first delta-debugging loop. It returns a subset of
// units for which test is still true and from which no single chunk at the
// final granularity can be removed.
func ddmin[T any](units []T, test func([]T) bool) []T {
	n := 2
	for len(units) >= 2 {
		chunk := (len(units) + n - 1) / n
		reduced := false
		for start := 0; start < len(units); start += chunk {
			end := start + chunk
			if end > len(units) {
				end = len(units)
			}
			candidate := make([]T, 0, len(units)-(end-start))
			candidate = append(candidate, units[:start]...)
			candidate = append(candidate, units[end:]...)
			if test(candidate) {
				units = candidate
				if n > 2 {
					n--
				}
				reduced = true
				break
			}
		}
		if reduced {
			continue
		}
		if n >= len(units) {
			break
		}
		n *= 2
		if n > len(units) {
			n = len(units)
		}
	}
	if len(units) == 1 {
		if test(units[:0]) {
			return units[:0]
		}
	}
	return units
}

// ---------------------------------------------------------------------------
// Receipt edit session replay.

// StepReport is one replayed session step.
type StepReport struct {
	Step              int    `json:"step"`
	EditClass         string `json:"edit_class"`
	Site              int    `json:"site"`
	Start             int    `json:"start"`
	OldEnd            int    `json:"old_end"`
	Replacement       string `json:"replacement"`
	SourceSHA256      string `json:"source_sha256"`
	GoIncEqualsFresh  bool   `json:"go_inc_equals_fresh"`
	GoFreshEqualsC    bool   `json:"go_fresh_equals_c"`
	GoIncEqualsC      bool   `json:"go_inc_equals_c"`
	CIncEqualsFresh   bool   `json:"c_inc_equals_fresh"`
	CHasError         bool   `json:"c_has_error"`
	GoFreshHasError   bool   `json:"go_fresh_has_error"`
	OneStepReproduces *bool  `json:"one_step_reproduces,omitempty"`
	Dumped            string `json:"dumped,omitempty"`
	Error             string `json:"error,omitempty"`
}

func (h *harness) replaySession(seed []byte, route, dumpDir string, maxDumps int) []StepReport {
	var reports []StepReport
	if len(seed) == 0 {
		seed = []byte("\n")
	}
	parser := h.newParser(route)
	current := append([]byte(nil), seed...)
	tree, err := h.parseGo(parser, current, nil)
	if err != nil || tree == nil {
		return []StepReport{{Error: fmt.Sprintf("initial Go parse: %v", err)}}
	}
	cTree := h.cParser.Parse(current, nil)
	if cTree == nil {
		tree.Release()
		return []StepReport{{Error: "initial C parse returned no tree"}}
	}
	dumps := 0
	dumpedParity := false
	step := 0
	for _, editClass := range []string{"insert", "replace", "delete"} {
		for classStep := 0; classStep < sessionStepsPerEditClass; classStep++ {
			site := classStep % sessionSitesPerEditClass
			start, oldEnd, replacement := selectEdit(current, editClass, site)
			next := applyEdit(current, start, oldEnd, replacement)
			edit := gotreesitter.InputEdit{
				StartByte: uint32(start), OldEndByte: uint32(oldEnd), NewEndByte: uint32(start + len(replacement)),
				StartPoint: pointAt(current, start), OldEndPoint: pointAt(current, oldEnd),
				NewEndPoint: pointAt(next, start+len(replacement)),
			}
			step++
			report := StepReport{
				Step: step, EditClass: editClass, Site: site, Start: start, OldEnd: oldEnd,
				Replacement: string(replacement), SourceSHA256: sha256Hex(next),
			}
			tree.Edit(edit)
			incTree, incErr := h.parseGo(parser, next, tree)
			if incTree != tree {
				tree.Release()
			}
			if incErr != nil || incTree == nil {
				report.Error = fmt.Sprintf("Go incremental parse: %v", incErr)
				reports = append(reports, report)
				cTree.Close()
				return reports
			}
			freshTree, freshErr := h.parseGo(parser, next, nil)
			if freshErr != nil || freshTree == nil {
				report.Error = fmt.Sprintf("Go fresh parse: %v", freshErr)
				reports = append(reports, report)
				incTree.Release()
				cTree.Close()
				return reports
			}
			incDigest := goDigest(incTree, h.lang)
			freshDigest := goDigest(freshTree, h.lang)
			report.GoIncEqualsFresh = incDigest == freshDigest
			report.GoFreshHasError = freshTree.RootNode().HasError()

			cEdit := sitter.InputEdit{
				StartByte: uint(start), OldEndByte: uint(oldEnd), NewEndByte: uint(start + len(replacement)),
				StartPosition: cPoint(edit.StartPoint), OldEndPosition: cPoint(edit.OldEndPoint),
				NewEndPosition: cPoint(edit.NewEndPoint),
			}
			cTree.Edit(&cEdit)
			cInc := h.cParser.Parse(next, cTree)
			cTree.Close()
			cTree = cInc
			cFresh, cErr := h.parseC(next)
			if cErr != nil || cTree == nil {
				report.Error = "C parse failed"
				reports = append(reports, report)
				freshTree.Release()
				incTree.Release()
				if cTree != nil {
					cTree.Close()
				}
				return reports
			}
			cIncDigest, _ := cgoharness.COracleDeepDigest(cTree)
			report.CIncEqualsFresh = cIncDigest == cFresh.digest
			report.GoFreshEqualsC = freshDigest == cFresh.digest
			report.GoIncEqualsC = incDigest == cFresh.digest
			report.CHasError = cFresh.hasError
			cFresh.tree.Close()

			if !report.GoIncEqualsFresh {
				oneStep := h.incrementalDiffers(current, next, route)
				report.OneStepReproduces = &oneStep
			}
			if dumpDir != "" && !report.GoIncEqualsFresh && dumps < maxDumps {
				dumps++
				report.Dumped = h.dumpStep(dumpDir, "inv", step, current, next)
			} else if dumpDir != "" && report.GoIncEqualsFresh && !report.GoFreshEqualsC && !dumpedParity {
				dumpedParity = true
				report.Dumped = h.dumpStep(dumpDir, "parity", step, current, next)
			}
			reports = append(reports, report)
			freshTree.Release()
			tree = incTree
			current = next
		}
	}
	tree.Release()
	cTree.Close()
	return reports
}

func (h *harness) dumpStep(dir, kind string, step int, before, after []byte) string {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "dump failed: " + err.Error()
	}
	base := filepath.Join(dir, fmt.Sprintf("%s-%s-step%02d", h.name, kind, step))
	if err := os.WriteFile(base+".before", before, 0o644); err != nil {
		return "dump failed: " + err.Error()
	}
	if err := os.WriteFile(base+".after", after, 0o644); err != nil {
		return "dump failed: " + err.Error()
	}
	return base
}

// incrementalDiffers parses before fresh, applies the single edit that turns
// before into after, reparses incrementally, and reports whether the result
// differs from a fresh parse of after.
func (h *harness) incrementalDiffers(before, after []byte, route string) bool {
	differs, ok := h.incrementalCompare(before, after, route)
	return ok && differs
}

func (h *harness) incrementalCompare(before, after []byte, route string) (differs, ok bool) {
	inc, fresh, ok := h.incrementalDigests(before, after, route)
	return ok && inc != fresh, ok
}

// incrementalDigests parses before fresh, applies the single edit that turns
// before into after, reparses incrementally, and returns the digests of the
// incremental tree and of a fresh parse of after. ok is false when a parse
// fails or does not stop at an accepted state.
func (h *harness) incrementalDigests(before, after []byte, route string) (incDigest, freshDigest string, ok bool) {
	start, oldEnd, newEnd := diffEdit(before, after)
	parser := h.newParser(route)
	oldTree, err := h.parseGo(parser, before, nil)
	if err != nil || oldTree == nil {
		return "", "", false
	}
	if reason := oldTree.ParseStopReason(); reason != gotreesitter.ParseStopAccepted {
		oldTree.Release()
		return "", "", false
	}
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte: uint32(start), OldEndByte: uint32(oldEnd), NewEndByte: uint32(newEnd),
		StartPoint: pointAt(before, start), OldEndPoint: pointAt(before, oldEnd),
		NewEndPoint: pointAt(after, newEnd),
	})
	incTree, err := h.parseGo(parser, after, oldTree)
	if incTree != oldTree {
		oldTree.Release()
	}
	if err != nil || incTree == nil {
		return "", "", false
	}
	defer incTree.Release()
	freshTree, err := h.parseGo(parser, after, nil)
	if err != nil || freshTree == nil {
		return "", "", false
	}
	defer freshTree.Release()
	if incTree.ParseStopReason() != gotreesitter.ParseStopAccepted || freshTree.ParseStopReason() != gotreesitter.ParseStopAccepted {
		return "", "", false
	}
	return goDigest(incTree, h.lang), goDigest(freshTree, h.lang), true
}

// diffEdit returns the single contiguous edit that turns before into after.
func diffEdit(before, after []byte) (start, oldEnd, newEnd int) {
	limit := len(before)
	if len(after) < limit {
		limit = len(after)
	}
	for start < limit && before[start] == after[start] {
		start++
	}
	for start > 0 && start < len(before) && !utf8.RuneStart(before[start]) {
		start--
	}
	suffix := 0
	for suffix < len(before)-start && suffix < len(after)-start && before[len(before)-1-suffix] == after[len(after)-1-suffix] {
		suffix++
	}
	for suffix > 0 && len(before)-suffix < len(before) && !utf8.RuneStart(before[len(before)-suffix]) {
		suffix--
	}
	return start, len(before) - suffix, len(after) - suffix
}

// ---------------------------------------------------------------------------
// Multi-edit chain shrinking.

type ChainEdit struct {
	Start       int    `json:"start"`
	OldEnd      int    `json:"old_end"`
	Replacement string `json:"replacement"`
}

type recordedChainEdit struct {
	ChainEdit
	Before []byte
}

type ChainMinResult struct {
	K              int         `json:"k"`
	Start          []byte      `json:"-"`
	StartText      string      `json:"start_text"`
	Edits          []ChainEdit `json:"edits"`
	BeforeBytes    int         `json:"before_bytes"`
	AfterBytes     int         `json:"after_bytes"`
	Tests          int         `json:"tests"`
	BudgetExceeded bool        `json:"budget_exceeded,omitempty"`
	Error          string      `json:"error,omitempty"`
}

// minimizeChain captures the receipt edits through step and minimizes the
// shortest failing suffix, then its starting text and edit sequence.
func (h *harness) minimizeChain(seed []byte, step int, route string, maxTests int, timeout time.Duration) (ChainMinResult, error) {
	if step < 1 || step > 3*sessionStepsPerEditClass {
		return ChainMinResult{}, fmt.Errorf("-step must be between 1 and %d", 3*sessionStepsPerEditClass)
	}
	if len(seed) == 0 {
		seed = []byte("\n")
	}
	recorded := recordSessionEdits(seed, step)
	result := ChainMinResult{}
	started := time.Now()
	deadline := started.Add(timeout)
	tests := 0
	tryChain := func(start []byte, edits []ChainEdit) bool {
		if tests >= maxTests || time.Now().After(deadline) {
			result.BudgetExceeded = true
			return false
		}
		tests++
		differs, ok := h.chainDiffers(start, edits, route)
		return ok && differs
	}

	// Work from the full prefix back toward the last edit. The first match is
	// the smallest suffix length that still needs incremental history.
	for k := step; k >= 1; k-- {
		first := recorded[step-k]
		edits := recordedChainEdits(recorded[step-k:])
		if tryChain(first.Before, edits) {
			result.K = k
			result.Start = append([]byte(nil), first.Before...)
			result.Edits = edits
			break
		}
		if result.BudgetExceeded {
			break
		}
	}
	result.Tests = tests
	if result.K == 0 {
		result.Error = "no failing edit chain found at the requested session step"
		return result, nil
	}

	startText := result.Start
	chain := result.Edits
	shrinkStart := func(src []byte, edits []ChainEdit) ([]byte, []ChainEdit) {
		units := make([]chainChar, 0, len(splitChars(src)))
		offset := 0
		for _, part := range splitChars(src) {
			units = append(units, chainChar{bytes: part, start: offset, end: offset + len(part)})
			offset += len(part)
		}
		interesting := func(candidate []chainChar) bool {
			if tests >= maxTests || time.Now().After(deadline) {
				result.BudgetExceeded = true
				return false
			}
			tests++
			candidateText, candidateEdits, ok := chainTextAfterRemovals(src, edits, candidate)
			if !ok {
				return false
			}
			differs, valid := h.chainDiffers(candidateText, candidateEdits, route)
			return valid && differs
		}
		units = ddmin(units, interesting)
		candidateText, candidateEdits, ok := chainTextAfterRemovals(src, edits, units)
		if !ok {
			return src, edits
		}
		return candidateText, candidateEdits
	}

	// Removing a chain edit is an additional reduction dimension. Keep the
	// final edit, and translate later offsets back across each omitted edit.
	for {
		beforeText, beforeCount := len(startText), len(chain)
		startText, chain = shrinkStart(startText, chain)
		if len(chain) <= 1 || result.BudgetExceeded {
			break
		}
		indexes := make([]int, len(chain)-1)
		for i := range indexes {
			indexes[i] = i
		}
		interestingEdits := func(kept []int) bool {
			if tests >= maxTests || time.Now().After(deadline) {
				result.BudgetExceeded = true
				return false
			}
			tests++
			candidate := dropChainEdits(chain, kept)
			if candidate == nil {
				return false
			}
			differs, ok := h.chainDiffers(startText, candidate, route)
			return ok && differs
		}
		kept := ddmin(indexes, interestingEdits)
		if candidate := dropChainEdits(chain, kept); candidate != nil {
			chain = candidate
		}
		if result.BudgetExceeded || (len(startText) == beforeText && len(chain) == beforeCount) {
			break
		}
	}
	result.Start = append([]byte(nil), startText...)
	result.StartText = string(startText)
	result.Edits = chain
	result.BeforeBytes = len(startText)
	result.AfterBytes = len(applyChain(startText, chain))
	result.Tests = tests
	return result, nil
}

type chainChar struct {
	bytes []byte
	start int
	end   int
}

func recordSessionEdits(seed []byte, through int) []recordedChainEdit {
	current := append([]byte(nil), seed...)
	recorded := make([]recordedChainEdit, 0, through)
	step := 0
	for _, class := range []string{"insert", "replace", "delete"} {
		for classStep := 0; classStep < sessionStepsPerEditClass && step < through; classStep++ {
			site := classStep % sessionSitesPerEditClass
			start, oldEnd, replacement := selectEdit(current, class, site)
			recorded = append(recorded, recordedChainEdit{
				ChainEdit: ChainEdit{Start: start, OldEnd: oldEnd, Replacement: string(replacement)},
				Before:    append([]byte(nil), current...),
			})
			current = applyEdit(current, start, oldEnd, replacement)
			step++
		}
	}
	return recorded
}

func recordedChainEdits(recorded []recordedChainEdit) []ChainEdit {
	out := make([]ChainEdit, len(recorded))
	for i := range recorded {
		out[i] = recorded[i].ChainEdit
	}
	return out
}

func (h *harness) chainDiffers(start []byte, edits []ChainEdit, route string) (differs, ok bool) {
	parser := h.newParser(route)
	tree, err := h.parseGo(parser, start, nil)
	if err != nil || tree == nil {
		if tree != nil {
			tree.Release()
		}
		return false, false
	}
	if tree.ParseStopReason() != gotreesitter.ParseStopAccepted {
		tree.Release()
		return false, false
	}
	current := append([]byte(nil), start...)
	for _, edit := range edits {
		if edit.Start < 0 || edit.OldEnd < edit.Start || edit.OldEnd > len(current) {
			tree.Release()
			return false, false
		}
		replacement := []byte(edit.Replacement)
		next := applyEdit(current, edit.Start, edit.OldEnd, replacement)
		tree.Edit(gotreesitter.InputEdit{
			StartByte: uint32(edit.Start), OldEndByte: uint32(edit.OldEnd), NewEndByte: uint32(edit.Start + len(replacement)),
			StartPoint: pointAt(current, edit.Start), OldEndPoint: pointAt(current, edit.OldEnd),
			NewEndPoint: pointAt(next, edit.Start+len(replacement)),
		})
		incTree, parseErr := h.parseGo(parser, next, tree)
		if incTree != tree {
			tree.Release()
		}
		if parseErr != nil || incTree == nil {
			if incTree != nil {
				incTree.Release()
			}
			return false, false
		}
		if incTree.ParseStopReason() != gotreesitter.ParseStopAccepted {
			incTree.Release()
			return false, false
		}
		tree = incTree
		current = next
	}
	incDigest := goDigest(tree, h.lang)
	tree.Release()
	fresh, err := h.parseGo(parser, current, nil)
	if err != nil || fresh == nil {
		if fresh != nil {
			fresh.Release()
		}
		return false, false
	}
	defer fresh.Release()
	if fresh.ParseStopReason() != gotreesitter.ParseStopAccepted {
		return false, false
	}
	return incDigest != goDigest(fresh, h.lang), true
}

func applyChain(start []byte, edits []ChainEdit) []byte {
	current := append([]byte(nil), start...)
	for _, edit := range edits {
		if edit.Start < 0 || edit.OldEnd < edit.Start || edit.OldEnd > len(current) {
			return nil
		}
		current = applyEdit(current, edit.Start, edit.OldEnd, []byte(edit.Replacement))
	}
	return current
}

// adjustEditOffsetsForRemoval removes [removeStart, removeEnd) from the initial
// text and translates every edit through that deletion. It rejects a deletion
// that consumes bytes an edit needs to replace.
func adjustEditOffsetsForRemoval(edits []ChainEdit, removeStart, removeEnd int) ([]ChainEdit, bool) {
	if removeStart < 0 || removeEnd <= removeStart {
		return nil, false
	}
	out := append([]ChainEdit(nil), edits...)
	start, end := removeStart, removeEnd
	width := end - start
	for i := range out {
		edit := &out[i]
		if edit.Start < 0 || edit.OldEnd < edit.Start {
			return nil, false
		}
		if start < edit.OldEnd && end > edit.Start {
			return nil, false
		}
		if end <= edit.Start {
			edit.Start -= width
			edit.OldEnd -= width
			continue
		}
		if start >= edit.OldEnd {
			delta := len(edit.Replacement) - (edit.OldEnd - edit.Start)
			start += delta
			end += delta
			continue
		}
		return nil, false
	}
	return out, true
}

func chainTextAfterRemovals(src []byte, edits []ChainEdit, kept []chainChar) ([]byte, []ChainEdit, bool) {
	var text []byte
	for _, unit := range kept {
		text = append(text, unit.bytes...)
	}
	var removed [][2]int
	cursor := 0
	for _, unit := range kept {
		if unit.start > cursor {
			removed = append(removed, [2]int{cursor, unit.start})
		}
		cursor = unit.end
	}
	if cursor < len(src) {
		removed = append(removed, [2]int{cursor, len(src)})
	}
	adjusted := append([]ChainEdit(nil), edits...)
	for i := len(removed) - 1; i >= 0; i-- {
		var ok bool
		adjusted, ok = adjustEditOffsetsForRemoval(adjusted, removed[i][0], removed[i][1])
		if !ok {
			return nil, nil, false
		}
	}
	return text, adjusted, true
}

// dropChainEdits returns a chain retaining the requested pre-final edits and
// always keeps the final edit. nil means the offset translation was ambiguous.
func dropChainEdits(edits []ChainEdit, kept []int) []ChainEdit {
	if len(edits) == 0 {
		return nil
	}
	keep := make(map[int]bool, len(kept)+1)
	for _, i := range kept {
		keep[i] = true
	}
	keep[len(edits)-1] = true
	out := append([]ChainEdit(nil), edits...)
	for i := len(out) - 2; i >= 0; i-- {
		if keep[i] {
			continue
		}
		out, _ = removeChainEdit(out, i)
		if out == nil {
			return nil
		}
	}
	return out
}

func removeChainEdit(edits []ChainEdit, index int) ([]ChainEdit, bool) {
	if index < 0 || index >= len(edits)-1 {
		return nil, false
	}
	removed := edits[index]
	postStart := removed.Start
	postEnd := postStart + len(removed.Replacement)
	delta := (removed.OldEnd - removed.Start) - len(removed.Replacement)
	out := append([]ChainEdit(nil), edits...)
	for i := index + 1; i < len(out); i++ {
		next := &out[i]
		if next.Start < postEnd && next.OldEnd > postStart {
			return nil, false
		}
		if next.Start == next.OldEnd && next.Start > postStart && next.Start < postEnd {
			return nil, false
		}
		if next.Start >= postEnd {
			next.Start += delta
			next.OldEnd += delta
		}
	}
	out = append(out[:index], out[index+1:]...)
	return out, true
}

// ---------------------------------------------------------------------------
// Incremental shrinking.

// IncMinResult reports one incremental shrink.
type IncMinResult struct {
	Grammar      string `json:"grammar"`
	Route        string `json:"route"`
	Reproduced   bool   `json:"reproduced"`
	OrigBytes    int    `json:"orig_bytes"`
	MinBytes     int    `json:"min_bytes"`
	Tests        int    `json:"tests"`
	Elapsed      string `json:"elapsed"`
	Before       []byte `json:"-"`
	After        []byte `json:"-"`
	BeforeText   string `json:"before"`
	AfterText    string `json:"after"`
	GoFreshMatch bool   `json:"go_fresh_after_matches_c"`
	GoIncMatch   bool   `json:"go_inc_after_matches_c"`
	CHasError    bool   `json:"c_after_has_error"`
	BudgetExceed bool   `json:"budget_exceeded,omitempty"`
}

type editUnit struct {
	part  byte // 'p' prefix, 's' suffix
	bytes []byte
}

func (h *harness) minimizeIncremental(before, after []byte, route string, maxTests int, timeout time.Duration) IncMinResult {
	start := time.Now()
	res := IncMinResult{Grammar: h.name, Route: route, OrigBytes: len(before)}
	differs, ok := h.incrementalCompare(before, after, route)
	if !ok || !differs {
		res.Before, res.After = before, after
		res.BeforeText, res.AfterText = string(before), string(after)
		res.MinBytes = len(before)
		res.Elapsed = time.Since(start).String()
		return res
	}
	res.Reproduced = true
	s, oe, ne := diffEdit(before, after)
	removed := append([]byte(nil), before[s:oe]...)
	inserted := append([]byte(nil), after[s:ne]...)
	var units []editUnit
	for _, ch := range splitChars(before[:s]) {
		units = append(units, editUnit{part: 'p', bytes: ch})
	}
	for _, ch := range splitChars(before[oe:]) {
		units = append(units, editUnit{part: 's', bytes: ch})
	}
	build := func(us []editUnit) ([]byte, []byte) {
		var pre, suf []byte
		for _, u := range us {
			if u.part == 'p' {
				pre = append(pre, u.bytes...)
			} else {
				suf = append(suf, u.bytes...)
			}
		}
		b := append(append(append([]byte(nil), pre...), removed...), suf...)
		a := append(append(append([]byte(nil), pre...), inserted...), suf...)
		return b, a
	}
	deadline := start.Add(timeout)
	tests := 0
	interesting := func(us []editUnit) bool {
		if tests >= maxTests || time.Now().After(deadline) {
			res.BudgetExceed = true
			return false
		}
		tests++
		b, a := build(us)
		d, ok := h.incrementalCompare(b, a, route)
		return ok && d
	}
	// Coarse pass over lines first: group units into line-sized chunks.
	units = shrinkEditUnits(units, interesting)
	b, a := build(units)
	res.Before, res.After = b, a
	res.BeforeText, res.AfterText = string(b), string(a)
	res.MinBytes = len(b)
	res.Tests = tests
	if c, err := h.parseC(a); err == nil {
		res.CHasError = c.hasError
		if incDigest, freshDigest, ok := h.incrementalDigests(b, a, route); ok {
			res.GoIncMatch = incDigest == c.digest
			res.GoFreshMatch = freshDigest == c.digest
		}
		c.tree.Close()
	}
	res.Elapsed = time.Since(start).String()
	return res
}

func shrinkEditUnits(units []editUnit, interesting func([]editUnit) bool) []editUnit {
	// Group into lines for the first pass.
	var lines [][]editUnit
	var line []editUnit
	for _, u := range units {
		line = append(line, u)
		if len(u.bytes) == 1 && u.bytes[0] == '\n' {
			lines = append(lines, line)
			line = nil
		}
	}
	if len(line) > 0 {
		lines = append(lines, line)
	}
	flatten := func(ls [][]editUnit) []editUnit {
		var out []editUnit
		for _, l := range ls {
			out = append(out, l...)
		}
		return out
	}
	lines = ddmin(lines, func(ls [][]editUnit) bool { return interesting(flatten(ls)) })
	return ddmin(flatten(lines), interesting)
}

// ---------------------------------------------------------------------------
// Triage over one receipt.

type receiptFile struct {
	Path         string `json:"path"`
	SourceSHA256 string `json:"source_sha256"`
	Pass         bool   `json:"pass"`
}

type receiptDoc struct {
	Grammar struct {
		Name string `json:"name"`
	} `json:"grammar"`
	Corpus struct {
		Files []struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		} `json:"files"`
	} `json:"corpus"`
	FreshParity struct {
		Status string        `json:"status"`
		Files  []receiptFile `json:"files"`
	} `json:"fresh_parity"`
	IncrementalParity struct {
		Status string `json:"status"`
	} `json:"incremental_parity"`
	InvariantGate struct {
		IncrementalFreshFailures int `json:"incremental_fresh_failures"`
	} `json:"invariant_gate"`
}

func receiptGrammarName(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var doc receiptDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return "", fmt.Errorf("decode receipt %s: %w", path, err)
	}
	return doc.Grammar.Name, nil
}

// TriageRow is one triage finding.
type TriageRow struct {
	Grammar   string        `json:"grammar"`
	Kind      string        `json:"kind"` // fresh | edited-parity | invariant
	File      string        `json:"file"`
	Step      int           `json:"step,omitempty"`
	Routes    string        `json:"routes,omitempty"`
	MinRoutes string        `json:"min_routes,omitempty"`
	Class     string        `json:"class"`
	Signature string        `json:"signature,omitempty"`
	Leaf      *LeafDiff     `json:"leaf,omitempty"`
	CHasError bool          `json:"c_has_error"`
	OrigBytes int           `json:"orig_bytes"`
	MinBytes  int           `json:"min_bytes"`
	MinPath   string        `json:"min_path,omitempty"`
	MinInput  string        `json:"min_input,omitempty"`
	Check     *CheckResult  `json:"check,omitempty"`
	OneStep   *bool         `json:"one_step_reproduces,omitempty"`
	IncMin    *IncMinResult `json:"incmin,omitempty"`
	Note      string        `json:"note,omitempty"`
}

func (h *harness) triage(cfg config, enc *json.Encoder) error {
	data, err := os.ReadFile(cfg.receipt)
	if err != nil {
		return err
	}
	var doc receiptDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("decode receipt: %w", err)
	}
	readCorpus := func(rel, want string) ([]byte, error) {
		path, err := corpusFilePath(cfg.corpus, h.name, rel)
		if err != nil {
			return nil, err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if want != "" && sha256Hex(src) != want {
			return nil, fmt.Errorf("corpus file %s sha256 %s, receipt says %s", rel, sha256Hex(src), want)
		}
		return src, nil
	}
	dump := cfg.dumpDir
	if dump == "" {
		dump = filepath.Join(os.TempDir(), "gts_mismatch")
	}
	// Fresh parity failures.
	for i, file := range doc.FreshParity.Files {
		if file.Pass {
			continue
		}
		src, err := readCorpus(file.Path, file.SourceSHA256)
		if err != nil {
			if encErr := enc.Encode(TriageRow{Grammar: h.name, Kind: "fresh", File: file.Path, Class: "corpus-error", Note: err.Error()}); encErr != nil {
				return encErr
			}
			continue
		}
		row := h.triageFresh(src, cfg, filepath.Join(dump, fmt.Sprintf("%s-fresh%d.min", h.name, i)))
		row.Kind, row.File = "fresh", file.Path
		if err := enc.Encode(row); err != nil {
			return err
		}
	}
	// Session: only when the incremental gate failed.
	if doc.IncrementalParity.Status != "fail" && doc.InvariantGate.IncrementalFreshFailures == 0 {
		return nil
	}
	if len(doc.Corpus.Files) == 0 {
		return nil
	}
	seedPath := doc.Corpus.Files[0].Path
	seed, err := readCorpus(seedPath, doc.Corpus.Files[0].SHA256)
	if err != nil {
		return enc.Encode(TriageRow{Grammar: h.name, Kind: "session", File: seedPath, Class: "corpus-error", Note: err.Error()})
	}
	route := h.receiptRoute()
	steps := h.replaySession(seed, route, dump, cfg.maxDumps)
	for _, step := range steps {
		if step.Error != "" {
			if err := enc.Encode(TriageRow{Grammar: h.name, Kind: "session", File: seedPath, Step: step.Step, Class: "session-error", Note: step.Error}); err != nil {
				return err
			}
			continue
		}
		if step.Dumped == "" {
			continue
		}
		if strings.HasPrefix(step.Dumped, "dump failed") {
			if err := enc.Encode(TriageRow{Grammar: h.name, Kind: "session", File: seedPath, Step: step.Step, Class: "session-error", Note: step.Dumped}); err != nil {
				return err
			}
			continue
		}
		before, errBefore := os.ReadFile(step.Dumped + ".before")
		after, errAfter := os.ReadFile(step.Dumped + ".after")
		if errBefore != nil || errAfter != nil {
			if err := enc.Encode(TriageRow{Grammar: h.name, Kind: "session", File: seedPath, Step: step.Step, Class: "session-error", Note: fmt.Sprintf("read dumped step: %v %v", errBefore, errAfter)}); err != nil {
				return err
			}
			continue
		}
		if !step.GoIncEqualsFresh {
			row := TriageRow{Grammar: h.name, Kind: "invariant", File: seedPath, Step: step.Step, OneStep: step.OneStepReproduces, CHasError: step.CHasError, OrigBytes: len(before)}
			if step.OneStepReproduces != nil && *step.OneStepReproduces {
				inc := h.minimizeIncremental(before, after, route, cfg.maxTests, cfg.timeout)
				row.IncMin = &inc
				row.MinBytes = inc.MinBytes
				row.Class = "invariant:one-step"
				minBase := step.Dumped + ".min"
				errBefore := os.WriteFile(minBase+".before", inc.Before, 0o644)
				errAfter := os.WriteFile(minBase+".after", inc.After, 0o644)
				if errBefore == nil && errAfter == nil {
					row.MinPath = minBase
				} else {
					row.Note = fmt.Sprintf("write shrunk step: %v %v", errBefore, errAfter)
				}
			} else {
				row.Class = "invariant:needs-history"
			}
			if err := enc.Encode(row); err != nil {
				return err
			}
			continue
		}
		row := h.triageFresh(after, cfg, step.Dumped+".min")
		row.Kind, row.File, row.Step = "edited-parity", seedPath, step.Step
		if err := enc.Encode(row); err != nil {
			return err
		}
	}
	return nil
}

// RouteMinResult reports one shrink that keeps a compact-route decline.
type RouteMinResult struct {
	Grammar    string `json:"grammar"`
	Reason     string `json:"reason"`
	Reproduced bool   `json:"reproduced"`
	OrigBytes  int    `json:"orig_bytes"`
	MinBytes   int    `json:"min_bytes"`
	Tests      int    `json:"tests"`
	Input      []byte `json:"-"`
	InputText  string `json:"input"`
	Decline    string `json:"decline_reason,omitempty"`
}

// compactDecline parses src on the compact route and returns the decline
// reason, or "" when the route accepted.
func (h *harness) compactDecline(src []byte) string {
	parser := h.newParser("compact")
	gotreesitter.ResetAdmissionCandidateCounters()
	tree, err := h.parseGo(parser, src, nil)
	if tree != nil {
		defer tree.Release()
	}
	if err != nil || tree == nil {
		return ""
	}
	if routed, declined := gotreesitter.AdmissionCandidateCounters(); routed == 0 && declined > 0 {
		return gotreesitter.AdmissionCandidateLastFallbackReason()
	}
	return ""
}

// minimizeRouteDecline shrinks src while the compact route still declines
// with a reason containing want. No C parse is needed.
func (h *harness) minimizeRouteDecline(src []byte, want string, maxTests int, timeout time.Duration) RouteMinResult {
	res := RouteMinResult{Grammar: h.name, Reason: want, OrigBytes: len(src)}
	first := h.compactDecline(src)
	if !strings.Contains(first, want) {
		res.Input, res.InputText, res.MinBytes, res.Decline = src, string(src), len(src), first
		return res
	}
	res.Reproduced = true
	deadline := time.Now().Add(timeout)
	tests := 0
	min := shrinkBytes(src, func(candidate []byte) bool {
		if tests >= maxTests || time.Now().After(deadline) {
			return false
		}
		tests++
		return strings.Contains(h.compactDecline(candidate), want)
	})
	res.Input, res.InputText, res.MinBytes, res.Tests = min, string(min), len(min), tests
	res.Decline = h.compactDecline(min)
	return res
}

// corpusFilePath joins a receipt's relative path under root/grammar and
// rejects paths that would leave that directory.
func corpusFilePath(root, grammar, rel string) (string, error) {
	base := filepath.Join(root, grammar)
	path := filepath.Join(base, filepath.FromSlash(rel))
	inside, err := filepath.Rel(base, path)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("receipt path %q leaves the corpus directory", rel)
	}
	return path, nil
}

// ReceiptFileCheck is one sampled corpus file of a receipt, checked again.
type ReceiptFileCheck struct {
	Grammar      string `json:"grammar"`
	File         string `json:"file"`
	ReceiptPass  bool   `json:"receipt_pass"`
	CHasError    bool   `json:"c_has_error"`
	DefaultMatch bool   `json:"default_match"`
	CompactMatch bool   `json:"compact_match"`
	Error        string `json:"error,omitempty"`
}

// receiptCheck re-checks every sampled fresh-parity file of one receipt,
// passing or failing, so an experiment can count both fixes and regressions.
func (h *harness) receiptCheck(cfg config, enc *json.Encoder) error {
	data, err := os.ReadFile(cfg.receipt)
	if err != nil {
		return err
	}
	var doc receiptDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("decode receipt: %w", err)
	}
	for _, file := range doc.FreshParity.Files {
		row := ReceiptFileCheck{Grammar: h.name, File: file.Path, ReceiptPass: file.Pass}
		path, err := corpusFilePath(cfg.corpus, h.name, file.Path)
		var src []byte
		if err == nil {
			src, err = os.ReadFile(path)
		}
		if err != nil {
			row.Error = err.Error()
		} else {
			check := h.check(src)
			row.CHasError = check.CHasError
			row.DefaultMatch = check.Default.Match
			row.CompactMatch = check.Compact.Match
			row.Error = check.CError
		}
		if err := enc.Encode(row); err != nil {
			return err
		}
	}
	return nil
}

func (h *harness) triageFresh(src []byte, cfg config, minPath string) TriageRow {
	row := TriageRow{Grammar: h.name, OrigBytes: len(src)}
	check := h.check(src)
	row.Routes = check.Routes
	row.CHasError = check.CHasError
	if check.Routes == "both-match" {
		row.Class = "no-longer-reproduces"
		row.MinBytes = len(src)
		return row
	}
	res, err := h.minimizeFresh(src, "auto", cfg.maxTests, cfg.timeout)
	if err != nil {
		row.Class = "min-error"
		row.Note = err.Error()
		return row
	}
	row.MinBytes = res.MinBytes
	row.MinInput = res.InputText
	row.Signature = res.Signature
	minCheck := res.Check
	row.Check = &minCheck
	row.MinRoutes = res.Check.Routes
	outcome := res.Check.Default
	if res.Route == "compact" {
		outcome = res.Check.Compact
	}
	row.Class = classify(res.Check.CHasError, outcome)
	row.Leaf = outcome.Leaf
	if res.BudgetExceed {
		row.Note = "shrink budget exceeded"
	}
	if minPath != "" {
		if err := os.MkdirAll(filepath.Dir(minPath), 0o755); err == nil {
			if os.WriteFile(minPath, res.Input, 0o644) == nil {
				row.MinPath = minPath
			}
		}
	}
	return row
}

// ---------------------------------------------------------------------------
// Helpers shared with cmd/gts_grammar_receipt.

func selectEdit(source []byte, editClass string, site int) (start, oldEnd int, replacement []byte) {
	length := len(source)
	if editClass == "insert" {
		start = site * (length + 1) / (sessionSitesPerEditClass - 1)
		if start > length {
			start = length
		}
		for start < length && !utf8.RuneStart(source[start]) {
			start++
		}
		return start, start, []byte{'x'}
	}
	if length == 0 {
		return 0, 0, []byte{'x'}
	}
	start = site * length / sessionSitesPerEditClass
	if start >= length {
		start = length - 1
	}
	for start > 0 && !utf8.RuneStart(source[start]) {
		start--
	}
	_, runeSize := utf8.DecodeRune(source[start:])
	if runeSize <= 0 {
		runeSize = 1
	}
	oldEnd = start + runeSize
	if editClass == "replace" {
		value := byte('x')
		if runeSize == 1 && source[start] == value {
			value = 'y'
		}
		return start, oldEnd, []byte{value}
	}
	return start, oldEnd, nil
}

func applyEdit(source []byte, start, oldEnd int, replacement []byte) []byte {
	out := make([]byte, 0, len(source)-(oldEnd-start)+len(replacement))
	out = append(out, source[:start]...)
	out = append(out, replacement...)
	out = append(out, source[oldEnd:]...)
	return out
}

func pointAt(source []byte, offset int) gotreesitter.Point {
	var point gotreesitter.Point
	if offset > len(source) {
		offset = len(source)
	}
	for _, value := range source[:offset] {
		if value == '\n' {
			point.Row++
			point.Column = 0
		} else {
			point.Column++
		}
	}
	return point
}

func cPoint(point gotreesitter.Point) sitter.Point {
	return sitter.Point{Row: uint(point.Row), Column: uint(point.Column)}
}

func goDigest(tree *gotreesitter.Tree, lang *gotreesitter.Language) string {
	if tree == nil || tree.RootNode() == nil {
		return ""
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), lang)
	if err != nil {
		return "error:" + err.Error()
	}
	return inspection.SHA256
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
