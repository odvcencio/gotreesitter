package gotreesitter_test

import (
	"bytes"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// benchTagsQuery is a tags query suitable for Go source.
// It matches function/method definitions and call references.
const benchTagsQuery = `
(function_declaration (identifier) @name) @definition.function
(method_declaration (field_identifier) @name) @definition.method
(call_expression (identifier) @name) @reference.call
(call_expression (selector_expression (field_identifier) @name)) @reference.call
(type_declaration (type_spec (type_identifier) @name)) @definition.type
`

var taggerBenchSink int
var codeUnderstandingBenchSink int

// BenchmarkTaggerTag measures tagging a 500-function Go file from scratch.
func BenchmarkTaggerTag(b *testing.B) {
	entry := grammars.DetectLanguage("main.go")
	if entry == nil {
		b.Skip("Go grammar not available")
	}

	lang := entry.Language()
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))

	var opts []gotreesitter.TaggerOption
	if entry.TokenSourceFactory != nil {
		factory := entry.TokenSourceFactory
		opts = append(opts, gotreesitter.WithTaggerTokenSourceFactory(func(s []byte) gotreesitter.TokenSource {
			return factory(s, lang)
		}))
	}

	tagger, err := gotreesitter.NewTagger(lang, benchTagsQuery, opts...)
	if err != nil {
		b.Fatalf("NewTagger failed: %v", err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tags := tagger.Tag(src)
		if len(tags) == 0 {
			b.Fatal("tagger returned no tags")
		}
	}
}

// BenchmarkExtractCodeUnderstandingGo measures parsing followed by separate
// definition and call extraction passes.
func BenchmarkExtractCodeUnderstandingGo(b *testing.B) {
	entry := grammars.DetectLanguage("main.go")
	if entry == nil {
		b.Skip("Go grammar not available")
	}

	lang := entry.Language()
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))
	parser := gotreesitter.NewParser(lang)

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var (
			tree *gotreesitter.Tree
			err  error
		)
		if entry.TokenSourceFactory != nil {
			tree, err = parser.ParseWithTokenSource(src, entry.TokenSourceFactory(src, lang))
		} else {
			tree, err = parser.Parse(src)
		}
		if err != nil {
			b.Fatalf("parse failed: %v", err)
		}
		defs := gotreesitter.ExtractDefinitionSpans(tree)
		calls := gotreesitter.ExtractCalls(tree)
		if len(defs) == 0 {
			b.Fatalf("understanding helpers returned defs=%d calls=%d", len(defs), len(calls))
		}
		codeUnderstandingBenchSink += len(defs) + len(calls)
		tree.Release()
	}
}

// BenchmarkFactProgramGo measures parsing followed by one compiled fact pass.
func BenchmarkFactProgramGo(b *testing.B) {
	entry := grammars.DetectLanguage("main.go")
	if entry == nil {
		b.Skip("Go grammar not available")
	}

	lang := entry.Language()
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))
	parser := gotreesitter.NewParser(lang)
	program, err := gotreesitter.NewFactProgram(lang, gotreesitter.FactDefinitions|gotreesitter.FactCalls)
	if err != nil {
		b.Fatalf("NewFactProgram failed: %v", err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var tree *gotreesitter.Tree
		if entry.TokenSourceFactory != nil {
			tree, err = parser.ParseWithTokenSource(src, entry.TokenSourceFactory(src, lang))
		} else {
			tree, err = parser.Parse(src)
		}
		if err != nil {
			b.Fatalf("parse failed: %v", err)
		}
		facts := program.Extract(tree)
		if len(facts.Definitions) == 0 {
			b.Fatalf("FactProgram returned definitions=%d calls=%d", len(facts.Definitions), len(facts.Calls))
		}
		codeUnderstandingBenchSink += len(facts.Definitions) + len(facts.Calls)
		tree.Release()
	}
}

