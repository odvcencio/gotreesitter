//go:build cgo && treesitter_c_parity && gts_program_audit

package cgoharness

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// This optional audit reports public navigation, changed-range, and query
// observations. It does not establish graduation, change parser defaults, or
// waive failures in the parity suites. Run with GTS_PROGRAM_AUDIT_LANGUAGE=go
// and the gts_program_audit,treesitter_c_parity tags in an isolated container.
// Add GTS_PROGRAM_AUDIT_LARGE_QUERY=1 to measure the shipped highlight query
// on a generated 1 MiB Go source. Times are diagnostic observations on the
// current host; use randomized benchmarks for performance comparisons.
func TestProgramAuditPublicSurfaces(t *testing.T) {
	if os.Getenv("GTS_PROGRAM_AUDIT_LANGUAGE") != "go" {
		t.Skip("set GTS_PROGRAM_AUDIT_LANGUAGE=go; run one grammar at a time")
	}
	lang := grammars.GoLanguage()
	cLang, err := ParityCLanguage("go")
	if err != nil {
		t.Fatal(err)
	}
	for _, compact := range []bool{false, true} {
		t.Run(fmt.Sprintf("compact=%t", compact), func(t *testing.T) {
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(compact)
			var builder strings.Builder
			builder.WriteString("package audit\n")
			for i := 0; i < 128; i++ {
				fmt.Fprintf(&builder, "func F%d() int { return %d }\n", i, i)
			}
			src := []byte(builder.String())
			old, err := p.Parse(src)
			if err != nil || old == nil {
				t.Fatalf("initial parse: %v", err)
			}
			defer old.Release()
			beforeBad, beforeEdges := programAuditParents(old.RootNode())
			at := strings.Index(string(src), "func F127")
			insert := []byte("func Added() {}\n")
			nextSource := append(append(append([]byte{}, src[:at]...), insert...), src[at:]...)
			edit := gts.InputEdit{StartByte: uint32(at), OldEndByte: uint32(at), NewEndByte: uint32(at + len(insert)), StartPoint: pointAtOffset(src, at), OldEndPoint: pointAtOffset(src, at), NewEndPoint: pointAtOffset(nextSource, at+len(insert))}
			old.Edit(edit)
			next, err := p.ParseIncremental(nextSource, old)
			if err != nil || next == nil {
				t.Fatalf("incremental parse: %v", err)
			}
			defer next.Release()
			fresh, err := p.Parse(nextSource)
			if err != nil || fresh == nil {
				t.Fatalf("fresh parse: %v", err)
			}
			defer fresh.Release()
			oldBad, oldEdges := programAuditParents(old.RootNode())
			newBad, newEdges := programAuditParents(next.RootNode())
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			ct := cp.Parse(nextSource, nil)
			if ct == nil {
				t.Fatal("C parse returned no tree")
			}
			defer ct.Close()
			incrementalDigest := programAuditGoDigest(t, next, lang)
			freshDigest := programAuditGoDigest(t, fresh, lang)
			cDigest, err := COracleDeepDigest(ct)
			if err != nil {
				t.Fatal(err)
			}
			rt := next.ParseRuntime()
			programAuditLog(t, "navigation", map[string]any{
				"compact_requested": compact, "same_tree": old == next, "same_root": old.RootNode() == next.RootNode(), "before_bad_parents": beforeBad, "before_edges": beforeEdges,
				"old_bad_parents": oldBad, "old_edges": oldEdges, "new_bad_parents": newBad, "new_edges": newEdges,
				"digest_format": benchfixtures.DeepTreeDigestVersion, "incremental_digest": incrementalDigest,
				"fresh_digest": freshDigest, "C_fresh_digest": cDigest,
				"incremental_equals_fresh": incrementalDigest == freshDigest, "incremental_equals_C_fresh": incrementalDigest == cDigest,
				"stop_reason": rt.StopReason, "compact_reuse_route": rt.CompactIncrementalReuseRoute,
				"compact_fallback_reason": rt.CompactIncrementalFallbackReason, "compact_reused_bytes": rt.CompactIncrementalReusedBytes,
			})
			programAuditCaptures(t, next, lang, ct, cLang, nextSource)
		})
	}
	programAuditRanges(t, lang, cLang)
	programAuditQueries(t, lang, cLang)
}

func programAuditGoDigest(t *testing.T, tree *gts.Tree, lang *gts.Language) string {
	t.Helper()
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	return inspection.SHA256
}

func programAuditLog(t *testing.T, kind string, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("PROGRAM_AUDIT %s %s", kind, encoded)
}

