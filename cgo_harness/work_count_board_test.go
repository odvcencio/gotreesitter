//go:build cgo && treesitter_c_parity && treesitter_c_perfscan

package cgoharness

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

const (
	workCountBoardEnableEnv               = "GTS_WORK_COUNT_BOARD"
	workCountBoardReceiptEnv              = "GTS_WORK_COUNT_BOARD_RECEIPT"
	workCountBoardGoBackendEnv            = "GTS_WORK_COUNT_GO_BACKEND"
	workCountBoardGoProduction            = "production"
	workCountBoardGoParserCore            = "parsercore_phase0"
	workCountBoardParserCoreChildSchema   = "gts-work-count-parsercore-child/v4"
	workCountBoardParserCoreEngine        = "go-compact-parsercore-phase0-tagged-diagnostic"
	workCountBoardSchema                  = "gts-work-count-board/v5"
	workCountBoardContractSchema          = "gts-work-count-board-contract/v3"
	workCountBoardContractSHA256          = "a06d4fd18a0c675bc2662a20d4cef98aa314cbf4485b6b5d14d671502c071cb4"
	workCountBoardInstrumentationRule     = "blocked unless every mandatory event has one identical semantic definition, a present direct counter in both engines, and a comparable relationship"
	workCountBoardWorkAuditRule           = "a mandatory comparable event with present nonzero-denominator counts outside the inclusive Go/C interval [0.8,1.2] produces a work-audit finding but does not change instrumentation completeness"
	workCountBoardWorkAuditMinimum        = 0.8
	workCountBoardWorkAuditMaximum        = 1.2
	workCountBoardInstrumentationBlocked  = "blocked"
	workCountBoardInstrumentationComplete = "complete"
	workCountBoardWorkAuditClear          = "clear"
	workCountBoardWorkAuditFindings       = "findings"
	workCountBoardStatePresent            = "present"
	workCountBoardStateUnavailable        = "unavailable"
	workCountBoardComparable              = "comparable"
	workCountBoardIncomparable            = "incomparable"
	workCountBoardRatioComputed           = "computed"
	workCountBoardRatioZero               = "zero_denominator"
	workCountBoardRatioUnavailable        = "unavailable"
	workCountBoardRatioIncomparable       = "incomparable"
)

var workCountBoardFixtureIDs = []string{"query_compile", "rewrite", "language", "grammargen_lr"}

type workCountBoardContract struct {
	Schema                string                         `json:"schema"`
	BoardSchema           string                         `json:"board_schema"`
	CounterContract       string                         `json:"counter_contract"`
	EngineStates          []string                       `json:"engine_states"`
	ComparisonStates      []string                       `json:"comparison_states"`
	RatioStates           []string                       `json:"ratio_states"`
	InstrumentationStates []string                       `json:"instrumentation_states"`
	WorkAuditStates       []string                       `json:"work_audit_states"`
	InstrumentationRule   string                         `json:"instrumentation_rule"`
	WorkAuditRule         string                         `json:"work_audit_rule"`
	Metrics               []workCountBoardMetricContract `json:"metrics"`
}

type workCountBoardMetricContract struct {
	ID                 string `json:"id"`
	Category           string `json:"category"`
	SemanticDefinition string `json:"semantic_definition"`
	GoUnit             string `json:"go_unit"`
	StaticCUnit        string `json:"static_c_unit"`
	Mandatory          bool   `json:"mandatory"`
}

// A pointer is intentional: present zero is serialized as value:0, while an
// unavailable hook has no value at all. Never use zero as an availability
// sentinel.
type workCountBoardEngineValue struct {
	State  string  `json:"state"`
	Unit   string  `json:"unit"`
	Value  *uint64 `json:"value,omitempty"`
	Reason string  `json:"reason,omitempty"`
}

type workCountBoardMetric struct {
	ID               string                    `json:"id"`
	Category         string                    `json:"category"`
	Mandatory        bool                      `json:"mandatory"`
	Go               workCountBoardEngineValue `json:"go"`
	StaticC          workCountBoardEngineValue `json:"static_c"`
	Comparison       string                    `json:"comparison"`
	ComparisonReason string                    `json:"comparison_reason,omitempty"`
	RatioStatus      string                    `json:"ratio_status"`
	GoOverC          *float64                  `json:"go_over_c,omitempty"`
}

type workCountBoardScheduling struct {
	MaxStacksSeen    int    `json:"max_stacks_seen"`
	MultiStackIters  int    `json:"multi_stack_iterations"`
	MultiStackTokens uint64 `json:"multi_stack_tokens"`
}

type workCountBoardProof struct {
	DiagnosticArtifactsTimingEligible bool     `json:"diagnostic_artifacts_timing_eligible"`
	PublicationTimingArtifact         string   `json:"publication_timing_artifact"`
	GoBackend                         string   `json:"go_backend"`
	UntaggedAssemblyTest              string   `json:"untagged_assembly_test"`
	UntaggedAssemblyPassed            bool     `json:"untagged_assembly_passed"`
	SchedulingFields                  []string `json:"scheduling_fields"`
}

type workCountBoardRow struct {
	Fixture               string                             `json:"fixture"`
	SourceSHA256          string                             `json:"source_sha256"`
	SourceBytes           uint32                             `json:"source_bytes"`
	DeepTreeSHA256        string                             `json:"deep_tree_sha256"`
	GrammarCommit         string                             `json:"grammar_commit"`
	GrammarBlobSHA256     string                             `json:"grammar_blob_sha256"`
	GoBackend             string                             `json:"go_backend"`
	UntaggedGoScheduling  workCountBoardScheduling           `json:"untagged_go_scheduling"`
	TaggedGoScheduling    *workCountBoardScheduling          `json:"tagged_go_scheduling,omitempty"`
	ParserCoreScheduler   *workCountBoardParserCoreScheduler `json:"parsercore_scheduler,omitempty"`
	RepeatIdenticalCounts bool                               `json:"repeat_identical_counts"`
	Metrics               []workCountBoardMetric             `json:"metrics"`
}

type workCountBoardParserCoreWork struct {
	Shifts                                 uint64 `json:"shifts"`
	Reductions                             uint64 `json:"reductions"`
	ReductionPopRequests                   uint64 `json:"reduction_pop_requests"`
	EmittedPopPaths                        uint64 `json:"emitted_pop_paths"`
	EmittedPopPayloads                     uint64 `json:"emitted_pop_payloads"`
	GraphLinkAdditionsProxy                uint64 `json:"graph_link_additions_proxy"`
	LeafConstructionsProxy                 uint64 `json:"leaf_constructions_proxy"`
	ParentConstructionsProxy               uint64 `json:"parent_constructions_proxy"`
	PredecessorLinkUnionAttempts           uint64 `json:"predecessor_link_union_attempts"`
	PredecessorLinkUnionDuplicateNoop      uint64 `json:"predecessor_link_union_duplicate_noop"`
	PredecessorLinkUnionPrecedenceReplaced uint64 `json:"predecessor_link_union_precedence_replaced"`
	PredecessorLinkUnionRecursiveChanged   uint64 `json:"predecessor_link_union_recursive_changed"`
	PredecessorLinkUnionAlternateAppended  uint64 `json:"predecessor_link_union_alternate_appended"`
	PredecessorLinkUnionRejected           uint64 `json:"predecessor_link_union_rejected"`
	Overflow                               bool   `json:"overflow"`
}

type workCountBoardParserCoreScheduler struct {
	ActionLookups              uint64 `json:"action_lookups"`
	Accepts                    uint64 `json:"accepts"`
	Elections                  uint64 `json:"elections"`
	Forks                      uint64 `json:"forks"`
	Conflicts                  uint64 `json:"conflicts"`
	ConflictActions            uint64 `json:"conflict_actions"`
	ConflictActionArmsAdmitted uint64 `json:"conflict_action_arms_admitted"`
	CausalConflictForks        uint64 `json:"causal_conflict_forks"`
	Canonicalizations          uint64 `json:"canonicalizations"`
	PeakHeaders                uint64 `json:"peak_headers"`
	Overflow                   bool   `json:"overflow"`
}

type workCountBoardSelectedCensus struct {
	Nodes   uint64 `json:"nodes"`
	Parents uint64 `json:"parents"`
	Leaves  uint64 `json:"leaves"`
}

type workCountBoardRawSelectedCensus struct {
	Nodes    uint64 `json:"nodes"`
	Parents  uint64 `json:"parents"`
	Leaves   uint64 `json:"leaves"`
	Overflow bool   `json:"overflow"`
}

type workCountFrozenCausalVector struct {
	ConflictActionArmsAdmitted             uint64
	CausalConflictForks                    uint64
	PredecessorLinkUnionAttempts           uint64
	PredecessorLinkUnionDuplicateNoop      uint64
	PredecessorLinkUnionPrecedenceReplaced uint64
	PredecessorLinkUnionRecursiveChanged   uint64
	PredecessorLinkUnionAlternateAppended  uint64
	PredecessorLinkUnionRejected           uint64
	RawSelectedInternalNodes               uint64
	RawSelectedInternalParentOccurrences   uint64
	RawSelectedInternalLeafOccurrences     uint64
}

