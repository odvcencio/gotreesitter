//go:build cgo && treesitter_c_parity && treesitter_c_perfscan

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

const (
	workCountEnableEnv                 = "GTS_WORK_COUNT_ORACLE"
	workCountReceiptEnv                = "GTS_WORK_COUNT_RECEIPT"
	workCountFixtureID                 = "query_compile"
	workCountGoAdmissionChildSchema    = "gts-work-count-go-child/v3"
	workCountTaggedGoChildSchema       = "gts-work-count-go-child/v6"
	workCountCChildSchema              = "gts-work-count-c-child/v6"
	workCountContract                  = "gts-work-count/v2"
	workCountReceiptSchema             = "gts-work-count-receipt/v4"
	workCountTimeout                   = 2 * time.Minute
	workCountSharedManifestSHA256      = "a438ddae8eaaf8c343faeb5efccdc15035911ee4bf7d22e627091d123a9ff46d"
	workCountConvergenceManifestSHA256 = "a4f28e92b4916277c56683b39a9e8da7358a9c8dd824b08f7f2be470e0daadde"

	workCountCAdmissionChildEnv    = "GTS_WORK_COUNT_C_ADMISSION_CHILD"
	workCountCAdmissionSourceEnv   = "GTS_WORK_COUNT_C_ADMISSION_SOURCE"
	workCountCAdmissionResultEnv   = "GTS_WORK_COUNT_C_ADMISSION_RESULT"
	workCountCAdmissionChildSchema = "gts-work-count-c-admission-child/v1"
	workCountRepoChildEnv          = "GTS_WORK_COUNT_REPO_CHILD"
	workCountRepoLabelEnv          = "GTS_WORK_COUNT_REPO_LABEL"
	workCountRepoURLEnv            = "GTS_WORK_COUNT_REPO_URL"
	workCountRepoCommitEnv         = "GTS_WORK_COUNT_REPO_COMMIT"
	workCountRepoDestinationEnv    = "GTS_WORK_COUNT_REPO_DESTINATION"
	workCountRepoLanguageEnv       = "GTS_WORK_COUNT_REPO_LANGUAGE"
	workCountFixtureEnv            = "GTS_WORK_COUNT_FIXTURE"
	workCountCExactModelEnv        = "GTS_WORK_COUNT_C_EXACT_MODEL"
)

type workCountCExactModelResult struct {
	Schema string `json:"schema"`
	Passed bool   `json:"passed"`
}

func TestWorkCountPinnedCExactModel(t *testing.T) {
	if os.Getenv(workCountCExactModelEnv) != "1" {
		t.Skip(workCountCExactModelEnv + "=1 is required")
	}
	repoRoot := workCountRepoRoot(t)
	tempRoot := t.TempDir()
	sourceSnapshot := workCountPrepareSourceSnapshot(t, repoRoot, tempRoot)
	workCountClearAmbientGitRouting(t)
	patchPath := filepath.Join(sourceSnapshot.Root, "cgo_harness", "work_count", "tree_sitter_v0_25_1.patch")
	driverPath := filepath.Join(sourceSnapshot.Root, "cgo_harness", "pure_c", "work_count_oracle.c")
	build := workCountBuildC(t, repoRoot, sourceSnapshot.Root, patchPath, driverPath)
	defer build.Cleanup()
	stdout, stderr, err := workCountRunCaptured("", workCountSanitizedEnv(os.Environ(), workCountCBuildEnvironment(), nil), workCountBuildTimeout, build.Artifact, "--exact-model")
	if err != nil || len(stderr) != 0 {
		t.Fatalf("pinned C exact model: err=%v stdout=%s stderr=%s", err, bytes.TrimSpace(stdout), bytes.TrimSpace(stderr))
	}
	var result workCountCExactModelResult
	workCountDecodeExact(t, stdout, &result)
	if result.Schema != "gts-work-count-c-exact-model/v1" || !result.Passed {
		t.Fatalf("pinned C exact model result=%+v", result)
	}
	fixtures, err := benchfixtures.LoadGoFullParseFixtures()
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.Fixture.ID, func(t *testing.T) {
			fixtureRoot := filepath.Join(tempRoot, fixture.Fixture.ID)
			if err := os.MkdirAll(fixtureRoot, 0o755); err != nil {
				t.Fatal(err)
			}
			sourcePath := filepath.Join(fixtureRoot, fixture.Fixture.ID+".go")
			if err := os.WriteFile(sourcePath, fixture.Source, 0o600); err != nil {
				t.Fatal(err)
			}
			cResult := workCountRunC(t, build.Artifact, sourcePath, fixtureRoot)
			workCountValidateCChild(t, "static C exact-vector model", "static-c-instrumented-glr", cResult, fixture, fixture.Fixture.DeepTreeSHA256)
			workCountRequireFrozenStaticCCausalVector(t, fixture.Fixture.ID, cResult.BoardDirect)
		})
	}
	if err := build.Recheck(); err != nil {
		t.Fatalf("pinned C exact model identity drift: %v", err)
	}
}

var workCountDirectFields = []string{
	"shifts", "reductions", "explicit_recover_actions",
	"reduction_pop_requests", "emitted_pop_paths", "emitted_pop_payloads",
	"selected_nodes", "selected_parent_nodes", "selected_leaf_nodes",
}

var workCountTerminalFields = []string{"accept_actions"}

var workCountProxyFields = []string{
	"table_lookups_proxy", "action_entries_examined_proxy",
	"lexer_front_door_calls_proxy", "stack_version_creations_proxy",
	"merge_attempts_proxy", "merge_successes_proxy",
	"graph_link_additions_proxy", "leaf_constructions_proxy",
	"parent_constructions_proxy", "pending_parent_constructions_proxy",
	"no_tree_parent_constructions_proxy",
}

var workCountConvergenceManifestPhases = []string{
	"reduce_window_select", "post_reduce_primary", "post_reduce_pending", "boundary_gss", "boundary_equivalence",
	"boundary_cull", "pending_work", "final_expand", "final_select", "terminal_accept",
}

var workCountConvergenceManifestOutcomes = []string{
	"candidate", "packed", "rejected", "pending_queued", "pending_drained", "pending_discarded",
	"accepted_head", "packed_root_emitted", "mutation_partial", "cap_hit", "observation_unknown", "selected",
}

var workCountConvergenceManifestReasons = []string{
	"status", "score", "shifted", "error_cost", "not_gss", "not_clean", "distinct_shape", "preflight", "cull",
}

var workCountConvergencePhaseOutcomes = map[string][]string{
	"reduce_window_select": {"candidate", "cap_hit"},
	"post_reduce_primary":  {"candidate", "packed", "rejected", "mutation_partial"},
	"post_reduce_pending":  {"candidate", "packed", "rejected", "mutation_partial"},
	"boundary_gss":         {"candidate", "packed", "rejected", "mutation_partial"},
	"boundary_equivalence": {"rejected"},
	"boundary_cull":        {"rejected"},
	"pending_work":         {"pending_queued", "pending_drained", "pending_discarded"},
	"final_expand":         {"packed_root_emitted", "observation_unknown"},
	"final_select":         {"selected"},
	"terminal_accept":      {"accepted_head"},
}

type workCountCounters struct {
	Contract string `json:"contract"`
	Overflow bool   `json:"overflow"`
	workCountCounterValues
}

type workCountCounterValues struct {
	Shifts                 uint64 `json:"shifts"`
	Reductions             uint64 `json:"reductions"`
	AcceptActions          uint64 `json:"accept_actions"`
	ExplicitRecoverActions uint64 `json:"explicit_recover_actions"`
	ReductionPopRequests   uint64 `json:"reduction_pop_requests"`
	EmittedPopPaths        uint64 `json:"emitted_pop_paths"`
	EmittedPopPayloads     uint64 `json:"emitted_pop_payloads"`
	SelectedNodes          uint64 `json:"selected_nodes"`
	SelectedParentNodes    uint64 `json:"selected_parent_nodes"`
	SelectedLeafNodes      uint64 `json:"selected_leaf_nodes"`

	TableLookupsProxy               uint64 `json:"table_lookups_proxy"`
	ActionEntriesExaminedProxy      uint64 `json:"action_entries_examined_proxy"`
	LexerFrontDoorCallsProxy        uint64 `json:"lexer_front_door_calls_proxy"`
	StackVersionCreationsProxy      uint64 `json:"stack_version_creations_proxy"`
	MergeAttemptsProxy              uint64 `json:"merge_attempts_proxy"`
	MergeSuccessesProxy             uint64 `json:"merge_successes_proxy"`
	GraphLinkAdditionsProxy         uint64 `json:"graph_link_additions_proxy"`
	LeafConstructionsProxy          uint64 `json:"leaf_constructions_proxy"`
	ParentConstructionsProxy        uint64 `json:"parent_constructions_proxy"`
	PendingParentConstructionsProxy uint64 `json:"pending_parent_constructions_proxy"`
	NoTreeParentConstructionsProxy  uint64 `json:"no_tree_parent_constructions_proxy"`
}

type workCountAttempt struct {
	Index          uint32 `json:"index"`
	LogicalRung    string `json:"logical_rung"`
	OperationCause string `json:"operation_cause"`

	RequestedMaxStacks       int  `json:"requested_max_stacks"`
	RequestedMaxNodes        int  `json:"requested_max_nodes"`
	RequestedMaxMergePerKey  int  `json:"requested_max_merge_per_key"`
	CapsResolved             bool `json:"caps_resolved"`
	ResolvedMaxStacks        int  `json:"resolved_max_stacks"`
	ResolvedRetryPass        bool `json:"resolved_retry_pass"`
	ResolvedMaxMergePerKey   int  `json:"resolved_max_merge_per_key"`
	ResolvedStackCullTrigger int  `json:"resolved_stack_cull_trigger"`
	ResolvedMaxIterations    int  `json:"resolved_max_iterations"`
	ResolvedMaxNodes         int  `json:"resolved_max_nodes"`

	StopReason   string `json:"stop_reason"`
	RootHasError bool   `json:"root_has_error"`
	RootEndByte  uint32 `json:"root_end_byte"`

	EntryToCaps    workCountCounterValues `json:"entry_to_caps"`
	CapsToFinalize workCountCounterValues `json:"caps_to_finalize"`
	Finalize       workCountCounterValues `json:"finalize"`
	Counters       workCountCounterValues `json:"counters"`
}

type workCountGoCounters struct {
	workCountCounters
	Attempts       []workCountAttempt     `json:"attempts"`
	OutsideAttempt workCountCounterValues `json:"outside_attempt"`
	Convergence    workCountConvergence   `json:"convergence_frontier"`
}

type workCountConvergence struct {
	Capability            string                      `json:"capability"`
	MaxEvents             uint16                      `json:"max_events"`
	MaxPackedPathCount    uint16                      `json:"max_packed_path_count"`
	EventsSeen            uint64                      `json:"events_seen"`
	EventTruncated        bool                        `json:"event_truncated"`
	ArithmeticOverflow    bool                        `json:"arithmetic_overflow"`
	VocabularyViolation   bool                        `json:"vocabulary_violation"`
	SnapshotCountCapped   bool                        `json:"snapshot_count_capped"`
	SnapshotCountUnknown  bool                        `json:"snapshot_count_unknown"`
	PathWalkWorkBudget    uint32                      `json:"path_walk_work_budget"`
	PathWalkWorkUsed      uint32                      `json:"path_walk_work_used"`
	PathWalkWorkExhausted bool                        `json:"path_walk_work_exhausted"`
	Events                []workCountConvergenceEvent `json:"events"`
	FirstRejects          []workCountConvergenceEvent `json:"first_rejects"`
	Aggregates            workCountConvergenceTotals  `json:"aggregates"`
}

type workCountConvergenceTotals struct {
	Phases                    workCountConvergencePhaseTotals   `json:"phases"`
	Outcomes                  workCountConvergenceOutcomeTotals `json:"outcomes"`
	Rejections                workCountConvergenceRejectTotals  `json:"rejections"`
	CandidateCountBeforeTotal uint64                            `json:"candidate_count_before_total"`
	CandidateCountAfterTotal  uint64                            `json:"candidate_count_after_total"`
	LinksBeforeTotal          uint64                            `json:"links_before_total"`
	LinksAfterTotal           uint64                            `json:"links_after_total"`
	PathsBeforeTotal          uint64                            `json:"packed_paths_before_total"`
	PathsAfterTotal           uint64                            `json:"packed_paths_after_total"`
	MutationPartial           uint64                            `json:"mutation_partial"`
	CapHits                   uint64                            `json:"cap_hits"`
	AcceptedHeads             uint64                            `json:"accepted_heads"`
	PackedRoots               uint64                            `json:"packed_roots_emitted"`
}

type workCountConvergencePhaseTotals struct {
	ReduceWindowSelect  uint64 `json:"reduce_window_select"`
	PostReducePrimary   uint64 `json:"post_reduce_primary"`
	PostReducePending   uint64 `json:"post_reduce_pending"`
	BoundaryGSS         uint64 `json:"boundary_gss"`
	BoundaryEquivalence uint64 `json:"boundary_equivalence"`
	BoundaryCull        uint64 `json:"boundary_cull"`
	PendingWork         uint64 `json:"pending_work"`
	FinalExpand         uint64 `json:"final_expand"`
	FinalSelect         uint64 `json:"final_select"`
	TerminalAccept      uint64 `json:"terminal_accept"`
}

type workCountConvergenceOutcomeTotals struct {
	Candidate          uint64 `json:"candidate"`
	Packed             uint64 `json:"packed"`
	Rejected           uint64 `json:"rejected"`
	PendingQueued      uint64 `json:"pending_queued"`
	PendingDrained     uint64 `json:"pending_drained"`
	PendingDiscarded   uint64 `json:"pending_discarded"`
	AcceptedHead       uint64 `json:"accepted_head"`
	PackedRoot         uint64 `json:"packed_root_emitted"`
	MutationPartial    uint64 `json:"mutation_partial"`
	CapHit             uint64 `json:"cap_hit"`
	ObservationUnknown uint64 `json:"observation_unknown"`
	Selected           uint64 `json:"selected"`
}

type workCountConvergenceRejectTotals struct {
	Status        uint64 `json:"status"`
	Score         uint64 `json:"score"`
	Shifted       uint64 `json:"shifted"`
	ErrorCost     uint64 `json:"error_cost"`
	NotGSS        uint64 `json:"not_gss"`
	NotClean      uint64 `json:"not_clean"`
	DistinctShape uint64 `json:"distinct_shape"`
	Preflight     uint64 `json:"preflight"`
	Cull          uint64 `json:"cull"`
}

type workCountConvergenceEvent struct {
	Sequence                         uint64                   `json:"sequence"`
	Attempt                          uint32                   `json:"attempt"`
	Iteration                        uint64                   `json:"iteration"`
	DecisionID                       uint32                   `json:"decision_id"`
	LookaheadSymbol                  uint32                   `json:"lookahead_symbol"`
	LookaheadStartByte               uint32                   `json:"lookahead_start_byte"`
	LookaheadEndByte                 uint32                   `json:"lookahead_end_byte"`
	LookaheadPresent                 bool                     `json:"lookahead_present"`
	LookaheadNoLookahead             bool                     `json:"lookahead_no_lookahead"`
	LookaheadExternal                bool                     `json:"lookahead_external"`
	ElectionScannerComparable        bool                     `json:"election_scanner_comparable"`
	ElectionScannerCheckpointPresent bool                     `json:"election_scanner_checkpoint_present"`
	ElectionScannerCheckpointHash    uint64                   `json:"election_scanner_checkpoint_hash"`
	Phase                            string                   `json:"phase"`
	Outcome                          string                   `json:"outcome"`
	RejectReason                     string                   `json:"reject_reason,omitempty"`
	Detail                           string                   `json:"detail,omitempty"`
	Target                           workCountConvergenceHead `json:"target"`
	Candidate                        workCountConvergenceHead `json:"candidate"`
	CandidateCountBefore             uint32                   `json:"candidate_count_before"`
	CandidateCountAfter              uint32                   `json:"candidate_count_after"`
	LinksBefore                      uint16                   `json:"links_before"`
	LinksAfter                       uint16                   `json:"links_after"`
	PathsBefore                      uint16                   `json:"packed_paths_before"`
	PathsAfter                       uint16                   `json:"packed_paths_after"`
	MutationPartial                  bool                     `json:"mutation_partial"`
	CapHit                           bool                     `json:"cap_hit"`
	SnapshotCapped                   bool                     `json:"snapshot_capped"`
	SnapshotUnknown                  bool                     `json:"snapshot_unknown"`
}

