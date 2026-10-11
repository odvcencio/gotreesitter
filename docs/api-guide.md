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

Go declaration, signature and call-argument facts are opt-in. Attach the grammar's
rows with parallel options; `FactAll` still selects only the original four kinds:

```go
entry := grammars.DetectLanguage("main.go")
lang := entry.Language()
program, err := gotreesitter.NewFactProgramWithOptions(lang,
    gotreesitter.FactDeclarations | gotreesitter.FactSignatures | gotreesitter.FactCallArguments,
    gotreesitter.WithDeclarationRules(grammars.DeclarationRules(*entry)),
    gotreesitter.WithSignatureRules(grammars.SignatureRules(*entry)),
    gotreesitter.WithCallArgumentRules(grammars.CallArgumentRules(*entry)))
if err != nil {
    return err
}
facts := program.Extract(tree)
```

For Go, `Declarations` includes package variables and constants, type shapes and aliases,
struct fields, interface methods, and embedded interface types (kind `embed`).
Embedded struct fields set `Embedded` and use the base type identifier as their
name: `io.Reader`, `*Base` and `pkg.Box[int]` name `Reader`, `Base` and `Box`.
Interface unions, approximations and predeclared constraint terms such as
`comparable` are omitted. These are syntax facts; they do not resolve a name's
binding or decide whether a user-defined type implements an interface.

Anonymous struct members use dotted containers. In
`type Config struct { Server struct { Host string } }`, `Host` has container
`Config.Server`; its container range covers the `Server` field declaration.
Package-level `var cfg struct { Debug bool }` gives `Debug` container `cfg` and a
container range covering the variable spec. Grouped names produce sibling
containers. Local variables and fields without a named syntax owner are omitted.

For Go, `Signatures` has one entry per function declaration, receiver method and interface
method. It preserves declaration/name ranges and receiver, type parameter,
parameter and result slices. A method's container is its receiver's base type;
`func (l *List[T]) Push` has container `List`. An interface method uses its
interface's container. Function literals and function types have no signatures.

Each `ParameterFact` preserves the exact source type text and byte range. Grouped
names (`a, b int`) share the declaration and type ranges; unnamed parameters and
results have empty names and zero name ranges. A single result type uses its type
range as the declaration range. `Variadic` records `...T`; `Type` and its range
cover `T`, while the declaration range includes the ellipsis. `Pointer` is set
only for pointer receivers. Type constraints are preserved as written.

`CallArguments` reports direct arguments in source order, with a zero-based
`Index`, a normalized `Kind`, the real `NodeType`, and the argument's range.
`CallStartByte` and `CallEndByte` equal the corresponding `CallRef` range even
when `FactCalls` is off. Ordinary call arguments join exactly one call. JavaScript,
TypeScript and TSX constructor arguments are standalone: their call range covers
the `new_expression`, and they join no `CallRef`. A zero-argument call or
constructor has no argument entries. Nested calls have their own ranges and argument lists.
For `args...`, `Spread` is true, the range includes the ellipsis and `NodeType`
is `variadic_argument`; `Kind` classifies the wrapped expression.

Grammar rows own the normalized kinds: `identifier`, `selector`, `call`,
`composite_literal`, `func_literal`, `literal`, `unary`, `binary`, `index`, `slice`,
`type_assertion`, `parenthesized`, `type`, `keyword`, and `other`. Python uses
`keyword` for `name=value`, without classifying the value separately. Type operands in `make`
and `new` use `type`; numeric, string, raw string, rune, boolean and nil nodes
use `literal`. Unsupported grammar symbols or fields drop the affected rule.
No types, bindings or cross-file relationships are inferred.

Each option replaces earlier rows for the same kind. Without the corresponding
bit, no rules are retained, instructions compiled, or extraction pass added.
`ExtractInto` clears and reuses the outer result slices. Signature parameter
slices are freshly allocated on each extraction; clearing old signatures releases
their nested slices. Results share the destination's outer storage, so copy them
before extracting again if they need to survive. Go, Python, JavaScript,
TypeScript and TSX supply rows. Unsupported languages return no rows.
`Shape` is set only on type declarations; member modifiers are not recorded.


### Python

`grammars.DetectLanguage("main.py")` supplies all three row sets.

