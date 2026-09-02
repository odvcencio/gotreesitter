package gotreesitter

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	compactRouteLifecycleRegistryPath                 = "testdata/compact_route_campaign_lifecycle_v1.json"
	compactRouteLifecycleSourceRevision               = "5f6c67f40ebfbc36798023462cf3eb97c4d0e43e"
	compactRouteLifecycleHistoricalProofEnv           = "GTS_REQUIRE_HISTORICAL_RECEIPT_PROOF"
	compactRouteLifecycleReceiptPolicyCurrentRequired = "current_required"
	compactRouteLifecycleReceiptPolicyHistoricalOnly  = "historical_only"
	compactRouteLifecycleReceiptPolicyOptional        = "optional"
)

type compactRouteLifecycleHistoricalProofMode struct {
	Strict       bool
	Shallow      bool
	GitAvailable bool
}

var (
	compactRouteLifecycleCommitPattern   = regexp.MustCompile(`^[0-9a-f]{40}$`)
	compactRouteLifecycleSHA256Pattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	compactRouteLifecycleEnvPattern      = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	compactRouteLifecycleBuildTagPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
	compactRouteLifecyclePRRefPattern    = regexp.MustCompile(`^pr:#([0-9]+)$`)
)

var compactRouteLifecycleKnownProofRevisions = map[string]string{
	"pr:#1016": "8bb49959d95fce0cf8bea5468d86e30978ccf393",
	"cgo_harness/parity_compact_per_version_lexer_scala_test.go#TestCompactPerVersionLexerScalaCOracle": "8bb49959d95fce0cf8bea5468d86e30978ccf393",
	"pr:#1017": "865d03b07ffd74ee44163a4cfa2f7ddc057134e6",
	"internal/parsercorephase0/physical_head_merge_stage2_test.go#TestStage2PhysicalHeadMergeAtSixVersionBoundaryRetainsEveryPath": "865d03b07ffd74ee44163a4cfa2f7ddc057134e6",
	"pr:#1018": "9f9e903940655da46fb8c976fa2e08139153df0f",
	"parsercore_phase0_recovery_lineage_fork_internal_test.go#TestS5MissingInsertionForkPublishesBothRecoveryLineages": "9f9e903940655da46fb8c976fa2e08139153df0f",
	"pr:#1019": "0beb95d399655d200e687e0912d3265b1c9562e9",
	"pr:#1028": "90f60a65c61d87768b1221b78cb0f66d25aff8ec",
	"cgo_harness/eof_recovery_admission_oracle_test.go#TestEOFRecoveryAdmissionUsesLockedCEventsAndPublishedTree":       "2ebeb6c64632163622691786a71ac6da76a14929",
	"incremental_invariant_gate_test.go#TestIncrementalInvariantGatePython":                                             "bf7ab0567122e863ef831e8aa56dbee93127ad93",
	"w5_editor_latency_gate_test.go#TestW5EditorLatencyGate":                                                            "e734c93543654818082e8d3ae3721cdb6fe9e4a5",
	"cgo_harness/stage5_compact_incremental_test.go#TestStage5CompactPythonScannerCheckpointReuse":                      "5070ffd4594a819e8ebe78fa2bf651c197cbdce4",
	"cgo_harness/stage5_compact_incremental_test.go#TestStage5PythonWideIndentTransitionParity":                         "5070ffd4594a819e8ebe78fa2bf651c197cbdce4",
	"parser_external_scanner_prefix_frontier_test.go#TestCheckpointedScannerWithoutPrefixFrontierRequirementKeepsReuse": "e1de4484b5655ab11057f70d9dc7b1245972d03f",
	"parser_external_scanner_prefix_frontier_test.go#TestCheckpointedScannerPrefixFrontierAllowsSoleChildReuse":         "90f60a65c61d87768b1221b78cb0f66d25aff8ec",
	"cgo_harness/stage5_sole_child_padding_probe_test.go#TestStage5SoleChildHorizontalPaddingProbe":                     "90f60a65c61d87768b1221b78cb0f66d25aff8ec",
	"cgo_harness/stage6_typescript_compact_certification_test.go#TestStage6TypeScriptCompactCertification":              "0df3c2ab3b33fcd3374cf426ad7ea2a18ea33411",
	"cgo_harness/stage6_python_compact_certification_test.go#TestStage6PythonCompactCertification":                      "8c6a8e1fe472ade08462b91a2ba00624ab437d61",
	"cgo_harness/stage6_javascript_compact_certification_test.go#TestStage6JavaScriptCompactCertification":              "2455af16a52e4a886ae5a32842a98bb623f6671c",
	"cgo_harness/stage6_go_compact_certification_test.go#TestStage6GoCompactCertification":                              "9ec95dbcbc9a18071b697ce260e49189c0966036",
	"cgo_harness/stage6_rust_compact_certification_test.go#TestStage6RustCompactCertification":                          "80c9ed104785d46acd84a88c6df225836c876a32",
	"cgo_harness/stage6_json_compact_certification_test.go#TestStage6JSONCompactCertification":                          "f737578353f60bee8de5f6413b9534cc8713cc51",
	"cgo_harness/stage6_c_compact_certification_test.go#TestStage6CCompactCertification":                                "f5fc2537b3d8126bc1bdaeed6f42c0f21f139ffa",
	"cgo_harness/stage6_css_compact_certification_test.go#TestStage6CSSCompactCertification":                            "030eeb6af96005367d4e4eb9ad92f204c72aa1d4",
	"cgo_harness/stage6_json5_compact_certification_test.go#TestStage6JSON5CompactCertification":                        "5f6c67f40ebfbc36798023462cf3eb97c4d0e43e",
	"external_scanner_checkpoints_test.go#TestRebuildExternalScannerCheckpointsCopiesBorrowedArenaSnapshots":            "b4833798bb7d934bcb8d280a7cd5937d34fa2c9a",
	"cgo_harness/stage5_compact_incremental_test.go#TestStage5PythonPrefixFrontierAdversarialParity":                    "b4833798bb7d934bcb8d280a7cd5937d34fa2c9a",
	"parser_external_scanner_incremental_test.go#TestPythonSameSymbolScannerStateEditFallsBack":                         "9bd3e9a971b323e2cdeed302aa045af174ffe53c",
	"cgo_harness/stage5_compact_incremental_test.go#TestStage5PythonSameLengthScannerDelimiterParity":                   "9bd3e9a971b323e2cdeed302aa045af174ffe53c",
	"incremental_leaf_fastpath_test.go#TestIncludedRangeCheckpointLeafProbeRestoresTokenProvenance":                     "425ffdc2eb919b82b38484a0b3a2107d5c7341f1",
	"internal/parsercorephase0/recursive_insert_test.go#TestAuthenticatedTerminalScannerProvenanceCoversOrdinaryLeaf":   "0d6683a465c6d20477c6a16b6b1f70ac2a90e87b",
	"testdata/incremental_allowlist.json":                                        "95a1d13156b82e2221415342c9ac8048f2caf644",
	"docs/compact-route-real-corpus-matrix.md#Current compact-graduation matrix": "41af6afa6da47c9f8b55b3156fd88fb56686373f",
	"docs/compact-route-coverage-census.md":                                      "38d86d417fc6209228b8b175bc2aa7bb9700b8d3",
	"docs/a8-derivation-divergence-certificate-v1.md":                            "acccb81d20128e337a35ee274a2e3e3631b73b1a",
}

var compactRouteLifecycleKnownCorpusTreeSHA256 = map[string]string{
	"scala-owned-width":        "81dc569b1ad3d567ed158aea75fd30a388ec1f4d3e1cbdd08c29a6535bd456d3",
	"typescript-compact-smoke": "840eb4cf52faf1b944dacf47bb13591a5fd61fbfe33f196172094d4b4acab907",
	"python-compact-smoke":     "7d857f4d100cc7c7bf61bd1e77845066add55e2fd4f82e60309203ceb8569543",
	"javascript-compact-smoke": "ed169501f3515f625f9337c18ffe3f67ba41b1573df02e07cc54d85a26978501",
	"go-compact-smoke":         "94fcf4849cea5bcccc24ae0a1e70ee06218e0daad96434b74c27864db9fe0c81",
	"rust-compact-smoke":       "f7120d52d2861af7fe4dc6b6d7f0073404236fe8cc6f812f9e40a16253521c63",
	"json-compact-smoke":       "a6a9ea2b18c6a1299d3b6c92150ac65bb6bdb4d3e5471835bf6a8ceabe37e36e",
	"c-compact-smoke":          "b35547117f044e74311e70eeb45bd3967598fdca38a963937e2eeaad29bed7b7",
	"css-compact-smoke":        "1363ea554c5d75bffc553bce1514068e02eb934c204f585e452ddc9211ea4af0",
	"json5-compact-smoke":      "84634d0f446a4af1d216bd534c2456006827b71c13a99f0fdfc4f71c76d19dc8",
}

type compactRouteLifecycleRegistry struct {
	Schema               string                          `json:"schema"`
	SourceRevision       string                          `json:"source_revision"`
	Scope                string                          `json:"scope"`
	Authority            []string                        `json:"authority"`
	CampaignIdentifiers  []string                        `json:"campaign_identifiers"`
	Funnel               compactRouteLifecycleFunnel     `json:"funnel"`
	CorePRGate           compactRouteLifecycleCorePRGate `json:"core_pr_gate"`
	GraduationDefinition []string                        `json:"graduation_definition"`
	HistoryPolicy        compactRouteLifecycleHistory    `json:"history_policy"`
	Vocabularies         compactRouteLifecycleVocab      `json:"vocabularies"`
	Controls             []compactRouteLifecycleControl  `json:"controls"`
	Entries              []compactRouteLifecycleEntry    `json:"entries"`
}

type compactRouteLifecycleFunnel struct {
	OrderedStages                  []int    `json:"ordered_stages"`
	ActiveCoreStage                int      `json:"active_core_stage"`
	CoreRule                       string   `json:"core_rule"`
	ParallelLanes                  []string `json:"parallel_lanes"`
	CorrectnessPrecedesPerformance bool     `json:"correctness_precedes_performance"`
}