type workCountConvergenceHead struct {
	State               uint32 `json:"state"`
	Byte                uint32 `json:"byte"`
	Status              string `json:"status"`
	HasError            bool   `json:"has_error"`
	Recovering          bool   `json:"recovering"`
	Shifted             bool   `json:"shifted"`
	Score               int64  `json:"score"`
	BranchOrder         uint64 `json:"branch_order"`
	Depth               uint32 `json:"depth"`
	Dead                bool   `json:"dead"`
	Accepted            bool   `json:"accepted"`
	Paused              bool   `json:"paused"`
	NodeBaseline        uint32 `json:"node_baseline"`
	ErrorCost           uint32 `json:"error_cost"`
	ErrorCostComparable bool   `json:"error_cost_comparable"`
	HasGSS              bool   `json:"has_gss"`
	LinkCount           uint16 `json:"link_count"`
	LinkCountCapped     bool   `json:"link_count_capped"`
	HeadContentHash     uint64 `json:"head_content_hash"`
	PredecessorHash     uint64 `json:"predecessor_content_hash"`
}

type workCountGoChildResult struct {
	Schema            string `json:"schema"`
	Engine            string `json:"engine"`
	Fixture           string `json:"fixture"`
	SourceSHA256      string `json:"source_sha256"`
	SourceBytes       uint32 `json:"source_bytes"`
	GrammarCommit     string `json:"grammar_commit"`
	GrammarBlobSHA256 string `json:"grammar_blob_sha256"`
	DigestFormat      string `json:"digest_format"`
	DeepTreeSHA256    string `json:"deep_tree_sha256"`
	RootEndByte       uint32 `json:"root_end_byte"`
	RootHasError      bool   `json:"root_has_error"`
	MaxStacksSeen     int    `json:"max_stacks_seen"`
	MultiStackIters   int    `json:"multi_stack_iterations"`
	MultiStackTokens  uint64 `json:"multi_stack_tokens"`
}

type workCountTaggedChildResult struct {
	workCountGoChildResult
	Counters    workCountGoCounters        `json:"counters"`
	BoardDirect workCountBoardDirectCounts `json:"board_direct"`
}

type workCountBoardDirectCounts struct {
	Schema                                 string `json:"schema"`
	FrontierLexerElectionsAvailable        bool   `json:"frontier_lexer_elections_available"`
	FrontierLexerElections                 uint64 `json:"frontier_lexer_elections"`
	PerVersionLexRequestsAvailable         bool   `json:"per_version_lex_requests_available"`
	PerVersionLexRequests                  uint64 `json:"per_version_lex_requests"`
	RawMainLexerInvocations                uint64 `json:"raw_main_lexer_invocations"`
	ResolvedActionCellsExamined            uint64 `json:"resolved_action_cells_examined"`
	RawActionEntriesBeyondFirst            uint64 `json:"raw_action_entries_beyond_first"`
	ConflictActionArmsAdmitted             uint64 `json:"conflict_action_arms_admitted"`
	CausalConflictForks                    uint64 `json:"causal_conflict_forks"`
	PredecessorLinkUnionAttempts           uint64 `json:"predecessor_link_union_attempts"`
	PredecessorLinkUnionDuplicateNoop      uint64 `json:"predecessor_link_union_duplicate_noop"`
	PredecessorLinkUnionPrecedenceReplaced uint64 `json:"predecessor_link_union_precedence_replaced"`
	PredecessorLinkUnionRecursiveChanged   uint64 `json:"predecessor_link_union_recursive_changed"`
	PredecessorLinkUnionAlternateAppended  uint64 `json:"predecessor_link_union_alternate_appended"`
	PredecessorLinkUnionRejected           uint64 `json:"predecessor_link_union_rejected"`
	AlternatePredecessorLinksAppended      uint64 `json:"alternate_predecessor_links_appended"`
	RawSelectedInternalNodes               uint64 `json:"raw_selected_internal_nodes"`
	RawSelectedInternalParents             uint64 `json:"raw_selected_internal_parent_occurrences"`
	RawSelectedInternalLeaves              uint64 `json:"raw_selected_internal_leaf_occurrences"`
	Overflow                               bool   `json:"overflow"`
}

type workCountCChildResult struct {
	Schema         string                     `json:"schema"`
	Engine         string                     `json:"engine"`
	DigestFormat   string                     `json:"digest_format"`
	DeepTreeSHA256 string                     `json:"deep_tree_sha256,omitempty"`
	SourceBytes    uint32                     `json:"source_bytes"`
	RootEndByte    uint32                     `json:"root_end_byte"`
	RootHasError   bool                       `json:"root_has_error"`
	BoardDirect    workCountBoardDirectCounts `json:"board_direct"`
	Counters       workCountCounters          `json:"counters"`
}

type workCountCAdmissionChildResult struct {
	Schema         string `json:"schema"`
	DigestFormat   string `json:"digest_format"`
	DeepTreeSHA256 string `json:"deep_tree_sha256"`
	SourceSHA256   string `json:"source_sha256"`
}

type workCountToolIdentity struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

type workCountArtifactIdentity struct {
	ArtifactSHA256    string                `json:"artifact_sha256"`
	SourceSnapshotSHA string                `json:"source_snapshot_sha256,omitempty"`
	Tool              workCountToolIdentity `json:"tool"`
	Flags             []string              `json:"flags"`
}

type workCountCIdentity struct {
	Artifact      workCountArtifactIdentity        `json:"artifact"`
	Linker        workCountToolIdentity            `json:"linker"`
	PatchTool     workCountToolIdentity            `json:"patch_tool"`
	SymbolTool    workCountToolIdentity            `json:"symbol_tool"`
	VerifierTools map[string]workCountToolIdentity `json:"verifier_tools"`
	LinkageProof  string                           `json:"linkage_proof"`
	RuntimeCommit string                           `json:"runtime_commit"`
	RuntimeTree   string                           `json:"runtime_source_tree_oid"`
	PatchSHA256   string                           `json:"patch_sha256"`
	DriverSHA256  string                           `json:"driver_sha256"`
	InputSHA256   string                           `json:"input_snapshot_sha256"`
	InputFiles    int                              `json:"input_snapshot_files"`
	Environment   map[string]string                `json:"environment"`
	Language      perfScanOracleLanguageIdentity   `json:"language"`
}

type workCountRatio struct {
	Field   string   `json:"field"`
	Class   string   `json:"class"`
	Go      uint64   `json:"go"`
	C       uint64   `json:"c"`
	GoOverC *float64 `json:"go_over_c,omitempty"`
}

type workCountReceipt struct {
	Schema                      string                    `json:"schema"`
	Fixture                     string                    `json:"fixture"`
	SourceSHA256                string                    `json:"source_sha256"`
	SourceBytes                 int                       `json:"source_bytes"`
	DigestFormat                string                    `json:"digest_format"`
	DeepTreeSHA256              string                    `json:"deep_tree_sha256"`
	SharedCounterManifestSHA256 string                    `json:"shared_counter_manifest_sha256"`
	ConvergenceManifestSHA256   string                    `json:"convergence_manifest_sha256"`
	Source                      workCountGitProvenance    `json:"source"`
	GoEnvironment               workCountEnvironment      `json:"go_environment"`
	GoAdmission                 workCountArtifactIdentity `json:"go_admission_artifact"`
	GoArtifact                  workCountArtifactIdentity `json:"go_work_count_artifact"`
	CArtifact                   workCountCIdentity        `json:"static_c_artifact"`
	GoCounts                    workCountGoCounters       `json:"go_counts"`
	CCounts                     workCountCounters         `json:"static_c_counts"`
	Ratios                      []workCountRatio          `json:"ratios"`
	GoSurplus                   uint64                    `json:"go_construction_surplus"`
	CSurplus                    uint64                    `json:"static_c_construction_surplus"`
}

type workCountCBuild struct {
	Artifact string
	Identity workCountCIdentity
	Cleanup  func()
	Recheck  func() error
}

func TestAuthenticatedWorkCountOracle(t *testing.T) {
	if os.Getenv(workCountEnableEnv) != "1" {
		t.Skip(workCountEnableEnv + "=1 is required")
	}
	repoRoot := workCountRepoRoot(t)
	receiptPath := workCountPrepareReceiptPath(t, repoRoot)
	tempRoot := t.TempDir()
	sourceSnapshot := workCountPrepareSourceSnapshot(t, repoRoot, tempRoot)
	// Worktree-routing variables are needed only to authenticate and snapshot
	// the mounted source. They must not leak into pinned runtime/grammar Git
	// operations performed later by the static-C builders.
	workCountClearAmbientGitRouting(t)
	fixture := workCountLoadFixture(t)
	sharedManifestPath := filepath.Join(sourceSnapshot.Root, "cgo_harness", "work_count", "contract_v2.json")
	convergenceManifestPath := filepath.Join(sourceSnapshot.Root, "cgo_harness", "work_count", "contract_v3.json")
	patchPath := filepath.Join(sourceSnapshot.Root, "cgo_harness", "work_count", "tree_sitter_v0_25_1.patch")
	driverPath := filepath.Join(sourceSnapshot.Root, "cgo_harness", "pure_c", "work_count_oracle.c")
	sharedManifestSHA := workCountFileSHA(t, sharedManifestPath)
	convergenceManifestSHA := workCountFileSHA(t, convergenceManifestPath)
	if sharedManifestSHA != workCountSharedManifestSHA256 || convergenceManifestSHA != workCountConvergenceManifestSHA256 {
		t.Fatalf("work-count manifest identity shared=%s/%s convergence=%s/%s", sharedManifestSHA, workCountSharedManifestSHA256, convergenceManifestSHA, workCountConvergenceManifestSHA256)
	}
	patchSHA := workCountFileSHA(t, patchPath)
	driverSHA := workCountFileSHA(t, driverPath)
	workCountValidateManifest(t, sharedManifestPath)
	workCountValidateConvergenceManifest(t, convergenceManifestPath)
	workCountValidateManifestAgreement(t, sharedManifestPath, convergenceManifestPath)

	sourcePath := filepath.Join(tempRoot, "query_compile.go")
	if err := os.WriteFile(sourcePath, fixture.Source, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := workCountFileSHA(t, sourcePath); got != fixture.Fixture.SHA256 {
		t.Fatalf("source snapshot sha=%s want=%s", got, fixture.Fixture.SHA256)
	}

	// Admission deliberately precedes every instrumented build and run. An
	// ordinary untagged child built from the private source snapshot and the
	// unmodified static C oracle must reproduce the frozen digest first.
	goEnvironment := workCountChosenEnvironment()
	t.Log("build: ordinary untagged Go admission child")
	goAdmissionArtifact, goAdmissionIdentity, goAdmissionRecheck := workCountBuildGo(t, sourceSnapshot, tempRoot, goEnvironment, false)
	t.Log("admission: ordinary untagged Go child")
	uninstGo := workCountRunGoAdmission(t, sourceSnapshot.Root, goAdmissionArtifact, fixture.Fixture.ID, sourcePath, tempRoot, goEnvironment)
	workCountValidateGoChild(t, "ordinary Go admission", workCountGoAdmissionChildSchema, "go-production-glr-untagged", uninstGo, fixture)
	t.Log("admission: unmodified static C")
	uninstC := workCountUninstrumentedCAdmission(t, fixture, sourcePath, tempRoot)
	if uninstGo.DeepTreeSHA256 != uninstC || uninstGo.DeepTreeSHA256 != fixture.Fixture.DeepTreeSHA256 {
		t.Fatalf("uninstrumented admission mismatch: go=%s static_c=%s frozen=%s", uninstGo.DeepTreeSHA256, uninstC, fixture.Fixture.DeepTreeSHA256)
	}

	t.Log("build: instrumented static C child")
	cBuild := workCountBuildC(t, repoRoot, sourceSnapshot.Root, patchPath, driverPath)
	defer cBuild.Cleanup()
	t.Log("build: tagged diagnostic Go child")
	goArtifact, goIdentity, goRecheck := workCountBuildGo(t, sourceSnapshot, tempRoot, goEnvironment, true)

	t.Log("run: instrumented static C child")
	cResult := workCountRunC(t, cBuild.Artifact, sourcePath, tempRoot)
	t.Log("run: tagged diagnostic Go child")
	goResult := workCountRunGo(t, sourceSnapshot.Root, goArtifact, fixture.Fixture.ID, sourcePath, tempRoot, goEnvironment)
	workCountValidateCChild(t, "static C", "static-c-instrumented-glr", cResult, fixture, uninstC)
	workCountValidateGoChild(t, "tagged Go", workCountTaggedGoChildSchema, "go-production-glr-tagged-diagnostic", goResult.workCountGoChildResult, fixture)
	workCountValidateGoCounters(t, "tagged Go", goResult.Counters, uint32(len(fixture.Source)))
	if cResult.DeepTreeSHA256 != goResult.DeepTreeSHA256 {
		t.Fatalf("instrumented deep digest mismatch: go=%s static_c=%s", goResult.DeepTreeSHA256, cResult.DeepTreeSHA256)
	}
	if err := cBuild.Recheck(); err != nil {
		t.Fatalf("static C identity drift: %v", err)
	}
	if err := goRecheck(); err != nil {
		t.Fatalf("Go identity drift: %v", err)
	}
	if err := goAdmissionRecheck(); err != nil {
		t.Fatalf("Go admission identity drift: %v", err)
	}
	if err := sourceSnapshot.Recheck(); err != nil {
		t.Fatalf("source provenance drift: %v", err)
	}
	if got := workCountFileSHA(t, sourcePath); got != fixture.Fixture.SHA256 {
		t.Fatalf("source snapshot identity drift: got %s want %s", got, fixture.Fixture.SHA256)
	}
	for label, pathAndSHA := range map[string][2]string{
		"shared counter manifest": {sharedManifestPath, sharedManifestSHA},
		"convergence manifest":    {convergenceManifestPath, convergenceManifestSHA},
		"patch":                   {patchPath, patchSHA},
		"driver":                  {driverPath, driverSHA},
	} {
		if got := workCountFileSHA(t, pathAndSHA[0]); got != pathAndSHA[1] {
			t.Fatalf("%s identity drift: got %s want %s", label, got, pathAndSHA[1])
		}
	}

	receipt := workCountReceipt{
		Schema: workCountReceiptSchema, Fixture: fixture.Fixture.ID,
		SourceSHA256: fixture.Fixture.SHA256, SourceBytes: len(fixture.Source),
		DigestFormat: benchfixtures.DeepTreeDigestVersion, DeepTreeSHA256: uninstGo.DeepTreeSHA256,
		SharedCounterManifestSHA256: sharedManifestSHA, ConvergenceManifestSHA256: convergenceManifestSHA,
		Source:        sourceSnapshot.Provenance,
		GoEnvironment: goEnvironment, GoAdmission: goAdmissionIdentity,
		GoArtifact: goIdentity, CArtifact: cBuild.Identity,
		GoCounts: goResult.Counters, CCounts: cResult.Counters,
		Ratios:    workCountRatios(goResult.Counters.workCountCounters, cResult.Counters),
		GoSurplus: workCountConstructionSurplus(t, "Go", goResult.Counters.workCountCounters),
		CSurplus:  workCountConstructionSurplus(t, "static C", cResult.Counters),
	}
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := workCountAtomicPublish(receiptPath, data); err != nil {
		t.Fatal(err)
	}
	if sourceSnapshot.Provenance.Authoritative {
		t.Logf("authenticated work-count receipt: %s", receiptPath)
	} else {
		t.Logf("non-authoritative dirty-source work-count receipt: %s", receiptPath)
	}
}

func workCountRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(wd)
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("resolve repository root from %s: %v", wd, err)
	}
	return root
}

