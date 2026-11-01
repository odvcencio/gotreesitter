package gotreesitter_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

type familyBenchmarkSpec struct {
	family   string
	language string
	lang     func() *gotreesitter.Language
	source   func(int) []byte
}

var familyParseBenchmarkSpecs = []familyBenchmarkSpec{
	{family: "apple", language: "swift", lang: grammars.SwiftLanguage, source: makeSwiftBenchmarkSource},
	{family: "c-family", language: "c", lang: grammars.CLanguage, source: makeCBenchmarkSource},
	{family: "data", language: "json", lang: grammars.JsonLanguage, source: makeJSONBenchmarkSource},
	{family: "dotnet", language: "csharp", lang: grammars.CSharpLanguage, source: makeCSharpBenchmarkSource},
	{family: "jvm", language: "java", lang: grammars.JavaLanguage, source: makeJavaBenchmarkSource},
	{family: "jvm", language: "kotlin", lang: grammars.KotlinLanguage, source: makeKotlinBenchmarkSource},
	{family: "markup", language: "html", lang: grammars.HtmlLanguage, source: makeHTMLBenchmarkSource},
	{family: "scientific", language: "fortran", lang: grammars.FortranLanguage, source: makeFortranBenchmarkSource},
	{family: "scripting", language: "bash", lang: grammars.BashLanguage, source: makeBashBenchmarkSource},
	{family: "scripting", language: "python", lang: grammars.PythonLanguage, source: makePythonBenchmarkSource},
	{family: "scripting", language: "ruby", lang: grammars.RubyLanguage, source: makeRubyBenchmarkSource},
	{family: "styles", language: "css", lang: grammars.CssLanguage, source: makeCSSBenchmarkSource},
	{family: "systems", language: "go", lang: grammars.GoLanguage, source: makeGoBenchmarkSource},
	{family: "systems", language: "rust", lang: grammars.RustLanguage, source: makeRustBenchmarkSource},
	{family: "web", language: "javascript", lang: grammars.JavascriptLanguage, source: makeJavaScriptBenchmarkSource},
	{family: "web", language: "typescript", lang: grammars.TypescriptLanguage, source: makeTypeScriptBenchmarkSource},
	{family: "web", language: "tsx", lang: grammars.TsxLanguage, source: makeTSXBenchmarkSource},
}

func BenchmarkFamilyParseFullDFA(b *testing.B) {
	unitCount := familyBenchmarkUnitCount(b)
	for _, spec := range familyParseBenchmarkSpecs {
		spec := spec
		b.Run(spec.family+"/"+spec.language, func(b *testing.B) {
			benchmarkParseFullDFA(b, dfaBenchmarkSpec{
				name:            spec.language,
				lang:            spec.lang,
				source:          spec.source,
				funcCount:       unitCount,
				requireNoErrors: true,
				warmupBeforeRun: true,
			})
		})
	}
}

func familyBenchmarkUnitCount(b *testing.B) int {
	if strings.TrimSpace(os.Getenv("GOT_BENCH_FUNC_COUNT")) != "" {
		return benchmarkFuncCount(b)
	}
	if testing.Short() {
		return 10
	}
	return 25
}

func makeJavaScriptBenchmarkSource(funcCount int) []byte {
	var sb strings.Builder
	sb.Grow(funcCount * 80)
	for i := 0; i < funcCount; i++ {
		fmt.Fprintf(&sb, "export function f%d() { const v = %d; return v }\n", i, i)
	}
	return []byte(sb.String())
}

func makeTSXBenchmarkSource(componentCount int) []byte {
	var sb strings.Builder
	sb.Grow(componentCount * 150)
	sb.WriteString("import React from 'react'\n\n")
	for i := 0; i < componentCount; i++ {
		fmt.Fprintf(&sb, "export function C%d() { const v = %d; return <section data-id={v}><span>{v}</span></section> }\n", i, i)
	}
	return []byte(sb.String())
}

func makeJSONBenchmarkSource(itemCount int) []byte {
	var sb strings.Builder
	sb.Grow(itemCount * 96)
	sb.WriteString("{\"items\":[")
	for i := 0; i < itemCount; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, "{\"id\":%d,\"name\":\"item-%d\",\"enabled\":%t,\"tags\":[\"alpha\",\"beta\"]}", i, i, i%2 == 0)
	}
	sb.WriteString("]}\n")
	return []byte(sb.String())
}

