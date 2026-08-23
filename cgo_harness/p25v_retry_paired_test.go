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

const p25vRetryBypassEnv = "GOT_DIAGNOSTIC_SKIP_ACCEPTED_ERROR_BASE_MERGE_RETRY"

type p25vSpan struct {
	Start uint32 `json:"start_byte"`
	End   uint32 `json:"end_byte"`
	Kind  string `json:"kind"`
}

type p25vRoot struct {
	StartByte  uint32    `json:"start_byte"`
	EndByte    uint32    `json:"end_byte"`
	HasError   bool      `json:"has_error"`
	ChildCount int       `json:"child_count"`
	FirstError *p25vSpan `json:"first_error,omitempty"`
}

type p25vRuntime struct {
	StopReason                          string `json:"stop_reason"`
	Truncated                           bool   `json:"truncated"`
	TokenSourceEOFEarly                 bool   `json:"token_source_eof_early"`
	RootEndByte                         uint32 `json:"root_end_byte"`
	RetryAttempts                       uint8  `json:"retry_attempts"`
	RetryAdopted                        bool   `json:"retry_adopted"`
	RetryMergePerKey                    int    `json:"retry_merge_per_key"`
	RetryCause                          string `json:"retry_cause"`
	OldTreeReuseRoute                   bool   `json:"old_tree_reuse_route"`
	CRecoveryEnteredErrorState          bool   `json:"c_recovery_entered_error_state"`
	CRecoveryDroppedErrorForClean       bool   `json:"c_recovery_dropped_error_for_clean"`
	CRecoverySwallowedFallbackAttempted bool   `json:"c_recovery_swallowed_fallback_attempted"`
	TokensConsumed                      uint64 `json:"tokens_consumed"`
	Iterations                          int    `json:"iterations"`
	NodesAllocated                      int    `json:"nodes_allocated"`
	MaxStacksSeen                       int    `json:"max_stacks_seen"`
	SingleStackIterations               int    `json:"single_stack_iterations"`
	MultiStackIterations                int    `json:"multi_stack_iterations"`
	GSSNodesAllocated                   uint64 `json:"gss_nodes_allocated"`
	GSSNodesRetained                    uint64 `json:"gss_nodes_retained"`
	GSSNodesDropped                     uint64 `json:"gss_nodes_dropped_same_token"`
	ParentNodesAllocated                uint64 `json:"parent_nodes_allocated"`
	ParentNodesRetained                 uint64 `json:"parent_nodes_retained"`
	ParentNodesDropped                  uint64 `json:"parent_nodes_dropped_same_token"`
	LeafNodesAllocated                  uint64 `json:"leaf_nodes_allocated"`
	LeafNodesRetained                   uint64 `json:"leaf_nodes_retained"`
	LeafNodesDropped                    uint64 `json:"leaf_nodes_dropped_same_token"`
	ArenaBytesAllocated                 int64  `json:"arena_bytes_allocated"`
	ScratchBytesAllocated               int64  `json:"scratch_bytes_allocated"`
	GSSBytesAllocated                   int64  `json:"gss_bytes_allocated"`
	MergeStacksIn                       uint64 `json:"merge_stacks_in"`
	MergeStacksOut                      uint64 `json:"merge_stacks_out"`
	MergeSlotsUsed                      uint64 `json:"merge_slots_used"`
}

