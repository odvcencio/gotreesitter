//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const (
	canonicalGoIncrementalManifestSchema = "canonical-incremental-before-state-v2"
	canonicalIncrementalReceiptSchema    = "canonical-incremental-before-state-receipt-v1"
	canonicalIncrementalReceiptEnv       = "GTS_CANONICAL_INCREMENTAL_RECEIPT_OUT"
)

//go:embed testdata/canonical_go_incremental_edits.json
var canonicalGoIncrementalManifestJSON []byte

//go:embed testdata/canonical_incremental_python_scanner.py
var canonicalIncrementalPythonScannerSource []byte

//go:embed testdata/canonical_incremental_go_leaf_control.go
var canonicalIncrementalGoLeafSource []byte

type canonicalGoIncrementalManifest struct {
	Schema string                           `json:"schema"`
	Edits  []canonicalGoIncrementalEditSpec `json:"edits"`
}

type canonicalGoIncrementalEditSpec struct {
	Name                  string `json:"name"`
	Language              string `json:"language"`
	Fixture               string `json:"fixture"`
	Role                  string `json:"role"`
	StartByte             int    `json:"start_byte"`
	OldEndByte            int    `json:"old_end_byte"`
	OldText               string `json:"old_text"`
	NewText               string `json:"new_text"`
	SourceSHA256          string `json:"source_sha256"`
	EditedSHA256          string `json:"edited_sha256"`
	ForwardClassification string `json:"forward_classification"`
	ReverseClassification string `json:"reverse_classification"`
	ForwardOutcome        string `json:"forward_outcome"`
	ReverseOutcome        string `json:"reverse_outcome"`
	TimingEligible        bool   `json:"timing_eligible"`
}

type canonicalGoIncrementalCase struct {
	spec     canonicalGoIncrementalEditSpec
	source   []byte
	edited   []byte
	forward  gotreesitter.InputEdit
	reverse  gotreesitter.InputEdit
	cForward sitter.InputEdit
	cReverse sitter.InputEdit
}

type canonicalGoIncrementalDirection struct {
	name                   string
	from                   []byte
	to                     []byte
	goEdit                 gotreesitter.InputEdit
	cEdit                  sitter.InputEdit
	applyEdit              bool
	expectedClassification string
	expectedOutcome        string
	targetSHA              string
	targetKind             string
}

type canonicalIncrementalReceipt struct {
	Schema          string                            `json:"schema"`
	ManifestSchema  string                            `json:"manifest_schema"`
	ManifestSHA256  string                            `json:"manifest_sha256"`
	Status          string                            `json:"status"`
	MechanismStatus string                            `json:"mechanism_status"`
	Cases           []canonicalIncrementalCaseReceipt `json:"cases"`
}

type canonicalIncrementalCaseReceipt struct {
	Name           string                                 `json:"name"`
	Language       string                                 `json:"language"`
	Fixture        string                                 `json:"fixture"`
	Role           string                                 `json:"role"`
	TimingEligible bool                                   `json:"timing_eligible"`
	Directions     []canonicalIncrementalDirectionReceipt `json:"directions"`
}

type canonicalIncrementalDirectionReceipt struct {
	Name                   string                             `json:"name"`
	AppliedEdit            bool                               `json:"applied_edit"`
	FromSHA256             string                             `json:"from_sha256"`
	ToSHA256               string                             `json:"to_sha256"`
	ExpectedClassification string                             `json:"expected_classification"`
	ObservedClassification string                             `json:"observed_classification"`
	ExpectedOutcome        string                             `json:"expected_outcome"`
	ObservedStatus         string                             `json:"observed_status"`
	FirstDifference        string                             `json:"first_difference,omitempty"`
	ReturnedOldTree        bool                               `json:"returned_old_tree"`
	GoFreshDigest          string                             `json:"go_fresh_digest"`
	CFreshDigest           string                             `json:"c_fresh_digest"`
	GoIncrementalDigest    string                             `json:"go_incremental_digest"`
	CIncrementalDigest     string                             `json:"c_incremental_digest"`
	GoRoot                 canonicalIncrementalRootReceipt    `json:"go_root"`
	CRoot                  canonicalIncrementalRootReceipt    `json:"c_root"`
	GoFreshRoot            canonicalIncrementalRootReceipt    `json:"go_fresh_root"`
	CFreshRoot             canonicalIncrementalRootReceipt    `json:"c_fresh_root"`
	Profile                canonicalIncrementalProfileReceipt `json:"profile"`
	Runtime                canonicalIncrementalRuntimeReceipt `json:"runtime"`
}

type canonicalIncrementalRootReceipt struct {
	StartByte  uint32 `json:"start_byte"`
	EndByte    uint32 `json:"end_byte"`
	HasError   bool   `json:"has_error"`
	Stop       string `json:"stop"`
	ChildCount int    `json:"child_count"`
}

type canonicalIncrementalProfileReceipt struct {
	ReuseUnsupported              bool   `json:"reuse_unsupported"`
	ReuseUnsupportedReason        string `json:"reuse_unsupported_reason,omitempty"`
	AcceptedErrorRetryAttempts    uint8  `json:"accepted_error_retry_attempts"`
	AcceptedErrorRetryAdopted     bool   `json:"accepted_error_retry_adopted"`
	AcceptedErrorRetryMergePerKey int    `json:"accepted_error_retry_merge_per_key"`
	AcceptedErrorRetryCause       string `json:"accepted_error_retry_cause"`
	OldTreeReuseRoute             bool   `json:"old_tree_reuse_route"`
	ReusedSubtrees                uint64 `json:"reused_subtrees"`
	ReusedBytes                   uint64 `json:"reused_bytes"`
	NewNodesAllocated             uint64 `json:"new_nodes_allocated"`
	TokensConsumed                uint64 `json:"tokens_consumed"`
	MaxStacksSeen                 int    `json:"max_stacks_seen"`
	SingleStackIterations         int    `json:"single_stack_iterations"`
	MultiStackIterations          int    `json:"multi_stack_iterations"`
	RecoverSearches               uint64 `json:"recover_searches"`
	RecoverStateChecks            uint64 `json:"recover_state_checks"`
	RecoverHits                   uint64 `json:"recover_hits"`
	StopReason                    string `json:"stop_reason"`
}

type canonicalIncrementalRuntimeReceipt struct {
	StopReason                         string `json:"stop_reason"`
	AcceptedErrorRetryAttempts         uint8  `json:"accepted_error_retry_attempts"`
	AcceptedErrorRetryAdopted          bool   `json:"accepted_error_retry_adopted"`
	AcceptedErrorRetryMergePerKey      int    `json:"accepted_error_retry_merge_per_key"`
	AcceptedErrorRetryCause            string `json:"accepted_error_retry_cause"`
	OldTreeReuseRoute                  bool   `json:"old_tree_reuse_route"`
	TokensConsumed                     uint64 `json:"tokens_consumed"`
	MaxStacksSeen                      int    `json:"max_stacks_seen"`
	SingleStackIterations              int    `json:"single_stack_iterations"`
	MultiStackIterations               int    `json:"multi_stack_iterations"`
	NodesAllocated                     int    `json:"nodes_allocated"`
	CRecoveryEnteredErrorState         bool   `json:"c_recovery_entered_error_state"`
	ExternalScannerCheckpointRecords   uint64 `json:"external_scanner_checkpoint_records"`
	ExternalScannerCheckpointLeafNodes uint64 `json:"external_scanner_checkpoint_leaf_nodes"`
	ExternalScannerSnapshotBytes       uint64 `json:"external_scanner_snapshot_bytes"`
}

