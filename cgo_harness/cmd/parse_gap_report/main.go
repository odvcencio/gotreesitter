//go:build cgo && treesitter_c_parity

package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
	cgoharness "github.com/odvcencio/gotreesitter/cgo_harness"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const resultSchema = "parse-gap-v1"

type corpusManifest struct {
	Schema string          `json:"schema"`
	Sets   []corpusSetSpec `json:"sets"`
}

type corpusSetSpec struct {
	Name         string   `json:"name"`
	Language     string   `json:"language"`
	Root         string   `json:"root,omitempty"`
	Roots        []string `json:"roots,omitempty"`
	Files        []string `json:"files,omitempty"`
	Include      []string `json:"include,omitempty"`
	Order        string   `json:"order,omitempty"`
	MaxFiles     int      `json:"max_files,omitempty"`
	MaxFileBytes int64    `json:"max_file_bytes,omitempty"`
}

type queryManifest struct {
	Schema  string      `json:"schema"`
	Queries []querySpec `json:"queries"`
}

type querySpec struct {
	Language string `json:"language"`
	Name     string `json:"name"`
	Query    string `json:"query"`
}

type editManifest struct {
	Schema   string     `json:"schema"`
	Fixtures []editSpec `json:"fixtures"`
}

type editSpec struct {
	Language string            `json:"language"`
	Name     string            `json:"name"`
	Selector string            `json:"selector"`
	Edit     map[string]string `json:"edit"`
}

type sample struct {
	Set      string
	Language string
	Path     string
	RelPath  string
	Bytes    int
	Source   []byte
	SHA256   string
}

type runner struct {
	name          string
	entry         grammars.LangEntry
	support       grammars.ParseSupport
	goLang        *gotreesitter.Language
	goParser      *gotreesitter.Parser
	cLang         *sitter.Language
	c             *sitter.Parser
	profile       *gotreesitter.AmbiguityProfile
	hotShapeLimit int
}

type reportRow struct {
	Schema      string        `json:"schema"`
	Repo        string        `json:"repo,omitempty"`
	Commit      string        `json:"commit,omitempty"`
	Branch      string        `json:"branch,omitempty"`
	GoVersion   string        `json:"go_version,omitempty"`
	DockerImage string        `json:"docker_image,omitempty"`
	CPULimit    string        `json:"cpu_limit,omitempty"`
	MemoryLimit string        `json:"memory_limit,omitempty"`
	Language    string        `json:"language"`
	Corpus      string        `json:"corpus"`
	Sample      string        `json:"sample"`
	Mode        string        `json:"mode"`
	Bytes       int           `json:"bytes"`
	Iterations  int           `json:"iterations"`
	MedianNS    int64         `json:"median_ns"`
	MeanNS      int64         `json:"mean_ns"`
	P95NS       int64         `json:"p95_ns"`
	BOp         int64         `json:"b_op"`
	AllocsOp    float64       `json:"allocs_op"`
	RSSKB       int64         `json:"rss_kb"`
	Parity      paritySummary `json:"parity"`
	Runtime     runtimeStats  `json:"runtime,omitempty"`
	Error       string        `json:"error,omitempty"`
	Blocker     string        `json:"blocker,omitempty"`
}

type paritySummary struct {
	NoError     bool   `json:"no_error"`
	SExpr       bool   `json:"sexpr"`
	Deep        bool   `json:"deep"`
	Highlight   *bool  `json:"highlight,omitempty"`
	Query       *bool  `json:"query,omitempty"`
	Incremental *bool  `json:"incremental,omitempty"`
	Error       string `json:"error,omitempty"`
}

type runtimeStats struct {
	Tokens                                uint64        `json:"tokens,omitempty"`
	Iterations                            int           `json:"iterations,omitempty"`
	NodesAllocated                        int           `json:"nodes_allocated,omitempty"`
	FinalNodes                            uint64        `json:"final_nodes,omitempty"`
	GSSNodes                              uint64        `json:"gss_nodes,omitempty"`
	MaxStacksSeen                         int           `json:"max_stacks_seen,omitempty"`
	SingleStackIterations                 int           `json:"single_stack_iterations,omitempty"`
	MultiStackIterations                  int           `json:"multi_stack_iterations,omitempty"`
	SingleStackTokens                     uint64        `json:"single_stack_tokens,omitempty"`
	MultiStackTokens                      uint64        `json:"multi_stack_tokens,omitempty"`
	MergeStacksIn                         uint64        `json:"merge_stacks_in,omitempty"`
	MergeStacksOut                        uint64        `json:"merge_stacks_out,omitempty"`
	MergeSlotsUsed                        uint64        `json:"merge_slots_used,omitempty"`
	GlobalCullStacksIn                    uint64        `json:"global_cull_stacks_in,omitempty"`
	GlobalCullStacksOut                   uint64        `json:"global_cull_stacks_out,omitempty"`
	ArenaLiveB                            int64         `json:"arena_live_b,omitempty"`
	ArenaCapacityB                        int64         `json:"arena_capacity_b,omitempty"`
	ArenaCapacityWaste                    uint64        `json:"arena_capacity_waste,omitempty"`
	FinalChildRangeDrains                 uint64        `json:"final_child_range_drains,omitempty"`
	PublicNodesMaterialized               uint64        `json:"public_nodes_materialized,omitempty"`
	DenseFallbacks                        uint64        `json:"dense_fallbacks,omitempty"`
	ResultSelectionNS                     int64         `json:"result_selection_ns,omitempty"`
	ResultBuildNS                         int64         `json:"result_build_ns,omitempty"`
	ResultCompatibilityNS                 int64         `json:"result_compatibility_ns,omitempty"`
	ResultParentLinkNS                    int64         `json:"result_parent_link_ns,omitempty"`
	ResultFinalizeRootNS                  int64         `json:"result_finalize_root_ns,omitempty"`
	ResultExtendTrailingNS                int64         `json:"result_extend_trailing_ns,omitempty"`
	ResultNormalizeRootNS                 int64         `json:"result_normalize_root_start_ns,omitempty"`
	TransientParentMatNS                  int64         `json:"transient_parent_materialize_ns,omitempty"`
	TransientChildMatNS                   int64         `json:"transient_child_materialize_ns,omitempty"`
	NormalizationNS                       int64         `json:"normalization_ns,omitempty"`
	NormalizationPassesRun                uint64        `json:"normalization_passes_run,omitempty"`
	NormalizationNodes                    uint64        `json:"normalization_nodes_visited,omitempty"`
	NormalizationRewrites                 uint64        `json:"normalization_nodes_rewritten,omitempty"`
	NormalizationPasses                   *[]passStats  `json:"normalization_passes,omitempty"`
	ParseWallNS                           int64         `json:"parse_wall_ns,omitempty"`
	ParserLoopNS                          int64         `json:"parser_loop_ns,omitempty"`
	TokenNextNS                           int64         `json:"token_next_ns,omitempty"`
	ActionDispatchNS                      int64         `json:"action_dispatch_ns,omitempty"`
	ActionLookupNS                        int64         `json:"action_lookup_ns,omitempty"`
	GLRMergeNS                            int64         `json:"glr_merge_ns,omitempty"`
	GLRCullNS                             int64         `json:"glr_cull_ns,omitempty"`
	ReduceRangeNS                         int64         `json:"reduce_range_ns,omitempty"`
	ReducePendingParentNS                 int64         `json:"reduce_pending_parent_ns,omitempty"`
	ReduceChildBuildNS                    int64         `json:"reduce_child_build_ns,omitempty"`
	ReduceParentBuildNS                   int64         `json:"reduce_parent_build_ns,omitempty"`
	ReduceSpanNS                          int64         `json:"reduce_span_ns,omitempty"`
	ReduceStackPushNS                     int64         `json:"reduce_stack_push_ns,omitempty"`
	ReduceNoTreeBuildNS                   int64         `json:"reduce_notree_build_ns,omitempty"`
	ActionExtraShiftNS                    int64         `json:"action_extra_shift_ns,omitempty"`
	ActionNoActionNS                      int64         `json:"action_no_action_ns,omitempty"`
	ActionNoActionRelexNS                 int64         `json:"action_no_action_relex_ns,omitempty"`
	ActionNoActionMissingNS               int64         `json:"action_no_action_missing_ns,omitempty"`
	ActionNoActionRecoverNS               int64         `json:"action_no_action_recover_ns,omitempty"`
	ActionNoActionErrorNS                 int64         `json:"action_no_action_error_ns,omitempty"`
	ActionConflictChoiceNS                int64         `json:"action_conflict_choice_ns,omitempty"`
	ActionConflictForkNS                  int64         `json:"action_conflict_fork_ns,omitempty"`
	ActionSingleShiftNS                   int64         `json:"action_single_shift_ns,omitempty"`
	ActionSingleReduceNS                  int64         `json:"action_single_reduce_ns,omitempty"`
	ActionSingleAcceptNS                  int64         `json:"action_single_accept_ns,omitempty"`
	ActionSingleRecoverNS                 int64         `json:"action_single_recover_ns,omitempty"`
	ActionSingleOtherNS                   int64         `json:"action_single_other_ns,omitempty"`
	QueryCaptures                         uint64        `json:"query_captures,omitempty"`
	QueryCompileNS                        int64         `json:"query_compile_ns,omitempty"`
	QueryExecNS                           int64         `json:"query_exec_ns,omitempty"`
	QueryRootNS                           int64         `json:"query_root_ns,omitempty"`
	QueryCursorNS                         int64         `json:"query_cursor_ns,omitempty"`
	CursorNodes                           uint64        `json:"cursor_nodes,omitempty"`
	MergeCalls                            uint64        `json:"merge_calls,omitempty"`
	MergeDeadPruned                       uint64        `json:"merge_dead_pruned,omitempty"`
	MergeReplacements                     uint64        `json:"merge_replacements,omitempty"`
	StackEquivalentCalls                  uint64        `json:"stack_equivalent_calls,omitempty"`
	StackEquivalentTrue                   uint64        `json:"stack_equivalent_true,omitempty"`
	StackEquivDepthMismatch               uint64        `json:"stack_equiv_depth_mismatch,omitempty"`
	StackEquivHashMismatch                uint64        `json:"stack_equiv_hash_mismatch,omitempty"`
	StackEquivStateMismatch               uint64        `json:"stack_equiv_state_mismatch,omitempty"`
	StackEquivPayloadMismatch             uint64        `json:"stack_equiv_payload_mismatch,omitempty"`
	StackEquivEntryCompares               uint64        `json:"stack_equiv_entry_compares,omitempty"`
	StackEquivStateMismatchDepthSum       uint64        `json:"stack_equiv_state_mismatch_depth_sum,omitempty"`
	StackEquivStateMismatchMaxDepth       uint32        `json:"stack_equiv_state_mismatch_max_depth,omitempty"`
	StackEquivStateMismatchDepthBuckets   []uint64      `json:"stack_equiv_state_mismatch_depth_buckets,omitempty"`
	StackEquivPayloadMismatchDepthSum     uint64        `json:"stack_equiv_payload_mismatch_depth_sum,omitempty"`
	StackEquivPayloadMismatchMaxDepth     uint32        `json:"stack_equiv_payload_mismatch_max_depth,omitempty"`
	StackEquivPayloadMismatchDepthBuckets []uint64      `json:"stack_equiv_payload_mismatch_depth_buckets,omitempty"`
	StackEquivPayloadHeaderSigDiff        uint64        `json:"stack_equiv_payload_header_sig_diff,omitempty"`
	StackEquivPayloadHeaderSigSame        uint64        `json:"stack_equiv_payload_header_sig_same,omitempty"`
	StackEquivPayloadShallowSigDiff       uint64        `json:"stack_equiv_payload_shallow_sig_diff,omitempty"`
	StackEquivPayloadShallowSigSame       uint64        `json:"stack_equiv_payload_shallow_sig_same,omitempty"`
	StackEquivPairKeyed                   uint64        `json:"stack_equiv_pair_keyed,omitempty"`
	StackEquivPairUnkeyed                 uint64        `json:"stack_equiv_pair_unkeyed,omitempty"`
	StackEquivPairRepeats                 uint64        `json:"stack_equiv_pair_repeats,omitempty"`
	StackEquivPairRepeatTrue              uint64        `json:"stack_equiv_pair_repeat_true,omitempty"`
	StackEquivPairRepeatFalse             uint64        `json:"stack_equiv_pair_repeat_false,omitempty"`
	StackEquivPairRepeatMismatch          uint64        `json:"stack_equiv_pair_repeat_mismatch,omitempty"`
	StackEquivPairStores                  uint64        `json:"stack_equiv_pair_stores,omitempty"`
	EquivCacheLookups                     uint64        `json:"equiv_cache_lookups,omitempty"`
	EquivCacheHits                        uint64        `json:"equiv_cache_hits,omitempty"`
	EquivCacheStores                      uint64        `json:"equiv_cache_stores,omitempty"`
	EquivCacheMisses                      uint64        `json:"equiv_cache_misses,omitempty"`
	EquivCacheTrueHits                    uint64        `json:"equiv_cache_true_hits,omitempty"`
	EquivCacheFalseHits                   uint64        `json:"equiv_cache_false_hits,omitempty"`
	EquivCacheEpochMisses                 uint64        `json:"equiv_cache_epoch_misses,omitempty"`
	EquivCacheKeyMisses                   uint64        `json:"equiv_cache_key_misses,omitempty"`
	EquivCacheVersionMisses               uint64        `json:"equiv_cache_version_misses,omitempty"`
	EquivSkipError                        uint64        `json:"equiv_skip_error,omitempty"`
	EquivSkipLeaf                         uint64        `json:"equiv_skip_leaf,omitempty"`
	EquivSkipFieldMismatch                uint64        `json:"equiv_skip_field_mismatch,omitempty"`
	EquivExactCalls                       uint64        `json:"equiv_exact_calls,omitempty"`
	EquivExactTrue                        uint64        `json:"equiv_exact_true,omitempty"`
	EquivExactPointerTrue                 uint64        `json:"equiv_exact_pointer_true,omitempty"`
	EquivExactNilMismatch                 uint64        `json:"equiv_exact_nil_mismatch,omitempty"`
	EquivExactHeaderMismatch              uint64        `json:"equiv_exact_header_mismatch,omitempty"`
	EquivExactChildMismatch               uint64        `json:"equiv_exact_child_mismatch,omitempty"`
	EquivExactTerminalCalls               uint64        `json:"equiv_exact_terminal_calls,omitempty"`
	EquivExactTerminalTrue                uint64        `json:"equiv_exact_terminal_true,omitempty"`
	EquivExactTerminalFalse               uint64        `json:"equiv_exact_terminal_false,omitempty"`
	EquivFrontierCalls                    uint64        `json:"equiv_frontier_calls,omitempty"`
	EquivFrontierTrue                     uint64        `json:"equiv_frontier_true,omitempty"`
	EquivExactChildCompares               uint64        `json:"equiv_exact_child_compares,omitempty"`
	EquivFrontierChildScans               uint64        `json:"equiv_frontier_child_scans,omitempty"`
	EquivFrontierCandidateCompares        uint64        `json:"equiv_frontier_candidate_compares,omitempty"`
	StackCompareCalls                     uint64        `json:"stack_compare_calls,omitempty"`
	ForkCount                             uint64        `json:"fork_count,omitempty"`
	ConflictRR                            uint64        `json:"conflict_rr,omitempty"`
	ConflictRS                            uint64        `json:"conflict_rs,omitempty"`
	ConflictOther                         uint64        `json:"conflict_other,omitempty"`
	LexBytes                              uint64        `json:"lex_bytes,omitempty"`
	LexTokens                             uint64        `json:"lex_tokens,omitempty"`
	ReduceChainSteps                      uint64        `json:"reduce_chain_steps,omitempty"`
	ReduceChainMaxLen                     uint64        `json:"reduce_chain_max_len,omitempty"`
	ReduceChainClassHits                  uint64        `json:"reduce_chain_class_hits,omitempty"`
	ReduceChainStopNoAction               uint64        `json:"reduce_chain_stop_no_action,omitempty"`
	ReduceChainStopMulti                  uint64        `json:"reduce_chain_stop_multi,omitempty"`
	ReduceChainStopShift                  uint64        `json:"reduce_chain_stop_shift,omitempty"`
	ReduceChainStopAccept                 uint64        `json:"reduce_chain_stop_accept,omitempty"`
	ReduceChainStopDead                   uint64        `json:"reduce_chain_stop_dead,omitempty"`
	ReduceChainStopCycle                  uint64        `json:"reduce_chain_stop_cycle,omitempty"`
	ReduceChainStopLimit                  uint64        `json:"reduce_chain_stop_limit,omitempty"`
	ReduceChainHintCandidates             uint64        `json:"reduce_chain_hint_candidates,omitempty"`
	ReduceChainHintTaken                  uint64        `json:"reduce_chain_hint_taken,omitempty"`
	ReduceChainHintSteps                  uint64        `json:"reduce_chain_hint_steps,omitempty"`
	ReduceChainHintTerminalOK             uint64        `json:"reduce_chain_hint_terminal_ok,omitempty"`
	ReduceChainHintTerminalMismatch       uint64        `json:"reduce_chain_hint_terminal_mismatch,omitempty"`
	ReduceChainHintLimit                  uint64        `json:"reduce_chain_hint_limit,omitempty"`
	ReduceChainHintDead                   uint64        `json:"reduce_chain_hint_dead,omitempty"`
	ReduceChainHintUnexpected             uint64        `json:"reduce_chain_hint_unexpected_action,omitempty"`
	ParentChildPointers                   uint64        `json:"parent_child_pointers,omitempty"`
	ReduceChildrenFastGSS                 uint64        `json:"reduce_children_fast_gss,omitempty"`
	ReduceChildrenAllVisible              uint64        `json:"reduce_children_all_visible,omitempty"`
	ReduceChildrenNoAlias                 uint64        `json:"reduce_children_no_alias,omitempty"`
	ReduceChildrenScratch                 uint64        `json:"reduce_children_scratch,omitempty"`
	ReduceScratchNoAlias                  uint64        `json:"reduce_scratch_no_alias,omitempty"`
	ReduceScratchGeneral                  uint64        `json:"reduce_scratch_general,omitempty"`
	ReduceChildFastGSS                    *pathStats    `json:"reduce_child_fast_gss,omitempty"`
	ReduceChildAllVisible                 *pathStats    `json:"reduce_child_all_visible,omitempty"`
	ReduceChildNoAlias                    *pathStats    `json:"reduce_child_no_alias,omitempty"`
	ReduceChildScratchGeneral             *pathStats    `json:"reduce_child_scratch_general,omitempty"`
	ReduceChildScratchNoAlias             *pathStats    `json:"reduce_child_scratch_no_alias,omitempty"`
	NoTreeReduceNodes                     uint64        `json:"notree_reduce_nodes,omitempty"`
	NoTreeLeafNodes                       uint64        `json:"notree_leaf_nodes,omitempty"`
	CloneTreePublicNodes                  uint64        `json:"clone_tree_public_nodes,omitempty"`
	CloneOffsetPublicNodes                uint64        `json:"clone_offset_public_nodes,omitempty"`
	NodeEditCompactRefs                   uint64        `json:"node_edit_compact_refs,omitempty"`
	NodeEditPublicFallbacks               uint64        `json:"node_edit_public_fallbacks,omitempty"`
	MutationChildRefCOW                   uint64        `json:"mutation_child_ref_cow,omitempty"`
	HotAmbiguities                        []hotGLRState `json:"hot_ambiguities,omitempty"`
	HotReduceChains                       []hotGLRState `json:"hot_reduce_chains,omitempty"`
	HotReduceChainRuns                    []hotGLRState `json:"hot_reduce_chain_runs,omitempty"`
	HotMergeStates                        []hotGLRState `json:"hot_merge_states,omitempty"`
	HotEquivStates                        []hotGLRState `json:"hot_equiv_states,omitempty"`
}