type p25vProfile struct {
	ReuseCursorNanos          int64  `json:"reuse_cursor_nanos"`
	ReparseNanos              int64  `json:"reparse_nanos"`
	ReusedSubtrees            uint64 `json:"reused_subtrees"`
	ReusedBytes               uint64 `json:"reused_bytes"`
	NewNodesAllocated         uint64 `json:"new_nodes_allocated"`
	RetryAttempts             uint8  `json:"retry_attempts"`
	RetryAdopted              bool   `json:"retry_adopted"`
	RetryMergePerKey          int    `json:"retry_merge_per_key"`
	RetryCause                string `json:"retry_cause"`
	OldTreeReuseRoute         bool   `json:"old_tree_reuse_route"`
	ReuseRejectDirty          uint64 `json:"reuse_reject_dirty"`
	ReuseRejectHasError       uint64 `json:"reuse_reject_has_error"`
	ReuseRejectFragileNonLeaf uint64 `json:"reuse_reject_fragile_non_leaf"`
	RecoverSearches           uint64 `json:"recover_searches"`
	RecoverStateChecks        uint64 `json:"recover_state_checks"`
	RecoverHits               uint64 `json:"recover_hits"`
	TokensConsumed            uint64 `json:"tokens_consumed"`
	MaxStacksSeen             int    `json:"max_stacks_seen"`
	SingleStackIterations     int    `json:"single_stack_iterations"`
	MultiStackIterations      int    `json:"multi_stack_iterations"`
	GSSNodesAllocated         uint64 `json:"gss_nodes_allocated"`
	GSSNodesRetained          uint64 `json:"gss_nodes_retained"`
	GSSNodesDropped           uint64 `json:"gss_nodes_dropped_same_token"`
	ParentNodesAllocated      uint64 `json:"parent_nodes_allocated"`
	ParentNodesRetained       uint64 `json:"parent_nodes_retained"`
	ParentNodesDropped        uint64 `json:"parent_nodes_dropped_same_token"`
	LeafNodesAllocated        uint64 `json:"leaf_nodes_allocated"`
	LeafNodesRetained         uint64 `json:"leaf_nodes_retained"`
	LeafNodesDropped          uint64 `json:"leaf_nodes_dropped_same_token"`
	ArenaBytesAllocated       int64  `json:"arena_bytes_allocated"`
	ScratchBytesAllocated     int64  `json:"scratch_bytes_allocated"`
	GSSBytesAllocated         int64  `json:"gss_bytes_allocated"`
	MergeStacksIn             uint64 `json:"merge_stacks_in"`
	MergeStacksOut            uint64 `json:"merge_stacks_out"`
	MergeSlotsUsed            uint64 `json:"merge_slots_used"`
	GlobalCullStacksIn        uint64 `json:"global_cull_stacks_in"`
	GlobalCullStacksOut       uint64 `json:"global_cull_stacks_out"`
}

type p25vRoute struct {
	DigestEqualFreshGoC         bool   `json:"digest_equal_fresh_go_c"`
	DigestEqualIncrementalGoC   bool   `json:"digest_equal_incremental_go_c"`
	DigestEqualFreshIncremental bool   `json:"digest_equal_fresh_incremental"`
	AllDeepDigestsEqual         bool   `json:"all_deep_digests_equal"`
	GoFreshDigest               string `json:"go_fresh_digest"`
	CFreshDigest                string `json:"c_fresh_digest"`
	GoIncrementalDigest         string `json:"go_incremental_digest"`
	CIncrementalDigest          string `json:"c_incremental_digest"`
	FreshFirstDifference        string `json:"fresh_first_difference,omitempty"`
	IncrementalFirstDifference  string `json:"incremental_first_difference,omitempty"`
}

type p25vCaseResult struct {
	Name                   string      `json:"name"`
	Role                   string      `json:"role"`
	Mode                   string      `json:"mode"`
	SourceBytes            int         `json:"source_bytes"`
	EditedBytes            int         `json:"edited_bytes"`
	SourceSHA256           string      `json:"source_sha256"`
	EditedSHA256           string      `json:"edited_sha256"`
	GoFreshWallNanos       int64       `json:"go_fresh_wall_nanos"`
	CFreshWallNanos        int64       `json:"c_fresh_wall_nanos"`
	GoIncrementalWallNanos int64       `json:"go_incremental_wall_nanos"`
	CIncrementalWallNanos  int64       `json:"c_incremental_wall_nanos"`
	GoFreshRoot            p25vRoot    `json:"go_fresh_root"`
	CFreshRoot             p25vRoot    `json:"c_fresh_root"`
	GoIncrementalRoot      p25vRoot    `json:"go_incremental_root"`
	CIncrementalRoot       p25vRoot    `json:"c_incremental_root"`
	GoFreshRuntime         p25vRuntime `json:"go_fresh_runtime"`
	GoIncrementalRuntime   p25vRuntime `json:"go_incremental_runtime"`
	GoIncrementalProfile   p25vProfile `json:"go_incremental_profile"`
	Route                  p25vRoute   `json:"route"`
}

