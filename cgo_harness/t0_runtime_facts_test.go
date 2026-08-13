//go:build cgo && treesitter_c_parity && treesitter_c_perfscan && gts_workcount

package cgoharness

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	t0CardEnvGate         = "GTS_T0_CARD"
	t0CardEnvLanguage     = "GTS_T0_CARD_LANG"
	t0CardEnvPath         = "GTS_T0_CARD_PATH"
	t0CardEnvSourceStatus = "GTS_T0_CARD_SOURCE_STATUS"
	t0CardEnvOutput       = "GTS_T0_CARD_OUT"
	t0CardEnvTimeoutMS    = "GTS_T0_CARD_TIMEOUT_MS"
	t0CardReceiptSchema   = "gts-t0-card/v1"
)

type t0SourceStatusRow struct {
	Language string `json:"language"`
	RepoURL  string `json:"repo_url"`
	Commit   string `json:"commit"`
	Head     string `json:"head"`
	Clean    bool   `json:"clean"`
}

type t0SourceStatusEnvelope struct {
	Languages []t0SourceStatusRow `json:"languages"`
}

type t0CardGoAttempt struct {
	LogicalRung    string `json:"logical_rung"`
	OperationCause string `json:"operation_cause"`
	RootEndByte    uint32 `json:"root_end_byte"`
}

type t0CardGoRuntime struct {
	StoppedEarly         bool   `json:"stopped_early"`
	ArenaBytesAllocated  int64  `json:"arena_bytes_allocated"`
	GSSBytesAllocated    int64  `json:"gss_bytes_allocated"`
	MaterializationNanos int64  `json:"materialization_nanos"`
	ParserLoopNanos      int64  `json:"parser_loop_nanos"`
	ResultSelectionNanos int64  `json:"result_selection_nanos"`
	ResultTreeBuildNanos int64  `json:"result_tree_build_nanos"`
	FinalNodes           uint64 `json:"final_nodes"`
	FinalParentNodes     uint64 `json:"final_parent_nodes"`
	FinalLeafNodes       uint64 `json:"final_leaf_nodes"`
	FinalChildSlices     uint64 `json:"final_child_slices"`
	FinalChildPointers   uint64 `json:"final_child_pointers"`
}

type t0CardGoParse struct {
	WallNanos                    int64             `json:"wall_nanos"`
	TotalAllocBytes              uint64            `json:"total_alloc_bytes"`
	Attempts                     []t0CardGoAttempt `json:"attempts"`
	SelectedRetryRung            string            `json:"selected_retry_rung"`
	TreePresent                  bool              `json:"tree_present"`
	RootStartByte                uint32            `json:"root_start_byte"`
	RootEndByte                  uint32            `json:"root_end_byte"`
	RootHasError                 bool              `json:"root_has_error"`
	DeepTreeSHA256               string            `json:"deep_tree_sha256"`
	Runtime                      t0CardGoRuntime   `json:"runtime"`
	TreeObservationRetainedNanos int64             `json:"tree_observation_retained_nanos"`
	PeakRSSBytes                 uint64            `json:"peak_rss_bytes"`
	RSSSource                    string            `json:"rss_source"`
}

type t0CardGoResponse struct {
	Schema            string        `json:"schema"`
	Language          string        `json:"language"`
	SourceBytes       int           `json:"source_bytes"`
	SourceSHA256      string        `json:"source_sha256"`
	CandidateRevision string        `json:"candidate_revision"`
	BuildModified     bool          `json:"build_modified"`
	Parse             t0CardGoParse `json:"parse"`
}

type t0GoChildIdentity struct {
	Schema            string   `json:"schema"`
	BinarySHA256      string   `json:"binary_sha256"`
	CandidateRevision string   `json:"candidate_revision"`
	BuildTags         []string `json:"build_tags"`
	CGOEnabled        bool     `json:"cgo_enabled"`
}

type t0CardProcessContract struct {
	GOMAXPROCS      string `json:"gomaxprocs"`
	ChildTimeout    string `json:"child_timeout"`
	RSSSource       string `json:"rss_source"`
	Materialization string `json:"materialization_formula"`
}

