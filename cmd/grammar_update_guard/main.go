// Command grammar_update_guard checks lock-update reports for scanner-facing
// changes that require hand-written scanner review before grammar blobs move.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/odvcencio/gotreesitter/grammars"
)

// scannerSourceFileNames lists the upstream external-scanner source file
// names the guard checks under a grammar's subdir. Tree-sitter grammars
// write their hand-maintained scanner in one of these two files; a grammar
// with neither present on either ref has no external scanner to review.
var scannerSourceFileNames = []string{"scanner.c", "scanner.cc"}

type updateStatus string

const (
	updateStatusApplied   updateStatus = "applied"
	updateStatusAvailable updateStatus = "available"
)

type updateReport struct {
	GeneratedAt string         `json:"generated_at"`
	Results     []updateResult `json:"results"`
}

type updateResult struct {
	Name    string       `json:"name"`
	RepoURL string       `json:"repo_url"`
	OldRef  string       `json:"old_ref,omitempty"`
	NewRef  string       `json:"new_ref,omitempty"`
	Subdir  string       `json:"subdir,omitempty"`
	Status  updateStatus `json:"status"`
	Applied bool         `json:"applied"`
}

type guardReport struct {
	GeneratedAt  string        `json:"generated_at"`
	UpdatesPath  string        `json:"updates_path"`
	CheckedCount int           `json:"checked_count"`
	BlockedCount int           `json:"blocked_count"`
	Results      []guardResult `json:"results"`
}

type guardResult struct {
	Name    string       `json:"name"`
	RepoURL string       `json:"repo_url"`
	OldRef  string       `json:"old_ref,omitempty"`
	NewRef  string       `json:"new_ref,omitempty"`
	Status  updateStatus `json:"status"`
	// HasScannerSpec reports whether a hand-reviewed ExternalScannerSpec is
	// registered for this language. It is informational only: the blocking
	// decision below always diffs the old and new ref directly, so a missing
	// registration can no longer waive scanner review by itself.
	HasScannerSpec bool               `json:"has_scanner_spec"`
	Blocked        bool               `json:"blocked"`
	Reasons        []string           `json:"reasons,omitempty"`
	SourceFiles    []sourceFileResult `json:"source_files,omitempty"`
	// ExpectedExternal and ActualExternal are the externals array read from
	// grammar.json at the old ref and the new ref, respectively (order
	// preserved). They are populated only when both refs have a readable
	// grammar.json under Subdir.
	ExpectedExternal []string `json:"expected_externals,omitempty"`
	ActualExternal   []string `json:"actual_externals,omitempty"`
}

// sourceFileResult records the old-ref-vs-new-ref comparison for one
// scanner-facing source file. MissingOld/MissingNew let the guard tell "the
// file never existed" apart from "the file was added" or "the file was
// removed" between the two refs: a change on either side is scanner-facing.
type sourceFileResult struct {
	Path       string `json:"path"`
	OldSHA256  string `json:"old_sha256,omitempty"`
	NewSHA256  string `json:"new_sha256,omitempty"`
	Changed    bool   `json:"changed"`
	MissingOld bool   `json:"missing_old,omitempty"`
	MissingNew bool   `json:"missing_new,omitempty"`
}

func main() {
	var (
		updatesPath     = flag.String("updates", "grammars/grammar_updates.json", "grammar_updater JSON report path")
		reportPath      = flag.String("report", "", "optional output path for scanner guard JSON report")
		blockedListPath = flag.String("blocked-list", "", "optional output path for a newline-delimited list of blocked grammar names, for use as grammar_updater's -exclude-list")
		failOnBlocked   = flag.Bool("fail-on-blocked", true, "exit non-zero when scanner-facing changes are detected")
		keepWork        = flag.Bool("keep-work", false, "keep temporary fetched repos for debugging")
	)
	flag.Parse()

	report, err := run(*updatesPath, *keepWork)
	if err != nil {
		exitf("%v", err)
	}
	if *reportPath != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			exitf("marshal report: %v", err)
		}
		if err := os.WriteFile(*reportPath, append(data, '\n'), 0o644); err != nil {
			exitf("write report: %v", err)
		}
	}
	if *blockedListPath != "" {
		if err := writeBlockedList(*blockedListPath, report); err != nil {
			exitf("write blocked list: %v", err)
		}
	}

	fmt.Printf("grammar_update_guard: checked=%d blocked=%d\n", report.CheckedCount, report.BlockedCount)
	for _, result := range report.Results {
		if result.Blocked {
			fmt.Printf("blocked %s: %s\n", result.Name, strings.Join(result.Reasons, "; "))
		}
	}
	if *failOnBlocked && report.BlockedCount > 0 {
		os.Exit(1)
	}
}

