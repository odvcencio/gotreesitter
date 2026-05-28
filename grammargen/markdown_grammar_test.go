package grammargen

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestMarkdownGrammarGenerates verifies that MarkdownGrammar() compiles without
// error and produces a language with the expected number of external symbols.
func TestMarkdownGrammarGenerates(t *testing.T) {
	g := MarkdownGrammar()
	if g == nil {
		t.Fatal("MarkdownGrammar() returned nil")
	}
	if g.Name != "markdown" {
		t.Errorf("expected grammar name %q, got %q", "markdown", g.Name)
	}
	if len(g.Rules) == 0 {
		t.Fatal("grammar has no rules")
	}
	if len(g.Externals) != 47 {
		t.Errorf("expected 47 external tokens, got %d", len(g.Externals))
	}
	if g.RuleOrder[0] != "document" {
		t.Errorf("expected first rule to be %q, got %q", "document", g.RuleOrder[0])
	}
	t.Logf("MarkdownGrammar: %d rules, %d externals, %d conflicts",
		len(g.Rules), len(g.Externals), len(g.Conflicts))
}

// TestMarkdownGrammarCompilesAndAdaptsScanner verifies that GenerateLanguage
// succeeds and that AdaptScannerForLanguage attaches the external scanner.
func TestMarkdownGrammarCompilesAndAdaptsScanner(t *testing.T) {
	g := MarkdownGrammar()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	genLang, err := GenerateLanguageWithContext(ctx, g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}
	if genLang == nil {
		t.Fatal("GenerateLanguage returned nil language")
	}
	if len(genLang.ExternalSymbols) == 0 {
		t.Fatal("generated language has no ExternalSymbols; externals likely not wired")
	}
	t.Logf("generated language: ExternalSymbols=%d", len(genLang.ExternalSymbols))

	ok := grammars.AdaptScannerForLanguage("markdown", genLang)
	if !ok {
		t.Fatal("AdaptScannerForLanguage(markdown) returned false")
	}
	if genLang.ExternalScanner == nil {
		t.Fatal("ExternalScanner is nil after AdaptScannerForLanguage")
	}
}

// markdownSmokeInputs is a small set of CommonMark + GFM inputs used for
// parse-equality checks between MarkdownGrammar() and MarkdownLanguage().
var markdownSmokeInputs = []struct {
	name  string
	input string
}{
	{"heading_atx", "# Hello World\n"},
	{"heading_setext", "Hello\n=====\n"},
	{"paragraph", "This is a paragraph.\n"},
	{"thematic_break", "---\n"},
	{"blank_doc", "\n"},
	{"fenced_code_go", "```go\npackage main\n```\n"},
	{"blockquote", "> A quote\n"},
	{"unordered_list", "- one\n- two\n- three\n"},
	{"ordered_list", "1. first\n2. second\n"},
	{"indented_code", "    code here\n"},
	{"link", "See [example](https://example.com).\n"},
	{"image", "![alt](image.png)\n"},
	{"emphasis", "*em* and **strong**\n"},
	{"task_list", "- [x] done\n- [ ] todo\n"},
	{"heading_h2", "## Level 2\n"},
	{"heading_h3_h6", "### Three\n#### Four\n##### Five\n###### Six\n"},
}

// generateMarkdownLang is a shared helper that compiles MarkdownGrammar() and
// attaches the external scanner.
func generateMarkdownLang(t *testing.T) *gotreesitter.Language {
	t.Helper()
	g := MarkdownGrammar()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	genLang, err := GenerateLanguageWithContext(ctx, g)
	if err != nil {
		t.Fatalf("GenerateLanguage failed: %v", err)
	}
	if !grammars.AdaptScannerForLanguage("markdown", genLang) {
		t.Fatal("AdaptScannerForLanguage(markdown) returned false")
	}
	return genLang
}