type pathStats struct {
	SlicesAllocated   uint64 `json:"slices_allocated,omitempty"`
	SlicesRetained    uint64 `json:"slices_retained,omitempty"`
	SlicesDropped     uint64 `json:"slices_dropped,omitempty"`
	PointersAllocated uint64 `json:"pointers_allocated,omitempty"`
	PointersRetained  uint64 `json:"pointers_retained,omitempty"`
	PointersDropped   uint64 `json:"pointers_dropped,omitempty"`
}

type hotGLRState struct {
	State                                 uint32         `json:"state"`
	Lookahead                             uint16         `json:"lookahead,omitempty"`
	LookaheadName                         string         `json:"lookahead_name,omitempty"`
	ActionCount                           uint8          `json:"action_count,omitempty"`
	ShiftCount                            uint8          `json:"shift_count,omitempty"`
	ReduceCount                           uint8          `json:"reduce_count,omitempty"`
	ReduceSymbol                          uint16         `json:"reduce_symbol,omitempty"`
	ReduceSymbolName                      string         `json:"reduce_symbol_name,omitempty"`
	ChildCount                            uint8          `json:"child_count,omitempty"`
	ProductionID                          uint16         `json:"production_id,omitempty"`
	Hits                                  uint64         `json:"hits,omitempty"`
	Forks                                 uint64         `json:"forks,omitempty"`
	MultiStackHits                        uint64         `json:"multi_stack_hits,omitempty"`
	StackInTotal                          uint64         `json:"stack_in_total,omitempty"`
	StackInMax                            int            `json:"stack_in_max,omitempty"`
	ReduceChainHits                       uint64         `json:"reduce_chain_hits,omitempty"`
	ReduceChainSteps                      uint64         `json:"reduce_chain_steps,omitempty"`
	ReduceChainMaxLen                     int            `json:"reduce_chain_max_len,omitempty"`
	ReduceChainNS                         int64          `json:"reduce_chain_ns,omitempty"`
	ReduceChainRuns                       uint64         `json:"reduce_chain_runs,omitempty"`
	ReduceChainClassHits                  uint64         `json:"reduce_chain_class_hits,omitempty"`
	ReduceChainStopNoAction               uint64         `json:"reduce_chain_stop_no_action,omitempty"`
	ReduceChainStopMulti                  uint64         `json:"reduce_chain_stop_multi,omitempty"`
	ReduceChainStopShift                  uint64         `json:"reduce_chain_stop_shift,omitempty"`
	ReduceChainStopAccept                 uint64         `json:"reduce_chain_stop_accept,omitempty"`
	ReduceChainStopDead                   uint64         `json:"reduce_chain_stop_dead,omitempty"`
	ReduceChainStopCycle                  uint64         `json:"reduce_chain_stop_cycle,omitempty"`
	ReduceChainStopLimit                  uint64         `json:"reduce_chain_stop_limit,omitempty"`
	ReduceChainTerminalState              uint32         `json:"reduce_chain_terminal_state,omitempty"`
	ReduceChainTerminalActionClass        uint8          `json:"reduce_chain_terminal_action_class,omitempty"`
	ReduceChainTerminalActionName         string         `json:"reduce_chain_terminal_action_name,omitempty"`
	ActionNS                              int64          `json:"action_ns,omitempty"`
	ExtraShiftNS                          int64          `json:"extra_shift_ns,omitempty"`
	NoActionNS                            int64          `json:"no_action_ns,omitempty"`
	ConflictChoiceNS                      int64          `json:"conflict_choice_ns,omitempty"`
	ConflictForkNS                        int64          `json:"conflict_fork_ns,omitempty"`
	SingleShiftNS                         int64          `json:"single_shift_ns,omitempty"`
	SingleReduceNS                        int64          `json:"single_reduce_ns,omitempty"`
	SingleAcceptNS                        int64          `json:"single_accept_ns,omitempty"`
	SingleRecoverNS                       int64          `json:"single_recover_ns,omitempty"`
	SingleOtherNS                         int64          `json:"single_other_ns,omitempty"`
	MergeCalls                            uint64         `json:"merge_calls,omitempty"`
	MergeStacksIn                         uint64         `json:"merge_stacks_in,omitempty"`
	MergeStacksOut                        uint64         `json:"merge_stacks_out,omitempty"`
	MergeStacksInMax                      int            `json:"merge_stacks_in_max,omitempty"`
	MergeStacksOutMax                     int            `json:"merge_stacks_out_max,omitempty"`
	EquivCacheLookups                     uint64         `json:"equiv_cache_lookups,omitempty"`
	EquivCacheHits                        uint64         `json:"equiv_cache_hits,omitempty"`
	EquivCacheStores                      uint64         `json:"equiv_cache_stores,omitempty"`
	EquivCacheMisses                      uint64         `json:"equiv_cache_misses,omitempty"`
	EquivCacheTrueHits                    uint64         `json:"equiv_cache_true_hits,omitempty"`
	EquivCacheFalseHits                   uint64         `json:"equiv_cache_false_hits,omitempty"`
	EquivCacheKeyMisses                   uint64         `json:"equiv_cache_key_misses,omitempty"`
	EquivCacheEpochMisses                 uint64         `json:"equiv_cache_epoch_misses,omitempty"`
	EquivCacheVersionMisses               uint64         `json:"equiv_cache_version_misses,omitempty"`
	StackEquivCalls                       uint64         `json:"stack_equiv_calls,omitempty"`
	StackEquivTrue                        uint64         `json:"stack_equiv_true,omitempty"`
	StackEquivDepthMismatch               uint64         `json:"stack_equiv_depth_mismatch,omitempty"`
	StackEquivHashMismatch                uint64         `json:"stack_equiv_hash_mismatch,omitempty"`
	StackEquivStateMismatch               uint64         `json:"stack_equiv_state_mismatch,omitempty"`
	StackEquivPayloadMismatch             uint64         `json:"stack_equiv_payload_mismatch,omitempty"`
	StackEquivEntryCompares               uint64         `json:"stack_equiv_entry_compares,omitempty"`
	StackEquivStateMismatchDepthSum       uint64         `json:"stack_equiv_state_mismatch_depth_sum,omitempty"`
	StackEquivStateMismatchMaxDepth       uint32         `json:"stack_equiv_state_mismatch_max_depth,omitempty"`
	StackEquivStateMismatchDepthBuckets   []uint64       `json:"stack_equiv_state_mismatch_depth_buckets,omitempty"`
	StackEquivPayloadMismatchDepthSum     uint64         `json:"stack_equiv_payload_mismatch_depth_sum,omitempty"`
	StackEquivPayloadMismatchMaxDepth     uint32         `json:"stack_equiv_payload_mismatch_max_depth,omitempty"`
	StackEquivPayloadMismatchDepthBuckets []uint64       `json:"stack_equiv_payload_mismatch_depth_buckets,omitempty"`
	StackEquivPayloadHeaderSigDiff        uint64         `json:"stack_equiv_payload_header_sig_diff,omitempty"`
	StackEquivPayloadHeaderSigSame        uint64         `json:"stack_equiv_payload_header_sig_same,omitempty"`
	StackEquivPayloadShallowSigDiff       uint64         `json:"stack_equiv_payload_shallow_sig_diff,omitempty"`
	StackEquivPayloadShallowSigSame       uint64         `json:"stack_equiv_payload_shallow_sig_same,omitempty"`
	StackEquivPairKeyed                   uint64         `json:"stack_equiv_pair_keyed,omitempty"`
	StackEquivPairUnkeyed                 uint64         `json:"stack_equiv_pair_unkeyed,omitempty"`
	StackEquivPairRepeats                 uint64         `json:"stack_equiv_pair_repeats,omitempty"`
	StackEquivPairRepeatTrue              uint64         `json:"stack_equiv_pair_repeat_true,omitempty"`
	StackEquivPairRepeatFalse             uint64         `json:"stack_equiv_pair_repeat_false,omitempty"`
	StackEquivPairRepeatMismatch          uint64         `json:"stack_equiv_pair_repeat_mismatch,omitempty"`
	StackEquivPairStores                  uint64         `json:"stack_equiv_pair_stores,omitempty"`
	EquivSkipError                        uint64         `json:"equiv_skip_error,omitempty"`
	EquivSkipLeaf                         uint64         `json:"equiv_skip_leaf,omitempty"`
	EquivSkipFieldMismatch                uint64         `json:"equiv_skip_field_mismatch,omitempty"`
	EquivExactCalls                       uint64         `json:"equiv_exact_calls,omitempty"`
	EquivExactTrue                        uint64         `json:"equiv_exact_true,omitempty"`
	EquivExactPointerTrue                 uint64         `json:"equiv_exact_pointer_true,omitempty"`
	EquivExactNilMismatch                 uint64         `json:"equiv_exact_nil_mismatch,omitempty"`
	EquivExactHeaderMismatch              uint64         `json:"equiv_exact_header_mismatch,omitempty"`
	EquivExactChildMismatch               uint64         `json:"equiv_exact_child_mismatch,omitempty"`
	EquivExactTerminalCalls               uint64         `json:"equiv_exact_terminal_calls,omitempty"`
	EquivExactTerminalTrue                uint64         `json:"equiv_exact_terminal_true,omitempty"`
	EquivExactTerminalFalse               uint64         `json:"equiv_exact_terminal_false,omitempty"`
	EquivFrontierCalls                    uint64         `json:"equiv_frontier_calls,omitempty"`
	EquivFrontierTrue                     uint64         `json:"equiv_frontier_true,omitempty"`
	EquivExactChildCompares               uint64         `json:"equiv_exact_child_compares,omitempty"`
	EquivFrontierChildScans               uint64         `json:"equiv_frontier_child_scans,omitempty"`
	EquivFrontierCandidateCompares        uint64         `json:"equiv_frontier_candidate_compares,omitempty"`
	Actions                               []hotGLRAction `json:"actions,omitempty"`
}