var workCountFrozenStaticCCausalVectors = map[string]workCountFrozenCausalVector{
	"rewrite": {
		ConflictActionArmsAdmitted: 264, CausalConflictForks: 109,
		PredecessorLinkUnionAttempts: 177, PredecessorLinkUnionDuplicateNoop: 46,
		PredecessorLinkUnionPrecedenceReplaced: 1, PredecessorLinkUnionRecursiveChanged: 10,
		PredecessorLinkUnionAlternateAppended: 120,
		RawSelectedInternalNodes:              2345, RawSelectedInternalParentOccurrences: 1310, RawSelectedInternalLeafOccurrences: 1035,
	},
	"query_compile": {
		ConflictActionArmsAdmitted: 1287, CausalConflictForks: 521,
		PredecessorLinkUnionAttempts: 819, PredecessorLinkUnionDuplicateNoop: 172,
		PredecessorLinkUnionPrecedenceReplaced: 25, PredecessorLinkUnionRecursiveChanged: 48,
		PredecessorLinkUnionAlternateAppended: 574,
		RawSelectedInternalNodes:              11724, RawSelectedInternalParentOccurrences: 6599, RawSelectedInternalLeafOccurrences: 5125,
	},
	"language": {
		ConflictActionArmsAdmitted: 1358, CausalConflictForks: 486,
		PredecessorLinkUnionAttempts: 1143, PredecessorLinkUnionDuplicateNoop: 392,
		PredecessorLinkUnionPrecedenceReplaced: 17, PredecessorLinkUnionRecursiveChanged: 52,
		PredecessorLinkUnionAlternateAppended: 682,
		RawSelectedInternalNodes:              11184, RawSelectedInternalParentOccurrences: 6127, RawSelectedInternalLeafOccurrences: 5057,
	},
	"grammargen_lr": {
		ConflictActionArmsAdmitted: 13249, CausalConflictForks: 5421,
		PredecessorLinkUnionAttempts: 10300, PredecessorLinkUnionDuplicateNoop: 2914,
		PredecessorLinkUnionPrecedenceReplaced: 98, PredecessorLinkUnionRecursiveChanged: 694,
		PredecessorLinkUnionAlternateAppended: 6594,
		RawSelectedInternalNodes:              113511, RawSelectedInternalParentOccurrences: 63600, RawSelectedInternalLeafOccurrences: 49911,
	},
}

func workCountRequireFrozenStaticCCausalVector(t *testing.T, fixture string, direct workCountBoardDirectCounts) {
	t.Helper()
	want, ok := workCountFrozenStaticCCausalVectors[fixture]
	if !ok {
		t.Fatalf("static C causal vector fixture %q is not frozen", fixture)
	}
	got := workCountFrozenCausalVector{
		ConflictActionArmsAdmitted:             direct.ConflictActionArmsAdmitted,
		CausalConflictForks:                    direct.CausalConflictForks,
		PredecessorLinkUnionAttempts:           direct.PredecessorLinkUnionAttempts,
		PredecessorLinkUnionDuplicateNoop:      direct.PredecessorLinkUnionDuplicateNoop,
		PredecessorLinkUnionPrecedenceReplaced: direct.PredecessorLinkUnionPrecedenceReplaced,
		PredecessorLinkUnionRecursiveChanged:   direct.PredecessorLinkUnionRecursiveChanged,
		PredecessorLinkUnionAlternateAppended:  direct.PredecessorLinkUnionAlternateAppended,
		PredecessorLinkUnionRejected:           direct.PredecessorLinkUnionRejected,
		RawSelectedInternalNodes:               direct.RawSelectedInternalNodes,
		RawSelectedInternalParentOccurrences:   direct.RawSelectedInternalParents,
		RawSelectedInternalLeafOccurrences:     direct.RawSelectedInternalLeaves,
	}
	if got != want {
		t.Fatalf("static C causal vector drifted for %s: got=%+v want=%+v", fixture, got, want)
	}
}

type workCountBoardParserCoreChildResult struct {
	Schema             string                            `json:"schema"`
	Engine             string                            `json:"engine"`
	CounterContract    string                            `json:"counter_contract"`
	DigestFormat       string                            `json:"digest_format"`
	Fixture            string                            `json:"fixture"`
	SourceSHA256       string                            `json:"source_sha256"`
	SourceBytes        uint32                            `json:"source_bytes"`
	GrammarCommit      string                            `json:"grammar_commit"`
	GrammarBlobSHA256  string                            `json:"grammar_blob_sha256"`
	DeepTreeSHA256     string                            `json:"deep_tree_sha256"`
	RootEndByte        uint32                            `json:"root_end_byte"`
	RootHasError       bool                              `json:"root_has_error"`
	CandidateFallbacks uint64                            `json:"candidate_fallbacks"`
	BoardDirect        workCountBoardDirectCounts        `json:"board_direct"`
	CoreWork           workCountBoardParserCoreWork      `json:"core_work"`
	SchedulerWork      workCountBoardParserCoreScheduler `json:"scheduler_work"`
	Selected           workCountBoardSelectedCensus      `json:"selected_census"`
	RawSelected        workCountBoardRawSelectedCensus   `json:"raw_selected_internal_census"`
}

type workCountBoard struct {
	Schema                      string                    `json:"schema"`
	InstrumentationStatus       string                    `json:"instrumentation_status"`
	InstrumentationBlockers     []string                  `json:"instrumentation_blockers,omitempty"`
	WorkAuditStatus             string                    `json:"work_audit_status"`
	WorkAuditFindings           []string                  `json:"work_audit_findings,omitempty"`
	CounterContract             string                    `json:"counter_contract"`
	SharedCounterManifestSHA256 string                    `json:"shared_counter_manifest_sha256"`
	BoardContract               string                    `json:"board_contract"`
	BoardContractSHA256         string                    `json:"board_contract_sha256"`
	DigestFormat                string                    `json:"digest_format"`
	Source                      workCountGitProvenance    `json:"source"`
	GoEnvironment               workCountEnvironment      `json:"go_environment"`
	GoAdmission                 workCountArtifactIdentity `json:"go_admission_artifact"`
	GoArtifact                  workCountArtifactIdentity `json:"go_work_count_artifact"`
	CArtifact                   workCountCIdentity        `json:"static_c_artifact"`
	Proof                       workCountBoardProof       `json:"instrumentation_proof"`
	Rows                        []workCountBoardRow       `json:"rows"`
}