type p25vReport struct {
	Schema    string           `json:"schema"`
	Mode      string           `json:"mode"`
	BypassEnv string           `json:"bypass_env"`
	Cases     []p25vCaseResult `json:"cases"`
}

func TestP25vRetryPaired(t *testing.T) {
	bypass := os.Getenv(p25vRetryBypassEnv) == "1"
	mode := "retry-enabled"
	if bypass {
		mode = "retry-bypassed"
	}
	caseNames := []struct {
		name string
		role string
	}{
		{name: "recovery_deletion", role: "authenticated-forward-malformed-recovery"},
		{name: "same_length_leaf_validation", role: "clean-incremental-control"},
		{name: "same_line_length_change", role: "accepted-error-recovery-control"},
	}
	cases := loadCanonicalGoIncrementalCases(t)
	report := p25vReport{Schema: "p25v-retry-paired-v1", Mode: mode, BypassEnv: os.Getenv(p25vRetryBypassEnv)}
	allEqual := true
	for _, want := range caseNames {
		var tc *canonicalGoIncrementalCase
		for i := range cases {
			if cases[i].spec.Name == want.name {
				candidate := cases[i]
				tc = &candidate
				break
			}
		}
		if tc == nil {
			t.Fatalf("canonical case %q is missing", want.name)
		}
		result := runP25vCase(t, tc, want.role, mode)
		report.Cases = append(report.Cases, result)
		if !result.Route.AllDeepDigestsEqual {
			allEqual = false
		}
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal P25v report: %v", err)
	}
	t.Logf("P25V_RESULT %s", encoded)
	if path := os.Getenv("P25V_RESULT_OUT"); path != "" {
		if err := os.WriteFile(path, encoded, 0o644); err != nil {
			t.Fatalf("write P25v report %q: %v", path, err)
		}
	}
	if !allEqual {
		t.Fatalf("P25v %s mode changed a fresh, incremental, or locked-C deep digest", mode)
	}
}