type t0CardReceipt struct {
	Schema             string                  `json:"schema"`
	Status             string                  `json:"status"`
	GeneratedAt        string                  `json:"generated_at"`
	GitRevision        string                  `json:"git_revision"`
	Language           string                  `json:"language"`
	File               string                  `json:"file"`
	SourceBytes        int                     `json:"source_bytes"`
	SourceSHA256       string                  `json:"source_sha256"`
	CorpusRoot         string                  `json:"corpus_root"`
	CorpusLock         string                  `json:"corpus_lock"`
	CorpusLockSHA256   string                  `json:"corpus_lock_sha256"`
	SourceStatus       string                  `json:"source_status"`
	SourceStatusSHA256 string                  `json:"source_status_sha256"`
	SourceRepository   t0SourceStatusRow       `json:"source_repository"`
	GoChild            t0GoChildIdentity       `json:"go_child"`
	Go                 json.RawMessage         `json:"go"`
	COracle            perfScanOracleIdentity  `json:"c_oracle"`
	CAdmission         perfScanOracleAdmission `json:"c_admission"`
	Process            t0CardProcessContract   `json:"process"`
}

type t0CardChildBuild struct {
	Path     string
	Identity t0GoChildIdentity
}

// TestT0RuntimeFactsCard writes one evidence-only card for one exact source.
// It does not change parser routing or grant performance credit.
func TestT0RuntimeFactsCard(t *testing.T) {
	if !t0CardGateEnabled() {
		t.Skipf("set %s=1 to run the T0 card collector", t0CardEnvGate)
	}
	language := strings.TrimSpace(os.Getenv(t0CardEnvLanguage))
	cardPath := strings.TrimSpace(os.Getenv(t0CardEnvPath))
	statusPath := strings.TrimSpace(os.Getenv(t0CardEnvSourceStatus))
	outPath := strings.TrimSpace(os.Getenv(t0CardEnvOutput))
	if language == "" || cardPath == "" || statusPath == "" || outPath == "" {
		t.Fatalf("set %s, %s, %s, and %s", t0CardEnvLanguage, t0CardEnvPath, t0CardEnvSourceStatus, t0CardEnvOutput)
	}
	t.Setenv("GTS_REAL_CORPUS_BENCH_LOCK_FILTER", "1")
	corpusRoot := realCorpusBenchmarkRootForTest(t)
	lockPath := strings.TrimSpace(os.Getenv("GTS_REAL_CORPUS_BENCH_LOCK"))
	if lockPath == "" {
		t.Fatal("set GTS_REAL_CORPUS_BENCH_LOCK")
	}
	lockSHA, err := fileSHA256(lockPath)
	if err != nil {
		t.Fatalf("hash corpus lock: %v", err)
	}
	statusSHA, err := fileSHA256(statusPath)
	if err != nil {
		t.Fatalf("hash source status: %v", err)
	}
	status := t0LoadSourceStatus(t, statusPath, language)
	if status.RepoURL == "" || status.Commit == "" || status.Head == "" || status.Commit != status.Head || !status.Clean {
		t.Fatalf("source status for %s is not a clean locked checkout: %+v", language, status)
	}
	if err := t0VerifySourceCheckout(t, corpusRoot, language, status); err != nil {
		t.Fatal(err)
	}
	sourcePath, source := t0LoadCardSource(t, corpusRoot, language, cardPath)
	sourceSum := sha256.Sum256(source)
	sourceSHA := hex.EncodeToString(sourceSum[:])

	revision, modified := t0GitRevision(t)
	if modified {
		t.Fatal("the repository must be clean before building the T0 child")
	}
	child := t0BuildGoChild(t, revision)
	timeout := t0CardTimeout(t)
	goResult, goJSON := t0RunGoChild(t, child, language, sourcePath, timeout)
	t0ValidateGoResult(t, goResult, language, len(source), sourceSHA, revision)

	oracle, err := buildStaticCPerfOracle(language)
	if err != nil {
		t.Fatalf("build locked C oracle: %v", err)
	}
	defer oracle.Close()
	measurer := &perfScanLangMeasurer{
		lang:    language,
		staticC: oracle,
		budget:  timeout,
		cfg:     perfScanConfig{CgoAdmissionRSSMB: perfScanDefaultCgoAdmissionRSSMB},
	}
	cAdmission, err := measurer.admitStaticOracle(source)
	if err != nil {
		t.Fatalf("admit locked C oracle: %v", err)
	}
	if !cAdmission.Admitted || goResult.Parse.DeepTreeSHA256 != cAdmission.ParityDeepSHA256 {
		t.Fatalf("Go/C deep digest mismatch: go=%s c=%s admitted=%t", goResult.Parse.DeepTreeSHA256, cAdmission.ParityDeepSHA256, cAdmission.Admitted)
	}
	if cAdmission.CgoPeakRSSBytes <= 0 {
		t.Fatalf("locked C admission did not record peak RSS: %d", cAdmission.CgoPeakRSSBytes)
	}

	receipt := t0CardReceipt{
		Schema:             t0CardReceiptSchema,
		Status:             "evidence_only",
		GeneratedAt:        time.Now().UTC().Format(time.RFC3339Nano),
		GitRevision:        revision,
		Language:           language,
		File:               filepath.ToSlash(cardPath),
		SourceBytes:        len(source),
		SourceSHA256:       sourceSHA,
		CorpusRoot:         corpusRoot,
		CorpusLock:         lockPath,
		CorpusLockSHA256:   lockSHA,
		SourceStatus:       statusPath,
		SourceStatusSHA256: statusSHA,
		SourceRepository:   status,
		GoChild:            child.Identity,
		Go:                 goJSON,
		COracle:            oracle.identity,
		CAdmission:         *cAdmission,
		Process: t0CardProcessContract{
			GOMAXPROCS:      os.Getenv("GOMAXPROCS"),
			ChildTimeout:    timeout.String(),
			RSSSource:       "linux /proc/self/status VmHWM for Go; managed child polling for C",
			Materialization: "sum of ParseRuntime result-selection, transient-parent, result-tree, transient-child, final-root, trailing, root-normalization, compatibility, and parent-link timings; components may overlap",
		},
	}
	if err := t0WriteReceipt(outPath, receipt); err != nil {
		t.Fatalf("write T0 receipt: %v", err)
	}
	t.Logf("T0 card %s/%s: Go/C digest=%s Go RSS=%d C RSS=%d", language, cardPath, goResult.Parse.DeepTreeSHA256, goResult.Parse.PeakRSSBytes, cAdmission.CgoPeakRSSBytes)
}