// TestMarkdownGrammarParseEquality checks that MarkdownGrammar() + the
// adapted scanner produces S-expression-identical output to the checked-in
// MarkdownLanguage() blob for a set of representative inputs.
//
// This test logs divergences rather than failing hard when S-expressions
// differ, because symbol-ID ordering may cause minor field-name differences.
// The hard gate is: no unexpected ERROR nodes in the generated tree.
func TestMarkdownGrammarParseEquality(t *testing.T) {
	refLang := grammars.MarkdownLanguage()
	if refLang == nil {
		t.Fatal("reference MarkdownLanguage() not available")
	}

	genLang := generateMarkdownLang(t)

	refParser := gotreesitter.NewParser(refLang)
	genParser := gotreesitter.NewParser(genLang)

	passed, diverged := 0, 0
	for _, tc := range markdownSmokeInputs {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.input)

			refTree, err := refParser.Parse(src)
			if err != nil {
				t.Fatalf("reference parse failed: %v", err)
			}
			genTree, err := genParser.Parse(src)
			if err != nil {
				t.Fatalf("generated parse failed: %v", err)
			}

			refSexp := refTree.RootNode().SExpr(refLang)
			genSexp := genTree.RootNode().SExpr(genLang)

			if genSexp == refSexp {
				passed++
				return
			}
			diverged++
			t.Logf("S-expression mismatch for %q:", tc.name)
			t.Logf("  ref: %.400s", refSexp)
			t.Logf("  gen: %.400s", genSexp)
			// Hard gate: no ERROR nodes in generated where reference has none.
			if strings.Contains(genSexp, "ERROR") && !strings.Contains(refSexp, "ERROR") {
				t.Errorf("generated tree has ERROR nodes not present in reference for %q", tc.name)
			}
		})
	}
	t.Logf("parse equality summary: %d/%d passed, %d diverged",
		passed, passed+diverged, diverged)
}

// TestMarkdownGrammarConformanceInputs parses the mdpp conformance inputs
// (when available on disk) and verifies that neither the generated nor the
// reference parser produces ERROR nodes.
func TestMarkdownGrammarConformanceInputs(t *testing.T) {
	const conformanceRoot = "/home/draco/work/mdpp/examples/conformance"
	entries, err := os.ReadDir(conformanceRoot)
	if err != nil {
		t.Skipf("conformance directory not available: %v", err)
	}

	refLang := grammars.MarkdownLanguage()
	if refLang == nil {
		t.Fatal("reference MarkdownLanguage() not available")
	}

	genLang := generateMarkdownLang(t)

	refParser := gotreesitter.NewParser(refLang)
	genParser := gotreesitter.NewParser(genLang)

	// The conformance suite includes Markdown++ extension inputs that are not
	// part of the base CommonMark grammar. Skip them; they are tested by mdpp's
	// own ExtendGrammar tests. The reference MarkdownLanguage() blob is the
	// mdpp blob, so it handles these, but our base grammar is spec-faithful.
	mdppOnlyInputs := map[string]bool{
		"011-admonition-note":    true,
		"012-admonition-caution": true,
		"013-container-warning":  true,
		"014-container-details":  true,
		"015-container-nesting":  true,
		"026-definition-list":    true,
		"028-embed-youtube":      true,
		"029-embed-generic":      true,
		"030-diagram-mermaid":    true,
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if mdppOnlyInputs[entry.Name()] {
			continue // base grammar does not handle mdpp extensions
		}
		inputPath := filepath.Join(conformanceRoot, entry.Name(), "input.md")
		src, err := os.ReadFile(inputPath)
		if err != nil {
			continue // skip dirs without input.md
		}
		t.Run(entry.Name(), func(t *testing.T) {
			refTree, err := refParser.Parse(src)
			if err != nil {
				t.Fatalf("reference parse failed: %v", err)
			}
			genTree, err := genParser.Parse(src)
			if err != nil {
				t.Fatalf("generated parse failed: %v", err)
			}

			refSexp := refTree.RootNode().SExpr(refLang)
			genSexp := genTree.RootNode().SExpr(genLang)

			// Hard gate: unexpected ERROR nodes.
			if strings.Contains(genSexp, "ERROR") && !strings.Contains(refSexp, "ERROR") {
				t.Errorf("generated tree has unexpected ERROR nodes:\n  gen: %.400s\n  ref: %.400s",
					genSexp, refSexp)
			}
			// Soft log: note divergences for future tightening.
			if genSexp != refSexp {
				t.Logf("S-expression divergence (non-ERROR):\n  ref: %.200s\n  gen: %.200s",
					refSexp, genSexp)
			}
		})
	}
}

