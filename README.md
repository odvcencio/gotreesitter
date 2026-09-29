# gotreesitter

A pure-Go implementation of the [tree-sitter](https://tree-sitter.github.io/) runtime. It parses source code into syntax trees, updates those trees as the code changes, and searches them with queries. It uses no CGo and no C toolchain.

Pure Go matters for four practical reasons:

- **Cross-compiling works.** Any `GOOS`/`GOARCH` pair that Go supports works, including `wasip1` and the browser (`GOOS=js GOARCH=wasm`). CGo bindings need a C cross-compiler for each target.
- **`go install` works** for people who have no C compiler.
- **`go test -race` sees the whole parser.** CGo hides the C side from the race detector.
- **One `go build` produces one static binary.** There is no shared library to ship.

The parser, lexer, query engine, incremental reparser, arena allocator, external scanners, and tree cursor are all Go. The one input that is not Go source is a grammar blob: a compressed parse table that `ts2go` generates from an upstream tree-sitter grammar.

The current release is **v0.55.1**. It ships **206 grammars** (counted from `grammars.AllLanguages()`).

## What is tree-sitter?

Tree-sitter is a parsing library that editors and code tools use to understand source code. Given a grammar for a language, it turns text into a concrete syntax tree: a tree of nodes such as `function_declaration` or `identifier`, each tied to a byte range in the source. It keeps producing a useful tree when the code has errors, which is what an editor needs while you type. It can also reparse after an edit by reusing the parts of the old tree that did not change.

The tree-sitter project publishes grammars for many languages, and most code tools that highlight, fold, index, or refactor code use them. gotreesitter reads those same grammars. It is a Go reimplementation of the runtime, not a wrapper around the C one.

## Install

```sh
go get github.com/odvcencio/gotreesitter
```

The module needs Go 1.22 or newer (see `go.mod`).

## Quick start

```go
import (
    "fmt"

    "github.com/odvcencio/gotreesitter"
    "github.com/odvcencio/gotreesitter/grammars"
)

func main() {
    lang := grammars.GoLanguage()
    parser := gotreesitter.NewParser(lang)

    tree, _ := parser.Parse([]byte("package main\n\nfunc main() {}\n"))
    fmt.Println(tree.RootNode().SExpr(lang))
}
```

`SExpr` prints the tree as an S-expression, for example `(source_file (package_clause ...) (function_declaration ...))`.

## Examples

Each snippet below was compiled and run against this repository at v0.55.1. They share the variables `src` (a `[]byte` of Go source), `lang`, `parser`, and `tree` from the steps above them. The [API guide](docs/api-guide.md) covers each topic in depth.

### Pick a language from a filename

```go
entry := grammars.DetectLanguage("server/main.go") // *grammars.LangEntry, or nil if unknown
lang := entry.Language()
parser := gotreesitter.NewParser(lang)
```

`grammars.DetectLanguageByName("go")` looks up by language name, and `grammars.DetectLanguageByShebang` reads a `#!` line. To parse a file in one call, use `grammars.ParseFile("x.py", src)`. It detects the language and returns a `*gotreesitter.BoundTree` that you `Release` when done.

### Run a query and read its captures

A query is a pattern written in S-expression syntax. Each `@name` in the pattern is a capture that labels the node it matched.

```go
src := []byte("package main\n\nfunc add(a, b int) int { return a + b }\n\nfunc main() {}\n")
tree, _ := parser.Parse(src)
defer tree.Release()

q, err := gotreesitter.NewQuery(`(function_declaration name: (identifier) @fn)`, lang)
if err != nil {
    panic(err)
}
cursor := q.Exec(tree.RootNode(), lang, src)
for {
    match, ok := cursor.NextMatch()
    if !ok {
        break
    }
    for _, c := range match.Captures {
        fmt.Println(c.Name, c.Node.Text(src)) // fn add, then fn main
    }
}
```

### Reparse after an edit

Tell the old tree what changed with an `InputEdit`, then call `ParseIncremental`. The parser reuses every part of the old tree that the edit did not touch. This example renames `add` to `sum`, which keeps the same length.

```go
start := bytes.Index(src, []byte("add")) // byte offset of the name
newSrc := append(append(append([]byte{}, src[:start]...), "sum"...), src[start+3:]...)

tree.Edit(gotreesitter.InputEdit{
    StartByte:   uint32(start),
    OldEndByte:  uint32(start + 3),
    NewEndByte:  uint32(start + 3),
    StartPoint:  gotreesitter.Point{Row: 2, Column: 5},
    OldEndPoint: gotreesitter.Point{Row: 2, Column: 8},
    NewEndPoint: gotreesitter.Point{Row: 2, Column: 8},
})
tree2, _ := parser.ParseIncremental(newSrc, tree)
defer tree2.Release()
fmt.Println(tree2.RootNode().HasError()) // false
```

`Rewriter` builds these `InputEdit` records for you when you change source programmatically. See [Incremental reparsing](docs/api-guide.md#incremental-reparsing) for the ownership rules on old and new trees.

### Highlight and tag

Highlighting labels byte ranges (keyword, string, function name). Tagging lists definitions and references, which is what code search and outline views use. Both run a query that ships with the grammar.

```go
hl, _ := gotreesitter.NewHighlighter(lang, entry.HighlightQuery)
for _, r := range hl.Highlight(newSrc) {
    fmt.Printf("%s %q\n", r.Capture, newSrc[r.StartByte:r.EndByte]) // keyword "package", ...
}

tagger, _ := gotreesitter.NewTagger(lang, grammars.ResolveTagsQuery(*entry))
for _, tag := range tagger.Tag(newSrc) {
    fmt.Println(tag.Kind, tag.Name) // definition.function sum, definition.function main
}
```

### Parse in the browser

Build `./wasm/runtime` with `GOOS=js GOARCH=wasm`, serve it with the grammar blob for your language, and call it from JavaScript:

```js
const api = await loadGotreesitter("/wasm/gotreesitter-runtime.wasm");
api.loadBlob("go", blobBytes, "");
const parsed = api.parse("go", "package main\nfunc hello() {}\n");
console.log(parsed.sexp);
```

The [WebAssembly guide](wasm/README.md) has the full build steps, the document API for editors, and a second target that generates grammars inside the browser. The browser snippet above is condensed from that guide's example; I did not run it in a browser for this README.

## How it works

```
 upstream grammar repo
        |  src/parser.c
        v
     ts2go  ------------------>  compressed grammar blob (grammars/grammar_blobs/*.bin)
                                          |  lazy load on first use (LRU cache)
                                          v
 source bytes --> lexer (DFA or Go token source, plus Go external scanner)
                       |
                       v
                  GLR parser  --(previous tree + InputEdit)--> subtree reuse
                       |
                       v
                  tree in an arena --> queries, highlight, tags, outline, cursor
```

1. **Grammar to blob.** A tree-sitter grammar compiles to a `parser.c` file that holds its parse tables. `ts2go` reads those tables, compresses them into a binary *grammar blob*, and writes it under `grammars/grammar_blobs/`. The runtime loads a blob the first time you ask for that language. `grammargen` can also build a blob directly from a `grammar.json` file.
2. **Lexing.** The lexer splits text into tokens. Most languages use a DFA lexer (a table-driven state machine) generated from the grammar. Seven languages use a hand-written Go token source instead (`authzed`, `c`, `cpp`, `go`, `java`, `json`, `lua`, per [docs/languages.md](docs/languages.md)).
3. **External scanners.** Some tokens depend on context that a table cannot express, such as Python indentation or heredocs. Upstream grammars handle these with a C *external scanner*. gotreesitter ports each one to Go: 119 grammars register one.
4. **GLR parsing.** The parser is table-driven LR(1) with a GLR fallback. *GLR* means that when the table allows more than one action, the parser forks its stack, follows every branch, and merges branches that converge. Safety limits scale with input size to stop runaway ambiguity.
5. **Incremental reparse.** After an edit, the parser walks the old tree, finds the edited region, and reuses unchanged subtrees by reference.
6. **Arena and tree.** Nodes come from slab arenas, and `Tree.Release` frees them in bulk. This keeps GC pressure low.
7. **Query engine.** The engine compiles S-expression patterns and streams matches from a cursor. It supports the standard predicates (`#eq?`, `#match?`, `#any-of?`, and others) and directives.
8. **Injections.** An *injection* is code in one language inside a document in another, such as JavaScript in HTML or a fenced code block in Markdown. `InjectionParser` finds these regions with a query and parses each with the right grammar.

[docs/architecture.md](docs/architecture.md) describes each component in detail.

## Correctness

The target is the same tree the C runtime produces. The project checks this by differential testing: it runs the C runtime and gotreesitter on the same input and compares the trees node by node (type, byte range, named or anonymous, missing, child count).

- **Parity harness.** `cgo_harness/` is a separate Go module that links the C runtime through CGo, so the main module stays CGo-free. Its tests compare fresh parses, incremental parses, highlight captures, query results, supertype maps, and error recovery against the C runtime.
- **Real-code corpus.** The harness builds a corpus of pinned open-source files per language, in size tiers, alongside small smoke samples, minimized repro cases, and malformed inputs.
- **Invariant gates.** An incremental parse must produce the same tree as a fresh parse of the same text. Where the project cannot yet prove that, the parser falls back to a fresh parse.

Where it stands, from [docs/c-parity-boards.md](docs/c-parity-boards.md) (counts dated 2026-09-08 unless noted):

| Board | Result |
|---|---|
| Highlight captures, every bundled query | 204 of 206 languages agree with C. The other 2 (`hurl`, `mojo`) have no C reference build. |
| Query semantics | 101 of 103 cases agree. 2 known divergences are documented. |
| Supertype maps | 40 of 69 grammars agree; 29 diverge. |
| Error recovery on malformed input | 39 of 79 cases match C on the default route (2026-09-19). |

So valid code parses to the C tree in the cases the project tests, and error recovery does not yet match C in about half of the recovery cases. A `full` parse-quality entry also does not guarantee an error-free tree on every input: safety limits can stop a very large or highly ambiguous parse early. Check `tree.RootNode().HasError()` and use `ParseStrict` when a partial tree must fail the request.

The project runs the parity tests in Docker, one language at a time. [docs/testing-guide.md](docs/testing-guide.md) has the commands. [docs/roadmap.md](docs/roadmap.md) and the [v1 design](docs/v1-design.md) describe the work that remains, including merging the two engine routes and stabilizing the v1 API.

## Performance

gotreesitter trades some full-parse speed for portability. [BENCH.md](BENCH.md) is the one page that holds performance claims. Every number there links to a receipt.

- **Full parse versus C.** On four real Go files (5 KB to 236 KB), the sealed v9 receipt measures full parses at 4.8 times the C runtime's time (geometric mean, production route). Across the language fleet, the median ratio is about 3x on the 2026-07-11 ledger. 10 languages run at or below 1x, and small-file grammars show the largest ratios because fixed per-parse cost dominates.
- **Incremental work.** On a generated 500-function Go file (a control, not a typical file), a reparse with no edit takes 8.30 ns and allocates nothing. A one-byte edit takes 186 µs and 5 allocations. A full parse of the same file takes 8.57 ms. These are medians of 20 runs on a quiet 4-core VM at commit `5cbd4fb2`, 2026-09-27.

Read [BENCH.md](BENCH.md) for the methodology and caveats, and [docs/benchmark-notes.md](docs/benchmark-notes.md) for withdrawn claims. Compare code changes with paired runs on one host, not across hosts.

## Languages

206 grammars ship in the registry, from `ada` to `zig`. All 206 parse their smoke samples without errors. [docs/languages.md](docs/languages.md) has the full list, the parse-quality levels (`full`, `partial`, `none`), and the query feature matrix.

You can import one grammar directly, which keeps the other blobs out of your binary:

```go
import python "github.com/odvcencio/gotreesitter/grammars/python"

parser := gotreesitter.NewParser(python.Language())
```

For a smaller binary with the registry (and filename detection), pick a subset with build tags:

```sh
go build -tags 'grammar_subset grammar_subset_go grammar_subset_java'   # embed only Go and Java
go build -tags grammar_set_core                                         # a curated set of about 100 grammars
go build -tags grammar_blobs_external                                   # embed none; load blobs from disk
```

[docs/build-tags.md](docs/build-tags.md) explains each mode, the cache settings, and the environment variables.

To add a language, you can use a grammar from your own project without forking this repository: [docs/authoring-languages.md](docs/authoring-languages.md) covers the path from `grammar.json` to a loaded language, and [docs/external-scanners.md](docs/external-scanners.md) covers writing a scanner in Go. To add a grammar to this repository, follow [Adding a language](docs/languages.md#adding-a-language).

## Tools

| Command | What it does |
|---|---|
| `cmd/ts2go` | Extracts parse tables from an upstream `parser.c` and writes a compressed grammar blob. |
| `cmd/grammargen` | Builds a blob from a Go DSL, `grammar.json`, or `grammar.js`, without the tree-sitter CLI. See [grammargen/README.md](grammargen/README.md). |
| `cmd/tsquery` | Generates typed Go structs and match helpers from a `.scm` query file. |
| `cmd/parity_report` | Reports parse support for every registered grammar. |
| `cmd/wasmassets` | Builds a single-language browser bundle with a digest manifest. |
| `cmd/grammar_updater` | Refreshes the pinned upstream grammar refs in `grammars/languages.lock`. |

Typed query codegen, for example:

```sh
go run ./cmd/tsquery -input queries/go_functions.scm -lang go -output go_functions_query.go -package queries
```

The [API guide](docs/api-guide.md#typed-query-codegen) shows the generated code. Other `cmd/` tools support the benchmark, release, and CI gates, and are described in [docs/releasing.md](docs/releasing.md) and [docs/ci-gate-coverage.md](docs/ci-gate-coverage.md).

## Documentation

Pick the page that matches what you are doing.

**Use the library**

| Read this when you want to... | Where |
|---|---|
| Parse, query, reparse incrementally, use UTF-16 offsets, walk with a cursor, highlight, tag, or outline | [docs/api-guide.md](docs/api-guide.md) |
| Run in the browser | [wasm/README.md](wasm/README.md) |
| Shrink your binary or tune grammar caching | [docs/build-tags.md](docs/build-tags.md) |
| See which languages are supported and which query features work | [docs/languages.md](docs/languages.md) |

**Understand the design**

| Read this when you want to... | Where |
|---|---|
| Learn how the parser, lexer, scanners, arena, and query engine fit together | [docs/architecture.md](docs/architecture.md) |
| Find the file that owns a subsystem | [docs/repository-map.md](docs/repository-map.md), [docs/package-layout.md](docs/package-layout.md) |
| Understand the result-compatibility tier (`parser_result_*.go`) | [docs/compat-tier.md](docs/compat-tier.md) |
| Know which external scanners are certified for incremental reuse | [docs/external-scanners.md](docs/external-scanners.md) |
| See where the project is going | [docs/roadmap.md](docs/roadmap.md), [docs/v1-design.md](docs/v1-design.md) |

**Check the claims**

| Read this when you want to... | Where |
|---|---|
| Run the tests and parity gates | [docs/testing-guide.md](docs/testing-guide.md) |
| See how parity with C stands, board by board | [docs/c-parity-boards.md](docs/c-parity-boards.md) |
| Reproduce a benchmark or read the methodology | [BENCH.md](BENCH.md), [docs/benchmark-notes.md](docs/benchmark-notes.md) |

**Extend or maintain**

| Read this when you want to... | Where |
|---|---|
| Add a grammar from outside this repository | [docs/authoring-languages.md](docs/authoring-languages.md) |
| Write or certify an external scanner | [docs/external-scanners.md](docs/external-scanners.md) |
| Cut a release | [docs/releasing.md](docs/releasing.md) |
| See what changed in each version | [CHANGELOG.md](CHANGELOG.md), [docs/changelog/](docs/changelog/) |

## Contributing

Issues and pull requests are welcome. Before you open a pull request:

1. Read [AGENTS.md](AGENTS.md). It sets the workflow for correctness gates, performance checks, and how to write PR descriptions.
2. Run the focused tests for the code you changed. For parser, lexer, or grammar changes, run the parity check for the affected language in Docker ([docs/testing-guide.md](docs/testing-guide.md)).
3. Fill in the [pull request template](.github/PULL_REQUEST_TEMPLATE.md). It asks for your correctness evidence, before and after performance numbers when they matter, and what you did not validate.

Report a wrong parse with the [parser report template](.github/ISSUE_TEMPLATE/parser_report.yml).

## Agent skill

Agents working with gotreesitter should use the [using-gotreesitter](https://github.com/odvcencio/m31labs-skills/blob/main/skills/using-gotreesitter/SKILL.md) skill.

## License

[MIT](LICENSE). Third-party notices are in [THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES) and [docs/licensing.md](docs/licensing.md).