// BenchmarkTaggerTagTreeGo measures tags-query execution over an existing tree.
func BenchmarkTaggerTagTreeGo(b *testing.B) {
	entry := grammars.DetectLanguage("main.go")
	if entry == nil {
		b.Skip("Go grammar not available")
	}

	lang := entry.Language()
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))
	tree := parseBenchmarkTree(b, entry, lang, src)
	defer tree.Release()

	tagger, err := gotreesitter.NewTagger(lang, benchTagsQuery)
	if err != nil {
		b.Fatalf("NewTagger failed: %v", err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tags := tagger.TagTree(tree)
		if len(tags) == 0 {
			b.Fatal("tagger returned no tags")
		}
		taggerBenchSink += len(tags)
	}
}

// BenchmarkExtractCodeUnderstandingTreeGo measures separate extraction passes
// over an existing tree.
func BenchmarkExtractCodeUnderstandingTreeGo(b *testing.B) {
	entry := grammars.DetectLanguage("main.go")
	if entry == nil {
		b.Skip("Go grammar not available")
	}

	lang := entry.Language()
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))
	tree := parseBenchmarkTree(b, entry, lang, src)
	defer tree.Release()

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		defs := gotreesitter.ExtractDefinitionSpans(tree)
		calls := gotreesitter.ExtractCalls(tree)
		if len(defs) == 0 {
			b.Fatalf("understanding helpers returned defs=%d calls=%d", len(defs), len(calls))
		}
		codeUnderstandingBenchSink += len(defs) + len(calls)
	}
}

// BenchmarkFactProgramTreeGo measures one compiled fact pass over an existing
// tree. It isolates extraction cost from parser cost.
func BenchmarkFactProgramTreeGo(b *testing.B) {
	entry := grammars.DetectLanguage("main.go")
	if entry == nil {
		b.Skip("Go grammar not available")
	}

	lang := entry.Language()
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))
	tree := parseBenchmarkTree(b, entry, lang, src)
	defer tree.Release()
	program, err := gotreesitter.NewFactProgram(lang, gotreesitter.FactDefinitions|gotreesitter.FactCalls)
	if err != nil {
		b.Fatalf("NewFactProgram failed: %v", err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		facts := program.Extract(tree)
		if len(facts.Definitions) == 0 {
			b.Fatalf("FactProgram returned definitions=%d calls=%d", len(facts.Definitions), len(facts.Calls))
		}
		codeUnderstandingBenchSink += len(facts.Definitions) + len(facts.Calls)
	}
}

// BenchmarkExtractAllFactsTreeGo measures four separate extraction passes over
// an existing tree.
func BenchmarkExtractAllFactsTreeGo(b *testing.B) {
	entry := grammars.DetectLanguage("main.go")
	if entry == nil {
		b.Skip("Go grammar not available")
	}

	lang := entry.Language()
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))
	tree := parseBenchmarkTree(b, entry, lang, src)
	defer tree.Release()

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		defs := gotreesitter.ExtractDefinitionSpans(tree)
		calls := gotreesitter.ExtractCalls(tree)
		heritage := gotreesitter.ExtractHeritage(tree)
		imports := gotreesitter.ExtractImports(tree)
		if len(defs) == 0 || len(imports) == 0 {
			b.Fatalf("fact extractors returned definitions=%d calls=%d heritage=%d imports=%d", len(defs), len(calls), len(heritage), len(imports))
		}
		codeUnderstandingBenchSink += len(defs) + len(calls) + len(heritage) + len(imports)
	}
}

// BenchmarkFactProgramAllTreeGo measures one compiled pass for every fact kind
// over an existing tree.
func BenchmarkFactProgramAllTreeGo(b *testing.B) {
	benchmarkFactProgramAllTreeGo(b, false)
}

// BenchmarkFactProgramAllTreeGoReuse measures extraction with retained result storage.
func BenchmarkFactProgramAllTreeGoReuse(b *testing.B) {
	benchmarkFactProgramAllTreeGo(b, true)
}

