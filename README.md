# gotreesitter

Pure-Go [tree-sitter](https://tree-sitter.github.io/) runtime. No CGo, no C toolchain. Cross-compiles to any `GOOS`/`GOARCH` target Go supports, including `wasip1`.

```sh
go get github.com/odvcencio/gotreesitter
```

gotreesitter loads the same parse-table format that tree-sitter's C runtime uses. Grammar tables are extracted from upstream `parser.c` files by `ts2go`, compressed into binary blobs, and deserialized on first use. 206 grammars ship in the registry.

## Agent Skill

Agents working with gotreesitter should use the [using-gotreesitter](https://github.com/odvcencio/m31labs-skills/blob/main/skills/using-gotreesitter/SKILL.md) skill.

## Motivation

Every Go tree-sitter binding in the ecosystem depends on CGo:

- Cross-compilation requires a C cross-toolchain per target. `GOOS=wasip1`, `GOARCH=arm64` from a Linux host, or any Windows build without MSYS2/MinGW, will not link.
- CI images must carry `gcc` and the grammar's C sources. `go install` fails for downstream users who don't have a C compiler.
- The Go race detector, coverage instrumentation, and fuzzer cannot see across the CGo boundary. Bugs in the C runtime or in FFI marshaling are invisible to `go test -race`.

gotreesitter eliminates the C dependency entirely. The parser, lexer, query engine, incremental reparsing, arena allocator, external scanners, and tree cursor are all implemented in Go. The only input is the grammar blob.

## Quick start

```go
import (
    "fmt"

    "github.com/odvcencio/gotreesitter"
    "github.com/odvcencio/gotreesitter/grammars"
)

func main() {
    src := []byte(`package main

func main() {}
`)

    lang := grammars.GoLanguage()
    parser := gotreesitter.NewParser(lang)

    tree, _ := parser.Parse(src)
    fmt.Println(tree.RootNode())
}
```

`grammars.DetectLanguage("main.go")` resolves a filename to the appropriate `LangEntry`.

### Browser and WebAssembly

The repository ships two `GOOS=js GOARCH=wasm` targets: a blob-loading runtime
and a grammargen build that imports Tree-sitter
grammar JSON and generates tables in the browser. The runtime exposes parsing,
queries, and highlighting; structured parse and query results include both
UTF-8 byte offsets and JavaScript UTF-16 code-unit offsets. It can also retain
an incrementally updated document tree and reuse it for highlights, tags, and
queries. `cmd/wasmassets` emits a reproducible, single-language browser bundle
for either the Go or TinyGo WebAssembly compiler.

See the [WebAssembly guide](wasm/README.md) for build commands, the complete
JavaScript APIs, node and match limits, and a browser example.

### Strict parsing and partial trees

The default parse methods preserve tree-sitter's partial-tree behavior: if a
timeout, cancellation flag, token-source EOF, or parser safety limit stops the
parse early, the returned tree records the stop reason and the error remains
nil. This is useful for editors and diagnostics, where a partial tree is still
valuable.

Use strict methods when partial output should fail a request:

```go
parser.SetTimeoutMicros(50_000)

tree, err := parser.ParseStrict(src)
if errors.Is(err, gotreesitter.ErrParseStoppedEarly) {
    fmt.Println(tree.ParseStopReason())
}
```

Strict variants are available for full parse, incremental parse, token-source
parse, factory parse, `ParseWith`, and `ParserPool`.

High-level analysis can use `WithHighlighterTimeoutMicros` or
`WithTaggerTimeoutMicros` to bound parser work. The
`HighlightIncrementalStrict` and `TagIncrementalStrict` methods return the
partial tree together with `ErrParseStoppedEarly` and do not run queries after
an early stop.

### Tree lookup helpers

Use `NodeAtByte` or `NamedNodeAtByte` when turning an editor byte offset into a
syntax node:

```go
node := tree.NamedNodeAtByte(offset)
```

The helpers return the smallest matching descendant and handle exact end-byte
boundaries without requiring callers to hand-roll a tree walk.

### Code-understanding helpers

For hot indexing paths that need common symbols but not arbitrary tags-query
semantics, use the one-pass extractors instead of running a fanout of queries:

```go
defs := gotreesitter.ExtractDefinitionSpans(tree)
calls := gotreesitter.ExtractCalls(tree)
bases := gotreesitter.ExtractHeritage(tree)

enclosing, ok := tree.EnclosingDefinition(offset)
```

These helpers cover common Go, JavaScript, TypeScript/TSX, Python, Starlark,
and Java definitions/calls, plus JavaScript/TypeScript, Python, and Java
heritage edges. Unsupported languages or ambiguous shapes are skipped
conservatively.

### Taproot DSL harness

The `taproot` package is a small front-end harness for grammargen-backed DSLs.
It caches generated or blob-loaded languages, parses source, returns a `Walker`
with common CST helpers, and reports syntax errors while still returning the
partial root for diagnostics:

```go
root, walker, err := taproot.ParseFromBlob("dsl", blob, buildGrammar, src)
if err != nil {
    fmt.Println(err)
}
fmt.Println(walker.Type(root))
```

Use `taproot.LanguageFromBlob` when a DSL embeds a generated grammar blob but
still wants a source-grammar fallback during development.

### Queries

```go
q, _ := gotreesitter.NewQuery(`(function_declaration name: (identifier) @fn)`, lang)
cursor := q.Exec(tree.RootNode(), lang, src)

for {
    match, ok := cursor.NextMatch()
    if !ok {
        break
    }
    for _, cap := range match.Captures {
        fmt.Println(cap.Node.Text(src))
    }
}
```

The query engine supports the full S-expression pattern language: structural quantifiers (`?`, `*`, `+`), alternation (`[...]`), field constraints, negated fields, anchor (`!`), and all standard predicates. See [Query API](#query-api).

### Typed query codegen

Generate type-safe Go wrappers from `.scm` query files:

```sh
go run ./cmd/tsquery -input queries/go_functions.scm -lang go -output go_functions_query.go -package queries
```

Given a query like `(function_declaration name: (identifier) @name body: (block) @body)`, `tsquery` generates:

```go
type FunctionDeclarationMatch struct {
    Name *gotreesitter.Node
    Body *gotreesitter.Node
}

q, _ := queries.NewGoFunctionsQuery(lang)
cursor := q.Exec(tree.RootNode(), lang, src)
for {
    match, ok := cursor.Next()
    if !ok { break }
    fmt.Println(match.Name.Text(src))
}
```

Multi-pattern queries generate one struct per pattern with `MatchPatternN` conversion helpers.

### Multi-language documents (injection parsing)

Parse documents with embedded languages (HTML+JS+CSS, Markdown+code fences, Vue/Svelte templates):

```go
ip := gotreesitter.NewInjectionParser()
ip.RegisterLanguage("html", htmlLang)
ip.RegisterLanguage("javascript", jsLang)
ip.RegisterLanguage("css", cssLang)
ip.RegisterInjectionQuery("html", injectionQuery)

result, _ := ip.Parse(source, "html")

for _, inj := range result.Injections {
    fmt.Printf("%s: %d ranges\n", inj.Language, len(inj.Ranges))
    // inj.Tree is the child language's parse tree
}
```

Supports static (`#set! injection.language "javascript"`) and dynamic (`@injection.language` capture) language detection, recursive nested injections, and incremental reparse with child tree reuse.

### Source rewriting

Collect source-level edits and apply atomically, producing `InputEdit` records for incremental reparse:

```go
rw := gotreesitter.NewRewriter(src)
rw.Replace(funcNameNode, []byte("newName"))
rw.InsertBefore(bodyNode, []byte("// added\n"))
rw.Delete(unusedNode)

newSrc, _ := rw.ApplyToTree(tree)
newTree, _ := parser.ParseIncremental(newSrc, tree)
```

`Apply()` returns both the new source bytes and the `[]InputEdit` records. `ApplyToTree()` is a convenience that calls `tree.Edit()` for each edit and returns source ready for `ParseIncremental`.

### Incremental reparsing

```go
tree, _ := parser.Parse(src)

// User types "x" at byte offset 42
src = append(src[:42], append([]byte("x"), src[42:]...)...)

tree.Edit(gotreesitter.InputEdit{
    StartByte:   42,
    OldEndByte:  42,
    NewEndByte:  43,
    StartPoint:  gotreesitter.Point{Row: 3, Column: 10},
    OldEndPoint: gotreesitter.Point{Row: 3, Column: 10},
    NewEndPoint: gotreesitter.Point{Row: 3, Column: 11},
})

tree2, _ := parser.ParseIncremental(src, tree)
```

`ParseIncremental` walks the old tree's spine, identifies the edit region, and reuses unchanged subtrees by reference. Only the invalidated span is re-lexed and re-parsed. Both leaf and non-leaf subtrees are eligible for reuse; non-leaf reuse is driven by pre-goto state tracking on interior nodes, so the parser can skip entire subtrees without re-deriving their contents.

When no edit has occurred, `ParseIncremental` detects the nil-edit on a pointer check and returns in single-digit nanoseconds with zero allocations.

### UTF-16 input and editor coordinates

UTF-16 callers can parse Go-native code units or endian-specific byte buffers
without converting offsets by hand. The parser core keeps its canonical UTF-8
view internally, while the returned tree retains the original UTF-16 source and
maps nodes, edits, included ranges, query filters, highlights, tags, and
injections back to UTF-16 code-unit coordinates.

```go
src := utf16.Encode([]rune("1+2"))

parser := gotreesitter.NewParser(lang)
tree, _ := parser.ParseUTF16(src)

rng, _ := tree.UTF16RangeForNode(tree.RootNode())
fmt.Println(rng.StartCodeUnit, rng.EndCodeUnit)

node := tree.DescendantForUTF16Range(0, uint32(len(src)))
_ = node

// Incremental edits can be described in UTF-16 code units.
next := utf16.Encode([]rune("1+3"))
tree.EditUTF16(gotreesitter.UTF16Edit{
    StartCodeUnit:  2,
    OldEndCodeUnit: 3,
    NewEndCodeUnit: 3,
}, next)
tree2, _ := parser.ParseIncrementalUTF16(next, tree)
_ = tree2
```

UTF-16 byte input is explicit about byte order:

```go
tree, _ := parser.ParseUTF16Bytes(buf, gotreesitter.UTF16LittleEndian)
```

Editor-facing APIs have UTF-16 variants:

```go
q, _ := gotreesitter.NewQuery(`(NUMBER) @number`, lang)
cursor := q.Exec(tree.RootNode(), lang, tree.Source())
cursor.SetUTF16Range(tree, 2, 3)

hl, _ := gotreesitter.NewHighlighter(lang, `(NUMBER) @number`)
highlightRanges := hl.HighlightUTF16(src)

tagger, _ := gotreesitter.NewTagger(lang, `(NUMBER) @name @definition.number`)
tags := tagger.TagUTF16(src)
```

Node byte APIs such as `DescendantForByteRange` still use the tree's canonical
UTF-8 byte offsets. Use `DescendantForUTF16Range` or convert with
`UTF8ByteForUTF16Offset` when starting from editor UTF-16 offsets.

### Tree cursor

`TreeCursor` maintains an explicit `(node, childIndex)` frame stack. Parent, child, and sibling movement are O(1) with zero allocations — sibling traversal indexes directly into the parent's `children[]` slice.

```go
c := gotreesitter.NewTreeCursorFromTree(tree)

c.GotoFirstChild()
c.GotoChildByFieldName("body")

for ok := c.GotoFirstNamedChild(); ok; ok = c.GotoNextNamedSibling() {
    fmt.Printf("%s at %d\n", c.CurrentNodeType(), c.CurrentNode().StartByte())
}

idx := c.GotoFirstChildForByte(128)
```

Movement methods: `GotoFirstChild`, `GotoLastChild`, `GotoNextSibling`, `GotoPrevSibling`, `GotoParent`, named-only variants (`GotoFirstNamedChild`, etc.), field-based (`GotoChildByFieldName`, `GotoChildByFieldID`), and position-based (`GotoFirstChildForByte`, `GotoFirstChildForPoint`).

Cursors hold direct pointers into tree nodes. Recreate after `Tree.Release()`, `Tree.Edit(...)`, or incremental reparse.

### Highlighting

```go
hl, _ := gotreesitter.NewHighlighter(lang, highlightQuery)
ranges := hl.Highlight(src)

for _, r := range ranges {
    fmt.Printf("%s: %q\n", r.Capture, src[r.StartByte:r.EndByte])
}
```

### Tagging

```go
entry := grammars.DetectLanguage("main.go")
lang := entry.Language()

tagger, _ := gotreesitter.NewTagger(lang, entry.TagsQuery)
tags := tagger.Tag(src)

for _, tag := range tags {
    fmt.Printf("%s %s at %d:%d\n", tag.Kind, tag.Name,
        tag.NameRange.StartPoint.Row, tag.NameRange.StartPoint.Column)
}
```

## Benchmarks

Canonical, linkable performance claims live in [BENCH.md](BENCH.md) — the
real-code full-parse matrix, historical incremental controls, the Go-vs-C
fleet scoreboard, memory receipts, and the methodology that keeps them honest.

`BenchmarkGoParseFullDFA` calls `Parser.Parse`, requires a complete root, and
releases the materialized tree. Its generated 500-function source is now a
historical straight-LR control, not the representative full-parse headline.
`BenchmarkGoParseCoreDFA` remains a parser-loop diagnostic that suppresses
ordinary tree materialization.

An audit on 2026-07-11 found that older versions of
`BenchmarkGoParseFullDFA` called `ParseNoResultCompatibilityBenchmarkOnly`,
which also enabled the no-tree path. The previously published `1.54 ms`,
`728 B/op`, and `7 allocs/op` figures—and v0.24.0's later `978 B/op` and
`5 allocs/op` figures—therefore described parser-core diagnostics, not a full
materialized parse. Those headline comparisons have been withdrawn. The
corrected historical control later measured 10.907 ms on a pinned quiet host.
A second audit found that this source never forks and that its C comparison
used a different Go grammar from gotreesitter. The former 1.895x ratio and 29%
materialization decomposition are therefore withdrawn rather than promoted as
representative claims.

The replacement canonical matrix freezes four clean, human-authored Go files
spanning 5-236 KiB. They reach 12-18 live stacks and constructed-to-selected
node ratios of 3.65-4.47. Admission requires exact deep parity against one
oracle: upstream tree-sitter v0.25.1 commit `f5afe475…`, tree-sitter-go commit
`2346a3ab…`, compiled with `-O2` into a fingerprinted static C artifact. The
benchmark and parity lanes share those oracle sources and identity; see
[BENCH.md](BENCH.md) for the full contract and fixture hashes.

The first strict materialized real-Go publication receipt, shipped with
v0.37.0, measured **5.481673x C** by equal-fixture geomean and **6.313799x C**
for the fixed-suite sum of medians. Those corrected real-code results, rather
than the withdrawn 1.895x straight-LR comparison, established the full-parse
baseline; [BENCH.md](BENCH.md) records every per-fixture median, RSS value, and
receipt hash.

A strict v0.40.0 production receipt at `1935a42c` measures public
`Parser.Parse` at **4.851050x C** by equal-fixture geomean and **5.472406x C**
by fixed-suite sum of medians, with a **5.608320x C** worst fixture. The latest
clean publication of the separately build-tagged, fail-closed selected-store
candidate, at `ba1ed1bf`, measures **2.685181x C** and **2.676794x C**,
respectively, with a **2.791974x C** worst fixture and zero timed fallbacks.
The measured lifecycle seals the accepted compact payloads into the indexed
selected store, walks them through `SelectedCursor`, and releases the store.
That candidate result is diagnostic: it authenticates visible
`gts-deep-tree-v1` structure for the four clean fresh-full fixtures, not
parser-state metadata, recovery, incremental reuse, included ranges, or public
`Parser.Parse`. See [BENCH.md](BENCH.md) for the paired tables, exact
identities, hashes, and support boundary.

The historical incremental measurements on the same generated 500-function Go
workload were `649 ns` for a one-byte edit and `2.43 ns` for a no-edit reparse.
They remain narrow workload-specific controls. The locked incremental matrix
validates correctness and classifies work, but does not establish a general
comparative Go/C speed headline.

```sh
# Authenticated real-code Go full-parse lanes:
GOMAXPROCS=1 go test . -run '^$' \
  -bench '^BenchmarkGoParseWarmRealDFA$' \
  -benchmem -count=10 -benchtime=750ms

# Complete locked static-C publication receipt (clean, quiet, Docker host):
bash cgo_harness/pure_c/run_canonical_go_full_parse.sh --core <idle-cpu>

# Build-tagged compact fresh-full candidate (diagnostic, fail-closed):
bash cgo_harness/pure_c/run_canonical_go_full_parse.sh \
  --go-backend candidate --core <idle-cpu>

# Historical straight-LR full parse plus incremental API controls:
GOMAXPROCS=1 go test . -run '^$' \
  -bench 'BenchmarkGoParseFullDFA|BenchmarkGoParseIncrementalSingleByteEditDFA|BenchmarkGoParseIncrementalNoEditDFA' \
  -benchmem -count=10 -benchtime=750ms

# Parser-core attribution only:
GOMAXPROCS=1 go test . -run '^$' \
  -bench '^BenchmarkGoParseCoreDFA$' \
  -benchmem -count=10 -benchtime=750ms
```

Correctness and performance are gated separately. For Go-vs-C real-corpus
timing, see [`cgo_harness/perf_scan`](cgo_harness/perf_scan/README.md); its
ratchet records caveats, timeouts, and resource gaps instead of extrapolating
from this generated-Go microbenchmark.

### Benchmark matrix

For repeatable multi-workload tracking:

```sh
go run ./cmd/benchmatrix --count 10
```

Emits `bench_out/matrix.json` (machine-readable), `bench_out/matrix.md` (summary), and raw logs under `bench_out/raw/`.
The default matrix includes a bounded, warmed language-family full-parse group, reported with `MB/s` so parser throughput can be compared across generated source sizes. Use `--only-family` to isolate that group, `--family-unit-count` to scale it, or `--no-family` for the narrower Go/editor matrix.

## Supported languages

206 grammars ship in the registry. All 206 produce error-free parse trees on smoke samples. Run `go run ./cmd/parity_report` for current status.

- 116 external scanners (hand-written Go implementations of upstream C scanners)
- 7 hand-written Go token sources (authzed, c, cpp, go, java, json, lua)
- Remaining languages use the DFA lexer generated from grammar tables

### Parse quality

Each `LangEntry` carries a `Quality` field:

| Quality | Meaning |
|---|---|
| `full` | All scanner and lexer components present. Parser has full access to the grammar. |
| `partial` | Missing external scanner. DFA lexer handles what it can; external tokens are skipped. |
| `none` | Cannot parse. |

`full` means the parser has every component the grammar requires. It does not guarantee error-free trees on all inputs — grammars with high GLR ambiguity may produce syntax errors on very large or deeply nested constructs due to parser safety limits (iteration cap, stack depth cap, node count cap). These limits scale with input size. Check `tree.RootNode().HasError()` at runtime.

<details>
<summary>Full language list (206)</summary>

`ada`, `agda`, `angular`, `apex`, `arduino`, `asm`, `astro`, `authzed`, `awk`, `bash`, `bass`, `beancount`, `bibtex`, `bicep`, `bitbake`, `blade`, `brightscript`, `c`, `c_sharp`, `caddy`, `cairo`, `capnp`, `chatito`, `circom`, `clojure`, `cmake`, `cobol`, `comment`, `commonlisp`, `cooklang`, `corn`, `cpon`, `cpp`, `crystal`, `css`, `csv`, `cuda`, `cue`, `cylc`, `d`, `dart`, `desktop`, `devicetree`, `dhall`, `diff`, `disassembly`, `djot`, `dockerfile`, `dot`, `doxygen`, `dtd`, `earthfile`, `ebnf`, `editorconfig`, `eds`, `eex`, `elisp`, `elixir`, `elm`, `elsa`, `embedded_template`, `enforce`, `erlang`, `facility`, `faust`, `fennel`, `fidl`, `firrtl`, `fish`, `foam`, `forth`, `fortran`, `fsharp`, `gdscript`, `git_config`, `git_rebase`, `gitattributes`, `gitcommit`, `gitignore`, `gleam`, `glsl`, `gn`, `go`, `godot_resource`, `gomod`, `graphql`, `groovy`, `hack`, `hare`, `haskell`, `haxe`, `hcl`, `heex`, `hlsl`, `html`, `http`, `hurl`, `hyprlang`, `ini`, `janet`, `java`, `javascript`, `jinja2`, `jq`, `jsdoc`, `json`, `json5`, `jsonnet`, `julia`, `just`, `kconfig`, `kdl`, `kotlin`, `ledger`, `less`, `linkerscript`, `liquid`, `llvm`, `lua`, `luau`, `make`, `markdown`, `markdown_inline`, `matlab`, `mermaid`, `meson`, `mojo`, `move`, `nginx`, `nickel`, `nim`, `ninja`, `nix`, `norg`, `nushell`, `objc`, `ocaml`, `odin`, `org`, `pascal`, `pem`, `perl`, `php`, `pkl`, `powershell`, `prisma`, `prolog`, `promql`, `properties`, `proto`, `pug`, `puppet`, `purescript`, `python`, `ql`, `r`, `racket`, `regex`, `rego`, `requirements`, `rescript`, `robot`, `ron`, `rst`, `ruby`, `rust`, `scala`, `scheme`, `scss`, `smithy`, `solidity`, `sparql`, `sql`, `squirrel`, `ssh_config`, `starlark`, `svelte`, `swift`, `tablegen`, `tcl`, `teal`, `templ`, `textproto`, `thrift`, `tlaplus`, `tmux`, `todotxt`, `toml`, `tsx`, `turtle`, `twig`, `typescript`, `typst`, `uxntal`, `v`, `verilog`, `vhdl`, `vimdoc`, `vue`, `wat`, `wgsl`, `wolfram`, `xml`, `yaml`, `yuck`, `zig`

</details>

## Query API

| Feature | Status |
|---|---|
| Compile + execute (`NewQuery`, `Execute`, `ExecuteNode`) | supported |
| Cursor streaming (`Exec`, `NextMatch`, `NextCapture`) | supported |
| Structural quantifiers (`?`, `*`, `+`) | supported |
| Alternation (`[...]`) | supported |
| Field matching (`name: (identifier)`) | supported |
| `#eq?` / `#not-eq?` | supported |
| `#match?` / `#not-match?` | supported |
| `#any-of?` / `#not-any-of?` | supported |
| `#lua-match?` | supported |
| `#has-ancestor?` / `#not-has-ancestor?` | supported |
| `#has-parent?` / `#not-has-parent?` | supported |
| `#is?` / `#is-not?` | supported |
| `#any-eq?` / `#any-not-eq?` | supported |
| `#any-match?` / `#any-not-match?` | supported |
| `#select-adjacent!` | supported |
| `#strip!` | supported |
| `#set!` / `#offset!` directives | parsed and accepted |
| `SetValues` (read `#set!` metadata from matches) | supported |

All shipped highlight and tags queries compile (`156/156` highlight, `69/69` tags).

## Repository layout note: the compat tier

The root package carries ~177 `parser_result_<language>*.go` files — the
C-faithful result-normalization tier that reshapes raw GLR output into the
exact tree the C runtime selects. It is internal plumbing, oracle-gated per
witness, and shrinks as certified engine mechanisms subsume shims. See
[docs/compat-tier.md](docs/compat-tier.md).

## Known limitations

- **Full-parse throughput**: the strict materialized real-Go production receipt at the v0.40.0 tag target `1935a42c` measures public `Parser.Parse` at **4.851050x C** by equal-fixture geomean and **5.472406x C** by fixed-suite sum of medians against the exact static `-O2` oracle (see [BENCH.md](BENCH.md)); its worst fixture is **5.608320x C**. The compact candidate's lower diagnostic ratio is not a public-parser claim. The former ~2.1x row used a straight-LR synthetic and a different Go grammar, so it is historical only. The locked incremental matrix validates correctness and classifies work, but general incremental Go/C performance has no current publication-grade headline. Full-parse throughput varies by grammar and corpus shape; GLR-heavy code, highly ambiguous languages, and very large generated files are the main performance frontier.
- **GLR safety caps**: The parser enforces iteration, stack depth, and node count limits proportional to input size. These prevent pathological blowup on grammars with high ambiguity but impose a ceiling on the maximum input complexity that parses without error. The caps are tunable but not removable without risking unbounded resource consumption.

## Adding a language

> Adding a language to your own project does **not** require forking this
> repo or following the in-tree steps below. See
> [docs/authoring-languages.md](docs/authoring-languages.md) for the
> out-of-tree pipeline (grammar.json → blob → `LoadLanguage` /
> `RegisterExtension`) and
> [docs/external-scanners.md](docs/external-scanners.md) for external
> scanners in Go. The steps below are for grammars embedded in this repo.

1. Add the grammar repo to `grammars/languages.manifest`
2. Refresh pinned refs in `grammars/languages.lock`:
   `go run ./cmd/grammar_updater -lock grammars/languages.lock -write -report grammars/grammar_updates.json`
3. Generate tables: `go run ./cmd/ts2go -manifest grammars/languages.manifest -outdir ./grammars -package grammars -compact=true`
4. Add smoke samples to `cmd/parity_report/main.go` and `grammars/parse_support_test.go`
5. Verify: `go run ./cmd/parity_report && go test ./grammars/...`

## Grammar lock updates

- `grammars/languages.lock` stores pinned refs for grammar update + parity automation.
- `cmd/grammar_updater` refreshes refs and emits a machine-readable report.
- `.github/workflows/grammar-lock-update.yml` opens scheduled/dispatch update PRs.
- Hand-written scanner ports can also declare `ExternalScannerSpec` metadata
  with upstream source hashes and external-token names. When a grammar update
  changes `src/scanner.c` or the external-token list, treat it as scanner work:
  update the Go scanner binding/port before replacing generated blobs. Grammar
  JSON-only changes with unchanged externals can usually follow the normal
  `grammar.json -> grammargen Go DSL -> blob -> parity` path.

Manual refresh:

```sh
go run ./cmd/grammar_updater \
  -lock grammars/languages.lock \
  -allow-list grammars/update_tier1_core100.txt \
  -max-updates 10 \
  -write \
  -report grammars/grammar_updates.json
```

## Architecture

gotreesitter is a ground-up reimplementation of the tree-sitter runtime in Go. No code is shared with or translated from the C implementation.

**Parser** — Table-driven LR(1) with GLR fallback. When a `(state, symbol)` pair maps to multiple actions in the parse table, the parser forks the stack and explores all alternatives in parallel. Stack merging collapses equivalent paths. Safety limits (iteration count, stack depth, node count) scale with input size and prevent runaway exploration on ambiguous grammars.

**Incremental engine** — Walks the edit region of the previous tree and reuses unchanged subtrees by reference. Non-leaf subtree reuse is enabled by storing a pre-goto parser state on each interior node, allowing the parser to skip an entire subtree and resume in the correct state without re-deriving its contents. External scanner state is serialized on each node boundary so scanner-dependent subtrees can be reused without replaying the scanner from the start.

**Lexer** — Two paths. A DFA lexer is generated from the grammar's lex tables by `ts2go` and handles the majority of languages. For grammars where the DFA is insufficient (e.g., Go's automatic semicolons, YAML's indentation-sensitive structure), hand-written Go token sources implement the `TokenSource` interface directly.

**External scanners** — 116 grammars require external scanners for context-sensitive tokens (Python indentation, HTML implicit close tags, Rust raw string delimiters, Swift operator disambiguation, etc.). Each scanner is a hand-written Go implementation of the grammar's `ExternalScanner` interface: `Create`, `Serialize`, `Deserialize`, `Scan`. Scanner state is snapshotted after every token and stored on tree nodes so incremental reuse can restore scanner state on skip.

**Arena allocator** — Nodes are allocated from slab-based arenas to reduce GC pressure. Arenas are released in bulk when a tree is freed.

**Query engine** — S-expression pattern compiler with predicate evaluation and streaming cursor iteration. Supports all standard tree-sitter predicates (`#eq?`, `#match?`, `#any-of?`, `#has-ancestor?`, etc.) and directive annotations (`#set!`, `#offset!`, `#select-adjacent!`, `#strip!`).

**Injection parser** — Orchestrates multi-language parsing. Runs injection queries against a parent tree to find embedded regions, spawns child parsers with `SetIncludedRanges()`, and recurses for nested injections. Incremental reparse reuses unchanged child trees.

**Rewriter** — Collects source-level edits (replace, insert, delete) targeting byte ranges, applies them atomically, and produces `InputEdit` records for incremental reparse. Edits are validated for non-overlap and applied in a single pass.

**Grammar loading** — `ts2go` extracts parse tables, lex tables, field maps, symbol metadata, and external token lists from upstream `parser.c` files. These are serialized to compressed binary blobs under `grammars/grammar_blobs/` and lazy-loaded via `loadEmbeddedLanguage()` with an LRU cache. String and transition interning reduce memory footprint across loaded grammars. Grammargen-backed blobs use the same CLI surface; for example, the Go blob can be regenerated with `go run ./cmd/grammargen -lr-split -bin grammars/grammar_blobs/go.bin go`. When loading a raw blob yourself, prefer `grammars.LoadLanguage(name, blob)` over `gotreesitter.LoadLanguage(blob)` so the registered external scanner and external lex-state support for that language are attached automatically.

### Build tags and environment

**External grammar blobs** (avoid embedding in the binary):

```sh
go build -tags grammar_blobs_external
GOTREESITTER_GRAMMAR_BLOB_DIR=/path/to/blobs  # required
GOTREESITTER_GRAMMAR_BLOB_MMAP=false           # disable mmap (Unix only)
```

**Curated language set** (smaller binary):

```sh
go build -tags grammar_set_core  # curated Core100 embedded grammar set
GOTREESITTER_GRAMMAR_SET=go,json,python  # runtime restriction
```

**Selective embedded grammars** (smallest self-contained binary — pick exactly the languages you ship):

```sh
# Embeds ONLY go.bin + java.bin into the binary (everything else is dropped at
# link time). No GOTREESITTER_GRAMMAR_BLOB_DIR needed — still a single static binary.
go build -tags 'grammar_subset grammar_subset_go grammar_subset_java'
```

Add one `grammar_subset_<lang>` tag per grammar you need (names match the blob
file: `grammar_subset_c_sharp`, `grammar_subset_python`, …). A single-language
build drops from ~24MB to a few MB. This is finer-grained than `grammar_set_core`
(a fixed set) and, unlike `grammar_blobs_external`, keeps the blobs embedded.
Pairing `grammar_subset` with `grammar_blobs_external` instead loads the selected
blobs from `GOTREESITTER_GRAMMAR_BLOB_DIR` at runtime (no embedded blobs at all).

> The four embedding modes are mutually exclusive at the build-tag level:
> default (all embedded) · `grammar_set_core` (Core100 embedded) ·
> `grammar_subset` + `grammar_subset_<lang>` (selected embedded) ·
> `grammar_blobs_external` (none embedded). Regenerate the per-language embed
> files after adding a grammar with `go run ./cmd/gen_subset_blob_embeds`.

**Grammar cache tuning** (long-lived processes):

```go
grammars.SetEmbeddedLanguageCacheLimit(8)    // LRU cap
grammars.UnloadEmbeddedLanguage("rust.bin")  // drop one
grammars.PurgeEmbeddedLanguageCache()        // drop all

fmt.Println(lang.Size())                      // approximate decoded table bytes
```

```sh
GOTREESITTER_GRAMMAR_CACHE_LIMIT=8       # LRU cap via env
GOTREESITTER_GRAMMAR_IDLE_TTL=5m         # evict after idle
GOTREESITTER_GRAMMAR_IDLE_SWEEP=30s      # sweep interval
GOTREESITTER_GRAMMAR_COMPACT=true        # loader compaction (default)
GOTREESITTER_GRAMMAR_STRING_INTERN_LIMIT=200000
GOTREESITTER_GRAMMAR_TRANSITION_INTERN_LIMIT=20000
```

**GLR stack cap override**:

```sh
GOT_GLR_MAX_STACKS=8  # overrides default GLR stack cap (default: 8)
```

Default is tuned for correctness. Increase only if a grammar/workload
needs more GLR alternatives to preserve parity.

**Legacy benchmark compatibility only**:

```sh
GOT_PARSE_NODE_LIMIT_SCALE=3
```

`GOT_PARSE_NODE_LIMIT_SCALE` is only needed for comparisons against older truncation-prone benchmark baselines. On current branches, keep it unset.

## Testing

```sh
bash cgo_harness/docker/run_single_grammar_parity.sh typescript
```

For local correctness/parity work, prefer isolated one-language Docker runs:

```sh
# Real-corpus parity for one grammar
bash cgo_harness/docker/run_single_grammar_parity.sh typescript

# Focused grammargen real-corpus lane for one language
bash cgo_harness/docker/run_grammargen_focus_targets.sh --mode real-corpus --langs typescript

# Focused grammargen-vs-C lane for one language
bash cgo_harness/docker/run_grammargen_focus_targets.sh --mode cgo --langs typescript
```

`run_grammargen_focus_targets.sh` is the safest local lane for high-value
grammars: it runs one grammar per container and defaults to a single-worker
profile (`--cpus 1`, `--pids 512`, `GOMAXPROCS=1`, `GOFLAGS=-p=1`).

For Fortran, both real-corpus runners also default to a tighter bounded local
preset unless you explicitly override it or pass
`--unsafe-fortran-defaults`: `--memory 3g`, `--cpus 1`, `--pids 512`,
`GOMAXPROCS=1`, `GOFLAGS=-p=1`, `GOT_LALR_LR0_CORE_BUDGET=160000000`, and
`GTS_GRAMMARGEN_REAL_CORPUS_GENERATE_TIMEOUT=15m`.

If you only need a fast package-local regression check, keep it in Docker and
narrow the `-run` regex:

```sh
bash cgo_harness/docker/run_parity_in_docker.sh \
  -- "cd /workspace && go test ./grammargen -run '^TestTypeScriptConditionalTypeParity$' -count=1"
```

Avoid `go test ./...` and host-side multi-language or race sweeps on developer
machines while chasing OOMs. Use CI or a dedicated container when broader race
coverage is required.

Other focused correctness/parity commands:

```sh
# Top-50 smoke correctness for the grammars package only
bash cgo_harness/docker/run_parity_in_docker.sh \
  -- "cd /workspace && go test ./grammars -run '^TestTop50(ParseSmokeNoErrors|CorrectnessListMatchesLockFile)$' -count=1 -v"

# Top-50 grammargen import/parity registry coverage
bash cgo_harness/docker/run_parity_in_docker.sh \
  -- "cd /workspace && go test ./grammargen -run '^TestTop50GrammarImportParityCoverage$' -count=1 -v"

# C-oracle parity suites inside the cgo harness
bash cgo_harness/docker/run_parity_in_docker.sh \
  --run '^TestParityFreshParse$|^TestParityHasNoErrors$|^TestParityIssue3Repros$|^TestParityGLRCanaryGo$'
bash cgo_harness/docker/run_parity_in_docker.sh \
  --run '^TestParityCorpusFreshParse$'
```

CI may still run broader race coverage on hosted runners. Do not copy those
commands onto a developer host during OOM diagnosis.

Test suite covers: smoke tests (206 grammars), golden S-expression snapshots, highlight query validation, query pattern matching, incremental reparse correctness, error recovery, GLR fork/merge, injection parsing, source rewriting, and fuzz targets.

## Roadmap

The current release is **v0.41.0**. It banks the post-v0.40 correctness and
tooling tranche plus five exact, build-tagged compact-parser performance wins.
Subsequent unreleased selected-store work keeps the public parser unchanged
while reducing the authenticated compact candidate to **2.685181x C** by
equal-fixture geomean, **2.676794x C** by fixed-suite sum, and **2.791974x C**
on its worst fixture, with zero timed fallback. Its measured lifecycle seals,
traverses, and releases the selected store; it is not public `Parser.Parse`.
The v0.41.0 release also includes build-time PGO,
forest-path pooling and copy elimination, query work budgeting, and incremental
reuse boundary corrections from the v0.40.0 line.
The authenticated production receipt at tag target `1935a42c` measures public
`Parser.Parse` at **4.851050x C** by equal-fixture geomean, **5.472406x C** by
fixed-suite sum, and **5.608320x C** on the worst fixture against the locked
static `-O2` C oracle. Its 0.716% geomean improvement over v0.39.0 is below the
reproducible 2% win threshold. The latest build-tagged selected-store candidate,
measured at `ba1ed1bf`, remains outside public `Parser.Parse` despite its lower
authenticated ratio.
The 206-grammar curated parity milestone is banked, the invalid historical
1.895x headline remains withdrawn, and the incremental matrix is a
correctness/work-classification receipt rather than a representative
comparative speed headline. Detailed history lives in
[CHANGELOG.md](CHANGELOG.md).

### Now — performance and extreme hygiene

- Keep correctness, C-oracle parity, and performance gates separate. Every
  optimization must preserve the selected full-span tree before its timing or
  memory result is considered.
- Hold the corrected materialized canonical full parse against the locked,
  authenticated real-code publication benchmark. The historical 1.895x result
  is not a target. No-tree, parser-core, and straight-LR lanes remain
  attribution tools, not substitutes for the public benchmark.
- Keep the exact-revision fleet sweep current with clean/error splits and
  C-oracle fingerprints. Eliminate valid rows above 3x and all timeout, hard-RSS,
  truncation, or unreported-stop cliffs, prioritizing absolute user cost over
  ratio noise on tiny files.
- Reduce recurring fixed work, discarded forest attempts, recovery and GLR
  construction debris, retained scratch, and compatibility walks only through
  mechanisms that generalize across measured witnesses.
- Track wall time, allocations, retained arena/scratch bytes, and hard-cgroup
  maximum RSS. Keep the public full-parse, incremental-edit, and incremental
  no-edit benchmark lanes alongside the forking real-code full-parse matrix;
  no-tree and synthetic straight-LR measurements stay diagnostic.
- Remove temporary telemetry and failed experimental paths when each lane
  closes. Add no public parse variant or parser-core language-name switch when
  an internal diagnostic or generated runtime profile can express the need.

### Measured memory boundary

- A production frozen-tree store remains closed by the current measurements:
  whole-tree conversion does not recover enough full-parse time and regresses
  incremental reuse. Do not treat pointer-light migration as an authorized next
  step without new evidence that clears those gates.
- Bounded semispace retention, earlier reclamation of unselected construction
  debris, and tighter parser-budget-to-process-RSS behavior remain viable
  experiments. Each must preserve query, cursor, and incremental semantics.

### Deferred — Go-native experience and broader architecture

- Context-aware `Engine`/`ParseRequest`, typed limits and stop reasons,
  immutable registries/profiles, and internal pooling/diagnostic modes.
- `Document`/`Snapshot` ownership for edits, changed ranges, cached queries,
  highlights, tags, injections, and release lifetimes.
- Changed-range analysis plans, a provenance-rich sectioned grammar bundle,
  stronger scanner conformance tooling, and measured optional AOT tiers.

## License

[MIT](LICENSE)
