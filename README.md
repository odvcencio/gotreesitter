# gotreesitter

Pure-Go [tree-sitter](https://tree-sitter.github.io/) runtime — no CGo, no C toolchain, WASM-ready.

Implements the same parse table format tree-sitter uses, so existing grammars work without recompilation. Faster than the CGo binding for incremental edits — the dominant workload in editors and language servers.

## Quick Start

```go
src := []byte(`package main

func main() {}
`)

lang := grammars.GoLanguage()
parser := gotreesitter.NewParser(lang)

tree := parser.Parse(src)
fmt.Println(tree.RootNode())

// Incremental reparse
// (apply tree.Edit(...) edits before ParseIncremental when source changed)
tree2 := parser.ParseIncremental(src, tree)
_ = tree2
```

### Queries

```go
q, _ := gotreesitter.NewQuery(`(function_declaration name: (identifier) @fn)`, lang)
cursor := q.Exec(tree.RootNode(), tree.Language(), src)

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

> **Note:** `ExecuteNode` requires `source []byte` for text predicates (`#eq?`, `#match?`, `#any-of?`, `#not-eq?`) to evaluate correctly. Passing `nil` disables text-predicate checks for that call.

---

## Benchmarks

Measured against [`go-tree-sitter`](https://github.com/smacker/go-tree-sitter) (the standard CGo binding), parsing a Go source file.

```
goos: linux / goarch: amd64 / cpu: Intel(R) Core(TM) Ultra 9 285
go test -run '^$' -tags treesitter_c_bench -bench 'Benchmark(GoParse|CTreeSitterGoParse)' -benchmem
```

| Benchmark | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| `BenchmarkCTreeSitterGoParseFull` | 1,960,803 | 600 | 6 |
| `BenchmarkCTreeSitterGoParseIncrementalSingleByteEdit` | 116,598 | 648 | 7 |
| `BenchmarkCTreeSitterGoParseIncrementalNoEdit` | 114,872 | 600 | 6 |
| `BenchmarkGoParseFull` | 813,371 | 12,698 | 2,495 |
| `BenchmarkGoParseIncrementalSingleByteEdit` | 6,360 | 193 | 6 |
| `BenchmarkGoParseIncrementalNoEdit` | 8 | 0 | 0 |

**Summary:**

| Workload | gotreesitter | CGo binding | Ratio |
|---|---:|---:|---|
| Full parse | 813 µs | 1,961 µs | **2.4× faster** |
| Incremental (single-byte edit) | 6.4 µs | 117 µs | **18× faster** |
| Incremental (no-op reparse) | 8 ns | 115 µs | **~14,000× faster** |

The no-edit path exits in a single nil-check: zero allocations, ~8 ns. The CGo binding pays CGo call overhead unconditionally.

---

## Supported Languages

`go run ./cmd/parity_report` → `parseable=25 total=25 unsupported=0`

| Language | Backend | Status |
|---|---|---|
| `bash` | `dfa-partial` | `ok` |
| `c` | `token_source` | `ok` |
| `cpp` | `token_source` | `ok` |
| `css` | `dfa-partial` | `ok` |
| `elixir` | `dfa-partial` | `degraded` |
| `go` | `token_source` | `ok` |
| `html` | `token_source` | `ok` |
| `java` | `token_source` | `ok` |
| `javascript` | `token_source` | `ok` |
| `json` | `token_source` | `ok` |
| `kotlin` | `dfa-partial` | `ok` |
| `lua` | `token_source` | `ok` |
| `nix` | `dfa-partial` | `ok` |
| `php` | `dfa-partial` | `ok` |
| `python` | `dfa-partial` | `ok` |
| `ruby` | `dfa-partial` | `ok` |
| `rust` | `token_source` | `ok` |
| `scala` | `dfa-partial` | `ok` |
| `sql` | `dfa-partial` | `ok` |
| `swift` | `dfa` | `ok` |
| `toml` | `token_source` | `ok` |
| `tsx` | `dfa-partial` | `ok` |
| `typescript` | `token_source` | `ok` |
| `yaml` | `dfa-partial` | `degraded` |
| `zig` | `dfa` | `ok` |

**Backend key:**
- `dfa` — lexer fully generated from grammar tables in `parser.c`
- `dfa-partial` — DFA path is available, but grammar needs external-scanner behavior not fully registered; runtime may synthesize a subset of external tokens
- `token_source` — parser uses a hand-written pure-Go lexer bridge for that grammar

`degraded` means the language parses and produces a tree, but some external-scanner-dependent tokens may be misclassified. `elixir` and `yaml` are the current cases.

---

## Query API

| Feature | Status |
|---|---|
| Compile + execute (`NewQuery`, `Execute`, `ExecuteNode`) | ✅ |
| Cursor streaming (`Exec`, `NextMatch`, `NextCapture`) | ✅ |
| Structural quantifiers (`?`, `*`, `+`) | ✅ |
| `#eq?` | ✅ |
| `#match?` | ✅ |
| `#any-of?` | ✅ |
| `#not-eq?` | ✅ |

---

## Adding a Language

The manifest pipeline is the repeatable path for adding grammar support.

**1.** Add the grammar repo to `grammars/languages.manifest`.

**2.** Generate bindings:

```sh
go run ./cmd/ts2go -manifest grammars/languages.manifest -outdir ./grammars -package grammars
```

**3.** Add smoke samples:
- `cmd/parity_report/main.go`
- `grammars/parse_support_test.go`

**4.** Verify:

```sh
go run ./cmd/parity_report
```

`graphql` and `hcl` have generated bindings but are missing highlight query stubs from their upstream repos — PRs welcome.

---

## Why No CGo?

CGo adds build complexity, blocks trivial cross-compilation to WASM, and requires a C toolchain in every consumer environment. This runtime is implemented entirely in Go against the same parse table format tree-sitter uses.

---

## Status

Pre-v0.1.0. API is stabilizing. Breaking changes will be noted in releases.
