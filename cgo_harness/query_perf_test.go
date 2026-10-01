//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"embed"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"slices"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

//go:embed testdata/query_perf/*/*.scm
var queryPerfQueries embed.FS

var queryPerfLanguages = []string{"go", "javascript", "typescript", "python", "rust", "yaml", "java"}

type queryPerfFixture struct {
	source     []byte
	lang       *gts.Language
	tree       *gts.Tree
	cTree      *sitter.Tree
	cLang      *sitter.Language
	start, end uint32
}

func newQueryPerfFixture(tb testing.TB, name string, size int, edited bool) queryPerfFixture {
	tb.Helper()
	source, marker, err := benchfixtures.GeneratedSource(name, size)
	// Exercise nonempty injection queries without changing the pinned common
	// fixture generator used by parser lanes.
	if name == "javascript" {
		source = bytes.ReplaceAll(source, []byte("  const x0 = a + b;"), []byte("  const x0 = html`<p>hello</p>`; // template\n  const regex = /hello+/;"))
		if boundary := bytes.Index(source[size:], []byte("\n\n")); boundary >= 0 {
			source = source[:size+boundary+2]
		}
	}
	if err != nil {
		tb.Fatal(err)
	}
	entry := grammars.DetectLanguageByName(name)
	lang := entry.Language()
	p := gts.NewParser(lang)
	parse := func(src []byte) *gts.Tree {
		var tree *gts.Tree
		var err error
		if entry.TokenSourceFactory != nil {
			tree, err = p.ParseWithTokenSource(src, entry.TokenSourceFactory(src, lang))
		} else {
			tree, err = p.Parse(src)
		}
		if err != nil {
			tb.Fatal(err)
		}
		return tree
	}
	tree := parse(source)
	var start, end uint32
	if edited {
		at := bytes.Index(source[len(source)/2:], []byte(marker))
		if at < 0 {
			tb.Fatal("missing edit marker")
		}
		at += len(source) / 2
		next := bytes.Clone(source)
		next[at] = 'y'
		tree.Edit(canonicalGoInputEdit(source, next, at, at+1, at+1))
		var updated *gts.Tree
		if entry.TokenSourceFactory != nil {
			updated, err = p.ParseIncrementalWithTokenSource(next, tree, entry.TokenSourceFactory(next, lang))
		} else {
			updated, err = p.ParseIncremental(next, tree)
		}
		if err != nil {
			tb.Fatal(err)
		}
		tree.Release()
		tree = updated
		fresh := parse(next)
		var diffs []string
		compareGoNodes(tree.RootNode(), lang, fresh.RootNode(), "root", &diffs)
		fresh.Release()
		if len(diffs) != 0 {
			tb.Fatalf("incremental/fresh differ: %v", diffs[:min(3, len(diffs))])
		}
		source = next
		start, end = uint32(max(0, at-128)), uint32(min(len(source), at+129))
	}
	tb.Cleanup(tree.Release)
	cl, err := COracleLanguage(name)
	if err != nil {
		tb.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		tb.Fatal(err)
	}
	ct := cp.Parse(source, nil)
	if ct == nil {
		tb.Fatal("nil C tree")
	}
	tb.Cleanup(ct.Close)
	if tree.RootNode().HasError() || ct.RootNode().HasError() || tree.RootNode().EndByte() != uint32(len(source)) || ct.RootNode().EndByte() != uint(len(source)) {
		tb.Fatal("fixture did not parse completely and cleanly")
	}
	var diffs []string
	compareNodes(tree.RootNode(), lang, ct.RootNode(), "root", &diffs)
	if len(diffs) != 0 {
		tb.Fatalf("Go/C trees differ before querying: %v", diffs[:min(3, len(diffs))])
	}
	return queryPerfFixture{source, lang, tree, ct, cl, start, end}
}

func queryPerfSources(name string) map[string]string {
	queries := map[string]string{"highlights": grammars.DetectLanguageByName(name).HighlightQuery}
	for _, kind := range []string{"locals", "injections"} {
		if data, err := queryPerfQueries.ReadFile("testdata/query_perf/" + name + "/" + kind + ".scm"); err == nil {
			queries[kind] = string(data)
		}
	}
	return queries
}

