package gotreesitter_test

// Benchmarks for critical paths not covered by the existing benchmark suite.
// Targets: LoadLanguage, Query compilation, Node.SExpr, Rewriter, InjectionParser.

import (
	"os"
	"strconv"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// BenchmarkLoadLanguage measures grammar blob deserialization.
// This path runs once per language per process; hot in multi-tenant servers
// that spin up new language pools on demand.
func BenchmarkLoadLanguage(b *testing.B) {
	blob, err := os.ReadFile("grammars/grammar_blobs/go.bin") //nolint:gocritic
	if err != nil {
		b.Skipf("grammar blob not accessible: %v", err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(blob)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		loaded, err := gotreesitter.LoadLanguage(blob)
		if err != nil || loaded == nil {
			b.Fatalf("LoadLanguage: %v", err)
		}
	}
}

// BenchmarkQueryCompile measures NewQuery pattern compilation from scratch.
// The existing BenchmarkQueryExecCompiled pre-compiles once; this isolates
// the compilation cost (pattern parse + symbol resolution + DFA build).
func BenchmarkQueryCompile(b *testing.B) {
	lang := grammars.GoLanguage()
	pattern := `
(function_declaration name: (identifier) @name) @func
(method_declaration name: (field_identifier) @name) @method
(call_expression function: (identifier) @callee)
`
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q, err := gotreesitter.NewQuery(pattern, lang)
		if err != nil || q == nil {
			b.Fatalf("NewQuery: %v", err)
		}
	}
}

func BenchmarkQueryCompileLuaPattern(b *testing.B) {
	lang := grammars.GoLanguage()
	const pattern = `((identifier) @name (#lua-match? @name "^%a[%w_]*$"))`
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if q, err := gotreesitter.NewQuery(pattern, lang); err != nil || q == nil {
			b.Fatalf("NewQuery: %v", err)
		}
	}
}

// BenchmarkNodeSExpr measures S-expression generation from a parsed tree.
// This path is hit heavily by editor integrations that display parse trees
// and by tests that compare output against reference strings.
func BenchmarkNodeSExpr(b *testing.B) {
	lang := grammars.GoLanguage()
	parser := gotreesitter.NewParser(lang)
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))

	tree, err := parser.Parse(src)
	if err != nil {
		b.Fatalf("parse: %v", err)
	}
	b.Cleanup(func() { tree.Release() })

	root := tree.RootNode()
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		s := root.SExpr(lang)
		if s == "" {
			b.Fatal("SExpr returned empty string")
		}
	}
}

// BenchmarkRewriterApply measures Rewriter.Apply for a rename-across-file edit.
// Rewriter is used in all refactoring and code-generation workflows; its edit
// validation and byte-splicing loop are not covered by any existing benchmark.
func BenchmarkRewriterApply(b *testing.B) {
	lang := grammars.GoLanguage()
	parser := gotreesitter.NewParser(lang)
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))

	tree, err := parser.Parse(src)
	if err != nil {
		b.Fatalf("parse: %v", err)
	}
	b.Cleanup(func() { tree.Release() })

	// Pre-collect all identifier nodes so we don't include query overhead.
	q, err := gotreesitter.NewQuery(`(identifier) @id`, lang)
	if err != nil {
		b.Fatalf("NewQuery: %v", err)
	}
	matches := q.Execute(tree)
	if len(matches) == 0 {
		b.Fatal("no identifiers found")
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rw := gotreesitter.NewRewriter(src)
		// Rename every occurrence of "f0" -> "g0" to exercise the full rewrite path.
		for _, m := range matches {
			for _, cap := range m.Captures {
				n := cap.Node
				if string(src[n.StartByte():n.EndByte()]) == "f0" {
					rw.Replace(n, []byte("g0"))
				}
			}
		}
		newSrc, _, err := rw.Apply()
		if err != nil {
			b.Fatalf("Apply: %v", err)
		}
		if len(newSrc) == 0 {
			b.Fatal("Apply returned empty source")
		}
	}
}

