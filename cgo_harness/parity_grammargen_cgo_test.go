//go:build cgo && treesitter_c_parity

package cgoharness

// Direct grammargen-vs-C parity test.
//
// This closes the transitive trust gap: instead of relying on
//   grammargen ≈ ts2go-blob (Go runtime) ≈ C runtime
// we directly compare:
//   grammargen blob (Go runtime) vs C runtime
//
// Run with:
//   GTS_GRAMMARGEN_CGO_ENABLE=1 go test . -tags treesitter_c_parity \
//     -run TestGrammargenCGOParity -v -timeout 30m
//
// Requires /tmp/grammar_parity/ with cloned grammar repos (same as grammargen
// parity tests). Use the Docker runner for OOM-safe execution:
//   cgo_harness/docker/run_grammargen_c_parity.sh --memory 8g

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammargen"
	"github.com/odvcencio/gotreesitter/grammars"
)

const (
	grammargenCGOEnableEnv     = "GTS_GRAMMARGEN_CGO_ENABLE"
	grammargenCGORootEnv       = "GTS_GRAMMARGEN_CGO_ROOT"
	grammargenCGOLangsEnv      = "GTS_GRAMMARGEN_CGO_LANGS"
	grammargenCGOMaxCasesEnv   = "GTS_GRAMMARGEN_CGO_MAX_CASES"
	grammargenCGOMaxBytesEnv   = "GTS_GRAMMARGEN_CGO_MAX_BYTES"
	grammargenCGOFloorsPathEnv = "GTS_GRAMMARGEN_CGO_FLOORS_PATH"
	grammargenCGORatchetEnv    = "GTS_GRAMMARGEN_CGO_RATCHET_UPDATE"
	grammargenCGOMaxWalkFiles  = 6000
)

// grammargenCGOGrammar describes a grammar to test through the
// grammargen → Go runtime → C runtime comparison pipeline.
type grammargenCGOGrammar struct {
	name       string
	jsonPath   string // path to grammar.json (preferred)
	jsPath     string // path to grammar.js (fallback)
	blobFunc   func() *gotreesitter.Language
	genTimeout time.Duration
}

// grammargenCGOFloorEntry records per-grammar ratchet metrics.
type grammargenCGOFloorEntry struct {
	Eligible    int `json:"eligible"`
	NoError     int `json:"no_error"`
	TreeParity  int `json:"tree_parity"`
	Divergences int `json:"divergences"`
}

type grammargenCGOFloorFile struct {
	Version      int                                `json:"version"`
	GeneratedAt  string                             `json:"generated_at"`
	CommitSHA    string                             `json:"commit_sha,omitempty"`
	CorpusRoot   string                             `json:"corpus_root"`
	MaxCases     int                                `json:"max_cases"`
	MaxBytes     int                                `json:"max_bytes"`
	GrammarCount int                                `json:"grammar_count"`
	TotalElig    int                                `json:"total_eligible"`
	TotalNoErr   int                                `json:"total_no_error"`
	TotalParity  int                                `json:"total_tree_parity"`
	Metrics      map[string]grammargenCGOFloorEntry `json:"metrics"`
}

