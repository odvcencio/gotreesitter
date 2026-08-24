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