type compactRouteLifecycleCorePRGate struct {
	RequiredFields  []string `json:"required_fields"`
	PerformanceRule string   `json:"performance_rule"`
}

type compactRouteLifecycleHistory struct {
	PreserveEntries     bool                             `json:"preserve_entries"`
	PreserveReceiptRefs bool                             `json:"preserve_receipt_refs"`
	PreservePlanRefs    bool                             `json:"preserve_plan_refs"`
	RetirementRule      string                           `json:"retirement_rule"`
	StaleRule           string                           `json:"stale_rule"`
	StatePolicy         compactRouteLifecycleStatePolicy `json:"state_policy"`
}

type compactRouteLifecycleStatePolicy struct {
	CurrentReceiptRequired []string `json:"current_receipt_required"`
	HistoricalOnly         []string `json:"historical_only"`
	ReceiptsOptional       []string `json:"receipts_optional"`
}

type compactRouteLifecycleVocab struct {
	Owners          []string `json:"owners"`
	Statuses        []string `json:"statuses"`
	ReceiptStatuses []string `json:"receipt_statuses"`
	PlanStatuses    []string `json:"plan_statuses"`
	Retention       []string `json:"retention"`
	Lanes           []string `json:"lanes"`
	ControlKinds    []string `json:"control_kinds"`
	PathRoles       []string `json:"path_roles"`
}

type compactRouteLifecycleControl struct {
	ID         string                          `json:"id"`
	Kind       string                          `json:"kind"`
	Expression string                          `json:"expression"`
	Scope      string                          `json:"scope"`
	Name       string                          `json:"name"`
	Default    string                          `json:"default"`
	ReadSites  []compactRouteLifecycleReadSite `json:"read_sites"`
}

type compactRouteLifecycleReadSite struct {
	Path   string `json:"path"`
	Symbol string `json:"symbol"`
	Line   int    `json:"line"`
}

type compactRouteLifecycleEntry struct {
	ID                string                         `json:"id"`
	Stage             int                            `json:"stage"`
	Lane              string                         `json:"lane"`
	Status            string                         `json:"status"`
	Owner             string                         `json:"owner"`
	Campaigns         []string                       `json:"campaigns"`
	Purpose           string                         `json:"purpose"`
	BuildExpressions  []string                       `json:"build_expressions"`
	ControlIDs        []string                       `json:"control_ids"`
	Paths             []compactRouteLifecyclePath    `json:"paths"`
	TestCommands      []compactRouteLifecycleCommand `json:"test_commands"`
	Corpus            []compactRouteLifecycleCorpus  `json:"corpus"`
	Gates             []string                       `json:"gates"`
	Telemetry         []string                       `json:"telemetry"`
	PlanRefs          []compactRouteLifecyclePlan    `json:"plan_refs"`
	ProofReceipts     []compactRouteLifecycleReceipt `json:"proof_receipts"`
	DeletionCondition string                         `json:"deletion_condition"`
	Retention         string                         `json:"retention"`
	HistoryPreserved  bool                           `json:"history_preserved"`
}

type compactRouteLifecyclePath struct {
	Path            string `json:"path"`
	Role            string `json:"role"`
	BuildExpression string `json:"build_expression"`
	Symbol          string `json:"symbol"`
}

type compactRouteLifecycleCommand struct {
	Name       string `json:"name"`
	Command    string `json:"command"`
	WorkingDir string `json:"working_dir"`
	Isolation  string `json:"isolation"`
}

type compactRouteLifecycleCorpus struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Path         string `json:"path"`
	Source       string `json:"source"`
	SourceSHA256 string `json:"source_sha256"`
	TreeSHA256   string `json:"tree_sha256"`
	Availability string `json:"availability"`
	LockStatus   string `json:"lock_status"`
	Plan         string `json:"plan"`
	LockOrDigest string `json:"lock_or_digest"`
}

type compactRouteLifecyclePlan struct {
	Ref            string `json:"ref"`
	Kind           string `json:"kind"`
	SourceRevision string `json:"source_revision"`
	Status         string `json:"status"`
}

type compactRouteLifecycleReceipt struct {
	Ref            string `json:"ref"`
	Kind           string `json:"kind"`
	SourceRevision string `json:"source_revision"`
	Status         string `json:"status"`
}

func TestCompactRouteCampaignLifecycleRegistry(t *testing.T) {
	registry := loadCompactRouteLifecycleRegistry(t)

	if registry.Schema != "gotreesitter/compact-route-campaign-lifecycle/v1" {
		t.Fatalf("schema = %q", registry.Schema)
	}
	if registry.SourceRevision != compactRouteLifecycleSourceRevision {
		t.Fatalf("source_revision = %q, want %q", registry.SourceRevision, compactRouteLifecycleSourceRevision)
	}
	if !compactRouteLifecycleCommitPattern.MatchString(registry.SourceRevision) {
		t.Fatalf("source_revision is not a commit hash: %q", registry.SourceRevision)
	}
	if strings.TrimSpace(registry.Scope) == "" {
		t.Fatal("scope is empty")
	}
	if len(registry.Authority) == 0 || len(registry.CampaignIdentifiers) == 0 {
		t.Fatal("authority and campaign_identifiers are required")
	}
	validateStringVocabulary(t, "authority", registry.Authority, nil)
	validateStringVocabulary(t, "campaign_identifiers", registry.CampaignIdentifiers, nil)
	root, err := compactRouteLifecycleRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	historicalProofMode, err := compactRouteLifecycleHistoricalProofModeForRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := compactRouteLifecycleValidateRegistryBase(root, registry.SourceRevision, historicalProofMode); err != nil {
		t.Fatal(err)
	}

	validateCompactRouteLifecycleFunnel(t, registry)
	validateCompactRouteLifecycleGate(t, registry)
	validateCompactRouteLifecycleHistory(t, registry)
	validateCompactRouteLifecycleVocabularies(t, registry)

	campaigns := compactRouteLifecycleSet(t, "campaign_identifiers", registry.CampaignIdentifiers)
	for _, authority := range registry.Authority {
		if !campaigns[authority] {
			t.Errorf("authority %q is not a campaign identifier", authority)
		}
	}

	controls := validateCompactRouteLifecycleControls(t, registry, root)
	validateCompactRouteLifecycleEntries(t, registry, campaigns, controls, root, historicalProofMode)
}

func TestCompactRouteLifecycleRejectsFabricatedProofReceipt(t *testing.T) {
	registry := loadCompactRouteLifecycleRegistry(t)
	receipt := registry.Entries[0].ProofReceipts[0]
	receipt.SourceRevision = strings.Repeat("0", 40)
	if err := compactRouteLifecycleProofReceiptError(receipt); err == nil {
		t.Fatal("fabricated proof receipt was accepted")
	}
}

func TestCompactRouteLifecycleDefaultAllowsMissingHistoricalProofInShallowMode(t *testing.T) {
	registry := loadCompactRouteLifecycleRegistry(t)
	root, err := compactRouteLifecycleRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	mode, err := compactRouteLifecycleDecideHistoricalProofMode(false, true, true)
	if err != nil {
		t.Fatal(err)
	}
	receipt := registry.Entries[0].ProofReceipts[0]
	receipt.SourceRevision = strings.Repeat("0", 40)
	if err := compactRouteLifecycleProofRepositoryErrorAtMode(root, registry.SourceRevision, receipt, mode); err != nil {
		t.Fatalf("default shallow mode rejected an unavailable historical object: %v", err)
	}
}

func TestCompactRouteLifecycleShallowModeStillRequiresAllowlistEquality(t *testing.T) {
	registry := loadCompactRouteLifecycleRegistry(t)
	root, err := compactRouteLifecycleRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	mode, err := compactRouteLifecycleDecideHistoricalProofMode(false, true, true)
	if err != nil {
		t.Fatal(err)
	}
	receipt := registry.Entries[0].ProofReceipts[0]
	receipt.SourceRevision = strings.Repeat("0", 40)
	receipt.Ref = "pr:#9999"
	if err := compactRouteLifecycleProofReceiptErrorAtMode(root, registry.SourceRevision, receipt, mode); err == nil {
		t.Fatal("shallow mode accepted a proof receipt outside the immutable allowlist")
	}
}

func TestCompactRouteLifecycleShallowModeChecksAvailableHistoricalProof(t *testing.T) {
	registry := loadCompactRouteLifecycleRegistry(t)
	root, err := compactRouteLifecycleRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	mode, err := compactRouteLifecycleDecideHistoricalProofMode(false, true, true)
	if err != nil {
		t.Fatal(err)
	}
	receipt := registry.Entries[0].ProofReceipts[1]
	receipt.Ref += "#fabricatedLifecycleAnchor"
	if err := compactRouteLifecycleProofReceiptErrorAtMode(root, registry.SourceRevision, receipt, mode); err == nil {
		t.Fatal("shallow mode accepted an invalid anchor for available historical objects")
	}
}

func TestCompactRouteLifecycleRejectsPRReceiptWithWrongCommitMessage(t *testing.T) {
	registry := loadCompactRouteLifecycleRegistry(t)
	root, err := compactRouteLifecycleRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	receipt := registry.Entries[0].ProofReceipts[0]
	receipt.Ref = "pr:#1017"
	strictMode := compactRouteLifecycleHistoricalProofMode{Strict: true, GitAvailable: true}
	if err := compactRouteLifecycleProofRepositoryErrorAtMode(root, registry.SourceRevision, receipt, strictMode); err == nil {
		t.Fatal("a real ancestor with a different pull-request number was accepted")
	}
}

func TestCompactRouteLifecycleStrictModeRejectsShallowRepository(t *testing.T) {
	if _, err := compactRouteLifecycleDecideHistoricalProofMode(true, true, true); err == nil {
		t.Fatal("strict historical proof mode accepted a shallow repository")
	}
}

func TestCompactRouteLifecycleStrictModeRequiresGitRepository(t *testing.T) {
	if _, err := compactRouteLifecycleDecideHistoricalProofMode(true, false, false); err == nil {
		t.Fatal("strict historical proof mode accepted an unavailable repository")
	}
}

