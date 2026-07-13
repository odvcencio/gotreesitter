//go:build cgo && treesitter_c_parity && treesitter_c_perfscan

package cgoharness

// This file contains the merge-only fleet reducer for independently produced
// one-language perf-scan scoreboards. It intentionally lives beside the scan
// harness so reduction shares its scoreboard, aggregation, rendering, and
// hard-gate implementation. Measurement and clean reducer revisions are
// authenticated separately so historical shards can be reduced by a later fix.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	perfScanEnvReduceInputs = "GTS_PERF_SCAN_REDUCE_INPUTS"
	perfScanEnvReduceMode   = "GTS_PERF_SCAN_REDUCE_MODE"

	perfScanReduceModeReport  = "report"
	perfScanReduceModeCertify = "certify"
)

// TestPerfScanReduce authenticates and merges one completed scoreboard for
// every language in the corpus lock. It never invokes a parser, so interrupted
// campaigns can resume by retaining their completed shard scoreboards and
// supplying the full set here once the last language finishes.
func TestPerfScanReduce(t *testing.T) {
	if !perfScanGateEnabled() {
		t.Skipf("set %s=1 to run the perf scan reducer", perfScanEnvGate)
	}
	if strings.TrimSpace(os.Getenv(perfScanEnvLang)) != "" {
		t.Fatalf("%s is set; refusing to reduce inside a language child", perfScanEnvLang)
	}
	reduceMode, err := perfScanResolveReduceMode(os.Getenv(perfScanEnvReduceMode))
	if err != nil {
		t.Fatal(err)
	}
	paths, err := perfScanReduceInputPaths(os.Getenv(perfScanEnvReduceInputs))
	if err != nil {
		t.Fatalf("resolve shard scoreboards: %v", err)
	}
	reducerProvenance, err := perfScanRepositoryProvenance(true)
	if err != nil {
		t.Fatalf("authenticate reducer repository provenance: %v", err)
	}

	lockCfg := perfScanConfig{
		CorpusLock:   strings.TrimSpace(os.Getenv(perfScanEnvCorpusLock)),
		HardGate:     true,
		RequireFleet: true,
	}
	lockEntries, coverage, err := perfScanPrepareCorpusLock(&lockCfg)
	if err != nil {
		t.Fatalf("prepare authenticated corpus lock: %v", err)
	}
	if len(coverage.MissingFromRegistry) > 0 {
		t.Fatalf("authenticated lock has registry gaps: %s", strings.Join(coverage.MissingFromRegistry, ","))
	}
	lockLanguages := make([]string, 0, len(lockEntries))
	for lang := range lockEntries {
		lockLanguages = append(lockLanguages, lang)
	}
	sort.Strings(lockLanguages)

	board, err := perfScanReduceScoreboards(paths, coverage.LockPath, coverage.LockSHA256, lockLanguages, time.Now().UTC())
	if err != nil {
		t.Fatalf("reduce shard scoreboards: %v", err)
	}
	if err := perfScanAuthenticateReducer(board, reducerProvenance); err != nil {
		t.Fatalf("authenticate reducer against shards: %v", err)
	}
	outDir := strings.TrimSpace(os.Getenv(perfScanEnvOut))
	if outDir == "" {
		outDir = filepath.Join("perf_scan", "out", "reduced_"+time.Now().UTC().Format("20060102T150405Z"))
	}
	absOut, err := filepath.Abs(outDir)
	if err != nil {
		t.Fatalf("resolve output directory: %v", err)
	}
	if err := perfScanPublishReducedScoreboard(absOut, board, reduceMode); err != nil {
		t.Fatal(err)
	}
	t.Logf("reduced %d authenticated shard scoreboards measured at %s with reducer %s", len(board.Languages), board.GitRevision, board.Reduction.GitRevision)
	t.Logf("reducer mode: %s; hard gate: %s", reduceMode, strings.ToUpper(board.Gate.Status))
	t.Logf("scoreboard: %s", filepath.Join(absOut, "scoreboard.json"))
	t.Logf("scoreboard: %s", filepath.Join(absOut, "scoreboard.md"))
}

func perfScanResolveReduceMode(raw string) (string, error) {
	mode := strings.TrimSpace(raw)
	if mode == "" {
		return perfScanReduceModeCertify, nil
	}
	switch mode {
	case perfScanReduceModeReport, perfScanReduceModeCertify:
		return mode, nil
	default:
		return "", fmt.Errorf("%s must be %q or %q, got %q", perfScanEnvReduceMode, perfScanReduceModeReport, perfScanReduceModeCertify, raw)
	}
}

// perfScanPublishReducedScoreboard publishes the exact same recomputed board
// in both reducer modes. Report mode preserves a failing gate as evidence;
// certify mode publishes that evidence first and then returns a blocking error.
func perfScanPublishReducedScoreboard(absOut string, board *perfScanScoreboard, mode string) error {
	if mode != perfScanReduceModeReport && mode != perfScanReduceModeCertify {
		return fmt.Errorf("invalid resolved reducer mode %q", mode)
	}
	if board == nil || board.Gate == nil {
		return fmt.Errorf("cannot publish reduced scoreboard without a recomputed hard gate")
	}
	if board.Gate.Status != perfScanGatePass && board.Gate.Status != perfScanGateFail {
		return fmt.Errorf("cannot publish reduced scoreboard with hard gate status %q", board.Gate.Status)
	}
	if err := os.MkdirAll(absOut, 0o755); err != nil {
		return fmt.Errorf("create reduced scoreboard output directory: %w", err)
	}
	if err := perfScanWriteScoreboard(absOut, board); err != nil {
		return fmt.Errorf("write reduced scoreboard: %w", err)
	}
	if mode == perfScanReduceModeCertify && board.Gate.Status == perfScanGateFail {
		findings := len(board.Gate.Failures)
		return fmt.Errorf("reduced fleet hard gate failed with %d finding(s); scoreboard: %s", findings, filepath.Join(absOut, "scoreboard.json"))
	}
	return nil
}

func perfScanAuthenticateReducer(board *perfScanScoreboard, provenance perfScanRepositoryState) error {
	if board == nil {
		return fmt.Errorf("nil reduced scoreboard")
	}
	if !provenance.Clean {
		return fmt.Errorf("reducer execution does not attest a clean repository")
	}
	revision, err := perfScanNormalizeRevision(provenance.Revision)
	if err != nil {
		return fmt.Errorf("reducer execution revision: %w", err)
	}
	board.Reduction = &perfScanReductionProvenance{GitRevision: revision, GitClean: true}
	return nil
}

func perfScanReduceInputPaths(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	var paths []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		matches, err := filepath.Glob(part)
		if err != nil {
			return nil, fmt.Errorf("glob %q: %w", part, err)
		}
		if len(matches) == 0 {
			if _, err := os.Stat(part); err != nil {
				return nil, fmt.Errorf("input %q matched no scoreboard: %w", part, err)
			}
			matches = []string{part}
		}
		paths = append(paths, matches...)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("%s must name one or more scoreboard paths or glob patterns", perfScanEnvReduceInputs)
	}
	sort.Strings(paths)
	return paths, nil
}

func perfScanReadScoreboard(path string) (*perfScanScoreboard, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var board perfScanScoreboard
	// Plain Unmarshal deliberately keeps v1's backward-compatible decoding.
	// Authoritative reduction applies its stronger provenance requirements
	// after decoding and therefore rejects old revisionless shards.
	if err := json.Unmarshal(data, &board); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return &board, nil
}

