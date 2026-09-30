//go:build cgo && treesitter_c_parity

// Command gts_grammar_receipt emits one gts-grammar-receipt/v1 JSON file for
// one grammar. It is built and run in the cgo harness Docker image so the
// recorded C identity and parity evidence use the locked oracle toolchain.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	gotreesitter "github.com/odvcencio/gotreesitter"
	cgo_harness "github.com/odvcencio/gotreesitter/cgo_harness"
	"github.com/odvcencio/gotreesitter/grammars"
	leanpkg "github.com/odvcencio/gotreesitter/grammars/lean"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	"github.com/odvcencio/gotreesitter/internal/grammarreceipt"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const (
	defaultCorpusRoot = "/corpus_sources"
	maxCorpusFiles    = 4
	maxCorpusFileSize = 1 << 10
	invariantSteps    = 72
	stepsPerEditClass = 24
	sitesPerEditClass = 16
)

var cohortOne = map[string]bool{
	"go":       true,
	"c_sharp":  true,
	"elixir":   true,
	"html":     true,
	"markdown": true,
	"php":      true,
	"python":   true,
}

var cohortFourE = map[string]bool{"ada": true, "apex": true, "jsdoc": true, "meson": true}

func main() {
	var grammarName, outputPath, repoRoot, corpusRoot, corpusLock, expectedLockSHA, leanOracleLock, gotreesitterCommit string
	var gitTreeDirty bool
	var recordTimeout bool
	var timeoutLimit string
	flag.StringVar(&grammarName, "grammar", "", "one grammar name (required)")
	flag.StringVar(&outputPath, "out", "", "receipt JSON output path (required)")
	flag.StringVar(&repoRoot, "repo-root", "..", "gotreesitter repository root")
	flag.StringVar(&corpusRoot, "corpus-root", defaultCorpusRoot, "pinned corpus source checkout root")
	flag.StringVar(&corpusLock, "corpus-lock", "", "corpus_sources.lock path")
	flag.StringVar(&expectedLockSHA, "corpus-lock-sha256", "", "expected SHA-256 for the complete corpus lock")
	flag.StringVar(&leanOracleLock, "lean-c-oracle-lock", "", "extra C grammar lock for Lean 4")
	flag.StringVar(&gotreesitterCommit, "gotreesitter-commit", "", "host-verified gotreesitter Git commit")
	flag.BoolVar(&gitTreeDirty, "git-tree-dirty", false, "whether the host worktree had changes")
	flag.BoolVar(&recordTimeout, "record-timeout", false, "write an explicit timeout receipt without running parity")
	flag.StringVar(&timeoutLimit, "time-limit", "", "per-grammar wall-time limit recorded for a timeout receipt")
	flag.Parse()
	if flag.NArg() != 0 || grammarName == "" || outputPath == "" || corpusLock == "" {
		flag.Usage()
		os.Exit(2)
	}
	var err error
	if recordTimeout {
		if timeoutLimit == "" {
			fmt.Fprintln(os.Stderr, "gts_grammar_receipt: -time-limit is required with -record-timeout")
			os.Exit(2)
		}
		err = writeTimeoutReceipt(grammarName, outputPath, repoRoot, corpusRoot, corpusLock, expectedLockSHA, leanOracleLock, gotreesitterCommit, gitTreeDirty, timeoutLimit)
	} else {
		err = run(grammarName, outputPath, repoRoot, corpusRoot, corpusLock, expectedLockSHA, leanOracleLock, gotreesitterCommit, gitTreeDirty)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gts_grammar_receipt:", err)
		os.Exit(1)
	}
}