func TestCompactRouteLifecycleAllowsStaleReceiptOutsideCurrentLineage(t *testing.T) {
	registry := loadCompactRouteLifecycleRegistry(t)
	root, err := compactRouteLifecycleRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	mode, err := compactRouteLifecycleHistoricalProofModeForRepository(root)
	if err != nil {
		t.Skipf("strict stale-receipt proof requires a full Git repository: %v", err)
	}
	if mode.Shallow {
		t.Skip("strict stale-receipt proof requires a non-shallow Git repository")
	}
	var stale compactRouteLifecycleReceipt
	for _, receipt := range registry.Entries[4].ProofReceipts {
		if receipt.Status == "stale" {
			stale = receipt
			break
		}
	}
	if stale.Ref == "" {
		t.Fatal("stage 5 has no stale receipt")
	}
	strictMode := compactRouteLifecycleHistoricalProofMode{Strict: true, GitAvailable: true}
	if err := compactRouteLifecycleProofRepositoryErrorAtMode(root, registry.SourceRevision, stale, strictMode); err != nil {
		t.Fatalf("stale receipt outside current lineage was rejected: %v", err)
	}
}

func TestCompactRouteLifecycleHistoricalProofEnvironmentContract(t *testing.T) {
	t.Setenv(compactRouteLifecycleHistoricalProofEnv, "1")
	strictMode, err := compactRouteLifecycleDecideHistoricalProofMode(os.Getenv(compactRouteLifecycleHistoricalProofEnv) == "1", true, true)
	if err == nil || !strictMode.Strict || !strictMode.Shallow {
		t.Fatalf("strict environment contract produced mode=%+v err=%v", strictMode, err)
	}
	t.Setenv(compactRouteLifecycleHistoricalProofEnv, "0")
	defaultMode, err := compactRouteLifecycleDecideHistoricalProofMode(os.Getenv(compactRouteLifecycleHistoricalProofEnv) == "1", true, true)
	if err != nil || defaultMode.Strict || !defaultMode.Shallow {
		t.Fatalf("non-strict environment value produced mode=%+v err=%v", defaultMode, err)
	}
}

func TestCompactRouteLifecycleRejectsFabricatedProofCommit(t *testing.T) {
	registry := loadCompactRouteLifecycleRegistry(t)
	root, err := compactRouteLifecycleRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	receipt := registry.Entries[0].ProofReceipts[0]
	receipt.SourceRevision = strings.Repeat("0", 40)
	if err := compactRouteLifecycleProofReceiptErrorAt(root, registry.SourceRevision, receipt); err == nil {
		t.Fatal("fabricated proof commit was accepted")
	}
}

func TestCompactRouteLifecycleRejectsFabricatedProofPath(t *testing.T) {
	registry := loadCompactRouteLifecycleRegistry(t)
	root, err := compactRouteLifecycleRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	receipt := registry.Entries[0].ProofReceipts[1]
	receipt.Ref = "compact_route_campaign_lifecycle_test.go#TestCompactRouteCampaignLifecycleRegistry"
	if err := compactRouteLifecycleProofReceiptErrorAt(root, registry.SourceRevision, receipt); err == nil {
		t.Fatal("fabricated proof path was accepted")
	}
}

func TestCompactRouteLifecycleRejectsFabricatedProofAnchor(t *testing.T) {
	registry := loadCompactRouteLifecycleRegistry(t)
	root, err := compactRouteLifecycleRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	receipt := registry.Entries[0].ProofReceipts[1]
	receipt.Ref += "#fabricatedLifecycleAnchor"
	if err := compactRouteLifecycleProofReceiptErrorAt(root, registry.SourceRevision, receipt); err == nil {
		t.Fatal("fabricated proof anchor was accepted")
	}
}

func TestCompactRouteLifecycleRejectsEmbeddedCorpusHashMismatch(t *testing.T) {
	registry := loadCompactRouteLifecycleRegistry(t)
	corpus := registry.Entries[3].Corpus[0]
	corpus.Source = "[\\n"
	if err := compactRouteLifecycleEmbeddedCorpusHashError(corpus); err == nil {
		t.Fatal("embedded corpus hash mismatch was accepted")
	}
}

func TestCompactRouteLifecycleRejectsFileBackedCorpusHashMismatch(t *testing.T) {
	registry := loadCompactRouteLifecycleRegistry(t)
	root, err := compactRouteLifecycleRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	corpus := registry.Entries[4].Corpus[len(registry.Entries[4].Corpus)-1]
	corpus.SourceSHA256 = strings.Repeat("0", 64)
	if err := compactRouteLifecycleCorpusHashError(root, corpus); err == nil {
		t.Fatal("file-backed corpus hash mismatch was accepted")
	}
}

func TestCompactRouteLifecycleRejectsKnownCorpusTreeHashMismatch(t *testing.T) {
	registry := loadCompactRouteLifecycleRegistry(t)
	root, err := compactRouteLifecycleRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	corpus := registry.Entries[0].Corpus[0]
	corpus.TreeSHA256 = strings.Repeat("0", 64)
	if err := compactRouteLifecycleCorpusHashError(root, corpus); err == nil {
		t.Fatal("known corpus tree hash mismatch was accepted")
	}
}

func TestCompactRouteLifecycleRejectsCurrentReceiptForHistoricalStates(t *testing.T) {
	registry := loadCompactRouteLifecycleRegistry(t)
	receipt := registry.Entries[0].ProofReceipts[0]
	for _, status := range []string{"retired", "superseded"} {
		policy := compactRouteLifecycleReceiptPolicyForStatus(registry.HistoryPolicy.StatePolicy, status)
		if err := compactRouteLifecycleReceiptStateError(status, []compactRouteLifecycleReceipt{receipt}, policy); err == nil {
			t.Errorf("%s entry accepted a current receipt", status)
		}
		for _, receiptStatus := range []string{"stale", "historical"} {
			retained := receipt
			retained.Status = receiptStatus
			if err := compactRouteLifecycleReceiptStateError(status, []compactRouteLifecycleReceipt{retained}, policy); err != nil {
				t.Errorf("%s entry rejected a %s receipt: %v", status, receiptStatus, err)
			}
		}
	}
}

func TestCompactRouteLifecycleRejectsStaleReadSite(t *testing.T) {
	registry := loadCompactRouteLifecycleRegistry(t)
	root, err := compactRouteLifecycleRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	site := registry.Controls[0].ReadSites[0]
	t.Run("stale line", func(t *testing.T) {
		stale := site
		stale.Line = 1
		if err := compactRouteLifecycleReadSiteError(root, "negative", stale); err == nil {
			t.Fatal("stale read-site line was accepted")
		}
	})
	t.Run("stale symbol", func(t *testing.T) {
		stale := site
		stale.Symbol = "fabricatedLifecycleSymbol"
		if err := compactRouteLifecycleReadSiteError(root, "negative", stale); err == nil {
			t.Fatal("stale read-site symbol was accepted")
		}
	})
}

func TestCompactRouteLifecycleRejectsIncompleteBuildExpression(t *testing.T) {
	if err := parseCompactRouteLifecycleBuildExpression("gts_parsercorephase0 &&"); err == nil {
		t.Fatal("incomplete build expression was accepted")
	}
}

func TestCompactRouteLifecycleRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink setup is unavailable: %v", err)
	}
	if _, err := compactRouteLifecycleResolvePath(root, "escape", true); err == nil {
		t.Fatal("symlink escape was accepted")
	}
}

func TestCompactRouteLifecycleRejectsUnsafeCommandDestinations(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{name: "absolute output flag", command: "go test . --output /tmp/x"},
		{name: "absolute receipt assignment", command: "RECEIPT=/tmp/x go test ."},
		{name: "parent traversal", command: "go test . --output ../outside"},
		{name: "home expansion", command: "go test . --output ~/x"},
		{name: "unresolved variable", command: "go test . --output ${DEST}"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := compactRouteLifecycleCommandDestinationError(test.command); err == nil {
				t.Fatalf("unsafe command destination was accepted: %s", test.command)
			}
		})
	}
	if err := compactRouteLifecycleCommandDestinationError("go test . --output harness_out/result.json"); err != nil {
		t.Fatalf("repository-relative harness_out destination was rejected: %v", err)
	}
}

func TestCompactRouteLifecycleRejectsDuplicateJSONKeys(t *testing.T) {
	_, err := decodeCompactRouteLifecycleRegistry([]byte(`{"schema":"one","schema":"two"}`))
	if err == nil || !strings.Contains(err.Error(), "duplicate JSON key") {
		t.Fatalf("duplicate JSON keys were accepted: %v", err)
	}
}

func TestCompactRouteLifecycleRejectsPlannedReceiptLabel(t *testing.T) {
	registry := loadCompactRouteLifecycleRegistry(t)
	plan := registry.Entries[5].PlanRefs[0]
	mislabelled := compactRouteLifecycleReceipt{
		Ref:            plan.Ref,
		Kind:           plan.Kind,
		SourceRevision: plan.SourceRevision,
		Status:         "current",
	}
	if err := compactRouteLifecycleProofReceiptError(mislabelled); err == nil {
		t.Fatal("planned reference was accepted as a proof receipt")
	}
}

func loadCompactRouteLifecycleRegistry(t *testing.T) compactRouteLifecycleRegistry {
	t.Helper()
	data, err := os.ReadFile(compactRouteLifecycleRegistryPath)
	if err != nil {
		t.Fatalf("open %s: %v", compactRouteLifecycleRegistryPath, err)
	}
	registry, err := decodeCompactRouteLifecycleRegistry(data)
	if err != nil {
		t.Fatalf("decode %s: %v", compactRouteLifecycleRegistryPath, err)
	}
	return registry
}

func decodeCompactRouteLifecycleRegistry(data []byte) (compactRouteLifecycleRegistry, error) {
	if err := rejectCompactRouteLifecycleDuplicateKeys(data); err != nil {
		return compactRouteLifecycleRegistry{}, err
	}
	var registry compactRouteLifecycleRegistry
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&registry); err != nil {
		return compactRouteLifecycleRegistry{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return compactRouteLifecycleRegistry{}, fmt.Errorf("trailing JSON data: %v", err)
	}
	return registry, nil
}

func rejectCompactRouteLifecycleDuplicateKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := consumeCompactRouteLifecycleJSONValue(decoder, "$"); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON data: %v", err)
	}
	return nil
}