func perfScanReduceScoreboards(paths []string, lockPath, lockSHA string, lockLanguages []string, generatedAt time.Time) (*perfScanScoreboard, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("no shard scoreboards")
	}
	lockSHA = strings.ToLower(strings.TrimSpace(lockSHA))
	if lockSHA == "" {
		return nil, fmt.Errorf("authenticated corpus lock SHA-256 is empty")
	}
	wantLanguages := append([]string(nil), lockLanguages...)
	sort.Strings(wantLanguages)
	if len(wantLanguages) == 0 {
		return nil, fmt.Errorf("authenticated corpus lock has no languages")
	}
	wantSet := make(map[string]bool, len(wantLanguages))
	for _, lang := range wantLanguages {
		if lang == "" || wantSet[lang] {
			return nil, fmt.Errorf("authenticated corpus lock contains duplicate or empty language %q", lang)
		}
		wantSet[lang] = true
	}

	var (
		baseConfig   *perfScanConfig
		baseHost     perfScanHost
		revision     string
		rowsByLang   = make(map[string]*perfScanLanguage, len(wantLanguages))
		generatedMax time.Time
	)
	for _, shardPath := range paths {
		shard, err := perfScanReadScoreboard(shardPath)
		if err != nil {
			return nil, err
		}
		if shard.Schema != perfScanSchema {
			return nil, fmt.Errorf("shard %s schema %q, want %q", shardPath, shard.Schema, perfScanSchema)
		}
		if shard.Reduction != nil {
			return nil, fmt.Errorf("shard %s contains reduction provenance; expected raw one-language measurement evidence", shardPath)
		}
		shardRevision, err := perfScanNormalizeRevision(shard.GitRevision)
		if err != nil {
			return nil, fmt.Errorf("shard %s git revision: %w", shardPath, err)
		}
		if revision == "" {
			revision = shardRevision
		} else if shardRevision != revision {
			return nil, fmt.Errorf("shard %s git revision %s differs from %s", shardPath, shardRevision, revision)
		}
		if !shard.GitClean {
			return nil, fmt.Errorf("shard %s does not attest a clean tracked and untracked repository", shardPath)
		}
		if shard.Config.Contended {
			return nil, fmt.Errorf("shard %s is contended: %s", shardPath, shard.Config.ContendedNote)
		}
		if strings.HasPrefix(shard.Config.ContendedNote, "explicit ") {
			return nil, fmt.Errorf("shard %s used an explicit contention override: %s", shardPath, shard.Config.ContendedNote)
		}
		if len(shard.Config.ExcludePaths) != 0 {
			return nil, fmt.Errorf("shard %s has excluded paths: %s", shardPath, strings.Join(shard.Config.ExcludePaths, ","))
		}
		if !shard.Config.HardGate {
			return nil, fmt.Errorf("shard %s did not run with the hard gate enabled", shardPath)
		}
		if shard.Config.RequireFleet {
			return nil, fmt.Errorf("shard %s has require_fleet=true; one-language shards must set it false", shardPath)
		}
		if shard.Config.CorpusLockSHA256 != lockSHA || shard.Corpus.LockSHA256 != lockSHA {
			return nil, fmt.Errorf("shard %s corpus lock SHA mismatch: config=%q coverage=%q want=%q", shardPath, shard.Config.CorpusLockSHA256, shard.Corpus.LockSHA256, lockSHA)
		}
		if shard.Config.CorpusLockLanguages != len(wantLanguages) || shard.Corpus.LockLanguages != len(wantLanguages) {
			return nil, fmt.Errorf("shard %s corpus lock language count mismatch: config=%d coverage=%d want=%d", shardPath, shard.Config.CorpusLockLanguages, shard.Corpus.LockLanguages, len(wantLanguages))
		}
		if shard.Corpus.SelectedLanguages != 1 || len(shard.Corpus.MissingFromLock) != 0 || len(shard.Corpus.MissingFromRegistry) != 0 {
			return nil, fmt.Errorf("shard %s has incomplete corpus coverage: selected=%d missing_lock=%v missing_registry=%v", shardPath, shard.Corpus.SelectedLanguages, shard.Corpus.MissingFromLock, shard.Corpus.MissingFromRegistry)
		}
		if len(shard.Config.Languages) != 1 || len(shard.Languages) != 1 || shard.Languages[0] == nil {
			return nil, fmt.Errorf("shard %s must contain exactly one configured language and one language row", shardPath)
		}
		lang := shard.Languages[0].Language
		row := shard.Languages[0]
		if shard.Config.Languages[0] != lang {
			return nil, fmt.Errorf("shard %s configured language %q does not match row %q", shardPath, shard.Config.Languages[0], lang)
		}
		if !wantSet[lang] {
			return nil, fmt.Errorf("shard %s language %q is not in the authenticated corpus lock", shardPath, lang)
		}
		if _, exists := rowsByLang[lang]; exists {
			return nil, fmt.Errorf("duplicate shard language %q", lang)
		}
		seenPaths := map[string]bool{}
		for _, file := range row.Files {
			if file == nil || file.Classification == nil {
				return nil, fmt.Errorf("shard %s language %q has incomplete full-parse classification coverage", shardPath, lang)
			}
			if file.Path == "" || seenPaths[file.Path] {
				return nil, fmt.Errorf("shard %s language %q has empty or duplicate file path %q", shardPath, lang, file.Path)
			}
			seenPaths[file.Path] = true
			if err := perfScanValidateClassification(file.Classification); err != nil {
				return nil, fmt.Errorf("shard %s language %q file %q classification: %w", shardPath, lang, file.Path, err)
			}
		}

		comparable := perfScanComparableShardConfig(shard.Config)
		if baseConfig == nil {
			copyConfig := comparable
			baseConfig = &copyConfig
			baseHost = shard.Host
		} else if !reflect.DeepEqual(*baseConfig, comparable) {
			return nil, fmt.Errorf("shard %s measurement config differs from earlier shards (only languages and require_fleet may differ)", shardPath)
		}
		if err := perfScanValidateHostIdentity(shard.Host); err != nil {
			return nil, fmt.Errorf("shard %s host identity: %w", shardPath, err)
		}
		if !reflect.DeepEqual(perfScanComparableHost(baseHost), perfScanComparableHost(shard.Host)) {
			return nil, fmt.Errorf("shard %s host identity differs from earlier shards", shardPath)
		}

		recomputedGate := perfScanCanonicalizeGateReport(perfScanEvaluateHardGate(shard))
		if shard.Gate == nil {
			return nil, fmt.Errorf("shard %s has no stored hard gate", shardPath)
		}
		if shard.Gate.Status != perfScanGatePass && shard.Gate.Status != perfScanGateFail {
			return nil, fmt.Errorf("shard %s stored hard gate has unknown status %q", shardPath, shard.Gate.Status)
		}
		storedGate := perfScanCanonicalizeGateReport(*shard.Gate)
		if !reflect.DeepEqual(storedGate, recomputedGate) {
			return nil, fmt.Errorf("shard %s stored hard gate is stale or contradicts its file evidence", shardPath)
		}
		rowsByLang[lang] = row
		if ts, err := time.Parse(time.RFC3339, shard.GeneratedAt); err == nil && ts.After(generatedMax) {
			generatedMax = ts
		}
	}

	var missing []string
	for _, lang := range wantLanguages {
		if rowsByLang[lang] == nil {
			missing = append(missing, lang)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing shard languages: %s", strings.Join(missing, ","))
	}
	if len(rowsByLang) != len(wantLanguages) {
		return nil, fmt.Errorf("reduced %d unique languages, authenticated lock contains %d", len(rowsByLang), len(wantLanguages))
	}

	outputConfig := *baseConfig
	outputConfig.Languages = append([]string(nil), wantLanguages...)
	outputConfig.RequireFleet = true
	outputConfig.HardGate = true
	outputConfig.ExcludePaths = nil
	outputConfig.CorpusLock = lockPath
	outputConfig.CorpusLockSHA256 = lockSHA
	outputConfig.CorpusLockLanguages = len(wantLanguages)
	board := &perfScanScoreboard{
		Schema:      perfScanSchema,
		GeneratedAt: generatedAt.UTC().Format(time.RFC3339),
		GitRevision: revision,
		GitClean:    true,
		Host:        baseHost,
		Config:      outputConfig,
		Summary:     map[string]int{},
		Corpus: perfScanCorpusCoverage{
			LockPath:          lockPath,
			LockSHA256:        lockSHA,
			LockLanguages:     len(wantLanguages),
			SelectedLanguages: len(wantLanguages),
		},
		Notes: []string{fmt.Sprintf("Reduced from %d authenticated one-language shard scoreboards without rerunning parses.", len(paths))},
	}
	if !generatedMax.IsZero() {
		board.Notes = append(board.Notes, "Latest source shard generated at "+generatedMax.UTC().Format(time.RFC3339)+".")
	}
	for _, lang := range wantLanguages {
		row := rowsByLang[lang]
		perfScanRefreshLanguageAggregates(row, outputConfig)
		board.Languages = append(board.Languages, row)
		board.Summary[row.Verdict]++
	}
	board.FullParseSplit = perfScanAggregateFleetCleanErrorSplit(board.Languages)
	gate := perfScanEvaluateHardGate(board)
	board.Gate = &gate
	return board, nil
}