func benchmarkFactProgramAllTreeGo(b *testing.B, reuse bool) {
	entry := grammars.DetectLanguage("main.go")
	if entry == nil {
		b.Skip("Go grammar not available")
	}

	lang := entry.Language()
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))
	tree := parseBenchmarkTree(b, entry, lang, src)
	defer tree.Release()
	program, err := gotreesitter.NewFactProgram(lang, gotreesitter.FactAll)
	if err != nil {
		b.Fatalf("NewFactProgram failed: %v", err)
	}

	var facts gotreesitter.FactSet
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if reuse {
			program.ExtractInto(tree, &facts)
		} else {
			facts = program.Extract(tree)
		}
		if len(facts.Definitions) == 0 || len(facts.Imports) == 0 {
			b.Fatalf("FactProgram returned definitions=%d calls=%d heritage=%d imports=%d", len(facts.Definitions), len(facts.Calls), len(facts.Heritage), len(facts.Imports))
		}
		codeUnderstandingBenchSink += len(facts.Definitions) + len(facts.Calls) + len(facts.Heritage) + len(facts.Imports)
	}
}

func parseBenchmarkTree(b *testing.B, entry *grammars.LangEntry, lang *gotreesitter.Language, src []byte) *gotreesitter.Tree {
	b.Helper()
	parser := gotreesitter.NewParser(lang)
	if entry.TokenSourceFactory != nil {
		tree, err := parser.ParseWithTokenSource(src, entry.TokenSourceFactory(src, lang))
		if err != nil {
			b.Fatalf("parse failed: %v", err)
		}
		return tree
	}
	tree, err := parser.Parse(src)
	if err != nil {
		b.Fatalf("parse failed: %v", err)
	}
	return tree
}

// BenchmarkTaggerTagIncremental measures re-tagging after a single-byte edit.
func BenchmarkTaggerTagIncremental(b *testing.B) {
	entry := grammars.DetectLanguage("main.go")
	if entry == nil {
		b.Skip("Go grammar not available")
	}

	lang := entry.Language()
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))

	// Locate the edit site: "v := 0" -> toggle the '0'.
	editAt := bytes.Index(src, []byte("v := 0"))
	if editAt < 0 {
		b.Fatal("could not find edit marker")
	}
	editAt += len("v := ")
	start := pointAtOffset(src, editAt)
	end := pointAtOffset(src, editAt+1)

	// Build the initial tree.
	parser := gotreesitter.NewParser(lang)
	ts := mustGoTokenSource(b, src, lang)
	tree, err := parser.ParseWithTokenSource(src, ts)
	if err != nil {
		b.Fatalf("initial parse failed: %v", err)
	}
	if tree.RootNode() == nil {
		b.Fatal("initial parse returned nil root")
	}

	var opts []gotreesitter.TaggerOption
	if entry.TokenSourceFactory != nil {
		factory := entry.TokenSourceFactory
		opts = append(opts, gotreesitter.WithTaggerTokenSourceFactory(func(s []byte) gotreesitter.TokenSource {
			return factory(s, lang)
		}))
	}

	tagger, err := gotreesitter.NewTagger(lang, benchTagsQuery, opts...)
	if err != nil {
		b.Fatalf("NewTagger failed: %v", err)
	}

	edit := gotreesitter.InputEdit{
		StartByte:   uint32(editAt),
		OldEndByte:  uint32(editAt + 1),
		NewEndByte:  uint32(editAt + 1),
		StartPoint:  start,
		OldEndPoint: end,
		NewEndPoint: end,
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Toggle one ASCII digit in place.
		if src[editAt] == '0' {
			src[editAt] = '1'
		} else {
			src[editAt] = '0'
		}

		tree.Edit(edit)
		tags, newTree := tagger.TagIncremental(src, tree)
		if len(tags) == 0 {
			b.Fatal("incremental tagger returned no tags")
		}
		if newTree != tree {
			tree.Release()
		}
		tree = newTree
	}
	tree.Release()
}