func programAuditParents(root *gts.Node) (bad, edges int) {
	stack := []*gts.Node{root}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for i := 0; i < n.ChildCount(); i++ {
			child := n.Child(i)
			edges++
			if child.Parent() != n {
				bad++
			}
			stack = append(stack, child)
		}
	}
	return
}

func programAuditCaptures(t *testing.T, tree *gts.Tree, lang *gts.Language, ct *sitter.Tree, cl *sitter.Language, src []byte) {
	t.Helper()
	text := `(function_declaration name: (identifier) @name body: (block) @body) @function (identifier) @identifier`
	q, err := gts.NewQuery(text, lang)
	if err != nil {
		t.Fatal(err)
	}
	cq, queryErr := sitter.NewQuery(cl, text)
	if queryErr != nil {
		t.Fatal(queryErr)
	}
	defer cq.Close()
	cursor := q.Exec(tree.RootNode(), lang, src)
	cc := sitter.NewQueryCursor()
	defer cc.Close()
	captures := cc.Captures(cq, ct.RootNode(), src)
	var goOrder, cOrder []uint
	for i := 0; i < 8; i++ {
		if capture, ok := cursor.NextCapture(); ok {
			goOrder = append(goOrder, uint(capture.Node.StartByte()))
		}
		if match, index := captures.Next(); match != nil {
			cOrder = append(cOrder, match.Captures[index].Node.StartByte())
		}
	}
	programAuditLog(t, "capture_order", map[string]any{"go_start_bytes": goOrder, "C_start_bytes": cOrder})
}

func programAuditRanges(t *testing.T, lang *gts.Language, cl *sitter.Language) {
	t.Helper()
	src := []byte("package audit\nvar a = 1\nvar b = 2\n")
	at := strings.Index(string(src), "var b")
	insert := []byte("var c = 3\n")
	nextSource := append(append(append([]byte{}, src[:at]...), insert...), src[at:]...)
	edit := gts.InputEdit{StartByte: uint32(at), OldEndByte: uint32(at), NewEndByte: uint32(at + len(insert)), StartPoint: pointAtOffset(src, at), OldEndPoint: pointAtOffset(src, at), NewEndPoint: pointAtOffset(nextSource, at+len(insert))}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	oldC := cp.Parse(src, nil)
	if oldC == nil {
		t.Fatal("C initial parse returned no tree")
	}
	defer oldC.Close()
	oldC.Edit(&sitter.InputEdit{StartByte: uint(edit.StartByte), OldEndByte: uint(edit.OldEndByte), NewEndByte: uint(edit.NewEndByte), StartPosition: sitter.Point{Row: uint(edit.StartPoint.Row), Column: uint(edit.StartPoint.Column)}, OldEndPosition: sitter.Point{Row: uint(edit.OldEndPoint.Row), Column: uint(edit.OldEndPoint.Column)}, NewEndPosition: sitter.Point{Row: uint(edit.NewEndPoint.Row), Column: uint(edit.NewEndPoint.Column)}})
	nextC := cp.Parse(nextSource, oldC)
	if nextC == nil {
		t.Fatal("C incremental parse returned no tree")
	}
	defer nextC.Close()
	for _, compact := range []bool{false, true} {
		p := gts.NewParser(lang)
		p.SetAdmissionCandidateRoute(compact)
		old, err := p.Parse(src)
		if err != nil || old == nil {
			t.Fatalf("Go initial parse: %v", err)
		}
		old.Edit(edit)
		next, err := p.ParseIncremental(nextSource, old)
		if err != nil || next == nil {
			old.Release()
			t.Fatalf("Go incremental parse: %v", err)
		}
		programAuditLog(t, "changed_ranges", map[string]any{"compact_requested": compact, "go": gts.DiffChangedRanges(old, next), "C": oldC.ChangedRanges(nextC), "source_bytes": len(nextSource)})
		next.Release()
		old.Release()
	}
}

