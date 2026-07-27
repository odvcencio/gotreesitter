// Command corpuscheck runs gotreesitter against upstream tree-sitter corpora.
// Each grammar repository supplies its test fixtures.
// The command reports exact matches, mismatches, and skips for each language.
//
// Usage:
//
//	go run ./corpuscheck/cmd/corpuscheck -root /path/to/gts-corpora/grammar_parity -langs json,go,python
//	go run ./corpuscheck/cmd/corpuscheck -root /path/to/gts-corpora/grammar_parity -langs all -max-langs 30
//
// The command does not use cgo or the C runtime.
// Committed grammar output provides the comparison oracle.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/odvcencio/gotreesitter/corpuscheck"
	"github.com/odvcencio/gotreesitter/grammars"
)

func main() {
	var (
		root       string
		langsFlag  string
		maxLangs   int
		maxCasesPL int
		jsonOut    string
		verbose    bool
	)
	flag.StringVar(&root, "root", "", "grammar_parity root directory (each subdirectory is one upstream grammar checkout)")
	flag.StringVar(&langsFlag, "langs", "", "comma-separated language names (registry names), or 'all' to sweep every language with a discoverable corpus dir")
	flag.IntVar(&maxLangs, "max-langs", 0, "if >0 and -langs=all, stop after this many languages (bounds a full sweep)")
	flag.IntVar(&maxCasesPL, "max-cases-per-lang", 0, "if >0, stop after this many test cases per language (bounds memory/time on a shared machine)")
	flag.StringVar(&jsonOut, "json", "", "optional path to write full per-case results as JSON")
	flag.BoolVar(&verbose, "v", false, "print each failing/skipped case")
	flag.Parse()

	if root == "" {
		fatalf("-root is required")
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		fatalf("-root %q is not a directory: %v", root, err)
	}
	if langsFlag == "" {
		fatalf("-langs is required (comma list or 'all')")
	}

	langs, err := resolveLanguages(root, langsFlag, maxLangs)
	if err != nil {
		fatalf("%v", err)
	}

	var allReports []*corpuscheck.LanguageReport
	totalCases, totalPass, totalPassLenient := 0, 0, 0

	for _, lang := range langs {
		corpusDir, requireTag, err := resolveCorpus(root, lang)
		if err != nil {
			r := &corpuscheck.LanguageReport{Language: lang, SkipReason: err.Error()}
			allReports = append(allReports, r)
			printLanguage(r, verbose)
			continue
		}
		report := corpuscheck.RunLanguageWithOptions(lang, corpusDir, corpuscheck.RunOptions{
			MaxCases:           maxCasesPL,
			RequireLanguageTag: requireTag,
		})
		allReports = append(allReports, report)
		totalCases += report.Cases
		totalPass += report.Pass
		totalPassLenient += report.PassFieldLenient
		printLanguage(report, verbose)
	}

	fmt.Println()
	fmt.Println("=== HEADLINE ===")
	fmt.Printf("languages attempted: %d\n", len(langs))
	ran := 0
	for _, r := range allReports {
		if r.SkipReason == "" {
			ran++
		}
	}
	fmt.Printf("languages run: %d\n", ran)
	fmt.Printf("total cases: %d\n", totalCases)
	fmt.Printf("total exact matches: %d\n", totalPass)
	if totalCases > 0 {
		fmt.Printf("pass rate (strict): %.2f%%\n", 100*float64(totalPass)/float64(totalCases))
		fmt.Printf("pass rate (field-lenient secondary lens -- see PassFieldLenient doc): %.2f%% (%d/%d)\n",
			100*float64(totalPassLenient)/float64(totalCases), totalPassLenient, totalCases)
	}

	failByCategory := map[string]int{}
	skipByReason := map[string]int{}
	for _, r := range allReports {
		for k, v := range r.FailCategoryCounts() {
			failByCategory[k] += v
		}
		for k, v := range r.SkipReasonCounts() {
			skipByReason[k] += v
		}
	}
	printCounts("mismatch categories", failByCategory)
	printCounts("skip reasons", skipByReason)

	if jsonOut != "" {
		f, err := os.Create(jsonOut)
		if err != nil {
			fatalf("create %s: %v", jsonOut, err)
		}
		defer f.Close()
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		if err := enc.Encode(allReports); err != nil {
			fatalf("write json: %v", err)
		}
	}
}

func printCounts(title string, counts map[string]int) {
	if len(counts) == 0 {
		return
	}
	fmt.Printf("\n%s:\n", title)
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return counts[keys[i]] > counts[keys[j]] })
	for _, k := range keys {
		fmt.Printf("  %-32s %d\n", k, counts[k])
	}
}