func workCountLoadFixture(t *testing.T) benchfixtures.LoadedFixture {
	t.Helper()
	fixtures, err := benchfixtures.LoadGoFullParseFixtures()
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		if fixture.Fixture.ID == workCountFixtureID {
			return fixture
		}
	}
	t.Fatalf("fixture %s not found", workCountFixtureID)
	return benchfixtures.LoadedFixture{}
}

func workCountUninstrumentedCAdmission(t *testing.T, fixture benchfixtures.LoadedFixture, sourcePath, tempRoot string) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(tempRoot, "static_c_admission.json")
	environment := workCountSanitizedEnv(os.Environ(), workCountCBuildEnvironment(), map[string]string{
		workCountCAdmissionChildEnv:  "1",
		workCountCAdmissionSourceEnv: sourcePath,
		workCountCAdmissionResultEnv: resultPath,
	})
	_, stderr, err := workCountRunCaptured(
		"",
		environment,
		workCountBuildTimeout+workCountTimeout+staticCPerfWallGrace,
		self,
		"-test.run", "^TestWorkCountStaticCAdmissionChild$",
		"-test.count=1",
	)
	if err != nil {
		t.Fatalf("bounded static C admission child: %v", err)
	}
	if len(stderr) != 0 {
		t.Fatalf("bounded static C admission child wrote stderr: %s", strings.TrimSpace(string(stderr)))
	}
	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var result workCountCAdmissionChildResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.Schema != workCountCAdmissionChildSchema || result.DigestFormat != benchfixtures.DeepTreeDigestVersion {
		t.Fatalf("static C admission child identity=%q/%q", result.Schema, result.DigestFormat)
	}
	if result.SourceSHA256 != fixture.Fixture.SHA256 {
		t.Fatalf("static C admission source sha=%s want=%s", result.SourceSHA256, fixture.Fixture.SHA256)
	}
	if err := fixture.Fixture.VerifyDeepTreeDigest(result.DeepTreeSHA256); err != nil {
		t.Fatal(err)
	}
	return result.DeepTreeSHA256
}