func programAuditQueries(t *testing.T, lang *gts.Language, cl *sitter.Language) {
	t.Helper()
	for _, comments := range []int{16, 32, 64} {
		src := []byte("package audit\n" + strings.Repeat("// audit\n", comments) + "func F() {}\n")
		p := gts.NewParser(lang)
		tree, err := p.Parse(src)
		if err != nil || tree == nil {
			t.Fatalf("Go query tree: %v", err)
		}
		cp := sitter.NewParser()
		if err := cp.SetLanguage(cl); err != nil {
			t.Fatal(err)
		}
		ct := cp.Parse(src, nil)
		if ct == nil {
			t.Fatal("C query tree returned no tree")
		}
		text := `(source_file (comment)+ @comment . (type_declaration) @type)`
		q, err := gts.NewQuery(text, lang)
		if err != nil {
			t.Fatal(err)
		}
		cq, queryErr := sitter.NewQuery(cl, text)
		if queryErr != nil {
			t.Fatal(queryErr)
		}
		start := time.Now()
		matches := q.Execute(tree)
		executeNanos := time.Since(start).Nanoseconds()
		start = time.Now()
		streamed := q.ExecuteInto(tree, nil)
		streamNanos := time.Since(start).Nanoseconds()
		cc := sitter.NewQueryCursor()
		start = time.Now()
		cm := cc.Matches(cq, ct.RootNode(), src)
		cMatches := 0
		for cm.Next() != nil {
			cMatches++
		}
		cNanos := time.Since(start).Nanoseconds()
		programAuditLog(t, "quantified_query", map[string]any{"comments": comments, "execute_matches": len(matches), "stream_matches": len(streamed), "C_matches": cMatches, "execute_ns": executeNanos, "stream_ns": streamNanos, "C_ns": cNanos, "tree_equals_C": FirstDivergenceDumpV1(tree.RootNode(), lang, ct.RootNode()) == nil})
		cc.Close()
		cq.Close()
		ct.Close()
		cp.Close()
		tree.Release()
	}
}

// TestProgramAuditLargeQuery times the same shipped query on already-parsed
// trees. C traversal includes binding overhead, so these observations are
// not a native-C timing receipt. Allocation counts are Go heap charges only.
func TestProgramAuditLargeQuery(t *testing.T) {
	if os.Getenv("GTS_PROGRAM_AUDIT_LANGUAGE") != "go" || os.Getenv("GTS_PROGRAM_AUDIT_LARGE_QUERY") != "1" {
		t.Skip("set GTS_PROGRAM_AUDIT_LANGUAGE=go and GTS_PROGRAM_AUDIT_LARGE_QUERY=1")
	}
	src, _, err := benchfixtures.GeneratedSource("go", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	entry := grammars.DetectLanguageByName("go")
	if entry == nil {
		t.Fatal("missing Go registry entry")
	}
	lang := entry.Language()
	p := gts.NewParser(lang)
	tree, err := p.Parse(src)
	if err != nil || tree == nil {
		t.Fatalf("Go parse: %v", err)
	}
	defer tree.Release()
	cl, err := ParityCLanguage("go")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	ct := cp.Parse(src, nil)
	if ct == nil {
		t.Fatal("C parse returned no tree")
	}
	defer ct.Close()
	goDigest := programAuditGoDigest(t, tree, lang)
	cDigest, err := COracleDeepDigest(ct)
	if err != nil {
		t.Fatal(err)
	}
	queryText := entry.HighlightQuery
	q, err := gts.NewQuery(queryText, lang)
	if err != nil {
		t.Fatal(err)
	}
	cq, queryErr := sitter.NewQuery(cl, queryText)
	if queryErr != nil {
		t.Fatal(queryErr)
	}
	defer cq.Close()
	for rep := 0; rep < 3; rep++ {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		start := time.Now()
		matches := q.Execute(tree)
		goNanos := time.Since(start).Nanoseconds()
		runtime.ReadMemStats(&after)
		goCaptures := 0
		for _, match := range matches {
			goCaptures += len(match.Captures)
		}
		cc := sitter.NewQueryCursor()
		start = time.Now()
		cm := cc.Matches(cq, ct.RootNode(), src)
		cMatches, cCaptures := 0, 0
		for m := cm.Next(); m != nil; m = cm.Next() {
			cMatches++
			cCaptures += len(m.Captures)
		}
		cNanos := time.Since(start).Nanoseconds()
		cc.Close()
		programAuditLog(t, "large_query", map[string]any{"rep": rep, "source_bytes": len(src), "source_sha256": fmt.Sprintf("%x", sha256.Sum256(src)), "query_sha256": fmt.Sprintf("%x", sha256.Sum256([]byte(queryText))), "digest_format": benchfixtures.DeepTreeDigestVersion, "go_digest": goDigest, "C_digest": cDigest, "tree_equals_C": goDigest == cDigest, "go_matches": len(matches), "go_captures": goCaptures, "C_matches": cMatches, "C_captures": cCaptures, "go_ns": goNanos, "C_binding_ns": cNanos, "go_heap_bytes": after.TotalAlloc - before.TotalAlloc, "go_heap_allocs": after.Mallocs - before.Mallocs})
	}
}