type passStats struct {
	Name     string `json:"name,omitempty"`
	Checked  uint64 `json:"checked,omitempty"`
	Run      uint64 `json:"run,omitempty"`
	NS       int64  `json:"ns,omitempty"`
	Nodes    uint64 `json:"nodes_visited,omitempty"`
	Rewrites uint64 `json:"nodes_rewritten,omitempty"`
}

type hotGLRAction struct {
	Type              uint8  `json:"type"`
	TypeName          string `json:"type_name,omitempty"`
	State             uint32 `json:"state,omitempty"`
	Symbol            uint16 `json:"symbol,omitempty"`
	SymbolName        string `json:"symbol_name,omitempty"`
	ChildCount        uint8  `json:"child_count,omitempty"`
	DynamicPrecedence int16  `json:"dynamic_precedence,omitempty"`
	ProductionID      uint16 `json:"production_id,omitempty"`
	Extra             bool   `json:"extra,omitempty"`
	Repetition        bool   `json:"repetition,omitempty"`
}

type runMeasurement struct {
	durations []int64
	bytes     uint64
	allocs    uint64
	runtime   runtimeStats
	err       error
}

type queryCapture struct {
	Name      string
	Type      string
	Named     bool
	StartByte uint32
	EndByte   uint32
	Text      string
}

type queryMatch struct {
	PatternIndex int
	Captures     []queryCapture
}

type metadata struct {
	Schema              string            `json:"schema"`
	Repo                string            `json:"repo,omitempty"`
	Commit              string            `json:"commit,omitempty"`
	Branch              string            `json:"branch,omitempty"`
	Dirty               string            `json:"dirty,omitempty"`
	GoVersion           string            `json:"go_version"`
	DockerImage         string            `json:"docker_image,omitempty"`
	CPULimit            string            `json:"cpu_limit,omitempty"`
	MemoryLimit         string            `json:"memory_limit,omitempty"`
	Modes               []string          `json:"modes"`
	Languages           []string          `json:"languages"`
	Count               int               `json:"count"`
	GateOnly            bool              `json:"gate_only"`
	ArenaBreakdown      bool              `json:"arena_breakdown,omitempty"`
	HotShapeLimit       int               `json:"hot_shape_limit,omitempty"`
	EquivCounters       bool              `json:"equiv_counters,omitempty"`
	RuntimeAudit        bool              `json:"runtime_audit,omitempty"`
	ReduceTiming        bool              `json:"reduce_timing,omitempty"`
	ActionTiming        bool              `json:"action_timing,omitempty"`
	CorpusManifest      string            `json:"corpus_manifest,omitempty"`
	CorpusManifestSHA   string            `json:"corpus_manifest_sha256,omitempty"`
	QueryManifest       string            `json:"query_manifest,omitempty"`
	QueryManifestSHA    string            `json:"query_manifest_sha256,omitempty"`
	EditManifest        string            `json:"edit_manifest,omitempty"`
	EditManifestSHA     string            `json:"edit_manifest_sha256,omitempty"`
	Environment         map[string]string `json:"environment,omitempty"`
	GeneratedAtUTC      string            `json:"generated_at_utc"`
	TotalSamples        int               `json:"total_samples"`
	TotalRows           int               `json:"total_rows"`
	ParityFailures      int               `json:"parity_failures"`
	RequiredParityLangs []string          `json:"required_parity_languages,omitempty"`
	RequiredParityFails int               `json:"required_parity_failures,omitempty"`
	ModeFailures        int               `json:"mode_failures"`
	UnsupportedSamples  int               `json:"unsupported_samples"`
}

func main() {
	var (
		langsFlag         string
		modesFlag         string
		corpusFlag        string
		queryFlag         string
		editFlag          string
		outFlag           string
		repoRootFlag      string
		requireParityFlag string
		countFlag         int
		allowParityFail   bool
		timeParityFails   bool
		gateOnly          bool
		arenaBreakdown    bool
		phaseTiming       bool
		hotShapeLimit     int
		equivCounters     bool
		runtimeAudit      bool
		reduceTiming      bool
		actionTiming      bool
	)
	flag.StringVar(&langsFlag, "langs", "go,python,rust,java,c", "comma-separated languages to include")
	flag.StringVar(&modesFlag, "modes", "cgo_full,go_full,go_no_tree", "comma-separated modes")
	flag.StringVar(&corpusFlag, "corpus", "cgo_harness/corpus_manifest.json", "corpus manifest path")
	flag.StringVar(&queryFlag, "queries", "cgo_harness/query_manifest.json", "query manifest path")
	flag.StringVar(&editFlag, "edits", "cgo_harness/edit_fixtures.json", "edit fixture manifest path")
	flag.StringVar(&outFlag, "out", "harness_out/parse_gap/latest", "output directory")
	flag.StringVar(&repoRootFlag, "repo-root", "", "repository root; autodetected when empty")
	flag.StringVar(&requireParityFlag, "require-parity-langs", "", "comma-separated languages that must pass parse/highlight/query parity even when --allow-parity-fail is set")
	flag.IntVar(&countFlag, "count", 10, "iterations per sample/mode")
	flag.BoolVar(&allowParityFail, "allow-parity-fail", false, "write parity failures but exit zero")
	flag.BoolVar(&timeParityFails, "time-parity-failures", false, "run timing modes even when correctness gates fail")
	flag.BoolVar(&gateOnly, "gate-only", false, "run only parse/highlight/query correctness gates and skip timing modes")
	flag.BoolVar(&arenaBreakdown, "arena-breakdown", false, "enable detailed gotreesitter arena breakdown while measuring")
	flag.BoolVar(&phaseTiming, "phase-timing", false, "enable gotreesitter parser phase timing while measuring")
	flag.IntVar(&hotShapeLimit, "hot-shapes", 0, "include top-N GLR fork/reduce/merge hot-shape rows in runtime JSON")
	flag.BoolVar(&equivCounters, "equiv-counters", false, "enable lightweight GLR equivalence attribution counters")
	flag.BoolVar(&runtimeAudit, "runtime-audit", false, "enable heavyweight survivor/runtime audit counters while measuring")
	flag.BoolVar(&reduceTiming, "reduce-timing", false, "enable reduce subphase timing while measuring; implies --phase-timing")
	flag.BoolVar(&actionTiming, "action-timing", false, "enable action-dispatch subphase timing while measuring; implies --phase-timing")
	flag.Parse()

	if countFlag <= 0 {
		fatalf("--count must be > 0")
	}
	repoRoot, err := resolveRepoRoot(repoRootFlag)
	if err != nil {
		fatalf("resolve repo root: %v", err)
	}
	langs := splitCSV(langsFlag)
	if len(langs) == 0 {
		fatalf("no languages selected")
	}
	requiredParityLangs := splitCSV(requireParityFlag)
	requiredParity := make(map[string]struct{}, len(requiredParityLangs))
	for _, lang := range requiredParityLangs {
		requiredParity[lang] = struct{}{}
	}
	modes := splitCSV(modesFlag)
	if len(modes) == 0 && !gateOnly {
		fatalf("no modes selected")
	}
	outDir := resolvePath(repoRoot, outFlag)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fatalf("create output dir: %v", err)
	}

	corpusPath := resolvePath(repoRoot, corpusFlag)
	corpus, err := loadJSONFile[corpusManifest](corpusPath)
	if err != nil {
		fatalf("load corpus manifest: %v", err)
	}
	queryPath := resolvePath(repoRoot, queryFlag)
	queries, queryErr := loadJSONFile[queryManifest](queryPath)
	if queryErr != nil && strings.TrimSpace(queryFlag) != "" {
		fatalf("load query manifest: %v", queryErr)
	}
	editPath := resolvePath(repoRoot, editFlag)
	edits, editErr := loadJSONFile[editManifest](editPath)
	if editErr != nil && strings.TrimSpace(editFlag) != "" {
		fatalf("load edit manifest: %v", editErr)
	}

	selected := map[string]struct{}{}
	for _, lang := range langs {
		selected[lang] = struct{}{}
	}
	samples, err := collectSamples(repoRoot, corpus, selected)
	if err != nil {
		fatalf("collect corpus: %v", err)
	}
	if len(samples) == 0 {
		fatalf("zero corpus samples for languages %s", strings.Join(langs, ","))
	}

	entries, support := registryMaps()
	runners := make(map[string]*runner)
	defer func() {
		for _, r := range runners {
			r.close()
		}
	}()
	queryByLang := queries.byLanguage()
	editByLang := edits.byLanguage()

	gotreesitter.EnableArenaBreakdown(arenaBreakdown)
	defer gotreesitter.EnableArenaBreakdown(false)
	gotreesitter.EnableRuntimeAudit(runtimeAudit)
	defer gotreesitter.EnableRuntimeAudit(false)
	gotreesitter.EnableGLREquivAudit(equivCounters)
	defer gotreesitter.EnableGLREquivAudit(false)
	if reduceTiming || actionTiming {
		phaseTiming = true
	}
	if phaseTiming {
		if err := os.Setenv("GOT_PARSE_PHASE_TIMING", "1"); err != nil {
			fatalf("enable phase timing: %v", err)
		}
		if reduceTiming {
			if err := os.Setenv("GOT_PARSE_REDUCE_TIMING", "1"); err != nil {
				fatalf("enable reduce timing: %v", err)
			}
		}
		if actionTiming {
			if err := os.Setenv("GOT_PARSE_ACTION_TIMING", "1"); err != nil {
				fatalf("enable action timing: %v", err)
			}
		}
		gotreesitter.ResetParseEnvConfigCacheForTests()
	}

	common := commonRowFields(repoRoot)
	resultsPath := filepath.Join(outDir, "results.jsonl")
	resultsFile, err := os.Create(resultsPath)
	if err != nil {
		fatalf("create results: %v", err)
	}
	defer resultsFile.Close()
	writer := bufio.NewWriter(resultsFile)
	enc := json.NewEncoder(writer)
	enc.SetEscapeHTML(false)

	var rows []reportRow
	parityFailures := 0
	requiredParityFailures := 0
	modeFailures := 0
	unsupportedSamples := 0

	for _, s := range samples {
		r, err := runnerForLanguage(s.Language, entries, support, runners, hotShapeLimit)
		if err != nil {
			unsupportedSamples++
			row := errorRow(common, s, "setup", countFlag, err)
			rows = append(rows, row)
			_ = enc.Encode(row)
			continue
		}
		parity := computeParity(r, s.Source, queryByLang[s.Language])
		if gateOnly {
			if parity.Error != "" {
				parityFailures++
				if _, ok := requiredParity[s.Language]; ok {
					requiredParityFailures++
				}
			}
			row := gateRow(common, s, countFlag, parity)
			rows = append(rows, row)
			if err := enc.Encode(row); err != nil {
				fatalf("write results: %v", err)
			}
			continue
		}
		if parity.Error != "" {
			parityFailures++
			if _, ok := requiredParity[s.Language]; ok {
				requiredParityFailures++
			}
			row := gateRow(common, s, countFlag, parity)
			rows = append(rows, row)
			if err := enc.Encode(row); err != nil {
				fatalf("write results: %v", err)
			}
			if !timeParityFails {
				continue
			}
		}
		for _, mode := range modes {
			measurement := measureMode(r, mode, s.Source, countFlag, queryByLang[s.Language], editByLang[s.Language])
			row := buildRow(common, s, mode, countFlag, parity, measurement)
			row.Blocker = classify(row)
			if measurement.err != nil {
				modeFailures++
			}
			rows = append(rows, row)
			if err := enc.Encode(row); err != nil {
				fatalf("write results: %v", err)
			}
		}
	}
	if err := writer.Flush(); err != nil {
		fatalf("flush results: %v", err)
	}
	if err := resultsFile.Close(); err != nil {
		fatalf("close results: %v", err)
	}

	meta := metadata{
		Schema:              "parse-gap-metadata-v1",
		Repo:                common.Repo,
		Commit:              common.Commit,
		Branch:              common.Branch,
		Dirty:               gitOutput(repoRoot, "status", "--short"),
		GoVersion:           runtime.Version(),
		DockerImage:         common.DockerImage,
		CPULimit:            common.CPULimit,
		MemoryLimit:         common.MemoryLimit,
		Modes:               modes,
		Languages:           langs,
		Count:               countFlag,
		GateOnly:            gateOnly,
		ArenaBreakdown:      arenaBreakdown,
		HotShapeLimit:       hotShapeLimit,
		EquivCounters:       equivCounters,
		RuntimeAudit:        runtimeAudit,
		ReduceTiming:        reduceTiming,
		ActionTiming:        actionTiming,
		CorpusManifest:      relOrAbs(repoRoot, corpusPath),
		CorpusManifestSHA:   sha256File(corpusPath),
		QueryManifest:       relOrAbs(repoRoot, queryPath),
		QueryManifestSHA:    sha256File(queryPath),
		EditManifest:        relOrAbs(repoRoot, editPath),
		EditManifestSHA:     sha256File(editPath),
		Environment:         captureEnvironment(),
		GeneratedAtUTC:      time.Now().UTC().Format(time.RFC3339),
		TotalSamples:        len(samples),
		TotalRows:           len(rows),
		ParityFailures:      parityFailures,
		RequiredParityLangs: requiredParityLangs,
		RequiredParityFails: requiredParityFailures,
		ModeFailures:        modeFailures,
		UnsupportedSamples:  unsupportedSamples,
	}
	if err := writeJSON(filepath.Join(outDir, "metadata.json"), meta); err != nil {
		fatalf("write metadata: %v", err)
	}
	summary := renderSummary(rows)
	if err := os.WriteFile(filepath.Join(outDir, "summary.md"), []byte(summary), 0o644); err != nil {
		fatalf("write summary: %v", err)
	}
	fmt.Print(summary)
	fmt.Printf("\nresults: %s\n", resultsPath)

	if requiredParityFailures > 0 {
		fatalf("%d required parity failure(s) for language(s): %s", requiredParityFailures, strings.Join(requiredParityLangs, ","))
	}
	if parityFailures > 0 && !allowParityFail {
		fatalf("%d parity failure(s); rerun with --allow-parity-fail to keep exit zero", parityFailures)
	}
	if modeFailures > 0 {
		fatalf("%d mode failure(s)", modeFailures)
	}
}

