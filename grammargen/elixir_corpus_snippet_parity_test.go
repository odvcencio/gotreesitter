package grammargen

import (
	"os"
	"testing"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestElixirImportedCorpusSnippetParity(t *testing.T) {
	genLang, refLang := loadImportedParityLanguages(t, "elixir")
	assertGeneratedAndReferenceDeepParity(t, genLang, refLang, elixirOperatorLeftAssociativeCorpusBlock)
}

func TestElixirImportedAddSubOperatorParity(t *testing.T) {
	genLang, refLang := loadImportedParityLanguages(t, "elixir")
	assertGeneratedAndReferenceDeepParity(t, genLang, refLang, "a + b + c\na - b - c\n")
}

func TestElixirImportedOperatorsHighlightParity(t *testing.T) {
	genLang, refLang := loadImportedParityLanguages(t, "elixir")
	assertGeneratedAndReferenceDeepParity(t, genLang, refLang, elixirOperatorsHighlightCorpusBlock)
}

func TestElixirImportedGuardedDefDoBlockParity(t *testing.T) {
	genLang, refLang := loadImportedParityLanguages(t, "elixir")
	assertGeneratedAndReferenceDeepParity(t, genLang, refLang, "defmodule M do\n  def func(x) when is_integer(x) do\n    priv(x) + priv(x)\n  end\nend\n")
}

func TestElixirImportedBareCallListArgumentParity(t *testing.T) {
	genLang, refLang := loadImportedParityLanguages(t, "elixir")
	for _, tc := range []struct {
		name string
		src  string
	}{
		{
			name: "bare_call_list_argument",
			src:  "defexception [:message]\n",
		},
		{
			name: "bare_call_list_argument_in_do_block",
			src:  "defmodule M do\n  defexception [:message]\nend\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertGeneratedAndReferenceDeepParity(t, genLang, refLang, tc.src)
		})
	}
}

func TestElixirImportedBareCallKeywordArgumentParity(t *testing.T) {
	genLang, refLang := loadImportedParityLanguages(t, "elixir")
	for _, tc := range []struct {
		name string
		src  string
	}{
		{
			name: "bare_call_keyword_argument",
			src:  "defstruct items: []\n",
		},
		{
			name: "bare_call_keyword_argument_in_do_block",
			src:  "defmodule M do\n  defstruct items: []\nend\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertGeneratedAndReferenceDeepParity(t, genLang, refLang, tc.src)
		})
	}
}

func TestElixirImportedForReduceDoBlockParity(t *testing.T) {
	genLang, refLang := loadImportedParityLanguages(t, "elixir")
	assertGeneratedAndReferenceDeepParity(t, genLang, refLang, "for x <- [1, 2, 1], reduce: %{} do\n  acc -> Map.update(acc, x, 1, & &1 + 1)\nend\n")
}

func TestElixirImportedDoEndStabClauseBodyParity(t *testing.T) {
	genLang, refLang := loadImportedParityLanguages(t, "elixir")
	assertGeneratedAndReferenceDeepParity(t, genLang, refLang, elixirDoEndStabClauseBodyCorpusBlock)
}

func TestElixirImportedQuotedInterpolationDeepParity(t *testing.T) {
	assertImportedDeepParityCases(t, "elixir", []struct {
		name string
		src  string
	}{
		{"quoted_atom", ":\"with #{1 + 1} interpol\"\n"},
		{"string", "\"with #{1 + 1} interpol\"\n"},
		{"quoted_keyword", "[\"with #{1 + 1} interpol\": 1]\n"},
	})
}

func TestElixirImportedRemoteCallOperatorRegressionParity(t *testing.T) {
	genLang, refLang := loadImportedParityLanguages(t, "elixir")
	for _, tc := range []struct {
		name string
		src  string
	}{
		{
			name: "pipe_remote_call_without_parentheses",
			src:  "\"hello\" |> String.upcase |> String.downcase()\n",
		},
		{
			name: "remote_call_negative_unary_argument",
			src:  "Mod.fun -1\n",
		},
		{
			name: "remote_call_positive_unary_argument",
			src:  "Mod.fun +1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertGeneratedAndReferenceDeepParity(t, genLang, refLang, tc.src)
		})
	}
}