func perfScanComparableShardConfig(cfg perfScanConfig) perfScanConfig {
	cfg.Languages = nil
	cfg.RequireFleet = false
	return cfg
}

func perfScanComparableHost(host perfScanHost) perfScanHost {
	host.LoadavgStart = ""
	host.LoadavgEnd = ""
	return host
}

func perfScanValidateHostIdentity(host perfScanHost) error {
	if strings.TrimSpace(host.Hostname) == "" || strings.TrimSpace(host.GOOS) == "" || strings.TrimSpace(host.GOARCH) == "" || strings.TrimSpace(host.GoVersion) == "" {
		return fmt.Errorf("hostname, GOOS, GOARCH, and Go version must be recorded")
	}
	if host.NumCPU <= 0 || host.GOMAXPROCS <= 0 {
		return fmt.Errorf("num_cpu=%d and gomaxprocs=%d must both be positive", host.NumCPU, host.GOMAXPROCS)
	}
	threshold := float64(host.NumCPU) / 4
	if threshold < 2 {
		threshold = 2
	}
	for label, raw := range map[string]string{"start": host.LoadavgStart, "end": host.LoadavgEnd} {
		fields := strings.Fields(raw)
		if len(fields) == 0 {
			return fmt.Errorf("loadavg %s must be recorded", label)
		}
		load1, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			return fmt.Errorf("loadavg %s %q is invalid: %w", label, raw, err)
		}
		if load1 >= threshold {
			return fmt.Errorf("loadavg %s %.2f meets contention threshold %.2f", label, load1, threshold)
		}
	}
	return nil
}

func perfScanValidateClassification(classification *perfScanFileClassification) error {
	if classification == nil {
		return fmt.Errorf("missing classification")
	}
	if strings.TrimSpace(classification.Reason) == "" || strings.TrimSpace(classification.GoStatus) == "" {
		return fmt.Errorf("reason and go_status must be recorded")
	}
	switch classification.Class {
	case perfScanClassClean:
		if classification.GoStatus != perfScanStatusOK || !classification.FullSpan || classification.RootHasError || classification.StoppedEarly || classification.StopReason != "" {
			return fmt.Errorf("clean class contradicts status/span/error/stop facts")
		}
	case perfScanClassError:
		if classification.StoppedEarly || classification.StopReason != "" {
			return fmt.Errorf("error class contradicts stopped facts")
		}
		if perfScanClassificationStatusIsStopped(classification.GoStatus) {
			return fmt.Errorf("error class uses stopped go_status %q", classification.GoStatus)
		}
		if classification.GoStatus == perfScanStatusOK && classification.FullSpan && !classification.RootHasError {
			return fmt.Errorf("error class has all clean acceptance facts")
		}
	case perfScanClassStopped:
		if !classification.StoppedEarly || classification.StopReason == "" || !perfScanClassificationStatusIsStopped(classification.GoStatus) {
			return fmt.Errorf("stopped class lacks stopped status and reason facts")
		}
	default:
		return fmt.Errorf("unknown class %q", classification.Class)
	}
	return nil
}

func perfScanClassificationStatusIsStopped(status string) bool {
	return status == "go_timeout" || status == "go_budget_stop" || status == "go_stopped"
}

func perfScanRefreshLanguageAggregates(row *perfScanLanguage, cfg perfScanConfig) {
	row.FilesMeasured = len(row.Files)
	row.BytesMeasured = 0
	for _, file := range row.Files {
		if file == nil {
			continue
		}
		row.BytesMeasured += int64(file.Bytes)
		for _, result := range file.Axes {
			if result == nil {
				continue
			}
			result.Ratio = 0
			result.RatioIsLowerBound = false
			result.Verdict = perfScanBucketNoData
			switch {
			case result.Status == perfScanStatusOK && result.GoMedianNs > 0 && result.CMedianNs > 0:
				result.Ratio = float64(result.GoMedianNs) / float64(result.CMedianNs)
				result.Verdict = perfScanVerdictBucket(result.Ratio)
			case result.Status == "go_timeout" && result.CMedianNs > 0:
				result.Ratio = float64(time.Duration(cfg.FileBudgetMS)*time.Millisecond) / float64(result.CMedianNs)
				result.RatioIsLowerBound = true
				result.Verdict = perfScanVerdictBucket(result.Ratio)
			}
		}
	}
	row.Axes = map[string]*perfScanLangAxis{}
	row.FullParseSplit = nil
	perfScanAggregateLanguage(row, cfg)
}

func perfScanAggregateFleetCleanErrorSplit(rows []*perfScanLanguage) *perfScanCleanErrorSplit {
	var files []*perfScanFile
	for _, row := range rows {
		if row != nil {
			files = append(files, row.Files...)
		}
	}
	if len(files) == 0 {
		return nil
	}
	synthetic := &perfScanLanguage{Files: files}
	perfScanAggregateCleanErrorSplit(synthetic)
	return synthetic.FullParseSplit
}

