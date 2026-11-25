//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/cquery"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// cqueryCorpusPath is corpus 1 of the query cursor port.
const cqueryCorpusPath = "../internal/cquery/testdata/cases.jsonl"

// TestCQueryCorpus recomputes every record of corpus 1
// (internal/cquery/testdata/cases.jsonl) from pinned C and fails on a stale
// record, printing the first differing record. Run one language at a time:
//
//	go test -tags treesitter_c_parity -run '^TestCQueryCorpus$/^json$' .
//
// GTS_CQUERY_UPDATE=1 rewrites the records of the languages that ran instead
// of comparing them, and adds the cases of matchOrderGuardCases and
// parityQuerySemanticsCases that the corpus lacks (class "unclassified": a
// person assigns the class). GTS_CQUERY_PRINT=<id>,<id> (or "all") logs a
// case's records from pinned C.
func TestCQueryCorpus(t *testing.T) {
	cases, err := cquery.ReadCorpus(cqueryCorpusPath)
	if err != nil {
		t.Fatal(err)
	}
	update := os.Getenv("GTS_CQUERY_UPDATE") == "1"
	printIDs := cqueryPrintSet(os.Getenv("GTS_CQUERY_PRINT"))

	added := cquerySyncSourceCases(&cases)
	if len(added) > 0 && !update {
		t.Errorf("the corpus lacks %d cases or ids of the guard and semantics lists; rerun with GTS_CQUERY_UPDATE=1:\n  %s",
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
					got := cqueryRecordsFromC(t, c)
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
		if err := cquery.WriteCorpus(cqueryCorpusPath, cases); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %d cases to %s", len(cases), cqueryCorpusPath)
	}
}

// cqueryCRecords are one case's records as pinned C produces them.
type cqueryCRecords struct {
	usable bool
	tree   string
	l1, l2 []string
	l3, l4 []string
}

func (r cqueryCRecords) store(c *cquery.Case) {
	c.Usable = r.usable
	c.Tree = r.tree
	c.L1 = r.l1
	c.L2 = r.l2
	c.L3 = r.l3
	c.L4 = r.l4
}

func (r cqueryCRecords) check(t *testing.T, c *cquery.Case) {
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
		name         cquery.Layer
		corpus, from []string
	}{
		{cquery.LayerSteps, c.L1, r.l1},
		{cquery.LayerAnalysis, c.L2, r.l2},
		{cquery.LayerMatches, c.L3, r.l3},
		{cquery.LayerCaptures, c.L4, r.l4},
	} {
		if d, ok := cquery.FirstDifference(layer.name, layer.corpus, layer.from); ok {
			t.Errorf("stale record: %s (want is the corpus, got is pinned C)", d)
		}
	}
}

func (r cqueryCRecords) format() string {
	var b strings.Builder
	fmt.Fprintf(&b, "usable=%v tree=%s\n", r.usable, r.tree)
	for _, layer := range []struct {
		name    cquery.Layer
		records []string
	}{{cquery.LayerSteps, r.l1}, {cquery.LayerAnalysis, r.l2}, {cquery.LayerMatches, r.l3}, {cquery.LayerCaptures, r.l4}} {
		fmt.Fprintf(&b, "%s (%s):\n", layer.name, cquery.Digest(layer.records))
		for _, rec := range layer.records {
			fmt.Fprintf(&b, "  %s\n", rec)
		}
	}
	return b.String()
}

func cqueryPrintSet(raw string) map[string]bool {
	set := make(map[string]bool)
	for _, id := range strings.Split(raw, ",") {
		if id = strings.TrimSpace(id); id != "" {
			set[id] = true
		}
	}
	return set
}

// cqueryRecordsFromC parses the case's input with Go and pinned C, checks
// that the trees are structurally equal (compareNodes), and computes pinned
// C's records for the query.
func cqueryRecordsFromC(t *testing.T, c *cquery.Case) cqueryCRecords {
	t.Helper()
	src := []byte(c.Input)
	var r cqueryCRecords

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
	r.tree = cqueryCTreeDigest(cTree.RootNode())

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

	cq, qErr := sitter.NewQuery(cLang, c.Query)
	if qErr != nil {
		r.l2 = []string{cquery.Verdict{ErrorKind: cqueryErrorKind(qErr.Kind), ErrorOffset: uint32(qErr.Offset)}.String()}
		return r
	}
	defer cq.Close()
	r.l1 = cqueryStepRecords(cq, cLang)
	r.l2 = append([]string{cquery.Verdict{Accepted: true}.String()}, cqueryAnalysisRecords(cq, cLang)...)
	r.l3 = cqueryMatchRecords(collectCExactQueryMatches(t, cLang, cTree, c.Query, src))
	r.l4 = cqueryCaptureStream(cq, cTree, src)
	return r
}