func makeCSSBenchmarkSource(ruleCount int) []byte {
	var sb strings.Builder
	sb.Grow(ruleCount * 140)
	for i := 0; i < ruleCount; i++ {
		fmt.Fprintf(&sb, ".item-%d { color: #%06x; margin: %dpx; padding: 4px; }\n", i, (uint32(i)*uint32(2654435761))&0xffffff, i%24)
		fmt.Fprintf(&sb, "@media (min-width: 640px) { .item-%d:hover { transform: translateX(%dpx); } }\n", i, i%16)
	}
	return []byte(sb.String())
}

func makeHTMLBenchmarkSource(sectionCount int) []byte {
	var sb strings.Builder
	sb.Grow(sectionCount * 140)
	sb.WriteString("<!doctype html><html><body>\n")
	for i := 0; i < sectionCount; i++ {
		fmt.Fprintf(&sb, "<section id=\"item-%d\"><h2>Item %d</h2><p data-id=\"%d\">Hello <span>world</span></p></section>\n", i, i, i)
	}
	sb.WriteString("</body></html>\n")
	return []byte(sb.String())
}

func makeBashBenchmarkSource(funcCount int) []byte {
	var sb strings.Builder
	sb.Grow(funcCount * 70)
	sb.WriteString("#!/usr/bin/env bash\nset -euo pipefail\n")
	for i := 0; i < funcCount; i++ {
		fmt.Fprintf(&sb, "f%d() { local v=%d; echo \"$v\"; }\n", i, i)
	}
	return []byte(sb.String())
}

func makeCBenchmarkSource(funcCount int) []byte {
	var sb strings.Builder
	sb.Grow(funcCount * 64)
	sb.WriteString("#include <stdio.h>\n")
	for i := 0; i < funcCount; i++ {
		fmt.Fprintf(&sb, "int f%d(void) { int v = %d; return v; }\n", i, i)
	}
	return []byte(sb.String())
}

func makeCSharpBenchmarkSource(classCount int) []byte {
	var sb strings.Builder
	sb.Grow(classCount * 115)
	sb.WriteString("namespace Bench {\n")
	for i := 0; i < classCount; i++ {
		fmt.Fprintf(&sb, "public static class C%d { public static int F%d() { var v = %d; return v; } }\n", i, i, i)
	}
	sb.WriteString("}\n")
	return []byte(sb.String())
}

func makeJavaBenchmarkSource(classCount int) []byte {
	var sb strings.Builder
	sb.Grow(classCount * 90)
	sb.WriteString("package bench;\n")
	for i := 0; i < classCount; i++ {
		fmt.Fprintf(&sb, "class C%d { int f%d() { int v = %d; return v; } }\n", i, i, i)
	}
	return []byte(sb.String())
}

func makeKotlinBenchmarkSource(funcCount int) []byte {
	var sb strings.Builder
	sb.Grow(funcCount * 55)
	sb.WriteString("fun main() {\n")
	for i := 0; i < funcCount; i++ {
		fmt.Fprintf(&sb, "    val x%d: Int? = %d\n    println(x%d)\n", i, i, i)
	}
	sb.WriteString("}\n")
	return []byte(sb.String())
}

func makeSwiftBenchmarkSource(funcCount int) []byte {
	var sb strings.Builder
	sb.Grow(funcCount * 58)
	for i := 0; i < funcCount; i++ {
		fmt.Fprintf(&sb, "func f%d() -> Int {\n    let v = %d\n    return v\n}\n\n", i, i)
	}
	return []byte(sb.String())
}

func makeFortranBenchmarkSource(funcCount int) []byte {
	var sb strings.Builder
	sb.Grow(funcCount * 90)
	sb.WriteString("program bench\ncontains\n")
	for i := 0; i < funcCount; i++ {
		fmt.Fprintf(&sb, "  integer function f%d()\n    integer :: v\n    v = %d\n    f%d = v\n  end function f%d\n", i, i, i, i)
	}
	sb.WriteString("end program bench\n")
	return []byte(sb.String())
}

func makeRubyBenchmarkSource(funcCount int) []byte {
	var sb strings.Builder
	sb.Grow(funcCount * 35)
	for i := 0; i < funcCount; i++ {
		fmt.Fprintf(&sb, "def f%d\n  v = %d\n  v\nend\n\n", i, i)
	}
	return []byte(sb.String())
}