// TestAuthenticatedFourFixtureWorkCountBoard builds each diagnostic artifact
// once, then runs the complete frozen real-Go suite through the selected Go
// backend and the locked static -O2 C oracle. It is separate from the
// publication receipt: this diagnostic board compares work counts and never
// times either implementation.
func TestAuthenticatedFourFixtureWorkCountBoard(t *testing.T) {
	if os.Getenv(workCountBoardEnableEnv) != "1" {
		t.Skip(workCountBoardEnableEnv + "=1 is required")
	}
	goBackend := workCountBoardBackend(t)
	repoRoot := workCountRepoRoot(t)
	receiptPath := workCountPrepareBoardReceiptPath(t)
	tempRoot := t.TempDir()
	sourceSnapshot := workCountPrepareSourceSnapshot(t, repoRoot, tempRoot)
	workCountClearAmbientGitRouting(t)

	sharedManifestPath := filepath.Join(sourceSnapshot.Root, "cgo_harness", "work_count", "contract_v2.json")
	sharedManifestSHA := workCountFileSHA(t, sharedManifestPath)
	if sharedManifestSHA != workCountSharedManifestSHA256 {
		t.Fatalf("work-count shared manifest identity=%s want=%s", sharedManifestSHA, workCountSharedManifestSHA256)
	}
	workCountValidateManifest(t, sharedManifestPath)
	boardContractPath := filepath.Join(sourceSnapshot.Root, "cgo_harness", "work_count", "board_contract_v3.json")
	boardContractSHA := workCountFileSHA(t, boardContractPath)
	if boardContractSHA != workCountBoardContractSHA256 {
		t.Fatalf("work-count board contract identity=%s want=%s", boardContractSHA, workCountBoardContractSHA256)
	}
	boardContract := workCountLoadBoardContract(t, boardContractPath)
	patchPath := filepath.Join(sourceSnapshot.Root, "cgo_harness", "work_count", "tree_sitter_v0_25_1.patch")
	driverPath := filepath.Join(sourceSnapshot.Root, "cgo_harness", "pure_c", "work_count_oracle.c")
	if got := workCountFileSHA(t, patchPath); got == "" {
		t.Fatal("work-count C patch identity is empty")
	}
	if got := workCountFileSHA(t, driverPath); got == "" {
		t.Fatal("work-count C driver identity is empty")
	}

	fixtures, err := benchfixtures.LoadGoFullParseFixtures()
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != len(workCountBoardFixtureIDs) {
		t.Fatalf("canonical fixture count=%d want=%d", len(fixtures), len(workCountBoardFixtureIDs))
	}
	for index, id := range workCountBoardFixtureIDs {
		if fixtures[index].Fixture.ID != id {
			t.Fatalf("canonical fixture %d=%q want=%q", index, fixtures[index].Fixture.ID, id)
		}
	}

	goEnvironment := workCountChosenEnvironment()
	t.Log("build: ordinary untagged Go admission child")
	goAdmissionArtifact, goAdmissionIdentity, goAdmissionRecheck := workCountBuildGo(t, sourceSnapshot, tempRoot, goEnvironment, false)
	var goArtifact string
	var goIdentity workCountArtifactIdentity
	var goRecheck func() error
	switch goBackend {
	case workCountBoardGoProduction:
		t.Log("build: tagged production diagnostic Go child")
		goArtifact, goIdentity, goRecheck = workCountBuildGo(t, sourceSnapshot, tempRoot, goEnvironment, true)
	case workCountBoardGoParserCore:
		t.Log("build: tagged compact parser-core diagnostic Go child")
		goArtifact, goIdentity, goRecheck = workCountBuildParserCoreGo(t, sourceSnapshot, tempRoot, goEnvironment)
	default:
		t.Fatalf("unsupported Go work-count backend %q", goBackend)
	}
	t.Log("build: instrumented locked static C child")
	cBuild := workCountBuildC(t, repoRoot, sourceSnapshot.Root, patchPath, driverPath)
	defer cBuild.Cleanup()

	board := workCountBoard{
		Schema: workCountBoardSchema, CounterContract: workCountContract,
		SharedCounterManifestSHA256: sharedManifestSHA,
		BoardContract:               boardContract.Schema,
		BoardContractSHA256:         boardContractSHA,
		DigestFormat:                benchfixtures.DeepTreeDigestVersion,
		Source:                      sourceSnapshot.Provenance, GoEnvironment: goEnvironment,
		GoAdmission: goAdmissionIdentity, GoArtifact: goIdentity, CArtifact: cBuild.Identity,
		Proof: workCountBoardProof{
			DiagnosticArtifactsTimingEligible: false,
			PublicationTimingArtifact:         "ordinary untagged Go plus locked uninstrumented static C only",
			GoBackend:                         goBackend,
			UntaggedAssemblyTest:              "TestWorkCountProductionAssemblyHasNoDiagnosticScaffolding",
			SchedulingFields:                  []string{"production: max_stacks_seen/multi_stack_iterations/multi_stack_tokens", "parsercore_phase0: action_lookups/accepts/elections/forks/conflicts/conflict_actions/canonicalizations/peak_headers"},
		},
		Rows: make([]workCountBoardRow, 0, len(fixtures)),
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.Fixture.ID, func(t *testing.T) {
			fixtureTemp := filepath.Join(tempRoot, "fixtures", fixture.Fixture.ID)
			if err := os.MkdirAll(fixtureTemp, 0o755); err != nil {
				t.Fatal(err)
			}
			sourcePath := filepath.Join(fixtureTemp, fixture.Fixture.ID+".go")
			if err := os.WriteFile(sourcePath, fixture.Source, 0o600); err != nil {
				t.Fatal(err)
			}
			if got := workCountFileSHA(t, sourcePath); got != fixture.Fixture.SHA256 {
				t.Fatalf("source snapshot sha=%s want=%s", got, fixture.Fixture.SHA256)
			}

			uninstGo := workCountRunGoAdmission(t, sourceSnapshot.Root, goAdmissionArtifact, fixture.Fixture.ID, sourcePath, fixtureTemp, goEnvironment)
			workCountValidateGoChild(t, "ordinary Go admission", workCountGoAdmissionChildSchema, "go-production-glr-untagged", uninstGo, fixture)
			uninstC := workCountUninstrumentedCAdmission(t, fixture, sourcePath, fixtureTemp)
			if uninstGo.DeepTreeSHA256 != uninstC || uninstC != fixture.Fixture.DeepTreeSHA256 {
				t.Fatalf("uninstrumented admission mismatch: go=%s static_c=%s frozen=%s", uninstGo.DeepTreeSHA256, uninstC, fixture.Fixture.DeepTreeSHA256)
			}

			cResult := workCountRunC(t, cBuild.Artifact, sourcePath, fixtureTemp)
			workCountValidateCChild(t, "static C", "static-c-instrumented-glr", cResult, fixture, fixture.Fixture.DeepTreeSHA256)
			workCountRequireFrozenStaticCCausalVector(t, fixture.Fixture.ID, cResult.BoardDirect)
			untaggedScheduling := workCountScheduling(uninstGo)
			switch goBackend {
			case workCountBoardGoProduction:
				goResult := workCountRunGo(t, sourceSnapshot.Root, goArtifact, fixture.Fixture.ID, sourcePath, fixtureTemp, goEnvironment)
				workCountValidateGoChild(t, "tagged Go", workCountTaggedGoChildSchema, "go-production-glr-tagged-diagnostic", goResult.workCountGoChildResult, fixture)
				workCountValidateGoBoardCounters(t, goResult.Counters, uint32(len(fixture.Source)))
				if cResult.DeepTreeSHA256 != goResult.DeepTreeSHA256 {
					t.Fatalf("instrumented deep digest mismatch: go=%s static_c=%s", goResult.DeepTreeSHA256, cResult.DeepTreeSHA256)
				}
				taggedScheduling := workCountScheduling(goResult.workCountGoChildResult)
				if untaggedScheduling != taggedScheduling {
					t.Fatalf("diagnostic scheduling changed: untagged=%+v tagged=%+v", untaggedScheduling, taggedScheduling)
				}
				metrics := workCountBuildBoardMetrics(t, boardContract, goResult.Counters.workCountCounters, cResult.Counters, goResult.BoardDirect, cResult.BoardDirect)
				board.Rows = append(board.Rows, workCountBoardRow{
					Fixture: fixture.Fixture.ID, SourceSHA256: fixture.Fixture.SHA256,
					SourceBytes: uint32(len(fixture.Source)), DeepTreeSHA256: fixture.Fixture.DeepTreeSHA256,
					GrammarCommit: goResult.GrammarCommit, GrammarBlobSHA256: goResult.GrammarBlobSHA256,
					GoBackend: goBackend, UntaggedGoScheduling: untaggedScheduling, TaggedGoScheduling: &taggedScheduling,
					Metrics: metrics,
				})
			case workCountBoardGoParserCore:
				first := workCountRunParserCoreGo(t, sourceSnapshot.Root, goArtifact, fixture.Fixture.ID, sourcePath, fixtureTemp, "first", goEnvironment)
				second := workCountRunParserCoreGo(t, sourceSnapshot.Root, goArtifact, fixture.Fixture.ID, sourcePath, fixtureTemp, "repeat", goEnvironment)
				workCountValidateParserCoreChild(t, first, fixture)
				workCountValidateParserCoreChild(t, second, fixture)
				if !reflect.DeepEqual(first, second) {
					t.Fatalf("parser-core work counts are not repeat-identical: first=%+v repeat=%+v", first, second)
				}
				if cResult.DeepTreeSHA256 != first.DeepTreeSHA256 {
					t.Fatalf("instrumented deep digest mismatch: parsercore=%s static_c=%s", first.DeepTreeSHA256, cResult.DeepTreeSHA256)
				}
				metrics := workCountBuildParserCoreBoardMetrics(t, boardContract, first, cResult.Counters, cResult.BoardDirect)
				scheduler := first.SchedulerWork
				board.Rows = append(board.Rows, workCountBoardRow{
					Fixture: fixture.Fixture.ID, SourceSHA256: fixture.Fixture.SHA256,
					SourceBytes: uint32(len(fixture.Source)), DeepTreeSHA256: fixture.Fixture.DeepTreeSHA256,
					GrammarCommit: first.GrammarCommit, GrammarBlobSHA256: first.GrammarBlobSHA256,
					GoBackend: goBackend, UntaggedGoScheduling: untaggedScheduling,
					ParserCoreScheduler: &scheduler, RepeatIdenticalCounts: true, Metrics: metrics,
				})
			}
			if got := workCountFileSHA(t, sourcePath); got != fixture.Fixture.SHA256 {
				t.Fatalf("source snapshot drift after diagnostic runs: sha=%s want=%s", got, fixture.Fixture.SHA256)
			}
		})
	}
	if len(board.Rows) != len(fixtures) {
		t.Fatalf("board rows=%d want=%d", len(board.Rows), len(fixtures))
	}
	board.Proof.UntaggedAssemblyPassed = workCountRunUntaggedAssemblyProof(t, sourceSnapshot.Root, goAdmissionArtifact, goEnvironment)
	board.InstrumentationStatus, board.InstrumentationBlockers, board.WorkAuditStatus, board.WorkAuditFindings = workCountBoardStatuses(boardContract, board.Rows)
	workCountValidateBoard(t, boardContract, board)
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

	data, err := json.MarshalIndent(board, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if receiptPath != "" {
		if err := workCountAtomicPublish(receiptPath, data); err != nil {
			t.Fatal(err)
		}
		if sourceSnapshot.Provenance.Authoritative {
			t.Logf("authenticated four-fixture work-count board: %s", receiptPath)
		} else {
			t.Logf("non-authoritative dirty-source four-fixture work-count board: %s", receiptPath)
		}
		return
	}
	t.Logf("four-fixture work-count board (set %s to publish):\n%s", workCountBoardReceiptEnv, data)
}

func workCountValidateGoBoardCounters(t *testing.T, counts workCountGoCounters, sourceBytes uint32) {
	t.Helper()
	workCountValidateCounters(t, "tagged Go", counts.workCountCounters)
	if len(counts.Attempts) != 1 {
		t.Fatalf("tagged Go canonical parse attempts=%d want=1", len(counts.Attempts))
	}
	for index, attempt := range counts.Attempts {
		if attempt.Index != uint32(index+1) || !attempt.CapsResolved {
			t.Fatalf("tagged Go attempt %d identity/caps=%d/%v", index, attempt.Index, attempt.CapsResolved)
		}
		if attempt.StopReason == "" {
			t.Fatalf("tagged Go attempt %d has no stop reason", index)
		}
		workCountRequireValueSum(t, fmt.Sprintf("tagged Go attempt %d phases", index+1), attempt.Counters, attempt.EntryToCaps, attempt.CapsToFinalize, attempt.Finalize)
	}
	finalAttempt := counts.Attempts[len(counts.Attempts)-1]
	if finalAttempt.Index != 1 || finalAttempt.LogicalRung != "initial_full" || finalAttempt.OperationCause != "fresh_dfa_full_parse" {
		t.Fatalf("tagged Go attempt identity=%d/%q/%q want=1/initial_full/fresh_dfa_full_parse", finalAttempt.Index, finalAttempt.LogicalRung, finalAttempt.OperationCause)
	}
	if finalAttempt.StopReason != "accepted" || finalAttempt.RootHasError || finalAttempt.RootEndByte != sourceBytes {
		t.Fatalf("tagged Go final attempt stop=%q error=%v end=%d want=%d", finalAttempt.StopReason, finalAttempt.RootHasError, finalAttempt.RootEndByte, sourceBytes)
	}
	parts := make([]workCountCounterValues, 0, len(counts.Attempts)+1)
	for _, attempt := range counts.Attempts {
		parts = append(parts, attempt.Counters)
	}
	parts = append(parts, counts.OutsideAttempt)
	workCountRequireValueSum(t, "tagged Go aggregate attribution", counts.workCountCounterValues, parts...)
}

func workCountLoadBoardContract(t *testing.T, path string) workCountBoardContract {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var contract workCountBoardContract
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatalf("decode work-count board contract: %v", err)
	}
	if contract.Schema != workCountBoardContractSchema || contract.BoardSchema != workCountBoardSchema || contract.CounterContract != workCountContract {
		t.Fatalf("board contract identity=%q/%q/%q", contract.Schema, contract.BoardSchema, contract.CounterContract)
	}
	if strings.Join(contract.EngineStates, ",") != workCountBoardStatePresent+","+workCountBoardStateUnavailable ||
		strings.Join(contract.ComparisonStates, ",") != workCountBoardComparable+","+workCountBoardIncomparable ||
		strings.Join(contract.RatioStates, ",") != strings.Join([]string{workCountBoardRatioComputed, workCountBoardRatioZero, workCountBoardRatioIncomparable, workCountBoardRatioUnavailable}, ",") ||
		strings.Join(contract.InstrumentationStates, ",") != workCountBoardInstrumentationComplete+","+workCountBoardInstrumentationBlocked ||
		strings.Join(contract.WorkAuditStates, ",") != workCountBoardWorkAuditClear+","+workCountBoardWorkAuditFindings ||
		contract.InstrumentationRule != workCountBoardInstrumentationRule ||
		contract.WorkAuditRule != workCountBoardWorkAuditRule {
		t.Fatalf("board contract status vocabularies drifted: %+v", contract)
	}
	seen := make(map[string]bool, len(contract.Metrics))
	for _, metric := range contract.Metrics {
		if strings.TrimSpace(metric.ID) == "" || strings.TrimSpace(metric.Category) == "" || strings.TrimSpace(metric.SemanticDefinition) == "" || strings.TrimSpace(metric.GoUnit) == "" || strings.TrimSpace(metric.StaticCUnit) == "" {
			t.Fatalf("incomplete board metric contract: %+v", metric)
		}
		if seen[metric.ID] {
			t.Fatalf("duplicate board metric contract %q", metric.ID)
		}
		seen[metric.ID] = true
	}
	return contract
}