// TestMarkdownFencedCodeBlockContentParity locks in the fix from commit
// 2c642068 "fix(grammargen): isolate markdown fenced code block newlines".
//
// Before the fix, ``` ```go\npackage main\n``` ``` parsed `package main`
// as a `paragraph` because LALR state merging caused `_close_block` to
// fire prematurely inside `code_fence_content`. The fix introduces a
// distinct `_fenced_code_block_newline` rule (same pattern as the earlier
// `_indented_chunk_newline` / `_html_block_newline` isolations) so the
// fenced-context newline doesn't share LR state with newline contexts
// where `_close_block` is in the FOLLOW set.
//
// Hard gates this test enforces:
//   - Generated parser's S-expression equals the bundled MarkdownLanguage().
//   - The fenced body is a `code_fence_content` node (NOT a `paragraph`).
//   - No `ERROR` nodes anywhere in the generated tree.
//   - No `_close_block` token marker leaks into the fence body (no
//     extraneous close-of-block before the closing ```).
//
// If this test starts failing, suspect a regression of commit 2c642068
// or a change to MarkdownGrammar() that re-merges the fenced-code newline
// with the global `_newline` rule.
func TestMarkdownFencedCodeBlockContentParity(t *testing.T) {
	const input = "```go\npackage main\n```\n"

	refLang := grammars.MarkdownLanguage()
	if refLang == nil {
		t.Fatal("reference MarkdownLanguage() not available")
	}
	genLang := generateMarkdownLang(t)

	refParser := gotreesitter.NewParser(refLang)
	genParser := gotreesitter.NewParser(genLang)

	src := []byte(input)
	refTree, err := refParser.Parse(src)
	if err != nil {
		t.Fatalf("reference parse failed: %v", err)
	}
	genTree, err := genParser.Parse(src)
	if err != nil {
		t.Fatalf("generated parse failed: %v", err)
	}

	refSexp := refTree.RootNode().SExpr(refLang)
	genSexp := genTree.RootNode().SExpr(genLang)

	// Hard gate 1: byte-identical S-expression CST.
	if genSexp != refSexp {
		t.Errorf("fenced code block CST diverges from bundled parser:\n  ref: %s\n  gen: %s",
			refSexp, genSexp)
	}

	// Hard gate 2: the body must be code_fence_content, not paragraph.
	// (This is the load-bearing assertion — before the fix, it was paragraph.)
	if !strings.Contains(genSexp, "code_fence_content") {
		t.Errorf("generated CST missing code_fence_content node; got: %s", genSexp)
	}
	if strings.Contains(genSexp, "paragraph") {
		t.Errorf("generated CST contains paragraph node — fenced body misparsed as paragraph; got: %s",
			genSexp)
	}

	// Hard gate 3: no ERROR nodes.
	if strings.Contains(genSexp, "ERROR") {
		t.Errorf("generated CST contains ERROR node(s); got: %s", genSexp)
	}
	if strings.Contains(refSexp, "ERROR") {
		t.Errorf("reference CST contains ERROR node(s); got: %s", refSexp)
	}

	// Hard gate 4: _close_block must not appear inside the fence body.
	// _close_block is a hidden external marker; if it materialized as a
	// visible node inside the tree, that's the premature-close symptom.
	if strings.Contains(genSexp, "_close_block") || strings.Contains(genSexp, "close_block") {
		t.Errorf("generated CST contains a close_block marker — _close_block fired prematurely inside fence body; got: %s",
			genSexp)
	}

	t.Logf("fenced code block parity OK; CST: %s", genSexp)
}
