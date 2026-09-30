//go:build gts_workcount

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

const (
	childSchema         = "gts-retry-profile-cert-child/v1"
	modeScanner         = "skip_external_scanner_repeat"
	modeSkipComplete    = "skip_complete_accepted_error"
	modeSkipFresh       = "skip_fresh_complete_accepted_error"
	modeShortLadder     = "short_complete_accepted_error_ladder"
	modeWorkBudget      = "first_pass_work_budget"
	modeReuseCleanWide  = "reuse_clean_wide_for_wide_retry"
	orderBaselineFirst  = "baseline_first"
	orderCandidateFirst = "candidate_first"
)

type childAttempt struct {
	LogicalRung        string                       `json:"logical_rung"`
	OperationCause     string                       `json:"operation_cause"`
	StopReason         gotreesitter.ParseStopReason `json:"stop_reason"`
	RootHasError       bool                         `json:"root_has_error"`
	RootEndByte        uint32                       `json:"root_end_byte"`
	ResolvedMaxStacks  int                          `json:"resolved_max_stacks"`
	ResolvedRetryPass  bool                         `json:"resolved_retry_pass"`
	ResolvedMergeLimit int                          `json:"resolved_max_merge_per_key"`
}

type childParse struct {
	WallNanos   int64                        `json:"wall_nanos"`
	TotalAlloc  uint64                       `json:"total_alloc_bytes"`
	Attempts    []childAttempt               `json:"attempts"`
	TreePresent bool                         `json:"tree_present"`
	StopReason  gotreesitter.ParseStopReason `json:"stop_reason"`
	RootStart   uint32                       `json:"root_start_byte"`
	RootEnd     uint32                       `json:"root_end_byte"`
	HasError    bool                         `json:"root_has_error"`
	DeepSHA256  string                       `json:"deep_tree_sha256"`
}

type childResponse struct {
	Schema            string     `json:"schema"`
	CandidateRevision string     `json:"candidate_revision"`
	BuildModified     bool       `json:"build_modified"`
	Language          string     `json:"language"`
	Mode              string     `json:"mode"`
	ParseOrder        string     `json:"parse_order"`
	BlobSHA256        string     `json:"blob_sha256"`
	SourceSHA256      string     `json:"source_sha256"`
	Baseline          childParse `json:"baseline"`
	Candidate         childParse `json:"candidate"`
}

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	flags := flag.NewFlagSet("retry-profile-cert-child", flag.ContinueOnError)
	language := flags.String("language", "", "language name")
	mode := flags.String("mode", "", "candidate mode")
	order := flags.String("order", "", "parse order")
	sourcePath := flags.String("source", "", "source path")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	if *language == "" || *sourcePath == "" {
		return fmt.Errorf("language and source are required")
	}
	if *order != orderBaselineFirst && *order != orderCandidateFirst {
		return fmt.Errorf("invalid parse order %q", *order)
	}

	source, err := os.ReadFile(*sourcePath)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}
	blob := grammars.BlobByName(*language)
	if len(blob) == 0 {
		return fmt.Errorf("language %q has no grammar blob", *language)
	}
	entry := grammars.DetectLanguageByName(*language)
	if entry == nil || entry.Language == nil {
		return fmt.Errorf("language %q is not registered", *language)
	}
	registered := entry.Language()
	if registered == nil {
		return fmt.Errorf("load registered language %q", *language)
	}
	baselineValue, candidateValue := *registered, *registered
	baseline, candidate := &baselineValue, &candidateValue
	if err := configureLanguages(*mode, baseline, candidate); err != nil {
		return err
	}

	var baselineResult, candidateResult childParse
	if *order == orderCandidateFirst {
		candidateResult, err = parseOne(candidate, source)
		if err == nil {
			baselineResult, err = parseOne(baseline, source)
		}
	} else {
		baselineResult, err = parseOne(baseline, source)
		if err == nil {
			candidateResult, err = parseOne(candidate, source)
		}
	}
	if err != nil {
		return err
	}

	blobSum := sha256.Sum256(blob)
	sourceSum := sha256.Sum256(source)
	revision, modified := buildRevision()
	response := childResponse{
		Schema:            childSchema,
		CandidateRevision: revision,
		BuildModified:     modified,
		Language:          *language,
		Mode:              *mode,
		ParseOrder:        *order,
		BlobSHA256:        hex.EncodeToString(blobSum[:]),
		SourceSHA256:      hex.EncodeToString(sourceSum[:]),
		Baseline:          baselineResult,
		Candidate:         candidateResult,
	}
	if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	return nil
}

