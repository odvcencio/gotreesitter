package grammarruntime

import "testing"

func TestGoMissingTurnRangeExclusionRequiresCurrentExactArtifact(t *testing.T) {
	current := mustRuntimeProfileSHA256("9ad1f8d7d0f2b9aa32c61bd8295b56177caabb0ae2fb8dd4617559ee0f71fed4")
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
	lang := GoLanguage()
	identity, ok := lang.GrammarBlobSHA256()
	if !ok || identity != current || !lang.RecoveryMissingVersionTurnsCertified {
		t.Fatalf("run against the exact existing current-artifact certification; identity=%x valid=%v certified=%v", identity, ok, lang.RecoveryMissingVersionTurnsCertified)
	}
	bound, ok := lang.ExternalScanner.(interface {
		ExcludesMissingVersionTurnsForIncludedRanges([32]byte) bool
	})
	if !ok || !bound.ExcludesMissingVersionTurnsForIncludedRanges(identity) {
		t.Fatal("the bound scanner lost its artifact-owned mode exclusion")
	}
	if !builtinLanguageRuntimeProfiles["sql"].recoveryMissingVersionTurns {
		t.Fatal("the existing SQL profile grant changed")
	}
}
