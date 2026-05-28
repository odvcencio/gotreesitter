package grammargen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// fixtureMode classifies how each fixture in mdpp/testdata/cst-snapshots
// participates in the parity assertion.
type fixtureMode int

const (
	// modeAssertEqual: vanilla-equivalent content. Both parsers must produce
	// byte-identical CST. Failure here is a regression in MarkdownGrammar.
	modeAssertEqual fixtureMode = iota

	// modeInformational: content that exercises mdpp block-level extensions
	// (:::, $$, [^id]:, definition lists). Vanilla parsers don't recognize
	// these. Divergences are logged but don't fail — mdpp's full pipeline
	// (with the Phase-3 grammar extensions) is what will handle these.
	modeInformational

	// modeKnownTrackedBug: a real vanilla divergence tracked as a separate
	// task. Skipped here with a reason; the fix is the responsibility of
	// the tracking task, not this test.
	modeKnownTrackedBug
)

// modeForFixture returns the parity mode for a given relative path under
// mdpp/testdata/cst-snapshots, plus a tracking reference (empty if none).
func modeForFixture(rel string) (fixtureMode, string) {
	switch rel {
	case "auto-embed/input.md", "emoji/input.md", "superscript-subscript/input.md":
		return modeAssertEqual, ""
	case "admonition/input.md":
		return modeKnownTrackedBug, "Task 0.1.5 (conformance/010 blockquote double-emit family)"
	case "container-directive/input.md",
		"definition-list/input.md",
		"footnote/input.md",
		"math-inline-and-block/input.md":
		return modeInformational, ""
	}
	// New fixtures default to assertEqual — fail-loud forces explicit
	// classification when the corpus grows.
	return modeAssertEqual, ""
}

// TestMarkdownGrammarMdppCorpusParity walks ~/work/mdpp/testdata/cst-snapshots
// and, per fixture, either asserts byte-identical CST S-expressions between
// grammargen.MarkdownGrammar()'s generated parser and the bundled
// grammars.MarkdownLanguage() blob, logs divergence informationally, or skips
// with a tracking reference. See modeForFixture for the per-file policy.
//
// Purpose: proves the generated parser is a drop-in replacement for mdpp's
// real workload on vanilla-equivalent content, which is the gate for Task 0.3
// (parse.go swap). Extension-content fixtures are intentionally informational
// because vanilla parsers don't recognize mdpp's block-level extensions
// (:::, $$, [^id]:, definition lists) — the Phase-3 grammar extensions will.
func TestMarkdownGrammarMdppCorpusParity(t *testing.T) {
	corpusRoot := mdppCstSnapshotsDir(t)

	refLang := grammars.MarkdownLanguage()
	if refLang == nil {
		t.Skip("bundled MarkdownLanguage unavailable")
	}
	genLang := generateMarkdownLang(t)
	if genLang == nil {
		t.Skip("could not generate markdown language")
	}

	var files []string
	err := filepath.Walk(corpusRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".md") && !strings.HasSuffix(path, ".mdpp") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", corpusRoot, err)
	}
	if len(files) == 0 {
		t.Fatalf("no .md/.mdpp files found under %s", corpusRoot)
	}

	refParser := gotreesitter.NewParser(refLang)
	genParser := gotreesitter.NewParser(genLang)

	var (
		asserted             int
		informational        int
		skipped              int
		divergentInformative int
	)

	for _, file := range files {
		file := file
		rel, _ := filepath.Rel(corpusRoot, file)
		mode, trackingRef := modeForFixture(rel)
		t.Run(rel, func(t *testing.T) {
			if mode == modeKnownTrackedBug {
				skipped++
				t.Skipf("known-tracked-bug: %s", trackingRef)
				return
			}

			src, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("reading %s: %v", file, err)
			}
			refTree, err := refParser.Parse(src)
			if err != nil {
				t.Fatalf("reference parser failed on %s: %v", file, err)
			}
			if refTree == nil {
				t.Fatalf("reference parser returned nil tree for %s", file)
			}
			genTree, err := genParser.Parse(src)
			if err != nil {
				t.Fatalf("generated parser failed on %s: %v", file, err)
			}
			if genTree == nil {
				t.Fatalf("generated parser returned nil tree for %s", file)
			}

			refSExp := refTree.RootNode().SExpr(refLang)
			genSExp := genTree.RootNode().SExpr(genLang)

			switch mode {
			case modeAssertEqual:
				asserted++
				if refSExp != genSExp {
					t.Errorf("CST diff for %s\nref: %s\ngen: %s", rel,
						truncateSExp(refSExp, 4000), truncateSExp(genSExp, 4000))
				}
			case modeInformational:
				informational++
				if refSExp != genSExp {
					divergentInformative++
					t.Logf("informational CST diff for %s (extension content; vanilla parsers diverge as expected)\nref: %s\ngen: %s",
						rel, truncateSExp(refSExp, 2000), truncateSExp(genSExp, 2000))
				}
			}
		})
	}

	t.Logf("%d asserted, %d informational, %d skipped, %d divergent (informational)",
		asserted, informational, skipped, divergentInformative)
}

// mdppCstSnapshotsDir returns the absolute path to mdpp's CST snapshot corpus.
// It's a sibling repo to gotreesitter on the test host. If the directory is
// missing (e.g. CI doesn't check out mdpp), the test skips rather than fails.
func mdppCstSnapshotsDir(t *testing.T) string {
	t.Helper()
	candidates := []string{
		os.ExpandEnv("$HOME/work/mdpp/testdata/cst-snapshots"),
		"/home/draco/work/mdpp/testdata/cst-snapshots",
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	t.Skipf("mdpp testdata/cst-snapshots not found in any of: %v", candidates)
	return ""
}

// truncateSExp trims an S-expression to maxLen with a tail marker so test
// failure logs stay readable.
func truncateSExp(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...[truncated]"
}