func TestCanonicalGoIncrementalManifest(t *testing.T) {
	cases := loadCanonicalGoIncrementalCases(t)
	if len(cases) != 7 {
		t.Fatalf("canonical incremental cases=%d want=7", len(cases))
	}
}

func TestCanonicalGoIncrementalParity(t *testing.T) {
	if os.Getenv("GTS_CANONICAL_GO_INCREMENTAL") != "1" {
		t.Skip("set GTS_CANONICAL_GO_INCREMENTAL=1 to run locked four-way incremental parity")
	}
	receiptPath := prepareCanonicalIncrementalReceiptOutput(t)
	cases := loadCanonicalGoIncrementalCases(t)
	receipt := canonicalIncrementalReceipt{
		Schema:          canonicalIncrementalReceiptSchema,
		ManifestSchema:  canonicalGoIncrementalManifestSchema,
		ManifestSHA256:  sha256Hex(canonicalGoIncrementalManifestJSON),
		Status:          "passed",
		MechanismStatus: "passed",
		Cases:           make([]canonicalIncrementalCaseReceipt, 0, len(cases)),
	}
	for _, tc := range cases {
		tc := tc
		var caseReceipt canonicalIncrementalCaseReceipt
		if !t.Run(tc.spec.Name, func(t *testing.T) {
			goLang := canonicalIncrementalGoLanguage(t, tc.spec.Language)
			cLang := canonicalIncrementalCLanguage(t, tc.spec.Language)
			caseReceipt = admitCanonicalGoIncrementalCase(t, tc, goLang, cLang)
		}) {
			return
		}
		receipt.Cases = append(receipt.Cases, caseReceipt)
	}
	receipt.Status = canonicalIncrementalClosureStatus(receipt)
	if receipt.Status != "passed" {
		t.Fatalf("canonical incremental closure status=%s want=passed", receipt.Status)
	}
	if receiptPath != "" {
		publishCanonicalIncrementalReceipt(t, receiptPath, receipt)
	}
	if len(receipt.Cases) != len(cases) {
		t.Fatalf("canonical incremental receipt cases=%d want=%d", len(receipt.Cases), len(cases))
	}
}

func TestCanonicalIncrementalClassificationRejectsFallback(t *testing.T) {
	profile := gotreesitter.IncrementalParseProfile{
		ReuseUnsupported:       true,
		ReuseUnsupportedReason: "external_scanner_unsupported",
	}
	runtime := gotreesitter.ParseRuntime{StopReason: gotreesitter.ParseStopAccepted}
	if got := classifyCanonicalIncrementalDirection(false, false, profile, runtime, false); got != "full_parse_fallback" {
		t.Fatalf("fallback classification=%q want=full_parse_fallback", got)
	}
}

func TestCanonicalIncrementalClassificationCompactReparse(t *testing.T) {
	profile := gotreesitter.IncrementalParseProfile{
		OldTreeReuseRoute: true, ReparseNanos: 1, TokensConsumed: 8,
		NewNodesAllocated: 12, ReusedSubtrees: 1, ReusedBytes: 14,
	}
	runtime := gotreesitter.ParseRuntime{
		StopReason: gotreesitter.ParseStopAccepted, IncrementalOldTreeReuseRoute: true,
		CompactIncrementalReuseRoute: true, CompactIncrementalReusedSubtrees: 1,
		CompactIncrementalReusedBytes: 14, TokensConsumed: 8, NodesAllocated: 12,
	}
	if got := classifyCanonicalIncrementalDirection(true, false, profile, runtime, false); got != "compact_incremental_reparse" {
		t.Fatalf("compact classification=%q", got)
	}
	for _, mutate := range []func(*gotreesitter.ParseRuntime){
		func(r *gotreesitter.ParseRuntime) { r.CompactIncrementalReuseRoute = false },
		func(r *gotreesitter.ParseRuntime) { r.CompactIncrementalReusedBytes++ },
		func(r *gotreesitter.ParseRuntime) { r.NodesAllocated = 0 },
		func(r *gotreesitter.ParseRuntime) { r.CompactIncrementalFullRecoveryRoute = true },
	} {
		changed := runtime
		mutate(&changed)
		if got := classifyCanonicalIncrementalDirection(true, false, profile, changed, false); got == "compact_incremental_reparse" {
			t.Fatalf("incomplete compact proof accepted: %+v", changed)
		}
	}
}

func TestCanonicalIncrementalClassificationSingleStackReparse(t *testing.T) {
	profile := gotreesitter.IncrementalParseProfile{
		OldTreeReuseRoute: true, ReparseNanos: 1, TokensConsumed: 3,
		NewNodesAllocated: 2, ReusedSubtrees: 2, ReusedBytes: 4,
		MaxStacksSeen: 1, SingleStackIterations: 3,
	}
	runtime := gotreesitter.ParseRuntime{
		StopReason: gotreesitter.ParseStopAccepted, IncrementalOldTreeReuseRoute: true,
		MaxStacksSeen: 1, SingleStackIterations: 3,
	}
	if got := classifyCanonicalIncrementalDirection(true, false, profile, runtime, false); got != "single_stack_incremental_reparse" {
		t.Fatalf("classification=%q", got)
	}
	for _, mutate := range []func(*gotreesitter.IncrementalParseProfile){
		func(p *gotreesitter.IncrementalParseProfile) { p.NewNodesAllocated = 0 },
		func(p *gotreesitter.IncrementalParseProfile) { p.ReparseNanos = 0 },
		func(p *gotreesitter.IncrementalParseProfile) { p.OldTreeReuseRoute = false },
		func(p *gotreesitter.IncrementalParseProfile) { p.ReusedSubtrees = 0 },
	} {
		changed := profile
		mutate(&changed)
		if got := classifyCanonicalIncrementalDirection(true, false, changed, runtime, false); got == "single_stack_incremental_reparse" {
			t.Fatalf("incomplete work proof classified as reparse: %+v", changed)
		}
	}
	profile.MaxStacksSeen = 2
	profile.MultiStackIterations = 1
	if got := classifyCanonicalIncrementalDirection(true, false, profile, runtime, false); got != "genuine_incremental_glr" {
		t.Fatalf("multi-stack classification=%q", got)
	}
	if got := classifyCanonicalIncrementalDirection(false, true, gotreesitter.IncrementalParseProfile{}, runtime, false); got != "unchanged_snapshot_identity" {
		t.Fatalf("no-edit classification=%q", got)
	}
}

