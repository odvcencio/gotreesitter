package corpuscheck

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// FileResult is the outcome for one corpus file.
type FileResult struct {
	Path             string
	Cases            int
	Pass             int
	PassFieldLenient int
	Fail             []CaseResult
	Skipped          []CaseResult
	// FormatErrors holds test-framing errors ParseFile recovered from
	// (see TestCase.ParseError); each one means at least one test in the
	// file could not be extracted at all.
	FormatErrors []error
}

// CaseResult records one test case's outcome.
type CaseResult struct {
	File   string
	Name   string
	Line   int
	Reason string // skip reason, or mismatch category on failure
	Detail string
	Path   string // divergence path, if applicable
}

// LanguageReport aggregates every file's results for one language.
type LanguageReport struct {
	Language   string
	CorpusRoot string
	Files      int
	Cases      int
	Pass       int
	// PassFieldLenient counts cases that pass under the strict
	// comparison OR whose only divergences are "fixture has no field
	// name, gotreesitter's actual tree does" (see
	// CompareOptions.IgnoreMissingExpectedFields). This is a secondary,
	// clearly-separate lens: several upstream grammar repos' own corpus
	// fixtures are stale with respect to fields their grammar.js added
	// years ago (verified via each repo's git history for json and go;
	// see the corpuscheck report). It is never folded into Pass.
	PassFieldLenient int
	Skipped          []CaseResult
	Failed           []CaseResult
	FormatErrors     []error
	// SkipReason, if non-empty, means the whole language was not run at
	// all (e.g. unsupported parse backend, corpus directory not found).
	SkipReason string
}

// FailCategoryCounts tallies Failed by Reason.
func (r *LanguageReport) FailCategoryCounts() map[string]int {
	out := map[string]int{}
	for _, c := range r.Failed {
		out[c.Reason]++
	}
	return out
}

// SkipReasonCounts tallies Skipped by Reason.
func (r *LanguageReport) SkipReasonCounts() map[string]int {
	out := map[string]int{}
	for _, c := range r.Skipped {
		out[c.Reason]++
	}
	return out
}

