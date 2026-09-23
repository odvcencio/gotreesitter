# API guide

This page collects the deep usage material that used to live in the README:
strict parsing, tree lookup, code-understanding helpers, the Taproot DSL
harness, queries, typed query codegen, injection parsing, source rewriting,
incremental reparsing, UTF-16 input, the tree cursor, highlighting, tagging,
and file outlines. See [README.md](../README.md) for the ten-line quick
start and [docs/repository-map.md](repository-map.md) for where each
subsystem lives in the source tree.

## Strict parsing and partial trees

The default parse methods preserve tree-sitter's partial-tree behavior. If a
timeout, cancellation flag, token-source EOF, or parser safety limit stops
the parse early, the returned tree records the stop reason and the error
stays nil. This is useful for editors and diagnostics, where a partial tree
still has value.

Use strict methods when partial output should fail a request:

```go
parser.SetTimeoutMicros(50_000)

tree, err := parser.ParseStrict(src)
if errors.Is(err, gotreesitter.ErrParseStoppedEarly) {
    fmt.Println(tree.ParseStopReason())
}
```

Use work limits to select parser thresholds without a wall-clock deadline:

```go
limits := gotreesitter.ParseWorkLimits{
    IterationLimit:  200_000,
    StackDepthLimit: 8_192,
    NodeLimit:       1_000_000,
}
parser.SetParseWorkLimits(limits)

pool := gotreesitter.NewParserPool(lang,
    gotreesitter.WithParserPoolParseWorkLimits(limits))
```

Positive fields replace the corresponding source-derived threshold. Zero or negative fields keep the default.
The thresholds apply to each production parser loop, including recovery parses. They do not count total operation work or allocated bytes.
The parser checks node and depth thresholds between steps. A step can exceed either threshold before the next check.
Stable parse methods bypass compact and forest speculation while a work limit is configured.
This route change can affect performance and tree selection. Clear all fields to restore normal routing.
Timeouts, cancellation, and memory budgets can still stop a parse first.

The per-parse memory budget grows with the input. It is the larger of 512 MiB
and 512 bytes for each input byte, so valid input does not need a manual
setting. Set a fixed budget to cap parser memory, for example in a service
with a memory limit:

```go
parser.SetMemoryBudgetBytes(256 << 20) // 256 MiB for each parse

pool := gotreesitter.NewParserPool(lang,
    gotreesitter.WithParserPoolMemoryBudgetBytes(256<<20))
```

A budget stop returns a partial tree with `ParseStopMemoryBudget`. A negative
budget turns the per-parse budget off. The process-heap ceiling still stops a
runaway parse. It is the larger of 2 GiB and twice the budget.

A threshold stop returns a partial tree with one of these reasons:

- `ParseStopIterationLimit`
- `ParseStopStackDepthLimit`
- `ParseStopNodeLimit`

Strict variants are available for full parse, incremental parse, token-source
parse, factory parse, `ParseWith`, and `ParserPool`.

`grammars.ParseFilePooledStrict` returns a pooled `*BoundTree` only after a
complete parse. The caller must call `Release` on a returned tree. It returns
`nil` and an error for parser setup failures, parse failures, and early stops.

`Tagger.TagStrict` returns tags only after a complete parse. It releases its
internal tree before it returns. It returns `nil` and an error for parser
failures and early stops.

High-level analysis can use `WithHighlighterTimeoutMicros` or
`WithTaggerTimeoutMicros` to bound parser work. The
`HighlightIncrementalStrict` and `TagIncrementalStrict` methods return the
partial tree together with `ErrParseStoppedEarly` and skip running queries
after an early stop.

## Tree lookup helpers

Use `NodeAtByte` or `NamedNodeAtByte` to turn an editor byte offset into a
syntax node:

```go
node := tree.NamedNodeAtByte(offset)
```

The helpers return the smallest matching descendant and handle exact end-byte
boundaries, so callers do not need to hand-roll a tree walk.

## Code-understanding helpers

For hot indexing paths that need common symbols but not arbitrary tags-query
semantics, compile one `FactProgram` and reuse it across trees:

```go
program, err := gotreesitter.NewFactProgram(lang, gotreesitter.FactAll)
if err != nil {
	return err
}
facts := program.Extract(tree)

enclosing, ok := tree.EnclosingDefinition(offset)
```

Call `ExtractInto` to reuse fact storage across trees:

```go
var reusable gotreesitter.FactSet
program.ExtractInto(tree, &reusable)
```

Consume or clone the results before the next extraction replaces them.
Use a separate destination for each concurrent extraction.
Assign `gotreesitter.FactSet{}` to release retained storage.

The compiled program extracts definitions, calls, heritage edges, and imports
during one tree traversal. The individual `ExtractDefinitionSpans`,
`ExtractCalls`, `ExtractHeritage`, and `ExtractImports` APIs remain available.

These APIs cover common Go, JavaScript, TypeScript/TSX, Python, Starlark, and
Java facts. They skip unsupported languages or ambiguous shapes.

## Taproot DSL harness

The `taproot` package is a small front-end harness for grammargen-backed
DSLs. It caches generated or blob-loaded languages, parses source, returns a
`Walker` with common CST helpers, and reports syntax errors while still
returning the partial root for diagnostics:

```go
root, walker, err := taproot.ParseFromBlob("dsl", blob, buildGrammar, src)
if err != nil {
    fmt.Println(err)
}
fmt.Println(walker.Type(root))
```

Use `taproot.LanguageFromBlob` when a DSL embeds a generated grammar blob but
still wants a source-grammar fallback during development.

## Queries

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