func consumeCompactRouteLifecycleJSONValue(decoder *json.Decoder, path string) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]bool)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key at %s is not a string", path)
			}
			if seen[key] {
				return fmt.Errorf("duplicate JSON key %q at %s", key, path)
			}
			seen[key] = true
			if err := consumeCompactRouteLifecycleJSONValue(decoder, path+"."+key); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return fmt.Errorf("object at %s is not closed", path)
		}
	case '[':
		index := 0
		for decoder.More() {
			if err := consumeCompactRouteLifecycleJSONValue(decoder, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
			index++
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return fmt.Errorf("array at %s is not closed", path)
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q at %s", delim, path)
	}
	return nil
}

func validateCompactRouteLifecycleFunnel(t *testing.T, registry compactRouteLifecycleRegistry) {
	t.Helper()
	wantStages := []int{1, 2, 3, 4, 5, 6, 7}
	if !equalCompactRouteLifecycleInts(registry.Funnel.OrderedStages, wantStages) {
		t.Errorf("ordered_stages = %v, want %v", registry.Funnel.OrderedStages, wantStages)
	}
	if registry.Funnel.ActiveCoreStage != 6 {
		t.Errorf("active_core_stage = %d, want 6", registry.Funnel.ActiveCoreStage)
	}
	if strings.TrimSpace(registry.Funnel.CoreRule) == "" {
		t.Error("funnel core_rule is empty")
	}
	if !registry.Funnel.CorrectnessPrecedesPerformance {
		t.Error("funnel must require correctness before performance")
	}
	validateStringVocabulary(t, "parallel_lanes", registry.Funnel.ParallelLanes, []string{"adversarial", "certification", "performance"})
}

func validateCompactRouteLifecycleGate(t *testing.T, registry compactRouteLifecycleRegistry) {
	t.Helper()
	want := []string{
		"smallest_c_oracle_falsifier",
		"focused_implementation",
		"one_core_implementation_stage_active",
		"exact_tree_range_error_assertions",
		"one_grammar_docker_parity",
		"cleanup_and_ownership_checks",
		"intended_path_telemetry",
		"no_unrelated_performance_work",
	}
	if !equalCompactRouteLifecycleStrings(registry.CorePRGate.RequiredFields, want) {
		t.Errorf("core_pr_gate.required_fields = %v, want %v", registry.CorePRGate.RequiredFields, want)
	}
	if strings.TrimSpace(registry.CorePRGate.PerformanceRule) == "" {
		t.Error("core_pr_gate.performance_rule is empty")
	}
	if len(registry.GraduationDefinition) < 6 {
		t.Errorf("graduation_definition has %d items, want at least 6", len(registry.GraduationDefinition))
	}
	for index, item := range registry.GraduationDefinition {
		if strings.TrimSpace(item) == "" {
			t.Errorf("graduation_definition[%d] is empty", index)
		}
	}
}

func validateCompactRouteLifecycleHistory(t *testing.T, registry compactRouteLifecycleRegistry) {
	t.Helper()
	if !registry.HistoryPolicy.PreserveEntries || !registry.HistoryPolicy.PreserveReceiptRefs || !registry.HistoryPolicy.PreservePlanRefs {
		t.Error("history policy must preserve entries, proof receipts, and plans")
	}
	if strings.TrimSpace(registry.HistoryPolicy.RetirementRule) == "" || strings.TrimSpace(registry.HistoryPolicy.StaleRule) == "" {
		t.Error("history policy requires retirement and stale rules")
	}
	validateCompactRouteLifecycleStatePolicy(t, registry)
}

func validateCompactRouteLifecycleStatePolicy(t *testing.T, registry compactRouteLifecycleRegistry) {
	t.Helper()
	policy := registry.HistoryPolicy.StatePolicy
	validateStringVocabulary(t, "state_policy.current_receipt_required", policy.CurrentReceiptRequired, []string{"active", "graduated"})
	validateStringVocabulary(t, "state_policy.historical_only", policy.HistoricalOnly, []string{"retired", "superseded"})
	validateStringVocabulary(t, "state_policy.receipts_optional", policy.ReceiptsOptional, []string{"planned"})
	sets := map[string][]string{
		compactRouteLifecycleReceiptPolicyCurrentRequired: policy.CurrentReceiptRequired,
		compactRouteLifecycleReceiptPolicyHistoricalOnly:  policy.HistoricalOnly,
		compactRouteLifecycleReceiptPolicyOptional:        policy.ReceiptsOptional,
	}
	seen := make(map[string]string)
	for policyName, statuses := range sets {
		for _, status := range statuses {
			if previous, ok := seen[status]; ok {
				t.Errorf("lifecycle state %q appears in state policies %q and %q", status, previous, policyName)
			}
			seen[status] = policyName
		}
	}
	for _, status := range registry.Vocabularies.Statuses {
		if _, ok := seen[status]; !ok {
			t.Errorf("lifecycle state %q has no state policy", status)
		}
	}
	for status := range seen {
		if !containsCompactRouteLifecycleString(registry.Vocabularies.Statuses, status) {
			t.Errorf("state policy names unknown lifecycle state %q", status)
		}
	}
}

func compactRouteLifecycleReceiptPolicyForStatus(policy compactRouteLifecycleStatePolicy, status string) string {
	if containsCompactRouteLifecycleString(policy.CurrentReceiptRequired, status) {
		return compactRouteLifecycleReceiptPolicyCurrentRequired
	}
	if containsCompactRouteLifecycleString(policy.HistoricalOnly, status) {
		return compactRouteLifecycleReceiptPolicyHistoricalOnly
	}
	if containsCompactRouteLifecycleString(policy.ReceiptsOptional, status) {
		return compactRouteLifecycleReceiptPolicyOptional
	}
	return ""
}

func validateCompactRouteLifecycleVocabularies(t *testing.T, registry compactRouteLifecycleRegistry) {
	t.Helper()
	validateStringVocabulary(t, "owners", registry.Vocabularies.Owners, nil)
	validateStringVocabulary(t, "statuses", registry.Vocabularies.Statuses, nil)
	validateStringVocabulary(t, "receipt_statuses", registry.Vocabularies.ReceiptStatuses, []string{"current", "stale", "historical"})
	validateStringVocabulary(t, "plan_statuses", registry.Vocabularies.PlanStatuses, []string{"active", "planned", "stale", "historical"})
	validateStringVocabulary(t, "retention", registry.Vocabularies.Retention, nil)
	validateStringVocabulary(t, "lanes", registry.Vocabularies.Lanes, []string{"core", "adversarial", "certification", "performance"})
	validateStringVocabulary(t, "control_kinds", registry.Vocabularies.ControlKinds, []string{"build_expression", "environment"})
	validateStringVocabulary(t, "path_roles", registry.Vocabularies.PathRoles, nil)
}

func validateStringVocabulary(t *testing.T, name string, values, required []string) {
	t.Helper()
	if len(values) == 0 {
		t.Errorf("%s is empty", name)
		return
	}
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			t.Errorf("%s contains an empty value", name)
		}
		if seen[value] {
			t.Errorf("%s contains duplicate value %q", name, value)
		}
		seen[value] = true
	}
	for _, value := range required {
		if !seen[value] {
			t.Errorf("%s does not contain required value %q", name, value)
		}
	}
}

func validateCompactRouteLifecycleControls(t *testing.T, registry compactRouteLifecycleRegistry, root string) map[string]compactRouteLifecycleControl {
	t.Helper()
	allowedScopes := map[string]bool{"production": true, "test": true, "harness": true}
	allowedKinds := compactRouteLifecycleSet(t, "control_kinds", registry.Vocabularies.ControlKinds)
	controls := make(map[string]compactRouteLifecycleControl, len(registry.Controls))
	environmentNames := make(map[string]bool)
	for _, control := range registry.Controls {
		if strings.TrimSpace(control.ID) == "" || controls[control.ID].ID != "" {
			t.Errorf("control IDs must be unique and non-empty: %q", control.ID)
			continue
		}
		if !allowedKinds[control.Kind] {
			t.Errorf("control %q has unsupported kind %q", control.ID, control.Kind)
		}
		if !allowedScopes[control.Scope] {
			t.Errorf("control %q has an unsupported scope %q", control.ID, control.Scope)
		}
		if control.Kind == "build_expression" && strings.TrimSpace(control.Expression) == "" {
			t.Errorf("build control %q has no expression", control.ID)
		} else if control.Kind == "build_expression" {
			if err := parseCompactRouteLifecycleBuildExpression(control.Expression); err != nil {
				t.Errorf("build control %q: %v", control.ID, err)
			}
		}
		if control.Kind == "environment" {
			if !compactRouteLifecycleEnvPattern.MatchString(control.Name) {
				t.Errorf("control %q has invalid environment name %q", control.ID, control.Name)
			}
			if control.ID != "env."+control.Name {
				t.Errorf("environment control %q does not match name %q", control.ID, control.Name)
			}
			if environmentNames[control.Name] {
				t.Errorf("environment name %q is duplicated", control.Name)
			}
			environmentNames[control.Name] = true
			if strings.TrimSpace(control.Default) == "" {
				t.Errorf("environment control %q has no default", control.ID)
			}
		}
		if len(control.ReadSites) == 0 {
			t.Errorf("control %q has no read sites", control.ID)
		}
		for _, site := range control.ReadSites {
			validateCompactRouteLifecycleReadSite(t, root, control.ID, site)
		}
		controls[control.ID] = control
	}
	return controls
}

func validateCompactRouteLifecycleReadSite(t *testing.T, root, owner string, site compactRouteLifecycleReadSite) {
	t.Helper()
	if err := compactRouteLifecycleReadSiteError(root, owner, site); err != nil {
		t.Error(err)
	}
}

