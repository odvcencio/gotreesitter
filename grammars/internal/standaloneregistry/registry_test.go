package standaloneregistry

import "testing"

func TestRegisterReplacesEntryAndSnapshotOwnsSlices(t *testing.T) {
	extensions := []string{".standalone-registry-test"}
	Register(Entry{Name: "standalone-registry-test", Extensions: extensions})
	extensions[0] = ".mutated"

	entries, firstGeneration := Snapshot()
	entry := findTestEntry(t, entries)
	if got := entry.Extensions[0]; got != ".standalone-registry-test" {
		t.Fatalf("registered extension = %q", got)
	}
	entry.Extensions[0] = ".snapshot-mutation"
	entries, _ = Snapshot()
	if got := findTestEntry(t, entries).Extensions[0]; got != ".standalone-registry-test" {
		t.Fatalf("stored extension after snapshot mutation = %q", got)
	}

	Register(Entry{Name: "standalone-registry-test", Aliases: []string{"standalone-test"}})
	entries, secondGeneration := Snapshot()
	if secondGeneration != firstGeneration+1 {
		t.Fatalf("generation = %d, want %d", secondGeneration, firstGeneration+1)
	}
	entry = findTestEntry(t, entries)
	if len(entry.Extensions) != 0 || len(entry.Aliases) != 1 || entry.Aliases[0] != "standalone-test" {
		t.Fatalf("replacement entry = %+v", entry)
	}
}

func findTestEntry(t *testing.T, entries []Entry) Entry {
	t.Helper()
	for _, entry := range entries {
		if entry.Name == "standalone-registry-test" {
			return entry
		}
	}
	t.Fatal("standalone registry test entry is absent")
	return Entry{}
}