func queryPerfCompare(a, b queryPerfCapture) int {
	if n := cmp.Compare(a.start, b.start); n != 0 {
		return n
	}
	if n := cmp.Compare(a.end, b.end); n != 0 {
		return n
	}
	if n := cmp.Compare(a.pattern, b.pattern); n != 0 {
		return n
	}
	return cmp.Compare(a.capture, b.capture)
}

// Read the compile-time candidate index only for a deterministic work receipt.
// Reflection keeps this diagnostic out of the public query API and hot path.
func queryPerfCandidateWork(q *gts.Query, f queryPerfFixture) uint64 {
	index := reflect.ValueOf(q).Elem().FieldByName("rootCandidatesDense")
	fallback := reflect.ValueOf(q).Elem().FieldByName("rootFallbackCandidates").Len()
	var work uint64
	var walk func(*gts.Node)
	walk = func(node *gts.Node) {
		if node == nil {
			return
		}
		if f.end != 0 && (node.StartByte() >= f.end || node.EndByte() < f.start || (node.EndByte() == f.start && node.StartByte() != node.EndByte())) {
			return
		}
		sym := int(f.lang.PublicSymbolForNamedness(node.Symbol(), node.IsNamed()))
		n := fallback
		if sym < index.Len() && !index.Index(sym).IsNil() {
			n = index.Index(sym).Len()
		}
		work += uint64(n)
		for i := 0; i < node.ChildCount(); i++ {
			walk(node.Child(i))
		}
	}
	walk(f.tree.RootNode())
	return work
}

func queryPerfGo(q *gts.Query, ids map[string]uint32, f queryPerfFixture) []queryPerfCapture {
	var rows []queryPerfCapture
	appendMatch := func(m gts.QueryMatch) {
		for _, cap := range m.Captures {
			rows = append(rows, queryPerfCapture{cap.Node.StartByte(), cap.Node.EndByte(), uint32(m.PatternIndex), ids[cap.Name]})
		}
	}
	if f.end == 0 {
		for _, m := range q.Execute(f.tree) {
			appendMatch(m)
		}
	} else {
		cursor := q.Exec(f.tree.RootNode(), f.lang, f.source)
		cursor.SetByteRange(f.start, f.end)
		for {
			m, ok := cursor.NextMatch()
			if !ok {
				break
			}
			appendMatch(m)
		}
		if cursor.DidExceedMatchLimit() {
			panic("query work limit exceeded")
		}
	}
	slices.SortFunc(rows, queryPerfCompare)
	return rows
}

func queryPerfBinding(q *sitter.Query, f queryPerfFixture) []queryPerfCapture {
	cursor := sitter.NewQueryCursor()
	defer cursor.Close()
	cursor.SetByteRange(uint(f.start), uint(f.end))
	matches := cursor.Matches(q, f.cTree.RootNode(), f.source)
	var rows []queryPerfCapture
	for {
		m := matches.Next()
		if m == nil {
			break
		}
		if !cQueryMatchSatisfiesGeneralPredicates(m, q, f.source) {
			continue
		}
		for _, cap := range m.Captures {
			rows = append(rows, queryPerfCapture{uint32(cap.Node.StartByte()), uint32(cap.Node.EndByte()), uint32(m.PatternIndex), cap.Index})
		}
	}
	if cursor.DidExceedMatchLimit() {
		panic("C query work limit exceeded")
	}
	slices.SortFunc(rows, queryPerfCompare)
	return rows
}

type queryPerfNodeCapture struct {
	row   queryPerfCapture
	kind  string
	named bool
}