func TestCanonicalIncrementalClassificationRecognizesAcceptedErrorRetry(t *testing.T) {
	profile := gotreesitter.IncrementalParseProfile{
		AcceptedErrorRetryAttempts:    1,
		AcceptedErrorRetryAdopted:     true,
		AcceptedErrorRetryMergePerKey: 3,
		AcceptedErrorRetryCause:       gotreesitter.IncrementalRetryCauseAcceptedErrorBaseMerge,
		OldTreeReuseRoute:             true,
		TokensConsumed:                2,
		NewNodesAllocated:             1,
		MaxStacksSeen:                 2,
		MultiStackIterations:          1,
	}
	runtime := gotreesitter.ParseRuntime{
		StopReason:                               gotreesitter.ParseStopAccepted,
		IncrementalAcceptedErrorRetryAttempts:    1,
		IncrementalAcceptedErrorRetryAdopted:     true,
		IncrementalAcceptedErrorRetryMergePerKey: 3,
		IncrementalAcceptedErrorRetryCause:       gotreesitter.IncrementalRetryCauseAcceptedErrorBaseMerge,
		IncrementalOldTreeReuseRoute:             true,
	}
	if got := classifyCanonicalIncrementalDirection(true, false, profile, runtime, false); got != "accepted_error_base_merge_retry" {
		t.Fatalf("accepted-error retry classification=%q want=accepted_error_base_merge_retry", got)
	}
}

func TestCanonicalIncrementalClosureStatus(t *testing.T) {
	passed := canonicalIncrementalReceipt{Cases: []canonicalIncrementalCaseReceipt{{Directions: []canonicalIncrementalDirectionReceipt{{ObservedStatus: "parity"}}}}}
	if got := canonicalIncrementalClosureStatus(passed); got != "passed" {
		t.Fatalf("passing closure status=%q want=passed", got)
	}
	blocked := canonicalIncrementalReceipt{Cases: []canonicalIncrementalCaseReceipt{{Directions: []canonicalIncrementalDirectionReceipt{{ObservedStatus: "incremental_mismatch"}}}}}
	if got := canonicalIncrementalClosureStatus(blocked); got != "blocked" {
		t.Fatalf("known-gap closure status=%q want=blocked", got)
	}
}

func TestCanonicalIncrementalReceiptPathStaysInHarness(t *testing.T) {
	for _, path := range []string{"../receipt.json", filepath.Join(string(filepath.Separator), "tmp", "receipt.json")} {
		t.Run(strings.ReplaceAll(path, string(filepath.Separator), "_"), func(t *testing.T) {
			if _, err := canonicalIncrementalReceiptPath(path); err == nil {
				t.Fatalf("receipt path %q unexpectedly accepted", path)
			}
		})
	}
}

// BenchmarkParityGoCanonicalIncremental measures only timing-eligible locked
// edits. Admission verifies fresh Go=fresh C, Go incremental=fresh Go, and C
// incremental=fresh C in both edit directions before either backend timer
// starts.
func BenchmarkParityGoCanonicalIncremental(b *testing.B) {
	cases := loadCanonicalGoIncrementalCases(b)
	for _, tc := range cases {
		tc := tc
		if !tc.spec.TimingEligible {
			continue
		}
		b.Run(tc.spec.Name, func(b *testing.B) {
			goLang := canonicalIncrementalGoLanguage(b, tc.spec.Language)
			cLang := canonicalIncrementalCLanguage(b, tc.spec.Language)
			admitCanonicalGoIncrementalCase(b, tc, goLang, cLang)
			b.Run("gotreesitter", func(b *testing.B) {
				benchmarkCanonicalGoIncremental(b, tc, goLang)
			})
			b.Run("tree-sitter-c", func(b *testing.B) {
				benchmarkCanonicalCIncremental(b, tc, cLang)
			})
		})
	}
}

