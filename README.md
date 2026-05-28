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

All measurements below use the same workload: a generated Go source file with 500 functions (`19294` bytes).
Numbers are medians from 10 runs on:

```text
goos: linux
goarch: amd64
cpu: Intel(R) Core(TM) Ultra 9 285
```

| Runtime | Full parse | Incremental (1-byte edit) | Incremental (no edit) |
|---|---:|---:|---:|
| Native C (pure C runtime) | 1.76 ms | 102.3 μs | 101.7 μs |
| CGo binding (C runtime via cgo) | ~2.0 ms | ~130 μs | — |
| gotreesitter (pure Go) | 1.54 ms | 649 ns | 2.43 ns |

On this workload:

- Full parse is faster than both listed C baselines: ~1.15x faster than native C and ~1.29x faster than the CGo binding.
- Incremental single-byte edits are ~158x faster than native C (~200x faster than CGo).
- No-edit reparses are ~41,800x faster than native C, zero allocations.

<details>
<summary>Raw benchmark output</summary>

```sh
# Pure Go (this repo):
GOMAXPROCS=1 go test . -run '^$' \
  -bench 'BenchmarkGoParseFullDFA|BenchmarkGoParseIncrementalSingleByteEditDFA|BenchmarkGoParseIncrementalNoEditDFA' \
  -benchmem -count=10 -benchtime=750ms

# CGo binding benchmarks:
cd cgo_harness
GOMAXPROCS=1 go test . -run '^$' -tags treesitter_c_bench \
  -bench 'BenchmarkCTreeSitterGoParseFull|BenchmarkCTreeSitterGoParseIncrementalSingleByteEdit|BenchmarkCTreeSitterGoParseIncrementalNoEdit' \
  -benchmem -count=10 -benchtime=750ms

# Native C benchmarks (no Go, direct C binary):
./pure_c/run_go_benchmark.sh 500 2000 20000
```

| Benchmark | Median ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| Native C full parse | 1,764,436 | — | — |
| Native C incremental (1-byte edit) | 102,336 | — | — |
| Native C incremental (no edit) | 101,740 | — | — |
| `CTreeSitterGoParseFull` | ~1,990,000 | 600 | 6 |
| `CTreeSitterGoParseIncrementalSingleByteEdit` | ~130,000 | 648 | 7 |
| `GoParseFullDFA` | 1,538,089 | 728 | 7 |
| `GoParseIncrementalSingleByteEditDFA` | 648.9 | 176 | 3 |
| `GoParseIncrementalNoEditDFA` | 2.432 | 0 | 0 |

</details>

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

## Known limitations

- **Full-parse throughput**: the 500-function Go benchmark is now faster than the listed C baselines, but full-parse throughput still varies by grammar and corpus shape. Highly ambiguous languages and very large generated files remain the main parity/performance frontier.
- **GLR safety caps**: The parser enforces iteration, stack depth, and node count limits proportional to input size. These prevent pathological blowup on grammars with high ambiguity but impose a ceiling on the maximum input complexity that parses without error. The caps are tunable but not removable without risking unbounded resource consumption.

## Adding a language

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

**Grammar loading** — `ts2go` extracts parse tables, lex tables, field maps, symbol metadata, and external token lists from upstream `parser.c` files. These are serialized to compressed binary blobs under `grammars/grammar_blobs/` and lazy-loaded via `loadEmbeddedLanguage()` with an LRU cache. String and transition interning reduce memory footprint across loaded grammars. Grammargen-backed blobs use the same CLI surface; for example, the Go blob can be regenerated with `go run ./cmd/grammargen -lr-split -bin grammars/grammar_blobs/go.bin go`.

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

v0.19.x — GLR materialization, query parity, and parser hot-path release.
Compact/lazy final child refs now survive parser result assembly and public tree
operations, so queries, cursors, edits, descendant lookup, and traversal can
avoid broad eager materialization. Nested repeated query patterns now preserve
tree-sitter-compatible match rows, including downstream Kotlin Orion queries
such as `source_file -> import_list -> import_header`. The release also adds
reduce-chain hints, GLR/action/result timing attribution, parse-gap reporting,
and full-parse scratch tuning while restoring compatibility shapes for Go,
JavaScript/TypeScript, Python, Rust, C, and Java edge cases.