type commonFields struct {
	Repo        string
	Commit      string
	Branch      string
	GoVersion   string
	DockerImage string
	CPULimit    string
	MemoryLimit string
}

func commonRowFields(repoRoot string) commonFields {
	return commonFields{
		Repo:        "odvcencio/gotreesitter",
		Commit:      gitOutput(repoRoot, "rev-parse", "HEAD"),
		Branch:      gitOutput(repoRoot, "rev-parse", "--abbrev-ref", "HEAD"),
		GoVersion:   runtime.Version(),
		DockerImage: getenvDefault("GTS_PARSE_GAP_DOCKER_IMAGE", ""),
		CPULimit:    firstNonEmpty(os.Getenv("GTS_PARSE_GAP_CPUS"), os.Getenv("CPUS")),
		MemoryLimit: firstNonEmpty(os.Getenv("GTS_PARSE_GAP_MEMORY"), os.Getenv("MEMORY")),
	}
}

func registryMaps() (map[string]grammars.LangEntry, map[string]grammars.ParseSupport) {
	entries := make(map[string]grammars.LangEntry)
	for _, entry := range grammars.AllLanguages() {
		entries[entry.Name] = entry
	}
	support := make(map[string]grammars.ParseSupport)
	for _, report := range grammars.AuditParseSupport() {
		support[report.Name] = report
	}
	return entries, support
}

func runnerForLanguage(name string, entries map[string]grammars.LangEntry, support map[string]grammars.ParseSupport, cache map[string]*runner, hotShapeLimit int) (*runner, error) {
	if r := cache[name]; r != nil {
		r.setHotShapeLimit(hotShapeLimit)
		return r, nil
	}
	entry, ok := entries[name]
	if !ok {
		return nil, fmt.Errorf("language %q is not in grammars registry", name)
	}
	report, ok := support[name]
	if !ok || report.Backend == grammars.ParseBackendUnsupported {
		return nil, fmt.Errorf("language %q parse backend is unsupported", name)
	}
	goLang := entry.Language()
	cLang, err := cgoharness.ParityCLanguage(name)
	if err != nil {
		return nil, fmt.Errorf("load C oracle language: %w", err)
	}
	cParser := sitter.NewParser()
	if err := cParser.SetLanguage(cLang); err != nil {
		cParser.Close()
		return nil, fmt.Errorf("set C language: %w", err)
	}
	r := &runner{
		name:     name,
		entry:    entry,
		support:  report,
		goLang:   goLang,
		goParser: gotreesitter.NewParser(goLang),
		cLang:    cLang,
		c:        cParser,
	}
	r.setHotShapeLimit(hotShapeLimit)
	cache[name] = r
	return r, nil
}

func (r *runner) setHotShapeLimit(limit int) {
	if r == nil {
		return
	}
	r.hotShapeLimit = limit
	if limit <= 0 {
		r.profile = nil
		r.goParser.SetAmbiguityProfile(nil)
		return
	}
	if r.profile == nil {
		r.profile = gotreesitter.NewAmbiguityProfile()
	}
	r.goParser.SetAmbiguityProfile(r.profile)
}

func (r *runner) resetMeasurementDiagnostics() {
	gotreesitter.ResetPerfCounters()
	if r != nil && r.profile != nil {
		r.profile.Reset()
	}
}

func (r *runner) close() {
	if r != nil && r.c != nil {
		r.c.Close()
	}
}

func computeParity(r *runner, source []byte, queries []querySpec) paritySummary {
	cTree := r.c.Parse(source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		return paritySummary{Error: "c_parse: C parser returned nil tree"}
	}
	defer cTree.Close()

	goTree, err := parseGo(r, source)
	if err != nil {
		return paritySummary{Error: "go_parse: " + err.Error()}
	}
	if goTree == nil || goTree.RootNode() == nil {
		return paritySummary{Error: "go_parse: gotreesitter returned nil tree"}
	}
	defer goTree.Release()

	cRoot := cTree.RootNode()
	goRoot := goTree.RootNode()
	noError := !cRoot.HasError() && !goRoot.HasError()
	diff := cgoharness.FirstDivergenceDumpV1(goRoot, r.goLang, cRoot)
	deep := diff == nil
	summary := paritySummary{
		NoError: noError,
		SExpr:   deep,
		Deep:    deep,
	}
	if !noError {
		if goRoot.HasError() != cRoot.HasError() {
			summary.Error = fmt.Sprintf("root_error: go=%v c=%v", goRoot.HasError(), cRoot.HasError())
		}
	}
	if diff != nil {
		summary.Error = fmt.Sprintf("deep_%s at %s: go=%s c=%s", diff.Category, diff.Path, diff.GoValue, diff.CValue)
	}
	if summary.Error != "" {
		return summary
	}
	if strings.TrimSpace(r.entry.HighlightQuery) != "" {
		ok, err := highlightParity(r, goTree, cTree, source, r.entry.HighlightQuery)
		summary.Highlight = &ok
		if err != nil {
			summary.Error = "highlight: " + err.Error()
		} else if !ok {
			summary.Error = "highlight: capture mismatch"
		}
	}
	if len(queries) > 0 {
		ok, err := queryParity(r, goTree, cTree, source, queries[0].Query)
		summary.Query = &ok
		if err != nil {
			summary.Error = "query: " + err.Error()
		} else if !ok && summary.Error == "" {
			summary.Error = "query: capture mismatch"
		}
	}
	return summary
}

func parseGo(r *runner, source []byte) (*gotreesitter.Tree, error) {
	switch r.support.Backend {
	case grammars.ParseBackendTokenSource:
		if r.entry.TokenSourceFactory == nil {
			return nil, fmt.Errorf("token_source backend without factory")
		}
		return r.goParser.ParseWithTokenSource(source, r.entry.TokenSourceFactory(source, r.goLang))
	case grammars.ParseBackendDFA, grammars.ParseBackendDFAPartial:
		return r.goParser.Parse(source)
	default:
		return nil, fmt.Errorf("unsupported backend %q", r.support.Backend)
	}
}

func parseGoIncremental(r *runner, source []byte, oldTree *gotreesitter.Tree) (*gotreesitter.Tree, gotreesitter.IncrementalParseProfile, bool, error) {
	switch r.support.Backend {
	case grammars.ParseBackendTokenSource:
		if r.entry.TokenSourceFactory == nil {
			return nil, gotreesitter.IncrementalParseProfile{}, false, fmt.Errorf("token_source backend without factory")
		}
		tree, profile, err := r.goParser.ParseIncrementalWithTokenSourceProfiled(source, oldTree, r.entry.TokenSourceFactory(source, r.goLang))
		return tree, profile, true, err
	case grammars.ParseBackendDFA, grammars.ParseBackendDFAPartial:
		tree, profile, err := r.goParser.ParseIncrementalProfiled(source, oldTree)
		return tree, profile, true, err
	default:
		return nil, gotreesitter.IncrementalParseProfile{}, false, fmt.Errorf("unsupported backend %q", r.support.Backend)
	}
}

func measureMode(r *runner, mode string, source []byte, count int, queries []querySpec, edits []editSpec) runMeasurement {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	var last runtimeStats
	durations := make([]int64, 0, count)
	for i := 0; i < count; i++ {
		r.resetMeasurementDiagnostics()
		start := time.Now()
		stats, err := runModeOnce(r, mode, source, queries, edits)
		elapsed := time.Since(start).Nanoseconds()
		if err != nil {
			runtime.ReadMemStats(&after)
			return runMeasurement{
				durations: durations,
				bytes:     after.TotalAlloc - before.TotalAlloc,
				allocs:    after.Mallocs - before.Mallocs,
				runtime:   last,
				err:       err,
			}
		}
		durations = append(durations, elapsed)
		last = stats
	}
	runtime.ReadMemStats(&after)
	return runMeasurement{
		durations: durations,
		bytes:     after.TotalAlloc - before.TotalAlloc,
		allocs:    after.Mallocs - before.Mallocs,
		runtime:   last,
	}
}

func runModeOnce(r *runner, mode string, source []byte, queries []querySpec, edits []editSpec) (runtimeStats, error) {
	_ = edits
	switch mode {
	case "cgo_full":
		tree := r.c.Parse(source, nil)
		if tree == nil || tree.RootNode() == nil {
			return runtimeStats{}, fmt.Errorf("C parser returned nil tree")
		}
		tree.Close()
		return runtimeStats{}, nil
	case "go_full":
		tree, err := parseGo(r, source)
		return statsFromGoTree(r, tree, 0, 0), releaseTree(tree, err)
	case "go_no_compat":
		tree, err := r.goParser.ParseNoResultCompatibilityBenchmarkOnly(source)
		return statsFromGoTree(r, tree, 0, 0), releaseTree(tree, err)
	case "go_no_tree":
		tree, err := r.goParser.ParseNoTreeBenchmarkOnly(source)
		return statsFromGoTree(r, tree, 0, 0), releaseTree(tree, err)
	case "go_parse_query":
		if len(queries) == 0 {
			return runtimeStats{}, fmt.Errorf("no query manifest entry for %s", r.name)
		}
		tree, err := parseGo(r, source)
		if err != nil {
			releaseTree(tree, nil)
			return runtimeStats{}, err
		}
		captures, qTiming, qErr := runGoQuery(r, tree, source, queries[0].Query)
		stats := statsFromGoTree(r, tree, captures, 0, qTiming)
		return stats, releaseTree(tree, qErr)
	case "go_cursor_walk":
		tree, err := parseGo(r, source)
		if err != nil {
			return statsFromGoTree(r, tree, 0, 0), releaseTree(tree, err)
		}
		nodes := walkCursor(tree)
		stats := statsFromGoTree(r, tree, 0, nodes)
		return stats, releaseTree(tree, nil)
	case "go_sexpr":
		tree, err := parseGo(r, source)
		if err != nil {
			return statsFromGoTree(r, tree, 0, 0), releaseTree(tree, err)
		}
		_ = tree.RootNode().SExpr(r.goLang)
		return statsFromGoTree(r, tree, 0, 0), releaseTree(tree, nil)
	case "go_parent_sibling":
		tree, err := parseGo(r, source)
		if err != nil {
			return statsFromGoTree(r, tree, 0, 0), releaseTree(tree, err)
		}
		touchParentSibling(tree.RootNode())
		return statsFromGoTree(r, tree, 0, 0), releaseTree(tree, nil)
	case "go_edit":
		stats, err := runGoEdit(r, source, false)
		return stats, err
	case "go_noop_incremental":
		stats, err := runGoEdit(r, source, true)
		return stats, err
	default:
		return runtimeStats{}, fmt.Errorf("unsupported mode %q", mode)
	}
}

func releaseTree(tree *gotreesitter.Tree, err error) error {
	if tree != nil {
		tree.Release()
	}
	return err
}

type queryTimingStats struct {
	compileNS int64
	execNS    int64
	rootNS    int64
	cursorNS  int64
}

