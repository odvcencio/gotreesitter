package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
)

type reportRow struct {
	Language string        `json:"language"`
	Sample   string        `json:"sample"`
	Mode     string        `json:"mode"`
	Bytes    int           `json:"bytes"`
	MedianNS int64         `json:"median_ns"`
	BOp      int64         `json:"b_op"`
	AllocsOp float64       `json:"allocs_op"`
	Parity   paritySummary `json:"parity"`
	Runtime  runtimeStats  `json:"runtime"`
	Error    string        `json:"error"`
	Blocker  string        `json:"blocker"`
}

type paritySummary struct {
	Highlight *bool  `json:"highlight,omitempty"`
	Query     *bool  `json:"query,omitempty"`
	Deep      bool   `json:"deep"`
	Error     string `json:"error,omitempty"`
}

type runtimeStats struct {
	Tokens                          uint64        `json:"tokens,omitempty"`
	NodesAllocated                  int           `json:"nodes_allocated,omitempty"`
	FinalNodes                      uint64        `json:"final_nodes,omitempty"`
	GSSNodes                        uint64        `json:"gss_nodes,omitempty"`
	MaxStacksSeen                   int           `json:"max_stacks_seen,omitempty"`
	SingleStackTokens               uint64        `json:"single_stack_tokens,omitempty"`
	MultiStackTokens                uint64        `json:"multi_stack_tokens,omitempty"`
	MergeStacksIn                   uint64        `json:"merge_stacks_in,omitempty"`
	MergeStacksOut                  uint64        `json:"merge_stacks_out,omitempty"`
	ArenaLiveB                      int64         `json:"arena_live_b,omitempty"`
	ArenaCapacityB                  int64         `json:"arena_capacity_b,omitempty"`
	ArenaCapacityWaste              uint64        `json:"arena_capacity_waste,omitempty"`
	FinalChildRangeDrains           uint64        `json:"final_child_range_drains,omitempty"`
	PublicNodesMaterialized         uint64        `json:"public_nodes_materialized,omitempty"`
	DenseFallbacks                  uint64        `json:"dense_fallbacks,omitempty"`
	ResultSelectionNS               int64         `json:"result_selection_ns,omitempty"`
	ResultBuildNS                   int64         `json:"result_build_ns,omitempty"`
	ResultCompatibilityNS           int64         `json:"result_compatibility_ns,omitempty"`
	ResultParentLinkNS              int64         `json:"result_parent_link_ns,omitempty"`
	ResultFinalizeRootNS            int64         `json:"result_finalize_root_ns,omitempty"`
	ResultExtendTrailingNS          int64         `json:"result_extend_trailing_ns,omitempty"`
	NormalizationNS                 int64         `json:"normalization_ns,omitempty"`
	NormalizationNodes              uint64        `json:"normalization_nodes_visited,omitempty"`
	NormalizationRewrites           uint64        `json:"normalization_nodes_rewritten,omitempty"`
	ParseWallNS                     int64         `json:"parse_wall_ns,omitempty"`
	ParserLoopNS                    int64         `json:"parser_loop_ns,omitempty"`
	TokenNextNS                     int64         `json:"token_next_ns,omitempty"`
	ActionDispatchNS                int64         `json:"action_dispatch_ns,omitempty"`
	ActionLookupNS                  int64         `json:"action_lookup_ns,omitempty"`
	GLRMergeNS                      int64         `json:"glr_merge_ns,omitempty"`
	GLRCullNS                       int64         `json:"glr_cull_ns,omitempty"`
	ReduceRangeNS                   int64         `json:"reduce_range_ns,omitempty"`
	ReducePendingParentNS           int64         `json:"reduce_pending_parent_ns,omitempty"`
	ReduceChildBuildNS              int64         `json:"reduce_child_build_ns,omitempty"`
	ReduceParentBuildNS             int64         `json:"reduce_parent_build_ns,omitempty"`
	ReduceSpanNS                    int64         `json:"reduce_span_ns,omitempty"`
	ReduceStackPushNS               int64         `json:"reduce_stack_push_ns,omitempty"`
	ReduceNoTreeBuildNS             int64         `json:"reduce_notree_build_ns,omitempty"`
	ActionExtraShiftNS              int64         `json:"action_extra_shift_ns,omitempty"`
	ActionNoActionNS                int64         `json:"action_no_action_ns,omitempty"`
	ActionNoActionRelexNS           int64         `json:"action_no_action_relex_ns,omitempty"`
	ActionNoActionMissingNS         int64         `json:"action_no_action_missing_ns,omitempty"`
	ActionNoActionRecoverNS         int64         `json:"action_no_action_recover_ns,omitempty"`
	ActionNoActionErrorNS           int64         `json:"action_no_action_error_ns,omitempty"`
	ActionConflictChoiceNS          int64         `json:"action_conflict_choice_ns,omitempty"`
	ActionConflictForkNS            int64         `json:"action_conflict_fork_ns,omitempty"`
	ActionSingleShiftNS             int64         `json:"action_single_shift_ns,omitempty"`
	ActionSingleReduceNS            int64         `json:"action_single_reduce_ns,omitempty"`
	ActionSingleAcceptNS            int64         `json:"action_single_accept_ns,omitempty"`
	ActionSingleRecoverNS           int64         `json:"action_single_recover_ns,omitempty"`
	ActionSingleOtherNS             int64         `json:"action_single_other_ns,omitempty"`
	MergeCalls                      uint64        `json:"merge_calls,omitempty"`
	EquivCacheLookups               uint64        `json:"equiv_cache_lookups,omitempty"`
	EquivCacheHits                  uint64        `json:"equiv_cache_hits,omitempty"`
	EquivCacheStores                uint64        `json:"equiv_cache_stores,omitempty"`
	EquivCacheMisses                uint64        `json:"equiv_cache_misses,omitempty"`
	EquivCacheEpochMisses           uint64        `json:"equiv_cache_epoch_misses,omitempty"`
	EquivCacheKeyMisses             uint64        `json:"equiv_cache_key_misses,omitempty"`
	EquivCacheVersionMisses         uint64        `json:"equiv_cache_version_misses,omitempty"`
	EquivSkipError                  uint64        `json:"equiv_skip_error,omitempty"`
	EquivSkipLeaf                   uint64        `json:"equiv_skip_leaf,omitempty"`
	EquivSkipFieldMismatch          uint64        `json:"equiv_skip_field_mismatch,omitempty"`
	EquivExactCalls                 uint64        `json:"equiv_exact_calls,omitempty"`
	EquivExactTrue                  uint64        `json:"equiv_exact_true,omitempty"`
	EquivFrontierCalls              uint64        `json:"equiv_frontier_calls,omitempty"`
	EquivFrontierTrue               uint64        `json:"equiv_frontier_true,omitempty"`
	EquivExactChildCompares         uint64        `json:"equiv_exact_child_compares,omitempty"`
	EquivFrontierChildScans         uint64        `json:"equiv_frontier_child_scans,omitempty"`
	EquivFrontierCandidateCompares  uint64        `json:"equiv_frontier_candidate_compares,omitempty"`
	ForkCount                       uint64        `json:"fork_count,omitempty"`
	ConflictRR                      uint64        `json:"conflict_rr,omitempty"`
	ConflictRS                      uint64        `json:"conflict_rs,omitempty"`
	ConflictOther                   uint64        `json:"conflict_other,omitempty"`
	LexBytes                        uint64        `json:"lex_bytes,omitempty"`
	LexTokens                       uint64        `json:"lex_tokens,omitempty"`
	ReduceChainSteps                uint64        `json:"reduce_chain_steps,omitempty"`
	ReduceChainMaxLen               uint64        `json:"reduce_chain_max_len,omitempty"`
	ReduceChainClassHits            uint64        `json:"reduce_chain_class_hits,omitempty"`
	ReduceChainStopNoAction         uint64        `json:"reduce_chain_stop_no_action,omitempty"`
	ReduceChainStopMulti            uint64        `json:"reduce_chain_stop_multi,omitempty"`
	ReduceChainStopShift            uint64        `json:"reduce_chain_stop_shift,omitempty"`
	ReduceChainStopAccept           uint64        `json:"reduce_chain_stop_accept,omitempty"`
	ReduceChainStopDead             uint64        `json:"reduce_chain_stop_dead,omitempty"`
	ReduceChainStopCycle            uint64        `json:"reduce_chain_stop_cycle,omitempty"`
	ReduceChainStopLimit            uint64        `json:"reduce_chain_stop_limit,omitempty"`
	ReduceChainHintCandidates       uint64        `json:"reduce_chain_hint_candidates,omitempty"`
	ReduceChainHintTaken            uint64        `json:"reduce_chain_hint_taken,omitempty"`
	ReduceChainHintSteps            uint64        `json:"reduce_chain_hint_steps,omitempty"`
	ReduceChainHintTerminalOK       uint64        `json:"reduce_chain_hint_terminal_ok,omitempty"`
	ReduceChainHintTerminalMismatch uint64        `json:"reduce_chain_hint_terminal_mismatch,omitempty"`
	ReduceChainHintLimit            uint64        `json:"reduce_chain_hint_limit,omitempty"`
	ReduceChainHintDead             uint64        `json:"reduce_chain_hint_dead,omitempty"`
	ReduceChainHintUnexpected       uint64        `json:"reduce_chain_hint_unexpected_action,omitempty"`
	NoTreeReduceNodes               uint64        `json:"notree_reduce_nodes,omitempty"`
	NoTreeLeafNodes                 uint64        `json:"notree_leaf_nodes,omitempty"`
	HotAmbiguities                  []hotGLRState `json:"hot_ambiguities,omitempty"`
	HotReduceChains                 []hotGLRState `json:"hot_reduce_chains,omitempty"`
	HotReduceChainRuns              []hotGLRState `json:"hot_reduce_chain_runs,omitempty"`
	HotMergeStates                  []hotGLRState `json:"hot_merge_states,omitempty"`
	HotEquivStates                  []hotGLRState `json:"hot_equiv_states,omitempty"`
}