func workCountBoardBackend(t *testing.T) string {
	t.Helper()
	backend := strings.TrimSpace(os.Getenv(workCountBoardGoBackendEnv))
	if backend == "" {
		return workCountBoardGoProduction
	}
	if backend != workCountBoardGoProduction && backend != workCountBoardGoParserCore {
		t.Fatalf("%s=%q, want %q or %q", workCountBoardGoBackendEnv, backend, workCountBoardGoProduction, workCountBoardGoParserCore)
	}
	return backend
}

func workCountBuildParserCoreGo(t *testing.T, source workCountSourceSnapshot, tempRoot string, environment workCountEnvironment) (string, workCountArtifactIdentity, func() error) {
	t.Helper()
	tool := workCountTool(t, "go")
	artifact := filepath.Join(tempRoot, "go_parsercore_work_count.test")
	flags := []string{"test", "-c", "-tags", "gts_parsercorephase0,gts_workcount", "."}
	args := []string{"test", "-c", "-tags", "gts_parsercorephase0,gts_workcount", "-o", artifact, "."}
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
			return fmt.Errorf("parser-core artifact sha=%s want=%s err=%v", gotSHA, sha, err)
		}
		gotTool, err := workCountToolAt(tool.Path)
		if err != nil || gotTool != tool {
			return fmt.Errorf("Go tool identity=%+v want=%+v err=%v", gotTool, tool, err)
		}
		return source.Recheck()
	}
}

func workCountRunParserCoreGo(t *testing.T, sourceRoot, artifact, fixtureID, sourcePath, tempRoot, pass string, environment workCountEnvironment) workCountBoardParserCoreChildResult {
	t.Helper()
	resultPath := filepath.Join(tempRoot, "go-parsercore-work-count-"+pass+".json")
	workCountRunGoChild(t, sourceRoot, artifact, "^TestParserCoreWorkCountChild$", fixtureID, sourcePath, resultPath, environment)
	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("decode parser-core child object: %v: %s", err, data)
	}
	workCountRequireKeys(t, "parser-core child", raw, []string{
		"schema", "engine", "counter_contract", "digest_format", "fixture", "source_sha256", "source_bytes",
		"grammar_commit", "grammar_blob_sha256", "deep_tree_sha256", "root_end_byte", "root_has_error",
		"candidate_fallbacks", "board_direct", "core_work", "scheduler_work", "selected_census", "raw_selected_internal_census",
	})
	workCountValidateBoardDirectObject(t, raw["board_direct"])
	var result workCountBoardParserCoreChildResult
	workCountDecodeExact(t, data, &result)
	return result
}

func workCountValidateParserCoreChild(t *testing.T, result workCountBoardParserCoreChildResult, fixture benchfixtures.LoadedFixture) {
	t.Helper()
	if result.Schema != workCountBoardParserCoreChildSchema || result.Engine != workCountBoardParserCoreEngine || result.CounterContract != workCountContract || result.DigestFormat != benchfixtures.DeepTreeDigestVersion {
		t.Fatalf("parser-core child identity=%q/%q/%q/%q", result.Schema, result.Engine, result.CounterContract, result.DigestFormat)
	}
	if result.Fixture != fixture.Fixture.ID || result.SourceSHA256 != fixture.Fixture.SHA256 || result.SourceBytes != uint32(len(fixture.Source)) || result.RootEndByte != uint32(len(fixture.Source)) || result.RootHasError {
		t.Fatalf("parser-core child fixture/span drift: %+v", result)
	}
	if result.GrammarCommit != benchfixtures.GoGrammarCommit || result.GrammarBlobSHA256 != benchfixtures.GoGrammarBlobSHA256 || result.DeepTreeSHA256 != fixture.Fixture.DeepTreeSHA256 {
		t.Fatalf("parser-core child grammar/tree identity drift: grammar=%s/%s tree=%s", result.GrammarCommit, result.GrammarBlobSHA256, result.DeepTreeSHA256)
	}
	if result.CandidateFallbacks != 0 || result.CoreWork.Reductions != result.CoreWork.ReductionPopRequests {
		t.Fatalf("parser-core child fallback/overflow/reduce contract: %+v", result)
	}
	if err := workCountValidateParserCoreCounterEnvelope(result); err != nil {
		t.Fatalf("parser-core child counter envelope: %v", err)
	}
	if result.Selected.Nodes == 0 || result.Selected.Nodes != result.Selected.Parents+result.Selected.Leaves {
		t.Fatalf("parser-core child selected census invalid: %+v", result.Selected)
	}
	if result.RawSelected.Overflow || result.RawSelected.Nodes == 0 || result.RawSelected.Nodes != result.RawSelected.Parents+result.RawSelected.Leaves {
		t.Fatalf("parser-core child raw selected census invalid: %+v", result.RawSelected)
	}
	unionOutcomes := result.CoreWork.PredecessorLinkUnionDuplicateNoop +
		result.CoreWork.PredecessorLinkUnionPrecedenceReplaced +
		result.CoreWork.PredecessorLinkUnionRecursiveChanged +
		result.CoreWork.PredecessorLinkUnionAlternateAppended +
		result.CoreWork.PredecessorLinkUnionRejected
	if unionOutcomes != result.CoreWork.PredecessorLinkUnionAttempts {
		t.Fatalf("parser-core child union partition=%d attempts=%d", unionOutcomes, result.CoreWork.PredecessorLinkUnionAttempts)
	}
	if result.CoreWork.LeafConstructionsProxy < result.RawSelected.Leaves || result.CoreWork.ParentConstructionsProxy < result.RawSelected.Parents {
		t.Fatalf("parser-core raw construction surplus underflow: core=%+v raw=%+v", result.CoreWork, result.RawSelected)
	}
	if result.SchedulerWork.ActionLookups == 0 || result.SchedulerWork.Accepts != 1 || result.SchedulerWork.Elections == 0 || result.SchedulerWork.PeakHeaders < 2 || result.SchedulerWork.Conflicts == 0 || result.SchedulerWork.Forks == 0 {
		t.Fatalf("parser-core child scheduler work invalid: %+v", result.SchedulerWork)
	}
	if !result.BoardDirect.FrontierLexerElectionsAvailable || result.BoardDirect.PerVersionLexRequestsAvailable || result.BoardDirect.FrontierLexerElections != result.SchedulerWork.Elections || result.BoardDirect.ResolvedActionCellsExamined != result.SchedulerWork.ActionLookups || result.BoardDirect.RawMainLexerInvocations == 0 {
		t.Fatalf("parser-core child direct hooks drifted from scheduler work: direct=%+v scheduler=%+v", result.BoardDirect, result.SchedulerWork)
	}
}

func workCountValidateParserCoreCounterEnvelope(result workCountBoardParserCoreChildResult) error {
	if result.CoreWork.Overflow || result.SchedulerWork.Overflow || result.RawSelected.Overflow {
		return fmt.Errorf("counter overflow: core=%t scheduler=%t raw=%t", result.CoreWork.Overflow, result.SchedulerWork.Overflow, result.RawSelected.Overflow)
	}
	if result.CoreWork.LeafConstructionsProxy < result.RawSelected.Leaves || result.CoreWork.ParentConstructionsProxy < result.RawSelected.Parents {
		return fmt.Errorf("construction surplus underflow: core=%+v raw=%+v", result.CoreWork, result.RawSelected)
	}
	return nil
}

func workCountPresent(unit string, value uint64) workCountBoardEngineValue {
	return workCountBoardEngineValue{State: workCountBoardStatePresent, Unit: unit, Value: &value}
}

func workCountUnavailable(unit, reason string) workCountBoardEngineValue {
	return workCountBoardEngineValue{State: workCountBoardStateUnavailable, Unit: unit, Reason: reason}
}

func workCountBoardMetricValue(contract workCountBoardMetricContract, goValue, cValue workCountBoardEngineValue, comparison, reason string) workCountBoardMetric {
	metric := workCountBoardMetric{
		ID: contract.ID, Category: contract.Category, Mandatory: contract.Mandatory,
		Go: goValue, StaticC: cValue, Comparison: comparison, ComparisonReason: reason,
	}
	switch {
	case goValue.State == workCountBoardStateUnavailable || cValue.State == workCountBoardStateUnavailable:
		metric.RatioStatus = workCountBoardRatioUnavailable
	case comparison == workCountBoardIncomparable:
		metric.RatioStatus = workCountBoardRatioIncomparable
	case cValue.Value == nil || *cValue.Value == 0:
		metric.RatioStatus = workCountBoardRatioZero
	default:
		ratio := float64(*goValue.Value) / float64(*cValue.Value)
		metric.RatioStatus = workCountBoardRatioComputed
		metric.GoOverC = &ratio
	}
	return metric
}

