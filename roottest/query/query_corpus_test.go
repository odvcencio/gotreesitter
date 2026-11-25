package query_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/query"
)

// queryCorpusPath is corpus 1 of the query cursor port: pinned C's records
// for each case, written by TestQueryCorpus in cgo_harness.
const queryCorpusPath = "../../internal/query/testdata/cases.jsonl"

// matcherEntry names the entry points of the current matcher that a case
// runs through.
type matcherEntry uint8

const (
	matcherExecute matcherEntry = 1 << iota
	matcherNextMatch
	matcherBoth = matcherExecute | matcherNextMatch
)

func (e matcherEntry) String() string {
	switch e {
	case matcherExecute:
		return "Execute"
	case matcherNextMatch:
		return "NextMatch"
	case matcherBoth:
		return "Execute and NextMatch"
	}
	return "none"
}

// matcherGap is a known difference between the current matcher and pinned C:
// the case's class and the entry points whose ordered matches (L3) differ.
type matcherGap struct {
	class   string
	entries matcherEntry
}

// matcherKnownGaps lists, by case id, every usable corpus case whose ordered
// matches from the current matcher differ from pinned C's. The test fails when
// a case outside the list differs (a regression) and when a listed case
// equals C (remove it). The port in internal/query is meant to empty it.
// On 24 November 2026 the list held the 20 review cases that the review of
// PR 1734 found wrong, the two guard cases marked known, and three queries
// that pinned C rejects and Go compiles.
var matcherKnownGaps = map[string]matcherGap{
	"review:X01": {"alternation hold", matcherBoth},
	"review:X02": {"alternation hold", matcherBoth},
	"review:X03": {"alternation hold", matcherBoth},
	"review:X16": {"alternation hold", matcherBoth},

	"review:X09": {"capture dropped in quantified group", matcherBoth},
	"review:X10": {"capture dropped in quantified group", matcherBoth},

	"guard:K02_longest_match_subset_drop": {"longest-match subset drop", matcherBoth},
	"review:J18":                          {"longest-match subset drop", matcherBoth},

	"review:M1":  {"merge finish", matcherBoth},
	"review:M3":  {"merge finish", matcherBoth},
	"review:X17": {"merge finish", matcherBoth},

	"review:A29": {"non-adjacent repetition", matcherBoth},
	"review:J04": {"non-adjacent repetition", matcherBoth},
	"review:J05": {"non-adjacent repetition", matcherBoth},
	"review:J28": {"non-adjacent repetition", matcherBoth},
	"review:P11": {"non-adjacent repetition", matcherBoth},

	"guard:K01_order_across_roots": {"order across roots and patterns", matcherBoth},
	"review:J22":                   {"order across roots and patterns", matcherBoth},
	"review:J33":                   {"order across roots and patterns", matcherBoth},
	"review:X22":                   {"order across roots and patterns", matcherBoth},
	"review:X23":                   {"order across roots and patterns", matcherBoth},

	"review:A11": {"trailing anchor", matcherBoth},

	// Pinned C rejects these queries as impossible patterns; Go compiles
	// them (the known notes in cgo_harness/parity_query_semantics_test.go).
	"semantics:S040": {"supertype patterns", matcherBoth},
	"semantics:S051": {"supertype patterns", matcherBoth},
	"semantics:S099": {"wildcards and ERROR nodes", matcherBoth},
}

