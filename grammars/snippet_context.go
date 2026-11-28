package grammars

import "strings"

// SnippetContexts maps language names to a template that places a short code
// snippet where the grammar accepts statements or expressions. The marker
// SnippetPlaceholder stands for the snippet. A snippet that is not a valid
// top-level construct (for example a call, which some grammars read as a
// conversion or a syntax error at the top level) parses as written inside the
// template. Languages not listed parse the bare snippet.
var SnippetContexts = map[string]string{
	"go": "package p\n\nfunc f() {\n\t" + SnippetPlaceholder + "\n}\n",
}

// SnippetPlaceholder marks the snippet's position in a snippet context.
const SnippetPlaceholder = "$$"

// SnippetContext returns the snippet context of a language, or "" when it has
// none.
func SnippetContext(name string) string {
	ctx := SnippetContexts[name]
	if !strings.Contains(ctx, SnippetPlaceholder) {
		return ""
	}
	return ctx
}
