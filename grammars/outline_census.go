package grammars

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// OutlineTier is the computed coverage classification for one registered
// language, per spec.outline-api.v1 section 5. Every language falls into
// exactly one tier; nothing here is hand-maintained -- the census computes
// it fresh from the registry, the inferred tags-query table, and (when
// present) an external real-source corpus.
type OutlineTier string

const (
	// OutlineTierOne: the inferred tags query is non-empty AND a real
	// per-language corpus is present on this host and holds at least one
	// file matching the language's registered extensions.
	OutlineTierOne OutlineTier = "T1"
	// OutlineTierOneUnmeasured: the inferred tags query is non-empty, but
	// the real-corpus root itself is absent on this host, so T1 versus T2
	// cannot be decided here. This tier exists so a missing corpus is
	// reported honestly instead of silently folded into T2.
	OutlineTierOneUnmeasured OutlineTier = "T1_unmeasured"
	// OutlineTierTwo: the inferred tags query is non-empty and, on a host
	// where the real-corpus root is present, this language has no matching
	// real-corpus file. Fixture = the language's dedicated smoke sample.
	OutlineTierTwo OutlineTier = "T2"
	// OutlineTierThree: the inferred tags query is empty, but the grammar
	// defines at least one candidate definition-shape node type drawn from
	// the inference pattern table -- the promotion backlog. Remediation is
	// a data edit to inferredTagsQueryPatterns/inferredTagsQueryOverrides,
	// never a code branch.
	OutlineTierThree OutlineTier = "T3"
	// OutlineTierFour: the inferred tags query is empty and the grammar
	// defines none of the inference table's candidate definition-shape node
	// types. Declined with a receipt; nothing to promote without new data.
	OutlineTierFour OutlineTier = "T4"
)

// outlineRealCorpusRootEnv overrides the real-corpus root. This matches the
// convention every other real-corpus test in this repository already uses
// (grammargen/dfa_minimize_parity_test.go, parser_recover_cycle_sweep_test.go,
// grammars/csharp_external_lex_states_regression_test.go): an external
// sibling checkout, not an in-repo path. cgo_harness/corpus_sources/<name>
// (the path spec.outline-api.v1 section 5 names) corresponds to submodule
// registrations that exist only in local, uncommitted .git/config state on
// some hosts -- no .gitmodules is tracked and no gitlink entries exist in
// the tree, so that path is never populated by a plain clone. The working,
// already-adopted mechanism is this env var, defaulting to a sibling
// checkout of the gotreesitter-corpora repository.
const outlineRealCorpusRootEnv = "GOT_CORPUS_SOURCES_ROOT"

// outlineRealCorpusWalkBudget bounds the per-language directory walk so one
// large corpus checkout cannot make the census slow. Every corpus this repo
// pulls from is a language's own grammar-source repository, so a matching
// file is expected within the first few hundred entries; this budget is
// headroom, not a fit to any single corpus's size.
const outlineRealCorpusWalkBudget = 50000

// OutlineCensusRow is the computed, per-language record the outline-tier
// census produces.
type OutlineCensusRow struct {
	Language          string      `json:"language"`
	Tier              OutlineTier `json:"tier"`
	TagsQueryNonEmpty bool        `json:"tags_query_non_empty"`
	SmokeCaptureCount int         `json:"smoke_capture_count"`
	SmokeCaptureNames []string    `json:"smoke_capture_names,omitempty"`
	SmokeParseError   string      `json:"smoke_parse_error,omitempty"`
	RealCorpusPresent bool        `json:"real_corpus_present"`
	CandidateSymbols  []string    `json:"candidate_symbols,omitempty"`
}

// OutlineCensusResult is the full, stable census output.
type OutlineCensusResult struct {
	ReproductionCommand  string             `json:"reproduction_command"`
	LanguageCount        int                `json:"language_count"`
	RealCorpusRoot       string             `json:"real_corpus_root"`
	RealCorpusRootExists bool               `json:"real_corpus_root_exists"`
	CountsByTier         map[string]int     `json:"counts_by_tier"`
	Rows                 []OutlineCensusRow `json:"rows"`
}

// outlineCandidateDefinitionSymbols returns the deduplicated set of outer
// (definition-shape) node types referenced by inferredTagsQueryPatterns, in
// table order. These are the node types the T3 promotion backlog measures
// grammars against.
func outlineCandidateDefinitionSymbols() []string {
	seen := make(map[string]struct{}, len(inferredTagsQueryPatterns))
	out := make([]string, 0, len(inferredTagsQueryPatterns))
	for _, pattern := range inferredTagsQueryPatterns {
		if len(pattern.Symbols) == 0 {
			continue
		}
		sym := pattern.Symbols[0]
		if _, ok := seen[sym]; ok {
			continue
		}
		seen[sym] = struct{}{}
		out = append(out, sym)
	}
	return out
}

// outlineResolveRealCorpusRoot resolves the real-corpus root the same way
// the existing real-corpus tests do: an explicit env override, else a
// sibling checkout under the user's home directory.
func outlineResolveRealCorpusRoot() string {
	if root := strings.TrimSpace(os.Getenv(outlineRealCorpusRootEnv)); root != "" {
		return root
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, "work", "gotreesitter-corpora", "corpus_sources")
}

// errOutlineCorpusMatchFound is a sentinel walk error that stops
// outlineRealCorpusHasMatch's directory walk the moment a matching file
// turns up, rather than merely skipping the rest of one directory.
var errOutlineCorpusMatchFound = errStr("outline: corpus match found")

type errStr string

func (e errStr) Error() string { return string(e) }