// TestQueryCorpusCurrentMatcher parses each usable corpus input with Go, runs
// the corpus query through Query.Execute and through QueryCursor.NextMatch,
// and compares the ordered matches with pinned C's L3 records.
func TestQueryCorpusCurrentMatcher(t *testing.T) {
	cases, err := query.ReadCorpus(queryCorpusPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := query.Validate(cases); err != nil {
		t.Fatal(err)
	}

	type tally struct{ equal, differ, unusable int }
	bySource := map[matcherEntry]map[string]*tally{}
	count := func(entry matcherEntry, c *query.Case, outcome string) {
		if bySource[entry] == nil {
			bySource[entry] = map[string]*tally{}
		}
		for _, id := range c.IDs {
			source := query.IDSource(id)
			if bySource[entry][source] == nil {
				bySource[entry][source] = &tally{}
			}
			switch outcome {
			case "equal":
				bySource[entry][source].equal++
			case "differ":
				bySource[entry][source].differ++
			default:
				bySource[entry][source].unusable++
			}
		}
	}

	differing := map[string]matcherGap{}
	details := map[string]string{}
	for i := range cases {
		c := &cases[i]
		if !c.Usable {
			count(matcherExecute, c, "unusable")
			count(matcherNextMatch, c, "unusable")
			if digest, err := corpusGoTreeDigest(c); err == nil && digest == c.Tree {
				t.Errorf("%s: the corpus marks the case unusable, but the Go tree now has pinned C's digest; rerun TestQueryCorpus with GTS_QUERY_CORPUS_UPDATE=1", c.ID())
			}
			continue
		}
		got, err := runCurrentMatcher(c)
		if err != nil {
			t.Errorf("%s: %v", c.ID(), err)
			continue
		}
		var gap matcherEntry
		for _, run := range []struct {
			entry   matcherEntry
			records []string
			err     string
		}{{matcherExecute, got.execute, got.compileErr}, {matcherNextMatch, got.nextMatch, got.compileErr}} {
			outcome := "equal"
			switch {
			case !c.Accepted() && run.err == "":
				outcome = "differ"
				details[c.ID()] += fmt.Sprintf("\n    %s: pinned C rejects the query (%s); Go compiles it", run.entry, c.Verdict())
			case c.Accepted() && run.err != "":
				outcome = "differ"
				details[c.ID()] += fmt.Sprintf("\n    %s: Go rejects the query: %s", run.entry, run.err)
			case c.Accepted():
				if d, ok := query.FirstDifference(query.LayerMatches, c.L3, run.records); ok {
					outcome = "differ"
					details[c.ID()] += fmt.Sprintf("\n    %s: %s", run.entry, d)
				}
			}
			if outcome == "differ" {
				gap |= run.entry
			}
			count(run.entry, c, outcome)
		}
		if gap != 0 {
			differing[c.ID()] = matcherGap{class: c.Class, entries: gap}
		}
	}

	var problems []string
	for id, gap := range differing {
		known, ok := matcherKnownGaps[id]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("%s (%s) differs from pinned C through %s:%s", id, gap.class, gap.entries, details[id]))
		case known != gap:
			problems = append(problems, fmt.Sprintf("%s: known gap %s (%s), now %s (%s):%s", id, known.entries, known.class, gap.entries, gap.class, details[id]))
		}
	}
	for id, known := range matcherKnownGaps {
		if _, ok := differing[id]; !ok {
			problems = append(problems, fmt.Sprintf("%s (%s) now equals pinned C through %s, or is no longer usable; remove it from matcherKnownGaps", id, known.class, known.entries))
		}
	}
	sort.Strings(problems)

	for _, entry := range []matcherEntry{matcherExecute, matcherNextMatch} {
		sources := make([]string, 0, len(bySource[entry]))
		for source := range bySource[entry] {
			sources = append(sources, source)
		}
		sort.Strings(sources)
		for _, source := range sources {
			n := bySource[entry][source]
			t.Logf("%s, %s ids: %d equal pinned C at L3, %d differ, %d unusable", entry, source, n.equal, n.differ, n.unusable)
		}
	}
	if len(problems) > 0 {
		t.Errorf("the current matcher's differences from pinned C changed:\n  %s\nThe differences today, as matcherKnownGaps:\n%s",
			strings.Join(problems, "\n  "), formatMatcherGaps(differing))
	}
}

// formatMatcherGaps prints gaps as the source of matcherKnownGaps, by class.
func formatMatcherGaps(gaps map[string]matcherGap) string {
	ids := make([]string, 0, len(gaps))
	for id := range gaps {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if gaps[ids[i]].class != gaps[ids[j]].class {
			return gaps[ids[i]].class < gaps[ids[j]].class
		}
		return ids[i] < ids[j]
	})
	names := map[matcherEntry]string{matcherExecute: "matcherExecute", matcherNextMatch: "matcherNextMatch", matcherBoth: "matcherBoth"}
	var b strings.Builder
	for _, id := range ids {
		fmt.Fprintf(&b, "\t%q: {%q, %s},\n", id, gaps[id].class, names[gaps[id].entries])
	}
	return b.String()
}

// matcherRecords are the current matcher's L3 records for one case.
type matcherRecords struct {
	execute    []string
	nextMatch  []string
	compileErr string
}