func statsFromGoTree(r *runner, tree *gotreesitter.Tree, queryCaptures, cursorNodes uint64, queryTiming ...queryTimingStats) runtimeStats {
	if tree == nil {
		return runtimeStats{}
	}
	rt := tree.ParseRuntime()
	stats := statsFromRuntime(rt)
	stats.QueryCaptures = queryCaptures
	if len(queryTiming) > 0 {
		stats.QueryCompileNS = queryTiming[0].compileNS
		stats.QueryExecNS = queryTiming[0].execNS
		stats.QueryRootNS = queryTiming[0].rootNS
		stats.QueryCursorNS = queryTiming[0].cursorNS
	}
	stats.CursorNodes = cursorNodes
	if breakdown, ok := tree.ArenaBreakdown(); ok {
		stats.ArenaLiveB = breakdown.NodeStructBytesAllocated +
			breakdown.NoTreeNodeBytesAllocated +
			breakdown.CompactFullLeafBytesAllocated +
			breakdown.PendingParentBytesAllocated +
			breakdown.PendingChildEntryBytesAllocated +
			breakdown.FinalChildSidecarBytesAllocated +
			breakdown.ChildSliceBytesAllocated +
			breakdown.FieldIDBytesAllocated +
			breakdown.FieldSourceBytesAllocated
		stats.ArenaCapacityWaste = breakdown.NodeCapacityWaste
	}
	perf := gotreesitter.PerfCountersSnapshot()
	stats.DenseFallbacks = perf.DenseMutationDrains
	stats.CloneTreePublicNodes = perf.CloneTreePublicNodes
	stats.CloneOffsetPublicNodes = perf.CloneOffsetPublicNodes
	stats.NodeEditCompactRefs = perf.NodeEditCompactRefs
	stats.NodeEditPublicFallbacks = subUint64(perf.NodeEditMarked, perf.NodeEditCompactRefs)
	stats.MutationChildRefCOW = perf.MutationChildRefCOW
	stats.MergeCalls = perf.MergeCalls
	stats.MergeDeadPruned = perf.MergeDeadPruned
	stats.MergeReplacements = perf.MergeReplacements
	stats.StackEquivalentCalls = perf.StackEquivalentCalls
	stats.StackEquivalentTrue = perf.StackEquivalentTrue
	stats.StackCompareCalls = perf.StackCompareCalls
	stats.ForkCount = perf.ForkCount
	stats.ConflictRR = perf.ConflictRR
	stats.ConflictRS = perf.ConflictRS
	stats.ConflictOther = perf.ConflictOther
	stats.LexBytes = perf.LexBytes
	stats.LexTokens = perf.LexTokens
	stats.ReduceChainSteps = perf.ReduceChainSteps
	stats.ReduceChainMaxLen = perf.ReduceChainMaxLen
	stats.ReduceChainHintCandidates = perf.ReduceChainHintCandidates
	stats.ReduceChainHintTaken = perf.ReduceChainHintTaken
	stats.ReduceChainHintSteps = perf.ReduceChainHintSteps
	stats.ReduceChainHintTerminalOK = perf.ReduceChainHintTerminalOK
	stats.ReduceChainHintTerminalMismatch = perf.ReduceChainHintTerminalMismatch
	stats.ReduceChainHintLimit = perf.ReduceChainHintLimit
	stats.ReduceChainHintDead = perf.ReduceChainHintDead
	stats.ReduceChainHintUnexpected = perf.ReduceChainHintUnexpected
	stats.ParentChildPointers = perf.ParentChildPointers
	stats.ReduceChildrenFastGSS = perf.ReduceChildrenFastGSS
	stats.ReduceChildrenAllVisible = perf.ReduceChildrenAllVis
	stats.ReduceChildrenNoAlias = perf.ReduceChildrenNoAlias
	stats.ReduceChildrenScratch = perf.ReduceChildrenScratch
	stats.ReduceScratchNoAlias = perf.ReduceScratchNoAlias
	stats.ReduceScratchGeneral = perf.ReduceScratchGeneral
	if r != nil && r.profile != nil && r.hotShapeLimit > 0 {
		chainTotals := r.profile.SnapshotReduceChainTotals()
		stats.ReduceChainClassHits = chainTotals.ReduceChainClassHits
		stats.ReduceChainStopNoAction = chainTotals.ReduceChainStopNoAction
		stats.ReduceChainStopMulti = chainTotals.ReduceChainStopMulti
		stats.ReduceChainStopShift = chainTotals.ReduceChainStopShift
		stats.ReduceChainStopAccept = chainTotals.ReduceChainStopAccept
		stats.ReduceChainStopDead = chainTotals.ReduceChainStopDead
		stats.ReduceChainStopCycle = chainTotals.ReduceChainStopCycle
		stats.ReduceChainStopLimit = chainTotals.ReduceChainStopLimit
		stats.HotAmbiguities = hotGLRStatesFromProfile(r.goLang, r.profile.SnapshotTop(r.hotShapeLimit))
		stats.HotReduceChains = hotGLRStatesFromProfile(r.goLang, r.profile.SnapshotTopReduceChains(r.hotShapeLimit))
		stats.HotReduceChainRuns = hotGLRStatesFromProfile(r.goLang, r.profile.SnapshotTopReduceChainRuns(r.hotShapeLimit))
		stats.HotMergeStates = hotGLRStatesFromProfile(r.goLang, r.profile.SnapshotTopMergeStates(r.hotShapeLimit))
	}
	if r != nil && r.hotShapeLimit > 0 {
		stats.HotEquivStates = hotEquivStatesFromRuntime(rt.EquivStateStats, r.hotShapeLimit)
	}
	return stats
}

func hotGLRStatesFromProfile(lang *gotreesitter.Language, stats []gotreesitter.AmbiguityStat) []hotGLRState {
	if len(stats) == 0 {
		return nil
	}
	out := make([]hotGLRState, 0, len(stats))
	for _, stat := range stats {
		row := hotGLRState{
			State:                          uint32(stat.State),
			Lookahead:                      uint16(stat.Lookahead),
			LookaheadName:                  symbolName(lang, stat.Lookahead),
			ActionCount:                    stat.ActionCount,
			ShiftCount:                     stat.ShiftCount,
			ReduceCount:                    stat.ReduceCount,
			ReduceSymbol:                   uint16(stat.ReduceSymbol),
			ChildCount:                     stat.ChildCount,
			ProductionID:                   stat.ProductionID,
			Hits:                           stat.Hits,
			Forks:                          stat.Forks,
			MultiStackHits:                 stat.MultiStackHits,
			StackInTotal:                   stat.StackInTotal,
			StackInMax:                     stat.StackInMax,
			ReduceChainHits:                stat.ReduceChainHits,
			ReduceChainSteps:               stat.ReduceChainSteps,
			ReduceChainMaxLen:              stat.ReduceChainMaxLen,
			ReduceChainNS:                  stat.ReduceChainNanos,
			ReduceChainRuns:                stat.ReduceChainRuns,
			ReduceChainClassHits:           stat.ReduceChainClassHits,
			ReduceChainStopNoAction:        stat.ReduceChainStopNoAction,
			ReduceChainStopMulti:           stat.ReduceChainStopMulti,
			ReduceChainStopShift:           stat.ReduceChainStopShift,
			ReduceChainStopAccept:          stat.ReduceChainStopAccept,
			ReduceChainStopDead:            stat.ReduceChainStopDead,
			ReduceChainStopCycle:           stat.ReduceChainStopCycle,
			ReduceChainStopLimit:           stat.ReduceChainStopLimit,
			ReduceChainTerminalState:       uint32(stat.ReduceChainTerminalState),
			ReduceChainTerminalActionClass: stat.ReduceChainTerminalActionClass,
			ReduceChainTerminalActionName:  classifiedActionClassName(stat.ReduceChainTerminalActionClass),
			ActionNS:                       stat.ActionNanos,
			ExtraShiftNS:                   stat.ExtraShiftNanos,
			NoActionNS:                     stat.NoActionNanos,
			ConflictChoiceNS:               stat.ConflictChoiceNanos,
			ConflictForkNS:                 stat.ConflictForkNanos,
			SingleShiftNS:                  stat.SingleShiftNanos,
			SingleReduceNS:                 stat.SingleReduceNanos,
			SingleAcceptNS:                 stat.SingleAcceptNanos,
			SingleRecoverNS:                stat.SingleRecoverNanos,
			SingleOtherNS:                  stat.SingleOtherNanos,
			MergeCalls:                     stat.MergeCalls,
			MergeStacksIn:                  stat.MergeStacksIn,
			MergeStacksOut:                 stat.MergeStacksOut,
			MergeStacksInMax:               stat.MergeStacksInMax,
			MergeStacksOutMax:              stat.MergeStacksOutMax,
		}
		if stat.ReduceSymbol != 0 || stat.ChildCount != 0 || stat.ProductionID != 0 {
			row.ReduceSymbolName = symbolName(lang, stat.ReduceSymbol)
		}
		if len(stat.Actions) > 0 {
			row.Actions = make([]hotGLRAction, 0, len(stat.Actions))
			for _, action := range stat.Actions {
				row.Actions = append(row.Actions, hotGLRAction{
					Type:              uint8(action.Type),
					TypeName:          parseActionTypeName(action.Type),
					State:             uint32(action.State),
					Symbol:            uint16(action.Symbol),
					SymbolName:        symbolName(lang, action.Symbol),
					ChildCount:        action.ChildCount,
					DynamicPrecedence: action.DynamicPrecedence,
					ProductionID:      action.ProductionID,
					Extra:             action.Extra,
					Repetition:        action.Repetition,
				})
			}
		}
		out = append(out, row)
	}
	return out
}

func symbolName(lang *gotreesitter.Language, sym gotreesitter.Symbol) string {
	if lang == nil {
		return ""
	}
	idx := int(sym)
	if idx < 0 || idx >= len(lang.SymbolNames) {
		return ""
	}
	return lang.SymbolNames[idx]
}

func parseActionTypeName(t gotreesitter.ParseActionType) string {
	switch t {
	case gotreesitter.ParseActionShift:
		return "shift"
	case gotreesitter.ParseActionReduce:
		return "reduce"
	case gotreesitter.ParseActionAccept:
		return "accept"
	case gotreesitter.ParseActionRecover:
		return "recover"
	default:
		return fmt.Sprintf("action_%d", t)
	}
}

func classifiedActionClassName(class uint8) string {
	switch class {
	case 0:
		return "no_action"
	case 1:
		return "single_reduce"
	case 2:
		return "single_shift"
	case 3:
		return "single_accept"
	case 4:
		return "single_other"
	case 5:
		return "multi"
	default:
		return fmt.Sprintf("class_%d", class)
	}
}

func hotEquivStatesFromRuntime(stats []gotreesitter.ParseEquivStateRuntime, limit int) []hotGLRState {
	if len(stats) == 0 || limit == 0 {
		return nil
	}
	sort.Slice(stats, func(i, j int) bool {
		di := stats[i].StackEquivCalls + stats[i].StackEquivEntryCompares + stats[i].StackEquivPairRepeats + stats[i].EquivCacheLookups + stats[i].EquivExactCalls + stats[i].EquivFrontierCalls
		dj := stats[j].StackEquivCalls + stats[j].StackEquivEntryCompares + stats[j].StackEquivPairRepeats + stats[j].EquivCacheLookups + stats[j].EquivExactCalls + stats[j].EquivFrontierCalls
		if di == dj {
			return stats[i].State < stats[j].State
		}
		return di > dj
	})
	if limit > 0 && len(stats) > limit {
		stats = stats[:limit]
	}
	out := make([]hotGLRState, 0, len(stats))
	for _, stat := range stats {
		out = append(out, hotGLRState{
			State:                                 uint32(stat.State),
			StackEquivCalls:                       stat.StackEquivCalls,
			StackEquivTrue:                        stat.StackEquivTrue,
			StackEquivDepthMismatch:               stat.StackEquivDepthMismatch,
			StackEquivHashMismatch:                stat.StackEquivHashMismatch,
			StackEquivStateMismatch:               stat.StackEquivStateMismatch,
			StackEquivPayloadMismatch:             stat.StackEquivPayloadMismatch,
			StackEquivEntryCompares:               stat.StackEquivEntryCompares,
			StackEquivStateMismatchDepthSum:       stat.StackEquivStateMismatchDepthSum,
			StackEquivStateMismatchMaxDepth:       stat.StackEquivStateMismatchMaxDepth,
			StackEquivStateMismatchDepthBuckets:   nonzeroBuckets(stat.StackEquivStateMismatchDepthBuckets),
			StackEquivPayloadMismatchDepthSum:     stat.StackEquivPayloadMismatchDepthSum,
			StackEquivPayloadMismatchMaxDepth:     stat.StackEquivPayloadMismatchMaxDepth,
			StackEquivPayloadMismatchDepthBuckets: nonzeroBuckets(stat.StackEquivPayloadMismatchDepthBuckets),
			StackEquivPayloadHeaderSigDiff:        stat.StackEquivPayloadHeaderSigDiff,
			StackEquivPayloadHeaderSigSame:        stat.StackEquivPayloadHeaderSigSame,
			StackEquivPayloadShallowSigDiff:       stat.StackEquivPayloadShallowSigDiff,
			StackEquivPayloadShallowSigSame:       stat.StackEquivPayloadShallowSigSame,
			StackEquivPairKeyed:                   stat.StackEquivPairKeyed,
			StackEquivPairUnkeyed:                 stat.StackEquivPairUnkeyed,
			StackEquivPairRepeats:                 stat.StackEquivPairRepeats,
			StackEquivPairRepeatTrue:              stat.StackEquivPairRepeatTrue,
			StackEquivPairRepeatFalse:             stat.StackEquivPairRepeatFalse,
			StackEquivPairRepeatMismatch:          stat.StackEquivPairRepeatMismatch,
			StackEquivPairStores:                  stat.StackEquivPairStores,
			EquivCacheLookups:                     stat.EquivCacheLookups,
			EquivCacheHits:                        stat.EquivCacheHits,
			EquivCacheStores:                      stat.EquivCacheStores,
			EquivCacheMisses:                      stat.EquivCacheMisses,
			EquivCacheTrueHits:                    stat.EquivCacheTrueHits,
			EquivCacheFalseHits:                   stat.EquivCacheFalseHits,
			EquivCacheEpochMisses:                 stat.EquivCacheEpochMisses,
			EquivCacheKeyMisses:                   stat.EquivCacheKeyMisses,
			EquivCacheVersionMisses:               stat.EquivCacheVersionMisses,
			EquivSkipError:                        stat.EquivSkipError,
			EquivSkipLeaf:                         stat.EquivSkipLeaf,
			EquivSkipFieldMismatch:                stat.EquivSkipFieldMismatch,
			EquivExactCalls:                       stat.EquivExactCalls,
			EquivExactTrue:                        stat.EquivExactTrue,
			EquivExactPointerTrue:                 stat.EquivExactPointerTrue,
			EquivExactNilMismatch:                 stat.EquivExactNilMismatch,
			EquivExactHeaderMismatch:              stat.EquivExactHeaderMismatch,
			EquivExactChildMismatch:               stat.EquivExactChildMismatch,
			EquivExactTerminalCalls:               stat.EquivExactTerminalCalls,
			EquivExactTerminalTrue:                stat.EquivExactTerminalTrue,
			EquivExactTerminalFalse:               stat.EquivExactTerminalFalse,
			EquivFrontierCalls:                    stat.EquivFrontierCalls,
			EquivFrontierTrue:                     stat.EquivFrontierTrue,
			EquivExactChildCompares:               stat.EquivExactChildCompares,
			EquivFrontierChildScans:               stat.EquivFrontierChildScans,
			EquivFrontierCandidateCompares:        stat.EquivFrontierCandidateCompares,
		})
	}
	return out
}