func runP25vCase(t *testing.T, tc *canonicalGoIncrementalCase, role, mode string) p25vCaseResult {
	t.Helper()
	direction := tc.directions()[0]
	goLang := canonicalIncrementalGoLanguage(t, tc.spec.Language)
	cLang := canonicalIncrementalCLanguage(t, tc.spec.Language)
	goParser := gotreesitter.NewParser(goLang)
	cParser := sitter.NewParser()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatalf("%s set C language: %v", tc.spec.Name, err)
	}

	goFreshStart := time.Now()
	goFresh, err := goParser.Parse(direction.to)
	goFreshWall := time.Since(goFreshStart)
	if err != nil {
		t.Fatalf("%s fresh Go parse: %v", tc.spec.Name, err)
	}
	requireCanonicalGoIncrementalTree(t, goFresh, direction.to, tc.spec.Name+" P25v fresh Go", nil)
	goFreshDigest := canonicalGoTreeDigest(t, goFresh, goLang, tc.spec.Name+" P25v fresh Go")

	cFreshStart := time.Now()
	cFresh := cParser.Parse(direction.to, nil)
	cFreshWall := time.Since(cFreshStart)
	requireCanonicalCIncrementalTree(t, cFresh, direction.to, tc.spec.Name+" P25v fresh C")
	cFreshDigest := canonicalCTreeDigest(t, cFresh, tc.spec.Name+" P25v fresh C")

	goOld, err := goParser.Parse(direction.from)
	if err != nil {
		t.Fatalf("%s old Go parse: %v", tc.spec.Name, err)
	}
	requireCanonicalGoIncrementalTree(t, goOld, direction.from, tc.spec.Name+" P25v old Go", nil)
	goOld.Edit(direction.goEdit)
	goIncrementalStart := time.Now()
	goIncremental, profile, err := goParser.ParseIncrementalProfiled(direction.to, goOld)
	goIncrementalWall := time.Since(goIncrementalStart)
	if err != nil {
		t.Fatalf("%s incremental Go parse: %v", tc.spec.Name, err)
	}
	if goIncremental != goOld {
		releaseCanonicalGoTree(goOld)
	}
	requireCanonicalGoIncrementalTree(t, goIncremental, direction.to, tc.spec.Name+" P25v incremental Go", nil)
	goIncrementalDigest := canonicalGoTreeDigest(t, goIncremental, goLang, tc.spec.Name+" P25v incremental Go")

	cOld := cParser.Parse(direction.from, nil)
	requireCanonicalCIncrementalTree(t, cOld, direction.from, tc.spec.Name+" P25v old C")
	cOld.Edit(&direction.cEdit)
	cIncrementalStart := time.Now()
	cIncremental := cParser.Parse(direction.to, cOld)
	cIncrementalWall := time.Since(cIncrementalStart)
	if cIncremental != cOld {
		closeCanonicalCTree(cOld)
	}
	requireCanonicalCIncrementalTree(t, cIncremental, direction.to, tc.spec.Name+" P25v incremental C")
	cIncrementalDigest := canonicalCTreeDigest(t, cIncremental, tc.spec.Name+" P25v incremental C")

	goFreshRoot := p25vGoRoot(goFresh.RootNode())
	cFreshRoot := p25vCRoot(cFresh.RootNode())
	goIncrementalRoot := p25vGoRoot(goIncremental.RootNode())
	cIncrementalRoot := p25vCRoot(cIncremental.RootNode())
	goFreshRuntime := p25vRuntimeFrom(goFresh.ParseRuntime())
	goIncrementalRuntime := p25vRuntimeFrom(goIncremental.ParseRuntime())
	freshFirstDifference := ""
	incrementalFirstDifference := ""
	if diff := FirstDivergenceDumpV1(goFresh.RootNode(), goLang, cFresh.RootNode()); diff != nil {
		freshFirstDifference = formatRealCorpusDivergence(diff)
	}
	if diff := FirstDivergenceDumpV1(goIncremental.RootNode(), goLang, cIncremental.RootNode()); diff != nil {
		incrementalFirstDifference = formatRealCorpusDivergence(diff)
	}
	route := p25vRoute{
		DigestEqualFreshGoC:         goFreshDigest == cFreshDigest,
		DigestEqualIncrementalGoC:   goIncrementalDigest == cIncrementalDigest,
		DigestEqualFreshIncremental: goFreshDigest == goIncrementalDigest,
		AllDeepDigestsEqual:         goFreshDigest == cFreshDigest && goFreshDigest == goIncrementalDigest && goFreshDigest == cIncrementalDigest,
		GoFreshDigest:               goFreshDigest,
		CFreshDigest:                cFreshDigest,
		GoIncrementalDigest:         goIncrementalDigest,
		CIncrementalDigest:          cIncrementalDigest,
		FreshFirstDifference:        freshFirstDifference,
		IncrementalFirstDifference:  incrementalFirstDifference,
	}

	result := p25vCaseResult{
		Name:                   tc.spec.Name,
		Role:                   role,
		Mode:                   mode,
		SourceBytes:            len(direction.from),
		EditedBytes:            len(direction.to),
		SourceSHA256:           direction.fromSHA256(),
		EditedSHA256:           direction.toSHA256(),
		GoFreshWallNanos:       goFreshWall.Nanoseconds(),
		CFreshWallNanos:        cFreshWall.Nanoseconds(),
		GoIncrementalWallNanos: goIncrementalWall.Nanoseconds(),
		CIncrementalWallNanos:  cIncrementalWall.Nanoseconds(),
		GoFreshRoot:            goFreshRoot,
		CFreshRoot:             cFreshRoot,
		GoIncrementalRoot:      goIncrementalRoot,
		CIncrementalRoot:       cIncrementalRoot,
		GoFreshRuntime:         goFreshRuntime,
		GoIncrementalRuntime:   goIncrementalRuntime,
		GoIncrementalProfile:   p25vProfileFrom(profile),
		Route:                  route,
	}

	t.Logf("P25V_CASE mode=%s case=%s role=%s all_deep_equal=%t fresh_go=%s fresh_c=%s incremental_go=%s incremental_c=%s go_inc_wall_ns=%d c_inc_wall_ns=%d retry_attempts=%d retry_adopted=%t retry_cap=%d stop=%s root_error=%t", mode, result.Name, result.Role, route.AllDeepDigestsEqual, goFreshDigest, cFreshDigest, goIncrementalDigest, cIncrementalDigest, result.GoIncrementalWallNanos, result.CIncrementalWallNanos, profile.AcceptedErrorRetryAttempts, profile.AcceptedErrorRetryAdopted, profile.AcceptedErrorRetryMergePerKey, goIncrementalRuntime.StopReason, goIncrementalRoot.HasError)

	releaseCanonicalGoTree(goFresh)
	releaseCanonicalGoTree(goIncremental)
	closeCanonicalCTree(cFresh)
	closeCanonicalCTree(cIncremental)
	cParser.Close()
	return result
}

