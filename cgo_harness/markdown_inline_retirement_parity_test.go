//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestMarkdownInlineConflictPolicyLockedCParity(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	language := grammars.MarkdownInlineLanguage()
	if language == nil {
		t.Fatal("Markdown inline Go language is nil")
	}
	if got := len(language.ConflictPolicies); got != 4 {
		t.Fatalf("Markdown inline conflict policies = %d, want 4", got)
	}
	cLanguage, err := COracleLanguage("markdown_inline")
	if err != nil {
		t.Fatalf("load locked Markdown inline language: %v", err)
	}

	tests := []struct {
		name            string
		source          string
		requireDirect   bool
		requireFallback bool
	}{
		{name: "scorecard smoke", source: "hello **world**\n", requireDirect: true},
		{name: "attribute link tag", source: `<link rel="stylesheet" href="x">`, requireDirect: true},
		{name: "simple link tag", source: `<link>`},
		{name: "unquoted attribute", source: `<link rel=x>`},
		{name: "image attributes", source: `<img src="x" alt="y">`},
		{name: "paired anchor", source: `<a href="x">text</a>`},
		{name: "custom paired tag", source: `<custom data-x="1">body</custom>`},
		{name: "embedded tag", source: `before <link rel="stylesheet" href="x"> after`},
		{name: "plain text", source: "plain text"},
		{name: "emphasis families", source: "**bold** and *emphasis*"},
		{name: "literal tilde link", source: `[Context](https://example.com/~user/file.pdf)`, requireFallback: true},
		{name: "nested emphasis counterexample", source: `*foo**bar**baz*`, requireFallback: true},
		{name: "comment", source: `<!-- comment -->`},
	}

	direct, fallback := 0, 0
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source)
			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked C parser returned no tree")
			}
			t.Cleanup(cTree.Close)
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				t.Fatal(err)
			}

			productionParser := gotreesitter.NewParser(language)
			productionParser.SetAdmissionCandidateRoute(false)
			production, err := productionParser.Parse(source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			t.Cleanup(production.Release)

			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			candidateParser := gotreesitter.NewParser(language)
			candidateParser.SetAdmissionCandidateRoute(true)
			candidate, err := candidateParser.Parse(source)
			if err != nil {
				t.Fatalf("candidate parse: %v", err)
			}
			t.Cleanup(candidate.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			routedDelta := routedAfter - routedBefore
			fallbackDelta := fallbackAfter - fallbackBefore
			switch {
			case routedDelta == 1 && fallbackDelta == 0:
				direct++
				t.Log("candidate route=direct")
			case routedDelta == 0 && fallbackDelta == 1:
				fallback++
				t.Logf("candidate route=fallback reason=%s", gotreesitter.AdmissionCandidateLastFallbackReason())
			default:
				t.Fatalf("candidate route delta=%d/%d, want one direct or fallback route", routedDelta, fallbackDelta)
			}
			if test.requireDirect && routedDelta != 1 {
				t.Fatalf("candidate route delta=%d/%d, want direct", routedDelta, fallbackDelta)
			}
			if test.requireFallback && fallbackDelta != 1 {
				t.Fatalf("candidate route delta=%d/%d, want fallback", routedDelta, fallbackDelta)
			}

			assertJsdocLockedCTreeExact(t, "Markdown inline production", production, language, cTree, cDigest)
			assertJsdocLockedCTreeExact(t, "Markdown inline candidate", candidate, language, cTree, cDigest)
		})
	}
	if direct == 0 || fallback == 0 {
		t.Fatalf("candidate routes direct=%d fallback=%d, want both", direct, fallback)
	}
}