func loadCanonicalGoIncrementalCases(tb testing.TB) []canonicalGoIncrementalCase {
	tb.Helper()
	decoder := json.NewDecoder(bytes.NewReader(canonicalGoIncrementalManifestJSON))
	decoder.DisallowUnknownFields()
	var manifest canonicalGoIncrementalManifest
	if err := decoder.Decode(&manifest); err != nil {
		tb.Fatalf("decode canonical Go incremental manifest: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		tb.Fatalf("canonical Go incremental manifest has trailing JSON: %v", err)
	}
	if manifest.Schema != canonicalGoIncrementalManifestSchema {
		tb.Fatalf("canonical Go incremental manifest schema=%q want=%q", manifest.Schema, canonicalGoIncrementalManifestSchema)
	}

	fixtures := loadCanonicalGoFixtures(tb)
	fixtureByID := make(map[string]benchfixtures.LoadedFixture, len(fixtures))
	for _, fixture := range fixtures {
		fixtureByID[fixture.Fixture.ID] = fixture
	}
	expected := []struct {
		name           string
		language       string
		fixture        string
		role           string
		timingEligible bool
	}{
		{name: "unchanged_snapshot_identity", language: "go", fixture: "query_compile", role: "control", timingEligible: true},
		{name: "same_length_leaf_validation", language: "go", fixture: "go_leaf_control", role: "control", timingEligible: true},
		{name: "token_class_change", language: "go", fixture: "query_compile", role: "representative", timingEligible: true},
		{name: "same_line_length_change", language: "go", fixture: "language", role: "representative", timingEligible: true},
		{name: "early_newline", language: "go", fixture: "grammargen_lr", role: "representative", timingEligible: true},
		{name: "recovery_deletion", language: "go", fixture: "rewrite", role: "representative", timingEligible: true},
		{name: "python_scanner_dedent", language: "python", fixture: "python_scanner_control", role: "control", timingEligible: true},
	}
	if len(manifest.Edits) != len(expected) {
		tb.Fatalf("canonical Go incremental manifest edits=%d want=%d", len(manifest.Edits), len(expected))
	}

	out := make([]canonicalGoIncrementalCase, 0, len(expected))
	for i, want := range expected {
		spec := manifest.Edits[i]
		if spec.Name != want.name || spec.Language != want.language || spec.Fixture != want.fixture || spec.Role != want.role || spec.TimingEligible != want.timingEligible {
			tb.Fatalf("canonical incremental edit[%d]=%s/%s/%s/%s timing=%v want=%s/%s/%s/%s timing=%v", i, spec.Name, spec.Language, spec.Fixture, spec.Role, spec.TimingEligible, want.name, want.language, want.fixture, want.role, want.timingEligible)
		}
		var source []byte
		switch spec.Language {
		case "go":
			if spec.Fixture == "go_leaf_control" {
				source = canonicalIncrementalGoLeafSource
				break
			}
			fixture, ok := fixtureByID[spec.Fixture]
			if !ok {
				tb.Fatalf("canonical incremental edit %q names unknown Go fixture %q", spec.Name, spec.Fixture)
			}
			if err := fixture.Fixture.VerifySource(fixture.Source); err != nil {
				tb.Fatalf("canonical incremental edit %q source: %v", spec.Name, err)
			}
			source = fixture.Source
		case "python":
			if spec.Fixture != "python_scanner_control" {
				tb.Fatalf("canonical incremental edit %q names unknown Python fixture %q", spec.Name, spec.Fixture)
			}
			source = canonicalIncrementalPythonScannerSource
		default:
			tb.Fatalf("canonical incremental edit %q names unsupported language %q", spec.Name, spec.Language)
		}
		out = append(out, makeCanonicalGoIncrementalCase(tb, spec, source))
	}
	return out
}

func makeCanonicalGoIncrementalCase(tb testing.TB, spec canonicalGoIncrementalEditSpec, source []byte) canonicalGoIncrementalCase {
	tb.Helper()
	observedSourceSHA := sha256Hex(source)
	if observedSourceSHA != spec.SourceSHA256 {
		tb.Fatalf("canonical incremental edit %q source sha256=%s want=%s", spec.Name, observedSourceSHA, spec.SourceSHA256)
	}
	if spec.StartByte < 0 || spec.OldEndByte < spec.StartByte || spec.OldEndByte > len(source) {
		tb.Fatalf("canonical incremental edit %q range=[%d,%d) source=%d", spec.Name, spec.StartByte, spec.OldEndByte, len(source))
	}
	if got := string(source[spec.StartByte:spec.OldEndByte]); got != spec.OldText {
		tb.Fatalf("canonical incremental edit %q old text=%q want=%q", spec.Name, got, spec.OldText)
	}
	edited := applyCanonicalGoIncrementalEdit(source, spec)
	if got := sha256Hex(edited); got != spec.EditedSHA256 {
		tb.Fatalf("canonical incremental edit %q edited sha256=%s want=%s", spec.Name, got, spec.EditedSHA256)
	}
	forward := canonicalGoInputEdit(source, edited, spec.StartByte, spec.OldEndByte, spec.StartByte+len(spec.NewText))
	reverse := canonicalGoInputEdit(edited, source, spec.StartByte, spec.StartByte+len(spec.NewText), spec.StartByte+len(spec.OldText))
	return canonicalGoIncrementalCase{
		spec:     spec,
		source:   source,
		edited:   edited,
		forward:  forward,
		reverse:  reverse,
		cForward: realCorpusCInputEdit(forward),
		cReverse: realCorpusCInputEdit(reverse),
	}
}

func applyCanonicalGoIncrementalEdit(source []byte, spec canonicalGoIncrementalEditSpec) []byte {
	out := make([]byte, 0, len(source)-(spec.OldEndByte-spec.StartByte)+len(spec.NewText))
	out = append(out, source[:spec.StartByte]...)
	out = append(out, spec.NewText...)
	out = append(out, source[spec.OldEndByte:]...)
	return out
}

func canonicalGoInputEdit(oldSource, newSource []byte, start, oldEnd, newEnd int) gotreesitter.InputEdit {
	return gotreesitter.InputEdit{
		StartByte:   uint32(start),
		OldEndByte:  uint32(oldEnd),
		NewEndByte:  uint32(newEnd),
		StartPoint:  pointAtOffset(oldSource, start),
		OldEndPoint: pointAtOffset(oldSource, oldEnd),
		NewEndPoint: pointAtOffset(newSource, newEnd),
	}
}

func (tc canonicalGoIncrementalCase) directions() []canonicalGoIncrementalDirection {
	if tc.spec.ForwardClassification == "unchanged_snapshot_identity" {
		return []canonicalGoIncrementalDirection{
			{
				name:                   "identity",
				from:                   tc.source,
				to:                     tc.source,
				applyEdit:              false,
				expectedClassification: tc.spec.ForwardClassification,
				expectedOutcome:        tc.spec.ForwardOutcome,
				targetSHA:              tc.spec.SourceSHA256,
				targetKind:             "original",
			},
		}
	}
	return []canonicalGoIncrementalDirection{
		{
			name:                   "forward",
			from:                   tc.source,
			to:                     tc.edited,
			goEdit:                 tc.forward,
			cEdit:                  tc.cForward,
			applyEdit:              true,
			expectedClassification: tc.spec.ForwardClassification,
			expectedOutcome:        tc.spec.ForwardOutcome,
			targetSHA:              tc.spec.EditedSHA256,
			targetKind:             "edited",
		},
		{
			name:                   "reverse",
			from:                   tc.edited,
			to:                     tc.source,
			goEdit:                 tc.reverse,
			cEdit:                  tc.cReverse,
			applyEdit:              true,
			expectedClassification: tc.spec.ReverseClassification,
			expectedOutcome:        tc.spec.ReverseOutcome,
			targetSHA:              tc.spec.SourceSHA256,
			targetKind:             "original",
		},
	}
}

func admitCanonicalGoIncrementalCase(tb testing.TB, tc canonicalGoIncrementalCase, goLang *gotreesitter.Language, cLang *sitter.Language) canonicalIncrementalCaseReceipt {
	tb.Helper()
	goParser := gotreesitter.NewParser(goLang)
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		tb.Fatalf("%s set pinned %s C reference: %v", tc.spec.Name, tc.spec.Language, err)
	}
	receipt := canonicalIncrementalCaseReceipt{
		Name:           tc.spec.Name,
		Language:       tc.spec.Language,
		Fixture:        tc.spec.Fixture,
		Role:           tc.spec.Role,
		TimingEligible: tc.spec.TimingEligible,
		Directions:     make([]canonicalIncrementalDirectionReceipt, 0, len(tc.directions())),
	}
	for _, direction := range tc.directions() {
		receipt.Directions = append(receipt.Directions, admitCanonicalGoIncrementalDirection(tb, tc.spec.Name, direction, goParser, cParser, goLang))
	}
	return receipt
}

