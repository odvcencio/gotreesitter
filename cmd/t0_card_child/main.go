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
	"strconv"
	"strings"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

const t0CardChildSchema = "gts-t0-card-go-child/v1"

type t0CardAttempt struct {
	LogicalRung        string                       `json:"logical_rung"`
	OperationCause     string                       `json:"operation_cause"`
	StopReason         gotreesitter.ParseStopReason `json:"stop_reason"`
	RootHasError       bool                         `json:"root_has_error"`
	RootEndByte        uint32                       `json:"root_end_byte"`
	ResolvedMaxStacks  int                          `json:"resolved_max_stacks"`
	ResolvedRetryPass  bool                         `json:"resolved_retry_pass"`
	ResolvedMergeLimit int                          `json:"resolved_max_merge_per_key"`
}

type t0CardRuntime struct {
	StopReason               gotreesitter.ParseStopReason `json:"stop_reason"`
	StoppedEarly             bool                         `json:"stopped_early"`
	TokensConsumed           uint64                       `json:"tokens_consumed"`
	Iterations               int                          `json:"iterations"`
	NodesAllocated           int                          `json:"nodes_allocated"`
	ArenaBytesAllocated      int64                        `json:"arena_bytes_allocated"`
	ArenaBaselineBytes       int64                        `json:"arena_baseline_bytes"`
	ScratchBytesAllocated    int64                        `json:"scratch_bytes_allocated"`
	ScratchBaselineBytes     int64                        `json:"scratch_baseline_bytes"`
	EntryScratchBytes        int64                        `json:"entry_scratch_bytes_allocated"`
	EntryScratchPeak         uint64                       `json:"entry_scratch_peak"`
	GSSBytesAllocated        int64                        `json:"gss_bytes_allocated"`
	GSSBaselineBytes         int64                        `json:"gss_baseline_bytes"`
	GSSSlabCount             int                          `json:"gss_slab_count"`
	GSSNodesUsed             int                          `json:"gss_nodes_used"`
	GSSNodesCapacity         int                          `json:"gss_nodes_capacity"`
	GSSDemotions             uint64                       `json:"gss_demotions"`
	GSSNodesDemoted          uint64                       `json:"gss_nodes_demoted"`
	PeakStackDepth           int                          `json:"peak_stack_depth"`
	MaxStacksSeen            int                          `json:"max_stacks_seen"`
	GSSNodesAllocated        uint64                       `json:"gss_nodes_allocated"`
	GSSNodesRetained         uint64                       `json:"gss_nodes_retained"`
	GSSNodesDropped          uint64                       `json:"gss_nodes_dropped_same_token"`
	ParentNodesAllocated     uint64                       `json:"parent_nodes_allocated"`
	ParentNodesRetained      uint64                       `json:"parent_nodes_retained"`
	LeafNodesAllocated       uint64                       `json:"leaf_nodes_allocated"`
	LeafNodesRetained        uint64                       `json:"leaf_nodes_retained"`
	ParseWallNanos           int64                        `json:"parse_wall_nanos"`
	ParserLoopNanos          int64                        `json:"parser_loop_nanos"`
	TokenNextNanos           int64                        `json:"token_next_nanos"`
	ActionDispatchNanos      int64                        `json:"action_dispatch_nanos"`
	ActionLookupNanos        int64                        `json:"action_lookup_nanos"`
	GLRMergeNanos            int64                        `json:"glr_merge_nanos"`
	GLRCullNanos             int64                        `json:"glr_cull_nanos"`
	ResultSelectionNanos     int64                        `json:"result_selection_nanos"`
	TransientParentNanos     int64                        `json:"transient_parent_materialization_nanos"`
	ResultTreeBuildNanos     int64                        `json:"result_tree_build_nanos"`
	TransientChildNanos      int64                        `json:"transient_child_materialization_nanos"`
	ResultFinalizeRootNanos  int64                        `json:"result_finalize_root_nanos"`
	ResultTrailingNanos      int64                        `json:"result_extend_trailing_nanos"`
	ResultNormalizeNanos     int64                        `json:"result_normalize_root_start_nanos"`
	ResultCompatibilityNanos int64                        `json:"result_compatibility_nanos"`
	ResultParentLinkNanos    int64                        `json:"result_parent_link_nanos"`
	MaterializationNanos     int64                        `json:"materialization_nanos"`
	FinalNodes               uint64                       `json:"final_nodes"`
	FinalParentNodes         uint64                       `json:"final_parent_nodes"`
	FinalLeafNodes           uint64                       `json:"final_leaf_nodes"`
	FinalChildSlices         uint64                       `json:"final_child_slices"`
	FinalChildPointers       uint64                       `json:"final_child_pointers"`
}

type t0CardMemo struct {
	PeakTier    string `json:"peak_tier"`
	PeakEntries uint32 `json:"peak_entries"`
	PeakBytes   uint32 `json:"peak_bytes"`
	Collisions  uint32 `json:"collisions"`
}