func TestMarkdownInlineConflictPolicyCorpusLockedCParity(t *testing.T) {
	gotreesitter.ResetAdmissionCandidateCounters()
	language := grammars.MarkdownInlineLanguage()
	baseline, err := grammars.LoadLanguage("markdown_inline", grammars.BlobByName("markdown_inline"))
	if err != nil {
		t.Fatalf("load baseline Markdown inline language: %v", err)
	}
	baseline.ConflictPolicies = nil
	cLanguage, err := COracleLanguage("markdown_inline")
	if err != nil {
		t.Fatalf("load locked Markdown inline language: %v", err)
	}
	cases, err := markdownInlineLockedCorpusCases()
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) < 100 {
		t.Fatalf("Markdown inline locked corpus cases = %d, want at least 100", len(cases))
	}

	direct, fallback := 0, 0
	for _, test := range cases {
		baselineParser := gotreesitter.NewParser(baseline)
		baselineParser.SetAdmissionCandidateRoute(false)
		baselineTree, err := baselineParser.Parse(test.source)
		if err != nil {
			t.Fatalf("%s baseline parse: %v", test.label, err)
		}

		productionParser := gotreesitter.NewParser(language)
		productionParser.SetAdmissionCandidateRoute(false)
		productionTree, err := productionParser.Parse(test.source)
		if err != nil {
			baselineTree.Release()
			t.Fatalf("%s production parse: %v", test.label, err)
		}

		baselineDigest := markdownInlineGoDigest(t, test.label+" baseline", baselineTree, baseline)
		productionDigest := markdownInlineGoDigest(t, test.label+" production", productionTree, language)
		baselineTree.Release()
		if productionDigest != baselineDigest {
			productionTree.Release()
			t.Fatalf("%s policy changed production tree: baseline=%s production=%s", test.label, baselineDigest, productionDigest)
		}

		routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
		candidateParser := gotreesitter.NewParser(language)
		candidateParser.SetAdmissionCandidateRoute(true)
		candidateTree, err := candidateParser.Parse(test.source)
		if err != nil {
			productionTree.Release()
			t.Fatalf("%s candidate parse: %v", test.label, err)
		}
		routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
		routedDelta := routedAfter - routedBefore
		fallbackDelta := fallbackAfter - fallbackBefore
		candidateDigest := markdownInlineGoDigest(t, test.label+" candidate", candidateTree, language)
		if candidateDigest != productionDigest {
			candidateTree.Release()
			productionTree.Release()
			t.Fatalf("%s candidate=%s production=%s", test.label, candidateDigest, productionDigest)
		}

		switch {
		case routedDelta == 1 && fallbackDelta == 0:
			direct++
			cParser := sitter.NewParser()
			if err := cParser.SetLanguage(cLanguage); err != nil {
				cParser.Close()
				candidateTree.Release()
				productionTree.Release()
				t.Fatalf("%s set C language: %v", test.label, err)
			}
			cTree := cParser.Parse(test.source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				cParser.Close()
				candidateTree.Release()
				productionTree.Release()
				t.Fatalf("%s locked C parser returned no tree", test.label)
			}
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				cTree.Close()
				cParser.Close()
				candidateTree.Release()
				productionTree.Release()
				t.Fatalf("%s C digest: %v", test.label, err)
			}
			if candidateDigest != cDigest {
				cTree.Close()
				cParser.Close()
				candidateTree.Release()
				productionTree.Release()
				t.Fatalf("%s direct compact tree diverges from C: compact=%s C=%s", test.label, candidateDigest, cDigest)
			}
			cTree.Close()
			cParser.Close()
		case routedDelta == 0 && fallbackDelta == 1:
			fallback++
		default:
			candidateTree.Release()
			productionTree.Release()
			t.Fatalf("%s candidate route delta=%d/%d", test.label, routedDelta, fallbackDelta)
		}
		candidateTree.Release()
		productionTree.Release()
	}
	if direct == 0 || fallback == 0 {
		t.Fatalf("Markdown inline locked corpus routes direct=%d fallback=%d, want both", direct, fallback)
	}
	t.Logf("Markdown inline locked corpus cases=%d direct=%d fallback=%d", len(cases), direct, fallback)
}

type markdownInlineCorpusCase struct {
	label  string
	source []byte
}

