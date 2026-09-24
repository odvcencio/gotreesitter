# gotreesitter

Pure-Go [tree-sitter](https://tree-sitter.github.io/) runtime. No CGo, no C toolchain. It cross-compiles to any `GOOS`/`GOARCH` target Go supports, including `wasip1`.

Every Go tree-sitter binding in the ecosystem depends on CGo, which needs a
C cross-toolchain per target, breaks `go install` for downstream users
without a C compiler, and hides bugs from `go test -race`. gotreesitter
removes the C dependency entirely: the parser, lexer, query engine,
incremental reparsing, arena allocator, external scanners, and tree cursor
are all implemented in Go. The grammar blob is the only input.

## Install

```sh
go get github.com/odvcencio/gotreesitter
```

gotreesitter loads the same parse-table format that tree-sitter's C runtime uses. `ts2go` extracts grammar tables from upstream `parser.c` files, compresses them into binary blobs, and deserializes them on first use. 206 grammars ship in the registry.

The current release is **v0.55.0**. See [docs/roadmap.md](docs/roadmap.md) for
release scope and history.

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

`grammars.DetectLanguage("main.go")` resolves a filename to the matching `LangEntry`.

## Features

- **206 grammars**, all producing error-free trees on smoke samples; 119 ship a hand-written Go external scanner.
- **Incremental reparsing** that reuses unchanged tree content by reference, plus a no-edit fast path with zero allocations.
- **Queries** with the full S-expression pattern language and all standard tree-sitter predicates and directives.
- **Typed query codegen** (`cmd/tsquery`) that generates Go structs and match helpers from `.scm` files.
- **Injection parsing** for multi-language documents (HTML+JS+CSS, Markdown+code fences, Vue/Svelte).
- **Highlighting, tagging, and file outlines** built on the same tags-query captures.
- **UTF-16 input and editor coordinates**, so editor integrations do not hand-convert offsets.
- **Source rewriting** that produces `InputEdit` records ready for incremental reparse.
- **WebAssembly/browser runtime** with both a blob-loading target and an in-browser grammargen target. See the [WebAssembly guide](wasm/README.md).
- **Build-tag-selected grammar embedding** (external blobs, a curated core set, or a hand-picked subset) for smaller binaries.

## Agent skill

Agents working with gotreesitter should use the [using-gotreesitter](https://github.com/odvcencio/m31labs-skills/blob/main/skills/using-gotreesitter/SKILL.md) skill.

## Documentation

| Topic | Where |
|---|---|
| Parsing, queries, injections, incremental reparse, UTF-16, cursor, highlighting, tagging, outlines | [docs/api-guide.md](docs/api-guide.md) |
| Runtime architecture (parser, lexer, scanners, arena, query engine, grammar loading) | [docs/architecture.md](docs/architecture.md) |
| Build tags and environment variables | [docs/build-tags.md](docs/build-tags.md) |
| Supported languages, query feature matrix, adding a language | [docs/languages.md](docs/languages.md) |
| Running tests and correctness/parity gates | [docs/testing-guide.md](docs/testing-guide.md) |
| Benchmarks and methodology | [BENCH.md](BENCH.md), [docs/benchmark-notes.md](docs/benchmark-notes.md) |
| Current release scope and roadmap | [docs/roadmap.md](docs/roadmap.md) |
| Root package file map and ownership | [docs/repository-map.md](docs/repository-map.md) |
| Root-package file group map by subsystem | [docs/package-layout.md](docs/package-layout.md) |
| Result-compatibility tier (`parser_result_*.go`) | [docs/compat-tier.md](docs/compat-tier.md) |
| Adding a grammar outside this repo | [docs/authoring-languages.md](docs/authoring-languages.md) |
| External scanner certification and fallback | [docs/external-scanners.md](docs/external-scanners.md) |
| Release process | [docs/releasing.md](docs/releasing.md) |
| Full changelog | [CHANGELOG.md](CHANGELOG.md), [docs/changelog/](docs/changelog/) |

## License

[MIT](LICENSE)