// grammargenCGOGrammars lists all grammars that can be imported by grammargen
// and tested against the C reference. Names must match languages.lock entries.
var grammargenCGOGrammars = []grammargenCGOGrammar{
	// ── Languages with C oracle in cgo_harness curated set ──
	{name: "json", jsonPath: "/tmp/grammar_parity/json/src/grammar.json", blobFunc: grammars.JsonLanguage},
	{name: "json5", jsonPath: "/tmp/grammar_parity/json5/src/grammar.json", blobFunc: grammars.Json5Language},
	{name: "css", jsonPath: "/tmp/grammar_parity/css/src/grammar.json", blobFunc: grammars.CssLanguage, genTimeout: 90 * time.Second},
	{name: "html", jsonPath: "/tmp/grammar_parity/html/src/grammar.json", blobFunc: grammars.HtmlLanguage},
	{name: "graphql", jsonPath: "/tmp/grammar_parity/graphql/src/grammar.json", blobFunc: grammars.GraphqlLanguage},
	{name: "toml", jsonPath: "/tmp/grammar_parity/toml/src/grammar.json", blobFunc: grammars.TomlLanguage},
	{name: "ini", jsonPath: "/tmp/grammar_parity/ini/src/grammar.json", blobFunc: grammars.IniLanguage},
	{name: "hcl", jsonPath: "/tmp/grammar_parity/hcl/src/grammar.json", blobFunc: grammars.HclLanguage, genTimeout: 60 * time.Second},
	{name: "nix", jsonPath: "/tmp/grammar_parity/nix/src/grammar.json", blobFunc: grammars.NixLanguage},
	{name: "sql", jsonPath: "/tmp/grammar_parity/sql/src/grammar.json", blobFunc: grammars.SqlLanguage, genTimeout: 90 * time.Second},
	{name: "make", jsonPath: "/tmp/grammar_parity/make/src/grammar.json", blobFunc: grammars.MakeLanguage},
	{name: "scala", jsPath: "/tmp/grammar_parity/scala/grammar.js", blobFunc: grammars.ScalaLanguage, genTimeout: 45 * time.Second},
	{name: "gomod", jsonPath: "/tmp/grammar_parity/gomod/src/grammar.json", blobFunc: grammars.GomodLanguage},
	{name: "go", jsonPath: "/tmp/grammar_parity/go/src/grammar.json", blobFunc: grammars.GoLanguage, genTimeout: 45 * time.Second},
	{name: "c", jsonPath: "/tmp/grammar_parity/c/src/grammar.json", blobFunc: grammars.CLanguage, genTimeout: 60 * time.Second},
	// ── Languages without prior C oracle ──
	{name: "csv", jsonPath: "/tmp/grammar_parity/csv/csv/src/grammar.json", blobFunc: grammars.CsvLanguage},
	{name: "diff", jsonPath: "/tmp/grammar_parity/diff/src/grammar.json", blobFunc: grammars.DiffLanguage},
	{name: "gitcommit", jsonPath: "/tmp/grammar_parity/gitcommit_gbprod/src/grammar.json", blobFunc: grammars.GitcommitLanguage},
	{name: "dot", jsonPath: "/tmp/grammar_parity/dot/src/grammar.json", blobFunc: grammars.DotLanguage},
	{name: "ron", jsonPath: "/tmp/grammar_parity/ron/src/grammar.json", blobFunc: grammars.RonLanguage},
	{name: "proto", jsonPath: "/tmp/grammar_parity/proto/src/grammar.json", blobFunc: grammars.ProtoLanguage},
	{name: "comment", jsonPath: "/tmp/grammar_parity/comment/src/grammar.json", blobFunc: grammars.CommentLanguage},
	{name: "pem", jsonPath: "/tmp/grammar_parity/pem/src/grammar.json", blobFunc: grammars.PemLanguage},
	{name: "dockerfile", jsonPath: "/tmp/grammar_parity/dockerfile/src/grammar.json", blobFunc: grammars.DockerfileLanguage},
	{name: "gitattributes", jsonPath: "/tmp/grammar_parity/gitattributes/src/grammar.json", blobFunc: grammars.GitattributesLanguage},
	{name: "jq", jsonPath: "/tmp/grammar_parity/jq/src/grammar.json", blobFunc: grammars.JqLanguage, genTimeout: 60 * time.Second},
	{name: "regex", jsonPath: "/tmp/grammar_parity/regex/src/grammar.json", blobFunc: grammars.RegexLanguage, genTimeout: 90 * time.Second},
	{name: "eds", jsonPath: "/tmp/grammar_parity/eds/src/grammar.json", blobFunc: grammars.EdsLanguage},
	{name: "eex", jsonPath: "/tmp/grammar_parity/eex/src/grammar.json", blobFunc: grammars.EexLanguage},
	{name: "todotxt", jsonPath: "/tmp/grammar_parity/todotxt/src/grammar.json", blobFunc: grammars.TodotxtLanguage},
	{name: "git_rebase", jsonPath: "/tmp/grammar_parity/git_rebase/src/grammar.json", blobFunc: grammars.GitRebaseLanguage},
	{name: "gitignore", jsonPath: "/tmp/grammar_parity/gitignore/src/grammar.json", blobFunc: grammars.GitignoreLanguage},
	{name: "git_config", jsonPath: "/tmp/grammar_parity/git_config/src/grammar.json", blobFunc: grammars.GitConfigLanguage},
	{name: "forth", jsonPath: "/tmp/grammar_parity/forth/src/grammar.json", blobFunc: grammars.ForthLanguage},
	{name: "cpon", jsonPath: "/tmp/grammar_parity/cpon/src/grammar.json", blobFunc: grammars.CponLanguage},
	{name: "scheme", jsonPath: "/tmp/grammar_parity/scheme/src/grammar.json", blobFunc: grammars.SchemeLanguage},
	{name: "textproto", jsonPath: "/tmp/grammar_parity/textproto/src/grammar.json", blobFunc: grammars.TextprotoLanguage},
	{name: "promql", jsonPath: "/tmp/grammar_parity/promql/src/grammar.json", blobFunc: grammars.PromqlLanguage, genTimeout: 60 * time.Second},
	{name: "jsdoc", jsonPath: "/tmp/grammar_parity/jsdoc/src/grammar.json", blobFunc: grammars.JsdocLanguage},
	{name: "properties", jsonPath: "/tmp/grammar_parity/properties/src/grammar.json", blobFunc: grammars.PropertiesLanguage},
	{name: "requirements", jsonPath: "/tmp/grammar_parity/requirements/src/grammar.json", blobFunc: grammars.RequirementsLanguage},
	{name: "ssh_config", jsonPath: "/tmp/grammar_parity/ssh_config/src/grammar.json", blobFunc: grammars.SshConfigLanguage, genTimeout: 45 * time.Second},
	{name: "corn", jsonPath: "/tmp/grammar_parity/corn/src/grammar.json", blobFunc: grammars.CornLanguage},
}