func TestPerfScanReduceScoreboards(t *testing.T) {
	const (
		lockPath = "/corpus/corpus_sources.lock"
		lockSHA  = "cf108b005fe41c4513bae14eafd4f4ec72e64454ca2eb6bbf4d18d25caab24f2"
		revision = "7b797554397b814f981e9a3fb462b84a3d0f1837"
	)
	lockLanguages := []string{"go", "python"}
	makeInputs := func(t *testing.T, mutate func(goShard, pythonShard *perfScanScoreboard)) []string {
		t.Helper()
		goShard := perfScanTestShard("go", lockPath, lockSHA, len(lockLanguages), revision)
		pythonShard := perfScanTestShard("python", lockPath, lockSHA, len(lockLanguages), revision)
		if mutate != nil {
			mutate(goShard, pythonShard)
		}
		dir := t.TempDir()
		return []string{
			perfScanWriteTestScoreboard(t, dir, "go.json", goShard),
			perfScanWriteTestScoreboard(t, dir, "python.json", pythonShard),
		}
	}
	reduce := func(paths []string) (*perfScanScoreboard, error) {
		return perfScanReduceScoreboards(paths, lockPath, lockSHA, lockLanguages, time.Unix(1_752_400_000, 0))
	}

	t.Run("happy merge recomputes authoritative fleet", func(t *testing.T) {
		board, err := reduce(makeInputs(t, func(goShard, _ *perfScanScoreboard) {
			goShard.Summary = map[string]int{perfScanBucketNoData: 99}
			goShard.Languages[0].Axes[perfScanAxisFull].RatioByTotal = 99
			goShard.Languages[0].Files[0].Axes[perfScanAxisFull].Ratio = 99
			goShard.FullParseSplit = nil
		}))
		if err != nil {
			t.Fatalf("reduce: %v", err)
		}
		if board.GitRevision != revision || board.Schema != perfScanSchema {
			t.Fatalf("provenance = schema=%q revision=%q", board.Schema, board.GitRevision)
		}
		if !board.Config.RequireFleet || !board.Config.HardGate || !reflect.DeepEqual(board.Config.Languages, lockLanguages) {
			t.Fatalf("fleet config = %+v", board.Config)
		}
		if board.Corpus.SelectedLanguages != len(lockLanguages) || board.Corpus.LockLanguages != len(lockLanguages) {
			t.Fatalf("coverage = %+v", board.Corpus)
		}
		if len(board.Languages) != 2 || board.Languages[0].Language != "go" || board.Languages[1].Language != "python" {
			t.Fatalf("languages = %+v", board.Languages)
		}
		if board.Summary[perfScanBucketLe12] != 2 {
			t.Fatalf("summary = %+v", board.Summary)
		}
		if board.Languages[0].Axes[perfScanAxisFull].RatioByTotal != 1 || board.Languages[0].Files[0].Axes[perfScanAxisFull].Ratio != 1 {
			t.Fatalf("language aggregates were not recomputed: %+v", board.Languages[0])
		}
		if board.FullParseSplit == nil || board.FullParseSplit.ClassifiedFiles != 2 || board.FullParseSplit.Clean.Files != 2 || board.FullParseSplit.Clean.RatioByTotal != 1 {
			t.Fatalf("fleet split = %+v", board.FullParseSplit)
		}
		if board.Gate == nil || board.Gate.Status != perfScanGatePass || board.Gate.FullFilesEvaluated != 2 {
			t.Fatalf("gate = %+v", board.Gate)
		}
		if board.Reduction != nil {
			t.Fatalf("pure reduction unexpectedly recorded execution provenance: %+v", board.Reduction)
		}
		markdown := perfScanRenderMarkdown(board)
		for _, want := range []string{"git revision", "Fleet full-parse clean/error split", "2/2"} {
			if !strings.Contains(markdown, want) {
				t.Fatalf("markdown missing %q:\n%s", want, markdown)
			}
		}
	})

	t.Run("gate evidence is independent of axis insertion order", func(t *testing.T) {
		setStoppedAxes := func(shard *perfScanScoreboard, order []string) {
			file := shard.Languages[0].Files[0]
			file.Classification = &perfScanFileClassification{
				Class:        perfScanClassStopped,
				Reason:       "Go parser stopped before accepting the full source",
				GoStatus:     "go_timeout",
				StoppedEarly: true,
				StopReason:   perfScanStopParserTimeout,
			}
			file.Axes = make(map[string]*perfScanFileAxis, len(order))
			for _, axis := range order {
				file.Axes[axis] = &perfScanFileAxis{
					Status: "go_timeout",
					Detail: "parser timeout on " + axis,
					Stop: &perfScanStop{
						Class:          perfScanStopParserTimeout,
						Reason:         perfScanStopParserTimeout,
						Implementation: "go",
						Phase:          "timed",
					},
				}
			}
		}

		forward := perfScanTestShard("python", lockPath, lockSHA, len(lockLanguages), revision)
		reverse := perfScanTestShard("python", lockPath, lockSHA, len(lockLanguages), revision)
		setStoppedAxes(forward, []string{perfScanAxisFull, perfScanAxisNoEdit})
		setStoppedAxes(reverse, []string{perfScanAxisNoEdit, perfScanAxisFull})
		forwardGate := perfScanEvaluateHardGate(forward)
		reverseGate := perfScanEvaluateHardGate(reverse)
		if !reflect.DeepEqual(forwardGate, reverseGate) {
			t.Fatalf("opposite axis insertion orders differ:\nforward=%+v\nreverse=%+v", forwardGate, reverseGate)
		}

		makeFastBoard := func(order []string) *perfScanScoreboard {
			board := &perfScanScoreboard{}
			for _, lang := range order {
				row := perfScanTestShard(lang, lockPath, lockSHA, len(lockLanguages), revision).Languages[0]
				full := row.Files[0].Axes[perfScanAxisFull]
				full.GoMedianNs = 100
				full.CMedianNs = 1_000
				board.Languages = append(board.Languages, row)
			}
			return board
		}
		forwardFast := perfScanEvaluateHardGate(makeFastBoard([]string{"go", "python"}))
		reverseFast := perfScanEvaluateHardGate(makeFastBoard([]string{"python", "go"}))
		if !reflect.DeepEqual(forwardFast, reverseFast) {
			t.Fatalf("opposite language orders differ:\nforward=%+v\nreverse=%+v", forwardFast, reverseFast)
		}
		if got := []string{forwardFast.FastFullFiles[0].Language, forwardFast.FastFullFiles[1].Language}; !reflect.DeepEqual(got, lockLanguages) {
			t.Fatalf("canonical fast languages = %v, want %v", got, lockLanguages)
		}

		paths := makeInputs(t, func(_ *perfScanScoreboard, pythonShard *perfScanScoreboard) {
			setStoppedAxes(pythonShard, []string{perfScanAxisNoEdit, perfScanAxisFull})
			gate := perfScanEvaluateHardGate(pythonShard)
			for left, right := 0, len(gate.Failures)-1; left < right; left, right = left+1, right-1 {
				gate.Failures[left], gate.Failures[right] = gate.Failures[right], gate.Failures[left]
			}
			pythonShard.Gate = &gate
		})
		board, err := reduce(paths)
		if err != nil {
			t.Fatalf("reduce semantically identical gate evidence: %v", err)
		}
		var axes []string
		for _, finding := range board.Gate.Failures {
			if finding.Language == "python" && finding.Kind == "go_stop" {
				axes = append(axes, finding.Axis)
			}
		}
		if want := []string{perfScanAxisFull, perfScanAxisNoEdit}; !reflect.DeepEqual(axes, want) {
			t.Fatalf("canonical stopped axes = %v, want %v", axes, want)
		}
	})

	for _, tc := range []struct {
		name     string
		mutate   func(*perfScanScoreboard)
		wantKind string
	}{
		{
			name: "ratio cliff remains authenticated evidence",
			mutate: func(shard *perfScanScoreboard) {
				shard.Languages[0].Files[0].Axes[perfScanAxisFull].GoMedianNs = 11_001
			},
			wantKind: "ratio",
		},
		{
			name: "parser stop remains authenticated evidence",
			mutate: func(shard *perfScanScoreboard) {
				file := shard.Languages[0].Files[0]
				file.Classification = &perfScanFileClassification{
					Class:        perfScanClassStopped,
					Reason:       "Go parser stopped before accepting the full source",
					GoStatus:     "go_timeout",
					StoppedEarly: true,
					StopReason:   perfScanStopParserTimeout,
				}
				file.Axes[perfScanAxisFull] = &perfScanFileAxis{
					Status: "go_timeout",
					Detail: "parser timeout",
					Stop: &perfScanStop{
						Class:          perfScanStopParserTimeout,
						Reason:         perfScanStopParserTimeout,
						Implementation: "go",
						Phase:          "timed",
					},
				}
			},
			wantKind: "go_stop",
		},
		{
			name: "missing ratio remains authenticated evidence",
			mutate: func(shard *perfScanScoreboard) {
				shard.Languages[0].Files[0].Axes[perfScanAxisFull].GoMedianNs = 0
			},
			wantKind: "coverage",
		},
		{
			name: "partial coverage remains authenticated evidence",
			mutate: func(shard *perfScanScoreboard) {
				shard.Languages[0].FilesSelected = 2
			},
			wantKind: "coverage",
		},
		{
			name: "zero coverage remains authenticated evidence",
			mutate: func(shard *perfScanScoreboard) {
				row := shard.Languages[0]
				row.FilesSelected = 0
				row.FilesMeasured = 0
				row.BytesMeasured = 0
				row.Files = nil
			},
			wantKind: "coverage",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			paths := makeInputs(t, func(_ *perfScanScoreboard, pythonShard *perfScanScoreboard) {
				tc.mutate(pythonShard)
				gate := perfScanEvaluateHardGate(pythonShard)
				pythonShard.Gate = &gate
			})
			board, err := reduce(paths)
			if err != nil {
				t.Fatalf("reduce valid failing evidence: %v", err)
			}
			if board.Gate == nil || board.Gate.Status != perfScanGateFail {
				t.Fatalf("combined gate = %+v, want FAIL", board.Gate)
			}
			found := false
			for _, finding := range board.Gate.Failures {
				if finding.Language == "python" && finding.Kind == tc.wantKind {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("combined gate did not retain %s finding: %+v", tc.wantKind, board.Gate)
			}
		})
	}

	tests := []struct {
		name   string
		paths  func(*testing.T) []string
		mutate func(*perfScanScoreboard, *perfScanScoreboard)
		want   string
	}{
		{
			name: "missing language",
			paths: func(t *testing.T) []string {
				return makeInputs(t, nil)[:1]
			},
			want: "missing shard languages: python",
		},
		{
			name: "duplicate language",
			mutate: func(_, pythonShard *perfScanScoreboard) {
				pythonShard.Config.Languages = []string{"go"}
				pythonShard.Languages[0].Language = "go"
			},
			want: "duplicate shard language",
		},
		{
			name: "schema mismatch",
			mutate: func(_, pythonShard *perfScanScoreboard) {
				pythonShard.Schema = "gts-perf-scan/v0"
			},
			want: "schema",
		},
		{
			name: "chained reduction provenance",
			mutate: func(_, pythonShard *perfScanScoreboard) {
				pythonShard.Reduction = &perfScanReductionProvenance{
					GitRevision: strings.Repeat("b", 40),
					GitClean:    true,
				}
			},
			want: "contains reduction provenance",
		},
		{
			name: "config mismatch",
			mutate: func(_, pythonShard *perfScanScoreboard) {
				pythonShard.Config.Reps++
			},
			want: "measurement config differs",
		},
		{
			name: "corpus lock SHA mismatch",
			mutate: func(_, pythonShard *perfScanScoreboard) {
				pythonShard.Config.CorpusLockSHA256 = strings.Repeat("0", 64)
			},
			want: "corpus lock SHA mismatch",
		},
		{
			name: "excluded paths",
			mutate: func(_, pythonShard *perfScanScoreboard) {
				pythonShard.Config.ExcludePaths = []string{"generated/**"}
			},
			want: "excluded paths",
		},
		{
			name: "contended shard",
			mutate: func(_, pythonShard *perfScanScoreboard) {
				pythonShard.Config.Contended = true
				pythonShard.Config.ContendedNote = "loadavg 8"
			},
			want: "is contended",
		},
		{
			name: "forced contention override",
			mutate: func(_, pythonShard *perfScanScoreboard) {
				pythonShard.Config.ContendedNote = "explicit GTS_PERF_SCAN_CONTENDED=0"
			},
			want: "contention override",
		},
		{
			name: "contended end load",
			mutate: func(_, pythonShard *perfScanScoreboard) {
				pythonShard.Host.LoadavgEnd = "8.0 1.0 1.0"
			},
			want: "contention threshold",
		},
		{
			name: "dirty repository",
			mutate: func(_, pythonShard *perfScanScoreboard) {
				pythonShard.GitClean = false
			},
			want: "does not attest a clean",
		},
		{
			name: "mixed host identity",
			mutate: func(_, pythonShard *perfScanScoreboard) {
				pythonShard.Host.Hostname = "another-box"
			},
			want: "host identity differs",
		},
		{
			name: "mixed revisions",
			mutate: func(_, pythonShard *perfScanScoreboard) {
				pythonShard.GitRevision = strings.Repeat("a", 40)
			},
			want: "differs from",
		},
		{
			name: "absent revision",
			mutate: func(_, pythonShard *perfScanScoreboard) {
				pythonShard.GitRevision = ""
			},
			want: "git revision",
		},
		{
			name: "incomplete coverage",
			mutate: func(_, pythonShard *perfScanScoreboard) {
				pythonShard.Corpus.SelectedLanguages = 0
			},
			want: "incomplete corpus coverage",
		},
		{
			name: "incomplete classification coverage",
			mutate: func(_, pythonShard *perfScanScoreboard) {
				pythonShard.Languages[0].Files[0].Classification = nil
			},
			want: "incomplete full-parse classification coverage",
		},
		{
			name: "duplicate file path",
			mutate: func(_, pythonShard *perfScanScoreboard) {
				row := pythonShard.Languages[0]
				row.Files = append(row.Files, row.Files[0])
				row.FilesSelected = 2
				row.FilesMeasured = 2
			},
			want: "duplicate file path",
		},
		{
			name: "contradictory clean classification",
			mutate: func(_, pythonShard *perfScanScoreboard) {
				pythonShard.Languages[0].Files[0].Classification.RootHasError = true
			},
			want: "clean class contradicts",
		},
		{
			name: "contradictory error classification",
			mutate: func(_, pythonShard *perfScanScoreboard) {
				pythonShard.Languages[0].Files[0].Classification.Class = perfScanClassError
			},
			want: "error class has all clean acceptance facts",
		},
		{
			name: "contradictory stopped classification",
			mutate: func(_, pythonShard *perfScanScoreboard) {
				classification := pythonShard.Languages[0].Files[0].Classification
				classification.Class = perfScanClassStopped
				classification.GoStatus = "go_timeout"
			},
			want: "stopped class lacks stopped status and reason facts",
		},
		{
			name: "unknown classification",
			mutate: func(_, pythonShard *perfScanScoreboard) {
				pythonShard.Languages[0].Files[0].Classification.Class = "mystery"
			},
			want: "unknown class",
		},
		{
			name: "missing shard hard gate",
			mutate: func(_, pythonShard *perfScanScoreboard) {
				pythonShard.Gate = nil
			},
			want: "no stored hard gate",
		},
		{
			name: "stale stored pass for failing evidence",
			mutate: func(_, pythonShard *perfScanScoreboard) {
				pythonShard.Languages[0].Files[0].Axes[perfScanAxisFull].GoMedianNs = 11_001
			},
			want: "stale or contradicts",
		},
		{
			name: "stale stored fail for passing evidence",
			mutate: func(_, pythonShard *perfScanScoreboard) {
				full := pythonShard.Languages[0].Files[0].Axes[perfScanAxisFull]
				full.GoMedianNs = 11_001
				gate := perfScanEvaluateHardGate(pythonShard)
				pythonShard.Gate = &gate
				full.GoMedianNs = 1_000
			},
			want: "stale or contradicts",
		},
		{
			name: "unknown stored hard gate status",
			mutate: func(_, pythonShard *perfScanScoreboard) {
				pythonShard.Gate.Status = "unknown"
			},
			want: "unknown status",
		},
		{
			name: "multiple language rows",
			mutate: func(_, pythonShard *perfScanScoreboard) {
				pythonShard.Languages = append(pythonShard.Languages, perfScanTestShard("go", lockPath, lockSHA, len(lockLanguages), revision).Languages[0])
			},
			want: "exactly one configured language and one language row",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var paths []string
			if tc.paths != nil {
				paths = tc.paths(t)
			} else {
				paths = makeInputs(t, tc.mutate)
			}
			if _, err := reduce(paths); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("reduce error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestPerfScanRefreshLanguageAggregatesClearsStaleAxes(t *testing.T) {
	const revision = "7b797554397b814f981e9a3fb462b84a3d0f1837"
	board := perfScanTestShard("go", "/corpus.lock", strings.Repeat("a", 64), 1, revision)
	row := board.Languages[0]
	row.Files[0].Axes[perfScanAxisNoEdit] = &perfScanFileAxis{
		Status:            "c_timeout",
		CMedianNs:         1_000,
		Ratio:             999,
		RatioIsLowerBound: true,
		Verdict:           perfScanBucketCliff,
	}
	row.Files[0].Axes[perfScanAxisEdit] = &perfScanFileAxis{
		Status:            perfScanStatusOK,
		CMedianNs:         1_000,
		Ratio:             999,
		RatioIsLowerBound: true,
		Verdict:           perfScanBucketCliff,
	}
	row.Files[0].Axes["timeout_probe"] = &perfScanFileAxis{
		Status:            "go_timeout",
		CMedianNs:         int64(time.Second),
		Ratio:             999,
		RatioIsLowerBound: false,
		Verdict:           perfScanBucketCliff,
	}
	cfg := board.Config
	cfg.FileBudgetMS = 1_000
	cfg.Axes = []string{perfScanAxisFull, perfScanAxisNoEdit, perfScanAxisEdit, "timeout_probe"}
	perfScanRefreshLanguageAggregates(row, cfg)

	for _, axis := range []string{perfScanAxisNoEdit, perfScanAxisEdit} {
		result := row.Files[0].Axes[axis]
		if result.Ratio != 0 || result.RatioIsLowerBound || result.Verdict != perfScanBucketNoData {
			t.Fatalf("axis %s retained stale derived fields: %+v", axis, result)
		}
	}
	timeout := row.Files[0].Axes["timeout_probe"]
	if timeout.Ratio != 1 || !timeout.RatioIsLowerBound || timeout.Verdict != perfScanBucketLe12 {
		t.Fatalf("timeout lower bound was not recomputed: %+v", timeout)
	}
}

func TestPerfScanRepositoryProvenancePolicy(t *testing.T) {
	revA := strings.Repeat("a", 40)
	revB := strings.Repeat("b", 40)
	cleanGit := perfScanVCSCandidate{Revision: revA, Known: true, CleanKnown: true}
	for _, tc := range []struct {
		name          string
		git           perfScanVCSCandidate
		build         perfScanVCSCandidate
		override      string
		assertedClean bool
		wantErr       string
	}{
		{name: "dirty git", git: perfScanVCSCandidate{Revision: revA, Known: true, CleanKnown: true, Modified: true}, wantErr: "dirty or modified"},
		{name: "modified build", build: perfScanVCSCandidate{Revision: revA, Known: true, CleanKnown: true, Modified: true}, wantErr: "dirty or modified"},
		{name: "build cleanliness unknown", build: perfScanVCSCandidate{Revision: revA, Known: true}, wantErr: perfScanEnvGitClean},
		{name: "git build mismatch", git: cleanGit, build: perfScanVCSCandidate{Revision: revB, Known: true, CleanKnown: true}, wantErr: "revisions disagree"},
		{name: "override cannot replace git", git: cleanGit, override: revB, wantErr: "cannot supersede"},
		{name: "metadata poor needs clean assertion", override: revA, wantErr: perfScanEnvGitClean},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := perfScanSelectRepositoryProvenance(tc.git, tc.build, tc.override, tc.assertedClean, true)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("provenance error = %v, want %q", err, tc.wantErr)
			}
		})
	}
	state, err := perfScanSelectRepositoryProvenance(perfScanVCSCandidate{}, perfScanVCSCandidate{}, revA, true, true)
	if err != nil || state.Revision != revA || !state.Clean {
		t.Fatalf("explicit metadata-poor provenance = %+v, %v", state, err)
	}
	dirty, err := perfScanSelectRepositoryProvenance(
		perfScanVCSCandidate{Revision: revA, Known: true, CleanKnown: true, Modified: true},
		perfScanVCSCandidate{}, "", false, false,
	)
	if err != nil || dirty.Revision != revA || dirty.Clean {
		t.Fatalf("dirty exploratory provenance = %+v, %v", dirty, err)
	}
}

