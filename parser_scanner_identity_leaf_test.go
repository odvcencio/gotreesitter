package gotreesitter_test

import (
	"bytes"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

type sqlCheckpointIdentityDriftScanner struct {
	gts.ExternalScanner
	identity gts.ExternalScannerCheckpointIdentity
}

func (s sqlCheckpointIdentityDriftScanner) SupportsIncrementalReuse() bool { return true }
func (s sqlCheckpointIdentityDriftScanner) UsesExternalScannerCheckpoints() bool {
	return true
}
func (s sqlCheckpointIdentityDriftScanner) CheckpointIdentity() (gts.ExternalScannerCheckpointIdentity, bool) {
	return s.identity, true
}

type checkpointCapabilityRemovedScanner struct {
	gts.ExternalScanner
}

func TestSQLTextInvariantLeafRejectsCheckpointIdentityDowngrade(t *testing.T) {
	for _, mode := range []string{"baseline", "identity drift", "capability removal"} {
		t.Run(mode, func(t *testing.T) {
			lang := grammars.SqlLanguage()
			originalScanner := lang.ExternalScanner
			t.Cleanup(func() { lang.ExternalScanner = originalScanner })
			parser := gts.NewParser(lang)
			source := []byte("SELECT alpha;\n")
			oldTree, err := parser.Parse(source)
			if err != nil {
				t.Fatalf("parse SQL identity source: %v", err)
			}
			defer oldTree.Release()

			switch mode {
			case "baseline":
			case "identity drift":
				provider, ok := originalScanner.(gts.ExternalScannerCheckpointIdentityProvider)
				if !ok {
					t.Fatalf("exact SQL scanner has no checkpoint identity: %T", originalScanner)
				}
				identity, ok := provider.CheckpointIdentity()
				if !ok {
					t.Fatal("exact SQL scanner returned no checkpoint identity")
				}
				identity.Scanner = append([]byte(nil), identity.Scanner...)
				identity.Scanner[0]++
				lang.ExternalScanner = sqlCheckpointIdentityDriftScanner{
					ExternalScanner: originalScanner,
					identity:        identity,
				}
			case "capability removal":
				lang.ExternalScanner = checkpointCapabilityRemovedScanner{ExternalScanner: originalScanner}
			}

			edited := bytes.Replace(source, []byte("alpha"), []byte("bravo"), 1)
			offset := bytes.Index(source, []byte("alpha"))
			oldTree.Edit(gts.InputEdit{
				StartByte:   uint32(offset),
				OldEndByte:  uint32(offset + len("alpha")),
				NewEndByte:  uint32(offset + len("bravo")),
				StartPoint:  pointForOffset(source, offset),
				OldEndPoint: pointForOffset(source, offset+len("alpha")),
				NewEndPoint: pointForOffset(edited, offset+len("bravo")),
			})
			incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
			if err != nil {
				t.Fatalf("parse SQL identity downgrade: %v", err)
			}
			defer incremental.Release()
			usedShortcut := profile.TokensConsumed == 0 && profile.ReusedBytes == uint64(len(source))
			requireReleaseSameWidthReparse(t, profile)
			if profile.NewNodesAllocated == 0 || usedShortcut {
				t.Fatalf("SQL edit bypassed actual reparsing: %+v", profile)
			}
			if mode == "baseline" && (profile.ReuseUnsupported || !profile.OldTreeReuseRoute ||
				profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 || profile.ReusedBytes >= uint64(len(source))) {
				t.Fatalf("SQL witness lost ordinary partial reuse: %+v", profile)
			}
			fresh, err := gts.NewParser(lang).Parse(edited)
			if err != nil {
				t.Fatalf("fresh SQL identity-downgrade parse: %v", err)
			}
			defer fresh.Release()
			requireIncrementalDeepTreeMatchesFresh(t, incremental, fresh, lang)
		})
	}
}