// TestGrammargenCGOParity generates a Language via grammargen, parses
// real-world corpus samples with the Go runtime, then compares node-by-node
// against the C reference parser from languages.lock.
//
// This is the single-hop direct oracle test: grammargen blob vs C runtime.
func TestGrammargenCGOParity(t *testing.T) {
	if !envBool(grammargenCGOEnableEnv, false) {
		t.Skipf("set %s=1 to enable grammargen direct C parity", grammargenCGOEnableEnv)
	}

	root := strings.TrimSpace(os.Getenv(grammargenCGORootEnv))
	if root == "" {
		root = "/tmp/grammar_parity"
	}
	if _, err := os.Stat(root); err != nil {
		t.Skipf("grammar root unavailable: %s (%v)", root, err)
	}

	maxCases := envInt(grammargenCGOMaxCasesEnv, 20)
	maxBytes := envInt(grammargenCGOMaxBytesEnv, 256*1024)
	updateRatchet := envBool(grammargenCGORatchetEnv, false)
	langFilter := parseLangFilter(os.Getenv(grammargenCGOLangsEnv))

	floorsPath := strings.TrimSpace(os.Getenv(grammargenCGOFloorsPathEnv))
	if floorsPath == "" {
		floorsPath = defaultGrammargenCGOFloorsPath()
	}
	floors, foundFloors, err := loadGrammargenCGOFloors(floorsPath)
	if err != nil {
		t.Fatalf("load floor file %s: %v", floorsPath, err)
	}

	testedGrammars := 0
	totalElig := 0
	totalNoErr := 0
	totalParity := 0
	observed := map[string]grammargenCGOFloorEntry{}

	for _, g := range grammargenCGOGrammars {
		if len(langFilter) > 0 && !langFilter[g.name] {
			continue
		}

		g := g
		t.Run(g.name, func(t *testing.T) {
			// Stage 1: Import grammar source.
			gram, err := importGrammargenSource(g)
			if err != nil {
				t.Skipf("import unavailable: %v", err)
				return
			}

			// Stage 2: Generate Language with timeout.
			timeout := g.genTimeout
			if timeout == 0 {
				timeout = 30 * time.Second
			}
			genLang, err := grammargenGenerate(gram, timeout)
			if err != nil {
				t.Skipf("generate failed: %v", err)
				return
			}

			// Adapt external scanner from ts2go blob if grammargen
			// produced external symbols.
			refLang := g.blobFunc()
			if refLang.ExternalScanner != nil && len(genLang.ExternalSymbols) > 0 {
				if scanner, ok := gotreesitter.AdaptExternalScannerByExternalOrder(refLang, genLang); ok {
					genLang.ExternalScanner = scanner
				}
			}

			// Stage 3: Load C reference parser.
			cLang, err := ParityCLanguage(g.name)
			if err != nil {
				if skipReason := parityReferenceSkipReason(err); skipReason != "" {
					t.Skipf("skip C reference: %s", skipReason)
					return
				}
				t.Fatalf("load C parser: %v", err)
			}
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				if skipReason := parityReferenceSkipReason(err); skipReason != "" {
					t.Skipf("skip C parser SetLanguage: %s", skipReason)
					return
				}
				t.Fatalf("C parser SetLanguage: %v", err)
			}

			// Stage 4: Collect corpus samples.
			candidates := collectGrammargenCorpusSamples(t, g, root, maxCases*8, maxBytes)
			if len(candidates) == 0 {
				t.Skip("no corpus samples found")
				return
			}

			genParser := gotreesitter.NewParser(genLang)
			metrics := grammargenCGOFloorEntry{}
			mismatchLogs := 0

			for i, sample := range candidates {
				if metrics.Eligible >= maxCases {
					break
				}

				// Parse with C reference.
				cTree := cParser.Parse([]byte(sample), nil)
				if cTree == nil || cTree.RootNode() == nil {
					continue
				}
				cRoot := cTree.RootNode()
				if cRoot.HasError() {
					cTree.Close()
					continue
				}
				metrics.Eligible++

				// Parse with grammargen Go runtime.
				genTree, _ := genParser.Parse([]byte(sample))
				genRoot := genTree.RootNode()
				if genRoot.HasError() {
					metrics.Divergences++
					cTree.Close()
					if mismatchLogs < 3 {
						mismatchLogs++
						t.Logf("sample %d: grammargen has ERROR, C is clean", i)
					}
					continue
				}
				metrics.NoError++

				// Node-by-node structural comparison.
				var errs []string
				compareGrammargenVsC(genRoot, genLang, cRoot, "root", &errs)
				cTree.Close()

				if len(errs) == 0 {
					metrics.TreeParity++
				} else {
					metrics.Divergences += len(errs)
					if mismatchLogs < 3 {
						mismatchLogs++
						shown := errs
						if len(shown) > 5 {
							shown = shown[:5]
						}
						t.Logf("sample %d: %d divergence(s): %s", i, len(errs), strings.Join(shown, "; "))
					}
				}
			}

			if metrics.Eligible == 0 {
				t.Skip("no clean C-reference samples in corpus")
				return
			}

			// Ratchet enforcement.
			if !updateRatchet && foundFloors {
				if floor, ok := floors.Metrics[g.name]; ok {
					enforceGrammargenCGORatchet(t, g.name, floor, metrics)
				}
			}

			observed[g.name] = metrics
			totalElig += metrics.Eligible
			totalNoErr += metrics.NoError
			totalParity += metrics.TreeParity
			testedGrammars++

			t.Logf("grammargen-vs-C: eligible=%d no_error=%d tree_parity=%d divergences=%d",
				metrics.Eligible, metrics.NoError, metrics.TreeParity, metrics.Divergences)
		})
	}

	if testedGrammars == 0 {
		t.Skipf("no grammars tested (root=%s)", root)
	}

	if updateRatchet && len(observed) > 0 {
		writeGrammargenCGOFloors(t, floorsPath, floors, observed, root, maxCases, maxBytes)
	}

	t.Logf("GRAMMARGEN-CGO SUMMARY: grammars=%d eligible=%d no_error=%d tree_parity=%d",
		testedGrammars, totalElig, totalNoErr, totalParity)
}

