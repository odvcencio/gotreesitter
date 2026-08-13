//go:build cgo && treesitter_c_parity && treesitter_c_perfscan

package cgoharness

import (
	"os"
	"path/filepath"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestB16RuntimeEvidenceCaptureSwiftWitnesses verifies the scan adapter on
// the outside-reported Swift witnesses and on a same-language clean control.
// The test records facts only; it does not change parser routing.
func TestB16RuntimeEvidenceCaptureSwiftWitnesses(t *testing.T) {
	t.Setenv(perfScanEnvRuntimeEvidence, "1")
	gotreesitter.EnableRecoveryRuntimeTelemetry(true)
	t.Cleanup(func() { gotreesitter.EnableRecoveryRuntimeTelemetry(false) })
	gotreesitter.EnableArenaBreakdown(true)
	t.Cleanup(func() { gotreesitter.EnableArenaBreakdown(false) })
	previousRoute := gotreesitter.AdmissionCandidateRouteDefault()
	gotreesitter.SetAdmissionCandidateRouteDefault(false)
	t.Cleanup(func() { gotreesitter.SetAdmissionCandidateRouteDefault(previousRoute) })

	witnesses := []struct {
		name         string
		path         string
		wantRecovery bool
	}{
		{
			name:         "swift-586-floating-point",
			path:         filepath.Join("..", "grammars", "testdata", "swift_corpus", "stdlib_FloatingPointToString.swift"),
			wantRecovery: true,
		},
		{
			name:         "swift-576-collection-algorithms",
			path:         filepath.Join("..", "grammars", "testdata", "swift_corpus", "stdlib_CollectionAlgorithms.swift"),
			wantRecovery: true,
		},
		{
			name:         "swift-clean-control",
			wantRecovery: false,
		},
	}

	for _, witness := range witnesses {
		t.Run(witness.name, func(t *testing.T) {
			source := []byte("let answer = 1\n")
			if witness.path != "" {
				var err error
				source, err = os.ReadFile(witness.path)
				if err != nil {
					t.Fatalf("read Swift witness: %v", err)
				}
			}

			parser := gotreesitter.NewParser(grammars.SwiftLanguage())
			parser.SetParsePhaseTiming(true)
			tree, err := parser.Parse(source)
			if err != nil {
				t.Fatalf("parse Swift witness: %v", err)
			}
			if tree == nil || tree.RootNode() == nil {
				t.Fatal("Swift witness returned no tree")
			}
			evidence := perfScanCaptureRuntimeEvidence(parser, tree)
			tree.Release()
			if evidence == nil || evidence.Recovery == nil || evidence.Parse == nil || evidence.Arena == nil {
				t.Fatalf("runtime evidence = %+v, want all sections", evidence)
			}
			if evidence.Parse.ParseWallNanos <= 0 || evidence.Parse.ParserLoopNanos <= 0 {
				t.Fatalf("phase timing = %+v, want positive parse and loop timing", evidence.Parse)
			}
			if !evidence.Recovery.Completed {
				t.Fatalf("recovery evidence is incomplete: %+v", evidence.Recovery)
			}
			if witness.wantRecovery {
				if evidence.Recovery.RecoveryEntryCount == 0 || evidence.Recovery.ErrorNodeCount == 0 {
					t.Fatalf("recovery witness lacks recovery facts: %+v", evidence.Recovery)
				}
				if evidence.Recovery.RetrySelectedAttempt == "" {
					t.Fatalf("recovery witness lacks selected retry facts: %+v", evidence.Recovery)
				}
			} else if evidence.Recovery.RecoveryEntryCount != 0 || evidence.Recovery.ErrorNodeCount != 0 {
				t.Fatalf("clean control recorded recovery facts: %+v", evidence.Recovery)
			}
		})
	}
}

func TestB16RuntimeEvidenceForcesProductionRoute(t *testing.T) {
	t.Setenv(perfScanEnvRuntimeEvidence, "1")
	gotreesitter.EnableRecoveryRuntimeTelemetry(true)
	t.Cleanup(func() { gotreesitter.EnableRecoveryRuntimeTelemetry(false) })
	gotreesitter.EnableArenaBreakdown(true)
	t.Cleanup(func() { gotreesitter.EnableArenaBreakdown(false) })
	previousRoute := gotreesitter.AdmissionCandidateRouteDefault()
	gotreesitter.SetAdmissionCandidateRouteDefault(false)
	t.Cleanup(func() { gotreesitter.SetAdmissionCandidateRouteDefault(previousRoute) })
	gotreesitter.SetGLRForestEnabled(true)
	t.Cleanup(func() { gotreesitter.SetGLRForestEnabled(true) })

	tests := []struct {
		name     string
		language *gotreesitter.Language
		source   []byte
	}{
		{
			name:     "automatic-forest-dispatch",
			language: grammars.JavascriptLanguage(),
			source:   []byte("const answer = 1;\n"),
		},
		{
			name:     "forest-recovery-replacement",
			language: grammars.KdlLanguage(),
			source:   []byte("node \"Hello\\n\\tWorld\" \"Hello\\n\\tWorld\" \"Hello\\n\\tWorld\" \"Hello\\n\\tWorld\" \"Hello\\n\\tWorld\"\n"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			control := gotreesitter.NewParser(test.language)
			controlTree, err := control.Parse(test.source)
			if err != nil {
				t.Fatalf("parse forest control: %v", err)
			}
			if controlTree == nil || !controlTree.UsedForestFastPath() {
				t.Fatal("control did not use the forest route")
			}
			controlTree.Release()

			parser := gotreesitter.NewParser(test.language)
			perfScanConfigureParserForRuntimeEvidence(parser, true)
			parser.SetParsePhaseTiming(true)
			tree, err := parser.Parse(test.source)
			if err != nil {
				t.Fatalf("parse production control: %v", err)
			}
			if tree == nil {
				t.Fatal("production control returned no tree")
			}
			if tree.UsedForestFastPath() {
				t.Fatal("runtime evidence returned a forest tree")
			}
			evidence := perfScanCaptureRuntimeEvidence(parser, tree)
			tree.Release()
			if evidence == nil || evidence.Recovery == nil || evidence.Parse == nil || evidence.Arena == nil {
				t.Fatalf("runtime evidence = %+v, want all production sections", evidence)
			}
			if evidence.Parse.ParseWallNanos <= 0 || evidence.Parse.ParserLoopNanos <= 0 {
				t.Fatalf("phase timing = %+v, want positive production timing", evidence.Parse)
			}
		})
	}
}