func TestWorkCountStaticCAdmissionChild(t *testing.T) {
	if os.Getenv(workCountCAdmissionChildEnv) != "1" {
		t.Skip("work-count static C admission child is not configured")
	}
	sourcePath := strings.TrimSpace(os.Getenv(workCountCAdmissionSourceEnv))
	resultPath := strings.TrimSpace(os.Getenv(workCountCAdmissionResultEnv))
	if sourcePath == "" || resultPath == "" {
		t.Fatal("work-count static C admission child paths are incomplete")
	}
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := buildStaticCPerfOracle("go")
	if err != nil {
		t.Fatal(err)
	}
	defer oracle.Close()
	digest, sourceSHA, err := oracle.deepDigest(source, workCountTimeout)
	if err != nil {
		t.Fatal(err)
	}
	result := workCountCAdmissionChildResult{
		Schema: workCountCAdmissionChildSchema, DigestFormat: benchfixtures.DeepTreeDigestVersion,
		DeepTreeSHA256: digest, SourceSHA256: sourceSHA,
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultPath, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func workCountEnsurePinnedRepo(t *testing.T, label, repoURL, commit, destination, language string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	environment := workCountSanitizedEnv(os.Environ(), workCountCBuildEnvironment(), map[string]string{
		workCountRepoChildEnv:       "1",
		workCountRepoLabelEnv:       label,
		workCountRepoURLEnv:         repoURL,
		workCountRepoCommitEnv:      commit,
		workCountRepoDestinationEnv: destination,
		workCountRepoLanguageEnv:    language,
		"GIT_TERMINAL_PROMPT":       "0",
	})
	_, stderr, err := workCountRunCaptured(
		"",
		environment,
		workCountBuildTimeout,
		self,
		"-test.run", "^TestWorkCountEnsurePinnedRepoChild$",
		"-test.count=1",
	)
	if err != nil {
		t.Fatalf("bounded %s acquisition: %v", label, err)
	}
	if len(stderr) != 0 {
		t.Fatalf("bounded %s acquisition wrote stderr: %s", label, strings.TrimSpace(string(stderr)))
	}
	if err := workCountVerifyPinnedRepo(destination, commit, environment); err != nil {
		t.Fatalf("bounded %s verification: %v", label, err)
	}
}

func TestWorkCountEnsurePinnedRepoChild(t *testing.T) {
	if os.Getenv(workCountRepoChildEnv) != "1" {
		t.Skip("work-count pinned repository child is not configured")
	}
	label := strings.TrimSpace(os.Getenv(workCountRepoLabelEnv))
	repoURL := strings.TrimSpace(os.Getenv(workCountRepoURLEnv))
	commit := strings.TrimSpace(os.Getenv(workCountRepoCommitEnv))
	destination := strings.TrimSpace(os.Getenv(workCountRepoDestinationEnv))
	language := strings.TrimSpace(os.Getenv(workCountRepoLanguageEnv))
	if label == "" || repoURL == "" || commit == "" || destination == "" {
		t.Fatal("work-count pinned repository child configuration is incomplete")
	}
	if err := staticCEnsurePinnedRepo(label, repoURL, commit, destination, language); err != nil {
		t.Fatal(err)
	}
}

func workCountGitOutput(repoDir string, environment []string, args ...string) (string, error) {
	gitArgs := append([]string{"-c", "safe.directory=" + repoDir, "-C", repoDir}, args...)
	output, err := workCountRunBounded("", environment, workCountGitTimeout, "git", gitArgs...)
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func workCountVerifyPinnedRepo(repoDir, commit string, environment []string) error {
	head, err := workCountGitOutput(repoDir, environment, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return fmt.Errorf("read HEAD: %w", err)
	}
	want, err := workCountGitOutput(repoDir, environment, "rev-parse", "--verify", commit+"^{commit}")
	if err != nil {
		return fmt.Errorf("resolve pinned commit: %w", err)
	}
	if strings.TrimSpace(head) != strings.TrimSpace(want) {
		return fmt.Errorf("HEAD mismatch: got %s want %s", strings.TrimSpace(head), strings.TrimSpace(want))
	}
	status, err := workCountGitOutput(repoDir, environment, "status", "--porcelain=v1", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return fmt.Errorf("inspect worktree: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		return fmt.Errorf("pinned worktree is dirty: %s", strings.TrimSpace(status))
	}
	return nil
}

func workCountResolveGoGrammarSources(entry parityLockEntry, pinnedDir string) (grammarDir, srcDir, parserPath, sourceMode string, cleanup func(), err error) {
	cleanup = func() {}
	srcDir = filepath.Join(pinnedDir, entry.Subdir)
	parserPath = filepath.Join(srcDir, "parser.c")
	info, err := os.Stat(parserPath)
	if err != nil || !info.Mode().IsRegular() {
		return "", "", "", "", cleanup, fmt.Errorf("locked Go parser is unavailable at %s: %w", parserPath, err)
	}
	if version, ok := readParserLanguageVersion(parserPath); ok && (version < parityMinLanguageVersion || version > parityMaxLanguageVersion) {
		return "", "", "", "", cleanup, fmt.Errorf("locked Go parser ABI %d is outside %d..%d", version, parityMinLanguageVersion, parityMaxLanguageVersion)
	}
	return pinnedDir, srcDir, parserPath, staticCSourceLocked, cleanup, nil
}

func workCountLanguageIdentity(entry parityLockEntry, grammarDir, srcDir, parserPath string, scanners []string, symbol, sourceMode string, hasCXX bool, linker workCountToolIdentity) (perfScanOracleLanguageIdentity, error) {
	environment := workCountSanitizedEnv(os.Environ(), workCountCBuildEnvironment(), nil)
	tree, err := workCountGitOutput(grammarDir, environment, "rev-parse", entry.Commit+":"+filepath.ToSlash(entry.Subdir))
	if err != nil {
		return perfScanOracleLanguageIdentity{}, err
	}
	parserSHA, err := fileSHA256(parserPath)
	if err != nil {
		return perfScanOracleLanguageIdentity{}, err
	}
	identity := perfScanOracleLanguageIdentity{
		Language: entry.Name, GrammarRepo: entry.RepoURL, GrammarCommit: entry.Commit,
		GrammarSourceTree: strings.TrimSpace(tree), GrammarSourceMode: sourceMode,
		LanguageSymbol: symbol,
		Parser: perfScanOracleSourceIdentity{
			Path: staticCSourceRelative(srcDir, parserPath), SHA256: parserSHA, CompileFlags: staticCPerfGrammarCFlags,
		},
		LinkerPath: linker.Path, LinkerVersion: linker.Version, LinkerSHA256: linker.SHA256,
	}
	for _, scanner := range scanners {
		sha, err := fileSHA256(scanner)
		if err != nil {
			return perfScanOracleLanguageIdentity{}, err
		}
		flags := staticCPerfGrammarCFlags
		if ext := strings.ToLower(filepath.Ext(scanner)); ext == ".cc" || ext == ".cpp" || ext == ".cxx" {
			flags = staticCPerfGrammarCXXFlags
		}
		identity.Scanners = append(identity.Scanners, perfScanOracleSourceIdentity{
			Path: staticCSourceRelative(srcDir, scanner), SHA256: sha, CompileFlags: flags,
		})
	}
	if hasCXX && filepath.Base(linker.Path) != "c++" && filepath.Base(linker.Path) != "g++" {
		return perfScanOracleLanguageIdentity{}, fmt.Errorf("C++ grammar resolved non-C++ linker %s", linker.Path)
	}
	return identity, nil
}

func workCountBuildC(t *testing.T, repoRoot, sourceRoot, patchPath, driverPath string) workCountCBuild {
	t.Helper()
	lockPath := filepath.Join(sourceRoot, "grammars", "languages.lock")
	lock, err := loadParityLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	entry := lock["go"]
	cacheRoot := strings.TrimSpace(os.Getenv(staticCPerfCacheEnv))
	if cacheRoot == "" {
		cacheRoot = filepath.Join(repoRoot, "harness_out", "c_oracle", "fleet_static")
	}
	runtimeDir := filepath.Join(cacheRoot, "sources", "tree-sitter-"+COracleRuntimeCommit)
	grammarDir := filepath.Join(cacheRoot, "sources", paritySafeName("go")+"-"+entry.Commit)
	workCountEnsurePinnedRepo(t, "runtime", staticCPerfRuntimeRepo, COracleRuntimeCommit, runtimeDir, "")
	workCountEnsurePinnedRepo(t, "go grammar", entry.RepoURL, entry.Commit, grammarDir, "go")
	resolvedGrammar, srcDir, parserPath, sourceMode, cleanupGrammar, err := workCountResolveGoGrammarSources(entry, grammarDir)
	if err != nil {
		t.Fatal(err)
	}
	scanners := staticCScannerSources(srcDir)
	hasCXX := false
	for _, scanner := range scanners {
		switch strings.ToLower(filepath.Ext(scanner)) {
		case ".cc", ".cpp", ".cxx":
			hasCXX = true
		}
	}
	compiler := workCountTool(t, "cc")
	linkerName := "cc"
	if hasCXX {
		linkerName = "c++"
	}
	linker := workCountTool(t, linkerName)
	patchTool := workCountTool(t, "git")
	symbolTool := workCountTool(t, "nm")
	cEnvironment := workCountCBuildEnvironment()
	commandEnvironment := workCountSanitizedEnv(os.Environ(), cEnvironment, nil)
	buildRoot, err := os.MkdirTemp("", "gts-work-count-c-*")
	if err != nil {
		cleanupGrammar()
		t.Fatal(err)
	}
	cleanup := func() {
		cleanupGrammar()
		_ = workCountMakeTreeWritable(filepath.Join(buildRoot, "inputs"))
		_ = os.RemoveAll(buildRoot)
	}

	// Copy every compiler input into a private snapshot before patching or
	// compiling. The pinned caches and outer worktree are never compiler input.
	inputRoot := filepath.Join(buildRoot, "inputs")
	privateRuntime := filepath.Join(inputRoot, "runtime")
	privateGrammar := filepath.Join(inputRoot, "grammar")
	privatePatch := filepath.Join(inputRoot, "tree_sitter.patch")
	privateDriver := filepath.Join(inputRoot, "work_count_oracle.c")
	privateLock := filepath.Join(inputRoot, "languages.lock")
	if err := workCountCopyTree(filepath.Join(runtimeDir, "lib"), filepath.Join(privateRuntime, "lib")); err != nil {
		cleanup()
		t.Fatal(err)
	}
	if err := workCountCopyTree(srcDir, privateGrammar); err != nil {
		cleanup()
		t.Fatal(err)
	}
	if err := workCountCopyFile(patchPath, privatePatch, 0o444); err != nil {
		cleanup()
		t.Fatal(err)
	}
	if err := workCountCopyFile(driverPath, privateDriver, 0o444); err != nil {
		cleanup()
		t.Fatal(err)
	}
	if err := workCountCopyFile(lockPath, privateLock, 0o444); err != nil {
		cleanup()
		t.Fatal(err)
	}
	inputSHA, inputFiles, err := workCountTreeSHA(inputRoot)
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	if err := workCountMakeTreeReadOnly(inputRoot); err != nil {
		cleanup()
		t.Fatal(err)
	}
	if got, files, err := workCountTreeSHA(inputRoot); err != nil || got != inputSHA || files != inputFiles {
		cleanup()
		t.Fatalf("C input snapshot drift after sealing: sha=%s files=%d want=%s/%d err=%v", got, files, inputSHA, inputFiles, err)
	}
	parserRel := staticCSourceRelative(srcDir, parserPath)
	privateParser := filepath.Join(privateGrammar, filepath.FromSlash(parserRel))
	privateScanners := make([]string, 0, len(scanners))
	for _, scanner := range scanners {
		privateScanners = append(privateScanners, filepath.Join(privateGrammar, filepath.FromSlash(staticCSourceRelative(srcDir, scanner))))
	}

	patchedRuntime := filepath.Join(buildRoot, "patched-runtime")
	if err := workCountCopyTree(privateRuntime, patchedRuntime); err != nil {
		cleanup()
		t.Fatal(err)
	}
	if err := workCountMakeTreeWritable(patchedRuntime); err != nil {
		cleanup()
		t.Fatal(err)
	}
	if _, err := workCountRunBounded(patchedRuntime, commandEnvironment, workCountBuildTimeout, patchTool.Path, "apply", "--check", privatePatch); err != nil {
		cleanup()
		t.Fatal(err)
	}
	if _, err := workCountRunBounded(patchedRuntime, commandEnvironment, workCountBuildTimeout, patchTool.Path, "apply", privatePatch); err != nil {
		cleanup()
		t.Fatal(err)
	}

	objects := filepath.Join(buildRoot, "objects")
	if err := os.MkdirAll(objects, 0o755); err != nil {
		cleanup()
		t.Fatal(err)
	}
	runtimeObj := filepath.Join(objects, "runtime.o")
	runtimeFlags := strings.Fields(staticCPerfRuntimeCFlags)
	runtimeArgs := append([]string{}, runtimeFlags...)
	runtimeArgs = append(runtimeArgs, "-I", filepath.Join(patchedRuntime, "lib", "include"), "-I", filepath.Join(patchedRuntime, "lib", "src"), "-c", filepath.Join(patchedRuntime, "lib", "src", "lib.c"), "-o", runtimeObj)
	if _, err := workCountRunBounded("", commandEnvironment, workCountBuildTimeout, compiler.Path, runtimeArgs...); err != nil {
		cleanup()
		t.Fatal(err)
	}
	var grammarObjects []string
	for i, source := range append([]string{privateParser}, privateScanners...) {
		obj := filepath.Join(objects, fmt.Sprintf("grammar_%d.o", i))
		tool := compiler.Path
		flags := strings.Fields(staticCPerfGrammarCFlags)
		ext := strings.ToLower(filepath.Ext(source))
		if ext == ".cc" || ext == ".cpp" || ext == ".cxx" {
			tool = linker.Path
			flags = strings.Fields(staticCPerfGrammarCXXFlags)
		}
		args := append([]string{}, flags...)
		args = append(args, "-I", privateGrammar, "-I", filepath.Join(patchedRuntime, "lib", "include"), "-c", source, "-o", obj)
		if _, err := workCountRunBounded("", commandEnvironment, workCountBuildTimeout, tool, args...); err != nil {
			cleanup()
			t.Fatal(err)
		}
		grammarObjects = append(grammarObjects, obj)
	}
	symbol := workCountDefinedLanguageSymbol(t, entry, symbolTool.Path, grammarObjects[0], commandEnvironment)
	languageIdentity, err := workCountLanguageIdentity(entry, resolvedGrammar, privateGrammar, privateParser, privateScanners, symbol, sourceMode, hasCXX, linker)
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	driverObj := filepath.Join(objects, "driver.o")
	driverFlags := strings.Fields(staticCPerfDriverCFlags)
	driverArgs := append([]string{}, driverFlags...)
	driverArgs = append(driverArgs, "-DTS_LANG_FN="+symbol, "-I", filepath.Join(patchedRuntime, "lib", "include", "tree_sitter"), "-I", filepath.Join(patchedRuntime, "lib", "src"), "-c", privateDriver, "-o", driverObj)
	if _, err := workCountRunBounded("", commandEnvironment, workCountBuildTimeout, compiler.Path, driverArgs...); err != nil {
		cleanup()
		t.Fatal(err)
	}
	artifact := filepath.Join(buildRoot, "work_count_oracle")
	linkArgs := append(strings.Fields(staticCPerfLinkFlags), "-o", artifact, driverObj)
	linkArgs = append(linkArgs, grammarObjects...)
	linkArgs = append(linkArgs, runtimeObj)
	if _, err := workCountRunBounded("", commandEnvironment, workCountBuildTimeout, linker.Path, linkArgs...); err != nil {
		cleanup()
		t.Fatal(err)
	}
	artifactSHA := workCountFileSHA(t, artifact)
	linkageProof, verifierTools, err := workCountVerifyStaticArtifact(artifact, symbol, commandEnvironment)
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	runtimeTree, err := workCountGitOutput(runtimeDir, commandEnvironment, "rev-parse", COracleRuntimeCommit+":lib/src")
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	buildKeyInput, err := json.Marshal(struct {
		RuntimeCommit string                         `json:"runtime_commit"`
		RuntimeTree   string                         `json:"runtime_tree"`
		PatchSHA256   string                         `json:"patch_sha256"`
		DriverSHA256  string                         `json:"driver_sha256"`
		Compiler      workCountToolIdentity          `json:"compiler"`
		Linker        workCountToolIdentity          `json:"linker"`
		SymbolTool    workCountToolIdentity          `json:"symbol_tool"`
		Language      perfScanOracleLanguageIdentity `json:"language"`
		Flags         []string                       `json:"flags"`
		Environment   map[string]string              `json:"environment"`
		InputSHA256   string                         `json:"input_snapshot_sha256"`
	}{
		RuntimeCommit: COracleRuntimeCommit, RuntimeTree: strings.TrimSpace(runtimeTree),
		PatchSHA256: workCountFileSHA(t, privatePatch), DriverSHA256: workCountFileSHA(t, privateDriver),
		Compiler: compiler, Linker: linker, SymbolTool: symbolTool, Language: languageIdentity,
		Flags: []string{
			"runtime:" + staticCPerfRuntimeCFlags,
			"grammar_c:" + staticCPerfGrammarCFlags,
			"grammar_cxx:" + staticCPerfGrammarCXXFlags,
			"driver:" + staticCPerfDriverCFlags,
			"link:" + staticCPerfLinkFlags,
		},
		Environment: cEnvironment, InputSHA256: inputSHA,
	})
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	buildKey := sha256.Sum256(buildKeyInput)
	languageIdentity.BuildKeySHA256 = hex.EncodeToString(buildKey[:])
	languageIdentity.ArtifactSHA256 = artifactSHA
	languageIdentity.ArtifactStatic = true
	languageIdentity.ArtifactLinkage = linkageProof
	identity := workCountCIdentity{
		Artifact: workCountArtifactIdentity{ArtifactSHA256: artifactSHA, Tool: compiler, Flags: []string{
			"runtime:" + staticCPerfRuntimeCFlags,
			"grammar_c:" + staticCPerfGrammarCFlags,
			"grammar_cxx:" + staticCPerfGrammarCXXFlags,
			"driver:" + staticCPerfDriverCFlags,
			"link:" + staticCPerfLinkFlags,
		}},
		Linker: linker, PatchTool: patchTool, SymbolTool: symbolTool, VerifierTools: verifierTools, LinkageProof: linkageProof,
		RuntimeCommit: COracleRuntimeCommit, RuntimeTree: strings.TrimSpace(runtimeTree),
		PatchSHA256: workCountFileSHA(t, privatePatch), DriverSHA256: workCountFileSHA(t, privateDriver),
		InputSHA256: inputSHA, InputFiles: inputFiles, Environment: cEnvironment,
		Language: languageIdentity,
	}
	recheck := func() error {
		if got, err := fileSHA256(artifact); err != nil || got != artifactSHA {
			return fmt.Errorf("artifact sha=%s want=%s err=%v", got, artifactSHA, err)
		}
		if got, files, err := workCountTreeSHA(inputRoot); err != nil || got != inputSHA || files != inputFiles {
			return fmt.Errorf("C input snapshot sha=%s files=%d want=%s/%d err=%v", got, files, inputSHA, inputFiles, err)
		}
		tools := map[string]workCountToolIdentity{"compiler": compiler, "linker": linker, "patch": patchTool, "symbol": symbolTool}
		for label, identity := range verifierTools {
			tools["verifier_"+label] = identity
		}
		for label, want := range tools {
			got, err := workCountToolAt(want.Path)
			if err != nil || got != want {
				return fmt.Errorf("%s identity=%+v want=%+v err=%v", label, got, want, err)
			}
		}
		return nil
	}
	return workCountCBuild{Artifact: artifact, Identity: identity, Cleanup: cleanup, Recheck: recheck}
}

func workCountDefinedLanguageSymbol(t *testing.T, entry parityLockEntry, symbolTool, object string, environment []string) string {
	t.Helper()
	output, err := workCountRunBounded("", environment, workCountBuildTimeout, symbolTool, "-g", "--defined-only", object)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		symbol := fields[len(fields)-1]
		if strings.HasPrefix(symbol, "tree_sitter_") {
			found[symbol] = true
		}
	}
	for _, candidate := range parityLanguageSymbols(entry) {
		if found[candidate] {
			if len(found) != 1 {
				t.Fatalf("compiled parser object has ambiguous language symbols: %v", found)
			}
			return candidate
		}
	}
	t.Fatalf("compiled parser object has no accepted language symbol: %v", found)
	return ""
}

func workCountVerifyStaticArtifact(artifact, symbol string, environment []string) (string, map[string]workCountToolIdentity, error) {
	if strings.TrimSpace(symbol) == "" {
		return "", nil, fmt.Errorf("language symbol is empty")
	}
	nm, err := workCountToolNamed("nm")
	if err != nil {
		return "", nil, err
	}
	readelf, err := workCountToolNamed("readelf")
	if err != nil {
		return "", nil, err
	}
	nmOutput, err := workCountRunBounded("", environment, workCountGitTimeout, nm.Path, artifact)
	if err != nil {
		return "", nil, fmt.Errorf("nm: %w", err)
	}
	defined := make(map[string]bool)
	for _, line := range strings.Split(string(nmOutput), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && !strings.EqualFold(fields[len(fields)-2], "U") {
			defined[fields[len(fields)-1]] = true
		}
	}
	for _, required := range []string{"ts_parser_parse", symbol} {
		if !defined[required] {
			return "", nil, fmt.Errorf("artifact does not define %s", required)
		}
	}
	dynamicOutput, err := workCountRunBounded("", environment, workCountGitTimeout, readelf.Path, "-d", artifact)
	if err != nil {
		return "", nil, fmt.Errorf("readelf dynamic-section proof: %w", err)
	}
	if bytes.Contains(dynamicOutput, []byte("NEEDED")) {
		return "", nil, fmt.Errorf("artifact has dynamic dependencies: %s", strings.TrimSpace(string(dynamicOutput)))
	}
	programOutput, err := workCountRunBounded("", environment, workCountGitTimeout, readelf.Path, "-l", artifact)
	if err != nil {
		return "", nil, fmt.Errorf("readelf program-header proof: %w", err)
	}
	if bytes.Contains(programOutput, []byte("INTERP")) {
		return "", nil, fmt.Errorf("artifact has a dynamic program interpreter: %s", strings.TrimSpace(string(programOutput)))
	}
	return staticCPerfLinkageProof, map[string]workCountToolIdentity{"nm": nm, "readelf": readelf}, nil
}

func workCountBuildGo(t *testing.T, source workCountSourceSnapshot, tempRoot string, environment workCountEnvironment, tagged bool) (string, workCountArtifactIdentity, func() error) {
	t.Helper()
	tool := workCountTool(t, "go")
	name := "go_admission.test"
	flags := []string{"test", "-c", "."}
	args := []string{"test", "-c"}
	if tagged {
		name = "go_work_count.test"
		flags = []string{"test", "-c", "-tags", "gts_workcount", "."}
		args = append(args, "-tags", "gts_workcount")
	}
	artifact := filepath.Join(tempRoot, name)
	args = append(args, "-o", artifact, ".")
	buildEnv := workCountSanitizedEnv(os.Environ(), environment.Build, nil)
	if _, err := workCountRunBounded(source.Root, buildEnv, workCountBuildTimeout, tool.Path, args...); err != nil {
		t.Fatal(err)
	}
	sha := workCountFileSHA(t, artifact)
	identity := workCountArtifactIdentity{
		ArtifactSHA256: sha, SourceSnapshotSHA: source.Provenance.SnapshotSHA256,
		Tool: tool, Flags: flags,
	}
	return artifact, identity, func() error {
		gotSHA, err := fileSHA256(artifact)
		if err != nil || gotSHA != sha {
			return fmt.Errorf("artifact sha=%s want=%s err=%v", gotSHA, sha, err)
		}
		gotTool, err := workCountToolAt(tool.Path)
		if err != nil || gotTool != tool {
			return fmt.Errorf("Go tool identity=%+v want=%+v err=%v", gotTool, tool, err)
		}
		return source.Recheck()
	}
}

func workCountRunC(t *testing.T, artifact, sourcePath, tempRoot string) workCountCChildResult {
	t.Helper()
	dumpPath := filepath.Join(tempRoot, "c.deep")
	if err := os.Remove(dumpPath); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	environment := workCountSanitizedEnv(os.Environ(), workCountCBuildEnvironment(), nil)
	stdout, stderr, err := workCountRunCaptured("", environment, workCountTimeout+staticCPerfWallGrace, artifact, sourcePath, dumpPath, strconv.FormatInt(workCountTimeout.Microseconds(), 10))
	if err != nil {
		t.Fatalf("static C work-count child: %v", err)
	}
	if len(stderr) != 0 {
		t.Fatalf("static C work-count child wrote stderr: %s", strings.TrimSpace(string(stderr)))
	}
	result := workCountDecodeCChild(t, stdout)
	result.DeepTreeSHA256 = workCountFileSHA(t, dumpPath)
	return result
}

func workCountRunGoAdmission(t *testing.T, sourceRoot, artifact, fixtureID, sourcePath, tempRoot string, environment workCountEnvironment) workCountGoChildResult {
	t.Helper()
	resultPath := filepath.Join(tempRoot, "go-admission.json")
	workCountRunGoChild(t, sourceRoot, artifact, "^TestWorkCountAdmissionChild$", fixtureID, sourcePath, resultPath, environment)
	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	return workCountDecodeGoChild(t, data)
}

func workCountRunGo(t *testing.T, sourceRoot, artifact, fixtureID, sourcePath, tempRoot string, environment workCountEnvironment) workCountTaggedChildResult {
	t.Helper()
	resultPath := filepath.Join(tempRoot, "go-work-count.json")
	workCountRunGoChild(t, sourceRoot, artifact, "^TestDiagnosticWorkCountChild$", fixtureID, sourcePath, resultPath, environment)
	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	return workCountDecodeTaggedGoChild(t, data)
}

func workCountRunGoChild(t *testing.T, sourceRoot, artifact, testPattern, fixtureID, sourcePath, resultPath string, environment workCountEnvironment) {
	t.Helper()
	if !filepath.IsAbs(sourceRoot) {
		t.Fatal("Go child source root must be an absolute snapshot path")
	}
	if strings.TrimSpace(fixtureID) == "" {
		t.Fatal("Go child fixture ID is empty")
	}
	if err := os.Remove(resultPath); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	runtimeEnv := workCountSanitizedEnv(os.Environ(), environment.Runtime, map[string]string{
		workCountFixtureEnv:     fixtureID,
		"GTS_WORK_COUNT_SOURCE": sourcePath,
		"GTS_WORK_COUNT_RESULT": resultPath,
	})
	// Package initialization can read fixtures before the selected test starts.
	// Resolve those files inside the authenticated source snapshot.
	stdout, stderr, err := workCountRunCaptured(sourceRoot, runtimeEnv, workCountTimeout+staticCPerfWallGrace, artifact, "-test.run", testPattern, "-test.count=1")
	if err != nil {
		t.Fatalf("Go child: %v: stdout=%s stderr=%s", err, strings.TrimSpace(string(stdout)), strings.TrimSpace(string(stderr)))
	}
	if len(stderr) != 0 {
		t.Fatalf("Go child wrote stderr: %s", strings.TrimSpace(string(stderr)))
	}
}

var workCountGoChildFields = []string{
	"schema", "engine", "fixture", "source_sha256", "source_bytes",
	"grammar_commit", "grammar_blob_sha256", "digest_format",
	"deep_tree_sha256", "root_end_byte", "root_has_error",
	"max_stacks_seen", "multi_stack_iterations", "multi_stack_tokens",
}

func workCountDecodeGoChild(t *testing.T, data []byte) workCountGoChildResult {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("decode child object: %v: %s", err, data)
	}
	workCountRequireKeys(t, "Go child", raw, workCountGoChildFields)
	var result workCountGoChildResult
	workCountDecodeExact(t, data, &result)
	return result
}

func workCountDecodeTaggedGoChild(t *testing.T, data []byte) workCountTaggedChildResult {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("decode child object: %v: %s", err, data)
	}
	fields := append(append([]string(nil), workCountGoChildFields...), "counters", "board_direct")
	workCountRequireKeys(t, "tagged Go child", raw, fields)
	workCountValidateGoCounterObject(t, raw["counters"])
	workCountValidateBoardDirectObject(t, raw["board_direct"])
	var result workCountTaggedChildResult
	workCountDecodeExact(t, data, &result)
	return result
}

func workCountDecodeCChild(t *testing.T, data []byte) workCountCChildResult {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("decode C child object: %v: %s", err, data)
	}
	workCountRequireKeys(t, "C child", raw, []string{"schema", "engine", "digest_format", "source_bytes", "root_end_byte", "root_has_error", "board_direct", "counters"})
	workCountValidateCounterObject(t, raw["counters"])
	workCountValidateBoardDirectObject(t, raw["board_direct"])
	var result workCountCChildResult
	workCountDecodeExact(t, data, &result)
	return result
}