type hotGLRState struct {
	State                          uint32 `json:"state"`
	Lookahead                      uint16 `json:"lookahead,omitempty"`
	LookaheadName                  string `json:"lookahead_name,omitempty"`
	ActionCount                    uint8  `json:"action_count,omitempty"`
	ShiftCount                     uint8  `json:"shift_count,omitempty"`
	ReduceCount                    uint8  `json:"reduce_count,omitempty"`
	ReduceSymbol                   uint16 `json:"reduce_symbol,omitempty"`
	ReduceSymbolName               string `json:"reduce_symbol_name,omitempty"`
	ChildCount                     uint8  `json:"child_count,omitempty"`
	ProductionID                   uint16 `json:"production_id,omitempty"`
	Hits                           uint64 `json:"hits,omitempty"`
	Forks                          uint64 `json:"forks,omitempty"`
	MultiStackHits                 uint64 `json:"multi_stack_hits,omitempty"`
	StackInTotal                   uint64 `json:"stack_in_total,omitempty"`
	StackInMax                     int    `json:"stack_in_max,omitempty"`
	ReduceChainHits                uint64 `json:"reduce_chain_hits,omitempty"`
	ReduceChainSteps               uint64 `json:"reduce_chain_steps,omitempty"`
	ReduceChainMaxLen              int    `json:"reduce_chain_max_len,omitempty"`
	ReduceChainNS                  int64  `json:"reduce_chain_ns,omitempty"`
	ReduceChainRuns                uint64 `json:"reduce_chain_runs,omitempty"`
	ReduceChainClassHits           uint64 `json:"reduce_chain_class_hits,omitempty"`
	ReduceChainStopNoAction        uint64 `json:"reduce_chain_stop_no_action,omitempty"`
	ReduceChainStopMulti           uint64 `json:"reduce_chain_stop_multi,omitempty"`
	ReduceChainStopShift           uint64 `json:"reduce_chain_stop_shift,omitempty"`
	ReduceChainStopAccept          uint64 `json:"reduce_chain_stop_accept,omitempty"`
	ReduceChainStopDead            uint64 `json:"reduce_chain_stop_dead,omitempty"`
	ReduceChainStopCycle           uint64 `json:"reduce_chain_stop_cycle,omitempty"`
	ReduceChainStopLimit           uint64 `json:"reduce_chain_stop_limit,omitempty"`
	ReduceChainTerminalState       uint32 `json:"reduce_chain_terminal_state,omitempty"`
	ReduceChainTerminalActionClass uint8  `json:"reduce_chain_terminal_action_class,omitempty"`
	ReduceChainTerminalActionName  string `json:"reduce_chain_terminal_action_name,omitempty"`
	ActionNS                       int64  `json:"action_ns,omitempty"`
	ExtraShiftNS                   int64  `json:"extra_shift_ns,omitempty"`
	NoActionNS                     int64  `json:"no_action_ns,omitempty"`
	ConflictChoiceNS               int64  `json:"conflict_choice_ns,omitempty"`
	ConflictForkNS                 int64  `json:"conflict_fork_ns,omitempty"`
	SingleShiftNS                  int64  `json:"single_shift_ns,omitempty"`
	SingleReduceNS                 int64  `json:"single_reduce_ns,omitempty"`
	SingleAcceptNS                 int64  `json:"single_accept_ns,omitempty"`
	SingleRecoverNS                int64  `json:"single_recover_ns,omitempty"`
	SingleOtherNS                  int64  `json:"single_other_ns,omitempty"`
	MergeCalls                     uint64 `json:"merge_calls,omitempty"`
	MergeStacksIn                  uint64 `json:"merge_stacks_in,omitempty"`
	MergeStacksOut                 uint64 `json:"merge_stacks_out,omitempty"`
	MergeStacksInMax               int    `json:"merge_stacks_in_max,omitempty"`
	MergeStacksOutMax              int    `json:"merge_stacks_out_max,omitempty"`
	EquivCacheLookups              uint64 `json:"equiv_cache_lookups,omitempty"`
	EquivCacheHits                 uint64 `json:"equiv_cache_hits,omitempty"`
	EquivCacheStores               uint64 `json:"equiv_cache_stores,omitempty"`
	EquivCacheMisses               uint64 `json:"equiv_cache_misses,omitempty"`
	EquivCacheEpochMisses          uint64 `json:"equiv_cache_epoch_misses,omitempty"`
	EquivCacheKeyMisses            uint64 `json:"equiv_cache_key_misses,omitempty"`
	EquivCacheVersionMisses        uint64 `json:"equiv_cache_version_misses,omitempty"`
	EquivSkipError                 uint64 `json:"equiv_skip_error,omitempty"`
	EquivSkipLeaf                  uint64 `json:"equiv_skip_leaf,omitempty"`
	EquivSkipFieldMismatch         uint64 `json:"equiv_skip_field_mismatch,omitempty"`
	EquivExactCalls                uint64 `json:"equiv_exact_calls,omitempty"`
	EquivExactTrue                 uint64 `json:"equiv_exact_true,omitempty"`
	EquivFrontierCalls             uint64 `json:"equiv_frontier_calls,omitempty"`
	EquivFrontierTrue              uint64 `json:"equiv_frontier_true,omitempty"`
	EquivExactChildCompares        uint64 `json:"equiv_exact_child_compares,omitempty"`
	EquivFrontierChildScans        uint64 `json:"equiv_frontier_child_scans,omitempty"`
	EquivFrontierCandidateCompares uint64 `json:"equiv_frontier_candidate_compares,omitempty"`
}

type modeAgg struct {
	rows      int
	errors    int
	ns        int64
	bytes     int64
	bop       int64
	allocs    float64
	runtime   runtimeStats
	maxStacks int
}

type langAgg struct {
	language       string
	samples        map[string]struct{}
	bytesBySample  map[string]int
	parityFailures int
	queryFailures  int
	highFailures   int
	modes          map[string]*modeAgg
}

type langScore struct {
	lang                string
	samples             int
	parityFailures      int
	queryFailures       int
	highlightFailures   int
	bytes               int64
	cgoFull             int64
	goFull              int64
	goNoTree            int64
	goQuery             int64
	goCursor            int64
	goEdit              int64
	goNoop              int64
	goFullRuntime       runtimeStats
	goNoTreeRuntime     runtimeStats
	goQueryRuntime      runtimeStats
	fullRatio           float64
	noTreeRatio         float64
	queryRatio          float64
	queryOverFull       float64
	fullOverNoTree      float64
	attrs               map[string]float64
	hotAmbiguities      []hotGLRState
	hotReduceChains     []hotGLRState
	hotReduceChainRuns  []hotGLRState
	hintReduceChainRuns []hotGLRState
	hotMergeStates      []hotGLRState
	hotEquivStates      []hotGLRState
	bucket              string
}

func main() {
	var resultsPath string
	flag.StringVar(&resultsPath, "results", "", "parse_gap_report results.jsonl path")
	flag.Parse()
	if strings.TrimSpace(resultsPath) == "" {
		fmt.Fprintln(os.Stderr, "--results is required")
		os.Exit(2)
	}
	rows, err := readRows(resultsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read results: %v\n", err)
		os.Exit(1)
	}
	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "no rows")
		os.Exit(1)
	}
	scores := scoreRows(rows)
	render(scores)
}

func readRows(path string) ([]reportRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var rows []reportRow
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 64*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var row reportRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, fmt.Errorf("decode line %d: %w", len(rows)+1, err)
		}
		rows = append(rows, row)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