type t0CardParse struct {
	WallNanos                    int64                        `json:"wall_nanos"`
	TotalAllocBytes              uint64                       `json:"total_alloc_bytes"`
	Attempts                     []t0CardAttempt              `json:"attempts"`
	SelectedRetryRung            string                       `json:"selected_retry_rung"`
	TreePresent                  bool                         `json:"tree_present"`
	StopReason                   gotreesitter.ParseStopReason `json:"stop_reason"`
	RootStartByte                uint32                       `json:"root_start_byte"`
	RootEndByte                  uint32                       `json:"root_end_byte"`
	RootHasError                 bool                         `json:"root_has_error"`
	DeepTreeSHA256               string                       `json:"deep_tree_sha256"`
	Memo                         t0CardMemo                   `json:"memo"`
	Runtime                      t0CardRuntime                `json:"runtime"`
	TreeObservationRetainedNanos int64                        `json:"tree_observation_retained_nanos"`
	PeakRSSBytes                 uint64                       `json:"peak_rss_bytes"`
	RSSSource                    string                       `json:"rss_source"`
}

type t0CardResponse struct {
	Schema            string      `json:"schema"`
	Language          string      `json:"language"`
	SourceBytes       int         `json:"source_bytes"`
	SourceSHA256      string      `json:"source_sha256"`
	BlobSHA256        string      `json:"blob_sha256"`
	CandidateRevision string      `json:"candidate_revision"`
	BuildModified     bool        `json:"build_modified"`
	Parse             t0CardParse `json:"parse"`
}

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	flags := flag.NewFlagSet("t0-card-child", flag.ContinueOnError)
	language := flags.String("language", "", "language name")
	sourcePath := flags.String("source", "", "source path")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	if strings.TrimSpace(*language) == "" || strings.TrimSpace(*sourcePath) == "" {
		return fmt.Errorf("language and source are required")
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
	languageValue := entry.Language()
	if languageValue == nil {
		return fmt.Errorf("load registered language %q", *language)
	}

	parsed, err := parseOne(languageValue, entry, source)
	if err != nil {
		return err
	}
	blobSum := sha256.Sum256(blob)
	sourceSum := sha256.Sum256(source)
	revision, modified := buildRevision()
	response := t0CardResponse{
		Schema:            t0CardChildSchema,
		Language:          *language,
		SourceBytes:       len(source),
		SourceSHA256:      hex.EncodeToString(sourceSum[:]),
		BlobSHA256:        hex.EncodeToString(blobSum[:]),
		CandidateRevision: revision,
		BuildModified:     modified,
		Parse:             parsed,
	}
	if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	return nil
}

