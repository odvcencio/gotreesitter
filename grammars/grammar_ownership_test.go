package grammars

import "testing"

func TestTierOneGrammarOwnershipManifest(t *testing.T) {
	tierOneCount := 0
	for name, ownership := range grammarOwnershipManifest {
		if ownership.Tier == 1 {
			tierOneCount++
			continue
		}
		// A manifest entry outside Tier 1 must still be Own: the manifest
		// otherwise holds only the Tier 1 set (see docs/grammar-ownership.md).
		if ownership.MaintenanceClass != GrammarMaintenanceOwn {
			t.Fatalf("%s tier = %d, want 1 (non-Tier-1 manifest entries must be Own)", name, ownership.Tier)
		}
	}
	if tierOneCount != 30 {
		t.Fatalf("Tier 1 grammar count = %d, want 30", tierOneCount)
	}
}

func TestOwnedGrammarRegistrySource(t *testing.T) {
	for _, name := range []string{"go", "yaml", "swift", "regex"} {
		t.Run(name, func(t *testing.T) {
			ownership, ok := GrammarOwnershipFor(name)
			if !ok {
				t.Fatalf("missing ownership record for %q", name)
			}
			if ownership.MaintenanceClass != GrammarMaintenanceOwn {
				t.Fatalf("maintenance class = %q, want %q", ownership.MaintenanceClass, GrammarMaintenanceOwn)
			}
			if ownership.UpstreamRepo == "" || ownership.UpstreamCommit == "" || ownership.UpstreamLicense == "" {
				t.Fatalf("owned grammar lacks complete upstream provenance: %+v", ownership)
			}
			entry := DetectLanguageByName(name)
			if entry == nil {
				t.Fatalf("missing registry entry for %q", name)
			}
			if entry.GrammarSource != GrammarSourceGrammargenBlob {
				t.Fatalf("GrammarSource = %q, want %q", entry.GrammarSource, GrammarSourceGrammargenBlob)
			}
		})
	}
}
