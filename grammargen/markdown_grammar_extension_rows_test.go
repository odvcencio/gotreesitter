package grammargen

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// markdownExtensionRowsEnv opts in to TestMarkdownGrammarExtensionExternalRows.
// The test pins a known table limitation and fails on the current builder, so
// it is skipped by default to keep the grammargen CI job green.
const markdownExtensionRowsEnv = "GTS_RUN_TABLE_LIMIT_REPRO"

// markdownExtensionRowsCase is one MarkdownGrammar extension whose new block
// body reuses the shared `_newline` and `_line` rules, the way the upstream
// tree-sitter-markdown grammar writes its fenced code block body.
type markdownExtensionRowsCase struct {
	name   string
	extend func(g *Grammar)
	// inputs maps a source text to its expected tree. An empty expected tree
	// means "the bundled MarkdownLanguage() tree for the same text".
	inputs []markdownExtensionRowsInput
}

type markdownExtensionRowsInput struct {
	src  string
	want string
}

// markdownBlockMathExtension adds a `$$ ... $$` block whose body is
// repeat1(choice(_line, _newline)).
func markdownBlockMathExtension(g *Grammar) {
	g.Define("block_math", Seq(
		Alias(Str("$$"), "block_math_delimiter", true),
		Sym("_newline"),
		Choice(Alias(Repeat1(Choice(Sym("_line"), Sym("_newline"))), "math_content", true), Blank()),
		Alias(Str("$$"), "block_math_delimiter", true),
		Choice(Sym("_newline"), Sym("_eof"))))
	for _, name := range []string{"_block_not_section", "_block_in_container"} {
		r := g.Rules[name]
		r.Children = append(r.Children, Sym("block_math"))
	}
}

// markdownUpstreamFenceExtension restores the upstream form of the fenced
// code block: its newlines and body lines use the shared `_newline` and
// `_line` rules instead of MarkdownGrammar's private copies.
func markdownUpstreamFenceExtension(g *Grammar) {
	rename := map[string]string{
		"_fenced_code_block_newline": "_newline",
		"_fenced_code_block_line":    "_line",
	}
	var walk func(r *Rule)
	walk = func(r *Rule) {
		if r == nil {
			return
		}
		if r.Kind == RuleSymbol {
			if to, ok := rename[r.Value]; ok {
				r.Value = to
			}
		}
		for _, c := range r.Children {
			walk(c)
		}
	}
	for _, name := range []string{"_fenced_code_block", "code_fence_content"} {
		walk(g.Rules[name])
	}
}

const markdownBlockMathTree = "(document (section (block_math (block_math_delimiter) (math_content) (block_math_delimiter))))"

var markdownExtensionRowsCases = []markdownExtensionRowsCase{
	{
		name:   "block_math_body",
		extend: markdownBlockMathExtension,
		// Expected trees: tree-sitter's generator builds tables for this
		// grammar (exported with ExportGrammarJSON) that give these trees
		// with the same Go scanner and runtime.
		inputs: []markdownExtensionRowsInput{
			{"$$\nx\n$$\n", markdownBlockMathTree},
			{"$$\n- x\n$$\n", markdownBlockMathTree},
			{"$$\n> x\n$$\n", markdownBlockMathTree},
			{"$$\n---\n$$\n", markdownBlockMathTree},
			{"p\n\n$$\n1. x\n$$\n", "(document (section (paragraph (inline)) (block_math (block_math_delimiter) (math_content) (block_math_delimiter))))"},
		},
	},
	{
		name:   "upstream_fenced_code_body",
		extend: markdownUpstreamFenceExtension,
		inputs: []markdownExtensionRowsInput{
			{"```\nx\n```\n", ""},
			{"```\n- x\n```\n", ""},
			{"```\n> x\n```\n", ""},
			{"```\n---\n```\n", ""},
		},
	},
}

// markdownRowRecorder wraps an external scanner and records, in order with
// the parser's token log, the lookahead rune and the external tokens that
// the parse state offered on each Scan call.
type markdownRowRecorder struct {
	inner  gotreesitter.ExternalScanner
	lang   *gotreesitter.Language
	events *[]markdownRowEvent
}

// markdownRowEvent is either a Scan call (scan is true) or a token that the
// parser consumed (tokenEnd is its end byte).
type markdownRowEvent struct {
	scan      bool
	lookahead rune
	valid     []string
	tokenEnd  int
}

func (r markdownRowRecorder) Create() any                     { return r.inner.Create() }
func (r markdownRowRecorder) Destroy(p any)                   { r.inner.Destroy(p) }
func (r markdownRowRecorder) Serialize(p any, buf []byte) int { return r.inner.Serialize(p, buf) }
func (r markdownRowRecorder) Deserialize(p any, buf []byte)   { r.inner.Deserialize(p, buf) }
func (r markdownRowRecorder) Scan(p any, lexer *gotreesitter.ExternalLexer, valid []bool) bool {
	ev := markdownRowEvent{scan: true, lookahead: lexer.Lookahead(), tokenEnd: -1}
	for i, ok := range valid {
		if ok && i < len(r.lang.ExternalSymbols) {
			ev.valid = append(ev.valid, r.lang.SymbolNames[r.lang.ExternalSymbols[i]])
		}
	}
	*r.events = append(*r.events, ev)
	return r.inner.Scan(p, lexer, valid)
}