func markdownInlineLockedCorpusCases() ([]markdownInlineCorpusCase, error) {
	entry, ok := parityCRefState.lock["markdown_inline"]
	if !ok {
		return nil, fmt.Errorf("locked Markdown inline entry is unavailable")
	}
	repo := filepath.Join(parityCRefState.rootDir, "repos", "markdown_inline")
	if _, err := os.Stat(repo); os.IsNotExist(err) {
		commitShort := entry.Commit
		if len(commitShort) > 12 {
			commitShort = commitShort[:12]
		}
		if cacheDir := parityRepoCacheDir(); cacheDir != "" {
			cachedRepo, cacheErr := findCachedParityRepo(cacheDir, entry.Name, commitShort)
			if cacheErr != nil {
				return nil, cacheErr
			}
			if err := clonePinnedRepoFromLocalCache(cachedRepo, entry.Commit, repo); err != nil {
				return nil, err
			}
		} else if err := clonePinnedRepo(entry.RepoURL, entry.Commit, repo); err != nil {
			return nil, err
		}
	}
	if err := verifyPinnedRepo(repo, entry.Commit); err != nil {
		return nil, err
	}
	corpus := filepath.Join(repo, filepath.Dir(entry.Subdir), "test", "corpus")
	if info, err := os.Stat(corpus); err != nil || !info.IsDir() {
		var corpusDirs []string
		findErr := filepath.WalkDir(repo, func(path string, item fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if item.IsDir() && item.Name() == "corpus" && filepath.Base(filepath.Dir(path)) == "test" {
				corpusDirs = append(corpusDirs, path)
			}
			return nil
		})
		if findErr != nil {
			return nil, findErr
		}
		corpus = ""
		for _, candidate := range corpusDirs {
			if strings.Contains(filepath.ToSlash(candidate), "markdown-inline") {
				corpus = candidate
				break
			}
		}
		if corpus == "" && len(corpusDirs) == 1 {
			corpus = corpusDirs[0]
		}
		if corpus == "" {
			return nil, fmt.Errorf("locked Markdown inline corpus directory is unavailable under %s; candidates=%v", repo, corpusDirs)
		}
	}
	var cases []markdownInlineCorpusCase
	err := filepath.WalkDir(corpus, func(path string, item fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if item.IsDir() {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(corpus, path)
		if err != nil {
			return err
		}
		fileCases, ok := markdownInlineSplitCorpusSources(content)
		if !ok {
			return fmt.Errorf("parse Markdown inline corpus file %s", rel)
		}
		for index, test := range fileCases {
			cases = append(cases, markdownInlineCorpusCase{
				label:  fmt.Sprintf("%s:%d:%s", filepath.ToSlash(rel), index, test.label),
				source: test.source,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return cases, nil
}

func markdownInlineSplitCorpusSources(content []byte) ([]markdownInlineCorpusCase, bool) {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	lines := strings.SplitAfter(text, "\n")
	cases := make([]markdownInlineCorpusCase, 0, 4)
	for index := 0; index < len(lines); {
		if !markdownInlineRepeatedCorpusLine(lines[index], '=') {
			index++
			continue
		}
		if index+1 >= len(lines) {
			return nil, false
		}
		label := strings.TrimSpace(lines[index+1])
		index += 2
		for index < len(lines) && !markdownInlineRepeatedCorpusLine(lines[index], '=') {
			line := strings.TrimSpace(lines[index])
			if line != "" && !strings.HasPrefix(line, ":") {
				return nil, false
			}
			index++
		}
		if index >= len(lines) {
			return nil, false
		}
		index++
		if index < len(lines) && strings.TrimSpace(lines[index]) == "" {
			index++
		}
		start := index
		for index < len(lines) && !markdownInlineRepeatedCorpusLine(lines[index], '-') {
			index++
		}
		if index >= len(lines) {
			return nil, false
		}
		source := strings.Join(lines[start:index], "")
		if strings.TrimSpace(source) == "" {
			return nil, false
		}
		cases = append(cases, markdownInlineCorpusCase{label: label, source: []byte(source)})
		index++
		for index < len(lines) && !markdownInlineRepeatedCorpusLine(lines[index], '=') {
			index++
		}
	}
	return cases, len(cases) > 0
}

func markdownInlineRepeatedCorpusLine(line string, want rune) bool {
	line = strings.TrimSpace(line)
	if len(line) < 3 {
		return false
	}
	for _, current := range line {
		if current != want {
			return false
		}
	}
	return true
}

func markdownInlineGoDigest(t *testing.T, label string, tree *gotreesitter.Tree, language *gotreesitter.Language) string {
	t.Helper()
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatalf("%s digest: %v", label, err)
	}
	return inspection.SHA256
}