func TestPerfScanAuthenticateReducer(t *testing.T) {
	measurementRevision := strings.Repeat("a", 40)
	reducerRevision := strings.Repeat("b", 40)
	board := &perfScanScoreboard{GitRevision: measurementRevision, GitClean: true}
	if err := perfScanAuthenticateReducer(board, perfScanRepositoryState{Revision: reducerRevision, Clean: true}); err != nil {
		t.Fatalf("different clean reducer provenance: %v", err)
	}
	if board.GitRevision != measurementRevision || !board.GitClean {
		t.Fatalf("measurement provenance changed: %+v", board)
	}
	if board.Reduction == nil || board.Reduction.GitRevision != reducerRevision || !board.Reduction.GitClean {
		t.Fatalf("recorded reducer provenance = %+v", board.Reduction)
	}
	markdown := perfScanRenderMarkdown(board)
	for _, want := range []string{
		"measurement git revision: `" + measurementRevision + "`",
		"reducer git revision: `" + reducerRevision + "`",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("reduced markdown missing %q:\n%s", want, markdown)
		}
	}

	board = &perfScanScoreboard{GitRevision: measurementRevision, GitClean: true}
	if err := perfScanAuthenticateReducer(board, perfScanRepositoryState{Revision: measurementRevision, Clean: true}); err != nil {
		t.Fatalf("matching reducer provenance: %v", err)
	}
	if board.Reduction == nil || board.Reduction.GitRevision != measurementRevision || !board.Reduction.GitClean {
		t.Fatalf("matching recorded reducer provenance = %+v", board.Reduction)
	}

	board = &perfScanScoreboard{GitRevision: measurementRevision, GitClean: true}
	if err := perfScanAuthenticateReducer(board, perfScanRepositoryState{Revision: reducerRevision, Clean: false}); err == nil || !strings.Contains(err.Error(), "clean repository") {
		t.Fatalf("dirty reducer provenance error = %v", err)
	}
	if board.Reduction != nil {
		t.Fatalf("dirty reducer recorded provenance: %+v", board.Reduction)
	}

	if err := perfScanAuthenticateReducer(board, perfScanRepositoryState{Revision: "not-a-revision", Clean: true}); err == nil || !strings.Contains(err.Error(), "reducer execution revision") {
		t.Fatalf("invalid reducer provenance error = %v", err)
	}
	if board.Reduction != nil {
		t.Fatalf("invalid reducer recorded provenance: %+v", board.Reduction)
	}
}

