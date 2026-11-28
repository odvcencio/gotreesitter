package grammarruntime

import "testing"

func TestGoMissingTurnRangeExclusionRequiresFrozenArtifact(t *testing.T) {
	// The scanner serves only the compact scheduler's frozen artifact.
	current := mustRuntimeProfileSHA256("67b873053c9917055e1b5aec94d3c0d9f7d26834f4340a39159e5970c0a0a598")
	previous := mustRuntimeProfileSHA256("214322327c9ee7dda29febf32e415f7ed9e00a90b7690d3c32522e393211b8da")
	scanner := GoExternalScanner{}
	if !scanner.ExcludesMissingVersionTurnsForIncludedRanges(current) {
		t.Fatal("current artifact lacks its unqualified-mode containment")
	}
	if scanner.ExcludesMissingVersionTurnsForIncludedRanges(previous) || scanner.ExcludesMissingVersionTurnsForIncludedRanges([32]byte{}) {
		t.Fatal("current-artifact containment changed an older or unknown artifact")
	}
	changed := current
	changed[0] ^= 1
	if scanner.ExcludesMissingVersionTurnsForIncludedRanges(changed) {
		t.Fatal("a different artifact acquired the exclusion")
	}
	if !builtinLanguageRuntimeProfiles["sql"].recoveryMissingVersionTurns {
		t.Fatal("the existing SQL profile grant changed")
	}
}
