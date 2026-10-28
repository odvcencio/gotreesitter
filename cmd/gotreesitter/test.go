package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/odvcencio/gotreesitter/corpuscheck"
	"github.com/odvcencio/gotreesitter/grammars"
)

// test runs upstream tree-sitter corpus files against gotreesitter and reports
// exact matches, mismatches and skips per language. Each subdirectory of
// --root is one upstream grammar checkout (the gts-corpora grammar_parity layout).
func (c *cli) test(args []string) int {
	fs := c.flags("test")
	root := fs.String("root", "", "directory of upstream grammar checkouts, one subdirectory per language")
	langsFlag := fs.String("langs", "", "comma-separated language names, or 'all' for every language with a corpus")
	maxLangs := fs.Int("max-langs", 0, "with -langs all, stop after this many languages")
	maxCases := fs.Int("max-cases", 0, "stop after this many cases per language (bounds time and memory)")
	jsonOut := fs.String("json", "", "write per-case results to this JSON file")
	verbose := fs.Bool("v", false, "print each failing or skipped case")
	if code, stop := parseFlags(fs, args); stop {
		return code
	}
	if info, err := os.Stat(*root); err != nil || !info.IsDir() {
		return c.fail("test needs --root <directory> (got %q)", *root)
	}
	if *langsFlag == "" {
		return c.fail("test needs --langs <a,b,...|all>")
	}
	langs, err := corpusLanguages(*root, *langsFlag, *maxLangs)
	if err != nil {
		return c.fail("%v", err)
	}

	var reports []*corpuscheck.LanguageReport
	cases, pass, lenient, failed, ran := 0, 0, 0, 0, 0
	for _, lang := range langs {
		dir, requireTag, err := resolveCorpus(*root, lang)
		r := &corpuscheck.LanguageReport{Language: lang}
		if err != nil {
			r.SkipReason = err.Error()
		} else {
			r = corpuscheck.RunLanguageWithOptions(lang, dir, corpuscheck.RunOptions{MaxCases: *maxCases, RequireLanguageTag: requireTag})
		}
		reports = append(reports, r)
		if r.SkipReason == "" {
			ran++
			cases, pass, lenient, failed = cases+r.Cases, pass+r.Pass, lenient+r.PassFieldLenient, failed+len(r.Failed)
		}
		c.printLanguage(r, *verbose)
	}

	fmt.Fprintf(c.stdout, "\n=== HEADLINE ===\nlanguages attempted: %d\nlanguages run: %d\ntotal cases: %d\ntotal exact matches: %d\n",
		len(langs), ran, cases, pass)
	if cases > 0 {
		fmt.Fprintf(c.stdout, "pass rate (strict): %.2f%%\n", 100*float64(pass)/float64(cases))
		fmt.Fprintf(c.stdout, "pass rate (field-lenient secondary lens, see PassFieldLenient): %.2f%% (%d/%d)\n",
			100*float64(lenient)/float64(cases), lenient, cases)
	}
	fail, skip := map[string]int{}, map[string]int{}
	for _, r := range reports {
		for k, v := range r.FailCategoryCounts() {
			fail[k] += v
		}
		for k, v := range r.SkipReasonCounts() {
			skip[k] += v
		}
	}
	c.printCounts("mismatch categories", fail)
	c.printCounts("skip reasons", skip)

	if *jsonOut != "" {
		b, err := json.MarshalIndent(reports, "", "  ")
		if err == nil {
			err = os.WriteFile(*jsonOut, append(b, '\n'), 0o644)
		}
		if err != nil {
			return c.fail("write %s: %v", *jsonOut, err)
		}
	}
	if failed > 0 {
		return 1
	}
	// Nothing ran, or a language named on the command line was skipped: the
	// input is wrong, so a green exit code would mislead.
	if ran == 0 || (strings.TrimSpace(*langsFlag) != "all" && ran < len(langs)) {
		return c.fail("test ran %d of %d languages; see the SKIP lines above", ran, len(langs))
	}
	return 0
}

func (c *cli) printCounts(title string, counts map[string]int) {
	if len(counts) == 0 {
		return
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return counts[keys[i]] > counts[keys[j]] })
	fmt.Fprintf(c.stdout, "\n%s:\n", title)
	for _, k := range keys {
		fmt.Fprintf(c.stdout, "  %-32s %d\n", k, counts[k])
	}
}

