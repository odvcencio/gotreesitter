# Preserve injected highlight query identity

This is a one-use patch request. You have no interactive tools.

Return only a complete Git unified diff. Its first bytes must be `diff --git `.
End the response with a newline. Do not use Markdown fences or prose.

Use exact base `cd444724807f58ad19e1fe8bc772de04e66e15d0`.

## Defect

The public injection resolver returns a child language and a highlight query.
The cache uses only the derived language key. It ignores the returned query.

Two resolver results can use the same language with different query sources.
The second result then reuses the first compiled query and emits wrong captures.

The following focused probe fails on this base:

```go
func TestChildHighlightQueryCacheIncludesQuerySource(t *testing.T) {
	lang := queryTestLanguage()
	h := &Highlighter{childQueries: make(map[string]*Query)}

	first, ok := h.childHighlightQuery("shared-language", `(identifier) @first`, lang)
	if !ok {
		t.Fatal("first child query did not compile")
	}
	second, ok := h.childHighlightQuery("shared-language", `(identifier) @second`, lang)
	if !ok {
		t.Fatal("second child query did not compile")
	}
	if first == second {
		t.Fatal("different resolver query sources reused one cached query")
	}

	matches := second.Execute(buildSimpleTree(lang))
	if len(matches) != 1 || len(matches[0].Captures) != 1 || matches[0].Captures[0].Name != "second" {
		t.Fatalf("second query captures = %#v, want @second", matches)
	}
}
```

Observed failure:

```text
different resolver query sources reused one cached query
```

## Required change

Make the cache identity include both the language key and the exact query
source. Keep reuse for repeated identical pairs. Add the focused regression.

Keep the change small. Do not change parsing, injection range logic, public
APIs, dependencies, generated files, or unrelated tests.

Change only `highlight_injections_exec.go` and one focused test file. A new
`highlight_injections_exec_test.go` file is acceptable.

## Exact production source

The relevant context in `highlight_injections_exec.go` is:

```go
package gotreesitter

import "strings"

type injectedHighlightContext struct {
	lang               *Language
	querySource        string
	tokenSourceFactory func(source []byte) TokenSource
	cacheKey           string
	source             []byte
	start              uint32
}

func injectedHighlightCacheKey(lang *Language, fallback string) string {
	if lang.Name != "" {
		return lang.Name
	}
	return fallback
}

func (h *Highlighter) collectInjectedHighlightRanges(ctx injectedHighlightContext) []HighlightRange {
	childTree, err := h.parseInjectedTree(ctx.lang, ctx.tokenSourceFactory, ctx.source)
	if err != nil || childTree == nil || childTree.RootNode() == nil {
		if childTree != nil {
			childTree.Release()
		}
		return nil
	}
	defer childTree.Release()

	childQuery, ok := h.childHighlightQuery(ctx.cacheKey, ctx.querySource, ctx.lang)
	if !ok {
		return nil
	}
	childRanges := collectHighlightRanges(childQuery, childTree)
	if len(childRanges) == 0 && ctx.cacheKey == "go" {
		childRanges = h.collectWrappedGoHighlightRanges(ctx, childQuery)
	}
	return childRanges
}

func (h *Highlighter) childHighlightQuery(cacheKey string, querySource string, lang *Language) (*Query, bool) {
	childQuery := h.childQueries[cacheKey]
	if childQuery != nil {
		return childQuery, true
	}
	childQuery, err := NewQuery(querySource, lang)
	if err != nil {
		return nil, false
	}
	h.childQueries[cacheKey] = childQuery
	return childQuery, true
}
```

The `Highlighter` owns this existing field:

```go
	childQueries map[string]*Query
```

`NewHighlighter` initializes it when injection support exists:

```go
			h.childQueries = make(map[string]*Query)
```

## Exact test helpers

Tests in package `gotreesitter` can call these existing helpers:

```go
func queryTestLanguage() *Language
func buildSimpleTree(lang *Language) *Tree
```

`buildSimpleTree` returns one `identifier` named `main`. A query
`(identifier) @second` returns one match with one capture named `second`.

Run formatting mentally. The reviewer will run all tests.
