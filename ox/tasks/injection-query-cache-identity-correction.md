# Correct the injected query cache diff

Return only an applicable Git unified diff. Its first bytes must be
`diff --git `. Its final byte must be a newline.

Do not use Markdown fences, prose, fake index hashes, or incomplete hunks.
You have no tools. Use exact base
`f279549dcc4b9865c9769010535dc601f15c8bea`.

The prior response had the correct bounded intent. It combined `cacheKey` with
the exact `querySource` and added a focused test. The reviewer did not apply it.
An unnecessary import rewrite had a wrong hunk count.

Emit a new complete diff with these constraints:

- Do not change the import declaration. The production change needs no import.
- Change only `childHighlightQuery` in `highlight_injections_exec.go`.
- Make the map key include `cacheKey` and the exact `querySource`.
- Reuse one compiled query for repeated identical key-source pairs.
- Do not reuse a compiled query for different sources with one language key.
- Add `highlight_injections_exec_test.go` with the focused regression below.
- Do not change APIs, dependencies, other code, or other tests.

Exact current production function:

```go
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

Add this exact test in a new file:

```go
package gotreesitter

import "testing"

func TestChildHighlightQueryCacheIncludesQuerySource(t *testing.T) {
	lang := queryTestLanguage()
	h := &Highlighter{childQueries: make(map[string]*Query)}

	first, ok := h.childHighlightQuery("shared-language", `(identifier) @first`, lang)
	if !ok {
		t.Fatal("first child query did not compile")
	}
	firstAgain, ok := h.childHighlightQuery("shared-language", `(identifier) @first`, lang)
	if !ok {
		t.Fatal("repeated child query did not compile")
	}
	if firstAgain != first {
		t.Fatal("identical resolver query sources did not reuse one cached query")
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

Use complete hunks with correct counts. The reviewer will run tests.
