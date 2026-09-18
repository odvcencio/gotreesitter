package grammars

import "testing"

func TestOwnedGrammarRegistrationPreservesDeclaredSource(t *testing.T) {
	preserveRegistryState(t)
	registered := DetectLanguageByName("go")
	if registered == nil {
		t.Fatal("Go registry entry is missing")
	}
	entry := *registered
	for _, source := range []GrammarSource{GrammarSourceUnknown, GrammarSourceTS2GoBlob, GrammarSourceGrammargenBlob} {
		entry.GrammarSource = source
		Register(entry)
		if got := DetectLanguageByName("go").GrammarSource; got != source {
			t.Fatalf("registration changed declared source %q to %q", source, got)
		}
	}
}

func TestTierOneGrammarOwnershipManifest(t *testing.T) {
	if got := len(grammarOwnershipManifest); got != 30 {
		t.Fatalf("Tier 1 grammar count = %d, want 30", got)
	}
	for name, ownership := range grammarOwnershipManifest {
		if ownership.Tier != 1 {
			t.Fatalf("%s tier = %d, want 1", name, ownership.Tier)
		}
	}
}

func TestOwnedGrammarRegistrySource(t *testing.T) {
	for _, name := range []string{"go", "yaml"} {
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
			want := GrammarSourceGrammargenBlob
			if name == "go" {
				want = GrammarSourceTS2GoBlob
			}
			if entry.GrammarSource != want {
				t.Fatalf("GrammarSource = %q, want %q", entry.GrammarSource, want)
			}
			if name == "go" && entry.Language().GeneratedByGrammargen {
				t.Fatal("Go blob provenance does not match its C-table registry source")
			}
		})
	}
}