func scoreRows(rows []reportRow) []langScore {
	langs := map[string]*langAgg{}
	for _, row := range rows {
		agg := langs[row.Language]
		if agg == nil {
			agg = &langAgg{
				language:      row.Language,
				samples:       map[string]struct{}{},
				bytesBySample: map[string]int{},
				modes:         map[string]*modeAgg{},
			}
			langs[row.Language] = agg
		}
		agg.samples[row.Sample] = struct{}{}
		if row.Bytes > agg.bytesBySample[row.Sample] {
			agg.bytesBySample[row.Sample] = row.Bytes
		}
		if row.Parity.Error != "" {
			agg.parityFailures++
		}
		if row.Parity.Query != nil && !*row.Parity.Query {
			agg.queryFailures++
		}
		if row.Parity.Highlight != nil && !*row.Parity.Highlight {
			agg.highFailures++
		}
		m := agg.modes[row.Mode]
		if m == nil {
			m = &modeAgg{}
			agg.modes[row.Mode] = m
		}
		if row.Error != "" {
			m.errors++
		}
		if row.MedianNS > 0 && row.Error == "" {
			m.rows++
			m.ns += row.MedianNS
			m.bytes += int64(row.Bytes)
			m.bop += row.BOp
			m.allocs += row.AllocsOp
			m.runtime.add(row.Runtime)
			if row.Runtime.MaxStacksSeen > m.maxStacks {
				m.maxStacks = row.Runtime.MaxStacksSeen
			}
		}
	}
	scores := make([]langScore, 0, len(langs))
	for _, agg := range langs {
		var bytes int64
		for _, n := range agg.bytesBySample {
			bytes += int64(n)
		}
		s := langScore{
			lang:              agg.language,
			samples:           len(agg.samples),
			parityFailures:    agg.parityFailures,
			queryFailures:     agg.queryFailures,
			highlightFailures: agg.highFailures,
			bytes:             bytes,
			cgoFull:           modeNS(agg, "cgo_full"),
			goFull:            modeNS(agg, "go_full"),
			goNoTree:          modeNS(agg, "go_no_tree"),
			goQuery:           modeNS(agg, "go_parse_query"),
			goCursor:          modeNS(agg, "go_cursor_walk"),
			goEdit:            modeNS(agg, "go_edit"),
			goNoop:            modeNS(agg, "go_noop_incremental"),
			attrs:             map[string]float64{},
		}
		s.fullRatio = ratio(s.goFull, s.cgoFull)
		s.noTreeRatio = ratio(s.goNoTree, s.cgoFull)
		s.queryRatio = ratio(s.goQuery, s.cgoFull)
		s.queryOverFull = ratio(s.goQuery, s.goFull)
		s.fullOverNoTree = ratio(s.goFull, s.goNoTree)
		if nt := agg.modes["go_no_tree"]; nt != nil {
			s.goNoTreeRuntime = nt.runtime
		}
		if gf := agg.modes["go_full"]; gf != nil {
			s.goFullRuntime = gf.runtime
		}
		if gq := agg.modes["go_parse_query"]; gq != nil {
			s.goQueryRuntime = gq.runtime
		}
		if gf := agg.modes["go_full"]; gf != nil {
			r := gf.runtime
			tokens := float64(max64(r.Tokens, 1))
			bytesDen := float64(maxInt64(gf.bytes, 1))
			wall := float64(maxInt64(r.ParseWallNS, gf.ns))
			s.attrs["no_tree_ratio"] = s.noTreeRatio
			s.attrs["full_over_no_tree"] = s.fullOverNoTree
			s.attrs["query_over_full"] = s.queryOverFull
			s.attrs["bytes"] = float64(bytes)
			s.attrs["tokens_per_kb"] = tokens / (bytesDen / 1024.0)
			s.attrs["gss_per_token"] = float64(r.GSSNodes) / tokens
			s.attrs["final_nodes_per_token"] = float64(r.FinalNodes) / tokens
			s.attrs["arena_live_per_byte"] = float64(r.ArenaLiveB) / bytesDen
			s.attrs["arena_capacity_waste_per_byte"] = float64(r.ArenaCapacityWaste) / bytesDen
			s.attrs["public_materialized_per_token"] = float64(r.PublicNodesMaterialized) / tokens
			s.attrs["final_child_drains_per_token"] = float64(r.FinalChildRangeDrains) / tokens
			s.attrs["dense_fallbacks_per_token"] = float64(r.DenseFallbacks) / tokens
			s.attrs["merge_calls_per_token"] = float64(r.MergeCalls) / tokens
			s.attrs["equiv_cache_lookups_per_token"] = float64(r.EquivCacheLookups) / tokens
			s.attrs["equiv_cache_hit_share"] = safeRatioUint(r.EquivCacheHits, r.EquivCacheLookups)
			s.attrs["equiv_cache_stores_per_token"] = float64(r.EquivCacheStores) / tokens
			s.attrs["equiv_cache_misses_per_token"] = float64(r.EquivCacheMisses) / tokens
			s.attrs["equiv_cache_miss_share"] = safeRatioUint(r.EquivCacheMisses, r.EquivCacheLookups)
			s.attrs["equiv_cache_epoch_miss_share"] = safeRatioUint(r.EquivCacheEpochMisses, r.EquivCacheMisses)
			s.attrs["equiv_cache_key_miss_share"] = safeRatioUint(r.EquivCacheKeyMisses, r.EquivCacheMisses)
			s.attrs["equiv_cache_version_miss_share"] = safeRatioUint(r.EquivCacheVersionMisses, r.EquivCacheMisses)
			s.attrs["equiv_skip_leaf_per_token"] = float64(r.EquivSkipLeaf) / tokens
			s.attrs["equiv_skip_error_per_token"] = float64(r.EquivSkipError) / tokens
			s.attrs["equiv_skip_field_mismatch_per_token"] = float64(r.EquivSkipFieldMismatch) / tokens
			s.attrs["equiv_exact_calls_per_token"] = float64(r.EquivExactCalls) / tokens
			s.attrs["equiv_exact_true_share"] = safeRatioUint(r.EquivExactTrue, r.EquivExactCalls)
			s.attrs["equiv_frontier_calls_per_token"] = float64(r.EquivFrontierCalls) / tokens
			s.attrs["equiv_frontier_true_share"] = safeRatioUint(r.EquivFrontierTrue, r.EquivFrontierCalls)
			s.attrs["equiv_exact_child_compares_per_token"] = float64(r.EquivExactChildCompares) / tokens
			s.attrs["equiv_frontier_child_scans_per_token"] = float64(r.EquivFrontierChildScans) / tokens
			s.attrs["equiv_frontier_candidate_compares_per_token"] = float64(r.EquivFrontierCandidateCompares) / tokens
			s.attrs["equiv_exact_child_compares_per_call"] = safeRatioUint(r.EquivExactChildCompares, r.EquivExactCalls)
			s.attrs["equiv_frontier_child_scans_per_call"] = safeRatioUint(r.EquivFrontierChildScans, r.EquivFrontierCalls)
			s.attrs["equiv_frontier_candidate_compares_per_call"] = safeRatioUint(r.EquivFrontierCandidateCompares, r.EquivFrontierCalls)
			s.attrs["forks_per_token"] = float64(r.ForkCount) / tokens
			s.attrs["merge_stack_in_per_token"] = float64(r.MergeStacksIn) / tokens
			s.attrs["multi_stack_token_share"] = float64(r.MultiStackTokens) / tokens
			s.attrs["reduce_steps_per_token"] = float64(r.ReduceChainSteps) / tokens
			s.attrs["reduce_class_hits_per_token"] = float64(r.ReduceChainClassHits) / tokens
			s.attrs["reduce_class_hits_per_step"] = safeRatioUint(r.ReduceChainClassHits, r.ReduceChainSteps)
			s.attrs["reduce_stop_shift_per_token"] = float64(r.ReduceChainStopShift) / tokens
			s.attrs["reduce_stop_multi_per_token"] = float64(r.ReduceChainStopMulti) / tokens
			s.attrs["reduce_stop_no_action_per_token"] = float64(r.ReduceChainStopNoAction) / tokens
			s.attrs["reduce_stop_accept_per_token"] = float64(r.ReduceChainStopAccept) / tokens
			s.attrs["reduce_stop_dead_per_token"] = float64(r.ReduceChainStopDead) / tokens
			s.attrs["reduce_stop_cycle_per_token"] = float64(r.ReduceChainStopCycle) / tokens
			s.attrs["reduce_stop_limit_per_token"] = float64(r.ReduceChainStopLimit) / tokens
			s.attrs["reduce_hint_candidates_per_token"] = float64(r.ReduceChainHintCandidates) / tokens
			s.attrs["reduce_hint_taken_per_token"] = float64(r.ReduceChainHintTaken) / tokens
			s.attrs["reduce_hint_steps_per_token"] = float64(r.ReduceChainHintSteps) / tokens
			s.attrs["reduce_hint_terminal_ok_share"] = safeRatioUint(r.ReduceChainHintTerminalOK, r.ReduceChainHintTaken)
			s.attrs["reduce_hint_terminal_mismatch_per_token"] = float64(r.ReduceChainHintTerminalMismatch) / tokens
			s.attrs["reduce_hint_limit_per_token"] = float64(r.ReduceChainHintLimit) / tokens
			s.attrs["reduce_hint_dead_per_token"] = float64(r.ReduceChainHintDead) / tokens
			s.attrs["reduce_hint_unexpected_action_per_token"] = float64(r.ReduceChainHintUnexpected) / tokens
			s.attrs["normalization_nodes_per_token"] = float64(r.NormalizationNodes) / tokens
			s.attrs["normalization_rewrites_per_token"] = float64(r.NormalizationRewrites) / tokens
			s.attrs["parser_loop_share"] = float64(r.ParserLoopNS) / wall
			s.attrs["token_next_share"] = float64(r.TokenNextNS) / wall
			s.attrs["action_lookup_share"] = float64(r.ActionLookupNS) / wall
			s.attrs["action_dispatch_share"] = float64(r.ActionDispatchNS) / wall
			s.attrs["glr_merge_share"] = float64(r.GLRMergeNS) / wall
			s.attrs["reduce_range_share"] = float64(r.ReduceRangeNS) / wall
			s.attrs["reduce_pending_parent_share"] = float64(r.ReducePendingParentNS) / wall
			s.attrs["reduce_child_build_share"] = float64(r.ReduceChildBuildNS) / wall
			s.attrs["reduce_parent_build_share"] = float64(r.ReduceParentBuildNS) / wall
			s.attrs["reduce_span_share"] = float64(r.ReduceSpanNS) / wall
			s.attrs["reduce_stack_push_share"] = float64(r.ReduceStackPushNS) / wall
			s.attrs["reduce_notree_build_share"] = float64(r.ReduceNoTreeBuildNS) / wall
			s.attrs["action_extra_shift_share"] = float64(r.ActionExtraShiftNS) / wall
			s.attrs["action_no_action_share"] = float64(r.ActionNoActionNS) / wall
			s.attrs["action_no_action_relex_share"] = float64(r.ActionNoActionRelexNS) / wall
			s.attrs["action_no_action_missing_share"] = float64(r.ActionNoActionMissingNS) / wall
			s.attrs["action_no_action_recover_share"] = float64(r.ActionNoActionRecoverNS) / wall
			s.attrs["action_no_action_error_share"] = float64(r.ActionNoActionErrorNS) / wall
			s.attrs["action_conflict_choice_share"] = float64(r.ActionConflictChoiceNS) / wall
			s.attrs["action_conflict_fork_share"] = float64(r.ActionConflictForkNS) / wall
			s.attrs["action_single_shift_share"] = float64(r.ActionSingleShiftNS) / wall
			s.attrs["action_single_reduce_share"] = float64(r.ActionSingleReduceNS) / wall
			s.attrs["action_single_accept_share"] = float64(r.ActionSingleAcceptNS) / wall
			s.attrs["action_single_recover_share"] = float64(r.ActionSingleRecoverNS) / wall
			s.attrs["action_single_other_share"] = float64(r.ActionSingleOtherNS) / wall
			s.attrs["result_build_share"] = float64(r.ResultBuildNS) / wall
			s.attrs["result_compat_share"] = float64(r.ResultCompatibilityNS) / wall
			s.attrs["normalization_share"] = float64(r.NormalizationNS) / wall
		}
		if gf := agg.modes["go_full"]; gf != nil && runtimeHasHotShapes(gf.runtime) {
			assignHotShapes(&s, gf.runtime)
		} else if nt := agg.modes["go_no_tree"]; nt != nil && runtimeHasHotShapes(nt.runtime) {
			assignHotShapes(&s, nt.runtime)
		}
		s.bucket = classify(s)
		scores = append(scores, s)
	}
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].fullRatio == scores[j].fullRatio {
			return scores[i].lang < scores[j].lang
		}
		return scores[i].fullRatio > scores[j].fullRatio
	})
	return scores
}