func workCountValidateBoardDirectObject(t *testing.T, data json.RawMessage) {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	workCountRequireKeys(t, "board direct", raw, []string{
		"schema", "frontier_lexer_elections_available", "frontier_lexer_elections",
		"per_version_lex_requests_available", "per_version_lex_requests", "raw_main_lexer_invocations",
		"resolved_action_cells_examined", "raw_action_entries_beyond_first",
		"conflict_action_arms_admitted", "causal_conflict_forks",
		"predecessor_link_union_attempts", "predecessor_link_union_duplicate_noop",
		"predecessor_link_union_precedence_replaced", "predecessor_link_union_recursive_changed",
		"predecessor_link_union_alternate_appended", "predecessor_link_union_rejected",
		"alternate_predecessor_links_appended", "raw_selected_internal_nodes",
		"raw_selected_internal_parent_occurrences", "raw_selected_internal_leaf_occurrences", "overflow",
	})
	var direct workCountBoardDirectCounts
	workCountDecodeExact(t, data, &direct)
	if direct.Schema != "gts-work-count-board-direct/v3" || direct.Overflow {
		t.Fatalf("invalid board direct counts: %+v", direct)
	}
	if direct.FrontierLexerElectionsAvailable == direct.PerVersionLexRequestsAvailable || (!direct.FrontierLexerElectionsAvailable && direct.FrontierLexerElections != 0) || (!direct.PerVersionLexRequestsAvailable && direct.PerVersionLexRequests != 0) {
		t.Fatalf("invalid engine-specific lexer request availability: %+v", direct)
	}
	unionOutcomes := direct.PredecessorLinkUnionDuplicateNoop +
		direct.PredecessorLinkUnionPrecedenceReplaced +
		direct.PredecessorLinkUnionRecursiveChanged +
		direct.PredecessorLinkUnionAlternateAppended +
		direct.PredecessorLinkUnionRejected
	if unionOutcomes != direct.PredecessorLinkUnionAttempts {
		t.Fatalf("board direct union partition=%d attempts=%d", unionOutcomes, direct.PredecessorLinkUnionAttempts)
	}
	if direct.RawSelectedInternalNodes != direct.RawSelectedInternalParents+direct.RawSelectedInternalLeaves {
		t.Fatalf("board direct raw selected census invalid: %+v", direct)
	}
}

func workCountValidateCounterObject(t *testing.T, data json.RawMessage) {
	t.Helper()
	var counterRaw map[string]json.RawMessage
	if err := json.Unmarshal(data, &counterRaw); err != nil {
		t.Fatal(err)
	}
	counterKeys := append([]string{"contract", "overflow"}, workCountDirectFields...)
	counterKeys = append(counterKeys, workCountTerminalFields...)
	counterKeys = append(counterKeys, workCountProxyFields...)
	workCountRequireKeys(t, "counters", counterRaw, counterKeys)
}

func workCountValidateGoCounterObject(t *testing.T, data json.RawMessage) {
	t.Helper()
	var counterRaw map[string]json.RawMessage
	if err := json.Unmarshal(data, &counterRaw); err != nil {
		t.Fatal(err)
	}
	counterKeys := append([]string{"contract", "overflow"}, workCountDirectFields...)
	counterKeys = append(counterKeys, workCountTerminalFields...)
	counterKeys = append(counterKeys, workCountProxyFields...)
	counterKeys = append(counterKeys, "attempts", "outside_attempt", "convergence_frontier")
	workCountRequireKeys(t, "Go counters", counterRaw, counterKeys)
	workCountValidateCounterValuesObject(t, "Go outside-attempt counters", counterRaw["outside_attempt"])
	workCountValidateGoConvergenceObject(t, counterRaw["convergence_frontier"])
	var attempts []json.RawMessage
	if err := json.Unmarshal(counterRaw["attempts"], &attempts); err != nil {
		t.Fatal(err)
	}
	for i, attemptRaw := range attempts {
		var attempt map[string]json.RawMessage
		if err := json.Unmarshal(attemptRaw, &attempt); err != nil {
			t.Fatal(err)
		}
		workCountRequireKeys(t, fmt.Sprintf("Go attempt %d", i+1), attempt, []string{
			"index", "logical_rung", "operation_cause",
			"requested_max_stacks", "requested_max_nodes", "requested_max_merge_per_key",
			"caps_resolved", "resolved_max_stacks", "resolved_retry_pass", "resolved_max_merge_per_key",
			"resolved_stack_cull_trigger", "resolved_max_iterations", "resolved_max_nodes",
			"stop_reason", "root_has_error", "root_end_byte",
			"entry_to_caps", "caps_to_finalize", "finalize", "counters",
		})
		for _, key := range []string{"entry_to_caps", "caps_to_finalize", "finalize", "counters"} {
			workCountValidateCounterValuesObject(t, fmt.Sprintf("Go attempt %d %s", i+1, key), attempt[key])
		}
	}
}

func workCountValidateGoConvergenceObject(t *testing.T, data json.RawMessage) {
	t.Helper()
	raw := workCountDecodeRawObject(t, "Go convergence frontier", data)
	workCountRequireKeys(t, "Go convergence frontier", raw, []string{
		"capability", "max_events", "max_packed_path_count", "events_seen", "event_truncated",
		"arithmetic_overflow", "vocabulary_violation", "snapshot_count_capped", "snapshot_count_unknown",
		"path_walk_work_budget", "path_walk_work_used", "path_walk_work_exhausted",
		"events", "first_rejects", "aggregates",
	})
	var frontier workCountConvergence
	workCountDecodeExact(t, data, &frontier)
	var events, firstRejects []json.RawMessage
	if err := json.Unmarshal(raw["events"], &events); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw["first_rejects"], &firstRejects); err != nil {
		t.Fatal(err)
	}
	for i, event := range events {
		workCountValidateConvergenceEvent(t, fmt.Sprintf("Go convergence event %d", i+1), event)
	}
	for i, event := range firstRejects {
		workCountValidateConvergenceEvent(t, fmt.Sprintf("Go convergence first reject %d", i+1), event)
	}
	workCountValidateConvergenceAggregates(t, raw["aggregates"])
	workCountValidateGoConvergence(t, frontier)
}

func workCountValidateGoConvergence(t *testing.T, frontier workCountConvergence) {
	t.Helper()
	workCountValidateConvergenceBounds(t, frontier)
	workCountValidateConvergenceEvents(t, frontier)
	workCountValidatePostReduceLifecycle(t, frontier)
	reasons := workCountConvergenceRejectMap(frontier.Aggregates.Rejections)
	workCountValidateConvergenceFirstRejects(t, frontier, reasons)
	workCountValidateConvergenceTotals(t, frontier, reasons)
}

func workCountValidatePostReduceLifecycle(t *testing.T, frontier workCountConvergence) {
	t.Helper()
	isPostReduce := func(phase string) bool {
		return phase == "post_reduce_primary" || phase == "post_reduce_pending"
	}
	isTerminal := func(outcome string) bool {
		return outcome == "packed" || outcome == "rejected" || outcome == "mutation_partial"
	}
	for i, event := range frontier.Events {
		if !isPostReduce(event.Phase) {
			continue
		}
		switch {
		case event.Outcome == "candidate":
			if i+1 == len(frontier.Events) && frontier.EventTruncated {
				// The retained chronological prefix may end between the candidate
				// observation and its terminal event.
				continue
			}
			if i+1 == len(frontier.Events) {
				t.Fatalf("Go convergence post-reduce candidate sequence=%d has no terminal event", event.Sequence)
			}
			next := frontier.Events[i+1]
			if next.DecisionID != event.DecisionID || next.Phase != event.Phase || !isTerminal(next.Outcome) {
				t.Fatalf("Go convergence post-reduce candidate sequence=%d is not followed by one same-decision terminal: next=%+v", event.Sequence, next)
			}
		case isTerminal(event.Outcome):
			if i == 0 {
				t.Fatalf("Go convergence post-reduce terminal sequence=%d has no candidate event", event.Sequence)
			}
			previous := frontier.Events[i-1]
			if previous.DecisionID != event.DecisionID || previous.Phase != event.Phase || previous.Outcome != "candidate" {
				t.Fatalf("Go convergence post-reduce terminal sequence=%d is not preceded by its same-decision candidate: previous=%+v", event.Sequence, previous)
			}
		}
	}
}

func workCountValidateConvergenceBounds(t *testing.T, frontier workCountConvergence) {
	t.Helper()
	if frontier.Capability != "go_only_detailed_convergence" || frontier.MaxEvents != 256 || frontier.MaxPackedPathCount != 7 || frontier.PathWalkWorkBudget != 16*1024 {
		t.Fatalf("Go convergence capability/bounds=%q events=%d paths=%d work=%d", frontier.Capability, frontier.MaxEvents, frontier.MaxPackedPathCount, frontier.PathWalkWorkBudget)
	}
	if frontier.ArithmeticOverflow || frontier.VocabularyViolation || frontier.PathWalkWorkUsed > frontier.PathWalkWorkBudget || (frontier.PathWalkWorkExhausted && !frontier.SnapshotCountUnknown) {
		t.Fatalf("Go convergence integrity overflow=%v vocabulary=%v snapshots=%v/%v path_work=%d/%d exhausted=%v", frontier.ArithmeticOverflow, frontier.VocabularyViolation, frontier.SnapshotCountCapped, frontier.SnapshotCountUnknown, frontier.PathWalkWorkUsed, frontier.PathWalkWorkBudget, frontier.PathWalkWorkExhausted)
	}
	wantRetained := frontier.EventsSeen
	if wantRetained > uint64(frontier.MaxEvents) {
		wantRetained = uint64(frontier.MaxEvents)
	}
	if frontier.EventsSeen == 0 || uint64(len(frontier.Events)) != wantRetained || frontier.EventTruncated != (frontier.EventsSeen > uint64(frontier.MaxEvents)) {
		t.Fatalf("Go convergence event bound seen=%d retained=%d truncated=%v max=%d", frontier.EventsSeen, len(frontier.Events), frontier.EventTruncated, frontier.MaxEvents)
	}
}

func workCountValidateConvergenceEvents(t *testing.T, frontier workCountConvergence) {
	t.Helper()
	for i, event := range frontier.Events {
		if event.Sequence != uint64(i+1) {
			t.Fatalf("Go convergence event %d sequence=%d want=%d", i+1, event.Sequence, i+1)
		}
		workCountValidateConvergenceEventValue(t, fmt.Sprintf("Go convergence event %d", i+1), event)
		if event.SnapshotCapped && !frontier.SnapshotCountCapped {
			t.Fatalf("Go convergence event %d capped snapshot missing top-level signal", i+1)
		}
		if event.SnapshotUnknown && !frontier.SnapshotCountUnknown {
			t.Fatalf("Go convergence event %d unknown snapshot missing top-level signal", i+1)
		}
	}
	var retainedCapped, retainedUnknown bool
	for _, event := range frontier.Events {
		retainedCapped = retainedCapped || event.SnapshotCapped
		retainedUnknown = retainedUnknown || event.SnapshotUnknown
	}
	if !frontier.EventTruncated && (frontier.SnapshotCountCapped != retainedCapped || frontier.SnapshotCountUnknown != retainedUnknown) {
		t.Fatalf("Go convergence top-level snapshot OR capped=%v/%v unknown=%v/%v", frontier.SnapshotCountCapped, retainedCapped, frontier.SnapshotCountUnknown, retainedUnknown)
	}
}

func workCountValidateConvergenceFirstRejects(t *testing.T, frontier workCountConvergence, reasons map[string]uint64) {
	t.Helper()
	seenReasons := make(map[string]bool, len(frontier.FirstRejects))
	seenSequences := make(map[uint64]bool, len(frontier.FirstRejects))
	for i, event := range frontier.FirstRejects {
		workCountValidateConvergenceEventValue(t, fmt.Sprintf("Go convergence first reject %d", i+1), event)
		if event.Sequence == 0 || event.Sequence > frontier.EventsSeen || event.Outcome != "rejected" || event.RejectReason == "" || seenReasons[event.RejectReason] || seenSequences[event.Sequence] {
			t.Fatalf("Go convergence first reject %d invalid/duplicate: %+v", i+1, event)
		}
		seenReasons[event.RejectReason] = true
		seenSequences[event.Sequence] = true
		if event.Sequence <= uint64(len(frontier.Events)) {
			chronological := frontier.Events[event.Sequence-1]
			if !reflect.DeepEqual(event, chronological) {
				t.Fatalf("Go convergence first reject %d is not exact chronological event %d", i+1, event.Sequence)
			}
		}
		for _, chronological := range frontier.Events {
			if chronological.Sequence >= event.Sequence {
				break
			}
			if chronological.RejectReason == event.RejectReason {
				t.Fatalf("Go convergence first reject %d reason=%s is not minimal; earlier sequence=%d", i+1, event.RejectReason, chronological.Sequence)
			}
		}
	}
	for reason, count := range reasons {
		if (count != 0) != seenReasons[reason] {
			t.Fatalf("Go convergence first-reject coverage reason=%s count=%d retained=%v", reason, count, seenReasons[reason])
		}
	}
}

func workCountValidateConvergenceTotals(t *testing.T, frontier workCountConvergence, reasons map[string]uint64) {
	t.Helper()
	phases := workCountConvergencePhaseMap(frontier.Aggregates.Phases)
	outcomes := workCountConvergenceOutcomeMap(frontier.Aggregates.Outcomes)
	if workCountSumUint64Fields(t, "Go convergence phases", phases) != frontier.EventsSeen || workCountSumUint64Fields(t, "Go convergence outcomes", outcomes) != frontier.EventsSeen {
		t.Fatalf("Go convergence aggregate/event mismatch seen=%d phases=%v outcomes=%v", frontier.EventsSeen, phases, outcomes)
	}
	if workCountSumUint64Fields(t, "Go convergence rejections", reasons) != outcomes["rejected"] {
		t.Fatalf("Go convergence rejection sum=%d rejected=%d", workCountSumUint64Fields(t, "Go convergence rejections", reasons), outcomes["rejected"])
	}
	if frontier.Aggregates.AcceptedHeads != outcomes["accepted_head"] {
		t.Fatalf("Go convergence accepted heads scalar=%d outcomes=%d", frontier.Aggregates.AcceptedHeads, outcomes["accepted_head"])
	}
	if frontier.Aggregates.MutationPartial != outcomes["mutation_partial"] {
		t.Fatalf("Go convergence mutation-partial scalar=%d outcomes=%d", frontier.Aggregates.MutationPartial, outcomes["mutation_partial"])
	}
	if frontier.Aggregates.CapHits < outcomes["cap_hit"] {
		t.Fatalf("Go convergence cap-hit scalar=%d cap-hit outcomes=%d", frontier.Aggregates.CapHits, outcomes["cap_hit"])
	}
	workCountValidatePackedRootTotals(t, frontier, outcomes["packed_root_emitted"])
	workCountValidateRetainedAggregateBounds(t, frontier, phases, outcomes, reasons)
}

