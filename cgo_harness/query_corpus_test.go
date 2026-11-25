//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/query"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// queryCorpusPath is corpus 1 of the query cursor port.
const queryCorpusPath = "../internal/query/testdata/cases.jsonl"

// TestQueryCorpus recomputes every record of corpus 1
// (internal/query/testdata/cases.jsonl) from pinned C and fails on a stale
// record, printing the first differing record. Run one language at a time:
//
//	go test -tags treesitter_c_parity -run '^TestQueryCorpus$/^json$' .
//
// GTS_QUERY_CORPUS_UPDATE=1 rewrites the records of the languages that ran
// instead of comparing them, and adds the cases of matchOrderGuardCases and
// parityQuerySemanticsCases that the corpus lacks (class "unclassified": a
// person assigns the class). GTS_QUERY_CORPUS_PRINT=<id>,<id> (or "all")
// logs a case's records from pinned C.
func TestQueryCorpus(t *testing.T) {
	cases, err := query.ReadCorpus(queryCorpusPath)
	if err != nil {
		t.Fatal(err)
	}
	update := os.Getenv("GTS_QUERY_CORPUS_UPDATE") == "1"
	printIDs := queryCorpusPrintSet(os.Getenv("GTS_QUERY_CORPUS_PRINT"))

	added := queryCorpusSyncSourceCases(&cases)
	if len(added) > 0 && !update {
		t.Errorf("the corpus lacks %d cases or ids of the guard and semantics lists; rerun with GTS_QUERY_CORPUS_UPDATE=1:\n  %s",
			len(added), strings.Join(added, "\n  "))
	}

	languages := make(map[string]bool)
	for _, c := range cases {
		languages[c.Language] = true
	}
	names := make([]string, 0, len(languages))
	for name := range languages {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, lang := range names {
		t.Run(lang, func(t *testing.T) {
			for i := range cases {
				c := &cases[i]
				if c.Language != lang {
					continue
				}
				t.Run(c.ID(), func(t *testing.T) {
					got := queryCorpusRecordsFromPinned(t, c)
					if printIDs["all"] || printIDs[c.ID()] {
						t.Logf("%s records from pinned C:\n%s", c.ID(), got.format())
					}
					if update {
						got.store(c)
						return
					}
					got.check(t, c)
				})
			}
		})
	}

	if update {
		if err := query.WriteCorpus(queryCorpusPath, cases); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %d cases to %s", len(cases), queryCorpusPath)
	}
}

// queryCorpusRecords are one case's records as pinned C produces them.
type queryCorpusRecords struct {
	usable bool
	tree   string
	l1, l2 []string
	l3, l4 []string
}

func (r queryCorpusRecords) store(c *query.Case) {
	c.Usable = r.usable
	c.Tree = r.tree
	c.L1 = r.l1
	c.L2 = r.l2
	c.L3 = r.l3
	c.L4 = r.l4
}

func (r queryCorpusRecords) check(t *testing.T, c *query.Case) {
	t.Helper()
	if c.Usable != r.usable {
		t.Errorf("stale usable flag: corpus %v, pinned C and Go trees now compare %v", c.Usable, r.usable)
	}
	if c.Tree != r.tree {
		t.Errorf("stale tree digest: corpus %s, pinned C %s", c.Tree, r.tree)
	}
	// The corpus side is "want" here: each record it stores is what pinned C
	// must still produce.
	for _, layer := range []struct {
		name         query.Layer
		corpus, from []string
	}{
		{query.LayerSteps, c.L1, r.l1},
		{query.LayerAnalysis, c.L2, r.l2},
		{query.LayerMatches, c.L3, r.l3},
		{query.LayerCaptures, c.L4, r.l4},
	} {
		if d, ok := query.FirstDifference(layer.name, layer.corpus, layer.from); ok {
			t.Errorf("stale record: %s (want is the corpus, got is pinned C)", d)
		}
	}
}

func (r queryCorpusRecords) format() string {
	var b strings.Builder
	fmt.Fprintf(&b, "usable=%v tree=%s\n", r.usable, r.tree)
	for _, layer := range []struct {
		name    query.Layer
		records []string
	}{{query.LayerSteps, r.l1}, {query.LayerAnalysis, r.l2}, {query.LayerMatches, r.l3}, {query.LayerCaptures, r.l4}} {
		fmt.Fprintf(&b, "%s (%s):\n", layer.name, query.Digest(layer.records))
		for _, rec := range layer.records {
			fmt.Fprintf(&b, "  %s\n", rec)
		}
	}
	return b.String()
}

func queryCorpusPrintSet(raw string) map[string]bool {
	set := make(map[string]bool)
	for _, id := range strings.Split(raw, ",") {
		if id = strings.TrimSpace(id); id != "" {
			set[id] = true
		}
	}
	return set
}

// queryCorpusRecordsFromPinned parses the case's input with Go and pinned C,
// checks that the trees are structurally equal (compareNodes), and computes
// pinned C's records for the query.
func queryCorpusRecordsFromPinned(t *testing.T, c *query.Case) queryCorpusRecords {
	t.Helper()
	src := []byte(c.Input)
	var r queryCorpusRecords

	cLang, err := ParityCLanguage(c.Language)
	if err != nil {
		t.Fatalf("load C parser: %v", err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatalf("C SetLanguage: %v", err)
	}
	cTree := cParser.Parse(src, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("C parser returned nil tree")
	}
	defer cTree.Close()
	r.tree = pinnedTreeDigest(cTree.RootNode())

	goTree, goLang, goErr := parseWithGo(parityCase{name: c.Language, source: c.Input}, src, nil)
	if goErr != nil {
		t.Logf("unusable: Go parse error: %v", goErr)
	} else {
		var structuralErrs []string
		compareNodes(goTree.RootNode(), goLang, cTree.RootNode(), "root", &structuralErrs)
		releaseGoTree(goTree)
		r.usable = len(structuralErrs) == 0
		if !r.usable {
			t.Logf("unusable: trees differ:\n%s", firstLines(structuralErrs, 6))
		}
	}

	oracleQuery, qErr := sitter.NewQuery(cLang, c.Query)
	if qErr != nil {
		r.l2 = []string{query.Verdict{ErrorKind: queryErrorKindName(qErr.Kind), ErrorOffset: uint32(qErr.Offset)}.String()}
		return r
	}
	defer oracleQuery.Close()
	r.l1 = pinnedStepRecords(oracleQuery, cLang)
	r.l2 = append([]string{query.Verdict{Accepted: true}.String()}, pinnedAnalysisRecords(oracleQuery, cLang)...)
	r.l3 = queryCorpusMatchRecords(collectCExactQueryMatches(t, cLang, cTree, c.Query, src))
	r.l4 = pinnedCaptureStream(oracleQuery, cTree, src)
	return r
}

// pinnedTreeDigest feeds pinned C's tree to the corpus tree digest, with the
// facts compareNodes compares.
func pinnedTreeDigest(root *sitter.Node) string {
	h := query.NewTreeHasher()
	var walk func(n *sitter.Node, depth int, field string)
	walk = func(n *sitter.Node, depth int, field string) {
		count := n.ChildCount()
		h.Add(query.TreeNode{
			Depth: depth, Field: field, Type: n.Kind(), Named: n.IsNamed(), Missing: n.IsMissing(),
			StartByte: uint32(n.StartByte()), EndByte: uint32(n.EndByte()), ChildCount: int(count),
		})
		for i := uint(0); i < count; i++ {
			walk(n.Child(i), depth+1, n.FieldNameForChild(uint32(i)))
		}
	}
	walk(root, 0, "")
	return h.Sum()
}

func queryErrorKindName(kind sitter.QueryErrorKind) string {
	switch kind {
	case sitter.QueryErrorSyntax:
		return "syntax"
	case sitter.QueryErrorNodeType:
		return "node_type"
	case sitter.QueryErrorField:
		return "field"
	case sitter.QueryErrorCapture:
		return "capture"
	case sitter.QueryErrorPredicate:
		return "predicate"
	case sitter.QueryErrorStructure:
		return "structure"
	case sitter.QueryErrorLanguage:
		return "language"
	}
	return fmt.Sprintf("kind%d", int(kind))
}

func pinnedSymbol(lang *sitter.Language, id uint32) query.Symbol {
	return query.Symbol{ID: uint16(id), Name: lang.NodeKindForId(uint16(id))}
}

func pinnedField(lang *sitter.Language, id uint32) query.Symbol {
	return query.Symbol{ID: uint16(id), Name: lang.FieldNameForId(uint16(id))}
}

var pinnedQuantifierNames = map[sitter.CaptureQuantifier]query.Quantifier{
	sitter.CaptureQuantifierZero:       query.QuantifierZero,
	sitter.CaptureQuantifierZeroOrOne:  query.QuantifierZeroOrOne,
	sitter.CaptureQuantifierZeroOrMore: query.QuantifierZeroOrMore,
	sitter.CaptureQuantifierOne:        query.QuantifierOne,
	sitter.CaptureQuantifierOneOrMore:  query.QuantifierOneOrMore,
}

// pinnedStepRecords returns pinned C's L1 records: capture names, the
// quantifiers other than zero, patterns, the pattern map, steps and step
// offsets, read through the query_steps.patch accessor.
func pinnedStepRecords(q *sitter.Query, lang *sitter.Language) []string {
	var out []string
	for i, name := range q.CaptureNames() {
		out = append(out, query.CaptureName{ID: i, Name: name}.String())
	}
	for p := uint(0); p < q.PatternCount(); p++ {
		for capture, quantifier := range q.CaptureQuantifiers(p) {
			if quantifier != sitter.CaptureQuantifierZero {
				out = append(out, query.CaptureQuantifier{Pattern: int(p), Capture: capture, Quantifier: pinnedQuantifierNames[quantifier]}.String())
			}
		}
	}
	for i, row := range q.QueryTableRows(sitter.QueryTablePatterns) {
		out = append(out, query.Pattern{Index: i, StepOffset: row[0], StepCount: row[1], StartByte: row[2], EndByte: row[3]}.String())
	}
	for i, row := range q.QueryTableRows(sitter.QueryTablePatternMap) {
		out = append(out, query.PatternEntry{Index: i, Step: int(row[0]), Pattern: int(row[1]), Rooted: row[2] != 0}.String())
	}
	negated := q.QueryTableRows(sitter.QueryTableNegatedFields)
	for i, row := range q.QueryTableRows(sitter.QueryTableSteps) {
		step := query.Step{
			Index:             i,
			Symbol:            pinnedSymbol(lang, row[0]),
			Depth:             uint16(row[6]),
			AlternativeIndex:  uint16(row[7]),
			Named:             row[9]&sitter.QueryStepIsNamed != 0,
			Immediate:         row[9]&sitter.QueryStepIsImmediate != 0,
			LastChild:         row[9]&sitter.QueryStepIsLastChild != 0,
			PassThrough:       row[9]&sitter.QueryStepIsPassThrough != 0,
			DeadEnd:           row[9]&sitter.QueryStepIsDeadEnd != 0,
			InsideAlternation: row[9]&sitter.QueryStepIsInsideAlternation != 0,
			Missing:           row[9]&sitter.QueryStepIsMissing != 0,
			AlternativeIsSkip: row[9]&sitter.QueryStepAlternativeIsSkip != 0,
		}
		if row[1] != 0 {
			step.Supertype = pinnedSymbol(lang, row[1])
		}
		if row[2] != 0 {
			step.Field = pinnedField(lang, row[2])
		}
		for _, id := range row[3:6] {
			if uint16(id) != query.NoStep {
				step.CaptureIDs = append(step.CaptureIDs, uint16(id))
			}
		}
		for j := int(row[8]); j < len(negated) && negated[j][0] != 0; j++ {
			step.NegatedFields = append(step.NegatedFields, pinnedField(lang, negated[j][0]))
		}
		out = append(out, step.String())
	}
	for _, row := range q.QueryTableRows(sitter.QueryTableStepOffsets) {
		out = append(out, query.StepOffset{ByteOffset: row[0], Step: int(row[1])}.String())
	}
	return out
}

// pinnedAnalysisRecords returns pinned C's L2 records after the verdict: the
// rooted and non-local flags per pattern, the three analysis flags per step,
// and the repeat symbols of rootless patterns.
func pinnedAnalysisRecords(q *sitter.Query, lang *sitter.Language) []string {
	var out []string
	for p := uint(0); p < q.PatternCount(); p++ {
		out = append(out, query.PatternAnalysis{Pattern: int(p), Rooted: q.IsPatternRooted(p), NonLocal: q.IsPatternNonLocal(p)}.String())
	}
	for i, row := range q.QueryTableRows(sitter.QueryTableSteps) {
		out = append(out, query.StepAnalysis{
			Step:                    i,
			ContainsCaptures:        row[9]&sitter.QueryStepContainsCaptures != 0,
			RootPatternGuaranteed:   row[9]&sitter.QueryStepRootPatternGuaranteed != 0,
			ParentPatternGuaranteed: row[9]&sitter.QueryStepParentPatternGuaranteed != 0,
		}.String())
	}
	for _, row := range q.QueryTableRows(sitter.QueryTableRepeatSymbols) {
		out = append(out, query.RepeatSymbol{Symbol: pinnedSymbol(lang, row[0])}.String())
	}
	return out
}

// queryCorpusMatchRecords converts the harness's exact matches (pinned C's
// ordered matches after the binding's text predicates) to L3 records.
func queryCorpusMatchRecords(matches []exactQueryMatch) []string {
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		rec := query.Match{Pattern: m.PatternIndex}
		for _, c := range m.Captures {
			rec.Captures = append(rec.Captures, query.Capture{Name: c.Name, Type: c.Type, Named: c.Named, StartByte: c.StartByte, EndByte: c.EndByte})
		}
		out = append(out, rec.String())
	}
	return out
}

// pinnedCaptureStream returns pinned C's L4 records: the binding's Captures
// iterator (ts_query_cursor_next_capture after the text predicates), one
// record per returned capture.
func pinnedCaptureStream(q *sitter.Query, tree *sitter.Tree, src []byte) []string {
	cursor := sitter.NewQueryCursor()
	defer cursor.Close()
	names := q.CaptureNames()
	captures := cursor.Captures(q, tree.RootNode(), src)
	var out []string
	for {
		m, index := captures.Next()
		if m == nil {
			return out
		}
		c := m.Captures[index]
		name := ""
		if int(c.Index) < len(names) {
			name = names[c.Index]
		}
		out = append(out, query.StreamCapture{
			Pattern: int(m.PatternIndex),
			Index:   int(index),
			Capture: query.Capture{
				Name: name, Type: c.Node.Kind(), Named: c.Node.IsNamed(),
				StartByte: uint32(c.Node.StartByte()), EndByte: uint32(c.Node.EndByte()),
			},
		}.String())
	}
}

// queryCorpusSourceCase is one case of a source list that the corpus must hold.
type queryCorpusSourceCase struct {
	id        string
	semantics bool
	key       query.Key
}

// queryCorpusSourceCases lists the guard cases and the semantics cases. A
// semantics case without its own source runs on the language's smoke sample,
// as TestParityQuerySemantics does.
func queryCorpusSourceCases() []queryCorpusSourceCase {
	var out []queryCorpusSourceCase
	for _, tc := range matchOrderGuardCases {
		out = append(out, queryCorpusSourceCase{id: "guard:" + tc.id, key: query.Key{Language: tc.lang, Query: tc.query, Input: tc.source}})
	}
	for _, tc := range parityQuerySemanticsCases {
		src := tc.source
		if src == "" {
			src = grammars.ParseSmokeSample(tc.lang)
		}
		out = append(out, queryCorpusSourceCase{semantics: true, key: query.Key{Language: tc.lang, Query: tc.query, Input: src}})
	}
	return out
}

// queryCorpusSyncSourceCases adds to cases every source case and id that it
// lacks and returns a description of each addition. A source case whose
// language, query and input equal an existing case's adds its id to that
// case. A semantics case gets the next free id semantics:S<n>.
func queryCorpusSyncSourceCases(cases *[]query.Case) []string {
	byKey := make(map[query.Key]int)
	nextSemantics := 1
	for i := range *cases {
		c := &(*cases)[i]
		byKey[c.Key()] = i
		for _, id := range c.IDs {
			var n int
			if _, err := fmt.Sscanf(id, "semantics:S%d", &n); err == nil && n >= nextSemantics {
				nextSemantics = n + 1
			}
		}
	}
	hasSource := func(c *query.Case, source string) bool {
		for _, id := range c.IDs {
			if query.IDSource(id) == source {
				return true
			}
		}
		return false
	}
	hasID := func(c *query.Case, want string) bool {
		for _, id := range c.IDs {
			if id == want {
				return true
			}
		}
		return false
	}
	var added []string
	for _, sc := range queryCorpusSourceCases() {
		i, ok := byKey[sc.key]
		if ok {
			c := &(*cases)[i]
			switch {
			case sc.semantics && !hasSource(c, "semantics"):
				id := fmt.Sprintf("semantics:S%03d", nextSemantics)
				nextSemantics++
				c.IDs = append(c.IDs, id)
				added = append(added, id+" (same case as "+c.ID()+")")
			case !sc.semantics && !hasID(c, sc.id):
				c.IDs = append(c.IDs, sc.id)
				added = append(added, sc.id+" (same case as "+c.ID()+")")
			}
			continue
		}
		id := sc.id
		if sc.semantics {
			id = fmt.Sprintf("semantics:S%03d", nextSemantics)
			nextSemantics++
		}
		*cases = append(*cases, query.Case{
			IDs: []string{id}, Language: sc.key.Language, Class: query.Unclassified,
			Query: sc.key.Query, Input: sc.key.Input,
		})
		byKey[sc.key] = len(*cases) - 1
		added = append(added, id)
	}
	return added
}