// runCurrentMatcher parses the case's input with Go, checks that the tree
// still has pinned C's digest, and runs the query through Execute and
// NextMatch.
func runCurrentMatcher(c *query.Case) (matcherRecords, error) {
	var out matcherRecords
	src := []byte(c.Input)
	tree, lang, err := parseCorpusInput(c.Language, src)
	if err != nil {
		return out, err
	}
	defer tree.Release()
	if digest := goTreeDigest(tree.RootNode(), lang); digest != c.Tree {
		return out, fmt.Errorf("the Go tree (%s) no longer equals pinned C's tree (%s), so the matches cannot be compared; check the parser, then rerun TestQueryCorpus", digest, c.Tree)
	}
	q, err := gotreesitter.NewQuery(c.Query, lang)
	if err != nil {
		out.compileErr = strings.SplitN(err.Error(), "\n", 2)[0]
		return out, nil
	}
	out.execute = matcherMatchRecords(q.Execute(tree), lang)
	cursor := q.Exec(tree.RootNode(), lang, src)
	var matches []gotreesitter.QueryMatch
	for {
		m, ok := cursor.NextMatch()
		if !ok {
			break
		}
		matches = append(matches, m)
	}
	out.nextMatch = matcherMatchRecords(matches, lang)
	return out, nil
}

// matcherMatchRecords converts matches to L3 records, with the fields the
// harness's exactQueryMatch compares apart from text. Captures without a node
// are left out, as there.
func matcherMatchRecords(matches []gotreesitter.QueryMatch, lang *gotreesitter.Language) []string {
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		rec := query.Match{Pattern: m.PatternIndex}
		for _, c := range m.Captures {
			if c.Node == nil {
				continue
			}
			rec.Captures = append(rec.Captures, query.Capture{
				Name: c.Name, Type: c.Node.Type(lang), Named: c.Node.IsNamed(),
				StartByte: c.Node.StartByte(), EndByte: c.Node.EndByte(),
			})
		}
		out = append(out, rec.String())
	}
	return out
}

// parseCorpusInput parses src with the language's supported backend, as the
// harness's parseWithGo does for a fresh parse.
func parseCorpusInput(name string, src []byte) (*gotreesitter.Tree, *gotreesitter.Language, error) {
	var entry *grammars.LangEntry
	for _, e := range grammars.AllLanguages() {
		if e.Name == name {
			e := e
			entry = &e
			break
		}
	}
	if entry == nil {
		return nil, nil, fmt.Errorf("no registry entry for %q", name)
	}
	report, ok := grammars.ParseSupportFor(name)
	if !ok {
		return nil, nil, fmt.Errorf("no parse support report for %q", name)
	}
	lang := entry.Language()
	parser := gotreesitter.NewParser(lang)
	var tree *gotreesitter.Tree
	var err error
	switch report.Backend {
	case grammars.ParseBackendTokenSource:
		if entry.TokenSourceFactory == nil {
			return nil, nil, fmt.Errorf("token source backend without factory for %q", name)
		}
		tree, err = parser.ParseWithTokenSource(src, entry.TokenSourceFactory(src, lang))
	case grammars.ParseBackendDFA, grammars.ParseBackendDFAPartial:
		tree, err = parser.Parse(src)
	default:
		return nil, nil, fmt.Errorf("unsupported parse backend %q for %q", report.Backend, name)
	}
	if err != nil {
		return nil, nil, err
	}
	if tree == nil || tree.RootNode() == nil {
		return nil, nil, fmt.Errorf("parse of %q returned no tree", name)
	}
	return tree, lang, nil
}

func corpusGoTreeDigest(c *query.Case) (string, error) {
	tree, lang, err := parseCorpusInput(c.Language, []byte(c.Input))
	if err != nil {
		return "", err
	}
	defer tree.Release()
	return goTreeDigest(tree.RootNode(), lang), nil
}

// goTreeDigest feeds a Go tree to the corpus tree digest, with the facts
// the harness's compareNodes compares.
func goTreeDigest(root *gotreesitter.Node, lang *gotreesitter.Language) string {
	h := query.NewTreeHasher()
	var walk func(n *gotreesitter.Node, depth int, field string)
	walk = func(n *gotreesitter.Node, depth int, field string) {
		typ := n.Type(lang)
		// The NUL token: C's name for it converts to "" through cgo, as in
		// the harness's dumpV1GoType.
		if typ == "\\0" || typ == "\x00" {
			typ = ""
		}
		count := n.ChildCount()
		h.Add(query.TreeNode{
			Depth: depth, Field: field, Type: typ, Named: n.IsNamed(), Missing: n.IsMissing(),
			StartByte: n.StartByte(), EndByte: n.EndByte(), ChildCount: count,
		})
		for i := 0; i < count; i++ {
			walk(n.Child(i), depth+1, n.FieldNameForChild(i, lang))
		}
	}
	walk(root, 0, "")
	return h.Sum()
}