- `Declarations` covers module and class-body assignments, annotations, tuple/list
  unpacking, starred targets and assignment chains of up to three targets.
  Module names are `variable`; class-body names are `field`; classes are `type`
  with shape `class`; class-body functions are `method`. Nested class containers
  use dotted names. Python has no constant facts. `_`, attribute/subscript
  targets, augmented assignments, locals, conditional bindings and classes
  nested in functions have no declaration facts.
- `Signatures` covers every `def`, including async and decorated declarations.
  Methods in supported module/class scopes name their class container; other
  functions have no container. `self` and `cls` remain parameters. `*args` and
  `**kw` set `Variadic` and keep their stars in `Name`; annotations fill `Type`.
  Defaults and decorators are omitted. `/` and bare `*` separators are skipped;
  a return annotation is the single result.
- `CallArguments` includes `keyword` for `name=value`. `*x` and `**x` set
  `Spread`, retain the wrapper node/range and classify the operand. A bare
  generator expression without an argument-list node has no argument entries.

### JavaScript

`grammars.DetectLanguage("main.js")` supplies all three row sets.

- `Declarations` covers program-level and exported `const`, `let` and `var`
  names, named classes, and public/private class fields and methods.
  `const` uses `constant`; `let` and `var` use `variable`; classes use `type`
  with shape `class`. Members name their class container. Static methods,
  getters and setters remain `method` with empty `Shape`.
- `Signatures` covers program-level function/generator declarations, named class
  methods and program-level `const` bindings of arrow/function/generator values.
  Single arrow parameters, defaults and rest are supported. Destructured
  parameters use the pattern text as `Name`; all types remain empty.
- `CallArguments` covers ordinary and optional calls and standalone constructor
  arguments. Spread keeps its full wrapper range. Unmapped expressions use
  `other`, including a constructor used as an argument.

Local and conditional bindings, destructured declarations, class expressions,
computed members, object-literal methods, unbound function expressions and
nested function declarations have no matching declaration/signature rows.
Static/getter/setter modifiers, defaults and decorators are not recorded.

### TypeScript and TSX

`grammars.DetectLanguage("main.ts")` and `"main.tsx"` supply rows for all three
kinds, with the same shapes in both grammars.

`Declarations` covers program-level bindings (including `export` and `declare`).
`const` bindings have kind `constant`; `let` and `var` bindings have kind
`variable`. Classes and abstract classes have kind `type` and shape `class`,
interfaces `interface`, `type X = ...` aliases `alias`, and enums `enum`.
Members carry the owner as `Container`: class fields (`field`), methods, getters,
setters, overload signatures and abstract methods (`method`), interface
properties (`field`) and methods (`method`), and enum members (`constant`).
`interface A extends B, ns.C<T>` emits kind `embed` facts named `B` and `C`
with container `A`; they share the `extends` clause range. Locals, namespaces,
class expressions, destructured bindings, computed member names and constructor
parameter properties (`constructor(public x: T)`) have no declaration facts,
though the constructor signature still includes the parameter.

`Signatures` covers function and generator declarations, overload and `declare`
function signatures, class methods and interface methods, and arrow or function
expressions bound by a program-level `const` (the fact spans the declarator).
`Type` is the annotation without the colon; an unannotated parameter has an
empty `Type` and zero type ranges. `Optional` is set only for TypeScript `?`.
A parameter written `x?: T` sets `Optional`;
a default value (`x = 1`) does not. `...rest: T[]` sets `Variadic`. A
destructured or `this` parameter is kept with an empty `Name`. Type parameters
use their `extends` constraint as `Type`. Call and construct signatures, function
types and object-literal methods have no signatures. Named nested functions and
local class methods do have signatures; class-expression methods carry no
container. TSX uses the same rows but excludes angle-bracket assertions, which
conflict with JSX. JSX arguments keep their real node types and use `other`.

`CallArguments` covers `call_expression` arguments. Type arguments (`f<T>(x)`)
and tagged templates are not value arguments. `new` expressions are not
`CallRef`s; their arguments are standalone facts with the constructor range. `spread_element` sets `Spread`.
Besides the Go kinds, `object` and `array` literals use `composite_literal`,
`as`, `satisfies` and `<T>x` use `type_assertion`, and `new X()` uses `call`.


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