// Check captured node identity as well as output ranges. Parents and children
// can have equal spans, so byte ranges alone cannot prove capture equality.
func checkQueryPerfNodes(tb testing.TB, q *gts.Query, cq *sitter.Query, ids map[string]uint32, f queryPerfFixture) {
	tb.Helper()
	var got, want []queryPerfNodeCapture
	add := func(m gts.QueryMatch) {
		for _, cap := range m.Captures {
			got = append(got, queryPerfNodeCapture{queryPerfCapture{cap.Node.StartByte(), cap.Node.EndByte(), uint32(m.PatternIndex), ids[cap.Name]}, cap.Node.Type(f.lang), cap.Node.IsNamed()})
		}
	}
	if f.end == 0 {
		for _, m := range q.Execute(f.tree) {
			add(m)
		}
	} else {
		cursor := q.Exec(f.tree.RootNode(), f.lang, f.source)
		cursor.SetByteRange(f.start, f.end)
		for {
			m, ok := cursor.NextMatch()
			if !ok {
				break
			}
			add(m)
		}
	}
	cursor := sitter.NewQueryCursor()
	defer cursor.Close()
	cursor.SetByteRange(uint(f.start), uint(f.end))
	matches := cursor.Matches(cq, f.cTree.RootNode(), f.source)
	for {
		m := matches.Next()
		if m == nil {
			break
		}
		if !cQueryMatchSatisfiesGeneralPredicates(m, cq, f.source) {
			continue
		}
		for _, cap := range m.Captures {
			want = append(want, queryPerfNodeCapture{queryPerfCapture{uint32(cap.Node.StartByte()), uint32(cap.Node.EndByte()), uint32(m.PatternIndex), cap.Index}, cap.Node.Kind(), cap.Node.IsNamed()})
		}
	}
	compare := func(a, b queryPerfNodeCapture) int {
		if n := queryPerfCompare(a.row, b.row); n != 0 {
			return n
		}
		if n := cmp.Compare(a.kind, b.kind); n != 0 {
			return n
		}
		if a.named == b.named {
			return 0
		}
		if a.named {
			return 1
		}
		return -1
	}
	slices.SortFunc(got, compare)
	slices.SortFunc(want, compare)
	if !slices.Equal(got, want) {
		tb.Fatal("captured node types or namedness differ from C")
	}
}

func checkQueryPerf(tb testing.TB, f queryPerfFixture, source string) (*gts.Query, map[string]uint32, *queryPerfNativeOracle, int) {
	tb.Helper()
	q, err := gts.NewQuery(source, f.lang)
	if err != nil {
		tb.Fatal(err)
	}
	ids := make(map[string]uint32)
	for i, name := range q.CaptureNames() {
		ids[name] = uint32(i)
	}
	cq, errC := sitter.NewQuery(f.cLang, source)
	if errC != nil {
		tb.Fatal(errC)
	}
	defer cq.Close()
	if !slices.Equal(q.CaptureNames(), cq.CaptureNames()) {
		tb.Fatal("capture IDs differ")
	}
	oracle, err := newQueryPerfNativeOracle(f.cLang, f.source, source)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(oracle.close)
	want := queryPerfBinding(cq, f)
	native, free, complete := oracle.run(f.start, f.end)
	defer free()
	if !complete || !slices.Equal(want, native) {
		tb.Fatalf("native C/binding outputs differ: binding=%d native=%d complete=%t", len(want), len(native), complete)
	}
	got := queryPerfGo(q, ids, f)
	if !slices.Equal(want, got) {
		i := 0
		for i < min(len(want), len(got)) && want[i] == got[i] {
			i++
		}
		tb.Fatalf("capture mismatch at %d: C=%d Go=%d; C next=%v Go next=%v", i, len(want), len(got), want[i:min(i+3, len(want))], got[i:min(i+3, len(got))])
	}
	checkQueryPerfNodes(tb, q, cq, ids, f)
	return q, ids, oracle, len(want)
}