// These benchmarks report opt-in projection costs separately from the four
// legacy FactProgram benchmarks. They use the same 500-function source.
func BenchmarkFactProgramSignaturesTreeGo(b *testing.B) {
	benchmarkRichFactsGo(b, gotreesitter.FactSignatures, false)
}
func BenchmarkFactProgramSignaturesTreeGoReuse(b *testing.B) {
	benchmarkRichFactsGo(b, gotreesitter.FactSignatures, true)
}
func BenchmarkFactProgramCallArgumentsTreeGo(b *testing.B) {
	benchmarkRichFactsGo(b, gotreesitter.FactCallArguments, false)
}
func BenchmarkFactProgramCallArgumentsTreeGoReuse(b *testing.B) {
	benchmarkRichFactsGo(b, gotreesitter.FactCallArguments, true)
}
func BenchmarkFactProgramRichTreeGo(b *testing.B) {
	benchmarkRichFactsGo(b, gotreesitter.FactAll|gotreesitter.FactDeclarations|gotreesitter.FactSignatures|gotreesitter.FactCallArguments, false)
}
func BenchmarkFactProgramRichTreeGoReuse(b *testing.B) {
	benchmarkRichFactsGo(b, gotreesitter.FactAll|gotreesitter.FactDeclarations|gotreesitter.FactSignatures|gotreesitter.FactCallArguments, true)
}

func benchmarkRichFactsGo(b *testing.B, kinds gotreesitter.FactKind, reuse bool) {
	entry := grammars.DetectLanguage("main.go")
	if entry == nil {
		b.Skip("Go grammar not available")
	}
	src := makeGoBenchmarkSource(benchmarkFuncCount(b))
	// The legacy fixture has no calls. Add one representative mixed call for
	// each function when measuring arguments, while keeping signatures identical.
	if kinds&gotreesitter.FactCallArguments != 0 {
		src = bytes.ReplaceAll(src, []byte("; return v"), []byte("; consume(v, make([]int, 3), values...); return v"))
	}
	tree := parseBenchmarkTree(b, entry, entry.Language(), src)
	defer tree.Release()
	if root := tree.RootNode(); root == nil || root.HasError() || root.EndByte() != uint32(len(src)) {
		b.Fatal("incomplete benchmark tree")
	}

	program, err := gotreesitter.NewFactProgramWithOptions(entry.Language(), kinds,
		gotreesitter.WithDeclarationRules(grammars.DeclarationRules(*entry)),
		gotreesitter.WithSignatureRules(grammars.SignatureRules(*entry)),
		gotreesitter.WithCallArgumentRules(grammars.CallArgumentRules(*entry)))
	if err != nil {
		b.Fatal(err)
	}
	var facts gotreesitter.FactSet
	// Warm lazy wrappers and destination capacity before measuring.
	program.ExtractInto(tree, &facts)
	count := benchmarkFuncCount(b)
	if kinds&gotreesitter.FactSignatures != 0 && len(facts.Signatures) != count {
		b.Fatalf("signatures=%d, want %d", len(facts.Signatures), count)
	}
	if kinds&gotreesitter.FactCallArguments != 0 && len(facts.CallArguments) != 5*count {
		b.Fatalf("arguments=%d, want %d", len(facts.CallArguments), 5*count)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if reuse {
			program.ExtractInto(tree, &facts)
		} else {
			facts = program.Extract(tree)
		}
		if kinds&gotreesitter.FactSignatures != 0 && len(facts.Signatures) == 0 {
			b.Fatal("no signatures")
		}
		if kinds&gotreesitter.FactCallArguments != 0 && len(facts.CallArguments) == 0 {
			b.Fatal("no arguments")
		}
		codeUnderstandingBenchSink += len(facts.Signatures) + len(facts.CallArguments) + len(facts.Declarations)
	}
}
