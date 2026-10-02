//go:build cgo && treesitter_c_parity && treesitter_c_perfscan && gts_workcount

package cgoharness

import (
	"bufio"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"testing"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

const (
	retryProfileCertSchema           = "gts-retry-profile-cert/v6"
	retryProfileCertChildSchema      = "gts-retry-profile-cert-child/v1"
	retryProfileCertEnvMode          = "GTS_RETRY_PROFILE_CERT_MODE"
	retryProfileCertEnvChildTimeout  = "GTS_RETRY_PROFILE_CERT_CHILD_TIMEOUT"
	retryProfileCertModeScanner      = "skip_external_scanner_repeat"
	retryProfileCertModeSkipComplete = "skip_complete_accepted_error"
	retryProfileCertModeSkipFresh    = "skip_fresh_complete_accepted_error"
	retryProfileCertModeShortLadder  = "short_complete_accepted_error_ladder"
	retryProfileCertModeReuseClean   = "reuse_clean_wide_for_wide_retry"
	retryProfileCertBaselineFirst    = "baseline_first"
	retryProfileCertCandidateFirst   = "candidate_first"
	retryProfileCertEffectActivated  = "activated"
	retryProfileCertEffectUnchanged  = "unchanged"
	retryProfileCertChildTimeout     = 2 * time.Minute
)

type retryProfileCertAttempt struct {
	LogicalRung        string                       `json:"logical_rung"`
	OperationCause     string                       `json:"operation_cause"`
	StopReason         gotreesitter.ParseStopReason `json:"stop_reason"`
	RootHasError       bool                         `json:"root_has_error"`
	RootEndByte        uint32                       `json:"root_end_byte"`
	ResolvedMaxStacks  int                          `json:"resolved_max_stacks"`
	ResolvedRetryPass  bool                         `json:"resolved_retry_pass"`
	ResolvedMergeLimit int                          `json:"resolved_max_merge_per_key"`
}

type retryProfileCertParse struct {
	WallNanos   int64                        `json:"wall_nanos"`
	TotalAlloc  uint64                       `json:"total_alloc_bytes"`
	Attempts    []retryProfileCertAttempt    `json:"attempts"`
	TreePresent bool                         `json:"tree_present"`
	StopReason  gotreesitter.ParseStopReason `json:"stop_reason"`
	RootStart   uint32                       `json:"root_start_byte"`
	RootEnd     uint32                       `json:"root_end_byte"`
	HasError    bool                         `json:"root_has_error"`
	DeepSHA256  string                       `json:"deep_tree_sha256"`
}

type retryProfileCertFile struct {
	Path                  string                `json:"path"`
	Bytes                 int                   `json:"bytes"`
	SourceSHA256          string                `json:"source_sha256"`
	Class                 string                `json:"class"`
	ParseOrder            string                `json:"parse_order"`
	PolicyMode            string                `json:"policy_mode"`
	PolicyEffect          string                `json:"policy_effect"`
	AttemptsEliminated    int                   `json:"attempts_eliminated"`
	OracleDeepSHA256      string                `json:"oracle_deep_tree_sha256"`
	BaselineOracleParity  bool                  `json:"baseline_oracle_parity"`
	CandidateOracleParity bool                  `json:"candidate_oracle_parity"`
	OracleStatus          string                `json:"oracle_status"`
	OracleDetail          string                `json:"oracle_detail,omitempty"`
	Baseline              retryProfileCertParse `json:"baseline"`
	Candidate             retryProfileCertParse `json:"candidate"`
}

type retryProfileCertTotals struct {
	Files               int    `json:"files"`
	Bytes               int64  `json:"bytes"`
	BaselineFirstFiles  int    `json:"baseline_first_files"`
	CandidateFirstFiles int    `json:"candidate_first_files"`
	ActivatedFiles      int    `json:"activated_files"`
	UnchangedFiles      int    `json:"unchanged_files"`
	BaselineWallNanos   int64  `json:"baseline_wall_nanos"`
	CandidateWallNanos  int64  `json:"candidate_wall_nanos"`
	BaselineTotalAlloc  uint64 `json:"baseline_total_alloc_bytes"`
	CandidateTotalAlloc uint64 `json:"candidate_total_alloc_bytes"`
	BaselineAttempts    int    `json:"baseline_attempts"`
	CandidateAttempts   int    `json:"candidate_attempts"`
	OracleMatches       int    `json:"oracle_matches"`
	OracleMismatches    int    `json:"oracle_mismatches"`
	OracleCrashes       int    `json:"oracle_crashes"`
	OracleUnavailable   int    `json:"oracle_unavailable"`
}

type retryProfileCertFailure struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type retryProfileCertCandidateProfile struct {
	Mode                            string `json:"mode"`
	SkipExternalScannerRepeat       bool   `json:"skip_external_scanner_repeat"`
	SkipCompleteAcceptedError       bool   `json:"skip_complete_accepted_error"`
	SkipFreshCompleteAcceptedError  bool   `json:"skip_fresh_complete_accepted_error"`
	SkipInitialAcceptedErrorMerge   bool   `json:"skip_initial_accepted_error_merge"`
	SkipCompleteMinSourceBytes      uint32 `json:"skip_complete_min_source_bytes,omitempty"`
	SkipCompleteMaxEntryScratchPeak uint32 `json:"skip_complete_max_entry_scratch_peak,omitempty"`
	SkipCompleteMaxStacksSeen       uint16 `json:"skip_complete_max_stacks_seen,omitempty"`
	ReuseCleanWideForWideRetry      bool   `json:"reuse_clean_wide_for_wide_retry"`
	ReuseCleanWideMinSourceBytes    uint32 `json:"reuse_clean_wide_min_source_bytes,omitempty"`
}

type retryProfileCertGoChildIdentity struct {
	Schema            string   `json:"schema"`
	BinarySHA256      string   `json:"binary_sha256"`
	CandidateRevision string   `json:"candidate_revision"`
	BuildTags         []string `json:"build_tags"`
	CGOEnabled        bool     `json:"cgo_enabled"`
	PairTimeout       string   `json:"pair_timeout"`
}

type retryProfileCertGoChild struct {
	path     string
	identity retryProfileCertGoChildIdentity
}

type retryProfileCertGoChildResponse struct {
	Schema            string                `json:"schema"`
	CandidateRevision string                `json:"candidate_revision"`
	BuildModified     bool                  `json:"build_modified"`
	Language          string                `json:"language"`
	Mode              string                `json:"mode"`
	ParseOrder        string                `json:"parse_order"`
	BlobSHA256        string                `json:"blob_sha256"`
	SourceSHA256      string                `json:"source_sha256"`
	Baseline          retryProfileCertParse `json:"baseline"`
	Candidate         retryProfileCertParse `json:"candidate"`
}

type retryProfileCertManifest struct {
	Schema               string                           `json:"schema"`
	Status               string                           `json:"status"`
	ResumeKey            string                           `json:"resume_key"`
	GeneratedAt          string                           `json:"generated_at"`
	Language             string                           `json:"language"`
	BlobSHA256           string                           `json:"blob_sha256"`
	CandidateRevision    string                           `json:"candidate_revision"`
	CorpusRoot           string                           `json:"corpus_root"`
	CorpusLock           string                           `json:"corpus_lock"`
	CorpusLockSHA256     string                           `json:"corpus_lock_sha256"`
	CorpusManifestSHA256 string                           `json:"corpus_manifest_sha256"`
	Oracle               perfScanOracleIdentity           `json:"oracle"`
	OracleIdentitySHA256 string                           `json:"oracle_identity_sha256"`
	GoChild              retryProfileCertGoChildIdentity  `json:"go_child"`
	ParserConfig         []string                         `json:"parser_config"`
	SelectionConfig      []string                         `json:"selection_config"`
	CandidateProfile     retryProfileCertCandidateProfile `json:"candidate_profile"`
	Totals               retryProfileCertTotals           `json:"totals"`
	Clean                retryProfileCertTotals           `json:"clean"`
	Error                retryProfileCertTotals           `json:"error"`
	Files                []retryProfileCertFile           `json:"files"`
	Counterexample       *retryProfileCertFile            `json:"counterexample,omitempty"`
	Failure              *retryProfileCertFailure         `json:"failure,omitempty"`
}

type retryProfileCertJournalRecord struct {
	Schema    string               `json:"schema"`
	ResumeKey string               `json:"resume_key"`
	File      retryProfileCertFile `json:"file"`
}

type retryProfileCertJournal struct {
	file  *os.File
	prior map[string]retryProfileCertFile
	key   string
}

func TestRetryProfileCorpusCertification(t *testing.T) {
	if strings.TrimSpace(os.Getenv("GTS_RETRY_PROFILE_CERT")) != "1" {
		t.Skip("set GTS_RETRY_PROFILE_CERT=1 to certify a retry profile")
	}
	name := strings.TrimSpace(os.Getenv("GTS_RETRY_PROFILE_CERT_LANG"))
	if name == "" || strings.Contains(name, ",") {
		t.Fatal("GTS_RETRY_PROFILE_CERT_LANG must name exactly one language")
	}
	blob := grammars.BlobByName(name)
	if len(blob) == 0 {
		t.Fatalf("BlobByName(%s) returned no data", name)
	}
	entry := grammars.DetectLanguageByName(name)
	if entry == nil || entry.Language == nil {
		t.Fatalf("language %q is not registered", name)
	}
	registered := entry.Language()
	if registered == nil {
		t.Fatalf("load registered language %q", name)
	}
	baselineValue, candidateValue := *registered, *registered
	baseline, candidate := &baselineValue, &candidateValue
	candidateProfile, err := retryProfileCertConfigureCandidate(baseline, candidate)
	if err != nil {
		t.Fatalf("configure candidate profile: %v", err)
	}
	staticOracle, err := buildStaticCPerfOracle(name)
	if err != nil {
		t.Fatalf("build locked static C oracle: %v", err)
	}
	defer staticOracle.Close()

	root := realCorpusBenchmarkRootForTest(t)
	langRoot := realCorpusBenchmarkLanguageRoot(t, root, name)
	files := loadRealCorpusBenchmarkFiles(t, langRoot, realCorpusBenchmarkFileFiltersFor(t, name, root))
	lockPath := strings.TrimSpace(os.Getenv("GTS_REAL_CORPUS_BENCH_LOCK"))
	if lockPath == "" {
		t.Fatal("set GTS_REAL_CORPUS_BENCH_LOCK to the authenticated corpus lock")
	}
	lockData, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read corpus lock: %v", err)
	}
	blobSum := sha256.Sum256(blob)
	lockSum := sha256.Sum256(lockData)
	candidateRevision, err := retryProfileCertCandidateRevision()
	if err != nil {
		t.Fatalf("authenticate candidate revision: %v", err)
	}
	goChild, err := retryProfileCertBuildGoChild(t, candidateRevision)
	if err != nil {
		t.Fatalf("build pure-Go certification child: %v", err)
	}
	oracleIdentityData, err := json.Marshal(staticOracle.identity)
	if err != nil {
		t.Fatalf("encode locked static C oracle identity: %v", err)
	}
	oracleIdentitySum := sha256.Sum256(oracleIdentityData)
	manifest := retryProfileCertManifest{
		Schema:               retryProfileCertSchema,
		GeneratedAt:          time.Now().UTC().Format(time.RFC3339),
		Language:             name,
		BlobSHA256:           hex.EncodeToString(blobSum[:]),
		CandidateRevision:    candidateRevision,
		CorpusRoot:           langRoot,
		CorpusLock:           lockPath,
		CorpusLockSHA256:     hex.EncodeToString(lockSum[:]),
		Oracle:               staticOracle.identity,
		OracleIdentitySHA256: hex.EncodeToString(oracleIdentitySum[:]),
		GoChild:              goChild.identity,
		ParserConfig:         retryProfileCertParserConfig(),
		SelectionConfig:      retryProfileCertSelectionConfig(),
		Files:                make([]retryProfileCertFile, 0, len(files)),
	}
	manifest.CandidateProfile = candidateProfile
	manifest.ResumeKey = retryProfileCertResumeKey(manifest)
	journal, err := openRetryProfileCertJournal(manifest)
	if err != nil {
		t.Fatalf("open retry-profile journal: %v", err)
	}
	if journal != nil {
		defer journal.Close()
	}
	defer func() {
		if manifest.Status == "" {
			manifest.Status = "failed"
		}
		if err := retryProfileCertWriteManifest(manifest); err != nil {
			t.Errorf("write retry-profile receipt: %v", err)
		}
	}()
	corpusHash := sha256.New()

	for fileIndex, file := range files {
		rel, _ := filepath.Rel(langRoot, file.path)
		sourceSum := sha256.Sum256(file.source)
		_, _ = fmt.Fprintf(corpusHash, "%s\x00%d\x00%x\n", filepath.ToSlash(rel), len(file.source), sourceSum)
		manifest.CorpusManifestSHA256 = hex.EncodeToString(corpusHash.Sum(nil))
		if journal != nil {
			if prior, ok, err := retryProfileCertResumeRow(journal, filepath.ToSlash(rel), file.source); err != nil {
				manifest.Failure = &retryProfileCertFailure{Path: filepath.ToSlash(rel), Reason: "invalid resumed row: " + err.Error()}
				t.Fatalf("resume %s: %v", rel, err)
			} else if ok {
				retryProfileCertAccumulate(&manifest, prior)
				continue
			}
		}

		parseOrder := retryProfileCertBaselineFirst
		if retryProfileCertCandidateRunsFirst(name, fileIndex) {
			parseOrder = retryProfileCertCandidateFirst
		}
		baselineResult, candidateResult, err := retryProfileCertParsePair(
			t, goChild, candidateRevision, name, hex.EncodeToString(blobSum[:]),
			candidateProfile.Mode, parseOrder, file.path, file.source,
		)
		if err != nil {
			manifest.Failure = &retryProfileCertFailure{Path: filepath.ToSlash(rel), Reason: err.Error()}
			t.Fatalf("parse isolated Go pair %s: %v", rel, err)
		}
		oracleDigest, oracleStatus, oracleDetail := retryProfileCertStaticOracle(staticOracle, file.source)
		class := "clean"
		if candidateResult.HasError {
			class = "error"
		}
		attemptsEliminated := len(baselineResult.Attempts) - len(candidateResult.Attempts)
		policyEffect := retryProfileCertEffectUnchanged
		if attemptsEliminated > 0 {
			policyEffect = retryProfileCertEffectActivated
		}
		row := retryProfileCertFile{
			Path:                  filepath.ToSlash(rel),
			Bytes:                 len(file.source),
			SourceSHA256:          hex.EncodeToString(sourceSum[:]),
			Class:                 class,
			ParseOrder:            parseOrder,
			PolicyMode:            candidateProfile.Mode,
			PolicyEffect:          policyEffect,
			AttemptsEliminated:    attemptsEliminated,
			OracleDeepSHA256:      oracleDigest,
			BaselineOracleParity:  oracleDigest != "" && baselineResult.DeepSHA256 == oracleDigest,
			CandidateOracleParity: oracleDigest != "" && candidateResult.DeepSHA256 == oracleDigest,
			OracleStatus:          oracleStatus,
			OracleDetail:          oracleDetail,
			Baseline:              baselineResult,
			Candidate:             candidateResult,
		}
		if row.OracleStatus == "completed" {
			if row.CandidateOracleParity {
				row.OracleStatus = "matched"
			} else {
				row.OracleStatus = "mismatch"
			}
		}
		if err := validateRetryProfileCertRow(row, filepath.ToSlash(rel), file.source); err != nil {
			manifest.Counterexample = &row
			manifest.Failure = &retryProfileCertFailure{Path: row.Path, Reason: err.Error()}
			t.Fatalf("%s: %v", rel, err)
		}
		if journal != nil {
			if err := journal.Append(row); err != nil {
				t.Fatalf("append retry-profile journal: %v", err)
			}
		}
		retryProfileCertAccumulate(&manifest, row)
	}
	if journal != nil && len(journal.prior) != 0 {
		paths := make([]string, 0, len(journal.prior))
		for path := range journal.prior {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		manifest.Failure = &retryProfileCertFailure{Reason: fmt.Sprintf("journal contains %d unselected rows", len(paths))}
		t.Fatalf("journal contains unselected rows: %s", strings.Join(paths, ", "))
	}
	if manifest.Totals.BaselineAttempts <= manifest.Totals.CandidateAttempts {
		manifest.Failure = &retryProfileCertFailure{Reason: "candidate did not eliminate retry attempts"}
		t.Fatalf("candidate did not eliminate retry attempts: baseline=%d candidate=%d", manifest.Totals.BaselineAttempts, manifest.Totals.CandidateAttempts)
	}
	if manifest.Totals.ActivatedFiles == 0 {
		manifest.Failure = &retryProfileCertFailure{Reason: "candidate policy did not activate"}
		t.Fatal("candidate policy did not activate on the selected corpus")
	}
	if manifest.Totals.CandidateWallNanos*100 > manifest.Totals.BaselineWallNanos*98 {
		manifest.Failure = &retryProfileCertFailure{Reason: "candidate wall-time improvement is below 2%"}
		t.Fatalf("candidate wall-time improvement below 2%%: baseline=%s candidate=%s",
			time.Duration(manifest.Totals.BaselineWallNanos), time.Duration(manifest.Totals.CandidateWallNanos))
	}
	manifest.Status = "passed"
	t.Logf("retry profile certified: language=%s mode=%s files=%d activated=%d unchanged=%d bytes=%d attempts=%d->%d wall=%s->%s oracle=%d_match/%d_mismatch/%d_crash/%d_unavailable manifest=%s",
		name, manifest.CandidateProfile.Mode, manifest.Totals.Files,
		manifest.Totals.ActivatedFiles, manifest.Totals.UnchangedFiles, manifest.Totals.Bytes,
		manifest.Totals.BaselineAttempts, manifest.Totals.CandidateAttempts,
		time.Duration(manifest.Totals.BaselineWallNanos), time.Duration(manifest.Totals.CandidateWallNanos),
		manifest.Totals.OracleMatches, manifest.Totals.OracleMismatches,
		manifest.Totals.OracleCrashes, manifest.Totals.OracleUnavailable,
		manifest.CorpusManifestSHA256)
}

func retryProfileCertConfigureCandidate(baseline, candidate *gotreesitter.Language) (retryProfileCertCandidateProfile, error) {
	if baseline == nil || candidate == nil {
		return retryProfileCertCandidateProfile{}, fmt.Errorf("baseline and candidate languages are required")
	}
	if baseline == candidate {
		return retryProfileCertCandidateProfile{}, fmt.Errorf("baseline and candidate languages share one instance")
	}
	mode := strings.TrimSpace(os.Getenv(retryProfileCertEnvMode))
	if mode == "" {
		mode = retryProfileCertModeScanner
	}
	switch mode {
	case retryProfileCertModeScanner:
		baseline.ExternalScannerFullParseRetryPolicy = gotreesitter.ExternalScannerFullParseRetryDefault
		if candidate.ExternalScannerFullParseRetryPolicy != gotreesitter.ExternalScannerFullParseRetrySkipRepeat {
			return retryProfileCertCandidateProfile{}, fmt.Errorf("candidate does not suppress the external-scanner repeat")
		}
	case retryProfileCertModeSkipComplete:
		baselineProfile := baseline.FullParseAcceptedErrorRetryProfile
		baselineProfile.SkipCompleteAcceptedErrorRetry = false
		baseline.FullParseAcceptedErrorRetryProfile = baselineProfile
		if !candidate.FullParseAcceptedErrorRetryProfile.SkipCompleteAcceptedErrorRetry {
			return retryProfileCertCandidateProfile{}, fmt.Errorf("candidate does not suppress the complete accepted-error retry")
		}
	case retryProfileCertModeSkipFresh:
		baselineProfile := baseline.FullParseAcceptedErrorRetryProfile
		baselineProfile.SkipFreshCompleteAcceptedErrorRetry = false
		baseline.FullParseAcceptedErrorRetryProfile = baselineProfile
		if !candidate.FullParseAcceptedErrorRetryProfile.SkipFreshCompleteAcceptedErrorRetry {
			return retryProfileCertCandidateProfile{}, fmt.Errorf("candidate does not suppress the fresh complete accepted-error retry")
		}
	case retryProfileCertModeShortLadder:
		baselineProfile := baseline.FullParseAcceptedErrorRetryProfile
		baselineProfile.SkipInitialCompleteAcceptedErrorMergeRetry = false
		baselineProfile.ReuseCleanWideForWideRetry = false
		baselineProfile.ReuseCleanWideMinSourceBytes = 0
		baseline.FullParseAcceptedErrorRetryProfile = baselineProfile
		candidateProfile := candidate.FullParseAcceptedErrorRetryProfile
		if !candidateProfile.SkipInitialCompleteAcceptedErrorMergeRetry ||
			!candidateProfile.ReuseCleanWideForWideRetry || candidateProfile.ReuseCleanWideMinSourceBytes == 0 {
			return retryProfileCertCandidateProfile{}, fmt.Errorf("candidate does not select the short accepted-error ladder")
		}
	case retryProfileCertModeReuseClean:
		baselineProfile := baseline.FullParseAcceptedErrorRetryProfile
		baselineProfile.SkipInitialCompleteAcceptedErrorMergeRetry = false
		baselineProfile.ReuseCleanWideForWideRetry = false
		baselineProfile.ReuseCleanWideMinSourceBytes = 0
		baseline.FullParseAcceptedErrorRetryProfile = baselineProfile
		candidateProfile := candidate.FullParseAcceptedErrorRetryProfile
		if candidateProfile.SkipInitialCompleteAcceptedErrorMergeRetry ||
			!candidateProfile.ReuseCleanWideForWideRetry || candidateProfile.ReuseCleanWideMinSourceBytes == 0 {
			return retryProfileCertCandidateProfile{}, fmt.Errorf("candidate does not select clean-wide reuse alone")
		}
	default:
		return retryProfileCertCandidateProfile{}, fmt.Errorf("invalid %s %q", retryProfileCertEnvMode, mode)
	}
	candidateRetry := candidate.FullParseAcceptedErrorRetryProfile
	return retryProfileCertCandidateProfile{
		Mode:                            mode,
		SkipExternalScannerRepeat:       candidate.ExternalScannerFullParseRetryPolicy == gotreesitter.ExternalScannerFullParseRetrySkipRepeat,
		SkipCompleteAcceptedError:       candidateRetry.SkipCompleteAcceptedErrorRetry,
		SkipFreshCompleteAcceptedError:  candidateRetry.SkipFreshCompleteAcceptedErrorRetry,
		SkipInitialAcceptedErrorMerge:   candidateRetry.SkipInitialCompleteAcceptedErrorMergeRetry,
		SkipCompleteMinSourceBytes:      candidateRetry.SkipCompleteMinSourceBytes,
		SkipCompleteMaxEntryScratchPeak: candidateRetry.SkipCompleteMaxEntryScratchPeak,
		SkipCompleteMaxStacksSeen:       candidateRetry.SkipCompleteMaxStacksSeen,
		ReuseCleanWideForWideRetry:      candidateRetry.ReuseCleanWideForWideRetry,
		ReuseCleanWideMinSourceBytes:    candidateRetry.ReuseCleanWideMinSourceBytes,
	}, nil
}

func retryProfileCertEquivalent(baseline, candidate retryProfileCertParse) bool {
	return baseline.TreePresent == candidate.TreePresent &&
		baseline.DeepSHA256 == candidate.DeepSHA256 &&
		baseline.StopReason == candidate.StopReason &&
		baseline.RootStart == candidate.RootStart &&
		baseline.RootEnd == candidate.RootEnd &&
		baseline.HasError == candidate.HasError
}

func retryProfileCertAttemptsEquivalent(baseline, candidate []retryProfileCertAttempt) bool {
	if len(baseline) != len(candidate) {
		return false
	}
	for i := range baseline {
		if baseline[i] != candidate[i] {
			return false
		}
	}
	return true
}

func retryProfileCertCandidateRunsFirst(language string, fileIndex int) bool {
	sum := sha256.Sum256([]byte("retry-profile-order\x00" + language))
	return (int(sum[0]&1)+fileIndex)%2 == 1
}

func validateRetryProfileCertRow(row retryProfileCertFile, wantPath string, source []byte) error {
	cleanPath := filepath.ToSlash(filepath.Clean(row.Path))
	if row.Path == "" || filepath.IsAbs(row.Path) || cleanPath != row.Path || row.Path == ".." || strings.HasPrefix(row.Path, "../") {
		return fmt.Errorf("invalid normalized path %q", row.Path)
	}
	if row.Path != wantPath {
		return fmt.Errorf("path=%q want=%q", row.Path, wantPath)
	}
	if row.Bytes != len(source) {
		return fmt.Errorf("bytes=%d want=%d", row.Bytes, len(source))
	}
	sourceSum := sha256.Sum256(source)
	if row.SourceSHA256 != hex.EncodeToString(sourceSum[:]) {
		return fmt.Errorf("source sha256=%q want=%x", row.SourceSHA256, sourceSum)
	}
	if err := validateRetryProfileCertParse("baseline", row.Baseline, len(source)); err != nil {
		return err
	}
	if err := validateRetryProfileCertParse("candidate", row.Candidate, len(source)); err != nil {
		return err
	}
	if !retryProfileCertEquivalent(row.Baseline, row.Candidate) {
		return fmt.Errorf("baseline and candidate parse results differ")
	}
	wantClass := "clean"
	if row.Candidate.HasError {
		wantClass = "error"
	}
	if row.Class != wantClass {
		return fmt.Errorf("class=%q want=%q", row.Class, wantClass)
	}
	if row.ParseOrder != retryProfileCertBaselineFirst && row.ParseOrder != retryProfileCertCandidateFirst {
		return fmt.Errorf("invalid parse order %q", row.ParseOrder)
	}
	if !retryProfileCertValidMode(row.PolicyMode) {
		return fmt.Errorf("invalid policy mode %q", row.PolicyMode)
	}
	attemptsEliminated := len(row.Baseline.Attempts) - len(row.Candidate.Attempts)
	if row.AttemptsEliminated != attemptsEliminated {
		return fmt.Errorf("attempts_eliminated=%d want=%d", row.AttemptsEliminated, attemptsEliminated)
	}
	if attemptsEliminated < 0 {
		return fmt.Errorf("candidate added %d retry attempts", -attemptsEliminated)
	}
	switch row.PolicyEffect {
	case retryProfileCertEffectActivated:
		if attemptsEliminated == 0 {
			return fmt.Errorf("activated policy eliminated no retry attempts")
		}
		if row.PolicyMode == retryProfileCertModeShortLadder || row.PolicyMode == retryProfileCertModeReuseClean {
			if !retryProfileCertReducedLadderAttemptsEquivalent(row.PolicyMode, row.Baseline.Attempts, row.Candidate.Attempts) {
				return fmt.Errorf("reduced ladder altered an attempt outside its certified rungs")
			}
		} else if !row.Candidate.TreePresent || row.Candidate.StopReason != gotreesitter.ParseStopAccepted ||
			row.Candidate.RootEnd != uint32(len(source)) || !row.Candidate.HasError {
			return fmt.Errorf("activated policy lacks a complete accepted-error result")
		}
	case retryProfileCertEffectUnchanged:
		if attemptsEliminated != 0 {
			return fmt.Errorf("unchanged policy eliminated %d retry attempts", attemptsEliminated)
		}
		if !retryProfileCertAttemptsEquivalent(row.Baseline.Attempts, row.Candidate.Attempts) {
			return fmt.Errorf("unchanged policy altered the parse attempt sequence")
		}
	default:
		return fmt.Errorf("invalid policy effect %q", row.PolicyEffect)
	}
	baselineOracleParity := row.OracleDeepSHA256 != "" && row.Baseline.DeepSHA256 == row.OracleDeepSHA256
	candidateOracleParity := row.OracleDeepSHA256 != "" && row.Candidate.DeepSHA256 == row.OracleDeepSHA256
	if row.BaselineOracleParity != baselineOracleParity {
		return fmt.Errorf("baseline_oracle_parity=%t want=%t from authenticated digests", row.BaselineOracleParity, baselineOracleParity)
	}
	if row.CandidateOracleParity != candidateOracleParity {
		return fmt.Errorf("candidate_oracle_parity=%t want=%t from authenticated digests", row.CandidateOracleParity, candidateOracleParity)
	}
	if row.BaselineOracleParity != row.CandidateOracleParity {
		return fmt.Errorf("baseline/candidate oracle relation differs")
	}
	switch row.OracleStatus {
	case "matched":
		if !validRetryProfileCertSHA256(row.OracleDeepSHA256) || !row.BaselineOracleParity || !row.CandidateOracleParity || row.OracleDetail != "" {
			return fmt.Errorf("invalid matched oracle result")
		}
	case "mismatch":
		if !validRetryProfileCertSHA256(row.OracleDeepSHA256) || row.BaselineOracleParity || row.CandidateOracleParity || row.OracleDetail != "" {
			return fmt.Errorf("invalid mismatched oracle result")
		}
	case "oracle_crash",
		staticCStatusAdmissionError,
		staticCStatusParserError, staticCStatusParserTimeout,
		staticCStatusTransportError, staticCStatusTransportTimeout,
		staticCStatusDigestError, staticCStatusDigestTimeout,
		staticCStatusProtocolError, staticCStatusIncomplete:
		if row.OracleDeepSHA256 != "" || row.BaselineOracleParity || row.CandidateOracleParity || strings.TrimSpace(row.OracleDetail) == "" {
			return fmt.Errorf("invalid unavailable oracle result status=%q", row.OracleStatus)
		}
	default:
		return fmt.Errorf("invalid oracle status %q", row.OracleStatus)
	}
	return nil
}

func retryProfileCertValidMode(mode string) bool {
	switch mode {
	case retryProfileCertModeScanner, retryProfileCertModeSkipComplete,
		retryProfileCertModeSkipFresh, retryProfileCertModeShortLadder,
		retryProfileCertModeReuseClean:
		return true
	default:
		return false
	}
}

func retryProfileCertReducedLadderAttemptsEquivalent(mode string, baseline, candidate []retryProfileCertAttempt) bool {
	candidateIndex := 0
	for _, attempt := range baseline {
		if candidateIndex < len(candidate) && attempt == candidate[candidateIndex] {
			candidateIndex++
			continue
		}
		switch attempt.LogicalRung {
		case "initial_merge":
			if mode != retryProfileCertModeShortLadder {
				return false
			}
		case "recovery_wide_or_node":
		default:
			return false
		}
	}
	return candidateIndex == len(candidate)
}

func retryProfileCertResumeRow(journal *retryProfileCertJournal, path string, source []byte) (retryProfileCertFile, bool, error) {
	if journal == nil {
		return retryProfileCertFile{}, false, nil
	}
	row, ok := journal.prior[path]
	if !ok {
		return retryProfileCertFile{}, false, nil
	}
	if err := validateRetryProfileCertRow(row, path, source); err != nil {
		return retryProfileCertFile{}, false, err
	}
	delete(journal.prior, path)
	return row, true, nil
}

func validateRetryProfileCertParse(label string, parse retryProfileCertParse, sourceBytes int) error {
	if parse.WallNanos <= 0 {
		return fmt.Errorf("%s wall_nanos=%d", label, parse.WallNanos)
	}
	if parse.TreePresent {
		if !validRetryProfileCertSHA256(parse.DeepSHA256) {
			return fmt.Errorf("%s has invalid deep digest %q", label, parse.DeepSHA256)
		}
	} else if parse.DeepSHA256 != "" || parse.RootStart != 0 {
		return fmt.Errorf("%s absent tree contains root data", label)
	}
	if parse.StopReason == gotreesitter.ParseStopNone || !validRetryProfileCertStopReason(parse.StopReason) {
		return fmt.Errorf("%s has invalid final stop reason %q", label, parse.StopReason)
	}
	if parse.RootStart > parse.RootEnd || parse.RootEnd > uint32(sourceBytes) {
		return fmt.Errorf("%s root=%d..%d exceeds source=%d", label, parse.RootStart, parse.RootEnd, sourceBytes)
	}
	if len(parse.Attempts) == 0 {
		return fmt.Errorf("%s has no parse attempts", label)
	}
	for i, attempt := range parse.Attempts {
		if strings.TrimSpace(attempt.LogicalRung) == "" || strings.TrimSpace(attempt.OperationCause) == "" {
			return fmt.Errorf("%s attempt %d lacks rung/cause", label, i)
		}
		if !validRetryProfileCertStopReason(attempt.StopReason) {
			return fmt.Errorf("%s attempt %d has invalid stop reason %q", label, i, attempt.StopReason)
		}
		if attempt.RootEndByte > uint32(sourceBytes) || attempt.ResolvedMaxStacks <= 0 || attempt.ResolvedMergeLimit <= 0 {
			return fmt.Errorf("%s attempt %d has invalid bounds", label, i)
		}
	}
	return nil
}

func validRetryProfileCertSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validRetryProfileCertStopReason(reason gotreesitter.ParseStopReason) bool {
	switch reason {
	case gotreesitter.ParseStopNone,
		gotreesitter.ParseStopAccepted,
		gotreesitter.ParseStopNoStacksAlive,
		gotreesitter.ParseStopTokenSourceEOF,
		gotreesitter.ParseStopTimeout,
		gotreesitter.ParseStopCancelled,
		gotreesitter.ParseStopIterationLimit,
		gotreesitter.ParseStopStackDepthLimit,
		gotreesitter.ParseStopNodeLimit,
		gotreesitter.ParseStopMemoryBudget:
		return true
	default:
		return false
	}
}

func retryProfileCertAccumulate(manifest *retryProfileCertManifest, row retryProfileCertFile) {
	manifest.Files = append(manifest.Files, row)
	retryProfileCertAddTotals(&manifest.Totals, row)
	if row.Class == "error" {
		retryProfileCertAddTotals(&manifest.Error, row)
	} else {
		retryProfileCertAddTotals(&manifest.Clean, row)
	}
}

func retryProfileCertResumeKey(manifest retryProfileCertManifest) string {
	candidateProfile, err := json.Marshal(manifest.CandidateProfile)
	if err != nil {
		panic("encode retry-profile candidate: " + err.Error())
	}
	goChild, err := json.Marshal(manifest.GoChild)
	if err != nil {
		panic("encode retry-profile Go child: " + err.Error())
	}
	parts := []string{
		manifest.Schema,
		manifest.Language,
		manifest.BlobSHA256,
		manifest.CandidateRevision,
		manifest.CorpusRoot,
		manifest.CorpusLock,
		manifest.CorpusLockSHA256,
		manifest.OracleIdentitySHA256,
		string(candidateProfile),
		string(goChild),
		"parser_config",
	}
	parts = append(parts, manifest.ParserConfig...)
	parts = append(parts, "selection_config")
	parts = append(parts, manifest.SelectionConfig...)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func openRetryProfileCertJournal(manifest retryProfileCertManifest) (*retryProfileCertJournal, error) {
	out := strings.TrimSpace(os.Getenv("GTS_RETRY_PROFILE_CERT_OUT"))
	if out == "" {
		return nil, nil
	}
	path := out + ".files.jsonl"
	journal := &retryProfileCertJournal{key: manifest.ResumeKey, prior: make(map[string]retryProfileCertFile)}
	if strings.TrimSpace(os.Getenv("GTS_RETRY_PROFILE_CERT_RESUME")) == "1" {
		input, err := os.Open(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if input != nil {
			scanner := bufio.NewScanner(input)
			scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
			for scanner.Scan() {
				var record retryProfileCertJournalRecord
				if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
					_ = input.Close()
					return nil, fmt.Errorf("decode %s: %w", path, err)
				}
				if record.Schema != retryProfileCertSchema {
					_ = input.Close()
					return nil, fmt.Errorf("%s contains schema %q, want %q", path, record.Schema, retryProfileCertSchema)
				}
				if record.ResumeKey != manifest.ResumeKey {
					_ = input.Close()
					return nil, fmt.Errorf("%s belongs to resume key %s, want %s", path, record.ResumeKey, manifest.ResumeKey)
				}
				if _, exists := journal.prior[record.File.Path]; exists {
					_ = input.Close()
					return nil, fmt.Errorf("%s contains duplicate path %q", path, record.File.Path)
				}
				journal.prior[record.File.Path] = record.File
			}
			if err := scanner.Err(); err != nil {
				_ = input.Close()
				return nil, err
			}
			if err := input.Close(); err != nil {
				return nil, err
			}
		}
	} else if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	journal.file = file
	return journal, nil
}

func (journal *retryProfileCertJournal) Append(row retryProfileCertFile) error {
	data, err := json.Marshal(retryProfileCertJournalRecord{Schema: retryProfileCertSchema, ResumeKey: journal.key, File: row})
	if err != nil {
		return err
	}
	_, err = journal.file.Write(append(data, '\n'))
	return err
}

func (journal *retryProfileCertJournal) Close() error {
	if journal == nil || journal.file == nil {
		return nil
	}
	return journal.file.Close()
}

func retryProfileCertWriteManifest(manifest retryProfileCertManifest) error {
	out := strings.TrimSpace(os.Getenv("GTS_RETRY_PROFILE_CERT_OUT"))
	if out == "" {
		return nil
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	return os.WriteFile(out, append(data, '\n'), 0o644)
}

func retryProfileCertStaticOracle(oracle *staticCPerfOracle, source []byte) (digest, status, detail string) {
	digest, _, err := oracle.deepDigest(source, 10*time.Second)
	if err == nil {
		return digest, "completed", ""
	}
	status = staticCOracleErrorStatus(err)
	detail = err.Error()
	lower := strings.ToLower(detail)
	if strings.Contains(lower, "sigabrt") || strings.Contains(lower, "signal: aborted") || strings.Contains(lower, "assertion") {
		status = "oracle_crash"
	}
	return "", status, detail
}

func realCorpusBenchmarkRootForTest(t testing.TB) string {
	t.Helper()
	if root := strings.TrimSpace(os.Getenv("GTS_REAL_CORPUS_BENCH_ROOT")); root != "" {
		return root
	}
	t.Fatal("set GTS_REAL_CORPUS_BENCH_ROOT")
	return ""
}

func retryProfileCertBuildGoChild(t testing.TB, candidateRevision string) (retryProfileCertGoChild, error) {
	t.Helper()
	repoRoot, err := retryProfileCertGitOutput("rev-parse", "--show-toplevel")
	if err != nil {
		return retryProfileCertGoChild{}, err
	}
	pairTimeout, err := retryProfileCertEffectiveChildTimeout()
	if err != nil {
		return retryProfileCertGoChild{}, err
	}
	binaryPath := filepath.Join(t.TempDir(), "retry-profile-cert-child")
	command := exec.Command("go", "build", "-trimpath", "-buildvcs=true", "-tags", "gts_workcount", "-o", binaryPath, "./cmd/retry_profile_cert_child")
	command.Dir = repoRoot
	command.Env = retryProfileCertEnvWithOverrides(os.Environ(), map[string]string{
		"CGO_ENABLED": "0",
		"GOWORK":      "off",
	})
	if output, err := command.CombinedOutput(); err != nil {
		return retryProfileCertGoChild{}, fmt.Errorf("build pure-Go child: %w: %s", err, strings.TrimSpace(string(output)))
	}
	info, err := buildinfo.ReadFile(binaryPath)
	if err != nil {
		return retryProfileCertGoChild{}, fmt.Errorf("read pure-Go child build info: %w", err)
	}
	var revision string
	var modified bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if revision != candidateRevision || modified {
		return retryProfileCertGoChild{}, fmt.Errorf("pure-Go child revision=%q modified=%t want revision=%q modified=false", revision, modified, candidateRevision)
	}
	binary, err := os.ReadFile(binaryPath)
	if err != nil {
		return retryProfileCertGoChild{}, fmt.Errorf("read pure-Go child: %w", err)
	}
	binarySum := sha256.Sum256(binary)
	return retryProfileCertGoChild{
		path: binaryPath,
		identity: retryProfileCertGoChildIdentity{
			Schema:            retryProfileCertChildSchema,
			BinarySHA256:      hex.EncodeToString(binarySum[:]),
			CandidateRevision: candidateRevision,
			BuildTags:         []string{"gts_workcount"},
			CGOEnabled:        false,
			PairTimeout:       pairTimeout.String(),
		},
	}, nil
}

func retryProfileCertEffectiveChildTimeout() (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(retryProfileCertEnvChildTimeout))
	if raw == "" {
		return retryProfileCertChildTimeout, nil
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil || timeout <= 0 {
		return 0, fmt.Errorf("invalid %s %q", retryProfileCertEnvChildTimeout, raw)
	}
	return timeout, nil
}

func retryProfileCertEnvWithOverrides(input []string, overrides map[string]string) []string {
	output := make([]string, 0, len(input)+len(overrides))
	for _, entry := range input {
		key, _, ok := strings.Cut(entry, "=")
		if ok {
			if _, replaced := overrides[key]; replaced {
				continue
			}
		}
		output = append(output, entry)
	}
	keys := make([]string, 0, len(overrides))
	for key := range overrides {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		output = append(output, key+"="+overrides[key])
	}
	return output
}

func retryProfileCertParsePair(
	t testing.TB,
	child retryProfileCertGoChild,
	candidateRevision, language, blobSHA256, mode, order, sourcePath string,
	source []byte,
) (retryProfileCertParse, retryProfileCertParse, error) {
	t.Helper()
	timeout, err := time.ParseDuration(child.identity.PairTimeout)
	if err != nil || timeout <= 0 {
		return retryProfileCertParse{}, retryProfileCertParse{}, fmt.Errorf("invalid child pair timeout %q", child.identity.PairTimeout)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	command := exec.CommandContext(ctx, child.path,
		"-language", language,
		"-mode", mode,
		"-order", order,
		"-source", sourcePath,
	)
	var stderr strings.Builder
	command.Stderr = &stderr
	output, err := command.Output()
	if ctx.Err() != nil {
		return retryProfileCertParse{}, retryProfileCertParse{}, fmt.Errorf("pure-Go child exceeded %s", timeout)
	}
	if err != nil {
		return retryProfileCertParse{}, retryProfileCertParse{}, fmt.Errorf("pure-Go child: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var response retryProfileCertGoChildResponse
	if err := json.Unmarshal(output, &response); err != nil {
		return retryProfileCertParse{}, retryProfileCertParse{}, fmt.Errorf("decode pure-Go child: %w", err)
	}
	sourceSum := sha256.Sum256(source)
	if response.Schema != retryProfileCertChildSchema ||
		response.CandidateRevision != candidateRevision || response.BuildModified ||
		response.Language != language || response.Mode != mode || response.ParseOrder != order ||
		response.BlobSHA256 != blobSHA256 || response.SourceSHA256 != hex.EncodeToString(sourceSum[:]) {
		return retryProfileCertParse{}, retryProfileCertParse{}, fmt.Errorf("pure-Go child identity mismatch: %+v", response)
	}
	return response.Baseline, response.Candidate, nil
}

func retryProfileCertAddTotals(totals *retryProfileCertTotals, row retryProfileCertFile) {
	totals.Files++
	totals.Bytes += int64(row.Bytes)
	if row.ParseOrder == retryProfileCertCandidateFirst {
		totals.CandidateFirstFiles++
	} else {
		totals.BaselineFirstFiles++
	}
	if row.PolicyEffect == retryProfileCertEffectActivated {
		totals.ActivatedFiles++
	} else {
		totals.UnchangedFiles++
	}
	totals.BaselineWallNanos += row.Baseline.WallNanos
	totals.CandidateWallNanos += row.Candidate.WallNanos
	totals.BaselineTotalAlloc += row.Baseline.TotalAlloc
	totals.CandidateTotalAlloc += row.Candidate.TotalAlloc
	totals.BaselineAttempts += len(row.Baseline.Attempts)
	totals.CandidateAttempts += len(row.Candidate.Attempts)
	switch row.OracleStatus {
	case "matched":
		totals.OracleMatches++
	case "mismatch":
		totals.OracleMismatches++
	case "oracle_crash":
		totals.OracleCrashes++
	default:
		totals.OracleUnavailable++
	}
}

func retryProfileCertCandidateRevision() (string, error) {
	explicit := strings.TrimSpace(os.Getenv("GTS_RETRY_PROFILE_CERT_CANDIDATE_REVISION"))
	buildRevision, buildModified := retryProfileCertBuildRevision()
	if explicit == "" {
		if buildRevision == "" {
			return "", fmt.Errorf("build revision unavailable; set GTS_RETRY_PROFILE_CERT_CANDIDATE_REVISION to the exact clean HEAD")
		}
		explicit = buildRevision
	}
	if !validRetryProfileCertRevision(explicit) {
		return "", fmt.Errorf("candidate revision %q is not a canonical 40- or 64-digit lowercase hex revision", explicit)
	}
	if buildModified {
		return "", fmt.Errorf("candidate build reports vcs.modified=true")
	}
	if buildRevision != "" && buildRevision != explicit {
		return "", fmt.Errorf("candidate revision %s differs from build revision %s", explicit, buildRevision)
	}
	head, err := retryProfileCertGitOutput("rev-parse", "--verify", "HEAD")
	if err != nil {
		return "", fmt.Errorf("authenticate git HEAD: %w", err)
	}
	if head != explicit {
		return "", fmt.Errorf("candidate revision %s differs from git HEAD %s", explicit, head)
	}
	resolved, err := retryProfileCertGitOutput("rev-parse", "--verify", explicit+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("resolve candidate revision: %w", err)
	}
	if resolved != explicit {
		return "", fmt.Errorf("candidate revision %s resolves to %s", explicit, resolved)
	}
	status, err := retryProfileCertGitOutput("status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return "", fmt.Errorf("authenticate clean candidate worktree: %w", err)
	}
	if status != "" {
		return "", fmt.Errorf("candidate worktree is dirty")
	}
	return explicit, nil
}

func retryProfileCertBuildRevision() (string, bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}
	var revision string
	var modified bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			if validRetryProfileCertRevision(setting.Value) {
				revision = setting.Value
			}
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	return revision, modified
}

func validRetryProfileCertRevision(value string) bool {
	if (len(value) != 40 && len(value) != 64) || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func retryProfileCertGitOutput(args ...string) (string, error) {
	command := exec.Command("git", args...)
	data, err := command.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func retryProfileCertParserConfig() []string {
	return retryProfileCertEnvSnapshot(func(key string) bool {
		return strings.HasPrefix(key, "GOT_") ||
			strings.HasPrefix(key, "GOTREESITTER_GRAMMAR_") ||
			key == "GTS_ADMISSION_CANDIDATE" ||
			key == "GOMAXPROCS" || key == "GOMEMLIMIT" || key == "GODEBUG"
	})
}

func retryProfileCertSelectionConfig() []string {
	return retryProfileCertEnvSnapshot(func(key string) bool {
		return strings.HasPrefix(key, "GTS_REAL_CORPUS_BENCH_") ||
			key == "GTS_RETRY_PROFILE_CERT_LANG" || key == retryProfileCertEnvMode ||
			key == retryProfileCertEnvChildTimeout ||
			key == "GTS_C_ORACLE_CACHE"
	})
}

func retryProfileCertEnvSnapshot(include func(string) bool) []string {
	config := make([]string, 0)
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if ok && include(key) {
			config = append(config, entry)
		}
	}
	sort.Strings(config)
	return config
}

func TestRetryProfileCertConfigureCandidate(t *testing.T) {
	t.Run("external scanner", func(t *testing.T) {
		t.Setenv(retryProfileCertEnvMode, retryProfileCertModeScanner)
		baseline := &gotreesitter.Language{ExternalScannerFullParseRetryPolicy: gotreesitter.ExternalScannerFullParseRetrySkipRepeat}
		candidate := &gotreesitter.Language{ExternalScannerFullParseRetryPolicy: gotreesitter.ExternalScannerFullParseRetrySkipRepeat}
		profile, err := retryProfileCertConfigureCandidate(baseline, candidate)
		if err != nil {
			t.Fatal(err)
		}
		if baseline.ExternalScannerFullParseRetryPolicy != gotreesitter.ExternalScannerFullParseRetryDefault ||
			profile.Mode != retryProfileCertModeScanner || !profile.SkipExternalScannerRepeat {
			t.Fatalf("configured profile = %+v", profile)
		}
	})

	t.Run("complete accepted error", func(t *testing.T) {
		t.Setenv(retryProfileCertEnvMode, retryProfileCertModeSkipComplete)
		baseline := &gotreesitter.Language{FullParseAcceptedErrorRetryProfile: gotreesitter.FullParseAcceptedErrorRetryProfile{
			SkipCompleteAcceptedErrorRetry: true,
			SkipCompleteMinSourceBytes:     128 * 1024,
			SkipCompleteMaxStacksSeen:      5,
		}}
		candidate := &gotreesitter.Language{FullParseAcceptedErrorRetryProfile: baseline.FullParseAcceptedErrorRetryProfile}
		profile, err := retryProfileCertConfigureCandidate(baseline, candidate)
		if err != nil {
			t.Fatal(err)
		}
		if baseline.FullParseAcceptedErrorRetryProfile.SkipCompleteAcceptedErrorRetry ||
			profile.Mode != retryProfileCertModeSkipComplete || !profile.SkipCompleteAcceptedError ||
			profile.SkipCompleteMinSourceBytes != 128*1024 || profile.SkipCompleteMaxStacksSeen != 5 {
			t.Fatalf("configured profile = %+v", profile)
		}
	})

	t.Run("fresh complete accepted error", func(t *testing.T) {
		t.Setenv(retryProfileCertEnvMode, retryProfileCertModeSkipFresh)
		baseline := &gotreesitter.Language{FullParseAcceptedErrorRetryProfile: gotreesitter.FullParseAcceptedErrorRetryProfile{
			SkipFreshCompleteAcceptedErrorRetry: true,
			SkipCompleteMinSourceBytes:          128 * 1024,
		}}
		candidate := &gotreesitter.Language{FullParseAcceptedErrorRetryProfile: baseline.FullParseAcceptedErrorRetryProfile}
		profile, err := retryProfileCertConfigureCandidate(baseline, candidate)
		if err != nil {
			t.Fatal(err)
		}
		if baseline.FullParseAcceptedErrorRetryProfile.SkipFreshCompleteAcceptedErrorRetry ||
			profile.Mode != retryProfileCertModeSkipFresh || !profile.SkipFreshCompleteAcceptedError ||
			profile.SkipCompleteMinSourceBytes != 128*1024 {
			t.Fatalf("configured profile = %+v", profile)
		}
	})

	t.Run("short complete accepted-error ladder", func(t *testing.T) {
		t.Setenv(retryProfileCertEnvMode, retryProfileCertModeShortLadder)
		baseline := &gotreesitter.Language{FullParseAcceptedErrorRetryProfile: gotreesitter.FullParseAcceptedErrorRetryProfile{
			SkipInitialCompleteAcceptedErrorMergeRetry: true,
			SkipCompleteMinSourceBytes:                 128 * 1024,
			ReuseCleanWideForWideRetry:                 true,
			ReuseCleanWideMinSourceBytes:               128 * 1024,
		}}
		candidate := &gotreesitter.Language{FullParseAcceptedErrorRetryProfile: baseline.FullParseAcceptedErrorRetryProfile}
		profile, err := retryProfileCertConfigureCandidate(baseline, candidate)
		if err != nil {
			t.Fatal(err)
		}
		baselineProfile := baseline.FullParseAcceptedErrorRetryProfile
		if baselineProfile.SkipInitialCompleteAcceptedErrorMergeRetry || baselineProfile.ReuseCleanWideForWideRetry ||
			profile.Mode != retryProfileCertModeShortLadder || !profile.SkipInitialAcceptedErrorMerge ||
			!profile.ReuseCleanWideForWideRetry || profile.ReuseCleanWideMinSourceBytes != 128*1024 {
			t.Fatalf("configured profile = %+v baseline = %+v", profile, baselineProfile)
		}
	})

	t.Run("clean-wide reuse", func(t *testing.T) {
		t.Setenv(retryProfileCertEnvMode, retryProfileCertModeReuseClean)
		baseline := &gotreesitter.Language{FullParseAcceptedErrorRetryProfile: gotreesitter.FullParseAcceptedErrorRetryProfile{
			ReuseCleanWideForWideRetry:   true,
			ReuseCleanWideMinSourceBytes: 128 * 1024,
		}}
		candidate := &gotreesitter.Language{FullParseAcceptedErrorRetryProfile: baseline.FullParseAcceptedErrorRetryProfile}
		profile, err := retryProfileCertConfigureCandidate(baseline, candidate)
		if err != nil {
			t.Fatal(err)
		}
		baselineProfile := baseline.FullParseAcceptedErrorRetryProfile
		if baselineProfile.ReuseCleanWideForWideRetry || profile.Mode != retryProfileCertModeReuseClean ||
			profile.SkipInitialAcceptedErrorMerge || !profile.ReuseCleanWideForWideRetry ||
			profile.ReuseCleanWideMinSourceBytes != 128*1024 {
			t.Fatalf("configured profile = %+v baseline = %+v", profile, baselineProfile)
		}
	})
}

func TestRetryProfileCertParseOrderIsCounterbalanced(t *testing.T) {
	const fileCount = 29
	var candidateFirst int
	for fileIndex := 0; fileIndex < fileCount; fileIndex++ {
		if retryProfileCertCandidateRunsFirst("v", fileIndex) {
			candidateFirst++
		}
	}
	baselineFirst := fileCount - candidateFirst
	if delta := candidateFirst - baselineFirst; delta < -1 || delta > 1 {
		t.Fatalf("parse order is not counterbalanced: baseline=%d candidate=%d", baselineFirst, candidateFirst)
	}
}

func TestRetryProfileCertAllowsUnchangedIncompleteResult(t *testing.T) {
	source := []byte("x")
	row := retryProfileCertValidTestRow(source)
	row.Baseline.StopReason = gotreesitter.ParseStopNoStacksAlive
	row.Candidate.StopReason = gotreesitter.ParseStopNoStacksAlive
	row.Baseline.RootEnd = 0
	row.Candidate.RootEnd = 0
	row.Baseline.Attempts[0].StopReason = gotreesitter.ParseStopNoStacksAlive
	row.Candidate.Attempts[0].StopReason = gotreesitter.ParseStopNoStacksAlive
	row.Baseline.Attempts[0].RootEndByte = 0
	row.Candidate.Attempts[0].RootEndByte = 0
	if err := validateRetryProfileCertRow(row, row.Path, source); err != nil {
		t.Fatalf("validate unchanged incomplete result: %v", err)
	}
}

func TestRetryProfileCertAllowsUnchangedAbsentTree(t *testing.T) {
	source := []byte("x")
	row := retryProfileCertValidTestRow(source)
	for _, parse := range []*retryProfileCertParse{&row.Baseline, &row.Candidate} {
		parse.TreePresent = false
		parse.StopReason = gotreesitter.ParseStopNoStacksAlive
		parse.RootEnd = 0
		parse.DeepSHA256 = ""
		parse.Attempts[0].StopReason = gotreesitter.ParseStopNoStacksAlive
		parse.Attempts[0].RootEndByte = 0
	}
	row.OracleDeepSHA256 = strings.Repeat("d", sha256.Size*2)
	row.BaselineOracleParity = false
	row.CandidateOracleParity = false
	row.OracleStatus = "mismatch"
	if err := validateRetryProfileCertRow(row, row.Path, source); err != nil {
		t.Fatalf("validate unchanged absent tree: %v", err)
	}
}

func TestRetryProfileCertRejectsActivatedIncompleteResult(t *testing.T) {
	source := []byte("x")
	row := retryProfileCertValidTestRow(source)
	row.PolicyEffect = retryProfileCertEffectActivated
	row.AttemptsEliminated = 1
	row.Baseline.Attempts = append(row.Baseline.Attempts, row.Baseline.Attempts[0])
	row.Baseline.StopReason = gotreesitter.ParseStopNoStacksAlive
	row.Candidate.StopReason = gotreesitter.ParseStopNoStacksAlive
	if err := validateRetryProfileCertRow(row, row.Path, source); err == nil {
		t.Fatal("activated incomplete result was accepted")
	}
}

func TestRetryProfileCertAllowsShortLadderWithoutPublicTree(t *testing.T) {
	source := []byte("x")
	row := retryProfileCertValidTestRow(source)
	newAttempt := func(rung string) retryProfileCertAttempt {
		return retryProfileCertAttempt{
			LogicalRung:        rung,
			OperationCause:     rung,
			StopReason:         gotreesitter.ParseStopAccepted,
			RootHasError:       true,
			RootEndByte:        uint32(len(source)),
			ResolvedMaxStacks:  8,
			ResolvedMergeLimit: 6,
		}
	}
	initial := newAttempt("initial_full")
	initialMerge := newAttempt("initial_merge")
	cleanWide := newAttempt("clean_wide")
	recoveryWide := newAttempt("recovery_wide_or_node")
	finalMerge := newAttempt("final_merge")
	row.Baseline.Attempts = []retryProfileCertAttempt{initial, initialMerge, cleanWide, recoveryWide, finalMerge}
	row.Candidate.Attempts = []retryProfileCertAttempt{initial, cleanWide, finalMerge}
	for _, parse := range []*retryProfileCertParse{&row.Baseline, &row.Candidate} {
		parse.TreePresent = false
		parse.HasError = true
		parse.DeepSHA256 = ""
	}
	row.Class = "error"
	row.PolicyMode = retryProfileCertModeShortLadder
	row.PolicyEffect = retryProfileCertEffectActivated
	row.AttemptsEliminated = 2
	row.OracleDeepSHA256 = strings.Repeat("d", sha256.Size*2)
	row.BaselineOracleParity = false
	row.CandidateOracleParity = false
	row.OracleStatus = "mismatch"
	if err := validateRetryProfileCertRow(row, row.Path, source); err != nil {
		t.Fatalf("validate short ladder without public tree: %v", err)
	}
}

func TestRetryProfileCertReuseCleanWideRemovesOnlyRecoveryWide(t *testing.T) {
	source := []byte("x")
	row := retryProfileCertValidTestRow(source)
	newAttempt := func(rung string) retryProfileCertAttempt {
		return retryProfileCertAttempt{
			LogicalRung:        rung,
			OperationCause:     rung,
			StopReason:         gotreesitter.ParseStopAccepted,
			RootHasError:       true,
			RootEndByte:        uint32(len(source)),
			ResolvedMaxStacks:  8,
			ResolvedMergeLimit: 6,
		}
	}
	initial := newAttempt("initial_full")
	initialMerge := newAttempt("initial_merge")
	cleanWide := newAttempt("clean_wide")
	recoveryWide := newAttempt("recovery_wide_or_node")
	finalMerge := newAttempt("final_merge")
	row.Baseline.Attempts = []retryProfileCertAttempt{initial, initialMerge, cleanWide, recoveryWide, finalMerge}
	row.Candidate.Attempts = []retryProfileCertAttempt{initial, initialMerge, cleanWide, finalMerge}
	row.PolicyMode = retryProfileCertModeReuseClean
	row.PolicyEffect = retryProfileCertEffectActivated
	row.AttemptsEliminated = 1
	if err := validateRetryProfileCertRow(row, row.Path, source); err != nil {
		t.Fatalf("validate clean-wide reuse: %v", err)
	}

	row.Candidate.Attempts = []retryProfileCertAttempt{initial, cleanWide, finalMerge}
	row.AttemptsEliminated = 2
	if err := validateRetryProfileCertRow(row, row.Path, source); err == nil {
		t.Fatal("clean-wide reuse accepted an initial-merge removal")
	}
}

func TestRetryProfileCertResumeRejectsCounterexample(t *testing.T) {
	source := []byte("x")
	row := retryProfileCertValidTestRow(source)
	otherDigest := sha256.Sum256([]byte("counterexample"))
	row.Candidate.DeepSHA256 = hex.EncodeToString(otherDigest[:])

	out := filepath.Join(t.TempDir(), "receipt.json")
	t.Setenv("GTS_RETRY_PROFILE_CERT_OUT", out)
	t.Setenv("GTS_RETRY_PROFILE_CERT_RESUME", "1")
	manifest := retryProfileCertManifest{ResumeKey: strings.Repeat("a", sha256.Size*2)}
	record, err := json.Marshal(retryProfileCertJournalRecord{
		Schema:    retryProfileCertSchema,
		ResumeKey: manifest.ResumeKey,
		File:      row,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out+".files.jsonl", append(record, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	journal, err := openRetryProfileCertJournal(manifest)
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	defer journal.Close()
	if _, ok, err := retryProfileCertResumeRow(journal, row.Path, source); err == nil || ok {
		t.Fatalf("counterexample resumed: ok=%v err=%v", ok, err)
	}
	if _, exists := journal.prior[row.Path]; !exists {
		t.Fatal("invalid resumed row was consumed")
	}
}

func TestRetryProfileCertResumeRejectsForgedOracleRelation(t *testing.T) {
	source := []byte("x")
	row := retryProfileCertValidTestRow(source)
	otherDigest := sha256.Sum256([]byte("different oracle tree"))
	row.OracleDeepSHA256 = hex.EncodeToString(otherDigest[:])
	// These persisted claims are internally consistent with the status, but not
	// with either authenticated parse digest. Resume must derive the relation
	// again instead of trusting the journal's booleans.
	row.BaselineOracleParity = true
	row.CandidateOracleParity = true
	row.OracleStatus = "matched"

	out := filepath.Join(t.TempDir(), "receipt.json")
	t.Setenv("GTS_RETRY_PROFILE_CERT_OUT", out)
	t.Setenv("GTS_RETRY_PROFILE_CERT_RESUME", "1")
	manifest := retryProfileCertManifest{ResumeKey: strings.Repeat("c", sha256.Size*2)}
	record, err := json.Marshal(retryProfileCertJournalRecord{
		Schema:    retryProfileCertSchema,
		ResumeKey: manifest.ResumeKey,
		File:      row,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out+".files.jsonl", append(record, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	journal, err := openRetryProfileCertJournal(manifest)
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	defer journal.Close()
	if _, ok, err := retryProfileCertResumeRow(journal, row.Path, source); err == nil || ok {
		t.Fatalf("forged oracle relation resumed: ok=%v err=%v", ok, err)
	}
	if _, exists := journal.prior[row.Path]; !exists {
		t.Fatal("invalid resumed row was consumed")
	}
}

func TestRetryProfileCertJournalRejectsMixedSchema(t *testing.T) {
	out := filepath.Join(t.TempDir(), "receipt.json")
	t.Setenv("GTS_RETRY_PROFILE_CERT_OUT", out)
	t.Setenv("GTS_RETRY_PROFILE_CERT_RESUME", "1")
	manifest := retryProfileCertManifest{ResumeKey: strings.Repeat("b", sha256.Size*2)}
	record, err := json.Marshal(retryProfileCertJournalRecord{
		Schema:    "gts-retry-profile-cert/v1",
		ResumeKey: manifest.ResumeKey,
		File:      retryProfileCertValidTestRow([]byte("x")),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out+".files.jsonl", append(record, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if journal, err := openRetryProfileCertJournal(manifest); err == nil {
		if journal != nil {
			_ = journal.Close()
		}
		t.Fatal("mixed-schema journal was accepted")
	}
}

func TestRetryProfileCertAllowsMatchingRootAfterLeadingTrivia(t *testing.T) {
	source := []byte("\nx")
	row := retryProfileCertValidTestRow(source)
	row.Path = "x.m"
	row.Baseline.RootStart = 1
	row.Candidate.RootStart = 1
	if err := validateRetryProfileCertRow(row, row.Path, source); err != nil {
		t.Fatalf("validate matching nonzero root start: %v", err)
	}
}

func retryProfileCertValidTestRow(source []byte) retryProfileCertFile {
	sourceDigest := sha256.Sum256(source)
	treeDigest := sha256.Sum256([]byte("tree"))
	parse := retryProfileCertParse{
		WallNanos:   1,
		TotalAlloc:  1,
		TreePresent: true,
		Attempts: []retryProfileCertAttempt{{
			LogicalRung:        "initial",
			OperationCause:     "full_parse",
			StopReason:         gotreesitter.ParseStopAccepted,
			RootEndByte:        uint32(len(source)),
			ResolvedMaxStacks:  1,
			ResolvedMergeLimit: 1,
		}},
		StopReason: gotreesitter.ParseStopAccepted,
		RootEnd:    uint32(len(source)),
		DeepSHA256: hex.EncodeToString(treeDigest[:]),
	}
	return retryProfileCertFile{
		Path:                  "x.cr",
		Bytes:                 len(source),
		SourceSHA256:          hex.EncodeToString(sourceDigest[:]),
		Class:                 "clean",
		ParseOrder:            retryProfileCertBaselineFirst,
		PolicyMode:            retryProfileCertModeSkipComplete,
		PolicyEffect:          retryProfileCertEffectUnchanged,
		OracleDeepSHA256:      parse.DeepSHA256,
		BaselineOracleParity:  true,
		CandidateOracleParity: true,
		OracleStatus:          "matched",
		Baseline:              parse,
		Candidate:             parse,
	}
}