func subUint64(a, b uint64) uint64 {
	if a < b {
		return 0
	}
	return a - b
}

func nonzeroBuckets(buckets [8]uint64) []uint64 {
	last := -1
	for i, bucket := range buckets {
		if bucket != 0 {
			last = i
		}
	}
	if last < 0 {
		return nil
	}
	out := make([]uint64, last+1)
	copy(out, buckets[:last+1])
	return out
}

func statsFromRuntime(rt gotreesitter.ParseRuntime) runtimeStats {
	publicMaterialized := rt.CompactFullLeafMaterialized + rt.PendingParentMaterialized + rt.FinalChildRefSingleChildMaterializedChildren
	stats := runtimeStats{
		Tokens:                                rt.TokensConsumed,
		Iterations:                            rt.Iterations,
		NodesAllocated:                        rt.NodesAllocated,
		FinalNodes:                            rt.FinalNodes,
		GSSNodes:                              rt.GSSNodesAllocated,
		MaxStacksSeen:                         rt.MaxStacksSeen,
		SingleStackIterations:                 rt.SingleStackIterations,
		MultiStackIterations:                  rt.MultiStackIterations,
		SingleStackTokens:                     rt.SingleStackTokens,
		MultiStackTokens:                      rt.MultiStackTokens,
		MergeStacksIn:                         rt.MergeStacksIn,
		MergeStacksOut:                        rt.MergeStacksOut,
		MergeSlotsUsed:                        rt.MergeSlotsUsed,
		GlobalCullStacksIn:                    rt.GlobalCullStacksIn,
		GlobalCullStacksOut:                   rt.GlobalCullStacksOut,
		ArenaCapacityB:                        rt.ArenaBytesAllocated,
		FinalChildRangeDrains:                 rt.FinalChildRefMaterializedChildren,
		PublicNodesMaterialized:               publicMaterialized,
		ResultSelectionNS:                     rt.ResultSelectionNanos,
		ResultBuildNS:                         rt.ResultTreeBuildNanos,
		ResultCompatibilityNS:                 rt.ResultCompatibilityNanos,
		ResultParentLinkNS:                    rt.ResultParentLinkNanos,
		ResultFinalizeRootNS:                  rt.ResultFinalizeRootNanos,
		ResultExtendTrailingNS:                rt.ResultExtendTrailingNanos,
		ResultNormalizeRootNS:                 rt.ResultNormalizeRootStartNanos,
		TransientParentMatNS:                  rt.TransientParentMaterializationNanos,
		TransientChildMatNS:                   rt.TransientChildMaterializationNanos,
		NormalizationNS:                       rt.NormalizationNanos,
		NormalizationPassesRun:                rt.NormalizationPassesRun,
		NormalizationNodes:                    rt.NormalizationNodesVisited,
		NormalizationRewrites:                 rt.NormalizationNodesRewritten,
		ParseWallNS:                           rt.ParseWallNanos,
		ParserLoopNS:                          rt.ParserLoopNanos,
		TokenNextNS:                           rt.TokenNextNanos,
		ActionDispatchNS:                      rt.ActionDispatchNanos,
		ActionLookupNS:                        rt.ActionLookupNanos,
		GLRMergeNS:                            rt.GLRMergeNanos,
		GLRCullNS:                             rt.GLRCullNanos,
		StackEquivDepthMismatch:               rt.StackEquivDepthMismatch,
		StackEquivHashMismatch:                rt.StackEquivHashMismatch,
		StackEquivStateMismatch:               rt.StackEquivStateMismatch,
		StackEquivPayloadMismatch:             rt.StackEquivPayloadMismatch,
		StackEquivEntryCompares:               rt.StackEquivEntryCompares,
		StackEquivStateMismatchDepthSum:       rt.StackEquivStateMismatchDepthSum,
		StackEquivStateMismatchMaxDepth:       rt.StackEquivStateMismatchMaxDepth,
		StackEquivStateMismatchDepthBuckets:   nonzeroBuckets(rt.StackEquivStateMismatchDepthBuckets),
		StackEquivPayloadMismatchDepthSum:     rt.StackEquivPayloadMismatchDepthSum,
		StackEquivPayloadMismatchMaxDepth:     rt.StackEquivPayloadMismatchMaxDepth,
		StackEquivPayloadMismatchDepthBuckets: nonzeroBuckets(rt.StackEquivPayloadMismatchDepthBuckets),
		StackEquivPayloadHeaderSigDiff:        rt.StackEquivPayloadHeaderSigDiff,
		StackEquivPayloadHeaderSigSame:        rt.StackEquivPayloadHeaderSigSame,
		StackEquivPayloadShallowSigDiff:       rt.StackEquivPayloadShallowSigDiff,
		StackEquivPayloadShallowSigSame:       rt.StackEquivPayloadShallowSigSame,
		StackEquivPairKeyed:                   rt.StackEquivPairKeyed,
		StackEquivPairUnkeyed:                 rt.StackEquivPairUnkeyed,
		StackEquivPairRepeats:                 rt.StackEquivPairRepeats,
		StackEquivPairRepeatTrue:              rt.StackEquivPairRepeatTrue,
		StackEquivPairRepeatFalse:             rt.StackEquivPairRepeatFalse,
		StackEquivPairRepeatMismatch:          rt.StackEquivPairRepeatMismatch,
		StackEquivPairStores:                  rt.StackEquivPairStores,
		EquivCacheLookups:                     rt.EquivCacheLookups,
		EquivCacheHits:                        rt.EquivCacheHits,
		EquivCacheStores:                      rt.EquivCacheStores,
		EquivCacheMisses:                      rt.EquivCacheMisses,
		EquivCacheTrueHits:                    rt.EquivCacheTrueHits,
		EquivCacheFalseHits:                   rt.EquivCacheFalseHits,
		EquivCacheEpochMisses:                 rt.EquivCacheEpochMisses,
		EquivCacheKeyMisses:                   rt.EquivCacheKeyMisses,
		EquivCacheVersionMisses:               rt.EquivCacheVersionMisses,
		EquivSkipError:                        rt.EquivSkipError,
		EquivSkipLeaf:                         rt.EquivSkipLeaf,
		EquivSkipFieldMismatch:                rt.EquivSkipFieldMismatch,
		EquivExactCalls:                       rt.EquivExactCalls,
		EquivExactTrue:                        rt.EquivExactTrue,
		EquivExactPointerTrue:                 rt.EquivExactPointerTrue,
		EquivExactNilMismatch:                 rt.EquivExactNilMismatch,
		EquivExactHeaderMismatch:              rt.EquivExactHeaderMismatch,
		EquivExactChildMismatch:               rt.EquivExactChildMismatch,
		EquivExactTerminalCalls:               rt.EquivExactTerminalCalls,
		EquivExactTerminalTrue:                rt.EquivExactTerminalTrue,
		EquivExactTerminalFalse:               rt.EquivExactTerminalFalse,
		EquivFrontierCalls:                    rt.EquivFrontierCalls,
		EquivFrontierTrue:                     rt.EquivFrontierTrue,
		EquivExactChildCompares:               rt.EquivExactChildCompares,
		EquivFrontierChildScans:               rt.EquivFrontierChildScans,
		EquivFrontierCandidateCompares:        rt.EquivFrontierCandidateCompares,
		NoTreeReduceNodes:                     rt.NoTreeReduceNodesConstructed,
		NoTreeLeafNodes:                       rt.NoTreeLeafNodesConstructed,
		ReduceChildFastGSS:                    pathStatsFromRuntime(rt.ReduceChildFastGSS),
		ReduceChildAllVisible:                 pathStatsFromRuntime(rt.ReduceChildAllVisible),
		ReduceChildNoAlias:                    pathStatsFromRuntime(rt.ReduceChildNoAlias),
		ReduceChildScratchGeneral:             pathStatsFromRuntime(rt.ReduceChildScratchGeneral),
		ReduceChildScratchNoAlias:             pathStatsFromRuntime(rt.ReduceChildScratchNoAlias),
	}
	if reduceTiming := rt.ReduceTiming; reduceTiming != nil {
		stats.ReduceRangeNS = reduceTiming.RangeNanos
		stats.ReducePendingParentNS = reduceTiming.PendingParentNanos
		stats.ReduceChildBuildNS = reduceTiming.ChildBuildNanos
		stats.ReduceParentBuildNS = reduceTiming.ParentBuildNanos
		stats.ReduceSpanNS = reduceTiming.SpanNanos
		stats.ReduceStackPushNS = reduceTiming.StackPushNanos
		stats.ReduceNoTreeBuildNS = reduceTiming.NoTreeBuildNanos
	}
	if actionTiming := rt.ActionTiming; actionTiming != nil {
		stats.ActionExtraShiftNS = actionTiming.ExtraShiftNanos
		stats.ActionNoActionNS = actionTiming.NoActionNanos
		stats.ActionNoActionRelexNS = actionTiming.NoActionRelexNanos
		stats.ActionNoActionMissingNS = actionTiming.NoActionMissingNanos
		stats.ActionNoActionRecoverNS = actionTiming.NoActionRecoverNanos
		stats.ActionNoActionErrorNS = actionTiming.NoActionErrorNanos
		stats.ActionConflictChoiceNS = actionTiming.ConflictChoiceNanos
		stats.ActionConflictForkNS = actionTiming.ConflictForkNanos
		stats.ActionSingleShiftNS = actionTiming.SingleShiftNanos
		stats.ActionSingleReduceNS = actionTiming.SingleReduceNanos
		stats.ActionSingleAcceptNS = actionTiming.SingleAcceptNanos
		stats.ActionSingleRecoverNS = actionTiming.SingleRecoverNanos
		stats.ActionSingleOtherNS = actionTiming.SingleOtherNanos
	}
	if rt.NormalizationPasses != nil && len(*rt.NormalizationPasses) > 0 {
		passes := make([]passStats, len(*rt.NormalizationPasses))
		for i, pass := range *rt.NormalizationPasses {
			passes[i] = passStats{
				Name:     pass.Name,
				Checked:  pass.Checked,
				Run:      pass.Run,
				NS:       pass.Nanos,
				Nodes:    pass.NodesVisited,
				Rewrites: pass.NodesRewritten,
			}
		}
		stats.NormalizationPasses = &passes
	}
	return stats
}

func pathStatsFromRuntime(rt gotreesitter.ReduceChildPathRuntime) *pathStats {
	if rt.SlicesAllocated == 0 &&
		rt.SlicesRetained == 0 &&
		rt.SlicesDropped == 0 &&
		rt.PointersAllocated == 0 &&
		rt.PointersRetained == 0 &&
		rt.PointersDropped == 0 {
		return nil
	}
	return &pathStats{
		SlicesAllocated:   rt.SlicesAllocated,
		SlicesRetained:    rt.SlicesRetained,
		SlicesDropped:     rt.SlicesDropped,
		PointersAllocated: rt.PointersAllocated,
		PointersRetained:  rt.PointersRetained,
		PointersDropped:   rt.PointersDropped,
	}
}

func runGoEdit(r *runner, source []byte, noop bool) (runtimeStats, error) {
	oldTree, err := parseGo(r, source)
	if err != nil {
		return runtimeStats{}, err
	}
	defer oldTree.Release()
	edited := source
	if !noop {
		candidate, ok := chooseEdit(source)
		if !ok {
			return runtimeStats{}, fmt.Errorf("no safe edit candidate")
		}
		edited = applyEdit(source, candidate)
		oldTree.Edit(candidate.inputEdit(source, edited))
	}
	tree, profile, ok, err := parseGoIncremental(r, edited, oldTree)
	if err != nil {
		return runtimeStats{}, err
	}
	defer tree.Release()
	stats := statsFromGoTree(r, tree, 0, 0)
	if ok {
		stats.ParseWallNS = profile.ReparseNanos + profile.ReuseCursorNanos
		stats.ParserLoopNS = profile.ParserLoopNanos
		stats.TokenNextNS = profile.TokenNextNanos
		stats.ActionDispatchNS = profile.ActionDispatchNanos
		stats.ActionLookupNS = profile.ActionLookupNanos
		stats.GLRMergeNS = profile.GLRMergeNanos
		stats.GLRCullNS = profile.GLRCullNanos
		stats.ReduceRangeNS = profile.ReduceRangeNanos
		stats.ReducePendingParentNS = profile.ReducePendingParentNanos
		stats.ReduceChildBuildNS = profile.ReduceChildBuildNanos
		stats.ReduceParentBuildNS = profile.ReduceParentBuildNanos
		stats.ReduceSpanNS = profile.ReduceSpanNanos
		stats.ReduceStackPushNS = profile.ReduceStackPushNanos
		stats.ReduceNoTreeBuildNS = profile.ReduceNoTreeBuildNanos
		stats.ActionExtraShiftNS = profile.ActionExtraShiftNanos
		stats.ActionNoActionNS = profile.ActionNoActionNanos
		stats.ActionNoActionRelexNS = profile.ActionNoActionRelexNanos
		stats.ActionNoActionMissingNS = profile.ActionNoActionMissingNanos
		stats.ActionNoActionRecoverNS = profile.ActionNoActionRecoverNanos
		stats.ActionNoActionErrorNS = profile.ActionNoActionErrorNanos
		stats.ActionConflictChoiceNS = profile.ActionConflictChoiceNanos
		stats.ActionConflictForkNS = profile.ActionConflictForkNanos
		stats.ActionSingleShiftNS = profile.ActionSingleShiftNanos
		stats.ActionSingleReduceNS = profile.ActionSingleReduceNanos
		stats.ActionSingleAcceptNS = profile.ActionSingleAcceptNanos
		stats.ActionSingleRecoverNS = profile.ActionSingleRecoverNanos
		stats.ActionSingleOtherNS = profile.ActionSingleOtherNanos
		stats.ResultBuildNS = profile.ResultTreeBuildNanos
		stats.NormalizationNS = profile.NormalizationNanos
		stats.NodesAllocated = int(profile.NewNodesAllocated)
		stats.Tokens = profile.TokensConsumed
	}
	return stats, nil
}