// BenchmarkInjectionParseFull measures InjectionParser.Parse for a Markdown
// document with fenced code blocks. Injection parsing is used by editors that
// highlight embedded languages (markdown+Go, HTML+JS+CSS, Vue, Svelte).
func BenchmarkInjectionParseFull(b *testing.B) {
	mdEntry := grammars.DetectLanguage("README.md")
	if mdEntry == nil {
		b.Skip("Markdown grammar not available")
	}
	goEntry := grammars.DetectLanguage("main.go")
	if goEntry == nil {
		b.Skip("Go grammar not available")
	}

	ip := gotreesitter.NewInjectionParser()
	ip.RegisterLanguage("markdown", mdEntry.Language())
	ip.RegisterLanguage("go", goEntry.Language())

	// Use a markdown injection query that extracts fenced code blocks.
	const mdInjectionQuery = `
(fenced_code_block
  (info_string (language) @injection.language)
  (code_fence_content) @injection.content)
`
	if err := ip.RegisterInjectionQuery("markdown", mdInjectionQuery); err != nil {
		b.Skipf("RegisterInjectionQuery: %v", err)
	}

	src := makeMDWithGoBlocks(benchmarkFuncCount(b))

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result, err := ip.Parse(src, "markdown")
		if err != nil {
			b.Fatalf("InjectionParser.Parse: %v", err)
		}
		if result == nil {
			b.Fatal("Parse returned nil result")
		}
	}
}

// BenchmarkInjectionParseIncremental measures InjectionParser.ParseIncremental
// for the same Markdown+Go corpus after a single-byte edit.
func BenchmarkInjectionParseIncremental(b *testing.B) {
	mdEntry := grammars.DetectLanguage("README.md")
	if mdEntry == nil {
		b.Skip("Markdown grammar not available")
	}
	goEntry := grammars.DetectLanguage("main.go")
	if goEntry == nil {
		b.Skip("Go grammar not available")
	}

	ip := gotreesitter.NewInjectionParser()
	ip.RegisterLanguage("markdown", mdEntry.Language())
	ip.RegisterLanguage("go", goEntry.Language())

	const mdInjectionQuery = `
(fenced_code_block
  (info_string (language) @injection.language)
  (code_fence_content) @injection.content)
`
	if err := ip.RegisterInjectionQuery("markdown", mdInjectionQuery); err != nil {
		b.Skipf("RegisterInjectionQuery: %v", err)
	}

	src := makeMDWithGoBlocks(benchmarkFuncCount(b))

	// Warm-up parse to get the initial InjectionResult.
	first, err := ip.Parse(src, "markdown")
	if err != nil {
		b.Fatalf("initial parse: %v", err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	prev := first
	for i := 0; i < b.N; i++ {
		// InjectionParser.ParseIncremental re-parses from current src; the caller
		// is responsible for mutating src and marking edits on the inner trees
		// directly. Here we just re-parse with toggled source to exercise the path.
		result, err := ip.ParseIncremental(src, "markdown", prev)
		if err != nil {
			b.Fatalf("ParseIncremental: %v", err)
		}
		prev = result
	}
}

// makeMDWithGoBlocks builds a Markdown document with n fenced Go code blocks.
func makeMDWithGoBlocks(n int) []byte {
	var sb strings.Builder
	sb.WriteString("# Doc\n\n")
	for i := 0; i < n; i++ {
		sb.WriteString("```go\n")
		sb.WriteString("func f")
		sb.WriteString(itoa(i))
		sb.WriteString("() int { return ")
		sb.WriteString(itoa(i))
		sb.WriteString(" }\n")
		sb.WriteString("```\n\n")
	}
	return []byte(sb.String())
}

// BenchmarkParserPoolSerial measures checkout→parse→release in a single goroutine.
// This isolates pool overhead (sync.Pool Get/Put + applyDefaults) from parse time.
func BenchmarkParserPoolSerial(b *testing.B) {
	lang := grammars.GoLanguage()
	pool := gotreesitter.NewParserPool(lang)
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree, err := pool.Parse(src)
		if err != nil || tree == nil {
			b.Fatalf("ParserPool.Parse: %v", err)
		}
	}
}

// BenchmarkParserPoolConcurrentThroughput measures throughput under goroutine
// contention — the scenario that justifies pooling over per-request allocation.
// RunParallel drives GOMAXPROCS goroutines simultaneously; sync.Pool shines here
// because each OS thread maintains a per-P free list, minimising cross-core
// cache traffic on the Parser's reuse cursor and arena hint fields.
func BenchmarkParserPoolConcurrentThroughput(b *testing.B) {
	lang := grammars.GoLanguage()
	pool := gotreesitter.NewParserPool(lang)
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			tree, err := pool.Parse(src)
			if err != nil || tree == nil {
				b.Fatalf("ParserPool.Parse: %v", err)
			}
		}
	})
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