func TestPerfScanReduceModes(t *testing.T) {
	revision := strings.Repeat("a", 40)
	passBoard := perfScanTestShard("go", "/corpus.lock", strings.Repeat("b", 64), 1, revision)
	failBoard := perfScanTestShard("go", "/corpus.lock", strings.Repeat("b", 64), 1, revision)
	failBoard.Languages[0].Files[0].Axes[perfScanAxisFull].GoMedianNs = 11_001
	perfScanRefreshLanguageAggregates(failBoard.Languages[0], failBoard.Config)
	failBoard.Summary = map[string]int{failBoard.Languages[0].Verdict: 1}
	failBoard.FullParseSplit = perfScanAggregateFleetCleanErrorSplit(failBoard.Languages)
	failGate := perfScanEvaluateHardGate(failBoard)
	failBoard.Gate = &failGate

	t.Run("default is certify", func(t *testing.T) {
		mode, err := perfScanResolveReduceMode("")
		if err != nil || mode != perfScanReduceModeCertify {
			t.Fatalf("default reduce mode = %q, %v", mode, err)
		}
		out := filepath.Join(t.TempDir(), "default-certify")
		err = perfScanPublishReducedScoreboard(out, failBoard, mode)
		assertPerfScanCertificationFailure(t, out, err, 1)
	})

	t.Run("FAIL artifacts are mode-equivalent while exit behavior differs", func(t *testing.T) {
		reportOut := filepath.Join(t.TempDir(), "report")
		certifyOut := filepath.Join(t.TempDir(), "certify")
		if err := perfScanPublishReducedScoreboard(reportOut, failBoard, perfScanReduceModeReport); err != nil {
			t.Fatalf("report publication: %v", err)
		}
		assertPerfScanPublishedGate(t, reportOut, perfScanGateFail)
		err := perfScanPublishReducedScoreboard(certifyOut, failBoard, perfScanReduceModeCertify)
		assertPerfScanCertificationFailure(t, certifyOut, err, 1)
		assertPerfScanPublishedArtifactsEqual(t, reportOut, certifyOut)
	})

	t.Run("PASS artifacts are mode-equivalent", func(t *testing.T) {
		reportOut := filepath.Join(t.TempDir(), "report")
		certifyOut := filepath.Join(t.TempDir(), "certify")
		if err := perfScanPublishReducedScoreboard(reportOut, passBoard, perfScanReduceModeReport); err != nil {
			t.Fatalf("report PASS publication: %v", err)
		}
		if err := perfScanPublishReducedScoreboard(certifyOut, passBoard, perfScanReduceModeCertify); err != nil {
			t.Fatalf("certify PASS publication: %v", err)
		}
		assertPerfScanPublishedArtifactsEqual(t, reportOut, certifyOut)
	})

	t.Run("unknown mode fails before output", func(t *testing.T) {
		for _, raw := range []string{"observe", "REPORT"} {
			if _, err := perfScanResolveReduceMode(raw); err == nil || !strings.Contains(err.Error(), perfScanEnvReduceMode) {
				t.Fatalf("unknown reduce mode %q error = %v", raw, err)
			}
		}
		out := filepath.Join(t.TempDir(), "unknown")
		if err := perfScanPublishReducedScoreboard(out, passBoard, "observe"); err == nil {
			t.Fatal("unknown reduce mode unexpectedly published")
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatalf("unknown reduce mode created output before failing: %v", err)
		}
	})
}