func workCountBuildBoardMetrics(t *testing.T, contract workCountBoardContract, goCounts, cCounts workCountCounters, goDirect, cDirect workCountBoardDirectCounts) []workCountBoardMetric {
	t.Helper()
	workCountValidateCounters(t, "Go", goCounts)
	workCountValidateCounters(t, "static C", cCounts)
	if goDirect.Schema != "gts-work-count-board-direct/v3" || cDirect.Schema != goDirect.Schema || goDirect.Overflow || cDirect.Overflow || !goDirect.FrontierLexerElectionsAvailable || goDirect.PerVersionLexRequestsAvailable || cDirect.FrontierLexerElectionsAvailable || !cDirect.PerVersionLexRequestsAvailable {
		t.Fatalf("invalid paired board-direct counters: go=%+v C=%+v", goDirect, cDirect)
	}
	metrics := make([]workCountBoardMetric, 0, len(contract.Metrics))
	for _, definition := range contract.Metrics {
		present := func(goValue, cValue uint64, comparison, reason string) {
			metrics = append(metrics, workCountBoardMetricValue(definition, workCountPresent(definition.GoUnit, goValue), workCountPresent(definition.StaticCUnit, cValue), comparison, reason))
		}
		missing := func(goValue, cValue workCountBoardEngineValue, comparison, reason string) {
			metrics = append(metrics, workCountBoardMetricValue(definition, goValue, cValue, comparison, reason))
		}
		switch definition.ID {
		case "lexer_elections":
			missing(workCountPresent(definition.GoUnit, goDirect.FrontierLexerElections), workCountUnavailable(definition.StaticCUnit, "locked C exposes per-version ts_parser__lex cache misses, not a runnable union-frontier election"), workCountBoardIncomparable, "C per-version lex request cannot satisfy the Go union-frontier contract")
		case "lexer_calls":
			present(goDirect.RawMainLexerInvocations, cDirect.RawMainLexerInvocations, workCountBoardComparable, "direct main-DFA callable-entry seam; comparability is scoped to the clean locked quartet")
		case "resolved_action_cells_examined":
			present(goDirect.ResolvedActionCellsExamined, cDirect.ResolvedActionCellsExamined, workCountBoardComparable, "direct dispatch seam; comparability is scoped to the clean locked quartet")
		case "action_table_lookups_proxy":
			present(goCounts.TableLookupsProxy, cCounts.TableLookupsProxy, workCountBoardIncomparable, "implementation-specific lookup front doors")
		case "action_entries_examined_proxy":
			present(goCounts.ActionEntriesExaminedProxy, cCounts.ActionEntriesExaminedProxy, workCountBoardIncomparable, "implementation-specific dispatch entry exposure")
		case "shifts":
			present(goCounts.Shifts, cCounts.Shifts, workCountBoardComparable, "")
		case "reductions":
			present(goCounts.Reductions, cCounts.Reductions, workCountBoardComparable, "")
		case "explicit_recover_actions":
			present(goCounts.ExplicitRecoverActions, cCounts.ExplicitRecoverActions, workCountBoardComparable, "")
		case "accept_actions":
			present(goCounts.AcceptActions, cCounts.AcceptActions, workCountBoardComparable, "")
		case "conflict_action_arms_admitted", "causal_conflict_forks":
			missing(workCountUnavailable(definition.GoUnit, "production-Go causal admission seam is not yet complete across all guarded conflict routes"), workCountPresent(definition.StaticCUnit, map[string]uint64{"conflict_action_arms_admitted": cDirect.ConflictActionArmsAdmitted, "causal_conflict_forks": cDirect.CausalConflictForks}[definition.ID]), workCountBoardComparable, "production-Go causal conflict hook incomplete")
		case "raw_action_entries_beyond_first":
			present(goDirect.RawActionEntriesBeyondFirst, cDirect.RawActionEntriesBeyondFirst, workCountBoardComparable, "")
		case "stack_version_creations_proxy":
			present(goCounts.StackVersionCreationsProxy, cCounts.StackVersionCreationsProxy, workCountBoardIncomparable, "initialization and implementation-specific copies are included")
		case "reduction_pop_requests":
			present(goCounts.ReductionPopRequests, cCounts.ReductionPopRequests, workCountBoardComparable, "")
		case "emitted_pop_paths":
			present(goCounts.EmittedPopPaths, cCounts.EmittedPopPaths, workCountBoardComparable, "")
		case "emitted_pop_payloads":
			present(goCounts.EmittedPopPayloads, cCounts.EmittedPopPayloads, workCountBoardComparable, "")
		case "graph_link_additions_proxy":
			present(goCounts.GraphLinkAdditionsProxy, cCounts.GraphLinkAdditionsProxy, workCountBoardIncomparable, "representation-specific physical graph links")
		case "subtree_leaf_constructions_proxy":
			present(goCounts.LeafConstructionsProxy, cCounts.LeafConstructionsProxy, workCountBoardIncomparable, "representation-specific transient payloads")
		case "subtree_parent_constructions_proxy":
			present(goCounts.ParentConstructionsProxy, cCounts.ParentConstructionsProxy, workCountBoardIncomparable, "representation-specific transient payloads")
		case "merge_attempts_proxy":
			present(goCounts.MergeAttemptsProxy, cCounts.MergeAttemptsProxy, workCountBoardIncomparable, "implementation-specific merge front doors")
		case "merge_successes_proxy":
			present(goCounts.MergeSuccessesProxy, cCounts.MergeSuccessesProxy, workCountBoardIncomparable, "implementation-specific merge success boundaries")
		case "predecessor_link_union_attempts", "predecessor_link_union_duplicate_noop", "predecessor_link_union_precedence_replaced", "predecessor_link_union_recursive_changed", "predecessor_link_union_alternate_appended", "predecessor_link_union_rejected", "predecessor_link_union_material_changes":
			missing(workCountUnavailable(definition.GoUnit, "production-Go logical union outcome seam is not complete across every GSS representation"), workCountUnavailable(definition.StaticCUnit, "direct C value exists but no paired production-Go contract"), workCountBoardComparable, "production-Go logical union hook incomplete")
		case "alternate_predecessor_links_appended":
			present(goDirect.AlternatePredecessorLinksAppended, cDirect.AlternatePredecessorLinksAppended, workCountBoardComparable, "")
		case "raw_selected_internal_nodes", "raw_selected_internal_parent_occurrences", "raw_selected_internal_leaf_occurrences", "construction_surplus_parent", "construction_surplus_leaf":
			missing(workCountUnavailable(definition.GoUnit, "production-Go accepted raw internal graph census is not available before normalization"), workCountUnavailable(definition.StaticCUnit, "direct C value exists but no paired production-Go raw census"), workCountBoardComparable, "production-Go raw internal census absent")
		case "physical_versions_created_proxy":
			present(goCounts.StackVersionCreationsProxy, cCounts.StackVersionCreationsProxy, workCountBoardIncomparable, "physical version lifecycle proxy; no ratio")
		case "physical_records_discarded_proxy":
			missing(workCountUnavailable(definition.GoUnit, "physical discard cause not attributed"), workCountUnavailable(definition.StaticCUnit, "physical discard cause not attributed"), workCountBoardIncomparable, "physical lifecycle proxy unavailable")
		case "public_selected_nodes":
			present(goCounts.SelectedNodes, cCounts.SelectedNodes, workCountBoardComparable, "public deep-parity census")
		default:
			t.Fatalf("board contract metric %q has no mapping", definition.ID)
		}
	}
	return metrics
}