func runtimeHasHotShapes(r runtimeStats) bool {
	return len(r.HotAmbiguities) > 0 || len(r.HotReduceChains) > 0 ||
		len(r.HotReduceChainRuns) > 0 || len(r.HotMergeStates) > 0 ||
		len(r.HotEquivStates) > 0
}

func assignHotShapes(s *langScore, r runtimeStats) {
	s.hotAmbiguities = topHotStates(r.HotAmbiguities, func(h hotGLRState) uint64 {
		if h.ActionNS > 0 {
			return uint64(h.ActionNS)
		}
		if h.StackInTotal > 0 {
			return h.StackInTotal
		}
		return h.Hits
	}, 5)
	s.hotReduceChains = topHotStates(r.HotReduceChains, func(h hotGLRState) uint64 {
		if h.ReduceChainNS > 0 {
			return uint64(h.ReduceChainNS)
		}
		return h.ReduceChainSteps
	}, 5)
	s.hotReduceChainRuns = topHotStates(r.HotReduceChainRuns, func(h hotGLRState) uint64 {
		if h.ReduceChainNS > 0 {
			return uint64(h.ReduceChainNS)
		}
		if h.ReduceChainSteps > 0 {
			return h.ReduceChainSteps
		}
		return h.ReduceChainRuns
	}, 5)
	s.hintReduceChainRuns = aggregateHotStates(r.HotReduceChainRuns)
	s.hotMergeStates = topHotStates(r.HotMergeStates, func(h hotGLRState) uint64 {
		return h.MergeStacksIn
	}, 5)
	s.hotEquivStates = topHotStates(r.HotEquivStates, func(h hotGLRState) uint64 {
		return h.EquivCacheLookups + h.EquivExactCalls + h.EquivFrontierCalls
	}, 5)
}

func modeNS(agg *langAgg, mode string) int64 {
	if agg == nil || agg.modes[mode] == nil {
		return 0
	}
	return agg.modes[mode].ns
}

func classify(s langScore) string {
	if s.parityFailures > 0 {
		return "parity_blocked"
	}
	coreGap := s.noTreeRatio > 2.0
	resultGap := s.fullOverNoTree > 2.0
	switch {
	case s.fullRatio == 0:
		return "missing_full_timing"
	case coreGap && resultGap:
		return "parser_core_plus_result_gap"
	case coreGap:
		return "parser_core_gap"
	case resultGap:
		return "result_or_materialization_gap"
	case s.queryOverFull > 1.5:
		return "query_payback_gap"
	case s.fullRatio <= 1.15:
		return "cgo_class"
	case s.fullRatio <= 2.0:
		return "sub_2x_followup"
	default:
		return "mixed_gap"
	}
}