func admitCanonicalGoIncrementalDirection(tb testing.TB, editName string, direction canonicalGoIncrementalDirection, goParser *gotreesitter.Parser, cParser *sitter.Parser, goLang *gotreesitter.Language) canonicalIncrementalDirectionReceipt {
	tb.Helper()
	label := editName + "/" + direction.name
	if got := sha256Hex(direction.to); got != direction.targetSHA {
		tb.Fatalf("%s %s target sha256=%s want=%s", label, direction.targetKind, got, direction.targetSHA)
	}

	goFresh, err := goParser.Parse(direction.to)
	requireCanonicalGoIncrementalTree(tb, goFresh, direction.to, label+" fresh Go", err)
	defer releaseCanonicalGoTree(goFresh)
	cFresh := cParser.Parse(direction.to, nil)
	requireCanonicalCIncrementalTree(tb, cFresh, direction.to, label+" fresh C")
	defer closeCanonicalCTree(cFresh)
	if diff := FirstDivergenceDumpV1(goFresh.RootNode(), goLang, cFresh.RootNode()); diff != nil {
		tb.Fatalf("%s fresh Go != fresh C: %s", label, formatRealCorpusDivergence(diff))
	}

	goFreshDigest := canonicalGoTreeDigest(tb, goFresh, goLang, label+" fresh Go")
	cFreshDigest := canonicalCTreeDigest(tb, cFresh, label+" fresh C")
	if goFreshDigest != cFreshDigest {
		tb.Fatalf("%s fresh deep digest Go=%s C=%s", label, goFreshDigest, cFreshDigest)
	}

	cOld := cParser.Parse(direction.from, nil)
	requireCanonicalCIncrementalTree(tb, cOld, direction.from, label+" old C")
	if direction.applyEdit {
		cOld.Edit(&direction.cEdit)
	}
	cIncremental := cParser.Parse(direction.to, cOld)
	cOld.Close()
	requireCanonicalCIncrementalTree(tb, cIncremental, direction.to, label+" incremental C")
	defer cIncremental.Close()
	cIncrementalDigest := canonicalCTreeDigest(tb, cIncremental, label+" incremental C")
	if cIncrementalDigest != cFreshDigest {
		tb.Fatalf("%s incremental C != fresh C: digest=%s want=%s", label, cIncrementalDigest, cFreshDigest)
	}

	goOld, err := goParser.Parse(direction.from)
	requireCanonicalGoIncrementalTree(tb, goOld, direction.from, label+" old Go", err)
	if direction.applyEdit {
		goOld.Edit(direction.goEdit)
	}
	goIncremental, profile, err := goParser.ParseIncrementalProfiled(direction.to, goOld)
	returnedOldTree := goIncremental == goOld
	if !returnedOldTree {
		releaseCanonicalGoTree(goOld)
	}
	requireCanonicalGoIncrementalTree(tb, goIncremental, direction.to, label+" incremental Go", err)
	defer releaseCanonicalGoTree(goIncremental)
	goIncrementalDigest := canonicalGoTreeDigest(tb, goIncremental, goLang, label+" incremental Go")
	observedStatus := "parity"
	firstDifference := ""
	if goIncrementalDigest != goFreshDigest {
		var errs []string
		compareGoNodes(goIncremental.RootNode(), goLang, goFresh.RootNode(), "root", &errs)
		observedStatus = "incremental_mismatch"
		firstDifference = firstRealCorpusBenchmarkError(errs)
	}
	if direction.expectedOutcome == "" {
		tb.Fatalf("%s manifest omitted expected outcome", label)
	}
	if observedStatus != direction.expectedOutcome {
		tb.Fatalf("%s observed outcome=%s want=%s: incremental_digest=%s fresh_digest=%s first=%s", label, observedStatus, direction.expectedOutcome, goIncrementalDigest, goFreshDigest, firstDifference)
	}
	runtime := goIncremental.ParseRuntime()
	observedClassification := classifyCanonicalIncrementalDirection(direction.applyEdit, returnedOldTree, profile, runtime, goIncremental.RootNode().HasError())
	requireCanonicalIncrementalClassification(tb, label, direction, observedClassification, returnedOldTree, profile, runtime, goIncremental.RootNode().HasError())

	goRoot := goIncremental.RootNode()
	cRoot := cIncremental.RootNode()
	goFreshRoot := goFresh.RootNode()
	cFreshRoot := cFresh.RootNode()
	return canonicalIncrementalDirectionReceipt{
		Name:                   direction.name,
		AppliedEdit:            direction.applyEdit,
		FromSHA256:             sha256Hex(direction.from),
		ToSHA256:               sha256Hex(direction.to),
		ExpectedClassification: direction.expectedClassification,
		ObservedClassification: observedClassification,
		ExpectedOutcome:        direction.expectedOutcome,
		ObservedStatus:         observedStatus,
		FirstDifference:        firstDifference,
		ReturnedOldTree:        returnedOldTree,
		GoFreshDigest:          goFreshDigest,
		CFreshDigest:           cFreshDigest,
		GoIncrementalDigest:    goIncrementalDigest,
		CIncrementalDigest:     cIncrementalDigest,
		GoRoot: canonicalIncrementalRootReceipt{
			StartByte:  goRoot.StartByte(),
			EndByte:    goRoot.EndByte(),
			HasError:   goRoot.HasError(),
			Stop:       string(runtime.StopReason),
			ChildCount: goRoot.ChildCount(),
		},
		CRoot: canonicalIncrementalRootReceipt{
			StartByte:  uint32(cRoot.StartByte()),
			EndByte:    uint32(cRoot.EndByte()),
			HasError:   cRoot.HasError(),
			Stop:       "accepted_full_span",
			ChildCount: int(cRoot.ChildCount()),
		},
		GoFreshRoot: canonicalIncrementalRootReceipt{
			StartByte:  goFreshRoot.StartByte(),
			EndByte:    goFreshRoot.EndByte(),
			HasError:   goFreshRoot.HasError(),
			Stop:       string(goFresh.ParseRuntime().StopReason),
			ChildCount: goFreshRoot.ChildCount(),
		},
		CFreshRoot: canonicalIncrementalRootReceipt{
			StartByte:  uint32(cFreshRoot.StartByte()),
			EndByte:    uint32(cFreshRoot.EndByte()),
			HasError:   cFreshRoot.HasError(),
			Stop:       "accepted_full_span",
			ChildCount: int(cFreshRoot.ChildCount()),
		},
		Profile: canonicalIncrementalProfileReceipt{
			ReuseUnsupported:              profile.ReuseUnsupported,
			ReuseUnsupportedReason:        profile.ReuseUnsupportedReason,
			AcceptedErrorRetryAttempts:    profile.AcceptedErrorRetryAttempts,
			AcceptedErrorRetryAdopted:     profile.AcceptedErrorRetryAdopted,
			AcceptedErrorRetryMergePerKey: profile.AcceptedErrorRetryMergePerKey,
			AcceptedErrorRetryCause:       canonicalIncrementalRetryCause(profile.AcceptedErrorRetryCause),
			OldTreeReuseRoute:             profile.OldTreeReuseRoute,
			ReusedSubtrees:                profile.ReusedSubtrees,
			ReusedBytes:                   profile.ReusedBytes,
			NewNodesAllocated:             profile.NewNodesAllocated,
			TokensConsumed:                profile.TokensConsumed,
			MaxStacksSeen:                 profile.MaxStacksSeen,
			SingleStackIterations:         profile.SingleStackIterations,
			MultiStackIterations:          profile.MultiStackIterations,
			RecoverSearches:               profile.RecoverSearches,
			RecoverStateChecks:            profile.RecoverStateChecks,
			RecoverHits:                   profile.RecoverHits,
			StopReason:                    string(profile.StopReason),
		},
		Runtime: canonicalIncrementalRuntimeReceipt{
			StopReason:                         string(runtime.StopReason),
			AcceptedErrorRetryAttempts:         runtime.IncrementalAcceptedErrorRetryAttempts,
			AcceptedErrorRetryAdopted:          runtime.IncrementalAcceptedErrorRetryAdopted,
			AcceptedErrorRetryMergePerKey:      runtime.IncrementalAcceptedErrorRetryMergePerKey,
			AcceptedErrorRetryCause:            canonicalIncrementalRetryCause(runtime.IncrementalAcceptedErrorRetryCause),
			OldTreeReuseRoute:                  runtime.IncrementalOldTreeReuseRoute,
			TokensConsumed:                     runtime.TokensConsumed,
			MaxStacksSeen:                      runtime.MaxStacksSeen,
			SingleStackIterations:              runtime.SingleStackIterations,
			MultiStackIterations:               runtime.MultiStackIterations,
			NodesAllocated:                     runtime.NodesAllocated,
			CRecoveryEnteredErrorState:         runtime.CRecoveryEnteredErrorState,
			ExternalScannerCheckpointRecords:   runtime.ExternalScannerCheckpointRecords,
			ExternalScannerCheckpointLeafNodes: runtime.ExternalScannerCheckpointLeafNodes,
			ExternalScannerSnapshotBytes:       runtime.ExternalScannerSnapshotBytesAllocated,
		},
	}
}