func workCountBuildParserCoreBoardMetrics(t *testing.T, contract workCountBoardContract, candidate workCountBoardParserCoreChildResult, cCounts workCountCounters, cDirect workCountBoardDirectCounts) []workCountBoardMetric {
	t.Helper()
	workCountValidateCounters(t, "static C", cCounts)
	if err := workCountValidateParserCoreCounterEnvelope(candidate); err != nil {
		t.Fatalf("invalid parser-core counter envelope: %v", err)
	}
	if candidate.BoardDirect.Schema != "gts-work-count-board-direct/v3" || cDirect.Schema != candidate.BoardDirect.Schema || candidate.BoardDirect.Overflow || cDirect.Overflow || !candidate.BoardDirect.FrontierLexerElectionsAvailable || candidate.BoardDirect.PerVersionLexRequestsAvailable || cDirect.FrontierLexerElectionsAvailable || !cDirect.PerVersionLexRequestsAvailable {
		t.Fatalf("invalid parser-core/static-C board counters: candidate=%+v C=%+v", candidate.CoreWork, cDirect)
	}
	if cCounts.LeafConstructionsProxy < cDirect.RawSelectedInternalLeaves || cCounts.ParentConstructionsProxy < cDirect.RawSelectedInternalParents {
		t.Fatalf("static C raw construction surplus underflow: counters=%+v raw=%+v", cCounts, cDirect)
	}
	metrics := make([]workCountBoardMetric, 0, len(contract.Metrics))
	for _, definition := range contract.Metrics {
		present := func(goValue, cValue uint64, comparison, reason string) {
			metrics = append(metrics, workCountBoardMetricValue(definition, workCountPresent(definition.GoUnit, goValue), workCountPresent(definition.StaticCUnit, cValue), comparison, reason))
		}
		missing := func(goValue, cValue workCountBoardEngineValue, comparison, reason string) {
			metrics = append(metrics, workCountBoardMetricValue(definition, goValue, cValue, comparison, reason))
		}
		candidateUnavailable := func(reason string) workCountBoardEngineValue {
			return workCountUnavailable(definition.GoUnit, "compact parser-core: "+reason)
		}
		switch definition.ID {
		case "lexer_elections":
			missing(workCountPresent(definition.GoUnit, candidate.BoardDirect.FrontierLexerElections), workCountUnavailable(definition.StaticCUnit, "locked C exposes per-version ts_parser__lex cache misses, not a runnable union-frontier election"), workCountBoardIncomparable, "C per-version lex request cannot satisfy the Go union-frontier contract")
		case "lexer_calls":
			present(candidate.BoardDirect.RawMainLexerInvocations, cDirect.RawMainLexerInvocations, workCountBoardComparable, "direct main-DFA callable-entry seam; comparability is scoped to the clean locked quartet")
		case "resolved_action_cells_examined":
			present(candidate.BoardDirect.ResolvedActionCellsExamined, cDirect.ResolvedActionCellsExamined, workCountBoardComparable, "direct dispatch seam; comparability is scoped to the clean locked quartet")
		case "action_table_lookups_proxy":
			present(candidate.SchedulerWork.ActionLookups, cCounts.TableLookupsProxy, workCountBoardIncomparable, "implementation-specific compact ClassifyBoundary versus C table lookup front doors")
		case "action_entries_examined_proxy":
			missing(candidateUnavailable("scheduler records conflict action cardinality but not total action entries exposed by single-action cells"), workCountPresent(definition.StaticCUnit, cCounts.ActionEntriesExaminedProxy), workCountBoardIncomparable, "candidate total action-entry proxy unavailable")
		case "shifts":
			present(candidate.CoreWork.Shifts, cCounts.Shifts, workCountBoardComparable, "")
		case "reductions":
			present(candidate.CoreWork.Reductions, cCounts.Reductions, workCountBoardComparable, "")
		case "explicit_recover_actions":
			present(0, cCounts.ExplicitRecoverActions, workCountBoardComparable, "authenticated compact route declines recovery")
		case "accept_actions":
			present(candidate.SchedulerWork.Accepts, cCounts.AcceptActions, workCountBoardComparable, "")
		case "conflict_action_arms_admitted":
			present(candidate.SchedulerWork.ConflictActionArmsAdmitted, cDirect.ConflictActionArmsAdmitted, workCountBoardComparable, "")
		case "causal_conflict_forks":
			present(candidate.SchedulerWork.CausalConflictForks, cDirect.CausalConflictForks, workCountBoardComparable, "")
		case "raw_action_entries_beyond_first":
			missing(candidateUnavailable("scheduler Forks records generic conflict-arm fanout, not an independently authenticated raw-cell hook"), workCountPresent(definition.StaticCUnit, cDirect.RawActionEntriesBeyondFirst), workCountBoardComparable, "candidate raw action-entry hook absent")
		case "stack_version_creations_proxy":
			missing(candidateUnavailable("persistent compact graph has no C-style stack-version creation class"), workCountPresent(definition.StaticCUnit, cCounts.StackVersionCreationsProxy), workCountBoardIncomparable, "representation-specific version proxy absent")
		case "reduction_pop_requests":
			present(candidate.CoreWork.ReductionPopRequests, cCounts.ReductionPopRequests, workCountBoardComparable, "")
		case "emitted_pop_paths":
			present(candidate.CoreWork.EmittedPopPaths, cCounts.EmittedPopPaths, workCountBoardComparable, "")
		case "emitted_pop_payloads":
			present(candidate.CoreWork.EmittedPopPayloads, cCounts.EmittedPopPayloads, workCountBoardComparable, "")
		case "graph_link_additions_proxy":
			present(candidate.CoreWork.GraphLinkAdditionsProxy, cCounts.GraphLinkAdditionsProxy, workCountBoardIncomparable, "representation-specific physical graph links")
		case "subtree_leaf_constructions_proxy":
			present(candidate.CoreWork.LeafConstructionsProxy, cCounts.LeafConstructionsProxy, workCountBoardIncomparable, "compact records versus C transient subtrees")
		case "subtree_parent_constructions_proxy":
			present(candidate.CoreWork.ParentConstructionsProxy, cCounts.ParentConstructionsProxy, workCountBoardIncomparable, "compact records versus C transient subtrees")
		case "predecessor_link_union_attempts":
			present(candidate.CoreWork.PredecessorLinkUnionAttempts, cDirect.PredecessorLinkUnionAttempts, workCountBoardComparable, "")
		case "predecessor_link_union_duplicate_noop":
			present(candidate.CoreWork.PredecessorLinkUnionDuplicateNoop, cDirect.PredecessorLinkUnionDuplicateNoop, workCountBoardComparable, "")
		case "predecessor_link_union_precedence_replaced":
			present(candidate.CoreWork.PredecessorLinkUnionPrecedenceReplaced, cDirect.PredecessorLinkUnionPrecedenceReplaced, workCountBoardComparable, "")
		case "predecessor_link_union_recursive_changed":
			present(candidate.CoreWork.PredecessorLinkUnionRecursiveChanged, cDirect.PredecessorLinkUnionRecursiveChanged, workCountBoardComparable, "")
		case "predecessor_link_union_alternate_appended":
			present(candidate.CoreWork.PredecessorLinkUnionAlternateAppended, cDirect.PredecessorLinkUnionAlternateAppended, workCountBoardComparable, "")
		case "predecessor_link_union_rejected":
			present(candidate.CoreWork.PredecessorLinkUnionRejected, cDirect.PredecessorLinkUnionRejected, workCountBoardComparable, "")
		case "predecessor_link_union_material_changes":
			goChanges := candidate.CoreWork.PredecessorLinkUnionPrecedenceReplaced + candidate.CoreWork.PredecessorLinkUnionRecursiveChanged + candidate.CoreWork.PredecessorLinkUnionAlternateAppended
			cChanges := cDirect.PredecessorLinkUnionPrecedenceReplaced + cDirect.PredecessorLinkUnionRecursiveChanged + cDirect.PredecessorLinkUnionAlternateAppended
			present(goChanges, cChanges, workCountBoardComparable, "derived exact outcome partition")
		case "merge_attempts_proxy", "merge_successes_proxy":
			missing(candidateUnavailable("compact boundary canonicalization has no production-Go merge-front-door proxy"), workCountPresent(definition.StaticCUnit, map[string]uint64{"merge_attempts_proxy": cCounts.MergeAttemptsProxy, "merge_successes_proxy": cCounts.MergeSuccessesProxy}[definition.ID]), workCountBoardIncomparable, "candidate merge proxy unavailable")
		case "alternate_predecessor_links_appended":
			present(candidate.CoreWork.PredecessorLinkUnionAlternateAppended, cDirect.AlternatePredecessorLinksAppended, workCountBoardComparable, "")
		case "raw_selected_internal_nodes":
			present(candidate.RawSelected.Nodes, cDirect.RawSelectedInternalNodes, workCountBoardComparable, "pre-public accepted internal graph")
		case "raw_selected_internal_parent_occurrences":
			present(candidate.RawSelected.Parents, cDirect.RawSelectedInternalParents, workCountBoardComparable, "pre-public accepted internal graph")
		case "raw_selected_internal_leaf_occurrences":
			present(candidate.RawSelected.Leaves, cDirect.RawSelectedInternalLeaves, workCountBoardComparable, "pre-public accepted internal graph")
		case "construction_surplus_parent":
			present(candidate.CoreWork.ParentConstructionsProxy-candidate.RawSelected.Parents, cCounts.ParentConstructionsProxy-cDirect.RawSelectedInternalParents, workCountBoardComparable, "constructor calls minus raw selected occurrences; not discard")
		case "construction_surplus_leaf":
			present(candidate.CoreWork.LeafConstructionsProxy-candidate.RawSelected.Leaves, cCounts.LeafConstructionsProxy-cDirect.RawSelectedInternalLeaves, workCountBoardComparable, "constructor calls minus raw selected occurrences; not discard")
		case "physical_versions_created_proxy":
			missing(candidateUnavailable("compact headers have no C StackHead lifecycle identity"), workCountPresent(definition.StaticCUnit, cCounts.StackVersionCreationsProxy), workCountBoardIncomparable, "physical lifecycle proxy; no ratio")
		case "physical_records_discarded_proxy":
			missing(candidateUnavailable("append-only arena does not expose C refcount last-release identity"), workCountUnavailable(definition.StaticCUnit, "C last releases are not causally attributed"), workCountBoardIncomparable, "physical lifecycle proxy; no ratio")
		case "public_selected_nodes":
			present(candidate.Selected.Nodes, cCounts.SelectedNodes, workCountBoardComparable, "public deep-parity census")
		default:
			t.Fatalf("board contract metric %q has no parser-core mapping", definition.ID)
		}
	}
	return metrics
}

func workCountScheduling(result workCountGoChildResult) workCountBoardScheduling {
	return workCountBoardScheduling{MaxStacksSeen: result.MaxStacksSeen, MultiStackIters: result.MultiStackIters, MultiStackTokens: result.MultiStackTokens}
}

func workCountBoardStatuses(contract workCountBoardContract, rows []workCountBoardRow) (string, []string, string, []string) {
	mandatory := make(map[string]bool, len(contract.Metrics))
	for _, definition := range contract.Metrics {
		mandatory[definition.ID] = definition.Mandatory
	}
	var instrumentationBlockers []string
	var workAuditFindings []string
	seenBlocker := make(map[string]bool)
	for _, row := range rows {
		for _, metric := range row.Metrics {
			if !mandatory[metric.ID] {
				continue
			}
			if metric.Go.State != workCountBoardStatePresent || metric.StaticC.State != workCountBoardStatePresent || metric.Comparison != workCountBoardComparable {
				reason := metric.ComparisonReason
				if reason == "" {
					reason = fmt.Sprintf("Go state=%s, static C state=%s, comparison=%s", metric.Go.State, metric.StaticC.State, metric.Comparison)
				}
				blocker := metric.ID + ": " + reason
				if !seenBlocker[blocker] {
					seenBlocker[blocker] = true
					instrumentationBlockers = append(instrumentationBlockers, blocker)
				}
				continue
			}
			if metric.RatioStatus != workCountBoardRatioComputed || metric.GoOverC == nil {
				continue
			}
			if ratio := *metric.GoOverC; ratio < workCountBoardWorkAuditMinimum || ratio > workCountBoardWorkAuditMaximum {
				identity := metric.ID
				if row.Fixture != "" {
					identity = row.Fixture + "/" + identity
				}
				workAuditFindings = append(workAuditFindings, fmt.Sprintf("%s: Go/C ratio %.6f outside [%.1f,%.1f]", identity, ratio, workCountBoardWorkAuditMinimum, workCountBoardWorkAuditMaximum))
			}
		}
	}
	instrumentationStatus := workCountBoardInstrumentationComplete
	if len(instrumentationBlockers) != 0 {
		instrumentationStatus = workCountBoardInstrumentationBlocked
	}
	workAuditStatus := workCountBoardWorkAuditClear
	if len(workAuditFindings) != 0 {
		workAuditStatus = workCountBoardWorkAuditFindings
	}
	return instrumentationStatus, instrumentationBlockers, workAuditStatus, workAuditFindings
}

