package grammarruntime

import (
	"crypto/sha256"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestGoProfileCarriesNoOwnedEOFBundle(t *testing.T) {
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })
	lang := GoLanguage()
	if lang.CompactOwnedEOFRecoveryCertified || lang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("the Go blob built from the upstream grammar text received the earlier blob's owned EOF bundle or split drops")
	}
	if lang.CompactStrategy2ErrorRegionCertified || lang.CompactStackSummaryRecoveryCertified ||
		lang.CompactMissingTokenInsertionCertified || lang.CompactS5EOFMissingInsertionCertified ||
		lang.CompactFaithfulS5RecoveryCertified || lang.CompactRecoveryPlainFirstCertified ||
		lang.CompactRecoverEOFCertified {
		t.Fatal("the Go profile leaked shared-recovery capabilities")
	}
	custom := &gotreesitter.Language{Name: "go"}
	AttachLanguageSupport("go", custom)
	if custom.CompactOwnedEOFRecoveryCertified {
		t.Fatal("same-name custom Go received the owned EOF bundle")
	}
	stale := &gotreesitter.Language{Name: "go"}
	if attachBuiltinLanguageRuntimeProfile("go", sha256.Sum256([]byte("stale")), stale) || stale.SmallSourceReuseAuthority != nil {
		t.Fatal("stale Go received a profile")
	}
	exact := &gotreesitter.Language{Name: "go"}
	if !attachBuiltinLanguageRuntimeProfile("go", sha256.Sum256(BlobByName("go")), exact) || exact.CompactOwnedEOFRecoveryCertified {
		t.Fatal("exact Go blob did not attach its profile, or its profile granted the owned EOF bundle")
	}
	for name, profile := range builtinLanguageRuntimeProfiles {
		if profile.compactOwnedEOFRecovery {
			t.Fatalf("the owned EOF bundle unexpectedly certified %s", name)
		}
	}
	if !builtinLanguageRuntimeProfiles["scala"].compactFaithfulS5Recovery {
		t.Fatal("the independent Scala recovery profile changed")
	}
}