func parseOne(language *gotreesitter.Language, entry *grammars.LangEntry, source []byte) (t0CardParse, error) {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	gotreesitter.BeginDiagnosticRetryTrace()
	started := time.Now()
	parser := gotreesitter.NewParser(language)
	var tree *gotreesitter.Tree
	var err error
	if entry.TokenSourceFactory != nil {
		tree, err = parser.ParseWithTokenSource(source, entry.TokenSourceFactory(source, language))
	} else {
		tree, err = parser.Parse(source)
	}
	wall := time.Since(started)
	trace := gotreesitter.EndDiagnosticRetryTrace()
	runtime.ReadMemStats(&after)
	result := t0CardParse{
		WallNanos:       wall.Nanoseconds(),
		TotalAllocBytes: after.TotalAlloc - before.TotalAlloc,
		Attempts:        make([]t0CardAttempt, 0, len(trace.Attempts)),
	}
	for _, attempt := range trace.Attempts {
		result.Attempts = append(result.Attempts, t0CardAttempt{
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
	if len(result.Attempts) > 0 {
		result.SelectedRetryRung = result.Attempts[len(result.Attempts)-1].LogicalRung
	}
	if err != nil {
		if tree != nil {
			tree.Release()
		}
		return t0CardParse{}, fmt.Errorf("parse: %w", err)
	}
	if tree == nil || tree.RootNode() == nil {
		if tree != nil {
			tree.Release()
		}
		return t0CardParse{}, fmt.Errorf("parse returned no tree")
	}

	parseCompleted := time.Now()
	root := tree.RootNode()
	runtimeResult := tree.ParseRuntime()
	inspection, err := benchfixtures.InspectGoTree(root, language)
	if err != nil {
		tree.Release()
		return t0CardParse{}, fmt.Errorf("deep digest: %w", err)
	}
	memo := tree.RecoveryNodeMemoRuntime()
	result.TreePresent = true
	result.StopReason = runtimeResult.StopReason
	result.RootStartByte = root.StartByte()
	result.RootEndByte = root.EndByte()
	result.RootHasError = root.HasError()
	result.DeepTreeSHA256 = inspection.SHA256
	result.Memo = t0CardMemo{
		PeakTier:    memoTierName(memo.PeakTier),
		PeakEntries: memo.PeakTier.Entries(),
		PeakBytes:   memo.PeakTier.Bytes(),
		Collisions:  memo.Collisions,
	}
	result.Runtime = t0CardRuntimeFrom(runtimeResult, tree.ParseStoppedEarly())
	result.TreeObservationRetainedNanos = time.Since(parseCompleted).Nanoseconds()
	tree.Release()
	result.PeakRSSBytes, result.RSSSource = peakRSSBytes()
	return result, nil
}

func t0CardRuntimeFrom(rt gotreesitter.ParseRuntime, stoppedEarly bool) t0CardRuntime {
	return t0CardRuntime{
		StopReason: rt.StopReason, StoppedEarly: stoppedEarly,
		TokensConsumed: rt.TokensConsumed, Iterations: rt.Iterations, NodesAllocated: rt.NodesAllocated,
		ArenaBytesAllocated: rt.ArenaBytesAllocated, ArenaBaselineBytes: rt.ArenaBaselineBytes,
		ScratchBytesAllocated: rt.ScratchBytesAllocated, ScratchBaselineBytes: rt.ScratchBaselineBytes,
		EntryScratchBytes: rt.EntryScratchBytesAllocated, EntryScratchPeak: rt.EntryScratchPeak,
		GSSBytesAllocated: rt.GSSBytesAllocated, GSSBaselineBytes: rt.GSSBaselineBytes,
		GSSSlabCount: rt.GSSSlabCount, GSSNodesUsed: rt.GSSNodesUsed, GSSNodesCapacity: rt.GSSNodesCapacity,
		GSSDemotions: rt.GSSDemotions, GSSNodesDemoted: rt.GSSNodesDemoted,
		PeakStackDepth: rt.PeakStackDepth, MaxStacksSeen: rt.MaxStacksSeen,
		GSSNodesAllocated: rt.GSSNodesAllocated, GSSNodesRetained: rt.GSSNodesRetained,
		GSSNodesDropped: rt.GSSNodesDroppedSameToken, ParentNodesAllocated: rt.ParentNodesAllocated,
		ParentNodesRetained: rt.ParentNodesRetained, LeafNodesAllocated: rt.LeafNodesAllocated,
		LeafNodesRetained: rt.LeafNodesRetained, ParseWallNanos: rt.ParseWallNanos,
		ParserLoopNanos: rt.ParserLoopNanos, TokenNextNanos: rt.TokenNextNanos,
		ActionDispatchNanos: rt.ActionDispatchNanos, ActionLookupNanos: rt.ActionLookupNanos,
		GLRMergeNanos: rt.GLRMergeNanos, GLRCullNanos: rt.GLRCullNanos,
		ResultSelectionNanos:     rt.ResultSelectionNanos,
		TransientParentNanos:     rt.TransientParentMaterializationNanos,
		ResultTreeBuildNanos:     rt.ResultTreeBuildNanos,
		TransientChildNanos:      rt.TransientChildMaterializationNanos,
		ResultFinalizeRootNanos:  rt.ResultFinalizeRootNanos,
		ResultTrailingNanos:      rt.ResultExtendTrailingNanos,
		ResultNormalizeNanos:     rt.ResultNormalizeRootStartNanos,
		ResultCompatibilityNanos: rt.ResultCompatibilityNanos,
		ResultParentLinkNanos:    rt.ResultParentLinkNanos,
		MaterializationNanos:     sumMaterializationNanos(rt),
		FinalNodes:               rt.FinalNodes, FinalParentNodes: rt.FinalParentNodes,
		FinalLeafNodes: rt.FinalLeafNodes, FinalChildSlices: rt.FinalChildSlices,
		FinalChildPointers: rt.FinalChildPointers,
	}
}

func sumMaterializationNanos(rt gotreesitter.ParseRuntime) int64 {
	values := []int64{
		rt.ResultSelectionNanos,
		rt.TransientParentMaterializationNanos,
		rt.ResultTreeBuildNanos,
		rt.TransientChildMaterializationNanos,
		rt.ResultFinalizeRootNanos,
		rt.ResultExtendTrailingNanos,
		rt.ResultNormalizeRootStartNanos,
		rt.ResultCompatibilityNanos,
		rt.ResultParentLinkNanos,
	}
	var total int64
	for _, value := range values {
		if value > 0 && total <= int64(^uint64(0)>>1)-value {
			total += value
		}
	}
	return total
}

func memoTierName(tier gotreesitter.RecoveryNodeMemoTier) string {
	switch tier {
	case gotreesitter.RecoveryNodeMemoTierNone:
		return "none"
	case gotreesitter.RecoveryNodeMemoTierInitial:
		return "initial"
	case gotreesitter.RecoveryNodeMemoTierStandard:
		return "standard"
	case gotreesitter.RecoveryNodeMemoTierTemporary:
		return "temporary"
	default:
		return "unknown"
	}
}

func peakRSSBytes() (uint64, string) {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, "unavailable"
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "VmHWM:" {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, "unavailable"
		}
		return value * 1024, "linux_proc_vm_hwm"
	}
	return 0, "unavailable"
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