func workCountValidateRetainedAggregateBounds(t *testing.T, frontier workCountConvergence, phases, outcomes, reasons map[string]uint64) {
	t.Helper()
	retainedPhases := make(map[string]uint64, len(phases))
	retainedOutcomes := make(map[string]uint64, len(outcomes))
	retainedReasons := make(map[string]uint64, len(reasons))
	var beforeCandidates, afterCandidates, beforeLinks, afterLinks, beforePaths, afterPaths uint64
	for _, event := range frontier.Events {
		retainedPhases[event.Phase]++
		retainedOutcomes[event.Outcome]++
		if event.RejectReason != "" {
			retainedReasons[event.RejectReason]++
		}
		beforeCandidates += uint64(event.CandidateCountBefore)
		afterCandidates += uint64(event.CandidateCountAfter)
		beforeLinks += uint64(event.LinksBefore)
		afterLinks += uint64(event.LinksAfter)
		beforePaths += uint64(event.PathsBefore)
		afterPaths += uint64(event.PathsAfter)
	}
	checkMap := func(label string, aggregate, retained map[string]uint64) {
		for key, got := range aggregate {
			want := retained[key]
			if (!frontier.EventTruncated && got != want) || (frontier.EventTruncated && got < want) {
				t.Fatalf("Go convergence %s aggregate %s=%d retained=%d truncated=%v", label, key, got, want, frontier.EventTruncated)
			}
		}
	}
	checkMap("phase", phases, retainedPhases)
	checkMap("outcome", outcomes, retainedOutcomes)
	checkMap("reason", reasons, retainedReasons)
	values := [][3]uint64{
		{frontier.Aggregates.CandidateCountBeforeTotal, beforeCandidates, 0}, {frontier.Aggregates.CandidateCountAfterTotal, afterCandidates, 0},
		{frontier.Aggregates.LinksBeforeTotal, beforeLinks, 0}, {frontier.Aggregates.LinksAfterTotal, afterLinks, 0},
		{frontier.Aggregates.PathsBeforeTotal, beforePaths, 0}, {frontier.Aggregates.PathsAfterTotal, afterPaths, 0},
	}
	for i, pair := range values {
		if (!frontier.EventTruncated && pair[0] != pair[1]) || (frontier.EventTruncated && pair[0] < pair[1]) {
			t.Fatalf("Go convergence scalar total %d=%d retained=%d truncated=%v", i, pair[0], pair[1], frontier.EventTruncated)
		}
	}
	workCountValidateRetainedScalarTotals(t, frontier)
}

func workCountValidatePackedRootTotals(t *testing.T, frontier workCountConvergence, packedRootEvents uint64) {
	t.Helper()
	if packedRootEvents == 0 {
		if frontier.Aggregates.PackedRoots != 0 {
			t.Fatalf("Go convergence packed roots=%d without packed-root events", frontier.Aggregates.PackedRoots)
		}
	} else {
		maxRoots := packedRootEvents * uint64(frontier.MaxPackedPathCount-1)
		if frontier.Aggregates.PackedRoots < packedRootEvents || frontier.Aggregates.PackedRoots > maxRoots || frontier.Aggregates.PackedRoots > frontier.Aggregates.PathsAfterTotal {
			t.Fatalf("Go convergence packed roots=%d events=%d max=%d paths-after=%d", frontier.Aggregates.PackedRoots, packedRootEvents, maxRoots, frontier.Aggregates.PathsAfterTotal)
		}
	}
}

func workCountValidateRetainedScalarTotals(t *testing.T, frontier workCountConvergence) {
	t.Helper()
	var mutationPartial, capHits, acceptedHeads, packedRoots uint64
	for _, event := range frontier.Events {
		if event.MutationPartial {
			mutationPartial++
		}
		if event.CapHit {
			capHits++
		}
		if event.Outcome == "accepted_head" {
			acceptedHeads++
		}
		if event.Outcome == "packed_root_emitted" {
			packedRoots += uint64(event.PathsAfter)
		}
	}
	exact := !frontier.EventTruncated
	bad := func(total, retained uint64) bool { return (exact && total != retained) || (!exact && total < retained) }
	if bad(frontier.Aggregates.MutationPartial, mutationPartial) || bad(frontier.Aggregates.CapHits, capHits) || bad(frontier.Aggregates.AcceptedHeads, acceptedHeads) || bad(frontier.Aggregates.PackedRoots, packedRoots) {
		t.Fatalf("Go convergence scalar aggregate drift mutation=%d/%d caps=%d/%d accepted=%d/%d roots=%d/%d", frontier.Aggregates.MutationPartial, mutationPartial, frontier.Aggregates.CapHits, capHits, frontier.Aggregates.AcceptedHeads, acceptedHeads, frontier.Aggregates.PackedRoots, packedRoots)
	}
}

func workCountValidateConvergenceEventValue(t *testing.T, label string, event workCountConvergenceEvent) {
	t.Helper()
	workCountValidateConvergenceEventIdentity(t, label, event)
	workCountValidateConvergenceEventSnapshots(t, label, event)
	workCountValidateConvergenceEventHeads(t, label, event)
}

func workCountValidateConvergenceEventIdentity(t *testing.T, label string, event workCountConvergenceEvent) {
	t.Helper()
	if event.Sequence == 0 || event.Attempt != 1 || !workCountStringIn(event.Phase, workCountConvergenceManifestPhases) || !workCountStringIn(event.Outcome, workCountConvergenceManifestOutcomes) || (event.RejectReason != "" && !workCountStringIn(event.RejectReason, workCountConvergenceManifestReasons)) {
		t.Fatalf("%s closed vocabulary/identity=%+v", label, event)
	}
	if (event.Outcome == "rejected") != (event.RejectReason != "") {
		t.Fatalf("%s rejected/reason mismatch outcome=%q reason=%q", label, event.Outcome, event.RejectReason)
	}
	if !workCountConvergencePhaseAllowsOutcome(event.Phase, event.Outcome) {
		t.Fatalf("%s phase/outcome mismatch phase=%q outcome=%q", label, event.Phase, event.Outcome)
	}
	if (event.DecisionID == 0) != (event.Target.Status == "absent" && event.Candidate.Status == "absent") {
		t.Fatalf("%s decision/head identity mismatch decision=%d target=%q candidate=%q", label, event.DecisionID, event.Target.Status, event.Candidate.Status)
	}
	if !event.LookaheadPresent && (event.LookaheadSymbol != 0 || event.LookaheadStartByte != 0 || event.LookaheadEndByte != 0 || event.LookaheadNoLookahead || event.LookaheadExternal) {
		t.Fatalf("%s unset lookahead carries data: %+v", label, event)
	}
	if !event.ElectionScannerComparable && (event.ElectionScannerCheckpointPresent || event.ElectionScannerCheckpointHash != 0) {
		t.Fatalf("%s non-comparable election scanner carries checkpoint evidence", label)
	}
	if !event.ElectionScannerCheckpointPresent && event.ElectionScannerCheckpointHash != 0 {
		t.Fatalf("%s absent election scanner checkpoint carries hash=%d", label, event.ElectionScannerCheckpointHash)
	}
	if event.ElectionScannerCheckpointPresent && !event.ElectionScannerComparable {
		t.Fatalf("%s present election scanner checkpoint is not comparable", label)
	}
}

func workCountValidateConvergenceEventSnapshots(t *testing.T, label string, event workCountConvergenceEvent) {
	t.Helper()
	if event.Outcome == "observation_unknown" && (!event.SnapshotUnknown || event.CapHit || event.PathsAfter != 0) {
		t.Fatalf("%s unknown observation claims result: %+v", label, event)
	}
	if event.Outcome == "packed_root_emitted" && (event.SnapshotUnknown || event.PathsAfter == 0) {
		t.Fatalf("%s packed-root emission is not proven: %+v", label, event)
	}
	if event.PathsBefore > 7 || event.PathsAfter > 7 {
		t.Fatalf("%s packed-path snapshot exceeds contract bound: before=%d after=%d", label, event.PathsBefore, event.PathsAfter)
	}
	if (event.PathsBefore == 7 || event.PathsAfter == 7) && !event.SnapshotCapped && !event.SnapshotUnknown {
		t.Fatalf("%s packed-path sentinel lacks capped or unknown evidence: before=%d after=%d", label, event.PathsBefore, event.PathsAfter)
	}
	if event.LookaheadStartByte > event.LookaheadEndByte {
		t.Fatalf("%s lookahead range is reversed: start=%d end=%d", label, event.LookaheadStartByte, event.LookaheadEndByte)
	}
}

func workCountValidateConvergenceEventHeads(t *testing.T, label string, event workCountConvergenceEvent) {
	t.Helper()
	workCountValidateConvergenceHeadValue(t, label+" target", event.Target)
	workCountValidateConvergenceHeadValue(t, label+" candidate", event.Candidate)
	if event.Outcome == "accepted_head" && (event.Target.Status != "accepted" || event.Candidate.Status != "absent") {
		t.Fatalf("%s accepted-head event target status=%q", label, event.Target.Status)
	}
	if event.Phase == "final_expand" || event.Phase == "final_select" {
		if event.Target.Status == "absent" || event.Candidate.Status != "absent" {
			t.Fatalf("%s final event head identity target=%q candidate=%q", label, event.Target.Status, event.Candidate.Status)
		}
	}
	if event.Outcome == "selected" && event.Candidate.Status != "absent" {
		t.Fatalf("%s selected event has candidate head=%q", label, event.Candidate.Status)
	}
}

func workCountConvergencePhaseAllowsOutcome(phase, outcome string) bool {
	return workCountStringIn(outcome, workCountConvergencePhaseOutcomes[phase])
}

func workCountValidateConvergenceHeadValue(t *testing.T, label string, head workCountConvergenceHead) {
	t.Helper()
	if !workCountStringIn(head.Status, []string{"absent", "live", "dead", "accepted", "paused"}) {
		t.Fatalf("%s status=%q is outside the closed vocabulary", label, head.Status)
	}
	if head.Status == "absent" {
		if head != (workCountConvergenceHead{Status: "absent"}) {
			t.Fatalf("%s absent head carries data: %+v", label, head)
		}
		return
	}
	if !head.ErrorCostComparable && head.ErrorCost != 0 {
		t.Fatalf("%s non-comparable head reports error cost=%d", label, head.ErrorCost)
	}
	if !head.HasGSS && (head.LinkCount != 0 || head.LinkCountCapped || head.HeadContentHash != 0 || head.PredecessorHash != 0) {
		t.Fatalf("%s non-GSS head reports graph identity: %+v", label, head)
	}
	if head.LinkCountCapped && head.LinkCount != math.MaxUint16 {
		t.Fatalf("%s capped link count=%d want=%d", label, head.LinkCount, uint16(math.MaxUint16))
	}
	wantStatus := "live"
	if head.Dead {
		wantStatus = "dead"
	} else if head.Accepted {
		wantStatus = "accepted"
	} else if head.Paused {
		wantStatus = "paused"
	}
	if head.Status != wantStatus {
		t.Fatalf("%s status/bit mismatch: %+v", label, head)
	}
}

func workCountValidateConvergenceEvent(t *testing.T, label string, data json.RawMessage) {
	t.Helper()
	raw := workCountDecodeRawObject(t, label, data)
	workCountRequireKeysWithOptional(t, label, raw, []string{
		"sequence", "attempt", "iteration", "decision_id",
		"lookahead_symbol", "lookahead_start_byte", "lookahead_end_byte", "lookahead_present", "lookahead_no_lookahead", "lookahead_external",
		"election_scanner_comparable", "election_scanner_checkpoint_present", "election_scanner_checkpoint_hash",
		"phase", "outcome", "target", "candidate",
		"candidate_count_before", "candidate_count_after", "links_before", "links_after", "packed_paths_before", "packed_paths_after",
		"mutation_partial", "cap_hit", "snapshot_capped", "snapshot_unknown",
	}, []string{"reject_reason", "detail"})
	workCountValidateConvergenceHead(t, label+" target", raw["target"])
	workCountValidateConvergenceHead(t, label+" candidate", raw["candidate"])
	var event workCountConvergenceEvent
	workCountDecodeExact(t, data, &event)
	workCountValidateConvergenceEventValue(t, label, event)
}

func workCountValidateConvergenceHead(t *testing.T, label string, data json.RawMessage) {
	t.Helper()
	raw := workCountDecodeRawObject(t, label, data)
	workCountRequireKeys(t, label, raw, []string{
		"state", "byte", "status", "has_error", "recovering", "shifted", "score", "branch_order", "depth",
		"dead", "accepted", "paused", "node_baseline", "error_cost", "error_cost_comparable",
		"has_gss", "link_count", "link_count_capped",
		"head_content_hash", "predecessor_content_hash",
	})
	var head workCountConvergenceHead
	workCountDecodeExact(t, data, &head)
	workCountValidateConvergenceHeadValue(t, label, head)
}

func workCountValidateConvergenceAggregates(t *testing.T, data json.RawMessage) {
	t.Helper()
	raw := workCountDecodeRawObject(t, "Go convergence aggregates", data)
	workCountRequireKeys(t, "Go convergence aggregates", raw, []string{
		"phases", "outcomes", "rejections", "candidate_count_before_total", "candidate_count_after_total",
		"links_before_total", "links_after_total", "packed_paths_before_total", "packed_paths_after_total",
		"mutation_partial", "cap_hits", "accepted_heads", "packed_roots_emitted",
	})
	workCountRequireKeys(t, "Go convergence phase aggregates", workCountDecodeRawObject(t, "Go convergence phase aggregates", raw["phases"]), workCountConvergenceManifestPhases)
	workCountRequireKeys(t, "Go convergence outcome aggregates", workCountDecodeRawObject(t, "Go convergence outcome aggregates", raw["outcomes"]), workCountConvergenceManifestOutcomes)
	workCountRequireKeys(t, "Go convergence rejection aggregates", workCountDecodeRawObject(t, "Go convergence rejection aggregates", raw["rejections"]), workCountConvergenceManifestReasons)
	var aggregates workCountConvergenceTotals
	workCountDecodeExact(t, data, &aggregates)
}

func workCountConvergencePhaseMap(value workCountConvergencePhaseTotals) map[string]uint64 {
	return map[string]uint64{
		"reduce_window_select": value.ReduceWindowSelect,
		"post_reduce_primary":  value.PostReducePrimary,
		"post_reduce_pending":  value.PostReducePending,
		"boundary_gss":         value.BoundaryGSS,
		"boundary_equivalence": value.BoundaryEquivalence,
		"boundary_cull":        value.BoundaryCull,
		"pending_work":         value.PendingWork,
		"final_expand":         value.FinalExpand,
		"final_select":         value.FinalSelect,
		"terminal_accept":      value.TerminalAccept,
	}
}

func workCountConvergenceOutcomeMap(value workCountConvergenceOutcomeTotals) map[string]uint64 {
	return map[string]uint64{
		"candidate":           value.Candidate,
		"packed":              value.Packed,
		"rejected":            value.Rejected,
		"pending_queued":      value.PendingQueued,
		"pending_drained":     value.PendingDrained,
		"pending_discarded":   value.PendingDiscarded,
		"accepted_head":       value.AcceptedHead,
		"packed_root_emitted": value.PackedRoot,
		"mutation_partial":    value.MutationPartial,
		"cap_hit":             value.CapHit,
		"observation_unknown": value.ObservationUnknown,
		"selected":            value.Selected,
	}
}

