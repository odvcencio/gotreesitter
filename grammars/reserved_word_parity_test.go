package grammars

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// reservedWordParityLanguages are the six languages whose checked-in blob
// predates cmd/ts2go's ABI 15 reserved-word extraction (extractReservedWords)
// and now carries its reserved-word table through a
// runtime/*_reserved_words_gen.go sidecar (cmd/ts2go -reservedwords-only).
var reservedWordParityLanguages = []string{
	"javascript",
	"python",
	"php",
	"ocaml",
	"pkl",
	"templ",
}

// parseReservedWordParitySource parses src with name's registered backend
// and reports whether the resulting tree has any error node.
func parseReservedWordParitySource(t *testing.T, name, src string) bool {
	t.Helper()
	entry, ok := findLangEntry(name)
	if !ok {
		t.Fatalf("language %q not registered", name)
	}
	lang := entry.Language()
	report := EvaluateParseSupport(entry, lang)
	parser := gotreesitter.NewParser(lang)
	source := []byte(src)

	var (
		tree *gotreesitter.Tree
		err  error
	)
	switch report.Backend {
	case ParseBackendTokenSource:
		if entry.TokenSourceFactory == nil {
			t.Fatalf("%s: token source backend without factory", name)
		}
		tree, err = parser.ParseWithTokenSource(source, entry.TokenSourceFactory(source, lang))
	case ParseBackendDFA, ParseBackendDFAPartial:
		tree, err = parser.Parse(source)
	default:
		t.Fatalf("%s: unsupported parse backend %q", name, report.Backend)
	}
	if err != nil {
		t.Fatalf("%s: parse failed: %v", name, err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatalf("%s: parse returned nil root", name)
	}
	defer tree.Release()
	return tree.RootNode().HasError()
}

func findLangEntry(name string) (LangEntry, bool) {
	for _, e := range AllLanguages() {
		if e.Name == name {
			return e, true
		}
	}
	return LangEntry{}, false
}

// TestReservedWordAttachedForSixLanguages confirms the runtime attach guard
// (attachRegisteredReservedWords) actually merges the generated sidecar
// table onto each of the six languages' decoded embedded Language: the data
// gap this sidecar closes is silent by construction (a language with no
// reserved words parses exactly like one whose reserved-word table failed
// the attach guard), so this test is the only signal that the sidecar
// pipeline (cmd/ts2go -reservedwords-only through
// grammars/runtime/*_reserved_words_gen.go) reached the embedded language at
// all.
func TestReservedWordAttachedForSixLanguages(t *testing.T) {
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })
	for _, name := range reservedWordParityLanguages {
		name := name
		t.Run(name, func(t *testing.T) {
			entry, ok := findLangEntry(name)
			if !ok {
				t.Fatalf("language %q not registered", name)
			}
			UnloadEmbeddedLanguage(entry.Name + ".bin")
			t.Cleanup(func() { UnloadEmbeddedLanguage(entry.Name + ".bin") })
			lang := entry.Language()
			if len(lang.ReservedWords) == 0 || lang.MaxReservedWordSetSize == 0 {
				t.Fatalf("%s: reserved-word sidecar did not attach (ReservedWords=%d, MaxReservedWordSetSize=%d)",
					name, len(lang.ReservedWords), lang.MaxReservedWordSetSize)
			}
		})
	}
}

// TestJavaScriptReservedWordBlocksVarIf matches C tree-sitter: "if" is a
// reserved word in the state that follows "var", so the parse state has no
// shift/reduce action for it once the fix promotes it to the keyword token.
// Before the promoteKeyword fix, the reserved-word branch returned early
// without setting tok.Symbol, so the parser kept reading "if" as a plain
// identifier and accepted this source cleanly — see the go-treesitter-parity
// background finding: locked C tree-sitter (tree-sitter-javascript
// 58404d8c) reports
// (program (ERROR [0,0]-[0,8]) (expression_statement (number))) for this
// exact source.
func TestJavaScriptReservedWordBlocksVarIf(t *testing.T) {
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })
	if !parseReservedWordParitySource(t, "javascript", "var if = 1;\n") {
		t.Fatal("javascript: \"var if = 1;\" parsed with no error; want a reserved-word parse error, matching C")
	}
}

// TestReservedWordAllowedPositionsStillParseClean confirms the reserved-word
// fix does not newly reject legitimate keyword-as-identifier usage: a
// keyword's own reserved-word set only applies to the specific parse states
// that reference it, and every one of these sources uses a reserved word in
// a parse state that C's grammar (and this fix) both leave alone.
func TestReservedWordAllowedPositionsStillParseClean(t *testing.T) {
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	cases := []struct {
		name string
		lang string
		src  string
	}{
		{
			name: "javascript_member_and_method_name",
			lang: "javascript",
			src:  "obj.if = 1; class A { if() {} }\n",
		},
		{
			name: "php_method_named_like_a_keyword",
			lang: "php",
			src:  "<?php class A { public function default() {} }\n",
		},
		{
			name: "php_method_call_named_like_a_keyword",
			lang: "php",
			src:  "<?php $obj->case();\n",
		},
		{
			name: "python_soft_keyword_as_identifier",
			lang: "python",
			src:  "match = 1\n",
		},
		{
			name: "python_keyword_prefixed_attribute",
			lang: "python",
			src:  "print(x.if_)\n",
		},
		{
			name: "ocaml_ordinary_code",
			lang: "ocaml",
			src:  "let x = 1\n",
		},
		{
			name: "pkl_ordinary_code",
			lang: "pkl",
			src:  "x = 1\n",
		},
		{
			name: "templ_ordinary_code",
			lang: "templ",
			src:  "templ T() { <div>ok</div> }\n",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			if parseReservedWordParitySource(t, c.lang, c.src) {
				t.Fatalf("%s: %q produced an error node; want a clean parse", c.lang, c.src)
			}
		})
	}
}

// TestReservedWordSixLanguageSmokeSamplesStillParseClean confirms the
// embedded ParseSmokeSamples entry for each of the six languages still
// parses with no error node once its reserved-word sidecar is attached.
func TestReservedWordSixLanguageSmokeSamplesStillParseClean(t *testing.T) {
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })
	for _, name := range reservedWordParityLanguages {
		name := name
		t.Run(name, func(t *testing.T) {
			src := ParseSmokeSample(name)
			if parseReservedWordParitySource(t, name, src) {
				t.Fatalf("%s: smoke sample %q produced an error node", name, src)
			}
		})
	}
}