func canonicalIncrementalGoLanguage(tb testing.TB, name string) *gotreesitter.Language {
	tb.Helper()
	var lang *gotreesitter.Language
	switch name {
	case "go":
		lang = grammars.GoLanguage()
	case "python":
		lang = grammars.PythonLanguage()
	default:
		tb.Fatalf("canonical incremental language %q is not locked", name)
	}
	if lang == nil {
		tb.Fatalf("canonical incremental language %q returned nil", name)
	}
	return lang
}

func canonicalIncrementalCLanguage(tb testing.TB, name string) *sitter.Language {
	tb.Helper()
	if name == "go" {
		return loadCanonicalGoCLanguage(tb)
	}
	lang, err := ParityCLanguage(name)
	if err != nil {
		tb.Fatalf("load pinned %s C reference: %v", name, err)
	}
	return lang
}

func classifyCanonicalIncrementalDirection(appliedEdit, returnedOldTree bool, profile gotreesitter.IncrementalParseProfile, runtime gotreesitter.ParseRuntime, rootHasError bool) string {
	if profile.ReuseUnsupported {
		return "full_parse_fallback"
	}
	if !appliedEdit && returnedOldTree && canonicalIncrementalProfileIsZero(profile) {
		return "unchanged_snapshot_identity"
	}
	if appliedEdit && !returnedOldTree && !rootHasError && canonicalCompactReparseProof(profile, runtime) {
		return "compact_incremental_reparse"
	}
	if appliedEdit && !returnedOldTree && !rootHasError &&
		profile.AcceptedErrorRetryAttempts == 1 && profile.AcceptedErrorRetryAdopted &&
		profile.AcceptedErrorRetryMergePerKey > 0 &&
		profile.AcceptedErrorRetryCause == gotreesitter.IncrementalRetryCauseAcceptedErrorBaseMerge &&
		profile.OldTreeReuseRoute &&
		runtime.IncrementalAcceptedErrorRetryAttempts == 1 && runtime.IncrementalAcceptedErrorRetryAdopted &&
		runtime.IncrementalAcceptedErrorRetryMergePerKey == profile.AcceptedErrorRetryMergePerKey &&
		runtime.IncrementalAcceptedErrorRetryCause == profile.AcceptedErrorRetryCause &&
		runtime.IncrementalOldTreeReuseRoute {
		return "accepted_error_base_merge_retry"
	}
	if rootHasError && runtime.CRecoveryEnteredErrorState {
		return "recovery_incremental"
	}
	if runtime.ExternalScannerCheckpointRecords > 0 && runtime.ExternalScannerCheckpointLeafNodes > 0 && runtime.ExternalScannerSnapshotBytesAllocated > 0 {
		return "scanner_state_incremental"
	}
	if appliedEdit && !returnedOldTree && profile.TokensConsumed > 1 && profile.NewNodesAllocated > 0 &&
		(profile.MultiStackIterations > 0 || profile.MaxStacksSeen > 1) {
		return "genuine_incremental_glr"
	}
	if appliedEdit && !returnedOldTree && !rootHasError && canonicalSingleStackReparseProof(profile, runtime) {
		return "single_stack_incremental_reparse"
	}
	return "unclassified"
}

func canonicalCompactReparseProof(profile gotreesitter.IncrementalParseProfile, runtime gotreesitter.ParseRuntime) bool {
	return runtime.CompactIncrementalReuseRoute && !runtime.CompactIncrementalFullRecoveryRoute &&
		!profile.ReuseUnsupported && profile.OldTreeReuseRoute && runtime.IncrementalOldTreeReuseRoute &&
		profile.ReparseNanos > 0 && profile.TokensConsumed > 1 && profile.NewNodesAllocated > 0 &&
		profile.ReusedSubtrees > 0 && profile.ReusedBytes > 0 &&
		runtime.CompactIncrementalReusedSubtrees == profile.ReusedSubtrees && runtime.CompactIncrementalReusedBytes == profile.ReusedBytes &&
		runtime.TokensConsumed == profile.TokensConsumed && runtime.NodesAllocated > 0 && uint64(runtime.NodesAllocated) == profile.NewNodesAllocated &&
		runtime.StopReason == gotreesitter.ParseStopAccepted && !runtime.CRecoveryEnteredErrorState &&
		!runtime.Truncated && !runtime.TokenSourceEOFEarly && profile.AcceptedErrorRetryAttempts == 0 &&
		profile.RecoverSearches == 0 && profile.RecoverStateChecks == 0 && profile.RecoverHits == 0
}

func canonicalSingleStackReparseProof(profile gotreesitter.IncrementalParseProfile, runtime gotreesitter.ParseRuntime) bool {
	return !profile.ReuseUnsupported && profile.OldTreeReuseRoute && runtime.IncrementalOldTreeReuseRoute &&
		!runtime.CompactIncrementalReuseRoute &&
		profile.ReparseNanos > 0 && profile.TokensConsumed > 1 && profile.NewNodesAllocated > 0 &&
		profile.ReusedSubtrees > 0 && profile.ReusedBytes > 0 &&
		profile.MaxStacksSeen == 1 && profile.SingleStackIterations > 0 && profile.MultiStackIterations == 0 &&
		runtime.MaxStacksSeen == 1 && runtime.SingleStackIterations > 0 && runtime.MultiStackIterations == 0 &&
		runtime.StopReason == gotreesitter.ParseStopAccepted && !runtime.CRecoveryEnteredErrorState &&
		!runtime.Truncated && !runtime.TokenSourceEOFEarly && profile.AcceptedErrorRetryAttempts == 0 &&
		profile.RecoverSearches == 0 && profile.RecoverStateChecks == 0 && profile.RecoverHits == 0
}

func canonicalIncrementalProfileIsZero(profile gotreesitter.IncrementalParseProfile) bool {
	return profile.ReuseCursorNanos == 0 && profile.ReparseNanos == 0 &&
		profile.ReusedSubtrees == 0 && profile.ReusedBytes == 0 && profile.NewNodesAllocated == 0 &&
		!profile.ReuseUnsupported && profile.ReuseUnsupportedReason == "" &&
		profile.AcceptedErrorRetryAttempts == 0 && !profile.AcceptedErrorRetryAdopted &&
		profile.AcceptedErrorRetryMergePerKey == 0 && profile.AcceptedErrorRetryCause == gotreesitter.IncrementalRetryCauseNone &&
		!profile.OldTreeReuseRoute &&
		profile.TokensConsumed == 0 && profile.MaxStacksSeen == 0 &&
		profile.SingleStackIterations == 0 && profile.MultiStackIterations == 0 &&
		profile.RecoverSearches == 0 && profile.RecoverStateChecks == 0 && profile.RecoverHits == 0 &&
		(profile.StopReason == "" || profile.StopReason == gotreesitter.ParseStopNone)
}