func (d canonicalGoIncrementalDirection) fromSHA256() string {
	return sha256Hex(d.from)
}

func (d canonicalGoIncrementalDirection) toSHA256() string {
	return sha256Hex(d.to)
}

func p25vGoRoot(root *gotreesitter.Node) p25vRoot {
	if root == nil {
		return p25vRoot{}
	}
	return p25vRoot{StartByte: root.StartByte(), EndByte: root.EndByte(), HasError: root.HasError(), ChildCount: root.ChildCount(), FirstError: p25vFirstGoError(root)}
}

func p25vCRoot(root *sitter.Node) p25vRoot {
	if root == nil {
		return p25vRoot{}
	}
	return p25vRoot{StartByte: uint32(root.StartByte()), EndByte: uint32(root.EndByte()), HasError: root.HasError(), ChildCount: int(root.ChildCount()), FirstError: p25vFirstCError(root)}
}

func p25vFirstGoError(node *gotreesitter.Node) *p25vSpan {
	if node == nil {
		return nil
	}
	if node.IsError() || node.IsMissing() {
		kind := "ERROR"
		if node.IsMissing() {
			kind = "MISSING"
		}
		return &p25vSpan{Start: node.StartByte(), End: node.EndByte(), Kind: kind}
	}
	for i := 0; i < node.ChildCount(); i++ {
		if span := p25vFirstGoError(node.Child(i)); span != nil {
			return span
		}
	}
	return nil
}

func p25vFirstCError(node *sitter.Node) *p25vSpan {
	if node == nil {
		return nil
	}
	if node.IsError() || node.IsMissing() {
		kind := "ERROR"
		if node.IsMissing() {
			kind = "MISSING"
		}
		return &p25vSpan{Start: uint32(node.StartByte()), End: uint32(node.EndByte()), Kind: kind}
	}
	for i := uint(0); i < node.ChildCount(); i++ {
		if span := p25vFirstCError(node.Child(i)); span != nil {
			return span
		}
	}
	return nil
}