// DiscoverCorpusDirs recursively finds every directory literally named
// "corpus" (or, to also catch variants such as tree-sitter-foam's
// "corpus_windows" CRLF suite, named with a "corpus" prefix) under root.
// Multi-grammar repos (typescript+tsx, markdown+markdown-inline, apex's
// three sub-grammars, ...) surface as multiple directories; callers that
// only want the single dominant grammar for a language name should use
// the first (shortest-path) entry.
func DiscoverCorpusDirs(root string) ([]string, error) {
	var dirs []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if name == ".git" {
			return filepath.SkipDir
		}
		if name == "corpus" || strings.HasPrefix(name, "corpus_") || strings.HasPrefix(name, "corpus-") {
			dirs = append(dirs, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(dirs, func(i, j int) bool {
		if len(dirs[i]) != len(dirs[j]) {
			return len(dirs[i]) < len(dirs[j])
		}
		return dirs[i] < dirs[j]
	})
	return dirs, nil
}

// nonCorpusFileNames excludes housekeeping files that legitimately live
// inside a "corpus" directory but aren't corpus fixtures themselves.
// Everything else is accepted regardless of extension: most grammars use
// *.txt, but several (asm, groovy: *.test; d, scheme, typst: *.scm;
// nushell: *.nu; vhdl: *.vhd; racket: *.rkt; make: *.mk; llvm: *.ll; wat:
// *.wat/*.wast; org: *.tst; eds: *.eds; perl, cooklang: no extension at
// all) name their fixtures after their own language's usual file
// extension instead. The real tree-sitter CLI doesn't filter by
// extension either -- it treats every file under corpus/ as a fixture.
var nonCorpusFileNames = map[string]bool{
	".gitattributes": true,
	".gitignore":     true,
	".editorconfig":  true,
	".gitkeep":       true,
}

// collectTxtFiles finds every corpus fixture file under dir, sorted. See
// nonCorpusFileNames for the (small) exclusion list.
func collectTxtFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") || nonCorpusFileNames[name] || strings.EqualFold(filepath.Ext(name), ".patch") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// languageParser wraps everything needed to parse source for one language
// with the correct backend (DFA or a registered TokenSourceFactory),
// mirroring cgo_harness/cmd/corpus_parity's buildRunner/parseWithGoRunner.
type languageParser struct {
	entry   grammars.LangEntry
	support grammars.ParseSupport
	lang    *gotreesitter.Language
	parser  *gotreesitter.Parser
}

func newLanguageParser(name string) (*languageParser, error) {
	var entry grammars.LangEntry
	found := false
	for _, e := range grammars.AllLanguages() {
		if e.Name == name {
			entry = e
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("language %q is not registered in grammars.AllLanguages()", name)
	}
	support, ok := grammars.ParseSupportFor(name)
	if !ok {
		return nil, fmt.Errorf("language %q has no parse support report", name)
	}
	if support.Backend == grammars.ParseBackendUnsupported {
		return nil, fmt.Errorf("language %q parse backend is unsupported: %s", name, support.Reason)
	}
	lang := entry.Language()
	if lang == nil {
		return nil, fmt.Errorf("language %q loader returned nil", name)
	}
	parser := gotreesitter.NewParser(lang)
	// Corpus fixtures are small (a few lines), so a legitimate parse
	// should never approach this; it exists only to bound a pathological
	// GLR blowup on one of these deliberately error-inducing fixtures
	// during an unattended multi-language sweep.
	parser.SetTimeoutMicros(5_000_000)
	return &languageParser{
		entry:   entry,
		support: support,
		lang:    lang,
		parser:  parser,
	}, nil
}

func (lp *languageParser) parse(src []byte) (*gotreesitter.Tree, error) {
	switch lp.support.Backend {
	case grammars.ParseBackendTokenSource:
		if lp.entry.TokenSourceFactory == nil {
			return nil, fmt.Errorf("token_source backend without TokenSourceFactory")
		}
		return lp.parser.ParseWithTokenSource(src, lp.entry.TokenSourceFactory(src, lp.lang))
	case grammars.ParseBackendDFA, grammars.ParseBackendDFAPartial:
		return lp.parser.Parse(src)
	default:
		return nil, fmt.Errorf("unsupported backend %q", lp.support.Backend)
	}
}

// RunOptions controls RunLanguageWithOptions.
type RunOptions struct {
	// MaxCases stops issuing new parses once the report's Cases count
	// reaches this many (0 means unlimited).
	MaxCases int
	// RequireLanguageTag is for languages whose fixtures live inside
	// another language's shared corpus directory, distinguished only by
	// a `:language(name)` attribute (e.g. tsx and dtd both live inside
	// typescript's and xml's corpus directories respectively, alongside
	// untagged typescript/xml tests). When true, a test case with no
	// `:language()` attribute at all is skipped rather than treated as
	// belonging to this language.
	RequireLanguageTag bool
}

// RunLanguage parses every corpus file under corpusDir with language's
// registered grammar loader and compares each test case against its
// upstream expected S-expression.
func RunLanguage(language, corpusDir string) *LanguageReport {
	return RunLanguageWithOptions(language, corpusDir, RunOptions{})
}

// RunLanguageLimited behaves like RunLanguage but stops issuing new
// parses once report.Cases reaches maxCases (0 means unlimited).
func RunLanguageLimited(language, corpusDir string, maxCases int) *LanguageReport {
	return RunLanguageWithOptions(language, corpusDir, RunOptions{MaxCases: maxCases})
}

// RunLanguageWithOptions is RunLanguage with full control over
// RunOptions. See RunOptions for the -- language() tag filtering and
// per-language case cap -- knobs it exposes.
func RunLanguageWithOptions(language, corpusDir string, opts RunOptions) *LanguageReport {
	report := &LanguageReport{Language: language, CorpusRoot: corpusDir}

	lp, err := newLanguageParser(language)
	if err != nil {
		report.SkipReason = err.Error()
		return report
	}

	files, err := collectTxtFiles(corpusDir)
	if err != nil {
		report.SkipReason = fmt.Sprintf("walk corpus dir: %v", err)
		return report
	}
	if len(files) == 0 {
		report.SkipReason = "no *.txt files found"
		return report
	}
	report.Files = len(files)

	for _, f := range files {
		if opts.MaxCases > 0 && report.Cases >= opts.MaxCases {
			break
		}
		fr := runFile(lp, language, f, opts.RequireLanguageTag)
		report.Cases += fr.Cases
		report.Pass += fr.Pass
		report.PassFieldLenient += fr.PassFieldLenient
		report.Failed = append(report.Failed, fr.Fail...)
		report.Skipped = append(report.Skipped, fr.Skipped...)
		report.FormatErrors = append(report.FormatErrors, fr.FormatErrors...)
	}
	return report
}

func runFile(lp *languageParser, language, path string, requireLanguageTag bool) FileResult {
	fr := FileResult{Path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		fr.FormatErrors = append(fr.FormatErrors, fmt.Errorf("%s: %w", path, err))
		return fr
	}
	cases, err := ParseFile(data)
	if err != nil {
		fr.FormatErrors = append(fr.FormatErrors, fmt.Errorf("%s: %w", path, err))
		return fr
	}

	for _, tc := range cases {
		if tc.ParseError != nil {
			fr.FormatErrors = append(fr.FormatErrors, fmt.Errorf("%s:%d: %w", path, tc.Line, tc.ParseError))
			continue
		}
		if reason, skip := languageTagFilter(tc, language, requireLanguageTag); skip {
			fr.Cases++
			fr.Skipped = append(fr.Skipped, CaseResult{File: path, Name: tc.Name, Line: tc.Line, Reason: reason})
			continue
		}
		fr.Cases++
		res := runCaseRecovered(lp, path, tc)
		if res.passFieldLenient {
			fr.PassFieldLenient++
		}
		switch res.kind {
		case caseSkip:
			fr.Skipped = append(fr.Skipped, res.result)
		case casePass:
			fr.Pass++
		case caseFail:
			fr.Fail = append(fr.Fail, res.result)
		}
	}
	return fr
}

// languageTagFilter decides whether tc belongs to language, for corpus
// directories shared by more than one grammar (typescript's directory
// also holds tsx fixtures, xml's also holds dtd fixtures, distinguished
// only by a `:language(name)` attribute). A case explicitly tagged for a
// different language is always excluded; an untagged case is excluded
// only when the caller says this language requires the tag (i.e. this
// language is the "guest" in someone else's directory).
func languageTagFilter(tc TestCase, language string, requireLanguageTag bool) (reason string, skip bool) {
	value, ok := tc.AttrValue("language")
	if !ok {
		if requireLanguageTag {
			return "missing_language_attr", true
		}
		return "", false
	}
	if !strings.EqualFold(value, language) {
		return "different_language_attr:" + value, true
	}
	return "", false
}

type caseOutcomeKind int

const (
	casePass caseOutcomeKind = iota
	caseFail
	caseSkip
)

type caseOutcome struct {
	kind             caseOutcomeKind
	result           CaseResult
	passFieldLenient bool
}

// runCaseRecovered isolates a single test case's parse+compare from a
// panic in gotreesitter or the comparator, so one adversarial corpus
// fixture (these fixtures exist specifically to exercise error recovery)
// can't take down an entire language's -- or the whole sweep's -- run.
func runCaseRecovered(lp *languageParser, path string, tc TestCase) (out caseOutcome) {
	defer func() {
		if r := recover(); r != nil {
			out = caseOutcome{
				kind: caseFail,
				result: CaseResult{
					File:   path,
					Name:   tc.Name,
					Line:   tc.Line,
					Reason: "panic",
					Detail: fmt.Sprintf("%v", r),
				},
			}
		}
	}()
	return runCase(lp, path, tc)
}

func runCase(lp *languageParser, path string, tc TestCase) caseOutcome {
	base := CaseResult{File: path, Name: tc.Name, Line: tc.Line}

	if tc.HasAttr("skip") {
		base.Reason = "attr:skip"
		return caseOutcome{kind: caseSkip, result: base}
	}
	if strings.TrimSpace(tc.Expected) != "" {
		if strings.Contains(tc.Expected, "(UNEXPECTED") {
			base.Reason = CategoryUnexpectedUnsupported
			base.Detail = "expected output contains an UNEXPECTED node; gotreesitter has no equivalent node kind"
			return caseOutcome{kind: caseSkip, result: base}
		}
	}

	tree, err := lp.parse(tc.Input)
	if err != nil {
		base.Reason = "go_parse_error"
		base.Detail = err.Error()
		return caseOutcome{kind: caseFail, result: base}
	}
	if tree == nil || tree.RootNode() == nil {
		base.Reason = "go_parse_error"
		base.Detail = "parser returned a nil tree"
		return caseOutcome{kind: caseFail, result: base}
	}
	defer tree.Release()
	root := tree.RootNode()

	if strings.TrimSpace(tc.Expected) == "" {
		// ":error"-tagged tests may omit the expected tree entirely --
		// the author is only asserting the parse contains an error.
		if tc.HasAttr("error") {
			if root.HasError() {
				return caseOutcome{kind: casePass, passFieldLenient: true}
			}
			base.Reason = "expected_error_not_produced"
			base.Detail = "test is marked :error with no expected tree, but gotreesitter's root has no error"
			return caseOutcome{kind: caseFail, result: base}
		}
		base.Reason = "empty_expected_without_error_attr"
		base.Detail = "corpus fixture has no expected output and is not marked :error"
		return caseOutcome{kind: caseSkip, result: base}
	}

	expected, err := ParseExpected(tc.Expected)
	if err != nil {
		base.Reason = "expected_parse_error"
		base.Detail = err.Error()
		return caseOutcome{kind: caseSkip, result: base}
	}

	actual := RenderTree(root, lp.lang)
	if d := Compare(expected, actual); d != nil {
		base.Reason = d.Category
		base.Detail = d.String()
		base.Path = d.Path
		lenientPass := CompareWithOptions(expected, actual, CompareOptions{IgnoreMissingExpectedFields: true}) == nil
		return caseOutcome{kind: caseFail, result: base, passFieldLenient: lenientPass}
	}
	return caseOutcome{kind: casePass, passFieldLenient: true}
}