func isUnavailableCorpusSample(err error, corpusRoot, name string) bool {
	if err == nil {
		return false
	}
	if strings.Contains(err.Error(), "no corpus files selected for "+name) {
		return true
	}
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) || !errors.Is(err, os.ErrNotExist) {
		return false
	}
	grammarRoot, err := filepath.Abs(filepath.Join(corpusRoot, name))
	if err != nil {
		return false
	}
	missingPath, err := filepath.Abs(pathErr.Path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(grammarRoot, missingPath)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func run(name, outputPath, repoRoot, corpusRoot, corpusLock, expectedLockSHA, leanOracleLock, gotreesitterCommit string, gitTreeDirty bool) error {
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	if err := os.Chdir(root); err != nil {
		return fmt.Errorf("change to repository root: %w", err)
	}
	name = strings.TrimSpace(name)
	entry, ok := findGrammar(name)
	if !ok {
		return fmt.Errorf("unknown grammar %q", name)
	}
	lang := entry.Language()
	if lang == nil {
		return fmt.Errorf("grammar %q returned a nil language", name)
	}
	top50Ranks, err := loadTop50Ranks(root)
	if err != nil {
		return err
	}
	if name == "lean" {
		if strings.TrimSpace(leanOracleLock) == "" {
			return errors.New("Lean receipt requires -lean-c-oracle-lock")
		}
		if err := os.Setenv("GTS_PARITY_EXTRA_LOCK", leanOracleLock); err != nil {
			return fmt.Errorf("set Lean C oracle lock: %w", err)
		}
	}
	gotreesitter.SetGLRForestEnabled(true)

	head := strings.TrimSpace(gotreesitterCommit)
	if head == "" {
		var err error
		head, gitTreeDirty, err = gitIdentity(root)
		if err != nil {
			return err
		}
	}
	if !validGitCommit(head) {
		return fmt.Errorf("gotreesitter commit %q is not a full Git commit", head)
	}
	blobPath, err := grammarBlobPath(root, name)
	if err != nil {
		return err
	}
	blobSHA, err := hashFile(blobPath)
	if err != nil {
		return fmt.Errorf("hash grammar blob %s: %w", blobPath, err)
	}

	manifest, err := cgo_harness.MaterializeForestCorpusManifest(cgo_harness.ForestCorpusMaterializeOptions{
		GotreesitterRevision: head,
		CorpusLockPath:       corpusLock,
		CorpusRoot:           corpusRoot,
		Languages:            []string{name},
		RegistryExtensions:   map[string][]string{name: entry.Extensions},
		Selection: cgo_harness.ForestCorpusSelection{
			Order: "largest", MaxFiles: maxCorpusFiles, MaxFileBytes: maxCorpusFileSize,
		},
	})
	corpusUnavailableReason := ""
	if err != nil {
		if !isUnavailableCorpusSample(err, corpusRoot, name) {
			return fmt.Errorf("select pinned corpus: %w", err)
		}
		var emptyErr error
		manifest, emptyErr = emptyCorpusManifest(head, corpusLock)
		if emptyErr != nil {
			return emptyErr
		}
		corpusUnavailableReason = err.Error()
	}
	if expectedLockSHA != "" && !strings.EqualFold(manifest.CorpusLock.SHA256, strings.TrimSpace(expectedLockSHA)) {
		return fmt.Errorf("corpus lock sha256 %s, want %s", manifest.CorpusLock.SHA256, expectedLockSHA)
	}
	corpusJSON, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encode corpus manifest identity: %w", err)
	}
	files := make([]selectedFile, 0, len(manifest.Files))
	for _, file := range manifest.Files {
		path := filepath.Join(corpusRoot, name, filepath.FromSlash(file.Path))
		source, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read selected corpus %s: %w", file.Path, err)
		}
		files = append(files, selectedFile{identity: file, path: path, source: source})
	}
	sort.Slice(files, func(i, j int) bool {
		if len(files[i].source) != len(files[j].source) {
			return len(files[i].source) > len(files[j].source)
		}
		return files[i].identity.Path < files[j].identity.Path
	})

	cIdentity, err := cgo_harness.COracleIdentity(name)
	if err != nil {
		return fmt.Errorf("load locked C oracle identity: %w", err)
	}
	cLang, err := cgo_harness.COracleLanguage(name)
	if err != nil {
		return fmt.Errorf("load locked C grammar: %w", err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		return fmt.Errorf("set locked C grammar: %w", err)
	}

	route, err := routeProbe(entry, lang)
	if err != nil {
		return fmt.Errorf("classify compact route: %w", err)
	}
	cohort := graduationCohort(name, lang, route.Status, top50Ranks)
	if cohort == "" {
		return fmt.Errorf("could not classify graduation cohort for %q", name)
	}
	goParser := newGoParser(lang, route.Status)

	corpus := grammarreceipt.CorpusIdentity{
		LockPath: filepath.Base(corpusLock), LockSHA256: manifest.CorpusLock.SHA256,
		ManifestSHA256: sha256String(corpusJSON),
		Selection:      grammarreceipt.CorpusSelection{Order: "largest", MaxFiles: maxCorpusFiles, MaxFileBytes: maxCorpusFileSize},
	}
	for _, file := range files {
		corpus.Files = append(corpus.Files, grammarreceipt.CorpusFile{
			Repository: file.identity.Repository, Revision: file.identity.Revision,
			Path: file.identity.Path, Bytes: file.identity.Bytes, SHA256: file.identity.SHA256,
		})
	}

	fresh := freshParity(entry, lang, route, files, goParser, cParser)
	if corpusUnavailableReason != "" && fresh.FirstFailure != nil {
		fresh.FirstFailure.Error = corpusUnavailableReason
	}
	addCorpusRouteSamples(&route, fresh.Files)
	incremental, invariant := incrementalGate(entry, lang, files, goParser, cParser)
	if corpusUnavailableReason != "" {
		if incremental.FirstFailure != nil {
			incremental.FirstFailure.Error = corpusUnavailableReason
		}
		if len(invariant.Failures) > 0 {
			invariant.Failures[0].Error = corpusUnavailableReason
		}
	}
	for _, file := range fresh.Files {
		if !file.RootCoversInput && file.GoStopReason == string(gotreesitter.ParseStopAccepted) {
			invariant.RootCoverageFailures++
			appendInvariantFailure(&invariant, grammarreceipt.Failure{Category: "fresh-corpus-root-does-not-cover-input", Path: file.Path, GoValue: file.GoStopReason})
		}
		if !file.ErrorRootHasError {
			invariant.ErrorRootFailures++
			appendInvariantFailure(&invariant, grammarreceipt.Failure{Category: "fresh-corpus-error-root-without-has-error", Path: file.Path})
		}
	}
	if invariant.RootCoverageFailures != 0 || invariant.ErrorRootFailures != 0 {
		invariant.Status = grammarreceipt.ResultFail
	}

	receipt := grammarreceipt.Receipt{
		Schema: grammarreceipt.SchemaV1, GeneratedAt: time.Now().UTC(),
		Grammar:            grammarreceipt.GrammarIdentity{Name: name, BlobSHA256: blobSHA},
		GotreesitterCommit: head, GitTreeDirty: gitTreeDirty,
		COracle: grammarreceipt.COracleIdentity{
			RuntimeVersion: cIdentity.RuntimeVersion, RuntimeCommit: cIdentity.RuntimeCommit,
			SourcesManifestSHA256: cIdentity.SourcesManifestSHA256,
			Grammar:               grammarreceipt.CGrammarArtifact{Repository: cIdentity.GrammarRepo, Commit: cIdentity.GrammarCommit, SHA256: cIdentity.GrammarArtifactSHA256},
			BindingModule:         cIdentity.BindingModule, BindingVersion: cIdentity.BindingVersion,
			BindingCommit: cIdentity.BindingCommit, Transport: cIdentity.Transport,
			CompilerPath: cIdentity.CompilerPath, CompilerVersion: cIdentity.CompilerVersion,
			CompileFlags: cIdentity.GrammarCompileFlags, RuntimeLinkage: cIdentity.RuntimeLinkage,
			GrammarLinkage: cIdentity.GrammarLinkage,
		},
		Cohort: cohort, Route: route, Corpus: corpus,
		FreshParity: fresh, IncrementalParity: incremental, InvariantGate: invariant,
	}
	if err := receipt.Validate(); err != nil {
		return fmt.Errorf("generated invalid receipt: %w", err)
	}
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return fmt.Errorf("encode receipt: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return fmt.Errorf("write receipt: %w", err)
	}
	fmt.Printf("receipt=%s grammar=%s cohort=%s route=%s fresh=%s incremental=%s invariant=%s\n",
		outputPath, name, cohort, route.Status, fresh.Status, incremental.Status, invariant.Status)
	return nil
}