The query engine supports the full S-expression pattern language: structural quantifiers (`?`, `*`, `+`), alternation (`[...]`), field constraints, negated fields, anchor (`!`), and all standard predicates. See the [query API table](languages.md#query-api).

## Typed query codegen

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

## Multi-language documents (injection parsing)

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

It supports static (`#set! injection.language "javascript"`) and dynamic (`@injection.language` capture) language detection, recursive nested injections, and incremental reparse with child tree reuse.

## Source rewriting

Collect source-level edits and apply them atomically, producing `InputEdit` records for incremental reparse:

```go
rw := gotreesitter.NewRewriter(src)
rw.Replace(funcNameNode, []byte("newName"))
rw.InsertBefore(bodyNode, []byte("// added\n"))
rw.Delete(unusedNode)

newSrc, _ := rw.ApplyToTree(tree)
newTree, _ := parser.ParseIncremental(newSrc, tree)
```

`Apply()` returns both the new source bytes and the `[]InputEdit` records. `ApplyToTree()` is a convenience method that calls `tree.Edit()` for each edit and returns source ready for `ParseIncremental`.

## Incremental reparsing

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
tree.Release()
defer tree2.Release()
```

Release the old tree and the new tree once each. The new tree can share
nodes with the old tree. `ParseIncremental` updates the parent links of those
shared nodes, so do not read the old tree from another goroutine during the
call. After the call, read the new tree.

`ParseIncremental` walks the old tree's spine, identifies the edit region, and reuses unchanged content by reference. For an admitted clean edit, re-lex/reparse work can stay close to the invalidated region: reuse covers leaf nodes, the unchanged suffix, the untouched root, and unchanged top-level siblings after the edit. Admission for top-level sibling reuse is per node: a fragility bit plus byte-range equality, not a language allowlist.

Parse reuse does not guarantee absolute `O(edit)` work. Length or point changes
also require `Tree.Edit` to update coordinates in affected trailing subtrees.
That maintenance grows with the number of trailing nodes or compact entries
whose coordinates move. Same-length edits with unchanged points avoid that shift.
The compact route also supports bounded nested nonterminal reuse when parser
state, node provenance, and lexer dependency proofs permit it.
The v0.52.0 release disables the unsafe same-length token-invariant shortcut.
Ordinary subtree reuse and no-edit reuse remain available. Restoring the shortcut
requires complete lexical dependency proofs under
[issue #1087](https://github.com/odvcencio/gotreesitter/issues/1087).
The unreleased implementation restores bounded reuse after authenticating earlier
lexical reads and the edited token. Unknown coverage or an exhausted proof
budget requires reparsing. See
[pull request #1093](https://github.com/odvcencio/gotreesitter/pull/1093)
for the restoration and its validation.
External scanners need certification for general old-tree reuse. Unsupported
cases use the legacy full-parse fallback. See the
[per-language incremental scanner matrix](external-scanners.md#incremental-reuse-certification-matrix).

When no edit has occurred, `ParseIncremental` detects the nil-edit on a pointer check and returns in single-digit nanoseconds with zero allocations. It returns the old tree itself and adds a handle to it, so releasing the old tree does not invalidate the result.

## UTF-16 input and editor coordinates

UTF-16 callers can parse Go-native code units or endian-specific byte buffers
without converting offsets by hand. The parser core keeps its canonical UTF-8
view internally, while the returned tree retains the original UTF-16 source
and maps nodes, edits, included ranges, query filters, highlights, tags, and
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

UTF-16 byte input states its byte order explicitly:

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

Node byte APIs such as `DescendantForByteRange` still use the tree's
canonical UTF-8 byte offsets. Use `DescendantForUTF16Range`, or convert with
`UTF8ByteForUTF16Offset` when starting from editor UTF-16 offsets.

## Tree cursor

`TreeCursor` maintains an explicit `(node, childIndex)` frame stack. Parent, child, and sibling movement run in O(1) with zero allocations — sibling traversal indexes directly into the parent's `children[]` slice.

```go
c := gotreesitter.NewTreeCursorFromTree(tree)

c.GotoFirstChild()
c.GotoChildByFieldName("body")

for ok := c.GotoFirstNamedChild(); ok; ok = c.GotoNextNamedSibling() {
    fmt.Printf("%s at %d\n", c.CurrentNodeType(), c.CurrentNode().StartByte())
}

idx := c.GotoFirstChildForByte(128)
```

Movement methods: `GotoFirstChild`, `GotoLastChild`, `GotoNextSibling`, `GotoPrevSibling`, `GotoParent`, named-only variants (for example, `GotoFirstNamedChild`), field-based (`GotoChildByFieldName`, `GotoChildByFieldID`), and position-based (`GotoFirstChildForByte`, `GotoFirstChildForPoint`).

Cursors hold direct pointers into tree nodes. Recreate the cursor after `Tree.Release()`, `Tree.Edit(...)`, or an incremental reparse.

## Highlighting

```go
hl, _ := gotreesitter.NewHighlighter(lang, highlightQuery)
ranges := hl.Highlight(src)

for _, r := range ranges {
    fmt.Printf("%s: %q\n", r.Capture, src[r.StartByte:r.EndByte])
}
```

## Tagging

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

## File outlines

`Outliner` projects a symbol outline from a parsed tree: kind, name, spans, and lexical nesting, from the same tags-query captures `Tagger` uses.

```go
entry := grammars.DetectLanguageByName("go")
lang := entry.Language()
outliner, _ := gotreesitter.NewOutliner(lang, grammars.ResolveTagsQuery(*entry))
symbols, report := outliner.OutlineTree(tree)
for _, sym := range symbols {
    fmt.Println(sym.Kind, sym.Name)
}
```

An ambiguous or unnamed definition is omitted and counted in `report`, never guessed. See [docs/outline.md](outline.md) for the full API, per-language coverage, and the C-oracle differential that guards it.