// writeBlockedList emits a newline-delimited list of blocked grammar names.
// The workflow feeds this file straight to grammar_updater's -exclude-list so
// the apply step holds back exactly the grammars the guard flagged, while
// every other cleared update still lands.
func writeBlockedList(path string, report *guardReport) error {
	var b strings.Builder
	for _, result := range report.Results {
		if !result.Blocked {
			continue
		}
		b.WriteString(result.Name)
		b.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func run(updatesPath string, keepWork bool) (*guardReport, error) {
	updates, err := readUpdateReport(updatesPath)
	if err != nil {
		return nil, err
	}

	workDir, err := os.MkdirTemp("", "grammar-update-guard-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	if !keepWork {
		defer os.RemoveAll(workDir)
	} else {
		fmt.Fprintf(os.Stderr, "keeping work dir: %s\n", workDir)
	}

	report := &guardReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		UpdatesPath: updatesPath,
		Results:     make([]guardResult, 0),
	}

	for _, update := range updates.Results {
		if !shouldCheck(update) {
			continue
		}
		report.CheckedCount++
		result := checkUpdate(workDir, update)
		if result.Blocked {
			report.BlockedCount++
		}
		report.Results = append(report.Results, result)
	}
	return report, nil
}

func readUpdateReport(path string) (*updateReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read update report: %w", err)
	}
	var report updateReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("parse update report: %w", err)
	}
	return &report, nil
}

func shouldCheck(update updateResult) bool {
	if strings.TrimSpace(update.NewRef) == "" || strings.TrimSpace(update.RepoURL) == "" {
		return false
	}
	return update.Applied || update.Status == updateStatusApplied || update.Status == updateStatusAvailable
}

// checkUpdate decides whether a grammar's upstream update is scanner-safe. It
// fetches both the old (currently locked) ref and the new ref and diffs them
// directly: it never trusts a per-language registration to say a grammar has
// no external scanner, because that registration is opt-in and easy to miss
// (see the 2026-09-20 c_sharp/cmake/yaml incident, where the guard silently
// cleared all three because no ExternalScannerSpec had ever been registered
// for them).
func checkUpdate(workDir string, update updateResult) guardResult {
	result := guardResult{
		Name:    update.Name,
		RepoURL: update.RepoURL,
		OldRef:  update.OldRef,
		NewRef:  update.NewRef,
		Status:  update.Status,
	}
	if _, ok := grammars.LookupExternalScannerSpec(update.Name); ok {
		result.HasScannerSpec = true
	}

	if strings.TrimSpace(update.OldRef) == "" {
		result.Blocked = true
		result.Reasons = append(result.Reasons, "missing old ref: cannot diff scanner-facing files against the currently locked commit")
		return result
	}

	oldDir, err := fetchRef(workDir, update.Name, "old", update.RepoURL, update.OldRef)
	if err != nil {
		result.Blocked = true
		result.Reasons = append(result.Reasons, fmt.Sprintf("fetch old ref: %v", err))
		return result
	}
	newDir, err := fetchRef(workDir, update.Name, "new", update.RepoURL, update.NewRef)
	if err != nil {
		result.Blocked = true
		result.Reasons = append(result.Reasons, fmt.Sprintf("fetch new ref: %v", err))
		return result
	}

	applyGrammarDiff(&result, oldDir, newDir, update.Subdir)
	return result
}