func printLanguage(r *corpuscheck.LanguageReport, verbose bool) {
	if r.SkipReason != "" {
		fmt.Printf("%-16s SKIP (%s)\n", r.Language, r.SkipReason)
		return
	}
	fmt.Printf("%-16s cases=%-5d pass=%-5d pass_lenient=%-5d fail=%-5d skip=%-5d format_errors=%d  (%s)\n",
		r.Language, r.Cases, r.Pass, r.PassFieldLenient, len(r.Failed), len(r.Skipped), len(r.FormatErrors), r.CorpusRoot)
	if verbose {
		for _, f := range r.Failed {
			fmt.Printf("    FAIL [%s] %s:%d %q\n      %s\n", f.Reason, filepath.Base(f.File), f.Line, f.Name, f.Detail)
		}
		for _, s := range r.Skipped {
			fmt.Printf("    SKIP [%s] %s:%d %q\n", s.Reason, filepath.Base(s.File), s.Line, s.Name)
		}
		for _, e := range r.FormatErrors {
			fmt.Printf("    FORMAT ERROR: %v\n", e)
		}
	}
}

// resolveLanguages expands "all" to each registered corpus directory.
// A positive maxLangs value limits the result.
// The function filters a comma-separated list to registered names.
func resolveLanguages(root, langsFlag string, maxLangs int) ([]string, error) {
	if strings.TrimSpace(langsFlag) != "all" {
		parts := strings.Split(langsFlag, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		return out, nil
	}

	registered := map[string]bool{}
	for _, e := range grammars.AllLanguages() {
		registered[e.Name] = true
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read root %s: %w", root, err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !registered[name] {
			continue
		}
		if _, _, err := resolveCorpus(root, name); err != nil {
			continue
		}
		out = append(out, name)
	}
	for name := range corpusAliases {
		if !registered[name] {
			continue
		}
		if _, _, err := resolveCorpus(root, name); err != nil {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	if maxLangs > 0 && len(out) > maxLangs {
		out = out[:maxLangs]
	}
	return out, nil
}

// corpusAliasEntry redirects a registered language name to a corpus
// directory that lives under a *different* top-level checkout name, for
// grammars whose upstream repo doesn't ship its own corpus/ directory at
// all -- instead its fixtures are interleaved into another grammar's
// corpus, distinguished only by a `:language(name)` attribute:
//
//   - tsx fixtures live inside typescript's test/corpus (same upstream
//     repo, tsx is a second grammar defined by the same grammar.js).
//   - dtd fixtures live inside xml's test/corpus (ditto).
//   - gitcommit's upstream directory is named gitcommit_gbprod (a
//     disambiguated checkout name, not a shared corpus -- no tag needed).
//   - markdown_inline's upstream directory is
//     markdown/tree-sitter-markdown-inline (a second grammar in the
//     tree-sitter-markdown monorepo, but with its own dedicated corpus
//     dir -- no tag needed).
type corpusAliasEntry struct {
	dirRel     string // relative to root
	requireTag bool
}

var corpusAliases = map[string]corpusAliasEntry{
	"tsx":             {dirRel: filepath.Join("typescript", "test", "corpus"), requireTag: true},
	"dtd":             {dirRel: filepath.Join("xml", "test", "corpus"), requireTag: true},
	"gitcommit":       {dirRel: filepath.Join("gitcommit_gbprod", "test", "corpus")},
	"markdown_inline": {dirRel: filepath.Join("markdown", "tree-sitter-markdown-inline", "test", "corpus")},
}

// languagesRequiringTag names registry languages whose OWN corpus
// directory is shared with a guest language via corpusAliases (see
// corpusAliasEntry) -- so their own untagged cases must NOT be handed to
// the guest, but the guest's tagged cases must also not count for them.
// languageTagFilter (runner.go) already excludes cases explicitly tagged
// for a different language; this only matters for whether an UNTAGGED
// case belongs to the host language, which is always true for the host.

// resolveCorpus locates the corpus directory for a registry language
// name under root, and reports whether that language only owns a subset
// of that directory's cases (selected via a `:language()` attribute; see
// corpusAliases). It tries, in order: a corpusAliases entry, then
// <root>/<name>/test/corpus, then <root>/<name>/corpus, then the
// shortest corpus/corpus_* directory discovered anywhere under
// <root>/<name> (covering nested/multi-grammar repos such as apex,
// markdown, wat, v -- see DiscoverCorpusDirs).
func resolveCorpus(root, name string) (dir string, requireTag bool, err error) {
	if alias, ok := corpusAliases[name]; ok {
		candidate := filepath.Join(root, alias.dirRel)
		if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
			return candidate, alias.requireTag, nil
		}
		return "", false, fmt.Errorf("aliased corpus directory %s not found", candidate)
	}

	langRoot := filepath.Join(root, name)
	if info, statErr := os.Stat(langRoot); statErr != nil || !info.IsDir() {
		return "", false, fmt.Errorf("no %s directory under %s", name, root)
	}
	for _, candidate := range []string{
		filepath.Join(langRoot, "test", "corpus"),
		filepath.Join(langRoot, "corpus"),
	} {
		if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
			return candidate, false, nil
		}
	}
	dirs, discErr := corpuscheck.DiscoverCorpusDirs(langRoot)
	if discErr != nil {
		return "", false, discErr
	}
	if len(dirs) == 0 {
		return "", false, fmt.Errorf("no corpus directory found under %s", langRoot)
	}
	return dirs[0], false, nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