// cqueryCTreeDigest feeds pinned C's tree to the corpus tree digest, with the
// facts compareNodes compares.
func cqueryCTreeDigest(root *sitter.Node) string {
	h := cquery.NewTreeHasher()
	var walk func(n *sitter.Node, depth int, field string)
	walk = func(n *sitter.Node, depth int, field string) {
		count := n.ChildCount()
		h.Add(cquery.TreeNode{
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

func cqueryErrorKind(kind sitter.QueryErrorKind) string {
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

func cquerySymbol(lang *sitter.Language, id uint32) cquery.Symbol {
	return cquery.Symbol{ID: uint16(id), Name: lang.NodeKindForId(uint16(id))}
}

func cqueryField(lang *sitter.Language, id uint32) cquery.Symbol {
	return cquery.Symbol{ID: uint16(id), Name: lang.FieldNameForId(uint16(id))}
}

var cqueryQuantifierNames = map[sitter.CaptureQuantifier]cquery.Quantifier{
	sitter.CaptureQuantifierZero:       cquery.QuantifierZero,
	sitter.CaptureQuantifierZeroOrOne:  cquery.QuantifierZeroOrOne,
	sitter.CaptureQuantifierZeroOrMore: cquery.QuantifierZeroOrMore,
	sitter.CaptureQuantifierOne:        cquery.QuantifierOne,
	sitter.CaptureQuantifierOneOrMore:  cquery.QuantifierOneOrMore,
}

// cqueryStepRecords returns pinned C's L1 records: capture names, the
// quantifiers other than zero, patterns, the pattern map, steps and step
// offsets, read through the query_steps.patch accessor.
func cqueryStepRecords(q *sitter.Query, lang *sitter.Language) []string {
	var out []string
	for i, name := range q.CaptureNames() {
		out = append(out, cquery.CaptureName{ID: i, Name: name}.String())
	}
	for p := uint(0); p < q.PatternCount(); p++ {
		for capture, quantifier := range q.CaptureQuantifiers(p) {
			if quantifier != sitter.CaptureQuantifierZero {
				out = append(out, cquery.CaptureQuantifier{Pattern: int(p), Capture: capture, Quantifier: cqueryQuantifierNames[quantifier]}.String())
			}
		}
	}
	for i, row := range q.QueryTableRows(sitter.QueryTablePatterns) {
		out = append(out, cquery.Pattern{Index: i, StepOffset: row[0], StepCount: row[1], StartByte: row[2], EndByte: row[3]}.String())
	}
	for i, row := range q.QueryTableRows(sitter.QueryTablePatternMap) {
		out = append(out, cquery.PatternEntry{Index: i, Step: int(row[0]), Pattern: int(row[1]), Rooted: row[2] != 0}.String())
	}
	negated := q.QueryTableRows(sitter.QueryTableNegatedFields)
	for i, row := range q.QueryTableRows(sitter.QueryTableSteps) {
		step := cquery.Step{
			Index:             i,
			Symbol:            cquerySymbol(lang, row[0]),
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
			step.Supertype = cquerySymbol(lang, row[1])
		}
		if row[2] != 0 {
			step.Field = cqueryField(lang, row[2])
		}
		for _, id := range row[3:6] {
			if uint16(id) != cquery.NoStep {
				step.CaptureIDs = append(step.CaptureIDs, uint16(id))
			}
		}
		for j := int(row[8]); j < len(negated) && negated[j][0] != 0; j++ {
			step.NegatedFields = append(step.NegatedFields, cqueryField(lang, negated[j][0]))
		}
		out = append(out, step.String())
	}
	for _, row := range q.QueryTableRows(sitter.QueryTableStepOffsets) {
		out = append(out, cquery.StepOffset{ByteOffset: row[0], Step: int(row[1])}.String())
	}
	return out
}

// cqueryAnalysisRecords returns pinned C's L2 records after the verdict: the
// rooted and non-local flags per pattern, the three analysis flags per step,
// and the repeat symbols of rootless patterns.
func cqueryAnalysisRecords(q *sitter.Query, lang *sitter.Language) []string {
	var out []string
	for p := uint(0); p < q.PatternCount(); p++ {
		out = append(out, cquery.PatternAnalysis{Pattern: int(p), Rooted: q.IsPatternRooted(p), NonLocal: q.IsPatternNonLocal(p)}.String())
	}
	for i, row := range q.QueryTableRows(sitter.QueryTableSteps) {
		out = append(out, cquery.StepAnalysis{
			Step:                    i,
			ContainsCaptures:        row[9]&sitter.QueryStepContainsCaptures != 0,
			RootPatternGuaranteed:   row[9]&sitter.QueryStepRootPatternGuaranteed != 0,
			ParentPatternGuaranteed: row[9]&sitter.QueryStepParentPatternGuaranteed != 0,
		}.String())
	}
	for _, row := range q.QueryTableRows(sitter.QueryTableRepeatSymbols) {
		out = append(out, cquery.RepeatSymbol{Symbol: cquerySymbol(lang, row[0])}.String())
	}
	return out
}

// cqueryMatchRecords converts the harness's exact matches (pinned C's
// ordered matches after the binding's text predicates) to L3 records.
func cqueryMatchRecords(matches []exactQueryMatch) []string {
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		rec := cquery.Match{Pattern: m.PatternIndex}
		for _, c := range m.Captures {
			rec.Captures = append(rec.Captures, cquery.Capture{Name: c.Name, Type: c.Type, Named: c.Named, StartByte: c.StartByte, EndByte: c.EndByte})
		}
		out = append(out, rec.String())
	}
	return out
}

// cqueryCaptureStream returns pinned C's L4 records: the binding's Captures
// iterator (ts_query_cursor_next_capture after the text predicates), one
// record per returned capture.
func cqueryCaptureStream(q *sitter.Query, tree *sitter.Tree, src []byte) []string {
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
		out = append(out, cquery.StreamCapture{
			Pattern: int(m.PatternIndex),
			Index:   int(index),
			Capture: cquery.Capture{
				Name: name, Type: c.Node.Kind(), Named: c.Node.IsNamed(),
				StartByte: uint32(c.Node.StartByte()), EndByte: uint32(c.Node.EndByte()),
			},
		}.String())
	}
}

// cquerySourceCase is one case of a source list that the corpus must hold.
type cquerySourceCase struct {
	id        string
	semantics bool
	key       cquery.Key
}

// cquerySourceCases lists the guard cases and the semantics cases. A
// semantics case without its own source runs on the language's smoke sample,
// as TestParityQuerySemantics does.
func cquerySourceCases() []cquerySourceCase {
	var out []cquerySourceCase
	for _, tc := range matchOrderGuardCases {
		out = append(out, cquerySourceCase{id: "guard:" + tc.id, key: cquery.Key{Language: tc.lang, Query: tc.query, Input: tc.source}})
	}
	for _, tc := range parityQuerySemanticsCases {
		src := tc.source
		if src == "" {
			src = grammars.ParseSmokeSample(tc.lang)
		}
		out = append(out, cquerySourceCase{semantics: true, key: cquery.Key{Language: tc.lang, Query: tc.query, Input: src}})
	}
	return out
}

// cquerySyncSourceCases adds to cases every source case and id that it
// lacks and returns a description of each addition. A source case whose
// language, query and input equal an existing case's adds its id to that
// case. A semantics case gets the next free id semantics:S<n>.
func cquerySyncSourceCases(cases *[]cquery.Case) []string {
	byKey := make(map[cquery.Key]int)
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
	hasSource := func(c *cquery.Case, source string) bool {
		for _, id := range c.IDs {
			if cquery.IDSource(id) == source {
				return true
			}
		}
		return false
	}
	hasID := func(c *cquery.Case, want string) bool {
		for _, id := range c.IDs {
			if id == want {
				return true
			}
		}
		return false
	}
	var added []string
	for _, sc := range cquerySourceCases() {
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
		*cases = append(*cases, cquery.Case{
			IDs: []string{id}, Language: sc.key.Language, Class: cquery.Unclassified,
			Query: sc.key.Query, Input: sc.key.Input,
		})
		byKey[sc.key] = len(*cases) - 1
		added = append(added, id)
	}
	return added
}