type editCandidate struct {
	start       int
	oldEnd      int
	replacement []byte
}

func chooseEdit(source []byte) (editCandidate, bool) {
	for i, b := range source {
		repl, ok := replacementByte(b)
		if ok {
			return editCandidate{start: i, oldEnd: i + 1, replacement: []byte{repl}}, true
		}
	}
	if len(source) > 0 {
		return editCandidate{start: len(source) / 2, oldEnd: len(source) / 2, replacement: []byte(" ")}, true
	}
	return editCandidate{}, false
}

func replacementByte(b byte) (byte, bool) {
	switch {
	case b >= '0' && b <= '8':
		return b + 1, true
	case b == '9':
		return '0', true
	case b >= 'a' && b <= 'y':
		return b + 1, true
	case b == 'z':
		return 'a', true
	case b >= 'A' && b <= 'Y':
		return b + 1, true
	case b == 'Z':
		return 'A', true
	default:
		return 0, false
	}
}

func applyEdit(source []byte, edit editCandidate) []byte {
	out := make([]byte, 0, len(source)-(edit.oldEnd-edit.start)+len(edit.replacement))
	out = append(out, source[:edit.start]...)
	out = append(out, edit.replacement...)
	out = append(out, source[edit.oldEnd:]...)
	return out
}

func (e editCandidate) inputEdit(oldSource, newSource []byte) gotreesitter.InputEdit {
	start := pointAtByte(oldSource, e.start)
	oldEnd := pointAtByte(oldSource, e.oldEnd)
	newEnd := pointAtByte(newSource, e.start+len(e.replacement))
	return gotreesitter.InputEdit{
		StartByte:   uint32(e.start),
		OldEndByte:  uint32(e.oldEnd),
		NewEndByte:  uint32(e.start + len(e.replacement)),
		StartPoint:  start,
		OldEndPoint: oldEnd,
		NewEndPoint: newEnd,
	}
}

func pointAtByte(source []byte, offset int) gotreesitter.Point {
	if offset < 0 {
		offset = 0
	}
	if offset > len(source) {
		offset = len(source)
	}
	var row, col uint32
	for _, b := range source[:offset] {
		if b == '\n' {
			row++
			col = 0
		} else {
			col++
		}
	}
	return gotreesitter.Point{Row: row, Column: col}
}

func runGoQuery(r *runner, tree *gotreesitter.Tree, source []byte, queryText string) (uint64, queryTimingStats, error) {
	timingEnabled := strings.TrimSpace(os.Getenv("GOT_PARSE_PHASE_TIMING")) != ""
	timing := queryTimingStats{}
	compileStart := time.Time{}
	if timingEnabled {
		compileStart = time.Now()
	}
	q, err := gotreesitter.NewQuery(queryText, r.goLang)
	if err != nil {
		return 0, timing, err
	}
	if timingEnabled {
		timing.compileNS = time.Since(compileStart).Nanoseconds()
	}
	execStart := time.Time{}
	if timingEnabled {
		execStart = time.Now()
	}
	rootStart := time.Time{}
	if timingEnabled {
		rootStart = time.Now()
	}
	root := tree.RootNode()
	if timingEnabled {
		timing.rootNS = time.Since(rootStart).Nanoseconds()
	}
	cursorStart := time.Time{}
	if timingEnabled {
		cursorStart = time.Now()
	}
	cursor := q.Exec(root, r.goLang, source)
	var captures uint64
	for {
		match, ok := cursor.NextMatch()
		if !ok {
			break
		}
		captures += uint64(len(match.Captures))
	}
	if timingEnabled {
		timing.cursorNS = time.Since(cursorStart).Nanoseconds()
		timing.execNS = time.Since(execStart).Nanoseconds()
	}
	return captures, timing, nil
}

func queryParity(r *runner, goTree *gotreesitter.Tree, cTree *sitter.Tree, source []byte, queryText string) (bool, error) {
	goMatches, err := collectGoQueryMatches(r, goTree, source, queryText)
	if err != nil {
		return false, err
	}
	cMatches, err := collectCQueryMatches(r, cTree, source, queryText)
	if err != nil {
		return false, err
	}
	gb, _ := json.Marshal(goMatches)
	cb, _ := json.Marshal(cMatches)
	return bytes.Equal(gb, cb), nil
}

type highlightCapture struct {
	Name      string `json:"name"`
	StartByte uint32 `json:"start_byte"`
	EndByte   uint32 `json:"end_byte"`
}

func highlightParity(r *runner, goTree *gotreesitter.Tree, cTree *sitter.Tree, source []byte, queryText string) (bool, error) {
	goCaps, err := collectGoHighlightCaptures(r, goTree, queryText)
	if err != nil {
		return false, err
	}
	cCaps, err := collectCHighlightCaptures(r, cTree, source, queryText)
	if err != nil {
		return false, err
	}
	gb, _ := json.Marshal(goCaps)
	cb, _ := json.Marshal(cCaps)
	return bytes.Equal(gb, cb), nil
}

func collectGoHighlightCaptures(r *runner, tree *gotreesitter.Tree, queryText string) ([]highlightCapture, error) {
	q, err := gotreesitter.NewQuery(queryText, r.goLang)
	if err != nil {
		return nil, err
	}
	matches := q.Execute(tree)
	var caps []highlightCapture
	for _, m := range matches {
		for _, capture := range m.Captures {
			if capture.Node == nil || !includeHighlightCaptureName(capture.Name) {
				continue
			}
			caps = append(caps, highlightCapture{
				Name:      capture.Name,
				StartByte: capture.Node.StartByte(),
				EndByte:   capture.Node.EndByte(),
			})
		}
	}
	return deduplicateHighlightCaptures(caps), nil
}

func collectCHighlightCaptures(r *runner, tree *sitter.Tree, source []byte, queryText string) ([]highlightCapture, error) {
	query, err := sitter.NewQuery(r.cLang, queryText)
	if err != nil {
		return nil, err
	}
	defer query.Close()
	cursor := sitter.NewQueryCursor()
	defer cursor.Close()
	names := query.CaptureNames()
	iter := cursor.Matches(query, tree.RootNode(), source)
	var caps []highlightCapture
	for {
		m := iter.Next()
		if m == nil {
			break
		}
		if !cQueryMatchSatisfiesGeneralPredicates(m, query, source) {
			continue
		}
		for _, capture := range m.Captures {
			name := ""
			if int(capture.Index) < len(names) {
				name = names[capture.Index]
			}
			if !includeHighlightCaptureName(name) {
				continue
			}
			caps = append(caps, highlightCapture{
				Name:      name,
				StartByte: uint32(capture.Node.StartByte()),
				EndByte:   uint32(capture.Node.EndByte()),
			})
		}
	}
	return deduplicateHighlightCaptures(caps), nil
}

func includeHighlightCaptureName(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && !strings.HasPrefix(name, "_")
}

func deduplicateHighlightCaptures(caps []highlightCapture) []highlightCapture {
	sort.Slice(caps, func(i, j int) bool {
		if caps[i].StartByte != caps[j].StartByte {
			return caps[i].StartByte < caps[j].StartByte
		}
		if caps[i].EndByte != caps[j].EndByte {
			return caps[i].EndByte < caps[j].EndByte
		}
		return caps[i].Name < caps[j].Name
	})
	out := caps[:0]
	var prev highlightCapture
	for i, cap := range caps {
		if i > 0 && cap == prev {
			continue
		}
		out = append(out, cap)
		prev = cap
	}
	return out
}

func cQueryMatchSatisfiesGeneralPredicates(m *sitter.QueryMatch, query *sitter.Query, source []byte) bool {
	if m == nil || query == nil {
		return true
	}
	for _, pred := range query.GeneralPredicates(m.PatternIndex) {
		switch pred.Operator {
		case "lua-match?":
			if len(pred.Args) != 2 || pred.Args[0].CaptureId == nil || pred.Args[1].String == nil {
				return false
			}
			text, ok := cFirstCaptureTextForID(m, *pred.Args[0].CaptureId, source)
			if !ok {
				return false
			}
			rx, err := compileHighlightLuaPattern(*pred.Args[1].String)
			if err != nil || !rx.MatchString(text) {
				return false
			}
		}
	}
	return true
}

func cFirstCaptureTextForID(m *sitter.QueryMatch, captureID uint, source []byte) (string, bool) {
	for _, capture := range m.Captures {
		if uint(capture.Index) != captureID {
			continue
		}
		start := uint32(capture.Node.StartByte())
		end := uint32(capture.Node.EndByte())
		if start > end || end > uint32(len(source)) {
			return "", false
		}
		return string(source[start:end]), true
	}
	return "", false
}

func compileHighlightLuaPattern(pattern string) (*regexp.Regexp, error) {
	var out strings.Builder
	inClass := false
	classContentStart := false

	writeLuaClass := func(ch byte, inClass bool) bool {
		inClassText := ""
		outsideText := ""
		switch ch {
		case 'a':
			inClassText = "A-Za-z"
			outsideText = "[A-Za-z]"
		case 'A':
			inClassText = "^A-Za-z"
			outsideText = "[^A-Za-z]"
		case 'd':
			inClassText = "0-9"
			outsideText = "[0-9]"
		case 'D':
			inClassText = "^0-9"
			outsideText = "[^0-9]"
		case 'l':
			inClassText = "a-z"
			outsideText = "[a-z]"
		case 'L':
			inClassText = "^a-z"
			outsideText = "[^a-z]"
		case 's':
			inClassText = "\\s"
			outsideText = "\\s"
		case 'S':
			inClassText = "^\\s"
			outsideText = "\\S"
		case 'u':
			inClassText = "A-Z"
			outsideText = "[A-Z]"
		case 'U':
			inClassText = "^A-Z"
			outsideText = "[^A-Z]"
		case 'w':
			inClassText = "A-Za-z0-9"
			outsideText = "[A-Za-z0-9]"
		case 'W':
			inClassText = "^A-Za-z0-9"
			outsideText = "[^A-Za-z0-9]"
		case 'x':
			inClassText = "A-Fa-f0-9"
			outsideText = "[A-Fa-f0-9]"
		case 'X':
			inClassText = "^A-Fa-f0-9"
			outsideText = "[^A-Fa-f0-9]"
		default:
			return false
		}
		if inClass {
			out.WriteString(inClassText)
		} else {
			out.WriteString(outsideText)
		}
		return true
	}

	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '%':
			if i+1 >= len(pattern) {
				out.WriteString("%")
				continue
			}
			i++
			next := pattern[i]
			if writeLuaClass(next, inClass) {
				classContentStart = false
				continue
			}
			out.WriteString(regexp.QuoteMeta(string(next)))
			classContentStart = false
		case '[':
			inClass = true
			classContentStart = true
			out.WriteByte('[')
		case ']':
			inClass = false
			classContentStart = false
			out.WriteByte(']')
		case '-':
			if inClass {
				out.WriteByte('-')
			} else {
				out.WriteString("*?")
			}
			classContentStart = false
		case '+', '*', '?':
			out.WriteByte(ch)
			classContentStart = false
		case '^':
			if inClass && !classContentStart {
				out.WriteString("\\^")
			} else {
				out.WriteByte('^')
			}
			classContentStart = false
		case '$':
			if inClass {
				out.WriteString("\\$")
			} else {
				out.WriteByte('$')
			}
			classContentStart = false
		default:
			if strings.ContainsRune(`\.+(){}|`, rune(ch)) {
				out.WriteByte('\\')
			}
			out.WriteByte(ch)
			classContentStart = false
		}
	}
	return regexp.Compile(out.String())
}

func collectGoQueryMatches(r *runner, tree *gotreesitter.Tree, source []byte, queryText string) ([]queryMatch, error) {
	q, err := gotreesitter.NewQuery(queryText, r.goLang)
	if err != nil {
		return nil, err
	}
	cursor := q.Exec(tree.RootNode(), r.goLang, source)
	var matches []queryMatch
	for {
		m, ok := cursor.NextMatch()
		if !ok {
			break
		}
		snap := queryMatch{PatternIndex: m.PatternIndex}
		for _, capture := range m.Captures {
			if capture.Node == nil {
				continue
			}
			snap.Captures = append(snap.Captures, queryCapture{
				Name:      capture.Name,
				Type:      capture.Node.Type(r.goLang),
				Named:     capture.Node.IsNamed(),
				StartByte: capture.Node.StartByte(),
				EndByte:   capture.Node.EndByte(),
				Text:      capture.Text(source),
			})
		}
		matches = append(matches, snap)
	}
	return matches, nil
}