func assertPerfScanCertificationFailure(t *testing.T, out string, err error, findings int) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("%d finding(s)", findings)) || !strings.Contains(err.Error(), filepath.Join(out, "scoreboard.json")) {
		t.Fatalf("certification error = %v", err)
	}
	assertPerfScanPublishedGate(t, out, perfScanGateFail)
}

func assertPerfScanPublishedGate(t *testing.T, out, wantStatus string) {
	t.Helper()
	board, err := perfScanReadScoreboard(filepath.Join(out, "scoreboard.json"))
	if err != nil {
		t.Fatalf("read published JSON: %v", err)
	}
	if board.Gate == nil || board.Gate.Status != wantStatus {
		t.Fatalf("published gate = %+v, want %s", board.Gate, wantStatus)
	}
	markdown, err := os.ReadFile(filepath.Join(out, "scoreboard.md"))
	if err != nil {
		t.Fatalf("read published Markdown: %v", err)
	}
	if !strings.Contains(string(markdown), "outcome: `"+strings.ToUpper(wantStatus)+"`") {
		t.Fatalf("published Markdown does not render %s gate", wantStatus)
	}
}

func assertPerfScanPublishedArtifactsEqual(t *testing.T, reportOut, certifyOut string) {
	t.Helper()
	for _, name := range []string{"scoreboard.json", "scoreboard.md"} {
		reportData, err := os.ReadFile(filepath.Join(reportOut, name))
		if err != nil {
			t.Fatalf("read report %s: %v", name, err)
		}
		certifyData, err := os.ReadFile(filepath.Join(certifyOut, name))
		if err != nil {
			t.Fatalf("read certify %s: %v", name, err)
		}
		if !reflect.DeepEqual(reportData, certifyData) {
			t.Fatalf("%s differs by mode", name)
		}
	}
}