func render(scores []langScore) {
	fmt.Println("# Parse Gap Correlation")
	fmt.Println()
	fmt.Println("Ratios use summed per-sample medians, so large and small files both contribute their measured lane cost. Correlations are directional; top-12 rings are intentionally small.")
	fmt.Println()
	fmt.Println("| lang | samples | parity_fail | full/cgo | no_tree/cgo | full/no_tree | query/full | bytes | bucket |")
	fmt.Println("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |")
	for _, s := range scores {
		fmt.Printf("| %s | %d | %d | %s | %s | %s | %s | %d | %s |\n",
			s.lang,
			s.samples,
			s.parityFailures,
			ratioText(s.fullRatio),
			ratioText(s.noTreeRatio),
			ratioText(s.fullOverNoTree),
			ratioText(s.queryOverFull),
			s.bytes,
			s.bucket,
		)
	}

	fmt.Println()
	fmt.Println("## Go Full Attribution")
	fmt.Println()
	fmt.Println("| lang | go_full | cgo_full | go_no_tree | parser_loop | token_next | action_lookup | glr_merge | result_build | compat | gss/token | merge/token | public_mat/token | drains/token |")
	fmt.Println("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
	for _, s := range scores {
		fmt.Printf("| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %.3f | %.3f | %.3f | %.3f |\n",
			s.lang,
			nsText(s.goFull),
			nsText(s.cgoFull),
			nsText(s.goNoTree),
			pctText(s.attrs["parser_loop_share"]),
			pctText(s.attrs["token_next_share"]),
			pctText(s.attrs["action_lookup_share"]),
			pctText(s.attrs["glr_merge_share"]),
			pctText(s.attrs["result_build_share"]),
			pctText(s.attrs["result_compat_share"]),
			s.attrs["gss_per_token"],
			s.attrs["merge_calls_per_token"],
			s.attrs["public_materialized_per_token"],
			s.attrs["final_child_drains_per_token"],
		)
	}
	printActionAttribution(scores)
	printReduceAttribution(scores)
	printEquivAttribution(scores)
	printNoTreeAttribution(scores)

	type corr struct {
		name string
		r    float64
		n    int
	}
	var corrs []corr
	names := attrNames(scores)
	for _, name := range names {
		var xs, ys []float64
		for _, s := range scores {
			x, ok := s.attrs[name]
			if !ok || !finite(x) || !finite(s.fullRatio) || s.fullRatio == 0 {
				continue
			}
			xs = append(xs, x)
			ys = append(ys, s.fullRatio)
		}
		if len(xs) < 3 {
			continue
		}
		corrs = append(corrs, corr{name: name, r: pearson(xs, ys), n: len(xs)})
	}
	sort.Slice(corrs, func(i, j int) bool {
		ai, aj := math.Abs(corrs[i].r), math.Abs(corrs[j].r)
		if ai == aj {
			return corrs[i].name < corrs[j].name
		}
		return ai > aj
	})
	fmt.Println()
	fmt.Println("## Strongest Directional Correlations")
	fmt.Println()
	fmt.Println("| attribute | r(full/cgo) | n |")
	fmt.Println("| --- | ---: | ---: |")
	limit := min(len(corrs), 16)
	for i := 0; i < limit; i++ {
		fmt.Printf("| %s | %.3f | %d |\n", corrs[i].name, corrs[i].r, corrs[i].n)
	}

	if hasHotShapes(scores) {
		fmt.Println()
		fmt.Println("## Hot GLR Shapes")
		fmt.Println()
		for _, s := range scores {
			if len(s.hotAmbiguities) == 0 && len(s.hotReduceChains) == 0 && len(s.hotReduceChainRuns) == 0 && len(s.hotMergeStates) == 0 && len(s.hotEquivStates) == 0 {
				continue
			}
			fmt.Printf("### %s\n\n", s.lang)
			printHotTable("fork/action buckets", s.hotAmbiguities, "action_ns")
			printHotTable("reduce-chain buckets", s.hotReduceChains, "reduce_chain_ns")
			printHotReduceChainRunTable("reduce-chain run buckets", s.hotReduceChainRuns)
			printHotTable("merge-state buckets", s.hotMergeStates, "merge_in")
			printHotEquivTable("equivalence-state buckets", s.hotEquivStates)
		}
	}
	printActiveReduceChainHints(scores)
	printReduceChainHintCandidates(scores)

	fmt.Println()
	fmt.Println("## Attention Buckets")
	fmt.Println()
	buckets := map[string][]string{}
	for _, s := range scores {
		buckets[s.bucket] = append(buckets[s.bucket], s.lang)
	}
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		sort.Strings(buckets[key])
		fmt.Printf("- `%s`: %s\n", key, strings.Join(buckets[key], ", "))
	}
}