func requireCanonicalIncrementalClassification(tb testing.TB, label string, direction canonicalGoIncrementalDirection, observed string, returnedOldTree bool, profile gotreesitter.IncrementalParseProfile, runtime gotreesitter.ParseRuntime, rootHasError bool) {
	tb.Helper()
	if profile.ReuseUnsupported || observed == "full_parse_fallback" {
		tb.Fatalf("%s unplanned full-parse fallback: unsupported=%v reason=%q observed=%s", label, profile.ReuseUnsupported, profile.ReuseUnsupportedReason, observed)
	}
	if runtime.StopReason != gotreesitter.ParseStopAccepted || runtime.Truncated || runtime.TokenSourceEOFEarly {
		tb.Fatalf("%s incremental runtime is not accepted full-span work: stop=%s truncated=%v token_source_eof_early=%v", label, runtime.StopReason, runtime.Truncated, runtime.TokenSourceEOFEarly)
	}
	if direction.expectedClassification == "" {
		tb.Fatalf("%s manifest omitted expected classification", label)
	}
	if observed != direction.expectedClassification {
		tb.Fatalf("%s classification=%s want=%s: returned_old=%v profile={reuse=%d/%d new=%d tokens=%d stacks=%d single=%d multi=%d recovery=%d/%d/%d unsupported=%v:%q} runtime={tokens=%d stacks=%d single=%d multi=%d nodes=%d recovery=%v checkpoints=%d leaves=%d snapshot=%d} root_error=%v",
			label, observed, direction.expectedClassification, returnedOldTree,
			profile.ReusedSubtrees, profile.ReusedBytes, profile.NewNodesAllocated, profile.TokensConsumed, profile.MaxStacksSeen,
			profile.SingleStackIterations, profile.MultiStackIterations, profile.RecoverSearches, profile.RecoverStateChecks, profile.RecoverHits,
			profile.ReuseUnsupported, profile.ReuseUnsupportedReason,
			runtime.TokensConsumed, runtime.MaxStacksSeen, runtime.SingleStackIterations, runtime.MultiStackIterations, runtime.NodesAllocated,
			runtime.CRecoveryEnteredErrorState, runtime.ExternalScannerCheckpointRecords, runtime.ExternalScannerCheckpointLeafNodes,
			runtime.ExternalScannerSnapshotBytesAllocated, rootHasError)
	}
	switch observed {
	case "unchanged_snapshot_identity":
		if direction.applyEdit || !returnedOldTree || !canonicalIncrementalProfileIsZero(profile) {
			tb.Fatalf("%s identity classification lacks pointer/profile proof", label)
		}
	case "compact_incremental_reparse":
		if !direction.applyEdit || returnedOldTree || rootHasError || !canonicalCompactReparseProof(profile, runtime) ||
			profile.ReusedBytes >= uint64(len(direction.to)) {
			tb.Fatalf("%s compact reparse lacks authenticated work and partial reuse: %+v runtime=%s", label, profile, runtime.Summary())
		}
	case "single_stack_incremental_reparse":
		if !direction.applyEdit || returnedOldTree || rootHasError || !canonicalSingleStackReparseProof(profile, runtime) ||
			profile.ReusedBytes >= uint64(len(direction.to)) {
			tb.Fatalf("%s single-stack reparse lacks partial reuse and real work proof: %+v runtime=%s", label, profile, runtime.Summary())
		}
	case "genuine_incremental_glr":
		if !direction.applyEdit || returnedOldTree || profile.TokensConsumed <= 1 || profile.NewNodesAllocated == 0 ||
			(profile.MultiStackIterations == 0 && profile.MaxStacksSeen <= 1) || rootHasError {
			tb.Fatalf("%s genuine GLR classification lacks clean multi-stack incremental work proof: %+v root_error=%v", label, profile, rootHasError)
		}
	case "accepted_error_base_merge_retry":
		if !direction.applyEdit || returnedOldTree || rootHasError ||
			profile.AcceptedErrorRetryAttempts != 1 || !profile.AcceptedErrorRetryAdopted ||
			profile.AcceptedErrorRetryMergePerKey != 3 ||
			profile.AcceptedErrorRetryCause != gotreesitter.IncrementalRetryCauseAcceptedErrorBaseMerge || !profile.OldTreeReuseRoute ||
			runtime.IncrementalAcceptedErrorRetryAttempts != 1 || !runtime.IncrementalAcceptedErrorRetryAdopted ||
			runtime.IncrementalAcceptedErrorRetryMergePerKey != 3 ||
			runtime.IncrementalAcceptedErrorRetryCause != gotreesitter.IncrementalRetryCauseAcceptedErrorBaseMerge || !runtime.IncrementalOldTreeReuseRoute ||
			profile.TokensConsumed <= 1 || profile.NewNodesAllocated == 0 ||
			(profile.MultiStackIterations == 0 && profile.MaxStacksSeen <= 1) {
			tb.Fatalf("%s accepted-error retry lacks exact adopted old-tree retry proof: profile=%+v runtime=%s root_error=%v", label, profile, runtime.Summary(), rootHasError)
		}
	case "recovery_incremental":
		if !direction.applyEdit || returnedOldTree || !rootHasError || !runtime.CRecoveryEnteredErrorState || profile.TokensConsumed == 0 {
			tb.Fatalf("%s recovery classification lacks ERROR/recovery work proof: %+v runtime=%s root_error=%v", label, profile, runtime.Summary(), rootHasError)
		}
	case "scanner_state_incremental":
		if !direction.applyEdit || returnedOldTree || profile.TokensConsumed == 0 || profile.ReusedSubtrees == 0 ||
			runtime.ExternalScannerCheckpointRecords == 0 || runtime.ExternalScannerCheckpointLeafNodes == 0 ||
			runtime.ExternalScannerSnapshotBytesAllocated == 0 {
			tb.Fatalf("%s scanner-state classification lacks checkpointed reuse proof: %+v runtime=%s", label, profile, runtime.Summary())
		}
	default:
		tb.Fatalf("%s unrecognized incremental classification %q", label, observed)
	}
}

func canonicalIncrementalRetryCause(cause gotreesitter.IncrementalRetryCause) string {
	switch cause {
	case gotreesitter.IncrementalRetryCauseNone:
		return "none"
	case gotreesitter.IncrementalRetryCauseAcceptedErrorBaseMerge:
		return "accepted_error_base_merge"
	default:
		return fmt.Sprintf("unknown_%d", cause)
	}
}

