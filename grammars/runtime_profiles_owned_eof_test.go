package grammars

import (
	"crypto/sha256"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestGoOwnedEOFRecoveryProfileRequiresExactBlob(t *testing.T) {
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })
	lang := GoLanguage()
	if !lang.CompactOwnedEOFRecoveryCertified || !lang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("exact Go profile lacks the owned EOF bundle or existing split drops")
	}
	if lang.CompactStrategy2ErrorRegionCertified || lang.CompactStackSummaryRecoveryCertified ||
		lang.CompactMissingTokenInsertionCertified || lang.CompactS5EOFMissingInsertionCertified ||
		lang.CompactFaithfulS5RecoveryCertified || lang.CompactRecoveryPlainFirstCertified ||
		lang.CompactRecoverEOFCertified {
		t.Fatal("the Go bundle leaked independent shared-recovery capabilities")
	}
	custom := &gotreesitter.Language{Name: "go"}
	AttachLanguageSupport("go", custom)
	if custom.CompactOwnedEOFRecoveryCertified {
		t.Fatal("same-name custom Go received the owned EOF bundle")
	}
	stale := &gotreesitter.Language{Name: "go"}
	if attachBuiltinLanguageRuntimeProfile("go", sha256.Sum256([]byte("stale")), stale) || stale.CompactOwnedEOFRecoveryCertified {
		t.Fatal("stale Go received the owned EOF bundle")
	}
	exact := &gotreesitter.Language{Name: "go"}
	if !attachBuiltinLanguageRuntimeProfile("go", sha256.Sum256(BlobByName("go")), exact) || !exact.CompactOwnedEOFRecoveryCertified {
		t.Fatal("exact Go blob did not receive the owned EOF bundle")
	}
	for name, profile := range builtinLanguageRuntimeProfiles {
		if name != "go" && profile.compactOwnedEOFRecovery {
			t.Fatalf("the owned EOF bundle unexpectedly certified %s", name)
		}
	}
	if !builtinLanguageRuntimeProfiles["scala"].compactFaithfulS5Recovery {
		t.Fatal("the independent Scala recovery profile changed")
	}
}