func makeRustBenchmarkSource(funcCount int) []byte {
	var sb strings.Builder
	sb.Grow(funcCount * 55)
	for i := 0; i < funcCount; i++ {
		fmt.Fprintf(&sb, "pub fn f%d() -> i32 { let v = %d; v }\n", i, i)
	}
	return []byte(sb.String())
}

// The packed-lane benchmarks parse three synthetic inputs of about 137 KB each.
// cWaveReduceWindowsFromGSSInto serves the reduces that pop packed GSS links.
// Only the languages with a certified packed version order run it in an
// ordinary parse; this file measures three of them. Of these inputs, the Swift
// and C++ (Parser.Parse) ones reach it, and the TypeScript one does not: its
// allocation counts are the same before and after the scratch. Each input is a
// header followed by one block per index; "@I@" in a block stands for the index.
// The digests pin the exact bytes, so a counter or timing taken on one build can
// be compared with another.

func packedLaneLines(lines ...string) string {
	return strings.Join(lines, "\n") + "\n"
}

type packedLaneParseSpec struct {
	name     string
	language string // registry name
	registry bool   // parse through the registry's token source factory
	header   string
	block    string
	blocks   int
	sha256   string
}

func (s packedLaneParseSpec) source() []byte {
	var sb strings.Builder
	sb.WriteString(s.header)
	for i := 1; i <= s.blocks; i++ {
		sb.WriteString(strings.ReplaceAll(s.block, "@I@", strconv.Itoa(i)))
	}
	return []byte(sb.String())
}

var packedLaneParseSpecs = []packedLaneParseSpec{
	{name: "swift", language: "swift", header: packedLaneSwiftHeader, block: packedLaneSwiftBlock, blocks: 91, sha256: "91d1539ebf0741c3581e7d30f7913102023dc4abaf7b3a2daaa8ff9a8928f523"},
	{name: "cpp", language: "cpp", header: packedLaneCppHeader, block: packedLaneCppBlock, blocks: 101, sha256: "9522a6fb51824db52f958762b7fe6d9a0588e3ed9366bc049c737687a537d78c"},
	{name: "typescript", language: "typescript", header: packedLaneTypeScriptHeader, block: packedLaneTypeScriptBlock, blocks: 83, sha256: "3bf827d03befdfcf6276a47269eae5ba6baf9ee5230b448c4085ebf7917ae428"},
	// The registry route lexes C++ with its own token source and builds a
	// different tree. This input does not reach the scratch path on that route
	// (its allocation counts are the same before and after the scratch); inputs
	// with syntax errors do.
	{name: "cpp_registry", language: "cpp", registry: true, header: packedLaneCppHeader, block: packedLaneCppBlock, blocks: 101, sha256: "9522a6fb51824db52f958762b7fe6d9a0588e3ed9366bc049c737687a537d78c"},
}

func TestPackedLaneParseSourcesMatchTheirDigests(t *testing.T) {
	for _, spec := range packedLaneParseSpecs {
		sum := sha256.Sum256(spec.source())
		if got := hex.EncodeToString(sum[:]); got != spec.sha256 {
			t.Errorf("%s source SHA-256 = %s, want %s", spec.name, got, spec.sha256)
		}
	}
}

// BenchmarkPackedLaneParseFull parses each input from scratch on a warm parser.
// Besides the standard metrics it reports the multi-stack GSS node count, a
// deterministic work counter that must not change between builds.
func BenchmarkPackedLaneParseFull(b *testing.B) {
	specs := append([]packedLaneParseSpec(nil), packedLaneParseSpecs...)
	// scripts/run_randomized_benchmarks.sh sets GTS_BENCHMARK_SEED. Both builds of a
	// pair shuffle the same way, so the order differs between seeds only.
	if seed, err := strconv.ParseInt(os.Getenv("GTS_BENCHMARK_SEED"), 10, 64); err == nil {
		rand.New(rand.NewSource(seed)).Shuffle(len(specs), func(i, j int) { specs[i], specs[j] = specs[j], specs[i] })
	}
	for _, spec := range specs {
		spec := spec
		b.Run(spec.name, func(b *testing.B) {
			entry := grammars.DetectLanguageByName(spec.language)
			if entry == nil {
				b.Fatalf("no language %q", spec.language)
			}
			lang := entry.Language()
			parser := gotreesitter.NewParser(lang)
			src := spec.source()
			parse := func() *gotreesitter.Tree {
				var tree *gotreesitter.Tree
				var err error
				if spec.registry {
					tree, err = parser.ParseWithTokenSource(src, entry.TokenSourceFactory(src, lang))
				} else {
					tree, err = parser.Parse(src)
				}
				if err != nil || tree == nil || tree.RootNode() == nil || tree.RootNode().EndByte() != uint32(len(src)) {
					b.Fatalf("%s parse failed or stopped early: err=%v", spec.name, err)
				}
				return tree
			}
			parse().Release() // warm the pooled scratch
			b.ReportAllocs()
			b.SetBytes(int64(len(src)))
			b.ResetTimer()
			var rt gotreesitter.ParseRuntime
			for i := 0; i < b.N; i++ {
				tree := parse()
				rt = tree.ParseRuntime()
				tree.Release()
			}
			b.StopTimer()
			b.ReportMetric(float64(rt.MultiStackGSSNodes), "multistack_gss_nodes")
			b.ReportMetric(float64(rt.SingleStackGSSNodes), "singlestack_gss_nodes")
		})
	}
}

