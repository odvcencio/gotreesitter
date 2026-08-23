//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type p25wExactLanguageResult struct {
	Mode                   string      `json:"mode"`
	Case                   string      `json:"case"`
	SourceSHA256           string      `json:"source_sha256"`
	EditedSHA256           string      `json:"edited_sha256"`
	GoFreshDigest          string      `json:"go_fresh_digest"`
	GoIncrementalDigest    string      `json:"go_incremental_digest"`
	CFreshDigest           string      `json:"c_fresh_digest"`
	CIncrementalDigest     string      `json:"c_incremental_digest"`
	GoFreshRoot            p25vRoot    `json:"go_fresh_root"`
	GoIncrementalRoot      p25vRoot    `json:"go_incremental_root"`
	CFreshRoot             p25vRoot    `json:"c_fresh_root"`
	CIncrementalRoot       p25vRoot    `json:"c_incremental_root"`
	GoIncrementalRuntime   p25vRuntime `json:"go_incremental_runtime"`
	GoIncrementalProfile   p25vProfile `json:"go_incremental_profile"`
	FreshDifference        string      `json:"fresh_difference,omitempty"`
	IncrementalDifference  string      `json:"incremental_difference,omitempty"`
	GoFreshWallNanos       int64       `json:"go_fresh_wall_nanos"`
	GoIncrementalWallNanos int64       `json:"go_incremental_wall_nanos"`
}

func TestP25wExactLanguageRoute(t *testing.T) {
	mode := "retry-enabled"
	if os.Getenv(p25vRetryBypassEnv) == "1" {
		mode = "retry-bypassed"
	}
	cases := loadCanonicalGoIncrementalCases(t)
	var tc *canonicalGoIncrementalCase
	for i := range cases {
		if cases[i].spec.Name == "same_line_length_change" {
			candidate := cases[i]
			tc = &candidate
			break
		}
	}
	if tc == nil {
		t.Fatal("same_line_length_change case is missing")
	}
	direction := tc.directions()[0]
	goLang := canonicalIncrementalGoLanguage(t, tc.spec.Language)
	cLang := canonicalIncrementalCLanguage(t, tc.spec.Language)

	goFreshParser := gotreesitter.NewParser(goLang)
	goFreshParser.SetAdmissionCandidateRoute(false)
	goFreshStart := time.Now()
	goFresh, err := goFreshParser.Parse(direction.to)
	goFreshWall := time.Since(goFreshStart)
	if err != nil {
		t.Fatalf("fresh Go parse: %v", err)
	}
	requireCanonicalGoIncrementalTree(t, goFresh, direction.to, "P25w fresh Go", nil)
	goFreshDigest := canonicalGoTreeDigest(t, goFresh, goLang, "P25w fresh Go")

	goOldParser := gotreesitter.NewParser(goLang)
	goOldParser.SetAdmissionCandidateRoute(false)
	goOld, err := goOldParser.Parse(direction.from)
	if err != nil {
		t.Fatalf("old Go parse: %v", err)
	}
	requireCanonicalGoIncrementalTree(t, goOld, direction.from, "P25w old Go", nil)
	goOld.Edit(direction.goEdit)
	goIncrementalParser := gotreesitter.NewParser(goLang)
	goIncrementalParser.SetAdmissionCandidateRoute(false)
	goIncrementalStart := time.Now()
	goIncremental, profile, err := goIncrementalParser.ParseIncrementalProfiled(direction.to, goOld)
	goIncrementalWall := time.Since(goIncrementalStart)
	if err != nil {
		t.Fatalf("incremental Go parse: %v", err)
	}
	if goIncremental != goOld {
		releaseCanonicalGoTree(goOld)
	}
	requireCanonicalGoIncrementalTree(t, goIncremental, direction.to, "P25w incremental Go", nil)
	goIncrementalDigest := canonicalGoTreeDigest(t, goIncremental, goLang, "P25w incremental Go")

	cParser := sitter.NewParser()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatalf("set C language: %v", err)
	}
	cFresh := cParser.Parse(direction.to, nil)
	requireCanonicalCIncrementalTree(t, cFresh, direction.to, "P25w fresh C")
	cFreshDigest := canonicalCTreeDigest(t, cFresh, "P25w fresh C")
	cOld := cParser.Parse(direction.from, nil)
	requireCanonicalCIncrementalTree(t, cOld, direction.from, "P25w old C")
	cOld.Edit(&direction.cEdit)
	cIncremental := cParser.Parse(direction.to, cOld)
	if cIncremental != cOld {
		closeCanonicalCTree(cOld)
	}
	requireCanonicalCIncrementalTree(t, cIncremental, direction.to, "P25w incremental C")
	cIncrementalDigest := canonicalCTreeDigest(t, cIncremental, "P25w incremental C")

	result := p25wExactLanguageResult{
		Mode:                   mode,
		Case:                   tc.spec.Name,
		SourceSHA256:           direction.fromSHA256(),
		EditedSHA256:           direction.toSHA256(),
		GoFreshDigest:          goFreshDigest,
		GoIncrementalDigest:    goIncrementalDigest,
		CFreshDigest:           cFreshDigest,
		CIncrementalDigest:     cIncrementalDigest,
		GoFreshRoot:            p25vGoRoot(goFresh.RootNode()),
		GoIncrementalRoot:      p25vGoRoot(goIncremental.RootNode()),
		CFreshRoot:             p25vCRoot(cFresh.RootNode()),
		CIncrementalRoot:       p25vCRoot(cIncremental.RootNode()),
		GoIncrementalRuntime:   p25vRuntimeFrom(goIncremental.ParseRuntime()),
		GoIncrementalProfile:   p25vProfileFrom(profile),
		GoFreshWallNanos:       goFreshWall.Nanoseconds(),
		GoIncrementalWallNanos: goIncrementalWall.Nanoseconds(),
	}
	if diff := FirstDivergenceDumpV1(goFresh.RootNode(), goLang, cFresh.RootNode()); diff != nil {
		result.FreshDifference = formatRealCorpusDivergence(diff)
	}
	if diff := FirstDivergenceDumpV1(goIncremental.RootNode(), goLang, cIncremental.RootNode()); diff != nil {
		result.IncrementalDifference = formatRealCorpusDivergence(diff)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal P25w result: %v", err)
	}
	t.Logf("P25W_RESULT %s", encoded)
	if path := os.Getenv("P25W_RESULT_OUT"); path != "" {
		if err := os.WriteFile(path, encoded, 0o644); err != nil {
			t.Fatalf("write P25w result: %v", err)
		}
	}
	if goIncrementalDigest != goFreshDigest || goIncrementalDigest != cFreshDigest || goIncrementalDigest != cIncrementalDigest {
		t.Errorf("P25w exact route deep digest mismatch: Go fresh=%s Go incremental=%s C fresh=%s C incremental=%s", goFreshDigest, goIncrementalDigest, cFreshDigest, cIncrementalDigest)
	}

	releaseCanonicalGoTree(goFresh)
	releaseCanonicalGoTree(goIncremental)
	closeCanonicalCTree(cFresh)
	closeCanonicalCTree(cIncremental)
	cParser.Close()
}