// compareGrammargenVsC walks a grammargen Go tree and a C sitter tree in
// lockstep, recording structural divergences.
func compareGrammargenVsC(goNode *gotreesitter.Node, goLang *gotreesitter.Language, cNode *sitter.Node, path string, errs *[]string) {
	if len(*errs) >= 20 {
		return
	}

	goType := goNode.Type(goLang)
	cType := cNode.Kind()
	if goType != cType {
		*errs = append(*errs, fmt.Sprintf("%s: Type gen=%q c=%q", path, goType, cType))
		return
	}
	if uint(goNode.StartByte()) != cNode.StartByte() {
		*errs = append(*errs, fmt.Sprintf("%s: StartByte gen=%d c=%d", path, goNode.StartByte(), cNode.StartByte()))
	}
	if uint(goNode.EndByte()) != cNode.EndByte() {
		*errs = append(*errs, fmt.Sprintf("%s: EndByte gen=%d c=%d", path, goNode.EndByte(), cNode.EndByte()))
	}
	if goNode.IsNamed() != cNode.IsNamed() {
		*errs = append(*errs, fmt.Sprintf("%s: IsNamed gen=%v c=%v", path, goNode.IsNamed(), cNode.IsNamed()))
	}

	goCC := goNode.ChildCount()
	cCC := int(cNode.ChildCount())
	if goCC != cCC {
		*errs = append(*errs, fmt.Sprintf("%s: ChildCount gen=%d c=%d", path, goCC, cCC))
		return
	}
	for i := 0; i < goCC; i++ {
		childPath := fmt.Sprintf("%s[%d]", path, i)
		goChild := goNode.Child(i)
		cChild := cNode.Child(uint(i))
		if goChild == nil || cChild == nil {
			if goChild != nil || cChild != nil {
				*errs = append(*errs, fmt.Sprintf("%s: nil mismatch", childPath))
			}
			continue
		}
		compareGrammargenVsC(goChild, goLang, cChild, childPath, errs)
	}
}

