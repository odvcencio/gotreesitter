//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"runtime"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/cgo_harness/internal/queryalloc"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestQueryAllocationReceipt records complete-operation Go allocations and
// native C allocation requests separately from randomized timing. Parsing,
// compilation, and warming immutable node views are outside each measurement.
func TestQueryAllocationReceipt(t *testing.T) {
	for _, fixture := range []struct {
		name, query       string
		comments, matches int
	}{
		{"failed32", `(source_file (comment)+ @comment . (type_declaration) @type)`, 32, 0},
		{"success4096", `(source_file (comment)+ @comment)`, 4096, 1},
		{"root4096", `(comment)+ @comment`, 4096, 1},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			source := []byte("package audit\n" + strings.Repeat("// audit\n", fixture.comments) + "func F() {}\n")
			tree, lang, err := parseWithGo(parityCase{name: "go", source: string(source)}, source, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			q, err := gts.NewQuery(fixture.query, lang)
			if err != nil {
				t.Fatal(err)
			}
			cLang, err := ParityCLanguage("go")
			if err != nil {
				t.Fatal(err)
			}
			parser := sitter.NewParser()
			defer parser.Close()
			if err := parser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			cTree := parser.Parse(source, nil)
			defer cTree.Close()
			cQuery, queryErr := sitter.NewQuery(cLang, fixture.query)
			if queryErr != nil {
				t.Fatal(queryErr)
			}
			defer cQuery.Close()
			root := cTree.RootNode()
			runGo := func() {
				matches, status := q.ExecuteIntoWithStatus(tree, nil)
				if len(matches) != fixture.matches || status != gts.QueryComplete {
					t.Fatal("incomplete Go operation")
				}
			}
			runC := func() {
				cursor := sitter.NewQueryCursor()
				iterator := cursor.Matches(cQuery, root, source)
				count := 0
				for iterator.Next() != nil {
					count++
				}
				cursor.Close()
				if count != fixture.matches {
					t.Fatal("incomplete C operation")
				}
			}
			runGo()
			runC()
			measure := func(run func()) (bytes, allocations uint64) {
				runtime.GC()
				var before, after runtime.MemStats
				runtime.ReadMemStats(&before)
				for i := 0; i < 100; i++ {
					run()
				}
				runtime.ReadMemStats(&after)
				return (after.TotalAlloc - before.TotalAlloc) / 100, (after.Mallocs - before.Mallocs) / 100
			}
			goBytes, goAllocations := measure(runGo)
			bindingBytes, bindingAllocations := measure(runC)
			queryalloc.Start()
			defer queryalloc.Stop()
			for i := 0; i < 100; i++ {
				runC()
			}
			stats := queryalloc.Stop()
			if stats.Allocations == 0 {
				t.Fatal("C runtime allocation hooks were not observed")
			}
			nativeCalls := uint64(fixture.matches + 1)
			nativeBytes := nativeCalls * queryalloc.MatchRecordBytes()
			t.Logf("Go bytes/op=%d allocs/op=%d; C binding Go bytes/op=%d allocs/op=%d; C runtime requested bytes/op=%d allocation-calls/op=%d; C binding native bytes/op=%d allocation-calls/op=%d; total C requested bytes/op=%d allocations/op=%d", goBytes, goAllocations, bindingBytes, bindingAllocations, stats.Bytes/100, stats.Allocations/100, nativeBytes, nativeCalls, stats.Bytes/100+nativeBytes+bindingBytes, stats.Allocations/100+nativeCalls+bindingAllocations)
		})
	}
}