func compactRouteLifecycleReadSiteError(root, owner string, site compactRouteLifecycleReadSite) error {
	if strings.TrimSpace(site.Path) == "" || strings.TrimSpace(site.Symbol) == "" || site.Line < 1 {
		return fmt.Errorf("%s read site has an empty path, symbol, or invalid line", owner)
	}
	resolved, err := compactRouteLifecycleResolvePath(root, site.Path, true)
	if err != nil {
		return fmt.Errorf("%s read site %s: %v", owner, site.Path, err)
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return fmt.Errorf("%s read site %s: %v", owner, site.Path, err)
	}
	lines := strings.Split(string(data), "\n")
	if site.Line > len(lines) {
		return fmt.Errorf("%s read site %s:%d exceeds %d lines", owner, site.Path, site.Line, len(lines))
	}
	lo := site.Line - 12
	if lo < 1 {
		lo = 1
	}
	hi := site.Line + 12
	if hi > len(lines) {
		hi = len(lines)
	}
	for _, line := range lines[lo-1 : hi] {
		if strings.Contains(line, site.Symbol) {
			return nil
		}
	}
	return fmt.Errorf("%s read site %s:%d does not contain symbol %q nearby", owner, site.Path, site.Line, site.Symbol)
}

func validateCompactRouteLifecycleEntries(t *testing.T, registry compactRouteLifecycleRegistry, campaigns map[string]bool, controls map[string]compactRouteLifecycleControl, root string, historicalProofMode compactRouteLifecycleHistoricalProofMode) {
	t.Helper()
	allowedStatuses := compactRouteLifecycleSet(t, "statuses", registry.Vocabularies.Statuses)
	allowedOwners := compactRouteLifecycleSet(t, "owners", registry.Vocabularies.Owners)
	allowedLanes := compactRouteLifecycleSet(t, "lanes", registry.Vocabularies.Lanes)
	allowedRoles := compactRouteLifecycleSet(t, "path_roles", registry.Vocabularies.PathRoles)
	allowedRetention := compactRouteLifecycleSet(t, "retention", registry.Vocabularies.Retention)
	allowedReceiptStatuses := compactRouteLifecycleSet(t, "receipt_statuses", registry.Vocabularies.ReceiptStatuses)
	allowedPlanStatuses := compactRouteLifecycleSet(t, "plan_statuses", registry.Vocabularies.PlanStatuses)

	want := map[int]struct {
		id        string
		status    string
		owner     string
		retention string
	}{
		1: {"stage-1-per-version-lexer", "graduated", "scanner_checkpoint_state", "preserve_receipt"},
		2: {"stage-2-physical-graph-head-merge", "graduated", "graph_head_lifecycle", "preserve_receipt"},
		3: {"stage-3-recovery-through-ambiguity", "graduated", "derivation_election_selection", "preserve_receipt"},
		4: {"stage-4-end-of-file-recovery", "graduated", "scheduler_action_semantics", "preserve_receipt"},
		5: {"stage-5-incremental-integration", "graduated", "incremental_edit_reuse", "preserve_receipt"},
		6: {"stage-6-grammar-certification", "active", "oracle_certification", "preserve_receipt"},
		7: {"stage-7-performance-and-release", "planned", "performance_release", "keep_emergency_fallback"},
	}
	seenStages := make(map[int]bool)
	seenIDs := make(map[string]bool)
	seenProofRefs := make(map[string]int, len(compactRouteLifecycleKnownProofRevisions))
	activeCoreStages := 0
	for _, entry := range registry.Entries {
		if strings.TrimSpace(entry.ID) == "" {
			t.Error("entry ID is empty")
			continue
		}
		spec, ok := want[entry.Stage]
		if !ok {
			t.Errorf("entry %q has unsupported stage %d", entry.ID, entry.Stage)
			continue
		}
		if seenIDs[entry.ID] {
			t.Errorf("entry IDs must be unique: %q", entry.ID)
		}
		seenIDs[entry.ID] = true
		if entry.ID == spec.id {
			if seenStages[entry.Stage] {
				t.Errorf("canonical stage %d is duplicated", entry.Stage)
			}
			seenStages[entry.Stage] = true
		}
		if entry.ID == spec.id && (entry.Status != spec.status || entry.Owner != spec.owner || entry.Retention != spec.retention) {
			t.Errorf("stage %d identity = %q/%q/%q/%q, want %q/%q/%q/%q", entry.Stage, entry.ID, entry.Status, entry.Owner, entry.Retention, spec.id, spec.status, spec.owner, spec.retention)
		}
		if entry.Lane != "core" || !allowedLanes[entry.Lane] {
			t.Errorf("stage %d lane = %q, want core", entry.Stage, entry.Lane)
		}
		if !allowedStatuses[entry.Status] || !allowedOwners[entry.Owner] || !allowedRetention[entry.Retention] {
			t.Errorf("stage %d uses a vocabulary value outside the registry", entry.Stage)
		}
		if strings.TrimSpace(entry.Purpose) == "" || strings.TrimSpace(entry.DeletionCondition) == "" || !entry.HistoryPreserved {
			t.Errorf("stage %d requires purpose, deletion condition, and history preservation", entry.Stage)
		}
		if len(entry.Campaigns) == 0 || len(entry.BuildExpressions) == 0 || len(entry.ControlIDs) == 0 || len(entry.Paths) == 0 || len(entry.TestCommands) == 0 || len(entry.Corpus) == 0 || len(entry.Gates) == 0 || len(entry.Telemetry) == 0 {
			t.Errorf("stage %d is missing a lifecycle section", entry.Stage)
		}
		receiptPolicy := compactRouteLifecycleReceiptPolicyForStatus(registry.HistoryPolicy.StatePolicy, entry.Status)
		if receiptPolicy == "" {
			t.Errorf("stage %d status %q has no receipt policy", entry.Stage, entry.Status)
		}
		if entry.Status == "planned" && len(entry.PlanRefs) == 0 {
			t.Errorf("planned stage %d has no plan references", entry.Stage)
		}
		if receiptPolicy == compactRouteLifecycleReceiptPolicyCurrentRequired && len(entry.ProofReceipts) == 0 {
			t.Errorf("stage %d status %q has no proof receipts", entry.Stage, entry.Status)
		}
		if entry.Status == "active" && entry.Lane == "core" {
			activeCoreStages++
		}
		validateCompactRouteLifecycleCampaigns(t, entry, campaigns)
		validateCompactRouteLifecycleBuildExpressions(t, entry)
		for _, controlID := range entry.ControlIDs {
			if _, ok := controls[controlID]; !ok {
				t.Errorf("stage %d references unknown control %q", entry.Stage, controlID)
			}
		}
		for _, path := range entry.Paths {
			if !allowedRoles[path.Role] {
				t.Errorf("stage %d path %q has unsupported role %q", entry.Stage, path.Path, path.Role)
			}
			if !containsCompactRouteLifecycleString(entry.BuildExpressions, path.BuildExpression) {
				t.Errorf("stage %d path %q uses unlisted build expression %q", entry.Stage, path.Path, path.BuildExpression)
			}
			validateCompactRouteLifecyclePath(t, root, entry.Stage, path)
		}
		validateCompactRouteLifecycleCommands(t, root, entry)
		validateCompactRouteLifecycleCorpus(t, root, entry)
		staleReceipts := validateCompactRouteLifecycleReceipts(t, root, registry.SourceRevision, entry, allowedReceiptStatuses, receiptPolicy, historicalProofMode)
		for _, receipt := range entry.ProofReceipts {
			if previousStage, ok := seenProofRefs[receipt.Ref]; ok {
				t.Errorf("proof receipt %q is duplicated in stages %d and %d", receipt.Ref, previousStage, entry.Stage)
			}
			seenProofRefs[receipt.Ref] = entry.Stage
		}
		validateCompactRouteLifecyclePlans(t, root, entry, allowedPlanStatuses)
		if receiptPolicy == compactRouteLifecycleReceiptPolicyOptional && staleReceipts == 0 && len(entry.ProofReceipts) > 0 {
			t.Errorf("planned stage %d must preserve stale proof evidence", entry.Stage)
		}
	}
	for stage := 1; stage <= 7; stage++ {
		if !seenStages[stage] {
			t.Errorf("stage %d is missing", stage)
		}
	}
	if activeCoreStages != 1 {
		t.Errorf("active core stages = %d, want 1", activeCoreStages)
	}
	for ref := range compactRouteLifecycleKnownProofRevisions {
		if _, ok := seenProofRefs[ref]; !ok {
			t.Errorf("immutable proof allowlist entry %q is missing from the registry", ref)
		}
	}
	if len(seenProofRefs) != len(compactRouteLifecycleKnownProofRevisions) {
		t.Errorf("proof receipt set has %d entries, want immutable allowlist size %d", len(seenProofRefs), len(compactRouteLifecycleKnownProofRevisions))
	}
}

func validateCompactRouteLifecycleCampaigns(t *testing.T, entry compactRouteLifecycleEntry, campaigns map[string]bool) {
	t.Helper()
	seen := make(map[string]bool, len(entry.Campaigns))
	for _, campaign := range entry.Campaigns {
		if !campaigns[campaign] {
			t.Errorf("stage %d references unknown campaign %q", entry.Stage, campaign)
		}
		if seen[campaign] {
			t.Errorf("stage %d repeats campaign %q", entry.Stage, campaign)
		}
		seen[campaign] = true
	}
}

func validateCompactRouteLifecycleBuildExpressions(t *testing.T, entry compactRouteLifecycleEntry) {
	t.Helper()
	seen := make(map[string]bool, len(entry.BuildExpressions))
	for _, expression := range entry.BuildExpressions {
		if strings.TrimSpace(expression) == "" || seen[expression] {
			t.Errorf("stage %d has an empty or duplicate build expression %q", entry.Stage, expression)
		} else if err := parseCompactRouteLifecycleBuildExpression(expression); err != nil {
			t.Errorf("stage %d build expression %q: %v", entry.Stage, expression, err)
		}
		seen[expression] = true
	}
}

func validateCompactRouteLifecycleCommands(t *testing.T, root string, entry compactRouteLifecycleEntry) {
	t.Helper()
	allowedIsolation := map[string]bool{"local": true, "docker": true, "quiet_host": true, "spot_vm_optional": true}
	hasDocker := false
	for _, command := range entry.TestCommands {
		if strings.TrimSpace(command.Name) == "" || strings.TrimSpace(command.Command) == "" || !allowedIsolation[command.Isolation] {
			t.Errorf("stage %d has an incomplete test command %q", entry.Stage, command.Name)
		}
		if command.Isolation == "docker" {
			hasDocker = true
		}
		if err := compactRouteLifecycleCommandDestinationError(command.Command); err != nil {
			t.Errorf("stage %d command %q: %v", entry.Stage, command.Name, err)
		}
		if !validateCompactRouteLifecyclePathName(t, root, fmt.Sprintf("stage %d command", entry.Stage), command.WorkingDir, true) {
			continue
		}
	}
	if !hasDocker {
		t.Errorf("stage %d has no isolated Docker command", entry.Stage)
	}
}