func importGrammargenSource(g grammargenCGOGrammar) (*grammargen.Grammar, error) {
	if g.jsonPath != "" {
		data, err := os.ReadFile(g.jsonPath)
		if err != nil {
			return nil, err
		}
		return grammargen.ImportGrammarJSON(data)
	}
	data, err := os.ReadFile(g.jsPath)
	if err != nil {
		return nil, err
	}
	return grammargen.ImportGrammarJS(data)
}

func grammargenGenerate(gram *grammargen.Grammar, timeout time.Duration) (*gotreesitter.Language, error) {
	type result struct {
		lang *gotreesitter.Language
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		lang, err := grammargen.GenerateLanguage(gram)
		ch <- result{lang, err}
	}()
	select {
	case r := <-ch:
		return r.lang, r.err
	case <-time.After(timeout):
		return nil, fmt.Errorf("generation timed out after %v", timeout)
	}
}

// collectGrammargenCorpusSamples gathers test inputs from the grammar's
// repository: tree-sitter corpus blocks, highlight fixtures, raw examples.
func collectGrammargenCorpusSamples(t *testing.T, g grammargenCGOGrammar, root string, limit, maxBytes int) []string {
	t.Helper()

	repoRoot := grammargenRepoRoot(g, root)
	if repoRoot == "" {
		return nil
	}

	corpusDirs := existingSubdirs(repoRoot, []string{
		"test/corpus", "tests/corpus", "corpus",
	})
	sampleDirs := existingSubdirs(repoRoot, []string{
		"examples", "example", "samples", "fixtures",
		"test/highlight", "tests/highlight",
		"test/highlights", "tests/highlights",
		"test/fixtures", "tests/fixtures",
	})

	seen := map[string]struct{}{}
	var out []string

	// Corpus blocks first (highest signal).
	for _, dir := range corpusDirs {
		for _, path := range walkFiles(dir) {
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			for _, block := range extractCorpusBlocks(data) {
				if s, ok := cleanSample(block, maxBytes, seen); ok {
					out = append(out, s)
				}
			}
			if len(extractCorpusBlocks(data)) == 0 {
				if s, ok := cleanSample(string(data), maxBytes, seen); ok {
					out = append(out, s)
				}
			}
		}
	}

	// Raw sample files.
	for _, dir := range sampleDirs {
		for _, path := range walkFiles(dir) {
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			if s, ok := cleanSample(string(data), maxBytes, seen); ok {
				out = append(out, s)
			}
		}
	}

	// Sort: smaller first for fast feedback.
	sort.Slice(out, func(i, j int) bool { return len(out[i]) < len(out[j]) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func grammargenRepoRoot(g grammargenCGOGrammar, root string) string {
	for _, p := range []string{g.jsonPath, g.jsPath} {
		if p == "" {
			continue
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "../") || rel == ".." {
			continue
		}
		parts := strings.Split(rel, "/")
		if len(parts) == 0 || parts[0] == "." || parts[0] == "" {
			continue
		}
		repoRoot := filepath.Join(root, parts[0])
		if info, statErr := os.Stat(repoRoot); statErr == nil && info.IsDir() {
			return repoRoot
		}
	}
	return ""
}

func existingSubdirs(root string, subs []string) []string {
	var out []string
	for _, sub := range subs {
		dir := filepath.Join(root, sub)
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			out = append(out, dir)
		}
	}
	return out
}

func walkFiles(dir string) []string {
	var out []string
	scanned := 0
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "target" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		scanned++
		if scanned > grammargenCGOMaxWalkFiles {
			return fs.SkipAll
		}
		out = append(out, path)
		return nil
	})
	sort.Strings(out)
	return out
}

