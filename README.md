# gotreesitter

Pure-Go [tree-sitter](https://tree-sitter.github.io/) runtime. No CGo, no C toolchain. It cross-compiles to any `GOOS`/`GOARCH` target Go supports, including `wasip1`.

The widely used Go bindings (`smacker/go-tree-sitter` and
`tree-sitter/go-tree-sitter`) wrap the C runtime through CGo. That needs a C
cross-toolchain per target, breaks `go install` for downstream users without a
C compiler, and hides bugs from `go test -race`. gotreesitter has no C
dependency: the parser, lexer, query engine, incremental reparsing, arena
allocator, external scanners, and tree cursor are all implemented in Go. The
grammar blob is the only input.

Docs site: <https://gotreesitter.m31labs.dev>. Try it in the browser:
<https://gotreesitter.m31labs.dev/playground>.

## Install

```sh
go get github.com/odvcencio/gotreesitter
```

gotreesitter loads the same parse-table format that tree-sitter's C runtime uses. `ts2go` extracts grammar tables from upstream `parser.c` files, compresses them into binary blobs, and deserializes them on first use. 206 grammars ship in the registry.

The current release is **v0.58.0**, in which 34 grammars parse by default
through `ccore`, the C-faithful parser core. On `main`, 61 grammars do (one for
each record in `testdata/ccore_certificates.json`), among them C, TypeScript,
JSON, Python, Rust, Ruby, Kotlin, Elixir, Erlang, CSS, TOML and Objective-C.
Each passed a graduation gate: trees and subtree reuse equal pinned C, and time
and memory at or below the existing engine. The other grammars keep the
existing engine. No API call changes, but trees for these grammars now match C
where v0.57.0 did not (see the upgrade notes).

On `main` the default also holds for the registry helpers: `LangEntry.Parse`,
`ParseContext` and `ParseStrict`, `grammars.ParseFile` and `ParseFilePooled`,
and the highlighter from `LangEntry.NewHighlighter`. For C, the registry
attaches a hand-written Go token source; when the core serves the request it
lexes with the grammar's own tables instead, so these helpers give the same tree
as `Parser.Parse`. In v0.58.0 they kept C on the existing engine.

To keep the existing engine for one parser:

```go
parser := gotreesitter.NewParser(lang)
parser.SetAdmissionCandidateRoute(false) // applies from the next parse
```

If you change engines between parses, the first parse on the new engine starts
fresh. Later parses on that engine can reuse trees it produced.

For editor invalidation, edit the old tree before reparsing. Use
DiffChangedRanges(oldTree, newTree) for syntax changes. Include
oldTree.ChangedRanges() when query predicates read source text, since a
text edit can preserve the syntax tree shape. Node pointers are scoped to
their tree.

See [CHANGELOG.md](CHANGELOG.md) for every certified grammar, upgrade notes and
fixes, `testdata/ccore_certificates.json` for the exact grammar blobs, and
[docs/v1-design.md](docs/v1-design.md) for engine requirements and validation
gates.

## Quick start

```go
package main

import (
    "fmt"

    "github.com/odvcencio/gotreesitter"
    golang "github.com/odvcencio/gotreesitter/grammars/go"
)

func main() {
    lang := golang.Language()
    parser := gotreesitter.NewParser(lang)

    tree, _ := parser.Parse([]byte("package main\n\nfunc main() {}\n"))
    fmt.Println(tree.RootNode().SExpr(lang))
}
```

The standalone grammar package links the selected grammar without the full
catalog. For automatic language detection, import `grammars` and use
`grammars.DetectLanguage("main.go")` to resolve a filename to a `LangEntry`.
See [build tags](docs/build-tags.md) for other ways to select grammars.

To detect the language and parse in one call, use `grammars.ParseFile`. The
caller must release the returned tree:

```go
bt, err := grammars.ParseFile("main.go", src)
if err != nil {
    return err
}
defer bt.Release()
```

Each grammar is also a standalone package, so a binary that needs one language
does not import the whole catalog:

```go
import "github.com/odvcencio/gotreesitter/grammars/typescript"

lang := typescript.Language()
```

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
| Building, testing, and sending a pull request | [CONTRIBUTING.md](CONTRIBUTING.md) |
| Benchmarks and methodology | [docs/benchmarks.md](docs/benchmarks.md) |
| Engine requirements and validation | [docs/v1-design.md](docs/v1-design.md) |
| Docs site and playground | <https://gotreesitter.m31labs.dev>, <https://gotreesitter.m31labs.dev/playground> |
| Root package file map and ownership | [docs/repository-map.md](docs/repository-map.md) |
| Root-package file group map by subsystem | [docs/repository-map.md](docs/repository-map.md#root-file-name-prefixes) |
| Result-compatibility tier (`parser_result_*.go`) | [docs/compat-tier.md](docs/compat-tier.md) |
| Adding a grammar outside this repo | [docs/authoring-languages.md](docs/authoring-languages.md) |
| External scanner certification and fallback | [docs/external-scanners.md](docs/external-scanners.md) |
| Release process | [docs/releasing.md](docs/releasing.md) |
| Full changelog | [CHANGELOG.md](CHANGELOG.md), [docs/changelog/](docs/changelog/) |

## License

[MIT](LICENSE). The default `grammars` import currently includes four
copyleft grammars: caddy, disassembly and jq (GPL-3.0) and nim (MPL-2.0). Build
with `-tags gotreesitter_no_copyleft` to leave them out. See
[docs/licensing.md](docs/licensing.md).