v0.18.x — Cold dependency extraction and parser materialization diagnostics
release. Adds language-neutral import extraction APIs, source-vs-tree import
parity fixtures, `cgo_harness/cmd/import_replay`, Python materialization
benchmarks, and parser runtime attribution for arena usage, checkpoint storage,
reduction/transient storage, final tree materialization, normalization timing,
and GLR collapse behavior. Hybrid source extraction now gives downstream
dependency scanners a fast path with structured fallback reporting.

v0.17.x — Java corpus parity and parser-performance release. Java now has
bounded Docker corpus lanes for Apache Lucene, including largest-file, random,
timeout-sweep, cgo comparison, no-tree diagnostic, UAX generated-file,
ambiguity, materialization, traversal, and query/API-shape runs. Targeted Java
lexer and GLR fixes close the correctness/timeout cliff for the sampled Lucene
corpora, while deferred parent-link wiring and parser scratch reuse move Java
full parses much closer to cgo. The release also expands `grammargen` top-50
parity coverage and fixes Bash, Python, Swift, comment, gomod, ini, CPON, D,
PowerShell, Julia, and Java parity gaps.

v0.16.x — Grammar extensibility and parser-resilience release. Adds native
UTF-16 parser/editor APIs, `grammargen` DSL constructors and extension smoke
coverage for Kotlin, Swift, JavaScript, TypeScript/TSX, and Fortran, and
grammar-update guardrails that block scanner-facing lock refreshes until
regenerated artifacts and focused parity are handled. C# pathological recovery
is bounded, TypeScript and Fortran `grammargen` parity advanced, Python
f-string scanner checkpoints preserve interpolated-string state, and
parser-result compatibility shims are isolated behind an explicit strut registry
with language-owned helper files.

v0.15.x — Large-repo consumer safety and parser-maintenance release. `ParsePolicy.ShouldSkipDir` lets gateway callers prune generated/vendor directories before descent, the GLR node-equivalence cache is smaller and checks epoch first for L2-friendly lookups, `Tree.Edit` avoids scanning unchanged right-side siblings when there is no tail shift, and parser-result compatibility normalization now keeps language-specific call sequences beside the relevant `parser_result_*.go` helpers. The v0.15.1 patch also hardens arena release/GC behavior, releases retry loser arenas promptly, and fixes query predicate backtracking for nested Starlark dictionary matches. v0.15.2 folds the drifting `main` and release lines back together, adds a Swift ABI mangling grammar, and ships `grammar_updater` pin verification and manifest-only sync flags. v0.15.3 caps JavaScript/TypeScript full-parse merge survivors, tunes markdown retry and node budgets, tolerates external-scanner symbol-list drift, and adds a scoped Canopy harness runner for bounded repo analysis. This line carries the post-0.14 tier-1 grammar refreshes and reserved-word import fixes.

v0.14.x — Go grammar now shipped as a grammargen-compiled blob (our own pure-Go LR(1) state-splitting compiler), eliminating a dead-end state inherited from tree-sitter-go that wrapped several valid Go files in ERROR. Combined with arena retention/initial-sizing fixes, retry-lifecycle cleanup, and a GLR cap update keyed to the new grammar's conflict profile, warm-reuse heap allocation across a six-file self-parse benchmark dropped ~54% (498 → 229 MB/iter); cold-case dropped ~61%.

v0.12.x — 206 grammars (all OK), 116 external scanners, pure-Go runtime plus `grammargen`, ABI 15 support including reserved-word sets, GLR parser, incremental reparsing with external scanner checkpoints, query engine, tree cursor, highlighting, tagging, injection parser, typed query codegen, CST rewriter, parser pool, arena memory budgets, and structural parity against 100+ curated C reference grammars.

Next:
- Retire parser-result struts by moving C#, Rust, Scala, TypeScript, and Python recovery into runtime or grammar generation paths
- Grammar refresh automation that moves from lock-only PRs to regenerated artifacts and focused parity for allow-listed `grammargen`-backed languages
- Table-size and codegen compaction work for Unicode-heavy grammars

Release history and retroactive notes are tracked in [CHANGELOG.md](CHANGELOG.md).

## License

[MIT](LICENSE)