func writeTimeoutReceipt(name, outputPath, repoRoot, corpusRoot, corpusLock, expectedLockSHA, leanOracleLock, gotreesitterCommit string, gitTreeDirty bool, timeLimit string) error {
	limit, err := time.ParseDuration(timeLimit)
	if err != nil || limit <= 0 {
		return fmt.Errorf("invalid timeout limit %q", timeLimit)
	}
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	if err := os.Chdir(root); err != nil {
		return fmt.Errorf("change to repository root: %w", err)
	}
	name = strings.TrimSpace(name)
	entry, ok := findGrammar(name)
	if !ok {
		return fmt.Errorf("unknown grammar %q", name)
	}
	lang := entry.Language()
	if lang == nil {
		return fmt.Errorf("grammar %q returned a nil language", name)
	}
	if name == "lean" {
		if strings.TrimSpace(leanOracleLock) == "" {
			return errors.New("Lean timeout receipt requires -lean-c-oracle-lock")
		}
		if err := os.Setenv("GTS_PARITY_EXTRA_LOCK", leanOracleLock); err != nil {
			return fmt.Errorf("set Lean C oracle lock: %w", err)
		}
	}
	gotreesitter.SetGLRForestEnabled(true)
	commit := strings.TrimSpace(gotreesitterCommit)
	if !validGitCommit(commit) {
		return fmt.Errorf("gotreesitter commit %q is not a full Git commit", commit)
	}
	blobPath, err := grammarBlobPath(root, name)
	if err != nil {
		return err
	}
	blobSHA, err := hashFile(blobPath)
	if err != nil {
		return fmt.Errorf("hash grammar blob %s: %w", blobPath, err)
	}
	top50Ranks, err := loadTop50Ranks(root)
	if err != nil {
		return err
	}
	manifest, err := cgo_harness.MaterializeForestCorpusManifest(cgo_harness.ForestCorpusMaterializeOptions{
		GotreesitterRevision: commit,
		CorpusLockPath:       corpusLock,
		CorpusRoot:           corpusRoot,
		Languages:            []string{name},
		RegistryExtensions:   map[string][]string{name: entry.Extensions},
		Selection:            cgo_harness.ForestCorpusSelection{Order: "largest", MaxFiles: maxCorpusFiles, MaxFileBytes: maxCorpusFileSize},
	})
	if err != nil {
		// A grammar with no eligible sample still gets a receipt, the same as
		// in the normal path: an empty manifest that pins the corpus lock.
		if !isUnavailableCorpusSample(err, corpusRoot, name) {
			return fmt.Errorf("select pinned corpus: %w", err)
		}
		var emptyErr error
		manifest, emptyErr = emptyCorpusManifest(commit, corpusLock)
		if emptyErr != nil {
			return emptyErr
		}
	}
	if expectedLockSHA != "" && !strings.EqualFold(manifest.CorpusLock.SHA256, strings.TrimSpace(expectedLockSHA)) {
		return fmt.Errorf("corpus lock sha256 %s, want %s", manifest.CorpusLock.SHA256, expectedLockSHA)
	}
	corpusJSON, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encode corpus manifest identity: %w", err)
	}
	cIdentity, err := cgo_harness.COracleIdentity(name)
	if err != nil {
		return fmt.Errorf("load locked C oracle identity: %w", err)
	}
	route, err := routeProbe(entry, lang)
	if err != nil {
		return fmt.Errorf("classify compact route: %w", err)
	}
	cohort := graduationCohort(name, lang, route.Status, top50Ranks)
	if cohort == "" {
		return fmt.Errorf("could not classify graduation cohort for %q", name)
	}
	corpus := grammarreceipt.CorpusIdentity{
		LockPath: filepath.Base(corpusLock), LockSHA256: manifest.CorpusLock.SHA256,
		ManifestSHA256: sha256String(corpusJSON),
		Selection:      grammarreceipt.CorpusSelection{Order: "largest", MaxFiles: maxCorpusFiles, MaxFileBytes: maxCorpusFileSize},
	}
	for _, file := range manifest.Files {
		corpus.Files = append(corpus.Files, grammarreceipt.CorpusFile{
			Repository: file.Repository, Revision: file.Revision, Path: file.Path, Bytes: file.Bytes, SHA256: file.SHA256,
		})
	}
	failure := &grammarreceipt.Failure{Category: "timeout", Error: "Docker wall-time limit reached before parity and invariant checks completed"}
	cOracle := grammarreceipt.COracleIdentity{
		RuntimeVersion: cIdentity.RuntimeVersion, RuntimeCommit: cIdentity.RuntimeCommit,
		SourcesManifestSHA256: cIdentity.SourcesManifestSHA256,
		Grammar:               grammarreceipt.CGrammarArtifact{Repository: cIdentity.GrammarRepo, Commit: cIdentity.GrammarCommit, SHA256: cIdentity.GrammarArtifactSHA256},
		BindingModule:         cIdentity.BindingModule, BindingVersion: cIdentity.BindingVersion, BindingCommit: cIdentity.BindingCommit,
		Transport: cIdentity.Transport, CompilerPath: cIdentity.CompilerPath, CompilerVersion: cIdentity.CompilerVersion,
		CompileFlags: cIdentity.GrammarCompileFlags, RuntimeLinkage: cIdentity.RuntimeLinkage, GrammarLinkage: cIdentity.GrammarLinkage,
	}
	receipt := grammarreceipt.Receipt{
		Schema: grammarreceipt.SchemaV1, GeneratedAt: time.Now().UTC(),
		Grammar:            grammarreceipt.GrammarIdentity{Name: name, BlobSHA256: blobSHA},
		GotreesitterCommit: commit, GitTreeDirty: gitTreeDirty, COracle: cOracle,
		Cohort: cohort, Route: route, Corpus: corpus,
		Execution:         &grammarreceipt.ExecutionResult{Status: grammarreceipt.ResultTimeout, TimeLimit: timeLimit, Details: failure.Error},
		FreshParity:       grammarreceipt.ParityResult{Status: grammarreceipt.ResultTimeout, Cases: len(manifest.Files), FirstFailure: failure},
		IncrementalParity: grammarreceipt.ParityResult{Status: grammarreceipt.ResultTimeout, Cases: invariantSteps, FirstFailure: failure},
		InvariantGate:     grammarreceipt.InvariantResult{Status: grammarreceipt.ResultTimeout, SessionSteps: invariantSteps, NoEditReparsePass: false, Failures: []grammarreceipt.Failure{*failure}},
	}
	if err := receipt.Validate(); err != nil {
		return fmt.Errorf("generated invalid timeout receipt: %w", err)
	}
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return fmt.Errorf("encode timeout receipt: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return fmt.Errorf("write timeout receipt: %w", err)
	}
	fmt.Printf("recorded timeout receipt=%s grammar=%s time_limit=%s parity=timeout invariant=timeout\n", outputPath, name, timeLimit)
	return nil
}