func t0CardGateEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(t0CardEnvGate))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func t0LoadSourceStatus(t testing.TB, path, language string) t0SourceStatusRow {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read source status: %v", err)
	}
	var rows []t0SourceStatusRow
	if err := json.Unmarshal(data, &rows); err != nil {
		var envelope t0SourceStatusEnvelope
		if envelopeErr := json.Unmarshal(data, &envelope); envelopeErr != nil {
			t.Fatalf("decode source status: %v", err)
		}
		rows = envelope.Languages
	}
	for _, row := range rows {
		if row.Language == language {
			return row
		}
	}
	t.Fatalf("source status has no language %q", language)
	return t0SourceStatusRow{}
}

func t0VerifySourceCheckout(t testing.TB, corpusRoot, language string, expected t0SourceStatusRow) error {
	t.Helper()
	repoDir := filepath.Join(corpusRoot, language)
	head, err := t0GitOutput(repoDir, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("read %s source head: %w", language, err)
	}
	if head != expected.Head || head != expected.Commit {
		return fmt.Errorf("source head for %s=%s, want %s", language, head, expected.Commit)
	}
	clean, err := t0GitOutput(repoDir, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return fmt.Errorf("read %s source status: %w", language, err)
	}
	if strings.TrimSpace(clean) != "" {
		return fmt.Errorf("source checkout %s is dirty: %s", language, clean)
	}
	return nil
}