func TestQueryPerfCaptureParity(t *testing.T) {
	queryPerfUseNativeAllocator()
	t.Cleanup(func() { sitter.SetAllocator(nil, nil, nil, nil) })
	for _, name := range queryPerfLanguages {
		t.Run(name, func(t *testing.T) {
			for _, size := range []int{32, 137} {
				t.Run(fmt.Sprintf("%dKiB", size), func(t *testing.T) {
					for _, edited := range []bool{false, true} {
						mode := "full"
						if edited {
							mode = "editrange"
						}
						t.Run(mode, func(t *testing.T) {
							f := newQueryPerfFixture(t, name, size*1024, edited)
							for kind, source := range queryPerfSources(name) {
								t.Run(kind, func(t *testing.T) {
									q, ids, _, n := checkQueryPerf(t, f, source)
									t.Logf("QUERY_COUNTERS source_bytes=%d source_sha256=%x query_sha256=%x captures=%d capture_sha256=%x candidate_work=%d range=%d:%d", len(f.source), sha256.Sum256(f.source), sha256.Sum256([]byte(source)), n, sha256.Sum256([]byte(fmt.Sprint(queryPerfGo(q, ids, f)))), queryPerfCandidateWork(q, f), f.start, f.end)
								})
							}
						})
					}
				})
			}
		})
	}
}

// Query compilation, parsing and edit/reparse setup stay outside the timer.
// Each measured operation creates a cursor, visits all matches, evaluates text
// predicates, allocates captures and orders them, preserving duplicates.
func BenchmarkQueryPerf(b *testing.B) {
	queryPerfUseNativeAllocator()
	b.Cleanup(func() { sitter.SetAllocator(nil, nil, nil, nil) })
	names := queryPerfLanguages
	if name := os.Getenv("GTS_QUERY_PERF_LANGUAGE"); name != "" {
		names = []string{name}
	}
	rng := rand.New(rand.NewSource(queryPerfShuffleSeed()))
	for _, name := range names {
		b.Run(name, func(b *testing.B) {
			sizes := []int{32, 137}
			rng.Shuffle(len(sizes), func(i, j int) { sizes[i], sizes[j] = sizes[j], sizes[i] })
			for _, size := range sizes {
				b.Run(fmt.Sprintf("%dKiB", size), func(b *testing.B) {
					modes := []bool{false, true}
					rng.Shuffle(len(modes), func(i, j int) { modes[i], modes[j] = modes[j], modes[i] })
					for _, edited := range modes {
						mode := "full"
						if edited {
							mode = "editrange"
						}
						b.Run(mode, func(b *testing.B) {
							f := newQueryPerfFixture(b, name, size*1024, edited)
							queries := queryPerfSources(name)
							kinds := make([]string, 0, len(queries))
							for kind := range queries {
								kinds = append(kinds, kind)
							}
							slices.Sort(kinds)
							rng.Shuffle(len(kinds), func(i, j int) { kinds[i], kinds[j] = kinds[j], kinds[i] })
							for _, kind := range kinds {
								b.Run(kind, func(b *testing.B) {
									q, ids, oracle, captures := checkQueryPerf(b, f, queries[kind])
									// Paired Go-C-C-Go cycles (or their randomized reverse).
									engines := []string{"Go1", "C1", "C2", "Go2"}
									if rng.Intn(2) != 0 {
										engines = []string{"C1", "Go1", "Go2", "C2"}
									}
									for _, engine := range engines {
										b.Run(engine, func(b *testing.B) {
											b.ReportAllocs()
											b.SetBytes(int64(len(f.source)))
											b.ResetTimer()
											for i := 0; i < b.N; i++ {
												if engine[:2] == "Go" {
													if len(queryPerfGo(q, ids, f)) != captures {
														b.Fatal("capture count changed")
													}
												} else {
													rows, free, complete := oracle.run(f.start, f.end)
													n := len(rows)
													free()
													if !complete || n != captures {
														b.Fatal("native capture count changed")
													}
												}
											}
											b.ReportMetric(float64(captures), "captures/op")
										})
									}
								})
							}
						})
					}
				})
			}
		})
	}
}

func queryPerfShuffleSeed() int64 {
	// The benchmark wrapper always supplies an explicit -test.shuffle seed.
	var seed int64 = 1
	for _, arg := range os.Args {
		if len(arg) > 14 && arg[:14] == "-test.shuffle=" {
			fmt.Sscan(arg[14:], &seed)
		}
	}
	return seed
}