func TestPerfScanScoreboardAtomicWrite(t *testing.T) {
	const revision = "7b797554397b814f981e9a3fb462b84a3d0f1837"
	board := perfScanTestShard("go", "/corpus.lock", strings.Repeat("a", 64), 1, revision)
	outDir := t.TempDir()
	if err := perfScanWriteScoreboard(outDir, board); err != nil {
		t.Fatalf("write scoreboard: %v", err)
	}
	decoded, err := perfScanReadScoreboard(filepath.Join(outDir, "scoreboard.json"))
	if err != nil {
		t.Fatalf("read scoreboard: %v", err)
	}
	if decoded.GitRevision != revision || !decoded.GitClean || len(decoded.Languages) != 1 {
		t.Fatalf("round-trip scoreboard = %+v", decoded)
	}
	if decoded.Reduction != nil {
		t.Fatalf("unreduced shard recorded reducer provenance: %+v", decoded.Reduction)
	}
	jsonData, err := os.ReadFile(filepath.Join(outDir, "scoreboard.json"))
	if err != nil || strings.Contains(string(jsonData), `"reduction"`) {
		t.Fatalf("unreduced shard JSON contains reduction provenance: %v %s", err, jsonData)
	}
	markdown, err := os.ReadFile(filepath.Join(outDir, "scoreboard.md"))
	if err != nil || !strings.Contains(string(markdown), revision) {
		t.Fatalf("read markdown = %v; contents=%q", err, markdown)
	}
	assertNoTemps := func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read dir: %v", err)
		}
		for _, entry := range entries {
			if strings.Contains(entry.Name(), ".tmp-") {
				t.Fatalf("temporary scoreboard survived: %s", entry.Name())
			}
		}
	}
	assertNoTemps(outDir)

	failureDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(failureDir, "scoreboard.md"), 0o755); err != nil {
		t.Fatalf("create rename blocker: %v", err)
	}
	if err := perfScanWriteScoreboard(failureDir, board); err == nil {
		t.Fatal("scoreboard write unexpectedly succeeded through rename blocker")
	}
	assertNoTemps(failureDir)
	if _, err := os.Stat(filepath.Join(failureDir, "scoreboard.json")); !os.IsNotExist(err) {
		t.Fatalf("JSON commit marker exists after failed publication: %v", err)
	}

	jsonFailureDir := t.TempDir()
	renames := 0
	failJSONRename := func(oldPath, newPath string) error {
		renames++
		if renames == 2 {
			return fmt.Errorf("injected JSON rename failure")
		}
		return os.Rename(oldPath, newPath)
	}
	if err := perfScanWriteScoreboardWithRename(jsonFailureDir, board, failJSONRename); err == nil || !strings.Contains(err.Error(), "injected JSON") {
		t.Fatalf("JSON rename failure = %v", err)
	}
	assertNoTemps(jsonFailureDir)
	for _, name := range []string{"scoreboard.md", "scoreboard.json"} {
		if _, err := os.Stat(filepath.Join(jsonFailureDir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s exposed after failed JSON commit: %v", name, err)
		}
	}

	rollbackDir := t.TempDir()
	if err := perfScanWriteScoreboard(rollbackDir, board); err != nil {
		t.Fatalf("write prior checkpoint: %v", err)
	}
	newRevision := strings.Repeat("b", 40)
	newBoard := perfScanTestShard("go", "/corpus.lock", strings.Repeat("a", 64), 1, newRevision)
	failJSONCommit := func(oldPath, newPath string) error {
		if filepath.Base(newPath) == "scoreboard.json" {
			return fmt.Errorf("injected JSON commit failure")
		}
		return os.Rename(oldPath, newPath)
	}
	if err := perfScanWriteScoreboardWithRename(rollbackDir, newBoard, failJSONCommit); err == nil {
		t.Fatal("replacement checkpoint unexpectedly succeeded")
	}
	prior, err := perfScanReadScoreboard(filepath.Join(rollbackDir, "scoreboard.json"))
	if err != nil || prior.GitRevision != revision {
		t.Fatalf("prior JSON checkpoint was not preserved: %+v, %v", prior, err)
	}
	priorMarkdown, err := os.ReadFile(filepath.Join(rollbackDir, "scoreboard.md"))
	if err != nil || !strings.Contains(string(priorMarkdown), revision) || strings.Contains(string(priorMarkdown), newRevision) {
		t.Fatalf("prior Markdown checkpoint was not restored: %v %q", err, priorMarkdown)
	}
	assertNoTemps(rollbackDir)
}

func TestPerfScanHardGateRequiresEvidence(t *testing.T) {
	if report := perfScanEvaluateHardGate(&perfScanScoreboard{}); report.Status != perfScanGateFail || len(report.Failures) == 0 {
		t.Fatalf("empty scoreboard passed hard gate: %+v", report)
	}
	row := &perfScanLanguage{Language: "go", Status: perfScanStatusOK}
	board := &perfScanScoreboard{
		Config:    perfScanConfig{RequireFleet: true},
		Corpus:    perfScanCorpusCoverage{LockLanguages: 1, SelectedLanguages: 1},
		Languages: []*perfScanLanguage{row},
	}
	if report := perfScanEvaluateHardGate(board); report.Status != perfScanGateFail || report.FullFilesEvaluated != 0 {
		t.Fatalf("zero-file language passed hard gate: %+v", report)
	}
}

func perfScanTestShard(lang, lockPath, lockSHA string, lockLanguages int, revision string) *perfScanScoreboard {
	full := &perfScanFileAxis{
		Status:     perfScanStatusOK,
		GoMedianNs: 1_000,
		CMedianNs:  1_000,
		Ratio:      1,
		Verdict:    perfScanBucketLe12,
	}
	row := &perfScanLanguage{
		Language:      lang,
		Status:        perfScanStatusOK,
		FilesSelected: 1,
		FilesMeasured: 1,
		BytesMeasured: 64,
		Axes:          map[string]*perfScanLangAxis{},
		Files: []*perfScanFile{{
			Path:  "fixture." + lang,
			Bytes: 64,
			Classification: &perfScanFileClassification{
				Class:    perfScanClassClean,
				Reason:   "accepted full-span tree without ERROR nodes",
				GoStatus: perfScanStatusOK,
				FullSpan: true,
			},
			Axes: map[string]*perfScanFileAxis{perfScanAxisFull: full},
		}},
	}
	cfg := perfScanConfig{
		CorpusRoot:          "/corpus/corpus_sources",
		Reps:                5,
		Warmup:              1,
		FileBudgetMS:        10_000,
		LangTimeoutMS:       600_000,
		MaxFiles:            8,
		MaxFileBytes:        4 << 20,
		Order:               "largest",
		Axes:                []string{perfScanAxisFull},
		HardGate:            true,
		RequireFleet:        false,
		CorpusLock:          lockPath,
		CorpusLockSHA256:    lockSHA,
		CorpusLockLanguages: lockLanguages,
		Languages:           []string{lang},
	}
	perfScanAggregateLanguage(row, cfg)
	board := &perfScanScoreboard{
		Schema:      perfScanSchema,
		GeneratedAt: time.Unix(1_752_300_000, 0).UTC().Format(time.RFC3339),
		GitRevision: revision,
		GitClean:    true,
		Host: perfScanHost{
			Hostname:     "quiet-box",
			GOOS:         "linux",
			GOARCH:       "amd64",
			NumCPU:       16,
			GOMAXPROCS:   1,
			GoVersion:    "go1.test",
			LoadavgStart: "0.10 0.10 0.10",
			LoadavgEnd:   "0.20 0.10 0.10",
		},
		Config:    cfg,
		Summary:   map[string]int{row.Verdict: 1},
		Languages: []*perfScanLanguage{row},
		Corpus: perfScanCorpusCoverage{
			LockPath:          lockPath,
			LockSHA256:        lockSHA,
			LockLanguages:     lockLanguages,
			SelectedLanguages: 1,
		},
	}
	board.FullParseSplit = perfScanAggregateFleetCleanErrorSplit(board.Languages)
	gate := perfScanEvaluateHardGate(board)
	board.Gate = &gate
	return board
}

func perfScanWriteTestScoreboard(t *testing.T, dir, name string, board *perfScanScoreboard) string {
	t.Helper()
	data, err := json.MarshalIndent(board, "", "  ")
	if err != nil {
		t.Fatalf("marshal shard: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write shard: %v", err)
	}
	return path
}
