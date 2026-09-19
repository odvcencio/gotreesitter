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
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/odvcencio/gotreesitter/grammars"
)

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
	Name             string             `json:"name"`
	RepoURL          string             `json:"repo_url"`
	OldRef           string             `json:"old_ref,omitempty"`
	NewRef           string             `json:"new_ref,omitempty"`
	Status           updateStatus       `json:"status"`
	HasScannerSpec   bool               `json:"has_scanner_spec"`
	Blocked          bool               `json:"blocked"`
	Reasons          []string           `json:"reasons,omitempty"`
	SourceFiles      []sourceFileResult `json:"source_files,omitempty"`
	ExpectedExternal []string           `json:"expected_externals,omitempty"`
	ActualExternal   []string           `json:"actual_externals,omitempty"`
}

type sourceFileResult struct {
	Path     string `json:"path"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Changed  bool   `json:"changed"`
	Missing  bool   `json:"missing,omitempty"`
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

func checkUpdate(workDir string, update updateResult) guardResult {
	result := guardResult{
		Name:    update.Name,
		RepoURL: update.RepoURL,
		OldRef:  update.OldRef,
		NewRef:  update.NewRef,
		Status:  update.Status,
	}

	spec, ok := grammars.LookupExternalScannerSpec(update.Name)
	if !ok {
		return result
	}
	result.HasScannerSpec = true
	result.ExpectedExternal = append([]string(nil), spec.Externals...)

	repoDir, err := fetchUpdateRef(workDir, update)
	if err != nil {
		result.Blocked = true
		result.Reasons = append(result.Reasons, err.Error())
		return result
	}

	for _, source := range spec.SourceFiles {
		fileResult := hashSourceFile(repoDir, source)
		result.SourceFiles = append(result.SourceFiles, fileResult)
		if fileResult.Missing {
			result.Blocked = true
			result.Reasons = append(result.Reasons, fmt.Sprintf("%s missing", source.Path))
			continue
		}
		if fileResult.Changed && !isGrammarJSON(source.Path) {
			result.Blocked = true
			result.Reasons = append(result.Reasons, fmt.Sprintf("%s changed", source.Path))
		}
	}

	grammarPath := filepath.Join(repoDir, "src", "grammar.json")
	actual, err := readExternalNames(grammarPath)
	if err != nil {
		result.Blocked = true
		result.Reasons = append(result.Reasons, fmt.Sprintf("read externals: %v", err))
		return result
	}
	result.ActualExternal = actual
	if !slices.Equal(spec.Externals, actual) {
		result.Blocked = true
		result.Reasons = append(result.Reasons, "external token list changed")
	}

	return result
}

func fetchUpdateRef(workDir string, update updateResult) (string, error) {
	repoDir := filepath.Join(workDir, safeDirName(update.Name))
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		return "", fmt.Errorf("create repo dir: %w", err)
	}
	if err := runGit(repoDir, "init", "--quiet"); err != nil {
		return "", err
	}
	if err := runGit(repoDir, "remote", "add", "origin", update.RepoURL); err != nil {
		return "", err
	}
	if err := runGit(repoDir, "fetch", "--quiet", "--depth=1", "origin", update.NewRef); err != nil {
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

func hashSourceFile(repoDir string, source grammars.ExternalScannerSourceFile) sourceFileResult {
	result := sourceFileResult{Path: source.Path, Expected: source.SHA256}
	data, err := os.ReadFile(filepath.Join(repoDir, filepath.FromSlash(source.Path)))
	if errors.Is(err, os.ErrNotExist) {
		result.Missing = true
		return result
	}
	if err != nil {
		result.Missing = true
		return result
	}
	sum := sha256.Sum256(data)
	result.Actual = hex.EncodeToString(sum[:])
	result.Changed = result.Actual != result.Expected
	return result
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

func isGrammarJSON(path string) bool {
	return filepath.Base(filepath.FromSlash(path)) == "grammar.json"
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