func printActionAttribution(scores []langScore) {
	if !hasAnyAttr(scores,
		"action_single_reduce_share",
		"action_single_shift_share",
		"action_conflict_fork_share",
		"action_conflict_choice_share",
		"action_no_action_share",
		"action_extra_shift_share",
	) {
		return
	}
	fmt.Println()
	fmt.Println("## Action Dispatch Attribution")
	fmt.Println()
	fmt.Println("| lang | dispatch | lookup | single_reduce | single_shift | conflict_fork | conflict_choice | no_action | extra_shift |")
	fmt.Println("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
	for _, s := range scores {
		fmt.Printf("| %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			s.lang,
			pctText(s.attrs["action_dispatch_share"]),
			pctText(s.attrs["action_lookup_share"]),
			pctText(s.attrs["action_single_reduce_share"]),
			pctText(s.attrs["action_single_shift_share"]),
			pctText(s.attrs["action_conflict_fork_share"]),
			pctText(s.attrs["action_conflict_choice_share"]),
			pctText(s.attrs["action_no_action_share"]),
			pctText(s.attrs["action_extra_shift_share"]),
		)
	}
}

func printReduceAttribution(scores []langScore) {
	if !hasAnyAttr(scores,
		"reduce_range_share",
		"reduce_child_build_share",
		"reduce_parent_build_share",
		"reduce_span_share",
		"reduce_stack_push_share",
		"reduce_notree_build_share",
	) {
		return
	}
	fmt.Println()
	fmt.Println("## Reduce Subphase Attribution")
	fmt.Println()
	fmt.Println("| lang | range | child_build | parent_build | span | stack_push | notree_build |")
	fmt.Println("| --- | ---: | ---: | ---: | ---: | ---: | ---: |")
	for _, s := range scores {
		fmt.Printf("| %s | %s | %s | %s | %s | %s | %s |\n",
			s.lang,
			pctText(s.attrs["reduce_range_share"]),
			pctText(s.attrs["reduce_child_build_share"]),
			pctText(s.attrs["reduce_parent_build_share"]),
			pctText(s.attrs["reduce_span_share"]),
			pctText(s.attrs["reduce_stack_push_share"]),
			pctText(s.attrs["reduce_notree_build_share"]),
		)
	}
}

func printEquivAttribution(scores []langScore) {
	if !hasAnyAttr(scores,
		"equiv_cache_lookups_per_token",
		"equiv_exact_calls_per_token",
		"equiv_frontier_calls_per_token",
		"equiv_frontier_child_scans_per_token",
	) {
		return
	}
	fmt.Println()
	fmt.Println("## Equivalence Attribution")
	fmt.Println()
	fmt.Println("| lang | lookups/token | hit | miss | stores/token | epoch_miss | key_miss | version_miss | exact_calls/token | exact_true | frontier_calls/token | frontier_true | exact_child/token | frontier_scan/token | frontier_candidate/token |")
	fmt.Println("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
	for _, s := range scores {
		fmt.Printf("| %s | %.3f | %s | %s | %.3f | %s | %s | %s | %.3f | %s | %.3f | %s | %.3f | %.3f | %.3f |\n",
			s.lang,
			s.attrs["equiv_cache_lookups_per_token"],
			pctText(s.attrs["equiv_cache_hit_share"]),
			pctText(s.attrs["equiv_cache_miss_share"]),
			s.attrs["equiv_cache_stores_per_token"],
			pctText(s.attrs["equiv_cache_epoch_miss_share"]),
			pctText(s.attrs["equiv_cache_key_miss_share"]),
			pctText(s.attrs["equiv_cache_version_miss_share"]),
			s.attrs["equiv_exact_calls_per_token"],
			pctText(s.attrs["equiv_exact_true_share"]),
			s.attrs["equiv_frontier_calls_per_token"],
			pctText(s.attrs["equiv_frontier_true_share"]),
			s.attrs["equiv_exact_child_compares_per_token"],
			s.attrs["equiv_frontier_child_scans_per_token"],
			s.attrs["equiv_frontier_candidate_compares_per_token"],
		)
	}
}

func printNoTreeAttribution(scores []langScore) {
	if !hasAnyNoTreeRuntime(scores) {
		return
	}
	fmt.Println()
	fmt.Println("## Go No-Tree Attribution")
	fmt.Println()
	fmt.Println("| lang | go_no_tree | parser_loop | token_next | action_lookup | glr_merge | dispatch | single_reduce | conflict_fork | merge/token | reduce_steps/token | class_hits/token | stop_shift/token | equiv_lookup/token |")
	fmt.Println("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
	for _, s := range scores {
		r := s.goNoTreeRuntime
		wall := float64(maxInt64(r.ParseWallNS, s.goNoTree))
		tokens := float64(max64(r.Tokens, 1))
		fmt.Printf("| %s | %s | %s | %s | %s | %s | %s | %s | %s | %.3f | %.3f | %.3f | %.3f | %.3f |\n",
			s.lang,
			nsText(s.goNoTree),
			pctText(float64(r.ParserLoopNS)/wall),
			pctText(float64(r.TokenNextNS)/wall),
			pctText(float64(r.ActionLookupNS)/wall),
			pctText(float64(r.GLRMergeNS)/wall),
			pctText(float64(r.ActionDispatchNS)/wall),
			pctText(float64(r.ActionSingleReduceNS)/wall),
			pctText(float64(r.ActionConflictForkNS)/wall),
			float64(r.MergeCalls)/tokens,
			float64(r.ReduceChainSteps)/tokens,
			float64(r.ReduceChainClassHits)/tokens,
			float64(r.ReduceChainStopShift)/tokens,
			float64(r.EquivCacheLookups)/tokens,
		)
	}
}

func hasAnyNoTreeRuntime(scores []langScore) bool {
	for _, s := range scores {
		if s.goNoTreeRuntime.ParseWallNS != 0 || s.goNoTreeRuntime.Tokens != 0 {
			return true
		}
	}
	return false
}

func hasAnyAttr(scores []langScore, names ...string) bool {
	for _, s := range scores {
		for _, name := range names {
			if v, ok := s.attrs[name]; ok && v != 0 {
				return true
			}
		}
	}
	return false
}

func topHotStates(in []hotGLRState, score func(h hotGLRState) uint64, limit int) []hotGLRState {
	if len(in) == 0 || limit == 0 {
		return nil
	}
	out := aggregateHotStates(in)
	sort.Slice(out, func(i, j int) bool {
		di, dj := score(out[i]), score(out[j])
		if di == dj {
			if out[i].State == out[j].State {
				return out[i].Lookahead < out[j].Lookahead
			}
			return out[i].State < out[j].State
		}
		return di > dj
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func aggregateHotStates(in []hotGLRState) []hotGLRState {
	type key struct {
		state                    uint32
		lookahead                uint16
		actionCount              uint8
		shiftCount               uint8
		reduceCount              uint8
		reduceSymbol             uint16
		childCount               uint8
		productionID             uint16
		reduceChainTerminalState uint32
		reduceChainTerminalClass uint8
	}
	byKey := map[key]*hotGLRState{}
	for _, h := range in {
		k := key{h.State, h.Lookahead, h.ActionCount, h.ShiftCount, h.ReduceCount, h.ReduceSymbol, h.ChildCount, h.ProductionID, h.ReduceChainTerminalState, h.ReduceChainTerminalActionClass}
		dst := byKey[k]
		if dst == nil {
			copied := h
			byKey[k] = &copied
			continue
		}
		dst.Hits += h.Hits
		dst.Forks += h.Forks
		dst.MultiStackHits += h.MultiStackHits
		dst.StackInTotal += h.StackInTotal
		if h.StackInMax > dst.StackInMax {
			dst.StackInMax = h.StackInMax
		}
		dst.ReduceChainHits += h.ReduceChainHits
		dst.ReduceChainSteps += h.ReduceChainSteps
		dst.ReduceChainNS += h.ReduceChainNS
		dst.ReduceChainRuns += h.ReduceChainRuns
		dst.ReduceChainClassHits += h.ReduceChainClassHits
		dst.ReduceChainStopNoAction += h.ReduceChainStopNoAction
		dst.ReduceChainStopMulti += h.ReduceChainStopMulti
		dst.ReduceChainStopShift += h.ReduceChainStopShift
		dst.ReduceChainStopAccept += h.ReduceChainStopAccept
		dst.ReduceChainStopDead += h.ReduceChainStopDead
		dst.ReduceChainStopCycle += h.ReduceChainStopCycle
		dst.ReduceChainStopLimit += h.ReduceChainStopLimit
		dst.ActionNS += h.ActionNS
		dst.ExtraShiftNS += h.ExtraShiftNS
		dst.NoActionNS += h.NoActionNS
		dst.ConflictChoiceNS += h.ConflictChoiceNS
		dst.ConflictForkNS += h.ConflictForkNS
		dst.SingleShiftNS += h.SingleShiftNS
		dst.SingleReduceNS += h.SingleReduceNS
		dst.SingleAcceptNS += h.SingleAcceptNS
		dst.SingleRecoverNS += h.SingleRecoverNS
		dst.SingleOtherNS += h.SingleOtherNS
		if h.ReduceChainMaxLen > dst.ReduceChainMaxLen {
			dst.ReduceChainMaxLen = h.ReduceChainMaxLen
		}
		dst.MergeCalls += h.MergeCalls
		dst.MergeStacksIn += h.MergeStacksIn
		dst.MergeStacksOut += h.MergeStacksOut
		if h.MergeStacksInMax > dst.MergeStacksInMax {
			dst.MergeStacksInMax = h.MergeStacksInMax
		}
		if h.MergeStacksOutMax > dst.MergeStacksOutMax {
			dst.MergeStacksOutMax = h.MergeStacksOutMax
		}
		dst.EquivCacheLookups += h.EquivCacheLookups
		dst.EquivCacheHits += h.EquivCacheHits
		dst.EquivCacheStores += h.EquivCacheStores
		dst.EquivCacheMisses += h.EquivCacheMisses
		dst.EquivCacheEpochMisses += h.EquivCacheEpochMisses
		dst.EquivCacheKeyMisses += h.EquivCacheKeyMisses
		dst.EquivCacheVersionMisses += h.EquivCacheVersionMisses
		dst.EquivSkipError += h.EquivSkipError
		dst.EquivSkipLeaf += h.EquivSkipLeaf
		dst.EquivSkipFieldMismatch += h.EquivSkipFieldMismatch
		dst.EquivExactCalls += h.EquivExactCalls
		dst.EquivExactTrue += h.EquivExactTrue
		dst.EquivFrontierCalls += h.EquivFrontierCalls
		dst.EquivFrontierTrue += h.EquivFrontierTrue
		dst.EquivExactChildCompares += h.EquivExactChildCompares
		dst.EquivFrontierChildScans += h.EquivFrontierChildScans
		dst.EquivFrontierCandidateCompares += h.EquivFrontierCandidateCompares
	}
	out := make([]hotGLRState, 0, len(byKey))
	for _, h := range byKey {
		out = append(out, *h)
	}
	return out
}

func hasHotShapes(scores []langScore) bool {
	for _, s := range scores {
		if len(s.hotAmbiguities) > 0 || len(s.hotReduceChains) > 0 || len(s.hotReduceChainRuns) > 0 || len(s.hotMergeStates) > 0 || len(s.hotEquivStates) > 0 {
			return true
		}
	}
	return false
}

func printHotTable(label string, rows []hotGLRState, metric string) {
	if len(rows) == 0 {
		return
	}
	fmt.Printf("`%s`\n\n", label)
	fmt.Println("| state | lookahead | lookahead_name | prod | reduce_symbol | actions | shifts | reduces | action_ns | single_reduce | conflict_fork | reduce_chain_ns | forks | multi_stack | stack_in | reduce_steps | merge_in | merge_out | metric |")
	fmt.Println("| ---: | ---: | --- | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
	for _, h := range rows {
		var score uint64
		switch metric {
		case "action_ns":
			if h.ActionNS > 0 {
				score = uint64(h.ActionNS)
			} else {
				score = h.StackInTotal
			}
		case "reduce_chain_ns":
			if h.ReduceChainNS > 0 {
				score = uint64(h.ReduceChainNS)
			} else {
				score = h.ReduceChainSteps
			}
		case "reduce_steps":
			score = h.ReduceChainSteps
		case "merge_in":
			score = h.MergeStacksIn
		default:
			score = h.StackInTotal
		}
		scoreText := fmt.Sprintf("%d", score)
		if metric == "action_ns" || metric == "reduce_chain_ns" {
			scoreText = nsText(int64(score))
		}
		fmt.Printf("| %d | %d | `%s` | %d | `%s` | %d | %d | %d | %s | %s | %s | %s | %d | %d | %d | %d | %d | %d | %s |\n",
			h.State,
			h.Lookahead,
			escapePipes(h.LookaheadName),
			h.ProductionID,
			escapePipes(h.ReduceSymbolName),
			h.ActionCount,
			h.ShiftCount,
			h.ReduceCount,
			nsText(h.ActionNS),
			nsText(h.SingleReduceNS),
			nsText(h.ConflictForkNS),
			nsText(h.ReduceChainNS),
			h.Forks,
			h.MultiStackHits,
			h.StackInTotal,
			h.ReduceChainSteps,
			h.MergeStacksIn,
			h.MergeStacksOut,
			scoreText,
		)
	}
	fmt.Println()
}

func printHotReduceChainRunTable(label string, rows []hotGLRState) {
	if len(rows) == 0 {
		return
	}
	fmt.Printf("`%s`\n\n", label)
	fmt.Println("| state | lookahead | lookahead_name | terminal_state | terminal_action | runs | steps | class_hits | avg_len | max_len | ns | stop_shift | stop_multi | stop_no_action | stop_accept | stop_dead | stop_cycle | stop_limit |")
	fmt.Println("| ---: | ---: | --- | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
	for _, h := range rows {
		terminalAction := h.ReduceChainTerminalActionName
		if terminalAction == "" && h.ReduceChainTerminalActionClass != 0 {
			terminalAction = classifiedActionClassName(h.ReduceChainTerminalActionClass)
		}
		fmt.Printf("| %d | %d | `%s` | %d | `%s` | %d | %d | %d | %.2f | %d | %s | %d | %d | %d | %d | %d | %d | %d |\n",
			h.State,
			h.Lookahead,
			escapePipes(h.LookaheadName),
			h.ReduceChainTerminalState,
			escapePipes(terminalAction),
			h.ReduceChainRuns,
			h.ReduceChainSteps,
			h.ReduceChainClassHits,
			safeRatioUint(h.ReduceChainSteps, h.ReduceChainRuns),
			h.ReduceChainMaxLen,
			nsText(h.ReduceChainNS),
			h.ReduceChainStopShift,
			h.ReduceChainStopMulti,
			h.ReduceChainStopNoAction,
			h.ReduceChainStopAccept,
			h.ReduceChainStopDead,
			h.ReduceChainStopCycle,
			h.ReduceChainStopLimit,
		)
	}
	fmt.Println()
}

type reduceChainHintCandidate struct {
	lang           string
	state          uint32
	lookahead      uint16
	lookaheadName  string
	terminalAction string
	terminalStates []uint32
	runs           uint64
	steps          uint64
	classHits      uint64
	maxLen         int
	ns             int64
}

type activeReduceChainHint struct {
	lang       string
	mode       string
	taken      uint64
	steps      uint64
	ok         uint64
	mismatch   uint64
	limit      uint64
	dead       uint64
	unexpected uint64
}

func printActiveReduceChainHints(scores []langScore) {
	active := collectActiveReduceChainHints(scores)
	if len(active) == 0 {
		return
	}
	sort.Slice(active, func(i, j int) bool {
		if active[i].taken == active[j].taken {
			if active[i].lang == active[j].lang {
				return active[i].mode < active[j].mode
			}
			return active[i].lang < active[j].lang
		}
		return active[i].taken > active[j].taken
	})
	fmt.Println()
	fmt.Println("## Active Reduce-Chain Hints")
	fmt.Println()
	fmt.Println("| lang | mode | taken | steps | terminal_ok | ok% | mismatch | limit | dead | unexpected |")
	fmt.Println("| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
	for _, h := range active {
		fmt.Printf("| %s | `%s` | %d | %d | %d | %.1f%% | %d | %d | %d | %d |\n",
			h.lang,
			h.mode,
			h.taken,
			h.steps,
			h.ok,
			100*safeRatioUint(h.ok, h.taken),
			h.mismatch,
			h.limit,
			h.dead,
			h.unexpected,
		)
	}
}

func collectActiveReduceChainHints(scores []langScore) []activeReduceChainHint {
	out := []activeReduceChainHint{}
	for _, s := range scores {
		for _, mode := range []struct {
			name string
			r    runtimeStats
		}{
			{name: "go_full", r: s.goFullRuntime},
			{name: "go_no_tree", r: s.goNoTreeRuntime},
			{name: "go_parse_query", r: s.goQueryRuntime},
		} {
			if mode.r.ReduceChainHintTaken == 0 {
				continue
			}
			out = append(out, activeReduceChainHint{
				lang:       s.lang,
				mode:       mode.name,
				taken:      mode.r.ReduceChainHintTaken,
				steps:      mode.r.ReduceChainHintSteps,
				ok:         mode.r.ReduceChainHintTerminalOK,
				mismatch:   mode.r.ReduceChainHintTerminalMismatch,
				limit:      mode.r.ReduceChainHintLimit,
				dead:       mode.r.ReduceChainHintDead,
				unexpected: mode.r.ReduceChainHintUnexpected,
			})
		}
	}
	return out
}

func printReduceChainHintCandidates(scores []langScore) {
	candidates := collectReduceChainHintCandidates(scores)
	if len(candidates) == 0 {
		return
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].ns == candidates[j].ns {
			if candidates[i].steps == candidates[j].steps {
				if candidates[i].lang == candidates[j].lang {
					return candidates[i].state < candidates[j].state
				}
				return candidates[i].lang < candidates[j].lang
			}
			return candidates[i].steps > candidates[j].steps
		}
		return candidates[i].ns > candidates[j].ns
	})
	if len(candidates) > 16 {
		candidates = candidates[:16]
	}
	fmt.Println()
	fmt.Println("## Reduce-Chain Hint Candidates")
	fmt.Println()
	fmt.Println("| lang | state | lookahead | lookahead_name | terminal_action | terminal_states | runs | steps | class_hits | avg_len | max_len | ns |")
	fmt.Println("| --- | ---: | ---: | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |")
	for _, c := range candidates {
		fmt.Printf("| %s | %d | %d | `%s` | `%s` | `%s` | %d | %d | %d | %.2f | %d | %s |\n",
			c.lang,
			c.state,
			c.lookahead,
			escapePipes(c.lookaheadName),
			escapePipes(c.terminalAction),
			uint32ListText(c.terminalStates),
			c.runs,
			c.steps,
			c.classHits,
			safeRatioUint(c.steps, c.runs),
			c.maxLen,
			nsText(c.ns),
		)
	}
}

func collectReduceChainHintCandidates(scores []langScore) []reduceChainHintCandidate {
	type key struct {
		lang              string
		state             uint32
		lookahead         uint16
		terminalAction    string
		terminalActionCls uint8
	}
	type agg struct {
		reduceChainHintCandidate
		terminalSeen map[uint32]struct{}
	}
	byKey := map[key]*agg{}
	for _, s := range scores {
		for _, h := range s.hintReduceChainRuns {
			if h.ReduceChainRuns == 0 || h.ReduceChainSteps == 0 {
				continue
			}
			if h.ReduceChainStopShift != h.ReduceChainRuns {
				continue
			}
			if h.ReduceChainStopMulti != 0 || h.ReduceChainStopNoAction != 0 ||
				h.ReduceChainStopAccept != 0 || h.ReduceChainStopDead != 0 ||
				h.ReduceChainStopCycle != 0 || h.ReduceChainStopLimit != 0 {
				continue
			}
			terminalAction := h.ReduceChainTerminalActionName
			if terminalAction == "" && h.ReduceChainTerminalActionClass != 0 {
				terminalAction = classifiedActionClassName(h.ReduceChainTerminalActionClass)
			}
			if terminalAction != "single_shift" {
				continue
			}
			k := key{
				lang:              s.lang,
				state:             h.State,
				lookahead:         h.Lookahead,
				terminalAction:    terminalAction,
				terminalActionCls: h.ReduceChainTerminalActionClass,
			}
			dst := byKey[k]
			if dst == nil {
				dst = &agg{
					reduceChainHintCandidate: reduceChainHintCandidate{
						lang:           s.lang,
						state:          h.State,
						lookahead:      h.Lookahead,
						lookaheadName:  h.LookaheadName,
						terminalAction: terminalAction,
					},
					terminalSeen: map[uint32]struct{}{},
				}
				byKey[k] = dst
			}
			dst.runs += h.ReduceChainRuns
			dst.steps += h.ReduceChainSteps
			dst.classHits += h.ReduceChainClassHits
			dst.ns += h.ReduceChainNS
			if h.ReduceChainMaxLen > dst.maxLen {
				dst.maxLen = h.ReduceChainMaxLen
			}
			if _, ok := dst.terminalSeen[h.ReduceChainTerminalState]; !ok {
				dst.terminalSeen[h.ReduceChainTerminalState] = struct{}{}
				dst.terminalStates = append(dst.terminalStates, h.ReduceChainTerminalState)
			}
		}
	}
	out := make([]reduceChainHintCandidate, 0, len(byKey))
	for _, candidate := range byKey {
		if candidate.runs < 8 || candidate.steps < 16 || candidate.maxLen > 32 {
			continue
		}
		sort.Slice(candidate.terminalStates, func(i, j int) bool {
			return candidate.terminalStates[i] < candidate.terminalStates[j]
		})
		out = append(out, candidate.reduceChainHintCandidate)
	}
	return out
}

func printHotEquivTable(label string, rows []hotGLRState) {
	if len(rows) == 0 {
		return
	}
	fmt.Printf("`%s`\n\n", label)
	fmt.Println("| state | lookups | hit% | miss% | key_miss% | exact_calls | exact_true | frontier_calls | frontier_true | exact_child | frontier_scans | frontier_candidates | skips | score |")
	fmt.Println("| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
	for _, h := range rows {
		skips := h.EquivSkipError + h.EquivSkipLeaf + h.EquivSkipFieldMismatch
		score := h.EquivCacheLookups + h.EquivExactCalls + h.EquivFrontierCalls
		fmt.Printf("| %d | %d | %.1f%% | %.1f%% | %.1f%% | %d | %.1f%% | %d | %.1f%% | %d | %d | %d | %d | %d |\n",
			h.State,
			h.EquivCacheLookups,
			100*safeRatioUint(h.EquivCacheHits, h.EquivCacheLookups),
			100*safeRatioUint(h.EquivCacheMisses, h.EquivCacheLookups),
			100*safeRatioUint(h.EquivCacheKeyMisses, h.EquivCacheMisses),
			h.EquivExactCalls,
			100*safeRatioUint(h.EquivExactTrue, h.EquivExactCalls),
			h.EquivFrontierCalls,
			100*safeRatioUint(h.EquivFrontierTrue, h.EquivFrontierCalls),
			h.EquivExactChildCompares,
			h.EquivFrontierChildScans,
			h.EquivFrontierCandidateCompares,
			skips,
			score,
		)
	}
	fmt.Println()
}

func escapePipes(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

func uint32ListText(values []uint32) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("%d", value))
	}
	return strings.Join(parts, ",")
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

func attrNames(scores []langScore) []string {
	seen := map[string]struct{}{}
	for _, s := range scores {
		for name := range s.attrs {
			seen[name] = struct{}{}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func pearson(xs, ys []float64) float64 {
	if len(xs) != len(ys) || len(xs) < 2 {
		return 0
	}
	var sx, sy float64
	for i := range xs {
		sx += xs[i]
		sy += ys[i]
	}
	mx := sx / float64(len(xs))
	my := sy / float64(len(ys))
	var num, dx, dy float64
	for i := range xs {
		x := xs[i] - mx
		y := ys[i] - my
		num += x * y
		dx += x * x
		dy += y * y
	}
	if dx == 0 || dy == 0 {
		return 0
	}
	return num / math.Sqrt(dx*dy)
}

func (r *runtimeStats) add(o runtimeStats) {
	r.Tokens += o.Tokens
	r.NodesAllocated += o.NodesAllocated
	r.FinalNodes += o.FinalNodes
	r.GSSNodes += o.GSSNodes
	if o.MaxStacksSeen > r.MaxStacksSeen {
		r.MaxStacksSeen = o.MaxStacksSeen
	}
	r.SingleStackTokens += o.SingleStackTokens
	r.MultiStackTokens += o.MultiStackTokens
	r.MergeStacksIn += o.MergeStacksIn
	r.MergeStacksOut += o.MergeStacksOut
	r.ArenaLiveB += o.ArenaLiveB
	r.ArenaCapacityB += o.ArenaCapacityB
	r.ArenaCapacityWaste += o.ArenaCapacityWaste
	r.FinalChildRangeDrains += o.FinalChildRangeDrains
	r.PublicNodesMaterialized += o.PublicNodesMaterialized
	r.DenseFallbacks += o.DenseFallbacks
	r.ResultSelectionNS += o.ResultSelectionNS
	r.ResultBuildNS += o.ResultBuildNS
	r.ResultCompatibilityNS += o.ResultCompatibilityNS
	r.ResultParentLinkNS += o.ResultParentLinkNS
	r.ResultFinalizeRootNS += o.ResultFinalizeRootNS
	r.ResultExtendTrailingNS += o.ResultExtendTrailingNS
	r.NormalizationNS += o.NormalizationNS
	r.NormalizationNodes += o.NormalizationNodes
	r.NormalizationRewrites += o.NormalizationRewrites
	r.ParseWallNS += o.ParseWallNS
	r.ParserLoopNS += o.ParserLoopNS
	r.TokenNextNS += o.TokenNextNS
	r.ActionDispatchNS += o.ActionDispatchNS
	r.ActionLookupNS += o.ActionLookupNS
	r.GLRMergeNS += o.GLRMergeNS
	r.GLRCullNS += o.GLRCullNS
	r.ReduceRangeNS += o.ReduceRangeNS
	r.ReducePendingParentNS += o.ReducePendingParentNS
	r.ReduceChildBuildNS += o.ReduceChildBuildNS
	r.ReduceParentBuildNS += o.ReduceParentBuildNS
	r.ReduceSpanNS += o.ReduceSpanNS
	r.ReduceStackPushNS += o.ReduceStackPushNS
	r.ReduceNoTreeBuildNS += o.ReduceNoTreeBuildNS
	r.ActionExtraShiftNS += o.ActionExtraShiftNS
	r.ActionNoActionNS += o.ActionNoActionNS
	r.ActionNoActionRelexNS += o.ActionNoActionRelexNS
	r.ActionNoActionMissingNS += o.ActionNoActionMissingNS
	r.ActionNoActionRecoverNS += o.ActionNoActionRecoverNS
	r.ActionNoActionErrorNS += o.ActionNoActionErrorNS
	r.ActionConflictChoiceNS += o.ActionConflictChoiceNS
	r.ActionConflictForkNS += o.ActionConflictForkNS
	r.ActionSingleShiftNS += o.ActionSingleShiftNS
	r.ActionSingleReduceNS += o.ActionSingleReduceNS
	r.ActionSingleAcceptNS += o.ActionSingleAcceptNS
	r.ActionSingleRecoverNS += o.ActionSingleRecoverNS
	r.ActionSingleOtherNS += o.ActionSingleOtherNS
	r.MergeCalls += o.MergeCalls
	r.EquivCacheLookups += o.EquivCacheLookups
	r.EquivCacheHits += o.EquivCacheHits
	r.EquivCacheStores += o.EquivCacheStores
	r.EquivCacheMisses += o.EquivCacheMisses
	r.EquivCacheEpochMisses += o.EquivCacheEpochMisses
	r.EquivCacheKeyMisses += o.EquivCacheKeyMisses
	r.EquivCacheVersionMisses += o.EquivCacheVersionMisses
	r.EquivSkipError += o.EquivSkipError
	r.EquivSkipLeaf += o.EquivSkipLeaf
	r.EquivSkipFieldMismatch += o.EquivSkipFieldMismatch
	r.EquivExactCalls += o.EquivExactCalls
	r.EquivExactTrue += o.EquivExactTrue
	r.EquivFrontierCalls += o.EquivFrontierCalls
	r.EquivFrontierTrue += o.EquivFrontierTrue
	r.EquivExactChildCompares += o.EquivExactChildCompares
	r.EquivFrontierChildScans += o.EquivFrontierChildScans
	r.EquivFrontierCandidateCompares += o.EquivFrontierCandidateCompares
	r.ForkCount += o.ForkCount
	r.ConflictRR += o.ConflictRR
	r.ConflictRS += o.ConflictRS
	r.ConflictOther += o.ConflictOther
	r.LexBytes += o.LexBytes
	r.LexTokens += o.LexTokens
	r.ReduceChainSteps += o.ReduceChainSteps
	r.ReduceChainClassHits += o.ReduceChainClassHits
	r.ReduceChainStopNoAction += o.ReduceChainStopNoAction
	r.ReduceChainStopMulti += o.ReduceChainStopMulti
	r.ReduceChainStopShift += o.ReduceChainStopShift
	r.ReduceChainStopAccept += o.ReduceChainStopAccept
	r.ReduceChainStopDead += o.ReduceChainStopDead
	r.ReduceChainStopCycle += o.ReduceChainStopCycle
	r.ReduceChainStopLimit += o.ReduceChainStopLimit
	r.ReduceChainHintCandidates += o.ReduceChainHintCandidates
	r.ReduceChainHintTaken += o.ReduceChainHintTaken
	r.ReduceChainHintSteps += o.ReduceChainHintSteps
	r.ReduceChainHintTerminalOK += o.ReduceChainHintTerminalOK
	r.ReduceChainHintTerminalMismatch += o.ReduceChainHintTerminalMismatch
	r.ReduceChainHintLimit += o.ReduceChainHintLimit
	r.ReduceChainHintDead += o.ReduceChainHintDead
	r.ReduceChainHintUnexpected += o.ReduceChainHintUnexpected
	if o.ReduceChainMaxLen > r.ReduceChainMaxLen {
		r.ReduceChainMaxLen = o.ReduceChainMaxLen
	}
	r.NoTreeReduceNodes += o.NoTreeReduceNodes
	r.NoTreeLeafNodes += o.NoTreeLeafNodes
	r.HotAmbiguities = append(r.HotAmbiguities, o.HotAmbiguities...)
	r.HotReduceChains = append(r.HotReduceChains, o.HotReduceChains...)
	r.HotReduceChainRuns = append(r.HotReduceChainRuns, o.HotReduceChainRuns...)
	r.HotMergeStates = append(r.HotMergeStates, o.HotMergeStates...)
	r.HotEquivStates = append(r.HotEquivStates, o.HotEquivStates...)
}

func ratio(num, den int64) float64 {
	if num <= 0 || den <= 0 {
		return 0
	}
	return float64(num) / float64(den)
}

func safeRatioUint(num, den uint64) float64 {
	if den == 0 {
		return 0
	}
	return float64(num) / float64(den)
}

func ratioText(v float64) string {
	if v <= 0 || !finite(v) {
		return "n/a"
	}
	return fmt.Sprintf("%.2fx", v)
}

func nsText(ns int64) string {
	if ns <= 0 {
		return "n/a"
	}
	if ns >= 1_000_000 {
		return fmt.Sprintf("%.2fms", float64(ns)/1_000_000)
	}
	if ns >= 1_000 {
		return fmt.Sprintf("%.2fus", float64(ns)/1_000)
	}
	return fmt.Sprintf("%dns", ns)
}

func pctText(v float64) string {
	if v <= 0 || !finite(v) {
		return "n/a"
	}
	return fmt.Sprintf("%.1f%%", v*100)
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func max64(v, floor uint64) uint64 {
	if v < floor {
		return floor
	}
	return v
}

func maxInt64(v, floor int64) int64 {
	if v < floor {
		return floor
	}
	return v
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