func compactRouteLifecycleCommandDestinationError(command string) error {
	tokens, err := compactRouteLifecycleCommandTokens(command)
	if err != nil {
		return err
	}
	for index := 0; index < len(tokens); index++ {
		token := tokens[index]
		if name, value, ok := compactRouteLifecycleAssignmentToken(token); ok {
			if compactRouteLifecycleDestinationVariable(name) {
				if err := compactRouteLifecycleDestinationPathError(value); err != nil {
					return fmt.Errorf("environment destination %s: %v", name, err)
				}
			}
			continue
		}
		flag, value, takesValue, ok := compactRouteLifecycleDestinationFlag(token)
		if !ok {
			continue
		}
		if !takesValue {
			if index+1 >= len(tokens) {
				return fmt.Errorf("destination flag %s has no value", flag)
			}
			index++
			value = tokens[index]
		}
		if err := compactRouteLifecycleDestinationPathError(value); err != nil {
			return fmt.Errorf("destination flag %s: %v", flag, err)
		}
	}
	return nil
}

func compactRouteLifecycleAssignmentToken(token string) (name, value string, ok bool) {
	separator := strings.IndexByte(token, '=')
	if separator <= 0 {
		return "", "", false
	}
	name = token[:separator]
	if !compactRouteLifecycleEnvPattern.MatchString(name) {
		return "", "", false
	}
	return name, token[separator+1:], true
}

func compactRouteLifecycleDestinationVariable(name string) bool {
	name = strings.ToUpper(name)
	return name == "RECEIPT" || strings.Contains(name, "RECEIPT") ||
		name == "OUTPUT" || strings.Contains(name, "OUTPUT") ||
		name == "OUT" || strings.HasSuffix(name, "_OUT") || strings.HasSuffix(name, "_DIR")
}

func compactRouteLifecycleDestinationFlag(token string) (flag, value string, takesValue, ok bool) {
	if !strings.HasPrefix(token, "-") {
		return "", "", false, false
	}
	separator := strings.IndexByte(token, '=')
	flag = token
	if separator >= 0 {
		flag = token[:separator]
		value = token[separator+1:]
		takesValue = true
	}
	name := strings.ToLower(strings.TrimLeft(flag, "-"))
	if name == "" || (!strings.Contains(name, "output") && !strings.Contains(name, "receipt") && name != "out" && !strings.HasPrefix(name, "out-") && !strings.HasSuffix(name, "-dir")) {
		return "", "", false, false
	}
	return flag, value, takesValue, true
}

func compactRouteLifecycleDestinationPathError(destination string) error {
	if destination == "" || strings.TrimSpace(destination) != destination {
		return fmt.Errorf("destination %q is empty or contains whitespace", destination)
	}
	if strings.ContainsAny(destination, "\x00$%`'\";|&<>()") {
		return fmt.Errorf("destination %q contains unsupported shell syntax or an unresolved variable", destination)
	}
	if strings.HasPrefix(destination, "~") {
		return fmt.Errorf("destination %q uses home expansion", destination)
	}
	if strings.Contains(destination, "\\") {
		return fmt.Errorf("destination %q uses unsupported backslash path syntax", destination)
	}
	if len(destination) >= 3 && ((destination[0] >= 'A' && destination[0] <= 'Z') || (destination[0] >= 'a' && destination[0] <= 'z')) && destination[1] == ':' && destination[2] == '/' {
		return fmt.Errorf("destination %q is an absolute Windows path", destination)
	}
	if strings.HasPrefix(destination, "//") {
		return fmt.Errorf("destination %q is an absolute network path", destination)
	}
	if strings.Contains(destination, "=") {
		return fmt.Errorf("destination %q is not a plain path", destination)
	}
	if strings.HasPrefix(destination, "-") {
		return fmt.Errorf("destination %q is another command flag", destination)
	}
	if err := compactRouteLifecyclePathSyntaxError(destination); err != nil {
		return err
	}
	return nil
}

func compactRouteLifecycleCommandTokens(command string) ([]string, error) {
	var tokens []string
	var token strings.Builder
	inSingle, inDouble, escaped, started := false, false, false, false
	flush := func() {
		if started {
			tokens = append(tokens, token.String())
			token.Reset()
			started = false
		}
	}
	for _, character := range command {
		if escaped {
			token.WriteRune(character)
			started = true
			escaped = false
			continue
		}
		if character == '\\' && !inSingle {
			escaped = true
			started = true
			continue
		}
		if character == '\'' && !inDouble {
			inSingle = !inSingle
			started = true
			continue
		}
		if character == '"' && !inSingle {
			inDouble = !inDouble
			started = true
			continue
		}
		if (character == ' ' || character == '\t' || character == '\n' || character == '\r') && !inSingle && !inDouble {
			flush()
			continue
		}
		token.WriteRune(character)
		started = true
	}
	if escaped || inSingle || inDouble {
		return nil, fmt.Errorf("command has an incomplete quoted or escaped token")
	}
	flush()
	return tokens, nil
}

func validateCompactRouteLifecycleCorpus(t *testing.T, root string, entry compactRouteLifecycleEntry) {
	t.Helper()
	seen := make(map[string]bool, len(entry.Corpus))
	for _, corpus := range entry.Corpus {
		if strings.TrimSpace(corpus.ID) == "" || seen[corpus.ID] {
			t.Errorf("stage %d corpus IDs must be unique and non-empty: %q", entry.Stage, corpus.ID)
		}
		seen[corpus.ID] = true
		if strings.TrimSpace(corpus.Kind) == "" || strings.TrimSpace(corpus.Path) == "" {
			t.Errorf("stage %d corpus %q needs kind and path", entry.Stage, corpus.ID)
		}
		unavailable := corpus.Availability == "unavailable"
		if unavailable {
			if corpus.LockStatus != "unlocked" || strings.TrimSpace(corpus.Plan) == "" || corpus.LockOrDigest != "" || corpus.SourceSHA256 != "" || corpus.TreeSHA256 != "" {
				t.Errorf("stage %d unavailable corpus %q needs unlocked plan metadata without digests", entry.Stage, corpus.ID)
			}
			validateCompactRouteLifecyclePathName(t, root, fmt.Sprintf("stage %d unavailable corpus", entry.Stage), corpus.Path, false)
		} else {
			if corpus.LockStatus != "" && corpus.LockStatus != "locked" {
				t.Errorf("stage %d corpus %q has unsupported lock_status %q", entry.Stage, corpus.ID, corpus.LockStatus)
			}
			if strings.TrimSpace(corpus.LockOrDigest) == "" {
				t.Errorf("stage %d locked corpus %q needs lock_or_digest", entry.Stage, corpus.ID)
			}
			validateCompactRouteLifecyclePathName(t, root, fmt.Sprintf("stage %d corpus", entry.Stage), corpus.Path, true)
		}
		if corpus.SourceSHA256 != "" && !compactRouteLifecycleSHA256Pattern.MatchString(corpus.SourceSHA256) {
			t.Errorf("stage %d corpus %q has invalid source_sha256", entry.Stage, corpus.ID)
		}
		if err := compactRouteLifecycleCorpusHashError(root, corpus); err != nil {
			t.Errorf("stage %d: %v", entry.Stage, err)
		}
		if corpus.TreeSHA256 != "" && !compactRouteLifecycleSHA256Pattern.MatchString(corpus.TreeSHA256) {
			t.Errorf("stage %d corpus %q has invalid tree_sha256", entry.Stage, corpus.ID)
		}
	}
}

func compactRouteLifecycleEmbeddedCorpusHashError(corpus compactRouteLifecycleCorpus) error {
	if corpus.SourceSHA256 == "" {
		return nil
	}
	if corpus.Source == "" {
		return nil
	}
	if !compactRouteLifecycleSHA256Pattern.MatchString(corpus.SourceSHA256) {
		return fmt.Errorf("corpus %q has invalid source_sha256", corpus.ID)
	}
	got := fmt.Sprintf("%x", sha256.Sum256([]byte(corpus.Source)))
	if got != corpus.SourceSHA256 {
		return fmt.Errorf("corpus %q source_sha256 = %s, want %s for the embedded source bytes", corpus.ID, corpus.SourceSHA256, got)
	}
	return nil
}

func compactRouteLifecycleCorpusHashError(root string, corpus compactRouteLifecycleCorpus) error {
	if err := compactRouteLifecycleEmbeddedCorpusHashError(corpus); err != nil {
		return err
	}
	if corpus.SourceSHA256 != "" && corpus.Source == "" {
		resolved, err := compactRouteLifecycleResolvePath(root, corpus.Path, true)
		if err != nil {
			return fmt.Errorf("corpus %q source hash path: %v", corpus.ID, err)
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			return fmt.Errorf("corpus %q source hash path %q: %v", corpus.ID, corpus.Path, err)
		}
		got := fmt.Sprintf("%x", sha256.Sum256(data))
		if got != corpus.SourceSHA256 {
			return fmt.Errorf("corpus %q source_sha256 = %s, want %s for %s", corpus.ID, corpus.SourceSHA256, got, corpus.Path)
		}
	}
	if corpus.TreeSHA256 != "" {
		want, ok := compactRouteLifecycleKnownCorpusTreeSHA256[corpus.ID]
		if !ok {
			return fmt.Errorf("corpus %q has no authenticated tree_sha256", corpus.ID)
		}
		if corpus.TreeSHA256 != want {
			return fmt.Errorf("corpus %q tree_sha256 = %s, want authenticated %s", corpus.ID, corpus.TreeSHA256, want)
		}
	}
	return nil
}