func t0LoadCardSource(t testing.TB, corpusRoot, language, cardPath string) (string, []byte) {
	t.Helper()
	langRoot := realCorpusBenchmarkLanguageRoot(t, corpusRoot, language)
	rel := filepath.Clean(filepath.FromSlash(cardPath))
	if rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Fatalf("T0 card path escapes language root: %q", cardPath)
	}
	filters := realCorpusBenchmarkFileFiltersFor(t, language, langRoot)
	if !realCorpusBenchmarkFileAllowed(filepath.ToSlash(rel), filters) {
		t.Fatalf("T0 card path is not selected by the authenticated lock: %s", cardPath)
	}
	path := filepath.Join(langRoot, rel)
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("stat T0 card source %s: %v", path, err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("T0 card source is not a regular file: %s", path)
	}
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read T0 card source %s: %v", path, err)
	}
	return path, source
}

func t0GitRevision(t testing.TB) (string, bool) {
	t.Helper()
	revision, err := t0GitOutput(".", "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("read Go revision: %v", err)
	}
	status, err := t0GitOutput(".", "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		t.Fatalf("read Go status: %v", err)
	}
	return revision, strings.TrimSpace(status) != ""
}

func t0GitOutput(dir string, args ...string) (string, error) {
	commandArgs := append([]string{"-C", dir}, args...)
	command := exec.Command("git", append([]string{"-c", "safe.directory=*"}, commandArgs...)...)
	data, err := command.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %s", strings.Join(commandArgs, " "), strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func t0BuildGoChild(t testing.TB, revision string) t0CardChildBuild {
	t.Helper()
	repoRoot, err := t0GitOutput(".", "rev-parse", "--show-toplevel")
	if err != nil {
		t.Fatalf("resolve Go repository: %v", err)
	}
	sourceDir := filepath.Join(t.TempDir(), "t0-card-source")
	clone := exec.Command("git", "-c", "safe.directory=*", "clone", "--no-local", repoRoot, sourceDir)
	clone.Env = t0EnvWithOverrides(os.Environ(), map[string]string{
		"GIT_CONFIG_COUNT":   "1",
		"GIT_CONFIG_KEY_0":   "safe.directory",
		"GIT_CONFIG_VALUE_0": "*",
	})
	if output, err := clone.CombinedOutput(); err != nil {
		t.Fatalf("create clean T0 child clone: %v: %s", err, strings.TrimSpace(string(output)))
	}
	if _, err := t0GitOutput(sourceDir, "checkout", "--detach", revision); err != nil {
		t.Fatalf("select T0 child revision: %v", err)
	}
	path := filepath.Join(t.TempDir(), "t0-card-child")
	command := exec.Command("go", "build", "-trimpath", "-buildvcs=true", "-tags", "gts_workcount", "-o", path, "./cmd/t0_card_child")
	command.Dir = sourceDir
	command.Env = t0EnvWithOverrides(os.Environ(), map[string]string{
		"CGO_ENABLED":        "0",
		"GOWORK":             "off",
		"GIT_CONFIG_COUNT":   "1",
		"GIT_CONFIG_KEY_0":   "safe.directory",
		"GIT_CONFIG_VALUE_0": "*",
	})
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build T0 Go child: %v: %s", err, strings.TrimSpace(string(output)))
	}
	buildSettings, err := t0ReadBuildSettings(path)
	if err != nil {
		t.Fatalf("read T0 child build info: %v", err)
	}
	if buildSettings.revision != revision || buildSettings.modified {
		t.Fatalf("T0 child revision=%q modified=%t, want %q modified=false", buildSettings.revision, buildSettings.modified, revision)
	}
	binary, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read T0 child binary: %v", err)
	}
	sum := sha256.Sum256(binary)
	return t0CardChildBuild{
		Path: path,
		Identity: t0GoChildIdentity{
			Schema:            "gts-t0-card-go-child/v1",
			BinarySHA256:      hex.EncodeToString(sum[:]),
			CandidateRevision: revision,
			BuildTags:         []string{"gts_workcount"},
			CGOEnabled:        false,
		},
	}
}