// markdownBodyLineRow returns the external tokens that the parse state
// offered when the scanner was first called at the start of the first body
// line, the line after the opening "$$" or "```" line.
func markdownBodyLineRow(t *testing.T, lang *gotreesitter.Language, src string) []string {
	t.Helper()
	open := -1
	for _, delim := range []string{"$$\n", "```\n"} {
		if i := strings.Index(src, delim); i >= 0 && (open < 0 || i < open) {
			open = i + len(delim)
		}
	}
	if open < 0 || open >= len(src) {
		return nil
	}
	first := rune(src[open])
	var events []markdownRowEvent
	inner := lang.ExternalScanner
	lang.ExternalScanner = markdownRowRecorder{inner: inner, lang: lang, events: &events}
	defer func() { lang.ExternalScanner = inner }()
	parser := gotreesitter.NewParser(lang)
	parser.SetLogger(func(kind gotreesitter.ParserLogType, msg string) {
		var sym, start, end int
		if kind != gotreesitter.ParserLogLex || !strings.HasPrefix(msg, "token sym=") {
			return
		}
		if _, err := fmt.Sscanf(msg, "token sym=%d start=%d end=%d", &sym, &start, &end); err == nil {
			events = append(events, markdownRowEvent{tokenEnd: end})
		}
	})
	if _, err := parser.Parse([]byte(src)); err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	reached := false
	for _, ev := range events {
		if !ev.scan {
			if ev.tokenEnd >= open {
				reached = true
			}
			continue
		}
		if reached && ev.lookahead == first {
			sort.Strings(ev.valid)
			return ev.valid
		}
	}
	return nil
}

// TestMarkdownGrammarExtensionExternalRows pins a limitation of grammargen's
// parse tables that blocks extending MarkdownGrammar.
//
// grammargen builds Markdown with LALR(1) item sets, so every state reached
// after the `_line_ending` of the shared `_newline` rule is one state, and its
// external-token row is the union over all of its contexts. Inside a block
// body that row offers the block-start markers of the top level (list, block
// quote and thematic-break markers, `_close_block`, and more), and the Markdown
// scanner returns the first of them that matches the text. tree-sitter's
// generator builds canonical LR(1) states and does not merge a state with one
// that would add an external token, so the same body state offers only the
// tokens that can continue the body.
//
// MarkdownGrammar avoids the shared rules in its own block bodies with private
// copies (`_fenced_code_block_newline`, `_fenced_code_block_line`, and about a
// dozen more) plus name-keyed row suppressions in buildExternalLexStates. An
// extension that writes a block body the way upstream does gets the merged
// row. The block_math case is the kind of construct Markdown++ adds; the
// fenced-code case restores upstream's own rule.
//
// Each case also requires the extended grammar to keep the bundled tree on
// the asserted CommonMark parity inputs, so a fix cannot trade plain Markdown
// for the extension.
func TestMarkdownGrammarExtensionExternalRows(t *testing.T) {
	if os.Getenv(markdownExtensionRowsEnv) != "1" {
		t.Skipf("known table limitation (LALR-merged external-token rows in Markdown block bodies); set %s=1 to run", markdownExtensionRowsEnv)
	}
	refLang := grammars.MarkdownLanguage()
	refParser := gotreesitter.NewParser(refLang)
	refTree := func(src string) string {
		tree, err := refParser.Parse([]byte(src))
		if err != nil {
			t.Fatalf("bundled parse %q: %v", src, err)
		}
		return tree.RootNode().SExpr(refLang)
	}

	for _, tc := range markdownExtensionRowsCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			g := ExtendGrammar("markdown", MarkdownGrammar(), tc.extend)
			ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
			defer cancel()
			lang, err := GenerateLanguageWithContext(ctx, g)
			if err != nil {
				t.Fatalf("GenerateLanguage: %v", err)
			}
			if !grammars.AdaptScannerForLanguage("markdown", lang) {
				t.Fatal("AdaptScannerForLanguage(markdown) returned false")
			}
			parser := gotreesitter.NewParser(lang)

			for _, in := range tc.inputs {
				want := in.want
				if want == "" {
					want = refTree(in.src)
				}
				tree, err := parser.Parse([]byte(in.src))
				if err != nil {
					t.Fatalf("parse %q: %v", in.src, err)
				}
				got := tree.RootNode().SExpr(lang)
				if got != want {
					row := markdownBodyLineRow(t, lang, in.src)
					t.Errorf("%q: block body misparsed by grammargen tables\n  want: %s\n  got:  %s\n  external tokens offered at the start of the body line (%d): %v",
						in.src, want, got, len(row), row)
				}
			}

			for _, pc := range commonMarkParityCorpus {
				if !pc.assert {
					continue
				}
				tree, err := parser.Parse([]byte(pc.src))
				if err != nil {
					t.Fatalf("parse %q: %v", pc.src, err)
				}
				if got, want := tree.RootNode().SExpr(lang), refTree(pc.src); got != want {
					t.Errorf("plain Markdown %s changed under the extension\n  want: %s\n  got:  %s",
						pc.name, truncateSExp(want, 600), truncateSExp(got, 600))
				}
			}
		})
	}
}