func collectCQueryMatches(r *runner, tree *sitter.Tree, source []byte, queryText string) ([]queryMatch, error) {
	query, err := sitter.NewQuery(r.cLang, queryText)
	if err != nil {
		return nil, err
	}
	defer query.Close()
	cursor := sitter.NewQueryCursor()
	defer cursor.Close()
	names := query.CaptureNames()
	iter := cursor.Matches(query, tree.RootNode(), source)
	var matches []queryMatch
	for {
		m := iter.Next()
		if m == nil {
			break
		}
		snap := queryMatch{PatternIndex: int(m.PatternIndex)}
		for _, capture := range m.Captures {
			name := ""
			if int(capture.Index) < len(names) {
				name = names[capture.Index]
			}
			start := uint32(capture.Node.StartByte())
			end := uint32(capture.Node.EndByte())
			snap.Captures = append(snap.Captures, queryCapture{
				Name:      name,
				Type:      capture.Node.Kind(),
				Named:     capture.Node.IsNamed(),
				StartByte: start,
				EndByte:   end,
				Text:      string(source[start:end]),
			})
		}
		matches = append(matches, snap)
	}
	return matches, nil
}

func walkCursor(tree *gotreesitter.Tree) uint64 {
	cursor := gotreesitter.NewTreeCursorFromTree(tree)
	var count uint64
	var walk func()
	walk = func() {
		if cursor.CurrentNode() == nil {
			return
		}
		count++
		if cursor.GotoFirstChild() {
			for {
				walk()
				if !cursor.GotoNextSibling() {
					break
				}
			}
			cursor.GotoParent()
		}
	}
	walk()
	return count
}

func touchParentSibling(root *gotreesitter.Node) {
	var walk func(*gotreesitter.Node)
	walk = func(n *gotreesitter.Node) {
		if n == nil {
			return
		}
		_ = n.Parent()
		_ = n.NextSibling()
		_ = n.PrevSibling()
		for i := 0; i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
}

func buildRow(common commonFields, s sample, mode string, count int, parity paritySummary, measurement runMeasurement) reportRow {
	row := reportRow{
		Schema:      resultSchema,
		Repo:        common.Repo,
		Commit:      common.Commit,
		Branch:      common.Branch,
		GoVersion:   common.GoVersion,
		DockerImage: common.DockerImage,
		CPULimit:    common.CPULimit,
		MemoryLimit: common.MemoryLimit,
		Language:    s.Language,
		Corpus:      s.Set,
		Sample:      s.RelPath,
		Mode:        mode,
		Bytes:       s.Bytes,
		Iterations:  len(measurement.durations),
		BOp:         0,
		AllocsOp:    0,
		RSSKB:       maxRSSKB(),
		Parity:      parity,
		Runtime:     measurement.runtime,
	}
	if len(measurement.durations) > 0 {
		row.MedianNS = median(measurement.durations)
		row.MeanNS = mean(measurement.durations)
		row.P95NS = percentile(measurement.durations, 0.95)
		row.BOp = int64(measurement.bytes / uint64(len(measurement.durations)))
		row.AllocsOp = float64(measurement.allocs) / float64(len(measurement.durations))
	}
	if measurement.err != nil {
		row.Error = measurement.err.Error()
	}
	if count > 0 && row.Iterations == 0 {
		row.Iterations = count
	}
	return row
}

func errorRow(common commonFields, s sample, mode string, count int, err error) reportRow {
	return reportRow{
		Schema:      resultSchema,
		Repo:        common.Repo,
		Commit:      common.Commit,
		Branch:      common.Branch,
		GoVersion:   common.GoVersion,
		DockerImage: common.DockerImage,
		CPULimit:    common.CPULimit,
		MemoryLimit: common.MemoryLimit,
		Language:    s.Language,
		Corpus:      s.Set,
		Sample:      s.RelPath,
		Mode:        mode,
		Bytes:       s.Bytes,
		Iterations:  count,
		RSSKB:       maxRSSKB(),
		Parity:      paritySummary{Error: err.Error()},
		Error:       err.Error(),
		Blocker:     "parity_blocked",
	}
}

func gateRow(common commonFields, s sample, count int, parity paritySummary) reportRow {
	blocker := "unclassified"
	if parity.Error != "" {
		blocker = "parity_blocked"
	}
	return reportRow{
		Schema:      resultSchema,
		Repo:        common.Repo,
		Commit:      common.Commit,
		Branch:      common.Branch,
		GoVersion:   common.GoVersion,
		DockerImage: common.DockerImage,
		CPULimit:    common.CPULimit,
		MemoryLimit: common.MemoryLimit,
		Language:    s.Language,
		Corpus:      s.Set,
		Sample:      s.RelPath,
		Mode:        "correctness_gate",
		Bytes:       s.Bytes,
		Iterations:  count,
		RSSKB:       maxRSSKB(),
		Parity:      parity,
		Error:       parity.Error,
		Blocker:     blocker,
	}
}

func classify(row reportRow) string {
	if row.Parity.Error != "" {
		return "parity_blocked"
	}
	if row.Error != "" {
		return "mode_error"
	}
	if row.Mode == "go_cursor_walk" && row.Runtime.FinalChildRangeDrains > 0 {
		return "cursor_payback_gap"
	}
	if row.Mode == "go_parse_query" && row.Runtime.FinalChildRangeDrains > 0 {
		return "query_payback_gap"
	}
	if row.Mode == "go_edit" && row.Runtime.PublicNodesMaterialized > 0 {
		return "edit_incremental_gap"
	}
	return "unclassified"
}

func renderSummary(rows []reportRow) string {
	type key struct {
		lang string
		mode string
	}
	by := make(map[key][]reportRow)
	for _, row := range rows {
		by[key{row.Language, row.Mode}] = append(by[key{row.Language, row.Mode}], row)
	}
	langs := make([]string, 0)
	seenLang := map[string]struct{}{}
	for _, row := range rows {
		if _, ok := seenLang[row.Language]; ok {
			continue
		}
		seenLang[row.Language] = struct{}{}
		langs = append(langs, row.Language)
	}
	sort.Strings(langs)

	var b strings.Builder
	fmt.Fprintf(&b, "# Parse Gap Report\n\n")
	fmt.Fprintf(&b, "| lang | samples | parse | highlight | query | go_full/cgo | go_no_tree/cgo | go_query/cgo | go_edit | noop | blocker |\n")
	fmt.Fprintf(&b, "| --- | ---: | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- |\n")
	for _, lang := range langs {
		sampleCount := uniqueSamples(rows, lang)
		blockers := map[string]struct{}{}
		for _, row := range rows {
			if row.Language != lang {
				continue
			}
			if row.Blocker != "" && row.Blocker != "unclassified" {
				blockers[row.Blocker] = struct{}{}
			}
		}
		cgoFull := averageMedian(by[key{lang, "cgo_full"}])
		goFull := averageMedian(by[key{lang, "go_full"}])
		goNoTree := averageMedian(by[key{lang, "go_no_tree"}])
		goQuery := averageMedian(by[key{lang, "go_parse_query"}])
		goEdit := averageMedian(by[key{lang, "go_edit"}])
		goNoop := averageMedian(by[key{lang, "go_noop_incremental"}])
		fmt.Fprintf(&b, "| %s | %d | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			lang,
			sampleCount,
			parseGateString(rows, lang),
			boolGateString(rows, lang, func(p paritySummary) *bool { return p.Highlight }),
			boolGateString(rows, lang, func(p paritySummary) *bool { return p.Query }),
			ratioString(goFull, cgoFull),
			ratioString(goNoTree, cgoFull),
			ratioString(goQuery, cgoFull),
			nsString(goEdit),
			nsString(goNoop),
			joinSet(blockers),
		)
	}
	return b.String()
}

func parseGateString(rows []reportRow, lang string) string {
	seen := false
	for _, row := range rows {
		if row.Language != lang {
			continue
		}
		seen = true
		if !row.Parity.Deep {
			return "fail"
		}
	}
	if !seen {
		return "n/a"
	}
	return "ok"
}

func boolGateString(rows []reportRow, lang string, value func(paritySummary) *bool) string {
	seen := false
	for _, row := range rows {
		if row.Language != lang {
			continue
		}
		v := value(row.Parity)
		if v == nil {
			continue
		}
		seen = true
		if !*v {
			return "fail"
		}
	}
	if !seen {
		return "n/a"
	}
	return "ok"
}

func uniqueSamples(rows []reportRow, lang string) int {
	seen := map[string]struct{}{}
	for _, row := range rows {
		if row.Language == lang {
			seen[row.Sample] = struct{}{}
		}
	}
	return len(seen)
}

func averageMedian(rows []reportRow) int64 {
	if len(rows) == 0 {
		return 0
	}
	var total int64
	var n int64
	for _, row := range rows {
		if row.MedianNS > 0 && row.Error == "" {
			total += row.MedianNS
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return total / n
}

func ratioString(num, den int64) string {
	if num <= 0 || den <= 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.2fx", float64(num)/float64(den))
}

func nsString(ns int64) string {
	if ns <= 0 {
		return "n/a"
	}
	switch {
	case ns >= 1_000_000:
		return fmt.Sprintf("%.2fms", float64(ns)/1_000_000)
	case ns >= 1_000:
		return fmt.Sprintf("%.2fus", float64(ns)/1_000)
	default:
		return fmt.Sprintf("%dns", ns)
	}
}

func joinSet(set map[string]struct{}) string {
	if len(set) == 0 {
		return "none"
	}
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return strings.Join(values, ",")
}

func collectSamples(repoRoot string, manifest corpusManifest, selected map[string]struct{}) ([]sample, error) {
	var out []sample
	for _, set := range manifest.Sets {
		if _, ok := selected[set.Language]; !ok {
			continue
		}
		files, err := filesForSet(repoRoot, set)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", set.Name, err)
		}
		for _, path := range files {
			src, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("%s: read %s: %w", set.Name, path, err)
			}
			hash := sha256.Sum256(src)
			rel := relOrAbs(repoRoot, path)
			out = append(out, sample{
				Set:      set.Name,
				Language: set.Language,
				Path:     path,
				RelPath:  rel,
				Bytes:    len(src),
				Source:   src,
				SHA256:   hex.EncodeToString(hash[:]),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Language != out[j].Language {
			return out[i].Language < out[j].Language
		}
		if out[i].Set != out[j].Set {
			return out[i].Set < out[j].Set
		}
		return out[i].RelPath < out[j].RelPath
	})
	return out, nil
}

func filesForSet(repoRoot string, set corpusSetSpec) ([]string, error) {
	var files []string
	for _, file := range set.Files {
		path := resolvePath(repoRoot, file)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			if includeFile(path, set) {
				files = append(files, path)
			}
		}
	}
	roots := set.Roots
	if set.Root != "" {
		roots = append([]string{set.Root}, roots...)
	}
	for _, root := range roots {
		rootPath := resolvePath(repoRoot, root)
		if _, err := os.Stat(rootPath); err != nil {
			continue
		}
		err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			if includeFile(path, set) {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if set.Order == "largest" {
		sort.Slice(files, func(i, j int) bool {
			return fileSize(files[i]) > fileSize(files[j])
		})
	} else {
		sort.Strings(files)
	}
	if set.MaxFiles > 0 && len(files) > set.MaxFiles {
		files = files[:set.MaxFiles]
	}
	return files, nil
}

func includeFile(path string, set corpusSetSpec) bool {
	if set.MaxFileBytes > 0 && fileSize(path) > set.MaxFileBytes {
		return false
	}
	if len(set.Include) == 0 {
		return true
	}
	base := filepath.Base(path)
	for _, pattern := range set.Include {
		if ok, _ := filepath.Match(pattern, base); ok {
			return true
		}
	}
	return false
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func (m queryManifest) byLanguage() map[string][]querySpec {
	out := make(map[string][]querySpec)
	for _, q := range m.Queries {
		if q.Language == "" || q.Query == "" {
			continue
		}
		out[q.Language] = append(out[q.Language], q)
	}
	return out
}

func (m editManifest) byLanguage() map[string][]editSpec {
	out := make(map[string][]editSpec)
	for _, e := range m.Fixtures {
		if e.Language == "" {
			continue
		}
		out[e.Language] = append(out[e.Language], e)
	}
	return out
}

func median(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	cp := append([]int64(nil), values...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	mid := len(cp) / 2
	if len(cp)%2 == 1 {
		return cp[mid]
	}
	return (cp[mid-1] + cp[mid]) / 2
}

func mean(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	var total int64
	for _, value := range values {
		total += value
	}
	return total / int64(len(values))
}

func percentile(values []int64, p float64) int64 {
	if len(values) == 0 {
		return 0
	}
	cp := append([]int64(nil), values...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	idx := int(math.Ceil(float64(len(cp))*p)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	return cp[idx]
}

func maxRSSKB() int64 {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return 0
	}
	return usage.Maxrss
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		value := canonicalLanguage(strings.TrimSpace(part))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func canonicalLanguage(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "js":
		return "javascript"
	case "ts":
		return "typescript"
	case "c++", "cplusplus":
		return "cpp"
	case "c#", "csharp":
		return "c_sharp"
	case "c_lang":
		return "c"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func resolveRepoRoot(flagValue string) (string, error) {
	if strings.TrimSpace(flagValue) != "" {
		return filepath.Abs(flagValue)
	}
	if out := gitOutput(".", "rev-parse", "--show-toplevel"); out != "" {
		return out, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if fileExists(filepath.Join(wd, "go.mod")) && fileExists(filepath.Join(wd, "cgo_harness", "go.mod")) {
			return wd, nil
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			break
		}
		wd = parent
	}
	return "", fmt.Errorf("could not autodetect repo root")
}

func resolvePath(repoRoot, path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(repoRoot, path)
}

func relOrAbs(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil {
		return rel
	}
	return path
}

func loadJSONFile[T any](path string) (T, error) {
	var out T
	b, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return out, err
	}
	return out, nil
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(value)
}

func sha256File(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func gitOutput(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func captureEnvironment() map[string]string {
	keys := []string{
		"GOMAXPROCS",
		"GOMEMLIMIT",
		"GTS_PARSE_GAP_DOCKER_IMAGE",
		"GTS_PARSE_GAP_CPUS",
		"GTS_PARSE_GAP_MEMORY",
	}
	out := make(map[string]string)
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			out[key] = value
		}
	}
	return out
}

func getenvDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "parse_gap_report: "+format+"\n", args...)
	os.Exit(1)
}