func prepareCanonicalIncrementalReceiptOutput(tb testing.TB) string {
	tb.Helper()
	raw := strings.TrimSpace(os.Getenv(canonicalIncrementalReceiptEnv))
	if raw == "" {
		return ""
	}
	path, err := canonicalIncrementalReceiptPath(raw)
	if err != nil {
		tb.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatalf("create canonical incremental receipt directory: %v", err)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		tb.Fatalf("remove stale canonical incremental receipt: %v", err)
	}
	return path
}

func canonicalIncrementalReceiptPath(raw string) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("canonical incremental receipt working directory: %w", err)
	}
	root, err := filepath.Abs(wd)
	if err != nil {
		return "", fmt.Errorf("canonical incremental receipt harness root: %w", err)
	}
	path := raw
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("canonical incremental receipt path: %w", err)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("canonical incremental receipt path %q must name a file under %s", raw, root)
	}
	return path, nil
}

func publishCanonicalIncrementalReceipt(tb testing.TB, path string, receipt canonicalIncrementalReceipt) {
	tb.Helper()
	tmp, err := os.CreateTemp(filepath.Dir(path), ".canonical-incremental-receipt-*.tmp")
	if err != nil {
		tb.Fatalf("create canonical incremental receipt temp file: %v", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(receipt); err != nil {
		tmp.Close()
		tb.Fatalf("encode canonical incremental receipt: %v", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		tb.Fatalf("sync canonical incremental receipt: %v", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		tb.Fatalf("chmod canonical incremental receipt: %v", err)
	}
	if err := tmp.Close(); err != nil {
		tb.Fatalf("close canonical incremental receipt: %v", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		tb.Fatalf("publish canonical incremental receipt: %v", err)
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		tb.Fatalf("open canonical incremental receipt directory: %v", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		tb.Fatalf("sync canonical incremental receipt directory: %v", err)
	}
}

func canonicalIncrementalClosureStatus(receipt canonicalIncrementalReceipt) string {
	for _, tc := range receipt.Cases {
		for _, direction := range tc.Directions {
			if direction.ObservedStatus != "parity" {
				return "blocked"
			}
		}
	}
	return "passed"
}

func benchmarkCanonicalGoIncremental(b *testing.B, tc canonicalGoIncrementalCase, lang *gotreesitter.Language) {
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(tc.source)
	requireCanonicalGoIncrementalTree(b, tree, tc.source, tc.spec.Name+" initial Go", err)
	defer func() { releaseCanonicalGoTree(tree) }()

	b.ReportAllocs()
	b.SetBytes(int64((len(tc.source) + len(tc.edited)) / 2))
	b.ResetTimer()
	var totals realCorpusIncrementalProfileTotals
	edited := false
	for i := 0; i < b.N; i++ {
		target := tc.edited
		edit := tc.forward
		if edited {
			target = tc.source
			edit = tc.reverse
		}
		editStart := time.Now()
		if tc.spec.ForwardClassification != "unchanged_snapshot_identity" {
			tree.Edit(edit)
		}
		totals.addEdit(time.Since(editStart))
		oldTree := tree
		parseStart := time.Now()
		newTree, profile, err := parser.ParseIncrementalProfiled(target, oldTree)
		totals.addParseWall(time.Since(parseStart))
		requireCanonicalGoIncrementalTree(b, newTree, target, tc.spec.Name+" timed Go", err)
		totals.add(profile)
		if newTree != oldTree {
			oldTree.Release()
		}
		tree = newTree
		edited = !edited
	}
	b.StopTimer()
	totals.report(b, b.N)
}

func benchmarkCanonicalCIncremental(b *testing.B, tc canonicalGoIncrementalCase, lang *sitter.Language) {
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(lang); err != nil {
		b.Fatalf("%s set pinned Go C reference: %v", tc.spec.Name, err)
	}
	tree := parser.Parse(tc.source, nil)
	requireCanonicalCIncrementalTree(b, tree, tc.source, tc.spec.Name+" initial C")
	defer func() { tree.Close() }()

	b.ReportAllocs()
	b.SetBytes(int64((len(tc.source) + len(tc.edited)) / 2))
	b.ResetTimer()
	var editNanos int64
	var parseNanos int64
	edited := false
	for i := 0; i < b.N; i++ {
		target := tc.edited
		edit := tc.cForward
		if edited {
			target = tc.source
			edit = tc.cReverse
		}
		editStart := time.Now()
		if tc.spec.ForwardClassification != "unchanged_snapshot_identity" {
			tree.Edit(&edit)
		}
		editNanos += time.Since(editStart).Nanoseconds()
		oldTree := tree
		parseStart := time.Now()
		newTree := parser.Parse(target, oldTree)
		parseNanos += time.Since(parseStart).Nanoseconds()
		requireCanonicalCIncrementalTree(b, newTree, target, tc.spec.Name+" timed C")
		if newTree != oldTree {
			oldTree.Close()
		}
		tree = newTree
		edited = !edited
	}
	b.StopTimer()
	if b.N > 0 {
		b.ReportMetric(float64(editNanos)/float64(b.N), "edit_ns/op")
		b.ReportMetric(float64(parseNanos)/float64(b.N), "parse_wall_ns/op")
	}
}

func requireCanonicalGoIncrementalTree(tb testing.TB, tree *gotreesitter.Tree, source []byte, phase string, err error) {
	tb.Helper()
	if err != nil {
		releaseCanonicalGoTree(tree)
		tb.Fatalf("%s: %v", phase, err)
	}
	if tree == nil || tree.RootNode() == nil {
		tb.Fatalf("%s returned nil tree or root", phase)
	}
	root := tree.RootNode()
	if root.StartByte() != 0 || root.EndByte() != uint32(len(source)) || tree.ParseStoppedEarly() {
		tb.Fatalf("%s root=%d..%d want=0..%d stopped=%v (%s)", phase, root.StartByte(), root.EndByte(), len(source), tree.ParseStoppedEarly(), tree.ParseRuntime().Summary())
	}
}

func requireCanonicalCIncrementalTree(tb testing.TB, tree *sitter.Tree, source []byte, phase string) {
	tb.Helper()
	if tree == nil || tree.RootNode() == nil {
		closeCanonicalCTree(tree)
		tb.Fatalf("%s returned nil tree or root", phase)
	}
	root := tree.RootNode()
	if root.StartByte() != 0 || root.EndByte() != uint(len(source)) {
		closeCanonicalCTree(tree)
		tb.Fatalf("%s root=%d..%d want=0..%d", phase, root.StartByte(), root.EndByte(), len(source))
	}
}

func canonicalGoTreeDigest(tb testing.TB, tree *gotreesitter.Tree, lang *gotreesitter.Language, phase string) string {
	tb.Helper()
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), lang)
	if err != nil {
		tb.Fatalf("%s deep digest: %v", phase, err)
	}
	return inspection.SHA256
}

func canonicalCTreeDigest(tb testing.TB, tree *sitter.Tree, phase string) string {
	tb.Helper()
	inspection, err := canonicalCTreeInspection(tree.RootNode())
	if err != nil {
		tb.Fatalf("%s deep digest: %v", phase, err)
	}
	return inspection.SHA256
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
