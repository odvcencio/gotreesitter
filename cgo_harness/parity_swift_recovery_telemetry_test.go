//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestB16SwiftRecoveryTelemetryCOracle records the locked-C identity for the
// Swift witnesses. Keep this correctness receipt separate from benchmarks.
func TestB16SwiftRecoveryTelemetryCOracle(t *testing.T) {
	previousAdmissionRoute := gotreesitter.AdmissionCandidateRouteDefault()
	gotreesitter.SetAdmissionCandidateRouteDefault(false)
	t.Cleanup(func() { gotreesitter.SetAdmissionCandidateRouteDefault(previousAdmissionRoute) })
	gotreesitter.EnableRecoveryRuntimeTelemetry(true)
	t.Cleanup(func() { gotreesitter.EnableRecoveryRuntimeTelemetry(false) })

	identity, err := COracleIdentity("swift")
	if err != nil {
		t.Fatalf("load Swift C oracle identity: %v", err)
	}
	cLang, err := ParityCLanguage("swift")
	if err != nil {
		t.Fatalf("load Swift C language: %v", err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatalf("set Swift C language: %v", err)
	}

	witnesses := []struct {
		name         string
		issue        string
		path         string
		wantHasError bool
	}{
		{
			name:         "swift-586-floating-point",
			issue:        "#586/#576",
			path:         filepath.Join("..", "grammars", "testdata", "swift_corpus", "stdlib_FloatingPointToString.swift"),
			wantHasError: true,
		},
		{
			name:         "swift-576-collection-algorithms",
			issue:        "#576",
			path:         filepath.Join("..", "grammars", "testdata", "swift_corpus", "stdlib_CollectionAlgorithms.swift"),
			wantHasError: true,
		},
		{
			name:         "swift-clean-control",
			issue:        "control",
			path:         "",
			wantHasError: false,
		},
	}

	for _, witness := range witnesses {
		t.Run(witness.name, func(t *testing.T) {
			var source []byte
			if witness.path == "" {
				source = []byte("let answer = 1\n")
			} else {
				source, err = os.ReadFile(witness.path)
				if err != nil {
					t.Fatalf("read Swift witness %q: %v", witness.path, err)
				}
			}

			goLang := grammars.SwiftLanguage()
			goParser := gotreesitter.NewParser(goLang)
			goTree, parseErr := goParser.Parse(source)
			if parseErr != nil {
				t.Fatalf("parse Swift witness with Go: %v", parseErr)
			}
			if goTree == nil || goTree.RootNode() == nil {
				t.Fatal("Go Swift witness returned no tree")
			}
			goRoot := goTree.RootNode()
			goRuntime := goTree.ParseRuntime()
			goMemo := goTree.RecoveryNodeMemoRuntime()
			goStats := goParser.DebugRecoveryRuntimeStats()
			goInspection, err := benchfixtures.InspectGoTree(goRoot, goLang)
			if err != nil {
				goTree.Release()
				t.Fatalf("inspect Go Swift witness: %v", err)
			}

			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				goTree.Release()
				t.Fatal("C Swift witness returned no tree")
			}
			cRoot := cTree.RootNode()
			cDigest, err := COracleDeepDigest(cTree)
			if err != nil {
				cTree.Close()
				goTree.Release()
				t.Fatalf("inspect C Swift witness: %v", err)
			}

			if goRoot.HasError() != witness.wantHasError || cRoot.HasError() != witness.wantHasError {
				cTree.Close()
				goTree.Release()
				t.Fatalf("HasError() = Go:%v C:%v, want %v/%v", goRoot.HasError(), cRoot.HasError(), witness.wantHasError, witness.wantHasError)
			}
			if goRoot.StartByte() != 0 || goRoot.EndByte() != uint32(len(source)) || cRoot.StartByte() != 0 || cRoot.EndByte() != uint(len(source)) {
				cTree.Close()
				goTree.Release()
				t.Fatalf("root span = Go:%d..%d C:%d..%d, want 0..%d", goRoot.StartByte(), goRoot.EndByte(), cRoot.StartByte(), cRoot.EndByte(), len(source))
			}
			if !goStats.Enabled || !goStats.Completed {
				cTree.Close()
				goTree.Release()
				t.Fatalf("telemetry status = enabled:%v completed:%v, want enabled and completed", goStats.Enabled, goStats.Completed)
			}

			sourceDigest := sha256.Sum256(source)
			t.Logf("B16_C_WITNESS version=1 name=%s issue=%s source_sha256=%x source_bytes=%d grammar=swift grammar_lock_commit=%s c_runtime=%s@%s c_grammar_repo=%s c_grammar_commit=%s c_grammar_artifact_sha256=%s result_class=%s go_deep_sha256=%s c_deep_sha256=%s go_stop_reason=%s go_truncated=%t go_recovery_entries=%d go_strategy1_elections=%d go_cost_competitions=%d go_cost_walks=%d go_cost_walk_ns=%d go_error_nodes=%d go_error_span_bytes=%d go_retry_passes=%d go_retry_reason=%q go_memo_tier=%d go_memo_entries=%d go_memo_bytes=%d go_memo_collisions=%d go_arena_bytes=%d go_scratch_bytes=%d go_entry_scratch_bytes=%d go_gss_bytes=%d go_gss_nodes=%d go_max_stacks=%d",
				witness.name,
				witness.issue,
				sourceDigest,
				len(source),
				identity.GrammarCommit,
				identity.RuntimeVersion,
				identity.RuntimeCommit,
				identity.GrammarRepo,
				identity.GrammarCommit,
				identity.GrammarArtifactSHA256,
				map[bool]string{true: "error", false: "clean"}[goRoot.HasError()],
				goInspection.SHA256,
				cDigest,
				goRuntime.StopReason,
				goRuntime.Truncated,
				goStats.RecoveryEntryCount,
				goStats.Strategy1ElectionCount,
				goStats.RecoveryCostCompetitionCount,
				goStats.RecoveryCostWalkCount,
				goStats.RecoveryCostWalkNanos,
				goStats.ErrorNodeCount,
				goStats.ErrorSpanBytes,
				goStats.RetryPassCount,
				goStats.RetryReason,
				goMemo.PeakTier,
				goMemo.PeakTier.Entries(),
				goMemo.PeakTier.Bytes(),
				goMemo.Collisions,
				goRuntime.ArenaBytesAllocated,
				goRuntime.ScratchBytesAllocated,
				goRuntime.EntryScratchBytesAllocated,
				goRuntime.GSSBytesAllocated,
				goRuntime.GSSNodesUsed,
				goRuntime.MaxStacksSeen,
			)
			cTree.Close()
			goTree.Release()
		})
	}
}