func TestElixirImportedCaseRemoteDotDoBlockParity(t *testing.T) {
	genLang, refLang := loadImportedParityLanguages(t, "elixir")
	assertGeneratedAndReferenceDeepParity(t, genLang, refLang, "case __ENV__.line do\n  x when is_integer(x) -> x\n  x when x in 1..12 -> -x\nend\n")
}

func TestElixirImportedLRSplitCorpusSnippetParity(t *testing.T) {
	genLang, refLang := loadImportedElixirLRSplitParityLanguages(t)
	assertGeneratedAndReferenceDeepParity(t, genLang, refLang, elixirOperatorLeftAssociativeCorpusBlock)
}

const elixirOperatorLeftAssociativeCorpusBlock = "a ** b ** c\n\n" +
	"a * b * c\n" +
	"a / b / c\n\n" +
	"a + b + c\n" +
	"a - b - c\n\n" +
	"a ^^^ b ^^^ c\n\n" +
	"a in b in c\n" +
	"a not in b not in c\n\n" +
	"a |> b |> c\n" +
	"a <<< b <<< c\n" +
	"a >>> b >>> c\n" +
	"a <<~ b <<~ c\n" +
	"a ~>> b ~>> c\n" +
	"a <~ b <~ c\n" +
	"a ~> b ~> c\n" +
	"a <~> b <~> c\n" +
	"a <|> b <|> c\n\n" +
	"a < b < c\n" +
	"a > b > c\n" +
	"a <= b <= c\n" +
	"a >= b >= c\n\n" +
	"a == b == c\n" +
	"a != b != c\n" +
	"a =~ b =~ c\n" +
	"a === b === c\n" +
	"a !== b !== c\n\n" +
	"a && b && c\n" +
	"a &&& b &&& c\n" +
	"a and b and c\n\n" +
	"a || b || c\n" +
	"a ||| b ||| c\n" +
	"a or b or c\n\n" +
	"a <- b <- c\n" +
	"a \\\\ b \\\\ c\n"

const elixirOperatorsHighlightCorpusBlock = "a in b\n" +
	"# <- variable\n" +
	"# ^ keyword\n" +
	"#    ^ variable\n" +
	"\n" +
	"a not in b\n" +
	"# <- variable\n" +
	"# ^ keyword\n" +
	"#     ^ keyword\n" +
	"#        ^ variable\n" +
	"\n" +
	"a not  in b\n" +
	"# <- variable\n" +
	"# ^ keyword\n" +
	"#      ^ keyword\n" +
	"#         ^ variable\n" +
	"\n" +
	"a ~>> b = bind(a, b)\n" +
	"# <- variable\n" +
	"#  ^ operator\n" +
	"#     ^ variable\n" +
	"#       ^ operator\n" +
	"#         ^ function\n" +
	"#             ^ punctuation.bracket\n" +
	"#              ^ variable\n" +
	"#               ^ punctuation.delimiter\n" +
	"#                 ^ variable\n" +
	"#                  ^ punctuation.bracket\n" +
	"\n" +
	"a ~> b\n" +
	"# ^ operator\n" +
	"\n" +
	"a + b\n" +
	"# ^ operator\n" +
	"\n" +
	"... == !x && y || z\n" +
	"# <- variable\n" +
	"#   ^ operator\n" +
	"#      ^ operator\n" +
	"#       ^ variable\n" +
	"#         ^ operator\n" +
	"#            ^ variable\n" +
	"#              ^ operator\n" +
	"#                 ^ variable\n" +
	"\n" +
	"x = 1 + 2.0 * 3\n" +
	"# <- variable\n" +
	"# ^ operator\n" +
	"#   ^ number\n" +
	"#     ^ operator\n" +
	"#       ^ number\n" +
	"#           ^ operator\n" +
	"#             ^ number\n" +
	"\n" +
	"y = true and false\n" +
	"# <- variable\n" +
	"# ^ operator\n" +
	"#   ^ constant\n" +
	"#         ^ keyword\n" +
	"#             ^ constant\n" +
	"\n" +
	"{ ^z, a } = {true, x}\n" +
	"# <- punctuation.bracket\n" +
	"# ^ operator\n" +
	"#  ^ variable\n" +
	"#   ^ punctuation.delimiter\n" +
	"#     ^ variable\n" +
	"#         ^ operator\n" +
	"#           ^ punctuation.bracket\n" +
	"#            ^ constant\n" +
	"#                ^ punctuation.delimiter\n" +
	"#                  ^ variable\n" +
	"#                   ^ punctuation.bracket\n" +
	"\n" +
	"\"hello\" |> String.upcase |> String.downcase()\n" +
	"# ^ string\n" +
	"#       ^ operator\n" +
	"#          ^ module\n" +
	"#                ^ operator\n" +
	"#                 ^ function\n" +
	"#                        ^ operator\n" +
	"#                           ^ module\n" +
	"#                                 ^ operator\n" +
	"#                                  ^ function\n" +
	"\n" +
	"range = ..\n" +
	"# <- variable\n" +
	"#     ^ operator\n" +
	"#        ^ operator\n"