func cleanSample(text string, maxBytes int, seen map[string]struct{}) (string, bool) {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	trimmed := strings.TrimSpace(normalized)
	if trimmed == "" || len(trimmed) > maxBytes || !utf8.ValidString(normalized) || strings.ContainsRune(normalized, '\x00') {
		return "", false
	}
	if _, ok := seen[trimmed]; ok {
		return "", false
	}
	seen[trimmed] = struct{}{}
	return normalized, true
}

func extractCorpusBlocks(data []byte) []string {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	var out []string
	for i := 0; i < len(lines); {
		if !isEqualsFence(lines[i]) {
			i++
			continue
		}
		i++
		for i < len(lines) && !isEqualsFence(lines[i]) {
			i++
		}
		if i >= len(lines) {
			break
		}
		i++
		for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
			i++
		}
		start := i
		for i < len(lines) && strings.TrimSpace(lines[i]) != "---" {
			i++
		}
		if i > start {
			src := strings.Trim(strings.Join(lines[start:i], "\n"), "\n")
			if strings.TrimSpace(src) != "" {
				out = append(out, src)
			}
		}
		if i < len(lines) {
			i++
		}
	}
	return out
}

func isEqualsFence(line string) bool {
	s := strings.TrimSpace(line)
	if len(s) < 3 {
		return false
	}
	for _, r := range s {
		if r != '=' {
			return false
		}
	}
	return true
}

func enforceGrammargenCGORatchet(t *testing.T, name string, floor, cur grammargenCGOFloorEntry) {
	t.Helper()
	for _, msg := range grammargenCGORatchetRegressions(name, floor, cur) {
		t.Error(msg)
	}
}