// applyGrammarDiff compares an already-checked-out old ref and new ref and
// records the scanner-facing differences on result. It performs no network
// or git access, so tests exercise it directly against fixture directories
// under testdata/ instead of cloning real upstream repos.
func applyGrammarDiff(result *guardResult, oldDir, newDir, subdir string) {
	subdir = strings.TrimSpace(subdir)
	if subdir == "" {
		subdir = "src"
	}

	for _, name := range scannerSourceFileNames {
		rel := path.Join(subdir, name)
		fileResult, changed := diffSourceFile(oldDir, newDir, rel)
		if fileResult == nil {
			continue // absent on both refs: this grammar has no such scanner file.
		}
		result.SourceFiles = append(result.SourceFiles, *fileResult)
		if changed {
			result.Blocked = true
			result.Reasons = append(result.Reasons, scannerFileChangeReason(*fileResult))
		}
	}

	grammarRel := path.Join(subdir, "grammar.json")
	oldExternals, oldErr := readExternalNames(filepath.Join(oldDir, filepath.FromSlash(grammarRel)))
	newExternals, newErr := readExternalNames(filepath.Join(newDir, filepath.FromSlash(grammarRel)))
	switch {
	case oldErr != nil && newErr != nil:
		// Neither ref carries a readable grammar.json under this subdir;
		// there is no external token list to compare.
	case oldErr != nil || newErr != nil:
		result.Blocked = true
		result.Reasons = append(result.Reasons, fmt.Sprintf("read %s: old=%v new=%v", grammarRel, oldErr, newErr))
	default:
		result.ExpectedExternal = oldExternals
		result.ActualExternal = newExternals
		if !slices.Equal(oldExternals, newExternals) {
			result.Blocked = true
			result.Reasons = append(result.Reasons, "external token list changed")
		}
	}
}

// diffSourceFile hashes rel under oldDir and newDir. It returns nil when the
// file is absent on both sides: that grammar simply has no such file, which
// is not itself a scanner-facing change. changed is true when the file
// content hash differs or the file's presence differs between the two refs.
func diffSourceFile(oldDir, newDir, rel string) (result *sourceFileResult, changed bool) {
	oldSum, oldExists, oldErr := hashFileIfExists(oldDir, rel)
	newSum, newExists, newErr := hashFileIfExists(newDir, rel)
	if !oldExists && !newExists && oldErr == nil && newErr == nil {
		return nil, false
	}

	fr := sourceFileResult{
		Path:       rel,
		MissingOld: !oldExists,
		MissingNew: !newExists,
	}
	if oldExists {
		fr.OldSHA256 = oldSum
	}
	if newExists {
		fr.NewSHA256 = newSum
	}
	fr.Changed = oldErr != nil || newErr != nil || oldExists != newExists || (oldExists && newExists && oldSum != newSum)
	return &fr, fr.Changed
}

// hashFileIfExists returns the hex SHA-256 digest of dir/rel. exists is false
// when the file does not exist; err reports any other read failure, which
// the caller treats as a change since the file's true content is unknown.
func hashFileIfExists(dir, rel string) (sum string, exists bool, err error) {
	data, readErr := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if errors.Is(readErr, os.ErrNotExist) {
		return "", false, nil
	}
	if readErr != nil {
		return "", false, readErr
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), true, nil
}

func scannerFileChangeReason(fr sourceFileResult) string {
	switch {
	case fr.MissingOld && !fr.MissingNew:
		return fmt.Sprintf("%s added", fr.Path)
	case !fr.MissingOld && fr.MissingNew:
		return fmt.Sprintf("%s removed", fr.Path)
	default:
		return fmt.Sprintf("%s changed", fr.Path)
	}
}

// fetchRef clones repoURL at ref into a label-suffixed directory under
// workDir (for example "kotlin_old", "kotlin_new") so the old and new
// checkouts coexist for a direct file-by-file diff.
func fetchRef(workDir, name, label, repoURL, ref string) (string, error) {
	repoDir := filepath.Join(workDir, safeDirName(name)+"_"+label)
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		return "", fmt.Errorf("create repo dir: %w", err)
	}
	if err := runGit(repoDir, "init", "--quiet"); err != nil {
		return "", err
	}
	if err := runGit(repoDir, "remote", "add", "origin", repoURL); err != nil {
		return "", err
	}
	if err := runGit(repoDir, "fetch", "--quiet", "--depth=1", "origin", ref); err != nil {
		return "", err
	}
	if err := runGit(repoDir, "checkout", "--quiet", "FETCH_HEAD"); err != nil {
		return "", err
	}
	return repoDir, nil
}

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func readExternalNames(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Externals []json.RawMessage `json:"externals"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(payload.Externals))
	for _, raw := range payload.Externals {
		name, err := externalName(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, nil
}

func externalName(raw json.RawMessage) (string, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil
	}
	var obj struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", err
	}
	if obj.Name != "" {
		return obj.Name, nil
	}
	return obj.Value, nil
}

func safeDirName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "grammar"
	}
	return b.String()
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