func validateCompactRouteLifecyclePath(t *testing.T, root string, stage int, path compactRouteLifecyclePath) {
	t.Helper()
	if !validateCompactRouteLifecyclePathName(t, root, fmt.Sprintf("stage %d path", stage), path.Path, true) {
		return
	}
	resolved, err := compactRouteLifecycleResolvePath(root, path.Path, true)
	if err != nil {
		t.Errorf("stage %d path %q: %v", stage, path.Path, err)
		return
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		t.Errorf("stage %d path %q: %v", stage, path.Path, err)
		return
	}
	if strings.TrimSpace(path.Symbol) == "" || !strings.Contains(string(data), path.Symbol) {
		t.Errorf("stage %d path %q does not contain symbol %q", stage, path.Path, path.Symbol)
	}
	actual := compactRouteLifecycleFileBuildExpression(data)
	if actual != path.BuildExpression {
		t.Errorf("stage %d path %q build expression = %q, want %q", stage, path.Path, path.BuildExpression, actual)
	}
}

func validateCompactRouteLifecyclePlans(t *testing.T, root string, entry compactRouteLifecycleEntry, allowedStatuses map[string]bool) {
	t.Helper()
	seen := make(map[string]bool, len(entry.PlanRefs))
	allowedKinds := map[string]bool{"plan": true, "runbook": true}
	for _, plan := range entry.PlanRefs {
		if strings.TrimSpace(plan.Ref) == "" || seen[plan.Ref] {
			t.Errorf("stage %d plan refs must be unique and non-empty: %q", entry.Stage, plan.Ref)
		}
		seen[plan.Ref] = true
		if !allowedKinds[plan.Kind] || !allowedStatuses[plan.Status] || !compactRouteLifecycleCommitPattern.MatchString(plan.SourceRevision) {
			t.Errorf("stage %d plan %q has an invalid kind, status, or source revision", entry.Stage, plan.Ref)
		}
		if plan.Status == "active" && strings.HasPrefix(plan.Ref, "pr:") {
			t.Errorf("active stage %d cannot claim an unverified active PR", entry.Stage)
		}
		if !validateCompactRouteLifecycleReferencePath(t, root, fmt.Sprintf("stage %d plan", entry.Stage), plan.Ref) {
			continue
		}
	}
}

func validateCompactRouteLifecycleReceipts(t *testing.T, root, registrySourceRevision string, entry compactRouteLifecycleEntry, allowedStatuses map[string]bool, receiptPolicy string, historicalProofMode compactRouteLifecycleHistoricalProofMode) int {
	t.Helper()
	allowedKinds := map[string]bool{"merge": true, "test": true, "document": true, "policy": true}
	seen := make(map[string]bool, len(entry.ProofReceipts))
	stale := 0
	for _, receipt := range entry.ProofReceipts {
		if strings.TrimSpace(receipt.Ref) == "" || seen[receipt.Ref] {
			t.Errorf("stage %d proof receipts must be unique and non-empty: %q", entry.Stage, receipt.Ref)
		}
		seen[receipt.Ref] = true
		if !allowedKinds[receipt.Kind] || !allowedStatuses[receipt.Status] || !compactRouteLifecycleCommitPattern.MatchString(receipt.SourceRevision) {
			t.Errorf("stage %d proof receipt %q has an invalid kind, status, or source revision", entry.Stage, receipt.Ref)
		}
		if err := compactRouteLifecycleProofReceiptErrorAtMode(root, registrySourceRevision, receipt, historicalProofMode); err != nil {
			t.Errorf("stage %d: %v", entry.Stage, err)
		}
		if entry.Status == "active" && strings.HasPrefix(receipt.Ref, "pr:") {
			t.Errorf("active stage %d cannot claim an unverified active PR", entry.Stage)
		}
		switch receipt.Status {
		case "stale":
			stale++
			if receipt.SourceRevision == registrySourceRevision {
				t.Errorf("stage %d stale proof receipt %q points at the registry revision", entry.Stage, receipt.Ref)
			}
		}
	}
	if err := compactRouteLifecycleReceiptStateError(entry.Status, entry.ProofReceipts, receiptPolicy); err != nil {
		t.Errorf("stage %d: %v", entry.Stage, err)
	}
	return stale
}

func compactRouteLifecycleReceiptStateError(status string, receipts []compactRouteLifecycleReceipt, receiptPolicy string) error {
	current := 0
	for _, receipt := range receipts {
		if receipt.Status == "current" {
			current++
		}
	}
	switch receiptPolicy {
	case compactRouteLifecycleReceiptPolicyCurrentRequired:
		if current == 0 {
			return fmt.Errorf("status %q requires at least one current proof receipt", status)
		}
	case compactRouteLifecycleReceiptPolicyHistoricalOnly:
		for _, receipt := range receipts {
			if receipt.Status != "stale" && receipt.Status != "historical" {
				return fmt.Errorf("status %q may retain only stale or historical proof receipts, got %q for %q", status, receipt.Status, receipt.Ref)
			}
		}
	case compactRouteLifecycleReceiptPolicyOptional:
		return nil
	default:
		return fmt.Errorf("status %q has unknown receipt policy %q", status, receiptPolicy)
	}
	return nil
}

func compactRouteLifecycleProofReceiptError(receipt compactRouteLifecycleReceipt) error {
	root, err := compactRouteLifecycleRepositoryRoot()
	if err != nil {
		return err
	}
	return compactRouteLifecycleProofReceiptErrorAt(root, compactRouteLifecycleSourceRevision, receipt)
}

func compactRouteLifecycleProofReceiptErrorAt(root, registrySourceRevision string, receipt compactRouteLifecycleReceipt) error {
	historicalProofMode, err := compactRouteLifecycleHistoricalProofModeForRepository(root)
	if err != nil {
		return err
	}
	return compactRouteLifecycleProofReceiptErrorAtMode(root, registrySourceRevision, receipt, historicalProofMode)
}

func compactRouteLifecycleProofReceiptErrorAtMode(root, registrySourceRevision string, receipt compactRouteLifecycleReceipt, historicalProofMode compactRouteLifecycleHistoricalProofMode) error {
	switch receipt.Kind {
	case "merge", "test", "document", "policy":
	default:
		return fmt.Errorf("proof receipt %q has non-proof kind %q", receipt.Ref, receipt.Kind)
	}
	if !compactRouteLifecycleCommitPattern.MatchString(receipt.SourceRevision) {
		return fmt.Errorf("proof receipt %q has invalid source revision", receipt.Ref)
	}
	if err := compactRouteLifecycleProofRepositoryErrorAtMode(root, registrySourceRevision, receipt, historicalProofMode); err != nil {
		return err
	}
	want, ok := compactRouteLifecycleKnownProofRevisions[receipt.Ref]
	if !ok {
		return fmt.Errorf("proof receipt %q is not an authenticated registry receipt", receipt.Ref)
	}
	if want != receipt.SourceRevision {
		return fmt.Errorf("proof receipt %q has revision %q, want authenticated %q", receipt.Ref, receipt.SourceRevision, want)
	}
	return nil
}

func compactRouteLifecycleProofRepositoryError(root, registrySourceRevision string, receipt compactRouteLifecycleReceipt) error {
	historicalProofMode, err := compactRouteLifecycleHistoricalProofModeForRepository(root)
	if err != nil {
		return err
	}
	return compactRouteLifecycleProofRepositoryErrorAtMode(root, registrySourceRevision, receipt, historicalProofMode)
}

func compactRouteLifecycleProofRepositoryErrorAtMode(root, registrySourceRevision string, receipt compactRouteLifecycleReceipt, historicalProofMode compactRouteLifecycleHistoricalProofMode) error {
	if !historicalProofMode.GitAvailable {
		if historicalProofMode.Strict {
			return fmt.Errorf("%s=1 requires a non-shallow Git repository", compactRouteLifecycleHistoricalProofEnv)
		}
		return nil
	}
	if historicalProofMode.Strict && historicalProofMode.Shallow {
		return fmt.Errorf("%s=1 requires a non-shallow Git repository", compactRouteLifecycleHistoricalProofEnv)
	}
	if historicalProofMode.Strict {
		if err := compactRouteLifecycleRequireCommit(root, registrySourceRevision, "registry source revision"); err != nil {
			return err
		}
		if err := compactRouteLifecycleRequireCommit(root, receipt.SourceRevision, fmt.Sprintf("proof receipt %q", receipt.Ref)); err != nil {
			return err
		}
		if receipt.Status != "stale" && receipt.Status != "historical" {
			if _, err := compactRouteLifecycleGitOutput(root, "merge-base", "--is-ancestor", receipt.SourceRevision, registrySourceRevision); err != nil {
				return fmt.Errorf("proof receipt %q revision %s is not an ancestor of registry source %s: %w", receipt.Ref, receipt.SourceRevision, registrySourceRevision, err)
			}
		}
	} else {
		if !compactRouteLifecycleCommitObjectAvailable(root, receipt.SourceRevision) {
			return nil
		}
		if receipt.Status != "stale" && receipt.Status != "historical" && compactRouteLifecycleCommitObjectAvailable(root, registrySourceRevision) {
			if _, err := compactRouteLifecycleGitOutput(root, "merge-base", "--is-ancestor", receipt.SourceRevision, registrySourceRevision); err != nil {
				return fmt.Errorf("proof receipt %q revision %s is not an ancestor of registry source %s: %w", receipt.Ref, receipt.SourceRevision, registrySourceRevision, err)
			}
		}
	}

	if strings.HasPrefix(receipt.Ref, "pr:") {
		prNumber, err := compactRouteLifecyclePullRequestNumber(receipt.Ref)
		if err != nil {
			return fmt.Errorf("proof receipt %q: %v", receipt.Ref, err)
		}
		if err := compactRouteLifecyclePullRequestCommitMessageError(root, receipt.SourceRevision, prNumber); err != nil {
			return fmt.Errorf("proof receipt %q: %v", receipt.Ref, err)
		}
		// A pull-request receipt has no local path or blob anchor. Its immutable
		// allowlist entry still requires the referenced merge commit and message.
		return nil
	}
	parts := strings.SplitN(receipt.Ref, "#", 2)
	blob, err := compactRouteLifecycleHistoricalBlob(root, receipt.SourceRevision, receipt.Ref)
	if err != nil {
		return err
	}
	if len(parts) == 2 {
		if strings.TrimSpace(parts[1]) == "" {
			return fmt.Errorf("proof receipt %q has an empty historical blob anchor", receipt.Ref)
		}
		if !bytes.Contains(blob, []byte(parts[1])) {
			return fmt.Errorf("proof receipt %q has no anchor %q in the historical blob at %s", receipt.Ref, parts[1], receipt.SourceRevision)
		}
	}
	return nil
}