// packedLaneSwiftHeader and packedLaneSwiftBlock build a 138276-byte input (SHA-256 91d1539ebf0741c3581e7d30f7913102023dc4abaf7b3a2daaa8ff9a8928f523).
var packedLaneSwiftHeader = packedLaneLines(
	"// synthetic swift fixture",
	"import Foundation",
	"",
)

var packedLaneSwiftBlock = packedLaneLines(
	"protocol Shape@I@ {",
	"    var area: Double { get }",
	"    func describe() -> String",
	"}",
	"",
	"struct Rect@I@: Shape@I@, Equatable {",
	"    var width: Double",
	"    var height: Double",
	"    var area: Double { width * height }",
	"    func describe() -> String { \"Rect \\(width)x\\(height)\" }",
	"}",
	"",
	"enum Outcome@I@<T> {",
	"    case success(T)",
	"    case failure(Error)",
	"}",
	"",
	"final class Store@I@<Element: Hashable> {",
	"    private var items: [Element: Int] = [:]",
	"    var onChange: ((Element) -> Void)?",
	"",
	"    init(seed: [Element] = []) {",
	"        for (index, element) in seed.enumerated() {",
	"            items[element] = index",
	"        }",
	"    }",
	"",
	"    func insert(_ element: Element) -> Bool {",
	"        guard items[element] == nil else { return false }",
	"        items[element] = items.count",
	"        onChange?(element)",
	"        return true",
	"    }",
	"",
	"    func total() -> Int {",
	"        var sum = 0",
	"        for (_, value) in items where value > @I@ {",
	"            sum += value",
	"        }",
	"        return sum",
	"    }",
	"}",
	"",
	"extension Store@I@ where Element == String {",
	"    func joined() -> String {",
	"        items.keys.sorted().map { $0.uppercased() }.joined(separator: \", \")",
	"    }",
	"}",
	"",
	"func process@I@(_ values: [Int]) throws -> Outcome@I@<Int> {",
	"    let filtered = values.filter { $0 % 2 == 0 }",
	"    if let first = filtered.first, first > 0 {",
	"        return .success(filtered.reduce(0, +))",
	"    }",
	"    switch values.count {",
	"    case 0: throw NSError(domain: \"empty\", code: @I@)",
	"    case 1...5: return .success(values[0])",
	"    default: return .failure(NSError(domain: \"big\", code: values.count))",
	"    }",
	"}",
	"",
)

// packedLaneCppHeader and packedLaneCppBlock build a 137270-byte input (SHA-256 9522a6fb51824db52f958762b7fe6d9a0588e3ed9366bc049c737687a537d78c).
var packedLaneCppHeader = packedLaneLines(
	"// synthetic c++ fixture",
	"#include <algorithm>",
	"#include <map>",
	"#include <memory>",
	"#include <string>",
	"#include <vector>",
	"",
)