func workCountConvergenceRejectMap(value workCountConvergenceRejectTotals) map[string]uint64 {
	return map[string]uint64{
		"status":         value.Status,
		"score":          value.Score,
		"shifted":        value.Shifted,
		"error_cost":     value.ErrorCost,
		"not_gss":        value.NotGSS,
		"not_clean":      value.NotClean,
		"distinct_shape": value.DistinctShape,
		"preflight":      value.Preflight,
		"cull":           value.Cull,
	}
}

func workCountSumUint64Fields(t *testing.T, label string, values map[string]uint64) uint64 {
	t.Helper()
	var sum uint64
	for field, value := range values {
		if ^uint64(0)-sum < value {
			t.Fatalf("%s field %s sum overflow", label, field)
		}
		sum += value
	}
	return sum
}

func workCountValidateCounterValuesObject(t *testing.T, label string, data json.RawMessage) {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	fields := append(append([]string(nil), workCountDirectFields...), workCountTerminalFields...)
	fields = append(fields, workCountProxyFields...)
	workCountRequireKeys(t, label, raw, fields)
}

func workCountDecodeExact(t *testing.T, data []byte, result any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(result); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("child JSON has trailing value: %v", err)
	}
}

func workCountValidateGoChild(t *testing.T, label, expectedSchema, expectedEngine string, result workCountGoChildResult, fixture benchfixtures.LoadedFixture) {
	t.Helper()
	if result.Schema != expectedSchema || result.DigestFormat != benchfixtures.DeepTreeDigestVersion {
		t.Fatalf("%s mixed contract: schema=%q digest=%q", label, result.Schema, result.DigestFormat)
	}
	if result.Engine != expectedEngine {
		t.Fatalf("%s engine=%q want=%q", label, result.Engine, expectedEngine)
	}
	if result.Fixture != fixture.Fixture.ID || result.SourceSHA256 != fixture.Fixture.SHA256 {
		t.Fatalf("%s fixture/source=%q/%s want=%q/%s", label, result.Fixture, result.SourceSHA256, fixture.Fixture.ID, fixture.Fixture.SHA256)
	}
	if result.GrammarCommit != benchfixtures.GoGrammarCommit || result.GrammarBlobSHA256 != benchfixtures.GoGrammarBlobSHA256 {
		t.Fatalf("%s grammar=%s/%s want=%s/%s", label, result.GrammarCommit, result.GrammarBlobSHA256, benchfixtures.GoGrammarCommit, benchfixtures.GoGrammarBlobSHA256)
	}
	if result.SourceBytes != uint32(len(fixture.Source)) || result.RootEndByte != uint32(len(fixture.Source)) || result.RootHasError {
		t.Fatalf("%s span/error admission: source=%d root_end=%d error=%v", label, result.SourceBytes, result.RootEndByte, result.RootHasError)
	}
	if result.DeepTreeSHA256 != fixture.Fixture.DeepTreeSHA256 {
		t.Fatalf("%s digest=%s want=%s", label, result.DeepTreeSHA256, fixture.Fixture.DeepTreeSHA256)
	}
	identity := fixture.Fixture.WorkloadIdentity
	if result.MaxStacksSeen < identity.MinMaxStacksSeen || result.MultiStackIters < identity.MinMultiStackIterations || result.MultiStackTokens < identity.MinMultiStackTokens {
		t.Fatalf("%s GLR identity stacks=%d iterations=%d tokens=%d", label, result.MaxStacksSeen, result.MultiStackIters, result.MultiStackTokens)
	}
}

func workCountValidateCChild(t *testing.T, label, expectedEngine string, result workCountCChildResult, fixture benchfixtures.LoadedFixture, digest string) {
	t.Helper()
	if result.Schema != workCountCChildSchema || result.DigestFormat != benchfixtures.DeepTreeDigestVersion {
		t.Fatalf("%s mixed contract: schema=%q digest=%q", label, result.Schema, result.DigestFormat)
	}
	if result.Engine != expectedEngine {
		t.Fatalf("%s engine=%q want=%q", label, result.Engine, expectedEngine)
	}
	if result.SourceBytes != uint32(len(fixture.Source)) || result.RootEndByte != uint32(len(fixture.Source)) || result.RootHasError {
		t.Fatalf("%s span/error admission: source=%d root_end=%d error=%v", label, result.SourceBytes, result.RootEndByte, result.RootHasError)
	}
	if result.DeepTreeSHA256 != digest {
		t.Fatalf("%s instrumented digest=%s want uninstrumented=%s", label, result.DeepTreeSHA256, digest)
	}
	workCountValidateCounters(t, label, result.Counters)
}

func workCountValidateCounters(t *testing.T, label string, c workCountCounters) {
	t.Helper()
	if c.Contract != workCountContract || c.Overflow {
		t.Fatalf("%s counters contract=%q overflow=%v", label, c.Contract, c.Overflow)
	}
	if c.SelectedNodes == 0 || c.SelectedNodes != c.SelectedParentNodes+c.SelectedLeafNodes {
		t.Fatalf("%s selected node census invalid: total=%d parent=%d leaf=%d", label, c.SelectedNodes, c.SelectedParentNodes, c.SelectedLeafNodes)
	}
	if c.Reductions != c.ReductionPopRequests {
		t.Fatalf("%s reduce/pop-request mismatch: reductions=%d requests=%d", label, c.Reductions, c.ReductionPopRequests)
	}
}

func workCountValidateGoCounters(t *testing.T, label string, c workCountGoCounters, sourceBytes uint32) {
	t.Helper()
	workCountValidateCounters(t, label, c.workCountCounters)
	if c.Convergence.Capability == "" {
		t.Fatalf("%s convergence frontier is absent", label)
	}
	workCountValidateGoConvergence(t, c.Convergence)
	if len(c.Attempts) != 1 {
		t.Fatalf("%s attempts=%d want=1", label, len(c.Attempts))
	}
	attempt := c.Attempts[0]
	if attempt.Index != 1 || attempt.LogicalRung != "initial_full" || attempt.OperationCause != "fresh_dfa_full_parse" {
		t.Fatalf("%s attempt identity=%d/%q/%q", label, attempt.Index, attempt.LogicalRung, attempt.OperationCause)
	}
	if !attempt.CapsResolved {
		t.Fatalf("%s attempt caps were not resolved", label)
	}
	if attempt.StopReason != string("accepted") || attempt.RootHasError || attempt.RootEndByte != sourceBytes {
		t.Fatalf("%s attempt result stop=%q error=%v end=%d want=%d", label, attempt.StopReason, attempt.RootHasError, attempt.RootEndByte, sourceBytes)
	}
	if c.AcceptActions != 3 || attempt.Counters.AcceptActions != 3 {
		t.Fatalf("%s accept actions aggregate=%d attempt=%d want=3", label, c.AcceptActions, attempt.Counters.AcceptActions)
	}
	workCountRequireValueSum(t, label+" attempt phases", attempt.Counters, attempt.EntryToCaps, attempt.CapsToFinalize, attempt.Finalize)
	parts := make([]workCountCounterValues, 0, len(c.Attempts)+1)
	for _, item := range c.Attempts {
		parts = append(parts, item.Counters)
	}
	parts = append(parts, c.OutsideAttempt)
	workCountRequireValueSum(t, label+" aggregate attribution", c.workCountCounterValues, parts...)
}

func workCountRequireValueSum(t *testing.T, label string, total workCountCounterValues, parts ...workCountCounterValues) {
	t.Helper()
	totalValues := workCountCounterValueMap(total)
	sums := make(map[string]uint64, len(totalValues))
	for _, part := range parts {
		for field, value := range workCountCounterValueMap(part) {
			if ^uint64(0)-sums[field] < value {
				t.Fatalf("%s field %s sum overflow", label, field)
			}
			sums[field] += value
		}
	}
	for field, want := range totalValues {
		if got := sums[field]; got != want {
			t.Fatalf("%s field %s sum=%d want=%d", label, field, got, want)
		}
	}
}

func workCountCounterValueMap(values workCountCounterValues) map[string]uint64 {
	data, _ := json.Marshal(values)
	var raw map[string]uint64
	_ = json.Unmarshal(data, &raw)
	return raw
}

func workCountRatios(goCounts, cCounts workCountCounters) []workCountRatio {
	goValues := workCountValues(goCounts)
	cValues := workCountValues(cCounts)
	var ratios []workCountRatio
	for _, classFields := range []struct {
		class  string
		fields []string
	}{{"direct", workCountDirectFields}, {"terminal", workCountTerminalFields}, {"proxy", workCountProxyFields}} {
		for _, field := range classFields.fields {
			ratio := workCountRatio{Field: field, Class: classFields.class, Go: goValues[field], C: cValues[field]}
			if ratio.C != 0 {
				value := float64(ratio.Go) / float64(ratio.C)
				ratio.GoOverC = &value
			}
			ratios = append(ratios, ratio)
		}
	}
	return ratios
}

func workCountValues(c workCountCounters) map[string]uint64 {
	data, _ := json.Marshal(c)
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(data, &raw)
	values := make(map[string]uint64, len(raw))
	for key, value := range raw {
		var count uint64
		if json.Unmarshal(value, &count) == nil {
			values[key] = count
		}
	}
	return values
}

func workCountConstructionSurplus(t *testing.T, label string, c workCountCounters) uint64 {
	t.Helper()
	if ^uint64(0)-c.LeafConstructionsProxy < c.ParentConstructionsProxy {
		t.Fatalf("%s construction sum overflow", label)
	}
	constructed := c.LeafConstructionsProxy + c.ParentConstructionsProxy
	if constructed < c.SelectedNodes {
		t.Fatalf("%s constructed payloads=%d below selected nodes=%d", label, constructed, c.SelectedNodes)
	}
	return constructed - c.SelectedNodes
}

func workCountValidateManifest(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	workCountRequireKeys(t, "manifest", raw, []string{"schema", "counter_contract", "scope", "direct", "terminal", "proxy", "derived", "attempt_attribution", "convergence_frontier", "admission"})
	var manifest struct {
		Schema          string            `json:"schema"`
		CounterContract string            `json:"counter_contract"`
		Scope           string            `json:"scope"`
		Direct          map[string]string `json:"direct"`
		Terminal        map[string]string `json:"terminal"`
		Proxy           map[string]string `json:"proxy"`
		Derived         map[string]string `json:"derived"`
		Attempt         struct {
			LogicalRungs                     []string `json:"logical_rungs"`
			ResolvedRetryPassIsIndependent   bool     `json:"resolved_retry_pass_is_independent"`
			Phases                           []string `json:"phases"`
			RequireAggregateEqualsAttributed bool     `json:"require_aggregate_equals_attempts_plus_outside"`
			CanonicalAttempts                int      `json:"canonical_query_compile_attempts"`
			CanonicalAcceptActions           uint64   `json:"canonical_query_compile_accept_actions"`
			RetryWitness                     struct {
				Path          string   `json:"path"`
				Bytes         int      `json:"bytes"`
				SHA256        string   `json:"sha256"`
				ExpectedRungs []string `json:"expected_rungs"`
			} `json:"retry_witness"`
			StraightLRControl struct {
				Path              string `json:"path"`
				Bytes             int    `json:"bytes"`
				SHA256            string `json:"sha256"`
				ExpectedMaxStacks int    `json:"expected_max_stacks"`
			} `json:"straight_lr_control"`
		} `json:"attempt_attribution"`
		Convergence map[string]any `json:"convergence_frontier"`
		Admission   map[string]any `json:"admission"`
	}
	workCountDecodeExact(t, data, &manifest)
	if manifest.Schema != "gts-work-count-contract/v2" || manifest.CounterContract != workCountContract || strings.TrimSpace(manifest.Scope) == "" {
		t.Fatalf("manifest contract mismatch: schema=%q counters=%q scope=%q", manifest.Schema, manifest.CounterContract, manifest.Scope)
	}
	workCountRequireStringKeys(t, "manifest direct", manifest.Direct, workCountDirectFields)
	workCountRequireStringKeys(t, "manifest terminal", manifest.Terminal, workCountTerminalFields)
	workCountRequireStringKeys(t, "manifest proxy", manifest.Proxy, workCountProxyFields)
	workCountRequireStringKeys(t, "manifest derived", manifest.Derived, []string{"construction_surplus"})
	var attemptRaw map[string]json.RawMessage
	if err := json.Unmarshal(raw["attempt_attribution"], &attemptRaw); err != nil {
		t.Fatal(err)
	}
	attempt := make(map[string]json.RawMessage, len(attemptRaw))
	for key := range attemptRaw {
		attempt[key] = nil
	}
	workCountRequireKeys(t, "manifest attempt attribution", attempt, []string{
		"logical_rungs", "resolved_retry_pass_is_independent", "phases",
		"require_aggregate_equals_attempts_plus_outside", "canonical_query_compile_attempts",
		"canonical_query_compile_accept_actions", "retry_witness", "straight_lr_control",
	})
	if got := strings.Join(manifest.Attempt.LogicalRungs, ","); got != "initial_full,initial_merge,clean_wide,clean_wide_merge,recovery_wide_or_node,secondary_node,final_merge" {
		t.Fatalf("manifest logical rungs=%q", got)
	}
	if got := strings.Join(manifest.Attempt.Phases, ","); got != "entry_to_caps,caps_to_finalize,finalize" {
		t.Fatalf("manifest attempt phases=%q", got)
	}
	if !manifest.Attempt.ResolvedRetryPassIsIndependent || !manifest.Attempt.RequireAggregateEqualsAttributed ||
		manifest.Attempt.CanonicalAttempts != 1 || manifest.Attempt.CanonicalAcceptActions != 3 {
		t.Fatalf("manifest attribution invariants are incomplete: %+v", manifest.Attempt)
	}
	if got := strings.Join(manifest.Attempt.RetryWitness.ExpectedRungs, ","); got != "initial_full,initial_merge" {
		t.Fatalf("manifest retry witness rungs=%q", got)
	}
	if manifest.Attempt.StraightLRControl.ExpectedMaxStacks != 1 {
		t.Fatalf("manifest straight-LR max stacks=%d want=1", manifest.Attempt.StraightLRControl.ExpectedMaxStacks)
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(path), "..", ".."))
	workCountValidateFrozenWitness(t, repoRoot, "retry", manifest.Attempt.RetryWitness.Path, manifest.Attempt.RetryWitness.Bytes, manifest.Attempt.RetryWitness.SHA256)
	workCountValidateFrozenWitness(t, repoRoot, "straight-LR", manifest.Attempt.StraightLRControl.Path, manifest.Attempt.StraightLRControl.Bytes, manifest.Attempt.StraightLRControl.SHA256)
	convergence := make(map[string]json.RawMessage, len(manifest.Convergence))
	for key := range manifest.Convergence {
		convergence[key] = nil
	}
	workCountRequireKeys(t, "manifest convergence frontier", convergence, []string{
		"status", "max_events", "record_first_reject_per_reason", "identity", "outcomes", "snapshots",
		"include_cumulative_direct_and_proxy_counts", "terminal_counters", "interpretation",
	})
	if status, _ := manifest.Convergence["status"].(string); status != "schema_reserved_not_implemented" {
		t.Fatalf("manifest convergence status=%q", status)
	}
	if maxEvents, _ := manifest.Convergence["max_events"].(float64); maxEvents != 256 {
		t.Fatalf("manifest convergence max events=%v", manifest.Convergence["max_events"])
	}
	admission := make(map[string]json.RawMessage, len(manifest.Admission))
	for key := range manifest.Admission {
		admission[key] = nil
	}
	workCountRequireKeys(t, "manifest admission", admission, []string{
		"digest_format", "fixture", "require_uninstrumented_go_static_c_digest_match_first",
		"require_instrumented_digest_match", "require_full_span", "require_clean_root",
		"require_no_overflow", "require_exact_fields", "require_ordinary_untagged_go_child_first",
		"require_clean_git_source_for_authoritative_receipt", "require_private_content_addressed_go_source_snapshot",
		"require_private_c_input_snapshot", "sanitize_go_and_parser_environment", "atomic_receipt_publish",
	})
}