func (c *cli) printLanguage(r *corpuscheck.LanguageReport, verbose bool) {
	if r.SkipReason != "" {
		fmt.Fprintf(c.stdout, "%-16s SKIP (%s)\n", r.Language, r.SkipReason)
		return
	}
	fmt.Fprintf(c.stdout, "%-16s cases=%-5d pass=%-5d pass_lenient=%-5d fail=%-5d skip=%-5d format_errors=%d  (%s)\n",
		r.Language, r.Cases, r.Pass, r.PassFieldLenient, len(r.Failed), len(r.Skipped), len(r.FormatErrors), r.CorpusRoot)
	if !verbose {
		return
	}
	for _, f := range r.Failed {
		fmt.Fprintf(c.stdout, "    FAIL [%s] %s:%d %q\n      %s\n", f.Reason, filepath.Base(f.File), f.Line, f.Name, f.Detail)
	}
	for _, s := range r.Skipped {
		fmt.Fprintf(c.stdout, "    SKIP [%s] %s:%d %q\n", s.Reason, filepath.Base(s.File), s.Line, s.Name)
	}
	for _, e := range r.FormatErrors {
		fmt.Fprintf(c.stdout, "    FORMAT ERROR: %v\n", e)
	}
}

// corpusLanguages expands a comma list, or 'all' to every registered language
// that has a corpus under root (limited to maxLangs when positive).
func corpusLanguages(root, langsFlag string, maxLangs int) ([]string, error) {
	if strings.TrimSpace(langsFlag) != "all" {
		var out []string
		for _, p := range strings.Split(langsFlag, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read root %s: %w", root, err)
	}
	cands := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() {
			cands[e.Name()] = true
		}
	}
	for name := range corpusAliases {
		cands[name] = true
	}
	var out []string
	for _, e := range grammars.AllLanguages() {
		if !cands[e.Name] {
			continue
		}
		if _, _, err := resolveCorpus(root, e.Name); err == nil {
			out = append(out, e.Name)
		}
	}
	sort.Strings(out)
	if maxLangs > 0 && len(out) > maxLangs {
		out = out[:maxLangs]
	}
	return out, nil
}

// corpusAliases redirects a language to a corpus directory under another
// checkout, for grammars whose upstream repository has no corpus of its own.
// requireTag means the directory is shared, and only cases tagged
// :language(name) belong to the language.
var corpusAliases = map[string]struct {
	dirRel     string // relative to root
	requireTag bool
}{
	"tsx":             {filepath.Join("typescript", "test", "corpus"), true},
	"dtd":             {filepath.Join("xml", "test", "corpus"), true},
	"gitcommit":       {filepath.Join("gitcommit_gbprod", "test", "corpus"), false},
	"markdown_inline": {filepath.Join("markdown", "tree-sitter-markdown-inline", "test", "corpus"), false},
}

// resolveCorpus finds the corpus directory for a language under root: an alias,
// then <name>/test/corpus, then <name>/corpus, then the shortest corpus
// directory found anywhere under <name>.
func resolveCorpus(root, name string) (dir string, requireTag bool, err error) {
	isDir := func(p string) bool { info, err := os.Stat(p); return err == nil && info.IsDir() }
	if a, ok := corpusAliases[name]; ok {
		if p := filepath.Join(root, a.dirRel); isDir(p) {
			return p, a.requireTag, nil
		}
		return "", false, fmt.Errorf("aliased corpus directory %s not found", filepath.Join(root, a.dirRel))
	}
	langRoot := filepath.Join(root, name)
	if !isDir(langRoot) {
		return "", false, fmt.Errorf("no %s directory under %s", name, root)
	}
	for _, p := range []string{filepath.Join(langRoot, "test", "corpus"), filepath.Join(langRoot, "corpus")} {
		if isDir(p) {
			return p, false, nil
		}
	}
	dirs, err := corpuscheck.DiscoverCorpusDirs(langRoot)
	if err != nil {
		return "", false, err
	}
	if len(dirs) == 0 {
		return "", false, fmt.Errorf("no corpus directory found under %s", langRoot)
	}
	return dirs[0], false, nil
}