func workCountValidateBoard(t *testing.T, contract workCountBoardContract, board workCountBoard) {
	t.Helper()
	if board.Schema != workCountBoardSchema || board.BoardContract != contract.Schema || board.BoardContractSHA256 != workCountBoardContractSHA256 {
		t.Fatalf("board identity drifted: %+v", board)
	}
	if board.Proof.DiagnosticArtifactsTimingEligible || !board.Proof.UntaggedAssemblyPassed ||
		board.Proof.GoBackend != workCountBoardGoProduction && board.Proof.GoBackend != workCountBoardGoParserCore {
		t.Fatalf("board instrumentation proof invalid: %+v", board.Proof)
	}
	for _, row := range board.Rows {
		if row.GoBackend != board.Proof.GoBackend || len(row.Metrics) != len(contract.Metrics) {
			t.Fatalf("row scheduling/metric contract drift: %+v", row)
		}
		switch row.GoBackend {
		case workCountBoardGoProduction:
			if row.TaggedGoScheduling == nil || *row.TaggedGoScheduling != row.UntaggedGoScheduling || row.ParserCoreScheduler != nil || row.RepeatIdenticalCounts {
				t.Fatalf("production row scheduling proof invalid: %+v", row)
			}
		case workCountBoardGoParserCore:
			if row.TaggedGoScheduling != nil || row.ParserCoreScheduler == nil || !row.RepeatIdenticalCounts || row.ParserCoreScheduler.Accepts != 1 || row.ParserCoreScheduler.ActionLookups == 0 {
				t.Fatalf("parser-core row scheduling proof invalid: %+v", row)
			}
		default:
			t.Fatalf("unknown row backend: %+v", row)
		}
		for index, metric := range row.Metrics {
			if metric.ID != contract.Metrics[index].ID || metric.Category != contract.Metrics[index].Category || metric.Mandatory != contract.Metrics[index].Mandatory {
				t.Fatalf("metric contract drift: %+v want=%+v", metric, contract.Metrics[index])
			}
			if metric.Go.State == workCountBoardStatePresent && metric.Go.Value == nil || metric.StaticC.State == workCountBoardStatePresent && metric.StaticC.Value == nil {
				t.Fatalf("present metric lacks value: %+v", metric)
			}
			if metric.Go.State == workCountBoardStateUnavailable && metric.Go.Value != nil || metric.StaticC.State == workCountBoardStateUnavailable && metric.StaticC.Value != nil {
				t.Fatalf("unavailable metric carries value: %+v", metric)
			}
			switch metric.RatioStatus {
			case workCountBoardRatioComputed:
				if metric.GoOverC == nil || metric.Comparison != workCountBoardComparable || metric.Go.Value == nil || metric.StaticC.Value == nil || *metric.StaticC.Value == 0 {
					t.Fatalf("invalid computed ratio: %+v", metric)
				}
			case workCountBoardRatioZero:
				if metric.GoOverC != nil || metric.Comparison != workCountBoardComparable || metric.StaticC.Value == nil || *metric.StaticC.Value != 0 {
					t.Fatalf("invalid zero-denominator ratio: %+v", metric)
				}
			case workCountBoardRatioIncomparable:
				if metric.GoOverC != nil || metric.Comparison != workCountBoardIncomparable {
					t.Fatalf("invalid incomparable ratio: %+v", metric)
				}
			case workCountBoardRatioUnavailable:
				if metric.GoOverC != nil || metric.Go.State != workCountBoardStateUnavailable && metric.StaticC.State != workCountBoardStateUnavailable {
					t.Fatalf("invalid unavailable ratio: %+v", metric)
				}
			default:
				t.Fatalf("unknown ratio status: %+v", metric)
			}
		}
	}
	instrumentationStatus, instrumentationBlockers, workAuditStatus, workAuditFindings := workCountBoardStatuses(contract, board.Rows)
	if instrumentationStatus != board.InstrumentationStatus ||
		strings.Join(instrumentationBlockers, "\n") != strings.Join(board.InstrumentationBlockers, "\n") ||
		workAuditStatus != board.WorkAuditStatus ||
		strings.Join(workAuditFindings, "\n") != strings.Join(board.WorkAuditFindings, "\n") {
		t.Fatalf("board statuses drifted: got=%q/%v %q/%v want=%q/%v %q/%v",
			board.InstrumentationStatus, board.InstrumentationBlockers, board.WorkAuditStatus, board.WorkAuditFindings,
			instrumentationStatus, instrumentationBlockers, workAuditStatus, workAuditFindings)
	}
}

func workCountRunUntaggedAssemblyProof(t *testing.T, sourceRoot, artifact string, environment workCountEnvironment) bool {
	t.Helper()
	env := workCountSanitizedEnv(os.Environ(), environment.Runtime, nil)
	stdout, stderr, err := workCountRunCaptured(sourceRoot, env, workCountBuildTimeout, artifact,
		"-test.run", "^TestWorkCountProductionAssemblyHasNoDiagnosticScaffolding$", "-test.count=1", "-test.v")
	if err != nil {
		t.Fatalf("untagged assembly proof: %v: stdout=%s stderr=%s", err, strings.TrimSpace(string(stdout)), strings.TrimSpace(string(stderr)))
	}
	if len(stderr) != 0 {
		t.Fatalf("untagged assembly proof wrote stderr: %s", strings.TrimSpace(string(stderr)))
	}
	const pass = "--- PASS: TestWorkCountProductionAssemblyHasNoDiagnosticScaffolding"
	if !strings.Contains(string(stdout), pass) {
		t.Fatalf("untagged assembly proof lacks explicit pass marker: %s", strings.TrimSpace(string(stdout)))
	}
	return true
}