// emptyCorpusManifest is the manifest for a grammar with no eligible corpus
// sample. It still pins the corpus lock by digest.
func emptyCorpusManifest(revision, corpusLock string) (cgo_harness.ForestCorpusManifest, error) {
	lockSHA, err := hashFile(corpusLock)
	if err != nil {
		return cgo_harness.ForestCorpusManifest{}, fmt.Errorf("hash corpus lock without samples: %w", err)
	}
	return cgo_harness.ForestCorpusManifest{
		Schema:               cgo_harness.ForestCorpusManifestSchema,
		GotreesitterRevision: revision,
		CorpusLock:           cgo_harness.ForestCorpusManifestLock{Path: filepath.Base(corpusLock), SHA256: lockSHA},
		Selection:            cgo_harness.ForestCorpusSelection{Order: "largest", MaxFiles: maxCorpusFiles, MaxFileBytes: maxCorpusFileSize},
		Files:                []cgo_harness.ForestCorpusManifestFile{},
	}, nil
}

type selectedFile struct {
	identity cgo_harness.ForestCorpusManifestFile
	path     string
	source   []byte
}

func findGrammar(name string) (grammars.LangEntry, bool) {
	if name == "lean" {
		return grammars.LangEntry{Name: "lean", Extensions: []string{".lean"}, Language: leanpkg.Language}, true
	}
	for _, entry := range grammars.AllLanguages() {
		if entry.Name == name && entry.Language != nil {
			return entry, true
		}
	}
	return grammars.LangEntry{}, false
}

