# Architecture

gotreesitter is a ground-up reimplementation of the tree-sitter runtime in Go. No code is shared with, or translated from, the C implementation.

**Parser** — Table-driven LR(1) with GLR fallback. When a `(state, symbol)` pair maps to multiple actions in the parse table, the parser forks the stack and explores all alternatives in parallel. Stack merging collapses equivalent paths. Safety limits (iteration count, stack depth, node count) scale with input size and prevent runaway exploration on ambiguous grammars.

**Incremental engine** — Walks the previous tree and reuses unchanged content
by reference. Reuse includes unchanged top-level siblings and bounded nested
nonterminals with authenticated compact dependency proofs. Length or point changes
still require coordinate updates in affected trailing subtrees. This is not an
absolute `O(edit)` guarantee. The v0.52.0 release disables the unsafe same-length
token-invariant shortcut while preserving ordinary subtree reuse and no-edit reuse.
The unreleased implementation restores this shortcut with authenticated lexical dependencies.
[Pull request #1093](https://github.com/odvcencio/gotreesitter/pull/1093) closed
[issue #1087](https://github.com/odvcencio/gotreesitter/issues/1087).
General reuse with external scanners requires explicit certification, with
boundary checkpoints where configured. Uncertified cases use the legacy
full-parse fallback documented in the
[scanner matrix](external-scanners.md#incremental-reuse-certification-matrix).

**Lexer** — Two paths. `ts2go` generates a DFA lexer from the grammar's lex tables, and it handles most languages. For grammars where the DFA is not enough (for example, Go's automatic semicolons, or YAML's indentation-sensitive structure), hand-written Go token sources implement the `TokenSource` interface directly.

**External scanners** — 119 registered grammars require external scanners for context-sensitive tokens.
Each scanner implements the grammar's `ExternalScanner` interface: `Create`, `Serialize`, `Deserialize`, and `Scan`.
Certified checkpoint-enabled scanners save state at external-token boundaries for incremental reuse.
Uncertified changed-edit cases use a fresh production parse.
The v0.52.0 release disables the former same-length token-invariant exception.
See the [per-language matrix](external-scanners.md#incremental-reuse-certification-matrix) for certification and fallback behavior.

**Arena allocator** — Nodes are allocated from slab-based arenas to reduce GC pressure. Arenas are released in bulk when a tree is freed.

**Query engine** — S-expression pattern compiler with predicate evaluation and streaming cursor iteration. It supports all standard tree-sitter predicates (`#eq?`, `#match?`, `#any-of?`, `#has-ancestor?`, and similar predicates) and directive annotations (`#set!`, `#offset!`, `#select-adjacent!`, `#strip!`).

**Injection parser** — Orchestrates multi-language parsing. It runs injection queries against a parent tree to find embedded regions, spawns child parsers with `SetIncludedRanges()`, and recurses for nested injections. Incremental reparse reuses unchanged child trees.

**Rewriter** — Collects source-level edits (replace, insert, delete) targeting byte ranges, applies them atomically, and produces `InputEdit` records for incremental reparse. It validates edits for non-overlap and applies them in a single pass.

**Grammar loading** — `ts2go` extracts parse tables, lex tables, field maps, symbol metadata, and external token lists from upstream `parser.c` files. These are serialized to compressed binary blobs under `grammars/grammar_blobs/` and lazy-loaded through `loadEmbeddedLanguage()` with an LRU cache. String and transition interning reduce memory footprint across loaded grammars. Grammargen-backed blobs use the same CLI surface; for example, you can regenerate the Go blob with `go run ./cmd/grammargen -bin grammars/grammar_blobs/go.bin go`. Do not pass `-lr-split` for Go: it interacts with the external ASI scanner symbol and produces a table that misparses ordinary single-line if-statements (see [grammargen/README.md](../grammargen/README.md#authoring-commands) and [docs/grammar-ownership.md](grammar-ownership.md)). When loading a raw blob yourself, prefer `grammars.LoadLanguage(name, blob)` over `gotreesitter.LoadLanguage(blob)`, so the runtime attaches the registered external scanner and external lex-state support for that language automatically.

**Standalone grammars** — Import `grammars/<name>` to select a grammar without
the aggregate catalog. All 206 blob-backed grammars provide this API.
These packages work with ordinary `go install` and need no build tags.

```go
import python "github.com/odvcencio/gotreesitter/grammars/python"

parser := gotreesitter.NewParser(python.Language())
```

The linker retains only the selected grammar blobs. The shared `grammars/runtime`
package supplies scanners, decoder repairs, cache controls, and exact runtime profiles.
For example, import `grammars/javascript` and call `javascript.Language()` for JavaScript.
Use the alias `golang` for the `grammars/go` package.

The native `grammars/lean` package also avoids the aggregate catalog.
Import both Lean and `grammars` when you need automatic `.lean` detection.
Existing aggregate imports and build tags remain supported.
When both APIs are imported, the aggregate catalog selects the shared blob source.
This preserves external blob configuration and its errors.

Regenerate package wrappers after you add or update a blob:

```sh
go run ./cmd/gen_grammar_packages
go run ./cmd/gen_grammar_packages -check
```

See [docs/build-tags.md](build-tags.md) for the build-tag-selected embedding modes, and [docs/repository-map.md](repository-map.md) for a file-level map of the root package.