func p25vRuntimeFrom(runtime gotreesitter.ParseRuntime) p25vRuntime {
	return p25vRuntime{
		StopReason:                          string(runtime.StopReason),
		Truncated:                           runtime.Truncated,
		TokenSourceEOFEarly:                 runtime.TokenSourceEOFEarly,
		RootEndByte:                         runtime.RootEndByte,
		RetryAttempts:                       runtime.IncrementalAcceptedErrorRetryAttempts,
		RetryAdopted:                        runtime.IncrementalAcceptedErrorRetryAdopted,
		RetryMergePerKey:                    runtime.IncrementalAcceptedErrorRetryMergePerKey,
		RetryCause:                          canonicalIncrementalRetryCause(runtime.IncrementalAcceptedErrorRetryCause),
		OldTreeReuseRoute:                   runtime.IncrementalOldTreeReuseRoute,
		CRecoveryEnteredErrorState:          runtime.CRecoveryEnteredErrorState,
		CRecoveryDroppedErrorForClean:       runtime.CRecoveryDroppedErrorForClean,
		CRecoverySwallowedFallbackAttempted: runtime.CRecoverySwallowedErrorFallbackAttempted,
		TokensConsumed:                      runtime.TokensConsumed,
		Iterations:                          runtime.Iterations,
		NodesAllocated:                      runtime.NodesAllocated,
		MaxStacksSeen:                       runtime.MaxStacksSeen,
		SingleStackIterations:               runtime.SingleStackIterations,
		MultiStackIterations:                runtime.MultiStackIterations,
		GSSNodesAllocated:                   runtime.GSSNodesAllocated,
		GSSNodesRetained:                    runtime.GSSNodesRetained,
		GSSNodesDropped:                     runtime.GSSNodesDroppedSameToken,
		ParentNodesAllocated:                runtime.ParentNodesAllocated,
		ParentNodesRetained:                 runtime.ParentNodesRetained,
		ParentNodesDropped:                  runtime.ParentNodesDroppedSameToken,
		LeafNodesAllocated:                  runtime.LeafNodesAllocated,
		LeafNodesRetained:                   runtime.LeafNodesRetained,
		LeafNodesDropped:                    runtime.LeafNodesDroppedSameToken,
		ArenaBytesAllocated:                 runtime.ArenaBytesAllocated,
		ScratchBytesAllocated:               runtime.ScratchBytesAllocated,
		GSSBytesAllocated:                   runtime.GSSBytesAllocated,
		MergeStacksIn:                       runtime.MergeStacksIn,
		MergeStacksOut:                      runtime.MergeStacksOut,
		MergeSlotsUsed:                      runtime.MergeSlotsUsed,
	}
}

func p25vProfileFrom(profile gotreesitter.IncrementalParseProfile) p25vProfile {
	return p25vProfile{
		ReuseCursorNanos:          profile.ReuseCursorNanos,
		ReparseNanos:              profile.ReparseNanos,
		ReusedSubtrees:            profile.ReusedSubtrees,
		ReusedBytes:               profile.ReusedBytes,
		NewNodesAllocated:         profile.NewNodesAllocated,
		RetryAttempts:             profile.AcceptedErrorRetryAttempts,
		RetryAdopted:              profile.AcceptedErrorRetryAdopted,
		RetryMergePerKey:          profile.AcceptedErrorRetryMergePerKey,
		RetryCause:                canonicalIncrementalRetryCause(profile.AcceptedErrorRetryCause),
		OldTreeReuseRoute:         profile.OldTreeReuseRoute,
		ReuseRejectDirty:          profile.ReuseRejectDirty,
		ReuseRejectHasError:       profile.ReuseRejectHasError,
		ReuseRejectFragileNonLeaf: profile.ReuseRejectFragileNonLeaf,
		RecoverSearches:           profile.RecoverSearches,
		RecoverStateChecks:        profile.RecoverStateChecks,
		RecoverHits:               profile.RecoverHits,
		TokensConsumed:            profile.TokensConsumed,
		MaxStacksSeen:             profile.MaxStacksSeen,
		SingleStackIterations:     profile.SingleStackIterations,
		MultiStackIterations:      profile.MultiStackIterations,
		GSSNodesAllocated:         profile.GSSNodesAllocated,
		GSSNodesRetained:          profile.GSSNodesRetained,
		GSSNodesDropped:           profile.GSSNodesDroppedSameToken,
		ParentNodesAllocated:      profile.ParentNodesAllocated,
		ParentNodesRetained:       profile.ParentNodesRetained,
		ParentNodesDropped:        profile.ParentNodesDroppedSameToken,
		LeafNodesAllocated:        profile.LeafNodesAllocated,
		LeafNodesRetained:         profile.LeafNodesRetained,
		LeafNodesDropped:          profile.LeafNodesDroppedSameToken,
		ArenaBytesAllocated:       profile.ArenaBytesAllocated,
		ScratchBytesAllocated:     profile.ScratchBytesAllocated,
		GSSBytesAllocated:         profile.GSSBytesAllocated,
		MergeStacksIn:             profile.MergeStacksIn,
		MergeStacksOut:            profile.MergeStacksOut,
		MergeSlotsUsed:            profile.MergeSlotsUsed,
		GlobalCullStacksIn:        profile.GlobalCullStacksIn,
		GlobalCullStacksOut:       profile.GlobalCullStacksOut,
	}
}