// outlineRealCorpusHasMatch reports whether root/name holds at least one
// file whose extension is in exts, bounded by outlineRealCorpusWalkBudget
// directory entries. It returns false immediately if root/name does not
// exist or is empty.
func outlineRealCorpusHasMatch(root, name string, exts []string) bool {
	if root == "" || name == "" || len(exts) == 0 {
		return false
	}
	langDir := filepath.Join(root, name)
	info, err := os.Stat(langDir)
	if err != nil || !info.IsDir() {
		return false
	}
	normalized := make([]string, 0, len(exts))
	for _, ext := range exts {
		normalized = append(normalized, strings.ToLower(ext))
	}
	visited := 0
	walkErr := filepath.Walk(langDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // best-effort walk; unreadable entries are skipped, not fatal
		}
		visited++
		if visited > outlineRealCorpusWalkBudget {
			return filepath.SkipAll
		}
		if fi.IsDir() {
			return nil
		}
		lowerName := strings.ToLower(fi.Name())
		for _, ext := range normalized {
			if strings.HasSuffix(lowerName, ext) {
				return errOutlineCorpusMatchFound
			}
		}
		return nil
	})
	return walkErr == errOutlineCorpusMatchFound
}

// outlineSmokeCaptures resolves the tags query, parses the language's smoke
// sample, and runs the query over the resulting tree. It returns the sorted,
// deduplicated capture names and total capture count. A non-nil error string
// records why the sample could not be measured (parse failure, no loader,
// etc); an empty query short-circuits with no error.
func outlineSmokeCaptures(entry LangEntry, query string) (count int, names []string, errMsg string) {
	if strings.TrimSpace(query) == "" {
		return 0, nil, ""
	}
	if entry.Language == nil {
		return 0, nil, "no language loader registered"
	}
	lang := entry.Language()
	if lang == nil {
		return 0, nil, "language loader returned nil"
	}
	q, err := gotreesitter.NewQuery(query, lang)
	if err != nil {
		return 0, nil, "compile tags query: " + err.Error()
	}
	sample := []byte(ParseSmokeSample(entry.Name))
	support := EvaluateParseSupport(entry, lang)
	parser := gotreesitter.NewParser(lang)
	var tree *gotreesitter.Tree
	if support.Backend == ParseBackendTokenSource && entry.TokenSourceFactory != nil {
		tree, err = parser.ParseWithTokenSource(sample, entry.TokenSourceFactory(sample, lang))
	} else {
		tree, err = parser.Parse(sample)
	}
	if err != nil {
		return 0, nil, "parse smoke sample: " + err.Error()
	}
	if tree == nil || tree.RootNode() == nil {
		return 0, nil, "parse smoke sample: nil tree"
	}
	matches := q.Execute(tree)
	nameSet := make(map[string]struct{})
	total := 0
	for _, m := range matches {
		for _, c := range m.Captures {
			total++
			nameSet[c.Name] = struct{}{}
		}
	}
	sortedNames := make([]string, 0, len(nameSet))
	for n := range nameSet {
		sortedNames = append(sortedNames, n)
	}
	sort.Strings(sortedNames)
	return total, sortedNames, ""
}

// computeOutlineTierCensus classifies every entry into exactly one
// OutlineTier and returns the full, stable census result. entries should be
// AllLanguages() in production use; a caller-supplied slice keeps this
// function unit-testable without the full registry.
func computeOutlineTierCensus(entries []LangEntry) OutlineCensusResult {
	corpusRoot := outlineResolveRealCorpusRoot()
	corpusRootExists := false
	if corpusRoot != "" {
		if info, err := os.Stat(corpusRoot); err == nil && info.IsDir() {
			corpusRootExists = true
		}
	}
	candidateSymbols := outlineCandidateDefinitionSymbols()

	rows := make([]OutlineCensusRow, 0, len(entries))
	counts := make(map[string]int, 5)
	for _, entry := range entries {
		query := ResolveTagsQuery(entry)
		nonEmpty := strings.TrimSpace(query) != ""
		captureCount, captureNames, parseErr := outlineSmokeCaptures(entry, query)

		row := OutlineCensusRow{
			Language:          entry.Name,
			TagsQueryNonEmpty: nonEmpty,
			SmokeCaptureCount: captureCount,
			SmokeCaptureNames: captureNames,
			SmokeParseError:   parseErr,
		}

		switch {
		case nonEmpty:
			if !corpusRootExists {
				row.Tier = OutlineTierOneUnmeasured
			} else {
				exts := entry.Extensions
				if outlineRealCorpusHasMatch(corpusRoot, entry.Name, exts) {
					row.Tier = OutlineTierOne
					row.RealCorpusPresent = true
				} else {
					row.Tier = OutlineTierTwo
				}
			}
		default:
			var present []string
			if entry.Language != nil {
				if lang := entry.Language(); lang != nil {
					for _, sym := range candidateSymbols {
						if _, ok := lang.SymbolByName(sym); ok {
							present = append(present, sym)
						}
					}
				}
			}
			row.CandidateSymbols = present
			if len(present) > 0 {
				row.Tier = OutlineTierThree
			} else {
				row.Tier = OutlineTierFour
			}
		}

		counts[string(row.Tier)]++
		rows = append(rows, row)
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Language < rows[j].Language })

	return OutlineCensusResult{
		ReproductionCommand: "GTS_OUTLINE_CENSUS_OUT=harness_out/outline/tier_census.json " +
			"GOFLAGS=-buildvcs=false go test ./grammars -run TestOutlineTierCensus -count=1",
		LanguageCount:        len(entries),
		RealCorpusRoot:       corpusRoot,
		RealCorpusRootExists: corpusRootExists,
		CountsByTier:         counts,
		Rows:                 rows,
	}
}

// writeOutlineCensusJSON marshals result as indented JSON to path, creating
// parent directories as needed.
func writeOutlineCensusJSON(path string, result OutlineCensusResult) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