func compactRouteLifecyclePullRequestNumber(reference string) (string, error) {
	match := compactRouteLifecyclePRRefPattern.FindStringSubmatch(reference)
	if len(match) != 2 {
		return "", fmt.Errorf("pull-request receipt must use the pr:#N form")
	}
	return match[1], nil
}

func compactRouteLifecyclePullRequestCommitMessageError(root, revision, number string) error {
	message, err := compactRouteLifecycleGitOutput(root, "show", "-s", "--format=%s%n%b", revision)
	if err != nil {
		return fmt.Errorf("cannot read the pull-request commit message at %s: %w", revision, err)
	}
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(^|[^0-9])(?:merge[[:space:]]+)?pull[[:space:]]+request[[:space:]]*#` + regexp.QuoteMeta(number) + `([^0-9]|$)`),
		regexp.MustCompile(`(?i)(^|[^0-9])\(#` + regexp.QuoteMeta(number) + `\)([^0-9]|$)`),
	}
	for _, pattern := range patterns {
		if pattern.Match(message) {
			return nil
		}
	}
	return fmt.Errorf("commit message at %s does not identify pull request #%s", revision, number)
}

func compactRouteLifecycleValidateRegistryBase(root, revision string, historicalProofMode compactRouteLifecycleHistoricalProofMode) error {
	if !historicalProofMode.GitAvailable {
		if historicalProofMode.Strict {
			return fmt.Errorf("%s=1 requires a non-shallow Git repository", compactRouteLifecycleHistoricalProofEnv)
		}
		return nil
	}
	if historicalProofMode.Strict && historicalProofMode.Shallow {
		return fmt.Errorf("%s=1 requires a non-shallow Git repository", compactRouteLifecycleHistoricalProofEnv)
	}
	if err := compactRouteLifecycleRequireCommit(root, revision, "registry source revision"); err != nil && historicalProofMode.Strict {
		return err
	}
	return nil
}

func compactRouteLifecycleCommitObjectAvailable(root, revision string) bool {
	if !compactRouteLifecycleCommitPattern.MatchString(revision) {
		return false
	}
	_, err := compactRouteLifecycleGitOutput(root, "cat-file", "-e", revision+"^{commit}")
	return err == nil
}

func compactRouteLifecycleHistoricalProofModeForRepository(root string) (compactRouteLifecycleHistoricalProofMode, error) {
	strict := os.Getenv(compactRouteLifecycleHistoricalProofEnv) == "1"
	output, err := compactRouteLifecycleGitOutput(root, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return compactRouteLifecycleDecideHistoricalProofMode(strict, false, false)
	}
	var shallow bool
	switch strings.TrimSpace(string(output)) {
	case "true":
		shallow = true
	case "false":
	default:
		if strict {
			return compactRouteLifecycleHistoricalProofMode{}, fmt.Errorf("%s=1 requires Git shallow-repository detection", compactRouteLifecycleHistoricalProofEnv)
		}
		return compactRouteLifecycleDecideHistoricalProofMode(false, false, false)
	}
	return compactRouteLifecycleDecideHistoricalProofMode(strict, shallow, true)
}

func compactRouteLifecycleDecideHistoricalProofMode(strict, shallow, gitAvailable bool) (compactRouteLifecycleHistoricalProofMode, error) {
	mode := compactRouteLifecycleHistoricalProofMode{Strict: strict, Shallow: shallow, GitAvailable: gitAvailable}
	if strict && (!gitAvailable || shallow) {
		return mode, fmt.Errorf("%s=1 requires a non-shallow Git repository", compactRouteLifecycleHistoricalProofEnv)
	}
	return mode, nil
}

func compactRouteLifecycleRequireCommit(root, revision, owner string) error {
	if !compactRouteLifecycleCommitPattern.MatchString(revision) {
		return fmt.Errorf("%s has invalid commit revision %q", owner, revision)
	}
	if _, err := compactRouteLifecycleGitOutput(root, "cat-file", "-e", revision+"^{commit}"); err != nil {
		return fmt.Errorf("%s %s does not exist as a commit: %w", owner, revision, err)
	}
	return nil
}

func compactRouteLifecycleHistoricalBlob(root, revision, reference string) ([]byte, error) {
	parts := strings.SplitN(reference, "#", 2)
	refPath := parts[0]
	if err := compactRouteLifecyclePathSyntaxError(refPath); err != nil {
		return nil, fmt.Errorf("proof receipt %q: %v", reference, err)
	}
	object := revision + ":" + refPath
	typ, err := compactRouteLifecycleGitOutput(root, "cat-file", "-t", object)
	if err != nil {
		return nil, fmt.Errorf("proof receipt %q path %q is absent at %s: %w", reference, refPath, revision, err)
	}
	if strings.TrimSpace(string(typ)) != "blob" {
		return nil, fmt.Errorf("proof receipt %q path %q at %s is %q, want blob", reference, refPath, revision, strings.TrimSpace(string(typ)))
	}
	blob, err := compactRouteLifecycleGitOutput(root, "show", object)
	if err != nil {
		return nil, fmt.Errorf("read proof receipt %q historical blob at %s: %w", reference, revision, err)
	}
	return blob, nil
}

func compactRouteLifecycleGitOutput(root string, args ...string) ([]byte, error) {
	commandArgs := append([]string{"-C", root}, args...)
	command := exec.Command("git", commandArgs...)
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func validateCompactRouteLifecycleReferencePath(t *testing.T, root, owner, reference string) bool {
	t.Helper()
	parts := strings.SplitN(reference, "#", 2)
	path := parts[0]
	if strings.HasPrefix(path, "pr:") {
		if _, err := compactRouteLifecyclePullRequestNumber(reference); err != nil {
			t.Errorf("%s reference %q: %v", owner, reference, err)
			return false
		}
		return true
	}
	if !validateCompactRouteLifecyclePathName(t, root, owner, path, true) {
		return false
	}
	resolved, err := compactRouteLifecycleResolvePath(root, path, true)
	if err != nil {
		t.Errorf("%s reference %q: %v", owner, reference, err)
		return false
	}
	if _, err := os.Stat(resolved); err != nil {
		t.Errorf("%s reference %q: %v", owner, reference, err)
		return false
	}
	if len(parts) == 2 {
		data, err := os.ReadFile(resolved)
		if err != nil || !strings.Contains(string(data), parts[1]) {
			t.Errorf("%s reference %q has no matching anchor", owner, reference)
			return false
		}
	}
	return true
}

func validateCompactRouteLifecyclePathName(t *testing.T, root, owner, name string, mustExist bool) bool {
	t.Helper()
	if err := compactRouteLifecyclePathSyntaxError(name); err != nil {
		t.Errorf("%s: %v", owner, err)
		return false
	}
	if _, err := compactRouteLifecycleResolvePath(root, name, mustExist); err != nil {
		t.Errorf("%s path %q: %v", owner, name, err)
		return false
	}
	return true
}

func compactRouteLifecyclePathSyntaxError(name string) error {
	if strings.TrimSpace(name) == "" || strings.ContainsRune(name, 0) || strings.HasPrefix(name, "external:") || filepath.IsAbs(name) {
		return fmt.Errorf("path %q is absolute, external, empty, or contains a NUL", name)
	}
	clean := filepath.Clean(name)
	if clean != name || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return fmt.Errorf("path %q is non-local", name)
	}
	return nil
}

func compactRouteLifecycleRepositoryRoot() (string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get lifecycle registry working directory: %w", err)
	}
	return filepath.EvalSymlinks(workingDir)
}

func compactRouteLifecycleResolvePath(root, name string, mustExist bool) (string, error) {
	if strings.TrimSpace(name) == "" || strings.HasPrefix(name, "external:") || filepath.IsAbs(name) {
		return "", fmt.Errorf("path is absolute or external")
	}
	clean := filepath.Clean(name)
	if clean != name || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes the repository")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	candidate := filepath.Join(resolvedRoot, clean)
	if mustExist {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			return "", err
		}
		if !compactRouteLifecycleWithinRoot(resolvedRoot, resolved) {
			return "", fmt.Errorf("resolved path escapes the repository")
		}
		return resolved, nil
	}
	ancestor := candidate
	for {
		if _, err := os.Lstat(ancestor); err == nil {
			resolved, err := filepath.EvalSymlinks(ancestor)
			if err != nil {
				return "", err
			}
			if !compactRouteLifecycleWithinRoot(resolvedRoot, resolved) {
				return "", fmt.Errorf("resolved path escapes the repository")
			}
			return candidate, nil
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", fmt.Errorf("path has no repository ancestor")
		}
		ancestor = parent
	}
}

func compactRouteLifecycleWithinRoot(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func parseCompactRouteLifecycleBuildExpression(expression string) error {
	expression = strings.TrimSpace(expression)
	if expression == "default" {
		return nil
	}
	if expression == "" {
		return fmt.Errorf("empty build expression")
	}
	parts := strings.Split(expression, "&&")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return fmt.Errorf("incomplete build expression %q", expression)
		}
		part = strings.TrimPrefix(part, "!")
		if !compactRouteLifecycleBuildTagPattern.MatchString(part) {
			return fmt.Errorf("unsupported build atom %q", part)
		}
	}
	return nil
}

func compactRouteLifecycleFileBuildExpression(data []byte) string {
	for _, line := range strings.Split(string(data), "\n")[:minCompactRouteLifecycleLines(20, 1+bytes.Count(data, []byte("\n")))] {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "//go:build ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "//go:build "))
		}
	}
	return "default"
}

func minCompactRouteLifecycleLines(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func compactRouteLifecycleSet(t *testing.T, name string, values []string) map[string]bool {
	t.Helper()
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		seen[value] = true
	}
	return seen
}

func containsCompactRouteLifecycleString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func equalCompactRouteLifecycleInts(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalCompactRouteLifecycleStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