func grammargenCGORatchetRegressions(name string, floor, cur grammargenCGOFloorEntry) []string {
	var out []string
	if cur.Eligible < floor.Eligible {
		out = append(out, fmt.Sprintf("ratchet regression [%s] eligible: %d < floor %d", name, cur.Eligible, floor.Eligible))
	}
	if cur.NoError < floor.NoError {
		out = append(out, fmt.Sprintf("ratchet regression [%s] no_error: %d < floor %d", name, cur.NoError, floor.NoError))
	}
	if cur.TreeParity < floor.TreeParity {
		out = append(out, fmt.Sprintf("ratchet regression [%s] tree_parity: %d < floor %d", name, cur.TreeParity, floor.TreeParity))
	}
	if floor.Divergences > 0 && cur.Divergences > floor.Divergences {
		out = append(out, fmt.Sprintf("ratchet regression [%s] divergences: %d > floor %d", name, cur.Divergences, floor.Divergences))
	}
	return out
}

func parseLangFilter(raw string) map[string]bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimSpace(part)
		if name != "" {
			out[name] = true
		}
	}
	return out
}

// envBool and envInt are defined in parity_breaker_test.go.

func defaultGrammargenCGOFloorsPath() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "cgo_harness/testdata/grammargen_cgo_parity_floors.json"
	}
	return filepath.Join(filepath.Dir(file), "testdata", "grammargen_cgo_parity_floors.json")
}

func loadGrammargenCGOFloors(path string) (grammargenCGOFloorFile, bool, error) {
	var out grammargenCGOFloorFile
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			out.Metrics = map[string]grammargenCGOFloorEntry{}
			return out, false, nil
		}
		return out, false, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, false, err
	}
	if out.Metrics == nil {
		out.Metrics = map[string]grammargenCGOFloorEntry{}
	}
	return out, true, nil
}

func mergeGrammargenCGOFloors(existing, observed map[string]grammargenCGOFloorEntry) map[string]grammargenCGOFloorEntry {
	merged := make(map[string]grammargenCGOFloorEntry, len(existing)+len(observed))
	for name, entry := range existing {
		merged[name] = entry
	}
	for name, cur := range observed {
		if prev, ok := merged[name]; ok {
			if cur.Eligible < prev.Eligible {
				cur.Eligible = prev.Eligible
			}
			if cur.NoError < prev.NoError {
				cur.NoError = prev.NoError
			}
			if cur.TreeParity < prev.TreeParity {
				cur.TreeParity = prev.TreeParity
			}
			if prev.Divergences > 0 && (cur.Divergences == 0 || prev.Divergences < cur.Divergences) {
				cur.Divergences = prev.Divergences
			}
		}
		merged[name] = cur
	}
	return merged
}

func writeGrammargenCGOFloors(t *testing.T, path string, existing grammargenCGOFloorFile, observed map[string]grammargenCGOFloorEntry, root string, maxCases, maxBytes int) {
	t.Helper()
	merged := mergeGrammargenCGOFloors(existing.Metrics, observed)
	grammarCount := len(merged)
	totalElig := 0
	totalNoErr := 0
	totalParity := 0
	for _, entry := range merged {
		totalElig += entry.Eligible
		totalNoErr += entry.NoError
		totalParity += entry.TreeParity
	}
	out := grammargenCGOFloorFile{
		Version:      1,
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		CorpusRoot:   root,
		MaxCases:     maxCases,
		MaxBytes:     maxBytes,
		GrammarCount: grammarCount,
		TotalElig:    totalElig,
		TotalNoErr:   totalNoErr,
		TotalParity:  totalParity,
		Metrics:      merged,
	}
	if sha := gitShortSHA(12); sha != "" {
		out.CommitSHA = sha
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for floors: %v", err)
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatalf("marshal floors: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write floors: %v", err)
	}
	t.Logf("wrote floor file: %s", path)
}

func gitShortSHA(n int) string {
	if n <= 0 {
		n = 12
	}
	out, err := exec.Command("git", "rev-parse", fmt.Sprintf("--short=%d", n), "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