func configureLanguages(mode string, baseline, candidate *gotreesitter.Language) error {
	if baseline == nil || candidate == nil || baseline == candidate {
		return fmt.Errorf("baseline and candidate languages must be distinct")
	}
	switch mode {
	case modeWorkBudget:
		baseline.FullParseRetryWorkBudgetEnabled = false
		candidate.FullParseRetryWorkBudgetEnabled = true
	case modeScanner:
		baseline.ExternalScannerFullParseRetryPolicy = gotreesitter.ExternalScannerFullParseRetryDefault
		if candidate.ExternalScannerFullParseRetryPolicy != gotreesitter.ExternalScannerFullParseRetrySkipRepeat {
			return fmt.Errorf("candidate does not suppress the external-scanner repeat")
		}
	case modeSkipComplete:
		profile := baseline.FullParseAcceptedErrorRetryProfile
		profile.SkipCompleteAcceptedErrorRetry = false
		baseline.FullParseAcceptedErrorRetryProfile = profile
		if !candidate.FullParseAcceptedErrorRetryProfile.SkipCompleteAcceptedErrorRetry {
			return fmt.Errorf("candidate does not suppress the complete accepted-error retry")
		}
	case modeSkipFresh:
		profile := baseline.FullParseAcceptedErrorRetryProfile
		profile.SkipFreshCompleteAcceptedErrorRetry = false
		baseline.FullParseAcceptedErrorRetryProfile = profile
		if !candidate.FullParseAcceptedErrorRetryProfile.SkipFreshCompleteAcceptedErrorRetry {
			return fmt.Errorf("candidate does not suppress the fresh complete accepted-error retry")
		}
	case modeShortLadder:
		profile := baseline.FullParseAcceptedErrorRetryProfile
		profile.SkipInitialCompleteAcceptedErrorMergeRetry = false
		profile.ReuseCleanWideForWideRetry = false
		profile.ReuseCleanWideMinSourceBytes = 0
		baseline.FullParseAcceptedErrorRetryProfile = profile
		candidateProfile := candidate.FullParseAcceptedErrorRetryProfile
		if !candidateProfile.SkipInitialCompleteAcceptedErrorMergeRetry ||
			!candidateProfile.ReuseCleanWideForWideRetry || candidateProfile.ReuseCleanWideMinSourceBytes == 0 {
			return fmt.Errorf("candidate does not select the short accepted-error ladder")
		}
	case modeReuseCleanWide:
		baselineProfile := baseline.FullParseAcceptedErrorRetryProfile
		baselineProfile.SkipInitialCompleteAcceptedErrorMergeRetry = false
		baselineProfile.ReuseCleanWideForWideRetry = false
		baselineProfile.ReuseCleanWideMinSourceBytes = 0
		baseline.FullParseAcceptedErrorRetryProfile = baselineProfile
		candidateProfile := candidate.FullParseAcceptedErrorRetryProfile
		if candidateProfile.SkipInitialCompleteAcceptedErrorMergeRetry ||
			!candidateProfile.ReuseCleanWideForWideRetry || candidateProfile.ReuseCleanWideMinSourceBytes == 0 {
			return fmt.Errorf("candidate does not select clean-wide reuse alone")
		}
	default:
		return fmt.Errorf("invalid candidate mode %q", mode)
	}
	return nil
}

func parseOne(language *gotreesitter.Language, source []byte) (childParse, error) {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	gotreesitter.BeginDiagnosticRetryTrace()
	started := time.Now()
	tree, err := gotreesitter.NewParser(language).Parse(source)
	wall := time.Since(started)
	trace := gotreesitter.EndDiagnosticRetryTrace()
	runtime.ReadMemStats(&after)
	if err != nil {
		if tree != nil {
			tree.Release()
		}
		return childParse{}, fmt.Errorf("parse: %w", err)
	}
	result := childParse{
		WallNanos:  wall.Nanoseconds(),
		TotalAlloc: after.TotalAlloc - before.TotalAlloc,
		Attempts:   make([]childAttempt, 0, len(trace.Attempts)),
	}
	for _, attempt := range trace.Attempts {
		result.Attempts = append(result.Attempts, childAttempt{
			LogicalRung:        attempt.LogicalRung,
			OperationCause:     attempt.OperationCause,
			StopReason:         attempt.StopReason,
			RootHasError:       attempt.RootHasError,
			RootEndByte:        attempt.RootEndByte,
			ResolvedMaxStacks:  attempt.ResolvedMaxStacks,
			ResolvedRetryPass:  attempt.ResolvedRetryPass,
			ResolvedMergeLimit: attempt.ResolvedMaxMergePerKey,
		})
	}
	if tree == nil || tree.RootNode() == nil {
		if tree != nil {
			tree.Release()
		}
		if len(result.Attempts) == 0 {
			return childParse{}, fmt.Errorf("parse returned no tree and no attempt receipt")
		}
		last := result.Attempts[len(result.Attempts)-1]
		result.StopReason = last.StopReason
		result.RootEnd = last.RootEndByte
		result.HasError = last.RootHasError
		return result, nil
	}
	defer tree.Release()
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		return childParse{}, fmt.Errorf("deep digest: %w", err)
	}
	runtimeResult := tree.ParseRuntime()
	result.TreePresent = true
	result.StopReason = runtimeResult.StopReason
	result.RootStart = tree.RootNode().StartByte()
	result.RootEnd = tree.RootNode().EndByte()
	result.HasError = tree.RootNode().HasError()
	result.DeepSHA256 = inspection.SHA256
	return result, nil
}

func buildRevision() (string, bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
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
	return revision, modified
}
