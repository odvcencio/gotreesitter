//go:build gts_parsercorephase0 && gts_workcount

package gotreesitter

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

const (
	parserCoreWorkCountChildSchema = "gts-work-count-parsercore-child/v5"
	parserCoreWorkCountEngine      = "go-compact-parsercore-phase0-tagged-diagnostic"
	parserCoreWorkCountContract    = "gts-work-count/v2"
	parserCoreWorkCountDigest      = "gts-deep-tree-v1"
	parserCoreWorkCountGrammar     = "2346a3ab1bb3857b48b29d779a1ef9799a248cd7"
	parserCoreWorkCountBlob        = "9cf914d26d962d1a62e7954f8b20b302337a44cb7d4a07218eec482c45a57a08"
)

type parserCoreWorkCountCoreWork struct {
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

type parserCoreWorkCountSchedulerWork struct {
	Passes uint64 `json:"passes"`
	// SingleHeaderPasses is the C4 stage-2 pass-mix row
	// (spec.c4-bytecode-isa.v1 section 5, obligation R6). It makes corridor
	// coverage a committed board row instead of a profile-derived figure.
	SingleHeaderPasses uint64 `json:"single_header_passes"`
	// CorridorPasses is the subset of SingleHeaderPasses the C4 bytecode
	// corridor executed. It is zero whenever the corridor lane is off.
	CorridorPasses             uint64 `json:"corridor_passes"`
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

type parserCoreWorkCountSelectedCensus struct {
	Nodes   uint64 `json:"nodes"`
	Parents uint64 `json:"parents"`
	Leaves  uint64 `json:"leaves"`
}

type parserCoreWorkCountRawSelectedCensus struct {
	Nodes    uint64 `json:"nodes"`
	Parents  uint64 `json:"parents"`
	Leaves   uint64 `json:"leaves"`
	Overflow bool   `json:"overflow"`
}

type parserCoreWorkCountChildResult struct {
	Schema             string                               `json:"schema"`
	Engine             string                               `json:"engine"`
	CounterContract    string                               `json:"counter_contract"`
	DigestFormat       string                               `json:"digest_format"`
	Fixture            string                               `json:"fixture"`
	SourceSHA256       string                               `json:"source_sha256"`
	SourceBytes        uint32                               `json:"source_bytes"`
	GrammarCommit      string                               `json:"grammar_commit"`
	GrammarBlobSHA256  string                               `json:"grammar_blob_sha256"`
	DeepTreeSHA256     string                               `json:"deep_tree_sha256"`
	RootEndByte        uint32                               `json:"root_end_byte"`
	RootHasError       bool                                 `json:"root_has_error"`
	CandidateFallbacks uint64                               `json:"candidate_fallbacks"`
	CoreWork           parserCoreWorkCountCoreWork          `json:"core_work"`
	SchedulerWork      parserCoreWorkCountSchedulerWork     `json:"scheduler_work"`
	Selected           parserCoreWorkCountSelectedCensus    `json:"selected_census"`
	RawSelected        parserCoreWorkCountRawSelectedCensus `json:"raw_selected_internal_census"`
	BoardDirect        DiagnosticWorkCountBoardDirect       `json:"board_direct"`
}

// TestParserCoreWorkCountChild is the fresh-process protocol endpoint used by
// the authenticated work-count board. It executes the exact tagged fresh-full
// runner once, materializes its sole accepted derivation, and emits only
// deterministic counts. It never participates in a timing receipt.
func TestParserCoreWorkCountChild(t *testing.T) {
	fixtureID := strings.TrimSpace(os.Getenv("GTS_WORK_COUNT_FIXTURE"))
	sourcePath := strings.TrimSpace(os.Getenv("GTS_WORK_COUNT_SOURCE"))
	resultPath := strings.TrimSpace(os.Getenv("GTS_WORK_COUNT_RESULT"))
	if fixtureID == "" || sourcePath == "" || resultPath == "" {
		t.Skip("parser-core work-count child protocol is not configured")
	}

	var admission *diagnosticParserCoreCanonicalAdmission
	for index := range diagnosticParserCoreCanonicalAdmissions {
		if diagnosticParserCoreCanonicalAdmissions[index].id == fixtureID {
			admission = &diagnosticParserCoreCanonicalAdmissions[index]
			break
		}
	}
	if admission == nil {
		t.Fatalf("canonical fixture %q is absent", fixtureID)
	}
	fixture := loadDiagnosticParserCoreCanonicalFixture(t, fixtureID)
	requireDiagnosticParserCoreCanonicalFixtureIdentity(t, fixture, *admission)
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(source, fixture.Source) {
		t.Fatalf("source snapshot differs from authenticated fixture %q: bytes=%d sha256=%x", fixtureID, len(source), sha256.Sum256(source))
	}

	runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	BeginDiagnosticWorkCount()
	scheduler, tokenSource, err := runner.executeSchedulerOpen(source, runner.compact, true)
	if err != nil {
		if tokenSource != nil {
			tokenSource.Close()
		}
		t.Fatal(err)
	}
	defer tokenSource.Close()
	derivations, err := runner.compact.Derivations(scheduler.acceptedHead)
	if err != nil || len(derivations) != 1 {
		t.Fatalf("accepted compact derivations=%d err=%v", len(derivations), err)
	}
	rawSelected, err := runner.compact.RawSelectedSubtreeCensus(derivations[0].Payloads)
	if err != nil || rawSelected.Overflow {
		t.Fatalf("raw selected internal census=%+v err=%v", rawSelected, err)
	}
	if rawSelected != admission.rawSelected {
		t.Fatalf("raw selected internal census=%+v want=%+v", rawSelected, admission.rawSelected)
	}
	tree, err := runner.materialize(source, runner.compact, scheduler.acceptedHead)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	if err := validateParserCoreFreshFullCanonicalTree(tree, len(source)); err != nil {
		t.Fatal(err)
	}
	digest := requireDiagnosticParserCoreCanonicalTreeDigest(t, tree, runner.lang)
	if digest != admission.deepTreeSHA256 {
		t.Fatalf("deep tree digest=%s want=%s", digest, admission.deepTreeSHA256)
	}

	selected := parserCoreWorkCountSelectedTree(tree.root)
	if selected.Nodes != admission.selectedNodes || selected.Parents != admission.selectedParents || selected.Leaves != admission.selectedLeaves || selected.Parents+selected.Leaves != selected.Nodes {
		t.Fatalf("selected tree census=%+v want=%d/%d/%d", selected, admission.selectedNodes, admission.selectedParents, admission.selectedLeaves)
	}
	// Summary-mode acceptance intentionally omits its diagnostic selected-node
	// payload. The materialized tree walk above is the board's exact census.
	coreWork := runner.compact.Work()
	if coreWork != admission.work || coreWork.Overflow {
		t.Fatalf("compact work=%+v want=%+v", coreWork, admission.work)
	}
	unionOutcomes := coreWork.PredecessorLinkUnionDuplicateNoop +
		coreWork.PredecessorLinkUnionPrecedenceReplaced +
		coreWork.PredecessorLinkUnionRecursiveChanged +
		coreWork.PredecessorLinkUnionAlternateAppended +
		coreWork.PredecessorLinkUnionRejected
	if unionOutcomes != coreWork.PredecessorLinkUnionAttempts {
		t.Fatalf("link-union outcome partition=%d attempts=%d", unionOutcomes, coreWork.PredecessorLinkUnionAttempts)
	}
	if coreWork.LeafConstructionsProxy < rawSelected.Leaves || coreWork.ParentConstructionsProxy < rawSelected.Parents {
		t.Fatalf("raw construction surplus underflow: work=%+v raw=%+v", coreWork, rawSelected)
	}
	schedulerWork := scheduler.work
	if schedulerWork.Overflow || schedulerWork.ConflictActionArmsAdmitted != admission.conflictArms || schedulerWork.CausalConflictForks != admission.causalForks || schedulerWork.Accepts != 1 || schedulerWork.ActionLookups == 0 || schedulerWork.Elections == 0 || schedulerWork.PeakHeaders < 2 {
		t.Fatalf("invalid scheduler work: %+v", schedulerWork)
	}
	// R6 partition invariant: single-header passes are a subset of all passes,
	// and corridor passes a subset of single-header passes.
	if schedulerWork.Passes == 0 || schedulerWork.SingleHeaderPasses == 0 ||
		schedulerWork.SingleHeaderPasses > schedulerWork.Passes ||
		schedulerWork.CorridorPasses > schedulerWork.SingleHeaderPasses {
		t.Fatalf("invalid pass mix: %+v", schedulerWork)
	}
	boardDirect := EndDiagnosticWorkCount().BoardDirect()
	if !boardDirect.FrontierLexerElectionsAvailable || boardDirect.PerVersionLexRequestsAvailable || boardDirect.FrontierLexerElections != schedulerWork.Elections || boardDirect.ResolvedActionCellsExamined != schedulerWork.ActionLookups || boardDirect.RawMainLexerInvocations == 0 {
		t.Fatalf("direct parser-core hooks diverged from scheduler work: direct=%+v scheduler=%+v", boardDirect, schedulerWork)
	}

	result := parserCoreWorkCountChildResult{
		Schema: parserCoreWorkCountChildSchema, Engine: parserCoreWorkCountEngine,
		CounterContract: parserCoreWorkCountContract, DigestFormat: parserCoreWorkCountDigest,
		Fixture: fixtureID, SourceSHA256: admission.sourceSHA256, SourceBytes: uint32(len(source)),
		GrammarCommit: parserCoreWorkCountGrammar, GrammarBlobSHA256: parserCoreWorkCountBlob,
		DeepTreeSHA256: digest, RootEndByte: tree.root.endByte, RootHasError: tree.root.HasError(),
		CandidateFallbacks: 0,
		CoreWork: parserCoreWorkCountCoreWork{
			Shifts: coreWork.Shifts, Reductions: coreWork.Reductions,
			ReductionPopRequests: coreWork.ReductionPopRequests,
			EmittedPopPaths:      coreWork.EmittedPopPaths, EmittedPopPayloads: coreWork.EmittedPopPayloads,
			GraphLinkAdditionsProxy:                coreWork.GraphLinkAdditionsProxy,
			LeafConstructionsProxy:                 coreWork.LeafConstructionsProxy,
			ParentConstructionsProxy:               coreWork.ParentConstructionsProxy,
			PredecessorLinkUnionAttempts:           coreWork.PredecessorLinkUnionAttempts,
			PredecessorLinkUnionDuplicateNoop:      coreWork.PredecessorLinkUnionDuplicateNoop,
			PredecessorLinkUnionPrecedenceReplaced: coreWork.PredecessorLinkUnionPrecedenceReplaced,
			PredecessorLinkUnionRecursiveChanged:   coreWork.PredecessorLinkUnionRecursiveChanged,
			PredecessorLinkUnionAlternateAppended:  coreWork.PredecessorLinkUnionAlternateAppended,
			PredecessorLinkUnionRejected:           coreWork.PredecessorLinkUnionRejected,
			Overflow:                               coreWork.Overflow,
		},
		SchedulerWork: parserCoreWorkCountSchedulerWork{
			Passes:             schedulerWork.Passes,
			SingleHeaderPasses: schedulerWork.SingleHeaderPasses,
			CorridorPasses:     schedulerWork.CorridorPasses,
			ActionLookups:      schedulerWork.ActionLookups, Accepts: schedulerWork.Accepts,
			Elections: schedulerWork.Elections, Forks: schedulerWork.Forks,
			Conflicts: schedulerWork.Conflicts, ConflictActions: schedulerWork.ConflictActions,
			ConflictActionArmsAdmitted: schedulerWork.ConflictActionArmsAdmitted,
			CausalConflictForks:        schedulerWork.CausalConflictForks,
			Canonicalizations:          schedulerWork.Canonicalizations, PeakHeaders: schedulerWork.PeakHeaders,
			Overflow: schedulerWork.Overflow,
		},
		Selected:    selected,
		RawSelected: parserCoreWorkCountRawSelectedCensus{Nodes: rawSelected.Nodes, Parents: rawSelected.Parents, Leaves: rawSelected.Leaves, Overflow: rawSelected.Overflow},
		BoardDirect: boardDirect,
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(resultPath, data, 0o600); err != nil {
		t.Fatal(fmt.Errorf("write parser-core work-count result: %w", err))
	}
}

func parserCoreWorkCountSelectedTree(root *Node) parserCoreWorkCountSelectedCensus {
	var census parserCoreWorkCountSelectedCensus
	stack := []*Node{root}
	for len(stack) != 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		census.Nodes++
		if len(node.children) == 0 {
			census.Leaves++
			continue
		}
		census.Parents++
		stack = append(stack, node.children...)
	}
	return census
}