const elixirDoEndStabClauseBodyCorpusBlock = "fun do\n" +
	"  1 ->\n" +
	"    1\n" +
	"    x\n\n" +
	"  1 ->\n" +
	"    1\n" +
	"end\n\n" +
	"fun do\n" +
	"  1 ->\n" +
	"    1\n" +
	"    Mod.fun\n\n" +
	"  1 ->\n" +
	"    1\n" +
	"end\n\n" +
	"fun do\n" +
	"  1 ->\n" +
	"    1\n" +
	"    mod.fun\n\n" +
	"  1 ->\n" +
	"    1\n" +
	"end\n\n" +
	"fun do\n" +
	"  1 ->\n" +
	"    1\n\n" +
	"  x 1 ->\n" +
	"    1\n" +
	"end\n"

func loadImportedElixirLRSplitParityLanguages(t *testing.T) (*gotreesitter.Language, *gotreesitter.Language) {
	t.Helper()

	var grammarSpec importParityGrammar
	for _, g := range importParityGrammars {
		if g.name == "elixir" {
			grammarSpec = g
			break
		}
	}
	if grammarSpec.name == "" {
		t.Fatal("elixir import parity grammar not found")
	}
	if grammarSpec.jsonPath != "" {
		grammarSpec.jsonPath = fallbackParitySeedPath(grammarSpec.jsonPath)
		if _, err := os.Stat(grammarSpec.jsonPath); err != nil {
			t.Skipf("elixir grammar.json not available: %v", err)
		}
	} else if grammarSpec.path != "" {
		grammarSpec.path = fallbackParitySeedPath(grammarSpec.path)
		if _, err := os.Stat(grammarSpec.path); err != nil {
			t.Skipf("elixir grammar.js not available: %v", err)
		}
	}

	gram, err := importParityGrammarSource(grammarSpec)
	if err != nil {
		t.Fatalf("import elixir grammar: %v", err)
	}
	gram.EnableLRSplitting = true

	timeout := grammarSpec.genTimeout
	if timeout == 0 {
		timeout = 180 * time.Second
	}
	genLang, err := generateWithTimeout(gram, timeout)
	if err != nil {
		t.Fatalf("generate elixir language: %v", err)
	}

	refLang := grammarSpec.blobFunc()
	adaptExternalScanner(refLang, genLang)
	return genLang, refLang
}