type t0BuildSettings struct {
	revision string
	modified bool
}

func t0ReadBuildSettings(path string) (t0BuildSettings, error) {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return t0BuildSettings{}, err
	}
	settings := t0BuildSettings{}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			settings.revision = setting.Value
		case "vcs.modified":
			settings.modified = setting.Value == "true"
		}
	}
	return settings, nil
}

func t0RunGoChild(t testing.TB, child t0CardChildBuild, language, source string, timeout time.Duration) (t0CardGoResponse, json.RawMessage) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	command := exec.CommandContext(ctx, child.Path, "-language", language, "-source", source)
	command.Env = t0EnvWithOverrides(os.Environ(), map[string]string{"GOMAXPROCS": "1"})
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run T0 Go child: %v: %s", err, strings.TrimSpace(string(output)))
	}
	output = bytes.TrimSpace(output)
	var result t0CardGoResponse
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode T0 Go child: %v: %s", err, strings.TrimSpace(string(output)))
	}
	return result, json.RawMessage(append([]byte(nil), output...))
}

func t0ValidateGoResult(t testing.TB, result t0CardGoResponse, language string, sourceBytes int, sourceSHA, revision string) {
	t.Helper()
	if result.Schema != "gts-t0-card-go-child/v1" || result.Language != language || result.SourceBytes != sourceBytes || result.SourceSHA256 != sourceSHA {
		t.Fatalf("incomplete Go child identity: schema=%q language=%q bytes=%d sha=%q", result.Schema, result.Language, result.SourceBytes, result.SourceSHA256)
	}
	if result.BuildModified || result.CandidateRevision != revision {
		t.Fatalf("Go child build identity revision=%q modified=%t, want %q clean", result.CandidateRevision, result.BuildModified, revision)
	}
	parse := result.Parse
	if len(parse.Attempts) == 0 || parse.SelectedRetryRung == "" || !parse.TreePresent || parse.DeepTreeSHA256 == "" {
		t.Fatalf("Go child omitted required parse facts: %+v", parse)
	}
	if parse.RootStartByte != 0 || parse.RootEndByte != uint32(sourceBytes) || parse.RootHasError || parse.Runtime.StoppedEarly {
		t.Fatalf("Go child did not produce a clean full-span tree: root=[%d,%d) bytes=%d error=%t stopped=%t", parse.RootStartByte, parse.RootEndByte, sourceBytes, parse.RootHasError, parse.Runtime.StoppedEarly)
	}
	if parse.PeakRSSBytes == 0 || parse.RSSSource == "" || parse.Runtime.ArenaBytesAllocated <= 0 || parse.Runtime.GSSBytesAllocated < 0 || parse.Runtime.MaterializationNanos < 0 || parse.TreeObservationRetainedNanos < 0 {
		t.Fatalf("Go child omitted runtime facts: rss=%d source=%q arena=%d gss=%d materialization=%d retained=%d", parse.PeakRSSBytes, parse.RSSSource, parse.Runtime.ArenaBytesAllocated, parse.Runtime.GSSBytesAllocated, parse.Runtime.MaterializationNanos, parse.TreeObservationRetainedNanos)
	}
}

func t0WriteReceipt(path string, receipt t0CardReceipt) error {
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func t0CardTimeout(t testing.TB) time.Duration {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv(t0CardEnvTimeoutMS))
	if raw == "" {
		return 10 * time.Second
	}
	millis, err := strconv.Atoi(raw)
	if err != nil || millis <= 0 {
		t.Fatalf("invalid %s=%q", t0CardEnvTimeoutMS, raw)
	}
	return time.Duration(millis) * time.Millisecond
}

func t0EnvWithOverrides(base []string, overrides map[string]string) []string {
	env := make([]string, 0, len(base)+len(overrides))
	for _, value := range base {
		key, _, ok := strings.Cut(value, "=")
		if ok {
			if _, replaced := overrides[key]; replaced {
				continue
			}
		}
		env = append(env, value)
	}
	for key, value := range overrides {
		env = append(env, key+"="+value)
	}
	return env
}