func workCountValidateManifestAgreement(t *testing.T, sharedPath, convergencePath string) {
	t.Helper()
	shared, err := os.ReadFile(sharedPath)
	if err != nil {
		t.Fatal(err)
	}
	convergence, err := os.ReadFile(convergencePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := workCountManifestAgreementError(shared, convergence); err != nil {
		t.Fatal(err)
	}
}

func workCountManifestAgreementError(shared, convergence []byte) error {
	var sharedRaw, convergenceRaw map[string]json.RawMessage
	if err := json.Unmarshal(shared, &sharedRaw); err != nil {
		return fmt.Errorf("decode shared counter manifest: %w", err)
	}
	if err := json.Unmarshal(convergence, &convergenceRaw); err != nil {
		return fmt.Errorf("decode convergence manifest: %w", err)
	}
	for _, section := range []string{"direct", "terminal", "proxy", "derived", "attempt_attribution"} {
		sharedSection, ok := sharedRaw[section]
		if !ok {
			return fmt.Errorf("shared counter manifest lacks %s", section)
		}
		convergenceSection, ok := convergenceRaw[section]
		if !ok {
			return fmt.Errorf("convergence manifest lacks %s", section)
		}
		var sharedValue, convergenceValue any
		if err := json.Unmarshal(sharedSection, &sharedValue); err != nil {
			return fmt.Errorf("decode shared counter manifest %s: %w", section, err)
		}
		if err := json.Unmarshal(convergenceSection, &convergenceValue); err != nil {
			return fmt.Errorf("decode convergence manifest %s: %w", section, err)
		}
		if !reflect.DeepEqual(sharedValue, convergenceValue) {
			return fmt.Errorf("convergence manifest %s drifts from immutable shared counter manifest", section)
		}
	}
	return nil
}

func workCountValidateConvergenceManifest(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw := workCountDecodeRawObject(t, "convergence manifest", data)
	workCountRequireKeys(t, "convergence manifest", raw, []string{
		"schema", "counter_contract", "scope", "historical_compatibility", "direct", "terminal", "proxy", "derived",
		"attempt_attribution", "convergence_frontier", "admission",
	})
	var manifest struct {
		Schema          string            `json:"schema"`
		CounterContract string            `json:"counter_contract"`
		Scope           string            `json:"scope"`
		Historical      map[string]string `json:"historical_compatibility"`
		Direct          map[string]string `json:"direct"`
		Terminal        map[string]string `json:"terminal"`
		Proxy           map[string]string `json:"proxy"`
		Derived         map[string]string `json:"derived"`
		Attempt         struct {
			LogicalRungs                     []string `json:"logical_rungs"`
			ResolvedRetryPassIsIndependent   bool     `json:"resolved_retry_pass_is_independent"`
			Phases                           []string `json:"phases"`
			RequireAggregateEqualsAttributed bool     `json:"require_aggregate_equals_attempts_plus_outside"`
			CanonicalAttempts                int      `json:"canonical_query_compile_attempts"`
			CanonicalAcceptActions           uint64   `json:"canonical_query_compile_accept_actions"`
			RetryWitness                     struct {
				Path          string   `json:"path"`
				Bytes         int      `json:"bytes"`
				SHA256        string   `json:"sha256"`
				ExpectedRungs []string `json:"expected_rungs"`
			} `json:"retry_witness"`
			StraightLRControl struct {
				Path              string `json:"path"`
				Bytes             int    `json:"bytes"`
				SHA256            string `json:"sha256"`
				ExpectedMaxStacks int    `json:"expected_max_stacks"`
			} `json:"straight_lr_control"`
		} `json:"attempt_attribution"`
		Convergence struct {
			Status               string              `json:"status"`
			Capability           string              `json:"capability"`
			StaticCComparable    bool                `json:"static_c_emits_comparable_events"`
			MaxEvents            int                 `json:"max_events"`
			MaxPackedPaths       int                 `json:"max_packed_path_count"`
			ParseWorkBudget      int                 `json:"packed_path_parse_work_budget"`
			PerWalkWorkBudget    int                 `json:"packed_path_per_walk_work_budget"`
			DepthBudget          int                 `json:"packed_path_depth_budget"`
			RecordFirstReject    bool                `json:"record_first_reject_per_reason"`
			FirstRejectSemantics string              `json:"first_reject_semantics"`
			DecisionIDs          string              `json:"decision_ids"`
			HeadStatusPriority   []string            `json:"head_status_priority"`
			Phases               []string            `json:"phases"`
			PhaseOutcomes        map[string][]string `json:"phase_outcomes"`
			Outcomes             []string            `json:"outcomes"`
			Reasons              []string            `json:"reject_reasons"`
			Identity             []string            `json:"identity"`
			Snapshots            []string            `json:"snapshots"`
			EventInvariants      []string            `json:"event_invariants"`
			CandidateCountUnits  string              `json:"candidate_count_units"`
			BoundedTrace         string              `json:"bounded_trace"`
			IntegritySignals     []string            `json:"separate_integrity_signals"`
			SaturatingAggregates bool                `json:"include_saturating_aggregates"`
			TerminalCounters     []string            `json:"terminal_counters"`
			Interpretation       string              `json:"interpretation"`
		} `json:"convergence_frontier"`
		Admission map[string]any `json:"admission"`
	}
	workCountDecodeExact(t, data, &manifest)
	if manifest.Schema != "gts-work-count-contract/v3" || manifest.CounterContract != workCountContract || strings.TrimSpace(manifest.Scope) == "" {
		t.Fatalf("convergence manifest contract mismatch: schema=%q counters=%q scope=%q", manifest.Schema, manifest.CounterContract, manifest.Scope)
	}
	workCountRequireStringKeys(t, "convergence manifest history", manifest.Historical, []string{"v2_manifest", "direct_and_proxy_counter_semantics", "detailed_convergence"})
	workCountRequireStringKeys(t, "convergence manifest direct", manifest.Direct, workCountDirectFields)
	workCountRequireStringKeys(t, "convergence manifest terminal", manifest.Terminal, workCountTerminalFields)
	workCountRequireStringKeys(t, "convergence manifest proxy", manifest.Proxy, workCountProxyFields)
	workCountRequireStringKeys(t, "convergence manifest derived", manifest.Derived, []string{"construction_surplus"})
	workCountRequireKeys(t, "convergence manifest attempt attribution", workCountDecodeRawObject(t, "convergence manifest attempt attribution", raw["attempt_attribution"]), []string{
		"logical_rungs", "resolved_retry_pass_is_independent", "phases", "require_aggregate_equals_attempts_plus_outside",
		"canonical_query_compile_attempts", "canonical_query_compile_accept_actions", "retry_witness", "straight_lr_control",
	})
	if strings.Join(manifest.Attempt.LogicalRungs, ",") != "initial_full,initial_merge,clean_wide,clean_wide_merge,recovery_wide_or_node,secondary_node,final_merge" ||
		strings.Join(manifest.Attempt.Phases, ",") != "entry_to_caps,caps_to_finalize,finalize" || !manifest.Attempt.ResolvedRetryPassIsIndependent ||
		!manifest.Attempt.RequireAggregateEqualsAttributed || manifest.Attempt.CanonicalAttempts != 1 || manifest.Attempt.CanonicalAcceptActions != 3 {
		t.Fatalf("convergence manifest attempt invariants=%+v", manifest.Attempt)
	}
	workCountRequireKeys(t, "convergence manifest frontier", workCountDecodeRawObject(t, "convergence manifest frontier", raw["convergence_frontier"]), []string{
		"status", "capability", "static_c_emits_comparable_events", "max_events", "max_packed_path_count",
		"packed_path_parse_work_budget", "packed_path_per_walk_work_budget", "packed_path_depth_budget",
		"record_first_reject_per_reason", "first_reject_semantics", "decision_ids", "head_status_priority", "phases", "phase_outcomes", "outcomes", "reject_reasons", "identity", "snapshots", "event_invariants",
		"candidate_count_units", "bounded_trace", "separate_integrity_signals", "include_saturating_aggregates", "terminal_counters", "interpretation",
	})
	c := manifest.Convergence
	if c.Status != "implemented" || c.Capability != "go_only_detailed_convergence" || c.StaticCComparable || c.MaxEvents != 256 || c.MaxPackedPaths != 7 ||
		c.ParseWorkBudget != 16*1024 || c.PerWalkWorkBudget != 256 || c.DepthBudget != 128 || !c.RecordFirstReject || !c.SaturatingAggregates ||
		strings.TrimSpace(c.FirstRejectSemantics) == "" || strings.TrimSpace(c.DecisionIDs) == "" || !reflect.DeepEqual(c.HeadStatusPriority, []string{"dead", "accepted", "paused", "live"}) || len(c.EventInvariants) == 0 || strings.TrimSpace(c.CandidateCountUnits) == "" || strings.TrimSpace(c.BoundedTrace) == "" || strings.TrimSpace(c.Interpretation) == "" {
		t.Fatalf("convergence manifest frontier invariants=%+v", c)
	}
	if strings.Join(c.Phases, ",") != strings.Join(workCountConvergenceManifestPhases, ",") || strings.Join(c.Outcomes, ",") != strings.Join(workCountConvergenceManifestOutcomes, ",") || strings.Join(c.Reasons, ",") != strings.Join(workCountConvergenceManifestReasons, ",") {
		t.Fatalf("convergence manifest vocabulary phases=%v outcomes=%v reasons=%v", c.Phases, c.Outcomes, c.Reasons)
	}
	if !reflect.DeepEqual(c.PhaseOutcomes, workCountConvergencePhaseOutcomes) {
		t.Fatalf("convergence manifest phase/outcome matrix=%v want=%v", c.PhaseOutcomes, workCountConvergencePhaseOutcomes)
	}
	for _, required := range []string{"decision_id", "lookahead_present", "head_link_count_capped", "election_scanner_comparable", "election_scanner_checkpoint_presence_and_hash"} {
		if !workCountStringIn(required, c.Identity) {
			t.Fatalf("convergence manifest identity misses %q", required)
		}
	}
	if strings.Join(c.TerminalCounters, ",") != "accept_actions,accepted_heads,packed_roots_emitted" {
		t.Fatalf("convergence manifest terminal counters=%v", c.TerminalCounters)
	}
	workCountRequireKeys(t, "convergence manifest admission", workCountDecodeRawObject(t, "convergence manifest admission", raw["admission"]), []string{
		"digest_format", "fixture", "require_uninstrumented_go_static_c_digest_match_first", "require_instrumented_digest_match",
		"require_full_span", "require_clean_root", "require_no_overflow", "require_exact_fields", "require_ordinary_untagged_go_child_first",
		"require_clean_git_source_for_authoritative_receipt", "require_private_content_addressed_go_source_snapshot", "require_private_c_input_snapshot",
		"sanitize_go_and_parser_environment", "atomic_receipt_publish",
	})
	for key, value := range manifest.Admission {
		if (strings.HasPrefix(key, "require_") || key == "sanitize_go_and_parser_environment" || key == "atomic_receipt_publish") && value != true {
			t.Fatalf("convergence manifest admission %s=%v want true", key, value)
		}
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(path), "..", ".."))
	workCountValidateFrozenWitness(t, repoRoot, "retry", manifest.Attempt.RetryWitness.Path, manifest.Attempt.RetryWitness.Bytes, manifest.Attempt.RetryWitness.SHA256)
	workCountValidateFrozenWitness(t, repoRoot, "straight-LR", manifest.Attempt.StraightLRControl.Path, manifest.Attempt.StraightLRControl.Bytes, manifest.Attempt.StraightLRControl.SHA256)
}

func workCountValidateFrozenWitness(t *testing.T, repoRoot, label, relativePath string, wantBytes int, wantSHA string) {
	t.Helper()
	if relativePath == "" || filepath.IsAbs(relativePath) {
		t.Fatalf("manifest %s witness path=%q", label, relativePath)
	}
	path := filepath.Clean(filepath.Join(repoRoot, filepath.FromSlash(relativePath)))
	rel, err := filepath.Rel(repoRoot, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Fatalf("manifest %s witness escapes repository: path=%q rel=%q err=%v", label, path, rel, err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Size() != int64(wantBytes) {
		t.Fatalf("manifest %s witness mode/bytes=%s/%d want regular/%d", label, info.Mode(), info.Size(), wantBytes)
	}
	if got := workCountFileSHA(t, path); got != wantSHA {
		t.Fatalf("manifest %s witness sha256=%s want=%s", label, got, wantSHA)
	}
}

func workCountRequireKeys(t *testing.T, label string, values map[string]json.RawMessage, want []string) {
	t.Helper()
	got := make([]string, 0, len(values))
	for key := range values {
		got = append(got, key)
	}
	sort.Strings(got)
	want = append([]string(nil), want...)
	sort.Strings(want)
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("%s fields=%v want=%v", label, got, want)
	}
}

func workCountRequireKeysWithOptional(t *testing.T, label string, values map[string]json.RawMessage, required, optional []string) {
	t.Helper()
	allowed := make(map[string]bool, len(required)+len(optional))
	for _, key := range required {
		allowed[key] = true
		if _, ok := values[key]; !ok {
			t.Fatalf("%s missing required field %q", label, key)
		}
	}
	for _, key := range optional {
		allowed[key] = true
	}
	for key := range values {
		if !allowed[key] {
			t.Fatalf("%s unexpected field %q", label, key)
		}
	}
}

func workCountDecodeRawObject(t *testing.T, label string, data json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("decode %s: %v", label, err)
	}
	if raw == nil {
		t.Fatalf("%s is not an object", label)
	}
	return raw
}

func workCountStringIn(value string, values []string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func workCountRequireStringKeys(t *testing.T, label string, values map[string]string, want []string) {
	t.Helper()
	raw := make(map[string]json.RawMessage, len(values))
	for key, value := range values {
		if strings.TrimSpace(value) == "" {
			t.Fatalf("%s field %s has empty definition", label, key)
		}
		raw[key] = nil
	}
	workCountRequireKeys(t, label, raw, want)
}

func workCountTool(t *testing.T, name string) workCountToolIdentity {
	t.Helper()
	identity, err := workCountToolNamed(name)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func workCountToolNamed(name string) (workCountToolIdentity, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return workCountToolIdentity{}, err
	}
	return workCountToolAt(path)
}

func workCountToolAt(path string) (workCountToolIdentity, error) {
	resolved, err := filepath.Abs(path)
	if err != nil {
		return workCountToolIdentity{}, err
	}
	args := []string{"--version"}
	if filepath.Base(resolved) == "go" || filepath.Base(resolved) == "go.exe" {
		args = []string{"version"}
	}
	environment := workCountSanitizedEnv(os.Environ(), workCountCBuildEnvironment(), nil)
	out, err := workCountRunBounded("", environment, workCountGitTimeout, resolved, args...)
	if err != nil {
		return workCountToolIdentity{}, fmt.Errorf("read tool version %s: %w", resolved, err)
	}
	version := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if version == "" {
		return workCountToolIdentity{}, fmt.Errorf("tool %s returned an empty version", resolved)
	}
	sha, err := fileSHA256(resolved)
	if err != nil {
		return workCountToolIdentity{}, err
	}
	return workCountToolIdentity{Path: resolved, Version: version, SHA256: sha}, nil
}

func workCountFileSHA(t *testing.T, path string) string {
	t.Helper()
	sha, err := fileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha
}

func workCountCopyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
		if err != nil {
			_ = in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeInErr := in.Close()
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeInErr != nil {
			return closeInErr
		}
		return closeErr
	})
}