func grammarBlobPath(root, name string) (string, error) {
	candidates := []string{
		filepath.Join(root, "grammars", "grammar_blobs", name+".bin"),
		filepath.Join(root, "grammars", name, name+".bin"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no embedded grammar blob found for %q", name)
}

func gitIdentity(root string) (string, bool, error) {
	commit, err := gitOutput(root, "rev-parse", "HEAD")
	if err != nil {
		return "", false, fmt.Errorf("read gotreesitter commit: %w", err)
	}
	status, err := gitOutput(root, "status", "--porcelain")
	if err != nil {
		return "", false, fmt.Errorf("read gotreesitter worktree status: %w", err)
	}
	return strings.TrimSpace(commit), status != "", nil
}

func gitOutput(root string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(output), nil
}

func validGitCommit(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func routeProbe(entry grammars.LangEntry, lang *gotreesitter.Language) (grammarreceipt.CompactRoute, error) {
	probeName := "grammars.ParseSmokeSample"
	probeSource := grammars.ParseSmokeSample(entry.Name)
	if entry.Name == "lean" {
		probeName = "lean.TestParseCoreAndNestedComments"
		probeSource = "/- nested /- comment -/ -/\ndef o4ReceiptProbe := 1\n"
	}
	probe := []byte(probeSource)
	route := grammarreceipt.CompactRoute{Probe: probeName, ProbeSHA256: sha256String(probe)}
	if gotreesitter.LanguageWantsForest(lang) {
		route.Status = grammarreceipt.RouteForest
		route.SampledFiles = 1
		route.ForestFiles = 1
		return route, nil
	}
	support := grammars.EvaluateParseSupport(entry, lang)
	if support.Backend != grammars.ParseBackendDFA {
		route.Status = grammarreceipt.RouteDeclined
		route.DeclineReason = support.Reason
		route.SampledFiles = 1
		route.DeclinedFiles = 1
		return route, nil
	}
	parser := gotreesitter.NewParser(lang)
	parser.SetAdmissionCandidateRoute(true)
	gotreesitter.ResetAdmissionCandidateCounters()
	tree, parseErr := parser.Parse(probe)
	usedForest := tree != nil && tree.UsedForestFastPath()
	if tree != nil {
		tree.Release()
	}
	if usedForest {
		route.Status = grammarreceipt.RouteForest
		route.SampledFiles = 1
		route.ForestFiles = 1
		return route, nil
	}
	routed, declined := gotreesitter.AdmissionCandidateCounters()
	if routed > 0 {
		route.Status = grammarreceipt.RouteAccepted
		route.AcceptedFiles = 1
		route.SampledFiles = 1
		return route, nil
	}
	if declined > 0 {
		route.Status = grammarreceipt.RouteDeclined
		route.DeclineReason = gotreesitter.AdmissionCandidateLastFallbackReason()
		route.DeclinedFiles = 1
		route.SampledFiles = 1
		if route.DeclineReason == "" {
			route.DeclineReason = "compact candidate declined without a published reason"
		}
		return route, nil
	}
	route.Status = grammarreceipt.RouteDeclined
	route.DeclineReason = "compact candidate route was not eligible for the smoke probe"
	route.SampledFiles = 1
	route.DeclinedFiles = 1
	if parseErr != nil {
		route.DeclineReason += ": " + parseErr.Error()
	}
	return route, nil
}

func graduationCohort(name string, lang *gotreesitter.Language, route grammarreceipt.RouteStatus, top50Ranks map[string]int) string {
	if name == "lean" {
		return "Lean 4"
	}
	if name == "go" {
		return "1a"
	}
	if cohortOne[name] {
		return "1b"
	}
	if index, ok := top50Ranks[name]; ok {
		if index < 20 {
			return "2"
		}
		return "3"
	}
	if gotreesitter.LanguageWantsForest(lang) {
		return "4-F"
	}
	if cohortFourE[name] {
		return "4-E"
	}
	if lang.ExternalScanner != nil {
		if scanner, ok := lang.ExternalScanner.(gotreesitter.StatelessExternalScanner); ok && scanner.ExternalScannerIsStateless() {
			return "4-B"
		}
		return "4-C"
	}
	if route == grammarreceipt.RouteAccepted {
		return "4-A"
	}
	return "4-D"
}

func loadTop50Ranks(root string) (map[string]int, error) {
	data, err := os.ReadFile(filepath.Join(root, "grammars", "update_tier1_top50.txt"))
	if err != nil {
		return nil, fmt.Errorf("read top-50 cohort roster: %w", err)
	}
	ranks := make(map[string]int, 50)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, exists := ranks[line]; exists {
			return nil, fmt.Errorf("top-50 cohort roster repeats %q", line)
		}
		ranks[line] = len(ranks)
	}
	if len(ranks) != 50 {
		return nil, fmt.Errorf("top-50 cohort roster has %d grammars, want 50", len(ranks))
	}
	return ranks, nil
}

func newGoParser(lang *gotreesitter.Language, route grammarreceipt.RouteStatus) *gotreesitter.Parser {
	parser := gotreesitter.NewParser(lang)
	parser.SetAdmissionCandidateRoute(route != grammarreceipt.RouteForest)
	return parser
}

func freshParity(entry grammars.LangEntry, lang *gotreesitter.Language, route grammarreceipt.CompactRoute, files []selectedFile, parser *gotreesitter.Parser, cParser *sitter.Parser) grammarreceipt.ParityResult {
	result := grammarreceipt.ParityResult{Cases: len(files)}
	if len(files) == 0 {
		result.Status = grammarreceipt.ResultUnavailable
		result.FirstFailure = &grammarreceipt.Failure{Category: "corpus-unavailable", Error: "no selected corpus files"}
		return result
	}
	for _, file := range files {
		fileResult := grammarreceipt.FileResult{
			Path: file.identity.Path, SourceSHA256: file.identity.SHA256, Bytes: len(file.source),
		}
		gotreesitter.ResetAdmissionCandidateCounters()
		goTree, goErr := parseFresh(entry, lang, parser, file.source)
		fileResult.Route, fileResult.DeclineReason = observedRoute(lang, route, goTree)
		if goTree == nil || goTree.RootNode() == nil || goErr != nil {
			fileResult.GoParseError = "Go parse failed"
			if goErr != nil {
				fileResult.GoParseError = goErr.Error()
			}
			result.Errors++
			markFreshFailure(&result, grammarreceipt.Failure{Category: "go-parse-error", Path: file.identity.Path, Error: fileResult.GoParseError})
			if goTree != nil {
				goTree.Release()
			}
			result.Files = append(result.Files, fileResult)
			continue
		}
		root := goTree.RootNode()
		fileResult.GoStopReason = fmt.Sprint(goTree.ParseStopReason())
		fileResult.RootCoversInput = rootCovers(root, len(file.source))
		fileResult.ErrorRootHasError = !root.IsError() || root.HasError()
		goInspection, goDigestErr := benchfixtures.InspectGoTree(root, lang)
		if goDigestErr == nil {
			fileResult.GoTreeSHA256 = goInspection.SHA256
		} else {
			fileResult.GoParseError = goDigestErr.Error()
			result.Errors++
		}
		cTree := cParser.Parse(file.source, nil)
		if cTree == nil || cTree.RootNode() == nil {
			fileResult.CParseError = "locked C parse returned no tree"
			result.Errors++
			markFreshFailure(&result, grammarreceipt.Failure{Category: "c-parse-error", Path: file.identity.Path, Error: fileResult.CParseError})
			goTree.Release()
			if cTree != nil {
				cTree.Close()
			}
			result.Files = append(result.Files, fileResult)
			continue
		}
		fileResult.RootCoversInput = rootCoversLikeC(root, len(file.source), cTree.RootNode())
		cDigest, cDigestErr := cgo_harness.COracleDeepDigest(cTree)
		if cDigestErr == nil {
			fileResult.CTreeSHA256 = cDigest
		} else {
			fileResult.CParseError = cDigestErr.Error()
			result.Errors++
		}
		if goDigestErr == nil && cDigestErr == nil && goInspection.SHA256 == cDigest {
			fileResult.Pass = true
			result.Matched++
		} else {
			fileResult.Divergence = freshDivergence(root, lang, cTree.RootNode(), file.identity.Path, goInspection.SHA256, cDigest)
			markFreshFailure(&result, *fileResult.Divergence)
			result.Mismatched++
		}
		cTree.Close()
		goTree.Release()
		result.Files = append(result.Files, fileResult)
	}
	if result.Errors == 0 && result.Mismatched == 0 {
		result.Status = grammarreceipt.ResultPass
	} else {
		result.Status = grammarreceipt.ResultFail
	}
	return result
}

func markFreshFailure(result *grammarreceipt.ParityResult, failure grammarreceipt.Failure) {
	if result.FirstFailure == nil {
		result.FirstFailure = &failure
	}
}

func observedRoute(lang *gotreesitter.Language, probe grammarreceipt.CompactRoute, tree *gotreesitter.Tree) (grammarreceipt.RouteStatus, string) {
	if probe.Status == grammarreceipt.RouteForest || gotreesitter.LanguageWantsForest(lang) || (tree != nil && tree.UsedForestFastPath()) {
		return grammarreceipt.RouteForest, ""
	}
	routed, declined := gotreesitter.AdmissionCandidateCounters()
	if routed > 0 {
		return grammarreceipt.RouteAccepted, ""
	}
	if declined > 0 {
		reason := gotreesitter.AdmissionCandidateLastFallbackReason()
		if reason == "" {
			reason = "compact candidate declined without a published reason"
		}
		return grammarreceipt.RouteDeclined, reason
	}
	if probe.Status == grammarreceipt.RouteDeclined && probe.DeclineReason != "" {
		return grammarreceipt.RouteDeclined, probe.DeclineReason
	}
	if tree == nil {
		return grammarreceipt.RouteDeclined, "parse did not produce a tree and route counters were not published"
	}
	return grammarreceipt.RouteDeclined, "compact candidate route was not eligible for this corpus file"
}

func addCorpusRouteSamples(route *grammarreceipt.CompactRoute, files []grammarreceipt.FileResult) {
	if route == nil {
		return
	}
	route.SampledFiles = 1 + len(files)
	route.AcceptedFiles, route.DeclinedFiles, route.ForestFiles = 0, 0, 0
	reasons := make(map[string]bool)
	if route.Status == grammarreceipt.RouteAccepted {
		route.AcceptedFiles++
	} else if route.Status == grammarreceipt.RouteDeclined {
		route.DeclinedFiles++
		if route.DeclineReason != "" {
			reasons[route.DeclineReason] = true
		}
	} else if route.Status == grammarreceipt.RouteForest {
		route.ForestFiles++
	}
	for _, file := range files {
		switch file.Route {
		case grammarreceipt.RouteAccepted:
			route.AcceptedFiles++
		case grammarreceipt.RouteDeclined:
			route.DeclinedFiles++
			if file.DeclineReason != "" {
				reasons[file.DeclineReason] = true
			}
		case grammarreceipt.RouteForest:
			route.ForestFiles++
		}
	}
	route.DeclineReasons = route.DeclineReasons[:0]
	for reason := range reasons {
		route.DeclineReasons = append(route.DeclineReasons, reason)
	}
	sort.Strings(route.DeclineReasons)
}

func freshDivergence(goRoot *gotreesitter.Node, lang *gotreesitter.Language, cRoot *sitter.Node, path, goDigest, cDigest string) *grammarreceipt.Failure {
	if divergence := cgo_harness.FirstDivergenceDumpV1(goRoot, lang, cRoot); divergence != nil {
		return &grammarreceipt.Failure{Category: divergence.Category, Path: path + ":" + divergence.Path, GoValue: divergence.GoValue, CValue: divergence.CValue}
	}
	return &grammarreceipt.Failure{Category: "tree-digest", Path: path, GoValue: goDigest, CValue: cDigest}
}

func incrementalGate(entry grammars.LangEntry, lang *gotreesitter.Language, files []selectedFile, parser *gotreesitter.Parser, cParser *sitter.Parser) (grammarreceipt.ParityResult, grammarreceipt.InvariantResult) {
	result := grammarreceipt.ParityResult{Reference: "fresh_c", Cases: invariantSteps, Steps: make([]grammarreceipt.StepResult, 0, invariantSteps)}
	invariant := grammarreceipt.InvariantResult{
		SessionSteps: invariantSteps,
		UniqueSitesByEditClass: map[string]int{
			"insert": 0, "delete": 0, "replace": 0,
		},
	}
	if len(files) == 0 {
		result.Status = grammarreceipt.ResultUnavailable
		result.FirstFailure = &grammarreceipt.Failure{Category: "corpus-unavailable", Error: "no selected corpus files"}
		invariant.Status = grammarreceipt.ResultUnavailable
		invariant.NoEditReparsePass = false
		return result, invariant
	}
	seed := append([]byte(nil), files[0].source...)
	if len(seed) == 0 {
		seed = []byte("\n")
	}
	currentSource := seed
	currentTree, err := parseFresh(entry, lang, parser, currentSource)
	if err != nil || currentTree == nil || currentTree.RootNode() == nil {
		failure := grammarreceipt.Failure{Category: "session-initial-parse", Path: files[0].identity.Path, Error: errorText(err)}
		result.Status = grammarreceipt.ResultFail
		result.FirstFailure = &failure
		invariant.Status = grammarreceipt.ResultFail
		invariant.Failures = append(invariant.Failures, failure)
		return result, invariant
	}
	currentCTree := cParser.Parse(currentSource, nil)

	stepIndex := 0
	siteSets := map[string]map[int]struct{}{
		"insert": {}, "delete": {}, "replace": {},
	}
	for _, editClass := range []string{"insert", "replace", "delete"} {
		for classStep := 0; classStep < stepsPerEditClass; classStep++ {
			start, oldEnd, replacement := selectEdit(currentSource, editClass, classStep%sitesPerEditClass)
			newSource := make([]byte, 0, len(currentSource)-(oldEnd-start)+len(replacement))
			newSource = append(newSource, currentSource[:start]...)
			newSource = append(newSource, replacement...)
			newSource = append(newSource, currentSource[oldEnd:]...)
			edit := gotreesitter.InputEdit{
				StartByte: uint32(start), OldEndByte: uint32(oldEnd), NewEndByte: uint32(start + len(replacement)),
				StartPoint: pointAt(currentSource, start), OldEndPoint: pointAt(currentSource, oldEnd),
				NewEndPoint: pointAt(newSource, start+len(replacement)),
			}
			currentTree.Edit(edit)
			incrementalTree, incErr := parseIncremental(entry, lang, parser, newSource, currentTree)
			if incrementalTree != currentTree {
				currentTree.Release()
			}
			stepIndex++
			siteSets[editClass][classStep%sitesPerEditClass] = struct{}{}
			invariant.UniqueSitesByEditClass[editClass] = len(siteSets[editClass])
			step := grammarreceipt.StepResult{
				Step: stepIndex, EditClass: editClass,
				Site:         fmt.Sprintf("%s:site-%02d", files[0].identity.Path, classStep%sitesPerEditClass),
				SourceSHA256: sha256String(newSource),
			}
			if incErr != nil || incrementalTree == nil || incrementalTree.RootNode() == nil {
				failure := grammarreceipt.Failure{Category: "incremental-parse-error", Path: step.Site, Error: errorText(incErr)}
				step.Failure = &failure
				appendIncrementalFailure(&result, step)
				appendInvariantFailure(&invariant, failure)
				if incrementalTree != nil {
					incrementalTree.Release()
				}
				if currentCTree != nil {
					currentCTree.Close()
					currentCTree = nil
				}
				currentTree, currentSource = nil, newSource
				break
			}
			freshTree, freshErr := parseFresh(entry, lang, parser, newSource)
			if freshErr != nil || freshTree == nil || freshTree.RootNode() == nil {
				failure := grammarreceipt.Failure{Category: "fresh-parse-error", Path: step.Site, Error: errorText(freshErr)}
				step.Failure = &failure
				step.GoIncrementalSHA256 = goTreeDigest(incrementalTree, lang)
				step.GoIncrementalEqualsFresh = false
				appendIncrementalFailure(&result, step)
				appendInvariantFailure(&invariant, failure)
				incrementalTree.Release()
				if freshTree != nil {
					freshTree.Release()
				}
				if currentCTree != nil {
					currentCTree.Close()
					currentCTree = nil
				}
				currentTree, currentSource = nil, newSource
				break
			}
			incDigest, incDigestErr := goDigest(incrementalTree, lang)
			freshDigest, freshDigestErr := goDigest(freshTree, lang)
			step.GoIncrementalSHA256, step.GoFreshSHA256 = incDigest, freshDigest
			step.GoIncrementalEqualsFresh = incDigestErr == nil && freshDigestErr == nil && incDigest == freshDigest
			if !step.GoIncrementalEqualsFresh {
				invariant.IncrementalFreshFailures++
				failure := grammarreceipt.Failure{Category: "incremental-fresh-mismatch", Path: step.Site, GoValue: incDigest, CValue: freshDigest, Error: joinErrors(incDigestErr, freshDigestErr)}
				step.Failure = &failure
				appendInvariantFailure(&invariant, failure)
			}
			incrementalRoot, freshRoot := incrementalTree.RootNode(), freshTree.RootNode()
			cFreshTree := cParser.Parse(newSource, nil)
			var cFreshRoot *sitter.Node
			if cFreshTree != nil {
				cFreshRoot = cFreshTree.RootNode()
			}
			if !rootCoversLikeC(incrementalRoot, len(newSource), cFreshRoot) && incrementalTree.ParseStopReason() == gotreesitter.ParseStopAccepted {
				invariant.RootCoverageFailures++
				failure := grammarreceipt.Failure{Category: "root-does-not-cover-input", Path: step.Site, GoValue: fmt.Sprintf("%d:%d", incrementalRoot.StartByte(), incrementalRoot.EndByte()), Error: fmt.Sprint(incrementalTree.ParseStopReason())}
				appendInvariantFailure(&invariant, failure)
				if step.Failure == nil {
					step.Failure = &failure
				}
			}
			if incrementalRoot.IsError() && !incrementalRoot.HasError() {
				invariant.ErrorRootFailures++
				failure := grammarreceipt.Failure{Category: "error-root-without-has-error", Path: step.Site}
				appendInvariantFailure(&invariant, failure)
				if step.Failure == nil {
					step.Failure = &failure
				}
			}
			if !rootCoversLikeC(freshRoot, len(newSource), cFreshRoot) && freshTree.ParseStopReason() == gotreesitter.ParseStopAccepted {
				invariant.RootCoverageFailures++
				failure := grammarreceipt.Failure{Category: "fresh-root-does-not-cover-input", Path: step.Site, GoValue: fmt.Sprintf("%d:%d", freshRoot.StartByte(), freshRoot.EndByte()), Error: fmt.Sprint(freshTree.ParseStopReason())}
				appendInvariantFailure(&invariant, failure)
				if step.Failure == nil {
					step.Failure = &failure
				}
			}
			if freshRoot.IsError() && !freshRoot.HasError() {
				invariant.ErrorRootFailures++
				failure := grammarreceipt.Failure{Category: "fresh-error-root-without-has-error", Path: step.Site}
				appendInvariantFailure(&invariant, failure)
				if step.Failure == nil {
					step.Failure = &failure
				}
			}
			step.InvariantPass = step.GoIncrementalEqualsFresh &&
				rootCoverageExplained(incrementalRoot, incrementalTree, len(newSource), cFreshRoot) && (!incrementalRoot.IsError() || incrementalRoot.HasError()) &&
				rootCoverageExplained(freshRoot, freshTree, len(newSource), cFreshRoot) && (!freshRoot.IsError() || freshRoot.HasError())
			if step.InvariantPass {
				invariant.StepsPassed++
			}
			cEdit := sitter.InputEdit{
				StartByte: uint(start), OldEndByte: uint(oldEnd), NewEndByte: uint(start + len(replacement)),
				StartPosition: cPoint(edit.StartPoint), OldEndPosition: cPoint(edit.OldEndPoint),
				NewEndPosition: cPoint(edit.NewEndPoint),
			}
			// A failed C incremental parse makes only that diagnostic axis
			// unavailable. Continue the Go session and its fresh-C gate.
			if currentCTree != nil {
				currentCTree.Edit(&cEdit)
				cIncrementalTree := cParser.Parse(newSource, currentCTree)
				currentCTree.Close()
				currentCTree = cIncrementalTree
			}

			cIncrementalDigest, cIncrementalErr := cgo_harness.COracleDeepDigest(currentCTree)
			cFreshDigest, cFreshErr := cgo_harness.COracleDeepDigest(cFreshTree)
			step.CIncrementalSHA256, step.CFreshSHA256 = cIncrementalDigest, cFreshDigest
			step.CIncrementalEqualsFresh = cIncrementalErr == nil && cFreshErr == nil && cIncrementalDigest == cFreshDigest
			step.LockedCParity = incDigestErr == nil && cFreshErr == nil && incDigest == cFreshDigest
			step.LockedCIncrementalParity = incDigestErr == nil && cIncrementalErr == nil && incDigest == cIncrementalDigest
			if cFreshErr != nil {
				step.Failure = &grammarreceipt.Failure{Category: "locked-c-digest-error", Path: step.Site, Error: cFreshErr.Error()}
			} else if !step.LockedCParity && step.Failure == nil {
				divergence := cgo_harness.FirstDivergenceDumpV1(incrementalRoot, lang, cFreshTree.RootNode())
				if divergence != nil {
					step.Failure = &grammarreceipt.Failure{Category: divergence.Category, Path: step.Site + ":" + divergence.Path, GoValue: divergence.GoValue, CValue: divergence.CValue}
				} else {
					step.Failure = &grammarreceipt.Failure{Category: "locked-c-tree-digest-mismatch", Path: step.Site, GoValue: incDigest, CValue: cFreshDigest}
				}
			}
			finishIncrementalStep(&step, cIncrementalErr)

			if cFreshTree != nil {
				cFreshTree.Close()
			}
			if step.Pass {
				result.Matched++
				result.Steps = append(result.Steps, step)
			} else {
				appendIncrementalFailure(&result, step)
			}
			freshTree.Release()
			currentTree = incrementalTree
			currentSource = newSource
		}
		if currentTree == nil {
			break
		}
	}
	if currentCTree != nil {
		currentCTree.Close()
	}
	if currentTree != nil {
		allocs := testing.AllocsPerRun(5, func() {
			unchanged, err := parser.ParseIncremental(currentSource, currentTree)
			if err != nil {
				panic(err)
			}
			if unchanged != nil {
				unchanged.Release()
			}
		})
		invariant.NoEditReparseAllocsPerRun = allocs
		invariant.NoEditReparsePass = math.Abs(allocs) < 0.0001
		if !invariant.NoEditReparsePass {
			appendInvariantFailure(&invariant, grammarreceipt.Failure{Category: "no-edit-reparse-allocations", GoValue: fmt.Sprintf("%.4f", allocs), CValue: "0"})
		}
		currentTree.Release()
	} else {
		invariant.NoEditReparsePass = false
		appendInvariantFailure(&invariant, grammarreceipt.Failure{Category: "no-edit-reparse-unavailable", Error: "edit session did not complete"})
	}
	result.Status = grammarreceipt.ResultPass
	if len(result.Steps) != invariantSteps || result.Matched != invariantSteps {
		result.Status = grammarreceipt.ResultFail
	}
	if result.Status == grammarreceipt.ResultFail && result.FirstFailure == nil && len(invariant.Failures) > 0 {
		failure := invariant.Failures[0]
		result.FirstFailure = &failure
	}
	if invariant.StepsPassed != invariantSteps || invariant.RootCoverageFailures != 0 || invariant.ErrorRootFailures != 0 || invariant.IncrementalFreshFailures != 0 || !invariant.NoEditReparsePass {
		invariant.Status = grammarreceipt.ResultFail
	} else {
		invariant.Status = grammarreceipt.ResultPass
	}
	return result, invariant
}

// finishIncrementalStep keeps the fresh oracle gate independent of C's own
// incremental recovery choices (organization decision 0012).
func finishIncrementalStep(step *grammarreceipt.StepResult, cIncrementalErr error) {
	if cIncrementalErr != nil {
		step.CIncrementalFailure = &grammarreceipt.Failure{Category: "c-incremental-digest-error", Path: step.Site, Error: cIncrementalErr.Error()}
	} else if !step.CIncrementalEqualsFresh {
		step.CIncrementalFailure = &grammarreceipt.Failure{Category: "c-incremental-fresh-mismatch", Path: step.Site, GoValue: step.CIncrementalSHA256, CValue: step.CFreshSHA256}
	}
	if !step.LockedCIncrementalParity {
		step.LockedCIncrementalFailure = &grammarreceipt.Failure{Category: "go-c-incremental-mismatch", Path: step.Site, GoValue: step.GoIncrementalSHA256, CValue: step.CIncrementalSHA256}
	}
	if cIncrementalErr != nil && step.LockedCIncrementalFailure != nil {
		step.LockedCIncrementalFailure.Category = "go-c-incremental-unavailable"
		step.LockedCIncrementalFailure.Error = cIncrementalErr.Error()
	}
	step.Pass = step.InvariantPass && step.GoIncrementalEqualsFresh && step.LockedCParity && step.Failure == nil
}

func appendIncrementalFailure(result *grammarreceipt.ParityResult, step grammarreceipt.StepResult) {
	if step.Failure != nil {
		if result.FirstFailure == nil {
			failure := *step.Failure
			result.FirstFailure = &failure
		}
	}
	result.Mismatched++
	result.Steps = append(result.Steps, step)
}

func appendInvariantFailure(result *grammarreceipt.InvariantResult, failure grammarreceipt.Failure) {
	if len(result.Failures) < 64 {
		result.Failures = append(result.Failures, failure)
	}
}

func selectEdit(source []byte, editClass string, site int) (start, oldEnd int, replacement []byte) {
	length := len(source)
	if editClass == "insert" {
		start = site * (length + 1) / (sitesPerEditClass - 1)
		if start > length {
			start = length
		}
		for start < length && !utf8.RuneStart(source[start]) {
			start++
		}
		return start, start, []byte{'x'}
	}
	if length == 0 {
		return 0, 0, []byte{'x'}
	}
	start = site * length / sitesPerEditClass
	if start >= length {
		start = length - 1
	}
	for start > 0 && !utf8.RuneStart(source[start]) {
		start--
	}
	_, runeSize := utf8.DecodeRune(source[start:])
	if runeSize <= 0 {
		runeSize = 1
	}
	oldEnd = start + runeSize
	if editClass == "replace" {
		value := byte('x')
		if runeSize == 1 && source[start] == value {
			value = 'y'
		}
		return start, oldEnd, []byte{value}
	}
	return start, oldEnd, nil
}

func pointAt(source []byte, offset int) gotreesitter.Point {
	var point gotreesitter.Point
	if offset > len(source) {
		offset = len(source)
	}
	for _, value := range source[:offset] {
		if value == '\n' {
			point.Row++
			point.Column = 0
		} else {
			point.Column++
		}
	}
	return point
}

func cPoint(point gotreesitter.Point) sitter.Point {
	return sitter.Point{Row: uint(point.Row), Column: uint(point.Column)}
}

func parseFresh(entry grammars.LangEntry, lang *gotreesitter.Language, parser *gotreesitter.Parser, source []byte) (*gotreesitter.Tree, error) {
	if entry.TokenSourceFactory == nil {
		return parser.Parse(source)
	}
	return parser.ParseWithTokenSourceFactory(source, func(input []byte) (gotreesitter.TokenSource, error) {
		tokenSource := entry.TokenSourceFactory(input, lang)
		if tokenSource == nil {
			return nil, fmt.Errorf("grammar %s token source factory returned nil", entry.Name)
		}
		return tokenSource, nil
	})
}

func parseIncremental(entry grammars.LangEntry, lang *gotreesitter.Language, parser *gotreesitter.Parser, source []byte, oldTree *gotreesitter.Tree) (*gotreesitter.Tree, error) {
	if entry.TokenSourceFactory == nil {
		return parser.ParseIncremental(source, oldTree)
	}
	return parser.ParseIncrementalWithTokenSourceFactory(source, oldTree, func(input []byte) (gotreesitter.TokenSource, error) {
		tokenSource := entry.TokenSourceFactory(input, lang)
		if tokenSource == nil {
			return nil, fmt.Errorf("grammar %s token source factory returned nil", entry.Name)
		}
		return tokenSource, nil
	})
}

func rootCovers(root *gotreesitter.Node, length int) bool {
	return root != nil && root.StartByte() == 0 && int(root.EndByte()) >= length
}

// rootCoversLikeC reports whether root covers the whole input the way the
// locked C runtime's root does. C's root span excludes leading padding, so
// its root can start after byte 0 (on "\na" both roots span bytes 1..2). A
// Go root that does not start at byte 0 passes only when C's root also
// starts after byte 0, the two spans are identical, and the root reaches the
// end of the input. Without a C root the strict rule applies.
func rootCoversLikeC(root *gotreesitter.Node, length int, cRoot *sitter.Node) bool {
	if rootCovers(root, length) {
		return true
	}
	if root == nil || cRoot == nil || cRoot.StartByte() == 0 {
		return false
	}
	return uint(root.StartByte()) == cRoot.StartByte() &&
		uint(root.EndByte()) == cRoot.EndByte() &&
		int(root.EndByte()) >= length
}

func rootCoverageExplained(root *gotreesitter.Node, tree *gotreesitter.Tree, length int, cRoot *sitter.Node) bool {
	return tree != nil && (rootCoversLikeC(root, length, cRoot) || tree.ParseStopReason() != gotreesitter.ParseStopAccepted)
}

func goDigest(tree *gotreesitter.Tree, lang *gotreesitter.Language) (string, error) {
	if tree == nil || tree.RootNode() == nil {
		return "", errors.New("nil Go tree")
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), lang)
	if err != nil {
		return "", err
	}
	return inspection.SHA256, nil
}

func goTreeDigest(tree *gotreesitter.Tree, lang *gotreesitter.Language) string {
	digest, _ := goDigest(tree, lang)
	return digest
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func joinErrors(a, b error) string {
	var details []string
	if a != nil {
		details = append(details, a.Error())
	}
	if b != nil {
		details = append(details, b.Error())
	}
	return strings.Join(details, "; ")
}

func sha256String(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return sha256String(data), nil
}