var packedLaneCppBlock = packedLaneLines(
	"namespace app@I@ {",
	"",
	"template <typename T, std::size_t N = @I@ + 4>",
	"class Buffer@I@ {",
	" public:",
	"  using value_type = T;",
	"  explicit Buffer@I@(std::size_t cap = N) : data_(cap), size_(0) {}",
	"  Buffer@I@(const Buffer@I@&) = default;",
	"  Buffer@I@& operator=(Buffer@I@&&) noexcept = default;",
	"  virtual ~Buffer@I@() = default;",
	"",
	"  void push(const T& v) {",
	"    if (size_ == data_.size()) {",
	"      data_.resize(data_.size() * 2 + 1);",
	"    }",
	"    data_[size_++] = v;",
	"  }",
	"",
	"  template <typename F>",
	"  std::vector<T> map(F&& f) const {",
	"    std::vector<T> out;",
	"    out.reserve(size_);",
	"    for (std::size_t k = 0; k < size_; ++k) {",
	"      out.push_back(f(data_[k]));",
	"    }",
	"    return out;",
	"  }",
	"",
	" private:",
	"  std::vector<T> data_;",
	"  std::size_t size_;",
	"};",
	"",
	"struct Point@I@ {",
	"  double x = 0.0, y = 0.0;",
	"  constexpr Point@I@ operator+(const Point@I@& o) const { return {x + o.x, y + o.y}; }",
	"};",
	"",
	"static int compute@I@(const std::map<std::string, int>& m, int limit) {",
	"  int total = 0;",
	"  for (const auto& [key, value] : m) {",
	"    if (value > limit && !key.empty()) {",
	"      total += value * @I@;",
	"    } else if (key == \"skip\") {",
	"      continue;",
	"    } else {",
	"      total -= 1;",
	"    }",
	"  }",
	"  auto fn = [&total, limit](int v) mutable { total += v; return total > limit; };",
	"  switch (total % 3) {",
	"    case 0: fn(1); break;",
	"    case 1: fn(2); break;",
	"    default: break;",
	"  }",
	"  return total;",
	"}",
	"",
	"}  // namespace app@I@",
	"",
)

// packedLaneTypeScriptHeader and packedLaneTypeScriptBlock build a 137032-byte input (SHA-256 3bf827d03befdfcf6276a47269eae5ba6baf9ee5230b448c4085ebf7917ae428).
var packedLaneTypeScriptHeader = packedLaneLines(
	"// synthetic typescript fixture",
	"",
)

var packedLaneTypeScriptBlock = packedLaneLines(
	"export interface Item@I@ {",
	"  id: number;",
	"  name: string;",
	"  tags?: string[];",
	"  meta: Record<string, unknown>;",
	"}",
	"",
	"export type Handler@I@<T extends Item@I@ = Item@I@> = (item: T, index: number) => Promise<void> | void;",
	"",
	"export enum Status@I@ {",
	"  Active = \"active\",",
	"  Inactive = \"inactive\",",
	"  Pending = 3,",
	"}",
	"",
	"export class Service@I@<T extends Item@I@> implements Iterable<T> {",
	"  private readonly items: Map<number, T> = new Map();",
	"  static count: number = @I@;",
	"",
	"  constructor(private readonly name: string, protected options: { retries?: number; verbose: boolean } = { verbose: false }) {}",
	"",
	"  add(item: T): this {",
	"    if (this.items.has(item.id)) {",
	"      throw new Error(`duplicate item ${item.id} in ${this.name}`);",
	"    }",
	"    this.items.set(item.id, item);",
	"    return this;",
	"  }",
	"",
	"  async run(handler: Handler@I@<T>): Promise<number> {",
	"    let processed = 0;",
	"    for (const [key, value] of this.items) {",
	"      try {",
	"        await handler(value, key);",
	"        processed += 1;",
	"      } catch (err: unknown) {",
	"        if (err instanceof Error && this.options.verbose) {",
	"          console.error(err.message);",
	"        }",
	"      } finally {",
	"        this.items.delete(key);",
	"      }",
	"    }",
	"    return processed;",
	"  }",
	"",
	"  *[Symbol.iterator](): Iterator<T> {",
	"    yield* this.items.values();",
	"  }",
	"}",
	"",
	"export function transform@I@<A, B>(input: A[], fn: (a: A) => B): B[] {",
	"  const out: B[] = [];",
	"  for (let n = 0; n < input.length; n++) {",
	"    out.push(fn(input[n]!));",
	"  }",
	"  return out.filter((x): x is NonNullable<B> => x != null);",
	"}",
	"",
	"export const config@I@ = {",
	"  name: \"service-@I@\",",
	"  limits: [1, 2, 3].map((v) => v * @I@),",
	"  enabled: true as const,",
	"} satisfies Record<string, unknown>;",
	"",
)