func TestWorkCountBoardMetricStatusAndRatios(t *testing.T) {
	contract := workCountLoadBoardContract(t, filepath.Join("work_count", "board_contract_v3.json"))
	counts := workCountCounters{Contract: workCountContract, workCountCounterValues: workCountCounterValues{
		Shifts: 8, Reductions: 5, AcceptActions: 1, ReductionPopRequests: 5,
		EmittedPopPaths: 6, EmittedPopPayloads: 9, SelectedNodes: 3, SelectedParentNodes: 1, SelectedLeafNodes: 2,
		TableLookupsProxy: 12, ActionEntriesExaminedProxy: 10, LexerFrontDoorCallsProxy: 7,
		StackVersionCreationsProxy: 5, MergeAttemptsProxy: 4, MergeSuccessesProxy: 2,
		GraphLinkAdditionsProxy: 11, LeafConstructionsProxy: 6, ParentConstructionsProxy: 4,
	}}
	direct := workCountBoardDirectCounts{
		Schema: "gts-work-count-board-direct/v3", FrontierLexerElectionsAvailable: true,
		FrontierLexerElections: 7, RawMainLexerInvocations: 7, ResolvedActionCellsExamined: 12,
		RawActionEntriesBeyondFirst: 4, ConflictActionArmsAdmitted: 4, CausalConflictForks: 1,
		PredecessorLinkUnionAttempts: 3, PredecessorLinkUnionDuplicateNoop: 1,
		PredecessorLinkUnionAlternateAppended: 2, AlternatePredecessorLinksAppended: 2,
		RawSelectedInternalNodes: 3, RawSelectedInternalParents: 1, RawSelectedInternalLeaves: 2,
	}
	cDirect := direct
	cDirect.FrontierLexerElectionsAvailable = false
	cDirect.FrontierLexerElections = 0
	cDirect.PerVersionLexRequestsAvailable = true
	cDirect.PerVersionLexRequests = 9
	metrics := workCountBuildBoardMetrics(t, contract, counts, counts, direct, cDirect)
	byID := make(map[string]workCountBoardMetric, len(metrics))
	for _, metric := range metrics {
		byID[metric.ID] = metric
	}
	if metric := byID["explicit_recover_actions"]; metric.Go.Value == nil || *metric.Go.Value != 0 || metric.RatioStatus != workCountBoardRatioZero {
		t.Fatalf("present-zero encoding drifted: %+v", metric)
	}
	if metric := byID["action_table_lookups_proxy"]; metric.RatioStatus != workCountBoardRatioIncomparable || metric.GoOverC != nil {
		t.Fatalf("incomparable ratio drifted: %+v", metric)
	}
	if metric := byID["causal_conflict_forks"]; metric.Go.State != workCountBoardStateUnavailable || metric.RatioStatus != workCountBoardRatioUnavailable {
		t.Fatalf("unavailable fork encoding drifted: %+v", metric)
	}
	if metric := byID["public_selected_nodes"]; metric.RatioStatus != workCountBoardRatioComputed || metric.GoOverC == nil || *metric.GoOverC != 1 {
		t.Fatalf("computed ratio drifted: %+v", metric)
	}
	if metric := byID["raw_action_entries_beyond_first"]; metric.RatioStatus != workCountBoardRatioComputed || metric.GoOverC == nil || *metric.GoOverC != 1 {
		t.Fatalf("conflict-alternative ratio drifted: %+v", metric)
	}
	for _, id := range []string{"lexer_calls", "resolved_action_cells_examined"} {
		if metric := byID[id]; metric.Go.State != workCountBoardStatePresent || metric.StaticC.State != workCountBoardStatePresent || metric.Comparison != workCountBoardComparable || metric.RatioStatus != workCountBoardRatioComputed || metric.GoOverC == nil || *metric.GoOverC != 1 {
			t.Fatalf("paired direct metric %q drifted: %+v", id, metric)
		}
	}
	if metric := byID["lexer_elections"]; metric.Go.State != workCountBoardStatePresent || metric.StaticC.State != workCountBoardStateUnavailable || metric.Comparison != workCountBoardIncomparable || metric.RatioStatus != workCountBoardRatioUnavailable || metric.GoOverC != nil {
		t.Fatalf("C per-version lex requests masqueraded as union-frontier elections: %+v", metric)
	}
	candidate := workCountBoardParserCoreChildResult{
		BoardDirect: workCountBoardDirectCounts{
			Schema: "gts-work-count-board-direct/v3", FrontierLexerElectionsAvailable: true, FrontierLexerElections: 7,
			RawMainLexerInvocations: 7, ResolvedActionCellsExamined: 12,
		},
		CoreWork: workCountBoardParserCoreWork{
			Shifts: 8, Reductions: 5, ReductionPopRequests: 5,
			EmittedPopPaths: 6, EmittedPopPayloads: 9,
			GraphLinkAdditionsProxy: 11, LeafConstructionsProxy: 6, ParentConstructionsProxy: 4,
			PredecessorLinkUnionAttempts: 3, PredecessorLinkUnionDuplicateNoop: 1,
			PredecessorLinkUnionAlternateAppended: 2,
		},
		SchedulerWork: workCountBoardParserCoreScheduler{ActionLookups: 12, Accepts: 1, Elections: 7, Forks: 4, Conflicts: 3, ConflictActionArmsAdmitted: 4, CausalConflictForks: 1},
		Selected:      workCountBoardSelectedCensus{Nodes: 3, Parents: 1, Leaves: 2},
		RawSelected:   workCountBoardRawSelectedCensus{Nodes: 3, Parents: 1, Leaves: 2},
	}
	overflowCandidate := candidate
	overflowCandidate.SchedulerWork.ConflictActionArmsAdmitted = math.MaxUint64
	overflowCandidate.SchedulerWork.CausalConflictForks = math.MaxUint64
	overflowCandidate.SchedulerWork.Overflow = true
	if err := workCountValidateParserCoreCounterEnvelope(overflowCandidate); err == nil || !strings.Contains(err.Error(), "scheduler=true") {
		t.Fatalf("scheduler saturation did not fail closed: %v", err)
	}
	candidateMetrics := workCountBuildParserCoreBoardMetrics(t, contract, candidate, counts, cDirect)
	candidateByID := make(map[string]workCountBoardMetric, len(candidateMetrics))
	for _, metric := range candidateMetrics {
		candidateByID[metric.ID] = metric
	}
	for _, id := range []string{"raw_action_entries_beyond_first"} {
		if metric := candidateByID[id]; metric.Go.State != workCountBoardStateUnavailable || metric.RatioStatus != workCountBoardRatioUnavailable {
			t.Fatalf("parser-core scheduler proxy must not masquerade as direct %q: %+v", id, metric)
		}
	}
	for _, id := range []string{"lexer_calls", "resolved_action_cells_examined"} {
		if metric := candidateByID[id]; metric.Go.State != workCountBoardStatePresent || metric.StaticC.State != workCountBoardStatePresent || metric.Comparison != workCountBoardComparable || metric.RatioStatus != workCountBoardRatioComputed || metric.GoOverC == nil || *metric.GoOverC != 1 {
			t.Fatalf("parser-core paired direct metric %q drifted: %+v", id, metric)
		}
	}
	if metric := candidateByID["lexer_elections"]; metric.Go.State != workCountBoardStatePresent || metric.StaticC.State != workCountBoardStateUnavailable || metric.Comparison != workCountBoardIncomparable || metric.RatioStatus != workCountBoardRatioUnavailable || metric.GoOverC != nil {
		t.Fatalf("parser-core C per-version lex requests masqueraded as union-frontier elections: %+v", metric)
	}
	if metric := candidateByID["action_table_lookups_proxy"]; metric.Go.Value == nil || *metric.Go.Value != candidate.SchedulerWork.ActionLookups || metric.Comparison != workCountBoardIncomparable || metric.RatioStatus != workCountBoardRatioIncomparable {
		t.Fatalf("parser-core action lookup proxy classification drifted: %+v", metric)
	}
	if metric := candidateByID["shifts"]; metric.Go.Value == nil || *metric.Go.Value != candidate.CoreWork.Shifts || metric.Comparison != workCountBoardComparable {
		t.Fatalf("parser-core direct shift mapping drifted: %+v", metric)
	}
	if metric := candidateByID["predecessor_link_union_attempts"]; metric.Go.Value == nil || *metric.Go.Value != 3 || metric.RatioStatus != workCountBoardRatioComputed {
		t.Fatalf("parser-core direct union mapping drifted: %+v", metric)
	}
	scheduling := workCountBoardScheduling{MaxStacksSeen: 2}
	row := workCountBoardRow{GoBackend: workCountBoardGoProduction, UntaggedGoScheduling: scheduling, TaggedGoScheduling: &scheduling, Metrics: metrics}
	instrumentationStatus, instrumentationBlockers, workAuditStatus, workAuditFindings := workCountBoardStatuses(contract, []workCountBoardRow{row})
	if instrumentationStatus != workCountBoardInstrumentationBlocked || len(instrumentationBlockers) == 0 {
		t.Fatalf("mandatory unavailable metrics must block instrumentation: status=%q blockers=%v", instrumentationStatus, instrumentationBlockers)
	}
	if workAuditStatus != workCountBoardWorkAuditClear || len(workAuditFindings) != 0 {
		t.Fatalf("equal available metrics must not create work-audit findings: status=%q findings=%v", workAuditStatus, workAuditFindings)
	}
	lowRatio := byID["alternate_predecessor_links_appended"]
	zero := 0.0
	lowRatio.GoOverC = &zero
	lowRatio.RatioStatus = workCountBoardRatioComputed
	lowRatio.Go = workCountPresent(lowRatio.Go.Unit, 0)
	lowRatio.StaticC = workCountPresent(lowRatio.StaticC.Unit, 1)
	row.Metrics = append([]workCountBoardMetric(nil), metrics...)
	for index := range row.Metrics {
		if row.Metrics[index].ID == lowRatio.ID {
			row.Metrics[index] = lowRatio
		}
	}
	lowInstrumentationStatus, lowInstrumentationBlockers, lowWorkAuditStatus, lowWorkAuditFindings := workCountBoardStatuses(contract, []workCountBoardRow{row})
	if lowInstrumentationStatus != instrumentationStatus || strings.Join(lowInstrumentationBlockers, "\n") != strings.Join(instrumentationBlockers, "\n") {
		t.Fatalf("work-ratio finding changed instrumentation completeness: before=%q/%v after=%q/%v", instrumentationStatus, instrumentationBlockers, lowInstrumentationStatus, lowInstrumentationBlockers)
	}
	if lowWorkAuditStatus != workCountBoardWorkAuditFindings || !strings.Contains(strings.Join(lowWorkAuditFindings, "\n"), "alternate_predecessor_links_appended: Go/C ratio 0.000000 outside [0.8,1.2]") {
		t.Fatalf("low-ratio work-audit finding missing: status=%q findings=%v", lowWorkAuditStatus, lowWorkAuditFindings)
	}
	highRatio := byID["resolved_action_cells_examined"]
	highRatioValue := 1.25
	highRatio.GoOverC = &highRatioValue
	highRatio.Go = workCountPresent(highRatio.Go.Unit, 5)
	highRatio.StaticC = workCountPresent(highRatio.StaticC.Unit, 4)
	row.Metrics = append([]workCountBoardMetric(nil), metrics...)
	for index := range row.Metrics {
		if row.Metrics[index].ID == highRatio.ID {
			row.Metrics[index] = highRatio
		}
	}
	_, _, highWorkAuditStatus, highWorkAuditFindings := workCountBoardStatuses(contract, []workCountBoardRow{row})
	if highWorkAuditStatus != workCountBoardWorkAuditFindings || !strings.Contains(strings.Join(highWorkAuditFindings, "\n"), "resolved_action_cells_examined: Go/C ratio 1.250000 outside [0.8,1.2]") {
		t.Fatalf("high-ratio work-audit finding missing: status=%q findings=%v", highWorkAuditStatus, highWorkAuditFindings)
	}
}

func TestWorkCountPerVersionCLexRequestCannotSatisfyUnionFrontierContract(t *testing.T) {
	contract := workCountLoadBoardContract(t, filepath.Join("work_count", "board_contract_v3.json"))
	counts := workCountCounters{Contract: workCountContract, workCountCounterValues: workCountCounterValues{
		SelectedNodes: 1, SelectedLeafNodes: 1,
	}}
	goDirect := workCountBoardDirectCounts{
		Schema: "gts-work-count-board-direct/v3", FrontierLexerElectionsAvailable: true,
		FrontierLexerElections: 1, RawMainLexerInvocations: 1, ResolvedActionCellsExamined: 1,
	}
	cDirect := workCountBoardDirectCounts{
		Schema: "gts-work-count-board-direct/v3", PerVersionLexRequestsAvailable: true,
		PerVersionLexRequests: 2, RawMainLexerInvocations: 1, ResolvedActionCellsExamined: 1,
	}
	assertUnavailable := func(label string, metrics []workCountBoardMetric) {
		t.Helper()
		for _, metric := range metrics {
			if metric.ID != "lexer_elections" {
				continue
			}
			if metric.Go.Value == nil || *metric.Go.Value != 1 || metric.StaticC.State != workCountBoardStateUnavailable || metric.Comparison != workCountBoardIncomparable || metric.RatioStatus != workCountBoardRatioUnavailable || metric.GoOverC != nil {
				t.Fatalf("%s mapped two per-version C lex requests onto one union-frontier election: %+v", label, metric)
			}
			return
		}
		t.Fatalf("%s lacks lexer_elections metric", label)
	}
	assertUnavailable("production", workCountBuildBoardMetrics(t, contract, counts, counts, goDirect, cDirect))
	candidate := workCountBoardParserCoreChildResult{BoardDirect: goDirect}
	assertUnavailable("parsercore", workCountBuildParserCoreBoardMetrics(t, contract, candidate, counts, cDirect))
}

func TestWorkCountParserCoreChildSchedulerOverflowProtocol(t *testing.T) {
	want := workCountBoardParserCoreChildResult{
		Schema: workCountBoardParserCoreChildSchema,
		SchedulerWork: workCountBoardParserCoreScheduler{
			ConflictActionArmsAdmitted: math.MaxUint64,
			CausalConflictForks:        math.MaxUint64,
			Overflow:                   true,
		},
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got workCountBoardParserCoreChildResult
	workCountDecodeExact(t, data, &got)
	if !got.SchedulerWork.Overflow || got.SchedulerWork.ConflictActionArmsAdmitted != math.MaxUint64 || got.SchedulerWork.CausalConflictForks != math.MaxUint64 {
		t.Fatalf("scheduler overflow protocol drifted: %+v", got.SchedulerWork)
	}
	if err := workCountValidateParserCoreCounterEnvelope(got); err == nil || !strings.Contains(err.Error(), "scheduler=true") {
		t.Fatalf("scheduler overflow protocol did not fail closed: %v", err)
	}
}
